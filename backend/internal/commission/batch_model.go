package commission

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

var (
	ErrCycleState      = errors.New("commission cycle state conflict")
	ErrCycleVersion    = errors.New("commission cycle version conflict")
	cycleCanonicalUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

const maxCycleVersion int64 = 1<<53 - 1

// CreateCycleInput is the closed request shape for creating an immutable cycle.
type CreateCycleInput struct {
	AnchorOrderID string `json:"anchor_order_id"`
	Reason        string `json:"reason"`
}

func (in *CreateCycleInput) UnmarshalJSON(data []byte) error {
	if in == nil || closedObject(data, "anchor_order_id", "reason") != nil {
		return ErrInvalid
	}
	var value struct {
		AnchorOrderID string `json:"anchor_order_id"`
		Reason        string `json:"reason"`
	}
	if json.Unmarshal(data, &value) != nil {
		return ErrInvalid
	}
	*in = CreateCycleInput(value)
	return in.Validate()
}

func (in CreateCycleInput) Validate() error {
	if !cycleCanonicalUUID.MatchString(in.AnchorOrderID) || !validCycleReason(in.Reason) {
		return ErrInvalid
	}
	return nil
}

// RetryCycleInput carries the optimistic version and audit reason for a retry.
type RetryCycleInput struct {
	Version int64  `json:"version"`
	Reason  string `json:"reason"`
}

func (in *RetryCycleInput) UnmarshalJSON(data []byte) error {
	if in == nil || closedObject(data, "version", "reason") != nil {
		return ErrInvalid
	}
	var value struct {
		Version int64  `json:"version"`
		Reason  string `json:"reason"`
	}
	if json.Unmarshal(data, &value) != nil {
		return ErrInvalid
	}
	*in = RetryCycleInput(value)
	return in.Validate()
}

func (in RetryCycleInput) Validate() error {
	if in.Version < 1 || in.Version > maxCycleVersion || !validCycleReason(in.Reason) {
		return ErrInvalid
	}
	return nil
}

func validCycleReason(reason string) bool {
	return utf8.ValidString(reason) && reason != "" && reason == strings.TrimSpace(reason) &&
		len([]byte(reason)) <= 500 && !strings.ContainsRune(reason, '\x00')
}

// AllowedCycle applies the commission cycle action policy. Viewing can be
// granted at brand or platform scope; mutations always require brand scope.
func AllowedCycle(account access.Account, brand, action string) bool {
	if !cycleCanonicalUUID.MatchString(brand) {
		return false
	}
	view := access.Authorize(account, "commission", "view", access.ScopeBrand, brand) ||
		access.Authorize(account, "commission", "view", access.ScopePlatform, "")
	if action == "view" {
		return view
	}
	return (action == "run" || action == "retry") && view && !account.SuperAdmin &&
		access.Authorize(account, "commission", action, access.ScopeBrand, brand)
}

// Cycle contains the API projection of a cycle and its current run. Counts are
// decimal strings to preserve PostgreSQL integer precision in JSON clients.
type Cycle struct {
	ID                 string    `json:"id"`
	BrandID            string    `json:"brand_id"`
	WindowFrom         time.Time `json:"window_from"`
	WindowTo           time.Time `json:"window_to"`
	AnchorOrderID      string    `json:"anchor_order_id"`
	Calendar           Calendar  `json:"calendar"`
	State              string    `json:"state"`
	Version            int64     `json:"version"`
	TargetCount        string    `json:"target_count"`
	ScanComplete       bool      `json:"scan_complete"`
	CurrentRunID       *string   `json:"current_run_id"`
	CurrentGeneration  *string   `json:"current_generation"`
	EvidenceEpoch      *string   `json:"evidence_epoch"`
	EvidenceCurrent    bool      `json:"evidence_current"`
	CalculatedCount    string    `json:"calculated_count"`
	EarningCount       string    `json:"earning_count"`
	TotalPoints        string    `json:"total_points"`
	CreatedBy          *string   `json:"created_by"`
	CreationActorType  string    `json:"creation_actor_type"`
	Reason             string    `json:"reason"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	LastErrorCode      *string   `json:"last_error_code"`
	CreationAuditLogID string    `json:"creation_audit_log_id"`
}

type CycleListPage struct {
	BrandID    string  `json:"brand_id"`
	Items      []Cycle `json:"items"`
	TotalCount string  `json:"total_count"`
	Limit      int     `json:"limit"`
	Offset     int     `json:"offset"`
}

type Earning struct {
	ID          string        `json:"id"`
	BrandID     string        `json:"brand_id"`
	CycleID     string        `json:"cycle_id"`
	RunID       string        `json:"run_id"`
	AgentID     string        `json:"agent_id"`
	MemberID    string        `json:"member_id"`
	ExactAmount ExactAmount   `json:"exact_amount"`
	Points      points.Amount `json:"points"`
	CreatedAt   time.Time     `json:"created_at"`
}

type EarningPage struct {
	BrandID    string    `json:"brand_id"`
	CycleID    string    `json:"cycle_id"`
	Items      []Earning `json:"items"`
	TotalCount string    `json:"total_count"`
	Limit      int       `json:"limit"`
	Offset     int       `json:"offset"`
}

func validCyclePage(brand string, limit, offset int) bool {
	return cycleCanonicalUUID.MatchString(brand) && limit >= 1 && limit <= 100 && offset >= 0 && offset <= 1_000_000
}
