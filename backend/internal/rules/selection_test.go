package rules

import (
	"errors"
	"reflect"
	"testing"
)

func TestValidateModelAndDrawAcrossAllModelKinds(t *testing.T) {
	tests := []struct {
		name  string
		model Model
		draw  Draw
		valid bool
	}{
		{
			name:  "X plus Y supports zero-based ranges",
			model: Model{Type: "X_PLUS_Y", RegularPool: Pool{Min: 0, Max: 5}, SpecialPool: Pool{Values: []int{7, 19}}, RegularCount: 2, SpecialCount: 1},
			draw:  Draw{Regular: []int{0, 5}, Special: []int{19}}, valid: true,
		},
		{
			name:  "M select N",
			model: Model{Type: "M_SELECT_N", PoolSize: 5, TotalCount: 2, RegularCount: 1, SpecialCount: 1, RegularPool: Pool{Values: []int{1, 2, 3, 4, 5}}, SpecialPool: Pool{Min: 1, Max: 5}},
			draw:  Draw{Regular: []int{1}, Special: []int{5}}, valid: true,
		},
		{
			name:  "M rejects global duplicate",
			model: Model{Type: "M_SELECT_N", PoolSize: 5, TotalCount: 2, RegularCount: 1, SpecialCount: 1, RegularPool: Pool{Min: 1, Max: 5}, SpecialPool: Pool{Values: []int{1, 2, 3, 4, 5}}},
			draw:  Draw{Regular: []int{3}, Special: []int{3}},
		},
		{
			name:  "digits allow repeated digits",
			model: Model{Type: "DIGITS_0_9", Length: 6, AllowRepeat: true, Ordered: true},
			draw:  Draw{Digits: []int{1, 2, 1, 1, 1, 1}}, valid: true,
		},
		{
			name:  "digits reject repeated digits when disabled",
			model: Model{Type: "DIGITS_0_9", Length: 6, Ordered: true},
			draw:  Draw{Digits: []int{1, 2, 1, 1, 1, 1}},
		},
		{
			name:  "wrong draw count",
			model: Model{Type: "X_PLUS_Y", RegularPool: Pool{Min: 0, Max: 5}, SpecialPool: Pool{Min: 1, Max: 3}, RegularCount: 2, SpecialCount: 1},
			draw:  Draw{Regular: []int{1}, Special: []int{2}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateDraw(test.model, test.draw)
			if test.valid && err != nil {
				t.Fatalf("ValidateDraw() error=%v", err)
			}
			if !test.valid && !errors.Is(err, ErrInvalid) {
				t.Fatalf("ValidateDraw() error=%v, want ErrInvalid", err)
			}
		})
	}
}

func TestValidateModelRejectsInvalidRangesAndCardinalities(t *testing.T) {
	validX := Model{Type: "X_PLUS_Y", RegularPool: Pool{Min: 1, Max: 49}, SpecialPool: Pool{Min: 0, Max: 9}, RegularCount: 6, SpecialCount: 1}
	invalid := []Model{
		{Type: "X_PLUS_Y", RegularPool: Pool{Min: 1, Max: 10_001}, SpecialPool: Pool{Min: 1, Max: 2}, RegularCount: 1, SpecialCount: 1},
		{Type: "X_PLUS_Y", RegularPool: Pool{Min: 1, Max: 3, Values: []int{1, 2}}, SpecialPool: Pool{Min: 1, Max: 2}, RegularCount: 1, SpecialCount: 1},
		{Type: "X_PLUS_Y", RegularPool: Pool{Min: 1, Values: []int{1, 2}}, SpecialPool: Pool{Min: 1, Max: 2}, RegularCount: 1, SpecialCount: 1},
		{Type: "X_PLUS_Y", RegularPool: Pool{Max: 3, Values: []int{1, 2}}, SpecialPool: Pool{Min: 1, Max: 2}, RegularCount: 1, SpecialCount: 1},
		{Type: "X_PLUS_Y", RegularPool: Pool{Values: []int{1, 1}}, SpecialPool: Pool{Min: 1, Max: 2}, RegularCount: 1, SpecialCount: 1},
		{Type: "M_SELECT_N", PoolSize: 5, TotalCount: 5, RegularCount: 4, SpecialCount: 1, RegularPool: Pool{Min: 1, Max: 5}, SpecialPool: Pool{Min: 1, Max: 5}},
		{Type: "M_SELECT_N", PoolSize: 5, TotalCount: 2, RegularCount: 1, SpecialCount: 1, RegularPool: Pool{Min: 1, Max: 4}, SpecialPool: Pool{Min: 1, Max: 5}},
		{Type: "M_SELECT_N", PoolSize: 5, TotalCount: 2, RegularCount: 1, SpecialCount: 1, RegularPool: Pool{Values: []int{1, 2, 3, 4, 5}}, SpecialPool: Pool{Values: []int{2, 3, 4, 5, 6}}},
		{Type: "DIGITS_0_9", Length: 0},
		{Type: "DIGITS_0_9", Length: 3, Ordered: false},
		{Type: "UNKNOWN"},
	}
	if err := ValidateModel(validX); err != nil {
		t.Fatalf("valid zero-inclusive range rejected: %v", err)
	}
	for i, model := range invalid {
		if err := ValidateModel(model); !errors.Is(err, ErrInvalid) {
			t.Errorf("invalid model %d error=%v, want ErrInvalid", i, err)
		}
	}
}

func TestDigitsExpansionDeduplicatesPerPositionAndAllows121111(t *testing.T) {
	definition := Definition{
		Model:     Model{Type: "DIGITS_0_9", Length: 6, AllowRepeat: true, Ordered: true},
		Selection: SelectionRule{Mode: "numbers"},
		Limits:    Limits{MaxCombinations: MaxCombinations},
	}
	positions := [][]int{{1, 1, 2}, {1, 2, 2}, {1, 1}, {1}, {1}, {1}}
	original := cloneSelection(Selection{Digits: positions})
	normalized, err := NormalizeSelection(definition, Selection{Digits: positions})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(normalized.Digits, [][]int{{1, 2}, {1, 2}, {1}, {1}, {1}, {1}}) {
		t.Fatalf("digit candidates not deduplicated: %v", normalized.Digits)
	}
	if !reflect.DeepEqual(positions, original.Digits) {
		t.Fatalf("NormalizeSelection mutated its input: got=%v want=%v", positions, original.Digits)
	}
	lines, err := Expand(definition, Selection{Digits: positions})
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 4 {
		t.Fatalf("digit expansion produced %d lines, want 4", len(lines))
	}
	want := [][]int{{1}, {2}, {1}, {1}, {1}, {1}}
	found := false
	for _, line := range lines {
		if reflect.DeepEqual(line.Digits, want) {
			found = true
		}
	}
	if !found {
		t.Fatalf("121111 missing from expanded lines: %+v", lines)
	}
	definition.Model.AllowRepeat = false
	if _, err := NormalizeSelection(definition, Selection{Digits: positions}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("impossible no-repeat digits error=%v, want ErrInvalid", err)
	}
	definition.Model.Length = 10
	positions = make([][]int, 10)
	for i := range positions {
		positions[i] = []int{0, 1, 2, 3, 4, 5, 6, 7, 8}
	}
	if _, err := NormalizeSelection(definition, Selection{Digits: positions}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("ten positions with only nine distinct digits error=%v, want ErrInvalid", err)
	}
}

func TestMSelectExpansionFiltersCrossPoolDuplicates(t *testing.T) {
	values := []int{1, 2, 3, 4, 5}
	definition := Definition{
		Model:     Model{Type: "M_SELECT_N", PoolSize: 5, TotalCount: 2, RegularCount: 1, SpecialCount: 1, RegularPool: Pool{Values: values}, SpecialPool: Pool{Values: []int{5, 4, 3, 2, 1}}},
		Selection: SelectionRule{Mode: "numbers", RegularCount: 1, SpecialCount: 1},
		Limits:    Limits{MaxCombinations: MaxCombinations},
	}
	selection := Selection{Regular: []int{2, 1, 2}, Special: []int{1, 3}}
	normalized, err := NormalizeSelection(definition, selection)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(normalized.Regular, []int{1, 2}) || !reflect.DeepEqual(normalized.Special, []int{1, 3}) {
		t.Fatalf("M candidates not normalized: %+v", normalized)
	}
	lines, err := Expand(definition, selection)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 3 {
		t.Fatalf("M expansion has %d lines, want 3: %+v", len(lines), lines)
	}
	for _, line := range lines {
		if intersects(line.Regular, line.Special) {
			t.Fatalf("M expansion duplicated across pools: %+v", line)
		}
	}
	if err := ValidateDraw(definition.Model, Draw{Regular: []int{1}, Special: []int{1}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("M draw repeated across pools error=%v, want ErrInvalid", err)
	}
}

func TestMSelectHallConstraintRejectsImpossibleCrossPoolQuota(t *testing.T) {
	pool := make([]int, 21)
	for i := range pool {
		pool[i] = i + 1
	}
	candidates := make([]int, 15)
	for i := range candidates {
		candidates[i] = i + 1
	}
	definition := Definition{
		Model:     Model{Type: "M_SELECT_N", PoolSize: 21, TotalCount: 20, RegularCount: 10, SpecialCount: 10, RegularPool: Pool{Values: pool}, SpecialPool: Pool{Values: append([]int(nil), pool...)}},
		Selection: SelectionRule{Mode: "numbers", RegularCount: 10, SpecialCount: 10},
	}
	_, err := NormalizeSelection(definition, Selection{Regular: candidates, Special: append([]int(nil), candidates...)})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("Hall-impossible candidate pools error=%v, want ErrInvalid", err)
	}
}

func TestSpecialOnlySelectionProducesFourBets(t *testing.T) {
	definition := Definition{
		Model:     Model{Type: "X_PLUS_Y", RegularPool: Pool{Min: 1, Max: 49}, SpecialPool: Pool{Values: []int{7, 19, 31, 43}}, RegularCount: 6, SpecialCount: 1},
		Selection: SelectionRule{Mode: "numbers", RegularCount: 0, SpecialCount: 1},
		Limits:    Limits{MaxCombinations: MaxCombinations},
	}
	selection := Selection{Special: []int{43, 7, 31, 19}}
	lines, err := Expand(definition, selection)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 4 {
		t.Fatalf("special-only expansion produced %d lines, want 4", len(lines))
	}
	for i, expected := range []int{7, 19, 31, 43} {
		if len(lines[i].Regular) != 0 || !reflect.DeepEqual(lines[i].Special, []int{expected}) {
			t.Fatalf("special-only line %d = %+v, want special %d", i, lines[i], expected)
		}
	}
	if _, err := NormalizeSelection(definition, Selection{Special: []int{7}, Digits: [][]int{{1}}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("irrelevant digit field for X_PLUS_Y error=%v, want ErrInvalid", err)
	}
}

func TestXPlusYPoolSpecificRepeatAndOrdering(t *testing.T) {
	model := Model{Type: "X_PLUS_Y", RegularPool: Pool{Values: []int{1, 2}, AllowRepeat: true}, SpecialPool: Pool{Min: 0, Max: 0}, RegularCount: 2}
	if err := ValidateDraw(model, Draw{Regular: []int{1, 1}}); err != nil {
		t.Fatalf("pool-enabled repeated draw rejected: %v", err)
	}
	model.RegularPool.AllowRepeat = false
	if err := ValidateDraw(model, Draw{Regular: []int{1, 1}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("pool-disabled repeated draw error=%v, want ErrInvalid", err)
	}

	definition := Definition{
		Model:     Model{Type: "X_PLUS_Y", RegularPool: Pool{Values: []int{1, 2}, AllowRepeat: true}, SpecialPool: Pool{Min: 0, Max: 0}, RegularCount: 2, Ordered: true},
		Selection: SelectionRule{Mode: "numbers", RegularCount: 2},
		Limits:    Limits{MaxCombinations: MaxCombinations},
	}
	lines, err := Expand(definition, Selection{Regular: []int{2, 1}})
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 4 || !reflect.DeepEqual(lines[0].Regular, []int{2, 2}) || !reflect.DeepEqual(lines[1].Regular, []int{2, 1}) || !reflect.DeepEqual(lines[2].Regular, []int{1, 2}) || !reflect.DeepEqual(lines[3].Regular, []int{1, 1}) {
		t.Fatalf("ordered repeat-with-replacement expansion mismatch: %+v", lines)
	}
}

func TestExcludeIsFixedSingleBetAndRejectsIrrelevantFields(t *testing.T) {
	definition := Definition{
		Model:     Model{Type: "X_PLUS_Y", RegularPool: Pool{Min: 0, Max: 9}, SpecialPool: Pool{Min: 10, Max: 19}, RegularCount: 2, SpecialCount: 1},
		Selection: SelectionRule{Mode: "exclude", ExcludeCount: 2},
		Limits:    Limits{MaxCombinations: MaxCombinations},
	}
	lines, err := Expand(definition, Selection{Exclude: []int{19, 0}})
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || !reflect.DeepEqual(lines[0].Exclude, []int{0, 19}) {
		t.Fatalf("exclude selection must remain one fixed bet: %+v", lines)
	}
	if _, err := NormalizeSelection(definition, Selection{Exclude: []int{0, 19}, Regular: []int{1}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("irrelevant number field error=%v, want ErrInvalid", err)
	}
	if _, err := NormalizeSelection(definition, Selection{Exclude: []int{0, 0}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("duplicate excludes error=%v, want ErrInvalid", err)
	}
}

func TestAttributesAndFeaturesExpandAsCartesianLines(t *testing.T) {
	attributes := Definition{
		Model:     Model{Type: "DIGITS_0_9", Length: 1, Ordered: true},
		Selection: SelectionRule{Mode: "attributes", AttributeGroups: []string{"color", "shape"}},
		NumberAttributes: map[string]map[string][]int{
			"color": {"red": {1}, "blue": {2}},
			"shape": {"round": {3}, "square": {4}},
		},
		Limits: Limits{MaxCombinations: MaxCombinations},
	}
	attributeLines, err := Expand(attributes, Selection{Attributes: map[string][]string{"color": {"blue", "red", "blue"}, "shape": {"round", "square"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(attributeLines) != 4 {
		t.Fatalf("attributes expanded to %d lines, want 4", len(attributeLines))
	}
	for _, line := range attributeLines {
		if len(line.Attributes) != 2 || len(line.Attributes["color"]) != 1 || len(line.Attributes["shape"]) != 1 {
			t.Fatalf("attribute line must have one value per group: %+v", line)
		}
	}
	if _, err := NormalizeSelection(attributes, Selection{Attributes: map[string][]string{"color": {"red"}}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing attribute group error=%v, want ErrInvalid", err)
	}

	features := Definition{
		Model:     Model{Type: "DIGITS_0_9", Length: 1, Ordered: true},
		Selection: SelectionRule{Mode: "features", FeatureChoices: map[string][]int{"parity": {0, 1}, "band": {1, 2, 3}}},
		Limits:    Limits{MaxCombinations: MaxCombinations},
	}
	featureLines, err := Expand(features, Selection{Features: map[string][]int{"parity": {0, 1}, "band": {3, 1, 2}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(featureLines) != 6 {
		t.Fatalf("features expanded to %d lines, want 6", len(featureLines))
	}
	for _, line := range featureLines {
		if len(line.Features) != 2 || len(line.Features["parity"]) != 1 || len(line.Features["band"]) != 1 {
			t.Fatalf("feature line must have one value per key: %+v", line)
		}
	}
	features.Selection.FeatureChoices = map[string][]int{"large": {20_000_000}}
	if _, err := NormalizeSelection(features, Selection{Features: map[string][]int{"large": {20_000_000}}}); err != nil {
		t.Fatalf("feature value at 20,000,000 rejected: %v", err)
	}
	features.Selection.FeatureChoices = map[string][]int{"too_large": {20_000_001}}
	if _, err := NormalizeSelection(features, Selection{Features: map[string][]int{"too_large": {20_000_001}}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("feature value above 20,000,000 error=%v, want ErrInvalid", err)
	}
}

func TestExpandEnforcesCombinationAndDefinitionLimits(t *testing.T) {
	definition := Definition{
		Model:     Model{Type: "X_PLUS_Y", RegularPool: Pool{Min: 0, Max: 99}, SpecialPool: Pool{Min: 0, Max: 0}, RegularCount: 6, SpecialCount: 1},
		Selection: SelectionRule{Mode: "numbers", RegularCount: 6, SpecialCount: 0},
		Limits:    Limits{MaxCombinations: 500},
	}
	selection := Selection{Regular: make([]int, 100)}
	for i := range selection.Regular {
		selection.Regular[i] = i
	}
	if _, err := Expand(definition, selection); !errors.Is(err, ErrLimit) {
		t.Fatalf("large combination expansion error=%v, want ErrLimit", err)
	}
	definition.Limits.MaxCombinations = MaxCombinations + 1
	if _, err := Expand(definition, selection); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid configured max combinations error=%v, want ErrInvalid", err)
	}
	definition.Limits.MaxCombinations = 0
	if _, err := Expand(definition, selection); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing configured max combinations error=%v, want ErrInvalid", err)
	}
}
