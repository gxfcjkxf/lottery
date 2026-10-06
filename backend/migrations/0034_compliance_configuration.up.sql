-- Configuration and explicit stub checks only; no implicit verification or live
-- registration, betting, withdrawal, member-status or wallet mutation.
CREATE FUNCTION default_compliance_config() RETURNS jsonb LANGUAGE sql IMMUTABLE AS $$
 SELECT '{"age_enabled":false,"minimum_age":null,"region_enabled":false,"allowed_countries":[],"identity_enabled":false}'::jsonb
$$;
CREATE FUNCTION valid_compliance_config(v jsonb) RETURNS boolean LANGUAGE plpgsql IMMUTABLE AS $$
DECLARE countries jsonb; n integer;
BEGIN
 IF jsonb_typeof(v)<>'object' OR (SELECT count(*) FROM jsonb_object_keys(v))<>5
 OR NOT(v ?& ARRAY['age_enabled','minimum_age','region_enabled','allowed_countries','identity_enabled'])
 OR jsonb_typeof(v->'age_enabled')<>'boolean' OR jsonb_typeof(v->'region_enabled')<>'boolean'
 OR jsonb_typeof(v->'identity_enabled')<>'boolean' OR jsonb_typeof(v->'allowed_countries')<>'array' THEN RETURN false; END IF;
 IF v->'minimum_age'<>'null'::jsonb AND (jsonb_typeof(v->'minimum_age')<>'number' OR v->>'minimum_age' !~ '^[0-9]+$' OR (v->>'minimum_age')::integer NOT BETWEEN 18 AND 120) THEN RETURN false; END IF;
 IF v->'age_enabled'='true'::jsonb AND v->'minimum_age'='null'::jsonb THEN RETURN false; END IF;
 n:=jsonb_array_length(v->'allowed_countries');
 IF n>250 OR (v->'region_enabled'='true'::jsonb AND n=0)
 OR EXISTS(SELECT 1 FROM jsonb_array_elements(v->'allowed_countries') e WHERE jsonb_typeof(e)<>'string' OR e#>>'{}' !~ '^[A-Z]{2}$') THEN RETURN false; END IF;
 SELECT coalesce(jsonb_agg(x ORDER BY x COLLATE "C"),'[]'::jsonb) INTO countries FROM (SELECT DISTINCT jsonb_array_elements_text(v->'allowed_countries') x) q;
 RETURN countries=v->'allowed_countries';
EXCEPTION WHEN OTHERS THEN RETURN false;
END $$;
CREATE TABLE brand_compliance_policies (
 brand_id uuid PRIMARY KEY REFERENCES brands(id), version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 config jsonb NOT NULL DEFAULT default_compliance_config() CHECK(valid_compliance_config(config) IS TRUE),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE compliance_policy_revisions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),brand_id uuid NOT NULL REFERENCES brands(id),version bigint NOT NULL CHECK(version>0),
 config jsonb NOT NULL CHECK(valid_compliance_config(config) IS TRUE),changed_by uuid REFERENCES admin_accounts(id),
 reason text NOT NULL CHECK(length(trim(reason))>0 AND octet_length(reason)<=500),audit_log_id uuid REFERENCES audit_logs(id),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),UNIQUE(brand_id,version),
 CHECK((version=1)=(changed_by IS NULL AND audit_log_id IS NULL)),CHECK(version=1 OR (changed_by IS NOT NULL AND audit_log_id IS NOT NULL))
);
CREATE FUNCTION guard_compliance_policy() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'compliance policy cannot be deleted'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.version<>1 OR NEW.config<>default_compliance_config() THEN RAISE EXCEPTION 'disabled initial compliance policy required'; END IF;
 ELSE
  IF NEW.brand_id IS DISTINCT FROM OLD.brand_id OR NEW.version<>OLD.version+1 THEN RAISE EXCEPTION 'compliance identity/version invalid'; END IF;
 END IF;
 NEW.updated_at:=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER guarded_compliance_policy BEFORE INSERT OR UPDATE OR DELETE ON brand_compliance_policies FOR EACH ROW EXECUTE FUNCTION guard_compliance_policy();
CREATE FUNCTION guard_compliance_revision() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p brand_compliance_policies;a audit_logs;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'compliance revision immutable'; END IF;
 SELECT * INTO p FROM brand_compliance_policies WHERE brand_id=NEW.brand_id;
 IF NOT FOUND OR p.version IS DISTINCT FROM NEW.version OR p.config IS DISTINCT FROM NEW.config THEN RAISE EXCEPTION 'compliance revision requires current policy'; END IF;
 IF NEW.version=1 THEN
  IF NEW.config<>default_compliance_config() THEN RAISE EXCEPTION 'initial compliance history must be disabled'; END IF;
 ELSE
  SELECT * INTO a FROM audit_logs WHERE id=NEW.audit_log_id;
  IF NOT FOUND OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.actor_type IS DISTINCT FROM 'admin' OR a.actor_id IS DISTINCT FROM NEW.changed_by
  OR a.action IS DISTINCT FROM 'compliance.policy.update' OR a.resource_type IS DISTINCT FROM 'compliance_policy' OR a.resource_id IS DISTINCT FROM NEW.brand_id
  OR a.reason IS DISTINCT FROM NEW.reason OR a.after_json->>'brand_id' IS DISTINCT FROM NEW.brand_id::text
  OR (a.after_json->>'version')::bigint IS DISTINCT FROM NEW.version OR a.after_json->'config' IS DISTINCT FROM NEW.config
  OR (a.before_json->>'version')::bigint IS DISTINCT FROM NEW.version-1 THEN RAISE EXCEPTION 'matching compliance audit required'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_compliance_revision BEFORE INSERT OR UPDATE OR DELETE ON compliance_policy_revisions FOR EACH ROW EXECUTE FUNCTION guard_compliance_revision();
CREATE FUNCTION require_compliance_history() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM compliance_policy_revisions WHERE brand_id=NEW.brand_id AND version=NEW.version AND config=NEW.config) THEN RAISE EXCEPTION 'compliance policy requires immutable history'; END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER compliance_history_required AFTER INSERT OR UPDATE ON brand_compliance_policies DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_compliance_history();
CREATE FUNCTION initialize_brand_compliance() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO brand_compliance_policies(brand_id) VALUES(NEW.id);
 INSERT INTO compliance_policy_revisions(brand_id,version,config,reason) VALUES(NEW.id,1,default_compliance_config(),'Initial disabled compliance configuration');
 RETURN NEW;
END $$;
INSERT INTO brand_compliance_policies(brand_id) SELECT id FROM brands;
INSERT INTO compliance_policy_revisions(brand_id,version,config,reason) SELECT id,1,default_compliance_config(),'Initial disabled compliance configuration' FROM brands;
CREATE TRIGGER brand_compliance_initialized AFTER INSERT ON brands FOR EACH ROW EXECUTE FUNCTION initialize_brand_compliance();

CREATE FUNCTION compliance_stub_checks(v jsonb) RETURNS jsonb LANGUAGE sql IMMUTABLE AS $$
 SELECT jsonb_build_array(
 jsonb_build_object('check','age','enabled',v->'age_enabled','decision',CASE WHEN v->'age_enabled'='true'::jsonb THEN 'review' ELSE 'allow' END,'reason_code',CASE WHEN v->'age_enabled'='true'::jsonb THEN 'ADAPTER_NOT_CONFIGURED' ELSE 'CHECK_DISABLED' END),
 jsonb_build_object('check','region','enabled',v->'region_enabled','decision',CASE WHEN v->'region_enabled'='true'::jsonb THEN 'review' ELSE 'allow' END,'reason_code',CASE WHEN v->'region_enabled'='true'::jsonb THEN 'ADAPTER_NOT_CONFIGURED' ELSE 'CHECK_DISABLED' END),
 jsonb_build_object('check','identity','enabled',v->'identity_enabled','decision',CASE WHEN v->'identity_enabled'='true'::jsonb THEN 'review' ELSE 'allow' END,'reason_code',CASE WHEN v->'identity_enabled'='true'::jsonb THEN 'ADAPTER_NOT_CONFIGURED' ELSE 'CHECK_DISABLED' END))
$$;
CREATE TABLE compliance_decisions (
 id uuid PRIMARY KEY,brand_id uuid NOT NULL REFERENCES brands(id),policy_version bigint NOT NULL,config jsonb NOT NULL CHECK(valid_compliance_config(config) IS TRUE),
 operation text NOT NULL CHECK(operation IN('registration','betting','withdrawal')),decision text NOT NULL CHECK(decision IN('allow','review','deny','freeze')),
 checks jsonb NOT NULL CHECK(checks=compliance_stub_checks(config)),adapter_mode text NOT NULL CHECK(adapter_mode='stub'),
 created_by uuid NOT NULL REFERENCES admin_accounts(id),reason text NOT NULL CHECK(length(trim(reason))>0 AND octet_length(reason)<=500),
 audit_log_id uuid NOT NULL UNIQUE REFERENCES audit_logs(id),created_at timestamptz NOT NULL,
 FOREIGN KEY(brand_id,policy_version) REFERENCES compliance_policy_revisions(brand_id,version),
 CHECK(decision=CASE WHEN config->'age_enabled'='true'::jsonb OR config->'region_enabled'='true'::jsonb OR config->'identity_enabled'='true'::jsonb THEN 'review' ELSE 'allow' END)
);
CREATE INDEX compliance_decisions_page ON compliance_decisions(brand_id,created_at DESC,id DESC);
CREATE INDEX compliance_decisions_operation_page ON compliance_decisions(brand_id,operation,created_at DESC,id DESC);
CREATE FUNCTION guard_compliance_decision() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p brand_compliance_policies;a audit_logs;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'compliance decision immutable'; END IF;
 SELECT * INTO p FROM brand_compliance_policies WHERE brand_id=NEW.brand_id;
 SELECT * INTO a FROM audit_logs WHERE id=NEW.audit_log_id;
 IF NOT FOUND OR p.brand_id IS NULL OR p.version IS DISTINCT FROM NEW.policy_version OR p.config IS DISTINCT FROM NEW.config
 OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.actor_type IS DISTINCT FROM 'admin' OR a.actor_id IS DISTINCT FROM NEW.created_by
 OR a.action IS DISTINCT FROM 'compliance.check' OR a.resource_type IS DISTINCT FROM 'compliance_decision' OR a.resource_id IS DISTINCT FROM NEW.id
 OR a.reason IS DISTINCT FROM NEW.reason OR a.after_json->>'id' IS DISTINCT FROM NEW.id::text OR a.after_json->>'brand_id' IS DISTINCT FROM NEW.brand_id::text
 OR (a.after_json->>'policy_version')::bigint IS DISTINCT FROM NEW.policy_version OR a.after_json->'config' IS DISTINCT FROM NEW.config
 OR a.after_json->>'operation' IS DISTINCT FROM NEW.operation OR a.after_json->>'decision' IS DISTINCT FROM NEW.decision
 OR a.after_json->'checks' IS DISTINCT FROM NEW.checks OR a.after_json->>'adapter_mode' IS DISTINCT FROM NEW.adapter_mode
 OR (a.after_json->>'created_at')::timestamptz IS DISTINCT FROM NEW.created_at THEN RAISE EXCEPTION 'matching compliance decision audit required'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_compliance_decision BEFORE INSERT OR UPDATE OR DELETE ON compliance_decisions FOR EACH ROW EXECUTE FUNCTION guard_compliance_decision();
-- Keep restoration safe with an empty caller search_path, including nested calls.
DO $$ DECLARE f text; BEGIN
 FOREACH f IN ARRAY ARRAY['default_compliance_config()','valid_compliance_config(jsonb)','guard_compliance_policy()','guard_compliance_revision()','require_compliance_history()','initialize_brand_compliance()','compliance_stub_checks(jsonb)','guard_compliance_decision()'] LOOP
  EXECUTE format('ALTER FUNCTION %I.%s SET search_path=pg_catalog,%I,pg_temp',current_schema(),f,current_schema());
 END LOOP;
END $$;
INSERT INTO permissions(key) VALUES('compliance_policy.view.brand'),('compliance_policy.view.platform'),('compliance_policy.write.brand'),('compliance_check.view.brand'),('compliance_check.view.platform'),('compliance_check.run.brand') ON CONFLICT DO NOTHING;
INSERT INTO role_permissions(role_id,permission_key) SELECT r.id,p.key FROM roles r CROSS JOIN permissions p WHERE r.is_bootstrap AND
 ((r.brand_id IS NOT NULL AND p.key IN('compliance_policy.view.brand','compliance_policy.write.brand','compliance_check.view.brand','compliance_check.run.brand')) OR
 (r.brand_id IS NULL AND p.key IN('compliance_policy.view.platform','compliance_check.view.platform') AND EXISTS(SELECT 1 FROM admin_account_roles ar JOIN admin_accounts a ON a.id=ar.account_id WHERE ar.role_id=r.id AND a.is_super_admin))) ON CONFLICT DO NOTHING;
