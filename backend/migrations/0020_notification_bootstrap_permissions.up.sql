-- Only explicit server-owner bootstrap roles are upgraded. Custom roles do not
-- gain new privileges; platform bootstrap administrators remain read-only here.
INSERT INTO role_permissions(role_id,permission_key)
SELECT r.id,p.key FROM roles r CROSS JOIN permissions p WHERE r.is_bootstrap AND
 ((r.brand_id IS NOT NULL AND p.key IN ('notification.view.brand','notification.retry.brand')) OR
 (r.brand_id IS NULL AND p.key='notification.view.platform' AND EXISTS
  (SELECT 1 FROM admin_account_roles ar JOIN admin_accounts a ON a.id=ar.account_id WHERE ar.role_id=r.id AND a.is_super_admin))) ON CONFLICT DO NOTHING;
