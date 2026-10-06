CREATE TABLE bet_order_exceptions (
 id uuid PRIMARY KEY,brand_id uuid NOT NULL,order_id uuid NOT NULL UNIQUE,
 order_version bigint NOT NULL CHECK(order_version>1),marked_by uuid NOT NULL REFERENCES admin_accounts(id),
 reason text NOT NULL CHECK(length(trim(reason))>0 AND octet_length(reason)<=500),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(brand_id,order_id) REFERENCES bet_orders(brand_id,id)
);
CREATE FUNCTION guard_bet_exception() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'bet exception evidence cannot be changed'; END IF;
 IF NOT EXISTS(SELECT 1 FROM bet_orders WHERE brand_id=NEW.brand_id AND id=NEW.order_id AND status='placed' AND version=NEW.order_version-1) THEN RAISE EXCEPTION 'invalid exception evidence state'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER immutable_bet_exception BEFORE INSERT OR UPDATE OR DELETE ON bet_order_exceptions FOR EACH ROW EXECUTE FUNCTION guard_bet_exception();
CREATE FUNCTION validate_bet_exception_final_state() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM bet_orders WHERE brand_id=NEW.brand_id AND id=NEW.order_id AND version>=NEW.order_version AND status IN ('abnormal','bet_cancelled','judged_cancelled')) THEN RAISE EXCEPTION 'exception evidence must commit with its classification'; END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER bet_exception_final_state AFTER INSERT ON bet_order_exceptions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_bet_exception_final_state();

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
  SELECT * INTO l FROM point_ledger_entries WHERE id=NEW.refund_entry_id;
  IF l.entry_type<>'refund' OR l.reference_type<>'bet_order' OR l.reference_id IS DISTINCT FROM OLD.id OR l.reversal_of IS DISTINCT FROM OLD.debit_entry_id OR l.source_allocation<>OLD.deduction_allocation OR NEW.cancel_reason='' THEN RAISE EXCEPTION 'invalid bet refund evidence'; END IF;
 END IF;
 RETURN NEW;
END $$;
INSERT INTO permissions(key) VALUES('bet.mark_abnormal.brand') ON CONFLICT DO NOTHING;
INSERT INTO role_permissions(role_id,permission_key) SELECT id,'bet.mark_abnormal.brand' FROM roles WHERE is_bootstrap AND brand_id IS NOT NULL ON CONFLICT DO NOTHING;
