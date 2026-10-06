package httpapi

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/betting"
	"testing"
)

func TestSettlementJobsHTTPRequiresExplicitModeAndReplaysWithoutDoublePayout(t *testing.T) {
	f, o := settlementHTTPFixture(t)
	ctx := context.Background()
	s := betting.Service{DB: f.pool}
	policyPath := "/api/v1/admin/settlement-policy"
	periodPath := "/api/v1/admin/periods/" + o.PeriodID
	mustStatus(t, f.call("GET", policyPath, "", f.token, managedBrand, nil), 403)
	for _, key := range []string{"settlement.view.brand", "settlement.run.brand", "settlement.approve.brand", "settlement.retry.brand", "settlement_policy.view.brand", "settlement_policy.write.brand"} {
		if _, e := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) SELECT role_id,$2 FROM admin_account_roles WHERE account_id=$1 ON CONFLICT DO NOTHING`, f.root, key); e != nil {
			t.Fatal(e)
		}
	}
	var p betting.SettlementPolicy
	r := f.call("GET", policyPath, "", f.token, managedBrand, nil)
	mustStatus(t, r, 200)
	managedData(t, r, &p)
	if p.Mode != nil || p.Version != 1 {
		t.Fatal(p)
	}
	var c betting.PeriodSettlementContext
	r = f.call("GET", periodPath+"/settlement-context", "", f.token, managedBrand, nil)
	mustStatus(t, r, 200)
	managedData(t, r, &c)
	in := betting.SettlementStartInput{Version: c.PeriodVersion, PolicyVersion: 1, DrawResultID: *c.DrawResultID, Reason: "real settlement admission"}
	mustStatus(t, f.call("POST", periodPath+"/settle", "unset-denied", f.token, managedBrand, in), 409)
	mustStatus(t, f.rawCall("PUT", policyPath, "mode-missing", f.token, managedBrand, `{"version":1,"reason":"no mode field"}`), 400)
	mustStatus(t, f.rawCall("PUT", policyPath, "mode-duplicate", f.token, managedBrand, `{"version":1,"mode":"automatic","mode":"manual","reason":"duplicate"}`), 400)
	mode := "manual"
	pin := betting.SettlementPolicyInput{Version: 1, Mode: &mode, Reason: "explicit manual test fixture"}
	r = f.call("PUT", policyPath, "manual-policy-once", f.token, managedBrand, pin)
	mustStatus(t, r, 200)
	managedData(t, r, &p)
	if p.Version != 2 || p.AuditLogID == "" {
		t.Fatal(p)
	}
	mustStatus(t, f.call("PUT", policyPath, "manual-policy-once", f.token, managedBrand, pin), 200)
	in.PolicyVersion = p.Version
	r = f.call("POST", periodPath+"/settle", "settle-once", f.token, managedBrand, in)
	mustStatus(t, r, 201)
	var j betting.SettlementJob
	managedData(t, r, &j)
	before := pointWallet(t, f)
	if _, e := s.ProcessSettlements(ctx, 20); e != nil {
		t.Fatal(e)
	}
	j, e := s.SettlementJob(ctx, managedBrand, j.ID)
	if e != nil || j.State != "awaiting_approval" || pointWallet(t, f).BySource != before.BySource {
		t.Fatal(j, e)
	}
	path := "/api/v1/admin/settlement-jobs/" + j.ID
	ain := betting.SettlementActionInput{Version: j.Version, Reason: "approved explicit test winnings"}
	mustStatus(t, f.call("POST", path+"/approve", "approve-once", f.token, managedBrand, ain), 200)
	if _, e = s.ProcessSettlements(ctx, 20); e != nil {
		t.Fatal(e)
	}
	replay := f.call("POST", periodPath+"/settle", "settle-once", f.token, managedBrand, in)
	mustStatus(t, replay, 201)
	var cached betting.SettlementJob
	managedData(t, replay, &cached)
	if cached.ID != j.ID || cached.State != "processing" {
		t.Fatal("expected original immutable idempotent receipt", cached)
	}
	mustStatus(t, f.call("POST", path+"/approve", "approve-once", f.token, managedBrand, ain), 200)
	var fresh betting.SettlementJob
	r = f.call("GET", path, "", f.token, managedBrand, nil)
	mustStatus(t, r, 200)
	managedData(t, r, &fresh)
	if fresh.State != "completed" || fresh.PaidCount != 1 || fresh.PaidPoints != "10" {
		t.Fatal(fresh)
	}
	mustStatus(t, f.call("GET", path+"/targets?limit=20&offset=0", "", f.token, managedBrand, nil), 200)
	var count int
	if e = f.pool.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE entry_type='prize'`).Scan(&count); e != nil || count != 1 {
		t.Fatal(count, e)
	}
	if w := pointWallet(t, f); w.WinningPoints != 10 || w.RechargePoints != 99 {
		t.Fatal(w)
	}
	changed := ain
	changed.Reason = "different intent"
	mustStatus(t, f.call("POST", path+"/approve", "approve-once", f.token, managedBrand, changed), 409)
	mustStatus(t, f.call("GET", path, "", f.userToken, managedBrand, nil), 401)
	mustStatus(t, f.call("GET", path, "", f.token, pointsBrandB, nil), 403)
	if _, e = f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key='settlement.approve.brand'`, f.root); e != nil {
		t.Fatal(e)
	}
	mustStatus(t, f.call("POST", path+"/approve", "approve-once", f.token, managedBrand, ain), 403)
	if _, e = f.pool.Exec(ctx, `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); e != nil {
		t.Fatal(e)
	}
	mustStatus(t, f.call("PUT", policyPath, "super-write-denied", f.token, managedBrand, pin), 403)
	mustStatus(t, f.call("POST", periodPath+"/settle", "settle-once", f.token, managedBrand, in), 403)
}
