package reporting

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rewards"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
)

const (
	rewardReportTestBrand  = "0199a000-0000-7000-8000-000000000001"
	rewardReportOtherBrand = "0199a000-0000-7000-8000-000000000002"
)

func TestRewardQueryValidationAndNullableFilters(t *testing.T) {
	now := time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)
	q := RewardQuery{From: now.Add(-time.Hour), To: now, GroupBy: "order", Limit: 20}
	if err := q.Validate("rewards"); err != nil {
		t.Fatal(err)
	}
	if err := q.Validate("reward_orders"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("order grouping accepted for current-order report: %v", err)
	}
	for _, edit := range []func(*RewardQuery){
		func(q *RewardQuery) { q.From = time.Time{} },
		func(q *RewardQuery) { q.To = q.From },
		func(q *RewardQuery) { q.To = q.From.Add(93*24*time.Hour + time.Nanosecond) },
		func(q *RewardQuery) { q.GroupBy = "state" },
		func(q *RewardQuery) { q.Limit = 101 },
		func(q *RewardQuery) { q.Offset = 1000001 },
		func(q *RewardQuery) { bad := "bad"; q.OrderID = &bad },
	} {
		bad := q
		edit(&bad)
		if !errors.Is(bad.Validate("rewards"), ErrInvalid) {
			t.Fatalf("invalid reward query accepted: %+v", bad)
		}
	}
	encoded, err := json.Marshal(q)
	if err != nil || !strings.Contains(string(encoded), `"member_id":null`) || !strings.Contains(string(encoded), `"order_id":null`) {
		t.Fatalf("nil filters must serialize as explicit null: %s (%v)", encoded, err)
	}
}

func TestRewardReportsUsePostingFactsAndIndependentCurrentOrderCohort(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	memberID, userID, accountID, adminID := ids.New(), ids.New(), ids.New(), ids.New()
	for _, item := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO global_users(id,username,password_hash) VALUES($1,$2,'report-reward-test')`, []any{userID, "reward_report_" + userID[:8]}},
		{`INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version) VALUES($1,$2,$3,'operator','1','1')`, []any{memberID, rewardReportTestBrand, userID}},
		{`INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'report-reward-test')`, []any{adminID, "reward_report_admin_" + adminID[:8]}},
		{`INSERT INTO point_accounts(id,brand_id,brand_member_id) VALUES($1,$2,$3)`, []any{accountID, rewardReportTestBrand, memberID}},
		{`INSERT INTO point_buckets(brand_id,account_id,source,state) SELECT $1,$2,s,t FROM unnest(ARRAY['recharge','winning','gift','commission']) s CROSS JOIN unnest(ARRAY['available','manual_frozen','system_frozen','withdrawal']) t ON CONFLICT DO NOTHING`, []any{rewardReportTestBrand, accountID}},
	} {
		if _, err := db.Exec(ctx, item.sql, item.args...); err != nil {
			t.Fatal(err)
		}
	}
	grant := access.Permission{Resource: "reward", Action: "grant", Scope: access.ScopeBrand}
	revoke := access.Permission{Resource: "reward", Action: "revoke", Scope: access.ScopeBrand}
	viewer := access.Permission{Resource: "reward", Action: "view", Scope: access.ScopeBrand}
	actor := access.Account{ID: adminID, Type: access.AccountAdmin, BrandIDs: []string{rewardReportTestBrand, rewardReportOtherBrand}, Roles: []access.Role{
		{BrandID: rewardReportTestBrand, Permissions: []access.Permission{grant, revoke, viewer}},
		{BrandID: rewardReportOtherBrand, Permissions: []access.Permission{grant, revoke, viewer}},
	}}
	service := rewards.Service{DB: db}
	foreignUser, foreignMember, foreignAccount := ids.New(), ids.New(), ids.New()
	for _, item := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO global_users(id,username,password_hash) VALUES($1,$2,'foreign-reward-report-test')`, []any{foreignUser, "foreign_reward_" + foreignUser}},
		{`INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version) VALUES($1,$2,$3,'operator','1','1')`, []any{foreignMember, rewardReportOtherBrand, foreignUser}},
		{`INSERT INTO point_accounts(id,brand_id,brand_member_id) VALUES($1,$2,$3)`, []any{foreignAccount, rewardReportOtherBrand, foreignMember}},
		{`INSERT INTO point_buckets(brand_id,account_id,source,state) SELECT $1,$2,s,t FROM unnest(ARRAY['recharge','winning','gift','commission']) s CROSS JOIN unnest(ARRAY['available','manual_frozen','system_frozen','withdrawal']) t ON CONFLICT DO NOTHING`, []any{rewardReportOtherBrand, foreignAccount}},
	} {
		if _, err := db.Exec(ctx, item.sql, item.args...); err != nil {
			t.Fatal(err)
		}
	}
	foreignTx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	foreignOrder, err := service.GrantTx(ctx, foreignTx, rewardReportOtherBrand, actor, rewards.GrantInput{MemberID: foreignMember, Points: 23, Reason: "foreign scope fixture"}, points.Metadata{ActorType: "admin", ActorID: adminID, RequestID: ids.New()})
	if err != nil {
		t.Fatal(err)
	}
	if err = foreignTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	start := time.Now().UTC()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	order, err := service.GrantTx(ctx, tx, rewardReportTestBrand, actor, rewards.GrantInput{MemberID: memberID, Points: 37, Reason: "report fixture grant"}, points.Metadata{ActorType: "admin", ActorID: adminID, RequestID: ids.New()})
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var grantAt, createdAt time.Time
	if err = db.QueryRow(ctx, `SELECT l.created_at,o.created_at FROM reward_orders o JOIN point_ledger_entries l ON l.id=o.grant_ledger_entry_id WHERE o.brand_id=$1 AND o.id=$2`, rewardReportTestBrand, order.ID).Scan(&grantAt, &createdAt); err != nil {
		t.Fatal(err)
	}
	if grantAt.Equal(createdAt) {
		t.Fatalf("fixture requires distinct persisted cohort/posting timestamps: created=%s grant=%s", createdAt, grantAt)
	}
	time.Sleep(25 * time.Millisecond)
	tx, err = db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.RevokeTx(ctx, tx, rewardReportTestBrand, order.ID, actor, rewards.ActionInput{Version: 1, Reason: "report fixture revoke"}, points.Metadata{ActorType: "admin", ActorID: adminID, RequestID: ids.New()})
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var revokeAt time.Time
	if err = db.QueryRow(ctx, `SELECT created_at FROM point_ledger_entries WHERE operation_key=$1`, "reward-revoke:"+order.ID).Scan(&revokeAt); err != nil {
		t.Fatal(err)
	}
	// A generic wallet adjustment is not a reward posting, even when it lands
	// in the same points window and touches the same gift/recharge wallet.
	store := points.Store{DB: db}
	var unrelatedDelta points.Balance
	unrelatedDelta[0][0] = 13
	tx, err = db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Post(ctx, tx, points.Change{BrandID: rewardReportTestBrand, MemberID: memberID, EntryType: "adjustment", ReferenceType: "manual", OperationKey: "reward-report-unrelated:" + ids.New(), Reason: "unrelated report fixture credit", ActorType: "system", RequestID: ids.New(), Delta: unrelatedDelta, Allocation: []points.Allocation{{Source: "recharge", State: "available", Points: 13}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	reporter := Service{DB: db}
	all := RewardQuery{From: start.Add(-time.Second), To: revokeAt.Add(time.Second), GroupBy: "member", Limit: 20}
	posting, err := reporter.Reward(ctx, rewardReportTestBrand, all)
	if err != nil {
		t.Fatal(err)
	}
	if posting.Summary.EntryCount != "2" || posting.Summary.GrantEntryCount != "1" || posting.Summary.ReversalEntryCount != "1" || posting.Summary.GrantPoints != "37" || posting.Summary.ReversalPoints != "37" || posting.Summary.NetPoints != "0" || len(posting.Items) != 1 {
		t.Fatalf("posting report=%+v", posting)
	}
	badFilter := all
	malformed := "not-a-uuid"
	badFilter.MemberID = &malformed
	if _, err = reporter.Reward(ctx, rewardReportTestBrand, badFilter); !errors.Is(err, ErrInvalid) {
		t.Fatalf("malformed member filter: %v", err)
	}
	badFilter = all
	badFilter.OrderID = &foreignOrder.ID
	if _, err = reporter.Reward(ctx, rewardReportTestBrand, badFilter); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown scoped order filter: %v", err)
	}
	badFilter = all
	badFilter.MemberID = &foreignMember
	if _, err = reporter.Reward(ctx, rewardReportTestBrand, badFilter); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign-brand member filter: %v", err)
	}
	badFilter = all
	badFilter.MemberID = &foreignMember
	if _, err = reporter.RewardOrders(ctx, rewardReportTestBrand, badFilter); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign-brand order cohort member filter: %v", err)
	}
	badFilter = all
	badFilter.OrderID = &foreignOrder.ID
	if _, err = reporter.RewardOrders(ctx, rewardReportTestBrand, badFilter); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign-brand order filter: %v", err)
	}
	// A timezone opposite the UTC half-day guarantees the same posting belongs
	// to another calendar date, regardless of when the test runs.
	zone := "Etc/GMT+12"
	if grantAt.UTC().Hour() >= 12 {
		zone = "Etc/GMT-12"
	}
	if _, err = db.Exec(ctx, `UPDATE brands SET timezone=$2 WHERE id=$1`, rewardReportTestBrand, zone); err != nil {
		t.Fatal(err)
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		t.Fatal(err)
	}
	localDay := grantAt.In(loc).Format("2006-01-02")
	if localDay == grantAt.UTC().Format("2006-01-02") {
		t.Fatalf("selected timezone did not cross UTC date: %s at %s", zone, grantAt)
	}
	dayQuery := RewardQuery{From: start.Add(-time.Second), To: revokeAt.Add(time.Second), GroupBy: "day", Limit: 20}
	dayReport, err := reporter.Reward(ctx, rewardReportTestBrand, dayQuery)
	if err != nil || dayReport.Timezone != zone || len(dayReport.Items) != 1 || dayReport.Items[0].Key != localDay {
		t.Fatalf("timezone-local posting day: %+v err=%v want=%s", dayReport, err, localDay)
	}
	orderDayQuery := RewardQuery{From: createdAt.Add(-time.Microsecond), To: createdAt.Add(time.Second), GroupBy: "day", Limit: 20, OrderID: &order.ID}
	orderDayReport, err := reporter.RewardOrders(ctx, rewardReportTestBrand, orderDayQuery)
	if err != nil || orderDayReport.Timezone != zone || len(orderDayReport.Items) != 1 || orderDayReport.Items[0].Key != createdAt.In(loc).Format("2006-01-02") {
		t.Fatalf("timezone-local order cohort day: %+v err=%v", orderDayReport, err)
	}
	// PostgreSQL stores microseconds. databaseBound must preserve [from,to)
	// semantics for sub-microsecond caller bounds instead of truncating them.
	nanosecondQuery := RewardQuery{From: grantAt.Add(-time.Nanosecond), To: grantAt.Add(time.Microsecond), GroupBy: "order", Limit: 20}
	nanosecondReport, err := reporter.Reward(ctx, rewardReportTestBrand, nanosecondQuery)
	if err != nil || nanosecondReport.Summary.EntryCount != "1" {
		t.Fatalf("nanosecond lower boundary lost exact posting: %+v err=%v", nanosecondReport.Summary, err)
	}
	nanosecondQuery.From, nanosecondQuery.To = grantAt, grantAt.Add(time.Microsecond)
	nanosecondReport, err = reporter.Reward(ctx, rewardReportTestBrand, nanosecondQuery)
	if err != nil || nanosecondReport.Summary.EntryCount != "1" {
		t.Fatalf("exact lower bound should include posting: %+v err=%v", nanosecondReport.Summary, err)
	}
	nanosecondQuery.From, nanosecondQuery.To = grantAt.Add(time.Nanosecond), grantAt.Add(time.Microsecond)
	nanosecondReport, err = reporter.Reward(ctx, rewardReportTestBrand, nanosecondQuery)
	if err != nil || nanosecondReport.Summary.EntryCount != "0" {
		t.Fatalf("nanosecond after posting must exclude it: %+v err=%v", nanosecondReport.Summary, err)
	}
	cohortFrom, cohortTo := createdAt, grantAt
	if grantAt.Before(createdAt) {
		cohortFrom, cohortTo = grantAt, createdAt
	}
	cohortBoundary := RewardQuery{From: cohortFrom, To: cohortTo, GroupBy: "day", Limit: 20, OrderID: &order.ID}
	cohortBetween, err := reporter.RewardOrders(ctx, rewardReportTestBrand, cohortBoundary)
	if err != nil {
		t.Fatal(err)
	}
	postingBoundary := cohortBoundary
	postingBoundary.GroupBy = "order"
	postingBetween, err := reporter.Reward(ctx, rewardReportTestBrand, postingBoundary)
	if err != nil {
		t.Fatal(err)
	}
	if createdAt.Before(grantAt) && (cohortBetween.Summary.OrderCount != "1" || postingBetween.Summary.EntryCount != "0") ||
		grantAt.Before(createdAt) && (cohortBetween.Summary.OrderCount != "0" || postingBetween.Summary.EntryCount != "1") {
		t.Fatalf("created cohort and posting windows collapsed: cohort=%+v posting=%+v created=%s grant=%s", cohortBetween.Summary, postingBetween.Summary, createdAt, grantAt)
	}
	// The reversal-only window has a posting but excludes the order's creation
	// cohort. This also proves a historical grant remains meaningful after revoke.
	reversalWindow := RewardQuery{From: grantAt.Add(10 * time.Millisecond), To: revokeAt.Add(time.Second), GroupBy: "order", Limit: 20}
	reversal, err := reporter.Reward(ctx, rewardReportTestBrand, reversalWindow)
	if err != nil {
		t.Fatal(err)
	}
	if reversal.Summary.EntryCount != "1" || reversal.Summary.GrantPoints != "0" || reversal.Summary.ReversalPoints != "37" || reversal.Summary.NetPoints != "-37" {
		t.Fatalf("reversal-only posting window=%+v", reversal.Summary)
	}
	cohort := RewardQuery{From: createdAt.Add(-time.Second), To: createdAt.Add(time.Second), GroupBy: "state", Limit: 20}
	cohort.OrderID = &order.ID
	orders, err := reporter.RewardOrders(ctx, rewardReportTestBrand, cohort)
	if err != nil {
		t.Fatal(err)
	}
	if orders.Summary.OrderCount != "1" || orders.Summary.OriginalPoints != "37" || orders.Summary.RevokedCount != "1" || orders.Summary.RevokedPoints != "37" || len(orders.Items) != 1 || orders.Items[0].Key != "revoked" {
		t.Fatalf("current order cohort=%+v", orders)
	}
	// A later order whose gift points are frozen can be pending; the report
	// counts its original amount even though it has no reversal ledger entry.
	tx, err = db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	pendingOrder, err := service.GrantTx(ctx, tx, rewardReportTestBrand, actor, rewards.GrantInput{MemberID: memberID, Points: 11, Reason: "report fixture pending grant"}, points.Metadata{ActorType: "admin", ActorID: adminID, RequestID: ids.New()})
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	allocation := []points.Allocation{{Source: "gift", State: "available", Points: 11}}
	move, err := points.AllocationDelta(allocation, "available", "manual_frozen")
	if err != nil {
		t.Fatal(err)
	}
	tx, err = db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Post(ctx, tx, points.Change{BrandID: rewardReportTestBrand, MemberID: memberID, EntryType: "adjustment", ReferenceType: "manual", OperationKey: "reward-report-freeze:" + ids.New(), Reason: "test pending original award", ActorType: "system", RequestID: ids.New(), Delta: move, Allocation: allocation})
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	tx, err = db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.RevokeTx(ctx, tx, rewardReportTestBrand, pendingOrder.ID, actor, rewards.ActionInput{Version: 1, Reason: "report fixture pending revoke"}, points.Metadata{ActorType: "admin", ActorID: adminID, RequestID: ids.New()})
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var pendingCreated time.Time
	if err = db.QueryRow(ctx, `SELECT created_at FROM reward_orders WHERE id=$1`, pendingOrder.ID).Scan(&pendingCreated); err != nil {
		t.Fatal(err)
	}
	pendingQuery := RewardQuery{From: pendingCreated.Add(-time.Second), To: pendingCreated.Add(time.Second), GroupBy: "state", Limit: 20, OrderID: &pendingOrder.ID}
	pendingReport, err := reporter.RewardOrders(ctx, rewardReportTestBrand, pendingQuery)
	if err != nil || pendingReport.Summary.OrderCount != "1" || pendingReport.Summary.PendingCount != "1" || pendingReport.Summary.PendingPoints != "11" || len(pendingReport.Items) != 1 || pendingReport.Items[0].Key != "revocation_pending" {
		t.Fatalf("pending state report=%+v err=%v", pendingReport, err)
	}
	noCohort := cohort
	noCohort.From, noCohort.To = reversalWindow.From, reversalWindow.To
	noCohort.GroupBy = "day"
	empty, err := reporter.RewardOrders(ctx, rewardReportTestBrand, noCohort)
	if err != nil || empty.Summary.OrderCount != "0" || empty.TotalGroups != "0" || empty.Items == nil {
		t.Fatalf("order cohort must be independent from ledger posting window: %+v %v", empty, err)
	}
	var before, after string
	fingerprintSQL := `SELECT concat_ws(':',(SELECT count(*) FROM point_ledger_entries),(SELECT count(*) FROM reward_order_actions),(SELECT count(*) FROM audit_logs),(SELECT count(*) FROM outbox_events))`
	if err = db.QueryRow(ctx, fingerprintSQL).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err = reporter.Reward(ctx, rewardReportTestBrand, all); err != nil {
		t.Fatal(err)
	}
	if _, err = reporter.RewardOrders(ctx, rewardReportTestBrand, cohort); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, fingerprintSQL).Scan(&after); err != nil || before != after {
		t.Fatalf("report modified financial/audit state: before=%s after=%s err=%v", before, after, err)
	}
	// Each grant is a valid bigint amount; the aggregate across two members is
	// larger than int64 and must remain exact numeric text in both report types.
	const maxAmount int64 = 9223372036854775807
	maxMemberIDs := make([]string, 2)
	for i := range maxMemberIDs {
		newUser, newMember, newAccount := ids.New(), ids.New(), ids.New()
		maxMemberIDs[i] = newMember
		for _, item := range []struct {
			sql  string
			args []any
		}{
			{`INSERT INTO global_users(id,username,password_hash) VALUES($1,$2,'reward-max-report-test')`, []any{newUser, "reward_max_" + newUser}},
			{`INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version) VALUES($1,$2,$3,'operator','1','1')`, []any{newMember, rewardReportTestBrand, newUser}},
			{`INSERT INTO point_accounts(id,brand_id,brand_member_id) VALUES($1,$2,$3)`, []any{newAccount, rewardReportTestBrand, newMember}},
			{`INSERT INTO point_buckets(brand_id,account_id,source,state) SELECT $1,$2,s,t FROM unnest(ARRAY['recharge','winning','gift','commission']) s CROSS JOIN unnest(ARRAY['available','manual_frozen','system_frozen','withdrawal']) t ON CONFLICT DO NOTHING`, []any{rewardReportTestBrand, newAccount}},
		} {
			if _, err = db.Exec(ctx, item.sql, item.args...); err != nil {
				t.Fatal(err)
			}
		}
		tx, err = db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		_, err = service.GrantTx(ctx, tx, rewardReportTestBrand, actor, rewards.GrantInput{MemberID: newMember, Points: points.Amount(maxAmount), Reason: "maximum valid report grant"}, points.Metadata{ActorType: "admin", ActorID: adminID, RequestID: ids.New()})
		if err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var maxCohort time.Time
	if err = db.QueryRow(ctx, `SELECT min(l.created_at) FROM reward_orders o JOIN point_ledger_entries l ON l.id=o.grant_ledger_entry_id WHERE o.member_id=ANY($1::uuid[])`, maxMemberIDs).Scan(&maxCohort); err != nil {
		t.Fatal(err)
	}
	maxQuery := RewardQuery{From: maxCohort.Add(-time.Millisecond), To: time.Now().UTC().Add(time.Minute), GroupBy: "day", Limit: 20}
	large, err := reporter.Reward(ctx, rewardReportTestBrand, maxQuery)
	wantLarge := "18446744073709551614"
	if err != nil || large.Summary.GrantPoints != wantLarge || large.Summary.EntryCount != "2" {
		t.Fatalf("large reward aggregate=%+v err=%v", large.Summary, err)
	}
	largeOrders, err := reporter.RewardOrders(ctx, rewardReportTestBrand, maxQuery)
	if err != nil || largeOrders.Summary.OriginalPoints != wantLarge || largeOrders.Summary.OrderCount != "2" {
		t.Fatalf("large order aggregate=%+v err=%v", largeOrders.Summary, err)
	}
}
