import 'package:shared_preferences/shared_preferences.dart';

/// Persists connection settings, the session and local preferences across app
/// launches — the phone's answer to the Web App's `localStorage`.
class Storage {
  static const _kApiBase = 'api_base';
  static const _kJwt = 'jwt';
  static const _kUser = 'user';
  static const _kEncPassphrase = 'enc_passphrase';
  static const _kThemePref = 'gsmnode-theme';
  static const _kAppLock = 'app_lock';

  final SharedPreferences _prefs;
  Storage(this._prefs);

  static Future<Storage> create() async {
    return Storage(await SharedPreferences.getInstance());
  }

  /// API Server base URL. Unlike the Web App there is no same-origin BFF to fall
  /// back on, so this is always set — a blank one would leave the app with
  /// nowhere to call.
  String? get apiBase => _prefs.getString(_kApiBase);
  set apiBase(String? v) => _set(_kApiBase, v);

  String? get jwt => _prefs.getString(_kJwt);
  set jwt(String? v) => _set(_kJwt, v);

  /// The signed-in user, as the raw JSON string the API Server returned.
  String? get userJson => _prefs.getString(_kUser);
  set userJson(String? v) => _set(_kUser, v);

  /// Shared E2E passphrase. Kept only on-device; must match the one entered in
  /// the Web App (and the Phone Agent) that reads these messages.
  String get encPassphrase => _prefs.getString(_kEncPassphrase) ?? '';
  set encPassphrase(String v) => _set(_kEncPassphrase, v.isEmpty ? null : v);

  /// "light" | "dark" | "system" — same key and vocabulary as the Web App's
  /// theme toggle, so the two surfaces stay conceptually identical.
  String get themePref => _prefs.getString(_kThemePref) ?? 'system';
  set themePref(String v) => _set(_kThemePref, v);

  /// Whether the console asks for a face or fingerprint before it will show a
  /// signed-in session. Off by default — the phone's own lock screen is already
  /// between a stranger and this app. Same key and name as the Phone Agent's.
  bool get appLockEnabled => _prefs.getBool(_kAppLock) ?? false;
  set appLockEnabled(bool v) => _prefs.setBool(_kAppLock, v);

  bool get isAuthenticated => (jwt ?? '').isNotEmpty;

  Future<void> clearSession() async {
    await _prefs.remove(_kJwt);
    await _prefs.remove(_kUser);
    // The API base, passphrase and app lock survive a sign-out: all three are
    // device setup, not session state, and re-typing them on every login would
    // be tedious. Leaving the lock armed is also the safer of the two defaults.
  }

  void _set(String key, String? v) {
    if (v == null) {
      _prefs.remove(key);
    } else {
      _prefs.setString(key, v);
    }
  }
}
