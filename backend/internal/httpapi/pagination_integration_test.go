package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

func TestPaginationTotalsMatchScopeAndStayStableAcrossPages(t *testing.T) {
	f := pointsFixture(t)
	ctx := context.Background()
	for _, tc := range []struct {
		path, query string
		args        []any
	}{
		{"/roles", `SELECT count(*)::text FROM roles WHERE brand_id=$1`, []any{managedBrand}},
		{"/accounts", `SELECT count(*)::text FROM admin_accounts a JOIN admin_brand_scopes b ON b.account_id=a.id AND b.brand_id=$1 WHERE a.status<>'deleted' AND NOT a.is_super_admin AND (SELECT count(*) FROM admin_brand_scopes s WHERE s.account_id=a.id)=1`, []any{managedBrand}},
		{"/wallets/" + f.memberID + "/ledger", `SELECT count(*)::text FROM point_ledger_entries WHERE brand_id=$1 AND member_id=$2`, []any{managedBrand, f.memberID}},
		{"/recharges", `SELECT count(*)::text FROM recharge_orders WHERE brand_id=$1`, []any{managedBrand}},
	} {
		t.Run(tc.path, func(t *testing.T) {
			var expected string
			if err := f.pool.QueryRow(ctx, tc.query, tc.args...).Scan(&expected); err != nil {
				t.Fatal(err)
			}
			for _, offset := range []int{0, 1, 99} {
				var page struct {
					Items []json.RawMessage `json:"items"`
					Total string            `json:"total_count"`
				}
				managedData(t, f.call("GET", fmt.Sprintf("/api/v1/admin%s?limit=1&offset=%d", tc.path, offset), "", f.token, managedBrand, nil), &page)
				if page.Total != expected || len(page.Items) > 1 {
					t.Fatalf("offset=%d total=%q want=%q items=%d", offset, page.Total, expected, len(page.Items))
				}
			}
		})
	}
	for _, resource := range []struct{ path, table string }{
		{"/wallet/ledger", "point_ledger_entries"},
		{"/recharges", "recharge_orders"},
		{"/notifications", "notifications"},
	} {
		var expected string
		if err := f.pool.QueryRow(ctx, "SELECT count(*)::text FROM "+resource.table+" WHERE brand_id=$1 AND member_id=$2", managedBrand, f.memberID).Scan(&expected); err != nil {
			t.Fatal(err)
		}
		for _, offset := range []int{0, 99} {
			var page struct {
				Total string `json:"total_count"`
			}
			managedData(t, f.call("GET", fmt.Sprintf("/api/v1/b/aurora%s?limit=1&offset=%d", resource.path, offset), "", f.userToken, "", nil), &page)
			if page.Total != expected {
				t.Fatalf("user %s offset=%d: total=%q want=%q", resource.path, offset, page.Total, expected)
			}
			mustStatus(t, f.call("GET", "/api/v1/b/harbor"+resource.path, "", f.userToken, "", nil), 401)
		}
	}
	for i := 0; i < 3; i++ {
		insertAuditExportFact(t, f.managementHTTP, managedBrand, "pagination.total", "pagination scope test")
	}
	insertAuditExportFact(t, f.managementHTTP, "0199a000-0000-7000-8000-000000000002", "pagination.total", "other brand")
	for _, offset := range []int{0, 1, 99} {
		var page struct {
			Items []json.RawMessage `json:"items"`
			Total string            `json:"total_count"`
		}
		managedData(t, f.call("GET", fmt.Sprintf("/api/v1/admin/audit?action=pagination.total&limit=1&offset=%d", offset), "", f.token, managedBrand, nil), &page)
		if page.Total != "3" || len(page.Items) > 1 {
			t.Fatalf("audit total leaked scope or included paging: %+v", page)
		}
	}
}
