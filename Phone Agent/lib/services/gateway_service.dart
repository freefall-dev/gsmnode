import 'dart:async';

import 'package:flutter/foundation.dart';

import '../config.dart';
import '../models/message.dart';
import 'api_client.dart';
import 'crypto_service.dart';
import 'sms_service.dart';

/// A single line in the on-screen activity log.
class LogEntry {
  final DateTime time;
  final String text;
  final bool error;
  LogEntry(this.text, {this.error = false}) : time = DateTime.now();
}

/// Orchestrates the gateway loop: polls the API Server for pending messages,
/// sends them over the radio, reports delivery state, forwards incoming SMS to
/// the inbox, and sends heartbeats.
///
/// This runs in the foreground while the app is open. Moving the loop to a
/// persistent background/foreground service (WorkManager) is a documented next
/// step — see README.
class GatewayService extends ChangeNotifier {
  final ApiClient api;
  final SmsService sms;

  GatewayService(this.api, this.sms);

  bool _running = false;
  bool get running => _running;

  final List<LogEntry> _log = [];
  List<LogEntry> get log => List.unmodifiable(_log);

  Timer? _pollTimer;
  Timer? _pingTimer;
  StreamSubscription<IncomingSms>? _incomingSub;
  StreamSubscription<SmsStatus>? _statusSub;
  StreamSubscription<IncomingCall>? _callSub;
  bool _polling = false;

  /// Messages pulled before their `schedule_at`, held until they come due.
  final List<GatewayMessage> _deferred = [];

  /// Rebuilt on each use so a passphrase change in Settings takes effect without
  /// restarting the gateway. Encryption is off when no passphrase is set.
  CryptoService get _crypto => CryptoService(api.storage.encPassphrase);

  void start() {
    if (_running) return;
    _running = true;
    api.storage.gatewayEnabled = true;
    _log0('Gateway started');

    // Keep the process alive while the screen is off / app backgrounded.
    sms.startBackgroundService().catchError(
        (e) => _log0('Foreground service failed to start: $e', error: true));

    _incomingSub = sms.incomingSms().listen(_onIncoming, onError: (e) {
      _log0('Incoming SMS stream error: $e', error: true);
    });
    _statusSub = sms.smsStatus().listen(_onStatus, onError: (e) {
      _log0('Status stream error: $e', error: true);
    });
    _callSub = sms.incomingCalls().listen(_onIncomingCall, onError: (e) {
      _log0('Incoming call stream error: $e', error: true);
    });

    _pollTimer = Timer.periodic(AppConfig.pollInterval, (_) => _poll());
    _pingTimer = Timer.periodic(AppConfig.pingInterval, (_) => _ping());
    _poll();
    _ping();
    _logSims();
    notifyListeners();
  }

  /// Restores a gateway the user had left running, after a cold start.
  ///
  /// Normally the cached engine keeps this object alive across the app being
  /// closed, so there is nothing to restore. This covers the harder case: the
  /// system killed the process and START_STICKY brought the service back, so
  /// Dart starts from scratch while the notification still promises a running
  /// gateway. A lingering service with the preference unset means the flag
  /// predates this build — believe what the user can see.
  Future<void> resume() async {
    if (_running || !api.storage.isRegistered) return;
    var wanted = api.storage.gatewayEnabled;
    if (!wanted) {
      wanted = await sms.isServiceRunning().catchError((_) => false);
    }
    if (wanted) start();
  }

  /// Logs the detected SIMs once (best-effort; enumeration needs the phone
  /// permission, which may still be pending when the gateway first starts).
  Future<void> _logSims() async {
    try {
      final sims = await sms.getSims();
      if (sims.isEmpty) return;
      final desc = sims
          .map((s) => 'slot ${s.slot}: ${s.carrier.isEmpty ? "SIM" : s.carrier}')
          .join(', ');
      _log0('SIMs detected — $desc');
    } catch (_) {
      // SIM enumeration unsupported or permission not granted; ignore.
    }
  }

  void stop() {
    _running = false;
    api.storage.gatewayEnabled = false;
    _pollTimer?.cancel();
    _pingTimer?.cancel();
    _incomingSub?.cancel();
    _statusSub?.cancel();
    _callSub?.cancel();
    sms.stopBackgroundService().catchError((_) {});
    // Best-effort: tell the server we've gone quiet so the consoles don't show
    // this phone as online until the heartbeat times out. If it fails (no
    // network — the usual reason the gateway is stopping) the timeout still
    // catches it.
    api.reportOffline().catchError((_) {});
    _log0('Gateway stopped');
    notifyListeners();
  }

  /// Merges freshly pulled messages with any parked for a future `schedule_at`,
  /// returning those ready to send now and re-parking the rest.
  ///
  /// The server already withholds scheduled messages until they come due; this
  /// only catches one handed over early by an older server. A parked message has
  /// already been marked Processed server-side, so if the app dies before it
  /// comes due the expiry sweeper fails it — which beats sending at the wrong
  /// time.
  List<GatewayMessage> _dueFrom(List<GatewayMessage> pulled) {
    final due = <GatewayMessage>[];
    for (final m in [..._deferred, ...pulled]) {
      if (m.isDue) {
        due.add(m);
      } else if (!_deferred.any((d) => d.id == m.id)) {
        _deferred.add(m);
        _log0('Holding ${m.id} until '
            '${m.scheduleAt!.toString().substring(0, 16)}');
      }
    }
    _deferred.removeWhere((m) => m.isDue);
    return due;
  }

  Future<void> _poll() async {
    if (_polling || !_running) return;
    _polling = true;
    try {
      // A failed pull must not strand messages already parked for a schedule, so
      // it is logged here and the loop continues with whatever is in hand.
      var pulled = const <GatewayMessage>[];
      try {
        pulled = await api.pullMessages();
      } catch (e) {
        _log0('Poll error: $e', error: true);
      }

      for (final m in _dueFrom(pulled)) {
        var handedOff = true;

        // Decrypt recipients + text when the message is end-to-end encrypted.
        // If we can't (no/wrong passphrase), fail the message rather than
        // sending ciphertext over the radio.
        List<String> recipients = m.phoneNumbers;
        String text = m.textMessage;
        if (m.encrypted) {
          try {
            recipients = [for (final p in m.phoneNumbers) await _crypto.decrypt(p)];
            text = await _crypto.decrypt(m.textMessage);
          } catch (e) {
            await api.reportMessage(m.id, 'Failed',
                error: 'decrypt failed (passphrase?): $e');
            _log0('Decrypt failed for ${m.id}: $e', error: true);
            continue;
          }
        }

        for (final phone in recipients) {
          try {
            if (m.isCall) {
              await sms.placeCall(phone, simSlot: m.simNumber);
              final on = m.simNumber != null ? ' on SIM ${m.simNumber}' : '';
              _log0('Calling $phone$on');
            } else if (m.isData) {
              await sms.sendDataSms(phone, m.dataPayload, m.dataPort ?? 0,
                  simSlot: m.simNumber, messageId: m.id);
              _log0('Sending data SMS to $phone (port ${m.dataPort ?? 0})');
            } else if (m.isMms) {
              await sms.sendMms(phone,
                  subject: m.subject,
                  text: text,
                  attachments: m.attachments.map((a) => a.toJson()).toList(),
                  simSlot: m.simNumber,
                  messageId: m.id);
              _log0('Sending MMS to $phone (${m.attachments.length} attachment(s))');
            } else {
              await sms.sendSms(phone, text,
                  simSlot: m.simNumber, messageId: m.id);
              _log0('Sending to $phone: "${_short(text)}"');
            }
          } catch (e) {
            handedOff = false;
            await api.reportMessage(m.id, 'Failed', error: e.toString());
            _log0('${m.isCall ? "Call" : "Send"} to $phone failed: $e',
                error: true);
            break;
          }
        }
        // Report Sent once handed off. For SMS the native send/delivery
        // PendingIntents then refine this to Delivered (or Failed) via _onStatus;
        // a call has no delivery report, so it stays Sent.
        if (handedOff) await api.reportMessage(m.id, 'Sent');
      }
    } catch (e) {
      _log0('Poll error: $e', error: true);
    } finally {
      _polling = false;
    }
  }

  Future<void> _ping() async {
    if (!_running) return;
    try {
      // Report the current SIMs alongside the heartbeat so the server can
      // advertise real slot choices. Only send when we actually have them, so a
      // pending permission doesn't clobber a previously reported list.
      List<Map<String, dynamic>>? sims;
      try {
        final list = await sms.getSims();
        if (list.isNotEmpty) {
          sims = list.map((s) => s.toJson()).toList(growable: false);
        }
      } catch (_) {
        // SIM enumeration unsupported or permission not granted; skip.
      }
      await api.ping(sims: sims);
    } catch (e) {
      _log0('Ping failed: $e', error: true);
    }
  }

  Future<void> _onStatus(SmsStatus s) async {
    final id = s.messageId;
    if (id == null || id.isEmpty) return;
    try {
      if (s.kind == 'delivered') {
        if (s.success) {
          await api.reportMessage(id, 'Delivered');
          _log0('Delivered: $id');
        } else {
          await api.reportMessage(id, 'Failed', error: 'delivery failed');
          _log0('Delivery failed: $id', error: true);
        }
      } else if (s.kind == 'sent' && !s.success) {
        await api.reportMessage(id, 'Failed', error: 'radio rejected message');
        _log0('Send rejected by radio: $id', error: true);
      }
    } catch (e) {
      _log0('Report status failed: $e', error: true);
    }
  }

  Future<void> _onIncoming(IncomingSms msg) async {
    final on = msg.simSlot != null ? ' on SIM ${msg.simSlot}' : '';
    final label = msg.type == 'sms' ? '' : ' [${msg.type}]';
    _log0('Received$label from ${msg.from}$on: "${_short(msg.body)}"');
    try {
      // Encrypt sender + body when a passphrase is set, so the server only ever
      // stores ciphertext. Data payloads / MMS attachments are left as-is.
      final crypto = _crypto;
      final enc = crypto.enabled && msg.type != 'data';
      final from = enc ? await crypto.encrypt(msg.from) : msg.from;
      final body = enc ? await crypto.encrypt(msg.body) : msg.body;
      await api.postInbox(
        from,
        body,
        type: msg.type,
        receivedAt: msg.timestamp,
        simSlot: msg.simSlot,
        encrypted: enc,
        dataPayload: msg.type == 'data' ? msg.dataPayload : null,
        dataPort: msg.type == 'data' ? msg.dataPort : null,
        subject: msg.type == 'mms' ? msg.subject : null,
        attachments: msg.type == 'mms' ? msg.attachments : null,
      );
    } catch (e) {
      _log0('Forward inbox failed: $e', error: true);
    }
  }

  Future<void> _onIncomingCall(IncomingCall call) async {
    final on = call.simSlot != null ? ' on SIM ${call.simSlot}' : '';
    _log0('${call.direction == "outgoing" ? "Outgoing" : "Incoming"} call '
        '${call.number}$on — ${call.status}');
    try {
      await api.reportCall(
        call.number,
        direction: call.direction,
        status: call.status,
        simSlot: call.simSlot,
        duration: call.duration,
        startedAt: call.timestamp,
      );
    } catch (e) {
      _log0('Report call failed: $e', error: true);
    }
  }

  void _log0(String text, {bool error = false}) {
    _log.insert(0, LogEntry(text, error: error));
    if (_log.length > 100) _log.removeLast();
    notifyListeners();
  }

  String _short(String s) => s.length > 40 ? '${s.substring(0, 40)}…' : s;

  @override
  void dispose() {
    stop();
    super.dispose();
  }
}
