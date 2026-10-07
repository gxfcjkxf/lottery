package adminsys

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	managementBrandA = "0199a000-0000-7000-8000-000000000001"
	managementBrandB = "0199a000-0000-7000-8000-000000000002"
)

type managementFixture struct {
	pool    *pgxpool.Pool
	store   Store
	actor   access.Account
	roleID  string
	brandID string
}

func managementFixtureFor(t *testing.T, grants ...string) managementFixture {
	t.Helper()
	p := testdb.New(t)
	ctx := context.Background()
	actorID, roleID := ids.New(), ids.New()
	roleBrand := ""
	for _, key := range grants {
		_, _, scope, ok := parsePermission(key)
		if !ok {
			t.Fatalf("bad test grant %q", key)
		}
		if scope == access.ScopeBrand {
			roleBrand = managementBrandA
		}
	}
	for _, key := range []string{"user.view.brand", "user.write.brand", "secret.manage.brand", "admin.write.platform"} {
		if _, err := p.Exec(ctx, `INSERT INTO permissions(key) VALUES($1) ON CONFLICT DO NOTHING`, key); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := p.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,'management_actor','opaque-hash')`, actorID); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `INSERT INTO admin_brand_scopes(account_id,brand_id) VALUES($1,$2)`, actorID, managementBrandA); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `INSERT INTO roles(id,brand_id,code,name) VALUES($1,NULLIF($2,'')::uuid,'management_actor','Management actor')`, roleID, roleBrand); err != nil {
		t.Fatal(err)
	}
	role := access.Role{BrandID: roleBrand}
	for _, key := range grants {
		if _, err := p.Exec(ctx, `INSERT INTO permissions(key) VALUES($1) ON CONFLICT DO NOTHING`, key); err != nil {
			t.Fatal(err)
		}
		if _, err := p.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,$2)`, roleID, key); err != nil {
			t.Fatal(err)
		}
		resource, action, scope, ok := parsePermission(key)
		if !ok {
			t.Fatalf("bad test grant %q", key)
		}
		role.Permissions = append(role.Permissions, access.Permission{Resource: resource, Action: action, Scope: scope})
	}
	if _, err := p.Exec(ctx, `INSERT INTO admin_account_roles(account_id,role_id) VALUES($1,$2)`, actorID, roleID); err != nil {
		t.Fatal(err)
	}
	return managementFixture{pool: p, store: Store{DB: p}, actor: access.Account{ID: actorID, Type: access.AccountAdmin, Roles: []access.Role{role}, BrandIDs: []string{managementBrandA}}, roleID: roleID, brandID: managementBrandA}
}

func beginManagementTx(t *testing.T, p *pgxpool.Pool) pgx.Tx {
	t.Helper()
	tx, err := p.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	return tx
}

func commitManagement(t *testing.T, tx pgx.Tx) {
	t.Helper()
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func managementGrants() []string {
	return []string{"role.view.brand", "role.write.brand", "admin.view.brand", "admin.write.brand", "user.view.brand"}
}

func TestManagementInputsUseSnakeCaseJSON(t *testing.T) {
	var role RoleInput
	if err := json.Unmarshal([]byte(`{"version":7,"code":"ops_role","name":"Ops","status":"disabled","permissions":["user.view.brand"],"reason":"review"}`), &role); err != nil {
		t.Fatal(err)
	}
	if role.Version != 7 || role.Code != "ops_role" || role.Name != "Ops" || role.Status != "disabled" || len(role.Permissions) != 1 || role.Reason != "review" {
		t.Fatalf("snake_case role input did not decode: %+v", role)
	}
	var admin AdminInput
	if err := json.Unmarshal([]byte(`{"username":"operator","version":9,"role_ids":["0199a000-0000-7000-8000-000000000001"],"status":"active","reason":"review"}`), &admin); err != nil {
		t.Fatal(err)
	}
	if admin.Version != 9 || len(admin.RoleIDs) != 1 || admin.RoleIDs[0] != managementBrandA {
		t.Fatalf("snake_case admin input did not decode: %+v", admin)
	}
}

func TestManagementInputBoundsAndUTF8(t *testing.T) {
	if _, err := cleanReason(string([]byte{0xff})); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid UTF-8 reason error=%v, want invalid", err)
	}
	if validRoleFields(string([]byte{0xff}), "active") {
		t.Fatal("invalid UTF-8 role name was accepted")
	}
	permissions := make([]string, 101)
	for i := range permissions {
		permissions[i] = "user.view.brand"
	}
	if _, err := cleanPermissions(permissions); !errors.Is(err, ErrInvalid) {
		t.Fatalf("101 permissions error=%v, want invalid", err)
	}
	roleIDs := make([]string, 101)
	for i := range roleIDs {
		roleIDs[i] = managementBrandA
	}
	if _, err := cleanRoleIDs(roleIDs); !errors.Is(err, ErrInvalid) {
		t.Fatalf("101 role IDs error=%v, want invalid", err)
	}
	if _, err := cleanRoleIDs([]string{"not-a-uuid"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("malformed role ID error=%v, want invalid", err)
	}
}

func TestManagementDelegationBlocksBootstrapAndHigherPrivilegeTakeover(t *testing.T) {
	f := managementFixtureFor(t, "admin.write.brand", "user.view.brand")
	ctx := context.Background()
	meta := identity.Metadata{RequestID: "management-delegation-test"}
	bootstrapID, safeRoleID := ids.New(), ids.New()
	for _, key := range []string{"role.write.brand", "admin.write.platform"} {
		if _, err := f.pool.Exec(ctx, `INSERT INTO permissions(key) VALUES($1) ON CONFLICT DO NOTHING`, key); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO roles(id,brand_id,code,name,is_bootstrap) VALUES($1,$2,'bootstrap_owner','Bootstrap owner',true)`, bootstrapID, managementBrandA); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,'role.write.brand')`, bootstrapID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO roles(id,brand_id,code,name) VALUES($1,$2,'safe_viewer','Safe viewer')`, safeRoleID, managementBrandA); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,'user.view.brand')`, safeRoleID); err != nil {
		t.Fatal(err)
	}
	tx := beginManagementTx(t, f.pool)
	_, err := f.store.CreateAdmin(ctx, tx, f.actor, managementBrandA,
		AdminInput{Username: "bootstrap_target", RoleIDs: []string{bootstrapID}, Status: "active", Reason: "delegation attempt"}, "opaque-hash", meta)
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("admin.write-only actor assigned bootstrap role: %v", err)
	}
	_ = tx.Rollback(ctx)

	for _, target := range []struct {
		username, code, permission string
		platformRole               bool
	}{{"high_target", "high_brand_role", "role.write.brand", false}, {"platform_target", "platform_owner", "admin.write.platform", true}} {
		adminID, roleID := ids.New(), ids.New()
		roleBrand := managementBrandA
		if target.platformRole {
			roleBrand = ""
		}
		if _, err := f.pool.Exec(ctx, `INSERT INTO roles(id,brand_id,code,name) VALUES($1,NULLIF($2,'')::uuid,$3,'Protected role')`, roleID, roleBrand, target.code); err != nil {
			t.Fatal(err)
		}
		if _, err := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,$2)`, roleID, target.permission); err != nil {
			t.Fatal(err)
		}
		if _, err := f.pool.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'original-hash')`, adminID, target.username); err != nil {
			t.Fatal(err)
		}
		if _, err := f.pool.Exec(ctx, `INSERT INTO admin_brand_scopes(account_id,brand_id) VALUES($1,$2)`, adminID, managementBrandA); err != nil {
			t.Fatal(err)
		}
		if _, err := f.pool.Exec(ctx, `INSERT INTO admin_account_roles(account_id,role_id) VALUES($1,$2)`, adminID, roleID); err != nil {
			t.Fatal(err)
		}
		sessionID := ids.New()
		if _, err := f.pool.Exec(ctx, `INSERT INTO sessions(id,token_hash,admin_id,expires_at) VALUES($1,$2,$3,now()+interval '1 day')`, sessionID, ids.New(), adminID); err != nil {
			t.Fatal(err)
		}

		tx = beginManagementTx(t, f.pool)
		_, err = f.store.UpdateAdmin(ctx, tx, f.actor, managementBrandA, adminID, AdminInput{Version: 1, RoleIDs: []string{safeRoleID}, Status: "disabled", Reason: "takeover attempt"}, meta)
		if !errors.Is(err, ErrDenied) {
			t.Fatalf("updated higher privilege target %q: %v", target.username, err)
		}
		_ = tx.Rollback(ctx)
		tx = beginManagementTx(t, f.pool)
		_, err = f.store.ResetAdminPassword(ctx, tx, f.actor, managementBrandA, adminID, 1, "replacement-hash", "takeover attempt", meta)
		if !errors.Is(err, ErrDenied) {
			t.Fatalf("reset higher privilege target %q: %v", target.username, err)
		}
		_ = tx.Rollback(ctx)

		var hash string
		var activeSessions int
		if err := f.pool.QueryRow(ctx, `SELECT password_hash FROM admin_accounts WHERE id=$1`, adminID).Scan(&hash); err != nil || hash != "original-hash" {
			t.Fatalf("denied operation changed %q password hash: %q err=%v", target.username, hash, err)
		}
		if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE id=$1 AND revoked_at IS NULL`, sessionID).Scan(&activeSessions); err != nil || activeSessions != 1 {
			t.Fatalf("denied operation revoked %q session: count=%d err=%v", target.username, activeSessions, err)
		}
	}
}

func TestRoleWriteCannotBorrowPermissionFromAnotherBrand(t *testing.T) {
	f := managementFixtureFor(t, "role.write.brand")
	f.actor.BrandIDs = append(f.actor.BrandIDs, managementBrandB)
	f.actor.Roles = append(f.actor.Roles, access.Role{BrandID: managementBrandB, Permissions: []access.Permission{{Resource: "user", Action: "write", Scope: access.ScopeBrand}}})
	tx := beginManagementTx(t, f.pool)
	_, err := f.store.WriteRole(context.Background(), tx, f.actor, managementBrandA, "", RoleInput{Code: "borrowed_grant", Name: "Borrowed grant", Status: "active", Permissions: []string{"user.write.brand"}, Reason: "cross-brand grant"}, identity.Metadata{RequestID: "cross-brand-grant"})
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("B-scoped user.write granted to A role: %v", err)
	}
	_ = tx.Rollback(context.Background())
}

func TestResetAdminPasswordRevokesAndRedactsHash(t *testing.T) {
	f := managementFixtureFor(t, "admin.write.brand", "user.view.brand")
	ctx := context.Background()
	meta := identity.Metadata{RequestID: "management-password-reset"}
	roleID := ids.New()
	if _, err := f.pool.Exec(ctx, `INSERT INTO roles(id,brand_id,code,name) VALUES($1,$2,'reset_target','Reset target')`, roleID, managementBrandA); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,'user.view.brand')`, roleID); err != nil {
		t.Fatal(err)
	}
	tx := beginManagementTx(t, f.pool)
	admin, err := f.store.CreateAdmin(ctx, tx, f.actor, managementBrandA, AdminInput{Username: "reset_target", RoleIDs: []string{roleID}, Status: "active", Reason: "create target"}, "old-hash", meta)
	if err != nil {
		t.Fatal(err)
	}
	commitManagement(t, tx)
	sessionID := ids.New()
	if _, err := f.pool.Exec(ctx, `INSERT INTO sessions(id,token_hash,admin_id,expires_at) VALUES($1,$2,$3,now()+interval '1 day')`, sessionID, ids.New(), admin.ID); err != nil {
		t.Fatal(err)
	}
	tx = beginManagementTx(t, f.pool)
	auditID, err := f.store.ResetAdminPassword(ctx, tx, f.actor, managementBrandA, admin.ID, admin.Version, "replacement-hash-secret", "credential reset", meta)
	if err != nil {
		t.Fatal(err)
	}
	commitManagement(t, tx)
	var hash string
	var version int64
	if err := f.pool.QueryRow(ctx, `SELECT password_hash,version FROM admin_accounts WHERE id=$1`, admin.ID).Scan(&hash, &version); err != nil || hash != "replacement-hash-secret" || version != admin.Version+1 {
		t.Fatalf("reset account hash=%q version=%d err=%v", hash, version, err)
	}
	var revoked bool
	if err := f.pool.QueryRow(ctx, `SELECT revoked_at IS NOT NULL FROM sessions WHERE id=$1`, sessionID).Scan(&revoked); err != nil || !revoked {
		t.Fatalf("password reset left session active: revoked=%v err=%v", revoked, err)
	}
	var snapshot string
	if err := f.pool.QueryRow(ctx, `SELECT before_json::text||after_json::text FROM audit_logs WHERE id=$1`, auditID).Scan(&snapshot); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(snapshot, "replacement-hash-secret") || !strings.Contains(snapshot, `"role_ids"`) {
		t.Fatalf("password reset audit leaked hash or omitted account config: %s", snapshot)
	}
}

func TestManagementRoleAdminBrandSecurityCASAndDedup(t *testing.T) {
	grants := managementGrants()
	f := managementFixtureFor(t, grants...)
	ctx := context.Background()
	meta := identity.Metadata{RequestID: "management-test"}

	if _, err := f.store.ListRoles(ctx, f.actor, managementBrandB, 10, 0); !errors.Is(err, ErrDenied) {
		t.Fatalf("cross-brand role list error=%v, want denied", err)
	}
	if _, err := f.store.ListAdmins(ctx, f.actor, managementBrandB, 10, 0); !errors.Is(err, ErrDenied) {
		t.Fatalf("cross-brand admin list error=%v, want denied", err)
	}
	if _, err := f.store.ListAdmins(ctx, f.actor, managementBrandA, 101, 0); !errors.Is(err, ErrInvalid) {
		t.Fatalf("oversized page error=%v, want invalid", err)
	}

	// A brand operator cannot edit a role that is currently granting their own
	// management authority, even when the replacement is otherwise valid.
	tx := beginManagementTx(t, f.pool)
	_, err := f.store.WriteRole(ctx, tx, f.actor, managementBrandA, f.roleID, RoleInput{Version: 1, Name: "Escalated", Status: "active", Permissions: grants, Reason: "self escalation"}, meta)
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("editing own assigned role error=%v, want denied", err)
	}
	_ = tx.Rollback(ctx)

	// A manager may grant only registered brand permissions that they hold.
	tx = beginManagementTx(t, f.pool)
	_, err = f.store.WriteRole(ctx, tx, f.actor, managementBrandA, "", RoleInput{Code: "elevated_role", Name: "Elevated role", Status: "active", Permissions: []string{"role.write.brand", "secret.manage.brand"}, Reason: "excess grant"}, meta)
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("granting an unheld permission error=%v, want denied", err)
	}
	_ = tx.Rollback(ctx)
	tx = beginManagementTx(t, f.pool)
	_, err = f.store.WriteRole(ctx, tx, f.actor, managementBrandA, "", RoleInput{Code: "platform_role", Name: "Platform role", Status: "active", Permissions: []string{"admin.write.platform"}, Reason: "scope check"}, meta)
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("granting platform permission error=%v, want invalid", err)
	}
	_ = tx.Rollback(ctx)

	tx = beginManagementTx(t, f.pool)
	role, err := f.store.WriteRole(ctx, tx, f.actor, managementBrandA, "", RoleInput{Code: "limited_role", Name: "Limited role", Status: "active", Permissions: []string{"user.view.brand"}, Reason: "create role"}, meta)
	if err != nil {
		t.Fatal(err)
	}
	commitManagement(t, tx)
	if len(role.ID) != 36 || role.Version != 1 || role.BrandID != managementBrandA || role.IsBootstrap {
		t.Fatalf("unexpected new role record: %+v", role)
	}
	tx = beginManagementTx(t, f.pool)
	_, err = f.store.WriteRole(ctx, tx, f.actor, managementBrandA, "", RoleInput{Code: "limited_role", Name: "Duplicate", Status: "active", Permissions: []string{"user.view.brand"}, Reason: "duplicate role"}, meta)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate role code error=%v, want conflict", err)
	}
	_ = tx.Rollback(ctx)

	tx = beginManagementTx(t, f.pool)
	created, err := f.store.CreateAdmin(ctx, tx, f.actor, managementBrandA,
		AdminInput{Username: "target_admin", RoleIDs: []string{role.ID, role.ID}, Status: "active", Reason: "create target"}, "opaque-hash", meta)
	if err != nil {
		t.Fatal(err)
	}
	commitManagement(t, tx)
	if len(created.RoleIDs) != 1 || len(created.RoleCodes) != 1 || len(created.BrandIDs) != 1 || created.BrandIDs[0] != managementBrandA {
		t.Fatalf("duplicate roles or scope leaked into account record: %+v", created)
	}

	// Brand operators neither see nor modify a multi-brand account.
	if _, err := f.pool.Exec(ctx, `INSERT INTO admin_brand_scopes(account_id,brand_id) VALUES($1,$2)`, created.ID, managementBrandB); err != nil {
		t.Fatal(err)
	}
	listed, err := f.store.ListAdmins(ctx, f.actor, managementBrandA, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range listed {
		if row.ID == created.ID {
			t.Fatalf("brand admin listing disclosed multi-brand account %+v", row)
		}
	}
	tx = beginManagementTx(t, f.pool)
	_, err = f.store.UpdateAdmin(ctx, tx, f.actor, managementBrandA, created.ID, AdminInput{Version: 1, RoleIDs: []string{role.ID}, Status: "active", Reason: "cross scope"}, meta)
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("editing multi-brand account error=%v, want denied", err)
	}
	_ = tx.Rollback(ctx)

	tx = beginManagementTx(t, f.pool)
	_, err = f.store.UpdateAdmin(ctx, tx, f.actor, managementBrandA, f.actor.ID, AdminInput{Version: 1, RoleIDs: []string{role.ID}, Status: "active", Reason: "self edit"}, meta)
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("editing own account error=%v, want denied", err)
	}
	_ = tx.Rollback(ctx)
	if acquired := f.pool.Stat().AcquiredConns(); acquired != 0 {
		t.Fatalf("completed management transactions retained %d pool connections", acquired)
	}
}

func TestManagementRoleUpdateAndAdminDisableRevokeSessions(t *testing.T) {
	grants := managementGrants()
	f := managementFixtureFor(t, grants...)
	ctx := context.Background()
	meta := identity.Metadata{RequestID: "management-session-test"}
	tx := beginManagementTx(t, f.pool)
	role, err := f.store.WriteRole(ctx, tx, f.actor, managementBrandA, "", RoleInput{Code: "target_role", Name: "Target role", Status: "active", Permissions: []string{"user.view.brand"}, Reason: "create role"}, meta)
	if err != nil {
		t.Fatal(err)
	}
	commitManagement(t, tx)
	tx = beginManagementTx(t, f.pool)
	admin, err := f.store.CreateAdmin(ctx, tx, f.actor, managementBrandA,
		AdminInput{Username: "session_target", RoleIDs: []string{role.ID}, Status: "active", Reason: "create target"}, "opaque-password-hash", meta)
	if err != nil {
		t.Fatal(err)
	}
	commitManagement(t, tx)
	sessionID := ids.New()
	if _, err := f.pool.Exec(ctx, `INSERT INTO sessions(id,token_hash,admin_id,expires_at) VALUES($1,$2,$3,now()+interval '1 day')`, sessionID, ids.New(), admin.ID); err != nil {
		t.Fatal(err)
	}

	tx = beginManagementTx(t, f.pool)
	updatedRole, err := f.store.WriteRole(ctx, tx, f.actor, managementBrandA, role.ID, RoleInput{Version: role.Version, Name: "Target role", Status: "disabled", Permissions: []string{"user.view.brand"}, Reason: "disable target role"}, meta)
	if err != nil {
		t.Fatal(err)
	}
	commitManagement(t, tx)
	if updatedRole.Version != 2 {
		t.Fatalf("role version=%d, want 2", updatedRole.Version)
	}
	var roleBefore, roleAfter string
	if err := f.pool.QueryRow(ctx, `SELECT before_json::text,after_json::text FROM audit_logs WHERE id=$1`, updatedRole.AuditLogID).Scan(&roleBefore, &roleAfter); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(roleBefore, `"name": "Target role"`) || !strings.Contains(roleBefore, `"permissions": ["user.view.brand"]`) || !strings.Contains(roleAfter, `"name": "Target role"`) {
		t.Fatalf("role audit snapshots omit old/new configuration: before=%s after=%s", roleBefore, roleAfter)
	}
	staleTx := beginManagementTx(t, f.pool)
	if _, err := f.store.WriteRole(ctx, staleTx, f.actor, managementBrandA, role.ID, RoleInput{Version: role.Version, Name: "Stale role", Status: "active", Permissions: []string{"user.view.brand"}, Reason: "stale role update"}, meta); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale role update error=%v, want conflict", err)
	}
	_ = staleTx.Rollback(ctx)
	var revoked bool
	if err := f.pool.QueryRow(ctx, `SELECT revoked_at IS NOT NULL FROM sessions WHERE id=$1`, sessionID).Scan(&revoked); err != nil || !revoked {
		t.Fatalf("role change did not revoke affected admin session: revoked=%v err=%v", revoked, err)
	}
	var targetVersion int64
	if err := f.pool.QueryRow(ctx, `SELECT version FROM admin_accounts WHERE id=$1`, admin.ID).Scan(&targetVersion); err != nil || targetVersion != 2 {
		t.Fatalf("role change target version=%d err=%v, want 2", targetVersion, err)
	}

	// Re-enable the role, create a fresh session, then disable the account with
	// its current CAS version and verify that both mutations remain auditable.
	tx = beginManagementTx(t, f.pool)
	roleEnabled, err := f.store.WriteRole(ctx, tx, f.actor, managementBrandA, role.ID, RoleInput{Version: updatedRole.Version, Name: "Target role", Status: "active", Permissions: []string{"user.view.brand"}, Reason: "re-enable role"}, meta)
	if err != nil {
		t.Fatal(err)
	}
	commitManagement(t, tx)
	if err := f.pool.QueryRow(ctx, `SELECT version FROM admin_accounts WHERE id=$1`, admin.ID).Scan(&targetVersion); err != nil {
		t.Fatal(err)
	}
	accountID := ids.New()
	if _, err := f.pool.Exec(ctx, `INSERT INTO sessions(id,token_hash,admin_id,expires_at) VALUES($1,$2,$3,now()+interval '1 day')`, accountID, ids.New(), admin.ID); err != nil {
		t.Fatal(err)
	}
	tx = beginManagementTx(t, f.pool)
	updatedAdmin, err := f.store.UpdateAdmin(ctx, tx, f.actor, managementBrandA, admin.ID,
		AdminInput{Version: targetVersion, RoleIDs: []string{role.ID}, Status: "disabled", Reason: "disable account"}, meta)
	if err != nil {
		t.Fatal(err)
	}
	commitManagement(t, tx)
	if updatedAdmin.Status != "disabled" || updatedAdmin.Version != targetVersion+1 || updatedAdmin.Username != "session_target" || roleEnabled.Version != 3 {
		t.Fatalf("unexpected update records admin=%+v role=%+v", updatedAdmin, roleEnabled)
	}
	adminTx := beginManagementTx(t, f.pool)
	if _, err := f.store.UpdateAdmin(ctx, adminTx, f.actor, managementBrandA, admin.ID, AdminInput{Username: "different_name", Version: updatedAdmin.Version, RoleIDs: []string{role.ID}, Status: "active", Reason: "username mutation"}, meta); !errors.Is(err, ErrInvalid) {
		t.Fatalf("store accepted username update: %v", err)
	}
	_ = adminTx.Rollback(ctx)
	var adminBefore string
	if err := f.pool.QueryRow(ctx, `SELECT before_json::text FROM audit_logs WHERE id=$1`, updatedAdmin.AuditLogID).Scan(&adminBefore); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(adminBefore, `"role_ids": ["`+role.ID+`"]`) {
		t.Fatalf("admin update audit omitted old role IDs: %s", adminBefore)
	}
	if err := f.pool.QueryRow(ctx, `SELECT revoked_at IS NOT NULL FROM sessions WHERE id=$1`, accountID).Scan(&revoked); err != nil || !revoked {
		t.Fatalf("account update did not revoke session: revoked=%v err=%v", revoked, err)
	}
	staleAdminTx := beginManagementTx(t, f.pool)
	if _, err := f.store.UpdateAdmin(ctx, staleAdminTx, f.actor, managementBrandA, admin.ID,
		AdminInput{Version: targetVersion, RoleIDs: []string{role.ID}, Status: "active", Reason: "stale update"}, meta); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale account update error=%v, want conflict", err)
	}
	_ = staleAdminTx.Rollback(ctx)
	var actions int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE resource_id=$1 AND action='admin.update'`, admin.ID).Scan(&actions); err != nil || actions != 1 {
		t.Fatalf("admin update audit entries=%d err=%v, want 1", actions, err)
	}
}

func TestPlatformManagementAndBootstrapProtection(t *testing.T) {
	f := managementFixtureFor(t, "role.view.platform", "role.write.platform", "admin.view.platform", "admin.write.platform")
	ctx := context.Background()
	meta := identity.Metadata{RequestID: "platform-management-test"}
	if _, err := f.pool.Exec(ctx, `DELETE FROM admin_brand_scopes WHERE account_id=$1`, f.actor.ID); err != nil {
		t.Fatal(err)
	}
	f.actor.BrandIDs = nil
	bootstrapID := ids.New()
	if _, err := f.pool.Exec(ctx, `INSERT INTO roles(id,brand_id,code,name,is_bootstrap) VALUES($1,$2,'bootstrap_keeper','Bootstrap',true)`, bootstrapID, managementBrandA); err != nil {
		t.Fatal(err)
	}
	bootstrapTx := beginManagementTx(t, f.pool)
	if _, err := f.store.WriteRole(ctx, bootstrapTx, f.actor, managementBrandA, bootstrapID, RoleInput{Version: 1, Name: "Changed", Status: "disabled", Reason: "attempt bootstrap edit"}, meta); !errors.Is(err, ErrDenied) {
		t.Fatalf("editing bootstrap role error=%v, want denied", err)
	}
	_ = bootstrapTx.Rollback(ctx)

	tx := beginManagementTx(t, f.pool)
	role, err := f.store.WriteRole(ctx, tx, f.actor, managementBrandA, "", RoleInput{Code: "platform_created", Name: "Platform-created brand role", Permissions: []string{"user.view.brand"}, Reason: "platform role create"}, meta)
	if err != nil {
		t.Fatal(err)
	}
	commitManagement(t, tx)
	if len(role.Permissions) != 1 || role.BrandID != managementBrandA || role.Status != "active" {
		t.Fatalf("platform grant created wrong role scope: %+v", role)
	}
	perms, err := f.store.ListPermissions(ctx, f.actor, managementBrandB)
	if err != nil {
		t.Fatal(err)
	}
	if len(perms) == 0 {
		t.Fatal("platform operator did not receive registered brand permissions")
	}
}
