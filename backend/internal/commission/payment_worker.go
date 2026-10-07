package commission

import (
	"context"
	"errors"

	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

// ProcessPayments commits one bounded target per step. The default-off rollout
// gate is independent of immutable bet-time economic policy. Calculations and
// discovery never need this gate, so disabling payouts cannot block betting.
func (s Service) ProcessPayments(ctx context.Context, maxSteps int) (int, error) {
	if s.DB == nil || maxSteps < 1 || maxSteps > 100 {
		return 0, ErrInvalid
	}
	committed := 0
	for i := 0; i < maxSteps; i++ {
		tx, err := s.DB.Begin(ctx)
		if err != nil {
			return committed, err
		}
		var brand, id string
		err = tx.QueryRow(ctx, `SELECT c.brand_id::text,c.id::text FROM commission_cycles c
 LEFT JOIN commission_payments p ON p.cycle_id=c.id AND p.state<>'stale'
 JOIN brand_commission_payment_policies pol ON pol.brand_id=c.brand_id
 JOIN brands b ON b.id=c.brand_id
 WHERE (p.id IS NOT NULL AND p.state NOT IN('stale','blocked') AND p.next_work_at<=clock_timestamp() AND
  (p.evidence_epoch<>c.evidence_epoch OR p.run_id IS DISTINCT FROM c.current_run_id OR p.state='paying' AND pol.enabled AND b.status<>'disabled'))
 OR (p.id IS NULL AND c.state='ready' AND pol.enabled AND b.status<>'disabled' AND
  NOT EXISTS(SELECT 1 FROM commission_payments old WHERE old.cycle_id=c.id AND old.run_id=c.current_run_id))
 ORDER BY coalesce(p.next_work_at,c.updated_at),c.id FOR UPDATE OF c SKIP LOCKED LIMIT 1`).Scan(&brand, &id)
		if errors.Is(err, pgx.ErrNoRows) {
			_ = tx.Rollback(ctx)
			break
		}
		if err != nil {
			_ = tx.Rollback(ctx)
			return committed, paymentDBError(err)
		}
		c, err := cycleLock(ctx, tx, brand, id)
		var p paymentRow
		if err == nil {
			var pid string
			e := tx.QueryRow(ctx, `SELECT id::text FROM commission_payments WHERE cycle_id=$1 AND state<>'stale'`, id).Scan(&pid)
			if e == nil {
				p, err = lockPayment(ctx, tx, brand, pid)
			} else if !errors.Is(e, pgx.ErrNoRows) {
				err = e
			}
		}
		if err == nil {
			err = s.paymentStep(ctx, tx, c, p, points.Metadata{ActorType: "system", RequestID: ids.New()})
		}
		if err == nil {
			err = tx.Commit(ctx)
		} else {
			_ = tx.Rollback(ctx)
		}
		if err != nil {
			_ = tx.Rollback(ctx)
			err = paymentDBError(err)
			if errors.Is(err, ErrBusy) {
				if p.ID != "" {
					_, e := s.DB.Exec(ctx, `UPDATE commission_payments SET next_work_at=clock_timestamp()+interval '1 second' WHERE id=$1 AND version=$2`, p.ID, p.Version)
					if e != nil {
						return committed, e
					}
				}
				continue
			}
			// Admission failures have no persisted job to mark; report rather than
			// inventing a receipt or silently registering an incomplete payment.
			if p.ID == "" || p.State != "paying" || ctx.Err() != nil {
				return committed, err
			}
			code := "COMMISSION_PAYMENT_POST_FAILED"
			if errors.Is(err, points.ErrOverflow) {
				code = "COMMISSION_PAYMENT_AMOUNT_OVERFLOW"
			}
			if errors.Is(err, points.ErrCorrupt) || errors.Is(err, ErrPaymentEvidence) {
				code = "COMMISSION_PAYMENT_EVIDENCE_INVALID"
			}
			if e := s.failPayment(ctx, p, code); e != nil {
				return committed, e
			}
		}
		committed++
	}
	return committed, nil
}

func (s Service) paymentStep(ctx context.Context, tx pgx.Tx, c cycleRow, p paymentRow, meta points.Metadata) error {
	if p.ID != "" {
		valid, err := paymentEvidence(ctx, tx, p)
		if err != nil {
			return err
		}
		if !valid {
			return invalidatePayment(ctx, tx, p, meta)
		}
	}
	if err := paymentBrand(ctx, tx, c.Brand); err != nil {
		return err
	}
	pol, err := lockPaymentPolicy(ctx, tx, c.Brand)
	if err != nil {
		return err
	}
	if !pol.Enabled {
		return ErrBusy
	}
	if p.ID == "" {
		return createPayment(ctx, tx, c, meta)
	}
	if p.State != "paying" {
		return ErrPaymentState
	}
	return s.postPaymentTarget(ctx, tx, p, meta)
}

func (s Service) postPaymentTarget(ctx context.Context, tx pgx.Tx, p paymentRow, meta points.Metadata) error {
	var earning, agent, member string
	var amount points.Amount
	err := tx.QueryRow(ctx, `SELECT e.id::text,e.agent_id::text,e.member_id::text,e.points FROM commission_earnings e WHERE e.run_id=$1 AND
 NOT EXISTS(SELECT 1 FROM commission_payment_targets t WHERE t.payment_id=$2 AND t.earning_id=e.id AND t.state='paid') ORDER BY e.agent_id LIMIT 1`, p.Run, p.ID).Scan(&earning, &agent, &member, &amount)
	if errors.Is(err, pgx.ErrNoRows) {
		_, err = transitionPayment(ctx, tx, p, "paid", "complete", "all cycle beneficiaries accounted for", nil, meta)
		return err
	}
	if err != nil {
		return err
	}
	var target string
	err = tx.QueryRow(ctx, `SELECT id::text FROM commission_payment_targets WHERE payment_id=$1 AND earning_id=$2 FOR UPDATE NOWAIT`, p.ID, earning).Scan(&target)
	if errors.Is(err, pgx.ErrNoRows) {
		target = ids.New()
		_, err = tx.Exec(ctx, `INSERT INTO commission_payment_targets(id,brand_id,payment_id,earning_id,points,member_id,agent_id,payment_version) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, target, p.Brand, p.ID, earning, amount, member, agent, p.Version)
	}
	if err != nil {
		return paymentDBError(err)
	}
	var ledger *string
	if amount > 0 {
		// Do not wait while holding the cycle epoch: settlement may own this wallet
		// and need that epoch. Retrying contention is not a business failure.
		var account string
		err = tx.QueryRow(ctx, `SELECT id::text FROM point_accounts WHERE brand_id=$1 AND brand_member_id=$2 FOR UPDATE NOWAIT`, p.Brand, member).Scan(&account)
		if err != nil {
			return paymentDBError(err)
		}
		var delta points.Balance
		delta[3][0] = amount
		entry, e := (points.Store{DB: s.DB}).Post(ctx, tx, points.Change{BrandID: p.Brand, MemberID: member, EntryType: "commission", ReferenceType: "commission_payment_target", ReferenceID: target, OperationKey: "commission-payment:" + target, Reason: "approved cycle commission credited to commission available points", ActorType: "system", RequestID: meta.RequestID, Delta: delta, Allocation: []points.Allocation{{Source: "commission", State: "available", Points: amount}}})
		if e != nil {
			return paymentDBError(e)
		}
		ledger = &entry.ID
	}
	log, err := audit.Append(ctx, tx, audit.Record{BrandID: p.Brand, ActorType: "system", Action: "commission.payment.target", ResourceType: "commission_payment_target", ResourceID: target, Reason: "account for immutable beneficiary total; zero amounts do not create ledger entries", RequestID: meta.RequestID, Before: map[string]any{"state": "pending"}, After: map[string]any{"state": "paid", "points": amount, "ledger_entry_id": ledger, "earning_id": earning, "payment_id": p.ID}})
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE commission_payment_targets SET state='paid',ledger_entry_id=$2,audit_log_id=$3,paid_at=clock_timestamp() WHERE id=$1`, target, ledger, log)
	if err != nil {
		return err
	}
	_, err = transitionPayment(ctx, tx, p, "paying", "post", "one beneficiary target committed atomically with its ledger", nil, meta, target)
	return err
}

func (s Service) failPayment(ctx context.Context, observed paymentRow, code string) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	p, err := lockPayment(ctx, tx, observed.Brand, observed.ID)
	if err != nil {
		return err
	}
	// An ambiguous commit may already have advanced the target/version. Never
	// overwrite that later committed result with a stale failure observation.
	if p.Version != observed.Version || p.State != "paying" {
		return nil
	}
	valid, err := paymentEvidence(ctx, tx, p)
	if err != nil {
		return err
	}
	meta := points.Metadata{ActorType: "system", RequestID: ids.New()}
	if !valid {
		err = invalidatePayment(ctx, tx, p, meta)
	} else {
		_, err = transitionPayment(ctx, tx, p, "failed", "fail", "commission posting failed; authorized operator retry required", &code, meta)
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
