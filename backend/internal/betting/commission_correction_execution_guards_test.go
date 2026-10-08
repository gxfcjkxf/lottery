package betting

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/jackc/pgx/v5"
)

func TestCommissionCorrectionExecutionCurrentActorVersionAndHeadFailureRollback(t *testing.T) {
	t.Parallel()
	f, _, _ := readyAutomaticCorrectionExecutionFixture(t)
	ctx := context.Background()
	postCorrectionExecutionCommissionFreeze(t, f, 1, true)
	if _, err := f.service.ProcessCorrectionExecutions(ctx, 10); err != nil {
		t.Fatal(err)
	}
	x := correctionExecutionsRead(t, f).Items[0]
	if x.State != commission.CorrectionExecutionPaused || !x.CycleHoldActive {
		t.Fatalf("real insufficient cycle not paused: %+v", x)
	}
	a := correctionExecutionActor(f, "continue")
	// Cached ordinary permissions cannot override the current administrator.
	if _, err := f.betting.db.Exec(ctx, `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := continueCorrectionExecution(t, f, x, a); !errors.Is(err, commission.ErrDenied) {
		t.Fatalf("current super admin continued through cached ordinary identity: %v", err)
	}
	if _, err := f.betting.db.Exec(ctx, `UPDATE admin_accounts SET is_super_admin=false,status='disabled' WHERE id=$1`, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := continueCorrectionExecution(t, f, x, a); !errors.Is(err, commission.ErrDenied) {
		t.Fatalf("current disabled admin continued through cached active identity: %v", err)
	}
	if _, err := f.betting.db.Exec(ctx, `UPDATE admin_accounts SET status='active' WHERE id=$1`, a.ID); err != nil {
		t.Fatal(err)
	}
	wrong := x
	wrong.Version++
	if _, err := continueCorrectionExecution(t, f, wrong, a); !errors.Is(err, commission.ErrCorrectionExecutionVersion) {
		t.Fatalf("future version continued paused cycle: %v", err)
	}
	if got := correctionExecutionRead(t, f, x.ID); got.Version != x.Version || !got.CycleHoldActive {
		t.Fatalf("denied operations changed job/hold: %+v", got)
	}
	postCorrectionExecutionCommissionFreeze(t, f, 1, false)
	if _, err := continueCorrectionExecution(t, f, x, a); err != nil {
		t.Fatal(err)
	}
	beforeBuckets, beforePostings := correctionExecBuckets(t, f), correctionExecutionPostingRead(t, f)
	if _, err := f.betting.db.Exec(ctx, `CREATE FUNCTION test_reject_correction_head() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test actual head outage'; END $$;
 CREATE TRIGGER test_reject_correction_head BEFORE INSERT OR UPDATE ON commission_correction_balance_heads FOR EACH ROW EXECUTE FUNCTION test_reject_correction_head()`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ProcessCorrectionExecutions(ctx, 10); err != nil {
		t.Fatal(err)
	}
	if _, err := f.betting.db.Exec(ctx, `DROP TRIGGER test_reject_correction_head ON commission_correction_balance_heads; DROP FUNCTION test_reject_correction_head()`); err != nil {
		t.Fatal(err)
	}
	failed := correctionExecutionRead(t, f, x.ID)
	if failed.State != commission.CorrectionExecutionFailed || failed.AppliedCount != "0" || failed.CycleHoldActive {
		t.Fatalf("head outage did not persist technical failure after explicit release: %+v", failed)
	}
	if got := correctionExecutionPostingRead(t, f); !reflect.DeepEqual(got, beforePostings) {
		t.Fatalf("head outage retained partial ledger or head: before=%+v after=%+v", beforePostings, got)
	}
	assertCorrectionExecOnlyCommissionBucketDelta(t, beforeBuckets, correctionExecBuckets(t, f), f.node.MemberID, 0)
	if targets := correctionExecutionTargetsRead(t, f, x.ID); len(targets.Items) != 0 {
		t.Fatalf("head failure committed an orphan target: %+v", targets)
	}
	if _, err := retryCorrectionExecution(t, f, failed, correctionExecutionActor(f, "execute_retry")); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ProcessCorrectionExecutions(ctx, 10); err != nil {
		t.Fatal(err)
	}
	if final := correctionExecutionRead(t, f, x.ID); final.State != commission.CorrectionExecutionCompleted || final.AppliedDebitPoints != "1" {
		t.Fatalf("head failure retry duplicated or omitted debit: %+v", final)
	}
	assertCorrectionExecOnlyCommissionBucketDelta(t, beforeBuckets, correctionExecBuckets(t, f), f.node.MemberID, -1)
	// Brand-scoped reads must not reveal the job or its targets through another
	// tenant even when the caller knows the UUID.
	err := commissionPaymentCallTx(t, f, func(tx pgx.Tx) error { _, err := f.service.CorrectionExecutionTx(ctx, tx, ids.New(), x.ID); return err })
	if !errors.Is(err, commission.ErrNotFound) {
		t.Fatalf("cross-brand execution leaked: %v", err)
	}
	err = commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
		_, err := f.service.CorrectionExecutionTargetsTx(ctx, tx, ids.New(), x.ID, 20, 0)
		return err
	})
	if !errors.Is(err, commission.ErrNotFound) {
		t.Fatalf("cross-brand target list leaked: %v", err)
	}
}
