import 'package:flutter/material.dart';

import '../../main.dart';
import '../../services/api_client.dart';
import '../../theme.dart';
import '../../widgets/ui.dart';

/// Manager-only user administration, mirroring `UsersManager.vue`.
///
/// A superadmin sees and edits everyone (including other superadmins) and may
/// move users between organizations; an admin manages only their own
/// organization's members and cannot touch superadmins. The UI mirrors those
/// limits — the API Server is what enforces them.
class UsersManager extends StatefulWidget {
  const UsersManager({super.key});

  @override
  State<UsersManager> createState() => _UsersManagerState();
}

class _UsersManagerState extends State<UsersManager> {
  List<Map<String, dynamic>> _users = const [];
  List<Map<String, dynamic>> _orgs = const [];
  String? _error;
  bool _busy = false;

  /// The user id being edited, `'new'` for the create form, or null for none.
  String? _editing;
  final _email = TextEditingController();
  final _name = TextEditingController();
  final _password = TextEditingController();
  String _role = 'user';
  String _org = '';

  bool get _isSuperadmin => auth.user?.isSuperadmin ?? false;
  List<String> get _roles =>
      _isSuperadmin ? const ['user', 'admin', 'superadmin'] : const ['user', 'admin'];

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    _email.dispose();
    _name.dispose();
    _password.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    try {
      final results = await Future.wait([
        apiClient.get('/users'),
        apiClient.get('/orgs'),
      ]);
      if (!mounted) return;
      setState(() {
        _users = itemsOf(results[0], 'users');
        _orgs = itemsOf(results[1], 'organizations');
        _error = null;
      });
    } catch (e) {
      if (!mounted) return;
      setState(() => _error = describeError(e));
    }
  }

  void _startNew() {
    setState(() {
      _editing = 'new';
      _email.text = '';
      _name.text = '';
      _password.text = '';
      _role = 'user';
      // A superadmin picks the org; an admin's new users land in their own.
      _org = _isSuperadmin ? '' : (auth.user?.organization ?? '');
    });
  }

  void _startEdit(Map<String, dynamic> u) {
    setState(() {
      _editing = (u['id'] ?? '').toString();
      _email.text = (u['email'] ?? '').toString();
      _name.text = (u['name'] ?? '').toString();
      _password.text = '';
      _role = (u['role'] ?? 'user').toString();
      _org = (u['organization'] ?? '').toString();
    });
  }

  Future<void> _save() async {
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final body = <String, dynamic>{
        'email': _email.text.trim(),
        'name': _name.text.trim(),
        'role': _role,
        'organization': _org,
      };
      if (_editing == 'new') {
        body['password'] = _password.text;
        await apiClient.post('/users', body);
      } else {
        // Only send a password when one was typed — blank means "leave it".
        if (_password.text.isNotEmpty) body['password'] = _password.text;
        await apiClient.patch('/users/$_editing', body);
      }
      if (!mounted) return;
      setState(() => _editing = null);
      await _load();
    } catch (e) {
      if (!mounted) return;
      setState(() => _error = describeError(e));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _remove(Map<String, dynamic> u) async {
    final ok = await confirmDialog(
      context,
      title: 'Delete user',
      message: 'Delete ${u['email']}? This cannot be undone.',
    );
    if (!ok) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await apiClient.delete('/users/${u['id']}');
      await _load();
    } catch (e) {
      if (!mounted) return;
      setState(() => _error = describeError(e));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  /// An admin may not touch a superadmin; a superadmin may touch anyone.
  bool _canManage(Map<String, dynamic> u) =>
      _isSuperadmin || u['role'] != 'superadmin';

  @override
  Widget build(BuildContext context) {
    final cg = context.cg;
    final meId = auth.user?.id;

    return GsmCard(
      padding: EdgeInsets.zero,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(16, 14, 10, 12),
            child: Row(
              children: [
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        'Users',
                        style: TextStyle(
                          fontSize: 15,
                          fontWeight: FontWeight.w600,
                          color: cg.textPrimary,
                        ),
                      ),
                      const SizedBox(height: 2),
                      Text(
                        _isSuperadmin ? 'All accounts' : 'Accounts you can manage',
                        style: TextStyle(fontSize: 11, color: cg.textMuted),
                      ),
                    ],
                  ),
                ),
                TextButton.icon(
                  onPressed: _startNew,
                  icon: const Icon(Icons.person_add_alt, size: 16),
                  label: const Text('New'),
                ),
              ],
            ),
          ),
          if (_error != null)
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 0, 16, 12),
              child: MessageBanner(_error!),
            ),
          if (_editing != null) _editor(cg),
          Divider(height: 1, color: cg.borderSubtle),
          if (_users.isEmpty)
            Padding(
              padding: const EdgeInsets.symmetric(vertical: 24),
              child: Text(
                'No users yet.',
                textAlign: TextAlign.center,
                style: TextStyle(fontSize: 13, color: cg.textMuted),
              ),
            )
          else
            for (final u in _users) _userRow(cg, u, meId),
        ],
      ),
    );
  }

  Widget _editor(GsmSemantic cg) {
    return Container(
      color: cg.sunkenBg,
      padding: const EdgeInsets.all(16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          LabeledField(
            label: 'Email',
            child: TextField(
              controller: _email,
              keyboardType: TextInputType.emailAddress,
              autocorrect: false,
            ),
          ),
          const SizedBox(height: 14),
          LabeledField(
            label: 'Name',
            child: TextField(controller: _name),
          ),
          const SizedBox(height: 14),
          LabeledField(
            label: _editing == 'new' ? 'Password' : 'New password',
            child: TextField(
              controller: _password,
              obscureText: true,
              decoration: InputDecoration(
                hintText: _editing == 'new'
                    ? 'at least 8 characters'
                    : 'leave blank to keep',
              ),
            ),
          ),
          const SizedBox(height: 14),
          LabeledField(
            label: 'Role',
            child: GsmDropdown<String>(
              value: _roles.contains(_role) ? _role : _roles.first,
              items: [
                for (final r in _roles)
                  DropdownMenuItem(value: r, child: Text(r)),
              ],
              onChanged: (v) => setState(() => _role = v ?? 'user'),
            ),
          ),
          if (_isSuperadmin) ...[
            const SizedBox(height: 14),
            LabeledField(
              label: 'Organization',
              child: GsmDropdown<String>(
                value: _orgs.any((o) => o['id'] == _org) ? _org : '',
                items: [
                  const DropdownMenuItem(value: '', child: Text('None')),
                  for (final o in _orgs)
                    DropdownMenuItem(
                      value: (o['id'] ?? '').toString(),
                      child: Text((o['name'] ?? '').toString(),
                          overflow: TextOverflow.ellipsis),
                    ),
                ],
                onChanged: (v) => setState(() => _org = v ?? ''),
              ),
            ),
          ],
          const SizedBox(height: 16),
          Row(
            children: [
              Expanded(
                child: FilledButton(
                  onPressed: _busy ? null : _save,
                  child: Text(_editing == 'new' ? 'Create' : 'Save'),
                ),
              ),
              const SizedBox(width: 10),
              Expanded(
                child: OutlinedButton(
                  onPressed: _busy ? null : () => setState(() => _editing = null),
                  child: const Text('Cancel'),
                ),
              ),
            ],
          ),
        ],
      ),
    );
  }

  Widget _userRow(GsmSemantic cg, Map<String, dynamic> u, String? meId) {
    final role = (u['role'] ?? 'user').toString();
    final (roleFg, roleBg) = switch (role) {
      'superadmin' => (cg.info, cg.infoTint),
      'admin' => (cg.warning, cg.warningTint),
      _ => (cg.textMuted, cg.sunkenBg),
    };
    final isMe = meId != null && u['id'] == meId;

    return Container(
      padding: const EdgeInsets.fromLTRB(16, 12, 8, 12),
      decoration: BoxDecoration(
        border: Border(top: BorderSide(color: cg.borderSubtle)),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  (u['email'] ?? '').toString(),
                  style: gsmMono(size: 12, color: cg.textPrimary),
                ),
                const SizedBox(height: 6),
                Wrap(
                  spacing: 6,
                  runSpacing: 6,
                  children: [
                    MonoChip(role, color: roleFg, background: roleBg),
                    if (isMe) const MonoChip('you'),
                    if ((u['name'] ?? '').toString().isNotEmpty)
                      Text(
                        u['name'].toString(),
                        style: TextStyle(fontSize: 12, color: cg.textSecondary),
                      ),
                    if ((u['organizationName'] ?? '').toString().isNotEmpty)
                      Text(
                        u['organizationName'].toString(),
                        style: TextStyle(fontSize: 12, color: cg.textSecondary),
                      ),
                  ],
                ),
              ],
            ),
          ),
          if (_canManage(u))
            IconButton(
              tooltip: 'Edit',
              onPressed: () => _startEdit(u),
              icon: Icon(Icons.edit_outlined, size: 18, color: cg.textSecondary),
            ),
          if (_canManage(u) && !isMe)
            IconButton(
              tooltip: 'Delete',
              onPressed: _busy ? null : () => _remove(u),
              icon: Icon(Icons.delete_outline, size: 18, color: cg.danger),
            ),
        ],
      ),
    );
  }
}
