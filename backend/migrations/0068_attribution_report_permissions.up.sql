-- Attribution reporting is a read-only projection of immutable placement-time
-- snapshots. It grants no commission, reward, wallet, or payment authority.
INSERT INTO permissions(key) VALUES
 ('report_attribution.view.brand'),('report_attribution.view.platform'),
 ('report_attribution.export.brand'),('report_attribution.export.platform')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions(role_id,permission_key)
 SELECT r.id,p.key FROM roles r CROSS JOIN permissions p
 WHERE r.is_bootstrap AND
  ((r.brand_id IS NOT NULL AND p.key IN('report_attribution.view.brand','report_attribution.export.brand')) OR
   (r.brand_id IS NULL AND p.key IN('report_attribution.view.platform','report_attribution.export.platform')))
ON CONFLICT DO NOTHING;
