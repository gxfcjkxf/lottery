-- Delivery state belongs to this consumer, not to a future broker publisher.
CREATE TABLE notification_deliveries (
 event_id uuid PRIMARY KEY REFERENCES outbox_events(id),
 brand_id uuid NOT NULL REFERENCES brands(id),
 status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','sent','failed')),
 attempt_count integer NOT NULL DEFAULT 0 CHECK(attempt_count>=0),
 last_error text,
 next_attempt_at timestamptz NOT NULL DEFAULT now(),
 sent_at timestamptz,
 CHECK((status='sent')=(sent_at IS NOT NULL)),
 UNIQUE(brand_id,event_id)
);
CREATE INDEX notification_delivery_pending ON notification_deliveries(next_attempt_at,event_id) WHERE status='pending';
CREATE TABLE notifications (
 id uuid PRIMARY KEY,
 brand_id uuid NOT NULL,
 member_id uuid NOT NULL,
 event_id uuid NOT NULL,
 event_type text NOT NULL CHECK(event_type IN ('member.joined','recharge.confirmed','bet.order.placed','bet.order.cancelled','bet.order.judged_cancelled','bet.order.abnormal')),
 template_key text NOT NULL CHECK(template_key=event_type),
 template_version integer NOT NULL CHECK(template_version=1),
 payload jsonb NOT NULL CHECK(jsonb_typeof(payload)='object'),
 created_at timestamptz NOT NULL DEFAULT now(),
 read_at timestamptz,
 UNIQUE(event_id,member_id),
 FOREIGN KEY(brand_id,member_id) REFERENCES brand_members(brand_id,id),
 FOREIGN KEY(brand_id,event_id) REFERENCES notification_deliveries(brand_id,event_id)
);
CREATE INDEX notifications_member_page ON notifications(brand_id,member_id,created_at DESC,id DESC);
CREATE INDEX notifications_member_unread ON notifications(brand_id,member_id) WHERE read_at IS NULL;
CREATE FUNCTION guard_notification_content() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'notification content is immutable'; END IF;
 IF (to_jsonb(NEW)-'read_at') IS DISTINCT FROM (to_jsonb(OLD)-'read_at') OR
    (OLD.read_at IS NOT NULL AND NEW.read_at IS DISTINCT FROM OLD.read_at) OR
    NEW.read_at IS NULL THEN RAISE EXCEPTION 'only the first read timestamp may be recorded'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER notification_immutable BEFORE UPDATE OR DELETE ON notifications FOR EACH ROW EXECUTE FUNCTION guard_notification_content();
INSERT INTO permissions(key) VALUES('notification.view.brand'),('notification.retry.brand'),('notification.view.platform') ON CONFLICT DO NOTHING;
