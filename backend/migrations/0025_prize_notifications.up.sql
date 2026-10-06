-- Only actual positive wallet postings are inbox facts. Existing messages stay
-- immutable; no historical payout messages or audience are manufactured.
ALTER TABLE notifications DROP CONSTRAINT notifications_event_type_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_event_type_check CHECK(event_type IN
 ('member.joined','recharge.confirmed','bet.order.placed','bet.order.cancelled','bet.order.judged_cancelled','bet.order.abnormal','bet.order.won','bet.order.prize_reversed'));
CREATE OR REPLACE FUNCTION enqueue_in_app_event() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.brand_id IS NOT NULL AND NEW.event_type IN
 ('member.joined','recharge.confirmed','bet.order.placed','bet.order.cancelled','bet.order.judged_cancelled','bet.order.abnormal','bet.order.won','bet.order.prize_reversed') THEN
  INSERT INTO notification_deliveries(event_id,brand_id) VALUES(NEW.id,NEW.brand_id) ON CONFLICT DO NOTHING;
 END IF;
 RETURN NEW;
END $$;
