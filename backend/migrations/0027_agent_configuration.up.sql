CREATE FUNCTION valid_agent_ratio(v text) RETURNS boolean LANGUAGE sql IMMUTABLE AS $$
 SELECT coalesce(v ~ '^(0|1|0\.[0-9]{0,5}[1-9])$',false)
$$;
CREATE FUNCTION valid_agent_config(v jsonb,brand_scope boolean) RETURNS boolean LANGUAGE plpgsql IMMUTABLE AS $$
BEGIN
 IF jsonb_typeof(v)<>'object' THEN RETURN false; END IF;
 IF brand_scope THEN
  RETURN (SELECT count(*)=5 FROM jsonb_object_keys(v)) AND v ?& ARRAY['enabled','max_depth','ratio_cap','mode','cycle']
   AND jsonb_typeof(v->'enabled')='boolean' AND jsonb_typeof(v->'max_depth')='number'
   AND v->>'max_depth' ~ '^[1-9][0-9]?$' AND (v->>'max_depth')::int<=32
   AND jsonb_typeof(v->'ratio_cap')='string' AND valid_agent_ratio(v->>'ratio_cap')
   AND v->>'mode' IN ('loss','turnover') AND v->>'cycle' IN ('weekly','monthly');
 END IF;
 RETURN (SELECT count(*)=4 FROM jsonb_object_keys(v)) AND v ?& ARRAY['ratio','mode','status','can_create_children']
  AND jsonb_typeof(v->'ratio')='string' AND valid_agent_ratio(v->>'ratio')
  AND (v->'mode'='null'::jsonb OR v->>'mode' IN ('loss','turnover'))
  AND v->>'status' IN ('active','disabled') AND jsonb_typeof(v->'can_create_children')='boolean';
EXCEPTION WHEN OTHERS THEN RETURN false;
END $$;
CREATE FUNCTION default_agent_policy() RETURNS jsonb LANGUAGE sql IMMUTABLE AS $$
 SELECT '{"enabled":false,"max_depth":5,"ratio_cap":"0","mode":"loss","cycle":"monthly"}'::jsonb
$$;
CREATE TABLE brand_agent_policies (
 brand_id uuid PRIMARY KEY REFERENCES brands(id),version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 config jsonb NOT NULL DEFAULT default_agent_policy() CHECK(valid_agent_config(config,true) IS TRUE),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE agent_nodes (
 id uuid PRIMARY KEY,brand_id uuid NOT NULL,member_id uuid NOT NULL,parent_id uuid,
 depth integer NOT NULL CHECK(depth BETWEEN 1 AND 32),path uuid[] NOT NULL,
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 config jsonb NOT NULL CHECK(valid_agent_config(config,false) IS TRUE),
 created_by uuid NOT NULL REFERENCES admin_accounts(id),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(brand_id,id),UNIQUE(brand_id,member_id),FOREIGN KEY(brand_id,member_id) REFERENCES brand_members(brand_id,id),
 FOREIGN KEY(brand_id,parent_id) REFERENCES agent_nodes(brand_id,id),CHECK(array_ndims(path)=1 AND cardinality(path)=depth AND path[depth]=id),
 CHECK((parent_id IS NULL)=(depth=1)),CHECK(parent_id IS DISTINCT FROM id)
);
CREATE INDEX agent_children_page ON agent_nodes(brand_id,parent_id,created_at,id);
CREATE TABLE agent_config_revisions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),brand_id uuid NOT NULL REFERENCES brands(id),agent_id uuid,
 version bigint NOT NULL CHECK(version>0),config jsonb NOT NULL CHECK(valid_agent_config(config,agent_id IS NULL) IS TRUE),
 actor_type text NOT NULL CHECK(actor_type IN ('system','admin','user')),actor_id uuid,
 reason text NOT NULL CHECK(length(trim(reason))>0 AND octet_length(reason)<=500),
 audit_log_id uuid REFERENCES audit_logs(id),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE NULLS NOT DISTINCT(brand_id,agent_id,version),FOREIGN KEY(brand_id,agent_id) REFERENCES agent_nodes(brand_id,id),
 CHECK((agent_id IS NULL AND version=1)=(actor_type='system' AND actor_id IS NULL AND audit_log_id IS NULL)),
 CHECK(actor_type='system' OR (actor_id IS NOT NULL AND audit_log_id IS NOT NULL))
);
CREATE INDEX agent_config_history ON agent_config_revisions(brand_id,agent_id,version DESC);
CREATE FUNCTION guard_agent_policy() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'agent policies cannot be deleted'; END IF;
 IF TG_OP='INSERT' THEN IF NEW.version<>1 OR NEW.config<>default_agent_policy() THEN RAISE EXCEPTION 'initial disabled agent policy required'; END IF;
 ELSE
  IF (to_jsonb(NEW)-ARRAY['version','config','updated_at']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['version','config','updated_at']) OR NEW.version<>OLD.version+1 THEN RAISE EXCEPTION 'agent policy identity/version immutable'; END IF;
  IF EXISTS(SELECT 1 FROM agent_nodes WHERE brand_id=NEW.brand_id AND ((config->>'ratio')::numeric>(NEW.config->>'ratio_cap')::numeric OR depth>(NEW.config->>'max_depth')::int)) THEN RAISE EXCEPTION 'existing agents exceed new policy'; END IF;
 END IF;
 NEW.updated_at:=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER guarded_agent_policy BEFORE INSERT OR UPDATE OR DELETE ON brand_agent_policies FOR EACH ROW EXECUTE FUNCTION guard_agent_policy();
CREATE FUNCTION guard_agent_node() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p jsonb; parent agent_nodes;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'agent identity cannot be deleted'; END IF;
 SELECT config INTO p FROM brand_agent_policies WHERE brand_id=NEW.brand_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION 'agent policy missing'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.version<>1 OR p->'enabled'<>'true'::jsonb OR NOT EXISTS(SELECT 1 FROM brand_members WHERE brand_id=NEW.brand_id AND id=NEW.member_id AND status='normal') THEN RAISE EXCEPTION 'agent admission invalid'; END IF;
 ELSE
  IF (to_jsonb(NEW)-ARRAY['version','config','updated_at']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['version','config','updated_at']) OR NEW.version<>OLD.version+1 THEN RAISE EXCEPTION 'agent identity/path/parent immutable'; END IF;
 END IF;
 IF NEW.depth>(p->>'max_depth')::int OR (NEW.config->>'ratio')::numeric>(p->>'ratio_cap')::numeric THEN RAISE EXCEPTION 'agent exceeds policy limit'; END IF;
 IF NEW.parent_id IS NULL THEN
  IF NEW.depth<>1 OR NEW.path<>ARRAY[NEW.id] THEN RAISE EXCEPTION 'invalid root lineage'; END IF;
 ELSE
  SELECT * INTO parent FROM agent_nodes WHERE brand_id=NEW.brand_id AND id=NEW.parent_id;
  IF NOT FOUND OR NEW.depth<>parent.depth+1 OR NEW.path<>array_append(parent.path,NEW.id) OR (NEW.config->>'ratio')::numeric>(parent.config->>'ratio')::numeric THEN RAISE EXCEPTION 'invalid child lineage or ratio'; END IF;
  IF TG_OP='INSERT' AND (parent.config->>'status'<>'active' OR parent.config->'can_create_children'<>'true'::jsonb OR EXISTS(SELECT 1 FROM agent_nodes WHERE brand_id=NEW.brand_id AND id=ANY(parent.path) AND config->>'status'<>'active')) THEN RAISE EXCEPTION 'parent cannot develop children'; END IF;
 END IF;
 IF EXISTS(SELECT 1 FROM agent_nodes WHERE brand_id=NEW.brand_id AND parent_id=NEW.id AND (config->>'ratio')::numeric>(NEW.config->>'ratio')::numeric) THEN RAISE EXCEPTION 'existing child exceeds new ratio'; END IF;
 NEW.updated_at:=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER guarded_agent_node BEFORE INSERT OR UPDATE OR DELETE ON agent_nodes FOR EACH ROW EXECUTE FUNCTION guard_agent_node();
CREATE FUNCTION guard_agent_revision() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE v bigint; cfg jsonb; a audit_logs;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'agent configuration history immutable'; END IF;
 IF NEW.agent_id IS NULL THEN SELECT version,config INTO v,cfg FROM brand_agent_policies WHERE brand_id=NEW.brand_id;
 ELSE SELECT version,config INTO v,cfg FROM agent_nodes WHERE brand_id=NEW.brand_id AND id=NEW.agent_id; END IF;
 IF NOT FOUND OR (NEW.version=1 AND (v<>1 OR cfg<>NEW.config)) OR (NEW.version>1 AND v<>NEW.version-1) THEN RAISE EXCEPTION 'agent revision must belong to current/next version'; END IF;
 IF NEW.actor_type<>'system' THEN
  SELECT * INTO a FROM audit_logs WHERE id=NEW.audit_log_id;
  IF NOT FOUND OR a.brand_id<>NEW.brand_id OR a.actor_type<>NEW.actor_type OR a.actor_id IS DISTINCT FROM NEW.actor_id OR
   a.resource_type<>(CASE WHEN NEW.agent_id IS NULL THEN 'agent_policy' ELSE 'agent_node' END) OR
   a.resource_id IS DISTINCT FROM coalesce(NEW.agent_id,NEW.brand_id) OR a.reason<>NEW.reason OR
   (a.after_json->>'version') IS DISTINCT FROM NEW.version::text OR (a.after_json->'config') IS DISTINCT FROM NEW.config THEN RAISE EXCEPTION 'agent revision needs matching audit'; END IF;
 END IF;
 NEW.created_at:=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER guarded_agent_revision BEFORE INSERT OR UPDATE OR DELETE ON agent_config_revisions FOR EACH ROW EXECUTE FUNCTION guard_agent_revision();
CREATE FUNCTION require_agent_revision() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE node uuid:=NULLIF(to_jsonb(NEW)->>'id','')::uuid;
BEGIN
 IF NOT EXISTS(SELECT 1 FROM agent_config_revisions WHERE brand_id=NEW.brand_id AND agent_id IS NOT DISTINCT FROM node AND version=NEW.version AND config=NEW.config) THEN RAISE EXCEPTION 'agent current version needs revision'; END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER agent_policy_revision_required AFTER INSERT OR UPDATE ON brand_agent_policies DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_agent_revision();
CREATE CONSTRAINT TRIGGER agent_node_revision_required AFTER INSERT OR UPDATE ON agent_nodes DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_agent_revision();
CREATE FUNCTION agent_revision_committed() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE v bigint;
BEGIN
 IF NEW.agent_id IS NULL THEN SELECT version INTO v FROM brand_agent_policies WHERE brand_id=NEW.brand_id;
 ELSE SELECT version INTO v FROM agent_nodes WHERE brand_id=NEW.brand_id AND id=NEW.agent_id; END IF;
 IF NOT FOUND OR v<NEW.version THEN RAISE EXCEPTION 'orphan agent revision'; END IF;RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER agent_revision_committed AFTER INSERT ON agent_config_revisions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION agent_revision_committed();
CREATE FUNCTION initialize_agent_policy_revision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO agent_config_revisions(brand_id,version,config,actor_type,reason) VALUES(NEW.brand_id,1,NEW.config,'system','Initial disabled agent policy');RETURN NULL;
END $$;
CREATE TRIGGER initial_agent_policy_revision AFTER INSERT ON brand_agent_policies FOR EACH ROW EXECUTE FUNCTION initialize_agent_policy_revision();
INSERT INTO brand_agent_policies(brand_id) SELECT id FROM brands;
CREATE FUNCTION initialize_agent_policy() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN INSERT INTO brand_agent_policies(brand_id) VALUES(NEW.id);RETURN NULL;END $$;
CREATE TRIGGER brand_agent_policy AFTER INSERT ON brands FOR EACH ROW EXECUTE FUNCTION initialize_agent_policy();
INSERT INTO permissions(key) VALUES('agent.view.brand'),('agent.view.platform'),('agent.write.brand'),('agent_policy.view.brand'),('agent_policy.view.platform'),('agent_policy.write.brand') ON CONFLICT DO NOTHING;
INSERT INTO role_permissions(role_id,permission_key) SELECT r.id,p.key FROM roles r CROSS JOIN permissions p WHERE r.is_bootstrap AND
 ((r.brand_id IS NOT NULL AND p.key IN ('agent.view.brand','agent.write.brand','agent_policy.view.brand','agent_policy.write.brand')) OR
 (r.brand_id IS NULL AND p.key IN ('agent.view.platform','agent_policy.view.platform') AND EXISTS(SELECT 1 FROM admin_account_roles ar JOIN admin_accounts a ON a.id=ar.account_id WHERE ar.role_id=r.id AND a.is_super_admin))) ON CONFLICT DO NOTHING;
