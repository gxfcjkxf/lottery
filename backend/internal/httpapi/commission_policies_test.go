package httpapi

import (
	"context"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/commission"
)

const commissionPolicyPath = "/api/v1/admin/commission-policy"

func grantCommissionPolicy(t *testing.T, f managementHTTP) {
	t.Helper()
	ctx := context.Background()
	for _, key := range []string{"commission_policy.view.brand", "commission_policy.write.brand"} {
		if _, err := f.pool.Exec(ctx, `INSERT INTO permissions(key) VALUES($1) ON CONFLICT DO NOTHING`, key); err != nil {
			t.Fatal(err)
		}
		if _, err := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) SELECT role_id,$2 FROM admin_account_roles WHERE account_id=$1 ON CONFLICT DO NOTHING`, f.root, key); err != nil {
			t.Fatal(err)
		}
	}
}

func enabledCommissionPolicyConfig() commission.PolicyConfig {
	weekday := 1
	return commission.PolicyConfig{
		Enabled: true,
		Calendar: &commission.Calendar{
			Timezone: "UTC", Cycle: "weekly", BoundaryTime: "00:00:00", Weekday: &weekday,
		},
		PayoutMode: commission.PayoutManual,
	}
}

func TestBrandCommissionPolicyIsReadOnly(t *testing.T) {
	f := managedFixture(t)
	grantCommissionPolicy(t, f)
	mustStatus(t, f.call("GET", commissionPolicyPath, "", f.token, managedBrand, nil), 200)
	mustStatus(t, f.call("PUT", commissionPolicyPath, "denied", f.token, managedBrand, commission.PolicyInput{Version: 1, Config: enabledCommissionPolicyConfig()}), 403)
}
