package commission

import (
	"context"
	"errors"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func paymentDBError(err error) error {
	var pe *pgconn.PgError
	if errors.As(err, &pe) && (pe.Code == "55P03" || pe.Code == "40P01" || pe.Code == "40001") {
		return ErrBusy
	}
	return err
}

func paymentActor(a access.Account, brand, action string, meta points.Metadata) error {
	if a.Type != access.AccountAdmin || !canonicalUUID(a.ID) || !canonicalUUID(brand) || meta.ActorType != "admin" || meta.ActorID != a.ID || meta.RequestID == "" || len(meta.RequestID) > 80 {
		return ErrInvalid
	}
	if !AllowedPayment(a, brand, action) {
		return ErrDenied
	}
	return nil
}

func paymentBrand(ctx context.Context, tx pgx.Tx, brand string) error {
	var state string
	err := tx.QueryRow(ctx, `SELECT status FROM brands WHERE id=$1 FOR SHARE NOWAIT`, brand).Scan(&state)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return paymentDBError(err)
	}
	if state == "disabled" {
		return ErrPaymentState
	}
	return nil
}

func lockPaymentPolicy(ctx context.Context, tx pgx.Tx, brand string) (PaymentPolicy, error) {
	var p PaymentPolicy
	var auditID *string
	err := tx.QueryRow(ctx, `SELECT brand_id::text,version,enabled,audit_log_id::text,updated_at FROM brand_commission_payment_policies WHERE brand_id=$1 FOR SHARE NOWAIT`, brand).Scan(&p.BrandID, &p.Version, &p.Enabled, &auditID, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	if auditID != nil {
		p.AuditLogID = *auditID
	}
	return p, paymentDBError(err)
}

func (s Service) UpdatePaymentPolicyTx(ctx context.Context, tx pgx.Tx, brand string, a access.Account, in PaymentPolicyInput, meta points.Metadata) (PaymentPolicy, error) {
	if tx == nil || in.Validate() != nil {
		return PaymentPolicy{}, ErrInvalid
	}
	if err := paymentActor(a, brand, "policy_write", meta); err != nil {
		return PaymentPolicy{}, err
	}
	if err := paymentBrand(ctx, tx, brand); err != nil {
		return PaymentPolicy{}, err
	}
	var version int64
	var enabled bool
	err := tx.QueryRow(ctx, `SELECT version,enabled FROM brand_commission_payment_policies WHERE brand_id=$1 FOR UPDATE NOWAIT`, brand).Scan(&version, &enabled)
	if err != nil {
		return PaymentPolicy{}, paymentDBError(err)
	}
	if version != in.Version || version >= maxCycleVersion {
		return PaymentPolicy{}, ErrPaymentVersion
	}
	log, err := audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: meta.ActorType, ActorID: meta.ActorID, Action: "commission.payment_policy.update", ResourceType: "commission_payment_policy", ResourceID: brand, Reason: in.Reason, RequestID: meta.RequestID, IP: meta.IP, Before: map[string]any{"version": version, "enabled": enabled}, After: map[string]any{"version": version + 1, "enabled": in.Enabled}})
	if err != nil {
		return PaymentPolicy{}, err
	}
	_, err = tx.Exec(ctx, `UPDATE brand_commission_payment_policies SET version=version+1,enabled=$2,audit_log_id=$3 WHERE brand_id=$1`, brand, in.Enabled, log)
	if err != nil {
		return PaymentPolicy{}, paymentDBError(err)
	}
	return s.PaymentPolicyTx(ctx, tx, brand)
}

type paymentRow struct {
	ID, Brand, Cycle, Run, State, Mode string
	Version, Epoch                     int64
	Approval                           *string
}

// Workers hold cycle -> payment -> target -> wallet. Administrative admission
// additionally guards brand/policy first. Every cross-worker lock uses NOWAIT
// to avoid waiting on the opposite settlement wallet/epoch order.
func lockPayment(ctx context.Context, tx pgx.Tx, brand, id string) (paymentRow, error) {
	var p paymentRow
	if tx == nil || !canonicalUUID(brand) || !canonicalUUID(id) {
		return p, ErrInvalid
	}
	var cycle string
	err := tx.QueryRow(ctx, `SELECT cycle_id::text FROM commission_payments WHERE brand_id=$1 AND id=$2`, brand, id).Scan(&cycle)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	if err != nil {
		return p, err
	}
	if _, err = cycleLock(ctx, tx, brand, cycle); err != nil {
		return p, err
	}
	err = tx.QueryRow(ctx, `SELECT id::text,brand_id::text,cycle_id::text,run_id::text,state,payout_mode,version,evidence_epoch,approval_audit_log_id::text FROM commission_payments WHERE brand_id=$1 AND id=$2 FOR UPDATE NOWAIT`, brand, id).Scan(&p.ID, &p.Brand, &p.Cycle, &p.Run, &p.State, &p.Mode, &p.Version, &p.Epoch, &p.Approval)
	return p, paymentDBError(err)
}

func paymentEvidence(ctx context.Context, tx pgx.Tx, p paymentRow) (bool, error) {
	var valid bool
	err := tx.QueryRow(ctx, `SELECT commission_payment_evidence_current($1,$2,$3,$4)`, p.Brand, p.Cycle, p.Run, p.Epoch).Scan(&valid)
	return valid, err
}

func transitionPayment(ctx context.Context, tx pgx.Tx, p paymentRow, state, operation, reason string, code *string, meta points.Metadata, targetIDs ...string) (string, error) {
	if p.Version >= maxCycleVersion {
		return "", ErrPaymentVersion
	}
	after := map[string]any{"version": p.Version + 1, "state": state, "run_id": p.Run, "error_code": code}
	if len(targetIDs) == 1 {
		after["target_id"] = targetIDs[0]
	}
	log, err := audit.Append(ctx, tx, audit.Record{BrandID: p.Brand, ActorType: meta.ActorType, ActorID: meta.ActorID, Action: "commission.payment." + operation, ResourceType: "commission_payment", ResourceID: p.ID, Reason: reason, RequestID: meta.RequestID, IP: meta.IP, Before: map[string]any{"version": p.Version, "state": p.State}, After: after})
	if err != nil {
		return "", err
	}
	if operation == "approve" {
		_, err = tx.Exec(ctx, `UPDATE commission_payments SET state=$3,version=version+1,last_audit_log_id=$4,last_error_code=$5,next_work_at=clock_timestamp(),approved_by=$6,approval_actor_type='admin',approval_audit_log_id=$4 WHERE brand_id=$1 AND id=$2`, p.Brand, p.ID, state, log, code, meta.ActorID)
	} else {
		_, err = tx.Exec(ctx, `UPDATE commission_payments SET state=$3,version=version+1,last_audit_log_id=$4,last_error_code=$5,next_work_at=clock_timestamp() WHERE brand_id=$1 AND id=$2`, p.Brand, p.ID, state, log, code)
	}
	return log, paymentDBError(err)
}

func (s Service) actPayment(ctx context.Context, tx pgx.Tx, brand, id string, a access.Account, in RetryCycleInput, meta points.Metadata, action string) (Payment, error) {
	if tx == nil || in.Validate() != nil {
		return Payment{}, ErrInvalid
	}
	if err := paymentActor(a, brand, action, meta); err != nil {
		return Payment{}, err
	}
	if err := paymentBrand(ctx, tx, brand); err != nil {
		return Payment{}, err
	}
	policy, err := lockPaymentPolicy(ctx, tx, brand)
	if err != nil {
		return Payment{}, err
	}
	if !policy.Enabled {
		return Payment{}, ErrPaymentState
	}
	p, err := lockPayment(ctx, tx, brand, id)
	if err != nil {
		return Payment{}, err
	}
	if p.Version != in.Version {
		return Payment{}, ErrPaymentVersion
	}
	manualReview := (p.Mode == "manual" || p.Mode == "mixed") && p.State == "awaiting_approval" && p.Approval == nil
	if action == "approve" && !manualReview || action == "retry" && (p.State != "failed" || p.Approval == nil) {
		return Payment{}, ErrPaymentState
	}
	valid, err := paymentEvidence(ctx, tx, p)
	if err != nil {
		return Payment{}, err
	}
	if !valid {
		return Payment{}, ErrPaymentEvidence
	}
	if _, err = transitionPayment(ctx, tx, p, "paying", action, in.Reason, nil, meta); err != nil {
		return Payment{}, err
	}
	return s.PaymentTx(ctx, tx, brand, id)
}

func (s Service) ApprovePaymentTx(ctx context.Context, tx pgx.Tx, brand, id string, a access.Account, in RetryCycleInput, meta points.Metadata) (Payment, error) {
	return s.actPayment(ctx, tx, brand, id, a, in, meta, "approve")
}
func (s Service) RetryPaymentTx(ctx context.Context, tx pgx.Tx, brand, id string, a access.Account, in RetryCycleInput, meta points.Metadata) (Payment, error) {
	return s.actPayment(ctx, tx, brand, id, a, in, meta, "retry")
}

func createPayment(ctx context.Context, tx pgx.Tx, c cycleRow, meta points.Metadata) error {
	if c.RunID == nil {
		return ErrPaymentEvidence
	}
	var mode, total string
	var count int64
	err := tx.QueryRow(ctx, `SELECT commission_payment_mode($1),coalesce(sum(points),0)::text,count(*) FROM commission_earnings WHERE run_id=$1`, *c.RunID).Scan(&mode, &total, &count)
	if err != nil {
		return err
	}
	state := "paying"
	var code *string
	var approval, actor *string
	if mode == "manual" || mode == "mixed" {
		state = "awaiting_approval"
	} else if mode != "automatic" && mode != "none" {
		return ErrPaymentEvidence
	}
	id := ids.New()
	log, err := audit.Append(ctx, tx, audit.Record{BrandID: c.Brand, ActorType: "system", Action: "commission.payment.create", ResourceType: "commission_payment", ResourceID: id, Reason: "register complete cycle for saved payout mode", RequestID: meta.RequestID, After: map[string]any{"version": 1, "state": state, "run_id": *c.RunID, "payout_mode": mode, "total_points": total, "target_count": count}})
	if err != nil {
		return err
	}
	if state == "paying" {
		approval = &log
		v := "system"
		actor = &v
	}
	_, err = tx.Exec(ctx, `INSERT INTO commission_payments(id,brand_id,cycle_id,run_id,evidence_epoch,payout_mode,state,total_points,target_count,approval_actor_type,approval_audit_log_id,creation_audit_log_id,last_audit_log_id,last_error_code) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$12,$13)`, id, c.Brand, c.ID, *c.RunID, c.Epoch, mode, state, total, count, actor, approval, log, code)
	return paymentDBError(err)
}

// A changed generation cannot authorize paying its full total a second time.
// Before any positive credit it may be superseded. After credit it must await
// explicit compensation, whose insufficient-balance semantics are not assumed.
func invalidatePayment(ctx context.Context, tx pgx.Tx, p paymentRow, meta points.Metadata) error {
	var credited bool
	// A zero original target may subsequently have an independent actual
	// adjustment. Preserve that financial history even when its current net
	// is zero. Reverse bindings also prevent a missing pointer hiding money.
	if err := tx.QueryRow(ctx, `SELECT
 EXISTS(SELECT 1 FROM commission_payment_targets t WHERE t.brand_id=$1 AND t.payment_id=$2 AND
 (t.ledger_entry_id IS NOT NULL OR EXISTS(SELECT 1 FROM point_ledger_entries l WHERE l.brand_id=$1 AND l.reference_type='commission_payment_target' AND l.reference_id=t.id)))
 OR EXISTS(SELECT 1 FROM commission_adjustments a WHERE a.brand_id=$1 AND a.payment_id=$2 AND
 (a.ledger_entry_id IS NOT NULL OR EXISTS(SELECT 1 FROM point_ledger_entries l WHERE l.brand_id=$1 AND l.reference_type='commission_adjustment' AND l.reference_id=a.id)))`, p.Brand, p.ID).Scan(&credited); err != nil {
		return err
	}
	state := "stale"
	var code *string
	if credited {
		state = "blocked"
		v := "COMMISSION_PAYMENT_CORRECTION_REQUIRED"
		code = &v
	}
	_, err := transitionPayment(ctx, tx, p, state, "invalidate", "settlement evidence changed; preserve original credits and stop new payments", code, meta)
	return err
}
