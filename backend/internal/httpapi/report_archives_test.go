package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/reportarchive"
)

const archiveHTTPPath = "/api/v1/admin/report-archives"

func archiveCall(f managementHTTP, method, path, key, token, brand, body string, actors ...string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://localhost"+path, strings.NewReader(body))
	r.RemoteAddr = "192.0.2.55:12345"
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Brand-ID", brand)
	r.Header.Set("Idempotency-Key", key)
	actor := f.root
	if len(actors) == 1 {
		actor = actors[0]
	}
	r.Header.Set("X-Report-Archive-Actor-ID", actor)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.http.ServeHTTP(w, r)
	return w
}

func archiveBody(revision int64) string {
	return fmt.Sprintf(`{"kind":"daily","period_key":"2020-01-01","expected_revision":%d,"reason":"immutable reporting observation"}`, revision)
}

func TestReportArchiveHTTPReceiptsVersionsCanonicalDownloadAndIndependentRights(t *testing.T) {
	t.Parallel()
	pf := pointsFixture(t)
	f := pf.managementHTTP
	ctx := context.Background()
	grantCorrectionHTTP(t, f, "report_archive.view.brand", "report_archive.create.brand")
	credit := func(amount, key string) {
		t.Helper()
		recharge := pointRecharge(t, pf, amount, "actual recharge before archive observation", key)
		mustStatus(t, pf.call("POST", "/api/v1/admin/recharges/"+recharge.ID+"/confirm", key+"-confirm", f.token, managedBrand, map[string]any{"version": recharge.Version, "reason": "actual offline confirmation"}), 200)
	}
	credit("37", "archive-real-credit-01")
	key, body := "report-archive-original-01", archiveBody(0)
	first := archiveCall(f, "POST", archiveHTTPPath, key, f.token, managedBrand, body)
	mustStatus(t, first, 201)
	var v1, v2, replay reportarchive.Record
	managedData(t, first, &v1)
	if v1.Revision != 1 || v1.PreviousID != nil || v1.PayloadSHA256 == "" || v1.Snapshot.WalletSnapshot.AtSnapshot != v1.SnapshotAt || v1.AuditLogID == "" {
		t.Fatalf("invalid first archive: %+v", v1)
	}
	if v1.Snapshot.WalletSnapshot.Balances.AvailablePoints != "37" || v1.Snapshot.Ledger.EntryCount != "0" {
		t.Fatal("current observed wallet confused with past-period ledger", v1.Snapshot)
	}
	credit("11", "archive-real-credit-02")
	second := archiveCall(f, "POST", archiveHTTPPath, "report-archive-version-02", f.token, managedBrand, archiveBody(1))
	mustStatus(t, second, 201)
	managedData(t, second, &v2)
	if v2.Revision != 2 || v2.PreviousID == nil || *v2.PreviousID != v1.ID || v2.Window != v1.Window {
		t.Fatal("version series or original calendar not retained")
	}
	if v2.Snapshot.WalletSnapshot.Balances.AvailablePoints != "48" || v1.Snapshot.WalletSnapshot.Balances.AvailablePoints != "37" {
		t.Fatal("later observation overwrote original wallet snapshot")
	}
	r := archiveCall(f, "POST", archiveHTTPPath, key, f.token, managedBrand, body)
	mustStatus(t, r, 201)
	managedData(t, r, &replay)
	if !reflect.DeepEqual(v1, replay) {
		t.Fatal("original receipt replaced with current archive")
	}
	mustStatus(t, archiveCall(f, "POST", archiveHTTPPath, key, f.token, managedBrand, archiveBody(1)), 409)
	mustStatus(t, archiveCall(f, "POST", archiveHTTPPath, "report-archive-stale-01", f.token, managedBrand, body), 409)
	r = archiveCall(f, "GET", archiveHTTPPath+"?limit=1&offset=0", "", f.token, managedBrand, "")
	mustStatus(t, r, 200)
	var page reportarchive.Page
	managedData(t, r, &page)
	if page.TotalCount != "2" || len(page.Items) != 1 || page.Items[0].ID != v2.ID {
		t.Fatal("stable archive page incorrect", page)
	}
	path := archiveHTTPPath + "/" + v1.ID + "/download"
	mustStatus(t, archiveCall(f, "GET", path, "", f.token, managedBrand, ""), 403)
	grantCorrectionHTTP(t, f, "report_archive.download.brand")
	r = archiveCall(f, "GET", path, "", f.token, managedBrand, "")
	mustStatus(t, r, 200)
	var raw, savedHash string
	if err := f.pool.QueryRow(ctx, `SELECT payload::text,payload_sha256 FROM report_archives WHERE id=$1`, v1.ID).Scan(&raw, &savedHash); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(r.Body.Bytes())
	if !bytes.Equal(r.Body.Bytes(), []byte(raw)) || hex.EncodeToString(sum[:]) != savedHash || r.Header().Get("X-Content-SHA256") != savedHash || r.Header().Get("Content-Length") != fmt.Sprint(len(raw)) || r.Header().Get("X-Archive-Revision") != "1" || r.Header().Get("X-Archive-Format-Version") != "1" || r.Header().Get("Content-Disposition") != fmt.Sprintf(`attachment; filename="report-archive-%s-v1.json"`, v1.ID) {
		t.Fatal("download was truncated, remarshal changed bytes, or hash/header disagreed")
	}
	var auditJSON []byte
	if err := f.pool.QueryRow(ctx, `SELECT after_json FROM audit_logs WHERE action='report_archive.download' ORDER BY created_at DESC LIMIT 1`).Scan(&auditJSON); err != nil {
		t.Fatal(err)
	}
	var evidence map[string]any
	if json.Unmarshal(auditJSON, &evidence) != nil || len(evidence) != 5 || evidence["payload_sha256"] != savedHash || evidence["bytes"] != float64(len(raw)) || evidence["archive_id"] != v1.ID {
		t.Fatal("download lacks precise audit", string(auditJSON))
	}
	if _, err := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key='report_archive.create.brand'`, f.root); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, archiveCall(f, "POST", archiveHTTPPath, key, f.token, managedBrand, body), 403)
	var count int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM report_archives`).Scan(&count); err != nil || count != 2 {
		t.Fatal("denied replay changed archives", count, err)
	}
	if wallet := pointWallet(t, pf); wallet.AvailablePoints != 48 {
		t.Fatal("archive operations changed actual wallet", wallet)
	}
}

func TestReportArchiveHTTPClosedInputActorAndPlatformReadOnlyScope(t *testing.T) {
	t.Parallel()
	f := managedFixture(t)
	grantCorrectionHTTP(t, f, "report_archive.view.brand", "report_archive.create.brand", "report_archive.download.brand")
	for i, body := range []string{`{}`, `null`, `{"kind":"daily","period_key":"2020-01-01","expected_revision":null,"reason":"observe"}`, `{"kind":"daily","period_key":"2020-01-01","expected_revision":0,"reason":"observe","extra":1}`, `{"kind":"daily","kind":"daily","period_key":"2020-01-01","expected_revision":0,"reason":"observe"}`, `{"kind":"daily","period_key":"2020-02-30","expected_revision":0,"reason":"observe"}`, `{"kind":"monthly","period_key":"2020-01-01","expected_revision":0,"reason":"observe"}`, `{"kind":"daily","period_key":"2020-01-01","expected_revision":9007199254740991,"reason":"observe"}`, `{"kind":"daily","period_key":"2020-01-01","expected_revision":0,"reason":" leading"}`, `{"kind":"daily","period_key":"2020-01-01","expected_revision":0,"reason":"line\tbreak"}`, "{\"kind\":\"daily\",\"period_key\":\"2020-01-01\",\"expected_revision\":0,\"reason\":\"\xff\"}"} {
		mustStatus(t, archiveCall(f, "POST", archiveHTTPPath, fmt.Sprintf("report-archive-closed-%02d", i), f.token, managedBrand, body), 400)
	}
	for _, q := range []string{"?", "?limit=020", "?limit=+20", "?limit=0", "?offset=-1", "?offset=1000001", "?limit=20&limit=20", "?other=1", "?%"} {
		mustStatus(t, archiveCall(f, "GET", archiveHTTPPath+q, "", f.token, managedBrand, ""), 400)
	}
	mustStatus(t, archiveCall(f, "GET", archiveHTTPPath, "", f.token, managedBrand, "null"), 400)
	mustStatus(t, archiveCall(f, "GET", archiveHTTPPath, "", "", managedBrand, ""), 401)
	mustStatus(t, archiveCall(f, "POST", archiveHTTPPath, "report-archive-actor-01", f.token, managedBrand, archiveBody(0), ""), 400)
	mustStatus(t, archiveCall(f, "POST", archiveHTTPPath, "report-archive-actor-01", f.token, managedBrand, archiveBody(0), ids.New()), 401)
	future := fmt.Sprintf(`{"kind":"daily","period_key":"%s","expected_revision":0,"reason":"future observation not allowed"}`, time.Now().UTC().AddDate(1, 0, 0).Format("2006-01-02"))
	mustStatus(t, archiveCall(f, "POST", archiveHTTPPath, "report-archive-future-01", f.token, managedBrand, future), 409)
	mustStatus(t, archiveCall(f, "GET", archiveHTTPPath, "", f.token, pointsBrandB, ""), 403)
	created := archiveCall(f, "POST", archiveHTTPPath, "report-archive-scope-01", f.token, managedBrand, archiveBody(0))
	mustStatus(t, created, 201)
	var record reportarchive.Record
	managedData(t, created, &record)
	ctx := context.Background()
	role := ids.New()
	if _, err := f.pool.Exec(ctx, `INSERT INTO roles(id,code,name) VALUES($1,'archive_platform','Platform archive reader')`, role); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO admin_account_roles(account_id,role_id) VALUES($1,$2)`, f.root, role); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"report_archive.view.platform", "report_archive.download.platform"} {
		if _, err := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,$2)`, role, key); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.pool.Exec(ctx, `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, archiveCall(f, "GET", archiveHTTPPath+"/"+record.ID, "", f.token, pointsBrandB, ""), 404)
	mustStatus(t, archiveCall(f, "GET", archiveHTTPPath+"/"+record.ID+"/download", "", f.token, managedBrand, ""), 200)
	mustStatus(t, archiveCall(f, "POST", archiveHTTPPath, "report-archive-super-01", f.token, managedBrand, archiveBody(1)), 403)
}

func TestReportArchiveHTTPBusyAndAuditFailureAtomicity(t *testing.T) {
	t.Parallel()
	f := managedFixture(t)
	ctx := context.Background()
	grantCorrectionHTTP(t, f, "report_archive.view.brand", "report_archive.create.brand", "report_archive.download.brand")
	lock, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback(ctx)
	if _, err = lock.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended(current_schema()||':report-archive:'||$1||':daily:2020-01-01',0))`, managedBrand); err != nil {
		t.Fatal(err)
	}
	key := "report-archive-busy-01"
	mustStatus(t, archiveCall(f, "POST", archiveHTTPPath, key, f.token, managedBrand, archiveBody(0)), 503)
	var n int
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM idempotency_requests WHERE key=$1`, key).Scan(&n); err != nil || n != 0 {
		t.Fatal("busy stored receipt", n, err)
	}
	if err = lock.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	created := archiveCall(f, "POST", archiveHTTPPath, key, f.token, managedBrand, archiveBody(0))
	mustStatus(t, created, 201)
	var record reportarchive.Record
	managedData(t, created, &record)
	if _, err = f.pool.Exec(ctx, `CREATE FUNCTION fail_archive_http_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action LIKE 'report_archive.%' THEN RAISE EXCEPTION 'test archive audit outage'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_archive_http_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION fail_archive_http_audit()`); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{archiveHTTPPath, archiveHTTPPath + "/" + record.ID, archiveHTTPPath + "/" + record.ID + "/download"} {
		r := archiveCall(f, "GET", path, "", f.token, managedBrand, "")
		mustStatus(t, r, 503)
		if strings.Contains(r.Body.String(), `"snapshot"`) || strings.Contains(r.Body.String(), `"wallet_snapshot"`) || r.Header().Get("Content-Disposition") != "" {
			t.Fatal("audit failure released archive data")
		}
	}
	mustStatus(t, archiveCall(f, "POST", archiveHTTPPath, "report-archive-audit-fail-01", f.token, managedBrand, archiveBody(1)), 503)
	var archives, receipts int
	if err = f.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM report_archives),(SELECT count(*) FROM idempotency_requests WHERE key='report-archive-audit-fail-01')`).Scan(&archives, &receipts); err != nil || archives != 1 || receipts != 0 {
		t.Fatal("audit failure partially committed", archives, receipts, err)
	}
}

func TestReportArchiveHTTPSessionExpiryDuringReadDownloadAndCreateAudit(t *testing.T) {
	for _, action := range []string{"read", "download", "create"} {
		t.Run(action, func(t *testing.T) {
			f := managedFixture(t)
			ctx := context.Background()
			grantCorrectionHTTP(t, f, "report_archive.view.brand", "report_archive.create.brand", "report_archive.download.brand")
			first := archiveCall(f, "POST", archiveHTTPPath, "report-archive-expiry-base-01", f.token, managedBrand, archiveBody(0))
			mustStatus(t, first, 201)
			var record reportarchive.Record
			managedData(t, first, &record)
			lock, err := f.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer lock.Rollback(ctx)
			if _, err = lock.Exec(ctx, `SELECT pg_advisory_xact_lock(77165001)`); err != nil {
				t.Fatal(err)
			}
			if _, err = f.pool.Exec(ctx, `CREATE FUNCTION wait_archive_http_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action LIKE 'report_archive.%' THEN PERFORM pg_advisory_xact_lock(77165001); END IF; RETURN NEW; END $$; CREATE TRIGGER wait_archive_http_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION wait_archive_http_audit()`); err != nil {
				t.Fatal(err)
			}
			result := make(chan *httptest.ResponseRecorder, 1)
			go func() {
				method, path, key, body := "GET", archiveHTTPPath+"/"+record.ID, "", ""
				if action == "download" {
					path += "/download"
				}
				if action == "create" {
					method, path, key, body = "POST", archiveHTTPPath, "report-archive-expired-new-01", archiveBody(1)
				}
				result <- archiveCall(f, method, path, key, f.token, managedBrand, body)
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
				t.Fatal("request did not reach actual audit wait")
			}
			if _, err = f.pool.Exec(ctx, `UPDATE sessions SET expires_at=clock_timestamp()-interval '1 second' WHERE admin_id=$1 AND revoked_at IS NULL`, f.root); err != nil {
				t.Fatal(err)
			}
			if err = lock.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			select {
			case r := <-result:
				mustStatus(t, r, 401)
				if strings.Contains(r.Body.String(), `"wallet_snapshot"`) || r.Header().Get("Content-Disposition") != "" {
					t.Fatal("expired session received saved bytes")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("request remained blocked")
			}
			var archives, audits, receipts int
			if err = f.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM report_archives),(SELECT count(*) FROM audit_logs WHERE action LIKE 'report_archive.%'),(SELECT count(*) FROM idempotency_requests WHERE key='report-archive-expired-new-01')`).Scan(&archives, &audits, &receipts); err != nil || archives != 1 || audits != 1 || receipts != 0 {
				t.Fatal("expiry committed new archive, query audit or receipt", archives, audits, receipts, err)
			}
		})
	}
}

func TestReportArchiveHTTPIntegrityFailureWithholdsBytesAndNewReceipts(t *testing.T) {
	t.Parallel()
	f := managedFixture(t)
	ctx := context.Background()
	grantCorrectionHTTP(t, f, "report_archive.view.brand", "report_archive.create.brand", "report_archive.download.brand")
	first := archiveCall(f, "POST", archiveHTTPPath, "report-archive-corrupt-base-01", f.token, managedBrand, archiveBody(0))
	mustStatus(t, first, 201)
	var record reportarchive.Record
	managedData(t, first, &record)
	// Simulate storage damage only in this owned schema. Ordinary callers cannot
	// bypass the immutable trigger; privileged database-owner attacks remain out
	// of scope. The application must still fail closed on a mismatched checksum.
	if _, err := f.pool.Exec(ctx, `ALTER TABLE report_archives DISABLE TRIGGER guarded_report_archive_version`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE report_archives SET payload_sha256=repeat('0',64) WHERE id=$1`, record.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `ALTER TABLE report_archives ENABLE TRIGGER guarded_report_archive_version`); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{archiveHTTPPath, archiveHTTPPath + "/" + record.ID, archiveHTTPPath + "/" + record.ID + "/download"} {
		r := archiveCall(f, "GET", path, "", f.token, managedBrand, "")
		mustStatus(t, r, 503)
		if !strings.Contains(r.Body.String(), "REPORT_ARCHIVE_INTEGRITY_UNAVAILABLE") || strings.Contains(r.Body.String(), `"wallet_snapshot"`) || r.Header().Get("X-Content-SHA256") != "" || r.Header().Get("Content-Disposition") != "" {
			t.Fatal("damaged storage returned a successful snapshot or file")
		}
	}
	key := "report-archive-corrupt-next-01"
	mustStatus(t, archiveCall(f, "POST", archiveHTTPPath, key, f.token, managedBrand, archiveBody(1)), 503)
	var count int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM idempotency_requests WHERE key=$1`, key).Scan(&count); err != nil || count != 0 {
		t.Fatal("corrupt predecessor cached a new receipt", count, err)
	}
}
