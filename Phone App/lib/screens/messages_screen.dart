import 'package:flutter/material.dart';

import '../main.dart';
import '../services/api_client.dart';
import '../theme.dart';
import '../widgets/status_badge.dart';
import '../widgets/ui.dart';

/// Outbound message history — the phone's `Messages.vue`. E2E items are
/// decrypted for display; anything the passphrase can't open shows the lock
/// marker rather than a ciphertext blob.
class MessagesScreen extends StatefulWidget {
  const MessagesScreen({super.key});

  @override
  State<MessagesScreen> createState() => _MessagesScreenState();
}

/// A message plus the plaintext to show for it.
class _Row {
  _Row(this.raw, this.recipients, this.text);

  final Map<String, dynamic> raw;
  final List<String> recipients;
  final String text;
}

class _MessagesScreenState extends State<MessagesScreen> {
  static const _statuses = [
    '',
    'Pending',
    'Processed',
    'Sent',
    'Delivered',
    'Failed',
  ];

  /// Icon + label for the non-plain-SMS types, matching `typeMeta` in the SPA.
  static const _typeMeta = {
    'data': (Icons.data_object, 'Data SMS'),
    'mms': (Icons.image_outlined, 'MMS'),
    'call': (Icons.phone_forwarded, 'Voice call'),
  };

  List<_Row> _rows = const [];
  bool _loading = true;
  String? _error;
  String _statusFilter = '';

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    if (mounted) setState(() => _loading = true);
    try {
      final query = _statusFilter.isEmpty
          ? ''
          : '?status=${Uri.encodeQueryComponent(_statusFilter)}';
      final res = await apiClient.get('/messages$query');
      final rows = <_Row>[];
      for (final m in itemsOf(res)) {
        final encrypted = m['encrypted'] == true;
        final numbers = (m['phone_numbers'] as List?) ?? const [];
        rows.add(_Row(
          m,
          encrypted
              ? await crypto.tryDecryptList(numbers)
              : numbers.map((e) => e.toString()).toList(),
          encrypted
              ? await crypto.tryDecrypt(m['text_message'])
              : (m['text_message'] ?? '').toString(),
        ));
      }
      if (!mounted) return;
      setState(() {
        _rows = rows;
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

  @override
  Widget build(BuildContext context) {
    final cg = context.cg;
    return RefreshIndicator(
      onRefresh: _load,
      child: ListView(
        padding: const EdgeInsets.fromLTRB(16, 18, 16, 32),
        children: [
          const PageHeader(
            title: 'Messages',
            subtitle: 'Outbound message history',
          ),
          Row(
            children: [
              Expanded(
                child: GsmDropdown<String>(
                  value: _statusFilter,
                  items: [
                    for (final s in _statuses)
                      DropdownMenuItem(
                        value: s,
                        child: Text(s.isEmpty ? 'All statuses' : s),
                      ),
                  ],
                  onChanged: (v) {
                    setState(() => _statusFilter = v ?? '');
                    _load();
                  },
                ),
              ),
              IconButton(
                tooltip: 'Refresh',
                onPressed: _load,
                icon: Icon(Icons.refresh, size: 20, color: cg.textSecondary),
              ),
            ],
          ),
          const SizedBox(height: 14),
          if (_error != null) ...[
            MessageBanner(_error!),
            const SizedBox(height: 12),
          ],
          if (_loading)
            const Padding(
              padding: EdgeInsets.symmetric(vertical: 40),
              child: Center(child: CircularProgressIndicator()),
            )
          else if (_rows.isEmpty)
            const EmptyState('No messages yet. Queue one from Send SMS.')
          else
            for (final r in _rows)
              Padding(
                padding: const EdgeInsets.only(bottom: 8),
                child: _messageCard(r),
              ),
        ],
      ),
    );
  }

  Widget _messageCard(_Row r) {
    final cg = context.cg;
    final m = r.raw;
    final meta = _typeMeta[m['type']];
    final error = (m['error'] ?? '').toString();

    return GsmCard(
      padding: const EdgeInsets.all(14),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Expanded(
                child: Text(
                  r.recipients.isEmpty ? '—' : r.recipients.join(', '),
                  style: gsmMono(
                    size: 12,
                    color: cg.textPrimary,
                    weight: FontWeight.w600,
                  ),
                ),
              ),
              const SizedBox(width: 8),
              StatusBadge(m['status']?.toString()),
            ],
          ),
          if (meta != null) ...[
            const SizedBox(height: 8),
            Row(
              children: [
                Icon(meta.$1, size: 14, color: cg.textSecondary),
                const SizedBox(width: 6),
                Eyebrow(meta.$2, color: cg.textSecondary),
                if (m['type'] == 'data' && m['data_port'] != null) ...[
                  const SizedBox(width: 6),
                  MonoChip('port ${m['data_port']}'),
                ],
              ],
            ),
          ],
          if (r.text.isNotEmpty) ...[
            const SizedBox(height: 8),
            Text(
              r.text,
              maxLines: 4,
              overflow: TextOverflow.ellipsis,
              style: TextStyle(fontSize: 13, color: cg.textSecondary),
            ),
          ],
          if (error.isNotEmpty) ...[
            const SizedBox(height: 6),
            Text(error, style: TextStyle(fontSize: 12, color: cg.danger)),
          ],
          const SizedBox(height: 10),
          Row(
            children: [
              if (m['encrypted'] == true) ...[
                const MonoChip('🔒 e2e'),
                const SizedBox(width: 8),
              ],
              Expanded(
                child: Text(
                  fmtTimestamp(m['created_at']),
                  textAlign: TextAlign.right,
                  style: gsmMono(size: 11, color: cg.textMuted),
                ),
              ),
            ],
          ),
        ],
      ),
    );
  }
}
