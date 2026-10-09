package httpapi

import (
	"context"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/commission"
)

const commissionDiscoveryHTTPPath = "/api/v1/admin/commission-discovery"

func TestCommissionDiscoveryHTTPReadAuthorizationPaginationAndAudit(t *testing.T) {
	f := managedFixture(t)
	ctx := context.Background()
	if got := f.call("GET", commissionDiscoveryHTTPPath, "", "", managedBrand, nil); got.Code != 401 {
		t.Fatalf("missing session status=%d body=%s", got.Code, got.Body.String())
	}
	mustStatus(t, f.call("GET", commissionDiscoveryHTTPPath, "", f.token, managedBrand, nil), 403)
	var denied int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='access.denied' AND after_json->>'attempted_brand'=$1 AND after_json->>'permission'='commission.view'`, managedBrand).Scan(&denied); err != nil || denied == 0 {
		t.Fatalf("permission denial was not audited: count=%d err=%v", denied, err)
	}
	grantCommissionCycle(t, f, "view")
	for _, query := range []string{"?limit=0", "?limit=101", "?offset=1000001", "?limit=20&limit=30", "?other=1", "?offset="} {
		mustStatus(t, f.call("GET", commissionDiscoveryHTTPPath+query, "", f.token, managedBrand, nil), 400)
	}
	response := f.call("GET", commissionDiscoveryHTTPPath, "", f.token, managedBrand, nil)
	mustStatus(t, response, 200)
	var page commission.DiscoveryPage
	managedData(t, response, &page)
	if page.BrandID != managedBrand || page.Limit != 20 || page.Offset != 0 || page.TotalCount != "0" || len(page.Items) != 0 || page.Items == nil {
		t.Fatalf("expected a real empty discovery page, got %+v", page)
	}
	var reads int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE brand_id=$1 AND action='commission.discovery.list' AND resource_type='query'`, managedBrand).Scan(&reads); err != nil || reads != 1 {
		t.Fatalf("successful discovery read audit count=%d err=%v", reads, err)
	}
}

func TestCommissionDiscoveryHTTPRetryGuardsAndAuthorizationAudit(t *testing.T) {
	f := managedFixture(t)
	id := "0199a000-0000-7000-8000-000000000099"
	body := `{"version":1,"reason":"retry failed discovery"}`
	if got := f.rawCall("POST", commissionDiscoveryHTTPPath+"/"+id+"/retry", "discovery-auth-guard-01", "", managedBrand, body); got.Code != 401 {
		t.Fatalf("missing session status=%d body=%s", got.Code, got.Body.String())
	}
	for _, path := range []string{
		commissionDiscoveryHTTPPath + "/NOT-A-UUID/retry",
		commissionDiscoveryHTTPPath + "/" + id + "/retry?limit=20",
	} {
		mustStatus(t, f.rawCall("POST", path, "discovery-guard-invalid-01", f.token, managedBrand, body), 400)
	}
	mustStatus(t, f.rawCall("POST", commissionDiscoveryHTTPPath+"/"+id+"/retry", "discovery-guard-invalid-02", f.token, managedBrand, `{"version":1,"reason":"ok","unexpected":true}`), 400)

	grantCommissionCycle(t, f, "view")
	response := f.rawCall("POST", commissionDiscoveryHTTPPath+"/"+id+"/retry", "discovery-retry-denied-01", f.token, managedBrand, body)
	mustStatus(t, response, 403)
	var denials int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_logs WHERE action='access.denied' AND after_json->>'attempted_brand'=$1 AND after_json->>'permission'='commission.discovery.retry'`, managedBrand).Scan(&denials); err != nil || denials != 1 {
		t.Fatalf("retry permission denial audit count=%d err=%v", denials, err)
	}
	grantCommissionCycle(t, f, "retry")
	if _, err := f.pool.Exec(context.Background(), `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	response = f.rawCall("POST", commissionDiscoveryHTTPPath+"/"+id+"/retry", "discovery-superadmin-denied-01", f.token, managedBrand, body)
	mustStatus(t, response, 403)
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_logs WHERE action='access.denied' AND after_json->>'attempted_brand'=$1 AND after_json->>'permission'='commission.discovery.retry'`, managedBrand).Scan(&denials); err != nil || denials != 1 {
		t.Fatalf("entry denial changed the original retry permission audit count=%d err=%v", denials, err)
	}
	var entryDenials int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_logs WHERE actor_id=$1 AND action='access.denied' AND after_json->>'attempted_brand'='' AND after_json->>'permission'='admin.entry'`, f.root).Scan(&entryDenials); err != nil || entryDenials != 1 {
		t.Fatalf("super-admin brand-entry denial audit count=%d err=%v", entryDenials, err)
	}
}
