import 'package:flutter/material.dart';
import 'package:intl/intl.dart';

import '../services/api_client.dart';
import '../theme.dart';

/// The small caps mono label the Web App calls `.gn-eyebrow`.
class Eyebrow extends StatelessWidget {
  const Eyebrow(this.text, {super.key, this.color});

  final String text;
  final Color? color;

  @override
  Widget build(BuildContext context) => Text(
        text.toUpperCase(),
        style: gsmEyebrow(context, color: color),
      );
}

/// Title + subtitle + optional trailing actions, matching `PageHeader.vue`.
class PageHeader extends StatelessWidget {
  const PageHeader({
    super.key,
    required this.title,
    this.subtitle,
    this.actions = const [],
  });

  final String title;
  final String? subtitle;
  final List<Widget> actions;

  @override
  Widget build(BuildContext context) {
    final cg = context.cg;
    return Padding(
      padding: const EdgeInsets.only(bottom: 18),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(title, style: gsmDisplay(size: 22, color: cg.textPrimary)),
                if (subtitle != null) ...[
                  const SizedBox(height: 3),
                  Text(
                    subtitle!,
                    style: TextStyle(fontSize: 13, color: cg.textSecondary),
                  ),
                ],
              ],
            ),
          ),
          if (actions.isNotEmpty) ...[
            const SizedBox(width: 12),
            Wrap(spacing: 8, crossAxisAlignment: WrapCrossAlignment.center, children: actions),
          ],
        ],
      ),
    );
  }
}

/// A bordered surface panel — the Web App's `rounded-lg border bg-card`.
class GsmCard extends StatelessWidget {
  const GsmCard({
    super.key,
    required this.child,
    this.padding = const EdgeInsets.all(16),
    this.color,
  });

  final Widget child;
  final EdgeInsetsGeometry padding;
  final Color? color;

  @override
  Widget build(BuildContext context) {
    final cg = context.cg;
    return Container(
      width: double.infinity,
      padding: padding,
      decoration: BoxDecoration(
        color: color ?? cg.card,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: cg.borderSubtle),
      ),
      child: child,
    );
  }
}

/// A titled panel with the eyebrow heading the Settings sections use.
class SectionCard extends StatelessWidget {
  const SectionCard({super.key, required this.title, required this.child});

  final String title;
  final Widget child;

  @override
  Widget build(BuildContext context) => GsmCard(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Eyebrow(title),
            const SizedBox(height: 14),
            child,
          ],
        ),
      );
}

enum BannerTone { error, success, warning, info }

/// The inline tinted message strip used for form errors and confirmations.
class MessageBanner extends StatelessWidget {
  const MessageBanner(this.text, {super.key, this.tone = BannerTone.error});

  final String text;
  final BannerTone tone;

  @override
  Widget build(BuildContext context) {
    final cg = context.cg;
    final (fg, bg) = switch (tone) {
      BannerTone.error => (cg.danger, cg.dangerTint),
      BannerTone.success => (cg.success, cg.successTint),
      BannerTone.warning => (cg.warning, cg.warningTint),
      BannerTone.info => (cg.textSecondary, cg.sunkenBg),
    };
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 9),
      decoration: BoxDecoration(
        color: bg,
        borderRadius: BorderRadius.circular(8),
      ),
      child: Text(text, style: TextStyle(fontSize: 13, color: fg)),
    );
  }
}

/// A small tinted mono pill — SIM slots, ports, `🔒 e2e`, role badges.
class MonoChip extends StatelessWidget {
  const MonoChip(this.text, {super.key, this.color, this.background});

  final String text;
  final Color? color;
  final Color? background;

  @override
  Widget build(BuildContext context) {
    final cg = context.cg;
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      decoration: BoxDecoration(
        color: background ?? cg.sunkenBg,
        borderRadius: BorderRadius.circular(4),
      ),
      child: Text(text, style: gsmMono(size: 10, color: color ?? cg.textMuted)),
    );
  }
}

/// The dashed "nothing here yet" panel.
class EmptyState extends StatelessWidget {
  const EmptyState(this.text, {super.key});

  final String text;

  @override
  Widget build(BuildContext context) {
    final cg = context.cg;
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.symmetric(vertical: 40, horizontal: 20),
      decoration: BoxDecoration(
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: cg.borderStrong, style: BorderStyle.solid),
      ),
      child: Text(
        text,
        textAlign: TextAlign.center,
        style: TextStyle(fontSize: 13, color: cg.textMuted),
      ),
    );
  }
}

/// The inset pill group used to switch message kind / call direction.
class SegmentedTabs<T> extends StatelessWidget {
  const SegmentedTabs({
    super.key,
    required this.value,
    required this.options,
    required this.onChanged,
  });

  final T value;
  final List<(T, String)> options;
  final ValueChanged<T> onChanged;

  @override
  Widget build(BuildContext context) {
    final cg = context.cg;
    return Container(
      padding: const EdgeInsets.all(2),
      decoration: BoxDecoration(
        color: cg.sunkenBg,
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: cg.borderSubtle),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          for (final (id, label) in options)
            GestureDetector(
              onTap: () => onChanged(id),
              behavior: HitTestBehavior.opaque,
              child: Container(
                padding:
                    const EdgeInsets.symmetric(horizontal: 14, vertical: 7),
                decoration: BoxDecoration(
                  color: id == value ? cg.card : Colors.transparent,
                  borderRadius: BorderRadius.circular(6),
                ),
                child: Text(
                  label,
                  style: TextStyle(
                    fontSize: 13,
                    fontWeight: FontWeight.w600,
                    color: id == value ? cg.textPrimary : cg.textMuted,
                  ),
                ),
              ),
            ),
        ],
      ),
    );
  }
}

/// The outlined chip row used for Inbox type filters and the Settings tabs,
/// optionally carrying a count badge.
class FilterChipsRow<T> extends StatelessWidget {
  const FilterChipsRow({
    super.key,
    required this.value,
    required this.options,
    required this.onChanged,
  });

  final T value;
  final List<FilterChipOption<T>> options;
  final ValueChanged<T> onChanged;

  @override
  Widget build(BuildContext context) {
    final cg = context.cg;
    return Wrap(
      spacing: 8,
      runSpacing: 8,
      children: [
        for (final o in options)
          GestureDetector(
            onTap: () => onChanged(o.id),
            behavior: HitTestBehavior.opaque,
            child: Container(
              padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 7),
              decoration: BoxDecoration(
                color: o.id == value ? cg.brandTint : cg.card,
                borderRadius: BorderRadius.circular(8),
                border: Border.all(
                  color: o.id == value ? cg.brand : cg.borderSubtle,
                ),
              ),
              child: Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  if (o.icon != null) ...[
                    Icon(
                      o.icon,
                      size: 14,
                      color: o.id == value ? cg.brandActive : cg.textSecondary,
                    ),
                    const SizedBox(width: 6),
                  ],
                  Text(
                    o.label,
                    style: TextStyle(
                      fontSize: 13,
                      fontWeight: FontWeight.w600,
                      color: o.id == value ? cg.brandActive : cg.textSecondary,
                    ),
                  ),
                  if (o.count != null) ...[
                    const SizedBox(width: 6),
                    MonoChip('${o.count}'),
                  ],
                ],
              ),
            ),
          ),
      ],
    );
  }
}

class FilterChipOption<T> {
  const FilterChipOption(this.id, this.label, {this.icon, this.count});

  final T id;
  final String label;
  final IconData? icon;
  final int? count;
}

/// A form field with the small bold label the Web App puts above its inputs.
class LabeledField extends StatelessWidget {
  const LabeledField({
    super.key,
    required this.label,
    required this.child,
    this.help,
  });

  final String label;
  final Widget child;
  final String? help;

  @override
  Widget build(BuildContext context) {
    final cg = context.cg;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          label,
          style: TextStyle(
            fontSize: 13,
            fontWeight: FontWeight.w600,
            color: cg.textPrimary,
          ),
        ),
        const SizedBox(height: 6),
        child,
        if (help != null) ...[
          const SizedBox(height: 5),
          Text(help!, style: TextStyle(fontSize: 11, color: cg.textMuted)),
        ],
      ],
    );
  }
}

/// A fully-controlled select, styled like the app's text fields.
///
/// Deliberately not `DropdownButtonFormField`: that one seeds itself from an
/// *initial* value, and every select here is driven by state that changes after
/// the first build (picking a device rewrites the SIM options, switching an
/// integration scope reseeds the form).
class GsmDropdown<T> extends StatelessWidget {
  const GsmDropdown({
    super.key,
    required this.value,
    required this.items,
    required this.onChanged,
  });

  final T value;
  final List<DropdownMenuItem<T>> items;
  final ValueChanged<T?>? onChanged;

  @override
  Widget build(BuildContext context) {
    final cg = context.cg;
    return InputDecorator(
      decoration: InputDecoration(
        contentPadding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
        enabled: onChanged != null,
      ),
      child: DropdownButtonHideUnderline(
        child: DropdownButton<T>(
          value: value,
          items: items,
          onChanged: onChanged,
          isExpanded: true,
          isDense: true,
          dropdownColor: cg.card,
          style: TextStyle(fontSize: 14, color: cg.textPrimary),
          icon: Icon(Icons.expand_more, size: 20, color: cg.textMuted),
        ),
      ),
    );
  }
}

/// Timestamps, standing in for the Web App's `toLocaleString()`. Renders an
/// em dash for a missing value so table cells stay aligned.
String fmtTimestamp(Object? ts) {
  final raw = ts?.toString() ?? '';
  if (raw.isEmpty) return '—';
  final parsed = DateTime.tryParse(raw);
  if (parsed == null) return raw;
  return DateFormat.yMd().add_jm().format(parsed.toLocal());
}

/// Human-facing text for a failed request. Transport failures get the "check the
/// server URL" hint the Web App shows, since on a phone that is nearly always
/// what went wrong.
String describeError(Object e) {
  if (e is ApiException) {
    if (e.unreachable) {
      return 'Cannot reach the API Server. Check the server URL in Settings.';
    }
    return e.message;
  }
  return e.toString();
}

/// A destructive confirm dialog, standing in for the Web App's `confirm()`.
Future<bool> confirmDialog(
  BuildContext context, {
  required String title,
  required String message,
  String confirmLabel = 'Delete',
}) async {
  final cg = context.cg;
  final ok = await showDialog<bool>(
    context: context,
    builder: (ctx) => AlertDialog(
      backgroundColor: cg.card,
      title: Text(title, style: gsmDisplay(size: 17, color: cg.textPrimary)),
      content: Text(message, style: TextStyle(color: cg.textSecondary)),
      actions: [
        TextButton(
          onPressed: () => Navigator.of(ctx).pop(false),
          child: const Text('Cancel'),
        ),
        TextButton(
          onPressed: () => Navigator.of(ctx).pop(true),
          style: TextButton.styleFrom(foregroundColor: cg.danger),
          child: Text(confirmLabel),
        ),
      ],
    ),
  );
  return ok ?? false;
}
