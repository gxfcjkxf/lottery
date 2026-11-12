package withdrawal

import (
	"errors"
	"math"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

func TestTurnoverCreditExactCalculation(t *testing.T) {
	tests := []struct {
		name          string
		base          points.Amount
		contributions []TurnoverContribution
		numerator     string
		denominator   string
		meets         bool
	}{
		{
			name:          "multiple games meet threshold",
			base:          10000,
			contributions: []TurnoverContribution{{ValidPoints: 10000, Multiple: "2"}, {ValidPoints: 25000, Multiple: "5"}},
			numerator:     "100",
			denominator:   "1",
			meets:         true,
		},
		{
			name:          "below threshold",
			base:          10100,
			contributions: []TurnoverContribution{{ValidPoints: 10000, Multiple: "2"}, {ValidPoints: 25000, Multiple: "5"}},
			numerator:     "100",
			denominator:   "1",
		},
		{
			name:          "fractions combine exactly",
			base:          100,
			contributions: []TurnoverContribution{{ValidPoints: 100, Multiple: "3"}, {ValidPoints: 200, Multiple: "3"}},
			numerator:     "1",
			denominator:   "1",
			meets:         true,
		},
		{
			name:          "preserves values above float integer precision",
			base:          points.Amount(900719925474099300),
			contributions: []TurnoverContribution{{ValidPoints: points.Amount(900719925474099300), Multiple: "1"}},
			numerator:     "9007199254740993",
			denominator:   "1",
			meets:         true,
		},
		{
			name:          "summed maximum amounts do not overflow",
			base:          points.Amount(math.MaxInt64),
			contributions: []TurnoverContribution{{ValidPoints: points.Amount(math.MaxInt64), Multiple: "1"}, {ValidPoints: points.Amount(math.MaxInt64), Multiple: "1"}},
			numerator:     "9223372036854775807",
			denominator:   "50",
			meets:         true,
		},
		{
			name:          "fractional multiple",
			base:          1000,
			contributions: []TurnoverContribution{{ValidPoints: 2500, Multiple: "2.5"}},
			numerator:     "10",
			denominator:   "1",
			meets:         true,
		},
		{
			name:          "noninteger credit stays an exact fraction",
			base:          100,
			contributions: []TurnoverContribution{{ValidPoints: 100, Multiple: "3"}},
			numerator:     "1",
			denominator:   "3",
		},
		{
			name:          "fraction just below threshold is not rounded up",
			base:          100,
			contributions: []TurnoverContribution{{ValidPoints: 99999900, Multiple: "1000000"}},
			numerator:     "999999",
			denominator:   "1000000",
		},
		{
			name:          "large deficit is not hidden by float rounding",
			base:          points.Amount(900719925474099300),
			contributions: []TurnoverContribution{{ValidPoints: points.Amount(900719925474099200), Multiple: "1"}},
			numerator:     "9007199254740992",
			denominator:   "1",
		},
		{
			name:          "separate bet time snapshot multiples",
			base:          7500,
			contributions: []TurnoverContribution{{ValidPoints: 10000, Multiple: "2"}, {ValidPoints: 10000, Multiple: "4"}},
			numerator:     "75",
			denominator:   "1",
			meets:         true,
		},
		{
			name:          "minimum multiple precision",
			base:          100,
			contributions: []TurnoverContribution{{ValidPoints: 100, Multiple: "0.000001"}},
			numerator:     "1000000",
			denominator:   "1",
			meets:         true,
		},
		{
			name:        "empty zero base is mathematically met",
			base:        0,
			numerator:   "0",
			denominator: "1",
			meets:       true,
		},
		{
			name:        "positive base with no contributions",
			base:        100,
			numerator:   "0",
			denominator: "1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := calculateTurnoverCredit(tc.base, tc.contributions)
			if err != nil {
				t.Fatalf("calculateTurnoverCredit() error = %v", err)
			}
			if got.Numerator != tc.numerator || got.Denominator != tc.denominator || got.MeetsThreshold != tc.meets {
				t.Fatalf("calculateTurnoverCredit() = %+v, want numerator=%s denominator=%s meets=%t", got, tc.numerator, tc.denominator, tc.meets)
			}
		})
	}
}

func TestTurnoverCreditOrderInvariantAndExactBoundary(t *testing.T) {
	contributions := []TurnoverContribution{{ValidPoints: 100, Multiple: "3"}, {ValidPoints: 200, Multiple: "3"}, {ValidPoints: 400, Multiple: "2"}}
	forward, err := calculateTurnoverCredit(300, contributions)
	if err != nil {
		t.Fatal(err)
	}
	reverse, err := calculateTurnoverCredit(300, []TurnoverContribution{contributions[2], contributions[1], contributions[0]})
	if err != nil {
		t.Fatal(err)
	}
	if forward != reverse {
		t.Fatalf("credit depends on contribution order: forward=%+v reverse=%+v", forward, reverse)
	}
	if forward.Numerator != "3" || forward.Denominator != "1" || !forward.MeetsThreshold {
		t.Fatalf("exact threshold boundary not met: %+v", forward)
	}
}

func TestTurnoverCreditRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name          string
		base          points.Amount
		contributions []TurnoverContribution
		wantErr       error
	}{
		{name: "negative base", base: -100, wantErr: ErrTurnoverCreditInvalid},
		{name: "negative valid points", base: 0, contributions: []TurnoverContribution{{ValidPoints: -100, Multiple: "1"}}, wantErr: ErrTurnoverCreditInvalid},
		{name: "noncanonical leading zero", base: 0, contributions: []TurnoverContribution{{Multiple: "01"}}, wantErr: ErrTurnoverCreditInvalid},
		{name: "exponent notation", base: 0, contributions: []TurnoverContribution{{Multiple: "1e2"}}, wantErr: ErrTurnoverCreditInvalid},
		{name: "out of range", base: 0, contributions: []TurnoverContribution{{Multiple: "1000000.000001"}}, wantErr: ErrTurnoverCreditInvalid},
		{name: "explicit zero", base: 0, contributions: []TurnoverContribution{{Multiple: "0"}}, wantErr: ErrTurnoverCreditInvalid},
		{name: "late zero after met prefix", base: 100, contributions: []TurnoverContribution{{ValidPoints: 100, Multiple: "1"}, {Multiple: "0"}}, wantErr: ErrTurnoverCreditInvalid},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := calculateTurnoverCredit(tc.base, tc.contributions)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("calculateTurnoverCredit() error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}
