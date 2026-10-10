package main

import (
	"context"
	"os"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
)

func TestDevelopmentSeedCreatesOneAuditedPlatformAdmin(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if err := seedDevelopmentAdmin(ctx, db, "development"); err != nil {
			t.Fatal(err)
		}
	}
	var accounts, roles, audits, scopes int
	var super bool
	var hash string
	if err := db.QueryRow(ctx, `SELECT password_hash,is_super_admin FROM admin_accounts WHERE username='admin'`).Scan(&hash, &super); err != nil {
		t.Fatal(err)
	}
	if !super || hash == "admin123" {
		t.Fatal("default admin must be platform-only and hashed")
	}
	for query, dest := range map[string]*int{
		`SELECT count(*) FROM admin_accounts WHERE username='admin'`:                                                       &accounts,
		`SELECT count(*) FROM admin_account_roles ar JOIN admin_accounts a ON a.id=ar.account_id WHERE a.username='admin'`: &roles,
		`SELECT count(*) FROM audit_logs WHERE action='admin.bootstrap'`:                                                   &audits,
		`SELECT count(*) FROM admin_brand_scopes`:                                                                          &scopes,
	} {
		if err := db.QueryRow(ctx, query).Scan(dest); err != nil {
			t.Fatal(err)
		}
	}
	if accounts != 1 || roles != 1 || audits != 1 || scopes != 0 {
		t.Fatal(accounts, roles, audits, scopes)
	}
	for _, key := range []string{"brand.create.platform", "brand_operation.write.platform", "admin.write.platform", "role.write.platform"} {
		var found bool
		if err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM role_permissions rp JOIN admin_account_roles ar ON ar.role_id=rp.role_id JOIN admin_accounts a ON a.id=ar.account_id WHERE a.username='admin' AND rp.permission_key=$1)`, key).Scan(&found); err != nil || !found {
			t.Fatal(key, found, err)
		}
	}
	if _, err := db.Exec(ctx, `UPDATE admin_accounts SET password_hash='preserved-hash', status='disabled' WHERE username='admin'`); err != nil {
		t.Fatal(err)
	}
	if err := seedDevelopmentAdmin(ctx, db, "development"); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := db.QueryRow(ctx, `SELECT password_hash,status FROM admin_accounts WHERE username='admin'`).Scan(&hash, &status); err != nil {
		t.Fatal(err)
	}
	if hash != "preserved-hash" || status != "disabled" {
		t.Fatal("seed overwrote an existing account")
	}
}

func TestDevelopmentSeedDoesNotCreateAdminOutsideDevelopment(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	for _, environment := range []string{"test", "production"} {
		if err := seedDevelopmentAdmin(ctx, db, environment); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM admin_accounts`).Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
}

func TestDevelopmentSeedAuditFailureRollsBackAccountAndGrants(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	if _, err := db.Exec(ctx, `CREATE FUNCTION reject_development_bootstrap() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected bootstrap audit failure'; END $$;
		CREATE TRIGGER reject_development_bootstrap BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_development_bootstrap()`); err != nil {
		t.Fatal(err)
	}
	if err := seedDevelopmentAdmin(ctx, db, "development"); err == nil {
		t.Fatal("audit failure must reject the bootstrap")
	}
	var accounts, roles int
	if err := db.QueryRow(ctx, `SELECT (SELECT count(*) FROM admin_accounts), (SELECT count(*) FROM roles)`).Scan(&accounts, &roles); err != nil || accounts != 0 || roles != 0 {
		t.Fatal("failed bootstrap left account or grants", accounts, roles, err)
	}
}

func TestExplicitBootstrapStillRejectsTheDevelopmentShortPassword(t *testing.T) {
	db := testdb.New(t)
	args := os.Args
	t.Cleanup(func() { os.Args = args })
	os.Args = []string{"platform", "create-admin", "--username", "admin", "--super"}
	t.Setenv("BOOTSTRAP_ADMIN_PASSWORD", "admin123")
	if err := createAdmin(context.Background(), db); err == nil {
		t.Fatal("explicit account creation accepted the short development password")
	}
}
