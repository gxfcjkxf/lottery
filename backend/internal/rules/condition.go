package rules

import (
	"sort"
	"strings"
)

const (
	maxConditionDepth = 8
	maxConditionNodes = 128
	maxDSLNumber      = 1_000_000
	maxComparison     = 20_000_000
	maxDrawSum        = 20_000_000
	maxDSLItems       = 10_000
)

type intDomain struct{ min, max int }

func validDSLNumber(value int) bool { return value >= 0 && value <= maxComparison }

func validPoolNumber(value int) bool { return value >= 0 && value <= maxDSLNumber }

func ValidateCondition(definition Definition, condition Condition) error {
	if err := ValidateModel(definition.Model); err != nil {
		return ErrInvalid
	}
	if definition.Model.RegularCount < 0 || definition.Model.SpecialCount < 0 || definition.Model.Length < 0 ||
		definition.Model.RegularCount > 10 || definition.Model.SpecialCount > 10 || definition.Model.Length > maxDSLItems ||
		definition.Selection.RegularCount < 0 || definition.Selection.SpecialCount < 0 || definition.Selection.ExcludeCount < 0 ||
		definition.Selection.RegularCount > maxDSLItems || definition.Selection.SpecialCount > maxDSLItems || definition.Selection.ExcludeCount > maxDSLItems {
		return ErrInvalid
	}
	nodes := 0
	return validateConditionNode(definition, condition, 1, &nodes)
}

func validateConditionNode(definition Definition, condition Condition, depth int, nodes *int) error {
	if depth > maxConditionDepth || *nodes >= maxConditionNodes {
		return ErrLimit
	}
	*nodes++
	switch condition.Op {
	case "all", "any", "not":
		if !logicFieldsEmpty(condition) {
			return ErrInvalid
		}
		if condition.Op == "not" && len(condition.Children) != 1 ||
			condition.Op != "not" && (len(condition.Children) < 1 || len(condition.Children) > 32) {
			return ErrInvalid
		}
		for _, child := range condition.Children {
			if err := validateConditionNode(definition, child, depth+1, nodes); err != nil {
				return err
			}
		}
		return nil
	case "equals", "in", "between", "selected":
		// Leaf operator; validated below.
	default:
		return ErrInvalid
	}
	if len(condition.Children) != 0 {
		return ErrInvalid
	}
	if err := validateLeafFields(definition, condition); err != nil {
		return err
	}
	domain, err := conditionDomain(definition, condition)
	if err != nil {
		return err
	}
	return conditionOverlapsDomain(definition, condition, domain)
}

func logicFieldsEmpty(c Condition) bool {
	return c.Field == "" && c.Target == "" && c.Position == nil && c.AttributeGroup == "" && c.AttributeValue == "" && c.SelectionKey == "" &&
		c.Value == nil && c.Values == nil && c.Min == nil && c.Max == nil
}

func validateLeafFields(definition Definition, c Condition) error {
	comparisonDetails := 0
	switch c.Op {
	case "equals":
		if c.Value == nil || c.Values != nil || c.Min != nil || c.Max != nil || c.SelectionKey != "" {
			return ErrInvalid
		}
		comparisonDetails = 1
		if !validDSLNumber(*c.Value) {
			return ErrInvalid
		}
	case "in":
		if c.Value != nil || c.Min != nil || c.Max != nil || c.SelectionKey != "" || len(c.Values) < 1 || len(c.Values) > 100 {
			return ErrInvalid
		}
		comparisonDetails = 1
		seen := make(map[int]struct{}, len(c.Values))
		for _, value := range c.Values {
			if !validDSLNumber(value) {
				return ErrInvalid
			}
			if _, exists := seen[value]; exists {
				return ErrInvalid
			}
			seen[value] = struct{}{}
		}
	case "between":
		if c.Value != nil || c.Values != nil || c.Min == nil || c.Max == nil || c.SelectionKey != "" || *c.Min > *c.Max ||
			!validDSLNumber(*c.Min) || !validDSLNumber(*c.Max) {
			return ErrInvalid
		}
		comparisonDetails = 1
	case "selected":
		if c.Value != nil || c.Values != nil || c.Min != nil || c.Max != nil || c.SelectionKey == "" || definition.Selection.Mode != "features" {
			return ErrInvalid
		}
		comparisonDetails = 1
		choices, ok := definition.Selection.FeatureChoices[c.SelectionKey]
		if !ok || len(choices) < 1 || len(choices) > 100 {
			return ErrInvalid
		}
		seen := make(map[int]struct{}, len(choices))
		for _, value := range choices {
			if !validDSLNumber(value) {
				return ErrInvalid
			}
			if _, exists := seen[value]; exists {
				return ErrInvalid
			}
			seen[value] = struct{}{}
		}
	}
	if comparisonDetails != 1 {
		return ErrInvalid
	}
	if c.Field == "" {
		return ErrInvalid
	}
	if !conditionField(c.Field) {
		return ErrInvalid
	}
	if c.Field == "draw_digit" || c.Field == "draw_parity" {
		if c.Position == nil || c.Target != "digits" || !isDigitModel(definition.Model) || *c.Position < 0 || *c.Position >= definition.Model.Length {
			return ErrInvalid
		}
	} else if c.Position != nil {
		return ErrInvalid
	}
	if c.Field == "attribute_match" {
		if c.AttributeGroup == "" || c.AttributeValue == "" {
			return ErrInvalid
		}
		labels, ok := definition.NumberAttributes[c.AttributeGroup]
		if !ok || len(labels) == 0 {
			return ErrInvalid
		}
		if c.AttributeValue != "$selection" {
			if strings.HasPrefix(c.AttributeValue, "$") {
				return ErrInvalid
			}
			if _, ok := labels[c.AttributeValue]; !ok {
				return ErrInvalid
			}
		} else if definition.Selection.Mode != "attributes" || !containsString(definition.Selection.AttributeGroups, c.AttributeGroup) {
			return ErrInvalid
		}
	} else if c.AttributeGroup != "" || c.AttributeValue != "" {
		return ErrInvalid
	}
	if c.Op != "selected" && c.SelectionKey != "" {
		return ErrInvalid
	}
	switch c.Field {
	case "draw_sum", "draw_odd_count", "draw_even_count", "draw_unique_count", "draw_all_same", "draw_first_last_same", "draw_span", "draw_consecutive", "attribute_match":
		if !validTarget(definition.Model, c.Target) {
			return ErrInvalid
		}
	case "draw_digit", "draw_parity":
		// Target and position were validated above.
	case "regular_match", "special_match", "position_match":
		if c.Target != "" {
			return ErrInvalid
		}
		if definition.Selection.Mode != "numbers" || c.Field == "position_match" && !isDigitModel(definition.Model) || c.Field != "position_match" && isDigitModel(definition.Model) {
			return ErrInvalid
		}
	case "excluded_match":
		if !validTarget(definition.Model, c.Target) || definition.Selection.Mode != "exclude" {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

func conditionField(field string) bool {
	switch field {
	case "regular_match", "special_match", "position_match", "excluded_match", "draw_sum", "draw_odd_count", "draw_even_count", "draw_unique_count", "draw_all_same", "draw_first_last_same", "draw_span", "draw_consecutive", "draw_digit", "draw_parity", "attribute_match":
		return true
	default:
		return false
	}
}

func isDigitModel(model Model) bool { return model.Type == "DIGITS_0_9" }

func validTarget(model Model, target string) bool {
	switch target {
	case "all":
		return true
	case "regular", "special":
		return !isDigitModel(model)
	case "digits":
		return isDigitModel(model) && model.Length > 0
	default:
		return false
	}
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func conditionDomain(definition Definition, c Condition) (intDomain, error) {
	model := definition.Model
	selection := definition.Selection
	switch c.Field {
	case "regular_match":
		return intDomain{min: 0, max: minInt(selection.RegularCount, model.RegularCount)}, nil
	case "special_match":
		return intDomain{min: 0, max: minInt(selection.SpecialCount, model.SpecialCount)}, nil
	case "position_match":
		if !isDigitModel(model) || model.Length < 1 {
			return intDomain{}, ErrInvalid
		}
		return intDomain{min: 0, max: model.Length}, nil
	case "excluded_match":
		count, err := targetCount(model, c.Target)
		if err != nil {
			return intDomain{}, err
		}
		return intDomain{min: 0, max: count}, nil
	case "draw_sum":
		return sumDomain(model, c.Target)
	case "draw_odd_count", "draw_even_count":
		count, err := targetCount(model, c.Target)
		return intDomain{min: 0, max: count}, err
	case "draw_unique_count":
		count, err := targetCount(model, c.Target)
		if count < 1 || err != nil {
			return intDomain{}, ErrInvalid
		}
		return intDomain{min: 1, max: count}, nil
	case "draw_all_same":
		count, err := targetCount(model, c.Target)
		if err != nil || count < 1 {
			return intDomain{}, ErrInvalid
		}
		return intDomain{min: 0, max: 1}, nil
	case "draw_first_last_same":
		count, err := targetCount(model, c.Target)
		if err != nil || count < 2 {
			return intDomain{}, ErrInvalid
		}
		return intDomain{min: 0, max: 1}, nil
	case "draw_span":
		count, err := targetCount(model, c.Target)
		if err != nil || count < 1 {
			return intDomain{}, ErrInvalid
		}
		_, maximum, err := targetNumberDomain(model, c.Target)
		if err != nil {
			return intDomain{}, err
		}
		minimum, _, err := targetNumberDomain(model, c.Target)
		if err != nil {
			return intDomain{}, err
		}
		return intDomain{min: 0, max: maximum - minimum}, nil
	case "draw_consecutive":
		count, err := targetCount(model, c.Target)
		if err != nil || count < 1 {
			return intDomain{}, ErrInvalid
		}
		return intDomain{min: 0, max: count - 1}, nil
	case "draw_digit":
		return intDomain{min: 0, max: 9}, nil
	case "draw_parity":
		return intDomain{min: 0, max: 1}, nil
	case "attribute_match":
		count, err := targetCount(model, c.Target)
		return intDomain{min: 0, max: count}, err
	default:
		return intDomain{}, ErrInvalid
	}
}

func conditionOverlapsDomain(definition Definition, c Condition, domain intDomain) error {
	possible := func(value int) bool { return value >= domain.min && value <= domain.max }
	switch c.Op {
	case "equals":
		if !possible(*c.Value) {
			return ErrInvalid
		}
	case "in":
		for _, value := range c.Values {
			if possible(value) {
				return nil
			}
		}
		return ErrInvalid
	case "between":
		if *c.Max < domain.min || *c.Min > domain.max {
			return ErrInvalid
		}
	case "selected":
		for _, value := range definition.Selection.FeatureChoices[c.SelectionKey] {
			if possible(value) {
				return nil
			}
		}
		return ErrInvalid
	default:
		return ErrInvalid
	}
	return nil
}

func targetCount(model Model, target string) (int, error) {
	if !validTarget(model, target) {
		return 0, ErrInvalid
	}
	switch target {
	case "regular":
		return model.RegularCount, nil
	case "special":
		return model.SpecialCount, nil
	case "digits":
		return model.Length, nil
	case "all":
		if isDigitModel(model) {
			return model.Length, nil
		}
		if model.RegularCount > maxDSLItems-model.SpecialCount {
			return 0, ErrLimit
		}
		return model.RegularCount + model.SpecialCount, nil
	default:
		return 0, ErrInvalid
	}
}

func poolNumberDomain(pool Pool) (int, int, error) {
	if len(pool.Values) > maxDSLItems {
		return 0, 0, ErrLimit
	}
	if len(pool.Values) > 0 {
		minimum, maximum := pool.Values[0], pool.Values[0]
		for _, value := range pool.Values {
			if !validPoolNumber(value) {
				return 0, 0, ErrInvalid
			}
			if value < minimum {
				minimum = value
			}
			if value > maximum {
				maximum = value
			}
		}
		return minimum, maximum, nil
	}
	if !validPoolNumber(pool.Min) || !validPoolNumber(pool.Max) || pool.Min > pool.Max {
		return 0, 0, ErrInvalid
	}
	return pool.Min, pool.Max, nil
}

func targetNumberDomain(model Model, target string) (int, int, error) {
	if !validTarget(model, target) {
		return 0, 0, ErrInvalid
	}
	if isDigitModel(model) {
		return 0, 9, nil
	}
	switch target {
	case "regular":
		return poolNumberDomain(model.RegularPool)
	case "special":
		return poolNumberDomain(model.SpecialPool)
	case "all":
		rmin, rmax, err := poolNumberDomain(model.RegularPool)
		if err != nil {
			return 0, 0, err
		}
		smin, smax, err := poolNumberDomain(model.SpecialPool)
		if err != nil {
			return 0, 0, err
		}
		if model.RegularCount == 0 {
			return smin, smax, nil
		}
		if model.SpecialCount == 0 {
			return rmin, rmax, nil
		}
		return minInt(rmin, smin), maxInt(rmax, smax), nil
	default:
		return 0, 0, ErrInvalid
	}
}

func sumDomain(model Model, target string) (intDomain, error) {
	count, err := targetCount(model, target)
	if err != nil || count < 1 {
		return intDomain{}, ErrInvalid
	}
	if isDigitModel(model) {
		maximum := int64(count) * 9
		if maximum > maxDrawSum {
			return intDomain{}, ErrLimit
		}
		return intDomain{min: 0, max: int(maximum)}, nil
	}
	var minimumSum, maximumSum int64
	addPool := func(pool Pool, poolCount int) error {
		if poolCount == 0 {
			return nil
		}
		minimum, maximum, e := poolNumberDomain(pool)
		if e != nil {
			return e
		}
		minimumSum += int64(minimum) * int64(poolCount)
		maximumSum += int64(maximum) * int64(poolCount)
		if maximumSum > maxDrawSum {
			return ErrLimit
		}
		return nil
	}
	if target == "all" {
		if err = addPool(model.RegularPool, model.RegularCount); err == nil {
			err = addPool(model.SpecialPool, model.SpecialCount)
		}
	} else if target == "regular" {
		err = addPool(model.RegularPool, model.RegularCount)
	} else {
		err = addPool(model.SpecialPool, model.SpecialCount)
	}
	if err != nil {
		return intDomain{}, err
	}
	return intDomain{min: int(minimumSum), max: int(maximumSum)}, nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func Evaluate(definition Definition, condition Condition, selection Selection, draw Draw) (Trace, error) {
	if err := ValidateCondition(definition, condition); err != nil {
		return Trace{}, err
	}
	if err := validateEvaluationInputs(definition, selection, draw); err != nil {
		return Trace{}, err
	}
	return evaluateNode(definition, condition, selection, draw)
}

func evaluateNode(definition Definition, condition Condition, selection Selection, draw Draw) (Trace, error) {
	trace := Trace{Op: condition.Op, Field: condition.Field}
	switch condition.Op {
	case "all", "any", "not":
		trace.Children = make([]Trace, 0, len(condition.Children))
		matchedCount := 0
		for _, child := range condition.Children {
			childTrace, err := evaluateNode(definition, child, selection, draw)
			if err != nil {
				return Trace{}, err
			}
			trace.Children = append(trace.Children, childTrace)
			if childTrace.Matched {
				matchedCount++
			}
		}
		trace.Actual = matchedCount
		switch condition.Op {
		case "all":
			trace.Matched = matchedCount == len(trace.Children)
		case "any":
			trace.Matched = matchedCount > 0
		case "not":
			trace.Actual = 0
			if trace.Children[0].Matched {
				trace.Actual = 1
			}
			trace.Matched = !trace.Children[0].Matched
		}
		return trace, nil
	default:
		actual, err := fieldValue(definition, condition, selection, draw)
		if err != nil {
			return Trace{}, err
		}
		trace.Actual = actual
		trace.Matched, err = compareField(definition, condition, selection, actual)
		return trace, err
	}
}

func compareField(definition Definition, condition Condition, selection Selection, actual int) (bool, error) {
	switch condition.Op {
	case "equals":
		return actual == *condition.Value, nil
	case "in":
		for _, value := range condition.Values {
			if actual == value {
				return true, nil
			}
		}
		return false, nil
	case "between":
		return actual >= *condition.Min && actual <= *condition.Max, nil
	case "selected":
		selected, err := selectionFeature(definition, selection, condition.SelectionKey)
		if err != nil {
			return false, err
		}
		return actual == selected, nil
	default:
		return false, ErrInvalid
	}
}

func selectionFeature(definition Definition, selection Selection, key string) (int, error) {
	choices, ok := definition.Selection.FeatureChoices[key]
	if !ok || len(choices) == 0 {
		return 0, ErrInvalid
	}
	values, ok := selection.Features[key]
	if !ok || len(values) != 1 {
		return 0, ErrInvalid
	}
	for _, choice := range choices {
		if values[0] == choice {
			return values[0], nil
		}
	}
	return 0, ErrInvalid
}

func fieldValue(definition Definition, condition Condition, selection Selection, draw Draw) (int, error) {
	switch condition.Field {
	case "regular_match":
		if len(selection.Regular) != definition.Selection.RegularCount || len(draw.Regular) != definition.Model.RegularCount {
			return 0, ErrInvalid
		}
		return multisetIntersectionCount(selection.Regular, draw.Regular), nil
	case "special_match":
		if len(selection.Special) != definition.Selection.SpecialCount || len(draw.Special) != definition.Model.SpecialCount {
			return 0, ErrInvalid
		}
		return multisetIntersectionCount(selection.Special, draw.Special), nil
	case "position_match":
		if len(selection.Digits) != definition.Model.Length || len(draw.Digits) != definition.Model.Length || definition.Model.Length < 1 {
			return 0, ErrInvalid
		}
		matches := 0
		for position, candidates := range selection.Digits {
			if len(candidates) == 0 {
				return 0, ErrInvalid
			}
			if containsInt(candidates, draw.Digits[position]) {
				matches++
			}
		}
		return matches, nil
	case "excluded_match":
		if len(selection.Exclude) != definition.Selection.ExcludeCount {
			return 0, ErrInvalid
		}
		target, err := targetDraw(definition.Model, condition.Target, draw)
		if err != nil {
			return 0, err
		}
		return excludedHitCount(selection.Exclude, target), nil
	case "draw_sum", "draw_odd_count", "draw_even_count", "draw_unique_count", "draw_all_same", "draw_first_last_same", "draw_span", "draw_consecutive":
		target, err := targetDraw(definition.Model, condition.Target, draw)
		if err != nil || len(target) == 0 {
			return 0, ErrInvalid
		}
		switch condition.Field {
		case "draw_sum":
			sum := int64(0)
			for _, value := range target {
				sum += int64(value)
				if sum > maxDrawSum {
					return 0, ErrLimit
				}
			}
			return int(sum), nil
		case "draw_odd_count":
			count := 0
			for _, value := range target {
				if value%2 == 1 {
					count++
				}
			}
			return count, nil
		case "draw_even_count":
			count := 0
			for _, value := range target {
				if value%2 == 0 {
					count++
				}
			}
			return count, nil
		case "draw_unique_count":
			return len(uniqueInts(target)), nil
		case "draw_all_same":
			for _, value := range target[1:] {
				if value != target[0] {
					return 0, nil
				}
			}
			return 1, nil
		case "draw_first_last_same":
			if target[0] == target[len(target)-1] {
				return 1, nil
			}
			return 0, nil
		case "draw_span":
			minimum, maximum := target[0], target[0]
			for _, value := range target[1:] {
				minimum = minInt(minimum, value)
				maximum = maxInt(maximum, value)
			}
			return maximum - minimum, nil
		case "draw_consecutive":
			sorted := uniqueInts(target)
			sort.Ints(sorted)
			consecutive := 0
			for i := 1; i < len(sorted); i++ {
				if sorted[i] == sorted[i-1]+1 {
					consecutive++
				}
			}
			return consecutive, nil
		}
	case "draw_digit":
		if !isDigitModel(definition.Model) || len(draw.Digits) != definition.Model.Length || condition.Position == nil || *condition.Position >= len(draw.Digits) {
			return 0, ErrInvalid
		}
		return draw.Digits[*condition.Position], nil
	case "draw_parity":
		if !isDigitModel(definition.Model) || len(draw.Digits) != definition.Model.Length || condition.Position == nil || *condition.Position >= len(draw.Digits) {
			return 0, ErrInvalid
		}
		return draw.Digits[*condition.Position] % 2, nil
	case "attribute_match":
		target, err := targetDraw(definition.Model, condition.Target, draw)
		if err != nil {
			return 0, err
		}
		label := condition.AttributeValue
		if label == "$selection" {
			values, ok := selection.Attributes[condition.AttributeGroup]
			if !ok || len(values) != 1 {
				return 0, ErrInvalid
			}
			label = values[0]
		}
		numbers, ok := definition.NumberAttributes[condition.AttributeGroup][label]
		if !ok {
			return 0, ErrInvalid
		}
		matching := make(map[int]struct{}, len(numbers))
		for _, number := range numbers {
			if !validPoolNumber(number) {
				return 0, ErrInvalid
			}
			matching[number] = struct{}{}
		}
		matched := make(map[int]struct{})
		for _, number := range target {
			if _, ok := matching[number]; ok {
				matched[number] = struct{}{}
			}
		}
		return len(matched), nil
	default:
		return 0, ErrInvalid
	}
	return 0, ErrInvalid
}

func targetDraw(model Model, target string, draw Draw) ([]int, error) {
	if !validTarget(model, target) {
		return nil, ErrInvalid
	}
	switch target {
	case "regular":
		if len(draw.Regular) != model.RegularCount {
			return nil, ErrInvalid
		}
		return draw.Regular, nil
	case "special":
		if len(draw.Special) != model.SpecialCount {
			return nil, ErrInvalid
		}
		return draw.Special, nil
	case "digits":
		if len(draw.Digits) != model.Length {
			return nil, ErrInvalid
		}
		return draw.Digits, nil
	case "all":
		if isDigitModel(model) {
			if len(draw.Digits) != model.Length {
				return nil, ErrInvalid
			}
			return draw.Digits, nil
		}
		if len(draw.Regular) != model.RegularCount || len(draw.Special) != model.SpecialCount {
			return nil, ErrInvalid
		}
		if len(draw.Regular) > maxDSLItems-len(draw.Special) {
			return nil, ErrLimit
		}
		all := make([]int, 0, len(draw.Regular)+len(draw.Special))
		all = append(all, draw.Regular...)
		all = append(all, draw.Special...)
		return all, nil
	default:
		return nil, ErrInvalid
	}
}

func multisetIntersectionCount(left, right []int) int {
	counts := make(map[int]int, len(left))
	for _, value := range left {
		counts[value]++
	}
	matched := 0
	for _, value := range right {
		if counts[value] > 0 {
			counts[value]--
			matched++
		}
	}
	return matched
}

func excludedHitCount(excluded, target []int) int {
	set := make(map[int]struct{}, len(excluded))
	for _, value := range excluded {
		set[value] = struct{}{}
	}
	hits := 0
	for _, value := range target {
		if _, ok := set[value]; ok {
			hits++
		}
	}
	return hits
}

func containsInt(values []int, wanted int) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func uniqueInts(values []int) []int {
	seen := make(map[int]struct{}, len(values))
	unique := make([]int, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	return unique
}

func validateEvaluationInputs(definition Definition, selection Selection, draw Draw) error {
	if len(selection.Regular) > maxDSLItems || len(selection.Special) > maxDSLItems || len(selection.Exclude) > maxDSLItems ||
		len(draw.Regular) > maxDSLItems || len(draw.Special) > maxDSLItems || len(draw.Digits) > maxDSLItems ||
		len(selection.Digits) > maxDSLItems || len(selection.Attributes) > maxConditionNodes || len(selection.Features) > maxConditionNodes {
		return ErrLimit
	}
	checkNumbers := func(values []int, digit bool) bool {
		for _, value := range values {
			if !validPoolNumber(value) || digit && value > 9 {
				return false
			}
		}
		return true
	}
	if !checkNumbers(selection.Regular, false) || !checkNumbers(selection.Special, false) || !checkNumbers(selection.Exclude, false) ||
		!checkNumbers(draw.Regular, false) || !checkNumbers(draw.Special, false) || !checkNumbers(draw.Digits, true) {
		return ErrInvalid
	}
	for _, candidates := range selection.Digits {
		if len(candidates) > maxDSLItems {
			return ErrLimit
		}
		if !checkNumbers(candidates, true) {
			return ErrInvalid
		}
	}
	for group, values := range selection.Attributes {
		if len(group) == 0 || len(values) > 100 {
			return ErrInvalid
		}
	}
	for key, values := range selection.Features {
		if key == "" || len(values) > 100 || !checkNumbers(values, false) {
			return ErrInvalid
		}
	}
	if err := validateRuntimeDraw(definition.Model, draw); err != nil {
		return err
	}
	return validateRuntimeSelection(definition, selection)
}

func validateRuntimeSelection(definition Definition, selection Selection) error {
	m := definition.Model
	switch definition.Selection.Mode {
	case "numbers":
		if len(selection.Exclude) != 0 || len(selection.Attributes) != 0 || len(selection.Features) != 0 || definition.Selection.ExcludeCount != 0 || len(definition.Selection.AttributeGroups) != 0 || len(definition.Selection.FeatureChoices) != 0 {
			return ErrInvalid
		}
		if isDigitModel(m) {
			if len(selection.Digits) != m.Length || len(selection.Regular) != 0 || len(selection.Special) != 0 || definition.Selection.RegularCount != 0 || definition.Selection.SpecialCount != 0 {
				return ErrInvalid
			}
			for _, candidates := range selection.Digits {
				if len(candidates) == 0 {
					return ErrInvalid
				}
			}
			return nil
		}
		if len(selection.Regular) != definition.Selection.RegularCount || len(selection.Special) != definition.Selection.SpecialCount || len(selection.Digits) != 0 {
			return ErrInvalid
		}
		if err := valuesInPool(selection.Regular, m.RegularPool); err != nil {
			return err
		}
		if err := valuesInPool(selection.Special, m.SpecialPool); err != nil {
			return err
		}
		if !m.RegularPool.AllowRepeat && hasDuplicates(selection.Regular) || !m.SpecialPool.AllowRepeat && hasDuplicates(selection.Special) {
			return ErrInvalid
		}
		if m.Type == "M_SELECT_N" && intersects(selection.Regular, selection.Special) {
			return ErrInvalid
		}
	case "exclude":
		if len(selection.Regular) != 0 || len(selection.Special) != 0 || len(selection.Digits) != 0 || len(selection.Attributes) != 0 || len(selection.Features) != 0 || len(selection.Exclude) != definition.Selection.ExcludeCount || definition.Selection.RegularCount != 0 || definition.Selection.SpecialCount != 0 || len(definition.Selection.AttributeGroups) != 0 || len(definition.Selection.FeatureChoices) != 0 {
			return ErrInvalid
		}
	case "attributes":
		if len(selection.Regular) != 0 || len(selection.Special) != 0 || len(selection.Digits) != 0 || len(selection.Exclude) != 0 || len(selection.Features) != 0 || definition.Selection.RegularCount != 0 || definition.Selection.SpecialCount != 0 || definition.Selection.ExcludeCount != 0 || len(definition.Selection.FeatureChoices) != 0 || len(selection.Attributes) != len(definition.Selection.AttributeGroups) {
			return ErrInvalid
		}
		for _, group := range definition.Selection.AttributeGroups {
			values, ok := selection.Attributes[group]
			if !ok || len(values) != 1 {
				return ErrInvalid
			}
			if _, ok := definition.NumberAttributes[group][values[0]]; !ok {
				return ErrInvalid
			}
		}
	case "features":
		if len(selection.Regular) != 0 || len(selection.Special) != 0 || len(selection.Digits) != 0 || len(selection.Exclude) != 0 || len(selection.Attributes) != 0 || definition.Selection.RegularCount != 0 || definition.Selection.SpecialCount != 0 || definition.Selection.ExcludeCount != 0 || len(definition.Selection.AttributeGroups) != 0 || len(selection.Features) != len(definition.Selection.FeatureChoices) {
			return ErrInvalid
		}
		for key, choices := range definition.Selection.FeatureChoices {
			values, ok := selection.Features[key]
			if !ok || len(values) != 1 || !containsInt(choices, values[0]) {
				return ErrInvalid
			}
		}
	default:
		return ErrInvalid
	}
	return nil
}

func validateRuntimeDraw(model Model, draw Draw) error {
	switch model.Type {
	case "DIGITS_0_9":
		if len(draw.Digits) != model.Length || len(draw.Regular) != 0 || len(draw.Special) != 0 {
			return ErrInvalid
		}
		if !model.AllowRepeat && hasDuplicates(draw.Digits) {
			return ErrInvalid
		}
	case "X_PLUS_Y", "M_SELECT_N":
		if len(draw.Regular) != model.RegularCount || len(draw.Special) != model.SpecialCount || len(draw.Digits) != 0 {
			return ErrInvalid
		}
		if err := valuesInPool(draw.Regular, model.RegularPool); err != nil {
			return err
		}
		if err := valuesInPool(draw.Special, model.SpecialPool); err != nil {
			return err
		}
		if !model.RegularPool.AllowRepeat && hasDuplicates(draw.Regular) || !model.SpecialPool.AllowRepeat && hasDuplicates(draw.Special) {
			return ErrInvalid
		}
		if model.Type == "M_SELECT_N" && intersects(draw.Regular, draw.Special) {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

func valuesInPool(values []int, pool Pool) error {
	minimum, maximum, err := poolNumberDomain(pool)
	if err != nil {
		return err
	}
	for _, value := range values {
		if value < minimum || value > maximum || len(pool.Values) > 0 && !containsInt(pool.Values, value) {
			return ErrInvalid
		}
	}
	return nil
}

func hasDuplicates(values []int) bool {
	seen := make(map[int]struct{}, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			return true
		}
		seen[value] = struct{}{}
	}
	return false
}
