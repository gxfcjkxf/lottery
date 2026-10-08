-- Read-only reconciliation edges from saved lottery/recharge business state
-- to the immutable wallet ledger. Historical point history is compared after
-- exact legacy-12-bucket normalization; no business or ledger rows are changed.

CREATE FUNCTION lottery_business_allocation_delta(v jsonb, negative boolean)
RETURNS jsonb LANGUAGE plpgsql IMMUTABLE AS $$
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
   jsonb_typeof(item->'points') IS DISTINCT FROM 'string' OR item->>'points' !~ '^[1-9][0-9]*$' THEN RETURN NULL; END IF;
  amount:=(item->>'points')::numeric;
  IF amount>9223372036854775807 THEN RETURN NULL; END IF;
  seen:=array_append(seen,src);
  expected:=jsonb_set(expected,ARRAY[src,st],to_jsonb((CASE WHEN negative THEN -amount ELSE amount END)::text),false);
 END LOOP;
 RETURN expected;
EXCEPTION WHEN OTHERS THEN RETURN NULL;
END $$;

CREATE FUNCTION lottery_business_negate_snapshot(v jsonb)
RETURNS jsonb LANGUAGE plpgsql IMMUTABLE AS $$
DECLARE expected jsonb:=normalize_point_snapshot(v); src text; st text; amount numeric;
BEGIN
 IF expected IS NULL THEN RETURN NULL; END IF;
 FOREACH src IN ARRAY ARRAY['recharge','winning','gift','commission'] LOOP
  FOREACH st IN ARRAY ARRAY['available','manual_frozen','system_frozen','withdrawal'] LOOP
   amount:=-(expected->src->>st)::numeric;
   IF amount < -9223372036854775808 OR amount > 9223372036854775807 THEN RETURN NULL; END IF;
   expected:=jsonb_set(expected,ARRAY[src,st],to_jsonb(amount::text),false);
  END LOOP;
 END LOOP;
 RETURN expected;
EXCEPTION WHEN OTHERS THEN RETURN NULL;
END $$;

CREATE FUNCTION lottery_business_ledger_applied(l point_ledger_entries,b uuid,a uuid)
RETURNS boolean LANGUAGE plpgsql STABLE AS $$
DECLARE before_value jsonb; delta_value jsonb; after_value jsonb; src text; st text;
BEGIN
 IF l.id IS NULL OR l.brand_id IS DISTINCT FROM b OR l.account_id IS DISTINCT FROM a OR
  l.version IS NULL OR l.version<1 OR
  valid_point_snapshot(l.before_snapshot,false,true) IS NOT TRUE OR
  valid_point_snapshot(l.delta_snapshot,true,true) IS NOT TRUE OR
  valid_point_snapshot(l.after_snapshot,false,true) IS NOT TRUE OR
  NOT EXISTS(SELECT 1 FROM point_accounts pa WHERE pa.brand_id=b AND pa.id=a AND pa.brand_member_id=l.member_id AND pa.version>=l.version) THEN
  RETURN false;
 END IF;
 before_value:=normalize_point_snapshot(l.before_snapshot);
 delta_value:=normalize_point_snapshot(l.delta_snapshot);
 after_value:=normalize_point_snapshot(l.after_snapshot);
 IF before_value IS NULL OR delta_value IS NULL OR after_value IS NULL THEN RETURN false; END IF;
 FOREACH src IN ARRAY ARRAY['recharge','winning','gift','commission'] LOOP
  FOREACH st IN ARRAY ARRAY['available','manual_frozen','system_frozen','withdrawal'] LOOP
   IF (before_value->src->>st)::numeric+(delta_value->src->>st)::numeric IS DISTINCT FROM (after_value->src->>st)::numeric THEN
    RETURN false;
   END IF;
  END LOOP;
 END LOOP;
 IF l.version=1 THEN
  IF before_value IS DISTINCT FROM point_zero_snapshot() THEN RETURN false; END IF;
 ELSIF NOT EXISTS(SELECT 1 FROM point_ledger_entries prev WHERE prev.brand_id=b AND prev.account_id=a AND prev.version=l.version-1 AND
  normalize_point_snapshot(prev.after_snapshot)=before_value) THEN
  RETURN false;
 END IF;
 RETURN COALESCE(EXISTS(
  SELECT 1 FROM audit_logs x WHERE x.brand_id=b AND x.actor_type=l.actor_type AND x.actor_id IS NOT DISTINCT FROM l.actor_id AND
   x.action='points.'||l.entry_type AND x.resource_type='point_account' AND x.resource_id=a AND x.request_id=l.request_id AND
   x.before_json=l.before_snapshot AND x.after_json->>'ledger_entry_id'=l.id::text AND x.after_json->'balance'=l.after_snapshot
 ),false);
EXCEPTION WHEN OTHERS THEN RETURN false;
END $$;

CREATE FUNCTION point_lottery_business_witnesses(b uuid,a uuid)
RETURNS TABLE(family text,source_id uuid,ledger_id uuid,source_valid boolean,expected_entry_type text)
LANGUAGE sql STABLE AS $$
 WITH recharge_edges AS (
  SELECT 'recharge'::text AS family,r.id AS source_id,r.ledger_entry_id AS ledger_id,'recharge'::text AS expected_entry_type,
   COALESCE(r.state='confirmed' AND r.confirmed_at IS NOT NULL AND r.confirmed_by IS NOT NULL AND
    pa.brand_member_id=r.member_id AND l.id IS NOT NULL AND l.brand_id=r.brand_id AND l.account_id=r.account_id AND l.member_id=r.member_id AND
    l.entry_type='recharge' AND l.reference_type='recharge' AND l.reference_id=r.id AND l.operation_key='recharge-confirm:'||r.id::text AND
    l.actor_type='admin' AND l.actor_id=r.confirmed_by AND l.reason<>'' AND l.reversal_of IS NULL AND
    l.source_allocation=jsonb_build_array(jsonb_build_object('source','recharge','state','available','points',r.points::text)) AND
    normalize_point_snapshot(l.delta_snapshot)=lottery_business_allocation_delta(l.source_allocation,false) AND
    lottery_business_ledger_applied(l,b,a) AND
    (SELECT count(*)=1 FROM point_ledger_entries x WHERE x.brand_id=b AND x.entry_type='recharge' AND x.reference_type='recharge' AND x.reference_id=r.id AND x.id=r.ledger_entry_id) AND
    EXISTS(SELECT 1 FROM audit_logs x WHERE x.brand_id=b AND x.actor_type='admin' AND x.actor_id=r.confirmed_by AND x.action='finance.recharge.confirm' AND
     x.resource_type='recharge_order' AND x.resource_id=r.id AND x.request_id=l.request_id AND x.reason=l.reason AND x.after_json->>'id'=r.id::text AND
     x.after_json->>'brand_id'=r.brand_id::text AND x.after_json->>'member_id'=r.member_id::text AND x.after_json->>'account_id'=r.account_id::text AND
     x.after_json->>'confirmed_by'=r.confirmed_by::text AND x.after_json->>'state'='confirmed' AND x.after_json->>'points'=r.points::text AND
     x.after_json->>'ledger_entry_id'=l.id::text),false) AS source_valid
  FROM recharge_orders r
  LEFT JOIN point_accounts pa ON pa.brand_id=r.brand_id AND pa.id=r.account_id
  LEFT JOIN point_ledger_entries l ON l.id=r.ledger_entry_id
  WHERE r.brand_id=b AND r.account_id=a AND r.state='confirmed'
 ), bet_edges AS (
  SELECT 'bet'::text AS family,o.id AS source_id,o.debit_entry_id AS ledger_id,'bet'::text AS expected_entry_type,
   COALESCE(pa.brand_member_id=o.brand_member_id AND l.id IS NOT NULL AND l.brand_id=o.brand_id AND l.account_id=o.account_id AND l.member_id=o.brand_member_id AND
    l.entry_type='bet' AND l.reference_type='bet_order' AND l.reference_id=o.id AND l.operation_key='bet:'||o.id::text AND
    l.actor_type='user' AND l.actor_id=o.global_user_id AND l.reason='user confirmed lottery stake' AND l.reversal_of IS NULL AND
    l.source_allocation=o.deduction_allocation AND
    normalize_point_snapshot(l.delta_snapshot)=lottery_business_allocation_delta(o.deduction_allocation,true) AND
    CASE WHEN lottery_business_allocation_delta(o.deduction_allocation,true) IS NOT NULL THEN
     (SELECT sum((x->>'points')::numeric) FROM jsonb_array_elements(o.deduction_allocation) x)
     ELSE NULL END=o.total_points AND
    lottery_business_ledger_applied(l,b,a) AND
    (SELECT count(*)=1 FROM point_ledger_entries x WHERE x.brand_id=b AND x.entry_type='bet' AND x.reference_type='bet_order' AND x.reference_id=o.id AND x.id=o.debit_entry_id) AND
    EXISTS(SELECT 1 FROM audit_logs x WHERE x.brand_id=b AND x.actor_type='user' AND x.actor_id=o.global_user_id AND x.action='bet.place' AND
     x.resource_type='bet_order' AND x.resource_id=o.id AND x.request_id=l.request_id AND x.after_json->>'order_id'=o.id::text AND
     x.after_json->>'points'=o.total_points::text AND x.after_json->>'debit_entry_id'=l.id::text),false) AS source_valid
  FROM bet_orders o
  LEFT JOIN point_accounts pa ON pa.brand_id=o.brand_id AND pa.id=o.account_id
  LEFT JOIN point_ledger_entries l ON l.id=o.debit_entry_id
  WHERE o.brand_id=b AND o.account_id=a
 ), refund_edges AS (
  SELECT 'refund'::text AS family,o.id AS source_id,o.refund_entry_id AS ledger_id,'refund'::text AS expected_entry_type,
   COALESCE(o.status IN('bet_cancelled','judged_cancelled') AND o.refund_entry_id IS NOT NULL AND pa.brand_member_id=o.brand_member_id AND
    debit.id IS NOT NULL AND l.id IS NOT NULL AND l.brand_id=o.brand_id AND l.account_id=o.account_id AND l.member_id=o.brand_member_id AND
    l.entry_type='refund' AND l.reference_type='bet_order' AND l.reference_id=o.id AND l.operation_key='bet-refund:'||o.id::text AND
    l.actor_type IN('user','admin') AND (l.actor_type<>'user' OR l.actor_id=o.global_user_id) AND l.reason=o.cancel_reason AND
    l.reversal_of=o.debit_entry_id AND l.source_allocation=o.deduction_allocation AND l.source_allocation=debit.source_allocation AND
    normalize_point_snapshot(l.delta_snapshot)=lottery_business_negate_snapshot(debit.delta_snapshot) AND lottery_business_ledger_applied(l,b,a) AND
    (SELECT count(*)=1 FROM point_ledger_entries x WHERE x.brand_id=b AND x.entry_type='refund' AND x.reference_type='bet_order' AND x.reference_id=o.id AND x.id=o.refund_entry_id) AND
    EXISTS(SELECT 1 FROM audit_logs x WHERE x.brand_id=b AND x.actor_type=l.actor_type AND x.actor_id IS NOT DISTINCT FROM l.actor_id AND
     x.action IN('bet.cancel','bet.judge_cancel') AND x.resource_type='bet_order' AND x.resource_id=o.id AND x.request_id=l.request_id AND
     x.after_json->>'status'=o.status AND x.after_json->>'refund_entry_id'=l.id::text) AND
    NOT EXISTS(SELECT 1 FROM period_cancellation_targets t LEFT JOIN period_cancellations c ON c.brand_id=t.brand_id AND c.period_id=t.period_id AND c.id=t.cancellation_id
     WHERE t.brand_id=o.brand_id AND t.period_id=o.period_id AND t.order_id=o.id AND
      (c.id IS NULL OR c.period_version<2 OR t.version<2 OR t.state NOT IN('refunded','already_refunded') OR
       t.refund_entry_id IS DISTINCT FROM o.refund_entry_id)),false) AS source_valid
  FROM bet_orders o
  LEFT JOIN point_accounts pa ON pa.brand_id=o.brand_id AND pa.id=o.account_id
  LEFT JOIN point_ledger_entries l ON l.id=o.refund_entry_id
  LEFT JOIN point_ledger_entries debit ON debit.id=o.debit_entry_id
  WHERE o.brand_id=b AND o.account_id=a AND (o.status IN('bet_cancelled','judged_cancelled') OR o.refund_entry_id IS NOT NULL)
 ), prize_edges AS (
  SELECT 'prize'::text AS family,coalesce(c.id,t.order_id) AS source_id,t.payout_entry_id AS ledger_id,'prize'::text AS expected_entry_type,
   COALESCE(t.state='paid' AND c.prize_points>0 AND c.won AND j.id IS NOT NULL AND j.brand_id=t.brand_id AND j.period_id=t.period_id AND
    c.brand_id=t.brand_id AND c.job_id=t.job_id AND c.period_id=t.period_id AND c.order_id=t.order_id AND c.id=t.calculation_id AND
    c.definition_hash=o.definition_hash AND c.order_version>0 AND j.game_id=o.game_id AND
    o.brand_id=t.brand_id AND o.id=t.order_id AND o.period_id=t.period_id AND pa.brand_member_id=o.brand_member_id AND
    l.id IS NOT NULL AND l.brand_id=t.brand_id AND l.account_id=o.account_id AND l.member_id=o.brand_member_id AND
    l.entry_type='prize' AND l.reference_type='settlement_calculation' AND l.reference_id=c.id AND l.operation_key='settlement-payout:'||c.id::text AND
    l.actor_type='system' AND l.actor_id IS NULL AND l.reason='lottery winnings' AND l.request_id='settlement:'||j.id::text AND l.reversal_of IS NULL AND
    l.source_allocation=jsonb_build_array(jsonb_build_object('source','winning','state','available','points',c.prize_points::text)) AND
    normalize_point_snapshot(l.delta_snapshot)=lottery_business_allocation_delta(l.source_allocation,false) AND lottery_business_ledger_applied(l,b,a) AND
    (SELECT count(*)=1 FROM point_ledger_entries x WHERE x.brand_id=b AND x.entry_type='prize' AND x.reference_type='settlement_calculation' AND x.reference_id=c.id AND x.id=t.payout_entry_id),false) AS source_valid
  FROM settlement_targets t
  LEFT JOIN settlement_calculations c ON c.brand_id=t.brand_id AND c.job_id=t.job_id AND c.period_id=t.period_id AND c.order_id=t.order_id AND c.id=t.calculation_id
  LEFT JOIN settlement_jobs j ON j.brand_id=t.brand_id AND j.period_id=t.period_id AND j.id=t.job_id
  JOIN bet_orders o ON o.brand_id=t.brand_id AND o.period_id=t.period_id AND o.id=t.order_id
  LEFT JOIN point_accounts pa ON pa.brand_id=o.brand_id AND pa.id=o.account_id
  LEFT JOIN point_ledger_entries l ON l.id=t.payout_entry_id
  WHERE t.brand_id=b AND o.account_id=a AND t.state='paid' AND (c.id IS NULL OR c.prize_points>0 OR t.payout_entry_id IS NOT NULL)
 ), reversal_edges AS (
  SELECT 'prize_reversal'::text AS family,t.order_id AS source_id,t.reversal_entry_id AS ledger_id,'prize_reversal'::text AS expected_entry_type,
   COALESCE(t.state='reversed' AND t.old_prize_points>0 AND dc.id IS NOT NULL AND dc.brand_id=t.brand_id AND dc.period_id=t.period_id AND
    c.id=t.old_calculation_id AND c.brand_id=t.brand_id AND c.period_id=t.period_id AND c.order_id=t.order_id AND c.prize_points=t.old_prize_points AND c.won AND
    old_target.state='paid' AND old_target.calculation_id=t.old_calculation_id AND old_target.payout_entry_id=t.old_payout_entry_id AND
    original.id IS NOT NULL AND original.entry_type='prize' AND original.reference_type='settlement_calculation' AND original.reference_id=c.id AND
    original.operation_key='settlement-payout:'||c.id::text AND original.actor_type='system' AND original.actor_id IS NULL AND
    original.source_allocation=jsonb_build_array(jsonb_build_object('source','winning','state','available','points',c.prize_points::text)) AND
    normalize_point_snapshot(original.delta_snapshot)=lottery_business_allocation_delta(original.source_allocation,false) AND
    lottery_business_ledger_applied(original,b,a) AND
    l.id IS NOT NULL AND lottery_business_ledger_applied(l,b,a) AND normalize_point_snapshot(l.delta_snapshot)=lottery_business_negate_snapshot(original.delta_snapshot) AND
    l.source_allocation=original.source_allocation AND correction_reversal_is_applied(t) IS TRUE AND
    l.reference_id=dc.id AND l.reference_type='draw_correction' AND l.operation_key='draw-correction:'||dc.id::text||':'||t.order_id::text AND
    l.actor_type='system' AND l.actor_id IS NULL AND l.request_id='correction:'||dc.id::text AND l.reason='corrected lottery result reverses original prize' AND
    (SELECT count(*)=1 FROM point_ledger_entries x WHERE x.brand_id=b AND x.operation_key=l.operation_key AND x.id=t.reversal_entry_id),false) AS source_valid
  FROM draw_correction_targets t
  LEFT JOIN draw_corrections dc ON dc.brand_id=t.brand_id AND dc.period_id=t.period_id AND dc.id=t.correction_id
  JOIN bet_orders o ON o.brand_id=t.brand_id AND o.period_id=t.period_id AND o.id=t.order_id
  LEFT JOIN settlement_calculations c ON c.brand_id=t.brand_id AND c.period_id=t.period_id AND c.order_id=t.order_id AND c.id=t.old_calculation_id
  LEFT JOIN settlement_targets old_target ON old_target.brand_id=t.brand_id AND old_target.period_id=t.period_id AND old_target.order_id=t.order_id AND old_target.calculation_id=t.old_calculation_id
  LEFT JOIN point_ledger_entries original ON original.brand_id=t.brand_id AND original.id=t.old_payout_entry_id
  LEFT JOIN point_ledger_entries l ON l.id=t.reversal_entry_id
  WHERE t.brand_id=b AND o.account_id=a AND t.old_prize_points>0 AND (t.state='reversed' OR t.reversal_entry_id IS NOT NULL)
 )
 SELECT family,source_id,ledger_id,COALESCE(source_valid,false),expected_entry_type FROM recharge_edges
 UNION ALL SELECT family,source_id,ledger_id,COALESCE(source_valid,false),expected_entry_type FROM bet_edges
 UNION ALL SELECT family,source_id,ledger_id,COALESCE(source_valid,false),expected_entry_type FROM refund_edges
 UNION ALL SELECT family,source_id,ledger_id,COALESCE(source_valid,false),expected_entry_type FROM prize_edges
 UNION ALL SELECT family,source_id,ledger_id,COALESCE(source_valid,false),expected_entry_type FROM reversal_edges
$$;

-- Resolve the application schema once, and keep every helper immune to a
-- caller-controlled search_path (while still permitting pg_temp last).
DO $$
DECLARE app_schema text:=current_schema();
BEGIN
 IF app_schema IS NULL THEN RAISE EXCEPTION 'application schema required'; END IF;
 EXECUTE format('ALTER FUNCTION %I.lottery_business_allocation_delta(jsonb,boolean) SET search_path = pg_catalog, %I, pg_temp',app_schema,app_schema);
 EXECUTE format('ALTER FUNCTION %I.lottery_business_negate_snapshot(jsonb) SET search_path = pg_catalog, %I, pg_temp',app_schema,app_schema);
 EXECUTE format('ALTER FUNCTION %I.lottery_business_ledger_applied(%I.point_ledger_entries,uuid,uuid) SET search_path = pg_catalog, %I, pg_temp',app_schema,app_schema,app_schema);
 EXECUTE format('ALTER FUNCTION %I.point_lottery_business_witnesses(uuid,uuid) SET search_path = pg_catalog, %I, pg_temp',app_schema,app_schema);
END $$;
