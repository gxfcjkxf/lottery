package httpapi

import (
	"context"
	"encoding/csv"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func attributionReportHTTPCall(f managementHTTP, path, token, brand string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", "http://localhost"+path, strings.NewReader(""))
	r.RemoteAddr = "192.0.2.55:12345"
	r.Header.Set("X-Brand-ID", brand)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.http.ServeHTTP(w, r)
	return w
}

func TestAttributionReportHTTPRequiresSeparateExplicitRightsAndAuditsBeforeRelease(t *testing.T) {
	f := managedFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE permission_key LIKE 'report_attribution.%' AND role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1)`, f.root); err != nil {
		t.Fatal(err)
	}
	from, to := reportWindow()
	query := url.Values{"from": {from}, "to": {to}, "group_by": {"day"}, "join_method": {"legacy"}}
	readURL := reportURL("/api/v1/admin/reports/attribution", query)
	exportURL := reportURL("/api/v1/admin/reports/attribution/export", query)
	call := func(path, brand string) *httptest.ResponseRecorder {
		return attributionReportHTTPCall(f, path, f.token, brand)
	}
	grantReportPermission(t, f, "report_betting.view.brand")
	grantReportPermission(t, f, "report_commission.view.brand")
	grantReportPermission(t, f, "report_reward.view.brand")

	if response := call(readURL, managedBrand); response.Code != 403 {
		t.Fatalf("no attribution permission status=%d body=%s", response.Code, response.Body.String())
	}
	if response := call(exportURL, managedBrand); response.Code != 403 {
		t.Fatalf("no attribution permissions allowed export: status=%d body=%s", response.Code, response.Body.String())
	}
	grantReportPermission(t, f, "report_attribution.view.brand")
	if response := call(readURL, managedBrand); response.Code != 200 {
		t.Fatalf("view permission failed: status=%d body=%s", response.Code, response.Body.String())
	}
	if response := call(exportURL, managedBrand); response.Code != 403 {
		t.Fatalf("view permission alone allowed export: status=%d body=%s", response.Code, response.Body.String())
	}
	if _, err := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE permission_key='report_attribution.view.brand' AND role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1)`, f.root); err != nil {
		t.Fatal(err)
	}
	grantReportPermission(t, f, "report_attribution.export.brand")
	if response := call(readURL, managedBrand); response.Code != 403 {
		t.Fatalf("export permission alone allowed read: status=%d body=%s", response.Code, response.Body.String())
	}
	if response := call(exportURL, managedBrand); response.Code != 403 {
		t.Fatalf("export permission alone satisfied read+export: status=%d body=%s", response.Code, response.Body.String())
	}
	grantReportPermission(t, f, "report_attribution.view.brand")
	export := call(exportURL, managedBrand)
	if export.Code != 200 || export.Header().Get("X-Report-Audit-ID") == "" || export.Header().Get("X-Report-Agent-Scope") != "direct" || export.Header().Get("X-Report-Join-Method") != "legacy" {
		t.Fatalf("separate view+export permissions failed: status=%d headers=%v body=%s", export.Code, export.Header(), export.Body.String())
	}
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(export.Body.String(), "\xef\xbb\xbf"))).ReadAll()
	if err != nil || len(rows) != 2 || len(rows[0]) != 29 || rows[1][0] != "summary" || rows[1][10] != "direct" || rows[1][11] != "legacy" {
		t.Fatalf("attribution export is not complete header+summary CSV: rows=%d err=%v body=%q", len(rows), err, export.Body.String())
	}
	if got := strings.Join(rows[0], ","); got != "record_type,brand_id,snapshot_at,timezone,from,to,group_by,game_id,member_id,agent_id,agent_scope,join_method,key,label,order_count,stake_points,placed_count,won_count,lost_count,abnormal_count,cancelled_count,refund_points,settled_stake_points,unfinalized_stake_points,abnormal_stake_points,current_prize_points,correction_open_count,final_lost_stake_points,legacy_attribution_count" {
		t.Fatalf("CSV column order drifted: %s", got)
	}
	if response := call(readURL, "0199a000-0000-7000-8000-000000000002"); response.Code != 403 {
		t.Fatalf("brand permission escaped selected brand scope: status=%d body=%s", response.Code, response.Body.String())
	}
	if _, err := f.pool.Exec(ctx, `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE permission_key LIKE 'report_attribution.%' AND role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1)`, f.root); err != nil {
		t.Fatal(err)
	}
	if response := call(readURL, managedBrand); response.Code != 403 {
		t.Fatalf("super-admin identity alone granted attribution access: status=%d body=%s", response.Code, response.Body.String())
	}
	if _, err := f.pool.Exec(ctx, `UPDATE admin_accounts SET is_super_admin=false WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	grantReportPermission(t, f, "report_attribution.view.platform")
	grantReportPermission(t, f, "report_attribution.export.platform")
	if response := call(readURL, managedBrand); response.Code != 200 {
		t.Fatalf("explicit platform view permission failed: status=%d body=%s", response.Code, response.Body.String())
	}
	if response := call(exportURL, managedBrand); response.Code != 200 {
		t.Fatalf("explicit platform view+export permissions failed: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAttributionReportHTTPWithholdsUnauditedResults(t *testing.T) {
	f := managedFixture(t)
	grantReportPermission(t, f, "report_attribution.view.brand")
	grantReportPermission(t, f, "report_attribution.export.brand")
	from, to := reportWindow()
	query := url.Values{"from": {from}, "to": {to}, "group_by": {"day"}}
	if _, err := f.pool.Exec(context.Background(), `CREATE FUNCTION reject_attribution_report_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action IN ('report.attribution.view','report.attribution.export') THEN RAISE EXCEPTION 'test attribution audit unavailable'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_attribution_report_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_attribution_report_audit()`); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		reportURL("/api/v1/admin/reports/attribution", query),
		reportURL("/api/v1/admin/reports/attribution/export", query),
	} {
		response := attributionReportHTTPCall(f, path, f.token, managedBrand)
		if response.Code != 503 || strings.Contains(response.Body.String(), "\"brand_id\"") || strings.Contains(response.Header().Get("Content-Type"), "text/csv") || response.Header().Get("X-Report-Audit-ID") != "" {
			t.Fatalf("unaudited attribution report leaked: status=%d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
		}
	}
}

func TestAttributionReportSessionExpiryWhileAuditWaitsWithholdsData(t *testing.T) {
	for _, tc := range []struct{ name, path string }{
		{"json", "/api/v1/admin/reports/attribution"},
		{"csv", "/api/v1/admin/reports/attribution/export"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := managedFixture(t)
			grantReportPermission(t, f, "report_attribution.view.brand")
			grantReportPermission(t, f, "report_attribution.export.brand")
			const lockKey int64 = 77118101
			ctx := context.Background()
			unlock, err := f.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer unlock.Rollback(ctx)
			if _, err := unlock.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, lockKey); err != nil {
				t.Fatal(err)
			}
			if _, err := f.pool.Exec(ctx, `CREATE FUNCTION wait_attribution_report_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action IN ('report.attribution.view','report.attribution.export') THEN PERFORM pg_advisory_xact_lock(77118101); END IF; RETURN NEW; END $$; CREATE TRIGGER wait_attribution_report_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION wait_attribution_report_audit()`); err != nil {
				t.Fatal(err)
			}
			from, to := reportWindow()
			path := reportURL(tc.path, url.Values{"from": {from}, "to": {to}, "group_by": {"day"}})
			result := make(chan *httptest.ResponseRecorder, 1)
			go func() { result <- attributionReportHTTPCall(f, path, f.token, managedBrand) }()
			waited := false
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				if err := f.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks waiting JOIN pg_locks held USING(locktype,classid,objid,objsubid) WHERE waiting.locktype='advisory' AND NOT waiting.granted AND held.granted AND held.pid=$1 AND waiting.pid<>pg_backend_pid())`, unlock.Conn().PgConn().PID()).Scan(&waited); err != nil {
					t.Fatal(err)
				}
				if waited {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
			if !waited {
				t.Fatal("attribution report did not wait inside its audit insert")
			}
			if _, err := f.pool.Exec(ctx, `UPDATE sessions SET expires_at=clock_timestamp()-interval '1 second' WHERE admin_id=$1 AND revoked_at IS NULL`, f.root); err != nil {
				t.Fatal(err)
			}
			if err := unlock.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			var response *httptest.ResponseRecorder
			select {
			case response = <-result:
			case <-time.After(3 * time.Second):
				t.Fatal("attribution report remained blocked after audit lock release")
			}
			if response.Code != 401 || strings.Contains(response.Body.String(), "\"brand_id\"") || strings.Contains(response.Header().Get("Content-Type"), "text/csv") || response.Header().Get("X-Report-Audit-ID") != "" || response.Header().Get("Content-Disposition") != "" {
				t.Fatalf("expired session received attribution data: status=%d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
			}
			var audits int
			if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE actor_id=$1 AND action IN('report.attribution.view','report.attribution.export')`, f.root).Scan(&audits); err != nil || audits != 0 {
				t.Fatalf("expired session audit transaction was not rolled back: count=%d err=%v", audits, err)
			}
		})
	}
}

func TestAttributionReportQueryStrictParsingAndEchoDefaults(t *testing.T) {
	from := url.QueryEscape("2026-10-01T08:00:00.123456789+08:00")
	to := url.QueryEscape("2026-10-02T08:00:00.987654321+08:00")
	r := httptest.NewRequest("GET", "/api/v1/admin/reports/attribution?from="+from+"&to="+to+"&group_by=agent", nil)
	q, err := attributionReportQuery(r, false)
	if err != nil {
		t.Fatal(err)
	}
	if q.Limit != 20 || q.Offset != 0 || q.AgentScope != "direct" || q.GameID != nil || q.MemberID != nil || q.AgentID != nil || q.JoinMethod != nil {
		t.Fatalf("wrong defaults or nullable query echo: %+v", q)
	}
	if q.From.Location() != time.UTC || q.To.Location() != time.UTC || q.From.Nanosecond() != 123456789 || q.To.Nanosecond() != 987654321 {
		t.Fatalf("timestamps were not normalized losslessly to UTC: from=%s to=%s", q.From, q.To)
	}

	full := "/api/v1/admin/reports/attribution?from=" + from + "&to=" + to + "&group_by=join_method&game_id=11111111-1111-4111-8111-111111111111&member_id=22222222-2222-4222-8222-222222222222&agent_id=33333333-3333-4333-8333-333333333333&agent_scope=downline&join_method=referral_code&limit=7&offset=9"
	q, err = attributionReportQuery(httptest.NewRequest("GET", full, nil), false)
	if err != nil {
		t.Fatal(err)
	}
	if q.AgentScope != "downline" || q.JoinMethod == nil || *q.JoinMethod != "referral_code" || q.Limit != 7 || q.Offset != 9 || q.GameID == nil || q.MemberID == nil || q.AgentID == nil {
		t.Fatalf("query fields were not retained: %+v", q)
	}
	if _, err = attributionReportQuery(httptest.NewRequest("GET", strings.Replace(full, "&agent_id=33333333-3333-4333-8333-333333333333", "", 1), nil), false); err == nil {
		t.Fatal("downline scope without agent_id was accepted")
	}
}

func TestAttributionReportQueryRejectsMalformedAndExportPaging(t *testing.T) {
	from, to := url.QueryEscape("2026-10-01T00:00:00Z"), url.QueryEscape("2026-10-02T00:00:00Z")
	base := "/api/v1/admin/reports/attribution?from=" + from + "&to=" + to + "&group_by=day"
	for _, suffix := range []string{
		"&unknown=1", "&from=" + from, "&member_id=", "&agent_scope=downline", "&join_method=other", "&join_method=Referral_Code",
		"&group_by=game&group_by=agent", "&limit=01", "&offset=1000001", "&limit=101",
	} {
		if _, err := attributionReportQuery(httptest.NewRequest("GET", base+suffix, nil), false); err == nil {
			t.Errorf("accepted invalid query suffix %q", suffix)
		}
	}
	for _, suffix := range []string{"&limit=20", "&offset=0"} {
		if _, err := attributionReportQuery(httptest.NewRequest("GET", base+suffix, nil), true); err == nil {
			t.Errorf("export accepted pagination suffix %q", suffix)
		}
	}
	for _, path := range []string{
		"/api/v1/admin/reports/attribution?from=not-a-date&to=" + to + "&group_by=day",
		"/api/v1/admin/reports/attribution?from=" + from + "&to=" + to,
		"/api/v1/admin/reports/attribution?from=" + to + "&to=" + from + "&group_by=day",
	} {
		if _, err := attributionReportQuery(httptest.NewRequest("GET", path, nil), false); err == nil {
			t.Errorf("accepted invalid query %q", path)
		}
	}
}

func TestAttributionReportRejectsGETBodyAsQueryError(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/admin/reports/attribution", strings.NewReader("{}"))
	if attributionReportGetHasNoBody(w, r) {
		t.Fatal("attribution report accepted a GET body")
	}
	if w.Code != 400 || !strings.Contains(w.Body.String(), "REPORT_QUERY_INVALID") {
		t.Fatalf("wrong GET body error: status=%d body=%s", w.Code, w.Body.String())
	}
}
