package points

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
)

func amountLimit(value Amount) *Amount { return &value }

func balanceTotalForPolicy(values ...Amount) Balance {
	var balance Balance
	for i, value := range values {
		balance[i/4][i%4] = value
	}
	return balance
}

func TestPolicyInputRequiresCompleteCanonicalReplacementJSON(t *testing.T) {
	valid := `{"version":3,"max_balance_points":"1000","max_recharge_points":null,"max_adjustment_points":"25","reason":"update policy"}`
	var input PolicyInput
	if err := json.Unmarshal([]byte(valid), &input); err != nil {
		t.Fatal(err)
	}
	if input.Version != 3 || input.MaxBalancePoints == nil || *input.MaxBalancePoints != 1000 || input.MaxRechargePoints != nil || input.MaxAdjustmentPoints == nil || *input.MaxAdjustmentPoints != 25 || input.Reason != "update policy" {
		t.Fatalf("decoded policy input=%+v", input)
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var roundTrip PolicyInput
	if err = json.Unmarshal(encoded, &roundTrip); err != nil {
		t.Fatalf("ordinary struct Marshal output was rejected: %s: %v", encoded, err)
	}
	if roundTrip.Version != input.Version || roundTrip.Reason != input.Reason || roundTrip.MaxBalancePoints == nil || *roundTrip.MaxBalancePoints != *input.MaxBalancePoints || roundTrip.MaxRechargePoints != nil || roundTrip.MaxAdjustmentPoints == nil || *roundTrip.MaxAdjustmentPoints != *input.MaxAdjustmentPoints {
		t.Fatalf("policy input round trip differs: got=%+v want=%+v", roundTrip, input)
	}
	invalid := []struct {
		name string
		json string
	}{
		{name: "missing version", json: `{"max_balance_points":null,"max_recharge_points":null,"max_adjustment_points":null,"reason":"x"}`},
		{name: "missing reason", json: `{"version":1,"max_balance_points":null,"max_recharge_points":null,"max_adjustment_points":null}`},
		{name: "missing balance cap", json: `{"version":1,"max_recharge_points":null,"max_adjustment_points":null,"reason":"x"}`},
		{name: "missing recharge cap", json: `{"version":1,"max_balance_points":null,"max_adjustment_points":null,"reason":"x"}`},
		{name: "missing adjustment cap", json: `{"version":1,"max_balance_points":null,"max_recharge_points":null,"reason":"x"}`},
		{name: "unknown field", json: `{"version":1,"max_balance_points":null,"max_recharge_points":null,"max_adjustment_points":null,"reason":"x","extra":1}`},
		{name: "duplicate key", json: `{"version":1,"version":1,"max_balance_points":null,"max_recharge_points":null,"max_adjustment_points":null,"reason":"x"}`},
		{name: "duplicate cap", json: `{"version":1,"max_balance_points":null,"max_balance_points":null,"max_recharge_points":null,"max_adjustment_points":null,"reason":"x"}`},
		{name: "numeric cap", json: `{"version":1,"max_balance_points":100,"max_recharge_points":null,"max_adjustment_points":null,"reason":"x"}`},
		{name: "noncanonical cap", json: `{"version":1,"max_balance_points":"0100","max_recharge_points":null,"max_adjustment_points":null,"reason":"x"}`},
		{name: "null version", json: `{"version":null,"max_balance_points":null,"max_recharge_points":null,"max_adjustment_points":null,"reason":"x"}`},
		{name: "null reason", json: `{"version":1,"max_balance_points":null,"max_recharge_points":null,"max_adjustment_points":null,"reason":null}`},
		{name: "trailing value", json: valid + ` true`},
		{name: "null object", json: `null`},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			var got PolicyInput
			err := json.Unmarshal([]byte(tc.json), &got)
			if err == nil {
				t.Fatalf("accepted invalid replacement body: %s", tc.json)
			}
			if tc.name != "trailing value" && !errors.Is(err, ErrInvalid) {
				t.Fatalf("error=%v, want ErrInvalid", err)
			}
		})
	}
}

func TestPolicyCheckBalanceIncreaseCapAndReversal(t *testing.T) {
	policy := Policy{MaxBalancePoints: amountLimit(100)}
	before := balanceTotalForPolicy(100)
	decrease := balanceTotalForPolicy(95)
	if err := policy.CheckBalance(before, decrease, false); err != nil {
		t.Fatalf("decrease from an already capped balance: %v", err)
	}
	unchanged := before
	if err := policy.CheckBalance(before, unchanged, false); err != nil {
		t.Fatalf("balance-neutral movement: %v", err)
	}
	increase := balanceTotalForPolicy(101)
	if err := policy.CheckBalance(before, increase, false); !errors.Is(err, ErrPolicyLimit) {
		t.Fatalf("over-cap net increase error=%v, want ErrPolicyLimit", err)
	}
	if err := policy.CheckBalance(before, increase, true); err != nil {
		t.Fatalf("reversal should bypass the balance cap: %v", err)
	}
	if err := policy.CheckBalance(Balance{}, increase, false); !errors.Is(err, ErrPolicyLimit) {
		t.Fatalf("new over-cap balance error=%v, want ErrPolicyLimit", err)
	}
	invalid := Balance{}
	invalid[0][0] = -1
	if err := policy.CheckBalance(Balance{}, invalid, true); !errors.Is(err, ErrInvalid) {
		t.Fatalf("reversal bypassed balance validity: %v", err)
	}
	overflow := Balance{}
	overflow[0][0], overflow[0][1] = Amount(math.MaxInt64), 1
	if err := (Policy{}).CheckBalance(Balance{}, overflow, false); !errors.Is(err, ErrOverflow) {
		t.Fatalf("total overflow error=%v, want ErrOverflow", err)
	}
}

func TestPolicyCheckRechargeAndAdjustmentPerOperationCaps(t *testing.T) {
	policy := Policy{MaxRechargePoints: amountLimit(50), MaxAdjustmentPoints: amountLimit(30)}
	if err := policy.CheckRecharge(50); err != nil {
		t.Fatal("exact recharge cap rejected:", err)
	}
	if err := policy.CheckRecharge(51); !errors.Is(err, ErrPolicyLimit) {
		t.Fatalf("recharge above cap error=%v", err)
	}
	for _, amount := range []Amount{0, -1} {
		if err := policy.CheckRecharge(amount); !errors.Is(err, ErrInvalid) {
			t.Errorf("CheckRecharge(%d) error=%v, want ErrInvalid", amount, err)
		}
	}
	var within, above Balance
	within[0][0], within[1][2] = 15, -15
	if err := policy.CheckAdjustment(within); err != nil {
		t.Fatalf("adjustment with 30 absolute points rejected: %v", err)
	}
	above[0][0], above[1][2] = 16, -15
	if err := policy.CheckAdjustment(above); !errors.Is(err, ErrPolicyLimit) {
		t.Fatalf("adjustment above per-operation cap error=%v", err)
	}
	var min Balance
	min[0][0] = Amount(math.MinInt64)
	if err := (Policy{}).CheckAdjustment(min); !errors.Is(err, ErrOverflow) {
		t.Fatalf("MinInt64 absolute adjustment error=%v, want ErrOverflow", err)
	}
	var sumOverflow Balance
	sumOverflow[0][0], sumOverflow[1][0] = Amount(math.MaxInt64), 1
	if err := (Policy{}).CheckAdjustment(sumOverflow); !errors.Is(err, ErrOverflow) {
		t.Fatalf("absolute adjustment sum overflow error=%v", err)
	}
	if err := policy.CheckAdjustment(Balance{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("zero adjustment error=%v", err)
	}
	if err := (Policy{}).CheckRecharge(Amount(math.MaxInt64)); err != nil {
		t.Fatalf("unlimited recharge unexpectedly rejected: %v", err)
	}
}

func TestPolicyReadUpdateCASAuditAndBrandIsolation(t *testing.T) {
	p := testdb.New(t)
	store := Store{DB: p}
	ctx := context.Background()
	initial, err := store.ReadPolicy(ctx, testBrand)
	if err != nil {
		t.Fatal(err)
	}
	if initial.BrandID != testBrand || initial.Version != 1 || initial.MaxBalancePoints != nil || initial.MaxRechargePoints != nil || initial.MaxAdjustmentPoints != nil {
		t.Fatalf("unexpected seeded default policy: %+v", initial)
	}
	other, err := store.ReadPolicy(ctx, "0199a000-0000-7000-8000-000000000002")
	if err != nil || other.Version != 1 || other.BrandID == initial.BrandID {
		t.Fatalf("other brand policy=%+v err=%v", other, err)
	}
	newBrand := ids.New()
	if _, err = p.Exec(ctx, "INSERT INTO brands(id,code,name,status) VALUES($1,$2,'Policy Trigger Test','active')", newBrand, "policy-"+strings.ReplaceAll(newBrand, "-", "")); err != nil {
		t.Fatal(err)
	}
	triggerDefault, err := store.ReadPolicy(ctx, newBrand)
	if err != nil || triggerDefault.Version != 1 || triggerDefault.MaxBalancePoints != nil || triggerDefault.MaxRechargePoints != nil || triggerDefault.MaxAdjustmentPoints != nil {
		t.Fatalf("new-brand policy trigger did not produce an unlimited version-1 row: policy=%+v err=%v", triggerDefault, err)
	}
	adminID := ids.New()
	if _, err = p.Exec(ctx, "INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'test-only-hash')", adminID, "policy_admin_"+adminID); err != nil {
		t.Fatal(err)
	}
	input := PolicyInput{Version: 1, MaxBalancePoints: amountLimit(900), MaxRechargePoints: amountLimit(300), MaxAdjustmentPoints: amountLimit(100), Reason: "  set brand point caps  "}
	meta := Metadata{ActorType: "admin", ActorID: adminID, RequestID: "policy-update-test", IP: "127.0.0.1"}
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := store.UpdatePolicy(ctx, tx, testBrand, input, meta)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if updated.Version != 2 || updated.AuditLogID == "" || updated.MaxBalancePoints == nil || *updated.MaxBalancePoints != 900 || updated.MaxRechargePoints == nil || *updated.MaxRechargePoints != 300 || updated.MaxAdjustmentPoints == nil || *updated.MaxAdjustmentPoints != 100 {
		t.Fatalf("unexpected updated policy: %+v", updated)
	}
	readBack, err := store.ReadPolicy(ctx, testBrand)
	if err != nil || readBack.Version != 2 || readBack.MaxBalancePoints == nil || *readBack.MaxBalancePoints != 900 {
		t.Fatalf("policy did not persist: policy=%+v err=%v", readBack, err)
	}
	otherAfter, err := store.ReadPolicy(ctx, other.BrandID)
	if err != nil || otherAfter.Version != 1 || otherAfter.MaxBalancePoints != nil || otherAfter.MaxRechargePoints != nil || otherAfter.MaxAdjustmentPoints != nil {
		t.Fatalf("update leaked to other brand: policy=%+v err=%v", otherAfter, err)
	}
	var actor, action, reason, beforeJSON, afterJSON string
	if err = p.QueryRow(ctx, "SELECT actor_id::text,action,reason,before_json::text,after_json::text FROM audit_logs WHERE id=$1", updated.AuditLogID).Scan(&actor, &action, &reason, &beforeJSON, &afterJSON); err != nil {
		t.Fatal(err)
	}
	if actor != adminID || action != "points.policy.update" || reason != "set brand point caps" || beforeJSON == "null" || afterJSON == "null" {
		t.Fatalf("unexpected policy audit actor=%s action=%s reason=%q before=%s after=%s", actor, action, reason, beforeJSON, afterJSON)
	}

	staleTx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	staleInput := input
	staleInput.MaxBalancePoints = amountLimit(950)
	stale, err := store.UpdatePolicy(ctx, staleTx, testBrand, staleInput, meta)
	_ = staleTx.Rollback(ctx)
	if !errors.Is(err, ErrConflict) || stale.Version != 0 {
		t.Fatalf("stale update policy=%+v err=%v, want ErrConflict", stale, err)
	}
}

func TestPolicyUpdateRejectsInvalidCapsAndReason(t *testing.T) {
	p := testdb.New(t)
	store := Store{DB: p}
	ctx := context.Background()
	adminID := ids.New()
	if _, err := p.Exec(ctx, "INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'test-only-hash')", adminID, "policy_invalid_"+adminID); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		input PolicyInput
	}{
		{name: "zero cap", input: PolicyInput{Version: 1, MaxRechargePoints: amountLimit(0), Reason: "valid reason"}},
		{name: "negative cap", input: PolicyInput{Version: 1, MaxAdjustmentPoints: amountLimit(-1), Reason: "valid reason"}},
		{name: "empty reason", input: PolicyInput{Version: 1, Reason: "  "}},
		{name: "long reason", input: PolicyInput{Version: 1, Reason: string(make([]byte, 501))}},
		{name: "invalid version", input: PolicyInput{Version: 0, Reason: "valid reason"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx, err := p.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			_, err = store.UpdatePolicy(ctx, tx, testBrand, tc.input, Metadata{ActorType: "admin", ActorID: adminID, RequestID: "invalid-policy-test"})
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("invalid policy update error=%v, want ErrInvalid", err)
			}
		})
	}
	validInput := PolicyInput{Version: 1, Reason: "valid policy change"}
	for _, tc := range []struct {
		name string
		meta Metadata
	}{
		{name: "non-admin actor", meta: Metadata{ActorType: "user", ActorID: adminID, RequestID: "request-1"}},
		{name: "missing actor id", meta: Metadata{ActorType: "admin", RequestID: "request-1"}},
		{name: "invalid actor uuid", meta: Metadata{ActorType: "admin", ActorID: "not-a-uuid", RequestID: "request-1"}},
		{name: "missing request id", meta: Metadata{ActorType: "admin", ActorID: adminID}},
		{name: "request id too long", meta: Metadata{ActorType: "admin", ActorID: adminID, RequestID: strings.Repeat("x", 81)}},
		{name: "request id invalid utf8", meta: Metadata{ActorType: "admin", ActorID: adminID, RequestID: string([]byte{0xff})}},
	} {
		t.Run("audit metadata/"+tc.name, func(t *testing.T) {
			tx, err := p.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			_, err = store.UpdatePolicy(ctx, tx, testBrand, validInput, tc.meta)
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("invalid audit metadata error=%v, want ErrInvalid", err)
			}
		})
	}
}

func TestPolicyLockedReadAndMissingBrand(t *testing.T) {
	p := testdb.New(t)
	store := Store{DB: p}
	ctx := context.Background()
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := store.LockedPolicy(ctx, tx, testBrand)
	if err != nil || policy.BrandID != testBrand || policy.Version != 1 {
		t.Fatalf("locked policy=%+v err=%v", policy, err)
	}
	if err = tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		t.Fatal(err)
	}
	if _, err = store.ReadPolicy(ctx, ids.New()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing policy read error=%v, want ErrNotFound", err)
	}
}

func TestPostRefundMayExceedBalanceCapAndFreezeUnfreezeStillWorks(t *testing.T) {
	p, store, member := fixture(t)
	ctx := context.Background()
	credit(t, p, store, member, 0, 100)
	adminID := ids.New()
	if _, err := p.Exec(ctx, "INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'test-only-hash')", adminID, "policy_refund_"+adminID); err != nil {
		t.Fatal(err)
	}
	capValue := Amount(90)
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := store.UpdatePolicy(ctx, tx, testBrand, PolicyInput{Version: 1, MaxBalancePoints: &capValue, Reason: "lower test balance cap"}, Metadata{
		ActorType: "admin", ActorID: adminID, RequestID: "refund-cap-policy",
	})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if policy.Version != 2 || policy.MaxBalancePoints == nil || *policy.MaxBalancePoints != 90 {
		t.Fatalf("cap policy update=%+v", policy)
	}
	wallet, err := store.Read(ctx, testBrand, member)
	if err != nil {
		t.Fatal(err)
	}
	allocation, err := wallet.BySource.Allocate(20, "available")
	if err != nil {
		t.Fatal(err)
	}
	debitDelta, err := AllocationDelta(allocation, "available", "")
	if err != nil {
		t.Fatal(err)
	}
	debit := change(member, "policy-cap-debit", debitDelta, allocation)
	debit.EntryType = "bet"
	debitEntry := post(t, p, store, debit)

	tx, err = p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Reverse(ctx, tx, testBrand, member, debitEntry.ID, "policy-cap-refund", "refund original debit over current cap", Metadata{ActorType: "system", RequestID: "refund-over-cap"}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("refund reversal should be cap-exempt: %v", err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	refunded, err := store.Read(ctx, testBrand, member)
	if err != nil || refunded.AvailablePoints != 100 || refunded.DisplayPoints != 100 {
		t.Fatalf("refund did not restore above-cap balance: wallet=%+v err=%v", refunded, err)
	}

	freezeAllocation, err := refunded.BySource.Allocate(10, "available")
	if err != nil {
		t.Fatal(err)
	}
	freezeDelta, err := AllocationDelta(freezeAllocation, "available", "manual_frozen")
	if err != nil {
		t.Fatal(err)
	}
	freeze := change(member, "policy-cap-freeze", freezeDelta, freezeAllocation)
	freeze.EntryType = "freeze"
	post(t, p, store, freeze)
	frozen, err := store.Read(ctx, testBrand, member)
	if err != nil || frozen.AvailablePoints != 90 || frozen.ManualFrozenPoints != 10 || frozen.DisplayPoints != 100 {
		t.Fatalf("freeze failed while balance is over cap: wallet=%+v err=%v", frozen, err)
	}
	unfreezeAllocation, err := frozen.BySource.Allocate(10, "manual_frozen")
	if err != nil {
		t.Fatal(err)
	}
	unfreezeDelta, err := AllocationDelta(unfreezeAllocation, "manual_frozen", "available")
	if err != nil {
		t.Fatal(err)
	}
	unfreeze := change(member, "policy-cap-unfreeze", unfreezeDelta, unfreezeAllocation)
	unfreeze.EntryType = "unfreeze"
	post(t, p, store, unfreeze)
	restored, err := store.Read(ctx, testBrand, member)
	if err != nil || restored.AvailablePoints != 100 || restored.ManualFrozenPoints != 0 || restored.DisplayPoints != 100 {
		t.Fatalf("unfreeze failed while balance is over cap: wallet=%+v err=%v", restored, err)
	}
}
