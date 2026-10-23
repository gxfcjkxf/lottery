package betting

import (
	"context"
	"errors"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/withdrawal"
)

// Different endpoints and different client keys must still share the wallet lock.
// Winning funds have no outstanding recharge/gift turnover requirement, so this
// exercises the real withdrawal qualification checker rather than an allow stub.
func TestConcurrentBetWithdrawalAndAdjustmentShareAccountLock(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	a := eligibilityAdmin(t, f)
	eligibilityBrandPolicy(t, f, a, "1")
	eligibilityFund(t, f, 0, 1)
	ctx := context.Background()
	start := make(chan struct{})
	type outcome struct {
		kind string
		err  error
	}
	results := make(chan outcome, 3)
	go func() {
		<-start
		_, err := placeBettingOrder(t, f, f.input, "mixed-endpoint-bet")
		results <- outcome{"bet", err}
	}()
	go func() {
		<-start
		_, err := eligibilityCreate(t, f, withdrawal.OrderInput{Points: 1, ClientKey: "mixed-endpoint-withdrawal", SourceAllocation: []points.Allocation{{Source: "winning", State: "available", Points: 1}}})
		results <- outcome{"withdrawal", err}
	}()
	go func() {
		<-start
		tx, err := f.db.Begin(ctx)
		if err == nil {
			defer tx.Rollback(ctx)
			var delta points.Balance
			delta[1][0] = -1
			_, err = (points.Store{DB: f.db}).Post(ctx, tx, points.Change{BrandID: f.brand, MemberID: f.member, EntryType: "adjustment", ReferenceType: "mixed_endpoint_test", OperationKey: "mixed-endpoint-adjustment", ActorType: "system", RequestID: ids.New(), Delta: delta, Allocation: []points.Allocation{{Source: "winning", State: "available", Points: 1}}})
			if err == nil {
				err = tx.Commit(ctx)
			}
		}
		results <- outcome{"adjustment", err}
	}()
	close(start)
	winner := ""
	for range 3 {
		result := <-results
		if result.err == nil {
			if winner != "" {
				t.Fatalf("two operations spent the same point: %s and %s", winner, result.kind)
			}
			winner = result.kind
		} else if !errors.Is(result.err, points.ErrInsufficient) {
			t.Errorf("unexpected %s rejection: %v", result.kind, result.err)
		}
	}
	if winner == "" {
		t.Fatal("no operation succeeded")
	}
	wallet, err := (points.Store{DB: f.db}).Read(ctx, f.brand, f.member)
	if err != nil || wallet.AvailablePoints != 0 {
		t.Fatalf("wallet=%+v err=%v", wallet, err)
	}
	var bets, withdrawals, changes int
	if err = f.db.QueryRow(ctx, `SELECT (SELECT count(*) FROM bet_orders WHERE brand_id=$1 AND brand_member_id=$2),(SELECT count(*) FROM withdrawal_orders WHERE brand_id=$1 AND member_id=$2),(SELECT count(*) FROM point_ledger_entries WHERE brand_id=$1 AND member_id=$2 AND entry_type IN ('bet','withdrawal_reserve') OR brand_id=$1 AND member_id=$2 AND operation_key='mixed-endpoint-adjustment')`, f.brand, f.member).Scan(&bets, &withdrawals, &changes); err != nil {
		t.Fatal(err)
	}
	if changes != 1 || (winner == "bet") != (bets == 1) || (winner == "withdrawal") != (withdrawals == 1) {
		t.Fatalf("winner=%s bets=%d withdrawals=%d debits=%d", winner, bets, withdrawals, changes)
	}
	reconciliation, err := (points.Store{DB: f.db}).Reconcile(ctx, f.brand, f.member)
	if err != nil || !reconciliation.Consistent {
		t.Fatalf("reconciliation=%+v err=%v", reconciliation, err)
	}
}
