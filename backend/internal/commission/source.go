package commission

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"reflect"
	"regexp"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrNotFound = errors.New("commission order scope not found")
	ErrBusy     = errors.New("commission order period is being changed")
	ErrEvidence = errors.New("commission final order evidence is incomplete or invalid")
	uuid        = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

// Source is an internal, non-mutating reader, not an HTTP permission boundary
// or a commission accrual/payout service. A future worker must explicitly select
// and validate the financial policy saved with the bet before recording money.
type Source struct{}

// Resolution distinguishes exclusions from corrupt final evidence. A missing
// witness for an otherwise final order is an error, never an eligible zero.
type Resolution struct {
	Reason string
	Fact   *OrderFact
}

// OrderFact is private settlement-generation evidence. It must not be used as
// a public DTO. The original attribution is retained, not rebuilt from today's
// mutable agent tree; legacy/null commission policy is NOT a payout permission.
type OrderFact struct {
	BrandID, OrderID, MemberID, AccountID, GameID, PeriodID, RuleVersionID string
	DebitEntryID, JobID, CalculationID                                     string
	Generation, BetLedgerVersion                                           int64
	Status                                                                 string
	PlacedAt, SettledAt                                                    time.Time
	PrizePoints                                                            points.Amount
	Basis                                                                  Basis
	Allocation                                                             []points.Allocation
	AttributionSnapshot                                                    json.RawMessage `json:"-"`
}

func (Source) FinalOrderTx(ctx context.Context, tx pgx.Tx, brand, order string) (Resolution, error) {
	if tx == nil || !uuid.MatchString(brand) || !uuid.MatchString(order) {
		return Resolution{}, ErrInvalid
	}
	// Keep the current result/job pointer stable until the caller ends its
	// transaction. NOWAIT also remains safe if a future caller owns a wallet:
	// correction/settlement use period -> wallet, never invert that wait order.
	var period string
	err := tx.QueryRow(ctx, `SELECT p.id::text FROM periods p JOIN bet_orders o
 ON o.brand_id=p.brand_id AND o.period_id=p.id WHERE o.brand_id=$1 AND o.id=$2
 FOR SHARE OF p NOWAIT`, brand, order).Scan(&period)
	if errors.Is(err, pgx.ErrNoRows) {
		return Resolution{}, ErrNotFound
	}
	var pgerr *pgconn.PgError
	if errors.As(err, &pgerr) && pgerr.Code == "55P03" {
		return Resolution{}, ErrBusy
	}
	if err != nil {
		return Resolution{}, err
	}

	var fact OrderFact
	var stake points.Amount
	var settled *time.Time
	var job, calculation *string
	var generation, debitVersion *int64
	var allocation, attribution []byte
	var periodState string
	var refunded, correctionOpen, witnesses bool
	err = tx.QueryRow(ctx, `SELECT o.brand_id::text,o.id::text,o.brand_member_id::text,o.account_id::text,
 o.game_id::text,o.period_id::text,o.rule_version_id::text,o.debit_entry_id::text,
 o.status,o.total_points,o.prize_points,o.placed_at,o.settled_at,o.deduction_allocation,o.attribution_snapshot,
 p.status,o.refund_entry_id IS NOT NULL,
 EXISTS(SELECT 1 FROM draw_corrections dc WHERE dc.brand_id=o.brand_id AND dc.period_id=p.id AND dc.state<>'completed'),
 j.id::text,c.id::text,j.generation,l.version,
 coalesce(j.state='completed' AND j.completed_at IS NOT NULL AND j.period_id=p.id AND j.game_id=o.game_id
 AND j.draw_result_id=p.draw_result_id AND c.job_id=j.id AND c.period_id=p.id AND c.order_id=o.id
 AND c.order_version=o.version-1 AND c.definition_hash=o.definition_hash AND c.draw_hash=d.result_hash
 AND c.won=(o.status='won') AND c.prize_points=o.prize_points
 AND t.period_id=p.id AND t.state='paid' AND t.calculation_id=c.id
 AND t.payout_entry_id IS NOT DISTINCT FROM o.payout_entry_id
 AND l.entry_type='bet' AND l.reference_type='bet_order' AND l.reference_id=o.id
 AND ((o.prize_points=0 AND o.payout_entry_id IS NULL) OR
 (o.prize_points>0 AND pl.id IS NOT NULL AND pl.entry_type='prize' AND pl.reference_type='settlement_calculation'
 AND pl.reference_id=c.id AND pl.operation_key='settlement-payout:'||c.id::text)),false)
 FROM bet_orders o JOIN periods p ON p.brand_id=o.brand_id AND p.id=o.period_id
 LEFT JOIN settlement_jobs j ON j.brand_id=o.brand_id AND j.id=p.current_settlement_job_id
 LEFT JOIN settlement_calculations c ON c.brand_id=o.brand_id AND c.id=o.settlement_calculation_id
 LEFT JOIN settlement_targets t ON t.brand_id=o.brand_id AND t.job_id=j.id AND t.order_id=o.id
 LEFT JOIN draw_results d ON d.brand_id=o.brand_id AND d.id=p.draw_result_id AND d.period_id=p.id AND d.game_id=o.game_id
 LEFT JOIN point_ledger_entries l ON l.brand_id=o.brand_id AND l.account_id=o.account_id AND l.member_id=o.brand_member_id AND l.id=o.debit_entry_id
 LEFT JOIN point_ledger_entries pl ON pl.brand_id=o.brand_id AND pl.account_id=o.account_id AND pl.member_id=o.brand_member_id AND pl.id=o.payout_entry_id
 WHERE o.brand_id=$1 AND o.id=$2`, brand, order).Scan(
		&fact.BrandID, &fact.OrderID, &fact.MemberID, &fact.AccountID, &fact.GameID, &fact.PeriodID, &fact.RuleVersionID, &fact.DebitEntryID,
		&fact.Status, &stake, &fact.PrizePoints, &fact.PlacedAt, &settled, &allocation, &attribution, &periodState, &refunded, &correctionOpen,
		&job, &calculation, &generation, &debitVersion, &witnesses)
	if err != nil {
		return Resolution{}, err
	}
	switch fact.Status {
	case "bet_cancelled", "judged_cancelled":
		return Resolution{Reason: "cancelled"}, nil
	case "abnormal":
		return Resolution{Reason: "abnormal"}, nil
	case "placed", "won", "lost":
	default:
		return Resolution{}, ErrEvidence
	}
	if correctionOpen {
		return Resolution{Reason: "correction_open"}, nil
	}
	if fact.Status == "placed" || periodState != "settled" {
		return Resolution{Reason: "not_final"}, nil
	}
	if !witnesses || refunded || settled == nil || job == nil || calculation == nil || generation == nil || *generation < 1 || debitVersion == nil || *debitVersion < 1 || !json.Valid(attribution) {
		return Resolution{}, ErrEvidence
	}
	fact.JobID, fact.CalculationID, fact.Generation, fact.BetLedgerVersion = *job, *calculation, *generation, *debitVersion
	fact.SettledAt = *settled
	if json.Unmarshal(allocation, &fact.Allocation) != nil {
		return Resolution{}, ErrEvidence
	}
	var total big.Int
	for _, a := range fact.Allocation {
		total.Add(&total, big.NewInt(int64(a.Points)))
	}
	if total.Cmp(big.NewInt(int64(stake))) != 0 {
		return Resolution{}, ErrEvidence
	}
	if err = verifyPosting(ctx, tx, fact, fact.DebitEntryID, "bet", "bet_order", fact.OrderID, fact.Allocation, "available", ""); err != nil {
		return Resolution{}, err
	}
	if fact.PrizePoints > 0 {
		var payout string
		if err = tx.QueryRow(ctx, `SELECT payout_entry_id::text FROM bet_orders WHERE brand_id=$1 AND id=$2`, brand, order).Scan(&payout); err != nil {
			return Resolution{}, err
		}
		if err = verifyPosting(ctx, tx, fact, payout, "prize", "settlement_calculation", fact.CalculationID, []points.Allocation{{Source: "winning", State: "available", Points: fact.PrizePoints}}, "", "available"); err != nil {
			return Resolution{}, err
		}
	}
	fact.Basis, err = Bases(fact.Status, stake, fact.PrizePoints)
	if err != nil {
		return Resolution{}, ErrEvidence
	}
	fact.AttributionSnapshot = append(json.RawMessage(nil), attribution...)
	return Resolution{Reason: "final", Fact: &fact}, nil
}

func verifyPosting(ctx context.Context, tx pgx.Tx, fact OrderFact, id, kind, reference, referenceID string, expectedAllocation []points.Allocation, from, to string) error {
	var allocation, delta, before, after []byte
	err := tx.QueryRow(ctx, `SELECT source_allocation,delta_snapshot,before_snapshot,after_snapshot
 FROM point_ledger_entries WHERE brand_id=$1 AND member_id=$2 AND account_id=$3 AND id=$4
 AND entry_type=$5 AND reference_type=$6 AND reference_id=$7`, fact.BrandID, fact.MemberID, fact.AccountID, id, kind, reference, referenceID).Scan(&allocation, &delta, &before, &after)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrEvidence
	}
	if err != nil {
		return err
	}
	var alloc []points.Allocation
	var d, b, a points.Balance
	expected, err := points.AllocationDelta(expectedAllocation, from, to)
	if err != nil || json.Unmarshal(allocation, &alloc) != nil || !reflect.DeepEqual(alloc, expectedAllocation) || json.Unmarshal(delta, &d) != nil || json.Unmarshal(before, &b) != nil || json.Unmarshal(after, &a) != nil || d != expected || b.Validate() != nil || a.Validate() != nil {
		return ErrEvidence
	}
	for si := range b {
		for st := range b[si] {
			if new(big.Int).Add(big.NewInt(int64(b[si][st])), big.NewInt(int64(d[si][st]))).Cmp(big.NewInt(int64(a[si][st]))) != 0 {
				return ErrEvidence
			}
		}
	}
	return nil
}
