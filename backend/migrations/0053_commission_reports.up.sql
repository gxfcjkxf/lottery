-- Separate, audited read/export permissions; no economic or rollout changes.
INSERT INTO permissions(key) VALUES
 ('report_commission.view.brand'),('report_commission.view.platform'),
 ('report_commission.export.brand'),('report_commission.export.platform') ON CONFLICT DO NOTHING;
INSERT INTO role_permissions(role_id,permission_key)
 SELECT r.id,p.key FROM roles r CROSS JOIN permissions p
 WHERE r.is_bootstrap AND r.brand_id IS NOT NULL AND p.key IN('report_commission.view.brand','report_commission.export.brand')
 ON CONFLICT DO NOTHING;
INSERT INTO role_permissions(role_id,permission_key)
 SELECT r.id,p.key FROM roles r CROSS JOIN permissions p
 WHERE r.is_bootstrap AND r.brand_id IS NULL AND p.key IN('report_commission.view.platform','report_commission.export.platform')
 ON CONFLICT DO NOTHING;
CREATE INDEX commission_report_posted_business_entries ON point_ledger_entries(brand_id,created_at,id)
 WHERE entry_type IN('commission','commission_adjustment');
