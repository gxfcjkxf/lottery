package betting

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rulebook"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
	"reflect"
	"testing"
	"time"
)

func previewActor(f bettingFixture) access.Account {
	a := judgeActor(f)
	a.Roles[0].Permissions = append(a.Roles[0].Permissions, access.Permission{Resource: "settlement", Action: "preview", Scope: access.ScopeBrand})
	return a
}
func drawnFixtureOrder(t *testing.T) (bettingFixture, Order) {
	return drawnFixtureSelectedOrder(t, nil)
}
func drawnFixtureSelectedOrder(t *testing.T, selection *rules.Selection) (bettingFixture, Order) {
	t.Helper()
	f := newBettingFixtureWithWindow(t, storeTestBrand, 2*time.Second, 2100*time.Millisecond)
	if selection != nil {
		f.input.Selection = *selection
	}
	ctx := context.Background()
	fundBettingWallet(t, f, 100)
	o, e := placeBettingOrder(t, f, f.input, "settlement-preview-order")
	if e != nil {
		t.Fatal(e)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, e = (rulebook.Store{DB: f.db}).Tick(ctx); e != nil {
			t.Fatal(e)
		}
		var ready bool
		f.db.QueryRow(ctx, `SELECT status='waiting_draw' FROM periods WHERE id=$1`, f.period.ID).Scan(&ready)
		if ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("fixture did not reach draw time")
		}
		time.Sleep(25 * time.Millisecond)
	}
	var version int64
	var at time.Time
	f.db.QueryRow(ctx, `SELECT version,draw_at FROM periods WHERE id=$1`, f.period.ID).Scan(&version, &at)
	publicManual(t, f, f.period.ID, version, f.period.PeriodNo, rules.Draw{Digits: []int{1, 2, 1}}, at)
	return f, o
}
func createPreview(t *testing.T, f bettingFixture, o Order) SettlementPreview {
	t.Helper()
	ctx := context.Background()
	c, e := f.service.SettlementContext(ctx, f.brand, o.ID)
	if e != nil {
		t.Fatal(e)
	}
	var p SettlementPreview
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var e error
		p, e = f.service.CreateSettlementPreview(ctx, tx, f.brand, previewActor(f), o.ID, SettlementPreviewInput{Version: o.Version, PeriodVersion: c.PeriodVersion, DrawResultID: *c.DrawResultID, Reason: "independent settlement verification"}, policyMeta(f.version.CreatedBy))
		return e
	})
	return p
}
func TestSettlementPreviewUsesOrderSnapshotPersistsTraceAndNeverPostsFunds(t *testing.T) {
	f, o := drawnFixtureOrder(t)
	ctx := context.Background()
	before := walletBySource(t, f)
	p := createPreview(t, f, o)
	if p.Outcome != "won" || p.Calculation == nil || p.Calculation.PrizePoints != 10 || p.Applied || !p.Current || p.AuditLogID == "" {
		t.Fatal(p)
	}
	if walletBySource(t, f) != before {
		t.Fatal("preview paid points")
	}
	fresh, e := f.service.Order(ctx, f.brand, f.member, o.ID)
	if e != nil || !reflect.DeepEqual(o, fresh) {
		t.Fatal("preview changed order", e)
	}
	lines, e := f.service.SettlementPreviewLines(ctx, f.brand, p.ID, 20, 0)
	if e != nil || len(lines.Items) != 1 || len(lines.Items[0].Hits) != 1 || !lines.Items[0].Hits[0].Selected {
		t.Fatal(lines, e)
	}
	history, e := f.service.SettlementPreviews(ctx, f.brand, o.ID, 1, 0)
	if e != nil || len(history.Items) != 1 || history.Items[0].ID != p.ID {
		t.Fatal(history, e)
	}
	if _, e = f.db.Exec(ctx, `UPDATE settlement_previews SET reason='replace history' WHERE id=$1`, p.ID); e == nil {
		t.Fatal("mutable preview")
	}
	if _, e = f.db.Exec(ctx, `DELETE FROM settlement_previews WHERE id=$1`, p.ID); e == nil {
		t.Fatal("deleted preview")
	}
	if _, e = f.service.SettlementPreview(ctx, storeTestOtherBrand, p.ID); !errors.Is(e, ErrNotFound) {
		t.Fatal("cross scope", e)
	}
	// A later cancellation invalidates current but preserves the earlier record.
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		_, e := f.service.CancelAdmin(ctx, tx, f.brand, judgeActor(f), o.ID, o.Version, "operator cancels after checking preview", policyMeta(f.version.CreatedBy))
		return e
	})
	old, e := f.service.SettlementPreview(ctx, f.brand, p.ID)
	if e != nil || old.Current {
		t.Fatal(old, e)
	}
	now, e := f.service.Order(ctx, f.brand, f.member, o.ID)
	if e != nil {
		t.Fatal(e)
	}
	excluded := createPreview(t, f, now)
	if excluded.Outcome != "excluded" || excluded.Calculation != nil || excluded.ErrorCode == nil {
		t.Fatal(excluded)
	}
}
func TestPreviewFailureAndStaleContextCannotLeaveEvidence(t *testing.T) {
	f, o := drawnFixtureOrder(t)
	ctx := context.Background()
	c, e := f.service.SettlementContext(ctx, f.brand, o.ID)
	if e != nil {
		t.Fatal(e)
	}
	in := SettlementPreviewInput{Version: o.Version, PeriodVersion: c.PeriodVersion, DrawResultID: *c.DrawResultID, Reason: "test evidence rollback"}
	for _, test := range []struct {
		input SettlementPreviewInput
		actor access.Account
		want  error
	}{{SettlementPreviewInput{Version: 2, PeriodVersion: in.PeriodVersion, DrawResultID: in.DrawResultID, Reason: in.Reason}, previewActor(f), ErrVersion}, {in, access.Account{ID: f.version.CreatedBy, SuperAdmin: true}, ErrDenied}} {
		tx, _ := f.db.Begin(ctx)
		_, e = f.service.CreateSettlementPreview(ctx, tx, f.brand, test.actor, o.ID, test.input, policyMeta(f.version.CreatedBy))
		tx.Rollback(ctx)
		if !errors.Is(e, test.want) {
			t.Fatal(e, test.want)
		}
	}
	if _, e = f.db.Exec(ctx, `CREATE FUNCTION reject_preview_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='settlement.preview' THEN RAISE EXCEPTION 'test';END IF;RETURN NEW;END $$;CREATE TRIGGER reject_preview_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_preview_audit()`); e != nil {
		t.Fatal(e)
	}
	tx, _ := f.db.Begin(ctx)
	_, e = f.service.CreateSettlementPreview(ctx, tx, f.brand, previewActor(f), o.ID, in, policyMeta(f.version.CreatedBy))
	tx.Rollback(ctx)
	if e == nil {
		t.Fatal("expected failed audit")
	}
	var n int
	f.db.QueryRow(ctx, `SELECT count(*) FROM settlement_previews`).Scan(&n)
	if n != 0 {
		t.Fatal(n)
	}
}
func TestSnapshotEvaluatorRejectsPartialCorruptionWithoutPartialPrize(t *testing.T) {
	_, o := drawnFixtureOrder(t)
	draw := rules.Draw{Regular: []int{}, Special: []int{}, Digits: []int{1, 2, 1}}
	for _, mutate := range []func(*Order){func(v *Order) { v.TotalPoints++ }, func(v *Order) { v.DefinitionHash = "bad" }, func(v *Order) { v.Expanded = []rules.Selection{} }, func(v *Order) { v.SelectionNormalized = rules.Selection{} }, func(v *Order) { v.Allocation = []points.Allocation{{Source: "gift", State: "available", Points: 0}} }} {
		bad := o
		mutate(&bad)
		_, code, e := evaluateSettlementSnapshot(context.Background(), bad, draw)
		if e != nil || code == "" {
			t.Fatal(code, e)
		}
	}
	if _, code, e := evaluateSettlementSnapshot(context.Background(), o, draw); e != nil || code != "" {
		t.Fatal(code, e)
	}
}

func TestPreviewStillUsesPurchasedOddsAfterNewImmediateRuleApproval(t *testing.T) {
	f, o := drawnFixtureOrder(t)
	ctx := context.Background()
	creator := bettingRuleActor(f.version.CreatedBy, f.brand, "write", "validate", "submit")
	reviewer := bettingRuleActor(f.version.ReviewedBy, f.brand, "review")
	d := bettingDefinition()
	d.PrizeTiers[0].Odds = "99"
	var next rulebook.Version
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var e error
		next, e = (rulebook.Store{DB: f.db}).CreateVersion(ctx, tx, f.brand, creator, f.play.ID, d, "immediate", "new odds unrelated to purchased ticket", policyMeta(creator.ID))
		return e
	})
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var e error
		next, e = (rulebook.Store{DB: f.db}).Validate(ctx, tx, f.brand, creator, next.ID, next.Version, []rules.ValidationCase{{Name: "new exact rule", Selection: f.input.Selection, Draw: rules.Draw{Digits: []int{1, 2, 1}}, Multiplier: 1, ExpectedBetPoints: amountPtr(1), ExpectedPrizePoints: amountPtr(99), ExpectedWon: boolPtr(true)}}, "test new odds", policyMeta(creator.ID))
		return e
	})
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var e error
		next, e = (rulebook.Store{DB: f.db}).Submit(ctx, tx, f.brand, creator, next.ID, next.Version, "independent review", policyMeta(creator.ID))
		return e
	})
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var e error
		next, e = (rulebook.Store{DB: f.db}).Review(ctx, tx, f.brand, reviewer, next.ID, next.Version, true, true, "approve new odds", policyMeta(reviewer.ID))
		return e
	})
	p := createPreview(t, f, o)
	if p.Calculation == nil || p.Calculation.PrizePoints != 10 || p.DefinitionHash != o.DefinitionHash {
		t.Fatal(p)
	}
	old, e := (rulebook.Store{DB: f.db}).GetVersion(ctx, f.brand, f.version.ID)
	if e != nil || old.Status != "expired" {
		t.Fatal(old, e)
	}
}
func TestMalformedStoredOrderGetsWholeOrderAbnormalEvidenceAndExistingAbnormalIsExcluded(t *testing.T) {
	f, o := drawnFixtureOrder(t)
	ctx := context.Background()
	before := walletBySource(t, f)
	// Only this disposable schema bypasses a trigger to model a corrupt restore.
	if _, e := f.db.Exec(ctx, `ALTER TABLE bet_orders DISABLE TRIGGER immutable_bet_order`); e != nil {
		t.Fatal(e)
	}
	if _, e := f.db.Exec(ctx, `UPDATE bet_orders SET selection_normalized='{}' WHERE id=$1`, o.ID); e != nil {
		t.Fatal(e)
	}
	if _, e := f.db.Exec(ctx, `ALTER TABLE bet_orders ENABLE TRIGGER immutable_bet_order`); e != nil {
		t.Fatal(e)
	}
	p := createPreview(t, f, o)
	if p.Outcome != "abnormal" || p.Calculation != nil || p.ErrorCode == nil || *p.ErrorCode != "STAKE_SNAPSHOT_MISMATCH" {
		t.Fatal(p)
	}
	lines, e := f.service.SettlementPreviewLines(ctx, f.brand, p.ID, 20, 0)
	if e != nil || len(lines.Items) != 0 || lines.Total != 0 {
		t.Fatal(lines, e)
	}
	if walletBySource(t, f) != before {
		t.Fatal("partial award")
	}
	// Classification remains a separate explicit operator action, not a preview side effect.
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		_, e := f.service.MarkAbnormal(ctx, tx, f.brand, exceptionActor(f), o.ID, 1, "corrupt snapshot investigation", policyMeta(f.version.CreatedBy))
		return e
	})
	current, e := f.service.Order(ctx, f.brand, f.member, o.ID)
	if e != nil {
		t.Fatal(e)
	}
	skip := createPreview(t, f, current)
	if skip.Outcome != "excluded" || skip.Calculation != nil || skip.ErrorCode == nil || *skip.ErrorCode != "ORDER_ABNORMAL" {
		t.Fatal(skip)
	}
}
func TestPreviewInputIsClosedAndContextCancellationIsNotAnomaly(t *testing.T) {
	for _, raw := range []string{`{}`, `{"version":1,"period_version":2,"draw_result_id":"x","reason":"r","version":2}`, `{"version":null,"period_version":2,"draw_result_id":"x","reason":"r"}`, `{"version":1,"period_version":2,"draw_result_id":"x","reason":"r","payout":true}`} {
		var in SettlementPreviewInput
		if json.Unmarshal([]byte(raw), &in) == nil {
			t.Fatal(raw)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, code, e := evaluateSettlementSnapshot(ctx, Order{}, rules.Draw{})
	if !errors.Is(e, context.Canceled) || code != "" {
		t.Fatal(code, e)
	}
}
