package adminsys

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

func isolated(t *testing.T) *pgxpool.Pool {
	return testdb.New(t)
}

type fixture struct {
	brandA, brandB                       string
	admin, super, user, memberA, memberB string
}

func seedFixture(t *testing.T, p *pgxpool.Pool) fixture {
	t.Helper()
	f := fixture{
		brandA: "0199a000-0000-7000-8000-000000000001",
		brandB: "0199a000-0000-7000-8000-000000000002",
		admin:  ids.New(), super: ids.New(), user: ids.New(), memberA: ids.New(), memberB: ids.New(),
	}
	ctx := context.Background()
	if _, err := p.Exec(ctx, `INSERT INTO global_users(id,username,phone,password_hash) VALUES($1,'adminsys_member','+10000000000','initial-secret-hash')`, f.user); err != nil {
		t.Fatal(err)
	}
	for _, v := range []struct{ id, brand string }{{f.memberA, f.brandA}, {f.memberB, f.brandB}} {
		if _, err := p.Exec(ctx, `INSERT INTO brand_members(id,brand_id,global_user_id,display_name,notes,join_method,privacy_policy_version,service_terms_version)
		VALUES($1,$2,$3,'Member display','private note','domain','1','1')`, v.id, v.brand, f.user); err != nil {
			t.Fatal(err)
		}
		if _, err := p.Exec(ctx, `INSERT INTO sessions(id,token_hash,user_id,member_id,brand_id,expires_at) VALUES($1,$2,$3,$4,$5,now()+interval '1 day')`, ids.New(), ids.New(), f.user, v.id, v.brand); err != nil {
			t.Fatal(err)
		}
	}
	for _, v := range []struct {
		id, name string
		super    bool
	}{{f.admin, "adminsys_admin", false}, {f.super, "adminsys_super", true}} {
		if _, err := p.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash,is_super_admin) VALUES($1,$2,'not-a-real-password-hash',$3)`, v.id, v.name, v.super); err != nil {
			t.Fatal(err)
		}
	}
	roleID := ids.New()
	if _, err := p.Exec(ctx, `INSERT INTO roles(id,code,name) VALUES($1,'adminsys_test','Adminsys test')`, roleID); err != nil {
		t.Fatal(err)
	}
	grants := []string{"user.view.platform", "user.view.brand", "user.write.brand", "user.kick.brand", "user.password_reset.brand", "user.write.platform"}
	for _, grant := range grants {
		if _, err := p.Exec(ctx, `INSERT INTO permissions(key) VALUES($1)`, grant); err != nil {
			t.Fatal(err)
		}
		if _, err := p.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,$2)`, roleID, grant); err != nil {
			t.Fatal(err)
		}
	}
	for _, accountID := range []string{f.admin, f.super} {
		if _, err := p.Exec(ctx, `INSERT INTO admin_account_roles(account_id,role_id) VALUES($1,$2)`, accountID, roleID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := p.Exec(ctx, `INSERT INTO admin_brand_scopes(account_id,brand_id) VALUES($1,$2)`, f.admin, f.brandA); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestAccountLoadsActiveRolesAndExactScopes(t *testing.T) {
	p := isolated(t)
	f := seedFixture(t, p)
	s := Store{DB: p}
	a, err := s.Account(context.Background(), f.admin)
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != f.admin || a.Type != access.AccountAdmin || a.SuperAdmin || len(a.BrandIDs) != 1 || a.BrandIDs[0] != f.brandA {
		t.Fatalf("unexpected account: %+v", a)
	}
	if !access.Authorize(a, "user", "write", access.ScopeBrand, f.brandA) || access.Authorize(a, "user", "write", access.ScopeBrand, f.brandB) {
		t.Fatalf("unexpected loaded grants: %+v", a)
	}
	if _, err := s.Account(context.Background(), ids.New()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown account error=%v, want ErrNotFound", err)
	}
	if _, err := p.Exec(context.Background(), `UPDATE admin_accounts SET status='disabled' WHERE id=$1`, f.admin); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Account(context.Background(), f.admin); !errors.Is(err, ErrDenied) {
		t.Fatalf("disabled account error=%v, want ErrDenied", err)
	}
}

func TestPlatformViewListsAcrossBrandsWithoutBrandScope(t *testing.T) {
	p := isolated(t)
	f := seedFixture(t, p)
	if _, err := p.Exec(context.Background(), `DELETE FROM admin_brand_scopes WHERE account_id=$1`, f.admin); err != nil {
		t.Fatal(err)
	}
	store := Store{DB: p}
	a, err := store.Account(context.Background(), f.admin)
	if err != nil {
		t.Fatal(err)
	}
	members, err := store.ListMembers(context.Background(), a, "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 2 || members[0].BrandID == members[1].BrandID {
		t.Fatalf("platform view did not list both brand memberships: %+v", members)
	}
	if got := members[0]; got.Username != "adminsys_member" || got.Phone != "+10000000000" || got.DisplayName != "Member display" || got.Notes != "private note" {
		t.Fatalf("member fields not loaded: %+v", got)
	}
}

func TestMemberMutationsAreScopedAndRedactSecrets(t *testing.T) {
	p := isolated(t)
	f := seedFixture(t, p)
	store := Store{DB: p}
	a, err := store.Account(context.Background(), f.admin)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	auditID, err := store.ChangeMember(ctx, tx, a, f.brandA, f.memberA, "frozen", "reviewed", "fraud review", "req-change", "127.0.0.1")
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if auditID == "" {
		t.Fatal("missing audit id")
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var status, notes string
	if err := p.QueryRow(ctx, `SELECT status,notes FROM brand_members WHERE id=$1`, f.memberA).Scan(&status, &notes); err != nil || status != "frozen" || notes != "reviewed" {
		t.Fatalf("member change status=%q notes=%q err=%v", status, notes, err)
	}
	var snapshot string
	if err := p.QueryRow(ctx, `SELECT before_json::text||after_json::text FROM audit_logs WHERE id=$1`, auditID).Scan(&snapshot); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(snapshot, "initial-secret-hash") || strings.Contains(snapshot, "+10000000000") || strings.Contains(snapshot, "private note") {
		t.Fatalf("sensitive data included in audit snapshots: %s", snapshot)
	}
	tx, err = p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.ChangeMember(ctx, tx, a, f.brandB, f.memberB, "disabled", "", "cross brand", "req-cross", "")
	_ = tx.Rollback(ctx)
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("cross-brand write err=%v, want ErrDenied", err)
	}
	adminSuper, err := store.Account(ctx, f.super)
	if err != nil {
		t.Fatal(err)
	}
	tx, err = p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Kick(ctx, tx, adminSuper, f.brandA, f.memberA, "super restriction", "req-super", "")
	_ = tx.Rollback(ctx)
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("super-admin user mutation err=%v, want ErrDenied", err)
	}
}

func TestKickIsBrandLocalAndPasswordResetIsGlobal(t *testing.T) {
	p := isolated(t)
	f := seedFixture(t, p)
	store := Store{DB: p}
	a, err := store.Account(context.Background(), f.admin)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	id, err := store.Kick(ctx, tx, a, f.brandA, f.memberA, "kick reason", "req-kick", "")
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var activeA, activeB int
	if err := p.QueryRow(ctx, `SELECT count(*) FILTER(WHERE brand_id=$1 AND revoked_at IS NULL),count(*) FILTER(WHERE brand_id=$2 AND revoked_at IS NULL) FROM sessions WHERE user_id=$3`, f.brandA, f.brandB, f.user).Scan(&activeA, &activeB); err != nil || activeA != 0 || activeB != 1 {
		t.Fatalf("kick active sessions A=%d B=%d err=%v", activeA, activeB, err)
	}
	if id == "" {
		t.Fatal("missing kick audit id")
	}
	tx, err = p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.ResetPassword(ctx, tx, a, f.brandA, f.memberA, "unauthorized-shared-reset", "reset reason", "req-deny-reset", "")
	_ = tx.Rollback(ctx)
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("brand A operator reset credentials shared with brand B: %v", err)
	}
	var originalHash string
	if err = p.QueryRow(ctx, `SELECT password_hash FROM global_users WHERE id=$1`, f.user).Scan(&originalHash); err != nil || originalHash != "initial-secret-hash" {
		t.Fatalf("denied reset changed global credentials: %v", err)
	}
	// A non-super administrator authorized for every affected brand may reset.
	if _, err = p.Exec(ctx, `INSERT INTO admin_brand_scopes(account_id,brand_id) VALUES($1,$2)`, f.admin, f.brandB); err != nil {
		t.Fatal(err)
	}
	a, err = store.Account(ctx, f.admin)
	if err != nil {
		t.Fatal(err)
	}
	tx, err = p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	id, err = store.ResetPassword(ctx, tx, a, f.brandA, f.memberA, "replacement-secret-hash", "reset reason", "req-reset", "")
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var hash string
	if err := p.QueryRow(ctx, `SELECT password_hash FROM global_users WHERE id=$1`, f.user).Scan(&hash); err != nil || hash != "replacement-secret-hash" {
		t.Fatalf("password reset hash=%q err=%v", hash, err)
	}
	if err := p.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE user_id=$1 AND revoked_at IS NULL`, f.user).Scan(&activeA); err != nil || activeA != 0 {
		t.Fatalf("global password reset left active sessions=%d err=%v", activeA, err)
	}
	var after string
	if err := p.QueryRow(ctx, `SELECT after_json::text FROM audit_logs WHERE id=$1`, id).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(after, "global_user_all_brands") || strings.Contains(after, "replacement-secret-hash") {
		t.Fatalf("global reset audit scope missing or hash leaked: %s", after)
	}
}

func TestInvalidMutationDoesNotWrite(t *testing.T) {
	p := isolated(t)
	f := seedFixture(t, p)
	store := Store{DB: p}
	a, err := store.Account(context.Background(), f.admin)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := p.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.ChangeMember(context.Background(), tx, a, f.brandA, f.memberA, "deleted", "", "bad status", "req-invalid", "")
	_ = tx.Rollback(context.Background())
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid status err=%v, want ErrInvalid", err)
	}
}
