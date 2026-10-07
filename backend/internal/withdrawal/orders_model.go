package withdrawal

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

var (
	ErrEligibilityNotConfigured = errors.New("withdrawal eligibility checker is not configured")
	ErrIneligible               = errors.New("withdrawal request is ineligible")
	ErrActiveOrder              = errors.New("withdrawal order already active")
	ErrOrderState               = errors.New("invalid withdrawal order state transition")
)

const maxSafeInteger int64 = 9007199254740991

type Order struct {
	ID                  string              `json:"id"`
	BrandID             string              `json:"brand_id"`
	MemberID            string              `json:"member_id"`
	AccountID           string              `json:"account_id"`
	Points              points.Amount       `json:"points"`
	State               string              `json:"state"`
	Version             int64               `json:"version"`
	SourceAllocation    []points.Allocation `json:"source_allocation"`
	PolicySnapshot      BrandPolicy         `json:"policy_snapshot"`
	EligibilityEvidence json.RawMessage     `json:"eligibility_evidence"`
	CycleFromAt         *time.Time          `json:"cycle_from_at"`
	CycleFromVersion    int64               `json:"cycle_from_version"`
	ReserveVersion      int64               `json:"reserve_version"`
	ReserveEntryID      string              `json:"reserve_entry_id"`
	ReleaseEntryID      string              `json:"release_entry_id"`
	PaidEntryID         string              `json:"paid_entry_id"`
	CreatedAt           time.Time           `json:"created_at"`
	UpdatedAt           time.Time           `json:"updated_at"`
	ReviewedAt          *time.Time          `json:"reviewed_at"`
	CompletedAt         *time.Time          `json:"completed_at"`
	DecisionReason      string              `json:"decision_reason"`
	AuditLogID          string              `json:"audit_log_id"`
}

type OrderInput struct {
	Points           points.Amount       `json:"points"`
	SourceAllocation []points.Allocation `json:"source_allocation"`
	ClientKey        string              `json:"client_key"`
}

type ActionInput struct {
	Version   int64  `json:"version"`
	ClientKey string `json:"client_key"`
	Reason    string `json:"reason"`
}

type EligibilityInput struct {
	BrandID          string
	MemberID         string
	Policy           BrandPolicy
	Wallet           points.Wallet
	Points           points.Amount
	SourceAllocation []points.Allocation
	CycleFromAt      *time.Time
	CycleFromVersion int64
	CutoffAt         time.Time
	CutoffVersion    int64
}

type EligibilityDecision struct {
	Allowed  bool
	Evidence json.RawMessage
}

type EligibilityChecker interface {
	Check(context.Context, pgx.Tx, EligibilityInput) (EligibilityDecision, error)
}

func ValidateOrderInput(in OrderInput) error {
	if !validClientKey(in.ClientKey) || in.Points <= 0 || len(in.SourceAllocation) < 1 || len(in.SourceAllocation) > 3 {
		return ErrInvalid
	}

	var total int64
	lastSourceIndex := -1
	for _, allocation := range in.SourceAllocation {
		sourceIndex, err := points.SourceIndex(allocation.Source)
		if err != nil || sourceIndex <= lastSourceIndex || allocation.State != "available" || allocation.Points <= 0 {
			return ErrInvalid
		}
		amount := int64(allocation.Points)
		if total > math.MaxInt64-amount {
			return ErrInvalid
		}
		total += amount
		lastSourceIndex = sourceIndex
	}
	if total != int64(in.Points) {
		return ErrInvalid
	}
	return nil
}

func ValidateActionInput(in ActionInput) error {
	if in.Version < 1 || in.Version > maxSafeInteger || !validClientKey(in.ClientKey) || !utf8.ValidString(in.Reason) {
		return ErrInvalid
	}
	reason := strings.TrimSpace(in.Reason)
	if len(reason) < 1 || len(reason) > 500 {
		return ErrInvalid
	}
	return nil
}

func TransitionTarget(state, action string) (string, error) {
	switch action {
	case "approve":
		if state == "reviewing" {
			return "processing", nil
		}
	case "reject":
		if state == "reviewing" {
			return "rejected", nil
		}
	case "cancel":
		if state == "reviewing" || state == "processing" {
			return "cancelled", nil
		}
	case "fail":
		if state == "processing" {
			return "failed", nil
		}
	case "mark_paid":
		if state == "processing" {
			return "paid", nil
		}
	}
	return "", ErrOrderState
}

func validClientKey(key string) bool {
	if len(key) < 8 || len(key) > 128 {
		return false
	}
	for i := 0; i < len(key); i++ {
		c := key[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == ':' || c == '.' || c == '-' {
			continue
		}
		return false
	}
	return true
}
