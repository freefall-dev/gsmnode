import 'dart:convert';
import 'dart:typed_data';

import 'package:cryptography/cryptography.dart';

/// End-to-end encryption matching the Web App's `crypto.js`.
///
/// A shared passphrase (entered by the user, stored only on-device) derives an
/// AES-256-GCM key via PBKDF2-HMAC-SHA256. Outbound messages are decrypted here
/// before being handed to the radio; inbound messages are encrypted before being
/// forwarded to the server, so the API Server / PocketBase only ever hold
/// ciphertext.
///
/// Wire format of an encrypted value:  "gsmenc:v1:" + base64( salt || iv || ct )
///   salt = 16 bytes (PBKDF2)
///   iv   = 12 bytes (AES-GCM nonce)
///   ct   = AES-GCM ciphertext with the 16-byte tag appended
class CryptoService {
  static const _prefix = 'gsmenc:v1:';
  static const _iterations = 150000;

  final String passphrase;
  CryptoService(this.passphrase);

  bool get enabled => passphrase.isNotEmpty;

  static bool isEncrypted(String value) => value.startsWith(_prefix);

  final _pbkdf2 = Pbkdf2(
    macAlgorithm: Hmac.sha256(),
    iterations: _iterations,
    bits: 256,
  );
  final _aes = AesGcm.with256bits();

  Future<SecretKey> _deriveKey(List<int> salt) => _pbkdf2.deriveKey(
        secretKey: SecretKey(utf8.encode(passphrase)),
        nonce: salt,
      );

  /// Encrypts [plain] with the passphrase. Empty strings and the no-passphrase
  /// case pass through unchanged.
  Future<String> encrypt(String plain) async {
    if (!enabled || plain.isEmpty) return plain;
    final salt = _randomBytes(16);
    final iv = _randomBytes(12);
    final key = await _deriveKey(salt);
    final box = await _aes.encrypt(utf8.encode(plain), secretKey: key, nonce: iv);
    // AES-GCM appends the 16-byte tag after the ciphertext in the wire format.
    final ct = Uint8List.fromList([...box.cipherText, ...box.mac.bytes]);
    final packed = Uint8List.fromList([...salt, ...iv, ...ct]);
    return _prefix + base64.encode(packed);
  }

  /// Decrypts a value produced by [encrypt] (or the Web App). Non-encrypted
  /// values pass through unchanged.
  Future<String> decrypt(String value) async {
    if (!isEncrypted(value)) return value;
    if (!enabled) {
      throw StateError('encrypted payload but no passphrase configured');
    }
    final packed = base64.decode(value.substring(_prefix.length));
    final salt = packed.sublist(0, 16);
    final iv = packed.sublist(16, 28);
    final rest = packed.sublist(28);
    final cipherText = rest.sublist(0, rest.length - 16);
    final mac = Mac(rest.sublist(rest.length - 16));
    final key = await _deriveKey(salt);
    final clear = await _aes.decrypt(
      SecretBox(cipherText, nonce: iv, mac: mac),
      secretKey: key,
    );
    return utf8.decode(clear);
  }

  /// Encrypts each element of a list (e.g. recipient phone numbers).
  Future<List<String>> encryptList(List<String> list) async =>
      [for (final v in list) await encrypt(v)];

  Uint8List _randomBytes(int n) {
    final rnd = SecretKeyData.random(length: n);
    return Uint8List.fromList(rnd.bytes);
  }
}
