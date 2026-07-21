import 'package:flutter/material.dart';

import 'storage.dart';

/// The persisted light/dark/system preference, mirroring the Web App's
/// `theme.js`. "system" follows the OS and tracks it live; the app bar's
/// quick-toggle pins an explicit choice so a flip away from "system" sticks
/// rather than snapping back.
class ThemeController extends ChangeNotifier {
  ThemeController(this.storage);

  final Storage storage;

  static const _valid = {'light', 'dark', 'system'};

  /// The user's choice — drives the Settings picker.
  String get pref {
    final v = storage.themePref;
    return _valid.contains(v) ? v : 'system';
  }

  ThemeMode get mode => switch (pref) {
        'light' => ThemeMode.light,
        'dark' => ThemeMode.dark,
        _ => ThemeMode.system,
      };

  void setPref(String value) {
    storage.themePref = _valid.contains(value) ? value : 'system';
    notifyListeners();
  }

  /// Whether the theme currently rendering is the dark one. Needs a context
  /// because under "system" only the platform can answer.
  bool isDark(BuildContext context) => switch (pref) {
        'light' => false,
        'dark' => true,
        _ => MediaQuery.platformBrightnessOf(context) == Brightness.dark,
      };

  /// App-bar quick toggle: flip the effective theme and pin it as an explicit
  /// choice.
  void toggle(BuildContext context) =>
      setPref(isDark(context) ? 'light' : 'dark');
}
