package attribution_test

import (
	"context"
	"encoding/json"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/attribution"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
	"strings"
	"testing"
)

func TestFirstJoinCodeSnapshotsAndDisabledSourceReplay(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	users, e := identity.New(db)
	if e != nil {
		t.Fatal(e)
	}
	engine, e := mutation.New(db, make([]byte, 32))
	if e != nil {
		t.Fatal(e)
	}
	execute := func(brand, key string, body any, run func(pgx.Tx) (mutation.Result, error)) mutation.Result {
		b, _ := json.Marshal(body)
		out, e := engine.Execute(ctx, brand, "test-join", "test.join", key, engine.Fingerprint(string(b)), func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) { return run(tx) })
		if e != nil {
			t.Fatal(e)
		}
		return out
	}
	ownerInput := identity.RegisterInput{Username: "join_source_owner", Password: "a-test-only-password-2026", Privacy: "dev-1", Terms: "dev-1"}
	ownerResult := execute(brand, "join-owner-register-001", ownerInput, func(tx pgx.Tx) (mutation.Result, error) {
		return users.Register(ctx, tx, brand, ownerInput, identity.Metadata{Domain: "aurora.localhost"})
	})
	var owner identity.Authentication
	if e = json.Unmarshal(ownerResult.Data, &owner); e != nil || ownerResult.Status != 201 {
		t.Fatal(ownerResult, e)
	}
	admin := ids.New()
	if _, e = db.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash)VALUES($1,$2,'unused-test-hash')`, admin, "join_admin_"+admin); e != nil {
		t.Fatal(e)
	}
	a := access.Account{ID: admin, Type: access.AccountAdmin, BrandIDs: []string{brand}, Roles: []access.Role{{BrandID: brand, Permissions: []access.Permission{{Resource: "join_code", Action: "write", Scope: access.ScopeBrand}}}}}
	s := attribution.Service{DB: db}
	var code attribution.Code
	tx, e := db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	code, e = s.Create(ctx, tx, brand, a, attribution.CreateInput{Kind: "referral", OwnerMemberID: owner.Member.ID, Reason: "create test referral"}, points.Metadata{RequestID: ids.New()})
	if e == nil {
		e = tx.Commit(ctx)
	} else {
		_ = tx.Rollback(ctx)
	}
	if e != nil {
		t.Fatal(e)
	}
	input := identity.RegisterInput{Username: "join_source_child", Password: "a-test-only-password-2026", Privacy: "dev-1", Terms: "dev-1", ReferralCode: strings.ToLower(code.Code)}
	register := func(tx pgx.Tx) (mutation.Result, error) {
		return users.Register(ctx, tx, brand, input, identity.Metadata{Domain: "aurora.localhost"})
	}
	result := execute(brand, "join-child-register-001", input, register)
	var child identity.Authentication
	if e = json.Unmarshal(result.Data, &child); e != nil || result.Status != 201 {
		t.Fatal(result, e)
	}
	attr, e := s.Attribution(ctx, brand, child.Member.ID)
	if e != nil || attr.JoinMethod != "referral_code" || attr.Legacy || attr.CodeID == nil || *attr.CodeID != code.ID || attr.SourceCode == nil || *attr.SourceCode != code.Code {
		t.Fatal(attr, e)
	}
	var before []byte
	if e = db.QueryRow(ctx, `SELECT attribution_snapshot FROM brand_members WHERE id=$1`, child.Member.ID).Scan(&before); e != nil {
		t.Fatal(e)
	}
	tx, e = db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.Update(ctx, tx, brand, a, code.ID, attribution.UpdateInput{Version: 1, Status: "disabled", Reason: "stop future joins"}, points.Metadata{RequestID: ids.New()})
	if e == nil {
		e = tx.Commit(ctx)
	} else {
		_ = tx.Rollback(ctx)
	}
	if e != nil {
		t.Fatal(e)
	}
	replay := execute(brand, "join-child-register-001", input, register)
	if replay.Status != 201 || string(replay.Data) != string(result.Data) {
		t.Fatal("committed join replay changed", replay)
	}
	invalid := input
	invalid.Username = "join_source_denied"
	out := execute(brand, "join-child-denied-001", invalid, func(tx pgx.Tx) (mutation.Result, error) {
		return users.Register(ctx, tx, brand, invalid, identity.Metadata{})
	})
	if out.Status != 400 || out.Error == nil || out.Error.Code != "JOIN_CODE_UNAVAILABLE" {
		t.Fatal(out)
	}
	var exists bool
	if e = db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM global_users WHERE username=$1)`, invalid.Username).Scan(&exists); e != nil || exists {
		t.Fatal("failed join left identity", exists, e)
	}
	login := identity.LoginInput{Identifier: input.Username, Password: input.Password, ReferralCode: code.Code}
	out = execute(brand, "join-child-reassign-001", login, func(tx pgx.Tx) (mutation.Result, error) {
		return users.Login(ctx, tx, brand, login, identity.Metadata{})
	})
	if out.Status != 409 || out.Error.Code != "JOIN_ATTRIBUTION_FIXED" {
		t.Fatal(out)
	}
	login.ReferralCode = ""
	out = execute(brand, "join-child-normal-login-001", login, func(tx pgx.Tx) (mutation.Result, error) {
		return users.Login(ctx, tx, brand, login, identity.Metadata{})
	})
	if out.Status != 200 {
		t.Fatal(out)
	}
	var after []byte
	if e = db.QueryRow(ctx, `SELECT attribution_snapshot FROM brand_members WHERE id=$1`, child.Member.ID).Scan(&after); e != nil || string(before) != string(after) {
		t.Fatal("history rewritten", e)
	}
	for _, q := range []string{`UPDATE brand_members SET attribution_snapshot='{}' WHERE id=$1`, `UPDATE brand_members SET join_method='domain' WHERE id=$1`, `UPDATE brand_members SET joined_at=clock_timestamp() WHERE id=$1`, `DELETE FROM brand_members WHERE id=$1`} {
		if _, e = db.Exec(ctx, q, child.Member.ID); e == nil {
			t.Fatal("member provenance mutation accepted", q)
		}
	}
	var ledger int
	if e = db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries`).Scan(&ledger); e != nil || ledger != 0 {
		t.Fatal("join credited funds", ledger, e)
	}
}
