import 'dart:async';

import 'package:flutter/material.dart';

import '../config.dart';
import '../main.dart';
import '../theme.dart';

/// Polls the API Server's public `/api/health` endpoint and reflects
/// reachability as a coloured dot, mirroring the Web App's `ApiStatus.vue`.
class ApiStatusIndicator extends StatefulWidget {
  const ApiStatusIndicator({super.key});

  @override
  State<ApiStatusIndicator> createState() => _ApiStatusIndicatorState();
}

enum _Health { checking, online, offline }

class _ApiStatusIndicatorState extends State<ApiStatusIndicator> {
  _Health _status = _Health.checking;
  int? _latencyMs;
  Timer? _timer;

  @override
  void initState() {
    super.initState();
    _check();
    _timer = Timer.periodic(AppConfig.healthInterval, (_) => _check());
  }

  @override
  void dispose() {
    _timer?.cancel();
    super.dispose();
  }

  Future<void> _check() async {
    final started = DateTime.now();
    try {
      await apiClient.get('/health');
      if (!mounted) return;
      setState(() {
        _latencyMs = DateTime.now().difference(started).inMilliseconds;
        _status = _Health.online;
      });
    } catch (_) {
      if (!mounted) return;
      setState(() {
        _latencyMs = null;
        _status = _Health.offline;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    final cg = context.cg;
    final (dot, label) = switch (_status) {
      _Health.checking => (cg.borderStrong, 'Checking…'),
      _Health.online => (cg.success, 'API online'),
      _Health.offline => (cg.danger, 'API offline'),
    };

    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Container(
          width: 8,
          height: 8,
          decoration: BoxDecoration(color: dot, shape: BoxShape.circle),
        ),
        const SizedBox(width: 8),
        Flexible(
          child: Text(
            _latencyMs != null ? '$label · ${_latencyMs}ms' : label,
            overflow: TextOverflow.ellipsis,
            style: gsmMono(
              size: 11,
              color: _status == _Health.offline ? cg.danger : cg.textSecondary,
            ),
          ),
        ),
      ],
    );
  }
}
