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
	"github.com/gxfcjkxf/lottery/backend/internal/telegramauth"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
	"testing"
	"time"
)

func TestMultiBrandOperatorAndVerifiedTelegramJoinPaths(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	store, e := identity.New(db)
	if e != nil {
		t.Fatal(e)
	}
	engine, e := mutation.New(db, make([]byte, 32))
	if e != nil {
		t.Fatal(e)
	}
	s := attribution.Service{DB: db}
	other := "0199a000-0000-7000-8000-000000000002"
	admin := ids.New()
	if _, e = db.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash)VALUES($1,$2,'unused-test-hash')`, admin, "path_"+admin); e != nil {
		t.Fatal(e)
	}
	a := access.Account{ID: admin, Type: access.AccountAdmin, BrandIDs: []string{brand, other}, Roles: []access.Role{{BrandID: brand, Permissions: []access.Permission{{Resource: "join_code", Action: "write", Scope: access.ScopeBrand}}}, {BrandID: other, Permissions: []access.Permission{{Resource: "join_code", Action: "write", Scope: access.ScopeBrand}}}}}
	makeSource := func(b string) attribution.Code {
		u, m := ids.New(), ids.New()
		if _, e := db.Exec(ctx, `INSERT INTO global_users(id,username)VALUES($1,$2)`, u, "source_"+u); e != nil {
			t.Fatal(e)
		}
		if _, e := db.Exec(ctx, `INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version)VALUES($1,$2,$3,'domain','dev-1','dev-1')`, m, b, u); e != nil {
			t.Fatal(e)
		}
		tx, e := db.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		out, e := s.Create(ctx, tx, b, a, attribution.CreateInput{Kind: "referral", OwnerMemberID: m, Reason: "path source"}, points.Metadata{RequestID: ids.New()})
		if e == nil {
			e = tx.Commit(ctx)
		} else {
			_ = tx.Rollback(ctx)
		}
		if e != nil {
			t.Fatal(e)
		}
		return out
	}
	codeA, codeB := makeSource(brand), makeSource(other)
	run := func(b string, body any, f func(pgx.Tx) (mutation.Result, error)) mutation.Result {
		raw, _ := json.Marshal(body)
		out, e := engine.Execute(ctx, b, "path-test", "path.join", ids.New(), engine.Fingerprint(string(raw)), func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) { return f(tx) })
		if e != nil {
			t.Fatal(e)
		}
		return out
	}
	in := identity.RegisterInput{Username: "multi_code_user", Password: "path-test-password-2026", Privacy: "dev-1", Terms: "dev-1", ReferralCode: codeA.Code}
	out := run(brand, in, func(tx pgx.Tx) (mutation.Result, error) {
		return store.Register(ctx, tx, brand, in, identity.Metadata{})
	})
	var original identity.Authentication
	if out.Status != 201 || json.Unmarshal(out.Data, &original) != nil {
		t.Fatal(out)
	}
	login := identity.LoginInput{Identifier: in.Username, Password: in.Password, Privacy: "dev-1", Terms: "dev-1", ReferralCode: codeA.Code}
	out = run(other, login, func(tx pgx.Tx) (mutation.Result, error) {
		return store.Login(ctx, tx, other, login, identity.Metadata{})
	})
	if out.Status != 400 || out.Error.Code != "JOIN_CODE_UNAVAILABLE" {
		t.Fatal("foreign code accepted", out)
	}
	login.ReferralCode = codeB.Code
	out = run(other, login, func(tx pgx.Tx) (mutation.Result, error) {
		return store.Login(ctx, tx, other, login, identity.Metadata{})
	})
	var joined identity.Authentication
	if out.Status != 200 || json.Unmarshal(out.Data, &joined) != nil || original.User.ID != joined.User.ID || original.Member.ID == joined.Member.ID {
		t.Fatal(out, original, joined)
	}
	attr, e := s.Attribution(ctx, other, joined.Member.ID)
	if e != nil || attr.CodeID == nil || *attr.CodeID != codeB.ID {
		t.Fatal(attr, e)
	}
	operator := identity.OperatorInput{Username: "operator_code_user", Password: "operator-code-password", Reason: "operator code attribution", ReferralCode: codeA.Code}
	out = run(brand, operator, func(tx pgx.Tx) (mutation.Result, error) {
		return store.OperatorCreate(ctx, tx, brand, admin, operator, identity.Metadata{})
	})
	var op struct {
		MemberID string `json:"member_id"`
	}
	if out.Status != 201 || json.Unmarshal(out.Data, &op) != nil {
		t.Fatal(out)
	}
	attr, e = s.Attribution(ctx, brand, op.MemberID)
	if e != nil || attr.JoinMethod != "operator" || attr.CodeID == nil || *attr.CodeID != codeA.ID {
		t.Fatal(attr, e)
	}
	var accepted bool
	if e = db.QueryRow(ctx, `SELECT terms_accepted FROM brand_members WHERE id=$1`, op.MemberID).Scan(&accepted); e != nil || accepted {
		t.Fatal("operator consent substituted", accepted, e)
	}
	opLogin := identity.LoginInput{Identifier: operator.Username, Password: operator.Password, Privacy: "dev-1", Terms: "dev-1"}
	out = run(brand, opLogin, func(tx pgx.Tx) (mutation.Result, error) {
		return store.Login(ctx, tx, brand, opLogin, identity.Metadata{})
	})
	if out.Status != 200 {
		t.Fatal(out)
	}
	tx, e := db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	out, e = store.UpdateAuthSettings(ctx, tx, brand, admin, identity.AuthSettingsInput{Version: 1, TelegramEnabled: true, TelegramClientID: "123", Reason: "domain verified-claims test only"}, identity.Metadata{})
	if e == nil {
		e = tx.Commit(ctx)
	} else {
		_ = tx.Rollback(ctx)
	}
	if e != nil || out.Status != 200 {
		t.Fatal(out, e)
	}
	tg := identity.TelegramInput{Privacy: "dev-1", Terms: "dev-1", ReferralCode: codeA.Code}
	out = run(brand, tg, func(tx pgx.Tx) (mutation.Result, error) {
		return store.Telegram(ctx, tx, brand, telegramauth.Claims{ID: "1234567891", Name: "Domain claims test"}, tg, "", identity.Metadata{})
	})
	var telegram identity.Authentication
	if out.Status != 200 || json.Unmarshal(out.Data, &telegram) != nil {
		t.Fatal(out)
	}
	attr, e = s.Attribution(ctx, brand, telegram.Member.ID)
	if e != nil || attr.JoinMethod != "referral_code" || attr.CodeID == nil || *attr.CodeID != codeA.ID {
		t.Fatal(attr, e)
	}
	var ledger int
	if e = db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries`).Scan(&ledger); e != nil || ledger != 0 {
		t.Fatal(ledger, e)
	}
}

func TestCodeTimeWindowAndFrozenOwner(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	u, m, admin := ids.New(), ids.New(), ids.New()
	for _, q := range []struct {
		sql  string
		args []any
	}{{`INSERT INTO global_users(id,username)VALUES($1,$2)`, []any{u, "window_" + u}}, {`INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version)VALUES($1,$2,$3,'domain','dev-1','dev-1')`, []any{m, brand, u}}, {`INSERT INTO admin_accounts(id,username,password_hash)VALUES($1,$2,'unused')`, []any{admin, "window_" + admin}}} {
		if _, e := db.Exec(ctx, q.sql, q.args...); e != nil {
			t.Fatal(e)
		}
	}
	a := access.Account{ID: admin, Type: access.AccountAdmin, BrandIDs: []string{brand}, Roles: []access.Role{{BrandID: brand, Permissions: []access.Permission{{Resource: "join_code", Action: "write", Scope: access.ScopeBrand}}}}}
	s := attribution.Service{DB: db}
	start, end := time.Now().UTC().Truncate(time.Microsecond).Add(time.Hour), time.Now().UTC().Truncate(time.Microsecond).Add(2*time.Hour)
	tx, e := db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	code, e := s.Create(ctx, tx, brand, a, attribution.CreateInput{Kind: "referral", OwnerMemberID: m, StartsAt: &start, ExpiresAt: &end, Reason: "future time window"}, points.Metadata{RequestID: ids.New()})
	if e == nil {
		e = tx.Commit(ctx)
	} else {
		_ = tx.Rollback(ctx)
	}
	if e != nil || code.Usable {
		t.Fatal(code, e)
	}
	var begins, ends bool
	if e = db.QueryRow(ctx, `SELECT join_code_usable_at(c,c.starts_at),join_code_usable_at(c,c.expires_at) FROM join_codes c WHERE id=$1`, code.ID).Scan(&begins, &ends); e != nil || !begins || ends {
		t.Fatal(begins, ends, e)
	}
	if _, e = db.Exec(ctx, `UPDATE brand_members SET status='frozen' WHERE id=$1`, m); e != nil {
		t.Fatal(e)
	}
	if e = db.QueryRow(ctx, `SELECT join_code_usable_at(c,c.starts_at) FROM join_codes c WHERE id=$1`, code.ID).Scan(&begins); e != nil || begins {
		t.Fatal("frozen owner usable", begins, e)
	}
}
