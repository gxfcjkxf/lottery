CREATE TABLE games (
 id uuid PRIMARY KEY, brand_id uuid NOT NULL REFERENCES brands(id),
 code text NOT NULL CHECK(code ~ '^[a-z][a-z0-9_]{0,47}$'), name text NOT NULL CHECK(length(name) BETWEEN 1 AND 120),
 model jsonb NOT NULL CHECK(jsonb_typeof(model)='object' AND model->>'model' IN ('X_PLUS_Y','M_SELECT_N','DIGITS_0_9')),
 timezone text NOT NULL, status text NOT NULL DEFAULT 'active' CHECK(status IN ('active','paused','disabled')),
 version bigint NOT NULL DEFAULT 1 CHECK(version>0), started_sequence bigint NOT NULL DEFAULT 0 CHECK(started_sequence>=0),
 created_by uuid NOT NULL REFERENCES admin_accounts(id), created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(brand_id,id), UNIQUE(brand_id,code)
);
CREATE TABLE play_definitions (
 id uuid PRIMARY KEY, brand_id uuid NOT NULL, game_id uuid NOT NULL,
 code text NOT NULL CHECK(code ~ '^[a-z][a-z0-9_]{0,47}$'), name text NOT NULL CHECK(length(name) BETWEEN 1 AND 120),
 status text NOT NULL DEFAULT 'active' CHECK(status IN ('active','paused','disabled')),
 active_version_id uuid, version bigint NOT NULL DEFAULT 1 CHECK(version>0), next_version_no bigint NOT NULL DEFAULT 1 CHECK(next_version_no>0),
 created_by uuid NOT NULL REFERENCES admin_accounts(id),created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(brand_id,game_id,id), UNIQUE(brand_id,game_id,code),
 FOREIGN KEY(brand_id,game_id) REFERENCES games(brand_id,id)
);
CREATE TABLE periods (
 id uuid PRIMARY KEY,brand_id uuid NOT NULL,game_id uuid NOT NULL,period_no text NOT NULL CHECK(length(period_no) BETWEEN 1 AND 80),
 sequence bigint NOT NULL CHECK(sequence>0),bet_start_at timestamptz NOT NULL,bet_end_at timestamptz NOT NULL,draw_at timestamptz NOT NULL,
 status text NOT NULL CHECK(status IN ('pending','betting','closed','waiting_draw','drawn','settling','settled','bet_cancelled','judged_cancelled')),
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(brand_id,game_id,id),UNIQUE(brand_id,game_id,period_no),UNIQUE(brand_id,game_id,sequence),
 CHECK(bet_start_at<bet_end_at AND bet_end_at<=draw_at),
 FOREIGN KEY(brand_id,game_id) REFERENCES games(brand_id,id)
);
CREATE TABLE rule_versions (
 id uuid PRIMARY KEY,brand_id uuid NOT NULL,game_id uuid NOT NULL,play_id uuid NOT NULL,
 version_no bigint NOT NULL CHECK(version_no>0),version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 definition jsonb NOT NULL CHECK(jsonb_typeof(definition)='object'),definition_sha256 text NOT NULL CHECK(definition_sha256 ~ '^[0-9a-f]{64}$'),
 status text NOT NULL DEFAULT 'draft' CHECK(status IN ('draft','pending_review','approved','active','expired','rejected','rolled_back')),
 effect_mode text NOT NULL CHECK(effect_mode IN ('immediate','next_period')),
 validation jsonb,created_by uuid NOT NULL REFERENCES admin_accounts(id),reviewed_by uuid REFERENCES admin_accounts(id),
 review_comment text NOT NULL DEFAULT '',reviewed_at timestamptz,effective_at timestamptz,effective_period_id uuid,effective_sequence bigint,
 source_version_id uuid,created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(brand_id,game_id,play_id,id),UNIQUE(brand_id,game_id,play_id,version_no),
 FOREIGN KEY(brand_id,game_id,play_id) REFERENCES play_definitions(brand_id,game_id,id),
 FOREIGN KEY(brand_id,game_id,effective_period_id) REFERENCES periods(brand_id,game_id,id),
 FOREIGN KEY(brand_id,game_id,play_id,source_version_id) REFERENCES rule_versions(brand_id,game_id,play_id,id),
 CHECK(reviewed_by IS NULL OR reviewed_by<>created_by),
 CHECK((status IN ('draft','pending_review') AND reviewed_by IS NULL AND reviewed_at IS NULL) OR (status NOT IN ('draft','pending_review') AND reviewed_by IS NOT NULL AND reviewed_at IS NOT NULL)),
 CHECK(status NOT IN ('pending_review','approved','active','expired','rolled_back') OR ((validation->>'passed'='true' AND validation->>'definition_hash'=definition_sha256) IS TRUE)),
 CHECK(status NOT IN ('active','expired','rolled_back') OR effective_at IS NOT NULL),
 CHECK(status<>'approved' OR ((effect_mode='next_period' AND effective_sequence>0 AND effective_at IS NULL) IS TRUE))
);
ALTER TABLE play_definitions ADD FOREIGN KEY(brand_id,game_id,id,active_version_id) REFERENCES rule_versions(brand_id,game_id,play_id,id);
CREATE UNIQUE INDEX one_active_rule_per_play ON rule_versions(brand_id,game_id,play_id) WHERE status='active';
CREATE UNIQUE INDEX one_scheduled_rule_per_play ON rule_versions(brand_id,game_id,play_id) WHERE status='approved';
CREATE INDEX rules_play_history ON rule_versions(brand_id,play_id,version_no DESC);
CREATE TABLE rule_version_contributors (
 rule_version_id uuid NOT NULL REFERENCES rule_versions(id),admin_id uuid NOT NULL REFERENCES admin_accounts(id),
 PRIMARY KEY(rule_version_id,admin_id)
);
CREATE TABLE period_rule_versions (
 brand_id uuid NOT NULL,game_id uuid NOT NULL,period_id uuid NOT NULL,play_id uuid NOT NULL,rule_version_id uuid NOT NULL,
 PRIMARY KEY(period_id,play_id),
 FOREIGN KEY(brand_id,game_id,period_id) REFERENCES periods(brand_id,game_id,id),
 FOREIGN KEY(brand_id,game_id,play_id,rule_version_id) REFERENCES rule_versions(brand_id,game_id,play_id,id)
);
CREATE FUNCTION guard_rule_version_history() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'rule history cannot be deleted'; END IF;
 IF NEW.id<>OLD.id OR NEW.brand_id<>OLD.brand_id OR NEW.game_id<>OLD.game_id OR NEW.play_id<>OLD.play_id OR NEW.version_no<>OLD.version_no OR NEW.created_by<>OLD.created_by OR NEW.created_at<>OLD.created_at OR NEW.source_version_id IS DISTINCT FROM OLD.source_version_id OR NEW.version<>OLD.version+1 THEN RAISE EXCEPTION 'immutable rule identity or invalid version'; END IF;
 IF OLD.status<>'draft' AND (NEW.definition IS DISTINCT FROM OLD.definition OR NEW.definition_sha256<>OLD.definition_sha256 OR NEW.effect_mode<>OLD.effect_mode OR NEW.validation IS DISTINCT FROM OLD.validation) THEN RAISE EXCEPTION 'reviewed definition is immutable'; END IF;
 IF OLD.source_version_id IS NOT NULL AND (NEW.definition IS DISTINCT FROM OLD.definition OR NEW.definition_sha256<>OLD.definition_sha256) THEN RAISE EXCEPTION 'rollback source definition is immutable'; END IF;
 IF NOT ((OLD.status='draft' AND NEW.status IN ('draft','pending_review')) OR (OLD.status='pending_review' AND NEW.status IN ('approved','active','rejected')) OR (OLD.status='approved' AND NEW.status='active') OR (OLD.status='active' AND NEW.status IN ('expired','rolled_back'))) THEN RAISE EXCEPTION 'invalid rule transition'; END IF;
 IF NEW.reviewed_by IS NOT NULL AND EXISTS(SELECT 1 FROM rule_version_contributors WHERE rule_version_id=OLD.id AND admin_id=NEW.reviewed_by) THEN RAISE EXCEPTION 'rule contributor cannot review'; END IF;
 IF OLD.reviewed_by IS NOT NULL AND (NEW.reviewed_by IS DISTINCT FROM OLD.reviewed_by OR NEW.reviewed_at IS DISTINCT FROM OLD.reviewed_at OR NEW.review_comment<>OLD.review_comment) THEN RAISE EXCEPTION 'immutable review evidence'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER immutable_rule_history BEFORE UPDATE OR DELETE ON rule_versions FOR EACH ROW EXECUTE FUNCTION guard_rule_version_history();
CREATE TRIGGER immutable_period_rules BEFORE UPDATE OR DELETE ON period_rule_versions FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER immutable_rule_contributors BEFORE UPDATE OR DELETE ON rule_version_contributors FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
INSERT INTO permissions(key) VALUES ('game.view.brand'),('game.view.platform'),('game.write.brand'),('rule.view.brand'),('rule.view.platform'),('rule.write.brand'),('rule.validate.brand'),('rule.submit.brand'),('rule.review.brand') ON CONFLICT DO NOTHING;
INSERT INTO role_permissions(role_id,permission_key)
 SELECT r.id,p.key FROM roles r CROSS JOIN permissions p WHERE r.is_bootstrap AND
 ((r.brand_id IS NOT NULL AND p.key IN ('game.view.brand','game.write.brand','rule.view.brand','rule.write.brand','rule.validate.brand','rule.submit.brand','rule.review.brand')) OR
 (r.brand_id IS NULL AND p.key IN ('game.view.platform','rule.view.platform') AND EXISTS(SELECT 1 FROM admin_account_roles ar JOIN admin_accounts a ON a.id=ar.account_id WHERE ar.role_id=r.id AND a.is_super_admin))) ON CONFLICT DO NOTHING;
