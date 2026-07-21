import 'package:flutter/material.dart';

import '../main.dart';
import '../theme.dart';
import 'gsmnode_mark.dart';

/// Covers the UI with an unlock screen while App lock is on.
///
/// Only the *UI* is gated. The gateway loop and its foreground service keep
/// routing SMS, MMS and calls behind the lock — which is the whole point on a
/// spare handset: it stays useful sitting on a desk, but nobody who picks it up
/// can stop the gateway, read the activity log, or sign out.
///
/// Mounted from `MaterialApp.builder` so it sits above the Navigator and covers
/// every route, including screens pushed after a later sign-in.
class AppLockGate extends StatefulWidget {
  const AppLockGate({super.key, required this.child});

  final Widget child;

  @override
  State<AppLockGate> createState() => _AppLockGateState();
}

class _AppLockGateState extends State<AppLockGate> with WidgetsBindingObserver {
  /// How long the app may sit in the background before it re-locks. Brief hops
  /// out — a notification, the SIM chooser, a permission dialog — shouldn't
  /// cost a prompt.
  static const _grace = Duration(seconds: 30);

  bool _locked = false;
  bool _prompting = false;
  String? _error;
  DateTime? _leftAt;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    appLock.addListener(_onLockPrefChanged);
    if (appLock.armed) {
      _locked = true;
      WidgetsBinding.instance.addPostFrameCallback((_) => _unlock());
    }
  }

  @override
  void dispose() {
    appLock.removeListener(_onLockPrefChanged);
    WidgetsBinding.instance.removeObserver(this);
    super.dispose();
  }

  /// Switching App lock off from the Home screen clears an armed gate.
  void _onLockPrefChanged() {
    if (!appLock.armed && _locked) setState(() => _locked = false);
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    // The biometric sheet pauses the app itself. Ignoring lifecycle changes
    // while a prompt is up stops it from re-arming the lock behind its own
    // dialog — which would loop forever.
    if (_prompting) return;

    if (state == AppLifecycleState.paused ||
        state == AppLifecycleState.hidden) {
      _leftAt = DateTime.now();
    } else if (state == AppLifecycleState.resumed) {
      final away = DateTime.now().difference(_leftAt ?? DateTime.now());
      if (appLock.armed && !_locked && away >= _grace) {
        setState(() => _locked = true);
        _unlock();
      }
    }
  }

  Future<void> _unlock() async {
    if (_prompting) return;
    setState(() {
      _prompting = true;
      _error = null;
    });
    final out = await biometrics.authenticate('Unlock gsmnode agent');

    // A phone that can no longer prompt at all — biometrics gone *and* the
    // screen lock removed — would otherwise strand its owner here forever,
    // since disabling App lock needs the same prompt. Let them back in and
    // disarm rather than leaving a reinstall (which un-registers the phone) as
    // the only way out.
    final stranded = !out.passed && !await biometrics.supported;
    if (stranded) appLock.disarm();

    if (!mounted) return;
    setState(() {
      _prompting = false;
      _locked = !out.passed && !stranded;
      _error = out.passed || stranded ? null : out.message;
      if (out.passed || stranded) _leftAt = null;
    });
  }

  @override
  Widget build(BuildContext context) {
    return Stack(
      children: [
        widget.child,
        if (_locked)
          Positioned.fill(
            child: _LockOverlay(
              busy: _prompting,
              error: _error,
              onUnlock: _unlock,
            ),
          ),
      ],
    );
  }
}

class _LockOverlay extends StatelessWidget {
  const _LockOverlay({
    required this.busy,
    required this.error,
    required this.onUnlock,
  });

  final bool busy;
  final String? error;
  final VoidCallback onUnlock;

  @override
  Widget build(BuildContext context) {
    final cg = context.cg;
    return Material(
      color: cg.pageBg,
      child: SafeArea(
        child: Center(
          child: SingleChildScrollView(
            padding: const EdgeInsets.all(24),
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 420),
              child: Column(
                mainAxisSize: MainAxisSize.min,
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  const Center(child: GsmNodeMark(size: 56)),
                  const SizedBox(height: 14),
                  const Center(child: GsmNodeWordmark(size: 26)),
                  const SizedBox(height: 8),
                  Center(
                    child: Text(
                      'LOCKED',
                      style: gsmMono(
                        size: 10,
                        color: cg.textMuted,
                        letterSpacing: 1.4,
                      ),
                    ),
                  ),
                  const SizedBox(height: 28),
                  Icon(Icons.fingerprint, size: 56, color: cg.textMuted),
                  const SizedBox(height: 12),
                  Text(
                    'The gateway keeps routing while locked.',
                    textAlign: TextAlign.center,
                    style: TextStyle(color: cg.textSecondary),
                  ),
                  if (error != null) ...[
                    const SizedBox(height: 16),
                    Container(
                      padding: const EdgeInsets.all(12),
                      decoration: BoxDecoration(
                        color: cg.dangerTint,
                        borderRadius: BorderRadius.circular(10),
                      ),
                      child: Text(
                        error!,
                        style: TextStyle(color: cg.danger),
                      ),
                    ),
                  ],
                  const SizedBox(height: 24),
                  FilledButton.icon(
                    onPressed: busy ? null : onUnlock,
                    icon: const Icon(Icons.lock_open),
                    label: Text(busy ? 'Waiting…' : 'Unlock'),
                    style: FilledButton.styleFrom(
                      minimumSize: const Size.fromHeight(48),
                    ),
                  ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}
