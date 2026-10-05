package rulebook

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

type concurrencyVersionResult struct {
	version Version
	err     error
}

func concurrentVersions(t *testing.T, s Store, operations []func(context.Context, pgx.Tx) (Version, error)) []concurrencyVersionResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	start := make(chan struct{})
	type indexedResult struct {
		index int
		concurrencyVersionResult
	}
	results := make(chan indexedResult, len(operations))
	for index, operation := range operations {
		go func(index int, operation func(context.Context, pgx.Tx) (Version, error)) {
			<-start
			tx, err := s.DB.Begin(ctx)
			if err != nil {
				results <- indexedResult{index: index, concurrencyVersionResult: concurrencyVersionResult{err: err}}
				return
			}
			defer tx.Rollback(context.Background())
			version, err := operation(ctx, tx)
			if err == nil {
				err = tx.Commit(ctx)
			}
			results <- indexedResult{index: index, concurrencyVersionResult: concurrencyVersionResult{version: version, err: err}}
		}(index, operation)
	}
	close(start)
	out := make([]concurrencyVersionResult, len(operations))
	for range operations {
		select {
		case result := <-results:
			out[result.index] = result.concurrencyVersionResult
		case <-ctx.Done():
			t.Fatalf("concurrent rulebook transactions did not complete: %v", ctx.Err())
		}
	}
	return out
}

func concurrencyApprove(t *testing.T, s Store, reviewer access.Account, version Version) Version {
	t.Helper()
	transact(t, s.DB, func(tx pgx.Tx) error {
		var err error
		version, err = s.Review(context.Background(), tx, brand, reviewer, version.ID, version.Version, true, true, "independent approval", points.Metadata{RequestID: "concurrency-review"})
		return err
	})
	return version
}

func concurrencyPrepare(t *testing.T, s Store, author access.Account, version Version, validationCase rules.ValidationCase) Version {
	t.Helper()
	transact(t, s.DB, func(tx pgx.Tx) error {
		var err error
		version, err = s.Validate(context.Background(), tx, brand, author, version.ID, version.Version, []rules.ValidationCase{validationCase}, "validate current definition", points.Metadata{})
		return err
	})
	if version.Validation == nil || !version.Validation.Passed || version.Validation.DefinitionHash != version.DefinitionHash {
		t.Fatalf("validation did not pass for current definition: %+v", version)
	}
	transact(t, s.DB, func(tx pgx.Tx) error {
		var err error
		version, err = s.Submit(context.Background(), tx, brand, author, version.ID, version.Version, "submit validated definition", points.Metadata{})
		return err
	})
	return version
}

func concurrencyReadVersion(t *testing.T, s Store, id string) Version {
	t.Helper()
	version, err := s.GetVersion(context.Background(), brand, id)
	if err != nil {
		t.Fatal(err)
	}
	return version
}

func concurrencyReadPlay(t *testing.T, s Store, id string) Play {
	t.Helper()
	play, err := scanPlay(s.DB.QueryRow(context.Background(), `SELECT `+playFields+` FROM play_definitions WHERE brand_id=$1 AND id=$2`, brand, id))
	if err != nil {
		t.Fatal(err)
	}
	return play
}

func concurrencyReadGame(t *testing.T, s Store, id string) Game {
	t.Helper()
	game, err := scanGame(s.DB.QueryRow(context.Background(), `SELECT `+gameFields+` FROM games WHERE brand_id=$1 AND id=$2`, brand, id))
	if err != nil {
		t.Fatal(err)
	}
	return game
}

func concurrencyCount(t *testing.T, s Store, query string, args ...any) int {
	t.Helper()
	var count int
	if err := s.DB.QueryRow(context.Background(), query, args...).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func concurrencyDefinition(t *testing.T, s Store, id string) string {
	t.Helper()
	var definition string
	if err := s.DB.QueryRow(context.Background(), `SELECT definition::text FROM rule_versions WHERE brand_id=$1 AND id=$2`, brand, id).Scan(&definition); err != nil {
		t.Fatal(err)
	}
	return definition
}

func concurrencyOpenPeriod(t *testing.T, s Store, gameID, number string) Period {
	t.Helper()
	now := time.Now()
	var period Period
	transact(t, s.DB, func(tx pgx.Tx) error {
		var err error
		period, err = s.OpenPeriod(context.Background(), tx, brand, gameID, number, now.Add(-time.Minute), now.Add(5*time.Minute), now.Add(10*time.Minute))
		return err
	})
	return period
}

func TestConcurrentReviewsApprovePendingVersionExactlyOnce(t *testing.T) {
	s, creator, reviewer, _, play, definition := fixture(t)
	pending := draftReady(t, s, creator, play, definition, "next_period")
	const requests = 8
	operations := make([]func(context.Context, pgx.Tx) (Version, error), requests)
	for i := range operations {
		index := i
		operations[i] = func(ctx context.Context, tx pgx.Tx) (Version, error) {
			return s.Review(ctx, tx, brand, reviewer, pending.ID, pending.Version, true, true, "concurrent approval", points.Metadata{RequestID: fmt.Sprintf("review-%d", index)})
		}
	}
	winners, conflicts := 0, 0
	for _, result := range concurrentVersions(t, s, operations) {
		switch {
		case result.err == nil:
			winners++
			if result.version.Status != "approved" || result.version.Version != pending.Version+1 {
				t.Fatalf("unexpected winning review: %+v", result.version)
			}
		case errors.Is(result.err, ErrVersion):
			conflicts++
		default:
			t.Fatalf("concurrent review returned unexpected error: %v", result.err)
		}
	}
	if winners != 1 || conflicts != requests-1 {
		t.Fatalf("reviews: winners=%d conflicts=%d, want 1/%d", winners, conflicts, requests-1)
	}
	current := concurrencyReadVersion(t, s, pending.ID)
	if current.Status != "approved" || current.ReviewedBy != reviewer.ID || current.Version != pending.Version+1 || current.EffectiveSequence == nil || *current.EffectiveSequence != 1 {
		t.Fatalf("persisted approval mismatch: %+v", current)
	}
	if count := concurrencyCount(t, s, `SELECT count(*) FROM audit_logs WHERE resource_id=$1 AND action='rule.review.approve'`, pending.ID); count != 1 {
		t.Fatalf("approval audit count=%d, want 1", count)
	}
}

func TestConcurrentNewDraftsAllocateDistinctSerialVersionNumbers(t *testing.T) {
	s, creator, _, _, play, definition := fixture(t)
	const requests = 8
	operations := make([]func(context.Context, pgx.Tx) (Version, error), requests)
	for i := range operations {
		index := i
		operations[i] = func(ctx context.Context, tx pgx.Tx) (Version, error) {
			return s.CreateVersion(ctx, tx, brand, creator, play.ID, definition, "immediate", "concurrent draft", points.Metadata{RequestID: fmt.Sprintf("draft-%d", index)})
		}
	}
	numbers := make([]int, 0, requests)
	idsSeen := make(map[string]bool, requests)
	for _, result := range concurrentVersions(t, s, operations) {
		if result.err != nil {
			t.Fatalf("concurrent draft failed: %v", result.err)
		}
		if result.version.Status != "draft" || result.version.Version != 1 || idsSeen[result.version.ID] {
			t.Fatalf("invalid concurrent draft: %+v", result.version)
		}
		idsSeen[result.version.ID] = true
		numbers = append(numbers, int(result.version.VersionNo))
	}
	sort.Ints(numbers)
	for i, number := range numbers {
		if number != i+1 {
			t.Fatalf("draft version numbers=%v, want 1..%d", numbers, requests)
		}
	}
	if count := concurrencyCount(t, s, `SELECT count(*) FROM rule_versions WHERE brand_id=$1 AND play_id=$2`, brand, play.ID); count != requests {
		t.Fatalf("stored drafts=%d, want %d", count, requests)
	}
	if next := concurrencyCount(t, s, `SELECT next_version_no FROM play_definitions WHERE id=$1`, play.ID); next != requests+1 {
		t.Fatalf("next version number=%d, want %d", next, requests+1)
	}
	if count := concurrencyCount(t, s, `SELECT count(*) FROM audit_logs WHERE action='rule.draft.create' AND resource_id IN (SELECT id FROM rule_versions WHERE play_id=$1)`, play.ID); count != requests {
		t.Fatalf("draft audits=%d, want %d", count, requests)
	}
}

type concurrencyRaceResult struct {
	version Version
	period  Period
	err     error
}

// Hold the game lock in the first transaction until PostgreSQL confirms that
// the competing transaction is blocked on it; this exercises both race orders.
func concurrencyGameRace(t *testing.T, s Store, gameID string, first, second func(context.Context, pgx.Tx) concurrencyRaceResult) (concurrencyRaceResult, concurrencyRaceResult) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	var firstPID, one int
	if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&firstPID); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT 1 FROM games WHERE brand_id=$1 AND id=$2 FOR UPDATE`, brand, gameID).Scan(&one); err != nil {
		t.Fatal(err)
	}
	pids := make(chan int, 1)
	results := make(chan concurrencyRaceResult, 1)
	go func() {
		other, err := s.DB.Begin(ctx)
		if err != nil {
			results <- concurrencyRaceResult{err: err}
			return
		}
		defer other.Rollback(context.Background())
		var pid int
		if err := other.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
			results <- concurrencyRaceResult{err: err}
			return
		}
		pids <- pid
		result := second(ctx, other)
		if result.err == nil {
			result.err = other.Commit(ctx)
		}
		results <- result
	}()
	var secondPID int
	select {
	case secondPID = <-pids:
	case result := <-results:
		t.Fatalf("competing transaction failed before contention: %v", result.err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var blocked bool
		if err := s.DB.QueryRow(ctx, `SELECT $2::int = ANY(pg_blocking_pids($1::int))`, secondPID, firstPID).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			break
		}
		select {
		case result := <-results:
			t.Fatalf("competing operation did not wait on the game lock: %+v", result)
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal("competing operation never contended on the game lock")
		}
	}
	firstResult := first(ctx, tx)
	if firstResult.err != nil {
		t.Fatalf("first operation: %v", firstResult.err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case secondResult := <-results:
		if secondResult.err != nil {
			t.Fatalf("second operation: %v", secondResult.err)
		}
		return firstResult, secondResult
	case <-ctx.Done():
		t.Fatal("competing operation did not finish after game lock release")
		return concurrencyRaceResult{}, concurrencyRaceResult{}
	}
}

func TestNextPeriodApprovalAndOpenPeriodSerializeBothRaceOrders(t *testing.T) {
	for _, approvalFirst := range []bool{true, false} {
		t.Run(fmt.Sprintf("approval_first_%t", approvalFirst), func(t *testing.T) {
			s, creator, reviewer, game, play, definition := fixture(t)
			old := concurrencyApprove(t, s, reviewer, draftReady(t, s, creator, play, definition, "immediate"))
			var otherPlay Play
			transact(t, s.DB, func(tx pgx.Tx) error {
				var err error
				otherPlay, err = s.CreatePlay(context.Background(), tx, brand, creator, game.ID, "second_exact", "Second exact", "second play", points.Metadata{})
				return err
			})
			otherActive := concurrencyApprove(t, s, reviewer, draftReady(t, s, creator, otherPlay, definition, "immediate"))
			pending := draftReady(t, s, creator, play, definition, "next_period")
			now := time.Now()
			approve := func(ctx context.Context, tx pgx.Tx) concurrencyRaceResult {
				version, err := s.Review(ctx, tx, brand, reviewer, pending.ID, pending.Version, true, true, "race approval", points.Metadata{})
				return concurrencyRaceResult{version: version, err: err}
			}
			open := func(ctx context.Context, tx pgx.Tx) concurrencyRaceResult {
				period, err := s.OpenPeriod(ctx, tx, brand, game.ID, "race-period", now.Add(-time.Minute), now.Add(5*time.Minute), now.Add(10*time.Minute))
				return concurrencyRaceResult{period: period, err: err}
			}
			var period Period
			if approvalFirst {
				_, opened := concurrencyGameRace(t, s, game.ID, approve, open)
				period = opened.period
			} else {
				opened, _ := concurrencyGameRace(t, s, game.ID, open, approve)
				period = opened.period
			}
			current := concurrencyReadVersion(t, s, pending.ID)
			activeID := old.ID
			if approvalFirst {
				activeID = pending.ID
				if current.Status != "active" || current.EffectiveSequence == nil || *current.EffectiveSequence != 1 || current.EffectivePeriodID != period.ID || current.EffectiveAt == nil || current.Version != pending.Version+2 {
					t.Fatalf("approved-before-open version state torn: %+v", current)
				}
				if oldState := concurrencyReadVersion(t, s, old.ID); oldState.Status != "expired" {
					t.Fatalf("previous active was not expired: %+v", oldState)
				}
			} else {
				if current.Status != "approved" || current.EffectiveSequence == nil || *current.EffectiveSequence != 2 || current.EffectiveAt != nil || current.EffectivePeriodID != "" || current.Version != pending.Version+1 {
					t.Fatalf("approved-after-open version state torn: %+v", current)
				}
				if oldState := concurrencyReadVersion(t, s, old.ID); oldState.Status != "active" {
					t.Fatalf("opening before approval changed existing active: %+v", oldState)
				}
			}
			if storedGame := concurrencyReadGame(t, s, game.ID); storedGame.StartedSequence != 1 || storedGame.Version != game.Version+1 || period.Sequence != 1 {
				t.Fatalf("period/game sequence mismatch: game=%+v period=%+v", storedGame, period)
			}
			if storedPlay := concurrencyReadPlay(t, s, play.ID); storedPlay.ActiveVersionID != activeID {
				t.Fatalf("active pointer=%s, want %s", storedPlay.ActiveVersionID, activeID)
			}
			if storedPlay := concurrencyReadPlay(t, s, otherPlay.ID); storedPlay.ActiveVersionID != otherActive.ID {
				t.Fatalf("unrelated play pointer changed: %+v", storedPlay)
			}
			if count := concurrencyCount(t, s, `SELECT count(*) FROM period_rule_versions WHERE period_id=$1`, period.ID); count != 2 {
				t.Fatalf("partial period snapshot: %d rows, want 2", count)
			}
			for playID, versionID := range map[string]string{play.ID: activeID, otherPlay.ID: otherActive.ID} {
				if count := concurrencyCount(t, s, `SELECT count(*) FROM period_rule_versions WHERE period_id=$1 AND play_id=$2 AND rule_version_id=$3`, period.ID, playID, versionID); count != 1 {
					t.Fatalf("period snapshot for play %s does not match active version %s", playID, versionID)
				}
			}
		})
	}
}

func TestOpenPeriodRollbackRestoresQueuePointersSnapshotsSequenceAndAudits(t *testing.T) {
	s, creator, reviewer, game, play, definition := fixture(t)
	old := concurrencyApprove(t, s, reviewer, draftReady(t, s, creator, play, definition, "immediate"))
	queued := concurrencyApprove(t, s, reviewer, draftReady(t, s, creator, play, definition, "next_period"))
	beforeOld := concurrencyReadVersion(t, s, old.ID)
	beforeQueued := concurrencyReadVersion(t, s, queued.ID)
	beforePlay := concurrencyReadPlay(t, s, play.ID)
	beforeGame := concurrencyReadGame(t, s, game.ID)
	beforeAudits := concurrencyCount(t, s, `SELECT count(*) FROM audit_logs`)
	ctx := context.Background()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	now := time.Now()
	period, err := s.OpenPeriod(ctx, tx, brand, game.ID, "rolled-back-period", now.Add(-time.Minute), now.Add(5*time.Minute), now.Add(10*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	activated, err := scanVersion(tx.QueryRow(ctx, `SELECT `+versionFields+` FROM rule_versions WHERE id=$1`, queued.ID))
	if err != nil || activated.Status != "active" || activated.Version != queued.Version+1 || activated.EffectivePeriodID != period.ID {
		t.Fatalf("activation did not occur inside transaction: %+v err=%v", activated, err)
	}
	var activeID string
	var sequence int64
	var snapshots, audits int
	if err := tx.QueryRow(ctx, `SELECT active_version_id::text FROM play_definitions WHERE id=$1`, play.ID).Scan(&activeID); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT started_sequence FROM games WHERE id=$1`, game.ID).Scan(&sequence); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM period_rule_versions WHERE period_id=$1 AND rule_version_id=$2`, period.ID, queued.ID).Scan(&snapshots); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE request_id=$1 AND action='rule.period.activate'`, period.ID).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if activeID != queued.ID || sequence != 1 || snapshots != 1 || audits != 1 {
		t.Fatalf("in-transaction activation incomplete: pointer=%s sequence=%d snapshots=%d audits=%d", activeID, sequence, snapshots, audits)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if got := concurrencyReadVersion(t, s, old.ID); !reflect.DeepEqual(got, beforeOld) {
		t.Fatalf("rollback changed previous active: before=%+v after=%+v", beforeOld, got)
	}
	if got := concurrencyReadVersion(t, s, queued.ID); !reflect.DeepEqual(got, beforeQueued) {
		t.Fatalf("rollback changed queued version: before=%+v after=%+v", beforeQueued, got)
	}
	if got := concurrencyReadPlay(t, s, play.ID); !reflect.DeepEqual(got, beforePlay) {
		t.Fatalf("rollback changed active pointer/play version: before=%+v after=%+v", beforePlay, got)
	}
	if got := concurrencyReadGame(t, s, game.ID); !reflect.DeepEqual(got, beforeGame) {
		t.Fatalf("rollback changed started sequence/game version: before=%+v after=%+v", beforeGame, got)
	}
	if periods := concurrencyCount(t, s, `SELECT count(*) FROM periods WHERE game_id=$1`, game.ID); periods != 0 {
		t.Fatalf("rollback left %d periods", periods)
	}
	if snapshots := concurrencyCount(t, s, `SELECT count(*) FROM period_rule_versions WHERE game_id=$1`, game.ID); snapshots != 0 {
		t.Fatalf("rollback left %d period snapshots", snapshots)
	}
	if audits := concurrencyCount(t, s, `SELECT count(*) FROM audit_logs`); audits != beforeAudits {
		t.Fatalf("rollback left audit changes: before=%d after=%d", beforeAudits, audits)
	}
	retried := concurrencyOpenPeriod(t, s, game.ID, "rolled-back-period")
	if retried.Sequence != 1 || concurrencyReadVersion(t, s, queued.ID).EffectivePeriodID != retried.ID {
		t.Fatalf("queue could not activate after rollback: %+v", retried)
	}
}

func TestDefinitionEditorCannotReviewAnotherCreatorsEditedVersion(t *testing.T) {
	s, creator, editor, _, play, definition := fixture(t)
	var version Version
	transact(t, s.DB, func(tx pgx.Tx) error {
		var err error
		version, err = s.CreateVersion(context.Background(), tx, brand, creator, play.ID, definition, "immediate", "creator draft", points.Metadata{})
		return err
	})
	definition.PrizeTiers[0].Odds = "11"
	transact(t, s.DB, func(tx pgx.Tx) error {
		var err error
		version, err = s.UpdateVersion(context.Background(), tx, brand, editor, version.ID, version.Version, definition, "immediate", "editor changes odds", points.Metadata{})
		return err
	})
	validationCase := testCase()
	validationCase.ExpectedPrizePoints = ptr(points.Amount(11))
	version = concurrencyPrepare(t, s, editor, version, validationCase)
	if version.CreatedBy != creator.ID || !Allowed(editor, brand, "rule", "review") {
		t.Fatal("fixture must retain creator A and give editor B a full review grant")
	}
	tx, err := s.DB.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, reviewErr := s.Review(context.Background(), tx, brand, editor, version.ID, version.Version, true, true, "editor attempts approval", points.Metadata{})
	_ = tx.Rollback(context.Background())
	if !errors.Is(reviewErr, ErrDenied) {
		t.Fatalf("definition editor review error=%v, want ErrDenied", reviewErr)
	}
	if current := concurrencyReadVersion(t, s, version.ID); current.Status != "pending_review" || current.Version != version.Version || current.ReviewedBy != "" {
		t.Fatalf("denied editor review mutated version: %+v", current)
	}
	if contributors := concurrencyCount(t, s, `SELECT count(*) FROM rule_version_contributors WHERE rule_version_id=$1 AND admin_id IN ($2,$3)`, version.ID, creator.ID, editor.ID); contributors != 2 {
		t.Fatalf("creator/editor contributor count=%d, want 2", contributors)
	}
	if audits := concurrencyCount(t, s, `SELECT count(*) FROM audit_logs WHERE resource_id=$1 AND action='rule.review.approve'`, version.ID); audits != 0 {
		t.Fatalf("denied editor review left %d approval audits", audits)
	}
	independent := actor(ids.New())
	if _, err := s.DB.Exec(context.Background(), `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,'rule_independent_reviewer','test-only')`, independent.ID); err != nil {
		t.Fatal(err)
	}
	if approved := concurrencyApprove(t, s, independent, version); approved.Status != "active" || approved.ReviewedBy != independent.ID {
		t.Fatalf("independent reviewer could not approve edited definition: %+v", approved)
	}
}

func TestNormalSupersessionExpiresActiveWithoutChangingHistoricalDefinition(t *testing.T) {
	s, creator, reviewer, game, play, definition := fixture(t)
	old := concurrencyApprove(t, s, reviewer, draftReady(t, s, creator, play, definition, "immediate"))
	oldDefinition := concurrencyDefinition(t, s, old.ID)
	period := concurrencyOpenPeriod(t, s, game.ID, "historical-period")
	definition.Limits.MaxMultiplier = 999
	newVersion := concurrencyApprove(t, s, reviewer, draftReady(t, s, creator, play, definition, "immediate"))
	previous := concurrencyReadVersion(t, s, old.ID)
	if previous.Status != "expired" || previous.Version != old.Version+1 || previous.DefinitionHash != old.DefinitionHash || !reflect.DeepEqual(previous.Validation, old.Validation) || !reflect.DeepEqual(previous.EffectiveAt, old.EffectiveAt) || concurrencyDefinition(t, s, old.ID) != oldDefinition {
		t.Fatalf("supersession modified historical definition/evidence: %+v", previous)
	}
	if newVersion.Status != "active" || newVersion.SourceVersionID != "" || newVersion.DefinitionHash == old.DefinitionHash || concurrencyReadPlay(t, s, play.ID).ActiveVersionID != newVersion.ID {
		t.Fatalf("normal supersession did not activate a distinct version: %+v", newVersion)
	}
	if history := concurrencyCount(t, s, `SELECT count(*) FROM rule_versions WHERE play_id=$1`, play.ID); history != 2 {
		t.Fatalf("normal supersession removed history: %d versions", history)
	}
	if snapshot := concurrencyCount(t, s, `SELECT count(*) FROM period_rule_versions WHERE period_id=$1 AND play_id=$2 AND rule_version_id=$3`, period.ID, play.ID, old.ID); snapshot != 1 {
		t.Fatal("normal supersession changed the historical period snapshot")
	}
}

func TestRollbackCloneReviewsHistoricalDefinitionAndPreservesAllHistory(t *testing.T) {
	s, creator, reviewer, game, play, definition := fixture(t)
	historical := concurrencyApprove(t, s, reviewer, draftReady(t, s, creator, play, definition, "immediate"))
	historicalDefinition := concurrencyDefinition(t, s, historical.ID)
	period := concurrencyOpenPeriod(t, s, game.ID, "rollback-history-period")
	definition.Limits.MaxMultiplier = 999
	current := concurrencyApprove(t, s, reviewer, draftReady(t, s, creator, play, definition, "immediate"))
	currentDefinition := concurrencyDefinition(t, s, current.ID)
	var clone Version
	transact(t, s.DB, func(tx pgx.Tx) error {
		var err error
		clone, err = s.Clone(context.Background(), tx, brand, creator, historical.ID, "immediate", "restore historical rule", points.Metadata{})
		return err
	})
	if clone.Status != "draft" || clone.Validation != nil || clone.SourceVersionID != historical.ID || clone.VersionNo != 3 || clone.DefinitionHash != historical.DefinitionHash || concurrencyDefinition(t, s, clone.ID) != historicalDefinition || concurrencyReadPlay(t, s, play.ID).ActiveVersionID != current.ID {
		t.Fatalf("rollback clone was not an isolated historical draft: %+v", clone)
	}
	tx, err := s.DB.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, submitErr := s.Submit(context.Background(), tx, brand, creator, clone.ID, clone.Version, "unvalidated rollback", points.Metadata{})
	_ = tx.Rollback(context.Background())
	if !errors.Is(submitErr, ErrValidation) {
		t.Fatalf("unvalidated rollback submit error=%v, want ErrValidation", submitErr)
	}
	clone = concurrencyPrepare(t, s, creator, clone, testCase())
	if clone.Status != "pending_review" || concurrencyReadPlay(t, s, play.ID).ActiveVersionID != current.ID {
		t.Fatalf("rollback activated before independent review: %+v", clone)
	}
	clone = concurrencyApprove(t, s, reviewer, clone)
	if clone.Status != "active" || clone.ReviewedBy != reviewer.ID || clone.Validation == nil || !clone.Validation.Passed || clone.Validation.DefinitionHash != clone.DefinitionHash || clone.SourceVersionID != historical.ID || concurrencyReadPlay(t, s, play.ID).ActiveVersionID != clone.ID {
		t.Fatalf("reviewed rollback did not activate the new version: %+v", clone)
	}
	rolledBack := concurrencyReadVersion(t, s, current.ID)
	if rolledBack.Status != "rolled_back" || rolledBack.DefinitionHash != current.DefinitionHash || concurrencyDefinition(t, s, current.ID) != currentDefinition {
		t.Fatalf("displaced active did not retain its definition: %+v", rolledBack)
	}
	old := concurrencyReadVersion(t, s, historical.ID)
	if old.Status != "expired" || old.DefinitionHash != historical.DefinitionHash || concurrencyDefinition(t, s, historical.ID) != historicalDefinition || concurrencyDefinition(t, s, clone.ID) != historicalDefinition {
		t.Fatalf("rollback changed historical source: %+v", old)
	}
	if history := concurrencyCount(t, s, `SELECT count(*) FROM rule_versions WHERE play_id=$1`, play.ID); history != 3 {
		t.Fatalf("rollback removed history: %d versions, want 3", history)
	}
	if snapshot := concurrencyCount(t, s, `SELECT count(*) FROM period_rule_versions WHERE period_id=$1 AND play_id=$2 AND rule_version_id=$3`, period.ID, play.ID, historical.ID); snapshot != 1 {
		t.Fatal("rollback changed the historical period snapshot")
	}
	for _, action := range []string{"rule.rollback.draft", "rule.draft.validate", "rule.review.submit", "rule.review.approve"} {
		if count := concurrencyCount(t, s, `SELECT count(*) FROM audit_logs WHERE resource_id=$1 AND action=$2`, clone.ID, action); count != 1 {
			t.Fatalf("rollback action %s audit count=%d, want 1", action, count)
		}
	}
}
