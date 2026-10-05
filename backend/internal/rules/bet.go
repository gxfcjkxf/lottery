package rules

import (
	"context"
	"math/big"

	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

// BetQuote contains the validated, expanded stake for a selection. It does not
// evaluate a draw or estimate any prize.
type BetQuote struct {
	Normalized       Selection     `json:"normalized"`
	Expanded         []Selection   `json:"expanded_bets"`
	CombinationCount int           `json:"combination_count"`
	UnitPoints       points.Amount `json:"unit_points"`
	Multiplier       points.Amount `json:"multiplier"`
	BetPoints        points.Amount `json:"bet_points"`
}

// PrepareBet validates a definition and selection and returns their exact
// stake. The caller owns persistence and settlement; this function has no draw
// input and performs no payout calculation.
func PrepareBet(ctx context.Context, d Definition, selection Selection, multiplier points.Amount) (BetQuote, error) {
	if ctx == nil {
		return BetQuote{}, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return BetQuote{}, err
	}
	if err := ValidateDefinition(d); err != nil {
		return BetQuote{}, err
	}
	if multiplier <= 0 || multiplier > d.Limits.MaxMultiplier {
		return BetQuote{}, ErrLimit
	}
	normalized, err := NormalizeSelection(d, selection)
	if err != nil {
		return BetQuote{}, err
	}
	if err := ctx.Err(); err != nil {
		return BetQuote{}, err
	}
	expanded, err := Expand(d, normalized)
	if err != nil {
		return BetQuote{}, err
	}
	if err := ctx.Err(); err != nil {
		return BetQuote{}, err
	}

	nodes := 0
	for _, tier := range d.PrizeTiers {
		nodes += nodeCount(tier.Condition)
	}
	if nodes == 0 || len(expanded) > maxTraceNodes/nodes {
		return BetQuote{}, ErrLimit
	}

	stake := new(big.Int).Mul(big.NewInt(int64(d.UnitPoints)), big.NewInt(int64(len(expanded))))
	stake.Mul(stake, big.NewInt(int64(multiplier)))
	if !stake.IsInt64() {
		return BetQuote{}, points.ErrOverflow
	}
	betPoints := points.Amount(stake.Int64())
	if d.Limits.MaxBetPoints != nil && betPoints > *d.Limits.MaxBetPoints {
		return BetQuote{}, ErrLimit
	}
	return BetQuote{
		Normalized: normalized, Expanded: expanded, CombinationCount: len(expanded),
		UnitPoints: d.UnitPoints, Multiplier: multiplier, BetPoints: betPoints,
	}, nil
}
