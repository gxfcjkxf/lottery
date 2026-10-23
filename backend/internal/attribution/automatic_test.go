package attribution_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/attribution"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
)

func TestAutomaticReferralBackfillConcurrencyAuditAndBrandIsolation(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	other := "0199a000-0000-7000-8000-000000000002"
	user, member, foreign, existingMember, admin := ids.New(), ids.New(), ids.New(), ids.New(), ids.New()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO global_users(id,username)VALUES($1,'automatic_referral_owner')`, []any{user}},
		{`INSERT INTO global_users(id,username)VALUES($1,'existing_referral_owner')`, []any{existingMember}},
		{`INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version) VALUES($1,$2,$3,'domain','dev-1','dev-1')`, []any{member, brand, user}},
		{`INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version) VALUES($1,$2,$3,'domain','dev-1','dev-1')`, []any{foreign, other, user}},
		{`INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version) VALUES($1,$2,$3,'domain','dev-1','dev-1')`, []any{existingMember, brand, existingMember}},
		{`INSERT INTO admin_accounts(id,username,password_hash)VALUES($1,'automatic_referral_admin','unused')`, []any{admin}},
	} {
		if _, err := db.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	actor := access.Account{ID: admin, Type: access.AccountAdmin, BrandIDs: []string{brand}, Roles: []access.Role{{BrandID: brand, Permissions: []access.Permission{{Resource: "join_code", Action: "write", Scope: access.ScopeBrand}}}}}
	svc := attribution.Service{DB: db}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	original, err := svc.Create(ctx, tx, brand, actor, attribution.CreateInput{Kind: "referral", OwnerMemberID: existingMember, Reason: "Existing referral retained"}, points.Metadata{RequestID: ids.New()})
	if err != nil {
		tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	// Rollback must remove both the code and its system audit/history.
	tx, err = db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `SELECT ensure_member_referral($1,$2)`, brand, member); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM join_codes WHERE owner_member_id=$1`, member).Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
	// Concurrent calls must all return the same identity, not just one active code.
	var group sync.WaitGroup
	results := make(chan string, 12)
	failures := make(chan error, 12)
	for i := 0; i < 12; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			var id string
			err := db.QueryRow(ctx, `SELECT ensure_member_referral($1,$2)::text`, brand, member).Scan(&id)
			if err != nil {
				failures <- err
			} else {
				results <- id
			}
		}()
	}
	group.Wait()
	close(results)
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	var own string
	for id := range results {
		if own == "" {
			own = id
		}
		if own != id {
			t.Fatal("duplicate allocation", own, id)
		}
	}
	// This is the same backfill statement used by the migration, repeated twice.
	for i := 0; i < 2; i++ {
		if _, err = db.Exec(ctx, `SELECT ensure_member_referral(brand_id,id) FROM brand_members ORDER BY brand_id,id`); err != nil {
			t.Fatal(err)
		}
	}
	var retained, foreignCode, ownCode string
	if err = db.QueryRow(ctx, `SELECT id::text FROM join_codes WHERE brand_id=$1 AND owner_member_id=$2 AND kind='referral'`, brand, existingMember).Scan(&retained); err != nil || retained != original.ID {
		t.Fatal(retained, original.ID, err)
	}
	if err = db.QueryRow(ctx, `SELECT code FROM join_codes WHERE brand_id=$1 AND owner_member_id=$2 AND kind='referral'`, brand, member).Scan(&ownCode); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, `SELECT code FROM join_codes WHERE brand_id=$1 AND owner_member_id=$2 AND kind='referral'`, other, foreign).Scan(&foreignCode); err != nil || ownCode == foreignCode {
		t.Fatal("brand code isolation", err)
	}
	if valid, err := svc.Validate(ctx, other, "", ownCode); err != nil || valid {
		t.Fatal("foreign code accepted", valid, err)
	}
	if _, err = db.Exec(ctx, `SELECT ensure_member_referral($1,$2)`, other, member); err == nil {
		t.Fatal("allocated a referral for a membership in another brand")
	}
	history, err := svc.History(ctx, brand, own, 20, 0)
	if err != nil || len(history.Items) != 1 || history.Items[0].ActorID != nil {
		t.Fatal(history, err)
	}
	var actorType string
	if err = db.QueryRow(ctx, `SELECT actor_type FROM audit_logs WHERE id=$1`, history.Items[0].AuditLogID).Scan(&actorType); err != nil || actorType != "system" {
		t.Fatal(actorType, err)
	}
	// A disabled dedicated code is retained, not replaced with a fresh usable code.
	tx, err = db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Update(ctx, tx, brand, actor, own, attribution.UpdateInput{Version: 1, Status: "disabled", Reason: "Disable own referral"}, points.Metadata{RequestID: ids.New()})
	if err != nil {
		tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var repeated string
	if err = db.QueryRow(ctx, `SELECT ensure_member_referral($1,$2)::text`, brand, member).Scan(&repeated); err != nil || repeated != own {
		t.Fatal(repeated, err)
	}
	tx, err = db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Create(ctx, tx, brand, actor, attribution.CreateInput{Kind: "referral", OwnerMemberID: member, Reason: "Duplicate must fail"}, points.Metadata{RequestID: ids.New()})
	tx.Rollback(ctx)
	if err != attribution.ErrState {
		t.Fatalf("duplicate manual allocation: %v", err)
	}
	if err = db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries`).Scan(&count); err != nil || count != 0 {
		t.Fatal("referral allocation touched funds", count, err)
	}
}

func TestAutomaticReferralRegistrationAndAuditFailureRollBackTogether(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	store, err := identity.New(db)
	if err != nil {
		t.Fatal(err)
	}
	register := func(name string) (identity.Authentication, error) {
		tx, err := db.Begin(ctx)
		if err != nil {
			return identity.Authentication{}, err
		}
		defer tx.Rollback(ctx)
		out, err := store.Register(ctx, tx, brand, identity.RegisterInput{Username: name, Password: "referral-test-password", Privacy: "dev-1", Terms: "dev-1"}, identity.Metadata{})
		if err != nil {
			return identity.Authentication{}, err
		}
		if out.Status != 201 {
			return identity.Authentication{}, fmt.Errorf("register status %d", out.Status)
		}
		var auth identity.Authentication
		if err = json.Unmarshal(out.Data, &auth); err != nil {
			return auth, err
		}
		return auth, tx.Commit(ctx)
	}
	auth, err := register("automatic_signup")
	if err != nil {
		t.Fatal(err)
	}
	page, err := (attribution.Service{DB: db}).SelfCodes(ctx, brand, auth.Member.ID, 20, 0)
	if err != nil || len(page.Items) != 1 || page.Items[0].Kind != "referral" || !page.Items[0].Usable {
		t.Fatal(page, err)
	}
	if _, err = db.Exec(ctx, `CREATE FUNCTION reject_auto_referral_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='join_code.create' AND NEW.actor_type='system' THEN RAISE EXCEPTION 'test audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_auto_referral_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_auto_referral_audit()`); err != nil {
		t.Fatal(err)
	}
	if _, err = register("automatic_failed_signup"); err == nil {
		t.Fatal("registration accepted missing audit")
	}
	var count int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM global_users WHERE username='automatic_failed_signup'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("failed registration left identity", count, err)
	}
	if err = db.QueryRow(ctx, `SELECT count(*) FROM join_codes`).Scan(&count); err != nil || count != 1 {
		t.Fatal("failed registration left code", count, err)
	}
}
