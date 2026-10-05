CREATE TABLE brand_point_policies (
 brand_id uuid PRIMARY KEY REFERENCES brands(id),
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 max_balance_points bigint CHECK(max_balance_points>0),
 max_recharge_points bigint CHECK(max_recharge_points>0),
 max_adjustment_points bigint CHECK(max_adjustment_points>0)
);
INSERT INTO brand_point_policies(brand_id) SELECT id FROM brands;
CREATE FUNCTION initialize_brand_point_policy() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN INSERT INTO brand_point_policies(brand_id) VALUES(NEW.id); RETURN NEW; END $$;
CREATE TRIGGER brand_point_policy_init AFTER INSERT ON brands
 FOR EACH ROW EXECUTE FUNCTION initialize_brand_point_policy();

CREATE TABLE point_balance_repairs (
 id uuid PRIMARY KEY,
 brand_id uuid NOT NULL,
 account_id uuid NOT NULL,
 member_id uuid NOT NULL,
 version bigint NOT NULL CHECK(version>=0),
 before_snapshot jsonb NOT NULL,
 after_snapshot jsonb NOT NULL,
 reason text NOT NULL,
 actor_id uuid NOT NULL REFERENCES admin_accounts(id),
 request_id text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(brand_id,account_id,member_id) REFERENCES point_accounts(brand_id,id,brand_member_id)
);
CREATE TRIGGER point_repairs_append_only BEFORE UPDATE OR DELETE ON point_balance_repairs
 FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE INDEX point_repairs_member ON point_balance_repairs(brand_id,member_id,created_at DESC,id DESC);

INSERT INTO permissions(key) VALUES
 ('point_policy.view.brand'),('point_policy.view.platform'),('point_policy.write.brand'),('wallet.repair.brand')
 ON CONFLICT DO NOTHING;
INSERT INTO role_permissions(role_id,permission_key)
 SELECT r.id,p.key FROM roles r CROSS JOIN permissions p WHERE r.is_bootstrap AND
 ((r.brand_id IS NOT NULL AND p.key IN ('point_policy.view.brand','point_policy.write.brand','wallet.repair.brand'))
 OR (r.brand_id IS NULL AND EXISTS(SELECT 1 FROM admin_account_roles ar JOIN admin_accounts a ON a.id=ar.account_id
 WHERE ar.role_id=r.id AND a.is_super_admin) AND p.key='point_policy.view.platform'))
 ON CONFLICT DO NOTHING;
