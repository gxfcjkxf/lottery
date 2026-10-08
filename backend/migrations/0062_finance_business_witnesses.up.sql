CREATE FUNCTION point_finance_business_witnesses(b uuid,a uuid)
RETURNS TABLE(family text,source_id uuid,ledger_id uuid,source_valid boolean,expected_entry_type text)
LANGUAGE plpgsql STABLE
AS $function$
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
   LEFT JOIN reward_order_actions r ON r.brand_id=o.brand_id AND r.order_id=o.id AND r.operation IN('revoke','retry') AND r.state_after='revoked'
  WHERE o.brand_id=b AND p.id=a AND o.points<>0 AND o.state='revoked'
 LOOP
  expected:=zero;
  allocation_valid:=true;
  IF edge.family='withdrawal' THEN
   allocation_valid:=false;
   IF jsonb_typeof(edge.allocation)='array' THEN
    allocation_valid:=jsonb_array_length(edge.allocation) BETWEEN 1 AND 4
     AND (SELECT coalesce(sum(CASE WHEN v->>'points' ~ '^[1-9][0-9]*$' THEN (v->>'points')::numeric ELSE 0 END),0)
      FROM jsonb_array_elements(edge.allocation) v)=edge.amount
     AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements(edge.allocation) v WHERE jsonb_typeof(v)<>'object'
      OR (SELECT count(*) FROM jsonb_object_keys(v))<>3 OR NOT(v ?& ARRAY['source','state','points'])
      OR v->>'source' NOT IN('recharge','winning','gift','commission')
      OR v->>'state' IS DISTINCT FROM CASE WHEN edge.kind='paid' THEN 'withdrawal' ELSE 'available' END
      OR jsonb_typeof(v->'points')<>'string' OR v->>'points' !~ '^[1-9][0-9]*$'
      OR CASE WHEN v->>'points' ~ '^[1-9][0-9]*$' THEN (v->>'points')::numeric>9223372036854775807 ELSE false END)
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
     AND q.id=edge.audit_id AND r.operation IN('revoke','retry') AND r.state_after='revoked'
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
    AND l.operation_key=edge.operation_key AND normalize_point_snapshot(l.delta_snapshot)=expected AND l.source_allocation=edge.allocation
    AND l.reversal_of IS NOT DISTINCT FROM edge.reversal_of
    AND EXISTS(SELECT 1 FROM audit_logs q WHERE q.id=edge.audit_id AND l.actor_type=q.actor_type
     AND l.actor_id IS NOT DISTINCT FROM q.actor_id AND l.request_id=q.request_id
     AND (edge.family IN('commission','commission_correction') OR l.reason=q.reason));
   IF edge.family IN('commission','commission_correction') THEN
    valid:=valid AND l.actor_type='system' AND l.actor_id IS NULL AND l.reason IS NOT NULL;
   ELSIF edge.family IN('commission_adjustment','reward') THEN
    valid:=valid AND l.actor_type='admin' AND l.actor_id IS NOT NULL;
   END IF;
   valid:=valid AND valid_point_snapshot(l.before_snapshot,false,true) IS TRUE
    AND valid_point_snapshot(l.delta_snapshot,true,true) IS TRUE
    AND valid_point_snapshot(l.after_snapshot,false,true) IS TRUE;
   before_value:=normalize_point_snapshot(l.before_snapshot);
   delta_value:=normalize_point_snapshot(l.delta_snapshot);
   after_value:=normalize_point_snapshot(l.after_snapshot);
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
$function$;

DO $$
DECLARE app_schema text:=current_schema();
BEGIN
 IF app_schema IS NULL THEN RAISE EXCEPTION 'application schema required'; END IF;
 EXECUTE format('ALTER FUNCTION %I.point_finance_business_witnesses(uuid,uuid) SET search_path = pg_catalog, %I, pg_temp',app_schema,app_schema);
END $$;
