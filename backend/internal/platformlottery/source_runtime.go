package platformlottery

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/rules"
)

// Feed errors carry a safe reason, never a URL, token, or response body.
type FeedError struct {
	Reason string
	Cause  error
}

func (e *FeedError) Error() string { return "draw source: " + e.Reason }
func (e *FeedError) Unwrap() error { return e.Cause }
func (e *FeedError) Is(target error) bool {
	return target == ErrInvalid && (e.Reason == "mapping_invalid" || e.Reason == "response_invalid" || e.Reason == "source_denied" || e.Reason == "credential_missing")
}

func feedFailureCode(err error) string {
	var failure *FeedError
	if errors.As(err, &failure) {
		if failure.Reason == "credential_missing" {
			return "credential_missing"
		}
		if failure.Reason == "mapping_invalid" || failure.Reason == "response_invalid" {
			return "invalid_result"
		}
		return "source_error"
	}
	if errors.Is(err, ErrInvalid) {
		return "invalid_result"
	}
	return "source_error"
}

func logFeedFailure(game, period string, err error) {
	reason := "network_error"
	var failure *FeedError
	if errors.As(err, &failure) {
		reason = failure.Reason
	} else if errors.Is(err, ErrInvalid) {
		reason = "response_invalid"
	}
	slog.Warn("draw source check failed", "game", game, "period", period, "reason", reason)
}

func validateSourceRuntime(sources AutomationSources, plan AutomationPlan) error {
	if plan.SourceType != "mapped" || plan.Mapping == nil || plan.Mapping.Validate() != nil {
		return &FeedError{Reason: "mapping_invalid"}
	}
	if !sources.Allows(plan.SourceURL) {
		return &FeedError{Reason: "source_denied"}
	}
	if plan.CredentialRef != "" && os.Getenv(plan.CredentialRef) == "" {
		return &FeedError{Reason: "credential_missing"}
	}
	return nil
}

// Includes frozen source versions still responsible for unfinished draws.
// probe performs read-only upstream requests; it never publishes or settles.
func (s Service) CheckDrawSources(ctx context.Context, sources AutomationSources, probe bool) error {
	rows, err := s.DB.Query(ctx, `SELECT g.code,g.model,`+automationPlanFields+` FROM platform_automation_versions v JOIN platform_games g ON g.id=v.game_id WHERE (v.enabled AND g.status='active' AND EXISTS(SELECT 1 FROM platform_automation_settings h WHERE h.game_id=v.game_id AND h.version=v.version)) OR (v.source_type='mapped' AND EXISTS(SELECT 1 FROM platform_game_periods p WHERE p.game_id=v.game_id AND p.automation_version=v.version AND p.result IS NULL)) ORDER BY g.code,v.version`)
	if err != nil {
		return err
	}
	type source struct {
		code  string
		model rules.Model
		plan  AutomationPlan
	}
	items := []source{}
	for rows.Next() {
		var item source
		p := &item.plan
		if err = rows.Scan(&item.code, &item.model, &p.Version, &p.Enabled, &p.AnchorDrawAt, &p.AnchorPeriodNo, &p.IntervalSeconds, &p.OpenBeforeSeconds, &p.CloseBeforeSeconds, &p.SourceURL, &p.CredentialRef, &p.SourceType, &p.Mapping); err != nil {
			rows.Close()
			return err
		}
		items = append(items, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	// Validate every configuration before issuing any network requests.
	for _, item := range items {
		if err = validateSourceRuntime(sources, item.plan); err != nil {
			return fmt.Errorf("game %s source version %d: %w (check PLATFORM_DRAW_ALLOWED_ORIGINS and source credentials)", item.code, item.plan.Version, err)
		}
	}
	if !probe {
		return nil
	}
	for _, item := range items {
		var period string
		if item.plan.Mapping.PeriodQuery != "" {
			err = s.DB.QueryRow(ctx, `SELECT period_no FROM platform_game_periods p JOIN platform_games g ON g.id=p.game_id WHERE g.code=$1 AND p.automation_version=$2 AND p.draw_at<=clock_timestamp() ORDER BY p.draw_at DESC LIMIT 1`, item.code, item.plan.Version).Scan(&period)
			if err != nil {
				return fmt.Errorf("game %s: no elapsed period available for source probe: %w", item.code, err)
			}
		}
		reading, err := FetchMappedFeed(ctx, sources, item.plan, period, item.code)
		if err != nil {
			logFeedFailure(item.code, period, err)
			return fmt.Errorf("game %s source version %d: %w", item.code, item.plan.Version, err)
		}
		observed := ""
		if reading.Completed != nil {
			c := reading.Completed
			if rules.ValidateDraw(item.model, c.Result) != nil || c.DrawnAt.After(time.Now().Add(10*time.Second)) {
				return fmt.Errorf("game %s: source probe returned invalid result", item.code)
			}
			observed = c.PeriodNo
		}
		slog.Info("draw source probe passed", "game", item.code, "version", item.plan.Version, "observed_period", observed, "waiting_result", reading.Completed == nil)
	}
	return nil
}
