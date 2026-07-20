import 'package:shared_preferences/shared_preferences.dart';

/// Persists connection settings and credentials across app launches.
class Storage {
  static const _kApiBase = 'api_base';
  static const _kJwt = 'jwt';
  static const _kDeviceToken = 'device_token';
  static const _kDeviceId = 'device_id';
  static const _kDeviceName = 'device_name';
  static const _kUserEmail = 'user_email';
  static const _kEncPassphrase = 'enc_passphrase';

  final SharedPreferences _prefs;
  Storage(this._prefs);

  static Future<Storage> create() async {
    return Storage(await SharedPreferences.getInstance());
  }

  String? get apiBase => _prefs.getString(_kApiBase);
  set apiBase(String? v) => _set(_kApiBase, v);

  String? get jwt => _prefs.getString(_kJwt);
  set jwt(String? v) => _set(_kJwt, v);

  String? get deviceToken => _prefs.getString(_kDeviceToken);
  set deviceToken(String? v) => _set(_kDeviceToken, v);

  String? get deviceId => _prefs.getString(_kDeviceId);
  set deviceId(String? v) => _set(_kDeviceId, v);

  String? get deviceName => _prefs.getString(_kDeviceName);
  set deviceName(String? v) => _set(_kDeviceName, v);

  String? get userEmail => _prefs.getString(_kUserEmail);
  set userEmail(String? v) => _set(_kUserEmail, v);

  /// Shared E2E passphrase. Kept only on-device; must match the one entered in
  /// the Web App (and any other device) that reads these messages.
  String get encPassphrase => _prefs.getString(_kEncPassphrase) ?? '';
  set encPassphrase(String v) => _set(_kEncPassphrase, v.isEmpty ? null : v);

  bool get isRegistered => (deviceToken ?? '').isNotEmpty;

  Future<void> clearSession() async {
    await _prefs.remove(_kJwt);
    await _prefs.remove(_kDeviceToken);
    await _prefs.remove(_kUserEmail);
    // Keep device_id stable across logout so re-registering updates the same
    // device record instead of creating a new (phantom) one.
  }

  void _set(String key, String? v) {
    if (v == null) {
      _prefs.remove(key);
    } else {
      _prefs.setString(key, v);
    }
  }
}
