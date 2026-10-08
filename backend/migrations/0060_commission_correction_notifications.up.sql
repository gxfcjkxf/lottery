-- Add only future real correction postings. Preserve old defaults, function
-- identity, delivered snapshots and historical 0059 targets without backfill.
DO $$ DECLARE defaults jsonb; BEGIN
 defaults:=notification_template_defaults() || '{
  "commission.corrected":{"en":{"title":"Commission correction recorded","body":"Historical record: a commission correction of {points} points was posted to your commission available balance. Positive points record a past additional credit; negative points record a past recovery. This is not your current balance, new income or an external payment. Check your current wallet; this record is retained."},"zh-CN":{"title":"佣金更正记录","body":"历史记录：佣金可用积分曾发生 {points} 积分更正。正数表示过去的补发，负数表示过去的追回；不代表当前余额、新收入或外部付款。请查看当前钱包；此记录会保留。"}}
 }'::jsonb;
 EXECUTE format('CREATE OR REPLACE FUNCTION %I.notification_template_defaults() RETURNS jsonb LANGUAGE sql IMMUTABLE AS %L',
  current_schema(),'SELECT '||quote_literal(defaults::text)||'::jsonb');
END $$;
INSERT INTO notification_templates(brand_id,template_key,content)
 SELECT b.id,d.key,d.value FROM brands b CROSS JOIN LATERAL jsonb_each(notification_template_defaults()) d
 WHERE d.key='commission.corrected';
INSERT INTO notification_template_revisions(brand_id,template_key,version,content,reason)
 SELECT brand_id,template_key,version,content,'Initial in-app notification template'
 FROM notification_templates WHERE template_key='commission.corrected';

ALTER TABLE notifications DROP CONSTRAINT notifications_event_type_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_event_type_check CHECK(event_type IN
 ('member.joined','recharge.confirmed','bet.order.placed','bet.order.cancelled','bet.order.judged_cancelled','bet.order.abnormal','bet.order.won','bet.order.prize_reversed',
  'withdrawal.order.reviewing','withdrawal.order.processing','withdrawal.order.paid','withdrawal.order.rejected','withdrawal.order.failed','withdrawal.order.cancelled',
  'commission.paid','commission.adjusted','commission.corrected','reward.order.granted','reward.order.revocation_pending','reward.order.revoked'));
CREATE OR REPLACE FUNCTION enqueue_in_app_event() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.brand_id IS NOT NULL AND notification_template_defaults() ? NEW.event_type THEN
  INSERT INTO notification_deliveries(event_id,brand_id) VALUES(NEW.id,NEW.brand_id) ON CONFLICT DO NOTHING;
 END IF;
 RETURN NEW;
END $$;
CREATE UNIQUE INDEX commission_correction_notification_once ON outbox_events(brand_id,event_type,aggregate_id)
 WHERE event_type='commission.corrected';
CREATE INDEX commission_correction_posting_report_idx ON point_ledger_entries(brand_id,created_at,id)
 WHERE entry_type='commission_correction';

-- Historical validity is independent of the current run, task state, wallet
-- balance or current agent policy. Every witness itself is immutable.
CREATE FUNCTION valid_commission_correction_notification_event(b uuid,k text,aggregate uuid,payload jsonb) RETURNS boolean
LANGUAGE plpgsql STABLE AS $$
DECLARE amount bigint; valid boolean;
BEGIN
 IF b IS NULL OR aggregate IS NULL OR k IS DISTINCT FROM 'commission.corrected' OR
  jsonb_typeof(payload) IS DISTINCT FROM 'object' OR (SELECT count(*) FROM jsonb_object_keys(payload))<>5 OR
  NOT(payload ?& ARRAY['member_id','resource_id','points','ledger_entry_id','target_id']) OR
  jsonb_typeof(payload->'member_id') IS DISTINCT FROM 'string' OR jsonb_typeof(payload->'resource_id') IS DISTINCT FROM 'string' OR
  jsonb_typeof(payload->'points') IS DISTINCT FROM 'string' OR jsonb_typeof(payload->'ledger_entry_id') IS DISTINCT FROM 'string' OR
  jsonb_typeof(payload->'target_id') IS DISTINCT FROM 'string' OR
  payload->>'points' !~ '^-?[1-9][0-9]*$' OR length(replace(payload->>'points','-',''))>19 THEN RETURN false; END IF;
 amount:=(payload->>'points')::bigint;
 IF amount=0 OR amount::text IS DISTINCT FROM payload->>'points' OR
  payload->>'resource_id' IS DISTINCT FROM aggregate::text OR payload->>'target_id' IS DISTINCT FROM aggregate::text THEN RETURN false; END IF;
 SELECT EXISTS(
  SELECT 1 FROM commission_correction_execution_targets t
  JOIN commission_correction_executions x ON x.brand_id=t.brand_id AND x.id=t.execution_id AND x.cycle_id=t.cycle_id
  JOIN point_ledger_entries l ON l.brand_id=t.brand_id AND l.id=t.ledger_entry_id
  JOIN audit_logs a ON a.id=t.audit_log_id
  JOIN commission_correction_execution_steps s ON s.brand_id=t.brand_id AND s.execution_id=t.execution_id AND s.target_id=t.id
  WHERE t.brand_id=b AND t.id=aggregate AND t.state='applied' AND t.delta_points=amount AND amount<>0
   AND t.member_id::text=payload->>'member_id' AND t.ledger_entry_id::text=payload->>'ledger_entry_id'
   AND t.applied_at IS NOT NULL AND t.financial_version IS NOT NULL AND x.approval_audit_log_id IS NOT NULL
   AND s.operation='apply' AND s.version=t.execution_version+1 AND s.actor_type='system' AND s.actor_id IS NULL
   AND a.brand_id=t.brand_id AND a.actor_type='system' AND a.actor_id IS NULL
   AND a.action='commission.correction_execution.target' AND a.resource_type='commission_correction_target'
   AND a.resource_id=t.id AND a.request_id=s.request_id
   AND a.before_json=jsonb_build_object('state','pending')
   AND a.after_json=jsonb_build_object('state','applied','execution_id',t.execution_id,'plan_target_id',t.plan_target_id,
    'points_before',t.points_before::text,'points_after',t.points_after::text,'delta_points',t.delta_points::text,
    'ledger_entry_id',t.ledger_entry_id,'financial_version',t.financial_version)
   AND l.member_id=t.member_id AND l.entry_type='commission_correction' AND l.reference_type='commission_correction_target'
   AND l.reference_id=t.id AND l.operation_key='commission-correction:'||t.id::text AND l.request_id=s.request_id
   AND l.actor_type='system' AND l.actor_id IS NULL AND l.reversal_of IS NULL
   AND l.delta_snapshot=jsonb_set(point_zero_snapshot(),'{commission,available}',to_jsonb(t.delta_points::text))
   AND l.source_allocation=jsonb_build_array(jsonb_build_object('source','commission','state','available','points',abs(t.delta_points::numeric)::text))
 ) INTO valid;
 RETURN valid;
EXCEPTION WHEN OTHERS THEN RETURN false;
END $$;

CREATE FUNCTION guard_commission_correction_notification_outbox() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN
  IF OLD.event_type='commission.corrected' THEN RAISE EXCEPTION 'correction notification event immutable'; END IF;
  RETURN OLD;
 END IF;
 IF TG_OP='UPDATE' THEN
  IF OLD.event_type='commission.corrected' OR NEW.event_type='commission.corrected' THEN
   IF (to_jsonb(NEW)-'published_at') IS DISTINCT FROM (to_jsonb(OLD)-'published_at') THEN RAISE EXCEPTION 'correction notification event immutable except published_at'; END IF;
   IF NOT valid_commission_correction_notification_event(NEW.brand_id,NEW.event_type,NEW.aggregate_id,NEW.payload) THEN
    RAISE EXCEPTION 'correction notification requires immutable posting evidence'; END IF;
  END IF;
  RETURN NEW;
 END IF;
 IF NEW.event_type='commission.corrected' AND
  (NOT valid_commission_correction_notification_event(NEW.brand_id,NEW.event_type,NEW.aggregate_id,NEW.payload) OR NOT EXISTS(
   SELECT 1 FROM commission_correction_execution_targets WHERE brand_id=NEW.brand_id AND id=NEW.aggregate_id AND creation_xid=pg_current_xact_id())) THEN
  RAISE EXCEPTION 'correction notification requires current real nonzero posting';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_commission_correction_notification_outbox BEFORE INSERT OR UPDATE OR DELETE ON outbox_events
 FOR EACH ROW EXECUTE FUNCTION guard_commission_correction_notification_outbox();

CREATE FUNCTION emit_commission_correction_notification() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.state='pending' AND NEW.state='applied' AND NEW.delta_points<>0 THEN
  INSERT INTO outbox_events(id,brand_id,event_type,aggregate_id,payload) VALUES(gen_random_uuid(),NEW.brand_id,'commission.corrected',NEW.id,
   jsonb_build_object('member_id',NEW.member_id::text,'resource_id',NEW.id::text,'points',NEW.delta_points::text,
    'ledger_entry_id',NEW.ledger_entry_id::text,'target_id',NEW.id::text));
 END IF;
 RETURN NULL;
END $$;
CREATE TRIGGER commission_correction_notification AFTER UPDATE ON commission_correction_execution_targets
 FOR EACH ROW EXECUTE FUNCTION emit_commission_correction_notification();

CREATE FUNCTION require_commission_correction_notification_commit() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.state='applied' AND NEW.delta_points<>0 AND NOT EXISTS(
  SELECT 1 FROM outbox_events e WHERE e.brand_id=NEW.brand_id AND e.aggregate_id=NEW.id AND e.event_type='commission.corrected'
   AND valid_commission_correction_notification_event(e.brand_id,e.event_type,e.aggregate_id,e.payload)) THEN
  RAISE EXCEPTION 'new correction posting requires matching immutable notification event';
 END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER commission_correction_notification_commit AFTER UPDATE ON commission_correction_execution_targets
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_commission_correction_notification_commit();

DO $$ DECLARE app_schema text:=current_schema(); f record;
BEGIN
 FOR f IN SELECT p.oid::regprocedure signature FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname=app_schema AND p.proname IN(
  'notification_template_defaults','enqueue_in_app_event','valid_commission_correction_notification_event','guard_commission_correction_notification_outbox',
  'emit_commission_correction_notification','require_commission_correction_notification_commit') LOOP
  EXECUTE format('ALTER FUNCTION %s SET search_path TO pg_catalog, %I, pg_temp',f.signature,app_schema);
 END LOOP;
END $$;
