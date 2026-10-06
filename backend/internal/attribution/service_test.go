package attribution_test

import (
	"context"
	"encoding/json"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/attribution"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
	"testing"
)

const brand = "0199a000-0000-7000-8000-000000000001"

func TestJoinCodeLifecycleAndImmutableAudit(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	s := attribution.Service{DB: db}
	user, member, admin := ids.New(), ids.New(), ids.New()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO global_users(id,username)VALUES($1,$2)`, []any{user, "jc_" + user}},
		{`INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version)VALUES($1,$2,$3,'domain','dev-1','dev-1')`, []any{member, brand, user}},
		{`INSERT INTO admin_accounts(id,username,password_hash)VALUES($1,$2,'unused-test-hash')`, []any{admin, "jc_" + admin}},
	} {
		if _, e := db.Exec(ctx, q.sql, q.args...); e != nil {
			t.Fatal(e)
		}
	}
	a := access.Account{ID: admin, Type: access.AccountAdmin, BrandIDs: []string{brand}, Roles: []access.Role{{BrandID: brand, Permissions: []access.Permission{{Resource: "join_code", Action: "write", Scope: access.ScopeBrand}}}}}
	transact := func(run func(pgx.Tx) error) error {
		tx, e := db.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer tx.Rollback(ctx)
		if e = run(tx); e != nil {
			return e
		}
		return tx.Commit(ctx)
	}
	var code attribution.Code
	e := transact(func(tx pgx.Tx) error {
		var e error
		code, e = s.Create(ctx, tx, brand, a, attribution.CreateInput{Kind: "referral", OwnerMemberID: member, Reason: "isolated referral fixture"}, points.Metadata{ActorType: "admin", ActorID: admin, RequestID: ids.New()})
		return e
	})
	if e != nil {
		t.Fatal(e)
	}
	if len(code.Code) != 24 || code.Version != 1 || !code.Usable || code.AgentID != nil {
		t.Fatal(code)
	}
	old := code
	e = transact(func(tx pgx.Tx) error {
		var e error
		code, e = s.Update(ctx, tx, brand, a, code.ID, attribution.UpdateInput{Version: code.Version, Status: "disabled", Reason: "disable without rewriting attribution"}, points.Metadata{ActorType: "admin", ActorID: admin, RequestID: ids.New()})
		return e
	})
	if e != nil {
		t.Fatal(e)
	}
	if code.Code != old.Code || code.Version != 2 || code.Usable {
		t.Fatal(code)
	}
	h, e := s.History(ctx, brand, code.ID, 20, 0)
	if e != nil || h.TotalCount != "2" || len(h.Items) != 2 || h.Items[0].Version != 2 {
		t.Fatal(h, e)
	}
	for _, q := range []string{`UPDATE join_codes SET code='000000000000000000000000' WHERE id=$1`, `DELETE FROM join_codes WHERE id=$1`, `UPDATE join_code_revisions SET reason='rewritten' WHERE code_id=$1`, `DELETE FROM join_code_revisions WHERE code_id=$1`} {
		if _, e := db.Exec(ctx, q, code.ID); e == nil {
			t.Fatal("forbidden rewrite accepted", q)
		}
	}
	var ledger int
	if e = db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries`).Scan(&ledger); e != nil || ledger != 0 {
		t.Fatal(ledger, e)
	}
}
func TestJoinCodeClosedInputsAndNormalization(t *testing.T) {
	if kind, code, e := attribution.Normalize(" abcdef012345abcdef012345 ", ""); e != nil || kind != "agent" || code != "ABCDEF012345ABCDEF012345" {
		t.Fatal(kind, code, e)
	}
	for _, v := range [][2]string{{"bad", ""}, {"ABCDEF012345ABCDEF012345", "ABCDEF012345ABCDEF012345"}} {
		if _, _, e := attribution.Normalize(v[0], v[1]); e == nil {
			t.Fatal(v)
		}
	}
	for _, raw := range []string{`{"version":1,"status":"active","starts_at":null,"expires_at":null,"reason":"x","version":2}`, `{"version":1,"status":"active","starts_at":null,"expires_at":null}`, `{"version":1,"status":"active","starts_at":null,"expires_at":null,"reason":null}`} {
		var in attribution.UpdateInput
		if e := json.Unmarshal([]byte(raw), &in); e == nil {
			t.Fatal(raw)
		}
	}
}
