// Package agency manages audited agent identities and exact commission settings.
// It does not calculate, accrue, or pay commissions or change any wallet.
package agency

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrInvalid = errors.New("invalid agent input")
var ErrNotFound = errors.New("agent resource not found")
var ErrDenied = errors.New("agent operation denied")
var ErrVersion = errors.New("agent version changed")
var ErrLimit = errors.New("agent hierarchy/ratio limit")
var ErrState = errors.New("agent state conflict")
var uuid = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var ratio = regexp.MustCompile(`^(0|1|0\.[0-9]{0,5}[1-9])$`)

func RatioMicros(s string) (int64, error) {
	if !ratio.MatchString(s) {
		return 0, ErrInvalid
	}
	if s == "1" {
		return 1000000, nil
	}
	if s == "0" {
		return 0, nil
	}
	return strconv.ParseInt(strings.TrimPrefix(s, "0.")+strings.Repeat("0", 6-len(s)+2), 10, 64)
}
func validMode(s string) bool { return s == "loss" || s == "turnover" }
func validReason(s string) bool {
	return len(strings.TrimSpace(s)) > 0 && len(s) <= 500 && utf8.ValidString(s)
}

type PolicyConfig struct {
	Enabled  bool   `json:"enabled"`
	MaxDepth int    `json:"max_depth"`
	RatioCap string `json:"ratio_cap"`
	Mode     string `json:"mode"`
	Cycle    string `json:"cycle"`
}

func (c PolicyConfig) Valid() bool {
	_, e := RatioMicros(c.RatioCap)
	return e == nil && c.MaxDepth >= 1 && c.MaxDepth <= 32 && validMode(c.Mode) && (c.Cycle == "weekly" || c.Cycle == "monthly")
}

type NodeConfig struct {
	Ratio             string  `json:"ratio"`
	Mode              *string `json:"mode"`
	Status            string  `json:"status"`
	CanCreateChildren bool    `json:"can_create_children"`
}

func (c NodeConfig) Valid() bool {
	_, e := RatioMicros(c.Ratio)
	return e == nil && (c.Mode == nil || validMode(*c.Mode)) && (c.Status == "active" || c.Status == "disabled")
}

type Policy struct {
	BrandID    string       `json:"brand_id"`
	Version    int64        `json:"version"`
	Config     PolicyConfig `json:"config"`
	UpdatedAt  time.Time    `json:"updated_at"`
	AuditLogID string       `json:"audit_log_id,omitempty"`
}
type Node struct {
	ID                string     `json:"id"`
	BrandID           string     `json:"brand_id"`
	MemberID          string     `json:"member_id"`
	ParentID          *string    `json:"parent_id"`
	Depth             int        `json:"depth"`
	Path              []string   `json:"path"`
	Version           int64      `json:"version"`
	Config            NodeConfig `json:"config"`
	EffectiveMode     string     `json:"effective_mode"`
	ModeSourceAgentID *string    `json:"mode_source_agent_id"`
	PolicyVersion     int64      `json:"policy_version"`
	ParentVersion     *int64     `json:"parent_version"`
	CreatedBy         string     `json:"created_by"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	AuditLogID        string     `json:"audit_log_id,omitempty"`
}
type PolicyInput struct {
	Version int64        `json:"version"`
	Config  PolicyConfig `json:"config"`
	Reason  string       `json:"reason"`
}
type CreateInput struct {
	PolicyVersion int64      `json:"policy_version"`
	MemberID      string     `json:"member_id"`
	ParentID      *string    `json:"parent_id"`
	ParentVersion *int64     `json:"parent_version"`
	Config        NodeConfig `json:"config"`
	Reason        string     `json:"reason"`
}
type UpdateInput struct {
	Version       int64      `json:"version"`
	PolicyVersion int64      `json:"policy_version"`
	ParentVersion *int64     `json:"parent_version"`
	Config        NodeConfig `json:"config"`
	Reason        string     `json:"reason"`
}
type ChildInput struct {
	Version       int64   `json:"version"`
	PolicyVersion int64   `json:"policy_version"`
	ParentVersion int64   `json:"parent_version"`
	Ratio         string  `json:"ratio"`
	Mode          *string `json:"mode"`
	Reason        string  `json:"reason"`
}
type Tree struct {
	BrandID    string  `json:"brand_id"`
	ParentID   *string `json:"parent_id"`
	Items      []Node  `json:"items"`
	Limit      int     `json:"limit"`
	Offset     int     `json:"offset"`
	TotalCount string  `json:"total_count"`
}
type Revision struct {
	ID         string          `json:"id"`
	BrandID    string          `json:"brand_id"`
	AgentID    *string         `json:"agent_id"`
	Version    int64           `json:"version"`
	Config     json.RawMessage `json:"config"`
	ActorType  string          `json:"actor_type"`
	ActorID    *string         `json:"actor_id"`
	Reason     string          `json:"reason"`
	CreatedAt  time.Time       `json:"created_at"`
	AuditLogID *string         `json:"audit_log_id"`
}
type History struct {
	BrandID    string     `json:"brand_id"`
	AgentID    *string    `json:"agent_id"`
	Items      []Revision `json:"items"`
	Limit      int        `json:"limit"`
	Offset     int        `json:"offset"`
	TotalCount string     `json:"total_count"`
}

func closed(data []byte, keys ...string) error {
	d := json.NewDecoder(bytes.NewReader(data))
	t, e := d.Token()
	if e != nil || t != json.Delim('{') {
		return ErrInvalid
	}
	seen := map[string]bool{}
	allowed := map[string]bool{}
	for _, k := range keys {
		allowed[k] = true
	}
	for d.More() {
		t, e = d.Token()
		if e != nil {
			return ErrInvalid
		}
		k, ok := t.(string)
		if !ok || !allowed[k] || seen[k] {
			return ErrInvalid
		}
		seen[k] = true
		var v json.RawMessage
		if d.Decode(&v) != nil {
			return ErrInvalid
		}
		if bytes.Equal(bytes.TrimSpace(v), []byte("null")) && k != "mode" && k != "parent_id" && k != "parent_version" {
			return ErrInvalid
		}
	}
	t, e = d.Token()
	if e != nil || t != json.Delim('}') || len(seen) != len(keys) {
		return ErrInvalid
	}
	var tail any
	if d.Decode(&tail) != io.EOF {
		return ErrInvalid
	}
	return nil
}
func (c *PolicyConfig) UnmarshalJSON(b []byte) error {
	if closed(b, "enabled", "max_depth", "ratio_cap", "mode", "cycle") != nil {
		return ErrInvalid
	}
	type plain PolicyConfig
	var p plain
	if json.Unmarshal(b, &p) != nil {
		return ErrInvalid
	}
	*c = PolicyConfig(p)
	if !c.Valid() {
		return ErrInvalid
	}
	return nil
}
func (c *NodeConfig) UnmarshalJSON(b []byte) error {
	if closed(b, "ratio", "mode", "status", "can_create_children") != nil {
		return ErrInvalid
	}
	type plain NodeConfig
	var p plain
	if json.Unmarshal(b, &p) != nil {
		return ErrInvalid
	}
	*c = NodeConfig(p)
	if !c.Valid() {
		return ErrInvalid
	}
	return nil
}
func (c *PolicyInput) UnmarshalJSON(b []byte) error {
	if closed(b, "version", "config", "reason") != nil {
		return ErrInvalid
	}
	type plain PolicyInput
	var p plain
	if json.Unmarshal(b, &p) != nil {
		return ErrInvalid
	}
	*c = PolicyInput(p)
	return nil
}
func (c *CreateInput) UnmarshalJSON(b []byte) error {
	if closed(b, "policy_version", "member_id", "parent_id", "parent_version", "config", "reason") != nil {
		return ErrInvalid
	}
	type plain CreateInput
	var p plain
	if json.Unmarshal(b, &p) != nil {
		return ErrInvalid
	}
	*c = CreateInput(p)
	return nil
}
func (c *UpdateInput) UnmarshalJSON(b []byte) error {
	if closed(b, "version", "policy_version", "parent_version", "config", "reason") != nil {
		return ErrInvalid
	}
	type plain UpdateInput
	var p plain
	if json.Unmarshal(b, &p) != nil {
		return ErrInvalid
	}
	*c = UpdateInput(p)
	return nil
}
func (c *ChildInput) UnmarshalJSON(b []byte) error {
	if closed(b, "version", "policy_version", "parent_version", "ratio", "mode", "reason") != nil {
		return ErrInvalid
	}
	type plain ChildInput
	var p plain
	if json.Unmarshal(b, &p) != nil {
		return ErrInvalid
	}
	*c = ChildInput(p)
	if _, e := RatioMicros(c.Ratio); e != nil || c.Mode != nil && !validMode(*c.Mode) {
		return ErrInvalid
	}
	return nil
}
