package finance

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"testing"
)

func TestRechargeOutboxFailureRollsBackCreditOrderAndAudit(t *testing.T) {
	f := newFinanceFixture(t)
	ctx := context.Background()
	tx := financeTx(t, f.pool)
	r, e := f.service.CreateRecharge(ctx, tx, f.brandID, f.memberID, points.Amount(20), "proof", "test", "fixture", financeMeta(f.adminID))
	if e != nil {
		t.Fatal(e)
	}
	commitFinanceTx(t, tx)
	if _, e = f.pool.Exec(ctx, `CREATE FUNCTION reject_recharge_event() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.event_type='recharge.confirmed' THEN RAISE EXCEPTION 'test'; END IF;RETURN NEW;END $$;CREATE TRIGGER reject_recharge_event BEFORE INSERT ON outbox_events FOR EACH ROW EXECUTE FUNCTION reject_recharge_event()`); e != nil {
		t.Fatal(e)
	}
	tx = financeTx(t, f.pool)
	_, e = f.service.ConfirmRecharge(ctx, tx, f.brandID, r.ID, 1, "should all rollback", financeMeta(f.adminID))
	tx.Rollback(ctx)
	if e == nil {
		t.Fatal("expected outbox failure")
	}
	var status string
	var balance, ledger, audits int
	if e = f.pool.QueryRow(ctx, `SELECT state,(SELECT sum(points) FROM point_buckets WHERE account_id=$2),(SELECT count(*) FROM point_ledger_entries),(SELECT count(*) FROM audit_logs WHERE action='finance.recharge.confirm') FROM recharge_orders WHERE id=$1`, r.ID, f.accountID).Scan(&status, &balance, &ledger, &audits); e != nil || status != "pending" || balance != 0 || ledger != 0 || audits != 0 {
		t.Fatal(status, balance, ledger, audits, e)
	}
}
