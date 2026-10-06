// Package brandops implements audited brand operation state changes.
package brandops

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	BrandID    string    `json:"brand_id"`
	Version    int64     `json:"version"`
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	UpdatedAt  time.Time `json:"updated_at"`
	AuditLogID string    `json:"audit_log_id,omitempty"`
}

type Input struct {
	Version int64  `json:"version"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
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
	if len(seen) != 3 || !seen["version"] || !seen["status"] || !seen["reason"] {
		return ErrInvalid
	}
	if err = validateInput(next); err != nil {
		return err
	}
	*in = next
	return nil
}

type Revision struct {
	ID             string    `json:"id"`
	BrandID        string    `json:"brand_id"`
	Version        int64     `json:"version"`
	PreviousStatus string    `json:"previous_status"`
	Status         string    `json:"status"`
	ChangedBy      string    `json:"changed_by"`
	Reason         string    `json:"reason"`
	AuditLogID     string    `json:"audit_log_id"`
	CreatedAt      time.Time `json:"created_at"`
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
		COALESCE((SELECT h.audit_log_id::text FROM brand_operation_revisions h WHERE h.brand_id=b.id AND h.version=b.config_version AND h.status=b.status),'')
		FROM brands b WHERE b.id=$1`, brand).Scan(&out.BrandID, &out.Version, &out.Name, &out.Status, &out.UpdatedAt, &out.AuditLogID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Record{}, ErrNotFound
	}
	out.UpdatedAt = out.UpdatedAt.UTC()
	return out, err
}

func (s Service) History(ctx context.Context, brand string, limit, offset int) ([]Revision, error) {
	if s.DB == nil || !brandUUID.MatchString(brand) || limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		return nil, ErrInvalid
	}
	var exists bool
	if err := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM brands WHERE id=$1)`, brand).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}
	rows, err := s.DB.Query(ctx, `SELECT id::text,brand_id::text,version,previous_status,status,changed_by::text,reason,audit_log_id::text,created_at
		FROM brand_operation_revisions WHERE brand_id=$1 ORDER BY version DESC LIMIT $2 OFFSET $3`, brand, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Revision, 0)
	for rows.Next() {
		var v Revision
		if err = rows.Scan(&v.ID, &v.BrandID, &v.Version, &v.PreviousStatus, &v.Status, &v.ChangedBy, &v.Reason, &v.AuditLogID, &v.CreatedAt); err != nil {
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
		COALESCE((SELECT h.audit_log_id::text FROM brand_operation_revisions h WHERE h.brand_id=b.id AND h.version=b.config_version AND h.status=b.status),'')
		FROM brands b WHERE b.id=$1 FOR UPDATE`, brand).Scan(&before.BrandID, &before.Version, &before.Name, &before.Status, &before.UpdatedAt, &before.AuditLogID)
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
	if (before.Status != "active" && before.Status != "paused") || in.Status == before.Status {
		return Record{}, ErrState
	}
	if in.Status != "active" && in.Status != "paused" {
		return Record{}, ErrState
	}
	var after Record
	err = tx.QueryRow(ctx, `UPDATE brands SET status=$2,config_version=config_version+1,updated_at=clock_timestamp() WHERE id=$1
		RETURNING id::text,config_version,name,status,updated_at`, brand, in.Status).Scan(&after.BrandID, &after.Version, &after.Name, &after.Status, &after.UpdatedAt)
	if err != nil {
		return Record{}, err
	}
	after.UpdatedAt = after.UpdatedAt.UTC()
	meta.Reason = in.Reason
	auditID, err := audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: actor.ID, Action: "brand_operation.update", ResourceType: "brand_operation", ResourceID: brand, Reason: in.Reason, RequestID: meta.RequestID, IP: meta.IP, Before: before, After: after})
	if err != nil {
		return Record{}, err
	}
	after.AuditLogID = auditID
	_, err = tx.Exec(ctx, `INSERT INTO brand_operation_revisions(id,brand_id,version,previous_status,status,changed_by,reason,audit_log_id)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, ids.New(), brand, after.Version, before.Status, after.Status, actor.ID, in.Reason, auditID)
	if err != nil {
		return Record{}, err
	}
	return after, nil
}

func validateInput(in Input) error {
	if in.Version <= 0 || (in.Status != "active" && in.Status != "paused") || !utf8.ValidString(in.Reason) || strings.TrimSpace(in.Reason) == "" || len(in.Reason) > 500 {
		return fmt.Errorf("%w: version, status, and reason are required", ErrInvalid)
	}
	return nil
}
