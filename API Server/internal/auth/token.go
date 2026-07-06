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
