package betting

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/platformlottery"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"
)

type configuredFixture struct {
	f     bettingFixture
	s     platformlottery.Service
	a, b  access.Account
	g     platformlottery.Game
	p     platformlottery.Period
	v     platformlottery.ConfiguredVersion
	cases []platformlottery.ConfiguredCase
	input ConfiguredInput
}

func newConfiguredFixture(t *testing.T, disjoint ...bool) configuredFixture {
	t.Helper()
	raw, e := os.ReadFile("../rules/config/examples/lucky_group_preview.json")
	if e != nil {
		t.Fatal(e)
	}
	if len(disjoint) > 0 && disjoint[0] {
		var strict rules.ConfiguredSnapshot
		if json.Unmarshal(raw, &strict) != nil {
			t.Fatal("invalid fixture")
		}
		strict.Catalog.DisjointGroups = true
		raw, _ = json.Marshal(strict)
	}
	cases := []platformlottery.ConfiguredCase{{Requests: []rules.ConfiguredRequest{{PlayID: "operator_new_play", Options: []string{"lucky"}, Plate: "A", Points: 1000, Multiplier: 1}}, Draw: rules.Draw{Regular: []int{1, 3, 7, 4, 5, 6}, Special: []int{2}}, Bet: 1000, Prize: 2500}}
	return configuredFixtureFrom(t, raw, cases)
}
func configuredFixtureFrom(t *testing.T, raw []byte, cases []platformlottery.ConfiguredCase, base ...bettingFixture) configuredFixture {
	t.Helper()
	ctx := context.Background()
	var f bettingFixture
	if len(base) == 1 {
		f = base[0]
	} else {
		f = newBettingFixture(t, storeTestBrand)
	}
	s := platformlottery.Service{DB: f.db}
	a := access.Account{ID: f.adminID, Type: access.AccountAdmin, SuperAdmin: true, Roles: []access.Role{{Permissions: []access.Permission{{Resource: "platform_game", Action: "write", Scope: access.ScopePlatform}}}}}
	b := access.Account{ID: a.ID, Type: access.AccountAdmin, BrandIDs: []string{f.brand}, Roles: []access.Role{{BrandID: f.brand, Permissions: []access.Permission{{Resource: "platform_game", Action: "connect", Scope: access.ScopeBrand}, {Resource: "platform_game", Action: "sell", Scope: access.ScopeBrand}}}}}
	engine, e := rules.RestoreConfiguredEngine(raw)
	if e != nil {
		t.Fatal(e)
	}
	var snapshot rules.ConfiguredSnapshot
	json.Unmarshal(engine.Snapshot(), &snapshot)
	var g platformlottery.Game
	var v platformlottery.ConfiguredVersion
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var e error
		g, e = s.Create(ctx, tx, a, platformlottery.GameInput{Code: "configured", Name: "Configured game", Timezone: "Asia/Singapore", Model: snapshot.Catalog.Model}, points.Metadata{})
		return e
	})

	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var e error
		v, e = s.SaveConfiguredRules(ctx, tx, a, g.ID, platformlottery.ConfiguredRulesInput{Snapshot: engine.Snapshot()}, points.Metadata{})
		return e
	})
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		_, e := s.Connect(ctx, tx, b, f.brand, g.ID, "", points.Metadata{})
		if e != nil {
			return e
		}
		_, e = s.WriteConfiguredSales(ctx, tx, b, f.brand, g.ID, platformlottery.ConfiguredSales{RuleID: v.ID, Enabled: true, Prices: snapshot.Prices, MinPoints: 100, MaxPoints: 10000, MaxPeriodPoints: 1000000, MaxUserPeriodPoints: 1000000}, points.Metadata{})
		return e
	})
	var p platformlottery.Period
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var e error
		p, e = s.CreatePeriod(ctx, tx, a, g.ID, platformlottery.PeriodInput{PeriodNo: "configured-1", BetStartAt: time.Now().Add(-time.Hour), BetEndAt: time.Now().Add(3 * time.Second), DrawAt: time.Now().Add(3 * time.Second)}, points.Metadata{})
		return e
	})
	return configuredFixture{f, s, a, b, g, p, v, cases, ConfiguredInput{PeriodID: p.ID, RuleID: v.ID, SalesVersion: 1, SnapshotHash: engine.SnapshotHash(), Requests: cases[0].Requests}}
}
func configuredPlace(t *testing.T, c configuredFixture, key string, in ConfiguredInput) (mutation.Result, error) {
	t.Helper()
	ctx := context.Background()
	m, e := mutation.New(c.f.db, make([]byte, 32))
	if e != nil {
		return mutation.Result{}, e
	}
	raw, _ := json.Marshal(in)
	return m.ExecuteChecked(ctx, c.f.brand, c.f.member, "configured.place", key, m.Fingerprint(string(raw)), func(ctx context.Context, tx pgx.Tx) error { return eligibility(ctx, tx, c.f.brand, c.f.user, false) }, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
		o, e := c.f.service.PlaceConfigured(ctx, tx, c.f.brand, c.f.user, in, points.Metadata{RequestID: ids.New()})
		if e != nil {
			return mutation.Result{}, e
		}
		if len(o.Items) != 1 {
			t.Fatalf("single-item fixture returned %d orders", len(o.Items))
		}
		return mutation.OK(201, o.Items[0]), nil
	})
}
func configuredPublishDraw(t *testing.T, c configuredFixture, n int) {
	t.Helper()
	time.Sleep(time.Until(c.p.DrawAt.Add(30 * time.Millisecond)))
	bettingTx(t, c.f.db, func(tx pgx.Tx) error {
		_, e := c.s.Publish(context.Background(), tx, c.a, c.p.ID, rules.Draw{Regular: []int{1, 3, n, 4, 5, 6}, Special: []int{2}}, "", points.Metadata{})
		return e
	})
}
func TestConfiguredLiveWorkflowFrozenSnapshotAndIsolation(t *testing.T) {
	c := newConfiguredFixture(t)
	f := c.f
	ctx := context.Background()
	fundBettingWallet(t, f, 10000)
	first, e := configuredPlace(t, c, "configured-repeat", c.input)
	if e != nil {
		t.Fatal(e)
	}
	replay, e := configuredPlace(t, c, "configured-repeat", c.input)
	if e != nil || string(first.Data) != string(replay.Data) {
		t.Fatal("idempotent replay", e)
	}
	altered := c.input
	altered.Requests = append([]rules.ConfiguredRequest{}, c.input.Requests...)
	altered.Requests[0].Points = 1100
	mismatch, e := configuredPlace(t, c, "configured-repeat", altered)
	if e == nil && mismatch.Status != 409 {
		t.Fatal("same key changed body", mismatch)
	}
	if _, e = f.service.ConfiguredCatalog(ctx, storeTestOtherBrand, c.g.ID); !errors.Is(e, platformlottery.ErrNotFound) {
		t.Fatal("foreign catalogue", e)
	}
	foreign, e := f.service.ConfiguredOrders(ctx, storeTestOtherBrand, f.member, c.g.ID, 0)
	if e != nil || len(foreign) != 0 {
		t.Fatal("foreign orders", e)
	}
	tx, _ := f.db.Begin(ctx)
	_, e = c.s.SaveConfiguredRules(ctx, tx, c.b, c.g.ID, platformlottery.ConfiguredRulesInput{ID: c.v.ID, Revision: c.v.Revision, Snapshot: c.v.Snapshot}, points.Metadata{})
	tx.Rollback(ctx)
	if !errors.Is(e, platformlottery.ErrDenied) {
		t.Fatal("brand rewrote platform rule", e)
	}
	sale, _ := platformlottery.ReadConfiguredSales(ctx, f.db, f.brand, c.g.ID, false)
	sale.Prices.Plates["A"]["lucky_price"] = "2.6"
	tx, _ = f.db.Begin(ctx)
	_, e = c.s.WriteConfiguredSales(ctx, tx, c.b, f.brand, c.g.ID, sale, points.Metadata{})
	tx.Rollback(ctx)
	if !errors.Is(e, platformlottery.ErrInvalid) {
		t.Fatal("price ceiling", e)
	}

	var snap rules.ConfiguredSnapshot
	json.Unmarshal(c.v.Snapshot, &snap)
	snap.Prices.Plates["A"]["lucky_price"] = "2"
	snap.Catalog.Domains["lucky"].Options[0].Mapping.Numbers = []int{8}
	raw, _ := json.Marshal(snap)
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		_, e := c.s.SaveConfiguredRules(ctx, tx, c.a, c.g.ID, platformlottery.ConfiguredRulesInput{ID: c.v.ID, Revision: c.v.Revision, Snapshot: raw}, points.Metadata{})
		return e
	})
	if _, e = configuredPlace(t, c, "configured-stale", c.input); !errors.Is(e, ErrVersion) {
		t.Fatal("stale admission", e)
	}
	var placed ConfiguredOrder
	json.Unmarshal(first.Data, &placed)
	tx, _ = f.db.Begin(ctx)
	_, e = tx.Exec(ctx, `UPDATE configured_orders SET snapshot='{}' WHERE id=$1`, placed.ID)
	tx.Rollback(ctx)
	if e == nil {
		t.Fatal("snapshot mutation accepted")
	}
	configuredPublishDraw(t, c, 7)
	// A published draw does not make an unprocessed purchase deletable.
	detail, e := c.s.Detail(ctx, f.brand, c.g.ID, 50, 0)
	if e != nil || detail.Game.Deletable {
		t.Fatal("pending settlement advertised as deletable", e)
	}
	tx, _ = f.db.Begin(ctx)
	_, e = c.s.SetLifecycle(ctx, tx, c.a, c.g.ID, platformlottery.LifecycleInput{ExpectedVersion: c.g.Version, Status: "deleted"}, points.Metadata{})
	tx.Rollback(ctx)
	if !errors.Is(e, platformlottery.ErrGameInUse) {
		t.Fatal("pending settlement deleted", e)
	}
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := f.service.ProcessConfiguredSettlements(ctx, 10); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	rows, e := f.service.ConfiguredOrders(ctx, f.brand, f.member, c.g.ID, 0)
	if e != nil || len(rows) != 1 || rows[0].Result == nil || rows[0].Result.PrizePoints != 2500 {
		t.Fatal("frozen payout", rows, e)
	}
	var audited bool
	if e := f.db.QueryRow(ctx, `SELECT before_json->>'status'='placed' AND after_json->>'status'='settled' AND (after_json->'result'->>'prize_points')::bigint=25 FROM audit_logs WHERE action='configured.settle' AND resource_id=$1`, placed.ID).Scan(&audited); e != nil || !audited {
		t.Fatal("settlement audit snapshots", audited, e)
	}
	if n, e := f.service.ProcessConfiguredSettlements(ctx, 10); e != nil || n != 0 {
		t.Fatal("repeated payout", n, e)
	}
	wallet, e := (points.Store{DB: f.db}).Read(ctx, f.brand, f.member)
	if e != nil || wallet.AvailablePoints != 11500 {
		t.Fatal("wallet", wallet, e)
	}
	var invalid int
	if e = f.db.QueryRow(ctx, `SELECT count(*) FROM point_configured_business_witnesses($1,$2) WHERE NOT source_valid`, f.brand, wallet.AccountID).Scan(&invalid); e != nil || invalid != 0 {
		t.Fatal("financial witnesses", invalid, e)
	}
	var consistent bool
	if e = f.db.QueryRow(ctx, `SELECT coalesce(bool_and(source_valid),false) FROM point_configured_business_witnesses($1,$2)`, f.brand, wallet.AccountID).Scan(&consistent); e != nil || !consistent {
		t.Fatal("configured money not reconciled", e)
	}
	detail, e = c.s.Detail(ctx, f.brand, c.g.ID, 50, 0)
	if e != nil || !detail.Game.Deletable {
		t.Fatal("settled history blocks deletion", e)
	}
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		_, e := c.s.SetLifecycle(ctx, tx, c.a, c.g.ID, platformlottery.LifecycleInput{ExpectedVersion: c.g.Version, Status: "deleted"}, points.Metadata{})
		return e
	})
	games, e := c.s.Games(ctx, "", 50, 0)
	if e != nil {
		t.Fatal(e)
	}
	for _, game := range games {
		if game.ID == c.g.ID {
			t.Fatal("deleted game still listed")
		}
	}
	history, e := c.s.Detail(ctx, f.brand, c.g.ID, 50, 0)
	if e != nil || history.Game.Status != "deleted" || len(history.Periods) != 1 {
		t.Fatal("deleted game history", e)
	}
	rows, e = f.service.ConfiguredOrders(ctx, f.brand, f.member, c.g.ID, 0)
	if e != nil || len(rows) != 1 || rows[0].Result.PrizePoints != 2500 {
		t.Fatal("settled order history", rows, e)
	}
	if _, e = c.s.Detail(ctx, storeTestOtherBrand, c.g.ID, 50, 0); !errors.Is(e, platformlottery.ErrNotFound) {
		t.Fatal("foreign deleted history", e)
	}
	var accepts bool
	if e = f.db.QueryRow(ctx, `SELECT shared_game_accepts($1,$2)`, f.brand, c.g.ID).Scan(&accepts); e != nil || accepts {
		t.Fatal("deleted game admits bets", e)
	}
	after, e := (points.Store{DB: f.db}).Read(ctx, f.brand, f.member)
	if e != nil || after.AvailablePoints != wallet.AvailablePoints || after.Version != wallet.Version {
		t.Fatal("deletion changed funds", after, e)
	}
	if e = f.db.QueryRow(ctx, `SELECT coalesce(bool_and(source_valid),false) FROM point_configured_business_witnesses($1,$2)`, f.brand, wallet.AccountID).Scan(&consistent); e != nil || !consistent {
		t.Fatal("deletion damaged reconciliation", e)
	}
}

func TestConfiguredRuleSaveIsBoundToCurrentRevision(t *testing.T) {
	c := newConfiguredFixture(t)
	ctx := context.Background()
	bettingTx(t, c.f.db, func(tx pgx.Tx) error {
		_, e := c.s.SaveConfiguredRules(ctx, tx, c.a, c.g.ID, platformlottery.ConfiguredRulesInput{ID: c.v.ID, Revision: c.v.Revision, Snapshot: c.v.Snapshot}, points.Metadata{})
		return e
	})
	var audited bool
	if e := c.f.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM audit_logs WHERE action='configured_rule.save' AND before_json->>'id'=$1 AND after_json->>'id'<>$1 AND before_json->'snapshot'=after_json->'snapshot')`, c.v.ID).Scan(&audited); e != nil || !audited {
		t.Fatal("rule replacement audit snapshots", audited, e)
	}
	tx, _ := c.f.db.Begin(ctx)
	defer tx.Rollback(ctx)
	_, e := c.s.SaveConfiguredRules(ctx, tx, c.a, c.g.ID, platformlottery.ConfiguredRulesInput{ID: c.v.ID, Revision: c.v.Revision, Snapshot: c.v.Snapshot}, points.Metadata{})
	if !errors.Is(e, platformlottery.ErrVersionConflict) {
		t.Fatal("stale rule overwritten", e)
	}
}

func TestConfiguredDrawPublicationChecksFrozenSemantics(t *testing.T) {
	c := newConfiguredFixture(t, true)
	fundBettingWallet(t, c.f, 10000)
	if _, e := configuredPlace(t, c, "configured-strict-draw", c.input); e != nil {
		t.Fatal(e)
	}
	time.Sleep(time.Until(c.p.DrawAt.Add(30 * time.Millisecond)))
	ctx := context.Background()
	tx, _ := c.f.db.Begin(ctx)
	_, e := c.s.Publish(ctx, tx, c.a, c.p.ID, rules.Draw{Regular: []int{1, 3, 7, 4, 5, 6}, Special: []int{1}}, "", points.Metadata{})
	tx.Rollback(ctx)
	if !errors.Is(e, platformlottery.ErrInvalid) {
		t.Fatal("published draw invalid for purchased snapshot", e)
	}
	var unpublished bool
	if e = c.f.db.QueryRow(ctx, `SELECT result IS NULL FROM platform_game_periods WHERE id=$1`, c.p.ID).Scan(&unpublished); e != nil || !unpublished {
		t.Fatal("invalid draw changed period", e)
	}
	configuredPublishDraw(t, c, 7)
	if _, e = c.f.service.ProcessConfiguredSettlements(ctx, 10); e != nil {
		t.Fatal(e)
	}
}
func TestConfiguredConcurrentAdmissionsDoNotOverdraw(t *testing.T) {
	c := newConfiguredFixture(t)
	fundBettingWallet(t, c.f, 10000)
	in := c.input
	in.Requests[0].Points = 3000
	var wg sync.WaitGroup
	var mu sync.Mutex
	success := 0
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := configuredPlace(t, c, ids.New(), in)
			if e == nil && r.Status == 201 {
				mu.Lock()
				success++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if success != 3 {
		t.Fatal("admission count", success)
	}
	w, e := (points.Store{DB: c.f.db}).Read(context.Background(), c.f.brand, c.f.member)
	if e != nil || w.AvailablePoints != 1000 {
		t.Fatal("overdraw", w, e)
	}
}
func TestConfiguredPushReturnsOriginalSourcesOnce(t *testing.T) {
	c := newConfiguredFixture(t)
	f := c.f
	fundBettingWallet(t, f, 1500, 500, 10000)
	c.input.Requests[0].Points = 3000
	if _, e := configuredPlace(t, c, "configured-push", c.input); e != nil {
		t.Fatal(e)
	}
	configuredPublishDraw(t, c, 49)
	if _, e := f.service.ProcessConfiguredSettlements(context.Background(), 10); e != nil {
		t.Fatal(e)
	}
	if n, e := f.service.ProcessConfiguredSettlements(context.Background(), 10); e != nil || n != 0 {
		t.Fatal(n, e)
	}
	w, e := (points.Store{DB: f.db}).Read(context.Background(), f.brand, f.member)
	if e != nil || w.BySource[0][0] != 1500 || w.BySource[1][0] != 500 || w.BySource[2][0] != 10000 {
		t.Fatal("refund allocation", w, e)
	}
}

func TestConfiguredDrawSourcesSelectionAndFrozenPeriods(t *testing.T) {
	c := newConfiguredFixture(t)
	ctx := context.Background()
	anchor := time.Now().Add(time.Hour)
	number := "900001"
	interval := 120
	plan := platformlottery.AutomationPlan{SourceType: "mapped", Mapping: &platformlottery.FeedMapping{Format: "json", Schedule: "interval", TimeFormat: "rfc3339", Period: platformlottery.FeedField{Path: "/period_no"}, DrawTime: platformlottery.FeedField{Path: "/drawn_at"}, Regular: &platformlottery.FeedNumbers{FeedField: platformlottery.FeedField{Path: "/result/regular"}, Count: 6}, Special: &platformlottery.FeedNumbers{FeedField: platformlottery.FeedField{Path: "/result/special"}, Count: 1}}, Enabled: true, AnchorDrawAt: &anchor, AnchorPeriodNo: &number, IntervalSeconds: &interval, OpenBeforeSeconds: &interval, CloseBeforeSeconds: 15, SourceURL: "https://first.example/result"}
	var first, second platformlottery.DrawSource
	save := func(name string, p platformlottery.AutomationPlan) *platformlottery.DrawSource {
		var out platformlottery.DrawSource
		bettingTx(t, c.f.db, func(tx pgx.Tx) error {
			var e error
			out, e = c.s.SaveDrawSource(ctx, tx, c.a, c.g.ID, platformlottery.DrawSourceInput{DrawSource: platformlottery.DrawSource{Name: name, Plan: p}}, points.Metadata{})
			return e
		})
		return &out
	}
	first = *save("First source", plan)
	plan.SourceURL = "https://second.example/result"
	later := anchor.Add(time.Hour)
	plan.AnchorDrawAt = &later
	number2 := "910001"
	plan.AnchorPeriodNo = &number2
	second = *save("Second source", plan)
	list, e := c.s.DrawSources(ctx, c.g.ID)
	if e != nil || len(list.Items) != 2 || list.Version != 0 || list.ActiveSourceID != "" {
		t.Fatal("saving activated a source", list, e)
	}
	var active platformlottery.AutomationPlan
	bettingTx(t, c.f.db, func(tx pgx.Tx) error {
		var e error
		active, e = c.s.ActivateDrawSource(ctx, tx, c.a, c.g.ID, first.ID, platformlottery.DrawSourceActivation{Revision: 1, Version: 0}, points.Metadata{})
		return e
	})
	var reserved platformlottery.Period
	bettingTx(t, c.f.db, func(tx pgx.Tx) error {
		var e error
		reserved, e = c.s.CreatePeriod(ctx, tx, c.a, c.g.ID, platformlottery.PeriodInput{PeriodNo: "900001", BetStartAt: anchor.Add(-time.Minute), BetEndAt: anchor.Add(-15 * time.Second), DrawAt: anchor, AutomationVersion: &active.Version}, points.Metadata{})
		return e
	})
	bettingTx(t, c.f.db, func(tx pgx.Tx) error {
		_, e := c.s.ActivateDrawSource(ctx, tx, c.a, c.g.ID, second.ID, platformlottery.DrawSourceActivation{Revision: 1, Version: 1}, points.Metadata{})
		return e
	})
	list, e = c.s.DrawSources(ctx, c.g.ID)
	if e != nil || list.ActiveSourceID != second.ID || list.ActiveSourceRevision != 1 || list.Version != 2 {
		t.Fatal("explicit selection", list, e)
	}
	var frozen string
	if e = c.f.db.QueryRow(ctx, `SELECT v.source_url FROM platform_game_periods p JOIN platform_automation_versions v ON v.game_id=p.game_id AND v.version=p.automation_version WHERE p.id=$1`, reserved.ID).Scan(&frozen); e != nil || frozen != first.Plan.SourceURL {
		t.Fatal("reserved source changed", frozen, e)
	}
	tx, _ := c.f.db.Begin(ctx)
	_, e = c.s.ActivateDrawSource(ctx, tx, c.a, c.g.ID, first.ID, platformlottery.DrawSourceActivation{Revision: 1, Version: 0}, points.Metadata{})
	tx.Rollback(ctx)
	if !errors.Is(e, platformlottery.ErrVersionConflict) {
		t.Fatal("stale activation", e)
	}
	tx, _ = c.f.db.Begin(ctx)
	_, e = c.s.SaveDrawSource(ctx, tx, c.b, c.g.ID, platformlottery.DrawSourceInput{DrawSource: first}, points.Metadata{})
	tx.Rollback(ctx)
	if !errors.Is(e, platformlottery.ErrDenied) {
		t.Fatal("brand edited source", e)
	}
	tx, _ = c.f.db.Begin(ctx)
	_, e = c.s.ActivateDrawSource(ctx, tx, c.b, c.g.ID, first.ID, platformlottery.DrawSourceActivation{Revision: 1, Version: 2}, points.Metadata{})
	tx.Rollback(ctx)
	if !errors.Is(e, platformlottery.ErrDenied) {
		t.Fatal("brand activated source", e)
	}
	bettingTx(t, c.f.db, func(tx pgx.Tx) error {
		first.Plan.SourceURL = "https://changed.example/result"
		_, e := c.s.SaveDrawSource(ctx, tx, c.a, c.g.ID, platformlottery.DrawSourceInput{DrawSource: first}, points.Metadata{})
		return e
	})
	list, e = c.s.DrawSources(ctx, c.g.ID)
	if e != nil || list.ActiveSourceID != second.ID || list.Version != 2 {
		t.Fatal("editing selected another source", e)
	}
}

func TestConfiguredDrawSourceAndManualResults(t *testing.T) {
	c := newConfiguredFixture(t)
	ctx := context.Background()
	anchor := time.Now().Add(time.Hour)
	number := "920001"
	interval := 120
	plan := platformlottery.AutomationPlan{SourceType: "mapped", Mapping: &platformlottery.FeedMapping{Format: "json", Schedule: "interval", TimeFormat: "rfc3339", Period: platformlottery.FeedField{Path: "/period_no"}, DrawTime: platformlottery.FeedField{Path: "/drawn_at"}, Regular: &platformlottery.FeedNumbers{FeedField: platformlottery.FeedField{Path: "/result/regular"}, Count: 6}, Special: &platformlottery.FeedNumbers{FeedField: platformlottery.FeedField{Path: "/result/special"}, Count: 1}}, Enabled: true, AnchorDrawAt: &anchor, AnchorPeriodNo: &number, IntervalSeconds: &interval, OpenBeforeSeconds: &interval, CloseBeforeSeconds: 15, SourceURL: "https://manual.example/result"}
	var source platformlottery.DrawSource
	bettingTx(t, c.f.db, func(tx pgx.Tx) error {
		var err error
		source, err = c.s.SaveDrawSource(ctx, tx, c.a, c.g.ID, platformlottery.DrawSourceInput{DrawSource: platformlottery.DrawSource{Name: "Manual period source", Plan: plan}}, points.Metadata{})
		return err
	})
	bettingTx(t, c.f.db, func(tx pgx.Tx) error {
		_, err := c.s.ActivateDrawSource(ctx, tx, c.a, c.g.ID, source.ID, platformlottery.DrawSourceActivation{Revision: 1, Version: 0}, points.Metadata{})
		return err
	})
	list, err := c.s.DrawSources(ctx, c.g.ID)
	if err != nil || len(list.Items) != 1 || list.ActiveSourceID != source.ID || list.Version != 1 {
		t.Fatal("current source selection", list, err)
	}
	// A manual period uses configured bindings, independently of the selected feed type.
	bettingTx(t, c.f.db, func(tx pgx.Tx) error {
		p, e := c.s.CreatePeriod(ctx, tx, c.a, c.g.ID, platformlottery.PeriodInput{PeriodNo: "manual-with-source", BetStartAt: time.Now().Add(-3 * time.Minute), BetEndAt: time.Now().Add(-2 * time.Minute), DrawAt: time.Now().Add(-time.Minute)}, points.Metadata{})
		if e != nil {
			return e
		}
		_, e = c.s.Publish(ctx, tx, c.a, p.ID, rules.Draw{Regular: []int{1, 3, 7, 4, 5, 6}, Special: []int{2}}, "", points.Metadata{})
		return e
	})
}

func TestConfiguredMappedJSONAndDOMCollection(t *testing.T) {
	for _, format := range []string{"json", "html"} {
		t.Run(format, func(t *testing.T) {
			c := newConfiguredFixture(t)
			ctx := context.Background()
			drawAt := time.Now().Add(-time.Minute).UTC().Truncate(time.Second)
			nextAt := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
			body := ""
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
			defer server.Close()
			m := &platformlottery.FeedMapping{Format: format, Schedule: "interval", TimeFormat: "rfc3339", Period: platformlottery.FeedField{Path: "/issue"}, DrawTime: platformlottery.FeedField{Path: "/time"}, Regular: &platformlottery.FeedNumbers{FeedField: platformlottery.FeedField{Path: "/balls"}, Count: 6}, Special: &platformlottery.FeedNumbers{FeedField: platformlottery.FeedField{Path: "/balls"}, Offset: 6, Count: 1}}
			anchor := nextAt
			number := "930001"
			interval := 120
			p := platformlottery.AutomationPlan{SourceType: "mapped", Mapping: m, Enabled: true, AnchorDrawAt: &anchor, AnchorPeriodNo: &number, IntervalSeconds: &interval, OpenBeforeSeconds: &interval, SourceURL: server.URL + "/unrelated-feed?lang=en"}
			if format == "html" {
				m.Schedule = "source"
				p.AnchorDrawAt = nil
				p.AnchorPeriodNo = nil
				p.IntervalSeconds = nil
				m.Period.Path = "#issue"
				m.DrawTime = platformlottery.FeedField{Path: "time.result", Attribute: "datetime"}
				m.Regular.Path = ".ball"
				m.Special.Path = ".ball"
				m.NextPeriod = &platformlottery.FeedField{Path: "#next"}
				m.NextDrawTime = &platformlottery.FeedField{Path: "time.next", Attribute: "datetime"}
			}
			var source platformlottery.DrawSource
			bettingTx(t, c.f.db, func(tx pgx.Tx) error {
				var e error
				source, e = c.s.SaveDrawSource(ctx, tx, c.a, c.g.ID, platformlottery.DrawSourceInput{DrawSource: platformlottery.DrawSource{Name: "Operator-defined " + format, Plan: p}}, points.Metadata{})
				return e
			})
			bettingTx(t, c.f.db, func(tx pgx.Tx) error {
				var e error
				p, e = c.s.ActivateDrawSource(ctx, tx, c.a, c.g.ID, source.ID, platformlottery.DrawSourceActivation{Revision: 1}, points.Metadata{})
				return e
			})
			var period platformlottery.Period
			bettingTx(t, c.f.db, func(tx pgx.Tx) error {
				var e error
				period, e = c.s.CreatePeriod(ctx, tx, c.a, c.g.ID, platformlottery.PeriodInput{PeriodNo: "mapped-result-1", BetStartAt: drawAt.Add(-2 * time.Minute), BetEndAt: drawAt.Add(-time.Minute), DrawAt: drawAt, AutomationVersion: &p.Version}, points.Metadata{})
				return e
			})
			if format == "json" {
				raw, _ := json.Marshal(map[string]any{"issue": period.PeriodNo, "time": drawAt.Format(time.RFC3339), "balls": []int{1, 3, 7, 4, 5, 6, 2}})
				body = string(raw)
			} else {
				body = `<b id="issue">` + period.PeriodNo + `</b><time class="result" datetime="` + drawAt.Format(time.RFC3339) + `"></time><b id="next">mapped-next-2</b><time class="next" datetime="` + nextAt.Format(time.RFC3339) + `"></time>`
				for _, n := range []string{"1", "3", "7", "4", "5", "6", "2"} {
					body += `<span class="ball">` + n + `</span>`
				}
			}
			// Updating the saved mapping does not alter the published period's reader.
			bettingTx(t, c.f.db, func(tx pgx.Tx) error {
				source.Plan.SourceURL = server.URL + "/edited"
				source.Plan.Mapping.Period.Path = "/wrong"
				if format == "html" {
					source.Plan.Mapping.Period.Path = "#wrong"
				}
				_, e := c.s.SaveDrawSource(ctx, tx, c.a, c.g.ID, platformlottery.DrawSourceInput{DrawSource: source}, points.Metadata{})
				return e
			})
			count, e := c.s.CollectAutomaticDraws(ctx, platformlottery.AutomationSources{Origins: []string{server.URL}, Client: server.Client()})
			if e != nil || count != 1 {
				t.Fatal("mapped collection", count, e)
			}
			var result rules.Draw
			var raw []byte
			if e = c.f.db.QueryRow(ctx, `SELECT result FROM platform_game_periods WHERE id=$1`, period.ID).Scan(&raw); e != nil {
				t.Fatal(e)
			}
			if e = json.Unmarshal(raw, &result); e != nil || len(result.Regular) != 6 || result.Special[0] != 2 {
				t.Fatal(result, e)
			}
			if format == "html" {
				var exists bool
				if e = c.f.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM platform_game_periods WHERE game_id=$1 AND period_no='mapped-next-2' AND automation_version=$2)`, c.g.ID, p.Version).Scan(&exists); e != nil || !exists {
					t.Fatal("mapped source reservation", e)
				}
			}
			count, e = c.s.CollectAutomaticDraws(ctx, platformlottery.AutomationSources{Origins: []string{server.URL}, Client: server.Client()})
			if e != nil || count != 0 {
				t.Fatal("duplicate collection", count, e)
			}
		})
	}
}

func TestConfiguredCatalogueRequiresCurrentBrandSales(t *testing.T) {
	c := newConfiguredFixture(t)
	ctx := context.Background()
	check := func(want int, status string) {
		t.Helper()
		games, e := c.f.service.ConfiguredGames(ctx, c.f.brand)
		if e != nil {
			t.Fatal(e)
		}
		if len(games) != want {
			t.Fatalf("catalogue size %d, want %d", len(games), want)
		}
		rows, e := c.s.Games(ctx, c.f.brand, 50, 0)
		if e != nil {
			t.Fatal(e)
		}
		found := false
		for _, g := range rows {
			if g.ID == c.g.ID {
				found = true
				if !g.RulePublished || g.SalesStatus != status {
					t.Fatalf("readiness %+v", g)
				}
			}
		}
		if !found {
			t.Fatal("game missing")
		}
	}
	check(1, "ready")
	if _, e := c.f.db.Exec(ctx, `UPDATE configured_sales SET enabled=false WHERE brand_id=$1 AND game_id=$2`, c.f.brand, c.g.ID); e != nil {
		t.Fatal(e)
	}
	check(0, "disabled")
	other, e := c.f.service.ConfiguredGames(ctx, "0199a000-0000-7000-8000-000000000002")
	if e != nil {
		t.Fatal(e)
	}
	if len(other) != 0 {
		t.Fatal("foreign brand catalogue leaked")
	}
	if _, e := c.f.db.Exec(ctx, `DELETE FROM configured_sales WHERE brand_id=$1 AND game_id=$2`, c.f.brand, c.g.ID); e != nil {
		t.Fatal(e)
	}
	check(0, "unconfigured")
}

func TestConfiguredCatalogueShowsImportedPreviousDrawWithoutSaleableHistory(t *testing.T) {
	c := newConfiguredFixture(t)
	ctx := context.Background()
	if _, err := c.f.db.Exec(ctx, `INSERT INTO platform_draw_history(game_id,period_no,draw_year,draw_date,result,source,regular_order,attributes) VALUES($1,'1993001',1993,'1993-01-05','{"regular":[1,2,3,4,5,6],"special":[7],"digits":[]}','hkjc','[6,5,4,3,2,1]','{"01":{"wave":"blue","zodiac":"rat","element":"water"}}')`, c.g.ID); err != nil {
		t.Fatal(err)
	}
	value, err := c.f.service.ConfiguredCatalog(ctx, c.f.brand, c.g.ID)
	if err != nil {
		t.Fatal(err)
	}
	data := value.(map[string]any)
	previous, ok := data["previous_draw"].(map[string]any)
	if !ok || previous["period_no"] != "1993001" {
		t.Fatalf("imported previous result missing: %+v", data["previous_draw"])
	}
	if got := previous["result"].(rules.Draw); len(got.Special) != 1 || got.Special[0] != 7 {
		t.Fatalf("wrong result: %+v", got)
	}
	if got := previous["attributes"].(map[string]ArchiveNumberAttribute)["01"].Wave; got != "blue" {
		t.Fatalf("saved wave replaced by current rule: %s", got)
	}
	var count int
	if err = c.f.db.QueryRow(ctx, `SELECT count(*) FROM platform_game_periods WHERE game_id=$1 AND result IS NOT NULL`, c.g.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("history turned into an operational period: %d %v", count, err)
	}
}

func TestConfiguredOpeningScheduleRejectsBeforeOpenWithoutDebit(t *testing.T) {
	c := newConfiguredFixture(t)
	ctx := context.Background()
	opening := 60
	draw := time.Now().Add(2 * time.Minute).UTC().Truncate(time.Second)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"status": "pending", "next": "opening-check", "next_at": draw.Format(time.RFC3339)})
	}))
	defer server.Close()
	mapping := &platformlottery.FeedMapping{Format: "json", Schedule: "source", TimeFormat: "rfc3339", Period: platformlottery.FeedField{Path: "/period"}, DrawTime: platformlottery.FeedField{Path: "/at"}, Regular: &platformlottery.FeedNumbers{FeedField: platformlottery.FeedField{Path: "/regular"}, Count: 6}, Special: &platformlottery.FeedNumbers{FeedField: platformlottery.FeedField{Path: "/special"}, Count: 1}, Complete: &platformlottery.FeedCondition{FeedField: platformlottery.FeedField{Path: "/status"}, Equals: "done"}, NextPeriod: &platformlottery.FeedField{Path: "/next"}, NextDrawTime: &platformlottery.FeedField{Path: "/next_at"}}
	bettingTx(t, c.f.db, func(tx pgx.Tx) error {
		source, e := c.s.SaveDrawSource(ctx, tx, c.a, c.g.ID, platformlottery.DrawSourceInput{DrawSource: platformlottery.DrawSource{Name: "Opening schedule", Plan: platformlottery.AutomationPlan{SourceType: "mapped", Mapping: mapping, Enabled: true, OpenBeforeSeconds: &opening, CloseBeforeSeconds: 15, SourceURL: server.URL}}}, points.Metadata{})
		if e != nil {
			return e
		}
		_, e = c.s.ActivateDrawSource(ctx, tx, c.a, c.g.ID, source.ID, platformlottery.DrawSourceActivation{Revision: source.Revision}, points.Metadata{})
		return e
	})
	if _, e := c.s.SyncMappedSources(ctx, platformlottery.AutomationSources{Origins: []string{server.URL}, Client: server.Client()}); e != nil {
		t.Fatal(e)
	}
	var start, end, at time.Time
	if e := c.f.db.QueryRow(ctx, `SELECT id::text,bet_start_at,bet_end_at,draw_at FROM platform_game_periods WHERE game_id=$1 AND period_no='opening-check'`, c.g.ID).Scan(&c.input.PeriodID, &start, &end, &at); e != nil {
		t.Fatal(e)
	}
	if !start.Equal(draw.Add(-60*time.Second)) || !end.Equal(draw.Add(-15*time.Second)) || !at.Equal(draw) {
		t.Fatal("configured opening/closing/draw times", start, end, at)
	}
	originalStart := start
	start = start.Add(15 * time.Second)
	bettingTx(t, c.f.db, func(tx pgx.Tx) error {
		changed, err := c.s.UpdatePeriodOpening(ctx, tx, c.a, c.input.PeriodID, platformlottery.PeriodOpeningInput{BetStartAt: start, ExpectedBetStartAt: originalStart}, points.Metadata{})
		if err == nil && !changed.BetStartAt.Equal(start) {
			t.Fatal("automatic period opening did not change")
		}
		return err
	})
	fundBettingWallet(t, c.f, 10000)
	before, e := (points.Store{DB: c.f.db}).Read(ctx, c.f.brand, c.f.member)
	if e != nil {
		t.Fatal(e)
	}
	tx, e := c.f.db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	_, e = c.f.service.PreviewConfigured(ctx, tx, c.f.brand, c.f.user, c.input, points.Metadata{})
	tx.Rollback(ctx)
	if !errors.Is(e, ErrClosed) {
		t.Fatal("pre-opening quote accepted", e)
	}
	if _, e = configuredPlace(t, c, "before-opening", c.input); !errors.Is(e, ErrClosed) {
		t.Fatal("pre-opening order accepted", e)
	}
	after, e := (points.Store{DB: c.f.db}).Read(ctx, c.f.brand, c.f.member)
	if e != nil || after.AvailablePoints != before.AvailablePoints || after.Version != before.Version {
		t.Fatal("pre-opening request changed wallet", after, e)
	}
	var orders int
	if e = c.f.db.QueryRow(ctx, `SELECT count(*) FROM configured_orders WHERE period_id=$1`, c.input.PeriodID).Scan(&orders); e != nil || orders != 0 {
		t.Fatal("pre-opening order persisted", orders, e)
	}
}

func TestConfiguredBetLimitsApplyToAdmissionWithoutDebiting(t *testing.T) {
	c := newConfiguredFixture(t)
	ctx := context.Background()
	bettingTx(t, c.f.db, func(tx pgx.Tx) error {
		period, err := c.s.CreatePeriod(ctx, tx, c.a, c.g.ID, platformlottery.PeriodInput{PeriodNo: "limits-admission", BetStartAt: time.Now().Add(-time.Hour), BetEndAt: time.Now().Add(time.Hour), DrawAt: time.Now().Add(time.Hour)}, points.Metadata{})
		c.input.PeriodID = period.ID
		return err
	})
	c.b.Roles[0].Permissions = append(c.b.Roles[0].Permissions, access.Permission{Resource: "bet_policy", Action: "write", Scope: access.ScopeBrand})
	before, e := c.s.BetLimits(ctx, c.f.brand, c.g.ID)
	if e != nil {
		t.Fatal(e)
	}
	next := before
	next.MaxPoints = 500
	next.MaxPeriodPoints = 5000
	next.MaxUserPeriodPoints = 2500
	bettingTx(t, c.f.db, func(tx pgx.Tx) error {
		var err error
		next, err = c.s.WriteBetLimits(ctx, tx, c.b, c.f.brand, c.g.ID, next, points.Metadata{})
		return err
	})
	if next.Version != before.Version+1 {
		t.Fatal("sales version did not advance")
	}
	fundBettingWallet(t, c.f, 10000)
	wallet, e := (points.Store{DB: c.f.db}).Read(ctx, c.f.brand, c.f.member)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = configuredPlace(t, c, "limit-stale-sales", c.input); !errors.Is(e, ErrVersion) {
		t.Fatal("old confirmation accepted after limit change", e)
	}
	c.input.SalesVersion = next.Version
	if _, e = configuredPlace(t, c, "limit-over-max", c.input); !errors.Is(e, ErrLimit) {
		t.Fatal("new maximum did not reject oversized bet", e)
	}
	after, e := (points.Store{DB: c.f.db}).Read(ctx, c.f.brand, c.f.member)
	if e != nil || after.AvailablePoints != wallet.AvailablePoints || after.Version != wallet.Version {
		t.Fatal("rejected bet debited wallet", after, e)
	}
	var orders int
	if e = c.f.db.QueryRow(ctx, `SELECT count(*) FROM configured_orders WHERE brand_id=$1 AND game_id=$2`, c.f.brand, c.g.ID).Scan(&orders); e != nil || orders != 0 {
		t.Fatal("rejected request created an order", orders, e)
	}
}

func TestConfiguredPurchasedAndClosedOpeningCannotChange(t *testing.T) {
	c := newConfiguredFixture(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	var period platformlottery.Period
	bettingTx(t, c.f.db, func(tx pgx.Tx) error {
		var err error
		period, err = c.s.CreatePeriod(ctx, tx, c.a, c.g.ID, platformlottery.PeriodInput{PeriodNo: "opening-purchased", BetStartAt: now.Add(-time.Hour), BetEndAt: now.Add(time.Hour), DrawAt: now.Add(2 * time.Hour)}, points.Metadata{})
		return err
	})
	c.input.PeriodID = period.ID
	fundBettingWallet(t, c.f, 10000)
	if _, err := configuredPlace(t, c, "opening-purchased", c.input); err != nil {
		t.Fatal(err)
	}
	tx, err := c.f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.s.UpdatePeriodOpening(ctx, tx, c.a, period.ID, platformlottery.PeriodOpeningInput{ExpectedBetStartAt: period.BetStartAt, BetStartAt: now.Add(time.Minute)}, points.Metadata{})
	tx.Rollback(ctx)
	if !errors.Is(err, platformlottery.ErrPeriodInUse) {
		t.Fatal("purchased opening changed", err)
	}
	// Database protection also rejects a direct rewrite of purchased history.
	tx, err = c.f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `UPDATE platform_game_periods SET bet_start_at=$2 WHERE id=$1`, period.ID, now.Add(time.Minute))
	tx.Rollback(ctx)
	if err == nil {
		t.Fatal("database allowed purchased opening rewrite")
	}
	bettingTx(t, c.f.db, func(tx pgx.Tx) error {
		var err error
		period, err = c.s.CreatePeriod(ctx, tx, c.a, c.g.ID, platformlottery.PeriodInput{PeriodNo: "opening-closed", BetStartAt: now.Add(-3 * time.Hour), BetEndAt: now.Add(-2 * time.Hour), DrawAt: now.Add(-time.Hour)}, points.Metadata{})
		return err
	})
	tx, err = c.f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.s.UpdatePeriodOpening(ctx, tx, c.a, period.ID, platformlottery.PeriodOpeningInput{ExpectedBetStartAt: period.BetStartAt, BetStartAt: now.Add(-4 * time.Hour)}, points.Metadata{})
	tx.Rollback(ctx)
	if !errors.Is(err, platformlottery.ErrPeriodInUse) {
		t.Fatal("closed period opening changed", err)
	}
}

func TestConfiguredRetiredAutomaticPeriodCannotAcceptFunds(t *testing.T) {
	c := newConfiguredFixture(t)
	f := c.f
	ctx := context.Background()
	fundBettingWallet(t, f, 10000)
	const version int64 = 1
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `INSERT INTO platform_automation_versions(game_id,version,enabled,anchor_draw_at,anchor_period_no,interval_seconds,open_before_seconds,close_before_seconds,source_url,credential_ref,created_by,source_type) VALUES($1,1,true,clock_timestamp(),'100',120,120,15,'https://draw.example.test/retired','',$2,'interval_json')`, c.g.ID, c.a.ID)
		if e != nil {
			return e
		}
		p, e := c.s.CreatePeriod(ctx, tx, c.a, c.g.ID, platformlottery.PeriodInput{AutomationVersion: func() *int64 { v := version; return &v }(), PeriodNo: "retired-source", BetStartAt: time.Now().Add(-time.Minute), BetEndAt: time.Now().Add(time.Minute), DrawAt: time.Now().Add(2 * time.Minute)}, points.Metadata{})
		c.input.PeriodID = p.ID
		return e
	})
	before := walletBySource(t, f)
	if _, e := configuredPlace(t, c, "retired-source-bet", c.input); !errors.Is(e, ErrClosed) {
		t.Fatal("retired source admitted", e)
	}
	if after := walletBySource(t, f); after != before {
		t.Fatal("retired source changed funds")
	}
}
