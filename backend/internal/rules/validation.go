package rules

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

type ValidationCase struct {
	Name                string         `json:"name"`
	Selection           Selection      `json:"selection"`
	Draw                Draw           `json:"draw"`
	Multiplier          points.Amount  `json:"multiplier"`
	ExpectedBetPoints   *points.Amount `json:"expected_bet_points"`
	ExpectedPrizePoints *points.Amount `json:"expected_prize_points"`
	ExpectedWon         *bool          `json:"expected_won"`
}

type CaseReport struct {
	Name                string         `json:"name"`
	Input               ValidationCase `json:"input"`
	ExpectedBetPoints   points.Amount  `json:"expected_bet_points"`
	ActualBetPoints     points.Amount  `json:"actual_bet_points"`
	ExpectedPrizePoints points.Amount  `json:"expected_prize_points"`
	ActualPrizePoints   points.Amount  `json:"actual_prize_points"`
	ExpectedWon         bool           `json:"expected_won"`
	ActualWon           bool           `json:"actual_won"`
	Matched             bool           `json:"matched"`
	Simulation          Simulation     `json:"simulation"`
}

type ValidationFinding struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Blocking bool   `json:"blocking"`
}

type ValidationReport struct {
	Passed         bool                `json:"passed"`
	DefinitionHash string              `json:"definition_hash"`
	Cases          []CaseReport        `json:"cases"`
	Warnings       []string            `json:"warnings"`
	Findings       []ValidationFinding `json:"findings"`
}

// ValidateCases is a pure calculation gate. Structural/execution errors are
// returned; mismatched expectations and proven unreachable tiers fail the report.
// The hash uses deterministic JSON of the typed Definition, preserving tier order.
// The existing 200000-node explanation budget is shared by the entire case suite.
func ValidateCases(ctx context.Context, d Definition, cases []ValidationCase) (ValidationReport, error) {
	report := ValidationReport{Cases: []CaseReport{}, Warnings: []string{}, Findings: []ValidationFinding{}}
	passed := true
	if ctx == nil {
		return report, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	if len(cases) == 0 {
		return report, ErrInvalid
	}
	if len(cases) > 32 {
		return report, ErrLimit
	}
	names := make(map[string]bool, len(cases))
	for _, c := range cases {
		name := strings.TrimSpace(c.Name)
		if name == "" || len(c.Name) > 120 || !utf8.ValidString(c.Name) || names[name] ||
			c.ExpectedBetPoints == nil || c.ExpectedPrizePoints == nil || c.ExpectedWon == nil ||
			*c.ExpectedBetPoints < 0 || *c.ExpectedPrizePoints < 0 {
			return report, ErrInvalid
		}
		names[name] = true
	}
	if err := ValidateDefinition(d); err != nil {
		return report, err
	}
	// Preflight exact combination counts before running any case. Using the
	// declared max_combinations would wrongly reject small suites with roomy limits.
	nodes, work := 0, 0
	for _, tier := range d.PrizeTiers {
		nodes += nodeCount(tier.Condition)
	}
	for _, c := range cases {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		if err := ValidateDraw(d.Model, c.Draw); err != nil {
			return report, err
		}
		normalized, err := NormalizeSelection(d, c.Selection)
		if err != nil {
			return report, err
		}
		lines, err := Expand(d, normalized)
		if err != nil {
			return report, err
		}
		if len(lines) > (maxTraceNodes-work)/nodes {
			return report, ErrLimit
		}
		work += len(lines) * nodes
	}
	raw, err := json.Marshal(d)
	if err != nil {
		return report, err
	}
	digest := sha256.Sum256(raw)
	report.DefinitionHash = hex.EncodeToString(digest[:])
	report.Warnings = Warnings(d)
	findings, err := validationStaticFindings(ctx, d)
	if err != nil {
		return report, err
	}
	for _, finding := range findings {
		report.Findings = append(report.Findings, finding)
		if finding.Blocking {
			passed = false
		} else {
			report.Warnings = append(report.Warnings, finding.Message)
		}
	}
	for _, c := range cases {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		out, err := SimulateContext(ctx, SimulationInput{Definition: d, Selection: c.Selection, Draw: c.Draw, Multiplier: c.Multiplier})
		if err != nil {
			return report, err
		}
		result := CaseReport{
			Name: strings.TrimSpace(c.Name), Input: validationCopyInput(c), ExpectedBetPoints: *c.ExpectedBetPoints, ActualBetPoints: out.BetPoints,
			ExpectedPrizePoints: *c.ExpectedPrizePoints, ActualPrizePoints: out.PrizePoints,
			ExpectedWon: *c.ExpectedWon, ActualWon: out.Won, Simulation: out,
		}
		result.Matched = result.ExpectedBetPoints == result.ActualBetPoints && result.ExpectedPrizePoints == result.ActualPrizePoints && result.ExpectedWon == result.ActualWon
		report.Cases = append(report.Cases, result)
		if !result.Matched {
			passed = false
			report.Findings = append(report.Findings, ValidationFinding{Code: "CASE_MISMATCH", Message: fmt.Sprintf("case %q does not match all expected bet/prize/won values", result.Name), Blocking: true})
		}
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	report.Passed = passed
	return report, nil
}

// Preserve the original case, including draw and raw candidate ordering, without
// sharing mutable slices, maps or expectation pointers with the caller.
func validationCopyInput(c ValidationCase) ValidationCase {
	c.Selection.Regular = slices.Clone(c.Selection.Regular)
	c.Selection.Special = slices.Clone(c.Selection.Special)
	c.Selection.Exclude = slices.Clone(c.Selection.Exclude)
	c.Selection.Digits = slices.Clone(c.Selection.Digits)
	for i := range c.Selection.Digits {
		c.Selection.Digits[i] = slices.Clone(c.Selection.Digits[i])
	}
	c.Selection.Attributes = validationCopyChoices(c.Selection.Attributes)
	c.Selection.Features = validationCopyChoices(c.Selection.Features)
	c.Draw.Regular = slices.Clone(c.Draw.Regular)
	c.Draw.Special = slices.Clone(c.Draw.Special)
	c.Draw.Digits = slices.Clone(c.Draw.Digits)
	bet, prize, won := *c.ExpectedBetPoints, *c.ExpectedPrizePoints, *c.ExpectedWon
	c.ExpectedBetPoints, c.ExpectedPrizePoints, c.ExpectedWon = &bet, &prize, &won
	return c
}

func validationCopyChoices[T any](choices map[string][]T) map[string][]T {
	if choices == nil {
		return nil
	}
	result := make(map[string][]T, len(choices))
	for key, values := range choices {
		result[key] = slices.Clone(values)
	}
	return result
}

// Static proofs intentionally do not combine different scalar fields as if they
// were independent, or enumerate arbitrary compound predicates. Necessary sets
// can prove an empty AND. Only exact single-scalar predicates are complemented
// or compared for overlap; selected with multiple choices is only a necessary set.
type validationScalarKey struct {
	field, target, group, value string
	position                    int
}

type validationInterval struct{ lo, hi int }
type validationSet []validationInterval
type validationScalar struct {
	key    validationScalarKey
	domain intDomain
	values validationSet
}
type validationProof struct {
	impossible, certain bool
	necessary           map[validationScalarKey]validationScalar
	scalar              *validationScalar
}

func validationStaticFindings(ctx context.Context, d Definition) ([]ValidationFinding, error) {
	findings := []ValidationFinding{}
	proofs := make([]validationProof, len(d.PrizeTiers))
	spellings := make([]string, len(d.PrizeTiers))
	for i, tier := range d.PrizeTiers {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		proofs[i] = validationAnalyze(d, tier.Condition)
		raw, _ := json.Marshal(tier.Condition)
		spellings[i] = string(raw)
		if proofs[i].impossible {
			findings = append(findings, ValidationFinding{Code: "TIER_UNREACHABLE", Message: fmt.Sprintf("tier %q is provably unreachable under scalar constraints or model invariants", tier.Code), Blocking: true})
		}
	}
	for i := range d.PrizeTiers {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for j := i + 1; j < len(d.PrizeTiers); j++ {
			if proofs[i].impossible || proofs[j].impossible {
				continue
			}
			description := ""
			if spellings[i] == spellings[j] {
				description = "share an identical predicate"
			} else if a, b := proofs[i].scalar, proofs[j].scalar; a != nil && b != nil && a.key == b.key && len(validationIntersect(a.values, b.values)) > 0 {
				description = "accept intersecting values of the same scalar field"
			}
			if description != "" {
				findings = append(findings, ValidationFinding{Code: "TIER_OVERLAP", Message: fmt.Sprintf("tiers %q and %q %s; review intentional overlap (not a proof of full compound coverage)", d.PrizeTiers[i].Code, d.PrizeTiers[j].Code, description)})
			}
		}
	}
	return findings, nil
}

func validationAnalyze(d Definition, c Condition) validationProof {
	switch c.Op {
	case "all":
		proof := validationProof{certain: true, necessary: map[validationScalarKey]validationScalar{}}
		exact := true
		for _, child := range c.Children {
			p := validationAnalyze(d, child)
			if p.impossible {
				return validationProof{impossible: true}
			}
			proof.certain = proof.certain && p.certain
			exact = exact && (p.certain || p.scalar != nil)
			for key, scalar := range p.necessary {
				if previous, ok := proof.necessary[key]; ok {
					scalar.values = validationIntersect(previous.values, scalar.values)
				}
				if len(scalar.values) == 0 {
					return validationProof{impossible: true}
				}
				proof.necessary[key] = scalar
			}
		}
		if exact && len(proof.necessary) == 1 {
			for _, scalar := range proof.necessary {
				copy := scalar
				proof.scalar = &copy
			}
		}
		return proof
	case "any":
		var scalar *validationScalar
		exact, possible := true, false
		for _, child := range c.Children {
			p := validationAnalyze(d, child)
			if p.certain {
				return validationProof{certain: true}
			}
			if p.impossible {
				continue
			}
			possible = true
			if p.scalar == nil {
				exact = false
				continue
			}
			if scalar == nil {
				copy := *p.scalar
				scalar = &copy
			} else if scalar.key != p.scalar.key {
				exact = false
			} else {
				scalar.values = validationUnion(scalar.values, p.scalar.values)
			}
		}
		if !possible {
			return validationProof{impossible: true}
		}
		if exact && scalar != nil {
			return validationScalarProof(*scalar, true)
		}
		return validationProof{}
	case "not":
		p := validationAnalyze(d, c.Children[0])
		if p.certain {
			return validationProof{impossible: true}
		}
		if p.impossible {
			return validationProof{certain: true}
		}
		if p.scalar != nil {
			scalar := *p.scalar
			scalar.values = validationComplement(scalar.values, scalar.domain)
			return validationScalarProof(scalar, true)
		}
		return validationProof{}
	}
	domain := validationDomain(d, c)
	scalar := validationScalar{key: validationScalarKey{field: c.Field, target: c.Target, group: c.AttributeGroup, value: c.AttributeValue, position: -1}, domain: domain}
	if c.Position != nil {
		scalar.key.position = *c.Position
	}
	exact := true
	switch c.Op {
	case "equals":
		scalar.values = validationSet{{*c.Value, *c.Value}}
	case "between":
		scalar.values = validationSet{{*c.Min, *c.Max}}
	case "in", "selected":
		values := c.Values
		if c.Op == "selected" {
			values = d.Selection.FeatureChoices[c.SelectionKey]
			exact = len(values) == 1
		}
		for _, value := range values {
			scalar.values = append(scalar.values, validationInterval{value, value})
		}
		scalar.values = validationUnion(scalar.values, nil)
	}
	scalar.values = validationIntersect(scalar.values, validationSet{{domain.min, domain.max}})
	return validationScalarProof(scalar, exact)
}

func validationScalarProof(scalar validationScalar, exact bool) validationProof {
	if len(scalar.values) == 0 {
		return validationProof{impossible: true}
	}
	proof := validationProof{necessary: map[validationScalarKey]validationScalar{scalar.key: scalar}}
	if exact {
		proof.scalar = &scalar
		proof.certain = len(validationComplement(scalar.values, scalar.domain)) == 0
	}
	return proof
}

func validationDomain(d Definition, c Condition) intDomain {
	domain, _ := conditionDomain(d, c) // ValidateDefinition already checked it.
	switch c.Field {
	case "draw_all_same", "draw_unique_count", "draw_first_last_same", "draw_span", "draw_consecutive":
		count, _ := targetCount(d.Model, c.Target)
		distinct := validationDistinctTarget(d.Model, c.Target)
		switch c.Field {
		case "draw_all_same":
			if count == 1 {
				return intDomain{1, 1}
			}
			if distinct || c.Target == "all" && !isDigitModel(d.Model) &&
				(d.Model.RegularCount > 1 && !d.Model.RegularPool.AllowRepeat || d.Model.SpecialCount > 1 && !d.Model.SpecialPool.AllowRepeat) {
				return intDomain{0, 0}
			}
		case "draw_unique_count":
			if distinct {
				return intDomain{count, count}
			}
		case "draw_first_last_same":
			if distinct {
				return intDomain{0, 0}
			}
		case "draw_span", "draw_consecutive":
			if count == 1 {
				return intDomain{0, 0}
			}
		}
	}
	return domain
}

func validationDistinctTarget(model Model, target string) bool {
	if isDigitModel(model) {
		return !model.AllowRepeat || model.Length == 1
	}
	if model.Type == "M_SELECT_N" {
		return true
	}
	switch target {
	case "regular":
		return !model.RegularPool.AllowRepeat || model.RegularCount <= 1
	case "special":
		return !model.SpecialPool.AllowRepeat || model.SpecialCount <= 1
	case "all":
		if model.RegularCount == 0 {
			return validationDistinctTarget(model, "special")
		}
		if model.SpecialCount == 0 {
			return validationDistinctTarget(model, "regular")
		}
		rmin, rmax, _ := poolNumberDomain(model.RegularPool)
		smin, smax, _ := poolNumberDomain(model.SpecialPool)
		return validationDistinctTarget(model, "regular") && validationDistinctTarget(model, "special") && (rmax < smin || smax < rmin)
	}
	return false
}

func validationIntersect(a, b validationSet) validationSet {
	result := validationSet{}
	for i, j := 0, 0; i < len(a) && j < len(b); {
		lo, hi := maxInt(a[i].lo, b[j].lo), minInt(a[i].hi, b[j].hi)
		if lo <= hi {
			result = append(result, validationInterval{lo, hi})
		}
		if a[i].hi < b[j].hi {
			i++
		} else {
			j++
		}
	}
	return result
}

func validationUnion(a, b validationSet) validationSet {
	values := append(append(validationSet{}, a...), b...)
	sort.Slice(values, func(i, j int) bool { return values[i].lo < values[j].lo })
	result := validationSet{}
	for _, value := range values {
		if len(result) == 0 || value.lo > result[len(result)-1].hi+1 {
			result = append(result, value)
		} else if value.hi > result[len(result)-1].hi {
			result[len(result)-1].hi = value.hi
		}
	}
	return result
}

func validationComplement(values validationSet, domain intDomain) validationSet {
	result := validationSet{}
	next := domain.min
	for _, value := range values {
		if next < value.lo {
			result = append(result, validationInterval{next, value.lo - 1})
		}
		next = value.hi + 1
	}
	if next <= domain.max {
		result = append(result, validationInterval{next, domain.max})
	}
	return result
}
