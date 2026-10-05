package rules

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"regexp"
	"strings"
	"unicode/utf8"
)

var namePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,47}$`)
var oddsPattern = regexp.MustCompile(`^(0|[1-9][0-9]{0,9})(\.[0-9]{1,6})?$`)

// Definitions reject duplicate keys at every depth, including attribute maps.
// An operator must never review one spelling while the server uses another.
func uniqueKeys(dec *json.Decoder) error {
	t, err := dec.Token()
	if err != nil {
		return err
	}
	d, ok := t.(json.Delim)
	if !ok {
		return nil
	}
	switch d {
	case '{':
		seen := map[string]bool{}
		for dec.More() {
			key, e := dec.Token()
			if e != nil {
				return e
			}
			k, ok := key.(string)
			if !ok || seen[k] {
				return ErrInvalid
			}
			seen[k] = true
			if e = uniqueKeys(dec); e != nil {
				return e
			}
		}
	case '[':
		for dec.More() {
			if err = uniqueKeys(dec); err != nil {
				return err
			}
		}
	default:
		return ErrInvalid
	}
	_, err = dec.Token()
	return err
}
func (d *Definition) UnmarshalJSON(raw []byte) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	if err := uniqueKeys(dec); err != nil {
		return fmt.Errorf("%w: ambiguous definition: %v", ErrInvalid, err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return ErrInvalid
	}
	type plain Definition
	var out plain
	dec = json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&out); err != nil {
		return fmt.Errorf("%w: definition schema: %v", ErrInvalid, err)
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return ErrInvalid
	}
	*d = Definition(out)
	return nil
}
func parseOdds(value string) (*big.Rat, error) {
	if !oddsPattern.MatchString(value) {
		return nil, fmt.Errorf("%w: odds must be a positive exact decimal", ErrInvalid)
	}
	out, ok := new(big.Rat).SetString(value)
	if !ok || out.Sign() <= 0 {
		return nil, ErrInvalid
	}
	return out, nil
}
func nodeCount(c Condition) int {
	n := 1
	for _, child := range c.Children {
		n += nodeCount(child)
	}
	return n
}
func containsNumber(p Pool, n int) bool {
	if len(p.Values) > 0 {
		for _, v := range p.Values {
			if v == n {
				return true
			}
		}
		return false
	}
	return n >= p.Min && n <= p.Max
}
func allowedAttributeNumber(m Model, n int) bool {
	if m.Type == "DIGITS_0_9" {
		return n >= 0 && n <= 9
	}
	return containsNumber(m.RegularPool, n) || containsNumber(m.SpecialPool, n)
}

func ValidateDefinition(d Definition) error {
	if d.SchemaVersion != 1 || d.UnitPoints <= 0 || d.Rounding != "half_up" || d.Limits.MaxCombinations < 1 || d.Limits.MaxCombinations > MaxCombinations || d.Limits.MaxMultiplier < 1 || len(d.PrizeTiers) < 1 || len(d.PrizeTiers) > 32 {
		return ErrInvalid
	}
	if err := ValidateModel(d.Model); err != nil {
		return err
	}
	if d.RoundingScope != "order" && d.RoundingScope != "line" && d.RoundingScope != "tier" {
		return ErrInvalid
	}
	if d.CapPoints != nil && *d.CapPoints <= 0 || d.Limits.MaxBetPoints != nil && *d.Limits.MaxBetPoints <= 0 {
		return ErrInvalid
	}
	if len(d.NumberAttributes) > 16 {
		return ErrLimit
	}
	for group, labels := range d.NumberAttributes {
		if !namePattern.MatchString(group) || len(labels) == 0 || len(labels) > 32 {
			return ErrInvalid
		}
		for label, numbers := range labels {
			if !namePattern.MatchString(label) || len(numbers) < 1 || len(numbers) > 10000 {
				return ErrInvalid
			}
			seen := map[int]bool{}
			for _, n := range numbers {
				if !allowedAttributeNumber(d.Model, n) || seen[n] {
					return ErrInvalid
				}
				seen[n] = true
			}
		}
	}
	sr := d.Selection
	if sr.RegularCount < 0 || sr.SpecialCount < 0 || sr.RegularCount > d.Model.RegularCount || sr.SpecialCount > d.Model.SpecialCount {
		return ErrInvalid
	}
	switch sr.Mode {
	case "numbers":
		if sr.ExcludeCount != 0 || len(sr.AttributeGroups) != 0 || len(sr.FeatureChoices) != 0 {
			return ErrInvalid
		}
		if d.Model.Type == "DIGITS_0_9" {
			if sr.RegularCount != 0 || sr.SpecialCount != 0 {
				return ErrInvalid
			}
		} else if sr.RegularCount+sr.SpecialCount == 0 {
			return ErrInvalid
		}
	case "exclude":
		if sr.ExcludeCount < 1 || sr.ExcludeCount > 100 || sr.RegularCount != 0 || sr.SpecialCount != 0 || len(sr.AttributeGroups) != 0 || len(sr.FeatureChoices) != 0 {
			return ErrInvalid
		}
	case "attributes":
		if sr.ExcludeCount != 0 || sr.RegularCount != 0 || sr.SpecialCount != 0 || len(sr.AttributeGroups) < 1 || len(sr.AttributeGroups) > 16 || len(sr.FeatureChoices) != 0 {
			return ErrInvalid
		}
		seen := map[string]bool{}
		for _, g := range sr.AttributeGroups {
			if len(d.NumberAttributes[g]) == 0 || seen[g] {
				return ErrInvalid
			}
			seen[g] = true
		}
	case "features":
		if sr.ExcludeCount != 0 || sr.RegularCount != 0 || sr.SpecialCount != 0 || len(sr.AttributeGroups) != 0 || len(sr.FeatureChoices) < 1 || len(sr.FeatureChoices) > 16 {
			return ErrInvalid
		}
		for key, values := range sr.FeatureChoices {
			if !namePattern.MatchString(key) || len(values) < 1 || len(values) > 100 {
				return ErrInvalid
			}
			seen := map[int]bool{}
			for _, v := range values {
				if seen[v] || v < 0 || v > 20000000 {
					return ErrInvalid
				}
				seen[v] = true
			}
		}
	default:
		return ErrInvalid
	}
	codes := map[string]bool{}
	exclusive, additive := false, false
	for _, tier := range d.PrizeTiers {
		if !namePattern.MatchString(tier.Code) || codes[tier.Code] {
			return ErrInvalid
		}
		codes[tier.Code] = true
		if _, err := parseOdds(tier.Odds); err != nil {
			return err
		}
		if tier.CapPoints != nil && *tier.CapPoints <= 0 {
			return ErrInvalid
		}
		if err := ValidateCondition(d, tier.Condition); err != nil {
			return err
		}
		exclusive = exclusive || tier.Exclusive
		additive = additive || !tier.Exclusive
	}
	if d.MixedTierPolicy != "" && d.MixedTierPolicy != "max_all" && d.MixedTierPolicy != "max_exclusive_plus_additive" {
		return ErrInvalid
	}
	if exclusive && additive && d.MixedTierPolicy == "" {
		return fmt.Errorf("%w: mixed tiers require an explicit policy", ErrInvalid)
	}
	return nil
}

// ValidateDefinition does not pretend that sample runs prove arbitrary
// overlapping predicates unreachable. Lifecycle approval adds explicit cases.
func Warnings(d Definition) []string {
	out := []string{"sample simulation does not prove complete payout coverage or commercial odds safety"}
	conditions := map[string]bool{}
	for _, t := range d.PrizeTiers {
		raw, _ := json.Marshal(t.Condition)
		key := string(raw)
		if conditions[key] {
			out = append(out, "multiple tiers share an identical predicate; verify intentional overlap")
		}
		conditions[key] = true
	}
	return out
}
func ValidateReason(reason string) bool {
	return strings.TrimSpace(reason) != "" && len(reason) <= 500 && utf8.ValidString(reason)
}
