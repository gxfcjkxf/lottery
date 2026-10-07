package commission

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"unicode/utf8"
)

const (
	PayoutManual    = "manual"
	PayoutAutomatic = "automatic"
)

// PolicyConfig deliberately contains only the financial policy choices in
// the commission contract. Financial defaults remain disabled until a brand
// explicitly configures them.
type PolicyConfig struct {
	Enabled    bool      `json:"enabled"`
	Calendar   *Calendar `json:"calendar"`
	PayoutMode string    `json:"payout_mode"`
}

func DefaultPolicyConfig() PolicyConfig {
	return PolicyConfig{PayoutMode: PayoutManual}
}

func (c PolicyConfig) Validate() error {
	if c.PayoutMode != PayoutManual && c.PayoutMode != PayoutAutomatic {
		return ErrInvalid
	}
	if c.Calendar != nil && c.Calendar.Validate() != nil {
		return ErrInvalid
	}
	if c.Enabled && c.Calendar == nil {
		return ErrInvalid
	}
	return nil
}

// MarshalJSON preserves the public calendar spelling without adding tags to
// Calendar itself, whose scheduling implementation remains independently
// owned.
func (c Calendar) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Timezone     string `json:"timezone"`
		Cycle        string `json:"cycle"`
		BoundaryTime string `json:"boundary_time"`
		Weekday      *int   `json:"weekday"`
		MonthDay     *int   `json:"month_day"`
		ShortMonth   string `json:"short_month"`
	}{c.Timezone, c.Cycle, c.BoundaryTime, c.Weekday, c.MonthDay, c.ShortMonth})
}

func (c *Calendar) UnmarshalJSON(data []byte) error {
	if c == nil || closedObject(data, "timezone", "cycle", "boundary_time", "weekday", "month_day", "short_month") != nil {
		return ErrInvalid
	}
	var value struct {
		Timezone     string `json:"timezone"`
		Cycle        string `json:"cycle"`
		BoundaryTime string `json:"boundary_time"`
		Weekday      *int   `json:"weekday"`
		MonthDay     *int   `json:"month_day"`
		ShortMonth   string `json:"short_month"`
	}
	if json.Unmarshal(data, &value) != nil {
		return ErrInvalid
	}
	*c = Calendar{Timezone: value.Timezone, Cycle: value.Cycle, BoundaryTime: value.BoundaryTime, Weekday: value.Weekday, MonthDay: value.MonthDay, ShortMonth: value.ShortMonth}
	if c.Validate() != nil {
		return ErrInvalid
	}
	return nil
}

func (c *PolicyConfig) UnmarshalJSON(data []byte) error {
	if c == nil || closedObject(data, "enabled", "calendar", "payout_mode") != nil {
		return ErrInvalid
	}
	var value struct {
		Enabled    bool      `json:"enabled"`
		Calendar   *Calendar `json:"calendar"`
		PayoutMode string    `json:"payout_mode"`
	}
	if json.Unmarshal(data, &value) != nil {
		return ErrInvalid
	}
	*c = PolicyConfig{Enabled: value.Enabled, Calendar: value.Calendar, PayoutMode: value.PayoutMode}
	if c.Validate() != nil {
		return ErrInvalid
	}
	return nil
}

type PolicyInput struct {
	Version int64        `json:"version"`
	Config  PolicyConfig `json:"config"`
	Reason  string       `json:"reason"`
}

func (in *PolicyInput) UnmarshalJSON(data []byte) error {
	if in == nil || closedObject(data, "version", "config", "reason") != nil {
		return ErrInvalid
	}
	var value struct {
		Version int64        `json:"version"`
		Config  PolicyConfig `json:"config"`
		Reason  string       `json:"reason"`
	}
	if json.Unmarshal(data, &value) != nil {
		return ErrInvalid
	}
	*in = PolicyInput{Version: value.Version, Config: value.Config, Reason: value.Reason}
	if in.Validate() != nil {
		return ErrInvalid
	}
	return nil
}

func (in PolicyInput) Validate() error {
	if in.Version < 1 || in.Config.Validate() != nil || !validPolicyReason(in.Reason) {
		return ErrInvalid
	}
	return nil
}

func validPolicyReason(reason string) bool {
	return strings.TrimSpace(reason) != "" && len([]byte(reason)) <= 500 && utf8.ValidString(reason) && !strings.ContainsRune(reason, '\x00')
}

// closedObject rejects missing, repeated, unknown, and trailing JSON fields.
func closedObject(data []byte, required ...string) error {
	if !utf8.Valid(data) {
		return ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(data))
	tok, err := d.Token()
	if err != nil || tok != json.Delim('{') {
		return ErrInvalid
	}
	want := make(map[string]bool, len(required))
	for _, key := range required {
		want[key] = true
	}
	seen := make(map[string]bool, len(required))
	for d.More() {
		tok, err = d.Token()
		key, ok := tok.(string)
		if err != nil || !ok || !want[key] || seen[key] {
			return ErrInvalid
		}
		seen[key] = true
		var value json.RawMessage
		if d.Decode(&value) != nil {
			return ErrInvalid
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) && key != "calendar" && key != "weekday" && key != "month_day" && key != "parent_id" {
			return ErrInvalid
		}
	}
	tok, err = d.Token()
	if err != nil || tok != json.Delim('}') || len(seen) != len(want) {
		return ErrInvalid
	}
	var trailing any
	if d.Decode(&trailing) != io.EOF {
		return ErrInvalid
	}
	return nil
}
