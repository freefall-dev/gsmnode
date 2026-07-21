import 'package:flutter/material.dart';

import 'config.dart';
import 'screens/home_shell.dart';
import 'screens/login_screen.dart';
import 'services/api_client.dart';
import 'services/auth_store.dart';
import 'services/crypto_service.dart';
import 'services/storage.dart';
import 'services/theme_controller.dart';
import 'theme.dart';

/// Process-wide services, bootstrapped in [main] and reached directly by the
/// screens — the same arrangement the Phone Agent uses, kept deliberately so the
/// two Flutter surfaces read alike.
late Storage storage;
late ApiClient apiClient;
late AuthStore auth;
late CryptoService crypto;
late ThemeController themeController;

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();

  storage = await Storage.create();
  // Unlike the browser there is no same-origin BFF to fall back on, so the app
  // always has a server to point at, even before anyone opens Settings.
  storage.apiBase ??= AppConfig.defaultApiBase;

  apiClient = ApiClient(storage);
  auth = AuthStore(apiClient, storage);
  crypto = CryptoService(storage);
  themeController = ThemeController(storage);

  runApp(const GsmNodeConsoleApp());
}

class GsmNodeConsoleApp extends StatelessWidget {
  const GsmNodeConsoleApp({super.key});

  @override
  Widget build(BuildContext context) {
    // Rebuilds on a theme change and on sign-in/sign-out, so `home` alone
    // decides which half of the app is showing — no screen has to navigate the
    // session boundary itself.
    return AnimatedBuilder(
      animation: Listenable.merge([themeController, auth]),
      builder: (context, _) => MaterialApp(
        title: 'gsmnode',
        debugShowCheckedModeBanner: false,
        theme: gsmnodeLightTheme(),
        darkTheme: gsmnodeDarkTheme(),
        themeMode: themeController.mode,
        home: auth.isAuthenticated ? const HomeShell() : const LoginScreen(),
      ),
    );
  }
}
