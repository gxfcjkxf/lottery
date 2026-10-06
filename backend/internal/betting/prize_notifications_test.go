package betting

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/notification"
	"github.com/jackc/pgx/v5"
	"testing"
)

func TestPrizeNotificationsUseImmutableFactsAfterCorrection(t *testing.T) {
	f, o, _ := settledCorrectionFixture(t)
	ctx := context.Background()
	createFixtureCorrection(t, f, correctedDigits(1, 2, 2))
	if _, e := f.service.ProcessCorrections(ctx, 20); e != nil {
		t.Fatal(e)
	}
	if _, e := f.service.ProcessSettlements(ctx, 20); e != nil {
		t.Fatal(e)
	}
	before := walletBySource(t, f)
	s := notification.Service{DB: f.db}
	if _, e := s.Process(ctx, 100); e != nil {
		t.Fatal(e)
	}
	page, e := s.List(ctx, f.brand, f.member, 100, 0)
	if e != nil {
		t.Fatal(e)
	}
	counts := map[string]int{}
	for _, n := range page.Items {
		if n.EventType == "bet.order.won" || n.EventType == "bet.order.prize_reversed" {
			counts[n.EventType]++
			if n.Payload.ResourceID != o.ID || n.Payload.Points == nil || *n.Payload.Points != "10" {
				t.Fatal(n)
			}
		}
	}
	if counts["bet.order.won"] != 1 || counts["bet.order.prize_reversed"] != 1 {
		t.Fatal("missing real prize facts", counts)
	}
	if walletBySource(t, f) != before {
		t.Fatal("notification mutated funds")
	}
	if _, e = s.Process(ctx, 100); e != nil {
		t.Fatal(e)
	}
	again, e := s.List(ctx, f.brand, f.member, 100, 0)
	if e != nil || len(again.Items) != len(page.Items) {
		t.Fatal(again, e)
	}
	// A subsequent corrected win is a distinct fact, not a replacement of history.
	createFixtureCorrection(t, f, correctedDigits(1, 2, 1))
	if _, e = f.service.ProcessCorrections(ctx, 20); e != nil {
		t.Fatal(e)
	}
	if _, e = f.service.ProcessSettlements(ctx, 20); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Process(ctx, 100); e != nil {
		t.Fatal(e)
	}
	final, e := s.List(ctx, f.brand, f.member, 100, 0)
	if e != nil {
		t.Fatal(e)
	}
	counts = map[string]int{}
	for _, n := range final.Items {
		counts[n.EventType]++
	}
	if counts["bet.order.won"] != 2 || counts["bet.order.prize_reversed"] != 1 {
		t.Fatal(counts)
	}
	for _, old := range page.Items {
		found := false
		for _, n := range final.Items {
			if n.ID == old.ID {
				found = true
				if n.EventType != old.EventType || n.CreatedAt != old.CreatedAt {
					t.Fatal("rewrote old inbox fact")
				}
			}
		}
		if !found {
			t.Fatal("lost old inbox fact")
		}
	}
}

func TestManualPrizeNoticeWaitsForActualApprovalAndPayout(t *testing.T) {
	f, o := drawnFixtureOrder(t)
	ctx := context.Background()
	a := settlementActor(f)
	mode := "manual"
	p := setSettlementMode(t, f, a, 1, &mode)
	j := startFixtureSettlement(t, f, a, p.Version)
	if _, e := f.service.ProcessSettlements(ctx, 20); e != nil {
		t.Fatal(e)
	}
	s := notification.Service{DB: f.db}
	if _, e := s.Process(ctx, 100); e != nil {
		t.Fatal(e)
	}
	page, e := s.List(ctx, f.brand, f.member, 100, 0)
	if e != nil {
		t.Fatal(e)
	}
	for _, n := range page.Items {
		if n.EventType == "bet.order.won" {
			t.Fatal("announced calculation before payout")
		}
	}
	j, e = f.service.SettlementJob(ctx, f.brand, j.ID)
	if e != nil || j.State != "awaiting_approval" {
		t.Fatal(j, e)
	}
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		_, e := f.service.ActOnSettlement(ctx, tx, f.brand, a, j.ID, "approve", SettlementActionInput{Version: j.Version, Reason: "approve actual prize"}, policyMeta(a.ID))
		return e
	})
	if _, e = f.service.ProcessSettlements(ctx, 20); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Process(ctx, 100); e != nil {
		t.Fatal(e)
	}
	page, e = s.List(ctx, f.brand, f.member, 100, 0)
	if e != nil {
		t.Fatal(e)
	}
	n := 0
	for _, v := range page.Items {
		if v.EventType == "bet.order.won" {
			n++
			if v.Payload.ResourceID != o.ID || *v.Payload.Points != "10" {
				t.Fatal(v)
			}
		}
	}
	if n != 1 {
		t.Fatal("missing paid prize", n)
	}
}

func TestReversalNoticeFailureRollsBackWalletAndOrderReset(t *testing.T) {
	f, o, _ := settledCorrectionFixture(t)
	ctx := context.Background()
	before := walletBySource(t, f)
	_, e := f.db.Exec(ctx, `CREATE FUNCTION reject_reverse_notice() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.event_type='bet.order.prize_reversed' THEN RAISE EXCEPTION 'test reverse notification failure'; END IF; RETURN NEW; END $$;CREATE TRIGGER reject_reverse_notice BEFORE INSERT ON outbox_events FOR EACH ROW EXECUTE FUNCTION reject_reverse_notice()`)
	if e != nil {
		t.Fatal(e)
	}
	c := createFixtureCorrection(t, f, correctedDigits(1, 2, 2))
	if _, e = f.service.ProcessCorrections(ctx, 20); e == nil {
		t.Fatal("expected reversal outbox failure")
	}
	c, e = f.service.Correction(ctx, f.brand, c.ID)
	if e != nil || c.State != "failed" {
		t.Fatal(c, e)
	}
	current, e := f.service.Order(ctx, f.brand, f.member, o.ID)
	if e != nil || current.Status != "won" || current.Version != o.Version {
		t.Fatal(current, e)
	}
	if walletBySource(t, f) != before {
		t.Fatal("reversal partly committed")
	}
	var n int
	if e = f.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE entry_type='prize_reversal'`).Scan(&n); e != nil || n != 0 {
		t.Fatal(n, e)
	}
}

func TestPrizeNotificationPoisonAmountAndBrandAreNotAcknowledged(t *testing.T) {
	f, o, j := settledCorrectionFixture(t)
	ctx := context.Background()
	wrongAmount, wrongMember := ids.New(), ids.New()
	for _, id := range []string{wrongAmount, wrongMember} {
		member := f.member
		amount := "11"
		if id == wrongMember {
			member = ids.New()
			amount = "10"
		}
		_, e := f.db.Exec(ctx, `INSERT INTO outbox_events(id,brand_id,event_type,aggregate_id,payload) VALUES($1,$2,'bet.order.won',$3,jsonb_build_object('member_id',$4::text,'order_id',$3::uuid::text,'points',$5::text,'calculation_id',$6::uuid::text,'payout_entry_id',$7::uuid::text,'job_id',$8::uuid::text,'period_id',$9::uuid::text))`, id, f.brand, o.ID, member, amount, *o.SettlementCalculationID, *o.PayoutEntryID, j.ID, o.PeriodID)
		if e != nil {
			t.Fatal(e)
		}
	}
	s := notification.Service{DB: f.db}
	before := walletBySource(t, f)
	if _, e := s.Process(ctx, 100); e != nil {
		t.Fatal(e)
	}
	for _, id := range []string{wrongAmount, wrongMember} {
		var status, code string
		var ack int
		if e := f.db.QueryRow(ctx, `SELECT status,last_error,(SELECT count(*) FROM consumed_events WHERE event_id=$1) FROM notification_deliveries WHERE event_id=$1`, id).Scan(&status, &code, &ack); e != nil || status != "failed" || code != "INVALID_EVENT" || ack != 0 {
			t.Fatal(status, code, ack, e)
		}
	}
	if walletBySource(t, f) != before {
		t.Fatal("poison consumer changed wallet")
	}
}

func TestPrizeOutboxFailureRollsBackPayout(t *testing.T) {
	f, o := drawnFixtureOrder(t)
	ctx := context.Background()
	a := settlementActor(f)
	mode := "automatic"
	p := setSettlementMode(t, f, a, 1, &mode)
	_, e := f.db.Exec(ctx, `CREATE FUNCTION reject_prize_notice() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.event_type='bet.order.won' THEN RAISE EXCEPTION 'test prize notification failure'; END IF; RETURN NEW; END $$;CREATE TRIGGER reject_prize_notice BEFORE INSERT ON outbox_events FOR EACH ROW EXECUTE FUNCTION reject_prize_notice()`)
	if e != nil {
		t.Fatal(e)
	}
	j := startFixtureSettlement(t, f, a, p.Version)
	before := walletBySource(t, f)
	if _, e = f.service.ProcessSettlements(ctx, 20); e == nil {
		t.Fatal("expected payout/outbox failure")
	}
	j, e = f.service.SettlementJob(ctx, f.brand, j.ID)
	if e != nil || j.State != "failed" {
		t.Fatal(j, e)
	}
	current, e := f.service.Order(ctx, f.brand, f.member, o.ID)
	if e != nil || current.Status != "placed" || current.PayoutEntryID != nil {
		t.Fatal(current, e)
	}
	if walletBySource(t, f) != before {
		t.Fatal("partial payout despite outbox failure")
	}
	var n int
	if e = f.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE entry_type='prize'`).Scan(&n); e != nil || n != 0 {
		t.Fatal(n, e)
	}
}
