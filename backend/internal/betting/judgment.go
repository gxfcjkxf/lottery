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

type JudgeCancelInput struct {
	Version int64  `json:"version"`
	Cause   string `json:"cause"`
	Reason  string `json:"reason"`
}
type Judgment struct {
	ID            string    `json:"id"`
	BrandID       string    `json:"brand_id"`
	GameID        string    `json:"game_id"`
	PeriodID      string    `json:"period_id"`
	OrderID       string    `json:"order_id"`
	OrderVersion  int64     `json:"order_version"`
	Cause         string    `json:"cause"`
	DrawResultID  string    `json:"draw_result_id"`
	JudgedBy      string    `json:"judged_by"`
	Reason        string    `json:"reason"`
	CreatedAt     time.Time `json:"created_at"`
	RefundEntryID string    `json:"refund_entry_id"`
}

func (s Service) Judgment(ctx context.Context, brand, id string) (*Judgment, error) {
	if _, err := s.Order(ctx, brand, "", id); err != nil {
		return nil, err
	}
	var v Judgment
	err := s.DB.QueryRow(ctx, `SELECT j.id::text,j.brand_id::text,j.game_id::text,j.period_id::text,j.order_id::text,j.order_version,j.cause,coalesce(j.draw_result_id::text,''),j.judged_by::text,j.reason,j.created_at,coalesce(o.refund_entry_id::text,'') FROM bet_order_judgments j JOIN bet_orders o ON o.brand_id=j.brand_id AND o.id=j.order_id WHERE j.brand_id=$1 AND j.order_id=$2`, brand, id).Scan(&v.ID, &v.BrandID, &v.GameID, &v.PeriodID, &v.OrderID, &v.OrderVersion, &v.Cause, &v.DrawResultID, &v.JudgedBy, &v.Reason, &v.CreatedAt, &v.RefundEntryID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	v.CreatedAt = v.CreatedAt.UTC()
	return &v, err
}
func (s Service) JudgeCancel(ctx context.Context, tx pgx.Tx, brand string, a access.Account, id string, in JudgeCancelInput, meta points.Metadata) (Order, error) {
	var o Order
	if tx == nil || !validIDs(brand, id, a.ID) || in.Version < 1 || !validPolicyReason(in.Reason) || (in.Cause != "no_result" && in.Cause != "invalid_result") {
		return o, ErrInvalid
	}
	if a.SuperAdmin || !access.Authorize(a, "bet", "judge_cancel", access.ScopeBrand, brand) {
		return o, ErrDenied
	}
	o, err := scanOrder(tx.QueryRow(ctx, `SELECT `+orderFields+` FROM bet_orders WHERE brand_id=$1 AND id=$2`, brand, id))
	if err != nil {
		return o, err
	}
	_, _, period, err := s.lockPeriod(ctx, tx, brand, o.PeriodID)
	if err != nil {
		return o, err
	}
	if period.Status == "settling" || period.Status == "settled" || in.Cause == "no_result" && period.DrawResultID != "" {
		return o, ErrState
	}
	policy, _, err := s.LockedPolicy(ctx, tx, brand, o.GameID)
	if err != nil {
		return o, err
	}
	if err = lockQuota(ctx, tx, o.PeriodID, policy); err != nil {
		return o, err
	}
	if _, err = (points.Store{DB: s.DB}).LockedSnapshot(ctx, tx, brand, o.MemberID); err != nil {
		return o, err
	}
	o, err = scanOrder(tx.QueryRow(ctx, `SELECT `+orderFields+` FROM bet_orders WHERE brand_id=$1 AND id=$2 FOR UPDATE`, brand, id))
	if err != nil {
		return o, err
	}
	if o.Version != in.Version || o.Version == math.MaxInt64 {
		return o, ErrVersion
	}
	if o.Status != "placed" && o.Status != "abnormal" {
		return o, ErrState
	}
	reason := strings.TrimSpace(in.Reason)
	evidenceID := ids.New()
	if _, err = tx.Exec(ctx, `INSERT INTO bet_order_judgments(id,brand_id,game_id,period_id,order_id,order_version,cause,draw_result_id,judged_by,reason) VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,'')::uuid,$9,$10)`, evidenceID, brand, o.GameID, o.PeriodID, o.ID, o.Version+1, in.Cause, period.DrawResultID, a.ID, reason); err != nil {
		return o, err
	}
	meta.ActorType = "admin"
	meta.ActorID = a.ID
	o, err = s.refundLocked(ctx, tx, o, "judged_cancelled", reason, meta)
	if err != nil {
		return o, err
	}
	_, err = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: a.ID, Action: "bet.judgment.recorded", ResourceType: "bet_order_judgment", ResourceID: evidenceID, Reason: reason, RequestID: meta.RequestID, IP: meta.IP, After: map[string]any{"order_id": o.ID, "order_version": o.Version, "cause": in.Cause, "draw_result_id": period.DrawResultID, "refund_entry_id": o.RefundEntryID}})
	return o, err
}
