-- Additive management history. Existing host bindings remain unchanged.
CREATE TABLE brand_domain_revisions (
 id uuid PRIMARY KEY,brand_id uuid NOT NULL REFERENCES brands(id),version bigint NOT NULL CHECK(version>1),
 changed_by uuid NOT NULL REFERENCES admin_accounts(id),reason text NOT NULL CHECK(length(trim(reason))>0 AND octet_length(reason)<=500),
 audit_log_id uuid NOT NULL REFERENCES audit_logs(id),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 before_domains jsonb NOT NULL CHECK(jsonb_typeof(before_domains)='array'),domains jsonb NOT NULL CHECK(jsonb_typeof(domains)='array'),UNIQUE(brand_id,version)
);
CREATE FUNCTION guard_brand_domain_revision() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE b brands;a audit_logs;projection jsonb;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'domain history immutable';END IF;
 SELECT * INTO b FROM brands WHERE id=NEW.brand_id;
 SELECT * INTO a FROM audit_logs WHERE id=NEW.audit_log_id;
 SELECT coalesce(jsonb_agg(jsonb_build_object('id',d.id,'domain',d.domain,'enabled',d.enabled,'is_primary',d.is_primary) ORDER BY d.domain,d.id),'[]'::jsonb) INTO projection FROM brand_domains d WHERE brand_id=NEW.brand_id;
 IF NOT FOUND OR b.id IS NULL OR a.id IS NULL OR b.config_version IS DISTINCT FROM NEW.version
 OR NEW.domains IS DISTINCT FROM projection OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.actor_type IS DISTINCT FROM 'admin'
 OR a.actor_id IS DISTINCT FROM NEW.changed_by OR a.action IS DISTINCT FROM 'brand_domains.update' OR a.resource_type IS DISTINCT FROM 'brand_domains'
 OR a.resource_id IS DISTINCT FROM NEW.brand_id OR a.reason IS DISTINCT FROM NEW.reason
 OR a.before_json->>'brand_id' IS DISTINCT FROM NEW.brand_id::text OR (a.before_json->>'version')::bigint IS DISTINCT FROM (NEW.version-1)
 OR a.before_json->'domains' IS DISTINCT FROM NEW.before_domains OR a.after_json->'domains' IS DISTINCT FROM NEW.domains
 OR a.after_json->>'brand_id' IS DISTINCT FROM NEW.brand_id::text OR (a.after_json->>'version')::bigint IS DISTINCT FROM NEW.version
 OR a.after_json->>'status' IS DISTINCT FROM b.status OR (a.after_json->>'updated_at')::timestamptz IS DISTINCT FROM b.updated_at THEN RAISE EXCEPTION 'domain audit witness missing';END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_domain_history BEFORE INSERT OR UPDATE OR DELETE ON brand_domain_revisions FOR EACH ROW EXECUTE FUNCTION guard_brand_domain_revision();
CREATE FUNCTION guard_brand_domain_binding() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' OR NEW.id IS DISTINCT FROM OLD.id OR NEW.brand_id IS DISTINCT FROM OLD.brand_id OR NEW.domain IS DISTINCT FROM OLD.domain THEN RAISE EXCEPTION 'domain binding immutable';END IF;
 IF NEW.is_primary AND NOT NEW.enabled THEN RAISE EXCEPTION 'primary domain must be enabled';END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_domain_binding BEFORE UPDATE OR DELETE ON brand_domains FOR EACH ROW EXECUTE FUNCTION guard_brand_domain_binding();
CREATE FUNCTION require_brand_domain_history() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE projection jsonb;v bigint;
BEGIN
 SELECT config_version INTO v FROM brands WHERE id=NEW.brand_id;
 SELECT coalesce(jsonb_agg(jsonb_build_object('id',d.id,'domain',d.domain,'enabled',d.enabled,'is_primary',d.is_primary) ORDER BY d.domain,d.id),'[]'::jsonb) INTO projection FROM brand_domains d WHERE brand_id=NEW.brand_id;
 IF NOT EXISTS(SELECT 1 FROM brand_domain_revisions h WHERE h.brand_id=NEW.brand_id AND h.version=v AND h.domains=projection) THEN RAISE EXCEPTION 'domain change requires audited history';END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER domain_history_required AFTER UPDATE ON brand_domains DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_brand_domain_history();
INSERT INTO permissions(key) VALUES('brand_domains.view.brand'),('brand_domains.write.brand'),('brand_domains.view.platform'),('brand_domains.write.platform') ON CONFLICT DO NOTHING;
INSERT INTO role_permissions(role_id,permission_key) SELECT r.id,p.key FROM roles r CROSS JOIN permissions p WHERE r.is_bootstrap AND
 ((r.brand_id IS NOT NULL AND p.key IN('brand_domains.view.brand','brand_domains.write.brand')) OR
 (r.brand_id IS NULL AND p.key IN('brand_domains.view.platform','brand_domains.write.platform') AND EXISTS(SELECT 1 FROM admin_account_roles ar JOIN admin_accounts a ON a.id=ar.account_id WHERE ar.role_id=r.id AND a.is_super_admin))) ON CONFLICT DO NOTHING;
