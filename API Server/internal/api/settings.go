package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"

	"smsgateway/apiserver/internal/config"
	"smsgateway/apiserver/internal/pb"
)

// pbProbe is the outcome of testing a PocketBase connection: whether the base
// URL answers its health check and whether the service-account credentials
// authenticate as a superuser.
type pbProbe struct {
	Reachable  bool   `json:"reachable"`
	HTTPStatus int    `json:"httpStatus,omitempty"`
	LatencyMs  int64  `json:"latencyMs,omitempty"`
	Superuser  bool   `json:"superuser"`
	Detail     string `json:"detail,omitempty"`
}

// pbConfigView is the PocketBase-connection shape returned to the panel. The
// password itself is never sent back — only whether one is set.
type pbConfigView struct {
	URL             string  `json:"url"`
	AdminEmail      string  `json:"adminEmail"`
	AdminConfigured bool    `json:"adminConfigured"`
	Probe           pbProbe `json:"probe"`
}

// probePB checks a PocketBase base URL's health and, when credentials are given,
// whether they authenticate as a superuser.
func probePB(ctx context.Context, url, email, password string) pbProbe {
	h := probe(ctx, url+"/api/health")
	p := pbProbe{Reachable: h.Status == "ok", HTTPStatus: h.HTTPStatus, LatencyMs: h.LatencyMs}
	if h.Error != "" {
		p.Detail = h.Error
	}
	if email != "" && password != "" {
		st, err := pb.SuperuserAuth(ctx, healthClient, url, email, password)
		if err == nil {
			p.Superuser = true
		} else if p.Reachable {
			p.Detail = "superuser auth failed"
			if st > 0 {
				p.Detail += " (HTTP " + strconv.Itoa(st) + ")"
			}
		}
	}
	return p
}

// normalizeURL trims, defaults the scheme to http, and drops a trailing slash.
func normalizeURL(u string) string {
	u = strings.TrimSpace(u)
	if u == "" {
		return ""
	}
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		u = "http://" + u
	}
	return strings.TrimRight(u, "/")
}

// viewFor builds the panel's connection view, including a live probe.
func viewFor(ctx context.Context, url, email, password string) pbConfigView {
	return pbConfigView{
		URL:             url,
		AdminEmail:      email,
		AdminConfigured: email != "" && password != "",
		Probe:           probePB(ctx, url, email, password),
	}
}

// GET /api/admin/pb-config — current PocketBase connection + a live probe.
func (s *Server) handleGetPBConfig(w http.ResponseWriter, r *http.Request) {
	url, email, password := s.pbSettings()
	writeJSON(w, http.StatusOK, viewFor(r.Context(), url, email, password))
}

// pbConfigBody is the editable connection payload. A blank adminPassword means
// "keep the current one"; a blank adminEmail/url means "keep current".
type pbConfigBody struct {
	URL           string `json:"url"`
	AdminEmail    string `json:"adminEmail"`
	AdminPassword string `json:"adminPassword"`
}

// resolve merges a request body onto the current settings, applying the
// keep-current semantics for blank fields.
func (s *Server) resolve(b pbConfigBody) (url, email, password string) {
	curURL, curEmail, curPassword := s.pbSettings()
	url = normalizeURL(b.URL)
	if url == "" {
		url = curURL
	}
	email = strings.TrimSpace(b.AdminEmail)
	if email == "" {
		email = curEmail
	}
	password = b.AdminPassword
	if password == "" {
		password = curPassword
	}
	return
}

// POST /api/admin/pb-config/test — probe a candidate connection WITHOUT applying
// it, so a superadmin can verify before saving.
func (s *Server) handleTestPBConfig(w http.ResponseWriter, r *http.Request) {
	var b pbConfigBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	url, email, password := s.resolve(b)
	writeJSON(w, http.StatusOK, probePB(r.Context(), url, email, password))
}

// PUT /api/admin/pb-config — apply a new PocketBase connection at runtime and
// persist it to .env. Returns the new config plus a fresh probe.
func (s *Server) handleUpdatePBConfig(w http.ResponseWriter, r *http.Request) {
	var b pbConfigBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if normalizeURL(b.URL) == "" {
		writeError(w, http.StatusBadRequest, "a PocketBase URL is required")
		return
	}
	url, email, password := s.resolve(b)

	// Apply at runtime, then persist so the change survives a restart.
	s.setPBConfig(url, email, password)
	if err := config.UpdateEnvFile(config.EnvFile, map[string]string{
		"POCKETBASE_URL":    url,
		"PB_ADMIN_EMAIL":    email,
		"PB_ADMIN_PASSWORD": password,
	}); err != nil {
		log.Printf("pb-config: persist to %s failed: %v", config.EnvFile, err)
		writeJSON(w, http.StatusOK, map[string]any{
			"config":  viewFor(r.Context(), url, email, password),
			"warning": "applied for this session, but could not be saved to .env: " + err.Error(),
		})
		return
	}

	log.Printf("pb-config: PocketBase connection updated to %s (by superadmin)", url)
	writeJSON(w, http.StatusOK, map[string]any{
		"config": viewFor(r.Context(), url, email, password),
	})
}

// webAppConfigView is the Web App shape returned to the panel: where the Web App
// lives (the address /api/status probes) and which browser origins CORS lets
// call this server.
type webAppConfigView struct {
	URL          string    `json:"url"`
	AllowOrigins []string  `json:"allowOrigins"`
	Probe        svcHealth `json:"probe"`
}

// webAppViewFor builds the panel's Web App view, including a live probe.
func webAppViewFor(ctx context.Context, url string, origins []string) webAppConfigView {
	return webAppConfigView{
		URL:          url,
		AllowOrigins: origins,
		Probe:        probe(ctx, url+"/healthz"),
	}
}

// GET /api/admin/webapp-config — current Web App settings + a live probe.
func (s *Server) handleGetWebAppConfig(w http.ResponseWriter, r *http.Request) {
	url, origins := s.webAppSettings()
	writeJSON(w, http.StatusOK, webAppViewFor(r.Context(), url, origins))
}

// webAppConfigBody is the editable Web App payload. A blank url means "keep the
// current one"; allowOrigins is taken as sent (it is a complete list, not a
// patch), so it may only be omitted, never emptied.
type webAppConfigBody struct {
	URL          string   `json:"url"`
	AllowOrigins []string `json:"allowOrigins"`
}

// POST /api/admin/webapp-config/test — probe a candidate Web App address WITHOUT
// applying it. CORS origins are not probeable, so this only checks the URL.
func (s *Server) handleTestWebAppConfig(w http.ResponseWriter, r *http.Request) {
	var b webAppConfigBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	url := normalizeURL(b.URL)
	if url == "" {
		url, _ = s.webAppSettings()
	}
	writeJSON(w, http.StatusOK, probe(r.Context(), url+"/healthz"))
}

// PUT /api/admin/webapp-config — apply new Web App settings at runtime and
// persist them to .env. The CORS middleware re-reads the origin list on every
// request, so a change here takes effect without a restart.
func (s *Server) handleUpdateWebAppConfig(w http.ResponseWriter, r *http.Request) {
	var b webAppConfigBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	url := normalizeURL(b.URL)
	if url == "" {
		writeError(w, http.StatusBadRequest, "a Web App URL is required")
		return
	}
	origins := cleanOrigins(b.AllowOrigins)
	if len(origins) == 0 {
		writeError(w, http.StatusBadRequest, "at least one allowed origin is required (use * for any)")
		return
	}

	// Apply at runtime, then persist so the change survives a restart.
	s.setWebAppConfig(url, origins)
	if err := config.UpdateEnvFile(config.EnvFile, map[string]string{
		"WEBAPP_URL":         url,
		"CORS_ALLOW_ORIGINS": strings.Join(origins, ","),
	}); err != nil {
		// The runtime change already took effect; report that persistence failed.
		log.Printf("webapp-config: persist to %s failed: %v", config.EnvFile, err)
		writeJSON(w, http.StatusOK, map[string]any{
			"config":  webAppViewFor(r.Context(), url, origins),
			"warning": "applied for this session, but could not be saved to .env: " + err.Error(),
		})
		return
	}

	log.Printf("webapp-config: Web App updated to %s, origins %s (by superadmin)", url, strings.Join(origins, ","))
	writeJSON(w, http.StatusOK, map[string]any{
		"config": webAppViewFor(r.Context(), url, origins),
	})
}

// cleanOrigins trims each origin and drops the blanks, so a trailing comma or a
// stray space in the panel's text field can't register an unmatchable origin.
func cleanOrigins(in []string) []string {
	out := make([]string, 0, len(in))
	for _, o := range in {
		if o = strings.TrimSpace(o); o != "" {
			out = append(out, o)
		}
	}
	return out
}
