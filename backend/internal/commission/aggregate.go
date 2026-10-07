package commission

import (
	"math/big"

	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

// Aggregate sums exact commission amounts and rounds the cycle total once,
// using half-up rounding. Inputs must be reduced canonical fractions: unsigned
// decimal integers without leading zeroes, with a positive denominator. Zero
// is represented only as 0/1. The returned exact amount is also reduced.
func Aggregate(items []ExactAmount) (ExactAmount, points.Amount, error) {
	total := new(big.Rat)
	for _, item := range items {
		numerator, ok := canonicalInteger(item.Numerator)
		if !ok {
			return ExactAmount{}, 0, ErrInvalid
		}
		denominator, ok := canonicalInteger(item.Denominator)
		if !ok || denominator.Sign() == 0 {
			return ExactAmount{}, 0, ErrInvalid
		}
		if new(big.Int).GCD(nil, nil, numerator, denominator).Cmp(big.NewInt(1)) != 0 {
			return ExactAmount{}, 0, ErrInvalid
		}
		value := new(big.Rat).SetFrac(numerator, denominator)
		total.Add(total, value)
	}

	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(total.Num(), total.Denom(), remainder)
	if new(big.Int).Lsh(remainder, 1).Cmp(total.Denom()) >= 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	maxInt64 := big.NewInt(1<<63 - 1)
	if quotient.Cmp(maxInt64) > 0 {
		return ExactAmount{}, 0, points.ErrOverflow
	}

	return ExactAmount{
		Numerator:   total.Num().String(),
		Denominator: total.Denom().String(),
	}, points.Amount(quotient.Int64()), nil
}

// canonicalInteger parses the unsigned canonical integer syntax used by
// ExactAmount fractions. Denominators are validated the same way; callers
// separately require them to be nonzero.
func canonicalInteger(value string) (*big.Int, bool) {
	if value == "" || (len(value) > 1 && value[0] == '0') {
		return nil, false
	}
	for _, digit := range value {
		if digit < '0' || digit > '9' {
			return nil, false
		}
	}
	n, ok := new(big.Int).SetString(value, 10)
	if !ok {
		return nil, false
	}
	return n, true
}
