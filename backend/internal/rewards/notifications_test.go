package rewards

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/notification"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

func TestRewardNotificationsBindHistoricActionsAndNeverMovePoints(t *testing.T) {
	f := newRewardFixture(t)
	ctx := context.Background()
	order := rewardGrant(t, f, 40)
	tx := rewardTx(t, f.db)
	meta := rewardMeta(f.admin)
	allocation := []points.Allocation{{Source: "gift", State: "available", Points: 40}}
	delta, err := points.AllocationDelta(allocation, "available", "manual_frozen")
	if err != nil {
		t.Fatal(err)
	}
	hold, err := f.points.Post(ctx, tx, points.Change{BrandID: f.brand, MemberID: f.member, EntryType: "freeze", ReferenceType: "manual", OperationKey: "reward-notification-hold", Reason: "hold only original reward", ActorType: "admin", ActorID: f.admin, RequestID: meta.RequestID, Delta: delta, Allocation: allocation})
	if err != nil {
		t.Fatal(err)
	}
	rewardCommit(t, tx)
	pending := order
	for _, operation := range []string{"revoke", "retry"} {
		tx = rewardTx(t, f.db)
		input := ActionInput{Version: pending.Version, Reason: "Explicit pending notification attempt"}
		if operation == "revoke" {
			pending, err = f.service.RevokeTx(ctx, tx, f.brand, order.ID, f.actor, input, rewardMeta(f.admin))
		} else {
			pending, err = f.service.RetryRevocationTx(ctx, tx, f.brand, order.ID, f.actor, input, rewardMeta(f.admin))
		}
		if err != nil || pending.State != "revocation_pending" {
			t.Fatalf("pending=%+v err=%v", pending, err)
		}
		rewardCommit(t, tx)
	}
	tx = rewardTx(t, f.db)
	if _, err = f.points.Reverse(ctx, tx, f.brand, f.member, hold.ID, "unfreeze", "release original hold", rewardMeta(f.admin)); err != nil {
		t.Fatal(err)
	}
	rewardCommit(t, tx)
	var events int
	if err = f.db.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE event_type LIKE 'reward.order.%'`).Scan(&events); err != nil || events != 3 {
		t.Fatalf("events=%d err=%v", events, err)
	}
	tx = rewardTx(t, f.db)
	revoked, err := f.service.RetryRevocationTx(ctx, tx, f.brand, order.ID, f.actor, ActionInput{Version: pending.Version, Reason: "Explicitly revoke after original hold released"}, rewardMeta(f.admin))
	if err != nil || revoked.State != "revoked" || revoked.Version != 4 {
		t.Fatalf("revoked=%+v err=%v", revoked, err)
	}
	rewardCommit(t, tx)
	before := rewardBalance(t, f)
	service := notification.Service{DB: f.db}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := service.Process(ctx, 20); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	page, err := service.List(ctx, f.brand, f.member, 20, 0)
	if err != nil || len(page.Items) != 4 {
		t.Fatalf("inbox=%+v err=%v", page, err)
	}
	kinds := map[string]int{}
	for _, item := range page.Items {
		kinds[item.EventType]++
		if item.Content == nil || item.TemplateKey != item.EventType || item.TemplateVersion != 1 || item.Payload.ResourceID != order.ID || item.Payload.Points == nil || *item.Payload.Points != "40" {
			t.Fatalf("incorrect reward snapshot: %+v", item)
		}
		raw, e := json.Marshal(item.Payload)
		if e != nil {
			t.Fatal(e)
		}
		var fields map[string]any
		if json.Unmarshal(raw, &fields) != nil || len(fields) != 2 {
			t.Fatalf("private witness in public payload: %s", raw)
		}
	}
	if !reflect.DeepEqual(kinds, map[string]int{"reward.order.granted": 1, "reward.order.revocation_pending": 2, "reward.order.revoked": 1}) {
		t.Fatal(kinds)
	}
	if _, err = service.Process(ctx, 20); err != nil {
		t.Fatal(err)
	}
	again, err := service.List(ctx, f.brand, f.member, 20, 0)
	if err != nil || !reflect.DeepEqual(page, again) {
		t.Fatalf("duplicate delivery changed inbox: %v", err)
	}
	if rewardBalance(t, f) != before {
		t.Fatal("notification delivery moved points")
	}
	foreign, err := service.List(ctx, rewardTestOtherBrand, f.member, 20, 0)
	if err != nil || len(foreign.Items) != 0 {
		t.Fatalf("foreign inbox=%+v err=%v", foreign, err)
	}
	var failed int
	if err = f.db.QueryRow(ctx, `SELECT count(*) FROM notification_deliveries WHERE status<>'sent'`).Scan(&failed); err != nil || failed != 0 {
		t.Fatalf("delayed historic event failed=%d err=%v", failed, err)
	}
}

func TestRewardNotificationFailureRollsBackWholeGrant(t *testing.T) {
	f := newRewardFixture(t)
	ctx := context.Background()
	before := rewardBalance(t, f)
	if _, err := f.db.Exec(ctx, `CREATE FUNCTION fail_reward_notification_test() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.event_type LIKE 'reward.order.%' THEN RAISE EXCEPTION 'owned injected outbox failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER failing_reward_notification_test BEFORE INSERT ON outbox_events FOR EACH ROW EXECUTE FUNCTION fail_reward_notification_test()`); err != nil {
		t.Fatal(err)
	}
	tx := rewardTx(t, f.db)
	if _, err := f.service.GrantTx(ctx, tx, f.brand, f.actor, GrantInput{MemberID: f.member, Points: 25, Reason: "Atomic reward notification failure"}, rewardMeta(f.admin)); err == nil {
		t.Fatal("outbox failure accepted")
	}
	_ = tx.Rollback(ctx)
	if rewardBalance(t, f) != before {
		t.Fatal("failed reward notification changed wallet")
	}
	for _, table := range []string{"reward_orders", "reward_order_actions", "point_ledger_entries", "audit_logs", "outbox_events", "notification_deliveries"} {
		var count int
		if err := f.db.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s count=%d err=%v", table, count, err)
		}
	}
}

func TestRewardNotificationWitnessAndOutboxHistoryCannotBeForged(t *testing.T) {
	f := newRewardFixture(t)
	ctx := context.Background()
	order := rewardGrant(t, f, 25)
	var id, aggregate string
	var raw []byte
	if err := f.db.QueryRow(ctx, `SELECT id::text,aggregate_id::text,payload FROM outbox_events WHERE event_type='reward.order.granted'`).Scan(&id, &aggregate, &raw); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	for _, change := range []struct {
		field string
		value any
	}{{"points", "24"}, {"resource_id", f.member}, {"action_id", order.ID}, {"audit_log_id", order.ID}, {"version", 2}, {"member_id", order.ID}, {"points", nil}, {"private_reason", "leaked"}} {
		altered := map[string]any{}
		for k, v := range payload {
			altered[k] = v
		}
		altered[change.field] = change.value
		body, _ := json.Marshal(altered)
		tx := rewardTx(t, f.db)
		var valid bool
		if err := tx.QueryRow(ctx, `SELECT valid_reward_notification_event($1,'reward.order.granted',$2,$3)`, f.brand, aggregate, body).Scan(&valid); err != nil || valid {
			t.Fatalf("forged %s validated=%v err=%v", change.field, valid, err)
		}
		_ = tx.Rollback(ctx)
	}
	for _, query := range []string{`UPDATE outbox_events SET payload=payload||'{"points":"24"}'::jsonb WHERE id=$1`, `DELETE FROM outbox_events WHERE id=$1`, `UPDATE outbox_events SET event_type='member.joined' WHERE id=$1`, `UPDATE outbox_events SET aggregate_id=brand_id WHERE id=$1`} {
		tx := rewardTx(t, f.db)
		if _, err := tx.Exec(ctx, query, id); err == nil {
			t.Fatalf("mutated reward history: %s", query)
		}
		_ = tx.Rollback(ctx)
	}
	if _, err := f.db.Exec(ctx, `UPDATE outbox_events SET published_at=clock_timestamp() WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := (notification.Service{DB: f.db}).Process(ctx, 20); err != nil {
		t.Fatal(err)
	}
	if rewardBalance(t, f)[2][0] != points.Amount(25) {
		t.Fatal("publishing/delivery changed grant balance")
	}
}
