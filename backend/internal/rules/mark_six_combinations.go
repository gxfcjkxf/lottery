package rules

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// A purchased batch is expanded into at most 1,000 independently priced lines.
const MarkSixMaxLines = 1000

var markSixAnimals = []string{"rat", "ox", "tiger", "rabbit", "dragon", "snake", "horse", "goat", "monkey", "rooster", "dog", "pig"}

type MarkSixCombination struct{ Size, Max, Min, Maximum int }

func MarkSixCombinationSpec(code string) (MarkSixCombination, bool) {
	switch code {
	case "linked_2":
		return MarkSixCombination{2, 49, 1, 49}, true
	case "linked_3_2", "linked_3":
		return MarkSixCombination{3, 49, 1, 49}, true
	}
	for n := 1; n <= 5; n++ {
		if code == fmt.Sprintf("zodiac_link_%d", n) {
			return MarkSixCombination{n, 12, 1, 12}, true
		}
	}
	for n := 2; n <= 5; n++ {
		if code == fmt.Sprintf("tail_link_%d", n) {
			return MarkSixCombination{n, 10, 0, 9}, true
		}
	}
	for n := 5; n <= 10; n++ {
		if code == fmt.Sprintf("miss_%d", n) {
			max := n + 3
			if n >= 8 {
				max = n + 2
			}
			return MarkSixCombination{n, max, 1, 49}, true
		}
	}
	return markSixExpandedSpec(code)
}
func MarkSixOptions() []string {
	out := MarkSixSpecialOptions()
	for n := 1; n <= 49; n++ {
		out = append(out, fmt.Sprintf("regular_%02d", n))
	}
	for position := 1; position <= 6; position++ {
		for n := 1; n <= 49; n++ {
			out = append(out, fmt.Sprintf("position_%d_%02d", position, n))
		}
	}
	out = append(out, "linked_2", "linked_3_2", "linked_3_2_full", "linked_3")
	for n := 1; n <= 5; n++ {
		out = append(out, fmt.Sprintf("zodiac_link_%d", n))
	}
	for n := 2; n <= 5; n++ {
		out = append(out, fmt.Sprintf("tail_link_%d", n), fmt.Sprintf("tail_link_%d_zero", n))
	}
	for n := 5; n <= 10; n++ {
		out = append(out, fmt.Sprintf("miss_%d", n))
	}
	for n := 0; n <= 4; n++ {
		out = append(out, fmt.Sprintf("head_%d", n))
	}
	for n := 0; n <= 9; n++ {
		out = append(out, fmt.Sprintf("special_tail_%d", n), fmt.Sprintf("draw_tail_%d", n))
	}
	out = append(out, "domestic", "wild", "seven_red", "seven_blue", "seven_green", "seven_tie")
	for _, wave := range []string{"red", "blue", "green"} {
		for _, side := range []string{"big", "small", "odd", "even"} {
			out = append(out, "half_"+wave+"_"+side)
		}
	}
	for n := 1; n <= 7; n++ {
		out = append(out, fmt.Sprintf("zodiac_count_%d", n))
	}
	return append(out, MarkSixExpandedOptions()...)
}
func MarkSixSelectable(code string) bool {
	return (code == "parlay" || slices.Contains(MarkSixOptions(), code)) && code != "linked_2_special_hit" && code != "linked_3_2_full" && !strings.HasSuffix(code, "_zero")
}
func MarkSixExpand(code string, selection []int) ([][]int, error) {
	if !MarkSixSelectable(code) {
		return nil, ErrInvalid
	}
	if code == "parlay" {
		if _, e := MarkSixParlayCodes(selection); e != nil {
			return nil, e
		}
		values := append([]int{}, selection...)
		sort.Ints(values)
		return [][]int{values}, nil
	}
	spec, combo := MarkSixCombinationSpec(code)
	if !combo {
		if len(selection) != 0 {
			return nil, ErrInvalid
		}
		return [][]int{nil}, nil
	}
	if len(selection) < spec.Size || len(selection) > spec.Max {
		return nil, ErrInvalid
	}
	values := append([]int{}, selection...)
	sort.Ints(values)
	for i, n := range values {
		if n < spec.Min || n > spec.Maximum || (i > 0 && values[i-1] == n) {
			return nil, ErrInvalid
		}
	}
	out := [][]int{}
	var expand func(int, []int) error
	expand = func(start int, line []int) error {
		if len(line) == spec.Size {
			if len(out) == MarkSixMaxLines {
				return ErrInvalid
			}
			out = append(out, append([]int{}, line...))
			return nil
		}
		for i := start; i <= len(values)-(spec.Size-len(line)); i++ {
			if e := expand(i+1, append(line, values[i])); e != nil {
				return e
			}
		}
		return nil
	}
	if e := expand(0, nil); e != nil {
		return nil, e
	}
	return out, nil
}
func MarkSixOddsCode(code string, selection []int) string {
	if strings.HasPrefix(code, "tail_link_") && slices.Contains(selection, 0) {
		return code + "_zero"
	}
	return code
}

// A line contains exactly one combination. Full indicates the three-hit tier
// of a 3-pick / 2-hit bet; its price is captured at purchase time.
func MarkSixLineOutcome(code string, selection []int, draw Draw, a MarkSixNumberAttributes) (outcome string, full bool, err error) {
	if len(draw.Regular) != 6 || len(draw.Special) != 1 {
		return "", false, ErrInvalid
	}
	lines, e := MarkSixExpand(code, selection)
	if e != nil || len(lines) != 1 {
		return "", false, ErrInvalid
	}
	won := func(hit bool) (string, bool, error) {
		if hit {
			return "won", false, nil
		}
		return "lost", false, nil
	}
	special := draw.Special[0]
	if slices.Contains(MarkSixSpecialOptions(), code) {
		v, e := MarkSixOutcome(code, special, a)
		return v, false, e
	}
	if code == "parlay" || slices.Contains(MarkSixExpandedOptions(), code) {
		return markSixExpandedOutcome(code, selection, draw, a)
	}

	if strings.HasPrefix(code, "regular_") {
		n, _ := strconv.Atoi(strings.TrimPrefix(code, "regular_"))
		return won(slices.Contains(draw.Regular, n))
	}
	if strings.HasPrefix(code, "position_") {
		var position, n int
		if _, e = fmt.Sscanf(code, "position_%d_%d", &position, &n); e != nil {
			return "", false, e
		}
		return won(draw.Regular[position-1] == n)
	}
	all := append(append([]int{}, draw.Regular...), special)
	animals := map[string]bool{}
	tails := map[int]bool{}
	// Annual zodiac attributes are needed only by zodiac-based plays.
	if strings.HasPrefix(code, "zodiac_") || code == "domestic" || code == "wild" {
		for _, n := range all {
			animal := a.Zodiac[fmt.Sprintf("%02d", n)]
			if !slices.Contains(markSixAnimals, animal) {
				return "", false, ErrInvalid
			}
			animals[animal] = true
		}
	}
	for _, n := range all {
		tails[n%10] = true
	}
	if strings.HasPrefix(code, "linked_") {
		hits := 0
		for _, n := range selection {
			if slices.Contains(draw.Regular, n) {
				hits++
			}
		}
		if code == "linked_3_2" && hits == 3 {
			return "won", true, nil
		}
		if code == "linked_3_2" {
			return won(hits == 2)
		}
		return won(hits == len(selection))
	}
	if strings.HasPrefix(code, "miss_") {
		for _, n := range selection {
			if slices.Contains(all, n) {
				return won(false)
			}
		}
		return won(true)
	}
	if strings.HasPrefix(code, "tail_link_") {
		for _, n := range selection {
			if !tails[n] {
				return won(false)
			}
		}
		return won(true)
	}
	if strings.HasPrefix(code, "zodiac_link_") {
		for _, n := range selection {
			if !animals[markSixAnimals[n-1]] {
				return won(false)
			}
		}
		return won(true)
	}
	if strings.HasPrefix(code, "zodiac_count_") {
		n, _ := strconv.Atoi(strings.TrimPrefix(code, "zodiac_count_"))
		return won(len(animals) == n)
	}
	if strings.HasPrefix(code, "head_") {
		n, _ := strconv.Atoi(strings.TrimPrefix(code, "head_"))
		return won(special/10 == n)
	}
	if strings.HasPrefix(code, "special_tail_") {
		n, _ := strconv.Atoi(strings.TrimPrefix(code, "special_tail_"))
		return won(special%10 == n)
	}
	if strings.HasPrefix(code, "draw_tail_") {
		n, _ := strconv.Atoi(strings.TrimPrefix(code, "draw_tail_"))
		return won(tails[n])
	}
	if code == "domestic" || code == "wild" {
		if special == 49 {
			return "push", false, nil
		}
		domestic := slices.Contains([]string{"goat", "horse", "ox", "pig", "dog", "rooster"}, a.Zodiac[fmt.Sprintf("%02d", special)])
		return won((code == "domestic") == domestic)
	}
	if strings.HasPrefix(code, "half_") {
		if special == 49 {
			return "push", false, nil
		}
		parts := strings.Split(code, "_")
		wave := a.Waves[fmt.Sprintf("%02d", special)]
		if !slices.Contains([]string{"red", "blue", "green"}, wave) {
			return "", false, ErrInvalid
		}
		side, e := MarkSixOutcome(parts[2], special, a)
		if e != nil {
			return "", false, e
		}
		return won(wave == parts[1] && side == "won")
	}
	if strings.HasPrefix(code, "seven_") {
		scores := map[string]int{"red": 0, "blue": 0, "green": 0}
		for i, n := range all {
			wave := a.Waves[fmt.Sprintf("%02d", n)]
			if _, ok := scores[wave]; !ok {
				return "", false, ErrInvalid
			}
			weight := 2
			if i == 6 {
				weight = 3
			}
			scores[wave] += weight
		}
		max := 0
		winner := ""
		tie := false
		for wave, score := range scores {
			if score > max {
				max = score
				winner = wave
				tie = false
			} else if score == max {
				tie = true
			}
		}
		if tie {
			if code == "seven_tie" {
				return won(true)
			}
			return "push", false, nil
		}
		return won(code == "seven_"+winner)
	}
	return "", false, ErrInvalid
}
