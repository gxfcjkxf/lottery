package betting

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
	"testing"
)

func correctionActor(f bettingFixture) access.Account {
	a := settlementActor(f)
	a.Roles[0].Permissions = append(a.Roles[0].Permissions, access.Permission{Resource: "draw", Action: "correct", Scope: access.ScopeBrand}, access.Permission{Resource: "draw", Action: "correction_retry", Scope: access.ScopeBrand})
	return a
}
func settledCorrectionFixture(t *testing.T) (bettingFixture, Order, SettlementJob) {
	t.Helper()
	f, o := drawnFixtureOrder(t)
	a := correctionActor(f)
	mode := "automatic"
	p := setSettlementMode(t, f, a, 1, &mode)
	j := startFixtureSettlement(t, f, a, p.Version)
	if _, e := f.service.ProcessSettlements(context.Background(), 20); e != nil {
		t.Fatal(e)
	}
	j, e := f.service.SettlementJob(context.Background(), f.brand, j.ID)
	if e != nil || j.State != "completed" {
		t.Fatal(j, e)
	}
	o, e = f.service.Order(context.Background(), f.brand, f.member, o.ID)
	if e != nil {
		t.Fatal(e)
	}
	return f, o, j
}
func createFixtureCorrection(t *testing.T, f bettingFixture, result rules.Draw) Correction {
	t.Helper()
	ctx := context.Background()
	c, e := f.service.CorrectionContext(ctx, f.brand, f.period.ID)
	if e != nil {
		t.Fatal(e)
	}
	var pv *int64
	if c.RequiresResettlement {
		v := c.PolicyVersion
		pv = &v
	}
	var out Correction
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var e error
		out, e = f.service.CreateCorrection(ctx, tx, f.brand, correctionActor(f), *c.DrawResultID, CorrectionInput{Version: c.PeriodVersion, PolicyVersion: pv, Result: result, Reason: "verified corrected result"}, policyMeta(f.version.CreatedBy))
		return e
	})
	return out
}
func correctedDigits(values ...int) rules.Draw {
	return rules.Draw{Regular: []int{}, Special: []int{}, Digits: values}
}

func TestCorrectionReversesOldPrizeBeforeNewGenerationAndPreservesAllEvidence(t *testing.T) {
	f, o, old := settledCorrectionFixture(t)
	ctx := context.Background()
	before := walletBySource(t, f)
	c := createFixtureCorrection(t, f, correctedDigits(1, 2, 2))
	if c.State != "reversing" || c.TargetCount != 1 || c.NewJobID != nil {
		t.Fatal(c)
	}
	if walletBySource(t, f) != before {
		t.Fatal("start directly reversed money")
	}
	if _, e := f.service.ProcessCorrections(ctx, 20); e != nil {
		t.Fatal(e)
	}
	c, e := f.service.Correction(ctx, f.brand, c.ID)
	if e != nil || c.State != "resettling" || c.NewJobID == nil || c.ReversedPoints != "10" {
		t.Fatal(c, e)
	}
	if w := walletBySource(t, f); w[0][0] != 99 || w[1][0] != 0 {
		t.Fatal(w)
	}
	pending, e := f.service.Order(ctx, f.brand, f.member, o.ID)
	if e != nil || pending.Status != "placed" || pending.DebitEntryID != o.DebitEntryID || pending.RefundEntryID != "" || pending.SettlementCalculationID != nil {
		t.Fatal(pending, e)
	}
	history, e := f.service.SettlementJob(ctx, f.brand, old.ID)
	if e != nil || history.Current || history.PaidPoints != "10" || history.State != "completed" {
		t.Fatal(history, e)
	}
	next, e := f.service.SettlementJob(ctx, f.brand, *c.NewJobID)
	if e != nil || next.Generation != 2 || next.PreviousJobID == nil || *next.PreviousJobID != old.ID || !next.Current {
		t.Fatal(next, e)
	}
	if _, e = f.service.ProcessSettlements(ctx, 20); e != nil {
		t.Fatal(e)
	}
	c, e = f.service.Correction(ctx, f.brand, c.ID)
	if e != nil || c.State != "completed" {
		t.Fatal(c, e)
	}
	final, e := f.service.Order(ctx, f.brand, f.member, o.ID)
	if e != nil || final.Status != "lost" || final.PrizePoints != 0 {
		t.Fatal(final, e)
	}
	var n int
	if e = f.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE reversal_of=$1 AND entry_type='prize_reversal'`, *o.PayoutEntryID).Scan(&n); e != nil || n != 1 {
		t.Fatal(n, e)
	}
	if _, e = f.db.Exec(ctx, `UPDATE draw_results SET result='{}' WHERE id=$1`, old.DrawResultID); e == nil {
		t.Fatal("old result rewritten")
	}
	if _, e = f.db.Exec(ctx, `UPDATE settlement_calculations SET prize_points=0 WHERE id=$1`, *o.SettlementCalculationID); e == nil {
		t.Fatal("old calculation rewritten")
	}
}

func TestCorrectionInsufficientWinningAvailableStopsWithoutOtherSourceDebit(t *testing.T) {
	f, o, _ := settledCorrectionFixture(t)
	ctx := context.Background()
	// Existing audited source-preserving freeze takes the prior prize out of
	// winning.available. A correction cannot silently draw from frozen/recharge.
	tx, e := f.db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	ps := points.Store{DB: f.db}
	var delta points.Balance
	delta[1][0] = -10
	delta[1][2] = 10
	if _, e = ps.Post(ctx, tx, points.Change{BrandID: f.brand, MemberID: f.member, EntryType: "freeze", ReferenceType: "test", OperationKey: "correction-freeze-winning", Reason: "audited original source freeze", ActorType: "admin", ActorID: f.version.CreatedBy, RequestID: "test-correction-freeze", Delta: delta, Allocation: []points.Allocation{{Source: "winning", State: "available", Points: 10}}}); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	before := walletBySource(t, f)
	c := createFixtureCorrection(t, f, correctedDigits(1, 2, 2))
	if _, e = f.service.ProcessCorrections(ctx, 20); e == nil {
		t.Fatal("insufficient original source not reported")
	}
	c, e = f.service.Correction(ctx, f.brand, c.ID)
	if e != nil || c.State != "failed" || c.LastErrorCode == nil || *c.LastErrorCode != "WINNING_AVAILABLE_INSUFFICIENT" || c.NewJobID != nil || walletBySource(t, f) != before {
		t.Fatal(c, e)
	}
	fresh, e := f.service.Order(ctx, f.brand, f.member, o.ID)
	if e != nil || fresh.Status != "won" {
		t.Fatal(fresh, e)
	}
	if n, e := f.service.ProcessCorrections(ctx, 20); e != nil || n != 0 {
		t.Fatal("automatic retry", n, e)
	}
}
