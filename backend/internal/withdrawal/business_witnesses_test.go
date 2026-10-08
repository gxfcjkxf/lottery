package withdrawal

import (
	"context"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

func TestFinanceBusinessWitnessesTrackReservationPaymentAndRelease(t *testing.T) {
	f := newOrderIntegrationFixture(t, true, true)
	ctx := context.Background()
	policyService := Service{DB: f.db}
	policy, err := policyService.BrandPolicy(ctx, orderTestBrand)
	if err != nil {
		t.Fatal(err)
	}
	config := policy.Config
	config.AllowedSources = append(config.AllowedSources, "commission")
	tx, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = policyService.UpdateBrand(ctx, tx, orderTestBrand, f.admin,
		BrandInput{Version: policy.Version, Config: config, Reason: "enable commission withdrawal witness coverage"},
		points.Metadata{ActorType: "admin", ActorID: f.adminID, RequestID: ids.New()}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	f.fund(t, []points.Allocation{{Source: "recharge", State: "available", Points: 20},
		{Source: "winning", State: "available", Points: 5}, {Source: "gift", State: "available", Points: 5},
		{Source: "commission", State: "available", Points: 5}})
	allocation := []points.Allocation{{Source: "recharge", State: "available", Points: 5},
		{Source: "winning", State: "available", Points: 5}, {Source: "gift", State: "available", Points: 5},
		{Source: "commission", State: "available", Points: 5}}
	paid := createOrder(t, f, 20, "witness-paid-order", allocation)
	meta := points.Metadata{ActorType: "admin", ActorID: f.adminID, RequestID: ids.New()}
	if _, err := orderAction(t, f, paid.ID, "approve", "witness-paid-approve", f.admin,
		ActionInput{Version: 1, ClientKey: "witness-paid-approve", Reason: "reviewed"}, meta); err != nil {
		t.Fatal(err)
	}
	if _, err := orderAction(t, f, paid.ID, "mark_paid", "witness-paid-final", f.admin,
		ActionInput{Version: 2, ClientKey: "witness-paid-final", Reason: "transfer complete"}, meta); err != nil {
		t.Fatal(err)
	}
	failed := createOrder(t, f, 10, "witness-release-order", []points.Allocation{{Source: "recharge", State: "available", Points: 10}})
	if _, err := orderAction(t, f, failed.ID, "reject", "witness-release-final", f.admin,
		ActionInput{Version: 1, ClientKey: "witness-release-final", Reason: "review declined"}, meta); err != nil {
		t.Fatal(err)
	}
	var total, invalid int
	if err := f.db.QueryRow(context.Background(), `SELECT count(*),count(*) FILTER(WHERE NOT source_valid)
	 FROM point_finance_business_witnesses($1,(SELECT id FROM point_accounts WHERE brand_id=$1 AND brand_member_id=$2))
	 WHERE family='withdrawal'`, orderTestBrand, f.member).Scan(&total, &invalid); err != nil {
		t.Fatal(err)
	}
	if total != 4 || invalid != 0 {
		t.Fatalf("withdrawal witness rows=%d invalid=%d; want two reserves, one paid edge, and one release edge", total, invalid)
	}
	var consistent bool
	if err = f.db.QueryRow(ctx, `SELECT (point_business_preview($1,(SELECT id FROM point_accounts WHERE brand_id=$1 AND brand_member_id=$2))->>'consistent')::boolean`, orderTestBrand, f.member).Scan(&consistent); err != nil || !consistent {
		t.Fatalf("full kernel rejected actual four-source withdrawal history: consistent=%v err=%v", consistent, err)
	}
}

func TestFinanceBusinessWitnessesRetainMissingPaidWithdrawalPointer(t *testing.T) {
	f := newOrderIntegrationFixture(t, true, true)
	allocation := []points.Allocation{{Source: "recharge", State: "available", Points: 10}}
	f.fund(t, allocation)
	order := createOrder(t, f, 10, "witness-missing-paid", allocation)
	meta := points.Metadata{ActorType: "admin", ActorID: f.adminID, RequestID: ids.New()}
	if _, err := orderAction(t, f, order.ID, "approve", "witness-missing-approve", f.admin,
		ActionInput{Version: 1, ClientKey: "witness-missing-approve", Reason: "reviewed"}, meta); err != nil {
		t.Fatal(err)
	}
	if _, err := orderAction(t, f, order.ID, "mark_paid", "witness-missing-paid-final", f.admin,
		ActionInput{Version: 2, ClientKey: "witness-missing-paid-final", Reason: "transfer complete"}, meta); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	tx, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `ALTER TABLE withdrawal_orders DISABLE TRIGGER withdrawal_projection;
		ALTER TABLE withdrawal_order_transitions DISABLE TRIGGER withdrawal_transition_evidence;
		DO $$ DECLARE constraint_name text; BEGIN
		 SELECT conname INTO constraint_name FROM pg_constraint
		 WHERE conrelid='withdrawal_orders'::regclass AND contype='c'
		 AND pg_get_constraintdef(oid) LIKE '%paid_entry_id%';
		 IF constraint_name IS NULL THEN RAISE EXCEPTION 'paid pointer check constraint not found'; END IF;
		 EXECUTE format('ALTER TABLE withdrawal_orders DROP CONSTRAINT %I',constraint_name);
		END $$`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE withdrawal_orders SET paid_entry_id=NULL WHERE id=$1`, order.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `DELETE FROM withdrawal_order_transitions WHERE order_id=$1 AND to_state='paid'`, order.ID); err != nil {
		t.Fatal(err)
	}
	var count int
	var valid, missingLedger bool
	if err = tx.QueryRow(ctx, `SELECT count(*),bool_and(source_valid),bool_and(ledger_id IS NULL) FROM point_finance_business_witnesses($1,$2)
	 WHERE family='withdrawal' AND source_id=$3 AND expected_entry_type='withdrawal_paid'`, orderTestBrand, order.AccountID, order.ID).Scan(&count, &valid, &missingLedger); err != nil {
		t.Fatal(err)
	}
	if count != 1 || valid || !missingLedger {
		t.Fatalf("missing required paid pointer witness: rows=%d valid=%t missing_ledger=%t; want one invalid row with no ledger pointer", count, valid, missingLedger)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	var guardsEnabled bool
	if err = f.db.QueryRow(ctx, `SELECT count(*)=2 FROM pg_trigger WHERE tgenabled='O' AND
		(tgrelid,tgname) IN (('withdrawal_orders'::regclass,'withdrawal_projection'),
		('withdrawal_order_transitions'::regclass,'withdrawal_transition_evidence'))`).Scan(&guardsEnabled); err != nil || !guardsEnabled {
		t.Fatalf("withdrawal guards not restored after rollback: enabled=%t err=%v", guardsEnabled, err)
	}
}
