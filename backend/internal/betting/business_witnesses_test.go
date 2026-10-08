package betting

import (
	"context"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/finance"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

type lotteryBusinessWitness struct {
	family, sourceID, ledgerID, entryType string
	valid                                 bool
}

func readLotteryBusinessWitnesses(t *testing.T, f bettingFixture) []lotteryBusinessWitness {
	t.Helper()
	ctx := context.Background()
	var account string
	if err := f.db.QueryRow(ctx, `SELECT id::text FROM point_accounts WHERE brand_id=$1 AND brand_member_id=$2`, f.brand, f.member).Scan(&account); err != nil {
		t.Fatal(err)
	}
	rows, err := f.db.Query(ctx, `SELECT family,source_id::text,COALESCE(ledger_id::text,''),source_valid,expected_entry_type
		FROM point_lottery_business_witnesses($1,$2)`, f.brand, account)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []lotteryBusinessWitness
	for rows.Next() {
		var w lotteryBusinessWitness
		if err = rows.Scan(&w.family, &w.sourceID, &w.ledgerID, &w.valid, &w.entryType); err != nil {
			t.Fatal(err)
		}
		out = append(out, w)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func requireLotteryWitness(t *testing.T, witnesses []lotteryBusinessWitness, family, sourceID, ledgerID, entryType string) {
	t.Helper()
	for _, w := range witnesses {
		if w.family == family && w.sourceID == sourceID && w.ledgerID == ledgerID && w.entryType == entryType {
			if !w.valid {
				t.Fatalf("invalid %s witness for source %s: %+v", family, sourceID, w)
			}
			return
		}
	}
	t.Fatalf("missing %s witness for source %s and ledger %s", family, sourceID, ledgerID)
}

func TestLotteryBusinessWitnessesFollowRechargeBetAndRefundWorkflows(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	ctx := context.Background()
	adminID := f.version.CreatedBy
	financeService := finance.Service{DB: f.db, Points: points.Store{DB: f.db}}
	meta := points.Metadata{ActorType: "admin", ActorID: adminID, RequestID: ids.New()}
	var recharge finance.Recharge
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		created, err := financeService.CreateRecharge(ctx, tx, f.brand, f.member, 20, "business witness proof", "", "confirmed for witness test", meta)
		if err != nil {
			return err
		}
		recharge, err = financeService.ConfirmRecharge(ctx, tx, f.brand, created.ID, created.Version, "verified for witness test", meta)
		return err
	})
	setUserCancellation(t, f, true)
	versions := readBettingPolicyVersions(t, f.service, f.brand, f.game.ID)
	f.input.PolicyVersions = &versions
	order, err := placeBettingOrder(t, f, f.input, "business-witness-bet-key")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := f.service.Cancel(ctx, tx, f.brand, f.user, order.ID, order.Version, "cancel for witness test", points.Metadata{RequestID: ids.New()})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	witnesses := readLotteryBusinessWitnesses(t, f)
	requireLotteryWitness(t, witnesses, "recharge", recharge.ID, recharge.LedgerEntryID, "recharge")
	requireLotteryWitness(t, witnesses, "bet", order.ID, order.DebitEntryID, "bet")
	requireLotteryWitness(t, witnesses, "refund", order.ID, cancelled.RefundEntryID, "refund")
	var businessConsistent bool
	if err = f.db.QueryRow(ctx, `SELECT (point_business_preview($1,(SELECT account_id FROM bet_orders WHERE id=$2))->>'consistent')::boolean`, f.brand, order.ID).Scan(&businessConsistent); err != nil || !businessConsistent {
		t.Fatalf("full kernel rejected actual recharge/bet/refund history: consistent=%v err=%v", businessConsistent, err)
	}

	var volatility, functionPath, appSchema string
	if err = f.db.QueryRow(ctx, `SELECT current_schema()`).Scan(&appSchema); err != nil {
		t.Fatal(err)
	}
	if err = f.db.QueryRow(ctx, `SELECT p.provolatile::text,array_to_string(p.proconfig,',') FROM pg_proc p
		WHERE p.oid='point_lottery_business_witnesses(uuid,uuid)'::regprocedure`).Scan(&volatility, &functionPath); err != nil {
		t.Fatal(err)
	}
	if volatility != "s" || !strings.Contains(functionPath, "search_path=pg_catalog, "+appSchema+", pg_temp") || strings.Contains(functionPath, "public") {
		t.Fatalf("witness function must be STABLE with a pinned application search_path: volatility=%q path=%q", volatility, functionPath)
	}

	// Break only the saved debit pointer after creating authentic history. The
	// fixture owns this random schema; disabling its update guard is transaction
	// local, and rollback restores the original business witness.
	tx, err = f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `ALTER TABLE bet_orders DISABLE TRIGGER immutable_bet_order`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE bet_orders SET debit_entry_id=$2 WHERE brand_id=$1 AND id=$3`, f.brand, recharge.LedgerEntryID, order.ID); err != nil {
		t.Fatal(err)
	}
	var corruptedValid bool
	if err = tx.QueryRow(ctx, `SELECT source_valid FROM point_lottery_business_witnesses($1,
		(SELECT account_id FROM bet_orders WHERE brand_id=$1 AND id=$2)) WHERE family='bet' AND source_id=$2`, f.brand, order.ID).Scan(&corruptedValid); err != nil {
		t.Fatal(err)
	}
	if corruptedValid {
		t.Fatal("witness accepted a same-account ledger pointer from another business source")
	}
}

func TestLotteryBusinessWitnessesPreserveSupersededPaidPrize(t *testing.T) {
	f, oldOrder, _ := settledCorrectionFixture(t)
	ctx := context.Background()
	correction := createFixtureCorrection(t, f, correctedDigits(1, 2, 2))
	if _, err := f.service.ProcessCorrections(ctx, 20); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ProcessSettlements(ctx, 20); err != nil {
		t.Fatal(err)
	}
	correction, err := f.service.Correction(ctx, f.brand, correction.ID)
	if err != nil || correction.State != "completed" {
		t.Fatalf("correction did not complete: %+v %v", correction, err)
	}
	currentOrder, err := f.service.Order(ctx, f.brand, f.member, oldOrder.ID)
	if err != nil || currentOrder.SettlementCalculationID == nil || *currentOrder.SettlementCalculationID == *oldOrder.SettlementCalculationID {
		t.Fatalf("fixture did not supersede the order's settlement pointer: %+v %v", currentOrder, err)
	}
	var reversalID string
	if err := f.db.QueryRow(ctx, `SELECT t.reversal_entry_id::text
		FROM draw_corrections c JOIN draw_correction_targets t ON t.correction_id=c.id
		WHERE c.brand_id=$1 AND t.order_id=$2 AND t.state='reversed'`, f.brand, oldOrder.ID).Scan(&reversalID); err != nil {
		t.Fatal(err)
	}
	witnesses := readLotteryBusinessWitnesses(t, f)
	requireLotteryWitness(t, witnesses, "bet", oldOrder.ID, oldOrder.DebitEntryID, "bet")
	requireLotteryWitness(t, witnesses, "prize", *oldOrder.SettlementCalculationID, *oldOrder.PayoutEntryID, "prize")
	requireLotteryWitness(t, witnesses, "prize_reversal", oldOrder.ID, reversalID, "prize_reversal")
	var businessConsistent bool
	if err = f.db.QueryRow(ctx, `SELECT (point_business_preview($1,(SELECT account_id FROM bet_orders WHERE id=$2))->>'consistent')::boolean`, f.brand, oldOrder.ID).Scan(&businessConsistent); err != nil || !businessConsistent {
		t.Fatalf("full kernel rejected superseded actual prize and reversal: consistent=%v err=%v", businessConsistent, err)
	}

	// Both saved pointers missing must not hide a previously paid business
	// edge. This damage is confined to a rolled-back owned-schema transaction.
	tx, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `ALTER TABLE settlement_targets DISABLE TRIGGER guarded_settlement_target`); err != nil {
		t.Fatal(err)
	}
	var checkName string
	if err = tx.QueryRow(ctx, `SELECT conname FROM pg_constraint WHERE conrelid='settlement_targets'::regclass AND contype='c' AND pg_get_constraintdef(oid) LIKE '%calculation_id IS NOT NULL%'`).Scan(&checkName); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `ALTER TABLE settlement_targets DROP CONSTRAINT `+pgx.Identifier{checkName}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE settlement_targets SET calculation_id=NULL,payout_entry_id=NULL WHERE brand_id=$1 AND order_id=$2 AND calculation_id=$3`, f.brand, oldOrder.ID, *oldOrder.SettlementCalculationID); err != nil {
		t.Fatal(err)
	}
	var required, missing, invalid bool
	if err = tx.QueryRow(ctx, `SELECT count(*)=1,bool_and(ledger_id IS NULL),bool_and(NOT source_valid) FROM point_lottery_business_witnesses($1,(SELECT account_id FROM bet_orders WHERE id=$2)) WHERE family='prize' AND source_id=$2`, f.brand, oldOrder.ID).Scan(&required, &missing, &invalid); err != nil || !required || !missing || !invalid {
		t.Fatalf("missing both pointers hid required prize edge: required=%v missing=%v invalid=%v err=%v", required, missing, invalid, err)
	}
}
