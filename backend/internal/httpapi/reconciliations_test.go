package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"strconv"
	"sync"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/reconciliation"
)

type reconciliationHTTPFixture struct {
	managementHTTP
	service reconciliation.Service
}

func (f reconciliationHTTPFixture) call(method, path, key, token, brand string, body any) *httptest.ResponseRecorder {
	if method != "GET" || body != nil {
		return f.managementHTTP.call(method, path, key, token, brand, body)
	}
	r := httptest.NewRequest(method, "http://localhost"+path, nil)
	r.RemoteAddr = "192.0.2.55:12345"
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Idempotency-Key", key)
	r.Header.Set("X-Brand-ID", brand)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.http.ServeHTTP(w, r)
	return w
}

func newReconciliationHTTPFixture(t *testing.T) reconciliationHTTPFixture {
	t.Helper()
	f := pointsFixture(t)
	ctx := context.Background()
	var roleID string
	if err := f.pool.QueryRow(ctx, `SELECT r.id::text FROM roles r JOIN admin_account_roles ar ON ar.role_id=r.id WHERE ar.account_id=$1 LIMIT 1`, f.root).Scan(&roleID); err != nil {
		t.Fatal(err)
	}
	for _, permission := range []string{"wallet.view.brand", "wallet.reconcile.brand"} {
		if _, err := f.pool.Exec(ctx, `INSERT INTO permissions(key) VALUES($1) ON CONFLICT DO NOTHING`, permission); err != nil {
			t.Fatal(err)
		}
		if _, err := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,$2) ON CONFLICT DO NOTHING`, roleID, permission); err != nil {
			t.Fatal(err)
		}
	}
	return reconciliationHTTPFixture{managementHTTP: f.managementHTTP, service: reconciliation.Service{DB: f.pool}}
}

func reconciliationCreate(t *testing.T, f reconciliationHTTPFixture, key, reason string) reconciliation.Job {
	t.Helper()
	r := f.call("POST", "/api/v1/admin/reconciliations", key, f.token, managedBrand, map[string]string{"reason": reason})
	if r.Code != 201 {
		t.Fatalf("create reconciliation: status=%d body=%s", r.Code, r.Body.String())
	}
	var job reconciliation.Job
	managedData(t, r, &job)
	return job
}

func TestReconciliationHTTPCreateReceiptSurvivesWorkerAndValidatesIdempotency(t *testing.T) {
	f := newReconciliationHTTPFixture(t)
	ctx := context.Background()
	created := reconciliationCreate(t, f, "reconcile-create-0001", "monthly wallet check")
	if created.State != "pending" || created.Version != 1 || created.CheckedCount != "0" || created.PendingCount != created.TargetCount || created.CreationAuditLogID == "" {
		t.Fatalf("unexpected creation receipt: %+v", created)
	}
	if _, err := f.service.Process(ctx, 20); err != nil {
		t.Fatal("process reconciliation:", err)
	}
	replay := f.call("POST", "/api/v1/admin/reconciliations", "reconcile-create-0001", f.token, managedBrand, map[string]string{"reason": "monthly wallet check"})
	mustStatus(t, replay, 201)
	var receipt reconciliation.Job
	managedData(t, replay, &receipt)
	if !reflect.DeepEqual(receipt, created) {
		t.Fatalf("replay returned live job state instead of original receipt\ncreated=%+v\nreplay=%+v", created, receipt)
	}
	live := f.call("GET", "/api/v1/admin/reconciliations/"+created.ID, "", f.token, managedBrand, nil)
	mustStatus(t, live, 200)
	var current reconciliation.Job
	managedData(t, live, &current)
	if current.ID != created.ID || current.State == "pending" && current.CheckedCount != "0" {
		t.Fatalf("worker did not advance live job: %+v", current)
	}
	targets := f.call("GET", "/api/v1/admin/reconciliations/"+created.ID+"/targets?limit=20&offset=0", "", f.token, managedBrand, nil)
	mustStatus(t, targets, 200)
	var targetPage reconciliation.TargetPage
	managedData(t, targets, &targetPage)
	if targetPage.BrandID != managedBrand || targetPage.JobID != created.ID || targetPage.TotalCount != created.TargetCount || len(targetPage.Items) != 1 {
		t.Fatalf("unexpected historical target page: %+v", targetPage)
	}
	target := targetPage.Items[0]
	if target.State != "checked" || target.Outcome == nil || target.Preview == nil || target.CheckedAt == nil || target.AuditLogID == nil || target.ErrorCode != nil || target.AttemptCount < 1 {
		t.Fatalf("checked target omitted its evidence: %+v", target)
	}
	changed := f.call("POST", "/api/v1/admin/reconciliations", "reconcile-create-0001", f.token, managedBrand, map[string]string{"reason": "different reason"})
	mustStatus(t, changed, 409)
	if _, err := f.service.Process(ctx, 20); err != nil {
		t.Fatal("reprocess reconciliation:", err)
	}
}

func TestReconciliationHTTPRetryReceiptSurvivesSecondWorkerPass(t *testing.T) {
	f := newReconciliationHTTPFixture(t)
	ctx := context.Background()
	job := reconciliationCreate(t, f, "reconcile-retry-create01", "retry failed observation")
	if _, err := f.pool.Exec(ctx, `ALTER TABLE point_buckets DROP CONSTRAINT point_buckets_source_check`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE point_buckets SET source='invalid-fixture-source' WHERE brand_id=$1 AND source='recharge' AND state='available'`, managedBrand); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Process(ctx, 20); err != nil {
		t.Fatal("process failed observation:", err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE point_buckets SET source='recharge' WHERE brand_id=$1 AND source='invalid-fixture-source' AND state='available'`, managedBrand); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `ALTER TABLE point_buckets ADD CONSTRAINT point_buckets_source_check CHECK(source IN ('recharge','winning','gift','commission'))`); err != nil {
		t.Fatal(err)
	}
	failedResp := f.call("GET", "/api/v1/admin/reconciliations/"+job.ID, "", f.token, managedBrand, nil)
	mustStatus(t, failedResp, 200)
	var failed reconciliation.Job
	managedData(t, failedResp, &failed)
	if failed.State != "failed" || !failed.CanRetry || failed.LastErrorCode == nil || *failed.LastErrorCode != "CHECK_FAILED" {
		t.Fatalf("failed worker observation was not retryable: %+v", failed)
	}
	failedTargets := f.call("GET", "/api/v1/admin/reconciliations/"+job.ID+"/targets?outcome=failed", "", f.token, managedBrand, nil)
	mustStatus(t, failedTargets, 200)
	var failedPage reconciliation.TargetPage
	managedData(t, failedTargets, &failedPage)
	if failedPage.TotalCount != "1" || len(failedPage.Items) != 1 || failedPage.Items[0].State != "failed" || failedPage.Items[0].ErrorCode == nil || *failedPage.Items[0].ErrorCode != "CHECK_FAILED" || failedPage.Items[0].Outcome != nil || failedPage.Items[0].Preview != nil || failedPage.Items[0].CheckedAt != nil || failedPage.Items[0].AuditLogID != nil {
		t.Fatalf("failed target shape mismatch: %+v", failedPage)
	}
	retryPath := "/api/v1/admin/reconciliations/" + job.ID + "/retry"
	retryReason := "retry after observation fault was cleared"
	for i, body := range []string{
		"null",
		`{"version":` + strconv.FormatInt(failed.Version, 10) + `,"version":` + strconv.FormatInt(failed.Version, 10) + `,"reason":"duplicate version"}`,
		`{"version":null,"reason":"null version"}`,
		`{"version":` + strconv.FormatInt(failed.Version, 10) + `,"reason":null}`,
		`{"version":` + strconv.FormatInt(failed.Version, 10) + `,"reason":"missing reason","extra":true}`,
		`{"reason":"missing version"}`,
	} {
		mustStatus(t, reconciliationRawCall(f, "POST", retryPath, "reconcile-retry-invalid-"+string(rune('1'+i)), body), 400)
	}
	retryBody := `{"reason":"` + retryReason + `","version":` + strconv.FormatInt(failed.Version, 10) + `}`
	first := reconciliationRawCall(f, "POST", retryPath, "reconcile-retry-http01", retryBody)
	mustStatus(t, first, 200)
	var retryReceipt reconciliation.Job
	managedData(t, first, &retryReceipt)
	if retryReceipt.State != "pending" || retryReceipt.Version != failed.Version+1 || retryReceipt.CreationAuditLogID != job.CreationAuditLogID || retryReceipt.PendingCount != retryReceipt.TargetCount {
		t.Fatalf("unexpected retry receipt: %+v", retryReceipt)
	}
	reset := f.call("GET", "/api/v1/admin/reconciliations/"+job.ID+"/targets?outcome=pending", "", f.token, managedBrand, nil)
	mustStatus(t, reset, 200)
	var resetPage reconciliation.TargetPage
	managedData(t, reset, &resetPage)
	if len(resetPage.Items) != 1 || resetPage.Items[0].State != "pending" || resetPage.Items[0].Outcome != nil || resetPage.Items[0].Preview != nil || resetPage.Items[0].CheckedAt != nil || resetPage.Items[0].AuditLogID != nil {
		t.Fatalf("retry did not reset the failed target: %+v", resetPage)
	}
	if _, err := f.service.Process(ctx, 20); err != nil {
		t.Fatal("process retried observation:", err)
	}
	replayBody := `{"version":` + strconv.FormatInt(failed.Version, 10) + `,"reason":"` + retryReason + `"}`
	replay := reconciliationRawCall(f, "POST", retryPath, "reconcile-retry-http01", replayBody)
	mustStatus(t, replay, 200)
	var replayReceipt reconciliation.Job
	managedData(t, replay, &replayReceipt)
	if !reflect.DeepEqual(replayReceipt, retryReceipt) {
		t.Fatalf("retry replay returned live state instead of its pending receipt\nfirst=%+v\nreplay=%+v", retryReceipt, replayReceipt)
	}
	completed := f.call("GET", "/api/v1/admin/reconciliations/"+job.ID, "", f.token, managedBrand, nil)
	mustStatus(t, completed, 200)
	var current reconciliation.Job
	managedData(t, completed, &current)
	if current.State != "completed" || current.Version <= retryReceipt.Version {
		t.Fatalf("retry worker did not complete the new generation: %+v", current)
	}
}

func TestReconciliationHTTPConcurrentCreateAllowsOnlyOneActiveJob(t *testing.T) {
	f := newReconciliationHTTPFixture(t)
	start := make(chan struct{})
	statuses := make(chan int, 2)
	var wait sync.WaitGroup
	for i, reason := range []string{"parallel review one", "parallel review two"} {
		wait.Add(1)
		go func(index int, text string) {
			defer wait.Done()
			<-start
			resp := f.call("POST", "/api/v1/admin/reconciliations", "reconcile-race-000"+string(rune('1'+index)), f.token, managedBrand, map[string]string{"reason": text})
			statuses <- resp.Code
		}(i, reason)
	}
	close(start)
	wait.Wait()
	close(statuses)
	created, conflict := 0, 0
	for status := range statuses {
		switch status {
		case 201:
			created++
		case 409:
			conflict++
		default:
			t.Fatalf("unexpected concurrent create status %d", status)
		}
	}
	if created != 1 || conflict != 1 {
		t.Fatalf("want one create and one active-job conflict, got created=%d conflict=%d", created, conflict)
	}
}

func TestReconciliationHTTPCrossBrandNotFoundAndFreshPermissionChecks(t *testing.T) {
	f := newReconciliationHTTPFixture(t)
	job := reconciliationCreate(t, f, "reconcile-scope-0001", "scope and revocation")
	mustStatus(t, f.call("GET", "/api/v1/admin/reconciliations/"+job.ID, "", f.token, pointsBrandB, nil), 403)
	mustStatus(t, f.call("GET", "/api/v1/admin/reconciliations/"+ids.New(), "", f.token, managedBrand, nil), 404)
	if _, err := f.pool.Exec(context.Background(), `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("POST", "/api/v1/admin/reconciliations", "reconcile-super-0001", f.token, managedBrand, map[string]string{"reason": "super admin remains read only"}), 403)
	if _, err := f.pool.Exec(context.Background(), `UPDATE admin_accounts SET is_super_admin=false WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(context.Background(), `DELETE FROM role_permissions WHERE role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key='wallet.reconcile.brand'`, f.root); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("GET", "/api/v1/admin/reconciliations/"+job.ID, "", f.token, managedBrand, nil), 200)
	mustStatus(t, f.call("POST", "/api/v1/admin/reconciliations", "reconcile-revoked-001", f.token, managedBrand, map[string]string{"reason": "permission revoked"}), 403)
	if _, err := f.pool.Exec(context.Background(), `DELETE FROM role_permissions WHERE role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key='wallet.view.brand'`, f.root); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("GET", "/api/v1/admin/reconciliations/"+job.ID, "", f.token, managedBrand, nil), 403)
	mustStatus(t, f.call("POST", "/api/v1/admin/auth/logout", "reconcile-session-revoke-01", f.token, managedBrand, map[string]any{}), 200)
	mustStatus(t, f.call("GET", "/api/v1/admin/reconciliations/"+job.ID, "", f.token, managedBrand, nil), 401)
}

func TestReconciliationHTTPRejectsInvalidQueriesAndBodyFields(t *testing.T) {
	f := newReconciliationHTTPFixture(t)
	for _, path := range []string{
		"/api/v1/admin/reconciliations?limit=1&limit=2",
		"/api/v1/admin/reconciliations?unexpected=1",
		"/api/v1/admin/reconciliations?limit=0",
		"/api/v1/admin/reconciliations/" + ids.New() + "/targets?outcome=unknown",
		"/api/v1/admin/reconciliations/" + ids.New() + "/targets?outcome=pending&extra=x",
	} {
		mustStatus(t, f.call("GET", path, "", f.token, managedBrand, nil), 400)
	}
	mustStatus(t, f.call("POST", "/api/v1/admin/reconciliations", "reconcile-unknown-01", f.token, managedBrand, map[string]any{"reason": "closed request body", "extra": true}), 400)
}

func TestReconciliationHTTPAuditFailureRollsBackWritesAndWithholdsReadData(t *testing.T) {
	f := newReconciliationHTTPFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `CREATE FUNCTION reject_reconciliation_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'audit unavailable'; END $$`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `CREATE TRIGGER reject_reconciliation_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_reconciliation_audit()`); err != nil {
		t.Fatal(err)
	}
	created := f.call("POST", "/api/v1/admin/reconciliations", "reconcile-audit-fail1", f.token, managedBrand, map[string]string{"reason": "audit rollback"})
	mustStatus(t, created, 503)
	var envelope struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &envelope); err != nil || envelope.Success || len(envelope.Data) != 0 {
		t.Fatalf("failed write returned data: envelope=%+v err=%v", envelope, err)
	}
	if _, err := f.pool.Exec(ctx, `DROP TRIGGER reject_reconciliation_audit ON audit_logs`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `DROP FUNCTION reject_reconciliation_audit()`); err != nil {
		t.Fatal(err)
	}
	listed := f.call("GET", "/api/v1/admin/reconciliations", "", f.token, managedBrand, nil)
	mustStatus(t, listed, 200)
	var page reconciliation.JobPage
	managedData(t, listed, &page)
	if page.TotalCount != "0" || len(page.Items) != 0 {
		t.Fatalf("audit failure left a reconciliation job: %+v", page)
	}
	// Withhold the read audit too: the transaction must not return the page.
	if _, err := f.pool.Exec(ctx, `CREATE FUNCTION reject_reconciliation_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'audit unavailable'; END $$`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `CREATE TRIGGER reject_reconciliation_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_reconciliation_audit()`); err != nil {
		t.Fatal(err)
	}
	failedRead := f.call("GET", "/api/v1/admin/reconciliations", "", f.token, managedBrand, nil)
	mustStatus(t, failedRead, 503)
	if err := json.Unmarshal(failedRead.Body.Bytes(), &envelope); err != nil || envelope.Success || len(envelope.Data) != 0 {
		t.Fatalf("failed read returned data: envelope=%+v err=%v", envelope, err)
	}
}
