package rules

import (
	"fmt"
	"testing"
)

func TestCanadaProfilesSpecialSettlement(t *testing.T) {
	d := Draw{Digits: []int{5, 4, 5}}
	for _, v := range []struct {
		p, c, w, k string
		total      int64
		effective  bool
	}{{"canada_28", "big", "push", "big", 30, true}, {"canada_web", "big", "won", "big", 30, false}, {"canada_996", "big", "won", "big", 30, false}, {"canada_vip", "big", "won", "special_side", 30, true}, {"canada_vip", "big_even", "won", "special_combo", 30, true}, {"canada_42", "big", "won", "special_side", 9999, false}, {"canada_42", "big", "push", "big", 10000, false}, {"canada_50", "big", "lost", "big", 30, false}, {"canada_28", "sum_14", "won", "sum_14", 30, true}, {"canada_50", "tail_4", "won", "tail_4", 30, false}, {"canada_baccarat", "banker", "push", "banker", 30, false}, {"canada_baccarat", "middle_4", "won", "middle_4", 30, false}, {"canada_baccarat", "tie", "won", "tie", 30, false}, {"canada_baccarat", "banker_pair", "lost", "banker_pair", 30, false}} {
		o, k, e, err := CanadaOutcome(v.p, v.c, d, v.total)
		if err != nil || o != v.w || k != v.k || e != v.effective {
			t.Fatalf("%+v: %s %s %v %v", v, o, k, e, err)
		}
	}
}
func TestCanadaPatternsTailsPositions(t *testing.T) {
	for _, v := range []struct {
		p, c string
		d    []int
		want string
	}{{"canada_28", "small", []int{1, 2, 3}, "push"}, {"canada_web", "straight", []int{9, 0, 1}, "lost"}, {"canada_web", "straight", []int{2, 0, 1}, "won"}, {"canada_web", "pair", []int{2, 2, 2}, "lost"}, {"canada_web", "triple", []int{2, 2, 2}, "won"}, {"canada_web", "tail_even", []int{3, 3, 4}, "lost"}, {"canada_web", "tail_odd", []int{3, 3, 3}, "lost"}, {"canada_web", "digit_1_big_odd", []int{5, 4, 0}, "won"}, {"canada_web", "compare_13_dragon", []int{4, 1, 4}, "lost"}, {"canada_web", "compare_13_tie", []int{4, 1, 4}, "won"}, {"canada_baccarat", "banker_pair", []int{5, 5, 3}, "won"}, {"canada_baccarat", "player_pair", []int{5, 3, 3}, "won"}} {
		o, _, _, err := CanadaOutcome(v.p, v.c, Draw{Digits: v.d}, 0)
		if err != nil || o != v.want {
			t.Fatal(v, o, err)
		}
	}
}
func TestCanadaCatalogAndAllDraws(t *testing.T) {
	for _, p := range CanadaProfiles {
		if p == "canada_niuniu" {
			continue
		}
		catalog, err := ConfiguredTemplateCatalog(p)
		if err != nil {
			t.Fatal(err)
		}
		odds := map[string]string{}
		for _, play := range catalog.Plays {
			for _, outcome := range play.Decisions.Outcomes {
				if outcome.PriceKey != "" {
					odds[outcome.PriceKey] = "2"
				}
			}
		}
		for n := 0; n < 1000; n++ {
			d := Draw{Digits: []int{n / 100, n / 10 % 10, n % 10}}
			for _, code := range CanadaOptions(p) {
				out, key, _, err := CanadaOutcome(p, code, d, 100)
				if err != nil || out != "won" && out != "lost" && out != "push" || odds[key] == "" {
					t.Fatal(fmt.Sprint(p, n, code, out, key, err))
				}
			}
		}
	}
	if _, _, _, e := CanadaOutcome("unknown", "big", Draw{Digits: []int{1, 2, 3}}, 0); e == nil {
		t.Fatal("unknown profile accepted")
	}
}
func TestCanadaNiuMultipliers(t *testing.T) {
	for _, v := range []struct {
		code      string
		draw      []int
		out       string
		win, loss int64
	}{{"a", []int{6, 7, 5}, "won", 6, 0}, {"aa", []int{6, 7, 5}, "won", 16, 0}, {"bb", []int{6, 4, 5}, "lost", 0, 15}, {"a", []int{0, 1, 9}, "won", 10, 0}, {"aa", []int{0, 0, 0}, "lost", 0, 20}, {"a", []int{6, 6, 6}, "lost", 0, 6}, {"allin_a", []int{6, 6, 6}, "lost", 0, 1}, {"aa", []int{5, 4, 5}, "push", 0, 0}} {
		o, w, l, e := CanadaNiu(v.code, Draw{Digits: v.draw})
		if e != nil || o != v.out || w != v.win || l != v.loss {
			t.Fatal(v, o, w, l, e)
		}
	}
}
func TestCanadaNiuProfitKeepsExactFeeBeforeFinalRounding(t *testing.T) {
	n, rate, e := CanadaNiuProfit(1234567, "1.000001", 20)
	if e != nil || n != 23950624 || rate != "19.40001940" {
		t.Fatal(n, rate, e)
	}
	n, _, e = CanadaNiuProfit(5, "1000", 20)
	if e != nil || n != 97000 {
		t.Fatal(n, e)
	}
}
