package httpapi

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/telegramauth"
	"github.com/gxfcjkxf/lottery/backend/internal/tenant"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
)

type localTelegramKeys map[string]*rsa.PublicKey

func (k localTelegramKeys) Keys(context.Context) (map[string]*rsa.PublicKey, error) {
	return k, nil
}

// Uses a test-only key, never contacts Telegram or enables a customer application.
func TestTelegramHTTPDoesNotShareOneAccountQuotaAcrossUsers(t *testing.T) {
	p := testdb.New(t)
	ctx := context.Background()
	brand := "0199a000-0000-7000-8000-000000000001"
	_, err := p.Exec(ctx, `UPDATE brands SET auth_config=auth_config || '{"telegram_enabled":true,"telegram_client_id":"123456"}'::jsonb WHERE id=$1`, brand)
	if err != nil {
		t.Fatal(err)
	}
	users, err := identity.New(p)
	if err != nil {
		t.Fatal(err)
	}
	engine, err := mutation.New(p, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	h := New(Dependencies{
		Brands: tenant.Store{DB: p}, Ready: p.Ping, Identity: users, Mutations: engine,
		Telegram: telegramauth.Verifier{Keys: localTelegramKeys{"local-test": &key.PublicKey}},
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	call := func(method, path, idempotency string, body any) *httptest.ResponseRecorder {
		encoded, _ := json.Marshal(body)
		r := httptest.NewRequest(method, "http://localhost"+path, bytes.NewReader(encoded))
		r.RemoteAddr = "192.0.2.40:12345"
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Idempotency-Key", idempotency)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	for i := 1; i <= 12; i++ {
		response := call("GET", "/api/v1/auth/telegram/challenge", "", nil)
		if response.Code != 200 {
			t.Fatalf("challenge %d: status %d", i, response.Code)
		}
		var challenge struct {
			Data identity.Challenge `json:"data"`
		}
		if err = json.Unmarshal(response.Body.Bytes(), &challenge); err != nil {
			t.Fatal(err)
		}
		header, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": "local-test"})
		claims, _ := json.Marshal(map[string]any{
			"iss": "https://oauth.telegram.org", "aud": "123456",
			"sub": fmt.Sprint(100000 + i), "id": 100000 + i, "name": "Local test member",
			"nonce": challenge.Data.Nonce, "iat": time.Now().Unix(), "exp": time.Now().Add(4 * time.Minute).Unix(),
		})
		unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
		digest := sha256.Sum256([]byte(unsigned))
		signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
		if err != nil {
			t.Fatal(err)
		}
		response = call("POST", "/api/v1/auth/telegram", fmt.Sprintf("local-tg-login-%03d", i), identity.TelegramInput{
			IDToken:     unsigned + "." + base64.RawURLEncoding.EncodeToString(signature),
			ChallengeID: challenge.Data.ID, Nonce: challenge.Data.Nonce, Privacy: "dev-1", Terms: "dev-1",
		})
		if response.Code != 200 {
			t.Fatalf("unrelated Telegram user %d was blocked: status %d", i, response.Code)
		}
	}
	var count int
	if err = p.QueryRow(ctx, `SELECT count(*) FROM global_users WHERE telegram_user_id IS NOT NULL`).Scan(&count); err != nil || count != 12 {
		t.Fatalf("expected 12 independently authenticated identities, got %d: %v", count, err)
	}
}
