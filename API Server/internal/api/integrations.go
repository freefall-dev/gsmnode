package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"smsgateway/apiserver/internal/pb"
	"smsgateway/apiserver/internal/plugins/builtin/emailtosms"
)

// This file exposes the email-to-sms plugin to end users under a three-layer
// cascade, so everyone can poll their own IMAP mailbox while a superadmin (and,
// in an organization, an org admin) can impose settings from above.
//
// Resolution is a cascade: the top layer wins, and a lower layer only fills a
// field the layers above left blank.
//
//   - global (L1): the plugin's config in plugins.json, set in the Plugins panel
//     by a superadmin. Supplies optional defaults (imap_default_host/port, mailbox)
//     and the master on/off switch (the plugin being enabled).
//   - org    (L2): pluginSettings["email-to-sms"] on the caller's organization.
//   - user   (L3): pluginSettings["email-to-sms"] on the caller's own user record.
//
// The IMAP username + password resolve together as a *pair* from the highest
// layer that supplies a username, so credential halves are never mixed. Host,
// port and mailbox resolve on their own. Enablement is per-user (L3), gated by
// the global master switch and, for org users, by the org gate.
//
// Secrets and inherited usernames are never returned in clear to a lower-
// privileged client; the effective config is resolved server-side and only
// masked values leave the API.

const (
	emailToSMSPlugin = emailtosms.Name
	e2sSecretMask    = "••••••••"
)

// e2sConfig is one layer's email-to-sms IMAP settings.
type e2sConfig struct {
	Host     string `json:"imap_host"`
	Port     string `json:"imap_port"`
	User     string `json:"imap_user"`
	Password string `json:"imap_password"`
	Mailbox  string `json:"imap_mailbox"`
}

// e2sStored is what we persist per user/org under pluginSettings["email-to-sms"].
type e2sStored struct {
	Config e2sConfig `json:"config"`
	// Enabled is the personal per-user opt-in (user layer). Default false.
	Enabled bool `json:"enabled"`
	// Disabled is the organization layer's off switch, stored inverted so absent
	// == enabled. Only meaningful on an org record.
	Disabled bool `json:"disabled,omitempty"`
}

type e2sSettingsDoc struct {
	EmailToSMS e2sStored `json:"email-to-sms"`
}

// e2sFieldView is one field's resolved state for the UI.
type e2sFieldView struct {
	Effective string `json:"effective"`
	Own       string `json:"own"`
	Source    string `json:"source"` // global | org | user | unset
	Locked    bool   `json:"locked"`
}

// e2sResolution is the fully-resolved email-to-sms state for one caller.
type e2sResolution struct {
	eff        e2sConfig
	userOwn    e2sConfig
	orgOwn     e2sConfig
	source     map[string]string
	isSuper    bool
	canOrg     bool
	available  bool
	orgEnabled bool
	enabled    bool
}

var e2sLayerRank = map[string]int{"global": 1, "org": 2, "user": 3}

// resolveE2S computes the cascade for a caller. userRaw is the caller's
// pluginSettings blob (from their user record).
func (s *Server) resolveE2S(ctx context.Context, who *callerIdentity, userRaw json.RawMessage) e2sResolution {
	g, masterEnabled, _ := s.plugins.RawConfig(emailToSMSPlugin)
	gc := e2sConfig{Host: g["imap_default_host"], Port: g["imap_default_port"], Mailbox: g["imap_mailbox"]}

	var oStored e2sStored
	if who.OrgID != "" {
		oStored, _ = s.orgE2S(ctx, who.OrgID)
	}
	oc := oStored.Config

	var uStored e2sStored
	if len(userRaw) > 0 {
		var d e2sSettingsDoc
		_ = json.Unmarshal(userRaw, &d)
		uStored = d.EmailToSMS
	}
	uc := uStored.Config

	res := e2sResolution{
		source:  map[string]string{},
		userOwn: uc,
		orgOwn:  oc,
		isSuper: who.isSuperadmin(),
		// An org admin may edit the organization layer. Requires the service
		// account (org writes go through it).
		canOrg:     who.isManager() && !who.isSuperadmin() && who.OrgID != "" && s.pb.Configured(),
		available:  masterEnabled,
		orgEnabled: !oStored.Disabled,
		enabled:    uStored.Enabled,
	}

	type layer struct {
		name string
		c    e2sConfig
	}
	layers := []layer{{"global", gc}}
	if who.OrgID != "" {
		layers = append(layers, layer{"org", oc})
	}
	layers = append(layers, layer{"user", uc})

	// Host, port and mailbox resolve independently: the highest layer that sets
	// them wins.
	pickIndependent := func(key string, get func(e2sConfig) string, set func(*e2sConfig, string)) {
		res.source[key] = "unset"
		for _, l := range layers {
			if v := strings.TrimSpace(get(l.c)); v != "" {
				set(&res.eff, v)
				res.source[key] = l.name
				break
			}
		}
	}
	pickIndependent("imap_host", func(c e2sConfig) string { return c.Host }, func(c *e2sConfig, v string) { c.Host = v })
	pickIndependent("imap_port", func(c e2sConfig) string { return c.Port }, func(c *e2sConfig, v string) { c.Port = v })
	pickIndependent("imap_mailbox", func(c e2sConfig) string { return c.Mailbox }, func(c *e2sConfig, v string) { c.Mailbox = v })

	// Credentials resolve as a pair from the highest layer with a username.
	credSrc := "unset"
	for _, l := range layers {
		if u := strings.TrimSpace(l.c.User); u != "" {
			res.eff.User, res.eff.Password, credSrc = u, l.c.Password, l.name
			break
		}
	}
	res.source["imap_user"] = credSrc
	res.source["imap_password"] = credSrc
	return res
}

// e2sLockedFor reports whether a field from source is locked for a caller whose
// editable layer is editable (i.e. the value is set above them).
func e2sLockedFor(source, editable string) bool {
	if editable == "none" {
		return true // superadmin edits the global layer in the panel, not here
	}
	sr, ok := e2sLayerRank[source]
	if !ok {
		return false // unset — the caller may be the first to set it
	}
	return sr < e2sLayerRank[editable]
}

func e2sMaskPresent(v string) string {
	if strings.TrimSpace(v) != "" {
		return e2sSecretMask
	}
	return ""
}

// orgE2S reads an organization's stored email-to-sms settings and raw blob via
// the service account. Best effort: zero values on any miss.
func (s *Server) orgE2S(ctx context.Context, orgID string) (e2sStored, json.RawMessage) {
	if orgID == "" || !s.pb.Configured() {
		return e2sStored{}, nil
	}
	rec, err := s.pb.GetOne(ctx, colOrgs, orgID)
	if err != nil {
		return e2sStored{}, nil
	}
	raw := rawJSON(rec["pluginSettings"])
	var doc e2sSettingsDoc
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &doc)
	}
	return doc.EmailToSMS, raw
}

// userPluginSettings reads a user's raw pluginSettings blob. Best effort: nil on miss.
func (s *Server) userPluginSettings(ctx context.Context, id string) json.RawMessage {
	if id == "" || !s.pb.Configured() {
		return nil
	}
	rec, err := s.pb.GetOne(ctx, colUsers, id)
	if err != nil {
		return nil
	}
	return rawJSON(rec["pluginSettings"])
}

// rawJSON re-marshals a decoded PocketBase field back to JSON bytes (nil for a
// null/absent field).
func rawJSON(v any) json.RawMessage {
	if v == nil {
		return nil
	}
	if s, ok := v.(string); ok {
		if strings.TrimSpace(s) == "" {
			return nil
		}
		return json.RawMessage(s)
	}
	b, err := json.Marshal(v)
	if err != nil || string(b) == "null" {
		return nil
	}
	return b
}

// mergeE2S applies a mutation to the email-to-sms entry of a pluginSettings blob,
// preserving any other plugin keys, and returns the new blob.
func mergeE2S(existing json.RawMessage, apply func(*e2sStored)) json.RawMessage {
	doc := map[string]json.RawMessage{}
	if len(existing) > 0 {
		_ = json.Unmarshal(existing, &doc)
	}
	if doc == nil {
		doc = map[string]json.RawMessage{}
	}
	var st e2sStored
	if raw, ok := doc[emailToSMSPlugin]; ok {
		_ = json.Unmarshal(raw, &st)
	}
	apply(&st)
	b, _ := json.Marshal(st)
	doc[emailToSMSPlugin] = b
	out, _ := json.Marshal(doc)
	return out
}

// e2sScopeView builds the masked field set for one editable scope.
func (s *Server) e2sScopeView(res e2sResolution, editable string) map[string]any {
	own := res.userOwn
	if editable == "org" {
		own = res.orgOwn
	}
	field := func(key, eff, ownv string, secret bool) e2sFieldView {
		src := res.source[key]
		locked := e2sLockedFor(src, editable)
		fv := e2sFieldView{Source: src, Locked: locked}
		switch {
		case secret:
			fv.Effective, fv.Own = e2sMaskPresent(eff), e2sMaskPresent(ownv)
		case key == "imap_user" && locked:
			fv.Effective, fv.Own = e2sMaskPresent(eff), e2sMaskPresent(ownv)
		default:
			fv.Effective, fv.Own = eff, ownv
		}
		return fv
	}
	return map[string]any{
		"editableLayer": editable,
		"fields": map[string]e2sFieldView{
			"imap_host":     field("imap_host", res.eff.Host, own.Host, false),
			"imap_port":     field("imap_port", res.eff.Port, own.Port, false),
			"imap_user":     field("imap_user", res.eff.User, own.User, false),
			"imap_password": field("imap_password", res.eff.Password, own.Password, true),
			"imap_mailbox":  field("imap_mailbox", res.eff.Mailbox, own.Mailbox, false),
		},
	}
}

// e2sView builds the masked, client-safe response body from a resolution.
func (s *Server) e2sView(who *callerIdentity, res e2sResolution) map[string]any {
	out := map[string]any{
		"available":    res.available,
		"orgEnabled":   res.orgEnabled,
		"enabled":      res.enabled,
		"role":         who.Role,
		"orgId":        who.OrgID,
		"canEditOrg":   res.canOrg,
		"isSuperadmin": res.isSuper,
	}
	if res.isSuper {
		out["editableLayer"] = "none"
		out["scopes"] = map[string]any{"user": s.e2sScopeView(res, "none")}
		return out
	}
	scopes := map[string]any{"user": s.e2sScopeView(res, "user")}
	if res.canOrg {
		scopes["org"] = s.e2sScopeView(res, "org")
	}
	out["scopes"] = scopes
	return out
}

// GET /api/integrations/email-to-sms — resolved view for the caller.
func (s *Server) handleGetEmailToSMS(w http.ResponseWriter, r *http.Request) {
	who := caller(r)
	if who == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	userRaw := s.userPluginSettings(r.Context(), who.ID)
	res := s.resolveE2S(r.Context(), who, userRaw)
	writeJSON(w, http.StatusOK, s.e2sView(who, res))
}

// PUT /api/integrations/email-to-sms — save the caller's editable layer. Body:
// {enabled?: bool, scope?: "user"|"org", config?: {imap_*}}.
func (s *Server) handlePutEmailToSMS(w http.ResponseWriter, r *http.Request) {
	who := caller(r)
	if who == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	if !s.pb.Configured() {
		writeError(w, http.StatusServiceUnavailable, "integration settings not configured on the server")
		return
	}
	var body struct {
		Enabled *bool             `json:"enabled"`
		Scope   string            `json:"scope"`
		Config  map[string]string `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	userRaw := s.userPluginSettings(r.Context(), who.ID)
	res := s.resolveE2S(r.Context(), who, userRaw)

	editable := "user"
	switch {
	case res.isSuper:
		editable = "none"
	case strings.EqualFold(strings.TrimSpace(body.Scope), "org"):
		if !res.canOrg {
			writeError(w, http.StatusForbidden, "only an organization admin can edit organization settings")
			return
		}
		editable = "org"
	}

	newOwn := res.userOwn
	if editable == "org" {
		newOwn = res.orgOwn
	}
	applyField := func(key string, set func(*e2sConfig, string)) {
		v, ok := body.Config[key]
		if !ok || e2sLockedFor(res.source[key], editable) {
			return
		}
		if key == "imap_password" && v == e2sSecretMask {
			return // keep current secret
		}
		set(&newOwn, strings.TrimSpace(v))
	}
	applyField("imap_host", func(c *e2sConfig, v string) { c.Host = v })
	applyField("imap_port", func(c *e2sConfig, v string) { c.Port = v })
	applyField("imap_user", func(c *e2sConfig, v string) { c.User = v })
	applyField("imap_password", func(c *e2sConfig, v string) { c.Password = v })
	applyField("imap_mailbox", func(c *e2sConfig, v string) { c.Mailbox = v })

	// Persist the organization layer (admins) via the service account.
	if editable == "org" {
		_, orgRaw := s.orgE2S(r.Context(), who.OrgID)
		newDoc := mergeE2S(orgRaw, func(st *e2sStored) {
			st.Config = newOwn
			if body.Enabled != nil {
				st.Disabled = !*body.Enabled // org master switch, stored inverted
			}
		})
		if _, err := s.pb.Update(r.Context(), colOrgs, who.OrgID,
			map[string]any{"pluginSettings": newDoc}); err != nil {
			writeUpstreamError(w, err)
			return
		}
	}

	// Persist the user record: personal enable flag (user scope) + personal config.
	personalEnable := body.Enabled != nil && editable != "org"
	if personalEnable || editable == "user" {
		newDoc := mergeE2S(userRaw, func(st *e2sStored) {
			if personalEnable {
				st.Enabled = *body.Enabled
			}
			if editable == "user" {
				st.Config = newOwn
			}
		})
		if _, err := s.pb.Update(r.Context(), colUsers, who.ID,
			map[string]any{"pluginSettings": newDoc}); err != nil {
			writeUpstreamError(w, err)
			return
		}
	}

	fresh := s.userPluginSettings(r.Context(), who.ID)
	res2 := s.resolveE2S(r.Context(), who, fresh)
	writeJSON(w, http.StatusOK, s.e2sView(who, res2))
}

// POST /api/integrations/email-to-sms/health — probe the caller's resolved IMAP
// mailbox. Never returns secrets.
func (s *Server) handleEmailToSMSHealth(w http.ResponseWriter, r *http.Request) {
	who := caller(r)
	if who == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	userRaw := s.userPluginSettings(r.Context(), who.ID)
	res := s.resolveE2S(r.Context(), who, userRaw)

	down := func(detail string) {
		writeJSON(w, http.StatusOK, map[string]any{"health": map[string]any{"status": "down", "detail": detail}})
	}
	switch {
	case !res.available:
		down("The email-to-sms integration is disabled by the administrator")
		return
	case !res.orgEnabled:
		down("The email-to-sms integration is disabled for your organization")
		return
	case strings.TrimSpace(res.eff.Host) == "" || strings.TrimSpace(res.eff.User) == "" || strings.TrimSpace(res.eff.Password) == "":
		down("Enter your IMAP host, username and password to connect")
		return
	}

	h := emailtosms.ProbeMailbox(r.Context(), emailtosms.IMAPTarget{
		OwnerID:  who.ID,
		Host:     res.eff.Host,
		Port:     portOr(res.eff.Port, 993),
		Username: res.eff.User,
		Password: res.eff.Password,
		Mailbox:  res.eff.Mailbox,
	})
	writeJSON(w, http.StatusOK, map[string]any{"health": h})
}

// portOr parses a port string, falling back to def for empty/invalid input.
func portOr(s string, def int) int {
	if n := asInt(s); n > 0 {
		return n
	}
	return def
}

// emailToSMSIMAPTargets resolves every user's mailbox to poll in IMAP mode. It is
// called by the plugin host adapter (plugin_host.go). Returns nil when the plugin
// is disabled globally.
func (s *Server) emailToSMSIMAPTargets(ctx context.Context) ([]emailtosms.IMAPTarget, error) {
	_, masterEnabled, ok := s.plugins.RawConfig(emailToSMSPlugin)
	if !ok || !masterEnabled || !s.pb.Configured() {
		return nil, nil
	}
	res, err := s.pb.List(ctx, colUsers, pb.ListOptions{PerPage: 500})
	if err != nil {
		return nil, err
	}
	var out []emailtosms.IMAPTarget
	for _, rec := range res.Items {
		who := &callerIdentity{
			ID:    asString(rec["id"]),
			Role:  asString(rec["role"]),
			OrgID: asString(rec["organization"]),
		}
		userRaw := rawJSON(rec["pluginSettings"])
		r := s.resolveE2S(ctx, who, userRaw)
		if !r.enabled || !r.orgEnabled {
			continue
		}
		if strings.TrimSpace(r.eff.Host) == "" || strings.TrimSpace(r.eff.User) == "" || strings.TrimSpace(r.eff.Password) == "" {
			continue
		}
		out = append(out, emailtosms.IMAPTarget{
			OwnerID:  who.ID,
			Host:     r.eff.Host,
			Port:     portOr(r.eff.Port, 993),
			Username: r.eff.User,
			Password: r.eff.Password,
			Mailbox:  r.eff.Mailbox,
		})
	}
	return out, nil
}
