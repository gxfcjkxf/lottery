package mutation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
)

func newEvidenceTable(t *testing.T, e *Engine, ctx context.Context) {
	t.Helper()
	if _, err := e.DB.Exec(ctx, `CREATE TABLE rejection_evidence_state (id integer PRIMARY KEY, business_value integer NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.DB.Exec(ctx, `INSERT INTO rejection_evidence_state VALUES (1, 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.DB.Exec(ctx, `CREATE TABLE rejection_evidence (id bigserial PRIMARY KEY, detail text NOT NULL)`); err != nil {
		t.Fatal(err)
	}
}

func evidenceCount(t *testing.T, e *Engine, ctx context.Context) int {
	t.Helper()
	var count int
	if err := e.DB.QueryRow(ctx, `SELECT count(*) FROM rejection_evidence`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestRejectedEvidencePersistsOnceWithCachedRejection(t *testing.T) {
	e, ctx := checkedEngine(t)
	newEvidenceTable(t, e, ctx)
	const workers = 12
	var callbacks atomic.Int32
	var wg sync.WaitGroup
	errCh := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := e.Execute(ctx, checkedBrand, "evidence-actor", "evidence.reject", "evidence-replay-01", e.Fingerprint("reject"), func(ctx context.Context, tx pgx.Tx) (Result, error) {
				if _, err := tx.Exec(ctx, `UPDATE rejection_evidence_state SET business_value=business_value+1 WHERE id=1`); err != nil {
					return Result{}, err
				}
				return WithRejectedEvidence(Fail(422, "REJECTED", "invalid request"), func(ctx context.Context, tx pgx.Tx) error {
					callbacks.Add(1)
					_, err := tx.Exec(ctx, `INSERT INTO rejection_evidence(detail) VALUES ('attempt recorded')`)
					return err
				}), nil
			})
			if err != nil {
				errCh <- err
			} else if result.Status != 422 {
				errCh <- fmt.Errorf("status=%d, want 422", result.Status)
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
	var businessValue int
	if err := e.DB.QueryRow(ctx, `SELECT business_value FROM rejection_evidence_state WHERE id=1`).Scan(&businessValue); err != nil {
		t.Fatal(err)
	}
	if businessValue != 0 || evidenceCount(t, e, ctx) != 1 || callbacks.Load() != 1 || countCheckedRows(t, ctx, e) != 1 {
		t.Fatalf("business=%d evidence=%d callbacks=%d cached=%d; want 0, 1, 1, 1", businessValue, evidenceCount(t, e, ctx), callbacks.Load(), countCheckedRows(t, ctx, e))
	}
}

func TestRejectedEvidenceErrorRollsBackEverythingAndAllowsRetry(t *testing.T) {
	e, ctx := checkedEngine(t)
	newEvidenceTable(t, e, ctx)
	key := "evidence-retry-01"
	boom := errors.New("evidence write failed")
	result, err := e.Execute(ctx, checkedBrand, "evidence-retry-actor", "evidence.retry", key, e.Fingerprint("retry"), func(ctx context.Context, tx pgx.Tx) (Result, error) {
		if _, err := tx.Exec(ctx, `UPDATE rejection_evidence_state SET business_value=business_value+1 WHERE id=1`); err != nil {
			return Result{}, err
		}
		return WithRejectedEvidence(Fail(409, "DECLINED", "declined"), func(ctx context.Context, tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `INSERT INTO rejection_evidence(detail) VALUES ('must roll back')`); err != nil {
				return err
			}
			return boom
		}), nil
	})
	if !errors.Is(err, boom) || result.Status != 0 {
		t.Fatalf("first attempt result=%+v err=%v, want evidence error", result, err)
	}
	var businessValue int
	if err := e.DB.QueryRow(ctx, `SELECT business_value FROM rejection_evidence_state WHERE id=1`).Scan(&businessValue); err != nil {
		t.Fatal(err)
	}
	if businessValue != 0 || evidenceCount(t, e, ctx) != 0 || countCheckedRows(t, ctx, e) != 0 {
		t.Fatalf("failed callback left business=%d evidence=%d cached=%d", businessValue, evidenceCount(t, e, ctx), countCheckedRows(t, ctx, e))
	}
	result, err = e.Execute(ctx, checkedBrand, "evidence-retry-actor", "evidence.retry", key, e.Fingerprint("retry"), func(ctx context.Context, tx pgx.Tx) (Result, error) {
		if _, err := tx.Exec(ctx, `UPDATE rejection_evidence_state SET business_value=business_value+1 WHERE id=1`); err != nil {
			return Result{}, err
		}
		return WithRejectedEvidence(Fail(409, "DECLINED", "declined"), func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `INSERT INTO rejection_evidence(detail) VALUES ('retry succeeded')`)
			return err
		}), nil
	})
	if err != nil || result.Status != 409 || evidenceCount(t, e, ctx) != 1 || countCheckedRows(t, ctx, e) != 1 {
		t.Fatalf("retry result=%+v err=%v evidence=%d cached=%d", result, err, evidenceCount(t, e, ctx), countCheckedRows(t, ctx, e))
	}
}

func TestRejectedEvidenceProtocolErrorsRollback(t *testing.T) {
	t.Run("run error skips callback", func(t *testing.T) {
		e, ctx := checkedEngine(t)
		newEvidenceTable(t, e, ctx)
		boom := errors.New("run failed")
		var calls atomic.Int32
		_, err := e.Execute(ctx, checkedBrand, "evidence-error-actor", "evidence.run-error", "evidence-run-error-01", e.Fingerprint("run error"), func(ctx context.Context, tx pgx.Tx) (Result, error) {
			return WithRejectedEvidence(Fail(400, "NO", "no"), func(context.Context, pgx.Tx) error { calls.Add(1); return nil }), boom
		})
		if !errors.Is(err, boom) || calls.Load() != 0 || evidenceCount(t, e, ctx) != 0 || countCheckedRows(t, ctx, e) != 0 {
			t.Fatalf("err=%v calls=%d evidence=%d cached=%d", err, calls.Load(), evidenceCount(t, e, ctx), countCheckedRows(t, ctx, e))
		}
	})
	t.Run("success callback forbidden", func(t *testing.T) {
		e, ctx := checkedEngine(t)
		newEvidenceTable(t, e, ctx)
		_, err := e.Execute(ctx, checkedBrand, "evidence-success-actor", "evidence.success", "evidence-success-01", e.Fingerprint("success"), func(ctx context.Context, tx pgx.Tx) (Result, error) {
			if _, err := tx.Exec(ctx, `UPDATE rejection_evidence_state SET business_value=7 WHERE id=1`); err != nil {
				return Result{}, err
			}
			return WithRejectedEvidence(OK(200, map[string]string{"ok": "yes"}), func(ctx context.Context, tx pgx.Tx) error {
				_, err := tx.Exec(ctx, `INSERT INTO rejection_evidence(detail) VALUES ('invalid protocol')`)
				return err
			}), nil
		})
		var businessValue int
		if scanErr := e.DB.QueryRow(ctx, `SELECT business_value FROM rejection_evidence_state WHERE id=1`).Scan(&businessValue); scanErr != nil {
			t.Fatal(scanErr)
		}
		if err == nil || !strings.Contains(err.Error(), "rejected evidence") || businessValue != 0 || evidenceCount(t, e, ctx) != 0 || countCheckedRows(t, ctx, e) != 0 {
			t.Fatalf("err=%v business=%d evidence=%d cached=%d", err, businessValue, evidenceCount(t, e, ctx), countCheckedRows(t, ctx, e))
		}
	})
	t.Run("callback status must be rejection", func(t *testing.T) {
		for _, status := range []int{399, 600} {
			t.Run(fmt.Sprint(status), func(t *testing.T) {
				e, ctx := checkedEngine(t)
				newEvidenceTable(t, e, ctx)
				_, err := e.Execute(ctx, checkedBrand, "evidence-status-actor", "evidence.status", fmt.Sprintf("evidence-status-%03d", status), e.Fingerprint(fmt.Sprint(status)), func(context.Context, pgx.Tx) (Result, error) {
					return WithRejectedEvidence(Result{Status: status}, func(context.Context, pgx.Tx) error { return nil }), nil
				})
				if err == nil || countCheckedRows(t, ctx, e) != 0 {
					t.Fatalf("status=%d err=%v cached=%d", status, err, countCheckedRows(t, ctx, e))
				}
			})
		}
	})
}

func TestRejectedEvidenceCallbackIsNotSerializedAndRevokedReplayIsHidden(t *testing.T) {
	e, ctx := checkedEngine(t)
	newEvidenceTable(t, e, ctx)
	var callbacks atomic.Int32
	key := "evidence-private-01"
	run := func(ctx context.Context, tx pgx.Tx) (Result, error) {
		return WithRejectedEvidence(Fail(422, "PRIVATE_REJECT", "private rejection"), func(ctx context.Context, tx pgx.Tx) error {
			callbacks.Add(1)
			_, err := tx.Exec(ctx, `INSERT INTO rejection_evidence(detail) VALUES ('private evidence')`)
			return err
		}), nil
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
	result, err := e.ExecuteChecked(ctx, checkedBrand, "evidence-private-actor", "evidence.private", key, e.Fingerprint("private"), check, run)
	if err != nil || result.Status != 422 || callbacks.Load() != 1 {
		t.Fatalf("initial result=%+v err=%v callbacks=%d", result, err, callbacks.Load())
	}
	encoded, err := json.Marshal(result)
	if err != nil || strings.Contains(string(encoded), "rejectedEvidence") || strings.Contains(string(encoded), "private evidence") {
		t.Fatalf("serialized result exposed private callback or evidence: %s err=%v", encoded, err)
	}
	replayed, err := e.ExecuteChecked(ctx, checkedBrand, "evidence-private-actor", "evidence.private", key, e.Fingerprint("private"), check, run)
	if err != nil || replayed.Status != 422 || replayed.rejectedEvidence != nil || callbacks.Load() != 1 {
		t.Fatalf("cached replay=%+v err=%v callback metadata=%v callbacks=%d", replayed, err, replayed.rejectedEvidence != nil, callbacks.Load())
	}
	if _, err := e.DB.Exec(ctx, `UPDATE checked_authorization SET allowed=false WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	revokedReplay, err := e.ExecuteChecked(ctx, checkedBrand, "evidence-private-actor", "evidence.private", key, e.Fingerprint("private"), check, run)
	if err == nil || revokedReplay.Status != 0 || callbacks.Load() != 1 || evidenceCount(t, e, ctx) != 1 || countCheckedRows(t, ctx, e) != 1 {
		t.Fatalf("revoked replay=%+v err=%v callbacks=%d evidence=%d cached=%d", revokedReplay, err, callbacks.Load(), evidenceCount(t, e, ctx), countCheckedRows(t, ctx, e))
	}
}
