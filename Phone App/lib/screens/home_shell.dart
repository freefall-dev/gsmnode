import 'package:flutter/material.dart';

import '../main.dart';
import '../theme.dart';
import '../widgets/api_status.dart';
import '../widgets/gsmnode_mark.dart';
import '../widgets/ui.dart';
import 'calls_screen.dart';
import 'devices_screen.dart';
import 'inbox_screen.dart';
import 'messages_screen.dart';
import 'send_screen.dart';
import 'settings_screen.dart';
import 'webhooks_screen.dart';

/// The signed-in shell. The Web App has room for a permanent sidebar; a phone
/// does not, so the same navigation lives in a drawer — the order, grouping and
/// the API-status footer are carried over unchanged.
class HomeShell extends StatefulWidget {
  const HomeShell({super.key});

  @override
  State<HomeShell> createState() => _HomeShellState();
}

class _NavItem {
  const _NavItem(this.label, this.icon, this.build);

  final String label;
  final IconData icon;
  final Widget Function() build;
}

class _HomeShellState extends State<HomeShell> {
  // Gateway destinations, matching the Web App sidebar's order.
  static final _gateway = <_NavItem>[
    _NavItem('Devices', Icons.smartphone_outlined, DevicesScreen.new),
    _NavItem('Send SMS', Icons.send_outlined, SendScreen.new),
    _NavItem('Calls', Icons.phone_outlined, CallsScreen.new),
    _NavItem('Messages', Icons.chat_bubble_outline, MessagesScreen.new),
    _NavItem('Inbox', Icons.inbox_outlined, InboxScreen.new),
    _NavItem('Webhooks', Icons.webhook_outlined, WebhooksScreen.new),
  ];

  static final _settings =
      _NavItem('Settings', Icons.settings_outlined, SettingsScreen.new);

  /// Index into [_gateway], or `-1` for Settings (which sits below the divider
  /// in its own group, exactly as in the Web App).
  int _index = 0;

  _NavItem get _current => _index < 0 ? _settings : _gateway[_index];

  void _select(int index) {
    Navigator.of(context).pop(); // close the drawer
    if (index != _index) setState(() => _index = index);
  }

  Future<void> _signOut() async {
    await auth.logout();
  }

  @override
  Widget build(BuildContext context) {
    final cg = context.cg;
    final isDark = themeController.isDark(context);

    return Scaffold(
      appBar: AppBar(
        title: Text(_current.label),
        actions: [
          IconButton(
            tooltip: isDark ? 'Switch to light theme' : 'Switch to dark theme',
            icon: Icon(isDark ? Icons.light_mode_outlined : Icons.dark_mode_outlined),
            onPressed: () => themeController.toggle(context),
          ),
          IconButton(
            tooltip: 'Sign out',
            icon: const Icon(Icons.logout),
            onPressed: _signOut,
          ),
          const SizedBox(width: 4),
        ],
      ),
      drawer: _buildDrawer(cg),
      body: SafeArea(
        // Keyed so switching destinations remounts the page and it re-fetches,
        // the way navigating a route in the SPA does.
        child: KeyedSubtree(
          key: ValueKey(_index),
          child: _current.build(),
        ),
      ),
    );
  }

  Widget _buildDrawer(GsmSemantic cg) {
    return Drawer(
      child: SafeArea(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 20, 20, 6),
              child: Row(
                children: [
                  const GsmNodeMark(size: 26),
                  const SizedBox(width: 10),
                  const GsmNodeWordmark(size: 19),
                ],
              ),
            ),
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 10, 20, 6),
              child: const Eyebrow('Gateway'),
            ),
            Expanded(
              child: ListView(
                padding: const EdgeInsets.symmetric(horizontal: 10),
                children: [
                  for (var i = 0; i < _gateway.length; i++)
                    _navTile(cg, _gateway[i], selected: _index == i, onTap: () => _select(i)),
                  Padding(
                    padding: const EdgeInsets.symmetric(vertical: 8, horizontal: 8),
                    child: Divider(height: 1, color: cg.borderSubtle),
                  ),
                  _navTile(cg, _settings,
                      selected: _index < 0, onTap: () => _select(-1)),
                ],
              ),
            ),
            Divider(height: 1, color: cg.borderSubtle),
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 14, 20, 14),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  if (auth.user != null) ...[
                    Text(
                      auth.user!.email,
                      overflow: TextOverflow.ellipsis,
                      style: gsmMono(size: 11, color: cg.textSecondary),
                    ),
                    const SizedBox(height: 8),
                  ],
                  const ApiStatusIndicator(),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _navTile(
    GsmSemantic cg,
    _NavItem item, {
    required bool selected,
    required VoidCallback onTap,
  }) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 1),
      child: InkWell(
        borderRadius: BorderRadius.circular(8),
        onTap: onTap,
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 11),
          decoration: BoxDecoration(
            color: selected ? cg.brandTint : Colors.transparent,
            borderRadius: BorderRadius.circular(8),
          ),
          child: Row(
            children: [
              Icon(
                item.icon,
                size: 19,
                color: selected ? cg.brandActive : cg.textSecondary,
              ),
              const SizedBox(width: 12),
              Text(
                item.label,
                style: TextStyle(
                  fontSize: 14,
                  fontWeight: FontWeight.w600,
                  color: selected ? cg.brandActive : cg.textSecondary,
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
