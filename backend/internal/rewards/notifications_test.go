package rewards

import (
	"context"
	"encoding/json"

	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/notification"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

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
