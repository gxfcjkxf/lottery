package rules

import (
	"context"
	"errors"
	"math"
	"reflect"
	"strconv"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

func betDefinition(model Model, selectionRule SelectionRule) Definition {
	zero := 0
	condition := Condition{Op: "equals", Field: "draw_even_count", Target: "all", Value: &zero}
	if model.Type == "DIGITS_0_9" {
		condition = Condition{Op: "equals", Field: "position_match", Value: &zero}
	}
	return Definition{
		SchemaVersion: 1, Model: model, Selection: selectionRule, UnitPoints: 2,
		PrizeTiers: []Tier{{Code: "ANY", Condition: condition, Odds: "1", Exclusive: true}},
		Rounding:   "half_up", RoundingScope: "order",
		Limits: Limits{MaxCombinations: MaxCombinations, MaxMultiplier: 10},
	}
}

func TestPrepareBetQuotesAllModelTypesAndDoesNotMutateSelection(t *testing.T) {
	digits := betDefinition(Model{Type: "DIGITS_0_9", Length: 2, Ordered: true, AllowRepeat: true}, SelectionRule{Mode: "numbers"})
	input := Selection{Digits: [][]int{{2, 1, 1}, {4, 3}}}
	before := cloneSelection(input)
	quote, err := PrepareBet(context.Background(), digits, input, 3)
	if err != nil || quote.CombinationCount != 4 || quote.BetPoints != 24 || !reflect.DeepEqual(quote.Normalized.Digits, [][]int{{1, 2}, {3, 4}}) {
		t.Fatalf("digits quote=%+v err=%v", quote, err)
	}
	if !reflect.DeepEqual(input, before) {
		t.Fatalf("input mutated: got=%+v want=%+v", input, before)
	}

	xPlusY := betDefinition(Model{Type: "X_PLUS_Y", RegularPool: Pool{Min: 1, Max: 4}, SpecialPool: Pool{Min: 7, Max: 8}, RegularCount: 2, SpecialCount: 1}, SelectionRule{Mode: "numbers", RegularCount: 1, SpecialCount: 1})
	quote, err = PrepareBet(context.Background(), xPlusY, Selection{Regular: []int{1, 2}, Special: []int{7, 8}}, 1)
	if err != nil || quote.CombinationCount != 4 || quote.BetPoints != 8 {
		t.Fatalf("X_PLUS_Y quote=%+v err=%v", quote, err)
	}

	values := []int{1, 2, 3, 4}
	mSelect := betDefinition(Model{Type: "M_SELECT_N", PoolSize: 4, TotalCount: 2, RegularCount: 1, SpecialCount: 1, RegularPool: Pool{Values: values}, SpecialPool: Pool{Values: []int{4, 3, 2, 1}}}, SelectionRule{Mode: "numbers", RegularCount: 1, SpecialCount: 1})
	quote, err = PrepareBet(context.Background(), mSelect, Selection{Regular: []int{1, 2}, Special: []int{2, 3}}, 1)
	if err != nil || quote.CombinationCount != 3 || quote.BetPoints != 6 {
		t.Fatalf("M_SELECT_N feasible combinations quote=%+v err=%v", quote, err)
	}
}

func TestPrepareBetQuotesExcludeAttributesFeaturesAndPoolRepeat(t *testing.T) {
	base := betDefinition(Model{Type: "X_PLUS_Y", RegularPool: Pool{Min: 0, Max: 4, AllowRepeat: true}, RegularCount: 2}, SelectionRule{Mode: "exclude", ExcludeCount: 2})
	quote, err := PrepareBet(context.Background(), base, Selection{Exclude: []int{0, 1}}, 1)
	if err != nil || quote.CombinationCount != 1 {
		t.Fatalf("exclude quote=%+v err=%v", quote, err)
	}

	attrs := betDefinition(Model{Type: "X_PLUS_Y", RegularPool: Pool{Min: 1, Max: 4}, RegularCount: 1}, SelectionRule{Mode: "attributes", AttributeGroups: []string{"color"}})
	attrs.NumberAttributes = map[string]map[string][]int{"color": {"red": {1}, "blue": {2, 3}}}
	quote, err = PrepareBet(context.Background(), attrs, Selection{Attributes: map[string][]string{"color": {"blue", "red", "blue"}}}, 2)
	if err != nil || quote.CombinationCount != 2 || quote.BetPoints != 8 {
		t.Fatalf("attributes quote=%+v err=%v", quote, err)
	}

	features := betDefinition(Model{Type: "X_PLUS_Y", RegularPool: Pool{Min: 1, Max: 4}, RegularCount: 1}, SelectionRule{Mode: "features", FeatureChoices: map[string][]int{"parity": {0, 1}, "band": {1, 2}}})
	quote, err = PrepareBet(context.Background(), features, Selection{Features: map[string][]int{"parity": {0, 1}, "band": {1, 2}}}, 1)
	if err != nil || quote.CombinationCount != 4 || quote.BetPoints != 8 {
		t.Fatalf("features quote=%+v err=%v", quote, err)
	}

	repeat := betDefinition(Model{Type: "X_PLUS_Y", RegularPool: Pool{Values: []int{7}, AllowRepeat: true}, RegularCount: 2}, SelectionRule{Mode: "numbers", RegularCount: 2})
	quote, err = PrepareBet(context.Background(), repeat, Selection{Regular: []int{7}}, 1)
	if err != nil || quote.CombinationCount != 1 || !reflect.DeepEqual(quote.Expanded[0].Regular, []int{7, 7}) {
		t.Fatalf("repeat-pool quote=%+v err=%v", quote, err)
	}
}

func TestPrepareBetDoublesCompoundTraceAndMatchFeasibility(t *testing.T) {
	d := betDefinition(Model{Type: "DIGITS_0_9", Length: 2, Ordered: true, AllowRepeat: true}, SelectionRule{Mode: "numbers"})
	quote, err := PrepareBet(context.Background(), d, Selection{Digits: [][]int{{5}, {5}}}, 2)
	if err != nil || quote.CombinationCount != 1 || !reflect.DeepEqual(quote.Expanded[0].Digits, [][]int{{5}, {5}}) {
		t.Fatalf("double quote=%+v err=%v", quote, err)
	}

	zero := 0
	d.PrizeTiers[0].Condition = Condition{Op: "all", Children: []Condition{
		{Op: "equals", Field: "position_match", Value: &zero},
		{Op: "any", Children: []Condition{
			{Op: "equals", Field: "position_match", Value: &zero},
			{Op: "equals", Field: "position_match", Value: &zero},
		}},
	}}
	quote, err = PrepareBet(context.Background(), d, Selection{Digits: [][]int{{5}, {5}}}, 1)
	if err != nil || quote.BetPoints != 2 {
		t.Fatalf("compound-condition quote=%+v err=%v", quote, err)
	}

	d.Model.AllowRepeat = false
	if _, err := PrepareBet(context.Background(), d, Selection{Digits: [][]int{{5}, {5}}}, 1); !errors.Is(err, ErrInvalid) {
		t.Fatalf("infeasible no-repeat selection error=%v", err)
	}
}

func TestPrepareBetEnforcesDefinitionMultiplierCombinationAndBetLimits(t *testing.T) {
	d := betDefinition(Model{Type: "DIGITS_0_9", Length: 1, Ordered: true, AllowRepeat: true}, SelectionRule{Mode: "numbers"})
	if _, err := PrepareBet(context.Background(), d, Selection{Digits: [][]int{{1}}}, 0); !errors.Is(err, ErrLimit) {
		t.Fatalf("zero multiplier error=%v", err)
	}
	if _, err := PrepareBet(context.Background(), d, Selection{Digits: [][]int{{1}}}, 11); !errors.Is(err, ErrLimit) {
		t.Fatalf("over multiplier error=%v", err)
	}
	d.Limits.MaxCombinations = 1
	if _, err := PrepareBet(context.Background(), d, Selection{Digits: [][]int{{1, 2}}}, 1); !errors.Is(err, ErrLimit) {
		t.Fatalf("combination limit error=%v", err)
	}
	d.Limits.MaxCombinations = 2
	maxBet := points.Amount(3)
	d.Limits.MaxBetPoints = &maxBet
	if _, err := PrepareBet(context.Background(), d, Selection{Digits: [][]int{{1, 2}}}, 1); !errors.Is(err, ErrLimit) {
		t.Fatalf("max bet limit error=%v", err)
	}
	d.Limits.MaxBetPoints = nil
	d.UnitPoints = points.Amount(math.MaxInt64)
	if _, err := PrepareBet(context.Background(), d, Selection{Digits: [][]int{{1, 2}}}, 2); !errors.Is(err, points.ErrOverflow) {
		t.Fatalf("overflow error=%v", err)
	}
}

func TestPrepareBetEnforcesAggregateTraceBudgetAndNeverNeedsDraw(t *testing.T) {
	zero := 0
	d := betDefinition(Model{Type: "DIGITS_0_9", Length: 4, Ordered: true, AllowRepeat: true}, SelectionRule{Mode: "numbers"})
	d.Limits.MaxCombinations = 10_000
	d.PrizeTiers = make([]Tier, 32)
	for i := range d.PrizeTiers {
		d.PrizeTiers[i] = Tier{Code: "T" + strconv.Itoa(i), Condition: Condition{Op: "equals", Field: "position_match", Value: &zero}, Odds: "1", Exclusive: true}
	}
	selection := Selection{Digits: [][]int{{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, {0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, {0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, {0, 1, 2, 3, 4, 5, 6, 7, 8, 9}}}
	if _, err := PrepareBet(context.Background(), d, selection, 1); !errors.Is(err, ErrLimit) {
		t.Fatalf("trace budget error=%v", err)
	}
	// A small quote succeeds with no draw supplied and no simulation metadata.
	d.PrizeTiers = d.PrizeTiers[:1]
	quote, err := PrepareBet(context.Background(), d, Selection{Digits: [][]int{{5}, {1}, {2}, {3}}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if quote.BetPoints != 2 || quote.CombinationCount != 1 {
		t.Fatalf("unexpected stake fields: %+v", quote)
	}
}

func TestPrepareBetContextChecks(t *testing.T) {
	d := betDefinition(Model{Type: "DIGITS_0_9", Length: 1, Ordered: true, AllowRepeat: true}, SelectionRule{Mode: "numbers"})
	if _, err := PrepareBet(nil, d, Selection{Digits: [][]int{{1}}}, 1); !errors.Is(err, ErrInvalid) {
		t.Fatalf("nil context error=%v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := PrepareBet(ctx, d, Selection{Digits: [][]int{{1}}}, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context error=%v", err)
	}
}
