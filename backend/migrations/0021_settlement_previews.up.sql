-- Computation evidence only; no order/period transitions or wallet writes.
CREATE TABLE settlement_previews (
 id uuid PRIMARY KEY,
 brand_id uuid NOT NULL,
 game_id uuid NOT NULL,
 period_id uuid NOT NULL,
 order_id uuid NOT NULL,
 order_version bigint NOT NULL CHECK(order_version>0),
 order_status text NOT NULL,
 period_version bigint NOT NULL CHECK(period_version>0),
 period_status text NOT NULL CHECK(period_status='drawn'),
 draw_result_id uuid NOT NULL,
 definition_hash text NOT NULL CHECK(definition_hash ~ '^[0-9a-f]{64}$'),
 draw_hash text NOT NULL CHECK(draw_hash ~ '^[0-9a-f]{64}$'),
 draw jsonb NOT NULL CHECK(jsonb_typeof(draw)='object'),
 outcome text NOT NULL CHECK(outcome IN ('won','lost','abnormal','excluded')),
 error_code text,
 calculation jsonb,
 created_by uuid NOT NULL REFERENCES admin_accounts(id),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 reason text NOT NULL CHECK(length(btrim(reason))>0 AND octet_length(reason)<=500),
 audit_log_id uuid NOT NULL REFERENCES audit_logs(id),
 UNIQUE(brand_id,id),
 FOREIGN KEY(brand_id,period_id,order_id) REFERENCES bet_orders(brand_id,period_id,id),
 FOREIGN KEY(brand_id,game_id,period_id) REFERENCES periods(brand_id,game_id,id),
 FOREIGN KEY(brand_id,game_id,period_id,draw_result_id) REFERENCES draw_results(brand_id,game_id,period_id,id),
 CHECK((outcome IN ('won','lost') AND error_code IS NULL AND calculation IS NOT NULL AND jsonb_typeof(calculation)='object') OR
 (outcome IN ('abnormal','excluded') AND error_code IS NOT NULL AND calculation IS NULL))
);
CREATE INDEX settlement_preview_order_history ON settlement_previews(brand_id,order_id,created_at DESC,id DESC);
CREATE TRIGGER immutable_settlement_preview BEFORE UPDATE OR DELETE ON settlement_previews FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION validate_settlement_preview_context() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM bet_orders o JOIN periods p ON p.id=o.period_id JOIN draw_results d ON d.id=p.draw_result_id
 WHERE o.id=NEW.order_id AND o.brand_id=NEW.brand_id AND o.game_id=NEW.game_id AND o.period_id=NEW.period_id
 AND o.version=NEW.order_version AND o.status=NEW.order_status AND o.definition_hash=NEW.definition_hash
 AND p.version=NEW.period_version AND p.status='drawn' AND p.draw_result_id=NEW.draw_result_id AND d.result_hash=NEW.draw_hash AND d.result=NEW.draw) THEN
 RAISE EXCEPTION 'settlement preview requires current locked order and result'; END IF;
 IF NOT EXISTS(SELECT 1 FROM audit_logs a WHERE a.id=NEW.audit_log_id AND a.brand_id=NEW.brand_id AND a.action='settlement.preview'
 AND a.resource_type='settlement_preview' AND a.resource_id=NEW.id AND a.actor_id=NEW.created_by) THEN RAISE EXCEPTION 'settlement preview requires audit witness'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER validate_settlement_preview_context BEFORE INSERT ON settlement_previews FOR EACH ROW EXECUTE FUNCTION validate_settlement_preview_context();
INSERT INTO permissions(key) VALUES('settlement.view.brand'),('settlement.view.platform'),('settlement.preview.brand') ON CONFLICT DO NOTHING;
INSERT INTO role_permissions(role_id,permission_key)
 SELECT r.id,p.key FROM roles r CROSS JOIN permissions p WHERE r.is_bootstrap AND
 ((r.brand_id IS NOT NULL AND p.key IN ('settlement.view.brand','settlement.preview.brand')) OR
 (r.brand_id IS NULL AND p.key='settlement.view.platform' AND EXISTS(SELECT 1 FROM admin_account_roles ar JOIN admin_accounts a ON a.id=ar.account_id WHERE ar.role_id=r.id AND a.is_super_admin))) ON CONFLICT DO NOTHING;
