package rewards

import (
	"context"
	"errors"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct{ DB *pgxpool.Pool }

func dbError(err error) error {
	var pe *pgconn.PgError
	if errors.As(err, &pe) && (pe.Code == "55P03" || pe.Code == "40P01" || pe.Code == "40001") {
		return ErrBusy
	}
	return err
}

func admit(ctx context.Context, tx pgx.Tx, brand string, actor access.Account, action string, meta points.Metadata) error {
	if tx == nil || !validID(brand) || !validID(actor.ID) || meta.ActorType != "admin" || meta.ActorID != actor.ID || meta.RequestID == "" || len(meta.RequestID) > 80 {
		return ErrInvalid
	}
	if !Allowed(actor, brand, action) {
		return ErrDenied
	}
	var active, super bool
	if err := tx.QueryRow(ctx, `SELECT status='active',is_super_admin FROM admin_accounts WHERE id=$1 FOR SHARE NOWAIT`, actor.ID).Scan(&active, &super); errors.Is(err, pgx.ErrNoRows) {
		return ErrDenied
	} else if err != nil {
		return dbError(err)
	}
	if !active || super {
		return ErrDenied
	}
	var state string
	err := tx.QueryRow(ctx, `SELECT status FROM brands WHERE id=$1 FOR SHARE NOWAIT`, brand).Scan(&state)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return dbError(err)
	}
	if state == "disabled" {
		return ErrState
	}
	return nil
}

func lockWallet(ctx context.Context, tx pgx.Tx, brand, member string, store points.Store) (points.Wallet, error) {
	var id string
	err := tx.QueryRow(ctx, `SELECT id::text FROM point_accounts WHERE brand_id=$1 AND brand_member_id=$2 FOR UPDATE NOWAIT`, brand, member).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return points.Wallet{}, ErrNotFound
	}
	if err != nil {
		return points.Wallet{}, dbError(err)
	}
	w, err := store.LockedSnapshot(ctx, tx, brand, member)
	if err != nil {
		return w, err
	}
	if w.Version == 0 {
		if w.BySource != (points.Balance{}) {
			return w, points.ErrCorrupt
		}
	} else {
		var latestID string
		if err := tx.QueryRow(ctx, `SELECT id::text FROM point_ledger_entries WHERE brand_id=$1 AND account_id=$2 AND version=$3`, brand, w.AccountID, w.Version).Scan(&latestID); err != nil {
			return w, points.ErrCorrupt
		}
		latest, err := store.Entry(ctx, tx, brand, member, latestID)
		if err != nil || latest.After != w.BySource {
			return w, points.ErrCorrupt
		}
	}
	return w, nil
}

func actionAudit(ctx context.Context, tx pgx.Tx, brand, orderID, actionID, operation, state string, version int64, before *Order, inReason string, actor access.Account, meta points.Metadata, extra map[string]any) (string, error) {
	after := map[string]any{"action_id": actionID, "version": version, "state": state}
	for key, value := range extra {
		after[key] = value
	}
	var previous any
	if before != nil {
		previous = map[string]any{"version": before.Version, "state": before.State}
	}
	return audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: actor.ID, Action: "reward.order." + operation,
		ResourceType: "reward_order", ResourceID: orderID, Reason: inReason, RequestID: meta.RequestID, IP: meta.IP, Before: previous, After: after})
}

func insertAction(ctx context.Context, tx pgx.Tx, brand, orderID, actionID, operation, state string, version int64, before *string, actorID, reason, auditID string) error {
	_, err := tx.Exec(ctx, `INSERT INTO reward_order_actions(id,brand_id,order_id,version,operation,state_before,state_after,actor_id,reason,audit_log_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		actionID, brand, orderID, version, operation, before, state, actorID, reason, auditID)
	return dbError(err)
}

// GrantTx is the single operator's explicit grant approval. Every business row,
// the points ledger, wallet and both audits must commit together.
func (s Service) GrantTx(ctx context.Context, tx pgx.Tx, brand string, actor access.Account, in GrantInput, meta points.Metadata) (Order, error) {
	if in.Validate() != nil {
		return Order{}, ErrInvalid
	}
	if err := admit(ctx, tx, brand, actor, "grant", meta); err != nil {
		return Order{}, err
	}
	store := points.Store{DB: s.DB}
	if _, err := lockWallet(ctx, tx, brand, in.MemberID, store); err != nil {
		return Order{}, err
	}
	policy, err := store.LockedPolicy(ctx, tx, brand)
	if err != nil {
		return Order{}, dbError(err)
	}
	id, actionID := ids.New(), ids.New()
	log, err := actionAudit(ctx, tx, brand, id, actionID, "grant", "granted", 1, nil, in.Reason, actor, meta,
		map[string]any{"member_id": in.MemberID, "points": in.Points})
	if err != nil {
		return Order{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO reward_orders(id,brand_id,member_id,points,creation_audit_log_id,last_audit_log_id,created_by,reason,point_policy_version,state) VALUES($1,$2,$3,$4,$5,$5,$6,$7,$8,'granted')`,
		id, brand, in.MemberID, in.Points, log, actor.ID, in.Reason, policy.Version); err != nil {
		return Order{}, dbError(err)
	}
	if err = insertAction(ctx, tx, brand, id, actionID, "grant", "granted", 1, nil, actor.ID, in.Reason, log); err != nil {
		return Order{}, err
	}
	var delta points.Balance
	delta[2][0] = in.Points
	entry, err := store.Post(ctx, tx, points.Change{BrandID: brand, MemberID: in.MemberID, EntryType: "reward_grant", ReferenceType: "reward_order", ReferenceID: id,
		OperationKey: "reward-grant:" + id, Reason: in.Reason, ActorType: "admin", ActorID: actor.ID, RequestID: meta.RequestID, IP: meta.IP,
		Delta: delta, Allocation: []points.Allocation{{Source: "gift", State: "available", Points: in.Points}}})
	if err != nil {
		return Order{}, dbError(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE reward_order_actions SET ledger_entry_id=$2 WHERE id=$1`, actionID, entry.ID); err != nil {
		return Order{}, dbError(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE reward_orders SET grant_ledger_entry_id=$2 WHERE id=$1`, id, entry.ID); err != nil {
		return Order{}, dbError(err)
	}
	return s.OrderTx(ctx, tx, brand, id)
}

func (s Service) RevokeTx(ctx context.Context, tx pgx.Tx, brand, id string, actor access.Account, in ActionInput, meta points.Metadata) (Order, error) {
	return s.revoke(ctx, tx, brand, id, actor, in, meta, "revoke")
}
func (s Service) RetryRevocationTx(ctx context.Context, tx pgx.Tx, brand, id string, actor access.Account, in ActionInput, meta points.Metadata) (Order, error) {
	return s.revoke(ctx, tx, brand, id, actor, in, meta, "retry")
}

func (s Service) revoke(ctx context.Context, tx pgx.Tx, brand, id string, actor access.Account, in ActionInput, meta points.Metadata, operation string) (Order, error) {
	if !validID(id) || in.Validate() != nil {
		return Order{}, ErrInvalid
	}
	if err := admit(ctx, tx, brand, actor, operation, meta); err != nil {
		return Order{}, err
	}
	o, err := scanOrder(tx.QueryRow(ctx, orderSelect+` WHERE brand_id=$1 AND id=$2 FOR UPDATE NOWAIT`, brand, id))
	if err != nil {
		return Order{}, dbError(err)
	}
	if o.Version != in.Version || o.Version >= MaxVersion {
		return Order{}, ErrVersion
	}
	if operation == "revoke" && o.State != "granted" || operation == "retry" && o.State != "revocation_pending" {
		return Order{}, ErrState
	}
	store := points.Store{DB: s.DB}
	wallet, err := lockWallet(ctx, tx, brand, o.MemberID, store)
	if err != nil {
		return Order{}, err
	}
	state := "revoked"
	var code *string
	if wallet.GiftPoints < o.Points {
		state = "revocation_pending"
		v := "REWARD_AVAILABLE_INSUFFICIENT"
		code = &v
	}
	actionID := ids.New()
	log, err := actionAudit(ctx, tx, brand, id, actionID, operation, state, o.Version+1, &o, in.Reason, actor, meta, nil)
	if err != nil {
		return Order{}, err
	}
	if err = insertAction(ctx, tx, brand, id, actionID, operation, state, o.Version+1, &o.State, actor.ID, in.Reason, log); err != nil {
		return Order{}, err
	}
	var ledger *string
	if state == "revoked" {
		var delta points.Balance
		delta[2][0] = -o.Points
		entry, err := store.Post(ctx, tx, points.Change{BrandID: brand, MemberID: o.MemberID, EntryType: "reward_reversal", ReferenceType: "reward_order", ReferenceID: id,
			OperationKey: "reward-revoke:" + id, Reason: in.Reason, ActorType: "admin", ActorID: actor.ID, RequestID: meta.RequestID, IP: meta.IP,
			Delta: delta, Allocation: []points.Allocation{{Source: "gift", State: "available", Points: o.Points}}, ReversalOf: o.GrantLedgerEntryID})
		if err != nil {
			return Order{}, dbError(err)
		}
		ledger = &entry.ID
		if _, err = tx.Exec(ctx, `UPDATE reward_order_actions SET ledger_entry_id=$2 WHERE id=$1`, actionID, entry.ID); err != nil {
			return Order{}, dbError(err)
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE reward_orders SET state=$3,version=version+1,last_audit_log_id=$4,last_error_code=$5,revoke_ledger_entry_id=$6,revoked_at=CASE WHEN $3='revoked' THEN clock_timestamp() ELSE NULL END WHERE brand_id=$1 AND id=$2`,
		brand, id, state, log, code, ledger); err != nil {
		return Order{}, dbError(err)
	}
	return s.OrderTx(ctx, tx, brand, id)
}
