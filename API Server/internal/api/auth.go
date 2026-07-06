package api

import (
	"net/http"
	"strings"
)

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type userDTO struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

type tokenResponse struct {
	AccessToken string  `json:"access_token"`
	TokenType   string  `json:"token_type"`
	ExpiresAt   string  `json:"expires_at"`
	User        userDTO `json:"user"`
}

// handleLogin authenticates a user against PocketBase and issues a client JWT.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	res, err := s.pb.AuthWithPassword(r.Context(), colUsers, req.Email, req.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	user := recordToUser(res.Record)
	token, exp, err := s.jwt.Issue(user.ID, user.Email, user.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not issue token")
		return
	}
	writeJSON(w, http.StatusOK, tokenResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresAt:   exp.UTC().Format("2006-01-02T15:04:05Z07:00"),
		User:        user,
	})
}

// handleRefresh issues a fresh token for an already-authenticated user.
func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	id, email := userFromCtx(r.Context())
	rec, err := s.pb.GetOne(r.Context(), colUsers, id)
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	user := recordToUser(rec)
	if user.Email == "" {
		user.Email = email
	}
	token, exp, err := s.jwt.Issue(user.ID, user.Email, user.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not issue token")
		return
	}
	writeJSON(w, http.StatusOK, tokenResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresAt:   exp.UTC().Format("2006-01-02T15:04:05Z07:00"),
		User:        user,
	})
}

// handleMe returns the current user's profile.
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	id, _ := userFromCtx(r.Context())
	rec, err := s.pb.GetOne(r.Context(), colUsers, id)
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, recordToUser(rec))
}

func recordToUser(rec map[string]any) userDTO {
	return userDTO{
		ID:    asString(rec["id"]),
		Email: asString(rec["email"]),
		Name:  asString(rec["name"]),
	}
}
