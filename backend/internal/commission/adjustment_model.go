package commission

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

var (
	ErrAdjustmentState        = errors.New("commission adjustment state conflict")
	ErrAdjustmentVersion      = errors.New("commission adjustment version conflict")
	ErrAdjustmentInsufficient = errors.New("commission adjustment funds insufficient")
)

// AdjustmentInput is the complete closed input for one human correction.
// Version identifies the current adjustment head; Points is the desired net
// commission amount, not a delta.
type AdjustmentInput struct {
	Version int64         `json:"version"`
	Points  points.Amount `json:"points"`
	Reason  string        `json:"reason"`
}

func (in *AdjustmentInput) UnmarshalJSON(data []byte) error {
	if in == nil || closedObject(data, "version", "points", "reason") != nil {
		return ErrInvalid
	}
	var value struct {
		Version int64         `json:"version"`
		Points  points.Amount `json:"points"`
		Reason  string        `json:"reason"`
	}
	if json.Unmarshal(data, &value) != nil {
		return ErrInvalid
	}
	*in = AdjustmentInput(value)
	return in.Validate()
}

func (in AdjustmentInput) Validate() error {
	if in.Version < 1 || in.Version > maxCycleVersion || in.Points < 0 ||
		!utf8.ValidString(in.Reason) || in.Reason == "" || in.Reason != strings.TrimSpace(in.Reason) ||
		len([]byte(in.Reason)) > 500 || strings.ContainsRune(in.Reason, '\x00') {
		return ErrInvalid
	}
	return nil
}

// AllowedAdjustment keeps adjustment permission separate from commission
// execution. Reads allow brand or platform view; writes require an ordinary
// administrator with both brand-scoped adjustment write and commission view.
func AllowedAdjustment(account access.Account, brand, action string) bool {
	if !cycleCanonicalUUID.MatchString(brand) {
		return false
	}
	view := access.Authorize(account, "commission", "view", access.ScopeBrand, brand) ||
		access.Authorize(account, "commission", "view", access.ScopePlatform, "")
	if action == "view" {
		return view
	}
	return action == "write" && account.Type == access.AccountAdmin && !account.SuperAdmin && view &&
		access.Authorize(account, "commission_adjustment", "write", access.ScopeBrand, brand)
}

// Adjustment is an immutable, sanitized manual correction record.
type Adjustment struct {
	ID                 string        `json:"id"`
	BrandID            string        `json:"brand_id"`
	TargetID           string        `json:"target_id"`
	PaymentID          string        `json:"payment_id"`
	Version            int64         `json:"version"`
	PointsBefore       points.Amount `json:"points_before"`
	PointsAfter        points.Amount `json:"points_after"`
	DeltaPoints        points.Amount `json:"delta_points"`
	LedgerEntryID      string        `json:"ledger_entry_id"`
	AuditLogID         string        `json:"audit_log_id"`
	CreatedBy          string        `json:"created_by"`
	Reason             string        `json:"reason"`
	PointPolicyVersion string        `json:"point_policy_version"`
	CreatedAt          time.Time     `json:"created_at"`
}

// Target is the privacy-limited view of a payout target and its current
// adjustment head. It intentionally excludes account, rule, and wallet data.
type Target struct {
	ID                string         `json:"id"`
	BrandID           string         `json:"brand_id"`
	PaymentID         string         `json:"payment_id"`
	EarningID         string         `json:"earning_id"`
	AgentID           string         `json:"agent_id"`
	MemberID          string         `json:"member_id"`
	OriginalPoints    points.Amount  `json:"original_points"`
	State             string         `json:"state"`
	LedgerEntryID     *string        `json:"ledger_entry_id"`
	PaidAt            *time.Time     `json:"paid_at"`
	AdjustmentVersion *int64         `json:"adjustment_version"`
	AdjustedPoints    *points.Amount `json:"adjusted_points"`
	LastAdjustmentID  *string        `json:"last_adjustment_id"`
}

type TargetPage struct {
	BrandID    string   `json:"brand_id"`
	PaymentID  string   `json:"payment_id"`
	Items      []Target `json:"items"`
	TotalCount string   `json:"total_count"`
	Limit      int      `json:"limit"`
	Offset     int      `json:"offset"`
}

type AdjustmentPage struct {
	BrandID    string       `json:"brand_id"`
	TargetID   string       `json:"target_id"`
	Items      []Adjustment `json:"items"`
	TotalCount string       `json:"total_count"`
	Limit      int          `json:"limit"`
	Offset     int          `json:"offset"`
}

func validAdjustmentPage(brand string, limit, offset int) bool {
	return cycleCanonicalUUID.MatchString(brand) && limit >= 1 && limit <= 100 && offset >= 0 && offset <= 1_000_000
}
