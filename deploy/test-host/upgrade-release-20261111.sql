CREATE TABLE announcements (
 id uuid PRIMARY KEY,
 brand_id uuid REFERENCES brands(id) ON DELETE CASCADE,
 version bigint NOT NULL CHECK(version>0),
 generation bigint NOT NULL CHECK(generation>0),
 status text NOT NULL CHECK(status IN ('draft','published','withdrawn')),
 config jsonb NOT NULL,
 published_at timestamptz,
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX announcements_scope ON announcements(brand_id,status);
CREATE TABLE announcement_assets (
 id text PRIMARY KEY CHECK(id ~ '^[a-f0-9]{64}$'),
 brand_id uuid REFERENCES brands(id) ON DELETE CASCADE,
 media_type text NOT NULL CHECK(media_type IN ('image/png','image/jpeg','image/webp')),
 content bytea NOT NULL CHECK(octet_length(content)<=5242880)
);
CREATE TABLE announcement_receipts (
 brand_id uuid NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
 subject text NOT NULL,
 announcement_id uuid NOT NULL REFERENCES announcements(id) ON DELETE CASCADE,
 generation bigint NOT NULL,
 seen_at timestamptz,
 session_key text NOT NULL,
 muted_day text,
 PRIMARY KEY(brand_id,subject,announcement_id,generation)
);
INSERT INTO permissions(key) VALUES
 ('announcement.view.platform'),('announcement.write.platform'),('announcement.publish.platform'),
 ('announcement.view.brand'),('announcement.write.brand'),('announcement.publish.brand');

DROP TABLE brand_compliance_policies,compliance_decisions,compliance_gate_rejections,compliance_policy_revisions,platform_compliance_settings CASCADE;
DROP FUNCTION compliance_stub_checks(jsonb) CASCADE;
DROP FUNCTION default_compliance_config() CASCADE;
DROP FUNCTION guard_compliance_decision() CASCADE;
DROP FUNCTION guard_compliance_gate_rejection() CASCADE;
DROP FUNCTION guard_compliance_policy() CASCADE;
DROP FUNCTION guard_compliance_revision() CASCADE;
DROP FUNCTION initial_compliance_config() CASCADE;
DROP FUNCTION initialize_brand_compliance() CASCADE;
DROP FUNCTION require_compliance_history() CASCADE;
DROP FUNCTION valid_compliance_config(jsonb) CASCADE;
DELETE FROM role_permissions WHERE permission_key LIKE 'compliance\_%' ESCAPE '\'; DELETE FROM permissions WHERE key LIKE 'compliance\_%' ESCAPE '\';
ALTER TABLE cash_balances DROP CONSTRAINT cash_balances_brand_id_member_id_fkey; ALTER TABLE cash_balances ADD CONSTRAINT cash_balances_wallet_owner_fkey FOREIGN KEY(brand_id,member_id) REFERENCES wallet_owners(brand_id,id); ALTER TABLE cash_orders DROP CONSTRAINT cash_orders_brand_id_member_id_fkey; ALTER TABLE cash_orders ADD CONSTRAINT cash_orders_wallet_owner_fkey FOREIGN KEY(brand_id,member_id) REFERENCES wallet_owners(brand_id,id);
