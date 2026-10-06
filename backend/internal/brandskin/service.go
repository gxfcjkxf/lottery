package brandskin

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"math"
	"strings"
	"time"
)

type Service struct{ DB *pgxpool.Pool }
type Record struct {
	BrandID    string    `json:"brand_id"`
	Version    int64     `json:"version"`
	Status     string    `json:"status"`
	BaseName   string    `json:"base_name"`
	Config     Config    `json:"config"`
	Effective  Effective `json:"effective"`
	UpdatedAt  time.Time `json:"updated_at"`
	AuditLogID string    `json:"audit_log_id,omitempty"`
}
type Revision struct {
	ID         string    `json:"id"`
	BrandID    string    `json:"brand_id"`
	Version    int64     `json:"version"`
	Config     Config    `json:"config"`
	Effective  Effective `json:"effective"`
	ChangedBy  string    `json:"changed_by"`
	Reason     string    `json:"reason"`
	AuditLogID string    `json:"audit_log_id"`
	CreatedAt  time.Time `json:"created_at"`
}

func Allowed(a access.Account, brand, action string) bool {
	return uuidPattern.MatchString(brand) && (action == "view" || action == "write") && (access.Authorize(a, "brand_presentation", action, access.ScopeBrand, brand) || access.Authorize(a, "brand_presentation", action, access.ScopePlatform, ""))
}

const recordSQL = `SELECT b.id::text,b.config_version,b.status,b.name,p.config,b.updated_at,
 COALESCE((SELECT h.audit_log_id::text FROM brand_presentation_revisions h WHERE h.brand_id=b.id AND h.version=b.config_version AND h.config=p.config),'')
 FROM brands b JOIN brand_presentations p ON p.brand_id=b.id WHERE b.id=$1`

func readRecord(row pgx.Row) (Record, error) {
	var r Record
	var raw []byte
	e := row.Scan(&r.BrandID, &r.Version, &r.Status, &r.BaseName, &raw, &r.UpdatedAt, &r.AuditLogID)
	if errors.Is(e, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	if e != nil {
		return r, e
	}
	if e = json.Unmarshal(raw, &r.Config); e != nil {
		return r, e
	}
	r.Effective, e = Resolve(r.BaseName, r.Config)
	r.UpdatedAt = r.UpdatedAt.UTC()
	return r, e
}
func (s Service) Read(ctx context.Context, brand string) (Record, error) {
	if s.DB == nil || !uuidPattern.MatchString(brand) {
		return Record{}, ErrInvalid
	}
	return readRecord(s.DB.QueryRow(ctx, recordSQL, brand))
}
func (s Service) Update(ctx context.Context, tx pgx.Tx, brand string, a access.Account, in Input, meta points.Metadata) (Record, error) {
	if tx == nil || !uuidPattern.MatchString(brand) || in.Version < 1 || !validText(in.Reason, 500, false) || strings.TrimSpace(in.Reason) == "" {
		return Record{}, ErrInvalid
	}
	if !Allowed(a, brand, "write") {
		return Record{}, ErrDenied
	}
	if !uuidPattern.MatchString(a.ID) || meta.ActorType != "admin" || meta.ActorID != a.ID || meta.RequestID == "" {
		return Record{}, ErrInvalid
	}
	effective, e := Resolve("Brand", in.Config)
	if e != nil {
		return Record{}, e
	}
	before, e := readRecord(tx.QueryRow(ctx, recordSQL+" FOR UPDATE OF b", brand))
	if e != nil {
		return Record{}, e
	}
	if before.Version != in.Version || before.Version == math.MaxInt64 {
		return Record{}, ErrVersion
	}
	if before.Status == "disabled" {
		return Record{}, ErrState
	}
	effective, e = Resolve(before.BaseName, in.Config)
	if e != nil {
		return Record{}, e
	}
	var at time.Time
	var version int64
	if e = tx.QueryRow(ctx, `UPDATE brands SET config_version=config_version+1,updated_at=clock_timestamp() WHERE id=$1 RETURNING config_version,updated_at`, brand).Scan(&version, &at); e != nil {
		return Record{}, e
	}
	raw, e := json.Marshal(in.Config)
	if e != nil {
		return Record{}, e
	}
	if _, e = tx.Exec(ctx, `UPDATE brand_presentations SET config=$2,version=$3 WHERE brand_id=$1`, brand, raw, version); e != nil {
		return Record{}, e
	}
	after := Record{BrandID: brand, Version: version, Status: before.Status, BaseName: before.BaseName, Config: in.Config, Effective: effective, UpdatedAt: at.UTC()}
	auditID, e := audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: a.ID, Action: "brand_presentation.update", ResourceType: "brand_presentation", ResourceID: brand, Reason: in.Reason, RequestID: meta.RequestID, IP: meta.IP, Before: before, After: after})
	if e != nil {
		return Record{}, e
	}
	resolved, e := json.Marshal(effective)
	if e != nil {
		return Record{}, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO brand_presentation_revisions(id,brand_id,version,config,effective,changed_by,reason,audit_log_id)VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, ids.New(), brand, version, raw, resolved, a.ID, in.Reason, auditID); e != nil {
		return Record{}, e
	}
	after.AuditLogID = auditID
	return after, nil
}
func (s Service) History(ctx context.Context, brand string, limit, offset int) ([]Revision, error) {
	if s.DB == nil {
		return nil, ErrInvalid
	}
	tx, e := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	out, e := s.HistoryTx(ctx, tx, brand, limit, offset)
	if e != nil {
		return nil, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, e
	}
	return out, nil
}

func (s Service) HistoryTx(ctx context.Context, tx pgx.Tx, brand string, limit, offset int) ([]Revision, error) {
	if tx == nil || !uuidPattern.MatchString(brand) || limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		return nil, ErrInvalid
	}
	var exists bool
	if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM brand_presentations WHERE brand_id=$1)`, brand).Scan(&exists); e != nil {
		return nil, e
	}
	if !exists {
		return nil, ErrNotFound
	}
	rows, e := tx.Query(ctx, `SELECT id::text,brand_id::text,version,config,effective,changed_by::text,reason,audit_log_id::text,created_at FROM brand_presentation_revisions WHERE brand_id=$1 ORDER BY version DESC LIMIT $2 OFFSET $3`, brand, limit, offset)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Revision{}
	for rows.Next() {
		var r Revision
		var config, resolved []byte
		if e = rows.Scan(&r.ID, &r.BrandID, &r.Version, &config, &resolved, &r.ChangedBy, &r.Reason, &r.AuditLogID, &r.CreatedAt); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(config, &r.Config); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(resolved, &r.Effective); e != nil {
			return nil, e
		}
		r.CreatedAt = r.CreatedAt.UTC()
		out = append(out, r)
	}
	return out, rows.Err()
}
