// Unit tests for the pieces that carry real logic and don't need a live server:
// the E2E crypto (which must stay wire-compatible with the Web App and the
// Phone Agent) and the shared response helpers.

import 'package:console/config.dart';
import 'package:console/main.dart';
import 'package:console/services/api_client.dart';
import 'package:console/services/biometric_service.dart';
import 'package:console/services/crypto_service.dart';
import 'package:console/services/storage.dart';
import 'package:console/theme.dart';
import 'package:console/widgets/app_lock_gate.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:local_auth/local_auth.dart';
import 'package:shared_preferences/shared_preferences.dart';

Future<CryptoService> cryptoWith(String passphrase) async {
  SharedPreferences.setMockInitialValues({});
  final storage = await Storage.create();
  storage.encPassphrase = passphrase;
  return CryptoService(storage);
}

/// A biometric prompt that answers however the test says, without a device.
class FakeLocalAuth implements LocalAuthentication {
  FakeLocalAuth({this.passes = true, this.deviceSupported = true});

  bool passes;
  bool deviceSupported;
  int prompts = 0;

  // `authMessages` is widened to Iterable<dynamic> — a legal override, and it
  // keeps the test off a direct dependency on the platform interface package
  // just to name AuthMessages.
  @override
  Future<bool> authenticate({
    required String localizedReason,
    Iterable<dynamic> authMessages = const [],
    AuthenticationOptions options = const AuthenticationOptions(),
  }) async {
    prompts++;
    return passes;
  }

  @override
  Future<bool> isDeviceSupported() async => deviceSupported;

  @override
  Future<bool> get canCheckBiometrics async => deviceSupported;

  @override
  Future<List<BiometricType>> getAvailableBiometrics() async =>
      deviceSupported ? [BiometricType.fingerprint] : [];

  @override
  Future<bool> stopAuthentication() async => true;
}

/// Stands the globals up around a fake prompt, as [bootstrapServices] would.
Future<FakeLocalAuth> bootstrapWithLock({
  required bool enabled,
  bool signedIn = true,
  bool passes = true,
  bool deviceSupported = true,
}) async {
  SharedPreferences.setMockInitialValues({
    if (signedIn) 'jwt': 'token',
    'app_lock': enabled,
  });
  await bootstrapServices();
  final fake = FakeLocalAuth(passes: passes, deviceSupported: deviceSupported);
  biometrics = BiometricService(fake);
  return fake;
}

/// Pumps the gate over a stand-in for the app, on a clock the test drives — the
/// grace window is otherwise only observable by waiting half a minute.
Future<(AppLockGateState, void Function(Duration))> pumpGate(
  WidgetTester tester,
) async {
  var now = DateTime(2026);
  await tester.pumpWidget(
    MaterialApp(
      theme: gsmnodeLightTheme(),
      home: AppLockGate(
        clock: () => now,
        child: const Scaffold(body: Text('the console')),
      ),
    ),
  );
  await tester.pumpAndSettle();
  final state = tester.state<AppLockGateState>(find.byType(AppLockGate));
  return (state, (Duration d) => now = now.add(d));
}

/// Drives a trip out of the app and back, as Android's lifecycle would.
Future<void> leaveAndReturn(
  WidgetTester tester, {
  required AppLifecycleState via,
}) async {
  tester.binding.handleAppLifecycleStateChanged(via);
  await tester.pump();
  tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
  await tester.pumpAndSettle();
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  group('CryptoService', () {
    test('round-trips text through the gsmenc:v1 envelope', () async {
      final crypto = await cryptoWith('correct horse battery staple');
      final sealed = await crypto.encrypt('hello gateway');

      expect(CryptoService.isEncrypted(sealed), isTrue);
      expect(sealed, startsWith('gsmenc:v1:'));
      expect(sealed, isNot(contains('hello gateway')));
      expect(await crypto.decrypt(sealed), 'hello gateway');
    });

    test('passes values through when no passphrase is set', () async {
      final crypto = await cryptoWith('');
      expect(crypto.enabled, isFalse);
      expect(await crypto.encrypt('plain'), 'plain');
      expect(await crypto.decrypt('plain'), 'plain');
    });

    test('salts every message, so the same text seals differently', () async {
      final crypto = await cryptoWith('shared');
      final a = await crypto.encrypt('same text');
      final b = await crypto.encrypt('same text');
      expect(a, isNot(b));
      expect(await crypto.decrypt(a), await crypto.decrypt(b));
    });

    test('tryDecrypt marks what the wrong passphrase cannot open', () async {
      final sealed = await (await cryptoWith('the right one')).encrypt('secret');
      final wrong = await cryptoWith('the wrong one');

      expect(await wrong.tryDecrypt(sealed), CryptoService.unreadable);
      // Plaintext still passes straight through.
      expect(await wrong.tryDecrypt('not encrypted'), 'not encrypted');
    });
  });

  group('itemsOf', () {
    test('reads the list under the requested key', () {
      final items = itemsOf({
        'items': [
          {'id': 'a'},
          {'id': 'b'},
        ]
      });
      expect(items.map((e) => e['id']), ['a', 'b']);
      expect(
        itemsOf({
          'organizations': [
            {'id': 'org1'}
          ]
        }, 'organizations').single['id'],
        'org1',
      );
    });

    test('treats a missing or malformed body as empty', () {
      // The API Server omits the key entirely when there is nothing to return.
      expect(itemsOf(const {}), isEmpty);
      expect(itemsOf(null), isEmpty);
      expect(itemsOf('unexpected'), isEmpty);
      expect(itemsOf(const {'items': 'not a list'}), isEmpty);
    });
  });

  group('AppLockController', () {
    test('arms only over a signed-in session', () async {
      await bootstrapWithLock(enabled: true);
      expect(appLock.armed, isTrue);

      await bootstrapWithLock(enabled: false);
      expect(appLock.armed, isFalse);

      // Nothing worth guarding on the login screen.
      await bootstrapWithLock(enabled: true, signedIn: false);
      expect(appLock.enabled, isTrue);
      expect(appLock.armed, isFalse);
    });

    test('the prompt gates the switch in both directions', () async {
      final fake = await bootstrapWithLock(enabled: false);

      fake.passes = false;
      expect((await appLock.setEnabled(true)).passed, isFalse);
      expect(appLock.enabled, isFalse, reason: 'a failed prompt must not arm');

      fake.passes = true;
      expect((await appLock.setEnabled(true)).passed, isTrue);
      expect(appLock.enabled, isTrue);

      // And disarming is not a free action for whoever is holding the phone.
      fake.passes = false;
      expect((await appLock.setEnabled(false)).passed, isFalse);
      expect(appLock.enabled, isTrue);
      expect(fake.prompts, 3);
    });
  });

  group('AppLockGate', () {
    testWidgets('an armed lock covers the app on a cold start', (tester) async {
      await bootstrapWithLock(enabled: true, passes: false);
      final (gate, _) = await pumpGate(tester);

      expect(gate.locked, isTrue);
      expect(find.text('LOCKED'), findsOneWidget);
      expect(find.text('Unlock'), findsOneWidget);
    });

    testWidgets('a passing prompt clears it', (tester) async {
      await bootstrapWithLock(enabled: true);
      final (gate, _) = await pumpGate(tester);

      expect(gate.locked, isFalse, reason: 'the prompt fires on first frame');
      expect(find.text('the console'), findsOneWidget);
    });

    testWidgets('a disarmed lock never covers anything', (tester) async {
      await bootstrapWithLock(enabled: false);
      final (gate, advance) = await pumpGate(tester);

      advance(const Duration(days: 1));
      await leaveAndReturn(tester, via: AppLifecycleState.paused);
      expect(gate.locked, isFalse);
    });

    testWidgets('a short excursion stays inside the grace window',
        (tester) async {
      // Arm it, then fail every prompt from here on, so a re-lock is visible
      // rather than immediately cleared.
      final fake = await bootstrapWithLock(enabled: true);
      final (gate, advance) = await pumpGate(tester);
      fake.passes = false;

      tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.paused);
      await tester.pump();
      advance(AppConfig.appLockGrace - const Duration(seconds: 1));
      tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
      await tester.pumpAndSettle();
      expect(gate.locked, isFalse, reason: 'the photo picker must not re-lock');

      tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.paused);
      await tester.pump();
      advance(AppConfig.appLockGrace);
      tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
      await tester.pumpAndSettle();
      expect(gate.locked, isTrue);
    });

    testWidgets('closing the app re-locks regardless of the grace',
        (tester) async {
      final fake = await bootstrapWithLock(enabled: true);
      final (gate, _) = await pumpGate(tester);
      fake.passes = false;

      // No time passes at all — a detach is a deliberate exit, not an
      // interruption, so the grace period does not apply to it.
      await leaveAndReturn(tester, via: AppLifecycleState.detached);
      expect(gate.locked, isTrue);
    });

    testWidgets('a phone that can no longer prompt is let back in',
        (tester) async {
      // Biometrics gone *and* the screen lock removed: unlocking is impossible
      // and so is disarming, so the gate would be a one-way door.
      await bootstrapWithLock(enabled: true, passes: false, deviceSupported: false);
      final (gate, _) = await pumpGate(tester);

      expect(gate.locked, isFalse);
      expect(appLock.enabled, isFalse, reason: 'it disarms itself');
    });
  });

  group('ApiException', () {
    test('flags a transport failure as unreachable', () {
      expect(ApiException(0, 'no route').unreachable, isTrue);
      expect(ApiException(401, 'unauthorized').unreachable, isFalse);
    });
  });
}
