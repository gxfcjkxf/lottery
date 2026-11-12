package platformlottery

import (
	"context"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"time"
)

// Timing changes publish a new plan; reserved periods keep their frozen version.
type AutomationTimingInput struct {
	Version            int64      `json:"version"`
	OpenBeforeSeconds  int        `json:"open_before_seconds"`
	CloseBeforeSeconds int        `json:"close_before_seconds"`
	AnchorDrawAt       *time.Time `json:"anchor_draw_at"`
	AnchorPeriodNo     *string    `json:"anchor_period_no"`
	IntervalSeconds    *int       `json:"interval_seconds"`
}

func (s Service) SaveAutomationTiming(ctx context.Context, tx pgx.Tx, a access.Account, game string, in AutomationTimingInput, meta points.Metadata) (AutomationPlan, error) {
	if !Allowed(a, "", "write") {
		return AutomationPlan{}, ErrDenied
	}
	if _, err := lockedGame(ctx, tx, game); err != nil {
		return AutomationPlan{}, err
	}
	p, err := scanAutomation(tx.QueryRow(ctx, `SELECT `+automationPlanFields+` FROM platform_automation_settings h JOIN platform_automation_versions v USING(game_id,version) WHERE h.game_id=$1`, game))
	if errors.Is(err, pgx.ErrNoRows) {
		return AutomationPlan{}, ErrConflict
	}
	if err != nil {
		return AutomationPlan{}, err
	}
	if p.Version != in.Version {
		return AutomationPlan{}, ErrVersionConflict
	}
	if p.Mapping == nil || (p.Mapping.Schedule != "source" && p.Mapping.Schedule != "interval") {
		return AutomationPlan{}, ErrInvalid
	}
	var source DrawSource
	if err = tx.QueryRow(ctx, `SELECT source_id::text,source_revision FROM platform_draw_source_activations WHERE game_id=$1 AND automation_version=$2`, game, p.Version).Scan(&source.ID, &source.Revision); err != nil {
		return AutomationPlan{}, err
	}
	if p.Enabled {
		sources, e := LoadAutomationSources()
		if e != nil {
			return AutomationPlan{}, e
		}
		if !sources.Allows(p.SourceURL) {
			return AutomationPlan{}, ErrInvalid
		}
	}
	p.OpenBeforeSeconds = &in.OpenBeforeSeconds
	p.CloseBeforeSeconds = in.CloseBeforeSeconds
	p.AnchorDrawAt = in.AnchorDrawAt
	p.AnchorPeriodNo = in.AnchorPeriodNo
	p.IntervalSeconds = in.IntervalSeconds
	return s.publishAutomation(ctx, tx, a, game, AutomationInput{AutomationPlan: p}, source, meta)
}
