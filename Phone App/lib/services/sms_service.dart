import 'package:flutter/services.dart';

/// An incoming SMS delivered from the native side.
class IncomingSms {
  final String from;
  final String body;
  final DateTime timestamp;

  /// 0-based physical SIM slot the message arrived on, or null if the device
  /// couldn't attribute it (single-SIM, or READ_PHONE_STATE not granted).
  final int? simSlot;

  IncomingSms(this.from, this.body, this.timestamp, {this.simSlot});
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

/// Bridges to the native Android side: SmsManager (sending), a BroadcastReceiver
/// (incoming), send/delivery PendingIntents (status), and the foreground service
/// that keeps the gateway alive. See android_overlay/ for the Kotlin side.
class SmsService {
  static const _method = MethodChannel('app.gsmnode/sms');
  static const _events = EventChannel('app.gsmnode/sms_incoming');
  static const _status = EventChannel('app.gsmnode/sms_status');

  /// Sends an SMS via the device radio. [simSlot] is 0-based; null = default SIM.
  /// [messageId] is echoed back on the status stream for correlation.
  /// Throws [PlatformException] on failure.
  Future<void> sendSms(String phoneNumber, String message,
      {int? simSlot, String? messageId}) async {
    await _method.invokeMethod('sendSms', {
      'phone': phoneNumber,
      'message': message,
      'simSlot': simSlot,
      'messageId': messageId,
    });
  }

  /// Enumerates the active SIMs on the device (empty if READ_PHONE_STATE isn't
  /// granted or the device has no telephony).
  Future<List<SimInfo>> getSims() async {
    final res = await _method.invokeMethod('getSims');
    final list = (res as List?) ?? const [];
    return list
        .map((e) => SimInfo.fromMap(e as Map))
        .toList(growable: false);
  }

  /// Places a phone call to [phoneNumber] via the native dialer (ACTION_CALL).
  /// Requires the CALL_PHONE permission. Throws [PlatformException] on failure.
  Future<void> placeCall(String phoneNumber) async {
    await _method.invokeMethod('placeCall', {'phone': phoneNumber});
  }

  /// Starts the foreground service so the gateway loop survives screen-off.
  Future<void> startBackgroundService() => _method.invokeMethod('startService');

  /// Stops the foreground service.
  Future<void> stopBackgroundService() => _method.invokeMethod('stopService');

  /// Stream of incoming SMS messages (active while the app process is alive).
  Stream<IncomingSms> incomingSms() {
    return _events.receiveBroadcastStream().map((event) {
      final map = Map<String, dynamic>.from(event as Map);
      final ts = map['timestamp'] as int?;
      final slot = (map['simSlot'] as num?)?.toInt();
      return IncomingSms(
        map['from'] as String? ?? '',
        map['body'] as String? ?? '',
        ts != null ? DateTime.fromMillisecondsSinceEpoch(ts) : DateTime.now(),
        simSlot: (slot != null && slot >= 0) ? slot : null,
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
