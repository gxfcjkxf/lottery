package betting

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/authcrypto"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rulebook"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const storeTestBrand = "0199a000-0000-7000-8000-000000000001"
const storeTestOtherBrand = "0199a000-0000-7000-8000-000000000002"

type bettingFixture struct {
	db      *pgxpool.Pool
	service Service
	user    identity.Session
	token   string
	brand   string
	game    rulebook.Game
	play    rulebook.Play
	version rulebook.Version
	period  rulebook.Period
	input   Input
	member  string
}

func bettingTx(t *testing.T, db interface {
	Begin(context.Context) (pgx.Tx, error)
}, fn func(pgx.Tx) error) {
	t.Helper()
	ctx := context.Background()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err = fn(tx); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func bettingRuleActor(id, brand string, actions ...string) access.Account {
	a := access.Account{ID: id, Type: access.AccountAdmin, BrandIDs: []string{brand}}
	r := access.Role{BrandID: brand}
	for _, action := range actions {
		r.Permissions = append(r.Permissions, access.Permission{Resource: "rule", Action: action, Scope: access.ScopeBrand})
	}
	r.Permissions = append(r.Permissions, access.Permission{Resource: "game", Action: "write", Scope: access.ScopeBrand})
	a.Roles = []access.Role{r}
	return a
}

func bettingDefinition() rules.Definition {
	return rules.Definition{
		SchemaVersion: 1,
		Model:         rules.Model{Type: "DIGITS_0_9", Length: 3, AllowRepeat: true, Ordered: true},
		Selection:     rules.SelectionRule{Mode: "numbers"},
		UnitPoints:    1,
		PrizeTiers:    []rules.Tier{{Code: "EXACT", Condition: rules.Condition{Op: "equals", Field: "position_match", Value: intPtr(3)}, Odds: "10", Exclusive: true}},
		Rounding:      "half_up",
		RoundingScope: "order",
		Limits:        rules.Limits{MaxCombinations: 100, MaxMultiplier: 1000},
	}
}

func newBettingFixture(t *testing.T, brand string) bettingFixture {
	return newBettingFixtureWithWindow(t, brand, time.Hour, 2*time.Hour)
}

func newBettingFixtureWithWindow(t *testing.T, brand string, betWindow, drawWindow time.Duration) bettingFixture {
	t.Helper()
	db := testdb.New(t)
	ctx := context.Background()
	f := bettingFixture{db: db, service: Service{DB: db}, brand: brand}
	creatorID, reviewerID, userID, memberID := ids.New(), ids.New(), ids.New(), ids.New()
	for i, id := range []string{creatorID, reviewerID} {
		if _, err := db.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'test-only')`, id, fmt.Sprintf("bet_rule_%s_%d", id[:8], i)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(ctx, `INSERT INTO global_users(id,username,password_hash) VALUES($1,$2,'test-only')`, userID, "bet_user_"+userID[:8]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version) VALUES($1,$2,$3,'domain','test-1','test-1')`, memberID, brand, userID); err != nil {
		t.Fatal(err)
	}
	accountID := ids.New()
	if _, err := db.Exec(ctx, `INSERT INTO point_accounts(id,brand_id,brand_member_id) VALUES($1,$2,$3)`, accountID, brand, memberID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO point_buckets(brand_id,account_id,source,state) SELECT $1,$2,s,t FROM unnest(ARRAY['recharge','winning','gift','commission'])s CROSS JOIN unnest(ARRAY['available','manual_frozen','system_frozen','withdrawal'])t ON CONFLICT DO NOTHING`, brand, accountID); err != nil {
		t.Fatal(err)
	}
	token, err := authcrypto.NewSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	digest := authcrypto.DigestSessionToken(token)
	if _, err := db.Exec(ctx, `INSERT INTO sessions(id,token_hash,user_id,member_id,brand_id,expires_at) VALUES($1,$2,$3,$4,$5,clock_timestamp()+interval '1 hour')`, ids.New(), hex.EncodeToString(digest[:]), userID, memberID, brand); err != nil {
		t.Fatal(err)
	}
	identityStore, err := identity.New(db)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	f.user, err = identityStore.AuthenticateTx(ctx, tx, brand, token)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	f.member = memberID
	f.token = token

	admin := bettingRuleActor(creatorID, brand, "write", "validate", "submit")
	reviewer := bettingRuleActor(reviewerID, brand, "review")
	rs := rulebook.Store{DB: db}
	bettingTx(t, db, func(tx pgx.Tx) error {
		var e error
		f.game, e = rs.CreateGame(ctx, tx, brand, admin, "bet_game_"+creatorID[:8], "Bet fixture", bettingDefinition().Model, "UTC", "create betting game", points.Metadata{})
		return e
	})
	bettingTx(t, db, func(tx pgx.Tx) error {
		var e error
		f.play, e = rs.CreatePlay(ctx, tx, brand, admin, f.game.ID, "exact", "Exact", "create betting play", points.Metadata{})
		return e
	})
	definition := bettingDefinition()
	bettingTx(t, db, func(tx pgx.Tx) error {
		var e error
		f.version, e = rs.CreateVersion(ctx, tx, brand, admin, f.play.ID, definition, "immediate", "create test rule", points.Metadata{})
		return e
	})
	bettingTx(t, db, func(tx pgx.Tx) error {
		var e error
		f.version, e = rs.Validate(ctx, tx, brand, admin, f.version.ID, f.version.Version, []rules.ValidationCase{{Name: "exact", Selection: rules.Selection{Digits: [][]int{{1}, {2}, {1}}}, Draw: rules.Draw{Digits: []int{1, 2, 1}}, Multiplier: 1, ExpectedBetPoints: amountPtr(1), ExpectedPrizePoints: amountPtr(10), ExpectedWon: boolPtr(true)}}, "validate test rule", points.Metadata{})
		return e
	})
	bettingTx(t, db, func(tx pgx.Tx) error {
		var e error
		f.version, e = rs.Submit(ctx, tx, brand, admin, f.version.ID, f.version.Version, "submit test rule", points.Metadata{})
		return e
	})
	bettingTx(t, db, func(tx pgx.Tx) error {
		var e error
		f.version, e = rs.Review(ctx, tx, brand, reviewer, f.version.ID, f.version.Version, true, true, "approve test rule", points.Metadata{})
		return e
	})
	now := time.Now().UTC()
	bettingTx(t, db, func(tx pgx.Tx) error {
		var e error
		f.period, e = rs.OpenPeriod(ctx, tx, brand, f.game.ID, "bet-"+creatorID[:8], now.Add(-time.Minute), now.Add(betWindow), now.Add(drawWindow))
		return e
	})
	policyVersions := readBettingPolicyVersions(t, f.service, brand, f.game.ID)
	f.input = Input{PeriodID: f.period.ID, PlayID: f.play.ID, RuleVersionID: f.version.ID, Selection: rules.Selection{Digits: [][]int{{1}, {2}, {1}}}, Multiplier: 1, PolicyVersions: &policyVersions}
	return f
}

func boolPtr(value bool) *bool { return &value }

func intPtr(value int) *int { return &value }

func readBettingPolicyVersions(t *testing.T, service Service, brand, game string) PolicyVersions {
	t.Helper()
	ctx := context.Background()
	brandRecord, err := service.BrandPolicy(ctx, brand)
	if err != nil {
		t.Fatal(err)
	}
	gameRecord, err := service.GamePolicy(ctx, brand, game)
	if err != nil {
		t.Fatal(err)
	}
	return PolicyVersions{Brand: brandRecord.Version, Game: gameRecord.Version}
}

func fundBettingWallet(t *testing.T, f bettingFixture, amounts ...points.Amount) {
	t.Helper()
	ctx := context.Background()
	ps := points.Store{DB: f.db}
	for i, amount := range amounts {
		var delta points.Balance
		delta[i][0] = amount
		allocation := []points.Allocation{{Source: []string{"recharge", "winning", "gift", "commission"}[i], State: "available", Points: amount}}
		tx, err := f.db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		_, err = ps.Post(ctx, tx, points.Change{BrandID: f.brand, MemberID: f.member, EntryType: "adjustment", ReferenceType: "test_fund", OperationKey: "bet-fund-" + ids.New(), Reason: "betting fixture funding", ActorType: "system", RequestID: ids.New(), Delta: delta, Allocation: allocation})
		if err != nil {
			_ = tx.Rollback(ctx)
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
}

func placeBettingOrder(t *testing.T, f bettingFixture, in Input, key string) (Order, error) {
	t.Helper()
	ctx := context.Background()
	tx, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	order, err := f.service.Place(ctx, tx, f.brand, f.user, in, key, points.Metadata{RequestID: ids.New()})
	if err != nil {
		_ = tx.Rollback(ctx)
		return order, err
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return order, nil
}

func walletBySource(t *testing.T, f bettingFixture) points.Balance {
	t.Helper()
	wallet, err := (points.Store{DB: f.db}).Read(context.Background(), f.brand, f.member)
	if err != nil {
		t.Fatal(err)
	}
	return wallet.BySource
}

func TestPlaceDeductsInSourceOrderAndCancelRestoresExactAllocation(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	fundBettingWallet(t, f, 10, 10, 10)
	setUserCancellation(t, f, true)
	f.input.PolicyVersions = ptrPolicyVersions(readBettingPolicyVersions(t, f.service, f.brand, f.game.ID))
	before := walletBySource(t, f)
	in := f.input
	in.Multiplier = 25
	order, err := placeBettingOrder(t, f, in, "source-order-001")
	if err != nil {
		t.Fatal(err)
	}
	wallet := walletBySource(t, f)
	if wallet[0][0] != 0 || wallet[1][0] != 0 || wallet[2][0] != 5 {
		t.Fatalf("unexpected post-debit buckets: %+v", wallet)
	}
	wantAllocation := []points.Allocation{{Source: "recharge", State: "available", Points: 10}, {Source: "winning", State: "available", Points: 10}, {Source: "gift", State: "available", Points: 5}}
	if fmt.Sprint(order.Allocation) != fmt.Sprint(wantAllocation) {
		t.Fatalf("allocation=%+v want %+v", order.Allocation, wantAllocation)
	}
	tx, err := f.db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := f.service.Cancel(context.Background(), tx, f.brand, f.user, order.ID, order.Version, "user changed mind", points.Metadata{RequestID: ids.New()})
	if err != nil {
		_ = tx.Rollback(context.Background())
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != "bet_cancelled" || cancelled.RefundEntryID == "" || cancelled.Version != order.Version+1 {
		t.Fatalf("cancel result=%+v", cancelled)
	}
	if restored := walletBySource(t, f); restored != before {
		t.Fatalf("wallet after refund=%+v want original %+v", restored, before)
	}
	var refEntry string
	if err = f.db.QueryRow(context.Background(), `SELECT reversal_of::text FROM point_ledger_entries WHERE id=$1`, cancelled.RefundEntryID).Scan(&refEntry); err != nil || refEntry != order.DebitEntryID {
		t.Fatalf("refund reversal=%q want debit=%q err=%v", refEntry, order.DebitEntryID, err)
	}
}

func TestPlaceUsesFourSourcePriorityAndRefundsExactAllocation(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	fundBettingWallet(t, f, 10, 10, 10, 10)
	setUserCancellation(t, f, true)
	f.input.PolicyVersions = ptrPolicyVersions(readBettingPolicyVersions(t, f.service, f.brand, f.game.ID))
	before := walletBySource(t, f)
	in := f.input
	in.Multiplier = 35
	order, err := placeBettingOrder(t, f, in, "four-source-refund-001")
	if err != nil {
		t.Fatal(err)
	}
	wantAllocation := []points.Allocation{{Source: "recharge", State: "available", Points: 10}, {Source: "winning", State: "available", Points: 10}, {Source: "commission", State: "available", Points: 10}, {Source: "gift", State: "available", Points: 5}}
	if fmt.Sprint(order.Allocation) != fmt.Sprint(wantAllocation) {
		t.Fatalf("allocation=%+v want %+v", order.Allocation, wantAllocation)
	}
	tx, err := f.db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := f.service.Cancel(context.Background(), tx, f.brand, f.user, order.ID, order.Version, "refund all four source rows", points.Metadata{RequestID: ids.New()})
	if err != nil {
		_ = tx.Rollback(context.Background())
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	if cancelled.RefundEntryID == "" || walletBySource(t, f) != before {
		t.Fatalf("four-source refund did not restore exact wallet: order=%+v wallet=%+v want=%+v", cancelled, walletBySource(t, f), before)
	}
}

// Test-only historical-shape construction: synthesize the exact pre-commission
// 12-bucket snapshots from a real three-source wallet. Production code never
// rewrites these records; only the two append-only guards are disabled here,
// and are restored before the controlled transaction commits.
func synthesizeLegacyThreeSourceLedger(t *testing.T, db *pgxpool.Pool, accountID string) {
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
			t.Fatalf("construct historical three-source evidence (%s): %v", stmt, err)
		}
	}
	for _, stmt := range []string{
		`UPDATE point_ledger_entries SET before_snapshot=before_snapshot-'commission',delta_snapshot=delta_snapshot-'commission',after_snapshot=after_snapshot-'commission' WHERE account_id=$1`,
		`UPDATE audit_logs a SET before_json=a.before_json-'commission',after_json=jsonb_set(a.after_json,'{balance}',(a.after_json->'balance')-'commission') FROM point_ledger_entries l WHERE l.account_id=$1 AND a.action='points.'||l.entry_type AND a.resource_type='point_account' AND a.resource_id=l.account_id AND a.after_json->>'ledger_entry_id'=l.id::text`,
	} {
		if _, err = tx.Exec(ctx, stmt, accountID); err != nil {
			t.Fatalf("construct historical three-source evidence (%s): %v", stmt, err)
		}
	}
	for _, stmt := range []string{`ALTER TABLE audit_logs ENABLE TRIGGER audit_immutable`, `ALTER TABLE point_ledger_entries ENABLE TRIGGER ledger_immutable`} {
		if _, err = tx.Exec(ctx, stmt); err != nil {
			t.Fatalf("restore append-only guard (%s): %v", stmt, err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyTwelveBucketBetLedgerCanBeCancelledWithExactSixteenBucketRefund(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	fundBettingWallet(t, f, 10, 10, 10)
	originalWallet := walletBySource(t, f)
	setUserCancellation(t, f, true)
	in := f.input
	in.Multiplier = 25
	in.PolicyVersions = ptrPolicyVersions(readBettingPolicyVersions(t, f.service, f.brand, f.game.ID))
	order, err := placeBettingOrder(t, f, in, "legacy-three-source-cancel")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	var accountID string
	if err = f.db.QueryRow(ctx, `SELECT account_id::text FROM bet_orders WHERE id=$1`, order.ID).Scan(&accountID); err != nil {
		t.Fatal(err)
	}
	var commissionRows, commissionTotal int
	if err = f.db.QueryRow(ctx, `SELECT count(*),coalesce(sum(points),0) FROM point_buckets WHERE account_id=$1 AND source='commission'`, accountID).Scan(&commissionRows, &commissionTotal); err != nil || commissionRows != 4 || commissionTotal != 0 {
		t.Fatalf("legacy fixture must have four zero commission buckets: rows=%d total=%d err=%v", commissionRows, commissionTotal, err)
	}
	synthesizeLegacyThreeSourceLedger(t, f.db, accountID)
	var oldBefore, oldDelta, oldAfter, oldAllocation []byte
	var oldHash string
	if err = f.db.QueryRow(ctx, `SELECT before_snapshot,delta_snapshot,after_snapshot,source_allocation,request_hash FROM point_ledger_entries WHERE id=$1`, order.DebitEntryID).Scan(&oldBefore, &oldDelta, &oldAfter, &oldAllocation, &oldHash); err != nil {
		t.Fatal(err)
	}
	for _, raw := range [][]byte{oldBefore, oldDelta, oldAfter} {
		var snapshot map[string]json.RawMessage
		if err = json.Unmarshal(raw, &snapshot); err != nil || len(snapshot) != 3 {
			t.Fatalf("expected historical 12-bucket snapshot, keys=%d err=%v", len(snapshot), err)
		}
	}
	var immutableOrderBefore []byte
	if err = f.db.QueryRow(ctx, `SELECT jsonb_build_object('deduction_allocation',deduction_allocation,'policy_snapshot',policy_snapshot,'debit_entry_id',debit_entry_id,'definition_hash',definition_hash,'client_key',client_key) FROM bet_orders WHERE id=$1`, order.ID).Scan(&immutableOrderBefore); err != nil {
		t.Fatal(err)
	}
	if rec, e := (points.Store{DB: f.db}).Reconcile(ctx, f.brand, f.member); e != nil || !rec.Consistent {
		t.Fatalf("synthetic legacy-format history/hash must reconcile: %+v err=%v", rec, e)
	}
	tx, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := f.service.Cancel(ctx, tx, f.brand, f.user, order.ID, order.Version, "legacy snapshot refund", points.Metadata{RequestID: ids.New()})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if cancelled.RefundEntryID == "" || walletBySource(t, f) != originalWallet {
		t.Fatalf("historical refund did not exactly restore all source rows: order=%+v wallet=%+v before=%+v", cancelled, walletBySource(t, f), originalWallet)
	}
	var gotBefore, gotDelta, gotAfter, gotAllocation []byte
	var gotHash string
	if err = f.db.QueryRow(ctx, `SELECT before_snapshot,delta_snapshot,after_snapshot,source_allocation,request_hash FROM point_ledger_entries WHERE id=$1`, order.DebitEntryID).Scan(&gotBefore, &gotDelta, &gotAfter, &gotAllocation, &gotHash); err != nil {
		t.Fatal(err)
	}
	if string(gotBefore) != string(oldBefore) || string(gotDelta) != string(oldDelta) || string(gotAfter) != string(oldAfter) || string(gotAllocation) != string(oldAllocation) || gotHash != oldHash {
		t.Fatal("business cancellation rewrote historical ledger evidence")
	}
	var orderLedgerCount int
	if err = f.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE reference_type='bet_order' AND reference_id=$1`, order.ID).Scan(&orderLedgerCount); err != nil || orderLedgerCount != 2 {
		t.Fatalf("expected exactly one debit and one refund record, count=%d err=%v", orderLedgerCount, err)
	}
	var immutableOrderAfter []byte
	if err = f.db.QueryRow(ctx, `SELECT jsonb_build_object('deduction_allocation',deduction_allocation,'policy_snapshot',policy_snapshot,'debit_entry_id',debit_entry_id,'definition_hash',definition_hash,'client_key',client_key) FROM bet_orders WHERE id=$1`, order.ID).Scan(&immutableOrderAfter); err != nil || string(immutableOrderAfter) != string(immutableOrderBefore) {
		t.Fatalf("cancellation rewrote original bet policy/allocation/hash: %s -> %s err=%v", immutableOrderBefore, immutableOrderAfter, err)
	}
	var refundBefore, refundDelta, refundAfter []byte
	if err = f.db.QueryRow(ctx, `SELECT before_snapshot,delta_snapshot,after_snapshot FROM point_ledger_entries WHERE id=$1`, cancelled.RefundEntryID).Scan(&refundBefore, &refundDelta, &refundAfter); err != nil {
		t.Fatal(err)
	}
	for _, raw := range [][]byte{refundBefore, refundDelta, refundAfter} {
		var snapshot map[string]json.RawMessage
		if err = json.Unmarshal(raw, &snapshot); err != nil || len(snapshot) != 4 {
			t.Fatalf("refund must use new sixteen-bucket format: keys=%d err=%v", len(snapshot), err)
		}
	}
	if rec, e := (points.Store{DB: f.db}).Reconcile(ctx, f.brand, f.member); e != nil || !rec.Consistent {
		t.Fatalf("post-refund legacy/new chain must reconcile without rewriting old hashes: %+v err=%v", rec, e)
	}
}

func TestPlaceIdempotencyAndBodyConflict(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	fundBettingWallet(t, f, 20)
	first, err := placeBettingOrder(t, f, f.input, "same-request-key-001")
	if err != nil {
		t.Fatal(err)
	}
	second, err := placeBettingOrder(t, f, f.input, "same-request-key-001")
	if err != nil || second.ID != first.ID {
		t.Fatalf("replay order=%+v first=%+v err=%v", second, first, err)
	}
	changed := f.input
	changed.Selection = rules.Selection{Digits: [][]int{{1}, {2}, {2}}}
	if _, err = placeBettingOrder(t, f, changed, "same-request-key-001"); !errors.Is(err, ErrState) {
		t.Fatalf("same key with different body error=%v, want ErrState", err)
	}
	wallet := walletBySource(t, f)
	if wallet[0][0] != 19 {
		t.Fatalf("replay or conflict debited more than once: %+v", wallet)
	}
	var count int
	if err = f.db.QueryRow(context.Background(), `SELECT count(*) FROM bet_orders WHERE brand_id=$1 AND brand_member_id=$2`, f.brand, f.member).Scan(&count); err != nil || count != 1 {
		t.Fatalf("order count=%d err=%v", count, err)
	}
}

func TestConcurrentDifferentKeysCannotOverspend(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	fundBettingWallet(t, f, 1)
	var wg sync.WaitGroup
	var mu sync.Mutex
	successes := 0
	var unexpected []error
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			in := f.input
			in.Multiplier = 1
			_, err := placeBettingOrder(t, f, in, fmt.Sprintf("concurrent-key-%03d", i))
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				successes++
			} else if !errors.Is(err, points.ErrInsufficient) {
				unexpected = append(unexpected, err)
			}
		}(i)
	}
	wg.Wait()
	if successes != 1 || len(unexpected) != 0 {
		t.Fatalf("successful orders=%d unexpected errors=%v", successes, unexpected)
	}
	if wallet := walletBySource(t, f); wallet[0][0] != 0 {
		t.Fatalf("wallet overdraw or incorrect debit: %+v", wallet)
	}
}

func TestExpiredPeriodDoesNotCreateOrderOrLedgerEntry(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	fundBettingWallet(t, f, 10)
	openShortBettingPeriod(t, &f, 300*time.Millisecond)
	waitForPeriodExpiry(t, f.db, f.period.ID)
	before := walletBySource(t, f)
	if _, err := placeBettingOrder(t, f, f.input, "expired-period-001"); !errors.Is(err, ErrClosed) {
		t.Fatalf("place on expired period=%v, want ErrClosed", err)
	}
	if after := walletBySource(t, f); after != before {
		t.Fatalf("expired bet changed wallet: before=%+v after=%+v", before, after)
	}
	var orders, betEntries int
	if err := f.db.QueryRow(context.Background(), `SELECT count(*) FROM bet_orders WHERE period_id=$1`, f.period.ID).Scan(&orders); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow(context.Background(), `SELECT count(*) FROM point_ledger_entries WHERE entry_type='bet' AND member_id=$1`, f.member).Scan(&betEntries); err != nil {
		t.Fatal(err)
	}
	if orders != 0 || betEntries != 0 {
		t.Fatalf("expired bet persisted order=%d ledger=%d", orders, betEntries)
	}
}

func openShortBettingPeriod(t *testing.T, f *bettingFixture, duration time.Duration) {
	t.Helper()
	finishUnusedBettingPeriod(t, *f)
	ctx := context.Background()
	rs := rulebook.Store{DB: f.db}
	now := time.Now().UTC()
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var err error
		f.period, err = rs.OpenPeriod(ctx, tx, f.brand, f.game.ID, "short-"+ids.New()[:8], now.Add(-time.Minute), now.Add(duration), now.Add(duration+time.Hour))
		return err
	})
	f.input.PeriodID = f.period.ID
}

func lockBettingWallet(t *testing.T, f bettingFixture) pgx.Tx {
	t.Helper()
	ctx := context.Background()
	tx, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var accountID string
	if err = tx.QueryRow(ctx, `SELECT id::text FROM point_accounts WHERE brand_id=$1 AND brand_member_id=$2 FOR UPDATE`, f.brand, f.member).Scan(&accountID); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	return tx
}

func waitForBettingLockWait(t *testing.T, ctx context.Context, db *pgxpool.Pool, pid int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var blocked bool
		if err := db.QueryRow(ctx, `SELECT cardinality(pg_blocking_pids($1))>0 FROM pg_stat_activity WHERE pid=$1`, pid).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(25 * time.Millisecond):
		}
	}
	t.Fatalf("backend %d did not wait on the held wallet lock", pid)
}

func waitForPeriodExpiry(t *testing.T, db *pgxpool.Pool, period string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	for {
		var expired bool
		if err := db.QueryRow(ctx, `SELECT bet_end_at<=clock_timestamp() FROM periods WHERE id=$1`, period).Scan(&expired); err != nil {
			t.Fatal(err)
		}
		if expired {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func runAuthenticatedPlace(t *testing.T, ctx context.Context, f bettingFixture, in Input, key string) (<-chan error, <-chan int) {
	t.Helper()
	result := make(chan error, 1)
	pidResult := make(chan int, 1)
	go func() {
		tx, err := f.db.Begin(ctx)
		if err != nil {
			result <- err
			return
		}
		defer tx.Rollback(ctx)
		var pid int
		if err = tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
			result <- err
			return
		}
		pidResult <- pid
		identityStore, err := identity.New(f.db)
		if err != nil {
			result <- err
			return
		}
		user, err := identityStore.AuthenticateTx(ctx, tx, f.brand, f.token)
		if err == nil {
			_, err = f.service.Place(ctx, tx, f.brand, user, in, key, points.Metadata{RequestID: ids.New()})
		}
		if err == nil {
			err = tx.Commit(ctx)
		}
		result <- err
	}()
	return result, pidResult
}

func TestPlaceRechecksPeriodAfterWalletWait(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	fundBettingWallet(t, f, 10)
	openShortBettingPeriod(t, &f, 800*time.Millisecond)
	blocker := lockBettingWallet(t, f)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	result, pidResult := runAuthenticatedPlace(t, ctx, f, f.input, "wallet-wait-period-001")
	var pid int
	select {
	case pid = <-pidResult:
	case err := <-result:
		t.Fatalf("place finished before acquiring wallet lock: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	waitForBettingLockWait(t, ctx, f.db, pid)
	// Wait against PostgreSQL's clock, so the window is certainly closed before releasing the account.
	for {
		var expired bool
		if err := f.db.QueryRow(ctx, `SELECT bet_end_at<=clock_timestamp() FROM periods WHERE id=$1`, f.period.ID).Scan(&expired); err != nil {
			t.Fatal(err)
		}
		if expired {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}
	if err := blocker.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-result; !errors.Is(err, ErrClosed) {
		t.Fatalf("place after wallet wait error=%v, want ErrClosed", err)
	}
	if wallet := walletBySource(t, f); wallet[0][0] != 10 {
		t.Fatalf("closed-period wait debited wallet: %+v", wallet)
	}
	var orderCount, debitCount int
	if err := f.db.QueryRow(ctx, `SELECT count(*) FROM bet_orders WHERE period_id=$1`, f.period.ID).Scan(&orderCount); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE member_id=$1 AND entry_type='bet'`, f.member).Scan(&debitCount); err != nil {
		t.Fatal(err)
	}
	if orderCount != 0 || debitCount != 0 {
		t.Fatalf("expired window persisted order=%d debit=%d", orderCount, debitCount)
	}
}

func TestPlaceRejectsSessionExpiredDuringWalletWait(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	fundBettingWallet(t, f, 10)
	blocker := lockBettingWallet(t, f)
	if _, err := f.db.Exec(context.Background(), `UPDATE sessions SET expires_at=clock_timestamp()+interval '1200 milliseconds' WHERE id=$1`, f.user.ID); err != nil {
		_ = blocker.Rollback(context.Background())
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	result, pidResult := runAuthenticatedPlace(t, ctx, f, f.input, "wallet-wait-session-001")
	var pid int
	select {
	case pid = <-pidResult:
	case err := <-result:
		t.Fatalf("place finished before wallet lock wait: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	waitForBettingLockWait(t, ctx, f.db, pid)
	for {
		var expired bool
		if err := f.db.QueryRow(ctx, `SELECT expires_at<=clock_timestamp() FROM sessions WHERE id=$1`, f.user.ID).Scan(&expired); err != nil {
			t.Fatal(err)
		}
		if expired {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}
	if err := blocker.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-result; !errors.Is(err, identity.ErrSession) {
		t.Fatalf("place after session expiry error=%v, want ErrSession", err)
	}
	if wallet := walletBySource(t, f); wallet[0][0] != 10 {
		t.Fatalf("expired-session wait debited wallet: %+v", wallet)
	}
	var orderCount, debitCount int
	if err := f.db.QueryRow(ctx, `SELECT count(*) FROM bet_orders WHERE brand_id=$1 AND brand_member_id=$2`, f.brand, f.member).Scan(&orderCount); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE member_id=$1 AND entry_type='bet'`, f.member).Scan(&debitCount); err != nil {
		t.Fatal(err)
	}
	if orderCount != 0 || debitCount != 0 {
		t.Fatalf("expired session persisted order=%d debit=%d", orderCount, debitCount)
	}
}

func TestCancelRejectsSessionExpiredDuringWalletWaitWithoutRefund(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	fundBettingWallet(t, f, 10)
	setUserCancellation(t, f, true)
	f.input.PolicyVersions = ptrPolicyVersions(readBettingPolicyVersions(t, f.service, f.brand, f.game.ID))
	order, err := placeBettingOrder(t, f, f.input, "cancel-expiry-order-001")
	if err != nil {
		t.Fatal(err)
	}
	debitBalance := walletBySource(t, f)
	blocker := lockBettingWallet(t, f)
	if _, err = f.db.Exec(context.Background(), `UPDATE sessions SET expires_at=clock_timestamp()+interval '1200 milliseconds' WHERE id=$1`, f.user.ID); err != nil {
		_ = blocker.Rollback(context.Background())
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	result := make(chan error, 1)
	pidResult := make(chan int, 1)
	go func() {
		tx, e := f.db.Begin(ctx)
		if e != nil {
			result <- e
			return
		}
		defer tx.Rollback(ctx)
		var pid int
		if e = tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); e != nil {
			result <- e
			return
		}
		pidResult <- pid
		identityStore, e := identity.New(f.db)
		var user identity.Session
		if e == nil {
			user, e = identityStore.AuthenticateTx(ctx, tx, f.brand, f.token)
		}
		if e == nil {
			_, e = f.service.Cancel(ctx, tx, f.brand, user, order.ID, order.Version, "cancel after session expiry", points.Metadata{RequestID: ids.New()})
		}
		if e == nil {
			e = tx.Commit(ctx)
		}
		result <- e
	}()
	var pid int
	select {
	case pid = <-pidResult:
	case err = <-result:
		t.Fatalf("cancel finished before wallet wait: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	waitForBettingLockWait(t, ctx, f.db, pid)
	for {
		var expired bool
		if err = f.db.QueryRow(ctx, `SELECT expires_at<=clock_timestamp() FROM sessions WHERE id=$1`, f.user.ID).Scan(&expired); err != nil {
			t.Fatal(err)
		}
		if expired {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}
	if err = blocker.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-result; !errors.Is(err, identity.ErrSession) {
		t.Fatalf("cancel after session expiry error=%v, want ErrSession", err)
	}
	if got := walletBySource(t, f); got != debitBalance {
		t.Fatalf("expired-session cancel refunded wallet: before=%+v after=%+v", debitBalance, got)
	}
	loaded, err := f.service.Order(ctx, f.brand, f.member, order.ID)
	if err != nil || loaded.Status != "placed" || loaded.RefundEntryID != "" {
		t.Fatalf("expired-session cancel changed order=%+v err=%v", loaded, err)
	}
}

func ptrPolicyVersions(v PolicyVersions) *PolicyVersions { return &v }

func TestBrandAndGameIsolation(t *testing.T) {
	a := newBettingFixture(t, storeTestBrand)
	b := newBettingFixture(t, storeTestOtherBrand)
	fundBettingWallet(t, a, 10)
	fundBettingWallet(t, b, 10)
	if _, err := placeBettingOrder(t, a, b.input, "cross-game-key-001"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign game configuration error=%v, want ErrNotFound", err)
	}
	if _, err := placeBettingOrder(t, b, a.input, "cross-brand-key-001"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign brand period error=%v, want ErrNotFound", err)
	}
	if got, err := a.service.Order(context.Background(), storeTestOtherBrand, a.member, "00000000-0000-0000-0000-000000000001"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign brand order read=%+v err=%v", got, err)
	}
	if wallet := walletBySource(t, a); wallet[0][0] != 10 {
		t.Fatalf("foreign configuration changed wallet: %+v", wallet)
	}
}

func TestPausedBrandAndFrozenMemberCannotBetButCanCancelOwnOrder(t *testing.T) {
	for _, state := range []struct {
		name  string
		setup func(*testing.T, bettingFixture)
	}{
		{name: "paused brand", setup: func(t *testing.T, f bettingFixture) {
			if _, err := f.db.Exec(context.Background(), `UPDATE brands SET status='paused' WHERE id=$1`, f.brand); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "frozen member", setup: func(t *testing.T, f bettingFixture) {
			if _, err := f.db.Exec(context.Background(), `UPDATE brand_members SET status='frozen' WHERE id=$1`, f.member); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(state.name, func(t *testing.T) {
			f := newBettingFixture(t, storeTestBrand)
			fundBettingWallet(t, f, 10)
			setUserCancellation(t, f, true)
			// Use the current policy version in the order request after enabling cancellation.
			versions := readBettingPolicyVersions(t, f.service, f.brand, f.game.ID)
			var err error
			f.input.PolicyVersions = &versions
			order, err := placeBettingOrder(t, f, f.input, "status-cancel-key-001")
			if err != nil {
				t.Fatal(err)
			}
			state.setup(t, f)
			if _, err = placeBettingOrder(t, f, f.input, "status-new-bet-key-001"); !errors.Is(err, ErrDenied) {
				t.Fatalf("new bet while %s error=%v, want ErrDenied", state.name, err)
			}
			tx, err := f.db.Begin(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			cancelled, err := f.service.Cancel(context.Background(), tx, f.brand, f.user, order.ID, order.Version, "cancel during status change", points.Metadata{RequestID: ids.New()})
			if err != nil {
				_ = tx.Rollback(context.Background())
				t.Fatalf("own cancellation while %s: %v", state.name, err)
			}
			if err = tx.Commit(context.Background()); err != nil {
				t.Fatal(err)
			}
			if cancelled.Status != "bet_cancelled" || walletBySource(t, f)[0][0] != 10 {
				t.Fatalf("cancel result=%+v wallet=%+v", cancelled, walletBySource(t, f))
			}
		})
	}
}

func setUserCancellation(t *testing.T, f bettingFixture, allowed bool) {
	t.Helper()
	ctx := context.Background()
	brandPolicy, err := f.service.BrandPolicy(ctx, f.brand)
	if err != nil {
		t.Fatal(err)
	}
	brandPolicy.Config.UserCancelAllowed = allowed
	actorID := ids.New()
	if _, err = f.db.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'test-only')`, actorID, "bet_policy_"+actorID[:8]); err != nil {
		t.Fatal(err)
	}
	actor := policyActor(actorID, f.brand)
	policyTx(t, f.db, func(tx pgx.Tx) error {
		_, e := WriteBrandPolicy(ctx, tx, f.brand, actor, brandPolicy.Version, brandPolicy.Config, "enable test user cancellation", policyMeta(actorID))
		return e
	})
}

func TestOrderSnapshotsPolicyVersionAndResolvedPolicy(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	fundBettingWallet(t, f, 10)
	beforeVersions := readBettingPolicyVersions(t, f.service, f.brand, f.game.ID)
	var err error
	f.input.PolicyVersions = &beforeVersions
	order, err := placeBettingOrder(t, f, f.input, "policy-snapshot-key-001")
	if err != nil {
		t.Fatal(err)
	}
	actorID := ids.New()
	if _, err = f.db.Exec(context.Background(), `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'test-only')`, actorID, "bet_policy_snapshot_"+actorID[:8]); err != nil {
		t.Fatal(err)
	}
	brandPolicy, err := f.service.BrandPolicy(context.Background(), f.brand)
	if err != nil {
		t.Fatal(err)
	}
	brandPolicy.Config.MinBetPoints = 2
	policyTx(t, f.db, func(tx pgx.Tx) error {
		_, e := WriteBrandPolicy(context.Background(), tx, f.brand, policyActor(actorID, f.brand), brandPolicy.Version, brandPolicy.Config, "change policy after order", policyMeta(actorID))
		return e
	})
	loaded, err := f.service.Order(context.Background(), f.brand, f.member, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.PolicyVersions != beforeVersions || loaded.Policy.MinBetPoints != 1 || loaded.TotalPoints != 1 {
		t.Fatalf("order did not preserve placement policy snapshot: versions=%+v policy=%+v total=%d", loaded.PolicyVersions, loaded.Policy, loaded.TotalPoints)
	}
	current := readBettingPolicyVersions(t, f.service, f.brand, f.game.ID)
	if current.Brand != beforeVersions.Brand+1 {
		t.Fatalf("current versions=%+v before=%+v err=%v", current, beforeVersions, err)
	}
}
