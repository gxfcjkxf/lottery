package rules

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"math/big"
	"reflect"
	"slices"
	"strconv"

	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

type ConfiguredSnapshot struct {
	Catalog  ConfiguredCatalog  `json:"catalog"`
	Bindings ConfiguredBindings `json:"bindings"`
	Prices   ConfiguredPrices   `json:"prices"`
}

// ConfiguredEngine owns a deep copy of its resolved snapshot. Restore the saved
// snapshot for historical calculations, never a current price/attribute lookup.
type ConfiguredEngine struct {
	snapshot        ConfiguredSnapshot
	raw             []byte
	hash            string
	definitionHash  string
	plays           map[string]ConfiguredPlay
	options         map[string]map[string]ConfiguredOption
	attributeValues map[string]map[int][]string
}

func NewConfiguredEngine(c ConfiguredCatalog, b ConfiguredBindings, p ConfiguredPrices) (*ConfiguredEngine, error) {
	raw, err := json.Marshal(ConfiguredSnapshot{c, b, p})
	if err != nil {
		return nil, err
	}
	return RestoreConfiguredEngine(raw)
}

func RestoreConfiguredEngine(raw []byte) (*ConfiguredEngine, error) {
	e := &ConfiguredEngine{}
	if err := decodeConfigured(raw, &e.snapshot); err != nil {
		return nil, err
	}
	c := e.snapshot.Catalog
	if err := ValidateConfiguredCatalog(c); err != nil {
		return nil, err
	}
	if !configuredID(e.snapshot.Bindings.Version) || !configuredID(e.snapshot.Prices.Version) {
		return nil, ErrInvalid
	}
	e.plays = map[string]ConfiguredPlay{}
	e.options = map[string]map[string]ConfiguredOption{}
	e.attributeValues = map[string]map[int][]string{}
	for _, p := range c.Plays {
		e.plays[p.ID] = p
	}
	for id, domain := range c.Domains {
		e.options[id] = map[string]ConfiguredOption{}
		for _, o := range domain.Options {
			e.options[id][o.ID] = o
		}
	}
	pool, err := unionPoolValues(c.Model)
	if err != nil {
		return nil, err
	}
	if len(e.snapshot.Bindings.Attributes) != len(c.Attributes) {
		return nil, ErrInvalid
	}
	for _, attr := range c.Attributes {
		bindings, ok := e.snapshot.Bindings.Attributes[attr.ID]
		if !ok || len(bindings) != len(attr.Labels) {
			return nil, ErrInvalid
		}
		values := map[int][]string{}
		for _, label := range attr.Labels {
			numbers, ok := bindings[label]
			if !ok || len(numbers) > maxDSLItems || hasDuplicates(numbers) {
				return nil, ErrInvalid
			}
			for _, n := range numbers {
				if !slices.Contains(pool, n) {
					return nil, ErrInvalid
				}
				values[n] = append(values[n], label)
			}
		}
		for _, n := range pool {
			if len(values[n]) == 0 || attr.Partition && len(values[n]) != 1 {
				return nil, ErrInvalid
			}
		}
		for key, label := range attr.Fixed {
			n, _ := strconv.Atoi(key) // canonical number keys checked with the definition
			if !slices.Contains(values[n], label) {
				return nil, ErrInvalid
			}
		}
		e.attributeValues[attr.ID] = values
	}
	keys := e.PriceKeys()
	lo, _ := parseOdds(c.PriceMin)
	hi, _ := parseOdds(c.PriceMax)
	if len(e.snapshot.Prices.Plates) != len(c.Plates) {
		return nil, ErrInvalid
	}
	for plate, book := range e.snapshot.Prices.Plates {
		if !slices.Contains(c.Plates, plate) || len(book) != len(keys) {
			return nil, ErrInvalid
		}
		for _, key := range keys {
			v, err := parseOdds(book[key])
			if err != nil || v.Cmp(lo) < 0 || v.Cmp(hi) > 0 {
				return nil, ErrInvalid
			}
		}
	}
	e.raw, err = json.Marshal(e.snapshot)
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(e.raw)
	e.hash = hex.EncodeToString(hash[:])
	definitionRaw, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	definitionHash := sha256.Sum256(definitionRaw)
	e.definitionHash = hex.EncodeToString(definitionHash[:])
	return e, nil
}

func (e *ConfiguredEngine) Snapshot() []byte { return append([]byte(nil), e.raw...) }
func (e *ConfiguredEngine) DefaultSettlementContext() ConfiguredSettlementContext {
	return DefaultConfiguredSettlementContext(e.snapshot.Catalog)
}
func (e *ConfiguredEngine) SnapshotHash() string   { return e.hash }
func (e *ConfiguredEngine) DefinitionHash() string { return e.definitionHash }

// CheckPriceCeilings is a pure exact comparison for the later brand-sales
// integration. Both books must cover the same declared plates and price cells.
func (e *ConfiguredEngine) CheckPriceCeilings(ceilings ConfiguredPrices) error {
	if !configuredID(ceilings.Version) || len(ceilings.Plates) != len(e.snapshot.Prices.Plates) {
		return ErrInvalid
	}
	lo, _ := parseOdds(e.snapshot.Catalog.PriceMin)
	hi, _ := parseOdds(e.snapshot.Catalog.PriceMax)
	for plate, book := range e.snapshot.Prices.Plates {
		ceiling, ok := ceilings.Plates[plate]
		if !ok || len(ceiling) != len(book) {
			return ErrInvalid
		}
		for key, value := range book {
			limit, err := parseOdds(ceiling[key])
			if err != nil {
				return err
			}
			if limit.Cmp(lo) < 0 || limit.Cmp(hi) > 0 {
				return ErrInvalid
			}
			actual, err := parseOdds(value)
			if err != nil {
				return err
			}
			if actual.Cmp(limit) > 0 {
				return ErrInvalid
			}
		}
	}
	return nil
}
func (e *ConfiguredEngine) PriceKeys() []string {
	keys := map[string]bool{}
	for _, p := range e.snapshot.Catalog.Plays {
		if p.Decisions != nil {
			for _, v := range p.Decisions.Outcomes {
				if v.PriceKey != "" {
					keys[v.PriceKey] = true
				}
			}
		}
		for _, t := range p.Tiers {
			switch t.Price.Kind {
			case "fixed":
				keys[t.Price.Key] = true
			case "option":
				for _, o := range e.snapshot.Catalog.Domains[p.Selection.Domain].Options {
					keys[o.PriceKey] = true
				}
			case "cases":
				for _, v := range t.Price.Cases {
					keys[v.Key] = true
				}
			}
		}
	}
	out := make([]string, 0, len(keys))
	for key := range keys {
		out = append(out, key)
	}
	slices.Sort(out)
	return out
}

func (e *ConfiguredEngine) selected(p ConfiguredPlay, ids []string) ([]ConfiguredOption, error) {
	seen := map[string]bool{}
	slots := map[string]bool{}
	for _, id := range ids {
		o, ok := e.options[p.Selection.Domain][id]
		if !ok || seen[id] || p.Selection.DistinctSlot && slots[o.Slot] {
			return nil, ErrInvalid
		}
		seen[id] = true
		slots[o.Slot] = true
	}
	out := make([]ConfiguredOption, 0, len(ids))
	for _, o := range e.snapshot.Catalog.Domains[p.Selection.Domain].Options {
		if seen[o.ID] {
			out = append(out, o)
		}
	}
	return out, nil
}

func configuredOptionIDs(options []ConfiguredOption) []string {
	out := make([]string, len(options))
	for i, o := range options {
		out[i] = o.ID
	}
	return out
}

func (e *ConfiguredEngine) price(p ConfiguredPlay, options []ConfiguredOption, plate string, s ConfiguredPrice, budget *configuredBudget) (string, error) {
	key := ""
	switch s.Kind {
	case "fixed":
		key = s.Key
	case "option":
		if len(options) != 1 {
			return "", ErrInvalid
		}
		key = options[0].PriceKey
	case "cases":
		eval := configuredEvaluation{engine: e, play: p, options: options, budget: budget}
		matches := 0
		for _, c := range s.Cases {
			trace, err := eval.condition(c.When)
			if err != nil {
				return "", err
			}
			if trace.Matched {
				matches++
				key = c.Key
			}
		}
		if matches != 1 {
			return "", ErrInvalid
		}
	default:
		return "", ErrInvalid
	}
	value, ok := e.snapshot.Prices.Plates[plate][key]
	if !ok {
		return "", ErrInvalid
	}
	return value, nil
}

func (e *ConfiguredEngine) prepareLine(p ConfiguredPlay, options []ConfiguredOption, plate string, stake points.Amount, budget *configuredBudget) (ConfiguredQuoteLine, error) {
	line := ConfiguredQuoteLine{PlayID: p.ID, Options: configuredOptionIDs(options), Plate: plate, Stake: stake, Prices: map[string]string{}}
	if p.Decisions != nil {
		for _, v := range p.Decisions.Outcomes {
			if v.PriceKey != "" {
				line.Prices[v.PriceKey] = e.snapshot.Prices.Plates[plate][v.PriceKey]
			}
		}
	}
	if p.Settlement == "product_of_legs" {
		for _, o := range options {
			ref := o.Reference
			leg := e.plays[ref.PlayID]
			selected, err := e.selected(leg, []string{ref.OptionID})
			if err != nil {
				return line, err
			}
			v, err := e.prepareLine(leg, selected, plate, stake, budget)
			if err != nil {
				return line, err
			}
			line.Legs = append(line.Legs, v)
		}
	} else {
		for _, t := range p.Tiers {
			price, err := e.price(p, options, plate, t.Price, budget)
			if err != nil {
				return line, err
			}
			line.Prices[t.ID] = price
		}
	}
	maximum, err := e.maximumLine(p, line)
	if err != nil {
		return line, err
	}
	if _, err = exactPoints(maximum, nil); err != nil {
		return line, err
	}
	return line, nil
}

func configuredCountBound(c ConfiguredCatalog, p ConfiguredPlay, m *ConfiguredMetric) (int, error) {
	if m == nil {
		return 0, ErrInvalid
	}
	switch m.Op {
	case "constant":
		return *m.Value, nil
	case "selected_count":
		if p.Selection.Mode == "as_selected" {
			return p.Selection.Max, nil
		}
		return p.Selection.PerLine, nil
	case "match_count":
		n, err := targetCount(c.Model, m.Scope.Target)
		if len(m.Scope.Positions) > 0 {
			n = len(m.Scope.Positions)
		}
		return n, err
	case "covered_option_count":
		if p.Selection.Mode == "as_selected" {
			return p.Selection.Max, nil
		}
		return p.Selection.PerLine, nil
	default:
		return 0, ErrInvalid
	}
}

func configuredCap(v *big.Rat, cap *points.Amount) *big.Rat {
	out := new(big.Rat).Set(v)
	if cap != nil && out.Cmp(big.NewRat(int64(*cap), 1)) > 0 {
		out.SetInt64(int64(*cap))
	}
	return out
}
func configuredPayout(stake points.Amount, price string, kind string, count int) (*big.Rat, error) {
	o, err := parseOdds(price)
	if err != nil {
		return nil, err
	}
	if kind == "net_profit_per_hit" {
		if count < 1 {
			return nil, ErrInvalid
		}
		o.Sub(o, big.NewRat(1, 1))
		o.Mul(o, big.NewRat(int64(count), 1))
		o.Add(o, big.NewRat(1, 1))
	} else if kind != "fixed_return" {
		return nil, ErrInvalid
	}
	return o.Mul(o, big.NewRat(int64(stake), 1)), nil
}

func (e *ConfiguredEngine) maximumLine(p ConfiguredPlay, line ConfiguredQuoteLine) (*big.Rat, error) {
	if p.Decisions != nil {
		return e.decisionMaximum(p, line)
	}
	if p.Settlement == "product_of_legs" {
		out := big.NewRat(int64(line.Stake), 1)
		for _, leg := range line.Legs {
			for _, price := range leg.Prices {
				o, err := parseOdds(price)
				if err != nil {
					return nil, err
				}
				if o.Cmp(big.NewRat(1, 1)) < 0 {
					o.SetInt64(1)
				}
				out.Mul(out, o)
			}
		}
		return out, nil
	}
	amounts := make([]*big.Rat, len(p.Tiers))
	matched := make([]bool, len(p.Tiers))
	exclusive := make([]bool, len(p.Tiers))
	for i, t := range p.Tiers {
		count := 1
		if t.Payout.Kind == "net_profit_per_hit" {
			var err error
			count, err = configuredCountBound(e.snapshot.Catalog, p, t.Payout.Count)
			if err != nil {
				return nil, err
			}
		}
		v, err := configuredPayout(line.Stake, line.Prices[t.ID], t.Payout.Kind, count)
		if err != nil {
			return nil, err
		}
		v = configuredCap(v, t.Cap)
		if p.RoundingScope == "tier" {
			v.SetInt(roundedInteger(v))
		}
		amounts[i] = v
		matched[i] = true
		exclusive[i] = t.Exclusive
	}
	// Conservative upper bound: mixed max_all can pay additive tiers alone when
	// no exclusive tier hits, so sum all nonexclusive amounts as well.
	selected := selectTierAmounts(amounts, matched, exclusive, p.MixedPolicy)
	out := new(big.Rat)
	additive := new(big.Rat)
	for i, v := range amounts {
		if selected[i] {
			out.Add(out, v)
		}
		if !exclusive[i] {
			additive.Add(additive, v)
		}
	}
	if additive.Cmp(out) > 0 {
		out = additive
	}
	return configuredCap(out, p.Cap), nil
}

func (e *ConfiguredEngine) Prepare(ctx context.Context, requests []ConfiguredRequest) (ConfiguredQuote, error) {
	q := ConfiguredQuote{DefinitionHash: e.definitionHash, SnapshotHash: e.hash, Lines: []ConfiguredQuoteLine{}}
	budget := &configuredBudget{ctx: ctx, remaining: maxTraceNodes}
	if err := budget.use(0); err != nil {
		return q, err
	}
	if len(requests) < 1 || len(requests) > e.snapshot.Catalog.MaxLines {
		return q, ErrLimit
	}
	seen := map[string]bool{}
	for _, r := range requests {
		if err := budget.use(1); err != nil {
			return q, err
		}
		p, ok := e.plays[r.PlayID]
		if !ok {
			return q, ErrInvalid
		}
		if _, ok := e.snapshot.Prices.Plates[r.Plate]; !ok {
			return q, ErrInvalid
		}
		if len(r.Options) < p.Selection.Min || len(r.Options) > p.Selection.Max || r.Multiplier < 1 || r.Multiplier > p.MaxMultiplier {
			return q, ErrInvalid
		}
		stake := r.Points
		if p.StakeMode == "unit_multiplier" {
			if r.Points != 0 {
				return q, ErrInvalid
			}
			v := new(big.Int).Mul(big.NewInt(int64(p.UnitPoints)), big.NewInt(int64(r.Multiplier)))
			if !v.IsInt64() {
				return q, points.ErrOverflow
			}
			stake = points.Amount(v.Int64())
		} else if r.Multiplier != 1 {
			return q, ErrInvalid
		}
		if stake <= 0 {
			return q, ErrInvalid
		}
		options, err := e.selected(p, r.Options)
		if err != nil {
			return q, err
		}
		indices := make([]int, len(options))
		for i := range indices {
			indices[i] = i
		}
		var lines [][]int
		if p.Selection.Mode == "as_selected" {
			lines = [][]int{indices}
		} else {
			lines, err = pickLines(indices, p.Selection.PerLine, false, false, e.snapshot.Catalog.MaxLines-len(q.Lines))
			if err != nil {
				return q, err
			}
		}
		if len(lines)+len(q.Lines) > e.snapshot.Catalog.MaxLines {
			return q, ErrLimit
		}
		for _, indices := range lines {
			selected := make([]ConfiguredOption, len(indices))
			for i, v := range indices {
				selected[i] = options[v]
			}
			keyRaw, _ := json.Marshal(struct {
				Play, Plate string
				Options     []string
			}{p.ID, r.Plate, configuredOptionIDs(selected)})
			if seen[string(keyRaw)] {
				return q, ErrInvalid
			}
			seen[string(keyRaw)] = true
			line, err := e.prepareLine(p, selected, r.Plate, stake, budget)
			if err != nil {
				return q, err
			}
			if int64(stake) > math.MaxInt64-int64(q.BetPoints) {
				return q, points.ErrOverflow
			}
			if e.snapshot.Catalog.Funding == "reserve" {
				if p.RiskMultiplier < 1 || int64(stake) > math.MaxInt64/int64(p.RiskMultiplier) {
					return q, ErrLimit
				}
				amount := stake * points.Amount(p.RiskMultiplier)
				if int64(amount) > math.MaxInt64-int64(q.ReservedPoints) {
					return q, ErrLimit
				}
				q.ReservedPoints += amount
			}
			q.BetPoints += stake
			q.Lines = append(q.Lines, line)
		}
		r.Options = append([]string(nil), r.Options...)
		q.Requests = append(q.Requests, r)
	}
	// Bound the complete batch too: individually representable prizes can still
	// overflow when added. Caps and rounding retain the same grouping as Settle.
	maximums := map[string]*big.Rat{}
	maximumCaps := map[string]*points.Amount{}
	for _, line := range q.Lines {
		p := e.plays[line.PlayID]
		keyRaw, _ := json.Marshal([]string{line.PlayID, line.Plate})
		key := string(keyRaw)
		v, err := e.maximumLine(p, line)
		if err != nil {
			return q, err
		}
		if p.RoundingScope != "order" {
			v.SetInt(roundedInteger(v))
		}
		if maximums[key] == nil {
			maximums[key] = new(big.Rat)
		}
		maximums[key].Add(maximums[key], v)
		maximumCaps[key] = p.Cap
	}
	maximum := new(big.Int)
	for key, v := range maximums {
		paid, err := exactPoints(v, maximumCaps[key])
		if err != nil {
			return q, err
		}
		maximum.Add(maximum, big.NewInt(int64(paid)))
	}
	if !maximum.IsInt64() {
		return q, points.ErrOverflow
	}
	return q, nil
}

func (e *ConfiguredEngine) settleLine(line ConfiguredQuoteLine, draw Draw, budget *configuredBudget) (ConfiguredLineResult, error) {
	p := e.plays[line.PlayID]
	out := ConfiguredLineResult{PlayID: p.ID, Options: append([]string(nil), line.Options...), Outcome: "lost", Raw: "0", Effective: line.Stake}
	options, err := e.selected(p, line.Options)
	if err != nil {
		return out, err
	}
	eval := configuredEvaluation{engine: e, play: p, options: options, draw: &draw, budget: budget}
	if p.Settlement == "product_of_legs" {
		value := big.NewRat(int64(line.Stake), 1)
		lost := false
		active := 0
		for _, leg := range line.Legs {
			v, err := e.settleLine(leg, draw, budget)
			if err != nil {
				return out, err
			}
			out.Legs = append(out.Legs, v)
			if v.Outcome == "lost" {
				lost = true
			}
			if v.Outcome == "won" {
				active++
				for _, price := range leg.Prices {
					o, err := parseOdds(price)
					if err != nil {
						return out, err
					}
					value.Mul(value, o)
				}
			}
		}
		if lost {
			return out, nil
		}
		if active == 0 {
			return out, ErrInvalid
		}
		out.Outcome = "won"
		out.Raw = value.RatString()
		out.Prize, err = exactPoints(value, nil)
		return out, err
	}
	if p.PushWhen != nil {
		trace, err := eval.condition(*p.PushWhen)
		if err != nil {
			return out, err
		}
		out.PushTrace = &trace
		if trace.Matched {
			out.Outcome = "push"
			out.Effective = 0
			out.Refund = line.Stake
			out.Raw = big.NewInt(int64(line.Stake)).String()
			return out, nil
		}
	}
	amounts := make([]*big.Rat, len(p.Tiers))
	matched := make([]bool, len(p.Tiers))
	exclusive := make([]bool, len(p.Tiers))
	for i, t := range p.Tiers {
		trace, err := eval.condition(t.When)
		if err != nil {
			return out, err
		}
		matched[i] = trace.Matched
		exclusive[i] = t.Exclusive
		raw := new(big.Rat)
		if trace.Matched {
			count := 1
			if t.Payout.Count != nil {
				n, err := eval.metric(t.Payout.Count)
				if err != nil {
					return out, err
				}
				if n.kind != "number" || !n.number.IsInt() || !n.number.Num().IsInt64() {
					return out, ErrInvalid
				}
				count = int(n.number.Num().Int64())
			}
			raw, err = configuredPayout(line.Stake, line.Prices[t.ID], t.Payout.Kind, count)
			if err != nil {
				return out, err
			}
		}
		v := configuredCap(raw, t.Cap)
		out.Tiers = append(out.Tiers, ConfiguredTierResult{ID: t.ID, Matched: trace.Matched, Price: line.Prices[t.ID], Raw: raw.RatString(), Capped: v.RatString(), Trace: trace})
		if p.RoundingScope == "tier" {
			v.SetInt(roundedInteger(v))
		}
		amounts[i] = v
	}
	selected := selectTierAmounts(amounts, matched, exclusive, p.MixedPolicy)
	total := new(big.Rat)
	for i, yes := range selected {
		out.Tiers[i].Selected = yes
		if yes {
			out.Outcome = "won"
			total.Add(total, amounts[i])
		}
	}
	out.Raw = total.RatString()
	out.Prize, err = exactPoints(total, p.Cap)
	return out, err
}

func (e *ConfiguredEngine) ValidateDraw(draw Draw) error {
	if err := ValidateDraw(e.snapshot.Catalog.Model, draw); err != nil {
		return err
	}
	if e.snapshot.Catalog.DisjointGroups && intersects(draw.Regular, draw.Special) {
		return ErrInvalid
	}
	return nil
}

func (e *ConfiguredEngine) Settle(ctx context.Context, q ConfiguredQuote, draw Draw) (ConfiguredResult, error) {
	return e.SettleWithContext(ctx, q, draw, ConfiguredSettlementContext{})
}
func (e *ConfiguredEngine) SettleWithContext(ctx context.Context, q ConfiguredQuote, draw Draw, settlement ConfiguredSettlementContext) (ConfiguredResult, error) {
	out := ConfiguredResult{Lines: []ConfiguredLineResult{}}
	if q.SnapshotHash != e.hash {
		return out, ErrInvalid
	}
	// Re-derive the quote using the immutable purchase snapshot. Caller-supplied
	// prices, stakes, legs, counts and normalized lines are never trusted.
	expected, err := e.Prepare(ctx, q.Requests)
	if err != nil {
		return out, err
	}
	if !reflect.DeepEqual(expected, q) {
		return out, ErrInvalid
	}
	if err := e.ValidateDraw(draw); err != nil {
		return out, err
	}
	out.BetPoints = q.BetPoints
	budget := &configuredBudget{ctx: ctx, remaining: maxTraceNodes}
	// Order rounding is accumulated per play/plate/request-independent definition;
	// batches may contain unrelated plays, so their rounding never mixes.
	orderTotals := map[string]*big.Rat{}
	orderDefinitions := map[string]ConfiguredRounding{}
	for _, line := range q.Lines {
		if err := budget.use(1); err != nil {
			return out, err
		}
		var v ConfiguredLineResult
		var err error
		if p := e.plays[line.PlayID]; p.Decisions != nil {
			v, err = e.settleDecision(p, line, draw, settlement)
		} else {
			v, err = e.settleLine(line, draw, budget)
		}
		if err != nil {
			return out, err
		}
		p := e.plays[line.PlayID]
		if v.Outcome != "push" {
			keyBytes, _ := json.Marshal([]string{p.ID, line.Plate})
			key := string(keyBytes)
			if orderTotals[key] == nil {
				orderTotals[key] = new(big.Rat)
			}
			raw, ok := new(big.Rat).SetString(v.Raw)
			if !ok {
				return out, ErrInvalid
			}
			if p.RoundingScope != "order" {
				raw.SetInt(roundedInteger(raw))
			}
			orderTotals[key].Add(orderTotals[key], raw)
			orderDefinitions[key] = ConfiguredRounding{PlayID: p.ID, Plate: line.Plate}
		}
		out.RefundPoints += v.Refund
		out.EffectivePoints += v.Effective
		out.Lines = append(out.Lines, v)
	}
	keys := make([]string, 0, len(orderTotals))
	for key := range orderTotals {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		v := orderTotals[key]
		summary := orderDefinitions[key]
		cap := e.plays[summary.PlayID].Cap
		prize, err := exactPoints(v, cap)
		if err != nil {
			return out, err
		}
		if int64(prize) > math.MaxInt64-int64(out.PrizePoints) {
			return out, points.ErrOverflow
		}
		out.PrizePoints += prize
		summary.Raw, summary.Capped, summary.Prize = v.RatString(), configuredCap(v, cap).RatString(), prize
		out.Rounding = append(out.Rounding, summary)
	}
	if e.snapshot.Catalog.PeriodPrizeCap != nil {
		if settlement.PrizeRemaining == nil || *settlement.PrizeRemaining < 0 || *settlement.PrizeRemaining > *e.snapshot.Catalog.PeriodPrizeCap {
			return out, ErrInvalid
		}
		remaining := *settlement.PrizeRemaining
		out.PrizePoints = 0
		for i := range out.Lines {
			v := &out.Lines[i]
			if v.Prize > remaining {
				v.Prize = remaining
			}
			remaining -= v.Prize
			out.PrizePoints += v.Prize
		}
	}
	if e.snapshot.Catalog.Funding == "reserve" || len(e.snapshot.Catalog.Aggregates) > 0 {
		out.ConsumedPoints = e.FundingAmount(q) - out.RefundPoints
		if out.ConsumedPoints < 0 {
			return out, ErrInvalid
		}
	}
	return out, nil
}
