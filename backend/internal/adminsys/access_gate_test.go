package adminsys

import (
	"context"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
)

func TestACLSharedGateBlocksRevocationAndReloadsPermissions(t *testing.T) {
	p := testdb.New(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s := Store{DB: p}
	account, role := ids.New(), ids.New()
	brand := "0199a000-0000-7000-8000-000000000001"
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,'gate_account','test-only-opaque-hash')`, []any{account}},
		{`INSERT INTO admin_brand_scopes(account_id,brand_id) VALUES($1,$2)`, []any{account, brand}},
		{`INSERT INTO roles(id,brand_id,code,name) VALUES($1,$2,'gate_role','Gate role')`, []any{role, brand}},
		{`INSERT INTO permissions(key) VALUES('user.write.brand') ON CONFLICT DO NOTHING`, nil},
		{`INSERT INTO role_permissions(role_id,permission_key) VALUES($1,'user.write.brand')`, []any{role}},
		{`INSERT INTO admin_account_roles(account_id,role_id) VALUES($1,$2)`, []any{account, role}},
	} {
		if _, err := p.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	shared, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer shared.Rollback(context.Background())
	before, err := s.LockAdminAccess(ctx, shared, account, false)
	if err != nil || !access.Authorize(before, "user", "write", access.ScopeBrand, brand) {
		t.Fatal("missing initial grant", err)
	}
	probe, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer probe.Rollback(context.Background())
	var acquired bool
	if err = probe.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock($1)`, adminAccessLock).Scan(&acquired); err != nil || acquired {
		t.Fatal("exclusive ACL update bypassed active shared authorization", err)
	}
	if err = shared.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	// After the authorized operation commits, revocation obtains its exclusive gate.
	if _, err = probe.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, adminAccessLock); err != nil {
		t.Fatal(err)
	}
	if _, err = probe.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1`, role); err != nil {
		t.Fatal(err)
	}
	if err = probe.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	next, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Rollback(context.Background())
	after, err := s.LockAdminAccess(ctx, next, account, false)
	if err != nil {
		t.Fatal(err)
	}
	if access.Authorize(after, "user", "write", access.ScopeBrand, brand) {
		t.Fatal("permission reload reused stale authorization")
	}
}

func TestRoleOwnershipAndAssignedScopeCannotBeRewritten(t *testing.T) {
	p := testdb.New(t)
	ctx := context.Background()
	account, role := ids.New(), ids.New()
	a := "0199a000-0000-7000-8000-000000000001"
	b := "0199a000-0000-7000-8000-000000000002"
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,'scope_account','test-only-opaque-hash')`, []any{account}},
		{`INSERT INTO admin_brand_scopes(account_id,brand_id) VALUES($1,$2)`, []any{account, a}},
		{`INSERT INTO roles(id,brand_id,code,name) VALUES($1,$2,'scope_role','Scope role')`, []any{role, a}},
		{`INSERT INTO admin_account_roles(account_id,role_id) VALUES($1,$2)`, []any{account, role}},
	} {
		if _, err := p.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := p.Exec(ctx, `UPDATE roles SET brand_id=$2 WHERE id=$1`, role, b); err == nil {
		t.Fatal("role changed owner brand")
	}
	if _, err := p.Exec(ctx, `DELETE FROM admin_brand_scopes WHERE account_id=$1 AND brand_id=$2`, account, a); err == nil {
		t.Fatal("scope detached while brand role remained assigned")
	}
	foreign := ids.New()
	if _, err := p.Exec(ctx, `INSERT INTO roles(id,brand_id,code,name) VALUES($1,$2,'scope_role','Same code in different brand')`, foreign, b); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `INSERT INTO admin_account_roles(account_id,role_id) VALUES($1,$2)`, account, foreign); err == nil {
		t.Fatal("assigned another brand's role without membership scope")
	}
}
