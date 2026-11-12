package withdrawal

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"time"
)

// ConvertTx consumes eligible points and their turnover cycle atomically with
// the caller's currency credit. Cash withdrawal never evaluates points again.
func (s OrderService) ConvertTx(ctx context.Context, tx pgx.Tx, brand, member, order string, amount points.Amount, meta points.Metadata) (points.Entry, json.RawMessage, error) {
	fail := func(e error) (points.Entry, json.RawMessage, error) { return points.Entry{}, nil, e }
	if amount <= 0 || s.Eligibility == nil {
		return fail(ErrEligibilityNotConfigured)
	}
	var allowed bool
	if e := tx.QueryRow(ctx, `SELECT b.status<>'disabled' AND m.status='normal' AND u.status='active' AND u.id=$3 FROM brands b JOIN brand_members m ON m.brand_id=b.id JOIN global_users u ON u.id=m.global_user_id WHERE b.id=$1 AND m.id=$2 FOR SHARE OF b,m,u`, brand, member, meta.ActorID).Scan(&allowed); e != nil {
		return fail(e)
	}
	if !allowed {
		return fail(ErrDenied)
	}
	policy, e := scanBrand(tx.QueryRow(ctx, `SELECT `+brandFields+` FROM brand_withdrawal_policies WHERE brand_id=$1 FOR SHARE`, brand))
	if e != nil {
		return fail(e)
	}
	if !policy.Config.Enabled || amount%policy.PointsStep != 0 || amount < policy.Config.MinPoints || policy.Config.MaxPoints != nil && amount > *policy.Config.MaxPoints {
		return fail(ErrIneligible)
	}

	if _, e = s.Points.LockedPolicy(ctx, tx, brand); e != nil {
		return fail(e)
	}
	wallet, e := s.Points.LockedSnapshot(ctx, tx, brand, member)
	if e != nil {
		return fail(e)
	}
	var active bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM retention_withdrawal_orders() WHERE brand_id=$1 AND member_id=$2 AND state IN('reviewing','processing'))`, brand, member).Scan(&active); e != nil {
		return fail(e)
	}
	if active {
		return fail(ErrActiveOrder)
	}
	// Only explicitly permitted sources can enter a withdrawable currency balance.
	eligible := wallet.BySource
	allowedSources := map[string]bool{}
	for _, src := range policy.Config.AllowedSources {
		allowedSources[src] = true
	}
	for _, src := range []string{"recharge", "winning", "gift", "commission"} {
		i, _ := points.SourceIndex(src)
		if !allowedSources[src] {
			eligible[i][0] = 0
		}
	}
	allocation, e := eligible.Allocate(amount, "available")
	if e != nil {
		return fail(e)
	}
	var from *time.Time
	var version int64
	e = tx.QueryRow(ctx, `SELECT cutoff_at,cutoff_version FROM (SELECT cutoff_at,cutoff_version FROM cash_turnover_cycles WHERE brand_id=$1 AND member_id=$2 UNION ALL SELECT cutoff_at,cutoff_version FROM withdrawal_turnover_cycles WHERE brand_id=$1 AND member_id=$2) c ORDER BY cutoff_version DESC LIMIT 1`, brand, member).Scan(&from, &version)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return fail(e)
	}
	var at time.Time
	if e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&at); e != nil {
		return fail(e)
	}
	base, e := turnoverBaseSnapshot(wallet)
	if e != nil {
		return fail(e)
	}
	q, e := s.Eligibility.Check(ctx, tx, EligibilityInput{BrandID: brand, MemberID: member, Policy: policy, Wallet: wallet, TurnoverBase: base, Points: amount, SourceAllocation: allocation, CycleFromAt: from, CycleFromVersion: version, CutoffAt: at, CutoffVersion: wallet.Version})
	if e != nil {
		return fail(e)
	}
	if !q.Allowed {
		return fail(ErrIneligible)
	}
	delta, e := points.AllocationDelta(allocation, "available", "")
	if e != nil {
		return fail(e)
	}
	entry, e := s.Points.Post(ctx, tx, points.Change{BrandID: brand, MemberID: member, EntryType: "currency_exchange_debit", ReferenceType: "cash_order", ReferenceID: order, OperationKey: "cash-exchange:" + order, ActorType: meta.ActorType, ActorID: meta.ActorID, RequestID: meta.RequestID, IP: meta.IP, Delta: delta, Allocation: allocation})
	if e != nil {
		return fail(e)
	}
	evidence, e := json.Marshal(map[string]any{"qualification": json.RawMessage(q.Evidence), "turnover_base": base, "policy": policy, "source_allocation": allocation, "from_version": version, "cutoff_version": entry.Version})
	if e != nil {
		return fail(e)
	}
	_, e = tx.Exec(ctx, `INSERT INTO cash_turnover_cycles(brand_id,member_id,cutoff_at,cutoff_version,order_id) VALUES($1,$2,$3,$4,$5) ON CONFLICT(brand_id,member_id) DO UPDATE SET cutoff_at=EXCLUDED.cutoff_at,cutoff_version=EXCLUDED.cutoff_version,order_id=EXCLUDED.order_id`, brand, member, at, entry.Version, order)
	return entry, evidence, e
}
