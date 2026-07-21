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
}
