package betting

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

type SettlementPolicy struct {
	BrandID    string    `json:"brand_id"`
	Version    int64     `json:"version"`
	Mode       *string   `json:"mode"`
	UpdatedAt  time.Time `json:"updated_at"`
	AuditLogID string    `json:"audit_log_id,omitempty"`
}
type SettlementPolicyInput struct {
	Version int64   `json:"version"`
	Mode    *string `json:"mode"`
	Reason  string  `json:"reason"`
}
type SettlementStartInput struct {
	Version       int64  `json:"version"`
	PolicyVersion int64  `json:"policy_version"`
	DrawResultID  string `json:"draw_result_id"`
	Reason        string `json:"reason"`
}
type SettlementActionInput struct {
	Version int64  `json:"version"`
	Reason  string `json:"reason"`
}
type PeriodSettlementContext struct {
	BrandID       string  `json:"brand_id"`
	GameID        string  `json:"game_id"`
	PeriodID      string  `json:"period_id"`
	PeriodVersion int64   `json:"period_version"`
	PeriodStatus  string  `json:"period_status"`
	DrawResultID  *string `json:"draw_result_id"`
	PolicyVersion int64   `json:"policy_version"`
	Mode          *string `json:"mode"`
	CanStart      bool    `json:"can_start"`
}
type SettlementJob struct {
	ID            string     `json:"id"`
	BrandID       string     `json:"brand_id"`
	GameID        string     `json:"game_id"`
	PeriodID      string     `json:"period_id"`
	DrawResultID  string     `json:"draw_result_id"`
	PeriodVersion int64      `json:"period_version"`
	PolicyVersion int64      `json:"policy_version"`
	Mode          string     `json:"mode"`
	State         string     `json:"state"`
	Version       int64      `json:"version"`
	TargetCount   int64      `json:"target_count"`
	CreatedBy     string     `json:"created_by"`
	ApprovedBy    *string    `json:"approved_by"`
	Reason        string     `json:"reason"`
	CreatedAt     time.Time  `json:"created_at"`
	CompletedAt   *time.Time `json:"completed_at"`
	LastErrorCode *string    `json:"last_error_code"`
	PendingCount  int64      `json:"pending_count"`
	ReadyCount    int64      `json:"ready_count"`
	PaidCount     int64      `json:"paid_count"`
	ExcludedCount int64      `json:"excluded_count"`
	FailedCount   int64      `json:"failed_count"`
	PrizePoints   string     `json:"prize_points"`
	PaidPoints    string     `json:"paid_points"`
	CanRetry      bool       `json:"can_retry"`
	ResumeState   *string    `json:"-"`
}
type SettlementTarget struct {
	OrderID       string         `json:"order_id"`
	MemberID      string         `json:"member_id"`
	State         string         `json:"state"`
	Version       int64          `json:"version"`
	CalculationID *string        `json:"calculation_id"`
	OrderVersion  int64          `json:"order_version"`
	OrderStatus   string         `json:"order_status"`
	Won           *bool          `json:"won"`
	PrizePoints   *points.Amount `json:"prize_points"`
	PayoutEntryID *string        `json:"payout_entry_id"`
	ErrorCode     *string        `json:"error_code"`
}
type SettlementTargets struct {
	BrandID string             `json:"brand_id"`
	JobID   string             `json:"job_id"`
	Items   []SettlementTarget `json:"items"`
	Limit   int                `json:"limit"`
	Offset  int                `json:"offset"`
	HasMore bool               `json:"has_more"`
}

const settlementJobFields = `j.id::text,j.brand_id::text,j.game_id::text,j.period_id::text,j.draw_result_id::text,j.period_version,j.policy_version,j.mode,j.state,j.version,j.target_count,j.created_by::text,j.approved_by::text,j.reason,j.created_at,j.completed_at,j.last_error_code,j.resume_state`
const settlementJobCounts = `,(SELECT count(*) FROM settlement_targets t WHERE t.job_id=j.id AND t.state='pending'),(SELECT count(*) FROM settlement_targets t WHERE t.job_id=j.id AND t.state='ready'),(SELECT count(*) FROM settlement_targets t WHERE t.job_id=j.id AND t.state='paid'),(SELECT count(*) FROM settlement_targets t WHERE t.job_id=j.id AND t.state='excluded'),(SELECT count(*) FROM settlement_targets t WHERE t.job_id=j.id AND t.state='failed'),coalesce((SELECT sum(c.prize_points)::text FROM settlement_targets t JOIN settlement_calculations c ON c.id=t.calculation_id WHERE t.job_id=j.id AND t.state IN ('ready','paid','failed')),'0'),coalesce((SELECT sum(c.prize_points)::text FROM settlement_targets t JOIN settlement_calculations c ON c.id=t.calculation_id WHERE t.job_id=j.id AND t.state='paid'),'0')`

func scanSettlementJob(row pgx.Row, counts bool) (SettlementJob, error) {
	var j SettlementJob
	dest := []any{&j.ID, &j.BrandID, &j.GameID, &j.PeriodID, &j.DrawResultID, &j.PeriodVersion, &j.PolicyVersion, &j.Mode, &j.State, &j.Version, &j.TargetCount, &j.CreatedBy, &j.ApprovedBy, &j.Reason, &j.CreatedAt, &j.CompletedAt, &j.LastErrorCode, &j.ResumeState}
	if counts {
		dest = append(dest, &j.PendingCount, &j.ReadyCount, &j.PaidCount, &j.ExcludedCount, &j.FailedCount, &j.PrizePoints, &j.PaidPoints)
	}
	e := row.Scan(dest...)
	if errors.Is(e, pgx.ErrNoRows) {
		e = ErrNotFound
	}
	j.CanRetry = j.State == "failed"
	return j, e
}
func (s Service) SettlementPolicy(ctx context.Context, brand string) (SettlementPolicy, error) {
	var p SettlementPolicy
	if !validIDs(brand) {
		return p, ErrInvalid
	}
	e := s.DB.QueryRow(ctx, `SELECT brand_id::text,version,mode,updated_at FROM brand_settlement_policies WHERE brand_id=$1`, brand).Scan(&p.BrandID, &p.Version, &p.Mode, &p.UpdatedAt)
	if errors.Is(e, pgx.ErrNoRows) {
		e = ErrNotFound
	}
	return p, e
}
func (s Service) SaveSettlementPolicy(ctx context.Context, tx pgx.Tx, brand string, a access.Account, in SettlementPolicyInput, meta points.Metadata) (SettlementPolicy, error) {
	var p SettlementPolicy
	if tx == nil || !validIDs(brand, a.ID) || in.Version < 1 || !validPolicyReason(in.Reason) || in.Mode != nil && *in.Mode != "automatic" && *in.Mode != "manual" {
		return p, ErrInvalid
	}
	if a.SuperAdmin || !access.Authorize(a, "settlement_policy", "write", access.ScopeBrand, brand) {
		return p, ErrDenied
	}
	e := tx.QueryRow(ctx, `SELECT brand_id::text,version,mode,updated_at FROM brand_settlement_policies WHERE brand_id=$1 FOR UPDATE`, brand).Scan(&p.BrandID, &p.Version, &p.Mode, &p.UpdatedAt)
	if errors.Is(e, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	if e != nil {
		return p, e
	}
	if in.Version != p.Version || p.Version == math.MaxInt64 {
		return p, ErrVersion
	}
	before := p
	p.Version++
	p.Mode = in.Mode
	if e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&p.UpdatedAt); e != nil {
		return p, e
	}
	p.AuditLogID, e = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: a.ID, Action: "settlement_policy.write", ResourceType: "brand", ResourceID: brand, Reason: strings.TrimSpace(in.Reason), RequestID: meta.RequestID, IP: meta.IP, Before: before, After: p})
	if e != nil {
		return p, e
	}
	_, e = tx.Exec(ctx, `INSERT INTO settlement_policy_history(brand_id,version,mode,changed_by,reason,audit_log_id) VALUES($1,$2,$3,$4,$5,$6)`, brand, p.Version, p.Mode, a.ID, strings.TrimSpace(in.Reason), p.AuditLogID)
	if e != nil {
		return p, e
	}
	e = tx.QueryRow(ctx, `UPDATE brand_settlement_policies SET version=$2,mode=$3,updated_at=$4 WHERE brand_id=$1 RETURNING updated_at`, brand, p.Version, p.Mode, p.UpdatedAt).Scan(&p.UpdatedAt)
	return p, e
}
func (s Service) PeriodSettlementContext(ctx context.Context, brand, period string) (PeriodSettlementContext, error) {
	var c PeriodSettlementContext
	if !validIDs(brand, period) {
		return c, ErrInvalid
	}
	e := s.DB.QueryRow(ctx, `SELECT p.brand_id::text,p.game_id::text,p.id::text,p.version,p.status,p.draw_result_id::text,b.version,b.mode FROM periods p JOIN brand_settlement_policies b ON b.brand_id=p.brand_id WHERE p.brand_id=$1 AND p.id=$2`, brand, period).Scan(&c.BrandID, &c.GameID, &c.PeriodID, &c.PeriodVersion, &c.PeriodStatus, &c.DrawResultID, &c.PolicyVersion, &c.Mode)
	if errors.Is(e, pgx.ErrNoRows) {
		e = ErrNotFound
	}
	c.CanStart = c.PeriodStatus == "drawn" && c.DrawResultID != nil && c.Mode != nil
	return c, e
}
func (s Service) SettlementJob(ctx context.Context, brand, id string) (SettlementJob, error) {
	if !validIDs(brand, id) {
		return SettlementJob{}, ErrInvalid
	}
	return scanSettlementJob(s.DB.QueryRow(ctx, `SELECT `+settlementJobFields+settlementJobCounts+` FROM settlement_jobs j WHERE brand_id=$1 AND id=$2`, brand, id), true)
}
func (s Service) PeriodSettlementJob(ctx context.Context, brand, period string) (*SettlementJob, error) {
	if _, e := s.PeriodSettlementContext(ctx, brand, period); e != nil {
		return nil, e
	}
	j, e := scanSettlementJob(s.DB.QueryRow(ctx, `SELECT `+settlementJobFields+settlementJobCounts+` FROM settlement_jobs j WHERE brand_id=$1 AND period_id=$2`, brand, period), true)
	if errors.Is(e, ErrNotFound) {
		return nil, nil
	}
	return &j, e
}
func (s Service) SettlementTargets(ctx context.Context, brand, id string, limit, offset int) (SettlementTargets, error) {
	out := SettlementTargets{BrandID: brand, JobID: id, Items: []SettlementTarget{}, Limit: limit, Offset: offset}
	if limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		return out, ErrInvalid
	}
	if _, e := s.SettlementJob(ctx, brand, id); e != nil {
		return out, e
	}
	rows, e := s.DB.Query(ctx, `SELECT t.order_id::text,o.brand_member_id::text,t.state,t.version,t.calculation_id::text,o.version,o.status,c.won,c.prize_points,t.payout_entry_id::text,t.error_code FROM settlement_targets t JOIN bet_orders o ON o.id=t.order_id LEFT JOIN settlement_calculations c ON c.id=t.calculation_id WHERE t.brand_id=$1 AND t.job_id=$2 ORDER BY t.order_id LIMIT $3 OFFSET $4`, brand, id, limit+1, offset)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var t SettlementTarget
		if e = rows.Scan(&t.OrderID, &t.MemberID, &t.State, &t.Version, &t.CalculationID, &t.OrderVersion, &t.OrderStatus, &t.Won, &t.PrizePoints, &t.PayoutEntryID, &t.ErrorCode); e != nil {
			return out, e
		}
		out.Items = append(out.Items, t)
	}
	if len(out.Items) > limit {
		out.HasMore = true
		out.Items = out.Items[:limit]
	}
	return out, rows.Err()
}
func (s Service) StartSettlement(ctx context.Context, tx pgx.Tx, brand string, a access.Account, period string, in SettlementStartInput, meta points.Metadata) (SettlementJob, error) {
	var out SettlementJob
	if tx == nil || !validIDs(brand, period, a.ID, in.DrawResultID) || in.Version < 1 || in.PolicyVersion < 1 || !validPolicyReason(in.Reason) {
		return out, ErrInvalid
	}
	if a.SuperAdmin || !access.Authorize(a, "settlement", "run", access.ScopeBrand, brand) {
		return out, ErrDenied
	}
	var game, status, result string
	var v int64
	e := tx.QueryRow(ctx, `SELECT game_id::text FROM periods WHERE brand_id=$1 AND id=$2`, brand, period).Scan(&game)
	if errors.Is(e, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if e != nil {
		return out, e
	}
	if e = tx.QueryRow(ctx, `SELECT id::text FROM games WHERE brand_id=$1 AND id=$2 FOR UPDATE`, brand, game).Scan(&game); e != nil {
		return out, e
	}
	if e = tx.QueryRow(ctx, `SELECT status,version,coalesce(draw_result_id::text,'') FROM periods WHERE brand_id=$1 AND id=$2 FOR UPDATE`, brand, period).Scan(&status, &v, &result); e != nil {
		return out, e
	}
	var pv int64
	var mode *string
	if e = tx.QueryRow(ctx, `SELECT version,mode FROM brand_settlement_policies WHERE brand_id=$1 FOR SHARE`, brand).Scan(&pv, &mode); e != nil {
		return out, e
	}
	if v != in.Version || pv != in.PolicyVersion || result != in.DrawResultID || v == math.MaxInt64 {
		return out, ErrVersion
	}
	if status != "drawn" || mode == nil {
		return out, ErrState
	}
	var total int64
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM bet_orders WHERE brand_id=$1 AND period_id=$2`, brand, period).Scan(&total); e != nil {
		return out, e
	}
	id := ids.New()
	_, e = tx.Exec(ctx, `INSERT INTO settlement_jobs(id,brand_id,game_id,period_id,draw_result_id,period_version,policy_version,mode,target_count,created_by,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, id, brand, game, period, result, v+1, pv, *mode, total, a.ID, strings.TrimSpace(in.Reason))
	if e != nil {
		return out, e
	}
	_, e = tx.Exec(ctx, `INSERT INTO settlement_targets(brand_id,job_id,period_id,order_id,state) SELECT brand_id,$1,period_id,id,CASE WHEN status='placed' THEN 'pending' ELSE 'excluded' END FROM bet_orders WHERE brand_id=$2 AND period_id=$3`, id, brand, period)
	if e != nil {
		return out, e
	}
	_, e = tx.Exec(ctx, `UPDATE periods SET status='settling',version=version+1,state_reason=$3 WHERE brand_id=$1 AND id=$2`, brand, period, strings.TrimSpace(in.Reason))
	if e != nil {
		return out, e
	}
	out, e = scanSettlementJob(tx.QueryRow(ctx, `SELECT `+settlementJobFields+settlementJobCounts+` FROM settlement_jobs j WHERE id=$1`, id), true)
	if e != nil {
		return out, e
	}
	if e = settlementJobEvent(ctx, tx, out, "settlement.started"); e != nil {
		return out, e
	}
	_, e = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: a.ID, Action: "settlement.run", ResourceType: "settlement_job", ResourceID: id, Reason: out.Reason, RequestID: meta.RequestID, IP: meta.IP, After: out})
	return out, e
}
func lockSettlementJob(ctx context.Context, tx pgx.Tx, brand, period string) (*SettlementJob, error) {
	j, e := scanSettlementJob(tx.QueryRow(ctx, `SELECT `+settlementJobFields+` FROM settlement_jobs j WHERE brand_id=$1 AND period_id=$2 FOR UPDATE`, brand, period), false)
	if errors.Is(e, ErrNotFound) {
		return nil, nil
	}
	return &j, e
}

// Called after the job lock and before wallet/order locks. A paid order cannot
// enter this path; unpaid calculations remain immutable but become excluded.
func excludeSettlementTarget(ctx context.Context, tx pgx.Tx, job *SettlementJob, order string) error {
	if job == nil {
		return nil
	}
	tag, e := tx.Exec(ctx, `UPDATE settlement_targets SET state='excluded',version=version+1,error_code=NULL WHERE job_id=$1 AND order_id=$2 AND state IN ('pending','ready','failed')`, job.ID, order)
	if e != nil || tag.RowsAffected() == 0 {
		return e
	}
	_, e = tx.Exec(ctx, `UPDATE settlement_jobs SET version=version+1 WHERE id=$1`, job.ID)
	return e
}
func (s Service) ActOnSettlement(ctx context.Context, tx pgx.Tx, brand string, a access.Account, id, action string, in SettlementActionInput, meta points.Metadata) (SettlementJob, error) {
	var j SettlementJob
	if tx == nil || !validIDs(brand, id, a.ID) || in.Version < 1 || !validPolicyReason(in.Reason) || (action != "approve" && action != "retry") {
		return j, ErrInvalid
	}
	if a.SuperAdmin || !access.Authorize(a, "settlement", action, access.ScopeBrand, brand) {
		return j, ErrDenied
	}
	var period string
	e := tx.QueryRow(ctx, `SELECT period_id::text FROM settlement_jobs WHERE brand_id=$1 AND id=$2`, brand, id).Scan(&period)
	if errors.Is(e, pgx.ErrNoRows) {
		return j, ErrNotFound
	}
	if e != nil {
		return j, e
	}
	if _, _, _, e = s.lockPeriod(ctx, tx, brand, period); e != nil {
		return j, e
	}
	locked, e := lockSettlementJob(ctx, tx, brand, period)
	if e != nil {
		return j, e
	}
	if locked == nil {
		return j, ErrNotFound
	}
	j = *locked
	if j.Version != in.Version || j.Version == math.MaxInt64 {
		return j, ErrVersion
	}
	if action == "approve" {
		if j.State != "awaiting_approval" || j.Mode != "manual" {
			return j, ErrState
		}
		_, e = tx.Exec(ctx, `UPDATE settlement_jobs SET state='paying',approved_by=$2,version=version+1 WHERE id=$1`, id, a.ID)
	} else {
		if j.State != "failed" || j.ResumeState == nil {
			return j, ErrState
		}
		state := "pending"
		if *j.ResumeState == "paying" {
			state = "ready"
		}
		_, e = tx.Exec(ctx, `UPDATE settlement_targets SET state=$2,version=version+1,error_code=NULL WHERE job_id=$1 AND state='failed'`, id, state)
		if e != nil {
			return j, e
		}
		_, e = tx.Exec(ctx, `UPDATE settlement_jobs SET state=resume_state,resume_state=NULL,last_error_code=NULL,version=version+1 WHERE id=$1`, id)
	}
	if e != nil {
		return j, e
	}
	j, e = scanSettlementJob(tx.QueryRow(ctx, `SELECT `+settlementJobFields+settlementJobCounts+` FROM settlement_jobs j WHERE id=$1`, id), true)
	if e != nil {
		return j, e
	}
	if e = settlementJobEvent(ctx, tx, j, "settlement."+action); e != nil {
		return j, e
	}
	_, e = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: a.ID, Action: "settlement." + action, ResourceType: "settlement_job", ResourceID: id, Reason: strings.TrimSpace(in.Reason), RequestID: meta.RequestID, IP: meta.IP, After: j})
	return j, e
}
func settlementJobEvent(ctx context.Context, tx pgx.Tx, j SettlementJob, event string) error {
	_, e := tx.Exec(ctx, `INSERT INTO outbox_events(id,brand_id,event_type,aggregate_id,payload) VALUES($1,$2,$3,$4::uuid,jsonb_build_object('job_id',$4::uuid::text,'period_id',$5::text,'state',$6::text,'version',$7::bigint))`, ids.New(), j.BrandID, event, j.ID, j.PeriodID, j.State, j.Version)
	return e
}
