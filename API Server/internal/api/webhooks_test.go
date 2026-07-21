package api

import "testing"

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
