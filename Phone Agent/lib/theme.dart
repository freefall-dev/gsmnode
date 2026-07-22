import 'package:flutter/material.dart';
import 'package:google_fonts/google_fonts.dart';

/// gsmnode design tokens. Brand is signal-green #2E9E6B on an ink/paper cool
/// scale; the Web App's `web/src/theme.js` and `style.css` carry the same ramp.
abstract class GsmColors {
  // Signal green (brand) ramp
  static const green100 = Color(0xFFE4F4EC);
  static const green200 = Color(0xFFBEE6D0);
  static const green300 = Color(0xFF6BC79A); // lifted text on dark
  static const green500 = Color(0xFF2E9E6B); // primary
  static const green600 = Color(0xFF278A5C); // hover
  static const green700 = Color(0xFF1F6E49); // active
  static const greenOnDark = Color(0xFF4FB985); // accent on ink surfaces

  // Ink & paper
  static const ink = Color(0xFF12161C); // primary text / dark surface
  static const ink900 = Color(0xFF0A0D11); // deepest surface
  static const ink800 = Color(0xFF1A1F27);
  static const paper = Color(0xFFFAFAF9); // page bg

  // Cool gray ramp
  static const gray50 = Color(0xFFF4F5F4);
  static const gray100 = Color(0xFFE9EBEA);
  static const gray200 = Color(0xFFDCDFDE); // subtle border
  static const gray300 = Color(0xFFC2C7C6); // strong border
  static const gray400 = Color(0xFF9AA0A2); // muted text
  static const gray500 = Color(0xFF6B7278);
  static const gray600 = Color(0xFF4A5157);
  static const gray700 = Color(0xFF333A40); // secondary text

  // Semantic (light)
  static const warning = Color(0xFFC68A2E);
  static const warningTint = Color(0xFFF6EAD6);
  static const danger = Color(0xFFC64A3E);
  static const dangerTint = Color(0xFFF6E1DC);

  // Dark surfaces
  static const dBase = Color(0xFF0A0D11);
  static const dSunken = Color(0xFF0E1216);
  static const dCard = Color(0xFF14191F);
  static const dRaised = Color(0xFF1A2027);
  static const dBorderSubtle = Color(0xFF262B33);
  static const dBorderStrong = Color(0xFF363C46);
  static const dTextPrimary = Color(0xFFF5F6F4);
  static const dTextSecondary = Color(0xFFC2C7C6);
  static const dTextMuted = Color(0xFF9AA0A2);

  // Semantic (dark — lifted for contrast)
  static const dWarning = Color(0xFFE0A84E);
  static const dDanger = Color(0xFFE0655B);
}

/// Semantic role colors that flip between light and dark, exposed as a
/// ThemeExtension so screens theme through roles, not raw ramp values.
class GsmSemantic extends ThemeExtension<GsmSemantic> {
  const GsmSemantic({
    required this.pageBg,
    required this.sunkenBg,
    required this.card,
    required this.borderSubtle,
    required this.borderStrong,
    required this.textPrimary,
    required this.textSecondary,
    required this.textMuted,
    required this.brandTint,
    required this.success,
    required this.successTint,
    required this.warning,
    required this.warningTint,
    required this.danger,
    required this.dangerTint,
  });

  final Color pageBg;
  final Color sunkenBg;
  final Color card;
  final Color borderSubtle;
  final Color borderStrong;
  final Color textPrimary;
  final Color textSecondary;
  final Color textMuted;
  final Color brandTint;
  final Color success;
  final Color successTint;
  final Color warning;
  final Color warningTint;
  final Color danger;
  final Color dangerTint;

  static const light = GsmSemantic(
    pageBg: GsmColors.paper,
    sunkenBg: GsmColors.gray50,
    card: Colors.white,
    borderSubtle: GsmColors.gray200,
    borderStrong: GsmColors.gray300,
    textPrimary: GsmColors.ink,
    textSecondary: GsmColors.gray700,
    textMuted: GsmColors.gray500,
    brandTint: GsmColors.green100,
    success: GsmColors.green500,
    successTint: GsmColors.green100,
    warning: GsmColors.warning,
    warningTint: GsmColors.warningTint,
    danger: GsmColors.danger,
    dangerTint: GsmColors.dangerTint,
  );

  static const dark = GsmSemantic(
    pageBg: GsmColors.dBase,
    sunkenBg: GsmColors.dSunken,
    card: GsmColors.dCard,
    borderSubtle: GsmColors.dBorderSubtle,
    borderStrong: GsmColors.dBorderStrong,
    textPrimary: GsmColors.dTextPrimary,
    textSecondary: GsmColors.dTextSecondary,
    textMuted: GsmColors.dTextMuted,
    brandTint: Color(0x292E9E6B), // green-500 @ 16%
    success: GsmColors.green300,
    successTint: Color(0x2E2E9E6B),
    warning: GsmColors.dWarning,
    warningTint: Color(0x2EC68A2E),
    danger: GsmColors.dDanger,
    dangerTint: Color(0x2EC64A3E),
  );

  @override
  GsmSemantic copyWith({Color? pageBg}) => this;

  @override
  GsmSemantic lerp(ThemeExtension<GsmSemantic>? other, double t) => this;
}

extension GsmThemeX on BuildContext {
  GsmSemantic get cg => Theme.of(this).extension<GsmSemantic>()!;
}

/// A mono (JetBrains Mono) style for IDs, numbers, timestamps and eyebrows.
TextStyle gsmMono({
  double size = 12,
  Color? color,
  FontWeight weight = FontWeight.w500,
  double letterSpacing = 0,
}) =>
    GoogleFonts.jetBrainsMono(
      fontSize: size,
      color: color,
      fontWeight: weight,
      letterSpacing: letterSpacing,
    );

/// Display (Space Grotesk) style for titles & metrics.
TextStyle gsmDisplay({
  double size = 20,
  Color? color,
  FontWeight weight = FontWeight.w700,
}) =>
    GoogleFonts.spaceGrotesk(
      fontSize: size,
      color: color,
      fontWeight: weight,
      letterSpacing: -0.02 * size,
    );

ThemeData _base(Brightness brightness, GsmSemantic s) {
  final isDark = brightness == Brightness.dark;
  final scheme = ColorScheme(
    brightness: brightness,
    primary: GsmColors.green500,
    onPrimary: Colors.white,
    secondary: isDark ? GsmColors.greenOnDark : GsmColors.green600,
    onSecondary: Colors.white,
    error: s.danger,
    onError: Colors.white,
    surface: s.card,
    onSurface: s.textPrimary,
    outline: s.borderStrong,
    outlineVariant: s.borderSubtle,
    surfaceContainerLowest: s.sunkenBg,
    surfaceContainerLow: s.pageBg,
    surfaceContainer: s.card,
  );

  // Body & UI in IBM Plex Sans; titles/metrics reach for Space Grotesk.
  final textTheme = GoogleFonts.ibmPlexSansTextTheme(
    (isDark ? ThemeData.dark() : ThemeData.light()).textTheme,
  ).apply(bodyColor: s.textPrimary, displayColor: s.textPrimary);

  const controlRadius = BorderRadius.all(Radius.circular(8));
  const cardRadius = BorderRadius.all(Radius.circular(18));

  return ThemeData(
    useMaterial3: true,
    colorScheme: scheme,
    scaffoldBackgroundColor: s.pageBg,
    textTheme: textTheme,
    extensions: [s],
    appBarTheme: AppBarTheme(
      backgroundColor: s.card,
      foregroundColor: s.textPrimary,
      elevation: 0,
      scrolledUnderElevation: 0,
      shape: Border(bottom: BorderSide(color: s.borderSubtle)),
      titleTextStyle: GoogleFonts.spaceGrotesk(
        fontSize: 17,
        fontWeight: FontWeight.w700,
        color: s.textPrimary,
        letterSpacing: -0.2,
      ),
    ),
    cardTheme: CardThemeData(
      color: s.card,
      elevation: 0,
      shape: RoundedRectangleBorder(
        borderRadius: cardRadius,
        side: BorderSide(color: s.borderSubtle),
      ),
      margin: EdgeInsets.zero,
    ),
    filledButtonTheme: FilledButtonThemeData(
      style: FilledButton.styleFrom(
        backgroundColor: GsmColors.green500,
        foregroundColor: Colors.white,
        elevation: 0,
        minimumSize: const Size.fromHeight(48),
        shape: const RoundedRectangleBorder(borderRadius: controlRadius),
        textStyle: GoogleFonts.ibmPlexSans(
          fontSize: 15,
          fontWeight: FontWeight.w600,
        ),
      ).copyWith(
        backgroundColor: WidgetStateProperty.resolveWith((states) {
          if (states.contains(WidgetState.disabled)) {
            return GsmColors.green500.withValues(alpha: 0.45);
          }
          if (states.contains(WidgetState.pressed)) return GsmColors.green700;
          if (states.contains(WidgetState.hovered)) return GsmColors.green600;
          return GsmColors.green500;
        }),
      ),
    ),
    outlinedButtonTheme: OutlinedButtonThemeData(
      style: OutlinedButton.styleFrom(
        foregroundColor: s.textPrimary,
        side: BorderSide(color: s.borderStrong),
        shape: const RoundedRectangleBorder(borderRadius: controlRadius),
        textStyle: GoogleFonts.ibmPlexSans(
          fontSize: 14,
          fontWeight: FontWeight.w600,
        ),
      ),
    ),
    textButtonTheme: TextButtonThemeData(
      style: TextButton.styleFrom(
        foregroundColor: isDark ? GsmColors.greenOnDark : GsmColors.green600,
        textStyle: GoogleFonts.ibmPlexSans(
          fontSize: 14,
          fontWeight: FontWeight.w600,
        ),
      ),
    ),
    inputDecorationTheme: InputDecorationTheme(
      filled: true,
      fillColor: s.card,
      contentPadding: const EdgeInsets.symmetric(horizontal: 12, vertical: 14),
      border: OutlineInputBorder(
        borderRadius: controlRadius,
        borderSide: BorderSide(color: s.borderStrong),
      ),
      enabledBorder: OutlineInputBorder(
        borderRadius: controlRadius,
        borderSide: BorderSide(color: s.borderStrong),
      ),
      focusedBorder: const OutlineInputBorder(
        borderRadius: controlRadius,
        borderSide: BorderSide(color: GsmColors.green500, width: 2),
      ),
      labelStyle: TextStyle(color: s.textSecondary),
      hintStyle: TextStyle(color: s.textMuted),
    ),
    dividerTheme: DividerThemeData(color: s.borderSubtle, thickness: 1),
    snackBarTheme: SnackBarThemeData(
      backgroundColor: isDark ? GsmColors.dRaised : GsmColors.ink,
      contentTextStyle: GoogleFonts.ibmPlexSans(color: Colors.white),
      shape: const RoundedRectangleBorder(borderRadius: controlRadius),
      behavior: SnackBarBehavior.floating,
    ),
  );
}

ThemeData gsmnodeLightTheme() => _base(Brightness.light, GsmSemantic.light);
ThemeData gsmnodeDarkTheme() => _base(Brightness.dark, GsmSemantic.dark);
