package rules

import (
	"math"
	"sort"
)

const (
	selectionNumberLimit = 100
	numberValueMax       = 1_000_000
	featureValueMax      = 20_000_000
	poolCardinalityMax   = 10_000
	selectionWorkLimit   = 1_000_000
)

func ValidateModel(model Model) error {
	switch model.Type {
	case "X_PLUS_Y":
		if !validCount(model.RegularCount) || !validCount(model.SpecialCount) || model.RegularCount+model.SpecialCount == 0 || model.PoolSize != 0 || model.TotalCount != 0 || model.Length != 0 || model.AllowRepeat {
			return ErrInvalid
		}
		regularSize, err := poolCardinality(model.RegularPool)
		if err != nil {
			return err
		}
		specialSize, err := poolCardinality(model.SpecialPool)
		if err != nil {
			return err
		}
		if model.RegularCount > 0 && regularSize == 0 || model.SpecialCount > 0 && specialSize == 0 {
			return ErrInvalid
		}
		if !model.RegularPool.AllowRepeat && model.RegularCount > regularSize || !model.SpecialPool.AllowRepeat && model.SpecialCount > specialSize {
			return ErrInvalid
		}
		return nil
	case "M_SELECT_N":
		if model.PoolSize < 1 || model.PoolSize > poolCardinalityMax || model.TotalCount < 1 || model.TotalCount >= model.PoolSize || !validCount(model.RegularCount) || !validCount(model.SpecialCount) || model.TotalCount != model.RegularCount+model.SpecialCount || model.Length != 0 || model.AllowRepeat {
			return ErrInvalid
		}
		regularSize, err := poolCardinality(model.RegularPool)
		if err != nil {
			return err
		}
		specialSize, err := poolCardinality(model.SpecialPool)
		if err != nil {
			return err
		}
		if regularSize != model.PoolSize || specialSize != model.PoolSize || model.RegularPool.AllowRepeat || model.SpecialPool.AllowRepeat {
			return ErrInvalid
		}
		regularValues, err := poolValues(model.RegularPool)
		if err != nil {
			return err
		}
		specialValues, err := poolValues(model.SpecialPool)
		if err != nil {
			return err
		}
		if !sameIntSet(regularValues, specialValues) {
			return ErrInvalid
		}
		return nil
	case "DIGITS_0_9":
		if model.Length < 1 || model.Length > 10 || !model.Ordered || model.RegularCount != 0 || model.SpecialCount != 0 || model.PoolSize != 0 || model.TotalCount != 0 || !emptyPool(model.RegularPool) || !emptyPool(model.SpecialPool) {
			return ErrInvalid
		}
		return nil
	default:
		return ErrInvalid
	}
}

func ValidateDraw(model Model, draw Draw) error {
	if err := ValidateModel(model); err != nil {
		return err
	}
	switch model.Type {
	case "X_PLUS_Y", "M_SELECT_N":
		if len(draw.Regular) != model.RegularCount || len(draw.Special) != model.SpecialCount || len(draw.Digits) != 0 {
			return ErrInvalid
		}
		regular, err := poolValues(model.RegularPool)
		if err != nil || !drawValuesValid(draw.Regular, regular, model.RegularPool.AllowRepeat) {
			return ErrInvalid
		}
		special, err := poolValues(model.SpecialPool)
		if err != nil || !drawValuesValid(draw.Special, special, model.SpecialPool.AllowRepeat) {
			return ErrInvalid
		}
		if model.Type == "M_SELECT_N" && intersects(draw.Regular, draw.Special) {
			return ErrInvalid
		}
		return nil
	case "DIGITS_0_9":
		if len(draw.Digits) != model.Length || len(draw.Regular) != 0 || len(draw.Special) != 0 {
			return ErrInvalid
		}
		seen := [10]bool{}
		for _, digit := range draw.Digits {
			if digit < 0 || digit > 9 || !model.AllowRepeat && seen[digit] {
				return ErrInvalid
			}
			seen[digit] = true
		}
		return nil
	default:
		return ErrInvalid
	}
}

func NormalizeSelection(definition Definition, selection Selection) (Selection, error) {
	model := definition.Model
	if err := ValidateModel(model); err != nil {
		return Selection{}, err
	}
	rule := definition.Selection
	result := cloneSelection(selection)
	switch rule.Mode {
	case "numbers":
		if rule.ExcludeCount != 0 || len(rule.AttributeGroups) != 0 || len(rule.FeatureChoices) != 0 || len(result.Exclude) != 0 || len(result.Attributes) != 0 || len(result.Features) != 0 {
			return Selection{}, ErrInvalid
		}
		if model.Type == "DIGITS_0_9" {
			if rule.RegularCount != 0 || rule.SpecialCount != 0 || len(result.Regular) != 0 || len(result.Special) != 0 || len(result.Digits) != model.Length {
				return Selection{}, ErrInvalid
			}
			for i := range result.Digits {
				values, err := normalizeDigits(result.Digits[i])
				if err != nil {
					return Selection{}, err
				}
				result.Digits[i] = values
			}
			if !model.AllowRepeat && !hasDistinctDigitsPossible(result.Digits) {
				return Selection{}, ErrInvalid
			}
			return result, nil
		}
		if len(result.Digits) != 0 {
			return Selection{}, ErrInvalid
		}
		if !validCount(rule.RegularCount) || !validCount(rule.SpecialCount) || rule.RegularCount+rule.SpecialCount == 0 || rule.RegularCount > model.RegularCount || rule.SpecialCount > model.SpecialCount {
			return Selection{}, ErrInvalid
		}
		regularValues, err := poolValues(model.RegularPool)
		if err != nil {
			return Selection{}, err
		}
		specialValues, err := poolValues(model.SpecialPool)
		if err != nil {
			return Selection{}, err
		}
		result.Regular, err = normalizeCandidates(result.Regular, rule.RegularCount, model.RegularPool, regularValues, model.Ordered)
		if err != nil {
			return Selection{}, err
		}
		result.Special, err = normalizeCandidates(result.Special, rule.SpecialCount, model.SpecialPool, specialValues, model.Ordered)
		if err != nil {
			return Selection{}, err
		}
		if model.Type == "M_SELECT_N" && !hasDistinctUnionPossible(result.Regular, rule.RegularCount, result.Special, rule.SpecialCount) {
			return Selection{}, ErrInvalid
		}
		return result, nil
	case "exclude":
		if !validExcludeCount(rule.ExcludeCount) || rule.RegularCount != 0 || rule.SpecialCount != 0 || len(rule.AttributeGroups) != 0 || len(rule.FeatureChoices) != 0 || len(result.Regular) != 0 || len(result.Special) != 0 || len(result.Digits) != 0 || len(result.Attributes) != 0 || len(result.Features) != 0 || len(result.Exclude) != rule.ExcludeCount {
			return Selection{}, ErrInvalid
		}
		allowed, err := unionPoolValues(model)
		if err != nil {
			return Selection{}, err
		}
		result.Exclude, err = normalizeFixedValues(result.Exclude, rule.ExcludeCount, allowed)
		if err != nil {
			return Selection{}, err
		}
		return result, nil
	case "attributes":
		if rule.RegularCount != 0 || rule.SpecialCount != 0 || rule.ExcludeCount != 0 || len(rule.AttributeGroups) == 0 || len(rule.AttributeGroups) > selectionNumberLimit || len(rule.FeatureChoices) != 0 || len(result.Regular) != 0 || len(result.Special) != 0 || len(result.Digits) != 0 || len(result.Exclude) != 0 || len(result.Features) != 0 {
			return Selection{}, ErrInvalid
		}
		groups, err := normalizeAttributes(rule, definition.NumberAttributes, result.Attributes)
		if err != nil {
			return Selection{}, err
		}
		result.Attributes = groups
		return result, nil
	case "features":
		if rule.RegularCount != 0 || rule.SpecialCount != 0 || rule.ExcludeCount != 0 || len(rule.AttributeGroups) != 0 || len(rule.FeatureChoices) == 0 || len(rule.FeatureChoices) > selectionNumberLimit || len(result.Regular) != 0 || len(result.Special) != 0 || len(result.Digits) != 0 || len(result.Exclude) != 0 || len(result.Attributes) != 0 {
			return Selection{}, ErrInvalid
		}
		features, err := normalizeFeatures(rule.FeatureChoices, result.Features)
		if err != nil {
			return Selection{}, err
		}
		result.Features = features
		return result, nil
	default:
		return Selection{}, ErrInvalid
	}
}

func Expand(definition Definition, selection Selection) ([]Selection, error) {
	if definition.Limits.MaxCombinations < 1 || definition.Limits.MaxCombinations > MaxCombinations {
		return nil, ErrInvalid
	}
	normalized, err := NormalizeSelection(definition, selection)
	if err != nil {
		return nil, err
	}
	limit := definition.Limits.MaxCombinations
	switch definition.Selection.Mode {
	case "exclude":
		return []Selection{normalized}, nil
	case "attributes":
		return expandAttributes(normalized, limit)
	case "features":
		return expandFeatures(normalized, limit)
	case "numbers":
		if definition.Model.Type == "DIGITS_0_9" {
			return expandDigits(normalized, definition.Model, limit)
		}
		return expandNumberGroups(normalized, definition, limit)
	default:
		return nil, ErrInvalid
	}
}

func validCount(value int) bool        { return value >= 0 && value <= 10 }
func validExcludeCount(value int) bool { return value >= 1 && value <= selectionNumberLimit }

func emptyPool(pool Pool) bool {
	return pool.Min == 0 && pool.Max == 0 && len(pool.Values) == 0 && !pool.AllowRepeat
}

func poolCardinality(pool Pool) (int, error) {
	if len(pool.Values) > 0 {
		if pool.Min != 0 || pool.Max != 0 || len(pool.Values) > poolCardinalityMax {
			return 0, ErrInvalid
		}
		seen := make(map[int]struct{}, len(pool.Values))
		for _, value := range pool.Values {
			if value < 0 || value > numberValueMax {
				return 0, ErrInvalid
			}
			if _, ok := seen[value]; ok {
				return 0, ErrInvalid
			}
			seen[value] = struct{}{}
		}
		return len(pool.Values), nil
	}
	if pool.Min < 0 || pool.Max < pool.Min || pool.Max > numberValueMax {
		return 0, ErrInvalid
	}
	cardinality := pool.Max - pool.Min + 1
	if cardinality < 1 || cardinality > poolCardinalityMax {
		return 0, ErrInvalid
	}
	return cardinality, nil
}

func poolValues(pool Pool) ([]int, error) {
	if _, err := poolCardinality(pool); err != nil {
		return nil, err
	}
	if len(pool.Values) > 0 {
		return append([]int(nil), pool.Values...), nil
	}
	values := make([]int, 0, pool.Max-pool.Min+1)
	for value := pool.Min; value <= pool.Max; value++ {
		values = append(values, value)
	}
	return values, nil
}

func drawValuesValid(values, pool []int, allowRepeat bool) bool {
	allowed := make(map[int]struct{}, len(pool))
	for _, value := range pool {
		allowed[value] = struct{}{}
	}
	seen := make(map[int]struct{}, len(values))
	for _, value := range values {
		if _, ok := allowed[value]; !ok {
			return false
		}
		if _, ok := seen[value]; ok && !allowRepeat {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

func intersects(a, b []int) bool {
	seen := make(map[int]struct{}, len(a))
	for _, value := range a {
		seen[value] = struct{}{}
	}
	for _, value := range b {
		if _, ok := seen[value]; ok {
			return true
		}
	}
	return false
}

func sameIntSet(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[int]struct{}, len(a))
	for _, value := range a {
		seen[value] = struct{}{}
	}
	for _, value := range b {
		if _, ok := seen[value]; !ok {
			return false
		}
	}
	return true
}

func cloneSelection(selection Selection) Selection {
	result := Selection{
		Regular: append([]int(nil), selection.Regular...), Special: append([]int(nil), selection.Special...),
		Exclude: append([]int(nil), selection.Exclude...),
	}
	if selection.Digits != nil {
		result.Digits = make([][]int, len(selection.Digits))
		for i := range selection.Digits {
			result.Digits[i] = append([]int(nil), selection.Digits[i]...)
		}
	}
	if selection.Attributes != nil {
		result.Attributes = make(map[string][]string, len(selection.Attributes))
		for key, values := range selection.Attributes {
			result.Attributes[key] = append([]string(nil), values...)
		}
	}
	if selection.Features != nil {
		result.Features = make(map[string][]int, len(selection.Features))
		for key, values := range selection.Features {
			result.Features[key] = append([]int(nil), values...)
		}
	}
	return result
}

func normalizeCandidates(values []int, count int, pool Pool, allowedValues []int, ordered bool) ([]int, error) {
	if count == 0 {
		if len(values) != 0 {
			return nil, ErrInvalid
		}
		return []int{}, nil
	}
	if len(values) == 0 || len(values) > selectionNumberLimit {
		return nil, ErrInvalid
	}
	allowed := make(map[int]struct{}, len(allowedValues))
	for _, value := range allowedValues {
		allowed[value] = struct{}{}
	}
	seen := make(map[int]struct{}, len(values))
	result := make([]int, 0, len(values))
	for _, value := range values {
		if _, ok := allowed[value]; !ok {
			return nil, ErrInvalid
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	if !pool.AllowRepeat && len(result) < count {
		return nil, ErrInvalid
	}
	if !ordered {
		sort.Ints(result)
	}
	return result, nil
}

func normalizeDigits(values []int) ([]int, error) {
	if len(values) == 0 || len(values) > 10 {
		return nil, ErrInvalid
	}
	seen := [10]bool{}
	result := make([]int, 0, len(values))
	for _, value := range values {
		if value < 0 || value > 9 {
			return nil, ErrInvalid
		}
		if seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Ints(result)
	return result, nil
}

func normalizeFixedValues(values []int, count int, allowed []int) ([]int, error) {
	if len(values) != count || len(values) > selectionNumberLimit {
		return nil, ErrInvalid
	}
	pool := make(map[int]struct{}, len(allowed))
	for _, value := range allowed {
		pool[value] = struct{}{}
	}
	seen := make(map[int]struct{}, len(values))
	result := append([]int(nil), values...)
	for _, value := range result {
		if _, ok := pool[value]; !ok {
			return nil, ErrInvalid
		}
		if _, ok := seen[value]; ok {
			return nil, ErrInvalid
		}
		seen[value] = struct{}{}
	}
	sort.Ints(result)
	return result, nil
}

func unionPoolValues(model Model) ([]int, error) {
	if model.Type == "DIGITS_0_9" {
		values := make([]int, 10)
		for i := range values {
			values[i] = i
		}
		return values, nil
	}
	regular, err := poolValues(model.RegularPool)
	if err != nil {
		return nil, err
	}
	special, err := poolValues(model.SpecialPool)
	if err != nil {
		return nil, err
	}
	seen := make(map[int]struct{}, len(regular)+len(special))
	values := make([]int, 0, len(regular)+len(special))
	for _, value := range append(regular, special...) {
		if _, ok := seen[value]; !ok {
			seen[value] = struct{}{}
			values = append(values, value)
		}
	}
	return values, nil
}

func normalizeAttributes(rule SelectionRule, available map[string]map[string][]int, selected map[string][]string) (map[string][]string, error) {
	groups := append([]string(nil), rule.AttributeGroups...)
	sort.Strings(groups)
	allowedGroups := make(map[string]struct{}, len(groups))
	for i, group := range groups {
		if group == "" || i > 0 && groups[i-1] == group {
			return nil, ErrInvalid
		}
		allowedGroups[group] = struct{}{}
	}
	if len(selected) != len(groups) {
		return nil, ErrInvalid
	}
	result := make(map[string][]string, len(groups))
	for _, group := range groups {
		selectedLabels, ok := selected[group]
		options, configured := available[group]
		if !ok || !configured || len(selectedLabels) == 0 || len(selectedLabels) > selectionNumberLimit {
			return nil, ErrInvalid
		}
		seen := make(map[string]struct{}, len(selectedLabels))
		labels := make([]string, 0, len(selectedLabels))
		for _, label := range selectedLabels {
			if _, ok := options[label]; !ok || label == "" {
				return nil, ErrInvalid
			}
			if _, ok := seen[label]; ok {
				continue
			}
			seen[label] = struct{}{}
			labels = append(labels, label)
		}
		sort.Strings(labels)
		result[group] = labels
	}
	for group := range selected {
		if _, ok := allowedGroups[group]; !ok {
			return nil, ErrInvalid
		}
	}
	return result, nil
}

func normalizeFeatures(allowed, selected map[string][]int) (map[string][]int, error) {
	if len(allowed) != len(selected) {
		return nil, ErrInvalid
	}
	result := make(map[string][]int, len(allowed))
	for feature, candidates := range allowed {
		values, ok := selected[feature]
		if feature == "" || !ok || len(candidates) == 0 || len(candidates) > selectionNumberLimit || len(values) == 0 || len(values) > selectionNumberLimit {
			return nil, ErrInvalid
		}
		valid := make(map[int]struct{}, len(candidates))
		for _, value := range candidates {
			if value < 0 || value > featureValueMax {
				return nil, ErrInvalid
			}
			valid[value] = struct{}{}
		}
		seen := make(map[int]struct{}, len(values))
		normalized := make([]int, 0, len(values))
		for _, value := range values {
			if _, ok := valid[value]; !ok {
				return nil, ErrInvalid
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			normalized = append(normalized, value)
		}
		sort.Ints(normalized)
		result[feature] = normalized
	}
	for feature := range selected {
		if _, ok := allowed[feature]; !ok {
			return nil, ErrInvalid
		}
	}
	return result, nil
}

func hasDistinctDigitsPossible(positions [][]int) bool {
	var owners [10]int
	for digit := range owners {
		owners[digit] = -1
	}
	for position := range positions {
		var seen [10]bool
		if !augmentDigitMatching(positions, position, &owners, &seen) {
			return false
		}
	}
	return true
}

func augmentDigitMatching(positions [][]int, position int, owners *[10]int, seen *[10]bool) bool {
	for _, digit := range positions[position] {
		if seen[digit] {
			continue
		}
		seen[digit] = true
		if owners[digit] == -1 || augmentDigitMatching(positions, owners[digit], owners, seen) {
			owners[digit] = position
			return true
		}
	}
	return false
}

func hasDistinctUnionPossible(regular []int, regularCount int, special []int, specialCount int) bool {
	if len(regular) < regularCount || len(special) < specialCount {
		return false
	}
	union := make(map[int]struct{}, len(regular)+len(special))
	for _, value := range regular {
		union[value] = struct{}{}
	}
	for _, value := range special {
		union[value] = struct{}{}
	}
	return len(union) >= regularCount+specialCount
}

func expandNumberGroups(selection Selection, definition Definition, limit int) ([]Selection, error) {
	model, rule := definition.Model, definition.Selection
	regRepeat, specRepeat := model.RegularPool.AllowRepeat, model.SpecialPool.AllowRepeat
	if model.Type == "M_SELECT_N" {
		regRepeat, specRepeat = false, false
	}
	regular, err := pickLines(selection.Regular, rule.RegularCount, regRepeat, model.Ordered, limit)
	if err != nil {
		return nil, err
	}
	special, err := pickLines(selection.Special, rule.SpecialCount, specRepeat, model.Ordered, limit)
	if err != nil {
		return nil, err
	}
	maxWork := limit * 100
	if maxWork > selectionWorkLimit {
		maxWork = selectionWorkLimit
	}
	result := make([]Selection, 0, min(limit, len(regular)*len(special)))
	work := 0
	for _, reg := range regular {
		for _, spec := range special {
			work++
			if work > maxWork {
				return nil, ErrLimit
			}
			if model.Type == "M_SELECT_N" && intersects(reg, spec) {
				continue
			}
			result = append(result, Selection{Regular: append([]int(nil), reg...), Special: append([]int(nil), spec...)})
			if len(result) > limit {
				return nil, ErrLimit
			}
		}
	}
	if len(result) == 0 {
		return nil, ErrInvalid
	}
	return result, nil
}

func pickLines(candidates []int, count int, allowRepeat, ordered bool, limit int) ([][]int, error) {
	if count == 0 {
		return [][]int{{}}, nil
	}
	if len(candidates) == 0 {
		return nil, ErrInvalid
	}
	ways := selectionWays(len(candidates), count, allowRepeat, ordered, limit)
	if ways < 0 {
		return nil, ErrLimit
	}
	if ways == 0 {
		return nil, ErrInvalid
	}
	result := make([][]int, 0, ways)
	line := make([]int, count)
	var visit func(pos, start int) error
	visit = func(pos, start int) error {
		if pos == count {
			result = append(result, append([]int(nil), line...))
			return nil
		}
		if ordered {
			for i, value := range candidates {
				if !allowRepeat && usedBefore(line, pos, value) {
					continue
				}
				line[pos] = value
				if err := visit(pos+1, 0); err != nil {
					return err
				}
				_ = i
			}
			return nil
		}
		minIndex := start
		for i := minIndex; i < len(candidates); i++ {
			if allowRepeat || pos == 0 || i > start-1 {
				line[pos] = candidates[i]
				if allowRepeat {
					if err := visit(pos+1, i); err != nil {
						return err
					}
				} else if err := visit(pos+1, i+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := visit(0, 0); err != nil {
		return nil, err
	}
	return result, nil
}

func selectionWays(n, k int, allowRepeat, ordered bool, limit int) int {
	if k == 0 {
		return 1
	}
	if !allowRepeat && k > n {
		return 0
	}
	ways := int64(1)
	if ordered {
		for i := 0; i < k; i++ {
			factor := n
			if !allowRepeat {
				factor -= i
			}
			ways = multiplyLimited(ways, int64(factor), int64(limit))
			if ways < 0 {
				return -1
			}
		}
	} else if allowRepeat {
		// Combinations with repetition: C(n+k-1,k).
		ways = combinationsLimited(n+k-1, k, int64(limit))
	} else {
		ways = combinationsLimited(n, k, int64(limit))
	}
	return int(ways)
}

func combinationsLimited(n, k int, limit int64) int64 {
	if k > n-k {
		k = n - k
	}
	value := int64(1)
	for i := 1; i <= k; i++ {
		factor := int64(n - k + i)
		// Divide before multiplying where possible to retain exactness.
		g := gcd64(factor, int64(i))
		factor /= g
		divisor := int64(i) / g
		g = gcd64(value, divisor)
		value /= g
		divisor /= g
		if divisor != 1 {
			return -1
		}
		value = multiplyLimited(value, factor, limit)
		if value < 0 {
			return -1
		}
	}
	return value
}

func multiplyLimited(a, b, limit int64) int64 {
	if b != 0 && (a > limit/b || a > math.MaxInt64/b) {
		return -1
	}
	result := a * b
	if result > limit {
		return -1
	}
	return result
}

func gcd64(a, b int64) int64 {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

func usedBefore(values []int, length, value int) bool {
	for i := 0; i < length; i++ {
		if values[i] == value {
			return true
		}
	}
	return false
}

func expandDigits(selection Selection, model Model, limit int) ([]Selection, error) {
	result := make([]Selection, 0, min(limit, 100))
	line := make([]int, model.Length)
	var visit func(int, [10]bool) error
	visit = func(position int, used [10]bool) error {
		if position == model.Length {
			digits := make([][]int, len(line))
			for i, digit := range line {
				digits[i] = []int{digit}
			}
			result = append(result, Selection{Digits: digits})
			if len(result) > limit {
				return ErrLimit
			}
			return nil
		}
		for _, value := range selection.Digits[position] {
			if !model.AllowRepeat && used[value] {
				continue
			}
			line[position] = value
			next := used
			next[value] = true
			if err := visit(position+1, next); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(0, [10]bool{}); err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, ErrInvalid
	}
	return result, nil
}

func expandAttributes(selection Selection, limit int) ([]Selection, error) {
	keys := sortedAttributeKeys(selection.Attributes)
	result := make([]Selection, 0, min(limit, 100))
	line := make(map[string][]string, len(keys))
	var visit func(int) error
	visit = func(position int) error {
		if position == len(keys) {
			attrs := make(map[string][]string, len(line))
			for key, labels := range line {
				attrs[key] = []string{labels[0]}
			}
			result = append(result, Selection{Attributes: attrs})
			if len(result) > limit {
				return ErrLimit
			}
			return nil
		}
		group := keys[position]
		for _, label := range selection.Attributes[group] {
			line[group] = []string{label}
			if err := visit(position + 1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(0); err != nil {
		return nil, err
	}
	return result, nil
}

func expandFeatures(selection Selection, limit int) ([]Selection, error) {
	keys := sortedFeatureKeys(selection.Features)
	result := make([]Selection, 0, min(limit, 100))
	line := make(map[string]int, len(keys))
	var visit func(int) error
	visit = func(position int) error {
		if position == len(keys) {
			features := make(map[string][]int, len(line))
			for key, value := range line {
				features[key] = []int{value}
			}
			result = append(result, Selection{Features: features})
			if len(result) > limit {
				return ErrLimit
			}
			return nil
		}
		feature := keys[position]
		for _, value := range selection.Features[feature] {
			line[feature] = value
			if err := visit(position + 1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(0); err != nil {
		return nil, err
	}
	return result, nil
}

func sortedAttributeKeys(values map[string][]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedFeatureKeys(values map[string][]int) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
