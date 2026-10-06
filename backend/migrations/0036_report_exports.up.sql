INSERT INTO permissions(key) VALUES
 ('report_betting.export.brand'),('report_betting.export.platform'),
 ('report_ledger.export.brand'),('report_ledger.export.platform') ON CONFLICT DO NOTHING;
-- No automatic expansion for custom roles. CSV access still requires explicit
-- corresponding view permission; export is not a user or financial write grant.
INSERT INTO role_permissions(role_id,permission_key)
 SELECT r.id,p.key FROM roles r CROSS JOIN permissions p WHERE r.is_bootstrap AND
 ((r.brand_id IS NOT NULL AND p.key IN('report_betting.export.brand','report_ledger.export.brand')) OR
 (r.brand_id IS NULL AND p.key IN('report_betting.export.platform','report_ledger.export.platform') AND EXISTS
  (SELECT 1 FROM admin_account_roles ar JOIN admin_accounts a ON a.id=ar.account_id WHERE ar.role_id=r.id AND a.is_super_admin))) ON CONFLICT DO NOTHING;
