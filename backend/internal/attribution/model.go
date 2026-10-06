package attribution

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgconn"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrInvalid     = errors.New("invalid join-code input")
	ErrNotFound    = errors.New("join code not found")
	ErrDenied      = errors.New("join-code permission denied")
	ErrVersion     = errors.New("join-code version changed")
	ErrState       = errors.New("join-code state conflict")
	ErrUnavailable = errors.New("join code unavailable")
	uuid           = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	codePattern    = regexp.MustCompile(`^[A-F0-9]{24}$`)
)

type Code struct {
	ID            string     `json:"id"`
	BrandID       string     `json:"brand_id"`
	Kind          string     `json:"kind"`
	Code          string     `json:"code"`
	OwnerMemberID string     `json:"owner_member_id"`
	AgentID       *string    `json:"agent_id"`
	Status        string     `json:"status"`
	StartsAt      *time.Time `json:"starts_at"`
	ExpiresAt     *time.Time `json:"expires_at"`
	Version       int64      `json:"version"`
	Usable        bool       `json:"usable"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	AuditLogID    string     `json:"audit_log_id,omitempty"`
}
type CreateInput struct {
	Kind          string     `json:"kind"`
	OwnerMemberID string     `json:"owner_member_id"`
	AgentID       *string    `json:"agent_id"`
	StartsAt      *time.Time `json:"starts_at"`
	ExpiresAt     *time.Time `json:"expires_at"`
	Reason        string     `json:"reason"`
}
type UpdateInput struct {
	Version   int64      `json:"version"`
	Status    string     `json:"status"`
	StartsAt  *time.Time `json:"starts_at"`
	ExpiresAt *time.Time `json:"expires_at"`
	Reason    string     `json:"reason"`
}
type Page struct {
	BrandID       string  `json:"brand_id"`
	Kind          *string `json:"kind"`
	OwnerMemberID *string `json:"owner_member_id"`
	Items         []Code  `json:"items"`
	Limit         int     `json:"limit"`
	Offset        int     `json:"offset"`
	TotalCount    string  `json:"total_count"`
}
type SelfPage struct {
	BrandID    string `json:"brand_id"`
	MemberID   string `json:"member_id"`
	Items      []Code `json:"items"`
	Limit      int    `json:"limit"`
	Offset     int    `json:"offset"`
	TotalCount string `json:"total_count"`
}
type Revision struct {
	ID         string     `json:"id"`
	BrandID    string     `json:"brand_id"`
	CodeID     string     `json:"code_id"`
	Version    int64      `json:"version"`
	Status     string     `json:"status"`
	StartsAt   *time.Time `json:"starts_at"`
	ExpiresAt  *time.Time `json:"expires_at"`
	ActorID    string     `json:"actor_id"`
	Reason     string     `json:"reason"`
	AuditLogID string     `json:"audit_log_id"`
	CreatedAt  time.Time  `json:"created_at"`
}
type History struct {
	BrandID    string     `json:"brand_id"`
	CodeID     string     `json:"code_id"`
	Items      []Revision `json:"items"`
	Limit      int        `json:"limit"`
	Offset     int        `json:"offset"`
	TotalCount string     `json:"total_count"`
}
type PublicAttribution struct {
	BrandID    string    `json:"brand_id"`
	MemberID   string    `json:"member_id"`
	JoinMethod string    `json:"join_method"`
	JoinedAt   time.Time `json:"joined_at"`
	CodeID     *string   `json:"code_id"`
	SourceCode *string   `json:"source_code"`
	Legacy     bool      `json:"legacy"`
}

func Normalize(agent, referral string) (string, string, error) {
	agent = strings.ToUpper(strings.TrimSpace(agent))
	referral = strings.ToUpper(strings.TrimSpace(referral))
	if agent != "" && referral != "" {
		return "", "", ErrInvalid
	}
	kind, code := "", agent
	if agent != "" {
		kind = "agent"
	} else if referral != "" {
		kind, code = "referral", referral
	}
	if code != "" && !codePattern.MatchString(code) {
		return "", "", ErrInvalid
	}
	return kind, code, nil
}
func DatabaseError(err error) error {
	var p *pgconn.PgError
	if errors.As(err, &p) && p.ConstraintName == "join_code_unavailable" {
		return ErrUnavailable
	}
	return err
}
func validWindow(start, end *time.Time) bool {
	for _, v := range []*time.Time{start, end} {
		if v != nil && (v.Year() < 1 || v.Year() > 9999 || v.Nanosecond()%1000 != 0) {
			return false
		}
	}
	return start == nil || end == nil || start.Before(*end)
}
func validReason(s string) bool {
	return utf8.ValidString(s) && strings.TrimSpace(s) != "" && len([]byte(s)) <= 500
}
func closed(b []byte, keys ...string) error {
	d := json.NewDecoder(bytes.NewReader(b))
	t, e := d.Token()
	if e != nil || t != json.Delim('{') {
		return ErrInvalid
	}
	allowed, seen := map[string]bool{}, map[string]bool{}
	for _, k := range keys {
		allowed[k] = true
	}
	for d.More() {
		t, e = d.Token()
		k, ok := t.(string)
		if e != nil || !ok || !allowed[k] || seen[k] {
			return ErrInvalid
		}
		seen[k] = true
		var v json.RawMessage
		if d.Decode(&v) != nil {
			return ErrInvalid
		}
		if bytes.Equal(bytes.TrimSpace(v), []byte("null")) && k != "agent_id" && k != "starts_at" && k != "expires_at" {
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
func (v *CreateInput) UnmarshalJSON(b []byte) error {
	if closed(b, "kind", "owner_member_id", "agent_id", "starts_at", "expires_at", "reason") != nil {
		return ErrInvalid
	}
	type plain CreateInput
	var p plain
	if json.Unmarshal(b, &p) != nil {
		return ErrInvalid
	}
	*v = CreateInput(p)
	return nil
}
func (v *UpdateInput) UnmarshalJSON(b []byte) error {
	if closed(b, "version", "status", "starts_at", "expires_at", "reason") != nil {
		return ErrInvalid
	}
	type plain UpdateInput
	var p plain
	if json.Unmarshal(b, &p) != nil {
		return ErrInvalid
	}
	*v = UpdateInput(p)
	return nil
}
