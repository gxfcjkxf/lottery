CREATE TABLE bet_order_judgments (
 id uuid PRIMARY KEY,brand_id uuid NOT NULL,game_id uuid NOT NULL,period_id uuid NOT NULL,order_id uuid NOT NULL UNIQUE,
 order_version bigint NOT NULL CHECK(order_version>1),cause text NOT NULL CHECK(cause IN ('no_result','invalid_result')),
 draw_result_id uuid,judged_by uuid NOT NULL REFERENCES admin_accounts(id),
 reason text NOT NULL CHECK(length(trim(reason))>0 AND octet_length(reason)<=500),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 CHECK(cause<>'no_result' OR draw_result_id IS NULL),
 FOREIGN KEY(brand_id,period_id,order_id) REFERENCES bet_orders(brand_id,period_id,id),
 FOREIGN KEY(brand_id,game_id,period_id) REFERENCES periods(brand_id,game_id,id),
 FOREIGN KEY(brand_id,game_id,period_id,draw_result_id) REFERENCES draw_results(brand_id,game_id,period_id,id)
);
CREATE FUNCTION guard_bet_judgment() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'judgment evidence cannot be changed'; END IF;
 IF NOT EXISTS(SELECT 1 FROM bet_orders o JOIN periods p ON p.id=o.period_id WHERE o.id=NEW.order_id AND o.brand_id=NEW.brand_id AND o.version=NEW.order_version-1 AND o.status IN ('placed','abnormal') AND p.status NOT IN ('settling','settled') AND p.draw_result_id IS NOT DISTINCT FROM NEW.draw_result_id) THEN RAISE EXCEPTION 'invalid judgment state or result snapshot'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_bet_judgment BEFORE INSERT OR UPDATE OR DELETE ON bet_order_judgments FOR EACH ROW EXECUTE FUNCTION guard_bet_judgment();
CREATE FUNCTION validate_bet_judgment_final_state() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM bet_orders WHERE id=NEW.order_id AND brand_id=NEW.brand_id AND version=NEW.order_version AND status='judged_cancelled' AND refund_entry_id IS NOT NULL AND cancel_reason=NEW.reason) THEN RAISE EXCEPTION 'judgment must commit with exact cancelled order and refund'; END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER bet_judgment_final_state AFTER INSERT ON bet_order_judgments DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_bet_judgment_final_state();

CREATE OR REPLACE FUNCTION guard_bet_order() RETURNS trigger LANGUAGE plpgsql AS $$
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
  IF OLD.status='placed' AND NEW.status='abnormal' THEN
   IF NEW.refund_entry_id IS DISTINCT FROM OLD.refund_entry_id OR NEW.cancelled_at IS DISTINCT FROM OLD.cancelled_at OR NEW.cancel_reason IS DISTINCT FROM OLD.cancel_reason OR NOT EXISTS(SELECT 1 FROM bet_order_exceptions WHERE brand_id=NEW.brand_id AND order_id=NEW.id AND order_version=NEW.version) THEN RAISE EXCEPTION 'invalid exception classification'; END IF;
   RETURN NEW;
  END IF;
  IF NOT (OLD.status IN ('placed','abnormal') AND NEW.status IN ('bet_cancelled','judged_cancelled')) THEN RAISE EXCEPTION 'invalid bet transition'; END IF;
  IF NEW.status='judged_cancelled' AND NOT EXISTS(SELECT 1 FROM bet_order_judgments WHERE brand_id=NEW.brand_id AND order_id=NEW.id AND order_version=NEW.version AND reason=NEW.cancel_reason) AND NOT EXISTS(SELECT 1 FROM period_cancellation_targets t JOIN period_cancellations c ON c.id=t.cancellation_id JOIN periods phase ON phase.id=c.period_id WHERE t.brand_id=NEW.brand_id AND t.order_id=NEW.id AND t.state='pending' AND c.mode='judged_cancelled' AND c.state='processing' AND phase.status=c.mode AND phase.version=c.period_version) THEN RAISE EXCEPTION 'judged refund needs individual or period judgment witness'; END IF;
  SELECT * INTO l FROM point_ledger_entries WHERE id=NEW.refund_entry_id;
  IF l.entry_type<>'refund' OR l.reference_type<>'bet_order' OR l.reference_id IS DISTINCT FROM OLD.id OR l.reversal_of IS DISTINCT FROM OLD.debit_entry_id OR l.source_allocation<>OLD.deduction_allocation OR NEW.cancel_reason='' THEN RAISE EXCEPTION 'invalid bet refund evidence'; END IF;
 END IF;
 RETURN NEW;
END $$;
INSERT INTO permissions(key) VALUES('bet.judge_cancel.brand') ON CONFLICT DO NOTHING;
INSERT INTO role_permissions(role_id,permission_key) SELECT id,'bet.judge_cancel.brand' FROM roles WHERE is_bootstrap AND brand_id IS NOT NULL ON CONFLICT DO NOTHING;
