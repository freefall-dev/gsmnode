// Package auth issues and verifies the client-facing JWTs used by the Web App
// and other integrators.
package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims is the JWT payload for an authenticated user session.
type Claims struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
	jwt.RegisteredClaims
}

// Manager issues and verifies JWTs.
type Manager struct {
	secret []byte
	ttl    time.Duration
}

// NewManager creates a JWT manager.
func NewManager(secret []byte, ttl time.Duration) *Manager {
	return &Manager{secret: secret, ttl: ttl}
}

// Issue creates a signed access token for the given user.
func (m *Manager) Issue(userID, email, name string) (token string, expiresAt time.Time, err error) {
	now := time.Now()
	expiresAt = now.Add(m.ttl)
	claims := Claims{
		Email: email,
		Name:  name,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := t.SignedString(m.secret)
	return signed, expiresAt, err
}

// Verify parses and validates a token, returning its claims.
func (m *Manager) Verify(token string) (*Claims, error) {
	claims := &Claims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return m.secret, nil
	})
	if err != nil {
		return nil, err
	}
	if !parsed.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}
