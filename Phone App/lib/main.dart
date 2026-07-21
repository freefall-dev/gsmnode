import 'package:flutter/material.dart';

import 'config.dart';
import 'screens/home_shell.dart';
import 'screens/login_screen.dart';
import 'services/api_client.dart';
import 'services/app_lock.dart';
import 'services/auth_store.dart';
import 'services/biometric_service.dart';
import 'services/crypto_service.dart';
import 'services/storage.dart';
import 'services/theme_controller.dart';
import 'theme.dart';
import 'widgets/app_lock_gate.dart';

/// Process-wide services, bootstrapped in [main] and reached directly by the
/// screens — the same arrangement the Phone Agent uses, kept deliberately so the
/// two Flutter surfaces read alike.
late Storage storage;
late ApiClient apiClient;
late AuthStore auth;
late CryptoService crypto;
late ThemeController themeController;
late BiometricService biometrics;
late AppLockController appLock;

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  await bootstrapServices();
  runApp(const GsmNodeConsoleApp());
}

/// Builds the service graph. Split out of [main] so tests can stand the app up
/// against mocked preferences without running `runApp`.
Future<void> bootstrapServices() async {
  storage = await Storage.create();
  // Unlike the browser there is no same-origin BFF to fall back on, so the app
  // always has a server to point at, even before anyone opens Settings.
  storage.apiBase ??= AppConfig.defaultApiBase;

  apiClient = ApiClient(storage);
  auth = AuthStore(apiClient, storage);
  crypto = CryptoService(storage);
  themeController = ThemeController(storage);
  biometrics = BiometricService();
  appLock = AppLockController();
}

class GsmNodeConsoleApp extends StatelessWidget {
  const GsmNodeConsoleApp({super.key});

  @override
  Widget build(BuildContext context) {
    // Rebuilds on a theme change and on sign-in/sign-out, so `home` alone
    // decides which half of the app is showing — no screen has to navigate the
    // session boundary itself. The lock is not in that list: [AppLockGate]
    // holds its own state and covers whatever is underneath.
    return AnimatedBuilder(
      animation: Listenable.merge([themeController, auth]),
      builder: (context, _) => MaterialApp(
        title: 'gsmnode',
        debugShowCheckedModeBanner: false,
        theme: gsmnodeLightTheme(),
        darkTheme: gsmnodeDarkTheme(),
        themeMode: themeController.mode,
        home: auth.isAuthenticated ? const HomeShell() : const LoginScreen(),
        // Above the navigator rather than inside it, so the lock covers every
        // route and dialog — the Phone Agent mounts its gate the same way.
        builder: (context, child) =>
            AppLockGate(child: child ?? const SizedBox.shrink()),
      ),
    );
  }
}
