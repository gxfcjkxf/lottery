package workbench

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rewards"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
)

func TestRewardWorkbenchHasIndependentViewAndDoesNotReadForbiddenOrders(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	out, err := snapshot(t, db, actor("reward"), brandA)
	if err != nil || out.Rewards.Status != "ready" || out.Rewards.Data == nil {
		t.Fatalf("reward only: %+v err=%v", out.Rewards, err)
	}
	raw, _ := json.Marshal(out.Rewards.Data)
	if string(raw) != `{"granted_count":"0","pending_count":"0","revoked_count":"0"}` {
		t.Fatalf("incorrect actual zero summary: %s", raw)
	}
	if out.Brand.Status != "forbidden" || out.Ledger.Status != "forbidden" {
		t.Fatal("reward view leaked another section")
	}
	platform := access.Account{Type: access.AccountAdmin, SuperAdmin: true, Roles: []access.Role{{Permissions: []access.Permission{{Resource: "reward", Action: "view", Scope: access.ScopePlatform}}}}}
	out, err = snapshot(t, db, platform, brandB)
	if err != nil || out.Rewards.Status != "ready" || out.Rewards.Data == nil {
		t.Fatalf("platform reward view: %+v %v", out.Rewards, err)
	}
	if _, err = db.Exec(ctx, `ALTER TABLE reward_orders RENAME TO hidden_reward_orders`); err != nil {
		t.Fatal(err)
	}
	out, err = snapshot(t, db, actor("brand"), brandA)
	if err != nil || out.Rewards.Status != "forbidden" || out.Rewards.Data != nil {
		t.Fatalf("forbidden query touched rewards: %+v %v", out.Rewards, err)
	}
	if _, err = snapshot(t, db, actor("reward"), brandA); err == nil {
		t.Fatal("authorized missing table masqueraded as zero")
	}
}

func TestRewardWorkbenchCountsRealCurrentOrdersWithoutChangingFinancialFacts(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	a := actor("reward")
	a.Roles[0].Permissions = append(a.Roles[0].Permissions, access.Permission{Resource: "reward", Action: "grant", Scope: access.ScopeBrand}, access.Permission{Resource: "reward", Action: "revoke", Scope: access.ScopeBrand})
	user, member, account := ids.New(), ids.New(), ids.New()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'test-only')`, []any{a.ID, "reward_workbench_" + a.ID}},
		{`INSERT INTO global_users(id,username,password_hash) VALUES($1,$2,'test-only')`, []any{user, "reward_workbench_member_" + user}},
		{`INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version) VALUES($1,$2,$3,'operator','1','1')`, []any{member, brandA, user}},
		{`INSERT INTO point_accounts(id,brand_id,brand_member_id) VALUES($1,$2,$3)`, []any{account, brandA, member}},
		{`INSERT INTO point_buckets(brand_id,account_id,source,state) SELECT $1,$2,s,t FROM unnest(ARRAY['recharge','winning','gift']) s CROSS JOIN unnest(ARRAY['available','manual_frozen','system_frozen','withdrawal']) t`, []any{brandA, account}},
	} {
		if _, err := db.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	service := rewards.Service{DB: db}
	meta := func() points.Metadata {
		return points.Metadata{ActorType: "admin", ActorID: a.ID, RequestID: ids.New()}
	}
	grant := func(amount points.Amount) rewards.Order {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		o, err := service.GrantTx(ctx, tx, brandA, a, rewards.GrantInput{MemberID: member, Points: amount, Reason: "workbench actual grant"}, meta())
		if err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		return o
	}
	grant(10)
	o := grant(20)
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.RevokeTx(ctx, tx, brandA, o.ID, a, rewards.ActionInput{Version: 1, Reason: "workbench actual reversal"}, meta()); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	pending := grant(30)
	tx, err = db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	allocation := []points.Allocation{{Source: "gift", State: "available", Points: 40}}
	delta, err := points.AllocationDelta(allocation, "available", "manual_frozen")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = (points.Store{DB: db}).Post(ctx, tx, points.Change{BrandID: brandA, MemberID: member, EntryType: "freeze", ReferenceType: "manual", OperationKey: "workbench-reward-hold", Reason: "hold original gift", ActorType: "admin", ActorID: a.ID, RequestID: ids.New(), Delta: delta, Allocation: allocation}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	tx, err = db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	p, err := service.RevokeTx(ctx, tx, brandA, pending.ID, a, rewards.ActionInput{Version: 1, Reason: "workbench pending reversal"}, meta())
	if err != nil || p.State != "revocation_pending" {
		t.Fatal(p, err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	fingerprint := func() string {
		var value string
		if err := db.QueryRow(ctx, `SELECT jsonb_build_object('wallet',(SELECT jsonb_agg(to_jsonb(b) ORDER BY source,state) FROM point_buckets b),'ledger',(SELECT jsonb_agg(to_jsonb(l) ORDER BY id) FROM point_ledger_entries l),'orders',(SELECT jsonb_agg(to_jsonb(o) ORDER BY id) FROM reward_orders o),'actions',(SELECT jsonb_agg(to_jsonb(a) ORDER BY id) FROM reward_order_actions a),'events',(SELECT jsonb_agg(to_jsonb(e) ORDER BY id) FROM outbox_events e),'audit',(SELECT jsonb_agg(to_jsonb(a) ORDER BY id) FROM audit_logs a))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	before := fingerprint()
	out, err := snapshot(t, db, a, brandA)
	if err != nil || out.Rewards.Status != "ready" {
		t.Fatal(out.Rewards, err)
	}
	raw, _ := json.Marshal(out.Rewards.Data)
	if string(raw) != `{"granted_count":"1","pending_count":"1","revoked_count":"1"}` {
		t.Fatalf("incorrect current counts: %s", raw)
	}
	if fingerprint() != before {
		t.Fatal("workbench read changed reward financial facts")
	}
	a.BrandIDs = append(a.BrandIDs, brandB)
	if _, err = snapshot(t, db, a, brandB); err == nil {
		t.Fatal("brand-specific reward view leaked to other brand")
	}
}
