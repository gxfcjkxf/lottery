package betting

import (
	"context"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
)

func TestCommissionCorrectionExecutionUpgradePreservesActualPaidHistoricalCycle(t *testing.T) {
	t.Parallel()
	db := testdb.NewAtVersion(t, 58)
	f := newAutomaticCommissionBatchFixtureFromBetting(t, newBettingFixtureWithDBWindow(t, db, storeTestBrand, 20*time.Second, 22*time.Second))
	ctx := context.Background()
	waitCommissionBoundary(t, f.boundary)
	cycle := createCommissionCycle(t, f)
	eligibilitySettle(t, f.betting)
	advanceCommissionWorker(t, f, 40)
	cycle = readCommissionCycle(t, f, cycle.ID)
	if cycle.State != "ready" || cycle.TotalPoints != "1" {
		t.Fatalf("real pre-upgrade earning missing: %+v", cycle)
	}
	gate := commissionPaymentPolicy(t, f)
	if err := updateCommissionPaymentPolicy(t, f, commissionPaymentActor(f), true, gate.Version); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ProcessPayments(ctx, 100); err != nil {
		t.Fatal(err)
	}
	payment := commissionPayments(t, f).Items[0]
	if payment.State != "paid" || payment.PaidPoints != "1" {
		t.Fatalf("pre-0059 real one-point posting absent: %+v", payment)
	}
	correctCommissionRunToZero(t, f, payment.CycleID, payment.RunID)
	if _, err := f.service.ProcessPayments(ctx, 100); err != nil {
		t.Fatal(err)
	}
	payment = commissionPaymentRead(t, f, payment.ID)
	if payment.State != "blocked" {
		t.Fatalf("old corrected positive payment must stop: %+v", payment)
	}
	before := correctionPlanSnapshot(t, f, payment.ID)
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if after := correctionPlanSnapshot(t, f, payment.ID); after != before {
		t.Fatal("0059 changed actual prior posting, wallet, audit or outbox history")
	}
	policy := correctionExecutionPolicyRead(t, f)
	if policy.Enabled || policy.Version != 1 || policy.AuditLogID != "" {
		t.Fatalf("financial correction gate opened on upgrade: %+v", policy)
	}
	if _, err := f.service.ProcessCorrectionPlans(ctx, 100); err != nil {
		t.Fatal(err)
	}
	plans := correctionPlansRead(t, f)
	if len(plans.Items) != 1 || plans.Items[0].State != commission.CorrectionPlanReady {
		t.Fatalf("actual historical cycle cannot prepare after upgrade: %+v", plans)
	}
	if n, err := f.service.ProcessCorrectionExecutions(ctx, 100); err != nil || n != 0 {
		t.Fatalf("old enabled payout gate triggered new financial work without opt-in: steps=%d err=%v", n, err)
	}
	if page := correctionExecutionsRead(t, f); page.TotalCount != "0" || len(page.Items) != 0 {
		t.Fatalf("default-off upgrade created a financial job: %+v", page)
	}
	if after := correctionPlanSnapshot(t, f, payment.ID); after != before {
		t.Fatal("historical planning/default-off execution changed old financial facts")
	}
	enableCorrectionExecutionPolicy(t, f)
	x := ensureCorrectionExecution(t, f)
	if x.State != commission.CorrectionExecutionCompleted || x.AppliedDebitPoints != "1" || x.PlanID != plans.Items[0].ID {
		t.Fatalf("opted-in historical difference did not execute exactly once: %+v", x)
	}
	if wallet := commissionWalletBySource(t, f, f.node.MemberID); wallet[3][0] != 0 {
		t.Fatalf("historical one-point debit did not use commission available: %+v", wallet)
	}
	if old := commissionPaymentRead(t, f, payment.ID); old.State != "blocked" || old.PaidPoints != "1" {
		t.Fatalf("actual recovery overwrote original payment: %+v", old)
	}
	beforeRepeat := correctionPlanSnapshot(t, f, payment.ID)
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if after := correctionPlanSnapshot(t, f, payment.ID); after != beforeRepeat {
		t.Fatal("migration replay changed executed historical compensation")
	}
}
