package commission

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/agency"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDenied         = errors.New("commission policy operation denied")
	ErrVersion        = errors.New("commission policy version changed")
	ErrPolicyEvidence = errors.New("commission policy evidence unavailable")
)

var policyUUID = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type Service struct{ DB *pgxpool.Pool }

type Policy struct {
	BrandID    string       `json:"brand_id"`
	Version    int64        `json:"version"`
	Config     PolicyConfig `json:"config"`
	CreatedAt  time.Time    `json:"created_at"`
	UpdatedAt  time.Time    `json:"updated_at"`
	RevisionID string       `json:"revision_id"`
	AuditLogID string       `json:"audit_log_id,omitempty"`
}

type Revision struct {
	ID         string          `json:"id"`
	BrandID    string          `json:"brand_id"`
	Version    int64           `json:"version"`
	Config     json.RawMessage `json:"config"`
	ChangedBy  *string         `json:"changed_by"`
	Reason     string          `json:"reason"`
	AuditLogID *string         `json:"audit_log_id"`
	CreatedAt  time.Time       `json:"created_at"`
}

type policyRow interface{ Scan(...any) error }

const policyFields = `p.brand_id::text,p.version,p.config,p.created_at,p.updated_at,r.id::text,r.audit_log_id::text,r.config`
const policyJoin = ` FROM brand_commission_policies p LEFT JOIN commission_policy_revisions r ON r.brand_id=p.brand_id AND r.version=p.version`

func scanPolicy(row policyRow) (Policy, error) {
	var out Policy
	var raw, revisionRaw []byte
	var revisionID *string
	var auditID *string
	err := row.Scan(&out.BrandID, &out.Version, &raw, &out.CreatedAt, &out.UpdatedAt, &revisionID, &auditID, &revisionRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		return Policy{}, ErrNotFound
	}
	if err != nil {
		return Policy{}, err
	}
	if revisionID == nil || *revisionID == "" {
		return Policy{}, ErrPolicyEvidence
	}
	out.RevisionID = *revisionID
	if json.Unmarshal(raw, &out.Config) != nil || out.Version < 1 {
		return Policy{}, ErrInvalid
	}
	if err = out.Config.Validate(); err != nil {
		return Policy{}, ErrInvalid
	}
	var revisionConfig PolicyConfig
	if json.Unmarshal(revisionRaw, &revisionConfig) != nil || !reflect.DeepEqual(revisionConfig, out.Config) {
		return Policy{}, ErrPolicyEvidence
	}
	out.CreatedAt = out.CreatedAt.UTC()
	out.UpdatedAt = out.UpdatedAt.UTC()
	if auditID != nil {
		out.AuditLogID = *auditID
	}
	return out, nil
}

func (s Service) Policy(ctx context.Context, brand string) (Policy, error) {
	if s.DB == nil || !policyUUID.MatchString(brand) {
		return Policy{}, ErrInvalid
	}
	return scanPolicy(s.DB.QueryRow(ctx, `SELECT `+policyFields+policyJoin+` WHERE p.brand_id=$1`, brand))
}

func policyPaging(limit, offset int) bool {
	return limit >= 1 && limit <= 100 && offset >= 0 && offset <= 1000000
}

func (s Service) History(ctx context.Context, brand string, limit, offset int) ([]Revision, error) {
	out := []Revision{}
	if s.DB == nil || !policyUUID.MatchString(brand) || !policyPaging(limit, offset) {
		return out, ErrInvalid
	}
	if _, err := s.Policy(ctx, brand); err != nil {
		return out, err
	}
	rows, err := s.DB.Query(ctx, `SELECT id::text,brand_id::text,version,config,changed_by::text,reason,audit_log_id::text,created_at FROM commission_policy_revisions WHERE brand_id=$1 ORDER BY version DESC LIMIT $2 OFFSET $3`, brand, limit, offset)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var revision Revision
		if err = rows.Scan(&revision.ID, &revision.BrandID, &revision.Version, &revision.Config, &revision.ChangedBy, &revision.Reason, &revision.AuditLogID, &revision.CreatedAt); err != nil {
			return out, err
		}
		revision.CreatedAt = revision.CreatedAt.UTC()
		out = append(out, revision)
	}
	return out, rows.Err()
}

func validateWrite(tx pgx.Tx, brand string, account access.Account, in PolicyInput, meta points.Metadata) error {
	if tx == nil || !policyUUID.MatchString(brand) || !policyUUID.MatchString(account.ID) || in.Validate() != nil ||
		meta.ActorType != "admin" || meta.ActorID != account.ID || meta.RequestID == "" || len(meta.RequestID) > 80 ||
		!utf8.ValidString(meta.RequestID) || strings.ContainsRune(meta.RequestID, '\x00') {
		return ErrInvalid
	}
	if account.SuperAdmin || account.Type != access.AccountAdmin || !access.Authorize(account, "commission_policy", "write", access.ScopeBrand, brand) {
		return ErrDenied
	}
	return nil
}

func validateEvidence(ctx context.Context, tx pgx.Tx, brand string, config PolicyConfig) error {
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT config FROM brand_agent_policies WHERE brand_id=$1 FOR SHARE`, brand).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrPolicyEvidence
	}
	if err != nil {
		return err
	}
	var agencyConfig agency.PolicyConfig
	if json.Unmarshal(raw, &agencyConfig) != nil || !agencyConfig.Valid() {
		return ErrPolicyEvidence
	}
	if config.Enabled && (!agencyConfig.Enabled || config.Calendar == nil || config.Calendar.Cycle != agencyConfig.Cycle) {
		return ErrPolicyEvidence
	}
	return nil
}

func (s Service) Update(ctx context.Context, tx pgx.Tx, brand string, account access.Account, in PolicyInput, meta points.Metadata) (Policy, error) {
	if err := validateWrite(tx, brand, account, in, meta); err != nil {
		return Policy{}, err
	}
	// Global lock order: financial policy first, then the agency policy.
	before, err := scanPolicy(tx.QueryRow(ctx, `SELECT `+policyFields+policyJoin+` WHERE p.brand_id=$1 FOR UPDATE OF p`, brand))
	if err != nil {
		return Policy{}, err
	}
	if before.Version != in.Version || before.Version == math.MaxInt64 {
		return before, ErrVersion
	}
	if err = validateEvidence(ctx, tx, brand, in.Config); err != nil {
		return before, err
	}
	nextVersion := before.Version + 1
	revisionID := ids.New()
	raw, err := json.Marshal(in.Config)
	if err != nil {
		return before, err
	}
	trimmedReason := strings.TrimSpace(in.Reason)
	logID, err := audit.Append(ctx, tx, audit.Record{
		BrandID: brand, ActorType: "admin", ActorID: account.ID,
		Action: "commission.policy.update", ResourceType: "commission_policy", ResourceID: brand,
		Reason: trimmedReason, RequestID: meta.RequestID, IP: meta.IP,
		Before: before,
		After:  map[string]any{"version": nextVersion, "config": in.Config, "revision_id": revisionID},
	})
	if err != nil {
		return before, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO commission_policy_revisions(id,brand_id,version,config,changed_by,reason,audit_log_id) VALUES($1,$2,$3,$4,$5,$6,$7)`, revisionID, brand, nextVersion, raw, account.ID, trimmedReason, logID)
	if err != nil {
		return before, err
	}
	_, err = tx.Exec(ctx, `UPDATE brand_commission_policies SET version=version+1,config=$2,updated_at=clock_timestamp() WHERE brand_id=$1 AND version=$3`, brand, raw, before.Version)
	if err != nil {
		return before, err
	}
	updated, err := scanPolicy(tx.QueryRow(ctx, `SELECT `+policyFields+policyJoin+` WHERE p.brand_id=$1`, brand))
	if err != nil {
		return before, err
	}
	updated.RevisionID = revisionID
	updated.AuditLogID = logID
	return updated, nil
}

// LockBetPolicies acquires the financial policy lock before the agency lock.
// Betting calls this before it takes a wallet lock so policy evidence stays
// stable for the transaction.
func LockBetPolicies(ctx context.Context, tx pgx.Tx, brand string) (Policy, error) {
	if tx == nil || !policyUUID.MatchString(brand) {
		return Policy{}, ErrInvalid
	}
	policy, err := scanPolicy(tx.QueryRow(ctx, `SELECT `+policyFields+policyJoin+` WHERE p.brand_id=$1 FOR SHARE OF p`, brand))
	if err != nil {
		return Policy{}, err
	}
	if err = validateEvidence(ctx, tx, brand, policy.Config); err != nil {
		return policy, err
	}
	return policy, nil
}
