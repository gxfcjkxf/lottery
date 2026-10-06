-- Enqueue in the business transaction; do not repeatedly scan all old outbox
-- rows at 500 writes/s. Publication remains independent of this consumer.
CREATE FUNCTION enqueue_in_app_event() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.brand_id IS NOT NULL AND NEW.event_type IN
 ('member.joined','recharge.confirmed','bet.order.placed','bet.order.cancelled','bet.order.judged_cancelled','bet.order.abnormal') THEN
  INSERT INTO notification_deliveries(event_id,brand_id) VALUES(NEW.id,NEW.brand_id) ON CONFLICT DO NOTHING;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER enqueue_in_app_event AFTER INSERT ON outbox_events FOR EACH ROW EXECUTE FUNCTION enqueue_in_app_event();
-- One-time recovery for committed facts predating this consumer. No synthetic
-- historical memberships, orders or account mutations are manufactured.
INSERT INTO notification_deliveries(event_id,brand_id)
SELECT id,brand_id FROM outbox_events WHERE brand_id IS NOT NULL AND event_type IN
 ('member.joined','recharge.confirmed','bet.order.placed','bet.order.cancelled','bet.order.judged_cancelled','bet.order.abnormal') ON CONFLICT DO NOTHING;
