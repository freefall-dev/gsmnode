import 'dart:async';

import 'package:flutter/material.dart';

import '../config.dart';
import '../main.dart';
import '../services/api_client.dart';
import '../theme.dart';
import '../widgets/status_badge.dart';
import '../widgets/ui.dart';

/// Phones registered against the gateway, mirroring `Devices.vue`. The Web App's
/// table becomes a card per device — the columns are the same fields.
class DevicesScreen extends StatefulWidget {
  const DevicesScreen({super.key});

  @override
  State<DevicesScreen> createState() => _DevicesScreenState();
}

class _DevicesScreenState extends State<DevicesScreen> {
  List<Map<String, dynamic>> _devices = const [];
  bool _loading = true;
  String? _error;
  Timer? _timer;

  @override
  void initState() {
    super.initState();
    _load();
    // Online/offline is derived from a heartbeat, so it changes without anyone
    // touching the screen. Poll, or the list keeps insisting a dead phone is
    // online until someone pulls to refresh.
    _timer = Timer.periodic(
      AppConfig.devicePollInterval,
      (_) => _load(quiet: true),
    );
  }

  @override
  void dispose() {
    _timer?.cancel();
    super.dispose();
  }

  /// [quiet] refreshes without flashing the spinner — used by the poller so a
  /// background refresh doesn't make the list blink every 10s.
  Future<void> _load({bool quiet = false}) async {
    if (!quiet && mounted) setState(() => _loading = true);
    try {
      final res = await apiClient.get('/devices');
      if (!mounted) return;
      setState(() {
        _devices = itemsOf(res);
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

  Future<void> _remove(Map<String, dynamic> d) async {
    final label = (d['name'] ?? d['device_id'] ?? '').toString();
    final ok = await confirmDialog(
      context,
      title: 'Remove device',
      message: 'Remove device "$label"?',
      confirmLabel: 'Remove',
    );
    if (!ok) return;
    try {
      await apiClient.delete('/devices/${d['id']}');
      if (!mounted) return;
      setState(() => _devices =
          _devices.where((x) => x['id'] != d['id']).toList(growable: false));
    } catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text('Could not remove device: ${describeError(e)}')),
      );
    }
  }

  static String _simLabel(Map<String, dynamic> sim) {
    final parts = ['Slot ${sim['slot']}'];
    final name = (sim['carrier'] ?? sim['display_name'] ?? '').toString();
    if (name.isNotEmpty) parts.add(name);
    final number = (sim['number'] ?? '').toString();
    if (number.isNotEmpty) parts.add(number);
    return parts.join(' · ');
  }

  @override
  Widget build(BuildContext context) {
    return RefreshIndicator(
      onRefresh: _load,
      child: ListView(
        padding: const EdgeInsets.fromLTRB(16, 18, 16, 32),
        children: [
          const PageHeader(
            title: 'Devices',
            subtitle: 'Phones connected to your gateway',
          ),
          if (_error != null) ...[
            MessageBanner(_error!),
            const SizedBox(height: 14),
          ],
          if (_loading)
            const Padding(
              padding: EdgeInsets.symmetric(vertical: 40),
              child: Center(child: CircularProgressIndicator()),
            )
          else if (_devices.isEmpty)
            const EmptyState(
              'No devices yet. Register one from the Phone Agent.',
            )
          else
            for (final d in _devices)
              Padding(
                padding: const EdgeInsets.only(bottom: 10),
                child: _deviceCard(d),
              ),
        ],
      ),
    );
  }

  Widget _deviceCard(Map<String, dynamic> d) {
    final cg = context.cg;
    final sims = (d['sims'] as List?)
            ?.whereType<Map>()
            .map((e) => e.cast<String, dynamic>())
            .toList() ??
        const <Map<String, dynamic>>[];
    final platform =
        [d['platform'], d['app_version']].where((v) => v != null && '$v'.isNotEmpty).join(' ');

    return GsmCard(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Expanded(
                child: Text(
                  (d['name'] ?? '').toString().isEmpty
                      ? '—'
                      : d['name'].toString(),
                  style: TextStyle(
                    fontSize: 15,
                    fontWeight: FontWeight.w600,
                    color: cg.textPrimary,
                  ),
                ),
              ),
              StatusBadge(d['status']?.toString()),
            ],
          ),
          const SizedBox(height: 6),
          Text(
            (d['device_id'] ?? '').toString(),
            style: gsmMono(size: 11, color: cg.textSecondary),
          ),
          if (platform.isNotEmpty) ...[
            const SizedBox(height: 4),
            Text(platform, style: TextStyle(fontSize: 12, color: cg.textSecondary)),
          ],
          if (sims.isNotEmpty) ...[
            const SizedBox(height: 10),
            Wrap(
              spacing: 6,
              runSpacing: 6,
              children: [for (final s in sims) MonoChip(_simLabel(s))],
            ),
          ],
          const SizedBox(height: 12),
          Row(
            children: [
              Expanded(
                child: Text(
                  'Last seen ${fmtTimestamp(d['last_seen_at'])}',
                  style: gsmMono(size: 11, color: cg.textMuted),
                ),
              ),
              TextButton(
                onPressed: () => _remove(d),
                style: TextButton.styleFrom(
                  foregroundColor: cg.danger,
                  padding: const EdgeInsets.symmetric(horizontal: 8),
                  minimumSize: Size.zero,
                  tapTargetSize: MaterialTapTargetSize.shrinkWrap,
                ),
                child: const Text('Remove'),
              ),
            ],
          ),
        ],
      ),
    );
  }
}
