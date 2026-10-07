package betting

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func commissionFact(t *testing.T, f bettingFixture, brand, order string) (commission.Resolution, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tx, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	return (commission.Source{}).FinalOrderTx(ctx, tx, brand, order)
}

func commissionFingerprint(t *testing.T, f bettingFixture) string {
	t.Helper()
	var value string
	err := f.db.QueryRow(context.Background(), `SELECT jsonb_build_object(
 'orders',(SELECT jsonb_agg(to_jsonb(o) ORDER BY id) FROM bet_orders o),
 'ledger',(SELECT jsonb_agg(to_jsonb(l) ORDER BY id) FROM point_ledger_entries l),
 'buckets',(SELECT jsonb_agg(to_jsonb(b) ORDER BY account_id,source,state) FROM point_buckets b),
 'jobs',(SELECT jsonb_agg(to_jsonb(j) ORDER BY id) FROM settlement_jobs j),
 'audit',(SELECT count(*) FROM audit_logs),
 'events',(SELECT count(*) FROM outbox_events))::text`).Scan(&value)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestCommissionFactsUseFinalWitnessesAllSourcesAndExcludeInvalidOrders(t *testing.T) {
	f := newBettingFixtureWithWindow(t, storeTestBrand, 5*time.Second, 5100*time.Millisecond)
	fundBettingWallet(t, f, 10, 10, 10)
	ctx := context.Background()
	in := f.input
	in.Multiplier = 25
	in.Selection = rules.Selection{Digits: [][]int{{0}, {0}, {0}}}
	lost, err := placeBettingOrder(t, f, in, "commission-valid-mixed-loss")
	if err != nil {
		t.Fatal(err)
	}
	won, err := placeBettingOrder(t, f, f.input, "commission-valid-win")
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := placeBettingOrder(t, f, f.input, "commission-cancelled")
	if err != nil {
		t.Fatal(err)
	}
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		_, err := f.service.CancelAdmin(ctx, tx, f.brand, judgeActor(f), cancelled.ID, cancelled.Version, "exclude refunded stake", policyMeta(f.version.CreatedBy))
		return err
	})
	abnormal, err := placeBettingOrder(t, f, f.input, "commission-abnormal")
	if err != nil {
		t.Fatal(err)
	}
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		_, err := f.service.MarkAbnormal(ctx, tx, f.brand, exceptionActor(f), abnormal.ID, abnormal.Version, "exclude invalid stake", policyMeta(f.version.CreatedBy))
		return err
	})
	judged, err := placeBettingOrder(t, f, f.input, "commission-judged")
	if err != nil {
		t.Fatal(err)
	}
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		_, err := f.service.JudgeCancel(ctx, tx, f.brand, judgeActor(f), judged.ID, JudgeCancelInput{Version: judged.Version, Cause: "no_result", Reason: "exclude judged cancelled stake"}, policyMeta(f.version.CreatedBy))
		return err
	})
	before := commissionFingerprint(t, f)
	for _, tt := range []struct{ order, reason string }{{lost.ID, "not_final"}, {cancelled.ID, "cancelled"}, {abnormal.ID, "abnormal"}, {judged.ID, "cancelled"}} {
		out, err := commissionFact(t, f, f.brand, tt.order)
		if err != nil || out.Reason != tt.reason || out.Fact != nil {
			t.Fatal(out, err)
		}
	}
	if commissionFingerprint(t, f) != before {
		t.Fatal("fact reads mutated pending business state")
	}
	eligibilitySettle(t, f)
	before = commissionFingerprint(t, f)
	for _, tt := range []struct {
		order string
		loss  int64
	}{{lost.ID, 25}, {won.ID, 0}} {
		out, err := commissionFact(t, f, f.brand, tt.order)
		if err != nil || out.Fact == nil || out.Reason != "final" {
			t.Fatal(out, err)
		}
		fact := out.Fact
		if int64(fact.Basis.LossPoints) != tt.loss || fact.JobID == "" || fact.CalculationID == "" || fact.Generation != 1 || fact.BetLedgerVersion <= 0 || fact.MemberID != f.member {
			t.Fatal(fact)
		}
		if tt.loss == 25 && (fact.Basis.TurnoverPoints != 25 || len(fact.Allocation) != 3) {
			t.Fatal("mixed source stake lost", fact)
		}
		if tt.loss == 0 && fact.Basis.TurnoverPoints != 1 {
			t.Fatal("winning stake excluded from turnover", fact)
		}
		var saved []byte
		if err := f.db.QueryRow(ctx, `SELECT attribution_snapshot FROM bet_orders WHERE id=$1`, tt.order).Scan(&saved); err != nil || !bytes.Equal(saved, fact.AttributionSnapshot) {
			t.Fatal("original attribution replaced", err)
		}
		public, err := json.Marshal(fact)
		if err != nil || bytes.Contains(public, []byte("commission_policy")) || bytes.Contains(public, []byte("member_attribution")) {
			t.Fatal("raw private attribution exposed", string(public), err)
		}
	}
	for _, order := range []string{cancelled.ID, abnormal.ID, judged.ID} {
		out, err := commissionFact(t, f, f.brand, order)
		if err != nil || out.Fact != nil {
			t.Fatal(out, err)
		}
	}
	for _, tt := range []struct{ brand, order string }{{storeTestOtherBrand, lost.ID}, {f.brand, ids.New()}} {
		_, err := commissionFact(t, f, tt.brand, tt.order)
		if !errors.Is(err, commission.ErrNotFound) {
			t.Fatal("scope leak", err)
		}
	}
	if commissionFingerprint(t, f) != before {
		t.Fatal("fact reads mutated final business state")
	}
}

func TestCommissionFactsFailClosedForMissingOrCorruptFinalWitnesses(t *testing.T) {
	f, order, _ := settledCorrectionFixture(t)
	ctx := context.Background()
	before := commissionFingerprint(t, f)
	// All fault injection is restricted to testdb.New's owned random schema.
	// DDL and changed evidence are in the same rolled-back transaction; no
	// production path disables witnesses or repairs facts from current config.
	for _, fault := range []struct{ trigger, statement, id string }{
		{"ALTER TABLE settlement_calculations DISABLE TRIGGER immutable_settlement_calculation", `UPDATE settlement_calculations SET draw_hash=repeat('0',64) WHERE id=$1`, *order.SettlementCalculationID},
		{"ALTER TABLE settlement_targets DISABLE TRIGGER guarded_settlement_target", `UPDATE settlement_targets SET state='excluded',calculation_id=NULL,payout_entry_id=NULL,version=version+1 WHERE order_id=$1`, order.ID},
		{"ALTER TABLE point_ledger_entries DISABLE TRIGGER ledger_immutable", `UPDATE point_ledger_entries SET delta_snapshot=jsonb_set(delta_snapshot,'{recharge,available}','"0"'::jsonb) WHERE id=$1`, order.DebitEntryID},
		{"ALTER TABLE point_ledger_entries DISABLE TRIGGER ledger_immutable", `UPDATE point_ledger_entries SET after_snapshot=jsonb_set(after_snapshot,'{winning,available}','"9"'::jsonb) WHERE id=$1`, *order.PayoutEntryID},
	} {
		func() {
			tx, err := f.db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			if _, err = tx.Exec(ctx, fault.trigger); err != nil {
				t.Fatal(err)
			}
			if _, err = tx.Exec(ctx, fault.statement, fault.id); err != nil {
				t.Fatal(err)
			}
			out, err := (commission.Source{}).FinalOrderTx(ctx, tx, f.brand, order.ID)
			if !errors.Is(err, commission.ErrEvidence) || out.Fact != nil {
				t.Fatal("incomplete final witness silently eligible", out, err)
			}
		}()
		if commissionFingerprint(t, f) != before {
			t.Fatal("test fault injection did not roll back")
		}
	}
	if _, err := commissionFact(t, f, "invalid-brand", order.ID); !errors.Is(err, commission.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := commissionFact(t, f, f.brand, "invalid-order"); !errors.Is(err, commission.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := (commission.Source{}).FinalOrderTx(ctx, nil, f.brand, order.ID); !errors.Is(err, commission.ErrInvalid) {
		t.Fatal(err)
	}
	out, err := commissionFact(t, f, f.brand, order.ID)
	if err != nil || out.Fact == nil {
		t.Fatal("fault rollback did not restore real witnesses", out, err)
	}
}

func TestCommissionFactLockKeepsGenerationStableUntilTransactionEnds(t *testing.T) {
	f, order, _ := settledCorrectionFixture(t)
	ctx := context.Background()
	c, err := f.service.CorrectionContext(ctx, f.brand, f.period.ID)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	out, err := (commission.Source{}).FinalOrderTx(ctx, tx, f.brand, order.ID)
	if err != nil || out.Fact == nil {
		t.Fatal(out, err)
	}
	before := commissionFingerprint(t, f)
	other, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Rollback(ctx)
	if _, err = other.Exec(ctx, `SET LOCAL lock_timeout='250ms'`); err != nil {
		t.Fatal(err)
	}
	_, err = f.service.CreateCorrection(ctx, other, f.brand, correctionActor(f), *c.DrawResultID, CorrectionInput{Version: c.PeriodVersion, PolicyVersion: &c.PolicyVersion, Result: correctedDigits(1, 2, 2), Reason: "must wait for commission fact transaction"}, policyMeta(f.version.CreatedBy))
	var blocked *pgconn.PgError
	if !errors.As(err, &blocked) || blocked.Code != "55P03" {
		t.Fatal("generation changed while fact was in use", err)
	}
	if err = other.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if commissionFingerprint(t, f) != before {
		t.Fatal("blocked correction left partial financial state")
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	created := createFixtureCorrection(t, f, correctedDigits(1, 2, 2))
	if created.State != "reversing" {
		t.Fatal("correction remained blocked after fact transaction ended", created)
	}
}

func TestCommissionFactsFollowCurrentGenerationAndFailBusyWithoutWrites(t *testing.T) {
	f, order, old := settledCorrectionFixture(t)
	ctx := context.Background()
	out, err := commissionFact(t, f, f.brand, order.ID)
	if err != nil || out.Fact == nil || out.Fact.JobID != old.ID || out.Fact.Basis.LossPoints != 0 {
		t.Fatal(out, err)
	}
	lock, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback(ctx)
	if _, err = lock.Exec(ctx, `SELECT id FROM periods WHERE id=$1 FOR UPDATE`, f.period.ID); err != nil {
		t.Fatal(err)
	}
	before := commissionFingerprint(t, f)
	_, err = commissionFact(t, f, f.brand, order.ID)
	if !errors.Is(err, commission.ErrBusy) {
		t.Fatal("period contention must fail without waiting", err)
	}
	if commissionFingerprint(t, f) != before {
		t.Fatal("busy fact read mutated business state")
	}
	if err = lock.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	correction := createFixtureCorrection(t, f, correctedDigits(1, 2, 2))
	out, err = commissionFact(t, f, f.brand, order.ID)
	if err != nil || out.Fact != nil || out.Reason != "correction_open" {
		t.Fatal("old calculation counted during correction", out, err)
	}
	if _, err = f.service.ProcessCorrections(ctx, 20); err != nil {
		t.Fatal(err)
	}
	out, err = commissionFact(t, f, f.brand, order.ID)
	if err != nil || out.Fact != nil || out.Reason != "correction_open" {
		t.Fatal("partial correction counted", out, err)
	}
	if _, err = f.service.ProcessSettlements(ctx, 20); err != nil {
		t.Fatal(err)
	}
	before = commissionFingerprint(t, f)
	out, err = commissionFact(t, f, f.brand, order.ID)
	if err != nil || out.Fact == nil || out.Fact.Generation != 2 || out.Fact.JobID == old.ID || out.Fact.CalculationID == *order.SettlementCalculationID || out.Fact.Basis.LossPoints != 1 {
		t.Fatal(out, err)
	}
	if commissionFingerprint(t, f) != before {
		t.Fatal("new generation read created another commission or posting")
	}
	var state string
	if err = f.db.QueryRow(ctx, `SELECT state FROM draw_corrections WHERE id=$1`, correction.ID).Scan(&state); err != nil || state != "completed" {
		t.Fatal(state, err)
	}
}
