package rules

import (
	"context"
	"math/big"
	"slices"
)

type configuredNumber struct {
	value  int
	source string
}
type configuredValue struct {
	kind    string
	number  *big.Rat
	numbers []configuredNumber
	labels  []string
	scores  map[string]*big.Rat
}
type configuredBudget struct {
	ctx       context.Context
	remaining int
}

func (b *configuredBudget) use(n int) error {
	if b.ctx == nil {
		return ErrInvalid
	}
	if err := b.ctx.Err(); err != nil {
		return err
	}
	b.remaining -= n
	if b.remaining < 0 {
		return ErrLimit
	}
	return nil
}

type configuredEvaluation struct {
	engine  *ConfiguredEngine
	play    ConfiguredPlay
	options []ConfiguredOption
	draw    *Draw // absent while pricing: draw metrics are never evaluated at purchase
	budget  *configuredBudget
}

func configuredScalar(n int) configuredValue {
	return configuredValue{kind: "number", number: big.NewRat(int64(n), 1)}
}

func configuredTransform(op string, n, parameter int) (int, error) {
	switch op {
	case "modulo":
		if parameter <= 0 {
			return 0, ErrInvalid
		}
		return n % parameter, nil
	case "quotient":
		if parameter <= 0 {
			return 0, ErrInvalid
		}
		return n / parameter, nil
	case "digit_sum":
		v := 0
		for ; n > 0; n /= 10 {
			v += n % 10
		}
		return v, nil
	default:
		return 0, ErrInvalid
	}
}

func (e *configuredEvaluation) scope(s *ConfiguredScope) ([]configuredNumber, error) {
	if e.draw == nil || s == nil {
		return nil, ErrInvalid
	}
	var out []configuredNumber
	appendValues := func(values []int, source string) {
		for _, v := range values {
			out = append(out, configuredNumber{v, source})
		}
	}
	switch s.Target {
	case "regular":
		appendValues(e.draw.Regular, "regular")
	case "special":
		appendValues(e.draw.Special, "special")
	case "digits":
		appendValues(e.draw.Digits, "digits")
	case "all":
		appendValues(e.draw.Regular, "regular")
		appendValues(e.draw.Special, "special")
		appendValues(e.draw.Digits, "digits")
	default:
		return nil, ErrInvalid
	}
	if len(s.Positions) > 0 {
		selected := make([]configuredNumber, 0, len(s.Positions))
		for _, p := range s.Positions {
			if p < 0 || p >= len(out) {
				return nil, ErrInvalid
			}
			selected = append(selected, out[p])
		}
		out = selected
	}
	if err := e.budget.use(len(out)); err != nil {
		return nil, err
	}
	return out, nil
}

func (e *configuredEvaluation) mapping(m ConfiguredMapping, n int) (bool, error) {
	if err := e.budget.use(1); err != nil {
		return false, err
	}
	switch m.Kind {
	case "numbers":
		if err := e.budget.use(len(m.Numbers)); err != nil {
			return false, err
		}
		return slices.Contains(m.Numbers, n), nil
	case "attribute":
		labels := e.engine.attributeValues[m.Attribute][n]
		for _, l := range labels {
			if slices.Contains(m.Labels, l) {
				return true, nil
			}
		}
		return false, nil
	case "all":
		matched := true
		for _, child := range m.Children {
			v, err := e.mapping(child, n)
			if err != nil {
				return false, err
			}
			matched = matched && v
		}
		return matched, nil
	case "transform":
		v, err := configuredTransform(m.Transform, n, m.Parameter)
		if err != nil {
			return false, err
		}
		return v == *m.Value, nil
	default:
		return false, ErrInvalid
	}
}

func (e *configuredEvaluation) metric(m *ConfiguredMetric) (configuredValue, error) {
	if m == nil {
		return configuredValue{}, ErrInvalid
	}
	if err := e.budget.use(1); err != nil {
		return configuredValue{}, err
	}
	switch m.Op {
	case "constant":
		return configuredScalar(*m.Value), nil
	case "selected_count":
		return configuredScalar(len(e.options)), nil
	case "selected_value":
		if len(e.options) != 1 || e.options[0].Value == nil {
			return configuredValue{}, ErrInvalid
		}
		return configuredScalar(*e.options[0].Value), nil
	case "selected_contains", "selected_tag":
		for _, o := range e.options {
			if m.Op == "selected_contains" && o.ID == m.OptionID || m.Op == "selected_tag" && slices.Contains(o.Tags, m.Tag) {
				return configuredScalar(1), nil
			}
		}
		return configuredScalar(0), nil
	case "numbers", "raw_value", "match_count", "covered_option_count", "attribute_values", "weighted_group_sum":
		values, err := e.scope(m.Scope)
		if err != nil {
			return configuredValue{}, err
		}
		switch m.Op {
		case "numbers":
			return configuredValue{kind: "numbers", numbers: values}, nil
		case "raw_value":
			if len(values) != 1 {
				return configuredValue{}, ErrInvalid
			}
			return configuredScalar(values[0].value), nil
		case "match_count", "covered_option_count":
			hits := 0
			seen := map[int]bool{}
			covered := map[string]bool{}
			for _, n := range values {
				matched := false
				for _, o := range e.options {
					if o.Mapping == nil {
						return configuredValue{}, ErrInvalid
					}
					ok, err := e.mapping(*o.Mapping, n.value)
					if err != nil {
						return configuredValue{}, err
					}
					if ok {
						matched = true
						covered[o.ID] = true
					}
				}
				if matched && (m.Counting == "occurrences" || !seen[n.value]) {
					hits++
				}
				seen[n.value] = true
			}
			if m.Op == "covered_option_count" {
				hits = len(covered)
			}
			return configuredScalar(hits), nil
		case "attribute_values":
			labels := []string{}
			for _, n := range values {
				v := e.engine.attributeValues[m.Attribute][n.value]
				if len(v) == 0 {
					return configuredValue{}, ErrInvalid
				}
				labels = append(labels, v...)
			}
			return configuredValue{kind: "labels", labels: labels}, nil
		case "weighted_group_sum":
			scores := map[string]*big.Rat{}
			attr, _ := configuredAttribute(e.engine.snapshot.Catalog, m.Attribute)
			for _, l := range attr.Labels {
				scores[l] = new(big.Rat)
			}
			for _, n := range values {
				v := e.engine.attributeValues[m.Attribute][n.value]
				if len(v) == 0 {
					return configuredValue{}, ErrInvalid
				}
				weight, err := parseOdds(m.Weights[n.source])
				if err != nil {
					return configuredValue{}, err
				}
				for _, label := range v {
					scores[label].Add(scores[label], weight)
				}
			}
			return configuredValue{kind: "scores", scores: scores}, nil
		}
	case "sum", "distinct_count", "modulo", "quotient", "digit_sum", "max_tie_count", "selected_is_unique_max":
		v, err := e.metric(m.Input)
		if err != nil {
			return configuredValue{}, err
		}
		switch m.Op {
		case "sum":
			n := new(big.Rat)
			for _, x := range v.numbers {
				n.Add(n, big.NewRat(int64(x.value), 1))
			}
			return configuredValue{kind: "number", number: n}, nil
		case "distinct_count":
			if v.kind == "labels" {
				seen := map[string]bool{}
				for _, l := range v.labels {
					seen[l] = true
				}
				return configuredScalar(len(seen)), nil
			}
			seen := map[int]bool{}
			for _, n := range v.numbers {
				seen[n.value] = true
			}
			return configuredScalar(len(seen)), nil
		case "modulo", "quotient", "digit_sum":
			if v.kind == "number" {
				if !v.number.IsInt() || !v.number.Num().IsInt64() {
					return configuredValue{}, ErrInvalid
				}
				n, err := configuredTransform(m.Op, int(v.number.Num().Int64()), m.Parameter)
				return configuredScalar(n), err
			}
			out := append([]configuredNumber(nil), v.numbers...)
			for i := range out {
				out[i].value, err = configuredTransform(m.Op, out[i].value, m.Parameter)
				if err != nil {
					return configuredValue{}, err
				}
			}
			return configuredValue{kind: "numbers", numbers: out}, nil
		case "max_tie_count", "selected_is_unique_max":
			var maximum *big.Rat
			count := 0
			winner := ""
			for label, score := range v.scores {
				if maximum == nil || score.Cmp(maximum) > 0 {
					maximum = score
					count = 1
					winner = label
				} else if score.Cmp(maximum) == 0 {
					count++
				}
			}
			if maximum == nil {
				return configuredValue{}, ErrInvalid
			}
			if m.Op == "max_tie_count" {
				return configuredScalar(count), nil
			}
			if len(e.options) != 1 || e.options[0].Mapping == nil || len(e.options[0].Mapping.Labels) != 1 {
				return configuredValue{}, ErrInvalid
			}
			if count == 1 && winner == e.options[0].Mapping.Labels[0] {
				return configuredScalar(1), nil
			}
			return configuredScalar(0), nil
		}
	}
	return configuredValue{}, ErrInvalid
}

func (e *configuredEvaluation) condition(c ConfiguredCondition) (ConfiguredTrace, error) {
	t := ConfiguredTrace{Op: c.Op}
	if err := e.budget.use(1); err != nil {
		return t, err
	}
	if c.Op == "all" || c.Op == "any" || c.Op == "not" {
		t.Matched = c.Op == "all"
		for _, child := range c.Children {
			trace, err := e.condition(child)
			if err != nil {
				return t, err
			}
			t.Children = append(t.Children, trace)
			switch c.Op {
			case "all":
				t.Matched = t.Matched && trace.Matched
			case "any":
				t.Matched = t.Matched || trace.Matched
			case "not":
				t.Matched = !trace.Matched
			}
		}
		return t, nil
	}
	left, err := e.metric(c.Left)
	if err != nil {
		return t, err
	}
	if left.kind != "number" {
		return t, ErrInvalid
	}
	t.Actual = left.number.RatString()
	if c.Op == "in" {
		for _, v := range c.Values {
			if left.number.Cmp(big.NewRat(int64(v), 1)) == 0 {
				t.Matched = true
			}
		}
		return t, nil
	}
	if c.Op == "between" {
		t.Matched = left.number.Cmp(big.NewRat(int64(*c.Min), 1)) >= 0 && left.number.Cmp(big.NewRat(int64(*c.Max), 1)) <= 0
		return t, nil
	}
	right, err := e.metric(c.Right)
	if err != nil {
		return t, err
	}
	if right.kind != "number" {
		return t, ErrInvalid
	}
	t.Expected = right.number.RatString()
	cmp := left.number.Cmp(right.number)
	switch c.Op {
	case "equals":
		t.Matched = cmp == 0
	case "greater_than":
		t.Matched = cmp > 0
	case "at_least":
		t.Matched = cmp >= 0
	case "at_most":
		t.Matched = cmp <= 0
	default:
		return t, ErrInvalid
	}
	return t, nil
}
