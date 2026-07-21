package api

import (
	"context"
	"log"
	"time"

	"smsgateway/apiserver/internal/pb"
)

// sweepInterval is how often stale messages are checked.
const sweepInterval = 30 * time.Second

// StartExpiryWorker launches a background loop that fails messages/calls which
// no device picked up or reported on within the configured MessageTTL. Without
// it, a message targeting an offline device would stay Pending forever.
func (s *Server) StartExpiryWorker(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(sweepInterval)
		defer ticker.Stop()
		// Run once at startup so stale items clear promptly.
		s.expireStaleMessages(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.expireStaleMessages(ctx)
			}
		}
	}()
}

// expireStaleMessages marks Pending/Processed messages older than MessageTTL as
// Failed. "Pending" means no online device pulled it; "Processed" means a device
// pulled it but never reported a terminal state (e.g. it crashed).
func (s *Server) expireStaleMessages(ctx context.Context) {
	now := time.Now().UTC()
	cutoff := pbTime(now.Add(-s.cfg.MessageTTL))
	filter := `(status = "` + statusPending + `" || status = "` + statusProcessed +
		`") && updated < "` + cutoff + `"`

	res, err := s.pb.List(ctx, colMessages, pb.ListOptions{Filter: filter, PerPage: 200})
	if err != nil {
		log.Printf("expiry sweep failed: %v", err)
		return
	}

	expired := 0
	for _, rec := range res.Items {
		// A scheduled message sits Pending until it comes due, so `updated` says
		// nothing about whether a device has had its chance. Measure the TTL from
		// the scheduled time instead: skip while it is still in the future, and
		// for one TTL afterwards so the device has a window to pull it.
		if at, ok := scheduleTime(rec); ok && now.Before(at.Add(s.cfg.MessageTTL)) {
			continue
		}
		id := asString(rec["id"])
		const reason = "expired: no device processed the message within the timeout"
		if _, err := s.pb.Update(ctx, colMessages, id, pb.Record{
			"status": statusFailed,
			"error":  reason,
		}); err != nil {
			log.Printf("expire message %s: %v", id, err)
			continue
		}
		expired++
		s.dispatchWebhooks(asString(rec["owner"]), asString(rec["device"]), "sms:failed", map[string]any{
			"message_id": id,
			"type":       asString(rec["type"]),
			"status":     statusFailed,
			"error":      reason,
		})
	}
	if expired > 0 {
		log.Printf("expired %d stale message(s) older than %s", expired, s.cfg.MessageTTL)
	}
}
