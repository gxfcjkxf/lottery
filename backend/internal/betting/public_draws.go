package betting

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

// These public views deliberately omit source IDs, endpoints, raw responses,
// claim leases, credentials, administrator identities and correction history.
type PublicDrawResult struct {
	ID      string      `json:"id"`
	Game    CatalogGame `json:"game"`
	Period  Period      `json:"period"`
	Result  rules.Draw  `json:"result"`
	DrawnAt time.Time   `json:"drawn_at"`
	Origin  string      `json:"origin"`
}
type PublicPeriod struct {
	Period Period            `json:"period"`
	Draw   *PublicDrawResult `json:"draw"`
}
type PublicDrawPage struct {
	Items       []PublicDrawResult `json:"items"`
	Limit       int                `json:"limit"`
	Offset      int                `json:"offset"`
	HasMore     bool               `json:"has_more"`
	ServerTime  time.Time          `json:"server_time"`
	BrandStatus string             `json:"brand_status"`
}
type PublicDrawDetail struct {
	Item        PublicDrawResult `json:"item"`
	ServerTime  time.Time        `json:"server_time"`
	BrandStatus string           `json:"brand_status"`
}
type PublicPeriodPage struct {
	Game        CatalogGame    `json:"game"`
	Items       []PublicPeriod `json:"items"`
	Limit       int            `json:"limit"`
	Offset      int            `json:"offset"`
	HasMore     bool           `json:"has_more"`
	ServerTime  time.Time      `json:"server_time"`
	BrandStatus string         `json:"brand_status"`
}
type PublicDrawFilter struct {
	GameID   string
	PeriodNo string
	Limit    int
	Offset   int
}

func validPublicDrawFilter(f PublicDrawFilter) bool {
	return (f.GameID == "" || validIDs(f.GameID)) && f.Limit >= 1 && f.Limit <= 100 && f.Offset >= 0 && f.Offset <= 1000000 && utf8.ValidString(f.PeriodNo) && len(f.PeriodNo) <= 80 && (f.PeriodNo == "" || strings.TrimSpace(f.PeriodNo) != "")
}

const publicPeriodFields = `p.id::text,p.game_id::text,p.period_no,p.status,p.bet_start_at,p.bet_end_at,p.draw_at,coalesce(p.draw_result_id::text,'')`
const publicGameFields = `g.id::text,g.code,g.name,g.model,g.timezone,g.status`
const publicDrawFields = `r.id::text,` + publicGameFields + `,` + publicPeriodFields + `,r.result,r.drawn_at,r.kind`
const publicDrawJoin = ` FROM periods p JOIN games g ON g.brand_id=p.brand_id AND g.id=p.game_id JOIN draw_results r ON r.brand_id=p.brand_id AND r.game_id=p.game_id AND r.period_id=p.id AND r.id=p.draw_result_id `
const publicDrawVisible = `p.brand_id=$1 AND g.status IN ('active','paused') AND p.bet_start_at<=$2 AND p.status IN ('drawn','settling','settled','bet_cancelled','judged_cancelled')`

func normalizePublicPeriod(p *Period) {
	p.BetStartAt = p.BetStartAt.UTC()
	p.BetEndAt = p.BetEndAt.UTC()
	p.DrawAt = p.DrawAt.UTC()
}
func decodePublicDraw(v *PublicDrawResult, raw []byte, kind string) error {
	if json.Unmarshal(raw, &v.Result) != nil || rules.ValidateDraw(v.Game.Model, v.Result) != nil {
		return ErrInvalid
	}
	if v.Result.Regular == nil {
		v.Result.Regular = []int{}
	}
	if v.Result.Special == nil {
		v.Result.Special = []int{}
	}
	if v.Result.Digits == nil {
		v.Result.Digits = []int{}
	}
	if kind == "manual" {
		v.Origin = "manual"
	} else if kind == "api" || kind == "dom" {
		v.Origin = "external"
	} else {
		return ErrInvalid
	}
	v.DrawnAt = v.DrawnAt.UTC()
	normalizePublicPeriod(&v.Period)
	return nil
}
func scanPublicDraw(row pgx.Row) (v PublicDrawResult, err error) {
	var model, raw []byte
	var kind string
	err = row.Scan(&v.ID, &v.Game.ID, &v.Game.Code, &v.Game.Name, &model, &v.Game.Timezone, &v.Game.Status, &v.Period.ID, &v.Period.GameID, &v.Period.PeriodNo, &v.Period.Status, &v.Period.BetStartAt, &v.Period.BetEndAt, &v.Period.DrawAt, &v.Period.DrawResultID, &raw, &v.DrawnAt, &kind)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	if err != nil {
		return v, err
	}
	if json.Unmarshal(model, &v.Game.Model) != nil || rules.ValidateModel(v.Game.Model) != nil {
		return v, ErrInvalid
	}
	return v, decodePublicDraw(&v, raw, kind)
}

// Only the current selected result is public. Superseded immutable result rows
// remain available to authorized operators, never through this archive.
func (s Service) PublicDraws(ctx context.Context, brand string, f PublicDrawFilter) (PublicDrawPage, error) {
	out := PublicDrawPage{Items: []PublicDrawResult{}, Limit: f.Limit, Offset: f.Offset}
	if !validPublicDrawFilter(f) {
		return out, ErrInvalid
	}
	tx, status, now, err := s.catalogTx(ctx, brand)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	out.ServerTime, out.BrandStatus = now, status
	if f.GameID != "" {
		if _, err = scanCatalogGame(tx.QueryRow(ctx, `SELECT `+catalogGameFields+` FROM games WHERE brand_id=$1 AND id=$2 AND status IN ('active','paused')`, brand, f.GameID)); err != nil {
			return out, err
		}
	}
	rows, err := tx.Query(ctx, `SELECT `+publicDrawFields+publicDrawJoin+`WHERE `+publicDrawVisible+` AND ($3='' OR p.game_id=NULLIF($3,'')::uuid) AND ($4='' OR p.period_no=$4) ORDER BY p.draw_at DESC,p.sequence DESC,p.id DESC LIMIT $5 OFFSET $6`, brand, now, f.GameID, f.PeriodNo, f.Limit+1, f.Offset)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		v, e := scanPublicDraw(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.Items = append(out.Items, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Items) > f.Limit {
		out.HasMore = true
		out.Items = out.Items[:f.Limit]
	}
	return out, tx.Commit(ctx)
}
func (s Service) PublicDraw(ctx context.Context, brand, id string) (PublicDrawDetail, error) {
	var out PublicDrawDetail
	if !validIDs(id) {
		return out, ErrInvalid
	}
	tx, status, now, err := s.catalogTx(ctx, brand)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	out.ServerTime, out.BrandStatus = now, status
	out.Item, err = scanPublicDraw(tx.QueryRow(ctx, `SELECT `+publicDrawFields+publicDrawJoin+`WHERE `+publicDrawVisible+` AND r.id=$3`, brand, now, id))
	if err != nil {
		return out, err
	}
	return out, tx.Commit(ctx)
}
func (s Service) PublicPeriods(ctx context.Context, brand, game string, f PublicDrawFilter) (PublicPeriodPage, error) {
	out := PublicPeriodPage{Items: []PublicPeriod{}, Limit: f.Limit, Offset: f.Offset}
	if !validIDs(game) || !validPublicDrawFilter(f) || f.GameID != "" && f.GameID != game {
		return out, ErrInvalid
	}
	tx, status, now, err := s.catalogTx(ctx, brand)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	out.ServerTime, out.BrandStatus = now, status
	out.Game, err = scanCatalogGame(tx.QueryRow(ctx, `SELECT `+catalogGameFields+` FROM games WHERE brand_id=$1 AND id=$2 AND status IN ('active','paused')`, brand, game))
	if err != nil {
		return out, err
	}
	rows, err := tx.Query(ctx, `SELECT `+publicPeriodFields+`,r.id::text,r.result,r.drawn_at,r.kind FROM periods p LEFT JOIN draw_results r ON r.brand_id=p.brand_id AND r.game_id=p.game_id AND r.period_id=p.id AND r.id=p.draw_result_id WHERE p.brand_id=$1 AND p.game_id=$2 AND p.bet_start_at<=$3 AND ($4='' OR p.period_no=$4) ORDER BY p.draw_at DESC,p.sequence DESC,p.id DESC LIMIT $5 OFFSET $6`, brand, game, now, f.PeriodNo, f.Limit+1, f.Offset)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var v PublicPeriod
		var id, kind *string
		var raw []byte
		var at *time.Time
		err = rows.Scan(&v.Period.ID, &v.Period.GameID, &v.Period.PeriodNo, &v.Period.Status, &v.Period.BetStartAt, &v.Period.BetEndAt, &v.Period.DrawAt, &v.Period.DrawResultID, &id, &raw, &at, &kind)
		if err != nil {
			rows.Close()
			return out, err
		}
		normalizePublicPeriod(&v.Period)
		if id != nil {
			if at == nil || kind == nil || (v.Period.Status != "drawn" && v.Period.Status != "settling" && v.Period.Status != "settled" && v.Period.Status != "bet_cancelled" && v.Period.Status != "judged_cancelled") {
				rows.Close()
				return out, ErrInvalid
			}
			v.Draw = &PublicDrawResult{ID: *id, Game: out.Game, Period: v.Period, DrawnAt: *at}
			if err = decodePublicDraw(v.Draw, raw, *kind); err != nil {
				rows.Close()
				return out, err
			}
		} else if v.Period.DrawResultID != "" {
			rows.Close()
			return out, ErrInvalid
		}
		out.Items = append(out.Items, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Items) > f.Limit {
		out.HasMore = true
		out.Items = out.Items[:f.Limit]
	}
	return out, tx.Commit(ctx)
}
