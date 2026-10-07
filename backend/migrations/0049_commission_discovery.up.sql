-- Durable discovery is operational work, not per-period commission accrual.
CREATE FUNCTION commission_calendar_boundary(wall timestamp,tz text) RETURNS timestamptz LANGUAGE plpgsql STABLE AS $$
DECLARE candidates timestamptz[];
BEGIN
 -- Match the Go calendar's exact round-trip resolver. PostgreSQL's default
 -- timezone conversion alone silently selects ambiguous/nonexistent walls.
 SELECT array_agg(DISTINCT candidate) INTO candidates FROM (
  SELECT (wall AT TIME ZONE 'UTC')-((sample AT TIME ZONE tz)-(sample AT TIME ZONE 'UTC')) candidate
  FROM generate_series((wall AT TIME ZONE 'UTC')-interval '36 hours',(wall AT TIME ZONE 'UTC')+interval '36 hours',interval '15 minutes') sample
 ) resolved WHERE candidate AT TIME ZONE tz=wall;
 IF coalesce(cardinality(candidates),0)<>1 THEN RAISE EXCEPTION 'invalid commission calendar boundary'; END IF;
 RETURN candidates[1];
END $$;
CREATE FUNCTION commission_calendar_window(c jsonb,at_time timestamptz)
 RETURNS TABLE(window_from timestamptz,window_to timestamptz) LANGUAGE plpgsql STABLE AS $$
DECLARE tz text; wall_date date; anchor date; candidate_date date; month_start date; month_last date;
 boundary_clock time; candidate timestamptz; day_number integer; delta integer; offset_days integer;
BEGIN
 IF at_time IS NULL OR NOT isfinite(at_time) OR valid_commission_policy(jsonb_build_object('enabled',true,'calendar',c,'payout_mode','manual')) IS DISTINCT FROM true THEN
  RAISE EXCEPTION 'invalid commission calendar'; END IF;
 tz:=c->>'timezone';boundary_clock:=(c->>'boundary_time')::time;wall_date:=(at_time AT TIME ZONE tz)::date;
 IF c->>'cycle'='weekly' THEN
  offset_days:=mod(extract(dow FROM wall_date)::integer-(c->>'weekday')::integer+7,7);
  anchor:=wall_date-offset_days;
  window_from:=commission_calendar_boundary(anchor+boundary_clock,tz);
  IF at_time<window_from THEN anchor:=anchor-7;window_from:=commission_calendar_boundary(anchor+boundary_clock,tz);END IF;
  window_to:=commission_calendar_boundary((anchor+7)+boundary_clock,tz);
 ELSE
  month_start:=date_trunc('month',wall_date)::date;day_number:=(c->>'month_day')::integer;
  FOR delta IN 0..2 LOOP
   anchor:=(month_start+make_interval(months=>-delta))::date;month_last:=(anchor+interval '1 month'-interval '1 day')::date;
   IF day_number>extract(day FROM month_last)::integer AND c->>'short_month'='skip' THEN CONTINUE;END IF;
   candidate_date:=anchor+(least(day_number,extract(day FROM month_last)::integer)-1);
   candidate:=commission_calendar_boundary(candidate_date+boundary_clock,tz);
   IF candidate<=at_time THEN window_from:=candidate;EXIT;END IF;
  END LOOP;
  FOR delta IN 0..2 LOOP
   anchor:=(month_start+make_interval(months=>delta))::date;month_last:=(anchor+interval '1 month'-interval '1 day')::date;
   IF day_number>extract(day FROM month_last)::integer AND c->>'short_month'='skip' THEN CONTINUE;END IF;
   candidate_date:=anchor+(least(day_number,extract(day FROM month_last)::integer)-1);
   candidate:=commission_calendar_boundary(candidate_date+boundary_clock,tz);
   IF candidate>window_from THEN window_to:=candidate;EXIT;END IF;
  END LOOP;
 END IF;
 IF window_from IS NULL OR window_to IS NULL THEN RAISE EXCEPTION 'invalid commission calendar window';END IF;
 RETURN NEXT;
END $$;

ALTER TABLE commission_cycles ALTER COLUMN created_by DROP NOT NULL;
ALTER TABLE commission_cycles ADD COLUMN creation_actor_type text NOT NULL DEFAULT 'admin'
 CHECK(creation_actor_type IN('admin','system'));
ALTER TABLE commission_cycles ADD CHECK((creation_actor_type='admin')=(created_by IS NOT NULL));

CREATE TABLE commission_discovery (
 id uuid PRIMARY KEY,brand_id uuid NOT NULL REFERENCES brands(id),
 state text NOT NULL DEFAULT 'pending' CHECK(state IN('pending','registered','failed')),
 version bigint NOT NULL DEFAULT 1 CHECK(version BETWEEN 1 AND 9007199254740991),
 cycle_id uuid,window_from timestamptz,window_to timestamptz,
 next_check_at timestamptz NOT NULL DEFAULT clock_timestamp(),last_error_code text,
 last_audit_log_id uuid REFERENCES audit_logs(id),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(brand_id,id),FOREIGN KEY(brand_id,id) REFERENCES bet_orders(brand_id,id),
 FOREIGN KEY(brand_id,cycle_id) REFERENCES commission_cycles(brand_id,id),
 CHECK((window_from IS NULL)=(window_to IS NULL)),CHECK(window_from IS NULL OR window_from<window_to),
 CHECK((state='registered')=(cycle_id IS NOT NULL)),CHECK((state='failed')=(last_error_code IS NOT NULL)),
 CHECK(state<>'registered' OR window_from IS NOT NULL)
);
CREATE INDEX commission_discovery_due ON commission_discovery(next_check_at,id) WHERE state='pending';
CREATE INDEX commission_discovery_brand ON commission_discovery(brand_id,created_at DESC,id DESC);

CREATE FUNCTION guard_commission_discovery() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE o bet_orders; a audit_logs; op text;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'commission discovery history immutable'; END IF;
 SELECT * INTO o FROM bet_orders WHERE brand_id=NEW.brand_id AND id=NEW.id;
 IF NOT FOUND OR o.commission_rule_snapshot IS NULL OR
  o.commission_rule_snapshot->'financial_policy'->'config'->'enabled' IS DISTINCT FROM 'true'::jsonb THEN
  RAISE EXCEPTION 'commission discovery requires enabled saved policy'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.state<>'pending' OR NEW.version<>1 OR NEW.cycle_id IS NOT NULL OR NEW.window_from IS NOT NULL OR NEW.last_error_code IS NOT NULL OR NEW.last_audit_log_id IS NOT NULL THEN
   RAISE EXCEPTION 'invalid initial commission discovery'; END IF;
  NEW.created_at:=clock_timestamp();NEW.updated_at:=NEW.created_at;RETURN NEW;
 END IF;
 IF (NEW.id,NEW.brand_id,NEW.created_at) IS DISTINCT FROM (OLD.id,OLD.brand_id,OLD.created_at) OR
  (OLD.window_from IS NOT NULL AND (NEW.window_from,NEW.window_to) IS DISTINCT FROM (OLD.window_from,OLD.window_to)) THEN
  RAISE EXCEPTION 'commission discovery identity immutable'; END IF;
 IF NEW.window_from IS NOT NULL AND (o.placed_at<NEW.window_from OR o.placed_at>=NEW.window_to) THEN
  RAISE EXCEPTION 'commission discovery window excludes original bet'; END IF;
 IF NEW.window_from IS NOT NULL AND OLD.window_from IS NULL AND NOT EXISTS(
  SELECT 1 FROM commission_calendar_window(o.commission_rule_snapshot->'financial_policy'->'config'->'calendar',o.placed_at) w
  WHERE w.window_from=NEW.window_from AND w.window_to=NEW.window_to) THEN RAISE EXCEPTION 'commission discovery calendar window mismatch'; END IF;
 IF NEW.state=OLD.state AND NEW.version=OLD.version AND NEW.cycle_id IS NOT DISTINCT FROM OLD.cycle_id AND
  NEW.last_error_code IS NOT DISTINCT FROM OLD.last_error_code AND NEW.last_audit_log_id IS NOT DISTINCT FROM OLD.last_audit_log_id THEN
  IF OLD.state<>'pending' THEN RAISE EXCEPTION 'only pending discovery can reschedule'; END IF;
 ELSE
  IF NEW.version<>OLD.version+1 OR NOT ((OLD.state='pending' AND NEW.state IN('registered','failed')) OR (OLD.state='failed' AND NEW.state='pending')) THEN
   RAISE EXCEPTION 'invalid commission discovery transition'; END IF;
  op:=CASE NEW.state WHEN 'registered' THEN 'register' WHEN 'failed' THEN 'failure' ELSE 'retry' END;
  SELECT * INTO a FROM audit_logs WHERE id=NEW.last_audit_log_id;
  IF NOT FOUND OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.resource_type<>'commission_discovery' OR a.resource_id IS DISTINCT FROM NEW.id OR
   a.action IS DISTINCT FROM 'commission.discovery.'||op OR a.before_json->>'version' IS DISTINCT FROM OLD.version::text OR
   a.before_json->>'state' IS DISTINCT FROM OLD.state OR a.after_json->>'version' IS DISTINCT FROM NEW.version::text OR
   a.after_json->>'state' IS DISTINCT FROM NEW.state OR a.after_json->>'cycle_id' IS DISTINCT FROM NEW.cycle_id::text OR
   a.after_json->>'last_error_code' IS DISTINCT FROM NEW.last_error_code OR
   ((op='retry') AND (a.actor_type<>'admin' OR a.actor_id IS NULL)) OR
   ((op<>'retry') AND (a.actor_type<>'system' OR a.actor_id IS NOT NULL)) THEN
   RAISE EXCEPTION 'commission discovery transition needs matching audit'; END IF;
 END IF;
 IF NEW.state='registered' AND NOT EXISTS(SELECT 1 FROM commission_cycles c WHERE c.brand_id=NEW.brand_id AND c.id=NEW.cycle_id AND
  c.window_from=NEW.window_from AND c.window_to=NEW.window_to AND c.window_to<=clock_timestamp()) THEN
  RAISE EXCEPTION 'discovery needs actual closed cycle'; END IF;
 NEW.updated_at:=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER guarded_commission_discovery BEFORE INSERT OR UPDATE OR DELETE ON commission_discovery FOR EACH ROW EXECUTE FUNCTION guard_commission_discovery();
CREATE FUNCTION enqueue_commission_discovery() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.commission_rule_snapshot->'financial_policy'->'config'->'enabled'='true'::jsonb THEN
  INSERT INTO commission_discovery(id,brand_id) VALUES(NEW.id,NEW.brand_id);
 END IF;
 RETURN NULL;
END $$;
CREATE TRIGGER enqueue_commission_discovery AFTER INSERT ON bet_orders FOR EACH ROW EXECUTE FUNCTION enqueue_commission_discovery();
-- Existing real 0047 snapshots are queued, without manufacturing old NULL rules.
INSERT INTO commission_discovery(id,brand_id)
 SELECT id,brand_id FROM bet_orders WHERE commission_rule_snapshot->'financial_policy'->'config'->'enabled'='true'::jsonb;

CREATE OR REPLACE FUNCTION guard_commission_cycle() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE a audit_logs; o bet_orders; step commission_cycle_steps; added bigint;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'commission cycle history immutable'; END IF;
 IF TG_OP='INSERT' THEN
  PERFORM 1 FROM brand_commission_policies WHERE brand_id=NEW.brand_id FOR UPDATE;
  SELECT * INTO o FROM bet_orders WHERE brand_id=NEW.brand_id AND id=NEW.anchor_order_id;
  IF NOT FOUND OR o.commission_rule_snapshot IS NULL OR o.commission_rule_snapshot->'financial_policy'->'config'->'enabled' IS DISTINCT FROM 'true'::jsonb OR
   o.commission_rule_snapshot->'financial_policy'->'config'->'calendar' IS DISTINCT FROM NEW.calendar OR
   o.placed_at<NEW.window_from OR o.placed_at>=NEW.window_to OR NEW.window_to>clock_timestamp() OR
   NEW.state<>'enumerating' OR NEW.version<>1 OR NEW.evidence_epoch<>0 OR NEW.target_count<>0 OR NEW.scan_complete OR NEW.current_run_id IS NOT NULL OR
   NEW.manifest_cursor_at IS NOT NULL OR NEW.manifest_cursor_id IS NOT NULL THEN RAISE EXCEPTION 'invalid closed commission cycle admission'; END IF;
  IF NOT EXISTS(SELECT 1 FROM commission_calendar_window(NEW.calendar,o.placed_at) w WHERE w.window_from=NEW.window_from AND w.window_to=NEW.window_to) THEN
   RAISE EXCEPTION 'commission cycle calendar window mismatch'; END IF;
  IF NEW.creation_actor_type='system' AND NOT EXISTS(SELECT 1 FROM commission_discovery d WHERE d.brand_id=NEW.brand_id AND d.id=NEW.anchor_order_id AND
   d.state='pending' AND d.window_from=NEW.window_from AND d.window_to=NEW.window_to) THEN RAISE EXCEPTION 'system cycle requires discovered saved window'; END IF;
  SELECT * INTO a FROM audit_logs WHERE id=NEW.creation_audit_log_id;
  IF NOT FOUND OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.actor_type IS DISTINCT FROM NEW.creation_actor_type OR a.actor_id IS DISTINCT FROM NEW.created_by OR
   a.resource_type<>'commission_cycle' OR a.resource_id IS DISTINCT FROM NEW.id OR a.action<>'commission.cycle.create' OR a.reason<>NEW.reason OR
   a.after_json->>'version' IS DISTINCT FROM '1' OR a.after_json->>'state' IS DISTINCT FROM 'enumerating' OR a.after_json->>'anchor_order_id' IS DISTINCT FROM NEW.anchor_order_id::text OR
   (a.after_json->>'window_from')::timestamptz IS DISTINCT FROM NEW.window_from OR (a.after_json->>'window_to')::timestamptz IS DISTINCT FROM NEW.window_to OR
   a.after_json->'calendar' IS DISTINCT FROM NEW.calendar THEN RAISE EXCEPTION 'commission cycle needs matching creation audit'; END IF;
 ELSE
  IF (to_jsonb(NEW)-ARRAY['state','version','target_count','manifest_cursor_at','manifest_cursor_id','scan_complete','current_run_id','updated_at','next_work_at','last_error_code','evidence_epoch']) IS DISTINCT FROM
   (to_jsonb(OLD)-ARRAY['state','version','target_count','manifest_cursor_at','manifest_cursor_id','scan_complete','current_run_id','updated_at','next_work_at','last_error_code','evidence_epoch']) THEN RAISE EXCEPTION 'commission cycle identity immutable'; END IF;
  IF (to_jsonb(NEW)-ARRAY['evidence_epoch']) IS NOT DISTINCT FROM (to_jsonb(OLD)-ARRAY['evidence_epoch']) THEN
   IF NEW.evidence_epoch<>OLD.evidence_epoch+1 THEN RAISE EXCEPTION 'commission evidence epoch cannot rewind'; END IF;
   RETURN NEW;
  END IF;
  IF (to_jsonb(NEW)-ARRAY['next_work_at']) IS NOT DISTINCT FROM (to_jsonb(OLD)-ARRAY['next_work_at']) THEN RETURN NEW; END IF;
  IF NEW.version<>OLD.version+1 THEN RAISE EXCEPTION 'commission cycle version invalid'; END IF;
  SELECT * INTO step FROM commission_cycle_steps WHERE cycle_id=NEW.id AND version=NEW.version;
  IF NOT FOUND OR step.from_state<>OLD.state OR step.to_state<>NEW.state THEN RAISE EXCEPTION 'commission cycle update needs step'; END IF;
  IF NOT ((OLD.state='enumerating' AND NEW.state IN('enumerating','waiting','failed')) OR
   (OLD.state='waiting' AND NEW.state IN('calculating','failed')) OR
   (OLD.state='calculating' AND NEW.state IN('calculating','summarizing','waiting','failed')) OR
   (OLD.state='summarizing' AND NEW.state IN('summarizing','ready','waiting','failed')) OR
   (OLD.state='ready' AND NEW.state='waiting') OR (OLD.state='failed' AND NEW.state IN('enumerating','waiting'))) THEN RAISE EXCEPTION 'invalid commission cycle transition'; END IF;
  IF NEW.evidence_epoch<>OLD.evidence_epoch THEN RAISE EXCEPTION 'commission step cannot rewrite its evidence fence'; END IF;
  IF NEW.state='ready' AND NOT EXISTS(SELECT 1 FROM commission_runs r WHERE r.id=NEW.current_run_id AND r.cycle_id=NEW.id AND r.state='ready' AND r.evidence_epoch=NEW.evidence_epoch) THEN RAISE EXCEPTION 'commission cycle requires complete current evidence'; END IF;
  IF OLD.state='enumerating' AND NEW.state IN('enumerating','waiting') THEN
   SELECT count(*) INTO added FROM commission_cycle_targets WHERE cycle_id=NEW.id AND creation_xid=pg_current_xact_id();
   IF NEW.target_count<>OLD.target_count+added THEN RAISE EXCEPTION 'commission manifest count mismatch'; END IF;
   IF NEW.scan_complete AND EXISTS(SELECT 1 FROM bet_orders b WHERE b.brand_id=NEW.brand_id AND b.placed_at>=NEW.window_from AND b.placed_at<NEW.window_to AND
    NOT EXISTS(SELECT 1 FROM commission_cycle_targets t WHERE t.cycle_id=NEW.id AND t.order_id=b.id)) THEN RAISE EXCEPTION 'commission manifest incomplete'; END IF;
  ELSE
   IF (NEW.target_count,NEW.manifest_cursor_at,NEW.manifest_cursor_id,NEW.scan_complete) IS DISTINCT FROM
    (OLD.target_count,OLD.manifest_cursor_at,OLD.manifest_cursor_id,OLD.scan_complete) THEN RAISE EXCEPTION 'commission manifest sealed'; END IF;
  END IF;
 END IF;
 NEW.updated_at:=clock_timestamp();RETURN NEW;
END $$;

DO $$
DECLARE app_schema text:=current_schema(); f record;
BEGIN
 IF app_schema IS NULL THEN RAISE EXCEPTION 'application schema unavailable'; END IF;
 FOR f IN SELECT p.oid::regprocedure signature FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace
  WHERE n.nspname=app_schema AND p.proname IN('commission_calendar_boundary','commission_calendar_window','guard_commission_cycle','guard_commission_discovery','enqueue_commission_discovery') LOOP
  EXECUTE format('ALTER FUNCTION %s SET search_path TO pg_catalog, %I, pg_temp',f.signature,app_schema);
 END LOOP;
END $$;
