package httpapi

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

func withdrawalReportURL(export bool, member string) string {
	from, to := reportWindow()
	path := "/api/v1/admin/reports/withdrawal"
	if export {
		path += "/export"
	}
	values := url.Values{"from": {from}, "to": {to}, "group_by": {"state"}}
	if member != "" {
		values.Set("member_id", member)
	}
	return reportURL(path, values)
}

func TestWithdrawalReportProjectionAuthorizationAndCSV(t *testing.T) {
	f := pointsFixture(t)
	configureWithdrawalHTTP(t, f)
	f.http = withdrawalHTTP(t, f, withdrawalHTTPChecker{})
	order := withdrawalHTTPCreate(t, f, "withdrawal-report-application")
	mustStatus(t, f.call("POST", "/api/v1/admin/withdrawals/"+order.ID+"/approve", "withdrawal-report-approve", f.token, managedBrand, map[string]any{"version": 1, "reason": "synthetic report projection"}), 200)
	mustStatus(t, f.call("POST", "/api/v1/admin/withdrawals/"+order.ID+"/mark-paid", "withdrawal-report-paid", f.token, managedBrand, map[string]any{"version": 2, "reason": "synthetic report projection"}), 200)
	path := withdrawalReportURL(false, f.memberID)
	mustStatus(t, f.call("GET", path, "", f.token, managedBrand, nil), 403)
	grantReportPermission(t, f.managementHTTP, "report_withdrawal.view.brand")
	mustStatus(t, f.call("GET", path, "", f.token, managedBrand, nil), 200)
	var envelope struct {
		Data struct {
			BrandID  string            `json:"brand_id"`
			Timezone string            `json:"timezone"`
			Summary  map[string]string `json:"summary"`
			Items    []struct {
				Key    string            `json:"key"`
				Totals map[string]string `json:"totals"`
			} `json:"items"`
			Query map[string]any `json:"query"`
		} `json:"data"`
	}
	r := f.call("GET", path, "", f.token, managedBrand, nil)
	mustStatus(t, r, 200)
	if err := json.Unmarshal([]byte(r.Body.String()), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.BrandID != managedBrand || envelope.Data.Timezone == "" || envelope.Data.Summary["order_count"] != "1" || envelope.Data.Summary["requested_points"] != "50" || envelope.Data.Summary["paid_count"] != "1" || envelope.Data.Summary["paid_points"] != "50" || len(envelope.Data.Items) != 1 || envelope.Data.Items[0].Key != "paid" || envelope.Data.Query["game_id"] != nil || order.State != "reviewing" {
		t.Fatalf("unexpected report projection: %+v", envelope.Data)
	}
	grantReportPermission(t, f.managementHTTP, "report_withdrawal.export.brand")
	export := f.call("GET", withdrawalReportURL(true, f.memberID), "", f.token, managedBrand, nil)
	checkExportBody(t, export, managedBrand, "withdrawal")
	if strings.Contains(export.Body.String(), "game_id") || !strings.Contains(export.Body.String(), "reviewing_points") {
		t.Fatal("wrong withdrawal CSV columns")
	}
	for _, suffix := range []string{"&offset=0", "&limit=20", "&game_id=0199a000-0000-7000-8000-000000000003", "&unknown=1"} {
		mustStatus(t, f.call("GET", withdrawalReportURL(true, f.memberID)+suffix, "", f.token, managedBrand, nil), 400)
	}
	foreign := "0199a000-0000-7000-8000-000000000005"
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO brands(id,code,name,status) VALUES($1,'withdrawal_report_foreign','Foreign report brand','active') ON CONFLICT DO NOTHING`, pointsBrandB); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO global_users(id,username) VALUES('0199a000-0000-7000-8000-000000000004','withdrawal_report_foreign') ON CONFLICT DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version) VALUES($1,$2,'0199a000-0000-7000-8000-000000000004','operator','test-1','test-1') ON CONFLICT DO NOTHING`, foreign, pointsBrandB); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("GET", withdrawalReportURL(false, foreign), "", f.token, managedBrand, nil), 404)
	missingExport := f.call("GET", withdrawalReportURL(true, foreign), "", f.token, managedBrand, nil)
	mustStatus(t, missingExport, 404)
	if strings.Contains(missingExport.Header().Get("Content-Type"), "text/csv") || strings.Contains(missingExport.Body.String(), "\xef\xbb\xbf") {
		t.Fatal("missing member export emitted a partial CSV", missingExport.Header(), missingExport.Body.String())
	}
	var exportAudits int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_logs WHERE actor_id=$1 AND action='report.withdrawal.export'`, f.root).Scan(&exportAudits); err != nil || exportAudits != 2 {
		t.Fatalf("successful and missing-scope exports must both be audited: count=%d err=%v", exportAudits, err)
	}
}
