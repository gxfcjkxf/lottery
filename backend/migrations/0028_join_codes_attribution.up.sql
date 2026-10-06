-- Additive: retain historical joins without inventing codes, trees or amounts.
CREATE TABLE join_codes (
 id uuid PRIMARY KEY,brand_id uuid NOT NULL REFERENCES brands(id),kind text NOT NULL CHECK(kind IN ('agent','referral')),
 code text NOT NULL CHECK(code ~ '^[A-F0-9]{24}$'),owner_member_id uuid NOT NULL,agent_id uuid,
 status text NOT NULL DEFAULT 'active' CHECK(status IN ('active','disabled')),
 starts_at timestamptz,expires_at timestamptz,version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 created_by uuid NOT NULL REFERENCES admin_accounts(id),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(brand_id,id),UNIQUE(brand_id,code),FOREIGN KEY(brand_id,owner_member_id)REFERENCES brand_members(brand_id,id),
 FOREIGN KEY(brand_id,agent_id)REFERENCES agent_nodes(brand_id,id),CHECK((kind='agent')=(agent_id IS NOT NULL)),
 CHECK(starts_at IS NULL OR expires_at IS NULL OR starts_at<expires_at)
);
CREATE INDEX join_codes_owner_page ON join_codes(brand_id,owner_member_id,created_at,id);
CREATE TABLE join_code_revisions (
 id uuid PRIMARY KEY,brand_id uuid NOT NULL,code_id uuid NOT NULL,version bigint NOT NULL CHECK(version>0),
 status text NOT NULL CHECK(status IN ('active','disabled')),starts_at timestamptz,expires_at timestamptz,
 actor_id uuid NOT NULL REFERENCES admin_accounts(id),reason text NOT NULL CHECK(length(trim(reason))>0 AND octet_length(reason)<=500),
 audit_log_id uuid NOT NULL REFERENCES audit_logs(id),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(brand_id,code_id,version),FOREIGN KEY(brand_id,code_id)REFERENCES join_codes(brand_id,id)
);
CREATE FUNCTION join_code_usable_at(c join_codes,at_time timestamptz)RETURNS boolean LANGUAGE sql VOLATILE AS $$
 SELECT c.status='active' AND (c.starts_at IS NULL OR c.starts_at<=at_time) AND (c.expires_at IS NULL OR c.expires_at>at_time)
 AND EXISTS(SELECT 1 FROM brand_members m JOIN global_users u ON u.id=m.global_user_id JOIN brands b ON b.id=m.brand_id WHERE m.brand_id=c.brand_id AND m.id=c.owner_member_id AND m.status='normal' AND u.status='active' AND b.status<>'disabled')
 AND (c.kind='referral' OR EXISTS(SELECT 1 FROM agent_nodes n JOIN brand_agent_policies p ON p.brand_id=n.brand_id WHERE n.brand_id=c.brand_id AND n.id=c.agent_id AND n.member_id=c.owner_member_id AND p.config->'enabled'='true'::jsonb AND NOT EXISTS(SELECT 1 FROM agent_nodes a WHERE a.brand_id=n.brand_id AND a.id=ANY(n.path) AND a.config->>'status'<>'active')))
$$;
CREATE FUNCTION join_code_usable(c join_codes)RETURNS boolean LANGUAGE sql VOLATILE AS $$ SELECT join_code_usable_at(c,clock_timestamp()) $$;
CREATE FUNCTION guard_join_code()RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p jsonb;source_user uuid;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'join codes cannot be deleted';END IF;
 PERFORM 1 FROM brand_agent_policies WHERE brand_id=NEW.brand_id FOR SHARE;
 IF TG_OP='INSERT' THEN
  IF NEW.version<>1 THEN RAISE EXCEPTION 'initial code version must be one';END IF;
 ELSE
  IF (to_jsonb(NEW)-ARRAY['version','status','starts_at','expires_at','updated_at']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['version','status','starts_at','expires_at','updated_at']) OR NEW.version<>OLD.version+1 THEN RAISE EXCEPTION 'join-code identity/version immutable';END IF;
 END IF;
 IF NEW.kind='agent' AND NOT EXISTS(SELECT 1 FROM agent_nodes WHERE brand_id=NEW.brand_id AND id=NEW.agent_id AND member_id=NEW.owner_member_id) THEN RAISE EXCEPTION 'agent code owner mismatch' USING ERRCODE='23514',CONSTRAINT='join_code_unavailable';END IF;
 -- Creating/enabling is not permitted for a disabled source. Expiration itself
 -- can be in the past; it simply prevents use, rather than rewriting history.
 IF NEW.status='active' THEN
  SELECT global_user_id INTO source_user FROM brand_members WHERE brand_id=NEW.brand_id AND id=NEW.owner_member_id;
  PERFORM 1 FROM global_users WHERE id=source_user FOR SHARE;
  PERFORM 1 FROM brand_members m JOIN global_users u ON u.id=m.global_user_id JOIN brands b ON b.id=m.brand_id WHERE m.brand_id=NEW.brand_id AND m.id=NEW.owner_member_id AND m.status='normal' AND u.status='active' AND b.status<>'disabled' FOR SHARE OF m;
  IF NOT FOUND THEN RAISE EXCEPTION 'join-code source unavailable' USING ERRCODE='23514',CONSTRAINT='join_code_unavailable';END IF;
  IF NEW.kind='agent' THEN
   SELECT config INTO p FROM brand_agent_policies WHERE brand_id=NEW.brand_id;
   IF p->'enabled'<>'true'::jsonb OR NOT EXISTS(SELECT 1 FROM agent_nodes n WHERE n.brand_id=NEW.brand_id AND n.id=NEW.agent_id AND n.member_id=NEW.owner_member_id AND NOT EXISTS(SELECT 1 FROM agent_nodes a WHERE a.brand_id=n.brand_id AND a.id=ANY(n.path) AND a.config->>'status'<>'active')) THEN RAISE EXCEPTION 'join-code source unavailable' USING ERRCODE='23514',CONSTRAINT='join_code_unavailable';END IF;
  END IF;
 END IF;
 NEW.updated_at:=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER guarded_join_code BEFORE INSERT OR UPDATE OR DELETE ON join_codes FOR EACH ROW EXECUTE FUNCTION guard_join_code();
CREATE FUNCTION guard_join_code_revision()RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE c join_codes;a audit_logs;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'join-code history immutable';END IF;
 SELECT * INTO c FROM join_codes WHERE brand_id=NEW.brand_id AND id=NEW.code_id;
 IF NOT FOUND OR ROW(c.version,c.status,c.starts_at,c.expires_at) IS DISTINCT FROM ROW(NEW.version,NEW.status,NEW.starts_at,NEW.expires_at) THEN RAISE EXCEPTION 'join-code history does not match current version';END IF;
 SELECT * INTO a FROM audit_logs WHERE id=NEW.audit_log_id;
 IF NOT FOUND OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.actor_type<>'admin' OR a.actor_id IS DISTINCT FROM NEW.actor_id OR a.resource_type<>'join_code' OR a.resource_id IS DISTINCT FROM NEW.code_id OR a.reason<>NEW.reason
 OR a.action IS DISTINCT FROM (CASE WHEN NEW.version=1 THEN 'join_code.create' ELSE 'join_code.update' END)
 OR (NEW.version=1 AND NEW.actor_id IS DISTINCT FROM c.created_by)
 OR a.after_json->>'id' IS DISTINCT FROM c.id::text OR a.after_json->>'brand_id' IS DISTINCT FROM c.brand_id::text OR a.after_json->>'kind' IS DISTINCT FROM c.kind OR a.after_json->>'code' IS DISTINCT FROM c.code OR a.after_json->>'owner_member_id' IS DISTINCT FROM c.owner_member_id::text OR (a.after_json->>'agent_id')::uuid IS DISTINCT FROM c.agent_id
 OR (a.after_json->>'version')::bigint IS DISTINCT FROM NEW.version OR a.after_json->>'status' IS DISTINCT FROM NEW.status
 OR (a.after_json->>'starts_at')::timestamptz IS DISTINCT FROM NEW.starts_at OR (a.after_json->>'expires_at')::timestamptz IS DISTINCT FROM NEW.expires_at THEN RAISE EXCEPTION 'join-code audit witness missing';END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER immutable_join_code_history BEFORE INSERT OR UPDATE OR DELETE ON join_code_revisions FOR EACH ROW EXECUTE FUNCTION guard_join_code_revision();
CREATE FUNCTION require_join_code_revision()RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE c join_codes;k uuid;
BEGIN
 IF TG_TABLE_NAME='join_codes' THEN k:=NEW.id;ELSE k:=NEW.code_id;END IF;
 SELECT * INTO c FROM join_codes WHERE id=k;
 IF NOT FOUND OR NOT EXISTS(SELECT 1 FROM join_code_revisions h WHERE h.brand_id=c.brand_id AND h.code_id=c.id AND h.version=c.version AND ROW(h.status,h.starts_at,h.expires_at) IS NOT DISTINCT FROM ROW(c.status,c.starts_at,c.expires_at)) THEN RAISE EXCEPTION 'join-code current version requires audited history';END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER join_code_revision_required AFTER INSERT OR UPDATE ON join_codes DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_join_code_revision();
CREATE CONSTRAINT TRIGGER join_code_history_complete AFTER INSERT ON join_code_revisions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_join_code_revision();

UPDATE brand_members SET attribution_snapshot=jsonb_build_object('schema_version',1,'legacy',true,'join_method',join_method,'join_domain',join_domain,'joined_at',joined_at,'code_id',NULL,'code_version',NULL,'code',NULL,'code_kind',NULL,'owner_member_id',NULL,'agent_id',NULL,'referrer_member_id',NULL,'agent_path','[]'::jsonb,'legacy_payload',attribution_snapshot);
CREATE FUNCTION capture_member_attribution()RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE c join_codes;n agent_nodes;selected_kind text;selected_code text;chain jsonb;policy brand_agent_policies;source_user uuid;join_time timestamptz;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'member provenance cannot be deleted';END IF;
 IF TG_OP='UPDATE' THEN
  IF ROW(NEW.id,NEW.brand_id,NEW.global_user_id,NEW.join_method,NEW.join_domain,NEW.joined_at,NEW.created_by,NEW.attribution_snapshot) IS DISTINCT FROM ROW(OLD.id,OLD.brand_id,OLD.global_user_id,OLD.join_method,OLD.join_domain,OLD.joined_at,OLD.created_by,OLD.attribution_snapshot) THEN RAISE EXCEPTION 'member provenance immutable';END IF;
  RETURN NEW;
 END IF;
 IF NEW.attribution_snapshot<>'{}'::jsonb THEN
  IF jsonb_typeof(NEW.attribution_snapshot)<>'object' THEN RAISE EXCEPTION 'invalid join-code selection' USING ERRCODE='23514',CONSTRAINT='join_code_unavailable';END IF;
  IF (SELECT count(*) FROM jsonb_object_keys(NEW.attribution_snapshot))<>2 THEN RAISE EXCEPTION 'invalid join-code selection' USING ERRCODE='23514',CONSTRAINT='join_code_unavailable';END IF;
  selected_kind:=NEW.attribution_snapshot->>'kind';selected_code:=NEW.attribution_snapshot->>'code';
  IF NEW.join_method='operator' AND NEW.created_by IS NULL THEN RAISE EXCEPTION 'operator code attribution requires creator' USING ERRCODE='23514',CONSTRAINT='join_code_unavailable';END IF;
  IF selected_kind NOT IN ('agent','referral') OR selected_code IS NULL OR selected_code!~'^[A-F0-9]{24}$' OR (NEW.join_method<>'operator' AND NEW.join_method<>selected_kind||'_code') THEN RAISE EXCEPTION 'invalid join-code selection' USING ERRCODE='23514',CONSTRAINT='join_code_unavailable';END IF;
  SELECT * INTO policy FROM brand_agent_policies WHERE brand_id=NEW.brand_id FOR SHARE;
  SELECT * INTO c FROM join_codes WHERE brand_id=NEW.brand_id AND join_codes.code=selected_code AND join_codes.kind=selected_kind FOR SHARE;
  IF NOT FOUND OR NOT join_code_usable(c) OR EXISTS(SELECT 1 FROM brand_members WHERE brand_id=NEW.brand_id AND id=c.owner_member_id AND global_user_id=NEW.global_user_id) THEN RAISE EXCEPTION 'join code unavailable' USING ERRCODE='23514',CONSTRAINT='join_code_unavailable';END IF;
  SELECT global_user_id INTO source_user FROM brand_members WHERE brand_id=NEW.brand_id AND id=c.owner_member_id;
  PERFORM 1 FROM global_users WHERE id=source_user FOR SHARE;
  PERFORM 1 FROM brand_members WHERE brand_id=NEW.brand_id AND id=c.owner_member_id FOR SHARE;
  join_time:=clock_timestamp();
  IF NOT join_code_usable_at(c,join_time) THEN RAISE EXCEPTION 'join code unavailable' USING ERRCODE='23514',CONSTRAINT='join_code_unavailable';END IF;
  IF selected_kind='agent' THEN
   SELECT * INTO n FROM agent_nodes WHERE brand_id=NEW.brand_id AND id=c.agent_id;
   SELECT jsonb_agg(jsonb_build_object('id',a.id,'version',a.version,'config',a.config) ORDER BY a.depth) INTO chain FROM agent_nodes a WHERE a.brand_id=NEW.brand_id AND a.id=ANY(n.path);
  END IF;
 ELSIF NEW.join_method NOT IN ('domain','operator') THEN RAISE EXCEPTION 'join code required' USING ERRCODE='23514',CONSTRAINT='join_code_unavailable';
 END IF;
 NEW.joined_at:=coalesce(join_time,clock_timestamp());
 NEW.attribution_snapshot:=jsonb_build_object('schema_version',1,'legacy',false,'join_method',NEW.join_method,'join_domain',NEW.join_domain,'joined_at',NEW.joined_at,'code_id',c.id,'code_version',c.version,'code',c.code,'code_kind',c.kind,'owner_member_id',c.owner_member_id,'agent_id',c.agent_id,'referrer_member_id',CASE WHEN c.kind='referral' THEN c.owner_member_id ELSE NULL END,'agent_path',coalesce(to_jsonb(n.path),'[]'::jsonb),'agent_configs_at_join',coalesce(chain,'[]'::jsonb),'agent_policy_at_join',CASE WHEN selected_kind='agent' THEN jsonb_build_object('version',policy.version,'config',policy.config) ELSE NULL END);
 RETURN NEW;
END $$;
CREATE TRIGGER immutable_member_attribution BEFORE INSERT OR UPDATE OR DELETE ON brand_members FOR EACH ROW EXECUTE FUNCTION capture_member_attribution();

ALTER TABLE bet_orders ADD COLUMN attribution_snapshot jsonb NOT NULL DEFAULT '{"schema_version":1,"legacy":true,"commission_policy":null}'::jsonb;
CREATE FUNCTION capture_order_attribution()RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE m brand_members;p brand_agent_policies;n agent_nodes;chain jsonb;
BEGIN
 IF TG_OP='UPDATE' THEN
  IF NEW.attribution_snapshot IS DISTINCT FROM OLD.attribution_snapshot THEN RAISE EXCEPTION 'order attribution immutable';END IF;RETURN NEW;
 END IF;
 SELECT * INTO m FROM brand_members WHERE brand_id=NEW.brand_id AND id=NEW.brand_member_id;
 IF NOT FOUND THEN RAISE EXCEPTION 'order member missing';END IF;
 SELECT * INTO p FROM brand_agent_policies WHERE brand_id=NEW.brand_id FOR SHARE;
 IF m.attribution_snapshot->>'agent_id' IS NOT NULL THEN
  SELECT * INTO n FROM agent_nodes WHERE brand_id=NEW.brand_id AND id=(m.attribution_snapshot->>'agent_id')::uuid;
  SELECT jsonb_agg(jsonb_build_object('id',a.id,'parent_id',a.parent_id,'depth',a.depth,'version',a.version,'config',a.config) ORDER BY a.depth) INTO chain FROM agent_nodes a WHERE a.brand_id=NEW.brand_id AND a.id=ANY(n.path);
 END IF;
 NEW.attribution_snapshot:=jsonb_build_object('schema_version',1,'legacy',coalesce((m.attribution_snapshot->>'legacy')::boolean,true),'member_attribution',m.attribution_snapshot,'agent_policy',jsonb_build_object('version',p.version,'config',p.config),'agent_configs_at_bet',coalesce(chain,'[]'::jsonb),'captured_at',clock_timestamp(),'commission_policy',NULL);
 RETURN NEW;
END $$;
CREATE TRIGGER immutable_order_attribution BEFORE INSERT OR UPDATE ON bet_orders FOR EACH ROW EXECUTE FUNCTION capture_order_attribution();

INSERT INTO permissions(key)VALUES('join_code.view.brand'),('join_code.view.platform'),('join_code.write.brand')ON CONFLICT DO NOTHING;
INSERT INTO role_permissions(role_id,permission_key)SELECT r.id,p.key FROM roles r CROSS JOIN permissions p WHERE r.is_bootstrap AND ((r.brand_id IS NOT NULL AND p.key IN('join_code.view.brand','join_code.write.brand')) OR (r.brand_id IS NULL AND p.key='join_code.view.platform'))ON CONFLICT DO NOTHING;
