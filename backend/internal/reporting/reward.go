package reporting

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// RewardQuery selects either immutable reward wallet postings or the current
// state of orders created in the selected cohort. The two reports are
// intentionally separate because their time and state semantics differ.
type RewardQuery struct {
	From     time.Time `json:"from"`
	To       time.Time `json:"to"`
	GroupBy  string    `json:"group_by"`
	Limit    int       `json:"limit"`
	Offset   int       `json:"offset"`
	MemberID *string   `json:"member_id"`
	OrderID  *string   `json:"order_id"`
}

func (q RewardQuery) Validate(kind string) error {
	if q.From.IsZero() || q.To.IsZero() || !q.To.After(q.From) || q.To.Sub(q.From) > 93*24*time.Hour || q.From.Year() < 1 || q.To.Year() > 9999 || q.Limit < 1 || q.Limit > 100 || q.Offset < 0 || q.Offset > 1000000 {
		return ErrInvalid
	}
	if kind == "rewards" {
		if q.GroupBy != "day" && q.GroupBy != "member" && q.GroupBy != "order" {
			return ErrInvalid
		}
	} else if kind == "reward_orders" {
		if q.GroupBy != "day" && q.GroupBy != "member" && q.GroupBy != "state" {
			return ErrInvalid
		}
	} else {
		return ErrInvalid
	}
	for _, id := range []*string{q.MemberID, q.OrderID} {
		if id != nil && !uuid.MatchString(*id) {
			return ErrInvalid
		}
	}
	return nil
}

// RewardTotals are exact, unbounded decimal strings. NetPoints is signed
// because a window may include a reversal without its original grant.
type RewardTotals struct {
	EntryCount         string `json:"entry_count"`
	GrantEntryCount    string `json:"grant_entry_count"`
	GrantPoints        string `json:"grant_points"`
	ReversalEntryCount string `json:"reversal_entry_count"`
	ReversalPoints     string `json:"reversal_points"`
	NetPoints          string `json:"net_points"`
}

type RewardReport struct {
	BrandID     string                `json:"brand_id"`
	SnapshotAt  time.Time             `json:"snapshot_at"`
	Timezone    string                `json:"timezone"`
	Query       RewardQuery           `json:"query"`
	Summary     RewardTotals          `json:"summary"`
	Items       []Group[RewardTotals] `json:"items"`
	TotalGroups string                `json:"total_groups"`
}

var rewardAggregates = []aggregate{
	{"entry_count", "count(*)"},
	{"grant_entry_count", "count(*) FILTER(WHERE kind='grant')"},
	{"grant_points", "coalesce(sum(points) FILTER(WHERE kind='grant'),0)"},
	{"reversal_entry_count", "count(*) FILTER(WHERE kind='reversal')"},
	{"reversal_points", "coalesce(sum(points) FILTER(WHERE kind='reversal'),0)"},
	{"net_points", "coalesce(sum(signed_points),0)"},
}

func (s Service) Reward(ctx context.Context, brand string, q RewardQuery) (RewardReport, error) {
	if s.DB == nil {
		return RewardReport{BrandID: brand, Query: q, Items: []Group[RewardTotals]{}}, ErrInvalid
	}
	return s.reward(ctx, s.DB, brand, q, q.Limit, q.Offset, false)
}

func (s Service) RewardRead(ctx context.Context, tx pgx.Tx, brand string, q RewardQuery) (RewardReport, error) {
	if tx == nil {
		return RewardReport{BrandID: brand, Query: q, Items: []Group[RewardTotals]{}}, ErrInvalid
	}
	return s.reward(ctx, tx, brand, q, q.Limit, q.Offset, false)
}

func (s Service) RewardExport(ctx context.Context, tx pgx.Tx, brand string, q RewardQuery) (RewardReport, error) {
	if tx == nil || q.Offset != 0 {
		return RewardReport{BrandID: brand, Query: q, Items: []Group[RewardTotals]{}}, ErrInvalid
	}
	return s.reward(ctx, tx, brand, q, ExportGroupLimit+1, 0, true)
}

func (s Service) reward(ctx context.Context, runner rowQuerier, brand string, q RewardQuery, limit, offset int, exporting bool) (RewardReport, error) {
	out := RewardReport{BrandID: brand, Query: q, Items: []Group[RewardTotals]{}}
	if runner == nil || !uuid.MatchString(brand) || q.Validate("rewards") != nil {
		return out, ErrInvalid
	}
	group := `to_char(f.posted_at AT TIME ZONE b.timezone,'YYYY-MM-DD')`
	if q.GroupBy == "member" {
		group = `f.member_id::text`
	} else if q.GroupBy == "order" {
		group = `f.order_id::text`
	}
	agg := aggregateJSON(rewardAggregates)
	// The action is the immutable operator/audit witness for each posting. Bind
	// ledger_entry_id directly so a pending action or later state update cannot
	// multiply or erase an earlier grant.
	sql := `WITH scope AS (SELECT id,timezone FROM brands WHERE id=$1), validity AS (
 SELECT ($4::uuid IS NULL OR EXISTS(SELECT 1 FROM brand_members WHERE brand_id=$1 AND id=$4)) AND
        ($5::uuid IS NULL OR EXISTS(SELECT 1 FROM reward_orders WHERE brand_id=$1 AND id=$5)) AS valid),
 facts AS (
 SELECT l.id,l.created_at AS posted_at,o.member_id,o.id AS order_id,'grant'::text AS kind,
        o.points::numeric AS points,o.points::numeric AS signed_points
 FROM point_ledger_entries l
	JOIN reward_order_actions a ON a.brand_id=l.brand_id AND a.ledger_entry_id=l.id
	JOIN reward_orders o ON o.brand_id=a.brand_id AND o.id=a.order_id
	JOIN audit_logs witness ON witness.id=a.audit_log_id AND witness.brand_id=a.brand_id AND witness.actor_id=a.actor_id
	WHERE l.brand_id=$1 AND l.created_at >= $2 AND l.created_at < $3
	  AND l.entry_type='reward_grant' AND l.reference_type='reward_order' AND l.reference_id=o.id
	  AND o.grant_ledger_entry_id=l.id
	  AND l.member_id=o.member_id AND l.actor_type='admin' AND l.operation_key='reward-grant:'||o.id::text AND l.reversal_of IS NULL
	  AND a.operation='grant' AND a.version=1 AND a.state_after='granted' AND a.actor_id=l.actor_id
	  AND a.reason=l.reason AND witness.actor_type='admin' AND witness.action='reward.order.grant' AND witness.resource_type='reward_order' AND witness.resource_id=o.id AND witness.reason=a.reason
	  AND witness.request_id=l.request_id AND witness.after_json->>'action_id'=a.id::text AND witness.after_json->>'version'=a.version::text
	  AND witness.before_json='null'::jsonb AND witness.after_json->>'state'=a.state_after
	  AND witness.after_json->>'member_id'=o.member_id::text AND witness.after_json->>'points'=o.points::text
  AND l.delta_snapshot=jsonb_build_object('recharge',jsonb_build_object('available','0','manual_frozen','0','system_frozen','0','withdrawal','0'),
   'winning',jsonb_build_object('available','0','manual_frozen','0','system_frozen','0','withdrawal','0'),
   'gift',jsonb_build_object('available',o.points::text,'manual_frozen','0','system_frozen','0','withdrawal','0'),
   'commission',jsonb_build_object('available','0','manual_frozen','0','system_frozen','0','withdrawal','0'))
  AND l.source_allocation=jsonb_build_array(jsonb_build_object('source','gift','state','available','points',o.points::text))
 UNION ALL
 SELECT l.id,l.created_at,o.member_id,o.id,'reversal'::text,o.points::numeric,-o.points::numeric
 FROM point_ledger_entries l
	JOIN reward_order_actions a ON a.brand_id=l.brand_id AND a.ledger_entry_id=l.id
	JOIN reward_orders o ON o.brand_id=a.brand_id AND o.id=a.order_id
	JOIN audit_logs witness ON witness.id=a.audit_log_id AND witness.brand_id=a.brand_id AND witness.actor_id=a.actor_id
	 WHERE l.brand_id=$1 AND l.created_at >= $2 AND l.created_at < $3
	  AND l.entry_type='reward_reversal' AND l.reference_type='reward_order' AND l.reference_id=o.id
	  AND o.state='revoked' AND o.revoke_ledger_entry_id=l.id
	  AND l.member_id=o.member_id AND l.actor_type='admin' AND l.operation_key='reward-revoke:'||o.id::text AND l.reversal_of=o.grant_ledger_entry_id
	  AND a.operation IN('revoke','retry') AND a.state_after='revoked' AND a.actor_id=l.actor_id
	  AND a.reason=l.reason AND witness.actor_type='admin' AND witness.action='reward.order.'||a.operation AND witness.resource_type='reward_order' AND witness.resource_id=o.id AND witness.reason=a.reason
	  AND witness.request_id=l.request_id AND witness.after_json->>'action_id'=a.id::text AND witness.after_json->>'version'=a.version::text
	  AND witness.after_json->>'state'=a.state_after AND witness.before_json->>'version'=(a.version-1)::text AND witness.before_json->>'state'=a.state_before
  AND l.delta_snapshot=jsonb_build_object('recharge',jsonb_build_object('available','0','manual_frozen','0','system_frozen','0','withdrawal','0'),
   'winning',jsonb_build_object('available','0','manual_frozen','0','system_frozen','0','withdrawal','0'),
   'gift',jsonb_build_object('available','-'||o.points::text,'manual_frozen','0','system_frozen','0','withdrawal','0'),
   'commission',jsonb_build_object('available','0','manual_frozen','0','system_frozen','0','withdrawal','0'))
  AND l.source_allocation=jsonb_build_array(jsonb_build_object('source','gift','state','available','points',o.points::text))),
 base AS (SELECT f.*,` + group + ` AS key,` + group + ` AS label FROM facts f JOIN scope b ON true
 WHERE ($4::uuid IS NULL OR f.member_id=$4) AND ($5::uuid IS NULL OR f.order_id=$5)),
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
