package commission

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

const cycleReadJSON = `jsonb_build_object(
 'id',c.id::text,'brand_id',c.brand_id::text,'window_from',c.window_from,'window_to',c.window_to,
 'anchor_order_id',c.anchor_order_id::text,'calendar',c.calendar,'state',c.state,'version',c.version,
 'target_count',c.target_count::text,'scan_complete',c.scan_complete,'current_run_id',r.id::text,
 'current_generation',r.generation::text,'evidence_epoch',r.evidence_epoch::text,
 'evidence_current',coalesce(r.evidence_epoch=c.evidence_epoch,false),
 'calculated_count',coalesce((SELECT count(*)::text FROM commission_calculations x WHERE x.brand_id=c.brand_id AND x.cycle_id=c.id AND x.run_id=r.id),'0'),
 'earning_count',coalesce((SELECT count(*)::text FROM commission_earnings x WHERE x.brand_id=c.brand_id AND x.cycle_id=c.id AND x.run_id=r.id),'0'),
 'total_points',coalesce((SELECT sum(x.points)::text FROM commission_earnings x WHERE x.brand_id=c.brand_id AND x.cycle_id=c.id AND x.run_id=r.id),'0'),
 'created_by',c.created_by::text,'reason',c.reason,'created_at',c.created_at,'updated_at',c.updated_at,
 'last_error_code',c.last_error_code,'creation_audit_log_id',c.creation_audit_log_id::text)`

const cycleCurrentRunJoin = ` LEFT JOIN commission_runs r ON r.brand_id=c.brand_id AND r.cycle_id=c.id AND r.id=c.current_run_id `

func (s Service) CycleTx(ctx context.Context, tx pgx.Tx, brand, id string) (Cycle, error) {
	var out Cycle
	if tx == nil || !cycleCanonicalUUID.MatchString(brand) || !cycleCanonicalUUID.MatchString(id) {
		return out, ErrInvalid
	}
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT `+cycleReadJSON+` FROM commission_cycles c `+cycleCurrentRunJoin+` WHERE c.brand_id=$1 AND c.id=$2`, brand, id).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal(raw, &out); err != nil {
		return Cycle{}, err
	}
	out.WindowFrom = out.WindowFrom.UTC()
	out.WindowTo = out.WindowTo.UTC()
	out.CreatedAt = out.CreatedAt.UTC()
	out.UpdatedAt = out.UpdatedAt.UTC()
	return out, nil
}

func (s Service) CyclesTx(ctx context.Context, tx pgx.Tx, brand string, limit, offset int) (CycleListPage, error) {
	out := CycleListPage{BrandID: brand, Items: []Cycle{}, Limit: limit, Offset: offset}
	if tx == nil || !validCyclePage(brand, limit, offset) {
		return out, ErrInvalid
	}
	var raw []byte
	err := tx.QueryRow(ctx, `WITH page AS (
 SELECT c.* FROM commission_cycles c WHERE c.brand_id=$1 ORDER BY c.created_at DESC,c.id DESC LIMIT $2 OFFSET $3
), contents AS (
 SELECT `+cycleReadJSON+` data,c.created_at,c.id FROM page c `+cycleCurrentRunJoin+`
)
SELECT (SELECT count(*)::text FROM commission_cycles WHERE brand_id=$1),
 coalesce((SELECT jsonb_agg(data ORDER BY created_at DESC,id DESC) FROM contents),'[]'::jsonb)`, brand, limit, offset).Scan(&out.TotalCount, &raw)
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal(raw, &out.Items); err != nil {
		return out, err
	}
	for i := range out.Items {
		out.Items[i].WindowFrom = out.Items[i].WindowFrom.UTC()
		out.Items[i].WindowTo = out.Items[i].WindowTo.UTC()
		out.Items[i].CreatedAt = out.Items[i].CreatedAt.UTC()
		out.Items[i].UpdatedAt = out.Items[i].UpdatedAt.UTC()
	}
	return out, nil
}

const earningReadJSON = `jsonb_build_object(
 'id',e.id::text,'brand_id',e.brand_id::text,'cycle_id',e.cycle_id::text,'run_id',e.run_id::text,
 'agent_id',e.agent_id::text,'member_id',e.member_id::text,
 'exact_amount',jsonb_build_object('numerator',e.numerator,'denominator',e.denominator),
 'points',e.points::text,'created_at',e.created_at)`

func (s Service) EarningsTx(ctx context.Context, tx pgx.Tx, brand, cycle string, limit, offset int) (EarningPage, error) {
	out := EarningPage{BrandID: brand, CycleID: cycle, Items: []Earning{}, Limit: limit, Offset: offset}
	if tx == nil || !validCyclePage(brand, limit, offset) || !cycleCanonicalUUID.MatchString(cycle) {
		return out, ErrInvalid
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM commission_cycles WHERE brand_id=$1 AND id=$2)`, brand, cycle).Scan(&exists); err != nil {
		return out, err
	}
	if !exists {
		return out, ErrNotFound
	}
	var raw []byte
	err := tx.QueryRow(ctx, `WITH current_earnings AS (
 SELECT e.* FROM commission_cycles c
 JOIN commission_earnings e ON e.brand_id=c.brand_id AND e.cycle_id=c.id AND e.run_id=c.current_run_id
 WHERE c.brand_id=$1 AND c.id=$2
), page AS (
 SELECT * FROM current_earnings ORDER BY created_at DESC,id DESC LIMIT $3 OFFSET $4
), contents AS (
 SELECT `+earningReadJSON+` data,e.created_at,e.id FROM page e
)
SELECT (SELECT count(*)::text FROM current_earnings),
 coalesce((SELECT jsonb_agg(data ORDER BY created_at DESC,id DESC) FROM contents),'[]'::jsonb)`, brand, cycle, limit, offset).Scan(&out.TotalCount, &raw)
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal(raw, &out.Items); err != nil {
		return out, err
	}
	for i := range out.Items {
		out.Items[i].CreatedAt = out.Items[i].CreatedAt.UTC()
	}
	return out, nil
}
