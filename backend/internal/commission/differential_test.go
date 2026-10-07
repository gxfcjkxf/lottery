package commission

import (
	"math"
	"math/big"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

func TestDifferentialSameBasisKeepsExactAmountsAndTelescopes(t *testing.T) {
	for _, tt := range []struct {
		base   points.Amount
		ratios []string
		want   []ExactAmount
	}{
		{100, []string{"0.1", "0.06"}, []ExactAmount{{"4", "1"}, {"6", "1"}}},
		{100, []string{"0.1", "0.06", "0.02"}, []ExactAmount{{"4", "1"}, {"4", "1"}, {"2", "1"}}},
		{11, []string{"0.1", "0.06"}, []ExactAmount{{"11", "25"}, {"33", "50"}}},
		{100, []string{"0.1", "0.1"}, []ExactAmount{{"0", "1"}, {"10", "1"}}},
		{0, []string{"0.1", "0"}, []ExactAmount{{"0", "1"}, {"0", "1"}}},
		{math.MaxInt64, []string{"1"}, []ExactAmount{{"9223372036854775807", "1"}}},
	} {
		got, err := DifferentialSameBasis(tt.base, tt.ratios)
		if err != nil || len(got) != len(tt.want) {
			t.Fatal(got, err)
		}
		sum := new(big.Rat)
		for i, v := range got {
			if v != tt.want[i] {
				t.Fatalf("base=%v ratios=%v got=%v want=%v", tt.base, tt.ratios, got, tt.want)
			}
			r, ok := new(big.Rat).SetString(v.Numerator + "/" + v.Denominator)
			if !ok {
				t.Fatal(v)
			}
			sum.Add(sum, r)
		}
		rate, _ := new(big.Rat).SetString(tt.ratios[0])
		if sum.Cmp(new(big.Rat).Mul(new(big.Rat).SetInt64(int64(tt.base)), rate)) != 0 {
			t.Fatal("layer amounts do not telescope", got)
		}
	}
}

func TestDifferentialSameBasisRejectsMalformedOrIncreasingChain(t *testing.T) {
	tooDeep := make([]string, 33)
	for i := range tooDeep {
		tooDeep[i] = "0.1"
	}
	for _, tt := range []struct {
		base   points.Amount
		ratios []string
	}{
		{-1, []string{"0.1"}}, {1, nil}, {1, tooDeep}, {1, []string{"0.06", "0.1"}},
		{1, []string{"0.10"}}, {1, []string{"1.01"}}, {1, []string{"-0.1"}}, {1, []string{"NaN"}},
	} {
		if _, err := DifferentialSameBasis(tt.base, tt.ratios); err != ErrInvalid {
			t.Fatal(tt, err)
		}
	}
}
