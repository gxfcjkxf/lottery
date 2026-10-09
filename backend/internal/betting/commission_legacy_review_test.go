package betting

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/commissionreview"
	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/reporting"
	"github.com/jackc/pgx/v5"
)

func TestCommissionLegacyReviewIncludesStaleManualMoneyAndLaterActualPayment(t *testing.T) {
	f, pay, target, adj := zeroOriginalManualOnlyFixture(t)
	ctx, db := context.Background(), f.betting.db
	next := createFixtureCorrection(t, f.betting, correctedDigits(1, 2, 2))
	if _, err := f.betting.service.ProcessCorrections(ctx, 20); err != nil {
		t.Fatal(err)
	}
	if got, err := f.betting.service.Correction(ctx, f.betting.brand, next.ID); err != nil || got.NewJobID == nil {
		t.Fatalf("genuine replacement: %+v %v", got, err)
	}
	if _, err := f.betting.service.ProcessSettlements(ctx, 20); err != nil {
		t.Fatal(err)
	}
	advanceCommissionWorker(t, f, 40)
	if err := commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
		log, err := audit.Append(ctx, tx, audit.Record{BrandID: f.betting.brand, ActorType: "system", Action: "commission.payment.invalidate", ResourceType: "commission_payment", ResourceID: pay.ID, Reason: "reproduce historical zero-original invalidation", RequestID: ids.New(), Before: map[string]any{"version": pay.Version, "state": pay.State}, After: map[string]any{"version": pay.Version + 1, "state": "stale"}})
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE commission_payments SET state='stale',version=version+1,last_error_code=NULL,last_audit_log_id=$2 WHERE id=$1`, pay.ID, log)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, db); err == nil || !strings.Contains(err.Error(), "stale commission payment has actual money") {
		t.Fatalf("ordinary migration bypassed historical refusal: %v", err)
	}
	before := correctionPlanSnapshot(t, f, pay.ID)
	manualBefore := commissionManualPolicyAdjustmentEvidence(t, f, adj)
	if err := database.PrepareCommissionHistoryReview(ctx, db); err != nil {
		t.Fatal(err)
	}
	if before != correctionPlanSnapshot(t, f, pay.ID) || manualBefore != commissionManualPolicyAdjustmentEvidence(t, f, adj) {
		t.Fatal("review checkpoint rewrote actual money or historical payment/adjustment/audit")
	}
	if err := database.CheckMigrations(ctx, db); err == nil {
		t.Fatal("partial review checkpoint admitted application runtime")
	}
	assertReport := func(paymentCount, actual, difference string) {
		t.Helper()
		before := correctionPlanSnapshot(t, f, pay.ID)
		wallet := commissionWalletBySource(t, f, target.MemberID)
		report, err := commissionreview.Inspect(ctx, db, f.betting.brand, pay.CycleID)
		if err != nil {
			t.Fatal(err)
		}
		if !report.ReviewOnly || report.ExecutionAuthorized || report.PaymentCount != paymentCount || report.StaleMoneyPaymentCount != "1" || report.Analysis.Summary.ActualNetPoints != actual || report.Analysis.Summary.CalculatedPoints == nil || *report.Analysis.Summary.CalculatedPoints != "1" || report.Analysis.Summary.EffectiveMinusActualPoints == nil || *report.Analysis.Summary.EffectiveMinusActualPoints != difference {
			t.Fatalf("offline report lost full actual history: %+v", report)
		}
		if len(report.Analysis.Items) != 1 || report.Analysis.Items[0].Totals.ActualNetPoints != actual || report.Analysis.Items[0].Key != target.AgentID {
			t.Fatalf("offline report lost saved beneficiary: %+v", report.Analysis)
		}
		if before != correctionPlanSnapshot(t, f, pay.ID) || wallet != commissionWalletBySource(t, f, target.MemberID) || manualBefore != commissionManualPolicyAdjustmentEvidence(t, f, adj) {
			t.Fatal("offline review mutated funds or historical evidence")
		}
		if strconv.Itoa(len(report.Payments)) != paymentCount || report.Payments[0].ID != pay.ID || report.Payments[0].State != "stale" || !report.Payments[0].HasMoneyEvidence {
			t.Fatalf("original stale manual-money payment missing: %+v", report.Payments)
		}
	}
	assertReport("1", "1", "0")
	// Simulate subsequent old-library financial work only in this owned schema.
	// The actual platform executable refuses to start at the 0074 checkpoint.
	if _, err := f.service.ProcessPayments(ctx, 20); err != nil {
		t.Fatal(err)
	}
	var second commission.Payment
	for _, p := range commissionPayments(t, f).Items {
		if p.ID != pay.ID {
			second = p
		}
	}
	if second.ID == "" || second.State != "awaiting_approval" {
		t.Fatalf("real replacement payout missing: %+v", second)
	}
	assertReport("2", "1", "0")
	paid := payManualCommissionForAdjustment(t, f, second)
	if paid.PaidPoints != "1" || paid.State != "paid" {
		t.Fatalf("replacement real payment=%+v", paid)
	}
	assertReport("2", "2", "-1")
	// A reverse reference to a zero original target is not actual commission
	// evidence. Corrupt only the existing synthetic funding row's reference in
	// a rollback-only transaction; retain all real amounts and restore guards.
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SET LOCAL session_replication_role=replica`); err != nil {
		t.Fatal(err)
	}
	tag, err := tx.Exec(ctx, `UPDATE point_ledger_entries SET reference_type='commission_payment_target',reference_id=$2 WHERE id=(SELECT id FROM point_ledger_entries WHERE brand_id=$1 AND reference_type='test_fund' ORDER BY id LIMIT 1)`, f.betting.brand, target.ID)
	if err != nil || tag.RowsAffected() != 1 {
		t.Fatalf("owned reverse-reference corruption=%d %v", tag.RowsAffected(), err)
	}
	if _, err = tx.Exec(ctx, `SET LOCAL session_replication_role=origin`); err != nil {
		t.Fatal(err)
	}
	c := readCommissionCycle(t, f, pay.CycleID)
	q := reporting.CommissionAnalysisQuery{From: c.WindowTo, To: c.WindowTo.Add(time.Microsecond), GroupBy: "agent", Limit: 100, CycleID: &pay.CycleID}
	for _, emptySelection := range []bool{false, true} {
		probe := q
		if emptySelection {
			probe.From = c.WindowTo.Add(time.Second)
			probe.To = probe.From.Add(time.Microsecond)
		}
		if _, err = (reporting.Service{}).CommissionAnalysisRead(ctx, tx, f.betting.brand, probe); !errors.Is(err, reporting.ErrAnalysisIntegrity) {
			t.Fatalf("unwitnessed reverse binding to zero original target accepted with empty selection=%v: %v", emptySelection, err)
		}
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	assertReport("2", "2", "-1")
	if err := database.Migrate(ctx, db); err == nil || !strings.Contains(err.Error(), "stale commission payment has actual money") {
		t.Fatalf("review silently authorized normal upgrade: %v", err)
	}
	// Unknown metadata must fail before releasing even the previously valid report.
	if _, err := db.Exec(ctx, `UPDATE schema_migrations SET checksum='tampered' WHERE name='0074_commission_analysis_evidence.up.sql'`); err != nil {
		t.Fatal(err)
	}
	report, err := commissionreview.Inspect(ctx, db, f.betting.brand, pay.CycleID)
	if err == nil || !reflect.DeepEqual(report, commissionreview.Report{}) {
		t.Fatalf("bad metadata released partial evidence: %+v %v", report, err)
	}
}
