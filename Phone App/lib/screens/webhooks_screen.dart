import 'package:flutter/material.dart';

import '../main.dart';
import '../services/api_client.dart';
import '../theme.dart';
import '../widgets/ui.dart';

/// Register and delete callbacks for gateway events — the phone's
/// `Webhooks.vue`.
class WebhooksScreen extends StatefulWidget {
  const WebhooksScreen({super.key});

  @override
  State<WebhooksScreen> createState() => _WebhooksScreenState();
}

class _WebhooksScreenState extends State<WebhooksScreen> {
  /// The events the API Server emits (see the root README's Webhooks section).
  static const _events = [
    'sms:received',
    'sms:sent',
    'sms:delivered',
    'sms:failed',
    'sms:data-received',
    'mms:received',
    'mms:downloaded',
    'call:received',
    'call:sent',
    'call:failed',
  ];

  final _url = TextEditingController();

  List<Map<String, dynamic>> _hooks = const [];
  bool _loading = true;
  bool _creating = false;
  String? _error;
  String _event = _events.first;

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    _url.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    if (mounted) setState(() => _loading = true);
    try {
      final res = await apiClient.get('/webhooks');
      if (!mounted) return;
      setState(() {
        _hooks = itemsOf(res);
        _error = null;
        _loading = false;
      });
    } catch (e) {
      if (!mounted) return;
      setState(() {
        _error = describeError(e);
        _loading = false;
      });
    }
  }

  Future<void> _create() async {
    final url = _url.text.trim();
    if (url.isEmpty) {
      setState(() => _error = 'URL is required.');
      return;
    }
    setState(() {
      _creating = true;
      _error = null;
    });
    try {
      final hook = await apiClient
          .post('/webhooks', {'event': _event, 'url': url}) as Map<String, dynamic>;
      if (!mounted) return;
      setState(() {
        _hooks = [hook.cast<String, dynamic>(), ..._hooks];
        _url.clear();
      });
    } catch (e) {
      if (!mounted) return;
      setState(() => _error = describeError(e));
    } finally {
      if (mounted) setState(() => _creating = false);
    }
  }

  Future<void> _remove(Map<String, dynamic> h) async {
    final ok = await confirmDialog(
      context,
      title: 'Delete webhook',
      message: 'Delete the ${h['event']} callback to ${h['url']}?',
    );
    if (!ok) return;
    try {
      await apiClient.delete('/webhooks/${h['id']}');
      if (!mounted) return;
      setState(() => _hooks =
          _hooks.where((x) => x['id'] != h['id']).toList(growable: false));
    } catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text('Could not delete: ${describeError(e)}')),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    final cg = context.cg;
    return RefreshIndicator(
      onRefresh: _load,
      child: ListView(
        padding: const EdgeInsets.fromLTRB(16, 18, 16, 32),
        children: [
          const PageHeader(
            title: 'Webhooks',
            subtitle: 'Get notified when messages change state or arrive',
          ),
          GsmCard(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                LabeledField(
                  label: 'Event',
                  child: GsmDropdown<String>(
                    value: _event,
                    items: [
                      for (final e in _events)
                        DropdownMenuItem(
                          value: e,
                          child: Text(e,
                              style: gsmMono(size: 12, color: cg.textPrimary)),
                        ),
                    ],
                    onChanged: (v) => setState(() => _event = v ?? _events.first),
                  ),
                ),
                const SizedBox(height: 16),
                LabeledField(
                  label: 'Target URL',
                  child: TextField(
                    controller: _url,
                    keyboardType: TextInputType.url,
                    autocorrect: false,
                    style: gsmMono(size: 12, color: cg.textPrimary),
                    decoration: const InputDecoration(
                      hintText: 'https://example.com/hook',
                    ),
                  ),
                ),
                const SizedBox(height: 16),
                SizedBox(
                  height: 46,
                  child: FilledButton.icon(
                    onPressed: _creating ? null : _create,
                    icon: const Icon(Icons.add, size: 18),
                    label: const Text('Add webhook'),
                  ),
                ),
              ],
            ),
          ),
          const SizedBox(height: 20),
          if (_error != null) ...[
            MessageBanner(_error!),
            const SizedBox(height: 12),
          ],
          if (_loading)
            const Padding(
              padding: EdgeInsets.symmetric(vertical: 40),
              child: Center(child: CircularProgressIndicator()),
            )
          else if (_hooks.isEmpty)
            const EmptyState('No webhooks.')
          else
            for (final h in _hooks)
              Padding(
                padding: const EdgeInsets.only(bottom: 8),
                child: GsmCard(
                  padding: const EdgeInsets.fromLTRB(14, 12, 6, 12),
                  child: Row(
                    children: [
                      Expanded(
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            MonoChip(
                              (h['event'] ?? '').toString(),
                              color: cg.brandActive,
                              background: cg.brandTint,
                            ),
                            const SizedBox(height: 8),
                            Text(
                              (h['url'] ?? '').toString(),
                              style: gsmMono(size: 11, color: cg.textSecondary),
                            ),
                          ],
                        ),
                      ),
                      IconButton(
                        tooltip: 'Delete',
                        onPressed: () => _remove(h),
                        icon: Icon(Icons.delete_outline,
                            size: 20, color: cg.danger),
                      ),
                    ],
                  ),
                ),
              ),
        ],
      ),
    );
  }
}
