package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	reportBettingBrand = "report_betting.view.brand"
	reportLedgerBrand  = "report_ledger.view.brand"
	reportBettingPlat  = "report_betting.view.platform"
	reportLedgerPlat   = "report_ledger.view.platform"
)

func grantReportPermission(t *testing.T, f managementHTTP, permission string) {
	t.Helper()
	ctx := context.Background()
	if strings.HasSuffix(permission, ".platform") {
		if _, err := f.pool.Exec(ctx, `INSERT INTO roles(id,brand_id,code,name) VALUES('0199a000-0000-7000-8000-000000000009',NULL,'report_platform_test','Report platform test') ON CONFLICT(id) DO NOTHING`); err != nil {
			t.Fatal(err)
		}
		if _, err := f.pool.Exec(ctx, `INSERT INTO admin_account_roles(account_id,role_id) VALUES($1,'0199a000-0000-7000-8000-000000000009') ON CONFLICT DO NOTHING`, f.root); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO permissions(key) VALUES($1) ON CONFLICT DO NOTHING`, permission); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) SELECT role_id,$2 FROM admin_account_roles WHERE account_id=$1 ON CONFLICT DO NOTHING`, f.root, permission); err != nil {
		t.Fatal(err)
	}
}

func reportURL(path string, values url.Values) string {
	return path + "?" + values.Encode()
}

func reportWindow() (string, string) {
	now := time.Now().UTC()
	return now.Add(-24 * time.Hour).Format(time.RFC3339Nano), now.Add(24 * time.Hour).Format(time.RFC3339Nano)
}

func checkExportBody(t *testing.T, w *httptest.ResponseRecorder, brand, kind string) {
	t.Helper()
	mustStatus(t, w, 200)
	h := w.Header()
	wantVersion := "1"
	if kind == "commission" {
		wantVersion = "2"
	}
	if h.Get("Content-Type") != "text/csv; charset=utf-8" || h.Get("Cache-Control") != "no-store" || h.Get("X-Report-Brand-ID") != brand || h.Get("X-Report-Kind") != kind || h.Get("X-Report-Format-Version") != wantVersion || !uuidPattern.MatchString(h.Get("X-Report-Audit-ID")) {
		t.Fatal("bad export metadata", h)
	}
	if h.Get("Content-Length") != strconv.Itoa(w.Body.Len()) {
		t.Fatal("incorrect length")
	}
	sum := sha256.Sum256(w.Body.Bytes())
	if h.Get("X-Report-SHA256") != hex.EncodeToString(sum[:]) {
		t.Fatal("incorrect content digest")
	}
	if !strings.HasPrefix(w.Body.String(), "\xef\xbb\xbfrecord_type,") {
		t.Fatal("missing UTF8/header")
	}
	rows, e := csv.NewReader(strings.NewReader(strings.TrimPrefix(w.Body.String(), "\xef\xbb\xbf"))).ReadAll()
	if e != nil || len(rows) < 2 || rows[1][0] != "summary" || rows[1][1] != brand {
		t.Fatal("bad CSV", e)
	}
}

func (f managementHTTP) rawCall(method, path, key, token, brand, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://localhost"+path, strings.NewReader(body))
	r.RemoteAddr = "192.0.2.55:12345"
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Idempotency-Key", key)
	r.Header.Set("X-Brand-ID", brand)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.http.ServeHTTP(w, r)
	return w
}
