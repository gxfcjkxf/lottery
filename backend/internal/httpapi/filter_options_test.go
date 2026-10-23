package httpapi

import (
	"context"
	"testing"
)

func TestFilterOptionsUseReportPermissionAndBrandScope(t *testing.T) {
	f := pointsFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) SELECT role_id,'report_commission.view.brand' FROM admin_account_roles WHERE account_id=$1 ON CONFLICT DO NOTHING`, f.root); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key='user.view.brand'`, f.root); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/admin/filter-options?resource=report_commission&kind=member&limit=100&offset=0"
	out := f.call("GET", path, "", f.token, managedBrand, nil)
	mustStatus(t, out, 200)
	var data struct {
		Brand string `json:"brand_id"`
		Items []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"items"`
	}
	managedData(t, out, &data)
	if data.Brand != managedBrand || len(data.Items) != 1 || data.Items[0].Name != "points_member" || data.Items[0].ID != f.memberID || out.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(data)
	}
	mustStatus(t, f.call("GET", path, "", f.token, pointsBrandB, nil), 403)
	mustStatus(t, f.call("GET", path+"&kind=agent", "", f.token, managedBrand, nil), 400)
	mustStatus(t, f.call("GET", "/api/v1/admin/filter-options?resource=report_commission&kind=reward_order", "", f.token, managedBrand, nil), 400)
	if _, err := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key='report_commission.view.brand'`, f.root); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("GET", path, "", f.token, managedBrand, nil), 403)
}

func TestFilterOptionsQueryAllSupportedNames(t *testing.T) {
	f := pointsFixture(t)
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO role_permissions(role_id,permission_key) SELECT ar.role_id,p.key FROM admin_account_roles ar CROSS JOIN permissions p WHERE ar.account_id=$1 AND p.key IN ('commission.view.brand','report_betting.view.brand','report_commission.view.brand','report_reward.view.brand','audit.view.brand','period.view.brand','draw.view.brand') ON CONFLICT DO NOTHING`, f.root); err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{{"report_betting", "game"}, {"report_commission", "agent"}, {"report_commission", "cycle"}, {"report_rewards", "reward_order"}, {"audit", "actor"}, {"audit", "audit_resource"}, {"period", "period"}, {"commission", "bet_order"}} {
		path := "/api/v1/admin/filter-options?resource=" + pair[0] + "&kind=" + pair[1] + "&limit=100&offset=0"
		t.Run(pair[0]+"_"+pair[1], func(t *testing.T) { mustStatus(t, f.call("GET", path, "", f.token, managedBrand, nil), 200) })
	}
}
