package brandregistry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"time"
	_ "time/tzdata"
	"unicode"
	"unicode/utf8"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

var (
	ErrInvalid      = errors.New("invalid brand creation input")
	ErrDenied       = errors.New("brand creation permission denied")
	ErrCodeConflict = errors.New("brand code already exists")
	codePattern     = regexp.MustCompile(`^[a-z][a-z0-9_]{0,47}$`)
	uuidPattern     = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

type Input struct {
	Code          string `json:"code"`
	Name          string `json:"name"`
	DefaultLocale string `json:"default_locale"`
	Timezone      string `json:"timezone"`
	Reason        string `json:"reason"`
}

func text(v string, max int) bool {
	if v == "" || strings.TrimSpace(v) != v || len(v) > max || !utf8.ValidString(v) {
		return false
	}
	for _, r := range v {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func Valid(in Input) bool {
	if !codePattern.MatchString(in.Code) || !text(in.Name, 120) || !text(in.Reason, 500) || !text(in.Timezone, 80) || in.Timezone == "Local" || (in.DefaultLocale != "en" && in.DefaultLocale != "zh-CN") {
		return false
	}
	_, err := time.LoadLocation(in.Timezone)
	return err == nil
}

func (in *Input) UnmarshalJSON(raw []byte) error {
	if in == nil || !utf8.Valid(raw) {
		return ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	tok, err := d.Token()
	if err != nil || tok != json.Delim('{') {
		return ErrInvalid
	}
	var next Input
	seen := map[string]bool{}
	for d.More() {
		tok, err = d.Token()
		key, ok := tok.(string)
		if err != nil || !ok || seen[key] {
			return ErrInvalid
		}
		seen[key] = true
		var value string
		var data json.RawMessage
		if err = d.Decode(&data); err != nil || bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
			return ErrInvalid
		}
		if err = json.Unmarshal(data, &value); err != nil {
			return ErrInvalid
		}
		switch key {
		case "code":
			next.Code = value
		case "name":
			next.Name = value
		case "default_locale":
			next.DefaultLocale = value
		case "timezone":
			next.Timezone = value
		case "reason":
			next.Reason = value
		default:
			return ErrInvalid
		}
	}
	if _, err = d.Token(); err != nil {
		return ErrInvalid
	}
	if _, err = d.Token(); err != io.EOF {
		return ErrInvalid
	}
	if len(seen) != 5 || !Valid(next) {
		return ErrInvalid
	}
	*in = next
	return nil
}

// Receipt is the immutable initial creation state, not the current brand status
// after later operation/configuration changes. Replays return the same receipt.
type Receipt struct {
	ID            string    `json:"id"`
	Code          string    `json:"code"`
	Name          string    `json:"name"`
	Status        string    `json:"status"`
	DefaultLocale string    `json:"default_locale"`
	Timezone      string    `json:"timezone"`
	Version       int64     `json:"version"`
	CreatedAt     time.Time `json:"created_at"`
	AuditLogID    string    `json:"audit_log_id"`
}

func Allowed(actor access.Account) bool {
	return uuidPattern.MatchString(actor.ID) && access.Authorize(actor, "brand", "create", access.ScopePlatform, "")
}

func Create(ctx context.Context, tx pgx.Tx, actor access.Account, in Input, meta points.Metadata) (Receipt, error) {
	var out Receipt
	if !Allowed(actor) {
		return out, ErrDenied
	}
	if tx == nil || !Valid(in) {
		return out, ErrInvalid
	}
	err := tx.QueryRow(ctx, `INSERT INTO brands(id,code,name,status,default_locale,timezone)
 VALUES($1,$2,$3,'paused',$4,$5) ON CONFLICT(code) DO NOTHING
 RETURNING id::text,code,name,status,default_locale,timezone,config_version,created_at`, ids.New(), in.Code, in.Name, in.DefaultLocale, in.Timezone).
		Scan(&out.ID, &out.Code, &out.Name, &out.Status, &out.DefaultLocale, &out.Timezone, &out.Version, &out.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrCodeConflict
	}
	if err != nil {
		return out, err
	}
	out.CreatedAt = out.CreatedAt.UTC()
	// Do not grant brand scope, create administrators, bind domains, enable
	// settlement or move points. Existing brand INSERT triggers initialize config.
	beforeAudit := out
	out.AuditLogID, err = audit.Append(ctx, tx, audit.Record{BrandID: out.ID, ActorType: "admin", ActorID: actor.ID, Action: "brand.create", ResourceType: "brand", ResourceID: out.ID, Reason: in.Reason, RequestID: meta.RequestID, IP: meta.IP, After: beforeAudit})
	if err != nil {
		return out, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO brand_creation_records(brand_id,code,name,default_locale,timezone,status,version,created_by,reason,audit_log_id,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, out.ID, out.Code, out.Name, out.DefaultLocale, out.Timezone, out.Status, out.Version, actor.ID, in.Reason, out.AuditLogID, out.CreatedAt)
	return out, err
}
