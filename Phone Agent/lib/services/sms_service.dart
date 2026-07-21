import 'package:flutter/services.dart';

/// An incoming message delivered from the native side. [type] is 'sms', 'data',
/// or 'mms'. For data SMS, [dataPayload] holds base64 bytes on [dataPort]. For
/// MMS, [subject] and [attachments] carry the multimedia parts.
class IncomingSms {
  final String from;
  final String body;
  final DateTime timestamp;
  final String type; // 'sms' | 'data' | 'mms'

  /// 0-based physical SIM slot the message arrived on, or null if the device
  /// couldn't attribute it (single-SIM, or READ_PHONE_STATE not granted).
  final int? simSlot;

  // Data SMS.
  final String dataPayload; // base64
  final int? dataPort;

  // MMS.
  final String subject;
  final List<Map<String, dynamic>> attachments; // [{filename, content_type, data}]

  IncomingSms(
    this.from,
    this.body,
    this.timestamp, {
    this.type = 'sms',
    this.simSlot,
    this.dataPayload = '',
    this.dataPort,
    this.subject = '',
    this.attachments = const [],
  });
}

/// An incoming/outgoing call event surfaced by the native call receiver.
class IncomingCall {
  final String number;
  final String direction; // 'incoming' | 'outgoing'
  final String status; // ringing | missed | answered | completed | rejected
  final int? simSlot;
  final int? duration; // seconds, when known
  final DateTime timestamp;

  IncomingCall(
    this.number,
    this.direction,
    this.status,
    this.timestamp, {
    this.simSlot,
    this.duration,
  });
}

/// A SIM card active in the device, as enumerated by the native side.
class SimInfo {
  final int slot; // 0-based physical slot index
  final int subscriptionId;
  final String carrier;
  final String number;
  final String displayName;

  SimInfo({
    required this.slot,
    required this.subscriptionId,
    required this.carrier,
    required this.number,
    required this.displayName,
  });

  factory SimInfo.fromMap(Map<dynamic, dynamic> m) => SimInfo(
        slot: (m['slot'] as num?)?.toInt() ?? 0,
        subscriptionId: (m['subscription_id'] as num?)?.toInt() ?? 0,
        carrier: m['carrier'] as String? ?? '',
        number: m['number'] as String? ?? '',
        displayName: m['display_name'] as String? ?? '',
      );

  Map<String, dynamic> toJson() => {
        'slot': slot,
        'subscription_id': subscriptionId,
        'carrier': carrier,
        'number': number,
        'display_name': displayName,
      };
}

/// A send/delivery status report for an outbound message.
class SmsStatus {
  final String? messageId;
  final String kind; // 'sent' or 'delivered'
  final bool success;
  SmsStatus(this.messageId, this.kind, this.success);
}

/// Bridges to the native Android side: SmsManager (text/data/MMS sending),
/// BroadcastReceivers (incoming SMS/data/MMS + calls), send/delivery
/// PendingIntents (status), and the foreground service. See android_overlay/.
class SmsService {
  static const _method = MethodChannel('app.gsmnode/sms');
  static const _events = EventChannel('app.gsmnode/sms_incoming');
  static const _status = EventChannel('app.gsmnode/sms_status');
  static const _calls = EventChannel('app.gsmnode/call_incoming');

  /// Sends a text SMS via the device radio. [simSlot] is 0-based; null = default.
  /// [messageId] is echoed back on the status stream for correlation.
  Future<void> sendSms(String phoneNumber, String message,
      {int? simSlot, String? messageId}) async {
    await _method.invokeMethod('sendSms', {
      'phone': phoneNumber,
      'message': message,
      'simSlot': simSlot,
      'messageId': messageId,
    });
  }

  /// Sends a binary data SMS to [port]. [payloadBase64] is the base64-encoded
  /// bytes to transmit.
  Future<void> sendDataSms(String phoneNumber, String payloadBase64, int port,
      {int? simSlot, String? messageId}) async {
    await _method.invokeMethod('sendDataSms', {
      'phone': phoneNumber,
      'payload': payloadBase64,
      'port': port,
      'simSlot': simSlot,
      'messageId': messageId,
    });
  }

  /// Sends an MMS with an optional [subject]/[text] and [attachments]
  /// ([{filename, content_type, data(base64)}]). Best-effort: real delivery
  /// depends on the carrier's MMSC / APN configuration.
  Future<void> sendMms(String phoneNumber,
      {String subject = '',
      String text = '',
      List<Map<String, dynamic>> attachments = const [],
      int? simSlot,
      String? messageId}) async {
    await _method.invokeMethod('sendMms', {
      'phone': phoneNumber,
      'subject': subject,
      'text': text,
      'attachments': attachments,
      'simSlot': simSlot,
      'messageId': messageId,
    });
  }

  /// Enumerates the active SIMs on the device (empty if READ_PHONE_STATE isn't
  /// granted or the device has no telephony).
  Future<List<SimInfo>> getSims() async {
    final res = await _method.invokeMethod('getSims');
    final list = (res as List?) ?? const [];
    return list.map((e) => SimInfo.fromMap(e as Map)).toList(growable: false);
  }

  /// Places a phone call to [phoneNumber]. Requires the CALL_PHONE permission.
  Future<void> placeCall(String phoneNumber) async {
    await _method.invokeMethod('placeCall', {'phone': phoneNumber});
  }

  Future<void> startBackgroundService() => _method.invokeMethod('startService');
  Future<void> stopBackgroundService() => _method.invokeMethod('stopService');

  /// Whether the foreground service is live — the notification the user sees.
  Future<bool> isServiceRunning() async =>
      (await _method.invokeMethod<bool>('isServiceRunning')) ?? false;

  /// Stream of incoming SMS / data SMS / MMS (active while the process is alive).
  Stream<IncomingSms> incomingSms() {
    return _events.receiveBroadcastStream().map((event) {
      final map = Map<String, dynamic>.from(event as Map);
      final ts = map['timestamp'] as int?;
      final slot = (map['simSlot'] as num?)?.toInt();
      final atts = (map['attachments'] as List?)
              ?.map((e) => Map<String, dynamic>.from(e as Map))
              .toList() ??
          const <Map<String, dynamic>>[];
      return IncomingSms(
        map['from'] as String? ?? '',
        map['body'] as String? ?? '',
        ts != null ? DateTime.fromMillisecondsSinceEpoch(ts) : DateTime.now(),
        type: map['type'] as String? ?? 'sms',
        simSlot: (slot != null && slot >= 0) ? slot : null,
        dataPayload: map['dataPayload'] as String? ?? '',
        dataPort: (map['dataPort'] as num?)?.toInt(),
        subject: map['subject'] as String? ?? '',
        attachments: atts,
      );
    });
  }

  /// Stream of incoming/outgoing call events reported by the native receiver.
  Stream<IncomingCall> incomingCalls() {
    return _calls.receiveBroadcastStream().map((event) {
      final map = Map<String, dynamic>.from(event as Map);
      final ts = map['timestamp'] as int?;
      final slot = (map['simSlot'] as num?)?.toInt();
      return IncomingCall(
        map['number'] as String? ?? '',
        map['direction'] as String? ?? 'incoming',
        map['status'] as String? ?? 'ringing',
        ts != null ? DateTime.fromMillisecondsSinceEpoch(ts) : DateTime.now(),
        simSlot: (slot != null && slot >= 0) ? slot : null,
        duration: (map['duration'] as num?)?.toInt(),
      );
    });
  }

  /// Stream of send/delivery status reports for outbound messages.
  Stream<SmsStatus> smsStatus() {
    return _status.receiveBroadcastStream().map((event) {
      final map = Map<String, dynamic>.from(event as Map);
      return SmsStatus(
        map['messageId'] as String?,
        map['kind'] as String? ?? '',
        map['success'] == true,
      );
    });
  }
}
