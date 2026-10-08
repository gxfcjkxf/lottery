package rewards

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

func rewardEvidence(t *testing.T, f rewardFixture) string {
	t.Helper()
	var out string
	if err := f.db.QueryRow(context.Background(), `SELECT jsonb_build_object(
 'orders',(SELECT jsonb_agg(to_jsonb(x) ORDER BY x.id) FROM reward_orders x),
 'actions',(SELECT jsonb_agg(to_jsonb(x) ORDER BY x.id) FROM reward_order_actions x),
 'accounts',(SELECT jsonb_agg(to_jsonb(x) ORDER BY x.id) FROM point_accounts x),
 'buckets',(SELECT jsonb_agg(to_jsonb(x) ORDER BY x.account_id,x.source,x.state) FROM point_buckets x),
 'ledger',(SELECT jsonb_agg(to_jsonb(x) ORDER BY x.id) FROM point_ledger_entries x),
 'audits',(SELECT jsonb_agg(to_jsonb(x) ORDER BY x.id) FROM audit_logs x))::text`).Scan(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestRewardMaxInt64AndCurrentBalanceLimit(t *testing.T) {
	t.Run("exact full-range grant and reversal", func(t *testing.T) {
		f := newRewardFixture(t)
		o := rewardGrant(t, f, points.Amount(math.MaxInt64))
		if o.Points != math.MaxInt64 || rewardBalance(t, f)[2][0] != math.MaxInt64 {
			t.Fatal("grant lost exact integer precision")
		}
		revoked, err := rewardRevoke(t, f, o, f.actor, "reverse exact maximum reward")
		if err != nil || revoked.State != "revoked" || rewardBalance(t, f) != (points.Balance{}) {
			t.Fatalf("max reward reversal=%+v err=%v", revoked, err)
		}
	})
	t.Run("failed over-cap grant leaves no business evidence", func(t *testing.T) {
		f := newRewardFixture(t)
		cap := points.Amount(10)
		tx := rewardTx(t, f.db)
		if _, err := f.points.UpdatePolicy(context.Background(), tx, f.brand, points.PolicyInput{Version: 1, MaxBalancePoints: &cap, Reason: "owned reward balance cap"}, rewardMeta(f.admin)); err != nil {
			t.Fatal(err)
		}
		rewardCommit(t, tx)
		before := rewardEvidence(t, f)
		tx = rewardTx(t, f.db)
		_, err := f.service.GrantTx(context.Background(), tx, f.brand, f.actor, GrantInput{MemberID: f.member, Points: 11, Reason: "reject over cap"}, rewardMeta(f.admin))
		_ = tx.Rollback(context.Background())
		if !errors.Is(err, points.ErrPolicyLimit) || rewardEvidence(t, f) != before {
			t.Fatalf("over cap reward err=%v or business evidence changed", err)
		}
	})
}

func TestRewardFullGenericReverseAndStandaloneCreditCannotBypassBusiness(t *testing.T) {
	f := newRewardFixture(t)
	o := rewardGrant(t, f, 25)
	before := rewardEvidence(t, f)
	tx := rewardTx(t, f.db)
	_, err := f.points.Reverse(context.Background(), tx, f.brand, f.member, o.GrantLedgerEntryID, "reward-bypass:"+ids.New(), "generic full reverse is not approval", rewardMeta(f.admin))
	_ = tx.Rollback(context.Background())
	if err == nil || !strings.Contains(err.Error(), "reward posting requires current explicit business action") {
		t.Fatalf("generic full reward reversal bypass=%v", err)
	}
	tx = rewardTx(t, f.db)
	var delta points.Balance
	delta[2][0] = 5
	_, err = f.points.Post(context.Background(), tx, points.Change{BrandID: f.brand, MemberID: f.member, EntryType: "reward_grant", ReferenceType: "reward_order", ReferenceID: ids.New(),
		OperationKey: "reward-bypass:" + ids.New(), Reason: "business-less reward grant", ActorType: "admin", ActorID: f.admin, RequestID: ids.New(),
		Delta: delta, Allocation: []points.Allocation{{Source: "gift", State: "available", Points: 5}}})
	_ = tx.Rollback(context.Background())
	if err == nil || !strings.Contains(err.Error(), "reward posting requires current explicit business action") {
		t.Fatalf("business-less reward grant bypass=%v", err)
	}
	if rewardEvidence(t, f) != before {
		t.Fatal("rejected reward bypass changed business or ledger history")
	}
}

func TestRewardPendingRequiresRealGiftShortageEvenWithMatchingAudit(t *testing.T) {
	f := newRewardFixture(t)
	o := rewardGrant(t, f, 25)
	before := rewardEvidence(t, f)
	tx := rewardTx(t, f.db)
	actionID := ids.New()
	log, err := audit.Append(context.Background(), tx, audit.Record{BrandID: f.brand, ActorType: "admin", ActorID: f.admin,
		Action: "reward.order.revoke", ResourceType: "reward_order", ResourceID: o.ID, Reason: "counterfeit shortage", RequestID: ids.New(),
		Before: map[string]any{"version": 1, "state": "granted"}, After: map[string]any{"action_id": actionID, "version": 2, "state": "revocation_pending"}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(context.Background(), `INSERT INTO reward_order_actions(id,brand_id,order_id,version,operation,state_before,state_after,actor_id,reason,audit_log_id) VALUES($1,$2,$3,2,'revoke','granted','revocation_pending',$4,'counterfeit shortage',$5)`, actionID, f.brand, o.ID, f.admin, log)
	_ = tx.Rollback(context.Background())
	if err == nil || !strings.Contains(err.Error(), "reward pending requires original gift available shortage") {
		t.Fatalf("counterfeit gift shortage=%v", err)
	}
	if rewardEvidence(t, f) != before {
		t.Fatal("counterfeit shortage changed reward or financial evidence")
	}
}

func TestRewardOrphanGrantCannotCommit(t *testing.T) {
	f := newRewardFixture(t)
	before := rewardEvidence(t, f)
	tx := rewardTx(t, f.db)
	id, actionID := ids.New(), ids.New()
	log, err := actionAudit(context.Background(), tx, f.brand, id, actionID, "grant", "granted", 1, nil, "orphan matching audit", f.actor, rewardMeta(f.admin), map[string]any{"member_id": f.member, "points": points.Amount(10)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(context.Background(), `INSERT INTO reward_orders(id,brand_id,member_id,points,state,creation_audit_log_id,last_audit_log_id,created_by,reason,point_policy_version) VALUES($1,$2,$3,10,'granted',$4,$4,$5,'orphan matching audit',1)`, id, f.brand, f.member, log, f.admin)
	if err != nil {
		t.Fatal(err)
	}
	err = tx.Commit(context.Background())
	if err == nil || !strings.Contains(err.Error(), "orphan or incomplete reward order cannot commit") {
		t.Fatalf("orphan reward committed: %v", err)
	}
	if rewardEvidence(t, f) != before {
		t.Fatal("orphan reward commit failure changed existing evidence")
	}
}

func TestRewardReadPagesAreBrandScopedAndStrict(t *testing.T) {
	f := newRewardFixture(t)
	o := rewardGrant(t, f, 5)
	before := rewardEvidence(t, f)
	tx, err := f.db.BeginTx(context.Background(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	for _, bounds := range [][2]int{{0, 0}, {101, 0}, {20, -1}, {20, 1000001}} {
		if _, err := f.service.OrdersTx(context.Background(), tx, f.brand, bounds[0], bounds[1]); !errors.Is(err, ErrInvalid) {
			t.Fatalf("bad page %v err=%v", bounds, err)
		}
	}
	for _, args := range []struct {
		brand  string
		offset int
	}{{f.brand, 1000000}, {rewardTestOtherBrand, 0}} {
		p, err := f.service.OrdersTx(context.Background(), tx, args.brand, 20, args.offset)
		if err != nil || len(p.Items) != 0 || p.Items == nil {
			t.Fatalf("empty page=%+v err=%v", p, err)
		}
	}
	if _, err := f.service.ActionsTx(context.Background(), tx, rewardTestOtherBrand, o.ID, 20, 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign history err=%v", err)
	}
	if err := tx.Commit(context.Background()); err != nil || rewardEvidence(t, f) != before {
		t.Fatalf("read pages changed business evidence, err=%v", err)
	}
}

func TestRewardCreditGuardRejectsWrongSourceAmountActorOrKey(t *testing.T) {
	for _, mode := range []string{"source", "amount", "actor", "key"} {
		t.Run(mode, func(t *testing.T) {
			f := newRewardFixture(t)
			before := rewardEvidence(t, f)
			tx := rewardTx(t, f.db)
			id, actionID := ids.New(), ids.New()
			reason := "guarded grant identity"
			log, err := actionAudit(context.Background(), tx, f.brand, id, actionID, "grant", "granted", 1, nil, reason, f.actor, rewardMeta(f.admin), map[string]any{"member_id": f.member, "points": points.Amount(25)})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = tx.Exec(context.Background(), `INSERT INTO reward_orders(id,brand_id,member_id,points,state,creation_audit_log_id,last_audit_log_id,created_by,reason,point_policy_version) VALUES($1,$2,$3,25,'granted',$4,$4,$5,$6,1)`, id, f.brand, f.member, log, f.admin, reason); err != nil {
				t.Fatal(err)
			}
			if err = insertAction(context.Background(), tx, f.brand, id, actionID, "grant", "granted", 1, nil, f.admin, reason, log); err != nil {
				t.Fatal(err)
			}
			amount, source, sourceIndex := points.Amount(25), "gift", 2
			actorType, actorID, key := "admin", f.admin, "reward-grant:"+id
			switch mode {
			case "source":
				source, sourceIndex = "winning", 1
			case "amount":
				amount = 26
			case "actor":
				actorType, actorID = "system", ""
			case "key":
				key += ":wrong"
			}
			var delta points.Balance
			delta[sourceIndex][0] = amount
			_, err = f.points.Post(context.Background(), tx, points.Change{BrandID: f.brand, MemberID: f.member, EntryType: "reward_grant", ReferenceType: "reward_order", ReferenceID: id,
				OperationKey: key, Reason: reason, ActorType: actorType, ActorID: actorID, RequestID: ids.New(), Delta: delta,
				Allocation: []points.Allocation{{Source: source, State: "available", Points: amount}}})
			_ = tx.Rollback(context.Background())
			if err == nil || !strings.Contains(err.Error(), "reward") {
				t.Fatalf("%s malformed posting did not reach/reject authoritative reward guard: %v", mode, err)
			}
			if rewardEvidence(t, f) != before {
				t.Fatal("rejected malformed reward posting changed history or funds")
			}
		})
	}
}

func TestRewardStaleActorCannotWriteAfterDatabasePromotesItToSuperAdmin(t *testing.T) {
	f := newRewardFixture(t)
	if _, err := f.db.Exec(context.Background(), `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.admin); err != nil {
		t.Fatal(err)
	}
	before := rewardEvidence(t, f)
	tx := rewardTx(t, f.db)
	_, err := f.service.GrantTx(context.Background(), tx, f.brand, f.actor, GrantInput{MemberID: f.member, Points: 25, Reason: "stale actor must not grant"}, rewardMeta(f.admin))
	_ = tx.Rollback(context.Background())
	if !errors.Is(err, ErrDenied) || rewardEvidence(t, f) != before {
		t.Fatalf("stale ordinary actor over real super-admin identity err=%v or evidence changed", err)
	}
	// Bypass Go admission with an otherwise matching audit. The SQL guard must
	// also use the real account flag rather than trust the claimed actor role.
	tx = rewardTx(t, f.db)
	id, actionID := ids.New(), ids.New()
	log, err := actionAudit(context.Background(), tx, f.brand, id, actionID, "grant", "granted", 1, nil, "super SQL bypass", f.actor, rewardMeta(f.admin), map[string]any{"member_id": f.member, "points": points.Amount(25)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(context.Background(), `INSERT INTO reward_orders(id,brand_id,member_id,points,state,creation_audit_log_id,last_audit_log_id,created_by,reason,point_policy_version) VALUES($1,$2,$3,25,'granted',$4,$4,$5,'super SQL bypass',1)`, id, f.brand, f.member, log, f.admin)
	_ = tx.Rollback(context.Background())
	if err == nil || !strings.Contains(err.Error(), "reward grant requires active ordinary operator") || rewardEvidence(t, f) != before {
		t.Fatalf("direct SQL over real super-admin identity err=%v or evidence changed", err)
	}
}
