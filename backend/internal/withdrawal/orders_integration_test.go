package withdrawal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	orderTestBrand  = "0199a000-0000-7000-8000-000000000001"
	orderOtherBrand = "0199a000-0000-7000-8000-000000000002"
)

type orderEligibilityStub struct {
	allowed bool
	err     error
}

func (s *orderEligibilityStub) Check(context.Context, pgx.Tx, EligibilityInput) (EligibilityDecision, error) {
	if s.err != nil {
		return EligibilityDecision{}, s.err
	}
	return EligibilityDecision{Allowed: s.allowed, Evidence: json.RawMessage(`{"test_adapter":true}`)}, nil
}

type orderIntegrationFixture struct {
	db      *pgxpool.Pool
	service OrderService
	points  points.Store
	member  string
	user    string
	admin   access.Account
	adminID string
	super   access.Account
	actor   points.Metadata
	checker *orderEligibilityStub
}

func newOrderIntegrationFixture(t *testing.T, enabled, allowed bool) orderIntegrationFixture {
	t.Helper()
	db := testdb.New(t)
	ctx := context.Background()
	userID, memberID := ids.New(), ids.New()
	if _, err := db.Exec(ctx, `INSERT INTO global_users(id,username,password_hash) VALUES($1,$2,'test-only-hash')`, userID, "withdrawal_"+userID[:8]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version) VALUES($1,$2,$3,'domain','dev-1','dev-1')`, memberID, orderTestBrand, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO point_accounts(id,brand_id,brand_member_id) VALUES($1,$2,$3)`, ids.New(), orderTestBrand, memberID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO point_buckets(brand_id,account_id,source,state) SELECT $1,id,s,t FROM point_accounts CROSS JOIN unnest(ARRAY['recharge','winning','gift','commission']) s CROSS JOIN unnest(ARRAY['available','manual_frozen','system_frozen','withdrawal']) t WHERE brand_id=$1 AND brand_member_id=$2 ON CONFLICT DO NOTHING`, orderTestBrand, memberID); err != nil {
		t.Fatal(err)
	}

	adminID, superID, roleID := ids.New(), ids.New(), ids.New()
	for _, a := range []struct {
		id    string
		name  string
		super bool
	}{{adminID, "withdrawal_admin_" + adminID[:8], false}, {superID, "withdrawal_super_" + superID[:8], true}} {
		if _, err := db.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash,is_super_admin) VALUES($1,$2,'test-only-hash',$3)`, a.id, a.name, a.super); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(ctx, `INSERT INTO admin_brand_scopes(account_id,brand_id) VALUES($1,$2),($3,$2)`, adminID, orderTestBrand, superID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO roles(id,brand_id,code,name) VALUES($1,$2,$3,'Withdrawal integration')`, roleID, orderTestBrand, "withdrawal_test_"+roleID[:8]); err != nil {
		t.Fatal(err)
	}
	permissions := []string{"withdrawal.view.brand", "withdrawal.approve.brand", "withdrawal.reject.brand", "withdrawal.cancel.brand", "withdrawal.fail.brand", "withdrawal.mark_paid.brand", "withdrawal_policy.write.brand"}
	for _, key := range permissions {
		if _, err := db.Exec(ctx, `INSERT INTO permissions(key) VALUES($1) ON CONFLICT DO NOTHING`, key); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,$2) ON CONFLICT DO NOTHING`, roleID, key); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{adminID, superID} {
		if _, err := db.Exec(ctx, `INSERT INTO admin_account_roles(account_id,role_id) VALUES($1,$2)`, id, roleID); err != nil {
			t.Fatal(err)
		}
	}
	adminStore := adminsys.Store{DB: db}
	admin, err := adminStore.Account(ctx, adminID)
	if err != nil {
		t.Fatal(err)
	}
	super, err := adminStore.Account(ctx, superID)
	if err != nil {
		t.Fatal(err)
	}
	checker := &orderEligibilityStub{allowed: allowed}
	service := OrderService{DB: db, Points: points.Store{DB: db}, Eligibility: checker}
	fixture := orderIntegrationFixture{db: db, service: service, points: service.Points, member: memberID, user: userID, admin: admin, adminID: adminID, super: super, actor: points.Metadata{ActorType: "user", ActorID: userID, RequestID: ids.New()}, checker: checker}
	fixture.updatePolicy(t, enabled, nil)
	return fixture
}

func (f orderIntegrationFixture) updatePolicy(t *testing.T, enabled bool, max *points.Amount) {
	t.Helper()
	s := Service{DB: f.db}
	policy, err := s.BrandPolicy(context.Background(), orderTestBrand)
	if err != nil {
		t.Fatal(err)
	}
	config := DefaultBrandConfig()
	config.Enabled = enabled
	config.MaxPoints = max
	tx, err := f.db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	_, err = s.UpdateBrand(context.Background(), tx, orderTestBrand, f.admin, BrandInput{Version: policy.Version, Config: config, Reason: "integration fixture policy"}, points.Metadata{ActorType: "admin", ActorID: f.adminID, RequestID: ids.New()})
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func (f orderIntegrationFixture) fund(t *testing.T, allocation []points.Allocation) {
	t.Helper()
	var delta points.Balance
	for _, a := range allocation {
		si, err := points.SourceIndex(a.Source)
		if err != nil {
			t.Fatal(err)
		}
		delta[si][0] += a.Points
	}
	tx, err := f.db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	_, err = f.points.Post(context.Background(), tx, points.Change{BrandID: orderTestBrand, MemberID: f.member, EntryType: "adjustment", ReferenceType: "test_funding", OperationKey: "withdrawal_fund_" + ids.New(), Reason: "integration test funding", ActorType: "system", RequestID: ids.New(), Delta: delta, Allocation: allocation})
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func createOrder(t *testing.T, f orderIntegrationFixture, amount points.Amount, key string, allocation []points.Allocation) Order {
	t.Helper()
	tx, err := f.db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	out, err := f.service.Create(context.Background(), tx, orderTestBrand, f.member, OrderInput{Points: amount, SourceAllocation: allocation, ClientKey: key}, f.actor)
	if err != nil {
		t.Fatalf("Create(%q): %v", key, err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	return out
}

func orderState(t *testing.T, order Order) string {
	t.Helper()
	raw, err := json.Marshal(order)
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if err = json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	state, _ := data["state"].(string)
	return state
}

func orderID(t *testing.T, order Order) string {
	t.Helper()
	raw, err := json.Marshal(order)
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if err = json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	id, _ := data["id"].(string)
	return id
}

func orderAction(t *testing.T, f orderIntegrationFixture, id, action, key string, actor access.Account, input ActionInput, meta points.Metadata) (Order, error) {
	t.Helper()
	tx, err := f.db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	out, err := f.service.Advance(context.Background(), tx, orderTestBrand, id, action, actor, input, meta)
	if err != nil {
		return Order{}, err
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	return out, nil
}

func TestOrderMixedSourceReservationReversalAndReceiptReplay(t *testing.T) {
	f := newOrderIntegrationFixture(t, true, true)
	allocation := []points.Allocation{{Source: "recharge", State: "available", Points: 40}, {Source: "winning", State: "available", Points: 30}, {Source: "gift", State: "available", Points: 20}}
	f.fund(t, allocation)
	order := createOrder(t, f, 90, "mixed-source-order", allocation)
	if got := orderState(t, order); got != "reviewing" {
		t.Fatalf("new order state=%q, want reviewing", got)
	}
	wallet, err := f.points.Read(context.Background(), orderTestBrand, f.member)
	if err != nil {
		t.Fatal(err)
	}
	if wallet.AvailablePoints != 0 || wallet.WithdrawalPoints != 90 {
		t.Fatalf("reservation wallet available/withdrawal=%d/%d, want 0/90", wallet.AvailablePoints, wallet.WithdrawalPoints)
	}
	if _, err = orderAction(t, f, orderID(t, order), "reject", "mixed-reject", f.admin, ActionInput{Version: 1, ClientKey: "mixed-reject", Reason: "review declined"}, points.Metadata{ActorType: "admin", ActorID: f.adminID, RequestID: ids.New()}); err != nil {
		t.Fatal(err)
	}
	wallet, err = f.points.Read(context.Background(), orderTestBrand, f.member)
	if err != nil {
		t.Fatal(err)
	}
	if wallet.BySource[0][0] != 40 || wallet.BySource[1][0] != 30 || wallet.BySource[2][0] != 20 || wallet.WithdrawalPoints != 0 {
		t.Fatalf("rejection did not restore original source allocation: %+v", wallet.BySource)
	}
	replay, err := f.service.Read(context.Background(), orderTestBrand, orderID(t, order))
	if err != nil || orderState(t, replay) != "rejected" {
		t.Fatalf("terminal order read state=%q err=%v", orderState(t, replay), err)
	}
	entryRows, err := f.db.Query(context.Background(), `SELECT source_allocation FROM point_ledger_entries WHERE reference_type='withdrawal' AND reference_id=$1 ORDER BY version`, orderID(t, order))
	if err != nil {
		t.Fatal(err)
	}
	defer entryRows.Close()
	count := 0
	for entryRows.Next() {
		var got []points.Allocation
		var raw []byte
		if err = entryRows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(raw, &got); err != nil {
			t.Fatal(err)
		}
		if len(got) != len(allocation) {
			t.Fatalf("ledger source allocation=%+v, want original allocation %+v", got, allocation)
		}
		count++
	}
	if count != 2 {
		t.Fatalf("reservation/reversal ledger count=%d, want 2", count)
	}
}

// Test-only historical-shape construction. This models immutable pre-commission
// evidence from an actual three-source business workflow; it is not a repair
// path. Only the two append-only guards are disabled and both are restored
// before the test transaction commits.
func synthesizeLegacyWithdrawalLedger(t *testing.T, db *pgxpool.Pool, accountID string) {
	t.Helper()
	ctx := context.Background()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var schema string
	if err = tx.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(schema, "test_") {
		t.Fatalf("refusing historical-shape construction outside an owned test schema: %q", schema)
	}
	var nonzeroCommission int
	zeroBucket := `{"available":"0","manual_frozen":"0","system_frozen":"0","withdrawal":"0"}`
	if err = tx.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM point_buckets WHERE account_id=$1 AND source='commission' AND points<>0)+
 (SELECT count(*) FROM point_ledger_entries l CROSS JOIN LATERAL (VALUES(l.before_snapshot),(l.delta_snapshot),(l.after_snapshot)) s(snapshot) WHERE l.account_id=$1 AND s.snapshot ? 'commission' AND s.snapshot->'commission' IS DISTINCT FROM $2::jsonb)+
 (SELECT count(*) FROM audit_logs a JOIN point_ledger_entries l ON a.after_json->>'ledger_entry_id'=l.id::text WHERE l.account_id=$1 AND a.action='points.'||l.entry_type AND a.resource_type='point_account' AND a.resource_id=l.account_id AND ((a.before_json ? 'commission' AND a.before_json->'commission' IS DISTINCT FROM $2::jsonb) OR (a.after_json->'balance' ? 'commission' AND a.after_json->'balance'->'commission' IS DISTINCT FROM $2::jsonb)))`, accountID, zeroBucket).Scan(&nonzeroCommission); err != nil {
		t.Fatal(err)
	}
	if nonzeroCommission != 0 {
		t.Fatalf("refusing to remove commission keys from nonzero historical evidence (nonzero records=%d)", nonzeroCommission)
	}
	for _, stmt := range []string{`ALTER TABLE point_ledger_entries DISABLE TRIGGER ledger_immutable`, `ALTER TABLE audit_logs DISABLE TRIGGER audit_immutable`} {
		if _, err = tx.Exec(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	for _, stmt := range []string{
		`UPDATE point_ledger_entries SET before_snapshot=before_snapshot-'commission',delta_snapshot=delta_snapshot-'commission',after_snapshot=after_snapshot-'commission' WHERE account_id=$1`,
		`UPDATE audit_logs a SET before_json=a.before_json-'commission',after_json=jsonb_set(a.after_json,'{balance}',(a.after_json->'balance')-'commission') FROM point_ledger_entries l WHERE l.account_id=$1 AND a.action='points.'||l.entry_type AND a.resource_type='point_account' AND a.resource_id=l.account_id AND a.after_json->>'ledger_entry_id'=l.id::text`,
	} {
		if _, err = tx.Exec(ctx, stmt, accountID); err != nil {
			t.Fatalf("construct historical withdrawal evidence: %v", err)
		}
	}
	for _, stmt := range []string{`ALTER TABLE audit_logs ENABLE TRIGGER audit_immutable`, `ALTER TABLE point_ledger_entries ENABLE TRIGGER ledger_immutable`} {
		if _, err = tx.Exec(ctx, stmt); err != nil {
			t.Fatalf("restore append-only guard: %v", err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyTwelveBucketWithdrawalReserveCanBeReleasedWithSixteenBucketProof(t *testing.T) {
	f := newOrderIntegrationFixture(t, true, true)
	allocation := []points.Allocation{{Source: "recharge", State: "available", Points: 40}, {Source: "winning", State: "available", Points: 30}, {Source: "gift", State: "available", Points: 20}}
	f.fund(t, allocation)
	originalWallet, err := f.points.Read(context.Background(), orderTestBrand, f.member)
	if err != nil {
		t.Fatal(err)
	}
	order := createOrder(t, f, 90, "legacy-three-source-withdrawal", allocation)
	ctx := context.Background()
	var accountID string
	if err := f.db.QueryRow(ctx, `SELECT account_id::text FROM withdrawal_orders WHERE id=$1`, order.ID).Scan(&accountID); err != nil {
		t.Fatal(err)
	}
	var commissionRows, commissionTotal int
	if err := f.db.QueryRow(ctx, `SELECT count(*),coalesce(sum(points),0) FROM point_buckets WHERE account_id=$1 AND source='commission'`, accountID).Scan(&commissionRows, &commissionTotal); err != nil || commissionRows != 4 || commissionTotal != 0 {
		t.Fatalf("legacy fixture must have four zero commission buckets: rows=%d total=%d err=%v", commissionRows, commissionTotal, err)
	}
	synthesizeLegacyWithdrawalLedger(t, f.db, accountID)
	var reserveID string
	var oldBefore, oldDelta, oldAfter, oldAllocation []byte
	var oldHash string
	if err := f.db.QueryRow(ctx, `SELECT id::text,before_snapshot,delta_snapshot,after_snapshot,source_allocation,request_hash FROM point_ledger_entries WHERE id=(SELECT reserve_entry_id FROM withdrawal_orders WHERE id=$1)`, order.ID).Scan(&reserveID, &oldBefore, &oldDelta, &oldAfter, &oldAllocation, &oldHash); err != nil {
		t.Fatal(err)
	}
	for _, raw := range [][]byte{oldBefore, oldDelta, oldAfter} {
		var snapshot map[string]json.RawMessage
		if err := json.Unmarshal(raw, &snapshot); err != nil || len(snapshot) != 3 {
			t.Fatalf("expected historical 12-bucket reserve, keys=%d err=%v", len(snapshot), err)
		}
	}
	var immutableOrderBefore []byte
	if err := f.db.QueryRow(ctx, `SELECT jsonb_build_object('source_allocation',source_allocation,'policy_snapshot',policy_snapshot,'reserve_entry_id',reserve_entry_id,'reserve_version',reserve_version,'points',points) FROM withdrawal_orders WHERE id=$1`, order.ID).Scan(&immutableOrderBefore); err != nil {
		t.Fatal(err)
	}
	if rec, err := f.points.Reconcile(ctx, orderTestBrand, f.member); err != nil || !rec.Consistent {
		t.Fatalf("synthetic historical ledger/hash must reconcile: %+v err=%v", rec, err)
	}
	reserved, err := f.points.Read(ctx, orderTestBrand, f.member)
	if err != nil {
		t.Fatal(err)
	}
	if reserved.WithdrawalPoints != 90 || reserved.AvailablePoints != 0 {
		t.Fatalf("fixture must contain a complete source reservation: %+v", reserved.BySource)
	}
	meta := points.Metadata{ActorType: "admin", ActorID: f.adminID, RequestID: ids.New()}
	input := ActionInput{Version: 1, ClientKey: "legacy-withdrawal-reject", Reason: "release legacy reservation"}
	if _, err = orderAction(t, f, order.ID, "reject", input.ClientKey, f.admin, input, meta); err != nil {
		t.Fatal(err)
	}
	after, err := f.points.Read(ctx, orderTestBrand, f.member)
	if err != nil || after.BySource != originalWallet.BySource || after.WithdrawalPoints != 0 {
		t.Fatalf("legacy reserve release did not restore exact wallet: original=%+v after=%+v err=%v", originalWallet.BySource, after.BySource, err)
	}
	var gotBefore, gotDelta, gotAfter, gotAllocation []byte
	var gotHash string
	if err = f.db.QueryRow(ctx, `SELECT before_snapshot,delta_snapshot,after_snapshot,source_allocation,request_hash FROM point_ledger_entries WHERE id=$1`, reserveID).Scan(&gotBefore, &gotDelta, &gotAfter, &gotAllocation, &gotHash); err != nil {
		t.Fatal(err)
	}
	if string(gotBefore) != string(oldBefore) || string(gotDelta) != string(oldDelta) || string(gotAfter) != string(oldAfter) || string(gotAllocation) != string(oldAllocation) || gotHash != oldHash {
		t.Fatal("withdrawal transition rewrote historical reserve evidence")
	}
	var immutableOrderAfter []byte
	if err = f.db.QueryRow(ctx, `SELECT jsonb_build_object('source_allocation',source_allocation,'policy_snapshot',policy_snapshot,'reserve_entry_id',reserve_entry_id,'reserve_version',reserve_version,'points',points) FROM withdrawal_orders WHERE id=$1`, order.ID).Scan(&immutableOrderAfter); err != nil || string(immutableOrderAfter) != string(immutableOrderBefore) {
		t.Fatalf("withdrawal transition rewrote original order snapshot: %s -> %s err=%v", immutableOrderBefore, immutableOrderAfter, err)
	}
	var releaseID string
	if err = f.db.QueryRow(ctx, `SELECT release_entry_id::text FROM withdrawal_orders WHERE id=$1`, order.ID).Scan(&releaseID); err != nil {
		t.Fatal(err)
	}
	var releaseBefore, releaseDelta, releaseAfter []byte
	if err = f.db.QueryRow(ctx, `SELECT before_snapshot,delta_snapshot,after_snapshot FROM point_ledger_entries WHERE id=$1`, releaseID).Scan(&releaseBefore, &releaseDelta, &releaseAfter); err != nil {
		t.Fatal(err)
	}
	for _, raw := range [][]byte{releaseBefore, releaseDelta, releaseAfter} {
		var snapshot map[string]json.RawMessage
		if err = json.Unmarshal(raw, &snapshot); err != nil || len(snapshot) != 4 {
			t.Fatalf("release proof must use new sixteen-bucket format: keys=%d err=%v", len(snapshot), err)
		}
	}
	var receiptCount int
	if err = f.db.QueryRow(ctx, `SELECT count(*) FROM withdrawal_operation_receipts WHERE order_id=$1 AND client_key=$2`, order.ID, input.ClientKey).Scan(&receiptCount); err != nil || receiptCount != 1 {
		t.Fatalf("expected one reject receipt, count=%d err=%v", receiptCount, err)
	}
	if _, err = orderAction(t, f, order.ID, "reject", input.ClientKey, f.admin, input, meta); err != nil {
		t.Fatalf("terminal action replay: %v", err)
	}
	var entryCount, receiptCountAfter int
	if err = f.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE reference_type='withdrawal' AND reference_id=$1`, order.ID).Scan(&entryCount); err != nil {
		t.Fatal(err)
	}
	if err = f.db.QueryRow(ctx, `SELECT count(*) FROM withdrawal_operation_receipts WHERE order_id=$1 AND client_key=$2`, order.ID, input.ClientKey).Scan(&receiptCountAfter); err != nil || entryCount != 2 || receiptCountAfter != 1 {
		t.Fatalf("replay duplicated financial evidence: ledger_entries=%d receipts=%d err=%v", entryCount, receiptCountAfter, err)
	}
	if rec, err := f.points.Reconcile(ctx, orderTestBrand, f.member); err != nil || !rec.Consistent {
		t.Fatalf("post-release legacy/new chain must reconcile without old hash rewrite: %+v err=%v", rec, err)
	}
}

func TestCommissionWithdrawalRequiresExplicitPolicyAndUsesSavedRuleSnapshot(t *testing.T) {
	f := newOrderIntegrationFixture(t, true, true)
	policyService := Service{DB: f.db}
	policy, err := policyService.BrandPolicy(context.Background(), orderTestBrand)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(policy.Config.AllowedSources) != fmt.Sprint([]string{"recharge", "winning", "gift"}) {
		t.Fatalf("legacy default authorization changed: %v", policy.Config.AllowedSources)
	}
	configured := policy.Config
	configured.AllowedSources = []string{"recharge", "winning", "gift", "commission"}
	tx, err := f.db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	policy, err = policyService.UpdateBrand(context.Background(), tx, orderTestBrand, f.admin, BrandInput{Version: policy.Version, Config: configured, Reason: "explicitly authorize commission withdrawals"}, points.Metadata{ActorType: "admin", ActorID: f.adminID, RequestID: ids.New()})
	if err != nil {
		_ = tx.Rollback(context.Background())
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	commissionAllocation := []points.Allocation{{Source: "commission", State: "available", Points: 100}}
	f.fund(t, commissionAllocation)
	first := createOrder(t, f, 25, "commission-refund-order", []points.Allocation{{Source: "commission", State: "available", Points: 25}})
	if fmt.Sprint(first.PolicySnapshot.Config.AllowedSources) != fmt.Sprint(configured.AllowedSources) {
		t.Fatalf("application lost its configured policy snapshot: %+v", first.PolicySnapshot)
	}
	rejected, err := orderAction(t, f, first.ID, "reject", "commission-refund-reject", f.admin, ActionInput{Version: 1, ClientKey: "commission-refund-reject", Reason: "refund commission allocation"}, points.Metadata{ActorType: "admin", ActorID: f.adminID, RequestID: ids.New()})
	if err != nil || rejected.State != "rejected" {
		t.Fatalf("commission source refund failed: %+v err=%v", rejected, err)
	}
	wallet, err := f.points.Read(context.Background(), orderTestBrand, f.member)
	if err != nil || wallet.BySource[3][0] != 100 {
		t.Fatalf("commission refund did not restore exact source: %+v err=%v", wallet.BySource, err)
	}
	paidOrder := createOrder(t, f, 40, "commission-paid-order", []points.Allocation{{Source: "commission", State: "available", Points: 40}})
	if fmt.Sprint(paidOrder.PolicySnapshot.Config.AllowedSources) != fmt.Sprint(configured.AllowedSources) {
		t.Fatalf("paid application did not capture commission authorization: %+v", paidOrder.PolicySnapshot)
	}
	// A later admin policy edit cannot rewrite the authorization snapshot used
	// by this already accepted order.
	current, err := policyService.BrandPolicy(context.Background(), orderTestBrand)
	if err != nil {
		t.Fatal(err)
	}
	currentConfig := current.Config
	currentConfig.AllowedSources = []string{"recharge", "winning", "gift"}
	tx, err = f.db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = policyService.UpdateBrand(context.Background(), tx, orderTestBrand, f.admin, BrandInput{Version: current.Version, Config: currentConfig, Reason: "remove commission for future orders"}, points.Metadata{ActorType: "admin", ActorID: f.adminID, RequestID: ids.New()}); err != nil {
		_ = tx.Rollback(context.Background())
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	approved, err := orderAction(t, f, paidOrder.ID, "approve", "commission-paid-approve", f.admin, ActionInput{Version: 1, ClientKey: "commission-paid-approve", Reason: "approve original commission request"}, points.Metadata{ActorType: "admin", ActorID: f.adminID, RequestID: ids.New()})
	if err != nil || approved.PolicySnapshot.Config.AllowedSources[3] != "commission" {
		t.Fatalf("approval did not retain original policy snapshot: %+v err=%v", approved, err)
	}
	paid, err := orderAction(t, f, paidOrder.ID, "mark_paid", "commission-paid-final", f.admin, ActionInput{Version: 2, ClientKey: "commission-paid-final", Reason: "pay original commission request"}, points.Metadata{ActorType: "admin", ActorID: f.adminID, RequestID: ids.New()})
	if err != nil || paid.State != "paid" || paid.PolicySnapshot.Config.AllowedSources[3] != "commission" {
		t.Fatalf("commission-funded request did not pay under saved rules: %+v err=%v", paid, err)
	}
	wallet, err = f.points.Read(context.Background(), orderTestBrand, f.member)
	if err != nil || wallet.BySource[3][0] != 60 || wallet.WithdrawalPoints != 0 {
		t.Fatalf("paid commission source balance=%+v err=%v", wallet.BySource, err)
	}
}

func TestOrderPaidConsumesReservationAndReplaysReceiptAfterTerminal(t *testing.T) {
	f := newOrderIntegrationFixture(t, true, true)
	allocation := []points.Allocation{{Source: "recharge", State: "available", Points: 75}, {Source: "winning", State: "available", Points: 25}}
	f.fund(t, allocation)
	order := createOrder(t, f, 100, "paid-order", allocation)
	approved, err := orderAction(t, f, orderID(t, order), "approve", "paid-approve", f.admin, ActionInput{Version: 1, ClientKey: "paid-approve", Reason: "verified"}, points.Metadata{ActorType: "admin", ActorID: f.adminID, RequestID: ids.New()})
	if err != nil || orderState(t, approved) != "processing" {
		t.Fatalf("approve state=%q err=%v", orderState(t, approved), err)
	}
	paid, err := orderAction(t, f, orderID(t, order), "mark_paid", "paid-final", f.admin, ActionInput{Version: 2, ClientKey: "paid-final", Reason: "transfer complete"}, points.Metadata{ActorType: "admin", ActorID: f.adminID, RequestID: ids.New()})
	if err != nil || orderState(t, paid) != "paid" {
		t.Fatalf("mark_paid state=%q err=%v", orderState(t, paid), err)
	}
	var replay Order
	tx, err := f.db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	replay, err = f.service.Advance(context.Background(), tx, orderTestBrand, orderID(t, order), "mark_paid", f.admin, ActionInput{Version: 2, ClientKey: "paid-final", Reason: "transfer complete"}, points.Metadata{ActorType: "admin", ActorID: f.adminID, RequestID: ids.New()})
	if err != nil {
		_ = tx.Rollback(context.Background())
		t.Fatalf("same-key terminal receipt replay: %v", err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	if orderID(t, replay) != orderID(t, paid) || orderState(t, replay) != "paid" {
		t.Fatalf("terminal replay changed receipt: replay=%+v paid=%+v", replay, paid)
	}
	tx, err = f.db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.service.Advance(context.Background(), tx, orderTestBrand, order.ID, "mark_paid", f.admin, ActionInput{Version: 2, ClientKey: "paid-final", Reason: "different transfer claim"}, points.Metadata{ActorType: "admin", ActorID: f.adminID, RequestID: ids.New()})
	_ = tx.Rollback(context.Background())
	if !errors.Is(err, points.ErrConflict) {
		t.Fatalf("same action receipt key with changed body error=%v, want points.ErrConflict", err)
	}
	wallet, err := f.points.Read(context.Background(), orderTestBrand, f.member)
	if err != nil {
		t.Fatal(err)
	}
	if wallet.AvailablePoints != 0 || wallet.WithdrawalPoints != 0 {
		t.Fatalf("paid order should consume reservation: available=%d withdrawal=%d", wallet.AvailablePoints, wallet.WithdrawalPoints)
	}
	createReplay, err := createOrderResult(f, OrderInput{Points: 100, SourceAllocation: allocation, ClientKey: "paid-order"})
	if err != nil || createReplay.ID != order.ID || createReplay.State != "reviewing" || createReplay.Version != 1 {
		t.Fatalf("original create receipt replay after paid=%+v err=%v", createReplay, err)
	}
	if _, err = createOrderResult(f, OrderInput{Points: 99, SourceAllocation: []points.Allocation{{Source: "recharge", State: "available", Points: 74}, {Source: "winning", State: "available", Points: 25}}, ClientKey: "paid-order"}); !errors.Is(err, points.ErrConflict) {
		t.Fatalf("changed create body with original key error=%v, want points.ErrConflict", err)
	}
}

func TestOrderAutomaticReviewReturnsProcessingReceiptWithoutPaying(t *testing.T) {
	f := newOrderIntegrationFixture(t, true, true)
	s := Service{DB: f.db}
	policy, err := s.BrandPolicy(context.Background(), orderTestBrand)
	if err != nil {
		t.Fatal(err)
	}
	config := policy.Config
	config.ReviewMode = "automatic"
	tx, err := f.db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.UpdateBrand(context.Background(), tx, orderTestBrand, f.admin, BrandInput{Version: policy.Version, Config: config, Reason: "test automatic review"}, points.Metadata{ActorType: "admin", ActorID: f.adminID, RequestID: ids.New()})
	if err != nil {
		_ = tx.Rollback(context.Background())
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	allocation := []points.Allocation{{Source: "recharge", State: "available", Points: 60}}
	f.fund(t, allocation)
	in := OrderInput{Points: 60, SourceAllocation: allocation, ClientKey: "automatic-review-order"}
	order, err := createOrderResult(f, in)
	if err != nil {
		t.Fatal(err)
	}
	if order.State != "processing" || order.Version != 2 || order.PaidEntryID != "" {
		t.Fatalf("automatic review result=%+v, want processing v2 without paid entry", order)
	}
	replay, err := createOrderResult(f, in)
	if err != nil || replay.ID != order.ID || replay.State != "processing" || replay.Version != 2 {
		t.Fatalf("automatic-review creation receipt replay=%+v err=%v", replay, err)
	}
	var transitions, payments int
	if err = f.db.QueryRow(context.Background(), `SELECT count(*) FROM withdrawal_order_transitions WHERE order_id=$1`, order.ID).Scan(&transitions); err != nil {
		t.Fatal(err)
	}
	if err = f.db.QueryRow(context.Background(), `SELECT count(*) FROM point_ledger_entries WHERE reference_type='withdrawal' AND reference_id=$1 AND entry_type='withdrawal_paid'`, order.ID).Scan(&payments); err != nil {
		t.Fatal(err)
	}
	if transitions != 2 || payments != 0 {
		t.Fatalf("automatic review transitions/payments=%d/%d, want 2/0", transitions, payments)
	}
}

func TestOrderCreateReceiptReplaysOriginalAfterCancellationAndBodyConflict(t *testing.T) {
	f := newOrderIntegrationFixture(t, true, true)
	allocation := []points.Allocation{{Source: "recharge", State: "available", Points: 30}, {Source: "winning", State: "available", Points: 20}}
	f.fund(t, allocation)
	in := OrderInput{Points: 50, SourceAllocation: allocation, ClientKey: "cancel-receipt-order"}
	order, err := createOrderResult(f, in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = orderAction(t, f, order.ID, "cancel", "cancel-receipt-action", f.admin, ActionInput{Version: 1, ClientKey: "cancel-receipt-action", Reason: "member cancelled"}, points.Metadata{ActorType: "admin", ActorID: f.adminID, RequestID: ids.New()}); err != nil {
		t.Fatal(err)
	}
	replay, err := createOrderResult(f, in)
	if err != nil || replay.ID != order.ID || replay.State != "reviewing" || replay.Version != 1 {
		t.Fatalf("original creation receipt replay=%+v err=%v", replay, err)
	}
	changed := in
	changed.Points = 40
	changed.SourceAllocation = []points.Allocation{{Source: "recharge", State: "available", Points: 20}, {Source: "winning", State: "available", Points: 20}}
	if _, err = createOrderResult(f, changed); !errors.Is(err, points.ErrConflict) {
		t.Fatalf("same-key changed body error=%v, want points.ErrConflict", err)
	}
}

func TestOrderReleaseActionsRestoreOriginalMixedAllocation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		actions []string
		final   string
	}{
		{name: "cancel while reviewing", actions: []string{"cancel"}, final: "cancelled"},
		{name: "cancel while processing", actions: []string{"approve", "cancel"}, final: "cancelled"},
		{name: "fail while processing", actions: []string{"approve", "fail"}, final: "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newOrderIntegrationFixture(t, true, true)
			allocation := []points.Allocation{{Source: "recharge", State: "available", Points: 17}, {Source: "winning", State: "available", Points: 19}, {Source: "gift", State: "available", Points: 23}}
			f.fund(t, allocation)
			order := createOrder(t, f, 59, "release-action-order", allocation)
			state, version := "reviewing", int64(1)
			for i, action := range tc.actions {
				key := fmt.Sprintf("release-action-%s-%c", action, 'a'+i)
				got, actionErr := orderAction(t, f, order.ID, action, key, f.admin, ActionInput{Version: version, ClientKey: key, Reason: "test " + action}, points.Metadata{ActorType: "admin", ActorID: f.adminID, RequestID: ids.New()})
				if actionErr != nil {
					t.Fatal(actionErr)
				}
				state, version = got.State, got.Version
			}
			if state != tc.final {
				t.Fatalf("terminal state=%q, want %q", state, tc.final)
			}
			wallet, readErr := f.points.Read(context.Background(), orderTestBrand, f.member)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if wallet.BySource[0][0] != 17 || wallet.BySource[1][0] != 19 || wallet.BySource[2][0] != 23 || wallet.WithdrawalPoints != 0 {
				t.Fatalf("%s did not restore full original allocation: %+v", tc.name, wallet.BySource)
			}
			var reversalOf string
			if err := f.db.QueryRow(context.Background(), `SELECT reversal_of::text FROM point_ledger_entries WHERE reference_type='withdrawal' AND reference_id=$1 AND entry_type='withdrawal_release'`, order.ID).Scan(&reversalOf); err != nil {
				t.Fatal(err)
			}
			if reversalOf != order.ReserveEntryID {
				t.Fatalf("release reversed %s, want original reservation %s", reversalOf, order.ReserveEntryID)
			}
		})
	}
}

func TestOrderPaidCycleUsesSubmissionBoundaryAndFailedOrderDoesNotAdvanceIt(t *testing.T) {
	f := newOrderIntegrationFixture(t, true, true)
	allocation := []points.Allocation{{Source: "recharge", State: "available", Points: 300}}
	f.fund(t, allocation)
	failed := createOrder(t, f, 50, "cycle-failed-order", oneRechargeAllocation(50))
	if _, err := orderAction(t, f, failed.ID, "approve", "cycle-failed-approve", f.admin, ActionInput{Version: 1, ClientKey: "cycle-failed-approve", Reason: "review"}, points.Metadata{ActorType: "admin", ActorID: f.adminID, RequestID: ids.New()}); err != nil {
		t.Fatal(err)
	}
	if _, err := orderAction(t, f, failed.ID, "fail", "cycle-failed-final", f.admin, ActionInput{Version: 2, ClientKey: "cycle-failed-final", Reason: "transfer failed"}, points.Metadata{ActorType: "admin", ActorID: f.adminID, RequestID: ids.New()}); err != nil {
		t.Fatal(err)
	}
	var cycles int
	if err := f.db.QueryRow(context.Background(), `SELECT count(*) FROM withdrawal_turnover_cycles WHERE brand_id=$1 AND member_id=$2`, orderTestBrand, f.member).Scan(&cycles); err != nil {
		t.Fatal(err)
	}
	if cycles != 0 {
		t.Fatalf("failed withdrawal advanced successful-cycle cursor: rows=%d", cycles)
	}
	paidOrder := createOrder(t, f, 60, "cycle-paid-order", oneRechargeAllocation(60))
	approved, err := orderAction(t, f, paidOrder.ID, "approve", "cycle-paid-approve", f.admin, ActionInput{Version: 1, ClientKey: "cycle-paid-approve", Reason: "review"}, points.Metadata{ActorType: "admin", ActorID: f.adminID, RequestID: ids.New()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := orderAction(t, f, paidOrder.ID, "mark_paid", "cycle-paid-final", f.admin, ActionInput{Version: 2, ClientKey: "cycle-paid-final", Reason: "transfer complete"}, points.Metadata{ActorType: "admin", ActorID: f.adminID, RequestID: ids.New()}); err != nil {
		t.Fatal(err)
	}
	var cutoffAt, submittedAt time.Time
	var cutoffVersion, reserveVersion int64
	var lastOrder string
	if err := f.db.QueryRow(context.Background(), `SELECT cutoff_at,cutoff_version,last_paid_order_id::text FROM withdrawal_turnover_cycles WHERE brand_id=$1 AND member_id=$2`, orderTestBrand, f.member).Scan(&cutoffAt, &cutoffVersion, &lastOrder); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow(context.Background(), `SELECT created_at,reserve_version FROM withdrawal_orders WHERE id=$1`, paidOrder.ID).Scan(&submittedAt, &reserveVersion); err != nil {
		t.Fatal(err)
	}
	if lastOrder != paidOrder.ID || cutoffVersion != reserveVersion || !cutoffAt.Equal(submittedAt) || approved.ReviewedAt == nil || !paidOrder.CreatedAt.Before(*approved.ReviewedAt) {
		t.Fatalf("cycle cursor=(%v,%d,%s), paid submission=(%v,%d,%s)", cutoffAt, cutoffVersion, lastOrder, submittedAt, reserveVersion, paidOrder.ID)
	}
	next := createOrder(t, f, 25, "cycle-next-order", oneRechargeAllocation(25))
	if next.CycleFromAt == nil || !next.CycleFromAt.Equal(paidOrder.CreatedAt) || next.CycleFromVersion != paidOrder.ReserveVersion {
		t.Fatalf("next order turnover boundary=(%v,%d), want (%v,%d)", next.CycleFromAt, next.CycleFromVersion, paidOrder.CreatedAt, paidOrder.ReserveVersion)
	}
}

func TestOrderConcurrentActionsHaveSingleWinner(t *testing.T) {
	for _, tc := range []struct {
		name, initial, actionA, actionB string
		version                         int64
	}{
		{name: "approve versus reject", initial: "reviewing", actionA: "approve", actionB: "reject", version: 1},
		{name: "paid versus fail", initial: "processing", actionA: "mark_paid", actionB: "fail", version: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newOrderIntegrationFixture(t, true, true)
			allocation := []points.Allocation{{Source: "recharge", State: "available", Points: 100}}
			f.fund(t, allocation)
			order := createOrder(t, f, 100, "action-race-order", allocation)
			if tc.initial == "processing" {
				if _, err := orderAction(t, f, order.ID, "approve", "action-race-setup", f.admin, ActionInput{Version: 1, ClientKey: "action-race-setup", Reason: "review"}, points.Metadata{ActorType: "admin", ActorID: f.adminID, RequestID: ids.New()}); err != nil {
					t.Fatal(err)
				}
			}
			start := make(chan struct{})
			results := make(chan error, 2)
			var wg sync.WaitGroup
			for _, action := range []string{tc.actionA, tc.actionB} {
				wg.Add(1)
				go func(action string) {
					defer wg.Done()
					<-start
					key := "action-race-" + action
					input := ActionInput{Version: tc.version, ClientKey: key, Reason: "race " + action}
					tx, err := f.db.Begin(context.Background())
					if err != nil {
						results <- err
						return
					}
					_, err = f.service.Advance(context.Background(), tx, orderTestBrand, order.ID, action, f.admin, input, points.Metadata{ActorType: "admin", ActorID: f.adminID, RequestID: ids.New()})
					if err != nil {
						_ = tx.Rollback(context.Background())
						results <- err
						return
					}
					results <- tx.Commit(context.Background())
				}(action)
			}
			close(start)
			wg.Wait()
			close(results)
			committed, conflicts := 0, 0
			for err := range results {
				if err == nil {
					committed++
				} else if errors.Is(err, ErrVersion) || errors.Is(err, ErrOrderState) {
					conflicts++
				} else {
					t.Errorf("unexpected concurrent action error: %v", err)
				}
			}
			if committed != 1 || conflicts != 1 {
				t.Fatalf("concurrent transition winners/conflicts=%d/%d, want 1/1", committed, conflicts)
			}
		})
	}
}

func TestOrderAdminWritesRequirePersistedExactBrandPermissionAndNeverSuper(t *testing.T) {
	f := newOrderIntegrationFixture(t, true, true)
	allocation := []points.Allocation{{Source: "recharge", State: "available", Points: 40}}
	f.fund(t, allocation)
	order := createOrder(t, f, 40, "permission-order", allocation)
	_, err := orderAction(t, f, order.ID, "approve", "super-must-not-write", f.super, ActionInput{Version: 1, ClientKey: "super-must-not-write", Reason: "try super write"}, points.Metadata{ActorType: "admin", ActorID: f.super.ID, RequestID: ids.New()})
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("super-admin write error=%v, want ErrDenied", err)
	}
	if _, err = f.db.Exec(context.Background(), `DELETE FROM role_permissions WHERE role_id=(SELECT id FROM roles WHERE code LIKE 'withdrawal_test_%') AND permission_key='withdrawal.approve.brand'`); err != nil {
		t.Fatal(err)
	}
	_, err = orderAction(t, f, order.ID, "approve", "view-is-not-write", f.admin, ActionInput{Version: 1, ClientKey: "view-is-not-write", Reason: "try without write grant"}, points.Metadata{ActorType: "admin", ActorID: f.adminID, RequestID: ids.New()})
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("view-only persisted access accepted write: %v", err)
	}
	if _, err = f.service.Read(context.Background(), orderOtherBrand, order.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-brand internal read error=%v, want ErrNotFound", err)
	}
}

func TestOrderActionRollbackLeavesNoProjectionAuditOrReceipt(t *testing.T) {
	f := newOrderIntegrationFixture(t, true, true)
	allocation := []points.Allocation{{Source: "recharge", State: "available", Points: 55}}
	f.fund(t, allocation)
	order := createOrder(t, f, 55, "rollback-order", allocation)
	tx, err := f.db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.service.Advance(context.Background(), tx, orderTestBrand, order.ID, "approve", f.admin, ActionInput{Version: 1, ClientKey: "rollback-approve", Reason: "rollback this"}, points.Metadata{ActorType: "admin", ActorID: f.adminID, RequestID: "withdrawal-rollback-request"})
	if err != nil {
		_ = tx.Rollback(context.Background())
		t.Fatal(err)
	}
	if err = tx.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	current, err := f.service.Read(context.Background(), orderTestBrand, order.ID)
	if err != nil || current.State != "reviewing" || current.Version != 1 {
		t.Fatalf("rolled-back projection=%+v err=%v", current, err)
	}
	var transitions, receipts, audits int
	if err = f.db.QueryRow(context.Background(), `SELECT count(*) FROM withdrawal_order_transitions WHERE order_id=$1`, order.ID).Scan(&transitions); err != nil {
		t.Fatal(err)
	}
	if err = f.db.QueryRow(context.Background(), `SELECT count(*) FROM withdrawal_operation_receipts WHERE order_id=$1 AND client_key='rollback-approve'`, order.ID).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if err = f.db.QueryRow(context.Background(), `SELECT count(*) FROM audit_logs WHERE resource_type='withdrawal_order' AND resource_id=$1 AND request_id='withdrawal-rollback-request'`, order.ID).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if transitions != 1 || receipts != 0 || audits != 0 {
		t.Fatalf("rollback left transition/receipt/audit counts %d/%d/%d, want 1/0/0", transitions, receipts, audits)
	}
}

func TestOrderSameKeyAndActiveMemberConcurrency(t *testing.T) {
	f := newOrderIntegrationFixture(t, true, true)
	allocation := []points.Allocation{{Source: "recharge", State: "available", Points: 500}}
	f.fund(t, allocation)
	const workers = 12
	start := make(chan struct{})
	var wg sync.WaitGroup
	orders := make(chan Order, workers)
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			tx, err := f.db.Begin(context.Background())
			if err != nil {
				errs <- err
				return
			}
			defer tx.Rollback(context.Background())
			out, err := f.service.Create(context.Background(), tx, orderTestBrand, f.member, OrderInput{Points: 25, SourceAllocation: oneRechargeAllocation(25), ClientKey: "one-concurrent-create"}, f.actor)
			if err != nil {
				errs <- err
				return
			}
			if err = tx.Commit(context.Background()); err != nil {
				errs <- err
				return
			}
			orders <- out
		}()
	}
	close(start)
	wg.Wait()
	close(orders)
	close(errs)
	for err := range errs {
		t.Errorf("concurrent same-key create: %v", err)
	}
	var first string
	for got := range orders {
		if first == "" {
			first = orderID(t, got)
		} else if orderID(t, got) != first {
			t.Errorf("same key returned different receipts: %s != %s", orderID(t, got), first)
		}
	}
	var count, reserves int
	if err := f.db.QueryRow(context.Background(), `SELECT count(*) FROM withdrawal_orders WHERE brand_id=$1 AND member_id=$2`, orderTestBrand, f.member).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow(context.Background(), `SELECT count(*) FROM point_ledger_entries WHERE reference_type='withdrawal' AND entry_type='withdrawal_reserve' AND reference_id=(SELECT id FROM withdrawal_orders WHERE brand_id=$1 AND member_id=$2)`, orderTestBrand, f.member).Scan(&reserves); err != nil {
		t.Fatal(err)
	}
	if count != 1 || reserves != 1 {
		t.Fatalf("concurrent same-key requests created %d orders and %d reserves, want 1/1", count, reserves)
	}
}

func TestOrderDifferentKeyConcurrentCreatesEnforceSingleActiveOrder(t *testing.T) {
	f := newOrderIntegrationFixture(t, true, true)
	f.fund(t, []points.Allocation{{Source: "recharge", State: "available", Points: 500}})
	const workers = 12
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			key := fmt.Sprintf("distinct-create-%02d", i)
			tx, err := f.db.Begin(context.Background())
			if err != nil {
				results <- err
				return
			}
			_, err = f.service.Create(context.Background(), tx, orderTestBrand, f.member, OrderInput{Points: 25, SourceAllocation: oneRechargeAllocation(25), ClientKey: key}, f.actor)
			if err != nil {
				_ = tx.Rollback(context.Background())
				results <- err
				return
			}
			results <- tx.Commit(context.Background())
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)
	created, activeConflicts := 0, 0
	for err := range results {
		if err == nil {
			created++
		} else if errors.Is(err, ErrActiveOrder) || errors.Is(err, points.ErrConflict) {
			activeConflicts++
		} else {
			t.Errorf("unexpected concurrent create error: %v", err)
		}
	}
	var count int
	if err := f.db.QueryRow(context.Background(), `SELECT count(*) FROM withdrawal_orders WHERE brand_id=$1 AND member_id=$2 AND state IN('reviewing','processing')`, orderTestBrand, f.member).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if created != 1 || count != 1 || activeConflicts != workers-1 {
		t.Fatalf("distinct-key concurrent creates: committed=%d conflicts=%d activeRows=%d, want 1/%d/1", created, activeConflicts, count, workers-1)
	}
}

func TestOrderNilOrDenyEligibilityDoesNotReserve(t *testing.T) {
	f := newOrderIntegrationFixture(t, true, false)
	allocation := []points.Allocation{{Source: "recharge", State: "available", Points: 50}}
	f.fund(t, allocation)
	tx, err := f.db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.service.Create(context.Background(), tx, orderTestBrand, f.member, OrderInput{Points: 50, SourceAllocation: allocation, ClientKey: "eligibility-denied"}, f.actor)
	_ = tx.Rollback(context.Background())
	if !errors.Is(err, ErrIneligible) {
		t.Fatalf("denied eligibility error=%v, want ErrIneligible", err)
	}
	wallet, readErr := f.points.Read(context.Background(), orderTestBrand, f.member)
	if readErr != nil || wallet.AvailablePoints != 50 || wallet.WithdrawalPoints != 0 {
		t.Fatalf("denied eligibility changed wallet: wallet=%+v err=%v", wallet, readErr)
	}
	f.service.Eligibility = nil
	tx, err = f.db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, nilErr := f.service.Create(context.Background(), tx, orderTestBrand, f.member, OrderInput{Points: 50, SourceAllocation: allocation, ClientKey: "eligibility-missing"}, f.actor)
	_ = tx.Rollback(context.Background())
	if !errors.Is(nilErr, ErrEligibilityNotConfigured) {
		t.Fatalf("nil eligibility checker error=%v, want ErrEligibilityNotConfigured", nilErr)
	}
	wallet, readErr = f.points.Read(context.Background(), orderTestBrand, f.member)
	if readErr != nil || wallet.AvailablePoints != 50 || wallet.WithdrawalPoints != 0 {
		t.Fatalf("missing eligibility checker changed wallet: wallet=%+v err=%v", wallet, readErr)
	}
}

func TestOrderBrandStatusAndPolicyLimits(t *testing.T) {
	cap := points.Amount(40)
	f := newOrderIntegrationFixture(t, true, true)
	f.updatePolicy(t, true, &cap)
	allocation := []points.Allocation{{Source: "recharge", State: "available", Points: 100}}
	f.fund(t, allocation)
	if _, err := createOrderResult(f, OrderInput{Points: 50, SourceAllocation: oneRechargeAllocation(50), ClientKey: "over-policy-cap"}); !errors.Is(err, ErrIneligible) {
		t.Fatalf("order above policy maximum error=%v, want ErrIneligible", err)
	}
	if _, err := f.db.Exec(context.Background(), `UPDATE brands SET status='paused' WHERE id=$1`, orderTestBrand); err != nil {
		t.Fatal(err)
	}
	if _, err := createOrderResult(f, OrderInput{Points: 25, SourceAllocation: oneRechargeAllocation(25), ClientKey: "paused-brand-order"}); err != nil {
		t.Fatalf("paused brand should allow existing-user withdrawal: %v", err)
	}
	if _, err := f.db.Exec(context.Background(), `UPDATE brands SET status='disabled' WHERE id=$1`, orderTestBrand); err != nil {
		t.Fatal(err)
	}
	if _, err := createOrderResult(f, OrderInput{Points: 25, SourceAllocation: oneRechargeAllocation(25), ClientKey: "disabled-brand-order"}); !errors.Is(err, ErrDenied) {
		t.Fatalf("disabled brand error=%v, want ErrDenied", err)
	}
}

func createOrderResult(f orderIntegrationFixture, in OrderInput) (Order, error) {
	tx, err := f.db.Begin(context.Background())
	if err != nil {
		return Order{}, err
	}
	defer tx.Rollback(context.Background())
	out, err := f.service.Create(context.Background(), tx, orderTestBrand, f.member, in, f.actor)
	if err != nil {
		return Order{}, err
	}
	if err = tx.Commit(context.Background()); err != nil {
		return Order{}, err
	}
	return out, nil
}

func oneRechargeAllocation(amount points.Amount) []points.Allocation {
	return []points.Allocation{{Source: "recharge", State: "available", Points: amount}}
}
