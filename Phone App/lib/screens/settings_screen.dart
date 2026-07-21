import 'package:flutter/material.dart';

import '../config.dart';
import '../main.dart';
import '../services/auth_store.dart';
import '../theme.dart';
import '../widgets/ui.dart';
import 'settings/integrations_section.dart';
import 'settings/org_manager.dart';
import 'settings/users_manager.dart';

/// Account, encryption, appearance and administration — the phone's
/// `Settings.vue`, plus one section the browser doesn't need: the API Server
/// URL, which on the web lives on the login screen because the site can always
/// fall back on its own BFF.
class SettingsScreen extends StatefulWidget {
  const SettingsScreen({super.key});

  @override
  State<SettingsScreen> createState() => _SettingsScreenState();
}

class _SettingsScreenState extends State<SettingsScreen> {
  String _tab = 'general';

  // Server
  late final _serverUrl = TextEditingController(text: storage.apiBase ?? '');
  bool _serverSaved = false;

  // Account
  late final _name = TextEditingController(text: auth.user?.name ?? '');
  bool _nameSaving = false;
  bool _nameSaved = false;
  String? _nameError;

  // App lock
  bool? _lockSupported;
  String _lockMethod = 'Face or fingerprint';
  bool _lockBusy = false;
  String? _lockError;

  // End-to-end encryption
  late final _passphrase = TextEditingController(text: storage.encPassphrase);
  bool _showPassphrase = false;

  // Change password
  final _oldPassword = TextEditingController();
  final _newPassword = TextEditingController();
  final _confirmPassword = TextEditingController();
  bool _passwordSaving = false;
  bool _passwordSaved = false;
  String? _passwordError;

  @override
  void initState() {
    super.initState();
    _loadLockCapability();
  }

  @override
  void dispose() {
    _serverUrl.dispose();
    _name.dispose();
    _passphrase.dispose();
    _oldPassword.dispose();
    _newPassword.dispose();
    _confirmPassword.dispose();
    super.dispose();
  }

  // --- actions --------------------------------------------------------------

  void _saveServer() {
    storage.apiBase = _serverUrl.text.trim().replaceAll(RegExp(r'/+$'), '');
    setState(() => _serverSaved = true);
  }

  Future<void> _saveName() async {
    setState(() {
      _nameSaving = true;
      _nameError = null;
      _nameSaved = false;
    });
    try {
      final updated = await apiClient
          .patch('/auth/me', {'name': _name.text.trim()}) as Map<String, dynamic>;
      final name = (updated['name'] ?? '').toString();
      auth.updateUser(name: name);
      if (!mounted) return;
      setState(() {
        _name.text = name;
        _nameSaved = true;
      });
    } catch (e) {
      if (!mounted) return;
      setState(() => _nameError = describeError(e));
    } finally {
      if (mounted) setState(() => _nameSaving = false);
    }
  }

  Future<void> _loadLockCapability() async {
    final supported = await biometrics.supported;
    final method = await biometrics.methodLabel();
    if (!mounted) return;
    setState(() {
      _lockSupported = supported;
      _lockMethod = method;
    });
  }

  Future<void> _toggleLock(bool value) async {
    setState(() {
      _lockBusy = true;
      _lockError = null;
    });
    // The prompt gates both directions — see [AppLockController.setEnabled].
    final out = await appLock.setEnabled(value);
    if (!mounted) return;
    setState(() {
      _lockBusy = false;
      _lockError = out.passed ? null : out.message;
    });
  }

  void _savePassphrase() {
    storage.encPassphrase = _passphrase.text.trim();
    setState(() {});
  }

  void _clearPassphrase() {
    _passphrase.clear();
    storage.encPassphrase = '';
    setState(() {});
  }

  Future<void> _savePassword() async {
    setState(() {
      _passwordError = null;
      _passwordSaved = false;
    });
    if (_newPassword.text.length < 8) {
      setState(() => _passwordError = 'New password must be at least 8 characters.');
      return;
    }
    if (_newPassword.text != _confirmPassword.text) {
      setState(() => _passwordError = 'New passwords do not match.');
      return;
    }
    setState(() => _passwordSaving = true);
    try {
      await apiClient.post('/auth/change-password', {
        'oldPassword': _oldPassword.text,
        'newPassword': _newPassword.text,
      });
      if (!mounted) return;
      setState(() {
        _oldPassword.clear();
        _newPassword.clear();
        _confirmPassword.clear();
        _passwordSaved = true;
      });
    } catch (e) {
      if (!mounted) return;
      setState(() => _passwordError = describeError(e));
    } finally {
      if (mounted) setState(() => _passwordSaving = false);
    }
  }

  // --- build ----------------------------------------------------------------

  @override
  Widget build(BuildContext context) {
    final user = auth.user;
    // Managers get the Users + Organization sections; an org-less non-superadmin
    // gets Organization alone, so they can stand up their own (which promotes
    // them to its admin). The API Server enforces the finer-grained scoping.
    final isManager = user?.isManager ?? false;
    final showCreateOwn =
        user != null && !user.isSuperadmin && user.organization.isEmpty;

    return ListView(
      padding: const EdgeInsets.fromLTRB(16, 18, 16, 32),
      children: [
        PageHeader(
          title: 'Settings',
          subtitle: _tab == 'integrations'
              ? 'Connect services to your gateway'
              : 'Manage your account and appearance',
        ),
        FilterChipsRow<String>(
          value: _tab,
          onChanged: (t) => setState(() => _tab = t),
          options: const [
            FilterChipOption('general', 'General', icon: Icons.tune),
            FilterChipOption('integrations', 'Integrations', icon: Icons.extension_outlined),
          ],
        ),
        const SizedBox(height: 16),
        if (_tab == 'integrations')
          const IntegrationsSection()
        else ...[
          _serverSection(),
          const SizedBox(height: 14),
          _accountSection(user),
          const SizedBox(height: 14),
          _appLockSection(),
          const SizedBox(height: 14),
          _encryptionSection(),
          const SizedBox(height: 14),
          _passwordSection(),
          const SizedBox(height: 14),
          _appearanceSection(),
          if (isManager || showCreateOwn) ...[
            const SizedBox(height: 22),
            Eyebrow(isManager ? 'Administration' : 'Organization'),
            const SizedBox(height: 12),
            if (isManager) ...[
              const UsersManager(),
              const SizedBox(height: 14),
            ],
            const OrgManager(),
          ],
          const SizedBox(height: 22),
          _sessionSection(),
        ],
      ],
    );
  }

  Widget _serverSection() {
    final cg = context.cg;
    return SectionCard(
      title: 'Server',
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          LabeledField(
            label: 'API Server URL',
            help: 'Every screen in this app talks to this server. Changing it '
                'takes effect on the next request.',
            child: TextField(
              controller: _serverUrl,
              keyboardType: TextInputType.url,
              autocorrect: false,
              style: gsmMono(size: 13, color: cg.textPrimary),
              decoration: const InputDecoration(
                hintText: 'http://10.2.1.101:8080',
              ),
              onChanged: (_) => setState(() => _serverSaved = false),
            ),
          ),
          const SizedBox(height: 14),
          FilledButton(
            onPressed: _saveServer,
            child: Text(_serverSaved ? 'Saved' : 'Save'),
          ),
        ],
      ),
    );
  }

  Widget _accountSection(AppUser? user) {
    final cg = context.cg;
    return SectionCard(
      title: 'Account',
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          LabeledField(
            label: 'Display name',
            child: TextField(
              controller: _name,
              decoration: const InputDecoration(hintText: 'Your name'),
              onChanged: (_) => setState(() => _nameSaved = false),
            ),
          ),
          const SizedBox(height: 12),
          FilledButton(
            onPressed: _nameSaving ? null : _saveName,
            child: Text(_nameSaving
                ? 'Saving…'
                : _nameSaved
                    ? 'Saved'
                    : 'Save'),
          ),
          if (_nameError != null) ...[
            const SizedBox(height: 10),
            MessageBanner(_nameError!),
          ],
          const SizedBox(height: 18),
          Text(
            'Email',
            style: TextStyle(
              fontSize: 13,
              fontWeight: FontWeight.w600,
              color: cg.textSecondary,
            ),
          ),
          const SizedBox(height: 8),
          Wrap(
            spacing: 8,
            runSpacing: 8,
            crossAxisAlignment: WrapCrossAlignment.center,
            children: [
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 7),
                decoration: BoxDecoration(
                  color: cg.sunkenBg,
                  borderRadius: BorderRadius.circular(6),
                  border: Border.all(color: cg.borderSubtle),
                ),
                child: Text(
                  user?.email ?? '',
                  style: gsmMono(size: 11, color: cg.textSecondary),
                ),
              ),
              MonoChip(
                user?.roleLabel ?? 'User',
                color: cg.brandActive,
                background: cg.brandTint,
              ),
              MonoChip(
                (user?.verified ?? false) ? 'Verified' : 'Unverified',
                color: (user?.verified ?? false) ? cg.success : cg.warning,
                background:
                    (user?.verified ?? false) ? cg.successTint : cg.warningTint,
              ),
            ],
          ),
          const SizedBox(height: 8),
          Text(
            'Your email and role are managed by an administrator.',
            style: TextStyle(fontSize: 11, color: cg.textMuted),
          ),
        ],
      ),
    );
  }

  Widget _appLockSection() {
    final cg = context.cg;
    final supported = _lockSupported;
    final on = appLock.enabled;

    return SectionCard(
      title: 'App lock',
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Text(
            'Ask for a face or fingerprint before this console will show '
            'anything. Your session stays signed in — the lock only decides who '
            'gets to see it, and closes again once the app has been in the '
            'background for ${AppConfig.appLockGrace.inSeconds} seconds, or as '
            'soon as the app is closed.',
            style: TextStyle(fontSize: 13, color: cg.textSecondary),
          ),
          const SizedBox(height: 14),
          Row(
            children: [
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      'Require $_lockMethod',
                      style: TextStyle(
                        fontSize: 14,
                        fontWeight: FontWeight.w600,
                        color: (supported ?? false) ? cg.textPrimary : cg.textMuted,
                      ),
                    ),
                    const SizedBox(height: 3),
                    Text(
                      // The screen lock is BiometricPrompt's own fallback, and
                      // worth saying out loud: a failed finger isn't a lockout.
                      // Switching this off asks for it too, so nobody holding an
                      // already-unlocked phone can quietly disarm it.
                      'Your PIN or pattern still works if the sensor won\'t. '
                      'Turning this off asks for it as well.',
                      style: TextStyle(fontSize: 12, color: cg.textMuted),
                    ),
                  ],
                ),
              ),
              const SizedBox(width: 10),
              Switch(
                value: on,
                onChanged:
                    ((supported ?? false) && !_lockBusy) ? _toggleLock : null,
              ),
            ],
          ),
          if (supported == null) ...[
            const SizedBox(height: 12),
            Text(
              'Checking what this phone supports…',
              style: TextStyle(fontSize: 12, color: cg.textMuted),
            ),
          ] else if (!supported) ...[
            const SizedBox(height: 12),
            const MessageBanner(
              'This phone has no biometric or screen lock to prompt with. Add '
              'one in Android Settings, then come back.',
              tone: BannerTone.info,
            ),
          ],
          if (_lockError != null) ...[
            const SizedBox(height: 12),
            MessageBanner(_lockError!),
          ],
          const SizedBox(height: 12),
          Align(
            alignment: Alignment.centerLeft,
            child: MonoChip(
              on ? 'Lock on' : 'Off',
              color: on ? cg.success : cg.textMuted,
              background: on ? cg.successTint : cg.sunkenBg,
            ),
          ),
        ],
      ),
    );
  }

  Widget _encryptionSection() {
    final cg = context.cg;
    final on = _passphrase.text.trim().isNotEmpty;
    return SectionCard(
      title: 'End-to-end encryption',
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Text(
            'Set a shared passphrase to encrypt message text and recipient '
            'numbers before they leave this phone. The server and database only '
            'ever store ciphertext. The passphrase is kept on this device only '
            'and never sent anywhere — enter the same one on every surface (Web '
            'App, Phone Agent) that must read the messages.',
            style: TextStyle(fontSize: 13, color: cg.textSecondary),
          ),
          const SizedBox(height: 14),
          TextField(
            controller: _passphrase,
            obscureText: !_showPassphrase,
            autocorrect: false,
            style: gsmMono(size: 13, color: cg.textPrimary),
            decoration: InputDecoration(
              hintText: 'Encryption passphrase',
              // Typing a shared passphrase on a phone keyboard is error-prone,
              // and a wrong one fails silently later as unreadable messages.
              suffixIcon: IconButton(
                iconSize: 20,
                color: cg.textMuted,
                tooltip: _showPassphrase ? 'Hide passphrase' : 'Show passphrase',
                icon: Icon(_showPassphrase
                    ? Icons.visibility_off
                    : Icons.visibility),
                onPressed: () =>
                    setState(() => _showPassphrase = !_showPassphrase),
              ),
            ),
            onChanged: (_) => setState(() {}),
          ),
          const SizedBox(height: 14),
          Row(
            children: [
              Expanded(
                child: FilledButton(
                  onPressed: on ? _savePassphrase : null,
                  child: const Text('Save'),
                ),
              ),
              if (storage.encPassphrase.isNotEmpty) ...[
                const SizedBox(width: 10),
                Expanded(
                  child: OutlinedButton(
                    onPressed: _clearPassphrase,
                    child: const Text('Clear'),
                  ),
                ),
              ],
            ],
          ),
          const SizedBox(height: 12),
          Align(
            alignment: Alignment.centerLeft,
            child: MonoChip(
              storage.encPassphrase.isNotEmpty ? 'Encryption on' : 'Off',
              color: storage.encPassphrase.isNotEmpty ? cg.success : cg.textMuted,
              background:
                  storage.encPassphrase.isNotEmpty ? cg.successTint : cg.sunkenBg,
            ),
          ),
        ],
      ),
    );
  }

  Widget _passwordSection() {
    return SectionCard(
      title: 'Change password',
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          TextField(
            controller: _oldPassword,
            obscureText: true,
            decoration: const InputDecoration(hintText: 'Current password'),
          ),
          const SizedBox(height: 10),
          TextField(
            controller: _newPassword,
            obscureText: true,
            decoration: const InputDecoration(hintText: 'New password'),
          ),
          const SizedBox(height: 10),
          TextField(
            controller: _confirmPassword,
            obscureText: true,
            decoration: const InputDecoration(hintText: 'Confirm new password'),
          ),
          if (_passwordError != null) ...[
            const SizedBox(height: 12),
            MessageBanner(_passwordError!),
          ],
          const SizedBox(height: 14),
          OutlinedButton(
            onPressed: _passwordSaving ? null : _savePassword,
            child: Text(_passwordSaving
                ? 'Updating…'
                : _passwordSaved
                    ? 'Password updated'
                    : 'Update password'),
          ),
        ],
      ),
    );
  }

  Widget _appearanceSection() {
    final cg = context.cg;
    return SectionCard(
      title: 'Appearance',
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            'Theme',
            style: TextStyle(
              fontSize: 13,
              fontWeight: FontWeight.w600,
              color: cg.textSecondary,
            ),
          ),
          const SizedBox(height: 10),
          FilterChipsRow<String>(
            value: themeController.pref,
            onChanged: themeController.setPref,
            options: const [
              FilterChipOption('light', 'Light', icon: Icons.light_mode_outlined),
              FilterChipOption('dark', 'Dark', icon: Icons.dark_mode_outlined),
              FilterChipOption('system', 'System', icon: Icons.phone_android),
            ],
          ),
          const SizedBox(height: 8),
          Text(
            '“System” follows your phone\'s light/dark setting.',
            style: TextStyle(fontSize: 11, color: cg.textMuted),
          ),
        ],
      ),
    );
  }

  Widget _sessionSection() {
    final cg = context.cg;
    return GsmCard(
      child: Row(
        children: [
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  'Session',
                  style: TextStyle(
                    fontSize: 14,
                    fontWeight: FontWeight.w600,
                    color: cg.textPrimary,
                  ),
                ),
                const SizedBox(height: 3),
                Text(
                  'Sign out of the gateway on this device.',
                  style: TextStyle(fontSize: 13, color: cg.textSecondary),
                ),
              ],
            ),
          ),
          const SizedBox(width: 10),
          OutlinedButton.icon(
            onPressed: () => auth.logout(),
            style: OutlinedButton.styleFrom(foregroundColor: cg.danger),
            icon: const Icon(Icons.logout, size: 16),
            label: const Text('Sign out'),
          ),
        ],
      ),
    );
  }
}
