package betting

import (
	"context"
	"encoding/json"
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

type CancelPeriodInput struct {
	Version int64  `json:"version"`
	Mode    string `json:"mode"`
	Cause   string `json:"cause"`
	Reason  string `json:"reason"`
}
type Cancellation struct {
	ID                   string     `json:"id"`
	BrandID              string     `json:"brand_id"`
	GameID               string     `json:"game_id"`
	PeriodID             string     `json:"period_id"`
	PeriodVersion        int64      `json:"period_version"`
	Mode                 string     `json:"mode"`
	Cause                string     `json:"cause"`
	State                string     `json:"state"`
	Version              int64      `json:"version"`
	Reason               string     `json:"reason"`
	CreatedBy            string     `json:"created_by"`
	CreatedAt            time.Time  `json:"created_at"`
	CompletedAt          *time.Time `json:"completed_at"`
	LastErrorCode        string     `json:"last_error_code"`
	TotalCount           int64      `json:"total_count"`
	PendingCount         int64      `json:"pending_count"`
	RefundedCount        int64      `json:"refunded_count"`
	AlreadyRefundedCount int64      `json:"already_refunded_count"`
	FailedCount          int64      `json:"failed_count"`
}

const cancellationFields = `c.id::text,c.brand_id::text,c.game_id::text,c.period_id::text,c.period_version,c.mode,c.cause,c.state,c.version,c.reason,c.created_by::text,c.created_at,c.completed_at,c.last_error_code,c.target_count`
const cancellationCounts = `,(SELECT count(*) FROM period_cancellation_targets t WHERE t.cancellation_id=c.id AND t.state='pending'),(SELECT count(*) FROM period_cancellation_targets t WHERE t.cancellation_id=c.id AND t.state='refunded'),(SELECT count(*) FROM period_cancellation_targets t WHERE t.cancellation_id=c.id AND t.state='already_refunded'),(SELECT count(*) FROM period_cancellation_targets t WHERE t.cancellation_id=c.id AND t.state='failed')`

func stringsWithoutAlias(fields string) string { return strings.ReplaceAll(fields, "c.", "") }
func scanCancellation(row pgx.Row, counts bool) (Cancellation, error) {
	var c Cancellation
	dest := []any{&c.ID, &c.BrandID, &c.GameID, &c.PeriodID, &c.PeriodVersion, &c.Mode, &c.Cause, &c.State, &c.Version, &c.Reason, &c.CreatedBy, &c.CreatedAt, &c.CompletedAt, &c.LastErrorCode, &c.TotalCount}
	if counts {
		dest = append(dest, &c.PendingCount, &c.RefundedCount, &c.AlreadyRefundedCount, &c.FailedCount)
	}
	err := row.Scan(dest...)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	c.CreatedAt = c.CreatedAt.UTC()
	if c.CompletedAt != nil {
		v := c.CompletedAt.UTC()
		c.CompletedAt = &v
	}
	return c, err
}
func (s Service) PeriodCancellation(ctx context.Context, brand, period string) (*Cancellation, error) {
	if !validIDs(brand, period) {
		return nil, ErrInvalid
	}
	var exists bool
	if err := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM periods WHERE brand_id=$1 AND id=$2)`, brand, period).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}
	out, err := scanCancellation(s.DB.QueryRow(ctx, `SELECT `+cancellationFields+cancellationCounts+` FROM period_cancellations c WHERE c.brand_id=$1 AND c.period_id=$2`, brand, period), true)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	return &out, err
}
func (s Service) CancelPeriod(ctx context.Context, tx pgx.Tx, brand string, a access.Account, period string, in CancelPeriodInput, meta points.Metadata) (Cancellation, error) {
	var out Cancellation
	if tx == nil || !validIDs(brand, period, a.ID) || !validPolicyReason(in.Reason) || in.Version < 1 {
		return out, ErrInvalid
	}
	if a.SuperAdmin || !access.Authorize(a, "period", "cancel", access.ScopeBrand, brand) {
		return out, ErrDenied
	}
	if !(in.Mode == "bet_cancelled" && in.Cause == "operator_cancel" || in.Mode == "judged_cancelled" && (in.Cause == "no_result" || in.Cause == "invalid_result")) {
		return out, ErrInvalid
	}
	var game, status, result string
	var version int64
	err := tx.QueryRow(ctx, `SELECT game_id::text FROM periods WHERE brand_id=$1 AND id=$2`, brand, period).Scan(&game)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, err
	}
	// Serialize cancellation against admissions, calendar clock and result commits.
	// Targets and the closed period are durable before any wallet is touched.
	if err = tx.QueryRow(ctx, `SELECT id::text FROM games WHERE brand_id=$1 AND id=$2 FOR UPDATE`, brand, game).Scan(&game); err != nil {
		return out, err
	}
	if err = tx.QueryRow(ctx, `SELECT status,version,coalesce(draw_result_id::text,'') FROM periods WHERE brand_id=$1 AND id=$2 FOR UPDATE`, brand, period).Scan(&status, &version, &result); err != nil {
		return out, err
	}
	if version != in.Version || version == math.MaxInt64 {
		return out, ErrVersion
	}
	if status == "settling" || status == "settled" || status == "bet_cancelled" || status == "judged_cancelled" {
		return out, ErrState
	}
	if in.Mode == "bet_cancelled" && status != "betting" || in.Cause == "no_result" && result != "" || status == "drawn" && in.Cause != "invalid_result" {
		return out, ErrState
	}
	var unsupported bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM bet_orders WHERE brand_id=$1 AND period_id=$2 AND status NOT IN ('placed','abnormal','bet_cancelled','judged_cancelled'))`, brand, period).Scan(&unsupported); err != nil {
		return out, err
	}
	if unsupported {
		return out, ErrState
	}
	var total int64
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM bet_orders WHERE brand_id=$1 AND period_id=$2 AND status IN ('placed','abnormal')`, brand, period).Scan(&total); err != nil {
		return out, err
	}
	id := ids.New()
	state := "processing"
	var created time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&created); err != nil {
		return out, err
	}
	var completed *time.Time
	if total == 0 {
		state = "completed"
		completed = &created
	}
	out, err = scanCancellation(tx.QueryRow(ctx, `INSERT INTO period_cancellations(id,brand_id,game_id,period_id,period_version,draw_result_id,mode,cause,state,target_count,reason,created_by,completed_at,created_at) VALUES($1,$2,$3,$4,$5,NULLIF($6,'')::uuid,$7,$8,$9,$10,$11,$12,$13,$14) RETURNING `+strings.ReplaceAll(cancellationFields, "c.", ""), id, brand, game, period, version+1, result, in.Mode, in.Cause, state, total, strings.TrimSpace(in.Reason), a.ID, completed, created), false)
	if err != nil {
		return out, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO period_cancellation_targets(cancellation_id,brand_id,period_id,order_id) SELECT $1,brand_id,period_id,id FROM bet_orders WHERE brand_id=$2 AND period_id=$3 AND status IN ('placed','abnormal')`, id, brand, period); err != nil {
		return out, err
	}
	if _, err = tx.Exec(ctx, `UPDATE periods SET status=$3,version=version+1,state_reason=$4,draw_claim_token=NULL,draw_claim_until=NULL,draw_next_poll_at=NULL WHERE brand_id=$1 AND id=$2`, brand, period, in.Mode, strings.TrimSpace(in.Reason)); err != nil {
		return out, err
	}
	out.PendingCount = total
	if err = cancellationEvent(ctx, tx, out, "period.cancellation.started"); err != nil {
		return out, err
	}
	if out.State == "completed" {
		if err = cancellationEvent(ctx, tx, out, "period.cancellation.completed"); err != nil {
			return out, err
		}
	}
	_, err = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: a.ID, Action: "period.cancel", ResourceType: "period_cancellation", ResourceID: id, Reason: out.Reason, RequestID: meta.RequestID, IP: meta.IP, Before: map[string]any{"period_id": period, "version": version, "status": status, "draw_result_id": result}, After: out})
	return out, err
}
func cancellationEvent(ctx context.Context, tx pgx.Tx, c Cancellation, event string) error {
	raw, err := json.Marshal(c)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO outbox_events(id,brand_id,event_type,aggregate_id,payload) VALUES($1,$2,$3,$4,$5)`, ids.New(), c.BrandID, event, c.ID, raw)
	return err
}
func (s Service) RetryPeriodCancellation(ctx context.Context, tx pgx.Tx, brand string, a access.Account, period string, version int64, reason string, meta points.Metadata) (Cancellation, error) {
	var out Cancellation
	if tx == nil || !validIDs(brand, period, a.ID) || version < 1 || !validPolicyReason(reason) {
		return out, ErrInvalid
	}
	if a.SuperAdmin || !access.Authorize(a, "period", "cancel_retry", access.ScopeBrand, brand) {
		return out, ErrDenied
	}
	if _, _, _, err := s.lockPeriod(ctx, tx, brand, period); err != nil {
		return out, err
	}
	out, err := scanCancellation(tx.QueryRow(ctx, `SELECT `+cancellationFields+` FROM period_cancellations c WHERE brand_id=$1 AND period_id=$2 FOR UPDATE`, brand, period), false)
	if err != nil {
		return out, err
	}
	if out.Version != version || version == math.MaxInt64 {
		return out, ErrVersion
	}
	if out.State != "failed" {
		return out, ErrState
	}
	if _, err = tx.Exec(ctx, `UPDATE period_cancellations SET state='processing',version=version+1,last_error_code='' WHERE id=$1`, out.ID); err != nil {
		return out, err
	}
	if _, err = tx.Exec(ctx, `UPDATE period_cancellation_targets SET state='pending',version=version+1,error_code='' WHERE cancellation_id=$1 AND state='failed'`, out.ID); err != nil {
		return out, err
	}
	out, err = scanCancellation(tx.QueryRow(ctx, `SELECT `+cancellationFields+cancellationCounts+` FROM period_cancellations c WHERE c.id=$1`, out.ID), true)
	if err != nil {
		return out, err
	}
	if err = cancellationEvent(ctx, tx, out, "period.cancellation.retry_requested"); err != nil {
		return out, err
	}
	_, err = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: a.ID, Action: "period.cancel_retry", ResourceType: "period_cancellation", ResourceID: out.ID, Reason: strings.TrimSpace(reason), RequestID: meta.RequestID, IP: meta.IP, Before: map[string]any{"state": "failed", "version": version}, After: out})
	return out, err
}
