package access

import "testing"

func TestUnionPermissionsDeduplicatesAndKeepsScopesDistinct(t *testing.T) {
	viewBrand := Permission{Resource: "withdrawal", Action: "view", Scope: ScopeBrand}
	viewPlatform := Permission{Resource: "withdrawal", Action: "view", Scope: ScopePlatform}
	got := UnionPermissions(
		Role{Permissions: []Permission{viewBrand, viewPlatform}},
		Role{Permissions: []Permission{viewBrand}},
	)
	if len(got) != 2 {
		t.Fatalf("got %d permissions, want 2: %#v", len(got), got)
	}
	if got[0] != viewBrand || got[1] != viewPlatform {
		t.Fatalf("permissions are not distinct and deterministically sorted: %#v", got)
	}
}

func TestAuthorizeRequiresExactPermissionAndBrandMembership(t *testing.T) {
	account := Account{
		Type: AccountAdmin,
		Roles: []Role{{Permissions: []Permission{
			{Resource: "withdrawal", Action: "view", Scope: ScopeBrand},
			{Resource: "withdrawal", Action: "view", Scope: ScopePlatform},
			{Resource: "*", Action: "approve", Scope: ScopeBrand},
		}}},
		BrandIDs: []string{"brand-a"},
	}
	tests := []struct {
		name     string
		resource string
		action   string
		scope    Scope
		brandID  string
		want     bool
	}{
		{"own brand", "withdrawal", "view", ScopeBrand, "brand-a", true},
		{"cross brand", "withdrawal", "view", ScopeBrand, "brand-b", false},
		{"missing brand", "withdrawal", "view", ScopeBrand, "", false},
		{"platform grant exact", "withdrawal", "view", ScopePlatform, "", true},
		{"brand does not become platform", "*", "approve", ScopePlatform, "", false},
		{"platform does not become brand", "withdrawal", "view", ScopeBrand, "brand-a", true},
		{"no wildcard expansion", "withdrawal", "approve", ScopeBrand, "brand-a", false},
		{"platform view does not imply write", "withdrawal", "write", ScopePlatform, "", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Authorize(account, test.resource, test.action, test.scope, test.brandID); got != test.want {
				t.Fatalf("Authorize()=%v, want %v", got, test.want)
			}
		})
	}
}

func TestUserAndSuperAdminRestrictions(t *testing.T) {
	grants := []Role{{Permissions: []Permission{
		{Resource: "user", Action: "view", Scope: ScopePlatform},
		{Resource: "user", Action: "write", Scope: ScopePlatform},
		{Resource: "user", Action: "kick", Scope: ScopePlatform},
	}}}
	user := Account{Type: AccountUser, Roles: grants}
	if Authorize(user, "user", "view", ScopePlatform, "") {
		t.Fatal("user account received administrative authorization")
	}
	superAdmin := Account{Type: AccountAdmin, SuperAdmin: true, Roles: grants}
	if !Authorize(superAdmin, "user", "view", ScopePlatform, "") {
		t.Fatal("super administrator cannot view users")
	}
	for _, action := range []string{"write", "kick", "delete"} {
		if Authorize(superAdmin, "user", action, ScopePlatform, "") {
			t.Errorf("super administrator was authorized for user %q", action)
		}
	}
}
