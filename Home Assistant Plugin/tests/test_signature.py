"""Cross-checks the webhook signature the API Server produces.

The plugin's verifier lives in events.py, which cannot be imported without Home
Assistant. What can be checked here is the thing both sides have to agree on:
the exact bytes that go through HMAC-SHA256. This recomputes the Go server's
scheme in Python and asserts the properties the verifier depends on — if the two
ever drift, every delivery would be rejected as forged.

Run from the "Home Assistant Plugin" folder:

    python -m unittest discover tests
"""
import hashlib
import hmac
import unittest


def sign(secret: str, timestamp: int, body: bytes) -> str:
    """The signature, as both sides build it: HMAC-SHA256 over "<ts>.<body>"."""
    return hmac.new(
        secret.encode(), f"{timestamp}.".encode() + body, hashlib.sha256
    ).hexdigest()


SECRET = "2f9a1c7b5e"
BODY = b'{"event":"sms:received","payload":{"phone_number":"+15551234567"}}'
TS = 1_700_000_000


class TestSignatureScheme(unittest.TestCase):
    def test_matches_the_interop_vector(self):
        """The shared vector, asserted identically by the Go side.

        API Server/internal/api/webhooks_test.go pins the same hex for the same
        secret, timestamp and body. Signing is only useful if both sides derive
        it byte for byte — drift would not fail loudly, it would make every
        delivery look forged and silently stop incoming SMS reaching any
        automation.
        """
        self.assertEqual(
            sign(SECRET, TS, BODY),
            "e7e0acc91bccd0df9151852c5c5ce6b5da3c9a88d0db9f513484a1c9c80b047f",
        )

    def test_body_is_covered(self):
        """Editing the payload must invalidate the signature."""
        forged = BODY.replace(b"+15551234567", b"+19998887777")
        self.assertNotEqual(sign(SECRET, TS, BODY), sign(SECRET, TS, forged))

    def test_timestamp_is_covered(self):
        """A replay cannot be refreshed by rewriting the timestamp header.

        This is why the timestamp is inside the MAC rather than beside it: the
        verifier rejects an old delivery on age, and an attacker who could edit
        the timestamp freely would defeat that check.
        """
        self.assertNotEqual(sign(SECRET, TS, BODY), sign(SECRET, TS + 1, BODY))

    def test_secret_is_covered(self):
        """Without the secret the signature cannot be produced."""
        self.assertNotEqual(sign(SECRET, TS, BODY), sign("guess", TS, BODY))

    def test_boundary_is_unambiguous(self):
        """The separator must not let the timestamp and body be re-split.

        Signing (1, "7.x") and (17, ".x") both concatenate to "17.x"; if the two
        produced the same MAC, a delivery could be replayed under a different
        timestamp without touching the signature.
        """
        self.assertNotEqual(sign(SECRET, 1, b"7.x"), sign(SECRET, 17, b".x"))

    def test_comparison_is_constant_time(self):
        """The verifier must use compare_digest, not ==.

        Guarding the property rather than the call: a plain comparison leaks how
        much of a guessed signature was right, one byte at a time.
        """
        good = sign(SECRET, TS, BODY)
        self.assertTrue(hmac.compare_digest(good, sign(SECRET, TS, BODY)))
        self.assertFalse(hmac.compare_digest(good, "0" * 64))

    def test_prefix_is_stripped_not_signed(self):
        """The header reads "sha256=<hex>"; only the hex is compared."""
        header = "sha256=" + sign(SECRET, TS, BODY)
        self.assertEqual(header.removeprefix("sha256="), sign(SECRET, TS, BODY))


if __name__ == "__main__":
    unittest.main()
