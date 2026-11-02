package httpapi

import (
	"context"
	"strings"
	"testing"
)

func TestAccountOptionsPrefixAndGrantScope(t *testing.T) {
	f := pointsFixture(t)
	path := "/api/v1/admin/filter-options?resource=reward&action=grant&kind=member&limit=100&offset=0&search=po"
	mustStatus(t, f.call("GET", path, "", f.token, managedBrand, nil), 403)
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO role_permissions(role_id,permission_key) SELECT role_id,'reward.grant.brand' FROM admin_account_roles WHERE account_id=$1 ON CONFLICT DO NOTHING`, f.root); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(context.Background(), `DELETE FROM role_permissions WHERE role_id IN (SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key='user.view.brand'`, f.root); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		search string
		count  int
	}{{"po", 1}, {"PO", 1}, {"points_", 1}, {"points%", 0}, {"oints", 0}, {"member", 0}} {
		response := f.call("GET", strings.Replace(path, "search=po", "search="+strings.ReplaceAll(tc.search, "%", "%25"), 1), "", f.token, managedBrand, nil)
		mustStatus(t, response, 200)
		var page struct{ Items []struct{ ID, Name string } }
		managedData(t, response, &page)
		if len(page.Items) != tc.count || tc.count > 0 && (page.Items[0].ID != f.memberID || page.Items[0].Name != "points_member") {
			t.Fatalf("%s: %s", tc.search, response.Body.String())
		}
	}
	for _, search := range []string{"", "p"} {
		mustStatus(t, f.call("GET", strings.Replace(path, "search=po", "search="+search, 1), "", f.token, managedBrand, nil), 400)
	}
	mustStatus(t, f.call("GET", path, "", f.token, pointsBrandB, nil), 403)
	mustStatus(t, f.call("GET", strings.Replace(path, "resource=reward", "resource=recharge", 1), "", f.token, managedBrand, nil), 400)
	mustStatus(t, f.call("GET", strings.Replace(path, "action=grant", "action=view", 1), "", f.token, managedBrand, nil), 403)
	rechargePath := strings.Replace(strings.Replace(path, "resource=reward", "resource=recharge", 1), "action=grant", "action=view", 1)
	mustStatus(t, f.call("GET", rechargePath, "", f.token, managedBrand, nil), 200)
}
