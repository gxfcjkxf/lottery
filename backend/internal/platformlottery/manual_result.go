package platformlottery

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

type ManualResultInput struct {
	PeriodNo string     `json:"period_no"`
	DrawDate string     `json:"draw_date"`
	Result   rules.Draw `json:"result"`
}

// Historical evidence has a calendar date, not invented betting windows.
// Existing operational periods must still pass the normal publication checks.
func (s Service) AddManualResult(ctx context.Context, tx pgx.Tx, actor access.Account, gameID string, in ManualResultInput, meta points.Metadata) (DrawHistoryItem, error) {
	out := DrawHistoryItem{PeriodNo: in.PeriodNo, DrawDate: &in.DrawDate, Result: in.Result, Source: "manual"}
	out.Result.Regular = append([]int{}, in.Result.Regular...)
	out.Result.Special = append([]int{}, in.Result.Special...)
	out.Result.Digits = append([]int{}, in.Result.Digits...)
	if !Allowed(actor, "", "write") {
		return out, ErrDenied
	}
	date, err := time.Parse("2006-01-02", in.DrawDate)
	if err != nil || date.Year() < 1 || !textOK(in.PeriodNo, 100) || in.PeriodNo != strings.TrimSpace(in.PeriodNo) {
		return out, ErrInvalid
	}
	game, err := lockedGame(ctx, tx, gameID)
	if err != nil {
		return out, err
	}
	if game.Status != "active" {
		return out, ErrConflict
	}
	var today string
	if err = tx.QueryRow(ctx, `SELECT to_char(clock_timestamp() AT TIME ZONE timezone,'YYYY-MM-DD') FROM platform_settings WHERE id=true`).Scan(&today); err != nil {
		return out, err
	}
	if in.DrawDate > today || rules.ValidateDraw(game.Model, in.Result) != nil {
		return out, ErrInvalid
	}
	version, err := ReadConfiguredPublication(ctx, tx, gameID)
	if err != nil {
		return out, err
	}
	engine, err := rules.RestoreConfiguredEngine(version.Snapshot)
	if err != nil || engine.ValidateDraw(in.Result) != nil {
		return out, ErrInvalid
	}
	var duplicate bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM platform_draw_history WHERE game_id=$1 AND period_no=$2)`, gameID, in.PeriodNo).Scan(&duplicate); err != nil {
		return out, err
	}
	if duplicate {
		return out, ErrConflict
	}
	var periodID, periodDate string
	err = tx.QueryRow(ctx, `SELECT p.id::text,to_char(p.draw_at AT TIME ZONE s.timezone,'YYYY-MM-DD') FROM platform_game_periods p CROSS JOIN platform_settings s WHERE p.game_id=$1 AND p.period_no=$2 AND s.id=true`, gameID, in.PeriodNo).Scan(&periodID, &periodDate)
	if err == nil {
		if periodDate != in.DrawDate {
			return out, ErrInvalid
		}
		if _, err = s.Publish(ctx, tx, actor, periodID, in.Result, "", meta); err != nil {
			return out, err
		}
		out.Source = "published"
		return out, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	raw, err := json.Marshal(out.Result)
	if err != nil {
		return out, err
	}
	cmd, err := tx.Exec(ctx, `INSERT INTO platform_draw_history(game_id,period_no,draw_year,draw_date,result,source) VALUES($1,$2,$3,$4,$5,'manual') ON CONFLICT(game_id,period_no) DO NOTHING`, gameID, in.PeriodNo, date.Year(), in.DrawDate, raw)
	if err != nil {
		return out, err
	}
	if cmd.RowsAffected() != 1 {
		return out, ErrConflict
	}
	return out, record(ctx, tx, actor, "", "platform_game.result.manual", gameID, "", meta, nil, out)
}
