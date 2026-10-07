package betting

import (
	"context"
	"testing"
	"time"
)

func TestCommissionDiscoveryEnqueueFailureRollsBackActualBetAndDebit(t *testing.T) {
	f := newCommissionBatchFixtureWithWindow(t, 20*time.Second, 22*time.Second)
	ctx := context.Background()
	fingerprint := func() string {
		var value string
		if err := f.betting.db.QueryRow(ctx, `SELECT jsonb_build_object(
		 'orders',(SELECT jsonb_agg(to_jsonb(o) ORDER BY id) FROM bet_orders o),
		 'queues',(SELECT jsonb_agg(to_jsonb(d) ORDER BY id) FROM commission_discovery d),
		 'accounts',(SELECT jsonb_agg(to_jsonb(a) ORDER BY id) FROM point_accounts a),
		 'buckets',(SELECT jsonb_agg(to_jsonb(b) ORDER BY account_id,source,state) FROM point_buckets b),
		 'ledger',(SELECT jsonb_agg(to_jsonb(l) ORDER BY id) FROM point_ledger_entries l),
		 'audit',(SELECT jsonb_agg(to_jsonb(a) ORDER BY id) FROM audit_logs a))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	before := fingerprint()
	_, err := f.betting.db.Exec(ctx, `CREATE FUNCTION test_reject_discovery_enqueue() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test discovery enqueue outage'; END $$;
 CREATE TRIGGER test_reject_discovery_enqueue BEFORE INSERT ON commission_discovery FOR EACH ROW EXECUTE FUNCTION test_reject_discovery_enqueue()`)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = f.betting.db.Exec(ctx, `DROP TRIGGER IF EXISTS test_reject_discovery_enqueue ON commission_discovery;DROP FUNCTION IF EXISTS test_reject_discovery_enqueue()`)
	})
	if _, err = placeBettingOrder(t, f.betting, f.betting.input, "discovery-atomic-bet-failure"); err == nil {
		t.Fatal("real bet committed despite failed discovery enqueue")
	}
	if after := fingerprint(); after != before {
		t.Fatal("failed discovery enqueue leaked debit, order, queue or audit")
	}
	if _, err = f.betting.db.Exec(ctx, `DROP TRIGGER test_reject_discovery_enqueue ON commission_discovery;DROP FUNCTION test_reject_discovery_enqueue()`); err != nil {
		t.Fatal(err)
	}
	order, err := placeBettingOrder(t, f.betting, f.betting.input, "discovery-atomic-bet-failure")
	if err != nil {
		t.Fatal(err)
	}
	d := commissionDiscoveryRead(t, f, order.ID)
	if d.State != "pending" || d.Version != 1 || d.CycleID != nil {
		t.Fatalf("recovered actual bet has no initial queue: %+v", d)
	}
}
