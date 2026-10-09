package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/reporting"
)

const (
	commissionAnalysisPath    = "/api/v1/admin/reports/commission-analysis"
	commissionAnalysisCSVPath = commissionAnalysisPath + ".csv"
)

func commissionAnalysisHTTPCall(f managementHTTP, path, body string, chunked bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, "http://localhost"+path, strings.NewReader(body))
	r.RemoteAddr = "192.0.2.55:12345"
	r.Header.Set("X-Brand-ID", managedBrand)
	r.Header.Set("Authorization", "Bearer "+f.token)
	if chunked {
		r.ContentLength = -1
		r.TransferEncoding = []string{"chunked"}
	}
	w := httptest.NewRecorder()
	f.http.ServeHTTP(w, r)
	return w
}

func commissionAnalysisQueryURL(path string) string {
	from := time.Date(2098, time.January, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(90 * 24 * time.Hour)
	return reportURL(path, url.Values{
		"from":     {from.Format(time.RFC3339Nano)},
		"to":       {to.Format(time.RFC3339Nano)},
		"group_by": {"cycle"},
	})
}

func grantCommissionAnalysisPermissions(t *testing.T, f managementHTTP, permissions ...string) {
	t.Helper()
	for _, permission := range permissions {
		grantReportPermission(t, f, permission)
	}
}

func clearCommissionAnalysisPermissions(t *testing.T, f managementHTTP) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), `DELETE FROM role_permissions WHERE permission_key IN (
		'commission.view.brand','commission.view.platform',
		'report_commission.view.brand','report_commission.view.platform',
		'report_commission.export.brand','report_commission.export.platform'
	) AND role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1)`, f.root); err != nil {
		t.Fatal(err)
	}
}

func TestCommissionAnalysisQueryStrictBoundsAndCanonicalIDs(t *testing.T) {
	valid := "from=2026-01-01T00%3A00%3A00Z&to=2026-04-01T00%3A00%3A00Z&group_by=cycle"
	for _, test := range []struct {
		name      string
		query     string
		exporting bool
		wantErr   bool
	}{
		{name: "cycle grouping", query: valid},
		{name: "agent grouping and canonical optional ids", query: valid[:len(valid)-5] + "agent&agent_id=aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa&member_id=bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb&cycle_id=cccccccc-cccc-4ccc-8ccc-cccccccccccc"},
		{name: "missing required", query: "from=2026-01-01T00%3A00%3A00Z&to=2026-01-02T00%3A00%3A00Z", wantErr: true},
		{name: "unsupported grouping", query: strings.Replace(valid, "cycle", "day", 1), wantErr: true},
		{name: "duplicate", query: valid + "&group_by=agent", wantErr: true},
		{name: "empty", query: valid + "&agent_id=", wantErr: true},
		{name: "unknown", query: valid + "&posting_time=created_at", wantErr: true},
		{name: "uppercase uuid is noncanonical", query: valid + "&agent_id=AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA", wantErr: true},
		{name: "page size above bound", query: valid + "&limit=101", wantErr: true},
		{name: "offset above bound", query: valid + "&offset=1000001", wantErr: true},
		{name: "zero limit", query: valid + "&limit=0", wantErr: true},
		{name: "interval above 93 days", query: "from=2026-01-01T00%3A00%3A00Z&to=2026-04-05T00%3A00%3A00Z&group_by=cycle", wantErr: true},
		{name: "CSV pagination", query: valid + "&limit=20", exporting: true, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/?"+test.query, nil)
			_, err := commissionAnalysisQuery(r, test.exporting)
			if (err != nil) != test.wantErr {
				t.Fatalf("commissionAnalysisQuery() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestCommissionAnalysisGetRejectsBodies(t *testing.T) {
	for _, test := range []struct {
		name       string
		body       string
		chunked    bool
		wantStatus int
	}{
		{name: "empty", wantStatus: http.StatusOK},
		{name: "body", body: "{}", wantStatus: http.StatusBadRequest},
		{name: "chunked body", body: "{}", chunked: true, wantStatus: http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", strings.NewReader(test.body))
			if test.chunked {
				r.ContentLength = -1
				r.TransferEncoding = []string{"chunked"}
			}
			w := httptest.NewRecorder()
			got := commissionAnalysisGetHasNoBody(w, r)
			if test.wantStatus == http.StatusOK {
				if !got {
					t.Fatalf("empty request rejected: %s", w.Body.String())
				}
			} else if got || w.Code != test.wantStatus {
				t.Fatalf("body request accepted or wrong status: accepted=%v status=%d", got, w.Code)
			}
		})
	}
}

func TestCommissionAnalysisHTTPEmptyCohortReturnsExactZeroAndNullableTotals(t *testing.T) {
	f := managedFixture(t)
	grantCommissionAnalysisPermissions(t, f,
		"commission.view.brand", "report_commission.view.brand", "report_commission.export.brand")
	query := commissionAnalysisQueryURL(commissionAnalysisPath)
	out := commissionAnalysisHTTPCall(f, query, "", false)
	mustStatus(t, out, http.StatusOK)
	var envelope struct {
		Data reporting.CommissionAnalysisReport `json:"data"`
	}
	if err := json.Unmarshal(out.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	r := envelope.Data
	if r.BrandID != managedBrand || r.Coverage.SelectedCycleCount != "0" || r.Coverage.ReadyCycleCount != "0" ||
		r.Coverage.UnreadyCycleCount != "0" ||
		r.TotalGroups != "0" || r.Items == nil || len(r.Items) != 0 {
		t.Fatalf("expected an empty valid brand cohort: %+v", r)
	}
	if r.Summary.ObservedCalculatedPoints != "0" || r.Summary.PaidEntryCount != "0" || r.Summary.PaidPoints != "0" ||
		r.Summary.AdjustmentEntryCount != "0" || r.Summary.AdjustmentCreditPoints != "0" || r.Summary.AdjustmentDebitPoints != "0" ||
		r.Summary.CorrectionEntryCount != "0" || r.Summary.CorrectionCreditPoints != "0" || r.Summary.CorrectionDebitPoints != "0" ||
		r.Summary.PostingEntryCount != "0" || r.Summary.ActualNetPoints != "0" || r.Summary.ManualAdjustmentNetPoints != "0" {
		t.Fatalf("empty cohort contained nonzero totals: %+v", r.Summary)
	}
	if r.Summary.CalculationComplete != (r.Summary.CalculatedPoints != nil) ||
		r.Summary.EffectiveTargetComplete != (r.Summary.EffectiveTargetPoints != nil) ||
		r.Summary.EffectiveTargetComplete && !r.Summary.CalculationComplete ||
		r.Summary.CalculationComplete && (r.Summary.CalculationMinusActualPoints == nil || *r.Summary.CalculationMinusActualPoints != "0") ||
		r.Summary.EffectiveTargetComplete && (r.Summary.EffectiveMinusActualPoints == nil || *r.Summary.EffectiveMinusActualPoints != "0") {
		t.Fatalf("nullable zero-cohort amounts and completeness flags disagree: %+v", r.Summary)
	}
}

func TestCommissionAnalysisHTTPRequiresBothExplicitPermissionFamilies(t *testing.T) {
	f := managedFixture(t)
	path := commissionAnalysisQueryURL(commissionAnalysisPath)
	if out := commissionAnalysisHTTPCall(f, path, "", false); out.Code != http.StatusForbidden {
		t.Fatalf("no analysis rights status=%d body=%s", out.Code, out.Body.String())
	}
	grantCommissionAnalysisPermissions(t, f, "report_commission.view.brand")
	if out := commissionAnalysisHTTPCall(f, path, "", false); out.Code != http.StatusForbidden {
		t.Fatalf("report-only rights satisfied commission.view: status=%d body=%s", out.Code, out.Body.String())
	}
	clearCommissionAnalysisPermissions(t, f)
	grantCommissionAnalysisPermissions(t, f, "commission.view.brand")
	if out := commissionAnalysisHTTPCall(f, path, "", false); out.Code != http.StatusForbidden {
		t.Fatalf("commission-only rights satisfied report_commission.view: status=%d body=%s", out.Code, out.Body.String())
	}
	grantCommissionAnalysisPermissions(t, f, "report_commission.view.brand")
	if out := commissionAnalysisHTTPCall(f, path, "", false); out.Code != http.StatusOK {
		t.Fatalf("combined brand rights failed: status=%d body=%s", out.Code, out.Body.String())
	}

	clearCommissionAnalysisPermissions(t, f)
	if _, err := f.pool.Exec(context.Background(), `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	if out := commissionAnalysisHTTPCall(f, path, "", false); out.Code != http.StatusForbidden {
		t.Fatalf("super-admin identity alone granted analysis: status=%d body=%s", out.Code, out.Body.String())
	}
	if _, err := f.pool.Exec(context.Background(), `UPDATE admin_accounts SET is_super_admin=false WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	grantCommissionAnalysisPermissions(t, f, "commission.view.platform", "report_commission.view.platform")
	if out := commissionAnalysisHTTPCall(f, path, "", false); out.Code != http.StatusOK {
		t.Fatalf("explicit combined platform grants failed: status=%d body=%s", out.Code, out.Body.String())
	}
	if out := commissionAnalysisHTTPCall(f, commissionAnalysisQueryURL(commissionAnalysisCSVPath), "", false); out.Code != http.StatusForbidden {
		t.Fatalf("platform view grants implied export: status=%d body=%s", out.Code, out.Body.String())
	}
}

func TestCommissionAnalysisHTTPStrictFiltersAndGETBodies(t *testing.T) {
	f := managedFixture(t)
	base := commissionAnalysisQueryURL(commissionAnalysisPath)
	for _, suffix := range []string{
		"&unknown=1", "&group_by=cycle&group_by=agent", "&agent_id=", "&member_id=not-a-uuid",
		"&cycle_id=AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA", "&limit=01", "&limit=101", "&offset=1000001",
	} {
		for _, path := range []string{commissionAnalysisPath, commissionAnalysisCSVPath} {
			out := commissionAnalysisHTTPCall(f, commissionAnalysisQueryURL(path)+suffix, "", false)
			if out.Code != http.StatusBadRequest || strings.Contains(out.Header().Get("Content-Type"), "text/csv") {
				t.Errorf("invalid query accepted or emitted CSV: path=%s suffix=%q status=%d body=%s", path, suffix, out.Code, out.Body.String())
			}
		}
	}
	for _, path := range []string{base, commissionAnalysisQueryURL(commissionAnalysisCSVPath)} {
		for _, chunked := range []bool{false, true} {
			out := commissionAnalysisHTTPCall(f, path, `{}`, chunked)
			if out.Code != http.StatusBadRequest || strings.Contains(out.Header().Get("Content-Type"), "text/csv") {
				t.Errorf("GET body accepted or emitted CSV: chunked=%t status=%d body=%s", chunked, out.Code, out.Body.String())
			}
		}
	}
	for _, suffix := range []string{"&limit=20", "&offset=0"} {
		out := commissionAnalysisHTTPCall(f, commissionAnalysisQueryURL(commissionAnalysisCSVPath)+suffix, "", false)
		if out.Code != http.StatusBadRequest {
			t.Errorf("CSV accepted pagination %q: status=%d body=%s", suffix, out.Code, out.Body.String())
		}
	}
}

func TestCommissionAnalysisHTTPAuditFailureWithholdsJSONAndCSV(t *testing.T) {
	f := managedFixture(t)
	grantCommissionAnalysisPermissions(t, f,
		"commission.view.brand", "report_commission.view.brand", "report_commission.export.brand")
	if _, err := f.pool.Exec(context.Background(), `CREATE FUNCTION reject_commission_analysis_audit() RETURNS trigger LANGUAGE plpgsql AS $$
	BEGIN
	 IF NEW.action IN ('report.commission_analysis.view','report.commission_analysis.export') THEN RAISE EXCEPTION 'injected commission analysis audit failure'; END IF;
	 RETURN NEW;
	END $$;
	CREATE TRIGGER reject_commission_analysis_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_commission_analysis_audit()`); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{commissionAnalysisPath, commissionAnalysisCSVPath} {
		out := commissionAnalysisHTTPCall(f, commissionAnalysisQueryURL(path), "", false)
		if out.Code != http.StatusServiceUnavailable || strings.Contains(out.Body.String(), `"brand_id"`) ||
			strings.Contains(out.Body.String(), `"summary"`) || strings.Contains(out.Header().Get("Content-Type"), "text/csv") ||
			out.Header().Get("X-Report-Audit-ID") != "" || out.Header().Get("Content-Disposition") != "" {
			t.Fatalf("audit failure leaked report data: status=%d headers=%v body=%s", out.Code, out.Header(), out.Body.String())
		}
	}
	var count int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_logs WHERE actor_id=$1 AND action IN ('report.commission_analysis.view','report.commission_analysis.export')`, f.root).Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed audit was committed: count=%d err=%v", count, err)
	}
}

func TestCommissionAnalysisHTTPLiveAuthorizationRevocationDuringQueryWithholdsData(t *testing.T) {
	for _, tc := range []struct {
		name, path, revoke string
		wantStatus         int
	}{
		{"json session expiry", commissionAnalysisPath, "session", http.StatusUnauthorized},
		{"csv session expiry", commissionAnalysisCSVPath, "session", http.StatusUnauthorized},
		{"json view grant revocation", commissionAnalysisPath, "permission", http.StatusForbidden},
		{"csv view grant revocation", commissionAnalysisCSVPath, "permission", http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := managedFixture(t)
			grantCommissionAnalysisPermissions(t, f,
				"commission.view.brand", "report_commission.view.brand", "report_commission.export.brand")
			ctx := context.Background()
			unlock, err := f.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer unlock.Rollback(ctx)
			// testdb owns this isolated schema; this real relation lock pauses the
			// report SELECT before it can finish, without fabricating report rows.
			if _, err := unlock.Exec(ctx, `LOCK TABLE commission_cycles IN ACCESS EXCLUSIVE MODE`); err != nil {
				t.Fatal(err)
			}
			path := commissionAnalysisQueryURL(tc.path)
			result := make(chan *httptest.ResponseRecorder, 1)
			go func() { result <- commissionAnalysisHTTPCall(f, path, "", false) }()
			waiting := false
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				if err := f.pool.QueryRow(ctx, `SELECT EXISTS(
				 SELECT 1 FROM pg_locks waiting JOIN pg_locks held
				   ON waiting.locktype=held.locktype
				  AND waiting.database IS NOT DISTINCT FROM held.database
				  AND waiting.relation IS NOT DISTINCT FROM held.relation
				 WHERE waiting.locktype='relation' AND waiting.relation='commission_cycles'::regclass
				   AND waiting.mode='AccessShareLock' AND NOT waiting.granted
				   AND held.granted AND held.pid=$1
				)`, unlock.Conn().PgConn().PID()).Scan(&waiting); err != nil {
					t.Fatal(err)
				}
				if waiting {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
			if !waiting {
				t.Fatal("commission analysis request did not enter and block its report query")
			}
			switch tc.revoke {
			case "session":
				if _, err := f.pool.Exec(ctx, `UPDATE sessions SET expires_at=clock_timestamp()-interval '1 second' WHERE admin_id=$1 AND revoked_at IS NULL`, f.root); err != nil {
					t.Fatal(err)
				}
			case "permission":
				if _, err := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE permission_key='report_commission.view.brand' AND role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1)`, f.root); err != nil {
					t.Fatal(err)
				}
			}
			if err := unlock.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			var out *httptest.ResponseRecorder
			select {
			case out = <-result:
			case <-time.After(3 * time.Second):
				t.Fatal("commission analysis remained blocked after releasing the report table")
			}
			if out.Code != tc.wantStatus || strings.Contains(out.Body.String(), `"brand_id"`) ||
				strings.Contains(out.Body.String(), `"summary"`) || strings.Contains(out.Header().Get("Content-Type"), "text/csv") ||
				out.Header().Get("X-Report-Audit-ID") != "" || out.Header().Get("Content-Disposition") != "" {
				t.Fatalf("revoked authorization received report data: status=%d headers=%v body=%s", out.Code, out.Header(), out.Body.String())
			}
			var audits int
			if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE actor_id=$1 AND action IN ('report.commission_analysis.view','report.commission_analysis.export')`, f.root).Scan(&audits); err != nil || audits != 0 {
				t.Fatalf("revoked query committed report audit: count=%d err=%v", audits, err)
			}
		})
	}
}

func TestCommissionAnalysisHTTPRevocationDuringAuditRollsBackReportAudit(t *testing.T) {
	f := managedFixture(t)
	grantCommissionAnalysisPermissions(t, f, "commission.view.brand", "report_commission.view.brand")
	const auditLock int64 = 77118432
	ctx := context.Background()
	unlock, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock.Rollback(ctx)
	if _, err := unlock.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, auditLock); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `CREATE FUNCTION wait_commission_analysis_audit() RETURNS trigger LANGUAGE plpgsql AS $$
	BEGIN
	 IF NEW.action='report.commission_analysis.view' THEN PERFORM pg_advisory_xact_lock(77118432); END IF;
	 RETURN NEW;
	END $$;
	CREATE TRIGGER wait_commission_analysis_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION wait_commission_analysis_audit()`); err != nil {
		t.Fatal(err)
	}
	result := make(chan *httptest.ResponseRecorder, 1)
	path := commissionAnalysisQueryURL(commissionAnalysisPath)
	go func() { result <- commissionAnalysisHTTPCall(f, path, "", false) }()
	waiting := false
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err := f.pool.QueryRow(ctx, `SELECT EXISTS(
		 SELECT 1 FROM pg_locks waiting JOIN pg_locks held USING(locktype,classid,objid,objsubid)
		 WHERE waiting.locktype='advisory' AND NOT waiting.granted AND held.granted AND held.pid=$1
		)`, unlock.Conn().PgConn().PID()).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !waiting {
		t.Fatal("commission analysis did not wait in its audit insert")
	}
	if _, err := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE permission_key='report_commission.view.brand' AND role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1)`, f.root); err != nil {
		t.Fatal(err)
	}
	if err := unlock.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var out *httptest.ResponseRecorder
	select {
	case out = <-result:
	case <-time.After(3 * time.Second):
		t.Fatal("commission analysis remained blocked after releasing its audit lock")
	}
	if out.Code != http.StatusForbidden || strings.Contains(out.Body.String(), `"brand_id"`) ||
		strings.Contains(out.Body.String(), `"summary"`) || out.Header().Get("X-Report-Audit-ID") != "" {
		t.Fatalf("revoked permission received report data: status=%d headers=%v body=%s", out.Code, out.Header(), out.Body.String())
	}
	var reportAudits, deniedAudits int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE action='report.commission_analysis.view'),
	 count(*) FILTER(WHERE action='access.denied' AND after_json->>'permission'='report_commission.view')
	 FROM audit_logs WHERE actor_id=$1`, f.root).Scan(&reportAudits, &deniedAudits); err != nil || reportAudits != 0 || deniedAudits != 1 {
		t.Fatalf("late permission revocation audit state: report=%d denied=%d err=%v", reportAudits, deniedAudits, err)
	}
}

func TestCommissionAnalysisFailureMapsIntegrityToConflict(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, commissionAnalysisPath, nil)
	commissionAnalysisFailure(w, r, reporting.ErrAnalysisIntegrity)
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusConflict || envelope.Error.Code != "COMMISSION_ANALYSIS_INTEGRITY" ||
		strings.Contains(w.Body.String(), `"summary"`) || strings.Contains(w.Body.String(), `"items"`) {
		t.Fatalf("integrity error did not map to fail-closed conflict: status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestCommissionAnalysisHTTPUnattributableLedgerFailsClosedAndAudits(t *testing.T) {
	f, order := settlementHTTPFixture(t)
	grantCommissionAnalysisPermissions(t, f.managementHTTP, "commission.view.brand", "report_commission.view.brand", "report_commission.export.brand")
	ctx := context.Background()
	var ledger string
	if err := f.pool.QueryRow(ctx, `SELECT debit_entry_id::text FROM bet_orders WHERE brand_id=$1 AND id=$2`, managedBrand, order.ID).Scan(&ledger); err != nil {
		t.Fatal(err)
	}
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	// This is a real funded bet, not invented commission success. Inject only
	// an unresolvable commission reference into its owned immutable ledger.
	if _, err = tx.Exec(ctx, `ALTER TABLE point_ledger_entries DISABLE TRIGGER ledger_immutable`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE point_ledger_entries SET reference_type='commission_adjustment' WHERE id=$1`, ledger); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `ALTER TABLE point_ledger_entries ENABLE TRIGGER ledger_immutable`); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	const moneySQL = `SELECT md5(jsonb_build_object('accounts',(SELECT jsonb_agg(to_jsonb(a) ORDER BY id) FROM point_accounts a),'buckets',(SELECT jsonb_agg(to_jsonb(b) ORDER BY account_id,source,state) FROM point_buckets b),'ledger',(SELECT jsonb_agg(to_jsonb(l) ORDER BY id) FROM point_ledger_entries l))::text)`
	var before, after string
	if err = f.pool.QueryRow(ctx, moneySQL).Scan(&before); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{commissionAnalysisPath, commissionAnalysisCSVPath} {
		out := commissionAnalysisHTTPCall(f.managementHTTP, commissionAnalysisQueryURL(path), "", false)
		if out.Code != http.StatusConflict || !strings.Contains(out.Body.String(), "COMMISSION_ANALYSIS_INTEGRITY") || strings.Contains(out.Body.String(), `"summary"`) || out.Header().Get("X-Report-Audit-ID") != "" || out.Header().Get("Content-Disposition") != "" {
			t.Fatalf("unattributable money was hidden as an empty report: status=%d headers=%v body=%s", out.Code, out.Header(), out.Body.String())
		}
	}
	var audits int
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE actor_id=$1 AND action IN('report.commission_analysis.view','report.commission_analysis.export') AND after_json->>'outcome'='integrity_failed'`, f.root).Scan(&audits); err != nil || audits != 2 {
		t.Fatalf("source failures were not audited: count=%d err=%v", audits, err)
	}
	if err = f.pool.QueryRow(ctx, moneySQL).Scan(&after); err != nil || after != before {
		t.Fatalf("failed report read moved financial data: err=%v", err)
	}
}
