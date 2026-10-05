package telegramauth

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"strconv"
	"strings"
	"time"
)

const (
	issuer           = "https://oauth.telegram.org"
	maxTokenSize     = 16 << 10
	clockSkew        = 30 * time.Second
	maxTokenAge      = 5 * time.Minute
	maxTokenLifetime = time.Hour
)

// ErrInvalidToken is returned for all malformed or invalid ID tokens. It does
// not include token contents or identify which check failed.
var ErrInvalidToken = errors.New("invalid Telegram ID token")

// ErrKeyUnavailable indicates that Telegram's signing keys could not be
// loaded. Details from the remote server are intentionally not exposed.
var ErrKeyUnavailable = errors.New("Telegram signing keys unavailable")

// KeySource provides Telegram signing keys indexed by their JWKS key ID.
type KeySource interface {
	Keys(context.Context) (map[string]*rsa.PublicKey, error)
}

// Verifier validates Telegram Login OIDC ID tokens signed with RS256.
type Verifier struct {
	Keys KeySource
}

// Claims contains the identity fields needed by the application. ID comes
// from Telegram's numeric id claim; Subject is the OIDC sub claim.
type Claims struct {
	ID      string
	Subject string
	Name    string
}

// Verify validates a Telegram Login ID token and returns its authenticated
// identity. expectedNonce must be the one-time nonce issued for this login.
func (v Verifier) Verify(ctx context.Context, token, clientID, expectedNonce string, now time.Time) (Claims, error) {
	if len(token) == 0 || len(token) > maxTokenSize || clientID == "" || expectedNonce == "" || v.Keys == nil {
		return Claims{}, ErrInvalidToken
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return Claims{}, ErrInvalidToken
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Claims{}, ErrInvalidToken
	}
	var header struct {
		Algorithm string `json:"alg"`
		KeyID     string `json:"kid"`
	}
	if err := decodeUniqueObject(headerBytes, &header); err != nil || header.Algorithm != "RS256" || header.KeyID == "" || len(header.KeyID) > 256 {
		return Claims{}, ErrInvalidToken
	}
	// Critical extensions and unencoded payloads are not part of Telegram's
	// supported JWT profile. Reject them rather than silently ignoring them.
	var headerFields map[string]json.RawMessage
	if err := json.Unmarshal(headerBytes, &headerFields); err != nil {
		return Claims{}, ErrInvalidToken
	}
	if _, ok := headerFields["crit"]; ok {
		return Claims{}, ErrInvalidToken
	}
	if b64, ok := headerFields["b64"]; ok {
		var enabled bool
		if json.Unmarshal(b64, &enabled) != nil || !enabled {
			return Claims{}, ErrInvalidToken
		}
	}

	keys, err := v.Keys.Keys(ctx)
	if err != nil {
		return Claims{}, ErrKeyUnavailable
	}
	key := keys[header.KeyID]
	if key == nil {
		if refresher, ok := v.Keys.(interface{ Refresh(context.Context) error }); ok {
			if refresher.Refresh(ctx) == nil {
				keys, err = v.Keys.Keys(ctx)
				if err == nil {
					key = keys[header.KeyID]
				}
			}
		}
	}
	if !validPublicKey(key) {
		return Claims{}, ErrInvalidToken
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(signature) != key.Size() {
		return Claims{}, ErrInvalidToken
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature); err != nil {
		return Claims{}, ErrInvalidToken
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, ErrInvalidToken
	}
	var payload map[string]json.RawMessage
	if err := decodeUniqueObject(payloadBytes, &payload); err != nil {
		return Claims{}, ErrInvalidToken
	}
	var claims struct {
		Issuer   string          `json:"iss"`
		Audience json.RawMessage `json:"aud"`
		Subject  string          `json:"sub"`
		ID       json.RawMessage `json:"id"`
		Name     string          `json:"name"`
		Nonce    string          `json:"nonce"`
		IssuedAt json.Number     `json:"iat"`
		Expires  json.Number     `json:"exp"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(payloadBytes)))
	decoder.UseNumber()
	if err := decoder.Decode(&claims); err != nil {
		return Claims{}, ErrInvalidToken
	}
	if claims.Issuer != issuer || !hasAudience(claims.Audience, clientID) || claims.Subject == "" || !nonceMatches(claims.Nonce, expectedNonce) {
		return Claims{}, ErrInvalidToken
	}
	issuedAt, okIAT := numericDate(claims.IssuedAt)
	expires, okExp := numericDate(claims.Expires)
	if !okIAT || !okExp {
		return Claims{}, ErrInvalidToken
	}
	now = now.UTC()
	issuedTime := time.Unix(issuedAt, 0)
	expiresTime := time.Unix(expires, 0)
	if issuedTime.After(now.Add(clockSkew)) || issuedTime.Before(now.Add(-maxTokenAge)) ||
		expiresTime.Before(now.Add(-clockSkew)) || expiresTime.After(now.Add(maxTokenLifetime)) ||
		expiresTime.Before(issuedTime) || expiresTime.Sub(issuedTime) > maxTokenLifetime {
		return Claims{}, ErrInvalidToken
	}
	id, ok := telegramID(claims.ID)
	if !ok {
		return Claims{}, ErrInvalidToken
	}
	return Claims{ID: id, Subject: claims.Subject, Name: claims.Name}, nil
}

func validPublicKey(key *rsa.PublicKey) bool {
	return key != nil && key.N != nil && key.N.BitLen() >= 2048 && key.N.BitLen() <= 8192 && key.E >= 3 && key.E <= 2147483647 && key.E%2 == 1
}

func hasAudience(raw json.RawMessage, clientID string) bool {
	var single string
	if json.Unmarshal(raw, &single) == nil {
		return single == clientID
	}
	var list []json.RawMessage
	if json.Unmarshal(raw, &list) != nil || len(list) == 0 || len(list) > 32 {
		return false
	}
	found := false
	for _, item := range list {
		var audience string
		if json.Unmarshal(item, &audience) != nil || audience == "" {
			return false
		}
		if audience == clientID {
			found = true
		}
	}
	return found
}

func nonceMatches(got, expected string) bool {
	if got == "" || expected == "" {
		return false
	}
	gotHash, expectedHash := sha256.Sum256([]byte(got)), sha256.Sum256([]byte(expected))
	return subtle.ConstantTimeCompare(gotHash[:], expectedHash[:]) == 1
}

func numericDate(number json.Number) (int64, bool) {
	if number == "" || strings.ContainsAny(number.String(), ".eE+") {
		return 0, false
	}
	value, err := strconv.ParseInt(number.String(), 10, 64)
	return value, err == nil
}

func telegramID(raw json.RawMessage) (string, bool) {
	var text string
	if len(raw) == 0 {
		return "", false
	}
	if raw[0] == '"' {
		if json.Unmarshal(raw, &text) != nil {
			return "", false
		}
	} else {
		text = string(raw)
	}
	if text == "" || len(text) > 20 {
		return "", false
	}
	for _, ch := range text {
		if ch < '0' || ch > '9' {
			return "", false
		}
	}
	value, err := strconv.ParseUint(text, 10, 64)
	if err != nil || value == 0 {
		return "", false
	}
	return strconv.FormatUint(value, 10), true
}

// decodeUniqueObject decodes one JSON object and rejects duplicate keys and
// trailing data, avoiding parser disagreement for security-sensitive claims.
func decodeUniqueObject(data []byte, destination any) error {
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return ErrInvalidToken
	}
	seen := make(map[string]struct{})
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return ErrInvalidToken
		}
		key, ok := keyToken.(string)
		if !ok {
			return ErrInvalidToken
		}
		if _, exists := seen[key]; exists {
			return ErrInvalidToken
		}
		seen[key] = struct{}{}
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return ErrInvalidToken
		}
	}
	if _, err := decoder.Token(); err != nil {
		return ErrInvalidToken
	}
	if _, err := decoder.Token(); err == nil {
		return ErrInvalidToken
	}
	return json.Unmarshal(data, destination)
}

// rsaKeyFromJWK converts and validates one RSA signing key. Kept separate so
// key-source tests can exercise the same validation rules as remote JWKS data.
func rsaKeyFromJWK(modulus, exponent string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(modulus)
	if err != nil || len(nBytes) == 0 {
		return nil, ErrKeyUnavailable
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(exponent)
	if err != nil || len(eBytes) == 0 || len(eBytes) > 4 || (len(eBytes) > 1 && eBytes[0] == 0) {
		return nil, ErrKeyUnavailable
	}
	e := 0
	for _, b := range eBytes {
		e = e<<8 | int(b)
	}
	key := &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: e}
	if !validPublicKey(key) {
		return nil, ErrKeyUnavailable
	}
	return key, nil
}
