package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
)

func exportURL(kind string) string {
	from, to := reportWindow()
	return reportURL("/api/v1/admin/reports/"+kind+"/export", url.Values{"from": {from}, "to": {to}, "group_by": {"day"}})
}
func checkExportBody(t *testing.T, w *httptest.ResponseRecorder, brand, kind string) {
	t.Helper()
	mustStatus(t, w, 200)
	h := w.Header()
	wantVersion := "1"
	if kind == "commission" {
		wantVersion = "2"
	}
	if h.Get("Content-Type") != "text/csv; charset=utf-8" || h.Get("Cache-Control") != "no-store" || h.Get("X-Report-Brand-ID") != brand || h.Get("X-Report-Kind") != kind || h.Get("X-Report-Format-Version") != wantVersion || !uuidPattern.MatchString(h.Get("X-Report-Audit-ID")) {
		t.Fatal("bad export metadata", h)
	}
	if h.Get("Content-Length") != strconv.Itoa(w.Body.Len()) {
		t.Fatal("incorrect length")
	}
	sum := sha256.Sum256(w.Body.Bytes())
	if h.Get("X-Report-SHA256") != hex.EncodeToString(sum[:]) {
		t.Fatal("incorrect content digest")
	}
	if !strings.HasPrefix(w.Body.String(), "\xef\xbb\xbfrecord_type,") {
		t.Fatal("missing UTF8/header")
	}
	rows, e := csv.NewReader(strings.NewReader(strings.TrimPrefix(w.Body.String(), "\xef\xbb\xbf"))).ReadAll()
	if e != nil || len(rows) < 2 || rows[1][0] != "summary" || rows[1][1] != brand {
		t.Fatal("bad CSV", e)
	}
}
func TestReportExportNeedsIndependentReadAndExportScope(t *testing.T) {
	f := managedFixture(t)
	path := exportURL("ledger")
	mustStatus(t, f.call("GET", path, "", f.token, managedBrand, nil), 403)
	grantReportPermission(t, f, "report_ledger.view.brand")
	mustStatus(t, f.call("GET", path, "", f.token, managedBrand, nil), 403)
	grantReportPermission(t, f, "report_ledger.export.brand")
	out := f.call("GET", path, "", f.token, managedBrand, nil)
	checkExportBody(t, out, managedBrand, "ledger")
	mustStatus(t, f.call("GET", exportURL("betting"), "", f.token, managedBrand, nil), 403)
	mustStatus(t, f.call("GET", path, "", f.token, "0199a000-0000-7000-8000-000000000002", nil), 403)
	for _, param := range []string{"&offset=0", "&limit=20", "&unexpected=1", "&game_id=0199a000-0000-7000-8000-000000000003", "&group_by=day"} {
		r := f.call("GET", path+param, "", f.token, managedBrand, nil)
		mustStatus(t, r, 400)
		if strings.Contains(r.Header().Get("Content-Type"), "csv") {
			t.Fatal("failed request emitted download")
		}
	}
	ctx := context.Background()
	if _, e := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key='report_ledger.export.brand'`, f.root); e != nil {
		t.Fatal(e)
	}
	mustStatus(t, f.call("GET", path, "", f.token, managedBrand, nil), 403)
	grantReportPermission(t, f, "report_ledger.export.platform")
	mustStatus(t, f.call("GET", path, "", f.token, managedBrand, nil), 200)
}
func TestReportExportAuditFailureDoesNotEmitCSV(t *testing.T) {
	f := managedFixture(t)
	grantReportPermission(t, f, "report_betting.view.brand")
	grantReportPermission(t, f, "report_betting.export.brand")
	ctx := context.Background()
	if _, e := f.pool.Exec(ctx, `CREATE FUNCTION reject_export_audit_test() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='report.betting.export' THEN RAISE EXCEPTION 'injected export audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_export_audit_test BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_export_audit_test()`); e != nil {
		t.Fatal(e)
	}
	r := f.call("GET", exportURL("betting"), "", f.token, managedBrand, nil)
	mustStatus(t, r, 503)
	if r.Header().Get("Content-Disposition") != "" || strings.Contains(r.Header().Get("Content-Type"), "csv") || strings.HasPrefix(r.Body.String(), "\xef\xbb\xbf") {
		t.Fatal("unaudited partial export emitted")
	}
	var count int
	if e := f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='report.betting.export'`).Scan(&count); e != nil || count != 0 {
		t.Fatal(count, e)
	}
}
func TestReportExportACLWaitObservesCommittedRevocation(t *testing.T) {
	f := managedFixture(t)
	grantReportPermission(t, f, "report_ledger.view.brand")
	grantReportPermission(t, f, "report_ledger.export.brand")
	ctx := context.Background()
	revoke, e := f.pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer revoke.Rollback(ctx)
	if _, e = (adminsys.Store{DB: f.pool}).LockAdminAccess(ctx, revoke, f.root, true); e != nil {
		t.Fatal(e)
	}
	result := make(chan *httptest.ResponseRecorder, 1)
	go func() { result <- f.call("GET", exportURL("ledger"), "", f.token, managedBrand, nil) }()
	waited := false
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if e = f.pool.QueryRow(ctx, `SELECT EXISTS(
 SELECT 1 FROM pg_locks waiting JOIN pg_locks held
 USING(locktype,classid,objid,objsubid)
 WHERE waiting.locktype='advisory' AND NOT waiting.granted
 AND waiting.mode='ShareLock' AND held.granted AND held.pid=$1
 AND waiting.pid<>pg_backend_pid())`, revoke.Conn().PgConn().PID()).Scan(&waited); e != nil {
			t.Fatal(e)
		}
		if waited {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !waited {
		t.Fatal("export did not demonstrably await authorization gate")
	}
	if _, e = revoke.Exec(ctx, `DELETE FROM role_permissions WHERE role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key='report_ledger.export.brand'`, f.root); e != nil {
		t.Fatal(e)
	}
	if e = revoke.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	var r *httptest.ResponseRecorder
	select {
	case r = <-result:
	case <-time.After(3 * time.Second):
		t.Fatal("export remained blocked after ACL revocation committed")
	}
	if r.Code != 401 && r.Code != 403 {
		t.Fatal("stale authorization exported after revocation", r.Code)
	}
	if strings.Contains(r.Header().Get("Content-Type"), "csv") {
		t.Fatal("revoked export bytes emitted")
	}
}
