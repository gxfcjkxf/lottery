package httpapi

import (
	"context"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/commission"
)

const commissionCyclesPath = "/api/v1/admin/commission-cycles"

func grantCommissionCycle(t *testing.T, f managementHTTP, actions ...string) {
	t.Helper()
	ctx := context.Background()
	for _, action := range actions {
		key := "commission." + action + ".brand"
		if _, err := f.pool.Exec(ctx, `INSERT INTO permissions(key) VALUES($1) ON CONFLICT DO NOTHING`, key); err != nil {
			t.Fatal(err)
		}
		if _, err := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) SELECT role_id,$2 FROM admin_account_roles WHERE account_id=$1 ON CONFLICT DO NOTHING`, f.root, key); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCommissionCyclesHTTPReadAuthorizationPaginationAndRevocation(t *testing.T) {
	f := managedFixture(t)
	ctx := context.Background()
	path := commissionCyclesPath
	if got := f.call("GET", path, "", "", managedBrand, nil); got.Code != 401 {
		t.Fatalf("missing session status=%d body=%s", got.Code, got.Body.String())
	}
	mustStatus(t, f.call("GET", path, "", f.token, managedBrand, nil), 403)
	grantCommissionCycle(t, f, "view")
	for _, query := range []string{"?limit=0", "?limit=101", "?offset=1000001", "?limit=20&limit=30", "?other=1", "?offset="} {
		mustStatus(t, f.call("GET", path+query, "", f.token, managedBrand, nil), 400)
	}
	pageResponse := f.call("GET", path, "", f.token, managedBrand, nil)
	mustStatus(t, pageResponse, 200)
	var page commission.CycleListPage
	managedData(t, pageResponse, &page)
	if page.BrandID != managedBrand || page.Limit != 20 || page.Offset != 0 || page.TotalCount != "0" || len(page.Items) != 0 || page.Items == nil {
		t.Fatalf("expected a real empty cycle page, got %+v", page)
	}
	var cycles int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM commission_cycles WHERE brand_id=$1`, managedBrand).Scan(&cycles); err != nil || cycles != 0 {
		t.Fatalf("empty read created a cycle: count=%d err=%v", cycles, err)
	}
	if _, err := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id IN (SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key='commission.view.brand'`, f.root); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("GET", path, "", f.token, managedBrand, nil), 403)
	grantPlatformPermission(t, f, "commission.view.platform")
	// Platform view grants visibility, but never substitutes for a brand-scoped run.
	mustStatus(t, f.call("GET", path, "", f.token, managedBrand, nil), 403)
	mustStatus(t, f.call("POST", path, "cycle-platform-run-denied-01", f.token, managedBrand, map[string]string{
		"anchor_order_id": "0199a000-0000-7000-8000-000000000099", "reason": "platform scope cannot run",
	}), 403)
	if _, err := f.pool.Exec(ctx, `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	platformToken := platformAdminToken(t, f)
	f.token = platformToken
	mustStatus(t, f.call("GET", "/api/v1/platform/commission-cycles", "", platformToken, managedBrand, nil), 200)
	mustStatus(t, f.call("POST", "/api/v1/platform/commission-cycles", "cycle-platform-entry-run-denied-01", platformToken, managedBrand, map[string]string{
		"anchor_order_id": "0199a000-0000-7000-8000-000000000099", "reason": "platform entry has no run route",
	}), 404)
}

func TestCommissionCyclesHTTPDetailAndCreateNotFoundAndClosedBodies(t *testing.T) {
	f := managedFixture(t)
	grantCommissionCycle(t, f, "view", "run", "retry")
	missing := "0199a000-0000-7000-8000-000000000099"
	mustStatus(t, f.call("GET", commissionCyclesPath+"/"+missing, "", f.token, managedBrand, nil), 404)
	mustStatus(t, f.call("GET", commissionCyclesPath+"/"+missing+"?limit=20", "", f.token, managedBrand, nil), 400)
	for _, body := range []string{
		`{"anchor_order_id":"` + missing + `","reason":"valid reason","extra":true}`,
		`{"anchor_order_id":"` + missing + `","anchor_order_id":"` + missing + `","reason":"valid reason"}`,
		`{"reason":""}`,
	} {
		mustStatus(t, f.rawCall("POST", commissionCyclesPath, "cycle-invalid-body-001", f.token, managedBrand, body), 400)
	}
	mustStatus(t, f.call("POST", commissionCyclesPath, "cycle-nonexistent-anchor-01", f.token, managedBrand, map[string]string{
		"anchor_order_id": missing, "reason": "nonexistent real anchor",
	}), 404)
	mustStatus(t, f.call("POST", commissionCyclesPath+"?limit=20", "cycle-query-rejected-01", f.token, managedBrand, map[string]string{
		"anchor_order_id": missing, "reason": "query is forbidden",
	}), 400)
}

func TestCommissionCycleCandidatesHTTPAuthorization(t *testing.T) {
	f := managedFixture(t)
	path := commissionCyclesPath + "/candidates?limit=100&offset=0"
	mustStatus(t, f.call("GET", path, "", "", managedBrand, nil), 401)
	mustStatus(t, f.call("GET", path, "", f.token, managedBrand, nil), 403)
	grantCommissionCycle(t, f, "view")
	response := f.call("GET", path, "", f.token, managedBrand, nil)
	mustStatus(t, response, 200)
	var page commission.CycleCandidatePage
	managedData(t, response, &page)
	if page.BrandID != managedBrand || page.Items == nil || len(page.Items) != 0 || page.Limit != 100 {
		t.Fatalf("unexpected page %+v", page)
	}
	mustStatus(t, f.call("GET", path, "", f.token, pointsBrandB, nil), 403)
	mustStatus(t, f.call("GET", commissionCyclesPath+"/candidates?other=1", "", f.token, managedBrand, nil), 400)
}
