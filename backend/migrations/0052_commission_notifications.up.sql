-- Capture and extend in place so SQL plans and triggers that already depend on
-- this function OID, including brand initialization, keep seeing all defaults.
DO $$ DECLARE defaults jsonb; BEGIN
 defaults:=notification_template_defaults() || '{
  "commission.paid":{"en":{"title":"Commission credit recorded","body":"Historical record: {points} points were credited to your commission wallet. This records a past credit, not new income or an external payment forecast. Check your current wallet balance; this record is retained."},"zh-CN":{"title":"佣金入账记录","body":"历史记录：{points} 积分曾记入佣金钱包。此记录表示过去的入账，不是新收入预测或外部付款承诺。请查看当前钱包余额；此记录会保留。"}},
  "commission.adjusted":{"en":{"title":"Commission adjustment recorded","body":"Historical record: a commission adjustment of {points} points was recorded. This records a past adjustment, not new income or an external payment forecast. Check your current wallet balance; this record is retained."},"zh-CN":{"title":"佣金调整记录","body":"历史记录：曾调整 {points} 积分。此记录表示过去的调整，不是新收入预测或外部付款承诺。请查看当前钱包余额；此记录会保留。"}}
 }'::jsonb;
 EXECUTE format('CREATE OR REPLACE FUNCTION %I.notification_template_defaults() RETURNS jsonb LANGUAGE sql IMMUTABLE AS %L',
  current_schema(),'SELECT '||quote_literal(defaults::text)||'::jsonb');
END $$;
CREATE OR REPLACE FUNCTION valid_notification_template_content(k text,v jsonb) RETURNS boolean LANGUAGE plpgsql IMMUTABLE AS $$
DECLARE lang text; c jsonb; title text; body text; rest text;
BEGIN
 IF NOT(notification_template_defaults() ? k) OR jsonb_typeof(v)<>'object'
 OR (SELECT count(*) FROM jsonb_object_keys(v))<>2 OR NOT(v ?& ARRAY['en','zh-CN']) THEN RETURN false; END IF;
 FOREACH lang IN ARRAY ARRAY['en','zh-CN'] LOOP
  c:=v->lang;
  IF jsonb_typeof(c)<>'object' OR (SELECT count(*) FROM jsonb_object_keys(c))<>2
  OR NOT(c ?& ARRAY['title','body']) OR jsonb_typeof(c->'title')<>'string' OR jsonb_typeof(c->'body')<>'string' THEN RETURN false; END IF;
  title:=c->>'title'; body:=c->>'body';
  IF title<>btrim(title) OR body<>btrim(body) OR octet_length(title) NOT BETWEEN 1 AND 120 OR octet_length(body) NOT BETWEEN 1 AND 1200
  OR title ~ '[[:cntrl:]<>]' OR replace(replace(body,chr(10),''),chr(9),'') ~ '[[:cntrl:]<>]'
  OR (title||body) ~* '(https?:|javascript:|data:|www[.])' THEN RETURN false; END IF;
  rest:=replace(replace(title||body,'{points}',''),'{resource_id}','');
  IF rest ~ '[{}]' OR (k='member.joined' AND position('{points}' IN title||body)>0)
  OR (k<>'member.joined' AND position('{points}' IN body)=0) THEN RETURN false; END IF;
 END LOOP;
 RETURN true;
EXCEPTION WHEN OTHERS THEN RETURN false;
END $$;

INSERT INTO notification_templates(brand_id,template_key,content)
 SELECT b.id,d.key,d.value FROM brands b CROSS JOIN LATERAL jsonb_each(notification_template_defaults()) d
 WHERE d.key IN('commission.paid','commission.adjusted');
INSERT INTO notification_template_revisions(brand_id,template_key,version,content,reason)
 SELECT brand_id,template_key,version,content,'Initial in-app notification template'
 FROM notification_templates WHERE template_key IN('commission.paid','commission.adjusted');

ALTER TABLE notifications DROP CONSTRAINT notifications_event_type_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_event_type_check CHECK(event_type IN
 ('member.joined','recharge.confirmed','bet.order.placed','bet.order.cancelled','bet.order.judged_cancelled','bet.order.abnormal','bet.order.won','bet.order.prize_reversed',
  'withdrawal.order.reviewing','withdrawal.order.processing','withdrawal.order.paid','withdrawal.order.rejected','withdrawal.order.failed','withdrawal.order.cancelled',
  'commission.paid','commission.adjusted'));
CREATE OR REPLACE FUNCTION enqueue_in_app_event() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.brand_id IS NOT NULL AND NEW.event_type IN
  ('member.joined','recharge.confirmed','bet.order.placed','bet.order.cancelled','bet.order.judged_cancelled','bet.order.abnormal','bet.order.won','bet.order.prize_reversed',
   'withdrawal.order.reviewing','withdrawal.order.processing','withdrawal.order.paid','withdrawal.order.rejected','withdrawal.order.failed','withdrawal.order.cancelled',
   'commission.paid','commission.adjusted') THEN
  INSERT INTO notification_deliveries(event_id,brand_id) VALUES(NEW.id,NEW.brand_id) ON CONFLICT DO NOTHING;
 END IF;
 RETURN NEW;
END $$;

CREATE UNIQUE INDEX commission_notification_event_once ON outbox_events(brand_id,event_type,aggregate_id)
 WHERE event_type IN('commission.paid','commission.adjusted');

CREATE FUNCTION valid_commission_notification_event(b uuid,k text,aggregate uuid,payload jsonb) RETURNS boolean
LANGUAGE plpgsql STABLE AS $$
DECLARE v_member_id uuid; v_resource_id uuid; v_target_id uuid; v_ledger_id uuid; v_amount bigint; target commission_payment_targets; adjustment commission_adjustments; valid boolean;
BEGIN
 IF jsonb_typeof(payload)<>'object' OR (SELECT count(*) FROM jsonb_object_keys(payload))<>5 OR
  NOT(payload ?& ARRAY['member_id','resource_id','points','ledger_entry_id','target_id']) OR
  jsonb_typeof(payload->'member_id')<>'string' OR jsonb_typeof(payload->'resource_id')<>'string' OR
  jsonb_typeof(payload->'points')<>'string' OR jsonb_typeof(payload->'ledger_entry_id')<>'string' OR jsonb_typeof(payload->'target_id')<>'string' THEN RETURN false; END IF;
 IF (payload->>'member_id') !~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$' OR
  (payload->>'resource_id') !~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$' OR
  (payload->>'ledger_entry_id') !~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$' OR
  (payload->>'target_id') !~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$' THEN RETURN false; END IF;
 v_member_id:=(payload->>'member_id')::uuid; v_resource_id:=(payload->>'resource_id')::uuid;
 v_ledger_id:=(payload->>'ledger_entry_id')::uuid; v_target_id:=(payload->>'target_id')::uuid;
 IF (payload->>'points') !~ '^-?[1-9][0-9]*$' OR length(replace(payload->>'points','-',''))>19 THEN RETURN false; END IF;
 v_amount:=(payload->>'points')::bigint;
 IF v_amount=0 OR v_amount::text<>(payload->>'points') THEN RETURN false; END IF;
 IF k='commission.paid' THEN
  IF v_amount<=0 OR aggregate<>v_target_id OR v_resource_id<>v_target_id THEN RETURN false; END IF;
  SELECT * INTO target FROM commission_payment_targets WHERE brand_id=b AND id=v_target_id;
  IF target.id IS NULL OR target.state<>'paid' OR target.points<=0 OR target.points::text<>(payload->>'points') OR target.member_id<>v_member_id OR target.ledger_entry_id<>v_ledger_id THEN RETURN false; END IF;
  SELECT EXISTS(SELECT 1 FROM point_ledger_entries l WHERE l.id=v_ledger_id AND l.brand_id=b AND l.member_id=v_member_id AND
   l.entry_type='commission' AND l.reference_type='commission_payment_target' AND l.reference_id=v_target_id AND
   l.operation_key='commission-payment:'||v_target_id::text AND l.actor_type='system' AND l.actor_id IS NULL AND l.reversal_of IS NULL AND
   l.delta_snapshot=jsonb_set(point_zero_snapshot(),'{commission,available}',to_jsonb(target.points::text)) AND
   l.source_allocation=jsonb_build_array(jsonb_build_object('source','commission','state','available','points',target.points::text))) INTO valid;
  RETURN valid;
 ELSIF k='commission.adjusted' THEN
  IF v_resource_id<>aggregate THEN RETURN false; END IF;
  SELECT * INTO adjustment FROM commission_adjustments WHERE brand_id=b AND id=aggregate;
  IF adjustment.id IS NULL OR adjustment.target_id<>v_target_id OR adjustment.ledger_entry_id<>v_ledger_id OR adjustment.delta_points<>v_amount THEN RETURN false; END IF;
  SELECT EXISTS(SELECT 1 FROM commission_payment_targets t JOIN point_ledger_entries l ON l.id=v_ledger_id AND l.brand_id=t.brand_id
   WHERE t.brand_id=b AND t.id=v_target_id AND t.member_id=v_member_id AND l.member_id=t.member_id AND
   l.entry_type='commission_adjustment' AND l.reference_type='commission_adjustment' AND l.reference_id=aggregate AND
   l.operation_key='commission-adjustment:'||aggregate::text AND l.actor_type='admin' AND l.actor_id=adjustment.created_by AND l.reversal_of IS NULL AND
   l.delta_snapshot=jsonb_set(point_zero_snapshot(),'{commission,available}',to_jsonb(adjustment.delta_points::text)) AND
   l.source_allocation=jsonb_build_array(jsonb_build_object('source','commission','state','available','points',abs(adjustment.delta_points::numeric)::text))) INTO valid;
  RETURN valid;
 END IF;
 RETURN false;
EXCEPTION WHEN OTHERS THEN RETURN false;
END $$;

CREATE FUNCTION guard_commission_notification_outbox() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN
  IF OLD.event_type IN('commission.paid','commission.adjusted') THEN RAISE EXCEPTION 'commission notification event is immutable'; END IF;
  RETURN OLD;
 END IF;
 IF TG_OP='UPDATE' THEN
  IF OLD.event_type IN('commission.paid','commission.adjusted') OR NEW.event_type IN('commission.paid','commission.adjusted') THEN
   IF (to_jsonb(NEW)-'published_at') IS DISTINCT FROM (to_jsonb(OLD)-'published_at') THEN RAISE EXCEPTION 'commission notification event is immutable except published_at'; END IF;
   IF NEW.brand_id IS NULL OR NOT valid_commission_notification_event(NEW.brand_id,NEW.event_type,NEW.aggregate_id,NEW.payload) THEN RAISE EXCEPTION 'commission notification event must bind real immutable ledger evidence'; END IF;
  END IF;
  RETURN NEW;
 END IF;
 IF NEW.event_type IN('commission.paid','commission.adjusted') AND
  (NEW.brand_id IS NULL OR NOT valid_commission_notification_event(NEW.brand_id,NEW.event_type,NEW.aggregate_id,NEW.payload)) THEN
  RAISE EXCEPTION 'commission notification event must bind real immutable ledger evidence';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_commission_notification_outbox BEFORE INSERT OR UPDATE OR DELETE ON outbox_events
 FOR EACH ROW EXECUTE FUNCTION guard_commission_notification_outbox();

CREATE FUNCTION emit_commission_paid_notification() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.state='pending' AND NEW.state='paid' AND NEW.points>0 THEN
  INSERT INTO outbox_events(id,brand_id,event_type,aggregate_id,payload) VALUES(
   gen_random_uuid(),NEW.brand_id,'commission.paid',NEW.id,
   jsonb_build_object('member_id',NEW.member_id::text,'resource_id',NEW.id::text,'points',NEW.points::text,
    'ledger_entry_id',NEW.ledger_entry_id::text,'target_id',NEW.id::text)) ON CONFLICT DO NOTHING;
 END IF;
 RETURN NULL;
END $$;
CREATE TRIGGER commission_paid_notification AFTER UPDATE ON commission_payment_targets
 FOR EACH ROW EXECUTE FUNCTION emit_commission_paid_notification();

CREATE FUNCTION emit_commission_adjusted_notification() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE member uuid;
BEGIN
 IF OLD.ledger_entry_id IS NULL AND NEW.ledger_entry_id IS NOT NULL THEN
  SELECT member_id INTO member FROM commission_payment_targets WHERE brand_id=NEW.brand_id AND id=NEW.target_id;
  INSERT INTO outbox_events(id,brand_id,event_type,aggregate_id,payload) VALUES(
   gen_random_uuid(),NEW.brand_id,'commission.adjusted',NEW.id,
   jsonb_build_object('member_id',member::text,'resource_id',NEW.id::text,'points',NEW.delta_points::text,
    'ledger_entry_id',NEW.ledger_entry_id::text,'target_id',NEW.target_id::text)) ON CONFLICT DO NOTHING;
 END IF;
 RETURN NULL;
END $$;
CREATE TRIGGER commission_adjusted_notification AFTER UPDATE ON commission_adjustments
 FOR EACH ROW EXECUTE FUNCTION emit_commission_adjusted_notification();

CREATE FUNCTION require_commission_notification_commit() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_TABLE_NAME='commission_payment_targets' THEN
  IF NEW.state='paid' AND NEW.points>0 AND NOT EXISTS(SELECT 1 FROM outbox_events e WHERE e.brand_id=NEW.brand_id AND e.event_type='commission.paid' AND e.aggregate_id=NEW.id AND
   valid_commission_notification_event(e.brand_id,e.event_type,e.aggregate_id,e.payload)) THEN RAISE EXCEPTION 'positive paid commission target requires its immutable notification event'; END IF;
 ELSIF OLD.ledger_entry_id IS NULL AND NEW.ledger_entry_id IS NOT NULL THEN
  IF NOT EXISTS(SELECT 1 FROM outbox_events e WHERE e.brand_id=NEW.brand_id AND e.event_type='commission.adjusted' AND e.aggregate_id=NEW.id AND
   valid_commission_notification_event(e.brand_id,e.event_type,e.aggregate_id,e.payload)) THEN RAISE EXCEPTION 'completed commission adjustment requires its immutable notification event'; END IF;
 END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER commission_paid_notification_commit AFTER UPDATE ON commission_payment_targets
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_commission_notification_commit();
CREATE CONSTRAINT TRIGGER commission_adjusted_notification_commit AFTER UPDATE ON commission_adjustments
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_commission_notification_commit();

DO $$ DECLARE app_schema text:=current_schema(); f record;
BEGIN
 FOR f IN SELECT p.oid::regprocedure signature FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace
  WHERE n.nspname=app_schema AND p.proname IN('notification_template_defaults','valid_notification_template_content',
   'enqueue_in_app_event','valid_commission_notification_event','guard_commission_notification_outbox','emit_commission_paid_notification',
   'emit_commission_adjusted_notification','require_commission_notification_commit') LOOP
  EXECUTE format('ALTER FUNCTION %s SET search_path TO pg_catalog, %I, pg_temp',f.signature,app_schema);
 END LOOP;
END $$;
