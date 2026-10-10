package adminsys

import (
	"context"
	"errors"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
)

func TestBrandAdministratorRoleIncludesRegisteredDirectory(t *testing.T) {
	f := managementFixtureFor(t, "role.view.platform", "role.write.platform", "admin.write.platform")
	ctx := context.Background()
	keys, err := f.store.ListPermissions(ctx, f.actor, managementBrandA)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) <= 100 {
		t.Fatalf("current registered directory should reproduce the old 100-permission limit: %d", len(keys))
	}
	tx := beginManagementTx(t, f.pool)
	role, err := f.store.WriteRole(ctx, tx, f.actor, managementBrandA, "", RoleInput{Code: "full_brand_admin", Name: "Brand administrator", Status: "active", Permissions: keys, Reason: "Create administrator from registered directory"}, identity.Metadata{RequestID: "full-brand-role"})
	if err != nil {
		t.Fatal(err)
	}
	if len(role.Permissions) != len(keys) {
		t.Fatalf("permission directory truncated: %d/%d", len(role.Permissions), len(keys))
	}
	account, err := f.store.CreateAdmin(ctx, tx, f.actor, managementBrandA, AdminInput{Username: "full_brand_admin", Status: "active", RoleIDs: []string{role.ID}, Reason: "Assign complete brand administrator role"}, "test-only-hash", identity.Metadata{RequestID: "full-brand-account"})
	if err != nil {
		t.Fatal(err)
	}
	commitManagement(t, tx)
	actual, err := f.store.Account(ctx, account.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range keys {
		resource, action, _, valid := parsePermission(key)
		if !valid || !access.Authorize(actual, resource, action, access.ScopeBrand, managementBrandA) {
			t.Fatalf("assigned account lacks %s", key)
		}
	}
	for _, key := range []string{"not_registered.view.brand", "admin.write.platform"} {
		invalidTx := beginManagementTx(t, f.pool)
		_, err := f.store.WriteRole(ctx, invalidTx, f.actor, managementBrandA, "", RoleInput{Code: "invalid_catalog_role", Name: "Invalid catalog", Status: "active", Permissions: append(append([]string{}, keys...), key), Reason: "Reject unknown or platform permission"}, identity.Metadata{RequestID: "invalid-brand-directory"})
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid permission %s passed: %v", key, err)
		}
		_ = invalidTx.Rollback(ctx)
	}
}
