package commission

import (
	"errors"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

var (
	ErrCorrectionPlanState    = errors.New("commission correction plan state conflict")
	ErrCorrectionPlanVersion  = errors.New("commission correction plan version conflict")
	ErrCorrectionPlanEvidence = errors.New("commission correction plan evidence unavailable")
	ErrCorrectionPlanLimit    = errors.New("commission correction plan limit exceeded")
)

const (
	CorrectionPlanPlanning = "planning"
	CorrectionPlanReady    = "ready"
	CorrectionPlanBlocked  = "blocked"
	CorrectionPlanFailed   = "failed"
	CorrectionPlanStale    = "stale"
	PayoutMixed            = "mixed"
	PayoutNone             = "none"
)

// CorrectionPlan is a frozen preview for a commission payment with paid
// credit, including partially paid targets; the entire payment need not be
// fully paid. It compares the paid basis with a replacement run. It is a plan
// only; this model contains no wallet or ledger mutation behavior.
type CorrectionPlan struct {
	ID                 string    `json:"id"`
	BrandID            string    `json:"brand_id"`
	CycleID            string    `json:"cycle_id"`
	PaymentID          string    `json:"payment_id"`
	RunID              string    `json:"run_id"`
	State              string    `json:"state"`
	PayoutMode         string    `json:"payout_mode"`
	Version            int64     `json:"version"`
	EvidenceEpoch      string    `json:"evidence_epoch"`
	BeforePoints       string    `json:"before_points"`
	CalculatedPoints   string    `json:"calculated_points"`
	CreditPoints       *string   `json:"credit_points"`
	DebitPoints        *string   `json:"debit_points"`
	NetPoints          *string   `json:"net_points"`
	TargetCount        string    `json:"target_count"`
	PlannedCount       string    `json:"planned_count"`
	CreationAuditLogID string    `json:"creation_audit_log_id"`
	LastAuditLogID     string    `json:"last_audit_log_id"`
	LastErrorCode      *string   `json:"last_error_code"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// CorrectionPlanTarget is the privacy-limited planned change for one saved
// commission beneficiary. Amounts use points.Amount's exact string JSON.
type CorrectionPlanTarget struct {
	ID                         string        `json:"id"`
	BrandID                    string        `json:"brand_id"`
	PlanID                     string        `json:"plan_id"`
	AgentID                    string        `json:"agent_id"`
	MemberID                   string        `json:"member_id"`
	OriginalTargetID           *string       `json:"original_target_id"`
	EarningID                  *string       `json:"earning_id"`
	AdjustmentVersion          *int64        `json:"adjustment_version"`
	PreviousCorrectionTargetID *string       `json:"previous_correction_target_id"`
	FinancialVersion           *int64        `json:"financial_version"`
	PointsBefore               points.Amount `json:"points_before"`
	PointsAfter                points.Amount `json:"points_after"`
	DeltaPoints                points.Amount `json:"delta_points"`
	CreationAuditLogID         string        `json:"creation_audit_log_id"`
	CreatedAt                  time.Time     `json:"created_at"`
}

type CorrectionPlanPage struct {
	BrandID    string           `json:"brand_id"`
	Items      []CorrectionPlan `json:"items"`
	TotalCount string           `json:"total_count"`
	Limit      int              `json:"limit"`
	Offset     int              `json:"offset"`
}

type CorrectionPlanTargetPage struct {
	BrandID    string                 `json:"brand_id"`
	PlanID     string                 `json:"plan_id"`
	Items      []CorrectionPlanTarget `json:"items"`
	TotalCount string                 `json:"total_count"`
	Limit      int                    `json:"limit"`
	Offset     int                    `json:"offset"`
}

// AllowedCorrectionPlan keeps preview reads under existing commission view
// grants. Retry is a separate brand-scoped permission and requires ordinary
// administrator status plus view access.
func AllowedCorrectionPlan(account access.Account, brand, action string) bool {
	if !cycleCanonicalUUID.MatchString(brand) {
		return false
	}
	view := access.Authorize(account, "commission", "view", access.ScopeBrand, brand) ||
		access.Authorize(account, "commission", "view", access.ScopePlatform, "")
	if action == "view" {
		return view
	}
	return action == "retry" && account.Type == access.AccountAdmin && !account.SuperAdmin && view &&
		access.Authorize(account, "commission_correction", "retry", access.ScopeBrand, brand)
}

// ComputeCorrectionDelta returns after-before without unsigned conversion or
// intermediate overflow. Both endpoints are valid nonnegative int64 points.
func ComputeCorrectionDelta(before, after points.Amount) (points.Amount, error) {
	if before < 0 || after < 0 {
		return 0, ErrInvalid
	}
	return after - before, nil
}

// MergeCorrectionMode combines original and replacement-run payout modes.
// It preserves mixed as a fact for the later approval flow; none contributes
// no constraint.
func MergeCorrectionMode(original, current string) (string, error) {
	if !validPaymentPayoutMode(original) || !validPaymentPayoutMode(current) {
		return "", ErrInvalid
	}
	if original == "mixed" || current == "mixed" {
		return PayoutMixed, nil
	}
	if original == "none" {
		return current, nil
	}
	if current == "none" || original == current {
		return original, nil
	}
	return "mixed", nil
}
