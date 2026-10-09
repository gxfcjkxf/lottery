package reporting

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrAttributionEvidence = errors.New("saved order attribution is incomplete or invalid")

// AttributionQuery selects an immutable placed-order cohort, not current
// membership, today's agent tree, commission earnings, or ledger posting time.
type AttributionQuery struct {
	From       time.Time `json:"from"`
	To         time.Time `json:"to"`
	GroupBy    string    `json:"group_by"`
	Limit      int       `json:"limit"`
	Offset     int       `json:"offset"`
	GameID     *string   `json:"game_id"`
	MemberID   *string   `json:"member_id"`
	AgentID    *string   `json:"agent_id"`
	AgentScope string    `json:"agent_scope"`
	JoinMethod *string   `json:"join_method"`
}

func (q AttributionQuery) Validate() error {
	if q.From.IsZero() || q.To.IsZero() || !q.To.After(q.From) || q.To.Sub(q.From) > 93*24*time.Hour || q.From.Year() < 1 || q.To.Year() > 9999 || q.Limit < 1 || q.Limit > 100 || q.Offset < 0 || q.Offset > 1000000 {
		return ErrInvalid
	}
	switch q.GroupBy {
	case "day", "game", "member", "agent", "join_method":
	default:
		return ErrInvalid
	}
	if q.AgentScope != "direct" && q.AgentScope != "downline" || q.AgentScope == "downline" && q.AgentID == nil {
		return ErrInvalid
	}
	for _, id := range []*string{q.GameID, q.MemberID, q.AgentID} {
		if id != nil && !uuid.MatchString(*id) {
			return ErrInvalid
		}
	}
	if q.JoinMethod != nil {
		switch *q.JoinMethod {
		case "domain", "operator", "agent_code", "referral_code", "legacy":
		default:
			return ErrInvalid
		}
	}
	return nil
}

// These are operational current-generation projections, not an authorization
// to accrue or pay commission. Final lost stake is not stake minus winnings.
// Counts and sums may exceed int64 and always remain exact decimal strings.
type AttributionTotals struct {
	BettingTotals
	FinalLostStakePoints   string `json:"final_lost_stake_points"`
	LegacyAttributionCount string `json:"legacy_attribution_count"`
}

type AttributionReport struct {
	BrandID     string                     `json:"brand_id"`
	SnapshotAt  time.Time                  `json:"snapshot_at"`
	Timezone    string                     `json:"timezone"`
	Query       AttributionQuery           `json:"query"`
	Summary     AttributionTotals          `json:"summary"`
	Items       []Group[AttributionTotals] `json:"items"`
	TotalGroups string                     `json:"total_groups"`
}

var attributionAggregates = append(append([]aggregate{}, betAggregates...),
	aggregate{"final_lost_stake_points", "coalesce(sum(total_points::numeric) FILTER(WHERE final AND status='lost'),0)"},
	aggregate{"legacy_attribution_count", "count(*) FILTER(WHERE legacy_attribution)"},
)

func (s Service) Attribution(ctx context.Context, brand string, q AttributionQuery) (AttributionReport, error) {
	if s.DB == nil {
		return AttributionReport{BrandID: brand, Query: q, Items: []Group[AttributionTotals]{}}, ErrInvalid
	}
	return s.attribution(ctx, s.DB, brand, q, q.Limit, q.Offset, false)
}

func (s Service) AttributionRead(ctx context.Context, tx pgx.Tx, brand string, q AttributionQuery) (AttributionReport, error) {
	if tx == nil {
		return AttributionReport{BrandID: brand, Query: q, Items: []Group[AttributionTotals]{}}, ErrInvalid
	}
	return s.attribution(ctx, tx, brand, q, q.Limit, q.Offset, false)
}

func (s Service) AttributionExport(ctx context.Context, tx pgx.Tx, brand string, q AttributionQuery) (AttributionReport, error) {
	if tx == nil || q.Offset != 0 {
		return AttributionReport{BrandID: brand, Query: q, Items: []Group[AttributionTotals]{}}, ErrInvalid
	}
	return s.attribution(ctx, tx, brand, q, ExportGroupLimit+1, 0, true)
}

func (s Service) attribution(ctx context.Context, runner rowQuerier, brand string, q AttributionQuery, limit, offset int, exporting bool) (AttributionReport, error) {
	out := AttributionReport{BrandID: brand, Query: q, Items: []Group[AttributionTotals]{}}
	if runner == nil || !uuid.MatchString(brand) || q.Validate() != nil {
		return out, ErrInvalid
	}
	q.From, q.To = q.From.UTC(), q.To.UTC()
	out.Query = q
	key, label := `to_char(f.placed_at AT TIME ZONE b.timezone,'YYYY-MM-DD')`, `to_char(f.placed_at AT TIME ZONE b.timezone,'YYYY-MM-DD')`
	switch q.GroupBy {
	case "game":
		key, label = `f.game_id::text`, `f.game_name`
	case "member":
		key, label = `f.brand_member_id::text`, `f.brand_member_id::text`
	case "agent":
		// Even a downline filter groups by the saved DIRECT agent. Expanding
		// every ancestor into its own row would duplicate summary and CSV sums.
		key, label = `coalesce(f.direct_agent_id,CASE WHEN f.legacy_attribution THEN 'legacy' ELSE 'none' END)`, `coalesce(f.direct_agent_id,CASE WHEN f.legacy_attribution THEN 'legacy' ELSE 'none' END)`
	case "join_method":
		key, label = `f.saved_join_method`, `f.saved_join_method`
	}
	agg := aggregateJSON(attributionAggregates)
	// Read only saved attribution. Identity existence validates scope and saved
	// path links, but mutable configs/status/path never reconstruct provenance.
	// Classify legacy explicitly, and fail the WHOLE cohort on malformed modern
	// provenance before applying attribution filters (no silent missing rows).
	sql := `WITH scope AS (SELECT id,timezone FROM brands WHERE id=$1), validity AS (
 SELECT ($4::uuid IS NULL OR EXISTS(SELECT 1 FROM games WHERE brand_id=$1 AND id=$4)) AND
 ($5::uuid IS NULL OR EXISTS(SELECT 1 FROM brand_members WHERE brand_id=$1 AND id=$5)) AND
 ($6::uuid IS NULL OR EXISTS(SELECT 1 FROM agent_nodes WHERE brand_id=$1 AND id=$6)) AS valid),
 cohort AS (
 SELECT o.*,g.name AS game_name,o.attribution_snapshot AS a,
 EXISTS(SELECT 1 FROM draw_corrections dc WHERE dc.brand_id=o.brand_id AND dc.period_id=o.period_id AND dc.state<>'completed') AS correction_open,
 coalesce(o.status IN ('won','lost') AND o.refund_entry_id IS NULL AND p.status='settled'
 AND j.state='completed' AND j.period_id=o.period_id AND j.game_id=o.game_id AND j.draw_result_id=p.draw_result_id
 AND c.id=o.settlement_calculation_id AND c.job_id=j.id AND c.period_id=o.period_id AND c.order_id=o.id
 AND c.won=(o.status='won') AND c.prize_points=o.prize_points AND t.state='paid'
 AND t.calculation_id=c.id AND t.payout_entry_id IS NOT DISTINCT FROM o.payout_entry_id
 AND debit.id IS NOT NULL AND debit.entry_type='bet' AND debit.reference_type='bet_order' AND debit.reference_id=o.id
 AND ((o.prize_points=0 AND o.payout_entry_id IS NULL) OR
 (o.prize_points>0 AND prize.id IS NOT NULL AND prize.entry_type='prize' AND prize.reference_type='settlement_calculation' AND prize.reference_id=c.id)),false) AS finalized
 FROM bet_orders o JOIN scope b ON b.id=o.brand_id JOIN games g ON g.brand_id=o.brand_id AND g.id=o.game_id
 JOIN periods p ON p.brand_id=o.brand_id AND p.id=o.period_id
 LEFT JOIN settlement_jobs j ON j.brand_id=o.brand_id AND j.id=p.current_settlement_job_id
 LEFT JOIN settlement_calculations c ON c.brand_id=o.brand_id AND c.id=o.settlement_calculation_id
 LEFT JOIN settlement_targets t ON t.brand_id=o.brand_id AND t.job_id=j.id AND t.order_id=o.id
 LEFT JOIN point_ledger_entries debit ON debit.brand_id=o.brand_id AND debit.id=o.debit_entry_id AND debit.member_id=o.brand_member_id AND debit.account_id=o.account_id
 LEFT JOIN point_ledger_entries prize ON prize.brand_id=o.brand_id AND prize.id=o.payout_entry_id AND prize.member_id=o.brand_member_id AND prize.account_id=o.account_id
 WHERE o.placed_at >= $2 AND o.placed_at < $3 AND ($4::uuid IS NULL OR o.game_id=$4) AND ($5::uuid IS NULL OR o.brand_member_id=$5)),
 shapes AS (SELECT cohort.*,a->'legacy'='true'::jsonb AS legacy_attribution,
 CASE WHEN jsonb_typeof(a->'agent_configs_at_bet')='array' THEN a->'agent_configs_at_bet' ELSE '[]'::jsonb END AS saved_nodes,
 a->'member_attribution' AS m FROM cohort),
 provenance AS (SELECT shapes.*,
 CASE WHEN legacy_attribution THEN NULL ELSE m->>'agent_id' END AS direct_agent_id,
 CASE WHEN legacy_attribution THEN 'legacy' ELSE m->>'join_method' END AS saved_join_method,
 coalesce(a->'schema_version'='1'::jsonb AND jsonb_typeof(a->'legacy')='boolean' AND
 (legacy_attribution OR (
 a->'legacy'='false'::jsonb AND jsonb_typeof(m)='object' AND m->'schema_version'='1'::jsonb AND m->'legacy'='false'::jsonb
 AND m->>'join_method' IN ('domain','operator','agent_code','referral_code')
 AND m ? 'agent_id' AND (m->'agent_id'='null'::jsonb OR (jsonb_typeof(m->'agent_id')='string' AND m->>'agent_id' ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'))
 AND jsonb_typeof(a->'agent_configs_at_bet')='array' AND jsonb_array_length(saved_nodes)<=32
 AND ((m->'agent_id'='null'::jsonb AND jsonb_array_length(saved_nodes)=0) OR
 (m->>'agent_id'=saved_nodes->-1->>'id' AND jsonb_array_length(saved_nodes)>0))
 AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements(saved_nodes) WITH ORDINALITY AS n(node,ord)
 WHERE jsonb_typeof(node)<>'object' OR node->>'id' IS NULL OR node->>'id' !~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
 OR node->'depth' IS DISTINCT FROM to_jsonb(ord) OR (ord=1 AND node->'parent_id' IS DISTINCT FROM 'null'::jsonb)
 OR (ord>1 AND node->>'parent_id' IS DISTINCT FROM saved_nodes->((ord-2)::int)->>'id')
 OR NOT EXISTS(SELECT 1 FROM agent_nodes an WHERE an.brand_id=$1 AND an.id::text=node->>'id'))
 AND (SELECT count(DISTINCT node->>'id') FROM jsonb_array_elements(saved_nodes) AS n(node))=jsonb_array_length(saved_nodes)
 )),false) AS provenance_valid FROM shapes),
 base AS (SELECT f.*,` + key + ` AS key,` + label + ` AS label,finalized AND NOT correction_open AS final
 FROM provenance f JOIN scope b ON true
 WHERE ($8::text IS NULL OR saved_join_method=$8) AND ($6::uuid IS NULL OR
 CASE WHEN $7='direct' THEN direct_agent_id=$6::uuid::text ELSE
 EXISTS(SELECT 1 FROM jsonb_array_elements(saved_nodes) n WHERE n->>'id'=$6::uuid::text) AND NOT legacy_attribution END)),
 grouped AS (SELECT key,label,` + agg + ` AS totals FROM base GROUP BY key,label),
 page AS (SELECT key,label,totals FROM grouped ORDER BY key COLLATE "C" LIMIT $9 OFFSET $10)
 SELECT statement_timestamp(),scope.timezone,validity.valid,
 coalesce((SELECT bool_and(provenance_valid) FROM provenance),true),(SELECT ` + agg + ` FROM base),
 coalesce((SELECT jsonb_agg(to_jsonb(page) ORDER BY key COLLATE "C") FROM page),'[]'::jsonb),
 (SELECT count(*)::text FROM grouped) FROM scope CROSS JOIN validity`
	var valid, evidence bool
	var summary, rows []byte
	err := runner.QueryRow(ctx, sql, brand, databaseBound(q.From), databaseBound(q.To), q.GameID, q.MemberID, q.AgentID, q.AgentScope, q.JoinMethod, limit, offset).Scan(&out.SnapshotAt, &out.Timezone, &valid, &evidence, &summary, &rows, &out.TotalGroups)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !valid {
		return out, ErrNotFound
	}
	if err != nil {
		return out, err
	}
	if !evidence {
		return out, ErrAttributionEvidence
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
