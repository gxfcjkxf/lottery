CREATE TABLE brand_bet_policies (
 brand_id uuid PRIMARY KEY REFERENCES brands(id),version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 config jsonb NOT NULL CHECK(jsonb_typeof(config)='object'),updated_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE game_bet_policies (
 brand_id uuid NOT NULL,game_id uuid PRIMARY KEY,version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 config jsonb NOT NULL CHECK(jsonb_typeof(config)='object'),updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(brand_id,game_id),FOREIGN KEY(brand_id,game_id) REFERENCES games(brand_id,id)
);
CREATE FUNCTION default_brand_bet_policy() RETURNS jsonb LANGUAGE sql IMMUTABLE AS $$ SELECT '{"min_bet_points":"1","max_bet_points":null,"max_period_points":null,"max_user_period_points":null,"user_cancel_allowed":false}'::jsonb $$;
CREATE FUNCTION default_game_bet_policy() RETURNS jsonb LANGUAGE sql IMMUTABLE AS $$ SELECT '{"min_bet_points":{"mode":"inherit","points":null},"max_bet_points":{"mode":"inherit","points":null},"max_period_points":{"mode":"inherit","points":null},"max_user_period_points":{"mode":"inherit","points":null},"user_cancel_allowed":null}'::jsonb $$;
INSERT INTO brand_bet_policies(brand_id,config) SELECT id,default_brand_bet_policy() FROM brands;
INSERT INTO game_bet_policies(brand_id,game_id,config) SELECT brand_id,id,default_game_bet_policy() FROM games;
CREATE FUNCTION initialize_bet_policy() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_TABLE_NAME='brands' THEN INSERT INTO brand_bet_policies(brand_id,config) VALUES(NEW.id,default_brand_bet_policy());
 ELSE INSERT INTO game_bet_policies(brand_id,game_id,config) VALUES(NEW.brand_id,NEW.id,default_game_bet_policy()); END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER brand_default_bet_policy AFTER INSERT ON brands FOR EACH ROW EXECUTE FUNCTION initialize_bet_policy();
CREATE TRIGGER game_default_bet_policy AFTER INSERT ON games FOR EACH ROW EXECUTE FUNCTION initialize_bet_policy();

CREATE TABLE bet_orders (
 id uuid PRIMARY KEY,brand_id uuid NOT NULL,global_user_id uuid NOT NULL,brand_member_id uuid NOT NULL,account_id uuid NOT NULL,
 game_id uuid NOT NULL,period_id uuid NOT NULL,play_id uuid NOT NULL,rule_version_id uuid NOT NULL,
 definition_snapshot jsonb NOT NULL CHECK(jsonb_typeof(definition_snapshot)='object'),definition_hash text NOT NULL CHECK(definition_hash ~ '^[0-9a-f]{64}$'),
 status text NOT NULL DEFAULT 'placed' CHECK(status IN ('placed','abnormal','won','lost','settled','bet_cancelled','judged_cancelled')),
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 selection_raw jsonb NOT NULL CHECK(jsonb_typeof(selection_raw)='object'),
 selection_normalized jsonb NOT NULL CHECK(jsonb_typeof(selection_normalized)='object'),
 expanded_bets jsonb NOT NULL CHECK(jsonb_typeof(expanded_bets)='array'),
 unit_points bigint NOT NULL CHECK(unit_points>0),combination_count integer NOT NULL CHECK(combination_count BETWEEN 1 AND 10000),
 multiplier bigint NOT NULL CHECK(multiplier>0),total_points bigint NOT NULL CHECK(total_points>0),
 deduction_allocation jsonb NOT NULL CHECK(jsonb_typeof(deduction_allocation)='array'),
 policy_snapshot jsonb NOT NULL CHECK(jsonb_typeof(policy_snapshot)='object'),
 brand_policy_version bigint NOT NULL CHECK(brand_policy_version>0),game_policy_version bigint NOT NULL CHECK(game_policy_version>0),
 debit_entry_id uuid NOT NULL,refund_entry_id uuid,client_key text NOT NULL CHECK(client_key ~ '^[A-Za-z0-9_:.-]{8,128}$'),
 placed_at timestamptz NOT NULL DEFAULT clock_timestamp(),cancelled_at timestamptz,cancel_reason text NOT NULL DEFAULT '',
 UNIQUE(brand_id,id),UNIQUE(brand_id,brand_member_id,client_key),
 FOREIGN KEY(brand_id,brand_member_id,global_user_id) REFERENCES brand_members(brand_id,id,global_user_id),
 FOREIGN KEY(brand_id,account_id,brand_member_id) REFERENCES point_accounts(brand_id,id,brand_member_id),
 FOREIGN KEY(brand_id,game_id,period_id) REFERENCES periods(brand_id,game_id,id),
 FOREIGN KEY(brand_id,game_id,play_id,rule_version_id) REFERENCES rule_versions(brand_id,game_id,play_id,id),
 FOREIGN KEY(brand_id,account_id,debit_entry_id) REFERENCES point_ledger_entries(brand_id,account_id,id),
 FOREIGN KEY(brand_id,account_id,refund_entry_id) REFERENCES point_ledger_entries(brand_id,account_id,id),
 CHECK(jsonb_array_length(expanded_bets)=combination_count),
 CHECK(total_points::numeric=unit_points::numeric*combination_count::numeric*multiplier::numeric),
 CHECK((status IN ('bet_cancelled','judged_cancelled'))=(refund_entry_id IS NOT NULL)),
 CHECK((refund_entry_id IS NOT NULL)=(cancelled_at IS NOT NULL))
);
CREATE INDEX bet_orders_member_history ON bet_orders(brand_id,brand_member_id,placed_at DESC,id DESC);
CREATE INDEX bet_orders_period_stakes ON bet_orders(brand_id,period_id) INCLUDE(brand_member_id,total_points,status);
CREATE INDEX bet_orders_game_history ON bet_orders(brand_id,game_id,placed_at DESC,id DESC);
CREATE FUNCTION guard_bet_order() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p periods; r rule_versions; l point_ledger_entries;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'bet history cannot be deleted'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.status<>'placed' OR NEW.version<>1 OR NEW.refund_entry_id IS NOT NULL OR NEW.cancel_reason<>'' THEN RAISE EXCEPTION 'invalid initial bet state'; END IF;
  SELECT * INTO p FROM periods WHERE id=NEW.period_id;
  IF p.status<>'betting' OR NEW.placed_at<p.bet_start_at OR NEW.placed_at>=p.bet_end_at OR NEW.placed_at>clock_timestamp() OR clock_timestamp()<p.bet_start_at OR clock_timestamp()>=p.bet_end_at THEN RAISE EXCEPTION 'bet outside period window'; END IF;
  SELECT * INTO r FROM rule_versions WHERE id=NEW.rule_version_id;
  IF r.status<>'active' OR r.definition<>NEW.definition_snapshot OR r.definition_sha256<>NEW.definition_hash OR NOT EXISTS(SELECT 1 FROM play_definitions WHERE id=NEW.play_id AND active_version_id=NEW.rule_version_id AND status='active') THEN RAISE EXCEPTION 'bet rule snapshot mismatch'; END IF;
  SELECT * INTO l FROM point_ledger_entries WHERE id=NEW.debit_entry_id;
  IF l.entry_type<>'bet' OR l.reference_type<>'bet_order' OR l.reference_id IS DISTINCT FROM NEW.id OR l.member_id<>NEW.brand_member_id OR l.source_allocation<>NEW.deduction_allocation OR l.actor_type<>'user' OR l.actor_id IS DISTINCT FROM NEW.global_user_id THEN RAISE EXCEPTION 'bet debit reference mismatch'; END IF;
  IF (SELECT sum((a->>'points')::numeric) FROM jsonb_array_elements(NEW.deduction_allocation) a) IS DISTINCT FROM NEW.total_points::numeric OR EXISTS(SELECT 1 FROM jsonb_array_elements(NEW.deduction_allocation) a WHERE a->>'state'<>'available') THEN RAISE EXCEPTION 'invalid bet source allocation'; END IF;
  IF (SELECT sum((s.value->>'available')::numeric) FROM jsonb_each(l.delta_snapshot) s) IS DISTINCT FROM -NEW.total_points::numeric OR EXISTS(SELECT 1 FROM jsonb_each(l.delta_snapshot) s WHERE (s.value->>'manual_frozen')::numeric<>0 OR (s.value->>'system_frozen')::numeric<>0 OR (s.value->>'withdrawal')::numeric<>0) THEN RAISE EXCEPTION 'bet must debit available points only'; END IF;
 ELSE
  IF (to_jsonb(NEW)-ARRAY['status','version','refund_entry_id','cancelled_at','cancel_reason']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['status','version','refund_entry_id','cancelled_at','cancel_reason']) OR NEW.version<>OLD.version+1 THEN RAISE EXCEPTION 'immutable bet identity or invalid version'; END IF;
  IF NOT (OLD.status IN ('placed','abnormal') AND NEW.status IN ('bet_cancelled','judged_cancelled')) THEN RAISE EXCEPTION 'invalid bet transition'; END IF;
  SELECT * INTO l FROM point_ledger_entries WHERE id=NEW.refund_entry_id;
  IF l.entry_type<>'refund' OR l.reference_type<>'bet_order' OR l.reference_id IS DISTINCT FROM OLD.id OR l.reversal_of IS DISTINCT FROM OLD.debit_entry_id OR l.source_allocation<>OLD.deduction_allocation OR NEW.cancel_reason='' THEN RAISE EXCEPTION 'invalid bet refund evidence'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER immutable_bet_order BEFORE INSERT OR UPDATE OR DELETE ON bet_orders FOR EACH ROW EXECUTE FUNCTION guard_bet_order();
INSERT INTO permissions(key) VALUES ('bet_policy.view.brand'),('bet_policy.view.platform'),('bet_policy.write.brand'),('bet.view.brand'),('bet.view.platform'),('bet.cancel.brand') ON CONFLICT DO NOTHING;
INSERT INTO role_permissions(role_id,permission_key)
 SELECT r.id,p.key FROM roles r CROSS JOIN permissions p WHERE r.is_bootstrap AND
 ((r.brand_id IS NOT NULL AND p.key IN ('bet_policy.view.brand','bet_policy.write.brand','bet.view.brand','bet.cancel.brand')) OR
 (r.brand_id IS NULL AND p.key IN ('bet_policy.view.platform','bet.view.platform') AND EXISTS(SELECT 1 FROM admin_account_roles ar JOIN admin_accounts a ON a.id=ar.account_id WHERE ar.role_id=r.id AND a.is_super_admin))) ON CONFLICT DO NOTHING;
