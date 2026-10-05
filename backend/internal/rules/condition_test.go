package rules

import (
	"errors"
	"testing"
)

func numberDefinition(regularCount, specialCount int) Definition {
	return Definition{
		Model: Model{
			Type: "X_PLUS_Y", RegularPool: Pool{Min: 1, Max: 49}, SpecialPool: Pool{Min: 1, Max: 10},
			RegularCount: regularCount, SpecialCount: specialCount,
		},
		Selection: SelectionRule{Mode: "numbers", RegularCount: regularCount, SpecialCount: specialCount},
	}
}

func equals(field, target string, value int) Condition {
	return Condition{Op: "equals", Field: field, Target: target, Value: &value}
}

func TestEvaluateNestedAllAnyNotKeepsEveryTraceChild(t *testing.T) {
	definition := numberDefinition(6, 1)
	selection := Selection{Regular: []int{1, 2, 5, 6, 8, 9}, Special: []int{7}}
	draw := Draw{Regular: []int{1, 2, 30, 40, 5, 6}, Special: []int{9}}
	condition := Condition{Op: "all", Children: []Condition{
		equals("regular_match", "", 4),
		{Op: "any", Children: []Condition{
			{Op: "not", Children: []Condition{equals("special_match", "", 1)}},
			equals("draw_sum", "all", 100),
		}},
	}}
	trace, err := Evaluate(definition, condition, selection, draw)
	if err != nil {
		t.Fatal(err)
	}
	if !trace.Matched || len(trace.Children) != 2 || !trace.Children[0].Matched || !trace.Children[1].Matched {
		t.Fatalf("unexpected root trace: %+v", trace)
	}
	any := trace.Children[1]
	if any.Op != "any" || len(any.Children) != 2 || !any.Children[0].Matched || any.Children[1].Matched {
		t.Fatalf("any did not preserve both child explanations: %+v", any)
	}
	if len(any.Children[0].Children) != 1 || any.Children[0].Children[0].Actual != 0 || any.Children[0].Children[0].Matched {
		t.Fatalf("not-child trace missing: %+v", any.Children[0])
	}
}

func TestEvaluateOddSumUniqueAndConsecutiveWithRepeatedDraws(t *testing.T) {
	definition := numberDefinition(4, 0)
	definition.Model.RegularPool.AllowRepeat = true
	selection := Selection{Regular: []int{2, 2, 3, 7}}
	draw := Draw{Regular: []int{2, 2, 3, 7}}
	cases := []struct {
		condition Condition
		actual    int
	}{
		{equals("draw_odd_count", "regular", 2), 2},
		{equals("draw_even_count", "regular", 2), 2},
		{equals("draw_sum", "regular", 14), 14},
		{equals("draw_unique_count", "regular", 3), 3},
		{equals("draw_consecutive", "regular", 1), 1},
		{equals("draw_span", "regular", 5), 5},
		{equals("draw_all_same", "regular", 0), 0},
		{equals("draw_first_last_same", "regular", 0), 0},
	}
	for _, tc := range cases {
		trace, err := Evaluate(definition, tc.condition, selection, draw)
		if err != nil || !trace.Matched || trace.Actual != tc.actual {
			t.Errorf("field %s trace=%+v err=%v", tc.condition.Field, trace, err)
		}
	}
	consecutive := Draw{Regular: []int{1, 2, 3, 5}}
	trace, err := Evaluate(definition, equals("draw_consecutive", "regular", 2), selection, consecutive)
	if err != nil || !trace.Matched {
		t.Fatalf("three-number run should have two adjacent sequential links: trace=%+v err=%v", trace, err)
	}
}

func TestRegularAndSpecialMatchUseBoundedMultisetIntersection(t *testing.T) {
	definition := numberDefinition(3, 1)
	definition.Model.RegularPool.AllowRepeat = true
	definition.Selection.RegularCount = 2
	definition.Selection.SpecialCount = 7
	selection := Selection{Regular: []int{4, 4}, Special: []int{1, 2, 3, 4, 5, 6, 7}}
	draw := Draw{Regular: []int{4, 4, 4}, Special: []int{7}}
	regularTrace, err := Evaluate(definition, equals("regular_match", "", 2), selection, draw)
	if err != nil || !regularTrace.Matched || regularTrace.Actual != 2 {
		t.Fatalf("regular multiset intersection=%+v err=%v", regularTrace, err)
	}
	specialTrace, err := Evaluate(definition, equals("special_match", "", 1), selection, draw)
	if err != nil || !specialTrace.Matched || specialTrace.Actual != 1 {
		t.Fatalf("special seven-candidate match=%+v err=%v", specialTrace, err)
	}
}

func TestAttributeMatchLiteralSelectionAndMultiAttributeNumbersCountOnce(t *testing.T) {
	definition := numberDefinition(3, 1)
	definition.Selection.Mode = "attributes"
	definition.Selection.RegularCount = 0
	definition.Selection.SpecialCount = 0
	definition.Selection.AttributeGroups = []string{"color"}
	definition.NumberAttributes = map[string]map[string][]int{
		"color": {"red": {1, 2}, "blue": {2, 3}, "odd": {1, 3}},
	}
	selection := Selection{
		Attributes: map[string][]string{"color": {"blue"}},
	}
	draw := Draw{Regular: []int{1, 2, 3}, Special: []int{2}}
	literalCondition := Condition{Op: "equals", Field: "attribute_match", Target: "all", AttributeGroup: "color", AttributeValue: "red", Value: intPointer(2)}
	literal, err := Evaluate(definition, literalCondition, selection, draw)
	if err != nil || !literal.Matched || literal.Actual != 2 {
		t.Fatalf("literal red attribute should count matching draw numbers 1 and 2 once: trace=%+v err=%v", literal, err)
	}
	selected := Condition{Op: "equals", Field: "attribute_match", Target: "regular", AttributeGroup: "color", AttributeValue: "$selection", Value: intPointer(2)}
	trace, err := Evaluate(definition, selected, selection, draw)
	if err != nil || !trace.Matched || trace.Actual != 2 {
		t.Fatalf("selected blue attribute should match 2 and 3: trace=%+v err=%v", trace, err)
	}
	selection.Attributes["color"] = []string{"blue", "red"}
	if _, err = Evaluate(definition, selected, selection, draw); !errors.Is(err, ErrInvalid) {
		t.Fatalf("multiple selected attribute values error=%v", err)
	}
}

func TestSelectedFeatureChoiceSupportsBooleanZeroAndOne(t *testing.T) {
	definition := numberDefinition(3, 0)
	definition.Selection.Mode = "features"
	definition.Selection.RegularCount = 0
	definition.Selection.FeatureChoices = map[string][]int{"parity": {0, 1}}
	selection := Selection{Features: map[string][]int{"parity": {1}}}
	draw := Draw{Regular: []int{1, 2, 4}}
	condition := Condition{Op: "selected", Field: "draw_odd_count", Target: "regular", SelectionKey: "parity"}
	trace, err := Evaluate(definition, condition, selection, draw)
	if err != nil || !trace.Matched || trace.Actual != 1 {
		t.Fatalf("boolean feature one not evaluated: trace=%+v err=%v", trace, err)
	}
	selection.Features["parity"] = []int{0}
	draw.Regular = []int{2, 4, 6}
	trace, err = Evaluate(definition, condition, selection, draw)
	if err != nil || !trace.Matched || trace.Actual != 0 {
		t.Fatalf("boolean feature zero not evaluated: trace=%+v err=%v", trace, err)
	}
	delete(selection.Features, "parity")
	if _, err = Evaluate(definition, condition, selection, draw); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing selected feature error=%v", err)
	}
}

func TestDigitPositionsAndDrawParity(t *testing.T) {
	definition := Definition{Model: Model{Type: "DIGITS_0_9", Length: 3, Ordered: true}, Selection: SelectionRule{Mode: "numbers"}}
	selection := Selection{Digits: [][]int{{1, 4}, {2}, {3, 9}}}
	draw := Draw{Digits: []int{4, 2, 9}}
	positionMatches, err := Evaluate(definition, equals("position_match", "", 3), selection, draw)
	if err != nil || !positionMatches.Matched || positionMatches.Actual != 3 {
		t.Fatalf("position matches=%+v err=%v", positionMatches, err)
	}
	parity, err := Evaluate(definition, Condition{Op: "equals", Field: "draw_parity", Target: "digits", Position: intPointer(1), Value: intPointer(0)}, selection, draw)
	if err != nil || !parity.Matched || parity.Actual != 0 {
		t.Fatalf("even digit parity=%+v err=%v", parity, err)
	}
	digit, err := Evaluate(definition, Condition{Op: "equals", Field: "draw_digit", Target: "digits", Position: intPointer(2), Value: intPointer(9)}, selection, draw)
	if err != nil || !digit.Matched || digit.Actual != 9 {
		t.Fatalf("position digit=%+v err=%v", digit, err)
	}
}

func TestConditionRejectsUnknownUnusedAndUnreachableFields(t *testing.T) {
	definition := numberDefinition(3, 1)
	valid := equals("draw_sum", "all", 10)
	bad := []struct {
		name      string
		condition Condition
	}{
		{name: "unknown op", condition: Condition{Op: "script", Field: "draw_sum", Target: "all", Value: intPointer(10)}},
		{name: "unknown field", condition: equals("eval", "all", 10)},
		{name: "unknown target", condition: equals("draw_sum", "__proto__", 10)},
		{name: "logic leaf parameter", condition: Condition{Op: "all", Field: "draw_sum", Children: []Condition{valid}}},
		{name: "leaf children", condition: Condition{Op: "equals", Field: "draw_sum", Target: "all", Value: intPointer(10), Children: []Condition{valid}}},
		{name: "unused position", condition: Condition{Op: "equals", Field: "draw_sum", Target: "all", Position: intPointer(0), Value: intPointer(10)}},
		{name: "unused attribute group", condition: Condition{Op: "equals", Field: "draw_sum", Target: "all", AttributeGroup: "x", Value: intPointer(10)}},
		{name: "unused selection key", condition: Condition{Op: "equals", Field: "draw_sum", Target: "all", SelectionKey: "feature", Value: intPointer(10)}},
		{name: "missing logic children", condition: Condition{Op: "any"}},
		{name: "too many not children", condition: Condition{Op: "not", Children: []Condition{valid, valid}}},
		{name: "duplicate in values", condition: Condition{Op: "in", Field: "draw_sum", Target: "all", Values: []int{10, 10}}},
		{name: "between reversed", condition: Condition{Op: "between", Field: "draw_sum", Target: "all", Min: intPointer(20), Max: intPointer(10)}},
		{name: "unreachable odd count", condition: equals("draw_odd_count", "all", 5)},
		{name: "invalid parity position", condition: Condition{Op: "equals", Field: "draw_parity", Target: "digits", Position: intPointer(0), Value: intPointer(1)}},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateCondition(definition, tc.condition); !errors.Is(err, ErrInvalid) {
				t.Fatalf("ValidateCondition error=%v, want ErrInvalid", err)
			}
		})
	}
}

func TestConditionDepthAndNodeBudgets(t *testing.T) {
	definition := numberDefinition(1, 0)
	leaf := equals("regular_match", "", 0)
	depthEight := leaf
	for i := 0; i < 7; i++ {
		depthEight = Condition{Op: "not", Children: []Condition{depthEight}}
	}
	if err := ValidateCondition(definition, depthEight); err != nil {
		t.Fatalf("depth 8 rejected: %v", err)
	}
	depthNine := Condition{Op: "not", Children: []Condition{depthEight}}
	if err := ValidateCondition(definition, depthNine); !errors.Is(err, ErrLimit) {
		t.Fatalf("depth 9 error=%v, want ErrLimit", err)
	}
	children := make([]Condition, 32)
	for i := range children {
		grandchildren := []Condition{leaf, leaf, leaf}
		children[i] = Condition{Op: "all", Children: grandchildren}
	}
	if err := ValidateCondition(definition, Condition{Op: "all", Children: children}); !errors.Is(err, ErrLimit) {
		t.Fatalf("129-node condition error=%v, want ErrLimit", err)
	}
}

func TestEvaluateMultisetExcludedAndInputBounds(t *testing.T) {
	definition := numberDefinition(3, 0)
	definition.Model.RegularPool.AllowRepeat = true
	definition.Selection.Mode = "exclude"
	definition.Selection.RegularCount = 0
	definition.Selection.ExcludeCount = 1
	selection := Selection{Exclude: []int{5}}
	draw := Draw{Regular: []int{5, 5, 6}}
	trace, err := Evaluate(definition, equals("excluded_match", "regular", 2), selection, draw)
	if err != nil || !trace.Matched || trace.Actual != 2 {
		t.Fatalf("excluded duplicate draw hit count=%+v err=%v", trace, err)
	}
	invalid := draw
	invalid.Regular = []int{1_000_001, 1, 2}
	if _, err := Evaluate(definition, equals("draw_sum", "regular", 3), selection, invalid); !errors.Is(err, ErrInvalid) {
		t.Fatalf("out-of-range draw error=%v", err)
	}
	tooLarge := Definition{Model: Model{RegularPool: Pool{Min: 0, Max: 1_000_000}, RegularCount: 11}}
	if err := ValidateCondition(tooLarge, equals("draw_sum", "regular", 1)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid oversized model error=%v, want ErrInvalid", err)
	}
}

func TestCanonicalModelTypesSelectedWhitelistAndAttributeLiteral(t *testing.T) {
	definition := numberDefinition(1, 0)
	definition.Selection.Mode = "attributes"
	definition.Selection.RegularCount = 0
	definition.NumberAttributes = map[string]map[string][]int{"color": {"red": {1}}}
	if err := ValidateCondition(definition, Condition{Op: "equals", Field: "attribute_match", Target: "regular", AttributeGroup: "color", AttributeValue: "red", Value: intPointer(1)}); err != nil {
		t.Fatalf("literal attribute should not require selection-configured group: %v", err)
	}
	badSelected := Condition{Op: "selected", Field: "unknown", SelectionKey: "feature"}
	definition.Selection.Mode = "features"
	definition.Selection.FeatureChoices = map[string][]int{"feature": {0, 1}}
	if err := ValidateCondition(definition, badSelected); !errors.Is(err, ErrInvalid) {
		t.Fatalf("selected with an unknown field error=%v", err)
	}
	badSelected.Field = "draw_sum"
	badSelected.Target = "not-a-target"
	if err := ValidateCondition(definition, badSelected); !errors.Is(err, ErrInvalid) {
		t.Fatalf("selected with an invalid target error=%v", err)
	}
	badModel := numberDefinition(1, 0)
	badModel.Model.Type = "numbers"
	if err := ValidateCondition(badModel, equals("draw_sum", "all", 1)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("noncanonical model enum error=%v", err)
	}
}

func TestDrawSumSupportsTwentyMillionAndRejectsInvalidRuntimeShape(t *testing.T) {
	definition := Definition{
		Model:     Model{Type: "X_PLUS_Y", RegularPool: Pool{Min: 990_001, Max: 1_000_000, AllowRepeat: true}, SpecialPool: Pool{Min: 990_001, Max: 1_000_000, AllowRepeat: true}, RegularCount: 10, SpecialCount: 10},
		Selection: SelectionRule{Mode: "numbers", RegularCount: 10, SpecialCount: 10},
	}
	condition := equals("draw_sum", "all", 20_000_000)
	if err := ValidateCondition(definition, condition); err != nil {
		t.Fatalf("maximum legal sum condition rejected: %v", err)
	}
	values := make([]int, 10)
	for i := range values {
		values[i] = 1_000_000
	}
	selection := Selection{Regular: values, Special: values}
	draw := Draw{Regular: values, Special: values}
	trace, err := Evaluate(definition, condition, selection, draw)
	if err != nil || !trace.Matched || trace.Actual != 20_000_000 {
		t.Fatalf("maximum draw sum=%+v err=%v", trace, err)
	}
	draw.Special = nil
	if _, err = Evaluate(definition, condition, selection, draw); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad draw length error=%v", err)
	}
}

func intPointer(value int) *int { return &value }
