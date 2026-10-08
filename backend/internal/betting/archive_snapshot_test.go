package betting

import (
	"context"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/reporting"
	"github.com/jackc/pgx/v5"
)

func captureArchiveForBettingTest(t *testing.T, db interface {
	Begin(context.Context) (pgx.Tx, error)
}, reports reporting.Service, brand string, from, to time.Time) reporting.ArchiveSnapshot {
	t.Helper()
	tx, err := db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	snapshot, err := reports.CaptureArchive(context.Background(), tx, brand, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestArchiveSnapshotBettingRetainsCorrectedPrizeHistory(t *testing.T) {
	f, _, _ := settledCorrectionFixture(t)
	ctx := context.Background()
	correction := createFixtureCorrection(t, f, correctedDigits(1, 2, 2))
	if correction.State != "reversing" {
		t.Fatalf("correction did not enter the real reversal workflow: %+v", correction)
	}
	if _, err := f.service.ProcessCorrections(ctx, 20); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ProcessSettlements(ctx, 20); err != nil {
		t.Fatal(err)
	}

	from, to := time.Now().UTC().Add(-time.Hour), time.Now().UTC().Add(time.Hour)
	reports := reporting.Service{DB: f.db}
	query := reporting.Query{From: from, To: to, GroupBy: "game", Limit: 20}
	bettingReport, err := reports.Betting(ctx, f.brand, query)
	if err != nil {
		t.Fatal(err)
	}
	ledgerQuery := reporting.Query{From: from, To: to, GroupBy: "entry_type", Limit: 20}
	ledgerReport, err := reports.Ledger(ctx, f.brand, ledgerQuery)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := captureArchiveForBettingTest(t, f.db, reports, f.brand, from, to)
	if snapshot.Betting != bettingReport.Summary || snapshot.Ledger != ledgerReport.Summary {
		t.Fatalf("archive differs from actual reports: betting=%+v/%+v ledger=%+v/%+v", snapshot.Betting, bettingReport.Summary, snapshot.Ledger, ledgerReport.Summary)
	}
	if snapshot.Betting.OrderCount != "1" || snapshot.Betting.LostCount != "1" || snapshot.Betting.CurrentPrizePoints != "0" || snapshot.Betting.CorrectionOpenCount != "0" {
		t.Fatalf("archive did not retain the corrected final projection: %+v", snapshot.Betting)
	}
	if snapshot.Ledger.PrizeCreditPoints != "10" || snapshot.Ledger.PrizeReversalPoints != "10" {
		t.Fatalf("archive lost historical prize credit/reversal: %+v", snapshot.Ledger)
	}
}

func TestArchiveSnapshotCommissionKeepsManualPaymentAndAdjustment(t *testing.T) {
	f := newCommissionBatchFixture(t)
	ctx := context.Background()
	reports := reporting.Service{DB: f.betting.db}
	waitCommissionBoundary(t, f.boundary)
	cycle := createCommissionCycle(t, f)
	eligibilitySettle(t, f.betting)
	advanceCommissionWorker(t, f, 30)
	cycle = readCommissionCycle(t, f, cycle.ID)
	if cycle.State != "ready" || cycle.TotalPoints != "1" {
		t.Fatalf("fixture did not calculate the expected commission: %+v", cycle)
	}
	policy := commissionPaymentPolicy(t, f)
	if policy.Enabled {
		t.Fatalf("manual payout fixture unexpectedly enabled the payment gate: %+v", policy)
	}
	if err := updateCommissionPaymentPolicy(t, f, commissionPaymentActor(f), true, policy.Version); err != nil {
		t.Fatal(err)
	}
	if steps, err := f.service.ProcessPayments(ctx, 20); err != nil || steps == 0 {
		t.Fatalf("register actual manual payment: steps=%d err=%v", steps, err)
	}
	paymentPage := commissionPayments(t, f)
	if len(paymentPage.Items) != 1 || paymentPage.Items[0].State != "awaiting_approval" {
		t.Fatalf("manual payment was not staged for approval: %+v", paymentPage)
	}
	paid := payManualCommissionForAdjustment(t, f, paymentPage.Items[0])
	if paid.State != "paid" || paid.PaidPoints != "1" || paid.PaidCount != "1" {
		t.Fatalf("manual payout did not post its real credit: %+v", paid)
	}
	targets := commissionTargets(t, f, paid.ID)
	if len(targets.Items) != 1 || targets.Items[0].LedgerEntryID == nil || targets.Items[0].AdjustmentVersion == nil {
		t.Fatalf("paid commission target lacks its ledger binding: %+v", targets)
	}
	up, err := adjustCommission(t, f, f.betting.brand, targets.Items[0], commissionAdjustmentActor(f), *targets.Items[0].AdjustmentVersion, 3)
	if err != nil || up.DeltaPoints != 2 {
		t.Fatalf("actual commission adjustment was not +2: %+v err=%v", up, err)
	}

	from, to := time.Now().UTC().Add(-time.Hour), time.Now().UTC().Add(time.Hour)
	query := commissionReportQuery(time.Now().UTC(), "day")
	query.From, query.To = from, to
	commissionReport, err := reports.Commission(ctx, f.betting.brand, query)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := captureArchiveForBettingTest(t, f.betting.db, reports, f.betting.brand, from, to)
	if snapshot.Commissions != commissionReport.Summary {
		t.Fatalf("archive commission totals=%+v, live totals=%+v", snapshot.Commissions, commissionReport.Summary)
	}
	if snapshot.Commissions.EntryCount != "2" || snapshot.Commissions.PaidPoints != "1" || snapshot.Commissions.AdjustmentCreditPoints != "2" || snapshot.Commissions.NetPoints != "3" {
		t.Fatalf("archive omitted actual payment or adjustment history: %+v", snapshot.Commissions)
	}
}

func TestArchiveSnapshotCommissionCorrectionIncludesActualDebit(t *testing.T) {
	f, payment, plan := readyAutomaticCorrectionExecutionFixture(t)
	execution := ensureCorrectionExecution(t, f)
	if execution.State != commission.CorrectionExecutionCompleted || execution.AppliedDebitPoints != "1" || execution.AppliedCreditPoints != "0" {
		t.Fatalf("real correction execution did not post its debit: %+v", execution)
	}
	var oldRunState string
	if err := f.betting.db.QueryRow(context.Background(), `SELECT state FROM commission_runs WHERE brand_id=$1 AND id=$2`, f.betting.brand, payment.RunID).Scan(&oldRunState); err != nil {
		t.Fatal(err)
	}
	if oldRunState != "abandoned" || plan.RunID == payment.RunID {
		t.Fatalf("fixture did not preserve an actually stale original run: state=%q payment=%+v plan=%+v", oldRunState, payment, plan)
	}

	from, to := time.Now().UTC().Add(-time.Hour), time.Now().UTC().Add(time.Hour)
	reports := reporting.Service{DB: f.betting.db}
	query := commissionReportQuery(time.Now().UTC(), "day")
	query.From, query.To = from, to
	commissionReport, err := reports.Commission(context.Background(), f.betting.brand, query)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := captureArchiveForBettingTest(t, f.betting.db, reports, f.betting.brand, from, to)
	if snapshot.Commissions != commissionReport.Summary {
		t.Fatalf("archive commission totals=%+v, live totals=%+v", snapshot.Commissions, commissionReport.Summary)
	}
	if snapshot.Commissions.PaidPoints != "1" || snapshot.Commissions.CorrectionDebitPoints != "1" || snapshot.Commissions.CorrectionCreditPoints != "0" {
		t.Fatalf("archive failed to retain original paid credit and executed correction debit: %+v", snapshot.Commissions)
	}
}
