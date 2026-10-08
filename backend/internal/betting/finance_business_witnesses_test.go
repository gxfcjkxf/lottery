package betting

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestFinanceBusinessWitnessesPreservePaidCommissionAndAdjustment(t *testing.T) {
	f := newCommissionBatchFixture(t)
	payment := payManualCommissionForAdjustment(t, f, prepareManualCommissionPayment(t, f))
	page := commissionTargets(t, f, payment.ID)
	if len(page.Items) != 1 || page.Items[0].State != "paid" {
		t.Fatalf("payment targets=%+v; want one paid target", page.Items)
	}
	target := page.Items[0]
	if _, err := adjustCommission(t, f, f.betting.brand, target, commissionAdjustmentActor(f), 1, 2); err != nil {
		t.Fatal(err)
	}
	var total, invalid int
	if err := f.betting.db.QueryRow(context.Background(), `SELECT count(*),count(*) FILTER(WHERE NOT source_valid)
	 FROM point_finance_business_witnesses($1,(SELECT id FROM point_accounts WHERE brand_id=$1 AND brand_member_id=$2))
	 WHERE family IN('commission','commission_adjustment')`, f.betting.brand, target.MemberID).Scan(&total, &invalid); err != nil {
		t.Fatal(err)
	}
	if total != 2 || invalid != 0 {
		t.Fatalf("commission witness rows=%d invalid=%d; want original paid target and adjustment", total, invalid)
	}
}

func TestFinanceBusinessWitnessesPreserveActualCorrectionAndRequireApplyStep(t *testing.T) {
	f, _, _ := readyAutomaticCorrectionExecutionFixture(t)
	execution := ensureCorrectionExecution(t, f)
	page := correctionExecutionTargetsRead(t, f, execution.ID)
	if len(page.Items) != 1 || page.Items[0].LedgerEntryID == nil {
		t.Fatalf("expected an actual nonzero correction target: %+v", page)
	}
	target := page.Items[0]
	ctx := context.Background()
	var account string
	if err := f.betting.db.QueryRow(ctx, `SELECT id::text FROM point_accounts WHERE brand_id=$1 AND brand_member_id=$2`, f.betting.brand, target.MemberID).Scan(&account); err != nil {
		t.Fatal(err)
	}
	var total, invalid int
	if err := f.betting.db.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE NOT source_valid) FROM point_finance_business_witnesses($1,$2) WHERE family IN('commission','commission_correction')`, f.betting.brand, account).Scan(&total, &invalid); err != nil || total != 2 || invalid != 0 {
		t.Fatalf("historical paid target or actual correction rejected: total=%d invalid=%d err=%v", total, invalid, err)
	}
	var fullConsistent bool
	if err := f.betting.db.QueryRow(ctx, `SELECT (point_business_preview($1,$2)->>'consistent')::boolean`, f.betting.brand, account).Scan(&fullConsistent); err != nil || !fullConsistent {
		t.Fatalf("full kernel rejected genuine beneficiary financial history: consistent=%v err=%v", fullConsistent, err)
	}

	// A missing immutable apply step cannot remove the required correction
	// edge and make absence of proof look like zero required work.
	tx, err := f.betting.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT tgname FROM pg_trigger WHERE tgrelid='commission_correction_execution_steps'::regclass AND NOT tgisinternal AND (tgtype & 2)=2 AND (tgtype & 8)=8`)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	rows.Close()
	if err = rows.Err(); err != nil || len(names) == 0 {
		t.Fatal("step delete guard missing", err)
	}
	for _, name := range names {
		if _, err = tx.Exec(ctx, `ALTER TABLE commission_correction_execution_steps DISABLE TRIGGER `+pgx.Identifier{name}.Sanitize()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = tx.Exec(ctx, `DELETE FROM commission_correction_execution_steps WHERE execution_id=$1 AND operation='apply' AND target_id=$2`, execution.ID, target.ID); err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE NOT source_valid) FROM point_finance_business_witnesses($1,$2) WHERE family='commission_correction' AND source_id=$3`, f.betting.brand, account, target.ID).Scan(&total, &invalid); err != nil || total != 1 || invalid != 1 {
		t.Fatalf("missing apply step disappeared or passed: total=%d invalid=%d err=%v", total, invalid, err)
	}
}
