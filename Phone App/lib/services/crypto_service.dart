import 'dart:convert';
import 'dart:typed_data';

import 'package:cryptography/cryptography.dart';

import 'storage.dart';

/// End-to-end encryption matching the Web App's `crypto.js` and the Phone
/// Agent's `crypto_service.dart`.
///
/// A shared passphrase (entered by the user, stored only on-device) derives an
/// AES-256-GCM key via PBKDF2-HMAC-SHA256. Outbound text and recipient numbers
/// are encrypted before they leave the phone; inbound items are decrypted for
/// display. The API Server and PocketBase only ever hold ciphertext.
///
/// Wire format of an encrypted value:  "gsmenc:v1:" + base64( salt || iv || ct )
///   salt = 16 bytes (PBKDF2)
///   iv   = 12 bytes (AES-GCM nonce)
///   ct   = AES-GCM ciphertext with the 16-byte tag appended
///
/// Deliberately *not* encrypted, matching the other two surfaces: the MMS
/// subject, data-SMS payloads and MMS attachments. Don't add encryption on one
/// end without the other.
class CryptoService {
  static const _prefix = 'gsmenc:v1:';
  static const _iterations = 150000;

  /// The marker shown in place of text the passphrase can't open, so a viewer
  /// never displays a raw ciphertext blob.
  static const unreadable = '🔒 encrypted (wrong or missing passphrase)';

  /// Reads the passphrase from [Storage] on every call rather than caching it,
  /// so changing it in Settings takes effect on the next screen refresh.
  final Storage storage;
  CryptoService(this.storage);

  String get passphrase => storage.encPassphrase;
  bool get enabled => passphrase.isNotEmpty;

  static bool isEncrypted(Object? value) =>
      value is String && value.startsWith(_prefix);

  final _pbkdf2 = Pbkdf2(
    macAlgorithm: Hmac.sha256(),
    iterations: _iterations,
    bits: 256,
  );
  final _aes = AesGcm.with256bits();

  Future<SecretKey> _deriveKey(String pass, List<int> salt) => _pbkdf2.deriveKey(
        secretKey: SecretKey(utf8.encode(pass)),
        nonce: salt,
      );

  /// Encrypts [plain] with the current passphrase. Empty strings and the
  /// no-passphrase case pass through unchanged.
  Future<String> encrypt(String plain) async {
    final pass = passphrase;
    if (pass.isEmpty || plain.isEmpty) return plain;
    final salt = _randomBytes(16);
    final iv = _randomBytes(12);
    final key = await _deriveKey(pass, salt);
    final box =
        await _aes.encrypt(utf8.encode(plain), secretKey: key, nonce: iv);
    // AES-GCM appends the 16-byte tag after the ciphertext in the wire format.
    final packed =
        Uint8List.fromList([...salt, ...iv, ...box.cipherText, ...box.mac.bytes]);
    return _prefix + base64.encode(packed);
  }

  /// Encrypts each element of a list (e.g. recipient phone numbers).
  Future<List<String>> encryptList(List<String> list) async =>
      [for (final v in list) await encrypt(v)];

  /// Decrypts a value produced by [encrypt] (or by the Web App / Phone Agent).
  /// Non-encrypted values pass through unchanged; a wrong or missing passphrase
  /// throws.
  Future<String> decrypt(String value) async {
    if (!isEncrypted(value)) return value;
    final pass = passphrase;
    if (pass.isEmpty) {
      throw StateError('encrypted; set the passphrase in Settings');
    }
    final packed = base64.decode(value.substring(_prefix.length));
    final salt = packed.sublist(0, 16);
    final iv = packed.sublist(16, 28);
    final rest = packed.sublist(28);
    final cipherText = rest.sublist(0, rest.length - 16);
    final mac = Mac(rest.sublist(rest.length - 16));
    final key = await _deriveKey(pass, salt);
    final clear = await _aes.decrypt(
      SecretBox(cipherText, nonce: iv, mac: mac),
      secretKey: key,
    );
    return utf8.decode(clear);
  }

  /// Best-effort decrypt for display: returns [unreadable] instead of throwing,
  /// so one bad item can't blank out a whole list.
  Future<String> tryDecrypt(Object? value) async {
    if (value == null) return '';
    if (!isEncrypted(value)) return value.toString();
    try {
      return await decrypt(value as String);
    } catch (_) {
      return unreadable;
    }
  }

  /// [tryDecrypt] over a list of values (e.g. a message's recipients).
  Future<List<String>> tryDecryptList(Iterable<Object?> values) async =>
      [for (final v in values) await tryDecrypt(v)];

  Uint8List _randomBytes(int n) =>
      Uint8List.fromList(SecretKeyData.random(length: n).bytes);
}
