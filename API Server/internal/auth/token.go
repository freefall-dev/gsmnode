// Package auth mints the opaque tokens used to authenticate registered mobile
// devices. Client (user) sessions no longer live here — those now use the
// PocketBase token proxied by the API Server.
package auth

import (
	"crypto/rand"
	"encoding/hex"
)

// NewDeviceToken returns a cryptographically random opaque token used to
// authenticate a registered mobile device.
func NewDeviceToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
