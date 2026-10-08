package main

import (
	"context"
	"os"
	"reflect"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
)

func TestBootstrapCommissionPolicyPermissionsAreExplicitAndScoped(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	before := os.Args
	t.Cleanup(func() { os.Args = before })
	t.Setenv("BOOTSTRAP_ADMIN_PASSWORD", "bootstrap-owned-test-only-password-2026")
	for _, tc := range []struct {
		username string
		args     []string
		want     []string
	}{
		{"commission_brand_test", []string{"--brand", "harbor"}, []string{"commission.retry.brand", "commission.run.brand", "commission.view.brand", "commission_adjustment.write.brand", "commission_payment.approve.brand", "commission_payment.retry.brand", "commission_payment_policy.write.brand", "commission_policy.view.brand", "commission_policy.write.brand"}},
		{"commission_platform_test", []string{"--super"}, []string{"commission.view.platform", "commission_policy.view.platform"}},
	} {
		os.Args = append([]string{"platform", "create-admin", "--username", tc.username}, tc.args...)
		if err := createAdmin(ctx, db); err != nil {
			t.Fatal(err)
		}
		rows, err := db.Query(ctx, `SELECT permission_key FROM role_permissions rp JOIN admin_account_roles ar ON ar.role_id=rp.role_id JOIN admin_accounts a ON a.id=ar.account_id WHERE a.username=$1 AND permission_key LIKE 'commission%' ORDER BY permission_key`, tc.username)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for rows.Next() {
			var key string
			if err = rows.Scan(&key); err != nil {
				t.Fatal(err)
			}
			got = append(got, key)
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			t.Fatal(err)
		}
		if len(got) != len(tc.want) {
			t.Fatalf("%s grants = %v", tc.username, got)
		}
		for i, key := range got {
			if key != tc.want[i] {
				t.Fatalf("%s grants = %v", tc.username, got)
			}
		}
		var reportGrants []string
		err = db.QueryRow(ctx, `SELECT coalesce(array_agg(permission_key ORDER BY permission_key),'{}') FROM role_permissions rp JOIN admin_account_roles ar ON ar.role_id=rp.role_id JOIN admin_accounts a ON a.id=ar.account_id WHERE a.username=$1 AND permission_key LIKE 'report_commission.%'`, tc.username).Scan(&reportGrants)
		if err != nil {
			t.Fatal(err)
		}
		scope := "brand"
		if tc.username == "commission_platform_test" {
			scope = "platform"
		}
		if len(reportGrants) != 2 || reportGrants[0] != "report_commission.export."+scope || reportGrants[1] != "report_commission.view."+scope {
			t.Fatalf("%s report grants=%v", tc.username, reportGrants)
		}
	}
}

func TestBootstrapRewardPermissionsAreExplicitAndScoped(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	before := os.Args
	t.Cleanup(func() { os.Args = before })
	t.Setenv("BOOTSTRAP_ADMIN_PASSWORD", "bootstrap-reward-owned-test-password-2026")
	for _, tc := range []struct {
		name string
		args []string
		want []string
	}{
		{"reward_brand_bootstrap", []string{"--brand", "harbor"}, []string{"reward.grant.brand", "reward.retry.brand", "reward.revoke.brand", "reward.view.brand"}},
		{"reward_platform_bootstrap", []string{"--super"}, []string{"reward.view.platform"}},
	} {
		os.Args = append([]string{"platform", "create-admin", "--username", tc.name}, tc.args...)
		if err := createAdmin(ctx, db); err != nil {
			t.Fatal(err)
		}
		var got []string
		if err := db.QueryRow(ctx, `SELECT coalesce(array_agg(permission_key ORDER BY permission_key),'{}') FROM role_permissions rp JOIN admin_account_roles ar ON ar.role_id=rp.role_id JOIN admin_accounts a ON a.id=ar.account_id WHERE a.username=$1 AND permission_key LIKE 'reward.%'`, tc.name).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%s rewards=%v want=%v", tc.name, got, tc.want)
		}
	}
}
