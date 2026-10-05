package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/tenant"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPersistentAuthBrandMembershipAndReplay(t *testing.T) {
	p := testdb.New(t)
	users, err := identity.New(p)
	if err != nil {
		t.Fatal(err)
	}
	engine, _ := mutation.New(p, make([]byte, 32))
	h := New(Dependencies{Brands: tenant.Store{DB: p}, Ready: p.Ping, Identity: users, Mutations: engine, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	call := func(method, path, key, token string, body any) *httptest.ResponseRecorder {
		b, _ := json.Marshal(body)
		r := httptest.NewRequest(method, "http://localhost"+path, bytes.NewReader(b))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Idempotency-Key", key)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	input := map[string]string{"username": "alice_test", "password": "a-test-password-123", "privacy_policy_version": "dev-1", "service_terms_version": "dev-1"}
	r := call("POST", "/api/v1/auth/register", "register-0001", "", input)
	if r.Code != 201 {
		t.Fatal(r.Code, r.Body.String())
	}
	var registered struct {
		Data identity.Authentication `json:"data"`
	}
	json.Unmarshal(r.Body.Bytes(), &registered)
	token := registered.Data.AccessToken
	replay := call("POST", "/api/v1/auth/register", "register-0001", "", input)
	if replay.Code != 201 {
		t.Fatal(replay.Body.String())
	}
	var twice struct {
		Data identity.Authentication `json:"data"`
	}
	json.Unmarshal(replay.Body.Bytes(), &twice)
	if twice.Data.AccessToken != token {
		t.Fatal("replay created another session")
	}
	input["password"] = "changed-password-123"
	if r := call("POST", "/api/v1/auth/register", "register-0001", "", input); r.Code != 409 {
		t.Fatal("no idempotency conflict", r.Code)
	}
	if r := call("GET", "/api/v1/me", "", token, nil); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	if r := call("GET", "/api/v1/b/harbor/me", "", token, nil); r.Code != 401 {
		t.Fatal("cross-brand token accepted", r.Code)
	}
	login := map[string]string{"identifier": "alice_test", "password": "a-test-password-123"}
	if r := call("POST", "/api/v1/b/harbor/auth/login", "login-newbrand-001", "", login); r.Code != 409 {
		t.Fatal("joined without terms", r.Code, r.Body.String())
	}
	login["privacy_policy_version"] = "dev-1"
	login["service_terms_version"] = "dev-1"
	r = call("POST", "/api/v1/b/harbor/auth/login", "login-newbrand-002", "", login)
	if r.Code != 200 {
		t.Fatal(r.Code, r.Body.String())
	}
	var joined struct {
		Data identity.Authentication `json:"data"`
	}
	json.Unmarshal(r.Body.Bytes(), &joined)
	if joined.Data.User.ID != registered.Data.User.ID || joined.Data.Member.ID == registered.Data.Member.ID {
		t.Fatal("global identity/brand snapshot mismatch")
	}
	if r := call("PATCH", "/api/v1/me/profile", "profile-first-001", token, map[string]string{"phone": "+639171234567"}); r.Code != 200 {
		t.Fatal(r.Code, r.Body.String())
	}
	if r := call("PATCH", "/api/v1/me/profile", "profile-again-002", token, map[string]string{"phone": "+639181234567"}); r.Code != 409 {
		t.Fatal("second profile update accepted", r.Code)
	}
	if r := call("POST", "/api/v1/auth/logout", "logout-00001", token, map[string]string{}); r.Code != 200 {
		t.Fatal(r.Code, r.Body.String())
	}
	if r := call("GET", "/api/v1/me", "", token, nil); r.Code != 401 {
		t.Fatal("logged out session live")
	}
	if r := call("GET", "/api/v1/b/harbor/me", "", joined.Data.AccessToken, nil); r.Code != 200 {
		t.Fatal("logout affected other brand")
	}
	var count int
	if err = p.QueryRow(context.Background(), "SELECT count(*) FROM sessions").Scan(&count); err != nil || count != 2 {
		t.Fatal("session count", count, err)
	}
	var plain bool
	p.QueryRow(context.Background(), "SELECT bool_or(response::text LIKE '%a-test-password%' OR response::text LIKE '%access_token%') FROM idempotency_requests").Scan(&plain)
	if plain {
		t.Fatal("sensitive idempotency body not encrypted")
	}
	cookie := registered.Data.AccessToken
	req := httptest.NewRequest(http.MethodPatch, "http://localhost/api/v1/me/profile", bytes.NewBufferString(`{"phone":"+639191234567"}`))
	req.AddCookie(&http.Cookie{Name: userCookieName(registered.Data.Member.BrandID), Value: cookie})
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatal("CSRF accepted", w.Code)
	}
}
