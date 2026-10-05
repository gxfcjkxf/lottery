// Package rules interprets a bounded, typed lottery DSL; never executable code.
package rules

import (
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

var (
	ErrInvalid = errors.New("invalid lottery rule or selection")
	ErrLimit   = errors.New("rule execution limit exceeded")
)

const MaxCombinations = 10000

type Pool struct {
	Min         int   `json:"min,omitempty"`
	Max         int   `json:"max,omitempty"`
	Values      []int `json:"values,omitempty"`
	AllowRepeat bool  `json:"allow_repeat"`
}
type Model struct {
	Type         string `json:"model"`
	RegularPool  Pool   `json:"regular_pool"`
	SpecialPool  Pool   `json:"special_pool"`
	RegularCount int    `json:"regular_count"`
	SpecialCount int    `json:"special_count"`
	PoolSize     int    `json:"pool_size"`
	TotalCount   int    `json:"total_count"`
	Length       int    `json:"length"`
	AllowRepeat  bool   `json:"allow_repeat"`
	Ordered      bool   `json:"ordered"`
}
type SelectionRule struct {
	Mode            string           `json:"mode"` // numbers / exclude / attributes / features
	RegularCount    int              `json:"regular_count"`
	SpecialCount    int              `json:"special_count"`
	ExcludeCount    int              `json:"exclude_count"`
	AttributeGroups []string         `json:"attribute_groups"`
	FeatureChoices  map[string][]int `json:"feature_choices"`
}
type Selection struct {
	Regular    []int               `json:"regular"`
	Special    []int               `json:"special"`
	Digits     [][]int             `json:"digits"` // one candidate list per ordered position
	Exclude    []int               `json:"exclude"`
	Attributes map[string][]string `json:"attributes"`
	Features   map[string][]int    `json:"features"`
}
type Draw struct {
	Regular []int `json:"regular"`
	Special []int `json:"special"`
	Digits  []int `json:"digits"`
}
type Condition struct {
	Op             string      `json:"op"` // all / any / not / equals / in / between / selected
	Field          string      `json:"field,omitempty"`
	Target         string      `json:"target,omitempty"`   // all / regular / special / digits
	Position       *int        `json:"position,omitempty"` // zero-based
	AttributeGroup string      `json:"attribute_group,omitempty"`
	AttributeValue string      `json:"attribute_value,omitempty"` // literal or $selection
	SelectionKey   string      `json:"selection_key,omitempty"`
	Value          *int        `json:"value,omitempty"`
	Values         []int       `json:"values,omitempty"`
	Min            *int        `json:"min,omitempty"`
	Max            *int        `json:"max,omitempty"`
	Children       []Condition `json:"children,omitempty"`
}
type Tier struct {
	Code      string         `json:"code"`
	Condition Condition      `json:"condition"`
	Odds      string         `json:"odds"` // exact positive decimal, up to six places
	Exclusive bool           `json:"exclusive"`
	CapPoints *points.Amount `json:"cap_points"`
}
type Limits struct {
	MaxCombinations int            `json:"max_combinations"`
	MaxMultiplier   points.Amount  `json:"max_multiplier"`
	MaxBetPoints    *points.Amount `json:"max_bet_points"`
}
type Definition struct {
	SchemaVersion    int                         `json:"schema_version"`
	Model            Model                       `json:"model"`
	Selection        SelectionRule               `json:"selection"`
	NumberAttributes map[string]map[string][]int `json:"number_attributes"`
	UnitPoints       points.Amount               `json:"unit_points"`
	PrizeTiers       []Tier                      `json:"prize_tiers"`
	// Mixed tiers have no inferred default: max_all or max_exclusive_plus_additive.
	MixedTierPolicy string         `json:"mixed_tier_policy"`
	CapPoints       *points.Amount `json:"cap_points"`
	Rounding        string         `json:"rounding"`       // half_up; final points always integer
	RoundingScope   string         `json:"rounding_scope"` // order / line / tier
	Limits          Limits         `json:"limits"`
}
type Trace struct {
	Op       string  `json:"op"`
	Field    string  `json:"field,omitempty"`
	Actual   int     `json:"actual"`
	Matched  bool    `json:"matched"`
	Children []Trace `json:"children,omitempty"`
}
type TierHit struct {
	Code         string `json:"code"`
	Exclusive    bool   `json:"exclusive"`
	Matched      bool   `json:"matched"`
	Selected     bool   `json:"selected"`
	RawPoints    string `json:"raw_points"` // exact rational n/d, not a float
	CappedPoints string `json:"capped_points"`
	Points       string `json:"points"` // arbitrary-precision intermediate integer
	Trace        Trace  `json:"trace"`
}
type LineResult struct {
	Selection Selection `json:"selection"`
	Hits      []TierHit `json:"hits"`
	Points    string    `json:"points"`
}
type SimulationInput struct {
	Definition Definition    `json:"definition"`
	Selection  Selection     `json:"selection"`
	Draw       Draw          `json:"draw"`
	Multiplier points.Amount `json:"multiplier"`
}
type Simulation struct {
	Won               bool          `json:"won"`
	Normalized        Selection     `json:"normalized"`
	CombinationCount  int           `json:"combination_count"`
	Multiplier        points.Amount `json:"multiplier"`
	BetPoints         points.Amount `json:"bet_points"`
	PrizePoints       points.Amount `json:"prize_points"`
	RawPrizePoints    string        `json:"raw_prize_points"`
	CappedPrizePoints string        `json:"capped_prize_points"`
	Lines             []LineResult  `json:"lines"`
	Warnings          []string      `json:"warnings"`
}
