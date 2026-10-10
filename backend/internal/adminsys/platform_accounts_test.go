package adminsys

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
)

type platformAccountsFixture struct {
	store Store
	actor access.Account
	role  string
}

func newPlatformAccountsFixture(t *testing.T) platformAccountsFixture {
	t.Helper()
	p := testdb.New(t)
	ctx := context.Background()
	actorID, roleID := ids.New(), ids.New()
	grants := []string{"admin.view.platform", "admin.write.platform", "role.view.platform", "secret.view.platform"}
	for _, key := range grants {
		if _, err := p.Exec(ctx, `INSERT INTO permissions(key) VALUES($1) ON CONFLICT DO NOTHING`, key); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := p.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash,is_super_admin) VALUES($1,$2,'test-opaque',true)`, actorID, platformTestName("actor")); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `INSERT INTO roles(id,code,name) VALUES($1,$2,'Platform account operator')`, roleID, platformTestName("operator_role")); err != nil {
		t.Fatal(err)
	}
	role := access.Role{}
	for _, key := range grants {
		if _, err := p.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,$2)`, roleID, key); err != nil {
			t.Fatal(err)
		}
		resource, action, scope, ok := parsePermission(key)
		if !ok {
			t.Fatalf("invalid test permission %q", key)
		}
		role.Permissions = append(role.Permissions, access.Permission{Resource: resource, Action: action, Scope: scope})
	}
	if _, err := p.Exec(ctx, `INSERT INTO admin_account_roles(account_id,role_id) VALUES($1,$2)`, actorID, roleID); err != nil {
		t.Fatal(err)
	}
	return platformAccountsFixture{store: Store{DB: p}, role: roleID, actor: access.Account{ID: actorID, Type: access.AccountAdmin, SuperAdmin: true, Roles: []access.Role{role}}}
}

func platformTestName(prefix string) string {
	id := strings.ReplaceAll(ids.New(), "-", "")
	return fmt.Sprintf("%s_%s", prefix, id[len(id)-8:])
}

func addPlatformRole(t *testing.T, f platformAccountsFixture, permissions ...string) (string, string) {
	return addScopedTestRole(t, f, "", permissions...)
}

func addScopedTestRole(t *testing.T, f platformAccountsFixture, brand string, permissions ...string) (string, string) {
	t.Helper()
	ctx := context.Background()
	id, code := ids.New(), platformTestName("platform_role")
	if _, err := f.store.DB.Exec(ctx, `INSERT INTO roles(id,brand_id,code,name) VALUES($1,NULLIF($2,'')::uuid,$3,'Test platform role')`, id, brand, code); err != nil {
		t.Fatal(err)
	}
	for _, key := range permissions {
		if _, err := f.store.DB.Exec(ctx, `INSERT INTO permissions(key) VALUES($1) ON CONFLICT DO NOTHING`, key); err != nil {
			t.Fatal(err)
		}
		if _, err := f.store.DB.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,$2)`, id, key); err != nil {
			t.Fatal(err)
		}
	}
	return id, code
}

func TestPlatformAccountsAuthorizedCRUDResetAndAuditRedaction(t *testing.T) {
	f := newPlatformAccountsFixture(t)
	ctx := context.Background()
	meta := identity.Metadata{RequestID: "platform-accounts-test"}
	roleID, roleCode := addPlatformRole(t, f, "admin.view.platform", "role.view.platform")
	brandRoleID, _ := addScopedTestRole(t, f, managementBrandA, "user.view.brand")

	roles, err := f.store.ListPlatformRoles(ctx, f.actor, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	var foundRole bool
	for _, role := range roles {
		if role.ID == roleID {
			foundRole = role.BrandID == "" && len(role.Permissions) == 2 && role.Permissions[0] == "admin.view.platform"
		}
		if role.ID == brandRoleID {
			t.Fatal("brand role appeared in platform role list")
		}
	}
	if !foundRole {
		t.Fatalf("platform role missing or malformed: %+v", roles)
	}

	admins, err := f.store.ListPlatformAdmins(ctx, f.actor, 100, 0)
	if err != nil || len(admins) != 1 || !admins[0].SuperAdmin || len(admins[0].BrandIDs) != 0 {
		t.Fatalf("platform admin list = %+v, err=%v", admins, err)
	}

	tx := beginManagementTx(t, f.store.DB)
	created, err := f.store.CreatePlatformAdmin(ctx, tx, f.actor, AdminInput{Username: platformTestName("platform_target"), RoleIDs: []string{roleID}, Status: "active", Reason: "create platform account"}, "initial-secret-hash", meta)
	if err != nil {
		t.Fatal(err)
	}
	commitManagement(t, tx)
	if !created.SuperAdmin || created.Version != 1 || len(created.BrandIDs) != 0 || len(created.RoleIDs) != 1 || created.RoleIDs[0] != roleID || created.RoleCodes[0] != roleCode {
		t.Fatalf("unexpected created platform admin record: %+v", created)
	}
	var scopes int
	if err := f.store.DB.QueryRow(ctx, `SELECT count(*) FROM admin_brand_scopes WHERE account_id=$1`, created.ID).Scan(&scopes); err != nil || scopes != 0 {
		t.Fatalf("created platform admin scopes=%d err=%v", scopes, err)
	}

	sessionID := ids.New()
	if _, err := f.store.DB.Exec(ctx, `INSERT INTO sessions(id,token_hash,admin_id,expires_at) VALUES($1,$2,$3,now()+interval '1 day')`, sessionID, ids.New(), created.ID); err != nil {
		t.Fatal(err)
	}
	tx = beginManagementTx(t, f.store.DB)
	updated, err := f.store.UpdatePlatformAdmin(ctx, tx, f.actor, created.ID, AdminInput{Version: created.Version, RoleIDs: []string{roleID}, Status: "disabled", Reason: "disable platform account"}, meta)
	if err != nil {
		t.Fatal(err)
	}
	commitManagement(t, tx)
	if updated.Status != "disabled" || updated.Version != 2 || len(updated.RoleIDs) != 1 || updated.RoleCodes[0] != roleCode {
		t.Fatalf("unexpected updated platform admin record: %+v", updated)
	}
	var revoked bool
	if err := f.store.DB.QueryRow(ctx, `SELECT revoked_at IS NOT NULL FROM sessions WHERE id=$1`, sessionID).Scan(&revoked); err != nil || !revoked {
		t.Fatalf("update did not revoke target session: revoked=%v err=%v", revoked, err)
	}

	staleTx := beginManagementTx(t, f.store.DB)
	if _, err := f.store.UpdatePlatformAdmin(ctx, staleTx, f.actor, created.ID, AdminInput{Version: 1, RoleIDs: []string{roleID}, Status: "active", Reason: "stale update"}, meta); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale update error=%v, want conflict", err)
	}
	_ = staleTx.Rollback(ctx)

	resetSession := ids.New()
	if _, err := f.store.DB.Exec(ctx, `INSERT INTO sessions(id,token_hash,admin_id,expires_at) VALUES($1,$2,$3,now()+interval '1 day')`, resetSession, ids.New(), created.ID); err != nil {
		t.Fatal(err)
	}
	tx = beginManagementTx(t, f.store.DB)
	resetAuditID, err := f.store.ResetPlatformAdminPassword(ctx, tx, f.actor, created.ID, updated.Version, "replacement-secret-hash", "credential reset", meta)
	if err != nil {
		t.Fatal(err)
	}
	commitManagement(t, tx)
	if err := f.store.DB.QueryRow(ctx, `SELECT revoked_at IS NOT NULL FROM sessions WHERE id=$1`, resetSession).Scan(&revoked); err != nil || !revoked {
		t.Fatalf("password reset did not revoke target session: revoked=%v err=%v", revoked, err)
	}
	var auditJSON string
	if err := f.store.DB.QueryRow(ctx, `SELECT before_json::text||after_json::text FROM audit_logs WHERE id=$1`, resetAuditID).Scan(&auditJSON); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(auditJSON, "replacement-secret-hash") || strings.Contains(auditJSON, "initial-secret-hash") || !strings.Contains(auditJSON, `"password": "redacted"`) {
		t.Fatalf("password reset audit is not redacted: %s", auditJSON)
	}
	for _, auditID := range []string{created.AuditLogID, updated.AuditLogID, resetAuditID} {
		if err := f.store.DB.QueryRow(ctx, `SELECT before_json::text||COALESCE(after_json::text,'') FROM audit_logs WHERE id=$1`, auditID).Scan(&auditJSON); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(auditJSON, "initial-secret-hash") || strings.Contains(auditJSON, "replacement-secret-hash") {
			t.Fatalf("account audit leaked a password hash: %s", auditJSON)
		}
	}
}

func TestPlatformAccountsRejectUnauthorizedActorsAndWrongTargets(t *testing.T) {
	f := newPlatformAccountsFixture(t)
	ctx := context.Background()
	roleID, _ := addPlatformRole(t, f, "admin.view.platform")
	brandActor := f.actor
	brandActor.SuperAdmin = false
	if _, err := f.store.ListPlatformAdmins(ctx, brandActor, 10, 0); !errors.Is(err, ErrDenied) {
		t.Fatalf("brand admin list error=%v, want denied", err)
	}
	noGrantActor := f.actor
	noGrantActor.Roles = nil
	if _, err := f.store.ListPlatformRoles(ctx, noGrantActor, 10, 0); !errors.Is(err, ErrDenied) {
		t.Fatalf("actor without explicit platform grant error=%v, want denied", err)
	}

	brandTarget := ids.New()
	if _, err := f.store.DB.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'brand-target-hash')`, brandTarget, platformTestName("brand_target")); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.DB.Exec(ctx, `INSERT INTO admin_brand_scopes(account_id,brand_id) VALUES($1,$2)`, brandTarget, managementBrandA); err != nil {
		t.Fatal(err)
	}
	tx := beginManagementTx(t, f.store.DB)
	_, err := f.store.UpdatePlatformAdmin(ctx, tx, f.actor, brandTarget, AdminInput{Version: 1, RoleIDs: []string{roleID}, Status: "active", Reason: "wrong target type"}, identity.Metadata{RequestID: "wrong-target"})
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("brand target update error=%v, want denied", err)
	}
	_ = tx.Rollback(ctx)
}

func TestPlatformAccountsRejectInvalidOrUndelegableRoles(t *testing.T) {
	f := newPlatformAccountsFixture(t)
	ctx := context.Background()
	foreignRole, _ := addScopedTestRole(t, f, managementBrandA, "user.view.brand")
	undelegableRole, _ := addPlatformRole(t, f, "secret.view.platform")
	actorWithoutSecret := f.actor
	for i := range actorWithoutSecret.Roles {
		permissions := actorWithoutSecret.Roles[i].Permissions[:0]
		for _, permission := range actorWithoutSecret.Roles[i].Permissions {
			if permission.Resource != "secret" {
				permissions = append(permissions, permission)
			}
		}
		actorWithoutSecret.Roles[i].Permissions = permissions
	}
	meta := identity.Metadata{RequestID: "invalid-platform-role"}
	for _, test := range []struct {
		name  string
		roles []string
		actor access.Account
		want  error
	}{
		{name: "missing active global role", roles: nil, actor: f.actor, want: ErrInvalid},
		{name: "brand role", roles: []string{foreignRole}, actor: f.actor, want: ErrInvalid},
		{name: "unheld platform permission", roles: []string{undelegableRole}, actor: actorWithoutSecret, want: ErrDenied},
	} {
		t.Run(test.name, func(t *testing.T) {
			tx := beginManagementTx(t, f.store.DB)
			_, err := f.store.CreatePlatformAdmin(ctx, tx, test.actor, AdminInput{Username: platformTestName("invalid_target"), RoleIDs: test.roles, Status: "active", Reason: "validate platform roles"}, "unused-hash", meta)
			if !errors.Is(err, test.want) {
				t.Fatalf("create error=%v, want %v", err, test.want)
			}
			_ = tx.Rollback(ctx)
		})
	}
}
