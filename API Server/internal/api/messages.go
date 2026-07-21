package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"smsgateway/apiserver/internal/pb"
)

// Message lifecycle states.
const (
	statusPending   = "Pending"   // enqueued, not yet picked up by a device
	statusProcessed = "Processed" // delivered to a device for sending
	statusSent      = "Sent"      // device handed it to the SIM/radio
	statusDelivered = "Delivered" // delivery report received
	statusFailed    = "Failed"    // sending failed
)

var validReportStates = map[string]bool{
	statusSent: true, statusDelivered: true, statusFailed: true, statusProcessed: true,
}

// Message kinds.
const (
	msgTypeSMS  = "sms"
	msgTypeCall = "call"
	msgTypeData = "data" // binary payload sent to a destination port
	msgTypeMMS  = "mms"  // multimedia message with subject + attachments
)

var validSendTypes = map[string]bool{
	msgTypeSMS: true, msgTypeData: true, msgTypeMMS: true,
}

// attachment is one part of an MMS: inline base64 data (outbound/inbound) or a
// URL the client can fetch (inbound, when the device reports a link instead).
type attachment struct {
	Filename    string `json:"filename,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	Data        string `json:"data,omitempty"` // base64-encoded bytes
	URL         string `json:"url,omitempty"`
}

type messageDTO struct {
	ID           string   `json:"id"`
	Type         string   `json:"type"`
	PhoneNumbers []string `json:"phone_numbers"`
	TextMessage  string   `json:"text_message"`
	DeviceID     string   `json:"device_id,omitempty"`
	// SimNumber is the 0-based SIM slot to send on; nil means the device's
	// default SIM. A pointer so slot 0 is distinguishable from "unset".
	SimNumber *int   `json:"sim_number,omitempty"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
	// Data SMS.
	DataPayload string `json:"data_payload,omitempty"`
	DataPort    *int   `json:"data_port,omitempty"`
	// MMS.
	Subject     string       `json:"subject,omitempty"`
	Attachments []attachment `json:"attachments,omitempty"`
	// Encrypted marks phone_numbers + text_message as E2E ciphertext.
	Encrypted  bool   `json:"encrypted,omitempty"`
	ScheduleAt string `json:"schedule_at,omitempty"`
	CreatedAt  string `json:"created_at"`
}

func recordToMessage(rec pb.Record) messageDTO {
	msgType := asString(rec["type"])
	if msgType == "" {
		msgType = msgTypeSMS
	}
	var dataPort *int
	if p := asInt(rec["data_port"]); p != 0 || asString(rec["data_payload"]) != "" {
		dataPort = &p
	}
	return messageDTO{
		ID:           asString(rec["id"]),
		Type:         msgType,
		PhoneNumbers: asStringSlice(rec["phone_numbers"]),
		TextMessage:  asString(rec["text_message"]),
		DeviceID:     asString(rec["device"]),
		SimNumber:    unpackSlot(rec["sim_number"]),
		Status:       asString(rec["status"]),
		Error:        asString(rec["error"]),
		DataPayload:  asString(rec["data_payload"]),
		DataPort:     dataPort,
		Subject:      asString(rec["subject"]),
		Attachments:  asAttachments(rec["attachments"]),
		Encrypted:    asBool(rec["encrypted"]),
		ScheduleAt:   asString(rec["schedule_at"]),
		CreatedAt:    asString(rec["created"]),
	}
}

type enqueueRequest struct {
	// Type selects the message kind: "sms" (default), "data", or "mms".
	Type         string   `json:"type"`
	PhoneNumbers []string `json:"phone_numbers"`
	TextMessage  string   `json:"text_message"`
	DeviceID     string   `json:"device_id"`
	// SimNumber is the 0-based SIM slot to send on. A pointer so slot 0 can be
	// selected explicitly; omit it (nil) to use the device's default SIM.
	SimNumber *int `json:"sim_number"`
	// Data SMS: base64 payload + destination port.
	DataPayload string `json:"data_payload"`
	DataPort    *int   `json:"data_port"`
	// MMS: subject + attachments.
	Subject     string       `json:"subject"`
	Attachments []attachment `json:"attachments"`
	// Encrypted marks phone_numbers + text_message as already-ciphertext (E2E);
	// the server stores and relays them without inspecting them.
	Encrypted  bool   `json:"encrypted"`
	ScheduleAt string `json:"schedule_at"`
}

// handleEnqueueMessage queues an outbound SMS, data SMS, or MMS for one of the
// user's devices, selected by the request's "type" field.
func (s *Server) handleEnqueueMessage(w http.ResponseWriter, r *http.Request) {
	uid, _ := userFromCtx(r.Context())

	var req enqueueRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	msgType := req.Type
	if msgType == "" {
		msgType = msgTypeSMS
	}
	if !validSendTypes[msgType] {
		writeError(w, http.StatusBadRequest, "invalid type (want sms, data, or mms)")
		return
	}
	phones := cleanPhones(req.PhoneNumbers)
	if len(phones) == 0 {
		writeError(w, http.StatusBadRequest, "at least one phone number is required")
		return
	}

	// Per-type payload validation. Encrypted payloads are opaque, so the
	// "required" checks only ensure the relevant field is present.
	switch msgType {
	case msgTypeSMS:
		if strings.TrimSpace(req.TextMessage) == "" {
			writeError(w, http.StatusBadRequest, "text_message is required")
			return
		}
	case msgTypeData:
		if strings.TrimSpace(req.DataPayload) == "" {
			writeError(w, http.StatusBadRequest, "data_payload (base64) is required for data SMS")
			return
		}
	case msgTypeMMS:
		if strings.TrimSpace(req.TextMessage) == "" && len(req.Attachments) == 0 {
			writeError(w, http.StatusBadRequest, "mms requires text_message or at least one attachment")
			return
		}
	}

	// Resolve the target device: explicit device_id, else the user's first
	// online device.
	deviceRecID, err := s.resolveDevice(r.Context(), uid, req.DeviceID)
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	if deviceRecID == "" {
		writeError(w, http.StatusBadRequest, "no device available; register a device first")
		return
	}

	fields := pb.Record{
		"phone_numbers": phones,
		"text_message":  req.TextMessage,
		"type":          msgType,
		"device":        deviceRecID,
		"owner":         uid,
		"status":        statusPending,
		"encrypted":     req.Encrypted,
	}
	if req.SimNumber != nil {
		fields["sim_number"] = packSlot(*req.SimNumber)
	}
	if msgType == msgTypeData {
		fields["data_payload"] = req.DataPayload
		if req.DataPort != nil {
			fields["data_port"] = *req.DataPort
		}
	}
	if msgType == msgTypeMMS {
		fields["subject"] = req.Subject
		if len(req.Attachments) > 0 {
			fields["attachments"] = req.Attachments
		}
	}
	if req.ScheduleAt != "" {
		fields["schedule_at"] = req.ScheduleAt
	}

	rec, err := s.pb.Create(r.Context(), colMessages, fields)
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, recordToMessage(rec))
}

// resolveDevice returns the PocketBase device record id to target. requested may
// be either the internal record id or the client-facing device_id.
func (s *Server) resolveDevice(ctx context.Context, uid, requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested != "" {
		dev, err := s.pb.FindFirst(ctx, colDevices,
			"owner = "+pbQuote(uid)+" && (id = "+pbQuote(requested)+" || device_id = "+pbQuote(requested)+")", "")
		if err != nil {
			return "", err
		}
		if dev == nil {
			return "", nil
		}
		return asString(dev["id"]), nil
	}
	dev, err := s.pb.FindFirst(ctx, colDevices,
		"owner = "+pbQuote(uid), "-last_seen_at")
	if err != nil || dev == nil {
		return "", err
	}
	return asString(dev["id"]), nil
}

// createSMS enqueues a plain outbound SMS owned by ownerID for one of that user's
// devices — the same path as POST /api/messages, used by server-side producers
// such as the email-to-sms plugin. It returns ErrNoDevice when the owner has no
// device registered.
func (s *Server) createSMS(ctx context.Context, ownerID string, phones []string, text string) (messageDTO, error) {
	phones = cleanPhones(phones)
	if len(phones) == 0 {
		return messageDTO{}, errors.New("at least one phone number is required")
	}
	if strings.TrimSpace(text) == "" {
		return messageDTO{}, errors.New("text_message is required")
	}
	deviceRecID, err := s.resolveDevice(ctx, ownerID, "")
	if err != nil {
		return messageDTO{}, err
	}
	if deviceRecID == "" {
		return messageDTO{}, ErrNoDevice
	}
	rec, err := s.pb.Create(ctx, colMessages, pb.Record{
		"phone_numbers": phones,
		"text_message":  text,
		"type":          msgTypeSMS,
		"device":        deviceRecID,
		"owner":         ownerID,
		"status":        statusPending,
		"encrypted":     false,
	})
	if err != nil {
		return messageDTO{}, err
	}
	return recordToMessage(rec), nil
}

// ErrNoDevice is returned by createSMS when the owner has no registered device.
var ErrNoDevice = errors.New("no device available; register a device first")

// handleListMessages lists the user's messages with optional filters.
func (s *Server) handleListMessages(w http.ResponseWriter, r *http.Request) {
	uid, _ := userFromCtx(r.Context())

	filters := []string{"owner = " + pbQuote(uid)}
	if st := r.URL.Query().Get("status"); st != "" {
		filters = append(filters, "status = "+pbQuote(st))
	}
	if dev := r.URL.Query().Get("device_id"); dev != "" {
		filters = append(filters, "device = "+pbQuote(dev))
	}
	if t := r.URL.Query().Get("type"); t != "" {
		filters = append(filters, "type = "+pbQuote(t))
	}

	res, err := s.pb.List(r.Context(), colMessages, pb.ListOptions{
		Filter:  strings.Join(filters, " && "),
		Sort:    "-created",
		Page:    queryInt(r, "page", 1),
		PerPage: clampPerPage(queryInt(r, "per_page", 50)),
	})
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	out := make([]messageDTO, 0, len(res.Items))
	for _, rec := range res.Items {
		out = append(out, recordToMessage(rec))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": out, "total": res.TotalItems, "page": res.Page,
	})
}

// handleGetMessage returns a single message's state.
func (s *Server) handleGetMessage(w http.ResponseWriter, r *http.Request) {
	uid, _ := userFromCtx(r.Context())
	rec, err := s.pb.GetOne(r.Context(), colMessages, r.PathValue("id"))
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	if asString(rec["owner"]) != uid {
		writeError(w, http.StatusForbidden, "not your message")
		return
	}
	writeJSON(w, http.StatusOK, recordToMessage(rec))
}

// handlePullMessages returns pending messages for the calling device and marks
// them Processed so they are not handed out twice.
func (s *Server) handlePullMessages(w http.ResponseWriter, r *http.Request) {
	device := deviceFromCtx(r.Context())
	deviceID := asString(device["id"])

	res, err := s.pb.List(r.Context(), colMessages, pb.ListOptions{
		Filter:  "device = " + pbQuote(deviceID) + " && status = " + pbQuote(statusPending),
		Sort:    "created",
		PerPage: clampPerPage(queryInt(r, "limit", 20)),
	})
	if err != nil {
		writeUpstreamError(w, err)
		return
	}

	out := make([]messageDTO, 0, len(res.Items))
	for _, rec := range res.Items {
		id := asString(rec["id"])
		updated, uerr := s.pb.Update(r.Context(), colMessages, id, pb.Record{"status": statusProcessed})
		if uerr != nil {
			// Skip a message we couldn't claim rather than failing the pull.
			continue
		}
		out = append(out, recordToMessage(updated))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

type reportRequest struct {
	Status string `json:"status"`
	Error  string `json:"error"`
}

// handleReportMessage records a delivery state reported by the device and fires
// the matching webhooks.
func (s *Server) handleReportMessage(w http.ResponseWriter, r *http.Request) {
	device := deviceFromCtx(r.Context())
	id := r.PathValue("id")

	var req reportRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !validReportStates[req.Status] {
		writeError(w, http.StatusBadRequest, "invalid status")
		return
	}

	rec, err := s.pb.GetOne(r.Context(), colMessages, id)
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	if asString(rec["device"]) != asString(device["id"]) {
		writeError(w, http.StatusForbidden, "message not assigned to this device")
		return
	}

	fields := pb.Record{"status": req.Status, "error": req.Error}
	now := time.Now().UTC().Format("2006-01-02 15:04:05.000Z")
	switch req.Status {
	case statusSent:
		fields["sent_at"] = now
	case statusDelivered:
		fields["delivered_at"] = now
	}
	updated, err := s.pb.Update(r.Context(), colMessages, id, fields)
	if err != nil {
		writeUpstreamError(w, err)
		return
	}

	// Dispatch webhooks for terminal/notable states.
	if event := eventForStatus(req.Status); event != "" {
		s.dispatchWebhooks(asString(device["owner"]), asString(device["id"]), event, map[string]any{
			"message_id":    id,
			"type":          asString(updated["type"]),
			"phone_numbers": asStringSlice(updated["phone_numbers"]),
			"status":        req.Status,
			"error":         req.Error,
			"encrypted":     asBool(updated["encrypted"]),
		})
	}

	writeJSON(w, http.StatusOK, recordToMessage(updated))
}

func eventForStatus(status string) string {
	switch status {
	case statusSent:
		return "sms:sent"
	case statusDelivered:
		return "sms:delivered"
	case statusFailed:
		return "sms:failed"
	default:
		return ""
	}
}

func cleanPhones(in []string) []string {
	out := make([]string, 0, len(in))
	for _, p := range in {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func clampPerPage(n int) int {
	if n <= 0 {
		return 50
	}
	if n > 200 {
		return 200
	}
	return n
}
