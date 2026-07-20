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

// POST /api/orgs — create an organization. Body: {name}. A superadmin may create
// any number of organizations and is not attached to them (they manage every
// tenant centrally). Any other user may create one only if they don't already
// belong to an organization, and they become its admin and first member — the
// self-service path for standing up a new tenant.
func (s *Server) handleCreateOrg(w http.ResponseWriter, r *http.Request) {
	if !s.pb.Configured() {
		writeError(w, http.StatusServiceUnavailable, "organizations are not configured on the server")
		return
	}
	who := caller(r)
	if !who.isSuperadmin() && who.OrgID != "" {
		writeError(w, http.StatusForbidden, "you already belong to an organization")
		return
	}
	name, ok := decodeOrgName(w, r)
	if !ok {
		return
	}
	rec, err := s.pb.Create(r.Context(), colOrgs, map[string]any{"name": name})
	if err != nil {
		writePBRelay(w, err) // surfaces PocketBase validation (e.g. duplicate name)
		return
	}
	org := recordToOrgView(rec)

	// A non-superadmin creator becomes the admin of, and first member of, the org
	// they just made. If that promotion fails, roll the org back so we never leave
	// a stranded organization that nobody can administer.
	if !who.isSuperadmin() {
		if _, err := s.pb.Update(r.Context(), colUsers, who.ID, map[string]any{
			"organization": org.ID,
			"role":         roleAdmin,
		}); err != nil {
			_ = s.pb.Delete(r.Context(), colOrgs, org.ID)
			writePBRelay(w, err)
			return
		}
	}
	writeJSON(w, http.StatusCreated, map[string]any{"organization": org})
}

// PATCH /api/orgs/{id} — rename an organization (manager only). A superadmin may
// rename any organization; an admin may rename only their own. Body: {name}.
func (s *Server) handleUpdateOrg(w http.ResponseWriter, r *http.Request) {
	who := caller(r)
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing organization id")
		return
	}
	// An admin is scoped to their own organization; a superadmin spans all.
	if !who.isSuperadmin() && (who.OrgID == "" || id != who.OrgID) {
		writeError(w, http.StatusForbidden, "you can only rename your own organization")
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

// DELETE /api/orgs/{id} — delete an organization (manager only). A superadmin may
// delete any organization, but only once it has no members. An admin may delete
// only their own organization, and only when they are its sole member: the admin
// is detached and demoted back to a plain user, then the org is removed. Either
// path refuses to silently orphan other members.
func (s *Server) handleDeleteOrg(w http.ResponseWriter, r *http.Request) {
	who := caller(r)
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing organization id")
		return
	}

	if who.isSuperadmin() {
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
	} else {
		// An admin is scoped to their own organization.
		if who.OrgID == "" || id != who.OrgID {
			writeError(w, http.StatusForbidden, "you can only delete your own organization")
			return
		}
		// Refuse if anyone other than the admin still belongs to it.
		others, err := s.pb.List(r.Context(), colUsers, pb.ListOptions{
			Filter:  "organization = " + pbQuote(id) + " && id != " + pbQuote(who.ID),
			PerPage: 1,
		})
		if err != nil {
			writeUpstreamError(w, err)
			return
		}
		if others.TotalItems > 0 {
			writeError(w, http.StatusConflict, "organization still has other members; reassign or remove them first")
			return
		}
		// Detach and demote the admin so the org is empty before it is removed.
		if _, err := s.pb.Update(r.Context(), colUsers, who.ID, map[string]any{
			"organization": "",
			"role":         roleUser,
		}); err != nil {
			writePBRelay(w, err)
			return
		}
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
