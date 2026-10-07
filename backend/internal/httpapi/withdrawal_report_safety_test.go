package httpapi

import (
	"context"
	"strings"
	"testing"
)

func TestWithdrawalReportAuditFailureWithholdsJSONAndCSV(t *testing.T) {
	f := pointsFixture(t)
	configureWithdrawalHTTP(t, f)
	f.http = withdrawalHTTP(t, f, withdrawalHTTPChecker{})
	withdrawalHTTPCreate(t, f, "report-audit-failure-order")
	grantReportPermission(t, f.managementHTTP, "report_withdrawal.view.brand")
	grantReportPermission(t, f.managementHTTP, "report_withdrawal.export.brand")
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `CREATE FUNCTION reject_withdrawal_report_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action IN ('report.withdrawal.view','report.withdrawal.export') THEN RAISE EXCEPTION 'test audit unavailable'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_withdrawal_report_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_withdrawal_report_audit()`); err != nil {
		t.Fatal(err)
	}
	for _, export := range []bool{false, true} {
		r := f.call("GET", withdrawalReportURL(export, f.memberID), "", f.token, managedBrand, nil)
		mustStatus(t, r, 503)
		if strings.Contains(r.Body.String(), "requested_points") || strings.Contains(r.Body.String(), f.memberID) || strings.Contains(r.Header().Get("Content-Type"), "text/csv") || r.Header().Get("X-Report-Audit-ID") != "" {
			t.Fatal("unaudited report leaked", r.Header(), r.Body.String())
		}
	}
}

func TestWithdrawalReportRevokedPermissionDoesNotReadBusinessTable(t *testing.T) {
	f := pointsFixture(t)
	configureWithdrawalHTTP(t, f)
	f.http = withdrawalHTTP(t, f, withdrawalHTTPChecker{})
	withdrawalHTTPCreate(t, f, "report-revoked-order")
	grantReportPermission(t, f.managementHTTP, "report_withdrawal.view.brand")
	grantReportPermission(t, f.managementHTTP, "report_withdrawal.export.brand")
	mustStatus(t, f.call("GET", withdrawalReportURL(false, f.memberID), "", f.token, managedBrand, nil), 200)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE permission_key IN ('report_withdrawal.view.brand','report_withdrawal.export.brand') AND role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1)`, f.root); err != nil {
		t.Fatal(err)
	}
	// Rename only this test's schema table to prove rejection happens before the
	// underlying financial SELECT, not by catching a failed data query afterward.
	if _, err := f.pool.Exec(ctx, `ALTER TABLE withdrawal_orders RENAME TO withdrawal_orders_revocation_test`); err != nil {
		t.Fatal(err)
	}
	for _, export := range []bool{false, true} {
		mustStatus(t, f.call("GET", withdrawalReportURL(export, f.memberID), "", f.token, managedBrand, nil), 403)
	}
}
