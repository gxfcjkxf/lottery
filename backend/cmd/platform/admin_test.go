package main

import (
	"context"
	"os"
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
		{"commission_brand_test", []string{"--brand", "harbor"}, []string{"commission.retry.brand", "commission.run.brand", "commission.view.brand", "commission_policy.view.brand", "commission_policy.write.brand"}},
		{"commission_platform_test", []string{"--super"}, []string{"commission.view.platform", "commission_policy.view.platform"}},
	} {
		os.Args = append([]string{"platform", "create-admin", "--username", tc.username}, tc.args...)
		if err := createAdmin(ctx, db); err != nil {
			t.Fatal(err)
		}
		rows, err := db.Query(ctx, `SELECT permission_key FROM role_permissions rp JOIN admin_account_roles ar ON ar.role_id=rp.role_id JOIN admin_accounts a ON a.id=ar.account_id WHERE a.username=$1 AND (permission_key LIKE 'commission_policy.%' OR permission_key LIKE 'commission.%') ORDER BY permission_key`, tc.username)
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
	}
}
