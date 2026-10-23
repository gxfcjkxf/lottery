package rules

import (
	"fmt"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"math/big"
	"slices"
	"strconv"
	"strings"
)

var CanadaProfiles = []string{"canada_28", "canada_996", "canada_vip", "canada_42", "canada_web", "canada_50", "canada_baccarat", "canada_niuniu"}

func ValidateCanadaModel(m Model) error {
	if m.Type != "DIGITS_0_9" || m.Length != 3 || !m.AllowRepeat || !m.Ordered {
		return ErrInvalid
	}
	return ValidateModel(m)
}
func CanadaOptions(profile string) []string {
	if !slices.Contains(CanadaProfiles, profile) {
		return nil
	}
	if profile == "canada_niuniu" {
		return []string{"a", "b", "aa", "bb", "allin_a", "allin_b"}
	}
	out := []string{}
	if profile == "canada_baccarat" {
		out = append(out, "banker", "player", "tie", "banker_pair", "player_pair")
		for _, v := range []string{"big", "small", "odd", "even", "big_odd", "big_even", "small_odd", "small_even"} {
			out = append(out, "middle_"+v)
		}
		for n := 0; n < 10; n++ {
			out = append(out, fmt.Sprintf("middle_%d", n))
		}
		return out
	}
	out = append(out, "big", "small", "odd", "even", "big_odd", "big_even", "small_odd", "small_even", "extreme_big", "extreme_small", "triple", "straight", "pair")
	for n := 0; n < 28; n++ {
		out = append(out, fmt.Sprintf("sum_%d", n))
	}
	for _, v := range []string{"big", "small", "odd", "even", "big_odd", "big_even", "small_odd", "small_even"} {
		out = append(out, "tail_"+v)
	}
	for n := 1; n <= 8; n++ {
		out = append(out, fmt.Sprintf("tail_%d", n))
	}
	if profile == "canada_web" || profile == "canada_996" || profile == "canada_vip" {
		for p := 1; p <= 3; p++ {
			for _, v := range []string{"big", "small", "odd", "even", "big_odd", "big_even", "small_odd", "small_even"} {
				out = append(out, fmt.Sprintf("digit_%d_%s", p, v))
			}
			for n := 0; n < 10; n++ {
				out = append(out, fmt.Sprintf("digit_%d_%d", p, n))
			}
		}
	}
	if profile == "canada_web" || profile == "canada_996" {
		for _, p := range []string{"12", "13", "23"} {
			for _, v := range []string{"dragon", "tiger", "tie"} {
				out = append(out, "compare_"+p+"_"+v)
			}
		}
	}
	return out
}
func canadaSide(code string, n, cut int) bool {
	big, odd := n >= cut, n%2 == 1
	switch code {
	case "big":
		return big
	case "small":
		return !big
	case "odd":
		return odd
	case "even":
		return !odd
	case "big_odd":
		return big && odd
	case "big_even":
		return big && !odd
	case "small_odd":
		return !big && odd
	case "small_even":
		return !big && !odd
	}
	return false
}

// Effective is independent of refunds: the reference 28 and 4.2 rules differ.
func CanadaOutcome(profile, code string, d Draw, nonTailTotal int64) (outcome, priceCode string, effective bool, err error) {
	if !slices.Contains(CanadaOptions(profile), code) || len(d.Digits) != 3 {
		return "", "", false, ErrInvalid
	}
	for _, n := range d.Digits {
		if n < 0 || n > 9 {
			return "", "", false, ErrInvalid
		}
	}
	a, b, c := d.Digits[0], d.Digits[1], d.Digits[2]
	sum := a + b + c
	won := false
	effective = true
	priceCode = code
	if profile == "canada_baccarat" {
		switch code {
		case "banker":
			if a == c {
				return "push", code, false, nil
			}
			won = a > c
		case "player":
			if a == c {
				return "push", code, false, nil
			}
			won = a < c
		case "tie":
			won = a == c
		case "banker_pair":
			won = a == b
		case "player_pair":
			won = c == b
		default:
			v := strings.TrimPrefix(code, "middle_")
			n, e := strconv.Atoi(v)
			if e == nil {
				won = n == b
			} else {
				won = canadaSide(v, b, 5)
			}
		}
		if a == c {
			effective = false
		}
	} else {
		tail := sum % 10
		isSide := slices.Contains([]string{"big", "small", "odd", "even", "big_odd", "big_even", "small_odd", "small_even"}, code)
		switch {
		case isSide:
			won = canadaSide(code, sum, 14)
		case code == "extreme_big":
			won = sum >= 22
		case code == "extreme_small":
			won = sum <= 5
		case code == "triple":
			won = a == b && b == c
		case code == "pair":
			won = (a == b || a == c || b == c) && !(a == b && b == c)
		case code == "straight":
			v := []int{a, b, c}
			slices.Sort(v)
			won = v[0]+1 == v[1] && v[1]+1 == v[2]
		case strings.HasPrefix(code, "sum_"):
			n, _ := strconv.Atoi(strings.TrimPrefix(code, "sum_"))
			won = sum == n
		case strings.HasPrefix(code, "tail_"):
			v := strings.TrimPrefix(code, "tail_")
			n, e := strconv.Atoi(v)
			if e == nil {
				won = tail == n
			} else {
				won = tail >= 1 && tail <= 8 && canadaSide(v, tail, 5)
			}
		case strings.HasPrefix(code, "digit_"):
			v := strings.SplitN(code, "_", 3)
			p, _ := strconv.Atoi(v[1])
			n, e := strconv.Atoi(v[2])
			if e == nil {
				won = d.Digits[p-1] == n
			} else {
				won = canadaSide(v[2], d.Digits[p-1], 5)
			}
		case strings.HasPrefix(code, "compare_"):
			v := strings.Split(code, "_")
			x, y := d.Digits[int(v[1][0]-'1')], d.Digits[int(v[1][1]-'1')]
			won = v[2] == "dragon" && x > y || v[2] == "tiger" && x < y || v[2] == "tie" && x == y
		}
		special := sum == 13 || sum == 14
		if profile == "canada_28" && isSide && (special || a == b || a == c || b == c || canadaStraight(a, b, c)) && won {
			return "push", code, true, nil
		}
		if (profile == "canada_web" || profile == "canada_996") && isSide && special && won {
			effective = false
		}
		if profile == "canada_vip" && isSide && special {
			priceCode = "special_side"
			if strings.Contains(code, "_") {
				priceCode = "special_combo"
			}
		}
		if (profile == "canada_42" || profile == "canada_50") && special {
			effective = false
			if isSide {
				if profile == "canada_50" {
					won = false
				} else if nonTailTotal >= 10000 || strings.Contains(code, "_") {
					if won {
						return "push", code, false, nil
					}
				} else {
					priceCode = "special_side"
				}
			}
		}
	}
	if won {
		return "won", priceCode, effective, nil
	}
	return "lost", priceCode, effective, nil
}
func canadaStraight(a, b, c int) bool {
	v := []int{a, b, c}
	slices.Sort(v)
	return v[0]+1 == v[1] && v[1]+1 == v[2]
}

func CanadaRisk(profile, code string) int64 {
	if profile != "canada_niuniu" {
		return 1
	}
	switch code {
	case "a", "b":
		return 10
	case "aa", "bb":
		return 20
	case "allin_a", "allin_b":
		return 1
	}
	return 0
}

// Niu rank zero is ten. AA/BB adds ten to both winning and losing ranks.
func CanadaNiu(code string, d Draw) (outcome string, win, loss int64, err error) {
	if len(d.Digits) != 3 || !slices.Contains(CanadaOptions("canada_niuniu"), code) {
		return "", 0, 0, ErrInvalid
	}
	rank := func(n int) int64 {
		if n == 0 {
			return 10
		}
		return int64(n)
	}
	p := d.Digits[0]
	if code == "b" || code == "bb" || code == "allin_b" {
		p = d.Digits[1]
	}
	dealer := d.Digits[2]
	pr, dr := rank(p), rank(dealer)
	triple := d.Digits[0] == d.Digits[1] && d.Digits[1] == dealer
	if pr == dr && !triple {
		return "push", 0, 0, nil
	}
	win, loss = pr, dr
	if code == "aa" || code == "bb" {
		win += 10
		loss += 10
	}
	if strings.HasPrefix(code, "allin_") {
		win, loss = 1, 1
	}
	if pr > dr && !triple {
		return "won", win, 0, nil
	}
	return "lost", 0, loss, nil
}

func CanadaNiuProfit(stake points.Amount, odds string, multiplier int64) (points.Amount, string, error) {
	rate, e := parseOdds(odds)
	if e != nil || stake <= 0 || multiplier < 1 || multiplier > 20 {
		return 0, "", ErrInvalid
	}
	rate.Mul(rate, big.NewRat(multiplier*97, 100))
	display := rate.FloatString(8)
	amount, e := exactPoints(new(big.Rat).Mul(rate, new(big.Rat).SetInt64(int64(stake))), nil)
	return amount, display, e
}
