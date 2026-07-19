import 'dart:async';

import 'package:flutter/foundation.dart';

import '../config.dart';
import 'api_client.dart';
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
  bool _polling = false;

  void start() {
    if (_running) return;
    _running = true;
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

    _pollTimer = Timer.periodic(AppConfig.pollInterval, (_) => _poll());
    _pingTimer = Timer.periodic(AppConfig.pingInterval, (_) => _ping());
    _poll();
    _ping();
    _logSims();
    notifyListeners();
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
    _pollTimer?.cancel();
    _pingTimer?.cancel();
    _incomingSub?.cancel();
    _statusSub?.cancel();
    sms.stopBackgroundService().catchError((_) {});
    _log0('Gateway stopped');
    notifyListeners();
  }

  Future<void> _poll() async {
    if (_polling || !_running) return;
    _polling = true;
    try {
      final messages = await api.pullMessages();
      for (final m in messages) {
        var handedOff = true;
        for (final phone in m.phoneNumbers) {
          try {
            if (m.isCall) {
              await sms.placeCall(phone);
              _log0('Calling $phone');
            } else {
              await sms.sendSms(phone, m.textMessage,
                  simSlot: m.simNumber, messageId: m.id);
              _log0('Sending to $phone: "${_short(m.textMessage)}"');
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
    _log0('Received from ${msg.from}$on: "${_short(msg.body)}"');
    try {
      await api.postInbox(msg.from, msg.body,
          receivedAt: msg.timestamp, simSlot: msg.simSlot);
    } catch (e) {
      _log0('Forward inbox failed: $e', error: true);
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
