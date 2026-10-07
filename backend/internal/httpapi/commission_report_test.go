package httpapi

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

const commissionReportPath = "/api/v1/admin/reports/commission"
const commissionReportCSVPath = "/api/v1/admin/reports/commission.csv"

func commissionReportURL(path string, from, to string) string {
	return reportURL(path, url.Values{"from": {from}, "to": {to}, "group_by": {"day"}})
}

func TestCommissionReportHTTPAuthStrictQueryAndSummaryOnlyCSV(t *testing.T) {
	f := managedFixture(t)
	from, to := reportWindow()
	jsonURL := commissionReportURL(commissionReportPath, from, to)
	csvURL := commissionReportURL(commissionReportCSVPath, from, to)
	mustStatus(t, f.call("GET", jsonURL, "", f.token, managedBrand, nil), 403)
	grantReportPermission(t, f, "report_commission.view.brand")
	mustStatus(t, f.call("GET", jsonURL, "", f.token, managedBrand, nil), 200)
	mustStatus(t, f.call("GET", csvURL, "", f.token, managedBrand, nil), 403)
	grantReportPermission(t, f, "report_commission.export.brand")

	response := f.call("GET", jsonURL, "", f.token, managedBrand, nil)
	mustStatus(t, response, 200)
	var envelope struct {
		Data struct {
			BrandID string            `json:"brand_id"`
			Summary map[string]string `json:"summary"`
			Items   []json.RawMessage `json:"items"`
			Query   struct {
				Limit   int     `json:"limit"`
				Offset  int     `json:"offset"`
				AgentID *string `json:"agent_id"`
			} `json:"query"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.BrandID != managedBrand || envelope.Data.Summary["entry_count"] != "0" || envelope.Data.Summary["net_points"] != "0" || len(envelope.Data.Items) != 0 || envelope.Data.Query.Limit != 20 || envelope.Data.Query.Offset != 0 || envelope.Data.Query.AgentID != nil {
		t.Fatalf("unexpected empty commission report: %+v", envelope.Data)
	}

	export := f.call("GET", csvURL, "", f.token, managedBrand, nil)
	checkExportBody(t, export, managedBrand, "commission")
	if export.Header().Get("X-Report-Byte-Count") != export.Header().Get("Content-Length") || export.Header().Get("X-Report-Timezone") == "" {
		t.Fatal("missing complete export metadata", export.Header())
	}
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(export.Body.String(), "\xef\xbb\xbf"))).ReadAll()
	if err != nil || len(rows) != 2 || rows[1][0] != "summary" || rows[1][12] != "0" || rows[1][18] != "0" {
		t.Fatalf("empty export should contain header and summary only: rows=%d err=%v", len(rows), err)
	}
	if got := strings.Join(rows[0], ","); got != "record_type,brand_id,snapshot_at,timezone,from,to,group_by,agent_id,member_id,cycle_id,key,label,entry_count,paid_entry_count,paid_points,adjustment_entry_count,adjustment_credit_points,adjustment_debit_points,net_points" {
		t.Fatalf("CSV header drifted: %s", got)
	}
	for _, suffix := range []string{"&limit=20", "&offset=0"} {
		mustStatus(t, f.call("GET", commissionReportURL(commissionReportPath, from, to)+suffix, "", f.token, managedBrand, nil), 200)
		mustStatus(t, f.call("GET", commissionReportURL(commissionReportCSVPath, from, to)+suffix, "", f.token, managedBrand, nil), 400)
	}
	for _, suffix := range []string{"&unknown=1", "&agent_id=", "&group_by=day&group_by=cycle", "&member_id=bad", "&limit=+20", "&offset=001"} {
		for _, path := range []string{commissionReportPath, commissionReportCSVPath} {
			out := f.call("GET", commissionReportURL(path, from, to)+suffix, "", f.token, managedBrand, nil)
			mustStatus(t, out, 400)
			if strings.Contains(out.Header().Get("Content-Type"), "text/csv") {
				t.Fatal("invalid report query emitted CSV", suffix)
			}
		}
	}
	var exportAudits int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_logs WHERE actor_id=$1 AND action='report.commission.export'`, f.root).Scan(&exportAudits); err != nil || exportAudits != 1 {
		t.Fatalf("successful CSV must be audited exactly once: count=%d err=%v", exportAudits, err)
	}
	if _, err := f.pool.Exec(context.Background(), `DELETE FROM role_permissions WHERE permission_key IN ('report_commission.view.brand','report_commission.export.brand') AND role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1)`, f.root); err != nil {
		t.Fatal(err)
	}
	denied := f.call("GET", jsonURL, "", f.token, managedBrand, nil)
	mustStatus(t, denied, 403)
	var denials int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_logs WHERE actor_id=$1 AND action='access.denied' AND after_json->>'attempted_brand'=$2 AND after_json->>'permission'='report_commission.view'`, f.root, managedBrand).Scan(&denials); err != nil || denials < 1 {
		t.Fatalf("revoked read permission denial was not audited: count=%d err=%v", denials, err)
	}
	if _, err := f.pool.Exec(context.Background(), `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("GET", jsonURL, "", f.token, managedBrand, nil), 403)
	if _, err := f.pool.Exec(context.Background(), `UPDATE admin_accounts SET is_super_admin=false WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
}

func TestCommissionReportExplicitPlatformPermissionAndDisabledBrand(t *testing.T) {
	f := managedFixture(t)
	from, to := reportWindow()
	grantReportPermission(t, f, "report_commission.view.platform")
	grantReportPermission(t, f, "report_commission.export.platform")
	if _, err := f.pool.Exec(context.Background(), `DELETE FROM role_permissions WHERE permission_key IN ('report_commission.view.brand','report_commission.export.brand') AND role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1)`, f.root); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(context.Background(), `UPDATE brands SET status='disabled' WHERE id=$1`, managedBrand); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("GET", commissionReportURL(commissionReportPath, from, to), "", f.token, managedBrand, nil), 200)
	mustStatus(t, f.call("GET", commissionReportURL(commissionReportCSVPath, from, to), "", f.token, managedBrand, nil), 200)
	if _, err := f.pool.Exec(context.Background(), `UPDATE admin_accounts SET status='disabled' WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("GET", commissionReportURL(commissionReportPath, from, to), "", f.token, managedBrand, nil), 401)
	if _, err := f.pool.Exec(context.Background(), `UPDATE admin_accounts SET status='active' WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
}

func TestCommissionReportAuditFailureWithholdsJSONAndCSV(t *testing.T) {
	f := managedFixture(t)
	grantReportPermission(t, f, "report_commission.view.brand")
	grantReportPermission(t, f, "report_commission.export.brand")
	from, to := reportWindow()
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `CREATE FUNCTION reject_commission_report_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action IN ('report.commission.view','report.commission.export') THEN RAISE EXCEPTION 'test audit unavailable'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_commission_report_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_commission_report_audit()`); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{commissionReportPath, commissionReportCSVPath} {
		out := f.call("GET", commissionReportURL(path, from, to), "", f.token, managedBrand, nil)
		mustStatus(t, out, 503)
		if strings.Contains(out.Body.String(), "entry_count") || strings.Contains(out.Body.String(), managedBrand) || strings.Contains(out.Header().Get("Content-Type"), "text/csv") || out.Header().Get("X-Report-Audit-ID") != "" {
			t.Fatal("unaudited report leaked", out.Header(), out.Body.String())
		}
	}
}
