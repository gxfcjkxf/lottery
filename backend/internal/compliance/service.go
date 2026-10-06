package compliance

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct{ DB *pgxpool.Pool }
type Policy struct {
	BrandID    string    `json:"brand_id"`
	Version    int64     `json:"version"`
	Config     Config    `json:"config"`
	UpdatedAt  time.Time `json:"updated_at"`
	AuditLogID string    `json:"audit_log_id,omitempty"`
}
type Revision struct {
	ID         string    `json:"id"`
	BrandID    string    `json:"brand_id"`
	Version    int64     `json:"version"`
	Config     Config    `json:"config"`
	ChangedBy  *string   `json:"changed_by"`
	Reason     string    `json:"reason"`
	AuditLogID *string   `json:"audit_log_id"`
	CreatedAt  time.Time `json:"created_at"`
}
type HistoryPage struct {
	BrandID    string     `json:"brand_id"`
	Items      []Revision `json:"items"`
	Limit      int        `json:"limit"`
	Offset     int        `json:"offset"`
	TotalCount string     `json:"total_count"`
}
type Decision struct {
	ID            string    `json:"id"`
	BrandID       string    `json:"brand_id"`
	PolicyVersion int64     `json:"policy_version"`
	Config        Config    `json:"config"`
	Operation     string    `json:"operation"`
	Decision      string    `json:"decision"`
	Checks        []Check   `json:"checks"`
	AdapterMode   string    `json:"adapter_mode"`
	CreatedBy     string    `json:"created_by"`
	Reason        string    `json:"reason"`
	AuditLogID    string    `json:"audit_log_id"`
	CreatedAt     time.Time `json:"created_at"`
}
type DecisionPage struct {
	BrandID    string     `json:"brand_id"`
	Operation  *string    `json:"operation"`
	Items      []Decision `json:"items"`
	Limit      int        `json:"limit"`
	Offset     int        `json:"offset"`
	TotalCount string     `json:"total_count"`
}

var uuid = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func Allowed(a access.Account, brand, resource, action string) bool {
	if !uuid.MatchString(brand) || (resource != "compliance_policy" && resource != "compliance_check") {
		return false
	}
	if action == "view" {
		return access.Authorize(a, resource, action, access.ScopeBrand, brand) || access.Authorize(a, resource, action, access.ScopePlatform, "")
	}
	if resource == "compliance_policy" && action != "write" || resource == "compliance_check" && action != "run" {
		return false
	}
	return !a.SuperAdmin && access.Authorize(a, resource, action, access.ScopeBrand, brand)
}

const policySQL = `SELECT p.brand_id::text,p.version,p.config,p.updated_at,coalesce((SELECT audit_log_id::text FROM compliance_policy_revisions WHERE brand_id=p.brand_id AND version=p.version),'') FROM brand_compliance_policies p WHERE p.brand_id=$1`

func scanPolicy(row pgx.Row) (v Policy, e error) {
	var raw []byte
	e = row.Scan(&v.BrandID, &v.Version, &raw, &v.UpdatedAt, &v.AuditLogID)
	if errors.Is(e, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	if e != nil {
		return v, e
	}
	e = json.Unmarshal(raw, &v.Config)
	v.UpdatedAt = v.UpdatedAt.UTC()
	return v, e
}
func (s Service) Read(ctx context.Context, brand string) (Policy, error) {
	if s.DB == nil || !uuid.MatchString(brand) {
		return Policy{}, ErrInvalid
	}
	return scanPolicy(s.DB.QueryRow(ctx, policySQL, brand))
}
func lockBrand(ctx context.Context, tx pgx.Tx, brand string) error {
	var status string
	e := tx.QueryRow(ctx, `SELECT status FROM brands WHERE id=$1 FOR SHARE`, brand).Scan(&status)
	if errors.Is(e, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if e != nil {
		return e
	}
	if status == "disabled" {
		return ErrState
	}
	return nil
}
func validMeta(a access.Account, m points.Metadata) bool {
	return uuid.MatchString(a.ID) && m.ActorType == "admin" && m.ActorID == a.ID && m.RequestID != ""
}
func (s Service) Update(ctx context.Context, tx pgx.Tx, brand string, a access.Account, in Input, meta points.Metadata) (Policy, error) {
	var out Policy
	if tx == nil || !uuid.MatchString(brand) || in.Version < 1 || !ValidReason(in.Reason) || !ValidConfig(in.Config) || !validMeta(a, meta) {
		return out, ErrInvalid
	}
	if !Allowed(a, brand, "compliance_policy", "write") {
		return out, ErrDenied
	}
	if e := lockBrand(ctx, tx, brand); e != nil {
		return out, e
	}
	before, e := scanPolicy(tx.QueryRow(ctx, policySQL+" FOR UPDATE OF p", brand))
	if e != nil {
		return out, e
	}
	if before.Version != in.Version || before.Version == math.MaxInt64 {
		return out, ErrVersion
	}
	raw, e := json.Marshal(in.Config)
	if e != nil {
		return out, e
	}
	out, e = scanPolicy(tx.QueryRow(ctx, `UPDATE brand_compliance_policies SET version=version+1,config=$2 WHERE brand_id=$1 RETURNING brand_id::text,version,config,updated_at,''`, brand, raw))
	if e != nil {
		return out, e
	}
	out.AuditLogID, e = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: a.ID, Action: "compliance.policy.update", ResourceType: "compliance_policy", ResourceID: brand, Reason: in.Reason, RequestID: meta.RequestID, IP: meta.IP, Before: before, After: out})
	if e != nil {
		return out, e
	}
	_, e = tx.Exec(ctx, `INSERT INTO compliance_policy_revisions(id,brand_id,version,config,changed_by,reason,audit_log_id) VALUES($1,$2,$3,$4,$5,$6,$7)`, ids.New(), brand, out.Version, raw, a.ID, in.Reason, out.AuditLogID)
	return out, e
}

// Check is an explicit administrative stub evaluation, not a claim that a user
// has passed verification and not a live workflow or balance/status mutation.
func (s Service) Check(ctx context.Context, tx pgx.Tx, brand string, a access.Account, in CheckInput, meta points.Metadata) (Decision, error) {
	var out Decision
	if tx == nil || !uuid.MatchString(brand) || in.Version < 1 || !ValidReason(in.Reason) || !ValidOperation(in.Operation) || !validMeta(a, meta) {
		return out, ErrInvalid
	}
	if !Allowed(a, brand, "compliance_check", "run") {
		return out, ErrDenied
	}
	if e := lockBrand(ctx, tx, brand); e != nil {
		return out, e
	}
	policy, e := scanPolicy(tx.QueryRow(ctx, policySQL+" FOR SHARE OF p", brand))
	if e != nil {
		return out, e
	}
	if policy.Version != in.Version {
		return out, ErrVersion
	}
	result, checks, e := Evaluate(policy.Config)
	if e != nil {
		return out, e
	}
	out = Decision{ID: ids.New(), BrandID: brand, PolicyVersion: policy.Version, Config: policy.Config, Operation: in.Operation, Decision: result, Checks: checks, AdapterMode: "stub", CreatedBy: a.ID, Reason: in.Reason}
	if e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&out.CreatedAt); e != nil {
		return out, e
	}
	out.CreatedAt = out.CreatedAt.UTC()
	out.AuditLogID, e = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: a.ID, Action: "compliance.check", ResourceType: "compliance_decision", ResourceID: out.ID, Reason: in.Reason, RequestID: meta.RequestID, IP: meta.IP, After: out})
	if e != nil {
		return out, e
	}
	raw, _ := json.Marshal(out.Config)
	rawChecks, _ := json.Marshal(out.Checks)
	_, e = tx.Exec(ctx, `INSERT INTO compliance_decisions(id,brand_id,policy_version,config,operation,decision,checks,adapter_mode,created_by,reason,audit_log_id,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, out.ID, brand, out.PolicyVersion, raw, out.Operation, out.Decision, rawChecks, out.AdapterMode, a.ID, out.Reason, out.AuditLogID, out.CreatedAt)
	return out, e
}
func validPage(brand string, limit, offset int) bool {
	return uuid.MatchString(brand) && limit >= 1 && limit <= 100 && offset >= 0 && offset <= 1000000
}
func (s Service) History(ctx context.Context, brand string, limit, offset int) (HistoryPage, error) {
	out := HistoryPage{BrandID: brand, Items: []Revision{}, Limit: limit, Offset: offset}
	if s.DB == nil || !validPage(brand, limit, offset) {
		return out, ErrInvalid
	}
	tx, e := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return out, e
	}
	defer tx.Rollback(ctx)
	if _, e = scanPolicy(tx.QueryRow(ctx, policySQL, brand)); e != nil {
		return out, e
	}
	if e = tx.QueryRow(ctx, `SELECT count(*)::text FROM compliance_policy_revisions WHERE brand_id=$1`, brand).Scan(&out.TotalCount); e != nil {
		return out, e
	}
	rows, e := tx.Query(ctx, `SELECT id::text,brand_id::text,version,config,changed_by::text,reason,audit_log_id::text,created_at FROM compliance_policy_revisions WHERE brand_id=$1 ORDER BY version DESC LIMIT $2 OFFSET $3`, brand, limit, offset)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var v Revision
		var raw []byte
		if e = rows.Scan(&v.ID, &v.BrandID, &v.Version, &raw, &v.ChangedBy, &v.Reason, &v.AuditLogID, &v.CreatedAt); e != nil {
			rows.Close()
			return out, e
		}
		if e = json.Unmarshal(raw, &v.Config); e != nil {
			rows.Close()
			return out, e
		}
		v.CreatedAt = v.CreatedAt.UTC()
		out.Items = append(out.Items, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	return out, tx.Commit(ctx)
}
func (s Service) Decisions(ctx context.Context, brand, operation string, limit, offset int) (DecisionPage, error) {
	out := DecisionPage{BrandID: brand, Items: []Decision{}, Limit: limit, Offset: offset}
	if operation != "" {
		out.Operation = &operation
	}
	if s.DB == nil || !validPage(brand, limit, offset) || operation != "" && !ValidOperation(operation) {
		return out, ErrInvalid
	}
	tx, e := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return out, e
	}
	defer tx.Rollback(ctx)
	if _, e = scanPolicy(tx.QueryRow(ctx, policySQL, brand)); e != nil {
		return out, e
	}
	if e = tx.QueryRow(ctx, `SELECT count(*)::text FROM compliance_decisions WHERE brand_id=$1 AND ($2='' OR operation=$2)`, brand, operation).Scan(&out.TotalCount); e != nil {
		return out, e
	}
	rows, e := tx.Query(ctx, `SELECT id::text,brand_id::text,policy_version,config,operation,decision,checks,adapter_mode,created_by::text,reason,audit_log_id::text,created_at FROM compliance_decisions WHERE brand_id=$1 AND ($2='' OR operation=$2) ORDER BY created_at DESC,id DESC LIMIT $3 OFFSET $4`, brand, operation, limit, offset)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var v Decision
		var raw, checks []byte
		if e = rows.Scan(&v.ID, &v.BrandID, &v.PolicyVersion, &raw, &v.Operation, &v.Decision, &checks, &v.AdapterMode, &v.CreatedBy, &v.Reason, &v.AuditLogID, &v.CreatedAt); e != nil {
			rows.Close()
			return out, e
		}
		if e = json.Unmarshal(raw, &v.Config); e != nil {
			rows.Close()
			return out, e
		}
		if e = json.Unmarshal(checks, &v.Checks); e != nil {
			rows.Close()
			return out, e
		}
		v.CreatedAt = v.CreatedAt.UTC()
		out.Items = append(out.Items, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	return out, tx.Commit(ctx)
}
