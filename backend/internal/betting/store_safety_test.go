package betting

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rulebook"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

func TestBetFailureAfterDebitRollsBackWalletHistoryAndEvents(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	fundBettingWallet(t, f, 10)
	ctx := context.Background()
	before := walletBySource(t, f)
	// Fail at the outbox boundary, after both the debit and order were inserted.
	if _, err := f.db.Exec(ctx, `CREATE FUNCTION reject_bet_event() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.event_type='bet.order.placed' THEN RAISE EXCEPTION 'test outbox failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_bet_event BEFORE INSERT ON outbox_events FOR EACH ROW EXECUTE FUNCTION reject_bet_event()`); err != nil {
		t.Fatal(err)
	}
	if _, err := placeBettingOrder(t, f, f.input, "failed-outbox-key-001"); err == nil {
		t.Fatal("expected outbox failure")
	}
	if got := walletBySource(t, f); got != before {
		t.Fatalf("failed business transaction changed balance: %+v", got)
	}
	var orders, debits, events, audits int
	if err := f.db.QueryRow(ctx, `SELECT (SELECT count(*) FROM bet_orders),(SELECT count(*) FROM point_ledger_entries WHERE entry_type='bet'),(SELECT count(*) FROM outbox_events WHERE event_type='bet.order.placed'),(SELECT count(*) FROM audit_logs WHERE action='bet.place')`).Scan(&orders, &debits, &events, &audits); err != nil {
		t.Fatal(err)
	}
	if orders != 0 || debits != 0 || events != 0 || audits != 0 {
		t.Fatalf("failed bet left order/debit/event/audit: %d/%d/%d/%d", orders, debits, events, audits)
	}
}

func TestConcurrentDifferentMembersCannotExceedPeriodQuota(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	other := f
	other.user.User.ID, other.member, other.user.ID = ids.New(), ids.New(), ids.New()
	other.user.Member.ID = other.member
	account := ids.New()
	hash := sha256.Sum256([]byte(other.user.ID))
	for _, stmt := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO global_users(id,username) VALUES($1,$2)`, []any{other.user.User.ID, "quota_user_" + other.user.User.ID}},
		{`INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version) VALUES($1,$2,$3,'domain','test-1','test-1')`, []any{other.member, f.brand, other.user.User.ID}},
		{`INSERT INTO point_accounts(id,brand_id,brand_member_id) VALUES($1,$2,$3)`, []any{account, f.brand, other.member}},
		{`INSERT INTO point_buckets(brand_id,account_id,source,state) SELECT $1,$2,s,t FROM unnest(ARRAY['recharge','winning','gift'])s CROSS JOIN unnest(ARRAY['available','manual_frozen','system_frozen','withdrawal'])t`, []any{f.brand, account}},
		{`INSERT INTO sessions(id,token_hash,user_id,member_id,brand_id,expires_at) VALUES($1,$2,$3,$4,$5,clock_timestamp()+interval '1 hour')`, []any{other.user.ID, fmt.Sprintf("%x", hash), other.user.User.ID, other.member, f.brand}},
	} {
		if _, err := f.db.Exec(ctx, stmt.sql, stmt.args...); err != nil {
			t.Fatal(err)
		}
	}
	fundBettingWallet(t, f, 10)
	fundBettingWallet(t, other, 10)
	actorID := ids.New()
	if _, err := f.db.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'test-only')`, actorID, "concurrent_quota_"+actorID); err != nil {
		t.Fatal(err)
	}
	policy, err := f.service.BrandPolicy(ctx, f.brand)
	if err != nil {
		t.Fatal(err)
	}
	policy.Config.MaxPeriodPoints = amountPtr(1)
	policyTx(t, f.db, func(tx pgx.Tx) error {
		_, e := WriteBrandPolicy(ctx, tx, f.brand, policyActor(actorID, f.brand), policy.Version, policy.Config, "cap concurrent admissions", policyMeta(actorID))
		return e
	})
	versions := readBettingPolicyVersions(t, f.service, f.brand, f.game.ID)
	f.input.PolicyVersions, other.input.PolicyVersions = &versions, &versions
	start, results := make(chan struct{}), make(chan error, 2)
	for i, member := range []bettingFixture{f, other} {
		go func(index int, member bettingFixture) {
			<-start
			tx, e := member.db.Begin(ctx)
			if e != nil {
				results <- e
				return
			}
			defer tx.Rollback(ctx)
			_, e = member.service.Place(ctx, tx, member.brand, member.user, member.input, fmt.Sprintf("different-member-key-%d", index), points.Metadata{RequestID: ids.New()})
			if e == nil {
				e = tx.Commit(ctx)
			}
			results <- e
		}(i, member)
	}
	close(start)
	successes, limited := 0, 0
	for i := 0; i < 2; i++ {
		select {
		case e := <-results:
			if e == nil {
				successes++
			} else if errors.Is(e, ErrLimit) {
				limited++
			} else {
				t.Fatal(e)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if successes != 1 || limited != 1 {
		t.Fatalf("admissions=%d limited=%d", successes, limited)
	}
	var exposure int64
	if err = f.db.QueryRow(ctx, `SELECT sum(total_points) FROM bet_orders WHERE period_id=$1`, f.period.ID).Scan(&exposure); err != nil || exposure != 1 {
		t.Fatalf("exposure=%d err=%v", exposure, err)
	}
}

func TestImmediateRuleApprovalAffectsOnlyNewOrders(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	fundBettingWallet(t, f, 10)
	old, err := placeBettingOrder(t, f, f.input, "old-rule-order-key")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	rs := rulebook.Store{DB: f.db}
	creator := bettingRuleActor(f.version.CreatedBy, f.brand, "write", "validate", "submit")
	reviewer := bettingRuleActor(f.version.ReviewedBy, f.brand, "review")
	definition := bettingDefinition()
	definition.PrizeTiers[0].Odds = "20"
	var next rulebook.Version
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var e error
		next, e = rs.CreateVersion(ctx, tx, f.brand, creator, f.play.ID, definition, "immediate", "new prospective odds", points.Metadata{})
		return e
	})
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var e error
		next, e = rs.Validate(ctx, tx, f.brand, creator, next.ID, next.Version, []rules.ValidationCase{{Name: "exact", Selection: f.input.Selection, Draw: rules.Draw{Digits: []int{1, 2, 1}}, Multiplier: 1, ExpectedBetPoints: amountPtr(1), ExpectedPrizePoints: amountPtr(20), ExpectedWon: boolPtr(true)}}, "validate prospective odds", points.Metadata{})
		return e
	})
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var e error
		next, e = rs.Submit(ctx, tx, f.brand, creator, next.ID, next.Version, "submit prospective odds", points.Metadata{})
		return e
	})
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var e error
		next, e = rs.Review(ctx, tx, f.brand, reviewer, next.ID, next.Version, true, true, "approve prospective odds", points.Metadata{})
		return e
	})
	if _, err = placeBettingOrder(t, f, f.input, "stale-rule-order-key"); !errors.Is(err, ErrVersion) {
		t.Fatalf("stale confirmation was accepted: %v", err)
	}
	in := f.input
	in.RuleVersionID = next.ID
	newOrder, err := placeBettingOrder(t, f, in, "new-rule-order-key")
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := f.service.Order(ctx, f.brand, f.member, old.ID)
	if err != nil || loaded.Definition.PrizeTiers[0].Odds != "10" || newOrder.Definition.PrizeTiers[0].Odds != "20" || loaded.PeriodID != newOrder.PeriodID {
		t.Fatalf("wrong prospective odds old=%+v new=%+v err=%v", loaded, newOrder, err)
	}
	var openingVersion string
	if err = f.db.QueryRow(ctx, `SELECT rule_version_id::text FROM period_rule_versions WHERE period_id=$1 AND play_id=$2`, f.period.ID, f.play.ID).Scan(&openingVersion); err != nil || openingVersion != old.RuleVersionID {
		t.Fatalf("opening audit snapshot should remain unchanged: %s err=%v", openingVersion, err)
	}
}

func TestRefundFailureDoesNotChangeOrderOrWallet(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	fundBettingWallet(t, f, 10)
	setUserCancellation(t, f, true)
	versions := readBettingPolicyVersions(t, f.service, f.brand, f.game.ID)
	f.input.PolicyVersions = &versions
	o, err := placeBettingOrder(t, f, f.input, "refund-failure-bet-key")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	before := walletBySource(t, f)
	if _, err = f.db.Exec(ctx, `CREATE FUNCTION reject_cancel_event() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.event_type='bet.order.cancelled' THEN RAISE EXCEPTION 'test refund outbox failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_cancel_event BEFORE INSERT ON outbox_events FOR EACH ROW EXECUTE FUNCTION reject_cancel_event()`); err != nil {
		t.Fatal(err)
	}
	tx, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.service.Cancel(ctx, tx, f.brand, f.user, o.ID, o.Version, "test failed refund", points.Metadata{RequestID: ids.New()})
	_ = tx.Rollback(ctx)
	if err == nil {
		t.Fatal("expected refund outbox failure")
	}
	loaded, err := f.service.Order(ctx, f.brand, f.member, o.ID)
	if err != nil || loaded.Status != "placed" || loaded.RefundEntryID != "" || loaded.Version != o.Version || walletBySource(t, f) != before {
		t.Fatalf("failed cancellation left changes: %+v err=%v", loaded, err)
	}
	var refunds int
	if err = f.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE reversal_of=$1`, o.DebitEntryID).Scan(&refunds); err != nil || refunds != 0 {
		t.Fatalf("failed cancellation left reversal: %d err=%v", refunds, err)
	}
}

func TestBetHistoryRejectsEditsDeletesAndUnfundedCancellation(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	fundBettingWallet(t, f, 10)
	o, err := placeBettingOrder(t, f, f.input, "immutable-bet-key-001")
	if err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		`DELETE FROM bet_orders WHERE id=$1`,
		`UPDATE bet_orders SET client_key='changed-client-key',version=version+1 WHERE id=$1`,
		`UPDATE bet_orders SET total_points=2,version=version+1 WHERE id=$1`,
		`UPDATE bet_orders SET status='bet_cancelled',version=version+1,cancel_reason='unsupported refund' WHERE id=$1`,
	} {
		if _, err := f.db.Exec(context.Background(), sql, o.ID); err == nil {
			t.Fatalf("history guard accepted %s", sql)
		}
	}
	got, err := f.service.Order(context.Background(), f.brand, f.member, o.ID)
	if err != nil || got.Version != o.Version || got.Status != "placed" || got.TotalPoints != 1 {
		t.Fatalf("history changed after rejected edits: %+v err=%v", got, err)
	}
}

func TestPeriodQuotaIsReleasedOnlyByExactRefund(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	fundBettingWallet(t, f, 10)
	setUserCancellation(t, f, true)
	ctx := context.Background()
	actorID := ids.New()
	if _, err := f.db.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'test-only')`, actorID, "quota_"+actorID); err != nil {
		t.Fatal(err)
	}
	policy, err := f.service.BrandPolicy(ctx, f.brand)
	if err != nil {
		t.Fatal(err)
	}
	policy.Config.MaxPeriodPoints = amountPtr(1)
	policy.Config.MaxUserPeriodPoints = amountPtr(1)
	policyTx(t, f.db, func(tx pgx.Tx) error {
		_, e := WriteBrandPolicy(ctx, tx, f.brand, policyActor(actorID, f.brand), policy.Version, policy.Config, "limit period exposure", policyMeta(actorID))
		return e
	})
	versions := readBettingPolicyVersions(t, f.service, f.brand, f.game.ID)
	f.input.PolicyVersions = &versions
	first, err := placeBettingOrder(t, f, f.input, "quota-first-key-001")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = placeBettingOrder(t, f, f.input, "quota-blocked-key-001"); !errors.Is(err, ErrLimit) {
		t.Fatalf("full quota admission=%v", err)
	}
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		_, e := f.service.Cancel(ctx, tx, f.brand, f.user, first.ID, first.Version, "release quota", points.Metadata{RequestID: ids.New()})
		return e
	})
	if _, err = placeBettingOrder(t, f, f.input, "quota-next-key-001"); err != nil {
		t.Fatalf("refund did not release quota: %v", err)
	}
	if got := walletBySource(t, f); got[0][0] != 9 {
		t.Fatalf("expected exactly one outstanding stake: %+v", got)
	}
}
