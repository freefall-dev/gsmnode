// Unit tests for the pieces that carry real logic and don't need a live server:
// the E2E crypto (which must stay wire-compatible with the Web App and the
// Phone Agent) and the shared response helpers.

import 'package:console/services/api_client.dart';
import 'package:console/services/crypto_service.dart';
import 'package:console/services/storage.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';

Future<CryptoService> cryptoWith(String passphrase) async {
  SharedPreferences.setMockInitialValues({});
  final storage = await Storage.create();
  storage.encPassphrase = passphrase;
  return CryptoService(storage);
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  group('CryptoService', () {
    test('round-trips text through the gsmenc:v1 envelope', () async {
      final crypto = await cryptoWith('correct horse battery staple');
      final sealed = await crypto.encrypt('hello gateway');

      expect(CryptoService.isEncrypted(sealed), isTrue);
      expect(sealed, startsWith('gsmenc:v1:'));
      expect(sealed, isNot(contains('hello gateway')));
      expect(await crypto.decrypt(sealed), 'hello gateway');
    });

    test('passes values through when no passphrase is set', () async {
      final crypto = await cryptoWith('');
      expect(crypto.enabled, isFalse);
      expect(await crypto.encrypt('plain'), 'plain');
      expect(await crypto.decrypt('plain'), 'plain');
    });

    test('salts every message, so the same text seals differently', () async {
      final crypto = await cryptoWith('shared');
      final a = await crypto.encrypt('same text');
      final b = await crypto.encrypt('same text');
      expect(a, isNot(b));
      expect(await crypto.decrypt(a), await crypto.decrypt(b));
    });

    test('tryDecrypt marks what the wrong passphrase cannot open', () async {
      final sealed = await (await cryptoWith('the right one')).encrypt('secret');
      final wrong = await cryptoWith('the wrong one');

      expect(await wrong.tryDecrypt(sealed), CryptoService.unreadable);
      // Plaintext still passes straight through.
      expect(await wrong.tryDecrypt('not encrypted'), 'not encrypted');
    });
  });

  group('itemsOf', () {
    test('reads the list under the requested key', () {
      final items = itemsOf({
        'items': [
          {'id': 'a'},
          {'id': 'b'},
        ]
      });
      expect(items.map((e) => e['id']), ['a', 'b']);
      expect(
        itemsOf({
          'organizations': [
            {'id': 'org1'}
          ]
        }, 'organizations').single['id'],
        'org1',
      );
    });

    test('treats a missing or malformed body as empty', () {
      // The API Server omits the key entirely when there is nothing to return.
      expect(itemsOf(const {}), isEmpty);
      expect(itemsOf(null), isEmpty);
      expect(itemsOf('unexpected'), isEmpty);
      expect(itemsOf(const {'items': 'not a list'}), isEmpty);
    });
  });

  group('ApiException', () {
    test('flags a transport failure as unreachable', () {
      expect(ApiException(0, 'no route').unreachable, isTrue);
      expect(ApiException(401, 'unauthorized').unreachable, isFalse);
    });
  });
}
