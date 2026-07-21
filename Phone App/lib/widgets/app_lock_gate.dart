import 'package:flutter/material.dart';

import '../config.dart';
import '../main.dart';
import '../theme.dart';
import 'gsmnode_mark.dart';
import 'ui.dart';

/// Covers the UI with an unlock screen while App lock is on.
///
/// The Phone Agent's `app_lock_gate.dart`, carried over with the differences a
/// console implies: the Agent keeps routing SMS behind its lock, where this app
/// has nothing running to protect — only what is on the screen and the session
/// behind it. Signing out is offered here because a console sign-out costs a
/// password, where the Agent's would un-register the phone.
///
/// Mounted from `MaterialApp.builder` so it sits above the Navigator and covers
/// every route, including dialogs — and so the shell underneath is never torn
/// down, which is what lets a half-written SMS survive the phone being pocketed.
class AppLockGate extends StatefulWidget {
  const AppLockGate({super.key, required this.child, this.clock = DateTime.now});

  final Widget child;

  /// Seam for the tests, which cannot sit out a 30-second grace window.
  final DateTime Function() clock;

  @override
  State<AppLockGate> createState() => AppLockGateState();
}

@visibleForTesting
class AppLockGateState extends State<AppLockGate> with WidgetsBindingObserver {
  bool _locked = false;
  bool _prompting = false;
  bool _confirmingSignOut = false;
  String? _error;
  DateTime? _leftAt;

  /// Set when the UI was torn off the engine entirely — the app was closed
  /// rather than merely backgrounded. It re-locks on return regardless of the
  /// grace period, because closing the app is a deliberate exit.
  bool _wasDetached = false;

  @visibleForTesting
  bool get locked => _locked;

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

  /// Switching App lock off from Settings clears an armed gate; signing out
  /// disarms it too, since [AppLockController.armed] needs a session.
  void _onLockPrefChanged() {
    if (!appLock.armed && _locked) setState(() => _locked = false);
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    // The biometric sheet pauses the app itself. Ignoring lifecycle changes
    // while a prompt is up stops it from re-arming the lock behind its own
    // dialog — which would loop forever.
    if (_prompting) return;

    if (state == AppLifecycleState.resumed) {
      final away = widget.clock().difference(_leftAt ?? widget.clock());
      final mustLock = _wasDetached || away >= AppConfig.appLockGrace;
      // Clear both before acting, so the next trip out is timed from scratch
      // rather than from a stamp left over by this one.
      _wasDetached = false;
      _leftAt = null;
      if (appLock.armed && !_locked && mustLock) {
        setState(() => _locked = true);
        _unlock();
      }
      return;
    }

    // Every other state means the UI is on its way out. Stamp the clock on the
    // first of them — Android delivers inactive → paused → detached, and which
    // ones arrive varies by version, so this must not depend on catching a
    // particular one.
    _leftAt ??= widget.clock();
    if (state == AppLifecycleState.detached) _wasDetached = true;
  }

  Future<void> _unlock() async {
    if (_prompting) return;
    setState(() {
      _prompting = true;
      _error = null;
    });
    final out = await biometrics.authenticate('Unlock the gsmnode console');

    // A phone that can no longer prompt at all — biometrics gone *and* the
    // screen lock removed — would otherwise be stuck here, since disabling App
    // lock needs the same prompt. Let them back in and disarm.
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

  Future<void> _signOut() async {
    await auth.logout();
    if (!mounted) return;
    // The session is gone, so the gate has nothing left to guard; the root
    // widget swaps in the login screen underneath.
    setState(() {
      _locked = false;
      _confirmingSignOut = false;
      _error = null;
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
              confirmingSignOut: _confirmingSignOut,
              onUnlock: _unlock,
              onSignOut: _signOut,
              onSignOutIntent: (v) => setState(() => _confirmingSignOut = v),
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
    required this.confirmingSignOut,
    required this.onUnlock,
    required this.onSignOut,
    required this.onSignOutIntent,
  });

  final bool busy;
  final String? error;
  final bool confirmingSignOut;
  final VoidCallback onUnlock;
  final VoidCallback onSignOut;
  final ValueChanged<bool> onSignOutIntent;

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
                  const Center(child: Eyebrow('Locked')),
                  const SizedBox(height: 28),
                  Icon(Icons.fingerprint, size: 56, color: cg.textMuted),
                  const SizedBox(height: 12),
                  Text(
                    'Your session is still signed in — nothing has to be typed '
                    'again.',
                    textAlign: TextAlign.center,
                    style: TextStyle(color: cg.textSecondary),
                  ),
                  if (auth.user != null) ...[
                    const SizedBox(height: 10),
                    Center(
                      child: Text(
                        auth.user!.email,
                        style: gsmMono(size: 11, color: cg.textMuted),
                      ),
                    ),
                  ],
                  if (error != null) ...[
                    const SizedBox(height: 16),
                    MessageBanner(error!),
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
                  const SizedBox(height: 10),
                  // Confirmed inline, not in a dialog: this gate is drawn above
                  // the navigator, so there is none to push one onto.
                  if (!confirmingSignOut)
                    TextButton(
                      onPressed: busy ? null : () => onSignOutIntent(true),
                      style: TextButton.styleFrom(foregroundColor: cg.textMuted),
                      child: const Text('Sign out instead'),
                    )
                  else ...[
                    Text(
                      'Sign out? You will need your email and password to get '
                      'back in.',
                      textAlign: TextAlign.center,
                      style: TextStyle(fontSize: 12, color: cg.textMuted),
                    ),
                    const SizedBox(height: 10),
                    Row(
                      children: [
                        Expanded(
                          child: OutlinedButton(
                            onPressed: () => onSignOutIntent(false),
                            child: const Text('Cancel'),
                          ),
                        ),
                        const SizedBox(width: 10),
                        Expanded(
                          child: OutlinedButton(
                            onPressed: onSignOut,
                            style: OutlinedButton.styleFrom(
                                foregroundColor: cg.danger),
                            child: const Text('Sign out'),
                          ),
                        ),
                      ],
                    ),
                  ],
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}
