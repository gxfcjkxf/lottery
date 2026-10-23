package rules

import (
	"fmt"
	"testing"
)

func expandedAttributes() MarkSixNumberAttributes {
	a := MarkSixNumberAttributes{Waves: map[string]string{}, Zodiac: map[string]string{}, Elements: map[string]string{}}
	for n := 1; n <= 49; n++ {
		key := fmt.Sprintf("%02d", n)
		a.Waves[key] = "red"
		a.Zodiac[key] = markSixAnimals[(19-n%12)%12]
		a.Elements[key] = "water"
	}
	a.Waves["49"] = "green"
	return a
}
func TestMarkSixExpandedOutcomesAndRegular49(t *testing.T) {
	a := expandedAttributes()
	d := Draw{Regular: []int{1, 13, 25, 37, 49, 2}, Special: []int{3}}
	tests := []struct {
		code      string
		selection []int
		outcome   string
		alt       bool
	}{
		{"regular_zodiac_horse", nil, "won", false},
		{"heaven", nil, "won", false}, {"earth", nil, "lost", false}, {"front", nil, "won", false}, {"back", nil, "lost", false},
		{"quarter_red_small_odd", nil, "won", false}, {"quarter_red_big_odd", nil, "lost", false}, {"quarter_blue_small_odd", nil, "lost", false},
		{"zodiac_count_odd", nil, "won", false}, {"zodiac_count_even", nil, "lost", false},
		{"zodiac_pair", []int{5, 7}, "won", false}, {"zodiac_miss_2", []int{1, 2}, "won", false}, {"zodiac_miss_2", []int{5, 7}, "lost", false},
		{"position_attribute_5_wave_green", nil, "won", false}, {"position_attribute_5_wave_red", nil, "lost", false}, {"position_attribute_5_big", nil, "push", false},
		{"linked_4", []int{1, 13, 25, 49}, "won", false}, {"linked_4", []int{1, 13, 25, 3}, "lost", false},
		{"linked_2_special", []int{49, 3}, "won", true}, {"linked_2_special", []int{49, 2}, "won", false},
		{"linked_special", []int{49, 3}, "won", false}, {"linked_special", []int{49, 2}, "lost", false},
		{"zodiac_nohit_3", []int{1, 2, 12}, "won", false}, {"zodiac_nohit_1", []int{7}, "lost", false},
		{"tail_nohit_2", []int{0, 4}, "won", false}, {"tail_nohit_1", []int{9}, "lost", false},
		{"parlay", []int{516, 300}, "won", false}, {"parlay", []int{500, 300}, "won", false}, {"parlay", []int{516, 301}, "lost", false},
	}
	for _, v := range tests {
		t.Run(v.code+fmt.Sprint(v.selection), func(t *testing.T) {
			got, alt, e := MarkSixLineOutcome(v.code, v.selection, d, a)
			if e != nil || got != v.outcome || alt != v.alt {
				t.Fatalf("got %s %v %v; want %s %v", got, alt, e, v.outcome, v.alt)
			}
		})
	}
	if hits := MarkSixRegularZodiacHits("regular_zodiac_horse", d, a); hits != 5 {
		t.Fatalf("regular 49 must count: %d", hits)
	}
	if paid, e := MarkSixZodiacPayout(100, "1.8", 5); e != nil || paid != 500 {
		t.Fatal("five hits return stake once plus five profits", paid, e)
	}
	if paid, e := MarkSixZodiacPayout(1, "1.5", 2); e != nil || paid != 2 {
		t.Fatal("round total once rather than each hit", paid, e)
	}
	d = Draw{Regular: []int{1, 13, 25, 37, 2, 3}, Special: []int{49}}
	if hits := MarkSixRegularZodiacHits("regular_zodiac_horse", d, a); hits != 4 {
		t.Fatalf("special 49 must not count: %d", hits)
	}
	for _, code := range []string{"wave_green", "wave_red", "wave_blue"} {
		got, _, e := MarkSixLineOutcome(code, nil, d, a)
		want := "lost"
		if code == "wave_green" {
			want = "won"
		}
		if e != nil || got != want {
			t.Fatal(code, got, e)
		}
	}
	if got, _, e := MarkSixLineOutcome("zodiac_miss_2", []int{1, 2}, d, a); e != nil || got != "push" {
		t.Fatal(got, e)
	}
}
func TestMarkSixSideBoundariesAndParlayValidation(t *testing.T) {
	for _, v := range []struct {
		code string
		n    int
		want string
	}{{"sum_big", 16, "won"}, {"sum_small", 15, "won"}, {"sum_even", 11, "won"}, {"sum_odd", 12, "won"}, {"tail_big", 15, "won"}, {"tail_small", 14, "won"}, {"sum_big", 49, "push"}, {"tail_big", 49, "push"}} {
		if got, e := MarkSixOutcome(v.code, v.n, expandedAttributes()); e != nil || got != v.want {
			t.Fatal(v, got, e)
		}
	}
	for _, n := range []int{24, 25} {
		d := Draw{Regular: []int{10, 20, 30, 40, 1, n}, Special: []int{49}}
		got, _, e := MarkSixLineOutcome("total_big", nil, d, expandedAttributes())
		want := "lost"
		if n == 25 {
			want = "won"
		}
		if e != nil || got != want {
			t.Fatal(n, got, e)
		}
	}
	for _, sel := range [][]int{{100}, {100, 102}, {100, 700}, {100, 217}, {100, 200, 300, 400, 500, 600, 601}} {
		if _, e := MarkSixExpand("parlay", sel); e == nil {
			t.Fatal("invalid parlay accepted", sel)
		}
	}
	values := []int{}
	for n := 1; n <= 14; n++ {
		values = append(values, n)
	}
	if _, e := MarkSixExpand("linked_4", values); e == nil {
		t.Fatal("1,001 four-number combinations accepted")
	}
	if p, e := MarkSixProduct([]string{"1.123456", "2.234567", "3.345678", "4.456789", "5.567891", "6.678912"}); e != nil || p == "" {
		t.Fatal(p, e)
	}
	if p, e := MarkSixProduct([]string{"1.5", "1.8"}); e != nil || p != "2.7" {
		t.Fatal(p, e)
	}
	if _, e := MarkSixZodiacPayout(9223372036854775807, "2", 5); e == nil {
		t.Fatal("overflow accepted")
	}
}
