CREATE TABLE brand_operation_revisions (
 id uuid PRIMARY KEY,
 brand_id uuid NOT NULL REFERENCES brands(id),
 version bigint NOT NULL CHECK(version>1),
 previous_status text NOT NULL CHECK(previous_status IN ('active','paused')),
 status text NOT NULL CHECK(status IN ('active','paused')),
 changed_by uuid NOT NULL REFERENCES admin_accounts(id),
 reason text NOT NULL CHECK(length(trim(reason))>0 AND octet_length(reason)<=500),
 audit_log_id uuid NOT NULL REFERENCES audit_logs(id),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(brand_id,version),
 CHECK(previous_status<>status)
);
CREATE INDEX brand_operation_history ON brand_operation_revisions(brand_id,version DESC);

CREATE FUNCTION guard_brand_operation_revision() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE b brands; a audit_logs;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'brand operation history is immutable'; END IF;
 SELECT * INTO b FROM brands WHERE id=NEW.brand_id;
 SELECT * INTO a FROM audit_logs WHERE id=NEW.audit_log_id;
 IF NOT FOUND OR b.id IS NULL OR b.config_version<>NEW.version OR b.status<>NEW.status OR
    a.brand_id IS DISTINCT FROM NEW.brand_id OR a.actor_type IS DISTINCT FROM 'admin' OR a.actor_id IS DISTINCT FROM NEW.changed_by OR
    a.action IS DISTINCT FROM 'brand_operation.update' OR a.resource_type IS DISTINCT FROM 'brand_operation' OR a.resource_id IS DISTINCT FROM NEW.brand_id OR a.reason IS DISTINCT FROM NEW.reason OR
    a.before_json->>'brand_id' IS DISTINCT FROM NEW.brand_id::text OR
    a.before_json->>'version' IS DISTINCT FROM (NEW.version-1)::text OR
    a.before_json->>'status' IS DISTINCT FROM NEW.previous_status OR
    a.before_json->>'name' IS DISTINCT FROM b.name OR
    a.after_json->>'brand_id' IS DISTINCT FROM NEW.brand_id::text OR
    a.after_json->>'version' IS DISTINCT FROM NEW.version::text OR
    a.after_json->>'status' IS DISTINCT FROM NEW.status OR
    a.after_json->>'name' IS DISTINCT FROM b.name OR
    (a.after_json->>'updated_at')::timestamptz IS DISTINCT FROM b.updated_at THEN
  RAISE EXCEPTION 'brand operation history requires matching current brand and audit witness';
 END IF;
 NEW.created_at:=clock_timestamp();
 RETURN NEW;
END $$;
CREATE TRIGGER immutable_brand_operation_revision BEFORE INSERT OR UPDATE OR DELETE ON brand_operation_revisions
 FOR EACH ROW EXECUTE FUNCTION guard_brand_operation_revision();

INSERT INTO permissions(key) VALUES
 ('brand_operation.view.brand'),('brand_operation.view.platform'),
 ('brand_operation.write.brand'),('brand_operation.write.platform')
ON CONFLICT DO NOTHING;
INSERT INTO role_permissions(role_id,permission_key)
SELECT r.id,p.key FROM roles r CROSS JOIN permissions p
WHERE r.is_bootstrap AND (
 (r.brand_id IS NOT NULL AND p.key IN ('brand_operation.view.brand','brand_operation.write.brand')) OR
 (r.brand_id IS NULL AND p.key IN ('brand_operation.view.platform','brand_operation.write.platform') AND
  EXISTS(SELECT 1 FROM admin_account_roles ar JOIN admin_accounts a ON a.id=ar.account_id WHERE ar.role_id=r.id AND a.is_super_admin))
)
ON CONFLICT DO NOTHING;
