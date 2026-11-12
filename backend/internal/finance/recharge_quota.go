package finance

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

var ErrRechargeQuota = errors.New("insufficient brand recharge quota")

type RechargeQuota struct {
	BrandID   string `json:"brand_id"`
	Available string `json:"available"`
}
type RechargeQuotaEntry struct {
	ID              string    `json:"id"`
	BrandID         string    `json:"brand_id"`
	Kind            string    `json:"kind"`
	Points          string    `json:"points"`
	BeforeAvailable string    `json:"before_available"`
	AfterAvailable  string    `json:"after_available"`
	RechargeOrderID *string   `json:"recharge_order_id"`
	MemberID        *string   `json:"member_id"`
	MemberAccount   *string   `json:"member_account"`
	ActorName       string    `json:"actor_name"`
	CreatedAt       time.Time `json:"created_at"`
}

func (s Service) ReadRechargeQuota(ctx context.Context, brand string) (RechargeQuota, error) {
	var out RechargeQuota
	if !financeUUIDPattern.MatchString(brand) {
		return out, points.ErrInvalid
	}
	err := s.DB.QueryRow(ctx, `SELECT b.id::text,coalesce(q.available,0)::text FROM brands b LEFT JOIN brand_recharge_quotas q ON q.brand_id=b.id WHERE b.id=$1`, brand).Scan(&out.BrandID, &out.Available)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, points.ErrNotFound
	}
	return out, err
}
func (s Service) ListRechargeQuotaEntries(ctx context.Context, brand string, limit, offset int) ([]RechargeQuotaEntry, error) {
	if !financeUUIDPattern.MatchString(brand) || limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		return nil, points.ErrInvalid
	}
	rows, err := s.DB.Query(ctx, `SELECT e.id::text,e.brand_id::text,e.kind,e.points::text,e.before_available::text,e.after_available::text,e.recharge_order_id::text,m.id::text,COALESCE(u.username,u.phone,u.telegram_user_id),a.username,e.created_at
 FROM brand_recharge_quota_entries e
 JOIN admin_accounts a ON a.id=e.actor_id
 LEFT JOIN recharge_orders r ON e.kind='manual_recharge' AND r.brand_id=e.brand_id AND r.id=e.recharge_order_id
 LEFT JOIN point_ledger_entries l ON e.kind='manual_adjustment' AND l.brand_id=e.brand_id AND l.id=e.id
 LEFT JOIN brand_members m ON m.brand_id=e.brand_id AND m.id=CASE e.kind WHEN 'manual_recharge' THEN r.member_id WHEN 'manual_adjustment' THEN l.member_id END
 LEFT JOIN global_users u ON u.id=m.global_user_id
 WHERE e.brand_id=$1 ORDER BY e.created_at DESC,e.id DESC LIMIT $2 OFFSET $3`, brand, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RechargeQuotaEntry{}
	for rows.Next() {
		var e RechargeQuotaEntry
		if err = rows.Scan(&e.ID, &e.BrandID, &e.Kind, &e.Points, &e.BeforeAvailable, &e.AfterAvailable, &e.RechargeOrderID, &e.MemberID, &e.MemberAccount, &e.ActorName, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
func (s Service) GrantRechargeQuota(ctx context.Context, tx pgx.Tx, brand string, amount points.Amount, meta points.Metadata) (RechargeQuota, error) {
	if tx == nil || !financeUUIDPattern.MatchString(brand) || amount <= 0 {
		return RechargeQuota{}, points.ErrInvalid
	}
	var active bool
	err := tx.QueryRow(ctx, `SELECT status='active' FROM brands WHERE id=$1 FOR SHARE`, brand).Scan(&active)
	if errors.Is(err, pgx.ErrNoRows) {
		return RechargeQuota{}, points.ErrNotFound
	}
	if err != nil {
		return RechargeQuota{}, err
	}
	if !active {
		return RechargeQuota{}, points.ErrConflict
	}
	if _, err = tx.Exec(ctx, `INSERT INTO brand_recharge_quotas(brand_id) VALUES($1) ON CONFLICT DO NOTHING`, brand); err != nil {
		return RechargeQuota{}, err
	}
	var before points.Amount
	if err = tx.QueryRow(ctx, `SELECT available FROM brand_recharge_quotas WHERE brand_id=$1 FOR UPDATE`, brand).Scan(&before); err != nil {
		return RechargeQuota{}, err
	}
	if before > points.Amount(math.MaxInt64)-amount {
		return RechargeQuota{}, points.ErrOverflow
	}
	after := before + amount
	if _, err = tx.Exec(ctx, `UPDATE brand_recharge_quotas SET available=$2 WHERE brand_id=$1`, brand, after); err != nil {
		return RechargeQuota{}, err
	}
	if err = appendQuotaEntry(ctx, tx, ids.New(), brand, "grant", "", amount, before, after, meta); err != nil {
		return RechargeQuota{}, err
	}
	return RechargeQuota{BrandID: brand, Available: after.String()}, nil
}

// ReclaimRechargeQuota returns only unused quota to the platform. The same row
// serializes this debit with member recharges and other quota changes.
func (s Service) ReclaimRechargeQuota(ctx context.Context, tx pgx.Tx, brand string, amount points.Amount, meta points.Metadata) (RechargeQuota, error) {
	if tx == nil || !financeUUIDPattern.MatchString(brand) || amount <= 0 {
		return RechargeQuota{}, points.ErrInvalid
	}
	var exists string
	err := tx.QueryRow(ctx, `SELECT id::text FROM brands WHERE id=$1 FOR SHARE`, brand).Scan(&exists)
	if errors.Is(err, pgx.ErrNoRows) {
		return RechargeQuota{}, points.ErrNotFound
	}
	if err != nil {
		return RechargeQuota{}, err
	}
	var after points.Amount
	err = tx.QueryRow(ctx, `UPDATE brand_recharge_quotas SET available=available-$2 WHERE brand_id=$1 AND available >= $2 RETURNING available`, brand, amount).Scan(&after)
	if errors.Is(err, pgx.ErrNoRows) {
		return RechargeQuota{}, ErrRechargeQuota
	}
	if err != nil {
		return RechargeQuota{}, err
	}
	if err = appendQuotaEntry(ctx, tx, ids.New(), brand, "reclaim", "", amount, after+amount, after, meta); err != nil {
		return RechargeQuota{}, err
	}
	return RechargeQuota{BrandID: brand, Available: after.String()}, nil
}

func (s Service) consumeRechargeQuota(ctx context.Context, tx pgx.Tx, brand, order string, amount points.Amount, meta points.Metadata) error {
	var after points.Amount
	err := tx.QueryRow(ctx, `UPDATE brand_recharge_quotas SET available=available-$2 WHERE brand_id=$1 AND available >= $2 RETURNING available`, brand, amount).Scan(&after)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrRechargeQuota
	}
	if err != nil {
		return err
	}
	return appendQuotaEntry(ctx, tx, ids.New(), brand, "manual_recharge", order, amount, after+amount, after, meta)
}
func appendQuotaEntry(ctx context.Context, tx pgx.Tx, id, brand, kind, order string, amount, before, after points.Amount, meta points.Metadata) error {
	_, err := tx.Exec(ctx, `INSERT INTO brand_recharge_quota_entries(id,brand_id,kind,points,before_available,after_available,recharge_order_id,actor_id) VALUES($1,$2,$3,$4,$5,$6,nullif($7,'')::uuid,$8)`, id, brand, kind, amount, before, after, order, meta.ActorID)
	if err != nil {
		return err
	}
	_, err = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: meta.ActorType, ActorID: meta.ActorID, Action: "finance.recharge_quota." + kind, ResourceType: "recharge_quota_entry", ResourceID: id, RequestID: meta.RequestID, IP: meta.IP, Before: map[string]any{"available": before.String()}, After: map[string]any{"available": after.String(), "points": amount.String(), "recharge_order_id": order}})
	return err
}

// PostManualAdjustment uses the same quota lock order as manual recharge:
// brand quota first, then member wallet. Its quota receipt uses the points
// entry ID, so replays cannot consume quota twice.
func (s Service) PostManualAdjustment(ctx context.Context, tx pgx.Tx, change points.Change) (points.Entry, error) {
	if tx == nil || change.EntryType != "adjust" || change.ReferenceType != "manual" {
		return points.Entry{}, points.ErrInvalid
	}
	amount, err := change.Delta.StateTotal(0)
	if err != nil {
		return points.Entry{}, err
	}
	if amount <= 0 {
		return s.Points.Post(ctx, tx, change)
	}
	var available points.Amount
	err = tx.QueryRow(ctx, `SELECT available FROM brand_recharge_quotas WHERE brand_id=$1 FOR UPDATE`, change.BrandID).Scan(&available)
	if errors.Is(err, pgx.ErrNoRows) {
		return points.Entry{}, ErrRechargeQuota
	}
	if err != nil {
		return points.Entry{}, err
	}
	entry, err := s.Points.Post(ctx, tx, change)
	if err != nil {
		return points.Entry{}, err
	}
	var recorded bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM brand_recharge_quota_entries WHERE brand_id=$1 AND id=$2)`, change.BrandID, entry.ID).Scan(&recorded); err != nil {
		return points.Entry{}, err
	}
	if recorded {
		return entry, nil
	}
	if available < amount {
		return points.Entry{}, ErrRechargeQuota
	}
	after := available - amount
	if _, err = tx.Exec(ctx, `UPDATE brand_recharge_quotas SET available=$2 WHERE brand_id=$1`, change.BrandID, after); err != nil {
		return points.Entry{}, err
	}
	err = appendQuotaEntry(ctx, tx, entry.ID, change.BrandID, "manual_adjustment", "", amount, available, after, points.Metadata{ActorType: change.ActorType, ActorID: change.ActorID, RequestID: change.RequestID, IP: change.IP})
	return entry, err
}
