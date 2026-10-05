package telegramauth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	testClientID = "123456789"
	testNonce    = "server-generated-one-time-nonce"
)

type staticKeys map[string]*rsa.PublicKey

func (s staticKeys) Keys(context.Context) (map[string]*rsa.PublicKey, error) { return s, nil }

func TestVerifyRS256IDToken(t *testing.T) {
	key := mustRSAKey(t, 2048)
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	token := makeToken(t, key, "key-1", "RS256", claimsJSON(now, "https://oauth.telegram.org", `"123456789"`, testNonce, `987654321`))
	verifier := Verifier{Keys: staticKeys{"key-1": &key.PublicKey}}

	got, err := verifier.Verify(context.Background(), token, testClientID, testNonce, now)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if got != (Claims{ID: "987654321", Subject: "subject-123", Name: "Jane Telegram"}) {
		t.Fatalf("Verify() claims = %#v", got)
	}
}

func TestVerifyRejectsInvalidTokens(t *testing.T) {
	key := mustRSAKey(t, 2048)
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	base := func(issuer, audience, nonce string, issuedAt, expires time.Time) string {
		return makeToken(t, key, "key-1", "RS256", fmt.Sprintf(`{"iss":%q,"aud":%s,"sub":"subject-123","id":987654321,"name":"Jane Telegram","nonce":%q,"iat":%d,"exp":%d}`,
			issuer, audience, nonce, issuedAt.Unix(), expires.Unix()))
	}
	valid := base(issuer, `"`+testClientID+`"`, testNonce, now, now.Add(time.Hour))
	noAlgorithm := makeToken(t, key, "key-1", "", claimsJSON(now, issuer, `"`+testClientID+`"`, testNonce, `987654321`))
	badSignatureBytes := []byte(valid)
	signatureStart := strings.LastIndexByte(valid, '.') + 1
	if badSignatureBytes[signatureStart] == 'A' {
		badSignatureBytes[signatureStart] = 'B'
	} else {
		badSignatureBytes[signatureStart] = 'A'
	}
	badSignature := string(badSignatureBytes)
	tooLong := strings.Repeat("a", maxTokenSize+1)
	tooOldIssued := base(issuer, `"`+testClientID+`"`, testNonce, now.Add(-maxTokenAge-time.Second), now.Add(time.Minute))
	tooLongLived := base(issuer, `"`+testClientID+`"`, testNonce, now, now.Add(time.Hour+time.Second))
	tooFarFuture := base(issuer, `"`+testClientID+`"`, testNonce, now.Add(clockSkew+time.Second), now.Add(time.Hour))
	tooFarExpired := base(issuer, `"`+testClientID+`"`, testNonce, now.Add(-time.Minute), now.Add(-clockSkew-time.Second))
	badDateType := makeToken(t, key, "key-1", "RS256", `{"iss":"https://oauth.telegram.org","aud":"123456789","sub":"s","id":1,"nonce":"server-generated-one-time-nonce","iat":"not-a-date","exp":1791288000}`)
	badID := makeToken(t, key, "key-1", "RS256", `{"iss":"https://oauth.telegram.org","aud":"123456789","sub":"s","id":0,"nonce":"server-generated-one-time-nonce","iat":1791288000,"exp":1791291600}`)
	duplicateClaim := makeToken(t, key, "key-1", "RS256", `{"iss":"https://oauth.telegram.org","iss":"https://oauth.telegram.org","aud":"123456789","sub":"s","id":1,"nonce":"server-generated-one-time-nonce","iat":1791288000,"exp":1791291600}`)
	badNonce := base(issuer, `"`+testClientID+`"`, "different-nonce", now, now.Add(time.Minute))
	wrongAudience := base(issuer, `"other-client"`, testNonce, now, now.Add(time.Minute))
	wrongIssuer := base("https://attacker.example", `"`+testClientID+`"`, testNonce, now, now.Add(time.Minute))
	arrayAudience := base(issuer, `[`+`"other-client","`+testClientID+`"]`, testNonce, now, now.Add(time.Minute))
	if _, err := (Verifier{Keys: staticKeys{"key-1": &key.PublicKey}}).Verify(context.Background(), arrayAudience, testClientID, testNonce, now); err != nil {
		t.Errorf("array audience should be accepted: %v", err)
	}

	cases := map[string]string{
		"wrong issuer": wrongIssuer, "wrong audience": wrongAudience, "wrong nonce": badNonce,
		"expired": tooFarExpired, "future iat": tooFarFuture, "old iat": tooOldIssued,
		"overlong lifetime": tooLongLived, "no alg": noAlgorithm, "bad signature": badSignature,
		"oversized token": tooLong, "bad date type": badDateType, "invalid Telegram ID": badID,
		"duplicate claim": duplicateClaim,
	}
	for name, token := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := (Verifier{Keys: staticKeys{"key-1": &key.PublicKey}}).Verify(context.Background(), token, testClientID, testNonce, now); err == nil {
				t.Fatal("Verify() unexpectedly accepted invalid token")
			}
		})
	}
}

func TestVerifyRejectsSmallRSAKey(t *testing.T) {
	key := mustRSAKey(t, 1024)
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	token := makeToken(t, key, "weak-key", "RS256", claimsJSON(now, issuer, `"`+testClientID+`"`, testNonce, `987654321`))
	if _, err := (Verifier{Keys: staticKeys{"weak-key": &key.PublicKey}}).Verify(context.Background(), token, testClientID, testNonce, now); err == nil {
		t.Fatal("Verify() accepted an RSA key smaller than 2048 bits")
	}
}

func TestRemoteKeysUnknownKIDRefreshAndCache(t *testing.T) {
	oldKey, newKey := mustRSAKey(t, 2048), mustRSAKey(t, 2048)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			_, _ = w.Write(jwksJSON(t, "old-kid", &oldKey.PublicKey))
			return
		}
		_, _ = w.Write(jwksJSON(t, "new-kid", &newKey.PublicKey))
	}))
	defer server.Close()
	keys := newRemoteKeys(server.Client(), server.URL)
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	token := makeToken(t, newKey, "new-kid", "RS256", claimsJSON(now, issuer, `"`+testClientID+`"`, testNonce, `987654321`))
	verifier := Verifier{Keys: keys}
	for i := 0; i < 2; i++ {
		if _, err := verifier.Verify(context.Background(), token, testClientID, testNonce, now); err != nil {
			t.Fatalf("Verify() call %d error = %v", i+1, err)
		}
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("JWKS requests = %d, want initial load + one unknown-kid refresh (cached thereafter)", got)
	}
}

func TestRemoteKeysRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", maxJWKSSize+1)))
	}))
	defer server.Close()
	if _, err := newRemoteKeys(server.Client(), server.URL).Keys(context.Background()); err == nil {
		t.Fatal("Keys() accepted an oversized JWKS response")
	}
}

func TestParseJWKSRejectsInvalidRSAParameters(t *testing.T) {
	key := mustRSAKey(t, 1024)
	if _, err := parseJWKS(jwksJSON(t, "small", &key.PublicKey)); err == nil {
		t.Fatal("parseJWKS() accepted an RSA key smaller than 2048 bits")
	}
}

func claimsJSON(now time.Time, issuer, audience, nonce, id string) string {
	return fmt.Sprintf(`{"iss":%q,"aud":%s,"sub":"subject-123","id":%s,"name":"Jane Telegram","nonce":%q,"iat":%d,"exp":%d}`,
		issuer, audience, id, nonce, now.Unix(), now.Add(time.Hour).Unix())
}

func makeToken(t *testing.T, key *rsa.PrivateKey, kid, algorithm, payload string) string {
	t.Helper()
	header, err := json.Marshal(map[string]string{"alg": algorithm, "kid": kid, "typ": "JWT"})
	if err != nil {
		t.Fatal(err)
	}
	encodedHeader := base64.RawURLEncoding.EncodeToString(header)
	encodedPayload := base64.RawURLEncoding.EncodeToString([]byte(payload))
	message := encodedHeader + "." + encodedPayload
	digest := sha256.Sum256([]byte(message))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return message + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func mustRSAKey(t *testing.T, bits int) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func jwksJSON(t *testing.T, kid string, key *rsa.PublicKey) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{"keys": []map[string]string{{
		"kty": "RSA", "kid": kid, "use": "sig", "alg": "RS256",
		"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
		"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	return body
}
