package commission

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

// RunHistoryItem is an auditable, immutable run summary. It deliberately has
// no current-run or payout-authority field.
type RunHistoryItem struct {
	ID              string    `json:"id"`
	BrandID         string    `json:"brand_id"`
	CycleID         string    `json:"cycle_id"`
	Generation      string    `json:"generation"`
	EvidenceEpoch   string    `json:"evidence_epoch"`
	State           string    `json:"state"`
	CalculatedCount string    `json:"calculated_count"`
	EarningCount    string    `json:"earning_count"`
	TotalPoints     string    `json:"total_points"`
	CreatedAt       time.Time `json:"created_at"`
}

type RunPage struct {
	BrandID    string           `json:"brand_id"`
	CycleID    string           `json:"cycle_id"`
	Items      []RunHistoryItem `json:"items"`
	TotalCount string           `json:"total_count"`
	Limit      int              `json:"limit"`
	Offset     int              `json:"offset"`
}

// CalculationHistoryItem is a sanitized per-order calculation projection.
// It omits the saved rule snapshot, agent path, and account identity.
type CalculationHistoryItem struct {
	ID            string        `json:"id"`
	BrandID       string        `json:"brand_id"`
	CycleID       string        `json:"cycle_id"`
	RunID         string        `json:"run_id"`
	OrderID       string        `json:"order_id"`
	MemberID      string        `json:"member_id"`
	Reason        string        `json:"reason"`
	Status        string        `json:"status"`
	StakePoints   points.Amount `json:"stake_points"`
	PrizePoints   points.Amount `json:"prize_points"`
	BasePoints    points.Amount `json:"base_points"`
	JobID         *string       `json:"job_id"`
	CalculationID *string       `json:"calculation_id"`
	Generation    *string       `json:"generation"`
	AuditLogID    string        `json:"audit_log_id"`
	CreatedAt     time.Time     `json:"created_at"`
}

type CalculationPage struct {
	BrandID    string                   `json:"brand_id"`
	CycleID    string                   `json:"cycle_id"`
	RunID      string                   `json:"run_id"`
	Items      []CalculationHistoryItem `json:"items"`
	TotalCount string                   `json:"total_count"`
	Limit      int                      `json:"limit"`
	Offset     int                      `json:"offset"`
}

const runHistoryJSON = `jsonb_build_object(
 'id',r.id::text,'brand_id',r.brand_id::text,'cycle_id',r.cycle_id::text,
 'generation',r.generation::text,'evidence_epoch',r.evidence_epoch::text,'state',r.state,
 'calculated_count',(SELECT count(*)::text FROM commission_calculations c WHERE c.brand_id=r.brand_id AND c.cycle_id=r.cycle_id AND c.run_id=r.id),
 'earning_count',(SELECT count(*)::text FROM commission_earnings e WHERE e.brand_id=r.brand_id AND e.cycle_id=r.cycle_id AND e.run_id=r.id),
 'total_points',coalesce((SELECT sum(e.points)::text FROM commission_earnings e WHERE e.brand_id=r.brand_id AND e.cycle_id=r.cycle_id AND e.run_id=r.id),'0'),
 'created_at',r.created_at)`

// RunsTx pages every immutable run for a cycle, including abandoned and
// superseded generations, regardless of the cycle's current_run_id.
func (s Service) RunsTx(ctx context.Context, tx pgx.Tx, brand, cycle string, limit, offset int) (RunPage, error) {
	out := RunPage{BrandID: brand, CycleID: cycle, Items: []RunHistoryItem{}, Limit: limit, Offset: offset}
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
	err := tx.QueryRow(ctx, `WITH page AS (
 SELECT r.* FROM commission_runs r WHERE r.brand_id=$1 AND r.cycle_id=$2 ORDER BY r.generation DESC LIMIT $3 OFFSET $4
), contents AS (
 SELECT `+runHistoryJSON+` data,r.generation,r.id FROM page r
)
SELECT (SELECT count(*)::text FROM commission_runs WHERE brand_id=$1 AND cycle_id=$2),
 coalesce((SELECT jsonb_agg(data ORDER BY generation DESC,id) FROM contents),'[]'::jsonb)`, brand, cycle, limit, offset).Scan(&out.TotalCount, &raw)
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

const calculationHistoryJSON = `jsonb_build_object(
 'id',c.id::text,'brand_id',c.brand_id::text,'cycle_id',c.cycle_id::text,'run_id',c.run_id::text,
 'order_id',c.order_id::text,'member_id',c.member_id::text,'reason',c.reason,'status',c.status,
 'stake_points',c.stake_points::text,'prize_points',c.prize_points::text,'base_points',c.base_points::text,
 'job_id',c.job_id::text,'calculation_id',c.calculation_id::text,'generation',c.generation::text,
 'audit_log_id',c.audit_log_id::text,'created_at',c.created_at)`

// CalculationsTx addresses a specific run explicitly. It never substitutes the
// cycle's current run, so clients can inspect calculations after invalidation.
func (s Service) CalculationsTx(ctx context.Context, tx pgx.Tx, brand, cycle, runID string, limit, offset int) (CalculationPage, error) {
	out := CalculationPage{BrandID: brand, CycleID: cycle, RunID: runID, Items: []CalculationHistoryItem{}, Limit: limit, Offset: offset}
	if tx == nil || !validCyclePage(brand, limit, offset) || !cycleCanonicalUUID.MatchString(cycle) || !cycleCanonicalUUID.MatchString(runID) {
		return out, ErrInvalid
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(
 SELECT 1 FROM commission_runs r JOIN commission_cycles c ON c.brand_id=r.brand_id AND c.id=r.cycle_id
 WHERE r.brand_id=$1 AND r.cycle_id=$2 AND r.id=$3
)`, brand, cycle, runID).Scan(&exists); err != nil {
		return out, err
	}
	if !exists {
		return out, ErrNotFound
	}
	var raw []byte
	err := tx.QueryRow(ctx, `WITH page AS (
 SELECT c.* FROM commission_calculations c WHERE c.brand_id=$1 AND c.cycle_id=$2 AND c.run_id=$3 ORDER BY c.order_id LIMIT $4 OFFSET $5
), contents AS (
 SELECT `+calculationHistoryJSON+` data,c.order_id FROM page c
)
SELECT (SELECT count(*)::text FROM commission_calculations WHERE brand_id=$1 AND cycle_id=$2 AND run_id=$3),
 coalesce((SELECT jsonb_agg(data ORDER BY order_id) FROM contents),'[]'::jsonb)`, brand, cycle, runID, limit, offset).Scan(&out.TotalCount, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
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
