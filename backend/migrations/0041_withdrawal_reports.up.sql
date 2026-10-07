INSERT INTO permissions(key) VALUES
 ('report_withdrawal.view.brand'),('report_withdrawal.view.platform'),
 ('report_withdrawal.export.brand'),('report_withdrawal.export.platform')
ON CONFLICT DO NOTHING;
