package httpapi

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/compliance"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"strings"
	"testing"
)

func grantCompliance(t *testing.T, f managementHTTP) string {
	t.Helper()
	ctx := context.Background()
	role := ids.New()
	if _, e := f.pool.Exec(ctx, `INSERT INTO roles(id,brand_id,code,name)VALUES($1,$2,$3,'Compliance operator')`, role, managedBrand, "compliance_"+role); e != nil {
		t.Fatal(e)
	}
	if _, e := f.pool.Exec(ctx, `INSERT INTO admin_account_roles(account_id,role_id)VALUES($1,$2)`, f.root, role); e != nil {
		t.Fatal(e)
	}
	for _, key := range []string{"compliance_policy.view.brand", "compliance_policy.write.brand", "compliance_check.view.brand", "compliance_check.run.brand"} {
		if _, e := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key)VALUES($1,$2)`, role, key); e != nil {
			t.Fatal(e)
		}
	}
	return role
}
func TestComplianceHTTPCheckedReplayHistoryAndScope(t *testing.T) {
	f := managedFixture(t)
	ctx := context.Background()
	const policy = "/api/v1/admin/compliance-policy"
	const checks = "/api/v1/admin/compliance-checks"
	mustStatus(t, f.call("GET", policy, "", f.token, managedBrand, nil), 403)
	role := grantCompliance(t, f)
	cfg := compliance.DefaultConfig()
	cfg.IdentityEnabled = true
	body := compliance.Input{Version: 1, Config: cfg, Reason: "Configure unintegrated identity check"}
	first := f.call("PUT", policy, "compliance-policy-http-01", f.token, managedBrand, body)
	mustStatus(t, first, 200)
	var receipt compliance.Policy
	managedData(t, first, &receipt)
	if receipt.Version != 2 || receipt.AuditLogID == "" {
		t.Fatal(receipt)
	}
	replay := f.call("PUT", policy, "compliance-policy-http-01", f.token, managedBrand, body)
	mustStatus(t, replay, 200)
	var same compliance.Policy
	managedData(t, replay, &same)
	if same.Version != receipt.Version || same.AuditLogID != receipt.AuditLogID {
		t.Fatal("policy replay changed")
	}
	in := compliance.CheckInput{Version: 2, Operation: "betting", Reason: "Explicit administrative check"}
	created := f.call("POST", checks, "compliance-check-http-01", f.token, managedBrand, in)
	mustStatus(t, created, 201)
	var d compliance.Decision
	managedData(t, created, &d)
	if d.Decision != "review" || d.Checks[2].ReasonCode != "ADAPTER_NOT_CONFIGURED" || d.AuditLogID == "" {
		t.Fatal(d)
	}
	mustStatus(t, f.call("POST", checks, "compliance-check-http-01", f.token, managedBrand, in), 201)
	var n int
	var raw string
	if e := f.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM compliance_decisions WHERE brand_id=$1),(SELECT response::text FROM idempotency_requests WHERE brand_id=$1 AND operation='admin.compliance.check' AND key='compliance-check-http-01')`, managedBrand).Scan(&n, &raw); e != nil || n != 1 || strings.Contains(raw, in.Reason) {
		t.Fatal("bad decision cache", n, e)
	}
	var page compliance.DecisionPage
	response := f.call("GET", checks+"?operation=betting&limit=1&offset=0", "", f.token, managedBrand, nil)
	mustStatus(t, response, 200)
	managedData(t, response, &page)
	if len(page.Items) != 1 || page.TotalCount != "1" || page.Items[0].ID != d.ID {
		t.Fatal(page)
	}
	mustStatus(t, f.call("GET", policy+"/history?limit=1&offset=1", "", f.token, managedBrand, nil), 200)
	for _, path := range []string{policy + "?unused=1", checks + "?operation=unknown", checks + "?operation=betting&operation=betting", checks + "?limit=", policy + "/history?limit=1&limit=1"} {
		mustStatus(t, f.call("GET", path, "", f.token, managedBrand, nil), 400)
	}
	mustStatus(t, f.call("GET", policy, "", f.token, "0199a000-0000-7000-8000-000000000002", nil), 403)
	changed := in
	changed.Operation = "withdrawal"
	mustStatus(t, f.call("POST", checks, "compliance-check-http-01", f.token, managedBrand, changed), 409)
	body.Version = 2
	body.Config = compliance.DefaultConfig()
	body.Reason = "Disable future check config"
	mustStatus(t, f.call("PUT", policy, "compliance-policy-http-02", f.token, managedBrand, body), 200)
	response = f.call("POST", checks, "compliance-check-http-01", f.token, managedBrand, in)
	mustStatus(t, response, 201)
	var historical compliance.Decision
	managedData(t, response, &historical)
	if historical.ID != d.ID || historical.PolicyVersion != 2 || historical.Decision != "review" {
		t.Fatal("old decision lost snapshot")
	}
	mustStatus(t, f.call("POST", checks, "compliance-check-http-stale", f.token, managedBrand, in), 409)
	if _, e := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1 AND permission_key IN('compliance_check.run.brand','compliance_policy.write.brand')`, role); e != nil {
		t.Fatal(e)
	}
	mustStatus(t, f.call("POST", checks, "compliance-check-http-01", f.token, managedBrand, in), 403)
	mustStatus(t, f.call("PUT", policy, "compliance-policy-http-01", f.token, managedBrand, body), 403)
}
func TestComplianceClosedInputAndDisabledReplay(t *testing.T) {
	f := managedFixture(t)
	grantCompliance(t, f)
	const path = "/api/v1/admin/compliance-policy"
	const checks = "/api/v1/admin/compliance-checks"
	for _, body := range []map[string]any{{"version": 1, "config": compliance.DefaultConfig()}, {"version": 1, "config": compliance.DefaultConfig(), "reason": "test", "subject_member_id": ids.New()}} {
		mustStatus(t, f.call("PUT", path, "compliance-invalid-01", f.token, managedBrand, body), 400)
	}
	in := compliance.CheckInput{Version: 1, Operation: "betting", Reason: "Disabled brand replay test"}
	mustStatus(t, f.call("POST", checks, "compliance-disabled-01", f.token, managedBrand, in), 201)
	if _, e := f.pool.Exec(context.Background(), `UPDATE brands SET status='disabled' WHERE id=$1`, managedBrand); e != nil {
		t.Fatal(e)
	}
	mustStatus(t, f.call("POST", checks, "compliance-disabled-01", f.token, managedBrand, in), 409)
	mustStatus(t, f.call("GET", path, "", f.token, managedBrand, nil), 200)
	if _, e := f.pool.Exec(context.Background(), `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); e != nil {
		t.Fatal(e)
	}
	mustStatus(t, f.call("POST", checks, "compliance-disabled-01", f.token, managedBrand, in), 403)
}
