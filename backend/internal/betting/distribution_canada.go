package betting

import (
	"context"
	"encoding/json"
	"math/big"
	"slices"

	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

type distributionPurchase struct {
	member  string
	catalog rules.ConfiguredCatalog
	engine  *rules.ConfiguredEngine
	quote   rules.ConfiguredQuote
}

func canadaDistribution(ctx context.Context, tx pgx.Tx, brand, game, period string, prices rules.ConfiguredPrices, out *BetDistribution) error {
	rows, err := tx.Query(ctx, `SELECT member_id::text,snapshot,quote FROM configured_orders WHERE brand_id=$1 AND game_id=$2 AND period_id=$3 AND status IN ('placed','settled') ORDER BY id`, brand, game, period)
	if err != nil {
		return err
	}
	defer rows.Close()
	purchases := []distributionPurchase{}
	engines := map[string]*rules.ConfiguredEngine{}
	for rows.Next() {
		var p distributionPurchase
		var raw []byte
		var purchased rules.ConfiguredQuote
		if err = rows.Scan(&p.member, &raw, &purchased); err != nil {
			return err
		}
		var snapshot rules.ConfiguredSnapshot
		if err = json.Unmarshal(raw, &snapshot); err != nil {
			return err
		}
		p.catalog = snapshot.Catalog
		if p.catalog.Model.Type != "DIGITS_0_9" || p.catalog.Model.Length != 3 || !p.catalog.Model.AllowRepeat || !p.catalog.Model.Ordered {
			return ErrSnapshot
		}
		p.engine = engines[string(raw)]
		if p.engine == nil {
			p.engine, err = rules.NewConfiguredEngine(p.catalog, snapshot.Bindings, prices)
			if err != nil {
				return ErrSnapshot
			}
			engines[string(raw)] = p.engine
		}
		p.quote, err = p.engine.Prepare(ctx, purchased.Requests)
		if err != nil {
			return ErrSnapshot
		}
		purchases = append(purchases, p)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	return projectCanadaDistribution(ctx, purchases, out)
}

// Each outcome settles every purchase. Member aggregates and prize caps use the
// complete period, independently for each hypothetical draw.
func projectCanadaDistribution(ctx context.Context, purchases []distributionPurchase, out *BetDistribution) error {
	total := new(big.Int)
	stakes, returns := make([]*big.Rat, 1000), make([]*big.Int, 1000)
	paid := map[string][]points.Amount{}
	for n := range stakes {
		stakes[n], returns[n] = new(big.Rat), new(big.Int)
	}
	for _, p := range purchases {
		if paid[p.member] == nil {
			paid[p.member] = make([]points.Amount, 1000)
		}
		settlement := p.engine.DefaultSettlementContext()
		for aggregate, plays := range p.catalog.Aggregates {
			for _, other := range purchases {
				if other.member != p.member {
					continue
				}
				for _, line := range other.quote.Lines {
					if slices.Contains(plays, line.PlayID) {
						settlement.Aggregates[aggregate] += line.Stake
					}
				}
			}
		}
		total.Add(total, big.NewInt(int64(p.engine.FundingAmount(p.quote))))
		results := make([]rules.ConfiguredResult, 1000)
		wins := make([]int, len(p.quote.Lines))
		for n := range results {
			if settlement.PrizeRemaining != nil {
				remaining := *p.catalog.PeriodPrizeCap - paid[p.member][n]
				if remaining < 0 {
					remaining = 0
				}
				settlement.PrizeRemaining = &remaining
			}
			result, err := p.engine.SettleWithContext(ctx, p.quote, rules.Draw{Digits: []int{n / 100, n / 10 % 10, n % 10}}, settlement)
			if err != nil {
				return err
			}
			results[n] = result
			paid[p.member][n] += result.PrizePoints
			returns[n].Add(returns[n], big.NewInt(int64(result.PrizePoints)))
			returns[n].Add(returns[n], big.NewInt(int64(result.RefundPoints)))
			for i, line := range result.Lines {
				if line.Outcome == "won" {
					wins[i]++
				}
			}
		}
		for n, result := range results {
			for i, line := range result.Lines {
				if line.Outcome == "won" {
					stakes[n].Add(stakes[n], big.NewRat(int64(p.quote.Lines[i].Stake), int64(wins[i])))
				}
			}
		}
	}
	out.TotalStake = distributionPoints(new(big.Rat).SetInt(total))
	for n := range stakes {
		profit := new(big.Int).Sub(total, returns[n])
		out.Numbers = append(out.Numbers, DistributionNumber{Number: n, Stake: distributionPoints(stakes[n]), Profit: distributionPoints(new(big.Rat).SetInt(profit))})
	}
	return nil
}
