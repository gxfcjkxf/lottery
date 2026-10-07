-- Only future committed withdrawal transitions produce inbox facts. Preserve
-- all existing template revisions, notifications, orders and wallet postings.
DO $$ DECLARE defaults jsonb; BEGIN
 defaults := notification_template_defaults() || '{
 "withdrawal.order.reviewing":{"en":{"title":"Withdrawal status recorded","body":"Historical withdrawal status: reviewing. Points involved: {points}. This is an internal points record, not proof of an external transfer. Check the withdrawal order for its current state."},"zh-CN":{"title":"提现状态记录","body":"历史提现状态：审核中，涉及 {points} 积分。此为内部积分记录，不证明外部转账；请查询提现订单的最新状态。"}},
 "withdrawal.order.processing":{"en":{"title":"Withdrawal status recorded","body":"Historical withdrawal status: processing. Points involved: {points}. This is an internal points record, not proof of an external transfer. Check the withdrawal order for its current state."},"zh-CN":{"title":"提现状态记录","body":"历史提现状态：提现中，涉及 {points} 积分。此为内部积分记录，不证明外部转账；请查询提现订单的最新状态。"}},
 "withdrawal.order.paid":{"en":{"title":"Withdrawal status recorded","body":"Historical withdrawal status: paid. Points involved: {points}. This is an internal points record, not proof of an external transfer. Check the withdrawal order for its current state."},"zh-CN":{"title":"提现状态记录","body":"历史提现状态：已提现，涉及 {points} 积分。此为内部积分记录，不证明外部转账；请查询提现订单的最新状态。"}},
 "withdrawal.order.rejected":{"en":{"title":"Withdrawal status recorded","body":"Historical withdrawal status: rejected. Points involved: {points}. This is an internal points record, not proof of an external transfer. Check the withdrawal order for its current state."},"zh-CN":{"title":"提现状态记录","body":"历史提现状态：已驳回，涉及 {points} 积分。此为内部积分记录，不证明外部转账；请查询提现订单的最新状态。"}},
 "withdrawal.order.failed":{"en":{"title":"Withdrawal status recorded","body":"Historical withdrawal status: failed. Points involved: {points}. This is an internal points record, not proof of an external transfer. Check the withdrawal order for its current state."},"zh-CN":{"title":"提现状态记录","body":"历史提现状态：失败，涉及 {points} 积分。此为内部积分记录，不证明外部转账；请查询提现订单的最新状态。"}},
 "withdrawal.order.cancelled":{"en":{"title":"Withdrawal status recorded","body":"Historical withdrawal status: cancelled. Points involved: {points}. This is an internal points record, not proof of an external transfer. Check the withdrawal order for its current state."},"zh-CN":{"title":"提现状态记录","body":"历史提现状态：已取消，涉及 {points} 积分。此为内部积分记录，不证明外部转账；请查询提现订单的最新状态。"}}
 }'::jsonb;
 -- Capture the previous eight immutable defaults verbatim rather than rewriting
 -- their migration or accidentally replacing an existing customized revision.
 EXECUTE format('CREATE OR REPLACE FUNCTION %I.notification_template_defaults() RETURNS jsonb LANGUAGE sql IMMUTABLE AS %L',current_schema(),'SELECT '||quote_literal(defaults::text)||'::jsonb');
END $$;
INSERT INTO notification_templates(brand_id,template_key,content)
 SELECT b.id,d.key,d.value FROM brands b CROSS JOIN jsonb_each(notification_template_defaults()) d
 WHERE d.key LIKE 'withdrawal.order.%';
INSERT INTO notification_template_revisions(brand_id,template_key,version,content,reason)
 SELECT brand_id,template_key,version,content,'Initial withdrawal status notification template'
 FROM notification_templates WHERE template_key LIKE 'withdrawal.order.%';

ALTER TABLE notifications DROP CONSTRAINT notifications_event_type_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_event_type_check CHECK(event_type IN
 ('member.joined','recharge.confirmed','bet.order.placed','bet.order.cancelled','bet.order.judged_cancelled','bet.order.abnormal','bet.order.won','bet.order.prize_reversed',
 'withdrawal.order.reviewing','withdrawal.order.processing','withdrawal.order.paid','withdrawal.order.rejected','withdrawal.order.failed','withdrawal.order.cancelled'));
CREATE OR REPLACE FUNCTION enqueue_in_app_event() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.brand_id IS NOT NULL AND NEW.event_type IN
 ('member.joined','recharge.confirmed','bet.order.placed','bet.order.cancelled','bet.order.judged_cancelled','bet.order.abnormal','bet.order.won','bet.order.prize_reversed',
 'withdrawal.order.reviewing','withdrawal.order.processing','withdrawal.order.paid','withdrawal.order.rejected','withdrawal.order.failed','withdrawal.order.cancelled') THEN
  INSERT INTO notification_deliveries(event_id,brand_id) VALUES(NEW.id,NEW.brand_id) ON CONFLICT DO NOTHING;
 END IF;
 RETURN NEW;
END $$;

CREATE UNIQUE INDEX withdrawal_transition_outbox_unique ON outbox_events(brand_id,aggregate_id,(payload->>'version')) WHERE event_type LIKE 'withdrawal.order.%';
CREATE FUNCTION guard_withdrawal_outbox() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE o withdrawal_orders; t withdrawal_order_transitions;
BEGIN
 IF TG_OP='DELETE' THEN
  IF OLD.event_type LIKE 'withdrawal.order.%' THEN RAISE EXCEPTION 'withdrawal event immutable'; END IF;
  RETURN OLD;
 END IF;
 IF TG_OP='UPDATE' THEN
  IF OLD.event_type LIKE 'withdrawal.order.%' OR NEW.event_type LIKE 'withdrawal.order.%' THEN
   IF OLD.event_type NOT LIKE 'withdrawal.order.%' OR (to_jsonb(NEW)-'published_at') IS DISTINCT FROM (to_jsonb(OLD)-'published_at') THEN RAISE EXCEPTION 'withdrawal event immutable'; END IF;
  END IF;
  RETURN NEW;
 END IF;
 IF NEW.event_type NOT LIKE 'withdrawal.order.%' THEN RETURN NEW; END IF;
 SELECT * INTO o FROM withdrawal_orders WHERE brand_id=NEW.brand_id AND id=NEW.aggregate_id;
 IF NOT FOUND THEN RAISE EXCEPTION 'withdrawal event requires its order'; END IF;
 SELECT * INTO t FROM withdrawal_order_transitions WHERE brand_id=o.brand_id AND order_id=o.id AND version=(NEW.payload->>'version')::bigint;
 IF NOT FOUND OR NEW.event_type IS DISTINCT FROM 'withdrawal.order.'||t.to_state
 OR NEW.payload IS DISTINCT FROM jsonb_build_object('member_id',o.member_id::text,'resource_id',o.id::text,'points',o.points::text,'status',t.to_state,'version',t.version,'audit_log_id',t.audit_log_id::text) THEN
  RAISE EXCEPTION 'withdrawal event requires matching immutable transition';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER withdrawal_outbox_guard BEFORE INSERT OR UPDATE OR DELETE ON outbox_events FOR EACH ROW EXECUTE FUNCTION guard_withdrawal_outbox();
CREATE FUNCTION require_withdrawal_transition_outbox() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM outbox_events WHERE brand_id=NEW.brand_id AND aggregate_id=NEW.order_id AND event_type='withdrawal.order.'||NEW.to_state AND payload->>'version'=NEW.version::text AND payload->>'audit_log_id'=NEW.audit_log_id::text) THEN
  RAISE EXCEPTION 'withdrawal transition requires transactional outbox evidence';
 END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER withdrawal_transition_outbox_required AFTER INSERT ON withdrawal_order_transitions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_withdrawal_transition_outbox();
DO $$ DECLARE f text; BEGIN
 FOREACH f IN ARRAY ARRAY['notification_template_defaults()','enqueue_in_app_event()','guard_withdrawal_outbox()','require_withdrawal_transition_outbox()'] LOOP
  EXECUTE format('ALTER FUNCTION %I.%s SET search_path=pg_catalog,%I,pg_temp',current_schema(),f,current_schema());
 END LOOP;
END $$;
