package telegramauth

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	telegramJWKSURL = "https://oauth.telegram.org/.well-known/jwks.json"
	maxJWKSSize     = 1 << 20
	keyCacheTTL     = 5 * time.Minute
	refreshInterval = time.Second
)

// RemoteKeys retrieves and caches Telegram's RS256 signing keys. The public
// constructor always uses Telegram's fixed HTTPS JWKS endpoint.
type RemoteKeys struct {
	client   *http.Client
	endpoint string

	mu             sync.Mutex
	keys           map[string]*rsa.PublicKey
	expiresAt      time.Time
	lastRefresh    time.Time
	refreshAttempt bool
}

// NewRemoteKeys creates a key source using Telegram's fixed JWKS endpoint and
// an HTTP client with a five-second request timeout.
func NewRemoteKeys() *RemoteKeys {
	client := &http.Client{
		Timeout:       5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return newRemoteKeys(client, telegramJWKSURL)
}

// newRemoteKeys is the test seam for an HTTP test server. Production callers
// must use NewRemoteKeys; endpoint is never taken from token or request data.
func newRemoteKeys(client *http.Client, endpoint string) *RemoteKeys {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	return &RemoteKeys{client: client, endpoint: endpoint}
}

// Keys returns the cached keys, fetching them when the cache is empty or stale.
func (r *RemoteKeys) Keys(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.keys) > 0 && time.Now().Before(r.expiresAt) {
		return cloneKeys(r.keys), nil
	}
	if err := r.fetchLocked(ctx); err != nil {
		return nil, ErrKeyUnavailable
	}
	return cloneKeys(r.keys), nil
}

// Refresh forces a bounded refresh after an otherwise-valid cache misses a
// token's key ID. Repeated/concurrent misses are throttled to one fetch/second.
func (r *RemoteKeys) Refresh(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.refreshAttempt && time.Since(r.lastRefresh) < refreshInterval {
		return nil
	}
	r.refreshAttempt = true
	r.lastRefresh = time.Now()
	return r.fetchLocked(ctx)
}

func (r *RemoteKeys) fetchLocked(ctx context.Context) error {
	if r.endpoint == "" || r.client == nil {
		return ErrKeyUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.endpoint, nil)
	if err != nil {
		return ErrKeyUnavailable
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return ErrKeyUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ErrKeyUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxJWKSSize+1))
	if err != nil || len(body) > maxJWKSSize {
		return ErrKeyUnavailable
	}
	keys, err := parseJWKS(body)
	if err != nil {
		return ErrKeyUnavailable
	}
	r.keys = keys
	r.expiresAt = time.Now().Add(keyCacheTTL)
	return nil
}

func cloneKeys(source map[string]*rsa.PublicKey) map[string]*rsa.PublicKey {
	copy := make(map[string]*rsa.PublicKey, len(source))
	for kid, key := range source {
		copy[kid] = key
	}
	return copy
}

type jwksDocument struct {
	Keys []jwk `json:"keys"`
}

type jwk struct {
	KeyType  string `json:"kty"`
	KeyID    string `json:"kid"`
	Use      string `json:"use"`
	Alg      string `json:"alg"`
	Modulus  string `json:"n"`
	Exponent string `json:"e"`
}

func parseJWKS(body []byte) (map[string]*rsa.PublicKey, error) {
	var document jwksDocument
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	if err := decoder.Decode(&document); err != nil || len(document.Keys) == 0 || len(document.Keys) > 100 {
		return nil, ErrKeyUnavailable
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, ErrKeyUnavailable
	}
	keys := make(map[string]*rsa.PublicKey)
	for _, item := range document.Keys {
		if item.KeyType != "RSA" || (item.Use != "" && item.Use != "sig") || (item.Alg != "" && item.Alg != "RS256") {
			continue
		}
		if item.KeyID == "" || len(item.KeyID) > 256 {
			return nil, ErrKeyUnavailable
		}
		if _, duplicate := keys[item.KeyID]; duplicate {
			return nil, ErrKeyUnavailable
		}
		key, err := rsaKeyFromJWK(item.Modulus, item.Exponent)
		if err != nil {
			return nil, ErrKeyUnavailable
		}
		keys[item.KeyID] = key
	}
	if len(keys) == 0 {
		return nil, ErrKeyUnavailable
	}
	return keys, nil
}
