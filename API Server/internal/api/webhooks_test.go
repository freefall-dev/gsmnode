package api

import (
	"strings"
	"testing"
)

// TestDispatchedEventsAreRegisterable is the regression guard for events that
// the server dispatches but refuses to let anyone subscribe to. The call, MMS
// and data-SMS events were added to the dispatch paths and the Web App's picker
// without being added to the registration whitelist, so choosing one returned
// "400 invalid event" and their dispatch code was unreachable in practice.
func TestDispatchedEventsAreRegisterable(t *testing.T) {
	dispatched := []string{
		// inbox.go
		"sms:received", "sms:data-received", "mms:received", "mms:downloaded",
		// messages.go (eventForStatus) and sweeper.go
		"sms:sent", "sms:delivered", "sms:failed",
		// calls.go (eventForCall)
		"call:received", "call:sent", "call:failed",
	}
	for _, e := range dispatched {
		if !validWebhookEvents[e] {
			t.Errorf("%q is dispatched but cannot be registered", e)
		}
	}
}

// TestEventMappersStayInTheCatalogue covers the mappers rather than the literal
// strings, so a new status or direction can't introduce an event that nothing
// is allowed to subscribe to.
func TestEventMappersStayInTheCatalogue(t *testing.T) {
	for _, status := range []string{statusSent, statusDelivered, statusFailed} {
		if e := eventForStatus(status); e != "" && !validWebhookEvents[e] {
			t.Errorf("eventForStatus(%q) = %q, not registerable", status, e)
		}
	}
	// statusProcessed is deliberately silent — it is not a notable state.
	if e := eventForStatus(statusProcessed); e != "" {
		t.Errorf("eventForStatus(%q) = %q, want no event", statusProcessed, e)
	}

	for _, dir := range []string{callIncoming, callOutgoing} {
		for _, status := range []string{"completed", "failed", "missed", "rejected"} {
			if e := eventForCall(dir, status); !validWebhookEvents[e] {
				t.Errorf("eventForCall(%q, %q) = %q, not registerable", dir, status, e)
			}
		}
	}
}

// TestSignPayloadBindsTimestampAndBody pins the signature scheme, which the
// Home Assistant plugin recomputes byte for byte on the other side. The
// timestamp is inside the MAC on purpose: a receiver rejects deliveries whose
// timestamp has aged out, and if it were only a header an attacker replaying a
// captured body could refresh it and be believed.
func TestSignPayloadBindsTimestampAndBody(t *testing.T) {
	const secret = "s3cret"
	body := []byte(`{"event":"sms:received"}`)
	base := signPayload(secret, 1_700_000_000, body)

	if len(base) != 64 {
		t.Fatalf("want a hex sha256, got %d chars", len(base))
	}
	if got := signPayload(secret, 1_700_000_000, body); got != base {
		t.Error("signing is not deterministic")
	}
	if got := signPayload(secret, 1_700_000_001, body); got == base {
		t.Error("a different timestamp must change the signature")
	}
	if got := signPayload(secret, 1_700_000_000, []byte(`{"event":"sms:sent"}`)); got == base {
		t.Error("a different body must change the signature")
	}
	if got := signPayload("other", 1_700_000_000, body); got == base {
		t.Error("a different secret must change the signature")
	}
	// The separator must not be forgeable by moving the boundary: signing
	// ts=1, body="7.x" must differ from ts=17, body=".x".
	if signPayload(secret, 1, []byte("7.x")) == signPayload(secret, 17, []byte(".x")) {
		t.Error("timestamp and body are not unambiguously separated")
	}
}

// TestRedactURLKeepsTheSecretOut covers the logging fix. A Home Assistant
// webhook is authenticated by its unguessable path alone, so the path is the
// credential — logging the full URL on a delivery failure wrote it to disk.
func TestRedactURLKeepsTheSecretOut(t *testing.T) {
	const secret = "2f9a1c7b5e"
	cases := []string{
		"http://10.2.1.20:8123/api/webhook/" + secret,
		"https://ha.example.com/api/webhook/" + secret + "?x=" + secret,
	}
	for _, raw := range cases {
		got := redactURL(raw)
		if strings.Contains(got, secret) {
			t.Errorf("redactURL(%q) = %q, still carries the secret", raw, got)
		}
		if !strings.Contains(got, "://") {
			t.Errorf("redactURL(%q) = %q, want the origin kept for debugging", raw, got)
		}
	}
	if got := redactURL("://nonsense"); strings.Contains(got, "nonsense") {
		t.Errorf("an unparseable url must not be echoed: %q", got)
	}
}

// TestSignPayloadInteropVector is the shared vector between this server and the
// Home Assistant plugin, whose tests/test_signature.py asserts the same hex for
// the same inputs. Signing is only useful if both sides derive it byte for
// byte: drift here would not fail loudly, it would make every delivery look
// forged and silently stop incoming SMS reaching any automation.
func TestSignPayloadInteropVector(t *testing.T) {
	const (
		secret = "2f9a1c7b5e"
		body   = `{"event":"sms:received","payload":{"phone_number":"+15551234567"}}`
		want   = "e7e0acc91bccd0df9151852c5c5ce6b5da3c9a88d0db9f513484a1c9c80b047f"
	)
	if got := signPayload(secret, 1_700_000_000, []byte(body)); got != want {
		t.Fatalf("interop vector drifted:\n got %s\nwant %s", got, want)
	}
}
