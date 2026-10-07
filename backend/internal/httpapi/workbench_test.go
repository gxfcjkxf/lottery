package httpapi

import (
	"context"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/workbench"
)

const workbenchPath = "/api/v1/admin/workbench"

func TestWorkbenchHTTPFreshAuthorizationAndScope(t *testing.T) {
	f := managedFixture(t)
	ctx := context.Background()
	mustStatus(t, f.call("GET", workbenchPath, "", "", managedBrand, nil), 401)
	for _, path := range []string{workbenchPath + "?", workbenchPath + "?brand_id=" + managedBrand, workbenchPath + "?from=2026-01-01"} {
		mustStatus(t, f.call("GET", path, "", f.token, managedBrand, nil), 400)
	}
	mustStatus(t, f.call("GET", workbenchPath, "", f.token, "bad-id", nil), 400)
	r := f.call("GET", workbenchPath, "", f.token, managedBrand, nil)
	mustStatus(t, r, 200)
	var out workbench.Snapshot
	managedData(t, r, &out)
	if out.Brand.Status != "ready" || out.Brand.Data == nil || out.Ledger.Status != "forbidden" || out.Ledger.Data != nil || out.Withdrawals.Status != "forbidden" || out.Withdrawals.Data != nil {
		t.Fatalf("incorrect section authorization: %+v", out)
	}
	mustStatus(t, f.call("GET", workbenchPath, "", f.token, pointsBrandB, nil), 403)
	if _, err := f.pool.Exec(ctx, `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("GET", workbenchPath, "", f.token, pointsBrandB, nil), 403)
	grantReportPermission(t, f, "brand.view.platform")
	mustStatus(t, f.call("GET", workbenchPath, "", f.token, pointsBrandB, nil), 200)
	mustStatus(t, f.call("GET", workbenchPath, "", f.token, "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee", nil), 404)
	if _, err := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key IN('brand.view.platform','brand.view.brand')`, f.root); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("GET", workbenchPath, "", f.token, managedBrand, nil), 403)
}

func TestWorkbenchHTTPRealRechargeFreezeAndNoBusinessWrites(t *testing.T) {
	f := pointsFixture(t)
	ctx := context.Background()
	grantReportPermission(t, f.managementHTTP, reportLedgerBrand)
	order := pointRecharge(t, f, "125", "confidential receipt reason", "workbench-funded-create")
	pending := pointRecharge(t, f, "40", "another private reason", "workbench-pending-create")
	_ = pending
	mustStatus(t, f.call("POST", "/api/v1/admin/recharges/"+order.ID+"/confirm", "workbench-funded-confirm", f.token, managedBrand, map[string]any{"version": order.Version, "reason": "private finance confirmation"}), 200)
	mustStatus(t, f.call("POST", "/api/v1/admin/wallets/"+f.memberID+"/freeze", "workbench-funded-freeze", f.token, managedBrand, map[string]string{"points": "20", "reason": "private freeze reason"}), 200)
	var before, after int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"active", "paused", "disabled"} {
		if _, err := f.pool.Exec(ctx, `UPDATE brands SET status=$2 WHERE id=$1`, managedBrand, status); err != nil {
			t.Fatal(err)
		}
		r := f.call("GET", workbenchPath, "", f.token, managedBrand, nil)
		mustStatus(t, r, 200)
		var out workbench.Snapshot
		managedData(t, r, &out)
		if out.Brand.Data.State != status || out.Recharges.Data.PendingCount != "1" || out.Recharges.Data.PendingPoints != "40" || out.Ledger.Data.EntryCount != "2" || out.Ledger.Data.NetPoints != "125" || out.Ledger.Data.RechargePoints != "125" || out.Balances.Data.AvailablePoints != "105" || out.Balances.Data.FrozenPoints != "20" || out.Balances.Data.TotalPoints != "125" {
			t.Fatalf("incorrect live financial facts: %+v / %+v / %+v", out.Recharges.Data, out.Ledger.Data, out.Balances.Data)
		}
		for _, secret := range []string{f.memberID, "Points member", "confidential receipt", "private freeze", "manual-proof", "proof_reference", "source_allocation"} {
			if strings.Contains(r.Body.String(), secret) {
				t.Fatalf("private data leaked: %s", secret)
			}
		}
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries`).Scan(&after); err != nil || before != after {
		t.Fatalf("GET changed ledger: %d -> %d (%v)", before, after, err)
	}
	var audits int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='workbench.view'`).Scan(&audits); err != nil || audits != 3 {
		t.Fatalf("successful reads must be audited: %d (%v)", audits, err)
	}
}

func TestWorkbenchHTTPRealWithdrawalQueueRequiresIndependentViewGrant(t *testing.T) {
	f := pointsFixture(t)
	configureWithdrawalHTTP(t, f)
	f.http = withdrawalHTTP(t, f, withdrawalHTTPChecker{})
	order := withdrawalHTTPCreate(t, f, "workbench-withdrawal-queue")

	if _, err := f.pool.Exec(context.Background(), `DELETE FROM role_permissions WHERE permission_key='withdrawal.view.brand' AND role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1)`, f.root); err != nil {
		t.Fatal(err)
	}
	r := f.call("GET", workbenchPath, "", f.token, managedBrand, nil)
	mustStatus(t, r, 200)
	var out workbench.Snapshot
	managedData(t, r, &out)
	if out.Withdrawals.Status != "forbidden" || out.Withdrawals.Data != nil {
		t.Fatalf("withdrawal workflow grants must not imply queue read: %+v", out.Withdrawals)
	}

	grantReportPermission(t, f.managementHTTP, "withdrawal.view.brand")
	r = f.call("GET", workbenchPath, "", f.token, managedBrand, nil)
	mustStatus(t, r, 200)
	managedData(t, r, &out)
	if out.Withdrawals.Status != "ready" || out.Withdrawals.Data == nil || out.Withdrawals.Data.ReviewingCount != "1" || out.Withdrawals.Data.ReviewingPoints != "50" || out.Withdrawals.Data.ProcessingCount != "0" || out.Withdrawals.Data.ProcessingPoints != "0" {
		t.Fatalf("review queue did not reflect persisted order %s: %+v", order.ID, out.Withdrawals)
	}

	mustStatus(t, f.call("POST", "/api/v1/admin/withdrawals/"+order.ID+"/approve", "workbench-withdrawal-approve", f.token, managedBrand, map[string]any{"version": 1, "reason": "move into processing"}), 200)
	r = f.call("GET", workbenchPath, "", f.token, managedBrand, nil)
	mustStatus(t, r, 200)
	managedData(t, r, &out)
	if out.Withdrawals.Data == nil || out.Withdrawals.Data.ReviewingCount != "0" || out.Withdrawals.Data.ReviewingPoints != "0" || out.Withdrawals.Data.ProcessingCount != "1" || out.Withdrawals.Data.ProcessingPoints != "50" {
		t.Fatalf("processing queue did not reflect persisted transition: %+v", out.Withdrawals)
	}
	mustStatus(t, f.call("POST", "/api/v1/admin/withdrawals/"+order.ID+"/fail", "workbench-withdrawal-fail", f.token, managedBrand, map[string]any{"version": 2, "reason": "terminal test state"}), 200)
	r = f.call("GET", workbenchPath, "", f.token, managedBrand, nil)
	mustStatus(t, r, 200)
	managedData(t, r, &out)
	if out.Withdrawals.Data == nil || out.Withdrawals.Data.ReviewingCount != "0" || out.Withdrawals.Data.ReviewingPoints != "0" || out.Withdrawals.Data.ProcessingCount != "0" || out.Withdrawals.Data.ProcessingPoints != "0" {
		t.Fatalf("terminal withdrawal must not be included in in-progress totals: %+v", out.Withdrawals)
	}
}

func TestWorkbenchHTTPFailureReturnsNoSnapshotAndAuditsReadFailure(t *testing.T) {
	f := managedFixture(t)
	ctx := context.Background()
	grantReportPermission(t, f, reportLedgerBrand)
	if _, err := f.pool.Exec(ctx, `ALTER TABLE point_buckets RENAME TO workbench_hidden_buckets`); err != nil {
		t.Fatal(err)
	}
	r := f.call("GET", workbenchPath, "", f.token, managedBrand, nil)
	mustStatus(t, r, 503)
	if strings.Contains(r.Body.String(), `"data"`) || strings.Contains(r.Body.String(), `"brand_id"`) {
		t.Fatal("failed query leaked a partial snapshot", r.Body.String())
	}
	var audits int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='workbench.read_failed'`).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("query failure audit=%d err=%v", audits, err)
	}
}

func TestWorkbenchHTTPAuditFailureWithholdsSuccessfulData(t *testing.T) {
	f := managedFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `CREATE FUNCTION reject_workbench_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='workbench.view' THEN RAISE EXCEPTION 'test audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_workbench_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_workbench_audit()`); err != nil {
		t.Fatal(err)
	}
	r := f.call("GET", workbenchPath, "", f.token, managedBrand, nil)
	mustStatus(t, r, 503)
	if strings.Contains(r.Body.String(), `"data"`) {
		t.Fatal("uncommitted audit returned snapshot", r.Body.String())
	}
	if _, err := f.pool.Exec(ctx, `DROP TRIGGER reject_workbench_audit ON audit_logs`); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("GET", workbenchPath, "", f.token, managedBrand, nil), 200)
	if _, err := f.pool.Exec(ctx, `UPDATE admin_accounts SET status='disabled' WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("GET", workbenchPath, "", f.token, managedBrand, nil), 401)
}
