import 'package:flutter/material.dart';

import '../main.dart';
import '../services/api_client.dart';
import '../theme.dart';
import '../widgets/ui.dart';

/// Place a call on a device and review the call log — the phone's `Call.vue`.
class CallsScreen extends StatefulWidget {
  const CallsScreen({super.key});

  @override
  State<CallsScreen> createState() => _CallsScreenState();
}

class _CallsScreenState extends State<CallsScreen> {
  final _phone = TextEditingController();

  List<Map<String, dynamic>> _devices = const [];
  String _deviceId = '';
  bool _calling = false;
  String? _error;
  String? _result;

  List<Map<String, dynamic>> _calls = const [];
  bool _logLoading = true;
  String? _logError;
  String _filter = 'all'; // all | incoming | outgoing

  @override
  void initState() {
    super.initState();
    _loadDevices();
    _loadLog();
  }

  @override
  void dispose() {
    _phone.dispose();
    super.dispose();
  }

  Future<void> _loadDevices() async {
    try {
      final res = await apiClient.get('/devices');
      if (!mounted) return;
      setState(() => _devices = itemsOf(res));
    } catch (_) {
      // Non-fatal for the form — "Auto (most recent)" still works.
    }
  }

  Future<void> _loadLog() async {
    if (mounted) setState(() => _logLoading = true);
    try {
      final res = await apiClient.get('/calls');
      if (!mounted) return;
      setState(() {
        _calls = itemsOf(res);
        _logError = null;
        _logLoading = false;
      });
    } catch (e) {
      if (!mounted) return;
      setState(() {
        _logError = describeError(e);
        _logLoading = false;
      });
    }
  }

  List<Map<String, dynamic>> get _visibleCalls => _filter == 'all'
      ? _calls
      : _calls.where((c) => c['direction'] == _filter).toList();

  Future<void> _makeCall() async {
    setState(() {
      _error = null;
      _result = null;
    });
    final number = _phone.text.trim();
    if (number.isEmpty) {
      setState(() => _error = 'Enter a phone number to call.');
      return;
    }
    setState(() => _calling = true);
    try {
      final body = <String, dynamic>{'phone_number': number};
      if (_deviceId.isNotEmpty) body['device_id'] = _deviceId;
      final res = await apiClient.post('/calls', body) as Map<String, dynamic>;
      if (!mounted) return;
      setState(() {
        _result = 'Queued — call ${res['id']} (${res['status']})';
        _phone.clear();
      });
    } catch (e) {
      if (!mounted) return;
      setState(() => _error = describeError(e));
    } finally {
      if (mounted) setState(() => _calling = false);
    }
  }

  /// A call that never connected reads as a miss regardless of direction.
  static bool _missed(Map<String, dynamic> c) =>
      c['status'] == 'missed' || c['status'] == 'rejected' || c['status'] == 'failed';

  static String _duration(Object? secs) {
    final s = secs is num ? secs.toInt() : int.tryParse('$secs');
    if (s == null) return '';
    return '${s ~/ 60}:${(s % 60).toString().padLeft(2, '0')}';
  }

  @override
  Widget build(BuildContext context) {
    final cg = context.cg;
    return RefreshIndicator(
      onRefresh: _loadLog,
      child: ListView(
        padding: const EdgeInsets.fromLTRB(16, 18, 16, 32),
        children: [
          const PageHeader(
            title: 'Calls',
            subtitle: 'Place calls and review your device call log',
          ),

          // New call
          GsmCard(
            padding: EdgeInsets.zero,
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Padding(
                  padding: const EdgeInsets.fromLTRB(16, 14, 16, 12),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        'New call',
                        style: TextStyle(
                          fontSize: 15,
                          fontWeight: FontWeight.w600,
                          color: cg.textPrimary,
                        ),
                      ),
                      const SizedBox(height: 2),
                      Text('POST /api/calls',
                          style: gsmMono(size: 11, color: cg.textMuted)),
                    ],
                  ),
                ),
                Divider(height: 1, color: cg.borderSubtle),
                Padding(
                  padding: const EdgeInsets.all(16),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      LabeledField(
                        label: 'Phone number',
                        child: TextField(
                          controller: _phone,
                          keyboardType: TextInputType.phone,
                          style: gsmMono(size: 13, color: cg.textPrimary),
                          decoration:
                              const InputDecoration(hintText: '+15551234567'),
                        ),
                      ),
                      const SizedBox(height: 18),
                      LabeledField(
                        label: 'Device',
                        child: GsmDropdown<String>(
                          value: _deviceId,
                          items: [
                            const DropdownMenuItem(
                                value: '', child: Text('Auto (most recent)')),
                            for (final d in _devices)
                              DropdownMenuItem(
                                value: (d['device_id'] ?? '').toString(),
                                child: Text(
                                  (d['name'] ?? '').toString().isEmpty
                                      ? (d['device_id'] ?? '').toString()
                                      : d['name'].toString(),
                                  overflow: TextOverflow.ellipsis,
                                ),
                              ),
                          ],
                          onChanged: (v) => setState(() => _deviceId = v ?? ''),
                        ),
                      ),
                      if (_error != null) ...[
                        const SizedBox(height: 14),
                        MessageBanner(_error!),
                      ],
                      if (_result != null) ...[
                        const SizedBox(height: 14),
                        MessageBanner(_result!, tone: BannerTone.success),
                      ],
                      const SizedBox(height: 18),
                      SizedBox(
                        height: 48,
                        child: FilledButton.icon(
                          onPressed: _calling ? null : _makeCall,
                          icon: const Icon(Icons.phone, size: 17),
                          label: Text(_calling ? 'Queuing…' : 'Place call'),
                        ),
                      ),
                    ],
                  ),
                ),
              ],
            ),
          ),

          const SizedBox(height: 28),

          // Call log
          Row(
            children: [
              Expanded(
                child: SingleChildScrollView(
                  scrollDirection: Axis.horizontal,
                  child: SegmentedTabs<String>(
                    value: _filter,
                    options: const [
                      ('all', 'All'),
                      ('incoming', 'Incoming'),
                      ('outgoing', 'Outgoing'),
                    ],
                    onChanged: (f) => setState(() => _filter = f),
                  ),
                ),
              ),
              IconButton(
                tooltip: 'Refresh',
                onPressed: _loadLog,
                icon: Icon(Icons.refresh, size: 20, color: cg.textSecondary),
              ),
            ],
          ),
          const SizedBox(height: 12),

          if (_logError != null) ...[
            MessageBanner(_logError!),
            const SizedBox(height: 12),
          ],
          if (_logLoading)
            const Padding(
              padding: EdgeInsets.symmetric(vertical: 32),
              child: Center(child: CircularProgressIndicator()),
            )
          else if (_visibleCalls.isEmpty)
            const EmptyState('No calls logged yet.')
          else
            for (final c in _visibleCalls)
              Padding(
                padding: const EdgeInsets.only(bottom: 8),
                child: _callTile(c),
              ),
        ],
      ),
    );
  }

  Widget _callTile(Map<String, dynamic> c) {
    final cg = context.cg;
    final missed = _missed(c);
    final outgoing = c['direction'] == 'outgoing';
    final (icon, fg, bg) = missed
        ? (Icons.phone_missed, cg.danger, cg.dangerTint)
        : outgoing
            ? (Icons.phone_forwarded, cg.brandActive, cg.brandTint)
            : (Icons.phone_callback, cg.success, cg.successTint);

    return GsmCard(
      padding: const EdgeInsets.all(12),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Container(
            width: 32,
            height: 32,
            decoration: BoxDecoration(
              color: bg,
              borderRadius: BorderRadius.circular(6),
            ),
            child: Icon(icon, size: 17, color: fg),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  (c['phone_number'] ?? '').toString(),
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
                  crossAxisAlignment: WrapCrossAlignment.center,
                  children: [
                    MonoChip((c['direction'] ?? '').toString().toUpperCase()),
                    if ((c['status'] ?? '').toString().isNotEmpty)
                      Text(
                        c['status'].toString(),
                        style: TextStyle(fontSize: 11, color: cg.textMuted),
                      ),
                    if (c['sim_slot'] != null) MonoChip('SIM ${c['sim_slot']}'),
                    if (c['duration'] != null)
                      Text(
                        _duration(c['duration']),
                        style: gsmMono(size: 11, color: cg.textMuted),
                      ),
                  ],
                ),
                const SizedBox(height: 6),
                Text(
                  fmtTimestamp(c['started_at'] ?? c['created_at']),
                  style: gsmMono(size: 11, color: cg.textMuted),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}
