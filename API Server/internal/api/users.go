package api

import (
	"net/http"
	"strings"

	"smsgateway/apiserver/internal/pb"
)

// userView is the trimmed user shape returned to managers.
type userView struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	Verified bool   `json:"verified"`
	Created  string `json:"created"`
}

func recordToUserView(rec pb.Record) userView {
	role := asString(rec["role"])
	if role == "" {
		role = roleUser
	}
	verified, _ := rec["verified"].(bool)
	return userView{
		ID:       asString(rec["id"]),
		Email:    asString(rec["email"]),
		Name:     asString(rec["name"]),
		Role:     role,
		Verified: verified,
		Created:  asString(rec["created"]),
	}
}

// getUserRecord fetches a single user, returning nil (not an error) when the
// user does not exist. Used to enforce role scoping before an edit or delete.
func (s *Server) getUserRecord(r *http.Request, id string) (*userView, error) {
	rec, err := s.pb.GetOne(r.Context(), colUsers, id)
	if err != nil {
		if pb.NotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	v := recordToUserView(rec)
	return &v, nil
}

// GET /api/users — list all users (manager only), sorted by email.
func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	res, err := s.pb.List(r.Context(), colUsers, pb.ListOptions{Sort: "email", PerPage: 500})
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	out := make([]userView, 0, len(res.Items))
	for _, rec := range res.Items {
		out = append(out, recordToUserView(rec))
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": out})
}

// POST /api/users — create a user (manager only). Body: {email, password, role,
// name?}. Only a superadmin may mint superadmins.
func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	who := caller(r)
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Role     string `json:"role"`
		Name     string `json:"name"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	body.Email = strings.TrimSpace(strings.ToLower(body.Email))
	if body.Email == "" || !strings.Contains(body.Email, "@") {
		writeError(w, http.StatusBadRequest, "a valid email is required")
		return
	}
	if len(body.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	role, ok := normalizeRole(body.Role)
	if !ok {
		writeError(w, http.StatusBadRequest, "role must be 'user', 'admin', or 'superadmin'")
		return
	}
	if role == roleSuperadmin && !who.isSuperadmin() {
		writeError(w, http.StatusForbidden, "only a superadmin can create superadmins")
		return
	}

	create := map[string]any{
		"email":           body.Email,
		"password":        body.Password,
		"passwordConfirm": body.Password,
		"name":            strings.TrimSpace(body.Name),
		"role":            role,
		"verified":        true,
		"emailVisibility": false,
	}
	rec, err := s.pb.Create(r.Context(), colUsers, create)
	if err != nil {
		writePBRelay(w, err) // surfaces PocketBase validation (e.g. duplicate email)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"user": recordToUserView(rec)})
}

// PATCH /api/users/{id} — edit a user (manager only). Any subset of
// {email, name, role, password, verified} may be supplied. Admins cannot touch
// superadmins or grant the superadmin role; nobody can change their own role.
func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	who := caller(r)
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing user id")
		return
	}

	target, err := s.getUserRecord(r, id)
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	if target == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if !who.isSuperadmin() && target.Role == roleSuperadmin {
		writeError(w, http.StatusForbidden, "you cannot edit a superadmin")
		return
	}

	var body struct {
		Email    string `json:"email"`
		Name     *string `json:"name"`
		Role     string `json:"role"`
		Password string `json:"password"`
		Verified *bool  `json:"verified"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	patch := map[string]any{}
	if email := strings.TrimSpace(strings.ToLower(body.Email)); email != "" {
		if !strings.Contains(email, "@") {
			writeError(w, http.StatusBadRequest, "a valid email is required")
			return
		}
		patch["email"] = email
	}
	if body.Name != nil {
		patch["name"] = strings.TrimSpace(*body.Name)
	}
	if body.Role != "" {
		role, ok := normalizeRole(body.Role)
		if !ok {
			writeError(w, http.StatusBadRequest, "role must be 'user', 'admin', or 'superadmin'")
			return
		}
		if role == roleSuperadmin && !who.isSuperadmin() {
			writeError(w, http.StatusForbidden, "only a superadmin can grant the superadmin role")
			return
		}
		if who != nil && who.ID == id && role != who.Role {
			writeError(w, http.StatusBadRequest, "you cannot change your own role")
			return
		}
		patch["role"] = role
	}
	if body.Password != "" {
		if len(body.Password) < 8 {
			writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
			return
		}
		patch["password"] = body.Password
		patch["passwordConfirm"] = body.Password
	}
	if body.Verified != nil {
		patch["verified"] = *body.Verified
	}
	if len(patch) == 0 {
		writeError(w, http.StatusBadRequest, "no changes provided")
		return
	}

	rec, err := s.pb.Update(r.Context(), colUsers, id, patch)
	if err != nil {
		writePBRelay(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": recordToUserView(rec)})
}

// DELETE /api/users/{id} — delete a user (manager only). Admins may not delete
// superadmins; nobody can delete their own account.
func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	who := caller(r)
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing user id")
		return
	}
	if who != nil && who.ID == id {
		writeError(w, http.StatusBadRequest, "you cannot delete your own account")
		return
	}

	target, err := s.getUserRecord(r, id)
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	if target == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if !who.isSuperadmin() && target.Role == roleSuperadmin {
		writeError(w, http.StatusForbidden, "you cannot delete a superadmin")
		return
	}

	if err := s.pb.Delete(r.Context(), colUsers, id); err != nil {
		writePBRelay(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// normalizeRole validates a client-supplied role, returning the canonical value
// and whether it was recognised. Empty means "user".
func normalizeRole(role string) (string, bool) {
	switch strings.TrimSpace(strings.ToLower(role)) {
	case "", roleUser:
		return roleUser, true
	case roleAdmin:
		return roleAdmin, true
	case roleSuperadmin:
		return roleSuperadmin, true
	default:
		return "", false
	}
}
