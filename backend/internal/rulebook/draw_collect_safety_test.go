package rulebook

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/drawfeed"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
)

func TestExpiredDrawClaimCannotClearOrReplaceNewWorkerClaim(t *testing.T) {
	s, _, _, sources, p := prepareDrawCollector(t, 1)
	ctx := context.Background()
	old, ok, e := s.claimDraw(ctx, brand, p.ID)
	if e != nil || !ok {
		t.Fatalf("old claim: %v %v", ok, e)
	}
	if _, e = s.DB.Exec(ctx, `UPDATE periods SET draw_claim_until=clock_timestamp()-interval '1 second' WHERE id=$1`, p.ID); e != nil {
		t.Fatal(e)
	}
	fresh, ok, e := s.claimDraw(ctx, brand, p.ID)
	if e != nil || !ok || fresh.token == old.token {
		t.Fatalf("new claim: %v %v", ok, e)
	}
	attempts := []drawfeed.Attempt{{SourceID: sources[0].ID, Status: "success", Code: "ok"}}
	candidate := drawCandidate(t, s, p, rules.Draw{Digits: []int{1, 2, 3}})
	result := drawfeed.Result{SourceID: sources[0].ID, Candidate: candidate, Attempts: attempts}
	n, e := s.finishClaim(ctx, old, "accepted", attempts, result, nil)
	if e != nil || n != 0 {
		t.Fatalf("old completion: n=%d err=%v", n, e)
	}
	var token string
	if e = s.DB.QueryRow(ctx, `SELECT draw_claim_token::text FROM periods WHERE id=$1`, p.ID).Scan(&token); e != nil || token != fresh.token {
		t.Fatalf("new lease removed: %q %v", token, e)
	}
	n, e = s.finishClaim(ctx, fresh, "accepted", attempts, result, nil)
	if e != nil || n != 1 {
		t.Fatalf("fresh completion: n=%d err=%v", n, e)
	}
	history, e := s.Draw(ctx, brand, p.ID, 50, 0)
	if e != nil || len(history.History) != 1 || len(history.Attempts) != 2 || history.Attempts[0].Status != "accepted" || history.Attempts[1].Status != "discarded" {
		t.Fatalf("history: %+v %v", history, e)
	}
}

func TestCancelledDrawFetchPersistsAttemptAndReleasesClaim(t *testing.T) {
	s, _, _, sources, p := prepareDrawCollector(t, 2)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	resolver := resolverFor(drawAdapterFunc(func(fetchCtx context.Context, _ drawfeed.Source, _ drawfeed.Request) (drawfeed.Candidate, error) {
		calls++
		cancel()
		return drawfeed.Candidate{}, fetchCtx.Err()
	}))
	n, e := s.CollectDraws(ctx, resolver)
	if n != 0 || !errors.Is(e, context.Canceled) || calls != 1 {
		t.Fatalf("cancelled collection: n=%d calls=%d err=%v", n, calls, e)
	}
	history, e := s.Draw(context.Background(), brand, p.ID, 50, 0)
	if e != nil || history.Current != nil || len(history.Attempts) != 1 || len(history.Attempts[0].Attempts) != 1 || history.Attempts[0].Attempts[0].SourceID != sources[0].ID || history.Attempts[0].Attempts[0].Code != "cancelled" {
		t.Fatalf("cancel evidence: %+v %v", history, e)
	}
	var claim *string
	var next *time.Time
	if e = s.DB.QueryRow(context.Background(), `SELECT draw_claim_token::text,draw_next_poll_at FROM periods WHERE id=$1`, p.ID).Scan(&claim, &next); e != nil || claim != nil || next == nil {
		t.Fatalf("cleanup claim=%v next=%v err=%v", claim, next, e)
	}
}
