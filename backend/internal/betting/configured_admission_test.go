package betting

import (
	"context"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/notification"
	"github.com/gxfcjkxf/lottery/backend/internal/platformlottery"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"sync"
	"testing"
)

func TestConfiguredPeriodLimitSerializesDifferentMembers(t *testing.T) {
	c := newConfiguredFixture(t)
	other := configuredMember(t, c, c.f.brand)
	ctx := context.Background()
	sale, e := platformlottery.ReadConfiguredSales(ctx, c.f.db, c.f.brand, c.g.ID, false)
	if e != nil {
		t.Fatal(e)
	}
	sale.MaxPoints = 1000
	sale.MaxPeriodPoints = 1000
	sale.MaxUserPeriodPoints = 1000
	bettingTx(t, c.f.db, func(tx pgx.Tx) error {
		var e error
		sale, e = c.s.WriteConfiguredSales(ctx, tx, c.b, c.f.brand, c.g.ID, sale, points.Metadata{})
		return e
	})
	c.input.SalesVersion = sale.Version
	other.input.SalesVersion = sale.Version
	fundBettingWallet(t, c.f, 10000)
	fundBettingWallet(t, other.f, 10000)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, fixture := range []configuredFixture{c, other} {
		wg.Add(1)
		go func(fixture configuredFixture) {
			defer wg.Done()
			_, e := configuredPlace(t, fixture, ids.New(), fixture.input)
			errs <- e
		}(fixture)
	}
	wg.Wait()
	close(errs)
	success, limited := 0, 0
	for err := range errs {
		if err == nil {
			success++
		} else if errors.Is(err, ErrLimit) {
			limited++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || limited != 1 {
		t.Fatal("period quota race", success, limited)
	}
	a, _ := (points.Store{DB: c.f.db}).Read(ctx, c.f.brand, c.f.member)
	b, _ := (points.Store{DB: c.f.db}).Read(ctx, c.f.brand, other.f.member)
	if a.AvailablePoints+b.AvailablePoints != 19000 {
		t.Fatal("quota rejection changed funds", a, b)
	}
}
func TestConfiguredOrderNotificationsUseCurrentFinancialEvidence(t *testing.T) {
	c := newConfiguredFixture(t)
	ctx := context.Background()
	fundBettingWallet(t, c.f, 10000)
	if _, err := configuredPlace(t, c, ids.New(), c.input); err != nil {
		t.Fatal(err)
	}
	configuredPublishDraw(t, c, 7)
	if _, err := c.f.service.ProcessConfiguredSettlements(ctx, 20); err != nil {
		t.Fatal(err)
	}
	svc := notification.Service{DB: c.f.db}
	if _, err := svc.Process(ctx, 100); err != nil {
		t.Fatal(err)
	}
	page, err := svc.List(ctx, c.f.brand, c.f.member, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for _, item := range page.Items {
		seen[item.EventType]++
	}
	if seen["bet.order.placed"] != 1 || seen["bet.order.won"] != 1 {
		t.Fatal("configured notifications", seen)
	}
	if _, err = svc.Process(ctx, 100); err != nil {
		t.Fatal(err)
	}
	replay, err := svc.List(ctx, c.f.brand, c.f.member, 50, 0)
	if err != nil || len(replay.Items) != len(page.Items) {
		t.Fatal("notification replay", replay, err)
	}
}
