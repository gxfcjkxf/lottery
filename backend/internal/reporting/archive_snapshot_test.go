package reporting

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/finance"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rewards"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCaptureArchiveUsesOneReadOnlyAggregateSnapshot(t *testing.T) {
	db := archiveTestDB(t)
	ctx := context.Background()
	memberID, userID, accountID, adminID := ids.New(), ids.New(), ids.New(), ids.New()
	for _, item := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO global_users(id,username,password_hash) VALUES($1,$2,'archive-snapshot-test')`, []any{userID, "archive_user_" + userID[:8]}},
		{`INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version) VALUES($1,$2,$3,'operator','1','1')`, []any{memberID, testBrand, userID}},
		{`INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'archive-snapshot-test')`, []any{adminID, "archive_admin_" + adminID[:8]}},
		{`INSERT INTO point_accounts(id,brand_id,brand_member_id) VALUES($1,$2,$3)`, []any{accountID, testBrand, memberID}},
		{`INSERT INTO point_buckets(brand_id,account_id,source,state) SELECT $1,$2,s,t FROM unnest(ARRAY['recharge','winning','gift','commission']) s CROSS JOIN unnest(ARRAY['available','manual_frozen','system_frozen','withdrawal']) t ON CONFLICT DO NOTHING`, []any{testBrand, accountID}},
	} {
		if _, err := db.Exec(ctx, item.sql, item.args...); err != nil {
			t.Fatal(err)
		}
	}

	meta := points.Metadata{ActorType: "admin", ActorID: adminID, RequestID: ids.New(), IP: "127.0.0.1"}
	financeService := finance.Service{DB: db, Points: points.Store{DB: db}}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rechargeOrder, err := financeService.CreateRecharge(ctx, tx, testBrand, memberID, 19, "archive fixture proof", "private note", "archive fixture", meta)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if _, err = financeService.ConfirmRecharge(ctx, tx, testBrand, rechargeOrder.ID, rechargeOrder.Version, "archive fixture confirmed", meta); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	grant, revoke, view := access.Permission{Resource: "reward", Action: "grant", Scope: access.ScopeBrand}, access.Permission{Resource: "reward", Action: "revoke", Scope: access.ScopeBrand}, access.Permission{Resource: "reward", Action: "view", Scope: access.ScopeBrand}
	actor := access.Account{ID: adminID, Type: access.AccountAdmin, BrandIDs: []string{testBrand}, Roles: []access.Role{{BrandID: testBrand, Permissions: []access.Permission{grant, revoke, view}}}}
	rewardService := rewards.Service{DB: db}
	tx, err = db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rewardMeta := meta
	rewardMeta.RequestID = ids.New()
	rewardOrder, err := rewardService.GrantTx(ctx, tx, testBrand, actor, rewards.GrantInput{MemberID: memberID, Points: 23, Reason: "archive fixture grant"}, rewardMeta)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	tx, err = db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rewardMeta.RequestID = ids.New()
	if _, err = rewardService.RevokeTx(ctx, tx, testBrand, rewardOrder.ID, actor, rewards.ActionInput{Version: 1, Reason: "archive fixture revoke"}, rewardMeta); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// This is a real manual points posting through the canonical ledger writer.
	var manualDelta points.Balance
	manualDelta[0][0] = 7
	tx, err = db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = (points.Store{DB: db}).Post(ctx, tx, points.Change{BrandID: testBrand, MemberID: memberID, EntryType: "adjustment", ReferenceType: "manual", OperationKey: "archive-manual:" + ids.New(), Reason: "archive fixture manual credit", ActorType: "admin", ActorID: adminID, RequestID: ids.New(), Delta: manualDelta, Allocation: []points.Allocation{{Source: "recharge", State: "available", Points: 7}}}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	// Two individually valid account postings prove the snapshot retains
	// numeric totals beyond int64 rather than narrowing through Go integers.
	for i := 0; i < 2; i++ {
		largeUser, largeMember, largeAccount := ids.New(), ids.New(), ids.New()
		if _, err = db.Exec(ctx, `INSERT INTO global_users(id,username) VALUES($1,$2)`, largeUser, "archive_large_"+largeUser); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(ctx, `INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version) VALUES($1,$2,$3,'operator','1','1')`, largeMember, testBrand, largeUser); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(ctx, `INSERT INTO point_accounts(id,brand_id,brand_member_id) VALUES($1,$2,$3)`, largeAccount, testBrand, largeMember); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(ctx, `INSERT INTO point_buckets(brand_id,account_id,source,state) SELECT $1,$2,s,t FROM unnest(ARRAY['recharge','winning','gift','commission']) s CROSS JOIN unnest(ARRAY['available','manual_frozen','system_frozen','withdrawal']) t ON CONFLICT DO NOTHING`, testBrand, largeAccount); err != nil {
			t.Fatal(err)
		}
		var largeDelta points.Balance
		largeDelta[0][0] = points.Amount(9223372036854775807)
		tx, err = db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		_, err = (points.Store{DB: db}).Post(ctx, tx, points.Change{BrandID: testBrand, MemberID: largeMember, EntryType: "adjustment", ReferenceType: "manual", OperationKey: "archive-large:" + largeMember, Reason: "large aggregate fixture", ActorType: "system", RequestID: ids.New(), Delta: largeDelta, Allocation: []points.Allocation{{Source: "recharge", State: "available", Points: largeDelta[0][0]}}})
		if err != nil {
			_ = tx.Rollback(ctx)
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}

	from, to := time.Now().UTC().Add(-time.Hour), time.Now().UTC().Add(time.Hour)
	q := Query{From: from, To: to, GroupBy: "day", Limit: 20}
	rq := RewardQuery{From: from, To: to, GroupBy: "day", Limit: 20}
	cq := CommissionQuery{From: from, To: to, GroupBy: "day", Limit: 20}
	s := Service{DB: db}
	ledgerReport, err := s.Ledger(ctx, testBrand, Query{From: from, To: to, GroupBy: "entry_type", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	withdrawalReport, err := s.Withdrawal(ctx, testBrand, q)
	if err != nil {
		t.Fatal(err)
	}
	commissionReport, err := s.Commission(ctx, testBrand, cq)
	if err != nil {
		t.Fatal(err)
	}
	rewardReport, err := s.Reward(ctx, testBrand, rq)
	if err != nil {
		t.Fatal(err)
	}
	rewardOrderReport, err := s.RewardOrders(ctx, testBrand, rq)
	if err != nil {
		t.Fatal(err)
	}
	// Betting is intentionally empty in this fixture; the live report still
	// establishes the aggregate's exact zero representation.
	bettingReport, err := s.Betting(ctx, testBrand, q)
	if err != nil {
		t.Fatal(err)
	}

	var before string
	fingerprint := `SELECT md5(
  (SELECT coalesce(jsonb_agg(to_jsonb(x) ORDER BY x.brand_id,x.account_id,x.source,x.state),'[]'::jsonb)::text FROM point_buckets x) || ':' ||
  (SELECT coalesce(jsonb_agg(to_jsonb(x) ORDER BY x.id),'[]'::jsonb)::text FROM point_ledger_entries x) || ':' ||
  (SELECT coalesce(jsonb_agg(to_jsonb(x) ORDER BY x.id),'[]'::jsonb)::text FROM recharge_orders x) || ':' ||
  (SELECT coalesce(jsonb_agg(to_jsonb(x) ORDER BY x.id),'[]'::jsonb)::text FROM reward_orders x) || ':' ||
  (SELECT coalesce(jsonb_agg(to_jsonb(x) ORDER BY x.id),'[]'::jsonb)::text FROM reward_order_actions x))`
	if err = db.QueryRow(ctx, fingerprint).Scan(&before); err != nil {
		t.Fatal(err)
	}
	tx, err = db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.CaptureArchive(ctx, tx, testBrand, from, to)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	tx, err = db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	localized, err := s.CaptureArchiveTimezone(ctx, tx, testBrand, from, to, "UTC")
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if localized.Timezone != "UTC" || localized.Betting != snapshot.Betting || localized.Ledger != snapshot.Ledger || localized.WalletSnapshot.Balances != snapshot.WalletSnapshot.Balances || localized.Withdrawals != snapshot.Withdrawals || localized.Commissions != snapshot.Commissions || localized.Rewards != snapshot.Rewards || localized.RewardOrders != snapshot.RewardOrders {
		t.Fatalf("timezone override changed report facts: %+v", localized)
	}
	if snapshot.FormatVersion != 1 || snapshot.BrandID != testBrand || snapshot.Timezone == "" || snapshot.SnapshotAt.IsZero() || !snapshot.WalletSnapshot.AtSnapshot.Equal(snapshot.SnapshotAt) {
		t.Fatalf("snapshot envelope: %+v", snapshot)
	}
	if snapshot.Betting != bettingReport.Summary || snapshot.Ledger != ledgerReport.Summary || snapshot.Withdrawals != withdrawalReport.Summary || snapshot.Commissions != commissionReport.Summary || snapshot.Rewards != rewardReport.Summary || snapshot.RewardOrders != rewardOrderReport.Summary {
		t.Fatalf("archive totals differ from live reports: archive=%+v", snapshot)
	}
	largeTotal := "18446744073709551640"
	if snapshot.Ledger.RechargePoints != "19" || snapshot.Ledger.NetPoints != largeTotal || snapshot.Rewards.EntryCount != "2" || snapshot.Rewards.NetPoints != "0" || snapshot.RewardOrders.OrderCount != "1" || snapshot.RewardOrders.RevokedPoints != "23" {
		t.Fatalf("source facts absent from snapshot: %+v", snapshot)
	}
	if snapshot.WalletSnapshot.Balances.AvailablePoints != largeTotal || snapshot.WalletSnapshot.Balances.TotalPoints != largeTotal {
		t.Fatalf("wallet snapshot=%+v", snapshot.WalletSnapshot)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"archive fixture proof", "private note", "archive fixture grant", "archive fixture manual credit"} {
		if containsString(string(encoded), secret) {
			t.Fatalf("snapshot leaked private source field %q: %s", secret, encoded)
		}
	}
	var after string
	if err = db.QueryRow(ctx, fingerprint).Scan(&after); err != nil || before != after {
		t.Fatalf("capture mutated financial state: before=%s after=%s err=%v", before, after, err)
	}
}

func TestCaptureArchiveValidationAndUnknownBrand(t *testing.T) {
	db := archiveTestDB(t)
	s := Service{DB: db}
	ctx := context.Background()
	now := time.Now().UTC()
	var volatility string
	var config []string
	if err := db.QueryRow(ctx, `SELECT p.provolatile,p.proconfig FROM pg_proc p WHERE p.oid='report_archive_capture(uuid,timestamp with time zone,timestamp with time zone,text)'::regprocedure`).Scan(&volatility, &config); err != nil {
		t.Fatal(err)
	}
	pinnedPath := ""
	if len(config) == 1 {
		pinnedPath = strings.ReplaceAll(config[0], ", ", ",")
	}
	if volatility != "s" || pinnedPath == "" || !strings.HasPrefix(pinnedPath, "search_path=pg_catalog,") || !strings.HasSuffix(pinnedPath, ",pg_temp") {
		t.Fatalf("archive capture function hardening: volatility=%q config=%v", volatility, config)
	}
	if _, err := s.CaptureArchive(ctx, nil, testBrand, now.Add(-time.Hour), now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("nil tx error=%v", err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	for _, tc := range []struct {
		brand string
		from  time.Time
		to    time.Time
	}{
		{"bad", now.Add(-time.Hour), now},
		{testBrand, now, now},
		{testBrand, now.Add(-94 * 24 * time.Hour), now},
		{testBrand, time.Time{}, now},
	} {
		if _, err = s.CaptureArchive(ctx, tx, tc.brand, tc.from, tc.to); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid input accepted: %+v err=%v", tc, err)
		}
	}
	if _, err = s.CaptureArchiveTimezone(ctx, tx, testBrand, now.Add(-time.Hour), now, "not/a-real-timezone"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid timezone error=%v", err)
	}
	if _, err = s.CaptureArchive(ctx, tx, ids.New(), now.Add(-time.Hour), now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown brand error=%v", err)
	}
}

func containsString(value, fragment string) bool {
	return len(fragment) > 0 && len(value) >= len(fragment) && strings.Contains(value, fragment)
}

func archiveTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	// Exercise the actual current migration chain, including the archive
	// persistence guards, rather than bypassing a later failing migration.
	return testdb.New(t)
}
