/// One MMS attachment handed to the device (base64 data) or reported back.
class Attachment {
  final String filename;
  final String contentType;
  final String data; // base64-encoded bytes

  Attachment({required this.filename, required this.contentType, required this.data});

  factory Attachment.fromJson(Map<String, dynamic> j) => Attachment(
        filename: j['filename'] as String? ?? '',
        contentType: j['content_type'] as String? ?? 'application/octet-stream',
        data: j['data'] as String? ?? '',
      );

  Map<String, dynamic> toJson() => {
        'filename': filename,
        'content_type': contentType,
        'data': data,
      };
}

/// An outbound message handed to the device by the API Server.
class GatewayMessage {
  final String id;
  final String type; // 'sms' | 'call' | 'data' | 'mms'
  final List<String> phoneNumbers;
  final String textMessage;
  final int? simNumber;
  final String status;
  // Data SMS.
  final String dataPayload; // base64
  final int? dataPort;
  // MMS.
  final String subject;
  final List<Attachment> attachments;
  // When true, phoneNumbers + textMessage are E2E ciphertext.
  final bool encrypted;
  /// When set, the message must not be sent before this moment. The server
  /// withholds scheduled messages until they are due, so this is a backstop
  /// against an older server handing one over early.
  final DateTime? scheduleAt;

  GatewayMessage({
    required this.id,
    required this.type,
    required this.phoneNumbers,
    required this.textMessage,
    this.simNumber,
    required this.status,
    this.dataPayload = '',
    this.dataPort,
    this.subject = '',
    this.attachments = const [],
    this.encrypted = false,
    this.scheduleAt,
  });

  bool get isCall => type == 'call';
  bool get isData => type == 'data';
  bool get isMms => type == 'mms';

  /// Whether the message is ready to send now.
  bool get isDue => scheduleAt == null || !scheduleAt!.isAfter(DateTime.now());

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
      dataPayload: json['data_payload'] as String? ?? '',
      dataPort: json['data_port'] as int?,
      subject: json['subject'] as String? ?? '',
      attachments: (json['attachments'] as List<dynamic>? ?? [])
          .map((e) => Attachment.fromJson(e as Map<String, dynamic>))
          .toList(),
      encrypted: json['encrypted'] == true,
      scheduleAt: _parseDate(json['schedule_at']),
    );
  }

  /// Parses a server datetime. PocketBase emits `2006-01-02 15:04:05.000Z`,
  /// which `DateTime.parse` accepts once the space is replaced by `T`.
  static DateTime? _parseDate(dynamic v) {
    final s = v as String? ?? '';
    if (s.isEmpty) return null;
    return DateTime.tryParse(s.replaceFirst(' ', 'T'))?.toLocal();
  }
}
