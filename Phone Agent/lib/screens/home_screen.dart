import 'package:flutter/material.dart';
import 'package:permission_handler/permission_handler.dart';

import '../main.dart';
import '../theme.dart';
import '../widgets/gsmnode_mark.dart';
import 'login_screen.dart';

class HomeScreen extends StatefulWidget {
  const HomeScreen({super.key});

  @override
  State<HomeScreen> createState() => _HomeScreenState();
}

class _HomeScreenState extends State<HomeScreen> {
  bool _permsGranted = false;

  @override
  void initState() {
    super.initState();
    gateway.addListener(_onChange);
    _checkPermissions();
  }

  @override
  void dispose() {
    gateway.removeListener(_onChange);
    super.dispose();
  }

  void _onChange() => setState(() {});

  Future<void> _checkPermissions() async {
    final sms = await Permission.sms.status;
    final phone = await Permission.phone.status;
    setState(() => _permsGranted = sms.isGranted && phone.isGranted);
  }

  Future<void> _requestPermissions() async {
    // SMS + phone gate the gateway; notification lets the foreground service
    // post its ongoing notification on Android 13+.
    await [Permission.sms, Permission.phone, Permission.notification].request();
    await _checkPermissions();
  }

  void _toggle() {
    if (gateway.running) {
      gateway.stop();
    } else {
      gateway.start();
    }
  }

  Future<void> _logout() async {
    gateway.stop();
    await storage.clearSession();
    if (!mounted) return;
    Navigator.of(context).pushReplacement(
      MaterialPageRoute(builder: (_) => const LoginScreen()),
    );
  }

  @override
  Widget build(BuildContext context) {
    final running = gateway.running;
    final cg = context.cg;
    return Scaffold(
      appBar: AppBar(
        title: const Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            GsmNodeMark(size: 22),
            SizedBox(width: 8),
            GsmNodeWordmark(size: 17),
          ],
        ),
        actions: [
          IconButton(
            onPressed: _logout,
            icon: const Icon(Icons.logout),
            tooltip: 'Sign out',
          ),
        ],
      ),
      body: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            _heroCard(running),
            const SizedBox(height: 12),
            if (!_permsGranted) ...[
              _permissionCard(),
              const SizedBox(height: 12),
            ],
            _infoCard(),
            const SizedBox(height: 16),
            Text(
              'ACTIVITY',
              style: gsmMono(size: 10, color: cg.textMuted, letterSpacing: 1.4),
            ),
            const SizedBox(height: 8),
            Expanded(child: _activityLog()),
          ],
        ),
      ),
    );
  }

  /// Hero per the mobile UI kit: brand ring + mark when routing, ink when
  /// paused. The primary action carries the brand glow.
  Widget _heroCard(bool running) {
    final cg = context.cg;
    return Container(
      padding: const EdgeInsets.all(24),
      decoration: BoxDecoration(
        color: running ? cg.brandTint : cg.sunkenBg,
        borderRadius: BorderRadius.circular(20),
        border: Border.all(
          color: running
              ? GsmColors.green500.withValues(alpha: 0.22)
              : cg.borderSubtle,
        ),
      ),
      child: Column(
        children: [
          Container(
            width: 84,
            height: 84,
            decoration: BoxDecoration(
              shape: BoxShape.circle,
              color: running ? GsmColors.green500 : cg.textMuted,
              boxShadow: running
                  ? [
                      BoxShadow(
                        color: GsmColors.green500.withValues(alpha: 0.28),
                        blurRadius: 18,
                        offset: const Offset(0, 6),
                      ),
                    ]
                  : null,
            ),
            child: const Center(
              child: GsmNodeMark(size: 44, color: Colors.white),
            ),
          ),
          const SizedBox(height: 14),
          Text(
            running ? 'Gateway active' : 'Gateway paused',
            style: gsmDisplay(size: 20, color: cg.textPrimary),
          ),
          const SizedBox(height: 4),
          Text(
            running
                ? 'routing sms · mms · calls'
                : 'not routing — messages will queue',
            style: gsmMono(
              size: 12,
              color: running ? GsmColors.green500 : cg.textMuted,
            ),
          ),
          const SizedBox(height: 18),
          FilledButton.icon(
            onPressed: _permsGranted ? _toggle : null,
            icon: Icon(running ? Icons.stop : Icons.play_arrow),
            label: Text(running ? 'Stop gateway' : 'Start gateway'),
            style: running
                ? FilledButton.styleFrom(
                    backgroundColor: cg.danger,
                    minimumSize: const Size.fromHeight(48),
                  )
                : null,
          ),
        ],
      ),
    );
  }

  Widget _permissionCard() {
    final cg = context.cg;
    return Container(
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(
        color: cg.warningTint,
        borderRadius: BorderRadius.circular(14),
      ),
      child: Row(
        children: [
          Icon(Icons.warning_amber_rounded, color: cg.warning),
          const SizedBox(width: 12),
          const Expanded(
            child: Text('SMS & phone permissions are required to send and '
                'receive messages.'),
          ),
          TextButton(
            onPressed: _requestPermissions,
            child: const Text('Grant'),
          ),
        ],
      ),
    );
  }

  Widget _infoCard() {
    final cg = context.cg;
    return Card(
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
        child: Column(
          children: [
            _infoRow('DEVICE', storage.deviceName ?? '—'),
            Divider(height: 16, color: cg.borderSubtle),
            _infoRow('ACCOUNT', storage.userEmail ?? '—'),
            Divider(height: 16, color: cg.borderSubtle),
            _infoRow('SERVER', storage.apiBase ?? '—'),
          ],
        ),
      ),
    );
  }

  Widget _infoRow(String label, String value) {
    final cg = context.cg;
    return Row(
      children: [
        SizedBox(
          width: 76,
          child: Text(
            label,
            style: gsmMono(size: 9, color: cg.textMuted, letterSpacing: 1.3),
          ),
        ),
        Expanded(
          child: Text(
            value,
            style: gsmMono(size: 12, color: cg.textPrimary),
            overflow: TextOverflow.ellipsis,
            textAlign: TextAlign.right,
          ),
        ),
      ],
    );
  }

  Widget _activityLog() {
    final cg = context.cg;
    final entries = gateway.log;
    if (entries.isEmpty) {
      return Center(
        child: Text(
          'No activity yet.',
          style: TextStyle(color: cg.textMuted),
        ),
      );
    }
    return ListView.separated(
      itemCount: entries.length,
      separatorBuilder: (_, __) => const SizedBox(height: 6),
      itemBuilder: (_, i) {
        final e = entries[i];
        final t = e.time;
        final hh = t.hour.toString().padLeft(2, '0');
        final mm = t.minute.toString().padLeft(2, '0');
        final ss = t.second.toString().padLeft(2, '0');
        return Container(
          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
          decoration: BoxDecoration(
            color: cg.card,
            borderRadius: BorderRadius.circular(10),
            border: Border.all(color: cg.borderSubtle),
          ),
          child: Row(
            children: [
              Container(
                width: 8,
                height: 8,
                decoration: BoxDecoration(
                  shape: BoxShape.circle,
                  color: e.error ? cg.danger : cg.success,
                ),
              ),
              const SizedBox(width: 10),
              Expanded(
                child: Text(e.text, style: const TextStyle(fontSize: 13)),
              ),
              const SizedBox(width: 10),
              Text(
                '$hh:$mm:$ss',
                style: gsmMono(size: 10, color: cg.textMuted),
              ),
            ],
          ),
        );
      },
    );
  }
}
