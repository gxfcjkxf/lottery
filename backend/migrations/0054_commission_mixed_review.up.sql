-- Preserve original payout_mode, creation audits, amounts and old blocked rows.
-- Mixed original snapshots now require whole-cycle brand-operator approval.
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
  SELECT EXISTS(SELECT 1 FROM commission_payment_targets WHERE payment_id=NEW.id AND ledger_entry_id IS NOT NULL) INTO credits;
  IF NEW.state='stale' AND credits THEN RAISE EXCEPTION 'credited payment needs compensation, cannot stale'; END IF;
  IF NEW.state='paid' AND (NEW.target_count<>(SELECT count(*) FROM commission_payment_targets WHERE payment_id=NEW.id AND state='paid') OR
   NEW.total_points IS DISTINCT FROM (SELECT coalesce(sum(points),0) FROM commission_payment_targets WHERE payment_id=NEW.id AND state='paid')) THEN RAISE EXCEPTION 'payment completion incomplete'; END IF;
 END IF;
 NEW.updated_at:=clock_timestamp(); RETURN NEW;
END $$;
DO $$
DECLARE app_schema text:=current_schema();
BEGIN
 EXECUTE format('ALTER FUNCTION guard_commission_payment() SET search_path TO pg_catalog, %I, pg_temp',app_schema);
END $$;
