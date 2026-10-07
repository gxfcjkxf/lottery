package commission

import (
	"math/big"

	"github.com/gxfcjkxf/lottery/backend/internal/agency"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

// ExactAmount is an unrounded internal commission amount. Neither numerator
// nor denominator is constrained to a JavaScript number or an int64 product.
type ExactAmount struct {
	Numerator   string `json:"numerator"`
	Denominator string `json:"denominator"`
}

// DifferentialSameBasis applies an ancestor-to-descendant rate chain to one
// common finalized base. It is NOT a mixed-mode policy, a payout authorization,
// or a rounding/period aggregation decision. The eventual worker must use the
// bet's financial snapshot, not today's agent ratios, and handle other bases
// only after their policy is explicitly defined.
func DifferentialSameBasis(base points.Amount, ratios []string) ([]ExactAmount, error) {
	if base < 0 || len(ratios) < 1 || len(ratios) > 32 {
		return nil, ErrInvalid
	}
	rates := make([]int64, len(ratios))
	for i, s := range ratios {
		value, err := agency.RatioMicros(s)
		if err != nil || i > 0 && value > rates[i-1] {
			return nil, ErrInvalid
		}
		rates[i] = value
	}
	out := make([]ExactAmount, len(rates))
	for i, rate := range rates {
		if i+1 < len(rates) {
			rate -= rates[i+1]
		}
		numerator := new(big.Int).Mul(big.NewInt(int64(base)), big.NewInt(rate))
		value := new(big.Rat).SetFrac(numerator, big.NewInt(1000000))
		out[i] = ExactAmount{Numerator: value.Num().String(), Denominator: value.Denom().String()}
	}
	return out, nil
}
