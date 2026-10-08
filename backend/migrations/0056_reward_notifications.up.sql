-- Extend the existing defaults without changing its OID or prior copy.
-- Old reward orders/actions are not backfilled into the notification queue.
DO $$ DECLARE defaults jsonb; BEGIN
 defaults:=notification_template_defaults() || '{
  "reward.order.granted":{"en":{"title":"Reward grant recorded","body":"Historical record: {points} points were credited to your gift available balance. This records a past grant, not your current wallet balance or an external payment. Check your current wallet; this record is retained."},"zh-CN":{"title":"奖励发放记录","body":"历史记录：{points} 积分曾记入赠送可用积分。此记录表示过去的发放，不代表当前钱包余额或外部付款。请查看当前钱包；此记录会保留。"}},
  "reward.order.revocation_pending":{"en":{"title":"Reward revocation pending recorded","body":"Historical record: a full reward reversal of {points} points was requested and recorded as awaiting operator handling. No points moved in this attempt. It does not retry, unfreeze or deduct automatically; check the current wallet."},"zh-CN":{"title":"奖励撤销待处理记录","body":"历史记录：曾申请全额撤销 {points} 积分奖励，并记录为待运营处理。本次没有积分变动，不会自动重试、解冻或扣除；请查看当前钱包。"}},
  "reward.order.revoked":{"en":{"title":"Reward reversal recorded","body":"Historical record: the full original reward of {points} points was reversed from your gift available balance. This records a past reversal, not your current wallet balance or an external payment. Check your current wallet; this record is retained."},"zh-CN":{"title":"奖励撤销记录","body":"历史记录：原奖励全额 {points} 积分曾从赠送可用积分撤销。此记录表示过去的撤销，不代表当前钱包余额或外部付款。请查看当前钱包；此记录会保留。"}}
 }'::jsonb;
 EXECUTE format('CREATE OR REPLACE FUNCTION %I.notification_template_defaults() RETURNS jsonb LANGUAGE sql IMMUTABLE AS %L',
  current_schema(),'SELECT '||quote_literal(defaults::text)||'::jsonb');
END $$;
INSERT INTO notification_templates(brand_id,template_key,content)
 SELECT b.id,d.key,d.value FROM brands b CROSS JOIN LATERAL jsonb_each(notification_template_defaults()) d
 WHERE d.key IN('reward.order.granted','reward.order.revocation_pending','reward.order.revoked');
INSERT INTO notification_template_revisions(brand_id,template_key,version,content,reason)
 SELECT brand_id,template_key,version,content,'Initial in-app notification template'
 FROM notification_templates WHERE template_key IN('reward.order.granted','reward.order.revocation_pending','reward.order.revoked');

ALTER TABLE notifications DROP CONSTRAINT notifications_event_type_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_event_type_check CHECK(event_type IN
 ('member.joined','recharge.confirmed','bet.order.placed','bet.order.cancelled','bet.order.judged_cancelled','bet.order.abnormal','bet.order.won','bet.order.prize_reversed',
  'withdrawal.order.reviewing','withdrawal.order.processing','withdrawal.order.paid','withdrawal.order.rejected','withdrawal.order.failed','withdrawal.order.cancelled',
  'commission.paid','commission.adjusted','reward.order.granted','reward.order.revocation_pending','reward.order.revoked'));
CREATE OR REPLACE FUNCTION enqueue_in_app_event() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.brand_id IS NOT NULL AND NEW.event_type IN
  ('member.joined','recharge.confirmed','bet.order.placed','bet.order.cancelled','bet.order.judged_cancelled','bet.order.abnormal','bet.order.won','bet.order.prize_reversed',
   'withdrawal.order.reviewing','withdrawal.order.processing','withdrawal.order.paid','withdrawal.order.rejected','withdrawal.order.failed','withdrawal.order.cancelled',
   'commission.paid','commission.adjusted','reward.order.granted','reward.order.revocation_pending','reward.order.revoked') THEN
  INSERT INTO notification_deliveries(event_id,brand_id) VALUES(NEW.id,NEW.brand_id) ON CONFLICT DO NOTHING;
 END IF;
 RETURN NEW;
END $$;
CREATE UNIQUE INDEX reward_notification_event_once ON outbox_events(brand_id,event_type,aggregate_id)
 WHERE event_type IN('reward.order.granted','reward.order.revocation_pending','reward.order.revoked');

CREATE FUNCTION valid_reward_notification_event(b uuid,k text,aggregate uuid,payload jsonb) RETURNS boolean
LANGUAGE plpgsql STABLE AS $$
DECLARE a reward_order_actions; o reward_orders; log audit_logs; l point_ledger_entries; amount bigint; expected jsonb;
BEGIN
 IF b IS NULL OR aggregate IS NULL OR k IS NULL OR k NOT IN('reward.order.granted','reward.order.revocation_pending','reward.order.revoked') OR
  jsonb_typeof(payload) IS DISTINCT FROM 'object' OR (SELECT count(*) FROM jsonb_object_keys(payload))<>6 OR
  NOT(payload ?& ARRAY['member_id','resource_id','points','action_id','version','audit_log_id']) OR
  jsonb_typeof(payload->'member_id') IS DISTINCT FROM 'string' OR jsonb_typeof(payload->'resource_id') IS DISTINCT FROM 'string' OR
  jsonb_typeof(payload->'points') IS DISTINCT FROM 'string' OR jsonb_typeof(payload->'action_id') IS DISTINCT FROM 'string' OR
  jsonb_typeof(payload->'version') IS DISTINCT FROM 'number' OR jsonb_typeof(payload->'audit_log_id') IS DISTINCT FROM 'string' THEN RETURN false; END IF;
 IF (payload->>'points') !~ '^[1-9][0-9]*$' OR length(payload->>'points')>19 OR
  (payload->>'version') !~ '^[1-9][0-9]*$' OR (payload->>'version')::numeric>9007199254740991 THEN RETURN false; END IF;
 amount:=(payload->>'points')::bigint;
 IF amount<=0 OR amount::text<>(payload->>'points') THEN RETURN false; END IF;
 SELECT * INTO a FROM reward_order_actions WHERE brand_id=b AND id=aggregate;
 SELECT * INTO o FROM reward_orders WHERE brand_id=b AND id=a.order_id;
 IF a.id IS NULL OR o.id IS NULL OR (payload->>'action_id') IS DISTINCT FROM a.id::text OR
  (payload->>'member_id') IS DISTINCT FROM o.member_id::text OR (payload->>'resource_id') IS DISTINCT FROM o.id::text OR
  (payload->>'version') IS DISTINCT FROM a.version::text OR (payload->>'audit_log_id') IS DISTINCT FROM a.audit_log_id::text OR
  amount<>o.points OR k<>'reward.order.'||a.state_after THEN RETURN false; END IF;
 SELECT * INTO log FROM audit_logs WHERE id=a.audit_log_id;
 IF log.brand_id IS DISTINCT FROM b OR log.actor_type IS DISTINCT FROM 'admin' OR log.actor_id IS DISTINCT FROM a.actor_id OR
  log.action IS DISTINCT FROM 'reward.order.'||a.operation OR log.resource_type IS DISTINCT FROM 'reward_order' OR log.resource_id IS DISTINCT FROM o.id OR
  log.reason IS DISTINCT FROM a.reason OR log.after_json->>'action_id' IS DISTINCT FROM a.id::text OR
  log.after_json->>'version' IS DISTINCT FROM a.version::text OR log.after_json->>'state' IS DISTINCT FROM a.state_after THEN RETURN false; END IF;
 IF a.state_after='revocation_pending' THEN
  RETURN a.operation IN('revoke','retry') AND a.version>1 AND a.ledger_entry_id IS NULL;
 END IF;
 SELECT * INTO l FROM point_ledger_entries WHERE id=a.ledger_entry_id;
 IF l.id IS NULL OR l.brand_id IS DISTINCT FROM b OR l.member_id IS DISTINCT FROM o.member_id OR
  l.reference_type IS DISTINCT FROM 'reward_order' OR l.reference_id IS DISTINCT FROM o.id OR
  l.actor_type IS DISTINCT FROM 'admin' OR l.actor_id IS DISTINCT FROM a.actor_id OR l.reason IS DISTINCT FROM a.reason OR
  l.source_allocation IS DISTINCT FROM jsonb_build_array(jsonb_build_object('source','gift','state','available','points',o.points::text)) THEN RETURN false; END IF;
 IF a.state_after='granted' THEN
  expected:=jsonb_set(point_zero_snapshot(),'{gift,available}',to_jsonb(o.points::text));
  RETURN a.operation='grant' AND a.version=1 AND a.state_before IS NULL AND a.ledger_entry_id=o.grant_ledger_entry_id AND
   l.entry_type='reward_grant' AND l.operation_key='reward-grant:'||o.id::text AND l.reversal_of IS NULL AND l.delta_snapshot=expected;
 END IF;
 expected:=jsonb_set(point_zero_snapshot(),'{gift,available}',to_jsonb((-o.points)::text));
 RETURN a.state_after='revoked' AND a.operation IN('revoke','retry') AND a.version>1 AND a.ledger_entry_id=o.revoke_ledger_entry_id AND
  l.entry_type='reward_reversal' AND l.operation_key='reward-revoke:'||o.id::text AND l.reversal_of=o.grant_ledger_entry_id AND l.delta_snapshot=expected;
EXCEPTION WHEN OTHERS THEN RETURN false;
END $$;

CREATE FUNCTION guard_reward_notification_outbox() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN
  IF OLD.event_type IN('reward.order.granted','reward.order.revocation_pending','reward.order.revoked') THEN RAISE EXCEPTION 'reward notification event immutable'; END IF;
  RETURN OLD;
 END IF;
 IF TG_OP='UPDATE' THEN
  IF OLD.event_type IN('reward.order.granted','reward.order.revocation_pending','reward.order.revoked') OR NEW.event_type IN('reward.order.granted','reward.order.revocation_pending','reward.order.revoked') THEN
   IF (to_jsonb(NEW)-'published_at') IS DISTINCT FROM (to_jsonb(OLD)-'published_at') THEN RAISE EXCEPTION 'reward notification event immutable except published_at'; END IF;
   IF NOT valid_reward_notification_event(NEW.brand_id,NEW.event_type,NEW.aggregate_id,NEW.payload) THEN RAISE EXCEPTION 'reward notification requires real immutable business evidence'; END IF;
  END IF;
  RETURN NEW;
 END IF;
 IF NEW.event_type IN('reward.order.granted','reward.order.revocation_pending','reward.order.revoked') AND
  (NOT valid_reward_notification_event(NEW.brand_id,NEW.event_type,NEW.aggregate_id,NEW.payload) OR NOT EXISTS(
   SELECT 1 FROM reward_order_actions WHERE brand_id=NEW.brand_id AND id=NEW.aggregate_id AND creation_xid=pg_current_xact_id())) THEN
  RAISE EXCEPTION 'reward notification requires current business action and immutable evidence';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_reward_notification_outbox BEFORE INSERT OR UPDATE OR DELETE ON outbox_events
 FOR EACH ROW EXECUTE FUNCTION guard_reward_notification_outbox();

CREATE FUNCTION emit_reward_notification() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE a reward_order_actions;
BEGIN
 IF (OLD.grant_ledger_entry_id IS NULL AND NEW.grant_ledger_entry_id IS NOT NULL) OR OLD.version<>NEW.version THEN
  SELECT * INTO a FROM reward_order_actions WHERE brand_id=NEW.brand_id AND order_id=NEW.id AND version=NEW.version;
  IF a.id IS NULL THEN RAISE EXCEPTION 'reward notification requires matching action'; END IF;
  INSERT INTO outbox_events(id,brand_id,event_type,aggregate_id,payload) VALUES(gen_random_uuid(),NEW.brand_id,'reward.order.'||a.state_after,a.id,
   jsonb_build_object('member_id',NEW.member_id::text,'resource_id',NEW.id::text,'points',NEW.points::text,
    'action_id',a.id::text,'version',a.version,'audit_log_id',a.audit_log_id::text));
 END IF;
 RETURN NULL;
END $$;
CREATE TRIGGER reward_notification AFTER UPDATE ON reward_orders FOR EACH ROW EXECUTE FUNCTION emit_reward_notification();

CREATE FUNCTION require_reward_notification_commit() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM outbox_events e WHERE e.brand_id=NEW.brand_id AND e.aggregate_id=NEW.id AND e.event_type='reward.order.'||NEW.state_after AND
  valid_reward_notification_event(e.brand_id,e.event_type,e.aggregate_id,e.payload)) THEN
  RAISE EXCEPTION 'new reward action requires matching immutable notification event';
 END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER reward_notification_commit AFTER INSERT ON reward_order_actions
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_reward_notification_commit();
DO $$ DECLARE app_schema text:=current_schema(); f record;
BEGIN
 FOR f IN SELECT p.oid::regprocedure signature FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname=app_schema AND p.proname IN(
  'notification_template_defaults','enqueue_in_app_event','valid_reward_notification_event','guard_reward_notification_outbox','emit_reward_notification','require_reward_notification_commit') LOOP
  EXECUTE format('ALTER FUNCTION %s SET search_path TO pg_catalog, %I, pg_temp',f.signature,app_schema);
 END LOOP;
END $$;
