package commission

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
)

var (
	ErrPaymentState    = errors.New("commission payment state conflict")
	ErrPaymentVersion  = errors.New("commission payment version conflict")
	ErrPaymentEvidence = errors.New("commission payment evidence unavailable")
)

// PaymentPolicy is the brand's explicit commission payout switch. New brands
// start disabled and must opt in before a payout can be prepared.
type PaymentPolicy struct {
	BrandID    string    `json:"brand_id"`
	Version    int64     `json:"version"`
	Enabled    bool      `json:"enabled"`
	AuditLogID string    `json:"audit_log_id"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// PaymentPolicyInput is closed so the payout switch cannot be changed through
// an accidentally accepted or ignored request field.
type PaymentPolicyInput struct {
	Version int64  `json:"version"`
	Enabled bool   `json:"enabled"`
	Reason  string `json:"reason"`
}

func (in *PaymentPolicyInput) UnmarshalJSON(data []byte) error {
	if in == nil || closedObject(data, "version", "enabled", "reason") != nil {
		return ErrInvalid
	}
	var value struct {
		Version int64  `json:"version"`
		Enabled bool   `json:"enabled"`
		Reason  string `json:"reason"`
	}
	if json.Unmarshal(data, &value) != nil {
		return ErrInvalid
	}
	*in = PaymentPolicyInput(value)
	return in.Validate()
}

func (in PaymentPolicyInput) Validate() error {
	if in.Version < 1 || in.Version > maxCycleVersion || !validCycleReason(in.Reason) {
		return ErrInvalid
	}
	return nil
}

// Payment is a sanitized payout projection. Totals and counts are decimal
// strings so PostgreSQL integer precision is preserved for JSON clients.
type Payment struct {
	ID                 string    `json:"id"`
	BrandID            string    `json:"brand_id"`
	CycleID            string    `json:"cycle_id"`
	RunID              string    `json:"run_id"`
	State              string    `json:"state"`
	PayoutMode         string    `json:"payout_mode"`
	Version            int64     `json:"version"`
	EvidenceEpoch      string    `json:"evidence_epoch"`
	TotalPoints        string    `json:"total_points"`
	PaidPoints         string    `json:"paid_points"`
	TargetCount        string    `json:"target_count"`
	PaidCount          string    `json:"paid_count"`
	CreationAuditLogID string    `json:"creation_audit_log_id"`
	LastErrorCode      *string   `json:"last_error_code"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type PaymentPage struct {
	BrandID    string    `json:"brand_id"`
	Items      []Payment `json:"items"`
	TotalCount string    `json:"total_count"`
	Limit      int       `json:"limit"`
	Offset     int       `json:"offset"`
}

// AllowedPayment applies brand/platform view access and the narrower payout
// mutation permissions. Policy writes use a dedicated permission namespace.
func AllowedPayment(account access.Account, brand, action string) bool {
	if !cycleCanonicalUUID.MatchString(brand) {
		return false
	}
	view := access.Authorize(account, "commission", "view", access.ScopeBrand, brand) ||
		access.Authorize(account, "commission", "view", access.ScopePlatform, "")
	if action == "view" {
		return view
	}
	if account.Type != access.AccountAdmin || account.SuperAdmin || !view {
		return false
	}
	switch action {
	case "approve", "retry":
		return access.Authorize(account, "commission_payment", action, access.ScopeBrand, brand)
	case "policy_write":
		return access.Authorize(account, "commission_payment_policy", "write", access.ScopeBrand, brand)
	default:
		return false
	}
}

func validPaymentPage(brand string, limit, offset int) bool {
	return cycleCanonicalUUID.MatchString(brand) && limit >= 1 && limit <= 100 && offset >= 0 && offset <= 1_000_000
}

func validPaymentPayoutMode(mode string) bool {
	return mode == PayoutManual || mode == PayoutAutomatic || mode == "mixed" || mode == "none"
}

func validPaymentReason(reason string) bool {
	return strings.TrimSpace(reason) == reason && validCycleReason(reason)
}
