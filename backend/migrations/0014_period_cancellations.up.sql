CREATE TABLE period_cancellations (
 id uuid PRIMARY KEY,brand_id uuid NOT NULL,game_id uuid NOT NULL,period_id uuid NOT NULL,
 period_version bigint NOT NULL CHECK(period_version>1),draw_result_id uuid,
 mode text NOT NULL CHECK(mode IN ('bet_cancelled','judged_cancelled')),
 cause text NOT NULL CHECK(cause IN ('operator_cancel','no_result','invalid_result')),
 state text NOT NULL CHECK(state IN ('processing','failed','completed')),
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),target_count bigint NOT NULL CHECK(target_count>=0),
 reason text NOT NULL CHECK(length(trim(reason))>0 AND octet_length(reason)<=500),
 created_by uuid NOT NULL REFERENCES admin_accounts(id),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 completed_at timestamptz,last_error_code text NOT NULL DEFAULT '',
 CHECK((state='completed')=(completed_at IS NOT NULL)),
 CHECK(completed_at IS NULL OR completed_at>=created_at),
 CHECK((state='failed')=(last_error_code<>'')),
 CHECK((mode='bet_cancelled' AND cause='operator_cancel') OR (mode='judged_cancelled' AND cause IN ('no_result','invalid_result'))),
 CHECK(cause<>'no_result' OR draw_result_id IS NULL),
 UNIQUE(brand_id,period_id),UNIQUE(brand_id,period_id,id),
 FOREIGN KEY(brand_id,game_id,period_id) REFERENCES periods(brand_id,game_id,id),
 FOREIGN KEY(brand_id,game_id,period_id,draw_result_id) REFERENCES draw_results(brand_id,game_id,period_id,id)
);
ALTER TABLE bet_orders ADD UNIQUE(brand_id,period_id,id);
CREATE TABLE period_cancellation_targets (
 cancellation_id uuid NOT NULL,brand_id uuid NOT NULL,period_id uuid NOT NULL,order_id uuid NOT NULL,
 state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','refunded','already_refunded','failed')),
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),refund_entry_id uuid REFERENCES point_ledger_entries(id),
 error_code text NOT NULL DEFAULT '',
 CHECK((state IN ('refunded','already_refunded'))=(refund_entry_id IS NOT NULL)),
 CHECK((state='failed')=(error_code<>'')),
 PRIMARY KEY(cancellation_id,order_id),
 FOREIGN KEY(brand_id,period_id,cancellation_id) REFERENCES period_cancellations(brand_id,period_id,id),
 FOREIGN KEY(brand_id,period_id,order_id) REFERENCES bet_orders(brand_id,period_id,id)
);
CREATE INDEX cancellation_processing ON period_cancellations(created_at,id) WHERE state='processing';
CREATE INDEX cancellation_targets_pending ON period_cancellation_targets(cancellation_id,order_id) WHERE state='pending';
CREATE TABLE period_cancellation_failures (
 id uuid PRIMARY KEY,cancellation_id uuid NOT NULL REFERENCES period_cancellations(id),
 order_id uuid NOT NULL,job_version bigint NOT NULL CHECK(job_version>0),
 code text NOT NULL CHECK(length(code)>0 AND octet_length(code)<=64),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(cancellation_id,order_id) REFERENCES period_cancellation_targets(cancellation_id,order_id)
);
CREATE TRIGGER immutable_cancellation_failures BEFORE UPDATE OR DELETE ON period_cancellation_failures FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION guard_period_cancellation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'cancellation history cannot be deleted'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.version<>1 OR NEW.state NOT IN ('processing','completed') OR NOT EXISTS(SELECT 1 FROM periods WHERE id=NEW.period_id AND brand_id=NEW.brand_id AND version=NEW.period_version-1 AND status NOT IN ('settling','settled','bet_cancelled','judged_cancelled')) THEN RAISE EXCEPTION 'invalid cancellation start'; END IF;
 ELSE
  IF (to_jsonb(NEW)-ARRAY['state','version','completed_at','last_error_code']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['state','version','completed_at','last_error_code']) OR NEW.version<>OLD.version+1 THEN RAISE EXCEPTION 'immutable cancellation identity or invalid version'; END IF;
  IF NOT ((OLD.state='processing' AND NEW.state IN ('failed','completed')) OR (OLD.state='failed' AND NEW.state='processing')) THEN RAISE EXCEPTION 'invalid cancellation task transition'; END IF;
  IF NEW.state='completed' AND EXISTS(SELECT 1 FROM period_cancellation_targets WHERE cancellation_id=NEW.id AND state IN ('pending','failed')) THEN RAISE EXCEPTION 'unfinished cancellation cannot complete'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_period_cancellation BEFORE INSERT OR UPDATE OR DELETE ON period_cancellations FOR EACH ROW EXECUTE FUNCTION guard_period_cancellation();
CREATE FUNCTION validate_period_cancellation_start() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM periods WHERE id=NEW.period_id AND brand_id=NEW.brand_id AND status=NEW.mode AND version=NEW.period_version AND draw_result_id IS NOT DISTINCT FROM NEW.draw_result_id) OR (SELECT count(*) FROM period_cancellation_targets WHERE cancellation_id=NEW.id)<>NEW.target_count OR EXISTS(SELECT 1 FROM bet_orders o WHERE o.brand_id=NEW.brand_id AND o.period_id=NEW.period_id AND o.status IN ('placed','abnormal') AND NOT EXISTS(SELECT 1 FROM period_cancellation_targets t WHERE t.cancellation_id=NEW.id AND t.order_id=o.id)) THEN RAISE EXCEPTION 'cancellation must commit with period and all targets'; END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER cancellation_final_state AFTER INSERT ON period_cancellations DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_period_cancellation_start();
CREATE FUNCTION guard_cancellation_target() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'cancellation target cannot be deleted'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.state<>'pending' OR NEW.version<>1 OR NOT EXISTS(SELECT 1 FROM bet_orders WHERE id=NEW.order_id AND status IN ('placed','abnormal')) OR NOT EXISTS(SELECT 1 FROM period_cancellations c JOIN periods p ON p.id=c.period_id WHERE c.id=NEW.cancellation_id AND c.state='processing' AND p.version=c.period_version-1) THEN RAISE EXCEPTION 'invalid cancellation target'; END IF;
 ELSE
  IF (to_jsonb(NEW)-ARRAY['state','version','refund_entry_id','error_code']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['state','version','refund_entry_id','error_code']) OR NEW.version<>OLD.version+1 OR NOT ((OLD.state='pending' AND NEW.state IN ('refunded','already_refunded','failed')) OR (OLD.state='failed' AND NEW.state='pending')) THEN RAISE EXCEPTION 'invalid cancellation target update'; END IF;
  IF NEW.state IN ('refunded','already_refunded') AND NOT EXISTS(SELECT 1 FROM bet_orders WHERE id=NEW.order_id AND brand_id=NEW.brand_id AND status IN ('bet_cancelled','judged_cancelled') AND refund_entry_id=NEW.refund_entry_id) THEN RAISE EXCEPTION 'missing cancellation refund witness'; END IF;
  IF OLD.state='failed' AND NEW.state='pending' AND NOT EXISTS(SELECT 1 FROM period_cancellations WHERE id=NEW.cancellation_id AND state='processing') THEN RAISE EXCEPTION 'retry requires processing task'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_cancellation_target BEFORE INSERT OR UPDATE OR DELETE ON period_cancellation_targets FOR EACH ROW EXECUTE FUNCTION guard_cancellation_target();
CREATE FUNCTION validate_cancellation_progress() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE job_id uuid; job_state text; has_failed boolean;
BEGIN
 IF TG_TABLE_NAME='period_cancellations' THEN job_id:=NEW.id; ELSE job_id:=NEW.cancellation_id; END IF;
 SELECT state INTO job_state FROM period_cancellations WHERE id=job_id;
 SELECT EXISTS(SELECT 1 FROM period_cancellation_targets WHERE cancellation_id=job_id AND state='failed') INTO has_failed;
 IF (job_state='failed')<>has_failed OR (job_state='completed' AND EXISTS(SELECT 1 FROM period_cancellation_targets WHERE cancellation_id=job_id AND state='pending')) THEN RAISE EXCEPTION 'cancellation state must match committed progress'; END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER cancellation_job_progress AFTER UPDATE ON period_cancellations DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_cancellation_progress();
CREATE CONSTRAINT TRIGGER cancellation_target_progress AFTER UPDATE ON period_cancellation_targets DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_cancellation_progress();

CREATE OR REPLACE FUNCTION guard_period_history() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE old_kind text; new_kind text; predecessor uuid;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'period history cannot be deleted'; END IF;
 IF NEW.id<>OLD.id OR NEW.brand_id<>OLD.brand_id OR NEW.game_id<>OLD.game_id OR NEW.period_no<>OLD.period_no OR NEW.sequence<>OLD.sequence OR NEW.bet_start_at<>OLD.bet_start_at OR NEW.bet_end_at<>OLD.bet_end_at OR NEW.draw_at<>OLD.draw_at OR NEW.schedule_id IS DISTINCT FROM OLD.schedule_id OR NEW.created_at<>OLD.created_at THEN RAISE EXCEPTION 'immutable period identity'; END IF;
 IF NEW.version=OLD.version AND NEW.status=OLD.status AND NEW.state_reason=OLD.state_reason AND NEW.draw_result_id IS NOT DISTINCT FROM OLD.draw_result_id AND OLD.status='waiting_draw' AND OLD.draw_result_id IS NULL THEN RETURN NEW; END IF;
 IF NEW.version<>OLD.version+1 THEN RAISE EXCEPTION 'invalid period version'; END IF;
 IF NEW.draw_result_id IS DISTINCT FROM OLD.draw_result_id THEN
  SELECT kind,corrected_from_id INTO new_kind,predecessor FROM draw_results WHERE id=NEW.draw_result_id;
  IF OLD.status='waiting_draw' AND NEW.status='drawn' AND OLD.draw_result_id IS NULL AND NEW.draw_result_id IS NOT NULL AND predecessor IS NULL THEN NULL;
  ELSIF OLD.status='drawn' AND NEW.status='drawn' AND new_kind='manual' AND predecessor=OLD.draw_result_id THEN
   SELECT kind INTO old_kind FROM draw_results WHERE id=OLD.draw_result_id;
   IF old_kind NOT IN ('api','dom') THEN RAISE EXCEPTION 'manual result already locked'; END IF;
  ELSE RAISE EXCEPTION 'invalid result pointer transition'; END IF;
  IF NEW.draw_claim_token IS NOT NULL OR NEW.draw_claim_until IS NOT NULL THEN RAISE EXCEPTION 'locked result cannot retain claim'; END IF;
 ELSIF NEW.status='drawn' AND OLD.status='waiting_draw' THEN RAISE EXCEPTION 'draw requires result';
 END IF;
 IF NEW.status IN ('bet_cancelled','judged_cancelled') THEN
  IF NOT (OLD.status='pending' AND NOT EXISTS(SELECT 1 FROM bet_orders WHERE period_id=OLD.id)) AND NOT EXISTS(SELECT 1 FROM period_cancellations WHERE period_id=OLD.id AND brand_id=OLD.brand_id AND period_version=NEW.version AND mode=NEW.status AND draw_result_id IS NOT DISTINCT FROM OLD.draw_result_id) THEN RAISE EXCEPTION 'cancellation requires durable refund task'; END IF;
  IF NEW.draw_claim_token IS NOT NULL OR NEW.draw_claim_until IS NOT NULL OR NEW.draw_next_poll_at IS NOT NULL THEN RAISE EXCEPTION 'cancelled period cannot retain draw lease'; END IF;
 END IF;
 IF NOT ((OLD.status='pending' AND NEW.status IN ('betting','judged_cancelled')) OR (OLD.status='betting' AND NEW.status IN ('closed','bet_cancelled','judged_cancelled')) OR (OLD.status='closed' AND NEW.status IN ('waiting_draw','judged_cancelled')) OR (OLD.status='waiting_draw' AND NEW.status IN ('drawn','judged_cancelled')) OR (OLD.status='drawn' AND NEW.status='drawn' AND NEW.draw_result_id IS DISTINCT FROM OLD.draw_result_id) OR (OLD.status='drawn' AND NEW.status IN ('settling','judged_cancelled')) OR (OLD.status='settling' AND NEW.status='settled')) THEN RAISE EXCEPTION 'invalid period transition'; END IF;
 RETURN NEW;
END $$;
INSERT INTO permissions(key) VALUES('period.cancel.brand'),('period.cancel_retry.brand') ON CONFLICT DO NOTHING;
INSERT INTO role_permissions(role_id,permission_key) SELECT r.id,p.key FROM roles r CROSS JOIN permissions p WHERE r.is_bootstrap AND r.brand_id IS NOT NULL AND p.key IN ('period.cancel.brand','period.cancel_retry.brand') ON CONFLICT DO NOTHING;
