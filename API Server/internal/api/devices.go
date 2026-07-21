package api

import (
	"context"
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

	// Owner identity, filled in only for the widened (scope=all) listing — in
	// the default "my devices" view the caller already knows whose they are.
	OwnerEmail string `json:"owner_email,omitempty"`
	OwnerName  string `json:"owner_name,omitempty"`
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
	// A device that said goodbye is offline immediately, whatever the clock
	// says: waiting out onlineWindow after an orderly stop showed a gateway as
	// online for minutes after it had stopped routing. Every ping and every
	// registration writes "online" back, so the stored field only ever holds
	// "offline" between a deliberate stop and the next contact.
	if deviceOnline(lastSeen) && asString(rec["status"]) != "offline" {
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

// deviceOwner is the slice of a user the widened device list reports.
type deviceOwner struct {
	Email string
	Name  string
}

// deviceScope resolves the PocketBase owner filter for a device listing, plus
// the owner lookup used to label rows. Widening is capped by role: a superadmin
// watches every registered device, an admin their own organization's, and
// anyone else only their own — so an untrusted ?scope=all cannot leak devices.
// An empty filter means "no owner restriction".
func (s *Server) deviceScope(ctx context.Context, who *callerIdentity, widen bool) (string, map[string]deviceOwner, error) {
	mine := "owner = " + pbQuote(who.ID)
	if !widen || !who.isManager() {
		return mine, nil, nil
	}

	opt := pb.ListOptions{Sort: "email", PerPage: 500}
	if !who.isSuperadmin() {
		// Admin: scoped to their organization. An org-less admin manages nobody,
		// so the widened view collapses back to their own devices.
		if who.OrgID == "" {
			return mine, nil, nil
		}
		opt.Filter = "organization = " + pbQuote(who.OrgID)
	}
	res, err := s.pb.List(ctx, colUsers, opt)
	if err != nil {
		return "", nil, err
	}

	owners := make(map[string]deviceOwner, len(res.Items))
	ids := make([]string, 0, len(res.Items))
	for _, rec := range res.Items {
		id := asString(rec["id"])
		owners[id] = deviceOwner{Email: asString(rec["email"]), Name: asString(rec["name"])}
		ids = append(ids, "owner = "+pbQuote(id))
	}
	if who.isSuperadmin() {
		return "", owners, nil // every device, whoever owns it
	}
	if len(ids) == 0 {
		return mine, owners, nil
	}
	return "(" + strings.Join(ids, " || ") + ")", owners, nil
}

// handleListDevices returns the caller's devices. ?scope=all widens the list as
// far as the caller's role allows (see deviceScope) — the panel's Overview uses
// it to watch every connected device; the Web App asks for the default view.
func (s *Server) handleListDevices(w http.ResponseWriter, r *http.Request) {
	who := caller(r)
	filter, owners, err := s.deviceScope(r.Context(), who, r.URL.Query().Get("scope") == "all")
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	res, err := s.pb.List(r.Context(), colDevices, pb.ListOptions{
		Filter:  filter,
		Sort:    "-created",
		PerPage: 200,
	})
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	out := make([]deviceDTO, 0, len(res.Items))
	for _, rec := range res.Items {
		d := recordToDevice(rec)
		if o, ok := owners[asString(rec["owner"])]; ok {
			d.OwnerEmail, d.OwnerName = o.Email, o.Name
		}
		out = append(out, d)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out, "total": res.TotalItems})
}

// canManageDevice reports whether the caller may rename or remove this device.
// The same ladder the widened listing uses: your own devices always, every
// device for a superadmin, an admin's own organization's devices for an admin.
// Anything else is somebody else's phone.
func (s *Server) canManageDevice(ctx context.Context, who *callerIdentity, rec pb.Record) (bool, error) {
	owner := asString(rec["owner"])
	if owner == who.ID || who.isSuperadmin() {
		return true, nil
	}
	if !who.isManager() || who.OrgID == "" {
		return false, nil
	}
	u, err := s.pb.GetOne(ctx, colUsers, owner)
	if err != nil {
		if pb.NotFound(err) {
			return false, nil // orphaned device: only a superadmin cleans those up
		}
		return false, err
	}
	return asString(u["organization"]) == who.OrgID, nil
}

// loadManageableDevice fetches the device at {id} and enforces the scope rules,
// writing the response itself when the caller may not touch it.
func (s *Server) loadManageableDevice(w http.ResponseWriter, r *http.Request) (pb.Record, bool) {
	who := caller(r)
	rec, err := s.pb.GetOne(r.Context(), colDevices, r.PathValue("id"))
	if err != nil {
		writeUpstreamError(w, err)
		return nil, false
	}
	ok, err := s.canManageDevice(r.Context(), who, rec)
	if err != nil {
		writeUpstreamError(w, err)
		return nil, false
	}
	if !ok {
		writeError(w, http.StatusForbidden, "not your device")
		return nil, false
	}
	return rec, true
}

// handleDeleteDevice removes a device the caller may manage.
func (s *Server) handleDeleteDevice(w http.ResponseWriter, r *http.Request) {
	rec, ok := s.loadManageableDevice(w, r)
	if !ok {
		return
	}
	if err := s.pb.Delete(r.Context(), colDevices, asString(rec["id"])); err != nil {
		writeUpstreamError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleUpdateDevice renames a device. Only the display name is editable:
// device_id, the token and the heartbeat fields are the phone's to report.
func (s *Server) handleUpdateDevice(w http.ResponseWriter, r *http.Request) {
	rec, ok := s.loadManageableDevice(w, r)
	if !ok {
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	updated, err := s.pb.Update(r.Context(), colDevices, asString(rec["id"]), pb.Record{"name": name})
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, recordToDevice(updated))
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

// handleGoingOffline marks a device offline on request. The phone calls this
// when its gateway is stopped, so the panel and Web App flip immediately rather
// than waiting for onlineWindow to lapse. last_seen_at is left alone — the
// device really was seen just now, it just isn't routing any more.
func (s *Server) handleGoingOffline(w http.ResponseWriter, r *http.Request) {
	device := deviceFromCtx(r.Context())
	_, err := s.pb.Update(r.Context(), colDevices, asString(device["id"]), pb.Record{
		"status": "offline",
	})
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "offline"})
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
