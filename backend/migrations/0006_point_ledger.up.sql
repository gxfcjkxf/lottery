ALTER TABLE point_accounts ADD CONSTRAINT point_account_version_nonnegative CHECK(version>=0);
ALTER TABLE point_accounts ADD CONSTRAINT point_account_member_identity UNIQUE(brand_id,id,brand_member_id);
ALTER TABLE point_ledger_entries ADD COLUMN member_id uuid;
ALTER TABLE point_ledger_entries ADD COLUMN version bigint;
ALTER TABLE point_ledger_entries ADD COLUMN request_hash text;
ALTER TABLE point_ledger_entries ADD COLUMN actor_type text;
-- No transaction entries existed before S3. Fail closed rather than fabricate
-- financial history if someone populated the previously unused ledger table.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM point_ledger_entries) THEN
  RAISE EXCEPTION 'existing ledger entries require explicit migration reconciliation';
 END IF;
END $$;
ALTER TABLE point_ledger_entries ALTER COLUMN member_id SET NOT NULL;
ALTER TABLE point_ledger_entries ALTER COLUMN version SET NOT NULL;
ALTER TABLE point_ledger_entries ALTER COLUMN request_hash SET NOT NULL;
ALTER TABLE point_ledger_entries ALTER COLUMN actor_type SET NOT NULL;
ALTER TABLE point_ledger_entries ADD CONSTRAINT ledger_version_positive CHECK(version>0);
ALTER TABLE point_ledger_entries ADD CONSTRAINT ledger_actor_type CHECK(actor_type IN ('user','admin','system'));
ALTER TABLE point_ledger_entries ADD CONSTRAINT ledger_hash_valid CHECK(request_hash ~ '^[0-9a-f]{64}$');
ALTER TABLE point_ledger_entries ADD CONSTRAINT ledger_account_member_fk
 FOREIGN KEY(brand_id,account_id,member_id) REFERENCES point_accounts(brand_id,id,brand_member_id);
ALTER TABLE point_ledger_entries ADD CONSTRAINT ledger_account_version_unique UNIQUE(brand_id,account_id,version);
ALTER TABLE point_ledger_entries ADD CONSTRAINT ledger_brand_id_unique UNIQUE(brand_id,id);
ALTER TABLE point_ledger_entries ADD CONSTRAINT ledger_account_id_unique UNIQUE(brand_id,account_id,id);
ALTER TABLE point_ledger_entries ADD CONSTRAINT ledger_reversal_same_brand
 FOREIGN KEY(brand_id,account_id,reversal_of) REFERENCES point_ledger_entries(brand_id,account_id,id);
CREATE UNIQUE INDEX ledger_single_reversal ON point_ledger_entries(reversal_of) WHERE reversal_of IS NOT NULL;
CREATE INDEX ledger_member_version ON point_ledger_entries(brand_id,member_id,version DESC);
CREATE INDEX ledger_reference ON point_ledger_entries(brand_id,reference_type,reference_id);

CREATE TABLE recharge_orders (
 id uuid PRIMARY KEY,
 brand_id uuid NOT NULL,
 member_id uuid NOT NULL,
 account_id uuid NOT NULL,
 points bigint NOT NULL CHECK(points>0),
 state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','confirmed','cancelled')),
 proof_reference text NOT NULL DEFAULT '',
 remark text NOT NULL DEFAULT '',
 created_by uuid NOT NULL REFERENCES admin_accounts(id),
 confirmed_by uuid REFERENCES admin_accounts(id),
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 created_at timestamptz NOT NULL DEFAULT now(),
 confirmed_at timestamptz,
 ledger_entry_id uuid,
 FOREIGN KEY(brand_id,account_id,member_id) REFERENCES point_accounts(brand_id,id,brand_member_id),
 FOREIGN KEY(brand_id,account_id,ledger_entry_id) REFERENCES point_ledger_entries(brand_id,account_id,id),
 UNIQUE(brand_id,id),
 UNIQUE(ledger_entry_id),
 CHECK((state='confirmed' AND confirmed_at IS NOT NULL AND confirmed_by IS NOT NULL AND ledger_entry_id IS NOT NULL)
 OR (state<>'confirmed' AND confirmed_at IS NULL AND confirmed_by IS NULL AND ledger_entry_id IS NULL))
);
CREATE INDEX recharge_brand_created ON recharge_orders(brand_id,created_at DESC,id DESC);
CREATE INDEX recharge_member_created ON recharge_orders(brand_id,member_id,created_at DESC,id DESC);

INSERT INTO permissions(key) VALUES
 ('wallet.view.brand'),('wallet.view.platform'),('wallet.freeze.brand'),('wallet.adjust.brand'),
 ('recharge.view.brand'),('recharge.view.platform'),('recharge.write.brand') ON CONFLICT DO NOTHING;
INSERT INTO role_permissions(role_id,permission_key)
 SELECT r.id,p.key FROM roles r CROSS JOIN permissions p WHERE r.is_bootstrap AND
 ((r.brand_id IS NOT NULL AND p.key IN ('wallet.view.brand','wallet.freeze.brand','wallet.adjust.brand','recharge.view.brand','recharge.write.brand'))
 OR (r.brand_id IS NULL AND EXISTS(SELECT 1 FROM admin_account_roles ar JOIN admin_accounts a ON a.id=ar.account_id
 WHERE ar.role_id=r.id AND a.is_super_admin) AND p.key IN ('wallet.view.platform','recharge.view.platform')))
 ON CONFLICT DO NOTHING;
