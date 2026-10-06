package rulebook

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/betting"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

type periodGateSnapshot struct {
	game      Game
	play      Play
	active    Version
	queued    Version
	prior     Period
	periods   int
	snapshots int
	audits    int
}

func readPeriodGateSnapshot(t *testing.T, tx pgx.Tx, game, play, active, queued, prior string) periodGateSnapshot {
	t.Helper()
	ctx := context.Background()
	var out periodGateSnapshot
	var err error
	if out.game, err = scanGame(tx.QueryRow(ctx, `SELECT `+gameFields+` FROM games WHERE id=$1`, game)); err != nil {
		t.Fatal(err)
	}
	if out.play, err = scanPlay(tx.QueryRow(ctx, `SELECT `+playFields+` FROM play_definitions WHERE id=$1`, play)); err != nil {
		t.Fatal(err)
	}
	if out.active, err = scanVersion(tx.QueryRow(ctx, `SELECT `+versionFields+` FROM rule_versions WHERE id=$1`, active)); err != nil {
		t.Fatal(err)
	}
	if out.queued, err = scanVersion(tx.QueryRow(ctx, `SELECT `+versionFields+` FROM rule_versions WHERE id=$1`, queued)); err != nil {
		t.Fatal(err)
	}
	if out.prior, err = scanPeriod(tx.QueryRow(ctx, `SELECT `+periodFields+` FROM periods WHERE id=$1`, prior)); err != nil {
		t.Fatal(err)
	}
	for _, query := range []struct {
		sql  string
		dest *int
	}{
		{`SELECT count(*) FROM periods`, &out.periods},
		{`SELECT count(*) FROM period_rule_versions`, &out.snapshots},
		{`SELECT count(*) FROM audit_logs`, &out.audits},
	} {
		if err := tx.QueryRow(ctx, query.sql).Scan(query.dest); err != nil {
			t.Fatal(err)
		}
	}
	return out
}

func TestOpenPeriodBlocksUnfinishedPriorWithoutMutation(t *testing.T) {
	for _, status := range []string{"pending", "betting", "closed", "waiting_draw"} {
		t.Run(status, func(t *testing.T) {
			s, creator, reviewer, game, play, definition := fixture(t)
			active := concurrencyApprove(t, s, reviewer, draftReady(t, s, creator, play, definition, "immediate"))
			now := databaseNow(t, s)
			var prior Period
			if status == "pending" {
				// Pending reservations block by planned draw chronology, not merely
				// by the order in which the calendar rows were created.
				prior = insertPendingPeriod(t, s, game, "unfinished-prior", 1, now.Add(-time.Minute), now.Add(30*time.Minute), now.Add(time.Hour), "")
			} else {
				// Once begun, even a period with a later planned draw blocks the
				// candidate until its settlement or refunds have completed.
				transact(t, s.DB, func(tx pgx.Tx) error {
					var err error
					prior, err = s.OpenPeriod(context.Background(), tx, brand, game.ID, "unfinished-prior", now.Add(-time.Minute), now.Add(3*time.Hour), now.Add(4*time.Hour))
					return err
				})
				for _, next := range []string{"closed", "waiting_draw"} {
					if prior.Status == status {
						break
					}
					transact(t, s.DB, func(tx pgx.Tx) error {
						var err error
						prior, err = transitionPeriod(context.Background(), tx, prior, next, "prepare unfinished prior", now)
						return err
					})
				}
			}
			queued := concurrencyApprove(t, s, reviewer, draftReady(t, s, creator, play, definition, "next_period"))
			ctx := context.Background()
			tx, err := s.DB.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			before := readPeriodGateSnapshot(t, tx, game.ID, play.ID, active.ID, queued.ID, prior.ID)
			_, err = s.OpenPeriod(ctx, tx, brand, game.ID, "must-wait", now.Add(-time.Minute), now.Add(time.Hour), now.Add(2*time.Hour))
			if !errors.Is(err, ErrState) {
				t.Errorf("OpenPeriod with prior %s error=%v, want ErrState", status, err)
			}
			// Inspect the same transaction: rollback must not conceal a partial opening.
			after := readPeriodGateSnapshot(t, tx, game.ID, play.ID, active.ID, queued.ID, prior.ID)
			if !reflect.DeepEqual(after, before) {
				t.Errorf("blocked opening mutated state: sequence=%d->%d active=%s->%s queued=%s->%s periods=%d->%d snapshots=%d->%d audits=%d->%d",
					before.game.StartedSequence, after.game.StartedSequence, before.active.Status, after.active.Status,
					before.queued.Status, after.queued.Status, before.periods, after.periods, before.snapshots, after.snapshots, before.audits, after.audits)
			}
		})
	}
}

func TestTickLeavesNextPendingWhilePriorUnfinished(t *testing.T) {
	s, creator, reviewer, game, play, definition := fixture(t)
	active := concurrencyApprove(t, s, reviewer, draftReady(t, s, creator, play, definition, "immediate"))
	prior := concurrencyOpenPeriod(t, s, game.ID, "tick-unfinished-prior")
	queued := concurrencyApprove(t, s, reviewer, draftReady(t, s, creator, play, definition, "next_period"))
	now := databaseNow(t, s)
	next := insertPendingPeriod(t, s, game, "tick-must-wait", 2, now.Add(-time.Minute), now.Add(time.Hour), now.Add(2*time.Hour), "")
	ctx := context.Background()
	var before periodGateSnapshot
	transact(t, s.DB, func(tx pgx.Tx) error {
		before = readPeriodGateSnapshot(t, tx, game.ID, play.ID, active.ID, queued.ID, prior.ID)
		return nil
	})
	n, err := s.Tick(ctx)
	if err != nil || n != 0 {
		t.Errorf("Tick count=%d error=%v, want no transitions while prior unfinished", n, err)
	}
	transact(t, s.DB, func(tx pgx.Tx) error {
		after := readPeriodGateSnapshot(t, tx, game.ID, play.ID, active.ID, queued.ID, prior.ID)
		if !reflect.DeepEqual(after, before) {
			t.Errorf("blocked Tick mutated state: sequence=%d->%d queued=%s->%s snapshots=%d->%d audits=%d->%d",
				before.game.StartedSequence, after.game.StartedSequence, before.queued.Status, after.queued.Status,
				before.snapshots, after.snapshots, before.audits, after.audits)
		}
		got, err := scanPeriod(tx.QueryRow(ctx, `SELECT `+periodFields+` FROM periods WHERE id=$1`, next.ID))
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(got, next) {
			t.Errorf("next period changed while prior unfinished: before=%+v after=%+v", next, got)
		}
		return nil
	})
}

func TestOpenPeriodGateIsolatedBetweenGames(t *testing.T) {
	s, creator, _, blockedGame, _, _ := fixture(t)
	prior := concurrencyOpenPeriod(t, s, blockedGame.ID, "other-game-unfinished")
	// Compare stored timestamps on both sides of the operation; PostgreSQL stores
	// microseconds, while OpenPeriod's returned input timestamps may be finer.
	var err error
	prior, err = scanPeriod(s.DB.QueryRow(context.Background(), `SELECT `+periodFields+` FROM periods WHERE id=$1`, prior.ID))
	if err != nil {
		t.Fatal(err)
	}
	var independent Game
	transact(t, s.DB, func(tx pgx.Tx) error {
		var err error
		independent, err = s.CreateGame(context.Background(), tx, brand, creator, "independent_gate", "Independent gate", blockedGame.Model, blockedGame.Timezone, "create independent game", points.Metadata{})
		return err
	})
	opened := concurrencyOpenPeriod(t, s, independent.ID, "independent-opening")
	if opened.Sequence != 1 || concurrencyReadGame(t, s, independent.ID).StartedSequence != 1 {
		t.Fatalf("independent game did not open its first period: %+v", opened)
	}
	got, err := scanPeriod(s.DB.QueryRow(context.Background(), `SELECT `+periodFields+` FROM periods WHERE id=$1`, prior.ID))
	if err != nil || !reflect.DeepEqual(got, prior) {
		t.Fatalf("independent opening changed other game's prior period: got=%+v error=%v", got, err)
	}
}

func TestOpenPeriodAllowsCompletedCancellationOrSkippedUnusedPrior(t *testing.T) {
	for _, mode := range []string{"bet_cancelled", "judged_cancelled", "unused_pending"} {
		t.Run(mode, func(t *testing.T) {
			s, creator, reviewer, game, play, definition := fixture(t)
			active := concurrencyApprove(t, s, reviewer, draftReady(t, s, creator, play, definition, "immediate"))
			now := databaseNow(t, s)
			var prior Period
			if mode == "unused_pending" {
				prior = insertPendingPeriod(t, s, game, "unused-prior", 1, now.Add(-3*time.Hour), now.Add(-2*time.Hour), now.Add(-time.Hour), "")
			} else {
				prior = concurrencyOpenPeriod(t, s, game.ID, "empty-prior")
			}
			queued := concurrencyApprove(t, s, reviewer, draftReady(t, s, creator, play, definition, "next_period"))
			ctx := context.Background()
			if mode == "unused_pending" {
				transact(t, s.DB, func(tx pgx.Tx) error {
					var err error
					prior, err = transitionPeriod(ctx, tx, prior, "judged_cancelled", "unused opening window expired", now)
					return err
				})
				if n := concurrencyCount(t, s, `SELECT count(*) FROM period_cancellations WHERE period_id=$1`, prior.ID); n != 0 {
					t.Fatalf("skipped unused period unexpectedly has %d cancellation tasks", n)
				}
			} else {
				canceller := actor(creator.ID)
				canceller.Roles[0].Permissions = append(canceller.Roles[0].Permissions, access.Permission{Resource: "period", Action: "cancel", Scope: access.ScopeBrand})
				cause := "operator_cancel"
				if mode == "judged_cancelled" {
					cause = "no_result"
				}
				transact(t, s.DB, func(tx pgx.Tx) error {
					job, err := (betting.Service{DB: s.DB}).CancelPeriod(ctx, tx, brand, canceller, prior.ID,
						betting.CancelPeriodInput{Version: prior.Version, Mode: mode, Cause: cause, Reason: "finish empty prior cancellation"}, points.Metadata{})
					if err == nil && (job.State != "completed" || job.CompletedAt == nil || job.TotalCount != 0) {
						t.Fatalf("empty cancellation did not complete legally: %+v", job)
					}
					return err
				})
			}
			var err error
			prior, err = scanPeriod(s.DB.QueryRow(ctx, `SELECT `+periodFields+` FROM periods WHERE id=$1`, prior.ID))
			if err != nil {
				t.Fatal(err)
			}
			beforeGame := concurrencyReadGame(t, s, game.ID)
			next := concurrencyOpenPeriod(t, s, game.ID, "after-finished-cancellation")
			if next.Sequence != 2 {
				t.Fatalf("next calendar sequence=%d, want 2", next.Sequence)
			}
			current := concurrencyReadVersion(t, s, queued.ID)
			if current.Status != "active" || current.EffectivePeriodID != next.ID || concurrencyReadPlay(t, s, play.ID).ActiveVersionID != queued.ID || concurrencyReadVersion(t, s, active.ID).Status != "expired" {
				t.Fatalf("finished cancellation did not allow queued activation: %+v", current)
			}
			if got := concurrencyReadGame(t, s, game.ID); got.StartedSequence != beforeGame.StartedSequence+1 || got.Version != beforeGame.Version+1 {
				t.Fatalf("opening after cancellation advanced game incorrectly: before=%+v after=%+v", beforeGame, got)
			}
			if n := concurrencyCount(t, s, `SELECT count(*) FROM period_rule_versions WHERE period_id=$1 AND rule_version_id=$2`, next.ID, queued.ID); n != 1 {
				t.Fatalf("new period rule snapshots=%d, want 1", n)
			}
			got, err := scanPeriod(s.DB.QueryRow(ctx, `SELECT `+periodFields+` FROM periods WHERE id=$1`, prior.ID))
			if err != nil || !reflect.DeepEqual(got, prior) {
				t.Fatalf("opening rewrote cancelled prior history: got=%+v error=%v", got, err)
			}
		})
	}
}

func TestFuturePendingCreatedFirstDoesNotBlockEarlierDraw(t *testing.T) {
	for _, opening := range []string{"OpenPeriod", "Tick"} {
		t.Run(opening, func(t *testing.T) {
			s, creator, reviewer, game, play, definition := fixture(t)
			active := concurrencyApprove(t, s, reviewer, draftReady(t, s, creator, play, definition, "immediate"))
			queued := concurrencyApprove(t, s, reviewer, draftReady(t, s, creator, play, definition, "next_period"))
			now := databaseNow(t, s)
			future := insertPendingPeriod(t, s, game, "future-created-first", 1, now.Add(2*time.Hour), now.Add(3*time.Hour), now.Add(4*time.Hour), "")
			ctx := context.Background()
			var earlier Period
			if opening == "OpenPeriod" {
				transact(t, s.DB, func(tx pgx.Tx) error {
					var err error
					earlier, err = s.OpenPeriod(ctx, tx, brand, game.ID, "earlier-created-second", now.Add(-time.Minute), now.Add(time.Hour), now.Add(2*time.Hour))
					return err
				})
			} else {
				earlier = insertPendingPeriod(t, s, game, "earlier-created-second", 2, now.Add(-time.Minute), now.Add(time.Hour), now.Add(2*time.Hour), "")
				n, err := s.Tick(ctx)
				if err != nil || n != 1 {
					t.Fatalf("Tick with future pending reservation count=%d error=%v, want one opening", n, err)
				}
			}
			got, err := scanPeriod(s.DB.QueryRow(ctx, `SELECT `+periodFields+` FROM periods WHERE id=$1`, earlier.ID))
			if err != nil || got.Status != "betting" || got.Sequence != 2 || !got.BetStartAt.Equal(earlier.BetStartAt) || !got.BetEndAt.Equal(earlier.BetEndAt) || !got.DrawAt.Equal(earlier.DrawAt) {
				t.Fatalf("earlier scheduled candidate did not open in its original window: got=%+v error=%v", got, err)
			}
			got, err = scanPeriod(s.DB.QueryRow(ctx, `SELECT `+periodFields+` FROM periods WHERE id=$1`, future.ID))
			if err != nil || !reflect.DeepEqual(got, future) {
				t.Fatalf("opening earlier draw changed future reservation: got=%+v error=%v", got, err)
			}
			current := concurrencyReadVersion(t, s, queued.ID)
			if current.Status != "active" || current.EffectivePeriodID != earlier.ID || concurrencyReadPlay(t, s, play.ID).ActiveVersionID != queued.ID || concurrencyReadVersion(t, s, active.ID).Status != "expired" {
				t.Fatalf("out-of-order calendar creation corrupted rule activation: %+v", current)
			}
			if sequence := concurrencyReadGame(t, s, game.ID).StartedSequence; sequence != 1 {
				t.Fatalf("activation sequence=%d, want first actual opening", sequence)
			}
			if n := concurrencyCount(t, s, `SELECT count(*) FROM period_rule_versions WHERE period_id=$1`, future.ID); n != 0 {
				t.Fatalf("future pending period received %d rule snapshots", n)
			}
			if n := concurrencyCount(t, s, `SELECT count(*) FROM period_rule_versions WHERE period_id=$1 AND rule_version_id=$2`, earlier.ID, queued.ID); n != 1 {
				t.Fatalf("earlier period received %d queued rule snapshots, want 1", n)
			}
		})
	}
}
