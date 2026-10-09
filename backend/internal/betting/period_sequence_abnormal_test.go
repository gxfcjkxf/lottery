package betting

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/rulebook"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

func TestNextPeriodWaitsForSettlementWithAbnormalOrder(t *testing.T) {
	f := newBettingFixtureWithWindow(t, storeTestBrand, 2*time.Second, 2100*time.Millisecond)
	f.input.Selection = rules.Selection{Digits: [][]int{{0}, {1}, {0}}}
	fundBettingWallet(t, f, 100)
	ctx := context.Background()

	normal, err := placeBettingOrder(t, f, f.input, "period-sequence-normal-order")
	if err != nil {
		t.Fatal(err)
	}
	abnormal, err := placeBettingOrder(t, f, f.input, "period-sequence-abnormal-order")
	if err != nil {
		t.Fatal(err)
	}
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var e error
		abnormal, e = f.service.MarkAbnormal(ctx, tx, f.brand, exceptionActor(f), abnormal.ID, abnormal.Version, "verified invalid betting data", policyMeta(f.version.CreatedBy))
		return e
	})
	evidence, err := f.service.Exception(ctx, f.brand, abnormal.ID)
	if err != nil || evidence == nil || evidence.OrderVersion != abnormal.Version || evidence.Reason != "verified invalid betting data" {
		t.Fatalf("abnormal order evidence=%+v err=%v", evidence, err)
	}
	abnormalWallet := walletBySource(t, f)
	if abnormal.Status != "abnormal" || abnormal.Version != 2 || abnormal.TotalPoints != 1 || abnormal.SettlementCalculationID != nil || abnormal.PayoutEntryID != nil || abnormal.PrizePoints != 0 || abnormalWallet[0][0] != 98 || abnormalWallet[1][0] != 0 {
		t.Fatalf("marking abnormal changed its economic state: order=%+v wallet=%v", abnormal, abnormalWallet)
	}

	draw := finishNoticeDraw(t, f)
	actor := settlementActor(f)
	mode := "manual"
	policy := setSettlementMode(t, f, actor, 1, &mode)
	job := startFixtureSettlement(t, f, actor, policy.Version)
	if job.TargetCount != 2 || job.ExcludedCount != 1 {
		t.Fatalf("settlement did not include the abnormal order as an excluded terminal target: %+v", job)
	}
	targets, err := f.service.SettlementTargets(ctx, f.brand, job.ID, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	var abnormalTarget *SettlementTarget
	for i := range targets.Items {
		if targets.Items[i].OrderID == abnormal.ID {
			abnormalTarget = &targets.Items[i]
		}
	}
	if abnormalTarget == nil || abnormalTarget.State != "excluded" || abnormalTarget.OrderStatus != "abnormal" || abnormalTarget.CalculationID != nil || abnormalTarget.PayoutEntryID != nil {
		t.Fatalf("abnormal order was made eligible for settlement: %+v", abnormalTarget)
	}

	if _, err = f.service.ProcessSettlements(ctx, 20); err != nil {
		t.Fatal(err)
	}
	job, err = f.service.SettlementJob(ctx, f.brand, job.ID)
	if err != nil || job.State != "awaiting_approval" || job.ReadyCount != 1 || job.ExcludedCount != 1 {
		t.Fatalf("manual settlement did not calculate only the normal order: job=%+v err=%v", job, err)
	}
	if _, err = attemptNextPeriod(t, f, "blocked-before-normal-payout"); !errors.Is(err, rulebook.ErrState) {
		t.Fatalf("next period opened before normal payout: %v", err)
	}

	job, err = actOnFixtureSettlement(t, f, actor, job, "approve", job.Version)
	if err != nil || job.State != "paying" {
		t.Fatalf("approve settlement: job=%+v err=%v", job, err)
	}
	if processed, processErr := f.service.ProcessSettlements(ctx, 1); processErr != nil || processed != 1 {
		t.Fatalf("pay normal order: processed=%d err=%v", processed, processErr)
	}
	job, err = f.service.SettlementJob(ctx, f.brand, job.ID)
	settledNormal, normalErr := f.service.Order(ctx, f.brand, f.member, normal.ID)
	if err != nil || normalErr != nil || job.State != "paying" || job.PaidCount != 1 || settledNormal.Status != "won" || settledNormal.PrizePoints != 10 {
		t.Fatalf("normal payout was not applied while settlement remained incomplete: job=%+v order=%+v err=%v orderErr=%v", job, settledNormal, err, normalErr)
	}
	if _, err = attemptNextPeriod(t, f, "blocked-before-settlement-completion"); !errors.Is(err, rulebook.ErrState) {
		t.Fatalf("next period opened before full period settlement completed: %v", err)
	}

	if _, err = f.service.ProcessSettlements(ctx, 20); err != nil {
		t.Fatal(err)
	}
	job, err = f.service.SettlementJob(ctx, f.brand, job.ID)
	if err != nil || job.State != "completed" || job.PaidCount != 1 || job.ExcludedCount != 1 || job.PaidPoints != "10" {
		t.Fatalf("settlement did not finish with one payout and one exclusion: job=%+v err=%v", job, err)
	}
	opened, err := attemptNextPeriod(t, f, "after-full-settlement")
	if err != nil {
		t.Fatalf("next period remained blocked after full settlement: %v", err)
	}

	loadedAbnormal, err := f.service.Order(ctx, f.brand, f.member, abnormal.ID)
	if err != nil || loadedAbnormal.Status != abnormal.Status || loadedAbnormal.Version != abnormal.Version || loadedAbnormal.TotalPoints != abnormal.TotalPoints || loadedAbnormal.DebitEntryID != abnormal.DebitEntryID || loadedAbnormal.SettlementCalculationID != nil || loadedAbnormal.PayoutEntryID != nil || loadedAbnormal.PrizePoints != 0 || loadedAbnormal.SettledAt != nil {
		t.Fatalf("settlement changed abnormal order or paid it: order=%+v err=%v", loadedAbnormal, err)
	}
	loadedEvidence, err := f.service.Exception(ctx, f.brand, abnormal.ID)
	if err != nil || loadedEvidence == nil || loadedEvidence.ID != evidence.ID || loadedEvidence.OrderVersion != evidence.OrderVersion || loadedEvidence.Reason != evidence.Reason {
		t.Fatalf("settlement changed abnormal evidence: evidence=%+v err=%v", loadedEvidence, err)
	}
	if wallet := walletBySource(t, f); wallet[0][0] != 98 || wallet[1][0] != 10 {
		t.Fatalf("unexpected wallet after settlement: %v", wallet)
	}
	in := f.input
	in.PeriodID = opened.ID
	if _, err = placeBettingOrder(t, f, in, "period-sequence-next-after-abnormal-settlement"); err != nil {
		t.Fatalf("next period rejected a bet after full settlement (draw %s): %v", draw.ID, err)
	}
	if wallet := walletBySource(t, f); wallet[0][0] != 97 || wallet[1][0] != 10 {
		t.Fatalf("next-period bet did not debit normally: %v", wallet)
	}
}
