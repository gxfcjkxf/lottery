ALTER TABLE roles ADD COLUMN brand_id uuid REFERENCES brands(id);
ALTER TABLE roles ADD COLUMN status text NOT NULL DEFAULT 'active' CHECK(status IN ('active','disabled'));
ALTER TABLE roles ADD COLUMN version bigint NOT NULL DEFAULT 1 CHECK(version>0);
ALTER TABLE roles ADD COLUMN is_bootstrap boolean NOT NULL DEFAULT false;
ALTER TABLE admin_accounts ADD COLUMN version bigint NOT NULL DEFAULT 1 CHECK(version>0);

-- Earlier server-owner bootstrap roles are private to a single admin. Attach
-- single-brand roles to that brand, preserving platform/legacy roles as NULL.
UPDATE roles r SET brand_id=(
 SELECT min(s.brand_id::text)::uuid FROM admin_account_roles ar
 JOIN admin_brand_scopes s ON s.account_id=ar.account_id WHERE ar.role_id=r.id
) WHERE (SELECT count(DISTINCT s.brand_id) FROM admin_account_roles ar
 JOIN admin_brand_scopes s ON s.account_id=ar.account_id WHERE ar.role_id=r.id)=1
 AND NOT EXISTS(SELECT 1 FROM admin_account_roles ar JOIN admin_accounts a ON a.id=ar.account_id
 WHERE ar.role_id=r.id AND a.is_super_admin);
UPDATE roles SET is_bootstrap=true WHERE left(code,10)='bootstrap_';
ALTER TABLE roles DROP CONSTRAINT roles_code_key;
ALTER TABLE roles ADD CONSTRAINT roles_brand_code_unique UNIQUE NULLS NOT DISTINCT(brand_id,code);
CREATE INDEX roles_brand_active ON roles(brand_id,status);

INSERT INTO permissions(key) VALUES
 ('user.create.brand'),('role.view.brand'),('role.write.brand'),
 ('admin.view.brand'),('admin.write.brand'),
 ('auth_config.view.brand'),('auth_config.write.brand'),
 ('role.view.platform'),('role.write.platform'),
 ('admin.view.platform'),('admin.write.platform'),
 ('auth_config.view.platform'),('auth_config.write.platform')
 ON CONFLICT DO NOTHING;
-- Upgrade only explicit bootstrap grants, never ordinary custom roles.
INSERT INTO role_permissions(role_id,permission_key)
 SELECT r.id,p.key FROM roles r CROSS JOIN permissions p
 WHERE r.is_bootstrap AND r.brand_id IS NOT NULL AND p.key IN
 ('user.create.brand','role.view.brand','role.write.brand','admin.view.brand',
 'admin.write.brand','auth_config.view.brand','auth_config.write.brand')
 ON CONFLICT DO NOTHING;
INSERT INTO role_permissions(role_id,permission_key)
 SELECT r.id,p.key FROM roles r CROSS JOIN permissions p
 WHERE r.is_bootstrap AND r.brand_id IS NULL AND EXISTS
 (SELECT 1 FROM admin_account_roles ar JOIN admin_accounts a ON a.id=ar.account_id
 WHERE ar.role_id=r.id AND a.is_super_admin) AND p.key IN
 ('role.view.platform','role.write.platform','admin.view.platform','admin.write.platform',
 'auth_config.view.platform','auth_config.write.platform') ON CONFLICT DO NOTHING;

CREATE FUNCTION enforce_admin_role_brand() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE role_brand uuid;
BEGIN
 SELECT brand_id INTO role_brand FROM roles WHERE id=NEW.role_id;
 IF role_brand IS NOT NULL AND NOT EXISTS
 (SELECT 1 FROM admin_brand_scopes WHERE account_id=NEW.account_id AND brand_id=role_brand) THEN
  RAISE EXCEPTION 'role is outside account brand scope' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER admin_role_brand_guard BEFORE INSERT OR UPDATE ON admin_account_roles
 FOR EACH ROW EXECUTE FUNCTION enforce_admin_role_brand();

CREATE FUNCTION immutable_admin_role_identity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.brand_id IS DISTINCT FROM OLD.brand_id OR NEW.code IS DISTINCT FROM OLD.code THEN
  RAISE EXCEPTION 'role brand and code are immutable' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER role_identity_guard BEFORE UPDATE OF brand_id,code ON roles
 FOR EACH ROW EXECUTE FUNCTION immutable_admin_role_identity();
CREATE FUNCTION protect_assigned_admin_scope() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='UPDATE' AND NEW.account_id=OLD.account_id AND NEW.brand_id=OLD.brand_id THEN
  RETURN NEW;
 END IF;
 IF EXISTS(SELECT 1 FROM admin_account_roles ar JOIN roles r ON r.id=ar.role_id
 WHERE ar.account_id=OLD.account_id AND r.brand_id=OLD.brand_id) THEN
  RAISE EXCEPTION 'remove assigned brand roles before changing account scope' USING ERRCODE='23514';
 END IF;
 IF TG_OP='DELETE' THEN RETURN OLD; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER admin_scope_assignment_guard BEFORE DELETE OR UPDATE ON admin_brand_scopes
 FOR EACH ROW EXECUTE FUNCTION protect_assigned_admin_scope();

ALTER TABLE brand_members ADD COLUMN terms_accepted boolean NOT NULL DEFAULT true;
ALTER TABLE brand_members ADD COLUMN created_by uuid REFERENCES admin_accounts(id);
ALTER TABLE brand_members ALTER COLUMN accepted_at DROP NOT NULL;
ALTER TABLE brand_members ADD CONSTRAINT member_consent_consistent
 CHECK((terms_accepted AND accepted_at IS NOT NULL) OR (NOT terms_accepted AND accepted_at IS NULL));
