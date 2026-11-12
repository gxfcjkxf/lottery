package betting

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"slices"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/platformlottery"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

func TestBetDistributionFullPeriodCurrentPricesAndIsolation(t *testing.T) {
	engine, req := templateEngine(t, "mark_six", "zodiac_rat")
	// Template bindings put rat on 01,13,25,37,49.
	c := configuredFixtureFrom(t, engine.Snapshot(), []platformlottery.ConfiguredCase{{Requests: []rules.ConfiguredRequest{req}}})
	// The aggregation case writes 51 orders; keep it independent of the short
	// settlement fixture window and use PostgreSQL's admission clock.
	now := bettingDatabaseNow(t, c.f.db)
	bettingTx(t, c.f.db, func(tx pgx.Tx) error {
		var err error
		c.p, err = c.s.CreatePeriod(context.Background(), tx, c.a, c.g.ID, platformlottery.PeriodInput{
			PeriodNo: "distribution-51", BetStartAt: now.Add(-time.Minute),
			BetEndAt: now.Add(5 * time.Minute), DrawAt: now.Add(5 * time.Minute),
		}, points.Metadata{})
		return err
	})
	c.input.PeriodID = c.p.ID

	empty, err := c.f.service.BetDistribution(context.Background(), c.f.brand, c.g.ID, "")
	if err != nil || len(empty.Numbers) != 49 || empty.Numbers[0].Zodiac != "rat" || empty.Numbers[0].Wave != "red" {
		t.Fatal("empty period attributes", empty, err)
	}
	fundBettingWallet(t, c.f, 1000000)
	// More than one list page: aggregation must include all 51 purchases.
	for i := 0; i < 51; i++ {
		bettingTx(t, c.f.db, func(tx pgx.Tx) error {
			_, err := c.f.service.PlaceConfigured(context.Background(), tx, c.f.brand, c.f.user, c.input, points.Metadata{RequestID: ids.New()})
			return err
		})
	}
	ctx := context.Background()
	sales, err := platformlottery.ReadConfiguredSales(ctx, c.f.db, c.f.brand, c.g.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	sales.Prices.Plates["A"]["zodiac_rat"] = "1.5"
	bettingTx(t, c.f.db, func(tx pgx.Tx) error {
		_, err := c.s.WriteConfiguredSales(ctx, tx, c.b, c.f.brand, c.g.ID, sales, points.Metadata{})
		return err
	})
	out, err := c.f.service.BetDistribution(ctx, c.f.brand, c.g.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if out.TotalStake != "510.00" || len(out.Numbers) != 49 || out.PeriodNo != c.p.PeriodNo {
		t.Fatalf("bad period total: %+v", out)
	}
	var snapshot rules.ConfiguredSnapshot
	if err = json.Unmarshal(engine.Snapshot(), &snapshot); err != nil {
		t.Fatal(err)
	}
	for _, n := range out.Numbers {
		if !slices.Contains(snapshot.Bindings.Attributes["wave"][n.Wave], n.Number) || !slices.Contains(snapshot.Bindings.Attributes["zodiac"][n.Zodiac], n.Number) {
			t.Fatalf("attributes differ from purchase bindings: %+v", n)
		}
		if (n.Number-1)%12 == 0 {
			if n.Stake != "102.00" || n.Profit != "-255.00" {
				t.Fatalf("wrong split/return: %+v", n)
			}
		} else if n.Stake != "0.00" || n.Profit != "510.00" {
			t.Fatalf("wrong losing number: %+v", n)
		}
	}
	if _, err = c.f.service.BetDistribution(ctx, storeTestOtherBrand, c.g.ID, out.PeriodNo); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign brand", err)
	}
	if _, err = c.f.service.BetDistribution(ctx, c.f.brand, c.g.ID, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatal("missing period", err)
	}
	games, err := c.f.service.DistributionGames(ctx, storeTestOtherBrand)
	if err != nil || len(games) != 0 {
		t.Fatal("foreign game directory", games, err)
	}
	// Stored purchase prices and funded totals are unchanged by this read.
	var raw []byte
	if err = c.f.db.QueryRow(ctx, `SELECT quote FROM configured_orders WHERE brand_id=$1 LIMIT 1`, c.f.brand).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var saved rules.ConfiguredQuote
	if err = json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Lines[0].Prices["win"] != "2.5" {
		t.Fatal("modified purchase price", saved)
	}
}

func TestDistributionFractionRoundingAfterAggregation(t *testing.T) {
	sum := new(big.Rat)
	for i := 0; i < 3; i++ {
		sum.Add(sum, big.NewRat(1000, 3))
	}
	if got := distributionPoints(sum); got != "10.00" {
		t.Fatal(got)
	}
}
