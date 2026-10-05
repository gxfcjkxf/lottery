package mutation

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
)

const checkedBrand = "0199a000-0000-7000-8000-000000000001"

func checkedEngine(t *testing.T) (*Engine, context.Context) {
	t.Helper()
	p := testdb.New(t)
	e, err := New(p, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := p.Exec(ctx, `CREATE TABLE checked_authorization (id integer PRIMARY KEY, allowed boolean NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `INSERT INTO checked_authorization VALUES (1, true)`); err != nil {
		t.Fatal(err)
	}
	return e, ctx
}

func countCheckedRows(t *testing.T, ctx context.Context, e *Engine) int {
	t.Helper()
	var n int
	if err := e.DB.QueryRow(ctx, `SELECT count(*) FROM idempotency_requests`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestExecuteCheckedChecksBeforeBusinessAndRejectsWithoutRecording(t *testing.T) {
	e, ctx := checkedEngine(t)
	var events []string
	runCalls := 0
	check := func(ctx context.Context, tx pgx.Tx) error {
		events = append(events, "check")
		var allowed bool
		if err := tx.QueryRow(ctx, `SELECT allowed FROM checked_authorization WHERE id=1`).Scan(&allowed); err != nil {
			return err
		}
		if !allowed {
			return errors.New("authorization revoked")
		}
		return nil
	}
	run := func(context.Context, pgx.Tx) (Result, error) {
		events = append(events, "run")
		runCalls++
		return OK(200, map[string]string{"ok": "yes"}), nil
	}

	if _, err := e.ExecuteChecked(ctx, checkedBrand, "checked-actor", "checked.initial", "checked-initial-01", e.Fingerprint("initial"), check, run); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(events) != "[check check run]" {
		t.Fatalf("callback order = %v, want [check check run]", events)
	}
	if runCalls != 1 || countCheckedRows(t, ctx, e) != 1 {
		t.Fatalf("run calls=%d persisted rows=%d, want 1 and 1", runCalls, countCheckedRows(t, ctx, e))
	}

	if _, err := e.DB.Exec(ctx, `UPDATE checked_authorization SET allowed=false WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	result, err := e.ExecuteChecked(ctx, checkedBrand, "checked-actor", "checked.initial", "checked-initial-02", e.Fingerprint("rejected"), check, run)
	if err == nil || result.Status != 0 {
		t.Fatalf("rejected call result=%+v err=%v, want authorization error", result, err)
	}
	if runCalls != 1 {
		t.Fatalf("business calls after rejected check = %d, want 1", runCalls)
	}
	if count := countCheckedRows(t, ctx, e); count != 1 {
		t.Fatalf("persisted rows after rejected check = %d, want 1", count)
	}
}

func TestExecuteCheckedRevalidatesAfterOperationLock(t *testing.T) {
	e, ctx := checkedEngine(t)
	checks, runs := 0, 0
	denied := errors.New("session expired during operation lock wait")
	_, err := e.ExecuteChecked(ctx, checkedBrand, "expiring-actor", "checked.expiring", "checked-expiring-key", e.Fingerprint("expiring"), func(context.Context, pgx.Tx) error {
		checks++
		if checks == 2 {
			return denied
		}
		return nil
	}, func(context.Context, pgx.Tx) (Result, error) {
		runs++
		return OK(200, nil), nil
	})
	if !errors.Is(err, denied) || checks != 2 || runs != 0 || countCheckedRows(t, ctx, e) != 0 {
		t.Fatalf("revalidation checks=%d runs=%d err=%v", checks, runs, err)
	}
}

func TestExecuteCheckedChecksBeforeCachedReplay(t *testing.T) {
	e, ctx := checkedEngine(t)
	calls := 0
	run := func(context.Context, pgx.Tx) (Result, error) {
		calls++
		return OK(201, map[string]string{"value": "cached"}), nil
	}
	check := func(ctx context.Context, tx pgx.Tx) error {
		var allowed bool
		if err := tx.QueryRow(ctx, `SELECT allowed FROM checked_authorization WHERE id=1`).Scan(&allowed); err != nil {
			return err
		}
		if !allowed {
			return errors.New("authorization revoked")
		}
		return nil
	}
	key := "checked-replay-01"
	fingerprint := e.Fingerprint("same request")
	if _, err := e.ExecuteChecked(ctx, checkedBrand, "replay-actor", "checked.replay", key, fingerprint, check, run); err != nil {
		t.Fatal(err)
	}
	if _, err := e.DB.Exec(ctx, `UPDATE checked_authorization SET allowed=false WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	result, err := e.ExecuteChecked(ctx, checkedBrand, "replay-actor", "checked.replay", key, fingerprint, check, run)
	if err == nil || result.Status != 0 {
		t.Fatalf("revoked replay result=%+v err=%v, want authorization error", result, err)
	}
	if calls != 1 {
		t.Fatalf("business calls = %d, want exactly 1", calls)
	}
	if count := countCheckedRows(t, ctx, e); count != 1 {
		t.Fatalf("persisted rows = %d, want existing cached row only", count)
	}
}

func TestExecuteCheckedHoldsCheckLocksUntilCommitOrRollback(t *testing.T) {
	for _, outcome := range []string{"commit", "rollback"} {
		t.Run(outcome, func(t *testing.T) {
			e, ctx := checkedEngine(t)
			started := make(chan struct{})
			release := make(chan struct{})
			finished := make(chan error, 1)
			go func() {
				_, err := e.ExecuteChecked(ctx, checkedBrand, "lock-actor-"+outcome, "checked.lock", "checked-lock-"+outcome, e.Fingerprint(outcome), func(ctx context.Context, tx pgx.Tx) error {
					var allowed bool
					return tx.QueryRow(ctx, `SELECT allowed FROM checked_authorization WHERE id=1 FOR UPDATE`).Scan(&allowed)
				}, func(context.Context, pgx.Tx) (Result, error) {
					close(started)
					<-release
					if outcome == "rollback" {
						return Result{}, errors.New("force transaction rollback")
					}
					return OK(200, map[string]string{"done": "yes"}), nil
				})
				finished <- err
			}()

			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("business callback did not start")
			}
			lockCtx, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
			_, lockErr := e.DB.Exec(lockCtx, `UPDATE checked_authorization SET allowed=false WHERE id=1`)
			cancel()
			if lockErr == nil {
				t.Fatal("authorization row lock was not held while business callback ran")
			}
			close(release)
			err := <-finished
			if outcome == "commit" && err != nil {
				t.Fatalf("checked execution: %v", err)
			}
			if outcome == "rollback" && err == nil {
				t.Fatal("expected forced rollback error")
			}

			updateCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			if _, err := e.DB.Exec(updateCtx, `UPDATE checked_authorization SET allowed=false WHERE id=1`); err != nil {
				t.Fatalf("authorization row remained locked after %s: %v", outcome, err)
			}
			wantRows := 1
			if outcome == "rollback" {
				wantRows = 0
			}
			if count := countCheckedRows(t, ctx, e); count != wantRows {
				t.Fatalf("persisted rows after %s = %d, want %d", outcome, count, wantRows)
			}
		})
	}
}
