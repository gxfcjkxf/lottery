package platform_test

import (
	"context"
	"reflect"
	"sort"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
)

func TestAttribution0067To0068GrantsOnlyBootstrapReportsAndRetainsHistory(t *testing.T) {
	ctx := context.Background()
	db := testdb.NewAtVersion(t, 67)
	const brand = "0199a000-0000-7000-8000-000000000001"
	type roleCase struct {
		id, brand, old string
		bootstrap      bool
		added          []string
		audit          string
	}
	roles := []roleCase{
		{ids.New(), brand, "report_commission.view.brand", true, []string{"report_attribution.export.brand", "report_attribution.view.brand"}, "audit.export.brand"},
		{ids.New(), brand, "report_commission.view.brand", false, nil, ""},
		{ids.New(), "", "report_commission.view.platform", true, []string{"report_attribution.export.platform", "report_attribution.view.platform"}, "audit.export.platform"},
		{ids.New(), "", "report_commission.view.platform", false, nil, ""},
	}
	for _, role := range roles {
		if _, err := db.Exec(ctx, `INSERT INTO roles(id,brand_id,code,name,is_bootstrap) VALUES($1,NULLIF($2,'')::uuid,$3,$3,$4)`, role.id, role.brand, "attribution_upgrade_"+role.id, role.bootstrap); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,$2)`, role.id, role.old); err != nil {
			t.Fatal(err)
		}
	}
	actor := ids.New()
	if _, err := db.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'owned-upgrade-fixture')`, actor, "attribution_actor_"+actor); err != nil {
		t.Fatal(err)
	}
	if err := reportArchivePostLedgerFixture(t, ctx, db, brand); err != nil {
		t.Fatal(err)
	}
	if err := reportArchiveCreateFixture(t, ctx, db, brand, actor); err != nil {
		t.Fatal(err)
	}
	money, archive := reportArchiveMoneySnapshot(t, ctx, db), reportArchiveArchiveSnapshot(t, ctx, db)
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if reportArchiveMoneySnapshot(t, ctx, db) != money || reportArchiveArchiveSnapshot(t, ctx, db) != archive {
		t.Fatal("read-only reporting migration rewrote financial or archive history")
	}
	for _, role := range roles {
		var grants []string
		if err := db.QueryRow(ctx, `SELECT array_agg(permission_key ORDER BY permission_key) FROM role_permissions WHERE role_id=$1`, role.id).Scan(&grants); err != nil {
			t.Fatal(err)
		}
		want := append([]string{role.old}, role.added...)
		if role.audit != "" {
			want = append(want, role.audit)
		}
		sort.Strings(want)
		if !reflect.DeepEqual(grants, want) {
			t.Fatalf("unexpected role expansion: got=%v want=%v", grants, want)
		}
	}
	var registeredAuditPermissions, invalidAuditGrants int
	if err := db.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM permissions WHERE key IN('audit.export.brand','audit.export.platform')),
 (SELECT count(*) FROM role_permissions rp JOIN roles r ON r.id=rp.role_id
  WHERE (rp.permission_key='audit.export.brand' AND (NOT r.is_bootstrap OR r.brand_id IS NULL))
     OR (rp.permission_key='audit.export.platform' AND (NOT r.is_bootstrap OR r.brand_id IS NOT NULL)))`).
		Scan(&registeredAuditPermissions, &invalidAuditGrants); err != nil {
		t.Fatal("audit export role grants:", err)
	}
	if registeredAuditPermissions != 2 || invalidAuditGrants != 0 {
		t.Fatalf("registered audit export permissions=%d invalid_scope_or_custom_grants=%d", registeredAuditPermissions, invalidAuditGrants)
	}
	permissions := reportArchivePermissionState(t, ctx, db)
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(permissions, reportArchivePermissionState(t, ctx, db)) || reportArchiveMoneySnapshot(t, ctx, db) != money || reportArchiveArchiveSnapshot(t, ctx, db) != archive {
		t.Fatal("repeat migration changed grants or retained financial/archive data")
	}
}
