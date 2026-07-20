package api

import (
	"net/http"
	"strings"
)

// Self-service account endpoints — a signed-in user editing their own record.
// Unlike the user-management endpoints in users.go (manager-gated, arbitrary
// target), these always act on the caller and expose only the two fields it is
// safe for anyone to change about themselves: their display name and password.

// PATCH /api/auth/me — update the caller's own profile. Only the display name is
// editable here; role, email, verification, and organization are managed by an
// admin through the user-management endpoints.
func (s *Server) handleUpdateMe(w http.ResponseWriter, r *http.Request) {
	who := caller(r)
	if who == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var body struct {
		Name *string `json:"name"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	patch := map[string]any{}
	if body.Name != nil {
		patch["name"] = strings.TrimSpace(*body.Name)
	}
	if len(patch) == 0 {
		writeError(w, http.StatusBadRequest, "no changes provided")
		return
	}

	rec, err := s.pb.Update(r.Context(), colUsers, who.ID, patch)
	if err != nil {
		writePBRelay(w, err)
		return
	}
	writeJSON(w, http.StatusOK, recordToUser(rec))
}

// POST /api/auth/change-password — change the caller's own password. The current
// password is re-verified against PocketBase (the API Server otherwise only ever
// talks to PocketBase as the service account), so a stolen token alone cannot
// change the password.
func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	who := caller(r)
	if who == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var body struct {
		OldPassword string `json:"oldPassword"`
		NewPassword string `json:"newPassword"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.OldPassword == "" {
		writeError(w, http.StatusBadRequest, "current password is required")
		return
	}
	if len(body.NewPassword) < 8 {
		writeError(w, http.StatusBadRequest, "new password must be at least 8 characters")
		return
	}

	// Verify the current password the same way login does.
	if _, err := s.pb.AuthWithPassword(r.Context(), colUsers, who.Email, body.OldPassword); err != nil {
		writeError(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}

	patch := map[string]any{"password": body.NewPassword, "passwordConfirm": body.NewPassword}
	if _, err := s.pb.Update(r.Context(), colUsers, who.ID, patch); err != nil {
		writePBRelay(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
