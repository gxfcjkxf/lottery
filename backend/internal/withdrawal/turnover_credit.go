package withdrawal

import (
	"errors"
	"math/big"

	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

var (
	ErrTurnoverCreditInvalid = errors.New("invalid turnover credit input")
)

// TurnoverContribution is one valid stake, or an int64-bounded subtotal of
// stakes whose bet-time snapshots share the same effective multiple. Callers
// must not overflow an Amount while building subtotals; the sum here is exact.
type TurnoverContribution struct {
	ValidPoints points.Amount
	Multiple    string
}

// TurnoverCreditResult contains the reduced exact credit fraction. The strings
// keep arbitrary precision values immutable and safe from caller mutation.
type TurnoverCreditResult struct {
	Numerator      string
	Denominator    string
	MeetsThreshold bool
}

// calculateTurnoverCredit computes sum(valid points / effective multiple) and
// compares it with base. It performs arithmetic only; the result does not
// authorize a withdrawal, reserve wallet points, or enable an eligibility check.
func calculateTurnoverCredit(base points.Amount, contributions []TurnoverContribution) (TurnoverCreditResult, error) {
	if base < 0 {
		return TurnoverCreditResult{}, ErrTurnoverCreditInvalid
	}

	// Validate and parse every input before calculating, so a met prefix cannot
	// hide a later invalid contribution.
	multiples := make([]*big.Rat, len(contributions))
	for i, contribution := range contributions {
		if contribution.ValidPoints < 0 || !ValidMultiple(contribution.Multiple) {
			return TurnoverCreditResult{}, ErrTurnoverCreditInvalid
		}
		multiple, ok := new(big.Rat).SetString(contribution.Multiple)
		if !ok || multiple.Sign() <= 0 {
			return TurnoverCreditResult{}, ErrTurnoverCreditInvalid
		}
		multiples[i] = multiple
	}

	credit := new(big.Rat)
	for i, contribution := range contributions {
		validPoints := new(big.Rat).SetInt64(int64(contribution.ValidPoints))
		credit.Add(credit, new(big.Rat).Quo(validPoints, multiples[i]))
	}
	threshold := new(big.Rat).SetInt64(int64(base))
	return TurnoverCreditResult{
		Numerator:      credit.Num().String(),
		Denominator:    credit.Denom().String(),
		MeetsThreshold: credit.Cmp(threshold) >= 0,
	}, nil
}
