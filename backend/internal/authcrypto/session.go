package authcrypto

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

const sessionTokenBytes = 32

// NewSessionToken returns a 256-bit cryptographically random URL-safe token.
func NewSessionToken() (string, error) {
	value := make([]byte, sessionTokenBytes)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

// DigestSessionToken returns the SHA-256 digest suitable for session storage.
// Store this digest rather than the bearer token returned by NewSessionToken.
func DigestSessionToken(token string) [sha256.Size]byte {
	return sha256.Sum256([]byte(token))
}
