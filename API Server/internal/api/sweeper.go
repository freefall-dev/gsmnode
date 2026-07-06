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
	cutoff := time.Now().UTC().Add(-s.cfg.MessageTTL).Format("2006-01-02 15:04:05.000Z")
	filter := `(status = "` + statusPending + `" || status = "` + statusProcessed +
		`") && updated < "` + cutoff + `"`

	res, err := s.pb.List(ctx, colMessages, pb.ListOptions{Filter: filter, PerPage: 200})
	if err != nil {
		log.Printf("expiry sweep failed: %v", err)
		return
	}

	expired := 0
	for _, rec := range res.Items {
		// Don't expire messages scheduled for the future.
		if asString(rec["schedule_at"]) != "" {
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
