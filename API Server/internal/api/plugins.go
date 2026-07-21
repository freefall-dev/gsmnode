package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"smsgateway/apiserver/internal/plugins"
)

// GET /api/admin/plugins — every known plugin (registry ∪ persisted), secrets masked.
func (s *Server) handleListPlugins(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"plugins": s.plugins.List()})
}

// GET /api/admin/plugins/{name} — one plugin's view.
func (s *Server) handleGetPlugin(w http.ResponseWriter, r *http.Request) {
	v, ok := s.plugins.Get(r.PathValue("name"))
	if !ok {
		writeError(w, http.StatusNotFound, "unknown plugin")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"plugin": v})
}

// PUT /api/admin/plugins/{name} — enable/disable + merge config. Body:
// {enabled?, config?}. A secret left at the mask keeps its stored value.
func (s *Server) handleUpdatePlugin(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	current, ok := s.plugins.Get(name)
	if !ok {
		writeError(w, http.StatusNotFound, "unknown plugin")
		return
	}
	var body struct {
		Enabled *bool             `json:"enabled"`
		Config  map[string]string `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	enabled := current.Enabled
	if body.Enabled != nil {
		enabled = *body.Enabled
	}

	v, err := s.plugins.Upsert(r.Context(), name, enabled, body.Config)
	if err != nil {
		if plugins.IsUnknown(err) {
			writeError(w, http.StatusNotFound, "unknown plugin")
			return
		}
		// A failed init (e.g. bad credentials) is reported but the state was saved.
		writeJSON(w, http.StatusOK, map[string]any{"plugin": v, "warning": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"plugin": v})
}

// POST /api/admin/plugins — register an external (remote HTTP) plugin. Body:
// {name, baseURL, provider?}. This is the "add a plugin without a rebuild" path.
func (s *Server) handleRegisterPlugin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string `json:"name"`
		BaseURL  string `json:"baseURL"`
		Provider string `json:"provider"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" || body.BaseURL == "" {
		writeError(w, http.StatusBadRequest, "name and baseURL are required")
		return
	}
	if err := s.plugins.RegisterExternal(body.Name, body.BaseURL, body.Provider); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	v, _ := s.plugins.Get(body.Name)
	writeJSON(w, http.StatusCreated, map[string]any{"plugin": v})
}

// DELETE /api/admin/plugins/{name} — remove an external plugin (builtins can
// only be disabled).
func (s *Server) handleDeletePlugin(w http.ResponseWriter, r *http.Request) {
	if err := s.plugins.Remove(r.Context(), r.PathValue("name")); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// POST /api/admin/plugins/{name}/health — run a health check now. Works on
// disabled plugins too, so a config can be verified before enabling it.
func (s *Server) handlePluginHealth(w http.ResponseWriter, r *http.Request) {
	h, err := s.plugins.HealthCheck(r.Context(), r.PathValue("name"))
	if err != nil {
		if plugins.IsUnknown(err) {
			writeError(w, http.StatusNotFound, "unknown plugin")
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"health": h})
}
