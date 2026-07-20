package api

import (
	"context"
	"net/http"

	"smsgateway/apiserver/internal/pb"
)

// Role names as stored in the PocketBase users.role select field. A missing or
// empty value is treated as roleUser.
const (
	roleUser       = "user"
	roleAdmin      = "admin"
	roleSuperadmin = "superadmin"
)

// callerIdentity is who the request token belongs to. It is resolved from
// PocketBase on each request (see identify), so a role change or a deletion
// takes effect immediately rather than lingering until the token expires.
type callerIdentity struct {
	ID       string
	Email    string
	Name     string
	Role     string
	OrgID    string // organization record id ("" = none); scopes what an admin manages
	Verified bool
}

func (c *callerIdentity) isSuperadmin() bool { return c != nil && c.Role == roleSuperadmin }
func (c *callerIdentity) isManager() bool {
	return c != nil && (c.Role == roleAdmin || c.Role == roleSuperadmin)
}

// caller returns the identity stashed on the request context by requireUser.
func caller(r *http.Request) *callerIdentity {
	if v, ok := r.Context().Value(ctxUser).(*callerIdentity); ok {
		return v
	}
	return nil
}

// identify resolves the caller's id/email/name/role from their PocketBase token
// by asking PocketBase to refresh it. A non-200 status means the token is
// invalid or expired.
func (s *Server) identify(ctx context.Context, token string) (*callerIdentity, int, error) {
	res, status, err := s.pb.AuthRefresh(ctx, colUsers, token)
	if err != nil {
		return nil, 0, err
	}
	if status != http.StatusOK || res == nil {
		return nil, status, nil
	}
	rec := res.Record
	role := asString(rec["role"])
	if role == "" {
		role = roleUser
	}
	verified, _ := rec["verified"].(bool)
	return &callerIdentity{
		ID:       asString(rec["id"]),
		Email:    asString(rec["email"]),
		Name:     asString(rec["name"]),
		Role:     role,
		OrgID:    asString(rec["organization"]),
		Verified: verified,
	}, http.StatusOK, nil
}

// authenticate resolves and validates the caller, writing the error response
// itself and returning ok=false when the request should not proceed.
func (s *Server) authenticate(w http.ResponseWriter, r *http.Request) (*callerIdentity, bool) {
	token := bearerToken(r)
	if token == "" {
		writeError(w, http.StatusUnauthorized, "missing bearer token")
		return nil, false
	}
	who, status, err := s.identify(r.Context(), token)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "cannot reach PocketBase", "detail": err.Error()})
		return nil, false
	}
	if status != http.StatusOK || who == nil {
		writeError(w, http.StatusUnauthorized, "invalid or expired token")
		return nil, false
	}
	return who, true
}

// requireRole is the shared gate for privileged handlers: it needs the service
// account (every privileged flow runs through it) and a caller satisfying ok.
func (s *Server) requireRole(next http.HandlerFunc, ok func(*callerIdentity) bool, denied string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.pb.Configured() {
			writeError(w, http.StatusServiceUnavailable, "user management not configured on the server")
			return
		}
		who, valid := s.authenticate(w, r)
		if !valid {
			return
		}
		if !ok(who) {
			writeError(w, http.StatusForbidden, denied)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), ctxUser, who)))
	}
}

// requireManager wraps a handler so only managers (admin or superadmin) proceed.
func (s *Server) requireManager(next http.HandlerFunc) http.HandlerFunc {
	return s.requireRole(next, func(c *callerIdentity) bool { return c.isManager() }, "admin role required")
}

// requireSuperadmin wraps a handler so only superadmins proceed.
func (s *Server) requireSuperadmin(next http.HandlerFunc) http.HandlerFunc {
	return s.requireRole(next, func(c *callerIdentity) bool { return c.isSuperadmin() }, "superadmin role required")
}

// requireSuperadminAuth gates a handler on a superadmin caller WITHOUT requiring
// the service account to already be configured. Used by the PocketBase settings
// endpoints, whose whole purpose is to configure that service account.
func (s *Server) requireSuperadminAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		who, valid := s.authenticate(w, r)
		if !valid {
			return
		}
		if !who.isSuperadmin() {
			writeError(w, http.StatusForbidden, "superadmin role required")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), ctxUser, who)))
	}
}

// loginRequest accepts either "email" or "identity" so the panel and other
// PocketBase-style clients can both log in.
type loginRequest struct {
	Email    string `json:"email"`
	Identity string `json:"identity"`
	Password string `json:"password"`
}

// userDTO is the compact identity shape embedded in auth responses.
type userDTO struct {
	ID           string `json:"id"`
	Email        string `json:"email"`
	Name         string `json:"name,omitempty"`
	Role         string `json:"role"`
	Organization string `json:"organization,omitempty"` // org record id, so the panel can default an admin's actions to their own org
	Verified     bool   `json:"verified"`
}

// handleLogin authenticates against the PocketBase users collection and returns
// the PocketBase token to the client. The API Server is a pure proxy here: the
// token the client receives is PocketBase's own, and PocketBase's address is
// never exposed. The response carries the token as both "access_token" (the
// shape existing GsmNode clients expect) and "token" (the PocketBase shape).
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	identity := req.Identity
	if identity == "" {
		identity = req.Email
	}
	if identity == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	res, err := s.pb.AuthWithPassword(r.Context(), colUsers, identity, req.Password)
	if err != nil {
		// PocketBase answers 400/404 for bad credentials — don't leak which.
		if apiErr, ok := err.(*pb.APIError); ok && (apiErr.Status == http.StatusBadRequest || apiErr.Status == http.StatusNotFound) {
			writeError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "cannot reach PocketBase", "detail": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": res.Token,
		"token":        res.Token,
		"token_type":   "Bearer",
		"user":         recordToUser(res.Record),
		"record":       res.Record,
	})
}

// handleValidate confirms a token is still valid (used by the panel to restore a
// session from a previous visit).
func (s *Server) handleValidate(w http.ResponseWriter, r *http.Request) {
	token := bearerToken(r)
	if token == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"valid": false})
		return
	}
	who, status, err := s.identify(r.Context(), token)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"valid": false, "detail": err.Error()})
		return
	}
	if status != http.StatusOK || who == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"valid": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"valid": true, "user": userDTO{ID: who.ID, Email: who.Email, Name: who.Name, Role: who.Role, Organization: who.OrgID, Verified: who.Verified}})
}

// handleRefresh exchanges a still-valid token for a fresh PocketBase token,
// returning the same shape as login. It validates the token itself, so it needs
// no auth gate.
func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	token := bearerToken(r)
	if token == "" {
		writeError(w, http.StatusUnauthorized, "missing bearer token")
		return
	}
	res, status, err := s.pb.AuthRefresh(r.Context(), colUsers, token)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "cannot reach PocketBase", "detail": err.Error()})
		return
	}
	if status != http.StatusOK || res == nil {
		writeError(w, http.StatusUnauthorized, "invalid or expired token")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": res.Token,
		"token":        res.Token,
		"token_type":   "Bearer",
		"user":         recordToUser(res.Record),
		"record":       res.Record,
	})
}

// handleMe returns the authenticated caller's identity, including their role.
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	who := caller(r)
	if who == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	writeJSON(w, http.StatusOK, userDTO{ID: who.ID, Email: who.Email, Name: who.Name, Role: who.Role, Organization: who.OrgID, Verified: who.Verified})
}

// recordToUser projects a PocketBase user record to the compact identity shape.
func recordToUser(rec pb.Record) userDTO {
	role := asString(rec["role"])
	if role == "" {
		role = roleUser
	}
	verified, _ := rec["verified"].(bool)
	return userDTO{
		ID:           asString(rec["id"]),
		Email:        asString(rec["email"]),
		Name:         asString(rec["name"]),
		Role:         role,
		Organization: asString(rec["organization"]),
		Verified:     verified,
	}
}
