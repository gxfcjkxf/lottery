-- Export is a separate read-only grant, never inferred from audit viewing.
INSERT INTO permissions(key) VALUES
 ('audit.export.brand'),('audit.export.platform') ON CONFLICT DO NOTHING;
INSERT INTO role_permissions(role_id,permission_key)
 SELECT r.id,p.key FROM roles r CROSS JOIN permissions p
 WHERE r.is_bootstrap AND
 ((r.brand_id IS NOT NULL AND p.key='audit.export.brand') OR
  (r.brand_id IS NULL AND p.key='audit.export.platform'))
ON CONFLICT DO NOTHING;

CREATE INDEX audit_brand_time_page ON audit_logs(brand_id,created_at DESC,id DESC);
CREATE INDEX audit_platform_time_page ON audit_logs(created_at DESC,id DESC);
