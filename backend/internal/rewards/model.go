// Package rewards defines the audited, brand-scoped rewards model.
package rewards

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

var (
	ErrInvalid  = errors.New("invalid rewards input")
	ErrDenied   = errors.New("rewards operation denied")
	ErrNotFound = errors.New("rewards resource not found")
	ErrState    = errors.New("rewards state conflict")
	ErrVersion  = errors.New("rewards version conflict")
	ErrBusy     = errors.New("rewards resource busy")
	uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

const MaxVersion int64 = 9007199254740991

type GrantInput struct {
	MemberID string        `json:"member_id"`
	Points   points.Amount `json:"points"`
	Reason   string        `json:"reason"`
}

type ActionInput struct {
	Version int64  `json:"version"`
	Reason  string `json:"reason"`
}

func (in GrantInput) Validate() error {
	if !validID(in.MemberID) || in.Points <= 0 || !validReason(in.Reason) {
		return ErrInvalid
	}
	return nil
}

func (in ActionInput) Validate() error {
	if in.Version < 1 || in.Version > MaxVersion || !validReason(in.Reason) {
		return ErrInvalid
	}
	return nil
}

func (in *GrantInput) UnmarshalJSON(data []byte) error {
	if in == nil {
		return ErrInvalid
	}
	fields, err := decodeClosedObject(data, "member_id", "points", "reason")
	if err != nil {
		return err
	}
	var parsed GrantInput
	if err = decodeField(fields, "member_id", &parsed.MemberID); err != nil {
		return err
	}
	if err = decodeField(fields, "points", &parsed.Points); err != nil {
		return err
	}
	if err = decodeField(fields, "reason", &parsed.Reason); err != nil {
		return err
	}
	if err = parsed.Validate(); err != nil {
		return err
	}
	*in = parsed
	return nil
}

func (in *ActionInput) UnmarshalJSON(data []byte) error {
	if in == nil {
		return ErrInvalid
	}
	fields, err := decodeClosedObject(data, "version", "reason")
	if err != nil {
		return err
	}
	var parsed ActionInput
	if err = decodeField(fields, "version", &parsed.Version); err != nil {
		return err
	}
	if err = decodeField(fields, "reason", &parsed.Reason); err != nil {
		return err
	}
	if err = parsed.Validate(); err != nil {
		return err
	}
	*in = parsed
	return nil
}

// decodeClosedObject requires exactly the declared keys and rejects duplicate
// keys. Decoding tokens also avoids encoding/json's usual last-key-wins rule.
func decodeClosedObject(data []byte, keys ...string) (map[string]json.RawMessage, error) {
	if !utf8.Valid(data) {
		return nil, ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, fmt.Errorf("%w: expected object", ErrInvalid)
	}
	wanted := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		wanted[key] = struct{}{}
	}
	fields := make(map[string]json.RawMessage, len(keys))
	for decoder.More() {
		token, tokenErr := decoder.Token()
		if tokenErr != nil {
			return nil, fmt.Errorf("%w: malformed object key", ErrInvalid)
		}
		key, ok := token.(string)
		if !ok {
			return nil, fmt.Errorf("%w: malformed object key", ErrInvalid)
		}
		if _, ok = wanted[key]; !ok {
			return nil, fmt.Errorf("%w: unknown key %q", ErrInvalid, key)
		}
		if _, ok = fields[key]; ok {
			return nil, fmt.Errorf("%w: duplicate key %q", ErrInvalid, key)
		}
		var raw json.RawMessage
		if err = decoder.Decode(&raw); err != nil {
			return nil, fmt.Errorf("%w: malformed field %q", ErrInvalid, key)
		}
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return nil, fmt.Errorf("%w: null field %q", ErrInvalid, key)
		}
		fields[key] = raw
	}
	if _, err = decoder.Token(); err != nil {
		return nil, fmt.Errorf("%w: malformed object", ErrInvalid)
	}
	if len(fields) != len(wanted) {
		return nil, fmt.Errorf("%w: missing required field", ErrInvalid)
	}
	var trailing any
	if err = decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("%w: trailing JSON value", ErrInvalid)
	}
	return fields, nil
}

func decodeField(fields map[string]json.RawMessage, name string, dst any) error {
	raw := bytes.TrimSpace(fields[name])
	if len(raw) > 0 && raw[0] == '"' && !pairedUnicodeEscapes(raw) {
		return ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("%w: invalid field %q", ErrInvalid, name)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("%w: trailing field data %q", ErrInvalid, name)
	}
	return nil
}

// encoding/json replaces lone UTF-16 surrogate escapes with U+FFFD. Reasons
// must preserve the reviewed text, not silently repair malformed Unicode.
func pairedUnicodeEscapes(raw []byte) bool {
	for i := 0; i < len(raw); i++ {
		if raw[i] != '\\' {
			continue
		}
		i++
		if i >= len(raw) {
			return false
		}
		if raw[i] != 'u' {
			continue
		}
		if i+4 >= len(raw) {
			return false
		}
		u, err := strconv.ParseUint(string(raw[i+1:i+5]), 16, 16)
		if err != nil || u >= 0xdc00 && u <= 0xdfff {
			return false
		}
		if u >= 0xd800 && u <= 0xdbff {
			if i+10 >= len(raw) || raw[i+5] != '\\' || raw[i+6] != 'u' {
				return false
			}
			low, err := strconv.ParseUint(string(raw[i+7:i+11]), 16, 16)
			if err != nil || low < 0xdc00 || low > 0xdfff {
				return false
			}
			i += 10
		} else {
			i += 4
		}
	}
	return true
}

type Order struct {
	ID                  string        `json:"id"`
	BrandID             string        `json:"brand_id"`
	MemberID            string        `json:"member_id"`
	Points              points.Amount `json:"points"`
	State               string        `json:"state"`
	Version             int64         `json:"version"`
	GrantLedgerEntryID  string        `json:"grant_ledger_entry_id"`
	RevokeLedgerEntryID *string       `json:"revoke_ledger_entry_id"`
	CreationAuditLogID  string        `json:"creation_audit_log_id"`
	LastAuditLogID      string        `json:"last_audit_log_id"`
	LastErrorCode       *string       `json:"last_error_code"`
	CreatedBy           string        `json:"created_by"`
	Reason              string        `json:"reason"`
	PointPolicyVersion  string        `json:"point_policy_version"`
	CreatedAt           time.Time     `json:"created_at"`
	UpdatedAt           time.Time     `json:"updated_at"`
	RevokedAt           *time.Time    `json:"revoked_at"`
}

type OrderPage struct {
	BrandID    string  `json:"brand_id"`
	Items      []Order `json:"items"`
	TotalCount string  `json:"total_count"`
	Limit      int     `json:"limit"`
	Offset     int     `json:"offset"`
}

type Action struct {
	ID            string    `json:"id"`
	BrandID       string    `json:"brand_id"`
	OrderID       string    `json:"order_id"`
	Version       int64     `json:"version"`
	Operation     string    `json:"operation"`
	StateBefore   *string   `json:"state_before"`
	StateAfter    string    `json:"state_after"`
	ActorID       string    `json:"actor_id"`
	Reason        string    `json:"reason"`
	AuditLogID    string    `json:"audit_log_id"`
	LedgerEntryID *string   `json:"ledger_entry_id"`
	CreatedAt     time.Time `json:"created_at"`
}

type ActionPage struct {
	BrandID    string   `json:"brand_id"`
	OrderID    string   `json:"order_id"`
	Items      []Action `json:"items"`
	TotalCount string   `json:"total_count"`
	Limit      int      `json:"limit"`
	Offset     int      `json:"offset"`
}

func Allowed(a access.Account, brand, action string) bool {
	if !canonicalUUID(brand) {
		return false
	}
	view := access.Authorize(a, "reward", "view", access.ScopeBrand, brand) ||
		access.Authorize(a, "reward", "view", access.ScopePlatform, "")
	if action == "view" {
		return view
	}
	if action != "grant" && action != "revoke" && action != "retry" || a.Type != access.AccountAdmin || a.SuperAdmin || !view {
		return false
	}
	return access.Authorize(a, "reward", action, access.ScopeBrand, brand)
}

func canonicalUUID(s string) bool { return uuidPattern.MatchString(s) }

func validID(s string) bool { return canonicalUUID(s) }

func validReason(s string) bool {
	if !utf8.ValidString(s) || s == "" || strings.TrimSpace(s) != s || len(s) > 500 {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
