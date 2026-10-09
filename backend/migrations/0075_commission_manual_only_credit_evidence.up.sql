-- Zero original payouts do not imply no actual money: independent manual
-- adjustments may already have credited the beneficiary. Preserve and use
-- differences, never stale that history and register another full payment.
CREATE FUNCTION commission_payment_has_actual_money(b uuid,pid uuid) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT EXISTS(SELECT 1 FROM commission_payment_targets t WHERE t.brand_id=b AND t.payment_id=pid AND
  (t.ledger_entry_id IS NOT NULL OR EXISTS(SELECT 1 FROM point_ledger_entries l WHERE l.brand_id=b AND l.reference_type='commission_payment_target' AND l.reference_id=t.id)))
 OR EXISTS(SELECT 1 FROM commission_adjustments a WHERE a.brand_id=b AND a.payment_id=pid AND
  (a.ledger_entry_id IS NOT NULL OR EXISTS(SELECT 1 FROM point_ledger_entries l WHERE l.brand_id=b AND l.reference_type='commission_adjustment' AND l.reference_id=a.id)))
 OR EXISTS(SELECT 1 FROM commission_correction_plans p
  JOIN commission_correction_executions x ON x.brand_id=p.brand_id AND x.plan_id=p.id
  JOIN commission_correction_execution_targets t ON t.brand_id=x.brand_id AND t.execution_id=x.id
  WHERE p.brand_id=b AND p.payment_id=pid AND t.state='applied' AND t.ledger_entry_id IS NOT NULL)
$$;
-- Do not conceal already-staled actual money or automatically repair/payout
-- it. Roll back this whole upgrade so legacy histories can be reviewed first.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM commission_payments p WHERE p.state='stale' AND commission_payment_has_actual_money(p.brand_id,p.id)) THEN
  RAISE EXCEPTION 'stale commission payment has actual money; explicit historical review required before upgrade';
 END IF;
END $$;
CREATE OR REPLACE FUNCTION guard_commission_payment() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE a audit_logs; mode text; expected_state text; credits boolean; legacy_review boolean;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'commission payment immutable history'; END IF;
 PERFORM 1 FROM commission_cycles WHERE brand_id=NEW.brand_id AND id=NEW.cycle_id FOR UPDATE NOWAIT;
 IF TG_OP='INSERT' THEN
  PERFORM 1 FROM brand_commission_payment_policies WHERE brand_id=NEW.brand_id AND enabled FOR SHARE NOWAIT;
  IF NOT FOUND OR NOT commission_payment_evidence_current(NEW.brand_id,NEW.cycle_id,NEW.run_id,NEW.evidence_epoch) THEN RAISE EXCEPTION 'commission payment requires current approved rollout and evidence'; END IF;
  mode:=commission_payment_mode(NEW.run_id);
  expected_state:=CASE mode WHEN 'manual' THEN 'awaiting_approval' WHEN 'mixed' THEN 'awaiting_approval' ELSE 'paying' END;
  IF NEW.version<>1 OR NEW.payout_mode IS DISTINCT FROM mode OR NEW.state<>expected_state OR
   NEW.total_points IS DISTINCT FROM (SELECT coalesce(sum(points),0) FROM commission_earnings WHERE run_id=NEW.run_id) OR
   NEW.target_count<>(SELECT count(*) FROM commission_earnings WHERE run_id=NEW.run_id) OR NEW.last_audit_log_id<>NEW.creation_audit_log_id THEN RAISE EXCEPTION 'commission payment must use complete immutable cycle totals'; END IF;
  SELECT * INTO a FROM audit_logs WHERE id=NEW.creation_audit_log_id;
  IF a.actor_type IS DISTINCT FROM 'system' OR a.actor_id IS NOT NULL OR a.action IS DISTINCT FROM 'commission.payment.create' OR
   a.brand_id IS DISTINCT FROM NEW.brand_id OR a.resource_type IS DISTINCT FROM 'commission_payment' OR a.resource_id IS DISTINCT FROM NEW.id OR
   a.after_json->>'version' IS DISTINCT FROM '1' OR a.after_json->>'state' IS DISTINCT FROM NEW.state OR
   a.after_json->>'run_id' IS DISTINCT FROM NEW.run_id::text OR a.after_json->>'total_points' IS DISTINCT FROM NEW.total_points::text OR
   a.after_json->>'payout_mode' IS DISTINCT FROM NEW.payout_mode THEN RAISE EXCEPTION 'commission payment creation audit mismatch'; END IF;
  IF NEW.state='paying' AND (NEW.approval_actor_type IS DISTINCT FROM 'system' OR NEW.approved_by IS NOT NULL OR NEW.approval_audit_log_id IS DISTINCT FROM NEW.creation_audit_log_id) OR
   NEW.state<>'paying' AND NEW.approval_audit_log_id IS NOT NULL OR
   NEW.last_error_code IS NOT NULL THEN RAISE EXCEPTION 'payment approval must follow saved mode'; END IF;
 ELSE
  legacy_review:=OLD.state='blocked' AND OLD.payout_mode='mixed' AND OLD.last_error_code='COMMISSION_PAYMENT_MODE_UNRESOLVED'
   AND OLD.approval_audit_log_id IS NULL AND NOT EXISTS(SELECT 1 FROM commission_payment_targets WHERE payment_id=OLD.id);
  IF (to_jsonb(NEW)-ARRAY['state','version','approved_by','approval_actor_type','approval_audit_log_id','last_audit_log_id','last_error_code','updated_at','next_work_at']) IS DISTINCT FROM
   (to_jsonb(OLD)-ARRAY['state','version','approved_by','approval_actor_type','approval_audit_log_id','last_audit_log_id','last_error_code','updated_at','next_work_at']) THEN RAISE EXCEPTION 'payment identity and totals immutable'; END IF;
  IF (to_jsonb(NEW)-ARRAY['next_work_at']) IS NOT DISTINCT FROM (to_jsonb(OLD)-ARRAY['next_work_at']) THEN RETURN NEW; END IF;
  IF NEW.version<>OLD.version+1 OR NOT((OLD.state='awaiting_approval' AND NEW.state IN('paying','stale','blocked')) OR
   (OLD.state='paying' AND NEW.state IN('paying','paid','failed','stale','blocked')) OR (OLD.state='failed' AND NEW.state IN('paying','stale','blocked')) OR
   (OLD.state='paid' AND NEW.state IN('blocked','stale')) OR (legacy_review AND NEW.state='paying')) THEN RAISE EXCEPTION 'payment state/version conflict'; END IF;
  SELECT * INTO a FROM audit_logs WHERE id=NEW.last_audit_log_id;
  IF a.id IS NULL OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.resource_type<>'commission_payment' OR a.resource_id IS DISTINCT FROM NEW.id OR
   a.before_json->>'version' IS DISTINCT FROM OLD.version::text OR a.before_json->>'state' IS DISTINCT FROM OLD.state OR
   a.after_json->>'version' IS DISTINCT FROM NEW.version::text OR a.after_json->>'state' IS DISTINCT FROM NEW.state OR
   a.action NOT IN('commission.payment.approve','commission.payment.retry','commission.payment.post','commission.payment.complete','commission.payment.fail','commission.payment.invalidate') THEN RAISE EXCEPTION 'payment state transition audit mismatch'; END IF;
  IF NOT ((a.action='commission.payment.approve' AND (OLD.state='awaiting_approval' OR legacy_review) AND NEW.state='paying' AND a.actor_type='admin' AND a.actor_id IS NOT NULL) OR
   (a.action='commission.payment.retry' AND OLD.state='failed' AND NEW.state='paying' AND a.actor_type='admin' AND a.actor_id IS NOT NULL) OR
   (a.action='commission.payment.post' AND OLD.state='paying' AND NEW.state='paying' AND a.actor_type='system' AND a.actor_id IS NULL) OR
   (a.action='commission.payment.complete' AND OLD.state='paying' AND NEW.state='paid' AND a.actor_type='system' AND a.actor_id IS NULL) OR
   (a.action='commission.payment.fail' AND OLD.state='paying' AND NEW.state='failed' AND a.actor_type='system' AND a.actor_id IS NULL) OR
   (a.action='commission.payment.invalidate' AND NEW.state IN('blocked','stale') AND a.actor_type='system' AND a.actor_id IS NULL)) THEN RAISE EXCEPTION 'payment operation/state/actor mismatch'; END IF;
  IF OLD.approval_audit_log_id IS NOT NULL AND (NEW.approved_by,NEW.approval_actor_type,NEW.approval_audit_log_id) IS DISTINCT FROM (OLD.approved_by,OLD.approval_actor_type,OLD.approval_audit_log_id) THEN RAISE EXCEPTION 'payment approval immutable'; END IF;
  IF (OLD.state='awaiting_approval' OR legacy_review) AND NEW.state='paying' THEN
   IF NEW.payout_mode NOT IN('manual','mixed') OR NEW.approval_actor_type IS DISTINCT FROM 'admin' OR NEW.approved_by IS DISTINCT FROM a.actor_id OR a.actor_type<>'admin' OR
    NEW.approval_audit_log_id IS DISTINCT FROM NEW.last_audit_log_id OR a.action<>'commission.payment.approve' THEN RAISE EXCEPTION 'manual payment requires operator approval'; END IF;
  ELSIF (NEW.approved_by,NEW.approval_actor_type,NEW.approval_audit_log_id) IS DISTINCT FROM (OLD.approved_by,OLD.approval_actor_type,OLD.approval_audit_log_id) THEN RAISE EXCEPTION 'payment approval cannot change'; END IF;
  IF NEW.state IN('paying','paid') AND NOT commission_payment_evidence_current(NEW.brand_id,NEW.cycle_id,NEW.run_id,NEW.evidence_epoch) THEN RAISE EXCEPTION 'payment evidence no longer current'; END IF;
  SELECT commission_payment_has_actual_money(NEW.brand_id,NEW.id) INTO credits;
  IF NEW.state='stale' AND credits THEN RAISE EXCEPTION 'credited payment needs compensation, cannot stale'; END IF;
  IF NEW.state='paid' AND (NEW.target_count<>(SELECT count(*) FROM commission_payment_targets WHERE payment_id=NEW.id AND state='paid') OR
   NEW.total_points IS DISTINCT FROM (SELECT coalesce(sum(points),0) FROM commission_payment_targets WHERE payment_id=NEW.id AND state='paid')) THEN RAISE EXCEPTION 'payment completion incomplete'; END IF;
 END IF;
 NEW.updated_at:=clock_timestamp(); RETURN NEW;
END $$;
CREATE OR REPLACE FUNCTION commission_correction_source_valid(b uuid,pid uuid,rid uuid) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT EXISTS(SELECT 1 FROM commission_payments p JOIN commission_runs r ON r.brand_id=p.brand_id AND r.cycle_id=p.cycle_id AND r.id=rid
  WHERE p.brand_id=b AND p.id=pid AND p.run_id<>rid AND p.state='blocked' AND p.last_error_code='COMMISSION_PAYMENT_CORRECTION_REQUIRED'
  AND commission_correction_heads_valid(b,p.cycle_id)
  AND commission_payment_has_actual_money(b,p.id))
 AND NOT EXISTS(
  SELECT 1 FROM commission_payment_targets t
  LEFT JOIN commission_adjustment_heads h ON h.brand_id=t.brand_id AND h.target_id=t.id
  LEFT JOIN point_ledger_entries l ON l.id=t.ledger_entry_id
  LEFT JOIN audit_logs a ON a.id=t.audit_log_id
  LEFT JOIN commission_earnings e ON e.brand_id=b AND e.run_id=rid AND e.agent_id=t.agent_id
  WHERE t.brand_id=b AND t.payment_id=pid AND t.state='paid' AND (
   h.target_id IS NULL OR h.version=1 AND (h.points<>t.points OR h.last_adjustment_id IS NOT NULL)
   OR h.version>1 AND NOT EXISTS(SELECT 1 FROM commission_adjustments x WHERE x.brand_id=b AND x.target_id=t.id AND x.id=h.last_adjustment_id AND x.version=h.version AND x.points_after=h.points AND x.ledger_entry_id IS NOT NULL)
   OR a.id IS NULL OR a.brand_id IS DISTINCT FROM b OR a.action<>'commission.payment.target' OR a.resource_type<>'commission_payment_target' OR a.resource_id IS DISTINCT FROM t.id
   OR t.points>0 AND (l.id IS NULL OR l.brand_id<>b OR l.member_id<>t.member_id OR l.entry_type<>'commission' OR l.reference_type<>'commission_payment_target' OR l.reference_id IS DISTINCT FROM t.id
    OR l.operation_key<>'commission-payment:'||t.id::text OR l.actor_type<>'system' OR l.actor_id IS NOT NULL OR l.reversal_of IS NOT NULL
    OR l.delta_snapshot IS DISTINCT FROM jsonb_set(point_zero_snapshot(),'{commission,available}',to_jsonb(t.points::text))
    OR l.source_allocation IS DISTINCT FROM jsonb_build_array(jsonb_build_object('source','commission','state','available','points',t.points::text)))
   OR t.points=0 AND t.ledger_entry_id IS NOT NULL OR e.id IS NOT NULL AND e.member_id<>t.member_id))
 AND NOT EXISTS(SELECT 1 FROM commission_correction_balance_heads h
  JOIN commission_payments p ON p.brand_id=h.brand_id AND p.cycle_id=h.cycle_id AND p.id=pid
  LEFT JOIN commission_payment_targets t ON t.brand_id=b AND t.id=h.original_target_id
  LEFT JOIN commission_earnings e ON e.brand_id=b AND e.run_id=rid AND e.agent_id=h.agent_id
  WHERE h.brand_id=b AND (h.original_target_id IS NOT NULL AND (t.id IS NULL OR t.payment_id<>pid OR t.agent_id<>h.agent_id OR t.member_id<>h.member_id)
   OR e.id IS NOT NULL AND e.member_id<>h.member_id))
$$;
DO $$ DECLARE app_schema text:=current_schema(); f record; BEGIN
 FOR f IN SELECT p.oid::regprocedure AS signature FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace
 WHERE n.nspname=app_schema AND p.proname IN('commission_payment_has_actual_money','guard_commission_payment','commission_correction_source_valid') LOOP
  EXECUTE format('ALTER FUNCTION %s SET search_path TO pg_catalog, %I, pg_temp',f.signature,app_schema);
 END LOOP;
END $$;
