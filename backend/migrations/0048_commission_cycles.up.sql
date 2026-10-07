-- A closed-window manifest and immutable run generations precede any payout.
-- This migration creates financial calculations, not point-ledger credits.
CREATE FUNCTION advance_commission_evidence_epoch() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_TABLE_NAME='periods' THEN
  IF (OLD.status='settled' OR NEW.status='settled') AND
   (NEW.status,NEW.draw_result_id,NEW.current_settlement_job_id) IS DISTINCT FROM
   (OLD.status,OLD.draw_result_id,OLD.current_settlement_job_id) THEN
   UPDATE commission_cycles c SET evidence_epoch=evidence_epoch+1 WHERE c.brand_id=NEW.brand_id
    AND c.window_from<NEW.bet_end_at AND c.window_to>NEW.bet_start_at
    AND EXISTS(SELECT 1 FROM bet_orders o WHERE o.brand_id=NEW.brand_id AND o.period_id=NEW.id AND o.placed_at>=c.window_from AND o.placed_at<c.window_to);
  END IF;
 ELSE
  IF (NEW.status,NEW.prize_points,NEW.settlement_calculation_id,NEW.payout_entry_id,NEW.refund_entry_id) IS DISTINCT FROM
   (OLD.status,OLD.prize_points,OLD.settlement_calculation_id,OLD.payout_entry_id,OLD.refund_entry_id) AND
   (OLD.status='abnormal' OR EXISTS(SELECT 1 FROM periods WHERE brand_id=NEW.brand_id AND id=NEW.period_id AND status='settled')) THEN
   UPDATE commission_cycles c SET evidence_epoch=evidence_epoch+1 WHERE c.brand_id=NEW.brand_id AND c.window_from<=NEW.placed_at AND c.window_to>NEW.placed_at;
  END IF;
 END IF;
 RETURN NULL;
END $$;
CREATE TRIGGER commission_period_epoch AFTER UPDATE ON periods FOR EACH ROW EXECUTE FUNCTION advance_commission_evidence_epoch();
CREATE TRIGGER commission_order_epoch AFTER UPDATE ON bet_orders FOR EACH ROW EXECUTE FUNCTION advance_commission_evidence_epoch();

CREATE TABLE commission_cycles (
 id uuid PRIMARY KEY,brand_id uuid NOT NULL REFERENCES brands(id),
 window_from timestamptz NOT NULL,window_to timestamptz NOT NULL,anchor_order_id uuid NOT NULL,
 calendar jsonb NOT NULL,state text NOT NULL DEFAULT 'enumerating' CHECK(state IN('enumerating','waiting','calculating','summarizing','ready','failed')),
 version bigint NOT NULL DEFAULT 1 CHECK(version BETWEEN 1 AND 9007199254740991),
 evidence_epoch bigint NOT NULL DEFAULT 0 CHECK(evidence_epoch>=0),
 target_count bigint NOT NULL DEFAULT 0 CHECK(target_count>=0),manifest_cursor_at timestamptz,manifest_cursor_id uuid,
 scan_complete boolean NOT NULL DEFAULT false,current_run_id uuid,
 created_by uuid NOT NULL REFERENCES admin_accounts(id),reason text NOT NULL CHECK(octet_length(reason) BETWEEN 1 AND 500 AND trim(reason)=reason),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 next_work_at timestamptz NOT NULL DEFAULT clock_timestamp(),last_error_code text,
 creation_audit_log_id uuid NOT NULL REFERENCES audit_logs(id),
 UNIQUE(brand_id,id),UNIQUE(brand_id,window_from,window_to),
 FOREIGN KEY(brand_id,anchor_order_id) REFERENCES bet_orders(brand_id,id),
 CHECK(window_from<window_to),CHECK((manifest_cursor_at IS NULL)=(manifest_cursor_id IS NULL)),
 CHECK((state='failed')=(last_error_code IS NOT NULL)),
 CHECK(state IN('enumerating','failed') OR scan_complete),
 CHECK(state NOT IN('calculating','summarizing','ready') OR current_run_id IS NOT NULL)
);
CREATE INDEX commission_cycles_history ON commission_cycles(brand_id,created_at DESC,id DESC);
CREATE INDEX commission_cycles_due ON commission_cycles(next_work_at,id) WHERE state NOT IN('ready','failed');
CREATE TABLE commission_cycle_targets (
 brand_id uuid NOT NULL,cycle_id uuid NOT NULL,order_id uuid NOT NULL,placed_at timestamptz NOT NULL,
 creation_xid xid8 NOT NULL DEFAULT pg_current_xact_id(),
 PRIMARY KEY(cycle_id,order_id),UNIQUE(brand_id,cycle_id,order_id),
 FOREIGN KEY(brand_id,cycle_id) REFERENCES commission_cycles(brand_id,id),
 FOREIGN KEY(brand_id,order_id) REFERENCES bet_orders(brand_id,id)
);
CREATE INDEX commission_target_scan ON commission_cycle_targets(cycle_id,placed_at,order_id);
CREATE INDEX commission_target_xid ON commission_cycle_targets(cycle_id,creation_xid);
CREATE TABLE commission_runs (
 id uuid PRIMARY KEY,brand_id uuid NOT NULL,cycle_id uuid NOT NULL,generation bigint NOT NULL CHECK(generation>0),
 evidence_epoch bigint NOT NULL CHECK(evidence_epoch>=0),state text NOT NULL CHECK(state IN('calculating','summarizing','ready','abandoned')),
 cursor_at timestamptz,cursor_order_id uuid,earnings_cursor_agent uuid,created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(brand_id,cycle_id,id),UNIQUE(cycle_id,generation),
 FOREIGN KEY(brand_id,cycle_id) REFERENCES commission_cycles(brand_id,id),
 CHECK((cursor_at IS NULL)=(cursor_order_id IS NULL))
);
ALTER TABLE commission_cycles ADD FOREIGN KEY(brand_id,id,current_run_id) REFERENCES commission_runs(brand_id,cycle_id,id);
CREATE TABLE commission_cycle_steps (
 id uuid PRIMARY KEY,brand_id uuid NOT NULL,cycle_id uuid NOT NULL,version bigint NOT NULL CHECK(version>1),
 from_state text NOT NULL,to_state text NOT NULL,operation text NOT NULL,run_id uuid,
 reason text NOT NULL CHECK(octet_length(reason) BETWEEN 1 AND 500 AND trim(reason)=reason),
 actor_type text NOT NULL CHECK(actor_type IN('system','admin')),actor_id uuid,audit_log_id uuid NOT NULL REFERENCES audit_logs(id),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),UNIQUE(cycle_id,version),
 FOREIGN KEY(brand_id,cycle_id) REFERENCES commission_cycles(brand_id,id),
 FOREIGN KEY(brand_id,cycle_id,run_id) REFERENCES commission_runs(brand_id,cycle_id,id),
 CHECK((actor_type='admin')=(actor_id IS NOT NULL))
);
CREATE TABLE commission_calculations (
 id uuid PRIMARY KEY,brand_id uuid NOT NULL,cycle_id uuid NOT NULL,run_id uuid NOT NULL,order_id uuid NOT NULL,
 reason text NOT NULL CHECK(reason IN('eligible','policy_disabled','unattributed','other_cycle','cancelled','abnormal')),
 status text NOT NULL,member_id uuid NOT NULL,account_id uuid NOT NULL,
 stake_points bigint NOT NULL CHECK(stake_points>0),prize_points bigint NOT NULL CHECK(prize_points>=0),base_points bigint NOT NULL CHECK(base_points>=0),
 job_id uuid,calculation_id uuid,generation bigint,rule_snapshot jsonb NOT NULL CHECK(jsonb_typeof(rule_snapshot)='object'),
 audit_log_id uuid NOT NULL REFERENCES audit_logs(id),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(run_id,order_id),UNIQUE(brand_id,cycle_id,run_id,id),
 FOREIGN KEY(brand_id,cycle_id,run_id) REFERENCES commission_runs(brand_id,cycle_id,id),
 FOREIGN KEY(brand_id,cycle_id,order_id) REFERENCES commission_cycle_targets(brand_id,cycle_id,order_id),
 FOREIGN KEY(brand_id,account_id,member_id) REFERENCES point_accounts(brand_id,id,brand_member_id),
 CHECK((job_id IS NULL)=(calculation_id IS NULL)),CHECK((job_id IS NULL)=(generation IS NULL)),
 CHECK(generation IS NULL OR generation>0)
);
CREATE INDEX commission_calculations_run ON commission_calculations(run_id,order_id);
CREATE FUNCTION valid_commission_fraction(n text,d text) RETURNS boolean LANGUAGE plpgsql IMMUTABLE AS $$
BEGIN
 IF n IS NULL OR d IS NULL OR length(n)>100 OR n !~ '^(0|[1-9][0-9]*)$' OR d !~ '^[1-9][0-9]{0,6}$' THEN RETURN false; END IF;
 RETURN d::numeric<=1000000 AND mod(1000000,d::numeric)=0 AND gcd(n::numeric,d::numeric)=1;
EXCEPTION WHEN OTHERS THEN RETURN false;
END
$$;
CREATE TABLE commission_allocations (
 brand_id uuid NOT NULL,cycle_id uuid NOT NULL,run_id uuid NOT NULL,calculation_id uuid NOT NULL,order_id uuid NOT NULL,
 agent_id uuid NOT NULL,member_id uuid NOT NULL,numerator text NOT NULL,denominator text NOT NULL,
 PRIMARY KEY(run_id,order_id,agent_id),
 FOREIGN KEY(brand_id,cycle_id,run_id,calculation_id) REFERENCES commission_calculations(brand_id,cycle_id,run_id,id),
 FOREIGN KEY(brand_id,agent_id) REFERENCES agent_nodes(brand_id,id),
 FOREIGN KEY(brand_id,member_id) REFERENCES brand_members(brand_id,id),
 CHECK(valid_commission_fraction(numerator,denominator) IS TRUE)
);
CREATE INDEX commission_allocations_sum ON commission_allocations(run_id,agent_id,member_id);
CREATE TABLE commission_earnings (
 id uuid PRIMARY KEY,brand_id uuid NOT NULL,cycle_id uuid NOT NULL,run_id uuid NOT NULL,agent_id uuid NOT NULL,member_id uuid NOT NULL,
 numerator text NOT NULL,denominator text NOT NULL,points bigint NOT NULL CHECK(points>=0),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(run_id,agent_id),UNIQUE(brand_id,cycle_id,run_id,id),
 FOREIGN KEY(brand_id,cycle_id,run_id) REFERENCES commission_runs(brand_id,cycle_id,id),
 FOREIGN KEY(brand_id,agent_id) REFERENCES agent_nodes(brand_id,id),
 FOREIGN KEY(brand_id,member_id) REFERENCES brand_members(brand_id,id),
 CHECK(valid_commission_fraction(numerator,denominator) IS TRUE)
);

CREATE FUNCTION guard_commission_cycle() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE a audit_logs; o bet_orders; step commission_cycle_steps; added bigint;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'commission cycle history immutable'; END IF;
 IF TG_OP='INSERT' THEN
  -- Shares the betting admission mutex. In-flight API bets complete first;
  -- later backdated SQL inserts are rejected by closed-window admission.
  PERFORM 1 FROM brand_commission_policies WHERE brand_id=NEW.brand_id FOR UPDATE;
  SELECT * INTO o FROM bet_orders WHERE brand_id=NEW.brand_id AND id=NEW.anchor_order_id;
  IF NOT FOUND OR o.commission_rule_snapshot IS NULL OR o.commission_rule_snapshot->'financial_policy'->'config'->'enabled'<>'true'::jsonb OR
   o.commission_rule_snapshot->'financial_policy'->'config'->'calendar' IS DISTINCT FROM NEW.calendar OR
   o.placed_at<NEW.window_from OR o.placed_at>=NEW.window_to OR NEW.window_to>clock_timestamp() OR
   NEW.state<>'enumerating' OR NEW.version<>1 OR NEW.evidence_epoch<>0 OR NEW.target_count<>0 OR NEW.scan_complete OR NEW.current_run_id IS NOT NULL OR
   NEW.manifest_cursor_at IS NOT NULL OR NEW.manifest_cursor_id IS NOT NULL THEN RAISE EXCEPTION 'invalid closed commission cycle admission'; END IF;
  SELECT * INTO a FROM audit_logs WHERE id=NEW.creation_audit_log_id;
  IF NOT FOUND OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.actor_type<>'admin' OR a.actor_id IS DISTINCT FROM NEW.created_by OR
   a.resource_type<>'commission_cycle' OR a.resource_id IS DISTINCT FROM NEW.id OR a.action<>'commission.cycle.create' OR a.reason<>NEW.reason OR
   a.after_json->>'version'<>'1' OR a.after_json->>'anchor_order_id' IS DISTINCT FROM NEW.anchor_order_id::text OR
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
 NEW.updated_at:=clock_timestamp(); RETURN NEW;
END $$;
CREATE TRIGGER guarded_commission_cycle BEFORE INSERT OR UPDATE OR DELETE ON commission_cycles FOR EACH ROW EXECUTE FUNCTION guard_commission_cycle();
CREATE FUNCTION guard_closed_commission_admission() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 -- Alphabetically after capture_bet_commission_snapshot, which holds SHARE.
 IF EXISTS(SELECT 1 FROM commission_cycles WHERE brand_id=NEW.brand_id AND window_from<=NEW.placed_at AND window_to>NEW.placed_at) THEN
  RAISE EXCEPTION 'bet belongs to sealed commission window'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER commission_closed_window_admission BEFORE INSERT ON bet_orders FOR EACH ROW EXECUTE FUNCTION guard_closed_commission_admission();
CREATE FUNCTION guard_commission_target() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE c commission_cycles; o bet_orders;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'commission manifest immutable'; END IF;
 SELECT * INTO c FROM commission_cycles WHERE brand_id=NEW.brand_id AND id=NEW.cycle_id;
 SELECT * INTO o FROM bet_orders WHERE brand_id=NEW.brand_id AND id=NEW.order_id;
 IF c.id IS NULL OR c.state<>'enumerating' OR c.scan_complete OR o.id IS NULL OR NEW.placed_at<>o.placed_at OR
  o.placed_at<c.window_from OR o.placed_at>=c.window_to OR
  (c.manifest_cursor_at IS NOT NULL AND (NEW.placed_at,NEW.order_id)<=(c.manifest_cursor_at,c.manifest_cursor_id)) THEN RAISE EXCEPTION 'invalid commission manifest target'; END IF;
 NEW.creation_xid:=pg_current_xact_id(); RETURN NEW;
END $$;
CREATE TRIGGER guarded_commission_target BEFORE INSERT OR UPDATE OR DELETE ON commission_cycle_targets FOR EACH ROW EXECUTE FUNCTION guard_commission_target();
CREATE FUNCTION guard_commission_step() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE c commission_cycles; a audit_logs;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'commission steps immutable'; END IF;
 SELECT * INTO c FROM commission_cycles WHERE brand_id=NEW.brand_id AND id=NEW.cycle_id;
 SELECT * INTO a FROM audit_logs WHERE id=NEW.audit_log_id;
 IF c.id IS NULL OR NEW.version<>c.version+1 OR NEW.from_state<>c.state OR a.id IS NULL OR a.brand_id IS DISTINCT FROM NEW.brand_id OR
  a.actor_type<>NEW.actor_type OR a.actor_id IS DISTINCT FROM NEW.actor_id OR a.resource_type<>'commission_cycle' OR a.resource_id IS DISTINCT FROM NEW.cycle_id OR
  a.action IS DISTINCT FROM 'commission.cycle.'||NEW.operation OR a.reason<>NEW.reason OR
  a.after_json->>'version' IS DISTINCT FROM NEW.version::text OR a.after_json->>'state' IS DISTINCT FROM NEW.to_state THEN RAISE EXCEPTION 'commission step needs matching audit'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_commission_step BEFORE INSERT OR UPDATE OR DELETE ON commission_cycle_steps FOR EACH ROW EXECUTE FUNCTION guard_commission_step();
CREATE FUNCTION require_commission_step_commit() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM commission_cycles WHERE id=NEW.cycle_id AND version>=NEW.version) THEN RAISE EXCEPTION 'orphan commission step'; END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER commission_step_commit AFTER INSERT ON commission_cycle_steps DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_commission_step_commit();
CREATE FUNCTION guard_commission_run() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE c commission_cycles; actual_epoch bigint;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'commission runs immutable'; END IF;
 SELECT * INTO c FROM commission_cycles WHERE brand_id=NEW.brand_id AND id=NEW.cycle_id;
 actual_epoch:=c.evidence_epoch;
 IF TG_OP='INSERT' THEN
  IF c.id IS NULL OR c.state<>'waiting' OR NOT c.scan_complete OR NEW.state<>'calculating' OR NEW.evidence_epoch<>actual_epoch OR
   NEW.generation<>(SELECT coalesce(max(generation),0)+1 FROM commission_runs WHERE cycle_id=NEW.cycle_id) OR
   NEW.cursor_at IS NOT NULL OR NEW.cursor_order_id IS NOT NULL OR NEW.earnings_cursor_agent IS NOT NULL THEN RAISE EXCEPTION 'invalid commission run'; END IF;
 ELSE
  IF (to_jsonb(NEW)-ARRAY['state','cursor_at','cursor_order_id','earnings_cursor_agent']) IS DISTINCT FROM
   (to_jsonb(OLD)-ARRAY['state','cursor_at','cursor_order_id','earnings_cursor_agent']) OR OLD.state='abandoned' OR
   (OLD.state='ready' AND NEW.state<>'abandoned') OR
   c.current_run_id IS DISTINCT FROM NEW.id OR NEW.state NOT IN(OLD.state,'summarizing','ready','abandoned') THEN RAISE EXCEPTION 'invalid commission run transition'; END IF;
  IF NEW.state='summarizing' AND OLD.state='calculating' AND
   (EXISTS(SELECT 1 FROM commission_cycle_targets t WHERE t.cycle_id=NEW.cycle_id AND NOT EXISTS(SELECT 1 FROM commission_calculations x WHERE x.run_id=NEW.id AND x.order_id=t.order_id)) OR
    EXISTS(SELECT 1 FROM commission_calculations x CROSS JOIN LATERAL jsonb_array_elements(x.rule_snapshot->'agent_path') node
     WHERE x.run_id=NEW.id AND x.reason='eligible' AND NOT EXISTS(SELECT 1 FROM commission_allocations a WHERE a.run_id=x.run_id AND a.order_id=x.order_id AND a.agent_id=(node->>'id')::uuid))) THEN RAISE EXCEPTION 'commission run calculations incomplete'; END IF;
  IF NEW.state='ready' AND (OLD.state<>'summarizing' OR NEW.evidence_epoch<>actual_epoch OR
   EXISTS(SELECT 1 FROM commission_allocations a WHERE a.run_id=NEW.id AND NOT EXISTS(SELECT 1 FROM commission_earnings e WHERE e.run_id=a.run_id AND e.agent_id=a.agent_id AND e.member_id=a.member_id))) THEN RAISE EXCEPTION 'commission run earnings incomplete'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_commission_run BEFORE INSERT OR UPDATE OR DELETE ON commission_runs FOR EACH ROW EXECUTE FUNCTION guard_commission_run();
CREATE FUNCTION require_commission_run_commit() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM commission_cycles c WHERE c.brand_id=NEW.brand_id AND c.id=NEW.cycle_id AND c.current_run_id=NEW.id) THEN RAISE EXCEPTION 'orphan commission run'; END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER commission_run_commit AFTER INSERT ON commission_runs DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_commission_run_commit();
CREATE FUNCTION guard_commission_calculation() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE c commission_cycles; r commission_runs; o bet_orders; p periods; calc settlement_calculations;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'commission calculations immutable'; END IF;
 SELECT * INTO c FROM commission_cycles WHERE brand_id=NEW.brand_id AND id=NEW.cycle_id;
 SELECT * INTO r FROM commission_runs WHERE id=NEW.run_id;
 SELECT * INTO o FROM bet_orders WHERE brand_id=NEW.brand_id AND id=NEW.order_id;
 SELECT * INTO p FROM periods WHERE brand_id=NEW.brand_id AND id=o.period_id;
 IF c.state<>'calculating' OR c.current_run_id IS DISTINCT FROM NEW.run_id OR r.state<>'calculating' OR o.id IS NULL OR
  NEW.rule_snapshot IS DISTINCT FROM o.commission_rule_snapshot OR NEW.status<>o.status OR NEW.member_id<>o.brand_member_id OR NEW.account_id<>o.account_id OR
  NEW.stake_points<>o.total_points OR NEW.prize_points<>o.prize_points OR r.evidence_epoch<>c.evidence_epoch OR
  NOT EXISTS(SELECT 1 FROM audit_logs a WHERE a.id=NEW.audit_log_id AND a.brand_id=NEW.brand_id AND a.action='commission.cycle.calculation_page' AND a.resource_type='commission_cycle' AND a.resource_id=NEW.cycle_id) THEN RAISE EXCEPTION 'commission calculation identity/evidence mismatch'; END IF;
 IF o.status IN('won','lost') THEN
  IF NEW.reason NOT IN('eligible','policy_disabled','unattributed','other_cycle') THEN RAISE EXCEPTION 'commission final-order disposition mismatch'; END IF;
  SELECT * INTO calc FROM settlement_calculations WHERE brand_id=NEW.brand_id AND id=o.settlement_calculation_id;
  IF p.status<>'settled' OR NEW.job_id IS DISTINCT FROM p.current_settlement_job_id OR NEW.calculation_id IS DISTINCT FROM calc.id OR
   NOT EXISTS(SELECT 1 FROM settlement_jobs j WHERE j.id=NEW.job_id AND j.brand_id=NEW.brand_id AND j.state='completed' AND j.generation=NEW.generation AND j.draw_result_id=p.draw_result_id) OR
   calc.job_id IS DISTINCT FROM NEW.job_id OR calc.order_id IS DISTINCT FROM NEW.order_id OR calc.prize_points<>NEW.prize_points OR
   calc.won<>(o.status='won') THEN RAISE EXCEPTION 'commission calculation final generation mismatch'; END IF;
 ELSE
  IF NEW.job_id IS NOT NULL OR NEW.calculation_id IS NOT NULL OR NEW.generation IS NOT NULL OR NEW.base_points<>0 OR
   NOT ((o.status IN('bet_cancelled','judged_cancelled') AND NEW.reason='cancelled') OR (o.status='abnormal' AND NEW.reason='abnormal')) THEN RAISE EXCEPTION 'commission excluded order mismatch'; END IF;
 END IF;
 IF NEW.reason='eligible' THEN
  IF NEW.rule_snapshot->'financial_policy'->'config'->'enabled'<>'true'::jsonb OR jsonb_array_length(NEW.rule_snapshot->'agent_path')=0 OR
   NEW.base_points IS DISTINCT FROM (CASE WHEN NEW.rule_snapshot->'agent_path'->0->>'effective_mode'='turnover' OR o.status='lost' THEN o.total_points ELSE 0 END) THEN RAISE EXCEPTION 'commission eligible base mismatch'; END IF;
 ELSIF NEW.base_points<>0 THEN RAISE EXCEPTION 'excluded commission must have zero base'; END IF;
 IF NEW.reason='policy_disabled' AND NEW.rule_snapshot->'financial_policy'->'config'->'enabled'<>'false'::jsonb THEN RAISE EXCEPTION 'commission policy-disabled disposition mismatch'; END IF;
 IF NEW.reason='unattributed' AND (NEW.rule_snapshot->'financial_policy'->'config'->'enabled'<>'true'::jsonb OR jsonb_array_length(NEW.rule_snapshot->'agent_path')<>0) THEN RAISE EXCEPTION 'commission unattributed disposition mismatch'; END IF;
 IF NEW.reason='other_cycle' AND (NEW.rule_snapshot->'financial_policy'->'config'->'enabled'<>'true'::jsonb OR NEW.rule_snapshot->'financial_policy'->'config'->'calendar' IS NOT DISTINCT FROM c.calendar) THEN RAISE EXCEPTION 'commission other-cycle disposition mismatch'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_commission_calculation BEFORE INSERT OR UPDATE OR DELETE ON commission_calculations FOR EACH ROW EXECUTE FUNCTION guard_commission_calculation();
CREATE FUNCTION guard_commission_allocation() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE c commission_calculations; node jsonb; next_node jsonb; rate numeric;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'commission allocations immutable'; END IF;
 SELECT * INTO c FROM commission_calculations WHERE brand_id=NEW.brand_id AND cycle_id=NEW.cycle_id AND run_id=NEW.run_id AND id=NEW.calculation_id;
 SELECT value INTO node FROM jsonb_array_elements(c.rule_snapshot->'agent_path') WHERE value->>'id'=NEW.agent_id::text;
 SELECT value INTO next_node FROM jsonb_array_elements(c.rule_snapshot->'agent_path') WHERE (value->>'depth')::int=(node->>'depth')::int+1;
 rate:=((node->'config'->>'ratio')::numeric-coalesce((next_node->'config'->>'ratio')::numeric,0))*1000000;
 IF c.id IS NULL OR c.reason<>'eligible' OR c.order_id<>NEW.order_id OR node IS NULL OR node->>'member_id' IS DISTINCT FROM NEW.member_id::text OR
  rate<0 OR NEW.numerator::numeric*1000000 IS DISTINCT FROM c.base_points::numeric*rate*NEW.denominator::numeric THEN RAISE EXCEPTION 'commission differential allocation mismatch'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_commission_allocation BEFORE INSERT OR UPDATE OR DELETE ON commission_allocations FOR EACH ROW EXECUTE FUNCTION guard_commission_allocation();
CREATE FUNCTION guard_commission_earning() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE r commission_runs; c commission_cycles; micro numeric;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'commission earnings immutable'; END IF;
 SELECT * INTO r FROM commission_runs WHERE brand_id=NEW.brand_id AND cycle_id=NEW.cycle_id AND id=NEW.run_id;
 SELECT * INTO c FROM commission_cycles WHERE brand_id=NEW.brand_id AND id=NEW.cycle_id;
 SELECT sum(numerator::numeric*(1000000/denominator::numeric)) INTO micro FROM commission_allocations WHERE run_id=NEW.run_id AND agent_id=NEW.agent_id AND member_id=NEW.member_id;
 IF r.state<>'summarizing' OR c.state<>'summarizing' OR c.current_run_id IS DISTINCT FROM NEW.run_id OR r.evidence_epoch<>c.evidence_epoch OR micro IS NULL OR NEW.numerator::numeric*1000000 IS DISTINCT FROM micro*NEW.denominator::numeric OR
  NEW.points::numeric IS DISTINCT FROM floor((micro*2+1000000)/2000000) THEN RAISE EXCEPTION 'commission cycle earning total mismatch'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_commission_earning BEFORE INSERT OR UPDATE OR DELETE ON commission_earnings FOR EACH ROW EXECUTE FUNCTION guard_commission_earning();

INSERT INTO permissions(key) VALUES('commission.view.brand'),('commission.view.platform'),('commission.run.brand'),('commission.retry.brand') ON CONFLICT DO NOTHING;
INSERT INTO role_permissions(role_id,permission_key) SELECT r.id,p.key FROM roles r CROSS JOIN permissions p WHERE r.is_bootstrap AND
 ((r.brand_id IS NOT NULL AND p.key IN('commission.view.brand','commission.run.brand','commission.retry.brand')) OR
 (r.brand_id IS NULL AND p.key='commission.view.platform')) ON CONFLICT DO NOTHING;
DO $$
DECLARE app_schema text:=current_schema(); f record;
BEGIN
 IF app_schema IS NULL THEN RAISE EXCEPTION 'application schema required'; END IF;
 FOR f IN SELECT p.proname,pg_get_function_identity_arguments(p.oid) args FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace
  WHERE n.nspname=app_schema AND p.proname IN('advance_commission_evidence_epoch','valid_commission_fraction',
   'guard_commission_cycle','guard_closed_commission_admission','guard_commission_target','guard_commission_step','require_commission_step_commit',
   'guard_commission_run','require_commission_run_commit','guard_commission_calculation','guard_commission_allocation','guard_commission_earning')
 LOOP EXECUTE format('ALTER FUNCTION %I.%I(%s) SET search_path = pg_catalog, %I, pg_temp',app_schema,f.proname,f.args,app_schema); END LOOP;
END $$;
