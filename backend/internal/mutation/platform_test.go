package mutation

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestPlatformCheckedHasNoBrandDependencyAndKeepsAuthority(t *testing.T) {
	e, ctx := checkedEngine(t)
	var calls int
	check := func(ctx context.Context, tx pgx.Tx) error {
		var allowed bool
		if err := tx.QueryRow(ctx, `SELECT allowed FROM checked_authorization WHERE id=1`).Scan(&allowed); err != nil {
			return err
		}
		if !allowed {
			return errors.New("revoked")
		}
		return nil
	}
	run := func(context.Context, pgx.Tx) (Result, error) {
		calls++
		return OK(201, map[string]string{"receipt": "platform-result"}), nil
	}
	key := "platform-no-brand-0001"
	first, err := e.ExecuteChecked(ctx, "", "platform-actor", "platform.create", key, e.Fingerprint("body"), check, run)
	if err != nil || first.Status != 201 {
		t.Fatalf("platform mutation still requires a brand: status=%d err=%v", first.Status, err)
	}
	same, err := e.ExecuteChecked(ctx, "", "platform-actor", "platform.create", key, e.Fingerprint("body"), check, run)
	if err != nil || string(same.Data) != string(first.Data) || calls != 1 {
		t.Fatal("platform replay differs", err, calls)
	}
	var raw string
	if err = e.DB.QueryRow(ctx, `SELECT response::text FROM platform_idempotency_requests`).Scan(&raw); err != nil || strings.Contains(raw, "platform-result") {
		t.Fatal("unencrypted platform receipt", err)
	}
	conflict, err := e.ExecuteChecked(ctx, "", "platform-actor", "platform.create", key, e.Fingerprint("different"), check, run)
	if err != nil || conflict.Status != 409 {
		t.Fatal("same-key body conflict missing", err, conflict)
	}
	if _, err = e.DB.Exec(ctx, `UPDATE checked_authorization SET allowed=false WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	if _, err = e.ExecuteChecked(ctx, "", "platform-actor", "platform.create", key, e.Fingerprint("body"), check, run); err == nil {
		t.Fatal("revoked authority replayed platform receipt")
	}
	if countCheckedRows(t, ctx, e) != 0 {
		t.Fatal("platform receipt leaked into brand partition")
	}
}

func TestPlatformConcurrentReplayRunsOnceAndFailureRollsBack(t *testing.T) {
	e, ctx := checkedEngine(t)
	var calls atomic.Int32
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := e.ExecuteChecked(ctx, "", "platform-actor", "platform.once", "platform-once-001", e.Fingerprint("body"), func(context.Context, pgx.Tx) error { return nil }, func(context.Context, pgx.Tx) (Result, error) {
				calls.Add(1)
				return OK(201, map[string]string{"id": "one"}), nil
			})
			if err != nil || r.Status != 201 {
				t.Errorf("concurrent platform replay: %v %d", err, r.Status)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal("platform ran repeatedly", calls.Load())
	}
	_, err := e.ExecuteChecked(ctx, "", "platform-actor", "platform.failure", "platform-fail-001", e.Fingerprint("bad"), func(context.Context, pgx.Tx) error { return nil }, func(ctx context.Context, tx pgx.Tx) (Result, error) {
		_, err := tx.Exec(ctx, `UPDATE brands SET name='must roll back' WHERE code='aurora'`)
		return Fail(409, "TEST_FAILURE", "expected"), err
	})
	if err != nil {
		t.Fatal(err)
	}
	var name string
	var n int
	if err = e.DB.QueryRow(ctx, `SELECT name FROM brands WHERE code='aurora'`).Scan(&name); err != nil || name != "Aurora" {
		t.Fatal(name, err)
	}
	if err = e.DB.QueryRow(ctx, `SELECT count(*) FROM platform_idempotency_requests`).Scan(&n); err != nil || n != 2 {
		t.Fatal(n, err)
	}
	if _, err = e.Execute(ctx, "", "anonymous", "bad.platform", "platform-unchecked-01", e.Fingerprint("x"), func(context.Context, pgx.Tx) (Result, error) { return OK(201, nil), nil }); err == nil {
		t.Fatal("unchecked platform write accepted")
	}
}
