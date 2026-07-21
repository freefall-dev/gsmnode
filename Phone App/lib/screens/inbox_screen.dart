import 'dart:convert';
import 'dart:typed_data';

import 'package:flutter/material.dart';

import '../main.dart';
import '../services/api_client.dart';
import '../theme.dart';
import '../widgets/ui.dart';

/// Incoming SMS, data SMS and MMS received by the devices — the phone's
/// `Inbox.vue`.
class InboxScreen extends StatefulWidget {
  const InboxScreen({super.key});

  @override
  State<InboxScreen> createState() => _InboxScreenState();
}

/// An inbox item plus the plaintext to show for it.
class _Row {
  _Row(this.raw, this.phoneNumber, this.message);

  final Map<String, dynamic> raw;
  final String phoneNumber;
  final String message;

  /// The API Server leaves `type` off plain SMS.
  String get type => (raw['type'] ?? 'sms').toString();
}

class _InboxScreenState extends State<InboxScreen> {
  List<_Row> _rows = const [];
  bool _loading = true;
  String? _error;
  String _tab = 'all';

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    if (mounted) setState(() => _loading = true);
    try {
      final res = await apiClient.get('/inbox');
      final rows = <_Row>[];
      for (final m in itemsOf(res)) {
        final encrypted = m['encrypted'] == true;
        rows.add(_Row(
          m,
          encrypted
              ? await crypto.tryDecrypt(m['phone_number'])
              : (m['phone_number'] ?? '').toString(),
          encrypted
              ? await crypto.tryDecrypt(m['message'])
              : (m['message'] ?? '').toString(),
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

  int _countOf(String tab) =>
      tab == 'all' ? _rows.length : _rows.where((r) => r.type == tab).length;

  List<_Row> get _visible =>
      _tab == 'all' ? _rows : _rows.where((r) => r.type == _tab).toList();

  static IconData _iconFor(String type) => switch (type) {
        'data' => Icons.data_object,
        'mms' => Icons.image_outlined,
        _ => Icons.chat_bubble_outline,
      };

  /// Decodes a base64 data-SMS payload to a readable preview, falling back to
  /// the raw base64 when it isn't valid UTF-8 text.
  static String _dataPreview(Map<String, dynamic> m) {
    final payload = (m['data_payload'] ?? '').toString();
    if (payload.isEmpty) return '';
    try {
      return utf8.decode(base64.decode(payload));
    } catch (_) {
      return payload;
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
          PageHeader(
            title: 'Inbox',
            subtitle: 'Incoming messages received by your devices',
            actions: [
              IconButton(
                tooltip: 'Refresh',
                onPressed: _load,
                icon: Icon(Icons.refresh, size: 20, color: cg.textSecondary),
              ),
            ],
          ),
          FilterChipsRow<String>(
            value: _tab,
            onChanged: (t) => setState(() => _tab = t),
            options: [
              FilterChipOption('all', 'All',
                  icon: Icons.chat_bubble_outline, count: _countOf('all')),
              FilterChipOption('sms', 'SMS',
                  icon: Icons.chat_bubble_outline, count: _countOf('sms')),
              FilterChipOption('data', 'Data SMS',
                  icon: Icons.data_object, count: _countOf('data')),
              FilterChipOption('mms', 'MMS',
                  icon: Icons.image_outlined, count: _countOf('mms')),
            ],
          ),
          const SizedBox(height: 16),
          if (_error != null) ...[
            MessageBanner(_error!),
            const SizedBox(height: 12),
          ],
          if (_loading)
            const Padding(
              padding: EdgeInsets.symmetric(vertical: 40),
              child: Center(child: CircularProgressIndicator()),
            )
          else if (_visible.isEmpty)
            EmptyState('No ${_tab == 'all' ? '' : '$_tab '}messages yet.')
          else
            for (final r in _visible)
              Padding(
                padding: const EdgeInsets.only(bottom: 10),
                child: _inboxCard(r),
              ),
        ],
      ),
    );
  }

  Widget _inboxCard(_Row r) {
    final cg = context.cg;
    final m = r.raw;
    final subject = (m['subject'] ?? '').toString();
    final attachments = (m['attachments'] as List?)
            ?.whereType<Map>()
            .map((e) => e.cast<String, dynamic>())
            .toList() ??
        const <Map<String, dynamic>>[];

    return GsmCard(
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Container(
            width: 32,
            height: 32,
            decoration: BoxDecoration(
              color: cg.successTint,
              borderRadius: BorderRadius.circular(6),
            ),
            child: Icon(_iconFor(r.type), size: 17, color: cg.success),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  r.phoneNumber,
                  style: gsmMono(
                    size: 13,
                    color: cg.textPrimary,
                    weight: FontWeight.w600,
                  ),
                ),
                const SizedBox(height: 6),
                Wrap(
                  spacing: 6,
                  runSpacing: 6,
                  children: [
                    if (r.type != 'sms')
                      MonoChip(
                        r.type.toUpperCase(),
                        color: cg.brandActive,
                        background: cg.brandTint,
                      ),
                    if (m['encrypted'] == true) const MonoChip('🔒 e2e'),
                    if (m['sim_slot'] != null) MonoChip('SIM ${m['sim_slot']}'),
                    MonoChip(fmtTimestamp(m['received_at'])),
                  ],
                ),
                if (r.message.isNotEmpty) ...[
                  const SizedBox(height: 8),
                  Text(
                    r.message,
                    style: TextStyle(fontSize: 13, color: cg.textSecondary),
                  ),
                ],
                if (subject.isNotEmpty) ...[
                  const SizedBox(height: 6),
                  Text(
                    'Subject: $subject',
                    style: TextStyle(
                      fontSize: 12,
                      fontWeight: FontWeight.w600,
                      color: cg.textPrimary,
                    ),
                  ),
                ],
                if (attachments.isNotEmpty) ...[
                  const SizedBox(height: 10),
                  for (final a in attachments) _attachment(a),
                ],
                if (r.type == 'data') ...[
                  const SizedBox(height: 8),
                  Row(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      if (m['data_port'] != null) ...[
                        MonoChip('port ${m['data_port']}'),
                        const SizedBox(width: 8),
                      ],
                      Expanded(
                        child: Container(
                          padding: const EdgeInsets.symmetric(
                              horizontal: 8, vertical: 6),
                          decoration: BoxDecoration(
                            color: cg.sunkenBg,
                            borderRadius: BorderRadius.circular(6),
                          ),
                          child: Text(
                            _dataPreview(m),
                            style: gsmMono(size: 11, color: cg.textSecondary),
                          ),
                        ),
                      ),
                    ],
                  ),
                ],
              ],
            ),
          ),
        ],
      ),
    );
  }

  /// The browser can hand an attachment straight to a download; a phone can't,
  /// so an inline image is rendered where possible and everything else is named.
  Widget _attachment(Map<String, dynamic> a) {
    final cg = context.cg;
    final contentType = (a['content_type'] ?? '').toString();
    final data = (a['data'] ?? '').toString();
    final filename = (a['filename'] ?? '').toString();

    if (contentType.startsWith('image/') && data.isNotEmpty) {
      Uint8List? bytes;
      try {
        bytes = base64.decode(data);
      } catch (_) {
        bytes = null; // a truncated payload shouldn't blank the whole card
      }
      if (bytes != null) {
        return Padding(
          padding: const EdgeInsets.only(bottom: 8),
          child: ClipRRect(
            borderRadius: BorderRadius.circular(8),
            child: Image.memory(
              bytes,
              fit: BoxFit.cover,
              errorBuilder: (_, __, ___) => MonoChip(
                filename.isEmpty ? contentType : filename,
              ),
            ),
          ),
        );
      }
    }

    return Padding(
      padding: const EdgeInsets.only(bottom: 6),
      child: Row(
        children: [
          Icon(Icons.attachment, size: 14, color: cg.textMuted),
          const SizedBox(width: 6),
          Flexible(
            child: Text(
              filename.isNotEmpty
                  ? filename
                  : contentType.isNotEmpty
                      ? contentType
                      : 'attachment',
              overflow: TextOverflow.ellipsis,
              style: gsmMono(size: 11, color: cg.textSecondary),
            ),
          ),
        ],
      ),
    );
  }
}
