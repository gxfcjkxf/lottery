package rules

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

// Odds include the stake. Number 49 wins green-wave bets;
// zodiac and element bets include 49 and use purchased number attributes.
type MarkSixNumberAttributes struct {
	Waves    map[string]string
	Zodiac   map[string]string
	Elements map[string]string
}

func MarkSixSpecialOptions() []string {
	out := make([]string, 0, 77)
	for n := 1; n <= 49; n++ {
		out = append(out, fmt.Sprintf("number_%02d", n))
	}
	out = append(out, "big", "small", "odd", "even", "big_odd", "big_even", "small_odd", "small_even", "sum_odd", "sum_even", "sum_big", "sum_small", "tail_big", "tail_small")
	for _, v := range []string{"red", "blue", "green"} {
		out = append(out, "wave_"+v)
	}
	for _, v := range []string{"rat", "ox", "tiger", "rabbit", "dragon", "snake", "horse", "goat", "monkey", "rooster", "dog", "pig"} {
		out = append(out, "zodiac_"+v)
	}
	for _, v := range []string{"metal", "wood", "water", "fire", "earth"} {
		out = append(out, "element_"+v)
	}
	return out
}

func ValidateMarkSixModel(m Model) error {
	if m.Type != "X_PLUS_Y" || m.RegularCount != 6 || m.SpecialCount != 1 || m.RegularPool.Min != 1 || m.RegularPool.Max != 49 || m.SpecialPool.Min != 1 || m.SpecialPool.Max != 49 || len(m.RegularPool.Values) != 0 || len(m.SpecialPool.Values) != 0 || m.RegularPool.AllowRepeat || m.SpecialPool.AllowRepeat || m.AllowRepeat {
		return ErrInvalid
	}
	return ValidateModel(m)
}

func MarkSixOutcome(code string, number int, attributes MarkSixNumberAttributes) (string, error) {
	if number < 1 || number > 49 {
		return "", ErrInvalid
	}
	valid := false
	for _, c := range MarkSixSpecialOptions() {
		if c == code {
			valid = true
			break
		}
	}
	if !valid {
		return "", ErrInvalid
	}
	if len(code) == 9 && code[:7] == "number_" {
		n, _ := strconv.Atoi(code[7:])
		if n == number {
			return "won", nil
		}
		return "lost", nil
	}
	key := fmt.Sprintf("%02d", number)
	var actual, want string
	switch {
	case strings.HasPrefix(code, "wave_"):
		if number == 49 {
			if code == "wave_green" {
				return "won", nil
			}
			return "lost", nil
		}
		actual = attributes.Waves[key]
		want = strings.TrimPrefix(code, "wave_")
		if actual != "red" && actual != "blue" && actual != "green" {
			return "", ErrInvalid
		}
	case strings.HasPrefix(code, "zodiac_"):
		actual = attributes.Zodiac[key]
		want = strings.TrimPrefix(code, "zodiac_")
		switch actual {
		case "rat", "ox", "tiger", "rabbit", "dragon", "snake", "horse", "goat", "monkey", "rooster", "dog", "pig":
		default:
			return "", ErrInvalid
		}
	case strings.HasPrefix(code, "element_"):
		actual = attributes.Elements[key]
		want = strings.TrimPrefix(code, "element_")
		switch actual {
		case "metal", "wood", "water", "fire", "earth":
		default:
			return "", ErrInvalid
		}
	}
	if want != "" {
		if actual == want {
			return "won", nil
		}
		return "lost", nil
	}
	if number == 49 {
		return "push", nil
	}
	bigNumber, odd := number >= 25, number%2 == 1
	sum, tail := number/10+number%10, number%10
	win := map[string]bool{"big": bigNumber, "small": !bigNumber, "odd": odd, "even": !odd, "big_odd": bigNumber && odd, "big_even": bigNumber && !odd, "small_odd": !bigNumber && odd, "small_even": !bigNumber && !odd, "sum_odd": sum%2 == 1, "sum_even": sum%2 == 0, "sum_big": sum >= 7, "sum_small": sum <= 6, "tail_big": tail >= 5, "tail_small": tail <= 4}[code]
	if win {
		return "won", nil
	}
	return "lost", nil
}

func MarkSixPayout(stake points.Amount, odds, outcome string) (points.Amount, error) {
	if stake <= 0 {
		return 0, ErrInvalid
	}
	switch outcome {
	case "push":
		return stake, nil
	case "lost":
		return 0, nil
	case "won":
		v, err := parseOdds(odds)
		if err != nil {
			return 0, err
		}
		return exactPoints(new(big.Rat).Mul(v, new(big.Rat).SetInt64(int64(stake))), nil)
	default:
		return 0, ErrInvalid
	}
}
