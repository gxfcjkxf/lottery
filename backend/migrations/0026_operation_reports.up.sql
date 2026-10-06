INSERT INTO permissions(key) VALUES
 ('report_betting.view.brand'),('report_betting.view.platform'),
 ('report_ledger.view.brand'),('report_ledger.view.platform') ON CONFLICT DO NOTHING;
-- Custom roles gain no financial visibility automatically.
INSERT INTO role_permissions(role_id,permission_key)
SELECT r.id,p.key FROM roles r CROSS JOIN permissions p WHERE r.is_bootstrap AND
 ((r.brand_id IS NOT NULL AND p.key IN ('report_betting.view.brand','report_ledger.view.brand')) OR
 (r.brand_id IS NULL AND p.key IN ('report_betting.view.platform','report_ledger.view.platform') AND EXISTS
  (SELECT 1 FROM admin_account_roles ar JOIN admin_accounts a ON a.id=ar.account_id WHERE ar.role_id=r.id AND a.is_super_admin))) ON CONFLICT DO NOTHING;
CREATE INDEX bet_orders_report_placed ON bet_orders(brand_id,placed_at,id);
CREATE INDEX ledger_report_created ON point_ledger_entries(brand_id,created_at,id);
