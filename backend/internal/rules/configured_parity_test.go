package rules

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"math/rand"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

func configuredFixture(t *testing.T) (ConfiguredCatalog, ConfiguredBindings, ConfiguredPrices, *ConfiguredEngine, MarkSixNumberAttributes) {
	t.Helper()
	c, err := ConfiguredTemplateCatalog("mark_six")
	if err != nil {
		t.Fatal(err)
	}
	b := ConfiguredBindings{Version: "period-attributes-1", Attributes: map[string]map[string][]int{}}
	for _, a := range c.Attributes {
		b.Attributes[a.ID] = map[string][]int{}
		for _, l := range a.Labels {
			b.Attributes[a.ID][l] = []int{}
		}
	}
	old := MarkSixNumberAttributes{Waves: map[string]string{}, Zodiac: map[string]string{}, Elements: map[string]string{}}
	for n := 1; n <= 49; n++ {
		key := fmt.Sprintf("%02d", n)
		z := markSixAnimals[(19-n%12)%12]
		w := []string{"red", "blue", "green"}[(n-1)%3]
		if n == 49 {
			w = "green"
		}
		element := []string{"metal", "wood", "water", "fire", "earth"}[(n-1)%5]
		old.Zodiac[key], old.Waves[key], old.Elements[key] = z, w, element
		b.Attributes["zodiac"][z] = append(b.Attributes["zodiac"][z], n)
		b.Attributes["wave"][w] = append(b.Attributes["wave"][w], n)
		b.Attributes["element"][element] = append(b.Attributes["element"][element], n)
	}
	p := ConfiguredPrices{Version: "prices-1", Plates: map[string]map[string]string{"A": {}, "B": {}}}
	// All prices are distinct enough to detect wrong selection/tier/plate keys.
	for i, key := range MarkSixOptions() {
		p.Plates["A"][key] = fmt.Sprintf("%d.%06d", 2+i%7, 123450+i)
		p.Plates["B"][key] = fmt.Sprintf("%d.%06d", 1+i%5, 223450+i)
	}
	e, err := NewConfiguredEngine(c, b, p)
	if err != nil {
		t.Fatal(err)
	}
	return c, b, p, e, old
}

type configuredSource struct {
	play   ConfiguredPlay
	option string
}

func configuredSources(c ConfiguredCatalog) map[string]configuredSource {
	out := map[string]configuredSource{}
	for _, p := range c.Plays {
		if p.SourceCode != "" {
			out[p.SourceCode] = configuredSource{p, ""}
		}
		for _, o := range c.Domains[p.Selection.Domain].Options {
			if o.SourceCode != "" {
				out[o.SourceCode] = configuredSource{p, o.ID}
			}
		}
	}
	return out
}

func TestConfiguredCatalogCoverage(t *testing.T) {
	c, _, _, e, _ := configuredFixture(t)
	groups := map[string]bool{}
	sources := map[string]bool{}
	for _, p := range c.Plays {
		groups[p.Group] = true
		add := func(code string) {
			if code != "" {
				if sources[code] {
					t.Fatalf("duplicate source %s", code)
				}
				sources[code] = true
			}
		}
		add(p.SourceCode)
		for _, o := range c.Domains[p.Selection.Domain].Options {
			add(o.SourceCode)
		}
	}
	if len(groups) != 22 || len(sources) != 664 || len(e.PriceKeys()) != 669 {
		t.Fatalf("groups=%d sources=%d prices=%d", len(groups), len(sources), len(e.PriceKeys()))
	}
	for _, code := range MarkSixOptions() {
		if !slices.Contains(e.PriceKeys(), code) {
			t.Fatal("unmapped price", code)
		}
		if MarkSixSelectable(code) && !sources[code] {
			t.Fatal("unmapped choice", code)
		}
	}
	if !sources["parlay"] {
		t.Fatal("missing product configuration")
	}
	for _, p := range c.Plays {
		if p.RoundingScope != "line" {
			t.Fatal("changed rounding", p.ID)
		}
	}
}

func configuredRequestForSource(t *testing.T, c ConfiguredCatalog, source configuredSource, selection []int, plate string, stake points.Amount) ConfiguredRequest {
	t.Helper()
	p := source.play
	r := ConfiguredRequest{PlayID: p.ID, Plate: plate, Points: stake, Multiplier: 1}
	if source.option != "" {
		r.Options = []string{source.option}
		return r
	}
	if p.Selection.Mode == "singles" {
		r.Options = []string{c.Domains[p.Selection.Domain].Options[0].ID}
		return r
	}
	if p.Settlement == "product_of_legs" {
		codes, err := MarkSixParlayCodes(selection)
		if err != nil {
			t.Fatal(err)
		}
		r.Options = codes
		return r
	}
	domain := c.Domains[p.Selection.Domain]
	for _, n := range selection {
		index := n - 1
		if domain.Options[0].ID == "0" {
			index = n
		}
		if index < 0 || index >= len(domain.Options) {
			t.Fatal("invalid test selection")
		}
		r.Options = append(r.Options, domain.Options[index].ID)
	}
	return r
}

func configuredOldPayout(t *testing.T, code string, selection []int, draw Draw, attrs MarkSixNumberAttributes, book map[string]string, stake points.Amount) (string, points.Amount, string) {
	t.Helper()
	outcome, full, err := MarkSixLineOutcome(code, selection, draw, attrs)
	if err != nil {
		t.Fatalf("legacy %s %v: %v", code, selection, err)
	}
	key := MarkSixOddsCode(code, selection)
	if full {
		switch code {
		case "linked_3_2":
			key = "linked_3_2_full"
		case "linked_2_special":
			key = "linked_2_special_hit"
		default:
			t.Fatal("unexpected alternate tier")
		}
	}
	price := book[key]
	var paid points.Amount
	if code == "parlay" && outcome == "won" {
		codes, err := MarkSixParlayCodes(selection)
		if err != nil {
			t.Fatal(err)
		}
		prices := []string{}
		for _, code := range codes {
			v, err := MarkSixPositionOutcome(code, draw, attrs)
			if err != nil {
				t.Fatal(err)
			}
			if v != "push" {
				prices = append(prices, book[code])
			}
		}
		product, err := MarkSixProduct(prices)
		if err != nil {
			t.Fatal(err)
		}
		rate, _ := new(big.Rat).SetString(product)
		paid, err = MarkSixExactPayout(stake, rate)
		if err != nil {
			t.Fatal(err)
		}
	} else if strings.HasPrefix(code, "regular_zodiac_") && outcome == "won" {
		paid, err = MarkSixZodiacPayout(stake, price, MarkSixRegularZodiacHits(code, draw, attrs))
	} else {
		paid, err = MarkSixPayout(stake, price, outcome)
	}
	if err != nil {
		t.Fatal(err)
	}
	return outcome, paid, price
}

func assertConfiguredParity(t *testing.T, c ConfiguredCatalog, e *ConfiguredEngine, prices ConfiguredPrices, attrs MarkSixNumberAttributes, code string, selection []int, draw Draw, plate string, stake points.Amount) {
	t.Helper()
	var source configuredSource
	ok := false
	for _, p := range c.Plays {
		if p.SourceCode == code {
			source = configuredSource{p, ""}
			ok = true
			break
		}
		for _, option := range c.Domains[p.Selection.Domain].Options {
			if option.SourceCode == code {
				source = configuredSource{p, option.ID}
				ok = true
				break
			}
		}
		if ok {
			break
		}
	}
	if !ok {
		t.Fatal("unknown source", code)
	}
	r := configuredRequestForSource(t, c, source, selection, plate, stake)
	q, err := e.Prepare(context.Background(), []ConfiguredRequest{r})
	if err != nil {
		t.Fatalf("prepare %s %v: %v", code, selection, err)
	}
	oldLines, err := MarkSixExpand(code, selection)
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Lines) != len(oldLines) || q.BetPoints != points.Amount(len(oldLines))*stake {
		t.Fatalf("count %s: %d/%d", code, len(q.Lines), len(oldLines))
	}
	out, err := e.Settle(context.Background(), q, draw)
	if err != nil {
		t.Fatalf("settle %s %v %+v: %v", code, selection, draw, err)
	}
	var totalPrize, totalRefund points.Amount
	for i, oldSelection := range oldLines {
		want, paid, price := configuredOldPayout(t, code, oldSelection, draw, attrs, prices.Plates[plate], stake)
		line := out.Lines[i]
		got := line.Prize
		if want == "push" {
			got = line.Refund
			totalRefund += paid
		} else {
			totalPrize += paid
		}
		if line.Outcome != want || got != paid {
			t.Fatalf("parity %s %v plate=%s draw=%+v: got=%s prize=%d refund=%d want=%s paid=%d", code, oldSelection, plate, draw, line.Outcome, line.Prize, line.Refund, want, paid)
		}
		if want == "won" && code != "parlay" {
			selected := 0
			for _, tier := range line.Tiers {
				if tier.Selected {
					selected++
					if tier.Price != price {
						t.Fatalf("wrong price %s got=%s want=%s", code, tier.Price, price)
					}
				}
			}
			if selected != 1 {
				t.Fatal("wrong tier count", code, selected)
			}
		}
	}
	if out.PrizePoints != totalPrize || out.RefundPoints != totalRefund || out.EffectivePoints != q.BetPoints-totalRefund {
		t.Fatal("batch totals", code, out)
	}
}

func TestConfiguredEveryChoiceMatchesExistingRules(t *testing.T) {
	c, _, prices, e, attrs := configuredFixture(t)
	sources := configuredSources(c)
	random := rand.New(rand.NewSource(19102026))
	// Sweep every special number, rotate original regular positions, and use
	// distinct candidate subsets for all parameterized choices on both plates.
	for special := 1; special <= 49; special++ {
		permutation := random.Perm(49)
		regular := []int{}
		for _, v := range permutation {
			if v+1 != special {
				regular = append(regular, v+1)
			}
			if len(regular) == 6 {
				break
			}
		}
		draw := Draw{Regular: regular, Special: []int{special}}
		plate := "A"
		if special%2 == 0 {
			plate = "B"
		}
		for code, source := range sources {
			var selection []int
			if source.play.Settlement == "product_of_legs" {
				selection = []int{100 + (special % 17), 200 + ((special + 3) % 17)}
			} else if source.play.Selection.Mode == "combinations" {
				d := c.Domains[source.play.Selection.Domain]
				perm := random.Perm(len(d.Options))
				for _, i := range perm[:source.play.Selection.PerLine] {
					n := i + 1
					if d.Options[0].ID == "0" {
						n = i
					}
					selection = append(selection, n)
				}
			}
			assertConfiguredParity(t, c, e, prices, attrs, code, selection, draw, plate, points.Amount(1+special%13))
		}
	}
}

func TestConfiguredCombinationExpansionAndSpecialPayouts(t *testing.T) {
	c, b, prices, e, attrs := configuredFixture(t)
	draw := Draw{Regular: []int{1, 13, 25, 37, 49, 2}, Special: []int{3}}
	for _, v := range []struct {
		code      string
		selection []int
	}{
		{"linked_2", []int{1, 2, 3, 13}}, {"linked_3_2", []int{1, 2, 3, 13, 25}}, {"linked_2_special", []int{1, 2, 3, 13}},
		{"tail_link_2", []int{0, 1, 2}}, {"zodiac_link_2", []int{1, 2, 3, 7}}, {"zodiac_pair", []int{1, 2, 3}},
		{"zodiac_miss_2", []int{1, 2, 3, 7}}, {"zodiac_nohit_3", []int{1, 2, 3, 7, 12}}, {"tail_nohit_2", []int{0, 1, 2, 9}},
		{"regular_zodiac_horse", nil}, {"parlay", []int{500, 300}}, {"parlay", []int{516, 300}},
	} {
		for _, plate := range []string{"A", "B"} {
			assertConfiguredParity(t, c, e, prices, attrs, v.code, v.selection, draw, plate, 100)
		}
	}
	for k := 5; k <= 10; k++ {
		max := k + 3
		if k >= 8 {
			max = k + 2
		}
		selection := []int{}
		for i := 1; i <= max; i++ {
			selection = append(selection, i)
		}
		assertConfiguredParity(t, c, e, prices, attrs, "miss_"+strconv.Itoa(k), selection, draw, "B", 1)
	}
	// Independent expected value: positive repeated hits return principal once.
	prices.Plates["A"]["regular_zodiac_horse"] = "1.8"
	e, err := NewConfiguredEngine(c, b, prices)
	if err != nil {
		t.Fatal(err)
	}
	source := configuredSources(c)["regular_zodiac_horse"]
	request := configuredRequestForSource(t, c, source, nil, "A", 100)
	q, err := e.Prepare(context.Background(), []ConfiguredRequest{request})
	if err != nil {
		t.Fatal(err)
	}
	out, err := e.Settle(context.Background(), q, draw)
	if err != nil || out.PrizePoints != 500 {
		t.Fatal("five hits including regular 49", out, err)
	}
	prices.Plates["A"]["regular_zodiac_horse"] = "1.5"
	e, err = NewConfiguredEngine(c, b, prices)
	if err != nil {
		t.Fatal(err)
	}
	request.Points = 1
	draw.Regular = []int{1, 13, 2, 3, 4, 5}
	q, err = e.Prepare(context.Background(), []ConfiguredRequest{request})
	if err != nil {
		t.Fatal(err)
	}
	out, err = e.Settle(context.Background(), q, draw)
	if err != nil || out.PrizePoints != 2 {
		t.Fatal("round once per line", out, err)
	}
	// Color tie: three red regular + three blue regular, special green.
	for _, label := range []string{"red", "blue", "green"} {
		b.Attributes["wave"][label] = nil
	}
	for n := 1; n <= 49; n++ {
		label := "green"
		if n <= 3 {
			label = "red"
		} else if n <= 6 {
			label = "blue"
		}
		b.Attributes["wave"][label] = append(b.Attributes["wave"][label], n)
		attrs.Waves[fmt.Sprintf("%02d", n)] = label
	}
	e, err = NewConfiguredEngine(c, b, prices)
	if err != nil {
		t.Fatal(err)
	}
	draw = Draw{Regular: []int{1, 2, 3, 4, 5, 6}, Special: []int{49}}
	for _, code := range []string{"seven_red", "seven_blue", "seven_green", "seven_tie"} {
		assertConfiguredParity(t, c, e, prices, attrs, code, nil, draw, "A", 10)
	}
}

func TestConfiguredFrozenSnapshotAndQuoteIntegrity(t *testing.T) {
	c, b, p, e, _ := configuredFixture(t)
	source := configuredSources(c)["number_07"]
	r := configuredRequestForSource(t, c, source, nil, "A", 10)
	q, err := e.Prepare(context.Background(), []ConfiguredRequest{r})
	if err != nil {
		t.Fatal(err)
	}
	d := Draw{Regular: []int{1, 2, 3, 4, 5, 6}, Special: []int{7}}
	expected, err := e.Settle(context.Background(), q, d)
	if err != nil {
		t.Fatal(err)
	}
	p.Plates["A"]["number_07"] = "999"
	b.Version = "new-period-attributes"
	c.Plays[0].ID = "mutated-caller"
	out, err := e.Settle(context.Background(), q, d)
	if err != nil || !reflect.DeepEqual(out, expected) {
		t.Fatal("caller mutated snapshot", err)
	}
	restored, err := RestoreConfiguredEngine(e.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	out, err = restored.Settle(context.Background(), q, d)
	if err != nil || !reflect.DeepEqual(out, expected) {
		t.Fatal("snapshot restoration", err)
	}
	var saved ConfiguredSnapshot
	if err := json.Unmarshal(e.Snapshot(), &saved); err != nil {
		t.Fatal(err)
	}
	saved.Prices.Version = "prices-2"
	saved.Prices.Plates["A"]["number_07"] = "999"
	current, err := NewConfiguredEngine(saved.Catalog, saved.Bindings, saved.Prices)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := current.Settle(context.Background(), q, d); err == nil {
		t.Fatal("accepted wrong snapshot")
	}
	if current.DefinitionHash() != e.DefinitionHash() || current.SnapshotHash() == e.SnapshotHash() {
		t.Fatal("definition and execution hashes have different responsibilities")
	}
	q.Lines[0].Prices["win"] = "999"
	if _, err := e.Settle(context.Background(), q, d); err == nil {
		t.Fatal("accepted forged price")
	}
}

func TestConfiguredPositiveCombinationTiersAndPosition49(t *testing.T) {
	c, _, prices, e, attrs := configuredFixture(t)
	cluster := Draw{Regular: []int{1, 13, 25, 37, 49, 2}, Special: []int{3}}
	for _, v := range []struct {
		code      string
		selection []int
	}{
		{"linked_2", []int{1, 13}}, {"linked_3", []int{1, 13, 25}}, {"linked_4", []int{1, 13, 25, 49}},
		{"linked_3_2", []int{1, 13, 25}}, {"linked_3_2", []int{1, 13, 4}},
		{"linked_2_special", []int{49, 3}}, {"linked_2_special", []int{49, 2}}, {"linked_special", []int{49, 3}},
	} {
		outcome, _, _ := configuredOldPayout(t, v.code, v.selection, cluster, attrs, prices.Plates["A"], 10)
		if outcome != "won" {
			t.Fatal("positive fixture", v)
		}
		assertConfiguredParity(t, c, e, prices, attrs, v.code, v.selection, cluster, "A", 10)
	}
	draw := Draw{Regular: []int{1, 2, 3, 4, 5, 6}, Special: []int{7}}
	for k := 1; k <= 5; k++ {
		selected := []int{}
		absent := []int{}
		for n := 1; n <= k; n++ {
			selected = append(selected, n)
			absent = append(absent, n+7)
		}
		assertConfiguredParity(t, c, e, prices, attrs, fmt.Sprintf("zodiac_link_%d", k), selected, draw, "B", 13)
		assertConfiguredParity(t, c, e, prices, attrs, fmt.Sprintf("zodiac_nohit_%d", k), absent, draw, "B", 13)
	}
	for k := 2; k <= 11; k++ {
		hit, miss := []int{}, []int{}
		for n := 1; n <= k; n++ {
			hit = append(hit, n)
			miss = append(miss, n+1)
		}
		code := fmt.Sprintf("zodiac_hit_%d", k)
		if k == 2 {
			code = "zodiac_pair"
		}
		assertConfiguredParity(t, c, e, prices, attrs, code, hit, draw, "A", 7)
		assertConfiguredParity(t, c, e, prices, attrs, fmt.Sprintf("zodiac_miss_%d", k), miss, draw, "A", 7)
	}
	for k := 2; k <= 5; k++ {
		selection := []int{}
		for n := 1; n <= k; n++ {
			selection = append(selection, n)
		}
		assertConfiguredParity(t, c, e, prices, attrs, fmt.Sprintf("tail_link_%d", k), selection, draw, "B", 9)
	}
	zero := Draw{Regular: []int{10, 21, 32, 43, 4, 15}, Special: []int{26}}
	for k := 2; k <= 5; k++ {
		selection := []int{}
		for n := 0; n < k; n++ {
			selection = append(selection, n)
		}
		assertConfiguredParity(t, c, e, prices, attrs, fmt.Sprintf("tail_link_%d", k), selection, zero, "B", 9)
	}
	for k := 5; k <= 10; k++ {
		selection := []int{}
		for n := 10; n < 10+k; n++ {
			selection = append(selection, n)
		}
		assertConfiguredParity(t, c, e, prices, attrs, fmt.Sprintf("miss_%d", k), selection, draw, "B", 3)
	}
	noTail := Draw{Regular: []int{1, 11, 21, 31, 41, 2}, Special: []int{3}}
	for k := 1; k <= 5; k++ {
		assertConfiguredParity(t, c, e, prices, attrs, fmt.Sprintf("tail_nohit_%d", k), []int{0, 4, 5, 6, 7}[:k], noTail, "B", 3)
	}
	for position := 1; position <= 6; position++ {
		d := Draw{Regular: []int{1, 2, 3, 4, 5, 6}, Special: []int{7}}
		d.Regular[position-1] = 49
		for _, attribute := range MarkSixPositionAttributes {
			assertConfiguredParity(t, c, e, prices, attrs, fmt.Sprintf("position_attribute_%d_%s", position, attribute), nil, d, "A", 10)
		}
	}
}
