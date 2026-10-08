// Package workbench provides an authorized, single-statement operations snapshot.
package workbench

import (
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"time"
)

var ErrInvalid = errors.New("invalid workbench scope")
var ErrDenied = errors.New("workbench access denied")
var ErrNotFound = errors.New("workbench brand not found")

type Section[T any] struct {
	Status string `json:"status"`
	Data   *T     `json:"data"`
}
type Brand struct {
	Name  string `json:"name"`
	Code  string `json:"code"`
	State string `json:"state"`
}
type Periods struct {
	Pending       string `json:"pending"`
	Betting       string `json:"betting"`
	Closed        string `json:"closed"`
	WaitingDraw   string `json:"waiting_draw"`
	Drawn         string `json:"drawn"`
	Settling      string `json:"settling"`
	RefundPending string `json:"refund_pending"`
	RefundFailed  string `json:"refund_failed"`
}
type Orders struct {
	Placed   string `json:"placed"`
	Abnormal string `json:"abnormal"`
}
type TodayBets struct {
	OrderCount     string `json:"order_count"`
	StakePoints    string `json:"stake_points"`
	CancelledCount string `json:"cancelled_count"`
	AbnormalCount  string `json:"abnormal_count"`
}
type Settlement struct {
	Processing       string `json:"processing"`
	AwaitingApproval string `json:"awaiting_approval"`
	Paying           string `json:"paying"`
	Failed           string `json:"failed"`
}
type Recharges struct {
	PendingCount  string `json:"pending_count"`
	PendingPoints string `json:"pending_points"`
}
type Ledger struct {
	EntryCount          string `json:"entry_count"`
	NetPoints           string `json:"net_points"`
	RechargePoints      string `json:"recharge_points"`
	PrizeCreditPoints   string `json:"prize_credit_points"`
	PrizeReversalPoints string `json:"prize_reversal_points"`
	RefundPoints        string `json:"refund_points"`
}
type Balances struct {
	AccountCount     string `json:"account_count"`
	AvailablePoints  string `json:"available_points"`
	FrozenPoints     string `json:"frozen_points"`
	WithdrawalPoints string `json:"withdrawal_points"`
	TotalPoints      string `json:"total_points"`
}
type Withdrawals struct {
	ReviewingCount   string `json:"reviewing_count"`
	ReviewingPoints  string `json:"reviewing_points"`
	ProcessingCount  string `json:"processing_count"`
	ProcessingPoints string `json:"processing_points"`
}

// Rewards counts the current states of all saved orders, not wallet balances,
// successful revocation attempts, or today's ledger posting amounts.
type Rewards struct {
	GrantedCount string `json:"granted_count"`
	PendingCount string `json:"pending_count"`
	RevokedCount string `json:"revoked_count"`
}
type ReconciliationJob struct {
	ID              string     `json:"id"`
	State           string     `json:"state"`
	CreatedAt       time.Time  `json:"created_at"`
	CompletedAt     *time.Time `json:"completed_at"`
	TargetCount     string     `json:"target_count"`
	CheckedCount    string     `json:"checked_count"`
	RepairableCount string     `json:"repairable_count"`
	CorruptCount    string     `json:"corrupt_count"`
	FailedCount     string     `json:"failed_count"`
}
type Reconciliation struct {
	LatestJob *ReconciliationJob `json:"latest_job"`
}
type Sources struct {
	AdapterState      string     `json:"adapter_state"`
	ConfiguredGames   string     `json:"configured_games"`
	EnabledAPISources string     `json:"enabled_api_sources"`
	EnabledDOMSources string     `json:"enabled_dom_sources"`
	AttemptsToday     string     `json:"attempts_today"`
	FailedToday       string     `json:"failed_today"`
	NoDataToday       string     `json:"no_data_today"`
	LastAttemptAt     *time.Time `json:"last_attempt_at"`
}
type Snapshot struct {
	BrandID        string                  `json:"brand_id"`
	SnapshotAt     time.Time               `json:"snapshot_at"`
	Timezone       string                  `json:"timezone"`
	DayFrom        time.Time               `json:"day_from"`
	Brand          Section[Brand]          `json:"brand"`
	Periods        Section[Periods]        `json:"periods"`
	Orders         Section[Orders]         `json:"orders"`
	TodayBets      Section[TodayBets]      `json:"today_bets"`
	Settlement     Section[Settlement]     `json:"settlement"`
	Recharges      Section[Recharges]      `json:"recharges"`
	Ledger         Section[Ledger]         `json:"ledger"`
	Balances       Section[Balances]       `json:"balances"`
	Reconciliation Section[Reconciliation] `json:"reconciliation"`
	Sources        Section[Sources]        `json:"sources"`
	Withdrawals    Section[Withdrawals]    `json:"withdrawals"`
	Commissions    Section[struct{}]       `json:"commissions"`
	Rewards        Section[Rewards]        `json:"rewards"`
}

func CanView(a access.Account, brand, resource string) bool {
	return access.Authorize(a, resource, "view", access.ScopeBrand, brand) || access.Authorize(a, resource, "view", access.ScopePlatform, "")
}
func Allowed(a access.Account, brand string) bool {
	for _, resource := range []string{"brand", "period", "bet", "report_betting", "settlement", "recharge", "report_ledger", "wallet", "draw_source", "withdrawal", "reward"} {
		if CanView(a, brand, resource) {
			return true
		}
	}
	return false
}
