package betting

import (
	"context"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/agency"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

func TestCommissionPolicyContentionDoesNotHoldBettingWallet(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	fundBettingWallet(t, f, 20)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	holder, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Rollback(context.Background())
	if _, err = (agency.Service{DB: f.db}).LockPolicy(ctx, holder, f.brand); err != nil {
		t.Fatal(err)
	}
	type result struct {
		order Order
		err   error
	}
	pid := make(chan uint32, 1)
	done := make(chan result, 1)
	go func() {
		tx, e := f.db.Begin(ctx)
		if e != nil {
			done <- result{err: e}
			return
		}
		defer tx.Rollback(context.Background())
		pid <- tx.Conn().PgConn().PID()
		o, e := f.service.Place(ctx, tx, f.brand, f.user, f.input, "commission-lock-first", points.Metadata{RequestID: "commission-lock-test"})
		if e == nil {
			e = tx.Commit(ctx)
		}
		done <- result{o, e}
	}()
	var betPID uint32
	select {
	case betPID = <-pid:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		var waiting bool
		err = f.db.QueryRow(ctx, `SELECT coalesce(wait_event_type='Lock',false) FROM pg_stat_activity WHERE pid=$1`, betPID).Scan(&waiting)
		if err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("bet never reached contended policy lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// The policy mutex is held, but a separate wallet transaction remains free.
	// This would fail NOWAIT if Place locked wallet before the policy.
	probe, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var account string
	err = probe.QueryRow(ctx, `SELECT id::text FROM point_accounts WHERE brand_id=$1 AND brand_member_id=$2 FOR UPDATE NOWAIT`, f.brand, f.member).Scan(&account)
	_ = probe.Rollback(ctx)
	if err != nil {
		t.Fatalf("bet held wallet while awaiting agency policy: %v", err)
	}
	if err = holder.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case r := <-done:
		if r.err != nil || r.order.ID == "" {
			t.Fatal(r.order, r.err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	var count int
	if err = f.db.QueryRow(ctx, `SELECT count(*) FROM bet_orders WHERE brand_id=$1 AND brand_member_id=$2 AND client_key='commission-lock-first' AND commission_rule_snapshot IS NOT NULL`, f.brand, f.member).Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
}
