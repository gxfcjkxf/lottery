package rules

import (
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"testing"
)

func TestMarkSixOutcomesAndRounding(t *testing.T) {
	for _, v := range []struct {
		code string
		n    int
		want string
	}{{"number_49", 49, "won"}, {"number_48", 49, "lost"}, {"big", 25, "won"}, {"small", 24, "won"}, {"big_even", 48, "won"}, {"small_odd", 23, "won"}, {"big", 24, "lost"}, {"small", 25, "lost"}} {
		got, e := MarkSixOutcome(v.code, v.n, MarkSixNumberAttributes{})
		if e != nil || got != v.want {
			t.Fatalf("%+v: %s %v", v, got, e)
		}
	}
	for _, code := range MarkSixOptions()[49:60] {
		got, e := MarkSixOutcome(code, 49, MarkSixNumberAttributes{})
		if e != nil || got != "push" {
			t.Fatal(code, got, e)
		}
	}
	if _, e := MarkSixOutcome("number_1", 1, MarkSixNumberAttributes{}); e == nil {
		t.Fatal("accepted noncanonical option")
	}
	for _, v := range []struct {
		stake         points.Amount
		odds, outcome string
		want          points.Amount
	}{{10, "48.99", "won", 490}, {1, "1.5", "won", 2}, {10, "1.98", "push", 10}, {10, "1.98", "lost", 0}} {
		got, e := MarkSixPayout(v.stake, v.odds, v.outcome)
		if e != nil || got != v.want {
			t.Fatalf("%+v: %v %v", v, got, e)
		}
	}
}

func TestMarkSixWaveZodiacAndElementOutcomes(t *testing.T) {
	a := MarkSixNumberAttributes{Waves: map[string]string{"01": "red", "02": "blue", "03": "green", "49": "green"}, Zodiac: map[string]string{"01": "rat", "49": "horse"}, Elements: map[string]string{"01": "metal", "49": "fire"}}
	for _, v := range []struct {
		code string
		n    int
		want string
	}{
		{"wave_red", 1, "won"}, {"wave_blue", 2, "won"}, {"wave_green", 3, "won"}, {"wave_blue", 1, "lost"},
		{"wave_red", 49, "lost"}, {"wave_blue", 49, "lost"}, {"wave_green", 49, "won"},
		{"zodiac_rat", 1, "won"}, {"zodiac_horse", 1, "lost"}, {"zodiac_horse", 49, "won"}, {"zodiac_rat", 49, "lost"},
		{"element_metal", 1, "won"}, {"element_fire", 1, "lost"}, {"element_fire", 49, "won"}, {"element_water", 49, "lost"},
	} {
		got, e := MarkSixOutcome(v.code, v.n, a)
		if e != nil || got != v.want {
			t.Fatalf("%+v: %s %v", v, got, e)
		}
	}
	// All attributes are supplied by the purchased snapshot, including changed mappings.
	a.Waves["01"] = "blue"
	a.Zodiac["01"] = "pig"
	a.Elements["01"] = "water"
	for _, code := range []string{"wave_blue", "zodiac_pig", "element_water"} {
		got, e := MarkSixOutcome(code, 1, a)
		if e != nil || got != "won" {
			t.Fatal(code, got, e)
		}
	}
	for _, code := range []string{"wave_red", "zodiac_rat", "element_metal"} {
		if _, e := MarkSixOutcome(code, 1, MarkSixNumberAttributes{}); e == nil {
			t.Fatal("missing snapshot accepted", code)
		}
	}
	a.Elements["01"] = "unknown"
	if _, e := MarkSixOutcome("element_fire", 1, a); e == nil {
		t.Fatal("invalid element accepted")
	}
	if _, e := MarkSixOutcome("zodiac_unknown", 1, a); e == nil {
		t.Fatal("unknown option accepted")
	}
}
