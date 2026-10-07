package notification

import (
	"context"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

func TestWithdrawalMigrationPreservesExistingCopyHistoryAndInbox(t *testing.T) {
	db := notificationSchemaBefore(t, "0040_")
	ctx := context.Background()
	admin, user, member := ids.New(), ids.New(), ids.New()
	if _, err := db.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'test-only')`, admin, "upgrade_admin_"+admin); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO global_users(id,username) VALUES($1,$2)`, user, "upgrade_member_"+user); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version) VALUES($1,$2,$3,'domain','dev-1','dev-1')`, member, brand, user); err != nil {
		t.Fatal(err)
	}
	s := Service{DB: db}
	a := access.Account{ID: admin, Type: access.AccountAdmin, BrandIDs: []string{brand}, Roles: []access.Role{{BrandID: brand, Permissions: []access.Permission{{Resource: "notification_template", Action: "write", Scope: access.ScopeBrand}}}}}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	content := Content{En: Copy{Title: "Custom welcome", Body: "Preserve customized copy."}, ZhCN: Copy{Title: "定制欢迎", Body: "保留已发布文案。"}}
	if _, err = s.UpdateTemplate(ctx, tx, brand, "member.joined", a, UpdateInput{Version: 1, Content: content, Reason: "customized before upgrade"}, points.Metadata{ActorType: "admin", ActorID: admin, RequestID: ids.New()}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	emit(t, s, brand, member)
	if _, err = s.Process(ctx, 20); err != nil {
		t.Fatal(err)
	}
	fingerprint := `SELECT jsonb_build_object('templates',(SELECT jsonb_agg(to_jsonb(t) ORDER BY brand_id,template_key) FROM notification_templates t WHERE template_key NOT LIKE 'withdrawal.order.%'),'revisions',(SELECT jsonb_agg(to_jsonb(r) ORDER BY brand_id,template_key,version) FROM notification_template_revisions r WHERE template_key NOT LIKE 'withdrawal.order.%'),'inbox',(SELECT jsonb_agg(to_jsonb(n) ORDER BY id) FROM notifications n),'events',(SELECT jsonb_agg(to_jsonb(e) ORDER BY id) FROM outbox_events e),'deliveries',(SELECT jsonb_agg(to_jsonb(d) ORDER BY event_id) FROM notification_deliveries d),'orders',(SELECT count(*) FROM withdrawal_orders),'ledger',(SELECT count(*) FROM point_ledger_entries))::text`
	var before, after string
	if err = db.QueryRow(ctx, fingerprint).Scan(&before); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = database.Migrate(ctx, db); err != nil {
			t.Fatal(err)
		}
	}
	if err = db.QueryRow(ctx, fingerprint).Scan(&after); err != nil || before != after {
		t.Fatal("upgrade rewrote existing facts", err)
	}
	templates, err := s.Templates(ctx, brand)
	if err != nil || len(templates) != 14 {
		t.Fatal(len(templates), err)
	}
	var count int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM notification_template_revisions WHERE brand_id=$1 AND template_key LIKE 'withdrawal.order.%' AND version=1`, brand).Scan(&count); err != nil || count != 6 {
		t.Fatal(count, err)
	}
	newBrand := ids.New()
	if _, err = db.Exec(ctx, `INSERT INTO brands(id,code,name,status) VALUES($1,$2,'New brand','paused')`, newBrand, "new_"+newBrand); err != nil {
		t.Fatal(err)
	}
	templates, err = s.Templates(ctx, newBrand)
	if err != nil || len(templates) != 14 {
		t.Fatal("new brand defaults", len(templates), err)
	}
}
