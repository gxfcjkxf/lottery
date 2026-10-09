package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
)

func auditExportPath(extra string) string {
	return "/api/v1/admin/audit/export?from=" + url.QueryEscape(time.Now().Add(-24*time.Hour).UTC().Format(time.RFC3339Nano)) + "&to=" + url.QueryEscape(time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)) + extra
}
func insertAuditExportFact(t *testing.T, f managementHTTP, brand, action, reason string) string {
	t.Helper()
	id := ids.New()
	if _, e := f.pool.Exec(context.Background(), `INSERT INTO audit_logs(id,brand_id,actor_type,actor_id,action,resource_type,resource_id,reason,request_id,before_json,after_json) VALUES($1,$2,'admin',$3,$4,'member',$5,$6,'audit-export-fact','{"status":"normal"}','{"status":"frozen"}')`, id, brand, f.root, action, ids.New(), reason); e != nil {
		t.Fatal(e)
	}
	return id
}
func TestAuditExportQueryAndCSVGuard(t *testing.T) {
	for _, query := range []string{"?unknown=1", "?action=", "?action=a&action=b", "?actor_id=bad", "?from=2026-01-01T00:00:00Z", "?from=2026-01-01T00:00:00Z&to=2026-03-01T00:00:00Z", "?limit=0", "?limit=201", "?offset=100001", "?request_id=%00", "?resource_type=%0A", "?limit=+1"} {
		if _, e := parseAuditQuery(httptest.NewRequest("GET", "http://localhost/"+query, nil), false); e == nil {
			t.Fatal("accepted invalid", query)
		}
	}
	if _, e := parseAuditQuery(httptest.NewRequest("GET", "http://localhost/", nil), true); e == nil {
		t.Fatal("unbounded export")
	}
	if _, e := parseAuditQuery(httptest.NewRequest("GET", "http://localhost/?from=2026-01-01T00:00:00%2B08:00&to=2026-01-02T00:00:00%2B08:00", nil), false); e == nil {
		t.Fatal("non-UTC input accepted")
	}
	for _, raw := range []string{"=1+1", " +cmd", "\t-1", "\ufeff@evil", "normal\ntext"} {
		if auditCSVCell(raw) != "'"+raw {
			t.Fatalf("not neutralized %q", raw)
		}
	}
	if auditCSVCell("ordinary") != "ordinary" {
		t.Fatal("ordinary value changed")
	}
	b := &auditCSVBuffer{}
	if _, e := b.Write(make([]byte, auditExportBytes)); e != nil {
		t.Fatal(e)
	}
	if _, e := b.Write([]byte{1}); e != errAuditExportLarge {
		t.Fatal(e)
	}
}

func TestAuditExportIndependentPermissionScopeFiltersAndEvidence(t *testing.T) {
	f := managedFixture(t)
	ctx := context.Background()
	id := insertAuditExportFact(t, f, managedBrand, "fixture.audit.export", " \t=HYPERLINK(\"bad\")")
	other := "0199a000-0000-7000-8000-000000000002"
	otherID := insertAuditExportFact(t, f, other, "fixture.audit.export", "foreign marker")
	path := auditExportPath("&action=fixture.audit.export&actor_id=" + f.root + "&resource_type=member&request_id=audit-export-fact")
	mustStatus(t, f.call("GET", path, "", f.token, managedBrand, nil), 403)
	grantReportPermission(t, f, "audit.export.brand")
	mustStatus(t, f.call("GET", path, "", f.token, "", nil), 400)
	mustStatus(t, f.call("GET", path, "", f.token, other, nil), 403)
	for _, extra := range []string{"&limit=10", "&offset=0", "&unknown=1", "&action=duplicate"} {
		mustStatus(t, f.call("GET", path+extra, "", f.token, managedBrand, nil), 400)
	}
	w := f.call("GET", path, "", f.token, managedBrand, nil)
	mustStatus(t, w, 200)
	h := w.Header()
	sum := sha256.Sum256(w.Body.Bytes())
	if h.Get("X-Audit-Brand-ID") != managedBrand || h.Get("X-Audit-Row-Count") != "1" || h.Get("X-Audit-SHA256") != hex.EncodeToString(sum[:]) || h.Get("X-Audit-Format-Version") != "1" || !uuidPattern.MatchString(h.Get("X-Audit-Export-ID")) || h.Get("Cache-Control") != "no-store" || h.Get("X-Content-Type-Options") != "nosniff" || h.Get("X-Read-Source") != "" {
		t.Fatal(h)
	}
	if _, e := time.Parse(time.RFC3339Nano, h.Get("X-Audit-Snapshot-At")); e != nil {
		t.Fatal(e)
	}
	rows, e := csv.NewReader(strings.NewReader(strings.TrimPrefix(w.Body.String(), "\xef\xbb\xbf"))).ReadAll()
	if e != nil || len(rows) != 2 || len(rows[0]) != 13 || rows[1][0] != id || rows[1][1] != managedBrand || !strings.HasPrefix(rows[1][7], "' \t=") || strings.Contains(w.Body.String(), otherID) {
		t.Fatal(rows, e)
	}
	var after map[string]any
	var raw []byte
	if e = f.pool.QueryRow(ctx, `SELECT after_json FROM audit_logs WHERE id=$1 AND action='audit.export'`, h.Get("X-Audit-Export-ID")).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(raw, &after); e != nil || after["sha256"] != h.Get("X-Audit-SHA256") || after["row_count"] != float64(1) {
		t.Fatal(after, e)
	}
	read := f.call("GET", "/api/v1/admin/audit?action=fixture.audit.export&limit=1&offset=0", "", f.token, managedBrand, nil)
	mustStatus(t, read, 200)
	var result struct {
		Items []auditItem `json:"items"`
	}
	managedData(t, read, &result)
	if len(result.Items) != 1 || result.Items[0].ID != id || result.Items[0].BrandID == nil || *result.Items[0].BrandID != managedBrand {
		t.Fatal(result)
	}
	// Export-only is insufficient; a platform export grant still needs viewing.
	if _, e = f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key='audit.view.brand'`, f.root); e != nil {
		t.Fatal(e)
	}
	mustStatus(t, f.call("GET", path, "", f.token, managedBrand, nil), 403)
	grantPlatformPermission(t, f, "audit.export.platform")
	mustStatus(t, f.call("GET", path, "", f.token, managedBrand, nil), 403)
	if _, e = f.pool.Exec(ctx, `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); e != nil {
		t.Fatal(e)
	}
	platformToken := platformAdminToken(t, f)
	platformExportPath := strings.Replace(auditExportPath("&action=fixture.audit.export"), "/api/v1/admin", "/api/v1/platform", 1)
	mustStatus(t, f.call("GET", path, "", f.token, managedBrand, nil), 403)
	mustStatus(t, f.call("GET", platformExportPath, "", platformToken, other, nil), 403)
	grantPlatformPermission(t, f, "audit.view.platform")
	mustStatus(t, f.call("GET", platformExportPath, "", platformToken, other, nil), 200)
	mustStatus(t, f.call("GET", strings.Replace(path, "/api/v1/admin", "/api/v1/platform", 1), "", platformToken, "", nil), 400)
}

func assertNoAuditFile(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	if w.Header().Get("Content-Disposition") != "" || w.Header().Get("X-Audit-Export-ID") != "" || strings.Contains(w.Header().Get("Content-Type"), "csv") || strings.HasPrefix(w.Body.String(), "\xef\xbb\xbf") {
		t.Fatal("partial/unaudited file released", w.Header())
	}
}
func TestAuditExportAuditFailureAndCapsReleaseNoFile(t *testing.T) {
	f := managedFixture(t)
	grantReportPermission(t, f, "audit.export.brand")
	ctx := context.Background()
	if _, e := f.pool.Exec(ctx, `CREATE FUNCTION reject_audit_export() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN IF NEW.action='audit.export' THEN RAISE EXCEPTION 'injected audit failure'; END IF; RETURN NEW; END$$;CREATE TRIGGER reject_audit_export BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_audit_export()`); e != nil {
		t.Fatal(e)
	}
	w := f.call("GET", auditExportPath(""), "", f.token, managedBrand, nil)
	mustStatus(t, w, 503)
	assertNoAuditFile(t, w)
	if _, e := f.pool.Exec(ctx, `DROP TRIGGER reject_audit_export ON audit_logs`); e != nil {
		t.Fatal(e)
	}
	if _, e := f.pool.Exec(ctx, `INSERT INTO audit_logs(id,brand_id,actor_type,action,resource_type,reason,request_id) SELECT gen_random_uuid(),$1,'system','fixture.audit.cap','test','','cap' FROM generate_series(1,10001)`, managedBrand); e != nil {
		t.Fatal(e)
	}
	w = f.call("GET", auditExportPath("&action=fixture.audit.cap"), "", f.token, managedBrand, nil)
	mustStatus(t, w, 422)
	assertNoAuditFile(t, w)
	if _, e := f.pool.Exec(ctx, `INSERT INTO audit_logs(id,brand_id,actor_type,action,resource_type,reason,request_id,after_json) VALUES(gen_random_uuid(),$1,'system','fixture.audit.bytes','test','','cap',jsonb_build_object('large',repeat('x',4194305)))`, managedBrand); e != nil {
		t.Fatal(e)
	}
	w = f.call("GET", auditExportPath("&action=fixture.audit.bytes"), "", f.token, managedBrand, nil)
	mustStatus(t, w, 422)
	assertNoAuditFile(t, w)
	w = f.call("GET", "/api/v1/admin/audit?action=fixture.audit.bytes", "", f.token, managedBrand, nil)
	mustStatus(t, w, 422)
	if strings.Contains(w.Body.String(), "items") {
		t.Fatal("oversized read released partial data")
	}
	var count int
	if e := f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='audit.export'`).Scan(&count); e != nil || count != 0 {
		t.Fatal(count, e)
	}
}

func TestAuditReadAggregatePageCapNotPerField(t *testing.T) {
	f := managedFixture(t)
	if _, e := f.pool.Exec(context.Background(), `INSERT INTO audit_logs(id,brand_id,actor_type,action,resource_type,request_id,after_json) SELECT gen_random_uuid(),$1,'system','fixture.audit.page','test','cap',jsonb_build_object('large',repeat('x',1500000)) FROM generate_series(1,3)`, managedBrand); e != nil {
		t.Fatal(e)
	}
	w := f.call("GET", "/api/v1/admin/audit?action=fixture.audit.page&limit=3", "", f.token, managedBrand, nil)
	mustStatus(t, w, 422)
	if strings.Contains(w.Body.String(), "items") {
		t.Fatal("oversized page released partial data")
	}
	w = f.call("GET", "/api/v1/admin/audit?action=fixture.audit.page&limit=1", "", f.token, managedBrand, nil)
	mustStatus(t, w, 200)
}

func TestAuditReadAndExportSessionExpiryDuringAuditWait(t *testing.T) {
	for _, export := range []bool{false, true} {
		t.Run(map[bool]string{false: "read", true: "export"}[export], func(t *testing.T) {
			f := managedFixture(t)
			grantReportPermission(t, f, "audit.export.brand")
			ctx := context.Background()
			wait, e := f.pool.Begin(ctx)
			if e != nil {
				t.Fatal(e)
			}
			defer wait.Rollback(ctx)
			if _, e = wait.Exec(ctx, `SELECT pg_advisory_xact_lock(77171001)`); e != nil {
				t.Fatal(e)
			}
			if _, e = f.pool.Exec(ctx, `CREATE FUNCTION wait_audit_export() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN IF NEW.action IN('audit.view','audit.export') THEN PERFORM pg_advisory_xact_lock(77171001);END IF;RETURN NEW;END$$;CREATE TRIGGER wait_audit_export BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION wait_audit_export()`); e != nil {
				t.Fatal(e)
			}
			path := "/api/v1/admin/audit"
			if export {
				path = auditExportPath("")
			}
			result := make(chan *httptest.ResponseRecorder, 1)
			go func() { result <- f.call("GET", path, "", f.token, managedBrand, nil) }()
			deadline := time.Now().Add(3 * time.Second)
			blocked := false
			for time.Now().Before(deadline) {
				if e = f.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks w JOIN pg_locks h USING(locktype,classid,objid,objsubid) WHERE w.locktype='advisory' AND NOT w.granted AND h.granted AND h.pid=$1 AND w.pid<>pg_backend_pid())`, wait.Conn().PgConn().PID()).Scan(&blocked); e != nil {
					t.Fatal(e)
				}
				if blocked {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
			if !blocked {
				t.Fatal("request never demonstrably waited in audit insert")
			}
			if _, e = f.pool.Exec(ctx, `UPDATE sessions SET expires_at=clock_timestamp()-interval '1 second' WHERE admin_id=$1`, f.root); e != nil {
				t.Fatal(e)
			}
			if e = wait.Commit(ctx); e != nil {
				t.Fatal(e)
			}
			select {
			case w := <-result:
				mustStatus(t, w, 401)
				assertNoAuditFile(t, w)
				if strings.Contains(w.Body.String(), "items") {
					t.Fatal("expired session got data")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("request remained blocked")
			}
			var count int
			if e = f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action IN('audit.view','audit.export')`).Scan(&count); e != nil || count != 0 {
				t.Fatal(count, e)
			}
		})
	}
}

func TestAuditExportACLWaitObservesRevocation(t *testing.T) {
	f := managedFixture(t)
	grantReportPermission(t, f, "audit.export.brand")
	ctx := context.Background()
	lock, e := f.pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer lock.Rollback(ctx)
	if _, e = (adminsys.Store{DB: f.pool}).LockAdminAccess(ctx, lock, f.root, true); e != nil {
		t.Fatal(e)
	}
	result := make(chan *httptest.ResponseRecorder, 1)
	go func() { result <- f.call("GET", auditExportPath(""), "", f.token, managedBrand, nil) }()
	blocked := false
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if e = f.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks w JOIN pg_locks h USING(locktype,classid,objid,objsubid) WHERE w.locktype='advisory' AND NOT w.granted AND h.granted AND h.pid=$1 AND w.pid<>pg_backend_pid())`, lock.Conn().PgConn().PID()).Scan(&blocked); e != nil {
			t.Fatal(e)
		}
		if blocked {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !blocked {
		t.Fatal("request did not await ACL")
	}
	if _, e = lock.Exec(ctx, `DELETE FROM role_permissions WHERE role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key='audit.export.brand'`, f.root); e != nil {
		t.Fatal(e)
	}
	if e = lock.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	select {
	case w := <-result:
		mustStatus(t, w, 403)
		assertNoAuditFile(t, w)
	case <-time.After(3 * time.Second):
		t.Fatal("request remained blocked")
	}
}
