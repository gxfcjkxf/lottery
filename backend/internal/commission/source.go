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

// Source reads immutable configured order facts inside the caller transaction.
// The commission worker separately validates the policy saved at purchase.
type Source struct{}

// Resolution distinguishes exclusions from corrupt final evidence. A missing
// witness for an otherwise final order is an error, never an eligible zero.
type Resolution struct {
	Reason string
	Fact   *OrderFact
}

// OrderFact is private configured settlement evidence. It must not be used as
// a public DTO. The original attribution is retained, not rebuilt from today's
// mutable agent tree; legacy/null commission policy is NOT a payout permission.
type OrderFact struct {
	BrandID, OrderID, MemberID, AccountID, GameID, PeriodID, RuleVersionID string
	DebitEntryID                                                           string
	BetLedgerVersion                                                       int64
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
	var period string
	err := tx.QueryRow(ctx, `SELECT p.id::text FROM platform_game_periods p JOIN retention_configured_orders() o ON o.period_id=p.id WHERE o.brand_id=$1 AND o.id=$2 FOR SHARE OF p NOWAIT`, brand, order).Scan(&period)
	if errors.Is(err, pgx.ErrNoRows) {
		return Resolution{}, ErrNotFound
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "55P03" {
		return Resolution{}, ErrBusy
	}
	if err != nil {
		return Resolution{}, err
	}
	var fact OrderFact
	var allocation, attribution []byte
	var settled *time.Time
	var loss points.Amount
	var reserved bool
	var payout, refund, lossEntry *string
	err = tx.QueryRow(ctx, `SELECT o.brand_id::text,o.id::text,o.brand_member_id::text,o.account_id::text,o.game_id::text,o.period_id::text,o.rule_version_id::text,o.debit_entry_id::text,
 o.status,o.effective_points,o.prize_points,o.loss_points,o.placed_at,o.settled_at,o.deduction_allocation,o.attribution_snapshot,l.version,
 coalesce(o.snapshot->'catalog'->>'funding'='reserve',false),o.payout_entry_id::text,o.refund_entry_id::text,o.loss_entry_id::text
 FROM betting_financial_facts o JOIN retention_point_ledger_entries() l ON l.id=o.debit_entry_id WHERE o.brand_id=$1 AND o.id=$2`, brand, order).Scan(
		&fact.BrandID, &fact.OrderID, &fact.MemberID, &fact.AccountID, &fact.GameID, &fact.PeriodID, &fact.RuleVersionID, &fact.DebitEntryID,
		&fact.Status, &fact.Basis.TurnoverPoints, &fact.PrizePoints, &loss, &fact.PlacedAt, &settled, &allocation, &attribution, &fact.BetLedgerVersion, &reserved, &payout, &refund, &lossEntry)
	if err != nil {
		return Resolution{}, err
	}
	if fact.Status == "placed" {
		return Resolution{Reason: "not_final"}, nil
	}
	if settled == nil || fact.BetLedgerVersion < 1 || !json.Valid(attribution) || json.Unmarshal(allocation, &fact.Allocation) != nil {
		return Resolution{}, ErrEvidence
	}
	if (fact.PrizePoints > 0) != (payout != nil) {
		return Resolution{}, ErrEvidence
	}
	fact.SettledAt = *settled
	fact.Basis.LossPoints = loss
	target := ""
	if reserved {
		target = "system_frozen"
	}
	if err = verifyPosting(ctx, tx, fact, fact.DebitEntryID, "bet", "configured_order", fact.OrderID, fact.Allocation, "available", target); err != nil {
		return Resolution{}, err
	}
	if payout != nil {
		if err = verifyPosting(ctx, tx, fact, *payout, "prize", "configured_order", fact.OrderID, []points.Allocation{{Source: "winning", State: "available", Points: fact.PrizePoints}}, "", "available"); err != nil {
			return Resolution{}, err
		}
	}
	for _, entry := range []struct {
		id             *string
		kind, from, to string
	}{{refund, "refund", target, "available"}, {lossEntry, "bet", "system_frozen", ""}} {
		if entry.id == nil {
			continue
		}
		var raw []byte
		var alloc []points.Allocation
		if err = tx.QueryRow(ctx, `SELECT source_allocation FROM retention_point_ledger_entries() WHERE id=$1`, *entry.id).Scan(&raw); err != nil {
			return Resolution{}, err
		}
		if json.Unmarshal(raw, &alloc) != nil {
			return Resolution{}, ErrEvidence
		}
		if err = verifyPosting(ctx, tx, fact, *entry.id, entry.kind, "configured_order", fact.OrderID, alloc, entry.from, entry.to); err != nil {
			return Resolution{}, err
		}
	}
	fact.AttributionSnapshot = append(json.RawMessage(nil), attribution...)
	return Resolution{Reason: "final", Fact: &fact}, nil
}

func verifyPosting(ctx context.Context, tx pgx.Tx, fact OrderFact, id, kind, reference, referenceID string, expectedAllocation []points.Allocation, from, to string) error {
	var allocation, delta, before, after []byte
	err := tx.QueryRow(ctx, `SELECT source_allocation,delta_snapshot,before_snapshot,after_snapshot
 FROM retention_point_ledger_entries() WHERE brand_id=$1 AND member_id=$2 AND account_id=$3 AND id=$4
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
