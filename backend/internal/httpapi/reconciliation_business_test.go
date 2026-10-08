package httpapi

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/reconciliation"
)

const reconciliationHTTPPath = "/api/v1/admin/reconciliations"

func reconciliationRawCall(f reconciliationHTTPFixture, method, path, key, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://localhost"+path, strings.NewReader(body))
	r.RemoteAddr = "192.0.2.55:12345"
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Idempotency-Key", key)
	r.Header.Set("X-Brand-ID", managedBrand)
	r.Header.Set("Authorization", "Bearer "+f.token)
	w := httptest.NewRecorder()
	f.http.ServeHTTP(w, r)
	return w
}

func TestReconciliationHTTPBusinessScopeAndWalletReceiptCompatibility(t *testing.T) {
	f := newReconciliationHTTPFixture(t)
	body := `{"reason":"business witness pass","check_scope":"wallet_and_business"}`
	created := reconciliationRawCall(f, "POST", reconciliationHTTPPath, "reconcile-business-scope-01", body)
	mustStatus(t, created, 201)
	var receipt reconciliation.Job
	managedData(t, created, &receipt)
	if receipt.CheckScope != reconciliation.ScopeWalletAndBusiness || receipt.State != "pending" {
		t.Fatalf("explicit business scope missing from creation receipt: %+v", receipt)
	}
	pending := f.call("GET", reconciliationHTTPPath+"/"+receipt.ID+"/targets", "", f.token, managedBrand, nil)
	mustStatus(t, pending, 200)
	var page reconciliation.TargetPage
	managedData(t, pending, &page)
	if len(page.Items) != 1 || page.Items[0].CheckScope != reconciliation.ScopeWalletAndBusiness || page.Items[0].State != "pending" || page.Items[0].BusinessPreview != nil {
		t.Fatalf("pending business observation exposed missing evidence: %+v", page)
	}
	if _, err := f.service.Process(context.Background(), 20); err != nil {
		t.Fatal("process business reconciliation:", err)
	}
	checked := f.call("GET", reconciliationHTTPPath+"/"+receipt.ID+"/targets?outcome=consistent", "", f.token, managedBrand, nil)
	mustStatus(t, checked, 200)
	managedData(t, checked, &page)
	if len(page.Items) != 1 || page.Items[0].State != "checked" || page.Items[0].BusinessPreview == nil || page.Items[0].Preview == nil || page.Items[0].AuditLogID == nil {
		t.Fatalf("checked business observation omitted evidence: %+v", page)
	}

	// Legacy callers omit check_scope. Their saved request and receipt remain wallet-only.
	legacyBody := `{"reason":"legacy wallet-only pass"}`
	legacy := reconciliationRawCall(f, "POST", reconciliationHTTPPath, "reconcile-legacy-wallet-01", legacyBody)
	mustStatus(t, legacy, 201)
	var legacyReceipt reconciliation.Job
	managedData(t, legacy, &legacyReceipt)
	if legacyReceipt.CheckScope != reconciliation.ScopeWallet {
		t.Fatalf("omitted check_scope did not retain wallet scope: %+v", legacyReceipt)
	}
	if _, err := f.service.Process(context.Background(), 20); err != nil {
		t.Fatal("process legacy reconciliation:", err)
	}
	replay := reconciliationRawCall(f, "POST", reconciliationHTTPPath, "reconcile-legacy-wallet-01", legacyBody)
	mustStatus(t, replay, 201)
	var replayReceipt reconciliation.Job
	managedData(t, replay, &replayReceipt)
	if replayReceipt != legacyReceipt {
		t.Fatalf("legacy retry did not return its original wallet receipt: original=%+v replay=%+v", legacyReceipt, replayReceipt)
	}
	changedMode := reconciliationRawCall(f, "POST", reconciliationHTTPPath, "reconcile-legacy-wallet-01", `{"reason":"legacy wallet-only pass","check_scope":"wallet_and_business"}`)
	mustStatus(t, changedMode, 409)
}

func TestReconciliationHTTPClosedCreateAndCanonicalGetInputs(t *testing.T) {
	f := newReconciliationHTTPFixture(t)
	for i, body := range []string{
		"null",
		`{"reason":"duplicate key","reason":"duplicate key"}`,
		`{"reason":"null scope","check_scope":null}`,
		`{"reason":"unknown field","extra":true}`,
		`{"reason":"wrong scope","check_scope":"business"}`,
	} {
		mustStatus(t, reconciliationRawCall(f, "POST", reconciliationHTTPPath, "reconcile-closed-body-"+string(rune('1'+i)), body), 400)
	}
	for _, path := range []string{
		reconciliationHTTPPath + "?",
		reconciliationHTTPPath + "?limit=020",
		reconciliationHTTPPath + "?limit=+20",
		reconciliationHTTPPath + "?limit=20&limit=20",
		reconciliationHTTPPath + "?unknown=1",
	} {
		mustStatus(t, f.call("GET", path, "", f.token, managedBrand, nil), 400)
	}
	mustStatus(t, reconciliationRawCall(f, "GET", reconciliationHTTPPath, "", `null`), 400)
	// Keep canonical pagination accepted; this also exercises a body-free GET request.
	mustStatus(t, f.call("GET", reconciliationHTTPPath+"?limit=20&offset=0", "", f.token, managedBrand, nil), 200)
	inlineTab := reconciliationRawCall(f, "POST", reconciliationHTTPPath, "reconcile-inline-tab-01", `{"reason":"inline\tseparator"}`)
	mustStatus(t, inlineTab, 201)
	var tabReceipt reconciliation.Job
	managedData(t, inlineTab, &tabReceipt)
	if tabReceipt.Reason != "inline\tseparator" {
		t.Fatalf("inline tab was not preserved by legacy reason validation: %q", tabReceipt.Reason)
	}
}

func TestReconciliationHTTPAuthExpiresWhileCreateOrReadAuditWaits(t *testing.T) {
	for _, write := range []bool{true, false} {
		t.Run(map[bool]string{true: "create", false: "read"}[write], func(t *testing.T) {
			f := newReconciliationHTTPFixture(t)
			ctx := context.Background()
			jobID := ""
			if !write {
				jobID = reconciliationCreate(t, f, "reconcile-expiry-read-setup", "prepare read expiry").ID
			}
			lock, err := f.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer lock.Rollback(ctx)
			const advisoryKey int64 = 77159002
			if _, err = lock.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, advisoryKey); err != nil {
				t.Fatal(err)
			}
			if _, err = f.pool.Exec(ctx, `CREATE FUNCTION wait_reconciliation_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action IN('wallet.reconciliation.create','wallet.reconcile.read') THEN PERFORM pg_advisory_xact_lock(77159002); END IF; RETURN NEW; END $$; CREATE TRIGGER wait_reconciliation_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION wait_reconciliation_audit()`); err != nil {
				t.Fatal(err)
			}
			key := "reconcile-expiry-write-01"
			response := make(chan *httptest.ResponseRecorder, 1)
			go func() {
				method, path, idempotencyKey, body := "GET", reconciliationHTTPPath+"/"+jobID, "", ""
				if write {
					method, path, idempotencyKey, body = "POST", reconciliationHTTPPath, key, `{"reason":"expire while create audit waits","check_scope":"wallet_and_business"}`
				}
				response <- reconciliationRawCall(f, method, path, idempotencyKey, body)
			}()
			waited := false
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				err = f.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks waiting JOIN pg_locks held USING(locktype,classid,objid,objsubid) WHERE waiting.locktype='advisory' AND NOT waiting.granted AND held.granted AND held.pid=$1 AND waiting.pid<>pg_backend_pid())`, lock.Conn().PgConn().PID()).Scan(&waited)
				if err != nil {
					t.Fatal(err)
				}
				if waited {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
			if !waited {
				t.Fatal("reconciliation request did not reach its audit wait")
			}
			if _, err = f.pool.Exec(ctx, `UPDATE sessions SET expires_at=clock_timestamp()-interval '1 second' WHERE admin_id=$1 AND revoked_at IS NULL`, f.root); err != nil {
				t.Fatal(err)
			}
			if err = lock.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			var r *httptest.ResponseRecorder
			select {
			case r = <-response:
			case <-time.After(3 * time.Second):
				t.Fatal("expired reconciliation request stayed blocked")
			}
			mustStatus(t, r, 401)
			if strings.Contains(r.Body.String(), `"creation_audit_log_id"`) || strings.Contains(r.Body.String(), `"check_scope"`) {
				t.Fatal("expired session received reconciliation data")
			}
			var jobs, audits, receipts int64
			if write {
				err = f.pool.QueryRow(ctx, `SELECT count(*),(SELECT count(*) FROM audit_logs WHERE action='wallet.reconciliation.create'),(SELECT count(*) FROM idempotency_requests WHERE key=$1) FROM point_reconciliation_jobs`, key).Scan(&jobs, &audits, &receipts)
				if err != nil || jobs != 0 || audits != 0 || receipts != 0 {
					t.Fatalf("expired create persisted writes: jobs=%d audits=%d receipts=%d err=%v", jobs, audits, receipts, err)
				}
			} else {
				err = f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='wallet.reconcile.read'`).Scan(&audits)
				if err != nil || audits != 0 {
					t.Fatalf("expired read persisted audit: audits=%d err=%v", audits, err)
				}
			}
		})
	}
}

func TestReconciliationHTTPWorkerAuditFailureRollsBackEvidence(t *testing.T) {
	f := newReconciliationHTTPFixture(t)
	created := reconciliationRawCall(f, "POST", reconciliationHTTPPath, "reconcile-worker-audit-fail", `{"reason":"worker evidence rollback","check_scope":"wallet_and_business"}`)
	mustStatus(t, created, 201)
	var job reconciliation.Job
	managedData(t, created, &job)
	if _, err := f.pool.Exec(context.Background(), `CREATE FUNCTION reject_reconciliation_worker_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action IN('wallet.reconciliation.checked','wallet.reconciliation.failed') THEN RAISE EXCEPTION 'audit unavailable'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_reconciliation_worker_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_reconciliation_worker_audit()`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Process(context.Background(), 20); err == nil {
		t.Fatal("worker unexpectedly succeeded without its audit")
	}
	var targets, results, audits int
	if err := f.pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM point_reconciliation_targets WHERE job_id=$1::uuid AND state='pending'),(SELECT count(*) FROM point_reconciliation_results WHERE job_id=$1::uuid),(SELECT count(*) FROM audit_logs WHERE action='wallet.reconciliation.checked' AND after_json->>'job_id'=$1::text)`, job.ID).Scan(&targets, &results, &audits); err != nil || targets != 1 || results != 0 || audits != 0 {
		t.Fatalf("worker audit failure persisted partial evidence: pending=%d results=%d audits=%d err=%v", targets, results, audits, err)
	}
}
