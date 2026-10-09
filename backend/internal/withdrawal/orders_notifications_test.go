package withdrawal

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/notification"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

func TestWithdrawalNotificationHistorySurvivesLaterStateAndConcurrentDelivery(t *testing.T) {
	for _, actions := range [][]string{{"reject"}, {"cancel"}, {"approve", "cancel"}, {"approve", "fail"}, {"approve", "mark_paid"}} {
		t.Run(strings.Join(actions, "_"), func(t *testing.T) {
			f := newOrderIntegrationFixture(t, true, true)
			f.fund(t, oneRechargeAllocation(100))
			original := createOrder(t, f, 40, "notify-create", oneRechargeAllocation(40))
			current := original
			states := []string{"reviewing"}
			for _, action := range actions {
				var err error
				current, err = orderAction(t, f, current.ID, action, "notify-"+action, f.admin, ActionInput{Version: current.Version, ClientKey: "notify-" + action, Reason: "private operational reason"}, points.Metadata{ActorType: "admin", ActorID: f.adminID, RequestID: ids.New()})
				if err != nil {
					t.Fatal(err)
				}
				states = append(states, current.State)
			}
			// Original receipt replay after terminal state must not enqueue again.
			replay := createOrder(t, f, 40, "notify-create", oneRechargeAllocation(40))
			if replay.ID != original.ID || replay.State != "reviewing" {
				t.Fatal(replay)
			}
			ctx := context.Background()
			s := notification.Service{DB: f.db}
			var workers sync.WaitGroup
			for i := 0; i < 4; i++ {
				workers.Add(1)
				go func() {
					defer workers.Done()
					if _, err := s.Process(ctx, 20); err != nil {
						t.Error(err)
					}
				}()
			}
			workers.Wait()
			page, err := s.List(ctx, orderTestBrand, f.member, 20, 0)
			if err != nil || len(page.Items) != len(states) {
				t.Fatalf("items=%+v err=%v", page.Items, err)
			}
			found := map[string]bool{}
			for _, n := range page.Items {
				found[n.EventType] = true
				if n.MemberID != f.member || n.Payload.ResourceID != original.ID || n.Payload.Points == nil || *n.Payload.Points != "40" || n.Content.En.Title == "" {
					t.Fatal(n)
				}
				raw, _ := json.Marshal(n)
				if strings.Contains(string(raw), "private operational reason") || strings.Contains(string(raw), f.adminID) {
					t.Fatalf("private data leaked: %s", raw)
				}
				if !strings.Contains(n.Content.En.Body, "not proof of an external transfer") {
					t.Fatal(n.Content)
				}
			}
			for _, state := range states {
				if !found["withdrawal.order."+state] {
					t.Fatalf("missing history %s: %+v", state, found)
				}
			}
			var events, delivered, consumed int
			if err := f.db.QueryRow(ctx, `SELECT (SELECT count(*) FROM outbox_events WHERE aggregate_id=$1),(SELECT count(*) FROM notification_deliveries WHERE status='sent'),(SELECT count(*) FROM consumed_events WHERE consumer=$2)`, original.ID, notification.Consumer).Scan(&events, &delivered, &consumed); err != nil || events != len(states) || delivered != len(states) || consumed != len(states) {
				t.Fatal(events, delivered, consumed, err)
			}
			if _, err = s.Process(ctx, 20); err != nil {
				t.Fatal(err)
			}
			other, err := s.List(ctx, orderOtherBrand, f.member, 20, 0)
			if err != nil || len(other.Items) != 0 {
				t.Fatal(other, err)
			}
		})
	}
}

func TestWithdrawalOutboxFailureRollsBackReservationAndStateMutation(t *testing.T) {
	for _, during := range []string{"reviewing", "paid", "failed"} {
		t.Run(during, func(t *testing.T) {
			f := newOrderIntegrationFixture(t, true, true)
			f.fund(t, oneRechargeAllocation(100))
			ctx := context.Background()
			var current Order
			if during != "reviewing" {
				current = createOrder(t, f, 40, "rollback-create", oneRechargeAllocation(40))
				var err error
				current, err = orderAction(t, f, current.ID, "approve", "rollback-approve", f.admin, ActionInput{Version: 1, ClientKey: "rollback-approve", Reason: "review"}, points.Metadata{ActorType: "admin", ActorID: f.adminID, RequestID: ids.New()})
				if err != nil {
					t.Fatal(err)
				}
			}
			if _, err := f.db.Exec(ctx, fmt.Sprintf(`CREATE FUNCTION fail_withdrawal_event() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.event_type='withdrawal.order.%s' THEN RAISE EXCEPTION 'test outbox failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_withdrawal_event BEFORE INSERT ON outbox_events FOR EACH ROW EXECUTE FUNCTION fail_withdrawal_event()`, during)); err != nil {
				t.Fatal(err)
			}
			var before, after string
			fingerprint := `SELECT jsonb_build_object('buckets',(SELECT jsonb_agg(to_jsonb(b) ORDER BY source,state) FROM point_buckets b),'ledger',(SELECT count(*) FROM point_ledger_entries),'orders',(SELECT jsonb_agg(to_jsonb(o) ORDER BY id) FROM withdrawal_orders o),'transitions',(SELECT count(*) FROM withdrawal_order_transitions),'events',(SELECT count(*) FROM outbox_events),'audit',(SELECT count(*) FROM audit_logs),'receipts',(SELECT count(*) FROM withdrawal_operation_receipts),'cycles',(SELECT count(*) FROM withdrawal_turnover_cycles))::text`
			if err := f.db.QueryRow(ctx, fingerprint).Scan(&before); err != nil {
				t.Fatal(err)
			}
			tx, err := f.db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if during == "reviewing" {
				_, err = f.service.Create(ctx, tx, orderTestBrand, f.member, OrderInput{Points: 40, SourceAllocation: oneRechargeAllocation(40), ClientKey: "rollback-failed-create"}, f.actor)
			} else {
				action := "mark_paid"
				if during == "failed" {
					action = "fail"
				}
				_, err = f.service.Advance(ctx, tx, orderTestBrand, current.ID, action, f.admin, ActionInput{Version: current.Version, ClientKey: "rollback-failed-action", Reason: "test"}, points.Metadata{ActorType: "admin", ActorID: f.adminID, RequestID: ids.New()})
			}
			if err == nil {
				_ = tx.Rollback(ctx)
				t.Fatal("outbox failure did not reject business mutation")
			}
			_ = tx.Rollback(ctx)
			if err := f.db.QueryRow(ctx, fingerprint).Scan(&after); err != nil || before != after {
				t.Fatal("partial financial mutation", err)
			}
		})
	}
}

func TestWithdrawalOutboxRejectsInventedDuplicatedAndRewrittenFacts(t *testing.T) {
	f := newOrderIntegrationFixture(t, true, true)
	f.fund(t, oneRechargeAllocation(100))
	o := createOrder(t, f, 40, "protected-outbox", oneRechargeAllocation(40))
	ctx := context.Background()
	var eventID string
	if err := f.db.QueryRow(ctx, `SELECT id::text FROM outbox_events WHERE aggregate_id=$1 AND event_type='withdrawal.order.reviewing'`, o.ID).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	for _, change := range []string{
		`UPDATE outbox_events SET payload=payload||'{"points":"41"}' WHERE id=$1`,
		`UPDATE outbox_events SET event_type='withdrawal.order.paid' WHERE id=$1`,
		`UPDATE outbox_events SET created_at=created_at+interval '1 second' WHERE id=$1`,
		`DELETE FROM outbox_events WHERE id=$1`,
		`INSERT INTO outbox_events(id,brand_id,event_type,aggregate_id,payload) SELECT gen_random_uuid(),brand_id,event_type,aggregate_id,payload FROM outbox_events WHERE id=$1`,
		`INSERT INTO outbox_events(id,brand_id,event_type,aggregate_id,payload) SELECT gen_random_uuid(),brand_id,'withdrawal.order.paid',aggregate_id,payload||'{"status":"paid","version":3}' FROM outbox_events WHERE id=$1`,
		`INSERT INTO outbox_events(id,brand_id,event_type,aggregate_id,payload) SELECT gen_random_uuid(),brand_id,event_type,aggregate_id,payload||'{"reason":"private"}' FROM outbox_events WHERE id=$1`,
	} {
		if _, err := f.db.Exec(ctx, change, eventID); err == nil {
			t.Fatalf("forged fact accepted: %s", change)
		}
	}
	// Publication acknowledgement can advance without rewriting the event fact.
	if _, err := f.db.Exec(ctx, `UPDATE outbox_events SET published_at=clock_timestamp() WHERE id=$1`, eventID); err != nil {
		t.Fatal(err)
	}
	s := notification.Service{DB: f.db}
	if _, err := s.Process(ctx, 20); err != nil {
		t.Fatal(err)
	}
	page, err := s.List(ctx, orderTestBrand, f.member, 20, 0)
	if err != nil || len(page.Items) != 1 {
		t.Fatal(page, err)
	}
}
