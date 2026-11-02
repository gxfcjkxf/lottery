package adminsys

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestBrandDomainPermissionsAreNotAssignable(t *testing.T) {
	for _, key := range []string{"brand_domains.view.brand", "brand_domains.write.brand"} {
		if _, err := cleanPermissions([]string{key}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("obsolete brand grant accepted: %s", key)
		}
	}
}

func TestBrandDomainPermissionsAbsentFromRoleEditor(t *testing.T) {
	f := managementFixtureFor(t, "role.view.brand")
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,'brand_domains.write.brand')`, f.roleID); err != nil {
		t.Fatal(err)
	}
	catalog, err := f.store.ListPermissions(ctx, f.actor, managementBrandA)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range catalog {
		if strings.HasPrefix(key, "brand_domains.") {
			t.Fatal("obsolete catalog permission", key)
		}
	}
	roles, err := f.store.ListRoles(ctx, f.actor, managementBrandA, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range roles {
		for _, key := range role.Permissions {
			if strings.HasPrefix(key, "brand_domains.") {
				t.Fatal("obsolete role permission", key)
			}
		}
	}
}
