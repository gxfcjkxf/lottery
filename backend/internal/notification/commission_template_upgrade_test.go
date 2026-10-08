package notification

import (
	"context"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
)

func TestCommissionDefaultsReplaceExistingFunctionOIDAndInitializeNewBrands(t *testing.T) {
	db := notificationSchemaBefore(t, "0052_")
	ctx := context.Background()
	preBrand := ids.New()
	if _, err := db.Exec(ctx, `INSERT INTO brands(id,code,name,status) VALUES($1,$2,'Pre-0052 brand','paused')`, preBrand, "pre_"+preBrand[:8]); err != nil {
		t.Fatal(err)
	}
	var defaultsBefore, priorRowsBefore, functionOIDBefore string
	if err := db.QueryRow(ctx, `SELECT (notification_template_defaults()-'commission.paid'-'commission.adjusted')::text`).Scan(&defaultsBefore); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT jsonb_build_object(
 'templates',(SELECT jsonb_agg(to_jsonb(t) ORDER BY brand_id,template_key) FROM notification_templates t WHERE brand_id=ANY($1::uuid[])),
 'revisions',(SELECT jsonb_agg(to_jsonb(r) ORDER BY brand_id,template_key,version) FROM notification_template_revisions r WHERE brand_id=ANY($1::uuid[])))::text`, []string{brand, preBrand}).Scan(&priorRowsBefore); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT to_regprocedure('notification_template_defaults()')::oid::text`).Scan(&functionOIDBefore); err != nil {
		t.Fatal(err)
	}
	var oldTemplateCount int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM notification_templates WHERE brand_id=$1`, preBrand).Scan(&oldTemplateCount); err != nil || oldTemplateCount != 14 {
		t.Fatalf("brand created before 0052 started with %d templates, err=%v", oldTemplateCount, err)
	}

	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	var defaultsAfter, priorRowsAfter, functionOIDAfter string
	if err := db.QueryRow(ctx, `SELECT (notification_template_defaults()-'commission.paid'-'commission.adjusted'-'reward.order.granted'-'reward.order.revocation_pending'-'reward.order.revoked')::text`).Scan(&defaultsAfter); err != nil || defaultsAfter != defaultsBefore {
		t.Fatalf("0052 changed one of the previous fourteen defaults: before=%s after=%s err=%v", defaultsBefore, defaultsAfter, err)
	}
	if err := db.QueryRow(ctx, `SELECT jsonb_build_object(
	'templates',(SELECT jsonb_agg(to_jsonb(t) ORDER BY brand_id,template_key) FROM notification_templates t WHERE brand_id=ANY($1::uuid[]) AND template_key NOT LIKE 'commission.%' AND template_key NOT LIKE 'reward.order.%'),
	 'revisions',(SELECT jsonb_agg(to_jsonb(r) ORDER BY brand_id,template_key,version) FROM notification_template_revisions r WHERE brand_id=ANY($1::uuid[]) AND template_key NOT LIKE 'commission.%' AND template_key NOT LIKE 'reward.order.%'))::text`, []string{brand, preBrand}).Scan(&priorRowsAfter); err != nil || priorRowsAfter != priorRowsBefore {
		t.Fatalf("0052 rewrote existing template content or revision history: err=%v", err)
	}
	if err := db.QueryRow(ctx, `SELECT to_regprocedure('notification_template_defaults()')::oid::text`).Scan(&functionOIDAfter); err != nil || functionOIDAfter != functionOIDBefore {
		t.Fatalf("default function OID changed across replacement: before=%s after=%s err=%v", functionOIDBefore, functionOIDAfter, err)
	}
	for _, id := range []string{brand, preBrand} {
		var count, revisions int
		if err := db.QueryRow(ctx, `SELECT count(*) FROM notification_templates WHERE brand_id=$1`, id).Scan(&count); err != nil || count != 19 {
			t.Fatalf("existing brand %s templates=%d, err=%v", id, count, err)
		}
		if err := db.QueryRow(ctx, `SELECT count(*) FROM notification_template_revisions WHERE brand_id=$1 AND template_key LIKE 'commission.%' AND version=1`, id).Scan(&revisions); err != nil || revisions != 2 {
			t.Fatalf("existing brand %s commission defaults revisions=%d, err=%v", id, revisions, err)
		}
	}

	postBrand := ids.New()
	if _, err := db.Exec(ctx, `INSERT INTO brands(id,code,name,status) VALUES($1,$2,'Post-0052 brand','paused')`, postBrand, "post_"+postBrand[:8]); err != nil {
		t.Fatal(err)
	}
	var templates, revisions int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM notification_templates WHERE brand_id=$1`, postBrand).Scan(&templates); err != nil || templates != 19 {
		t.Fatalf("new brand initialized with %d templates, err=%v", templates, err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM notification_template_revisions WHERE brand_id=$1 AND version=1`, postBrand).Scan(&revisions); err != nil || revisions != 19 {
		t.Fatalf("new brand initialized with %d default revisions, err=%v", revisions, err)
	}
}
