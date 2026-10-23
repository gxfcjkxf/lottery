package mutation

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestTemporaryFailuresRollbackAndOriginalKeyCanRecover(t *testing.T) {
	for _, platform := range []bool{false, true} {
		t.Run(fmt.Sprintf("platform=%v", platform), func(t *testing.T) {
			e, ctx := checkedEngine(t)
			brand := checkedBrand
			if platform {
				brand = ""
			}
			if _, err := e.DB.Exec(ctx, `CREATE TABLE retry_effects (id integer PRIMARY KEY)`); err != nil {
				t.Fatal(err)
			}
			calls := 0
			run := func(ctx context.Context, tx pgx.Tx) (Result, error) {
				calls++
				if _, err := tx.Exec(ctx, `INSERT INTO retry_effects VALUES(1)`); err != nil {
					return Result{}, err
				}
				if calls == 1 {
					return Fail(503, "TEMPORARILY_BUSY", "busy"), nil
				}
				return OK(201, map[string]string{"id": "one"}), nil
			}
			check := func(context.Context, pgx.Tx) error { return nil }
			execute := func() (Result, error) {
				return e.ExecuteChecked(ctx, brand, "transient-actor", "retry.write", "same-original-key", e.Fingerprint("original body"), check, run)
			}
			first, err := execute()
			if err != nil || first.Status != 503 {
				t.Fatalf("first=%+v err=%v", first, err)
			}
			var effects, cached int
			if err = e.DB.QueryRow(ctx, `SELECT (SELECT count(*) FROM retry_effects),(SELECT count(*) FROM idempotency_requests)+(SELECT count(*) FROM platform_idempotency_requests)`).Scan(&effects, &cached); err != nil || effects != 0 || cached != 0 {
				t.Fatalf("transient effects=%d cached=%d err=%v", effects, cached, err)
			}
			second, err := execute()
			if err != nil || second.Status != 201 {
				t.Fatalf("recovery=%+v err=%v", second, err)
			}
			replay, err := execute()
			if err != nil || replay.Status != 201 || string(replay.Data) != string(second.Data) || calls != 2 {
				t.Fatalf("replay=%+v calls=%d err=%v", replay, calls, err)
			}
			if err = e.DB.QueryRow(ctx, `SELECT (SELECT count(*) FROM retry_effects),(SELECT count(*) FROM idempotency_requests)+(SELECT count(*) FROM platform_idempotency_requests)`).Scan(&effects, &cached); err != nil || effects != 1 || cached != 1 {
				t.Fatalf("recovered effects=%d cached=%d err=%v", effects, cached, err)
			}
			conflict, err := e.ExecuteChecked(ctx, brand, "transient-actor", "retry.write", "same-original-key", e.Fingerprint("changed body"), check, run)
			if err != nil || conflict.Status != 409 || calls != 2 {
				t.Fatalf("changed-body result=%+v calls=%d err=%v", conflict, calls, err)
			}
		})
	}
}
