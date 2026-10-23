package httpapi

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/compliance"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"testing"
)

func TestPlatformOwnsAuthenticationAndRiskSettings(t *testing.T) {
	f := managedFixture(t)
	cfg := compliance.DefaultConfig()
	cfg.IdentityEnabled = true
	body := compliance.Input{Version: 1, Config: cfg}
	for _, key := range []string{"compliance_policy.view.brand", "compliance_policy.write.brand"} {
		if _, err := f.pool.Exec(context.Background(), `INSERT INTO role_permissions(role_id,permission_key) SELECT role_id,$2 FROM admin_account_roles WHERE account_id=$1 ON CONFLICT DO NOTHING`, f.root, key); err != nil {
			t.Fatal(err)
		}
	}
	mustStatus(t, f.call("GET", "/api/v1/admin/auth-settings", "", f.token, managedBrand, nil), 403)
	mustStatus(t, f.call("PATCH", "/api/v1/admin/auth-settings", "brand-auth-denied", f.token, managedBrand, identity.AuthSettingsInput{Version: 1}), 403)
	mustStatus(t, f.call("PUT", "/api/v1/admin/compliance-policy", "brand-risk-denied", f.token, managedBrand, body), 403)
	mustStatus(t, f.call("PUT", "/api/v1/platform/compliance-policy", "staff-platform-denied", f.token, managedBrand, body), 403)
	op := securityOperator(t, f)
	mustStatus(t, op.call("PUT", "/api/v1/platform/compliance-policy", "platform-no-grant", op.token, managedBrand, body), 403)
	role := grantPlatformPermission(t, op, "compliance_policy.view.platform", "compliance_policy.write.platform", "auth_config.view.platform", "auth_config.write.platform")
	mustStatus(t, op.call("PUT", "/api/v1/platform/compliance-policy", "platform-policy-save", op.token, managedBrand, body), 200)
	var auth identity.AuthSettingsRecord
	r := op.call("GET", "/api/v1/platform/auth-settings", "", op.token, managedBrand, nil)
	mustStatus(t, r, 200)
	managedData(t, r, &auth)
	in := identity.AuthSettingsInput{Version: auth.Version, CaptchaEnabled: true}
	first := op.call("PATCH", "/api/v1/platform/auth-settings", "platform-auth-save", op.token, managedBrand, in)
	mustStatus(t, first, 200)
	replay := op.call("PATCH", "/api/v1/platform/auth-settings", "platform-auth-save", op.token, managedBrand, in)
	mustStatus(t, replay, 200)
	var a, b identity.AuthSettingsRecord
	managedData(t, first, &a)
	managedData(t, replay, &b)
	if a.Version != b.Version || !a.CaptchaEnabled {
		t.Fatal(a, b)
	}
	if _, err := f.pool.Exec(context.Background(), `DELETE FROM role_permissions WHERE role_id=$1 AND permission_key='auth_config.write.platform'`, role); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, op.call("PATCH", "/api/v1/platform/auth-settings", "platform-auth-save", op.token, managedBrand, in), 403)
	other := op.call("GET", "/api/v1/platform/compliance-policy", "", op.token, "0199a000-0000-7000-8000-000000000002", nil)
	mustStatus(t, other, 200)
	var p compliance.Policy
	managedData(t, other, &p)
	if p.Config.IdentityEnabled {
		t.Fatal("another brand changed")
	}
}
