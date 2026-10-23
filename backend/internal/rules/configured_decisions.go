package rules

import (
	"math"
	"math/big"
	"slices"

	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

func validateConfiguredDecisions(c ConfiguredCatalog, p ConfiguredPlay) error {
	d := p.Decisions
	if d == nil || c.Model.Type != "DIGITS_0_9" || c.Model.Length > 4 || p.Selection.Mode != "singles" || p.Selection.Min != 1 || p.Selection.Max != 1 || p.PushWhen != nil || len(p.Tiers) != 0 || p.MixedPolicy != "" || p.Cap != nil || p.RoundingScope != "line" || p.RiskMultiplier < 1 {
		return ErrInvalid
	}
	count := 1
	for i := 0; i < c.Model.Length; i++ {
		count *= 10
	}
	if len(d.Index) != count || len(d.Outcomes) == 0 || len(d.Outcomes) > 100 {
		return ErrInvalid
	}
	for _, v := range d.Outcomes {
		if !slices.Contains([]string{"won", "lost", "push"}, v.Outcome) || v.LossFactor < 0 || v.LossFactor > p.RiskMultiplier {
			return ErrInvalid
		}
		if v.Outcome == "won" {
			if !configuredID(v.PriceKey) {
				return ErrInvalid
			}
			rate, e := parseOdds(v.PrizeFactor)
			if e != nil || rate.Sign() <= 0 {
				return ErrInvalid
			}
		} else if v.PriceKey != "" || v.PrizeFactor != "" {
			return ErrInvalid
		}
		if c.Funding != "reserve" && (p.RiskMultiplier != 1 || v.Outcome == "push" && v.LossFactor != 0 || v.Outcome != "push" && v.LossFactor != 1) {
			return ErrInvalid
		}
	}
	valid := func(index []int) bool {
		if len(index) != count {
			return false
		}
		for _, i := range index {
			if i < 0 || i >= len(d.Outcomes) {
				return false
			}
		}
		return true
	}
	if !valid(d.Index) {
		return ErrInvalid
	}
	if d.Alternate != nil {
		a := d.Alternate
		if !configuredID(a.Aggregate) || a.AtLeast < 1 || len(c.Aggregates[a.Aggregate]) == 0 || !valid(a.Index) {
			return ErrInvalid
		}
	}
	return nil
}
func (e *ConfiguredEngine) decisionMaximum(p ConfiguredPlay, line ConfiguredQuoteLine) (*big.Rat, error) {
	max := new(big.Rat)
	for _, o := range p.Decisions.Outcomes {
		if o.Outcome != "won" {
			continue
		}
		price, err := parseOdds(line.Prices[o.PriceKey])
		if err != nil {
			return nil, err
		}
		factor, err := parseOdds(o.PrizeFactor)
		if err != nil {
			return nil, err
		}
		price.Mul(price, factor)
		price.Mul(price, big.NewRat(int64(line.Stake), 1))
		if price.Cmp(max) > 0 {
			max.Set(price)
		}
	}
	return max, nil
}
func (e *ConfiguredEngine) settleDecision(p ConfiguredPlay, line ConfiguredQuoteLine, draw Draw, ctx ConfiguredSettlementContext) (ConfiguredLineResult, error) {
	out := ConfiguredLineResult{PlayID: p.ID, Options: append([]string(nil), line.Options...), Raw: "0"}
	key := 0
	for _, n := range draw.Digits {
		key = key*10 + n
	}
	d := p.Decisions
	index := d.Index
	if d.Alternate != nil {
		amount, ok := ctx.Aggregates[d.Alternate.Aggregate]
		if !ok {
			return out, ErrInvalid
		}
		if amount >= d.Alternate.AtLeast {
			index = d.Alternate.Index
		}
	}
	if key < 0 || key >= len(index) {
		return out, ErrInvalid
	}
	v := d.Outcomes[index[key]]
	out.Outcome = v.Outcome
	if v.Effective {
		out.Effective = line.Stake
	}
	if int64(line.Stake) > math.MaxInt64/int64(p.RiskMultiplier) {
		return out, ErrLimit
	}
	out.Refund = line.Stake * points.Amount(p.RiskMultiplier-v.LossFactor)
	if v.Outcome == "won" {
		price, err := parseOdds(line.Prices[v.PriceKey])
		if err != nil {
			return out, err
		}
		factor, err := parseOdds(v.PrizeFactor)
		if err != nil {
			return out, err
		}
		price.Mul(price, factor)
		price.Mul(price, big.NewRat(int64(line.Stake), 1))
		out.Raw = price.RatString()
		out.Prize, err = exactPoints(price, nil)
		if err != nil {
			return out, err
		}
	}
	return out, nil
}

// FundingAmount is calculated from the frozen quote; it is never client supplied.
func (e *ConfiguredEngine) FundingAmount(q ConfiguredQuote) points.Amount {
	if e.snapshot.Catalog.Funding == "reserve" {
		return q.ReservedPoints
	}
	return q.BetPoints
}
func (e *ConfiguredEngine) FundingMode() string { return e.snapshot.Catalog.Funding }
