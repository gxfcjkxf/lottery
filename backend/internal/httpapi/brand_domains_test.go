package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/gxfcjkxf/lottery/backend/internal/branddomains"
	"net/http/httptest"
	"testing"
)

const domainPath = "/api/v1/admin/brand-domains"

func domainGrant(t *testing.T, f managementHTTP) {
	t.Helper()
	for _, key := range []string{"brand_domains.view.brand", "brand_domains.write.brand"} {
		if _, e := f.pool.Exec(context.Background(), `INSERT INTO role_permissions(role_id,permission_key)SELECT role_id,$2 FROM admin_account_roles WHERE account_id=$1 ON CONFLICT DO NOTHING`, f.root, key); e != nil {
			t.Fatal(e)
		}
	}
}
func TestDomainHTTPCheckedReplayAndRealAuthority(t *testing.T) {
	f := managedFixture(t)
	mustStatus(t, f.call("GET", domainPath, "", f.token, managedBrand, nil), 403)
	domainGrant(t, f)
	var before branddomains.Record
	managedData(t, f.call("GET", domainPath, "", f.token, managedBrand, nil), &before)
	in := branddomains.Input{Version: before.Version, Domain: "http.example.test", Enabled: true, Primary: true, Reason: "create real authority"}
	first := f.call("POST", domainPath, "domain-create-0001", f.token, managedBrand, in)
	mustStatus(t, first, 201)
	var next branddomains.Record
	managedData(t, first, &next)
	if next.Version != before.Version+1 || next.AuditLogID == "" {
		t.Fatal(next)
	}
	var target string
	for _, d := range next.Domains {
		if d.Domain == in.Domain {
			target = d.ID
			if !d.Enabled || !d.Primary {
				t.Fatal(d)
			}
		}
	}
	if target == "" {
		t.Fatal("binding absent")
	}
	replay := f.call("POST", domainPath, "domain-create-0001", f.token, managedBrand, in)
	mustStatus(t, replay, 201)
	if replay.Body.String() == "" {
		t.Fatal("receipt absent")
	}
	var same branddomains.Record
	managedData(t, replay, &same)
	if same.AuditLogID != next.AuditLogID || same.Version != next.Version {
		t.Fatal(same)
	}
	r := httptest.NewRequest("GET", "http://http.example.test/api/v1/context", nil)
	r.Header.Set("X-Brand-ID", "harbor")
	w := httptest.NewRecorder()
	f.http.ServeHTTP(w, r)
	mustStatus(t, w, 200)
	mustStatus(t, f.call("PATCH", domainPath+"/"+target, "domain-cross-0001", f.token, "0199a000-0000-7000-8000-000000000002", branddomains.Input{Version: next.Version, Reason: "foreign"}), 403)
	off := branddomains.Input{Version: next.Version, Enabled: false, Primary: false, Reason: "disable authority"}
	mustStatus(t, f.call("PATCH", domainPath+"/"+target, "domain-disable-01", f.token, managedBrand, off), 200)
	w = httptest.NewRecorder()
	f.http.ServeHTTP(w, r)
	mustStatus(t, w, 404)
	mustStatus(t, f.call("POST", domainPath, "domain-create-0001", f.token, managedBrand, in), 201)
	_, e := f.pool.Exec(context.Background(), `UPDATE brands SET status='disabled' WHERE id=$1`, managedBrand)
	if e != nil {
		t.Fatal(e)
	}
	// The disabled brand's own public entry is closed before admin dispatch.
	mustStatus(t, f.call("POST", domainPath, "domain-create-0001", f.token, managedBrand, in), 404)
	// A separate configured entry can still enforce the selected brand's
	// read-only state instead of replaying its old success receipt.
	encoded, _ := json.Marshal(in)
	blocked := httptest.NewRequest("POST", "http://harbor.localhost"+domainPath, bytes.NewReader(encoded))
	blocked.Header.Set("Authorization", "Bearer "+f.token)
	blocked.Header.Set("X-Brand-ID", managedBrand)
	blocked.Header.Set("Idempotency-Key", "domain-create-0001")
	blocked.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	f.http.ServeHTTP(w, blocked)
	mustStatus(t, w, 409)
	_, e = f.pool.Exec(context.Background(), `UPDATE brands SET status='active' WHERE id=$1`, managedBrand)
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.pool.Exec(context.Background(), `DELETE FROM role_permissions WHERE role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1)AND permission_key='brand_domains.write.brand'`, f.root)
	if e != nil {
		t.Fatal(e)
	}
	mustStatus(t, f.call("POST", domainPath, "domain-create-0001", f.token, managedBrand, in), 403)
}
func TestDomainHTTPClosedInputAndQuery(t *testing.T) {
	f := managedFixture(t)
	domainGrant(t, f)
	for i, body := range []map[string]any{{"version": 1, "domain": "new.example.test", "enabled": nil, "is_primary": false, "reason": "test"}, {"version": 1, "domain": "https://example.test", "enabled": true, "is_primary": false, "reason": "test"}, {"version": 1, "domain": "new.example.test", "enabled": false, "is_primary": true, "reason": "test"}, {"version": 1, "domain": "new.example.test", "enabled": true, "is_primary": false, "reason": "test", "unknown": 1}} {
		mustStatus(t, f.call("POST", domainPath, "domain-invalid-0"+string(rune('0'+i)), f.token, managedBrand, body), 400)
	}
	for _, query := range []string{"?unknown=1", "?limit=0", "?limit=1&limit=2", "?offset=-1", "?offset=1000001", "?limit=%", "?limit=1;offset=0"} {
		mustStatus(t, f.call("GET", domainPath+"/history"+query, "", f.token, managedBrand, nil), 400)
	}
	mustStatus(t, f.call("GET", domainPath+"?ignored=1", "", f.token, managedBrand, nil), 400)
	mustStatus(t, f.call("GET", domainPath+"/history?limit=20&offset=0", "", f.token, managedBrand, nil), 200)
}
