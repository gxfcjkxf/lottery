
SET LOCAL check_function_bodies=false;
DROP VIEW IF EXISTS betting_financial_facts CASCADE;
DROP FUNCTION commission_correction_candidates(uuid,uuid,uuid);
DROP FUNCTION commission_correction_candidates_v2(uuid,uuid,uuid);
ALTER TABLE "brand_canada_sales" ALTER COLUMN "min_points" TYPE numeric;
ALTER TABLE "brand_canada_sales" ALTER COLUMN "max_points" TYPE numeric;
ALTER TABLE "brand_canada_sales" ALTER COLUMN "max_period_points" TYPE numeric;
ALTER TABLE "brand_canada_sales" ALTER COLUMN "max_user_period_points" TYPE numeric;
ALTER TABLE "brand_mark_six_sales" ALTER COLUMN "min_points" TYPE numeric;
ALTER TABLE "brand_mark_six_sales" ALTER COLUMN "max_points" TYPE numeric;
ALTER TABLE "brand_mark_six_sales" ALTER COLUMN "max_period_points" TYPE numeric;
ALTER TABLE "brand_mark_six_sales" ALTER COLUMN "max_user_period_points" TYPE numeric;
ALTER TABLE "brand_point_policies" ALTER COLUMN "max_balance_points" TYPE numeric;
ALTER TABLE "brand_point_policies" ALTER COLUMN "max_recharge_points" TYPE numeric;
ALTER TABLE "brand_point_policies" ALTER COLUMN "max_adjustment_points" TYPE numeric;
ALTER TABLE "brand_recharge_quota_entries" ALTER COLUMN "points" TYPE numeric;
ALTER TABLE "brand_recharge_quota_entries" ALTER COLUMN "before_available" TYPE numeric;
ALTER TABLE "brand_recharge_quota_entries" ALTER COLUMN "after_available" TYPE numeric;
ALTER TABLE "brand_recharge_quotas" ALTER COLUMN "available" TYPE numeric;
ALTER TABLE "brand_withdrawal_policies" ALTER COLUMN "points_step" TYPE numeric;
ALTER TABLE "commission_adjustment_heads" ALTER COLUMN "points" TYPE numeric;
ALTER TABLE "commission_adjustments" ALTER COLUMN "points_before" TYPE numeric;
ALTER TABLE "commission_adjustments" ALTER COLUMN "points_after" TYPE numeric;
ALTER TABLE "commission_adjustments" ALTER COLUMN "delta_points" TYPE numeric;
ALTER TABLE "commission_calculations" ALTER COLUMN "stake_points" TYPE numeric;
ALTER TABLE "commission_calculations" ALTER COLUMN "prize_points" TYPE numeric;
ALTER TABLE "commission_calculations" ALTER COLUMN "base_points" TYPE numeric;
ALTER TABLE "commission_correction_balance_heads" ALTER COLUMN "points" TYPE numeric;
ALTER TABLE "commission_correction_execution_targets" ALTER COLUMN "points_before" TYPE numeric;
ALTER TABLE "commission_correction_execution_targets" ALTER COLUMN "points_after" TYPE numeric;
ALTER TABLE "commission_correction_execution_targets" ALTER COLUMN "delta_points" TYPE numeric;
ALTER TABLE "commission_correction_plan_targets" ALTER COLUMN "points_before" TYPE numeric;
ALTER TABLE "commission_correction_plan_targets" ALTER COLUMN "points_after" TYPE numeric;
ALTER TABLE "commission_correction_plan_targets" ALTER COLUMN "delta_points" TYPE numeric;
ALTER TABLE "commission_earnings" ALTER COLUMN "points" TYPE numeric;
ALTER TABLE "commission_payment_targets" ALTER COLUMN "points" TYPE numeric;
ALTER TABLE "configured_orders" ALTER COLUMN "total_points" TYPE numeric;
ALTER TABLE "configured_sales" ALTER COLUMN "min_points" TYPE numeric;
ALTER TABLE "configured_sales" ALTER COLUMN "max_points" TYPE numeric;
ALTER TABLE "configured_sales" ALTER COLUMN "max_period_points" TYPE numeric;
ALTER TABLE "configured_sales" ALTER COLUMN "max_user_period_points" TYPE numeric;
ALTER TABLE "point_buckets" ALTER COLUMN "points" TYPE numeric;
ALTER TABLE "recharge_orders" ALTER COLUMN "points" TYPE numeric;
ALTER TABLE "reward_orders" ALTER COLUMN "points" TYPE numeric;
ALTER TABLE "withdrawal_orders" ALTER COLUMN "points" TYPE numeric;
ALTER TABLE "withdrawal_policy_revisions" ALTER COLUMN "points_step" TYPE numeric;
CREATE OR REPLACE FUNCTION commission_analysis_run_valid(b uuid, cid uuid, rid uuid) RETURNS boolean
    LANGUAGE plpgsql STABLE
    AS $$
BEGIN
 -- Revalidate immutable allocation arithmetic and complete earnings, rather
 -- than treating a ready flag or a sum of a partial table as known accrual.
 RETURN EXISTS(SELECT 1 FROM commission_cycles c WHERE c.brand_id=b AND c.id=cid AND
 c.target_count=(SELECT count(*) FROM commission_cycle_targets t WHERE t.brand_id=b AND t.cycle_id=cid)
 AND NOT EXISTS(SELECT 1 FROM betting_financial_facts o WHERE o.brand_id=b AND o.placed_at>=c.window_from AND o.placed_at<c.window_to
  AND NOT EXISTS(SELECT 1 FROM commission_cycle_targets t WHERE t.brand_id=b AND t.cycle_id=cid AND t.order_id=o.id))
 AND NOT EXISTS(SELECT 1 FROM commission_cycle_targets t LEFT JOIN betting_financial_facts o ON o.brand_id=t.brand_id AND o.id=t.order_id
  WHERE t.brand_id=b AND t.cycle_id=cid AND (o.id IS NULL OR t.placed_at IS DISTINCT FROM o.placed_at OR o.placed_at<c.window_from OR o.placed_at>=c.window_to)))
 AND NOT EXISTS(
  SELECT 1 FROM commission_calculations x
  LEFT JOIN betting_financial_facts o ON o.brand_id=x.brand_id AND o.id=x.order_id
  LEFT JOIN point_ledger_entries l ON l.id=o.debit_entry_id
  LEFT JOIN point_ledger_entries pl ON pl.id=o.payout_entry_id
  WHERE x.brand_id=b AND x.cycle_id=cid AND x.run_id=rid AND (
   o.id IS NULL OR NOT EXISTS(SELECT 1 FROM audit_logs a WHERE a.id=x.audit_log_id AND a.brand_id=b AND a.action='commission.cycle.calculation_page' AND a.resource_type='commission_cycle' AND a.resource_id=cid)
   OR l.id IS NULL OR l.brand_id IS DISTINCT FROM b OR l.account_id IS DISTINCT FROM x.account_id OR l.member_id IS DISTINCT FROM x.member_id
   OR l.entry_type<>'bet' OR l.reference_type<>'configured_order' OR l.reference_id IS DISTINCT FROM x.order_id
   OR l.source_allocation IS DISTINCT FROM o.deduction_allocation
   OR validated_point_snapshot(l.delta_snapshot) IS DISTINCT FROM configured_funding_delta(o.deduction_allocation,'available',CASE WHEN o.snapshot->'catalog'->>'funding'='reserve' THEN 'system_frozen' ELSE '' END)
   OR NOT lottery_business_ledger_applied(l,b,x.account_id)
   OR x.prize_points>0 AND (pl.id IS NULL OR pl.brand_id IS DISTINCT FROM b OR pl.account_id IS DISTINCT FROM x.account_id OR pl.member_id IS DISTINCT FROM x.member_id
    OR pl.entry_type<>'prize' OR pl.reference_type<>'configured_order' OR pl.reference_id IS DISTINCT FROM x.order_id
    OR pl.operation_key<>'configured-prize:'||x.order_id::text
    OR validated_point_snapshot(pl.delta_snapshot) IS DISTINCT FROM jsonb_set(point_zero_snapshot(),'{winning,available}',to_jsonb(x.prize_points::text))
    OR pl.source_allocation IS DISTINCT FROM jsonb_build_array(jsonb_build_object('source','winning','state','available','points',x.prize_points::text))
    OR NOT lottery_business_ledger_applied(pl,b,x.account_id))
   OR x.prize_points=0 AND o.payout_entry_id IS NOT NULL
   OR x.rule_snapshot->'schema_version' IS DISTINCT FROM '1'::jsonb
   OR x.rule_snapshot->>'brand_id' IS DISTINCT FROM b::text OR x.rule_snapshot->>'member_id' IS DISTINCT FROM x.member_id::text
   OR (x.rule_snapshot->>'captured_at')::timestamptz IS DISTINCT FROM o.placed_at
   OR x.rule_snapshot IS DISTINCT FROM o.commission_rule_snapshot
   OR x.rule_snapshot->'agency_policy'->'config' IS DISTINCT FROM o.attribution_snapshot->'agent_policy'->'config'
   OR x.rule_snapshot->'agency_policy'->>'version' IS DISTINCT FROM o.attribution_snapshot->'agent_policy'->>'version'
   OR jsonb_array_length(x.rule_snapshot->'agent_path') IS DISTINCT FROM jsonb_array_length(o.attribution_snapshot->'agent_configs_at_bet')
   OR EXISTS(SELECT 1 FROM jsonb_array_elements(x.rule_snapshot->'agent_path') WITH ORDINALITY p(node,ord) WHERE
    node->>'depth' IS DISTINCT FROM ord::text
    OR node->'id' IS DISTINCT FROM o.attribution_snapshot->'agent_configs_at_bet'->(ord::integer-1)->'id'
    OR node->'parent_id' IS DISTINCT FROM o.attribution_snapshot->'agent_configs_at_bet'->(ord::integer-1)->'parent_id'
    OR node->>'version' IS DISTINCT FROM o.attribution_snapshot->'agent_configs_at_bet'->(ord::integer-1)->>'version'
    OR node->'config' IS DISTINCT FROM o.attribution_snapshot->'agent_configs_at_bet'->(ord::integer-1)->'config'
    OR NOT EXISTS(SELECT 1 FROM agent_config_revisions r JOIN agent_nodes n ON n.brand_id=r.brand_id AND n.id=r.agent_id
     WHERE r.brand_id=b AND r.id=(node->>'revision_id')::uuid AND r.agent_id=(node->>'id')::uuid AND r.version::text=node->>'version'
     AND r.config=node->'config' AND r.created_at<=o.placed_at AND n.member_id::text=node->>'member_id'))
   OR (x.reason='eligible' AND (
    x.rule_snapshot->'financial_policy'->'config'->'enabled' IS DISTINCT FROM 'true'::jsonb
    OR jsonb_typeof(x.rule_snapshot->'agent_path') IS DISTINCT FROM 'array'
    OR jsonb_array_length(x.rule_snapshot->'agent_path')=0
    OR x.rule_snapshot->'financial_policy'->'config'->'calendar' IS DISTINCT FROM (SELECT calendar FROM commission_cycles WHERE brand_id=b AND id=cid)
    OR x.base_points IS DISTINCT FROM CASE WHEN x.rule_snapshot->'agent_path'->0->>'effective_mode'='turnover' THEN o.effective_points ELSE o.loss_points END
    OR (SELECT count(*) FROM commission_allocations a WHERE a.brand_id=b AND a.run_id=rid AND a.calculation_id=x.id)<>jsonb_array_length(x.rule_snapshot->'agent_path')))
   OR x.reason<>'eligible' AND (x.base_points<>0 OR EXISTS(SELECT 1 FROM commission_allocations a WHERE a.run_id=rid AND a.calculation_id=x.id))))
 AND NOT EXISTS(
  SELECT 1 FROM commission_allocations a
  LEFT JOIN commission_calculations x ON x.brand_id=a.brand_id AND x.cycle_id=a.cycle_id AND x.run_id=a.run_id AND x.id=a.calculation_id
  LEFT JOIN LATERAL (SELECT v AS node FROM jsonb_array_elements(x.rule_snapshot->'agent_path') v WHERE v->>'id'=a.agent_id::text) n ON true
  LEFT JOIN LATERAL (SELECT v AS node FROM jsonb_array_elements(x.rule_snapshot->'agent_path') v WHERE (v->>'depth')::integer=(n.node->>'depth')::integer+1) nx ON true
  WHERE a.brand_id=b AND a.cycle_id=cid AND a.run_id=rid AND (
   x.id IS NULL OR x.reason<>'eligible' OR a.order_id IS DISTINCT FROM x.order_id OR n.node IS NULL OR n.node->>'member_id' IS DISTINCT FROM a.member_id::text
   OR valid_commission_fraction(a.numerator,a.denominator) IS NOT TRUE
   OR (n.node->'config'->>'ratio')::numeric<coalesce((nx.node->'config'->>'ratio')::numeric,0)
   OR a.numerator::numeric IS DISTINCT FROM x.base_points::numeric*((n.node->'config'->>'ratio')::numeric-coalesce((nx.node->'config'->>'ratio')::numeric,0))*a.denominator::numeric
   OR NOT EXISTS(SELECT 1 FROM agent_nodes z WHERE z.brand_id=b AND z.id=a.agent_id AND z.member_id=a.member_id)))
 AND NOT EXISTS(
  WITH sums AS (
   SELECT agent_id,member_id,sum(numerator::numeric*(1000000/denominator::numeric)) AS micro
   FROM commission_allocations WHERE brand_id=b AND cycle_id=cid AND run_id=rid GROUP BY agent_id,member_id
  ), earnings AS (SELECT * FROM commission_earnings WHERE brand_id=b AND cycle_id=cid AND run_id=rid)
  SELECT 1 FROM sums s FULL JOIN earnings e ON e.agent_id=s.agent_id AND e.member_id=s.member_id
  WHERE e.id IS NULL OR s.agent_id IS NULL OR valid_commission_fraction(e.numerator,e.denominator) IS NOT TRUE
   OR e.numerator::numeric*1000000 IS DISTINCT FROM s.micro*e.denominator::numeric
   OR e.points::numeric IS DISTINCT FROM floor((s.micro*200+1000000)/2000000)/100);
EXCEPTION WHEN OTHERS THEN RETURN false;
END $$;
CREATE OR REPLACE FUNCTION commission_correction_candidates(b uuid, pid uuid, rid uuid) RETURNS TABLE(agent_id uuid, member_id uuid, original_target_id uuid, earning_id uuid, adjustment_version bigint, points_before numeric, points_after numeric, delta_points numeric)
    LANGUAGE sql STABLE
    AS $$ SELECT agent_id,member_id,original_target_id,earning_id,adjustment_version,points_before,points_after,delta_points FROM commission_correction_candidates_v2(b,pid,rid) $$;
CREATE OR REPLACE FUNCTION commission_correction_candidates_v2(b uuid, pid uuid, rid uuid) RETURNS TABLE(agent_id uuid, member_id uuid, original_target_id uuid, earning_id uuid, adjustment_version bigint, points_before numeric, points_after numeric, delta_points numeric, previous_correction_target_id uuid, financial_version bigint)
    LANGUAGE sql STABLE
    AS $$
 WITH original AS (
  SELECT t.agent_id,t.member_id,t.id,h.version,h.points FROM commission_payment_targets t
  JOIN commission_adjustment_heads h ON h.brand_id=t.brand_id AND h.target_id=t.id WHERE t.brand_id=b AND t.payment_id=pid AND t.state='paid'
 ), actual AS (
  SELECT h.* FROM commission_correction_balance_heads h JOIN commission_payments p ON p.brand_id=h.brand_id AND p.cycle_id=h.cycle_id WHERE p.brand_id=b AND p.id=pid
 ), basis AS (
  SELECT coalesce(h.agent_id,o.agent_id) agent_id,coalesce(h.member_id,o.member_id) member_id,coalesce(h.original_target_id,o.id) original_target_id,
   o.version adjustment_version,coalesce(h.points,o.points) points,h.last_execution_target_id,h.version financial_version
  FROM original o FULL JOIN actual h ON h.agent_id=o.agent_id
 ), corrected AS (SELECT * FROM commission_earnings WHERE brand_id=b AND run_id=rid)
 SELECT coalesce(o.agent_id,e.agent_id),coalesce(o.member_id,e.member_id),o.original_target_id,e.id,o.adjustment_version,
  coalesce(o.points,0),coalesce(e.points,0),coalesce(e.points,0)-coalesce(o.points,0),o.last_execution_target_id,o.financial_version
 FROM basis o FULL JOIN corrected e ON e.agent_id=o.agent_id
$$;
CREATE OR REPLACE FUNCTION guard_commission_correction_execution() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE p commission_correction_plans; s commission_correction_execution_steps; a audit_logs; held boolean; expected text; t commission_correction_plan_targets; available numeric;
 n bigint; credit numeric; debit numeric;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'correction executions retained'; END IF;
 IF TG_OP='UPDATE' AND NEW.version=OLD.version AND (to_jsonb(NEW)-'next_work_at')=(to_jsonb(OLD)-'next_work_at') THEN RETURN NEW; END IF;
 PERFORM 1 FROM commission_cycles WHERE brand_id=NEW.brand_id AND id=NEW.cycle_id FOR UPDATE NOWAIT;
 SELECT * INTO p FROM commission_correction_plans WHERE brand_id=NEW.brand_id AND id=NEW.plan_id FOR SHARE NOWAIT;
 SELECT coalesce((SELECT active FROM commission_correction_cycle_holds WHERE brand_id=NEW.brand_id AND cycle_id=NEW.cycle_id),false) INTO held;
 IF TG_OP='INSERT' THEN
  PERFORM 1 FROM brands WHERE id=NEW.brand_id AND status<>'disabled' FOR SHARE NOWAIT;
  IF NOT FOUND THEN RAISE EXCEPTION 'correction execution brand disabled'; END IF;
  PERFORM 1 FROM brand_commission_payment_policies WHERE brand_id=NEW.brand_id AND enabled FOR SHARE NOWAIT;
  IF NOT FOUND OR p.id IS NULL OR p.state<>'ready' OR p.version<>NEW.plan_version OR p.cycle_id<>NEW.cycle_id OR p.run_id<>NEW.run_id OR p.evidence_epoch<>NEW.evidence_epoch OR
   NOT commission_payment_evidence_current(NEW.brand_id,NEW.cycle_id,NEW.run_id,NEW.evidence_epoch) OR NOT commission_correction_source_valid(NEW.brand_id,p.payment_id,NEW.run_id) OR NOT commission_correction_heads_valid(NEW.brand_id,NEW.cycle_id) THEN RAISE EXCEPTION 'execution requires complete current plan'; END IF;
  expected:=(CASE WHEN p.payout_mode IN('manual','mixed') THEN 'awaiting_approval' WHEN held THEN 'paused' ELSE 'applying' END);
  SELECT * INTO a FROM audit_logs WHERE id=NEW.creation_audit_log_id;
  IF NEW.version<>1 OR NEW.creation_xid<>pg_current_xact_id() OR NEW.state<>expected OR NEW.payout_mode<>p.payout_mode OR
   NEW.credit_points<>p.credit_points OR NEW.debit_points<>p.debit_points OR NEW.net_points<>p.net_points OR NEW.target_count<>p.target_count OR
   NEW.last_audit_log_id<>NEW.creation_audit_log_id OR NEW.paused_plan_target_id IS NOT NULL OR a.id IS NULL OR a.brand_id IS DISTINCT FROM NEW.brand_id OR
   a.actor_type<>'system' OR a.actor_id IS NOT NULL OR a.action<>'commission.correction_execution.create' OR a.resource_type<>'commission_correction_execution' OR a.resource_id IS DISTINCT FROM NEW.id
   THEN RAISE EXCEPTION 'execution identity/mode/amount/creation mismatch'; END IF;
  PERFORM 1 FROM brand_commission_correction_policies WHERE brand_id=NEW.brand_id AND enabled FOR SHARE NOWAIT;
  IF NOT FOUND THEN RAISE EXCEPTION 'correction financial rollout disabled'; END IF;
  IF expected='awaiting_approval' THEN
   IF NEW.approval_actor_type IS NOT NULL OR NEW.approval_audit_log_id IS NOT NULL OR NEW.approved_by IS NOT NULL THEN RAISE EXCEPTION 'manual correction requires new approval'; END IF;
  ELSIF NEW.approval_actor_type IS DISTINCT FROM 'system' OR NEW.approval_audit_log_id IS DISTINCT FROM NEW.creation_audit_log_id OR NEW.approved_by IS NOT NULL OR
   (expected='paused' AND NEW.last_error_code IS DISTINCT FROM 'COMMISSION_CORRECTION_CYCLE_HELD') THEN RAISE EXCEPTION 'automatic correction authorization mismatch'; END IF;
 ELSE
  IF (to_jsonb(NEW)-ARRAY['state','version','last_audit_log_id','last_error_code','paused_plan_target_id','updated_at','next_work_at','approval_actor_type','approved_by','approval_audit_log_id']) IS DISTINCT FROM
   (to_jsonb(OLD)-ARRAY['state','version','last_audit_log_id','last_error_code','paused_plan_target_id','updated_at','next_work_at','approval_actor_type','approved_by','approval_audit_log_id']) OR NEW.version<>OLD.version+1 THEN RAISE EXCEPTION 'correction execution immutable identity/amount'; END IF;
  SELECT * INTO s FROM commission_correction_execution_steps WHERE brand_id=NEW.brand_id AND execution_id=NEW.id AND version=NEW.version;
  IF s.id IS NULL OR s.from_state IS DISTINCT FROM OLD.state OR s.to_state<>NEW.state OR s.audit_log_id<>NEW.last_audit_log_id OR s.error_code IS DISTINCT FROM NEW.last_error_code OR
   s.paused_plan_target_id IS DISTINCT FROM NEW.paused_plan_target_id THEN RAISE EXCEPTION 'execution transition requires matching immutable step'; END IF;
  IF NOT ((s.operation='approve' AND OLD.state='awaiting_approval' AND NEW.state IN('applying','paused')) OR
   (s.operation='apply' AND OLD.state='applying' AND NEW.state='applying') OR (s.operation='complete' AND OLD.state='applying' AND NEW.state='completed') OR
   (s.operation='pause' AND OLD.state='applying' AND NEW.state='paused') OR (s.operation='continue' AND OLD.state='paused' AND NEW.state='applying') OR
   (s.operation='retry' AND OLD.state='failed' AND NEW.state='applying') OR (s.operation='fail' AND OLD.state='applying' AND NEW.state='failed') OR
   (s.operation='invalidate' AND OLD.state<>'stale' AND NEW.state='stale')) THEN RAISE EXCEPTION 'correction execution state transition invalid'; END IF;
  IF s.operation='approve' THEN
   IF OLD.approval_audit_log_id IS NOT NULL OR NEW.approval_actor_type IS DISTINCT FROM 'admin' OR NEW.approved_by IS DISTINCT FROM s.actor_id OR NEW.approval_audit_log_id IS DISTINCT FROM s.audit_log_id THEN RAISE EXCEPTION 'new exact-plan approval required'; END IF;
  ELSIF (NEW.approval_actor_type,NEW.approved_by,NEW.approval_audit_log_id) IS DISTINCT FROM (OLD.approval_actor_type,OLD.approved_by,OLD.approval_audit_log_id) THEN RAISE EXCEPTION 'old approval immutable'; END IF;
  IF s.operation NOT IN('invalidate','fail') THEN
   PERFORM 1 FROM brands WHERE id=NEW.brand_id AND status<>'disabled' FOR SHARE NOWAIT;
   IF NOT FOUND THEN RAISE EXCEPTION 'execution brand disabled'; END IF;
   PERFORM 1 FROM brand_commission_payment_policies WHERE brand_id=NEW.brand_id AND enabled FOR SHARE NOWAIT;
   IF NOT FOUND OR NOT commission_correction_execution_current(NEW.brand_id,NEW.id) OR NOT commission_correction_source_valid(NEW.brand_id,p.payment_id,NEW.run_id) OR NOT commission_correction_heads_valid(NEW.brand_id,NEW.cycle_id) THEN RAISE EXCEPTION 'execution evidence or financial sources unavailable'; END IF;
   PERFORM 1 FROM brand_commission_correction_policies WHERE brand_id=NEW.brand_id AND enabled FOR SHARE NOWAIT;
   IF NOT FOUND THEN RAISE EXCEPTION 'correction financial rollout disabled'; END IF;
  END IF;
  IF s.operation='invalidate' AND commission_correction_execution_current(NEW.brand_id,NEW.id) THEN RAISE EXCEPTION 'cannot stale current correction execution'; END IF;
  IF NEW.state='applying' AND held THEN RAISE EXCEPTION 'cycle hold forbids new compensation'; END IF;
  IF s.operation='pause' THEN
   SELECT * INTO t FROM commission_correction_plan_targets WHERE brand_id=NEW.brand_id AND plan_id=NEW.plan_id AND id=NEW.paused_plan_target_id;
   SELECT pb.points INTO available FROM point_buckets pb JOIN point_accounts pa ON pa.brand_id=pb.brand_id AND pa.id=pb.account_id WHERE pb.brand_id=NEW.brand_id AND pa.brand_member_id=t.member_id AND pb.source='commission' AND pb.state='available';
   IF t.id IS NULL OR t.delta_points>=0 OR available IS NULL OR available>=-t.delta_points OR NOT held OR NEW.last_error_code IS DISTINCT FROM 'COMMISSION_CORRECTION_AVAILABLE_INSUFFICIENT' OR
    EXISTS(SELECT 1 FROM commission_correction_execution_targets done WHERE done.execution_id=NEW.id AND done.plan_target_id=t.id)
    THEN RAISE EXCEPTION 'pause must witness real C-available shortage'; END IF;
  ELSIF NEW.state='paused' AND (NOT held OR NEW.last_error_code IS DISTINCT FROM 'COMMISSION_CORRECTION_CYCLE_HELD') THEN RAISE EXCEPTION 'inherited cycle pause mismatch'; END IF;
  IF s.operation='apply' AND NOT EXISTS(SELECT 1 FROM commission_correction_execution_targets WHERE brand_id=NEW.brand_id AND execution_id=NEW.id AND id=s.target_id AND state='applied' AND execution_version=OLD.version) THEN RAISE EXCEPTION 'apply step requires completed actual target'; END IF;
  SELECT count(*),coalesce(sum(greatest(delta_points,0)::numeric),0),coalesce(sum(greatest(-delta_points,0)::numeric),0) INTO n,credit,debit FROM commission_correction_execution_targets WHERE execution_id=NEW.id AND state='applied';
  IF n>NEW.target_count OR credit>NEW.credit_points OR debit>NEW.debit_points OR NEW.state='completed' AND (n<>NEW.target_count OR credit<>NEW.credit_points OR debit<>NEW.debit_points) THEN RAISE EXCEPTION 'correction completion/amount incomplete'; END IF;
  NEW.updated_at:=clock_timestamp();
 END IF;
 RETURN NEW;
END $$;
CREATE OR REPLACE FUNCTION guard_commission_correction_execution_target() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE x commission_correction_executions; p commission_correction_plans; t commission_correction_plan_targets; h commission_correction_balance_heads; s commission_correction_execution_steps; a audit_logs; base numeric;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'correction financial targets retained'; END IF;
 SELECT * INTO x FROM commission_correction_executions WHERE brand_id=NEW.brand_id AND id=NEW.execution_id FOR UPDATE NOWAIT;
 SELECT * INTO p FROM commission_correction_plans WHERE brand_id=NEW.brand_id AND id=x.plan_id;
 SELECT * INTO t FROM commission_correction_plan_targets WHERE brand_id=NEW.brand_id AND plan_id=x.plan_id AND id=NEW.plan_target_id;
 SELECT * INTO h FROM commission_correction_balance_heads WHERE brand_id=NEW.brand_id AND cycle_id=x.cycle_id AND agent_id=NEW.agent_id FOR UPDATE NOWAIT;
 SELECT * INTO s FROM commission_correction_execution_steps WHERE execution_id=x.id AND version=x.version+1;
 IF x.state IS DISTINCT FROM 'applying' OR x.approval_audit_log_id IS NULL OR t.id IS NULL OR p.state<>'ready' OR NEW.cycle_id<>x.cycle_id OR NEW.member_id<>t.member_id OR NEW.agent_id<>t.agent_id OR
  NEW.points_before<>t.points_before OR NEW.points_after<>t.points_after OR NEW.delta_points<>t.delta_points OR NEW.base_financial_version IS DISTINCT FROM t.financial_version OR
  NEW.execution_version<>x.version OR h.version IS DISTINCT FROM t.financial_version OR h.last_execution_target_id IS DISTINCT FROM t.previous_correction_target_id OR
  h.agent_id IS NOT NULL AND (h.member_id<>NEW.member_id OR h.points<>NEW.points_before) OR s.operation IS DISTINCT FROM 'apply' OR s.target_id IS DISTINCT FROM NEW.id OR
  NOT commission_correction_execution_current(NEW.brand_id,NEW.execution_id) OR NOT commission_correction_source_valid(NEW.brand_id,p.payment_id,x.run_id) OR NOT commission_correction_heads_valid(NEW.brand_id,x.cycle_id) OR
  NOT EXISTS(SELECT 1 FROM commission_correction_cycle_holds WHERE brand_id=NEW.brand_id AND cycle_id=x.cycle_id AND NOT active) THEN RAISE EXCEPTION 'correction target requires exact authorized unprocessed net'; END IF;
 PERFORM 1 FROM brands WHERE id=NEW.brand_id AND status<>'disabled' FOR SHARE NOWAIT;
 IF NOT FOUND THEN RAISE EXCEPTION 'correction target brand disabled'; END IF;
 PERFORM 1 FROM brand_commission_correction_policies WHERE brand_id=NEW.brand_id AND enabled FOR SHARE NOWAIT;
 IF NOT FOUND THEN RAISE EXCEPTION 'correction target rollout disabled'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.state<>'pending' OR NEW.ledger_entry_id IS NOT NULL OR NEW.audit_log_id IS NOT NULL OR NEW.financial_version IS NOT NULL OR NEW.creation_xid<>pg_current_xact_id() THEN RAISE EXCEPTION 'correction target starts pending in its financial transaction'; END IF;
 ELSE
  IF OLD.state<>'pending' OR NEW.state<>'applied' OR OLD.creation_xid<>pg_current_xact_id() OR
   (to_jsonb(NEW)-ARRAY['state','ledger_entry_id','audit_log_id','financial_version','applied_at']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['state','ledger_entry_id','audit_log_id','financial_version','applied_at']) OR
   NEW.financial_version IS DISTINCT FROM coalesce(NEW.base_financial_version,0)+1 THEN RAISE EXCEPTION 'correction target completion must bind immutable exact receipt once'; END IF;
  SELECT * INTO a FROM audit_logs WHERE id=NEW.audit_log_id;
  IF a.id IS NULL OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.actor_type<>'system' OR a.actor_id IS NOT NULL OR a.action<>'commission.correction_execution.target' OR a.resource_type<>'commission_correction_target' OR
   a.resource_id IS DISTINCT FROM NEW.id OR a.request_id<>s.request_id OR a.before_json IS DISTINCT FROM jsonb_build_object('state','pending') OR
   a.after_json IS DISTINCT FROM jsonb_build_object('state','applied','execution_id',NEW.execution_id,'plan_target_id',NEW.plan_target_id,'points_before',NEW.points_before::text,'points_after',NEW.points_after::text,
    'delta_points',NEW.delta_points::text,'ledger_entry_id',NEW.ledger_entry_id,'financial_version',NEW.financial_version) OR
   NEW.delta_points<>0 AND NOT EXISTS(SELECT 1 FROM point_ledger_entries l WHERE l.id=NEW.ledger_entry_id AND l.brand_id=NEW.brand_id AND l.member_id=NEW.member_id AND l.entry_type='commission_correction' AND
    l.reference_type='commission_correction_target' AND l.reference_id=NEW.id AND l.operation_key='commission-correction:'||NEW.id::text AND l.request_id=s.request_id)
   THEN RAISE EXCEPTION 'correction target audit/ledger mismatch'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE OR REPLACE FUNCTION guard_commission_earning() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE r commission_runs; c commission_cycles; micro numeric;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'commission earnings immutable'; END IF;
 SELECT * INTO r FROM commission_runs WHERE brand_id=NEW.brand_id AND cycle_id=NEW.cycle_id AND id=NEW.run_id;
 SELECT * INTO c FROM commission_cycles WHERE brand_id=NEW.brand_id AND id=NEW.cycle_id;
 SELECT sum(numerator::numeric*(1000000/denominator::numeric)) INTO micro FROM commission_allocations WHERE run_id=NEW.run_id AND agent_id=NEW.agent_id AND member_id=NEW.member_id;
 IF r.state<>'summarizing' OR c.state<>'summarizing' OR c.current_run_id IS DISTINCT FROM NEW.run_id OR r.evidence_epoch<>c.evidence_epoch OR micro IS NULL OR NEW.numerator::numeric*1000000 IS DISTINCT FROM micro*NEW.denominator::numeric OR
  NEW.points::numeric IS DISTINCT FROM floor((micro*200+1000000)/2000000)/100 THEN RAISE EXCEPTION 'commission cycle earning total mismatch'; END IF;
 RETURN NEW;
END $$;
CREATE OR REPLACE FUNCTION guard_point_ledger_snapshot() RETURNS trigger
    LANGUAGE plpgsql
    AS $_$
DECLARE a point_accounts; prev jsonb; actual jsonb; original point_ledger_entries;
 src text; st text; item jsonb; seen text[]:='{}'; bucket_value numeric; amount numeric;
 negatives integer; positives integer; negative_state text; positive_state text;
 negative_amount numeric; positive_amount numeric; expected jsonb; allocated boolean;
BEGIN
 IF valid_point_snapshot(NEW.before_snapshot,false) IS NOT TRUE OR
  valid_point_snapshot(NEW.delta_snapshot,true) IS NOT TRUE OR
  valid_point_snapshot(NEW.after_snapshot,false) IS NOT TRUE THEN
  RAISE EXCEPTION 'new ledger requires complete sixteen-bucket snapshots' USING ERRCODE='23514',CONSTRAINT='ledger_four_source_snapshot';
 END IF;
 SELECT * INTO a FROM point_accounts WHERE brand_id=NEW.brand_id AND id=NEW.account_id AND brand_member_id=NEW.member_id FOR UPDATE;
 IF NOT FOUND OR a.version<>NEW.version-1 THEN RAISE EXCEPTION 'ledger must advance locked account by one version'; END IF;
 SELECT jsonb_object_agg(source,states) INTO actual FROM (
  SELECT source,jsonb_object_agg(state,trim_scale(points)::text) states FROM point_buckets
  WHERE brand_id=NEW.brand_id AND account_id=NEW.account_id GROUP BY source
 ) buckets;
 IF actual IS DISTINCT FROM NEW.before_snapshot THEN RAISE EXCEPTION 'ledger before must match complete current wallet'; END IF;
 IF a.version=0 THEN prev:=point_zero_snapshot();
 ELSE SELECT validated_point_snapshot(after_snapshot) INTO prev FROM point_ledger_entries
  WHERE brand_id=NEW.brand_id AND account_id=NEW.account_id AND version=a.version; END IF;
 IF prev IS DISTINCT FROM NEW.before_snapshot THEN RAISE EXCEPTION 'ledger before must match immutable predecessor'; END IF;
 IF NEW.before_snapshot=NEW.after_snapshot THEN RAISE EXCEPTION 'ledger cannot post empty change'; END IF;
 FOREACH src IN ARRAY ARRAY['recharge','winning','gift','commission'] LOOP
  FOREACH st IN ARRAY ARRAY['available','manual_frozen','system_frozen','withdrawal'] LOOP
   IF (NEW.before_snapshot->src->>st)::numeric+(NEW.delta_snapshot->src->>st)::numeric
     IS DISTINCT FROM (NEW.after_snapshot->src->>st)::numeric THEN RAISE EXCEPTION 'ledger snapshot arithmetic mismatch'; END IF;
  END LOOP;
 END LOOP;
 IF jsonb_typeof(NEW.source_allocation) IS DISTINCT FROM 'array' OR jsonb_array_length(NEW.source_allocation) NOT BETWEEN 1 AND 4 THEN RAISE EXCEPTION 'ledger requires source allocation'; END IF;
 FOR item IN SELECT value FROM jsonb_array_elements(NEW.source_allocation) LOOP
  src:=item->>'source'; st:=item->>'state';
  IF jsonb_typeof(item) IS DISTINCT FROM 'object' OR (SELECT count(*) FROM jsonb_object_keys(item))<>3 OR
   NOT(item ?& ARRAY['source','state','points']) OR src IS NULL OR src NOT IN('recharge','winning','gift','commission') OR src=ANY(seen) OR
   st IS NULL OR st NOT IN('available','manual_frozen','system_frozen','withdrawal') OR
   jsonb_typeof(item->'points') IS DISTINCT FROM 'string' OR item->>'points' !~ '^(0|[1-9][0-9]*)([.][0-9]{1,2})?$' OR
   ((item->>'points')::numeric<=0 OR (item->>'points')::numeric>92233720368547758.07) THEN RAISE EXCEPTION 'invalid ledger source allocation'; END IF;
  seen:=array_append(seen,src);
 END LOOP;
 IF NEW.reversal_of IS NOT NULL THEN
  SELECT * INTO original FROM point_ledger_entries WHERE brand_id=NEW.brand_id AND account_id=NEW.account_id AND id=NEW.reversal_of AND member_id=NEW.member_id;
  expected:=validated_point_snapshot(original.delta_snapshot);
  IF original.id IS NULL OR original.reversal_of IS NOT NULL OR expected IS NULL OR NEW.source_allocation IS DISTINCT FROM original.source_allocation THEN RAISE EXCEPTION 'invalid original-source reversal'; END IF;
  FOREACH src IN ARRAY ARRAY['recharge','winning','gift','commission'] LOOP
   FOREACH st IN ARRAY ARRAY['available','manual_frozen','system_frozen','withdrawal'] LOOP
    expected:=jsonb_set(expected,ARRAY[src,st],to_jsonb(trim_scale(-(expected->src->>st)::numeric)::text));
   END LOOP;
  END LOOP;
  IF NEW.delta_snapshot IS DISTINCT FROM expected THEN RAISE EXCEPTION 'reversal must negate exact original source changes'; END IF;
 ELSE
  FOREACH src IN ARRAY ARRAY['recharge','winning','gift','commission'] LOOP
   negatives:=0; positives:=0; negative_state:=NULL; positive_state:=NULL;
   negative_amount:=0; positive_amount:=0;
   FOREACH st IN ARRAY ARRAY['available','manual_frozen','system_frozen','withdrawal'] LOOP
    bucket_value:=(NEW.delta_snapshot->src->>st)::numeric;
    IF bucket_value<0 THEN negatives:=negatives+1; negative_state:=st; negative_amount:=-bucket_value; END IF;
    IF bucket_value>0 THEN positives:=positives+1; positive_state:=st; positive_amount:=bucket_value; END IF;
   END LOOP;
   allocated:=src=ANY(seen);
   IF (negatives>0 OR positives>0) IS DISTINCT FROM allocated OR negatives>1 OR positives>1 OR
    negatives=1 AND positives=1 AND negative_amount<>positive_amount THEN RAISE EXCEPTION 'source allocation must explain exact bucket changes'; END IF;
   IF allocated THEN
    SELECT (x->>'points')::numeric,x->>'state' INTO amount,st FROM jsonb_array_elements(NEW.source_allocation) x WHERE x->>'source'=src;
    IF amount IS DISTINCT FROM (CASE WHEN negatives=1 THEN negative_amount ELSE positive_amount END) OR
     st IS DISTINCT FROM (CASE WHEN negatives=1 THEN negative_state ELSE positive_state END) THEN RAISE EXCEPTION 'source allocation amount/state mismatch'; END IF;
   END IF;
  END LOOP;
 END IF;
 RETURN NEW;
END $_$;
CREATE OR REPLACE FUNCTION guard_unexecuted_commission_correction_credit() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE t commission_correction_execution_targets; x commission_correction_executions; p commission_correction_plans; s commission_correction_execution_steps; cap numeric; total numeric;
BEGIN
 IF NEW.entry_type NOT IN('commission_correction','commission_correction_plan') AND NEW.reference_type NOT IN('commission_correction_plan','commission_correction_plan_target','commission_correction_target') THEN RETURN NEW; END IF;
 SELECT * INTO t FROM commission_correction_execution_targets WHERE brand_id=NEW.brand_id AND id=NEW.reference_id FOR UPDATE NOWAIT;
 SELECT * INTO x FROM commission_correction_executions WHERE brand_id=NEW.brand_id AND id=t.execution_id FOR UPDATE NOWAIT;
 SELECT * INTO p FROM commission_correction_plans WHERE brand_id=NEW.brand_id AND id=x.plan_id;
 SELECT * INTO s FROM commission_correction_execution_steps WHERE execution_id=x.id AND version=x.version+1;
 IF t.id IS NULL OR t.state<>'pending' OR t.creation_xid<>pg_current_xact_id() OR t.delta_points=0 OR x.state<>'applying' OR x.approval_audit_log_id IS NULL OR s.operation IS DISTINCT FROM 'apply' OR s.target_id IS DISTINCT FROM t.id OR
  NOT commission_correction_execution_current(NEW.brand_id,x.id) OR NOT commission_correction_source_valid(NEW.brand_id,p.payment_id,x.run_id) OR NOT commission_correction_heads_valid(NEW.brand_id,x.cycle_id) OR
  NOT EXISTS(SELECT 1 FROM commission_correction_cycle_holds WHERE brand_id=NEW.brand_id AND cycle_id=x.cycle_id AND NOT active) OR
  NEW.entry_type<>'commission_correction' OR NEW.reference_type<>'commission_correction_target' OR NEW.member_id IS DISTINCT FROM t.member_id OR NEW.actor_type<>'system' OR NEW.actor_id IS NOT NULL OR
  NEW.request_id<>s.request_id OR NEW.operation_key<>'commission-correction:'||t.id::text OR NEW.reversal_of IS NOT NULL OR
  NEW.delta_snapshot IS DISTINCT FROM jsonb_set(point_zero_snapshot(),'{commission,available}',to_jsonb(t.delta_points::text)) OR
  NEW.source_allocation IS DISTINCT FROM jsonb_build_array(jsonb_build_object('source','commission','state','available','points',abs(t.delta_points)::text)) THEN RAISE EXCEPTION 'prepared correction plans cannot authorize financial ledger postings without a current approved execution target'; END IF;
 PERFORM 1 FROM brands WHERE id=NEW.brand_id AND status<>'disabled' FOR SHARE NOWAIT;
 IF NOT FOUND THEN RAISE EXCEPTION 'correction financial brand disabled'; END IF;
 PERFORM 1 FROM brand_commission_payment_policies WHERE brand_id=NEW.brand_id AND enabled FOR SHARE NOWAIT;
 IF NOT FOUND THEN RAISE EXCEPTION 'original financial gate disabled'; END IF;
 PERFORM 1 FROM brand_commission_correction_policies WHERE brand_id=NEW.brand_id AND enabled FOR SHARE NOWAIT;
 IF NOT FOUND THEN RAISE EXCEPTION 'correction financial gate disabled'; END IF;
 SELECT max_balance_points INTO cap FROM brand_point_policies WHERE brand_id=NEW.brand_id FOR SHARE NOWAIT;
 SELECT coalesce(sum(bucket.value::numeric),0) INTO total FROM jsonb_each(NEW.after_snapshot) src CROSS JOIN LATERAL jsonb_each_text(src.value) bucket;
 IF t.delta_points>0 AND cap IS NOT NULL AND total>cap THEN RAISE EXCEPTION 'correction credit exceeds current total balance policy'; END IF;
 RETURN NEW;
END $$;
CREATE OR REPLACE FUNCTION initialize_withdrawal_policy_revision() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 INSERT INTO withdrawal_policy_revisions(brand_id,game_id,version,config,reason,points_step) VALUES(NEW.brand_id,NULLIF(to_jsonb(NEW)->>'game_id','')::uuid,1,NEW.config,'Initial withdrawal policy',CASE WHEN TG_TABLE_NAME='brand_withdrawal_policies' THEN (to_jsonb(NEW)->>'points_step')::numeric ELSE 1 END);
 RETURN NULL;
END $$;
CREATE OR REPLACE FUNCTION lottery_business_allocation_delta(v jsonb, negative boolean) RETURNS jsonb
    LANGUAGE plpgsql IMMUTABLE
    AS $_$
DECLARE
 expected jsonb:=point_zero_snapshot(); item jsonb; src text; st text;
 amount numeric; seen text[]:='{}';
BEGIN
 IF jsonb_typeof(v) IS DISTINCT FROM 'array' OR jsonb_array_length(v) NOT BETWEEN 1 AND 4 THEN RETURN NULL; END IF;
 FOR item IN SELECT value FROM jsonb_array_elements(v) LOOP
  IF jsonb_typeof(item) IS DISTINCT FROM 'object' OR (SELECT count(*) FROM jsonb_object_keys(item))<>3 OR
   NOT(item ?& ARRAY['source','state','points']) THEN RETURN NULL; END IF;
  src:=item->>'source'; st:=item->>'state';
  IF src IS NULL OR src NOT IN('recharge','winning','gift','commission') OR src=ANY(seen) OR
   st IS NULL OR st NOT IN('available','manual_frozen','system_frozen','withdrawal') OR
   jsonb_typeof(item->'points') IS DISTINCT FROM 'string' OR item->>'points' !~ '^(0|[1-9][0-9]*)([.][0-9]{1,2})?$' THEN RETURN NULL; END IF;
  amount:=(item->>'points')::numeric;
  IF amount<=0 OR amount>92233720368547758.07 THEN RETURN NULL; END IF;
  seen:=array_append(seen,src);
  expected:=jsonb_set(expected,ARRAY[src,st],to_jsonb((CASE WHEN negative THEN -amount ELSE amount END)::text),false);
 END LOOP;
 RETURN expected;
EXCEPTION WHEN OTHERS THEN RETURN NULL;
END $_$;
CREATE OR REPLACE FUNCTION point_finance_business_witnesses(b uuid, a uuid) RETURNS TABLE(family text, source_id uuid, ledger_id uuid, source_valid boolean, expected_entry_type text)
    LANGUAGE plpgsql STABLE
    AS $_$
DECLARE
 edge record;
 l point_ledger_entries;
 zero jsonb := point_zero_snapshot();
 expected jsonb;
 before_value jsonb;
 delta_value jsonb;
 after_value jsonb;
 src text;
 bucket text;
 valid boolean;
 allocation_valid boolean;
 n numeric;
BEGIN
 IF b IS NULL OR a IS NULL THEN RETURN; END IF;

 FOR edge IN
  SELECT 'withdrawal'::text family,o.id source_id,o.reserve_entry_id ledger_id,o.member_id,o.account_id,
   'withdrawal_reserve'::text expected_entry_type,'withdrawal'::text reference_type,o.id reference_id,
   'withdrawal-reserve:'||o.id::text operation_key,
   (SELECT t.audit_log_id FROM withdrawal_order_transitions t WHERE t.order_id=o.id AND t.version=1) audit_id,
   o.source_allocation allocation,'reserve'::text kind,o.points::numeric amount,NULL::uuid reversal_of
  FROM withdrawal_orders o WHERE o.brand_id=b AND o.account_id=a
  UNION ALL
  SELECT 'withdrawal',o.id,o.paid_entry_id,o.member_id,o.account_id,'withdrawal_paid','withdrawal',o.id,
   'withdrawal-paid:'||o.id::text,t.audit_log_id,
   (SELECT jsonb_agg(jsonb_set(v,'{state}','"withdrawal"'::jsonb) ORDER BY ord) FROM jsonb_array_elements(o.source_allocation) WITH ORDINALITY x(v,ord)),
   'paid',o.points::numeric,NULL::uuid
  FROM withdrawal_orders o LEFT JOIN withdrawal_order_transitions t ON t.order_id=o.id AND t.to_state='paid'
   AND t.version=o.version
  WHERE o.brand_id=b AND o.account_id=a AND o.state='paid'
  UNION ALL
  SELECT 'withdrawal',o.id,o.release_entry_id,o.member_id,o.account_id,'withdrawal_release','withdrawal',o.id,
   'withdrawal-release:'||o.id::text,t.audit_log_id,o.source_allocation,'release',o.points::numeric,o.reserve_entry_id
  FROM withdrawal_orders o LEFT JOIN withdrawal_order_transitions t ON t.order_id=o.id AND t.to_state IN('rejected','failed','cancelled')
   AND t.version=o.version
  WHERE o.brand_id=b AND o.account_id=a AND o.state IN('rejected','failed','cancelled')
  UNION ALL
  SELECT 'commission',t.id,t.ledger_entry_id,t.member_id,p.id,'commission','commission_payment_target',t.id,
   'commission-payment:'||t.id::text,t.audit_log_id,
   jsonb_build_array(jsonb_build_object('source','commission','state','available','points',t.points::text)),
   'credit',t.points::numeric,NULL::uuid
  FROM commission_payment_targets t JOIN point_accounts p ON p.brand_id=t.brand_id AND p.brand_member_id=t.member_id
  WHERE t.brand_id=b AND p.id=a AND t.state='paid' AND t.points<>0
  UNION ALL
  SELECT 'commission_adjustment',x.id,x.ledger_entry_id,t.member_id,p.id,'commission_adjustment','commission_adjustment',x.id,
   'commission-adjustment:'||x.id::text,x.audit_log_id,
   jsonb_build_array(jsonb_build_object('source','commission','state','available','points',abs(x.delta_points)::text)),
   'adjustment',x.delta_points::numeric,NULL::uuid
  FROM commission_adjustments x JOIN commission_payment_targets t ON t.brand_id=x.brand_id AND t.id=x.target_id
   JOIN point_accounts p ON p.brand_id=t.brand_id AND p.brand_member_id=t.member_id
  WHERE x.brand_id=b AND p.id=a AND x.delta_points<>0
  UNION ALL
  SELECT 'commission_correction',t.id,t.ledger_entry_id,t.member_id,p.id,'commission_correction','commission_correction_target',t.id,
   'commission-correction:'||t.id::text,t.audit_log_id,
   jsonb_build_array(jsonb_build_object('source','commission','state','available','points',abs(t.delta_points)::text)),
   'correction',t.delta_points::numeric,NULL::uuid
  FROM commission_correction_execution_targets t
   LEFT JOIN commission_correction_executions x ON x.brand_id=t.brand_id AND x.id=t.execution_id
   LEFT JOIN commission_correction_execution_steps s ON s.brand_id=t.brand_id AND s.execution_id=t.execution_id
    AND s.version=t.execution_version+1 AND s.operation='apply' AND s.target_id=t.id
   JOIN point_accounts p ON p.brand_id=t.brand_id AND p.brand_member_id=t.member_id
  WHERE t.brand_id=b AND p.id=a AND t.state='applied' AND t.delta_points<>0
  UNION ALL
  SELECT 'reward',o.id,o.grant_ledger_entry_id,o.member_id,p.id,'reward_grant','reward_order',o.id,
   'reward-grant:'||o.id::text,coalesce(g.audit_log_id,o.creation_audit_log_id),
   jsonb_build_array(jsonb_build_object('source','gift','state','available','points',o.points::text)),
   'reward_grant',o.points::numeric,NULL::uuid
  FROM reward_orders o JOIN point_accounts p ON p.brand_id=o.brand_id AND p.brand_member_id=o.member_id
   LEFT JOIN reward_order_actions g ON g.brand_id=o.brand_id AND g.order_id=o.id AND g.version=1 AND g.operation='grant'
  WHERE o.brand_id=b AND p.id=a AND o.points<>0
  UNION ALL
  SELECT 'reward',coalesce(r.id,o.id),o.revoke_ledger_entry_id,o.member_id,p.id,'reward_reversal','reward_order',o.id,
   'reward-revoke:'||o.id::text,coalesce(r.audit_log_id,o.last_audit_log_id),
   jsonb_build_array(jsonb_build_object('source','gift','state','available','points',o.points::text)),
   'reward_reversal',(-o.points)::numeric,o.grant_ledger_entry_id
  FROM reward_orders o JOIN point_accounts p ON p.brand_id=o.brand_id AND p.brand_member_id=o.member_id
   LEFT JOIN reward_order_actions r ON r.brand_id=o.brand_id AND r.order_id=o.id AND r.operation IN('revoke') AND r.state_after='revoked'
  WHERE o.brand_id=b AND p.id=a AND o.points<>0 AND o.state='revoked'
 LOOP
  expected:=zero;
  allocation_valid:=true;
  IF edge.family='withdrawal' THEN
   allocation_valid:=false;
   IF jsonb_typeof(edge.allocation)='array' THEN
    allocation_valid:=jsonb_array_length(edge.allocation) BETWEEN 1 AND 4
     AND (SELECT coalesce(sum(CASE WHEN v->>'points' ~ '^(0|[1-9][0-9]*)([.][0-9]{1,2})?$' THEN (v->>'points')::numeric ELSE 0 END),0)
      FROM jsonb_array_elements(edge.allocation) v)=edge.amount
     AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements(edge.allocation) v WHERE jsonb_typeof(v)<>'object'
      OR (SELECT count(*) FROM jsonb_object_keys(v))<>3 OR NOT(v ?& ARRAY['source','state','points'])
      OR v->>'source' NOT IN('recharge','winning','gift','commission')
      OR v->>'state' IS DISTINCT FROM CASE WHEN edge.kind='paid' THEN 'withdrawal' ELSE 'available' END
      OR jsonb_typeof(v->'points')<>'string' OR v->>'points' !~ '^(0|[1-9][0-9]*)([.][0-9]{1,2})?$'
      OR CASE WHEN v->>'points' ~ '^(0|[1-9][0-9]*)([.][0-9]{1,2})?$' THEN (v->>'points')::numeric>9223372036854775807 ELSE false END)
     AND (SELECT count(DISTINCT v->>'source') FROM jsonb_array_elements(edge.allocation) v)=jsonb_array_length(edge.allocation);
   END IF;
   IF allocation_valid THEN
    FOR src IN SELECT s FROM unnest(ARRAY['recharge','winning','gift','commission']) s LOOP
    SELECT coalesce(sum((v->>'points')::numeric),0) INTO n FROM jsonb_array_elements(edge.allocation) v WHERE v->>'source'=src;
    IF edge.kind='reserve' THEN
     expected:=jsonb_set(expected,ARRAY[src,'available'],to_jsonb((-n)::text));
     expected:=jsonb_set(expected,ARRAY[src,'withdrawal'],to_jsonb(n::text));
    ELSIF edge.kind='paid' THEN
     expected:=jsonb_set(expected,ARRAY[src,'withdrawal'],to_jsonb((-n)::text));
    ELSE
     expected:=jsonb_set(expected,ARRAY[src,'available'],to_jsonb(n::text));
     expected:=jsonb_set(expected,ARRAY[src,'withdrawal'],to_jsonb((-n)::text));
    END IF;
   END LOOP;
   END IF;
  ELSIF edge.family IN('commission','commission_adjustment','commission_correction') THEN
   expected:=jsonb_set(expected,'{commission,available}',to_jsonb(edge.amount::text));
  ELSIF edge.kind='reward_grant' THEN
   expected:=jsonb_set(expected,'{gift,available}',to_jsonb(edge.amount::text));
  ELSE
   expected:=jsonb_set(expected,'{gift,available}',to_jsonb(edge.amount::text));
  END IF;

  valid := coalesce(allocation_valid,true) AND edge.audit_id IS NOT NULL AND
   (edge.family<>'withdrawal' OR EXISTS(
    SELECT 1 FROM withdrawal_orders o WHERE o.brand_id=b AND o.id=edge.source_id
     AND NOT(o.paid_entry_id IS NOT NULL AND o.release_entry_id IS NOT NULL)
     AND (edge.kind<>'paid' OR o.state='paid' AND o.release_entry_id IS NULL)
     AND (edge.kind<>'release' OR o.state IN('rejected','failed','cancelled') AND o.paid_entry_id IS NULL)
   ) AND EXISTS(
    SELECT 1 FROM audit_logs q JOIN withdrawal_order_transitions t ON t.audit_log_id=q.id
    WHERE q.id=edge.audit_id AND q.brand_id=b AND q.resource_type='withdrawal_order' AND q.resource_id=edge.source_id
     AND q.action='withdrawal.'||t.to_state AND q.actor_type=t.actor_type AND q.actor_id IS NOT DISTINCT FROM t.actor_id
     AND q.reason=t.reason AND q.after_json->>'version'=t.version::text));

  IF edge.family='commission' THEN
   valid:=valid AND EXISTS(SELECT 1 FROM audit_logs q WHERE q.id=edge.audit_id AND q.brand_id=b AND q.actor_type='system' AND q.actor_id IS NULL
    AND q.action='commission.payment.target' AND q.resource_type='commission_payment_target' AND q.resource_id=edge.source_id
    AND q.after_json->>'points'=edge.amount::text AND q.after_json->>'ledger_entry_id'=edge.ledger_id::text);
  ELSIF edge.family='commission_adjustment' THEN
   valid:=valid AND EXISTS(SELECT 1 FROM commission_adjustments x JOIN audit_logs q ON q.id=x.audit_log_id
    WHERE x.id=edge.source_id AND x.brand_id=b AND x.points_after::numeric-x.points_before::numeric=x.delta_points::numeric
     AND q.brand_id=b AND q.actor_type='admin' AND q.actor_id=x.created_by AND q.action='commission.adjustment.create'
     AND q.resource_type='commission_adjustment' AND q.resource_id=x.id AND q.reason=x.reason
     AND q.after_json->>'target_id'=x.target_id::text AND q.after_json->>'payment_id'=x.payment_id::text
     AND q.before_json->>'version'=(x.version-1)::text AND q.before_json->>'points'=x.points_before::text
     AND q.after_json->>'version'=x.version::text AND q.after_json->>'points'=x.points_after::text
     AND q.after_json->>'delta_points'=x.delta_points::text);
  ELSIF edge.family='commission_correction' THEN
   valid:=valid AND EXISTS(SELECT 1 FROM commission_correction_execution_targets t
    JOIN commission_correction_execution_steps s ON s.brand_id=t.brand_id AND s.execution_id=t.execution_id
     AND s.version=t.execution_version+1 AND s.operation='apply' AND s.target_id=t.id
    JOIN commission_correction_executions x ON x.brand_id=t.brand_id AND x.id=t.execution_id
    JOIN audit_logs q ON q.id=t.audit_log_id
    WHERE t.id=edge.source_id AND t.brand_id=b AND t.points_after::numeric-t.points_before::numeric=t.delta_points::numeric
     AND q.brand_id=b AND q.actor_type='system' AND q.actor_id IS NULL AND q.action='commission.correction_execution.target'
     AND q.resource_type='commission_correction_target' AND q.resource_id=t.id AND q.request_id=s.request_id
     AND t.ledger_entry_id IS NOT DISTINCT FROM edge.ledger_id AND x.approval_audit_log_id IS NOT NULL
     AND EXISTS(SELECT 1 FROM audit_logs aa WHERE aa.id=x.approval_audit_log_id AND aa.brand_id=b
      AND aa.resource_type='commission_correction_execution' AND aa.resource_id=x.id
      AND ((x.approval_actor_type='system' AND x.approved_by IS NULL AND aa.actor_type='system' AND aa.actor_id IS NULL
         AND aa.action='commission.correction_execution.create' AND x.approval_audit_log_id=x.creation_audit_log_id)
       OR (x.approval_actor_type='admin' AND x.approved_by IS NOT NULL AND aa.actor_type='admin' AND aa.actor_id=x.approved_by
         AND aa.action='commission.correction_execution.approve')))
     AND q.before_json=jsonb_build_object('state','pending') AND q.after_json=jsonb_build_object('state','applied',
      'execution_id',t.execution_id,'plan_target_id',t.plan_target_id,'points_before',t.points_before::text,
      'points_after',t.points_after::text,'delta_points',t.delta_points::text,'ledger_entry_id',t.ledger_entry_id,
      'financial_version',t.financial_version));
  ELSIF edge.family='reward' AND edge.kind='reward_grant' THEN
   valid:=valid AND EXISTS(SELECT 1 FROM reward_orders o JOIN reward_order_actions g ON g.brand_id=o.brand_id AND g.order_id=o.id AND g.version=1
    JOIN audit_logs q ON q.id=g.audit_log_id WHERE o.id=edge.source_id AND o.brand_id=b AND g.operation='grant'
     AND q.id=edge.audit_id AND g.ledger_entry_id IS NOT DISTINCT FROM edge.ledger_id
     AND o.grant_ledger_entry_id IS NOT DISTINCT FROM edge.ledger_id
     AND q.brand_id=b AND q.actor_type='admin' AND q.actor_id=o.created_by AND q.action='reward.order.grant'
     AND q.resource_type='reward_order' AND q.resource_id=o.id AND q.reason=o.reason
     AND q.after_json->>'action_id'=g.id::text AND q.after_json->>'version'='1' AND q.after_json->>'state'='granted'
     AND q.after_json->>'member_id'=o.member_id::text AND q.after_json->>'points'=o.points::text);
  ELSIF edge.family='reward' THEN
   valid:=valid AND EXISTS(SELECT 1 FROM reward_orders o JOIN reward_order_actions r ON r.brand_id=o.brand_id AND r.order_id=o.id
    JOIN audit_logs q ON q.id=r.audit_log_id WHERE r.id=edge.source_id AND o.brand_id=b AND o.state='revoked'
     AND o.revoke_ledger_entry_id IS NOT DISTINCT FROM edge.ledger_id AND r.ledger_entry_id IS NOT DISTINCT FROM edge.ledger_id
     AND q.id=edge.audit_id AND r.operation IN('revoke') AND r.state_after='revoked'
     AND q.brand_id=b AND q.actor_type='admin' AND q.actor_id=r.actor_id AND q.action='reward.order.'||r.operation
     AND q.resource_type='reward_order' AND q.resource_id=o.id AND q.reason=r.reason
     AND q.after_json->>'action_id'=r.id::text AND q.after_json->>'version'=r.version::text
     AND q.after_json->>'state'='revoked' AND q.before_json->>'state'=r.state_before
     AND q.before_json->>'version'=(r.version-1)::text);
  END IF;

  IF edge.ledger_id IS NOT NULL THEN
   SELECT * INTO l FROM point_ledger_entries WHERE id=edge.ledger_id;
   valid:=valid AND l.id IS NOT NULL AND l.brand_id=b AND l.account_id=edge.account_id AND l.member_id=edge.member_id
    AND l.entry_type=edge.expected_entry_type AND l.reference_type=edge.reference_type AND l.reference_id=edge.reference_id
    AND l.operation_key=edge.operation_key AND validated_point_snapshot(l.delta_snapshot)=expected AND l.source_allocation=edge.allocation
    AND l.reversal_of IS NOT DISTINCT FROM edge.reversal_of
    AND EXISTS(SELECT 1 FROM audit_logs q WHERE q.id=edge.audit_id AND l.actor_type=q.actor_type
     AND l.actor_id IS NOT DISTINCT FROM q.actor_id AND l.request_id=q.request_id
     AND (edge.family IN('commission','commission_correction') OR l.reason=q.reason));
   IF edge.family IN('commission','commission_correction') THEN
    valid:=valid AND l.actor_type='system' AND l.actor_id IS NULL AND l.reason IS NOT NULL;
   ELSIF edge.family IN('commission_adjustment','reward') THEN
    valid:=valid AND l.actor_type='admin' AND l.actor_id IS NOT NULL;
   END IF;
   valid:=valid AND valid_point_snapshot(l.before_snapshot,false) IS TRUE
    AND valid_point_snapshot(l.delta_snapshot,true) IS TRUE
    AND valid_point_snapshot(l.after_snapshot,false) IS TRUE;
   before_value:=validated_point_snapshot(l.before_snapshot);
   delta_value:=validated_point_snapshot(l.delta_snapshot);
   after_value:=validated_point_snapshot(l.after_snapshot);
   valid:=valid AND before_value IS NOT NULL AND delta_value IS NOT NULL AND after_value IS NOT NULL;
   IF before_value IS NOT NULL AND delta_value IS NOT NULL AND after_value IS NOT NULL THEN
    FOREACH src IN ARRAY ARRAY['recharge','winning','gift','commission'] LOOP
     FOREACH bucket IN ARRAY ARRAY['available','manual_frozen','system_frozen','withdrawal'] LOOP
      valid:=valid AND (before_value->src->>bucket)::numeric+(delta_value->src->>bucket)::numeric
       IS NOT DISTINCT FROM (after_value->src->>bucket)::numeric;
     END LOOP;
    END LOOP;
   END IF;
  ELSE
   valid:=false;
  END IF;

  family:=edge.family; source_id:=edge.source_id; ledger_id:=edge.ledger_id;
  source_valid:=coalesce(valid,false); expected_entry_type:=edge.expected_entry_type;
  RETURN NEXT;
 END LOOP;
END
$_$;
CREATE OR REPLACE FUNCTION valid_commission_correction_notification_event(b uuid, k text, aggregate uuid, payload jsonb) RETURNS boolean
    LANGUAGE plpgsql STABLE
    AS $_$
DECLARE amount numeric; valid boolean;
BEGIN
 IF b IS NULL OR aggregate IS NULL OR k IS DISTINCT FROM 'commission.corrected' OR
  jsonb_typeof(payload) IS DISTINCT FROM 'object' OR (SELECT count(*) FROM jsonb_object_keys(payload))<>5 OR
  NOT(payload ?& ARRAY['member_id','resource_id','points','ledger_entry_id','target_id']) OR
  jsonb_typeof(payload->'member_id') IS DISTINCT FROM 'string' OR jsonb_typeof(payload->'resource_id') IS DISTINCT FROM 'string' OR
  jsonb_typeof(payload->'points') IS DISTINCT FROM 'string' OR jsonb_typeof(payload->'ledger_entry_id') IS DISTINCT FROM 'string' OR
  jsonb_typeof(payload->'target_id') IS DISTINCT FROM 'string' OR
  payload->>'points' !~ '^-?(0|[1-9][0-9]*)([.][0-9]{1,2})?$' OR length(replace(payload->>'points','-',''))>19 THEN RETURN false; END IF;
 amount:=(payload->>'points')::numeric;
 IF amount=0 OR amount::text IS DISTINCT FROM payload->>'points' OR
  payload->>'resource_id' IS DISTINCT FROM aggregate::text OR payload->>'target_id' IS DISTINCT FROM aggregate::text THEN RETURN false; END IF;
 SELECT EXISTS(
  SELECT 1 FROM commission_correction_execution_targets t
  JOIN commission_correction_executions x ON x.brand_id=t.brand_id AND x.id=t.execution_id AND x.cycle_id=t.cycle_id
  JOIN point_ledger_entries l ON l.brand_id=t.brand_id AND l.id=t.ledger_entry_id
  JOIN audit_logs a ON a.id=t.audit_log_id
  JOIN commission_correction_execution_steps s ON s.brand_id=t.brand_id AND s.execution_id=t.execution_id AND s.target_id=t.id
  WHERE t.brand_id=b AND t.id=aggregate AND t.state='applied' AND t.delta_points=amount AND amount<>0
   AND t.member_id::text=payload->>'member_id' AND t.ledger_entry_id::text=payload->>'ledger_entry_id'
   AND t.applied_at IS NOT NULL AND t.financial_version IS NOT NULL AND x.approval_audit_log_id IS NOT NULL
   AND s.operation='apply' AND s.version=t.execution_version+1 AND s.actor_type='system' AND s.actor_id IS NULL
   AND a.brand_id=t.brand_id AND a.actor_type='system' AND a.actor_id IS NULL
   AND a.action='commission.correction_execution.target' AND a.resource_type='commission_correction_target'
   AND a.resource_id=t.id AND a.request_id=s.request_id
   AND a.before_json=jsonb_build_object('state','pending')
   AND a.after_json=jsonb_build_object('state','applied','execution_id',t.execution_id,'plan_target_id',t.plan_target_id,
    'points_before',t.points_before::text,'points_after',t.points_after::text,'delta_points',t.delta_points::text,
    'ledger_entry_id',t.ledger_entry_id,'financial_version',t.financial_version)
   AND l.member_id=t.member_id AND l.entry_type='commission_correction' AND l.reference_type='commission_correction_target'
   AND l.reference_id=t.id AND l.operation_key='commission-correction:'||t.id::text AND l.request_id=s.request_id
   AND l.actor_type='system' AND l.actor_id IS NULL AND l.reversal_of IS NULL
   AND l.delta_snapshot=jsonb_set(point_zero_snapshot(),'{commission,available}',to_jsonb(t.delta_points::text))
   AND l.source_allocation=jsonb_build_array(jsonb_build_object('source','commission','state','available','points',abs(t.delta_points::numeric)::text))
 ) INTO valid;
 RETURN valid;
EXCEPTION WHEN OTHERS THEN RETURN false;
END $_$;
CREATE OR REPLACE FUNCTION valid_commission_fraction(n text, d text) RETURNS boolean
    LANGUAGE plpgsql IMMUTABLE
    AS $_$
BEGIN
 IF n IS NULL OR d IS NULL OR length(n)>100 OR n !~ '^(0|[1-9][0-9]*)$' OR d !~ '^[1-9][0-9]{0,8}$' THEN RETURN false; END IF;
 RETURN d::numeric<=100000000 AND mod(100000000,d::numeric)=0 AND gcd(n::numeric,d::numeric)=1;
EXCEPTION WHEN OTHERS THEN RETURN false;
END
$_$;
CREATE OR REPLACE FUNCTION valid_commission_notification_event(b uuid, k text, aggregate uuid, payload jsonb) RETURNS boolean
    LANGUAGE plpgsql STABLE
    AS $_$
DECLARE v_member_id uuid; v_resource_id uuid; v_target_id uuid; v_ledger_id uuid; v_amount numeric; target commission_payment_targets; adjustment commission_adjustments; valid boolean;
BEGIN
 IF jsonb_typeof(payload)<>'object' OR (SELECT count(*) FROM jsonb_object_keys(payload))<>5 OR
  NOT(payload ?& ARRAY['member_id','resource_id','points','ledger_entry_id','target_id']) OR
  jsonb_typeof(payload->'member_id')<>'string' OR jsonb_typeof(payload->'resource_id')<>'string' OR
  jsonb_typeof(payload->'points')<>'string' OR jsonb_typeof(payload->'ledger_entry_id')<>'string' OR jsonb_typeof(payload->'target_id')<>'string' THEN RETURN false; END IF;
 IF (payload->>'member_id') !~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$' OR
  (payload->>'resource_id') !~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$' OR
  (payload->>'ledger_entry_id') !~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$' OR
  (payload->>'target_id') !~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$' THEN RETURN false; END IF;
 v_member_id:=(payload->>'member_id')::uuid; v_resource_id:=(payload->>'resource_id')::uuid;
 v_ledger_id:=(payload->>'ledger_entry_id')::uuid; v_target_id:=(payload->>'target_id')::uuid;
 IF (payload->>'points') !~ '^-?(0|[1-9][0-9]*)([.][0-9]{1,2})?$' OR length(replace(payload->>'points','-',''))>19 THEN RETURN false; END IF;
 v_amount:=(payload->>'points')::numeric;
 IF v_amount=0 OR v_amount::text<>(payload->>'points') THEN RETURN false; END IF;
 IF k='commission.paid' THEN
  IF v_amount<=0 OR aggregate<>v_target_id OR v_resource_id<>v_target_id THEN RETURN false; END IF;
  SELECT * INTO target FROM commission_payment_targets WHERE brand_id=b AND id=v_target_id;
  IF target.id IS NULL OR target.state<>'paid' OR target.points<=0 OR target.points::text<>(payload->>'points') OR target.member_id<>v_member_id OR target.ledger_entry_id<>v_ledger_id THEN RETURN false; END IF;
  SELECT EXISTS(SELECT 1 FROM point_ledger_entries l WHERE l.id=v_ledger_id AND l.brand_id=b AND l.member_id=v_member_id AND
   l.entry_type='commission' AND l.reference_type='commission_payment_target' AND l.reference_id=v_target_id AND
   l.operation_key='commission-payment:'||v_target_id::text AND l.actor_type='system' AND l.actor_id IS NULL AND l.reversal_of IS NULL AND
   l.delta_snapshot=jsonb_set(point_zero_snapshot(),'{commission,available}',to_jsonb(target.points::text)) AND
   l.source_allocation=jsonb_build_array(jsonb_build_object('source','commission','state','available','points',target.points::text))) INTO valid;
  RETURN valid;
 ELSIF k='commission.adjusted' THEN
  IF v_resource_id<>aggregate THEN RETURN false; END IF;
  SELECT * INTO adjustment FROM commission_adjustments WHERE brand_id=b AND id=aggregate;
  IF adjustment.id IS NULL OR adjustment.target_id<>v_target_id OR adjustment.ledger_entry_id<>v_ledger_id OR adjustment.delta_points<>v_amount THEN RETURN false; END IF;
  SELECT EXISTS(SELECT 1 FROM commission_payment_targets t JOIN point_ledger_entries l ON l.id=v_ledger_id AND l.brand_id=t.brand_id
   WHERE t.brand_id=b AND t.id=v_target_id AND t.member_id=v_member_id AND l.member_id=t.member_id AND
   l.entry_type='commission_adjustment' AND l.reference_type='commission_adjustment' AND l.reference_id=aggregate AND
   l.operation_key='commission-adjustment:'||aggregate::text AND l.actor_type='admin' AND l.actor_id=adjustment.created_by AND l.reversal_of IS NULL AND
   l.delta_snapshot=jsonb_set(point_zero_snapshot(),'{commission,available}',to_jsonb(adjustment.delta_points::text)) AND
   l.source_allocation=jsonb_build_array(jsonb_build_object('source','commission','state','available','points',abs(adjustment.delta_points::numeric)::text))) INTO valid;
  RETURN valid;
 END IF;
 RETURN false;
EXCEPTION WHEN OTHERS THEN RETURN false;
END $_$;
CREATE OR REPLACE FUNCTION valid_point_snapshot(v jsonb, signed_amounts boolean) RETURNS boolean
    LANGUAGE plpgsql IMMUTABLE
    AS $_$
DECLARE src text; st text; n numeric; total numeric:=0; sources integer;
BEGIN
 IF v IS NULL OR jsonb_typeof(v) IS DISTINCT FROM 'object' THEN RETURN false; END IF;
 SELECT count(*) INTO sources FROM jsonb_object_keys(v);
 IF NOT(v ?& ARRAY['recharge','winning','gift']) OR
  NOT(sources=4 AND v ? 'commission') THEN RETURN false; END IF;
 FOR src IN SELECT jsonb_object_keys(v) LOOP
  IF src NOT IN('recharge','winning','gift','commission') OR jsonb_typeof(v->src) IS DISTINCT FROM 'object' OR
   (SELECT count(*) FROM jsonb_object_keys(v->src))<>4 OR
   NOT(v->src ?& ARRAY['available','manual_frozen','system_frozen','withdrawal']) THEN RETURN false; END IF;
  FOREACH st IN ARRAY ARRAY['available','manual_frozen','system_frozen','withdrawal'] LOOP
   IF jsonb_typeof(v->src->st) IS DISTINCT FROM 'string' OR
    v->src->>st !~ (CASE WHEN signed_amounts THEN '^-?(0|[1-9][0-9]*)([.][0-9]{1,2})?$' ELSE '^(0|[1-9][0-9]*)([.][0-9]{1,2})?$' END) THEN RETURN false; END IF;
   n:=(v->src->>st)::numeric;
   IF n=0 AND left(v->src->>st,1)='-' THEN RETURN false; END IF;
   IF n < -92233720368547758.08 OR n > 92233720368547758.07 THEN RETURN false; END IF;
   total:=total+n;
  END LOOP;
 END LOOP;
 RETURN signed_amounts OR total<=92233720368547758.07;
EXCEPTION WHEN OTHERS THEN RETURN false;
END $_$;
CREATE OR REPLACE FUNCTION valid_reward_notification_event(b uuid, k text, aggregate uuid, payload jsonb) RETURNS boolean
    LANGUAGE plpgsql STABLE
    AS $_$
DECLARE a reward_order_actions; o reward_orders; log audit_logs; l point_ledger_entries; amount numeric; expected jsonb;
BEGIN
 IF b IS NULL OR aggregate IS NULL OR k IS NULL OR k NOT IN('reward.order.granted','reward.order.revoked') OR
  jsonb_typeof(payload) IS DISTINCT FROM 'object' OR (SELECT count(*) FROM jsonb_object_keys(payload))<>6 OR
  NOT(payload ?& ARRAY['member_id','resource_id','points','action_id','version','audit_log_id']) OR
  jsonb_typeof(payload->'member_id') IS DISTINCT FROM 'string' OR jsonb_typeof(payload->'resource_id') IS DISTINCT FROM 'string' OR
  jsonb_typeof(payload->'points') IS DISTINCT FROM 'string' OR jsonb_typeof(payload->'action_id') IS DISTINCT FROM 'string' OR
  jsonb_typeof(payload->'version') IS DISTINCT FROM 'number' OR jsonb_typeof(payload->'audit_log_id') IS DISTINCT FROM 'string' THEN RETURN false; END IF;
 IF (payload->>'points') !~ '^(0|[1-9][0-9]*)([.][0-9]{1,2})?$' OR length(payload->>'points')>19 OR
  (payload->>'version') !~ '^[1-9][0-9]*$' OR (payload->>'version')::numeric>9007199254740991 THEN RETURN false; END IF;
 amount:=(payload->>'points')::numeric;
 IF amount<=0 OR amount::text<>(payload->>'points') THEN RETURN false; END IF;
 SELECT * INTO a FROM reward_order_actions WHERE brand_id=b AND id=aggregate;
 SELECT * INTO o FROM reward_orders WHERE brand_id=b AND id=a.order_id;
 IF a.id IS NULL OR o.id IS NULL OR (payload->>'action_id') IS DISTINCT FROM a.id::text OR
  (payload->>'member_id') IS DISTINCT FROM o.member_id::text OR (payload->>'resource_id') IS DISTINCT FROM o.id::text OR
  (payload->>'version') IS DISTINCT FROM a.version::text OR (payload->>'audit_log_id') IS DISTINCT FROM a.audit_log_id::text OR
  amount<>o.points OR k<>'reward.order.'||a.state_after THEN RETURN false; END IF;
 SELECT * INTO log FROM audit_logs WHERE id=a.audit_log_id;
 IF log.brand_id IS DISTINCT FROM b OR log.actor_type IS DISTINCT FROM 'admin' OR log.actor_id IS DISTINCT FROM a.actor_id OR
  log.action IS DISTINCT FROM 'reward.order.'||a.operation OR log.resource_type IS DISTINCT FROM 'reward_order' OR log.resource_id IS DISTINCT FROM o.id OR
  log.reason IS DISTINCT FROM a.reason OR log.after_json->>'action_id' IS DISTINCT FROM a.id::text OR
  log.after_json->>'version' IS DISTINCT FROM a.version::text OR log.after_json->>'state' IS DISTINCT FROM a.state_after THEN RETURN false; END IF;
 SELECT * INTO l FROM point_ledger_entries WHERE id=a.ledger_entry_id;
 IF l.id IS NULL OR l.brand_id IS DISTINCT FROM b OR l.member_id IS DISTINCT FROM o.member_id OR
  l.reference_type IS DISTINCT FROM 'reward_order' OR l.reference_id IS DISTINCT FROM o.id OR
  l.actor_type IS DISTINCT FROM 'admin' OR l.actor_id IS DISTINCT FROM a.actor_id OR l.reason IS DISTINCT FROM a.reason OR
  l.source_allocation IS DISTINCT FROM jsonb_build_array(jsonb_build_object('source','gift','state','available','points',o.points::text)) THEN RETURN false; END IF;
 IF a.state_after='granted' THEN
  expected:=jsonb_set(point_zero_snapshot(),'{gift,available}',to_jsonb(o.points::text));
  RETURN a.operation='grant' AND a.version=1 AND a.state_before IS NULL AND a.ledger_entry_id=o.grant_ledger_entry_id AND
   l.entry_type='reward_grant' AND l.operation_key='reward-grant:'||o.id::text AND l.reversal_of IS NULL AND l.delta_snapshot=expected;
 END IF;
 expected:=jsonb_set(point_zero_snapshot(),'{gift,available}',to_jsonb((-o.points)::text));
 RETURN a.state_after='revoked' AND a.operation IN('revoke') AND a.version>1 AND a.ledger_entry_id=o.revoke_ledger_entry_id AND
  l.entry_type='reward_reversal' AND l.operation_key='reward-revoke:'||o.id::text AND l.reversal_of=o.grant_ledger_entry_id AND l.delta_snapshot=expected;
EXCEPTION WHEN OTHERS THEN RETURN false;
END $_$;
CREATE OR REPLACE FUNCTION valid_withdrawal_policy(v jsonb, game_scope boolean) RETURNS boolean
    LANGUAGE plpgsql IMMUTABLE
    AS $_$
DECLARE key_count integer; sources integer;
BEGIN
 IF jsonb_typeof(v)<>'object' THEN RETURN false; END IF;
 SELECT count(*) INTO key_count FROM jsonb_object_keys(v);
 IF game_scope THEN RETURN key_count=1 AND v ? 'turnover_multiple' AND (v->'turnover_multiple'='null'::jsonb OR valid_withdrawal_multiple(v->'turnover_multiple')); END IF;
 IF key_count<>6 OR NOT v ?& ARRAY['enabled','min_points','max_points','allowed_sources','review_mode','turnover_multiple'] OR jsonb_typeof(v->'enabled')<>'boolean' OR jsonb_typeof(v->'min_points')<>'string' OR NOT (v->>'min_points') ~ '^(0|[1-9][0-9]*)([.][0-9]{1,2})?$' OR (v->>'min_points')::numeric>9223372036854775807 OR jsonb_typeof(v->'allowed_sources')<>'array' OR jsonb_typeof(v->'review_mode')<>'string' OR v->>'review_mode' NOT IN ('manual','automatic') OR NOT valid_withdrawal_multiple(v->'turnover_multiple') THEN RETURN false; END IF;
 IF v->'max_points'<>'null'::jsonb AND (jsonb_typeof(v->'max_points')<>'string' OR NOT (v->>'max_points') ~ '^(0|[1-9][0-9]*)([.][0-9]{1,2})?$' OR (v->>'max_points')::numeric>9223372036854775807 OR (v->>'max_points')::numeric<(v->>'min_points')::numeric) THEN RETURN false; END IF;
 sources:=jsonb_array_length(v->'allowed_sources');
 IF sources<1 OR sources>4 OR EXISTS(SELECT 1 FROM jsonb_array_elements(v->'allowed_sources') s WHERE jsonb_typeof(s)<>'string' OR s#>>'{}' NOT IN ('recharge','winning','gift','commission')) OR (SELECT count(DISTINCT s) FROM jsonb_array_elements(v->'allowed_sources') s)<>sources THEN RETURN false; END IF;
 RETURN true;
EXCEPTION WHEN OTHERS THEN RETURN false;
END $_$;
CREATE OR REPLACE FUNCTION validate_point_ledger_projection() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE a point_accounts; tip point_ledger_entries; actual jsonb;
BEGIN
 SELECT * INTO a FROM point_accounts WHERE brand_id=NEW.brand_id AND id=NEW.account_id;
 SELECT * INTO tip FROM point_ledger_entries WHERE brand_id=NEW.brand_id AND account_id=NEW.account_id ORDER BY version DESC LIMIT 1;
 SELECT jsonb_object_agg(source,states) INTO actual FROM (
  SELECT source,jsonb_object_agg(state,trim_scale(points)::text) states FROM point_buckets
  WHERE brand_id=NEW.brand_id AND account_id=NEW.account_id GROUP BY source
 ) buckets;
 IF tip.id IS NULL OR a.version IS DISTINCT FROM tip.version OR actual IS DISTINCT FROM tip.after_snapshot OR
  NOT EXISTS(SELECT 1 FROM audit_logs x WHERE x.brand_id=NEW.brand_id AND x.action='points.'||NEW.entry_type AND
   x.actor_type=NEW.actor_type AND x.actor_id IS NOT DISTINCT FROM NEW.actor_id AND x.resource_type='point_account' AND x.resource_id=NEW.account_id AND
   x.before_json=NEW.before_snapshot AND x.after_json->>'ledger_entry_id'=NEW.id::text AND x.after_json->'balance'=NEW.after_snapshot) THEN
  RAISE EXCEPTION 'ledger requires atomically applied wallet tip and matching audit';
 END IF;
 RETURN NULL;
END $$;
CREATE OR REPLACE FUNCTION validate_settlement_payout_witness() RETURNS trigger
    LANGUAGE plpgsql
    AS $_$
DECLARE l point_ledger_entries; zero_bucket jsonb; expected jsonb; src text; bucket text;
BEGIN
 IF OLD.status<>'placed' OR NEW.status NOT IN ('won','lost') OR NEW.prize_points=0 THEN RETURN NEW; END IF;
 SELECT * INTO l FROM point_ledger_entries WHERE id=NEW.payout_entry_id;
 zero_bucket:=jsonb_build_object('available','0','manual_frozen','0','system_frozen','0','withdrawal','0');
 expected:=jsonb_build_object('recharge',zero_bucket,'gift',zero_bucket,'commission',zero_bucket,'winning',zero_bucket||jsonb_build_object('available',NEW.prize_points::text));
 IF l.id IS NULL OR l.account_id IS DISTINCT FROM NEW.account_id OR l.delta_snapshot IS DISTINCT FROM expected OR l.operation_key IS DISTINCT FROM 'settlement-payout:'||NEW.settlement_calculation_id::text OR
 jsonb_typeof(l.before_snapshot) IS DISTINCT FROM 'object' OR jsonb_typeof(l.after_snapshot) IS DISTINCT FROM 'object' THEN RAISE EXCEPTION 'incomplete settlement payout witness'; END IF;
 IF (SELECT count(*) FROM jsonb_object_keys(l.before_snapshot))<>4 OR (SELECT count(*) FROM jsonb_object_keys(l.after_snapshot))<>4 THEN RAISE EXCEPTION 'invalid payout snapshot sources'; END IF;
 FOREACH src IN ARRAY ARRAY['recharge','winning','gift','commission'] LOOP
  IF jsonb_typeof(l.before_snapshot->src) IS DISTINCT FROM 'object' OR jsonb_typeof(l.after_snapshot->src) IS DISTINCT FROM 'object' THEN RAISE EXCEPTION 'missing payout source snapshot'; END IF;
  IF (SELECT count(*) FROM jsonb_object_keys(l.before_snapshot->src))<>4 OR (SELECT count(*) FROM jsonb_object_keys(l.after_snapshot->src))<>4 THEN RAISE EXCEPTION 'invalid payout snapshot buckets'; END IF;
  FOREACH bucket IN ARRAY ARRAY['available','manual_frozen','system_frozen','withdrawal'] LOOP
   IF jsonb_typeof(l.before_snapshot->src->bucket) IS DISTINCT FROM 'string' OR jsonb_typeof(l.after_snapshot->src->bucket) IS DISTINCT FROM 'string' OR
   (l.before_snapshot->src->>bucket) !~ '^(0|[1-9][0-9]*)([.][0-9]{1,2})?$' OR (l.after_snapshot->src->>bucket) !~ '^(0|[1-9][0-9]*)([.][0-9]{1,2})?$' THEN RAISE EXCEPTION 'invalid payout integer snapshot'; END IF;
   IF (l.before_snapshot->src->>bucket)::numeric+(expected->src->>bucket)::numeric IS DISTINCT FROM (l.after_snapshot->src->>bucket)::numeric OR
   NOT EXISTS(SELECT 1 FROM point_buckets b WHERE b.brand_id=NEW.brand_id AND b.account_id=NEW.account_id AND b.source=src AND b.state=bucket AND b.points::numeric=(l.after_snapshot->src->>bucket)::numeric) THEN RAISE EXCEPTION 'payout snapshot must match applied buckets'; END IF;
  END LOOP;
 END LOOP;
 IF NOT EXISTS(SELECT 1 FROM point_accounts a WHERE a.brand_id=NEW.brand_id AND a.id=NEW.account_id AND a.version=l.version) OR
 NOT EXISTS(SELECT 1 FROM point_ledger_entries prev WHERE prev.brand_id=NEW.brand_id AND prev.account_id=NEW.account_id AND prev.version=l.version-1 AND validated_point_snapshot(prev.after_snapshot)=l.before_snapshot) OR
 NOT EXISTS(SELECT 1 FROM audit_logs a WHERE a.brand_id=NEW.brand_id AND a.actor_type='system' AND a.action='points.prize' AND a.resource_type='point_account' AND a.resource_id=NEW.account_id AND a.before_json=l.before_snapshot AND a.after_json->>'ledger_entry_id'=l.id::text AND a.after_json->'balance'=l.after_snapshot) THEN RAISE EXCEPTION 'payout requires applied ledger tip and audit witness'; END IF;
 RETURN NEW;
END $_$;
CREATE OR REPLACE FUNCTION validate_withdrawal_funds() RETURNS trigger
    LANGUAGE plpgsql
    AS $_$
DECLARE o withdrawal_orders; e point_ledger_entries; f point_ledger_entries; latest withdrawal_order_transitions; allocation jsonb; total numeric:=0; expected jsonb:='{}'; item jsonb; src text; n numeric; seen text[]:='{}'; h bigint;
BEGIN
 SELECT * INTO o FROM withdrawal_orders WHERE id=NEW.id;
 SELECT * INTO e FROM point_ledger_entries WHERE id=o.reserve_entry_id;
 IF e.brand_id IS DISTINCT FROM o.brand_id OR e.account_id IS DISTINCT FROM o.account_id OR e.member_id IS DISTINCT FROM o.member_id OR e.entry_type<>'withdrawal_reserve' OR e.reference_type<>'withdrawal' OR e.reference_id IS DISTINCT FROM o.id OR e.version<>o.reserve_version OR e.reversal_of IS NOT NULL OR e.source_allocation IS DISTINCT FROM o.source_allocation THEN RAISE EXCEPTION 'withdrawal reserve lacks exact ledger evidence'; END IF;
 IF jsonb_array_length(o.source_allocation) NOT BETWEEN 1 AND 4 THEN RAISE EXCEPTION 'invalid withdrawal allocation'; END IF;
 FOR item IN SELECT value FROM jsonb_array_elements(o.source_allocation) LOOP
  src:=item->>'source';
  IF jsonb_typeof(item)<>'object' OR (SELECT count(*) FROM jsonb_object_keys(item))<>3 OR NOT(item ?& ARRAY['source','state','points']) OR src NOT IN('recharge','winning','gift','commission') OR src=ANY(seen) OR item->>'state'<>'available' OR jsonb_typeof(item->'points')<>'string' OR item->>'points' !~ '^(0|[1-9][0-9]*)([.][0-9]{1,2})?$' THEN RAISE EXCEPTION 'invalid withdrawal source allocation'; END IF;
  n:=(item->>'points')::numeric; total:=total+n; seen:=array_append(seen,src);
 END LOOP;
 IF total<>o.points THEN RAISE EXCEPTION 'withdrawal allocation does not sum to amount'; END IF;
 FOREACH src IN ARRAY ARRAY['recharge','winning','gift','commission'] LOOP
  SELECT coalesce(sum((value->>'points')::numeric),0) INTO n FROM jsonb_array_elements(o.source_allocation) WHERE value->>'source'=src;
  expected:=expected||jsonb_build_object(src,jsonb_build_object('available',trim_scale(-n)::text,'manual_frozen','0','system_frozen','0','withdrawal',trim_scale(n)::text));
 END LOOP;
 IF validated_point_snapshot(e.delta_snapshot) IS DISTINCT FROM expected THEN RAISE EXCEPTION 'withdrawal reserve changed wrong buckets'; END IF;
 IF o.release_entry_id IS NOT NULL THEN
  SELECT * INTO f FROM point_ledger_entries WHERE id=o.release_entry_id;
  IF f.reversal_of IS DISTINCT FROM e.id OR f.entry_type<>'withdrawal_release' OR f.reference_type<>'withdrawal' OR f.reference_id IS DISTINCT FROM o.id OR f.member_id<>o.member_id OR f.source_allocation IS DISTINCT FROM o.source_allocation THEN RAISE EXCEPTION 'withdrawal release is not original-source full reversal'; END IF;
  expected:='{}';
  FOREACH src IN ARRAY ARRAY['recharge','winning','gift','commission'] LOOP
   n:=(validated_point_snapshot(e.delta_snapshot)->src->>'withdrawal')::bigint;
   expected:=expected||jsonb_build_object(src,jsonb_build_object('available',n::text,'manual_frozen','0','system_frozen','0','withdrawal',(-n)::text));
  END LOOP;
  IF validated_point_snapshot(f.delta_snapshot) IS DISTINCT FROM expected THEN RAISE EXCEPTION 'withdrawal release changed wrong buckets'; END IF;
 END IF;
 IF o.paid_entry_id IS NOT NULL THEN
  SELECT * INTO f FROM point_ledger_entries WHERE id=o.paid_entry_id;
  expected:='{}';
  FOREACH src IN ARRAY ARRAY['recharge','winning','gift','commission'] LOOP
   n:=(validated_point_snapshot(e.delta_snapshot)->src->>'withdrawal')::bigint;
   expected:=expected||jsonb_build_object(src,jsonb_build_object('available','0','manual_frozen','0','system_frozen','0','withdrawal',(-n)::text));
  END LOOP;
  IF f.entry_type<>'withdrawal_paid' OR f.reference_type<>'withdrawal' OR f.reference_id IS DISTINCT FROM o.id OR f.member_id<>o.member_id OR f.reversal_of IS NOT NULL OR validated_point_snapshot(f.delta_snapshot) IS DISTINCT FROM expected THEN RAISE EXCEPTION 'withdrawal paid evidence changed wrong buckets'; END IF;
  IF NOT EXISTS(SELECT 1 FROM withdrawal_turnover_cycles WHERE brand_id=o.brand_id AND member_id=o.member_id AND cutoff_version>=o.reserve_version) THEN RAISE EXCEPTION 'paid withdrawal has no successful cycle boundary'; END IF;
 END IF;
 SELECT * INTO latest FROM withdrawal_order_transitions WHERE order_id=o.id ORDER BY version DESC LIMIT 1;
 SELECT count(*) INTO h FROM withdrawal_order_transitions WHERE order_id=o.id;
 IF latest.id IS NULL OR latest.version<>o.version OR latest.to_state<>o.state OR latest.created_at<>o.updated_at OR h<>o.version THEN RAISE EXCEPTION 'withdrawal projection has no complete immutable history'; END IF;
 RETURN NULL;
END $_$;
CREATE OR REPLACE FUNCTION validate_withdrawal_policy_current() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE selected_game uuid:=NULLIF(to_jsonb(NEW)->>'game_id','')::uuid;
BEGIN
 IF NOT EXISTS(SELECT 1 FROM withdrawal_policy_revisions r WHERE r.brand_id=NEW.brand_id AND r.game_id IS NOT DISTINCT FROM selected_game AND r.version=NEW.version AND r.config=NEW.config AND (selected_game IS NOT NULL OR r.points_step=(to_jsonb(NEW)->>'points_step')::numeric)) THEN RAISE EXCEPTION 'current policy must commit with matching immutable revision'; END IF;
 RETURN NULL;
END $$;
CREATE OR REPLACE FUNCTION point_configured_business_witnesses(b uuid,a uuid)
RETURNS TABLE(family text,source_id uuid,ledger_id uuid,source_valid boolean,expected_entry_type text)
LANGUAGE sql STABLE AS $$
 SELECT e.kind,o.id,e.ledger,
 COALESCE(l.brand_id=o.brand_id AND l.member_id=o.member_id AND l.account_id=a AND
 l.reference_type='configured_order' AND l.reference_id=o.id AND l.entry_type=e.kind AND
 l.operation_key='configured-'||e.operation||':'||o.id::text AND l.reversal_of IS NULL AND
 lottery_business_ledger_applied(l,b,a) AND
 validated_point_snapshot(l.delta_snapshot)=configured_funding_delta(l.source_allocation,
  CASE WHEN e.operation='bet' THEN 'available' WHEN e.operation='loss' OR e.operation='refund' AND o.snapshot->'catalog'->>'funding'='reserve' THEN 'system_frozen' ELSE '' END,
  CASE WHEN e.operation='bet' AND o.snapshot->'catalog'->>'funding'='reserve' THEN 'system_frozen' WHEN e.operation IN('prize','refund') THEN 'available' ELSE '' END) AND
 (SELECT sum((x->>'points')::numeric) FROM jsonb_array_elements(l.source_allocation) x)=e.points AND
 CASE WHEN e.operation='bet' THEN l.source_allocation=o.allocation AND l.actor_type='user' AND l.actor_id=o.user_id
 ELSE o.status='settled' AND l.actor_type='system' AND l.actor_id IS NULL END AND
 EXISTS(SELECT 1 FROM audit_logs q WHERE q.brand_id=b AND q.resource_type='configured_order' AND q.resource_id=o.id AND
 q.action=CASE WHEN e.operation='bet' THEN 'configured.place' ELSE 'configured.settle' END AND q.request_id=l.request_id),false),e.kind
 FROM configured_orders o
 JOIN point_accounts pa ON pa.brand_id=o.brand_id AND pa.brand_member_id=o.member_id
 CROSS JOIN LATERAL (VALUES
 ('bet'::text,o.debit_entry_id,CASE WHEN o.snapshot->'catalog'->>'funding'='reserve' THEN (o.quote->>'reserved_points')::numeric ELSE o.total_points END,'bet'::text),
 ('bet',o.loss_entry_id,CASE WHEN o.snapshot->'catalog'->>'funding'='reserve' THEN coalesce((o.result->>'consumed_points')::numeric,0) ELSE 0 END,'loss'),
 ('prize',o.payout_entry_id,coalesce((o.result->>'prize_points')::numeric,0),'prize'),
 ('refund',o.refund_entry_id,coalesce((o.result->>'refund_points')::numeric,0),'refund')) e(kind,ledger,points,operation)
 LEFT JOIN point_ledger_entries l ON l.id=e.ledger
 WHERE o.brand_id=b AND pa.id=a AND e.points>0
$$;
CREATE VIEW betting_financial_facts AS SELECT o.id,o.brand_id,o.member_id AS brand_member_id,o.user_id AS global_user_id,
 d.account_id,o.game_id,o.period_id,o.rule_id AS rule_version_id,o.total_points,o.allocation AS deduction_allocation,
 o.debit_entry_id,o.payout_entry_id,o.refund_entry_id,o.loss_entry_id,o.placed_at,o.settled_at,
 o.attribution_snapshot,o.commission_rule_snapshot,o.withdrawal_rule_snapshot,o.snapshot,o.quote,o.result,
 CASE WHEN o.status='placed' THEN 'placed' WHEN (o.result->>'prize_points')::numeric>0 THEN 'won' ELSE 'lost' END AS status,
 coalesce((o.result->>'prize_points')::numeric,0) AS prize_points,
 CASE WHEN o.snapshot->'catalog'->>'funding'='reserve' THEN 0 ELSE coalesce((o.result->>'refund_points')::numeric,0) END AS refund_points,
 coalesce((o.result->>'effective_points')::numeric,0) AS effective_points,
 CASE WHEN o.snapshot->'catalog'->>'funding'='reserve' THEN coalesce((o.result->>'consumed_points')::numeric,0)
 ELSE o.total_points-coalesce((o.result->>'refund_points')::numeric,0) END AS consumed_points,
 greatest(CASE WHEN o.snapshot->'catalog'->>'funding'='reserve' THEN coalesce((o.result->>'consumed_points')::numeric,0)
 ELSE o.total_points::numeric-coalesce((o.result->>'refund_points')::numeric,0) END-coalesce((o.result->>'prize_points')::numeric,0),0)::numeric AS loss_points
 FROM configured_orders o JOIN point_ledger_entries d ON d.id=o.debit_entry_id;
ALTER TABLE brand_point_policies ADD CONSTRAINT brand_point_policies_max_balance_points_precision CHECK (max_balance_points=trunc(max_balance_points,2) AND max_balance_points BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE brand_point_policies ADD CONSTRAINT brand_point_policies_max_recharge_points_precision CHECK (max_recharge_points=trunc(max_recharge_points,2) AND max_recharge_points BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE brand_point_policies ADD CONSTRAINT brand_point_policies_max_adjustment_points_precision CHECK (max_adjustment_points=trunc(max_adjustment_points,2) AND max_adjustment_points BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE brand_withdrawal_policies ADD CONSTRAINT brand_withdrawal_policies_points_step_precision CHECK (points_step=trunc(points_step,2) AND points_step BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE commission_adjustment_heads ADD CONSTRAINT commission_adjustment_heads_points_precision CHECK (points=trunc(points,2) AND points BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE commission_adjustments ADD CONSTRAINT commission_adjustments_points_before_precision CHECK (points_before=trunc(points_before,2) AND points_before BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE commission_adjustments ADD CONSTRAINT commission_adjustments_points_after_precision CHECK (points_after=trunc(points_after,2) AND points_after BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE commission_adjustments ADD CONSTRAINT commission_adjustments_delta_points_precision CHECK (delta_points=trunc(delta_points,2) AND delta_points BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE commission_calculations ADD CONSTRAINT commission_calculations_stake_points_precision CHECK (stake_points=trunc(stake_points,2) AND stake_points BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE commission_calculations ADD CONSTRAINT commission_calculations_prize_points_precision CHECK (prize_points=trunc(prize_points,2) AND prize_points BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE commission_calculations ADD CONSTRAINT commission_calculations_base_points_precision CHECK (base_points=trunc(base_points,2) AND base_points BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE commission_correction_balance_heads ADD CONSTRAINT commission_correction_balance_heads_points_precision CHECK (points=trunc(points,2) AND points BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE commission_correction_execution_targets ADD CONSTRAINT commission_correction_execution_targets_points_before_precision CHECK (points_before=trunc(points_before,2) AND points_before BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE commission_correction_execution_targets ADD CONSTRAINT commission_correction_execution_targets_points_after_precision CHECK (points_after=trunc(points_after,2) AND points_after BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE commission_correction_execution_targets ADD CONSTRAINT commission_correction_execution_targets_delta_points_precision CHECK (delta_points=trunc(delta_points,2) AND delta_points BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE commission_correction_executions ADD CONSTRAINT commission_correction_executions_credit_points_precision CHECK (credit_points=trunc(credit_points,2) AND credit_points BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE commission_correction_executions ADD CONSTRAINT commission_correction_executions_debit_points_precision CHECK (debit_points=trunc(debit_points,2) AND debit_points BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE commission_correction_executions ADD CONSTRAINT commission_correction_executions_net_points_precision CHECK (net_points=trunc(net_points,2) AND net_points BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE commission_correction_plan_targets ADD CONSTRAINT commission_correction_plan_targets_points_before_precision CHECK (points_before=trunc(points_before,2) AND points_before BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE commission_correction_plan_targets ADD CONSTRAINT commission_correction_plan_targets_points_after_precision CHECK (points_after=trunc(points_after,2) AND points_after BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE commission_correction_plan_targets ADD CONSTRAINT commission_correction_plan_targets_delta_points_precision CHECK (delta_points=trunc(delta_points,2) AND delta_points BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE commission_correction_plans ADD CONSTRAINT commission_correction_plans_before_points_precision CHECK (before_points=trunc(before_points,2) AND before_points BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE commission_correction_plans ADD CONSTRAINT commission_correction_plans_calculated_points_precision CHECK (calculated_points=trunc(calculated_points,2) AND calculated_points BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE commission_correction_plans ADD CONSTRAINT commission_correction_plans_credit_points_precision CHECK (credit_points=trunc(credit_points,2) AND credit_points BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE commission_correction_plans ADD CONSTRAINT commission_correction_plans_debit_points_precision CHECK (debit_points=trunc(debit_points,2) AND debit_points BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE commission_correction_plans ADD CONSTRAINT commission_correction_plans_net_points_precision CHECK (net_points=trunc(net_points,2) AND net_points BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE commission_earnings ADD CONSTRAINT commission_earnings_points_precision CHECK (points=trunc(points,2) AND points BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE commission_payment_targets ADD CONSTRAINT commission_payment_targets_points_precision CHECK (points=trunc(points,2) AND points BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE commission_payments ADD CONSTRAINT commission_payments_total_points_precision CHECK (total_points=trunc(total_points,2) AND total_points BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE point_buckets ADD CONSTRAINT point_buckets_points_precision CHECK (points=trunc(points,2) AND points BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE recharge_orders ADD CONSTRAINT recharge_orders_points_precision CHECK (points=trunc(points,2) AND points BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE reward_orders ADD CONSTRAINT reward_orders_points_precision CHECK (points=trunc(points,2) AND points BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE withdrawal_orders ADD CONSTRAINT withdrawal_orders_points_precision CHECK (points=trunc(points,2) AND points BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE withdrawal_policy_revisions ADD CONSTRAINT withdrawal_policy_revisions_points_step_precision CHECK (points_step=trunc(points_step,2) AND points_step BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE brand_mark_six_sales ADD CONSTRAINT brand_mark_six_sales_min_points_precision CHECK (min_points=trunc(min_points,2) AND min_points BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE brand_mark_six_sales ADD CONSTRAINT brand_mark_six_sales_max_points_precision CHECK (max_points=trunc(max_points,2) AND max_points BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE brand_mark_six_sales ADD CONSTRAINT brand_mark_six_sales_max_period_points_precision CHECK (max_period_points=trunc(max_period_points,2) AND max_period_points BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE brand_mark_six_sales ADD CONSTRAINT brand_mark_six_sales_max_user_period_points_precision CHECK (max_user_period_points=trunc(max_user_period_points,2) AND max_user_period_points BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE brand_canada_sales ADD CONSTRAINT brand_canada_sales_min_points_precision CHECK (min_points=trunc(min_points,2) AND min_points BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE brand_canada_sales ADD CONSTRAINT brand_canada_sales_max_points_precision CHECK (max_points=trunc(max_points,2) AND max_points BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE brand_canada_sales ADD CONSTRAINT brand_canada_sales_max_period_points_precision CHECK (max_period_points=trunc(max_period_points,2) AND max_period_points BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE brand_canada_sales ADD CONSTRAINT brand_canada_sales_max_user_period_points_precision CHECK (max_user_period_points=trunc(max_user_period_points,2) AND max_user_period_points BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE configured_sales ADD CONSTRAINT configured_sales_min_points_precision CHECK (min_points=trunc(min_points,2) AND min_points BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE configured_sales ADD CONSTRAINT configured_sales_max_points_precision CHECK (max_points=trunc(max_points,2) AND max_points BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE configured_sales ADD CONSTRAINT configured_sales_max_period_points_precision CHECK (max_period_points=trunc(max_period_points,2) AND max_period_points BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE configured_sales ADD CONSTRAINT configured_sales_max_user_period_points_precision CHECK (max_user_period_points=trunc(max_user_period_points,2) AND max_user_period_points BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE configured_orders ADD CONSTRAINT configured_orders_total_points_precision CHECK (total_points=trunc(total_points,2) AND total_points BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE brand_recharge_quotas ADD CONSTRAINT brand_recharge_quotas_available_precision CHECK (available=trunc(available,2) AND available BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE brand_recharge_quota_entries ADD CONSTRAINT brand_recharge_quota_entries_points_precision CHECK (points=trunc(points,2) AND points BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE brand_recharge_quota_entries ADD CONSTRAINT brand_recharge_quota_entries_before_available_precision CHECK (before_available=trunc(before_available,2) AND before_available BETWEEN -92233720368547758.08 AND 92233720368547758.07);
ALTER TABLE brand_recharge_quota_entries ADD CONSTRAINT brand_recharge_quota_entries_after_available_precision CHECK (after_available=trunc(after_available,2) AND after_available BETWEEN -92233720368547758.08 AND 92233720368547758.07);
DO $$ DECLARE f regprocedure; BEGIN FOR f IN SELECT p.oid::regprocedure FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname=current_schema() AND p.proname<>'guard_platform_game_period' LOOP EXECUTE format('ALTER FUNCTION %s SET search_path = pg_catalog, %I, pg_temp',f,current_schema()); END LOOP; END $$;

ALTER TABLE global_users ADD COLUMN wechat text, ADD COLUMN telegram text, ADD COLUMN whatsapp text;
