package httpapi

import (
	"context"
	"strings"
	"testing"
)

func TestPointsAdjustmentSharesQuotaAndPreservesRemark(t *testing.T) {
	f := pointsFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `UPDATE brand_recharge_quotas SET available=100 WHERE brand_id=$1`, managedBrand); err != nil {
		t.Fatal(err)
	}
	quota := func(want string) {
		t.Helper()
		var got string
		if err := f.pool.QueryRow(ctx, `SELECT available::text FROM brand_recharge_quotas WHERE brand_id=$1`, managedBrand).Scan(&got); err != nil || got != want {
			t.Fatalf("quota=%s want=%s err=%v", got, want, err)
		}
	}
	path := "/api/v1/admin/wallets/" + f.memberID + "/adjust"
	for _, source := range []string{"recharge", "winning", "gift", "commission"} {
		body := map[string]string{"source": source, "delta": "10", "reason": "可选备注 adjustment"}
		key := "quota-adjust-" + source
		for i := 0; i < 2; i++ {
			r := f.call("POST", path, key, f.token, managedBrand, body)
			if r.Code != 200 {
				t.Fatalf("%s status=%d body=%s", source, r.Code, r.Body.String())
			}
			if !strings.Contains(r.Body.String(), "可选备注 adjustment") {
				t.Fatal("remark missing from receipt")
			}
		}
	}
	quota("60")
	if pointWallet(t, f).AvailablePoints != 4000 {
		t.Fatal("replay credited twice")
	}
	// A recharge consumes the very same pool.
	r := f.call("POST", "/api/v1/admin/recharges/manual", "quota-adjust-manual-recharge", f.token, managedBrand, map[string]string{"member_id": f.memberID, "points": "50", "remark": "", "reason": ""})
	if r.Code != 201 {
		t.Fatal(r.Code, r.Body.String())
	}
	quota("10")
	before := pointWallet(t, f)
	r = f.call("POST", path, "quota-adjust-exceeded", f.token, managedBrand, map[string]string{"source": "gift", "delta": "11", "reason": ""})
	if r.Code != 409 || !strings.Contains(r.Body.String(), "RECHARGE_QUOTA_INSUFFICIENT") {
		t.Fatal(r.Code, r.Body.String())
	}
	after := pointWallet(t, f)
	if after.Version != before.Version || after.AvailablePoints != before.AvailablePoints {
		t.Fatal("quota rejection changed wallet")
	}
	quota("10")
	for _, row := range []struct {
		path, key string
		body      map[string]string
	}{
		{path, "quota-adjust-debit", map[string]string{"source": "gift", "delta": "-5", "reason": ""}},
		{"/api/v1/admin/wallets/" + f.memberID + "/freeze", "quota-adjust-freeze", map[string]string{"points": "5", "reason": "冻结备注"}},
	} {
		r = f.call("POST", row.path, row.key, f.token, managedBrand, row.body)
		if r.Code != 200 {
			t.Fatal(r.Code, r.Body.String())
		}
		quota("10")
	}
	// Brand scoping is enforced before either wallet or quota can change.
	r = f.call("POST", path, "quota-adjust-other-brand", f.token, pointsBrandB, map[string]string{"source": "gift", "delta": "1", "reason": ""})
	if r.Code != 403 {
		t.Fatal(r.Code, r.Body.String())
	}
	quota("10")
	var records int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM brand_recharge_quota_entries WHERE brand_id=$1 AND kind='manual_adjustment'`, managedBrand).Scan(&records); err != nil || records != 4 {
		t.Fatalf("quota receipts=%d err=%v", records, err)
	}
}
