package rulebook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/drawfeed"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

const (
	drawCollectLimit = 5
	drawClaimTTL     = 30 * time.Second
	drawPollBackoff  = 30 * time.Second
)

type drawClaim struct {
	brandID       string
	gameID        string
	periodID      string
	token         string
	sourceSetID   string
	periodVersion int64
	game          Game
	period        Period
	sourceSet     SourceSet
	previous      *rules.Draw
}

// CollectDraws claims and processes at most five due periods. Each period is
// isolated so one feed or database failure does not prevent later claims.
func (s Store) CollectDraws(ctx context.Context, resolver drawfeed.Resolver) (int, error) {
	if ctx == nil || s.DB == nil {
		return 0, ErrInvalid
	}
	rows, err := s.DB.Query(ctx, `SELECT p.brand_id::text,p.id::text
		FROM periods p
		JOIN games g ON g.id=p.game_id AND g.brand_id=p.brand_id
		JOIN draw_source_sets ds ON ds.id=g.draw_source_set_id
		WHERE p.status='waiting_draw' AND p.draw_result_id IS NULL
		  AND (p.draw_claim_until IS NULL OR p.draw_claim_until<=clock_timestamp())
		  AND (p.draw_next_poll_at IS NULL OR p.draw_next_poll_at<=clock_timestamp())
		  AND EXISTS (SELECT 1 FROM jsonb_array_elements(ds.sources) src
		              WHERE src->>'enabled'='true' AND src->>'type' IN ('api','dom'))
		ORDER BY p.draw_at,p.id LIMIT $1`, drawCollectLimit)
	if err != nil {
		return 0, err
	}
	type key struct{ brand, period string }
	keys := make([]key, 0, drawCollectLimit)
	for rows.Next() {
		var item key
		if err := rows.Scan(&item.brand, &item.period); err != nil {
			rows.Close()
			return 0, err
		}
		keys = append(keys, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}

	accepted := 0
	var errs []error
	for _, item := range keys {
		if err := ctx.Err(); err != nil {
			return accepted, errors.Join(append(errs, err)...)
		}
		claim, claimed, err := s.claimDraw(ctx, item.brand, item.period)
		if err != nil {
			errs = append(errs, fmt.Errorf("claim period %s: %w", item.period, err))
			continue
		}
		if !claimed {
			continue
		}
		n, err := s.collectClaim(ctx, resolver, claim)
		if n > 0 {
			accepted += n
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("collect period %s: %w", item.period, err))
		}
	}
	return accepted, errors.Join(errs...)
}

func (s Store) claimDraw(ctx context.Context, brand, periodID string) (drawClaim, bool, error) {
	var out drawClaim
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return out, false, err
	}
	defer rollbackDrawTx(ctx, tx)
	g, p, err := lockDrawPeriod(ctx, tx, brand, periodID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return out, false, nil
		}
		return out, false, err
	}
	var resultID, claimToken string
	var claimUntil, nextPoll *time.Time
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT coalesce(draw_result_id::text,''),coalesce(draw_claim_token::text,''),draw_claim_until,draw_next_poll_at,clock_timestamp()
		FROM periods WHERE id=$1`, p.ID).Scan(&resultID, &claimToken, &claimUntil, &nextPoll, &now); err != nil {
		return out, false, err
	}
	if p.Status != "waiting_draw" || resultID != "" || claimToken != "" && claimUntil != nil && claimUntil.After(now) || nextPoll != nil && nextPoll.After(now) {
		return out, false, nil
	}
	set, err := scanSourceSet(tx.QueryRow(ctx, `SELECT `+sourceSetFields+` FROM games g JOIN draw_source_sets s ON s.id=g.draw_source_set_id WHERE g.id=$1`, g.ID))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return out, false, nil
		}
		return out, false, err
	}
	if set.ID == "" || set.GameID != g.ID || set.BrandID != brand {
		return out, false, nil
	}
	feedSources := drawfeed.FeedSources(set.Sources)
	hasEnabled := false
	for _, source := range feedSources {
		if source.Enabled && (source.Type == "api" || source.Type == "dom") {
			hasEnabled = true
			break
		}
	}
	if !hasEnabled {
		return out, false, nil
	}
	previous, err := previousDraw(ctx, tx, p)
	if err != nil {
		return out, false, err
	}
	token := ids.New()
	if _, err = tx.Exec(ctx, `UPDATE periods SET draw_claim_token=$2,draw_claim_until=clock_timestamp()+($3 * interval '1 second')
		WHERE id=$1 AND status='waiting_draw' AND draw_result_id IS NULL`, p.ID, token, int64(drawClaimTTL/time.Second)); err != nil {
		return out, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return out, false, err
	}
	out = drawClaim{brandID: brand, gameID: g.ID, periodID: p.ID, token: token, sourceSetID: set.ID, periodVersion: p.Version, game: g, period: p, sourceSet: set, previous: previous}
	return out, true, nil
}

func (s Store) collectClaim(ctx context.Context, resolver drawfeed.Resolver, claim drawClaim) (int, error) {
	request := drawfeed.Request{PeriodNo: claim.period.PeriodNo, Model: claim.game.Model, Previous: claim.previous}
	originalHook := resolver.BeforeAttempt
	originalCandidateCheck := resolver.CandidateCheck
	resolver.BeforeAttempt = func(hookCtx context.Context, source drawfeed.Source) error {
		if originalHook != nil {
			if err := originalHook(hookCtx, source); err != nil {
				return err
			}
		}
		var valid bool
		err := s.DB.QueryRow(hookCtx, `SELECT EXISTS(
			SELECT 1 FROM periods p JOIN games g ON g.id=p.game_id AND g.brand_id=p.brand_id
			WHERE p.id=$1 AND p.brand_id=$2 AND p.game_id=$3 AND p.status='waiting_draw'
			  AND p.draw_result_id IS NULL AND p.version=$4 AND p.draw_claim_token=$5
			  AND p.draw_claim_until>clock_timestamp() AND g.draw_source_set_id=$6
			  AND EXISTS (SELECT 1 FROM jsonb_array_elements((SELECT sources FROM draw_source_sets WHERE id=$6)) src
			              WHERE src->>'id'=$7 AND src->>'enabled'='true' AND src->>'type' IN ('api','dom'))
		)`, claim.periodID, claim.brandID, claim.gameID, claim.periodVersion, claim.token, claim.sourceSetID, source.ID).Scan(&valid)
		if err != nil {
			return err
		}
		if !valid {
			return errDrawSuperseded
		}
		return nil
	}
	resolver.CandidateCheck = func(checkCtx context.Context, candidate drawfeed.Candidate) error {
		if originalCandidateCheck != nil {
			if err := originalCandidateCheck(checkCtx, candidate); err != nil {
				return err
			}
		}
		var now time.Time
		if err := s.DB.QueryRow(checkCtx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
			return err
		}
		if candidate.DrawnAt.Before(claim.period.DrawAt) || candidate.DrawnAt.After(now) {
			return drawfeed.ErrInvalid
		}
		return nil
	}

	result, resolveErr := resolver.Resolve(ctx, request, drawfeed.FeedSources(claim.sourceSet.Sources))
	status := "failed"
	if resolveErr == nil {
		status = "accepted"
	} else if errors.Is(resolveErr, drawfeed.ErrNoData) {
		status = "no_data"
	}
	attempts := safeDrawAttempts(result.Attempts, claim.sourceSet.Sources)

	finalCtx := ctx
	cleanupCancel := func() {}
	if ctx.Err() != nil {
		finalCtx, cleanupCancel = context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	}
	defer cleanupCancel()
	n, finalErr := s.finishClaim(finalCtx, claim, status, attempts, result, resolveErr)
	if finalErr != nil {
		if errors.Is(finalErr, errDrawSuperseded) {
			return n, nil
		}
		return n, errors.Join(resolveErr, finalErr)
	}
	if errors.Is(resolveErr, errDrawSuperseded) {
		return n, nil
	}
	if expectedDrawFeedError(resolveErr) {
		return n, nil
	}
	return n, resolveErr
}

func expectedDrawFeedError(err error) bool {
	return errors.Is(err, drawfeed.ErrNoData) || errors.Is(err, drawfeed.ErrTimeout) || errors.Is(err, drawfeed.ErrAbnormal) || errors.Is(err, drawfeed.ErrInvalid) || errors.Is(err, ErrDrawAbnormal) || errors.Is(err, ErrInvalid) || errors.Is(err, ErrState) || errors.Is(err, ErrVersion)
}

func rollbackDrawTx(ctx context.Context, tx pgx.Tx) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	_ = tx.Rollback(cleanupCtx)
}

var errDrawSuperseded = errors.New("draw claim superseded")

func (s Store) finishClaim(ctx context.Context, claim drawClaim, status string, attempts []drawfeed.Attempt, result drawfeed.Result, resolveErr error) (int, error) {
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer rollbackDrawTx(ctx, tx)
	g, p, lockErr := lockDrawPeriod(ctx, tx, claim.brandID, claim.periodID)
	if lockErr != nil && !errors.Is(lockErr, ErrNotFound) {
		return 0, lockErr
	}
	var sourceSetID string
	var currentResult, currentToken string
	var claimUntil *time.Time
	var now time.Time
	if lockErr == nil {
		if err := tx.QueryRow(ctx, `SELECT coalesce(g.draw_source_set_id::text,''),coalesce(p.draw_result_id::text,''),coalesce(p.draw_claim_token::text,''),p.draw_claim_until,clock_timestamp()
			FROM games g JOIN periods p ON p.game_id=g.id WHERE p.id=$1`, claim.periodID).Scan(&sourceSetID, &currentResult, &currentToken, &claimUntil, &now); err != nil {
			return 0, err
		}
	}
	current := lockErr == nil && p.Status == "waiting_draw" && currentResult == "" && currentToken == claim.token && claimUntil != nil && claimUntil.After(now) && sourceSetID == claim.sourceSetID && p.Version == claim.periodVersion && g.ID == claim.gameID
	if !current {
		status = "discarded"
		if currentToken == claim.token && p.Status == "waiting_draw" && currentResult == "" {
			_, err = tx.Exec(ctx, `UPDATE periods SET draw_claim_token=NULL,draw_claim_until=NULL WHERE id=$1 AND draw_claim_token=$2`, claim.periodID, claim.token)
			if err != nil {
				return 0, err
			}
		}
	} else if status == "accepted" && resolveErr == nil {
		kind := ""
		for _, source := range claim.sourceSet.Sources {
			if source.ID == result.SourceID {
				kind = source.Type
				break
			}
		}
		if kind != "api" && kind != "dom" {
			status = "failed"
			resolveErr = drawfeed.ErrInvalid
			_, err = tx.Exec(ctx, `UPDATE periods SET draw_claim_token=NULL,draw_claim_until=NULL,draw_next_poll_at=clock_timestamp()+($3 * interval '1 second') WHERE id=$1 AND draw_claim_token=$2`, claim.periodID, claim.token, int64(drawPollBackoff/time.Second))
			if err != nil {
				return 0, err
			}
		} else {
			_, err = appendDrawResult(ctx, tx, g, p, result.SourceID, kind, result.Candidate, "", "external draw collection", points.Metadata{})
			if err != nil {
				if errors.Is(err, ErrDrawAbnormal) || errors.Is(err, ErrInvalid) || errors.Is(err, ErrState) || errors.Is(err, ErrVersion) {
					status = "failed"
					resolveErr = err
					_, err = tx.Exec(ctx, `UPDATE periods SET draw_claim_token=NULL,draw_claim_until=NULL,draw_next_poll_at=clock_timestamp()+($3 * interval '1 second') WHERE id=$1 AND draw_claim_token=$2`, claim.periodID, claim.token, int64(drawPollBackoff/time.Second))
					if err != nil {
						return 0, err
					}
				} else {
					return 0, err
				}
			}
		}
	} else {
		_, err = tx.Exec(ctx, `UPDATE periods SET draw_claim_token=NULL,draw_claim_until=NULL,draw_next_poll_at=clock_timestamp()+($3 * interval '1 second')
			WHERE id=$1 AND draw_claim_token=$2`, claim.periodID, claim.token, int64(drawPollBackoff/time.Second))
		if err != nil {
			return 0, err
		}
	}
	if err := insertDrawAttemptBatch(ctx, tx, claim, status, attempts); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	if status == "accepted" {
		return 1, nil
	}
	if status == "discarded" {
		return 0, nil
	}
	return 0, nil
}

func insertDrawAttemptBatch(ctx context.Context, tx pgx.Tx, claim drawClaim, status string, attempts []drawfeed.Attempt) error {
	if status != "accepted" && status != "no_data" && status != "failed" && status != "discarded" {
		status = "failed"
	}
	raw, err := json.Marshal(attempts)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO draw_attempt_batches(id,brand_id,game_id,period_id,source_set_id,observed_period_version,status,attempts)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, ids.New(), claim.brandID, claim.gameID, claim.periodID, claim.sourceSetID, claim.periodVersion, status, raw)
	return err
}

func safeDrawAttempts(got []drawfeed.Attempt, configs []drawfeed.SourceConfig) []drawfeed.Attempt {
	out := make([]drawfeed.Attempt, 0, len(configs))
	byID := make(map[string]bool, len(configs))
	for _, source := range configs {
		byID[source.ID] = true
	}
	for _, attempt := range got {
		if len(out) >= 16 || !byID[attempt.SourceID] {
			continue
		}
		status := attempt.Status
		if status != "success" && status != "no_data" && status != "timeout" && status != "abnormal" && status != "error" {
			status = "error"
		}
		code := attempt.Code
		switch code {
		case "ok", "no_data", "timeout", "invalid_candidate", "abnormal", "adapter_unavailable", "fetch_error", "candidate_check_error", "cancelled":
		default:
			code = "fetch_error"
		}
		out = append(out, drawfeed.Attempt{SourceID: attempt.SourceID, Status: status, Code: code})
	}
	return out
}
