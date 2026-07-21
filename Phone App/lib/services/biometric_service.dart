import 'package:flutter/services.dart';
import 'package:local_auth/error_codes.dart' as auth_error;
import 'package:local_auth/local_auth.dart';

/// The result of an unlock attempt. The UI only needs to know whether it passed
/// and, when it didn't, one line explaining why.
class AuthOutcome {
  const AuthOutcome.ok()
      : passed = true,
        message = null;
  const AuthOutcome.failed([this.message]) : passed = false;

  final bool passed;
  final String? message;
}

/// Face / fingerprint (and device PIN) prompts, wrapped so callers deal in
/// [AuthOutcome] instead of platform exceptions.
///
/// Deliberately the Phone Agent's `biometric_service.dart`, near enough line for
/// line: the two Flutter surfaces lock the same way for the same reasons, and a
/// second dialect of this would only drift.
///
/// The device credential is allowed as a fallback: a sensor that fails, or an
/// enrolment that gets cleared, must never cost someone the console. Here that
/// is milder than on the Agent — a signed-out console is one password away,
/// where a reinstalled Agent has to re-register the phone — but the prompt
/// should behave the same on both.
class BiometricService {
  BiometricService([LocalAuthentication? auth])
      : _auth = auth ?? LocalAuthentication();

  final LocalAuthentication _auth;

  /// Whether this phone can prompt at all — an enrolled biometric *or* a
  /// screen lock. False on a phone with neither, where App lock is pointless.
  Future<bool> get supported async {
    try {
      return await _auth.isDeviceSupported();
    } on PlatformException {
      return false;
    }
  }

  /// What the phone will actually ask for, for labelling the UI.
  Future<String> methodLabel() async {
    try {
      final types = await _auth.getAvailableBiometrics();
      final face = types.contains(BiometricType.face);
      final finger = types.contains(BiometricType.fingerprint);
      if (face && finger) return 'Face or fingerprint';
      if (face) return 'Face';
      if (finger) return 'Fingerprint';
      if (types.isNotEmpty) return 'Biometrics';
      return 'Device PIN'; // nothing enrolled — the credential fallback carries it
    } on PlatformException {
      return 'Device PIN';
    }
  }

  Future<AuthOutcome> authenticate(String reason) async {
    try {
      final ok = await _auth.authenticate(
        localizedReason: reason,
        options: const AuthenticationOptions(
          // The system sheet backgrounds the app; sticky auth keeps the attempt
          // alive across that round trip instead of cancelling it.
          stickyAuth: true,
          biometricOnly: false,
          useErrorDialogs: true,
        ),
      );
      return ok ? const AuthOutcome.ok() : const AuthOutcome.failed();
    } on PlatformException catch (e) {
      return AuthOutcome.failed(_explain(e));
    }
  }

  String _explain(PlatformException e) {
    switch (e.code) {
      case auth_error.notAvailable:
        return 'This phone has no usable biometric hardware.';
      case auth_error.notEnrolled:
        return 'No face or fingerprint is enrolled on this phone.';
      case auth_error.passcodeNotSet:
        return 'Set a screen lock on this phone first.';
      case auth_error.lockedOut:
        return 'Too many attempts. Wait a moment, then try again.';
      case auth_error.permanentlyLockedOut:
        return 'Locked out. Unlock the phone with its PIN or pattern first.';
      default:
        return e.message ?? 'Authentication failed.';
    }
  }
}
