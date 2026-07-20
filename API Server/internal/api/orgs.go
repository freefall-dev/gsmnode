package api

import (
	"context"
	"net/http"
	"strings"

	"smsgateway/apiserver/internal/pb"
)

// orgView is the trimmed organization shape returned to clients.
type orgView struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Created string `json:"created"`
}

func recordToOrgView(rec pb.Record) orgView {
	return orgView{
		ID:      asString(rec["id"]),
		Name:    asString(rec["name"]),
		Created: asString(rec["created"]),
	}
}

// orgNameMap returns an id→name map of all organizations. On any error it
// returns a non-nil (possibly empty) map, so callers can index it safely.
func (s *Server) orgNameMap(ctx context.Context) map[string]string {
	out := map[string]string{}
	if !s.pb.Configured() {
		return out
	}
	res, err := s.pb.List(ctx, colOrgs, pb.ListOptions{PerPage: 500})
	if err != nil {
		return out
	}
	for _, rec := range res.Items {
		out[asString(rec["id"])] = asString(rec["name"])
	}
	return out
}

// orgName resolves a single organization's name (best effort; "" on miss).
func (s *Server) orgName(ctx context.Context, id string) string {
	if id == "" || !s.pb.Configured() {
		return ""
	}
	rec, err := s.pb.GetOne(ctx, colOrgs, id)
	if err != nil {
		return ""
	}
	return asString(rec["name"])
}

// GET /api/orgs — list organizations (manager only). Superadmins see all;
// admins see only their own organization.
func (s *Server) handleListOrgs(w http.ResponseWriter, r *http.Request) {
	who := caller(r)
	opt := pb.ListOptions{Sort: "name", PerPage: 500}
	if !who.isSuperadmin() {
		// An org-less admin manages no organization.
		if who.OrgID == "" {
			writeJSON(w, http.StatusOK, map[string]any{"organizations": []orgView{}})
			return
		}
		opt.Filter = "id = " + pbQuote(who.OrgID)
	}
	res, err := s.pb.List(r.Context(), colOrgs, opt)
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	out := make([]orgView, 0, len(res.Items))
	for _, rec := range res.Items {
		out = append(out, recordToOrgView(rec))
	}
	writeJSON(w, http.StatusOK, map[string]any{"organizations": out})
}

// POST /api/orgs — create an organization (superadmin only). Body: {name}.
func (s *Server) handleCreateOrg(w http.ResponseWriter, r *http.Request) {
	name, ok := decodeOrgName(w, r)
	if !ok {
		return
	}
	rec, err := s.pb.Create(r.Context(), colOrgs, map[string]any{"name": name})
	if err != nil {
		writePBRelay(w, err) // surfaces PocketBase validation (e.g. duplicate name)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"organization": recordToOrgView(rec)})
}

// PATCH /api/orgs/{id} — rename an organization (superadmin only). Body: {name}.
func (s *Server) handleUpdateOrg(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing organization id")
		return
	}
	name, ok := decodeOrgName(w, r)
	if !ok {
		return
	}
	rec, err := s.pb.Update(r.Context(), colOrgs, id, map[string]any{"name": name})
	if err != nil {
		writePBRelay(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"organization": recordToOrgView(rec)})
}

// DELETE /api/orgs/{id} — delete an organization (superadmin only). Refused
// while the org still has members, to avoid silently orphaning users.
func (s *Server) handleDeleteOrg(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing organization id")
		return
	}

	// Guard: block deletion while any user still belongs to this org.
	members, err := s.pb.List(r.Context(), colUsers,
		pb.ListOptions{Filter: "organization = " + pbQuote(id), PerPage: 1})
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	if members.TotalItems > 0 {
		writeError(w, http.StatusConflict, "organization still has members; reassign or remove them first")
		return
	}

	if err := s.pb.Delete(r.Context(), colOrgs, id); err != nil {
		writePBRelay(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// decodeOrgName parses and validates a {name} body, writing an error response
// and returning ok=false on failure.
func decodeOrgName(w http.ResponseWriter, r *http.Request) (string, bool) {
	var body struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return "", false
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "organization name is required")
		return "", false
	}
	if len(name) > 120 {
		writeError(w, http.StatusBadRequest, "organization name is too long (max 120)")
		return "", false
	}
	return name, true
}
