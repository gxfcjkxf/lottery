package rewards

import (
	"context"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"reflect"
	"testing"
)

func TestInsufficientRevocationLeavesOrderWalletActionsAndEventsUnchanged(t *testing.T) {
	f := newRewardFixture(t)
	ctx := context.Background()
	order := rewardGrant(t, f, 30)
	rewardCredit(t, f, "recharge", 100, "other-source")
	rewardMove(t, f, "gift", "available", "manual_frozen", 20)
	before := rewardBalance(t, f)
	fingerprint := func() string {
		var v string
		if err := f.db.QueryRow(ctx, `SELECT concat_ws(':',(SELECT count(*) FROM reward_order_actions),(SELECT count(*) FROM point_ledger_entries),(SELECT count(*) FROM audit_logs),(SELECT count(*) FROM outbox_events))`).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	facts := fingerprint()
	_, err := rewardRevoke(t, f, order, f.actor, "cancel original grant")
	if !errors.Is(err, points.ErrInsufficient) {
		t.Fatalf("want insufficient original gift, got %v", err)
	}
	current, err := f.service.OrderTx(ctx, rewardTx(t, f.db), f.brand, order.ID)
	if err != nil || !reflect.DeepEqual(order, current) {
		t.Fatalf("order changed: %+v %v", current, err)
	}
	if rewardBalance(t, f) != before || fingerprint() != facts {
		t.Fatal("failed reversal changed wallet or financial evidence")
	}
	// Explicitly restoring gift availability allows a new normal revoke, exactly once.
	rewardMove(t, f, "gift", "manual_frozen", "available", 20)
	revoked, err := rewardRevoke(t, f, order, f.actor, "cancel original grant")
	if err != nil || revoked.State != "revoked" || revoked.Version != 2 {
		t.Fatalf("revoke: %+v %v", revoked, err)
	}
	after := rewardBalance(t, f)
	if after[2][0] != 0 || after[0][0] != 100 {
		t.Fatalf("wrong source deducted: %+v", after)
	}
	_, err = rewardRevoke(t, f, revoked, f.actor, "repeat cancellation")
	if !errors.Is(err, ErrState) {
		t.Fatalf("duplicate revoke: %v", err)
	}
	if rewardBalance(t, f) != after {
		t.Fatal("duplicate revoke moved points")
	}
}
