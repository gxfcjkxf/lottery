// Package withdrawal manages versioned withdrawal rules. This configuration
// module does not compute eligibility or create orders or point transfers.
package withdrawal

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"regexp"

	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

var (
	ErrInvalid      = errors.New("invalid withdrawal policy")
	ErrNotFound     = errors.New("withdrawal policy not found")
	ErrDenied       = errors.New("withdrawal policy operation denied")
	ErrVersion      = errors.New("withdrawal policy version conflict")
	uuidPattern     = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	multiplePattern = regexp.MustCompile(`^(0|[1-9][0-9]{0,6})([.][0-9]{0,5}[1-9])?$`)
)

type BrandConfig struct {
	Enabled          bool           `json:"enabled"`
	MinPoints        points.Amount  `json:"min_points"`
	MaxPoints        *points.Amount `json:"max_points"`
	AllowedSources   []string       `json:"allowed_sources"`
	ReviewMode       string         `json:"review_mode"`
	TurnoverMultiple string         `json:"turnover_multiple"`
}
type GameConfig struct {
	TurnoverMultiple *string `json:"turnover_multiple"`
}
type BrandInput struct {
	Version int64       `json:"version"`
	Config  BrandConfig `json:"config"`
	Reason  string      `json:"reason"`
}
type GameInput struct {
	Version int64      `json:"version"`
	Config  GameConfig `json:"config"`
	Reason  string     `json:"reason"`
}

func DefaultBrandConfig() BrandConfig {
	return BrandConfig{MinPoints: 1, AllowedSources: []string{"recharge", "winning", "gift"}, ReviewMode: "manual", TurnoverMultiple: "1"}
}
func ValidMultiple(v string) bool {
	if !validSavedMultiple(v) {
		return false
	}
	n, _ := new(big.Rat).SetString(v)
	return n.Sign() > 0
}
func validSavedMultiple(v string) bool {
	if !multiplePattern.MatchString(v) {
		return false
	}
	n, ok := new(big.Rat).SetString(v)
	return ok && n.Cmp(big.NewRat(1000000, 1)) <= 0
}
func validateSavedBrandConfig(c BrandConfig) error {
	if c.MinPoints <= 0 || c.MaxPoints != nil && *c.MaxPoints < c.MinPoints || !validSavedMultiple(c.TurnoverMultiple) || (c.ReviewMode != "manual" && c.ReviewMode != "automatic") || len(c.AllowedSources) < 1 || len(c.AllowedSources) > 4 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, v := range c.AllowedSources {
		if seen[v] || v != "recharge" && v != "winning" && v != "gift" && v != "commission" {
			return ErrInvalid
		}
		seen[v] = true
	}
	return nil
}
func validateSavedGameConfig(c GameConfig) error {
	if c.TurnoverMultiple != nil && !validSavedMultiple(*c.TurnoverMultiple) {
		return ErrInvalid
	}
	return nil
}
func ValidateBrandConfig(c BrandConfig) error {
	if validateSavedBrandConfig(c) != nil || !ValidMultiple(c.TurnoverMultiple) {
		return ErrInvalid
	}
	return nil
}
func ValidateGameConfig(c GameConfig) error {
	if validateSavedGameConfig(c) != nil || c.TurnoverMultiple != nil && !ValidMultiple(*c.TurnoverMultiple) {
		return ErrInvalid
	}
	return nil
}

// Full replacements reject missing, unknown and repeated fields. Explicit null
// is allowed only for the cap and the game override, never required scalars.
func fields(data []byte, keys ...string) (map[string]json.RawMessage, error) {
	d := json.NewDecoder(bytes.NewReader(data))
	tok, e := d.Token()
	if e != nil || tok != json.Delim('{') {
		return nil, ErrInvalid
	}
	out := map[string]json.RawMessage{}
	allowed := map[string]bool{}
	for _, key := range keys {
		allowed[key] = true
	}
	for d.More() {
		tok, e = d.Token()
		if e != nil {
			return nil, ErrInvalid
		}
		key, ok := tok.(string)
		if !ok || !allowed[key] || out[key] != nil {
			return nil, ErrInvalid
		}
		var raw json.RawMessage
		if d.Decode(&raw) != nil {
			return nil, ErrInvalid
		}
		out[key] = raw
	}
	if tok, e = d.Token(); e != nil || tok != json.Delim('}') || len(out) != len(keys) {
		return nil, ErrInvalid
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return nil, ErrInvalid
	}
	return out, nil
}
func decodeField(raw json.RawMessage, dest any, nullable bool) error {
	if !nullable && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return ErrInvalid
	}
	if json.Unmarshal(raw, dest) != nil {
		return ErrInvalid
	}
	return nil
}
func (c *BrandConfig) UnmarshalJSON(data []byte) error {
	v, e := fields(data, "enabled", "min_points", "max_points", "allowed_sources", "review_mode", "turnover_multiple")
	if e != nil {
		return e
	}
	var out BrandConfig
	for _, f := range []struct {
		key      string
		dest     any
		nullable bool
	}{{"enabled", &out.Enabled, false}, {"min_points", &out.MinPoints, false}, {"max_points", &out.MaxPoints, true}, {"allowed_sources", &out.AllowedSources, false}, {"review_mode", &out.ReviewMode, false}, {"turnover_multiple", &out.TurnoverMultiple, false}} {
		if decodeField(v[f.key], f.dest, f.nullable) != nil {
			return ErrInvalid
		}
	}
	if e = validateSavedBrandConfig(out); e != nil {
		return e
	}
	*c = out
	return nil
}
func (c *GameConfig) UnmarshalJSON(data []byte) error {
	v, e := fields(data, "turnover_multiple")
	if e != nil {
		return e
	}
	var out GameConfig
	if decodeField(v["turnover_multiple"], &out.TurnoverMultiple, true) != nil {
		return ErrInvalid
	}
	if e = validateSavedGameConfig(out); e != nil {
		return e
	}
	*c = out
	return nil
}
func (in *BrandInput) UnmarshalJSON(data []byte) error {
	v, e := fields(data, "version", "config", "reason")
	if e != nil {
		return e
	}
	var out BrandInput
	if decodeField(v["version"], &out.Version, false) != nil || decodeField(v["config"], &out.Config, false) != nil || decodeField(v["reason"], &out.Reason, false) != nil || ValidateBrandConfig(out.Config) != nil {
		return ErrInvalid
	}
	*in = out
	return nil
}
func (in *GameInput) UnmarshalJSON(data []byte) error {
	v, e := fields(data, "version", "config", "reason")
	if e != nil {
		return e
	}
	var out GameInput
	if decodeField(v["version"], &out.Version, false) != nil || decodeField(v["config"], &out.Config, false) != nil || decodeField(v["reason"], &out.Reason, false) != nil || ValidateGameConfig(out.Config) != nil {
		return ErrInvalid
	}
	*in = out
	return nil
}
func validIDs(values ...string) bool {
	for _, v := range values {
		if !uuidPattern.MatchString(v) {
			return false
		}
	}
	return true
}
