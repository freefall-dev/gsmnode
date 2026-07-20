package api

import (
	"net/http"
	"strings"
	"time"

	"smsgateway/apiserver/internal/pb"
)

// Inbound message kinds.
const (
	inTypeSMS  = "sms"
	inTypeData = "data"
	inTypeMMS  = "mms"
)

var validInboxTypes = map[string]bool{
	inTypeSMS: true, inTypeData: true, inTypeMMS: true,
}

type inboxDTO struct {
	ID          string `json:"id"`
	DeviceID    string `json:"device_id"`
	Type        string `json:"type"`
	PhoneNumber string `json:"phone_number"`
	Message     string `json:"message"`
	ReceivedAt  string `json:"received_at"`
	// 0-based SIM slot the message arrived on, omitted when the device couldn't
	// attribute it.
	SimSlot *int `json:"sim_slot,omitempty"`
	// Data SMS.
	DataPayload string `json:"data_payload,omitempty"`
	DataPort    *int   `json:"data_port,omitempty"`
	// MMS.
	Subject     string       `json:"subject,omitempty"`
	Attachments []attachment `json:"attachments,omitempty"`
	Encrypted   bool         `json:"encrypted,omitempty"`
}

func recordToInbox(rec pb.Record) inboxDTO {
	inType := asString(rec["type"])
	if inType == "" {
		inType = inTypeSMS
	}
	var dataPort *int
	if p := asInt(rec["data_port"]); p != 0 || asString(rec["data_payload"]) != "" {
		dataPort = &p
	}
	return inboxDTO{
		ID:          asString(rec["id"]),
		DeviceID:    asString(rec["device"]),
		Type:        inType,
		PhoneNumber: asString(rec["phone_number"]),
		Message:     asString(rec["message"]),
		ReceivedAt:  asString(rec["received_at"]),
		SimSlot:     unpackSlot(rec["sim_slot"]),
		DataPayload: asString(rec["data_payload"]),
		DataPort:    dataPort,
		Subject:     asString(rec["subject"]),
		Attachments: asAttachments(rec["attachments"]),
		Encrypted:   asBool(rec["encrypted"]),
	}
}

// handleListInbox lists received messages for the authenticated user, optionally
// filtered by ?type=sms|data|mms.
func (s *Server) handleListInbox(w http.ResponseWriter, r *http.Request) {
	uid, _ := userFromCtx(r.Context())
	filters := []string{"owner = " + pbQuote(uid)}
	if t := r.URL.Query().Get("type"); t != "" {
		filters = append(filters, "type = "+pbQuote(t))
	}
	res, err := s.pb.List(r.Context(), colInbox, pb.ListOptions{
		Filter:  strings.Join(filters, " && "),
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
	Type        string `json:"type"` // sms (default) | data | mms
	PhoneNumber string `json:"phone_number"`
	Message     string `json:"message"`
	ReceivedAt  string `json:"received_at"`
	SimSlot     *int   `json:"sim_slot"`
	// Data SMS.
	DataPayload string `json:"data_payload"`
	DataPort    *int   `json:"data_port"`
	// MMS.
	Subject     string       `json:"subject"`
	Attachments []attachment `json:"attachments"`
	Encrypted   bool         `json:"encrypted"`
}

// handleReceiveSMS records an incoming SMS, data SMS, or MMS reported by a
// device and fires the matching webhook (sms:received, sms:data-received,
// mms:received, and mms:downloaded once attachments are present).
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
	inType := req.Type
	if inType == "" {
		inType = inTypeSMS
	}
	if !validInboxTypes[inType] {
		writeError(w, http.StatusBadRequest, "invalid type (want sms, data, or mms)")
		return
	}
	receivedAt := req.ReceivedAt
	if receivedAt == "" {
		receivedAt = time.Now().UTC().Format("2006-01-02 15:04:05.000Z")
	}

	fields := pb.Record{
		"device":       asString(device["id"]),
		"owner":        asString(device["owner"]),
		"type":         inType,
		"phone_number": req.PhoneNumber,
		"message":      req.Message,
		"received_at":  receivedAt,
		"encrypted":    req.Encrypted,
	}
	if req.SimSlot != nil {
		fields["sim_slot"] = packSlot(*req.SimSlot)
	}
	if inType == inTypeData {
		fields["data_payload"] = req.DataPayload
		if req.DataPort != nil {
			fields["data_port"] = *req.DataPort
		}
	}
	if inType == inTypeMMS {
		fields["subject"] = req.Subject
		if len(req.Attachments) > 0 {
			fields["attachments"] = req.Attachments
		}
	}
	rec, err := s.pb.Create(r.Context(), colInbox, fields)
	if err != nil {
		writeUpstreamError(w, err)
		return
	}

	owner, devID := asString(device["owner"]), asString(device["id"])
	payload := map[string]any{
		"inbox_id":     asString(rec["id"]),
		"type":         inType,
		"phone_number": req.PhoneNumber,
		"message":      req.Message,
		"received_at":  receivedAt,
		"encrypted":    req.Encrypted,
	}
	if req.SimSlot != nil {
		payload["sim_slot"] = *req.SimSlot
	}
	switch inType {
	case inTypeData:
		payload["data_payload"] = req.DataPayload
		if req.DataPort != nil {
			payload["data_port"] = *req.DataPort
		}
		s.dispatchWebhooks(owner, devID, "sms:data-received", payload)
	case inTypeMMS:
		payload["subject"] = req.Subject
		payload["attachments"] = req.Attachments
		// mms:received is the arrival notification; mms:downloaded fires once the
		// device has pulled the full body + attachments from the carrier.
		s.dispatchWebhooks(owner, devID, "mms:received", payload)
		if len(req.Attachments) > 0 {
			s.dispatchWebhooks(owner, devID, "mms:downloaded", payload)
		}
	default:
		s.dispatchWebhooks(owner, devID, "sms:received", payload)
	}

	writeJSON(w, http.StatusCreated, recordToInbox(rec))
}
