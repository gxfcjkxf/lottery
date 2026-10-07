-- Financial policy is separate from agency admission/configuration. Installation
-- neither authorizes payouts nor invents a policy for old bets.
CREATE FUNCTION valid_commission_policy(v jsonb) RETURNS boolean LANGUAGE plpgsql STABLE AS $$
DECLARE c jsonb;
BEGIN
 IF jsonb_typeof(v)<>'object' OR (SELECT count(*) FROM jsonb_object_keys(v))<>3 OR
  NOT v ?& ARRAY['enabled','calendar','payout_mode'] OR jsonb_typeof(v->'enabled')<>'boolean' OR
  jsonb_typeof(v->'payout_mode')<>'string' OR v->>'payout_mode' NOT IN ('manual','automatic') THEN RETURN false; END IF;
 c:=v->'calendar';
 IF c='null'::jsonb THEN RETURN v->'enabled'='false'::jsonb; END IF;
 IF jsonb_typeof(c)<>'object' OR (SELECT count(*) FROM jsonb_object_keys(c))<>6 OR
  NOT c ?& ARRAY['timezone','cycle','boundary_time','weekday','month_day','short_month'] OR
  jsonb_typeof(c->'timezone')<>'string' OR c->>'timezone'='Local' OR
  NOT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=c->>'timezone') OR
  jsonb_typeof(c->'boundary_time')<>'string' OR c->>'boundary_time' !~ '^([01][0-9]|2[0-3]):[0-5][0-9]:[0-5][0-9]$' OR
  jsonb_typeof(c->'cycle')<>'string' OR jsonb_typeof(c->'short_month')<>'string' THEN RETURN false; END IF;
 IF c->>'cycle'='weekly' THEN
  RETURN c->'month_day'='null'::jsonb AND c->>'short_month'='' AND
   jsonb_typeof(c->'weekday')='number' AND c->>'weekday' ~ '^[0-6]$';
 ELSIF c->>'cycle'='monthly' THEN
  RETURN c->'weekday'='null'::jsonb AND c->>'short_month' IN ('last_day','skip') AND
   jsonb_typeof(c->'month_day')='number' AND c->>'month_day' ~ '^[1-9][0-9]?$' AND (c->>'month_day')::int<=31;
 END IF;
 RETURN false;
EXCEPTION WHEN OTHERS THEN RETURN false;
END $$;
CREATE FUNCTION default_commission_policy() RETURNS jsonb LANGUAGE sql IMMUTABLE AS $$
 SELECT '{"enabled":false,"calendar":null,"payout_mode":"manual"}'::jsonb
$$;
CREATE TABLE brand_commission_policies (
 brand_id uuid PRIMARY KEY REFERENCES brands(id),version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 config jsonb NOT NULL DEFAULT default_commission_policy() CHECK(valid_commission_policy(config) IS TRUE),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),updated_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE commission_policy_revisions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),brand_id uuid NOT NULL REFERENCES brands(id),
 version bigint NOT NULL CHECK(version>0),config jsonb NOT NULL CHECK(valid_commission_policy(config) IS TRUE),
 changed_by uuid REFERENCES admin_accounts(id),reason text NOT NULL CHECK(length(trim(reason))>0 AND octet_length(reason)<=500),
 audit_log_id uuid REFERENCES audit_logs(id),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(brand_id,version),UNIQUE(brand_id,id),
 CHECK((version=1)=(changed_by IS NULL AND audit_log_id IS NULL)),
 CHECK(version=1 OR (changed_by IS NOT NULL AND audit_log_id IS NOT NULL))
);
CREATE FUNCTION guard_commission_policy() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE agency jsonb;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'commission policy cannot be deleted'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.version<>1 OR NEW.config<>default_commission_policy() THEN RAISE EXCEPTION 'initial disabled commission policy required'; END IF;
 ELSE
  IF (to_jsonb(NEW)-ARRAY['version','config','updated_at']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['version','config','updated_at']) OR
   NEW.version<>OLD.version+1 THEN RAISE EXCEPTION 'commission policy identity/version immutable'; END IF;
 END IF;
 SELECT config INTO agency FROM brand_agent_policies WHERE brand_id=NEW.brand_id FOR SHARE;
 IF NEW.config->'enabled'='true'::jsonb AND
  (NOT FOUND OR agency->'enabled'<>'true'::jsonb OR NEW.config->'calendar'->>'cycle' IS DISTINCT FROM agency->>'cycle') THEN
  RAISE EXCEPTION 'commission policy needs matching enabled agency policy';
 END IF;
 NEW.updated_at:=clock_timestamp(); RETURN NEW;
END $$;
CREATE TRIGGER guarded_commission_policy BEFORE INSERT OR UPDATE OR DELETE ON brand_commission_policies
 FOR EACH ROW EXECUTE FUNCTION guard_commission_policy();
CREATE FUNCTION guard_commission_policy_revision() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p brand_commission_policies; a audit_logs;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'commission policy history immutable'; END IF;
 SELECT * INTO p FROM brand_commission_policies WHERE brand_id=NEW.brand_id;
 IF NOT FOUND OR (NEW.version=1 AND (p.version<>1 OR p.config<>NEW.config)) OR
  (NEW.version>1 AND p.version<>NEW.version-1) THEN RAISE EXCEPTION 'commission revision must belong to current/next version'; END IF;
 IF NEW.version>1 THEN
  SELECT * INTO a FROM audit_logs WHERE id=NEW.audit_log_id;
  IF NOT FOUND OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.actor_type<>'admin' OR a.actor_id IS DISTINCT FROM NEW.changed_by OR
   a.action<>'commission.policy.update' OR a.resource_type<>'commission_policy' OR a.resource_id IS DISTINCT FROM NEW.brand_id OR
   a.reason<>NEW.reason OR (a.after_json->>'version') IS DISTINCT FROM NEW.version::text OR
   a.after_json->'config' IS DISTINCT FROM NEW.config OR a.after_json->>'revision_id' IS DISTINCT FROM NEW.id::text THEN
   RAISE EXCEPTION 'commission policy revision needs matching audit';
  END IF;
 END IF;
 NEW.created_at:=clock_timestamp(); RETURN NEW;
END $$;
CREATE TRIGGER guarded_commission_revision BEFORE INSERT OR UPDATE OR DELETE ON commission_policy_revisions
 FOR EACH ROW EXECUTE FUNCTION guard_commission_policy_revision();
CREATE FUNCTION require_commission_policy_revision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM commission_policy_revisions WHERE brand_id=NEW.brand_id AND version=NEW.version AND config=NEW.config) THEN
  RAISE EXCEPTION 'commission current version needs revision'; END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER commission_policy_revision_required AFTER INSERT OR UPDATE ON brand_commission_policies
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_commission_policy_revision();
CREATE FUNCTION commission_policy_revision_committed() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM brand_commission_policies WHERE brand_id=NEW.brand_id AND version>=NEW.version) THEN
  RAISE EXCEPTION 'orphan commission policy revision'; END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER commission_revision_committed AFTER INSERT ON commission_policy_revisions
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION commission_policy_revision_committed();
CREATE FUNCTION initialize_commission_policy_revision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO commission_policy_revisions(brand_id,version,config,reason) VALUES(NEW.brand_id,1,NEW.config,'Initial disabled commission financial policy'); RETURN NULL;
END $$;
CREATE TRIGGER initial_commission_revision AFTER INSERT ON brand_commission_policies
 FOR EACH ROW EXECUTE FUNCTION initialize_commission_policy_revision();
INSERT INTO brand_commission_policies(brand_id) SELECT id FROM brands;
CREATE FUNCTION initialize_commission_policy() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN INSERT INTO brand_commission_policies(brand_id) VALUES(NEW.id); RETURN NULL; END $$;
CREATE TRIGGER brand_commission_policy AFTER INSERT ON brands FOR EACH ROW EXECUTE FUNCTION initialize_commission_policy();

-- Admission changes cannot invalidate an enabled financial calendar. Normal
-- APIs prelock financial -> agency; direct SQL also fails closed on drift.
CREATE FUNCTION guard_agency_commission_calendar() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE c jsonb;
BEGIN
 SELECT config INTO c FROM brand_commission_policies WHERE brand_id=NEW.brand_id FOR SHARE;
 IF c->'enabled'='true'::jsonb AND
  (NEW.config->'enabled'<>'true'::jsonb OR NEW.config->>'cycle' IS DISTINCT FROM c->'calendar'->>'cycle') THEN
  RAISE EXCEPTION 'disable financial commission policy before changing agency cycle';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_agency_commission_calendar BEFORE UPDATE OF config ON brand_agent_policies
 FOR EACH ROW EXECUTE FUNCTION guard_agency_commission_calendar();

ALTER TABLE bet_orders ADD COLUMN commission_rule_snapshot jsonb;
ALTER TABLE bet_orders ADD CONSTRAINT bet_commission_snapshot_object CHECK(
 commission_rule_snapshot IS NULL OR jsonb_typeof(commission_rule_snapshot)='object');
CREATE FUNCTION capture_bet_commission_snapshot() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE f brand_commission_policies; p brand_agent_policies; m brand_members; n agent_nodes;
 financial_revision uuid; agency_revision uuid; chain jsonb; expected_count int; captured_count int;
BEGIN
 IF TG_OP='UPDATE' THEN
  IF NEW.commission_rule_snapshot IS DISTINCT FROM OLD.commission_rule_snapshot THEN RAISE EXCEPTION 'bet commission snapshot immutable'; END IF;
  RETURN NEW;
 END IF;
 SELECT * INTO f FROM brand_commission_policies WHERE brand_id=NEW.brand_id FOR SHARE;
 IF NOT FOUND THEN RAISE EXCEPTION 'commission financial policy missing'; END IF;
 SELECT * INTO p FROM brand_agent_policies WHERE brand_id=NEW.brand_id FOR SHARE;
 IF NOT FOUND THEN RAISE EXCEPTION 'commission agency policy missing'; END IF;
 SELECT id INTO financial_revision FROM commission_policy_revisions WHERE brand_id=NEW.brand_id AND version=f.version AND config=f.config AND created_at<=NEW.placed_at;
 IF NOT FOUND THEN RAISE EXCEPTION 'commission financial revision missing'; END IF;
 SELECT id INTO agency_revision FROM agent_config_revisions WHERE brand_id=NEW.brand_id AND agent_id IS NULL AND version=p.version AND config=p.config AND created_at<=NEW.placed_at;
 IF NOT FOUND THEN RAISE EXCEPTION 'commission agency revision missing'; END IF;
 IF f.config->'enabled'='true'::jsonb AND (p.config->'enabled'<>'true'::jsonb OR f.config->'calendar'->>'cycle' IS DISTINCT FROM p.config->>'cycle') THEN
  RAISE EXCEPTION 'commission calendar inconsistent'; END IF;
 SELECT * INTO m FROM brand_members WHERE brand_id=NEW.brand_id AND id=NEW.brand_member_id;
 IF NOT FOUND THEN RAISE EXCEPTION 'commission order member missing'; END IF;
 chain:='[]'::jsonb;
 IF m.attribution_snapshot->>'agent_id' IS NOT NULL THEN
  SELECT * INTO n FROM agent_nodes WHERE brand_id=NEW.brand_id AND id=(m.attribution_snapshot->>'agent_id')::uuid;
  IF NOT FOUND THEN RAISE EXCEPTION 'commission agent lineage missing'; END IF;
  expected_count:=cardinality(n.path);
  SELECT count(*),jsonb_agg(jsonb_build_object('id',a.id,'member_id',a.member_id,'parent_id',a.parent_id,'depth',a.depth,
   'revision_id',r.id,'version',a.version::text,'config',a.config,
   'effective_mode',agent_effective_mode_for_path(a.brand_id,a.path,NULL,NULL,p.config->>'mode')) ORDER BY a.depth)
  INTO captured_count,chain FROM agent_nodes a JOIN agent_config_revisions r ON r.brand_id=a.brand_id AND r.agent_id=a.id AND r.version=a.version AND r.config=a.config AND r.created_at<=NEW.placed_at
   WHERE a.brand_id=NEW.brand_id AND a.id=ANY(n.path);
  IF captured_count<>expected_count THEN RAISE EXCEPTION 'commission agent revision missing'; END IF;
  IF EXISTS(SELECT 1 FROM agent_nodes a JOIN agent_nodes parent ON parent.brand_id=a.brand_id AND parent.id=a.parent_id
   WHERE a.brand_id=NEW.brand_id AND a.id=ANY(n.path) AND
    agent_effective_mode_for_path(a.brand_id,a.path,NULL,NULL,p.config->>'mode') IS DISTINCT FROM
    agent_effective_mode_for_path(parent.brand_id,parent.path,NULL,NULL,p.config->>'mode')) THEN RAISE EXCEPTION 'commission chain mode mismatch'; END IF;
 END IF;
 NEW.commission_rule_snapshot:=jsonb_build_object('schema_version',1,'brand_id',NEW.brand_id,'member_id',NEW.brand_member_id,
  'captured_at',NEW.placed_at,'financial_policy',jsonb_build_object('revision_id',financial_revision,'version',f.version::text,'config',f.config),
  'agency_policy',jsonb_build_object('revision_id',agency_revision,'version',p.version::text,'config',p.config),'agent_path',chain);
 RETURN NEW;
END $$;
CREATE TRIGGER capture_bet_commission_snapshot BEFORE INSERT OR UPDATE ON bet_orders
 FOR EACH ROW EXECUTE FUNCTION capture_bet_commission_snapshot();

INSERT INTO permissions(key) VALUES('commission_policy.view.brand'),('commission_policy.view.platform'),('commission_policy.write.brand') ON CONFLICT DO NOTHING;
INSERT INTO role_permissions(role_id,permission_key) SELECT r.id,p.key FROM roles r CROSS JOIN permissions p WHERE r.is_bootstrap AND
 ((r.brand_id IS NOT NULL AND p.key IN ('commission_policy.view.brand','commission_policy.write.brand')) OR
 (r.brand_id IS NULL AND p.key='commission_policy.view.platform' AND EXISTS(SELECT 1 FROM admin_account_roles ar JOIN admin_accounts a ON a.id=ar.account_id WHERE ar.role_id=r.id AND a.is_super_admin))) ON CONFLICT DO NOTHING;
DO $$
DECLARE app_schema text:=current_schema(); f record;
BEGIN
 IF app_schema IS NULL THEN RAISE EXCEPTION 'application schema required'; END IF;
 FOR f IN SELECT p.proname,pg_get_function_identity_arguments(p.oid) AS args FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace
  WHERE n.nspname=app_schema AND p.proname IN ('valid_commission_policy','default_commission_policy','guard_commission_policy',
   'guard_commission_policy_revision','require_commission_policy_revision','commission_policy_revision_committed',
   'initialize_commission_policy_revision','initialize_commission_policy','guard_agency_commission_calendar','capture_bet_commission_snapshot')
 LOOP EXECUTE format('ALTER FUNCTION %I.%I(%s) SET search_path = pg_catalog, %I, pg_temp',app_schema,f.proname,f.args,app_schema); END LOOP;
END $$;
