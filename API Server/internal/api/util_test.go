package api

import "testing"

// TestSlotRoundTrip covers the 1-based-at-rest SIM slot encoding. The key case
// is float64(0): PocketBase returns 0 (not null) for an unset number field, and
// that must decode to "unset" (nil), not slot 0.
func TestSlotRoundTrip(t *testing.T) {
	cases := []struct {
		name   string
		stored any  // what PocketBase hands back
		want   *int // decoded 0-based slot, nil = unset
	}{
		{"unset returns zero from pocketbase", float64(0), nil},
		{"absent field", nil, nil},
		{"slot 0 stored as 1", float64(1), ptr(0)},
		{"slot 1 stored as 2", float64(2), ptr(1)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := unpackSlot(c.stored)
			switch {
			case c.want == nil && got != nil:
				t.Fatalf("want nil, got %d", *got)
			case c.want != nil && got == nil:
				t.Fatalf("want %d, got nil", *c.want)
			case c.want != nil && *got != *c.want:
				t.Fatalf("want %d, got %d", *c.want, *got)
			}
		})
	}

	// packSlot/unpackSlot must round-trip every real slot, including slot 0.
	for slot := 0; slot < 4; slot++ {
		got := unpackSlot(float64(packSlot(slot)))
		if got == nil || *got != slot {
			t.Fatalf("round-trip slot %d failed: got %v", slot, got)
		}
	}
}

func ptr(n int) *int { return &n }
