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
	RevokedCount string `json:"revoked_count"`
}

// Commissions is a current task-state inventory, not today's earnings,
// actual credits, wallet balances or authorization to pay a ready cycle.
type Commissions struct {
	DiscoveryPendingCount          string `json:"discovery_pending_count"`
	DiscoveryFailedCount           string `json:"discovery_failed_count"`
	CycleProcessingCount           string `json:"cycle_processing_count"`
	CycleWaitingCount              string `json:"cycle_waiting_count"`
	CycleReadyCount                string `json:"cycle_ready_count"`
	CycleStaleCount                string `json:"cycle_stale_count"`
	CycleFailedCount               string `json:"cycle_failed_count"`
	PaymentAwaitingApprovalCount   string `json:"payment_awaiting_approval_count"`
	PaymentProcessingCount         string `json:"payment_processing_count"`
	PaymentBlockedCount            string `json:"payment_blocked_count"`
	PaymentFailedCount             string `json:"payment_failed_count"`
	PlanProcessingCount            string `json:"plan_processing_count"`
	PlanReadyCount                 string `json:"plan_ready_count"`
	PlanFailedCount                string `json:"plan_failed_count"`
	ExecutionAwaitingApprovalCount string `json:"execution_awaiting_approval_count"`
	ExecutionProcessingCount       string `json:"execution_processing_count"`
	ExecutionPausedCount           string `json:"execution_paused_count"`
	ExecutionFailedCount           string `json:"execution_failed_count"`
}

type Snapshot struct {
	BrandID     string               `json:"brand_id"`
	SnapshotAt  time.Time            `json:"snapshot_at"`
	Timezone    string               `json:"timezone"`
	DayFrom     time.Time            `json:"day_from"`
	Brand       Section[Brand]       `json:"brand"`
	Orders      Section[Orders]      `json:"orders"`
	TodayBets   Section[TodayBets]   `json:"today_bets"`
	Recharges   Section[Recharges]   `json:"recharges"`
	Ledger      Section[Ledger]      `json:"ledger"`
	Balances    Section[Balances]    `json:"balances"`
	Withdrawals Section[Withdrawals] `json:"withdrawals"`
	Commissions Section[Commissions] `json:"commissions"`
	Rewards     Section[Rewards]     `json:"rewards"`
}

func CanView(a access.Account, brand, resource string) bool {
	return access.Authorize(a, resource, "view", access.ScopeBrand, brand) || access.Authorize(a, resource, "view", access.ScopePlatform, "")
}
func Allowed(a access.Account, brand string) bool {
	for _, resource := range []string{"brand", "bet", "report_betting", "recharge", "report_ledger", "wallet", "withdrawal", "reward", "commission"} {
		if CanView(a, brand, resource) {
			return true
		}
	}
	return false
}
