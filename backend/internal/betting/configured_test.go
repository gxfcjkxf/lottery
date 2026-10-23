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
	cases := []platformlottery.ConfiguredCase{{Requests: []rules.ConfiguredRequest{{PlayID: "operator_new_play", Options: []string{"lucky"}, Plate: "A", Points: 10, Multiplier: 1}}, Draw: rules.Draw{Regular: []int{1, 3, 7, 4, 5, 6}, Special: []int{2}}, Bet: 10, Prize: 25}}
	return configuredFixtureFrom(t, raw, cases)
}
func configuredFixtureFrom(t *testing.T, raw []byte, cases []platformlottery.ConfiguredCase) configuredFixture {
	t.Helper()
	ctx := context.Background()
	f := newBettingFixture(t, storeTestBrand)
	s := platformlottery.Service{DB: f.db}
	a := access.Account{ID: f.version.CreatedBy, Type: access.AccountAdmin, SuperAdmin: true, Roles: []access.Role{{Permissions: []access.Permission{{Resource: "platform_game", Action: "write", Scope: access.ScopePlatform}}}}}
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
		v, e = s.SaveConfiguredDraft(ctx, tx, a, g.ID, platformlottery.ConfiguredDraftInput{Snapshot: engine.Snapshot(), Cases: cases}, points.Metadata{})
		return e
	})
	for _, action := range []string{"validate", "submit", "approve", "publish"} {
		bettingTx(t, f.db, func(tx pgx.Tx) error {
			var e error
			v, e = s.ActConfiguredRule(ctx, tx, a, g.ID, v.ID, platformlottery.ConfiguredAction{Revision: v.Revision, Action: action}, points.Metadata{})
			return e
		})
	}
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		_, e := s.Connect(ctx, tx, b, f.brand, g.ID, "", points.Metadata{})
		if e != nil {
			return e
		}
		_, e = s.WriteConfiguredSales(ctx, tx, b, f.brand, g.ID, platformlottery.ConfiguredSales{RuleID: v.ID, Enabled: true, Prices: snapshot.Prices, MinPoints: 1, MaxPoints: 100, MaxPeriodPoints: 10000, MaxUserPeriodPoints: 10000}, points.Metadata{})
		return e
	})
	var p platformlottery.Period
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var e error
		p, e = s.CreatePeriod(ctx, tx, a, g.ID, platformlottery.PeriodInput{PeriodNo: "configured-1", BetStartAt: time.Now().Add(-time.Hour), BetEndAt: time.Now().Add(3 * time.Second), DrawAt: time.Now().Add(3 * time.Second)}, points.Metadata{})
		return e
	})
	return configuredFixture{f, s, a, b, g, p, v, ConfiguredInput{PeriodID: p.ID, RuleID: v.ID, SalesVersion: 1, SnapshotHash: engine.SnapshotHash(), Requests: cases[0].Requests}}
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
		return mutation.OK(201, o), nil
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
	fundBettingWallet(t, f, 100)
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
	altered.Requests[0].Points = 11
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
	_, e = c.s.SaveConfiguredDraft(ctx, tx, c.b, c.g.ID, platformlottery.ConfiguredDraftInput{Snapshot: c.v.Snapshot}, points.Metadata{})
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
	var next platformlottery.ConfiguredVersion
	cases := append([]platformlottery.ConfiguredCase{}, c.v.Cases...)
	cases[0].Prize = 0
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var e error
		next, e = c.s.SaveConfiguredDraft(ctx, tx, c.a, c.g.ID, platformlottery.ConfiguredDraftInput{Snapshot: raw, Cases: cases}, points.Metadata{})
		return e
	})
	for _, action := range []string{"validate", "submit", "approve", "publish"} {
		bettingTx(t, f.db, func(tx pgx.Tx) error {
			var e error
			next, e = c.s.ActConfiguredRule(ctx, tx, c.a, c.g.ID, next.ID, platformlottery.ConfiguredAction{Revision: next.Revision, Action: action}, points.Metadata{})
			return e
		})
	}
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
	if e != nil || len(rows) != 1 || rows[0].Result == nil || rows[0].Result.PrizePoints != 25 {
		t.Fatal("frozen payout", rows, e)
	}
	if n, e := f.service.ProcessConfiguredSettlements(ctx, 10); e != nil || n != 0 {
		t.Fatal("repeated payout", n, e)
	}
	wallet, e := (points.Store{DB: f.db}).Read(ctx, f.brand, f.member)
	if e != nil || wallet.AvailablePoints != 115 {
		t.Fatal("wallet", wallet, e)
	}
	var invalid int
	if e = f.db.QueryRow(ctx, `SELECT count(*) FROM point_configured_business_witnesses($1,$2) WHERE NOT source_valid`, f.brand, wallet.AccountID).Scan(&invalid); e != nil || invalid != 0 {
		t.Fatal("financial witnesses", invalid, e)
	}
	var consistent bool
	if e = f.db.QueryRow(ctx, `SELECT (point_business_preview($1,$2)->>'consistent')::boolean`, f.brand, wallet.AccountID).Scan(&consistent); e != nil || !consistent {
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
	if e != nil || len(rows) != 1 || rows[0].Result.PrizePoints != 25 {
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
	if e = f.db.QueryRow(ctx, `SELECT (point_business_preview($1,$2)->>'consistent')::boolean`, f.brand, wallet.AccountID).Scan(&consistent); e != nil || !consistent {
		t.Fatal("deletion damaged reconciliation", e)
	}
}

func TestConfiguredWorkflowValidationIsBoundToRevision(t *testing.T) {
	c := newConfiguredFixture(t)
	ctx := context.Background()
	cases := append([]platformlottery.ConfiguredCase{}, c.v.Cases...)
	cases[0].Prize = 26
	var v platformlottery.ConfiguredVersion
	bettingTx(t, c.f.db, func(tx pgx.Tx) error {
		var e error
		v, e = c.s.SaveConfiguredDraft(ctx, tx, c.a, c.g.ID, platformlottery.ConfiguredDraftInput{Snapshot: c.v.Snapshot, Cases: cases}, points.Metadata{})
		return e
	})
	tx, _ := c.f.db.Begin(ctx)
	_, e := c.s.ActConfiguredRule(ctx, tx, c.a, c.g.ID, v.ID, platformlottery.ConfiguredAction{Revision: v.Revision, Action: "validate"}, points.Metadata{})
	tx.Rollback(ctx)
	if !errors.Is(e, platformlottery.ErrInvalid) {
		t.Fatal("accepted false expected result", e)
	}
	cases[0].Prize = 25
	bettingTx(t, c.f.db, func(tx pgx.Tx) error {
		var e error
		v, e = c.s.SaveConfiguredDraft(ctx, tx, c.a, c.g.ID, platformlottery.ConfiguredDraftInput{ID: v.ID, Revision: v.Revision, Snapshot: c.v.Snapshot, Cases: cases}, points.Metadata{})
		return e
	})
	bettingTx(t, c.f.db, func(tx pgx.Tx) error {
		var e error
		v, e = c.s.ActConfiguredRule(ctx, tx, c.a, c.g.ID, v.ID, platformlottery.ConfiguredAction{Revision: v.Revision, Action: "validate"}, points.Metadata{})
		return e
	})
	bettingTx(t, c.f.db, func(tx pgx.Tx) error {
		var e error
		v, e = c.s.SaveConfiguredDraft(ctx, tx, c.a, c.g.ID, platformlottery.ConfiguredDraftInput{ID: v.ID, Revision: v.Revision, Snapshot: c.v.Snapshot, Cases: cases}, points.Metadata{})
		return e
	})
	if v.Status != "draft" || len(v.Validation) > 0 {
		t.Fatal("save kept old evidence", v.Status)
	}
	tx, _ = c.f.db.Begin(ctx)
	_, e = c.s.ActConfiguredRule(ctx, tx, c.a, c.g.ID, v.ID, platformlottery.ConfiguredAction{Revision: v.Revision, Action: "submit"}, points.Metadata{})
	tx.Rollback(ctx)
	if !errors.Is(e, platformlottery.ErrConflict) {
		t.Fatal("unvalidated change submitted", e)
	}
	visible, e := c.s.ConfiguredVersions(ctx, c.f.brand, c.g.ID)
	if e != nil || len(visible) != 1 || visible[0].ID != c.v.ID {
		t.Fatal("brand read unpublished draft", visible, e)
	}
}

func TestConfiguredDrawPublicationChecksFrozenSemantics(t *testing.T) {
	c := newConfiguredFixture(t, true)
	fundBettingWallet(t, c.f, 100)
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
	fundBettingWallet(t, c.f, 100)
	in := c.input
	in.Requests[0].Points = 30
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
	if e != nil || w.AvailablePoints != 10 {
		t.Fatal("overdraw", w, e)
	}
}
func TestConfiguredPushReturnsOriginalSourcesOnce(t *testing.T) {
	c := newConfiguredFixture(t)
	f := c.f
	fundBettingWallet(t, f, 15, 5, 100)
	c.input.Requests[0].Points = 30
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
	if e != nil || w.BySource[0][0] != 15 || w.BySource[1][0] != 5 || w.BySource[2][0] != 100 {
		t.Fatal("refund allocation", w, e)
	}
}

func TestConfiguredDrawSourcesSelectionAndFrozenPeriods(t *testing.T) {
	c := newConfiguredFixture(t)
	ctx := context.Background()
	anchor := time.Now().Add(time.Hour)
	number := "900001"
	interval := 120
	plan := platformlottery.AutomationPlan{SourceType: "mapped", Mapping: &platformlottery.FeedMapping{Format: "json", Schedule: "interval", TimeFormat: "rfc3339", Period: platformlottery.FeedField{Path: "/period_no"}, DrawTime: platformlottery.FeedField{Path: "/drawn_at"}, Regular: &platformlottery.FeedNumbers{FeedField: platformlottery.FeedField{Path: "/result/regular"}, Count: 6}, Special: &platformlottery.FeedNumbers{FeedField: platformlottery.FeedField{Path: "/result/special"}, Count: 1}}, Enabled: true, AnchorDrawAt: &anchor, AnchorPeriodNo: &number, IntervalSeconds: &interval, CloseBeforeSeconds: 15, SourceURL: "https://first.example/result"}
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
	plan := platformlottery.AutomationPlan{SourceType: "mapped", Mapping: &platformlottery.FeedMapping{Format: "json", Schedule: "interval", TimeFormat: "rfc3339", Period: platformlottery.FeedField{Path: "/period_no"}, DrawTime: platformlottery.FeedField{Path: "/drawn_at"}, Regular: &platformlottery.FeedNumbers{FeedField: platformlottery.FeedField{Path: "/result/regular"}, Count: 6}, Special: &platformlottery.FeedNumbers{FeedField: platformlottery.FeedField{Path: "/result/special"}, Count: 1}}, Enabled: true, AnchorDrawAt: &anchor, AnchorPeriodNo: &number, IntervalSeconds: &interval, CloseBeforeSeconds: 15, SourceURL: "https://manual.example/result"}
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
			p := platformlottery.AutomationPlan{SourceType: "mapped", Mapping: m, Enabled: true, AnchorDrawAt: &anchor, AnchorPeriodNo: &number, IntervalSeconds: &interval, SourceURL: server.URL + "/unrelated-feed?lang=en"}
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
