package httpapi

import (
	"context"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/workbench"
)

func TestWorkbenchHTTPCommissionOnlyGrantAndAuthorizedSourceFailure(t *testing.T) {
	f := managedFixture(t)
	ctx := context.Background()
	grantReportPermission(t, f, "commission.view.brand")
	if _, err := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key<>'commission.view.brand'`, f.root); err != nil {
		t.Fatal(err)
	}
	response := f.call("GET", workbenchPath, "", f.token, managedBrand, nil)
	mustStatus(t, response, 200)
	var out workbench.Snapshot
	managedData(t, response, &out)
	if out.Commissions.Status != "ready" || out.Commissions.Data == nil || out.Commissions.Data.CycleReadyCount != "0" || out.Brand.Data != nil || out.Balances.Data != nil {
		t.Fatal("commission-only response leaks data or fails independent view", out)
	}
	if _, err := f.pool.Exec(ctx, `ALTER TABLE commission_payments RENAME TO workbench_unavailable_commission_payments`); err != nil {
		t.Fatal(err)
	}
	response = f.call("GET", workbenchPath, "", f.token, managedBrand, nil)
	mustStatus(t, response, 503)
	if strings.Contains(response.Body.String(), "cycle_ready_count") || strings.Contains(response.Body.String(), "workbench_unavailable") {
		t.Fatal("failed source released partial counters or internal detail")
	}
	var failures int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='workbench.read_failed' AND after_json->>'attempted_brand'=$1`, managedBrand).Scan(&failures); err != nil || failures != 1 {
		t.Fatal("source failure lacks committed audit", failures, err)
	}
	grantReportPermission(t, f, "brand.view.brand")
	if _, err := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key='commission.view.brand'`, f.root); err != nil {
		t.Fatal(err)
	}
	response = f.call("GET", workbenchPath, "", f.token, managedBrand, nil)
	mustStatus(t, response, 200)
	managedData(t, response, &out)
	if out.Commissions.Status != "forbidden" || out.Commissions.Data != nil {
		t.Fatal("ungranted table was accessed or data returned", out.Commissions)
	}
}
