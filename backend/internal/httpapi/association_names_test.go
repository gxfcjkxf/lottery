package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
)

func TestAssociationNamesOnlyProjectAuthorizedResponseReferences(t *testing.T) {
	id := "11111111-1111-4111-8111-111111111111"
	hidden := "22222222-2222-4222-8222-222222222222"
	ids := collectAssociationIDs(map[string]any{"member_id": id, "before_json": map[string]any{"member_id": hidden}, "payload": map[string]any{"role_ids": []any{hidden}}, "definition": map[string]any{"game_id": hidden}})
	if len(ids) != 1 || len(ids["member"]) != 1 || ids["member"][0] != id {
		t.Fatal(ids)
	}
}

func TestAssociationProjectionQueriesAndExactAccountSelection(t *testing.T) {
	f := pointsFixture(t)
	ctx := context.Background()
	// Exercise every projection query against the real schema. Missing records
	// remain missing; they must not turn into guessed names or business failures.
	r := httptest.NewRequest("GET", "/", nil)
	r = r.WithContext(context.WithValue(r.Context(), sessionIdentityKey{}, Dependencies{Admins: adminsys.Store{DB: f.pool}}))
	data := map[string]any{}
	for field := range associationFields {
		data[field] = f.memberID
	}
	names, failed := responseAssociationNames(r, data)
	if failed || names["member:"+f.memberID].Name != "points_member" {
		t.Fatal("projection queries failed", names, failed)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) SELECT role_id,'agent.write.brand' FROM admin_account_roles WHERE account_id=$1 ON CONFLICT DO NOTHING`, f.root); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id IN (SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key IN ('user.view.brand','role.view.brand')`, f.root); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/admin/association-selection?kind=member&resource=agent&action=write&username=points_member"
	selected := f.call("GET", path, "", f.token, managedBrand, nil)
	mustStatus(t, selected, 200)
	var account struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	}
	managedData(t, selected, &account)
	if account.ID != f.memberID || account.Username != "points_member" {
		t.Fatal(account)
	}
	mustStatus(t, f.call("GET", "/api/v1/admin/users", "", f.token, managedBrand, nil), 403)
	mustStatus(t, f.call("GET", path, "", f.token, pointsBrandB, nil), 403)
	mustStatus(t, f.call("GET", path+"&username=another", "", f.token, managedBrand, nil), 400)
	mustStatus(t, f.call("GET", strings.Replace(path, "points_member", "missing_member", 1), "", f.token, managedBrand, nil), 404)
	if _, err := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id IN (SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key='agent.write.brand'`, f.root); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("GET", path, "", f.token, managedBrand, nil), 403)
}

func TestAssociationNamesAccountResponseKeepsRoleAndBrandScope(t *testing.T) {
	f := managedFixture(t)
	result := f.call("GET", "/api/v1/admin/accounts", "", f.token, managedBrand, nil)
	mustStatus(t, result, 200)
	var response struct {
		Data struct {
			Items []struct {
				ID      string   `json:"id"`
				RoleIDs []string `json:"role_ids"`
			} `json:"items"`
		} `json:"data"`
		Names  map[string]associationName `json:"associations"`
		Failed bool                       `json:"association_error"`
	}
	if err := json.Unmarshal(result.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Failed || len(response.Data.Items) == 0 {
		t.Fatal(result.Body.String())
	}
	for _, account := range response.Data.Items {
		for _, role := range account.RoleIDs {
			if strings.TrimSpace(response.Names["role:"+role].Name) == "" {
				t.Fatal("missing role name", role)
			}
		}
	}
	forbidden := f.call("GET", "/api/v1/admin/accounts", "", f.token, "0199a000-0000-7000-8000-000000000002", nil)
	if forbidden.Code == 200 || strings.Contains(forbidden.Body.String(), `"associations"`) {
		t.Fatal("foreign scope returned names", forbidden.Body.String())
	}
}

func TestRewardMemberSelectionUsesOnlyExistingGrantAndBrandScope(t *testing.T) {
	f := pointsFixture(t)
	path := "/api/v1/admin/association-selection?kind=member&resource=reward&action=grant&username=points_member"
	mustStatus(t, f.call("GET", path, "", f.token, managedBrand, nil), 403)
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO role_permissions(role_id,permission_key) SELECT role_id,'reward.grant.brand' FROM admin_account_roles WHERE account_id=$1 ON CONFLICT DO NOTHING`, f.root); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(context.Background(), `DELETE FROM role_permissions WHERE role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key='user.view.brand'`, f.root); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("GET", path, "", f.token, managedBrand, nil), 200)
	mustStatus(t, f.call("GET", "/api/v1/admin/users", "", f.token, managedBrand, nil), 403)
	mustStatus(t, f.call("GET", path, "", f.token, pointsBrandB, nil), 403)
	mustStatus(t, f.call("GET", strings.Replace(path, "kind=member", "kind=agent", 1), "", f.token, managedBrand, nil), 400)
	mustStatus(t, f.call("GET", strings.Replace(path, "points_member", "missing_member", 1), "", f.token, managedBrand, nil), 404)
	if _, err := f.pool.Exec(context.Background(), `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("GET", path, "", f.token, managedBrand, nil), 403)
}
func TestExceptionMarkerProjectsOnlyItsAuthorizedAdminReference(t *testing.T) {
	id := "11111111-1111-4111-8111-111111111111"
	ids := collectAssociationIDs(map[string]any{"marked_by": id, "before_json": map[string]any{"marked_by": "22222222-2222-4222-8222-222222222222"}})
	if len(ids) != 1 || len(ids["admin"]) != 1 || ids["admin"][0] != id {
		t.Fatal(ids)
	}
}
