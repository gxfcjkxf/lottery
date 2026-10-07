package betting

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"regexp"
	"strings"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/compliance"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/periodgate"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var keyPattern = regexp.MustCompile(`^[A-Za-z0-9_:.-]{8,128}$`)

// Preserve ErrDenied compatibility while exposing the specified operational
// pause reason to authenticated clients. It never applies to refunds.
var ErrBrandPaused = errors.Join(ErrDenied, errors.New("brand is paused"))

type Period struct {
	ID           string    `json:"id"`
	GameID       string    `json:"game_id"`
	PeriodNo     string    `json:"period_no"`
	Status       string    `json:"status"`
	BetStartAt   time.Time `json:"bet_start_at"`
	BetEndAt     time.Time `json:"bet_end_at"`
	DrawAt       time.Time `json:"draw_at"`
	DrawResultID string    `json:"draw_result_id,omitempty"`
}
type contextRecord struct {
	GameID, GameStatus, PlayStatus, RuleID, Hash string
	Period                                       Period
	Definition                                   rules.Definition
	Policy                                       EffectivePolicy
	Versions                                     PolicyVersions
}
type Quote struct {
	rules.BetQuote
	PeriodID       string          `json:"period_id"`
	PlayID         string          `json:"play_id"`
	RuleVersionID  string          `json:"rule_version_id"`
	DefinitionHash string          `json:"definition_hash"`
	Policy         EffectivePolicy `json:"policy"`
	PolicyVersions PolicyVersions  `json:"policy_versions"`
	Period         Period          `json:"period"`
}

func validIDs(ids ...string) bool {
	for _, id := range ids {
		if !policyUUIDPattern.MatchString(id) {
			return false
		}
	}
	return true
}
func (s Service) lockPeriod(ctx context.Context, tx pgx.Tx, brand, period string) (string, string, Period, error) {
	var p Period
	var game, status string
	if !validIDs(brand, period) {
		return game, status, p, ErrInvalid
	}
	e := tx.QueryRow(ctx, `SELECT game_id::text FROM periods WHERE brand_id=$1 AND id=$2`, brand, period).Scan(&game)
	if errors.Is(e, pgx.ErrNoRows) {
		return game, status, p, ErrNotFound
	}
	if e != nil {
		return game, status, p, e
	}
	e = tx.QueryRow(ctx, `SELECT status FROM games WHERE brand_id=$1 AND id=$2 FOR SHARE`, brand, game).Scan(&status)
	if e != nil {
		return game, status, p, e
	}
	e = tx.QueryRow(ctx, `SELECT id::text,game_id::text,period_no,status,bet_start_at,bet_end_at,draw_at,coalesce(draw_result_id::text,'') FROM periods WHERE brand_id=$1 AND id=$2 FOR SHARE`, brand, period).Scan(&p.ID, &p.GameID, &p.PeriodNo, &p.Status, &p.BetStartAt, &p.BetEndAt, &p.DrawAt, &p.DrawResultID)
	p.BetStartAt = p.BetStartAt.UTC()
	p.BetEndAt = p.BetEndAt.UTC()
	p.DrawAt = p.DrawAt.UTC()
	return game, status, p, e
}
func (s Service) betContext(ctx context.Context, tx pgx.Tx, brand string, in Input) (contextRecord, error) {
	var c contextRecord
	if tx == nil || !validIDs(brand, in.PeriodID, in.PlayID, in.RuleVersionID) {
		return c, ErrInvalid
	}
	var e error
	c.GameID, c.GameStatus, c.Period, e = s.lockPeriod(ctx, tx, brand, in.PeriodID)
	if e != nil {
		return c, e
	}
	e = tx.QueryRow(ctx, `SELECT status,coalesce(active_version_id::text,'') FROM play_definitions WHERE brand_id=$1 AND game_id=$2 AND id=$3 FOR SHARE`, brand, c.GameID, in.PlayID).Scan(&c.PlayStatus, &c.RuleID)
	if errors.Is(e, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	if e != nil {
		return c, e
	}
	if c.GameStatus != "active" || c.PlayStatus != "active" {
		return c, ErrClosed
	}
	if c.RuleID == "" || c.RuleID != strings.ToLower(in.RuleVersionID) {
		return c, ErrVersion
	}
	var raw []byte
	var status string
	e = tx.QueryRow(ctx, `SELECT definition,definition_sha256,status FROM rule_versions WHERE brand_id=$1 AND game_id=$2 AND play_id=$3 AND id=$4`, brand, c.GameID, in.PlayID, c.RuleID).Scan(&raw, &c.Hash, &status)
	if e != nil {
		return c, e
	}
	if status != "active" {
		return c, ErrVersion
	}
	if e = json.Unmarshal(raw, &c.Definition); e != nil {
		return c, ErrInvalid
	}
	canonical, e := json.Marshal(c.Definition)
	if e != nil {
		return c, e
	}
	sum := sha256.Sum256(canonical)
	if hex.EncodeToString(sum[:]) != c.Hash {
		return c, ErrInvalid
	}
	c.Policy, c.Versions, e = s.LockedPolicy(ctx, tx, brand, c.GameID)
	if e != nil {
		return c, e
	}
	if in.PolicyVersions != nil && *in.PolicyVersions != c.Versions {
		return c, ErrVersion
	}
	return c, nil
}
func eligibility(ctx context.Context, tx pgx.Tx, brand string, v identity.Session, normal bool) error {
	if tx == nil {
		return ErrInvalid
	}
	if !validIDs(brand, v.User.ID, v.Member.ID, v.ID) || v.Member.BrandID != brand {
		return ErrDenied
	}
	var ok, sessionOK bool
	var brandStatus string
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM brands b JOIN brand_members m ON m.brand_id=b.id JOIN global_users u ON u.id=m.global_user_id WHERE b.id=$1 AND m.id=$2 AND u.id=$3 AND u.status='active' AND m.terms_accepted=true AND m.status IN ('normal','frozen') AND b.status<>'disabled' AND (NOT $4 OR (m.status='normal' AND b.status='active'))), EXISTS(SELECT 1 FROM sessions s WHERE s.id=$5 AND s.brand_id=$1 AND s.member_id=$2 AND s.user_id=$3 AND s.admin_id IS NULL AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp()), COALESCE((SELECT status FROM brands WHERE id=$1),'')`, brand, v.Member.ID, v.User.ID, normal, v.ID).Scan(&ok, &sessionOK, &brandStatus)
	if e != nil {
		return e
	}
	if !sessionOK {
		return identity.ErrSession
	}
	if !ok {
		if normal && brandStatus == "paused" {
			return ErrBrandPaused
		}
		return ErrDenied
	}
	return nil
}
func checkWindow(ctx context.Context, tx pgx.Tx, p Period) error {
	var now time.Time
	var brand string
	var sequence int64
	if e := tx.QueryRow(ctx, `SELECT clock_timestamp(),brand_id::text,sequence FROM periods WHERE id=$1`, p.ID).Scan(&now, &brand, &sequence); e != nil {
		return e
	}
	if p.Status != "betting" || now.Before(p.BetStartAt) || !now.Before(p.BetEndAt) || p.DrawResultID != "" {
		return ErrClosed
	}
	if e := periodgate.Check(ctx, tx, brand, p.GameID, sequence, p.DrawAt); e != nil {
		if errors.Is(e, periodgate.ErrBlocked) {
			return ErrClosed
		}
		return e
	}
	return nil
}
func quote(c contextRecord, in Input, q rules.BetQuote) Quote {
	return Quote{BetQuote: q, PeriodID: in.PeriodID, PlayID: in.PlayID, RuleVersionID: c.RuleID, DefinitionHash: c.Hash, Policy: c.Policy, PolicyVersions: c.Versions, Period: c.Period}
}
func (s Service) Preview(ctx context.Context, tx pgx.Tx, brand string, v identity.Session, in Input, metadata ...points.Metadata) (Quote, error) {
	var out Quote
	if tx == nil {
		return out, ErrInvalid
	}
	if e := eligibility(ctx, tx, brand, v, true); e != nil {
		return out, e
	}
	c, e := s.betContext(ctx, tx, brand, in)
	if e != nil {
		return out, e
	}
	q, e := rules.PrepareBet(ctx, c.Definition, in.Selection, in.Multiplier)
	if e != nil {
		return out, e
	}
	if q.BetPoints < c.Policy.MinBetPoints || c.Policy.MaxBetPoints != nil && q.BetPoints > *c.Policy.MaxBetPoints {
		return out, ErrLimit
	}
	if e = checkWindow(ctx, tx, c.Period); e != nil {
		return out, e
	}
	if e = checkQuotas(ctx, tx, brand, v.Member.ID, in.PeriodID, c.Policy, q.BetPoints); e != nil {
		return out, e
	}
	// Preview is a non-reserving estimate. Place checks quotas and balance again
	// under the account/quota locks before accepting any financial mutation.
	if len(metadata) > 1 {
		return out, ErrInvalid
	}
	requestID, ip := "", ""
	if len(metadata) == 1 {
		requestID, ip = metadata[0].RequestID, metadata[0].IP
	} else {
		requestID = ids.New()
	}
	if e = compliance.AssessTx(ctx, tx, brand, "bet_preview", compliance.GateSubject{ActorType: "user", ActorID: v.User.ID, MemberID: v.Member.ID, RequestID: requestID, IP: ip}); e != nil {
		return out, e
	}
	return quote(c, in, q), nil
}

const orderFields = `id::text,brand_id::text,global_user_id::text,brand_member_id::text,account_id::text,game_id::text,period_id::text,play_id::text,rule_version_id::text,definition_hash,definition_snapshot,status,version,selection_raw,selection_normalized,expanded_bets,unit_points,combination_count,multiplier,total_points,deduction_allocation,policy_snapshot,brand_policy_version,game_policy_version,debit_entry_id::text,coalesce(refund_entry_id::text,''),client_key,placed_at,cancelled_at,cancel_reason,settlement_calculation_id::text,payout_entry_id::text,prize_points,settled_at`

func scanOrder(row pgx.Row) (o Order, e error) {
	var def, raw, norm, expanded, alloc, policy []byte
	e = row.Scan(&o.ID, &o.BrandID, &o.UserID, &o.MemberID, &o.AccountID, &o.GameID, &o.PeriodID, &o.PlayID, &o.RuleVersionID, &o.DefinitionHash, &def, &o.Status, &o.Version, &raw, &norm, &expanded, &o.UnitPoints, &o.CombinationCount, &o.Multiplier, &o.TotalPoints, &alloc, &policy, &o.PolicyVersions.Brand, &o.PolicyVersions.Game, &o.DebitEntryID, &o.RefundEntryID, &o.ClientKey, &o.PlacedAt, &o.CancelledAt, &o.CancelReason, &o.SettlementCalculationID, &o.PayoutEntryID, &o.PrizePoints, &o.SettledAt)
	if errors.Is(e, pgx.ErrNoRows) {
		return o, ErrNotFound
	}
	if e != nil {
		return o, e
	}
	for _, item := range []struct {
		raw  []byte
		dest any
	}{{def, &o.Definition}, {raw, &o.SelectionRaw}, {norm, &o.SelectionNormalized}, {expanded, &o.Expanded}, {alloc, &o.Allocation}, {policy, &o.Policy}} {
		if e = json.Unmarshal(item.raw, item.dest); e != nil {
			return o, errors.Join(ErrSnapshot, e)
		}
	}
	o.PlacedAt = o.PlacedAt.UTC()
	if o.CancelledAt != nil {
		t := o.CancelledAt.UTC()
		o.CancelledAt = &t
	}
	return
}
func (s Service) Order(ctx context.Context, brand, member, id string) (Order, error) {
	if s.DB == nil || !validIDs(brand, id) || member != "" && !validIDs(member) {
		return Order{}, ErrInvalid
	}
	return scanOrder(s.DB.QueryRow(ctx, `SELECT `+orderFields+` FROM bet_orders WHERE brand_id=$1 AND id=$2 AND ($3::text='' OR brand_member_id=NULLIF($3,'')::uuid)`, brand, id, member))
}
func (s Service) Orders(ctx context.Context, brand, member string, limit, offset int) ([]Order, error) {
	out := []Order{}
	if s.DB == nil || !validIDs(brand) || member != "" && !validIDs(member) || limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		return out, ErrInvalid
	}
	rows, e := s.DB.Query(ctx, `SELECT `+orderFields+` FROM bet_orders WHERE brand_id=$1 AND ($2::text='' OR brand_member_id=NULLIF($2,'')::uuid) ORDER BY placed_at DESC,id DESC LIMIT $3 OFFSET $4`, brand, member, limit, offset)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		o, e := scanOrder(rows)
		if e != nil {
			return out, e
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
func existingOrder(ctx context.Context, tx pgx.Tx, brand, member, key string, in Input) (Order, error) {
	o, e := scanOrder(tx.QueryRow(ctx, `SELECT `+orderFields+` FROM bet_orders WHERE brand_id=$1 AND brand_member_id=$2 AND client_key=$3`, brand, member, key))
	if e != nil {
		return o, e
	}
	a, _ := json.Marshal(in.Selection)
	b, _ := json.Marshal(o.SelectionRaw)
	if o.PeriodID != strings.ToLower(in.PeriodID) || o.PlayID != strings.ToLower(in.PlayID) || o.RuleVersionID != strings.ToLower(in.RuleVersionID) || o.Multiplier != in.Multiplier || string(a) != string(b) || in.PolicyVersions == nil || *in.PolicyVersions != o.PolicyVersions {
		return o, ErrState
	}
	return o, nil
}
func lockQuota(ctx context.Context, tx pgx.Tx, period string, policy EffectivePolicy) error {
	if policy.MaxPeriodPoints == nil {
		return nil
	}
	// Unlimited periods have no hot counter or global mutex. A configured global
	// exposure cap necessarily serializes admissions/refunds for that period.
	_, e := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('lottery-period-quota:'||$1::text,0))`, period)
	return e
}
func checkQuotas(ctx context.Context, tx pgx.Tx, brand, member, period string, p EffectivePolicy, stake points.Amount) error {
	for _, check := range []struct {
		max  *points.Amount
		user bool
	}{{p.MaxPeriodPoints, false}, {p.MaxUserPeriodPoints, true}} {
		if check.max == nil {
			continue
		}
		var total string
		e := tx.QueryRow(ctx, `SELECT coalesce(sum(total_points::numeric),0)::text FROM bet_orders WHERE brand_id=$1 AND period_id=$2 AND refund_entry_id IS NULL AND (NOT $4 OR brand_member_id=$3)`, brand, period, member, check.user).Scan(&total)
		if e != nil {
			return e
		}
		used, ok := new(big.Int).SetString(total, 10)
		if !ok {
			return ErrInvalid
		}
		used.Add(used, big.NewInt(int64(stake)))
		if used.Cmp(big.NewInt(int64(*check.max))) > 0 {
			return ErrLimit
		}
	}
	return nil
}
func appendEvent(ctx context.Context, tx pgx.Tx, o Order, event string) error {
	raw, e := json.Marshal(map[string]any{"order_id": o.ID, "member_id": o.MemberID, "period_id": o.PeriodID, "version": o.Version, "status": o.Status, "points": o.TotalPoints})
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `INSERT INTO outbox_events(id,brand_id,event_type,aggregate_id,payload) VALUES($1,$2,$3,$4,$5)`, ids.New(), o.BrandID, event, o.ID, raw)
	return e
}
func (s Service) Place(ctx context.Context, tx pgx.Tx, brand string, v identity.Session, in Input, key string, meta points.Metadata) (Order, error) {
	var o Order
	if tx == nil || !validIDs(brand, in.PeriodID, in.PlayID, in.RuleVersionID, v.Member.ID, v.User.ID) || !keyPattern.MatchString(key) || in.PolicyVersions == nil {
		return o, ErrInvalid
	}
	if e := eligibility(ctx, tx, brand, v, false); e != nil {
		return o, e
	}
	if o, e := existingOrder(ctx, tx, brand, v.Member.ID, key, in); e == nil {
		return o, nil
	} else if !errors.Is(e, ErrNotFound) {
		return o, e
	}
	if e := eligibility(ctx, tx, brand, v, true); e != nil {
		return o, e
	}
	c, e := s.betContext(ctx, tx, brand, in)
	if e != nil {
		return o, e
	}
	q, e := rules.PrepareBet(ctx, c.Definition, in.Selection, in.Multiplier)
	if e != nil {
		return o, e
	}
	if q.BetPoints < c.Policy.MinBetPoints || c.Policy.MaxBetPoints != nil && q.BetPoints > *c.Policy.MaxBetPoints {
		return o, ErrLimit
	}
	if e = lockQuota(ctx, tx, in.PeriodID, c.Policy); e != nil {
		return o, e
	}
	if _, e = commission.LockBetPolicies(ctx, tx, brand); e != nil {
		return o, e
	}
	if e = lockWithdrawalSnapshotPolicies(ctx, tx, brand, c.GameID); e != nil {
		return o, e
	}
	ps := points.Store{DB: s.DB}
	wallet, e := ps.LockedSnapshot(ctx, tx, brand, v.Member.ID)
	if e != nil {
		return o, e
	}
	if e = eligibility(ctx, tx, brand, v, false); e != nil {
		return o, e
	}
	if cached, e := existingOrder(ctx, tx, brand, v.Member.ID, key, in); e == nil {
		return cached, nil
	} else if !errors.Is(e, ErrNotFound) {
		return o, e
	}
	if e = checkWindow(ctx, tx, c.Period); e != nil {
		return o, e
	}
	// A session may naturally expire while quota/account locks are contended,
	// even though AuthenticateTx protects it against revocation and edits.
	if e = eligibility(ctx, tx, brand, v, true); e != nil {
		return o, e
	}
	if e = checkQuotas(ctx, tx, brand, v.Member.ID, in.PeriodID, c.Policy, q.BetPoints); e != nil {
		return o, e
	}
	if e = compliance.AssessTx(ctx, tx, brand, "bet_place", compliance.GateSubject{ActorType: "user", ActorID: v.User.ID, MemberID: v.Member.ID, RequestID: meta.RequestID, IP: meta.IP}); e != nil {
		return o, e
	}
	alloc, e := wallet.BySource.Allocate(q.BetPoints, "available")
	if e != nil {
		return o, e
	}
	delta, e := points.AllocationDelta(alloc, "available", "")
	if e != nil {
		return o, e
	}
	id := ids.New()
	entry, e := ps.Post(ctx, tx, points.Change{BrandID: brand, MemberID: v.Member.ID, EntryType: "bet", ReferenceType: "bet_order", ReferenceID: id, OperationKey: "bet:" + id, Reason: "user confirmed lottery stake", ActorType: "user", ActorID: v.User.ID, RequestID: meta.RequestID, IP: meta.IP, Delta: delta, Allocation: alloc})
	if e != nil {
		return o, e
	}
	blobs := make([][]byte, 6)
	for i, item := range []any{c.Definition, in.Selection, q.Normalized, q.Expanded, alloc, c.Policy} {
		blobs[i], e = json.Marshal(item)
		if e != nil {
			return o, e
		}
	}
	o, e = scanOrder(tx.QueryRow(ctx, `INSERT INTO bet_orders(id,brand_id,global_user_id,brand_member_id,account_id,game_id,period_id,play_id,rule_version_id,definition_snapshot,definition_hash,selection_raw,selection_normalized,expanded_bets,unit_points,combination_count,multiplier,total_points,deduction_allocation,policy_snapshot,brand_policy_version,game_policy_version,debit_entry_id,client_key)
 SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24 WHERE EXISTS(SELECT 1 FROM periods WHERE id=$7 AND status='betting' AND bet_start_at<=clock_timestamp() AND bet_end_at>clock_timestamp() AND draw_result_id IS NULL) RETURNING `+orderFields, id, brand, v.User.ID, v.Member.ID, wallet.ID, c.GameID, in.PeriodID, in.PlayID, c.RuleID, blobs[0], c.Hash, blobs[1], blobs[2], blobs[3], q.UnitPoints, q.CombinationCount, q.Multiplier, q.BetPoints, blobs[4], blobs[5], c.Versions.Brand, c.Versions.Game, entry.ID, key))
	if errors.Is(e, ErrNotFound) {
		return o, ErrClosed
	}
	var pe *pgconn.PgError
	if errors.As(e, &pe) && pe.Code == "P0001" && pe.Message == "bet outside period window" {
		return o, ErrClosed
	}
	if e != nil {
		return o, e
	}
	if e = appendEvent(ctx, tx, o, "bet.order.placed"); e != nil {
		return o, e
	}
	_, e = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "user", ActorID: v.User.ID, Action: "bet.place", ResourceType: "bet_order", ResourceID: o.ID, Reason: "user confirmed lottery stake", RequestID: meta.RequestID, IP: meta.IP, After: map[string]any{"order_id": o.ID, "period_id": o.PeriodID, "rule_version_id": o.RuleVersionID, "points": o.TotalPoints, "debit_entry_id": entry.ID}})
	return o, e
}
func (s Service) Cancel(ctx context.Context, tx pgx.Tx, brand string, v identity.Session, id string, version int64, reason string, meta points.Metadata) (Order, error) {
	if e := eligibility(ctx, tx, brand, v, false); e != nil {
		return Order{}, e
	}
	return s.cancel(ctx, tx, brand, v.Member.ID, id, version, reason, points.Metadata{ActorType: "user", ActorID: v.User.ID, RequestID: meta.RequestID, IP: meta.IP}, false, &v)
}
func (s Service) CancelAdmin(ctx context.Context, tx pgx.Tx, brand string, a access.Account, id string, version int64, reason string, meta points.Metadata) (Order, error) {
	if a.SuperAdmin || !access.Authorize(a, "bet", "cancel", access.ScopeBrand, brand) {
		return Order{}, ErrDenied
	}
	meta.ActorType = "admin"
	meta.ActorID = a.ID
	return s.cancel(ctx, tx, brand, "", id, version, reason, meta, true, nil)
}
func (s Service) cancel(ctx context.Context, tx pgx.Tx, brand, member, id string, version int64, reason string, meta points.Metadata, admin bool, session *identity.Session) (Order, error) {
	var o Order
	if tx == nil || !validIDs(brand, id) || member != "" && !validIDs(member) || !validPolicyReason(reason) {
		return o, ErrInvalid
	}
	// Resolve immutable identity without locking; lock game -> period -> quota ->
	// account -> order on every admission/refund path to avoid lock upgrades.
	o, e := scanOrder(tx.QueryRow(ctx, `SELECT `+orderFields+` FROM bet_orders WHERE brand_id=$1 AND id=$2 AND ($3::text='' OR brand_member_id=NULLIF($3,'')::uuid)`, brand, id, member))
	if e != nil {
		return o, e
	}
	_, _, p, e := s.lockPeriod(ctx, tx, brand, o.PeriodID)
	if e != nil {
		return o, e
	}
	settlement, e := lockSettlementJob(ctx, tx, brand, o.PeriodID)
	if e != nil {
		return o, e
	}
	current, _, e := s.LockedPolicy(ctx, tx, brand, o.GameID)
	if e != nil {
		return o, e
	}
	if e = lockQuota(ctx, tx, o.PeriodID, current); e != nil {
		return o, e
	}
	ps := points.Store{DB: s.DB}
	if _, e = ps.LockedSnapshot(ctx, tx, brand, o.MemberID); e != nil {
		return o, e
	}
	o, e = scanOrder(tx.QueryRow(ctx, `SELECT `+orderFields+` FROM bet_orders WHERE brand_id=$1 AND id=$2 FOR UPDATE`, brand, id))
	if e != nil {
		return o, e
	}
	if o.Version != version || version == math.MaxInt64 {
		return o, ErrVersion
	}
	if o.Status != "placed" && !(admin && o.Status == "abnormal") {
		return o, ErrState
	}
	if !admin {
		if session == nil {
			return o, ErrDenied
		}
		if e = eligibility(ctx, tx, brand, *session, false); e != nil {
			return o, e
		}
		var now time.Time
		if e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); e != nil {
			return o, e
		}
		if !o.Policy.UserCancelAllowed {
			return o, ErrDenied
		}
		if !now.Before(p.DrawAt) || p.DrawResultID != "" || p.Status != "betting" && p.Status != "closed" && p.Status != "waiting_draw" {
			return o, ErrClosed
		}
	}
	o, e = s.refundLocked(ctx, tx, o, "bet_cancelled", reason, meta)
	if e == nil {
		e = excludeSettlementTarget(ctx, tx, settlement, o.ID)
	}
	return o, e
}

// Caller holds game/period, account and order locks. Both cancellation paths
// share the exact original debit reversal and immutable refund evidence.
func (s Service) refundLocked(ctx context.Context, tx pgx.Tx, o Order, status, reason string, meta points.Metadata) (Order, error) {
	if (o.Status != "placed" && o.Status != "abnormal") || (status != "bet_cancelled" && status != "judged_cancelled") {
		return o, ErrState
	}
	if o.Version == math.MaxInt64 {
		return o, ErrVersion
	}
	beforeStatus, version := o.Status, o.Version
	brand := o.BrandID
	ps := points.Store{DB: s.DB}
	original, e := ps.Entry(ctx, tx, brand, o.MemberID, o.DebitEntryID)
	if e != nil {
		return o, e
	}
	delta, e := points.Negate(original.Delta)
	if e != nil {
		return o, e
	}
	refund, e := ps.Post(ctx, tx, points.Change{BrandID: brand, MemberID: o.MemberID, EntryType: "refund", ReferenceType: "bet_order", ReferenceID: o.ID, OperationKey: "bet-refund:" + o.ID, Reason: strings.TrimSpace(reason), ActorType: meta.ActorType, ActorID: meta.ActorID, RequestID: meta.RequestID, IP: meta.IP, Delta: delta, Allocation: original.Allocation, ReversalOf: original.ID})
	if e != nil {
		return o, e
	}
	o, e = scanOrder(tx.QueryRow(ctx, `UPDATE bet_orders SET status=$5,version=version+1,refund_entry_id=$3,cancelled_at=clock_timestamp(),cancel_reason=$4 WHERE brand_id=$1 AND id=$2 RETURNING `+orderFields, brand, o.ID, refund.ID, strings.TrimSpace(reason), status))
	if e != nil {
		return o, e
	}
	event, action := "bet.order.cancelled", "bet.cancel"
	if status == "judged_cancelled" {
		event, action = "bet.order.judged_cancelled", "bet.judge_cancel"
	}
	if e = appendEvent(ctx, tx, o, event); e != nil {
		return o, e
	}
	_, e = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: meta.ActorType, ActorID: meta.ActorID, Action: action, ResourceType: "bet_order", ResourceID: o.ID, Reason: strings.TrimSpace(reason), RequestID: meta.RequestID, IP: meta.IP, Before: map[string]any{"version": version, "status": beforeStatus}, After: map[string]any{"version": o.Version, "status": o.Status, "refund_entry_id": refund.ID}})
	return o, e
}
