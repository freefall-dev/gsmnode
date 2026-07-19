package api

import (
	"net/http"
	"strings"
	"time"

	"smsgateway/apiserver/internal/auth"
	"smsgateway/apiserver/internal/pb"
)

type deviceDTO struct {
	ID         string    `json:"id"`
	DeviceID   string    `json:"device_id"`
	Name       string    `json:"name"`
	Platform   string    `json:"platform"`
	AppVersion string    `json:"app_version"`
	Status     string    `json:"status"`
	LastSeenAt string    `json:"last_seen_at"`
	Sims       []simInfo `json:"sims"`
}

// simInfo describes one SIM active in a device, as reported by the phone.
type simInfo struct {
	Slot           int    `json:"slot"`
	SubscriptionID int    `json:"subscription_id"`
	Carrier        string `json:"carrier"`
	Number         string `json:"number"`
	DisplayName    string `json:"display_name"`
}

// parseSims decodes the devices.sims JSON field into a slice. It always returns
// a non-nil slice so the DTO serializes as [] rather than null.
func parseSims(v any) []simInfo {
	out := []simInfo{}
	arr, ok := v.([]any)
	if !ok {
		return out
	}
	for _, e := range arr {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, simInfo{
			Slot:           asInt(m["slot"]),
			SubscriptionID: asInt(m["subscription_id"]),
			Carrier:        asString(m["carrier"]),
			Number:         asString(m["number"]),
			DisplayName:    asString(m["display_name"]),
		})
	}
	return out
}

// onlineWindow is how recently a device must have pinged to count as online.
// The phone pings every ~60s, so this tolerates a couple of missed pings.
const onlineWindow = 3 * time.Minute

func recordToDevice(rec pb.Record) deviceDTO {
	lastSeen := asString(rec["last_seen_at"])
	status := "offline"
	if deviceOnline(lastSeen) {
		status = "online"
	}
	return deviceDTO{
		ID:         asString(rec["id"]),
		DeviceID:   asString(rec["device_id"]),
		Name:       asString(rec["name"]),
		Platform:   asString(rec["platform"]),
		AppVersion: asString(rec["app_version"]),
		Status:     status,
		LastSeenAt: lastSeen,
		Sims:       parseSims(rec["sims"]),
	}
}

// deviceOnline reports whether last_seen_at is within the online window. It
// accepts the PocketBase datetime format as well as RFC3339.
func deviceOnline(lastSeenAt string) bool {
	if lastSeenAt == "" {
		return false
	}
	for _, layout := range []string{
		"2006-01-02 15:04:05.999Z07:00",
		"2006-01-02 15:04:05.999Z",
		time.RFC3339,
	} {
		if t, err := time.Parse(layout, lastSeenAt); err == nil {
			return time.Since(t) < onlineWindow
		}
	}
	return false
}

// handleListDevices returns the authenticated user's devices.
func (s *Server) handleListDevices(w http.ResponseWriter, r *http.Request) {
	uid, _ := userFromCtx(r.Context())
	res, err := s.pb.List(r.Context(), colDevices, pb.ListOptions{
		Filter:  "owner = " + pbQuote(uid),
		Sort:    "-created",
		PerPage: 200,
	})
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	out := make([]deviceDTO, 0, len(res.Items))
	for _, rec := range res.Items {
		out = append(out, recordToDevice(rec))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out, "total": res.TotalItems})
}

// handleDeleteDevice removes a device owned by the user.
func (s *Server) handleDeleteDevice(w http.ResponseWriter, r *http.Request) {
	uid, _ := userFromCtx(r.Context())
	id := r.PathValue("id")

	rec, err := s.pb.GetOne(r.Context(), colDevices, id)
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	if asString(rec["owner"]) != uid {
		writeError(w, http.StatusForbidden, "not your device")
		return
	}
	if err := s.pb.Delete(r.Context(), colDevices, id); err != nil {
		writeUpstreamError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type registerDeviceRequest struct {
	DeviceID   string `json:"device_id"`
	Name       string `json:"name"`
	Platform   string `json:"platform"`
	AppVersion string `json:"app_version"`
	PushToken  string `json:"push_token"`
}

type registerDeviceResponse struct {
	deviceDTO
	AuthToken string `json:"auth_token"`
}

// handleRegisterDevice registers (or re-registers) a mobile device for the
// authenticated user and returns an opaque device token. Re-registering with
// the same device_id rotates the token and updates metadata.
func (s *Server) handleRegisterDevice(w http.ResponseWriter, r *http.Request) {
	uid, _ := userFromCtx(r.Context())

	var req registerDeviceRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.DeviceID = strings.TrimSpace(req.DeviceID)
	if req.DeviceID == "" {
		writeError(w, http.StatusBadRequest, "device_id is required")
		return
	}
	if req.Platform == "" {
		req.Platform = "android"
	}

	token, err := auth.NewDeviceToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not generate token")
		return
	}

	now := time.Now().UTC().Format("2006-01-02 15:04:05.000Z")
	fields := pb.Record{
		"device_id":    req.DeviceID,
		"name":         req.Name,
		"platform":     req.Platform,
		"app_version":  req.AppVersion,
		"push_token":   req.PushToken,
		"status":       "online",
		"last_seen_at": now,
		"auth_token":   token,
		"owner":        uid,
	}

	// Upsert by (owner, device_id).
	existing, err := s.pb.FindFirst(r.Context(), colDevices,
		"owner = "+pbQuote(uid)+" && device_id = "+pbQuote(req.DeviceID), "")
	if err != nil {
		writeUpstreamError(w, err)
		return
	}

	var rec pb.Record
	if existing != nil {
		rec, err = s.pb.Update(r.Context(), colDevices, asString(existing["id"]), fields)
	} else {
		rec, err = s.pb.Create(r.Context(), colDevices, fields)
	}
	if err != nil {
		writeUpstreamError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, registerDeviceResponse{
		deviceDTO: recordToDevice(rec),
		AuthToken: token,
	})
}

type pingRequest struct {
	Sims []simInfo `json:"sims"`
}

// handlePing updates a device heartbeat (last_seen_at + online status). The body
// is optional; when it carries a "sims" list, the device's advertised SIM slots
// are refreshed so callers know which slots they can target.
func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	device := deviceFromCtx(r.Context())
	now := time.Now().UTC().Format("2006-01-02 15:04:05.000Z")
	fields := pb.Record{
		"status":       "online",
		"last_seen_at": now,
	}
	// Best-effort: an empty/absent body just means a plain heartbeat.
	var req pingRequest
	if decodeJSON(r, &req) == nil && req.Sims != nil {
		fields["sims"] = req.Sims
	}
	_, err := s.pb.Update(r.Context(), colDevices, asString(device["id"]), fields)
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}
