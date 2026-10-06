package betting

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
	"io"
	"math/big"
	"time"
)

// Previews are immutable computation evidence, never settlement or payout.
type SettlementPreviewInput struct {
	Version       int64  `json:"version"`
	PeriodVersion int64  `json:"period_version"`
	DrawResultID  string `json:"draw_result_id"`
	Reason        string `json:"reason"`
}

func (in *SettlementPreviewInput) UnmarshalJSON(raw []byte) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	token, e := d.Token()
	if e != nil || token != json.Delim('{') {
		return ErrInvalid
	}
	fields := map[string]json.RawMessage{}
	for d.More() {
		k, e := d.Token()
		if e != nil {
			return ErrInvalid
		}
		name, ok := k.(string)
		if !ok || fields[name] != nil {
			return ErrInvalid
		}
		switch name {
		case "version", "period_version", "draw_result_id", "reason":
		default:
			return ErrInvalid
		}
		var v json.RawMessage
		if d.Decode(&v) != nil || bytes.Equal(v, []byte("null")) {
			return ErrInvalid
		}
		fields[name] = v
	}
	if _, e = d.Token(); e != nil || len(fields) != 4 {
		return ErrInvalid
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return ErrInvalid
	}
	var out SettlementPreviewInput
	for _, v := range []struct {
		key string
		dst any
	}{{"version", &out.Version}, {"period_version", &out.PeriodVersion}, {"draw_result_id", &out.DrawResultID}, {"reason", &out.Reason}} {
		if json.Unmarshal(fields[v.key], v.dst) != nil {
			return ErrInvalid
		}
	}
	*in = out
	return nil
}

type SettlementContext struct {
	BrandID        string      `json:"brand_id"`
	GameID         string      `json:"game_id"`
	PeriodID       string      `json:"period_id"`
	OrderID        string      `json:"order_id"`
	OrderVersion   int64       `json:"order_version"`
	OrderStatus    string      `json:"order_status"`
	DefinitionHash string      `json:"definition_hash"`
	PeriodVersion  int64       `json:"period_version"`
	PeriodStatus   string      `json:"period_status"`
	DrawResultID   *string     `json:"draw_result_id"`
	DrawHash       *string     `json:"draw_hash"`
	Draw           *rules.Draw `json:"draw"`
	CanPreview     bool        `json:"can_preview"`
}
type SettlementSummary struct {
	Won               bool          `json:"won"`
	CombinationCount  int           `json:"combination_count"`
	Multiplier        points.Amount `json:"multiplier"`
	BetPoints         points.Amount `json:"bet_points"`
	PrizePoints       points.Amount `json:"prize_points"`
	RawPrizePoints    string        `json:"raw_prize_points"`
	CappedPrizePoints string        `json:"capped_prize_points"`
}
type SettlementPreview struct {
	ID             string             `json:"id"`
	BrandID        string             `json:"brand_id"`
	GameID         string             `json:"game_id"`
	PeriodID       string             `json:"period_id"`
	OrderID        string             `json:"order_id"`
	OrderVersion   int64              `json:"order_version"`
	OrderStatus    string             `json:"order_status"`
	PeriodVersion  int64              `json:"period_version"`
	PeriodStatus   string             `json:"period_status"`
	DrawResultID   string             `json:"draw_result_id"`
	DefinitionHash string             `json:"definition_hash"`
	DrawHash       string             `json:"draw_hash"`
	Draw           rules.Draw         `json:"draw"`
	Outcome        string             `json:"outcome"`
	ErrorCode      *string            `json:"error_code"`
	Calculation    *SettlementSummary `json:"calculation"`
	CreatedBy      string             `json:"created_by"`
	CreatedAt      time.Time          `json:"created_at"`
	Reason         string             `json:"reason"`
	AuditLogID     string             `json:"audit_log_id"`
	Current        bool               `json:"current"`
	Applied        bool               `json:"applied"`
}
type SettlementPreviewPage struct {
	BrandID string              `json:"brand_id"`
	OrderID string              `json:"order_id"`
	Items   []SettlementPreview `json:"items"`
	Limit   int                 `json:"limit"`
	Offset  int                 `json:"offset"`
	HasMore bool                `json:"has_more"`
}
type SettlementLinePage struct {
	BrandID   string             `json:"brand_id"`
	PreviewID string             `json:"preview_id"`
	OrderID   string             `json:"order_id"`
	Items     []rules.LineResult `json:"items"`
	Limit     int                `json:"limit"`
	Offset    int                `json:"offset"`
	HasMore   bool               `json:"has_more"`
	Total     int                `json:"total"`
}
type settlementQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func settlementContext(ctx context.Context, q settlementQuerier, brand, id string) (SettlementContext, error) {
	var c SettlementContext
	var raw []byte
	e := q.QueryRow(ctx, `SELECT o.brand_id::text,o.game_id::text,o.period_id::text,o.id::text,o.version,o.status,o.definition_hash,p.version,p.status,p.draw_result_id::text,d.result_hash,d.result
 FROM bet_orders o JOIN periods p ON p.brand_id=o.brand_id AND p.id=o.period_id LEFT JOIN draw_results d ON d.id=p.draw_result_id
 WHERE o.brand_id=$1 AND o.id=$2`, brand, id).Scan(&c.BrandID, &c.GameID, &c.PeriodID, &c.OrderID, &c.OrderVersion, &c.OrderStatus, &c.DefinitionHash, &c.PeriodVersion, &c.PeriodStatus, &c.DrawResultID, &c.DrawHash, &raw)
	if errors.Is(e, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	if e != nil {
		return c, e
	}
	if raw != nil {
		c.Draw = &rules.Draw{}
		if e = json.Unmarshal(raw, c.Draw); e != nil {
			return c, errors.Join(ErrSnapshot, e)
		}
	}
	c.CanPreview = c.PeriodStatus == "drawn" && c.DrawResultID != nil && c.DrawHash != nil && c.Draw != nil
	return c, nil
}
func (s Service) SettlementContext(ctx context.Context, brand, id string) (SettlementContext, error) {
	if !validIDs(brand, id) {
		return SettlementContext{}, ErrInvalid
	}
	return settlementContext(ctx, s.DB, brand, id)
}
func digest(value any) string {
	raw, e := json.Marshal(value)
	if e != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func equalJSON(a, b any) bool {
	ra, e1 := json.Marshal(a)
	rb, e2 := json.Marshal(b)
	return e1 == nil && e2 == nil && bytes.Equal(ra, rb)
}
func evaluateSettlementSnapshot(ctx context.Context, o Order, draw rules.Draw) (rules.Simulation, string, error) {
	var out rules.Simulation
	if e := ctx.Err(); e != nil {
		return out, "", e
	}
	if digest(o.Definition) != o.DefinitionHash {
		return out, "DEFINITION_HASH_MISMATCH", nil
	}
	q, e := rules.PrepareBet(ctx, o.Definition, o.SelectionRaw, o.Multiplier)
	if e != nil {
		if ctx.Err() != nil {
			return out, "", ctx.Err()
		}
		return out, "ORDER_SNAPSHOT_INVALID", nil
	}
	if q.UnitPoints != o.UnitPoints || q.CombinationCount != o.CombinationCount || q.Multiplier != o.Multiplier || q.BetPoints != o.TotalPoints || !equalJSON(q.Normalized, o.SelectionNormalized) || !equalJSON(q.Expanded, o.Expanded) {
		return out, "STAKE_SNAPSHOT_MISMATCH", nil
	}
	total := new(big.Int)
	last := -1
	for _, a := range o.Allocation {
		source, e := points.SourceIndex(a.Source)
		if e != nil || source <= last || a.State != "available" || a.Points <= 0 {
			return out, "ALLOCATION_SNAPSHOT_INVALID", nil
		}
		last = source
		total.Add(total, big.NewInt(int64(a.Points)))
	}
	if len(o.Allocation) == 0 || total.Cmp(big.NewInt(int64(o.TotalPoints))) != 0 {
		return out, "ALLOCATION_SNAPSHOT_INVALID", nil
	}
	if rules.ValidateDraw(o.Definition.Model, draw) != nil {
		return out, "DRAW_NOT_COMPATIBLE", nil
	}
	out, e = rules.SimulateContext(ctx, rules.SimulationInput{Definition: o.Definition, Selection: o.SelectionRaw, Multiplier: o.Multiplier, Draw: draw})
	if e != nil {
		if ctx.Err() != nil {
			return rules.Simulation{}, "", ctx.Err()
		}
		return rules.Simulation{}, "RULE_CALCULATION_INVALID", nil
	}
	if out.BetPoints != o.TotalPoints || out.CombinationCount != o.CombinationCount || !equalJSON(out.Normalized, o.SelectionNormalized) {
		return rules.Simulation{}, "STAKE_SNAPSHOT_MISMATCH", nil
	}
	return out, "", nil
}
func verifySettlementDebit(ctx context.Context, tx pgx.Tx, o Order) (bool, error) {
	var typ, ref, refID string
	var allocation, delta, before, after []byte
	e := tx.QueryRow(ctx, `SELECT entry_type,reference_type,coalesce(reference_id::text,''),source_allocation,delta_snapshot,before_snapshot,after_snapshot
 FROM point_ledger_entries WHERE brand_id=$1 AND account_id=$2 AND member_id=$3 AND id=$4`, o.BrandID, o.AccountID, o.MemberID, o.DebitEntryID).Scan(&typ, &ref, &refID, &allocation, &delta, &before, &after)
	if errors.Is(e, pgx.ErrNoRows) {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	var alloc []points.Allocation
	var d, b, a points.Balance
	if typ != "bet" || ref != "bet_order" || refID != o.ID || json.Unmarshal(allocation, &alloc) != nil || !equalJSON(alloc, o.Allocation) || json.Unmarshal(delta, &d) != nil || json.Unmarshal(before, &b) != nil || json.Unmarshal(after, &a) != nil {
		return false, nil
	}
	var expected points.Balance
	for _, v := range alloc {
		si, e := points.SourceIndex(v.Source)
		if e != nil || v.State != "available" || v.Points <= 0 {
			return false, nil
		}
		expected[si][0] = -v.Points
	}
	if d != expected {
		return false, nil
	}
	for si := range b {
		for st := range b[si] {
			n := new(big.Int).Add(big.NewInt(int64(b[si][st])), big.NewInt(int64(d[si][st])))
			if b[si][st] < 0 || a[si][st] < 0 || n.Cmp(big.NewInt(int64(a[si][st]))) != 0 {
				return false, nil
			}
		}
	}
	return true, nil
}
func (s Service) CreateSettlementPreview(ctx context.Context, tx pgx.Tx, brand string, a access.Account, id string, in SettlementPreviewInput, meta points.Metadata) (SettlementPreview, error) {
	var p SettlementPreview
	if tx == nil || !validIDs(brand, id, a.ID, in.DrawResultID) || in.Version < 1 || in.PeriodVersion < 1 || !validPolicyReason(in.Reason) {
		return p, ErrInvalid
	}
	if a.SuperAdmin || !access.Authorize(a, "settlement", "preview", access.ScopeBrand, brand) {
		return p, ErrDenied
	}
	var game, period string
	e := tx.QueryRow(ctx, `SELECT game_id::text,period_id::text FROM bet_orders WHERE brand_id=$1 AND id=$2`, brand, id).Scan(&game, &period)
	if errors.Is(e, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	if e != nil {
		return p, e
	}
	if _, _, _, e = s.lockPeriod(ctx, tx, brand, period); e != nil {
		return p, e
	}
	o, e := scanOrder(tx.QueryRow(ctx, `SELECT `+orderFields+` FROM bet_orders WHERE brand_id=$1 AND id=$2 FOR SHARE`, brand, id))
	badSnapshot := errors.Is(e, ErrSnapshot)
	if e != nil && !badSnapshot {
		return p, e
	}
	c, e := settlementContext(ctx, tx, brand, id)
	if e != nil {
		return p, e
	}
	if c.OrderVersion != in.Version || c.PeriodVersion != in.PeriodVersion || c.DrawResultID != nil && *c.DrawResultID != in.DrawResultID {
		return p, ErrVersion
	}
	if !c.CanPreview {
		return p, ErrState
	}
	p = SettlementPreview{ID: ids.New(), BrandID: brand, GameID: game, PeriodID: period, OrderID: id, OrderVersion: c.OrderVersion, OrderStatus: c.OrderStatus, PeriodVersion: c.PeriodVersion, PeriodStatus: c.PeriodStatus, DrawResultID: *c.DrawResultID, DefinitionHash: c.DefinitionHash, DrawHash: *c.DrawHash, Draw: *c.Draw, CreatedBy: a.ID, Reason: in.Reason, Current: true}
	var simulation *rules.Simulation
	code := ""
	switch o.Status {
	case "abnormal":
		p.Outcome = "excluded"
		code = "ORDER_ABNORMAL"
	case "bet_cancelled", "judged_cancelled":
		p.Outcome = "excluded"
		code = "ORDER_CANCELLED"
	case "won", "lost", "settled":
		p.Outcome = "excluded"
		code = "ORDER_ALREADY_SETTLED"
	default:
		p.Outcome = "abnormal"
		if badSnapshot {
			code = "ORDER_SNAPSHOT_INVALID"
		} else if digest(p.Draw) != p.DrawHash {
			code = "DRAW_HASH_MISMATCH"
		} else {
			v, issue, err := evaluateSettlementSnapshot(ctx, o, p.Draw)
			if err != nil {
				return p, err
			}
			code = issue
			if code == "" {
				ok, err := verifySettlementDebit(ctx, tx, o)
				if err != nil {
					return p, err
				}
				if !ok {
					code = "DEBIT_WITNESS_MISMATCH"
				} else {
					simulation = &v
					p.Calculation = &SettlementSummary{Won: v.Won, CombinationCount: v.CombinationCount, Multiplier: v.Multiplier, BetPoints: v.BetPoints, PrizePoints: v.PrizePoints, RawPrizePoints: v.RawPrizePoints, CappedPrizePoints: v.CappedPrizePoints}
					p.Outcome = "lost"
					if v.Won {
						p.Outcome = "won"
					}
				}
			}
		}
	}
	if code != "" {
		p.ErrorCode = &code
	}
	p.AuditLogID, e = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: a.ID, Action: "settlement.preview", ResourceType: "settlement_preview", ResourceID: p.ID, Reason: in.Reason, RequestID: meta.RequestID, IP: meta.IP, After: map[string]any{"order_id": id, "order_version": p.OrderVersion, "period_version": p.PeriodVersion, "draw_result_id": p.DrawResultID, "definition_hash": p.DefinitionHash, "outcome": p.Outcome, "error_code": p.ErrorCode, "calculation": p.Calculation, "applied": false}})
	if e != nil {
		return p, e
	}
	rawDraw, e := json.Marshal(p.Draw)
	if e != nil {
		return p, e
	}
	var rawCalc any
	if simulation != nil {
		raw, e := json.Marshal(simulation)
		if e != nil {
			return p, e
		}
		rawCalc = raw
	}
	e = tx.QueryRow(ctx, `INSERT INTO settlement_previews(id,brand_id,game_id,period_id,order_id,order_version,order_status,period_version,period_status,draw_result_id,definition_hash,draw_hash,draw,outcome,error_code,calculation,created_by,reason,audit_log_id)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19) RETURNING created_at`, p.ID, p.BrandID, p.GameID, p.PeriodID, p.OrderID, p.OrderVersion, p.OrderStatus, p.PeriodVersion, p.PeriodStatus, p.DrawResultID, p.DefinitionHash, p.DrawHash, rawDraw, p.Outcome, p.ErrorCode, rawCalc, p.CreatedBy, p.Reason, p.AuditLogID).Scan(&p.CreatedAt)
	p.CreatedAt = p.CreatedAt.UTC()
	return p, e
}

const settlementPreviewFields = `v.id::text,v.brand_id::text,v.game_id::text,v.period_id::text,v.order_id::text,v.order_version,v.order_status,v.period_version,v.period_status,v.draw_result_id::text,v.definition_hash,v.draw_hash,v.draw,v.outcome,v.error_code,
 CASE WHEN v.calculation IS NULL THEN NULL ELSE v.calculation-'lines'-'normalized'-'warnings' END,v.created_by::text,v.created_at,v.reason,v.audit_log_id::text,
 (o.version=v.order_version AND o.status=v.order_status AND o.definition_hash=v.definition_hash AND p.version=v.period_version AND p.status=v.period_status AND p.draw_result_id=v.draw_result_id AND d.result_hash=v.draw_hash)`
const settlementPreviewJoin = ` FROM settlement_previews v JOIN bet_orders o ON o.id=v.order_id JOIN periods p ON p.id=v.period_id JOIN draw_results d ON d.id=v.draw_result_id `

func scanSettlementPreview(row pgx.Row) (p SettlementPreview, e error) {
	var draw, calc []byte
	e = row.Scan(&p.ID, &p.BrandID, &p.GameID, &p.PeriodID, &p.OrderID, &p.OrderVersion, &p.OrderStatus, &p.PeriodVersion, &p.PeriodStatus, &p.DrawResultID, &p.DefinitionHash, &p.DrawHash, &draw, &p.Outcome, &p.ErrorCode, &calc, &p.CreatedBy, &p.CreatedAt, &p.Reason, &p.AuditLogID, &p.Current)
	if errors.Is(e, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	if e != nil {
		return p, e
	}
	if e = json.Unmarshal(draw, &p.Draw); e != nil {
		return p, e
	}
	if calc != nil {
		p.Calculation = &SettlementSummary{}
		if e = json.Unmarshal(calc, p.Calculation); e != nil {
			return p, e
		}
	}
	p.CreatedAt = p.CreatedAt.UTC()
	return p, nil
}
func (s Service) SettlementPreview(ctx context.Context, brand, id string) (SettlementPreview, error) {
	if !validIDs(brand, id) {
		return SettlementPreview{}, ErrInvalid
	}
	return scanSettlementPreview(s.DB.QueryRow(ctx, `SELECT `+settlementPreviewFields+settlementPreviewJoin+`WHERE v.brand_id=$1 AND v.id=$2`, brand, id))
}
func (s Service) SettlementPreviews(ctx context.Context, brand, order string, limit, offset int) (SettlementPreviewPage, error) {
	out := SettlementPreviewPage{BrandID: brand, OrderID: order, Items: []SettlementPreview{}, Limit: limit, Offset: offset}
	if !validIDs(brand, order) || limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		return out, ErrInvalid
	}
	var exists bool
	if e := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM bet_orders WHERE brand_id=$1 AND id=$2)`, brand, order).Scan(&exists); e != nil {
		return out, e
	}
	if !exists {
		return out, ErrNotFound
	}
	rows, e := s.DB.Query(ctx, `SELECT `+settlementPreviewFields+settlementPreviewJoin+`WHERE v.brand_id=$1 AND v.order_id=$2 ORDER BY v.created_at DESC,v.id DESC LIMIT $3 OFFSET $4`, brand, order, limit+1, offset)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		p, e := scanSettlementPreview(rows)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, p)
	}
	if e = rows.Err(); e != nil {
		return out, e
	}
	if len(out.Items) > limit {
		out.HasMore = true
		out.Items = out.Items[:limit]
	}
	return out, nil
}
func (s Service) SettlementPreviewLines(ctx context.Context, brand, id string, limit, offset int) (SettlementLinePage, error) {
	out := SettlementLinePage{BrandID: brand, PreviewID: id, Items: []rules.LineResult{}, Limit: limit, Offset: offset}
	if !validIDs(brand, id) || limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		return out, ErrInvalid
	}
	var raw []byte
	e := s.DB.QueryRow(ctx, `SELECT order_id::text,COALESCE(jsonb_array_length(calculation->'lines'),0),COALESCE((SELECT jsonb_agg(line ORDER BY ordinal) FROM jsonb_array_elements(calculation->'lines') WITH ORDINALITY AS x(line,ordinal) WHERE ordinal>$3 AND ordinal<=$3+$4),'[]'::jsonb) FROM settlement_previews WHERE brand_id=$1 AND id=$2`, brand, id, offset, limit).Scan(&out.OrderID, &out.Total, &raw)
	if errors.Is(e, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if e != nil {
		return out, e
	}
	if e = json.Unmarshal(raw, &out.Items); e != nil {
		return out, e
	}
	out.HasMore = offset+len(out.Items) < out.Total
	return out, nil
}
