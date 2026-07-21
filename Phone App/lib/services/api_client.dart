import 'dart:convert';

import 'package:http/http.dart' as http;

import 'storage.dart';

/// A non-2xx response, or a transport failure. [status] is `0` when the request
/// never reached a server — the Web App distinguishes the same case (its
/// `e.status === undefined`) to say "check the server URL" instead of echoing a
/// socket error at the user.
class ApiException implements Exception {
  final int status;
  final String message;
  ApiException(this.status, this.message);

  bool get unreachable => status == 0;

  @override
  String toString() => 'ApiException($status): $message';
}

/// Thin wrapper around the API Server's `/api` surface, mirroring the Web App's
/// `api.js`: bearer-token auth, JSON in and out, `{error}` bodies unwrapped into
/// [ApiException].
///
/// The Web App can fall back on its own origin (the BFF proxy); the phone always
/// calls a configured API Server directly and relies on the server's CORS-free
/// native HTTP path.
class ApiClient {
  final Storage storage;
  final http.Client _http;

  ApiClient(this.storage, {http.Client? client})
      : _http = client ?? http.Client();

  String get _base => (storage.apiBase ?? '').replaceAll(RegExp(r'/+$'), '');

  Map<String, String> _headers({bool json = false}) {
    final token = storage.jwt ?? '';
    return {
      if (json) 'Content-Type': 'application/json',
      if (token.isNotEmpty) 'Authorization': 'Bearer $token',
    };
  }

  Future<dynamic> get(String path) => _send('GET', path);
  Future<dynamic> post(String path, [Object? body]) => _send('POST', path, body);
  Future<dynamic> put(String path, [Object? body]) => _send('PUT', path, body);
  Future<dynamic> patch(String path, [Object? body]) =>
      _send('PATCH', path, body);
  Future<dynamic> delete(String path) => _send('DELETE', path);

  Future<dynamic> _send(String method, String path, [Object? body]) async {
    final uri = Uri.parse('$_base/api$path');
    final request = http.Request(method, uri)
      ..headers.addAll(_headers(json: body != null));
    if (body != null) request.body = jsonEncode(body);

    final http.Response res;
    try {
      res = await http.Response.fromStream(await _http.send(request));
    } catch (e) {
      throw ApiException(0, 'Cannot reach the API Server at $_base');
    }
    return _decode(res);
  }

  dynamic _decode(http.Response res) {
    dynamic data;
    if (res.body.isNotEmpty) {
      try {
        data = jsonDecode(res.body);
      } catch (_) {
        data = res.body; // a proxy or a crash can hand back plain text
      }
    }
    if (res.statusCode < 200 || res.statusCode >= 300) {
      final msg = (data is Map && data['error'] != null)
          ? data['error'].toString()
          : (res.reasonPhrase?.isNotEmpty ?? false)
              ? res.reasonPhrase!
              : 'HTTP ${res.statusCode}';
      throw ApiException(res.statusCode, msg);
    }
    return data;
  }
}

/// `{items: [...]}` list responses, normalised to a typed list of maps. The API
/// Server omits the key entirely when there is nothing to return.
List<Map<String, dynamic>> itemsOf(dynamic body, [String key = 'items']) {
  if (body is! Map) return const [];
  final raw = body[key];
  if (raw is! List) return const [];
  return raw.whereType<Map>().map((e) => e.cast<String, dynamic>()).toList();
}
