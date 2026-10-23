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
)

type AutomationPlan struct {
	Mapping            *FeedMapping `json:"mapping,omitempty"`
	SourceType         string       `json:"source_type"`
	Version            int64        `json:"version"`
	Enabled            bool         `json:"enabled"`
	AnchorDrawAt       *time.Time   `json:"anchor_draw_at"`
	AnchorPeriodNo     *string      `json:"anchor_period_no"`
	IntervalSeconds    *int         `json:"interval_seconds"`
	CloseBeforeSeconds int          `json:"close_before_seconds"`
	SourceURL          string       `json:"source_url"`
	CredentialRef      string       `json:"credential_ref"`
}
type BrandAutomation struct {
	Version            int64 `json:"version"`
	RuleVersion        int64 `json:"rule_version"`
	Enabled            bool  `json:"enabled"`
	CloseBeforeSeconds int   `json:"close_before_seconds"`
}
type AutomationAttempt struct {
	PeriodNo string    `json:"period_no"`
	At       time.Time `json:"at"`
	Code     string    `json:"code"`
	Attempts int       `json:"attempts"`
}
type AutomationConfig struct {
	Plan           *AutomationPlan     `json:"plan"`
	Brand          *BrandAutomation    `json:"brand"`
	Attempts       []AutomationAttempt `json:"attempts"`
	SourceCheck    *SourceCheck        `json:"source_check"`
	AllowedOrigins []string            `json:"allowed_origins"`
}
type AutomationInput struct {
	AutomationPlan
	Reason string `json:"reason"`
}
type BrandAutomationInput struct {
	BrandAutomation
	Reason string `json:"reason"`
}

const automationPlanFields = `v.version,v.enabled,v.anchor_draw_at,v.anchor_period_no,v.interval_seconds,v.close_before_seconds,v.source_url,v.credential_ref,v.source_type,v.mapping`

func scanAutomation(row pgx.Row) (p AutomationPlan, e error) {
	e = row.Scan(&p.Version, &p.Enabled, &p.AnchorDrawAt, &p.AnchorPeriodNo, &p.IntervalSeconds, &p.CloseBeforeSeconds, &p.SourceURL, &p.CredentialRef, &p.SourceType, &p.Mapping)
	if p.AnchorDrawAt != nil {
		v := p.AnchorDrawAt.UTC()
		p.AnchorDrawAt = &v
	}
	return
}
func (s Service) AutomationConfig(ctx context.Context, brand, game string) (out AutomationConfig, e error) {
	out.Attempts = []AutomationAttempt{}
	out.AllowedOrigins = []string{}
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
	if out.Plan != nil && (out.Plan.SourceType == "hk_marksix_6hch" || out.Plan.SourceType == "mapped" && out.Plan.Mapping != nil && out.Plan.Mapping.Schedule == "source") {
		out.SourceCheck, e = s.ReadSourceCheck(ctx, game, out.Plan.Version)
		if e != nil {
			return out, e
		}
	}
	if brand != "" {
		b := BrandAutomation{}
		err = s.DB.QueryRow(ctx, `SELECT version,rule_version,enabled,close_before_seconds FROM brand_automation_settings WHERE brand_id=$1 AND game_id=$2`, brand, game).Scan(&b.Version, &b.RuleVersion, &b.Enabled, &b.CloseBeforeSeconds)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return out, err
		}
		if err == nil {
			out.Brand = &b
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
	if p.SourceType != "mapped" || p.Mapping == nil || p.Mapping.Validate() != nil {
		return ErrInvalid
	}
	u, e := url.Parse(p.SourceURL)
	if e != nil || u.Host == "" || u.User != nil || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http") || len(p.SourceURL) > 2048 || p.CredentialRef != "" && !credentialName.MatchString(p.CredentialRef) {
		return ErrInvalid
	}
	if p.Mapping.Schedule == "source" {
		if p.AnchorDrawAt != nil || p.AnchorPeriodNo != nil || p.IntervalSeconds != nil || p.CloseBeforeSeconds < 0 || p.CloseBeforeSeconds > 86400 {
			return ErrInvalid
		}
	} else if p.AnchorDrawAt == nil || p.AnchorDrawAt.IsZero() || p.AnchorPeriodNo == nil || !periodNumber.MatchString(*p.AnchorPeriodNo) || p.IntervalSeconds == nil || *p.IntervalSeconds < 10 || *p.IntervalSeconds > 31536000 || p.CloseBeforeSeconds < 0 || p.CloseBeforeSeconds >= *p.IntervalSeconds {
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
	if !configured {
		return AutomationPlan{}, ErrInvalid
	}
	var current int64
	if e = tx.QueryRow(ctx, `SELECT coalesce((SELECT version FROM platform_automation_settings WHERE game_id=$1),0)`, game).Scan(&current); e != nil {
		return AutomationPlan{}, e
	}
	if in.Version != current {
		return AutomationPlan{}, ErrVersionConflict
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
	_, e = tx.Exec(ctx, `INSERT INTO platform_automation_versions(game_id,version,enabled,anchor_draw_at,anchor_period_no,interval_seconds,close_before_seconds,source_url,credential_ref,created_by,source_type,mapping)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, game, p.Version, p.Enabled, p.AnchorDrawAt, p.AnchorPeriodNo, p.IntervalSeconds, p.CloseBeforeSeconds, p.SourceURL, p.CredentialRef, a.ID, p.SourceType, p.Mapping)
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
	return p, record(ctx, tx, a, "", "platform_game.automation.publish", game, in.Reason, meta, p)
}
func (s Service) WriteBrandAutomation(ctx context.Context, tx pgx.Tx, a access.Account, brand, game string, in BrandAutomationInput, meta points.Metadata) (BrandAutomation, error) {
	if !Allowed(a, brand, "sell") {
		return BrandAutomation{}, ErrDenied
	}
	if in.Reason != "" && !textOK(in.Reason, 500) {
		return BrandAutomation{}, ErrInvalid
	}
	if _, e := lockedGame(ctx, tx, game); e != nil {
		return BrandAutomation{}, e
	}
	var connected bool
	if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM brand_platform_games WHERE brand_id=$1 AND game_id=$2)`, brand, game).Scan(&connected); e != nil {
		return BrandAutomation{}, e
	}
	if !connected {
		return BrandAutomation{}, ErrNotFound
	}
	p, e := scanAutomation(tx.QueryRow(ctx, `SELECT `+automationPlanFields+` FROM platform_automation_settings s JOIN platform_automation_versions v USING(game_id,version) WHERE s.game_id=$1`, game))
	if errors.Is(e, pgx.ErrNoRows) {
		return BrandAutomation{}, ErrNotFound
	}
	if e != nil {
		return BrandAutomation{}, e
	}
	if in.RuleVersion != p.Version {
		return BrandAutomation{}, ErrVersionConflict
	}
	if in.CloseBeforeSeconds < p.CloseBeforeSeconds || intervalPlan(p) && in.CloseBeforeSeconds >= *p.IntervalSeconds || !intervalPlan(p) && in.CloseBeforeSeconds > 86400 {
		return BrandAutomation{}, ErrInvalid
	}
	var current int64
	if e = tx.QueryRow(ctx, `SELECT coalesce((SELECT version FROM brand_automation_settings WHERE brand_id=$1 AND game_id=$2),0)`, brand, game).Scan(&current); e != nil {
		return BrandAutomation{}, e
	}
	if current != in.Version {
		return BrandAutomation{}, ErrVersionConflict
	}
	b := in.BrandAutomation
	b.Version++
	_, e = tx.Exec(ctx, `INSERT INTO brand_automation_settings(brand_id,game_id,version,rule_version,enabled,close_before_seconds)VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(brand_id,game_id) DO UPDATE SET version=excluded.version,rule_version=excluded.rule_version,enabled=excluded.enabled,close_before_seconds=excluded.close_before_seconds`, brand, game, b.Version, b.RuleVersion, b.Enabled, b.CloseBeforeSeconds)
	if e != nil {
		return b, e
	}
	if e = reserveBrandSeals(ctx, tx, game); e != nil {
		return b, e
	}
	return b, record(ctx, tx, a, brand, "platform_game.automation.sales", game, in.Reason, meta, b)
}
func automationSlot(p AutomationPlan, now time.Time, indexOffset int64) (Period, error) {
	if !intervalPlan(p) || p.IntervalSeconds == nil || p.AnchorDrawAt == nil || p.AnchorPeriodNo == nil {
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
	return Period{PeriodNo: number, BetStartAt: draw.Add(-interval), BetEndAt: draw.Add(-time.Duration(p.CloseBeforeSeconds) * time.Second), DrawAt: draw}, nil
}

// Read-only copy for a connected brand; never exposes source configuration to users.
func BrandPeriod(ctx context.Context, q queryer, brand string, p *Period) error {
	var automatic bool
	var enabled bool
	var before int
	e := q.QueryRow(ctx, `SELECT p.automation_version IS NOT NULL,coalesce(b.enabled AND seal.period_id IS NOT NULL,false),coalesce(seal.close_before_seconds,0) FROM platform_game_periods p LEFT JOIN platform_automation_versions v ON v.game_id=p.game_id AND v.version=p.automation_version LEFT JOIN brand_automation_settings b ON b.game_id=p.game_id AND b.brand_id=$2 LEFT JOIN brand_automation_periods seal ON seal.period_id=p.id AND seal.brand_id=$2 WHERE p.id=$1`, p.ID, brand).Scan(&automatic, &enabled, &before)
	if e != nil {
		return e
	}
	if automatic {
		p.BetEndAt = p.DrawAt.Add(-time.Duration(before) * time.Second)
		if !enabled {
			p.BetEndAt = p.BetStartAt
		}
	}
	return nil
}
func AutomationAllows(ctx context.Context, q queryer, brand, period string) (bool, error) {
	var allowed bool
	e := q.QueryRow(ctx, `SELECT shared_automation_accepts($1,$2)`, brand, period).Scan(&allowed)
	return allowed, e
}

func reserveBrandSeals(ctx context.Context, tx pgx.Tx, game string) error {
	_, e := tx.Exec(ctx, `INSERT INTO brand_automation_periods(brand_id,game_id,period_id,close_before_seconds)
 SELECT b.brand_id,p.game_id,p.id,greatest(v.close_before_seconds,b.close_before_seconds) FROM platform_game_periods p
 JOIN platform_automation_versions v ON v.game_id=p.game_id AND v.version=p.automation_version
 JOIN brand_automation_settings b ON b.game_id=p.game_id AND b.rule_version=p.automation_version
 WHERE p.game_id=$1 AND p.result IS NULL AND p.draw_at>clock_timestamp()
 ON CONFLICT(brand_id,period_id) DO NOTHING`, game)
	return e
}

func intervalPlan(p AutomationPlan) bool {
	return p.SourceType == "interval_json" || p.SourceType == "mapped" && p.Mapping != nil && p.Mapping.Schedule == "interval"
}
