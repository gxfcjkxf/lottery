-- Source identities and result/attempt evidence survive configuration replacement.
CREATE TABLE draw_sources (
 id uuid PRIMARY KEY,brand_id uuid NOT NULL,game_id uuid NOT NULL,
 type text NOT NULL CHECK(type IN ('api','dom','manual')),
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(brand_id,game_id,id),UNIQUE(brand_id,game_id,id,type),
 FOREIGN KEY(brand_id,game_id) REFERENCES games(brand_id,id)
);
CREATE UNIQUE INDEX one_manual_source_per_game ON draw_sources(brand_id,game_id) WHERE type='manual';
CREATE TABLE draw_source_sets (
 id uuid PRIMARY KEY,brand_id uuid NOT NULL,game_id uuid NOT NULL,
 revision bigint NOT NULL CHECK(revision>0),sources jsonb NOT NULL CHECK(jsonb_typeof(sources)='array' AND jsonb_array_length(sources)<=16),
 created_by uuid NOT NULL REFERENCES admin_accounts(id),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(brand_id,game_id,id),UNIQUE(brand_id,game_id,revision),
 FOREIGN KEY(brand_id,game_id) REFERENCES games(brand_id,id)
);
ALTER TABLE games ADD COLUMN draw_source_set_id uuid;
ALTER TABLE games ADD FOREIGN KEY(brand_id,id,draw_source_set_id) REFERENCES draw_source_sets(brand_id,game_id,id);
CREATE TABLE draw_results (
 id uuid PRIMARY KEY,brand_id uuid NOT NULL,game_id uuid NOT NULL,period_id uuid NOT NULL,
 source_id uuid NOT NULL,kind text NOT NULL CHECK(kind IN ('api','dom','manual')),
 result jsonb NOT NULL CHECK(jsonb_typeof(result)='object'),result_hash text NOT NULL CHECK(result_hash ~ '^[0-9a-f]{64}$'),
 drawn_at timestamptz NOT NULL,created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 created_by uuid REFERENCES admin_accounts(id),corrected_from_id uuid,
 CHECK((kind='manual')=(created_by IS NOT NULL)),
 UNIQUE(brand_id,game_id,period_id,id),
 FOREIGN KEY(brand_id,game_id,period_id) REFERENCES periods(brand_id,game_id,id),
 FOREIGN KEY(brand_id,game_id,source_id,kind) REFERENCES draw_sources(brand_id,game_id,id,type),
 FOREIGN KEY(brand_id,game_id,period_id,corrected_from_id) REFERENCES draw_results(brand_id,game_id,period_id,id)
);
ALTER TABLE periods ADD COLUMN draw_result_id uuid;
ALTER TABLE periods ADD COLUMN draw_claim_token uuid;
ALTER TABLE periods ADD COLUMN draw_claim_until timestamptz;
ALTER TABLE periods ADD COLUMN draw_next_poll_at timestamptz;
ALTER TABLE periods ADD CHECK((draw_claim_token IS NULL)=(draw_claim_until IS NULL));
ALTER TABLE periods ADD CHECK(status NOT IN ('drawn','settling','settled') OR draw_result_id IS NOT NULL);
ALTER TABLE periods ADD FOREIGN KEY(brand_id,game_id,id,draw_result_id) REFERENCES draw_results(brand_id,game_id,period_id,id);
CREATE TABLE draw_attempt_batches (
 id uuid PRIMARY KEY,brand_id uuid NOT NULL,game_id uuid NOT NULL,period_id uuid NOT NULL,
 source_set_id uuid NOT NULL,observed_period_version bigint NOT NULL CHECK(observed_period_version>0),
 status text NOT NULL CHECK(status IN ('accepted','no_data','failed','discarded')),
 attempts jsonb NOT NULL CHECK(jsonb_typeof(attempts)='array' AND jsonb_array_length(attempts)<=16),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(brand_id,game_id,period_id) REFERENCES periods(brand_id,game_id,id),
 FOREIGN KEY(brand_id,game_id,source_set_id) REFERENCES draw_source_sets(brand_id,game_id,id)
);
CREATE INDEX draw_results_period_history ON draw_results(brand_id,period_id,created_at DESC,id DESC);
CREATE INDEX draw_attempts_period_history ON draw_attempt_batches(brand_id,period_id,created_at DESC,id DESC);
CREATE INDEX periods_draw_poll ON periods(draw_next_poll_at,draw_at) WHERE status='waiting_draw' AND draw_result_id IS NULL;
CREATE TRIGGER immutable_draw_sources BEFORE UPDATE OR DELETE ON draw_sources FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER immutable_draw_source_sets BEFORE UPDATE OR DELETE ON draw_source_sets FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER immutable_draw_results BEFORE UPDATE OR DELETE ON draw_results FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER immutable_draw_attempt_batches BEFORE UPDATE OR DELETE ON draw_attempt_batches FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE OR REPLACE FUNCTION guard_period_history() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE old_kind text; new_kind text; predecessor uuid;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'period history cannot be deleted'; END IF;
 IF NEW.id<>OLD.id OR NEW.brand_id<>OLD.brand_id OR NEW.game_id<>OLD.game_id OR NEW.period_no<>OLD.period_no OR NEW.sequence<>OLD.sequence OR NEW.bet_start_at<>OLD.bet_start_at OR NEW.bet_end_at<>OLD.bet_end_at OR NEW.draw_at<>OLD.draw_at OR NEW.schedule_id IS DISTINCT FROM OLD.schedule_id OR NEW.created_at<>OLD.created_at THEN RAISE EXCEPTION 'immutable period identity'; END IF;
 -- Lease bookkeeping must not manufacture a business version or transition.
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
 IF NOT ((OLD.status='pending' AND NEW.status IN ('betting','judged_cancelled')) OR (OLD.status='betting' AND NEW.status IN ('closed','bet_cancelled','judged_cancelled')) OR (OLD.status='closed' AND NEW.status IN ('waiting_draw','judged_cancelled')) OR (OLD.status='waiting_draw' AND NEW.status IN ('drawn','judged_cancelled')) OR (OLD.status='drawn' AND NEW.status='drawn' AND NEW.draw_result_id IS DISTINCT FROM OLD.draw_result_id) OR (OLD.status='drawn' AND NEW.status='settling') OR (OLD.status='settling' AND NEW.status='settled')) THEN RAISE EXCEPTION 'invalid period transition'; END IF;
 RETURN NEW;
END $$;
INSERT INTO permissions(key) VALUES ('draw_source.view.brand'),('draw_source.view.platform'),('draw_source.write.brand'),('draw.view.brand'),('draw.view.platform'),('draw.manual_create.brand') ON CONFLICT DO NOTHING;
INSERT INTO role_permissions(role_id,permission_key)
 SELECT r.id,p.key FROM roles r CROSS JOIN permissions p WHERE r.is_bootstrap AND
 ((r.brand_id IS NOT NULL AND p.key IN ('draw_source.view.brand','draw_source.write.brand','draw.view.brand','draw.manual_create.brand')) OR
 (r.brand_id IS NULL AND p.key IN ('draw_source.view.platform','draw.view.platform') AND EXISTS(SELECT 1 FROM admin_account_roles ar JOIN admin_accounts a ON a.id=ar.account_id WHERE ar.role_id=r.id AND a.is_super_admin))) ON CONFLICT DO NOTHING;
