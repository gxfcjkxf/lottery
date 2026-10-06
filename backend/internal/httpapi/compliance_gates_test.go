package httpapi

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/compliance"
	"testing"
)

func TestComplianceRegistrationHTTPNoIdentityOnReplayAndScopedEvidence(t *testing.T) {
	f := managedFixture(t)
	ctx := context.Background()
	role := grantCompliance(t, f)
	cfg := compliance.DefaultConfig()
	cfg.IdentityEnabled = true
	mustStatus(t, f.call("PUT", "/api/v1/admin/compliance-policy", "gate-enable-register", f.token, managedBrand, compliance.Input{Version: 1, Config: cfg, Reason: "Require registration adapter"}), 200)
	body := map[string]string{"username": "gated_registration", "password": "registration-test-password-2026", "privacy_policy_version": "dev-1", "service_terms_version": "dev-1"}
	for range 2 {
		r := f.call("POST", "/api/v1/auth/register", "gate-registration-001", "", "", body)
		mustStatus(t, r, 409)
	}
	var identities, evidence int
	var raw string
	if e := f.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM global_users WHERE username='gated_registration'),(SELECT count(*) FROM compliance_gate_rejections WHERE brand_id=$1),(SELECT response::text FROM idempotency_requests WHERE brand_id=$1 AND operation='auth.register' AND key='gate-registration-001')`, managedBrand).Scan(&identities, &evidence, &raw); e != nil || identities != 0 || evidence != 1 {
		t.Fatal(identities, evidence, e)
	}
	read := f.call("GET", "/api/v1/admin/compliance-gates?operation=registration&limit=1", "", f.token, managedBrand, nil)
	mustStatus(t, read, 200)
	var page compliance.GatePage
	managedData(t, read, &page)
	if page.TotalCount != "1" || len(page.Items) != 1 || page.Items[0].ActorType != "anonymous" || page.Items[0].ActorID != nil || page.Items[0].MemberID != nil {
		t.Fatal(page)
	}
	for _, path := range []string{"?operation=withdrawal", "?operation=registration&operation=registration", "?limit=0", "?subject=anything"} {
		mustStatus(t, f.call("GET", "/api/v1/admin/compliance-gates"+path, "", f.token, managedBrand, nil), 400)
	}
	mustStatus(t, f.call("GET", "/api/v1/admin/compliance-gates", "", f.token, "0199a000-0000-7000-8000-000000000002", nil), 403)
	if _, e := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1 AND permission_key='compliance_check.view.brand'`, role); e != nil {
		t.Fatal(e)
	}
	mustStatus(t, f.call("GET", "/api/v1/admin/compliance-gates", "", f.token, managedBrand, nil), 403)
	mustStatus(t, f.call("PUT", "/api/v1/admin/compliance-policy", "gate-disable-register", f.token, managedBrand, compliance.Input{Version: 2, Config: compliance.DefaultConfig(), Reason: "Allow fresh registration intent"}), 200)
	mustStatus(t, f.call("POST", "/api/v1/auth/register", "gate-registration-001", "", "", body), 409)
	mustStatus(t, f.call("POST", "/api/v1/auth/register", "gate-registration-002", "", "", body), 201)
}
