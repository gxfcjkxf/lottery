-- Confirmed manual/recalculation policy: use the new calculation as target,
-- actual granted net as basis. Preserve prior adjustment/plan/audit histories.
-- No data backfill, gate change or automatic recovery of historical blocks.
CREATE OR REPLACE FUNCTION guard_commission_correction_plan() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE c commission_cycles; pay commission_payments; step commission_correction_plan_steps; a audit_logs; n bigint; before_sum numeric; after_sum numeric; credits numeric; debits numeric; recovering boolean:=false;
BEGIN
 IF TG_OP='UPDATE' THEN
  recovering:=OLD.state='blocked' AND OLD.last_error_code='COMMISSION_CORRECTION_MANUAL_POLICY_UNRESOLVED'
   AND OLD.planned_count=0 AND OLD.cursor_agent_id IS NULL AND OLD.credit_points IS NULL AND OLD.debit_points IS NULL AND OLD.net_points IS NULL AND NEW.state='planning';
 END IF;
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'commission correction plan history retained'; END IF;
 IF TG_OP='UPDATE' AND NEW.version=OLD.version AND (to_jsonb(NEW)-'next_work_at')=(to_jsonb(OLD)-'next_work_at') THEN RETURN NEW; END IF;
 SELECT * INTO c FROM commission_cycles WHERE brand_id=NEW.brand_id AND id=NEW.cycle_id FOR UPDATE NOWAIT;
 SELECT * INTO pay FROM commission_payments WHERE brand_id=NEW.brand_id AND id=NEW.payment_id FOR SHARE NOWAIT;
 IF TG_OP='INSERT' THEN
  PERFORM 1 FROM brands WHERE id=NEW.brand_id AND status<>'disabled' FOR SHARE NOWAIT;
  IF NOT FOUND THEN RAISE EXCEPTION 'correction preparation brand unavailable'; END IF;
  PERFORM 1 FROM brand_commission_payment_policies WHERE brand_id=NEW.brand_id AND enabled FOR SHARE NOWAIT;
  IF NOT FOUND OR pay.cycle_id IS DISTINCT FROM NEW.cycle_id OR NOT commission_payment_evidence_current(NEW.brand_id,NEW.cycle_id,NEW.run_id,NEW.evidence_epoch) OR
   NOT commission_correction_source_valid(NEW.brand_id,NEW.payment_id,NEW.run_id) THEN RAISE EXCEPTION 'correction preparation requires actual prior credit and current evidence'; END IF;
  SELECT count(*),coalesce(sum(points_before::numeric),0),coalesce(sum(points_after::numeric),0),coalesce(sum(greatest(delta_points,0)::numeric),0),coalesce(sum(greatest(-delta_points,0)::numeric),0)
   INTO n,before_sum,after_sum,credits,debits FROM commission_correction_candidates(NEW.brand_id,NEW.payment_id,NEW.run_id);
  SELECT * INTO a FROM audit_logs WHERE id=NEW.creation_audit_log_id;
  IF n>100000 OR NEW.target_count<>n OR NEW.before_points<>before_sum OR NEW.calculated_points<>after_sum OR
   NEW.payout_mode IS DISTINCT FROM commission_correction_mode(pay.payout_mode,commission_payment_mode(NEW.run_id)) OR
   NEW.version<>1 OR NEW.planned_count<>0 OR NEW.cursor_agent_id IS NOT NULL OR NEW.creation_xid<>pg_current_xact_id() OR NEW.last_audit_log_id<>NEW.creation_audit_log_id OR
   NEW.state<>'planning' OR NEW.last_error_code IS NOT NULL OR NEW.credit_points IS DISTINCT FROM credits OR NEW.debit_points IS DISTINCT FROM debits OR NEW.net_points IS DISTINCT FROM after_sum-before_sum OR
   a.id IS NULL OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.actor_type<>'system' OR a.actor_id IS NOT NULL OR a.action<>'commission.correction_plan.create' OR a.resource_type<>'commission_correction_plan' OR a.resource_id IS DISTINCT FROM NEW.id
   THEN RAISE EXCEPTION 'correction plan source/amount/version mismatch'; END IF;
 ELSE
  IF (to_jsonb(NEW)-(ARRAY['state','version','planned_count','cursor_agent_id','last_audit_log_id','last_error_code','updated_at','next_work_at'] || CASE WHEN recovering THEN ARRAY['credit_points','debit_points','net_points'] ELSE ARRAY[]::text[] END)) IS DISTINCT FROM
   (to_jsonb(OLD)-(ARRAY['state','version','planned_count','cursor_agent_id','last_audit_log_id','last_error_code','updated_at','next_work_at'] || CASE WHEN recovering THEN ARRAY['credit_points','debit_points','net_points'] ELSE ARRAY[]::text[] END)) OR NEW.version<>OLD.version+1
   THEN RAISE EXCEPTION 'correction plan identity and amounts immutable'; END IF;
  SELECT * INTO step FROM commission_correction_plan_steps WHERE brand_id=NEW.brand_id AND plan_id=NEW.id AND version=NEW.version;
  IF step.id IS NULL OR step.from_state IS DISTINCT FROM OLD.state OR step.to_state<>NEW.state OR step.audit_log_id<>NEW.last_audit_log_id OR
   step.planned_count<>NEW.planned_count OR step.cursor_agent_id IS DISTINCT FROM NEW.cursor_agent_id OR step.error_code IS DISTINCT FROM NEW.last_error_code THEN RAISE EXCEPTION 'correction plan update requires matching immutable step'; END IF;
  IF NOT ((step.operation='page' AND OLD.state='planning' AND NEW.state='planning' AND NEW.planned_count>OLD.planned_count AND NEW.planned_count<=OLD.planned_count+100) OR
   (step.operation='ready' AND OLD.state='planning' AND NEW.state='ready' AND NEW.planned_count=OLD.planned_count) OR
   (step.operation='invalidate' AND OLD.state<>'stale' AND NEW.state='stale' AND NEW.planned_count=OLD.planned_count) OR
   (step.operation='fail' AND OLD.state='planning' AND NEW.state='failed' AND NEW.planned_count=OLD.planned_count) OR
   (step.operation='retry' AND (OLD.state='failed' OR recovering) AND NEW.state='planning' AND NEW.planned_count=OLD.planned_count)) THEN RAISE EXCEPTION 'correction plan transition invalid'; END IF;
  IF step.operation NOT IN('invalidate','fail') THEN
   PERFORM 1 FROM brands WHERE id=NEW.brand_id AND status<>'disabled' FOR SHARE NOWAIT;
   IF NOT FOUND THEN RAISE EXCEPTION 'correction plan brand disabled'; END IF;
   PERFORM 1 FROM brand_commission_payment_policies WHERE brand_id=NEW.brand_id AND enabled FOR SHARE NOWAIT;
   IF NOT FOUND OR NOT commission_payment_evidence_current(NEW.brand_id,NEW.cycle_id,NEW.run_id,NEW.evidence_epoch) OR NOT commission_correction_source_valid(NEW.brand_id,NEW.payment_id,NEW.run_id) THEN RAISE EXCEPTION 'correction plan evidence unavailable'; END IF;
  END IF;
  IF step.operation='invalidate' AND commission_payment_evidence_current(NEW.brand_id,NEW.cycle_id,NEW.run_id,NEW.evidence_epoch) THEN RAISE EXCEPTION 'cannot stale a current correction plan'; END IF;
  IF recovering THEN
   SELECT count(*),coalesce(sum(points_before::numeric),0),coalesce(sum(points_after::numeric),0),coalesce(sum(greatest(delta_points,0)::numeric),0),coalesce(sum(greatest(-delta_points,0)::numeric),0)
    INTO n,before_sum,after_sum,credits,debits FROM commission_correction_candidates(NEW.brand_id,NEW.payment_id,NEW.run_id);
   IF NEW.target_count<>n OR NEW.before_points<>before_sum OR NEW.calculated_points<>after_sum OR NEW.credit_points IS DISTINCT FROM credits OR NEW.debit_points IS DISTINCT FROM debits OR NEW.net_points IS DISTINCT FROM after_sum-before_sum
    THEN RAISE EXCEPTION 'historical correction recovery requires unchanged frozen basis and exact confirmed differences'; END IF;
  END IF;
  SELECT count(*) INTO n FROM commission_correction_plan_targets WHERE plan_id=NEW.id;
  IF n<>NEW.planned_count OR NEW.cursor_agent_id IS DISTINCT FROM (SELECT agent_id FROM commission_correction_plan_targets WHERE plan_id=NEW.id ORDER BY agent_id DESC LIMIT 1) THEN RAISE EXCEPTION 'correction plan count/cursor mismatch'; END IF;
  IF NEW.state='ready' THEN
   SELECT coalesce(sum(points_before::numeric),0),coalesce(sum(points_after::numeric),0),coalesce(sum(greatest(delta_points,0)::numeric),0),coalesce(sum(greatest(-delta_points,0)::numeric),0)
    INTO before_sum,after_sum,credits,debits FROM commission_correction_plan_targets WHERE plan_id=NEW.id;
   IF NEW.before_points<>before_sum OR NEW.calculated_points<>after_sum OR NEW.credit_points<>credits OR NEW.debit_points<>debits THEN RAISE EXCEPTION 'correction plan cannot be ready with incomplete differences'; END IF;
  END IF;
  NEW.updated_at:=clock_timestamp();
 END IF;
 RETURN NEW;
END $$;

-- CREATE OR REPLACE retains the function identity; scope resolution stays pinned.
DO $$ DECLARE app_schema text:=current_schema(); BEGIN
 EXECUTE format('ALTER FUNCTION guard_commission_correction_plan() SET search_path TO pg_catalog, %I, pg_temp',app_schema);
END $$;
