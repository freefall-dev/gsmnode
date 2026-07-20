package api

import (
	"net/http"
	"strings"
	"time"

	"smsgateway/apiserver/internal/pb"
)

// Call directions and the webhook event each maps to.
const (
	callIncoming = "incoming"
	callOutgoing = "outgoing"
)

var validCallDirections = map[string]bool{callIncoming: true, callOutgoing: true}

var validCallStatuses = map[string]bool{
	"ringing": true, "missed": true, "answered": true,
	"completed": true, "rejected": true, "failed": true,
}

type callDTO struct {
	ID          string `json:"id"`
	DeviceID    string `json:"device_id"`
	PhoneNumber string `json:"phone_number"`
	Direction   string `json:"direction"`
	Status      string `json:"status"`
	SimSlot     *int   `json:"sim_slot,omitempty"`
	Duration    *int   `json:"duration,omitempty"`
	StartedAt   string `json:"started_at,omitempty"`
	CreatedAt   string `json:"created_at"`
}

func recordToCall(rec pb.Record) callDTO {
	var dur *int
	if d := asInt(rec["duration"]); d > 0 {
		dur = &d
	}
	return callDTO{
		ID:          asString(rec["id"]),
		DeviceID:    asString(rec["device"]),
		PhoneNumber: asString(rec["phone_number"]),
		Direction:   asString(rec["direction"]),
		Status:      asString(rec["status"]),
		SimSlot:     unpackSlot(rec["sim_slot"]),
		Duration:    dur,
		StartedAt:   asString(rec["started_at"]),
		CreatedAt:   asString(rec["created"]),
	}
}

type enqueueCallRequest struct {
	PhoneNumber string `json:"phone_number"`
	DeviceID    string `json:"device_id"`
}

// handleEnqueueCall queues an outbound phone call for one of the user's devices.
// A call is stored as a message with type "call"; the device pulls it through
// the same mobile pipeline as SMS and places the call natively.
func (s *Server) handleEnqueueCall(w http.ResponseWriter, r *http.Request) {
	uid, _ := userFromCtx(r.Context())

	var req enqueueCallRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	phone := strings.TrimSpace(req.PhoneNumber)
	if phone == "" {
		writeError(w, http.StatusBadRequest, "phone_number is required")
		return
	}

	deviceRecID, err := s.resolveDevice(r, uid, req.DeviceID)
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	if deviceRecID == "" {
		writeError(w, http.StatusBadRequest, "no device available; register a device first")
		return
	}

	rec, err := s.pb.Create(r.Context(), colMessages, pb.Record{
		"phone_numbers": []string{phone},
		"type":          msgTypeCall,
		"device":        deviceRecID,
		"owner":         uid,
		"status":        statusPending,
	})
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, recordToMessage(rec))
}

// handleListCalls lists the user's call log (incoming + outgoing), optionally
// filtered by ?direction=incoming|outgoing.
func (s *Server) handleListCalls(w http.ResponseWriter, r *http.Request) {
	uid, _ := userFromCtx(r.Context())
	filters := []string{"owner = " + pbQuote(uid)}
	if d := r.URL.Query().Get("direction"); d != "" {
		filters = append(filters, "direction = "+pbQuote(d))
	}
	res, err := s.pb.List(r.Context(), colCalls, pb.ListOptions{
		Filter:  strings.Join(filters, " && "),
		Sort:    "-created",
		Page:    queryInt(r, "page", 1),
		PerPage: clampPerPage(queryInt(r, "per_page", 50)),
	})
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	out := make([]callDTO, 0, len(res.Items))
	for _, rec := range res.Items {
		out = append(out, recordToCall(rec))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": out, "total": res.TotalItems, "page": res.Page,
	})
}

type reportCallRequest struct {
	PhoneNumber string `json:"phone_number"`
	Direction   string `json:"direction"`
	Status      string `json:"status"`
	SimSlot     *int   `json:"sim_slot"`
	Duration    *int   `json:"duration"`
	StartedAt   string `json:"started_at"`
}

// handleReportCall records a call event reported by a device and fires the
// matching webhook: call:received (incoming), call:sent (outgoing), or
// call:failed (status failed/missed/rejected).
func (s *Server) handleReportCall(w http.ResponseWriter, r *http.Request) {
	device := deviceFromCtx(r.Context())

	var req reportCallRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.PhoneNumber) == "" {
		writeError(w, http.StatusBadRequest, "phone_number is required")
		return
	}
	if req.Direction == "" {
		req.Direction = callIncoming
	}
	if !validCallDirections[req.Direction] {
		writeError(w, http.StatusBadRequest, "invalid direction (want incoming or outgoing)")
		return
	}
	if req.Status != "" && !validCallStatuses[req.Status] {
		writeError(w, http.StatusBadRequest, "invalid status")
		return
	}
	startedAt := req.StartedAt
	if startedAt == "" {
		startedAt = time.Now().UTC().Format("2006-01-02 15:04:05.000Z")
	}

	fields := pb.Record{
		"device":       asString(device["id"]),
		"owner":        asString(device["owner"]),
		"phone_number": req.PhoneNumber,
		"direction":    req.Direction,
		"status":       req.Status,
		"started_at":   startedAt,
	}
	if req.SimSlot != nil {
		fields["sim_slot"] = packSlot(*req.SimSlot)
	}
	if req.Duration != nil {
		fields["duration"] = *req.Duration
	}
	rec, err := s.pb.Create(r.Context(), colCalls, fields)
	if err != nil {
		writeUpstreamError(w, err)
		return
	}

	owner, devID := asString(device["owner"]), asString(device["id"])
	payload := map[string]any{
		"call_id":      asString(rec["id"]),
		"phone_number": req.PhoneNumber,
		"direction":    req.Direction,
		"status":       req.Status,
		"started_at":   startedAt,
	}
	if req.SimSlot != nil {
		payload["sim_slot"] = *req.SimSlot
	}
	if req.Duration != nil {
		payload["duration"] = *req.Duration
	}
	s.dispatchWebhooks(owner, devID, eventForCall(req.Direction, req.Status), payload)

	writeJSON(w, http.StatusCreated, recordToCall(rec))
}

// eventForCall maps a reported call to its webhook event. A failed/missed/
// rejected call fires call:failed regardless of direction; otherwise the
// direction decides (incoming → call:received, outgoing → call:sent).
func eventForCall(direction, status string) string {
	switch status {
	case "failed", "missed", "rejected":
		return "call:failed"
	}
	if direction == callOutgoing {
		return "call:sent"
	}
	return "call:received"
}
