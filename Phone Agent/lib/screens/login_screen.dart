import 'package:flutter/material.dart';

import '../main.dart';
import '../services/api_client.dart';
import '../theme.dart';
import '../widgets/gsmnode_mark.dart';
import 'home_screen.dart';

/// Login + device registration. The user authenticates against the API Server,
/// then this phone is registered as a gateway device.
class LoginScreen extends StatefulWidget {
  const LoginScreen({super.key});

  @override
  State<LoginScreen> createState() => _LoginScreenState();
}

class _LoginScreenState extends State<LoginScreen> {
  final _apiBase = TextEditingController(text: storage.apiBase ?? '');
  final _email = TextEditingController(text: storage.userEmail ?? '');
  final _password = TextEditingController();
  final _deviceName = TextEditingController(text: storage.deviceName ?? 'My Phone');
  final _passphrase = TextEditingController(text: storage.encPassphrase);

  bool _busy = false;
  String? _error;

  @override
  void dispose() {
    _apiBase.dispose();
    _email.dispose();
    _password.dispose();
    _deviceName.dispose();
    _passphrase.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      storage.apiBase = _apiBase.text.trim();
      storage.encPassphrase = _passphrase.text.trim();
      await apiClient.login(_email.text.trim(), _password.text);

      final deviceId = storage.deviceId ??
          'android-${DateTime.now().millisecondsSinceEpoch}';
      await apiClient.registerDevice(
        deviceId: deviceId,
        name: _deviceName.text.trim().isEmpty ? 'My Phone' : _deviceName.text.trim(),
      );

      if (!mounted) return;
      Navigator.of(context).pushReplacement(
        MaterialPageRoute(builder: (_) => const HomeScreen()),
      );
    } on ApiException catch (e) {
      setState(() => _error =
          e.status == 401 ? 'Invalid email or password.' : e.message);
    } catch (e) {
      setState(() => _error = 'Could not connect: $e');
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
                    'CONNECT THIS PHONE TO YOUR GATEWAY',
                    textAlign: TextAlign.center,
                    style: gsmMono(
                      size: 10,
                      color: cg.textMuted,
                      letterSpacing: 1.4,
                    ),
                  ),
                ),
                const SizedBox(height: 28),
                TextField(
                  controller: _apiBase,
                  decoration: const InputDecoration(
                    labelText: 'API Server URL',
                    hintText: 'http://10.0.2.2:8080',
                  ),
                  style: gsmMono(size: 14, color: cg.textPrimary),
                  keyboardType: TextInputType.url,
                ),
                const SizedBox(height: 12),
                TextField(
                  controller: _email,
                  decoration: const InputDecoration(labelText: 'Email'),
                  keyboardType: TextInputType.emailAddress,
                ),
                const SizedBox(height: 12),
                TextField(
                  controller: _password,
                  decoration: const InputDecoration(labelText: 'Password'),
                  obscureText: true,
                ),
                const SizedBox(height: 12),
                TextField(
                  controller: _deviceName,
                  decoration: const InputDecoration(labelText: 'Device name'),
                ),
                const SizedBox(height: 12),
                TextField(
                  controller: _passphrase,
                  decoration: const InputDecoration(
                    labelText: 'Encryption passphrase (optional)',
                    hintText: 'Match the Web App to read E2E messages',
                  ),
                  obscureText: true,
                ),
                const SizedBox(height: 16),
                if (_error != null)
                  Container(
                    padding: const EdgeInsets.all(12),
                    decoration: BoxDecoration(
                      color: cg.dangerTint,
                      borderRadius: BorderRadius.circular(10),
                    ),
                    child: Text(_error!, style: TextStyle(color: cg.danger)),
                  ),
                const SizedBox(height: 12),
                FilledButton(
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
                      : const Text('Sign in & register device'),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
