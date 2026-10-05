CREATE TABLE game_schedules (
 id uuid PRIMARY KEY,brand_id uuid NOT NULL,game_id uuid NOT NULL,
 revision bigint NOT NULL CHECK(revision>0),spec jsonb NOT NULL CHECK(jsonb_typeof(spec)='object'),
 created_by uuid NOT NULL REFERENCES admin_accounts(id),created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(brand_id,game_id,id),UNIQUE(brand_id,game_id,revision),
 FOREIGN KEY(brand_id,game_id) REFERENCES games(brand_id,id)
);
ALTER TABLE games ADD COLUMN schedule_id uuid;
ALTER TABLE games ADD FOREIGN KEY(brand_id,id,schedule_id) REFERENCES game_schedules(brand_id,game_id,id);
ALTER TABLE periods ADD COLUMN schedule_id uuid;
ALTER TABLE periods ADD COLUMN state_reason text NOT NULL DEFAULT '';
ALTER TABLE periods ADD FOREIGN KEY(brand_id,game_id,schedule_id) REFERENCES game_schedules(brand_id,game_id,id);
CREATE INDEX periods_due ON periods(bet_start_at,bet_end_at,draw_at) WHERE status IN ('pending','betting','closed');
CREATE TRIGGER immutable_game_schedules BEFORE UPDATE OR DELETE ON game_schedules FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION guard_period_history() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'period history cannot be deleted'; END IF;
 IF NEW.id<>OLD.id OR NEW.brand_id<>OLD.brand_id OR NEW.game_id<>OLD.game_id OR NEW.period_no<>OLD.period_no OR NEW.sequence<>OLD.sequence OR NEW.bet_start_at<>OLD.bet_start_at OR NEW.bet_end_at<>OLD.bet_end_at OR NEW.draw_at<>OLD.draw_at OR NEW.schedule_id IS DISTINCT FROM OLD.schedule_id OR NEW.created_at<>OLD.created_at OR NEW.version<>OLD.version+1 THEN RAISE EXCEPTION 'immutable period identity or invalid version'; END IF;
 IF NOT ((OLD.status='pending' AND NEW.status IN ('betting','judged_cancelled')) OR (OLD.status='betting' AND NEW.status IN ('closed','bet_cancelled','judged_cancelled')) OR (OLD.status='closed' AND NEW.status IN ('waiting_draw','judged_cancelled')) OR (OLD.status='waiting_draw' AND NEW.status IN ('drawn','judged_cancelled')) OR (OLD.status='drawn' AND NEW.status='settling') OR (OLD.status='settling' AND NEW.status='settled')) THEN RAISE EXCEPTION 'invalid period transition'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER immutable_period_history BEFORE UPDATE OR DELETE ON periods FOR EACH ROW EXECUTE FUNCTION guard_period_history();
INSERT INTO permissions(key) VALUES ('schedule.view.brand'),('schedule.view.platform'),('schedule.write.brand'),('period.view.brand'),('period.view.platform'),('period.generate.brand') ON CONFLICT DO NOTHING;
INSERT INTO role_permissions(role_id,permission_key)
 SELECT r.id,p.key FROM roles r CROSS JOIN permissions p WHERE r.is_bootstrap AND
 ((r.brand_id IS NOT NULL AND p.key IN ('schedule.view.brand','schedule.write.brand','period.view.brand','period.generate.brand')) OR
 (r.brand_id IS NULL AND p.key IN ('schedule.view.platform','period.view.platform') AND EXISTS(SELECT 1 FROM admin_account_roles ar JOIN admin_accounts a ON a.id=ar.account_id WHERE ar.role_id=r.id AND a.is_super_admin))) ON CONFLICT DO NOTHING;
