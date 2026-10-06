// Package compliance contains pure models and evaluation for explicitly
// configured compliance checks. It does not collect personal data or enforce
// checks in live user or money movement flows.
package compliance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	ErrInvalid  = errors.New("invalid compliance input")
	ErrDenied   = errors.New("compliance check denied")
	ErrNotFound = errors.New("compliance record not found")
	ErrVersion  = errors.New("compliance version conflict")
	ErrState    = errors.New("compliance state conflict")
)

var configKeys = []string{"age_enabled", "minimum_age", "region_enabled", "allowed_countries", "identity_enabled"}

type Config struct {
	AgeEnabled       bool     `json:"age_enabled"`
	MinimumAge       *int     `json:"minimum_age"`
	RegionEnabled    bool     `json:"region_enabled"`
	AllowedCountries []string `json:"allowed_countries"`
	IdentityEnabled  bool     `json:"identity_enabled"`
}

type Input struct {
	Version int64  `json:"version"`
	Config  Config `json:"config"`
	Reason  string `json:"reason"`
}

type CheckInput struct {
	Version   int64  `json:"version"`
	Operation string `json:"operation"`
	Reason    string `json:"reason"`
}

type Check struct {
	Check      string `json:"check"`
	Enabled    bool   `json:"enabled"`
	Decision   string `json:"decision"`
	ReasonCode string `json:"reason_code"`
}

// Adapter is reserved for a future explicit integration boundary. Evaluate
// intentionally does not accept an Adapter, so caller supplied behavior cannot
// turn this stub evaluator into a production identity or eligibility decision.
type Adapter interface {
	Check(context.Context, string, Config) (Check, error)
}

// StubAdapter describes the current non-integrated behavior. It never reads
// personal data and always returns review for enabled checks.
type StubAdapter struct{}

func (StubAdapter) Check(_ context.Context, name string, _ Config) (Check, error) {
	if !validCheckName(name) {
		return Check{}, ErrInvalid
	}
	return Check{Check: name, Enabled: true, Decision: "review", ReasonCode: "ADAPTER_NOT_CONFIGURED"}, nil
}

func DefaultConfig() Config {
	return Config{AllowedCountries: []string{}}
}

func ValidConfig(c Config) bool {
	if c.AgeEnabled && c.MinimumAge == nil {
		return false
	}
	if c.MinimumAge != nil && (*c.MinimumAge < 18 || *c.MinimumAge > 120) {
		return false
	}
	if c.AllowedCountries == nil || len(c.AllowedCountries) > 250 || (c.RegionEnabled && len(c.AllowedCountries) == 0) {
		return false
	}
	previous := ""
	for _, country := range c.AllowedCountries {
		if len(country) != 2 || country[0] < 'A' || country[0] > 'Z' || country[1] < 'A' || country[1] > 'Z' || country <= previous {
			return false
		}
		previous = country
	}
	return true
}

func ValidReason(reason string) bool {
	if !utf8.ValidString(reason) || len(reason) == 0 || len(reason) > 500 || strings.TrimSpace(reason) != reason {
		return false
	}
	for _, r := range reason {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func ValidOperation(operation string) bool {
	switch operation {
	case "registration", "betting", "withdrawal":
		return true
	default:
		return false
	}
}

func ValidDecision(decision string) bool {
	switch decision {
	case "allow", "review", "deny", "freeze":
		return true
	default:
		return false
	}
}

func Evaluate(c Config) (string, []Check, error) {
	if !ValidConfig(c) {
		return "", nil, ErrInvalid
	}
	checks := []Check{
		evaluateCheck("age", c.AgeEnabled),
		evaluateCheck("region", c.RegionEnabled),
		evaluateCheck("identity", c.IdentityEnabled),
	}
	decision := "allow"
	for _, check := range checks {
		if check.Decision != "allow" {
			decision = "review"
			break
		}
	}
	return decision, checks, nil
}

func evaluateCheck(name string, enabled bool) Check {
	if !enabled {
		return Check{Check: name, Enabled: false, Decision: "allow", ReasonCode: "CHECK_DISABLED"}
	}
	return Check{Check: name, Enabled: true, Decision: "review", ReasonCode: "ADAPTER_NOT_CONFIGURED"}
}

func (c *Config) UnmarshalJSON(data []byte) error {
	if c == nil || closedJSON(data, configKeys, map[string]bool{"minimum_age": true}) != nil {
		return ErrInvalid
	}
	type plain Config
	var next plain
	if json.Unmarshal(data, &next) != nil {
		return ErrInvalid
	}
	result := Config(next)
	if !ValidConfig(result) {
		return ErrInvalid
	}
	*c = result
	return nil
}

func (in *Input) UnmarshalJSON(data []byte) error {
	if in == nil || closedJSON(data, []string{"version", "config", "reason"}, nil) != nil {
		return ErrInvalid
	}
	type plain Input
	var next plain
	if json.Unmarshal(data, &next) != nil {
		return ErrInvalid
	}
	result := Input(next)
	if result.Version < 1 || !ValidConfig(result.Config) || !ValidReason(result.Reason) {
		return ErrInvalid
	}
	*in = result
	return nil
}

func (in *CheckInput) UnmarshalJSON(data []byte) error {
	if in == nil || closedJSON(data, []string{"version", "operation", "reason"}, nil) != nil {
		return ErrInvalid
	}
	type plain CheckInput
	var next plain
	if json.Unmarshal(data, &next) != nil {
		return ErrInvalid
	}
	result := CheckInput(next)
	if result.Version < 1 || !ValidOperation(result.Operation) || !ValidReason(result.Reason) {
		return ErrInvalid
	}
	*in = result
	return nil
}

func closedJSON(data []byte, keys []string, nullable map[string]bool) error {
	if !utf8.Valid(data) {
		return ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(data))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return ErrInvalid
	}
	allowed, seen := make(map[string]bool, len(keys)), make(map[string]bool, len(keys))
	for _, key := range keys {
		allowed[key] = true
	}
	for d.More() {
		token, err = d.Token()
		key, ok := token.(string)
		if err != nil || !ok || !allowed[key] || seen[key] {
			return ErrInvalid
		}
		seen[key] = true
		var raw json.RawMessage
		if d.Decode(&raw) != nil || (!nullable[key] && bytes.Equal(bytes.TrimSpace(raw), []byte("null"))) {
			return ErrInvalid
		}
	}
	if token, err = d.Token(); err != nil || token != json.Delim('}') || len(seen) != len(keys) {
		return ErrInvalid
	}
	var trailing any
	if d.Decode(&trailing) != io.EOF {
		return ErrInvalid
	}
	return nil
}

func validCheckName(name string) bool {
	switch name {
	case "age", "region", "identity":
		return true
	default:
		return false
	}
}
