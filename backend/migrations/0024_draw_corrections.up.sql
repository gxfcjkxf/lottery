-- Results, calculations, payouts and prior generations remain immutable.
ALTER TABLE settlement_jobs ADD COLUMN generation bigint NOT NULL DEFAULT 1 CHECK(generation>0),
 ADD COLUMN previous_job_id uuid, ADD COLUMN correction_id uuid;
ALTER TABLE settlement_jobs DROP CONSTRAINT settlement_jobs_brand_id_period_id_key;
ALTER TABLE settlement_jobs ADD UNIQUE(brand_id,period_id,generation),
 ADD FOREIGN KEY(brand_id,period_id,previous_job_id) REFERENCES settlement_jobs(brand_id,period_id,id),
 ADD CHECK((generation=1 AND previous_job_id IS NULL AND correction_id IS NULL) OR (generation>1 AND previous_job_id IS NOT NULL AND correction_id IS NOT NULL));
ALTER TABLE periods ADD COLUMN current_settlement_job_id uuid, ADD COLUMN current_correction_id uuid,
 ADD FOREIGN KEY(brand_id,id,current_settlement_job_id) REFERENCES settlement_jobs(brand_id,period_id,id);
-- Projection backfill only, protected by the migration's transaction/DDL lock;
-- do not manufacture another business version for existing history.
ALTER TABLE periods DISABLE TRIGGER immutable_period_history;
UPDATE periods p SET current_settlement_job_id=j.id FROM settlement_jobs j WHERE j.brand_id=p.brand_id AND j.period_id=p.id;
ALTER TABLE periods ENABLE TRIGGER immutable_period_history;

CREATE TABLE draw_corrections (
 id uuid PRIMARY KEY, brand_id uuid NOT NULL, game_id uuid NOT NULL, period_id uuid NOT NULL,
 previous_draw_result_id uuid NOT NULL, draw_result_id uuid NOT NULL,
 period_version bigint NOT NULL CHECK(period_version>1), previous_job_id uuid, new_job_id uuid,
 policy_version bigint, mode text CHECK(mode IN ('automatic','manual')),
 state text NOT NULL CHECK(state IN ('reversing','resettling','failed','completed')),
 version bigint NOT NULL DEFAULT 1 CHECK(version>0), target_count bigint NOT NULL CHECK(target_count>=0),
 created_by uuid NOT NULL REFERENCES admin_accounts(id), reason text NOT NULL CHECK(length(btrim(reason))>0 AND octet_length(reason)<=500),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),completed_at timestamptz,last_error_code text,
 UNIQUE(brand_id,id),UNIQUE(brand_id,period_id,id),
 FOREIGN KEY(brand_id,game_id,period_id,previous_draw_result_id) REFERENCES draw_results(brand_id,game_id,period_id,id),
 FOREIGN KEY(brand_id,game_id,period_id,draw_result_id) REFERENCES draw_results(brand_id,game_id,period_id,id),
 FOREIGN KEY(brand_id,period_id,previous_job_id) REFERENCES settlement_jobs(brand_id,period_id,id),
 FOREIGN KEY(brand_id,period_id,new_job_id) REFERENCES settlement_jobs(brand_id,period_id,id),
 FOREIGN KEY(brand_id,policy_version) REFERENCES settlement_policy_history(brand_id,version),
 CHECK(previous_draw_result_id<>draw_result_id), CHECK((state='completed')=(completed_at IS NOT NULL)),
 CHECK((state='failed')=(last_error_code IS NOT NULL)),
 CHECK((previous_job_id IS NULL AND new_job_id IS NULL AND policy_version IS NULL AND mode IS NULL AND state='completed' AND target_count=0) OR (previous_job_id IS NOT NULL AND policy_version IS NOT NULL AND mode IS NOT NULL)),
 CHECK(state<>'resettling' OR new_job_id IS NOT NULL)
);
CREATE UNIQUE INDEX one_active_draw_correction ON draw_corrections(brand_id,period_id) WHERE state<>'completed';
CREATE INDEX pending_draw_corrections ON draw_corrections(created_at,id) WHERE state='reversing';
ALTER TABLE settlement_jobs ADD FOREIGN KEY(brand_id,period_id,correction_id) REFERENCES draw_corrections(brand_id,period_id,id);
ALTER TABLE periods ADD FOREIGN KEY(brand_id,id,current_correction_id) REFERENCES draw_corrections(brand_id,period_id,id);
CREATE TABLE draw_correction_targets (
 brand_id uuid NOT NULL, correction_id uuid NOT NULL, period_id uuid NOT NULL, order_id uuid NOT NULL,
 old_order_version bigint NOT NULL CHECK(old_order_version>0),old_order_status text NOT NULL,
 old_calculation_id uuid,old_payout_entry_id uuid,old_prize_points bigint NOT NULL CHECK(old_prize_points>=0),
 state text NOT NULL CHECK(state IN ('pending','reversed','unchanged','excluded','failed')),
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),reversal_entry_id uuid,reset_order_version bigint,error_code text,
 PRIMARY KEY(correction_id,order_id), FOREIGN KEY(brand_id,period_id,correction_id) REFERENCES draw_corrections(brand_id,period_id,id),
 FOREIGN KEY(brand_id,period_id,order_id) REFERENCES bet_orders(brand_id,period_id,id),
 FOREIGN KEY(brand_id,order_id,old_calculation_id) REFERENCES settlement_calculations(brand_id,order_id,id),
 FOREIGN KEY(brand_id,old_payout_entry_id) REFERENCES point_ledger_entries(brand_id,id),
 FOREIGN KEY(brand_id,reversal_entry_id) REFERENCES point_ledger_entries(brand_id,id),
 CHECK((old_prize_points>0)=(old_payout_entry_id IS NOT NULL)),
 CHECK((state='reversed')=(reset_order_version IS NOT NULL)),
 CHECK(state='reversed' OR reversal_entry_id IS NULL)
);
CREATE TABLE draw_correction_failures (
 id uuid PRIMARY KEY,brand_id uuid NOT NULL,correction_id uuid NOT NULL,order_id uuid,
 correction_version bigint NOT NULL,error_code text NOT NULL,created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(brand_id,correction_id) REFERENCES draw_corrections(brand_id,id),
 FOREIGN KEY(correction_id,order_id) REFERENCES draw_correction_targets(correction_id,order_id)
);
CREATE TRIGGER immutable_draw_correction_failures BEFORE UPDATE OR DELETE ON draw_correction_failures FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();

INSERT INTO permissions(key) VALUES('draw.correct.brand'),('draw.correction_retry.brand') ON CONFLICT DO NOTHING;
INSERT INTO role_permissions(role_id,permission_key) SELECT r.id,p.key FROM roles r CROSS JOIN permissions p WHERE r.is_bootstrap AND r.brand_id IS NOT NULL AND p.key IN ('draw.correct.brand','draw.correction_retry.brand') ON CONFLICT DO NOTHING;

CREATE FUNCTION guard_draw_correction() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'correction history cannot be deleted'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.version<>1 OR NOT EXISTS(SELECT 1 FROM periods p JOIN draw_results d ON d.id=NEW.draw_result_id JOIN audit_logs a ON a.resource_id=NEW.id AND a.resource_type='draw_correction' AND a.action='draw.correct'
   WHERE p.id=NEW.period_id AND p.brand_id=NEW.brand_id AND p.draw_result_id=NEW.previous_draw_result_id AND p.version=NEW.period_version-1 AND d.corrected_from_id=NEW.previous_draw_result_id AND d.kind='manual' AND d.created_by=NEW.created_by AND a.actor_id=NEW.created_by AND a.brand_id=NEW.brand_id AND
   ((NEW.previous_job_id IS NULL AND p.current_settlement_job_id IS NULL AND p.status='drawn' AND NEW.state='completed') OR (NEW.previous_job_id=p.current_settlement_job_id AND p.status IN ('settling','settled') AND NEW.state='reversing' AND NEW.new_job_id IS NULL AND EXISTS(SELECT 1 FROM brand_settlement_policies b WHERE b.brand_id=p.brand_id AND b.version=NEW.policy_version AND b.mode=NEW.mode)))) THEN RAISE EXCEPTION 'invalid correction start or audit witness'; END IF;
 ELSE
  IF (to_jsonb(NEW)-ARRAY['state','version','new_job_id','completed_at','last_error_code']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['state','version','new_job_id','completed_at','last_error_code']) OR NEW.version<>OLD.version+1 OR OLD.state='completed' THEN RAISE EXCEPTION 'immutable correction identity'; END IF;
  IF OLD.state<>NEW.state AND NOT ((OLD.state='reversing' AND NEW.state IN ('failed','resettling')) OR (OLD.state='failed' AND NEW.state='reversing') OR (OLD.state='resettling' AND NEW.state='completed')) THEN RAISE EXCEPTION 'invalid correction transition'; END IF;
  IF NEW.new_job_id IS DISTINCT FROM OLD.new_job_id AND NOT (OLD.state='reversing' AND NEW.state='resettling' AND OLD.new_job_id IS NULL AND NEW.new_job_id IS NOT NULL) THEN RAISE EXCEPTION 'invalid correction child job'; END IF;
 END IF;
 RETURN NEW; END $$;
CREATE TRIGGER guarded_draw_correction BEFORE INSERT OR UPDATE OR DELETE ON draw_corrections FOR EACH ROW EXECUTE FUNCTION guard_draw_correction();

CREATE FUNCTION guard_draw_correction_target() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'correction target history cannot be deleted'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.version<>1 OR NEW.reversal_entry_id IS NOT NULL OR NEW.reset_order_version IS NOT NULL OR NEW.error_code IS NOT NULL OR NOT EXISTS(SELECT 1 FROM bet_orders o JOIN draw_corrections c ON c.id=NEW.correction_id WHERE c.state='reversing' AND c.version=1 AND o.id=NEW.order_id AND o.brand_id=NEW.brand_id AND o.period_id=NEW.period_id AND o.version=NEW.old_order_version AND o.status=NEW.old_order_status AND o.settlement_calculation_id IS NOT DISTINCT FROM NEW.old_calculation_id AND o.payout_entry_id IS NOT DISTINCT FROM NEW.old_payout_entry_id AND o.prize_points=NEW.old_prize_points AND ((o.status IN ('placed','won','lost') AND NEW.state='pending') OR (o.status IN ('abnormal','bet_cancelled','judged_cancelled') AND NEW.state='excluded'))) THEN RAISE EXCEPTION 'invalid correction target snapshot'; END IF;
 ELSE
  IF (to_jsonb(NEW)-ARRAY['state','version','reversal_entry_id','reset_order_version','error_code']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['state','version','reversal_entry_id','reset_order_version','error_code']) OR NEW.version<>OLD.version+1 OR OLD.state IN ('reversed','unchanged','excluded') THEN RAISE EXCEPTION 'immutable correction target'; END IF;
  IF NOT ((OLD.state='pending' AND NEW.state IN ('reversed','unchanged','excluded','failed')) OR (OLD.state='failed' AND NEW.state='pending')) THEN RAISE EXCEPTION 'invalid correction target transition'; END IF;
  IF NEW.state='reversed' AND (NEW.old_order_status NOT IN ('won','lost') OR NEW.reset_order_version<>NEW.old_order_version+1 OR (NEW.old_prize_points>0)<>(NEW.reversal_entry_id IS NOT NULL)) THEN RAISE EXCEPTION 'invalid reset version or reversal'; END IF;
  IF NEW.state='unchanged' AND NEW.old_order_status<>'placed' THEN RAISE EXCEPTION 'unsettled target only can be unchanged'; END IF;
 END IF;
 RETURN NEW; END $$;
CREATE TRIGGER guarded_draw_correction_target BEFORE INSERT OR UPDATE OR DELETE ON draw_correction_targets FOR EACH ROW EXECUTE FUNCTION guard_draw_correction_target();

CREATE FUNCTION correction_reversal_is_applied(t draw_correction_targets) RETURNS boolean LANGUAGE plpgsql AS $$
DECLARE l point_ledger_entries; original point_ledger_entries; zero_bucket jsonb; expected jsonb;
BEGIN
 IF t.old_prize_points=0 THEN RETURN t.old_payout_entry_id IS NULL AND t.reversal_entry_id IS NULL; END IF;
 SELECT * INTO original FROM point_ledger_entries WHERE id=t.old_payout_entry_id;
 SELECT * INTO l FROM point_ledger_entries WHERE id=t.reversal_entry_id;
 zero_bucket:=jsonb_build_object('available','0','manual_frozen','0','system_frozen','0','withdrawal','0');
 expected:=jsonb_build_object('recharge',zero_bucket,'gift',zero_bucket,'winning',zero_bucket||jsonb_build_object('available',(-t.old_prize_points)::text));
 RETURN COALESCE(l.id IS NOT NULL AND original.id IS NOT NULL AND l.brand_id=t.brand_id AND l.account_id=original.account_id AND l.member_id=original.member_id AND l.entry_type='prize_reversal' AND l.reference_type='draw_correction' AND l.reference_id=t.correction_id AND l.reversal_of=t.old_payout_entry_id AND l.operation_key='draw-correction:'||t.correction_id::text||':'||t.order_id::text AND l.actor_type='system' AND l.source_allocation=original.source_allocation AND l.delta_snapshot=expected AND
  EXISTS(SELECT 1 FROM audit_logs a WHERE a.brand_id=t.brand_id AND a.action='points.prize_reversal' AND a.actor_type='system' AND a.resource_id=l.account_id AND a.after_json->>'ledger_entry_id'=l.id::text AND a.before_json=l.before_snapshot AND a.after_json->'balance'=l.after_snapshot),FALSE);
END $$;
CREATE FUNCTION validate_correction_final_state() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE c draw_corrections; p periods; j settlement_jobs;
BEGIN
 IF TG_TABLE_NAME='draw_corrections' THEN SELECT * INTO c FROM draw_corrections WHERE id=NEW.id; ELSE SELECT * INTO c FROM draw_corrections WHERE id=NEW.correction_id; END IF;
 SELECT * INTO p FROM periods WHERE id=c.period_id;
 IF c.target_count<>(SELECT count(*) FROM draw_correction_targets WHERE correction_id=c.id) OR
 (c.state IN ('resettling','completed') AND EXISTS(SELECT 1 FROM draw_correction_targets WHERE correction_id=c.id AND state IN ('pending','failed'))) OR
 EXISTS(SELECT 1 FROM draw_correction_targets t JOIN bet_orders o ON o.id=t.order_id WHERE t.correction_id=c.id AND ((t.state='reversed' AND (o.version<t.reset_order_version OR correction_reversal_is_applied(t) IS NOT TRUE)) OR (t.state='excluded' AND o.status NOT IN ('abnormal','bet_cancelled','judged_cancelled')))) THEN RAISE EXCEPTION 'correction targets lack committed evidence'; END IF;
 IF c.state<>'completed' THEN
  IF p.current_correction_id IS DISTINCT FROM c.id OR p.status<>'settling' THEN RAISE EXCEPTION 'active correction requires period witness'; END IF;
 END IF;
 IF c.state IN ('reversing','failed') AND (p.draw_result_id<>c.previous_draw_result_id OR p.current_settlement_job_id IS DISTINCT FROM c.previous_job_id OR p.version<>c.period_version) THEN RAISE EXCEPTION 'correction must not publish before reversals finish'; END IF;
 IF c.new_job_id IS NOT NULL THEN
  SELECT * INTO j FROM settlement_jobs WHERE id=c.new_job_id;
  IF j.correction_id IS DISTINCT FROM c.id OR j.previous_job_id IS DISTINCT FROM c.previous_job_id OR j.draw_result_id<>c.draw_result_id OR j.policy_version<>c.policy_version OR j.mode<>c.mode OR (c.state='completed')<>(j.state='completed') THEN RAISE EXCEPTION 'correction requires matching resettlement generation'; END IF;
 END IF;
 RETURN NULL; END $$;
CREATE CONSTRAINT TRIGGER correction_final_state AFTER INSERT OR UPDATE ON draw_corrections DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_correction_final_state();
CREATE CONSTRAINT TRIGGER correction_target_final_state AFTER INSERT OR UPDATE ON draw_correction_targets DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_correction_final_state();

CREATE OR REPLACE FUNCTION guard_settlement_job() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'settlement jobs cannot be deleted'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.version<>1 OR NEW.state<>'processing' OR NEW.approved_by IS NOT NULL THEN RAISE EXCEPTION 'invalid settlement start'; END IF;
  IF NEW.generation=1 THEN
   IF NOT EXISTS(SELECT 1 FROM periods p JOIN brand_settlement_policies b ON b.brand_id=p.brand_id WHERE p.brand_id=NEW.brand_id AND p.id=NEW.period_id AND p.current_settlement_job_id IS NULL AND p.status='drawn' AND p.version=NEW.period_version-1 AND p.draw_result_id=NEW.draw_result_id AND b.version=NEW.policy_version AND b.mode=NEW.mode) THEN RAISE EXCEPTION 'invalid initial generation'; END IF;
  ELSE
   IF NOT EXISTS(SELECT 1 FROM draw_corrections c JOIN settlement_jobs previous ON previous.id=c.previous_job_id JOIN periods p ON p.id=c.period_id WHERE c.id=NEW.correction_id AND c.brand_id=NEW.brand_id AND c.period_id=NEW.period_id AND c.state='reversing' AND c.new_job_id IS NULL AND c.draw_result_id=NEW.draw_result_id AND c.previous_job_id=NEW.previous_job_id AND previous.generation+1=NEW.generation AND c.policy_version=NEW.policy_version AND c.mode=NEW.mode AND c.created_by=NEW.created_by AND p.current_correction_id=c.id AND p.current_settlement_job_id=c.previous_job_id AND p.version=NEW.period_version-1 AND NOT EXISTS(SELECT 1 FROM draw_correction_targets t WHERE t.correction_id=c.id AND t.state IN ('pending','failed'))) THEN RAISE EXCEPTION 'new generation requires completed original reversals'; END IF;
  END IF;
 ELSE
  IF NOT EXISTS(SELECT 1 FROM periods p WHERE p.id=OLD.period_id AND p.current_settlement_job_id=OLD.id) OR EXISTS(SELECT 1 FROM draw_corrections c WHERE c.previous_job_id=OLD.id AND c.state<>'completed') THEN RAISE EXCEPTION 'superseded settlement generation is immutable'; END IF;
  IF (to_jsonb(NEW)-ARRAY['state','resume_state','version','approved_by','completed_at','last_error_code']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['state','resume_state','version','approved_by','completed_at','last_error_code']) OR NEW.version<>OLD.version+1 OR OLD.state='completed' THEN RAISE EXCEPTION 'immutable settlement job identity'; END IF;
  IF NEW.approved_by IS DISTINCT FROM OLD.approved_by AND NOT (OLD.state='awaiting_approval' AND NEW.state='paying' AND OLD.approved_by IS NULL AND NEW.approved_by IS NOT NULL AND NEW.mode='manual') THEN RAISE EXCEPTION 'invalid settlement approval'; END IF;
  IF OLD.state<>NEW.state AND NOT ((OLD.state='processing' AND NEW.state IN ('paying','awaiting_approval','failed')) OR (OLD.state='awaiting_approval' AND NEW.state='paying') OR (OLD.state='paying' AND NEW.state IN ('completed','failed')) OR (OLD.state='failed' AND NEW.state=OLD.resume_state)) THEN RAISE EXCEPTION 'invalid settlement job transition'; END IF;
 END IF; RETURN NEW; END $$;

CREATE OR REPLACE FUNCTION validate_settlement_final_state() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE j settlement_jobs; p periods; current_job boolean;
BEGIN
 IF TG_TABLE_NAME='settlement_jobs' THEN SELECT * INTO j FROM settlement_jobs WHERE id=NEW.id; ELSE SELECT * INTO j FROM settlement_jobs WHERE id=NEW.job_id; END IF;
 SELECT * INTO p FROM periods WHERE id=j.period_id;
 current_job:=p.current_settlement_job_id=j.id AND NOT EXISTS(SELECT 1 FROM draw_corrections c WHERE c.previous_job_id=j.id AND c.state<>'completed');
 IF j.target_count<>(SELECT count(*) FROM settlement_targets WHERE job_id=j.id) OR
 (j.state IN ('awaiting_approval','paying','completed') AND EXISTS(SELECT 1 FROM settlement_targets WHERE job_id=j.id AND state IN ('pending','failed'))) OR
 (j.state='completed' AND EXISTS(SELECT 1 FROM settlement_targets WHERE job_id=j.id AND state NOT IN ('paid','excluded'))) THEN RAISE EXCEPTION 'settlement target counts must match generation'; END IF;
 IF current_job AND (p.draw_result_id IS DISTINCT FROM j.draw_result_id OR p.status NOT IN ('settling','settled') OR (j.state='completed')<>(p.status='settled') OR
 EXISTS(SELECT 1 FROM bet_orders o WHERE o.period_id=j.period_id AND NOT EXISTS(SELECT 1 FROM settlement_targets t WHERE t.job_id=j.id AND t.order_id=o.id)) OR
 EXISTS(SELECT 1 FROM settlement_targets t JOIN bet_orders o ON o.id=t.order_id WHERE t.job_id=j.id AND ((t.state='paid' AND (o.status NOT IN ('won','lost') OR o.settlement_calculation_id IS DISTINCT FROM t.calculation_id OR o.payout_entry_id IS DISTINCT FROM t.payout_entry_id)) OR (t.state='excluded' AND o.status NOT IN ('abnormal','bet_cancelled','judged_cancelled'))))) THEN RAISE EXCEPTION 'current settlement generation and period must commit together'; END IF;
 RETURN NULL; END $$;

CREATE OR REPLACE FUNCTION validate_period_settlement_witness() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p periods; j settlement_jobs; c draw_corrections;
BEGIN
 SELECT * INTO p FROM periods WHERE id=NEW.id;
 IF p.status IN ('settling','settled') THEN
  SELECT * INTO c FROM draw_corrections WHERE id=p.current_correction_id AND state IN ('reversing','failed');
  IF c.id IS NOT NULL THEN
   IF p.status<>'settling' OR p.draw_result_id<>c.previous_draw_result_id OR p.version<>c.period_version OR p.current_settlement_job_id IS DISTINCT FROM c.previous_job_id THEN RAISE EXCEPTION 'reversing correction period witness mismatch'; END IF;
  ELSE
   SELECT * INTO j FROM settlement_jobs WHERE id=p.current_settlement_job_id;
   IF j.id IS NULL OR p.draw_result_id IS DISTINCT FROM j.draw_result_id OR (p.status='settled')<>(j.state='completed') OR p.version<>j.period_version+(CASE WHEN p.status='settled' THEN 1 ELSE 0 END) THEN RAISE EXCEPTION 'period settlement requires current durable generation'; END IF;
  END IF;
 END IF; RETURN NULL; END $$;

CREATE OR REPLACE FUNCTION guard_period_history() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE old_kind text; new_kind text; predecessor uuid; c draw_corrections;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'period history cannot be deleted'; END IF;
 IF NEW.id<>OLD.id OR NEW.brand_id<>OLD.brand_id OR NEW.game_id<>OLD.game_id OR NEW.period_no<>OLD.period_no OR NEW.sequence<>OLD.sequence OR NEW.bet_start_at<>OLD.bet_start_at OR NEW.bet_end_at<>OLD.bet_end_at OR NEW.draw_at<>OLD.draw_at OR NEW.schedule_id IS DISTINCT FROM OLD.schedule_id OR NEW.created_at<>OLD.created_at THEN RAISE EXCEPTION 'immutable period identity'; END IF;
 IF NEW.version=OLD.version AND NEW.status=OLD.status AND NEW.state_reason=OLD.state_reason AND NEW.draw_result_id IS NOT DISTINCT FROM OLD.draw_result_id AND NEW.current_settlement_job_id IS NOT DISTINCT FROM OLD.current_settlement_job_id AND NEW.current_correction_id IS NOT DISTINCT FROM OLD.current_correction_id AND OLD.status='waiting_draw' AND OLD.draw_result_id IS NULL THEN RETURN NEW; END IF;
 IF NEW.version<>OLD.version+1 THEN RAISE EXCEPTION 'invalid period version'; END IF;
 IF NEW.current_correction_id IS DISTINCT FROM OLD.current_correction_id THEN
  SELECT * INTO c FROM draw_corrections WHERE id=NEW.current_correction_id;
  IF c.id IS NULL OR c.previous_draw_result_id IS DISTINCT FROM OLD.draw_result_id OR c.period_version<>NEW.version OR c.previous_job_id IS DISTINCT FROM OLD.current_settlement_job_id OR NEW.current_settlement_job_id IS DISTINCT FROM OLD.current_settlement_job_id THEN RAISE EXCEPTION 'invalid correction period admission'; END IF;
  IF NOT ((c.state='completed' AND c.previous_job_id IS NULL AND OLD.status='drawn' AND NEW.status='drawn' AND NEW.draw_result_id=c.draw_result_id) OR (c.state='reversing' AND OLD.status IN ('settled','settling') AND NEW.status='settling' AND NEW.draw_result_id=OLD.draw_result_id)) THEN RAISE EXCEPTION 'invalid correction period transition'; END IF;
  RETURN NEW;
 END IF;
 IF NEW.current_settlement_job_id IS DISTINCT FROM OLD.current_settlement_job_id THEN
  IF OLD.status='drawn' AND NEW.status='settling' AND OLD.current_settlement_job_id IS NULL AND NEW.draw_result_id=OLD.draw_result_id AND EXISTS(SELECT 1 FROM settlement_jobs j WHERE j.id=NEW.current_settlement_job_id AND j.period_id=OLD.id AND j.period_version=NEW.version AND j.generation=1) THEN RETURN NEW; END IF;
  SELECT * INTO c FROM draw_corrections WHERE id=OLD.current_correction_id;
  IF c.id IS NULL OR c.state<>'resettling' OR c.new_job_id IS DISTINCT FROM NEW.current_settlement_job_id OR c.previous_job_id IS DISTINCT FROM OLD.current_settlement_job_id OR c.previous_draw_result_id<>OLD.draw_result_id OR c.draw_result_id<>NEW.draw_result_id OR OLD.status<>'settling' OR NEW.status<>'settling' OR EXISTS(SELECT 1 FROM draw_correction_targets WHERE correction_id=c.id AND state IN ('pending','failed')) THEN RAISE EXCEPTION 'publication requires finished reversals and child generation'; END IF;
  RETURN NEW;
 END IF;
 IF NEW.draw_result_id IS DISTINCT FROM OLD.draw_result_id THEN
  SELECT kind,corrected_from_id INTO new_kind,predecessor FROM draw_results WHERE id=NEW.draw_result_id;
  IF OLD.status='waiting_draw' AND NEW.status='drawn' AND OLD.draw_result_id IS NULL AND NEW.draw_result_id IS NOT NULL AND predecessor IS NULL THEN NULL;
  ELSIF OLD.status='drawn' AND NEW.status='drawn' AND new_kind='manual' AND predecessor=OLD.draw_result_id THEN
   SELECT kind INTO old_kind FROM draw_results WHERE id=OLD.draw_result_id; IF old_kind NOT IN ('api','dom') THEN RAISE EXCEPTION 'manual result already locked'; END IF;
  ELSE RAISE EXCEPTION 'invalid result pointer transition'; END IF;
  IF NEW.draw_claim_token IS NOT NULL OR NEW.draw_claim_until IS NOT NULL THEN RAISE EXCEPTION 'locked result cannot retain claim'; END IF;
 ELSIF NEW.status='drawn' AND OLD.status='waiting_draw' THEN RAISE EXCEPTION 'draw requires result'; END IF;
 IF NEW.status IN ('bet_cancelled','judged_cancelled') THEN
  IF NOT (OLD.status='pending' AND NOT EXISTS(SELECT 1 FROM bet_orders WHERE period_id=OLD.id)) AND NOT EXISTS(SELECT 1 FROM period_cancellations WHERE period_id=OLD.id AND brand_id=OLD.brand_id AND period_version=NEW.version AND mode=NEW.status AND draw_result_id IS NOT DISTINCT FROM OLD.draw_result_id) THEN RAISE EXCEPTION 'cancellation requires durable refund task'; END IF;
  IF NEW.draw_claim_token IS NOT NULL OR NEW.draw_claim_until IS NOT NULL OR NEW.draw_next_poll_at IS NOT NULL THEN RAISE EXCEPTION 'cancelled period cannot retain draw lease'; END IF;
 END IF;
 IF NOT ((OLD.status='pending' AND NEW.status IN ('betting','judged_cancelled')) OR (OLD.status='betting' AND NEW.status IN ('closed','bet_cancelled','judged_cancelled')) OR (OLD.status='closed' AND NEW.status IN ('waiting_draw','judged_cancelled')) OR (OLD.status='waiting_draw' AND NEW.status IN ('drawn','judged_cancelled')) OR (OLD.status='drawn' AND NEW.status='drawn' AND NEW.draw_result_id IS DISTINCT FROM OLD.draw_result_id) OR (OLD.status='drawn' AND NEW.status IN ('settling','judged_cancelled')) OR (OLD.status='settling' AND NEW.status='settled')) THEN RAISE EXCEPTION 'invalid period transition'; END IF;
 RETURN NEW; END $$;

CREATE FUNCTION guard_current_settlement_write() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE job uuid;
BEGIN
 IF TG_TABLE_NAME='settlement_calculations' THEN job:=NEW.job_id;
 ELSIF OLD.status='placed' AND NEW.status IN ('won','lost') THEN SELECT job_id INTO job FROM settlement_calculations WHERE id=NEW.settlement_calculation_id;
 ELSE RETURN NEW; END IF;
 IF NOT EXISTS(SELECT 1 FROM settlement_jobs j JOIN periods p ON p.id=j.period_id WHERE j.id=job AND p.current_settlement_job_id=j.id AND p.draw_result_id=j.draw_result_id AND NOT EXISTS(SELECT 1 FROM draw_corrections c WHERE c.previous_job_id=j.id AND c.state<>'completed')) THEN RAISE EXCEPTION 'superseded generation cannot calculate or pay'; END IF;
 RETURN NEW; END $$;
CREATE TRIGGER current_calculation_guard BEFORE INSERT ON settlement_calculations FOR EACH ROW EXECUTE FUNCTION guard_current_settlement_write();
CREATE TRIGGER current_payout_guard BEFORE UPDATE ON bet_orders FOR EACH ROW EXECUTE FUNCTION guard_current_settlement_write();

CREATE OR REPLACE FUNCTION guard_settlement_bet_update() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE l point_ledger_entries; c settlement_calculations; j settlement_jobs;
BEGIN
 IF (to_jsonb(NEW)-ARRAY['status','version','refund_entry_id','cancelled_at','cancel_reason','settlement_calculation_id','payout_entry_id','prize_points','settled_at']) IS DISTINCT FROM
 (to_jsonb(OLD)-ARRAY['status','version','refund_entry_id','cancelled_at','cancel_reason','settlement_calculation_id','payout_entry_id','prize_points','settled_at']) OR NEW.version<>OLD.version+1 THEN RAISE EXCEPTION 'immutable bet identity or invalid version'; END IF;
 IF OLD.status IN ('won','lost') AND NEW.status='placed' THEN
  IF NEW.refund_entry_id IS DISTINCT FROM OLD.refund_entry_id OR NEW.cancelled_at IS DISTINCT FROM OLD.cancelled_at OR NEW.cancel_reason IS DISTINCT FROM OLD.cancel_reason OR NEW.settlement_calculation_id IS NOT NULL OR NEW.payout_entry_id IS NOT NULL OR NEW.prize_points<>0 OR NEW.settled_at IS NOT NULL OR NOT EXISTS(
   SELECT 1 FROM draw_correction_targets t JOIN draw_corrections dc ON dc.id=t.correction_id JOIN periods p ON p.id=dc.period_id
   WHERE t.brand_id=OLD.brand_id AND t.order_id=OLD.id AND t.old_order_version=OLD.version AND t.old_order_status=OLD.status AND t.old_calculation_id=OLD.settlement_calculation_id AND t.old_payout_entry_id IS NOT DISTINCT FROM OLD.payout_entry_id AND t.old_prize_points=OLD.prize_points AND t.state='reversed' AND t.reset_order_version=NEW.version AND dc.state='reversing' AND p.current_correction_id=dc.id AND p.draw_result_id=dc.previous_draw_result_id AND correction_reversal_is_applied(t) IS TRUE AND
   (t.old_prize_points=0 OR EXISTS(SELECT 1 FROM point_ledger_entries le JOIN point_accounts pa ON pa.id=le.account_id WHERE le.id=t.reversal_entry_id AND pa.version=le.version AND (SELECT count(*) FROM point_buckets pb WHERE pb.brand_id=le.brand_id AND pb.account_id=le.account_id AND pb.points::numeric=(le.after_snapshot->pb.source->>pb.state)::numeric)=12)))
   THEN RAISE EXCEPTION 'order reset requires applied full reversal'; END IF;
  RETURN NEW;
 END IF;
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
