package betting

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/platformlottery"
	"github.com/jackc/pgx/v5"
	"math/big"
	"slices"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
)

func TestCanadaDistributionIncludesEveryPlayAndDraw(t *testing.T) {
	engine, _ := templateEngine(t, "canada_28", "big")
	var snapshot rules.ConfiguredSnapshot
	if err := json.Unmarshal(engine.Snapshot(), &snapshot); err != nil {
		t.Fatal(err)
	}
	purchases := []distributionPurchase{}
	for _, play := range snapshot.Catalog.Plays {
		option := snapshot.Catalog.Domains[play.Selection.Domain].Options[0].ID
		q, err := engine.Prepare(context.Background(), []rules.ConfiguredRequest{{PlayID: play.ID, Options: []string{option}, Plate: "A", Points: 1000, Multiplier: 1}})
		if err != nil {
			t.Fatal(err)
		}
		purchases = append(purchases, distributionPurchase{member: play.ID, catalog: snapshot.Catalog, engine: engine, quote: q})
	}
	out := BetDistribution{}
	if err := projectCanadaDistribution(context.Background(), purchases, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Numbers) != 1000 || out.Numbers[0].Number != 0 || out.Numbers[999].Number != 999 {
		t.Fatal("missing draws")
	}
	for n, item := range out.Numbers {
		total, returned := int64(0), int64(0)
		for _, p := range purchases {
			result, err := engine.SettleWithContext(context.Background(), p.quote, rules.Draw{Digits: []int{n / 100, n / 10 % 10, n % 10}}, engine.DefaultSettlementContext())
			if err != nil {
				t.Fatal(err)
			}
			total += int64(p.quote.BetPoints)
			returned += int64(result.PrizePoints + result.RefundPoints)
		}
		if item.Profit != distributionPoints(big.NewRat(total-returned, 1)) {
			t.Fatalf("%03d: %+v", n, item)
		}
	}
}

func TestCanadaDistributionBigAndSum14CoverAllMatchingDraws(t *testing.T) {
	for _, code := range []string{"big", "sum_14"} {
		engine, request := templateEngine(t, "canada_28", code)
		var snapshot rules.ConfiguredSnapshot
		json.Unmarshal(engine.Snapshot(), &snapshot)
		q, err := engine.Prepare(context.Background(), []rules.ConfiguredRequest{request})
		if err != nil {
			t.Fatal(err)
		}
		out := BetDistribution{}
		if err = projectCanadaDistribution(context.Background(), []distributionPurchase{{member: "member", catalog: snapshot.Catalog, engine: engine, quote: q}}, &out); err != nil {
			t.Fatal(err)
		}
		wins := 0
		for n, item := range out.Numbers {
			sum := n/100 + n/10%10 + n%10
			if code == "sum_14" && (item.Stake != "0.00") != (sum == 14) {
				t.Fatalf("sum14 coverage %03d: %+v", n, item)
			}
			if item.Stake != "0.00" {
				wins++
			}
		}
		if code == "sum_14" && wins != 75 {
			t.Fatalf("sum14: %d", wins)
		}
		if code == "big" && wins == 0 {
			t.Fatal("big missing")
		}
	}
}

func TestCanadaDistributionMemberPrizeCap(t *testing.T) {
	engine, request := templateEngine(t, "canada_28", "sum_14", func(c *rules.ConfiguredCatalog) { cap := points.Amount(3000); c.PeriodPrizeCap = &cap })
	var snapshot rules.ConfiguredSnapshot
	json.Unmarshal(engine.Snapshot(), &snapshot)
	q, err := engine.Prepare(context.Background(), []rules.ConfiguredRequest{request})
	if err != nil {
		t.Fatal(err)
	}
	out := BetDistribution{}
	purchases := []distributionPurchase{{member: "one", catalog: snapshot.Catalog, engine: engine, quote: q}, {member: "one", catalog: snapshot.Catalog, engine: engine, quote: q}, {member: "two", catalog: snapshot.Catalog, engine: engine, quote: q}}
	if err = projectCanadaDistribution(context.Background(), purchases, &out); err != nil {
		t.Fatal(err)
	}
	// First member receives 25 + 5, second receives its own 25.
	if out.TotalStake != "30.00" || out.Numbers[149].Profit != "-25.00" {
		t.Fatal(out.TotalStake, out.Numbers[149])
	}
}

func TestCanadaDistributionDatabaseAndBrandIsolation(t *testing.T) {
	c := templatePurchase(t, "canada_28", "sum_14", rules.Draw{Digits: []int{1, 4, 9}}, 2500, 0)
	ctx := context.Background()
	empty, err := c.f.service.BetDistribution(ctx, c.f.brand, c.g.ID, c.p.PeriodNo)
	if err != nil || len(empty.Numbers) != 1000 || empty.NumberDigits != 3 || empty.TotalStake != "0.00" {
		t.Fatal(empty.TotalStake, err)
	}
	fundBettingWallet(t, c.f, 100000)
	bettingTx(t, c.f.db, func(tx pgx.Tx) error {
		now := bettingDatabaseNow(t, c.f.db)
		var e error
		c.p, e = c.s.CreatePeriod(ctx, tx, c.a, c.g.ID, platformlottery.PeriodInput{PeriodNo: "canada-distribution", BetStartAt: now.Add(-time.Minute), BetEndAt: now.Add(time.Minute), DrawAt: now.Add(time.Minute)}, points.Metadata{})
		return e
	})
	c.input.PeriodID = c.p.ID
	bettingTx(t, c.f.db, func(tx pgx.Tx) error {
		_, e := c.f.service.PlaceConfigured(ctx, tx, c.f.brand, c.f.user, c.input, points.Metadata{RequestID: ids.New()})
		return e
	})
	out, err := c.f.service.BetDistribution(ctx, c.f.brand, c.g.ID, c.p.PeriodNo)
	if err != nil || out.TotalStake != "10.00" || out.Numbers[149].Profit != "-15.00" || out.Numbers[0].Profit != "10.00" {
		t.Fatal(out.TotalStake, err)
	}
	games, err := c.f.service.DistributionGames(ctx, c.f.brand)
	if err != nil || !slices.ContainsFunc(games, func(g ConfiguredGameSummary) bool { return g.ID == c.g.ID }) {
		t.Fatal(games, err)
	}
	if _, err = c.f.service.BetDistribution(ctx, storeTestOtherBrand, c.g.ID, c.p.PeriodNo); !errors.Is(err, ErrNotFound) {
		t.Fatal("brand isolation", err)
	}
}
