package rules

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"math/big"
)

const maxTraceNodes = 200000

func roundedInteger(v *big.Rat) *big.Int {
	twice := new(big.Int).Lsh(new(big.Int).Set(v.Num()), 1)
	twice.Add(twice, v.Denom())
	den := new(big.Int).Lsh(new(big.Int).Set(v.Denom()), 1)
	return new(big.Int).Quo(twice, den)
}
func exactPoints(value *big.Rat, cap *points.Amount) (points.Amount, error) {
	v := new(big.Rat).Set(value)
	if cap != nil {
		maximum := new(big.Rat).SetInt64(int64(*cap))
		if v.Cmp(maximum) > 0 {
			v = maximum
		}
	}
	// Positive half-up: floor((2*n+d)/(2*d)), with arbitrary-precision arithmetic.
	rounded := roundedInteger(v)
	if !rounded.IsInt64() || rounded.Sign() < 0 {
		return 0, points.ErrOverflow
	}
	return points.Amount(rounded.Int64()), nil
}
func Simulate(in SimulationInput) (Simulation, error) {
	return SimulateContext(context.Background(), in)
}
func SimulateContext(ctx context.Context, in SimulationInput) (Simulation, error) {
	out := Simulation{Multiplier: in.Multiplier, Lines: []LineResult{}, Warnings: []string{}}
	d := in.Definition
	if err := ValidateDefinition(d); err != nil {
		return out, err
	}
	if in.Multiplier <= 0 || in.Multiplier > d.Limits.MaxMultiplier {
		return out, ErrLimit
	}
	if err := ValidateDraw(d.Model, in.Draw); err != nil {
		return out, err
	}
	normalized, err := NormalizeSelection(d, in.Selection)
	if err != nil {
		return out, err
	}
	lines, err := Expand(d, normalized)
	if err != nil {
		return out, err
	}
	nodes := 0
	for _, tier := range d.PrizeTiers {
		nodes += nodeCount(tier.Condition)
	}
	if len(lines) > maxTraceNodes/nodes {
		return out, ErrLimit
	}
	stake := new(big.Int).Mul(big.NewInt(int64(d.UnitPoints)), big.NewInt(int64(len(lines))))
	stake.Mul(stake, big.NewInt(int64(in.Multiplier)))
	if !stake.IsInt64() {
		return out, points.ErrOverflow
	}
	out.BetPoints = points.Amount(stake.Int64())
	if d.Limits.MaxBetPoints != nil && out.BetPoints > *d.Limits.MaxBetPoints {
		return out, ErrLimit
	}
	out.Normalized = normalized
	out.CombinationCount = len(lines)
	out.Warnings = Warnings(d)
	total := new(big.Rat)
	for _, line := range lines {
		if err = ctx.Err(); err != nil {
			return out, err
		}
		result := LineResult{Selection: line, Hits: []TierHit{}}
		rawAmounts := make([]*big.Rat, len(d.PrizeTiers))
		exclusiveHit := false
		bestAll, bestExclusive := -1, -1
		for i, tier := range d.PrizeTiers {
			trace, e := Evaluate(d, tier.Condition, line, in.Draw)
			if e != nil {
				return out, e
			}
			odds, _ := parseOdds(tier.Odds)
			raw := new(big.Rat).Mul(odds, new(big.Rat).SetInt64(int64(d.UnitPoints)))
			raw.Mul(raw, new(big.Rat).SetInt64(int64(in.Multiplier)))
			if !trace.Matched {
				raw.SetInt64(0)
			}
			rawText := raw.RatString()
			if tier.CapPoints != nil && raw.Cmp(new(big.Rat).SetInt64(int64(*tier.CapPoints))) > 0 {
				raw.SetInt64(int64(*tier.CapPoints))
			}
			cappedText := raw.RatString()
			if d.RoundingScope == "tier" {
				raw.SetInt(roundedInteger(raw))
			}
			rawAmounts[i] = raw
			rounded := roundedInteger(raw).String()
			result.Hits = append(result.Hits, TierHit{Code: tier.Code, Exclusive: tier.Exclusive, Matched: trace.Matched, RawPoints: rawText, CappedPoints: cappedText, Points: rounded, Trace: trace})
			if trace.Matched {
				if bestAll < 0 || raw.Cmp(rawAmounts[bestAll]) > 0 {
					bestAll = i
				}
				if tier.Exclusive {
					exclusiveHit = true
					if bestExclusive < 0 || raw.Cmp(rawAmounts[bestExclusive]) > 0 {
						bestExclusive = i
					}
				}
			}
		}
		lineTotal := new(big.Rat)
		for i, tier := range d.PrizeTiers {
			if !result.Hits[i].Matched {
				continue
			}
			selected := true
			if exclusiveHit {
				if d.MixedTierPolicy == "max_exclusive_plus_additive" {
					selected = !tier.Exclusive || i == bestExclusive
				} else {
					selected = i == bestAll
				}
			}
			result.Hits[i].Selected = selected
			if selected {
				out.Won = true
				lineTotal.Add(lineTotal, rawAmounts[i])
			}
		}
		result.Points = roundedInteger(lineTotal).String()
		if d.RoundingScope == "line" {
			lineTotal.SetInt(roundedInteger(lineTotal))
		}
		total.Add(total, lineTotal)
		out.Lines = append(out.Lines, result)
	}
	out.RawPrizePoints = total.RatString()
	cappedTotal := new(big.Rat).Set(total)
	if d.CapPoints != nil && cappedTotal.Cmp(new(big.Rat).SetInt64(int64(*d.CapPoints))) > 0 {
		cappedTotal.SetInt64(int64(*d.CapPoints))
	}
	out.CappedPrizePoints = cappedTotal.RatString()
	out.PrizePoints, err = exactPoints(total, d.CapPoints)
	if err != nil {
		return out, err
	}
	return out, nil
}
