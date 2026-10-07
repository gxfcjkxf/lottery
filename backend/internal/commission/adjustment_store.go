package commission

import (
	"context"
	"errors"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

// AdjustCommissionTx applies a single administrator-approved, exact difference
// to a completed target. The original earning, target and payout stay immutable.
// It never compensates a changed draw generation or clears a blocked cycle.
func (s Service) AdjustCommissionTx(ctx context.Context, tx pgx.Tx, brand, target string, a access.Account, in AdjustmentInput, meta points.Metadata) (Adjustment, error) {
	if tx == nil || !canonicalUUID(brand) || !canonicalUUID(target) || !canonicalUUID(a.ID) || in.Validate() != nil || meta.ActorType != "admin" || meta.ActorID != a.ID || meta.RequestID == "" || len(meta.RequestID) > 80 {
		return Adjustment{}, ErrInvalid
	}
	if a.Type != access.AccountAdmin || !AllowedAdjustment(a, brand, "write") {
		return Adjustment{}, ErrDenied
	}
	if err := paymentBrand(ctx, tx, brand); err != nil {
		return Adjustment{}, err
	}
	gate, err := lockPaymentPolicy(ctx, tx, brand)
	if err != nil {
		return Adjustment{}, err
	}
	if !gate.Enabled {
		return Adjustment{}, ErrAdjustmentState
	}
	var payment string
	err = tx.QueryRow(ctx, `SELECT payment_id::text FROM commission_payment_targets WHERE brand_id=$1 AND id=$2`, brand, target).Scan(&payment)
	if errors.Is(err, pgx.ErrNoRows) {
		return Adjustment{}, ErrNotFound
	}
	if err != nil {
		return Adjustment{}, err
	}
	p, err := lockPayment(ctx, tx, brand, payment)
	if err != nil {
		return Adjustment{}, err
	}
	if p.State != "paid" {
		return Adjustment{}, ErrAdjustmentState
	}
	current, err := paymentEvidence(ctx, tx, p)
	if err != nil {
		return Adjustment{}, err
	}
	if !current {
		return Adjustment{}, ErrPaymentEvidence
	}
	var state, member string
	err = tx.QueryRow(ctx, `SELECT state,member_id::text FROM commission_payment_targets WHERE brand_id=$1 AND id=$2 FOR SHARE NOWAIT`, brand, target).Scan(&state, &member)
	if err != nil {
		return Adjustment{}, paymentDBError(err)
	}
	if state != "paid" {
		return Adjustment{}, ErrAdjustmentState
	}
	var version int64
	var before points.Amount
	err = tx.QueryRow(ctx, `SELECT version,points FROM commission_adjustment_heads WHERE brand_id=$1 AND target_id=$2 FOR UPDATE NOWAIT`, brand, target).Scan(&version, &before)
	if errors.Is(err, pgx.ErrNoRows) {
		return Adjustment{}, ErrAdjustmentState
	}
	if err != nil {
		return Adjustment{}, paymentDBError(err)
	}
	if version != in.Version || version >= maxCycleVersion {
		return Adjustment{}, ErrAdjustmentVersion
	}
	if before == in.Points {
		return Adjustment{}, ErrAdjustmentState
	}
	// Both operands are nonnegative int64. Their difference cannot reach the
	// unrepresentable positive magnitude of MinInt64.
	deltaAmount := in.Points - before
	var delta points.Balance
	delta[3][0] = deltaAmount
	var account string
	if err = tx.QueryRow(ctx, `SELECT id::text FROM point_accounts WHERE brand_id=$1 AND brand_member_id=$2 FOR UPDATE NOWAIT`, brand, member).Scan(&account); err != nil {
		return Adjustment{}, paymentDBError(err)
	}
	store := points.Store{DB: s.DB}
	policy, err := store.LockedPolicy(ctx, tx, brand)
	if err != nil {
		return Adjustment{}, err
	}
	if err = policy.CheckAdjustment(delta); err != nil {
		return Adjustment{}, err
	}
	id := ids.New()
	log, err := audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: meta.ActorType, ActorID: meta.ActorID, Action: "commission.adjustment.create", ResourceType: "commission_adjustment", ResourceID: id, Reason: in.Reason, RequestID: meta.RequestID, IP: meta.IP, Before: map[string]any{"version": version, "points": before}, After: map[string]any{"version": version + 1, "points": in.Points, "delta_points": deltaAmount, "target_id": target, "payment_id": payment}})
	if err != nil {
		return Adjustment{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO commission_adjustments(id,brand_id,target_id,payment_id,version,points_before,points_after,delta_points,audit_log_id,created_by,reason,point_policy_version) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, id, brand, target, payment, version+1, before, in.Points, deltaAmount, log, a.ID, in.Reason, policy.Version)
	if err != nil {
		return Adjustment{}, paymentDBError(err)
	}
	magnitude := deltaAmount
	if magnitude < 0 {
		magnitude = -magnitude
	}
	entry, err := store.Post(ctx, tx, points.Change{BrandID: brand, MemberID: member, EntryType: "commission_adjustment", ReferenceType: "commission_adjustment", ReferenceID: id, OperationKey: "commission-adjustment:" + id, Reason: in.Reason, ActorType: "admin", ActorID: a.ID, RequestID: meta.RequestID, IP: meta.IP, Delta: delta, Allocation: []points.Allocation{{Source: "commission", State: "available", Points: magnitude}}})
	if errors.Is(err, points.ErrInsufficient) {
		return Adjustment{}, ErrAdjustmentInsufficient
	}
	if err != nil {
		return Adjustment{}, paymentDBError(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE commission_adjustments SET ledger_entry_id=$2 WHERE id=$1`, id, entry.ID); err != nil {
		return Adjustment{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE commission_adjustment_heads SET version=version+1,points=$3,last_adjustment_id=$4 WHERE brand_id=$1 AND target_id=$2`, brand, target, in.Points, id); err != nil {
		return Adjustment{}, err
	}
	// Read the immutable receipt, not the later target projection. Idempotent
	// replay must keep this original version even after another adjustment.
	var out Adjustment
	err = tx.QueryRow(ctx, `SELECT id::text,brand_id::text,target_id::text,payment_id::text,version,points_before,points_after,delta_points,ledger_entry_id::text,audit_log_id::text,created_by::text,reason,point_policy_version::text,created_at FROM commission_adjustments WHERE id=$1`, id).Scan(&out.ID, &out.BrandID, &out.TargetID, &out.PaymentID, &out.Version, &out.PointsBefore, &out.PointsAfter, &out.DeltaPoints, &out.LedgerEntryID, &out.AuditLogID, &out.CreatedBy, &out.Reason, &out.PointPolicyVersion, &out.CreatedAt)
	return out, err
}
