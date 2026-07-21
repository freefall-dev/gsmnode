package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"smsgateway/apiserver/internal/pb"
	"smsgateway/apiserver/internal/plugins"
	"smsgateway/apiserver/internal/plugins/builtin/emailtosms"
)

// This file exposes plugins that offer per-user settings to end users, under a
// three-layer cascade, so everyone can supply their own credentials while a
// superadmin (and, in an organization, an org admin) can impose settings from
// above.
//
// Which fields exist is not known here: each plugin declares them
// (plugins.UserConfigSpec, see internal/plugins/userconfig.go) and everything
// below — resolution, masking, persistence, and the form the Web App renders —
// is driven by that declaration.
//
// Resolution is a cascade: the top layer wins, and a lower layer only fills a
// field the layers above left blank.
//
//   - global (L1): the plugin's config in plugins.json, set in the Plugins panel
//     by a superadmin, read through each field's GlobalKey. Also the master
//     on/off switch (the plugin being enabled).
//   - org    (L2): pluginSettings[<plugin>] on the caller's organization.
//   - user   (L3): pluginSettings[<plugin>] on the caller's own user record.
//
// Fields sharing a Group resolve together as a unit from the highest layer that
// sets the group's first field, so the halves of a credential are never mixed.
// Enablement is per-user (L3), gated by the global master switch and, for org
// users, by the org gate.
//
// Secrets and inherited private values are never returned in clear to a lower-
// privileged client; the effective config is resolved server-side and only
// masked values leave the API.

const integrationMask = "••••••••"

// integrationStored is what we persist per user/org under
// pluginSettings[<plugin>]. The shape predates the generic cascade and is
// unchanged, so existing records keep resolving.
type integrationStored struct {
	Config map[string]string `json:"config"`
	// Enabled is the personal per-user opt-in (user layer). Default false.
	Enabled bool `json:"enabled"`
	// Disabled is the organization layer's off switch, stored inverted so absent
	// == enabled. Only meaningful on an org record.
	Disabled bool `json:"disabled,omitempty"`
}

// fieldView is one field's resolved state for the UI.
type fieldView struct {
	Effective string `json:"effective"`
	Own       string `json:"own"`
	Source    string `json:"source"` // global | org | user | unset
	Locked    bool   `json:"locked"`
}

// integrationResolution is one plugin's fully-resolved state for one caller.
type integrationResolution struct {
	name    string
	spec    plugins.UserConfigSpec
	eff     map[string]string
	userOwn map[string]string
	orgOwn  map[string]string
	source  map[string]string

	isSuper    bool
	canOrg     bool
	available  bool
	orgEnabled bool
	enabled    bool
}

var layerRank = map[string]int{"global": 1, "org": 2, "user": 3}

// globalLayer projects a plugin's global config onto the per-user field keys,
// honouring each field's GlobalKey (and skipping fields with no global layer).
func globalLayer(spec plugins.UserConfigSpec, g map[string]string) map[string]string {
	out := map[string]string{}
	for _, f := range spec.Fields {
		if f.GlobalKey == plugins.NoGlobalKey {
			continue
		}
		key := f.GlobalKey
		if key == "" {
			key = f.Key
		}
		out[f.Key] = g[key]
	}
	return out
}

// resolveIntegration computes the cascade for one caller and one plugin. userRaw
// is the caller's pluginSettings blob (from their user record).
func (s *Server) resolveIntegration(ctx context.Context, who *callerIdentity, name string, spec plugins.UserConfigSpec, userRaw json.RawMessage) integrationResolution {
	g, masterEnabled, _ := s.plugins.RawConfig(name)

	var oStored integrationStored
	if who.OrgID != "" {
		oStored, _ = s.orgIntegration(ctx, who.OrgID, name)
	}
	uStored := decodeIntegration(userRaw, name)

	res := integrationResolution{
		name:    name,
		spec:    spec,
		eff:     map[string]string{},
		source:  map[string]string{},
		userOwn: nonNil(uStored.Config),
		orgOwn:  nonNil(oStored.Config),
		isSuper: who.isSuperadmin(),
		// An org admin may edit the organization layer. Requires the service
		// account (org writes go through it).
		canOrg:     who.isManager() && !who.isSuperadmin() && who.OrgID != "" && s.pb.Configured(),
		available:  masterEnabled,
		orgEnabled: !oStored.Disabled,
		enabled:    uStored.Enabled,
	}

	res.eff, res.source = resolveLayers(spec, globalLayer(spec, g), res.orgOwn, res.userOwn, who.OrgID != "")
	return res
}

// resolveLayers is the cascade itself: given one layer's values each, it returns
// the effective value and the winning layer name per field. An org-less caller
// has no org layer at all (hasOrg false), so nothing can be imposed on them
// through one.
//
// Ungrouped fields resolve independently — the highest layer that sets one wins.
// Grouped fields resolve as a unit, keyed by the group's first field in spec
// order, so a password can never be paired with another layer's username.
func resolveLayers(spec plugins.UserConfigSpec, gc, oc, uc map[string]string, hasOrg bool) (eff, source map[string]string) {
	type layer struct {
		name string
		c    map[string]string
	}
	layers := []layer{{"global", gc}}
	if hasOrg {
		layers = append(layers, layer{"org", oc})
	}
	layers = append(layers, layer{"user", uc})

	eff, source = map[string]string{}, map[string]string{}

	groupLead := map[string]string{} // group → first field key in spec order
	for _, f := range spec.Fields {
		if f.Group != "" {
			if _, seen := groupLead[f.Group]; !seen {
				groupLead[f.Group] = f.Key
			}
		}
	}
	groupSource := map[string]string{} // group → winning layer name
	groupValues := map[string]map[string]string{}
	for group, lead := range groupLead {
		groupSource[group] = "unset"
		for _, l := range layers {
			if strings.TrimSpace(l.c[lead]) != "" {
				groupSource[group], groupValues[group] = l.name, l.c
				break
			}
		}
	}

	for _, f := range spec.Fields {
		if f.Group != "" {
			source[f.Key] = groupSource[f.Group]
			if vals := groupValues[f.Group]; vals != nil {
				eff[f.Key] = vals[f.Key]
			}
			continue
		}
		source[f.Key] = "unset"
		for _, l := range layers {
			if v := strings.TrimSpace(l.c[f.Key]); v != "" {
				eff[f.Key] = v
				source[f.Key] = l.name
				break
			}
		}
	}
	return eff, source
}

// lockedFor reports whether a field from source is locked for a caller whose
// editable layer is editable (i.e. the value is set above them).
func lockedFor(source, editable string) bool {
	if editable == "none" {
		return true // superadmin edits the global layer in the panel, not here
	}
	sr, ok := layerRank[source]
	if !ok {
		return false // unset — the caller may be the first to set it
	}
	return sr < layerRank[editable]
}

func maskPresent(v string) string {
	if strings.TrimSpace(v) != "" {
		return integrationMask
	}
	return ""
}

func nonNil(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

// decodeIntegration pulls one plugin's stored settings out of a pluginSettings blob.
func decodeIntegration(raw json.RawMessage, name string) integrationStored {
	var st integrationStored
	if len(raw) == 0 {
		return st
	}
	doc := map[string]json.RawMessage{}
	if json.Unmarshal(raw, &doc) != nil {
		return st
	}
	if entry, ok := doc[name]; ok {
		_ = json.Unmarshal(entry, &st)
	}
	return st
}

// orgIntegration reads an organization's stored settings for one plugin, and the
// org's raw blob, via the service account. Best effort: zero values on any miss.
func (s *Server) orgIntegration(ctx context.Context, orgID, name string) (integrationStored, json.RawMessage) {
	if orgID == "" || !s.pb.Configured() {
		return integrationStored{}, nil
	}
	rec, err := s.pb.GetOne(ctx, colOrgs, orgID)
	if err != nil {
		return integrationStored{}, nil
	}
	raw := rawJSON(rec["pluginSettings"])
	return decodeIntegration(raw, name), raw
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

// mergeIntegration applies a mutation to one plugin's entry in a pluginSettings
// blob, preserving every other plugin's key, and returns the new blob.
func mergeIntegration(existing json.RawMessage, name string, apply func(*integrationStored)) json.RawMessage {
	doc := map[string]json.RawMessage{}
	if len(existing) > 0 {
		_ = json.Unmarshal(existing, &doc)
	}
	if doc == nil {
		doc = map[string]json.RawMessage{}
	}
	var st integrationStored
	if raw, ok := doc[name]; ok {
		_ = json.Unmarshal(raw, &st)
	}
	apply(&st)
	b, _ := json.Marshal(st)
	doc[name] = b
	out, _ := json.Marshal(doc)
	return out
}

// scopeView builds the masked field set for one editable scope.
func scopeView(res integrationResolution, editable string) map[string]any {
	own := res.userOwn
	if editable == "org" {
		own = res.orgOwn
	}
	fields := map[string]fieldView{}
	for _, f := range res.spec.Fields {
		src := res.source[f.Key]
		locked := lockedFor(src, editable)
		fv := fieldView{Source: src, Locked: locked}
		switch {
		case f.Secret:
			fv.Effective, fv.Own = maskPresent(res.eff[f.Key]), maskPresent(own[f.Key])
		case f.MaskWhenInherited && locked:
			fv.Effective, fv.Own = maskPresent(res.eff[f.Key]), maskPresent(own[f.Key])
		default:
			fv.Effective, fv.Own = res.eff[f.Key], own[f.Key]
		}
		fields[f.Key] = fv
	}
	return map[string]any{"editableLayer": editable, "fields": fields}
}

// integrationView builds the masked, client-safe response body from a resolution.
func integrationView(who *callerIdentity, res integrationResolution) map[string]any {
	out := map[string]any{
		"name":         res.name,
		"spec":         res.spec,
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
		out["scopes"] = map[string]any{"user": scopeView(res, "none")}
		return out
	}
	scopes := map[string]any{"user": scopeView(res, "user")}
	if res.canOrg {
		scopes["org"] = scopeView(res, "org")
	}
	out["scopes"] = scopes
	return out
}

// integrationSpec looks up a plugin's per-user declaration, writing a 404 when
// the plugin does not offer one.
func (s *Server) integrationSpec(w http.ResponseWriter, name string) (plugins.UserConfigSpec, bool) {
	spec, ok := s.plugins.UserSpec(name)
	if !ok {
		writeError(w, http.StatusNotFound, "unknown integration")
		return plugins.UserConfigSpec{}, false
	}
	return spec, true
}

// GET /api/integrations — every integration the caller can configure, each
// fully resolved, so a client renders its whole settings page from one call.
func (s *Server) handleListIntegrations(w http.ResponseWriter, r *http.Request) {
	who := caller(r)
	if who == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	userRaw := s.userPluginSettings(r.Context(), who.ID)

	out := []map[string]any{}
	for _, name := range s.plugins.UserConfigurableNames() {
		spec, ok := s.plugins.UserSpec(name)
		if !ok {
			continue
		}
		res := s.resolveIntegration(r.Context(), who, name, spec, userRaw)
		out = append(out, integrationView(who, res))
	}
	writeJSON(w, http.StatusOK, map[string]any{"integrations": out})
}

// GET /api/integrations/{name} — resolved view for the caller.
func (s *Server) handleGetIntegration(w http.ResponseWriter, r *http.Request) {
	who := caller(r)
	if who == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	name := r.PathValue("name")
	spec, ok := s.integrationSpec(w, name)
	if !ok {
		return
	}
	userRaw := s.userPluginSettings(r.Context(), who.ID)
	res := s.resolveIntegration(r.Context(), who, name, spec, userRaw)
	writeJSON(w, http.StatusOK, integrationView(who, res))
}

// PUT /api/integrations/{name} — save the caller's editable layer. Body:
// {enabled?: bool, scope?: "user"|"org", config?: {<field key>: value}}.
func (s *Server) handlePutIntegration(w http.ResponseWriter, r *http.Request) {
	who := caller(r)
	if who == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	name := r.PathValue("name")
	spec, ok := s.integrationSpec(w, name)
	if !ok {
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
	res := s.resolveIntegration(r.Context(), who, name, spec, userRaw)

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

	newOwn := map[string]string{}
	for k, v := range res.userOwn {
		newOwn[k] = v
	}
	if editable == "org" {
		newOwn = map[string]string{}
		for k, v := range res.orgOwn {
			newOwn[k] = v
		}
	}
	for _, f := range spec.Fields {
		v, sent := body.Config[f.Key]
		if !sent || lockedFor(res.source[f.Key], editable) {
			continue
		}
		if f.Secret && v == integrationMask {
			continue // keep current secret
		}
		newOwn[f.Key] = strings.TrimSpace(v)
	}

	// Persist the organization layer (admins) via the service account.
	if editable == "org" {
		_, orgRaw := s.orgIntegration(r.Context(), who.OrgID, name)
		newDoc := mergeIntegration(orgRaw, name, func(st *integrationStored) {
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
		newDoc := mergeIntegration(userRaw, name, func(st *integrationStored) {
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
	res2 := s.resolveIntegration(r.Context(), who, name, spec, fresh)
	writeJSON(w, http.StatusOK, integrationView(who, res2))
}

// POST /api/integrations/{name}/health — probe the caller's resolved settings
// against the plugin. Never returns secrets.
func (s *Server) handleIntegrationHealth(w http.ResponseWriter, r *http.Request) {
	who := caller(r)
	if who == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	name := r.PathValue("name")
	spec, ok := s.integrationSpec(w, name)
	if !ok {
		return
	}
	userRaw := s.userPluginSettings(r.Context(), who.ID)
	res := s.resolveIntegration(r.Context(), who, name, spec, userRaw)

	down := func(detail string) {
		writeJSON(w, http.StatusOK, map[string]any{"health": map[string]any{"status": "down", "detail": detail}})
	}
	switch {
	case !res.available:
		down("The " + name + " integration is disabled by the administrator")
		return
	case !res.orgEnabled:
		down("The " + name + " integration is disabled for your organization")
		return
	}

	h, err := s.plugins.UserHealthCheckWith(r.Context(), name,
		plugins.UserContext{OwnerID: who.ID, Role: who.Role, OrgID: who.OrgID}, res.eff)
	if err != nil {
		down(err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"health": h})
}

// emailToSMSIMAPTargets resolves every user's mailbox to poll in IMAP mode. It is
// called by the plugin host adapter (plugin_host.go). Returns nil when the plugin
// is disabled globally.
func (s *Server) emailToSMSIMAPTargets(ctx context.Context) ([]emailtosms.IMAPTarget, error) {
	_, masterEnabled, ok := s.plugins.RawConfig(emailtosms.Name)
	if !ok || !masterEnabled || !s.pb.Configured() {
		return nil, nil
	}
	spec, ok := s.plugins.UserSpec(emailtosms.Name)
	if !ok {
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
		r := s.resolveIntegration(ctx, who, emailtosms.Name, spec, rawJSON(rec["pluginSettings"]))
		if !r.enabled || !r.orgEnabled {
			continue
		}
		t := emailtosms.IMAPTargetFrom(who.ID, r.eff)
		if t.Host == "" || t.Username == "" || t.Password == "" {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}
