-- Explicit opt-in: upgrading a previously calculation-only installation must
-- never start paying its old automatic policy without an operator decision.
CREATE TABLE brand_commission_payment_policies (
 brand_id uuid PRIMARY KEY REFERENCES brands(id),version bigint NOT NULL DEFAULT 1 CHECK(version BETWEEN 1 AND 9007199254740991),
 enabled boolean NOT NULL DEFAULT false,audit_log_id uuid REFERENCES audit_logs(id),updated_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE commission_payment_policy_revisions (
 brand_id uuid NOT NULL REFERENCES brands(id),version bigint NOT NULL,enabled boolean NOT NULL,
 audit_log_id uuid REFERENCES audit_logs(id),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),PRIMARY KEY(brand_id,version)
);
CREATE FUNCTION guard_commission_payment_policy() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE a audit_logs;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'commission payment policy cannot be deleted'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.version<>1 OR NEW.enabled OR NEW.audit_log_id IS NOT NULL THEN RAISE EXCEPTION 'payment policy starts disabled'; END IF;
 ELSE
  IF NEW.brand_id<>OLD.brand_id OR NEW.version<>OLD.version+1 THEN RAISE EXCEPTION 'payment policy version mismatch'; END IF;
  SELECT * INTO a FROM audit_logs WHERE id=NEW.audit_log_id;
  IF a.id IS NULL OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.actor_type<>'admin' OR a.actor_id IS NULL OR
   a.action<>'commission.payment_policy.update' OR a.resource_type<>'commission_payment_policy' OR a.resource_id IS DISTINCT FROM NEW.brand_id OR
   a.before_json->>'version' IS DISTINCT FROM OLD.version::text OR a.before_json->'enabled' IS DISTINCT FROM to_jsonb(OLD.enabled) OR
   a.after_json->>'version' IS DISTINCT FROM NEW.version::text OR a.after_json->'enabled' IS DISTINCT FROM to_jsonb(NEW.enabled) THEN
   RAISE EXCEPTION 'payment policy requires matching audit'; END IF;
 END IF;
 NEW.updated_at:=clock_timestamp(); RETURN NEW;
END $$;
CREATE TRIGGER guarded_commission_payment_policy BEFORE INSERT OR UPDATE OR DELETE ON brand_commission_payment_policies FOR EACH ROW EXECUTE FUNCTION guard_commission_payment_policy();
CREATE FUNCTION capture_commission_payment_policy() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO commission_payment_policy_revisions(brand_id,version,enabled,audit_log_id) VALUES(NEW.brand_id,NEW.version,NEW.enabled,NEW.audit_log_id); RETURN NULL;
END $$;
CREATE TRIGGER commission_payment_policy_revision AFTER INSERT OR UPDATE ON brand_commission_payment_policies FOR EACH ROW EXECUTE FUNCTION capture_commission_payment_policy();
CREATE FUNCTION guard_commission_payment_policy_revision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP<>'INSERT' OR NOT EXISTS(SELECT 1 FROM brand_commission_payment_policies p WHERE p.brand_id=NEW.brand_id AND p.version=NEW.version AND p.enabled=NEW.enabled AND p.audit_log_id IS NOT DISTINCT FROM NEW.audit_log_id) THEN
  RAISE EXCEPTION 'payment policy revisions immutable and bound'; END IF; RETURN NEW;
END $$;
CREATE TRIGGER guarded_commission_payment_policy_revision BEFORE INSERT OR UPDATE OR DELETE ON commission_payment_policy_revisions FOR EACH ROW EXECUTE FUNCTION guard_commission_payment_policy_revision();
INSERT INTO brand_commission_payment_policies(brand_id) SELECT id FROM brands;
CREATE FUNCTION initialize_commission_payment_policy() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN INSERT INTO brand_commission_payment_policies(brand_id) VALUES(NEW.id); RETURN NULL; END $$;
CREATE TRIGGER commission_payment_policy_init AFTER INSERT ON brands FOR EACH ROW EXECUTE FUNCTION initialize_commission_payment_policy();

-- Current evidence is tested again at approval and every posting, not taken
-- from an HTTP preview. The cycle row is the serialization fence: settlement
-- changes cannot commit their epoch increment while a credit holds that lock.
CREATE FUNCTION commission_payment_evidence_current(b uuid,cid uuid,rid uuid,epoch bigint) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT EXISTS(SELECT 1 FROM commission_cycles c JOIN commission_runs r ON r.brand_id=c.brand_id AND r.cycle_id=c.id AND r.id=c.current_run_id
  WHERE c.brand_id=b AND c.id=cid AND r.id=rid AND c.state='ready' AND c.scan_complete AND r.state='ready' AND c.evidence_epoch=epoch AND r.evidence_epoch=epoch
  AND c.target_count=(SELECT count(*) FROM commission_calculations x WHERE x.run_id=rid)
  AND NOT EXISTS(SELECT 1 FROM commission_calculations x
   LEFT JOIN bet_orders o ON o.brand_id=x.brand_id AND o.id=x.order_id
   LEFT JOIN periods p ON p.brand_id=o.brand_id AND p.id=o.period_id
   LEFT JOIN settlement_calculations sc ON sc.brand_id=x.brand_id AND sc.id=x.calculation_id
   LEFT JOIN settlement_jobs j ON j.brand_id=x.brand_id AND j.id=x.job_id
   WHERE x.run_id=rid AND (o.id IS NULL OR
    (x.status,x.member_id,x.account_id,x.stake_points,x.prize_points,x.rule_snapshot) IS DISTINCT FROM
    (o.status,o.brand_member_id,o.account_id,o.total_points,o.prize_points,o.commission_rule_snapshot) OR
    (o.status IN('won','lost') AND (p.status IS DISTINCT FROM 'settled' OR p.current_settlement_job_id IS DISTINCT FROM x.job_id OR
     o.settlement_calculation_id IS DISTINCT FROM x.calculation_id OR j.state IS DISTINCT FROM 'completed' OR j.generation IS DISTINCT FROM x.generation OR
     j.draw_result_id IS DISTINCT FROM p.draw_result_id OR sc.job_id IS DISTINCT FROM x.job_id OR sc.order_id IS DISTINCT FROM x.order_id OR
     sc.prize_points IS DISTINCT FROM x.prize_points OR sc.won IS DISTINCT FROM (o.status='won') OR
     EXISTS(SELECT 1 FROM draw_corrections dc WHERE dc.brand_id=b AND dc.period_id=p.id AND dc.state<>'completed'))))))
$$;
CREATE FUNCTION commission_payment_mode(rid uuid) RETURNS text LANGUAGE sql STABLE AS $$
 SELECT CASE count(DISTINCT rule_snapshot->'financial_policy'->'config'->>'payout_mode') WHEN 0 THEN 'none' WHEN 1 THEN min(rule_snapshot->'financial_policy'->'config'->>'payout_mode') ELSE 'mixed' END
 FROM commission_calculations WHERE run_id=rid AND reason='eligible'
$$;

CREATE TABLE commission_payments (
 id uuid PRIMARY KEY,brand_id uuid NOT NULL,cycle_id uuid NOT NULL,run_id uuid NOT NULL,evidence_epoch bigint NOT NULL CHECK(evidence_epoch>=0),
 payout_mode text NOT NULL CHECK(payout_mode IN('manual','automatic','mixed','none')),
 state text NOT NULL CHECK(state IN('awaiting_approval','paying','paid','failed','stale','blocked')),
 version bigint NOT NULL DEFAULT 1 CHECK(version BETWEEN 1 AND 9007199254740991),
 total_points numeric NOT NULL CHECK(total_points>=0 AND scale(total_points)=0),target_count bigint NOT NULL CHECK(target_count>=0),
 approved_by uuid REFERENCES admin_accounts(id),approval_actor_type text CHECK(approval_actor_type IN('admin','system')),approval_audit_log_id uuid REFERENCES audit_logs(id),
 creation_audit_log_id uuid NOT NULL REFERENCES audit_logs(id),last_audit_log_id uuid NOT NULL REFERENCES audit_logs(id),last_error_code text,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),next_work_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(brand_id,id),UNIQUE(cycle_id,run_id),
 FOREIGN KEY(brand_id,cycle_id,run_id) REFERENCES commission_runs(brand_id,cycle_id,id),
 CHECK((approval_actor_type='admin') IS NOT DISTINCT FROM (approved_by IS NOT NULL) OR approval_actor_type IS NULL AND approved_by IS NULL),
 CHECK((approval_actor_type IS NULL)=(approval_audit_log_id IS NULL)),
 CHECK(state NOT IN('paying','paid','failed') OR approval_audit_log_id IS NOT NULL),
 CHECK((state IN('failed','blocked'))=(last_error_code IS NOT NULL))
);
CREATE UNIQUE INDEX commission_payment_active_cycle ON commission_payments(cycle_id) WHERE state<>'stale';
CREATE INDEX commission_payment_due ON commission_payments(next_work_at,id) WHERE state NOT IN('stale','blocked');
CREATE TABLE commission_payment_targets (
 id uuid PRIMARY KEY,brand_id uuid NOT NULL,payment_id uuid NOT NULL,earning_id uuid NOT NULL,
 points bigint NOT NULL CHECK(points>=0),member_id uuid NOT NULL,agent_id uuid NOT NULL,payment_version bigint NOT NULL CHECK(payment_version>0),
 state text NOT NULL DEFAULT 'pending' CHECK(state IN('pending','paid')),ledger_entry_id uuid REFERENCES point_ledger_entries(id),
 audit_log_id uuid REFERENCES audit_logs(id),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),paid_at timestamptz,
 UNIQUE(payment_id,earning_id),UNIQUE(ledger_entry_id),
 FOREIGN KEY(brand_id,payment_id) REFERENCES commission_payments(brand_id,id),
 FOREIGN KEY(brand_id,member_id) REFERENCES brand_members(brand_id,id),
 CHECK((state='paid')=(paid_at IS NOT NULL)),CHECK((state='paid')=(audit_log_id IS NOT NULL)),
 CHECK(state<>'pending' OR ledger_entry_id IS NULL),CHECK(state<>'paid' OR (points=0)=(ledger_entry_id IS NULL))
);

CREATE FUNCTION guard_commission_payment() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE a audit_logs; mode text; expected_state text; credits boolean;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'commission payment immutable history'; END IF;
 PERFORM 1 FROM commission_cycles WHERE brand_id=NEW.brand_id AND id=NEW.cycle_id FOR UPDATE NOWAIT;
 IF TG_OP='INSERT' THEN
  PERFORM 1 FROM brand_commission_payment_policies WHERE brand_id=NEW.brand_id AND enabled FOR SHARE NOWAIT;
  IF NOT FOUND OR NOT commission_payment_evidence_current(NEW.brand_id,NEW.cycle_id,NEW.run_id,NEW.evidence_epoch) THEN RAISE EXCEPTION 'commission payment requires current approved rollout and evidence'; END IF;
  mode:=commission_payment_mode(NEW.run_id);
  expected_state:=CASE mode WHEN 'manual' THEN 'awaiting_approval' WHEN 'mixed' THEN 'blocked' ELSE 'paying' END;
  IF NEW.version<>1 OR NEW.payout_mode IS DISTINCT FROM mode OR NEW.state<>expected_state OR
   NEW.total_points IS DISTINCT FROM (SELECT coalesce(sum(points),0) FROM commission_earnings WHERE run_id=NEW.run_id) OR
   NEW.target_count<>(SELECT count(*) FROM commission_earnings WHERE run_id=NEW.run_id) OR NEW.last_audit_log_id<>NEW.creation_audit_log_id THEN RAISE EXCEPTION 'commission payment must use complete immutable cycle totals'; END IF;
  SELECT * INTO a FROM audit_logs WHERE id=NEW.creation_audit_log_id;
  IF a.actor_type IS DISTINCT FROM 'system' OR a.actor_id IS NOT NULL OR a.action IS DISTINCT FROM 'commission.payment.create' OR
   a.brand_id IS DISTINCT FROM NEW.brand_id OR a.resource_type IS DISTINCT FROM 'commission_payment' OR a.resource_id IS DISTINCT FROM NEW.id OR
   a.after_json->>'version' IS DISTINCT FROM '1' OR a.after_json->>'state' IS DISTINCT FROM NEW.state OR
   a.after_json->>'run_id' IS DISTINCT FROM NEW.run_id::text OR a.after_json->>'total_points' IS DISTINCT FROM NEW.total_points::text OR
   a.after_json->>'payout_mode' IS DISTINCT FROM NEW.payout_mode THEN RAISE EXCEPTION 'commission payment creation audit mismatch'; END IF;
  IF NEW.state='paying' AND (NEW.approval_actor_type IS DISTINCT FROM 'system' OR NEW.approved_by IS NOT NULL OR NEW.approval_audit_log_id IS DISTINCT FROM NEW.creation_audit_log_id) OR
   NEW.state<>'paying' AND NEW.approval_audit_log_id IS NOT NULL OR
   NEW.last_error_code IS DISTINCT FROM (CASE WHEN mode='mixed' THEN 'COMMISSION_PAYMENT_MODE_UNRESOLVED' ELSE NULL END) THEN RAISE EXCEPTION 'payment approval must follow saved mode'; END IF;
 ELSE
  IF (to_jsonb(NEW)-ARRAY['state','version','approved_by','approval_actor_type','approval_audit_log_id','last_audit_log_id','last_error_code','updated_at','next_work_at']) IS DISTINCT FROM
   (to_jsonb(OLD)-ARRAY['state','version','approved_by','approval_actor_type','approval_audit_log_id','last_audit_log_id','last_error_code','updated_at','next_work_at']) THEN RAISE EXCEPTION 'payment identity and totals immutable'; END IF;
  IF (to_jsonb(NEW)-ARRAY['next_work_at']) IS NOT DISTINCT FROM (to_jsonb(OLD)-ARRAY['next_work_at']) THEN RETURN NEW; END IF;
  IF NEW.version<>OLD.version+1 OR NOT((OLD.state='awaiting_approval' AND NEW.state IN('paying','stale','blocked')) OR
   (OLD.state='paying' AND NEW.state IN('paying','paid','failed','stale','blocked')) OR (OLD.state='failed' AND NEW.state IN('paying','stale','blocked')) OR
   (OLD.state='paid' AND NEW.state IN('blocked','stale'))) THEN RAISE EXCEPTION 'payment state/version conflict'; END IF;
  SELECT * INTO a FROM audit_logs WHERE id=NEW.last_audit_log_id;
  IF a.id IS NULL OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.resource_type<>'commission_payment' OR a.resource_id IS DISTINCT FROM NEW.id OR
   a.before_json->>'version' IS DISTINCT FROM OLD.version::text OR a.before_json->>'state' IS DISTINCT FROM OLD.state OR
   a.after_json->>'version' IS DISTINCT FROM NEW.version::text OR a.after_json->>'state' IS DISTINCT FROM NEW.state OR
   a.action NOT IN('commission.payment.approve','commission.payment.retry','commission.payment.post','commission.payment.complete','commission.payment.fail','commission.payment.invalidate') THEN RAISE EXCEPTION 'payment state transition audit mismatch'; END IF;
  IF NOT ((a.action='commission.payment.approve' AND OLD.state='awaiting_approval' AND NEW.state='paying' AND a.actor_type='admin' AND a.actor_id IS NOT NULL) OR
   (a.action='commission.payment.retry' AND OLD.state='failed' AND NEW.state='paying' AND a.actor_type='admin' AND a.actor_id IS NOT NULL) OR
   (a.action='commission.payment.post' AND OLD.state='paying' AND NEW.state='paying' AND a.actor_type='system' AND a.actor_id IS NULL) OR
   (a.action='commission.payment.complete' AND OLD.state='paying' AND NEW.state='paid' AND a.actor_type='system' AND a.actor_id IS NULL) OR
   (a.action='commission.payment.fail' AND OLD.state='paying' AND NEW.state='failed' AND a.actor_type='system' AND a.actor_id IS NULL) OR
   (a.action='commission.payment.invalidate' AND NEW.state IN('blocked','stale') AND a.actor_type='system' AND a.actor_id IS NULL)) THEN RAISE EXCEPTION 'payment operation/state/actor mismatch'; END IF;
  IF OLD.approval_audit_log_id IS NOT NULL AND (NEW.approved_by,NEW.approval_actor_type,NEW.approval_audit_log_id) IS DISTINCT FROM (OLD.approved_by,OLD.approval_actor_type,OLD.approval_audit_log_id) THEN RAISE EXCEPTION 'payment approval immutable'; END IF;
  IF OLD.state='awaiting_approval' AND NEW.state='paying' THEN
   IF NEW.payout_mode<>'manual' OR NEW.approval_actor_type IS DISTINCT FROM 'admin' OR NEW.approved_by IS DISTINCT FROM a.actor_id OR a.actor_type<>'admin' OR
    NEW.approval_audit_log_id IS DISTINCT FROM NEW.last_audit_log_id OR a.action<>'commission.payment.approve' THEN RAISE EXCEPTION 'manual payment requires operator approval'; END IF;
  ELSIF (NEW.approved_by,NEW.approval_actor_type,NEW.approval_audit_log_id) IS DISTINCT FROM (OLD.approved_by,OLD.approval_actor_type,OLD.approval_audit_log_id) THEN RAISE EXCEPTION 'payment approval cannot change'; END IF;
  IF NEW.state IN('paying','paid') AND NOT commission_payment_evidence_current(NEW.brand_id,NEW.cycle_id,NEW.run_id,NEW.evidence_epoch) THEN RAISE EXCEPTION 'payment evidence no longer current'; END IF;
  SELECT EXISTS(SELECT 1 FROM commission_payment_targets WHERE payment_id=NEW.id AND ledger_entry_id IS NOT NULL) INTO credits;
  IF NEW.state='stale' AND credits THEN RAISE EXCEPTION 'credited payment needs compensation, cannot stale'; END IF;
  IF NEW.state='paid' AND (NEW.target_count<>(SELECT count(*) FROM commission_payment_targets WHERE payment_id=NEW.id AND state='paid') OR
   NEW.total_points IS DISTINCT FROM (SELECT coalesce(sum(points),0) FROM commission_payment_targets WHERE payment_id=NEW.id AND state='paid')) THEN RAISE EXCEPTION 'payment completion incomplete'; END IF;
 END IF;
 NEW.updated_at:=clock_timestamp(); RETURN NEW;
END $$;
CREATE TRIGGER guarded_commission_payment BEFORE INSERT OR UPDATE OR DELETE ON commission_payments FOR EACH ROW EXECUTE FUNCTION guard_commission_payment();

CREATE FUNCTION guard_commission_payment_target() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p commission_payments; e commission_earnings; a audit_logs;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'commission payment target cannot be deleted'; END IF;
 SELECT * INTO p FROM commission_payments WHERE brand_id=NEW.brand_id AND id=NEW.payment_id FOR UPDATE NOWAIT;
 SELECT * INTO e FROM commission_earnings WHERE id=NEW.earning_id;
 IF p.state IS DISTINCT FROM 'paying' OR (e.brand_id,e.cycle_id,e.run_id,e.member_id,e.agent_id,e.points) IS DISTINCT FROM
  (NEW.brand_id,p.cycle_id,p.run_id,NEW.member_id,NEW.agent_id,NEW.points) THEN RAISE EXCEPTION 'payment target must bind approved earning'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.state<>'pending' OR NEW.ledger_entry_id IS NOT NULL OR NEW.audit_log_id IS NOT NULL OR NEW.paid_at IS NOT NULL OR NEW.payment_version<>p.version THEN RAISE EXCEPTION 'payment target starts pending at current version'; END IF;
 ELSE
  IF OLD.state<>'pending' OR NEW.state<>'paid' OR
   (to_jsonb(NEW)-ARRAY['state','ledger_entry_id','audit_log_id','paid_at']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['state','ledger_entry_id','audit_log_id','paid_at']) THEN RAISE EXCEPTION 'payment target identity immutable'; END IF;
  SELECT * INTO a FROM audit_logs WHERE id=NEW.audit_log_id;
  IF a.id IS NULL OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.actor_type<>'system' OR a.actor_id IS NOT NULL OR
   a.action<>'commission.payment.target' OR a.resource_type<>'commission_payment_target' OR a.resource_id IS DISTINCT FROM NEW.id OR
   a.after_json->>'points' IS DISTINCT FROM NEW.points::text OR a.after_json->>'ledger_entry_id' IS DISTINCT FROM NEW.ledger_entry_id::text THEN RAISE EXCEPTION 'payment target audit mismatch'; END IF;
 END IF; RETURN NEW;
END $$;
CREATE TRIGGER guarded_commission_payment_target BEFORE INSERT OR UPDATE OR DELETE ON commission_payment_targets FOR EACH ROW EXECUTE FUNCTION guard_commission_payment_target();

CREATE FUNCTION guard_commission_payment_credit() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE t commission_payment_targets; p commission_payments; expected jsonb;
BEGIN
 IF NEW.entry_type<>'commission' AND NEW.reference_type<>'commission_payment_target' THEN RETURN NEW; END IF;
 SELECT * INTO t FROM commission_payment_targets WHERE brand_id=NEW.brand_id AND id=NEW.reference_id FOR UPDATE NOWAIT;
 SELECT * INTO p FROM commission_payments WHERE brand_id=NEW.brand_id AND id=t.payment_id FOR UPDATE NOWAIT;
 PERFORM 1 FROM commission_cycles WHERE id=p.cycle_id AND brand_id=NEW.brand_id FOR UPDATE NOWAIT;
 IF NOT FOUND THEN RAISE EXCEPTION 'commission credit missing cycle'; END IF;
 PERFORM 1 FROM brand_commission_payment_policies WHERE brand_id=NEW.brand_id AND enabled FOR SHARE NOWAIT;
 IF NOT FOUND OR NOT commission_payment_evidence_current(NEW.brand_id,p.cycle_id,p.run_id,p.evidence_epoch) THEN RAISE EXCEPTION 'commission credit rollout or evidence unavailable'; END IF;
 PERFORM 1 FROM brands WHERE id=NEW.brand_id AND status<>'disabled' FOR SHARE NOWAIT;
 expected:=jsonb_set(point_zero_snapshot(),'{commission,available}',to_jsonb(t.points::text));
 IF NOT FOUND OR t.id IS NULL OR p.state IS DISTINCT FROM 'paying' OR p.approval_audit_log_id IS NULL OR t.state<>'pending' OR t.points<=0 OR
  NEW.entry_type<>'commission' OR NEW.reference_type<>'commission_payment_target' OR NEW.member_id IS DISTINCT FROM t.member_id OR
  NEW.operation_key IS DISTINCT FROM 'commission-payment:'||t.id::text OR NEW.actor_type<>'system' OR NEW.actor_id IS NOT NULL OR NEW.reversal_of IS NOT NULL OR
  NEW.delta_snapshot IS DISTINCT FROM expected OR NEW.source_allocation IS DISTINCT FROM jsonb_build_array(jsonb_build_object('source','commission','state','available','points',t.points::text)) THEN
  RAISE EXCEPTION 'commission credit must match unique approved target'; END IF; RETURN NEW;
END $$;
CREATE TRIGGER guarded_commission_payment_credit BEFORE INSERT ON point_ledger_entries FOR EACH ROW EXECUTE FUNCTION guard_commission_payment_credit();
CREATE FUNCTION require_commission_payment_credit_commit() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.entry_type='commission' OR NEW.reference_type='commission_payment_target' THEN
  IF NOT EXISTS(SELECT 1 FROM commission_payment_targets t WHERE t.brand_id=NEW.brand_id AND t.id=NEW.reference_id AND t.state='paid' AND t.ledger_entry_id=NEW.id) THEN RAISE EXCEPTION 'orphan commission credit'; END IF;
 END IF; RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER commission_payment_credit_commit AFTER INSERT ON point_ledger_entries DEFERRABLE INITIALLY DEFERRED FOR EACH ROW
 WHEN (NEW.entry_type='commission' OR NEW.reference_type='commission_payment_target') EXECUTE FUNCTION require_commission_payment_credit_commit();
CREATE FUNCTION require_commission_payment_target_commit() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.state='paid' AND NOT EXISTS(SELECT 1 FROM commission_payments p JOIN audit_logs a ON a.brand_id=p.brand_id AND a.resource_id=p.id
  WHERE p.brand_id=NEW.brand_id AND p.id=NEW.payment_id AND p.version>NEW.payment_version AND a.action='commission.payment.post' AND a.resource_type='commission_payment' AND
   a.actor_type='system' AND a.actor_id IS NULL AND a.before_json->>'version'=NEW.payment_version::text AND a.after_json->>'version'=(NEW.payment_version+1)::text AND
   a.after_json->>'target_id'=NEW.id::text) THEN RAISE EXCEPTION 'paid target needs matching committed payment step'; END IF;
 IF NEW.state='paid' AND NEW.points>0 AND NOT EXISTS(SELECT 1 FROM point_ledger_entries l WHERE l.id=NEW.ledger_entry_id AND l.brand_id=NEW.brand_id AND l.member_id=NEW.member_id AND
  l.entry_type='commission' AND l.reference_type='commission_payment_target' AND l.reference_id=NEW.id AND l.operation_key='commission-payment:'||NEW.id::text) THEN RAISE EXCEPTION 'paid commission target missing credit'; END IF; RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER commission_payment_target_commit AFTER UPDATE ON commission_payment_targets DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_commission_payment_target_commit();

INSERT INTO permissions(key) VALUES('commission_payment.approve.brand'),('commission_payment.retry.brand'),('commission_payment_policy.write.brand') ON CONFLICT DO NOTHING;
INSERT INTO role_permissions(role_id,permission_key) SELECT r.id,p.key FROM roles r CROSS JOIN permissions p WHERE r.is_bootstrap AND r.brand_id IS NOT NULL AND
 p.key IN('commission_payment.approve.brand','commission_payment.retry.brand','commission_payment_policy.write.brand') ON CONFLICT DO NOTHING;
DO $$
DECLARE app_schema text:=current_schema(); f record;
BEGIN
 FOR f IN SELECT p.oid::regprocedure signature FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname=app_schema AND p.proname IN(
  'guard_commission_payment_policy','capture_commission_payment_policy','guard_commission_payment_policy_revision','initialize_commission_payment_policy',
  'commission_payment_evidence_current','commission_payment_mode','guard_commission_payment','guard_commission_payment_target','guard_commission_payment_credit',
  'require_commission_payment_credit_commit','require_commission_payment_target_commit') LOOP
  EXECUTE format('ALTER FUNCTION %s SET search_path TO pg_catalog, %I, pg_temp',f.signature,app_schema);
 END LOOP;
END $$;
