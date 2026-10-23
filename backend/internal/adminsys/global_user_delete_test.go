package adminsys

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/authcrypto"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

func TestGlobalUserDeletionAcrossBrandsPreservesLedger(t *testing.T) {
	p := isolated(t)
	ctx := context.Background()
	f := fixture{brandA: "0199a000-0000-7000-8000-000000000001", brandB: "0199a000-0000-7000-8000-000000000002", admin: ids.New(), super: ids.New(), user: ids.New(), memberA: ids.New(), memberB: ids.New()}
	if _, err := p.Exec(ctx, `INSERT INTO global_users(id,username,password_hash) VALUES($1,'adminsys_member','test-secret')`, f.user); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash,is_super_admin) VALUES($1,'global_delete_admin','unused',true)`, f.super); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version) VALUES($1,$2,$3,'domain','dev-1','dev-1'),($4,$5,$3,'domain','dev-1','dev-1')`, f.memberA, f.brandA, f.user, f.memberB, f.brandB); err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("a", 43)
	digest := authcrypto.DigestSessionToken(token)
	if _, err := p.Exec(ctx, `INSERT INTO sessions(id,token_hash,user_id,member_id,brand_id,expires_at) VALUES($1,$2,$3,$4,$5,now()+interval '1 day')`, ids.New(), hex.EncodeToString(digest[:]), f.user, f.memberB, f.brandB); err != nil {
		t.Fatal(err)
	}
	identityStore, err := identity.New(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = identityStore.Authenticate(ctx, f.brandB, token); err != nil {
		t.Fatal("fixture session unavailable", err)
	}
	otherUser, otherMember := ids.New(), ids.New()
	if _, err = p.Exec(ctx, `INSERT INTO global_users(id,username,password_hash) VALUES($1,'unrelated_member','unrelated-hash')`, otherUser); err != nil {
		t.Fatal(err)
	}
	if _, err = p.Exec(ctx, `INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version) VALUES($1,$2,$3,'domain','dev-1','dev-1')`, otherMember, f.brandA, otherUser); err != nil {
		t.Fatal(err)
	}
	s := Store{DB: p}
	actor := access.Account{ID: f.super, Type: access.AccountAdmin, SuperAdmin: true, Roles: []access.Role{{Permissions: []access.Permission{{Resource: "user", Action: "delete", Scope: access.ScopePlatform}}}}}
	account := ids.New()
	if _, err := p.Exec(ctx, `INSERT INTO point_accounts(id,brand_id,brand_member_id) VALUES($1,$2,$3)`, account, f.brandA, f.memberA); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `INSERT INTO point_buckets(brand_id,account_id,source,state) SELECT $1,$2,s,t FROM unnest(ARRAY['recharge','winning','gift'])s CROSS JOIN unnest(ARRAY['available','manual_frozen','system_frozen','withdrawal'])t`, f.brandA, account); err != nil {
		t.Fatal(err)
	}
	var delta points.Balance
	delta[0][0] = 70
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ledger := points.Store{DB: p}
	_, err = ledger.Post(ctx, tx, points.Change{BrandID: f.brandA, MemberID: f.memberA, EntryType: "credit", ReferenceType: "test", OperationKey: "deletion-test-credit", Reason: "deletion test", ActorType: "system", RequestID: "deletion-test", Delta: delta, Allocation: []points.Allocation{{Source: "recharge", State: "available", Points: 70}}})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	before, err := ledger.Read(ctx, f.brandA, f.memberA)
	if err != nil {
		t.Fatal(err)
	}
	// A brand grant, or super-admin identity without the exact platform grant,
	// cannot widen a local membership deletion into global account deletion.
	for _, denied := range []access.Account{{ID: f.admin, Type: access.AccountAdmin, BrandIDs: []string{f.brandA}, Roles: []access.Role{{BrandID: f.brandA, Permissions: []access.Permission{{Resource: "user", Action: "delete", Scope: access.ScopeBrand}}}}}, {ID: f.super, Type: access.AccountAdmin, SuperAdmin: true}} {
		tx, err = p.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		_, err = s.DeleteGlobalUser(ctx, tx, denied, f.user, "denied", "")
		_ = tx.Rollback(ctx)
		if !errors.Is(err, ErrDenied) {
			t.Fatalf("permission widened: %v", err)
		}
	}
	// Rollback must undo the identity, memberships, sessions and deletion audit.
	tx, err = p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DeleteGlobalUser(ctx, tx, actor, f.user, "rollback", ""); err != nil {
		t.Fatal(err)
	}
	_ = tx.Rollback(ctx)
	var status string
	if err = p.QueryRow(ctx, `SELECT status FROM global_users WHERE id=$1`, f.user).Scan(&status); err != nil || status != "active" {
		t.Fatal(status, err)
	}
	var liveMembers, liveSessions, deletionAudits int
	if err = p.QueryRow(ctx, `SELECT (SELECT count(*) FROM brand_members WHERE global_user_id=$1 AND status='normal'),(SELECT count(*) FROM sessions WHERE user_id=$1 AND revoked_at IS NULL),(SELECT count(*) FROM audit_logs WHERE action='user.delete.platform' AND resource_id=$1)`, f.user).Scan(&liveMembers, &liveSessions, &deletionAudits); err != nil || liveMembers != 2 || liveSessions != 1 || deletionAudits != 0 {
		t.Fatal(liveMembers, liveSessions, deletionAudits, err)
	}
	tx, err = p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	deleted, err := s.DeleteGlobalUser(ctx, tx, actor, f.user, "delete-global", "")
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if deleted.ID != f.user || deleted.AccountName != "adminsys_member" || deleted.AuditLogID == "" {
		t.Fatal(deleted)
	}
	var cancelled, activeSessions, audits int
	var credentialsCleared bool
	if err = p.QueryRow(ctx, `SELECT status,password_hash IS NULL,(SELECT count(*) FROM brand_members WHERE global_user_id=$1 AND status='cancelled'),(SELECT count(*) FROM sessions WHERE user_id=$1 AND revoked_at IS NULL),(SELECT count(*) FROM audit_logs WHERE action='user.delete.platform' AND resource_id=$1) FROM global_users WHERE id=$1`, f.user).Scan(&status, &credentialsCleared, &cancelled, &activeSessions, &audits); err != nil {
		t.Fatal(err)
	}
	if status != "deleted" || !credentialsCleared || cancelled != 2 || activeSessions != 0 || audits != 1 {
		t.Fatal(status, credentialsCleared, cancelled, activeSessions, audits)
	}
	if _, err = identityStore.Authenticate(ctx, f.brandB, token); !errors.Is(err, identity.ErrSession) {
		t.Fatal("deleted identity session remains usable", err)
	}
	var unaffected bool
	if err = p.QueryRow(ctx, `SELECT u.status='active' AND u.password_hash='unrelated-hash' AND m.status='normal' FROM global_users u JOIN brand_members m ON m.global_user_id=u.id WHERE u.id=$1`, otherUser).Scan(&unaffected); err != nil || !unaffected {
		t.Fatal("unrelated account changed", err)
	}
	after, err := ledger.Read(ctx, f.brandA, f.memberA)
	if err != nil || after.DisplayPoints != before.DisplayPoints || after.Version != before.Version || after.BySource != before.BySource {
		t.Fatalf("wallet changed: before=%+v after=%+v err=%v", before, after, err)
	}
	var count int
	if err = p.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE account_id=$1`, account).Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	tx, err = p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.DeleteGlobalUser(ctx, tx, actor, f.user, "delete-again", "")
	_ = tx.Rollback(ctx)
	if !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}
