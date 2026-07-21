/// Compile-time defaults. The API base URL is also user-editable on the login
/// screen and persisted via [Storage].
class AppConfig {
  /// Default API Server base URL. Use the LAN IP of the machine running the
  /// API Server (not localhost — that points at the phone itself).
  ///
  /// 10.0.2.2 is the Android emulator's alias for the host machine.
  static const String defaultApiBase = 'http://10.0.2.2:8080';

  /// How often the header's reachability dot re-probes `/api/health`, matching
  /// the Web App's ApiStatus poller.
  static const Duration healthInterval = Duration(seconds: 10);

  /// How often the Devices list refreshes itself. Online/offline is derived
  /// from a heartbeat, so it changes with nobody touching the screen.
  static const Duration devicePollInterval = Duration(seconds: 10);

  /// How long the app may sit in the background before the biometric lock
  /// closes again. Short enough that a pocketed phone is protected, long enough
  /// that the excursions the app itself causes — the photo picker on an MMS, a
  /// glance at another app — don't demand a fingerprint on the way back.
  static const Duration appLockGrace = Duration(seconds: 30);
}
