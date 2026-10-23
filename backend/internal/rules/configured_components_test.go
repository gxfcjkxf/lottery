package rules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

func configuredInt(v int) *int { return &v }
func configuredConstant(v int) *ConfiguredMetric {
	return &ConfiguredMetric{Op: "constant", Value: configuredInt(v)}
}
func configuredComparison(m *ConfiguredMetric, n int) ConfiguredCondition {
	return ConfiguredCondition{Op: "equals", Left: m, Right: configuredConstant(n)}
}

func configuredCustom(t *testing.T) (ConfiguredCatalog, ConfiguredBindings, ConfiguredPrices) {
	t.Helper()
	c := ConfiguredCatalog{SchemaVersion: 2, SemanticsVersion: 1, Model: Model{Type: "X_PLUS_Y", RegularPool: Pool{Min: 1, Max: 49}, SpecialPool: Pool{Min: 1, Max: 49}, RegularCount: 6, SpecialCount: 1}, MaxLines: 1000, PriceMin: "0.01", PriceMax: "1000", Plates: []string{"new_plate"}, Domains: map[string]ConfiguredDomain{
		"novel_domain": {Options: []ConfiguredOption{{ID: "choice", Label: ConfiguredLabel{"Lucky set", "幸运组"}, Mapping: &ConfiguredMapping{Kind: "numbers", Numbers: []int{2, 7, 19, 31}}, PriceKey: "custom_price"}}},
	}, Plays: []ConfiguredPlay{{ID: "never_seen_before", Group: "operator_created", Label: ConfiguredLabel{"Lucky set", "幸运组"}, Selection: ConfiguredSelectionRule{Domain: "novel_domain", Mode: "singles", PerLine: 1, Min: 1, Max: 1}, StakeMode: "per_line", MaxMultiplier: 1, Settlement: "tiers", Rounding: "half_up", RoundingScope: "line", PushWhen: &ConfiguredCondition{Op: "equals", Left: &ConfiguredMetric{Op: "raw_value", Scope: &ConfiguredScope{Target: "regular", Positions: []int{2}}}, Right: configuredConstant(49)}, Tiers: []ConfiguredTier{{ID: "award", When: ConfiguredCondition{Op: "at_least", Left: &ConfiguredMetric{Op: "match_count", Scope: &ConfiguredScope{Target: "regular", Positions: []int{2}}, Counting: "occurrences"}, Right: configuredConstant(1)}, Price: ConfiguredPrice{Kind: "option"}, Payout: ConfiguredPayout{Kind: "fixed_return"}, Exclusive: true}}}}}
	b := ConfiguredBindings{Version: "no-attributes", Attributes: map[string]map[string][]int{}}
	p := ConfiguredPrices{Version: "operator-prices", Plates: map[string]map[string]string{"new_plate": {"custom_price": "2.5"}}}
	return c, b, p
}

func TestConfiguredNewPlayOnlyNeedsConfiguration(t *testing.T) {
	c, b, p := configuredCustom(t)
	e, err := NewConfiguredEngine(c, b, p)
	if err != nil {
		t.Fatal(err)
	}
	request := ConfiguredRequest{PlayID: "never_seen_before", Options: []string{"choice"}, Plate: "new_plate", Points: 3, Multiplier: 1}
	q, err := e.Prepare(context.Background(), []ConfiguredRequest{request})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []struct {
		number        int
		outcome       string
		prize, refund points.Amount
	}{{7, "won", 8, 0}, {8, "lost", 0, 0}, {49, "push", 0, 3}} {
		d := Draw{Regular: []int{1, 3, v.number, 4, 5, 6}, Special: []int{2}}
		out, err := e.Settle(context.Background(), q, d)
		if err != nil || out.Lines[0].Outcome != v.outcome || out.PrizePoints != v.prize || out.RefundPoints != v.refund {
			t.Fatal(v, out, err)
		}
	}
}

func TestConfiguredTypedMetricsAndCounting(t *testing.T) {
	c, b, p := configuredCustom(t)
	c.Domains["novel_domain"] = ConfiguredDomain{Options: []ConfiguredOption{
		{ID: "first", Label: ConfiguredLabel{"First", "第一组"}, Mapping: &ConfiguredMapping{Kind: "numbers", Numbers: []int{1, 2, 3}}, PriceKey: "custom_price"},
		{ID: "second", Label: ConfiguredLabel{"Second", "第二组"}, Mapping: &ConfiguredMapping{Kind: "numbers", Numbers: []int{1, 4}}, PriceKey: "custom_price"},
	}}
	c.Plays[0].PushWhen = nil
	c.Plays[0].Selection = ConfiguredSelectionRule{Domain: "novel_domain", Mode: "combinations", PerLine: 2, Min: 2, Max: 2}
	c.Plays[0].Tiers[0].Price = ConfiguredPrice{Kind: "fixed", Key: "custom_price"}
	c.Plays[0].Tiers[0].When = configuredComparison(&ConfiguredMetric{Op: "covered_option_count", Scope: &ConfiguredScope{Target: "all"}}, 2)
	e, err := NewConfiguredEngine(c, b, p)
	if err != nil {
		t.Fatal(err)
	}
	budget := &configuredBudget{ctx: context.Background(), remaining: maxTraceNodes}
	play := e.plays["never_seen_before"]
	options, err := e.selected(play, []string{"first", "second"})
	if err != nil {
		t.Fatal(err)
	}
	draw := Draw{Regular: []int{1, 2, 3, 5, 6, 7}, Special: []int{1}}
	eval := configuredEvaluation{engine: e, play: play, options: options, draw: &draw, budget: budget}
	for _, v := range []struct {
		op, counting string
		want         int
	}{{"match_count", "occurrences", 4}, {"match_count", "distinct_numbers", 3}, {"covered_option_count", "", 2}} {
		got, err := eval.metric(&ConfiguredMetric{Op: v.op, Scope: &ConfiguredScope{Target: "all"}, Counting: v.counting})
		if err != nil || got.number.Cmp(configuredScalar(v.want).number) != 0 {
			t.Fatal(v, got, err)
		}
	}
	// Raw sequences and numerical transforms have explicit signatures.
	for _, v := range []struct {
		m    ConfiguredMetric
		want int
	}{
		{ConfiguredMetric{Op: "digit_sum", Input: &ConfiguredMetric{Op: "constant", Value: configuredInt(49)}}, 13},
		{ConfiguredMetric{Op: "quotient", Parameter: 10, Input: configuredConstant(49)}, 4},
		{ConfiguredMetric{Op: "modulo", Parameter: 10, Input: configuredConstant(49)}, 9},
		{ConfiguredMetric{Op: "sum", Input: &ConfiguredMetric{Op: "numbers", Scope: &ConfiguredScope{Target: "regular"}}}, 24},
		{ConfiguredMetric{Op: "distinct_count", Input: &ConfiguredMetric{Op: "numbers", Scope: &ConfiguredScope{Target: "all"}}}, 6},
	} {
		n := 0
		if kind, err := validateConfiguredMetric(c, play, &v.m, false, 1, &n); err != nil || kind != "number" {
			t.Fatal("type", v, err)
		}
		got, err := eval.metric(&v.m)
		if err != nil || got.number.Cmp(configuredScalar(v.want).number) != 0 {
			t.Fatal(v, got, err)
		}
	}
	// Switching to the declared cross-group constraint rejects the repeated 1.
	c.DisjointGroups = true
	e, err = NewConfiguredEngine(c, b, p)
	if err != nil {
		t.Fatal(err)
	}
	q, err := e.Prepare(context.Background(), []ConfiguredRequest{{PlayID: play.ID, Options: []string{"first", "second"}, Plate: "new_plate", Points: 1, Multiplier: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.Settle(context.Background(), q, draw); err == nil {
		t.Fatal("missing declared cross-group constraint")
	}
}

func TestConfiguredStrictDefinitionsAndPrices(t *testing.T) {
	c, b, p := configuredCustom(t)
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		strings.Replace(string(raw), `"schema_version":2`, `"schema_version":2,"schema_version":2`, 1),
		strings.Replace(string(raw), `"schema_version":2`, `"schema_version":2,"unknown":1`, 1),
		strings.Replace(string(raw), `"kind":"numbers"`, `"kind":"numbers","execute":"code"`, 1),
		string(raw) + ` {}`,
	} {
		if _, err := DecodeConfiguredCatalog([]byte(bad)); err == nil {
			t.Fatal("accepted ambiguous/unknown JSON")
		}
	}
	mutations := []func(*ConfiguredCatalog){
		func(c *ConfiguredCatalog) { c.SchemaVersion = 1 },
		func(c *ConfiguredCatalog) { c.SemanticsVersion = 99 },
		func(c *ConfiguredCatalog) { c.Plays[0].Selection.Domain = "missing" },
		func(c *ConfiguredCatalog) { c.Plays[0].Tiers[0].When.Left.Op = "special_case_by_name" },
		func(c *ConfiguredCatalog) { c.Plays[0].Tiers[0].When.Left.Counting = "" },
		func(c *ConfiguredCatalog) { c.Plays[0].Tiers[0].When.Left.Scope.Positions = []int{6} },
		func(c *ConfiguredCatalog) {
			c.Plays[0].Tiers[0].When.Left = &ConfiguredMetric{Op: "raw_value", Scope: &ConfiguredScope{Target: "regular"}}
		},
		func(c *ConfiguredCatalog) {
			c.Plays[0].Tiers[0].When.Left = &ConfiguredMetric{Op: "numbers", Scope: &ConfiguredScope{Target: "regular"}}
		},
		func(c *ConfiguredCatalog) {
			c.Plays[0].Tiers[0].When.Left = &ConfiguredMetric{Op: "modulo", Parameter: 0, Input: configuredConstant(7)}
		},
		func(c *ConfiguredCatalog) {
			c.Plays[0].Tiers[0].When.Left = &ConfiguredMetric{Op: "constant", Value: configuredInt(1), Tag: "unused"}
		},
		func(c *ConfiguredCatalog) {
			c.Plays[0].Tiers[0].Price = ConfiguredPrice{Kind: "cases", Cases: []ConfiguredPriceCase{{Key: "custom_price", When: configuredComparison(&ConfiguredMetric{Op: "raw_value", Scope: &ConfiguredScope{Target: "special"}}, 7)}}}
		},
		func(c *ConfiguredCatalog) { c.Plays[0].Tiers[0].Payout = ConfiguredPayout{Kind: "execute_formula"} },
		func(c *ConfiguredCatalog) { c.Plays[0].Label.Chinese = "" },
		func(c *ConfiguredCatalog) {
			v := c.Domains["novel_domain"]
			v.Options[0].Mapping.Numbers = []int{99}
			c.Domains["novel_domain"] = v
		},
	}
	for i, change := range mutations {
		copy, err := DecodeConfiguredCatalog(raw)
		if err != nil {
			t.Fatal(err)
		}
		change(&copy)
		if err := ValidateConfiguredCatalog(copy); err == nil {
			t.Fatalf("accepted invalid definition %d", i)
		}
	}
	for _, price := range []string{"", "0", "-1", "1.1234567", "1001"} {
		bad := ConfiguredPrices{Version: p.Version, Plates: map[string]map[string]string{"new_plate": {"custom_price": price}}}
		if _, err := NewConfiguredEngine(c, b, bad); err == nil {
			t.Fatal("accepted price", price)
		}
	}
	delete(p.Plates["new_plate"], "custom_price")
	if _, err := NewConfiguredEngine(c, b, p); err == nil {
		t.Fatal("filled missing price")
	}
}

func TestConfiguredRuntimeRejectionsAndBudget(t *testing.T) {
	c, b, p, e, _ := configuredFixture(t)
	sources := configuredSources(c)
	r := configuredRequestForSource(t, c, sources["linked_2"], []int{1, 2}, "A", 1)
	for _, change := range []func(*ConfiguredRequest){
		func(r *ConfiguredRequest) { r.Options = []string{"1", "1"} }, func(r *ConfiguredRequest) { r.Options = []string{"0", "1"} }, func(r *ConfiguredRequest) { r.Options = []string{"1"} }, func(r *ConfiguredRequest) { r.PlayID = "unknown" }, func(r *ConfiguredRequest) { r.Plate = "C" }, func(r *ConfiguredRequest) { r.Points = 0 }, func(r *ConfiguredRequest) { r.Multiplier = 0 },
	} {
		bad := r
		change(&bad)
		if _, err := e.Prepare(context.Background(), []ConfiguredRequest{bad}); err == nil {
			t.Fatal("accepted invalid request", bad)
		}
	}
	if _, err := e.Prepare(context.Background(), []ConfiguredRequest{r, r}); err == nil {
		t.Fatal("duplicate batch line")
	}
	r = configuredRequestForSource(t, c, sources["linked_4"], []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14}, "A", 1)
	if _, err := e.Prepare(context.Background(), []ConfiguredRequest{r}); !errors.Is(err, ErrLimit) {
		t.Fatal("1001 combinations", err)
	}
	r = configuredRequestForSource(t, c, sources["parlay"], []int{100, 200}, "A", 1)
	r.Options = []string{"position_attribute_1_big", "position_attribute_1_odd"}
	if _, err := e.Prepare(context.Background(), []ConfiguredRequest{r}); err == nil {
		t.Fatal("two legs on one position")
	}
	r = configuredRequestForSource(t, c, sources["number_01"], nil, "A", points.Amount(math.MaxInt64))
	if _, err := e.Prepare(context.Background(), []ConfiguredRequest{r}); !errors.Is(err, points.ErrOverflow) {
		t.Fatal("overflow must fail before settlement", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := e.Prepare(ctx, []ConfiguredRequest{r}); !errors.Is(err, context.Canceled) {
		t.Fatal("ignored cancellation", err)
	}
	// Category overlap and no match are explicit errors, not first-match/default.
	for _, matches := range []int{0, 2} {
		cc, bb, pp := configuredCustom(t)
		condition := configuredComparison(configuredConstant(1), matches/2)
		cc.Plays[0].Tiers[0].Price = ConfiguredPrice{Kind: "cases", Cases: []ConfiguredPriceCase{{When: condition, Key: "custom_price"}, {When: condition, Key: "custom_price"}}}
		ee, err := NewConfiguredEngine(cc, bb, pp)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ee.Prepare(context.Background(), []ConfiguredRequest{{PlayID: "never_seen_before", Options: []string{"choice"}, Plate: "new_plate", Points: 1, Multiplier: 1}}); err == nil {
			t.Fatal("ambiguous pricing", matches)
		}
	}
	// A missing mapping cannot be copied from a different version/group.
	delete(b.Attributes["zodiac"], "horse")
	if _, err := NewConfiguredEngine(c, b, p); err == nil {
		t.Fatal("accepted incomplete attributes")
	}
}

func TestConfiguredRoundingAndTiersReuseExistingEngine(t *testing.T) {
	for _, scope := range []string{"line", "tier", "order"} {
		for _, policy := range []string{"exclusive", "additive", "max_all", "max_exclusive_plus_additive"} {
			for _, useCap := range []bool{false, true} {
				c, b, p := configuredCustom(t)
				play := &c.Plays[0]
				play.PushWhen = nil
				play.Selection.Max = 2
				play.StakeMode = "unit_multiplier"
				play.UnitPoints = 1
				play.MaxMultiplier = 10
				play.RoundingScope = scope
				d := c.Domains["novel_domain"]
				d.Options = append(d.Options, ConfiguredOption{ID: "other", Label: ConfiguredLabel{"Other", "另一项"}, Mapping: &ConfiguredMapping{Kind: "numbers", Numbers: []int{8}}, PriceKey: "custom_price"})
				c.Domains["novel_domain"] = d
				condition := configuredComparison(configuredConstant(1), 1)
				play.Tiers = []ConfiguredTier{{ID: "t0", When: condition, Price: ConfiguredPrice{Kind: "fixed", Key: "a"}, Payout: ConfiguredPayout{Kind: "fixed_return"}, Exclusive: policy != "additive"}, {ID: "t1", When: condition, Price: ConfiguredPrice{Kind: "fixed", Key: "b"}, Payout: ConfiguredPayout{Kind: "fixed_return"}, Exclusive: policy == "exclusive"}}
				if policy == "max_all" || policy == "max_exclusive_plus_additive" {
					play.MixedPolicy = policy
				}
				if useCap {
					cap := points.Amount(1)
					play.Cap = &cap
				}
				p.Plates["new_plate"] = map[string]string{"a": "0.49", "b": "0.51"}
				e, err := NewConfiguredEngine(c, b, p)
				if err != nil {
					t.Fatal(err)
				}
				q, err := e.Prepare(context.Background(), []ConfiguredRequest{{PlayID: play.ID, Options: []string{"choice", "other"}, Plate: "new_plate", Multiplier: 1}})
				if err != nil {
					t.Fatal(err)
				}
				draw := Draw{Regular: []int{1, 2, 3, 4, 5, 6}, Special: []int{7}}
				got, err := e.Settle(context.Background(), q, draw)
				if err != nil {
					t.Fatal(err)
				}
				always := Condition{Op: "between", Field: "special_match", Min: configuredInt(0), Max: configuredInt(1)}
				legacy := Definition{SchemaVersion: 1, Model: c.Model, Selection: SelectionRule{Mode: "numbers", SpecialCount: 1}, UnitPoints: 1, PrizeTiers: []Tier{{Code: "a", Condition: always, Odds: "0.49", Exclusive: play.Tiers[0].Exclusive}, {Code: "b", Condition: always, Odds: "0.51", Exclusive: play.Tiers[1].Exclusive}}, MixedTierPolicy: play.MixedPolicy, CapPoints: play.Cap, Rounding: "half_up", RoundingScope: scope, Limits: Limits{MaxCombinations: 1000, MaxMultiplier: 10}}
				want, err := Simulate(SimulationInput{Definition: legacy, Selection: Selection{Special: []int{7, 8}}, Draw: draw, Multiplier: 1})
				if err != nil {
					t.Fatal(err)
				}
				if got.BetPoints != want.BetPoints || got.PrizePoints != want.PrizePoints {
					t.Fatalf("%s %s cap=%v got=%+v want=%+v", scope, policy, useCap, got, want)
				}
			}
		}
	}
}

func TestConfiguredSnapshotCannotBeMutatedThroughReturnedBytes(t *testing.T) {
	c, b, p := configuredCustom(t)
	e, err := NewConfiguredEngine(c, b, p)
	if err != nil {
		t.Fatal(err)
	}
	raw := e.Snapshot()
	original := e.Snapshot()
	raw[0] = 'x'
	if !reflect.DeepEqual(original, e.Snapshot()) {
		t.Fatal("snapshot returned mutable backing bytes")
	}
}

func TestConfiguredLimitsCapsAndPriceCeilings(t *testing.T) {
	c, b, p := configuredCustom(t)
	c.Plays[0].PushWhen = nil
	c.Plays[0].Tiers[0].When = configuredComparison(configuredConstant(1), 1)
	d := c.Domains["novel_domain"]
	d.Options = append(d.Options, ConfiguredOption{ID: "other", Label: ConfiguredLabel{"Other", "另一项"}, Mapping: &ConfiguredMapping{Kind: "numbers", Numbers: []int{8}}, PriceKey: "custom_price"})
	c.Domains["novel_domain"] = d
	c.Plays[0].Selection.Max = 2
	p.Plates["new_plate"]["custom_price"] = "2"
	e, err := NewConfiguredEngine(c, b, p)
	if err != nil {
		t.Fatal(err)
	}
	request := ConfiguredRequest{PlayID: c.Plays[0].ID, Options: []string{"choice", "other"}, Plate: "new_plate", Points: points.Amount(math.MaxInt64 / 3), Multiplier: 1}
	if _, err := e.Prepare(context.Background(), []ConfiguredRequest{request}); !errors.Is(err, points.ErrOverflow) {
		t.Fatal("aggregate maximum prize overflow", err)
	}
	// Total cap precedes overflow checks, including sums bigger than int64.
	cap := points.Amount(5)
	c.Plays[0].Cap = &cap
	p.Plates["new_plate"]["custom_price"] = "1000"
	e, err = NewConfiguredEngine(c, b, p)
	if err != nil {
		t.Fatal(err)
	}
	q, err := e.Prepare(context.Background(), []ConfiguredRequest{request})
	if err != nil {
		t.Fatal(err)
	}
	out, err := e.Settle(context.Background(), q, Draw{Regular: []int{1, 2, 3, 4, 5, 6}, Special: []int{7}})
	if err != nil || out.PrizePoints != 5 {
		t.Fatal("cap before overflow", out, err)
	}
	if err := e.CheckPriceCeilings(p); err != nil {
		t.Fatal(err)
	}
	p.Plates["new_plate"]["custom_price"] = "999.999999"
	if err := e.CheckPriceCeilings(p); err == nil {
		t.Fatal("brand exceeds platform")
	}
	delete(p.Plates["new_plate"], "custom_price")
	if err := e.CheckPriceCeilings(p); err == nil {
		t.Fatal("missing ceiling")
	}
	// Unknown plates are not silently enabled by adding a complete price book.
	c, b, p = configuredCustom(t)
	p.Plates["unconfigured"] = map[string]string{"custom_price": "2"}
	if _, err := NewConfiguredEngine(c, b, p); err == nil {
		t.Fatal("undeclared plate")
	}
	// Depth/work checks reject excessive expression trees before execution.
	c, b, p = configuredCustom(t)
	condition := configuredComparison(configuredConstant(1), 1)
	for i := 0; i < maxConditionDepth; i++ {
		condition = ConfiguredCondition{Op: "not", Children: []ConfiguredCondition{condition}}
	}
	c.Plays[0].Tiers[0].When = condition
	if err := ValidateConfiguredCatalog(c); !errors.Is(err, ErrLimit) {
		t.Fatal("condition depth", err)
	}
	// Result metric and option mapping signatures cannot be interchanged.
	c, b, p = configuredCustom(t)
	d = c.Domains["novel_domain"]
	d.Options[0].Mapping = &ConfiguredMapping{Kind: "transform", Transform: "digit_sum", Value: configuredInt(13)}
	c.Domains["novel_domain"] = d
	e, err = NewConfiguredEngine(c, b, p)
	if err != nil {
		t.Fatal(err)
	}
	request = ConfiguredRequest{PlayID: c.Plays[0].ID, Options: []string{"choice"}, Plate: "new_plate", Points: 1, Multiplier: 1}
	q, err = e.Prepare(context.Background(), []ConfiguredRequest{request})
	if err != nil {
		t.Fatal(err)
	}
	out, err = e.Settle(context.Background(), q, Draw{Regular: []int{1, 2, 49, 4, 5, 6}, Special: []int{7}})
	if err != nil || out.RefundPoints != 1 {
		t.Fatal("push before mapping match", err)
	}
}

func TestConfiguredConcurrentEvaluation(t *testing.T) {
	c, b, p := configuredCustom(t)
	e, err := NewConfiguredEngine(c, b, p)
	if err != nil {
		t.Fatal(err)
	}
	q, err := e.Prepare(context.Background(), []ConfiguredRequest{{PlayID: c.Plays[0].ID, Options: []string{"choice"}, Plate: "new_plate", Points: 3, Multiplier: 1}})
	if err != nil {
		t.Fatal(err)
	}
	draw := Draw{Regular: []int{1, 3, 7, 4, 5, 6}, Special: []int{2}}
	results := make(chan error, 16)
	for i := 0; i < 16; i++ {
		go func() {
			for j := 0; j < 20; j++ {
				out, err := e.Settle(context.Background(), q, draw)
				if err != nil {
					results <- err
					return
				}
				if out.PrizePoints != 8 {
					results <- fmt.Errorf("unexpected prize %d", out.PrizePoints)
					return
				}
			}
			results <- nil
		}()
	}
	for i := 0; i < 16; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
}
