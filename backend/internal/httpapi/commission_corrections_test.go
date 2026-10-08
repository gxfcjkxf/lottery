package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
)

const correctionHTTPPrefix = "/api/v1/admin"

func correctionCall(f managementHTTP, method, path, key, token, brand, body string, actors ...string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://localhost"+path, strings.NewReader(body))
	r.RemoteAddr = "192.0.2.55:12345"
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Brand-ID", brand)
	r.Header.Set("Idempotency-Key", key)
	a := f.root
	if len(actors) == 1 {
		a = actors[0]
	}
	r.Header.Set("X-Commission-Correction-Actor-ID", a)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.http.ServeHTTP(w, r)
	return w
}

func grantCorrectionHTTP(t *testing.T, f managementHTTP, keys ...string) {
	t.Helper()
	for _, key := range keys {
		if _, err := f.pool.Exec(context.Background(), `INSERT INTO role_permissions(role_id,permission_key) SELECT role_id,$2 FROM admin_account_roles WHERE account_id=$1 ON CONFLICT DO NOTHING`, f.root, key); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCommissionCorrectionHTTPReadsAuthStrictQueriesAndIndependentRights(t *testing.T) {
	t.Parallel()
	f := managedFixture(t)
	for _, path := range []string{correctionPolicyPath, correctionPlansPath, correctionExecutionsPath} {
		mustStatus(t, correctionCall(f, "GET", correctionHTTPPrefix+path, "", "", managedBrand, ""), 401)
		mustStatus(t, correctionCall(f, "GET", correctionHTTPPrefix+path, "", f.token, managedBrand, ""), 403)
	}
	grantCorrectionHTTP(t, f, "commission.view.brand")
	for _, path := range []string{correctionPlansPath, correctionExecutionsPath} {
		r := correctionCall(f, "GET", correctionHTTPPrefix+path, "", f.token, managedBrand, "")
		mustStatus(t, r, 200)
		var page struct {
			Items []json.RawMessage `json:"items"`
			Total string            `json:"total_count"`
		}
		managedData(t, r, &page)
		if page.Total != "0" || page.Items == nil || len(page.Items) != 0 {
			t.Fatalf("empty read created a projection: %+v", page)
		}
		for _, q := range []string{"?", "?limit=+20", "?limit=020", "?offset=-1", "?limit=0", "?limit=101", "?offset=1000001", "?limit=20&limit=30", "?other=1", "?offset=", "?%"} {
			mustStatus(t, correctionCall(f, "GET", correctionHTTPPrefix+path+q, "", f.token, managedBrand, ""), 400)
		}
		mustStatus(t, correctionCall(f, "GET", correctionHTTPPrefix+path, "", f.token, managedBrand, "null"), 400)
		mustStatus(t, correctionCall(f, "GET", correctionHTTPPrefix+path+"/"+ids.New(), "", f.token, managedBrand, ""), 404)
		mustStatus(t, correctionCall(f, "GET", correctionHTTPPrefix+path+"/"+ids.New()+"/targets", "", f.token, managedBrand, ""), 404)
	}
	for _, entry := range []struct{ path, permission string }{
		{correctionPlansPath + "/" + ids.New() + "/retry", "commission_correction.retry"},
		{correctionExecutionsPath + "/" + ids.New() + "/approve", "commission_correction.approve"},
		{correctionExecutionsPath + "/" + ids.New() + "/continue", "commission_correction.continue"},
		{correctionExecutionsPath + "/" + ids.New() + "/retry", "commission_correction.execute_retry"},
	} {
		mustStatus(t, correctionCall(f, "POST", correctionHTTPPrefix+entry.path, ids.New(), f.token, managedBrand, `{"version":1,"reason":"must hold distinct permission"}`), 403)
		var n int
		err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_logs WHERE action='access.denied' AND after_json->>'permission'=$1`, entry.permission).Scan(&n)
		if err != nil || n != 1 {
			t.Fatalf("permission denial not recorded for %s: n=%d err=%v", entry.permission, n, err)
		}
	}
	grantCorrectionHTTP(t, f, "commission_correction.retry.brand")
	mustStatus(t, correctionCall(f, "POST", correctionHTTPPrefix+correctionExecutionsPath+"/"+ids.New()+"/retry", ids.New(), f.token, managedBrand, `{"version":1,"reason":"planning retry does not authorize money"}`), 403)
	mustStatus(t, correctionCall(f, "GET", correctionHTTPPrefix+correctionExecutionsPath, "", f.token, pointsBrandB, ""), 403)
}

func TestCommissionCorrectionHTTPPolicyActorReplayAndClosedBody(t *testing.T) {
	t.Parallel()
	f := managedFixture(t)
	path := correctionHTTPPrefix + correctionPolicyPath
	grantCorrectionHTTP(t, f, "commission.view.brand", "commission_correction_policy.write.brand")
	for _, body := range []string{`{}`, `null`, `{"version":1,"enabled":true}`, `{"version":1,"enabled":null,"reason":"enable"}`, `{"version":1,"enabled":true,"reason":"enable","extra":1}`, `{"version":1,"version":1,"enabled":true,"reason":"enable"}`, `{"version":1,"enabled":true,"reason":"newline\nhere"}`, `{"version":1,"enabled":true,"reason":" leading"}`, `{"version":1,"enabled":true,"reason":"enable"} {}`, "{\"version\":1,\"enabled\":true,\"reason\":\"\xff\"}"} {
		mustStatus(t, correctionCall(f, "PUT", path, "correction-policy-closed-01", f.token, managedBrand, body), 400)
	}
	body := `{"version":1,"enabled":true,"reason":"explicit financial correction opt-in"}`
	key := "correction-policy-receipt-01"
	for _, a := range []string{"", ids.New()} {
		want := 401
		if a == "" {
			want = 400
		}
		mustStatus(t, correctionCall(f, "PUT", path, key, f.token, managedBrand, body, a), want)
	}
	first := correctionCall(f, "PUT", path, key, f.token, managedBrand, body)
	mustStatus(t, first, 200)
	var original commission.CorrectionExecutionPolicy
	managedData(t, first, &original)
	if original.Version != 2 || !original.Enabled || original.AuditLogID == "" {
		t.Fatalf("invalid original opt-in receipt: %+v", original)
	}
	mustStatus(t, correctionCall(f, "PUT", path, "correction-policy-disable-01", f.token, managedBrand, `{"version":2,"enabled":false,"reason":"disable subsequent financial work"}`), 200)
	replayed := correctionCall(f, "PUT", path, key, f.token, managedBrand, body)
	mustStatus(t, replayed, 200)
	var replay commission.CorrectionExecutionPolicy
	managedData(t, replayed, &replay)
	if replay != original {
		t.Fatalf("old receipt was rewritten to current policy: old=%+v new=%+v", original, replay)
	}
	mustStatus(t, correctionCall(f, "PUT", path, key, f.token, managedBrand, `{"version":1,"enabled":false,"reason":"changed original intent"}`), 409)
	mustStatus(t, correctionCall(f, "PUT", path, key, f.token, managedBrand, body, ids.New()), 401)
	if _, err := f.pool.Exec(context.Background(), `DELETE FROM role_permissions WHERE role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key='commission_correction_policy.write.brand'`, f.root); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, correctionCall(f, "PUT", path, key, f.token, managedBrand, body), 403)
	var version int64
	if err := f.pool.QueryRow(context.Background(), `SELECT version FROM brand_commission_correction_policies WHERE brand_id=$1`, managedBrand).Scan(&version); err != nil || version != 3 {
		t.Fatalf("denied replay modified policy: version=%d err=%v", version, err)
	}
}

func TestCommissionCorrectionHTTPBusyAndAuditFailureDoNotCacheOrReleaseData(t *testing.T) {
	t.Parallel()
	f := managedFixture(t)
	ctx := context.Background()
	path := correctionHTTPPrefix + correctionPolicyPath
	grantCorrectionHTTP(t, f, "commission.view.brand", "commission_correction_policy.write.brand")
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT 1 FROM brand_commission_correction_policies WHERE brand_id=$1 FOR UPDATE`, managedBrand); err != nil {
		t.Fatal(err)
	}
	key := "correction-busy-not-cached-01"
	body := `{"version":1,"enabled":true,"reason":"independent gate change"}`
	mustStatus(t, correctionCall(f, "PUT", path, key, f.token, managedBrand, body), 503)
	var n int
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM idempotency_requests WHERE key=$1`, key).Scan(&n); err != nil || n != 0 {
		t.Fatalf("busy attempt stored receipt: n=%d err=%v", n, err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, correctionCall(f, "PUT", path, key, f.token, managedBrand, body), 200)
	if _, err = f.pool.Exec(ctx, `CREATE FUNCTION fail_correction_read_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='commission.correction_policy.read' THEN RAISE EXCEPTION 'test read audit outage'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_correction_read_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION fail_correction_read_audit()`); err != nil {
		t.Fatal(err)
	}
	r := correctionCall(f, "GET", path, "", f.token, managedBrand, "")
	mustStatus(t, r, 503)
	if strings.Contains(r.Body.String(), `"enabled"`) || strings.Contains(r.Body.String(), `"audit_log_id"`) {
		t.Fatal("read audit failure released policy DTO")
	}
}

func TestCommissionCorrectionHTTPSessionExpiryDuringQueryAndWriteAuditRollsBack(t *testing.T) {
	for _, write := range []bool{false, true} {
		t.Run(map[bool]string{false: "query", true: "write"}[write], func(t *testing.T) {
			f := managedFixture(t)
			ctx := context.Background()
			grantCorrectionHTTP(t, f, "commission.view.brand", "commission_correction_policy.write.brand")
			lock, err := f.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer lock.Rollback(ctx)
			const key int64 = 77159001
			if _, err = lock.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, key); err != nil {
				t.Fatal(err)
			}
			if _, err = f.pool.Exec(ctx, `CREATE FUNCTION wait_correction_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action IN('commission.correction_policy.read','commission.correction_policy.update') THEN PERFORM pg_advisory_xact_lock(77159001); END IF; RETURN NEW; END $$; CREATE TRIGGER wait_correction_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION wait_correction_audit()`); err != nil {
				t.Fatal(err)
			}
			response := make(chan *httptest.ResponseRecorder, 1)
			go func() {
				method, body, k := "GET", "", ""
				if write {
					method, body, k = "PUT", `{"version":1,"enabled":true,"reason":"expire while policy audit waits"}`, "correction-expired-write-01"
				}
				response <- correctionCall(f, method, correctionHTTPPrefix+correctionPolicyPath, k, f.token, managedBrand, body)
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
				t.Fatal("correction request did not reach its audit wait")
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
				t.Fatal("expired request stayed blocked")
			}
			mustStatus(t, r, 401)
			if strings.Contains(r.Body.String(), `"enabled"`) || strings.Contains(r.Body.String(), managedBrand) {
				t.Fatal("expired session received sensitive correction DTO")
			}
			var v, audits, receipts int64
			err = f.pool.QueryRow(ctx, `SELECT version,(SELECT count(*) FROM audit_logs WHERE action IN('commission.correction_policy.read','commission.correction_policy.update')),(SELECT count(*) FROM idempotency_requests WHERE operation='admin.commission_correction_policy.update') FROM brand_commission_correction_policies WHERE brand_id=$1`, managedBrand).Scan(&v, &audits, &receipts)
			if err != nil || v != 1 || audits != 0 || receipts != 0 {
				t.Fatalf("expired audit transaction persisted changes: v=%d audits=%d receipts=%d err=%v", v, audits, receipts, err)
			}
		})
	}
}
