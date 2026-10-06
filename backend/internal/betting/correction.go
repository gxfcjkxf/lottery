package betting

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/drawfeed"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

type CorrectionInput struct {
	Version       int64      `json:"version"`
	PolicyVersion *int64     `json:"policy_version"`
	Result        rules.Draw `json:"result"`
	Reason        string     `json:"reason"`
}

func (in *CorrectionInput) UnmarshalJSON(raw []byte) error {
	type alias CorrectionInput
	var v alias
	if e := decodeSettlementInput(raw, &v, "policy_version", "version", "policy_version", "result", "reason"); e != nil {
		return e
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return ErrInvalid
	}
	var groups map[string]json.RawMessage
	if e := decodeSettlementInput(fields["result"], &groups, "", "regular", "special", "digits"); e != nil {
		return e
	}
	for _, name := range []string{"regular", "special", "digits"} {
		var nums []int
		if json.Unmarshal(groups[name], &nums) != nil || nums == nil {
			return ErrInvalid
		}
	}
	*in = CorrectionInput(v)
	return nil
}

type CorrectionContext struct {
	BrandID              string      `json:"brand_id"`
	GameID               string      `json:"game_id"`
	PeriodID             string      `json:"period_id"`
	PeriodVersion        int64       `json:"period_version"`
	PeriodStatus         string      `json:"period_status"`
	DrawResultID         *string     `json:"draw_result_id"`
	Draw                 *rules.Draw `json:"draw"`
	Model                rules.Model `json:"model"`
	CurrentJobID         *string     `json:"current_job_id"`
	CurrentJobVersion    *int64      `json:"current_job_version"`
	PolicyVersion        int64       `json:"policy_version"`
	Mode                 *string     `json:"mode"`
	RequiresResettlement bool        `json:"requires_resettlement"`
	CanCorrect           bool        `json:"can_correct"`
}
type Correction struct {
	ID                   string     `json:"id"`
	BrandID              string     `json:"brand_id"`
	GameID               string     `json:"game_id"`
	PeriodID             string     `json:"period_id"`
	PreviousDrawResultID string     `json:"previous_draw_result_id"`
	DrawResultID         string     `json:"draw_result_id"`
	Result               rules.Draw `json:"result"`
	PeriodVersion        int64      `json:"period_version"`
	PreviousJobID        *string    `json:"previous_job_id"`
	NewJobID             *string    `json:"new_job_id"`
	PolicyVersion        *int64     `json:"policy_version"`
	Mode                 *string    `json:"mode"`
	State                string     `json:"state"`
	Version              int64      `json:"version"`
	TargetCount          int64      `json:"target_count"`
	CreatedBy            string     `json:"created_by"`
	Reason               string     `json:"reason"`
	CreatedAt            time.Time  `json:"created_at"`
	CompletedAt          *time.Time `json:"completed_at"`
	LastErrorCode        *string    `json:"last_error_code"`
	PendingCount         int64      `json:"pending_count"`
	ReversedCount        int64      `json:"reversed_count"`
	UnchangedCount       int64      `json:"unchanged_count"`
	ExcludedCount        int64      `json:"excluded_count"`
	FailedCount          int64      `json:"failed_count"`
	ReversePoints        string     `json:"reverse_points"`
	ReversedPoints       string     `json:"reversed_points"`
	CanRetry             bool       `json:"can_retry"`
	NewJobState          *string    `json:"new_job_state"`
	NewJobVersion        *int64     `json:"new_job_version"`
	NewJobErrorCode      *string    `json:"new_job_error_code"`
}
type CorrectionTarget struct {
	OrderID           string        `json:"order_id"`
	MemberID          string        `json:"member_id"`
	State             string        `json:"state"`
	Version           int64         `json:"version"`
	OldOrderVersion   int64         `json:"old_order_version"`
	OldOrderStatus    string        `json:"old_order_status"`
	OldCalculationID  *string       `json:"old_calculation_id"`
	OldPayoutEntryID  *string       `json:"old_payout_entry_id"`
	OldPrizePoints    points.Amount `json:"old_prize_points"`
	ReversalEntryID   *string       `json:"reversal_entry_id"`
	ResetOrderVersion *int64        `json:"reset_order_version"`
	ErrorCode         *string       `json:"error_code"`
}
type CorrectionPage struct {
	BrandID  string       `json:"brand_id"`
	PeriodID string       `json:"period_id"`
	Items    []Correction `json:"items"`
	Limit    int          `json:"limit"`
	Offset   int          `json:"offset"`
	HasMore  bool         `json:"has_more"`
}
type CorrectionTargetPage struct {
	BrandID      string             `json:"brand_id"`
	CorrectionID string             `json:"correction_id"`
	Items        []CorrectionTarget `json:"items"`
	Limit        int                `json:"limit"`
	Offset       int                `json:"offset"`
	HasMore      bool               `json:"has_more"`
}

const correctionFields = `c.id::text,c.brand_id::text,c.game_id::text,c.period_id::text,c.previous_draw_result_id::text,c.draw_result_id::text,d.result,c.period_version,c.previous_job_id::text,c.new_job_id::text,c.policy_version,c.mode,c.state,c.version,c.target_count,c.created_by::text,c.reason,c.created_at,c.completed_at,c.last_error_code`
const correctionJoin = ` FROM draw_corrections c JOIN draw_results d ON d.id=c.draw_result_id `
const correctionCounts = `,(SELECT count(*) FROM draw_correction_targets t WHERE t.correction_id=c.id AND t.state='pending'),(SELECT count(*) FROM draw_correction_targets t WHERE t.correction_id=c.id AND t.state='reversed'),(SELECT count(*) FROM draw_correction_targets t WHERE t.correction_id=c.id AND t.state='unchanged'),(SELECT count(*) FROM draw_correction_targets t WHERE t.correction_id=c.id AND t.state='excluded'),(SELECT count(*) FROM draw_correction_targets t WHERE t.correction_id=c.id AND t.state='failed'),coalesce((SELECT sum(t.old_prize_points)::text FROM draw_correction_targets t WHERE t.correction_id=c.id),'0'),coalesce((SELECT sum(t.old_prize_points)::text FROM draw_correction_targets t WHERE t.correction_id=c.id AND t.state='reversed'),'0'),(SELECT j.state FROM settlement_jobs j WHERE j.id=c.new_job_id),(SELECT j.version FROM settlement_jobs j WHERE j.id=c.new_job_id),(SELECT j.last_error_code FROM settlement_jobs j WHERE j.id=c.new_job_id)`

func scanCorrection(row pgx.Row, counts bool) (Correction, error) {
	var c Correction
	var raw []byte
	dst := []any{&c.ID, &c.BrandID, &c.GameID, &c.PeriodID, &c.PreviousDrawResultID, &c.DrawResultID, &raw, &c.PeriodVersion, &c.PreviousJobID, &c.NewJobID, &c.PolicyVersion, &c.Mode, &c.State, &c.Version, &c.TargetCount, &c.CreatedBy, &c.Reason, &c.CreatedAt, &c.CompletedAt, &c.LastErrorCode}
	if counts {
		dst = append(dst, &c.PendingCount, &c.ReversedCount, &c.UnchangedCount, &c.ExcludedCount, &c.FailedCount, &c.ReversePoints, &c.ReversedPoints, &c.NewJobState, &c.NewJobVersion, &c.NewJobErrorCode)
	}
	e := row.Scan(dst...)
	if errors.Is(e, pgx.ErrNoRows) {
		e = ErrNotFound
	}
	if e == nil {
		e = json.Unmarshal(raw, &c.Result)
	}
	c.CanRetry = c.State == "failed"
	return c, e
}
func readCorrectionContext(ctx context.Context, q settlementQuerier, brand, period string) (CorrectionContext, error) {
	var c CorrectionContext
	var model, draw []byte
	var active bool
	e := q.QueryRow(ctx, `SELECT p.brand_id::text,p.game_id::text,p.id::text,p.version,p.status,p.draw_result_id::text,d.result,g.model,p.current_settlement_job_id::text,j.version,b.version,b.mode,EXISTS(SELECT 1 FROM draw_corrections c WHERE c.brand_id=p.brand_id AND c.period_id=p.id AND c.state<>'completed')
 FROM periods p JOIN games g ON g.id=p.game_id JOIN brand_settlement_policies b ON b.brand_id=p.brand_id LEFT JOIN draw_results d ON d.id=p.draw_result_id LEFT JOIN settlement_jobs j ON j.id=p.current_settlement_job_id WHERE p.brand_id=$1 AND p.id=$2`, brand, period).Scan(&c.BrandID, &c.GameID, &c.PeriodID, &c.PeriodVersion, &c.PeriodStatus, &c.DrawResultID, &draw, &model, &c.CurrentJobID, &c.CurrentJobVersion, &c.PolicyVersion, &c.Mode, &active)
	if errors.Is(e, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	if e != nil {
		return c, e
	}
	if e = json.Unmarshal(model, &c.Model); e != nil {
		return c, e
	}
	if draw != nil {
		c.Draw = &rules.Draw{}
		if e = json.Unmarshal(draw, c.Draw); e != nil {
			return c, e
		}
	}
	c.RequiresResettlement = c.CurrentJobID != nil
	c.CanCorrect = !active && c.DrawResultID != nil && ((c.PeriodStatus == "drawn" && !c.RequiresResettlement) || ((c.PeriodStatus == "settling" || c.PeriodStatus == "settled") && c.RequiresResettlement && c.Mode != nil))
	return c, nil
}
func (s Service) CorrectionContext(ctx context.Context, brand, period string) (CorrectionContext, error) {
	if !validIDs(brand, period) {
		return CorrectionContext{}, ErrInvalid
	}
	return readCorrectionContext(ctx, s.DB, brand, period)
}
func (s Service) Correction(ctx context.Context, brand, id string) (Correction, error) {
	if !validIDs(brand, id) {
		return Correction{}, ErrInvalid
	}
	return scanCorrection(s.DB.QueryRow(ctx, `SELECT `+correctionFields+correctionCounts+correctionJoin+` WHERE c.brand_id=$1 AND c.id=$2`, brand, id), true)
}
func (s Service) Corrections(ctx context.Context, brand, period string, limit, offset int) (CorrectionPage, error) {
	out := CorrectionPage{BrandID: brand, PeriodID: period, Items: []Correction{}, Limit: limit, Offset: offset}
	if limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		return out, ErrInvalid
	}
	if _, e := s.CorrectionContext(ctx, brand, period); e != nil {
		return out, e
	}
	rows, e := s.DB.Query(ctx, `SELECT `+correctionFields+correctionCounts+correctionJoin+` WHERE c.brand_id=$1 AND c.period_id=$2 ORDER BY c.created_at DESC,c.id DESC LIMIT $3 OFFSET $4`, brand, period, limit+1, offset)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		c, e := scanCorrection(rows, true)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, c)
	}
	if len(out.Items) > limit {
		out.HasMore = true
		out.Items = out.Items[:limit]
	}
	return out, rows.Err()
}
func (s Service) CorrectionTargets(ctx context.Context, brand, id string, limit, offset int) (CorrectionTargetPage, error) {
	out := CorrectionTargetPage{BrandID: brand, CorrectionID: id, Items: []CorrectionTarget{}, Limit: limit, Offset: offset}
	if limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		return out, ErrInvalid
	}
	if _, e := s.Correction(ctx, brand, id); e != nil {
		return out, e
	}
	rows, e := s.DB.Query(ctx, `SELECT t.order_id::text,o.brand_member_id::text,t.state,t.version,t.old_order_version,t.old_order_status,t.old_calculation_id::text,t.old_payout_entry_id::text,t.old_prize_points,t.reversal_entry_id::text,t.reset_order_version,t.error_code FROM draw_correction_targets t JOIN bet_orders o ON o.id=t.order_id WHERE t.brand_id=$1 AND t.correction_id=$2 ORDER BY t.order_id LIMIT $3 OFFSET $4`, brand, id, limit+1, offset)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var t CorrectionTarget
		if e = rows.Scan(&t.OrderID, &t.MemberID, &t.State, &t.Version, &t.OldOrderVersion, &t.OldOrderStatus, &t.OldCalculationID, &t.OldPayoutEntryID, &t.OldPrizePoints, &t.ReversalEntryID, &t.ResetOrderVersion, &t.ErrorCode); e != nil {
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
func lockCorrectionPeriod(ctx context.Context, tx pgx.Tx, brand, period string) error {
	var game string
	e := tx.QueryRow(ctx, `SELECT game_id::text FROM periods WHERE brand_id=$1 AND id=$2`, brand, period).Scan(&game)
	if errors.Is(e, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if e != nil {
		return e
	}
	if e = tx.QueryRow(ctx, `SELECT id::text FROM games WHERE brand_id=$1 AND id=$2 FOR UPDATE`, brand, game).Scan(&game); e != nil {
		return e
	}
	return tx.QueryRow(ctx, `SELECT id::text FROM periods WHERE brand_id=$1 AND id=$2 FOR UPDATE`, brand, period).Scan(&period)
}
func lockCorrection(ctx context.Context, tx pgx.Tx, brand, id string) (Correction, error) {
	return scanCorrection(tx.QueryRow(ctx, `SELECT `+correctionFields+correctionJoin+` WHERE c.brand_id=$1 AND c.id=$2 FOR UPDATE OF c`, brand, id), false)
}

func (s Service) CreateCorrection(ctx context.Context, tx pgx.Tx, brand string, a access.Account, drawID string, in CorrectionInput, meta points.Metadata) (Correction, error) {
	var out Correction
	if tx == nil || !validIDs(brand, a.ID, drawID) || in.Version < 1 || !validPolicyReason(in.Reason) {
		return out, ErrInvalid
	}
	if a.SuperAdmin || !access.Authorize(a, "draw", "correct", access.ScopeBrand, brand) {
		return out, ErrDenied
	}
	var period string
	e := tx.QueryRow(ctx, `SELECT period_id::text FROM draw_results WHERE brand_id=$1 AND id=$2`, brand, drawID).Scan(&period)
	if errors.Is(e, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if e != nil {
		return out, e
	}
	if e = lockCorrectionPeriod(ctx, tx, brand, period); e != nil {
		return out, e
	}
	var lockedBrand string
	if e = tx.QueryRow(ctx, `SELECT brand_id::text FROM brand_settlement_policies WHERE brand_id=$1 FOR SHARE`, brand).Scan(&lockedBrand); e != nil {
		return out, e
	}
	c, e := readCorrectionContext(ctx, tx, brand, period)
	if e != nil {
		return out, e
	}
	if c.PeriodVersion != in.Version || c.PeriodVersion == math.MaxInt64 || c.DrawResultID == nil || *c.DrawResultID != drawID {
		return out, ErrVersion
	}
	if !c.CanCorrect {
		return out, ErrState
	}
	if c.RequiresResettlement {
		if !access.Authorize(a, "settlement", "run", access.ScopeBrand, brand) {
			return out, ErrDenied
		}
		if in.PolicyVersion == nil || *in.PolicyVersion != c.PolicyVersion {
			return out, ErrVersion
		}
		if _, e = scanSettlementJob(tx.QueryRow(ctx, `SELECT `+settlementJobFields+` FROM settlement_jobs j WHERE j.id=$1 FOR UPDATE`, *c.CurrentJobID), false); e != nil {
			return out, e
		}
	} else if in.PolicyVersion != nil {
		return out, ErrInvalid
	}
	result := in.Result
	result.Regular = append([]int{}, result.Regular...)
	result.Special = append([]int{}, result.Special...)
	result.Digits = append([]int{}, result.Digits...)
	if !c.Model.Ordered {
		sort.Ints(result.Regular)
		sort.Ints(result.Special)
	}
	if rules.ValidateDraw(c.Model, result) != nil {
		return out, ErrInvalid
	}
	if digest(result) == digest(*c.Draw) {
		return out, ErrState
	}
	var previous []byte
	var number string
	var drawnAt time.Time
	e = tx.QueryRow(ctx, `SELECT p.period_no,d.drawn_at FROM periods p JOIN draw_results d ON d.id=p.draw_result_id WHERE p.id=$1`, period).Scan(&number, &drawnAt)
	if e != nil {
		return out, e
	}
	e = tx.QueryRow(ctx, `SELECT d.result FROM periods p JOIN draw_results d ON d.id=p.draw_result_id WHERE p.brand_id=$1 AND p.game_id=$2 AND p.draw_at<(SELECT draw_at FROM periods WHERE id=$3) ORDER BY p.draw_at DESC,p.id DESC LIMIT 1`, brand, c.GameID, period).Scan(&previous)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return out, e
	}
	var prev *rules.Draw
	if previous != nil {
		prev = &rules.Draw{}
		if e = json.Unmarshal(previous, prev); e != nil {
			return out, e
		}
	}
	if drawfeed.ValidateCandidate(drawfeed.Request{PeriodNo: number, Model: c.Model, Previous: prev}, drawfeed.Candidate{PeriodNo: number, Draw: result, DrawnAt: drawnAt}) != nil {
		return out, ErrInvalid
	}
	var source string
	e = tx.QueryRow(ctx, `SELECT id::text FROM draw_sources WHERE brand_id=$1 AND game_id=$2 AND type='manual'`, brand, c.GameID).Scan(&source)
	if errors.Is(e, pgx.ErrNoRows) {
		source = ids.New()
		_, e = tx.Exec(ctx, `INSERT INTO draw_sources(id,brand_id,game_id,type) VALUES($1,$2,$3,'manual')`, source, brand, c.GameID)
	}
	if e != nil {
		return out, e
	}
	id, newDraw := ids.New(), ids.New()
	raw, e := json.Marshal(result)
	if e != nil {
		return out, e
	}
	_, e = tx.Exec(ctx, `INSERT INTO draw_results(id,brand_id,game_id,period_id,source_id,kind,result,result_hash,drawn_at,created_by,corrected_from_id) VALUES($1,$2,$3,$4,$5,'manual',$6,$7,$8,$9,$10)`, newDraw, brand, c.GameID, period, source, raw, digest(result), drawnAt, a.ID, drawID)
	if e != nil {
		return out, e
	}
	state := "completed"
	var completed *time.Time
	var now time.Time
	if e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); e != nil {
		return out, e
	}
	completed = &now
	var count int64
	var mode *string
	var pv *int64
	if c.RequiresResettlement {
		state = "reversing"
		completed = nil
		mode = c.Mode
		pv = in.PolicyVersion
		if e = tx.QueryRow(ctx, `SELECT count(*) FROM bet_orders WHERE brand_id=$1 AND period_id=$2`, brand, period).Scan(&count); e != nil {
			return out, e
		}
	}
	_, e = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: a.ID, Action: "draw.correct", ResourceType: "draw_correction", ResourceID: id, Reason: strings.TrimSpace(in.Reason), RequestID: meta.RequestID, IP: meta.IP, Before: c, After: map[string]any{"draw_result_id": newDraw, "result": result, "previous_draw_result_id": drawID, "previous_job_id": c.CurrentJobID, "policy_version": pv, "mode": mode}})
	if e != nil {
		return out, e
	}
	_, e = tx.Exec(ctx, `INSERT INTO draw_corrections(id,brand_id,game_id,period_id,previous_draw_result_id,draw_result_id,period_version,previous_job_id,policy_version,mode,state,target_count,created_by,reason,created_at,completed_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`, id, brand, c.GameID, period, drawID, newDraw, c.PeriodVersion+1, c.CurrentJobID, pv, mode, state, count, a.ID, strings.TrimSpace(in.Reason), now, completed)
	if e != nil {
		return out, e
	}
	if c.RequiresResettlement {
		_, e = tx.Exec(ctx, `INSERT INTO draw_correction_targets(brand_id,correction_id,period_id,order_id,old_order_version,old_order_status,old_calculation_id,old_payout_entry_id,old_prize_points,state) SELECT brand_id,$1,period_id,id,version,status,settlement_calculation_id,payout_entry_id,prize_points,CASE WHEN status IN ('abnormal','bet_cancelled','judged_cancelled') THEN 'excluded' ELSE 'pending' END FROM bet_orders WHERE brand_id=$2 AND period_id=$3`, id, brand, period)
		if e != nil {
			return out, e
		}
		_, e = tx.Exec(ctx, `UPDATE periods SET current_correction_id=$2,status='settling',version=version+1,state_reason='result correction reversing' WHERE id=$1`, period, id)
	} else {
		_, e = tx.Exec(ctx, `UPDATE periods SET current_correction_id=$2,draw_result_id=$3,version=version+1,state_reason='unsettled result corrected' WHERE id=$1`, period, id, newDraw)
	}
	if e != nil {
		return out, e
	}
	out, e = scanCorrection(tx.QueryRow(ctx, `SELECT `+correctionFields+correctionCounts+correctionJoin+` WHERE c.id=$1`, id), true)
	if e != nil {
		return out, e
	}
	if e = correctionEvent(ctx, tx, out, "draw.correction.started"); e != nil {
		return out, e
	}
	return out, nil
}
func (s Service) RetryCorrection(ctx context.Context, tx pgx.Tx, brand string, a access.Account, id string, in SettlementActionInput, meta points.Metadata) (Correction, error) {
	var out Correction
	if tx == nil || !validIDs(brand, id, a.ID) || in.Version < 1 || !validPolicyReason(in.Reason) {
		return out, ErrInvalid
	}
	if a.SuperAdmin || !access.Authorize(a, "draw", "correction_retry", access.ScopeBrand, brand) {
		return out, ErrDenied
	}
	var period string
	e := tx.QueryRow(ctx, `SELECT period_id::text FROM draw_corrections WHERE brand_id=$1 AND id=$2`, brand, id).Scan(&period)
	if errors.Is(e, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if e != nil {
		return out, e
	}
	if e = lockCorrectionPeriod(ctx, tx, brand, period); e != nil {
		return out, e
	}
	out, e = lockCorrection(ctx, tx, brand, id)
	if e != nil {
		return out, e
	}
	if out.Version != in.Version || out.Version == math.MaxInt64 {
		return out, ErrVersion
	}
	if out.State != "failed" {
		return out, ErrState
	}
	if !access.Authorize(a, "settlement", "run", access.ScopeBrand, brand) {
		return out, ErrDenied
	}
	_, e = tx.Exec(ctx, `UPDATE draw_correction_targets SET state='pending',version=version+1,error_code=NULL WHERE correction_id=$1 AND state='failed'`, id)
	if e != nil {
		return out, e
	}
	_, e = tx.Exec(ctx, `UPDATE draw_corrections SET state='reversing',version=version+1,last_error_code=NULL WHERE id=$1`, id)
	if e != nil {
		return out, e
	}
	out, e = scanCorrection(tx.QueryRow(ctx, `SELECT `+correctionFields+correctionCounts+correctionJoin+` WHERE c.id=$1`, id), true)
	if e != nil {
		return out, e
	}
	_, e = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: a.ID, Action: "draw.correction_retry", ResourceType: "draw_correction", ResourceID: id, Reason: strings.TrimSpace(in.Reason), RequestID: meta.RequestID, IP: meta.IP, After: out})
	if e != nil {
		return out, e
	}
	return out, correctionEvent(ctx, tx, out, "draw.correction.retried")
}
func correctionEvent(ctx context.Context, tx pgx.Tx, c Correction, event string) error {
	_, e := tx.Exec(ctx, `INSERT INTO outbox_events(id,brand_id,event_type,aggregate_id,payload) VALUES($1,$2,$3,$4::uuid,jsonb_build_object('correction_id',$4::uuid::text,'period_id',$5::text,'state',$6::text,'version',$7::bigint))`, ids.New(), c.BrandID, event, c.ID, c.PeriodID, c.State, c.Version)
	return e
}
