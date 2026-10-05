package rules

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"math"
	"testing"
)

func ip(v int) *int                 { return &v }
func amount(v int64) *points.Amount { p := points.Amount(v); return &p }
func specialDefinition() Definition {
	return Definition{SchemaVersion: 1, Model: Model{Type: "X_PLUS_Y", RegularPool: Pool{Min: 1, Max: 49}, SpecialPool: Pool{Min: 1, Max: 49}, RegularCount: 6, SpecialCount: 1}, Selection: SelectionRule{Mode: "numbers", SpecialCount: 1}, UnitPoints: 1, PrizeTiers: []Tier{{Code: "SPECIAL_MATCH", Condition: Condition{Op: "equals", Field: "special_match", Value: ip(1)}, Odds: "35", Exclusive: true}}, Rounding: "half_up", RoundingScope: "order", Limits: Limits{MaxCombinations: 100, MaxMultiplier: 10}}
}
func specialInput() SimulationInput {
	return SimulationInput{Definition: specialDefinition(), Selection: Selection{Special: []int{7, 19, 31, 43}}, Draw: Draw{Regular: []int{1, 2, 3, 4, 5, 6}, Special: []int{7}}, Multiplier: 2}
}
func TestSpecialOnlyCombinationMultiplierAndExactTrace(t *testing.T) {
	in := specialInput()
	out, err := Simulate(in)
	if err != nil || !out.Won || out.CombinationCount != 4 || out.BetPoints != 8 || out.PrizePoints != 70 {
		t.Fatal(out, err)
	}
	if out.Lines[0].Points != "70" || !out.Lines[0].Hits[0].Selected || out.Lines[0].Hits[0].Trace.Actual != 1 {
		t.Fatal(out.Lines)
	}
	if in.Selection.Special[0] != 7 || len(in.Selection.Special) != 4 {
		t.Fatal("raw selection mutated")
	}
	in.Draw.Special = []int{8}
	out, err = Simulate(in)
	if err != nil || out.Won || out.PrizePoints != 0 {
		t.Fatal(out, err)
	}
}
func TestPrizeExclusivityUsesMoneyNotTierOrderOrPriority(t *testing.T) {
	in := specialInput()
	in.Selection.Special = []int{7}
	in.Multiplier = 1
	small := in.Definition.PrizeTiers[0]
	small.Odds = "2"
	large := small
	large.Code = "LARGE"
	large.Odds = "9"
	in.Definition.PrizeTiers = []Tier{small, large}
	out, err := Simulate(in)
	if err != nil || out.PrizePoints != 9 || out.Lines[0].Hits[0].Selected || !out.Lines[0].Hits[1].Selected {
		t.Fatal(out, err)
	}
	in.Definition.PrizeTiers[0].Exclusive = false
	in.Definition.PrizeTiers[1].Exclusive = false
	out, err = Simulate(in)
	if err != nil || out.PrizePoints != 11 {
		t.Fatal(out, err)
	}
	in.Definition.PrizeTiers[1].Exclusive = true
	if err = ValidateDefinition(in.Definition); !errors.Is(err, ErrInvalid) {
		t.Fatal("mixed tiers inferred an unstated policy", err)
	}
	in.Definition.MixedTierPolicy = "max_all"
	out, err = Simulate(in)
	if err != nil || out.PrizePoints != 9 {
		t.Fatal(out, err)
	}
	in.Definition.MixedTierPolicy = "max_exclusive_plus_additive"
	out, err = Simulate(in)
	if err != nil || out.PrizePoints != 11 {
		t.Fatal(out, err)
	}
	in.Definition.PrizeTiers[1].CapPoints = amount(1)
	in.Definition.MixedTierPolicy = "max_all"
	out, err = Simulate(in)
	if err != nil || out.PrizePoints != 2 || !out.Lines[0].Hits[0].Selected {
		t.Fatal("highest capped money not selected", out, err)
	}
}
func TestExplicitRoundingScopeAndGlobalCapBeforeIntegerOverflow(t *testing.T) {
	in := specialInput()
	in.Multiplier = 1
	in.Selection.Special = []int{7, 19}
	in.Definition.PrizeTiers[0].Condition = Condition{Op: "equals", Field: "draw_sum", Target: "all", Value: ip(28)}
	in.Definition.PrizeTiers[0].Odds = "1.5"
	for scope, want := range map[string]points.Amount{"order": 3, "line": 4, "tier": 4} {
		in.Definition.RoundingScope = scope
		out, err := Simulate(in)
		if err != nil || out.PrizePoints != want {
			t.Fatal(scope, out, err)
		}
	}
	in.Definition.RoundingScope = "order"
	in.Definition.PrizeTiers[0].Odds = "0.49"
	in.Selection.Special = []int{7}
	out, err := Simulate(in)
	if err != nil || !out.Won || out.PrizePoints != 0 {
		t.Fatal("matching condition is not erased by zero rounded payout", out, err)
	}
	in.Definition.UnitPoints = points.Amount(math.MaxInt64)
	in.Definition.PrizeTiers[0].Odds = "2"
	in.Definition.CapPoints = amount(100)
	out, err = Simulate(in)
	if err != nil || out.PrizePoints != 100 || out.BetPoints != points.Amount(math.MaxInt64) {
		t.Fatal(out, err)
	}
	in.Definition.CapPoints = nil
	_, err = Simulate(in)
	if !errors.Is(err, points.ErrOverflow) {
		t.Fatal("unbounded final payout must reject overflow", err)
	}
}
func TestDefinitionClosedSchemaAndExecutionBounds(t *testing.T) {
	d := specialDefinition()
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Definition
	if err = json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{`{"schema_version":1,"schema_version":2}`, `{"script":"return true"}`, `{"model":{"model":"X_PLUS_Y","unknown":true}}`, `{"number_attributes":{"color":{"red":[1],"red":[2]}}}`} {
		if err = json.Unmarshal([]byte(invalid), &decoded); !errors.Is(err, ErrInvalid) {
			t.Fatal(invalid, err)
		}
	}
	in := specialInput()
	in.Definition.PrizeTiers[0].Odds = "1e3"
	if _, err = Simulate(in); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	in = specialInput()
	in.Definition.Limits.MaxBetPoints = amount(7)
	if _, err = Simulate(in); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
	in = specialInput()
	in.Definition.Limits.MaxCombinations = 3
	if _, err = Simulate(in); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
	in = specialInput()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = SimulateContext(ctx, in); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	in = specialInput()
	in.Multiplier = 0
	if _, err = Simulate(in); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
}

func TestThreeModelsFeaturesExclusionAndMultiAttributeIntegration(t *testing.T) {
	in := specialInput()
	in.Multiplier = 1
	in.Definition.Model = Model{Type: "DIGITS_0_9", Length: 3, AllowRepeat: true, Ordered: true}
	in.Definition.Selection = SelectionRule{Mode: "numbers"}
	in.Definition.PrizeTiers[0].Condition = Condition{Op: "equals", Field: "position_match", Value: ip(3)}
	in.Selection = Selection{Digits: [][]int{{1}, {2}, {1}}}
	in.Draw = Draw{Digits: []int{1, 2, 1}}
	out, err := Simulate(in)
	if err != nil || !out.Won || out.PrizePoints != 35 {
		t.Fatal("digits121", out, err)
	}
	in.Selection = Selection{Digits: [][]int{{1}, {1}, {1}}}
	in.Draw = Draw{Digits: []int{1, 1, 1}}
	out, err = Simulate(in)
	if err != nil || !out.Won {
		t.Fatal("digits111", out, err)
	}
	in.Definition.Selection = SelectionRule{Mode: "features", FeatureChoices: map[string][]int{"same": {0, 1}, "odd": {0, 1, 2, 3}}}
	in.Definition.PrizeTiers[0].Condition = Condition{Op: "all", Children: []Condition{{Op: "selected", Field: "draw_all_same", Target: "digits", SelectionKey: "same"}, {Op: "selected", Field: "draw_odd_count", Target: "digits", SelectionKey: "odd"}}}
	in.Selection = Selection{Features: map[string][]int{"same": {1}, "odd": {3}}}
	out, err = Simulate(in)
	if err != nil || !out.Won {
		t.Fatal("selected repeat/odd features", out, err)
	}
	in = specialInput()
	in.Multiplier = 1
	in.Definition.Model.Type = "M_SELECT_N"
	in.Definition.Model.PoolSize = 49
	in.Definition.Model.TotalCount = 7
	in.Definition.Selection.RegularCount = 6
	in.Definition.PrizeTiers[0].Condition = Condition{Op: "all", Children: []Condition{{Op: "equals", Field: "regular_match", Value: ip(6)}, {Op: "equals", Field: "special_match", Value: ip(1)}}}
	in.Selection = Selection{Regular: []int{1, 2, 3, 4, 5, 6}, Special: []int{7}}
	out, err = Simulate(in)
	if err != nil || !out.Won || out.CombinationCount != 1 {
		t.Fatal("M選N", out, err)
	}
	in.Draw.Special = []int{6}
	if _, err = Simulate(in); !errors.Is(err, ErrInvalid) {
		t.Fatal("M cross-group duplicate draw accepted", err)
	}
	in = specialInput()
	in.Multiplier = 1
	in.Definition.Selection = SelectionRule{Mode: "exclude", ExcludeCount: 3}
	in.Definition.PrizeTiers[0].Condition = Condition{Op: "equals", Field: "excluded_match", Target: "all", Value: ip(0)}
	in.Selection = Selection{Exclude: []int{11, 22, 33}}
	out, err = Simulate(in)
	if err != nil || !out.Won || out.CombinationCount != 1 {
		t.Fatal("exclusion", out, err)
	}
	in = specialInput()
	in.Multiplier = 1
	in.Definition.NumberAttributes = map[string]map[string][]int{"color": {"red": {7, 19, 31, 43}, "blue": {2, 4, 6, 8}}, "zodiac": {"rat": {7, 19}}}
	in.Definition.Selection = SelectionRule{Mode: "attributes", AttributeGroups: []string{"color", "zodiac"}}
	in.Definition.PrizeTiers[0].Condition = Condition{Op: "all", Children: []Condition{{Op: "between", Field: "attribute_match", Target: "special", AttributeGroup: "color", AttributeValue: "$selection", Min: ip(1), Max: ip(1)}, {Op: "equals", Field: "attribute_match", Target: "special", AttributeGroup: "zodiac", AttributeValue: "$selection", Value: ip(1)}}}
	in.Selection = Selection{Attributes: map[string][]string{"color": {"red", "blue"}, "zodiac": {"rat"}}}
	out, err = Simulate(in)
	if err != nil || !out.Won || out.CombinationCount != 2 || out.PrizePoints != 35 {
		t.Fatal("multiple attributes per number and cartesian attribute tickets", out, err)
	}
}
