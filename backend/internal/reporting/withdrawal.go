package reporting

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// WithdrawalTotals describes the current state projection of requests created
// in the selected window. It is not a payout or immutable settlement report.
type WithdrawalTotals struct {
	OrderCount       string `json:"order_count"`
	RequestedPoints  string `json:"requested_points"`
	ReviewingCount   string `json:"reviewing_count"`
	ReviewingPoints  string `json:"reviewing_points"`
	ProcessingCount  string `json:"processing_count"`
	ProcessingPoints string `json:"processing_points"`
	PaidCount        string `json:"paid_count"`
	PaidPoints       string `json:"paid_points"`
	RejectedCount    string `json:"rejected_count"`
	RejectedPoints   string `json:"rejected_points"`
	FailedCount      string `json:"failed_count"`
	FailedPoints     string `json:"failed_points"`
	CancelledCount   string `json:"cancelled_count"`
	CancelledPoints  string `json:"cancelled_points"`
}

type WithdrawalReport struct {
	BrandID     string                    `json:"brand_id"`
	SnapshotAt  time.Time                 `json:"snapshot_at"`
	Timezone    string                    `json:"timezone"`
	Query       Query                     `json:"query"`
	Summary     WithdrawalTotals          `json:"summary"`
	Items       []Group[WithdrawalTotals] `json:"items"`
	TotalGroups string                    `json:"total_groups"`
}

var withdrawalAggregates = []aggregate{
	{"order_count", "count(*)"}, {"requested_points", "coalesce(sum(points::numeric),0)"},
	{"reviewing_count", "count(*) FILTER(WHERE state='reviewing')"}, {"reviewing_points", "coalesce(sum(points::numeric) FILTER(WHERE state='reviewing'),0)"},
	{"processing_count", "count(*) FILTER(WHERE state='processing')"}, {"processing_points", "coalesce(sum(points::numeric) FILTER(WHERE state='processing'),0)"},
	{"paid_count", "count(*) FILTER(WHERE state='paid')"}, {"paid_points", "coalesce(sum(points::numeric) FILTER(WHERE state='paid'),0)"},
	{"rejected_count", "count(*) FILTER(WHERE state='rejected')"}, {"rejected_points", "coalesce(sum(points::numeric) FILTER(WHERE state='rejected'),0)"},
	{"failed_count", "count(*) FILTER(WHERE state='failed')"}, {"failed_points", "coalesce(sum(points::numeric) FILTER(WHERE state='failed'),0)"},
	{"cancelled_count", "count(*) FILTER(WHERE state='cancelled')"}, {"cancelled_points", "coalesce(sum(points::numeric) FILTER(WHERE state='cancelled'),0)"},
}

func (s Service) Withdrawal(ctx context.Context, brand string, q Query) (WithdrawalReport, error) {
	if s.DB == nil {
		return WithdrawalReport{BrandID: brand, Query: q, Items: []Group[WithdrawalTotals]{}}, ErrInvalid
	}
	return s.withdrawal(ctx, s.DB, brand, q, q.Limit, q.Offset, false)
}

func (s Service) WithdrawalRead(ctx context.Context, tx pgx.Tx, brand string, q Query) (WithdrawalReport, error) {
	if tx == nil {
		return WithdrawalReport{BrandID: brand, Query: q, Items: []Group[WithdrawalTotals]{}}, ErrInvalid
	}
	return s.withdrawal(ctx, tx, brand, q, q.Limit, q.Offset, false)
}

func (s Service) WithdrawalExport(ctx context.Context, tx pgx.Tx, brand string, q Query) (WithdrawalReport, error) {
	if tx == nil || q.Offset != 0 {
		return WithdrawalReport{BrandID: brand, Query: q, Items: []Group[WithdrawalTotals]{}}, ErrInvalid
	}
	return s.withdrawal(ctx, tx, brand, q, ExportGroupLimit+1, 0, true)
}

func (s Service) withdrawal(ctx context.Context, runner rowQuerier, brand string, q Query, limit, offset int, exporting bool) (WithdrawalReport, error) {
	out := WithdrawalReport{BrandID: brand, Query: q, Items: []Group[WithdrawalTotals]{}}
	if runner == nil || !uuid.MatchString(brand) || q.Validate("withdrawal") != nil {
		return out, ErrInvalid
	}
	group, label := `to_char(o.created_at AT TIME ZONE b.timezone,'YYYY-MM-DD')`, `to_char(o.created_at AT TIME ZONE b.timezone,'YYYY-MM-DD')`
	if q.GroupBy == "member" {
		group, label = `o.member_id::text`, `o.member_id::text`
	} else if q.GroupBy == "state" {
		group, label = `o.state`, `o.state`
	}
	agg := aggregateJSON(withdrawalAggregates)
	sql := `WITH scope AS (SELECT id,timezone FROM brands WHERE id=$1), validity AS (
 SELECT ($4::uuid IS NULL OR EXISTS(SELECT 1 FROM brand_members WHERE brand_id=$1 AND id=$4)) AS valid),
 base AS (SELECT o.*,` + group + ` AS key,` + label + ` AS label FROM withdrawal_orders o JOIN scope b ON b.id=o.brand_id
 WHERE o.created_at >= $2 AND o.created_at < $3 AND ($4::uuid IS NULL OR o.member_id=$4)),
 grouped AS (SELECT key,label,` + agg + ` AS totals FROM base GROUP BY key,label),
 page AS (SELECT key,label,totals FROM grouped ORDER BY key COLLATE "C" LIMIT $5 OFFSET $6)
 SELECT statement_timestamp(),scope.timezone,validity.valid,(SELECT ` + agg + ` FROM base),
 COALESCE((SELECT jsonb_agg(to_jsonb(page) ORDER BY key COLLATE "C") FROM page),'[]'::jsonb),
 (SELECT count(*)::text FROM grouped) FROM scope CROSS JOIN validity`
	var valid bool
	var summary, rows []byte
	e := runner.QueryRow(ctx, sql, brand, databaseBound(q.From), databaseBound(q.To), q.MemberID, limit, offset).Scan(&out.SnapshotAt, &out.Timezone, &valid, &summary, &rows, &out.TotalGroups)
	if errors.Is(e, pgx.ErrNoRows) || e == nil && !valid {
		return out, ErrNotFound
	}
	if e != nil {
		return out, e
	}
	if exporting && groupCountExceeds(out.TotalGroups, ExportGroupLimit) {
		return out, ErrExportTooLarge
	}
	if e = json.Unmarshal(summary, &out.Summary); e == nil {
		e = json.Unmarshal(rows, &out.Items)
	}
	return out, e
}
