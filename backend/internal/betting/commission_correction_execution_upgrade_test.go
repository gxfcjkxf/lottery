package betting

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
)

func correctCommissionRunToZeroForUpgrade(t *testing.T, f commissionBatchFixture, cycleID, oldRunID string) commission.Cycle {
	t.Helper()
	ctx := context.Background()
	var latestMigration int
	var hasNotificationFacts bool
	if err := f.betting.db.QueryRow(ctx, `SELECT
 COALESCE((SELECT max(split_part(name,'_',1)::integer) FROM schema_migrations WHERE split_part(name,'_',1) ~ '^[0-9]+$'),0),
 to_regclass('draw_notification_publications') IS NOT NULL`).Scan(&latestMigration, &hasNotificationFacts); err != nil {
		t.Fatal(err)
	}
	if latestMigration >= 70 || hasNotificationFacts {
		return correctCommissionRunToZero(t, f, cycleID, oldRunID)
	}

	correction := createFixtureCorrection(t, f.betting, correctedDigits(0, 0, 0))
	if correction.State != "reversing" {
		t.Fatalf("historical correction started in state %q", correction.State)
	}
	var workerErr error
	for processed := int64(0); processed < correction.TargetCount; processed++ {
		var pending bool
		if err := f.betting.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM draw_correction_targets WHERE correction_id=$1 AND state='pending')`, correction.ID).Scan(&pending); err != nil {
			t.Fatal(err)
		}
		if !pending {
			break
		}
		steps, err := f.betting.service.ProcessCorrections(ctx, 1)
		if err != nil {
			workerErr = err
			break
		}
		if steps != 1 {
			workerErr = fmt.Errorf("reversal worker advanced %d targets with limit=1", steps)
			break
		}
	}
	correction, err := f.betting.service.Correction(ctx, f.betting.brand, correction.ID)
	var pending, failed int64
	if countErr := f.betting.db.QueryRow(ctx, `SELECT count(*) FILTER(WHERE state='pending'),count(*) FILTER(WHERE state='failed') FROM draw_correction_targets WHERE correction_id=$1`, correction.ID).Scan(&pending, &failed); countErr != nil {
		t.Fatal(countErr)
	}
	if err != nil || workerErr != nil || pending != 0 || failed != 0 || correction.State != "reversing" {
		t.Fatalf("historical correction reversal incomplete: correction=%+v pending=%d failed=%d worker_err=%v read_err=%v", correction, pending, failed, workerErr, err)
	}
	publishHistoricalCommissionCorrection(t, f, correction.ID)

	if _, err = f.betting.service.ProcessSettlements(ctx, 20); err != nil {
		t.Fatal(err)
	}
	advanceCommissionWorker(t, f, 40)
	cycle := readCommissionCycle(t, f, cycleID)
	if cycle.State != "ready" || !cycle.EvidenceCurrent || cycle.CurrentRunID == nil || *cycle.CurrentRunID == oldRunID || cycle.TotalPoints != "0" {
		t.Fatalf("historical draw correction did not create a current zero-earning run: %+v", cycle)
	}
	return cycle
}

// publishHistoricalCommissionCorrection reproduces the pre-0070 correction
// publication transaction. The ordinary correction worker handles the real
// reversal first; this fixture then creates the historical replacement
// generation without calling the post-0070 draw notification publisher.
func publishHistoricalCommissionCorrection(t *testing.T, f commissionBatchFixture, correctionID string) {
	t.Helper()
	ctx := context.Background()
	tx, err := f.betting.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var periodID string
	if err = tx.QueryRow(ctx, `SELECT period_id::text FROM draw_corrections WHERE brand_id=$1 AND id=$2`, f.betting.brand, correctionID).Scan(&periodID); err != nil {
		t.Fatal(err)
	}
	if err = lockCorrectionPeriod(ctx, tx, f.betting.brand, periodID); err != nil {
		t.Fatal(err)
	}
	correction, err := lockCorrection(ctx, tx, f.betting.brand, correctionID)
	if err != nil || correction.State != "reversing" {
		t.Fatalf("historical correction is not ready to publish: %+v err=%v", correction, err)
	}
	var unresolved bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM draw_correction_targets WHERE correction_id=$1 AND state IN ('pending','failed'))`, correctionID).Scan(&unresolved); err != nil {
		t.Fatal(err)
	}
	if unresolved {
		t.Fatal("historical correction still has unresolved reversal targets")
	}
	if correction.PreviousJobID == nil || correction.PolicyVersion == nil || correction.Mode == nil {
		t.Fatalf("historical correction lacks its original settlement snapshot: %+v", correction)
	}
	var periodVersion, generation int64
	var currentJob string
	if err = tx.QueryRow(ctx, `SELECT p.version,p.current_settlement_job_id::text,j.generation FROM periods p JOIN settlement_jobs j ON j.id=p.current_settlement_job_id WHERE p.id=$1`, correction.PeriodID).Scan(&periodVersion, &currentJob, &generation); err != nil {
		t.Fatal(err)
	}
	if currentJob != *correction.PreviousJobID || periodVersion != correction.PeriodVersion {
		t.Fatalf("historical correction publication head changed: job=%s period_version=%d correction=%+v", currentJob, periodVersion, correction)
	}
	var targetCount int64
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM bet_orders WHERE brand_id=$1 AND period_id=$2`, f.betting.brand, correction.PeriodID).Scan(&targetCount); err != nil {
		t.Fatal(err)
	}
	newJobID := ids.New()
	if _, err = tx.Exec(ctx, `INSERT INTO settlement_jobs(id,brand_id,game_id,period_id,draw_result_id,period_version,policy_version,mode,target_count,created_by,reason,generation,previous_job_id,correction_id)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, newJobID, correction.BrandID, correction.GameID, correction.PeriodID, correction.DrawResultID, periodVersion+1, *correction.PolicyVersion, *correction.Mode, targetCount, correction.CreatedBy, correction.Reason, generation+1, *correction.PreviousJobID, correction.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO settlement_targets(brand_id,job_id,period_id,order_id,state)
SELECT brand_id,$1,period_id,id,CASE WHEN status='placed' THEN 'pending' ELSE 'excluded' END FROM bet_orders WHERE brand_id=$2 AND period_id=$3`, newJobID, correction.BrandID, correction.PeriodID); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE draw_corrections SET new_job_id=$2,state='resettling',version=version+1 WHERE id=$1`, correction.ID, newJobID); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE periods SET current_settlement_job_id=$2,draw_result_id=$3,version=version+1,state_reason='corrected result published after original reversals' WHERE id=$1`, correction.PeriodID, newJobID, correction.DrawResultID); err != nil {
		t.Fatal(err)
	}
	correction.State = "resettling"
	correction.Version++
	correction.NewJobID = &newJobID
	if err = correctionEvent(ctx, tx, correction, "draw.correction.published"); err != nil {
		t.Fatal(err)
	}
	if _, err = audit.Append(ctx, tx, audit.Record{BrandID: correction.BrandID, ActorType: "system", Action: "draw.correction.publish", ResourceType: "draw_correction", ResourceID: correction.ID, Reason: "all original targets reversed or excluded", RequestID: "correction:" + correction.ID, After: map[string]any{"draw_result_id": correction.DrawResultID, "new_job_id": newJobID, "generation": generation + 1, "policy_version": correction.PolicyVersion}}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}
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
	correctCommissionRunToZeroForUpgrade(t, f, payment.CycleID, payment.RunID)
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
