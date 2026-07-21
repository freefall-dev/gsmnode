package api

import (
	"testing"
	"time"

	"smsgateway/apiserver/internal/pb"
)

// TestDeviceStatus covers how a device's online badge is decided. The heartbeat
// window alone used to decide it, which left a phone that had deliberately
// stopped routing showing "online" in the panel and Web App until onlineWindow
// lapsed. An explicit offline report now wins over a recent heartbeat.
func TestDeviceStatus(t *testing.T) {
	fresh := pbTime(time.Now())
	stale := pbTime(time.Now().Add(-2 * onlineWindow))

	cases := []struct {
		name     string
		lastSeen string
		stored   any // devices.status as stored
		want     string
	}{
		{"recent heartbeat", fresh, "online", "online"},
		{"recent heartbeat, status absent", fresh, nil, "online"},
		{"said goodbye despite recent heartbeat", fresh, "offline", "offline"},
		{"heartbeat lapsed", stale, "online", "offline"},
		{"never seen", "", "online", "offline"},
		{"unparseable timestamp", "not-a-date", "online", "offline"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := pb.Record{"last_seen_at": c.lastSeen}
			if c.stored != nil {
				rec["status"] = c.stored
			}
			if got := recordToDevice(rec).Status; got != c.want {
				t.Fatalf("want %q, got %q", c.want, got)
			}
		})
	}
}
