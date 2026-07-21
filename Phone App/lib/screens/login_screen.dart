import 'package:flutter/material.dart';

import '../config.dart';
import '../main.dart';
import '../services/api_client.dart';
import '../theme.dart';
import '../widgets/gsmnode_mark.dart';
import '../widgets/ui.dart';

/// Sign-in, mirroring the Web App's `Login.vue` — including its collapsible
/// **Server settings** panel. On the phone that panel is not optional the way it
/// is in the browser: there is no built-in proxy to fall back on, so the app has
/// to be pointed at an API Server explicitly.
///
/// Nothing here navigates on success: [AuthStore] notifies, and the root
/// [GsmNodeConsoleApp] swaps `home` for the shell.
class LoginScreen extends StatefulWidget {
  const LoginScreen({super.key});

  @override
  State<LoginScreen> createState() => _LoginScreenState();
}

class _LoginScreenState extends State<LoginScreen> {
  final _email = TextEditingController();
  final _password = TextEditingController();
  late final _serverUrl =
      TextEditingController(text: storage.apiBase ?? AppConfig.defaultApiBase);

  // Opened on a fresh install (nobody has moved the app off the built-in
  // default yet), collapsed once a real server has been saved.
  late bool _showServer = storage.apiBase == AppConfig.defaultApiBase;
  bool _showPassword = false;
  bool _busy = false;
  String? _error;

  @override
  void dispose() {
    _email.dispose();
    _password.dispose();
    _serverUrl.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      storage.apiBase = _serverUrl.text.trim().replaceAll(RegExp(r'/+$'), '');
      await auth.login(_email.text.trim(), _password.text);
      // The root widget rebuilds into the shell; nothing to do here.
    } on ApiException catch (e) {
      setState(() => _error = switch (e.status) {
            401 => 'Invalid email or password.',
            0 => 'Cannot reach the API Server. Check the server URL.',
            _ => e.message,
          });
    } catch (e) {
      setState(() => _error = e.toString());
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final cg = context.cg;
    return Scaffold(
      body: Center(
        child: SingleChildScrollView(
          padding: const EdgeInsets.all(24),
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 420),
            child: GsmCard(
              padding: const EdgeInsets.all(24),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  const Center(child: GsmNodeMark(size: 52)),
                  const SizedBox(height: 14),
                  const Center(child: GsmNodeWordmark(size: 26)),
                  const SizedBox(height: 10),
                  const Center(child: Eyebrow('Sign in to your gateway')),
                  const SizedBox(height: 26),

                  LabeledField(
                    label: 'Email',
                    child: TextField(
                      controller: _email,
                      keyboardType: TextInputType.emailAddress,
                      autocorrect: false,
                      textInputAction: TextInputAction.next,
                      decoration:
                          const InputDecoration(hintText: 'you@example.com'),
                    ),
                  ),
                  const SizedBox(height: 14),
                  LabeledField(
                    label: 'Password',
                    child: TextField(
                      controller: _password,
                      obscureText: !_showPassword,
                      textInputAction: TextInputAction.done,
                      onSubmitted: (_) => _busy ? null : _submit(),
                      decoration: InputDecoration(
                        hintText: '••••••••',
                        suffixIcon: IconButton(
                          iconSize: 20,
                          color: cg.textMuted,
                          tooltip:
                              _showPassword ? 'Hide password' : 'Show password',
                          icon: Icon(_showPassword
                              ? Icons.visibility_off
                              : Icons.visibility),
                          onPressed: () =>
                              setState(() => _showPassword = !_showPassword),
                        ),
                      ),
                    ),
                  ),
                  const SizedBox(height: 16),

                  _serverSettings(cg),

                  if (_error != null) ...[
                    const SizedBox(height: 16),
                    MessageBanner(_error!),
                  ],
                  const SizedBox(height: 18),
                  SizedBox(
                    height: 48,
                    child: FilledButton(
                      onPressed: _busy ? null : _submit,
                      child: _busy
                          ? const SizedBox(
                              height: 20,
                              width: 20,
                              child: CircularProgressIndicator(
                                strokeWidth: 2,
                                color: Colors.white,
                              ),
                            )
                          : const Text('Sign in'),
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

  Widget _serverSettings(GsmSemantic cg) {
    return Container(
      decoration: BoxDecoration(
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: cg.borderSubtle),
      ),
      child: Column(
        children: [
          InkWell(
            borderRadius: BorderRadius.circular(8),
            onTap: () => setState(() => _showServer = !_showServer),
            child: Padding(
              padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 11),
              child: Row(
                children: [
                  Icon(Icons.tune, size: 16, color: cg.textSecondary),
                  const SizedBox(width: 8),
                  Expanded(
                    child: Text(
                      'Server settings',
                      style: TextStyle(
                        fontSize: 13,
                        fontWeight: FontWeight.w600,
                        color: cg.textSecondary,
                      ),
                    ),
                  ),
                  Icon(
                    _showServer ? Icons.expand_less : Icons.expand_more,
                    size: 18,
                    color: cg.textMuted,
                  ),
                ],
              ),
            ),
          ),
          if (_showServer) ...[
            Divider(height: 1, color: cg.borderSubtle),
            Padding(
              padding: const EdgeInsets.all(12),
              child: LabeledField(
                label: 'API Server URL',
                help: 'Use the server\'s LAN address — "localhost" would point '
                    'at this phone. 10.0.2.2 reaches the host from an emulator.',
                child: TextField(
                  controller: _serverUrl,
                  keyboardType: TextInputType.url,
                  autocorrect: false,
                  style: gsmMono(size: 13, color: cg.textPrimary),
                  decoration: const InputDecoration(
                    hintText: 'http://10.2.1.101:8080',
                  ),
                ),
              ),
            ),
          ],
        ],
      ),
    );
  }
}
