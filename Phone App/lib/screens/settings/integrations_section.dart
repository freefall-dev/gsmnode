import 'package:flutter/material.dart';

import '../../main.dart';
import '../../services/api_client.dart';
import '../../theme.dart';
import '../../widgets/ui.dart';
import 'integration_card.dart';

/// Every plugin that offers per-user settings, fetched already resolved through
/// the server's cascade — a port of `IntegrationsSection.vue`. One request
/// renders the whole section; the server decides which plugins the caller may
/// configure, so adding a plugin needs no change here.
class IntegrationsSection extends StatefulWidget {
  const IntegrationsSection({super.key});

  @override
  State<IntegrationsSection> createState() => _IntegrationsSectionState();
}

class _IntegrationsSectionState extends State<IntegrationsSection> {
  List<Map<String, dynamic>> _integrations = const [];
  bool _loaded = false;
  String? _error;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    try {
      final res = await apiClient.get('/integrations');
      if (!mounted) return;
      setState(() => _integrations = itemsOf(res, 'integrations'));
    } catch (e) {
      if (!mounted) return;
      setState(() => _error = describeError(e));
    } finally {
      if (mounted) setState(() => _loaded = true);
    }
  }

  @override
  Widget build(BuildContext context) {
    final cg = context.cg;

    if (!_loaded) {
      return const Padding(
        padding: EdgeInsets.symmetric(vertical: 32),
        child: Center(child: CircularProgressIndicator()),
      );
    }
    if (_error != null) {
      return GsmCard(child: MessageBanner(_error!));
    }
    if (_integrations.isEmpty) {
      return GsmCard(
        child: Text(
          'No integrations are available. A superadmin enables them in the API '
          'Server\'s Plugins panel.',
          style: TextStyle(fontSize: 13, color: cg.textMuted),
        ),
      );
    }

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        for (final i in _integrations)
          IntegrationCard(key: ValueKey(i['name']), integration: i),
      ],
    );
  }
}
