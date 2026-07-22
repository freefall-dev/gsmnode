package api

import (
	"encoding/json"
	"testing"
)

// TestEnqueueCallSim pins the SIM slot on an outbound call. The field is a
// pointer for the same reason a message's is: slot 0 is the first SIM, and a
// call asked to leave on it must be distinguishable from one that never named a
// slot at all. Decoding into an int would collapse the two and quietly send
// every explicit "SIM 0" call out on the phone's default account.
func TestEnqueueCallSim(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		wantSet  bool
		wantSlot int
		// wantStored is the value written to messages.sim_number, which is
		// 1-based so that "unset" can stay the zero value in PocketBase.
		wantStored int
	}{
		{"explicit first SIM", `{"phone_number":"+15551234567","sim_number":0}`, true, 0, 1},
		{"explicit second SIM", `{"phone_number":"+15551234567","sim_number":1}`, true, 1, 2},
		{"omitted", `{"phone_number":"+15551234567"}`, false, 0, 0},
		{"explicit null", `{"phone_number":"+15551234567","sim_number":null}`, false, 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var req enqueueCallRequest
			if err := json.Unmarshal([]byte(c.body), &req); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if (req.SimNumber != nil) != c.wantSet {
				t.Fatalf("set: want %v, got %v", c.wantSet, req.SimNumber != nil)
			}
			if !c.wantSet {
				return
			}
			if *req.SimNumber != c.wantSlot {
				t.Fatalf("slot: want %d, got %d", c.wantSlot, *req.SimNumber)
			}
			if got := packSlot(*req.SimNumber); got != c.wantStored {
				t.Fatalf("stored: want %d, got %d", c.wantStored, got)
			}
			// And back out again, the way the device receives it on pull.
			if got := unpackSlot(packSlot(*req.SimNumber)); got == nil || *got != c.wantSlot {
				t.Fatalf("round trip: want %d, got %v", c.wantSlot, got)
			}
		})
	}
}
