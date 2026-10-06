package httpapi

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/betting"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"testing"
)

func TestCorrectionHTTPRealReversalReplayAndSecondaryPermissionRecheck(t *testing.T) {
	f, o := settlementHTTPFixture(t)
	ctx := context.Background()
	s := betting.Service{DB: f.pool}
	for _, key := range []string{"settlement.view.brand", "settlement.run.brand", "settlement.approve.brand", "settlement.retry.brand", "settlement_policy.view.brand", "settlement_policy.write.brand", "draw.view.brand", "draw.correct.brand", "draw.correction_retry.brand"} {
		if _, e := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) SELECT role_id,$2 FROM admin_account_roles WHERE account_id=$1 ON CONFLICT DO NOTHING`, f.root, key); e != nil {
			t.Fatal(e)
		}
	}
	mode := "automatic"
	r := f.call("PUT", "/api/v1/admin/settlement-policy", "corr-policy-auto", f.token, managedBrand, betting.SettlementPolicyInput{Version: 1, Mode: &mode, Reason: "explicit correction fixture"})
	mustStatus(t, r, 200)
	var policy betting.SettlementPolicy
	managedData(t, r, &policy)
	c, e := s.PeriodSettlementContext(ctx, managedBrand, o.PeriodID)
	if e != nil {
		t.Fatal(e)
	}
	r = f.call("POST", "/api/v1/admin/periods/"+o.PeriodID+"/settle", "corr-first-generation", f.token, managedBrand, betting.SettlementStartInput{Version: c.PeriodVersion, PolicyVersion: policy.Version, DrawResultID: *c.DrawResultID, Reason: "original actual winnings"})
	mustStatus(t, r, 201)
	if _, e = s.ProcessSettlements(ctx, 20); e != nil {
		t.Fatal(e)
	}
	corrContext, e := s.CorrectionContext(ctx, managedBrand, o.PeriodID)
	if e != nil {
		t.Fatal(e)
	}
	in := betting.CorrectionInput{Version: corrContext.PeriodVersion, PolicyVersion: &policy.Version, Result: rules.Draw{Regular: []int{}, Special: []int{}, Digits: []int{1, 2, 2}}, Reason: "verified correction to original result"}
	path := "/api/v1/admin/draw-results/" + *corrContext.DrawResultID + "/correct"
	r = f.call("POST", path, "correct-once", f.token, managedBrand, in)
	mustStatus(t, r, 201)
	var corr betting.Correction
	managedData(t, r, &corr)
	if corr.State != "reversing" || corr.PeriodVersion != in.Version+1 {
		t.Fatal(corr)
	}
	if _, e = s.ProcessCorrections(ctx, 20); e != nil {
		t.Fatal(e)
	}
	if _, e = s.ProcessSettlements(ctx, 20); e != nil {
		t.Fatal(e)
	}
	replay := f.call("POST", path, "correct-once", f.token, managedBrand, in)
	mustStatus(t, replay, 201)
	var original betting.Correction
	managedData(t, replay, &original)
	if original.ID != corr.ID || original.State != "reversing" || original.NewJobID != nil {
		t.Fatal("cache replaced with live progress", original)
	}
	fresh, e := s.Correction(ctx, managedBrand, corr.ID)
	if e != nil || fresh.State != "completed" {
		t.Fatal(fresh, e)
	}
	mustStatus(t, f.call("GET", "/api/v1/admin/corrections/"+corr.ID+"/targets?limit=20&offset=0", "", f.token, managedBrand, nil), 200)
	mustStatus(t, f.call("GET", "/api/v1/admin/periods/"+o.PeriodID+"/corrections?limit=20&offset=0", "", f.token, managedBrand, nil), 200)
	if w := pointWallet(t, f); w.WinningPoints != 0 || w.RechargePoints != 99 {
		t.Fatal(w)
	}
	var n int
	if e = f.pool.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE entry_type='prize_reversal'`).Scan(&n); e != nil || n != 1 {
		t.Fatal(n, e)
	}
	changed := in
	changed.Reason = "different intent"
	mustStatus(t, f.call("POST", path, "correct-once", f.token, managedBrand, changed), 409)
	mustStatus(t, f.call("GET", "/api/v1/admin/corrections/"+corr.ID, "", f.userToken, managedBrand, nil), 401)
	mustStatus(t, f.call("GET", "/api/v1/admin/corrections/"+corr.ID, "", f.token, pointsBrandB, nil), 403)
	// Losing only the secondary financial permission must reject a cached write
	// even while draw.correct remains granted; a callback-only check is too late.
	if _, e = f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key='settlement.run.brand'`, f.root); e != nil {
		t.Fatal(e)
	}
	mustStatus(t, f.call("POST", path, "correct-once", f.token, managedBrand, in), 403)
	if _, e = f.pool.Exec(ctx, `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); e != nil {
		t.Fatal(e)
	}
	mustStatus(t, f.call("POST", path, "correct-once", f.token, managedBrand, in), 403)
}
