package betting

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
)

// Reproduce the pre-0073 preparation transaction against the real 0072 guard.
// Payment, adjustment and draw correction use ordinary services; no financial
// trigger is disabled and no wallet/ledger success is fabricated.
func legacyManualPolicyPlan(t *testing.T, f commissionBatchFixture, pay commission.Payment, c commission.Cycle) string {
	t.Helper()
	ctx := context.Background()
	id, request := ids.New(), ids.New()
	const code = "COMMISSION_CORRECTION_MANUAL_POLICY_UNRESOLVED"
	const reason = "historical preparation awaits manual recalculation policy"
	err := commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
		log, err := audit.Append(ctx, tx, audit.Record{BrandID: f.betting.brand, ActorType: "system", Action: "commission.correction_plan.create", ResourceType: "commission_correction_plan", ResourceID: id, Reason: reason, RequestID: request,
			After: map[string]any{"version": int64(1), "state": "blocked", "planned_count": "0", "cursor_agent_id": nil, "error_code": code}})
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO commission_correction_plans(id,brand_id,cycle_id,payment_id,run_id,evidence_epoch,payout_mode,state,before_points,calculated_points,target_count,creation_audit_log_id,last_audit_log_id,last_error_code)
   SELECT $1,$2,$3,$4,$5,$6,'manual','blocked',coalesce(sum(points_before::numeric),0),coalesce(sum(points_after::numeric),0),count(*),$7,$7,$8 FROM commission_correction_candidates($2,$4,$5)`, id, f.betting.brand, pay.CycleID, pay.ID, *c.CurrentRunID, *c.EvidenceEpoch, log, code)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO commission_correction_plan_steps(id,brand_id,plan_id,version,to_state,operation,planned_count,error_code,actor_type,reason,request_id,audit_log_id) VALUES($1,$2,$3,1,'blocked','create',0,$4,'system',$5,$6,$7)`, ids.New(), f.betting.brand, id, code, reason, request, log)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestCommissionManualPolicy0072UpgradeRetainsBlockUntilExplicitRecovery(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := testdb.NewAtVersion(t, 72)
	f := newCommissionBatchFixtureFromBetting(t, newBettingFixtureWithDBWindow(t, db, storeTestBrand, 20*time.Second, 22*time.Second))
	pay := payManualCommissionForAdjustment(t, f, prepareManualCommissionPayment(t, f))
	target := commissionTargets(t, f, pay.ID).Items[0]
	adjustment, err := adjustCommission(t, f, f.betting.brand, target, commissionAdjustmentActor(f), 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	c := correctCommissionRunToZero(t, f, pay.CycleID, pay.RunID)
	if _, err = f.service.ProcessPayments(ctx, 20); err != nil {
		t.Fatal(err)
	}
	id := legacyManualPolicyPlan(t, f, pay, c)
	old := correctionPlanRead(t, f, id)
	before := correctionPlanSnapshot(t, f, pay.ID)
	var historyBefore, historyAfter string
	const historySQL = `SELECT jsonb_build_object('plans',(SELECT jsonb_agg(to_jsonb(p) ORDER BY id) FROM commission_correction_plans p),'steps',(SELECT jsonb_agg(to_jsonb(s) ORDER BY id) FROM commission_correction_plan_steps s),'adjustments',(SELECT jsonb_agg(to_jsonb(a) ORDER BY id) FROM commission_adjustments a),'audits',(SELECT jsonb_agg(to_jsonb(a) ORDER BY id) FROM audit_logs a))::text`
	if err = db.QueryRow(ctx, historySQL).Scan(&historyBefore); err != nil {
		t.Fatal(err)
	}
	var oidBefore, oidAfter uint32
	if err = db.QueryRow(ctx, `SELECT 'guard_commission_correction_plan()'::regprocedure::oid`).Scan(&oidBefore); err != nil {
		t.Fatal(err)
	}
	if err = database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, historySQL).Scan(&historyAfter); err != nil {
		t.Fatal(err)
	}
	if historyBefore != historyAfter || before != correctionPlanSnapshot(t, f, pay.ID) {
		t.Fatal("0073 changed existing funds, adjustment, plan or audit history")
	}
	if err = db.QueryRow(ctx, `SELECT 'guard_commission_correction_plan()'::regprocedure::oid`).Scan(&oidAfter); err != nil || oidAfter != oidBefore {
		t.Fatalf("guard identity changed: %d -> %d err=%v", oidBefore, oidAfter, err)
	}
	if _, err = f.service.ProcessCorrectionPlans(ctx, 20); err != nil {
		t.Fatal(err)
	}
	if got := correctionPlanRead(t, f, id); got.State != "blocked" || got.Version != old.Version || got.CreditPoints != nil || got.PlannedCount != "0" {
		t.Fatalf("upgrade/worker automatically recovered historical block: %+v", got)
	}
	actor := correctionPlanActor(f)
	retry := func(version int64) error {
		return commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
			_, err := f.service.RetryCorrectionPlanTx(ctx, tx, f.betting.brand, id, actor, commission.RetryCycleInput{Version: version, Reason: "explicitly accept confirmed recalculation policy; preparation only"}, commissionPaymentMeta(actor))
			return err
		})
	}
	if err = retry(old.Version + 1); !errors.Is(err, commission.ErrCorrectionPlanVersion) {
		t.Fatalf("wrong version recovery=%v", err)
	}
	gate := commissionPaymentPolicy(t, f)
	if err = updateCommissionPaymentPolicy(t, f, commissionPaymentActor(f), false, gate.Version); err != nil {
		t.Fatal(err)
	}
	if err = retry(old.Version); !errors.Is(err, commission.ErrCorrectionPlanState) {
		t.Fatalf("closed payment gate recovery=%v", err)
	}
	gate = commissionPaymentPolicy(t, f)
	if err = updateCommissionPaymentPolicy(t, f, commissionPaymentActor(f), true, gate.Version); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, actor.ID); err != nil {
		t.Fatal(err)
	}
	if err = retry(old.Version); !errors.Is(err, commission.ErrDenied) {
		t.Fatalf("actual super admin recovery=%v", err)
	}
	if _, err = db.Exec(ctx, `UPDATE admin_accounts SET is_super_admin=false WHERE id=$1`, actor.ID); err != nil {
		t.Fatal(err)
	}
	fundsBeforeRecovery := correctionPlanSnapshot(t, f, pay.ID)
	// Even an otherwise correctly audited recovery cannot substitute arbitrary
	// gross credit/debit amounts with the same net (-2).
	forged := commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
		request, reason := ids.New(), "owned forged recovery amount rejection"
		log, err := audit.Append(ctx, tx, audit.Record{BrandID: f.betting.brand, ActorType: "admin", ActorID: actor.ID, Action: "commission.correction_plan.retry", ResourceType: "commission_correction_plan", ResourceID: id, Reason: reason, RequestID: request,
			Before: map[string]any{"version": old.Version, "state": "blocked"}, After: map[string]any{"version": old.Version + 1, "state": "planning", "planned_count": "0", "cursor_agent_id": nil, "error_code": nil}})
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO commission_correction_plan_steps(id,brand_id,plan_id,version,from_state,to_state,operation,planned_count,actor_type,actor_id,reason,request_id,audit_log_id)
 VALUES($1,$2,$3,$4,'blocked','planning','retry',0,'admin',$5,$6,$7,$8)`, ids.New(), f.betting.brand, id, old.Version+1, actor.ID, reason, request, log)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE commission_correction_plans SET version=version+1,state='planning',last_error_code=NULL,last_audit_log_id=$2,credit_points=1,debit_points=3,net_points=-2 WHERE id=$1`, id, log)
		return err
	})
	if forged == nil || !strings.Contains(forged.Error(), "historical correction recovery requires unchanged frozen basis") {
		t.Fatalf("forged same-net recovery accepted or wrong guard: %v", forged)
	}
	if err = retry(old.Version); err != nil {
		t.Fatal(err)
	}
	recovered := correctionPlanRead(t, f, id)
	if recovered.State != "planning" || recovered.Version != old.Version+1 || recovered.CreationAuditLogID != old.CreationAuditLogID || recovered.BeforePoints != "2" || recovered.CalculatedPoints != "0" || recovered.NetPoints == nil || *recovered.NetPoints != "-2" {
		t.Fatalf("explicit recovery changed frozen basis/history: %+v", recovered)
	}
	if err = retry(old.Version); !errors.Is(err, commission.ErrCorrectionPlanVersion) {
		t.Fatalf("old version recovered twice: %v", err)
	}
	if _, err = f.service.ProcessCorrectionPlans(ctx, 20); err != nil {
		t.Fatal(err)
	}
	if got := correctionPlanRead(t, f, id); got.State != "ready" || got.NetPoints == nil || *got.NetPoints != "-2" {
		t.Fatalf("recovered plan not ready: %+v", got)
	}
	if fundsBeforeRecovery != correctionPlanSnapshot(t, f, pay.ID) {
		t.Fatal("explicit recovery/preparation moved funds")
	}
	var retained, retries int
	if err = db.QueryRow(ctx, `SELECT (SELECT count(*) FROM commission_adjustments WHERE id=$1 AND points_before=1 AND points_after=2),(SELECT count(*) FROM commission_correction_plan_steps WHERE plan_id=$2 AND operation='retry' AND from_state='blocked' AND to_state='planning' AND actor_id=$3)`, adjustment.ID, id, actor.ID).Scan(&retained, &retries); err != nil || retained != 1 || retries != 1 {
		t.Fatalf("history/explicit audit retained=%d retries=%d err=%v", retained, retries, err)
	}
	if policy := correctionExecutionPolicyRead(t, f); policy.Enabled {
		t.Fatal("migration/recovery opened execution gate")
	}
	if n, err := f.service.ProcessCorrectionExecutions(ctx, 20); err != nil || n != 0 {
		t.Fatalf("recovery ran financial work without gate: %d %v", n, err)
	}
	enableCorrectionExecutionPolicy(t, f)
	if _, err := f.service.ProcessCorrectionExecutions(ctx, 20); err != nil {
		t.Fatal(err)
	}
	executions := correctionExecutionsRead(t, f)
	if len(executions.Items) != 1 || executions.Items[0].State != commission.CorrectionExecutionAwaitingApproval || executions.Items[0].PlanID != id {
		t.Fatalf("recovered history bypassed fresh manual approval: %+v", executions)
	}
	x := executions.Items[0]
	if _, err := approveCorrectionExecution(t, f, x, correctionExecutionActor(f, "approve")); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ProcessCorrectionExecutions(ctx, 20); err != nil {
		t.Fatal(err)
	}
	if got := correctionExecutionRead(t, f, x.ID); got.State != commission.CorrectionExecutionCompleted || got.AppliedDebitPoints != "2" || got.AppliedCreditPoints != "0" {
		t.Fatalf("recovered historical difference did not execute: %+v", got)
	}
	if wallet := commissionWalletBySource(t, f, target.MemberID); wallet[3][0] != 0 {
		t.Fatalf("historical recovery did not target recalculated zero: %+v", wallet)
	}
	if original := commissionPaymentRead(t, f, pay.ID); original.State != "blocked" || original.PaidPoints != "1" {
		t.Fatalf("historical execution rewrote original payment: %+v", original)
	}
}
