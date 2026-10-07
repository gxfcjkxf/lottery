package betting

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/jackc/pgx/v5"
)

func commissionCyclePage(t *testing.T, f bettingFixture, brand string, calendar commission.Calendar, window commission.Window, cursor *commission.CycleCursor, limit int) (commission.CyclePage, error) {
	t.Helper()
	ctx := context.Background()
	tx, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	return (commission.Source{}).CyclePageTx(ctx, tx, brand, calendar, window, cursor, limit)
}

func TestCommissionCyclePagesUsePlacementTimeEvenWhenSettlementCrossesBoundary(t *testing.T) {
	f := newBettingFixtureWithWindow(t, storeTestBrand, 6*time.Second, 6100*time.Millisecond)
	eligibilityFund(t, f, 10, 0, 0)
	first, err := placeBettingOrder(t, f, f.input, "cycle-first")
	if err != nil {
		t.Fatal(err)
	}
	second, err := placeBettingOrder(t, f, f.input, "cycle-second")
	if err != nil {
		t.Fatal(err)
	}
	third, err := placeBettingOrder(t, f, f.input, "cycle-cancelled")
	if err != nil {
		t.Fatal(err)
	}
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		_, err := f.service.CancelAdmin(context.Background(), tx, f.brand, judgeActor(f), third.ID, third.Version, "cycle excludes cancelled order", policyMeta(f.version.CreatedBy))
		return err
	})
	// A configurable real weekly boundary lies after all bets and before the
	// actual draw/settlement. No historical timestamps or financial facts change.
	boundary := f.period.BetEndAt.UTC().Truncate(time.Second)
	weekday := int(boundary.Weekday())
	calendar := commission.Calendar{Timezone: "UTC", Cycle: "weekly", BoundaryTime: boundary.Format("15:04:05"), Weekday: &weekday}
	window, err := calendar.WindowAt(first.PlacedAt)
	if err != nil || !window.To.Equal(boundary) || !third.PlacedAt.Before(boundary) {
		t.Fatal("fixture bets must precede the real commission boundary", window, boundary, err)
	}
	before := commissionFingerprint(t, f)
	page, err := commissionCyclePage(t, f, f.brand, calendar, window, nil, 2)
	if err != nil || len(page.Items) != 2 || page.Next == nil || page.Items[0].OrderID != first.ID || page.Items[1].OrderID != second.ID {
		t.Fatal(page, err)
	}
	for _, item := range page.Items {
		if item.Reason != "not_final" || item.Fact != nil {
			t.Fatal("pending order must be visible, not a zero or omitted", item)
		}
	}
	last, err := commissionCyclePage(t, f, f.brand, calendar, window, page.Next, 2)
	if err != nil || len(last.Items) != 1 || last.Next != nil || last.Items[0].OrderID != third.ID || last.Items[0].Reason != "cancelled" {
		t.Fatal("keyset pagination lost or duplicated a bet", last, err)
	}
	if commissionFingerprint(t, f) != before {
		t.Fatal("cycle reads changed business state")
	}
	eligibilitySettle(t, f)
	before = commissionFingerprint(t, f)
	final, err := commissionCyclePage(t, f, f.brand, calendar, window, nil, 100)
	if err != nil || len(final.Items) != 3 || final.Next != nil {
		t.Fatal(final, err)
	}
	for _, item := range final.Items[:2] {
		if item.Reason != "final" || item.Fact == nil || item.Fact.SettledAt.Before(window.To) || !item.Fact.PlacedAt.Before(window.To) {
			t.Fatal("late settlement lost its original betting cycle", item)
		}
	}
	nextWindow, err := calendar.WindowAt(window.To)
	if err != nil {
		t.Fatal(err)
	}
	next, err := commissionCyclePage(t, f, f.brand, calendar, nextWindow, nil, 100)
	if err != nil || len(next.Items) != 0 || next.Next != nil {
		t.Fatal("late settlement was moved into the next cycle", next, err)
	}
	other, err := commissionCyclePage(t, f, storeTestOtherBrand, calendar, window, nil, 100)
	if err != nil || len(other.Items) != 0 {
		t.Fatal("cycle reader crossed brands", other, err)
	}
	if commissionFingerprint(t, f) != before {
		t.Fatal("final cycle reads created commission or financial records")
	}
	// The first order is valid and read before a damaged second order. Even
	// then the caller must receive no usable partial page. Fault injection and
	// trigger changes are confined to this owned schema's rollback transaction.
	tx, err := f.db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(context.Background(), `ALTER TABLE point_ledger_entries DISABLE TRIGGER ledger_immutable`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(context.Background(), `UPDATE point_ledger_entries SET after_snapshot=jsonb_set(after_snapshot,'{winning,available}','"0"'::jsonb)
 WHERE id=(SELECT payout_entry_id FROM bet_orders WHERE id=$1)`, second.ID); err != nil {
		t.Fatal(err)
	}
	damaged, err := (commission.Source{}).CyclePageTx(context.Background(), tx, f.brand, calendar, window, nil, 100)
	if !errors.Is(err, commission.ErrEvidence) || len(damaged.Items) != 0 || damaged.Next != nil {
		t.Fatal("damaged later witness exposed a usable partial cycle", damaged, err)
	}
	if err = tx.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	if commissionFingerprint(t, f) != before {
		t.Fatal("cycle fault injection did not restore complete business state")
	}
}

func TestCommissionCyclePagesValidateWindowCursorAndFailWithoutPartialPage(t *testing.T) {
	f, order, _ := settledCorrectionFixture(t)
	day := 1
	calendar := commission.Calendar{Timezone: "UTC", Cycle: "monthly", BoundaryTime: "00:00:00", MonthDay: &day, ShortMonth: "last_day"}
	window, err := calendar.WindowAt(order.PlacedAt)
	if err != nil {
		t.Fatal(err)
	}
	before := commissionFingerprint(t, f)
	for _, test := range []struct {
		brand  string
		window commission.Window
		cursor *commission.CycleCursor
		limit  int
		err    error
	}{
		{f.brand, window, nil, 0, commission.ErrInvalid},
		{f.brand, window, nil, 101, commission.ErrInvalid},
		{"invalid", window, nil, 1, commission.ErrInvalid},
		{ids.New(), window, nil, 1, commission.ErrNotFound},
		{f.brand, commission.Window{From: window.From.Add(time.Second), To: window.To}, nil, 1, commission.ErrInvalid},
		{f.brand, commission.Window{From: window.From, To: window.To.Add(time.Second)}, nil, 1, commission.ErrInvalid},
		{f.brand, window, &commission.CycleCursor{OrderID: order.ID, PlacedAt: order.PlacedAt.Add(time.Second)}, 1, commission.ErrInvalid},
		{f.brand, window, &commission.CycleCursor{OrderID: ids.New(), PlacedAt: order.PlacedAt}, 1, commission.ErrInvalid},
		{storeTestOtherBrand, window, &commission.CycleCursor{OrderID: order.ID, PlacedAt: order.PlacedAt}, 1, commission.ErrInvalid},
		{f.brand, window, &commission.CycleCursor{OrderID: order.ID, PlacedAt: window.To}, 1, commission.ErrInvalid},
	} {
		page, err := commissionCyclePage(t, f, test.brand, calendar, test.window, test.cursor, test.limit)
		if !errors.Is(err, test.err) || len(page.Items) != 0 || page.Next != nil {
			t.Fatal("invalid input yielded a usable page", test, page, err)
		}
	}
	ctx := context.Background()
	if _, err := (commission.Source{}).CyclePageTx(ctx, nil, f.brand, calendar, window, nil, 1); !errors.Is(err, commission.ErrInvalid) {
		t.Fatal(err)
	}
	lock, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback(ctx)
	if _, err = lock.Exec(ctx, `SELECT id FROM periods WHERE id=$1 FOR UPDATE`, f.period.ID); err != nil {
		t.Fatal(err)
	}
	page, err := commissionCyclePage(t, f, f.brand, calendar, window, nil, 100)
	if !errors.Is(err, commission.ErrBusy) || len(page.Items) != 0 || page.Next != nil {
		t.Fatal("busy cycle yielded a usable partial page", page, err)
	}
	if err = lock.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if commissionFingerprint(t, f) != before {
		t.Fatal("rejected cycle reads changed financial state")
	}
}
