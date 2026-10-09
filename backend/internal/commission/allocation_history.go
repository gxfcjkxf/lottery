package commission

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/agency"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

// RunEarningPage explicitly binds an immutable generation. It never silently
// substitutes a later run and does not imply payment or wallet availability.
type RunEarningPage struct {
	BrandID    string    `json:"brand_id"`
	CycleID    string    `json:"cycle_id"`
	RunID      string    `json:"run_id"`
	Items      []Earning `json:"items"`
	TotalCount string    `json:"total_count"`
	Limit      int       `json:"limit"`
	Offset     int       `json:"offset"`
}

type AllocationQuery struct {
	Limit   int
	Offset  int
	AgentID *string
	OrderID *string
}

func (q AllocationQuery) valid(brand string) bool {
	if !validCyclePage(brand, q.Limit, q.Offset) {
		return false
	}
	for _, id := range []*string{q.AgentID, q.OrderID} {
		if id != nil && !cycleCanonicalUUID.MatchString(*id) {
			return false
		}
	}
	return true
}

// Allocation is one saved per-order beneficiary share, without rounding.
// MemberID is the beneficiary; BettorMemberID is the original betting member.
type Allocation struct {
	BrandID         string        `json:"brand_id"`
	CycleID         string        `json:"cycle_id"`
	RunID           string        `json:"run_id"`
	CalculationID   string        `json:"calculation_id"`
	OrderID         string        `json:"order_id"`
	AgentID         string        `json:"agent_id"`
	MemberID        string        `json:"member_id"`
	BettorMemberID  string        `json:"bettor_member_id"`
	BasePoints      points.Amount `json:"base_points"`
	Mode            string        `json:"mode"`
	AgentRatio      string        `json:"agent_ratio"`
	DownstreamRatio string        `json:"downstream_ratio"`
	DifferenceRatio string        `json:"difference_ratio"`
	ExactAmount     ExactAmount   `json:"exact_amount"`
	CreatedAt       time.Time     `json:"created_at"`
}

type AllocationPage struct {
	BrandID    string       `json:"brand_id"`
	CycleID    string       `json:"cycle_id"`
	RunID      string       `json:"run_id"`
	AgentID    *string      `json:"agent_id"`
	OrderID    *string      `json:"order_id"`
	Items      []Allocation `json:"items"`
	TotalCount string       `json:"total_count"`
	Limit      int          `json:"limit"`
	Offset     int          `json:"offset"`
}

// Private evidence is decoded separately and is never part of a public DTO.
type allocationHistoryRow struct {
	Allocation
	Snapshot        json.RawMessage `json:"snapshot"`
	PlacedAt        time.Time       `json:"placed_at"`
	Reason          string          `json:"reason"`
	SnapshotMatches bool            `json:"snapshot_matches"`
}

func projectAllocation(row allocationHistoryRow) (Allocation, error) {
	if row.Reason != "eligible" || !row.SnapshotMatches || row.BasePoints < 0 {
		return Allocation{}, ErrPolicyEvidence
	}
	snapshot, err := ParseBetSnapshot(row.Snapshot, row.BrandID, row.BettorMemberID, row.PlacedAt)
	if err != nil || !snapshot.Financial.Config.Enabled {
		return Allocation{}, ErrPolicyEvidence
	}
	ratios := make([]string, len(snapshot.Path))
	index := -1
	for i, node := range snapshot.Path {
		ratios[i] = node.Config.Ratio
		if node.ID == row.AgentID {
			if node.MemberID != row.MemberID {
				return Allocation{}, ErrPolicyEvidence
			}
			index = i
		}
	}
	if index < 0 {
		return Allocation{}, ErrPolicyEvidence
	}
	exacts, err := DifferentialSameBasis(row.BasePoints, ratios)
	if err != nil || exacts[index] != row.ExactAmount {
		return Allocation{}, ErrPolicyEvidence
	}
	out := row.Allocation
	out.Mode = snapshot.Path[index].EffectiveMode
	out.AgentRatio = ratios[index]
	out.DownstreamRatio = "0"
	if index+1 < len(ratios) {
		out.DownstreamRatio = ratios[index+1]
	}
	parent, _ := agency.RatioMicros(out.AgentRatio)
	child, _ := agency.RatioMicros(out.DownstreamRatio)
	difference := parent - child
	if difference == 1000000 {
		out.DifferenceRatio = "1"
	} else if difference == 0 {
		out.DifferenceRatio = "0"
	} else {
		out.DifferenceRatio = strings.TrimRight(fmt.Sprintf("0.%06d", difference), "0")
	}
	out.CreatedAt = out.CreatedAt.UTC()
	return out, nil
}

func (s Service) EarningsForRunTx(ctx context.Context, tx pgx.Tx, brand, cycle, run string, limit, offset int) (RunEarningPage, error) {
	out := RunEarningPage{BrandID: brand, CycleID: cycle, RunID: run, Items: []Earning{}, Limit: limit, Offset: offset}
	if tx == nil || !validCyclePage(brand, limit, offset) || !cycleCanonicalUUID.MatchString(cycle) || !cycleCanonicalUUID.MatchString(run) {
		return out, ErrInvalid
	}
	var valid bool
	var raw []byte
	err := tx.QueryRow(ctx, `WITH scope AS (
 SELECT r.id FROM commission_runs r JOIN commission_cycles c ON c.brand_id=r.brand_id AND c.id=r.cycle_id
 WHERE r.brand_id=$1 AND r.cycle_id=$2 AND r.id=$3
), facts AS (
 SELECT e.* FROM commission_earnings e JOIN scope s ON s.id=e.run_id WHERE e.brand_id=$1 AND e.cycle_id=$2
), page AS (SELECT * FROM facts ORDER BY created_at DESC,id DESC LIMIT $4 OFFSET $5)
SELECT EXISTS(SELECT 1 FROM scope),(SELECT count(*)::text FROM facts),
 coalesce((SELECT jsonb_agg(`+earningReadJSON+` ORDER BY e.created_at DESC,e.id DESC) FROM page e),'[]'::jsonb)`, brand, cycle, run, limit, offset).Scan(&valid, &out.TotalCount, &raw)
	if err != nil {
		return out, err
	}
	if !valid {
		return out, ErrNotFound
	}
	if err = json.Unmarshal(raw, &out.Items); err != nil {
		return out, err
	}
	for i := range out.Items {
		out.Items[i].CreatedAt = out.Items[i].CreatedAt.UTC()
	}
	return out, nil
}

func (s Service) AllocationsTx(ctx context.Context, tx pgx.Tx, brand, cycle, run string, q AllocationQuery) (AllocationPage, error) {
	out := AllocationPage{BrandID: brand, CycleID: cycle, RunID: run, AgentID: q.AgentID, OrderID: q.OrderID, Items: []Allocation{}, Limit: q.Limit, Offset: q.Offset}
	if tx == nil || !q.valid(brand) || !cycleCanonicalUUID.MatchString(cycle) || !cycleCanonicalUUID.MatchString(run) {
		return out, ErrInvalid
	}
	var valid bool
	var raw []byte
	var pageCount int
	err := tx.QueryRow(ctx, `WITH scope AS (
 SELECT r.id FROM commission_runs r JOIN commission_cycles c ON c.brand_id=r.brand_id AND c.id=r.cycle_id
 WHERE r.brand_id=$1 AND r.cycle_id=$2 AND r.id=$3
), facts AS (
 SELECT a.* FROM commission_allocations a JOIN scope s ON s.id=a.run_id
 WHERE a.brand_id=$1 AND a.cycle_id=$2 AND ($4::uuid IS NULL OR a.agent_id=$4) AND ($5::uuid IS NULL OR a.order_id=$5)
), page AS (SELECT * FROM facts ORDER BY order_id,agent_id LIMIT $6 OFFSET $7), contents AS (
 SELECT a.order_id,a.agent_id,jsonb_build_object(
 'brand_id',a.brand_id::text,'cycle_id',a.cycle_id::text,'run_id',a.run_id::text,'calculation_id',a.calculation_id::text,
 'order_id',a.order_id::text,'agent_id',a.agent_id::text,'member_id',a.member_id::text,'bettor_member_id',c.member_id::text,
 'base_points',c.base_points::text,'exact_amount',jsonb_build_object('numerator',a.numerator,'denominator',a.denominator),
 'created_at',c.created_at,'snapshot',c.rule_snapshot,'placed_at',o.placed_at,'reason',c.reason,
 'snapshot_matches',c.rule_snapshot=o.commission_rule_snapshot) data
 FROM page a JOIN commission_calculations c ON c.brand_id=a.brand_id AND c.cycle_id=a.cycle_id AND c.run_id=a.run_id AND c.id=a.calculation_id AND c.order_id=a.order_id
 JOIN bet_orders o ON o.brand_id=a.brand_id AND o.id=a.order_id
)
SELECT EXISTS(SELECT 1 FROM scope)
 AND ($4::uuid IS NULL OR EXISTS(SELECT 1 FROM agent_nodes WHERE brand_id=$1 AND id=$4))
 AND ($5::uuid IS NULL OR EXISTS(SELECT 1 FROM bet_orders WHERE brand_id=$1 AND id=$5)),
 (SELECT count(*)::text FROM facts),(SELECT count(*) FROM page),coalesce((SELECT jsonb_agg(data ORDER BY order_id,agent_id) FROM contents),'[]'::jsonb)`, brand, cycle, run, q.AgentID, q.OrderID, q.Limit, q.Offset).Scan(&valid, &out.TotalCount, &pageCount, &raw)
	if err != nil {
		return out, err
	}
	if !valid {
		return out, ErrNotFound
	}
	var rows []allocationHistoryRow
	if err = json.Unmarshal(raw, &rows); err != nil {
		return out, err
	}
	if len(rows) != pageCount {
		return out, ErrPolicyEvidence
	}
	for _, row := range rows {
		item, e := projectAllocation(row)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}
