package rewards

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	rewardTestBrand      = "0199a000-0000-7000-8000-000000000001"
	rewardTestOtherBrand = "0199a000-0000-7000-8000-000000000002"
)

type rewardFixture struct {
	db       *pgxpool.Pool
	service  Service
	points   points.Store
	brand    string
	member   string
	account  string
	admin    string
	actor    access.Account
	viewOnly access.Account
	super    access.Account
}

func newRewardFixture(t *testing.T) rewardFixture {
	t.Helper()
	ctx := context.Background()
	db := testdb.New(t)
	globalID, memberID, accountID, adminID := ids.New(), ids.New(), ids.New(), ids.New()
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO global_users(id,username,password_hash) VALUES($1,$2,'reward-test-only')`, []any{globalID, "reward_member_" + globalID[:8]}},
		{`INSERT INTO brand_members(id,brand_id,global_user_id,display_name,join_method,privacy_policy_version,service_terms_version) VALUES($1,$2,$3,'Reward test member','operator','1','1')`, []any{memberID, rewardTestBrand, globalID}},
		{`INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'reward-test-only')`, []any{adminID, "reward_admin_" + adminID[:8]}},
		{`INSERT INTO point_accounts(id,brand_id,brand_member_id) VALUES($1,$2,$3)`, []any{accountID, rewardTestBrand, memberID}},
		{`INSERT INTO point_buckets(brand_id,account_id,source,state,points)
SELECT $1,$2,s,t,0 FROM unnest(ARRAY['recharge','winning','gift']) s
CROSS JOIN unnest(ARRAY['available','manual_frozen','system_frozen','withdrawal']) t`, []any{rewardTestBrand, accountID}},
	} {
		if _, err := db.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("prepare reward fixture: %v", err)
		}
	}
	var bucketCount int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM point_buckets WHERE brand_id=$1 AND account_id=$2`, rewardTestBrand, accountID).Scan(&bucketCount); err != nil || bucketCount != 16 {
		t.Fatalf("fixture must have all 16 point buckets; count=%d err=%v", bucketCount, err)
	}

	grant := access.Permission{Resource: "reward", Action: "grant", Scope: access.ScopeBrand}
	revoke := access.Permission{Resource: "reward", Action: "revoke", Scope: access.ScopeBrand}
	retry := access.Permission{Resource: "reward", Action: "retry", Scope: access.ScopeBrand}
	view := access.Permission{Resource: "reward", Action: "view", Scope: access.ScopeBrand}
	all := access.Account{ID: adminID, Type: access.AccountAdmin, BrandIDs: []string{rewardTestBrand}, Roles: []access.Role{{BrandID: rewardTestBrand, Permissions: []access.Permission{view, grant, revoke, retry}}}}
	viewer := access.Account{ID: ids.New(), Type: access.AccountAdmin, BrandIDs: []string{rewardTestBrand}, Roles: []access.Role{{BrandID: rewardTestBrand, Permissions: []access.Permission{view}}}}
	super := access.Account{ID: ids.New(), Type: access.AccountAdmin, SuperAdmin: true, BrandIDs: []string{rewardTestBrand}, Roles: []access.Role{{Permissions: []access.Permission{view, grant, revoke, retry}}}}
	ps := points.Store{DB: db}
	return rewardFixture{db: db, service: Service{DB: db}, points: ps, brand: rewardTestBrand, member: memberID, account: accountID, admin: adminID, actor: all, viewOnly: viewer, super: super}
}

func rewardMeta(actor string) points.Metadata {
	return points.Metadata{ActorType: "admin", ActorID: actor, RequestID: ids.New(), IP: "127.0.0.1"}
}

func rewardTx(t *testing.T, db *pgxpool.Pool) pgx.Tx {
	t.Helper()
	tx, err := db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	return tx
}

func rewardCommit(t *testing.T, tx pgx.Tx) {
	t.Helper()
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func rewardGrant(t *testing.T, f rewardFixture, amount points.Amount) Order {
	t.Helper()
	tx := rewardTx(t, f.db)
	order, err := f.service.GrantTx(context.Background(), tx, f.brand, f.actor,
		GrantInput{MemberID: f.member, Points: amount, Reason: "approved test reward"}, rewardMeta(f.admin))
	if err != nil {
		t.Fatalf("grant %d: %v", amount, err)
	}
	rewardCommit(t, tx)
	return order
}

func rewardBalance(t *testing.T, f rewardFixture) points.Balance {
	t.Helper()
	tx := rewardTx(t, f.db)
	wallet, err := f.points.Snapshot(context.Background(), tx, f.brand, f.member)
	_ = tx.Rollback(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return wallet.BySource
}

func rewardCredit(t *testing.T, f rewardFixture, source string, amount points.Amount, suffix string) points.Entry {
	t.Helper()
	delta := points.Balance{}
	index, err := points.SourceIndex(source)
	if err != nil {
		t.Fatal(err)
	}
	delta[index][0] = amount
	tx := rewardTx(t, f.db)
	entry, err := f.points.Post(context.Background(), tx, points.Change{
		BrandID: f.brand, MemberID: f.member, EntryType: "adjustment", ReferenceType: "test",
		OperationKey: "reward-fixture:" + suffix + ":" + ids.New(), Reason: "seed isolated reward test funds",
		ActorType: "system", RequestID: ids.New(), Delta: delta,
		Allocation: []points.Allocation{{Source: source, State: "available", Points: amount}},
	})
	if err != nil {
		t.Fatalf("credit %s: %v", source, err)
	}
	rewardCommit(t, tx)
	return entry
}

func rewardMove(t *testing.T, f rewardFixture, source, from, to string, amount points.Amount) {
	t.Helper()
	allocation := []points.Allocation{{Source: source, State: from, Points: amount}}
	delta, err := points.AllocationDelta(allocation, from, to)
	if err != nil {
		t.Fatal(err)
	}
	tx := rewardTx(t, f.db)
	_, err = f.points.Post(context.Background(), tx, points.Change{
		BrandID: f.brand, MemberID: f.member, EntryType: "adjustment", ReferenceType: "test",
		OperationKey: "reward-fixture-move:" + ids.New(), Reason: "seed isolated reward test state",
		ActorType: "system", RequestID: ids.New(), Delta: delta, Allocation: allocation,
	})
	if err != nil {
		t.Fatalf("move %s %s -> %s: %v", source, from, to, err)
	}
	rewardCommit(t, tx)
}

func rewardField(t *testing.T, value any, name string) reflect.Value {
	t.Helper()
	v := reflect.Indirect(reflect.ValueOf(value))
	if !v.IsValid() || v.Kind() != reflect.Struct {
		t.Fatalf("%T is not a struct", value)
	}
	f := v.FieldByName(name)
	if !f.IsValid() {
		t.Fatalf("%T is missing contract field %s", value, name)
	}
	return f
}

func rewardID(t *testing.T, order Order) string {
	t.Helper()
	f := rewardField(t, order, "ID")
	if f.Kind() != reflect.String {
		t.Fatalf("Order.ID has kind %s", f.Kind())
	}
	return f.String()
}

func assertRewardOrder(t *testing.T, order Order, state string, version int64) {
	t.Helper()
	stateField, versionField := rewardField(t, order, "State"), rewardField(t, order, "Version")
	if stateField.String() != state || versionField.Int() != version {
		t.Fatalf("order state/version = %s/%d, want %s/%d (order=%+v)", stateField.String(), versionField.Int(), state, version, order)
	}
}

func rewardRevoke(t *testing.T, f rewardFixture, order Order, actor access.Account, reason string) (Order, error) {
	t.Helper()
	tx := rewardTx(t, f.db)
	out, err := f.service.RevokeTx(context.Background(), tx, f.brand, rewardID(t, order), actor,
		ActionInput{Version: rewardField(t, order, "Version").Int(), Reason: reason}, rewardMeta(actor.ID))
	if err != nil {
		_ = tx.Rollback(context.Background())
		return Order{}, err
	}
	rewardCommit(t, tx)
	return out, nil
}

func TestRewardGrantIsAtomicAndAudited(t *testing.T) {
	f := newRewardFixture(t)
	before := rewardBalance(t, f)
	order := rewardGrant(t, f, 125)
	assertRewardOrder(t, order, "granted", 1)
	want := before
	want[2][0] += 125
	if after := rewardBalance(t, f); after != want {
		t.Fatalf("grant did not credit only gift.available: before=%v after=%v", before, after)
	}

	ctx, err := f.points.Read(context.Background(), f.brand, f.member)
	if err != nil || ctx.BySource[2][0] != 125 || ctx.Version != 1 {
		t.Fatalf("formal points ledger snapshot=%+v err=%v", ctx, err)
	}
	var grants int
	if err := f.db.QueryRow(context.Background(), `SELECT count(*) FROM point_ledger_entries WHERE brand_id=$1 AND reference_type='reward_order' AND reference_id=$2 AND entry_type='reward_grant'`, f.brand, rewardID(t, order)).Scan(&grants); err != nil || grants != 1 {
		t.Fatalf("reward grant must have one formal ledger entry; count=%d err=%v", grants, err)
	}
	var audits int
	if err := f.db.QueryRow(context.Background(), `SELECT count(*) FROM audit_logs WHERE brand_id=$1 AND resource_type='reward_order' AND resource_id=$2`, f.brand, rewardID(t, order)).Scan(&audits); err != nil || audits < 1 {
		t.Fatalf("reward order must have audit history; count=%d err=%v", audits, err)
	}
	tx := rewardTx(t, f.db)
	got, err := f.service.OrderTx(context.Background(), tx, f.brand, rewardID(t, order))
	if err != nil || rewardID(t, got) != rewardID(t, order) {
		t.Fatalf("read order=%+v err=%v", got, err)
	}
	orders, err := f.service.OrdersTx(context.Background(), tx, f.brand, 20, 0)
	if err != nil || len(rewardPageItems(t, orders)) != 1 {
		t.Fatalf("orders page=%+v err=%v", orders, err)
	}
	actions, err := f.service.ActionsTx(context.Background(), tx, f.brand, rewardID(t, order), 20, 0)
	if err != nil || len(actions.Items) != 1 || actions.Items[0].Operation != "grant" || actions.Items[0].Version != 1 || actions.Items[0].StateAfter != "granted" || actions.Items[0].LedgerEntryID == nil {
		t.Fatalf("actions page=%+v err=%v", actions, err)
	}
	_ = tx.Rollback(context.Background())
}

func rewardPageItems(t *testing.T, page any) []reflect.Value {
	t.Helper()
	field := rewardField(t, page, "Items")
	if field.Kind() != reflect.Slice {
		t.Fatalf("%T.Items is not a slice", page)
	}
	items := make([]reflect.Value, field.Len())
	for i := range items {
		items[i] = field.Index(i)
	}
	return items
}

func TestRewardImmediateFullRevokeAndDuplicateConflict(t *testing.T) {
	f := newRewardFixture(t)
	order := rewardGrant(t, f, 90)
	grantRows := rewardLedgerRows(t, f, rewardID(t, order))
	if len(grantRows) != 1 {
		t.Fatalf("grant ledger rows=%+v", grantRows)
	}
	revoked, err := rewardRevoke(t, f, order, f.actor, "approved full reversal")
	if err != nil {
		t.Fatal(err)
	}
	assertRewardOrder(t, revoked, "revoked", 2)
	rows := rewardLedgerRows(t, f, rewardID(t, order))
	if len(rows) != 2 || rows[0].reversalOf != "" || rows[1].reversalOf != rows[0].id || rows[1].amount != -rows[0].amount || rows[1].entryType != "reward_reversal" {
		t.Fatalf("revoke must append one exact full reversal of its grant: %+v", rows)
	}
	if after := rewardBalance(t, f); after[2][0] != 0 {
		t.Fatalf("full revoke left gift.available=%d", after[2][0])
	}
	tx := rewardTx(t, f.db)
	_, err = f.service.RevokeTx(context.Background(), tx, f.brand, rewardID(t, order), f.actor, ActionInput{Version: 2, Reason: "duplicate revoke"}, rewardMeta(f.admin))
	_ = tx.Rollback(context.Background())
	if !errors.Is(err, ErrState) {
		t.Fatalf("duplicate revoke error=%v, want rewards.ErrState", err)
	}
	tx = rewardTx(t, f.db)
	_, err = f.service.RetryRevocationTx(context.Background(), tx, f.brand, rewardID(t, order), f.actor, ActionInput{Version: 2, Reason: "retry completed revoke"}, rewardMeta(f.admin))
	_ = tx.Rollback(context.Background())
	if !errors.Is(err, ErrState) {
		t.Fatalf("retry of non-pending order error=%v, want rewards.ErrState", err)
	}
	if rows = rewardLedgerRows(t, f, rewardID(t, order)); len(rows) != 2 {
		t.Fatalf("duplicate revoke changed ledger: %+v", rows)
	}
}

type rewardLedgerRow struct {
	id, entryType, reversalOf string
	amount                    int64
}

func rewardLedgerRows(t *testing.T, f rewardFixture, orderID string) []rewardLedgerRow {
	t.Helper()
	rows, err := f.db.Query(context.Background(), `SELECT id::text,entry_type,COALESCE(reversal_of::text,''),delta_snapshot->'gift'->>'available'
FROM point_ledger_entries WHERE brand_id=$1 AND reference_type='reward_order' AND reference_id=$2 ORDER BY version`, f.brand, orderID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []rewardLedgerRow
	for rows.Next() {
		var row rewardLedgerRow
		var amount string
		if err := rows.Scan(&row.id, &row.entryType, &row.reversalOf, &amount); err != nil {
			t.Fatal(err)
		}
		if _, err := fmt.Sscan(amount, &row.amount); err != nil {
			t.Fatal(err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestRewardPendingNeedsExplicitRetryAndOnlyGiftAvailableCounts(t *testing.T) {
	f := newRewardFixture(t)
	for i, source := range []string{"recharge", "winning", "commission"} {
		if source == "commission" {
			// Commission credit goes through the standard points store; this also
			// verifies the migration-created commission buckets are usable.
			rewardCredit(t, f, source, 500, fmt.Sprintf("source-%d", i))
		} else {
			rewardCredit(t, f, source, 500, fmt.Sprintf("source-%d", i))
		}
	}
	rewardCredit(t, f, "gift", 30, "gift-funds")
	rewardMove(t, f, "gift", "available", "manual_frozen", 30)
	order := rewardGrant(t, f, 100)
	// Freeze the newly granted amount too, leaving no gift.available funds to
	// reverse while preserving unrelated recharge/winning/commission balances.
	rewardMove(t, f, "gift", "available", "manual_frozen", 100)
	pending, err := rewardRevoke(t, f, order, f.actor, "revoke while gift funds are frozen")
	if err != nil {
		t.Fatal(err)
	}
	assertRewardOrder(t, pending, "revocation_pending", 2)
	code := rewardField(t, pending, "LastErrorCode")
	if code.Kind() == reflect.Ptr {
		if code.IsNil() || code.Elem().String() != "REWARD_AVAILABLE_INSUFFICIENT" {
			t.Fatalf("pending error code=%v", code)
		}
	} else if code.String() != "REWARD_AVAILABLE_INSUFFICIENT" {
		t.Fatalf("pending error code=%v", code)
	}
	unchanged := rewardBalance(t, f)
	if unchanged[2][0] != 0 || unchanged[2][1] != 130 || unchanged[0][0] != 500 || unchanged[1][0] != 500 || unchanged[3][0] != 500 {
		t.Fatalf("insufficient gift must not move any source: %v", unchanged)
	}
	if rows := rewardLedgerRows(t, f, rewardID(t, order)); len(rows) != 1 {
		t.Fatalf("pending revoke must keep grant without partial reversal: %+v", rows)
	}
	// Later credits to other sources cannot satisfy a gift-source reversal.
	for _, source := range []string{"recharge", "winning", "commission"} {
		rewardCredit(t, f, source, 200, "still-insufficient-"+source)
	}
	var current Order
	tx := rewardTx(t, f.db)
	current, err = f.service.OrderTx(context.Background(), tx, f.brand, rewardID(t, order))
	_ = tx.Rollback(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	assertRewardOrder(t, current, "revocation_pending", 2)
	tx = rewardTx(t, f.db)
	_, err = f.service.RetryRevocationTx(context.Background(), tx, f.brand, rewardID(t, order), f.actor, ActionInput{Version: 2, Reason: "still insufficient"}, rewardMeta(f.admin))
	if err != nil {
		t.Fatal(err)
	}
	rewardCommit(t, tx)
	var afterRetry Order
	tx = rewardTx(t, f.db)
	afterRetry, err = f.service.OrderTx(context.Background(), tx, f.brand, rewardID(t, order))
	_ = tx.Rollback(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	assertRewardOrder(t, afterRetry, "revocation_pending", 3)
	if rows := rewardLedgerRows(t, f, rewardID(t, order)); len(rows) != 1 {
		t.Fatalf("unsuccessful retry moved ledger: %+v", rows)
	}

	rewardMove(t, f, "gift", "manual_frozen", "available", 100)
	// Releasing gift funds alone still cannot initiate a retry.
	tx = rewardTx(t, f.db)
	stillPending, err := f.service.OrderTx(context.Background(), tx, f.brand, rewardID(t, order))
	_ = tx.Rollback(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	assertRewardOrder(t, stillPending, "revocation_pending", 3)
	tx = rewardTx(t, f.db)
	completed, err := f.service.RetryRevocationTx(context.Background(), tx, f.brand, rewardID(t, order), f.actor, ActionInput{Version: 3, Reason: "explicit retry after gift release"}, rewardMeta(f.admin))
	if err != nil {
		t.Fatal(err)
	}
	rewardCommit(t, tx)
	assertRewardOrder(t, completed, "revoked", 4)
	if rows := rewardLedgerRows(t, f, rewardID(t, order)); len(rows) != 2 || rows[1].reversalOf != rows[0].id || rows[1].amount != -rows[0].amount {
		t.Fatalf("retry must append exactly one full reversal, preserving grant: %+v", rows)
	}
	var actionCount int
	if err := f.db.QueryRow(context.Background(), `SELECT count(*) FROM reward_order_actions WHERE brand_id=$1 AND order_id=$2`, f.brand, rewardID(t, order)).Scan(&actionCount); err != nil || actionCount != 4 {
		t.Fatalf("grant, revoke and two explicit retry actions must remain audited: count=%d err=%v", actionCount, err)
	}
}

func TestRewardConcurrentRevokeCreatesAtMostOneReversal(t *testing.T) {
	f := newRewardFixture(t)
	order := rewardGrant(t, f, 77)
	start := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	success, conflict, busy, unexpected := 0, 0, 0, []error{}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			tx, err := f.db.Begin(context.Background())
			if err != nil {
				mu.Lock()
				unexpected = append(unexpected, err)
				mu.Unlock()
				return
			}
			_, err = f.service.RevokeTx(context.Background(), tx, f.brand, rewardID(t, order), f.actor,
				ActionInput{Version: 1, Reason: "concurrent revoke"}, rewardMeta(f.admin))
			if err == nil {
				err = tx.Commit(context.Background())
			} else {
				_ = tx.Rollback(context.Background())
			}
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				success++
			} else if errors.Is(err, ErrState) || errors.Is(err, ErrVersion) {
				conflict++
			} else if errors.Is(err, ErrBusy) {
				busy++
			} else {
				unexpected = append(unexpected, err)
			}
		}()
	}
	close(start)
	wg.Wait()
	if len(unexpected) != 0 || success != 1 || conflict+busy != 7 {
		t.Fatalf("concurrent revoke success=%d conflicts=%d busy=%d unexpected=%v", success, conflict, busy, unexpected)
	}
	if rows := rewardLedgerRows(t, f, rewardID(t, order)); len(rows) != 2 {
		t.Fatalf("concurrent revokes produced %d ledger entries: %+v", len(rows), rows)
	}
}

func TestRewardLedgerAndAuditFailuresRollbackOrderAndFunds(t *testing.T) {
	for _, failOn := range []string{"ledger", "audit"} {
		t.Run(failOn, func(t *testing.T) {
			f := newRewardFixture(t)
			requestID := "reward-fault-" + ids.New()
			trigger := "fail_reward_" + failOn + "_" + ids.New()[:8]
			var createSQL string
			if failOn == "ledger" {
				createSQL = fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.request_id='%s' THEN RAISE EXCEPTION 'injected reward ledger failure'; END IF; RETURN NEW; END $$;
CREATE TRIGGER %s BEFORE INSERT ON point_ledger_entries FOR EACH ROW EXECUTE FUNCTION %s()`, trigger, requestID, trigger, trigger)
			} else {
				createSQL = fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.request_id='%s' THEN RAISE EXCEPTION 'injected reward audit failure'; END IF; RETURN NEW; END $$;
CREATE TRIGGER %s BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION %s()`, trigger, requestID, trigger, trigger)
			}
			functionSQL, triggerSQL, ok := strings.Cut(createSQL, ";\n")
			if !ok {
				t.Fatal("failed to prepare fault injection statements")
			}
			if _, err := f.db.Exec(context.Background(), functionSQL); err != nil {
				t.Fatal(err)
			}
			if _, err := f.db.Exec(context.Background(), triggerSQL); err != nil {
				t.Fatal(err)
			}
			before := rewardBalance(t, f)
			tx := rewardTx(t, f.db)
			_, err := f.service.GrantTx(context.Background(), tx, f.brand, f.actor,
				GrantInput{MemberID: f.member, Points: 44, Reason: "fault injection"}, points.Metadata{ActorType: "admin", ActorID: f.admin, RequestID: requestID})
			_ = tx.Rollback(context.Background())
			if err == nil {
				t.Fatal("injected ledger/audit failure was ignored")
			}
			if got := rewardBalance(t, f); got != before {
				t.Fatalf("failed %s write changed wallet: before=%v after=%v", failOn, before, got)
			}
			orders, err := f.service.OrdersTx(context.Background(), rewardTx(t, f.db), f.brand, 20, 0)
			if err != nil || len(rewardPageItems(t, orders)) != 0 {
				t.Fatalf("failed %s write left an order: %+v err=%v", failOn, orders, err)
			}
		})
	}
}

func TestRewardPermissionsBrandsVersionsAndBrandStatus(t *testing.T) {
	f := newRewardFixture(t)
	grant := func(actor access.Account, brand string) error {
		tx := rewardTx(t, f.db)
		_, err := f.service.GrantTx(context.Background(), tx, brand, actor,
			GrantInput{MemberID: f.member, Points: 10, Reason: "permission test"}, rewardMeta(actor.ID))
		_ = tx.Rollback(context.Background())
		return err
	}
	if err := grant(f.viewOnly, f.brand); !errors.Is(err, ErrDenied) {
		t.Fatalf("view permission grant error=%v, want rewards.ErrDenied", err)
	}
	if err := grant(f.super, f.brand); !errors.Is(err, ErrDenied) {
		t.Fatalf("super administrator grant error=%v, want rewards.ErrDenied", err)
	}
	foreignBrandActor := f.actor
	foreignBrandActor.BrandIDs = append(append([]string{}, f.actor.BrandIDs...), rewardTestOtherBrand)
	foreignBrandActor.Roles = append(append([]access.Role{}, f.actor.Roles...), access.Role{BrandID: rewardTestOtherBrand, Permissions: []access.Permission{
		{Resource: "reward", Action: "view", Scope: access.ScopeBrand},
		{Resource: "reward", Action: "grant", Scope: access.ScopeBrand},
	}})
	if err := grant(foreignBrandActor, rewardTestOtherBrand); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-brand member lookup error=%v, want rewards.ErrNotFound", err)
	}
	order := rewardGrant(t, f, 10)
	for _, actor := range []access.Account{f.viewOnly, f.super} {
		tx := rewardTx(t, f.db)
		_, err := f.service.RevokeTx(context.Background(), tx, f.brand, rewardID(t, order), actor, ActionInput{Version: 1, Reason: "not permitted"}, rewardMeta(actor.ID))
		_ = tx.Rollback(context.Background())
		if !errors.Is(err, ErrDenied) {
			t.Fatalf("actor %+v revoke error=%v, want rewards.ErrDenied", actor, err)
		}
	}
	for _, query := range []struct {
		brand string
		id    string
	}{{rewardTestOtherBrand, rewardID(t, order)}, {f.brand, ids.New()}} {
		tx := rewardTx(t, f.db)
		_, err := f.service.OrderTx(context.Background(), tx, query.brand, query.id)
		_ = tx.Rollback(context.Background())
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("foreign order lookup (%s,%s) error=%v, want not found", query.brand, query.id, err)
		}
	}
	tx := rewardTx(t, f.db)
	_, err := f.service.RevokeTx(context.Background(), tx, f.brand, rewardID(t, order), f.actor, ActionInput{Version: 2, Reason: "stale version"}, rewardMeta(f.admin))
	_ = tx.Rollback(context.Background())
	if !errors.Is(err, ErrVersion) {
		t.Fatalf("stale revoke version error=%v, want rewards.ErrVersion", err)
	}
	if _, err := f.db.Exec(context.Background(), `UPDATE brands SET status='paused' WHERE id=$1`, f.brand); err != nil {
		t.Fatal(err)
	}
	if err := grant(f.actor, f.brand); err != nil {
		t.Fatalf("paused brand should permit scoped reward write: %v", err)
	}
	if _, err := f.db.Exec(context.Background(), `UPDATE brands SET status='disabled' WHERE id=$1`, f.brand); err != nil {
		t.Fatal(err)
	}
	if err := grant(f.actor, f.brand); err == nil {
		t.Fatal("disabled brand accepted reward grant")
	}
}

func TestRewardOrdinarySQLCannotBypassImmutableAuditAndReversalGuard(t *testing.T) {
	f := newRewardFixture(t)
	order := rewardGrant(t, f, 25)
	for _, statement := range []string{
		`UPDATE reward_orders SET state='revoked' WHERE id=$1`,
		`DELETE FROM reward_orders WHERE id=$1`,
		`UPDATE reward_order_actions SET reason='rewritten' WHERE order_id=$1`,
		`DELETE FROM reward_order_actions WHERE order_id=$1`,
	} {
		if _, err := f.db.Exec(context.Background(), statement, rewardID(t, order)); err == nil {
			t.Fatalf("ordinary SQL bypass accepted: %s", statement)
		}
	}
	// A direct reversal request with a wrong reference or partial delta is not
	// admitted by the authoritative points store even outside rewards.Service.
	grantRows := rewardLedgerRows(t, f, rewardID(t, order))
	if len(grantRows) != 1 {
		t.Fatalf("expected original grant: %+v", grantRows)
	}
	delta := points.Balance{}
	delta[2][0] = -24
	tx := rewardTx(t, f.db)
	_, err := f.points.Post(context.Background(), tx, points.Change{
		BrandID: f.brand, MemberID: f.member, EntryType: "reward_reversal", ReferenceType: "reward_order",
		ReferenceID: rewardID(t, order), OperationKey: "unauthorized-partial-reversal-" + ids.New(),
		Reason: "partial bypass", ActorType: "admin", ActorID: f.admin, RequestID: ids.New(), Delta: delta,
		Allocation: []points.Allocation{{Source: "gift", State: "available", Points: 25}}, ReversalOf: grantRows[0].id,
	})
	_ = tx.Rollback(context.Background())
	if err == nil {
		t.Fatal("points store accepted a partial reward reversal")
	}
}

func TestRewardMigration54To55PreservesFinancialFingerprintAndPinsGuards(t *testing.T) {
	ctx := context.Background()
	db := testdb.NewAtVersion(t, 54)
	globalID, memberID, accountID, adminID := ids.New(), ids.New(), ids.New(), ids.New()
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO global_users(id,username,password_hash) VALUES($1,$2,'reward-upgrade-test')`, []any{globalID, "reward_upgrade_member_" + globalID[:8]}},
		{`INSERT INTO brand_members(id,brand_id,global_user_id,display_name,join_method,privacy_policy_version,service_terms_version) VALUES($1,$2,$3,'Upgrade test member','operator','1','1')`, []any{memberID, rewardTestBrand, globalID}},
		{`INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'reward-upgrade-test')`, []any{adminID, "reward_upgrade_admin_" + adminID[:8]}},
		{`INSERT INTO point_accounts(id,brand_id,brand_member_id) VALUES($1,$2,$3)`, []any{accountID, rewardTestBrand, memberID}},
		{`INSERT INTO point_buckets(brand_id,account_id,source,state,points) SELECT $1,$2,s,t,0 FROM unnest(ARRAY['recharge','winning','gift']) s CROSS JOIN unnest(ARRAY['available','manual_frozen','system_frozen','withdrawal']) t`, []any{rewardTestBrand, accountID}},
	} {
		if _, err := db.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	var delta points.Balance
	delta[0][0] = 41
	tx := rewardTx(t, db)
	if _, err := (points.Store{DB: db}).Post(ctx, tx, points.Change{
		BrandID: rewardTestBrand, MemberID: memberID, EntryType: "adjustment", ReferenceType: "test",
		OperationKey: "reward-upgrade-seed:" + ids.New(), Reason: "preserve pre-upgrade financial history",
		ActorType: "system", RequestID: ids.New(), Delta: delta,
		Allocation: []points.Allocation{{Source: "recharge", State: "available", Points: 41}},
	}); err != nil {
		t.Fatal(err)
	}
	rewardCommit(t, tx)

	fingerprint := func() string {
		t.Helper()
		var raw string
		err := db.QueryRow(ctx, `SELECT jsonb_build_object(
 'brands',(SELECT jsonb_agg(to_jsonb(x) ORDER BY x.id) FROM brands x),
 'users',(SELECT jsonb_agg(to_jsonb(x) ORDER BY x.id) FROM global_users x),
 'members',(SELECT jsonb_agg(to_jsonb(x) ORDER BY x.id) FROM brand_members x),
 'admins',(SELECT jsonb_agg(to_jsonb(x) ORDER BY x.id) FROM admin_accounts x),
 'accounts',(SELECT jsonb_agg(to_jsonb(x) ORDER BY x.id) FROM point_accounts x),
 'buckets',(SELECT jsonb_agg(to_jsonb(x) ORDER BY x.account_id,x.source,x.state) FROM point_buckets x),
 'ledger',(SELECT jsonb_agg(to_jsonb(x) ORDER BY x.id) FROM point_ledger_entries x),
 'audits',(SELECT jsonb_agg(to_jsonb(x) ORDER BY x.id) FROM audit_logs x))::text`).Scan(&raw)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	before := fingerprint()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatalf("upgrade schema 54 to 55: %v", err)
	}
	if after := fingerprint(); after != before {
		t.Fatal("0055 migration changed pre-existing financial, identity, ledger, or audit data")
	}
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatalf("repeat migration: %v", err)
	}
	if after := fingerprint(); after != before {
		t.Fatal("repeated migration changed pre-existing financial history")
	}

	var schema string
	if err := db.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	guardNames := []string{"guard_reward_order", "guard_reward_action", "guard_reward_credit", "require_reward_order_commit", "require_reward_action_commit", "require_reward_ledger_commit"}
	rows, err := db.Query(ctx, `SELECT p.proname,p.proconfig FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname=$1 AND p.proname=ANY($2::text[])`, schema, guardNames)
	if err != nil {
		t.Fatal(err)
	}
	wantPath := "search_path=pg_catalog, " + schema + ", pg_temp"
	guarded := make(map[string]bool, len(guardNames))
	for rows.Next() {
		var name string
		var config []string
		if err := rows.Scan(&name, &config); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		for _, setting := range config {
			if setting == wantPath {
				guarded[name] = true
			}
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		t.Fatal(err)
	}
	rows.Close()
	for _, name := range guardNames {
		if !guarded[name] {
			t.Errorf("%s lacks pinned search_path %q", name, wantPath)
		}
	}

	service := Service{DB: db}
	actor := access.Account{ID: adminID, Type: access.AccountAdmin, BrandIDs: []string{rewardTestBrand}, Roles: []access.Role{{BrandID: rewardTestBrand, Permissions: []access.Permission{
		{Resource: "reward", Action: "view", Scope: access.ScopeBrand},
		{Resource: "reward", Action: "grant", Scope: access.ScopeBrand},
	}}}}
	tx = rewardTx(t, db)
	order, err := service.GrantTx(ctx, tx, rewardTestBrand, actor, GrantInput{MemberID: memberID, Points: 3, Reason: "search path guard probe"}, rewardMeta(adminID))
	if err != nil {
		t.Fatal(err)
	}
	rewardCommit(t, tx)
	qualifiedSchema := pgx.Identifier{schema}.Sanitize()
	tx = rewardTx(t, db)
	if _, err := tx.Exec(ctx, `SET LOCAL search_path TO pg_catalog`); err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, fmt.Sprintf(`UPDATE %s.reward_orders SET reason='tampered' WHERE id=$1`, qualifiedSchema), order.ID)
	_ = tx.Rollback(ctx)
	if err == nil || !strings.Contains(err.Error(), "reward state, version or immutable identity conflict") {
		t.Fatalf("qualified mutation with empty app search_path did not reach pinned reward guard: %v", err)
	}
}
