package commission

import (
	"errors"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

var (
	ErrCorrectionExecutionState    = errors.New("commission correction execution state conflict")
	ErrCorrectionExecutionVersion  = errors.New("commission correction execution version conflict")
	ErrCorrectionExecutionEvidence = errors.New("commission correction execution evidence unavailable")
)

const (
	CorrectionExecutionAwaitingApproval = "awaiting_approval"
	CorrectionExecutionApplying         = "applying"
	CorrectionExecutionCompleted        = "completed"
	CorrectionExecutionPaused           = "paused"
	CorrectionExecutionFailed           = "failed"
	CorrectionExecutionStale            = "stale"
	CorrectionExecutionTargetPending    = "pending"
	CorrectionExecutionTargetApplied    = "applied"
)

// CorrectionExecutionPolicy is a separate, default-disabled brand gate for
// financial correction execution. Enabling it is an explicit audited opt-in;
// migration or plan preparation must never trigger automatic recovery.
type CorrectionExecutionPolicy struct {
	BrandID    string    `json:"brand_id"`
	Version    int64     `json:"version"`
	Enabled    bool      `json:"enabled"`
	AuditLogID string    `json:"audit_log_id"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// CorrectionExecution is the persisted execution envelope for an approved
// correction plan. PayoutMode preserves the source fact: mixed remains mixed
// and requires fresh manual approval in the later approval workflow. The
// cycle hold describes the current whole-cycle hold, not historical state.
type CorrectionExecution struct {
	ID                  string    `json:"id"`
	BrandID             string    `json:"brand_id"`
	CycleID             string    `json:"cycle_id"`
	PlanID              string    `json:"plan_id"`
	RunID               string    `json:"run_id"`
	PayoutMode          string    `json:"payout_mode"`
	State               string    `json:"state"`
	Version             int64     `json:"version"`
	PlanVersion         int64     `json:"plan_version"`
	EvidenceEpoch       string    `json:"evidence_epoch"`
	CreditPoints        string    `json:"credit_points"`
	DebitPoints         string    `json:"debit_points"`
	NetPoints           string    `json:"net_points"`
	AppliedCreditPoints string    `json:"applied_credit_points"`
	AppliedDebitPoints  string    `json:"applied_debit_points"`
	TargetCount         string    `json:"target_count"`
	AppliedCount        string    `json:"applied_count"`
	PausedPlanTargetID  *string   `json:"paused_plan_target_id"`
	LastErrorCode       *string   `json:"last_error_code"`
	CreationAuditLogID  string    `json:"creation_audit_log_id"`
	LastAuditLogID      string    `json:"last_audit_log_id"`
	ApprovedBy          *string   `json:"approved_by"`
	ApprovalActorType   *string   `json:"approval_actor_type"`
	ApprovalAuditLogID  *string   `json:"approval_audit_log_id"`
	CycleHoldActive     bool      `json:"cycle_hold_active"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// CorrectionExecutionTarget exposes only the beneficiary and exact signed
// point difference, plus immutable posting/audit identifiers when applied.
// AppliedAt and posting witness fields are null until the target is applied.
type CorrectionExecutionTarget struct {
	ID               string        `json:"id"`
	BrandID          string        `json:"brand_id"`
	ExecutionID      string        `json:"execution_id"`
	PlanTargetID     string        `json:"plan_target_id"`
	AgentID          string        `json:"agent_id"`
	MemberID         string        `json:"member_id"`
	PointsBefore     points.Amount `json:"points_before"`
	PointsAfter      points.Amount `json:"points_after"`
	DeltaPoints      points.Amount `json:"delta_points"`
	State            string        `json:"state"`
	LedgerEntryID    *string       `json:"ledger_entry_id"`
	AuditLogID       *string       `json:"audit_log_id"`
	FinancialVersion *int64        `json:"financial_version"`
	CreatedAt        time.Time     `json:"created_at"`
	AppliedAt        *time.Time    `json:"applied_at"`
}

type CorrectionExecutionPage struct {
	BrandID    string                `json:"brand_id"`
	Items      []CorrectionExecution `json:"items"`
	TotalCount string                `json:"total_count"`
	Limit      int                   `json:"limit"`
	Offset     int                   `json:"offset"`
}

type CorrectionExecutionTargetPage struct {
	BrandID     string                      `json:"brand_id"`
	ExecutionID string                      `json:"execution_id"`
	Items       []CorrectionExecutionTarget `json:"items"`
	TotalCount  string                      `json:"total_count"`
	Limit       int                         `json:"limit"`
	Offset      int                         `json:"offset"`
}

// AllowedCorrectionExecution requires explicit commission view access for
// every action. Mutating actions additionally require an ordinary
// administrator and their distinct brand-scoped execution permission.
// Correction-plan retry permission never grants financial execution retry.
func AllowedCorrectionExecution(account access.Account, brand, action string) bool {
	if !cycleCanonicalUUID.MatchString(brand) {
		return false
	}
	view := access.Authorize(account, "commission", "view", access.ScopeBrand, brand) ||
		access.Authorize(account, "commission", "view", access.ScopePlatform, "")
	if action == "view" {
		return view
	}
	if !view || account.Type != access.AccountAdmin || account.SuperAdmin {
		return false
	}
	var permission string
	switch action {
	case "approve":
		permission = "approve"
	case "continue":
		permission = "continue"
	case "retry":
		permission = "execute_retry"
	case "policy_write":
		return access.Authorize(account, "commission_correction_policy", "write", access.ScopeBrand, brand)
	default:
		return false
	}
	return access.Authorize(account, "commission_correction", permission, access.ScopeBrand, brand)
}
