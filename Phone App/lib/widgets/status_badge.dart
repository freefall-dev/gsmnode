import 'package:flutter/material.dart';

import '../theme.dart';

/// Mono badge with a status dot, per the design system: semantic tints only,
/// color carries state — no emoji. Mirrors `StatusBadge.vue`, so the same
/// vocabulary covers both message statuses and device online/offline.
class StatusBadge extends StatelessWidget {
  const StatusBadge(this.status, {super.key});

  final String? status;

  @override
  Widget build(BuildContext context) {
    final cg = context.cg;
    final (fg, bg) = switch (status) {
      'Processed' => (cg.brandActive, cg.brandTint),
      'Sent' => (cg.warning, cg.warningTint),
      'Delivered' => (cg.success, cg.successTint),
      'Failed' => (cg.danger, cg.dangerTint),
      'online' => (cg.success, cg.successTint),
      'offline' => (cg.textMuted, cg.sunkenBg),
      _ => (cg.textSecondary, cg.sunkenBg), // includes "Pending"
    };

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
      decoration: BoxDecoration(
        color: bg,
        borderRadius: BorderRadius.circular(4),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Container(
            width: 6,
            height: 6,
            decoration: BoxDecoration(color: fg, shape: BoxShape.circle),
          ),
          const SizedBox(width: 6),
          Text(
            status?.isNotEmpty == true ? status! : '—',
            style: gsmMono(size: 11, color: fg, weight: FontWeight.w600),
          ),
        ],
      ),
    );
  }
}
