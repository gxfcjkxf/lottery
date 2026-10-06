package betting

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rulebook"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

func TestParallelPeriodWorkersAndIndividualCancelNeverDoubleRefund(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	fundBettingWallet(t, f, 20)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	orders := []Order{}
	for range 6 {
		o, e := placeBettingOrder(t, f, f.input, ids.New())
		if e != nil {
			t.Fatal(e)
		}
		orders = append(orders, o)
	}
	actor := periodCancelActor(f)
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		_, e := f.service.CancelPeriod(ctx, tx, f.brand, actor, f.period.ID, CancelPeriodInput{Version: f.period.Version, Mode: "judged_cancelled", Cause: "no_result", Reason: "parallel cancellation"}, policyMeta(actor.ID))
		return e
	})
	// Another valid endpoint can refund one captured target before the workers.
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		_, e := f.service.CancelAdmin(ctx, tx, f.brand, actor, orders[0].ID, 1, "independent operator refund", policyMeta(actor.ID))
		return e
	})
	var wg sync.WaitGroup
	failures := make(chan error, 4)
	for range 4 {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := f.service.ProcessCancellations(ctx, 20); failures <- e }()
	}
	wg.Wait()
	close(failures)
	for e := range failures {
		if e != nil {
			t.Fatal(e)
		}
	}
	job, e := f.service.PeriodCancellation(ctx, f.brand, f.period.ID)
	if e != nil || job.State != "completed" || job.RefundedCount != 5 || job.AlreadyRefundedCount != 1 || walletBySource(t, f)[0][0] != 20 {
		t.Fatalf("parallel completion %+v %v", job, e)
	}
	var refunds int
	if e = f.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE entry_type='refund'`).Scan(&refunds); e != nil || refunds != 6 {
		t.Fatalf("refunds %d %v", refunds, e)
	}
}

func TestDrawnPeriodInvalidResultCancellationPreservesLockedEvidence(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	fundBettingWallet(t, f, 10)
	ctx := context.Background()
	actor := periodCancelActor(f)
	actor.Roles[0].Permissions = append(actor.Roles[0].Permissions, access.Permission{Resource: "draw", Action: "manual_create", Scope: access.ScopeBrand})
	now := time.Now().UTC()
	rs := rulebook.Store{DB: f.db}
	var p rulebook.Period
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var e error
		p, e = rs.OpenPeriod(ctx, tx, f.brand, f.game.ID, "cancel-result-near", now.Add(-time.Second), now.Add(500*time.Millisecond), now.Add(time.Second))
		return e
	})
	input := f.input
	input.PeriodID = p.ID
	o, e := placeBettingOrder(t, f, input, "cancel-result-before-draw")
	if e != nil {
		t.Fatal(e)
	}
	time.Sleep(time.Until(p.DrawAt) + 30*time.Millisecond)
	if _, e = rs.Tick(ctx); e != nil {
		t.Fatal(e)
	}
	periods, e := rs.Periods(ctx, f.brand, f.game.ID, 100, 0)
	if e != nil {
		t.Fatal(e)
	}
	for _, candidate := range periods {
		if candidate.ID == p.ID {
			p = candidate
		}
	}
	var result rulebook.DrawResult
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var e error
		result, e = rs.ManualDraw(ctx, tx, f.brand, actor, p.ID, p.Version, p.PeriodNo, rules.Draw{Digits: []int{3, 2, 1}}, p.DrawAt, "known result before judgment", policyMeta(actor.ID))
		return e
	})
	var periodVersion int64
	if e = f.db.QueryRow(ctx, `SELECT version FROM periods WHERE id=$1`, p.ID).Scan(&periodVersion); e != nil {
		t.Fatal(e)
	}
	tx, e := f.db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.service.CancelPeriod(ctx, tx, f.brand, actor, p.ID, CancelPeriodInput{Version: periodVersion, Mode: "judged_cancelled", Cause: "no_result", Reason: "cannot claim absent result"}, policyMeta(actor.ID))
	_ = tx.Rollback(ctx)
	if !errors.Is(e, ErrState) {
		t.Fatalf("no-result judgment with locked result=%v", e)
	}
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		_, e := f.service.CancelPeriod(ctx, tx, f.brand, actor, p.ID, CancelPeriodInput{Version: periodVersion, Mode: "judged_cancelled", Cause: "invalid_result", Reason: "operator finds invalid result"}, policyMeta(actor.ID))
		return e
	})
	if _, e = f.service.ProcessCancellations(ctx, 10); e != nil {
		t.Fatal(e)
	}
	history, e := rs.Draw(ctx, f.brand, p.ID, 50, 0)
	if e != nil || history.Current == nil || history.Current.ID != result.ID || len(history.History) != 1 {
		t.Fatalf("erased locked draw %+v %v", history, e)
	}
	current, e := f.service.Order(ctx, f.brand, f.member, o.ID)
	if e != nil || current.Status != "judged_cancelled" || walletBySource(t, f)[0][0] != 10 {
		t.Fatalf("missing drawn-period refund %+v %v", current, e)
	}
	tx, e = f.db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	_, e = rs.ManualDraw(ctx, tx, f.brand, actor, p.ID, periodVersion+1, p.PeriodNo, rules.Draw{Digits: []int{1, 2, 3}}, p.DrawAt, "late result cannot reopen", policyMeta(actor.ID))
	_ = tx.Rollback(ctx)
	if !errors.Is(e, rulebook.ErrState) {
		t.Fatalf("late manual result reopened cancelled period %v", e)
	}
}

func periodCancelActor(f bettingFixture) access.Account {
	a := exceptionActor(f)
	a.Roles[0].Permissions = append(a.Roles[0].Permissions,
		access.Permission{Resource: "period", Action: "cancel", Scope: access.ScopeBrand},
		access.Permission{Resource: "period", Action: "cancel_retry", Scope: access.ScopeBrand})
	return a
}

func TestCancellationStartEventFailureRollsBackClosureAndTargets(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	fundBettingWallet(t, f, 10)
	ctx := context.Background()
	actor := periodCancelActor(f)
	if _, e := placeBettingOrder(t, f, f.input, "cancel-start-rollback-bet"); e != nil {
		t.Fatal(e)
	}
	if _, e := f.db.Exec(ctx, `CREATE FUNCTION reject_cancel_start() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.event_type='period.cancellation.started' THEN RAISE EXCEPTION 'cancel start witness failure'; END IF; RETURN NEW; END $$;CREATE TRIGGER reject_cancel_start BEFORE INSERT ON outbox_events FOR EACH ROW EXECUTE FUNCTION reject_cancel_start()`); e != nil {
		t.Fatal(e)
	}
	tx, e := f.db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.service.CancelPeriod(ctx, tx, f.brand, actor, f.period.ID, CancelPeriodInput{Version: f.period.Version, Mode: "judged_cancelled", Cause: "no_result", Reason: "start must be fully atomic"}, policyMeta(actor.ID))
	_ = tx.Rollback(ctx)
	if e == nil {
		t.Fatal("cancel start did not fail")
	}
	job, e := f.service.PeriodCancellation(ctx, f.brand, f.period.ID)
	if e != nil || job != nil {
		t.Fatalf("partial durable task %+v %v", job, e)
	}
	var status string
	var targets int
	if e = f.db.QueryRow(ctx, `SELECT status FROM periods WHERE id=$1`, f.period.ID).Scan(&status); e != nil || status != "betting" {
		t.Fatalf("partial close %s %v", status, e)
	}
	if e = f.db.QueryRow(ctx, `SELECT count(*) FROM period_cancellation_targets`).Scan(&targets); e != nil || targets != 0 || walletBySource(t, f)[0][0] != 9 {
		t.Fatalf("partial targets or money %d %v", targets, e)
	}
}

func TestPeriodCancellationWaitsForAdmittedBetAndCapturesItsRefund(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	fundBettingWallet(t, f, 10)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	blocker := lockBettingWallet(t, f)
	defer blocker.Rollback(ctx)
	placed, pids := runAuthenticatedPlace(t, ctx, f, f.input, "period-cancel-admitted-bet")
	var placePID int
	select {
	case placePID = <-pids:
	case e := <-placed:
		t.Fatalf("unexpected early placement %v", e)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	waitForBettingLockWait(t, ctx, f.db, placePID)
	actor := periodCancelActor(f)
	cancelPID := make(chan int, 1)
	result := make(chan error, 1)
	jobs := make(chan Cancellation, 1)
	go func() {
		tx, e := f.db.Begin(ctx)
		if e != nil {
			result <- e
			return
		}
		defer tx.Rollback(ctx)
		var pid int
		if e = tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); e != nil {
			result <- e
			return
		}
		cancelPID <- pid
		job, e := f.service.CancelPeriod(ctx, tx, f.brand, actor, f.period.ID, CancelPeriodInput{Version: f.period.Version, Mode: "judged_cancelled", Cause: "no_result", Reason: "capture in-flight admitted bet"}, policyMeta(actor.ID))
		if e == nil {
			e = tx.Commit(ctx)
		}
		jobs <- job
		result <- e
	}()
	var pid int
	select {
	case pid = <-cancelPID:
	case e := <-result:
		t.Fatal(e)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	waitForBettingLockWait(t, ctx, f.db, pid)
	if e := blocker.Rollback(ctx); e != nil {
		t.Fatal(e)
	}
	if e := <-placed; e != nil {
		t.Fatalf("already-admitted placement lost %v", e)
	}
	if e := <-result; e != nil {
		t.Fatal(e)
	}
	job := <-jobs
	if job.TotalCount != 1 {
		t.Fatalf("admitted order escaped targets %+v", job)
	}
	if n, e := f.service.ProcessCancellations(ctx, 10); e != nil || n != 1 || walletBySource(t, f)[0][0] != 10 {
		t.Fatalf("missed in-flight refund %d %v", n, e)
	}
}

func TestCancellationDatabaseRequiresAllTargetsAndRejectsPrematureCompletion(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	fundBettingWallet(t, f, 10)
	ctx := context.Background()
	actor := periodCancelActor(f)
	o, e := placeBettingOrder(t, f, f.input, "cancellation-sql-witness")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.db.Exec(ctx, `UPDATE periods SET status='judged_cancelled',version=version+1,state_reason='unwitnessed cancel' WHERE id=$1`, f.period.ID); e == nil {
		t.Fatal("period cancelled without durable task")
	}
	tx, e := f.db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	fakeID := ids.New()
	_, e = tx.Exec(ctx, `INSERT INTO period_cancellations(id,brand_id,game_id,period_id,period_version,mode,cause,state,target_count,reason,created_by) VALUES($1,$2,$3,$4,$5,'judged_cancelled','no_result','processing',0,'missing targets',$6)`, fakeID, f.brand, f.game.ID, f.period.ID, f.period.Version+1, actor.ID)
	if e != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(e)
	}
	_, e = tx.Exec(ctx, `UPDATE periods SET status='judged_cancelled',version=version+1,state_reason='missing targets' WHERE id=$1`, f.period.ID)
	if e != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e == nil {
		t.Fatal("task committed without its eligible orders")
	}
	var job Cancellation
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var e error
		job, e = f.service.CancelPeriod(ctx, tx, f.brand, actor, f.period.ID, CancelPeriodInput{Version: f.period.Version, Mode: "judged_cancelled", Cause: "no_result", Reason: "proper complete target snapshot"}, policyMeta(actor.ID))
		return e
	})
	for _, statement := range []string{`UPDATE period_cancellations SET state='completed',version=version+1,completed_at=clock_timestamp() WHERE id=$1`, `UPDATE period_cancellations SET reason='rewrite history' WHERE id=$1`, `DELETE FROM period_cancellations WHERE id=$1`, `DELETE FROM period_cancellation_targets WHERE cancellation_id=$1`} {
		if _, e = f.db.Exec(ctx, statement, job.ID); e == nil {
			t.Fatalf("unprotected history: %s", statement)
		}
	}
	if _, e = f.service.ProcessCancellations(ctx, 10); e != nil {
		t.Fatal(e)
	}
	final, e := f.service.Order(ctx, f.brand, f.member, o.ID)
	if e != nil || final.Status != "judged_cancelled" {
		t.Fatal(final, e)
	}
}

func TestPeriodCancellationRejectsForeignScopeSuperAndStaleVersion(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	ctx := context.Background()
	actor := periodCancelActor(f)
	in := CancelPeriodInput{Version: f.period.Version, Mode: "judged_cancelled", Cause: "no_result", Reason: "empty future period cancellation"}
	for _, test := range []struct {
		actor   access.Account
		brand   string
		version int64
		want    error
	}{
		{access.Account{ID: actor.ID, Type: access.AccountAdmin, BrandIDs: actor.BrandIDs}, f.brand, in.Version, ErrDenied},
		{func() access.Account { a := actor; a.SuperAdmin = true; return a }(), f.brand, in.Version, ErrDenied},
		{actor, f.brand, in.Version + 1, ErrVersion},
		{func() access.Account {
			a := actor
			a.BrandIDs = []string{storeTestOtherBrand}
			a.Roles = append([]access.Role{}, a.Roles...)
			a.Roles[0].BrandID = storeTestOtherBrand
			return a
		}(), storeTestOtherBrand, in.Version, ErrNotFound},
	} {
		tx, e := f.db.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		body := in
		body.Version = test.version
		_, e = f.service.CancelPeriod(ctx, tx, test.brand, test.actor, f.period.ID, body, policyMeta(actor.ID))
		_ = tx.Rollback(ctx)
		if !errors.Is(e, test.want) {
			t.Fatalf("rejected cancel %v want %v", e, test.want)
		}
	}
	var job Cancellation
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var e error
		job, e = f.service.CancelPeriod(ctx, tx, f.brand, actor, f.period.ID, in, policyMeta(actor.ID))
		return e
	})
	if job.State != "completed" || job.TotalCount != 0 || job.CompletedAt == nil {
		t.Fatalf("zero-target period stuck %+v", job)
	}
	if _, e := f.service.PeriodCancellation(ctx, storeTestOtherBrand, f.period.ID); !errors.Is(e, ErrNotFound) {
		t.Fatalf("foreign period summary %v", e)
	}
}

func TestPeriodCancellationClosesAdmissionAndResumableBatchesRefundExactSources(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	fundBettingWallet(t, f, 20)
	ctx := context.Background()
	orders := []Order{}
	for i := 0; i < 3; i++ {
		o, e := placeBettingOrder(t, f, f.input, ids.New())
		if e != nil {
			t.Fatal(e)
		}
		orders = append(orders, o)
	}
	actor := periodCancelActor(f)
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		_, e := f.service.MarkAbnormal(ctx, tx, f.brand, actor, orders[0].ID, 1, "one abnormal order is still fully refunded", policyMeta(actor.ID))
		return e
	})
	var job Cancellation
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var e error
		job, e = f.service.CancelPeriod(ctx, tx, f.brand, actor, f.period.ID, CancelPeriodInput{Version: f.period.Version, Mode: "judged_cancelled", Cause: "no_result", Reason: "missing draw operator judgment"}, policyMeta(actor.ID))
		return e
	})
	if job.State != "processing" || job.TotalCount != 3 || job.PendingCount != 3 || walletBySource(t, f)[0][0] != 17 {
		t.Fatalf("non-durable cancellation %+v", job)
	}
	_, e := placeBettingOrder(t, f, f.input, "after-period-cancel")
	if !errors.Is(e, ErrClosed) {
		t.Fatalf("cancelled period accepted bet: %v", e)
	}
	n, e := f.service.ProcessCancellations(ctx, 1)
	if e != nil || n != 1 {
		t.Fatalf("first batch %d %v", n, e)
	}
	got, e := f.service.PeriodCancellation(ctx, f.brand, f.period.ID)
	if e != nil || got == nil || got.PendingCount != 2 || got.State != "processing" {
		t.Fatalf("partial progress %+v %v", got, e)
	}
	// A separately constructed worker resumes committed work without an in-memory cursor.
	n, e = (Service{DB: f.db}).ProcessCancellations(ctx, 10)
	if e != nil || n != 2 {
		t.Fatalf("resume %d %v", n, e)
	}
	got, e = f.service.PeriodCancellation(ctx, f.brand, f.period.ID)
	if e != nil || got.State != "completed" || got.RefundedCount != 3 || got.CompletedAt == nil || walletBySource(t, f)[0][0] != 20 {
		t.Fatalf("incomplete original-source refunds %+v %v", got, e)
	}
	for _, o := range orders {
		final, e := f.service.Order(ctx, f.brand, f.member, o.ID)
		if e != nil || final.Status != "judged_cancelled" || final.RefundEntryID == "" {
			t.Fatalf("not judged/refunded %+v %v", final, e)
		}
		tx, e := f.db.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		refund, e := (points.Store{DB: f.db}).Entry(ctx, tx, f.brand, f.member, final.RefundEntryID)
		_ = tx.Rollback(ctx)
		if e != nil || refund.ReversalOf != o.DebitEntryID {
			t.Fatalf("missing debit lineage %+v %v", refund, e)
		}
	}
	n, e = f.service.ProcessCancellations(ctx, 10)
	if e != nil || n != 0 {
		t.Fatalf("completed task replay %d %v", n, e)
	}
	evidence, e := f.service.Exception(ctx, f.brand, orders[0].ID)
	if e != nil || evidence == nil {
		t.Fatal("period cancellation removed abnormal evidence")
	}
}

func TestPeriodRefundFailureRollsBackMoneyAndRequiresExplicitRetry(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	fundBettingWallet(t, f, 10)
	ctx := context.Background()
	actor := periodCancelActor(f)
	o, e := placeBettingOrder(t, f, f.input, "period-refund-failure")
	if e != nil {
		t.Fatal(e)
	}
	var job Cancellation
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var e error
		job, e = f.service.CancelPeriod(ctx, tx, f.brand, actor, f.period.ID, CancelPeriodInput{Version: f.period.Version, Mode: "bet_cancelled", Cause: "operator_cancel", Reason: "cancel betting window"}, policyMeta(actor.ID))
		return e
	})
	_, e = f.db.Exec(ctx, `CREATE FUNCTION reject_period_refund_event() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.event_type='bet.order.cancelled' THEN RAISE EXCEPTION 'refund witness failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_period_refund_event BEFORE INSERT ON outbox_events FOR EACH ROW EXECUTE FUNCTION reject_period_refund_event()`)
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.service.ProcessCancellations(ctx, 5)
	if e == nil {
		t.Fatal("missing refund failure")
	}
	failed, e := f.service.PeriodCancellation(ctx, f.brand, f.period.ID)
	if e != nil || failed.State != "failed" || failed.FailedCount != 1 || failed.LastErrorCode == "" || walletBySource(t, f)[0][0] != 9 {
		t.Fatalf("failure lost evidence or partially refunded %+v %v", failed, e)
	}
	current, e := f.service.Order(ctx, f.brand, f.member, o.ID)
	if e != nil || current.Status != "placed" || current.RefundEntryID != "" {
		t.Fatalf("partial refund %+v %v", current, e)
	}
	n, e := f.service.ProcessCancellations(ctx, 5)
	if e != nil || n != 0 {
		t.Fatalf("failed job automatically retried %d %v", n, e)
	}
	if _, e = f.db.Exec(ctx, `DROP TRIGGER reject_period_refund_event ON outbox_events`); e != nil {
		t.Fatal(e)
	}
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var e error
		job, e = f.service.RetryPeriodCancellation(ctx, tx, f.brand, actor, f.period.ID, failed.Version, "operator repaired transient failure", policyMeta(actor.ID))
		return e
	})
	if job.State != "processing" || job.Version != failed.Version+1 {
		t.Fatalf("retry version %+v", job)
	}
	n, e = f.service.ProcessCancellations(ctx, 5)
	if e != nil || n != 1 || walletBySource(t, f)[0][0] != 10 {
		t.Fatalf("explicit retry did not finish %d %v", n, e)
	}
	var failures int
	if e = f.db.QueryRow(ctx, `SELECT count(*) FROM period_cancellation_failures WHERE cancellation_id=$1`, job.ID).Scan(&failures); e != nil || failures != 1 {
		t.Fatalf("immutable failures %d %v", failures, e)
	}
}
