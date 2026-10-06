package betting

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/notification"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"testing"
)

func TestActualBetEventsMaterializeAllSupportedStatesWithoutChangingBalances(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	ctx := context.Background()
	fundBettingWallet(t, f, 30)
	setUserCancellation(t, f, true)
	f.input.PolicyVersions = ptrPolicyVersions(readBettingPolicyVersions(t, f.service, f.brand, f.game.ID))
	first, e := placeBettingOrder(t, f, f.input, "notification-bet-first")
	if e != nil {
		t.Fatal(e)
	}
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		_, e := f.service.Cancel(ctx, tx, f.brand, f.user, first.ID, first.Version, "test cancellation", points.Metadata{RequestID: ids.New()})
		return e
	})
	second, e := placeBettingOrder(t, f, f.input, "notification-bet-second")
	if e != nil {
		t.Fatal(e)
	}
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		_, e := f.service.MarkAbnormal(ctx, tx, f.brand, exceptionActor(f), second.ID, second.Version, "internal anomaly details", policyMeta(f.version.CreatedBy))
		return e
	})
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		_, e := f.service.JudgeCancel(ctx, tx, f.brand, judgeActor(f), second.ID, JudgeCancelInput{Version: 2, Cause: "no_result", Reason: "internal judgment evidence"}, policyMeta(f.version.CreatedBy))
		return e
	})
	before := walletBySource(t, f)
	s := notification.Service{DB: f.db}
	if _, e = s.Process(ctx, 20); e != nil {
		t.Fatal(e)
	}
	page, e := s.List(ctx, f.brand, f.member, 20, 0)
	if e != nil || len(page.Items) != 5 || page.UnreadCount != "5" {
		t.Fatal(page, e)
	}
	counts := map[string]int{}
	for _, v := range page.Items {
		counts[v.EventType]++
		if v.Payload.Points == nil || *v.Payload.Points != "1" {
			t.Fatal(v)
		}
	}
	if counts["bet.order.placed"] != 2 || counts["bet.order.cancelled"] != 1 || counts["bet.order.abnormal"] != 1 || counts["bet.order.judged_cancelled"] != 1 {
		t.Fatal(counts)
	}
	if walletBySource(t, f) != before {
		t.Fatal("consumer posted money")
	}
	if _, e = s.Process(ctx, 20); e != nil {
		t.Fatal(e)
	}
	again, e := s.List(ctx, f.brand, f.member, 20, 0)
	if e != nil || len(again.Items) != 5 {
		t.Fatal(again, e)
	}
}
