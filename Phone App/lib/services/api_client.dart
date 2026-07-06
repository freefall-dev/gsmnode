import 'dart:convert';
import 'package:http/http.dart' as http;

import '../models/message.dart';
import 'storage.dart';

class ApiException implements Exception {
  final int status;
  final String message;
  ApiException(this.status, this.message);
  @override
  String toString() => 'ApiException($status): $message';
}

/// Client for the API Server. Uses the user JWT for login/registration and the
/// opaque device token for the mobile gateway endpoints.
class ApiClient {
  final Storage storage;
  final http.Client _http;

  ApiClient(this.storage, {http.Client? client})
      : _http = client ?? http.Client();

  String get _base => (storage.apiBase ?? '').replaceAll(RegExp(r'/$'), '');

  Uri _uri(String path) => Uri.parse('$_base$path');

  Map<String, String> _headers(String? token) => {
        'Content-Type': 'application/json',
        if (token != null && token.isNotEmpty) 'Authorization': 'Bearer $token',
      };

  dynamic _decode(http.Response res) {
    final body = res.body.isNotEmpty ? jsonDecode(res.body) : null;
    if (res.statusCode < 200 || res.statusCode >= 300) {
      final msg = (body is Map && body['error'] != null)
          ? body['error'].toString()
          : 'HTTP ${res.statusCode}';
      throw ApiException(res.statusCode, msg);
    }
    return body;
  }

  /// Authenticates a user and stores the JWT. Returns the user email.
  Future<String> login(String email, String password) async {
    final res = await _http.post(
      _uri('/api/auth/login'),
      headers: _headers(null),
      body: jsonEncode({'email': email, 'password': password}),
    );
    final data = _decode(res) as Map<String, dynamic>;
    storage.jwt = data['access_token'] as String?;
    final user = data['user'] as Map<String, dynamic>?;
    storage.userEmail = user?['email'] as String?;
    return storage.userEmail ?? email;
  }

  /// Registers this device and stores the returned device token.
  Future<void> registerDevice({
    required String deviceId,
    required String name,
    String platform = 'android',
    String appVersion = '1.0.0',
    String? pushToken,
  }) async {
    final res = await _http.post(
      _uri('/api/mobile/v1/device'),
      headers: _headers(storage.jwt),
      body: jsonEncode({
        'device_id': deviceId,
        'name': name,
        'platform': platform,
        'app_version': appVersion,
        if (pushToken != null) 'push_token': pushToken,
      }),
    );
    final data = _decode(res) as Map<String, dynamic>;
    storage.deviceId = deviceId;
    storage.deviceName = name;
    storage.deviceToken = data['auth_token'] as String?;
  }

  /// Pulls pending messages for this device (the server marks them Processed).
  Future<List<GatewayMessage>> pullMessages() async {
    final res = await _http.get(
      _uri('/api/mobile/v1/messages'),
      headers: _headers(storage.deviceToken),
    );
    final data = _decode(res) as Map<String, dynamic>;
    final items = (data['items'] as List<dynamic>? ?? []);
    return items
        .map((e) => GatewayMessage.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  /// Reports a message's delivery state back to the server.
  Future<void> reportMessage(String id, String status, {String? error}) async {
    final res = await _http.patch(
      _uri('/api/mobile/v1/messages/$id'),
      headers: _headers(storage.deviceToken),
      body: jsonEncode({'status': status, if (error != null) 'error': error}),
    );
    _decode(res);
  }

  /// Posts an incoming SMS to the server's inbox.
  Future<void> postInbox(String phoneNumber, String message,
      {DateTime? receivedAt}) async {
    final res = await _http.post(
      _uri('/api/mobile/v1/inbox'),
      headers: _headers(storage.deviceToken),
      body: jsonEncode({
        'phone_number': phoneNumber,
        'message': message,
        if (receivedAt != null)
          'received_at': receivedAt.toUtc().toIso8601String(),
      }),
    );
    _decode(res);
  }

  /// Sends a heartbeat ping.
  Future<void> ping() async {
    final res = await _http.post(
      _uri('/api/mobile/v1/ping'),
      headers: _headers(storage.deviceToken),
    );
    _decode(res);
  }
}
