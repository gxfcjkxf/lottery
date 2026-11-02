package platformlottery

import (
	"context"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"math/big"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

type AutomationPlan struct {
	Mapping            *FeedMapping `json:"mapping,omitempty"`
	SourceType         string       `json:"source_type"`
	Version            int64        `json:"version"`
	Enabled            bool         `json:"enabled"`
	AnchorDrawAt       *time.Time   `json:"anchor_draw_at"`
	AnchorPeriodNo     *string      `json:"anchor_period_no"`
	IntervalSeconds    *int         `json:"interval_seconds"`
	OpenBeforeSeconds  *int         `json:"open_before_seconds"`
	CloseBeforeSeconds int          `json:"close_before_seconds"`
	SourceURL          string       `json:"source_url"`
	CredentialRef      string       `json:"credential_ref"`
}
type AutomationAttempt struct {
	PeriodNo string    `json:"period_no"`
	At       time.Time `json:"at"`
	Code     string    `json:"code"`
	Attempts int       `json:"attempts"`
}
type AutomationConfig struct {
	Schedule       *DrawSchedule       `json:"schedule"`
	Plan           *AutomationPlan     `json:"plan"`
	Attempts       []AutomationAttempt `json:"attempts"`
	SourceCheck    *SourceCheck        `json:"source_check"`
	AllowedOrigins []string            `json:"allowed_origins"`
}
type AutomationInput struct {
	AutomationPlan
	Reason string `json:"reason"`
}

const automationPlanFields = `v.version,v.enabled,v.anchor_draw_at,v.anchor_period_no,v.interval_seconds,v.open_before_seconds,v.close_before_seconds,v.source_url,v.credential_ref,v.source_type,v.mapping`

func scanAutomation(row pgx.Row) (p AutomationPlan, e error) {
	e = row.Scan(&p.Version, &p.Enabled, &p.AnchorDrawAt, &p.AnchorPeriodNo, &p.IntervalSeconds, &p.OpenBeforeSeconds, &p.CloseBeforeSeconds, &p.SourceURL, &p.CredentialRef, &p.SourceType, &p.Mapping)
	if p.AnchorDrawAt != nil {
		v := p.AnchorDrawAt.UTC()
		p.AnchorDrawAt = &v
	}
	return
}
func (s Service) AutomationConfig(ctx context.Context, brand, game string) (out AutomationConfig, e error) {
	out.Attempts = []AutomationAttempt{}
	out.AllowedOrigins = []string{}
	out.Schedule, e = s.DrawSchedule(ctx, game)
	if e != nil {
		return out, e
	}
	if _, e = s.Detail(ctx, brand, game, 1, 0); e != nil {
		return
	}
	p, err := scanAutomation(s.DB.QueryRow(ctx, `SELECT `+automationPlanFields+` FROM platform_automation_settings s JOIN platform_automation_versions v USING(game_id,version) WHERE s.game_id=$1`, game))
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	if err == nil {
		out.Plan = &p
	}
	if out.Schedule == nil && out.Plan != nil && (out.Plan.SourceType == "mapped" && out.Plan.Mapping != nil && out.Plan.Mapping.Schedule == "source") {
		out.SourceCheck, e = s.ReadSourceCheck(ctx, game, out.Plan.Version)
		if e != nil {
			return out, e
		}
	}
	rows, e := s.DB.Query(ctx, `SELECT p.period_no,a.last_attempt_at,a.code,a.attempts FROM platform_automation_attempts a JOIN platform_game_periods p ON p.id=a.period_id WHERE p.game_id=$1 ORDER BY p.draw_at DESC LIMIT 20`, game)
	if e != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var a AutomationAttempt
		if e = rows.Scan(&a.PeriodNo, &a.At, &a.Code, &a.Attempts); e != nil {
			return
		}
		out.Attempts = append(out.Attempts, a)
	}
	return out, rows.Err()
}

var periodNumber = regexp.MustCompile(`^[0-9]{1,40}$`)
var credentialName = regexp.MustCompile(`^DRAW_FEED_[A-Z0-9_]{1,109}$`)

func ValidateAutomation(p AutomationPlan) error {
	if p.Mapping != nil && p.Mapping.Schedule == "game" {
		if p.AnchorDrawAt != nil || p.AnchorPeriodNo != nil || p.IntervalSeconds != nil || p.OpenBeforeSeconds != nil || p.CloseBeforeSeconds != 0 {
			return ErrInvalid
		}
	} else if p.OpenBeforeSeconds == nil || *p.OpenBeforeSeconds <= p.CloseBeforeSeconds || *p.OpenBeforeSeconds > 31536000 {
		return ErrInvalid
	}
	if p.SourceType != "mapped" || p.Mapping == nil || p.Mapping.Validate() != nil {
		return ErrInvalid
	}
	u, e := url.Parse(p.SourceURL)
	if e != nil || u.Host == "" || u.User != nil || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http") || utf8.RuneCountInString(p.SourceURL) > 2048 || p.CredentialRef != "" && !credentialName.MatchString(p.CredentialRef) {
		return ErrInvalid
	}
	if p.Mapping.Schedule == "game" {
		return nil
	} else if p.Mapping.Schedule == "source" {
		if p.AnchorDrawAt != nil || p.AnchorPeriodNo != nil || p.IntervalSeconds != nil || p.CloseBeforeSeconds < 0 || p.CloseBeforeSeconds > 86400 {
			return ErrInvalid
		}
	} else if p.AnchorDrawAt == nil || p.AnchorDrawAt.IsZero() || p.AnchorPeriodNo == nil || !periodNumber.MatchString(*p.AnchorPeriodNo) || p.IntervalSeconds == nil || *p.IntervalSeconds < 10 || *p.IntervalSeconds > 31536000 || p.CloseBeforeSeconds < 0 || p.CloseBeforeSeconds >= *p.IntervalSeconds || *p.OpenBeforeSeconds > *p.IntervalSeconds {
		return ErrInvalid
	}

	return nil
}
func (s Service) publishAutomation(ctx context.Context, tx pgx.Tx, a access.Account, game string, in AutomationInput, source DrawSource, meta points.Metadata) (AutomationPlan, error) {
	if !Allowed(a, "", "write") {
		return AutomationPlan{}, ErrDenied
	}
	if ValidateAutomation(in.AutomationPlan) != nil || (in.Reason != "" && !textOK(in.Reason, 500)) {
		return AutomationPlan{}, ErrInvalid
	}
	g, e := lockedGame(ctx, tx, game)
	if e != nil {
		return AutomationPlan{}, e
	}
	if validateMappingModel(in.AutomationPlan, g.Model) != nil {
		return AutomationPlan{}, ErrInvalid
	}
	var configured bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM configured_rule_heads h JOIN configured_rule_versions v ON v.game_id=h.game_id AND v.id=h.version_id WHERE h.game_id=$1 AND v.status='published')`, game).Scan(&configured); e != nil {
		return AutomationPlan{}, e
	}
	if !configured && in.Enabled {
		return AutomationPlan{}, ErrInvalid
	}
	if in.Mapping.Schedule == "game" {
		var configured bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM platform_draw_schedule_settings WHERE game_id=$1)`, game).Scan(&configured); e != nil {
			return AutomationPlan{}, e
		}
		if !configured {
			return AutomationPlan{}, ErrInvalid
		}
	}
	var current int64
	if e = tx.QueryRow(ctx, `SELECT coalesce((SELECT version FROM platform_automation_settings WHERE game_id=$1),0)`, game).Scan(&current); e != nil {
		return AutomationPlan{}, e
	}
	if in.Version != current {
		return AutomationPlan{}, ErrVersionConflict
	}
	var before any
	if current > 0 {
		previous, err := scanAutomation(tx.QueryRow(ctx, `SELECT `+automationPlanFields+` FROM platform_automation_settings s JOIN platform_automation_versions v USING(game_id,version) WHERE s.game_id=$1`, game))
		if err != nil {
			return AutomationPlan{}, err
		}
		before = previous
	}
	// Published slots keep their schedule/source. New revisions start after every reserved slot.
	var last *time.Time
	if e = tx.QueryRow(ctx, `SELECT max(draw_at) FROM platform_game_periods WHERE game_id=$1 AND automation_version IS NOT NULL`, game).Scan(&last); e != nil {
		return AutomationPlan{}, e
	}
	if intervalPlan(in.AutomationPlan) && in.Enabled && last != nil && !in.AnchorDrawAt.After(*last) {
		return AutomationPlan{}, ErrConflict
	}
	if in.Enabled && intervalPlan(in.AutomationPlan) {
		for offset := int64(0); offset < 2; offset++ {
			slot, err := automationSlot(in.AutomationPlan, time.Now(), offset)
			if err != nil {
				return AutomationPlan{}, err
			}
			var exists bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM platform_game_periods WHERE game_id=$1 AND period_no=$2)`, game, slot.PeriodNo).Scan(&exists); err != nil {
				return AutomationPlan{}, err
			}
			if exists {
				return AutomationPlan{}, ErrConflict
			}
		}
	}
	p := in.AutomationPlan
	p.Version++
	_, e = tx.Exec(ctx, `INSERT INTO platform_automation_versions(game_id,version,enabled,anchor_draw_at,anchor_period_no,interval_seconds,open_before_seconds,close_before_seconds,source_url,credential_ref,created_by,source_type,mapping)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, game, p.Version, p.Enabled, p.AnchorDrawAt, p.AnchorPeriodNo, p.IntervalSeconds, p.OpenBeforeSeconds, p.CloseBeforeSeconds, p.SourceURL, p.CredentialRef, a.ID, p.SourceType, p.Mapping)
	if e != nil {
		return p, e
	}
	_, e = tx.Exec(ctx, `INSERT INTO platform_automation_settings(game_id,version)VALUES($1,$2) ON CONFLICT(game_id) DO UPDATE SET version=excluded.version`, game, p.Version)
	if e != nil {
		return p, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO platform_draw_source_activations(game_id,automation_version,source_id,source_revision) VALUES($1,$2,$3,$4)`, game, p.Version, source.ID, source.Revision); e != nil {
		return p, e
	}
	return p, record(ctx, tx, a, "", "platform_game.automation.publish", game, in.Reason, meta, before, p)
}
func automationSlot(p AutomationPlan, now time.Time, indexOffset int64) (Period, error) {
	if p.OpenBeforeSeconds == nil || *p.OpenBeforeSeconds <= p.CloseBeforeSeconds || !intervalPlan(p) || p.IntervalSeconds == nil || p.AnchorDrawAt == nil || p.AnchorPeriodNo == nil {
		return Period{}, ErrInvalid
	}
	interval := time.Duration(*p.IntervalSeconds) * time.Second
	var index int64
	if now.After(*p.AnchorDrawAt) {
		index = int64(now.Sub(*p.AnchorDrawAt) / interval)
		if p.AnchorDrawAt.Add(time.Duration(index) * interval).Before(now) {
			index++
		}
	}
	index += indexOffset
	n, _ := new(big.Int).SetString(*p.AnchorPeriodNo, 10)
	n.Add(n, big.NewInt(index))
	number := n.String()
	if len(number) < len(*p.AnchorPeriodNo) {
		number = strings.Repeat("0", len(*p.AnchorPeriodNo)-len(number)) + number
	}
	if len(number) > 100 {
		return Period{}, ErrInvalid
	}
	draw := p.AnchorDrawAt.Add(time.Duration(index) * interval)
	return Period{PeriodNo: number, BetStartAt: draw.Add(-time.Duration(*p.OpenBeforeSeconds) * time.Second), BetEndAt: draw.Add(-time.Duration(p.CloseBeforeSeconds) * time.Second), DrawAt: draw}, nil
}

func AutomationAllows(ctx context.Context, q queryer, brand, period string) (bool, error) {
	var allowed bool
	e := q.QueryRow(ctx, `SELECT shared_automation_accepts($1,$2)`, brand, period).Scan(&allowed)
	return allowed, e
}

func intervalPlan(p AutomationPlan) bool {
	return p.SourceType == "mapped" && p.Mapping != nil && p.Mapping.Schedule == "interval"
}
