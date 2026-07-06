/// An outbound message handed to the device by the API Server.
class GatewayMessage {
  final String id;
  final String type; // 'sms' or 'call'
  final List<String> phoneNumbers;
  final String textMessage;
  final int? simNumber;
  final String status;

  GatewayMessage({
    required this.id,
    required this.type,
    required this.phoneNumbers,
    required this.textMessage,
    this.simNumber,
    required this.status,
  });

  bool get isCall => type == 'call';

  factory GatewayMessage.fromJson(Map<String, dynamic> json) {
    return GatewayMessage(
      id: json['id'] as String? ?? '',
      type: json['type'] as String? ?? 'sms',
      phoneNumbers: (json['phone_numbers'] as List<dynamic>? ?? [])
          .map((e) => e.toString())
          .toList(),
      textMessage: json['text_message'] as String? ?? '',
      simNumber: json['sim_number'] as int?,
      status: json['status'] as String? ?? '',
    );
  }
}
