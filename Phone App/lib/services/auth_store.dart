import 'dart:convert';

import 'package:flutter/foundation.dart';

import 'api_client.dart';
import 'storage.dart';

/// The signed-in account, as the API Server describes it.
class AppUser {
  const AppUser({
    required this.id,
    required this.email,
    this.name = '',
    this.role = 'user',
    this.organization = '',
    this.verified = false,
  });

  final String id;
  final String email;
  final String name;
  final String role;
  final String organization;
  final bool verified;

  bool get isSuperadmin => role == 'superadmin';
  bool get isManager => role == 'admin' || role == 'superadmin';

  /// Title-cased role, for the Settings badge.
  String get roleLabel =>
      role.isEmpty ? 'User' : role[0].toUpperCase() + role.substring(1);

  factory AppUser.fromJson(Map<String, dynamic> j) => AppUser(
        id: (j['id'] ?? '').toString(),
        email: (j['email'] ?? '').toString(),
        name: (j['name'] ?? '').toString(),
        role: (j['role'] ?? 'user').toString(),
        organization: (j['organization'] ?? '').toString(),
        verified: j['verified'] == true,
      );

  Map<String, dynamic> toJson() => {
        'id': id,
        'email': email,
        'name': name,
        'role': role,
        'organization': organization,
        'verified': verified,
      };

  AppUser copyWith({String? name}) => AppUser(
        id: id,
        email: email,
        name: name ?? this.name,
        role: role,
        organization: organization,
        verified: verified,
      );
}

/// Login state, mirroring the Web App's `store/auth.js`. Screens listen to this
/// so a role change (e.g. creating an organization promotes you to its admin)
/// re-gates the UI without a re-login.
class AuthStore extends ChangeNotifier {
  AuthStore(this.api, this.storage) {
    final raw = storage.userJson;
    if (raw != null && raw.isNotEmpty) {
      try {
        _user = AppUser.fromJson(jsonDecode(raw) as Map<String, dynamic>);
      } catch (_) {
        _user = null; // a shape change shouldn't strand someone on a blank app
      }
    }
  }

  final ApiClient api;
  final Storage storage;

  AppUser? _user;
  AppUser? get user => _user;

  bool get isAuthenticated => storage.isAuthenticated;

  Future<AppUser> login(String email, String password) async {
    final res = await api.post('/auth/login', {
      'email': email,
      'password': password,
    }) as Map<String, dynamic>;
    storage.jwt = res['access_token'] as String?;
    _persist(AppUser.fromJson((res['user'] as Map).cast<String, dynamic>()));
    return _user!;
  }

  /// Merges fresh fields into the cached user (e.g. after editing the profile in
  /// Settings) so anything reading [user] updates without a re-login.
  void updateUser({String? name}) {
    if (_user == null) return;
    _persist(_user!.copyWith(name: name));
  }

  /// Re-fetches the caller's identity and replaces the cached user. Used after a
  /// self-affecting change (creating or deleting an organization flips the
  /// caller's role and org) so gating updates live. Fields are replaced rather
  /// than merged because the server omits an empty organization, and a merge
  /// would keep a stale one.
  Future<AppUser> refresh() async {
    final me = await api.get('/auth/me') as Map<String, dynamic>;
    _persist(AppUser.fromJson(me));
    return _user!;
  }

  Future<void> logout() async {
    _user = null;
    await storage.clearSession();
    notifyListeners();
  }

  void _persist(AppUser u) {
    _user = u;
    storage.userJson = jsonEncode(u.toJson());
    notifyListeners();
  }
}
