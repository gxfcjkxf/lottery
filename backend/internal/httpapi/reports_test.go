package httpapi

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"
)

const (
	reportBettingBrand = "report_betting.view.brand"
	reportLedgerBrand  = "report_ledger.view.brand"
	reportBettingPlat  = "report_betting.view.platform"
	reportLedgerPlat   = "report_ledger.view.platform"
)

func grantReportPermission(t *testing.T, f managementHTTP, permission string) {
	t.Helper()
	ctx := context.Background()
	if strings.HasSuffix(permission, ".platform") {
		if _, err := f.pool.Exec(ctx, `INSERT INTO roles(id,brand_id,code,name) VALUES('0199a000-0000-7000-8000-000000000009',NULL,'report_platform_test','Report platform test') ON CONFLICT(id) DO NOTHING`); err != nil {
			t.Fatal(err)
		}
		if _, err := f.pool.Exec(ctx, `INSERT INTO admin_account_roles(account_id,role_id) VALUES($1,'0199a000-0000-7000-8000-000000000009') ON CONFLICT DO NOTHING`, f.root); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO permissions(key) VALUES($1) ON CONFLICT DO NOTHING`, permission); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) SELECT role_id,$2 FROM admin_account_roles WHERE account_id=$1 ON CONFLICT DO NOTHING`, f.root, permission); err != nil {
		t.Fatal(err)
	}
}

func reportURL(path string, values url.Values) string {
	return path + "?" + values.Encode()
}

func reportWindow() (string, string) {
	now := time.Now().UTC()
	return now.Add(-24 * time.Hour).Format(time.RFC3339Nano), now.Add(24 * time.Hour).Format(time.RFC3339Nano)
}

func reportData(t *testing.T, response interface{ BodyString() string }) map[string]any {
	t.Helper()
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal([]byte(response.BodyString()), &envelope); err != nil {
		t.Fatal(err)
	}
	return envelope.Data
}

func TestAdminReportsAuthorizationAndScope(t *testing.T) {
	f := managedFixture(t)
	from, to := reportWindow()
	betting := reportURL("/api/v1/admin/reports/betting", url.Values{"from": {from}, "to": {to}, "group_by": {"day"}})
	ledger := reportURL("/api/v1/admin/reports/ledger", url.Values{"from": {from}, "to": {to}, "group_by": {"day"}})

	if r := f.call("GET", betting, "", f.token, managedBrand, nil); r.Code != 403 {
		t.Fatalf("existing admin without report permission: status=%d body=%s", r.Code, r.Body.String())
	}
	if r := f.call("GET", ledger, "", "", managedBrand, nil); r.Code != 401 {
		t.Fatalf("anonymous report query: status=%d body=%s", r.Code, r.Body.String())
	}
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO brands(id,code,name,status) VALUES($1,'report_foreign','Report foreign brand','active') ON CONFLICT(id) DO NOTHING`, pointsBrandB); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO global_users(id,username) VALUES('0199a000-0000-7000-8000-000000000004','report_foreign_member') ON CONFLICT(id) DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version) VALUES('0199a000-0000-7000-8000-000000000005',$1,'0199a000-0000-7000-8000-000000000004','operator','test-1','test-1') ON CONFLICT(id) DO NOTHING`, pointsBrandB); err != nil {
		t.Fatal(err)
	}
	grantReportPermission(t, f, reportLedgerBrand)
	foreignFilter := url.Values{"from": {from}, "to": {to}, "group_by": {"day"}, "member_id": {"0199a000-0000-7000-8000-000000000005"}}
	if r := f.call("GET", reportURL("/api/v1/admin/reports/ledger", foreignFilter), "", f.token, managedBrand, nil); r.Code != 404 {
		t.Fatalf("foreign member under requested brand: status=%d body=%s", r.Code, r.Body.String())
	}
	var queryAuditsBefore int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_logs WHERE resource_type='query'`).Scan(&queryAuditsBefore); err != nil {
		t.Fatal(err)
	}
	if r := f.call("GET", ledger, "", f.token, managedBrand, nil); r.Code != 200 {
		t.Fatalf("ledger-only admin query: status=%d body=%s", r.Code, r.Body.String())
	}
	if r := f.call("GET", betting, "", f.token, managedBrand, nil); r.Code != 403 {
		t.Fatalf("ledger permission granted betting access: status=%d body=%s", r.Code, r.Body.String())
	}
	grantReportPermission(t, f, reportBettingBrand)
	if r := f.call("GET", betting, "", f.token, pointsBrandB, nil); r.Code != 403 {
		t.Fatalf("brand report permission crossed brand namespace: status=%d body=%s", r.Code, r.Body.String())
	}

	grantReportPermission(t, f, reportBettingPlat)
	if _, err := f.pool.Exec(context.Background(), `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	if r := f.call("GET", betting, "", f.token, managedBrand, nil); r.Code != 200 {
		t.Fatalf("platform report permission with brand scope: status=%d body=%s", r.Code, r.Body.String())
	}
	if r := f.call("GET", betting, "", f.token, pointsBrandB, nil); r.Code != 200 {
		t.Fatalf("platform report permission did not authorize another brand: status=%d body=%s", r.Code, r.Body.String())
	}
	if r := f.call("GET", ledger, "", f.token, pointsBrandB, nil); r.Code != 403 {
		t.Fatalf("betting platform permission granted ledger access: status=%d body=%s", r.Code, r.Body.String())
	}
	if _, err := f.pool.Exec(context.Background(), `DELETE FROM role_permissions WHERE role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key=$2`, f.root, reportLedgerBrand); err != nil {
		t.Fatal(err)
	}
	if r := f.call("GET", ledger, "", f.token, managedBrand, nil); r.Code != 403 {
		t.Fatalf("revoked report permission remained cached: status=%d body=%s", r.Code, r.Body.String())
	}
	grantReportPermission(t, f, reportLedgerPlat)
	if r := f.call("GET", ledger, "", f.token, pointsBrandB, nil); r.Code != 200 {
		t.Fatalf("ledger platform permission did not authorize another brand: status=%d body=%s", r.Code, r.Body.String())
	}
	var queryAuditsAfter int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_logs WHERE resource_type='query'`).Scan(&queryAuditsAfter); err != nil || queryAuditsAfter-queryAuditsBefore != 4 {
		t.Fatalf("successful report GETs wrote %d query audits, want 4; err=%v", queryAuditsAfter-queryAuditsBefore, err)
	}
}

func TestAdminLedgerReportPostingTimeBalancesAndPrivacy(t *testing.T) {
	f := pointsFixture(t)
	grantReportPermission(t, f.managementHTTP, reportLedgerBrand)

	order := pointRecharge(t, f, "125", "report fixture confidential reason", "report-ledger-recharge")
	confirmed := f.call("POST", "/api/v1/admin/recharges/"+order.ID+"/confirm", "report-ledger-confirm", f.token, managedBrand, map[string]any{"version": order.Version, "reason": "report confirmation secret"})
	if confirmed.Code != 200 {
		t.Fatalf("confirm recharge: status=%d body=%s", confirmed.Code, confirmed.Body.String())
	}
	frozen := f.call("POST", "/api/v1/admin/wallets/"+f.memberID+"/freeze", "report-ledger-freeze", f.token, managedBrand, map[string]string{"points": "20", "reason": "private freeze reason"})
	if frozen.Code != 200 {
		t.Fatalf("freeze points: status=%d body=%s", frozen.Code, frozen.Body.String())
	}

	var beforeEntries int
	var beforeAvailable, beforeFrozen, beforeWithdrawal int64
	var beforeBets, beforeKeys int
	if err := f.pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM point_ledger_entries WHERE brand_id=$1 AND member_id=$2), (SELECT COALESCE(sum(points) FILTER(WHERE state='available'),0) FROM point_buckets b JOIN point_accounts a ON a.id=b.account_id WHERE a.brand_id=$1 AND a.brand_member_id=$2), (SELECT COALESCE(sum(points) FILTER(WHERE state IN('manual_frozen','system_frozen')),0) FROM point_buckets b JOIN point_accounts a ON a.id=b.account_id WHERE a.brand_id=$1 AND a.brand_member_id=$2), (SELECT COALESCE(sum(points) FILTER(WHERE state='withdrawal'),0) FROM point_buckets b JOIN point_accounts a ON a.id=b.account_id WHERE a.brand_id=$1 AND a.brand_member_id=$2)`, managedBrand, f.memberID).Scan(&beforeEntries, &beforeAvailable, &beforeFrozen, &beforeWithdrawal); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM bet_orders WHERE brand_id=$1), (SELECT count(*) FROM idempotency_requests WHERE brand_id=$1)`, managedBrand).Scan(&beforeBets, &beforeKeys); err != nil {
		t.Fatal(err)
	}
	from, to := reportWindow()
	values := url.Values{"from": {from}, "to": {to}, "group_by": {"entry_type"}, "limit": {"20"}, "offset": {"0"}}
	r := f.call("GET", reportURL("/api/v1/admin/reports/ledger", values), "", f.token, managedBrand, nil)
	if r.Code != 200 {
		t.Fatalf("ledger report: status=%d body=%s", r.Code, r.Body.String())
	}
	data := reportData(t, stringBody{body: r.Body.String()})
	var summary, balances map[string]any
	var totalGroups string
	if err := decodeReportFields(data, &summary, &balances, &totalGroups); err != nil {
		t.Fatal(err)
	}
	if summary["entry_count"] != "2" || summary["net_points"] != "125" || summary["recharge_points"] != "125" {
		t.Fatalf("ledger posting totals mismatch: %#v", summary)
	}
	if balances["account_count"] != "1" || balances["available_points"] != "105" || balances["frozen_points"] != "20" || balances["withdrawal_points"] != "0" || balances["total_points"] != "125" {
		t.Fatalf("current balances mismatch: %#v", balances)
	}
	for _, secret := range []string{"Points member", "manual-proof-report-ledger-recharge", "offline deposit", "report fixture confidential reason", "report confirmation secret", "private freeze reason", "proof_reference", "source_allocation"} {
		if strings.Contains(r.Body.String(), secret) {
			t.Fatalf("report leaked private ledger payload %q: %s", secret, r.Body.String())
		}
	}

	futureFrom, futureTo := "2099-01-01T00:00:00Z", "2099-01-02T00:00:00Z"
	filtered := url.Values{"from": {futureFrom}, "to": {futureTo}, "group_by": {"day"}}
	r = f.call("GET", reportURL("/api/v1/admin/reports/ledger", filtered), "", f.token, managedBrand, nil)
	if r.Code != 200 {
		t.Fatalf("empty posting-time range: status=%d body=%s", r.Code, r.Body.String())
	}
	data = reportData(t, stringBody{body: r.Body.String()})
	if got := data["summary"].(map[string]any)["entry_count"]; got != "0" {
		t.Fatalf("future posting range included entries: %v", got)
	}
	if got := data["balances"].(map[string]any)["total_points"]; got != "125" {
		t.Fatalf("range filter changed current balances: %v", got)
	}
	var afterEntries int
	var afterAvailable, afterFrozen, afterWithdrawal int64
	var afterBets, afterKeys int
	if err := f.pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM point_ledger_entries WHERE brand_id=$1 AND member_id=$2), COALESCE(sum(points) FILTER(WHERE state='available'),0), COALESCE(sum(points) FILTER(WHERE state IN('manual_frozen','system_frozen')),0), COALESCE(sum(points) FILTER(WHERE state='withdrawal'),0) FROM point_buckets b JOIN point_accounts a ON a.id=b.account_id WHERE a.brand_id=$1 AND a.brand_member_id=$2`, managedBrand, f.memberID).Scan(&afterEntries, &afterAvailable, &afterFrozen, &afterWithdrawal); err != nil || afterEntries != beforeEntries || afterAvailable != beforeAvailable || afterFrozen != beforeFrozen || afterWithdrawal != beforeWithdrawal {
		t.Fatalf("report query changed money state: ledger %d->%d available %d->%d frozen %d->%d withdrawal %d->%d err=%v", beforeEntries, afterEntries, beforeAvailable, afterAvailable, beforeFrozen, afterFrozen, beforeWithdrawal, afterWithdrawal, err)
	}
	if err := f.pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM bet_orders WHERE brand_id=$1), (SELECT count(*) FROM idempotency_requests WHERE brand_id=$1)`, managedBrand).Scan(&afterBets, &afterKeys); err != nil || afterBets != beforeBets || afterKeys != beforeKeys {
		t.Fatalf("report queries changed bets/idempotency keys: bets %d->%d keys %d->%d err=%v", beforeBets, afterBets, beforeKeys, afterKeys, err)
	}
}

type stringBody struct{ body string }

func (b stringBody) BodyString() string { return b.body }

func decodeReportFields(data map[string]any, summary, balances *map[string]any, totalGroups *string) error {
	encoded, err := json.Marshal(data)
	if err != nil {
		return err
	}
	var decoded struct {
		Summary     map[string]any `json:"summary"`
		Balances    map[string]any `json:"balances"`
		TotalGroups string         `json:"total_groups"`
	}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return err
	}
	*summary, *balances, *totalGroups = decoded.Summary, decoded.Balances, decoded.TotalGroups
	return nil
}

func TestAdminReportsRejectInvalidQueriesAndAuditFailure(t *testing.T) {
	f := managedFixture(t)
	grantReportPermission(t, f, reportBettingBrand)
	grantReportPermission(t, f, reportLedgerBrand)
	from, to := reportWindow()
	cases := []struct {
		name, path, rawQuery string
	}{
		{"missing required dates", "/api/v1/admin/reports/betting", "group_by=day"},
		{"invalid timestamp", "/api/v1/admin/reports/betting", "from=nope&to=" + url.QueryEscape(to) + "&group_by=day"},
		{"reversed dates", "/api/v1/admin/reports/betting", "from=" + url.QueryEscape(to) + "&to=" + url.QueryEscape(from) + "&group_by=day"},
		{"too wide", "/api/v1/admin/reports/ledger", "from=2026-01-01T00%3A00%3A00Z&to=2026-04-10T00%3A00%3A00Z&group_by=day"},
		{"bad group", "/api/v1/admin/reports/betting", "from=" + url.QueryEscape(from) + "&to=" + url.QueryEscape(to) + "&group_by=entry_type"},
		{"unknown query", "/api/v1/admin/reports/ledger", "from=" + url.QueryEscape(from) + "&to=" + url.QueryEscape(to) + "&group_by=day&surprise=1"},
		{"duplicate query", "/api/v1/admin/reports/betting", "from=" + url.QueryEscape(from) + "&from=" + url.QueryEscape(from) + "&to=" + url.QueryEscape(to) + "&group_by=day"},
		{"invalid pagination", "/api/v1/admin/reports/ledger", "from=" + url.QueryEscape(from) + "&to=" + url.QueryEscape(to) + "&group_by=day&limit=101"},
		{"ledger game filter", "/api/v1/admin/reports/ledger", "from=" + url.QueryEscape(from) + "&to=" + url.QueryEscape(to) + "&group_by=day&game_id=0199a000-0000-7000-8000-000000000003"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := f.call("GET", tc.path+"?"+tc.rawQuery, "", f.token, managedBrand, nil)
			if r.Code != 400 {
				t.Fatalf("status=%d body=%s", r.Code, r.Body.String())
			}
		})
	}

	if _, err := f.pool.Exec(context.Background(), `CREATE FUNCTION reject_report_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.resource_type='query' AND NEW.action ILIKE '%report%' THEN RAISE EXCEPTION 'test report audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_report_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_report_audit()`); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = f.pool.Exec(context.Background(), `DROP TRIGGER IF EXISTS reject_report_audit ON audit_logs; DROP FUNCTION IF EXISTS reject_report_audit()`)
	}()
	r := f.call("GET", reportURL("/api/v1/admin/reports/betting", url.Values{"from": {from}, "to": {to}, "group_by": {"day"}}), "", f.token, managedBrand, nil)
	if r.Code != 503 {
		t.Fatalf("audit failure status=%d body=%s", r.Code, r.Body.String())
	}
	if strings.Contains(r.Body.String(), `"data"`) {
		t.Fatalf("audit failure returned partial report data: %s", r.Body.String())
	}
}
