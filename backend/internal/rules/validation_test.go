package rules

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

func validationAmount(n int64) *points.Amount { v := points.Amount(n); return &v }
func validationBool(v bool) *bool             { return &v }

func validationDefinition() Definition {
	return Definition{
		SchemaVersion: 1,
		Model:         Model{Type: "DIGITS_0_9", Length: 1, Ordered: true, AllowRepeat: true},
		Selection:     SelectionRule{Mode: "numbers"}, UnitPoints: 2,
		PrizeTiers: []Tier{{Code: "DIGIT", Condition: Condition{Op: "equals", Field: "draw_digit", Target: "digits", Position: ip(0), Value: ip(5)}, Odds: "3", Exclusive: true}},
		Rounding:   "half_up", RoundingScope: "order",
		Limits: Limits{MaxCombinations: 100, MaxMultiplier: 10},
	}
}

func validationCase() ValidationCase {
	return ValidationCase{
		Name: "winning digit", Selection: Selection{Digits: [][]int{{5}}}, Draw: Draw{Digits: []int{5}}, Multiplier: 2,
		ExpectedBetPoints: validationAmount(4), ExpectedPrizePoints: validationAmount(12), ExpectedWon: validationBool(true),
	}
}

func validationHasFinding(report ValidationReport, code string, blocking bool) bool {
	for _, finding := range report.Findings {
		if finding.Code == code && finding.Blocking == blocking && finding.Message != "" {
			return true
		}
	}
	return false
}

func TestValidateCasesExactComparisonAndMismatchReport(t *testing.T) {
	d := validationDefinition()
	c := validationCase()
	d.UnitPoints = 9_007_199_254_740_993
	d.PrizeTiers[0].Odds = "1"
	c.Multiplier = 1
	c.ExpectedBetPoints = validationAmount(9_007_199_254_740_993)
	c.ExpectedPrizePoints = validationAmount(9_007_199_254_740_993)
	report, err := ValidateCases(context.Background(), d, []ValidationCase{c})
	if err != nil || !report.Passed || len(report.Cases) != 1 || !report.Cases[0].Matched {
		t.Fatalf("exact int64 comparison: %+v, %v", report, err)
	}
	c.ExpectedBetPoints = validationAmount(9_007_199_254_740_994)
	report, err = ValidateCases(context.Background(), d, []ValidationCase{c})
	if err != nil || report.Passed || report.Cases[0].Matched || !validationHasFinding(report, "CASE_MISMATCH", true) {
		t.Fatalf("valid simulation mismatch must be a failed report: %+v, %v", report, err)
	}
	result := report.Cases[0]
	if result.ActualBetPoints != 9_007_199_254_740_993 || result.ExpectedBetPoints != 9_007_199_254_740_994 || !result.ActualWon || !result.ExpectedWon {
		t.Fatalf("expected/actual details lost: %+v", result)
	}
	raw, err := json.Marshal(report)
	if err != nil || !strings.Contains(string(raw), `"actual_bet_points":"9007199254740993"`) {
		t.Fatalf("report amount must remain a JSON integer string: %s, %v", raw, err)
	}
}

func TestValidateCasesCanonicalDefinitionHash(t *testing.T) {
	d := validationDefinition()
	d.NumberAttributes = map[string]map[string][]int{"z": {"one": {1}}, "a": {"two": {2}}}
	first, err := ValidateCases(context.Background(), d, []ValidationCase{validationCase()})
	if err != nil || len(first.DefinitionHash) != 64 {
		t.Fatalf("definition hash: %+v, %v", first, err)
	}
	raw, _ := json.Marshal(d)
	digest := sha256.Sum256(raw)
	if first.DefinitionHash != hex.EncodeToString(digest[:]) {
		t.Fatal("hash did not match canonical typed definition JSON")
	}
	d.NumberAttributes = map[string]map[string][]int{"a": {"two": {2}}, "z": {"one": {1}}}
	second, err := ValidateCases(context.Background(), d, []ValidationCase{validationCase()})
	if err != nil || first.DefinitionHash != second.DefinitionHash {
		t.Fatalf("map insertion order affected canonical hash: %s, %s, %v", first.DefinitionHash, second.DefinitionHash, err)
	}
	d.PrizeTiers[0].Odds = "4"
	changed, err := ValidateCases(context.Background(), d, []ValidationCase{validationCase()})
	if err != nil || changed.DefinitionHash == first.DefinitionHash {
		t.Fatalf("changed definition retained old hash: %+v, %v", changed, err)
	}
}

func TestValidateCasesBlocksProvableConjunctionAndUnreachableTier(t *testing.T) {
	d := validationDefinition()
	d.PrizeTiers[0].Condition = Condition{Op: "all", Children: []Condition{
		{Op: "equals", Field: "draw_digit", Target: "digits", Position: ip(0), Value: ip(3)},
		{Op: "all", Children: []Condition{{Op: "between", Field: "draw_digit", Target: "digits", Position: ip(0), Min: ip(4), Max: ip(6)}}},
	}}
	c := validationCase()
	c.ExpectedPrizePoints, c.ExpectedWon = validationAmount(0), validationBool(false)
	report, err := ValidateCases(context.Background(), d, []ValidationCase{c})
	if err != nil || report.Passed || !report.Cases[0].Matched || !validationHasFinding(report, "TIER_UNREACHABLE", true) {
		t.Fatalf("provable empty AND must block even if the sample expectations match: %+v, %v", report, err)
	}
	// A false conjunction inside NOT is a legitimate true predicate.
	d.PrizeTiers[0].Condition = Condition{Op: "not", Children: []Condition{d.PrizeTiers[0].Condition}}
	c.ExpectedPrizePoints, c.ExpectedWon = validationAmount(12), validationBool(true)
	report, err = ValidateCases(context.Background(), d, []ValidationCase{c})
	if err != nil || !report.Passed || validationHasFinding(report, "TIER_UNREACHABLE", true) {
		t.Fatalf("NOT of a contradiction was incorrectly blocked: %+v, %v", report, err)
	}
	// No repeated draw digits makes an all-same three-digit tier impossible.
	d.Model.Length, d.Model.AllowRepeat = 3, false
	d.PrizeTiers[0].Condition = Condition{Op: "equals", Field: "draw_all_same", Target: "digits", Value: ip(1)}
	c.Selection, c.Draw = Selection{Digits: [][]int{{1}, {2}, {3}}}, Draw{Digits: []int{1, 2, 3}}
	c.ExpectedPrizePoints, c.ExpectedWon = validationAmount(0), validationBool(false)
	report, err = ValidateCases(context.Background(), d, []ValidationCase{c})
	if err != nil || report.Passed || !validationHasFinding(report, "TIER_UNREACHABLE", true) {
		t.Fatalf("unreachable non-repeating all-same tier was missed: %+v, %v", report, err)
	}
}

func TestValidateCasesScalarIntervalOverlapIsNonblocking(t *testing.T) {
	d := validationDefinition()
	d.PrizeTiers[0].Condition = Condition{Op: "between", Field: "draw_digit", Target: "digits", Position: ip(0), Min: ip(1), Max: ip(5)}
	second := d.PrizeTiers[0]
	second.Code = "SECOND"
	second.Condition.Min, second.Condition.Max = ip(4), ip(7)
	d.PrizeTiers = append(d.PrizeTiers, second)
	report, err := ValidateCases(context.Background(), d, []ValidationCase{validationCase()})
	if err != nil || !report.Passed || !validationHasFinding(report, "TIER_OVERLAP", false) || len(report.Warnings) < 2 {
		t.Fatalf("scalar overlap should require reviewer attention, not invalidate the cases: %+v, %v", report, err)
	}
	d.PrizeTiers[0].Condition.Min, d.PrizeTiers[0].Condition.Max = ip(1), ip(2)
	report, err = ValidateCases(context.Background(), d, []ValidationCase{validationCase()})
	if err != nil || validationHasFinding(report, "TIER_OVERLAP", false) {
		t.Fatalf("disjoint intervals were described as overlapping: %+v, %v", report, err)
	}
	d.PrizeTiers[1].Condition = d.PrizeTiers[0].Condition
	c := validationCase()
	c.Draw.Digits = []int{1}
	report, err = ValidateCases(context.Background(), d, []ValidationCase{c})
	if err != nil || !validationHasFinding(report, "TIER_OVERLAP", false) {
		t.Fatalf("duplicate predicates were not reported: %+v, %v", report, err)
	}
}

func TestValidateCasesKeepsIndependentConditionalTraces(t *testing.T) {
	d := validationDefinition()
	d.PrizeTiers[0].Condition = Condition{Op: "all", Children: []Condition{
		d.PrizeTiers[0].Condition,
		{Op: "not", Children: []Condition{{Op: "equals", Field: "draw_digit", Target: "digits", Position: ip(0), Value: ip(4)}}},
	}}
	first, second := validationCase(), validationCase()
	second.Name, second.Draw = "losing digit", Draw{Digits: []int{4}}
	second.ExpectedPrizePoints, second.ExpectedWon = validationAmount(0), validationBool(false)
	report, err := ValidateCases(context.Background(), d, []ValidationCase{first, second})
	if err != nil || !report.Passed || len(report.Cases) != 2 {
		t.Fatalf("independent cases: %+v, %v", report, err)
	}
	win := &report.Cases[0].Simulation.Lines[0].Hits[0].Trace
	lose := &report.Cases[1].Simulation.Lines[0].Hits[0].Trace
	if !win.Matched || lose.Matched || len(win.Children) != 2 || len(lose.Children) != 2 || win.Children[0].Actual != 5 || lose.Children[0].Actual != 4 || !win.Children[1].Matched || lose.Children[1].Matched {
		t.Fatalf("per-case trace outcomes mixed: %+v, %+v", win, lose)
	}
	win.Children[0].Actual = 99
	if lose.Children[0].Actual != 4 || first.Draw.Digits[0] != 5 || second.Draw.Digits[0] != 4 {
		t.Fatal("validation results alias another case or mutate input")
	}
}

func TestValidationReportPreservesOriginalInputForReviewer(t *testing.T) {
	c := validationCase()
	c.Name = "  original case  "
	c.Selection.Digits = [][]int{{5, 3}}
	c.ExpectedBetPoints, c.ExpectedPrizePoints = validationAmount(8), validationAmount(24)
	report, err := ValidateCases(context.Background(), validationDefinition(), []ValidationCase{c})
	if err != nil || !report.Passed {
		t.Fatalf("validation: %+v, %v", report, err)
	}
	input := report.Cases[0].Input
	if !reflect.DeepEqual(input, c) || input.Draw.Digits[0] != 5 || input.Selection.Digits[0][0] != 5 || report.Cases[0].Simulation.Normalized.Digits[0][0] != 3 {
		t.Fatalf("original draw/candidate order was lost: %+v", input)
	}
	raw, err := json.Marshal(report)
	if err != nil || !strings.Contains(string(raw), `"input":{"name":"  original case  "`) || !strings.Contains(string(raw), `"expected_bet_points":"8"`) {
		t.Fatalf("persisted report lacks complete original case: %s, %v", raw, err)
	}
	c.Selection.Digits[0][0], c.Draw.Digits[0] = 9, 4
	*c.ExpectedBetPoints, *c.ExpectedPrizePoints, *c.ExpectedWon = 99, 99, false
	if input.Selection.Digits[0][0] != 5 || input.Draw.Digits[0] != 5 || *input.ExpectedBetPoints != 8 || *input.ExpectedPrizePoints != 24 || !*input.ExpectedWon || input.Multiplier != 2 {
		t.Fatalf("report shares mutable original input: %+v", input)
	}
}

func TestValidateCasesRejectsIncompleteRequestsAndCancelledContext(t *testing.T) {
	d := validationDefinition()
	for _, mutate := range []func(*ValidationCase){
		func(c *ValidationCase) { c.Name = "   " },
		func(c *ValidationCase) { c.ExpectedBetPoints = nil },
		func(c *ValidationCase) { c.ExpectedPrizePoints = nil },
		func(c *ValidationCase) { c.ExpectedWon = nil },
		func(c *ValidationCase) { c.ExpectedPrizePoints = validationAmount(-1) },
	} {
		c := validationCase()
		mutate(&c)
		if _, err := ValidateCases(context.Background(), d, []ValidationCase{c}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid validation request error=%v", err)
		}
	}
	if _, err := ValidateCases(context.Background(), d, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty cases error=%v", err)
	}
	if _, err := ValidateCases(context.Background(), d, make([]ValidationCase, 33)); !errors.Is(err, ErrLimit) {
		t.Fatalf("too many cases error=%v", err)
	}
	if report, err := ValidateCases(context.Background(), d, []ValidationCase{validationCase(), validationCase()}); !errors.Is(err, ErrInvalid) || report.Passed {
		t.Fatalf("duplicate names must not produce a passing report: %+v, %v", report, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ValidateCases(ctx, d, []ValidationCase{validationCase()}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled validation error=%v", err)
	}
}

func TestValidateCasesDoesNotInventProofForIndependentFieldsOrSelectedChoices(t *testing.T) {
	d := validationDefinition()
	d.Model.Length = 2
	d.PrizeTiers[0].Condition = Condition{Op: "all", Children: []Condition{
		{Op: "equals", Field: "draw_digit", Target: "digits", Position: ip(0), Value: ip(3)},
		{Op: "equals", Field: "draw_digit", Target: "digits", Position: ip(1), Value: ip(4)},
	}}
	c := validationCase()
	c.Selection, c.Draw = Selection{Digits: [][]int{{3}, {4}}}, Draw{Digits: []int{3, 4}}
	if report, err := ValidateCases(context.Background(), d, []ValidationCase{c}); err != nil || !report.Passed {
		t.Fatalf("different positions were treated as one scalar: %+v, %v", report, err)
	}
	d = validationDefinition()
	d.Selection = SelectionRule{Mode: "features", FeatureChoices: map[string][]int{"parity": {0, 1}}}
	d.PrizeTiers[0].Condition = Condition{Op: "not", Children: []Condition{{Op: "selected", Field: "draw_parity", Target: "digits", Position: ip(0), SelectionKey: "parity"}}}
	c = validationCase()
	c.Selection = Selection{Features: map[string][]int{"parity": {0}}}
	if report, err := ValidateCases(context.Background(), d, []ValidationCase{c}); err != nil || !report.Passed {
		t.Fatalf("NOT(selected) was confused with complement of all possible feature choices: %+v, %v", report, err)
	} else {
		c.Selection.Features["parity"][0] = 1
		if report.Cases[0].Input.Selection.Features["parity"][0] != 0 {
			t.Fatal("persisted feature choices alias original input")
		}
	}
}

func TestValidateCasesFiniteSetHolesAndReachableAnyBranch(t *testing.T) {
	d := validationDefinition()
	contradiction := Condition{Op: "all", Children: []Condition{
		{Op: "in", Field: "draw_digit", Target: "digits", Position: ip(0), Values: []int{3, 1}},
		{Op: "equals", Field: "draw_digit", Target: "digits", Position: ip(0), Value: ip(2)},
	}}
	d.PrizeTiers[0].Condition = contradiction
	c := validationCase()
	c.ExpectedPrizePoints, c.ExpectedWon = validationAmount(0), validationBool(false)
	if report, err := ValidateCases(context.Background(), d, []ValidationCase{c}); err != nil || report.Passed || !validationHasFinding(report, "TIER_UNREACHABLE", true) {
		t.Fatalf("finite in-set hole was flattened into an interval: %+v, %v", report, err)
	}
	d.PrizeTiers[0].Condition = Condition{Op: "any", Children: []Condition{contradiction, validationDefinition().PrizeTiers[0].Condition}}
	c = validationCase()
	if report, err := ValidateCases(context.Background(), d, []ValidationCase{c}); err != nil || !report.Passed {
		t.Fatalf("one impossible ANY branch incorrectly blocked a reachable tier: %+v, %v", report, err)
	}
}

func TestValidateCasesSharesAggregateExecutionBudget(t *testing.T) {
	d := validationDefinition()
	d.UnitPoints = 1
	d.Model.Length = 4
	d.Limits.MaxCombinations = MaxCombinations
	d.PrizeTiers[0].Odds = "1"
	children := make([]Condition, 20)
	for i := range children {
		children[i] = Condition{Op: "equals", Field: "draw_sum", Target: "digits", Value: ip(0)}
	}
	d.PrizeTiers[0].Condition = Condition{Op: "all", Children: children}
	selection := Selection{Digits: [][]int{{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, {0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, {0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, {0, 1, 2, 3, 4}}}
	c := ValidationCase{Name: "one", Selection: selection, Draw: Draw{Digits: []int{0, 0, 0, 0}}, Multiplier: 1, ExpectedBetPoints: validationAmount(5000), ExpectedPrizePoints: validationAmount(5000), ExpectedWon: validationBool(true)}
	second := c
	second.Name = "two"
	if _, err := ValidateCases(context.Background(), d, []ValidationCase{c, second}); !errors.Is(err, ErrLimit) {
		t.Fatalf("two individually bounded 105000-node runs exceeded shared budget without rejection: %v", err)
	}
}
