-- Opt-in only. Existing and new brands begin with no payout mode.
CREATE TABLE brand_settlement_policies (
 brand_id uuid PRIMARY KEY REFERENCES brands(id), version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 mode text CHECK(mode IN ('automatic','manual')), updated_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
INSERT INTO brand_settlement_policies(brand_id) SELECT id FROM brands;
CREATE FUNCTION initialize_settlement_policy() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 INSERT INTO brand_settlement_policies(brand_id) VALUES(NEW.id); RETURN NEW; END $$;
CREATE TRIGGER brand_default_settlement_policy AFTER INSERT ON brands FOR EACH ROW EXECUTE FUNCTION initialize_settlement_policy();
CREATE TABLE settlement_policy_history (
 brand_id uuid NOT NULL REFERENCES brands(id), version bigint NOT NULL, mode text,
 changed_by uuid NOT NULL REFERENCES admin_accounts(id), reason text NOT NULL,
 audit_log_id uuid NOT NULL REFERENCES audit_logs(id), created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(brand_id,version), CHECK(version>1), CHECK(mode IS NULL OR mode IN ('automatic','manual'))
);
CREATE TRIGGER immutable_settlement_policy_history BEFORE UPDATE OR DELETE ON settlement_policy_history FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION guard_settlement_policy() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'settlement policy cannot be deleted'; END IF;
 IF NEW.brand_id<>OLD.brand_id OR NEW.version<>OLD.version+1 OR NEW.updated_at<OLD.updated_at OR
 NOT EXISTS(SELECT 1 FROM settlement_policy_history h JOIN audit_logs a ON a.id=h.audit_log_id WHERE h.brand_id=NEW.brand_id AND h.version=NEW.version AND h.mode IS NOT DISTINCT FROM NEW.mode AND a.action='settlement_policy.write' AND a.brand_id=NEW.brand_id) THEN RAISE EXCEPTION 'settlement policy requires versioned audit evidence'; END IF;
 RETURN NEW; END $$;
CREATE TRIGGER guarded_settlement_policy BEFORE UPDATE OR DELETE ON brand_settlement_policies FOR EACH ROW EXECUTE FUNCTION guard_settlement_policy();

CREATE TABLE settlement_jobs (
 id uuid PRIMARY KEY, brand_id uuid NOT NULL, game_id uuid NOT NULL, period_id uuid NOT NULL,
 draw_result_id uuid NOT NULL, period_version bigint NOT NULL CHECK(period_version>1),
 policy_version bigint NOT NULL CHECK(policy_version>1), mode text NOT NULL CHECK(mode IN ('automatic','manual')),
 state text NOT NULL DEFAULT 'processing' CHECK(state IN ('processing','awaiting_approval','paying','failed','completed')),
 resume_state text CHECK(resume_state IN ('processing','paying')), version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 target_count bigint NOT NULL CHECK(target_count>=0), created_by uuid NOT NULL REFERENCES admin_accounts(id),
 approved_by uuid REFERENCES admin_accounts(id), reason text NOT NULL CHECK(length(btrim(reason))>0 AND octet_length(reason)<=500),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), completed_at timestamptz, last_error_code text,
 UNIQUE(brand_id,id), UNIQUE(brand_id,period_id), UNIQUE(brand_id,period_id,id),
 FOREIGN KEY(brand_id,game_id,period_id,draw_result_id) REFERENCES draw_results(brand_id,game_id,period_id,id),
 FOREIGN KEY(brand_id,policy_version) REFERENCES settlement_policy_history(brand_id,version),
 CHECK((state='completed')=(completed_at IS NOT NULL)),
 CHECK((state='failed')=(last_error_code IS NOT NULL)), CHECK((state='failed')=(resume_state IS NOT NULL)),
 CHECK(mode='manual' OR approved_by IS NULL), CHECK(state<>'paying' OR mode='automatic' OR approved_by IS NOT NULL)
);
CREATE INDEX settlement_jobs_pending ON settlement_jobs(created_at,id) WHERE state IN ('processing','paying');
CREATE TABLE settlement_calculations (
 id uuid PRIMARY KEY, brand_id uuid NOT NULL, job_id uuid NOT NULL, period_id uuid NOT NULL, order_id uuid NOT NULL,
 order_version bigint NOT NULL CHECK(order_version>0), definition_hash text NOT NULL, draw_hash text NOT NULL,
 won boolean NOT NULL, prize_points bigint NOT NULL CHECK(prize_points>=0), calculation jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), UNIQUE(brand_id,id), UNIQUE(job_id,order_id),
 UNIQUE(brand_id,order_id,id), FOREIGN KEY(brand_id,period_id,job_id) REFERENCES settlement_jobs(brand_id,period_id,id),
 FOREIGN KEY(brand_id,period_id,order_id) REFERENCES bet_orders(brand_id,period_id,id)
);
CREATE TRIGGER immutable_settlement_calculation BEFORE UPDATE OR DELETE ON settlement_calculations FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TABLE settlement_targets (
 brand_id uuid NOT NULL, job_id uuid NOT NULL, period_id uuid NOT NULL, order_id uuid NOT NULL,
 state text NOT NULL CHECK(state IN ('pending','ready','paid','excluded','failed')),
 version bigint NOT NULL DEFAULT 1 CHECK(version>0), calculation_id uuid, payout_entry_id uuid, error_code text,
 PRIMARY KEY(job_id,order_id), FOREIGN KEY(brand_id,period_id,job_id) REFERENCES settlement_jobs(brand_id,period_id,id),
 FOREIGN KEY(brand_id,period_id,order_id) REFERENCES bet_orders(brand_id,period_id,id),
 FOREIGN KEY(brand_id,order_id,calculation_id) REFERENCES settlement_calculations(brand_id,order_id,id),
 FOREIGN KEY(payout_entry_id) REFERENCES point_ledger_entries(id),
 CHECK(state NOT IN ('ready','paid') OR calculation_id IS NOT NULL), CHECK(state='paid' OR payout_entry_id IS NULL)
);
CREATE TABLE settlement_failures (
 id uuid PRIMARY KEY, brand_id uuid NOT NULL, job_id uuid NOT NULL, order_id uuid,
 job_version bigint NOT NULL, phase text NOT NULL CHECK(phase IN ('processing','paying')), error_code text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), FOREIGN KEY(brand_id,job_id) REFERENCES settlement_jobs(brand_id,id),
 FOREIGN KEY(job_id,order_id) REFERENCES settlement_targets(job_id,order_id)
);
CREATE TRIGGER immutable_settlement_failure BEFORE UPDATE OR DELETE ON settlement_failures FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();

ALTER TABLE bet_orders ADD COLUMN settlement_calculation_id uuid, ADD COLUMN payout_entry_id uuid REFERENCES point_ledger_entries(id),
 ADD COLUMN prize_points bigint NOT NULL DEFAULT 0 CHECK(prize_points>=0), ADD COLUMN settled_at timestamptz;
ALTER TABLE bet_orders ADD FOREIGN KEY(brand_id,id,settlement_calculation_id) REFERENCES settlement_calculations(brand_id,order_id,id);
ALTER TABLE bet_orders ADD CHECK((status IN ('won','lost') AND settlement_calculation_id IS NOT NULL AND settled_at IS NOT NULL) OR (status NOT IN ('won','lost') AND settlement_calculation_id IS NULL AND settled_at IS NULL AND prize_points=0 AND payout_entry_id IS NULL));
ALTER TABLE bet_orders ADD CHECK((prize_points>0)=(payout_entry_id IS NOT NULL));
ALTER TABLE bet_order_exceptions ALTER COLUMN marked_by DROP NOT NULL;
ALTER TABLE bet_order_exceptions ADD COLUMN source text NOT NULL DEFAULT 'manual' CHECK(source IN ('manual','system')),
 ADD COLUMN job_id uuid, ADD COLUMN error_code text,
 ADD FOREIGN KEY(brand_id,job_id) REFERENCES settlement_jobs(brand_id,id),
 ADD CHECK((source='manual' AND marked_by IS NOT NULL AND job_id IS NULL AND error_code IS NULL) OR (source='system' AND marked_by IS NULL AND job_id IS NOT NULL AND error_code IS NOT NULL));

-- Retain the original admission/debit guard for inserts; a separate update
-- guard extends the existing cancellation/classification transitions.
DROP TRIGGER immutable_bet_order ON bet_orders;
CREATE TRIGGER bet_order_admission_guard BEFORE INSERT OR DELETE ON bet_orders FOR EACH ROW EXECUTE FUNCTION guard_bet_order();
CREATE FUNCTION guard_settlement_bet_update() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE l point_ledger_entries; c settlement_calculations; j settlement_jobs;
BEGIN
 IF (to_jsonb(NEW)-ARRAY['status','version','refund_entry_id','cancelled_at','cancel_reason','settlement_calculation_id','payout_entry_id','prize_points','settled_at']) IS DISTINCT FROM
 (to_jsonb(OLD)-ARRAY['status','version','refund_entry_id','cancelled_at','cancel_reason','settlement_calculation_id','payout_entry_id','prize_points','settled_at']) OR NEW.version<>OLD.version+1 THEN RAISE EXCEPTION 'immutable bet identity or invalid version'; END IF;
 IF OLD.status='placed' AND NEW.status IN ('won','lost') THEN
  SELECT * INTO c FROM settlement_calculations WHERE brand_id=NEW.brand_id AND order_id=NEW.id AND id=NEW.settlement_calculation_id;
  SELECT * INTO j FROM settlement_jobs WHERE id=c.job_id;
  IF c.id IS NULL OR c.order_version<>OLD.version OR c.definition_hash<>OLD.definition_hash OR NEW.prize_points<>c.prize_points OR NEW.status<>(CASE WHEN c.won THEN 'won' ELSE 'lost' END) OR j.state<>'paying' OR NOT EXISTS(SELECT 1 FROM periods WHERE id=j.period_id AND status='settling' AND draw_result_id=j.draw_result_id) OR NEW.refund_entry_id IS NOT NULL OR NEW.cancelled_at IS NOT NULL OR NEW.cancel_reason<>'' THEN RAISE EXCEPTION 'invalid payout calculation evidence'; END IF;
  IF c.prize_points>0 THEN
   SELECT * INTO l FROM point_ledger_entries WHERE id=NEW.payout_entry_id;
   IF l.id IS NULL OR l.brand_id<>NEW.brand_id OR l.member_id<>NEW.brand_member_id OR l.entry_type<>'prize' OR l.reference_type<>'settlement_calculation' OR l.reference_id IS DISTINCT FROM c.id OR l.actor_type<>'system' OR l.reversal_of IS NOT NULL OR l.source_allocation<>jsonb_build_array(jsonb_build_object('source','winning','state','available','points',c.prize_points::text)) OR
   (l.delta_snapshot->'winning'->>'available')::numeric<>c.prize_points OR EXISTS(SELECT 1 FROM jsonb_each(l.delta_snapshot) s WHERE (s.value->>'manual_frozen')::numeric<>0 OR (s.value->>'system_frozen')::numeric<>0 OR (s.value->>'withdrawal')::numeric<>0 OR (s.key<>'winning' AND (s.value->>'available')::numeric<>0)) THEN RAISE EXCEPTION 'invalid winning ledger evidence'; END IF;
  END IF;
  RETURN NEW;
 END IF;
 IF NEW.settlement_calculation_id IS DISTINCT FROM OLD.settlement_calculation_id OR NEW.payout_entry_id IS DISTINCT FROM OLD.payout_entry_id OR NEW.prize_points<>OLD.prize_points OR NEW.settled_at IS DISTINCT FROM OLD.settled_at THEN RAISE EXCEPTION 'settlement fields are immutable outside payout'; END IF;
 IF OLD.status='placed' AND NEW.status='abnormal' THEN
  IF NEW.refund_entry_id IS DISTINCT FROM OLD.refund_entry_id OR NEW.cancelled_at IS DISTINCT FROM OLD.cancelled_at OR NEW.cancel_reason IS DISTINCT FROM OLD.cancel_reason OR NOT EXISTS(SELECT 1 FROM bet_order_exceptions WHERE brand_id=NEW.brand_id AND order_id=NEW.id AND order_version=NEW.version) THEN RAISE EXCEPTION 'invalid exception classification'; END IF;
  RETURN NEW;
 END IF;
 IF NOT (OLD.status IN ('placed','abnormal') AND NEW.status IN ('bet_cancelled','judged_cancelled')) THEN RAISE EXCEPTION 'invalid bet transition'; END IF;
 IF NEW.status='judged_cancelled' AND NOT EXISTS(SELECT 1 FROM bet_order_judgments WHERE brand_id=NEW.brand_id AND order_id=NEW.id AND order_version=NEW.version AND reason=NEW.cancel_reason) AND NOT EXISTS(SELECT 1 FROM period_cancellation_targets t JOIN period_cancellations pc ON pc.id=t.cancellation_id JOIN periods phase ON phase.id=pc.period_id WHERE t.brand_id=NEW.brand_id AND t.order_id=NEW.id AND t.state='pending' AND pc.mode='judged_cancelled' AND pc.state='processing' AND phase.status=pc.mode AND phase.version=pc.period_version) THEN RAISE EXCEPTION 'judged refund needs individual or period judgment witness'; END IF;
 SELECT * INTO l FROM point_ledger_entries WHERE id=NEW.refund_entry_id;
 IF l.entry_type<>'refund' OR l.reference_type<>'bet_order' OR l.reference_id IS DISTINCT FROM OLD.id OR l.reversal_of IS DISTINCT FROM OLD.debit_entry_id OR l.source_allocation<>OLD.deduction_allocation OR NEW.cancel_reason='' THEN RAISE EXCEPTION 'invalid bet refund evidence'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER immutable_bet_order BEFORE UPDATE ON bet_orders FOR EACH ROW EXECUTE FUNCTION guard_settlement_bet_update();

CREATE FUNCTION validate_settlement_calculation() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM bet_orders o JOIN settlement_jobs j ON j.id=NEW.job_id JOIN periods p ON p.id=j.period_id JOIN draw_results d ON d.id=j.draw_result_id
 WHERE o.id=NEW.order_id AND o.brand_id=NEW.brand_id AND o.status='placed' AND o.version=NEW.order_version AND o.definition_hash=NEW.definition_hash AND j.state='processing' AND p.status='settling' AND p.draw_result_id=j.draw_result_id AND d.result_hash=NEW.draw_hash) OR
 NEW.calculation->>'won' IS DISTINCT FROM NEW.won::text OR NEW.calculation->>'prize_points' IS DISTINCT FROM NEW.prize_points::text THEN RAISE EXCEPTION 'invalid locked settlement calculation'; END IF;
 RETURN NEW; END $$;
CREATE TRIGGER validated_settlement_calculation BEFORE INSERT ON settlement_calculations FOR EACH ROW EXECUTE FUNCTION validate_settlement_calculation();
CREATE FUNCTION guard_settlement_target() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'settlement target history cannot be deleted'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.version<>1 OR NEW.calculation_id IS NOT NULL OR NEW.payout_entry_id IS NOT NULL OR NEW.error_code IS NOT NULL OR NOT EXISTS(SELECT 1 FROM bet_orders o JOIN settlement_jobs j ON j.id=NEW.job_id WHERE o.id=NEW.order_id AND o.period_id=NEW.period_id AND j.state='processing' AND j.version=1 AND ((o.status='placed' AND NEW.state='pending') OR (o.status IN ('abnormal','bet_cancelled','judged_cancelled') AND NEW.state='excluded'))) THEN RAISE EXCEPTION 'invalid initial settlement target'; END IF;
 ELSE
  IF (to_jsonb(NEW)-ARRAY['state','version','calculation_id','payout_entry_id','error_code']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['state','version','calculation_id','payout_entry_id','error_code']) OR NEW.version<>OLD.version+1 OR OLD.state IN ('paid','excluded') THEN RAISE EXCEPTION 'immutable settlement target identity'; END IF;
  IF NOT ((OLD.state='pending' AND NEW.state IN ('ready','excluded','failed')) OR (OLD.state='ready' AND NEW.state IN ('paid','excluded','failed')) OR (OLD.state='failed' AND NEW.state IN ('pending','ready','excluded'))) THEN RAISE EXCEPTION 'invalid settlement target transition'; END IF;
  IF OLD.calculation_id IS NOT NULL AND OLD.calculation_id IS DISTINCT FROM NEW.calculation_id THEN RAISE EXCEPTION 'immutable target calculation'; END IF;
 END IF;
 RETURN NEW; END $$;
CREATE TRIGGER guarded_settlement_target BEFORE INSERT OR UPDATE OR DELETE ON settlement_targets FOR EACH ROW EXECUTE FUNCTION guard_settlement_target();
CREATE FUNCTION guard_settlement_job() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'settlement jobs cannot be deleted'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.version<>1 OR NEW.state<>'processing' OR NEW.approved_by IS NOT NULL OR NOT EXISTS(SELECT 1 FROM periods p JOIN brand_settlement_policies b ON b.brand_id=p.brand_id WHERE p.brand_id=NEW.brand_id AND p.id=NEW.period_id AND p.status='drawn' AND p.version=NEW.period_version-1 AND p.draw_result_id=NEW.draw_result_id AND b.version=NEW.policy_version AND b.mode=NEW.mode) THEN RAISE EXCEPTION 'invalid settlement start'; END IF;
 ELSE
  IF (to_jsonb(NEW)-ARRAY['state','resume_state','version','approved_by','completed_at','last_error_code']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['state','resume_state','version','approved_by','completed_at','last_error_code']) OR NEW.version<>OLD.version+1 OR OLD.state='completed' THEN RAISE EXCEPTION 'immutable settlement job identity'; END IF;
  IF NEW.approved_by IS DISTINCT FROM OLD.approved_by AND NOT (OLD.state='awaiting_approval' AND NEW.state='paying' AND OLD.approved_by IS NULL AND NEW.approved_by IS NOT NULL AND NEW.mode='manual') THEN RAISE EXCEPTION 'invalid settlement approval'; END IF;
  IF OLD.state<>NEW.state AND NOT ((OLD.state='processing' AND NEW.state IN ('paying','awaiting_approval','failed')) OR (OLD.state='awaiting_approval' AND NEW.state='paying') OR (OLD.state='paying' AND NEW.state IN ('completed','failed')) OR (OLD.state='failed' AND NEW.state=OLD.resume_state)) THEN RAISE EXCEPTION 'invalid settlement job transition'; END IF;
 END IF;
 RETURN NEW; END $$;
CREATE TRIGGER guarded_settlement_job BEFORE INSERT OR UPDATE OR DELETE ON settlement_jobs FOR EACH ROW EXECUTE FUNCTION guard_settlement_job();
CREATE FUNCTION validate_settlement_final_state() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE j settlement_jobs; p periods;
BEGIN
 IF TG_TABLE_NAME='settlement_jobs' THEN SELECT * INTO j FROM settlement_jobs WHERE id=NEW.id;
 ELSE SELECT * INTO j FROM settlement_jobs WHERE id=NEW.job_id; END IF;
 SELECT * INTO p FROM periods WHERE id=j.period_id;
 IF p.draw_result_id IS DISTINCT FROM j.draw_result_id OR p.status NOT IN ('settling','settled') OR (j.state='completed')<>(p.status='settled') OR
 j.target_count<>(SELECT count(*) FROM settlement_targets WHERE job_id=j.id) OR
 EXISTS(SELECT 1 FROM bet_orders o WHERE o.period_id=j.period_id AND NOT EXISTS(SELECT 1 FROM settlement_targets t WHERE t.job_id=j.id AND t.order_id=o.id)) OR
 (j.state IN ('awaiting_approval','paying','completed') AND EXISTS(SELECT 1 FROM settlement_targets WHERE job_id=j.id AND state IN ('pending','failed'))) OR
 (j.state='completed' AND EXISTS(SELECT 1 FROM settlement_targets WHERE job_id=j.id AND state NOT IN ('paid','excluded'))) OR
 EXISTS(SELECT 1 FROM settlement_targets t JOIN bet_orders o ON o.id=t.order_id WHERE t.job_id=j.id AND ((t.state='paid' AND (o.status NOT IN ('won','lost') OR o.settlement_calculation_id IS DISTINCT FROM t.calculation_id OR o.payout_entry_id IS DISTINCT FROM t.payout_entry_id)) OR (t.state='excluded' AND o.status NOT IN ('abnormal','bet_cancelled','judged_cancelled')))) THEN RAISE EXCEPTION 'settlement targets and period must commit together'; END IF;
 RETURN NULL; END $$;
CREATE CONSTRAINT TRIGGER settlement_job_final_state AFTER INSERT OR UPDATE ON settlement_jobs DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_settlement_final_state();
CREATE CONSTRAINT TRIGGER settlement_target_final_state AFTER INSERT OR UPDATE ON settlement_targets DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_settlement_final_state();
CREATE FUNCTION validate_period_settlement_witness() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p periods; j settlement_jobs;
BEGIN
 SELECT * INTO p FROM periods WHERE id=NEW.id;
 IF p.status IN ('settling','settled') THEN
  SELECT * INTO j FROM settlement_jobs WHERE brand_id=p.brand_id AND period_id=p.id;
  IF j.id IS NULL OR p.draw_result_id IS DISTINCT FROM j.draw_result_id OR (p.status='settled')<>(j.state='completed') OR p.version<>j.period_version+(CASE WHEN p.status='settled' THEN 1 ELSE 0 END) THEN RAISE EXCEPTION 'period settlement requires matching durable job'; END IF;
 END IF;
 RETURN NULL; END $$;
CREATE CONSTRAINT TRIGGER period_settlement_witness AFTER UPDATE ON periods DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_period_settlement_witness();
INSERT INTO permissions(key) VALUES('settlement.run.brand'),('settlement.approve.brand'),('settlement.retry.brand'),('settlement_policy.view.brand'),('settlement_policy.view.platform'),('settlement_policy.write.brand') ON CONFLICT DO NOTHING;
INSERT INTO role_permissions(role_id,permission_key) SELECT r.id,p.key FROM roles r CROSS JOIN permissions p WHERE r.is_bootstrap AND ((r.brand_id IS NOT NULL AND p.key IN ('settlement.run.brand','settlement.approve.brand','settlement.retry.brand','settlement_policy.view.brand','settlement_policy.write.brand')) OR (r.brand_id IS NULL AND p.key='settlement_policy.view.platform' AND EXISTS(SELECT 1 FROM admin_account_roles ar JOIN admin_accounts a ON a.id=ar.account_id WHERE ar.role_id=r.id AND a.is_super_admin))) ON CONFLICT DO NOTHING;
