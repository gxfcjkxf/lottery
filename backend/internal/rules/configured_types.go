package rules

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

// ConfiguredCatalog is the schema-v2 pure calculation contract. Business IDs and
// source codes are opaque: only the declared component operations are executed.
type ConfiguredCatalog struct {
	Funding          string                      `json:"funding,omitempty"`
	PeriodPrizeCap   *points.Amount              `json:"period_prize_cap,omitempty"`
	Aggregates       map[string][]string         `json:"aggregates,omitempty"`
	SchemaVersion    int                         `json:"schema_version"`
	SemanticsVersion int                         `json:"semantics_version"`
	Model            Model                       `json:"model"`
	DisjointGroups   bool                        `json:"disjoint_draw_groups"`
	MaxLines         int                         `json:"max_lines"`
	PriceMin         string                      `json:"price_min"`
	PriceMax         string                      `json:"price_max"`
	Plates           []string                    `json:"plates"`
	Attributes       []ConfiguredAttribute       `json:"attributes"`
	Domains          map[string]ConfiguredDomain `json:"domains"`
	Plays            []ConfiguredPlay            `json:"plays"`
}

type ConfiguredLabel struct {
	English string `json:"en"`
	Chinese string `json:"zh-CN"`
}

type ConfiguredAttribute struct {
	ID        string            `json:"id"`
	Labels    []string          `json:"labels"`
	Partition bool              `json:"partition"`
	Fixed     map[string]string `json:"fixed,omitempty"`
}

type ConfiguredDomain struct {
	Options []ConfiguredOption `json:"options"`
}

type ConfiguredOption struct {
	ID         string             `json:"id"`
	Label      ConfiguredLabel    `json:"label"`
	Mapping    *ConfiguredMapping `json:"mapping,omitempty"`
	Value      *int               `json:"value,omitempty"`
	Reference  *ConfiguredLeg     `json:"reference,omitempty"`
	Slot       string             `json:"slot,omitempty"`
	Tags       []string           `json:"tags,omitempty"`
	PriceKey   string             `json:"price_key,omitempty"`
	SourceCode string             `json:"source_code,omitempty"`
}

type ConfiguredMapping struct {
	Kind      string              `json:"kind"` // numbers / attribute / all / transform
	Numbers   []int               `json:"numbers,omitempty"`
	Attribute string              `json:"attribute,omitempty"`
	Labels    []string            `json:"labels,omitempty"`
	Children  []ConfiguredMapping `json:"children,omitempty"`
	Transform string              `json:"transform,omitempty"` // modulo / quotient / digit_sum
	Parameter int                 `json:"parameter,omitempty"`
	Value     *int                `json:"value,omitempty"`
}

type ConfiguredLeg struct {
	PlayID   string `json:"play_id"`
	OptionID string `json:"option_id"`
}

type ConfiguredScope struct {
	Target    string `json:"target"`
	Positions []int  `json:"positions,omitempty"` // zero based, original draw order
}

type ConfiguredMetric struct {
	Op        string            `json:"op"`
	Scope     *ConfiguredScope  `json:"scope,omitempty"`
	Input     *ConfiguredMetric `json:"input,omitempty"`
	Value     *int              `json:"value,omitempty"`
	Attribute string            `json:"attribute,omitempty"`
	Counting  string            `json:"counting,omitempty"`
	Parameter int               `json:"parameter,omitempty"`
	OptionID  string            `json:"option_id,omitempty"`
	Tag       string            `json:"tag,omitempty"`
	Weights   map[string]string `json:"weights,omitempty"`
}

type ConfiguredCondition struct {
	Op       string                `json:"op"`
	Left     *ConfiguredMetric     `json:"left,omitempty"`
	Right    *ConfiguredMetric     `json:"right,omitempty"`
	Values   []int                 `json:"values,omitempty"`
	Min      *int                  `json:"min,omitempty"`
	Max      *int                  `json:"max,omitempty"`
	Children []ConfiguredCondition `json:"children,omitempty"`
}

type ConfiguredSelectionRule struct {
	Domain       string `json:"domain"`
	Mode         string `json:"mode"` // singles / combinations / as_selected
	PerLine      int    `json:"per_line"`
	Min          int    `json:"min"`
	Max          int    `json:"max"`
	DistinctSlot bool   `json:"distinct_slot"`
}

type ConfiguredPrice struct {
	Kind  string                `json:"kind"` // fixed / option / cases
	Key   string                `json:"key,omitempty"`
	Cases []ConfiguredPriceCase `json:"cases,omitempty"`
}

type ConfiguredPriceCase struct {
	When ConfiguredCondition `json:"when"`
	Key  string              `json:"key"`
}

type ConfiguredPayout struct {
	Kind  string            `json:"kind"` // fixed_return / net_profit_per_hit
	Count *ConfiguredMetric `json:"count,omitempty"`
}

type ConfiguredTier struct {
	ID        string              `json:"id"`
	When      ConfiguredCondition `json:"when"`
	Price     ConfiguredPrice     `json:"price"`
	Payout    ConfiguredPayout    `json:"payout"`
	Exclusive bool                `json:"exclusive"`
	Cap       *points.Amount      `json:"cap_points,omitempty"`
}

type ConfiguredPlay struct {
	RiskMultiplier int                      `json:"risk_multiplier,omitempty"`
	Decisions      *ConfiguredDecisionTable `json:"decisions,omitempty"`
	ID             string                   `json:"id"`
	Group          string                   `json:"group"`
	Label          ConfiguredLabel          `json:"label"`
	SourceCode     string                   `json:"source_code,omitempty"`
	Selection      ConfiguredSelectionRule  `json:"selection"`
	StakeMode      string                   `json:"stake_mode"` // per_line / unit_multiplier
	UnitPoints     points.Amount            `json:"unit_points,omitempty"`
	MaxMultiplier  points.Amount            `json:"max_multiplier"`
	Settlement     string                   `json:"settlement"` // tiers / product_of_legs
	PushWhen       *ConfiguredCondition     `json:"push_when,omitempty"`
	Tiers          []ConfiguredTier         `json:"tiers,omitempty"`
	MixedPolicy    string                   `json:"mixed_tier_policy"`
	Rounding       string                   `json:"rounding"`
	RoundingScope  string                   `json:"rounding_scope"`
	Cap            *points.Amount           `json:"cap_points,omitempty"`
}

// The supplied bindings are already pinned to a period; the engine never reads
// a clock, database, latest version, or external result/attribute provider.
type ConfiguredBindings struct {
	Version    string                      `json:"version"`
	Attributes map[string]map[string][]int `json:"attributes"`
}

type ConfiguredPrices struct {
	Version string                       `json:"version"`
	Plates  map[string]map[string]string `json:"plates"`
}

type ConfiguredRequest struct {
	PlayID     string        `json:"play_id"`
	Options    []string      `json:"options"`
	Plate      string        `json:"plate"`
	Points     points.Amount `json:"points"`
	Multiplier points.Amount `json:"multiplier"`
}

type ConfiguredQuoteLine struct {
	PlayID  string                `json:"play_id"`
	Options []string              `json:"options"`
	Plate   string                `json:"plate"`
	Stake   points.Amount         `json:"stake"`
	Prices  map[string]string     `json:"prices"`
	Legs    []ConfiguredQuoteLine `json:"legs,omitempty"`
}

type ConfiguredQuote struct {
	ReservedPoints points.Amount         `json:"reserved_points,omitempty"`
	DefinitionHash string                `json:"definition_hash"`
	SnapshotHash   string                `json:"snapshot_hash"`
	Requests       []ConfiguredRequest   `json:"requests"`
	Lines          []ConfiguredQuoteLine `json:"lines"`
	BetPoints      points.Amount         `json:"bet_points"`
}

type ConfiguredTrace struct {
	Op       string            `json:"op"`
	Actual   string            `json:"actual,omitempty"`
	Expected string            `json:"expected,omitempty"`
	Matched  bool              `json:"matched"`
	Children []ConfiguredTrace `json:"children,omitempty"`
}

type ConfiguredTierResult struct {
	ID       string          `json:"id"`
	Matched  bool            `json:"matched"`
	Selected bool            `json:"selected"`
	Price    string          `json:"price"`
	Raw      string          `json:"raw_points"`
	Capped   string          `json:"capped_points"`
	Trace    ConfiguredTrace `json:"trace"`
}

type ConfiguredLineResult struct {
	PlayID    string                 `json:"play_id"`
	Options   []string               `json:"options"`
	Outcome   string                 `json:"outcome"`
	Raw       string                 `json:"raw_points"`
	Prize     points.Amount          `json:"prize_points"`
	Refund    points.Amount          `json:"refund_points"`
	Effective points.Amount          `json:"effective_points"`
	Tiers     []ConfiguredTierResult `json:"tiers,omitempty"`
	PushTrace *ConfiguredTrace       `json:"push_trace,omitempty"`
	Legs      []ConfiguredLineResult `json:"legs,omitempty"`
}

type ConfiguredResult struct {
	ConsumedPoints  points.Amount          `json:"consumed_points,omitempty"`
	BetPoints       points.Amount          `json:"bet_points"`
	PrizePoints     points.Amount          `json:"prize_points"`
	RefundPoints    points.Amount          `json:"refund_points"`
	EffectivePoints points.Amount          `json:"effective_points"`
	Lines           []ConfiguredLineResult `json:"lines"`
	Rounding        []ConfiguredRounding   `json:"rounding"`
}

// Line prizes are independently rounded display values. Rounding records show
// the authoritative per-definition/plate aggregate, including a total cap.
type ConfiguredRounding struct {
	PlayID string        `json:"play_id"`
	Plate  string        `json:"plate"`
	Raw    string        `json:"raw_points"`
	Capped string        `json:"capped_points"`
	Prize  points.Amount `json:"prize_points"`
}

func decodeConfigured(raw []byte, destination any) error {
	if len(raw) > 4<<20 {
		return ErrLimit
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if err := uniqueKeys(d); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return ErrInvalid
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(destination); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return ErrInvalid
	}
	return nil
}

func DecodeConfiguredCatalog(raw []byte) (ConfiguredCatalog, error) {
	var c ConfiguredCatalog
	if err := decodeConfigured(raw, &c); err != nil {
		return c, err
	}
	return c, ValidateConfiguredCatalog(c)
}

// Dense finite-result rules are configuration data, independent of game names.
// Index entries address Outcomes using the configured digit model's base-ten key.
type ConfiguredDecisionTable struct {
	Index     []int                        `json:"index"`
	Outcomes  []ConfiguredDecisionOutcome  `json:"outcomes"`
	Alternate *ConfiguredDecisionAlternate `json:"alternate,omitempty"`
}
type ConfiguredDecisionAlternate struct {
	Aggregate string        `json:"aggregate"`
	AtLeast   points.Amount `json:"at_least"`
	Index     []int         `json:"index"`
}
type ConfiguredDecisionOutcome struct {
	Outcome     string `json:"outcome"`
	PriceKey    string `json:"price_key,omitempty"`
	Effective   bool   `json:"effective"`
	PrizeFactor string `json:"prize_factor,omitempty"`
	LossFactor  int    `json:"loss_factor"`
}
type ConfiguredSettlementContext struct {
	Aggregates     map[string]points.Amount `json:"aggregates"`
	PrizeRemaining *points.Amount           `json:"prize_remaining,omitempty"`
}

// DefaultConfiguredSettlementContext is the zero-aggregate, full-prize-cap
// context for previews and independent rule validation. Live settlement uses
// the period's actual aggregates and remaining cap.
func DefaultConfiguredSettlementContext(c ConfiguredCatalog) ConfiguredSettlementContext {
	ctx := ConfiguredSettlementContext{Aggregates: map[string]points.Amount{}}
	for id := range c.Aggregates {
		ctx.Aggregates[id] = 0
	}
	if c.PeriodPrizeCap != nil {
		remaining := *c.PeriodPrizeCap
		ctx.PrizeRemaining = &remaining
	}
	return ctx
}
