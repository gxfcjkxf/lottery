package commission

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
)

type CorrectionCase struct {
	ID             string         `json:"id"`
	Plan           CorrectionPlan `json:"plan"`
	ExecutionID    *string        `json:"execution_id"`
	ExecutionState *string        `json:"execution_state"`
	Queue          string         `json:"queue"`
}
type CorrectionCasePage struct {
	BrandID    string           `json:"brand_id"`
	Items      []CorrectionCase `json:"items"`
	TotalCount string           `json:"total_count"`
	Limit      int              `json:"limit"`
	Offset     int              `json:"offset"`
}

// CorrectionCasesTx joins each saved plan with its execution, without inferring financial authorization.
func (s Service) CorrectionCasesTx(ctx context.Context, tx pgx.Tx, brand string, limit, offset int, filter ListFilter) (CorrectionCasePage, error) {
	out := CorrectionCasePage{BrandID: brand, Items: []CorrectionCase{}, Limit: limit, Offset: offset}
	if tx == nil || !validCyclePage(brand, limit, offset) || filter.Validate("correction") != nil {
		return out, ErrInvalid
	}
	if err := filter.validateScope(ctx, tx, brand); err != nil {
		return out, err
	}
	var raw []byte
	err := tx.QueryRow(ctx, `WITH filtered AS (
 SELECT p.*,x.id execution_id,x.state execution_state,
 CASE WHEN p.state='stale' OR x.state IN ('completed','stale') THEN 'history'
 WHEN p.state='planning' OR x.state='applying' THEN 'processing'
 ELSE 'pending' END queue
 FROM commission_correction_plans p
 JOIN commission_cycles c ON c.brand_id=p.brand_id AND c.id=p.cycle_id
 LEFT JOIN commission_correction_executions x ON x.brand_id=p.brand_id AND x.plan_id=p.id
 WHERE p.brand_id=$1
 AND ($4::timestamptz IS NULL OR c.window_to >= $4) AND ($5::timestamptz IS NULL OR c.window_to < $5)
 AND ($7='' OR p.cycle_id::text=$7)
 AND ($8='' OR EXISTS(SELECT 1 FROM commission_correction_plan_targets t WHERE t.brand_id=p.brand_id AND t.plan_id=p.id AND t.agent_id::text=$8))
), matched AS (SELECT * FROM filtered WHERE $6='' OR queue=$6),
 page AS (SELECT * FROM matched ORDER BY created_at DESC,id DESC LIMIT $2 OFFSET $3)
 SELECT (SELECT count(*)::text FROM matched),coalesce((SELECT jsonb_agg(jsonb_build_object('id',p.id::text,'plan',`+correctionPlanJSON+`,'execution_id',p.execution_id::text,'execution_state',p.execution_state,'queue',p.queue) ORDER BY p.created_at DESC,p.id DESC) FROM page p),'[]'::jsonb)`, brand, limit, offset, filter.From, filter.To, filter.State, filter.CycleID, filter.AgentID).Scan(&out.TotalCount, &raw)
	if err != nil {
		return out, paymentDBError(err)
	}
	if err = json.Unmarshal(raw, &out.Items); err != nil {
		return out, err
	}
	return out, nil
}
