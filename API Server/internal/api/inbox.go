package api

import (
	"net/http"
	"strings"
	"time"

	"smsgateway/apiserver/internal/pb"
)

type inboxDTO struct {
	ID          string `json:"id"`
	DeviceID    string `json:"device_id"`
	PhoneNumber string `json:"phone_number"`
	Message     string `json:"message"`
	ReceivedAt  string `json:"received_at"`
	// 0-based SIM slot the message arrived on, omitted when the device couldn't
	// attribute it.
	SimSlot *int `json:"sim_slot,omitempty"`
}

func recordToInbox(rec pb.Record) inboxDTO {
	return inboxDTO{
		ID:          asString(rec["id"]),
		DeviceID:    asString(rec["device"]),
		PhoneNumber: asString(rec["phone_number"]),
		Message:     asString(rec["message"]),
		ReceivedAt:  asString(rec["received_at"]),
		SimSlot:     unpackSlot(rec["sim_slot"]),
	}
}

// handleListInbox lists received SMS for the authenticated user.
func (s *Server) handleListInbox(w http.ResponseWriter, r *http.Request) {
	uid, _ := userFromCtx(r.Context())
	res, err := s.pb.List(r.Context(), colInbox, pb.ListOptions{
		Filter:  "owner = " + pbQuote(uid),
		Sort:    "-received_at",
		Page:    queryInt(r, "page", 1),
		PerPage: clampPerPage(queryInt(r, "per_page", 50)),
	})
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	out := make([]inboxDTO, 0, len(res.Items))
	for _, rec := range res.Items {
		out = append(out, recordToInbox(rec))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": out, "total": res.TotalItems, "page": res.Page,
	})
}

type receiveSMSRequest struct {
	PhoneNumber string `json:"phone_number"`
	Message     string `json:"message"`
	ReceivedAt  string `json:"received_at"`
	SimSlot     *int   `json:"sim_slot"`
}

// handleReceiveSMS records an incoming SMS reported by a device and fires
// sms:received webhooks.
func (s *Server) handleReceiveSMS(w http.ResponseWriter, r *http.Request) {
	device := deviceFromCtx(r.Context())

	var req receiveSMSRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.PhoneNumber) == "" {
		writeError(w, http.StatusBadRequest, "phone_number is required")
		return
	}
	receivedAt := req.ReceivedAt
	if receivedAt == "" {
		receivedAt = time.Now().UTC().Format("2006-01-02 15:04:05.000Z")
	}

	fields := pb.Record{
		"device":       asString(device["id"]),
		"owner":        asString(device["owner"]),
		"phone_number": req.PhoneNumber,
		"message":      req.Message,
		"received_at":  receivedAt,
	}
	if req.SimSlot != nil {
		fields["sim_slot"] = packSlot(*req.SimSlot)
	}
	rec, err := s.pb.Create(r.Context(), colInbox, fields)
	if err != nil {
		writeUpstreamError(w, err)
		return
	}

	payload := map[string]any{
		"inbox_id":     asString(rec["id"]),
		"phone_number": req.PhoneNumber,
		"message":      req.Message,
		"received_at":  receivedAt,
	}
	if req.SimSlot != nil {
		payload["sim_slot"] = *req.SimSlot
	}
	s.dispatchWebhooks(asString(device["owner"]), asString(device["id"]), "sms:received", payload)

	writeJSON(w, http.StatusCreated, recordToInbox(rec))
}
