package database_test

import (
	"context"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
)

func TestAuditExportUpgradeOnlyBootstrapGrantsNoFinancialChanges(t *testing.T) {
	db := testdb.NewAtVersion(t, 70)
	ctx := context.Background()
	brand := "0199a000-0000-7000-8000-000000000001"
	bootstrap, custom, platform := ids.New(), ids.New(), ids.New()
	if _, e := db.Exec(ctx, `INSERT INTO permissions(key) VALUES('audit.view.brand'),('audit.view.platform') ON CONFLICT DO NOTHING`); e != nil {
		t.Fatal(e)
	}
	for _, r := range []struct {
		id, brand string
		bootstrap bool
	}{{bootstrap, brand, true}, {custom, brand, false}, {platform, "", true}} {
		if _, e := db.Exec(ctx, `INSERT INTO roles(id,brand_id,code,name,is_bootstrap) VALUES($1::uuid,NULLIF($2,'')::uuid,$1::text,'audit upgrade fixture',$3)`, r.id, r.brand, r.bootstrap); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := db.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,'audit.view.brand'),($2,'audit.view.brand'),($3,'audit.view.platform')`, bootstrap, custom, platform); e != nil {
		t.Fatal(e)
	}
	snapshot := func() string {
		var s string
		if e := db.QueryRow(ctx, `SELECT jsonb_build_object('ledger',(SELECT coalesce(jsonb_agg(to_jsonb(a) ORDER BY id),'[]') FROM point_ledger_entries a),'accounts',(SELECT coalesce(jsonb_agg(to_jsonb(a) ORDER BY id),'[]') FROM point_accounts a),'audit',(SELECT coalesce(jsonb_agg(to_jsonb(a) ORDER BY id),'[]') FROM audit_logs a),'templates',(SELECT coalesce(jsonb_agg(to_jsonb(a) ORDER BY brand_id,template_key),'[]') FROM notification_templates a))::text`).Scan(&s); e != nil {
			t.Fatal(e)
		}
		return s
	}
	before := snapshot()
	if e := database.Migrate(ctx, db); e != nil {
		t.Fatal(e)
	}
	if after := snapshot(); before != after {
		t.Fatal("export upgrade changed existing financial/audit/template facts")
	}
	var grants int
	if e := db.QueryRow(ctx, `SELECT count(*) FROM role_permissions WHERE (role_id=$1 AND permission_key='audit.export.brand') OR(role_id=$2 AND permission_key='audit.export.platform')`, bootstrap, platform).Scan(&grants); e != nil || grants != 2 {
		t.Fatal(grants, e)
	}
	if e := db.QueryRow(ctx, `SELECT count(*) FROM role_permissions WHERE role_id=$1`, custom).Scan(&grants); e != nil || grants != 1 {
		t.Fatal("custom role silently expanded", grants, e)
	}
	if e := database.Migrate(ctx, db); e != nil {
		t.Fatal(e)
	}
}
