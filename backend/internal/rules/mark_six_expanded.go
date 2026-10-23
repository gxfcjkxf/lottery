package rules

import (
	"fmt"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"math/big"
	"slices"
	"strconv"
	"strings"
)

var MarkSixPositionAttributes = []string{"big", "small", "odd", "even", "big_odd", "big_even", "small_odd", "small_even", "sum_odd", "sum_even", "sum_big", "sum_small", "tail_big", "tail_small", "wave_red", "wave_blue", "wave_green"}

func MarkSixExpandedOptions() []string {
	out := []string{"total_big", "total_small", "total_odd", "total_even", "linked_4", "linked_2_special", "linked_2_special_hit", "linked_special", "zodiac_count_odd", "zodiac_count_even", "heaven", "earth", "front", "back"}
	for p := 1; p <= 6; p++ {
		for _, v := range MarkSixPositionAttributes {
			out = append(out, fmt.Sprintf("position_attribute_%d_%s", p, v))
		}
	}
	for _, a := range markSixAnimals {
		out = append(out, "regular_zodiac_"+a)
	}
	for _, w := range []string{"red", "blue", "green"} {
		for _, v := range []string{"big_odd", "big_even", "small_odd", "small_even"} {
			out = append(out, "quarter_"+w+"_"+v)
		}
	}
	for n := 2; n <= 11; n++ {
		hitCode := fmt.Sprintf("zodiac_hit_%d", n)
		if n == 2 {
			hitCode = "zodiac_pair"
		}
		out = append(out, hitCode, fmt.Sprintf("zodiac_miss_%d", n))
	}
	for n := 1; n <= 5; n++ {
		out = append(out, fmt.Sprintf("zodiac_nohit_%d", n), fmt.Sprintf("tail_nohit_%d", n))
	}
	return out
}
func markSixExpandedSpec(code string) (MarkSixCombination, bool) {
	switch code {
	case "linked_4":
		return MarkSixCombination{4, 49, 1, 49}, true
	case "linked_2_special", "linked_special":
		return MarkSixCombination{2, 49, 1, 49}, true
	}
	for n := 2; n <= 11; n++ {
		hitCode := fmt.Sprintf("zodiac_hit_%d", n)
		if n == 2 {
			hitCode = "zodiac_pair"
		}
		if code == hitCode || code == fmt.Sprintf("zodiac_miss_%d", n) {
			return MarkSixCombination{n, 12, 1, 12}, true
		}
	}
	for n := 1; n <= 5; n++ {
		if code == fmt.Sprintf("zodiac_nohit_%d", n) {
			return MarkSixCombination{n, 12, 1, 12}, true
		}
		if code == fmt.Sprintf("tail_nohit_%d", n) {
			return MarkSixCombination{n, 10, 0, 9}, true
		}
	}
	return MarkSixCombination{}, false
}
func MarkSixParlayCodes(selection []int) ([]string, error) {
	if len(selection) < 2 || len(selection) > 6 {
		return nil, ErrInvalid
	}
	seen := map[int]bool{}
	out := []string{}
	for _, v := range selection {
		p, i := v/100, v%100
		if p < 1 || p > 6 || i < 0 || i >= len(MarkSixPositionAttributes) || seen[p] {
			return nil, ErrInvalid
		}
		seen[p] = true
		out = append(out, fmt.Sprintf("position_attribute_%d_%s", p, MarkSixPositionAttributes[i]))
	}
	return out, nil
}
func MarkSixProduct(prices []string) (string, error) {
	if len(prices) < 1 || len(prices) > 6 {
		return "", ErrInvalid
	}
	product := big.NewRat(1, 1)
	for _, price := range prices {
		v, e := parseOdds(price)
		if e != nil || v.Cmp(big.NewRat(1, 1)) < 0 {
			return "", ErrInvalid
		}
		product.Mul(product, v)
	}
	return strings.TrimRight(strings.TrimRight(product.FloatString(36), "0"), "."), nil
}
func MarkSixPositionOutcome(code string, draw Draw, a MarkSixNumberAttributes) (string, error) {
	parts := strings.SplitN(strings.TrimPrefix(code, "position_attribute_"), "_", 2)
	if len(parts) != 2 {
		return "", ErrInvalid
	}
	p, e := strconv.Atoi(parts[0])
	attr := parts[1]
	if e != nil || p < 1 || p > 6 || !slices.Contains(MarkSixPositionAttributes, attr) || len(draw.Regular) != 6 {
		return "", ErrInvalid
	}
	return MarkSixOutcome(attr, draw.Regular[p-1], a)
}
func markSixExpandedOutcome(code string, selection []int, draw Draw, a MarkSixNumberAttributes) (string, bool, error) {
	won := func(hit bool) (string, bool, error) {
		if hit {
			return "won", false, nil
		}
		return "lost", false, nil
	}
	all := append(append([]int{}, draw.Regular...), draw.Special...)
	special := draw.Special[0]
	if strings.HasPrefix(code, "position_attribute_") {
		v, e := MarkSixPositionOutcome(code, draw, a)
		return v, false, e
	}
	if code == "parlay" {
		codes, e := MarkSixParlayCodes(selection)
		if e != nil {
			return "", false, e
		}
		for _, leg := range codes {
			v, e := MarkSixPositionOutcome(leg, draw, a)
			if e != nil {
				return "", false, e
			}
			if v == "lost" {
				return won(false)
			}
		}
		return won(true)
	}
	if strings.HasPrefix(code, "regular_zodiac_") {
		return won(MarkSixRegularZodiacHits(code, draw, a) > 0)
	}
	if strings.HasPrefix(code, "total_") {
		sum := 0
		for _, n := range all {
			sum += n
		}
		return won(map[string]bool{"total_big": sum >= 175, "total_small": sum <= 174, "total_odd": sum%2 == 1, "total_even": sum%2 == 0}[code])
	}
	if code == "linked_2_special" || code == "linked_special" || code == "linked_4" {
		hits := 0
		for _, n := range selection {
			if slices.Contains(draw.Regular, n) {
				hits++
			}
		}
		if code == "linked_4" {
			return won(hits == 4)
		}
		specialHit := hits == 1 && slices.Contains(selection, special)
		if code == "linked_2_special" && specialHit {
			return "won", true, nil
		}
		return won((code == "linked_2_special" && hits == 2) || (code == "linked_special" && specialHit))
	}
	if code == "zodiac_pair" || strings.HasPrefix(code, "zodiac_hit_") || strings.HasPrefix(code, "zodiac_miss_") {
		if special == 49 {
			return "push", false, nil
		}
		index := slices.Index(markSixAnimals, a.Zodiac[fmt.Sprintf("%02d", special)]) + 1
		if index == 0 {
			return "", false, ErrInvalid
		}
		hit := slices.Contains(selection, index)
		return won(hit == (code == "zodiac_pair" || strings.HasPrefix(code, "zodiac_hit_")))
	}
	if strings.HasPrefix(code, "zodiac_nohit_") || strings.HasPrefix(code, "tail_nohit_") {
		for _, n := range all {
			value := n % 10
			if strings.HasPrefix(code, "zodiac_nohit_") {
				value = slices.Index(markSixAnimals, a.Zodiac[fmt.Sprintf("%02d", n)]) + 1
				if value == 0 {
					return "", false, ErrInvalid
				}
			}
			if slices.Contains(selection, value) {
				return won(false)
			}
		}
		return won(true)
	}
	if code == "zodiac_count_odd" || code == "zodiac_count_even" {
		signs := map[string]bool{}
		for _, n := range all {
			sign := a.Zodiac[fmt.Sprintf("%02d", n)]
			if !slices.Contains(markSixAnimals, sign) {
				return "", false, ErrInvalid
			}
			signs[sign] = true
		}
		return won((len(signs)%2 == 1) == (code == "zodiac_count_odd"))
	}
	if code == "heaven" || code == "earth" || code == "front" || code == "back" {
		if special == 49 {
			return "push", false, nil
		}
		sign := a.Zodiac[fmt.Sprintf("%02d", special)]
		if !slices.Contains(markSixAnimals, sign) {
			return "", false, ErrInvalid
		}
		heaven := slices.Contains([]string{"ox", "rabbit", "dragon", "horse", "monkey", "pig"}, sign)
		front := slices.Index(markSixAnimals, sign) < 6
		return won(map[string]bool{"heaven": heaven, "earth": !heaven, "front": front, "back": !front}[code])
	}
	if strings.HasPrefix(code, "quarter_") {
		if special == 49 {
			return "push", false, nil
		}
		parts := strings.SplitN(strings.TrimPrefix(code, "quarter_"), "_", 2)
		if len(parts) != 2 {
			return "", false, ErrInvalid
		}
		v, e := MarkSixOutcome(parts[1], special, a)
		if e != nil {
			return "", false, e
		}
		return won(a.Waves[fmt.Sprintf("%02d", special)] == parts[0] && v == "won")
	}
	return "", false, ErrInvalid
}
func MarkSixRegularZodiacHits(code string, draw Draw, a MarkSixNumberAttributes) int {
	sign := strings.TrimPrefix(code, "regular_zodiac_")
	hits := 0
	for _, n := range draw.Regular {
		if a.Zodiac[fmt.Sprintf("%02d", n)] == sign {
			hits++
		}
	}
	return hits
}
func MarkSixZodiacPayout(stake points.Amount, odds string, hits int) (points.Amount, error) {
	if hits < 1 || hits > 6 || stake <= 0 {
		return 0, ErrInvalid
	}
	v, e := parseOdds(odds)
	if e != nil {
		return 0, e
	}
	v.Sub(v, big.NewRat(1, 1))
	v.Mul(v, big.NewRat(int64(hits), 1))
	v.Add(v, big.NewRat(1, 1))
	return exactPoints(v.Mul(v, big.NewRat(int64(stake), 1)), nil)
}

func MarkSixExactPayout(stake points.Amount, rate *big.Rat) (points.Amount, error) {
	if stake <= 0 || rate == nil || rate.Sign() <= 0 {
		return 0, ErrInvalid
	}
	return exactPoints(new(big.Rat).Mul(rate, big.NewRat(int64(stake), 1)), nil)
}
