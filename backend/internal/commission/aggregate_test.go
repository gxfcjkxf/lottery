package commission

import (
	"errors"
	"math"
	"math/big"
	"reflect"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

func TestAggregateRoundsWholeSumOnce(t *testing.T) {
	tests := []struct {
		name  string
		items []ExactAmount
		exact ExactAmount
		whole points.Amount
	}{
		{"empty", nil, ExactAmount{"0", "1"}, 0},
		{"three tenths twice", []ExactAmount{{"3", "10"}, {"3", "10"}}, ExactAmount{"3", "5"}, 1},
		{"four tenths three times", []ExactAmount{{"2", "5"}, {"2", "5"}, {"2", "5"}}, ExactAmount{"6", "5"}, 1},
		{"thirds sum to one", []ExactAmount{{"1", "3"}, {"2", "3"}}, ExactAmount{"1", "1"}, 1},
		{"half rounds up", []ExactAmount{{"1", "2"}}, ExactAmount{"1", "2"}, 1},
		{"below half rounds down", []ExactAmount{{"49", "100"}}, ExactAmount{"49", "100"}, 0},
		{"zero", []ExactAmount{{"0", "1"}}, ExactAmount{"0", "1"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotExact, gotWhole, err := Aggregate(tt.items)
			if err != nil || gotExact != tt.exact || gotWhole != tt.whole {
				t.Fatalf("Aggregate(%v) = (%v, %v, %v), want (%v, %v, nil)", tt.items, gotExact, gotWhole, err, tt.exact, tt.whole)
			}
		})
	}
}

func TestAggregateIsPermutationInvariant(t *testing.T) {
	items := []ExactAmount{{"11", "25"}, {"33", "50"}, {"7", "12"}, {"1", "3"}}
	wantExact, wantWhole, err := Aggregate(items)
	if err != nil {
		t.Fatal(err)
	}
	for i := range items {
		permuted := append([]ExactAmount(nil), items...)
		permuted[i], permuted[len(permuted)-1-i] = permuted[len(permuted)-1-i], permuted[i]
		gotExact, gotWhole, err := Aggregate(permuted)
		if err != nil || gotExact != wantExact || gotWhole != wantWhole {
			t.Fatalf("permutation %v = (%v, %v, %v), want (%v, %v, nil)", permuted, gotExact, gotWhole, err, wantExact, wantWhole)
		}
	}
}

func TestAggregateCanonicalFractionsAndLargeIntegers(t *testing.T) {
	large := new(big.Int).Lsh(big.NewInt(1), 53)
	large.Add(large, big.NewInt(1))
	gotExact, gotWhole, err := Aggregate([]ExactAmount{{large.String(), "1"}, {"1", "1"}})
	want := new(big.Int).Add(large, big.NewInt(1)).String()
	if err != nil || gotExact != (ExactAmount{want, "1"}) || gotWhole != points.Amount(1<<53+2) {
		t.Fatalf("large exact sum = (%v, %v, %v)", gotExact, gotWhole, err)
	}

	denominator := new(big.Int).Exp(big.NewInt(10), big.NewInt(100), nil).String()
	gotExact, gotWhole, err = Aggregate([]ExactAmount{{"1", denominator}})
	if err != nil || gotExact != (ExactAmount{"1", denominator}) || gotWhole != 0 {
		t.Fatalf("large denominator result = (%v, %v, %v)", gotExact, gotWhole, err)
	}
}

func TestAggregateRejectsNoncanonicalFractions(t *testing.T) {
	for _, item := range []ExactAmount{
		{"", "1"}, {"00", "1"}, {"01", "1"}, {"+1", "1"}, {"-1", "1"},
		{"1.0", "1"}, {"1e2", "1"}, {"1", ""}, {"1", "0"}, {"1", "00"},
		{"1", "+1"}, {"1", "-1"}, {"1", "1.0"}, {"1", "1e2"},
		{"2", "4"}, {"0", "2"},
	} {
		if exact, amount, err := Aggregate([]ExactAmount{item}); !errors.Is(err, ErrInvalid) {
			t.Errorf("Aggregate(%v) = (%v, %v, %v), want ErrInvalid", item, exact, amount, err)
		}
	}
}

func TestAggregateInt64Boundaries(t *testing.T) {
	max := ExactAmount{big.NewInt(math.MaxInt64).String(), "1"}
	for _, tt := range []struct {
		name    string
		items   []ExactAmount
		want    points.Amount
		wantErr error
	}{
		{"max", []ExactAmount{max}, points.Amount(math.MaxInt64), nil},
		{"max plus less than half", []ExactAmount{max, {"49", "100"}}, points.Amount(math.MaxInt64), nil},
		{"max plus half", []ExactAmount{max, {"1", "2"}}, 0, points.ErrOverflow},
		{"rounded beyond max", []ExactAmount{max, {"51", "100"}}, 0, points.ErrOverflow},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, got, err := Aggregate(tt.items)
			if !errors.Is(err, tt.wantErr) || (err == nil && got != tt.want) {
				t.Fatalf("Aggregate(%v) amount/error = (%v, %v), want (%v, %v)", tt.items, got, err, tt.want, tt.wantErr)
			}
		})
	}
}

func TestAggregateOutputIsCanonical(t *testing.T) {
	got, _, err := Aggregate([]ExactAmount{{"1", "6"}, {"1", "3"}})
	if err != nil || !reflect.DeepEqual(got, ExactAmount{"1", "2"}) {
		t.Fatalf("Aggregate exact result = (%v, %v), want 1/2", got, err)
	}
}
