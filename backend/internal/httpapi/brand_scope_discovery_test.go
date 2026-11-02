package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func ordinaryBrandReader(t *testing.T) managementHTTP {
	t.Helper()
	f := managedFixture(t)
	ctx := context.Background()
	// Use an explicitly central management host, with no user-brand binding.
	op := securityOperator(t, f)
	grantPlatformPermission(t, op, "brand_domains.write.platform")
	var version int64
	if err := f.pool.QueryRow(ctx, `SELECT config_version FROM brands WHERE id=$1`, managedBrand).Scan(&version); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, op.call("DELETE", "/api/v1/platform/brand-domains/0199a000-0000-7000-8000-000000000101", "reader-unbind-user-host", op.token, managedBrand, map[string]int64{"version": version}), 200)
	if _, err := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1)`, f.root); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) SELECT role_id,'overview.view.brand' FROM admin_account_roles WHERE account_id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestOrdinaryBrandScopeDiscovery(t *testing.T) {
	f := ordinaryBrandReader(t)
	ctx := context.Background()
	check := func(want []string) {
		t.Helper()
		r := f.call("GET", "/api/v1/admin/brands", "", f.token, "", nil)
		mustStatus(t, r, 200)
		var out struct {
			Items []struct {
				ID string `json:"id"`
			}
		}
		managedData(t, r, &out)
		if len(out.Items) != len(want) {
			t.Fatal("incorrect authorized scope", r.Body.String())
		}
		for i, id := range want {
			if out.Items[i].ID != id {
				t.Fatal("foreign brand or missing assignment", r.Body.String())
			}
		}
	}
	check([]string{managedBrand})
	mustStatus(t, f.call("GET", "/api/v1/admin/overview", "", f.token, managedBrand, nil), 200)
	mustStatus(t, f.call("GET", "/api/v1/admin/overview", "", f.token, pointsBrandB, nil), 403)
	mustStatus(t, f.call("GET", "/api/v1/admin/users", "", f.token, managedBrand, nil), 403)
	// Removing a business permission does not erase the assigned context or grant reads.
	if _, err := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1)`, f.root); err != nil {
		t.Fatal(err)
	}
	check([]string{managedBrand})
	mustStatus(t, f.call("GET", "/api/v1/admin/overview", "", f.token, managedBrand, nil), 403)
	if _, err := f.pool.Exec(ctx, `DELETE FROM admin_account_roles WHERE account_id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO admin_brand_scopes(account_id,brand_id) VALUES($1,$2)`, f.root, pointsBrandB); err != nil {
		t.Fatal(err)
	}
	check([]string{managedBrand, pointsBrandB})
	if _, err := f.pool.Exec(ctx, `DELETE FROM admin_brand_scopes WHERE account_id=$1 AND brand_id=$2`, f.root, managedBrand); err != nil {
		t.Fatal(err)
	}
	check([]string{pointsBrandB})
	if _, err := f.pool.Exec(ctx, `DELETE FROM admin_brand_scopes WHERE account_id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	check(nil)
	mustStatus(t, f.call("GET", "/api/v1/admin/brands", "", "", "", nil), 401)
	// Platform discovery still requires its existing brand.view.platform grant.
	op := securityOperator(t, f)
	r := op.call("GET", "/api/v1/platform/brands", "", op.token, "", nil)
	mustStatus(t, r, 200)
	var out struct{ Items []any }
	managedData(t, r, &out)
	if len(out.Items) != 0 {
		t.Fatal("platform discovery widened")
	}
	grantPlatformPermission(t, op, "brand.view.platform")
	r = op.call("GET", "/api/v1/platform/brands", "", op.token, "", nil)
	mustStatus(t, r, 200)
	managedData(t, r, &out)
	if len(out.Items) != 2 {
		t.Fatal("platform grant ignored")
	}
}

func TestOrdinaryBrandReaderBrowserFixture(t *testing.T) {
	file := os.Getenv("BRAND_SCOPE_FIXTURE_FILE")
	if file == "" {
		t.Skip("opt-in isolated browser fixture")
	}
	f := ordinaryBrandReader(t)
	server := httptest.NewServer(f.http)
	defer server.Close()
	raw, _ := json.Marshal(map[string]string{"url": server.URL, "database": f.pool.Config().ConnConfig.Database, "brand": managedBrand, "foreign_brand": pointsBrandB})
	if err := os.WriteFile(file, raw, 0600); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(file)
	deadline := time.NewTimer(3 * time.Minute)
	defer deadline.Stop()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline.C:
			t.Fatal("ordinary brand browser fixture timed out")
		case <-ticker.C:
			if _, err := os.Stat(file + ".stop"); err == nil {
				os.Remove(file + ".stop")
				return
			}
		}
	}
}

func TestOrdinaryBrandScopeExplicitMemberViewGrant(t *testing.T) {
	f := ordinaryBrandReader(t)
	ctx := context.Background()
	path := "/api/v1/admin/users?limit=100&offset=0"
	mustStatus(t, f.call("GET", path, "", f.token, managedBrand, nil), 403)
	result, err := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key)
 SELECT r.id,'user.view.brand' FROM admin_account_roles ar JOIN roles r ON r.id=ar.role_id
 WHERE ar.account_id=$1 AND r.brand_id=$2`, f.root, managedBrand)
	if err != nil || result.RowsAffected() != 1 {
		t.Fatal("cannot grant member view", err)
	}
	// Existing sessions load fresh exact grants; no login or account replacement.
	mustStatus(t, f.call("GET", path, "", f.token, managedBrand, nil), 200)
	mustStatus(t, f.call("GET", path, "", f.token, pointsBrandB, nil), 403)
	if _, err := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key='user.view.brand'`, f.root); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("GET", path, "", f.token, managedBrand, nil), 403)
}
