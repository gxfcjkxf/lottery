package betting

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rulebook"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

func TestJudgmentDatabaseRejectsOrphanEvidenceAndUnwitnessedRefund(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	fundBettingWallet(t, f, 10)
	ctx := context.Background()
	actor := judgeActor(f)
	o, e := placeBettingOrder(t, f, f.input, "judgment-database-witness")
	if e != nil {
		t.Fatal(e)
	}
	tx, e := f.db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if _, _, _, e = f.service.lockPeriod(ctx, tx, f.brand, f.period.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = (points.Store{DB: f.db}).LockedSnapshot(ctx, tx, f.brand, f.member); e != nil {
		t.Fatal(e)
	}
	_, e = f.service.refundLocked(ctx, tx, o, "judged_cancelled", "no individual or period evidence", policyMeta(actor.ID))
	_ = tx.Rollback(ctx)
	if e == nil {
		t.Fatal("unwitnessed judged refund accepted")
	}
	tx, e = f.db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	_, e = tx.Exec(ctx, `INSERT INTO bet_order_judgments(id,brand_id,game_id,period_id,order_id,order_version,cause,judged_by,reason) VALUES($1,$2,$3,$4,$5,2,'no_result',$6,'orphan evidence')`, ids.New(), f.brand, f.game.ID, f.period.ID, o.ID, actor.ID)
	if e != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e == nil {
		t.Fatal("judgment committed without order/refund")
	}
	if got, e := f.service.Judgment(ctx, f.brand, o.ID); e != nil || got != nil || walletBySource(t, f)[0][0] != 9 {
		t.Fatalf("rejected state leaked %+v %v", got, e)
	}
}

func TestJudgmentPreservesDrawSnapshotAndRejectsSettlingPeriod(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	fundBettingWallet(t, f, 10)
	ctx := context.Background()
	actor := judgeActor(f)
	actor.Roles[0].Permissions = append(actor.Roles[0].Permissions, access.Permission{Resource: "draw", Action: "manual_create", Scope: access.ScopeBrand})
	rs := rulebook.Store{DB: f.db}
	now := time.Now().UTC()
	var p rulebook.Period
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var e error
		p, e = rs.OpenPeriod(ctx, tx, f.brand, f.game.ID, "single-judgment-result", now.Add(-time.Second), now.Add(time.Second), now.Add(2*time.Second))
		return e
	})
	in := f.input
	in.PeriodID = p.ID
	o, e := placeBettingOrder(t, f, in, "judge-known-result")
	if e != nil {
		t.Fatal(e)
	}
	neighbor, e := placeBettingOrder(t, f, in, "judge-settling-neighbor")
	if e != nil {
		t.Fatal(e)
	}
	time.Sleep(time.Until(p.DrawAt) + 30*time.Millisecond)
	if _, e = rs.Tick(ctx); e != nil {
		t.Fatal(e)
	}
	var version int64
	if e = f.db.QueryRow(ctx, `SELECT version FROM periods WHERE id=$1`, p.ID).Scan(&version); e != nil {
		t.Fatal(e)
	}
	var draw rulebook.DrawResult
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var e error
		draw, e = rs.ManualDraw(ctx, tx, f.brand, actor, p.ID, version, p.PeriodNo, rules.Draw{Digits: []int{3, 2, 1}}, p.DrawAt, "original draw witness", policyMeta(actor.ID))
		return e
	})
	tx, e := f.db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.service.JudgeCancel(ctx, tx, f.brand, actor, o.ID, JudgeCancelInput{Version: 1, Cause: "no_result", Reason: "cannot claim no result"}, policyMeta(actor.ID))
	_ = tx.Rollback(ctx)
	if !errors.Is(e, ErrState) {
		t.Fatalf("locked result misclassified %v", e)
	}
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		_, e := f.service.JudgeCancel(ctx, tx, f.brand, actor, o.ID, JudgeCancelInput{Version: 1, Cause: "invalid_result", Reason: "operator judgment on related order"}, policyMeta(actor.ID))
		return e
	})
	evidence, e := f.service.Judgment(ctx, f.brand, o.ID)
	if e != nil || evidence == nil || evidence.DrawResultID != draw.ID || evidence.Cause != "invalid_result" {
		t.Fatalf("lost result lineage %+v %v", evidence, e)
	}
	history, e := rs.Draw(ctx, f.brand, p.ID, 50, 0)
	if e != nil || history.Current == nil || history.Current.ID != draw.ID {
		t.Fatal("individual judgment changed draw history")
	}
	if _, e = f.db.Exec(ctx, `UPDATE periods SET status='settling',version=version+1,state_reason='simulate actual settlement admission' WHERE id=$1`, p.ID); e != nil {
		t.Fatal(e)
	}
	tx, e = f.db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.service.JudgeCancel(ctx, tx, f.brand, actor, neighbor.ID, JudgeCancelInput{Version: 1, Cause: "invalid_result", Reason: "must use settlement reversal path"}, policyMeta(actor.ID))
	_ = tx.Rollback(ctx)
	if !errors.Is(e, ErrState) {
		t.Fatalf("judgment entered settling period %v", e)
	}
	if record, e := f.service.Judgment(ctx, f.brand, neighbor.ID); e != nil || record != nil || walletBySource(t, f)[0][0] != 9 {
		t.Fatalf("settling denial left changes %+v %v", record, e)
	}
}

func judgeActor(f bettingFixture) access.Account {
	a := periodCancelActor(f)
	a.Roles[0].Permissions = append(a.Roles[0].Permissions, access.Permission{Resource: "bet", Action: "judge_cancel", Scope: access.ScopeBrand})
	return a
}

func TestConcurrentJudgmentAndOrdinaryCancelHaveOneRefundWinner(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	fundBettingWallet(t, f, 10)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	actor := judgeActor(f)
	o, e := placeBettingOrder(t, f, f.input, "judge-cancel-competing-requests")
	if e != nil {
		t.Fatal(e)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, judge := range []bool{true, false} {
		go func(judge bool) {
			<-start
			tx, e := f.db.Begin(ctx)
			if e != nil {
				results <- e
				return
			}
			defer tx.Rollback(ctx)
			if judge {
				_, e = f.service.JudgeCancel(ctx, tx, f.brand, actor, o.ID, JudgeCancelInput{Version: 1, Cause: "no_result", Reason: "concurrent judgment"}, policyMeta(actor.ID))
			} else {
				_, e = f.service.CancelAdmin(ctx, tx, f.brand, actor, o.ID, 1, "concurrent ordinary cancellation", policyMeta(actor.ID))
			}
			if e == nil {
				e = tx.Commit(ctx)
			}
			results <- e
		}(judge)
	}
	close(start)
	success, conflict := 0, 0
	for range 2 {
		e := <-results
		if e == nil {
			success++
		} else if errors.Is(e, ErrVersion) {
			conflict++
		} else {
			t.Fatalf("unexpected conflict %v", e)
		}
	}
	if success != 1 || conflict != 1 || walletBySource(t, f)[0][0] != 10 {
		t.Fatalf("winner/conflict %d/%d", success, conflict)
	}
	current, e := f.service.Order(ctx, f.brand, f.member, o.ID)
	if e != nil || current.Version != 2 || current.RefundEntryID == "" {
		t.Fatal(current, e)
	}
	judgment, e := f.service.Judgment(ctx, f.brand, o.ID)
	if e != nil {
		t.Fatal(e)
	}
	if (current.Status == "judged_cancelled") != (judgment != nil) {
		t.Fatalf("loser left judgment evidence %+v %+v", current, judgment)
	}
	var refunds int
	if e = f.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE entry_type='refund'`).Scan(&refunds); e != nil || refunds != 1 {
		t.Fatalf("refunds %d %v", refunds, e)
	}
}
func TestSingleJudgmentRefundsMixedSourcesWithoutCancellingPeriodOrNeighbor(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	fundBettingWallet(t, f, 10, 10, 10)
	ctx := context.Background()
	actor := judgeActor(f)
	before := walletBySource(t, f)
	input := f.input
	input.Multiplier = 25
	o, e := placeBettingOrder(t, f, input, "mixed-single-judgment")
	if e != nil {
		t.Fatal(e)
	}
	neighbor, e := placeBettingOrder(t, f, f.input, "judgment-neighbor-order")
	if e != nil {
		t.Fatal(e)
	}
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		_, e := f.service.MarkAbnormal(ctx, tx, f.brand, actor, o.ID, 1, "retain abnormal history", policyMeta(actor.ID))
		return e
	})
	var judged Order
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var e error
		judged, e = f.service.JudgeCancel(ctx, tx, f.brand, actor, o.ID, JudgeCancelInput{Version: 2, Cause: "no_result", Reason: "operator judges this single order"}, policyMeta(actor.ID))
		return e
	})
	if judged.Status != "judged_cancelled" || judged.Version != 3 || judged.RefundEntryID == "" || walletBySource(t, f)[0][0] != 10 || walletBySource(t, f)[1][0] != 10 || walletBySource(t, f)[2][0] != 9 {
		t.Fatalf("incorrect source refund %+v", judged)
	}
	evidence, e := f.service.Judgment(ctx, f.brand, o.ID)
	if e != nil || evidence == nil || evidence.OrderVersion != 3 || evidence.Cause != "no_result" || evidence.DrawResultID != "" || evidence.JudgedBy != actor.ID || evidence.RefundEntryID != judged.RefundEntryID {
		t.Fatalf("missing immutable judgment %+v %v", evidence, e)
	}
	if exception, e := f.service.Exception(ctx, f.brand, o.ID); e != nil || exception == nil {
		t.Fatal("judgment erased prior exception")
	}
	current, e := f.service.Order(ctx, f.brand, f.member, neighbor.ID)
	if e != nil || current.Status != "placed" {
		t.Fatalf("neighbor was cancelled %+v %v", current, e)
	}
	var state string
	if e = f.db.QueryRow(ctx, `SELECT status FROM periods WHERE id=$1`, f.period.ID).Scan(&state); e != nil || state != "betting" {
		t.Fatalf("single action cancelled period %s %v", state, e)
	}
	for _, sql := range []string{`UPDATE bet_order_judgments SET reason='rewrite' WHERE order_id=$1`, `DELETE FROM bet_order_judgments WHERE order_id=$1`} {
		if _, e = f.db.Exec(ctx, sql, o.ID); e == nil {
			t.Fatal("editable judgment evidence")
		}
	}
	// Later whole-period cancellation captures only the remaining unrefunded order.
	var job Cancellation
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var e error
		job, e = f.service.CancelPeriod(ctx, tx, f.brand, actor, f.period.ID, CancelPeriodInput{Version: f.period.Version, Mode: "judged_cancelled", Cause: "no_result", Reason: "cancel the remaining period"}, policyMeta(actor.ID))
		return e
	})
	if job.TotalCount != 1 {
		t.Fatalf("single judgment counted twice %+v", job)
	}
	if _, e = f.service.ProcessCancellations(ctx, 10); e != nil || walletBySource(t, f) != before {
		t.Fatalf("remaining refund failed %v", e)
	}
}

func TestSingleJudgmentEventFailureRollsBackEvidenceStateAndMoney(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	fundBettingWallet(t, f, 10)
	ctx := context.Background()
	actor := judgeActor(f)
	o, e := placeBettingOrder(t, f, f.input, "judgment-event-rollback")
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.db.Exec(ctx, `CREATE FUNCTION reject_judgment_event() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.event_type='bet.order.judged_cancelled' THEN RAISE EXCEPTION 'test judgment rollback'; END IF; RETURN NEW; END $$;CREATE TRIGGER reject_judgment_event BEFORE INSERT ON outbox_events FOR EACH ROW EXECUTE FUNCTION reject_judgment_event()`)
	if e != nil {
		t.Fatal(e)
	}
	tx, e := f.db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.service.JudgeCancel(ctx, tx, f.brand, actor, o.ID, JudgeCancelInput{Version: 1, Cause: "invalid_result", Reason: "operator flags external candidate"}, policyMeta(actor.ID))
	_ = tx.Rollback(ctx)
	if e == nil {
		t.Fatal("missing failure")
	}
	current, e := f.service.Order(ctx, f.brand, f.member, o.ID)
	if e != nil || current.Status != "placed" || current.Version != 1 || current.RefundEntryID != "" || walletBySource(t, f)[0][0] != 9 {
		t.Fatalf("partial refund %+v %v", current, e)
	}
	record, e := f.service.Judgment(ctx, f.brand, o.ID)
	if e != nil || record != nil {
		t.Fatalf("orphan judgment %+v %v", record, e)
	}
}

func TestJudgmentRejectsAbsentGrantSuperForeignBrandAndStaleVersion(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	fundBettingWallet(t, f, 10)
	ctx := context.Background()
	actor := judgeActor(f)
	o, e := placeBettingOrder(t, f, f.input, ids.New())
	if e != nil {
		t.Fatal(e)
	}
	for _, test := range []struct {
		actor   access.Account
		brand   string
		version int64
		cause   string
		want    error
	}{
		{access.Account{ID: actor.ID, Type: access.AccountAdmin, BrandIDs: actor.BrandIDs}, f.brand, 1, "no_result", ErrDenied},
		{func() access.Account { a := actor; a.SuperAdmin = true; return a }(), f.brand, 1, "no_result", ErrDenied},
		{actor, f.brand, 2, "no_result", ErrVersion},
		{actor, f.brand, 1, "operator_cancel", ErrInvalid},
		{func() access.Account {
			a := actor
			a.BrandIDs = []string{storeTestOtherBrand}
			a.Roles = append([]access.Role{}, a.Roles...)
			a.Roles[0].BrandID = storeTestOtherBrand
			return a
		}(), storeTestOtherBrand, 1, "no_result", ErrNotFound},
	} {
		tx, e := f.db.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		_, e = f.service.JudgeCancel(ctx, tx, test.brand, test.actor, o.ID, JudgeCancelInput{Version: test.version, Cause: test.cause, Reason: "denial fixture"}, policyMeta(actor.ID))
		_ = tx.Rollback(ctx)
		if !errors.Is(e, test.want) {
			t.Fatalf("got %v want %v", e, test.want)
		}
	}
	if _, e = f.service.Judgment(ctx, storeTestOtherBrand, o.ID); !errors.Is(e, ErrNotFound) {
		t.Fatal(e)
	}
}
