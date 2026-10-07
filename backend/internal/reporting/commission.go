package reporting

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// CommissionQuery selects immutable business ledger postings by posting time,
// not a bet cohort or the latest calculated earning. Filters name saved
// beneficiaries and cycles, never today's editable agent policy.
type CommissionQuery struct {
	From     time.Time `json:"from"`
	To       time.Time `json:"to"`
	GroupBy  string    `json:"group_by"`
	Limit    int       `json:"limit"`
	Offset   int       `json:"offset"`
	AgentID  *string   `json:"agent_id"`
	MemberID *string   `json:"member_id"`
	CycleID  *string   `json:"cycle_id"`
}

func (q CommissionQuery) Validate() error {
	if q.From.IsZero() || q.To.IsZero() || !q.To.After(q.From) || q.To.Sub(q.From) > 93*24*time.Hour || q.From.Year() < 1 || q.To.Year() > 9999 || q.Limit < 1 || q.Limit > 100 || q.Offset < 0 || q.Offset > 1000000 {
		return ErrInvalid
	}
	if q.GroupBy != "day" && q.GroupBy != "agent" && q.GroupBy != "cycle" {
		return ErrInvalid
	}
	for _, id := range []*string{q.AgentID, q.MemberID, q.CycleID} {
		if id != nil && !uuid.MatchString(*id) {
			return ErrInvalid
		}
	}
	return nil
}

// Totals are unbounded exact decimal strings: a window may contain debits for
// credits outside that window, so NetPoints may legitimately be negative.
type CommissionTotals struct {
	EntryCount             string `json:"entry_count"`
	PaidEntryCount         string `json:"paid_entry_count"`
	PaidPoints             string `json:"paid_points"`
	AdjustmentEntryCount   string `json:"adjustment_entry_count"`
	AdjustmentCreditPoints string `json:"adjustment_credit_points"`
	AdjustmentDebitPoints  string `json:"adjustment_debit_points"`
	NetPoints              string `json:"net_points"`
}
type CommissionReport struct {
	BrandID     string                    `json:"brand_id"`
	SnapshotAt  time.Time                 `json:"snapshot_at"`
	Timezone    string                    `json:"timezone"`
	Query       CommissionQuery           `json:"query"`
	Summary     CommissionTotals          `json:"summary"`
	Items       []Group[CommissionTotals] `json:"items"`
	TotalGroups string                    `json:"total_groups"`
}

var commissionAggregates = []aggregate{
	{"entry_count", "count(*)"},
	{"paid_entry_count", "count(*) FILTER(WHERE kind='paid')"},
	{"paid_points", "coalesce(sum(delta_points) FILTER(WHERE kind='paid'),0)"},
	{"adjustment_entry_count", "count(*) FILTER(WHERE kind='adjustment')"},
	{"adjustment_credit_points", "coalesce(sum(greatest(delta_points,0)) FILTER(WHERE kind='adjustment'),0)"},
	{"adjustment_debit_points", "coalesce(sum(greatest(-delta_points,0)) FILTER(WHERE kind='adjustment'),0)"},
	{"net_points", "coalesce(sum(delta_points),0)"},
}

func (s Service) Commission(ctx context.Context, brand string, q CommissionQuery) (CommissionReport, error) {
	if s.DB == nil {
		return CommissionReport{BrandID: brand, Query: q, Items: []Group[CommissionTotals]{}}, ErrInvalid
	}
	return s.commission(ctx, s.DB, brand, q, q.Limit, q.Offset, false)
}
func (s Service) CommissionRead(ctx context.Context, tx pgx.Tx, brand string, q CommissionQuery) (CommissionReport, error) {
	if tx == nil {
		return CommissionReport{BrandID: brand, Query: q, Items: []Group[CommissionTotals]{}}, ErrInvalid
	}
	return s.commission(ctx, tx, brand, q, q.Limit, q.Offset, false)
}
func (s Service) CommissionExport(ctx context.Context, tx pgx.Tx, brand string, q CommissionQuery) (CommissionReport, error) {
	if tx == nil || q.Offset != 0 {
		return CommissionReport{BrandID: brand, Query: q, Items: []Group[CommissionTotals]{}}, ErrInvalid
	}
	return s.commission(ctx, tx, brand, q, ExportGroupLimit+1, 0, true)
}

func (s Service) commission(ctx context.Context, runner rowQuerier, brand string, q CommissionQuery, limit, offset int, exporting bool) (CommissionReport, error) {
	out := CommissionReport{BrandID: brand, Query: q, Items: []Group[CommissionTotals]{}}
	if runner == nil || !uuid.MatchString(brand) || q.Validate() != nil {
		return out, ErrInvalid
	}
	key, label := `to_char(f.posted_at AT TIME ZONE b.timezone,'YYYY-MM-DD')`, `to_char(f.posted_at AT TIME ZONE b.timezone,'YYYY-MM-DD')`
	if q.GroupBy == "agent" {
		key, label = `f.agent_id::text`, `f.agent_id::text`
	} else if q.GroupBy == "cycle" {
		key, label = `f.cycle_id::text`, `f.cycle_id::text`
	}
	agg := aggregateJSON(commissionAggregates)
	// Each immutable ledger is selected once via its unique business binding.
	// Do not join adjustment history onto original credits: that would multiply
	// a paid target when it has several adjustments. Do not compare current
	// evidence/state: later draw corrections must retain past posting facts.
	sql := `WITH scope AS (SELECT id,timezone FROM brands WHERE id=$1), validity AS (
 SELECT ($4::uuid IS NULL OR EXISTS(SELECT 1 FROM agent_nodes WHERE brand_id=$1 AND id=$4)) AND
 ($5::uuid IS NULL OR EXISTS(SELECT 1 FROM brand_members WHERE brand_id=$1 AND id=$5)) AND
 ($6::uuid IS NULL OR EXISTS(SELECT 1 FROM commission_cycles WHERE brand_id=$1 AND id=$6)) AS valid),
 facts AS (
 SELECT l.id,l.created_at AS posted_at,t.agent_id,t.member_id,p.cycle_id,'paid'::text AS kind,
 (l.delta_snapshot->'commission'->>'available')::numeric AS delta_points
 FROM point_ledger_entries l JOIN commission_payment_targets t ON t.brand_id=l.brand_id AND t.ledger_entry_id=l.id
 JOIN commission_payments p ON p.brand_id=t.brand_id AND p.id=t.payment_id
 WHERE l.brand_id=$1 AND l.created_at >= $2 AND l.created_at < $3 AND t.state='paid'
 AND l.member_id=t.member_id AND l.entry_type='commission' AND l.reference_type='commission_payment_target' AND l.reference_id=t.id
 UNION ALL
 SELECT l.id,l.created_at,t.agent_id,t.member_id,p.cycle_id,'adjustment'::text,
 (l.delta_snapshot->'commission'->>'available')::numeric
 FROM point_ledger_entries l JOIN commission_adjustments a ON a.brand_id=l.brand_id AND a.ledger_entry_id=l.id
 JOIN commission_payment_targets t ON t.brand_id=a.brand_id AND t.id=a.target_id
 JOIN commission_payments p ON p.brand_id=a.brand_id AND p.id=a.payment_id
 WHERE l.brand_id=$1 AND l.created_at >= $2 AND l.created_at < $3
 AND l.member_id=t.member_id AND l.entry_type='commission_adjustment' AND l.reference_type='commission_adjustment' AND l.reference_id=a.id),
 base AS (SELECT f.*,` + key + ` AS key,` + label + ` AS label FROM facts f JOIN scope b ON true
 WHERE ($4::uuid IS NULL OR f.agent_id=$4) AND ($5::uuid IS NULL OR f.member_id=$5) AND ($6::uuid IS NULL OR f.cycle_id=$6)),
 grouped AS (SELECT key,label,` + agg + ` AS totals FROM base GROUP BY key,label),
 page AS (SELECT key,label,totals FROM grouped ORDER BY key COLLATE "C" LIMIT $7 OFFSET $8)
 SELECT statement_timestamp(),scope.timezone,validity.valid,(SELECT ` + agg + ` FROM base),
 coalesce((SELECT jsonb_agg(to_jsonb(page) ORDER BY key COLLATE "C") FROM page),'[]'::jsonb),
 (SELECT count(*)::text FROM grouped) FROM scope CROSS JOIN validity`
	var valid bool
	var summary, rows []byte
	err := runner.QueryRow(ctx, sql, brand, databaseBound(q.From), databaseBound(q.To), q.AgentID, q.MemberID, q.CycleID, limit, offset).Scan(&out.SnapshotAt, &out.Timezone, &valid, &summary, &rows, &out.TotalGroups)
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
