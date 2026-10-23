package commission

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/agency"
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
}

type policyRow interface{ Scan(...any) error }

const policyFields = `p.brand_id::text,p.version,p.config,p.created_at,p.updated_at,p.revision_id::text`
const policyJoin = ` FROM brand_commission_policies p`

func scanPolicy(row policyRow) (Policy, error) {
	var out Policy
	var raw []byte
	err := row.Scan(&out.BrandID, &out.Version, &raw, &out.CreatedAt, &out.UpdatedAt, &out.RevisionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Policy{}, ErrNotFound
	}
	if err != nil {
		return Policy{}, err
	}
	if json.Unmarshal(raw, &out.Config) != nil || out.Version < 1 || !canonicalUUID(out.RevisionID) {
		return Policy{}, ErrInvalid
	}
	if err = out.Config.Validate(); err != nil {
		return Policy{}, ErrInvalid
	}
	out.CreatedAt = out.CreatedAt.UTC()
	out.UpdatedAt = out.UpdatedAt.UTC()
	return out, nil
}

func (s Service) Policy(ctx context.Context, brand string) (Policy, error) {
	if s.DB == nil || !policyUUID.MatchString(brand) {
		return Policy{}, ErrInvalid
	}
	return scanPolicy(s.DB.QueryRow(ctx, `SELECT `+policyFields+policyJoin+` WHERE p.brand_id=$1`, brand))
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

func validateEvidence(ctx context.Context, tx pgx.Tx, brand string) error {
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
	// Materialize due agent revisions while holding the financial lock, before
	// validating a new calendar. Pending changes still block policy edits.
	if _, err = tx.Exec(ctx, `SELECT apply_due_agent_mode_changes($1)`, brand); err != nil {
		return before, err
	}
	if err = validateEvidence(ctx, tx, brand); err != nil {
		return before, err
	}
	raw, err := json.Marshal(in.Config)
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
	return updated, nil
}

// LockBetPolicies acquires the financial policy lock before the agency lock.
// Betting calls this before it takes a wallet lock so the snapshot stays
// stable for the transaction.
func LockBetPolicies(ctx context.Context, tx pgx.Tx, brand string) (Policy, error) {
	if tx == nil || !policyUUID.MatchString(brand) {
		return Policy{}, ErrInvalid
	}
	policy, err := scanPolicy(tx.QueryRow(ctx, `SELECT `+policyFields+policyJoin+` WHERE p.brand_id=$1 FOR SHARE OF p`, brand))
	if err != nil {
		return Policy{}, err
	}
	if err = validateEvidence(ctx, tx, brand); err != nil {
		return policy, err
	}
	return policy, nil
}
