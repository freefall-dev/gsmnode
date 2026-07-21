import 'package:flutter/foundation.dart';

import '../main.dart';
import 'biometric_service.dart';

/// Owns the "App lock" preference. [AppLockGate] listens to it to know when to
/// cover the UI; Settings toggles it.
///
/// The Phone Agent's controller of the same name, differing in one place: it
/// arms on a *registered device*, this one on a *signed-in session*.
class AppLockController extends ChangeNotifier {
  bool get enabled => storage.appLockEnabled;

  /// Armed only once someone is signed in. There is nothing worth guarding on
  /// the login screen, and locking it would strand someone who signed out.
  bool get armed => enabled && storage.isAuthenticated;

  /// Turns App lock on or off, but only after a successful prompt.
  ///
  /// Requiring it in *both* directions matters: turning it on proves the user
  /// can actually clear the lock they are about to arm, and turning it off
  /// stops someone holding an already-unlocked phone from quietly disarming it.
  Future<AuthOutcome> setEnabled(bool value) async {
    final out = await biometrics.authenticate(
      value ? 'Confirm to turn on App lock' : 'Confirm to turn off App lock',
    );
    if (!out.passed) return out;
    storage.appLockEnabled = value;
    notifyListeners();
    return out;
  }

  /// Turns App lock off *without* a prompt, for the one case where no prompt is
  /// possible: the phone no longer has a biometric or a screen lock at all.
  ///
  /// Safe despite skipping the check — clearing a screen lock requires entering
  /// the old one, so anyone who can put the phone in this state had already
  /// passed it. Without this the gate would be a one-way door: unlocking is
  /// impossible, and so is disabling the lock.
  void disarm() {
    if (!storage.appLockEnabled) return;
    storage.appLockEnabled = false;
    notifyListeners();
  }
}
