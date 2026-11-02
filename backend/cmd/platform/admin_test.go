package main

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
)

func TestBootstrapSharedLotteryPermissionsAndRepeatedRoleGrants(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	before := os.Args
	t.Cleanup(func() { os.Args = before })
	t.Setenv("BOOTSTRAP_ADMIN_PASSWORD", "bootstrap-pass-2026")
	for _, tc := range []struct {
		name string
		args []string
		want []string
	}{
		{"configured_platform_one", []string{"--super"}, []string{"platform_game.view.platform", "platform_game.write.platform"}},
		{"configured_platform_two", []string{"--super"}, []string{"platform_game.view.platform", "platform_game.write.platform"}},
		{"configured_brand_one", []string{"--brand", "aurora"}, []string{"platform_game.connect.brand", "platform_game.sell.brand", "platform_game.view.brand"}},
		{"configured_brand_two", []string{"--brand", "aurora"}, []string{"platform_game.connect.brand", "platform_game.sell.brand", "platform_game.view.brand"}},
	} {
		os.Args = append([]string{"platform", "create-admin", "--username", tc.name}, tc.args...)
		if err := createAdmin(ctx, db); err != nil {
			t.Fatal(err)
		}
		var got []string
		if err := db.QueryRow(ctx, `SELECT array_agg(permission_key ORDER BY permission_key) FROM role_permissions rp JOIN admin_account_roles ar ON ar.role_id=rp.role_id JOIN admin_accounts a ON a.id=ar.account_id WHERE a.username=$1 AND permission_key LIKE 'platform_game.%'`, tc.name).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestBootstrapPlatformWithdrawalPermissionIsReadOnly(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	before := os.Args
	t.Cleanup(func() { os.Args = before })
	t.Setenv("BOOTSTRAP_ADMIN_PASSWORD", "bootstrap-pass-2026")
	os.Args = []string{"platform", "create-admin", "--username", "platform_withdrawal_reader", "--super"}
	if err := createAdmin(ctx, db); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query(ctx, `SELECT permission_key FROM role_permissions rp JOIN admin_account_roles ar ON ar.role_id=rp.role_id JOIN admin_accounts a ON a.id=ar.account_id WHERE a.username='platform_withdrawal_reader' AND permission_key LIKE 'withdrawal.%' ORDER BY permission_key`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			t.Fatal(err)
		}
		got = append(got, key)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"withdrawal.view.platform"}) {
		t.Fatalf("unexpected withdrawal grants: %v", got)
	}
}

func TestBootstrapPlatformWithdrawalReportPermissionsAreExplicitAndReadOnly(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	before := os.Args
	t.Cleanup(func() { os.Args = before })
	t.Setenv("BOOTSTRAP_ADMIN_PASSWORD", "bootstrap-pass-2026")
	os.Args = []string{"platform", "create-admin", "--username", "platform_report_reader", "--super"}
	if err := createAdmin(ctx, db); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query(ctx, `SELECT permission_key FROM role_permissions rp JOIN admin_account_roles ar ON ar.role_id=rp.role_id JOIN admin_accounts a ON a.id=ar.account_id WHERE a.username='platform_report_reader' AND permission_key LIKE 'report_withdrawal.%' ORDER BY permission_key`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			t.Fatal(err)
		}
		got = append(got, key)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"report_withdrawal.export.platform", "report_withdrawal.view.platform"}) {
		t.Fatalf("unexpected withdrawal report grants: %v", got)
	}
}

func TestBootstrapCommissionPolicyPermissionsAreExplicitAndScoped(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	before := os.Args
	t.Cleanup(func() { os.Args = before })
	t.Setenv("BOOTSTRAP_ADMIN_PASSWORD", "bootstrap-pass-2026")
	for _, tc := range []struct {
		username string
		args     []string
		want     []string
	}{
		{"commission_brand_test", []string{"--brand", "harbor"}, []string{"commission.retry.brand", "commission.run.brand", "commission.view.brand", "commission_adjustment.write.brand", "commission_correction.approve.brand", "commission_correction.continue.brand", "commission_correction.execute_retry.brand", "commission_correction.retry.brand", "commission_correction_policy.write.brand", "commission_payment.approve.brand", "commission_payment.retry.brand", "commission_payment_policy.write.brand", "commission_policy.view.brand", "commission_policy.write.brand"}},
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
	t.Setenv("BOOTSTRAP_ADMIN_PASSWORD", "bootstrap-reward-own-test-2026")
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

func TestBootstrapRewardReportRightsAreExplicitAndScoped(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	before := os.Args
	t.Cleanup(func() { os.Args = before })
	t.Setenv("BOOTSTRAP_ADMIN_PASSWORD", "bootstrap-reward-rep-test-2026")
	for _, tc := range []struct {
		name  string
		args  []string
		scope string
	}{
		{"reward_report_brand_bootstrap", []string{"--brand", "harbor"}, "brand"},
		{"reward_report_platform_bootstrap", []string{"--super"}, "platform"},
	} {
		os.Args = append([]string{"platform", "create-admin", "--username", tc.name}, tc.args...)
		if err := createAdmin(ctx, db); err != nil {
			t.Fatal(err)
		}
		var got []string
		if err := db.QueryRow(ctx, `SELECT coalesce(array_agg(permission_key ORDER BY permission_key),'{}') FROM role_permissions rp JOIN admin_account_roles ar ON ar.role_id=rp.role_id JOIN admin_accounts a ON a.id=ar.account_id WHERE a.username=$1 AND permission_key LIKE 'report_reward.%'`, tc.name).Scan(&got); err != nil {
			t.Fatal(err)
		}
		want := []string{"report_reward.export." + tc.scope, "report_reward.view." + tc.scope}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s reward report grants=%v want=%v", tc.name, got, want)
		}
	}
}

func TestBootstrapReportArchiveRightsAreExplicitAndScoped(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	before := os.Args
	t.Cleanup(func() { os.Args = before })
	t.Setenv("BOOTSTRAP_ADMIN_PASSWORD", "bootstrap-report-arc-test-2026")
	for _, tc := range []struct {
		name string
		args []string
		want []string
	}{
		{"archive_brand_test", []string{"--brand", "harbor"}, []string{"report_archive.create.brand", "report_archive.download.brand", "report_archive.view.brand", "report_archive_policy.write.brand", "report_archive_task.retry.brand"}},
		{"archive_platform_test", []string{"--super"}, []string{"report_archive.download.platform", "report_archive.view.platform"}},
	} {
		os.Args = append([]string{"platform", "create-admin", "--username", tc.name}, tc.args...)
		if err := createAdmin(ctx, db); err != nil {
			t.Fatalf("createAdmin(%s): %v", tc.name, err)
		}
		var got []string
		if err := db.QueryRow(ctx, `SELECT coalesce(array_agg(permission_key ORDER BY permission_key),'{}')
		 FROM role_permissions rp JOIN admin_account_roles ar ON ar.role_id=rp.role_id
		 JOIN admin_accounts a ON a.id=ar.account_id
		 WHERE a.username=$1 AND (permission_key LIKE 'report_archive.%' OR permission_key IN('report_archive_policy.write.brand','report_archive_task.retry.brand'))`, tc.name).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%s report archive grants=%v want=%v", tc.name, got, tc.want)
		}
	}
}

func TestBootstrapRechargeQuotaGrantIsPlatformOnly(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	before := os.Args
	t.Cleanup(func() { os.Args = before })
	t.Setenv("BOOTSTRAP_ADMIN_PASSWORD", "quota-bootstrap-test-2026")
	for _, tc := range []struct {
		name  string
		args  []string
		grant bool
	}{
		{"quota_platform_one", []string{"--super"}, true},
		{"quota_platform_two", []string{"--super"}, true},
		{"quota_brand_one", []string{"--brand", "aurora"}, false},
	} {
		os.Args = append([]string{"platform", "create-admin", "--username", tc.name}, tc.args...)
		if err := createAdmin(ctx, db); err != nil {
			t.Fatal(err)
		}
		var id string
		if err := db.QueryRow(ctx, `SELECT id::text FROM admin_accounts WHERE username=$1`, tc.name).Scan(&id); err != nil {
			t.Fatal(err)
		}
		account, err := (adminsys.Store{DB: db}).Account(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if got := access.Authorize(account, "recharge_quota", "write", access.ScopePlatform, ""); got != tc.grant {
			t.Fatalf("%s quota write=%v want=%v", tc.name, got, tc.grant)
		}
		if tc.grant && !access.Authorize(account, "recharge", "view", access.ScopePlatform, "") {
			t.Fatalf("%s cannot read recharge quota", tc.name)
		}
	}
}

func TestPlatformBootstrapRegistersBrandBasePermissionCatalog(t *testing.T) {
	db := testdb.NewUnseeded(t)
	ctx := context.Background()
	before := os.Args
	t.Cleanup(func() { os.Args = before })
	t.Setenv("BOOTSTRAP_ADMIN_PASSWORD", "brand-catalog-test-2026")
	os.Args = []string{"platform", "create-admin", "--username", "catalog_owner", "--super"}
	if err := createAdmin(ctx, db); err != nil {
		t.Fatal(err)
	}
	for _, key := range append(bootstrapPermissions(false, false), bootstrapPermissions(true, true)...) {
		var exists bool
		if err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM permissions WHERE key=$1)`, key).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Fatalf("platform-only bootstrap omitted brand catalog key %s", key)
		}
	}
	var brandGrants int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM role_permissions rp JOIN roles r ON r.id=rp.role_id WHERE r.brand_id IS NULL AND split_part(rp.permission_key,'.',3)='brand'`).Scan(&brandGrants); err != nil {
		t.Fatal(err)
	}
	if brandGrants != 0 {
		t.Fatalf("platform bootstrap received %d brand grants", brandGrants)
	}
}

// Compare the complete installed catalog with both initialization policies, so
// additions to migrations cannot silently disappear from initial administrators.
func TestBootstrapCatalogAndScopedGrantCoverage(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	before := os.Args
	t.Cleanup(func() { os.Args = before })
	t.Setenv("BOOTSTRAP_ADMIN_PASSWORD", "catalog-coverage-2026")
	for _, tc := range []struct {
		username string
		args     []string
		scope    string
	}{
		{"admin", []string{"--super"}, "platform"},
		{"catalog_brand", []string{"--brand", "aurora"}, "brand"},
	} {
		os.Args = append([]string{"platform", "create-admin", "--username", tc.username}, tc.args...)
		if err := createAdmin(ctx, db); err != nil {
			t.Fatal(err)
		}
		rows, err := db.Query(ctx, `SELECT p.key FROM permissions p WHERE split_part(p.key,'.',3)=$2 AND p.key NOT IN ('brand_domains.view.brand','brand_domains.write.brand') AND NOT EXISTS(SELECT 1 FROM role_permissions rp JOIN admin_account_roles ar ON ar.role_id=rp.role_id JOIN admin_accounts a ON a.id=ar.account_id WHERE a.username=$1 AND rp.permission_key=p.key) ORDER BY p.key`, tc.username, tc.scope)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var key string
			if err := rows.Scan(&key); err != nil {
				t.Fatal(err)
			}
			t.Errorf("%s bootstrap omitted catalog permission %s", tc.scope, key)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
		var misplaced int
		if err := db.QueryRow(ctx, `SELECT count(*) FROM role_permissions rp JOIN admin_account_roles ar ON ar.role_id=rp.role_id JOIN admin_accounts a ON a.id=ar.account_id WHERE a.username=$1 AND split_part(rp.permission_key,'.',3)<>$2`, tc.username, tc.scope).Scan(&misplaced); err != nil {
			t.Fatal(err)
		}
		if misplaced != 0 {
			t.Fatalf("%s bootstrap has %d cross-scope permissions", tc.scope, misplaced)
		}
	}
}

func TestBootstrapFinancialAndComplianceGrants(t *testing.T) {
	brand := make(map[string]bool)
	for _, key := range bootstrapPermissions(false, false) {
		brand[key] = true
	}
	for _, key := range []string{"withdrawal.view.brand", "withdrawal.approve.brand", "withdrawal.reject.brand", "withdrawal.cancel.brand", "withdrawal.mark_paid.brand", "withdrawal.fail.brand", "report_withdrawal.view.brand", "report_withdrawal.export.brand"} {
		if !brand[key] {
			t.Errorf("brand bootstrap missing %s", key)
		}
	}
	platform := make(map[string]bool)
	for _, key := range bootstrapPermissions(true, false) {
		platform[key] = true
	}
	for _, key := range []string{"compliance_policy.write.platform", "compliance_check.run.platform", "recharge_quota.write.platform"} {
		if !platform[key] {
			t.Errorf("platform bootstrap missing %s", key)
		}
	}
	for key := range platform {
		if strings.HasPrefix(key, "withdrawal.") && key != "withdrawal.view.platform" {
			t.Errorf("platform received brand financial operation %s", key)
		}
	}
	if platform["admin.delete.platform"] {
		t.Fatal("ordinary platform initialization granted owner deletion permission")
	}
}
