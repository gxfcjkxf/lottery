package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/reportarchive"
)

const (
	archiveTaskPath   = "/api/v1/admin/report-archive-tasks"
	archivePolicyPath = "/api/v1/admin/report-archive-policy"
)

func archiveTaskCode(t *testing.T, r *httptest.ResponseRecorder) string {
	t.Helper()
	var envelope struct {
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(r.Body.Bytes(), &envelope); err != nil || envelope.Error == nil {
		t.Fatalf("expected API error envelope, status=%d body=%s err=%v", r.Code, r.Body.String(), err)
	}
	return envelope.Error.Code
}

func archiveTaskRequest(f managementHTTP, method, path, key, token, brand, actor, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://localhost"+path, strings.NewReader(body))
	r.RemoteAddr = "198.51.100.41:23456"
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Brand-ID", brand)
	r.Header.Set("Idempotency-Key", key)
	r.Header.Set("X-Report-Archive-Actor-ID", actor)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.http.ServeHTTP(w, r)
	return w
}

func grantArchiveTaskPermissions(t *testing.T, f managementHTTP, permissions ...string) {
	t.Helper()
	ctx := context.Background()
	var role string
	if err := f.pool.QueryRow(ctx, `SELECT role_id::text FROM admin_account_roles WHERE account_id=$1 LIMIT 1`, f.root).Scan(&role); err != nil {
		t.Fatal(err)
	}
	for _, permission := range permissions {
		if _, err := f.pool.Exec(ctx, `INSERT INTO permissions(key) VALUES($1) ON CONFLICT DO NOTHING`, permission); err != nil {
			t.Fatal(err)
		}
		if _, err := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,$2) ON CONFLICT DO NOTHING`, role, permission); err != nil {
			t.Fatal(err)
		}
	}
}

// Seed through the core's audited policy service and real automatic worker.
// The failure trigger rejects only the automatic capture audit, allowing the
// worker to persist a genuine failed task through its normal failure path.
func failedAutomaticArchiveTask(t *testing.T, f managementHTTP) reportarchive.AutomaticTask {
	t.Helper()
	ctx := context.Background()
	grantArchiveTaskPermissions(t, f, "report_archive.view.brand", "report_archive_policy.write.brand", "report_archive_task.retry.brand")
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	actor, err := (adminsys.Store{DB: f.pool}).LockAdminAccess(ctx, tx, f.root, false)
	if err != nil {
		t.Fatal(err)
	}
	var zone string
	if err = tx.QueryRow(ctx, `SELECT timezone FROM brands WHERE id=$1`, managedBrand).Scan(&zone); err != nil {
		t.Fatal(err)
	}
	localNow := time.Now().In(mustArchiveLocation(t, zone))
	start := localNow.AddDate(0, 0, -1).Format("2006-01-02")
	_, err = (reportarchive.Service{DB: f.pool}).UpdateAutomaticPolicyTx(ctx, tx, managedBrand, actor, reportarchive.AutomaticPolicyInput{
		Version: 1, DailyEnabled: true, DailyStartPeriod: &start, Reason: "explicit HTTP task fixture start",
	}, points.Metadata{ActorType: "admin", ActorID: actor.ID, RequestID: ids.New(), IP: "198.51.100.41"})
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(ctx, `CREATE FUNCTION fail_report_archive_capture_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='report_archive.create.automatic' THEN RAISE EXCEPTION 'isolated automatic capture failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_report_archive_capture_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION fail_report_archive_capture_audit()`); err != nil {
		t.Fatal(err)
	}
	s := reportarchive.Service{DB: f.pool}
	if n, e := s.DiscoverAutomatic(ctx, 20); e != nil || n == 0 {
		t.Fatalf("discover saved past periods: tasks=%d err=%v", n, e)
	}
	if n, e := s.ProcessAutomatic(ctx, 20); e != nil || n != 0 {
		t.Fatalf("expected capture to fail through worker path: processed=%d err=%v", n, e)
	}
	if _, err = f.pool.Exec(ctx, `DROP TRIGGER fail_report_archive_capture_audit ON audit_logs; DROP FUNCTION fail_report_archive_capture_audit()`); err != nil {
		t.Fatal(err)
	}
	pageTx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer pageTx.Rollback(ctx)
	page, err := s.AutomaticTasksTx(ctx, pageTx, managedBrand, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range page.Items {
		if task.State == "failed" {
			return task
		}
	}
	t.Fatal("worker did not persist a failed automatic task")
	return reportarchive.AutomaticTask{}
}

func mustArchiveLocation(t *testing.T, zone string) *time.Location {
	t.Helper()
	location, err := time.LoadLocation(zone)
	if err != nil {
		t.Fatal(err)
	}
	return location
}

func archiveRetryBody(version int64, reason string) string {
	return fmt.Sprintf(`{"version":%d,"reason":%q}`, version, reason)
}

func TestReportArchiveTaskHTTPDefaultPolicyAndEmptyQueue(t *testing.T) {
	t.Parallel()
	f := managedFixture(t)
	grantArchiveTaskPermissions(t, f, "report_archive.view.brand")
	policy := archiveTaskRequest(f, "GET", archivePolicyPath, "", f.token, managedBrand, "", "")
	mustStatus(t, policy, 200)
	var got reportarchive.AutomaticPolicy
	managedData(t, policy, &got)
	if got.DailyEnabled || got.MonthlyEnabled || got.Version != 1 || got.DailyStartPeriod != nil || got.MonthlyStartPeriod != nil {
		t.Fatalf("automatic archive policy must start disabled and empty: %+v", got)
	}
	page := archiveTaskRequest(f, "GET", archiveTaskPath, "", f.token, managedBrand, "", "")
	mustStatus(t, page, 200)
	var tasks reportarchive.AutomaticTaskPage
	managedData(t, page, &tasks)
	if tasks.Limit != 20 || tasks.Offset != 0 || tasks.TotalCount != "0" || len(tasks.Items) != 0 {
		t.Fatalf("unexpected empty task page: %+v", tasks)
	}
	for _, action := range []string{"report_archive.policy.read", "report_archive.task.list"} {
		var count int
		if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_logs WHERE action=$1 AND resource_type='query'`, action).Scan(&count); err != nil || count != 1 {
			t.Fatalf("read audit %s count=%d err=%v", action, count, err)
		}
	}
}

func TestReportArchiveTaskHTTPStrictInputsAuthorizationAndReadScopes(t *testing.T) {
	t.Parallel()
	f := managedFixture(t)
	grantArchiveTaskPermissions(t, f, "report_archive.view.brand", "report_archive_policy.write.brand", "report_archive_task.retry.brand")
	for i, body := range []string{
		`{}`, `null`, `{"version":null,"reason":"valid reason"}`, `{"version":1,"reason":"valid reason","extra":true}`,
		`{"version":1,"version":1,"reason":"valid reason"}`, `{"version":0,"reason":"valid reason"}`,
		`{"version":1,"reason":" leading"}`, `{"version":1,"reason":"line\nbreak"}`,
		`{"version":1,"reason":"valid reason"} {}`,
		"{\"version\":1,\"reason\":\"" + string([]byte{0xff}) + "\"}",
	} {
		r := archiveTaskRequest(f, "POST", archiveTaskPath+"/0199a000-0000-7000-8000-000000000010/retry", fmt.Sprintf("archive-task-input-%02d", i), f.token, managedBrand, f.root, body)
		mustStatus(t, r, 400)
		if code := archiveTaskCode(t, r); code != "REQUEST_INVALID" {
			t.Errorf("invalid body #%d returned %q: %s", i, code, r.Body.String())
		}
	}
	invalidPolicy := archiveTaskRequest(f, "PUT", archivePolicyPath, "archive-task-policy-write-invalid", f.token, managedBrand, f.root, `{}`)
	mustStatus(t, invalidPolicy, 400)
	if archiveTaskCode(t, invalidPolicy) != "REQUEST_INVALID" {
		t.Fatalf("incomplete policy write was not rejected: %s", invalidPolicy.Body.String())
	}
	for _, q := range []string{"?", "?limit=020", "?limit=+20", "?limit=0", "?offset=-1", "?offset=1000001", "?limit=20&limit=20", "?unknown=1", "?%"} {
		r := archiveTaskRequest(f, "GET", archiveTaskPath+q, "", f.token, managedBrand, "", "")
		mustStatus(t, r, 400)
	}
	mustStatus(t, archiveTaskRequest(f, "GET", archiveTaskPath+"/not-a-uuid", "", f.token, managedBrand, "", ""), 400)
	mustStatus(t, archiveTaskRequest(f, "GET", archiveTaskPath, "", "", managedBrand, "", ""), 401)
	mustStatus(t, archiveTaskRequest(f, "GET", archiveTaskPath, "", f.token, pointsBrandB, "", ""), 403)
	mustStatus(t, archiveTaskRequest(f, "GET", archiveTaskPath, "", f.token, managedBrand, "", "null"), 400)
	// Platform view is a separate read grant and remains read-only.
	var role string
	if err := f.pool.QueryRow(context.Background(), `SELECT role_id::text FROM admin_account_roles WHERE account_id=$1 LIMIT 1`, f.root).Scan(&role); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(context.Background(), `DELETE FROM role_permissions WHERE role_id=$1 AND permission_key='report_archive.view.brand'`, role); err != nil {
		t.Fatal(err)
	}
	grantArchiveTaskPlatformView(t, f)
	mustStatus(t, archiveTaskRequest(f, "GET", archiveTaskPath, "", f.token, managedBrand, "", ""), 403)
	if _, err := f.pool.Exec(context.Background(), `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	platformToken := platformAdminToken(t, f)
	platformPath := strings.Replace(archiveTaskPath, "/api/v1/admin", "/api/v1/platform", 1)
	mustStatus(t, archiveTaskRequest(f, "GET", platformPath, "", platformToken, managedBrand, "", ""), 200)
}

func TestReportArchiveTaskHTTPReadPaginationAndRetryReceiptLifecycle(t *testing.T) {
	t.Parallel()
	pf := pointsFixture(t)
	f := pf.managementHTTP
	task := failedAutomaticArchiveTask(t, f)
	grantArchiveTaskPlatformView(t, f)
	if _, err := f.pool.Exec(context.Background(), `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	platformToken := platformAdminToken(t, f)
	platformTaskPath := strings.Replace(archiveTaskPath, "/api/v1/admin", "/api/v1/platform", 1)
	platformPolicyPath := strings.Replace(archivePolicyPath, "/api/v1/admin", "/api/v1/platform", 1)
	foreign := archiveTaskRequest(f, "GET", platformTaskPath+"/"+task.ID, "", platformToken, pointsBrandB, "", "")
	mustStatus(t, foreign, 404)
	if archiveTaskCode(t, foreign) != "REPORT_ARCHIVE_NOT_FOUND" {
		t.Fatalf("cross-brand task read leaked or used wrong error: %s", foreign.Body.String())
	}
	policy := archiveTaskRequest(f, "GET", platformPolicyPath, "", platformToken, managedBrand, "", "")
	mustStatus(t, policy, 200)
	page := archiveTaskRequest(f, "GET", platformTaskPath+"?limit=1&offset=0", "", platformToken, managedBrand, "", "")
	mustStatus(t, page, 200)
	var got reportarchive.AutomaticTaskPage
	managedData(t, page, &got)
	if got.TotalCount != "1" || len(got.Items) != 1 || got.Items[0].ID != task.ID || got.Items[0].State != "failed" {
		t.Fatalf("task page mismatch: %+v", got)
	}
	one := archiveTaskRequest(f, "GET", platformTaskPath+"/"+task.ID, "", platformToken, managedBrand, "", "")
	mustStatus(t, one, 200)
	var detailed reportarchive.AutomaticTask
	managedData(t, one, &detailed)
	if detailed.ID != task.ID || detailed.State != "failed" {
		t.Fatalf("task read mismatch: %+v", detailed)
	}
	missing := archiveTaskRequest(f, "GET", platformTaskPath+"/0199a000-0000-7000-8000-000000000011", "", platformToken, managedBrand, "", "")
	mustStatus(t, missing, 404)
	if archiveTaskCode(t, missing) != "REPORT_ARCHIVE_NOT_FOUND" {
		t.Fatalf("unexpected missing-task code: %s", missing.Body.String())
	}
	if _, err := f.pool.Exec(context.Background(), `UPDATE admin_accounts SET is_super_admin=false WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	key := "archive-task-retry-receipt-01"
	body := archiveRetryBody(task.Version, "retry after isolated capture failure")
	missingActor := archiveTaskRequest(f, "POST", archiveTaskPath+"/"+task.ID+"/retry", "archive-task-retry-no-actor", f.token, managedBrand, "", body)
	mustStatus(t, missingActor, 400)
	if archiveTaskCode(t, missingActor) != "REPORT_ARCHIVE_INPUT_INVALID" {
		t.Fatalf("missing retry actor was not rejected as invalid input: %s", missingActor.Body.String())
	}
	changedActor := archiveTaskRequest(f, "POST", archiveTaskPath+"/"+task.ID+"/retry", "archive-task-retry-other-actor", f.token, managedBrand, ids.New(), body)
	mustStatus(t, changedActor, 401)
	if archiveTaskCode(t, changedActor) != "AUTH_ACTOR_CONTEXT_CHANGED" {
		t.Fatalf("changed retry actor was not rejected: %s", changedActor.Body.String())
	}
	first := archiveTaskRequest(f, "POST", archiveTaskPath+"/"+task.ID+"/retry", key, f.token, managedBrand, f.root, body)
	mustStatus(t, first, 200)
	var accepted reportarchive.AutomaticTask
	managedData(t, first, &accepted)
	if accepted.State != "pending" || accepted.Version != task.Version+1 || accepted.AttemptCount != task.AttemptCount || accepted.ArchiveID != nil {
		t.Fatalf("retry did not return the pending acknowledgement: %+v", accepted)
	}
	stale := archiveTaskRequest(f, "POST", archiveTaskPath+"/"+task.ID+"/retry", "archive-task-retry-stale-version", f.token, managedBrand, f.root, archiveRetryBody(task.Version-1, "stale task version"))
	mustStatus(t, stale, 409)
	if archiveTaskCode(t, stale) != "REPORT_ARCHIVE_VERSION_CONFLICT" {
		t.Fatalf("stale retry version used wrong conflict: %s", stale.Body.String())
	}
	wrongState := archiveTaskRequest(f, "POST", archiveTaskPath+"/"+task.ID+"/retry", "archive-task-retry-wrong-state", f.token, managedBrand, f.root, archiveRetryBody(accepted.Version, "retry while already pending"))
	mustStatus(t, wrongState, 409)
	if archiveTaskCode(t, wrongState) != "REPORT_ARCHIVE_STATE_CONFLICT" {
		t.Fatalf("pending task retry used wrong conflict: %s", wrongState.Body.String())
	}
	conflict := archiveTaskRequest(f, "POST", archiveTaskPath+"/"+task.ID+"/retry", key, f.token, managedBrand, f.root, archiveRetryBody(task.Version, "different exact request"))
	mustStatus(t, conflict, 409)
	if _, err := f.pool.Exec(context.Background(), `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	superAdminRetry := archiveTaskRequest(f, "POST", archiveTaskPath+"/"+task.ID+"/retry", "archive-task-retry-super-admin", f.token, managedBrand, f.root, body)
	mustStatus(t, superAdminRetry, 403)
	if _, err := f.pool.Exec(context.Background(), `UPDATE admin_accounts SET is_super_admin=false WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	if n, err := (reportarchive.Service{DB: f.pool}).ProcessAutomatic(context.Background(), 20); err != nil || n != 1 {
		t.Fatalf("worker did not finish retried task: processed=%d err=%v", n, err)
	}
	replay := archiveTaskRequest(f, "POST", archiveTaskPath+"/"+task.ID+"/retry", key, f.token, managedBrand, f.root, body)
	mustStatus(t, replay, 200)
	var replayed reportarchive.AutomaticTask
	managedData(t, replay, &replayed)
	if replayed.State != "pending" || replayed.Version != accepted.Version || replayed.ArchiveID != nil {
		t.Fatalf("same-key retry did not preserve original acknowledgement: got=%+v ack=%+v", replayed, accepted)
	}
	if _, err := f.pool.Exec(context.Background(), `DELETE FROM role_permissions WHERE role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key='report_archive_task.retry.brand'`, f.root); err != nil {
		t.Fatal(err)
	}
	revoked := archiveTaskRequest(f, "POST", archiveTaskPath+"/"+task.ID+"/retry", key, f.token, managedBrand, f.root, body)
	mustStatus(t, revoked, 403)
	wallet := pointWallet(t, pf)
	if wallet.AvailablePoints != 0 || wallet.FrozenPoints != 0 {
		t.Fatalf("archive task operations changed real points: %+v", wallet)
	}
	var audits int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_logs WHERE action IN('report_archive.policy.read','report_archive.task.list','report_archive.task.read','report_archive.task.retry')`).Scan(&audits); err != nil || audits < 5 {
		t.Fatalf("missing query/retry audit entries: count=%d err=%v", audits, err)
	}
	var beforeJSON, afterJSON []byte
	var ip, resourceType, resourceID string
	if err := f.pool.QueryRow(context.Background(), `SELECT before_json,after_json,ip_address,resource_type,resource_id FROM audit_logs WHERE action='report_archive.task.retry' AND resource_id=$1`, task.ID).Scan(&beforeJSON, &afterJSON, &ip, &resourceType, &resourceID); err != nil {
		t.Fatal(err)
	}
	var before, after map[string]any
	if json.Unmarshal(beforeJSON, &before) != nil || json.Unmarshal(afterJSON, &after) != nil {
		t.Fatalf("invalid retry audit JSON: before=%s after=%s", beforeJSON, afterJSON)
	}
	wantFields := []string{"state", "version", "attempt_count", "archive_id", "last_error_code"}
	fieldsMatch := len(before) == len(wantFields) && len(after) == len(wantFields)
	for _, field := range wantFields {
		_, hasBefore := before[field]
		_, hasAfter := after[field]
		fieldsMatch = fieldsMatch && hasBefore && hasAfter
	}
	if !fieldsMatch || before["state"] != "failed" || after["state"] != "pending" || ip != "198.51.100.41" || resourceType != "report_archive_task" || resourceID != task.ID {
		t.Fatalf("retry audit evidence mismatch: before=%s after=%s ip=%q resource=%s/%s", beforeJSON, afterJSON, ip, resourceType, resourceID)
	}
}

func grantArchiveTaskPlatformView(t *testing.T, f managementHTTP) {
	t.Helper()
	ctx := context.Background()
	role := ids.New()
	if _, err := f.pool.Exec(ctx, `INSERT INTO roles(id,code,name) VALUES($1,'archive_platform_reader','Archive platform reader')`, role); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO permissions(key) VALUES('report_archive.view.platform') ON CONFLICT DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	for _, q := range []struct {
		sql  string
		args []any
	}{{`INSERT INTO role_permissions(role_id,permission_key) VALUES($1,'report_archive.view.platform')`, []any{role}}, {`INSERT INTO admin_account_roles(account_id,role_id) VALUES($1,$2)`, []any{f.root, role}}} {
		if _, err := f.pool.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReportArchiveTaskHTTPReadAndRetryAuditWaitRechecksSession(t *testing.T) {
	for _, action := range []string{"read", "retry"} {
		t.Run(action, func(t *testing.T) {
			f := managedFixture(t)
			task := failedAutomaticArchiveTask(t, f)
			ctx := context.Background()
			lock, err := f.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer lock.Rollback(ctx)
			if _, err = lock.Exec(ctx, `SELECT pg_advisory_xact_lock(77165009)`); err != nil {
				t.Fatal(err)
			}
			auditAction := "report_archive.task.read"
			method, path, key, body := "GET", archiveTaskPath+"/"+task.ID, "", ""
			if action == "retry" {
				auditAction = "report_archive.task.retry"
				method, path, key, body = "POST", archiveTaskPath+"/"+task.ID+"/retry", "archive-task-expiring-retry", archiveRetryBody(task.Version, "retry during audit wait")
			}
			ddl := fmt.Sprintf(`CREATE FUNCTION wait_report_archive_task_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='%s' THEN PERFORM pg_advisory_xact_lock(77165009); END IF; RETURN NEW; END $$; CREATE TRIGGER wait_report_archive_task_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION wait_report_archive_task_audit()`, auditAction)
			if _, err = f.pool.Exec(ctx, ddl); err != nil {
				t.Fatal(err)
			}
			result := make(chan *httptest.ResponseRecorder, 1)
			go func() {
				result <- archiveTaskRequest(f, method, path, key, f.token, managedBrand, f.root, body)
			}()
			waited := false
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				if err = f.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks w JOIN pg_locks h USING(locktype,classid,objid,objsubid) WHERE w.locktype='advisory' AND NOT w.granted AND h.granted AND h.pid=$1 AND w.pid<>pg_backend_pid())`, lock.Conn().PgConn().PID()).Scan(&waited); err != nil {
					t.Fatal(err)
				}
				if waited {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
			if !waited {
				t.Fatal("request did not reach the actual query/retry audit wait")
			}
			if _, err = f.pool.Exec(ctx, `UPDATE sessions SET expires_at=clock_timestamp()-interval '1 second' WHERE admin_id=$1 AND revoked_at IS NULL`, f.root); err != nil {
				t.Fatal(err)
			}
			if err = lock.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			select {
			case response := <-result:
				mustStatus(t, response, 401)
				if archiveTaskCode(t, response) != "AUTH_SESSION_REVOKED" || strings.Contains(response.Body.String(), `"creation_audit_log_id"`) {
					t.Fatalf("expired session retained access or wrong error: %s", response.Body.String())
				}
			case <-time.After(3 * time.Second):
				t.Fatal("request remained blocked after audit wait released")
			}
			var state string
			var version int64
			if err = f.pool.QueryRow(ctx, `SELECT state,version FROM report_archive_automatic_tasks WHERE id=$1`, task.ID).Scan(&state, &version); err != nil || state != "failed" || version != task.Version {
				t.Fatalf("session expiry committed a partial retry: state=%s version=%d err=%v", state, version, err)
			}
			if action == "retry" {
				var receipts int
				if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM idempotency_requests WHERE key=$1`, key).Scan(&receipts); err != nil || receipts != 0 {
					t.Fatalf("expired retry stored an acknowledgement: count=%d err=%v", receipts, err)
				}
			}
		})
	}
}

func TestReportArchiveTaskHTTPRetryBusyAndAuditFailureRollback(t *testing.T) {
	t.Parallel()
	f := managedFixture(t)
	task := failedAutomaticArchiveTask(t, f)
	ctx := context.Background()
	body := archiveRetryBody(task.Version, "retry after isolated capture failure")
	key := "archive-task-retry-busy-01"
	lock, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback(ctx)
	if _, err = lock.Exec(ctx, `SELECT id FROM report_archive_automatic_tasks WHERE id=$1 FOR UPDATE`, task.ID); err != nil {
		t.Fatal(err)
	}
	busy := archiveTaskRequest(f, "POST", archiveTaskPath+"/"+task.ID+"/retry", key, f.token, managedBrand, f.root, body)
	mustStatus(t, busy, 503)
	var receipts int
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM idempotency_requests WHERE key=$1`, key).Scan(&receipts); err != nil || receipts != 0 {
		t.Fatalf("lock-busy retry cached a receipt: count=%d err=%v", receipts, err)
	}
	if err = lock.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(ctx, `CREATE FUNCTION fail_report_archive_task_retry_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action IN('report_archive.task.read','report_archive.task.retry') THEN RAISE EXCEPTION 'archive task audit unavailable'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_report_archive_task_retry_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION fail_report_archive_task_retry_audit()`); err != nil {
		t.Fatal(err)
	}
	readFailure := archiveTaskRequest(f, "GET", archiveTaskPath+"/"+task.ID, "", f.token, managedBrand, "", "")
	mustStatus(t, readFailure, 503)
	if strings.Contains(readFailure.Body.String(), `"creation_audit_log_id"`) {
		t.Fatalf("audit failure released task data: %s", readFailure.Body.String())
	}
	auditKey := "archive-task-retry-audit-fail-01"
	auditFailure := archiveTaskRequest(f, "POST", archiveTaskPath+"/"+task.ID+"/retry", auditKey, f.token, managedBrand, f.root, body)
	mustStatus(t, auditFailure, 503)
	if _, err = f.pool.Exec(ctx, `DROP TRIGGER fail_report_archive_task_retry_audit ON audit_logs; DROP FUNCTION fail_report_archive_task_retry_audit()`); err != nil {
		t.Fatal(err)
	}
	var state string
	var version int64
	if err = f.pool.QueryRow(ctx, `SELECT state,version FROM report_archive_automatic_tasks WHERE id=$1`, task.ID).Scan(&state, &version); err != nil || state != "failed" || version != task.Version {
		t.Fatalf("audit failure partially retried task: state=%s version=%d err=%v", state, version, err)
	}
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM idempotency_requests WHERE key=$1`, auditKey).Scan(&receipts); err != nil || receipts != 0 {
		t.Fatalf("audit failure stored acknowledgement: count=%d err=%v", receipts, err)
	}
}
