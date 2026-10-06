CREATE TABLE platform_idempotency_requests (
 actor_id uuid NOT NULL, operation text NOT NULL, key text NOT NULL,
 request_hash text NOT NULL, status_code integer, response jsonb,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(actor_id,operation,key)
);

CREATE TABLE brand_creation_records (
 brand_id uuid PRIMARY KEY REFERENCES brands(id),
 code text NOT NULL CHECK(code ~ '^[a-z][a-z0-9_]{0,47}$'),
 name text NOT NULL CHECK(length(trim(name))>0 AND octet_length(name)<=120),
 default_locale text NOT NULL CHECK(default_locale IN ('en','zh-CN')),
 timezone text NOT NULL CHECK(length(timezone)>0 AND octet_length(timezone)<=80),
 status text NOT NULL CHECK(status='paused'), version bigint NOT NULL CHECK(version=1),
 created_by uuid NOT NULL REFERENCES admin_accounts(id),
 reason text NOT NULL CHECK(length(trim(reason))>0 AND octet_length(reason)<=500),
 audit_log_id uuid NOT NULL UNIQUE REFERENCES audit_logs(id), created_at timestamptz NOT NULL
);
CREATE TRIGGER immutable_brand_creation_records BEFORE UPDATE OR DELETE ON brand_creation_records
 FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();

CREATE FUNCTION validate_brand_creation_record() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM brands b WHERE b.id=NEW.brand_id AND b.code=NEW.code AND b.name=NEW.name AND b.default_locale=NEW.default_locale AND b.timezone=NEW.timezone AND b.status=NEW.status AND b.config_version=NEW.version AND b.created_at=NEW.created_at)
 OR NOT EXISTS(SELECT 1 FROM audit_logs a WHERE a.id=NEW.audit_log_id AND a.brand_id=NEW.brand_id AND a.actor_type='admin' AND a.actor_id=NEW.created_by AND a.action='brand.create' AND a.resource_type='brand' AND a.resource_id=NEW.brand_id AND a.reason=NEW.reason AND a.after_json->>'code'=NEW.code AND a.after_json->>'name'=NEW.name AND a.after_json->>'status'=NEW.status AND a.after_json->>'default_locale'=NEW.default_locale AND a.after_json->>'timezone'=NEW.timezone)
 THEN RAISE EXCEPTION 'brand creation requires matching initial brand and audit'; END IF;
 RETURN NEW;
END $$;
DO $$ BEGIN
 EXECUTE format('ALTER FUNCTION %I.validate_brand_creation_record() SET search_path = pg_catalog, %I, pg_temp',current_schema(),current_schema());
END $$;
CREATE TRIGGER valid_brand_creation_record BEFORE INSERT ON brand_creation_records FOR EACH ROW EXECUTE FUNCTION validate_brand_creation_record();

INSERT INTO permissions(key) VALUES('brand.create.platform') ON CONFLICT DO NOTHING;
INSERT INTO role_permissions(role_id,permission_key)
 SELECT r.id,'brand.create.platform' FROM roles r WHERE r.is_bootstrap AND r.brand_id IS NULL
 AND EXISTS(SELECT 1 FROM admin_account_roles ar JOIN admin_accounts a ON a.id=ar.account_id WHERE ar.role_id=r.id AND a.is_super_admin)
 ON CONFLICT DO NOTHING;
