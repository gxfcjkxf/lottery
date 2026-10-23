package rules

import (
	"fmt"
	"testing"
)

func TestMarkSixCombinationExpansionAndCaps(t *testing.T) {
	for _, v := range []struct {
		code     string
		n, count int
	}{{"linked_2", 4, 6}, {"linked_3", 5, 10}, {"miss_5", 8, 56}, {"miss_6", 9, 84}, {"miss_7", 10, 120}, {"miss_8", 10, 45}, {"miss_9", 11, 55}, {"miss_10", 12, 66}, {"zodiac_link_5", 12, 792}, {"tail_link_5", 10, 252}} {
		values := []int{}
		for n := 1; n <= v.n; n++ {
			if v.code == "tail_link_5" {
				values = append(values, n-1)
			} else {
				values = append(values, n)
			}
		}
		lines, e := MarkSixExpand(v.code, values)
		if e != nil || len(lines) != v.count {
			t.Fatalf("%+v: %d %v", v, len(lines), e)
		}
	}
	for _, v := range []struct {
		code      string
		selection []int
	}{{"linked_2", []int{1, 1}}, {"linked_2", []int{0, 1}}, {"linked_3", []int{1, 2}}, {"miss_5", []int{1, 2, 3, 4, 5, 6, 7, 8, 9}}, {"zodiac_link_2", []int{1, 13}}, {"tail_link_2", []int{1, 10}}, {"linked_3_2_full", nil}, {"tail_link_2_zero", []int{0, 1}}, {"regular_01", []int{1}}} {
		if _, e := MarkSixExpand(v.code, v.selection); e == nil {
			t.Fatal("invalid selection accepted", v)
		}
	}
	values := []int{}
	for n := 1; n <= 20; n++ {
		values = append(values, n)
	}
	if _, e := MarkSixExpand("linked_3", values); e == nil {
		t.Fatal("1,140 combinations accepted")
	}
	lines, e := MarkSixExpand("linked_2", []int{3, 1, 2})
	if e != nil || fmt.Sprint(lines) != "[[1 2] [1 3] [2 3]]" {
		t.Fatal(lines, e)
	}
	if MarkSixOddsCode("tail_link_2", []int{0, 1}) != "tail_link_2_zero" || MarkSixOddsCode("tail_link_2", []int{1, 2}) != "tail_link_2" {
		t.Fatal("zero tail price")
	}
	odds := map[string]string{}
	for _, code := range MarkSixOptions() {
		if _, ok := odds[code]; ok {
			t.Fatal("duplicate odds", code)
		}
		odds[code] = "2"
	}
	if len(odds) != 669 {
		t.Fatal("complete odds catalog", len(odds))
	}
}
func TestMarkSixRemainingPlayOutcomes(t *testing.T) {
	draw := Draw{Regular: []int{1, 2, 3, 14, 15, 26}, Special: []int{49}}
	a := MarkSixNumberAttributes{Waves: map[string]string{}, Zodiac: map[string]string{}, Elements: map[string]string{}}
	for n := 1; n <= 49; n++ {
		a.Zodiac[fmt.Sprintf("%02d", n)] = markSixAnimals[(n-1)%12]
		a.Waves[fmt.Sprintf("%02d", n)] = "red"
	}
	a.Zodiac["49"] = "pig"
	for _, v := range []struct {
		code      string
		selection []int
		want      string
		full      bool
	}{
		{"regular_01", nil, "won", false}, {"regular_49", nil, "lost", false}, {"position_1_01", nil, "won", false}, {"position_2_01", nil, "lost", false},
		{"linked_2", []int{1, 2}, "won", false}, {"linked_2", []int{1, 49}, "lost", false}, {"linked_3_2", []int{1, 2, 4}, "won", false}, {"linked_3_2", []int{1, 2, 3}, "won", true}, {"linked_3", []int{1, 2, 3}, "won", false}, {"linked_3", []int{1, 2, 49}, "lost", false},
		{"zodiac_link_3", []int{1, 2, 3}, "won", false}, {"zodiac_link_4", []int{1, 2, 3, 4}, "lost", false}, {"zodiac_link_1", []int{12}, "won", false},
		{"tail_link_2", []int{1, 9}, "won", false}, {"tail_link_2", []int{0, 1}, "lost", false}, {"miss_5", []int{4, 5, 6, 7, 8}, "won", false}, {"miss_5", []int{4, 5, 6, 7, 49}, "lost", false},
		{"zodiac_pair", []int{1, 12}, "push", false}, {"zodiac_count_4", nil, "won", false}, {"head_4", nil, "won", false}, {"special_tail_9", nil, "won", false}, {"draw_tail_9", nil, "won", false}, {"draw_tail_0", nil, "lost", false}, {"domestic", nil, "push", false}, {"wild", nil, "push", false}, {"half_red_big", nil, "push", false},
	} {
		got, full, e := MarkSixLineOutcome(v.code, v.selection, draw, a)
		if e != nil || got != v.want || full != v.full {
			t.Fatalf("%+v: %s %v %v", v, got, full, e)
		}
	}
	draw.Special[0] = 48
	a.Zodiac["48"] = "goat"
	a.Waves["48"] = "blue"
	for _, code := range []string{"domestic", "half_blue_big", "half_blue_even"} {
		got, _, e := MarkSixLineOutcome(code, nil, draw, a)
		if e != nil || got != "won" {
			t.Fatal(code, got, e)
		}
	}
	got, _, e := MarkSixLineOutcome("zodiac_pair", []int{1, 8}, draw, a)
	if e != nil || got != "won" {
		t.Fatal("pair", got, e)
	}
}
func TestMarkSixSevenWaveTieAndWeight(t *testing.T) {
	d := Draw{Regular: []int{1, 2, 3, 4, 5, 6}, Special: []int{49}}
	a := MarkSixNumberAttributes{Waves: map[string]string{"01": "red", "02": "red", "03": "red", "04": "blue", "05": "blue", "06": "blue", "49": "green"}}
	for _, code := range []string{"seven_red", "seven_blue", "seven_green", "seven_tie"} {
		want := "push"
		if code == "seven_tie" {
			want = "won"
		}
		got, _, e := MarkSixLineOutcome(code, nil, d, a)
		if e != nil || got != want {
			t.Fatal(code, got, e)
		}
	}
	a.Waves["49"] = "red"
	for _, code := range []string{"seven_red", "seven_blue", "seven_tie"} {
		want := "lost"
		if code == "seven_red" {
			want = "won"
		}
		got, _, e := MarkSixLineOutcome(code, nil, d, a)
		if e != nil || got != want {
			t.Fatal(code, got, e)
		}
	}
}
