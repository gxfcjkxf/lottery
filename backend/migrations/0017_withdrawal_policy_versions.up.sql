CREATE FUNCTION valid_withdrawal_multiple(v jsonb) RETURNS boolean LANGUAGE plpgsql IMMUTABLE AS $$
BEGIN
 RETURN jsonb_typeof(v)='string' AND (v#>>'{}') ~ '^(0|[1-9][0-9]{0,6})([.][0-9]{0,5}[1-9])?$' AND (v#>>'{}')::numeric<=1000000;
EXCEPTION WHEN OTHERS THEN RETURN false;
END $$;
CREATE FUNCTION valid_withdrawal_policy(v jsonb,game_scope boolean) RETURNS boolean LANGUAGE plpgsql IMMUTABLE AS $$
DECLARE key_count integer; sources integer;
BEGIN
 IF jsonb_typeof(v)<>'object' THEN RETURN false; END IF;
 SELECT count(*) INTO key_count FROM jsonb_object_keys(v);
 IF game_scope THEN RETURN key_count=1 AND v ? 'turnover_multiple' AND (v->'turnover_multiple'='null'::jsonb OR valid_withdrawal_multiple(v->'turnover_multiple')); END IF;
 IF key_count<>6 OR NOT v ?& ARRAY['enabled','min_points','max_points','allowed_sources','review_mode','turnover_multiple'] OR jsonb_typeof(v->'enabled')<>'boolean' OR jsonb_typeof(v->'min_points')<>'string' OR NOT (v->>'min_points') ~ '^[1-9][0-9]{0,18}$' OR (v->>'min_points')::numeric>9223372036854775807 OR jsonb_typeof(v->'allowed_sources')<>'array' OR jsonb_typeof(v->'review_mode')<>'string' OR v->>'review_mode' NOT IN ('manual','automatic') OR NOT valid_withdrawal_multiple(v->'turnover_multiple') THEN RETURN false; END IF;
 IF v->'max_points'<>'null'::jsonb AND (jsonb_typeof(v->'max_points')<>'string' OR NOT (v->>'max_points') ~ '^[1-9][0-9]{0,18}$' OR (v->>'max_points')::numeric>9223372036854775807 OR (v->>'max_points')::numeric<(v->>'min_points')::numeric) THEN RETURN false; END IF;
 sources:=jsonb_array_length(v->'allowed_sources');
 IF sources<1 OR sources>3 OR EXISTS(SELECT 1 FROM jsonb_array_elements(v->'allowed_sources') s WHERE jsonb_typeof(s)<>'string' OR s#>>'{}' NOT IN ('recharge','winning','gift')) OR (SELECT count(DISTINCT s) FROM jsonb_array_elements(v->'allowed_sources') s)<>sources THEN RETURN false; END IF;
 RETURN true;
EXCEPTION WHEN OTHERS THEN RETURN false;
END $$;
CREATE FUNCTION default_brand_withdrawal_policy() RETURNS jsonb LANGUAGE sql IMMUTABLE AS $$
 SELECT '{"enabled":false,"min_points":"1","max_points":null,"allowed_sources":["recharge","winning","gift"],"review_mode":"manual","turnover_multiple":"1"}'::jsonb
$$;
CREATE TABLE brand_withdrawal_policies (
 brand_id uuid PRIMARY KEY REFERENCES brands(id),version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 config jsonb NOT NULL DEFAULT default_brand_withdrawal_policy() CHECK(valid_withdrawal_policy(config,false)),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE game_withdrawal_policies (
 brand_id uuid NOT NULL,game_id uuid NOT NULL,version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 config jsonb NOT NULL DEFAULT '{"turnover_multiple":null}'::jsonb CHECK(valid_withdrawal_policy(config,true)),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(brand_id,game_id),FOREIGN KEY(brand_id,game_id) REFERENCES games(brand_id,id)
);
CREATE TABLE withdrawal_policy_revisions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),brand_id uuid NOT NULL REFERENCES brands(id),game_id uuid,
 version bigint NOT NULL CHECK(version>0),config jsonb NOT NULL CHECK(valid_withdrawal_policy(config,game_id IS NOT NULL)),
 changed_by uuid REFERENCES admin_accounts(id),reason text NOT NULL CHECK(length(trim(reason))>0 AND octet_length(reason)<=500),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE NULLS NOT DISTINCT(brand_id,game_id,version),FOREIGN KEY(brand_id,game_id) REFERENCES games(brand_id,id),
 CHECK((version=1)=(changed_by IS NULL))
);
CREATE INDEX withdrawal_policy_history ON withdrawal_policy_revisions(brand_id,game_id,version DESC);
CREATE FUNCTION guard_withdrawal_policy_current() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'withdrawal policy cannot be deleted'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.version<>1 THEN RAISE EXCEPTION 'withdrawal policy starts at version one'; END IF;
 ELSE
  IF (to_jsonb(NEW)-ARRAY['version','config','updated_at']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['version','config','updated_at']) OR NEW.version<>OLD.version+1 THEN RAISE EXCEPTION 'immutable policy scope or invalid version'; END IF;
 END IF;
 NEW.updated_at:=clock_timestamp();
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_brand_withdrawal_policy BEFORE INSERT OR UPDATE OR DELETE ON brand_withdrawal_policies FOR EACH ROW EXECUTE FUNCTION guard_withdrawal_policy_current();
CREATE TRIGGER guarded_game_withdrawal_policy BEFORE INSERT OR UPDATE OR DELETE ON game_withdrawal_policies FOR EACH ROW EXECUTE FUNCTION guard_withdrawal_policy_current();
CREATE FUNCTION guard_withdrawal_policy_revision() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE current_version bigint; current_config jsonb;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'withdrawal policy history is immutable'; END IF;
 IF NEW.game_id IS NULL THEN SELECT version,config INTO current_version,current_config FROM brand_withdrawal_policies WHERE brand_id=NEW.brand_id;
 ELSE SELECT version,config INTO current_version,current_config FROM game_withdrawal_policies WHERE brand_id=NEW.brand_id AND game_id=NEW.game_id; END IF;
 IF NOT FOUND OR (NEW.version=1 AND (current_version<>1 OR current_config<>NEW.config)) OR (NEW.version>1 AND current_version<>NEW.version-1) THEN RAISE EXCEPTION 'revision must belong to current or next policy version'; END IF;
 NEW.created_at:=clock_timestamp();
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_withdrawal_policy_revision BEFORE INSERT OR UPDATE OR DELETE ON withdrawal_policy_revisions FOR EACH ROW EXECUTE FUNCTION guard_withdrawal_policy_revision();
CREATE FUNCTION validate_withdrawal_policy_current() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE selected_game uuid:=NULLIF(to_jsonb(NEW)->>'game_id','')::uuid;
BEGIN
 IF NOT EXISTS(SELECT 1 FROM withdrawal_policy_revisions r WHERE r.brand_id=NEW.brand_id AND r.game_id IS NOT DISTINCT FROM selected_game AND r.version=NEW.version AND r.config=NEW.config) THEN RAISE EXCEPTION 'current policy must commit with matching immutable revision'; END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER brand_withdrawal_policy_revision_required AFTER INSERT OR UPDATE ON brand_withdrawal_policies DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_withdrawal_policy_current();
CREATE CONSTRAINT TRIGGER game_withdrawal_policy_revision_required AFTER INSERT OR UPDATE ON game_withdrawal_policies DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_withdrawal_policy_current();
CREATE FUNCTION validate_withdrawal_policy_revision_final() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE current_version bigint;
BEGIN
 IF NEW.game_id IS NULL THEN SELECT version INTO current_version FROM brand_withdrawal_policies WHERE brand_id=NEW.brand_id;
 ELSE SELECT version INTO current_version FROM game_withdrawal_policies WHERE brand_id=NEW.brand_id AND game_id=NEW.game_id; END IF;
 IF NOT FOUND OR current_version<NEW.version THEN RAISE EXCEPTION 'policy revision cannot commit ahead of current policy'; END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER withdrawal_policy_revision_committed AFTER INSERT ON withdrawal_policy_revisions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_withdrawal_policy_revision_final();
CREATE FUNCTION initialize_withdrawal_policy_revision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO withdrawal_policy_revisions(brand_id,game_id,version,config,reason) VALUES(NEW.brand_id,NULLIF(to_jsonb(NEW)->>'game_id','')::uuid,1,NEW.config,'Initial withdrawal policy');
 RETURN NULL;
END $$;
CREATE TRIGGER initial_brand_withdrawal_policy_revision AFTER INSERT ON brand_withdrawal_policies FOR EACH ROW EXECUTE FUNCTION initialize_withdrawal_policy_revision();
CREATE TRIGGER initial_game_withdrawal_policy_revision AFTER INSERT ON game_withdrawal_policies FOR EACH ROW EXECUTE FUNCTION initialize_withdrawal_policy_revision();
INSERT INTO brand_withdrawal_policies(brand_id) SELECT id FROM brands;
INSERT INTO game_withdrawal_policies(brand_id,game_id) SELECT brand_id,id FROM games;
CREATE FUNCTION initialize_withdrawal_policy() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_TABLE_NAME='brands' THEN INSERT INTO brand_withdrawal_policies(brand_id) VALUES(NEW.id);
 ELSE INSERT INTO game_withdrawal_policies(brand_id,game_id) VALUES(NEW.brand_id,NEW.id); END IF;
 RETURN NULL;
END $$;
CREATE TRIGGER brand_default_withdrawal_policy AFTER INSERT ON brands FOR EACH ROW EXECUTE FUNCTION initialize_withdrawal_policy();
CREATE TRIGGER game_default_withdrawal_policy AFTER INSERT ON games FOR EACH ROW EXECUTE FUNCTION initialize_withdrawal_policy();
INSERT INTO permissions(key) VALUES('withdrawal_policy.view.brand'),('withdrawal_policy.view.platform'),('withdrawal_policy.write.brand') ON CONFLICT DO NOTHING;
INSERT INTO role_permissions(role_id,permission_key)
 SELECT r.id,p.key FROM roles r CROSS JOIN permissions p WHERE r.is_bootstrap AND
 ((r.brand_id IS NOT NULL AND p.key IN ('withdrawal_policy.view.brand','withdrawal_policy.write.brand')) OR
 (r.brand_id IS NULL AND p.key='withdrawal_policy.view.platform' AND EXISTS(SELECT 1 FROM admin_account_roles ar JOIN admin_accounts a ON a.id=ar.account_id WHERE ar.role_id=r.id AND a.is_super_admin))) ON CONFLICT DO NOTHING;
