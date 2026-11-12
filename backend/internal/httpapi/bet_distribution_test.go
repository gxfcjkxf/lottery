package httpapi

import (
	"context"
	"testing"
)

func TestBetDistributionReadPermissionAndBrandScope(t *testing.T) {
	f := managedFixture(t)
	directory := "/api/v1/admin/bet-distribution/games"
	distribution := "/api/v1/admin/bet-distribution?game_id=" + managedBrand
	mustStatus(t, f.call("GET", directory, "", f.token, managedBrand, nil), 403)
	mustStatus(t, f.call("GET", distribution, "", f.token, managedBrand, nil), 403)
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO role_permissions(role_id,permission_key) SELECT role_id,'bet.view.brand' FROM admin_account_roles WHERE account_id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("GET", directory, "", f.token, managedBrand, nil), 200)
	mustStatus(t, f.call("GET", distribution, "", f.token, managedBrand, nil), 404)
	mustStatus(t, f.call("GET", directory, "", f.token, pointsBrandB, nil), 403)
	mustStatus(t, f.call("GET", distribution, "", f.token, pointsBrandB, nil), 403)
	mustStatus(t, f.call("GET", "/api/v1/admin/bet-distribution?game_id=invalid", "", f.token, managedBrand, nil), 400)
	mustStatus(t, f.call("GET", directory, "", "", managedBrand, nil), 401)
}
