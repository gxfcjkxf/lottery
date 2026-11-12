package rules

import (
	"context"
	"testing"
)

func TestSpecialDistributionUsesCurrentOddsAndPeriodBindings(t *testing.T) {
	c, b, p, old, _ := configuredFixture(t)
	// The five-number zodiac group must share a 10-point stake across
	// 01,13,25,37,49; its full 10-point purchase determines the return.
	label := ""
	for key, ns := range b.Attributes["zodiac"] {
		if len(ns) == 5 {
			label = key
		}
	}
	q, err := old.Prepare(context.Background(), []ConfiguredRequest{{PlayID: "special_zodiac", Options: []string{label}, Plate: "A", Points: 1000, Multiplier: 1}})
	if err != nil {
		t.Fatal(err)
	}
	p.Plates["A"]["zodiac_"+label] = "5"
	current, err := NewConfiguredEngine(c, b, p)
	if err != nil {
		t.Fatal(err)
	}
	projected, err := current.ProjectSpecialLine(context.Background(), q.Lines[0])
	if err != nil {
		t.Fatal(err)
	}
	wins := 0
	for _, v := range projected {
		if v.Won {
			wins++
			if v.Return != 5000 || (v.Number-1)%12 != 0 {
				t.Fatalf("wrong current return or period mapping: %+v", v)
			}
		} else if v.Return != 0 {
			t.Fatalf("losing return: %+v", v)
		}
	}
	if wins != 5 || q.Lines[0].Stake != 1000 {
		t.Fatalf("wins=%d", wins)
	}
}

func TestSpecialDistributionPushAndRegularExclusion(t *testing.T) {
	_, _, _, e, _ := configuredFixture(t)
	for _, test := range []struct {
		play, option string
		excluded     bool
	}{{"special_sides", "big", false}, {"regular_numbers", "1", true}, {"linked_special", "1", true}} {
		// Excluded plays do not need a valid selection to be excluded.
		line := ConfiguredQuoteLine{PlayID: test.play, Options: []string{test.option}, Plate: "A", Stake: 1000}
		out, err := e.ProjectSpecialLine(context.Background(), line)
		if err != nil {
			t.Fatal(err)
		}
		if test.excluded {
			if len(out) != 0 {
				t.Fatalf("included %s", test.play)
			}
			continue
		}
		if out[48].Won || out[48].Return != 1000 {
			t.Fatalf("49 must refund: %+v", out[48])
		}
		wins := 0
		for _, v := range out {
			if v.Won {
				wins++
			}
		}
		if wins != 24 {
			t.Fatalf("big has %d numbers", wins)
		}
	}
}
