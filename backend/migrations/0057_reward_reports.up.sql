-- Reports are readonly financial projections with independent view/export
-- rights. They never grant reward writes or backfill balances/order history.
INSERT INTO permissions(key) VALUES
 ('report_reward.view.brand'),('report_reward.view.platform'),
 ('report_reward.export.brand'),('report_reward.export.platform') ON CONFLICT DO NOTHING;
INSERT INTO role_permissions(role_id,permission_key)
 SELECT r.id,p.key FROM roles r CROSS JOIN permissions p WHERE r.is_bootstrap AND
 ((r.brand_id IS NOT NULL AND p.key IN('report_reward.view.brand','report_reward.export.brand')) OR
  (r.brand_id IS NULL AND p.key IN('report_reward.view.platform','report_reward.export.platform'))) ON CONFLICT DO NOTHING;
CREATE INDEX reward_posting_report_window ON point_ledger_entries(brand_id,created_at,id)
 WHERE entry_type IN('reward_grant','reward_reversal') AND reference_type='reward_order';
