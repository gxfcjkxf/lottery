package betting

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

func TestCommissionCorrectionPlanFailedPageRetriesWithCurrentActorAndNoMoney(t *testing.T) {
	t.Parallel()
	f, payment, _ := createAndPayAutomaticCorrectionFixture(t)
	ctx := context.Background()
	correctCommissionRunToZero(t, f, payment.CycleID, payment.RunID)
	if _, err := f.service.ProcessPayments(ctx, 20); err != nil {
		t.Fatal(err)
	}
	if n, err := f.service.ProcessCorrectionPlans(ctx, 1); err != nil || n != 1 {
		t.Fatalf("create plan: %d %v", n, err)
	}
	plan := correctionPlansRead(t, f).Items[0]
	before := correctionPlanSnapshot(t, f, payment.ID)
	if _, err := f.betting.db.Exec(ctx, `CREATE FUNCTION fail_owned_plan_page() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'owned page failure'; END $$;
 CREATE TRIGGER fail_owned_plan_page BEFORE INSERT ON commission_correction_plan_targets FOR EACH ROW EXECUTE FUNCTION fail_owned_plan_page()`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ProcessCorrectionPlans(ctx, 1); err != nil {
		t.Fatalf("record page failure: %v", err)
	}
	failed := correctionPlanRead(t, f, plan.ID)
	if failed.State != "failed" || failed.Version != 2 || failed.PlannedCount != "0" || correctionPlanTargetsRead(t, f, plan.ID).TotalCount != "0" {
		t.Fatalf("failed page left partial targets/steps: %+v", failed)
	}
	if after := correctionPlanSnapshot(t, f, payment.ID); after != before {
		t.Fatal("failed metadata page changed original financial facts")
	}
	if _, err := f.betting.db.Exec(ctx, `DROP TRIGGER fail_owned_plan_page ON commission_correction_plan_targets; DROP FUNCTION fail_owned_plan_page()`); err != nil {
		t.Fatal(err)
	}
	actor := correctionPlanActor(f)
	retry := func(version int64) error {
		return commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
			_, err := f.service.RetryCorrectionPlanTx(ctx, tx, f.betting.brand, plan.ID, actor,
				commission.RetryCycleInput{Version: version, Reason: "retry repaired preparation only; no compensation authorization"}, commissionPaymentMeta(actor))
			return err
		})
	}
	if err := retry(1); !errors.Is(err, commission.ErrCorrectionPlanVersion) {
		t.Fatalf("old version retry=%v", err)
	}
	// A Go caller's cached ordinary status cannot mask the actual current flag.
	if _, err := f.betting.db.Exec(ctx, `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, actor.ID); err != nil {
		t.Fatal(err)
	}
	if err := retry(failed.Version); !errors.Is(err, commission.ErrDenied) {
		t.Fatalf("cached ordinary actor bypassed database super-admin flag: %v", err)
	}
	if _, err := f.betting.db.Exec(ctx, `UPDATE admin_accounts SET is_super_admin=false,status='disabled' WHERE id=$1`, actor.ID); err != nil {
		t.Fatal(err)
	}
	if err := retry(failed.Version); !errors.Is(err, commission.ErrDenied) {
		t.Fatalf("disabled actor retried plan: %v", err)
	}
	if _, err := f.betting.db.Exec(ctx, `UPDATE admin_accounts SET status='active' WHERE id=$1`, actor.ID); err != nil {
		t.Fatal(err)
	}
	if err := retry(failed.Version); err != nil {
		t.Fatalf("explicit corrected retry: %v", err)
	}
	if err := retry(failed.Version); !errors.Is(err, commission.ErrCorrectionPlanVersion) {
		t.Fatalf("same version applied a second retry: %v", err)
	}
	if _, err := f.service.ProcessCorrectionPlans(ctx, 20); err != nil {
		t.Fatal(err)
	}
	ready := correctionPlanRead(t, f, plan.ID)
	if ready.State != "ready" || ready.Version != 5 || ready.PlannedCount != "1" || ready.NetPoints == nil || *ready.NetPoints != "-1" {
		t.Fatalf("retry lost original frozen amounts or page progress: %+v", ready)
	}
	for _, labels := range [][2]string{
		{"commission_correction", "manual"},
		{"adjustment", "commission_correction_plan"},
		{"adjustment", "commission_correction_plan_target"},
		{"adjustment", "commission_correction_target"},
	} {
		tx, err := f.betting.db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var delta points.Balance
		delta[3][0] = 1
		_, err = (points.Store{DB: f.betting.db}).Post(ctx, tx, points.Change{BrandID: f.betting.brand, MemberID: f.node.MemberID,
			EntryType: labels[0], ReferenceType: labels[1], ReferenceID: plan.ID, OperationKey: "owned-fake-plan:" + ids.New(),
			Reason: "a prepared plan is not financial authorization", ActorType: "system", RequestID: ids.New(), Delta: delta,
			Allocation: []points.Allocation{{Source: "commission", State: "available", Points: 1}}})
		_ = tx.Rollback(ctx)
		if err == nil || !strings.Contains(err.Error(), "prepared correction plans cannot authorize financial ledger postings") {
			t.Fatalf("financial label %v forged a prepared-plan credit: %v", labels, err)
		}
	}
	if after := correctionPlanSnapshot(t, f, payment.ID); after != before {
		t.Fatal("successful preparation retry changed financial facts")
	}
}
