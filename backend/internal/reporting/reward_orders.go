package reporting

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// RewardOrderTotals reports the current projection for orders in the created
// time cohort. Pending points are the original order amount, not reserved funds.
type RewardOrderTotals struct {
	OrderCount     string `json:"order_count"`
	OriginalPoints string `json:"original_points"`
	GrantedCount   string `json:"granted_count"`
	GrantedPoints  string `json:"granted_points"`
	PendingCount   string `json:"pending_count"`
	PendingPoints  string `json:"pending_points"`
	RevokedCount   string `json:"revoked_count"`
	RevokedPoints  string `json:"revoked_points"`
}

type RewardOrderReport struct {
	BrandID     string                     `json:"brand_id"`
	SnapshotAt  time.Time                  `json:"snapshot_at"`
	Timezone    string                     `json:"timezone"`
	Query       RewardQuery                `json:"query"`
	Summary     RewardOrderTotals          `json:"summary"`
	Items       []Group[RewardOrderTotals] `json:"items"`
	TotalGroups string                     `json:"total_groups"`
}

var rewardOrderAggregates = []aggregate{
	{"order_count", "count(*)"},
	{"original_points", "coalesce(sum(points::numeric),0)"},
	{"granted_count", "count(*) FILTER(WHERE state='granted')"},
	{"granted_points", "coalesce(sum(points::numeric) FILTER(WHERE state='granted'),0)"},
	{"pending_count", "count(*) FILTER(WHERE state='revocation_pending')"},
	{"pending_points", "coalesce(sum(points::numeric) FILTER(WHERE state='revocation_pending'),0)"},
	{"revoked_count", "count(*) FILTER(WHERE state='revoked')"},
	{"revoked_points", "coalesce(sum(points::numeric) FILTER(WHERE state='revoked'),0)"},
}

func (s Service) RewardOrders(ctx context.Context, brand string, q RewardQuery) (RewardOrderReport, error) {
	if s.DB == nil {
		return RewardOrderReport{BrandID: brand, Query: q, Items: []Group[RewardOrderTotals]{}}, ErrInvalid
	}
	return s.rewardOrders(ctx, s.DB, brand, q, q.Limit, q.Offset, false)
}

func (s Service) RewardOrdersRead(ctx context.Context, tx pgx.Tx, brand string, q RewardQuery) (RewardOrderReport, error) {
	if tx == nil {
		return RewardOrderReport{BrandID: brand, Query: q, Items: []Group[RewardOrderTotals]{}}, ErrInvalid
	}
	return s.rewardOrders(ctx, tx, brand, q, q.Limit, q.Offset, false)
}

func (s Service) RewardOrdersExport(ctx context.Context, tx pgx.Tx, brand string, q RewardQuery) (RewardOrderReport, error) {
	if tx == nil || q.Offset != 0 {
		return RewardOrderReport{BrandID: brand, Query: q, Items: []Group[RewardOrderTotals]{}}, ErrInvalid
	}
	return s.rewardOrders(ctx, tx, brand, q, ExportGroupLimit+1, 0, true)
}

func (s Service) rewardOrders(ctx context.Context, runner rowQuerier, brand string, q RewardQuery, limit, offset int, exporting bool) (RewardOrderReport, error) {
	out := RewardOrderReport{BrandID: brand, Query: q, Items: []Group[RewardOrderTotals]{}}
	if runner == nil || !uuid.MatchString(brand) || q.Validate("reward_orders") != nil {
		return out, ErrInvalid
	}
	group := `to_char(o.created_at AT TIME ZONE b.timezone,'YYYY-MM-DD')`
	if q.GroupBy == "member" {
		group = `o.member_id::text`
	} else if q.GroupBy == "state" {
		group = `o.state`
	}
	agg := aggregateJSON(rewardOrderAggregates)
	sql := `WITH scope AS (SELECT id,timezone FROM brands WHERE id=$1), validity AS (
 SELECT ($4::uuid IS NULL OR EXISTS(SELECT 1 FROM brand_members WHERE brand_id=$1 AND id=$4)) AND
        ($5::uuid IS NULL OR EXISTS(SELECT 1 FROM reward_orders WHERE brand_id=$1 AND id=$5)) AS valid),
 base AS (SELECT o.id,o.member_id,o.points,o.state,` + group + ` AS key,` + group + ` AS label
 FROM reward_orders o JOIN scope b ON b.id=o.brand_id
 WHERE o.created_at >= $2 AND o.created_at < $3 AND ($4::uuid IS NULL OR o.member_id=$4) AND ($5::uuid IS NULL OR o.id=$5)),
 grouped AS (SELECT key,label,` + agg + ` AS totals FROM base GROUP BY key,label),
 page AS (SELECT key,label,totals FROM grouped ORDER BY key COLLATE "C" LIMIT $6 OFFSET $7)
 SELECT statement_timestamp(),scope.timezone,validity.valid,(SELECT ` + agg + ` FROM base),
 coalesce((SELECT jsonb_agg(to_jsonb(page) ORDER BY key COLLATE "C") FROM page),'[]'::jsonb),
 (SELECT count(*)::text FROM grouped) FROM scope CROSS JOIN validity`
	var valid bool
	var summary, rows []byte
	err := runner.QueryRow(ctx, sql, brand, databaseBound(q.From), databaseBound(q.To), q.MemberID, q.OrderID, limit, offset).Scan(&out.SnapshotAt, &out.Timezone, &valid, &summary, &rows, &out.TotalGroups)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !valid {
		return out, ErrNotFound
	}
	if err != nil {
		return out, err
	}
	if exporting && groupCountExceeds(out.TotalGroups, ExportGroupLimit) {
		return out, ErrExportTooLarge
	}
	if err = json.Unmarshal(summary, &out.Summary); err == nil {
		err = json.Unmarshal(rows, &out.Items)
	}
	out.SnapshotAt = out.SnapshotAt.UTC()
	return out, err
}
