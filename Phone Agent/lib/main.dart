import 'package:flutter/material.dart';

import 'config.dart';
import 'services/api_client.dart';
import 'services/app_lock.dart';
import 'services/biometric_service.dart';
import 'services/gateway_service.dart';
import 'services/sms_service.dart';
import 'services/storage.dart';
import 'screens/login_screen.dart';
import 'screens/home_screen.dart';
import 'theme.dart';
import 'widgets/app_lock_gate.dart';

late Storage storage;
late ApiClient apiClient;
late GatewayService gateway;
late BiometricService biometrics;
late AppLockController appLock;

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();

  storage = await Storage.create();
  storage.apiBase ??= AppConfig.defaultApiBase;

  apiClient = ApiClient(storage);
  gateway = GatewayService(apiClient, SmsService());
  biometrics = BiometricService();
  appLock = AppLockController();

  runApp(const GsmNodeApp());
}

class GsmNodeApp extends StatelessWidget {
  const GsmNodeApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'gsmnode agent',
      debugShowCheckedModeBanner: false,
      theme: gsmnodeLightTheme(),
      darkTheme: gsmnodeDarkTheme(),
      themeMode: ThemeMode.system,
      // Above the Navigator, so the lock covers every route rather than just
      // the one the app happened to start on.
      builder: (context, child) =>
          AppLockGate(child: child ?? const SizedBox.shrink()),
      home: storage.isRegistered ? const HomeScreen() : const LoginScreen(),
    );
  }
}
