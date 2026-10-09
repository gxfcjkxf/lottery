package rulebook

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

func TestConcurrentCreateGameKeepsDuplicateCodeCheckSerialized(t *testing.T) {
	s, a, _, g, _, _ := fixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := make(chan struct{})
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			tx, err := s.DB.Begin(ctx)
			if err != nil {
				results <- err
				return
			}
			defer tx.Rollback(context.Background())
			_, err = s.CreateGame(ctx, tx, brand, a, "duplicate_concurrent", "Concurrent code", g.Model, g.Timezone, "same-code serialization regression", points.Metadata{})
			if err == nil {
				err = tx.Commit(ctx)
			}
			results <- err
		}()
	}
	close(start)
	first, second := <-results, <-results
	if !((first == nil && errors.Is(second, ErrVersion)) || (second == nil && errors.Is(first, ErrVersion))) {
		t.Fatalf("want one success and one duplicate conflict, got %v and %v", first, second)
	}
}

// Login locks the administrator before its audit references the brand. Game
// creation must not block that foreign-key check while waiting for the creator.
func TestCreateGameDoesNotDeadlockConcurrentAdminLogin(t *testing.T) {
	s, a, _, g, _, _ := fixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	login, err := s.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer login.Rollback(context.Background())
	if _, err = login.Exec(ctx, `SELECT id FROM admin_accounts WHERE id=$1 FOR UPDATE`, a.ID); err != nil {
		t.Fatal(err)
	}
	creation, err := s.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer creation.Rollback(context.Background())
	pid := creation.Conn().PgConn().PID()
	finished := make(chan error, 1)
	go func() {
		_, e := s.CreateGame(ctx, creation, brand, a, "concurrent_login", "Concurrent login", g.Model, g.Timezone, "real login lock regression", points.Metadata{})
		finished <- e
	}()
	// Observe the real lock wait, rather than guessing a sleep duration.
	for {
		var blocked bool
		if err = s.DB.QueryRow(ctx, `SELECT cardinality(pg_blocking_pids($1))>0`, pid).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			break
		}
		select {
		case err = <-finished:
			t.Fatalf("creation did not wait for login lock: %v", err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
	// This is the lock PostgreSQL takes for audit_logs.brand_id's FK check.
	if _, err = login.Exec(ctx, `SELECT id FROM brands WHERE id=$1 FOR KEY SHARE`, brand); err != nil {
		t.Fatal(err)
	}
	if err = login.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-finished; err != nil {
		t.Fatal(err)
	}
	if err = creation.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = s.DB.QueryRow(ctx, `SELECT count(*) FROM games WHERE brand_id=$1 AND code='concurrent_login'`, brand).Scan(&count); err != nil || count != 1 {
		t.Fatalf("game count %d: %v", count, err)
	}
}
