package api

import (
	"net/http"
	"strings"

	"smsgateway/apiserver/internal/pb"
)

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
