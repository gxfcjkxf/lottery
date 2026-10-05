package access

import "testing"

func TestBrandRoleCannotLeakToOtherMembershipScopes(t *testing.T) {
	a := Account{Type: AccountAdmin, BrandIDs: []string{"a", "b"}, Roles: []Role{
		{BrandID: "a", Permissions: []Permission{{"user", "write", ScopeBrand}}},
		{BrandID: "b", Permissions: []Permission{{"user", "view", ScopeBrand}}},
	}}
	if !Authorize(a, "user", "write", ScopeBrand, "a") || Authorize(a, "user", "write", ScopeBrand, "b") {
		t.Fatal("brand-specific role escaped its ownership scope")
	}
	if !Authorize(a, "user", "view", ScopeBrand, "b") || Authorize(a, "user", "view", ScopeBrand, "a") {
		t.Fatal("read-only role leaked to another brand")
	}
	if len(UnionPermissions(a.Roles...)) != 2 {
		t.Fatal("permission union lost a distinct grant")
	}
}
