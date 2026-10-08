package platform_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rewards"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestBusinessReconciliation0060To0063PreservesLegacyWalletReceipt(t *testing.T) {
	ctx := context.Background()
	db := testdb.NewAtVersion(t, 60)
	brandID := "0199a000-0000-7000-8000-000000000001"
	globalID, memberID, accountID, adminID := ids.New(), ids.New(), ids.New(), ids.New()
	jobID, targetID, creationAuditID, checkedAuditID := ids.New(), ids.New(), ids.New(), ids.New()
	createdAt := time.Date(2026, 10, 7, 10, 0, 0, 0, time.UTC)
	checkedAt := createdAt.Add(time.Minute)

	if _, err := db.Exec(ctx, `INSERT INTO global_users(id,username) VALUES($1,$2)`, globalID, "business_upgrade_"+globalID[:8]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version) VALUES($1,$2,$3,'operator','v1','v1')`, memberID, brandID, globalID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO point_accounts(id,brand_id,brand_member_id) VALUES($1,$2,$3)`, accountID, brandID, memberID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO point_buckets(brand_id,account_id,source,state,points)
	 SELECT $1,$2,s,st,0 FROM unnest(ARRAY['recharge','winning','gift']::text[]) s
	 CROSS JOIN unnest(ARRAY['available','manual_frozen','system_frozen','withdrawal']::text[]) st
	 ON CONFLICT DO NOTHING`, brandID, accountID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'business-reconciliation-upgrade')`, adminID, "business_upgrade_admin_"+adminID[:8]); err != nil {
		t.Fatal(err)
	}
	actor := access.Account{
		ID: adminID, Type: access.AccountAdmin, BrandIDs: []string{brandID},
		Roles: []access.Role{{BrandID: brandID, Permissions: []access.Permission{
			{Resource: "reward", Action: "view", Scope: access.ScopeBrand},
			{Resource: "reward", Action: "grant", Scope: access.ScopeBrand},
		}}},
	}
	grantTx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	order, err := (rewards.Service{DB: db}).GrantTx(ctx, grantTx, brandID, actor,
		rewards.GrantInput{MemberID: memberID, Points: points.Amount(53), Reason: "pre-upgrade immutable reward fixture"},
		points.Metadata{ActorType: "admin", ActorID: adminID, RequestID: ids.New(), IP: "127.0.0.1"})
	if err != nil {
		_ = grantTx.Rollback(ctx)
		t.Fatalf("create real pre-upgrade reward business and ledger fixture: %v", err)
	}
	if err = grantTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if order.ID == "" {
		t.Fatal("reward service returned an empty pre-upgrade business record ID")
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	reason := "historical wallet audit"
	creationAfter := map[string]any{"target_count": 1, "created_at": createdAt}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(id,brand_id,actor_type,actor_id,action,resource_type,resource_id,after_json,reason,request_id,created_at)
	 VALUES($1,$2,'admin',$3,'wallet.reconciliation.create','wallet_reconciliation_job',$4,$5,$6,$7,$8)`, creationAuditID, brandID, adminID, jobID, creationAfter, reason, ids.New(), createdAt); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO point_reconciliation_jobs(id,brand_id,target_count,created_by,reason,created_at,creation_audit_log_id)
	 VALUES($1,$2,1,$3,$4,$5,$6)`, jobID, brandID, adminID, reason, createdAt, creationAuditID); err != nil {
		t.Fatalf("seed historical job using schema 0060: %v", err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO point_reconciliation_targets(id,brand_id,job_id,account_id,member_id)
	 VALUES($1,$2,$3,$4,$5)`, targetID, brandID, jobID, accountID, memberID); err != nil {
		t.Fatal(err)
	}
	var preview map[string]any
	if err = tx.QueryRow(ctx, `SELECT jsonb_build_object(
	 'account_id',$1::uuid::text,'member_id',$2::uuid::text,
	 'version',(SELECT version FROM point_accounts WHERE id=$1::uuid),'ledger_version',(SELECT count(*) FROM point_ledger_entries WHERE account_id=$1::uuid),
	 'actual',(SELECT coalesce(jsonb_object_agg(source,states),'{}'::jsonb) FROM (
	  SELECT source,jsonb_object_agg(state,points::text) states FROM point_buckets WHERE brand_id=$3::uuid AND account_id=$1::uuid GROUP BY source) b),
	 'expected',(SELECT after_snapshot FROM point_ledger_entries WHERE brand_id=$3::uuid AND account_id=$1::uuid ORDER BY version DESC LIMIT 1),
	 'consistent',false,'repairable',false,'issues',jsonb_build_array('legacy historical observation'),'token',repeat('a',64))`, accountID, memberID, brandID).Scan(&preview); err != nil {
		t.Fatal(err)
	}
	checkedAfter := map[string]any{"job_id": jobID, "outcome": "corrupt", "preview": preview, "checked_at": checkedAt}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(id,brand_id,actor_type,action,resource_type,resource_id,after_json,request_id,created_at)
	 VALUES($1,$2,'system','wallet.reconciliation.checked','wallet_reconciliation_target',$3,$4,$5,$6)`, checkedAuditID, brandID, targetID, checkedAfter, ids.New(), checkedAt); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO point_reconciliation_results(target_id,brand_id,job_id,outcome,preview,checked_at,audit_log_id)
	 VALUES($1,$2,$3,'corrupt',$4,$5,$6)`, targetID, brandID, jobID, preview, checkedAt, checkedAuditID); err != nil {
		t.Fatalf("seed historical checked result using schema 0060: %v", err)
	}
	if _, err = tx.Exec(ctx, `UPDATE point_reconciliation_targets SET state='checked',attempt_count=1 WHERE id=$1`, targetID); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatalf("commit historical wallet receipt: %v", err)
	}

	before := legacyReconciliationSnapshot(t, ctx, db, jobID)
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate schema 0060 through 0063: %v", err)
	}
	if after := legacyReconciliationSnapshot(t, ctx, db, jobID); after != before {
		t.Fatalf("0061-0063 rewrote wallet, reward business, reconciliation, or audit evidence:\nbefore hash %s\nafter  hash %s", before, after)
	}
	var scope string
	var businessPreview any
	if err := db.QueryRow(ctx, `SELECT j.check_scope,r.business_preview FROM point_reconciliation_jobs j
	 JOIN point_reconciliation_results r ON r.brand_id=j.brand_id AND r.job_id=j.id WHERE j.id=$1`, jobID).Scan(&scope, &businessPreview); err != nil {
		t.Fatal(err)
	}
	if scope != "wallet" || businessPreview != nil {
		t.Fatalf("legacy receipt was retagged: job scope=%q result business_preview=%v", scope, businessPreview)
	}
	var auditHasNewFields bool
	if err := db.QueryRow(ctx, `SELECT EXISTS(
	 SELECT 1 FROM audit_logs WHERE id=ANY($1::uuid[]) AND (after_json ? 'check_scope' OR after_json ? 'business_preview'))`, []string{creationAuditID, checkedAuditID}).Scan(&auditHasNewFields); err != nil {
		t.Fatal(err)
	}
	if auditHasNewFields {
		t.Fatal("migration added new scope or business preview fields to immutable legacy audits")
	}
}

func legacyReconciliationSnapshot(t *testing.T, ctx context.Context, db *pgxpool.Pool, jobID string) string {
	t.Helper()
	var raw string
	err := db.QueryRow(ctx, `SELECT jsonb_build_object(
	 'member',to_jsonb(m),
	 'account',to_jsonb(p),
	 'buckets',(SELECT jsonb_agg(to_jsonb(b) ORDER BY b.source,b.state) FROM point_buckets b WHERE b.brand_id=p.brand_id AND b.account_id=p.id),
	 'ledger',(SELECT jsonb_agg(to_jsonb(l) ORDER BY l.version,l.id) FROM point_ledger_entries l WHERE l.brand_id=p.brand_id AND l.account_id=p.id),
	 'reward_orders',(SELECT jsonb_agg(to_jsonb(o) ORDER BY o.id) FROM reward_orders o WHERE o.brand_id=p.brand_id AND o.member_id=p.brand_member_id),
	 'reward_actions',(SELECT jsonb_agg(to_jsonb(a) ORDER BY a.id) FROM reward_order_actions a
	  WHERE a.order_id IN(SELECT o.id FROM reward_orders o WHERE o.brand_id=p.brand_id AND o.member_id=p.brand_member_id)),
	 'job',to_jsonb(j)-'last_step_at'-'creation_xid'-'check_scope',
	 'target',to_jsonb(t)-'next_check_at',
	 'result',to_jsonb(r)-'business_preview',
	 'audits',(SELECT jsonb_agg(to_jsonb(a) ORDER BY a.id) FROM audit_logs a WHERE a.brand_id=p.brand_id))::text
	 FROM point_reconciliation_jobs j JOIN point_reconciliation_targets t ON t.brand_id=j.brand_id AND t.job_id=j.id
	 JOIN point_reconciliation_results r ON r.brand_id=t.brand_id AND r.job_id=t.job_id AND r.target_id=t.id
	 JOIN point_accounts p ON p.brand_id=t.brand_id AND p.id=t.account_id
	 JOIN brand_members m ON m.brand_id=p.brand_id AND m.id=p.brand_member_id WHERE j.id=$1`, jobID).Scan(&raw)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(hash[:])
}
