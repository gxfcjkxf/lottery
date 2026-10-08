package rewards

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

const orderSelect = `SELECT id::text,brand_id::text,member_id::text,points,state,version,grant_ledger_entry_id::text,revoke_ledger_entry_id::text,creation_audit_log_id::text,last_audit_log_id::text,last_error_code,created_by::text,reason,point_policy_version::text,created_at,updated_at,revoked_at FROM reward_orders`

func scanOrder(row pgx.Row) (Order, error) {
	var o Order
	err := row.Scan(&o.ID, &o.BrandID, &o.MemberID, &o.Points, &o.State, &o.Version, &o.GrantLedgerEntryID, &o.RevokeLedgerEntryID,
		&o.CreationAuditLogID, &o.LastAuditLogID, &o.LastErrorCode, &o.CreatedBy, &o.Reason, &o.PointPolicyVersion, &o.CreatedAt, &o.UpdatedAt, &o.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return o, ErrNotFound
	}
	return o, err
}

func (s Service) OrderTx(ctx context.Context, tx pgx.Tx, brand, id string) (Order, error) {
	if tx == nil || !validID(brand) || !validID(id) {
		return Order{}, ErrInvalid
	}
	return scanOrder(tx.QueryRow(ctx, orderSelect+` WHERE brand_id=$1 AND id=$2`, brand, id))
}

func validPage(brand string, limit, offset int) bool {
	return validID(brand) && limit >= 1 && limit <= 100 && offset >= 0 && offset <= 1000000
}

// Read methods use the caller's authenticated, repeatable-read transaction.
// They expose order/action history, never a mutable wallet balance or its raw
// complete ledger snapshots. HTTP receipt replay is a separate mutation layer.
func (s Service) OrdersTx(ctx context.Context, tx pgx.Tx, brand string, limit, offset int) (OrderPage, error) {
	p := OrderPage{BrandID: brand, Items: []Order{}, Limit: limit, Offset: offset}
	if tx == nil || !validPage(brand, limit, offset) {
		return p, ErrInvalid
	}
	if err := tx.QueryRow(ctx, `SELECT count(*)::text FROM reward_orders WHERE brand_id=$1`, brand).Scan(&p.TotalCount); err != nil {
		return p, err
	}
	rows, err := tx.Query(ctx, orderSelect+` WHERE brand_id=$1 ORDER BY created_at DESC,id DESC LIMIT $2 OFFSET $3`, brand, limit, offset)
	if err != nil {
		return p, err
	}
	defer rows.Close()
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return p, err
		}
		p.Items = append(p.Items, o)
	}
	return p, rows.Err()
}

func (s Service) ActionsTx(ctx context.Context, tx pgx.Tx, brand, id string, limit, offset int) (ActionPage, error) {
	p := ActionPage{BrandID: brand, OrderID: id, Items: []Action{}, Limit: limit, Offset: offset}
	if tx == nil || !validID(id) || !validPage(brand, limit, offset) {
		return p, ErrInvalid
	}
	if _, err := s.OrderTx(ctx, tx, brand, id); err != nil {
		return p, err
	}
	if err := tx.QueryRow(ctx, `SELECT count(*)::text FROM reward_order_actions WHERE brand_id=$1 AND order_id=$2`, brand, id).Scan(&p.TotalCount); err != nil {
		return p, err
	}
	rows, err := tx.Query(ctx, `SELECT id::text,brand_id::text,order_id::text,version,operation,state_before,state_after,actor_id::text,reason,audit_log_id::text,ledger_entry_id::text,created_at FROM reward_order_actions WHERE brand_id=$1 AND order_id=$2 ORDER BY version DESC LIMIT $3 OFFSET $4`, brand, id, limit, offset)
	if err != nil {
		return p, err
	}
	defer rows.Close()
	for rows.Next() {
		var a Action
		if err := rows.Scan(&a.ID, &a.BrandID, &a.OrderID, &a.Version, &a.Operation, &a.StateBefore, &a.StateAfter, &a.ActorID, &a.Reason, &a.AuditLogID, &a.LedgerEntryID, &a.CreatedAt); err != nil {
			return p, err
		}
		p.Items = append(p.Items, a)
	}
	return p, rows.Err()
}
