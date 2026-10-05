package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/tenant"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"
)

type resolver struct{ host, code string }

func (f *resolver) Resolve(_ context.Context, host, code string) (tenant.Brand, error) {
	f.host = host
	f.code = code
	if host == "evil.test" {
		return tenant.Brand{}, tenant.ErrNotFound
	}
	return tenant.Brand{ID: "brand-a", Code: "aurora", Status: "active", Theme: json.RawMessage("{}")}, nil
}
func TestContextDoesNotTrustBrandHeader(t *testing.T) {
	f := &resolver{}
	h := New(Dependencies{Brands: f, Ready: func(context.Context) error { return nil }, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	r := httptest.NewRequest("GET", "http://localhost/api/v1/context", nil)
	r.Header.Set("X-Brand-ID", "other-brand")
	r.Header.Set("X-Forwarded-Host", "evil.test")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 || f.host != "localhost" || f.code != "" {
		t.Fatalf("incorrect brand authority: %d %s", w.Code, f.host)
	}
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Request-ID") == "" {
		t.Fatal("missing cache or trace policy")
	}
}
func TestUnknownHostAndPathBrand(t *testing.T) {
	f := &resolver{}
	h := New(Dependencies{Brands: f, Ready: func(context.Context) error { return nil }})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "http://evil.test/api/v1/context", nil))
	if w.Code != 404 {
		t.Fatal(w.Code)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "http://localhost/api/v1/b/harbor/context", nil))
	if f.code != "harbor" || w.Code != 200 {
		t.Fatal("path brand missing")
	}
}
func TestReadinessFailure(t *testing.T) {
	h := New(Dependencies{Ready: func(context.Context) error { return errors.New("database secret DSN") }})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/health/ready", nil))
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
	var e envelope
	if json.Unmarshal(w.Body.Bytes(), &e) != nil || e.Error.Code != "SERVICE_NOT_READY" {
		t.Fatal(w.Body.String())
	}
}
