package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"smsgateway/apiserver/internal/bootstrap"
	"smsgateway/apiserver/internal/pb"
)

// validWebhookEvents gates webhook registration. It is derived from the schema's
// canonical list rather than restated here: PocketBase rejects any value outside
// the collection's select options, and an event missing from this map is
// silently unsubscribable even though the server dispatches it.
var validWebhookEvents = func() map[string]bool {
	m := make(map[string]bool, len(bootstrap.WebhookEvents))
	for _, e := range bootstrap.WebhookEvents {
		m[e] = true
	}
	return m
}()

type webhookDTO struct {
	ID       string `json:"id"`
	Event    string `json:"event"`
	URL      string `json:"url"`
	DeviceID string `json:"device_id,omitempty"`
}

func recordToWebhook(rec pb.Record) webhookDTO {
	return webhookDTO{
		ID:       asString(rec["id"]),
		Event:    asString(rec["event"]),
		URL:      asString(rec["url"]),
		DeviceID: asString(rec["device"]),
	}
}

// handleListWebhooks lists the user's registered webhooks.
func (s *Server) handleListWebhooks(w http.ResponseWriter, r *http.Request) {
	uid, _ := userFromCtx(r.Context())
	res, err := s.pb.List(r.Context(), colWebhooks, pb.ListOptions{
		Filter:  "owner = " + pbQuote(uid),
		Sort:    "-created",
		PerPage: 200,
	})
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	out := make([]webhookDTO, 0, len(res.Items))
	for _, rec := range res.Items {
		out = append(out, recordToWebhook(rec))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out, "total": res.TotalItems})
}

type createWebhookRequest struct {
	Event    string `json:"event"`
	URL      string `json:"url"`
	DeviceID string `json:"device_id"`
}

// handleCreateWebhook registers a webhook for the user.
func (s *Server) handleCreateWebhook(w http.ResponseWriter, r *http.Request) {
	uid, _ := userFromCtx(r.Context())

	var req createWebhookRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !validWebhookEvents[req.Event] {
		writeError(w, http.StatusBadRequest, "invalid event")
		return
	}
	if req.URL == "" {
		writeError(w, http.StatusBadRequest, "url is required")
		return
	}

	fields := pb.Record{"owner": uid, "event": req.Event, "url": req.URL}
	if req.DeviceID != "" {
		fields["device"] = req.DeviceID
	}
	rec, err := s.pb.Create(r.Context(), colWebhooks, fields)
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, recordToWebhook(rec))
}

// handleDeleteWebhook removes a webhook owned by the user.
func (s *Server) handleDeleteWebhook(w http.ResponseWriter, r *http.Request) {
	uid, _ := userFromCtx(r.Context())
	id := r.PathValue("id")

	rec, err := s.pb.GetOne(r.Context(), colWebhooks, id)
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	if asString(rec["owner"]) != uid {
		writeError(w, http.StatusForbidden, "not your webhook")
		return
	}
	if err := s.pb.Delete(r.Context(), colWebhooks, id); err != nil {
		writeUpstreamError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// dispatchWebhooks delivers an event to every matching webhook for the owner.
// Delivery is best-effort and runs in the background so it never blocks the
// device/client request.
func (s *Server) dispatchWebhooks(owner, deviceID, event string, payload map[string]any) {
	if owner == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		res, err := s.pb.List(ctx, colWebhooks, pb.ListOptions{
			Filter:  "owner = " + pbQuote(owner) + " && event = " + pbQuote(event),
			PerPage: 200,
		})
		if err != nil {
			log.Printf("webhook lookup failed for %s/%s: %v", owner, event, err)
			return
		}

		body := map[string]any{
			"event":      event,
			"device_id":  deviceID,
			"payload":    payload,
			"created_at": time.Now().UTC().Format(time.RFC3339),
		}
		raw, _ := json.Marshal(body)

		for _, hook := range res.Items {
			// A webhook scoped to a specific device only fires for that device.
			if dev := asString(hook["device"]); dev != "" && dev != deviceID {
				continue
			}
			deliverWebhook(ctx, asString(hook["url"]), raw)
		}
	}()
}

func deliverWebhook(ctx context.Context, url string, body []byte) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		log.Printf("webhook build request %s: %v", url, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "gsmnode-api/1.0")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("webhook delivery to %s failed: %v", url, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		log.Printf("webhook %s returned %d", url, resp.StatusCode)
	}
}
