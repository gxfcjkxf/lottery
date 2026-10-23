// Package brandops implements audited brand operation state changes.
package brandops

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalid  = errors.New("invalid brand operation request")
	ErrDenied   = errors.New("brand operation access denied")
	ErrNotFound = errors.New("brand not found")
	ErrVersion  = errors.New("brand operation version conflict")
	ErrState    = errors.New("brand operation state transition rejected")
	brandUUID   = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

type Record struct {
	BrandID       string     `json:"brand_id"`
	Version       int64      `json:"version"`
	Name          string     `json:"name"`
	Status        string     `json:"status"`
	UpdatedAt     time.Time  `json:"updated_at"`
	AuditLogID    string     `json:"audit_log_id,omitempty"`
	PauseStartsAt *time.Time `json:"pause_starts_at,omitempty"`
	PauseEndsAt   *time.Time `json:"pause_ends_at,omitempty"`
}

type Input struct {
	Version       int64      `json:"version"`
	Status        string     `json:"status"`
	Reason        string     `json:"reason"`
	PauseStartsAt *time.Time `json:"pause_starts_at,omitempty"`
	PauseEndsAt   *time.Time `json:"pause_ends_at,omitempty"`
}

func (in *Input) UnmarshalJSON(data []byte) error {
	if in == nil || !utf8.Valid(data) {
		return ErrInvalid
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('{') {
		return ErrInvalid
	}
	seen := map[string]bool{}
	var next Input
	for dec.More() {
		tok, err = dec.Token()
		if err != nil {
			return ErrInvalid
		}
		key, ok := tok.(string)
		if !ok || seen[key] {
			return ErrInvalid
		}
		seen[key] = true
		var raw json.RawMessage
		if err = dec.Decode(&raw); err != nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return ErrInvalid
		}
		switch key {
		case "version":
			if err = json.Unmarshal(raw, &next.Version); err != nil {
				return ErrInvalid
			}
		case "status":
			if err = json.Unmarshal(raw, &next.Status); err != nil {
				return ErrInvalid
			}
		case "pause_starts_at":
			if err = json.Unmarshal(raw, &next.PauseStartsAt); err != nil {
				return ErrInvalid
			}
		case "pause_ends_at":
			if err = json.Unmarshal(raw, &next.PauseEndsAt); err != nil {
				return ErrInvalid
			}
		case "reason":
			if err = json.Unmarshal(raw, &next.Reason); err != nil {
				return ErrInvalid
			}
		default:
			return ErrInvalid
		}
	}
	if _, err = dec.Token(); err != nil {
		return ErrInvalid
	}
	if _, err = dec.Token(); err != io.EOF {
		return ErrInvalid
	}
	if len(seen) < 2 || len(seen) > 5 || !seen["version"] || !seen["status"] {
		return ErrInvalid
	}
	if err = validateInput(next); err != nil {
		return err
	}
	*in = next
	return nil
}

type Revision struct {
	ID             string     `json:"id"`
	BrandID        string     `json:"brand_id"`
	Version        int64      `json:"version"`
	PreviousStatus string     `json:"previous_status"`
	Status         string     `json:"status"`
	ChangedBy      string     `json:"changed_by"`
	Reason         string     `json:"reason"`
	AuditLogID     string     `json:"audit_log_id"`
	CreatedAt      time.Time  `json:"created_at"`
	PauseStartsAt  *time.Time `json:"pause_starts_at,omitempty"`
	PauseEndsAt    *time.Time `json:"pause_ends_at,omitempty"`
}

type Service struct{ DB *pgxpool.Pool }

func Allowed(actor access.Account, brand, action string) bool {
	if brand == "" || (action != "view" && action != "write") {
		return false
	}
	return access.Authorize(actor, "brand_operation", action, access.ScopeBrand, brand) ||
		access.Authorize(actor, "brand_operation", action, access.ScopePlatform, "")
}

func (s Service) Read(ctx context.Context, brand string) (Record, error) {
	var out Record
	if s.DB == nil || !brandUUID.MatchString(brand) {
		return out, ErrInvalid
	}
	err := s.DB.QueryRow(ctx, `SELECT b.id::text,b.config_version,b.name,b.status,b.updated_at,
		COALESCE((SELECT h.audit_log_id::text FROM brand_operation_revisions h WHERE h.brand_id=b.id AND h.version=b.config_version AND h.status=b.status),''),b.pause_starts_at,b.pause_ends_at
		FROM brands b WHERE b.id=$1`, brand).Scan(&out.BrandID, &out.Version, &out.Name, &out.Status, &out.UpdatedAt, &out.AuditLogID, &out.PauseStartsAt, &out.PauseEndsAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Record{}, ErrNotFound
	}
	out.UpdatedAt = out.UpdatedAt.UTC()
	return out, err
}

func (s Service) History(ctx context.Context, brand string, limit, offset int) ([]Revision, error) {
	if s.DB == nil {
		return nil, ErrInvalid
	}
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	out, err := s.HistoryTx(ctx, tx, brand, limit, offset)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

func (s Service) HistoryTx(ctx context.Context, tx pgx.Tx, brand string, limit, offset int) ([]Revision, error) {
	if tx == nil || !brandUUID.MatchString(brand) || limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		return nil, ErrInvalid
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM brands WHERE id=$1)`, brand).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}
	rows, err := tx.Query(ctx, `SELECT id::text,brand_id::text,version,previous_status,status,changed_by::text,reason,audit_log_id::text,created_at,pause_starts_at,pause_ends_at
		FROM brand_operation_revisions WHERE brand_id=$1 ORDER BY version DESC LIMIT $2 OFFSET $3`, brand, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Revision, 0)
	for rows.Next() {
		var v Revision
		if err = rows.Scan(&v.ID, &v.BrandID, &v.Version, &v.PreviousStatus, &v.Status, &v.ChangedBy, &v.Reason, &v.AuditLogID, &v.CreatedAt, &v.PauseStartsAt, &v.PauseEndsAt); err != nil {
			return nil, err
		}
		v.CreatedAt = v.CreatedAt.UTC()
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s Service) Update(ctx context.Context, tx pgx.Tx, brand string, actor access.Account, in Input, meta points.Metadata) (Record, error) {
	if tx == nil || !brandUUID.MatchString(brand) {
		return Record{}, ErrInvalid
	}
	if err := validateInput(in); err != nil {
		return Record{}, err
	}
	if !Allowed(actor, brand, "write") {
		return Record{}, ErrDenied
	}
	if actor.Type != access.AccountAdmin || !brandUUID.MatchString(actor.ID) || meta.ActorType != "admin" || meta.ActorID != actor.ID || meta.RequestID == "" || !utf8.ValidString(meta.RequestID) {
		return Record{}, ErrInvalid
	}
	var before Record
	err := tx.QueryRow(ctx, `SELECT b.id::text,b.config_version,b.name,b.status,b.updated_at,
		COALESCE((SELECT h.audit_log_id::text FROM brand_operation_revisions h WHERE h.brand_id=b.id AND h.version=b.config_version AND h.status=b.status),''),b.pause_starts_at,b.pause_ends_at
		FROM brands b WHERE b.id=$1 FOR UPDATE`, brand).Scan(&before.BrandID, &before.Version, &before.Name, &before.Status, &before.UpdatedAt, &before.AuditLogID, &before.PauseStartsAt, &before.PauseEndsAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, err
	}
	before.UpdatedAt = before.UpdatedAt.UTC()
	if before.Version != in.Version || before.Version == int64(^uint64(0)>>1) {
		return Record{}, ErrVersion
	}
	if (before.Status == "disabled" || in.Status == "disabled") && !access.Authorize(actor, "brand_operation", "write", access.ScopePlatform, "") {
		return Record{}, ErrDenied
	}
	if in.PauseStartsAt != nil {
		var now time.Time
		if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
			return Record{}, err
		}
		if !in.PauseEndsAt.After(now) || before.Status == "disabled" {
			return Record{}, ErrInvalid
		}
		if in.PauseStartsAt.After(now) {
			if before.Status != "active" {
				return Record{}, ErrState
			}
			in.Status = "active"
		}
	}
	if in.Status == before.Status && in.PauseStartsAt == nil && before.PauseStartsAt == nil {
		return Record{}, ErrState
	}
	return saveTransition(ctx, tx, before, in, actor.ID, meta)
}

// Caller holds the brand row lock. Automatic revisions retain the scheduling
// administrator as changed_by; the audit actor is system with no actor_id.
func saveTransition(ctx context.Context, tx pgx.Tx, before Record, in Input, changedBy string, meta points.Metadata) (Record, error) {
	brand := before.BrandID
	var after Record
	err := tx.QueryRow(ctx, `UPDATE brands SET status=$2,config_version=config_version+1,updated_at=clock_timestamp(),pause_starts_at=$3,pause_ends_at=$4 WHERE id=$1
		RETURNING id::text,config_version,name,status,updated_at,pause_starts_at,pause_ends_at`, brand, in.Status, in.PauseStartsAt, in.PauseEndsAt).Scan(&after.BrandID, &after.Version, &after.Name, &after.Status, &after.UpdatedAt, &after.PauseStartsAt, &after.PauseEndsAt)
	if err != nil {
		return Record{}, err
	}
	after.UpdatedAt = after.UpdatedAt.UTC()
	auditID, err := audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: meta.ActorType, ActorID: meta.ActorID, Action: "brand_operation.update", ResourceType: "brand_operation", ResourceID: brand, RequestID: meta.RequestID, IP: meta.IP, Reason: in.Reason, Before: before, After: after})
	if err != nil {
		return Record{}, err
	}
	after.AuditLogID = auditID
	_, err = tx.Exec(ctx, `INSERT INTO brand_operation_revisions(id,brand_id,version,previous_status,status,changed_by,reason,audit_log_id,pause_starts_at,pause_ends_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, ids.New(), brand, after.Version, before.Status, after.Status, changedBy, in.Reason, auditID, in.PauseStartsAt, in.PauseEndsAt)
	if err != nil {
		return Record{}, err
	}
	return after, nil
}

func validateInput(in Input) error {
	if in.Version <= 0 || (in.Status != "active" && in.Status != "paused" && in.Status != "disabled") {
		return ErrInvalid
	}
	if !utf8.ValidString(in.Reason) || strings.TrimSpace(in.Reason) != in.Reason || utf8.RuneCountInString(in.Reason) > 500 {
		return ErrInvalid
	}
	if (in.PauseStartsAt == nil) != (in.PauseEndsAt == nil) {
		return ErrInvalid
	}
	if in.PauseStartsAt != nil && (in.Status != "paused" || !in.PauseEndsAt.After(*in.PauseStartsAt)) {
		return ErrInvalid
	}
	return nil
}
