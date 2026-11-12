package withdrawal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

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
	if _, err := db.Exec(ctx, `INSERT INTO brand_members(id,brand_id,global_user_id,join_method) VALUES($1,$2,$3,'domain')`, memberID, orderTestBrand, userID); err != nil {
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
	_, err = s.UpdateBrand(context.Background(), tx, orderTestBrand, f.admin, BrandInput{PointsStep: 100, Version: policy.Version, Config: config, Reason: "integration fixture policy"}, points.Metadata{ActorType: "admin", ActorID: f.adminID, RequestID: ids.New()})
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
	allocation := []points.Allocation{{Source: "recharge", State: "available", Points: 4000}, {Source: "winning", State: "available", Points: 3000}, {Source: "gift", State: "available", Points: 2000}}
	f.fund(t, allocation)
	order := createOrder(t, f, 9000, "mixed-source-order", allocation)
	if got := orderState(t, order); got != "reviewing" {
		t.Fatalf("new order state=%q, want reviewing", got)
	}
	wallet, err := f.points.Read(context.Background(), orderTestBrand, f.member)
	if err != nil {
		t.Fatal(err)
	}
	if wallet.AvailablePoints != 0 || wallet.WithdrawalPoints != 9000 {
		t.Fatalf("reservation wallet available/withdrawal=%d/%d, want 0/90", wallet.AvailablePoints, wallet.WithdrawalPoints)
	}
	if _, err = orderAction(t, f, orderID(t, order), "reject", "mixed-reject", f.admin, ActionInput{Version: 1, ClientKey: "mixed-reject", Reason: "review declined"}, points.Metadata{ActorType: "admin", ActorID: f.adminID, RequestID: ids.New()}); err != nil {
		t.Fatal(err)
	}
	wallet, err = f.points.Read(context.Background(), orderTestBrand, f.member)
	if err != nil {
		t.Fatal(err)
	}
	if wallet.BySource[0][0] != 4000 || wallet.BySource[1][0] != 3000 || wallet.BySource[2][0] != 2000 || wallet.WithdrawalPoints != 0 {
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
	policy, err = policyService.UpdateBrand(context.Background(), tx, orderTestBrand, f.admin, BrandInput{PointsStep: 100, Version: policy.Version, Config: configured, Reason: "explicitly authorize commission withdrawals"}, points.Metadata{ActorType: "admin", ActorID: f.adminID, RequestID: ids.New()})
	if err != nil {
		_ = tx.Rollback(context.Background())
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	commissionAllocation := []points.Allocation{{Source: "commission", State: "available", Points: 10000}}
	f.fund(t, commissionAllocation)
	first := createOrder(t, f, 2500, "commission-refund-order", []points.Allocation{{Source: "commission", State: "available", Points: 2500}})
	if fmt.Sprint(first.PolicySnapshot.Config.AllowedSources) != fmt.Sprint(configured.AllowedSources) {
		t.Fatalf("application lost its configured policy snapshot: %+v", first.PolicySnapshot)
	}
	rejected, err := orderAction(t, f, first.ID, "reject", "commission-refund-reject", f.admin, ActionInput{Version: 1, ClientKey: "commission-refund-reject", Reason: "refund commission allocation"}, points.Metadata{ActorType: "admin", ActorID: f.adminID, RequestID: ids.New()})
	if err != nil || rejected.State != "rejected" {
		t.Fatalf("commission source refund failed: %+v err=%v", rejected, err)
	}
	wallet, err := f.points.Read(context.Background(), orderTestBrand, f.member)
	if err != nil || wallet.BySource[3][0] != 10000 {
		t.Fatalf("commission refund did not restore exact source: %+v err=%v", wallet.BySource, err)
	}
	paidOrder := createOrder(t, f, 4000, "commission-paid-order", []points.Allocation{{Source: "commission", State: "available", Points: 4000}})
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
	if _, err = policyService.UpdateBrand(context.Background(), tx, orderTestBrand, f.admin, BrandInput{PointsStep: 100, Version: current.Version, Config: currentConfig, Reason: "remove commission for future orders"}, points.Metadata{ActorType: "admin", ActorID: f.adminID, RequestID: ids.New()}); err != nil {
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
	if err != nil || wallet.BySource[3][0] != 6000 || wallet.WithdrawalPoints != 0 {
		t.Fatalf("paid commission source balance=%+v err=%v", wallet.BySource, err)
	}
}

func TestOrderPaidConsumesReservationAndReplaysReceiptAfterTerminal(t *testing.T) {
	f := newOrderIntegrationFixture(t, true, true)
	allocation := []points.Allocation{{Source: "recharge", State: "available", Points: 7500}, {Source: "winning", State: "available", Points: 2500}}
	f.fund(t, allocation)
	order := createOrder(t, f, 10000, "paid-order", allocation)
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
	createReplay, err := createOrderResult(f, OrderInput{Points: 10000, SourceAllocation: allocation, ClientKey: "paid-order"})
	if err != nil || createReplay.ID != order.ID || createReplay.State != "reviewing" || createReplay.Version != 1 {
		t.Fatalf("original create receipt replay after paid=%+v err=%v", createReplay, err)
	}
	if _, err = createOrderResult(f, OrderInput{Points: 9900, SourceAllocation: []points.Allocation{{Source: "recharge", State: "available", Points: 7400}, {Source: "winning", State: "available", Points: 2500}}, ClientKey: "paid-order"}); !errors.Is(err, points.ErrConflict) {
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
	_, err = s.UpdateBrand(context.Background(), tx, orderTestBrand, f.admin, BrandInput{PointsStep: 100, Version: policy.Version, Config: config, Reason: "test automatic review"}, points.Metadata{ActorType: "admin", ActorID: f.adminID, RequestID: ids.New()})
	if err != nil {
		_ = tx.Rollback(context.Background())
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	allocation := []points.Allocation{{Source: "recharge", State: "available", Points: 6000}}
	f.fund(t, allocation)
	in := OrderInput{Points: 6000, SourceAllocation: allocation, ClientKey: "automatic-review-order"}
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
	allocation := []points.Allocation{{Source: "recharge", State: "available", Points: 3000}, {Source: "winning", State: "available", Points: 2000}}
	f.fund(t, allocation)
	in := OrderInput{Points: 5000, SourceAllocation: allocation, ClientKey: "cancel-receipt-order"}
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
	changed.Points = 4000
	changed.SourceAllocation = []points.Allocation{{Source: "recharge", State: "available", Points: 2000}, {Source: "winning", State: "available", Points: 2000}}
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
			allocation := []points.Allocation{{Source: "recharge", State: "available", Points: 1700}, {Source: "winning", State: "available", Points: 1900}, {Source: "gift", State: "available", Points: 2300}}
			f.fund(t, allocation)
			order := createOrder(t, f, 5900, "release-action-order", allocation)
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
			if wallet.BySource[0][0] != 1700 || wallet.BySource[1][0] != 1900 || wallet.BySource[2][0] != 2300 || wallet.WithdrawalPoints != 0 {
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
	allocation := []points.Allocation{{Source: "recharge", State: "available", Points: 30000}}
	f.fund(t, allocation)
	failed := createOrder(t, f, 5000, "cycle-failed-order", oneRechargeAllocation(5000))
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
	paidOrder := createOrder(t, f, 6000, "cycle-paid-order", oneRechargeAllocation(6000))
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
	next := createOrder(t, f, 2500, "cycle-next-order", oneRechargeAllocation(2500))
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
			allocation := []points.Allocation{{Source: "recharge", State: "available", Points: 10000}}
			f.fund(t, allocation)
			order := createOrder(t, f, 10000, "action-race-order", allocation)
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
	allocation := []points.Allocation{{Source: "recharge", State: "available", Points: 4000}}
	f.fund(t, allocation)
	order := createOrder(t, f, 4000, "permission-order", allocation)
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
	allocation := []points.Allocation{{Source: "recharge", State: "available", Points: 5500}}
	f.fund(t, allocation)
	order := createOrder(t, f, 5500, "rollback-order", allocation)
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
	allocation := []points.Allocation{{Source: "recharge", State: "available", Points: 50000}}
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
			out, err := f.service.Create(context.Background(), tx, orderTestBrand, f.member, OrderInput{Points: 2500, SourceAllocation: oneRechargeAllocation(2500), ClientKey: "one-concurrent-create"}, f.actor)
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
	f.fund(t, []points.Allocation{{Source: "recharge", State: "available", Points: 50000}})
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
			_, err = f.service.Create(context.Background(), tx, orderTestBrand, f.member, OrderInput{Points: 2500, SourceAllocation: oneRechargeAllocation(2500), ClientKey: key}, f.actor)
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
	allocation := []points.Allocation{{Source: "recharge", State: "available", Points: 5000}}
	f.fund(t, allocation)
	tx, err := f.db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.service.Create(context.Background(), tx, orderTestBrand, f.member, OrderInput{Points: 5000, SourceAllocation: allocation, ClientKey: "eligibility-denied"}, f.actor)
	_ = tx.Rollback(context.Background())
	if !errors.Is(err, ErrIneligible) {
		t.Fatalf("denied eligibility error=%v, want ErrIneligible", err)
	}
	wallet, readErr := f.points.Read(context.Background(), orderTestBrand, f.member)
	if readErr != nil || wallet.AvailablePoints != 5000 || wallet.WithdrawalPoints != 0 {
		t.Fatalf("denied eligibility changed wallet: wallet=%+v err=%v", wallet, readErr)
	}
	f.service.Eligibility = nil
	tx, err = f.db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, nilErr := f.service.Create(context.Background(), tx, orderTestBrand, f.member, OrderInput{Points: 5000, SourceAllocation: allocation, ClientKey: "eligibility-missing"}, f.actor)
	_ = tx.Rollback(context.Background())
	if !errors.Is(nilErr, ErrEligibilityNotConfigured) {
		t.Fatalf("nil eligibility checker error=%v, want ErrEligibilityNotConfigured", nilErr)
	}
	wallet, readErr = f.points.Read(context.Background(), orderTestBrand, f.member)
	if readErr != nil || wallet.AvailablePoints != 5000 || wallet.WithdrawalPoints != 0 {
		t.Fatalf("missing eligibility checker changed wallet: wallet=%+v err=%v", wallet, readErr)
	}
}

func TestOrderBrandStatusAndPolicyLimits(t *testing.T) {
	cap := points.Amount(4000)
	f := newOrderIntegrationFixture(t, true, true)
	f.updatePolicy(t, true, &cap)
	allocation := []points.Allocation{{Source: "recharge", State: "available", Points: 10000}}
	f.fund(t, allocation)
	if _, err := createOrderResult(f, OrderInput{Points: 5000, SourceAllocation: oneRechargeAllocation(5000), ClientKey: "over-policy-cap"}); !errors.Is(err, ErrIneligible) {
		t.Fatalf("order above policy maximum error=%v, want ErrIneligible", err)
	}
	if _, err := f.db.Exec(context.Background(), `UPDATE brands SET status='paused' WHERE id=$1`, orderTestBrand); err != nil {
		t.Fatal(err)
	}
	if _, err := createOrderResult(f, OrderInput{Points: 2500, SourceAllocation: oneRechargeAllocation(2500), ClientKey: "paused-brand-order"}); err != nil {
		t.Fatalf("paused brand should allow existing-user withdrawal: %v", err)
	}
	if _, err := f.db.Exec(context.Background(), `UPDATE brands SET status='disabled' WHERE id=$1`, orderTestBrand); err != nil {
		t.Fatal(err)
	}
	if _, err := createOrderResult(f, OrderInput{Points: 2500, SourceAllocation: oneRechargeAllocation(2500), ClientKey: "disabled-brand-order"}); !errors.Is(err, ErrDenied) {
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

func TestWithdrawalStepAndAvailableSources(t *testing.T) {
	f := newOrderIntegrationFixture(t, true, true)
	ctx := context.Background()
	f.fund(t, []points.Allocation{{Source: "recharge", State: "available", Points: 50000}, {Source: "commission", State: "available", Points: 20000}})
	policies := Service{DB: f.db}
	policy, err := policies.BrandPolicy(ctx, orderTestBrand)
	if err != nil {
		t.Fatal(err)
	}
	policy.Config.MinPoints = 10000
	transact(t, policies, func(tx pgx.Tx) error {
		_, e := policies.UpdateBrand(ctx, tx, orderTestBrand, f.admin, BrandInput{Version: policy.Version, PointsStep: 10000, Config: policy.Config}, points.Metadata{ActorType: "admin", ActorID: f.adminID, RequestID: ids.New()})
		return e
	})
	tx, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	available, err := f.service.AvailabilityTx(ctx, tx, orderTestBrand, f.member)
	tx.Rollback(ctx)
	if err != nil || available.PointsStep != 10000 || available.AvailablePoints != 50000 || len(available.SourceBalances) != 1 {
		t.Fatalf("availability=%+v error=%v", available, err)
	}
	for _, amount := range []points.Amount{5000, 15000, 60000} {
		_, err = createOrderResult(f, OrderInput{Points: amount, SourceAllocation: oneRechargeAllocation(amount), ClientKey: fmt.Sprintf("step-reject-%d", amount)})
		if amount == 60000 {
			if !errors.Is(err, points.ErrInsufficient) {
				t.Fatal(err)
			}
		} else if !errors.Is(err, ErrIneligible) {
			t.Fatal(err)
		}
	}
	wallet, err := f.points.Read(ctx, orderTestBrand, f.member)
	if err != nil || wallet.AvailablePoints != 70000 || wallet.WithdrawalPoints != 0 {
		t.Fatalf("invalid request reserved points: %+v %v", wallet, err)
	}
	accepted := createOrder(t, f, 20000, "step-accepted", oneRechargeAllocation(20000))
	currentPolicy, err := policies.BrandPolicy(ctx, orderTestBrand)
	if err != nil {
		t.Fatal(err)
	}
	transact(t, policies, func(tx pgx.Tx) error {
		_, e := policies.UpdateBrand(ctx, tx, orderTestBrand, f.admin, BrandInput{Version: currentPolicy.Version, PointsStep: 100000, Config: currentPolicy.Config}, points.Metadata{ActorType: "admin", ActorID: f.adminID, RequestID: ids.New()})
		return e
	})

	replay, err := createOrderResult(f, OrderInput{Points: 20000, SourceAllocation: oneRechargeAllocation(20000), ClientKey: "step-accepted"})
	if err != nil || replay.ID != accepted.ID {
		t.Fatalf("receipt replay=%+v %v", replay, err)
	}
	wallet, err = f.points.Read(ctx, orderTestBrand, f.member)
	if err != nil || wallet.AvailablePoints != 50000 || wallet.WithdrawalPoints != 20000 {
		t.Fatalf("withdrawal reserved more than once: %+v %v", wallet, err)
	}
}
