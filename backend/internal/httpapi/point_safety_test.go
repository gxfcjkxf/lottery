package httpapi

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/finance"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"testing"
)

func safetyFixture(t *testing.T) pointsHTTPFixture {
	t.Helper()
	f := pointsFixture(t)
	_, err := f.pool.Exec(context.Background(), `INSERT INTO role_permissions(role_id,permission_key) SELECT ar.role_id,p.key FROM admin_account_roles ar CROSS JOIN permissions p WHERE ar.account_id=$1 AND p.key IN('point_policy.view.brand','point_policy.write.brand','wallet.repair.brand') ON CONFLICT DO NOTHING`, f.root)
	if err != nil {
		t.Fatal(err)
	}
	return f
}
func TestPointSafetyPolicyLimitsAndPendingCancellation(t *testing.T) {
	f := safetyFixture(t)
	policy := f.call("GET", "/api/v1/admin/point-policy", "", f.token, managedBrand, nil)
	if policy.Code != 200 {
		t.Fatal(policy.Code, policy.Body.String())
	}
	update := map[string]any{"version": 1, "max_balance_points": "100", "max_recharge_points": "70", "max_adjustment_points": "20", "reason": "set brand caps"}
	r := f.call("PUT", "/api/v1/admin/point-policy", "policy-save-001", f.token, managedBrand, update)
	if r.Code != 200 {
		t.Fatal(r.Code, r.Body.String())
	}
	replay := f.call("PUT", "/api/v1/admin/point-policy", "policy-save-001", f.token, managedBrand, update)
	if replay.Code != 200 {
		t.Fatal(replay.Code, replay.Body.String())
	}
	var replayed points.Policy
	managedData(t, replay, &replayed)
	if replayed.Version != 2 {
		t.Fatal(replayed)
	}
	incomplete := f.call("PUT", "/api/v1/admin/point-policy", "policy-incomplete-002", f.token, managedBrand, map[string]any{"version": 2, "reason": "must not implicitly clear caps"})
	if incomplete.Code != 400 {
		t.Fatal(incomplete.Code, incomplete.Body.String())
	}
	unchanged := f.call("GET", "/api/v1/admin/point-policy", "", f.token, managedBrand, nil)
	var remaining points.Policy
	managedData(t, unchanged, &remaining)
	if remaining.Version != 2 || remaining.MaxRechargePoints == nil || *remaining.MaxRechargePoints != 70 {
		t.Fatal(remaining)
	}
	over := f.call("POST", "/api/v1/admin/recharges", "recharge-cap-001", f.token, managedBrand, map[string]string{"member_id": f.memberID, "points": "71", "reason": "too large"})
	if over.Code != 409 {
		t.Fatal(over.Code, over.Body.String())
	}
	recharge := pointRecharge(t, f, "70", "within cap", "recharge-cap-002")
	cancelled := f.call("POST", "/api/v1/admin/recharges/"+recharge.ID+"/cancel", "cancel-pending-001", f.token, managedBrand, map[string]any{"version": recharge.Version, "reason": "incorrect proof"})
	if cancelled.Code != 200 {
		t.Fatal(cancelled.Code, cancelled.Body.String())
	}
	var record finance.Recharge
	managedData(t, cancelled, &record)
	if record.State != "cancelled" || record.Version != 2 || pointWallet(t, f).DisplayPoints != 0 {
		t.Fatal(record)
	}
	denied := f.call("POST", "/api/v1/admin/recharges/"+record.ID+"/confirm", "confirm-cancelled-001", f.token, managedBrand, map[string]any{"version": record.Version, "reason": "cannot confirm cancelled"})
	if denied.Code != 409 {
		t.Fatal(denied.Code, denied.Body.String())
	}
	adjustment := f.call("POST", "/api/v1/admin/wallets/"+f.memberID+"/adjust", "adjust-cap-001", f.token, managedBrand, map[string]string{"source": "gift", "delta": "21", "reason": "over operation cap"})
	if adjustment.Code != 409 {
		t.Fatal(adjustment.Code, adjustment.Body.String())
	}
	stale := f.call("PUT", "/api/v1/admin/point-policy", "policy-stale-002", f.token, managedBrand, update)
	if stale.Code != 409 {
		t.Fatal(stale.Code, stale.Body.String())
	}
	foreign := f.call("GET", "/api/v1/admin/point-policy", "", f.token, pointsBrandB, nil)
	if foreign.Code != 403 {
		t.Fatal(foreign.Code, foreign.Body.String())
	}
}
func TestPointSafetyRepairPreviewApplyReplayAndAudit(t *testing.T) {
	f := safetyFixture(t)
	ctx := context.Background()
	recharge := pointRecharge(t, f, "100", "test projection", "repair-recharge-001")
	confirmed := f.call("POST", "/api/v1/admin/recharges/"+recharge.ID+"/confirm", "repair-confirm-001", f.token, managedBrand, map[string]any{"version": 1, "reason": "confirm test"})
	if confirmed.Code != 200 {
		t.Fatal(confirmed.Code, confirmed.Body.String())
	}
	wallet := pointWallet(t, f)
	if _, err := f.pool.Exec(ctx, `UPDATE point_buckets SET points=999 WHERE account_id=$1 AND source='recharge' AND state='available'`, wallet.AccountID); err != nil {
		t.Fatal(err)
	}
	r := f.call("GET", "/api/v1/admin/wallets/"+f.memberID+"/repair-preview", "", f.token, managedBrand, nil)
	if r.Code != 200 {
		t.Fatal(r.Code, r.Body.String())
	}
	var preview points.RepairPreview
	managedData(t, r, &preview)
	if !preview.Repairable || preview.Expected[0][0] != 100 {
		t.Fatal(preview)
	}
	body := map[string]any{"version": preview.Version, "token": preview.Token, "reason": "restore ledger projection"}
	applied := f.call("POST", "/api/v1/admin/wallets/"+f.memberID+"/repair", "repair-apply-001", f.token, managedBrand, body)
	if applied.Code != 200 {
		t.Fatal(applied.Code, applied.Body.String())
	}
	var record points.RepairRecord
	managedData(t, applied, &record)
	if record.ID == "" || record.AuditLogID == "" || pointWallet(t, f).DisplayPoints != 100 {
		t.Fatal(record)
	}
	replay := f.call("POST", "/api/v1/admin/wallets/"+f.memberID+"/repair", "repair-apply-001", f.token, managedBrand, body)
	if replay.Code != 200 {
		t.Fatal(replay.Code, replay.Body.String())
	}
	var same points.RepairRecord
	managedData(t, replay, &same)
	if same.ID != record.ID {
		t.Fatal("duplicate repair", same)
	}
	stale := f.call("POST", "/api/v1/admin/wallets/"+f.memberID+"/repair", "repair-stale-002", f.token, managedBrand, body)
	if stale.Code != 409 {
		t.Fatal(stale.Code, stale.Body.String())
	}
	history := f.call("GET", "/api/v1/admin/wallets/"+f.memberID+"/repairs", "", f.token, managedBrand, nil)
	if history.Code != 200 {
		t.Fatal(history.Code, history.Body.String())
	}
	var h struct {
		Items []points.RepairHistory `json:"items"`
	}
	managedData(t, history, &h)
	if len(h.Items) != 1 || h.Items[0].Reason != "restore ledger projection" {
		t.Fatal(h)
	}
	var count int
	f.pool.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE account_id=$1`, wallet.AccountID).Scan(&count)
	if count != 1 {
		t.Fatal("repair fabricated business ledger", count)
	}
	var role string
	f.pool.QueryRow(ctx, `SELECT role_id::text FROM admin_account_roles WHERE account_id=$1`, f.root).Scan(&role)
	f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1 AND permission_key='wallet.repair.brand'`, role)
	noGrant := f.call("POST", "/api/v1/admin/wallets/"+f.memberID+"/repair", "repair-deny-003", f.token, managedBrand, body)
	if noGrant.Code != 403 {
		t.Fatal(noGrant.Code, noGrant.Body.String())
	}
}
