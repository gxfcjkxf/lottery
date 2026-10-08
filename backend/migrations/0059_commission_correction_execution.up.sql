-- Real difference execution, separate from the immutable preparation plan.
-- Old earnings, payment targets, approvals, manual adjustments and ledgers
-- remain intact; current awarded net is maintained by witnessed differences.
CREATE TABLE brand_commission_correction_policies (
 brand_id uuid PRIMARY KEY REFERENCES brands(id),version bigint NOT NULL DEFAULT 1 CHECK(version BETWEEN 1 AND 9007199254740991),
 enabled boolean NOT NULL DEFAULT false,audit_log_id uuid REFERENCES audit_logs(id),updated_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE commission_correction_policy_revisions (
 brand_id uuid NOT NULL,version bigint NOT NULL,enabled boolean NOT NULL,audit_log_id uuid REFERENCES audit_logs(id),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(brand_id,version),FOREIGN KEY(brand_id) REFERENCES brand_commission_correction_policies(brand_id)
);
CREATE FUNCTION guard_commission_correction_policy() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE a audit_logs;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'correction financial policy retained'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.version<>1 OR NEW.enabled OR NEW.audit_log_id IS NOT NULL THEN RAISE EXCEPTION 'correction rollout begins disabled'; END IF;
 ELSE
  SELECT * INTO a FROM audit_logs WHERE id=NEW.audit_log_id;
  IF NEW.brand_id<>OLD.brand_id OR NEW.version<>OLD.version+1 OR a.id IS NULL OR a.brand_id IS DISTINCT FROM NEW.brand_id OR
   a.action<>'commission.correction_policy.update' OR a.resource_type<>'commission_correction_policy' OR a.resource_id IS DISTINCT FROM NEW.brand_id OR a.actor_type<>'admin' OR
   NOT EXISTS(SELECT 1 FROM admin_accounts WHERE id=a.actor_id AND status='active' AND NOT is_super_admin) OR
   a.before_json IS DISTINCT FROM jsonb_build_object('version',OLD.version,'enabled',OLD.enabled) OR a.after_json IS DISTINCT FROM jsonb_build_object('version',NEW.version,'enabled',NEW.enabled) THEN RAISE EXCEPTION 'correction rollout update requires current ordinary admin audit'; END IF;
  NEW.updated_at:=clock_timestamp();
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_commission_correction_policy BEFORE INSERT OR UPDATE OR DELETE ON brand_commission_correction_policies FOR EACH ROW EXECUTE FUNCTION guard_commission_correction_policy();
CREATE FUNCTION capture_commission_correction_policy() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN INSERT INTO commission_correction_policy_revisions(brand_id,version,enabled,audit_log_id) VALUES(NEW.brand_id,NEW.version,NEW.enabled,NEW.audit_log_id); RETURN NULL; END $$;
CREATE TRIGGER captured_commission_correction_policy AFTER INSERT OR UPDATE ON brand_commission_correction_policies FOR EACH ROW EXECUTE FUNCTION capture_commission_correction_policy();
CREATE FUNCTION guard_commission_correction_policy_revision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP<>'INSERT' OR NOT EXISTS(SELECT 1 FROM brand_commission_correction_policies WHERE brand_id=NEW.brand_id AND version=NEW.version AND enabled=NEW.enabled AND audit_log_id IS NOT DISTINCT FROM NEW.audit_log_id) THEN RAISE EXCEPTION 'correction policy history immutable'; END IF; RETURN NEW;
END $$;
CREATE TRIGGER guarded_commission_correction_policy_revision BEFORE INSERT OR UPDATE OR DELETE ON commission_correction_policy_revisions FOR EACH ROW EXECUTE FUNCTION guard_commission_correction_policy_revision();
INSERT INTO brand_commission_correction_policies(brand_id) SELECT id FROM brands;
CREATE FUNCTION initialize_commission_correction_policy() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN INSERT INTO brand_commission_correction_policies(brand_id) VALUES(NEW.id); RETURN NULL; END $$;
CREATE TRIGGER commission_correction_policy_init AFTER INSERT ON brands FOR EACH ROW EXECUTE FUNCTION initialize_commission_correction_policy();
ALTER TABLE commission_correction_plans ADD UNIQUE(brand_id,cycle_id,id);
ALTER TABLE commission_correction_plan_targets ADD UNIQUE(brand_id,plan_id,id);
ALTER TABLE commission_correction_plan_targets ADD COLUMN previous_correction_target_id uuid;
ALTER TABLE commission_correction_plan_targets ADD COLUMN financial_version bigint CHECK(financial_version BETWEEN 1 AND 9007199254740991);
ALTER TABLE commission_correction_plan_targets DROP CONSTRAINT commission_correction_plan_targets_check1;
ALTER TABLE commission_correction_plan_targets ADD CHECK(original_target_id IS NOT NULL OR earning_id IS NOT NULL OR previous_correction_target_id IS NOT NULL);
ALTER TABLE commission_correction_plan_targets ADD CHECK((previous_correction_target_id IS NULL)=(financial_version IS NULL));
CREATE TABLE commission_correction_executions (
 id uuid PRIMARY KEY,brand_id uuid NOT NULL,cycle_id uuid NOT NULL,plan_id uuid NOT NULL,run_id uuid NOT NULL,
 plan_version bigint NOT NULL CHECK(plan_version BETWEEN 1 AND 9007199254740991),evidence_epoch bigint NOT NULL CHECK(evidence_epoch>=0),
 payout_mode text NOT NULL CHECK(payout_mode IN('manual','automatic','mixed','none')),
 state text NOT NULL CHECK(state IN('awaiting_approval','applying','completed','paused','failed','stale')),
 version bigint NOT NULL DEFAULT 1 CHECK(version BETWEEN 1 AND 9007199254740991),
 credit_points numeric NOT NULL CHECK(credit_points>=0 AND scale(credit_points)=0),debit_points numeric NOT NULL CHECK(debit_points>=0 AND scale(debit_points)=0),
 net_points numeric NOT NULL CHECK(scale(net_points)=0 AND net_points=credit_points-debit_points),target_count bigint NOT NULL CHECK(target_count BETWEEN 0 AND 100000),
 approved_by uuid REFERENCES admin_accounts(id),approval_actor_type text CHECK(approval_actor_type IN('admin','system')),approval_audit_log_id uuid REFERENCES audit_logs(id),
 paused_plan_target_id uuid REFERENCES commission_correction_plan_targets(id),last_error_code text,
 creation_audit_log_id uuid NOT NULL REFERENCES audit_logs(id),last_audit_log_id uuid NOT NULL REFERENCES audit_logs(id),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),next_work_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 creation_xid xid8 NOT NULL DEFAULT pg_current_xact_id(),
 UNIQUE(brand_id,id),UNIQUE(plan_id),UNIQUE(brand_id,cycle_id,id),
 FOREIGN KEY(brand_id,cycle_id,plan_id) REFERENCES commission_correction_plans(brand_id,cycle_id,id),
 CHECK((approval_actor_type IS NULL)=(approval_audit_log_id IS NULL)),CHECK((approval_actor_type='admin') IS NOT DISTINCT FROM (approved_by IS NOT NULL) OR approval_actor_type IS NULL AND approved_by IS NULL),
 CHECK(state NOT IN('applying','completed','paused','failed') OR approval_audit_log_id IS NOT NULL),
 CHECK((state IN('paused','failed'))=(last_error_code IS NOT NULL)),
 CHECK((last_error_code='COMMISSION_CORRECTION_AVAILABLE_INSUFFICIENT') IS NOT DISTINCT FROM (paused_plan_target_id IS NOT NULL) OR last_error_code IS NULL AND paused_plan_target_id IS NULL)
);
CREATE UNIQUE INDEX commission_correction_execution_live_cycle ON commission_correction_executions(cycle_id) WHERE state<>'stale';
CREATE INDEX commission_correction_execution_due ON commission_correction_executions(next_work_at,id) WHERE state='applying';
CREATE TABLE commission_correction_execution_targets (
 id uuid PRIMARY KEY,brand_id uuid NOT NULL,cycle_id uuid NOT NULL,execution_id uuid NOT NULL,plan_target_id uuid NOT NULL,agent_id uuid NOT NULL,member_id uuid NOT NULL,
 points_before bigint NOT NULL CHECK(points_before>=0),points_after bigint NOT NULL CHECK(points_after>=0),delta_points bigint NOT NULL,
 base_financial_version bigint CHECK(base_financial_version BETWEEN 1 AND 9007199254740991),financial_version bigint CHECK(financial_version BETWEEN 1 AND 9007199254740991),
 execution_version bigint NOT NULL CHECK(execution_version BETWEEN 1 AND 9007199254740991),state text NOT NULL DEFAULT 'pending' CHECK(state IN('pending','applied')),
 ledger_entry_id uuid REFERENCES point_ledger_entries(id),audit_log_id uuid REFERENCES audit_logs(id),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),applied_at timestamptz,creation_xid xid8 NOT NULL DEFAULT pg_current_xact_id(),
 UNIQUE(brand_id,id),UNIQUE(execution_id,plan_target_id),UNIQUE(ledger_entry_id),UNIQUE(brand_id,cycle_id,agent_id,financial_version),
 FOREIGN KEY(brand_id,cycle_id,execution_id) REFERENCES commission_correction_executions(brand_id,cycle_id,id),
 FOREIGN KEY(brand_id,member_id) REFERENCES brand_members(brand_id,id),
 CHECK(points_after::numeric-points_before::numeric=delta_points::numeric),
 CHECK((state='applied')=(applied_at IS NOT NULL)),CHECK((state='applied')=(audit_log_id IS NOT NULL)),CHECK((state='applied')=(financial_version IS NOT NULL)),
 CHECK(state<>'pending' OR ledger_entry_id IS NULL),CHECK(state<>'applied' OR (delta_points=0)=(ledger_entry_id IS NULL))
);
ALTER TABLE commission_correction_plan_targets ADD FOREIGN KEY(brand_id,previous_correction_target_id) REFERENCES commission_correction_execution_targets(brand_id,id);
CREATE TABLE commission_correction_balance_heads (
 brand_id uuid NOT NULL,cycle_id uuid NOT NULL,agent_id uuid NOT NULL,member_id uuid NOT NULL,
 version bigint NOT NULL CHECK(version BETWEEN 1 AND 9007199254740991),points bigint NOT NULL CHECK(points>=0),
 original_target_id uuid,last_execution_target_id uuid NOT NULL,
 PRIMARY KEY(cycle_id,agent_id),FOREIGN KEY(brand_id,cycle_id) REFERENCES commission_cycles(brand_id,id),
 FOREIGN KEY(brand_id,member_id) REFERENCES brand_members(brand_id,id),
 FOREIGN KEY(brand_id,original_target_id) REFERENCES commission_payment_targets(brand_id,id),
 FOREIGN KEY(brand_id,last_execution_target_id) REFERENCES commission_correction_execution_targets(brand_id,id)
);
CREATE TABLE commission_correction_cycle_holds (
 cycle_id uuid PRIMARY KEY,brand_id uuid NOT NULL,version bigint NOT NULL DEFAULT 1 CHECK(version BETWEEN 1 AND 9007199254740991),
 active boolean NOT NULL DEFAULT false,blocked_execution_id uuid REFERENCES commission_correction_executions(id),audit_log_id uuid REFERENCES audit_logs(id),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),FOREIGN KEY(brand_id,cycle_id) REFERENCES commission_cycles(brand_id,id),
 CHECK(NOT active OR blocked_execution_id IS NOT NULL AND audit_log_id IS NOT NULL)
);
CREATE TABLE commission_correction_execution_steps (
 id uuid PRIMARY KEY,brand_id uuid NOT NULL,execution_id uuid NOT NULL,version bigint NOT NULL CHECK(version BETWEEN 1 AND 9007199254740991),
 from_state text,to_state text NOT NULL,operation text NOT NULL CHECK(operation IN('create','approve','apply','complete','pause','continue','retry','fail','invalidate')),
 target_id uuid REFERENCES commission_correction_execution_targets(id) DEFERRABLE INITIALLY DEFERRED,
 paused_plan_target_id uuid REFERENCES commission_correction_plan_targets(id),error_code text,
 actor_type text NOT NULL CHECK(actor_type IN('admin','system')),actor_id uuid REFERENCES admin_accounts(id),
 reason text NOT NULL CHECK(octet_length(reason) BETWEEN 1 AND 500),request_id text NOT NULL,
 audit_log_id uuid NOT NULL REFERENCES audit_logs(id),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(execution_id,version),UNIQUE(audit_log_id),FOREIGN KEY(brand_id,execution_id) REFERENCES commission_correction_executions(brand_id,id),
 CHECK((actor_type='admin')=(actor_id IS NOT NULL)),CHECK((version=1)=(from_state IS NULL)),CHECK((operation='apply')=(target_id IS NOT NULL))
);

-- Preserve the original 0058 function signature/OID for old callers. New
-- planning additionally freezes the last REAL executed target/version.
CREATE FUNCTION commission_correction_candidates_v2(b uuid,pid uuid,rid uuid)
RETURNS TABLE(agent_id uuid,member_id uuid,original_target_id uuid,earning_id uuid,adjustment_version bigint,points_before bigint,points_after bigint,delta_points bigint,previous_correction_target_id uuid,financial_version bigint)
LANGUAGE sql STABLE AS $$
 WITH original AS (
  SELECT t.agent_id,t.member_id,t.id,h.version,h.points FROM commission_payment_targets t
  JOIN commission_adjustment_heads h ON h.brand_id=t.brand_id AND h.target_id=t.id WHERE t.brand_id=b AND t.payment_id=pid AND t.state='paid'
 ), actual AS (
  SELECT h.* FROM commission_correction_balance_heads h JOIN commission_payments p ON p.brand_id=h.brand_id AND p.cycle_id=h.cycle_id WHERE p.brand_id=b AND p.id=pid
 ), basis AS (
  SELECT coalesce(h.agent_id,o.agent_id) agent_id,coalesce(h.member_id,o.member_id) member_id,coalesce(h.original_target_id,o.id) original_target_id,
   o.version adjustment_version,coalesce(h.points,o.points) points,h.last_execution_target_id,h.version financial_version
  FROM original o FULL JOIN actual h ON h.agent_id=o.agent_id
 ), corrected AS (SELECT * FROM commission_earnings WHERE brand_id=b AND run_id=rid)
 SELECT coalesce(o.agent_id,e.agent_id),coalesce(o.member_id,e.member_id),o.original_target_id,e.id,o.adjustment_version,
  coalesce(o.points,0),coalesce(e.points,0),coalesce(e.points,0)-coalesce(o.points,0),o.last_execution_target_id,o.financial_version
 FROM basis o FULL JOIN corrected e ON e.agent_id=o.agent_id
$$;
CREATE OR REPLACE FUNCTION commission_correction_candidates(b uuid,pid uuid,rid uuid)
RETURNS TABLE(agent_id uuid,member_id uuid,original_target_id uuid,earning_id uuid,adjustment_version bigint,points_before bigint,points_after bigint,delta_points bigint)
LANGUAGE sql STABLE AS $$ SELECT agent_id,member_id,original_target_id,earning_id,adjustment_version,points_before,points_after,delta_points FROM commission_correction_candidates_v2(b,pid,rid) $$;
CREATE FUNCTION commission_correction_heads_valid(b uuid,cid uuid) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT NOT EXISTS(SELECT 1 FROM commission_correction_balance_heads h
  LEFT JOIN commission_correction_execution_targets t ON t.brand_id=h.brand_id AND t.id=h.last_execution_target_id
  LEFT JOIN audit_logs a ON a.id=t.audit_log_id LEFT JOIN point_ledger_entries l ON l.id=t.ledger_entry_id
  WHERE h.brand_id=b AND h.cycle_id=cid AND (t.id IS NULL OR t.state<>'applied' OR t.cycle_id<>cid OR t.agent_id<>h.agent_id OR t.member_id<>h.member_id OR
   t.points_after<>h.points OR t.financial_version<>h.version OR a.id IS NULL OR a.brand_id IS DISTINCT FROM b OR a.actor_type<>'system' OR a.actor_id IS NOT NULL OR
   a.action<>'commission.correction_execution.target' OR a.resource_type<>'commission_correction_target' OR a.resource_id IS DISTINCT FROM t.id OR
   a.before_json IS DISTINCT FROM jsonb_build_object('state','pending') OR
   a.after_json IS DISTINCT FROM jsonb_build_object('state','applied','execution_id',t.execution_id,'plan_target_id',t.plan_target_id,
    'points_before',t.points_before::text,'points_after',t.points_after::text,'delta_points',t.delta_points::text,'ledger_entry_id',t.ledger_entry_id,'financial_version',t.financial_version) OR
   t.delta_points<>0 AND (l.id IS NULL OR l.brand_id<>b OR l.member_id<>h.member_id OR l.entry_type<>'commission_correction' OR l.reference_type<>'commission_correction_target' OR l.reference_id IS DISTINCT FROM t.id OR
    l.operation_key<>'commission-correction:'||t.id::text OR l.request_id<>a.request_id OR l.actor_type<>'system' OR l.actor_id IS NOT NULL OR l.reversal_of IS NOT NULL OR
    l.delta_snapshot IS DISTINCT FROM jsonb_set(point_zero_snapshot(),'{commission,available}',to_jsonb(t.delta_points::text)) OR
    l.source_allocation IS DISTINCT FROM jsonb_build_array(jsonb_build_object('source','commission','state','available','points',abs(t.delta_points)::text))) OR t.delta_points=0 AND t.ledger_entry_id IS NOT NULL))
$$;
-- Extend the existing source predicate without replacing its signature/OID.
-- Preparation may still record an unresolved original manual adjustment;
-- malformed actual correction witnesses never become a ready financial basis.
CREATE OR REPLACE FUNCTION commission_correction_source_valid(b uuid,pid uuid,rid uuid) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT EXISTS(SELECT 1 FROM commission_payments p JOIN commission_runs r ON r.brand_id=p.brand_id AND r.cycle_id=p.cycle_id AND r.id=rid
  WHERE p.brand_id=b AND p.id=pid AND p.run_id<>rid AND p.state='blocked' AND p.last_error_code='COMMISSION_PAYMENT_CORRECTION_REQUIRED'
  AND commission_correction_heads_valid(b,p.cycle_id)
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
 AND NOT EXISTS(SELECT 1 FROM commission_correction_balance_heads h
  JOIN commission_payments p ON p.brand_id=h.brand_id AND p.cycle_id=h.cycle_id AND p.id=pid
  LEFT JOIN commission_payment_targets t ON t.brand_id=b AND t.id=h.original_target_id
  LEFT JOIN commission_earnings e ON e.brand_id=b AND e.run_id=rid AND e.agent_id=h.agent_id
  WHERE h.brand_id=b AND (h.original_target_id IS NOT NULL AND (t.id IS NULL OR t.payment_id<>pid OR t.agent_id<>h.agent_id OR t.member_id<>h.member_id)
   OR e.id IS NOT NULL AND e.member_id<>h.member_id))
$$;
CREATE FUNCTION commission_correction_execution_current(b uuid,xid uuid) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT EXISTS(SELECT 1 FROM commission_correction_executions x JOIN commission_correction_plans p ON p.brand_id=x.brand_id AND p.id=x.plan_id
  WHERE x.brand_id=b AND x.id=xid AND p.state='ready' AND p.version=x.plan_version AND p.run_id=x.run_id AND p.evidence_epoch=x.evidence_epoch
   AND commission_payment_evidence_current(b,x.cycle_id,x.run_id,x.evidence_epoch))
$$;

CREATE OR REPLACE FUNCTION guard_commission_correction_plan_target() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p commission_correction_plans; candidate record; step commission_correction_plan_steps;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'correction plan targets immutable'; END IF;
 SELECT * INTO p FROM commission_correction_plans WHERE brand_id=NEW.brand_id AND id=NEW.plan_id FOR UPDATE NOWAIT;
 SELECT * INTO step FROM commission_correction_plan_steps WHERE plan_id=NEW.plan_id AND version=NEW.plan_version;
 IF p.state IS DISTINCT FROM 'planning' OR NEW.creation_xid<>pg_current_xact_id() OR NEW.plan_version<>p.version+1 OR NEW.agent_id<=p.cursor_agent_id OR
  step.operation IS DISTINCT FROM 'page' OR step.audit_log_id IS DISTINCT FROM NEW.creation_audit_log_id OR
  NOT commission_payment_evidence_current(p.brand_id,p.cycle_id,p.run_id,p.evidence_epoch) OR NOT commission_correction_source_valid(p.brand_id,p.payment_id,p.run_id) OR
  NOT commission_correction_heads_valid(p.brand_id,p.cycle_id) THEN RAISE EXCEPTION 'correction target requires live audited preparation'; END IF;
 SELECT * INTO candidate FROM commission_correction_candidates_v2(p.brand_id,p.payment_id,p.run_id) WHERE agent_id=NEW.agent_id;
 IF NOT FOUND OR NEW.member_id<>candidate.member_id OR NEW.original_target_id IS DISTINCT FROM candidate.original_target_id OR NEW.earning_id IS DISTINCT FROM candidate.earning_id OR
  NEW.adjustment_version IS DISTINCT FROM candidate.adjustment_version OR NEW.points_before<>candidate.points_before OR NEW.points_after<>candidate.points_after OR NEW.delta_points<>candidate.delta_points OR
  NEW.previous_correction_target_id IS DISTINCT FROM candidate.previous_correction_target_id OR NEW.financial_version IS DISTINCT FROM candidate.financial_version THEN RAISE EXCEPTION 'correction target must match actual financial net and calculation'; END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION guard_commission_correction_execution_step() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE x commission_correction_executions; a audit_logs;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'correction execution steps immutable'; END IF;
 SELECT * INTO x FROM commission_correction_executions WHERE brand_id=NEW.brand_id AND id=NEW.execution_id FOR UPDATE NOWAIT;
 SELECT * INTO a FROM audit_logs WHERE id=NEW.audit_log_id;
 IF x.id IS NULL OR NEW.version<>(CASE WHEN NEW.operation='create' THEN x.version ELSE x.version+1 END) OR
  NEW.operation='create' AND (x.version<>1 OR x.creation_xid<>pg_current_xact_id() OR NEW.to_state<>x.state OR NEW.paused_plan_target_id IS DISTINCT FROM x.paused_plan_target_id OR NEW.error_code IS DISTINCT FROM x.last_error_code) OR
  NEW.operation<>'create' AND NEW.from_state IS DISTINCT FROM x.state OR a.id IS NULL OR a.brand_id IS DISTINCT FROM NEW.brand_id OR
  a.action<>'commission.correction_execution.'||NEW.operation OR a.resource_type<>'commission_correction_execution' OR a.resource_id IS DISTINCT FROM NEW.execution_id OR
  a.actor_type<>NEW.actor_type OR a.actor_id IS DISTINCT FROM NEW.actor_id OR a.request_id<>NEW.request_id OR a.reason<>NEW.reason OR
  a.after_json IS DISTINCT FROM jsonb_build_object('version',NEW.version,'state',NEW.to_state,'target_id',NEW.target_id,'paused_plan_target_id',NEW.paused_plan_target_id,'error_code',NEW.error_code) OR
  a.before_json IS DISTINCT FROM (CASE WHEN NEW.operation='create' THEN 'null'::jsonb ELSE jsonb_build_object('version',x.version,'state',x.state) END) THEN RAISE EXCEPTION 'correction execution step/audit mismatch'; END IF;
 IF NEW.operation IN('approve','continue','retry') THEN
  IF NEW.actor_type<>'admin' OR NOT EXISTS(SELECT 1 FROM admin_accounts WHERE id=NEW.actor_id AND status='active' AND NOT is_super_admin) THEN RAISE EXCEPTION 'correction action requires current ordinary admin'; END IF;
 ELSIF NEW.actor_type<>'system' OR NEW.actor_id IS NOT NULL THEN RAISE EXCEPTION 'correction financial work requires system actor'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_commission_correction_execution_step BEFORE INSERT OR UPDATE OR DELETE ON commission_correction_execution_steps FOR EACH ROW EXECUTE FUNCTION guard_commission_correction_execution_step();
CREATE FUNCTION guard_commission_correction_cycle_hold() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE s commission_correction_execution_steps; x commission_correction_executions;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'correction cycle hold history retained'; END IF;
 PERFORM 1 FROM commission_cycles WHERE brand_id=NEW.brand_id AND id=NEW.cycle_id FOR UPDATE NOWAIT;
 IF NOT FOUND THEN RAISE EXCEPTION 'correction hold cycle mismatch'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.version<>1 OR NEW.active OR NEW.blocked_execution_id IS NOT NULL OR NEW.audit_log_id IS NOT NULL THEN RAISE EXCEPTION 'correction hold begins released without fabricated event'; END IF;
 ELSE
  SELECT * INTO s FROM commission_correction_execution_steps WHERE brand_id=NEW.brand_id AND audit_log_id=NEW.audit_log_id;
  SELECT * INTO x FROM commission_correction_executions WHERE brand_id=NEW.brand_id AND id=s.execution_id;
  IF NEW.brand_id<>OLD.brand_id OR NEW.cycle_id<>OLD.cycle_id OR NEW.version<>OLD.version+1 OR s.id IS NULL OR x.cycle_id<>NEW.cycle_id THEN RAISE EXCEPTION 'hold requires matching cycle action'; END IF;
  IF NEW.active THEN
   IF OLD.active OR s.operation<>'pause' OR s.actor_type<>'system' OR s.error_code<>'COMMISSION_CORRECTION_AVAILABLE_INSUFFICIENT' OR NEW.blocked_execution_id IS DISTINCT FROM x.id THEN RAISE EXCEPTION 'cycle pause requires real insufficient attempt'; END IF;
  ELSE
   IF NOT OLD.active OR s.operation<>'continue' OR s.actor_type<>'admin' OR NEW.blocked_execution_id IS DISTINCT FROM OLD.blocked_execution_id THEN RAISE EXCEPTION 'cycle hold needs explicit administrator release'; END IF;
  END IF;
  NEW.updated_at:=clock_timestamp();
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_commission_correction_cycle_hold BEFORE INSERT OR UPDATE OR DELETE ON commission_correction_cycle_holds FOR EACH ROW EXECUTE FUNCTION guard_commission_correction_cycle_hold();
CREATE FUNCTION guard_commission_correction_execution() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p commission_correction_plans; s commission_correction_execution_steps; a audit_logs; held boolean; expected text; t commission_correction_plan_targets; available bigint;
 n bigint; credit numeric; debit numeric;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'correction executions retained'; END IF;
 IF TG_OP='UPDATE' AND NEW.version=OLD.version AND (to_jsonb(NEW)-'next_work_at')=(to_jsonb(OLD)-'next_work_at') THEN RETURN NEW; END IF;
 PERFORM 1 FROM commission_cycles WHERE brand_id=NEW.brand_id AND id=NEW.cycle_id FOR UPDATE NOWAIT;
 SELECT * INTO p FROM commission_correction_plans WHERE brand_id=NEW.brand_id AND id=NEW.plan_id FOR SHARE NOWAIT;
 SELECT coalesce((SELECT active FROM commission_correction_cycle_holds WHERE brand_id=NEW.brand_id AND cycle_id=NEW.cycle_id),false) INTO held;
 IF TG_OP='INSERT' THEN
  PERFORM 1 FROM brands WHERE id=NEW.brand_id AND status<>'disabled' FOR SHARE NOWAIT;
  IF NOT FOUND THEN RAISE EXCEPTION 'correction execution brand disabled'; END IF;
  PERFORM 1 FROM brand_commission_payment_policies WHERE brand_id=NEW.brand_id AND enabled FOR SHARE NOWAIT;
  IF NOT FOUND OR p.id IS NULL OR p.state<>'ready' OR p.version<>NEW.plan_version OR p.cycle_id<>NEW.cycle_id OR p.run_id<>NEW.run_id OR p.evidence_epoch<>NEW.evidence_epoch OR
   NOT commission_payment_evidence_current(NEW.brand_id,NEW.cycle_id,NEW.run_id,NEW.evidence_epoch) OR NOT commission_correction_source_valid(NEW.brand_id,p.payment_id,NEW.run_id) OR NOT commission_correction_heads_valid(NEW.brand_id,NEW.cycle_id) THEN RAISE EXCEPTION 'execution requires complete current plan'; END IF;
  expected:=(CASE WHEN p.payout_mode IN('manual','mixed') THEN 'awaiting_approval' WHEN held THEN 'paused' ELSE 'applying' END);
  SELECT * INTO a FROM audit_logs WHERE id=NEW.creation_audit_log_id;
  IF NEW.version<>1 OR NEW.creation_xid<>pg_current_xact_id() OR NEW.state<>expected OR NEW.payout_mode<>p.payout_mode OR
   NEW.credit_points<>p.credit_points OR NEW.debit_points<>p.debit_points OR NEW.net_points<>p.net_points OR NEW.target_count<>p.target_count OR
   NEW.last_audit_log_id<>NEW.creation_audit_log_id OR NEW.paused_plan_target_id IS NOT NULL OR a.id IS NULL OR a.brand_id IS DISTINCT FROM NEW.brand_id OR
   a.actor_type<>'system' OR a.actor_id IS NOT NULL OR a.action<>'commission.correction_execution.create' OR a.resource_type<>'commission_correction_execution' OR a.resource_id IS DISTINCT FROM NEW.id
   THEN RAISE EXCEPTION 'execution identity/mode/amount/creation mismatch'; END IF;
  PERFORM 1 FROM brand_commission_correction_policies WHERE brand_id=NEW.brand_id AND enabled FOR SHARE NOWAIT;
  IF NOT FOUND THEN RAISE EXCEPTION 'correction financial rollout disabled'; END IF;
  IF expected='awaiting_approval' THEN
   IF NEW.approval_actor_type IS NOT NULL OR NEW.approval_audit_log_id IS NOT NULL OR NEW.approved_by IS NOT NULL THEN RAISE EXCEPTION 'manual correction requires new approval'; END IF;
  ELSIF NEW.approval_actor_type IS DISTINCT FROM 'system' OR NEW.approval_audit_log_id IS DISTINCT FROM NEW.creation_audit_log_id OR NEW.approved_by IS NOT NULL OR
   (expected='paused' AND NEW.last_error_code IS DISTINCT FROM 'COMMISSION_CORRECTION_CYCLE_HELD') THEN RAISE EXCEPTION 'automatic correction authorization mismatch'; END IF;
 ELSE
  IF (to_jsonb(NEW)-ARRAY['state','version','last_audit_log_id','last_error_code','paused_plan_target_id','updated_at','next_work_at','approval_actor_type','approved_by','approval_audit_log_id']) IS DISTINCT FROM
   (to_jsonb(OLD)-ARRAY['state','version','last_audit_log_id','last_error_code','paused_plan_target_id','updated_at','next_work_at','approval_actor_type','approved_by','approval_audit_log_id']) OR NEW.version<>OLD.version+1 THEN RAISE EXCEPTION 'correction execution immutable identity/amount'; END IF;
  SELECT * INTO s FROM commission_correction_execution_steps WHERE brand_id=NEW.brand_id AND execution_id=NEW.id AND version=NEW.version;
  IF s.id IS NULL OR s.from_state IS DISTINCT FROM OLD.state OR s.to_state<>NEW.state OR s.audit_log_id<>NEW.last_audit_log_id OR s.error_code IS DISTINCT FROM NEW.last_error_code OR
   s.paused_plan_target_id IS DISTINCT FROM NEW.paused_plan_target_id THEN RAISE EXCEPTION 'execution transition requires matching immutable step'; END IF;
  IF NOT ((s.operation='approve' AND OLD.state='awaiting_approval' AND NEW.state IN('applying','paused')) OR
   (s.operation='apply' AND OLD.state='applying' AND NEW.state='applying') OR (s.operation='complete' AND OLD.state='applying' AND NEW.state='completed') OR
   (s.operation='pause' AND OLD.state='applying' AND NEW.state='paused') OR (s.operation='continue' AND OLD.state='paused' AND NEW.state='applying') OR
   (s.operation='retry' AND OLD.state='failed' AND NEW.state='applying') OR (s.operation='fail' AND OLD.state='applying' AND NEW.state='failed') OR
   (s.operation='invalidate' AND OLD.state<>'stale' AND NEW.state='stale')) THEN RAISE EXCEPTION 'correction execution state transition invalid'; END IF;
  IF s.operation='approve' THEN
   IF OLD.approval_audit_log_id IS NOT NULL OR NEW.approval_actor_type IS DISTINCT FROM 'admin' OR NEW.approved_by IS DISTINCT FROM s.actor_id OR NEW.approval_audit_log_id IS DISTINCT FROM s.audit_log_id THEN RAISE EXCEPTION 'new exact-plan approval required'; END IF;
  ELSIF (NEW.approval_actor_type,NEW.approved_by,NEW.approval_audit_log_id) IS DISTINCT FROM (OLD.approval_actor_type,OLD.approved_by,OLD.approval_audit_log_id) THEN RAISE EXCEPTION 'old approval immutable'; END IF;
  IF s.operation NOT IN('invalidate','fail') THEN
   PERFORM 1 FROM brands WHERE id=NEW.brand_id AND status<>'disabled' FOR SHARE NOWAIT;
   IF NOT FOUND THEN RAISE EXCEPTION 'execution brand disabled'; END IF;
   PERFORM 1 FROM brand_commission_payment_policies WHERE brand_id=NEW.brand_id AND enabled FOR SHARE NOWAIT;
   IF NOT FOUND OR NOT commission_correction_execution_current(NEW.brand_id,NEW.id) OR NOT commission_correction_source_valid(NEW.brand_id,p.payment_id,NEW.run_id) OR NOT commission_correction_heads_valid(NEW.brand_id,NEW.cycle_id) THEN RAISE EXCEPTION 'execution evidence or financial sources unavailable'; END IF;
   PERFORM 1 FROM brand_commission_correction_policies WHERE brand_id=NEW.brand_id AND enabled FOR SHARE NOWAIT;
   IF NOT FOUND THEN RAISE EXCEPTION 'correction financial rollout disabled'; END IF;
  END IF;
  IF s.operation='invalidate' AND commission_correction_execution_current(NEW.brand_id,NEW.id) THEN RAISE EXCEPTION 'cannot stale current correction execution'; END IF;
  IF NEW.state='applying' AND held THEN RAISE EXCEPTION 'cycle hold forbids new compensation'; END IF;
  IF s.operation='pause' THEN
   SELECT * INTO t FROM commission_correction_plan_targets WHERE brand_id=NEW.brand_id AND plan_id=NEW.plan_id AND id=NEW.paused_plan_target_id;
   SELECT pb.points INTO available FROM point_buckets pb JOIN point_accounts pa ON pa.brand_id=pb.brand_id AND pa.id=pb.account_id WHERE pb.brand_id=NEW.brand_id AND pa.brand_member_id=t.member_id AND pb.source='commission' AND pb.state='available';
   IF t.id IS NULL OR t.delta_points>=0 OR available IS NULL OR available>=-t.delta_points OR NOT held OR NEW.last_error_code IS DISTINCT FROM 'COMMISSION_CORRECTION_AVAILABLE_INSUFFICIENT' OR
    EXISTS(SELECT 1 FROM commission_correction_execution_targets done WHERE done.execution_id=NEW.id AND done.plan_target_id=t.id)
    THEN RAISE EXCEPTION 'pause must witness real C-available shortage'; END IF;
  ELSIF NEW.state='paused' AND (NOT held OR NEW.last_error_code IS DISTINCT FROM 'COMMISSION_CORRECTION_CYCLE_HELD') THEN RAISE EXCEPTION 'inherited cycle pause mismatch'; END IF;
  IF s.operation='apply' AND NOT EXISTS(SELECT 1 FROM commission_correction_execution_targets WHERE brand_id=NEW.brand_id AND execution_id=NEW.id AND id=s.target_id AND state='applied' AND execution_version=OLD.version) THEN RAISE EXCEPTION 'apply step requires completed actual target'; END IF;
  SELECT count(*),coalesce(sum(greatest(delta_points,0)::numeric),0),coalesce(sum(greatest(-delta_points,0)::numeric),0) INTO n,credit,debit FROM commission_correction_execution_targets WHERE execution_id=NEW.id AND state='applied';
  IF n>NEW.target_count OR credit>NEW.credit_points OR debit>NEW.debit_points OR NEW.state='completed' AND (n<>NEW.target_count OR credit<>NEW.credit_points OR debit<>NEW.debit_points) THEN RAISE EXCEPTION 'correction completion/amount incomplete'; END IF;
  NEW.updated_at:=clock_timestamp();
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_commission_correction_execution BEFORE INSERT OR UPDATE OR DELETE ON commission_correction_executions FOR EACH ROW EXECUTE FUNCTION guard_commission_correction_execution();
CREATE FUNCTION guard_commission_correction_execution_target() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE x commission_correction_executions; p commission_correction_plans; t commission_correction_plan_targets; h commission_correction_balance_heads; s commission_correction_execution_steps; a audit_logs; base bigint;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'correction financial targets retained'; END IF;
 SELECT * INTO x FROM commission_correction_executions WHERE brand_id=NEW.brand_id AND id=NEW.execution_id FOR UPDATE NOWAIT;
 SELECT * INTO p FROM commission_correction_plans WHERE brand_id=NEW.brand_id AND id=x.plan_id;
 SELECT * INTO t FROM commission_correction_plan_targets WHERE brand_id=NEW.brand_id AND plan_id=x.plan_id AND id=NEW.plan_target_id;
 SELECT * INTO h FROM commission_correction_balance_heads WHERE brand_id=NEW.brand_id AND cycle_id=x.cycle_id AND agent_id=NEW.agent_id FOR UPDATE NOWAIT;
 SELECT * INTO s FROM commission_correction_execution_steps WHERE execution_id=x.id AND version=x.version+1;
 IF x.state IS DISTINCT FROM 'applying' OR x.approval_audit_log_id IS NULL OR t.id IS NULL OR p.state<>'ready' OR NEW.cycle_id<>x.cycle_id OR NEW.member_id<>t.member_id OR NEW.agent_id<>t.agent_id OR
  NEW.points_before<>t.points_before OR NEW.points_after<>t.points_after OR NEW.delta_points<>t.delta_points OR NEW.base_financial_version IS DISTINCT FROM t.financial_version OR
  NEW.execution_version<>x.version OR h.version IS DISTINCT FROM t.financial_version OR h.last_execution_target_id IS DISTINCT FROM t.previous_correction_target_id OR
  h.agent_id IS NOT NULL AND (h.member_id<>NEW.member_id OR h.points<>NEW.points_before) OR s.operation IS DISTINCT FROM 'apply' OR s.target_id IS DISTINCT FROM NEW.id OR
  NOT commission_correction_execution_current(NEW.brand_id,NEW.execution_id) OR NOT commission_correction_source_valid(NEW.brand_id,p.payment_id,x.run_id) OR NOT commission_correction_heads_valid(NEW.brand_id,x.cycle_id) OR
  NOT EXISTS(SELECT 1 FROM commission_correction_cycle_holds WHERE brand_id=NEW.brand_id AND cycle_id=x.cycle_id AND NOT active) THEN RAISE EXCEPTION 'correction target requires exact authorized unprocessed net'; END IF;
 PERFORM 1 FROM brands WHERE id=NEW.brand_id AND status<>'disabled' FOR SHARE NOWAIT;
 IF NOT FOUND THEN RAISE EXCEPTION 'correction target brand disabled'; END IF;
 PERFORM 1 FROM brand_commission_correction_policies WHERE brand_id=NEW.brand_id AND enabled FOR SHARE NOWAIT;
 IF NOT FOUND THEN RAISE EXCEPTION 'correction target rollout disabled'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.state<>'pending' OR NEW.ledger_entry_id IS NOT NULL OR NEW.audit_log_id IS NOT NULL OR NEW.financial_version IS NOT NULL OR NEW.creation_xid<>pg_current_xact_id() THEN RAISE EXCEPTION 'correction target starts pending in its financial transaction'; END IF;
 ELSE
  IF OLD.state<>'pending' OR NEW.state<>'applied' OR OLD.creation_xid<>pg_current_xact_id() OR
   (to_jsonb(NEW)-ARRAY['state','ledger_entry_id','audit_log_id','financial_version','applied_at']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['state','ledger_entry_id','audit_log_id','financial_version','applied_at']) OR
   NEW.financial_version IS DISTINCT FROM coalesce(NEW.base_financial_version,0)+1 THEN RAISE EXCEPTION 'correction target completion must bind immutable exact receipt once'; END IF;
  SELECT * INTO a FROM audit_logs WHERE id=NEW.audit_log_id;
  IF a.id IS NULL OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.actor_type<>'system' OR a.actor_id IS NOT NULL OR a.action<>'commission.correction_execution.target' OR a.resource_type<>'commission_correction_target' OR
   a.resource_id IS DISTINCT FROM NEW.id OR a.request_id<>s.request_id OR a.before_json IS DISTINCT FROM jsonb_build_object('state','pending') OR
   a.after_json IS DISTINCT FROM jsonb_build_object('state','applied','execution_id',NEW.execution_id,'plan_target_id',NEW.plan_target_id,'points_before',NEW.points_before::text,'points_after',NEW.points_after::text,
    'delta_points',NEW.delta_points::text,'ledger_entry_id',NEW.ledger_entry_id,'financial_version',NEW.financial_version) OR
   NEW.delta_points<>0 AND NOT EXISTS(SELECT 1 FROM point_ledger_entries l WHERE l.id=NEW.ledger_entry_id AND l.brand_id=NEW.brand_id AND l.member_id=NEW.member_id AND l.entry_type='commission_correction' AND
    l.reference_type='commission_correction_target' AND l.reference_id=NEW.id AND l.operation_key='commission-correction:'||NEW.id::text AND l.request_id=s.request_id)
   THEN RAISE EXCEPTION 'correction target audit/ledger mismatch'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_commission_correction_execution_target BEFORE INSERT OR UPDATE OR DELETE ON commission_correction_execution_targets FOR EACH ROW EXECUTE FUNCTION guard_commission_correction_execution_target();
CREATE FUNCTION guard_commission_correction_balance_head() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE t commission_correction_execution_targets; p commission_correction_plan_targets;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'correction actual net head retained'; END IF;
 SELECT * INTO t FROM commission_correction_execution_targets WHERE brand_id=NEW.brand_id AND id=NEW.last_execution_target_id;
 SELECT * INTO p FROM commission_correction_plan_targets WHERE brand_id=NEW.brand_id AND id=t.plan_target_id;
 IF t.id IS NULL OR t.state<>'applied' OR t.creation_xid<>pg_current_xact_id() OR t.cycle_id<>NEW.cycle_id OR t.agent_id<>NEW.agent_id OR t.member_id<>NEW.member_id OR
  t.points_after<>NEW.points OR t.financial_version<>NEW.version OR NEW.original_target_id IS DISTINCT FROM p.original_target_id THEN RAISE EXCEPTION 'actual net head requires current transaction financial target'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.version<>1 OR t.base_financial_version IS NOT NULL THEN RAISE EXCEPTION 'first actual net head starts from original awarded basis'; END IF;
 ELSE
  IF NEW.brand_id<>OLD.brand_id OR NEW.cycle_id<>OLD.cycle_id OR NEW.agent_id<>OLD.agent_id OR NEW.member_id<>OLD.member_id OR NEW.original_target_id IS DISTINCT FROM OLD.original_target_id OR
   NEW.version<>OLD.version+1 OR t.base_financial_version IS DISTINCT FROM OLD.version OR t.points_before<>OLD.points OR p.previous_correction_target_id IS DISTINCT FROM OLD.last_execution_target_id THEN RAISE EXCEPTION 'actual net chain cannot skip or replace a prior posted difference'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_commission_correction_balance_head BEFORE INSERT OR UPDATE OR DELETE ON commission_correction_balance_heads FOR EACH ROW EXECUTE FUNCTION guard_commission_correction_balance_head();
CREATE FUNCTION capture_commission_correction_balance_head() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE original uuid;
BEGIN
 IF OLD.state='pending' AND NEW.state='applied' THEN
  SELECT original_target_id INTO original FROM commission_correction_plan_targets WHERE brand_id=NEW.brand_id AND id=NEW.plan_target_id;
  IF EXISTS(SELECT 1 FROM commission_correction_balance_heads WHERE cycle_id=NEW.cycle_id AND agent_id=NEW.agent_id) THEN
   UPDATE commission_correction_balance_heads SET version=NEW.financial_version,points=NEW.points_after,last_execution_target_id=NEW.id WHERE cycle_id=NEW.cycle_id AND agent_id=NEW.agent_id;
  ELSE
   INSERT INTO commission_correction_balance_heads(brand_id,cycle_id,agent_id,member_id,version,points,original_target_id,last_execution_target_id)
    VALUES(NEW.brand_id,NEW.cycle_id,NEW.agent_id,NEW.member_id,NEW.financial_version,NEW.points_after,original,NEW.id);
  END IF;
 END IF;
 RETURN NULL;
END $$;
CREATE TRIGGER captured_commission_correction_balance_head AFTER UPDATE ON commission_correction_execution_targets FOR EACH ROW EXECUTE FUNCTION capture_commission_correction_balance_head();
CREATE OR REPLACE FUNCTION guard_unexecuted_commission_correction_credit() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE t commission_correction_execution_targets; x commission_correction_executions; p commission_correction_plans; s commission_correction_execution_steps; cap bigint; total numeric;
BEGIN
 IF NEW.entry_type NOT IN('commission_correction','commission_correction_plan') AND NEW.reference_type NOT IN('commission_correction_plan','commission_correction_plan_target','commission_correction_target') THEN RETURN NEW; END IF;
 SELECT * INTO t FROM commission_correction_execution_targets WHERE brand_id=NEW.brand_id AND id=NEW.reference_id FOR UPDATE NOWAIT;
 SELECT * INTO x FROM commission_correction_executions WHERE brand_id=NEW.brand_id AND id=t.execution_id FOR UPDATE NOWAIT;
 SELECT * INTO p FROM commission_correction_plans WHERE brand_id=NEW.brand_id AND id=x.plan_id;
 SELECT * INTO s FROM commission_correction_execution_steps WHERE execution_id=x.id AND version=x.version+1;
 IF t.id IS NULL OR t.state<>'pending' OR t.creation_xid<>pg_current_xact_id() OR t.delta_points=0 OR x.state<>'applying' OR x.approval_audit_log_id IS NULL OR s.operation IS DISTINCT FROM 'apply' OR s.target_id IS DISTINCT FROM t.id OR
  NOT commission_correction_execution_current(NEW.brand_id,x.id) OR NOT commission_correction_source_valid(NEW.brand_id,p.payment_id,x.run_id) OR NOT commission_correction_heads_valid(NEW.brand_id,x.cycle_id) OR
  NOT EXISTS(SELECT 1 FROM commission_correction_cycle_holds WHERE brand_id=NEW.brand_id AND cycle_id=x.cycle_id AND NOT active) OR
  NEW.entry_type<>'commission_correction' OR NEW.reference_type<>'commission_correction_target' OR NEW.member_id IS DISTINCT FROM t.member_id OR NEW.actor_type<>'system' OR NEW.actor_id IS NOT NULL OR
  NEW.request_id<>s.request_id OR NEW.operation_key<>'commission-correction:'||t.id::text OR NEW.reversal_of IS NOT NULL OR
  NEW.delta_snapshot IS DISTINCT FROM jsonb_set(point_zero_snapshot(),'{commission,available}',to_jsonb(t.delta_points::text)) OR
  NEW.source_allocation IS DISTINCT FROM jsonb_build_array(jsonb_build_object('source','commission','state','available','points',abs(t.delta_points)::text)) THEN RAISE EXCEPTION 'prepared correction plans cannot authorize financial ledger postings without a current approved execution target'; END IF;
 PERFORM 1 FROM brands WHERE id=NEW.brand_id AND status<>'disabled' FOR SHARE NOWAIT;
 IF NOT FOUND THEN RAISE EXCEPTION 'correction financial brand disabled'; END IF;
 PERFORM 1 FROM brand_commission_payment_policies WHERE brand_id=NEW.brand_id AND enabled FOR SHARE NOWAIT;
 IF NOT FOUND THEN RAISE EXCEPTION 'original financial gate disabled'; END IF;
 PERFORM 1 FROM brand_commission_correction_policies WHERE brand_id=NEW.brand_id AND enabled FOR SHARE NOWAIT;
 IF NOT FOUND THEN RAISE EXCEPTION 'correction financial gate disabled'; END IF;
 SELECT max_balance_points INTO cap FROM brand_point_policies WHERE brand_id=NEW.brand_id FOR SHARE NOWAIT;
 SELECT coalesce(sum(bucket.value::numeric),0) INTO total FROM jsonb_each(NEW.after_snapshot) src CROSS JOIN LATERAL jsonb_each_text(src.value) bucket;
 IF t.delta_points>0 AND cap IS NOT NULL AND total>cap THEN RAISE EXCEPTION 'correction credit exceeds current total balance policy'; END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION require_commission_correction_execution_commit() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE x commission_correction_executions;
BEGIN
 SELECT * INTO x FROM commission_correction_executions WHERE id=NEW.id;
 IF NOT EXISTS(SELECT 1 FROM commission_correction_execution_steps WHERE execution_id=x.id AND version=1 AND operation='create' AND audit_log_id=x.creation_audit_log_id) OR
  NOT EXISTS(SELECT 1 FROM commission_correction_execution_steps WHERE execution_id=x.id AND version=x.version AND audit_log_id=x.last_audit_log_id) THEN RAISE EXCEPTION 'orphan correction execution'; END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER commission_correction_execution_commit AFTER INSERT OR UPDATE ON commission_correction_executions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_commission_correction_execution_commit();
CREATE FUNCTION require_commission_correction_execution_step_commit() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM commission_correction_executions WHERE id=NEW.execution_id AND version>=NEW.version) OR
  NEW.operation='apply' AND NOT EXISTS(SELECT 1 FROM commission_correction_execution_targets WHERE id=NEW.target_id AND execution_id=NEW.execution_id AND state='applied' AND execution_version=NEW.version-1) THEN RAISE EXCEPTION 'orphan correction execution step'; END IF; RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER commission_correction_execution_step_commit AFTER INSERT ON commission_correction_execution_steps DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_commission_correction_execution_step_commit();
CREATE FUNCTION require_commission_correction_execution_target_commit() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE t commission_correction_execution_targets;
BEGIN
 SELECT * INTO t FROM commission_correction_execution_targets WHERE id=NEW.id;
 IF t.state<>'applied' OR NOT EXISTS(SELECT 1 FROM commission_correction_executions x JOIN commission_correction_execution_steps s ON s.execution_id=x.id AND s.version=t.execution_version+1 AND s.operation='apply' AND s.target_id=t.id
  WHERE x.id=t.execution_id AND x.version>t.execution_version) OR NOT EXISTS(SELECT 1 FROM commission_correction_balance_heads h WHERE h.cycle_id=t.cycle_id AND h.agent_id=t.agent_id AND h.version>=t.financial_version) THEN RAISE EXCEPTION 'orphan/partial correction financial target'; END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER commission_correction_execution_target_commit AFTER INSERT OR UPDATE ON commission_correction_execution_targets DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_commission_correction_execution_target_commit();
CREATE FUNCTION require_commission_correction_ledger_commit() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM commission_correction_execution_targets WHERE brand_id=NEW.brand_id AND id=NEW.reference_id AND state='applied' AND ledger_entry_id=NEW.id) THEN RAISE EXCEPTION 'orphan correction financial ledger'; END IF; RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER commission_correction_ledger_commit AFTER INSERT ON point_ledger_entries DEFERRABLE INITIALLY DEFERRED FOR EACH ROW
 WHEN(NEW.entry_type='commission_correction' OR NEW.reference_type='commission_correction_target') EXECUTE FUNCTION require_commission_correction_ledger_commit();
INSERT INTO permissions(key) VALUES('commission_correction.approve.brand'),('commission_correction.continue.brand'),('commission_correction.execute_retry.brand'),('commission_correction_policy.write.brand') ON CONFLICT DO NOTHING;
INSERT INTO role_permissions(role_id,permission_key) SELECT r.id,p.key FROM roles r CROSS JOIN permissions p WHERE r.is_bootstrap AND r.brand_id IS NOT NULL AND p.key IN('commission_correction.approve.brand','commission_correction.continue.brand','commission_correction.execute_retry.brand','commission_correction_policy.write.brand') ON CONFLICT DO NOTHING;
DO $$
DECLARE app_schema text:=current_schema(); f record;
BEGIN
 FOR f IN SELECT p.oid::regprocedure signature FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname=app_schema AND p.proname IN(
  'guard_commission_correction_policy','capture_commission_correction_policy','guard_commission_correction_policy_revision','initialize_commission_correction_policy',
  'commission_correction_candidates_v2','commission_correction_candidates','commission_correction_heads_valid','commission_correction_execution_current','commission_correction_source_valid',
  'guard_commission_correction_plan_target','guard_commission_correction_execution_step','guard_commission_correction_cycle_hold','guard_commission_correction_execution',
  'guard_commission_correction_execution_target','guard_commission_correction_balance_head','capture_commission_correction_balance_head','guard_unexecuted_commission_correction_credit',
  'require_commission_correction_execution_commit','require_commission_correction_execution_step_commit','require_commission_correction_execution_target_commit','require_commission_correction_ledger_commit') LOOP
  EXECUTE format('ALTER FUNCTION %s SET search_path TO pg_catalog, %I, pg_temp',f.signature,app_schema);
 END LOOP;
END $$;
