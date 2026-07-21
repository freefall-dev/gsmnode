/// Compile-time defaults. The API base URL is also user-editable on the login
/// screen and persisted via [Storage].
class AppConfig {
  /// Default API Server base URL. Use the LAN IP of the machine running the
  /// API Server (not localhost — that points at the phone itself).
  ///
  /// 10.0.2.2 is the Android emulator's alias for the host machine.
  static const String defaultApiBase = 'http://10.0.2.2:8080';

  /// How often the gateway polls the API Server for pending messages.
  static const Duration pollInterval = Duration(seconds: 10);

  /// How often the device sends a heartbeat ping.
  static const Duration pingInterval = Duration(minutes: 1);
}
