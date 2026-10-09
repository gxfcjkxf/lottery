-- Read-only analysis helpers and indexes; no financial/history backfill,
-- no permission grants, no policy or runtime gate changes.
CREATE INDEX commission_cycles_end_analysis ON commission_cycles(brand_id,window_to,id);
CREATE INDEX commission_analysis_ledger_family ON point_ledger_entries(brand_id,id)
 WHERE entry_type IN('commission','commission_adjustment','commission_correction')
 OR reference_type IN('commission_payment_target','commission_adjustment','commission_correction_target');
CREATE INDEX commission_analysis_ledger_binding ON point_ledger_entries(brand_id,reference_type,reference_id)
 WHERE reference_type IN('commission_payment_target','commission_adjustment','commission_correction_target');

CREATE FUNCTION commission_analysis_run_valid(b uuid,cid uuid,rid uuid) RETURNS boolean LANGUAGE plpgsql STABLE AS $$
BEGIN
 -- Revalidate immutable allocation arithmetic and complete earnings, rather
 -- than treating a ready flag or a sum of a partial table as known accrual.
 RETURN EXISTS(SELECT 1 FROM commission_cycles c WHERE c.brand_id=b AND c.id=cid AND
 c.target_count=(SELECT count(*) FROM commission_cycle_targets t WHERE t.brand_id=b AND t.cycle_id=cid)
 AND NOT EXISTS(SELECT 1 FROM bet_orders o WHERE o.brand_id=b AND o.placed_at>=c.window_from AND o.placed_at<c.window_to
  AND NOT EXISTS(SELECT 1 FROM commission_cycle_targets t WHERE t.brand_id=b AND t.cycle_id=cid AND t.order_id=o.id))
 AND NOT EXISTS(SELECT 1 FROM commission_cycle_targets t LEFT JOIN bet_orders o ON o.brand_id=t.brand_id AND o.id=t.order_id
  WHERE t.brand_id=b AND t.cycle_id=cid AND (o.id IS NULL OR t.placed_at IS DISTINCT FROM o.placed_at OR o.placed_at<c.window_from OR o.placed_at>=c.window_to)))
 AND NOT EXISTS(
  SELECT 1 FROM commission_calculations x
  LEFT JOIN bet_orders o ON o.brand_id=x.brand_id AND o.id=x.order_id
  LEFT JOIN point_ledger_entries l ON l.id=o.debit_entry_id
  LEFT JOIN point_ledger_entries pl ON pl.id=o.payout_entry_id
  WHERE x.brand_id=b AND x.cycle_id=cid AND x.run_id=rid AND (
   o.id IS NULL OR NOT EXISTS(SELECT 1 FROM audit_logs a WHERE a.id=x.audit_log_id AND a.brand_id=b AND a.action='commission.cycle.calculation_page' AND a.resource_type='commission_cycle' AND a.resource_id=cid)
   OR l.id IS NULL OR l.brand_id IS DISTINCT FROM b OR l.account_id IS DISTINCT FROM x.account_id OR l.member_id IS DISTINCT FROM x.member_id
   OR l.entry_type<>'bet' OR l.reference_type<>'bet_order' OR l.reference_id IS DISTINCT FROM x.order_id
   OR l.source_allocation IS DISTINCT FROM o.deduction_allocation
   OR (SELECT sum((v->>'points')::numeric) FROM jsonb_array_elements(o.deduction_allocation) v) IS DISTINCT FROM x.stake_points::numeric
   OR normalize_point_snapshot(l.delta_snapshot) IS DISTINCT FROM lottery_business_allocation_delta(o.deduction_allocation,true)
   OR NOT lottery_business_ledger_applied(l,b,x.account_id)
   OR x.prize_points>0 AND (pl.id IS NULL OR pl.brand_id IS DISTINCT FROM b OR pl.account_id IS DISTINCT FROM x.account_id OR pl.member_id IS DISTINCT FROM x.member_id
    OR pl.entry_type<>'prize' OR pl.reference_type<>'settlement_calculation' OR pl.reference_id IS DISTINCT FROM x.calculation_id
    OR pl.operation_key<>'settlement-payout:'||x.calculation_id::text
    OR normalize_point_snapshot(pl.delta_snapshot) IS DISTINCT FROM jsonb_set(point_zero_snapshot(),'{winning,available}',to_jsonb(x.prize_points::text))
    OR pl.source_allocation IS DISTINCT FROM jsonb_build_array(jsonb_build_object('source','winning','state','available','points',x.prize_points::text))
    OR NOT lottery_business_ledger_applied(pl,b,x.account_id))
   OR x.prize_points=0 AND o.payout_entry_id IS NOT NULL
   OR x.rule_snapshot->'schema_version' IS DISTINCT FROM '1'::jsonb
   OR x.rule_snapshot->>'brand_id' IS DISTINCT FROM b::text OR x.rule_snapshot->>'member_id' IS DISTINCT FROM x.member_id::text
   OR (x.rule_snapshot->>'captured_at')::timestamptz IS DISTINCT FROM o.placed_at
   OR NOT EXISTS(SELECT 1 FROM commission_policy_revisions r WHERE r.brand_id=b AND r.id=(x.rule_snapshot->'financial_policy'->>'revision_id')::uuid
    AND r.version::text=x.rule_snapshot->'financial_policy'->>'version' AND r.config=x.rule_snapshot->'financial_policy'->'config' AND r.created_at<=o.placed_at)
   OR NOT EXISTS(SELECT 1 FROM agent_config_revisions r WHERE r.brand_id=b AND r.agent_id IS NULL AND r.id=(x.rule_snapshot->'agency_policy'->>'revision_id')::uuid
    AND r.version::text=x.rule_snapshot->'agency_policy'->>'version' AND r.config=x.rule_snapshot->'agency_policy'->'config' AND r.created_at<=o.placed_at)
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
    OR x.base_points IS DISTINCT FROM CASE WHEN x.rule_snapshot->'agent_path'->0->>'effective_mode'='turnover' OR x.status='lost' THEN x.stake_points ELSE 0 END
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
   OR e.points::numeric IS DISTINCT FROM floor((s.micro*2+1000000)/2000000));
EXCEPTION WHEN OTHERS THEN RETURN false;
END $$;
DO $$ DECLARE app_schema text:=current_schema(); BEGIN
 EXECUTE format('ALTER FUNCTION commission_analysis_run_valid(uuid,uuid,uuid) SET search_path TO pg_catalog, %I, pg_temp',app_schema);
END $$;
