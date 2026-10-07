package betting

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestSettlementJobLossFinalizesWithoutZeroValueLedger(t *testing.T) {
	selection := rules.Selection{Digits: [][]int{{1}, {2}, {2}}}
	f, o := drawnFixtureSelectedOrder(t, &selection)
	ctx := context.Background()
	a := settlementActor(f)
	mode := "automatic"
	p := setSettlementMode(t, f, a, 1, &mode)
	j := startFixtureSettlement(t, f, a, p.Version)
	before := walletBySource(t, f)
	if _, e := f.service.ProcessSettlements(ctx, 20); e != nil {
		t.Fatal(e)
	}
	j, e := f.service.SettlementJob(ctx, f.brand, j.ID)
	if e != nil || j.State != "completed" || j.PaidCount != 1 || j.PaidPoints != "0" {
		t.Fatal(j, e)
	}
	fresh, e := f.service.Order(ctx, f.brand, f.member, o.ID)
	if e != nil || fresh.Status != "lost" || fresh.SettlementCalculationID == nil || fresh.SettledAt == nil || fresh.PayoutEntryID != nil || fresh.PrizePoints != 0 || walletBySource(t, f) != before {
		t.Fatal(fresh, e)
	}
	var n int
	if e = f.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE entry_type='prize'`).Scan(&n); e != nil || n != 0 {
		t.Fatal(n, e)
	}
}

func TestSettlementJobRejectsIncompletePrizeLedgerEvenWithMatchingReferences(t *testing.T) {
	f, o := drawnFixtureOrder(t)
	ctx := context.Background()
	a := settlementActor(f)
	mode := "automatic"
	p := setSettlementMode(t, f, a, 1, &mode)
	j := startFixtureSettlement(t, f, a, p.Version)
	for range 2 {
		if _, e := f.service.ProcessSettlements(ctx, 1); e != nil {
			t.Fatal(e)
		}
	}
	var calc string
	if e := f.db.QueryRow(ctx, `SELECT id::text FROM settlement_calculations WHERE job_id=$1 AND order_id=$2`, j.ID, o.ID).Scan(&calc); e != nil {
		t.Fatal(e)
	}
	tx, e := f.db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	entry := ids.New()
	if _, e = tx.Exec(ctx, `SAVEPOINT malformed_prize_attempt`); e != nil {
		t.Fatal(e)
	}
	_, e = tx.Exec(ctx, `INSERT INTO point_ledger_entries(id,brand_id,account_id,member_id,version,entry_type,reference_type,reference_id,operation_key,before_snapshot,delta_snapshot,after_snapshot,source_allocation,reason,actor_type,request_id,request_hash)
 SELECT $1,brand_id,account_id,member_id,version+1,'prize','settlement_calculation',$2::uuid,'settlement-payout:'||$2::uuid::text,after_snapshot,'{}',after_snapshot,'[{"source":"winning","state":"available","points":"10"}]','test incomplete JSON','system','test-forged-prize',repeat('0',64) FROM point_ledger_entries WHERE id=$3`, entry, calc, o.DebitEntryID)
	var pgerr *pgconn.PgError
	if !errors.As(e, &pgerr) || pgerr.ConstraintName != "ledger_four_source_snapshot" {
		t.Fatalf("malformed historical-width prize row should be rejected by the 16-bucket insert guard, got %v", e)
	}
	// Preserve the original matching-reference regression as well. In this
	// owned test transaction only, bypass the strict snapshot INSERT guard long
	// enough to create a malformed row with the exact settlement reference. The
	// trigger is restored before attempting payout, and the transaction is
	// always rolled back, so this is not a production weakening or committed
	// evidence.
	if _, rollbackErr := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT malformed_prize_attempt`); rollbackErr != nil {
		t.Fatal(rollbackErr)
	}
	if _, e = tx.Exec(ctx, `SAVEPOINT malformed_prize_fixture`); e != nil {
		t.Fatal(e)
	}
	// Temporarily suppress the deferred projection trigger too, so PostgreSQL
	// has no queued trigger event preventing us from restoring the strict
	// BEFORE trigger immediately after the synthetic INSERT. The entire tx is
	// rolled back below; neither guard is disabled outside this test fixture.
	if _, e = tx.Exec(ctx, `ALTER TABLE point_ledger_entries DISABLE TRIGGER ledger_four_source_projection`); e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(ctx, `ALTER TABLE point_ledger_entries DISABLE TRIGGER ledger_four_source_snapshot`); e != nil {
		t.Fatal(e)
	}
	_, e = tx.Exec(ctx, `INSERT INTO point_ledger_entries(id,brand_id,account_id,member_id,version,entry_type,reference_type,reference_id,operation_key,before_snapshot,delta_snapshot,after_snapshot,source_allocation,reason,actor_type,request_id,request_hash)
 SELECT $1,brand_id,account_id,member_id,version+1,'prize','settlement_calculation',$2::uuid,'settlement-payout:'||$2::uuid::text,after_snapshot,'{}',after_snapshot,'[{"source":"winning","state":"available","points":"10"}]','test incomplete JSON','system','test-forged-prize',repeat('0',64) FROM point_ledger_entries WHERE id=$3`, entry, calc, o.DebitEntryID)
	if e != nil {
		if _, rollbackErr := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT malformed_prize_fixture`); rollbackErr != nil {
			t.Fatalf("malformed fixture insert failed (%v), then rollback failed: %v", e, rollbackErr)
		}
		t.Fatalf("could not install controlled matching-reference malformed row: %v", e)
	}
	if _, e = tx.Exec(ctx, `ALTER TABLE point_ledger_entries ENABLE TRIGGER ledger_four_source_snapshot`); e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(ctx, `ALTER TABLE point_ledger_entries ENABLE TRIGGER ledger_four_source_projection`); e != nil {
		t.Fatal(e)
	}
	_, e = tx.Exec(ctx, `UPDATE bet_orders SET status='won',version=version+1,settlement_calculation_id=$2,payout_entry_id=$3,prize_points=10,settled_at=clock_timestamp() WHERE id=$1`, o.ID, calc, entry)
	if !errors.As(e, &pgerr) || pgerr.Message != "incomplete settlement payout witness" {
		t.Fatalf("matching malformed prize row should fail at the original payout witness guard, got %v", e)
	}
	_ = tx.Rollback(ctx)
	if walletBySource(t, f)[1][0] != 0 {
		t.Fatal("rejected payout changed balance")
	}
	if _, e = f.service.ProcessSettlements(ctx, 20); e != nil {
		t.Fatal(e)
	}
}
func TestSettlementJobZeroOrderRequiresApprovalAndClosesPeriod(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	ctx := context.Background()
	a := settlementActor(f)
	at := time.Now().UTC().Add(-time.Hour)
	id, v := publicTestPeriod(t, f, f.game.ID, "zero-settlement", 100, at)
	publicManual(t, f, id, v, "zero-settlement", rules.Draw{Digits: []int{1, 2, 1}}, at)
	f.period.ID = id
	mode := "manual"
	p := setSettlementMode(t, f, a, 1, &mode)
	j := startFixtureSettlement(t, f, a, p.Version)
	if _, e := f.service.ProcessSettlements(ctx, 20); e != nil {
		t.Fatal(e)
	}
	j, e := f.service.SettlementJob(ctx, f.brand, j.ID)
	if e != nil || j.State != "awaiting_approval" || j.TargetCount != 0 {
		t.Fatal(j, e)
	}
	if _, e = actOnFixtureSettlement(t, f, a, j, "approve", j.Version); e != nil {
		t.Fatal(e)
	}
	if _, e = f.service.ProcessSettlements(ctx, 20); e != nil {
		t.Fatal(e)
	}
	j, e = f.service.SettlementJob(ctx, f.brand, j.ID)
	if e != nil || j.State != "completed" || j.PaidPoints != "0" {
		t.Fatal(j, e)
	}
}
func TestSettlementJobDatabaseCannotPrematurelyCloseOrRewriteEvidence(t *testing.T) {
	f, o := drawnFixtureOrder(t)
	ctx := context.Background()
	a := settlementActor(f)
	if _, e := f.db.Exec(ctx, `UPDATE periods SET status='settling',version=version+1,state_reason='no job' WHERE id=$1`, f.period.ID); e == nil {
		t.Fatal("unwitnessed period settlement")
	}
	mode := "automatic"
	p := setSettlementMode(t, f, a, 1, &mode)
	j := startFixtureSettlement(t, f, a, p.Version)
	// One calculation and one phase transition, but no payout yet.
	for range 2 {
		if _, e := f.service.ProcessSettlements(ctx, 1); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := f.db.Exec(ctx, `UPDATE periods SET status='settled',version=version+1,state_reason='premature' WHERE id=$1`, f.period.ID); e == nil {
		t.Fatal("closed unpaid period")
	}
	if _, e := f.db.Exec(ctx, `UPDATE settlement_jobs SET state='completed',completed_at=clock_timestamp(),version=version+1 WHERE id=$1`, j.ID); e == nil {
		t.Fatal("completed with ready targets")
	}
	if _, e := f.db.Exec(ctx, `UPDATE settlement_calculations SET prize_points=999 WHERE job_id=$1`, j.ID); e == nil {
		t.Fatal("mutable calculation")
	}
	if _, e := f.db.Exec(ctx, `DELETE FROM settlement_targets WHERE job_id=$1`, j.ID); e == nil {
		t.Fatal("deleted target")
	}
	if _, e := f.service.ProcessSettlements(ctx, 20); e != nil {
		t.Fatal(e)
	}
	fresh, e := f.service.Order(ctx, f.brand, f.member, o.ID)
	if e != nil {
		t.Fatal(e)
	}
	tx, e := f.db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	if _, e = f.service.CancelAdmin(ctx, tx, f.brand, a, o.ID, fresh.Version, "cannot undo without reversal", policyMeta(a.ID)); !errors.Is(e, ErrState) {
		t.Fatal(e)
	}
	_ = tx.Rollback(ctx)
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var n int
		return tx.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE entry_type='prize'`).Scan(&n)
	})
}
