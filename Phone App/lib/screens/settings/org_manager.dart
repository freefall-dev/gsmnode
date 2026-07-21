import 'package:flutter/material.dart';

import '../../main.dart';
import '../../services/api_client.dart';
import '../../theme.dart';
import '../../widgets/ui.dart';

/// Organization management, mirroring `OrgManager.vue` and adapting to who's
/// looking:
///  - a user with no organization gets a "create your own" form and becomes the
///    admin of what they create;
///  - an admin sees their own org with Rename + Delete (deleting it removes them
///    from it and drops them back to a plain user);
///  - a superadmin sees every org and can create, rename, and delete any of them.
///
/// The API Server enforces all of this; this just mirrors it.
class OrgManager extends StatefulWidget {
  const OrgManager({super.key});

  @override
  State<OrgManager> createState() => _OrgManagerState();
}

class _OrgManagerState extends State<OrgManager> {
  final _draftName = TextEditingController();
  final _createName = TextEditingController();

  List<Map<String, dynamic>> _orgs = const [];
  String? _error;
  bool _busy = false;

  /// The org id being renamed, `'new'` for the create form, or null for none.
  String? _editing;

  bool get _isSuperadmin => auth.user?.isSuperadmin ?? false;
  bool get _isManager => auth.user?.isManager ?? false;
  bool get _showCreateOwn =>
      !_isSuperadmin && (auth.user?.organization ?? '').isEmpty;

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    _draftName.dispose();
    _createName.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    // Only managers may list organizations; an org-less user just gets the form.
    if (!_isManager) {
      if (mounted) setState(() => _orgs = const []);
      return;
    }
    try {
      final res = await apiClient.get('/orgs');
      if (!mounted) return;
      setState(() {
        _orgs = itemsOf(res, 'organizations');
        _error = null;
      });
    } catch (e) {
      if (!mounted) return;
      setState(() => _error = describeError(e));
    }
  }

  /// An org-less user creates their first org and is promoted to its admin.
  Future<void> _createOwn() async {
    final name = _createName.text.trim();
    if (name.isEmpty) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await apiClient.post('/orgs', {'name': name});
      _createName.clear();
      await auth.refresh(); // role -> admin, organization set
      await _load();
    } catch (e) {
      if (!mounted) return;
      setState(() => _error = describeError(e));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _save() async {
    final name = _draftName.text.trim();
    if (name.isEmpty) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      if (_editing == 'new') {
        await apiClient.post('/orgs', {'name': name});
      } else {
        await apiClient.patch('/orgs/$_editing', {'name': name});
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

  Future<void> _remove(Map<String, dynamic> o) async {
    final mine = !_isSuperadmin && o['id'] == auth.user?.organization;
    final ok = await confirmDialog(
      context,
      title: 'Delete organization',
      message: mine
          ? 'Delete your organization "${o['name']}"? You\'ll be removed from it '
              'and become a regular user. This cannot be undone.'
          : 'Delete organization "${o['name']}"? This cannot be undone.',
    );
    if (!ok) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await apiClient.delete('/orgs/${o['id']}');
      if (mine) await auth.refresh(); // now org-less + demoted to user
      await _load();
    } catch (e) {
      // The server refuses (409) while the org still has other members.
      if (!mounted) return;
      setState(() => _error = describeError(e));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final cg = context.cg;
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
                        _isSuperadmin ? 'Organizations' : 'Organization',
                        style: TextStyle(
                          fontSize: 15,
                          fontWeight: FontWeight.w600,
                          color: cg.textPrimary,
                        ),
                      ),
                      const SizedBox(height: 2),
                      Text(
                        _isSuperadmin
                            ? 'Tenants your users belong to'
                            : _showCreateOwn
                                ? 'Create one to manage your own team'
                                : 'Your organization',
                        style: TextStyle(fontSize: 11, color: cg.textMuted),
                      ),
                    ],
                  ),
                ),
                if (_isSuperadmin)
                  TextButton.icon(
                    onPressed: () => setState(() {
                      _editing = 'new';
                      _draftName.text = '';
                    }),
                    icon: const Icon(Icons.add, size: 16),
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
          if (_showCreateOwn)
            _createOwnForm(cg)
          else ...[
            if (_editing != null) _renameForm(cg),
            Divider(height: 1, color: cg.borderSubtle),
            if (_orgs.isEmpty)
              Padding(
                padding: const EdgeInsets.symmetric(vertical: 24),
                child: Text(
                  'No organizations yet.',
                  textAlign: TextAlign.center,
                  style: TextStyle(fontSize: 13, color: cg.textMuted),
                ),
              )
            else
              for (final o in _orgs) _orgRow(cg, o),
          ],
        ],
      ),
    );
  }

  Widget _createOwnForm(GsmSemantic cg) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 4, 16, 16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          LabeledField(
            label: 'Organization name',
            help: 'You\'ll become its admin and can then add and manage users.',
            child: TextField(
              controller: _createName,
              decoration: const InputDecoration(hintText: 'Acme Inc.'),
              onSubmitted: (_) => _busy ? null : _createOwn(),
            ),
          ),
          const SizedBox(height: 14),
          FilledButton(
            onPressed: _busy ? null : _createOwn,
            child: const Text('Create organization'),
          ),
        ],
      ),
    );
  }

  Widget _renameForm(GsmSemantic cg) {
    return Container(
      color: cg.sunkenBg,
      padding: const EdgeInsets.all(16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          LabeledField(
            label: 'Name',
            child: TextField(
              controller: _draftName,
              decoration: const InputDecoration(hintText: 'Acme Inc.'),
              onSubmitted: (_) => _busy ? null : _save(),
            ),
          ),
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

  Widget _orgRow(GsmSemantic cg, Map<String, dynamic> o) {
    return Container(
      padding: const EdgeInsets.fromLTRB(16, 12, 8, 12),
      decoration: BoxDecoration(
        border: Border(top: BorderSide(color: cg.borderSubtle)),
      ),
      child: Row(
        children: [
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  (o['name'] ?? '').toString(),
                  style: TextStyle(
                    fontSize: 14,
                    fontWeight: FontWeight.w600,
                    color: cg.textPrimary,
                  ),
                ),
                const SizedBox(height: 4),
                Text(
                  (o['id'] ?? '').toString(),
                  style: gsmMono(size: 11, color: cg.textMuted),
                ),
              ],
            ),
          ),
          IconButton(
            tooltip: 'Rename',
            onPressed: () => setState(() {
              _editing = (o['id'] ?? '').toString();
              _draftName.text = (o['name'] ?? '').toString();
            }),
            icon: Icon(Icons.edit_outlined, size: 18, color: cg.textSecondary),
          ),
          IconButton(
            tooltip: 'Delete',
            onPressed: _busy ? null : () => _remove(o),
            icon: Icon(Icons.delete_outline, size: 18, color: cg.danger),
          ),
        ],
      ),
    );
  }
}
