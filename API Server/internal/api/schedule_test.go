package api

import (
	"strings"
	"testing"
	"time"

	"smsgateway/apiserver/internal/pb"
)

// TestScheduleTime covers decoding messages.schedule_at. An unset or malformed
// value must report "not scheduled" so it stays on the normal send path rather
// than being withheld forever.
func TestScheduleTime(t *testing.T) {
	want := time.Date(2026, 7, 21, 14, 30, 0, 0, time.UTC)
	cases := []struct {
		name   string
		stored any
		ok     bool
	}{
		{"pocketbase format", "2026-07-21 14:30:00.000Z", true},
		{"rfc3339", "2026-07-21T14:30:00Z", true},
		{"unset", "", false},
		{"absent field", nil, false},
		{"malformed", "not a date", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := scheduleTime(pb.Record{"schedule_at": c.stored})
			if ok != c.ok {
				t.Fatalf("ok: want %v, got %v", c.ok, ok)
			}
			if ok && !got.Equal(want) {
				t.Fatalf("want %v, got %v", want, got)
			}
		})
	}
}

// TestDueFilter pins the pull-side scheduling gate: the filter must admit
// unscheduled messages and those already due, and exclude future ones. Without
// it a scheduled message is handed to the device on the next poll and sent
// immediately.
func TestDueFilter(t *testing.T) {
	now := time.Date(2026, 7, 21, 14, 30, 0, 0, time.UTC)
	got := dueFilter(now)

	if !strings.Contains(got, `schedule_at = ""`) {
		t.Errorf("filter must admit unscheduled messages: %s", got)
	}
	if !strings.Contains(got, `schedule_at <= "2026-07-21 14:30:00.000Z"`) {
		t.Errorf("filter must bound on the current time: %s", got)
	}
}

// TestSweeperSkipsScheduled documents the expiry rule for scheduled messages:
// their TTL runs from the scheduled time, not from creation. A message
// scheduled for the future — or only just due — must survive a sweep, or it
// would be failed before any device had a chance to pull it.
func TestSweeperSkipsScheduled(t *testing.T) {
	const ttl = 10 * time.Minute
	now := time.Date(2026, 7, 21, 14, 30, 0, 0, time.UTC)

	cases := []struct {
		name string
		at   time.Time
		skip bool
	}{
		{"scheduled far ahead", now.Add(2 * time.Hour), true},
		{"just came due", now.Add(-1 * time.Minute), true},
		{"due longer than the ttl ago", now.Add(-11 * time.Minute), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := pb.Record{"schedule_at": pbTime(c.at)}
			at, ok := scheduleTime(rec)
			if !ok {
				t.Fatal("schedule_at should parse")
			}
			// Mirrors the guard in expireStaleMessages.
			if skip := now.Before(at.Add(ttl)); skip != c.skip {
				t.Fatalf("skip: want %v, got %v", c.skip, skip)
			}
		})
	}
}
