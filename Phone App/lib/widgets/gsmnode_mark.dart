import 'package:flutter/material.dart';
import 'package:google_fonts/google_fonts.dart';

import '../theme.dart';

/// The gsmnode routing mark — two stacked arrows: the top (ink) routes right,
/// the bottom (green) routes left. Geometry matches the design system's
/// `mark-color.svg` (2.6px stroke on a 40px grid, rounded terminals).
///
/// When [color] is given the whole mark is drawn in that single color
/// (e.g. white on a filled tile); otherwise the top arrow takes the surface
/// ink color and the bottom arrow the signal green.
class GsmNodeMark extends StatelessWidget {
  const GsmNodeMark({super.key, this.size = 32, this.color});

  final double size;
  final Color? color;

  @override
  Widget build(BuildContext context) {
    final ink = color ?? Theme.of(context).colorScheme.onSurface;
    final green = color ?? GsmColors.green500;
    return CustomPaint(
      size: Size.square(size),
      painter: _MarkPainter(ink: ink, green: green),
    );
  }
}

class _MarkPainter extends CustomPainter {
  _MarkPainter({required this.ink, required this.green});

  final Color ink;
  final Color green;

  @override
  void paint(Canvas canvas, Size size) {
    final k = size.width / 40; // scale from the 40px design grid
    Paint stroke(Color c) => Paint()
      ..color = c
      ..style = PaintingStyle.stroke
      ..strokeWidth = 2.6 * k
      ..strokeCap = StrokeCap.round
      ..strokeJoin = StrokeJoin.round;

    final inkPaint = stroke(ink);
    final greenPaint = stroke(green);

    // Top arrow — routes right (ink).
    canvas.drawLine(Offset(7 * k, 15 * k), Offset(31 * k, 15 * k), inkPaint);
    canvas.drawPath(
      Path()
        ..moveTo(26 * k, 10 * k)
        ..lineTo(32 * k, 15 * k)
        ..lineTo(26 * k, 20 * k),
      inkPaint,
    );

    // Bottom arrow — routes left (green).
    canvas.drawLine(Offset(9 * k, 25 * k), Offset(33 * k, 25 * k), greenPaint);
    canvas.drawPath(
      Path()
        ..moveTo(14 * k, 20 * k)
        ..lineTo(8 * k, 25 * k)
        ..lineTo(14 * k, 30 * k),
      greenPaint,
    );
  }

  @override
  bool shouldRepaint(_MarkPainter old) => old.ink != ink || old.green != green;
}

/// The gsmnode wordmark: lowercase `gsm` in ink + `node` in green, monospace.
/// On dark surfaces `node` lifts to the on-dark green. Pass [color] to render
/// the whole wordmark in a single color.
class GsmNodeWordmark extends StatelessWidget {
  const GsmNodeWordmark({super.key, this.size = 24, this.color});

  final double size;
  final Color? color;

  @override
  Widget build(BuildContext context) {
    final isDark = Theme.of(context).brightness == Brightness.dark;
    final ink = color ?? Theme.of(context).colorScheme.onSurface;
    final green =
        color ?? (isDark ? GsmColors.greenOnDark : GsmColors.green500);
    return Text.rich(
      TextSpan(children: [
        TextSpan(text: 'gsm', style: TextStyle(color: ink)),
        TextSpan(text: 'node', style: TextStyle(color: green)),
      ]),
      style: GoogleFonts.jetBrainsMono(
        fontSize: size,
        fontWeight: FontWeight.w600,
        letterSpacing: -0.01 * size,
        height: 1,
      ),
    );
  }
}
