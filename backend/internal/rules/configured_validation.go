package rules

import (
	"fmt"
	"math/big"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

func configuredID(s string) bool { return s != "" && len(s) <= 120 && strings.TrimSpace(s) == s }
func configuredLabel(l ConfiguredLabel) bool {
	return configuredID(l.English) && configuredID(l.Chinese)
}

func configuredAttribute(c ConfiguredCatalog, id string) (ConfiguredAttribute, bool) {
	for _, a := range c.Attributes {
		if a.ID == id {
			return a, true
		}
	}
	return ConfiguredAttribute{}, false
}

func validateConfiguredScope(c ConfiguredCatalog, s *ConfiguredScope) error {
	if s == nil {
		return ErrInvalid
	}
	n, err := targetCount(c.Model, s.Target)
	if err != nil || n < 1 {
		return ErrInvalid
	}
	seen := map[int]bool{}
	for _, p := range s.Positions {
		if p < 0 || p >= n || seen[p] {
			return ErrInvalid
		}
		seen[p] = true
	}
	return nil
}

func configuredNode(depth int, nodes *int) error {
	*nodes++
	if depth > maxConditionDepth || *nodes > maxConditionNodes {
		return ErrLimit
	}
	return nil
}

func validateConfiguredMapping(c ConfiguredCatalog, m ConfiguredMapping, depth int, nodes *int) error {
	if err := configuredNode(depth, nodes); err != nil {
		return err
	}
	a := ConfiguredMapping{Kind: m.Kind}
	switch m.Kind {
	case "numbers":
		a.Numbers = m.Numbers
		if len(m.Numbers) == 0 || len(m.Numbers) > maxDSLItems || hasDuplicates(m.Numbers) {
			return ErrInvalid
		}
		pool, err := unionPoolValues(c.Model)
		if err != nil {
			return err
		}
		for _, n := range m.Numbers {
			if !slices.Contains(pool, n) {
				return ErrInvalid
			}
		}
	case "attribute":
		a.Attribute, a.Labels = m.Attribute, m.Labels
		attr, ok := configuredAttribute(c, m.Attribute)
		if !ok || len(m.Labels) == 0 || len(m.Labels) > maxDSLItems {
			return ErrInvalid
		}
		seen := map[string]bool{}
		for _, label := range m.Labels {
			if !slices.Contains(attr.Labels, label) || seen[label] {
				return ErrInvalid
			}
			seen[label] = true
		}
	case "all":
		a.Children = m.Children
		if len(m.Children) < 1 || len(m.Children) > 32 {
			return ErrInvalid
		}
		for _, child := range m.Children {
			if err := validateConfiguredMapping(c, child, depth+1, nodes); err != nil {
				return err
			}
		}
	case "transform":
		a.Transform, a.Parameter, a.Value = m.Transform, m.Parameter, m.Value
		if m.Value == nil || !validDSLNumber(*m.Value) {
			return ErrInvalid
		}
		if m.Transform == "digit_sum" {
			if m.Parameter != 0 {
				return ErrInvalid
			}
		} else if (m.Transform != "modulo" && m.Transform != "quotient") || m.Parameter < 1 || m.Parameter > maxDSLNumber {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	if !reflect.DeepEqual(a, m) {
		return ErrInvalid
	}
	return nil
}

// Metric signatures are checked before execution. No scalar/sequence/label
// coercions or first-element guesses are performed by the evaluator.
func validateConfiguredMetric(c ConfiguredCatalog, p ConfiguredPlay, m *ConfiguredMetric, priceOnly bool, depth int, nodes *int) (string, error) {
	if m == nil {
		return "", ErrInvalid
	}
	if err := configuredNode(depth, nodes); err != nil {
		return "", err
	}
	a := ConfiguredMetric{Op: m.Op}
	kind := "number"
	options := c.Domains[p.Selection.Domain].Options
	switch m.Op {
	case "constant":
		a.Value = m.Value
		if m.Value == nil || !validDSLNumber(*m.Value) {
			return "", ErrInvalid
		}
	case "selected_count":
	case "selected_value":
		for _, o := range options {
			if o.Value == nil {
				return "", ErrInvalid
			}
		}
		if p.Selection.PerLine != 1 || p.Selection.Mode == "as_selected" {
			return "", ErrInvalid
		}
	case "selected_contains":
		a.OptionID = m.OptionID
		if !slices.ContainsFunc(options, func(o ConfiguredOption) bool { return o.ID == m.OptionID }) {
			return "", ErrInvalid
		}
	case "selected_tag":
		a.Tag = m.Tag
		if !configuredID(m.Tag) || !slices.ContainsFunc(options, func(o ConfiguredOption) bool { return slices.Contains(o.Tags, m.Tag) }) {
			return "", ErrInvalid
		}
	case "numbers", "raw_value", "match_count", "covered_option_count", "attribute_values", "weighted_group_sum":
		if priceOnly {
			return "", ErrInvalid
		}
		a.Scope = m.Scope
		if err := validateConfiguredScope(c, m.Scope); err != nil {
			return "", err
		}
		switch m.Op {
		case "numbers":
			kind = "numbers"
		case "raw_value":
			n, _ := targetCount(c.Model, m.Scope.Target)
			if len(m.Scope.Positions) > 0 {
				n = len(m.Scope.Positions)
			}
			if n != 1 {
				return "", ErrInvalid
			}
		case "match_count", "covered_option_count":
			for _, o := range options {
				if o.Mapping == nil {
					return "", ErrInvalid
				}
			}
			if m.Op == "match_count" {
				a.Counting = m.Counting
				if m.Counting != "occurrences" && m.Counting != "distinct_numbers" {
					return "", ErrInvalid
				}
			}
		case "attribute_values", "weighted_group_sum":
			a.Attribute = m.Attribute
			if _, ok := configuredAttribute(c, m.Attribute); !ok {
				return "", ErrInvalid
			}
			if m.Op == "attribute_values" {
				kind = "labels"
			} else {
				kind = "scores"
				a.Weights = m.Weights
				needed := []string{m.Scope.Target}
				if m.Scope.Target == "all" {
					if c.Model.Type == "DIGITS_0_9" {
						needed = []string{"digits"}
					} else {
						needed = nil
						if c.Model.RegularCount > 0 {
							needed = append(needed, "regular")
						}
						if c.Model.SpecialCount > 0 {
							needed = append(needed, "special")
						}
					}
				}
				if len(m.Weights) != len(needed) {
					return "", ErrInvalid
				}
				for _, key := range needed {
					if _, err := parseOdds(m.Weights[key]); err != nil {
						return "", ErrInvalid
					}
				}
			}
		}
	case "sum", "distinct_count", "modulo", "quotient", "digit_sum", "max_tie_count", "selected_is_unique_max":
		a.Input = m.Input
		input, err := validateConfiguredMetric(c, p, m.Input, priceOnly, depth+1, nodes)
		if err != nil {
			return "", err
		}
		switch m.Op {
		case "sum":
			if input != "numbers" {
				return "", ErrInvalid
			}
		case "distinct_count":
			if input != "numbers" && input != "labels" {
				return "", ErrInvalid
			}
		case "modulo", "quotient", "digit_sum":
			if input != "number" && input != "numbers" {
				return "", ErrInvalid
			}
			kind = input
			if m.Op != "digit_sum" {
				a.Parameter = m.Parameter
				if m.Parameter < 1 || m.Parameter > maxDSLNumber {
					return "", ErrInvalid
				}
			}
		case "max_tie_count", "selected_is_unique_max":
			if input != "scores" {
				return "", ErrInvalid
			}
			if m.Op == "selected_is_unique_max" {
				for _, o := range options {
					if o.Mapping == nil || o.Mapping.Kind != "attribute" || o.Mapping.Attribute != m.Input.Attribute || len(o.Mapping.Labels) != 1 {
						return "", ErrInvalid
					}
				}
				if p.Selection.PerLine != 1 {
					return "", ErrInvalid
				}
			}
		}
	default:
		return "", ErrInvalid
	}
	if !reflect.DeepEqual(a, *m) {
		return "", ErrInvalid
	}
	return kind, nil
}

func validateConfiguredCondition(c ConfiguredCatalog, p ConfiguredPlay, v ConfiguredCondition, priceOnly bool, depth int, nodes *int) error {
	if err := configuredNode(depth, nodes); err != nil {
		return err
	}
	a := ConfiguredCondition{Op: v.Op}
	switch v.Op {
	case "all", "any", "not":
		a.Children = v.Children
		if len(v.Children) < 1 || len(v.Children) > 32 || v.Op == "not" && len(v.Children) != 1 {
			return ErrInvalid
		}
		for _, child := range v.Children {
			if err := validateConfiguredCondition(c, p, child, priceOnly, depth+1, nodes); err != nil {
				return err
			}
		}
	case "equals", "greater_than", "at_least", "at_most", "in", "between":
		a.Left = v.Left
		kind, err := validateConfiguredMetric(c, p, v.Left, priceOnly, depth+1, nodes)
		if err != nil {
			return err
		}
		if kind != "number" {
			return ErrInvalid
		}
		if v.Op == "in" {
			a.Values = v.Values
			if len(v.Values) < 1 || len(v.Values) > 100 || hasDuplicates(v.Values) {
				return ErrInvalid
			}
			for _, n := range v.Values {
				if !validDSLNumber(n) {
					return ErrInvalid
				}
			}
		} else if v.Op == "between" {
			a.Min, a.Max = v.Min, v.Max
			if v.Min == nil || v.Max == nil || *v.Min > *v.Max || !validDSLNumber(*v.Min) || !validDSLNumber(*v.Max) {
				return ErrInvalid
			}
		} else {
			a.Right = v.Right
			kind, err = validateConfiguredMetric(c, p, v.Right, priceOnly, depth+1, nodes)
			if err != nil {
				return err
			}
			if kind != "number" {
				return ErrInvalid
			}
		}
	default:
		return ErrInvalid
	}
	if !reflect.DeepEqual(a, v) {
		return ErrInvalid
	}
	return nil
}

func validateConfiguredPrice(c ConfiguredCatalog, p ConfiguredPlay, v ConfiguredPrice) error {
	a := ConfiguredPrice{Kind: v.Kind}
	switch v.Kind {
	case "fixed":
		a.Key = v.Key
		if !configuredID(v.Key) {
			return ErrInvalid
		}
	case "option":
		if p.Selection.PerLine != 1 || p.Selection.Mode == "as_selected" {
			return ErrInvalid
		}
		for _, o := range c.Domains[p.Selection.Domain].Options {
			if !configuredID(o.PriceKey) {
				return ErrInvalid
			}
		}
	case "cases":
		a.Cases = v.Cases
		if len(v.Cases) < 1 || len(v.Cases) > 32 {
			return ErrInvalid
		}
		for _, v := range v.Cases {
			if !configuredID(v.Key) {
				return ErrInvalid
			}
			n := 0
			if err := validateConfiguredCondition(c, p, v.When, true, 1, &n); err != nil {
				return err
			}
		}
	default:
		return ErrInvalid
	}
	if !reflect.DeepEqual(a, v) {
		return ErrInvalid
	}
	return nil
}

func ValidateConfiguredCatalog(c ConfiguredCatalog) error {
	if c.Funding != "" && c.Funding != "reserve" {
		return ErrInvalid
	}
	if c.PeriodPrizeCap != nil && *c.PeriodPrizeCap <= 0 {
		return ErrInvalid
	}
	if c.SchemaVersion != 2 || c.SemanticsVersion != 1 || ValidateModel(c.Model) != nil || c.MaxLines < 1 || c.MaxLines > MaxCombinations || len(c.Plays) < 1 || len(c.Plays) > maxDSLItems || len(c.Domains) < 1 || len(c.Domains) > maxDSLItems {
		return ErrInvalid
	}
	if len(c.Plates) < 1 || len(c.Plates) > 16 {
		return ErrInvalid
	}
	plates := map[string]bool{}
	for _, plate := range c.Plates {
		if !configuredID(plate) || plates[plate] {
			return ErrInvalid
		}
		plates[plate] = true
	}
	lo, e := parseOdds(c.PriceMin)
	if e != nil {
		return e
	}
	hi, e := parseOdds(c.PriceMax)
	if e != nil || lo.Cmp(hi) > 0 {
		return ErrInvalid
	}
	attrs := map[string]bool{}
	for _, a := range c.Attributes {
		if !configuredID(a.ID) || attrs[a.ID] || len(a.Labels) < 1 || len(a.Labels) > maxDSLItems {
			return ErrInvalid
		}
		attrs[a.ID] = true
		seen := map[string]bool{}
		for _, l := range a.Labels {
			if !configuredID(l) || seen[l] {
				return ErrInvalid
			}
			seen[l] = true
		}
		pool, err := unionPoolValues(c.Model)
		if err != nil {
			return err
		}
		for key, l := range a.Fixed {
			n, err := strconv.Atoi(key)
			if err != nil || strconv.Itoa(n) != key || !slices.Contains(pool, n) || !seen[l] {
				return ErrInvalid
			}
		}
	}
	for id, d := range c.Domains {
		if !configuredID(id) || len(d.Options) < 1 || len(d.Options) > maxDSLItems {
			return ErrInvalid
		}
		seen := map[string]bool{}
		for _, o := range d.Options {
			if !configuredID(o.ID) || !configuredLabel(o.Label) || seen[o.ID] {
				return ErrInvalid
			}
			seen[o.ID] = true
			k := 0
			if o.Mapping != nil {
				k++
				n := 0
				if e := validateConfiguredMapping(c, *o.Mapping, 1, &n); e != nil {
					return e
				}
			}
			if o.Value != nil {
				k++
				if !validDSLNumber(*o.Value) {
					return ErrInvalid
				}
			}
			if o.Reference != nil {
				k++
				if !configuredID(o.Reference.PlayID) || !configuredID(o.Reference.OptionID) || !configuredID(o.Slot) {
					return ErrInvalid
				}
			}
			if k != 1 {
				return ErrInvalid
			}
			tags := map[string]bool{}
			for _, tag := range o.Tags {
				if !configuredID(tag) || tags[tag] {
					return ErrInvalid
				}
				tags[tag] = true
			}
		}
	}
	plays := map[string]ConfiguredPlay{}
	for _, p := range c.Plays {
		if !configuredID(p.ID) || !configuredID(p.Group) || !configuredLabel(p.Label) || plays[p.ID].ID != "" {
			return ErrInvalid
		}
		plays[p.ID] = p
		d, ok := c.Domains[p.Selection.Domain]
		s := p.Selection
		if !ok || s.Min < 1 || s.Min > s.Max || s.Max > len(d.Options) || p.MaxMultiplier < 1 || p.Rounding != "half_up" || !slices.Contains([]string{"line", "tier", "order"}, p.RoundingScope) || p.Cap != nil && *p.Cap <= 0 {
			return ErrInvalid
		}
		if p.StakeMode == "per_line" {
			if p.UnitPoints != 0 || p.MaxMultiplier != 1 {
				return ErrInvalid
			}
		} else if p.StakeMode != "unit_multiplier" || p.UnitPoints <= 0 {
			return ErrInvalid
		}
		switch s.Mode {
		case "singles":
			if s.PerLine != 1 {
				return ErrInvalid
			}
		case "combinations":
			if s.PerLine < 1 || s.PerLine > s.Min {
				return ErrInvalid
			}
		case "as_selected":
			if s.PerLine != 0 {
				return ErrInvalid
			}
		default:
			return ErrInvalid
		}
		if s.DistinctSlot {
			for _, o := range d.Options {
				if !configuredID(o.Slot) {
					return ErrInvalid
				}
			}
		}
		if p.PushWhen != nil {
			n := 0
			if e := validateConfiguredCondition(c, p, *p.PushWhen, false, 1, &n); e != nil {
				return fmt.Errorf("%s push: %w", p.ID, e)
			}
		}
		if p.Settlement == "decision_table" {
			if err := validateConfiguredDecisions(c, p); err != nil {
				return err
			}
			continue
		}
		if p.Decisions != nil || p.RiskMultiplier != 0 || c.Funding == "reserve" {
			return ErrInvalid
		}
		if p.Settlement == "product_of_legs" {
			if s.Mode != "as_selected" || !s.DistinctSlot || p.PushWhen != nil || len(p.Tiers) != 0 || p.MixedPolicy != "" || p.RoundingScope != "line" || p.Cap != nil {
				return ErrInvalid
			}
			for _, o := range d.Options {
				if o.Reference == nil {
					return ErrInvalid
				}
			}
			continue
		}
		if p.Settlement != "tiers" || len(p.Tiers) < 1 || len(p.Tiers) > 32 {
			return ErrInvalid
		}
		for _, o := range d.Options {
			if o.Reference != nil {
				return ErrInvalid
			}
		}
		tiers := map[string]bool{}
		exclusive, additive := false, false
		for _, t := range p.Tiers {
			if !configuredID(t.ID) || tiers[t.ID] || t.Cap != nil && *t.Cap <= 0 {
				return ErrInvalid
			}
			tiers[t.ID] = true
			exclusive = exclusive || t.Exclusive
			additive = additive || !t.Exclusive
			n := 0
			if e := validateConfiguredCondition(c, p, t.When, false, 1, &n); e != nil {
				return fmt.Errorf("%s/%s condition: %w", p.ID, t.ID, e)
			}
			if e := validateConfiguredPrice(c, p, t.Price); e != nil {
				return e
			}
			if t.Payout.Kind == "fixed_return" {
				if t.Payout.Count != nil {
					return ErrInvalid
				}
			} else if t.Payout.Kind == "net_profit_per_hit" {
				kind, e := validateConfiguredMetric(c, p, t.Payout.Count, false, 1, &n)
				if e != nil || kind != "number" || lo.Cmp(big.NewRat(1, 1)) < 0 {
					return ErrInvalid
				}
				bound, e := configuredCountBound(c, p, t.Payout.Count)
				if e != nil || bound < 1 {
					return ErrInvalid
				}
			} else {
				return ErrInvalid
			}
		}
		if exclusive && additive {
			if !slices.Contains([]string{"max_all", "max_exclusive_plus_additive"}, p.MixedPolicy) {
				return ErrInvalid
			}
		} else if p.MixedPolicy != "" {
			return ErrInvalid
		}
	}
	for _, p := range c.Plays {
		if p.Settlement == "product_of_legs" {
			for _, o := range c.Domains[p.Selection.Domain].Options {
				r := o.Reference
				leg, ok := plays[r.PlayID]
				if !ok || leg.Settlement != "tiers" || leg.Selection.Mode != "singles" || leg.Selection.Min != 1 || leg.Selection.PerLine != 1 || leg.StakeMode != "per_line" || leg.RoundingScope != "line" || leg.Cap != nil || len(leg.Tiers) != 1 || leg.Tiers[0].Payout.Kind != "fixed_return" || leg.Tiers[0].Cap != nil {
					return ErrInvalid
				}
				if !slices.ContainsFunc(c.Domains[leg.Selection.Domain].Options, func(v ConfiguredOption) bool { return v.ID == r.OptionID }) {
					return ErrInvalid
				}
			}
		}
	}
	for name, ids := range c.Aggregates {
		if !configuredID(name) || len(ids) == 0 || len(slices.Compact(slices.Sorted(slices.Values(ids)))) != len(ids) {
			return ErrInvalid
		}
		for _, id := range ids {
			if plays[id].ID == "" {
				return ErrInvalid
			}
		}
	}
	return nil
}
