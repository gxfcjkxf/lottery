package betting

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"strconv"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/compliance"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/platformlottery"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

type ConfiguredInput struct {
	PeriodID     string                    `json:"period_id"`
	RuleID       string                    `json:"rule_id"`
	SalesVersion int64                     `json:"sales_version"`
	SnapshotHash string                    `json:"snapshot_hash"`
	ActorContext string                    `json:"actor_context"`
	Requests     []rules.ConfiguredRequest `json:"requests"`
}
type ConfiguredQuote struct {
	rules.ConfiguredQuote
	GameID       string `json:"game_id"`
	PeriodID     string `json:"period_id"`
	RuleID       string `json:"rule_id"`
	SalesVersion int64  `json:"sales_version"`
}

var ErrLegacyPeriod = errors.New("dedicated purchases exist in this period")

type ConfiguredOrder struct {
	GameName     string                  `json:"game_name"`
	MemberName   string                  `json:"member_name"`
	ID           string                  `json:"id"`
	GameID       string                  `json:"game_id"`
	PeriodID     string                  `json:"period_id"`
	PeriodNo     string                  `json:"period_no"`
	RuleID       string                  `json:"rule_id"`
	SalesVersion int64                   `json:"sales_version"`
	Snapshot     json.RawMessage         `json:"snapshot"`
	Quote        rules.ConfiguredQuote   `json:"quote"`
	Result       *rules.ConfiguredResult `json:"result"`
	Status       string                  `json:"status"`
	PlacedAt     time.Time               `json:"placed_at"`
	SettledAt    *time.Time              `json:"settled_at"`
}

type ConfiguredGameSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (s Service) ConfiguredGames(ctx context.Context, brand string) ([]ConfiguredGameSummary, error) {
	rows, e := s.DB.Query(ctx, `SELECT g.id::text,g.name FROM platform_games g JOIN brand_platform_games b ON b.game_id=g.id JOIN configured_rule_heads h ON h.game_id=g.id JOIN configured_sales s ON s.game_id=g.id AND s.brand_id=b.brand_id AND s.rule_id=h.version_id AND s.enabled WHERE b.brand_id=$1 AND b.enabled AND g.status='active' ORDER BY g.name,g.id`, brand)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []ConfiguredGameSummary{}
	for rows.Next() {
		var g ConfiguredGameSummary
		if e = rows.Scan(&g.ID, &g.Name); e != nil {
			return nil, e
		}
		out = append(out, g)
	}
	return out, rows.Err()
}
func (s Service) ConfiguredCatalog(ctx context.Context, brand, game string) (any, error) {
	detail, e := (platformlottery.Service{DB: s.DB}).Detail(ctx, brand, game, 50, 0)
	if e != nil {
		return nil, e
	}
	v, e := platformlottery.ReadConfiguredPublication(ctx, s.DB, game)
	if e != nil {
		return nil, e
	}
	sales, e := platformlottery.ReadConfiguredSales(ctx, s.DB, brand, game, false)
	if e != nil {
		return nil, e
	}
	engine, e := platformlottery.ConfiguredSalesEngine(v, sales)
	if e != nil {
		return nil, e
	}
	var period *platformlottery.Period
	for _, p := range detail.Periods {
		if p.Result == nil && p.BetEndAt.After(time.Now()) {
			candidate := p
			if e = platformlottery.BrandPeriod(ctx, s.DB, brand, &candidate); e != nil {
				return nil, e
			}
			if period == nil || candidate.BetStartAt.Before(period.BetStartAt) {
				period = &candidate
			}
		}
	}
	var snap rules.ConfiguredSnapshot
	if e = json.Unmarshal(engine.Snapshot(), &snap); e != nil {
		return nil, e
	}
	return map[string]any{"game": detail.Game, "snapshot": snap, "rule_id": v.ID, "sales": sales, "snapshot_hash": engine.SnapshotHash(), "period": period}, nil
}
func configuredWindow(ctx context.Context, tx pgx.Tx, brand, game, period, member string, total points.Amount, sale platformlottery.ConfiguredSales) error {
	var open, legacy bool
	if e := tx.QueryRow(ctx, `SELECT result IS NULL AND bet_start_at<=clock_timestamp() AND bet_end_at>clock_timestamp(), EXISTS(SELECT 1 FROM platform_mark_six_orders WHERE period_id=$1) OR EXISTS(SELECT 1 FROM platform_canada_orders WHERE period_id=$1) FROM platform_game_periods WHERE id=$1 AND game_id=$2`, period, game).Scan(&open, &legacy); e != nil {
		return e
	}
	if !open {
		return ErrClosed
	}
	// A period cannot mix independently scoped settlement caps and aggregates.
	if legacy {
		return ErrLegacyPeriod
	}
	var all, user string
	e := tx.QueryRow(ctx, `SELECT coalesce(sum(total_points::numeric),0)::text,coalesce(sum(total_points::numeric) FILTER(WHERE member_id=$3),0)::text FROM configured_orders WHERE brand_id=$1 AND period_id=$2`, brand, period, member).Scan(&all, &user)
	if e != nil {
		return e
	}
	for _, c := range []struct {
		raw   string
		limit points.Amount
	}{{all, sale.MaxPeriodPoints}, {user, sale.MaxUserPeriodPoints}} {
		n, ok := new(big.Int).SetString(c.raw, 10)
		if !ok {
			return ErrSnapshot
		}
		n.Add(n, big.NewInt(int64(total)))
		if n.Cmp(big.NewInt(int64(c.limit))) > 0 {
			return ErrLimit
		}
	}
	return nil
}
func (s Service) configuredQuote(ctx context.Context, tx pgx.Tx, brand string, v identity.Session, in ConfiguredInput, meta points.Metadata) (ConfiguredQuote, *rules.ConfiguredEngine, platformlottery.ConfiguredSales, error) {
	var q ConfiguredQuote
	var sales platformlottery.ConfiguredSales
	if !validIDs(brand, in.PeriodID, in.RuleID) {
		return q, nil, sales, ErrInvalid
	}
	if e := eligibility(ctx, tx, brand, v, true); e != nil {
		return q, nil, sales, e
	}
	var game string
	if e := tx.QueryRow(ctx, `SELECT game_id::text FROM platform_game_periods WHERE id=$1`, in.PeriodID).Scan(&game); errors.Is(e, pgx.ErrNoRows) {
		return q, nil, sales, ErrNotFound
	} else if e != nil {
		return q, nil, sales, e
	}
	if e := tx.QueryRow(ctx, `SELECT id::text FROM platform_games WHERE id=$1 FOR SHARE`, game).Scan(&game); e != nil {
		return q, nil, sales, e
	}
	var available bool
	if e := tx.QueryRow(ctx, `SELECT shared_game_accepts($1,$2)`, brand, game).Scan(&available); e != nil {
		return q, nil, sales, e
	}
	if !available {
		return q, nil, sales, ErrClosed
	}
	allowed, e := platformlottery.AutomationAllows(ctx, tx, brand, in.PeriodID)
	if e != nil {
		return q, nil, sales, e
	}
	if !allowed {
		return q, nil, sales, ErrClosed
	}
	sales, e = platformlottery.ReadConfiguredSales(ctx, tx, brand, game, true)
	if e != nil {
		return q, nil, sales, e
	}
	publication, e := platformlottery.ReadConfiguredPublication(ctx, tx, game)
	if e != nil {
		return q, nil, sales, e
	}
	if publication.ID != in.RuleID || sales.Version != in.SalesVersion {
		return q, nil, sales, ErrVersion
	}
	if !sales.Enabled {
		return q, nil, sales, ErrClosed
	}
	engine, e := platformlottery.ConfiguredSalesEngine(publication, sales)
	if e != nil {
		return q, nil, sales, e
	}
	if engine.SnapshotHash() != in.SnapshotHash {
		return q, nil, sales, ErrVersion
	}
	var period string
	if e = tx.QueryRow(ctx, `SELECT id::text FROM platform_game_periods WHERE id=$1 FOR SHARE`, in.PeriodID).Scan(&period); e != nil {
		return q, nil, sales, e
	}
	quote, e := engine.Prepare(ctx, in.Requests)
	if e != nil {
		return q, nil, sales, e
	}
	for _, line := range quote.Lines {
		if line.Stake < sales.MinPoints || line.Stake > sales.MaxPoints {
			return q, nil, sales, ErrLimit
		}
	}
	if e = configuredWindow(ctx, tx, brand, game, in.PeriodID, v.Member.ID, quote.BetPoints, sales); e != nil {
		return q, nil, sales, e
	}
	e = compliance.AssessTx(ctx, tx, brand, "bet_preview", compliance.GateSubject{ActorType: "user", ActorID: v.User.ID, MemberID: v.Member.ID, RequestID: meta.RequestID, IP: meta.IP})
	return ConfiguredQuote{ConfiguredQuote: quote, GameID: game, PeriodID: period, RuleID: publication.ID, SalesVersion: sales.Version}, engine, sales, e
}
func (s Service) PreviewConfigured(ctx context.Context, tx pgx.Tx, brand string, v identity.Session, in ConfiguredInput, meta points.Metadata) (ConfiguredQuote, error) {
	q, _, _, e := s.configuredQuote(ctx, tx, brand, v, in, meta)
	return q, e
}
func (s Service) PlaceConfigured(ctx context.Context, tx pgx.Tx, brand string, v identity.Session, in ConfiguredInput, meta points.Metadata) (ConfiguredOrder, error) {
	q, engine, sales, e := s.configuredQuote(ctx, tx, brand, v, in, meta)
	if e != nil {
		return ConfiguredOrder{}, e
	}
	ps := points.Store{DB: s.DB}
	wallet, e := ps.LockedSnapshot(ctx, tx, brand, v.Member.ID)
	if e != nil {
		return ConfiguredOrder{}, e
	}
	if e = eligibility(ctx, tx, brand, v, true); e != nil {
		return ConfiguredOrder{}, e
	}
	if e = configuredWindow(ctx, tx, brand, q.GameID, q.PeriodID, v.Member.ID, q.BetPoints, sales); e != nil {
		return ConfiguredOrder{}, e
	}
	if e = compliance.AssessTx(ctx, tx, brand, "bet_place", compliance.GateSubject{ActorType: "user", ActorID: v.User.ID, MemberID: v.Member.ID, RequestID: meta.RequestID, IP: meta.IP}); e != nil {
		return ConfiguredOrder{}, e
	}
	alloc, e := wallet.BySource.Allocate(engine.FundingAmount(q.ConfiguredQuote), "available")
	if e != nil {
		return ConfiguredOrder{}, e
	}
	target := ""
	if engine.FundingMode() == "reserve" {
		target = "system_frozen"
	}
	delta, e := points.AllocationDelta(alloc, "available", target)
	if e != nil {
		return ConfiguredOrder{}, e
	}
	id := ids.New()
	entry, e := ps.Post(ctx, tx, points.Change{BrandID: brand, MemberID: v.Member.ID, EntryType: "bet", ReferenceType: "configured_order", ReferenceID: id, OperationKey: "configured-bet:" + id, Reason: "confirmed configured stake", ActorType: "user", ActorID: v.User.ID, RequestID: meta.RequestID, IP: meta.IP, Delta: delta, Allocation: alloc})
	if e != nil {
		return ConfiguredOrder{}, e
	}
	if e = configuredWindow(ctx, tx, brand, q.GameID, q.PeriodID, v.Member.ID, q.BetPoints, sales); e != nil {
		return ConfiguredOrder{}, e
	}
	quote, _ := json.Marshal(q.ConfiguredQuote)
	allocation, _ := json.Marshal(alloc)
	saleRaw, _ := json.Marshal(sales)
	_, e = tx.Exec(ctx, `INSERT INTO configured_orders(id,brand_id,member_id,user_id,game_id,period_id,rule_id,sales_version,snapshot,quote,sales_snapshot,total_points,allocation,debit_entry_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, id, brand, v.Member.ID, v.User.ID, q.GameID, q.PeriodID, q.RuleID, q.SalesVersion, engine.Snapshot(), quote, saleRaw, q.BetPoints, allocation, entry.ID)
	if e != nil {
		return ConfiguredOrder{}, e
	}
	_, e = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "user", ActorID: v.User.ID, Action: "configured.place", ResourceType: "configured_order", ResourceID: id, Reason: "confirmed configured stake", RequestID: meta.RequestID, IP: meta.IP, After: q})
	if e != nil {
		return ConfiguredOrder{}, e
	}
	return scanConfiguredOrder(tx.QueryRow(ctx, configuredOrderSelect+` WHERE o.id=$1`, id))
}

const configuredOrderSelect = `SELECT o.id::text,o.game_id::text,o.period_id::text,p.period_no,o.rule_id::text,o.sales_version,o.snapshot,o.quote,o.result,o.status,o.placed_at,o.settled_at,u.username,g.name FROM configured_orders o JOIN platform_game_periods p ON p.id=o.period_id JOIN global_users u ON u.id=o.user_id JOIN platform_games g ON g.id=o.game_id`

func scanConfiguredOrder(row pgx.Row) (o ConfiguredOrder, e error) {
	var q, r []byte
	e = row.Scan(&o.ID, &o.GameID, &o.PeriodID, &o.PeriodNo, &o.RuleID, &o.SalesVersion, &o.Snapshot, &q, &r, &o.Status, &o.PlacedAt, &o.SettledAt, &o.MemberName, &o.GameName)
	if e != nil {
		return
	}
	e = json.Unmarshal(q, &o.Quote)
	if e == nil && len(r) > 0 {
		o.Result = &rules.ConfiguredResult{}
		e = json.Unmarshal(r, o.Result)
	}
	return
}
func (s Service) ConfiguredOrders(ctx context.Context, brand, member, game string, offset int) ([]ConfiguredOrder, error) {
	if !validIDs(brand) || (game != "" && !validIDs(game)) || (member != "" && !validIDs(member)) || offset < 0 || offset > 1000000 {
		return nil, ErrInvalid
	}
	rows, e := s.DB.Query(ctx, configuredOrderSelect+` WHERE o.brand_id=$1 AND ($2='' OR o.member_id::text=$2) AND ($3='' OR o.game_id::text=$3) ORDER BY o.placed_at DESC,o.id DESC LIMIT 50 OFFSET $4`, brand, member, game, offset)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []ConfiguredOrder{}
	for rows.Next() {
		o, e := scanConfiguredOrder(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
func (s Service) ProcessConfiguredSettlements(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 100 {
		return 0, ErrInvalid
	}
	for i := 0; i < limit; i++ {
		done, e := s.processConfiguredSettlement(ctx)
		if e != nil {
			return i, e
		}
		if !done {
			return i, nil
		}
	}
	return limit, nil
}
func (s Service) processConfiguredSettlement(ctx context.Context) (bool, error) {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return false, e
	}
	defer tx.Rollback(ctx)
	var id, brand, member string
	var rawAlloc []byte
	e = tx.QueryRow(ctx, `SELECT o.id::text,o.brand_id::text,o.member_id::text,o.allocation FROM configured_orders o JOIN platform_game_periods p ON p.id=o.period_id WHERE o.status='placed' AND p.result IS NOT NULL ORDER BY p.draw_at,o.id FOR UPDATE OF o SKIP LOCKED LIMIT 1`).Scan(&id, &brand, &member, &rawAlloc)
	if errors.Is(e, pgx.ErrNoRows) {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	o, e := scanConfiguredOrder(tx.QueryRow(ctx, configuredOrderSelect+` WHERE o.id=$1`, id))
	if e != nil {
		return false, e
	}
	var lockedPeriod string
	if e = tx.QueryRow(ctx, `SELECT id::text FROM platform_game_periods WHERE id=$1 FOR UPDATE`, o.PeriodID).Scan(&lockedPeriod); e != nil {
		return false, e
	}
	var drawRaw []byte
	if e = tx.QueryRow(ctx, `SELECT result FROM platform_game_periods WHERE id=$1`, o.PeriodID).Scan(&drawRaw); e != nil {
		return false, e
	}
	var draw rules.Draw
	if len(drawRaw) == 0 || json.Unmarshal(drawRaw, &draw) != nil {
		return false, ErrSnapshot
	}
	engine, e := rules.RestoreConfiguredEngine(o.Snapshot)
	if e != nil {
		return false, ErrSnapshot
	}
	var snapshot rules.ConfiguredSnapshot
	if json.Unmarshal(o.Snapshot, &snapshot) != nil {
		return false, ErrSnapshot
	}
	settlement := rules.DefaultConfiguredSettlementContext(snapshot.Catalog)
	for aggregate, playIDs := range snapshot.Catalog.Aggregates {
		var raw string
		if e = tx.QueryRow(ctx, `SELECT COALESCE(sum((line->>'stake')::numeric),0)::text
			FROM configured_orders o CROSS JOIN LATERAL jsonb_array_elements(o.quote->'lines') line
			WHERE o.period_id=$1 AND line->>'play_id'=ANY($2::text[]) AND o.brand_id=$3 AND o.member_id=$4`, o.PeriodID, playIDs, brand, member).Scan(&raw); e != nil {
			return false, e
		}
		amount, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || amount < 0 {
			return false, ErrSnapshot
		}
		settlement.Aggregates[aggregate] = points.Amount(amount)
	}
	if settlement.PrizeRemaining != nil {
		var raw string
		if e = tx.QueryRow(ctx, `SELECT COALESCE(sum((result->>'prize_points')::numeric),0)::text FROM configured_orders WHERE period_id=$1 AND status='settled' AND brand_id=$2 AND member_id=$3`, o.PeriodID, brand, member).Scan(&raw); e != nil {
			return false, e
		}
		paid, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || paid < 0 || paid > int64(*settlement.PrizeRemaining) {
			return false, ErrSnapshot
		}
		remaining := points.Amount(int64(*settlement.PrizeRemaining) - paid)
		settlement.PrizeRemaining = &remaining
	}
	result, e := engine.SettleWithContext(ctx, o.Quote, draw, settlement)
	if e != nil {
		return false, e
	}
	var alloc []points.Allocation
	if json.Unmarshal(rawAlloc, &alloc) != nil {
		return false, ErrSnapshot
	}
	refunds, losses, e := configuredSettlementAllocations(alloc, o.Quote, result, snapshot.Catalog)
	if e != nil {
		return false, e
	}
	ps := points.Store{DB: s.DB}
	if _, e = ps.LockedSnapshot(ctx, tx, brand, member); e != nil {
		return false, e
	}
	payoutID, refundID, lossID := "", "", ""
	if len(losses) > 0 {
		d, err := points.AllocationDelta(losses, "system_frozen", "")
		if err != nil {
			return false, err
		}
		l, err := ps.Post(ctx, tx, points.Change{BrandID: brand, MemberID: member, EntryType: "bet", ReferenceType: "configured_order", ReferenceID: id, OperationKey: "configured-loss:" + id, Reason: "configured loss from reserved original sources", ActorType: "system", RequestID: "configured:" + id, Delta: d, Allocation: losses})
		if err != nil {
			return false, err
		}
		lossID = l.ID
	}
	if result.PrizePoints > 0 {
		var d points.Balance
		d[1][0] = result.PrizePoints
		l, err := ps.Post(ctx, tx, points.Change{BrandID: brand, MemberID: member, EntryType: "prize", ReferenceType: "configured_order", ReferenceID: id, OperationKey: "configured-prize:" + id, Reason: "configured winnings including stake", ActorType: "system", RequestID: "configured:" + id, Delta: d, Allocation: []points.Allocation{{Source: "winning", State: "available", Points: result.PrizePoints}}})
		if err != nil {
			return false, err
		}
		payoutID = l.ID
	}
	if result.RefundPoints > 0 {
		from := ""
		if engine.FundingMode() == "reserve" {
			from = "system_frozen"
		}
		d, err := points.AllocationDelta(refunds, from, "available")
		if err != nil {
			return false, err
		}
		l, err := ps.Post(ctx, tx, points.Change{BrandID: brand, MemberID: member, EntryType: "refund", ReferenceType: "configured_order", ReferenceID: id, OperationKey: "configured-refund:" + id, Reason: "configured push original sources", ActorType: "system", RequestID: "configured:" + id, Delta: d, Allocation: refunds})
		if err != nil {
			return false, err
		}
		refundID = l.ID
	}
	raw, _ := json.Marshal(result)
	_, e = tx.Exec(ctx, `UPDATE configured_orders SET result=$2,status='settled',settled_at=clock_timestamp(),payout_entry_id=NULLIF($3,'')::uuid,refund_entry_id=NULLIF($4,'')::uuid,loss_entry_id=NULLIF($5,'')::uuid WHERE id=$1`, id, raw, payoutID, refundID, lossID)
	if e != nil {
		return false, e
	}
	_, e = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "system", Action: "configured.settle", ResourceType: "configured_order", ResourceID: id, Reason: "frozen configured rule settlement", RequestID: "configured:" + id, After: result})
	if e != nil {
		return false, e
	}
	return true, tx.Commit(ctx)
}

// Allocate each result against its purchased funding in the original source order.
// Reserve funding consumes actual losses and releases the remaining source amounts.
func configuredSettlementAllocations(alloc []points.Allocation, q rules.ConfiguredQuote, r rules.ConfiguredResult, catalog rules.ConfiguredCatalog) ([]points.Allocation, []points.Allocation, error) {
	if len(q.Lines) != len(r.Lines) {
		return nil, nil, ErrSnapshot
	}
	reserved := catalog.Funding == "reserve"
	plays := map[string]rules.ConfiguredPlay{}
	for _, p := range catalog.Plays {
		plays[p.ID] = p
	}
	remaining := append([]points.Allocation(nil), alloc...)
	releases, losses := map[string]points.Amount{}, map[string]points.Amount{}
	for i, line := range q.Lines {
		funding := line.Stake
		if reserved {
			factor := plays[line.PlayID].RiskMultiplier
			if factor < 1 || int64(line.Stake) > math.MaxInt64/int64(factor) {
				return nil, nil, ErrSnapshot
			}
			funding *= points.Amount(factor)
		}
		refund := r.Lines[i].Refund
		if refund < 0 || refund > funding {
			return nil, nil, ErrSnapshot
		}
		needed, lossNeeded := funding, funding-refund
		for j := range remaining {
			take := remaining[j].Points
			if take > needed {
				take = needed
			}
			loss := take
			if loss > lossNeeded {
				loss = lossNeeded
			}
			releases[remaining[j].Source] += take - loss
			if reserved {
				losses[remaining[j].Source] += loss
			}
			lossNeeded -= loss
			remaining[j].Points -= take
			needed -= take
			if needed == 0 {
				break
			}
		}
		if needed != 0 || lossNeeded != 0 {
			return nil, nil, ErrSnapshot
		}
	}
	refunds, consumed := []points.Allocation{}, []points.Allocation{}
	var refundTotal, lossTotal points.Amount
	state := "available"
	if reserved {
		state = "system_frozen"
	}
	for _, original := range alloc {
		if original.Points < 1 {
			return nil, nil, ErrSnapshot
		}
		if amount := releases[original.Source]; amount > 0 {
			refunds = append(refunds, points.Allocation{Source: original.Source, State: state, Points: amount})
			refundTotal += amount
		}
		if amount := losses[original.Source]; amount > 0 {
			consumed = append(consumed, points.Allocation{Source: original.Source, State: state, Points: amount})
			lossTotal += amount
		}
	}
	for _, a := range remaining {
		if a.Points != 0 {
			return nil, nil, ErrSnapshot
		}
	}
	if refundTotal != r.RefundPoints || reserved && lossTotal != r.ConsumedPoints {
		return nil, nil, ErrSnapshot
	}
	return refunds, consumed, nil
}
