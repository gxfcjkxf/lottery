package mutation

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
	"sync"
	"sync/atomic"
	"testing"
)

func TestConcurrentReplayExecutesOnceAndFailureRollsBack(t *testing.T) {
	p := testdb.New(t)
	e, err := New(p, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	brand := "0199a000-0000-7000-8000-000000000001"
	var calls atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := e.Execute(ctx, brand, "actor", "op", "operation-0001", e.Fingerprint("body"), func(ctx context.Context, tx pgx.Tx) (Result, error) {
				calls.Add(1)
				return OK(200, map[string]string{"value": "once"}), nil
			})
			if err != nil || r.Status != 200 {
				t.Errorf("replay: %v %+v", err, r)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal("logical action executed", calls.Load())
	}
	_, err = e.Execute(ctx, brand, "actor", "op", "operation-fail", e.Fingerprint("failure"), func(ctx context.Context, tx pgx.Tx) (Result, error) {
		_, err := tx.Exec(ctx, "UPDATE brands SET name='bad partial state' WHERE id=$1", brand)
		return Fail(409, "TEST_FAILED", "test"), err
	})
	if err != nil {
		t.Fatal(err)
	}
	var name string
	p.QueryRow(ctx, "SELECT name FROM brands WHERE id=$1", brand).Scan(&name)
	if name != "Aurora" {
		t.Fatal("failed business changes committed")
	}
	var count int
	p.QueryRow(ctx, "SELECT count(*) FROM idempotency_requests").Scan(&count)
	if count != 2 {
		t.Fatal("final failure not recorded", count)
	}
}
