// Package reporting provides scoped, snapshot-consistent read-only operational
// reports. Current order cohorts and historical wallet postings are distinct.
package reporting

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"math/big"
	"regexp"
	"strings"
	"time"
)

var ErrInvalid = errors.New("invalid report query")
var ErrNotFound = errors.New("report scope not found")
var ErrExportTooLarge = errors.New("report export exceeds group limit")

const ExportGroupLimit = 10000

var uuid = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type Service struct{ DB *pgxpool.Pool }

type rowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// PostgreSQL stores timestamps on a microsecond grid. Ceil both half-open
// bounds instead of letting the driver truncate sub-microsecond input: for a
// stored timestamp x, x>=from && x<to is then preserved exactly.
func databaseBound(t time.Time) time.Time {
	v := t.Truncate(time.Microsecond)
	if v.Before(t) {
		v = v.Add(time.Microsecond)
	}
	return v
}

type Query struct {
	From     time.Time `json:"from"`
	To       time.Time `json:"to"`
	GroupBy  string    `json:"group_by"`
	Limit    int       `json:"limit"`
	Offset   int       `json:"offset"`
	GameID   *string   `json:"game_id"`
	MemberID *string   `json:"member_id"`
}

func (q Query) Validate(kind string) error {
	if q.From.IsZero() || q.To.IsZero() || !q.To.After(q.From) || q.To.Sub(q.From) > 93*24*time.Hour || q.From.Year() < 1 || q.To.Year() > 9999 || q.Limit < 1 || q.Limit > 100 || q.Offset < 0 || q.Offset > 1000000 {
		return ErrInvalid
	}
	for _, id := range []*string{q.GameID, q.MemberID} {
		if id != nil && !uuid.MatchString(*id) {
			return ErrInvalid
		}
	}
	if kind == "betting" {
		if q.GroupBy != "day" && q.GroupBy != "game" && q.GroupBy != "member" {
			return ErrInvalid
		}
	} else if kind == "ledger" {
		if q.GameID != nil || (q.GroupBy != "day" && q.GroupBy != "entry_type") {
			return ErrInvalid
		}
	} else {
		return ErrInvalid
	}
	return nil
}

// Aggregates are decimal strings, not int64: sums of individually bounded
// accounts/orders can exceed int64 and must remain exact in JavaScript.
type BettingTotals struct {
	OrderCount             string `json:"order_count"`
	StakePoints            string `json:"stake_points"`
	PlacedCount            string `json:"placed_count"`
	WonCount               string `json:"won_count"`
	LostCount              string `json:"lost_count"`
	AbnormalCount          string `json:"abnormal_count"`
	CancelledCount         string `json:"cancelled_count"`
	RefundPoints           string `json:"refund_points"`
	SettledStakePoints     string `json:"settled_stake_points"`
	UnfinalizedStakePoints string `json:"unfinalized_stake_points"`
	AbnormalStakePoints    string `json:"abnormal_stake_points"`
	CurrentPrizePoints     string `json:"current_prize_points"`
	CorrectionOpenCount    string `json:"correction_open_count"`
}
type LedgerTotals struct {
	EntryCount          string `json:"entry_count"`
	NetPoints           string `json:"net_points"`
	RechargePoints      string `json:"recharge_points"`
	PrizeCreditPoints   string `json:"prize_credit_points"`
	PrizeReversalPoints string `json:"prize_reversal_points"`
	RefundPoints        string `json:"refund_points"`
}
type Balances struct {
	AccountCount     string `json:"account_count"`
	AvailablePoints  string `json:"available_points"`
	FrozenPoints     string `json:"frozen_points"`
	WithdrawalPoints string `json:"withdrawal_points"`
	TotalPoints      string `json:"total_points"`
}
type Group[T any] struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Totals T      `json:"totals"`
}
type Report[T any] struct {
	BrandID     string     `json:"brand_id"`
	SnapshotAt  time.Time  `json:"snapshot_at"`
	Timezone    string     `json:"timezone"`
	Query       Query      `json:"query"`
	Summary     T          `json:"summary"`
	Items       []Group[T] `json:"items"`
	TotalGroups string     `json:"total_groups"`
}
type LedgerReport struct {
	Report[LedgerTotals]
	Balances Balances `json:"balances"`
}

type aggregate struct{ name, expression string }

var betAggregates = []aggregate{
	{"order_count", "count(*)"}, {"stake_points", "coalesce(sum(total_points::numeric),0)"},
	{"placed_count", "count(*) FILTER(WHERE status='placed')"}, {"won_count", "count(*) FILTER(WHERE status='won')"},
	{"lost_count", "count(*) FILTER(WHERE status='lost')"}, {"abnormal_count", "count(*) FILTER(WHERE status='abnormal')"},
	{"cancelled_count", "count(*) FILTER(WHERE status IN ('bet_cancelled','judged_cancelled'))"},
	{"refund_points", "coalesce(sum(total_points::numeric) FILTER(WHERE refund_entry_id IS NOT NULL),0)"},
	{"settled_stake_points", "coalesce(sum(total_points::numeric) FILTER(WHERE final),0)"},
	{"unfinalized_stake_points", "coalesce(sum(total_points::numeric) FILTER(WHERE NOT final AND status IN ('placed','won','lost')),0)"},
	{"abnormal_stake_points", "coalesce(sum(total_points::numeric) FILTER(WHERE status='abnormal'),0)"},
	{"current_prize_points", "coalesce(sum(prize_points::numeric) FILTER(WHERE final),0)"},
	{"correction_open_count", "count(*) FILTER(WHERE correction_open)"},
}
var ledgerAggregates = []aggregate{
	{"entry_count", "count(*)"}, {"net_points", "coalesce(sum(net_points),0)"},
	{"recharge_points", "coalesce(sum(greatest(net_points,0)) FILTER(WHERE entry_type='recharge'),0)"},
	{"prize_credit_points", "coalesce(sum(greatest(net_points,0)) FILTER(WHERE entry_type='prize'),0)"},
	{"prize_reversal_points", "coalesce(sum(greatest(-net_points,0)) FILTER(WHERE entry_type='prize_reversal'),0)"},
	{"refund_points", "coalesce(sum(greatest(net_points,0)) FILTER(WHERE entry_type='refund'),0)"},
}

func aggregateJSON(fields []aggregate) string {
	parts := []string{}
	for _, v := range fields {
		parts = append(parts, "'"+v.name+"',("+v.expression+")::text")
	}
	return "jsonb_build_object(" + strings.Join(parts, ",") + ")"
}

const scopedBrand = `WITH scope AS (SELECT id,timezone FROM brands WHERE id=$1), validity AS (
 SELECT ($4::uuid IS NULL OR EXISTS(SELECT 1 FROM games WHERE brand_id=$1 AND id=$4)) AND
 ($5::uuid IS NULL OR EXISTS(SELECT 1 FROM brand_members WHERE brand_id=$1 AND id=$5)) AS valid)`

func reportTail(agg string, extra string) string {
	return `, grouped AS (SELECT key,label,` + agg + ` AS totals FROM base GROUP BY key,label),
 page AS (SELECT key,label,totals FROM grouped ORDER BY key COLLATE "C" LIMIT $6 OFFSET $7)
 SELECT statement_timestamp(),scope.timezone,validity.valid,(SELECT ` + agg + ` FROM base),
 COALESCE((SELECT jsonb_agg(to_jsonb(page) ORDER BY key COLLATE "C") FROM page),'[]'::jsonb),
 (SELECT count(*)::text FROM grouped)` + extra + ` FROM scope CROSS JOIN validity`
}
func (s Service) Betting(ctx context.Context, brand string, q Query) (Report[BettingTotals], error) {
	if s.DB == nil {
		return Report[BettingTotals]{BrandID: brand, Query: q, Items: []Group[BettingTotals]{}}, ErrInvalid
	}
	return s.betting(ctx, s.DB, brand, q, q.Limit, q.Offset, false)
}

func (s Service) BettingExport(ctx context.Context, tx pgx.Tx, brand string, q Query) (Report[BettingTotals], error) {
	if tx == nil || q.Offset != 0 {
		return Report[BettingTotals]{BrandID: brand, Query: q, Items: []Group[BettingTotals]{}}, ErrInvalid
	}
	return s.betting(ctx, tx, brand, q, ExportGroupLimit+1, 0, true)
}

func (s Service) betting(ctx context.Context, runner rowQuerier, brand string, q Query, limit, offset int, exporting bool) (Report[BettingTotals], error) {
	out := Report[BettingTotals]{BrandID: brand, Query: q, Items: []Group[BettingTotals]{}}
	if runner == nil || !uuid.MatchString(brand) || q.Validate("betting") != nil {
		return out, ErrInvalid
	}
	group, label := `to_char(o.placed_at AT TIME ZONE b.timezone,'YYYY-MM-DD')`, `to_char(o.placed_at AT TIME ZONE b.timezone,'YYYY-MM-DD')`
	if q.GroupBy == "game" {
		group, label = `o.game_id::text`, `g.name`
	} else if q.GroupBy == "member" {
		group, label = `o.brand_member_id::text`, `o.brand_member_id::text`
	}
	sql := scopedBrand + `, facts AS (SELECT o.*,` + group + ` AS key,` + label + ` AS label,
 EXISTS(SELECT 1 FROM draw_corrections d WHERE d.brand_id=o.brand_id AND d.period_id=o.period_id AND d.state<>'completed') AS correction_open,
 (o.status IN ('won','lost') AND p.status='settled' AND j.state='completed' AND c.id IS NOT NULL) AS finalized
 FROM bet_orders o JOIN scope b ON b.id=o.brand_id JOIN games g ON g.brand_id=o.brand_id AND g.id=o.game_id
 JOIN periods p ON p.brand_id=o.brand_id AND p.id=o.period_id
 LEFT JOIN settlement_jobs j ON j.brand_id=o.brand_id AND j.id=p.current_settlement_job_id
 LEFT JOIN settlement_calculations c ON c.brand_id=o.brand_id AND c.id=o.settlement_calculation_id AND c.job_id=j.id AND c.order_id=o.id
 WHERE o.placed_at >= $2 AND o.placed_at < $3 AND ($4::uuid IS NULL OR o.game_id=$4) AND ($5::uuid IS NULL OR o.brand_member_id=$5)),
 base AS (SELECT facts.*,coalesce(finalized,false) AND NOT correction_open AS final FROM facts)` + reportTail(aggregateJSON(betAggregates), "")
	var valid bool
	var summary, rows []byte
	e := runner.QueryRow(ctx, sql, brand, databaseBound(q.From), databaseBound(q.To), q.GameID, q.MemberID, limit, offset).Scan(&out.SnapshotAt, &out.Timezone, &valid, &summary, &rows, &out.TotalGroups)
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
func (s Service) Ledger(ctx context.Context, brand string, q Query) (LedgerReport, error) {
	if s.DB == nil {
		return LedgerReport{Report: Report[LedgerTotals]{BrandID: brand, Query: q, Items: []Group[LedgerTotals]{}}}, ErrInvalid
	}
	return s.ledger(ctx, s.DB, brand, q, q.Limit, q.Offset, false)
}

func (s Service) LedgerExport(ctx context.Context, tx pgx.Tx, brand string, q Query) (LedgerReport, error) {
	if tx == nil || q.Offset != 0 {
		return LedgerReport{Report: Report[LedgerTotals]{BrandID: brand, Query: q, Items: []Group[LedgerTotals]{}}}, ErrInvalid
	}
	return s.ledger(ctx, tx, brand, q, ExportGroupLimit+1, 0, true)
}

func (s Service) ledger(ctx context.Context, runner rowQuerier, brand string, q Query, limit, offset int, exporting bool) (LedgerReport, error) {
	out := LedgerReport{Report: Report[LedgerTotals]{BrandID: brand, Query: q, Items: []Group[LedgerTotals]{}}}
	if runner == nil || !uuid.MatchString(brand) || q.Validate("ledger") != nil {
		return out, ErrInvalid
	}
	group := `to_char(l.created_at AT TIME ZONE b.timezone,'YYYY-MM-DD')`
	if q.GroupBy == "entry_type" {
		group = `l.entry_type`
	}
	sql := scopedBrand + `, base AS (SELECT l.entry_type,` + group + ` AS key,` + group + ` AS label,
 (SELECT sum(v.value::numeric) FROM jsonb_each(l.delta_snapshot) s CROSS JOIN LATERAL jsonb_each_text(s.value) v) AS net_points
 FROM point_ledger_entries l JOIN scope b ON b.id=l.brand_id WHERE l.created_at >= $2 AND l.created_at < $3 AND ($5::uuid IS NULL OR l.member_id=$5)),
 accounts AS (SELECT id FROM point_accounts WHERE brand_id=$1 AND ($5::uuid IS NULL OR brand_member_id=$5)),
 balances AS (SELECT jsonb_build_object('account_count',(SELECT count(*)::text FROM accounts),
 'available_points',coalesce(sum(points::numeric) FILTER(WHERE state='available'),0)::text,
 'frozen_points',coalesce(sum(points::numeric) FILTER(WHERE state IN ('manual_frozen','system_frozen')),0)::text,
 'withdrawal_points',coalesce(sum(points::numeric) FILTER(WHERE state='withdrawal'),0)::text,
 'total_points',coalesce(sum(points::numeric),0)::text) AS totals FROM point_buckets WHERE brand_id=$1 AND account_id IN(SELECT id FROM accounts))` + reportTail(aggregateJSON(ledgerAggregates), `,(SELECT totals FROM balances)`)
	var valid bool
	var summary, rows, balances []byte
	e := runner.QueryRow(ctx, sql, brand, databaseBound(q.From), databaseBound(q.To), q.GameID, q.MemberID, limit, offset).Scan(&out.SnapshotAt, &out.Timezone, &valid, &summary, &rows, &out.TotalGroups, &balances)
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
	if e == nil {
		e = json.Unmarshal(balances, &out.Balances)
	}
	return out, e
}

func groupCountExceeds(count string, limit int64) bool {
	n, ok := new(big.Int).SetString(count, 10)
	return !ok || n.Cmp(big.NewInt(limit)) > 0
}
