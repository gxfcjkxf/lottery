package betting

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/reporting"
	"github.com/jackc/pgx/v5"
	"testing"
	"time"
)

func reportFixtureQuery(f bettingFixture, group string) reporting.Query {
	member, game := f.member, f.game.ID
	return reporting.Query{From: time.Now().Add(-time.Hour), To: time.Now().Add(time.Hour), GroupBy: group, Limit: 20, GameID: &game, MemberID: &member}
}

func TestReportsCancellationAbnormalPartitionAndBrandDay(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	ctx := context.Background()
	fundBettingWallet(t, f, 30)
	setUserCancellation(t, f, true)
	f.input.PolicyVersions = ptrPolicyVersions(readBettingPolicyVersions(t, f.service, f.brand, f.game.ID))
	one, e := placeBettingOrder(t, f, f.input, "report-cancel-first")
	if e != nil {
		t.Fatal(e)
	}
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		_, e := f.service.Cancel(ctx, tx, f.brand, f.user, one.ID, one.Version, "report cancellation evidence", points.Metadata{RequestID: "report-cancel"})
		return e
	})
	two, e := placeBettingOrder(t, f, f.input, "report-abnormal-second")
	if e != nil {
		t.Fatal(e)
	}
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		_, e := f.service.MarkAbnormal(ctx, tx, f.brand, exceptionActor(f), two.ID, two.Version, "report excluded abnormal", policyMeta(f.version.CreatedBy))
		return e
	})
	if _, e = f.db.Exec(ctx, `UPDATE brands SET timezone='Etc/GMT+12' WHERE id=$1`, f.brand); e != nil {
		t.Fatal(e)
	}
	s := reporting.Service{DB: f.db}
	q := reportFixtureQuery(f, "day")
	r, e := s.Betting(ctx, f.brand, q)
	if e != nil || r.Summary.OrderCount != "2" || r.Summary.StakePoints != "2" || r.Summary.CancelledCount != "1" || r.Summary.RefundPoints != "1" || r.Summary.AbnormalCount != "1" || r.Summary.AbnormalStakePoints != "1" || r.Summary.UnfinalizedStakePoints != "0" || len(r.Items) != 1 || r.Items[0].Key != one.PlacedAt.Add(-12*time.Hour).Format("2006-01-02") {
		t.Fatal(r, e)
	}
	q.GroupBy = "member"
	m, e := s.Betting(ctx, f.brand, q)
	if e != nil || len(m.Items) != 1 || m.Items[0].Key != f.member || m.Summary != r.Summary {
		t.Fatal(m, e)
	}
}
func TestReportsFinalProjectionAndLedgerRemainDistinctAcrossCorrections(t *testing.T) {
	f, o, _ := settledCorrectionFixture(t)
	ctx := context.Background()
	s := reporting.Service{DB: f.db}
	q := reportFixtureQuery(f, "game")
	initial, e := s.Betting(ctx, f.brand, q)
	if e != nil {
		t.Fatal(e)
	}
	if initial.Summary.OrderCount != "1" || initial.Summary.StakePoints != "1" || initial.Summary.SettledStakePoints != "1" || initial.Summary.CurrentPrizePoints != "10" || len(initial.Items) != 1 || initial.Items[0].Key != f.game.ID {
		t.Fatal(initial)
	}
	createFixtureCorrection(t, f, correctedDigits(1, 2, 2))
	open, e := s.Betting(ctx, f.brand, q)
	if e != nil || open.Summary.CurrentPrizePoints != "0" || open.Summary.UnfinalizedStakePoints != "1" || open.Summary.CorrectionOpenCount != "1" {
		t.Fatal(open, e)
	}
	if _, e = f.service.ProcessCorrections(ctx, 20); e != nil {
		t.Fatal(e)
	}
	if _, e = f.service.ProcessSettlements(ctx, 20); e != nil {
		t.Fatal(e)
	}
	loss, e := s.Betting(ctx, f.brand, q)
	if e != nil || loss.Summary.LostCount != "1" || loss.Summary.CurrentPrizePoints != "0" || loss.Summary.SettledStakePoints != "1" || loss.Summary.UnfinalizedStakePoints != "0" {
		t.Fatal(loss, e)
	}
	lq := q
	lq.GameID = nil
	lq.GroupBy = "entry_type"
	ledger, e := s.Ledger(ctx, f.brand, lq)
	if e != nil || ledger.Summary.PrizeCreditPoints != "10" || ledger.Summary.PrizeReversalPoints != "10" || ledger.Balances.TotalPoints != "99" {
		t.Fatal(ledger, e)
	}
	// A corrected win contributes just the new final projection. Actual historical
	// ledger still contains both credits and the compensating debit.
	createFixtureCorrection(t, f, correctedDigits(1, 2, 1))
	if _, e = f.service.ProcessCorrections(ctx, 20); e != nil {
		t.Fatal(e)
	}
	if _, e = f.service.ProcessSettlements(ctx, 20); e != nil {
		t.Fatal(e)
	}
	final, e := s.Betting(ctx, f.brand, q)
	if e != nil || final.Summary.CurrentPrizePoints != "10" || final.Summary.OrderCount != "1" {
		t.Fatal(final, e)
	}
	before := walletBySource(t, f)
	ledger, e = s.Ledger(ctx, f.brand, lq)
	if e != nil || ledger.Summary.PrizeCreditPoints != "20" || ledger.Summary.PrizeReversalPoints != "10" || ledger.Balances.TotalPoints != "109" {
		t.Fatal(ledger, e)
	}
	if walletBySource(t, f) != before {
		t.Fatal("report mutated balances")
	}
	// Half-open order window includes creation at from, but excludes at to.
	at := o.PlacedAt
	q.From = at
	q.To = at.Add(time.Nanosecond)
	included, e := s.Betting(ctx, f.brand, q)
	if e != nil || included.Summary.OrderCount != "1" {
		t.Fatal(included, e)
	}
	q.From = at.Add(-time.Hour)
	q.To = at
	excluded, e := s.Betting(ctx, f.brand, q)
	if e != nil || excluded.Summary.OrderCount != "0" || len(excluded.Items) != 0 {
		t.Fatal(excluded, e)
	}
}
