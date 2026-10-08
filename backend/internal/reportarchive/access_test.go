package reportarchive

import (
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
)

const accessTestBrand = "0199a000-0000-7000-8000-000000000001"

func TestAllowedRequiresIndependentScopedArchivePermissions(t *testing.T) {
	brandGrant := func(action string) access.Role {
		return access.Role{BrandID: accessTestBrand, Permissions: []access.Permission{{
			Resource: "report_archive", Action: action, Scope: access.ScopeBrand,
		}}}
	}
	platformGrant := func(action string) access.Role {
		return access.Role{Permissions: []access.Permission{{
			Resource: "report_archive", Action: action, Scope: access.ScopePlatform,
		}}}
	}
	tests := []struct {
		name    string
		account access.Account
		brand   string
		action  string
		want    bool
	}{
		{"brand view", access.Account{Type: access.AccountAdmin, BrandIDs: []string{accessTestBrand}, Roles: []access.Role{brandGrant("view")}}, accessTestBrand, "view", true},
		{"brand create with view", access.Account{Type: access.AccountAdmin, BrandIDs: []string{accessTestBrand}, Roles: []access.Role{brandGrant("view"), brandGrant("create")}}, accessTestBrand, "create", true},
		{"create without view", access.Account{Type: access.AccountAdmin, BrandIDs: []string{accessTestBrand}, Roles: []access.Role{brandGrant("create")}}, accessTestBrand, "create", false},
		{"super admin cannot create", access.Account{Type: access.AccountAdmin, SuperAdmin: true, BrandIDs: []string{accessTestBrand}, Roles: []access.Role{brandGrant("view"), brandGrant("create")}}, accessTestBrand, "create", false},
		{"download without view", access.Account{Type: access.AccountAdmin, BrandIDs: []string{accessTestBrand}, Roles: []access.Role{brandGrant("download")}}, accessTestBrand, "download", false},
		{"view without download", access.Account{Type: access.AccountAdmin, BrandIDs: []string{accessTestBrand}, Roles: []access.Role{brandGrant("view")}}, accessTestBrand, "download", false},
		{"independent brand download", access.Account{Type: access.AccountAdmin, BrandIDs: []string{accessTestBrand}, Roles: []access.Role{brandGrant("view"), brandGrant("download")}}, accessTestBrand, "download", true},
		{"platform view and download", access.Account{Type: access.AccountAdmin, Roles: []access.Role{platformGrant("view"), platformGrant("download")}}, accessTestBrand, "download", true},
		{"brand view with platform download", access.Account{Type: access.AccountAdmin, BrandIDs: []string{accessTestBrand}, Roles: []access.Role{brandGrant("view"), platformGrant("download")}}, accessTestBrand, "download", true},
		{"platform view with brand download", access.Account{Type: access.AccountAdmin, BrandIDs: []string{accessTestBrand}, Roles: []access.Role{platformGrant("view"), brandGrant("download")}}, accessTestBrand, "download", true},
		{"other brand scope denied", access.Account{Type: access.AccountAdmin, BrandIDs: []string{accessTestBrand}, Roles: []access.Role{brandGrant("view")}}, "0199a000-0000-7000-8000-000000000002", "view", false},
		{"invalid brand denied", access.Account{Type: access.AccountAdmin, Roles: []access.Role{platformGrant("view")}}, "invalid", "view", false},
		{"unknown action denied", access.Account{Type: access.AccountAdmin, BrandIDs: []string{accessTestBrand}, Roles: []access.Role{brandGrant("view")}}, accessTestBrand, "export", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Allowed(test.account, test.brand, test.action); got != test.want {
				t.Fatalf("Allowed(%q)=%v, want %v", test.action, got, test.want)
			}
		})
	}
}
