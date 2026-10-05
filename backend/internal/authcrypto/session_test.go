package authcrypto

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

func TestNewSessionTokenEntropyAndDigest(t *testing.T) {
	tokens := make(map[string]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		token, err := NewSessionToken()
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := base64.RawURLEncoding.DecodeString(token)
		if err != nil || len(decoded) != sessionTokenBytes {
			t.Fatalf("invalid token encoding or length: len=%d err=%v", len(decoded), err)
		}
		if _, exists := tokens[token]; exists {
			t.Fatalf("duplicate token among 1000 samples")
		}
		tokens[token] = struct{}{}
		if got, want := DigestSessionToken(token), sha256.Sum256([]byte(token)); got != want {
			t.Fatalf("token digest mismatch: got %x want %x", got, want)
		}
	}
	if DigestSessionToken("token") == DigestSessionToken("other") {
		t.Fatal("different tokens produced identical digests")
	}
}
