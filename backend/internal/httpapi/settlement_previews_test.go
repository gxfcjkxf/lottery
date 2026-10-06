package httpapi

import (
	"context"
	"encoding/json"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/betting"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rulebook"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
	"strings"
	"testing"
	"time"
)

func settlementHTTPFixture(t *testing.T) (pointsHTTPFixture, betting.Order) {
	t.Helper()
	f := pointsFixture(t)
	ctx := context.Background()
	rs := rulebook.Store{DB: f.pool}
	s := betting.Service{DB: f.pool}
	root := access.Account{ID: f.root, Type: access.AccountAdmin, BrandIDs: []string{managedBrand}, Roles: []access.Role{{BrandID: managedBrand, Permissions: []access.Permission{{Resource: "game", Action: "write", Scope: access.ScopeBrand}, {Resource: "rule", Action: "write", Scope: access.ScopeBrand}, {Resource: "rule", Action: "validate", Scope: access.ScopeBrand}, {Resource: "rule", Action: "submit", Scope: access.ScopeBrand}, {Resource: "draw", Action: "manual_create", Scope: access.ScopeBrand}}}}}
	reviewerID := ids.New()
	if _, e := f.pool.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'test-only')`, reviewerID, "preview_reviewer_"+reviewerID); e != nil {
		t.Fatal(e)
	}
	reviewer := access.Account{ID: reviewerID, Type: access.AccountAdmin, BrandIDs: []string{managedBrand}, Roles: []access.Role{{BrandID: managedBrand, Permissions: []access.Permission{{Resource: "rule", Action: "review", Scope: access.ScopeBrand}}}}}
	model := rules.Model{Type: "DIGITS_0_9", Length: 3, AllowRepeat: true, Ordered: true}
	position := 3
	one := points.Amount(1)
	ten := points.Amount(10)
	yes := true
	d := rules.Definition{SchemaVersion: 1, Model: model, Selection: rules.SelectionRule{Mode: "numbers"}, UnitPoints: 1, PrizeTiers: []rules.Tier{{Code: "EXACT", Condition: rules.Condition{Op: "equals", Field: "position_match", Value: &position}, Odds: "10", Exclusive: true}}, Rounding: "half_up", RoundingScope: "order", Limits: rules.Limits{MaxCombinations: 100, MaxMultiplier: 1000}}
	var game rulebook.Game
	var play rulebook.Play
	var v rulebook.Version
	var p rulebook.Period
	mutate := func(fn func(pgx.Tx) error) {
		tx, e := f.pool.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer tx.Rollback(ctx)
		if e = fn(tx); e != nil {
			t.Fatal(e)
		}
		if e = tx.Commit(ctx); e != nil {
			t.Fatal(e)
		}
	}
	meta := points.Metadata{ActorType: "admin", ActorID: f.root, RequestID: ids.New()}
	mutate(func(tx pgx.Tx) error {
		var e error
		game, e = rs.CreateGame(ctx, tx, managedBrand, root, "preview_http", "Preview HTTP", model, "UTC", "preview fixture", meta)
		return e
	})
	mutate(func(tx pgx.Tx) error {
		var e error
		play, e = rs.CreatePlay(ctx, tx, managedBrand, root, game.ID, "exact", "Exact", "preview fixture", meta)
		return e
	})
	mutate(func(tx pgx.Tx) error {
		var e error
		v, e = rs.CreateVersion(ctx, tx, managedBrand, root, play.ID, d, "immediate", "preview fixture", meta)
		return e
	})
	selection := rules.Selection{Digits: [][]int{{1}, {2}, {1}}}
	draw := rules.Draw{Digits: []int{1, 2, 1}}
	mutate(func(tx pgx.Tx) error {
		var e error
		v, e = rs.Validate(ctx, tx, managedBrand, root, v.ID, v.Version, []rules.ValidationCase{{Name: "exact", Selection: selection, Draw: draw, Multiplier: 1, ExpectedBetPoints: &one, ExpectedPrizePoints: &ten, ExpectedWon: &yes}}, "validated", meta)
		return e
	})
	mutate(func(tx pgx.Tx) error {
		var e error
		v, e = rs.Submit(ctx, tx, managedBrand, root, v.ID, v.Version, "ready", meta)
		return e
	})
	mutate(func(tx pgx.Tx) error {
		var e error
		v, e = rs.Review(ctx, tx, managedBrand, reviewer, v.ID, v.Version, true, true, "independent", meta)
		return e
	})
	recharge := pointRecharge(t, f, "100", "real preview test funding", "preview-fund-create")
	mustStatus(t, f.call("POST", "/api/v1/admin/recharges/"+recharge.ID+"/confirm", "preview-fund-confirm", f.token, managedBrand, map[string]any{"version": 1, "reason": "verified"}), 200)
	mutate(func(tx pgx.Tx) error {
		now := time.Now().UTC()
		var e error
		p, e = rs.OpenPeriod(ctx, tx, managedBrand, game.ID, "preview-http-001", now.Add(-time.Minute), now.Add(2*time.Second), now.Add(2100*time.Millisecond))
		return e
	})
	input := betting.Input{PeriodID: p.ID, PlayID: play.ID, RuleVersionID: v.ID, Selection: selection, Multiplier: 1, PolicyVersions: &betting.PolicyVersions{Brand: 1, Game: 1}}
	quote := f.call("POST", "/api/v1/bet-previews", "", f.userToken, managedBrand, input)
	mustStatus(t, quote, 200)
	var q struct {
		ActorContext string `json:"actor_context"`
	}
	managedData(t, quote, &q)
	input.ActorContext = q.ActorContext
	placed := f.call("POST", "/api/v1/bet-orders", "preview-real-bet-place", f.userToken, managedBrand, input)
	mustStatus(t, placed, 201)
	var order betting.Order
	managedData(t, placed, &order)
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, e := rs.Tick(ctx); e != nil {
			t.Fatal(e)
		}
		var status string
		f.pool.QueryRow(ctx, `SELECT status FROM periods WHERE id=$1`, p.ID).Scan(&status)
		if status == "waiting_draw" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("draw window did not mature")
		}
		time.Sleep(25 * time.Millisecond)
	}
	var version int64
	var at time.Time
	f.pool.QueryRow(ctx, `SELECT version,draw_at FROM periods WHERE id=$1`, p.ID).Scan(&version, &at)
	mutate(func(tx pgx.Tx) error {
		_, e := rs.ManualDraw(ctx, tx, managedBrand, root, p.ID, version, p.PeriodNo, draw, at, "test genuine result", meta)
		return e
	})
	c, e := s.SettlementContext(ctx, managedBrand, order.ID)
	if e != nil || !c.CanPreview {
		t.Fatal(c, e)
	}
	return f, order
}
func TestSettlementPreviewHTTPImmutableRealCalculationReplayAndScope(t *testing.T) {
	f, o := settlementHTTPFixture(t)
	ctx := context.Background()
	base := "/api/v1/admin/bet-orders/" + o.ID
	contextPath := base + "/settlement-context"
	mustStatus(t, f.call("GET", contextPath, "", f.token, managedBrand, nil), 403)
	for _, key := range []string{"settlement.view.brand", "settlement.preview.brand", "settlement.view.platform"} {
		if _, e := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) SELECT role_id,$2 FROM admin_account_roles WHERE account_id=$1 ON CONFLICT DO NOTHING`, f.root, key); e != nil {
			t.Fatal(e)
		}
	}
	r := f.call("GET", contextPath, "", f.token, managedBrand, nil)
	mustStatus(t, r, 200)
	var c betting.SettlementContext
	managedData(t, r, &c)
	before := pointWallet(t, f)
	in := betting.SettlementPreviewInput{Version: o.Version, PeriodVersion: c.PeriodVersion, DrawResultID: *c.DrawResultID, Reason: "operator saved calculation not payment"}
	path := base + "/settlement-previews"
	r = f.call("POST", path, "preview-create-once", f.token, managedBrand, in)
	mustStatus(t, r, 201)
	var p betting.SettlementPreview
	managedData(t, r, &p)
	if p.Applied || p.Outcome != "won" || p.Calculation == nil || p.Calculation.PrizePoints != 10 {
		t.Fatal(p)
	}
	replay := f.call("POST", path, "preview-create-once", f.token, managedBrand, in)
	mustStatus(t, replay, 201)
	var same betting.SettlementPreview
	managedData(t, replay, &same)
	if same.ID != p.ID || same.AuditLogID != p.AuditLogID {
		t.Fatal(same, p)
	}
	changed := in
	changed.Reason = "changed intent"
	mustStatus(t, f.call("POST", path, "preview-create-once", f.token, managedBrand, changed), 409)
	mustStatus(t, f.call("GET", path+"?limit=1", "", f.token, managedBrand, nil), 200)
	mustStatus(t, f.call("GET", "/api/v1/admin/settlement-previews/"+p.ID+"/lines?limit=20&offset=0", "", f.token, managedBrand, nil), 200)
	mustStatus(t, f.call("GET", contextPath, "", f.userToken, managedBrand, nil), 401)
	mustStatus(t, f.call("GET", contextPath, "", f.token, pointsBrandB, nil), 403)
	raw, _ := json.Marshal(in)
	bad := strings.TrimSuffix(string(raw), "}") + `,"version":1}`
	mustStatus(t, f.rawCall("POST", path, "preview-duplicate-input", f.token, managedBrand, bad), 400)
	mustStatus(t, pointsCall(f.managementHTTP, "POST", path, "preview-csrf", f.token, managedBrand, "https://evil.invalid", in), 403)
	var counts []int64
	if e := f.pool.QueryRow(ctx, `SELECT ARRAY[(SELECT count(*) FROM settlement_previews),(SELECT count(*) FROM audit_logs WHERE action='settlement.preview'),(SELECT count(*) FROM point_ledger_entries)]`).Scan(&counts); e != nil || len(counts) != 3 || counts[0] != 1 || counts[1] != 1 || counts[2] != 2 {
		t.Fatal(counts, e)
	}
	after := pointWallet(t, f)
	if after.Version != before.Version || after.BySource != before.BySource {
		t.Fatal("HTTP preview posted money")
	}
	if _, e := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key='settlement.preview.brand'`, f.root); e != nil {
		t.Fatal(e)
	}
	mustStatus(t, f.call("POST", path, "preview-create-once", f.token, managedBrand, in), 403)
	if _, e := f.pool.Exec(ctx, `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); e != nil {
		t.Fatal(e)
	}
	mustStatus(t, f.call("GET", contextPath, "", f.token, managedBrand, nil), 200)
	mustStatus(t, f.call("POST", path, "preview-create-once", f.token, managedBrand, in), 403)
}
