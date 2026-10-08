-- Preparation only: an immutable per-agent difference plan is NOT an approval,
-- posting, reclaim, or settled compensation. No existing money/history changes.
CREATE FUNCTION commission_correction_candidates(b uuid,pid uuid,rid uuid)
RETURNS TABLE(agent_id uuid,member_id uuid,original_target_id uuid,earning_id uuid,adjustment_version bigint,points_before bigint,points_after bigint,delta_points bigint)
LANGUAGE sql STABLE AS $$
 WITH original AS (
  SELECT t.agent_id,t.member_id,t.id,h.version,h.points
  FROM commission_payment_targets t JOIN commission_adjustment_heads h ON h.brand_id=t.brand_id AND h.target_id=t.id
  WHERE t.brand_id=b AND t.payment_id=pid AND t.state='paid'
 ), corrected AS (SELECT * FROM commission_earnings WHERE brand_id=b AND run_id=rid)
 SELECT coalesce(o.agent_id,e.agent_id),coalesce(o.member_id,e.member_id),o.id,e.id,o.version,
  coalesce(o.points,0),coalesce(e.points,0),coalesce(e.points,0)-coalesce(o.points,0)
 FROM original o FULL JOIN corrected e ON e.agent_id=o.agent_id
$$;
CREATE FUNCTION commission_correction_source_valid(b uuid,pid uuid,rid uuid) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT EXISTS(SELECT 1 FROM commission_payments p JOIN commission_runs r ON r.brand_id=p.brand_id AND r.cycle_id=p.cycle_id AND r.id=rid
  WHERE p.brand_id=b AND p.id=pid AND p.run_id<>rid AND p.state='blocked' AND p.last_error_code='COMMISSION_PAYMENT_CORRECTION_REQUIRED'
  AND EXISTS(SELECT 1 FROM commission_payment_targets t WHERE t.payment_id=p.id AND t.state='paid' AND t.points>0 AND t.ledger_entry_id IS NOT NULL))
 AND NOT EXISTS(
  SELECT 1 FROM commission_payment_targets t
  LEFT JOIN commission_adjustment_heads h ON h.brand_id=t.brand_id AND h.target_id=t.id
  LEFT JOIN point_ledger_entries l ON l.id=t.ledger_entry_id
  LEFT JOIN audit_logs a ON a.id=t.audit_log_id
  LEFT JOIN commission_earnings e ON e.brand_id=b AND e.run_id=rid AND e.agent_id=t.agent_id
  WHERE t.brand_id=b AND t.payment_id=pid AND t.state='paid' AND (
   h.target_id IS NULL OR h.version=1 AND (h.points<>t.points OR h.last_adjustment_id IS NOT NULL)
   OR h.version>1 AND NOT EXISTS(SELECT 1 FROM commission_adjustments x WHERE x.brand_id=b AND x.target_id=t.id AND x.id=h.last_adjustment_id AND x.version=h.version AND x.points_after=h.points AND x.ledger_entry_id IS NOT NULL)
   OR a.id IS NULL OR a.brand_id IS DISTINCT FROM b OR a.action<>'commission.payment.target' OR a.resource_type<>'commission_payment_target' OR a.resource_id IS DISTINCT FROM t.id
   OR t.points>0 AND (l.id IS NULL OR l.brand_id<>b OR l.member_id<>t.member_id OR l.entry_type<>'commission' OR l.reference_type<>'commission_payment_target' OR l.reference_id IS DISTINCT FROM t.id
    OR l.operation_key<>'commission-payment:'||t.id::text OR l.actor_type<>'system' OR l.actor_id IS NOT NULL OR l.reversal_of IS NOT NULL
    OR l.delta_snapshot IS DISTINCT FROM jsonb_set(point_zero_snapshot(),'{commission,available}',to_jsonb(t.points::text))
    OR l.source_allocation IS DISTINCT FROM jsonb_build_array(jsonb_build_object('source','commission','state','available','points',t.points::text)))
   OR t.points=0 AND t.ledger_entry_id IS NOT NULL OR e.id IS NOT NULL AND e.member_id<>t.member_id))
$$;
CREATE FUNCTION commission_correction_mode(original text,corrected text) RETURNS text LANGUAGE sql IMMUTABLE AS $$
 SELECT CASE WHEN original NOT IN('manual','automatic','mixed','none') OR corrected NOT IN('manual','automatic','mixed','none') THEN NULL
  WHEN original='mixed' OR corrected='mixed' OR original<>corrected AND original<>'none' AND corrected<>'none' THEN 'mixed'
  WHEN original='none' THEN corrected ELSE original END
$$;
CREATE TABLE commission_correction_plans (
 id uuid PRIMARY KEY,brand_id uuid NOT NULL,cycle_id uuid NOT NULL,payment_id uuid NOT NULL,run_id uuid NOT NULL,
 evidence_epoch bigint NOT NULL CHECK(evidence_epoch>=0),payout_mode text NOT NULL CHECK(payout_mode IN('manual','automatic','mixed','none')),
 state text NOT NULL CHECK(state IN('planning','ready','blocked','failed','stale')),
 version bigint NOT NULL DEFAULT 1 CHECK(version BETWEEN 1 AND 9007199254740991),
 before_points numeric NOT NULL CHECK(before_points>=0 AND scale(before_points)=0),
 calculated_points numeric NOT NULL CHECK(calculated_points>=0 AND scale(calculated_points)=0),
 credit_points numeric CHECK(credit_points>=0 AND scale(credit_points)=0),debit_points numeric CHECK(debit_points>=0 AND scale(debit_points)=0),
 net_points numeric CHECK(scale(net_points)=0),target_count bigint NOT NULL CHECK(target_count BETWEEN 0 AND 100000),
 planned_count bigint NOT NULL DEFAULT 0 CHECK(planned_count BETWEEN 0 AND target_count),cursor_agent_id uuid,
 creation_audit_log_id uuid NOT NULL REFERENCES audit_logs(id),last_audit_log_id uuid NOT NULL REFERENCES audit_logs(id),last_error_code text,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),next_work_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 creation_xid xid8 NOT NULL DEFAULT pg_current_xact_id(),
 UNIQUE(brand_id,id),UNIQUE(cycle_id,run_id),
 FOREIGN KEY(brand_id,payment_id) REFERENCES commission_payments(brand_id,id),
 FOREIGN KEY(brand_id,cycle_id,run_id) REFERENCES commission_runs(brand_id,cycle_id,id),
 CHECK((state IN('blocked','failed'))=(last_error_code IS NOT NULL)),
 CHECK((credit_points IS NULL)=(debit_points IS NULL) AND (credit_points IS NULL)=(net_points IS NULL)),
 CHECK(net_points IS NULL OR net_points=credit_points-debit_points AND net_points=calculated_points-before_points),
 CHECK(state<>'ready' OR planned_count=target_count AND credit_points IS NOT NULL),
 CHECK(state<>'blocked' OR last_error_code='COMMISSION_CORRECTION_MANUAL_POLICY_UNRESOLVED' AND credit_points IS NULL AND planned_count=0),
 CHECK((planned_count=0)=(cursor_agent_id IS NULL))
);
CREATE UNIQUE INDEX commission_correction_one_live_plan ON commission_correction_plans(cycle_id) WHERE state<>'stale';
CREATE INDEX commission_correction_plan_due ON commission_correction_plans(next_work_at,id) WHERE state='planning';
CREATE TABLE commission_correction_plan_steps (
 id uuid PRIMARY KEY,brand_id uuid NOT NULL,plan_id uuid NOT NULL,version bigint NOT NULL CHECK(version BETWEEN 1 AND 9007199254740991),
 from_state text,to_state text NOT NULL,operation text NOT NULL CHECK(operation IN('create','page','ready','invalidate','fail','retry')),
 planned_count bigint NOT NULL CHECK(planned_count>=0),cursor_agent_id uuid,error_code text,
 actor_type text NOT NULL CHECK(actor_type IN('admin','system')),actor_id uuid REFERENCES admin_accounts(id),
 reason text NOT NULL CHECK(octet_length(reason) BETWEEN 1 AND 500),request_id text NOT NULL,
 audit_log_id uuid NOT NULL REFERENCES audit_logs(id),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(plan_id,version),UNIQUE(audit_log_id),FOREIGN KEY(brand_id,plan_id) REFERENCES commission_correction_plans(brand_id,id),
 CHECK((actor_type='admin')=(actor_id IS NOT NULL)),CHECK((version=1)=(from_state IS NULL))
);
CREATE TABLE commission_correction_plan_targets (
 id uuid PRIMARY KEY,brand_id uuid NOT NULL,plan_id uuid NOT NULL,agent_id uuid NOT NULL,member_id uuid NOT NULL,
 original_target_id uuid,earning_id uuid REFERENCES commission_earnings(id),adjustment_version bigint,
 points_before bigint NOT NULL CHECK(points_before>=0),points_after bigint NOT NULL CHECK(points_after>=0),delta_points bigint NOT NULL,
 plan_version bigint NOT NULL CHECK(plan_version BETWEEN 2 AND 9007199254740991),
 creation_audit_log_id uuid NOT NULL REFERENCES audit_logs(id),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),creation_xid xid8 NOT NULL DEFAULT pg_current_xact_id(),
 UNIQUE(plan_id,agent_id),FOREIGN KEY(brand_id,plan_id) REFERENCES commission_correction_plans(brand_id,id),
 FOREIGN KEY(brand_id,original_target_id) REFERENCES commission_payment_targets(brand_id,id),
 FOREIGN KEY(brand_id,member_id) REFERENCES brand_members(brand_id,id),
 CHECK(points_after::numeric-points_before::numeric=delta_points::numeric),
 CHECK(original_target_id IS NOT NULL OR earning_id IS NOT NULL),CHECK((original_target_id IS NULL)=(adjustment_version IS NULL))
);
CREATE FUNCTION guard_commission_correction_plan_step() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p commission_correction_plans; a audit_logs;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'commission correction plan steps immutable'; END IF;
 SELECT * INTO p FROM commission_correction_plans WHERE brand_id=NEW.brand_id AND id=NEW.plan_id FOR UPDATE NOWAIT;
 SELECT * INTO a FROM audit_logs WHERE id=NEW.audit_log_id;
 IF p.id IS NULL OR NEW.version<>(CASE WHEN NEW.operation='create' THEN p.version ELSE p.version+1 END) OR
  NEW.operation='create' AND (p.version<>1 OR p.creation_xid<>pg_current_xact_id() OR NEW.to_state<>p.state OR NEW.planned_count<>p.planned_count OR NEW.cursor_agent_id IS DISTINCT FROM p.cursor_agent_id OR NEW.error_code IS DISTINCT FROM p.last_error_code) OR NEW.operation<>'create' AND NEW.from_state IS DISTINCT FROM p.state OR
  a.id IS NULL OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.resource_type<>'commission_correction_plan' OR a.resource_id IS DISTINCT FROM NEW.plan_id OR
  a.action<>'commission.correction_plan.'||NEW.operation OR a.actor_type<>NEW.actor_type OR a.actor_id IS DISTINCT FROM NEW.actor_id OR a.reason<>NEW.reason OR a.request_id<>NEW.request_id OR
  a.after_json IS DISTINCT FROM jsonb_build_object('version',NEW.version,'state',NEW.to_state,'planned_count',NEW.planned_count::text,'cursor_agent_id',NEW.cursor_agent_id,'error_code',NEW.error_code) OR
  a.before_json IS DISTINCT FROM (CASE WHEN NEW.operation='create' THEN 'null'::jsonb ELSE jsonb_build_object('version',p.version,'state',p.state) END)
 THEN RAISE EXCEPTION 'commission correction step requires exact live audit and version'; END IF;
 IF NEW.operation='retry' THEN
  IF NEW.actor_type<>'admin' OR NOT EXISTS(SELECT 1 FROM admin_accounts WHERE id=NEW.actor_id AND status='active' AND NOT is_super_admin) THEN RAISE EXCEPTION 'retry requires current ordinary administrator'; END IF;
 ELSIF NEW.actor_type<>'system' OR NEW.actor_id IS NOT NULL THEN RAISE EXCEPTION 'preparation steps require system actor'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_commission_correction_plan_step BEFORE INSERT OR UPDATE OR DELETE ON commission_correction_plan_steps FOR EACH ROW EXECUTE FUNCTION guard_commission_correction_plan_step();
CREATE FUNCTION guard_commission_correction_plan() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE c commission_cycles; pay commission_payments; step commission_correction_plan_steps; a audit_logs; n bigint; before_sum numeric; after_sum numeric; credits numeric; debits numeric; manual_changed boolean;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'commission correction plan history retained'; END IF;
 IF TG_OP='UPDATE' AND NEW.version=OLD.version AND (to_jsonb(NEW)-'next_work_at')=(to_jsonb(OLD)-'next_work_at') THEN RETURN NEW; END IF;
 SELECT * INTO c FROM commission_cycles WHERE brand_id=NEW.brand_id AND id=NEW.cycle_id FOR UPDATE NOWAIT;
 SELECT * INTO pay FROM commission_payments WHERE brand_id=NEW.brand_id AND id=NEW.payment_id FOR SHARE NOWAIT;
 IF TG_OP='INSERT' THEN
  PERFORM 1 FROM brands WHERE id=NEW.brand_id AND status<>'disabled' FOR SHARE NOWAIT;
  IF NOT FOUND THEN RAISE EXCEPTION 'correction preparation brand unavailable'; END IF;
  PERFORM 1 FROM brand_commission_payment_policies WHERE brand_id=NEW.brand_id AND enabled FOR SHARE NOWAIT;
  IF NOT FOUND OR pay.cycle_id IS DISTINCT FROM NEW.cycle_id OR NOT commission_payment_evidence_current(NEW.brand_id,NEW.cycle_id,NEW.run_id,NEW.evidence_epoch) OR
   NOT commission_correction_source_valid(NEW.brand_id,NEW.payment_id,NEW.run_id) THEN RAISE EXCEPTION 'correction preparation requires actual prior credit and current evidence'; END IF;
  SELECT count(*),coalesce(sum(points_before::numeric),0),coalesce(sum(points_after::numeric),0),coalesce(sum(greatest(delta_points,0)::numeric),0),coalesce(sum(greatest(-delta_points,0)::numeric),0),coalesce(bool_or(adjustment_version>1),false)
   INTO n,before_sum,after_sum,credits,debits,manual_changed FROM commission_correction_candidates(NEW.brand_id,NEW.payment_id,NEW.run_id);
  SELECT * INTO a FROM audit_logs WHERE id=NEW.creation_audit_log_id;
  IF n>100000 OR NEW.target_count<>n OR NEW.before_points<>before_sum OR NEW.calculated_points<>after_sum OR
   NEW.payout_mode IS DISTINCT FROM commission_correction_mode(pay.payout_mode,commission_payment_mode(NEW.run_id)) OR
   NEW.version<>1 OR NEW.planned_count<>0 OR NEW.cursor_agent_id IS NOT NULL OR NEW.creation_xid<>pg_current_xact_id() OR NEW.last_audit_log_id<>NEW.creation_audit_log_id OR
   (manual_changed AND (NEW.state<>'blocked' OR NEW.last_error_code IS DISTINCT FROM 'COMMISSION_CORRECTION_MANUAL_POLICY_UNRESOLVED' OR NEW.credit_points IS NOT NULL)) OR
   (NOT manual_changed AND (NEW.state<>'planning' OR NEW.credit_points IS DISTINCT FROM credits OR NEW.debit_points IS DISTINCT FROM debits OR NEW.net_points IS DISTINCT FROM after_sum-before_sum)) OR
   a.id IS NULL OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.actor_type<>'system' OR a.actor_id IS NOT NULL OR a.action<>'commission.correction_plan.create' OR a.resource_type<>'commission_correction_plan' OR a.resource_id IS DISTINCT FROM NEW.id
   THEN RAISE EXCEPTION 'correction plan source/amount/version mismatch'; END IF;
 ELSE
  IF (to_jsonb(NEW)-ARRAY['state','version','planned_count','cursor_agent_id','last_audit_log_id','last_error_code','updated_at','next_work_at']) IS DISTINCT FROM
   (to_jsonb(OLD)-ARRAY['state','version','planned_count','cursor_agent_id','last_audit_log_id','last_error_code','updated_at','next_work_at']) OR NEW.version<>OLD.version+1
   THEN RAISE EXCEPTION 'correction plan identity and amounts immutable'; END IF;
  SELECT * INTO step FROM commission_correction_plan_steps WHERE brand_id=NEW.brand_id AND plan_id=NEW.id AND version=NEW.version;
  IF step.id IS NULL OR step.from_state IS DISTINCT FROM OLD.state OR step.to_state<>NEW.state OR step.audit_log_id<>NEW.last_audit_log_id OR
   step.planned_count<>NEW.planned_count OR step.cursor_agent_id IS DISTINCT FROM NEW.cursor_agent_id OR step.error_code IS DISTINCT FROM NEW.last_error_code THEN RAISE EXCEPTION 'correction plan update requires matching immutable step'; END IF;
  IF NOT ((step.operation='page' AND OLD.state='planning' AND NEW.state='planning' AND NEW.planned_count>OLD.planned_count AND NEW.planned_count<=OLD.planned_count+100) OR
   (step.operation='ready' AND OLD.state='planning' AND NEW.state='ready' AND NEW.planned_count=OLD.planned_count) OR
   (step.operation='invalidate' AND OLD.state<>'stale' AND NEW.state='stale' AND NEW.planned_count=OLD.planned_count) OR
   (step.operation='fail' AND OLD.state='planning' AND NEW.state='failed' AND NEW.planned_count=OLD.planned_count) OR
   (step.operation='retry' AND OLD.state='failed' AND NEW.state='planning' AND NEW.planned_count=OLD.planned_count)) THEN RAISE EXCEPTION 'correction plan transition invalid'; END IF;
  IF step.operation NOT IN('invalidate','fail') THEN
   PERFORM 1 FROM brands WHERE id=NEW.brand_id AND status<>'disabled' FOR SHARE NOWAIT;
   IF NOT FOUND THEN RAISE EXCEPTION 'correction plan brand disabled'; END IF;
   PERFORM 1 FROM brand_commission_payment_policies WHERE brand_id=NEW.brand_id AND enabled FOR SHARE NOWAIT;
   IF NOT FOUND OR NOT commission_payment_evidence_current(NEW.brand_id,NEW.cycle_id,NEW.run_id,NEW.evidence_epoch) OR NOT commission_correction_source_valid(NEW.brand_id,NEW.payment_id,NEW.run_id) THEN RAISE EXCEPTION 'correction plan evidence unavailable'; END IF;
  END IF;
  IF step.operation='invalidate' AND commission_payment_evidence_current(NEW.brand_id,NEW.cycle_id,NEW.run_id,NEW.evidence_epoch) THEN RAISE EXCEPTION 'cannot stale a current correction plan'; END IF;
  SELECT count(*) INTO n FROM commission_correction_plan_targets WHERE plan_id=NEW.id;
  IF n<>NEW.planned_count OR NEW.cursor_agent_id IS DISTINCT FROM (SELECT agent_id FROM commission_correction_plan_targets WHERE plan_id=NEW.id ORDER BY agent_id DESC LIMIT 1) THEN RAISE EXCEPTION 'correction plan count/cursor mismatch'; END IF;
  IF NEW.state='ready' THEN
   SELECT coalesce(sum(points_before::numeric),0),coalesce(sum(points_after::numeric),0),coalesce(sum(greatest(delta_points,0)::numeric),0),coalesce(sum(greatest(-delta_points,0)::numeric),0)
    INTO before_sum,after_sum,credits,debits FROM commission_correction_plan_targets WHERE plan_id=NEW.id;
   IF NEW.before_points<>before_sum OR NEW.calculated_points<>after_sum OR NEW.credit_points<>credits OR NEW.debit_points<>debits THEN RAISE EXCEPTION 'correction plan cannot be ready with incomplete differences'; END IF;
  END IF;
  NEW.updated_at:=clock_timestamp();
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_commission_correction_plan BEFORE INSERT OR UPDATE OR DELETE ON commission_correction_plans FOR EACH ROW EXECUTE FUNCTION guard_commission_correction_plan();
CREATE FUNCTION guard_commission_correction_plan_target() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p commission_correction_plans; candidate record; step commission_correction_plan_steps;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'correction plan targets immutable'; END IF;
 SELECT * INTO p FROM commission_correction_plans WHERE brand_id=NEW.brand_id AND id=NEW.plan_id FOR UPDATE NOWAIT;
 SELECT * INTO step FROM commission_correction_plan_steps WHERE plan_id=NEW.plan_id AND version=NEW.plan_version;
 IF p.state IS DISTINCT FROM 'planning' OR NEW.creation_xid<>pg_current_xact_id() OR NEW.plan_version<>p.version+1 OR
  NEW.agent_id<=p.cursor_agent_id OR step.operation IS DISTINCT FROM 'page' OR step.audit_log_id IS DISTINCT FROM NEW.creation_audit_log_id OR
  NOT commission_payment_evidence_current(p.brand_id,p.cycle_id,p.run_id,p.evidence_epoch) OR NOT commission_correction_source_valid(p.brand_id,p.payment_id,p.run_id)
  THEN RAISE EXCEPTION 'correction target requires live audited preparation'; END IF;
 SELECT * INTO candidate FROM commission_correction_candidates(p.brand_id,p.payment_id,p.run_id) WHERE agent_id=NEW.agent_id;
 IF NOT FOUND OR NEW.member_id<>candidate.member_id OR NEW.original_target_id IS DISTINCT FROM candidate.original_target_id OR NEW.earning_id IS DISTINCT FROM candidate.earning_id OR
  NEW.adjustment_version IS DISTINCT FROM candidate.adjustment_version OR NEW.points_before<>candidate.points_before OR NEW.points_after<>candidate.points_after OR NEW.delta_points<>candidate.delta_points THEN RAISE EXCEPTION 'correction target must match exact immutable financial sources'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_commission_correction_plan_target BEFORE INSERT OR UPDATE OR DELETE ON commission_correction_plan_targets FOR EACH ROW EXECUTE FUNCTION guard_commission_correction_plan_target();
CREATE FUNCTION require_commission_correction_plan_commit() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p commission_correction_plans;
BEGIN
 SELECT * INTO p FROM commission_correction_plans WHERE id=NEW.id;
 IF NOT EXISTS(SELECT 1 FROM commission_correction_plan_steps s WHERE s.plan_id=p.id AND s.version=1 AND s.operation='create' AND s.audit_log_id=p.creation_audit_log_id) OR
  NOT EXISTS(SELECT 1 FROM commission_correction_plan_steps s WHERE s.plan_id=p.id AND s.version=p.version AND s.audit_log_id=p.last_audit_log_id) OR
  p.planned_count<>(SELECT count(*) FROM commission_correction_plan_targets WHERE plan_id=p.id) THEN RAISE EXCEPTION 'orphan or partial correction plan cannot commit'; END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER commission_correction_plan_commit AFTER INSERT OR UPDATE ON commission_correction_plans DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_commission_correction_plan_commit();
CREATE FUNCTION require_commission_correction_plan_step_commit() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM commission_correction_plans p WHERE p.id=NEW.plan_id AND p.version>=NEW.version) THEN RAISE EXCEPTION 'orphan correction step'; END IF; RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER commission_correction_plan_step_commit AFTER INSERT ON commission_correction_plan_steps DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_commission_correction_plan_step_commit();
CREATE FUNCTION require_commission_correction_plan_target_commit() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM commission_correction_plans p WHERE p.id=NEW.plan_id AND p.version>=NEW.plan_version AND p.planned_count=(SELECT count(*) FROM commission_correction_plan_targets WHERE plan_id=p.id)) THEN RAISE EXCEPTION 'orphan correction target'; END IF; RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER commission_correction_plan_target_commit AFTER INSERT ON commission_correction_plan_targets DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_commission_correction_plan_target_commit();
-- Reserve correction financial labels: preparation cannot be used as a
-- ledger authorization. The execution phase must replace this with a guard
-- that binds a genuine, approved, current compensation target atomically.
CREATE FUNCTION guard_unexecuted_commission_correction_credit() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.entry_type IN('commission_correction','commission_correction_plan') OR
  NEW.reference_type IN('commission_correction_plan','commission_correction_plan_target','commission_correction_target') THEN
  RAISE EXCEPTION 'prepared correction plans cannot authorize financial ledger postings';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_unexecuted_commission_correction_credit BEFORE INSERT ON point_ledger_entries FOR EACH ROW EXECUTE FUNCTION guard_unexecuted_commission_correction_credit();
INSERT INTO permissions(key) VALUES('commission_correction.retry.brand') ON CONFLICT DO NOTHING;
INSERT INTO role_permissions(role_id,permission_key) SELECT id,'commission_correction.retry.brand' FROM roles WHERE is_bootstrap AND brand_id IS NOT NULL ON CONFLICT DO NOTHING;
DO $$
DECLARE app_schema text:=current_schema(); f record;
BEGIN
 FOR f IN SELECT p.oid::regprocedure signature FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname=app_schema AND p.proname IN(
 'commission_correction_candidates','commission_correction_source_valid','commission_correction_mode','guard_commission_correction_plan_step','guard_commission_correction_plan',
 'guard_commission_correction_plan_target','require_commission_correction_plan_commit','require_commission_correction_plan_step_commit','require_commission_correction_plan_target_commit','guard_unexecuted_commission_correction_credit') LOOP
  EXECUTE format('ALTER FUNCTION %s SET search_path TO pg_catalog, %I, pg_temp',f.signature,app_schema);
 END LOOP;
END $$;
