package betting

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/rulebook"
	"github.com/jackc/pgx/v5"
)

func attemptNextPeriod(t *testing.T, f bettingFixture, number string) (rulebook.Period, error) {
	t.Helper()
	ctx := context.Background()
	tx, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	now := time.Now().UTC()
	p, err := (rulebook.Store{DB: f.db}).OpenPeriod(ctx, tx, f.brand, f.game.ID, number, now.Add(-time.Second), now.Add(time.Hour), now.Add(2*time.Hour))
	if err == nil {
		err = tx.Commit(ctx)
	}
	return p, err
}

func finishUnusedBettingPeriod(t *testing.T, f bettingFixture) {
	t.Helper()
	actor := periodCancelActor(f)
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		job, err := f.service.CancelPeriod(context.Background(), tx, f.brand, actor, f.period.ID, CancelPeriodInput{Version: f.period.Version, Mode: "bet_cancelled", Cause: "operator_cancel", Reason: "finish unused test fixture before next period"}, policyMeta(actor.ID))
		if err == nil && job.State != "completed" {
			t.Fatalf("unused fixture has unfinished refunds: %+v", job)
		}
		return err
	})
}

func TestNextPeriodLegacyBettingRowCannotBypassAdmissionGate(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	fundBettingWallet(t, f, 10)
	ctx := context.Background()
	next := ids.New()
	if _, err := f.db.Exec(ctx, `INSERT INTO periods(id,brand_id,game_id,period_no,sequence,bet_start_at,bet_end_at,draw_at,status) VALUES($1,$2,$3,'legacy-overlapping',2,clock_timestamp()-interval '1 minute',clock_timestamp()+interval '1 hour',clock_timestamp()+interval '2 hours','betting')`, next, f.brand, f.game.ID); err != nil {
		t.Fatal(err)
	}
	in := f.input
	in.PeriodID = next
	before := walletBySource(t, f)
	tx, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.service.Preview(ctx, tx, f.brand, f.user, in)
	_ = tx.Rollback(ctx)
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("legacy next preview allowed: %v", err)
	}
	if _, err = placeBettingOrder(t, f, in, "legacy-next-must-be-blocked"); !errors.Is(err, ErrClosed) {
		t.Fatalf("legacy next bet allowed: %v", err)
	}
	if got := walletBySource(t, f); got != before {
		t.Fatalf("rejected admission changed wallet: %v -> %v", before, got)
	}
	var bets, orders int
	if err = f.db.QueryRow(ctx, `SELECT (SELECT count(*) FROM bet_orders),(SELECT count(*) FROM point_ledger_entries WHERE entry_type='bet')`).Scan(&orders, &bets); err != nil || orders != 0 || bets != 0 {
		t.Fatalf("rejected admission persisted orders=%d debits=%d err=%v", orders, bets, err)
	}
	finishUnusedBettingPeriod(t, f)
	if _, err = placeBettingOrder(t, f, in, "legacy-next-after-finalization"); err != nil {
		t.Fatal(err)
	}
}

func TestNextPeriodWaitsForCompletedSettlementNotCalculationOrApproval(t *testing.T) {
	f, _ := drawnFixtureOrder(t)
	ctx := context.Background()
	actor := settlementActor(f)
	if _, err := attemptNextPeriod(t, f, "blocked-after-draw"); !errors.Is(err, rulebook.ErrState) {
		t.Fatalf("next period before settlement: %v", err)
	}
	mode := "manual"
	policy := setSettlementMode(t, f, actor, 1, &mode)
	job := startFixtureSettlement(t, f, actor, policy.Version)
	if _, err := f.service.ProcessSettlements(ctx, 20); err != nil {
		t.Fatal(err)
	}
	job, err := f.service.SettlementJob(ctx, f.brand, job.ID)
	if err != nil || job.State != "awaiting_approval" {
		t.Fatal(job, err)
	}
	if _, err = attemptNextPeriod(t, f, "blocked-after-calculation"); !errors.Is(err, rulebook.ErrState) {
		t.Fatalf("next period before approval: %v", err)
	}
	job, err = actOnFixtureSettlement(t, f, actor, job, "approve", job.Version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = attemptNextPeriod(t, f, "blocked-before-payout"); !errors.Is(err, rulebook.ErrState) {
		t.Fatalf("next period before payout completion: %v", err)
	}
	if _, err = f.service.ProcessSettlements(ctx, 20); err != nil {
		t.Fatal(err)
	}
	next, err := attemptNextPeriod(t, f, "after-settlement")
	if err != nil {
		t.Fatal(err)
	}
	in := f.input
	in.PeriodID = next.ID
	if _, err = placeBettingOrder(t, f, in, "next-after-completed-settlement"); err != nil {
		t.Fatal(err)
	}
	if got := walletBySource(t, f); got[0][0] != 98 || got[1][0] != 10 {
		t.Fatalf("unexpected wallet after next bet: %v", got)
	}
}

func TestNextPeriodWaitsForAllCancellationRefunds(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	fundBettingWallet(t, f, 10)
	ctx := context.Background()
	for _, key := range []string{"prior-refund-one", "prior-refund-two"} {
		if _, err := placeBettingOrder(t, f, f.input, key); err != nil {
			t.Fatal(err)
		}
	}
	actor := periodCancelActor(f)
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		_, err := f.service.CancelPeriod(ctx, tx, f.brand, actor, f.period.ID, CancelPeriodInput{Version: f.period.Version, Mode: "bet_cancelled", Cause: "operator_cancel", Reason: "cancel prior period"}, policyMeta(actor.ID))
		return err
	})
	if _, err := attemptNextPeriod(t, f, "blocked-before-refund"); !errors.Is(err, rulebook.ErrState) {
		t.Fatalf("next period before refunds: %v", err)
	}
	if _, err := f.service.ProcessCancellations(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := attemptNextPeriod(t, f, "blocked-after-partial-refund"); !errors.Is(err, rulebook.ErrState) {
		t.Fatalf("next period after partial refund: %v", err)
	}
	if _, err := f.service.ProcessCancellations(ctx, 20); err != nil {
		t.Fatal(err)
	}
	if got := walletBySource(t, f); got[0][0] != 10 {
		t.Fatalf("refund not restored: %v", got)
	}
	if _, err := attemptNextPeriod(t, f, "after-all-refunds"); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentNextPeriodOpeningAfterRefundsOpensExactlyOne(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	finishUnusedBettingPeriod(t, f)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, no := range []string{"concurrent-next-a", "concurrent-next-b"} {
		go func(no string) {
			<-start
			tx, err := f.db.Begin(ctx)
			if err != nil {
				results <- err
				return
			}
			defer tx.Rollback(context.Background())
			now := time.Now().UTC()
			_, err = (rulebook.Store{DB: f.db}).OpenPeriod(ctx, tx, f.brand, f.game.ID, no, now.Add(-time.Second), now.Add(time.Hour), now.Add(2*time.Hour))
			if err == nil {
				err = tx.Commit(ctx)
			}
			results <- err
		}(no)
	}
	close(start)
	winners, blocked := 0, 0
	for range 2 {
		select {
		case err := <-results:
			if err == nil {
				winners++
			} else if errors.Is(err, rulebook.ErrState) {
				blocked++
			} else {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	var opened int
	var ordinal int64
	if err := f.db.QueryRow(ctx, `SELECT (SELECT count(*) FROM periods WHERE game_id=$1 AND status='betting'),started_sequence FROM games WHERE id=$1`, f.game.ID).Scan(&opened, &ordinal); err != nil {
		t.Fatal(err)
	}
	if winners != 1 || blocked != 1 || opened != 1 || ordinal != 2 {
		t.Fatalf("opening race winners=%d blocked=%d opened=%d ordinal=%d", winners, blocked, opened, ordinal)
	}
}
