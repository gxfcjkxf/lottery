-- Archive creation, reading, and download are independent capabilities.
-- Platform archive access remains read-only; custom roles gain no grants.
INSERT INTO permissions(key) VALUES
 ('report_archive.view.brand'),('report_archive.create.brand'),('report_archive.download.brand'),
 ('report_archive.view.platform'),('report_archive.download.platform')
 ON CONFLICT DO NOTHING;

INSERT INTO role_permissions(role_id,permission_key)
 SELECT r.id,p.key FROM roles r CROSS JOIN permissions p
 WHERE r.is_bootstrap AND r.brand_id IS NOT NULL AND p.key IN
 ('report_archive.view.brand','report_archive.create.brand','report_archive.download.brand')
 ON CONFLICT DO NOTHING;

INSERT INTO role_permissions(role_id,permission_key)
 SELECT r.id,p.key FROM roles r CROSS JOIN permissions p
 WHERE r.is_bootstrap AND r.brand_id IS NULL AND p.key IN
 ('report_archive.view.platform','report_archive.download.platform')
 AND EXISTS(SELECT 1 FROM admin_account_roles ar JOIN admin_accounts a ON a.id=ar.account_id
  WHERE ar.role_id=r.id AND a.is_super_admin)
 ON CONFLICT DO NOTHING;
