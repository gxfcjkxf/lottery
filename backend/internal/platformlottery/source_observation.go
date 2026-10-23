package platformlottery

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
	"time"
)

type DrawAttributes struct {
	Waves    map[string]string `json:"waves"`
	Zodiac   map[string]string `json:"zodiac"`
	Elements map[string]string `json:"elements"`
}
type SourceObservedDraw struct {
	PeriodNo   string          `json:"period_no"`
	DrawnAt    time.Time       `json:"drawn_at"`
	Result     rules.Draw      `json:"result"`
	Attributes *DrawAttributes `json:"attributes,omitempty"`
}
type SourceCheck struct {
	At           time.Time           `json:"at"`
	Code         string              `json:"code"`
	NextPeriodNo *string             `json:"next_period_no"`
	NextDrawAt   *time.Time          `json:"next_draw_at"`
	LastResult   *SourceObservedDraw `json:"last_result"`
}

func (s Service) ReadSourceCheck(ctx context.Context, game string, version int64) (*SourceCheck, error) {
	var c SourceCheck
	var raw []byte
	e := s.DB.QueryRow(ctx, `SELECT checked_at,code,next_period_no,next_draw_at,last_result FROM platform_hk_source_checks WHERE game_id=$1 AND version=$2`, game, version).Scan(&c.At, &c.Code, &c.NextPeriodNo, &c.NextDrawAt, &raw)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	if len(raw) > 0 {
		e = json.Unmarshal(raw, &c.LastResult)
	}
	return &c, e
}
