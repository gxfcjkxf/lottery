// Package betting owns transactional, brand-isolated lottery stakes and refunds.
package betting

import (
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

var (
	ErrInvalid  = errors.New("invalid betting input")
	ErrDenied   = errors.New("betting operation denied")
	ErrNotFound = errors.New("betting resource not found")
	ErrVersion  = errors.New("betting configuration version changed")
	ErrClosed   = errors.New("betting period not accepting this operation")
	ErrLimit    = errors.New("betting limit exceeded")
	ErrState    = errors.New("bet order state conflict")
)

type Service struct{ DB *pgxpool.Pool }
type Input struct {
	PeriodID       string          `json:"period_id"`
	PlayID         string          `json:"play_id"`
	RuleVersionID  string          `json:"rule_version_id"`
	Selection      rules.Selection `json:"selection"`
	Multiplier     points.Amount   `json:"multiplier"`
	PolicyVersions *PolicyVersions `json:"policy_versions,omitempty"`
}
type Order struct {
	ID                  string              `json:"id"`
	BrandID             string              `json:"brand_id"`
	UserID              string              `json:"global_user_id"`
	MemberID            string              `json:"brand_member_id"`
	AccountID           string              `json:"account_id"`
	GameID              string              `json:"game_id"`
	PeriodID            string              `json:"period_id"`
	PlayID              string              `json:"play_id"`
	RuleVersionID       string              `json:"rule_version_id"`
	DefinitionHash      string              `json:"definition_hash"`
	Definition          rules.Definition    `json:"definition_snapshot"`
	Status              string              `json:"status"`
	Version             int64               `json:"version"`
	SelectionRaw        rules.Selection     `json:"selection_raw"`
	SelectionNormalized rules.Selection     `json:"selection_normalized"`
	Expanded            []rules.Selection   `json:"expanded_bets"`
	UnitPoints          points.Amount       `json:"unit_points"`
	CombinationCount    int                 `json:"combination_count"`
	Multiplier          points.Amount       `json:"multiplier"`
	TotalPoints         points.Amount       `json:"total_points"`
	Allocation          []points.Allocation `json:"deduction_allocation"`
	Policy              EffectivePolicy     `json:"policy_snapshot"`
	PolicyVersions      PolicyVersions      `json:"policy_versions"`
	DebitEntryID        string              `json:"debit_entry_id"`
	RefundEntryID       string              `json:"refund_entry_id,omitempty"`
	ClientKey           string              `json:"client_key"`
	PlacedAt            time.Time           `json:"placed_at"`
	CancelledAt         *time.Time          `json:"cancelled_at,omitempty"`
	CancelReason        string              `json:"cancel_reason,omitempty"`
}
