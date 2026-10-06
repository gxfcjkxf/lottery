-- Presentation is additive; preserve legacy columns and all financial facts.
CREATE FUNCTION initial_brand_presentation(b brands) RETURNS jsonb LANGUAGE sql IMMUTABLE AS $$
 SELECT jsonb_build_object(
 'display_name',NULL,'logo_text',NULL,'logo_url',NULL,'favicon_url',NULL,
 'primary_color',CASE WHEN b.theme->>'primary_color' ~ '^#[0-9a-fA-F]{6}$' THEN b.theme->'primary_color' ELSE NULL END,
 'accent_color',CASE WHEN b.theme->>'accent_color' ~ '^#[0-9a-fA-F]{6}$' THEN b.theme->'accent_color' ELSE NULL END,
 'success_color',CASE WHEN b.theme->>'success_color' ~ '^#[0-9a-fA-F]{6}$' THEN b.theme->'success_color' ELSE NULL END,
 'warning_color',CASE WHEN b.theme->>'warning_color' ~ '^#[0-9a-fA-F]{6}$' THEN b.theme->'warning_color' ELSE NULL END,
 'danger_color',CASE WHEN b.theme->>'danger_color' ~ '^#[0-9a-fA-F]{6}$' THEN b.theme->'danger_color' ELSE NULL END,
 'font_family',NULL,'font_scale',NULL,'radius',NULL,'shadow',NULL,
 'default_locale',CASE WHEN b.default_locale IN('en','zh-CN') THEN b.default_locale WHEN b.default_locale='zh' THEN 'zh-CN' ELSE NULL END,'available_locales',NULL,'content',NULL)
$$;
CREATE TABLE brand_presentations (
 brand_id uuid PRIMARY KEY REFERENCES brands(id),
 version bigint NOT NULL CHECK(version>0),
 config jsonb NOT NULL CHECK(jsonb_typeof(config)='object')
);
INSERT INTO brand_presentations(brand_id,version,config) SELECT b.id,b.config_version,initial_brand_presentation(b) FROM brands b;
CREATE FUNCTION initialize_brand_presentation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN INSERT INTO brand_presentations(brand_id,version,config)VALUES(NEW.id,NEW.config_version,initial_brand_presentation(NEW));RETURN NEW;END $$;
CREATE TRIGGER brand_presentation_initialized AFTER INSERT ON brands FOR EACH ROW EXECUTE FUNCTION initialize_brand_presentation();

CREATE TABLE brand_presentation_revisions (
 id uuid PRIMARY KEY,brand_id uuid NOT NULL REFERENCES brands(id),version bigint NOT NULL CHECK(version>1),
 config jsonb NOT NULL,effective jsonb NOT NULL,changed_by uuid NOT NULL REFERENCES admin_accounts(id),
 reason text NOT NULL CHECK(length(trim(reason))>0 AND octet_length(reason)<=500),
 audit_log_id uuid NOT NULL REFERENCES audit_logs(id),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(brand_id,version)
);
CREATE FUNCTION guard_brand_presentation_revision() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE b brands;p brand_presentations;a audit_logs;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'presentation history immutable';END IF;
 SELECT * INTO b FROM brands WHERE id=NEW.brand_id;
 SELECT * INTO p FROM brand_presentations WHERE brand_id=NEW.brand_id;
 SELECT * INTO a FROM audit_logs WHERE id=NEW.audit_log_id;
 IF NOT FOUND OR b.id IS NULL OR p.brand_id IS NULL OR p.version IS DISTINCT FROM NEW.version OR b.config_version IS DISTINCT FROM NEW.version
 OR p.config IS DISTINCT FROM NEW.config OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.actor_type IS DISTINCT FROM 'admin'
 OR a.actor_id IS DISTINCT FROM NEW.changed_by OR a.action IS DISTINCT FROM 'brand_presentation.update'
 OR a.resource_type IS DISTINCT FROM 'brand_presentation' OR a.resource_id IS DISTINCT FROM NEW.brand_id OR a.reason IS DISTINCT FROM NEW.reason
 OR a.before_json->>'brand_id' IS DISTINCT FROM NEW.brand_id::text OR (a.before_json->>'version')::bigint IS DISTINCT FROM (NEW.version-1)
 OR a.after_json->>'brand_id' IS DISTINCT FROM NEW.brand_id::text OR (a.after_json->>'version')::bigint IS DISTINCT FROM NEW.version
 OR a.after_json->>'status' IS DISTINCT FROM b.status OR a.after_json->>'base_name' IS DISTINCT FROM b.name
 OR a.after_json->'config' IS DISTINCT FROM NEW.config OR a.after_json->'effective' IS DISTINCT FROM NEW.effective
 OR (a.after_json->>'updated_at')::timestamptz IS DISTINCT FROM b.updated_at THEN RAISE EXCEPTION 'presentation audit witness missing';END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_presentation_history BEFORE INSERT OR UPDATE OR DELETE ON brand_presentation_revisions FOR EACH ROW EXECUTE FUNCTION guard_brand_presentation_revision();
CREATE FUNCTION guard_brand_presentation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'presentation cannot be deleted';END IF;
 IF NEW.brand_id IS DISTINCT FROM OLD.brand_id OR NEW.version<=OLD.version OR NOT EXISTS(SELECT 1 FROM brands WHERE id=NEW.brand_id AND config_version=NEW.version) THEN RAISE EXCEPTION 'presentation identity/version invalid';END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_presentation_update BEFORE UPDATE OR DELETE ON brand_presentations FOR EACH ROW EXECUTE FUNCTION guard_brand_presentation();
CREATE FUNCTION require_brand_presentation_history() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p brand_presentations;
BEGIN
 SELECT * INTO p FROM brand_presentations WHERE brand_id=NEW.brand_id;
 IF NOT EXISTS(SELECT 1 FROM brand_presentation_revisions h WHERE h.brand_id=p.brand_id AND h.version=p.version AND h.config=p.config) THEN RAISE EXCEPTION 'presentation requires audited history';END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER presentation_history_required AFTER UPDATE ON brand_presentations DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_brand_presentation_history();
INSERT INTO permissions(key) VALUES('brand_presentation.view.brand'),('brand_presentation.write.brand'),('brand_presentation.view.platform'),('brand_presentation.write.platform') ON CONFLICT DO NOTHING;
INSERT INTO role_permissions(role_id,permission_key)SELECT r.id,p.key FROM roles r CROSS JOIN permissions p WHERE r.is_bootstrap AND
 ((r.brand_id IS NOT NULL AND p.key IN('brand_presentation.view.brand','brand_presentation.write.brand')) OR
 (r.brand_id IS NULL AND p.key IN('brand_presentation.view.platform','brand_presentation.write.platform') AND EXISTS(SELECT 1 FROM admin_account_roles ar JOIN admin_accounts a ON a.id=ar.account_id WHERE ar.role_id=r.id AND a.is_super_admin)))ON CONFLICT DO NOTHING;
