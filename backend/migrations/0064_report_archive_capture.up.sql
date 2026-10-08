CREATE FUNCTION report_archive_capture(b uuid, f timestamptz, t timestamptz, z text DEFAULT NULL)
RETURNS jsonb
LANGUAGE sql
STABLE
AS $$
WITH scope AS (
 SELECT id, CASE WHEN $4 IS NULL THEN timezone ELSE $4 END AS timezone
 FROM brands
 WHERE id=$1 AND ($4 IS NULL OR EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=$4))
),
-- Keep the betting facts and finality test aligned with reporting.Betting.
betting_facts AS (
 SELECT o.*,
  EXISTS(SELECT 1 FROM draw_corrections d WHERE d.brand_id=o.brand_id AND d.period_id=o.period_id AND d.state<>'completed') AS correction_open,
  (o.status IN ('won','lost') AND p.status='settled' AND j.state='completed' AND c.id IS NOT NULL) AS finalized
 FROM bet_orders o JOIN scope s ON s.id=o.brand_id
 JOIN games g ON g.brand_id=o.brand_id AND g.id=o.game_id
 JOIN periods p ON p.brand_id=o.brand_id AND p.id=o.period_id
 LEFT JOIN settlement_jobs j ON j.brand_id=o.brand_id AND j.id=p.current_settlement_job_id
 LEFT JOIN settlement_calculations c ON c.brand_id=o.brand_id AND c.id=o.settlement_calculation_id AND c.job_id=j.id AND c.order_id=o.id
 WHERE o.placed_at >= $2 AND o.placed_at < $3
),
betting_base AS (
 SELECT betting_facts.*,coalesce(finalized,false) AND NOT correction_open AS final FROM betting_facts
),
betting_totals AS (
 SELECT jsonb_build_object(
  'order_count',count(*)::text,
  'stake_points',coalesce(sum(total_points::numeric),0)::text,
  'placed_count',count(*) FILTER(WHERE status='placed')::text,
  'won_count',count(*) FILTER(WHERE status='won')::text,
  'lost_count',count(*) FILTER(WHERE status='lost')::text,
  'abnormal_count',count(*) FILTER(WHERE status='abnormal')::text,
  'cancelled_count',count(*) FILTER(WHERE status IN ('bet_cancelled','judged_cancelled'))::text,
  'refund_points',coalesce(sum(total_points::numeric) FILTER(WHERE refund_entry_id IS NOT NULL),0)::text,
  'settled_stake_points',coalesce(sum(total_points::numeric) FILTER(WHERE final),0)::text,
  'unfinalized_stake_points',coalesce(sum(total_points::numeric) FILTER(WHERE NOT final AND status IN ('placed','won','lost')),0)::text,
  'abnormal_stake_points',coalesce(sum(total_points::numeric) FILTER(WHERE status='abnormal'),0)::text,
  'current_prize_points',coalesce(sum(prize_points::numeric) FILTER(WHERE final),0)::text,
  'correction_open_count',count(*) FILTER(WHERE correction_open)::text
 ) AS totals FROM betting_base
),
-- Ledger totals reduce the stored point delta snapshot exactly as the live
-- ledger report does; wallet balances below remain a separate current view.
ledger_base AS (
 SELECT l.entry_type,
  (SELECT sum(v.value::numeric) FROM jsonb_each(l.delta_snapshot) s CROSS JOIN LATERAL jsonb_each_text(s.value) v) AS net_points
 FROM point_ledger_entries l JOIN scope s ON s.id=l.brand_id
 WHERE l.created_at >= $2 AND l.created_at < $3
),
ledger_totals AS (
 SELECT jsonb_build_object(
  'entry_count',count(*)::text,
  'net_points',coalesce(sum(net_points),0)::text,
  'recharge_points',coalesce(sum(greatest(net_points,0)) FILTER(WHERE entry_type='recharge'),0)::text,
  'prize_credit_points',coalesce(sum(greatest(net_points,0)) FILTER(WHERE entry_type='prize'),0)::text,
  'prize_reversal_points',coalesce(sum(greatest(-net_points,0)) FILTER(WHERE entry_type='prize_reversal'),0)::text,
  'refund_points',coalesce(sum(greatest(net_points,0)) FILTER(WHERE entry_type='refund'),0)::text
 ) AS totals FROM ledger_base
),
-- Current balances are restricted through this brand's point accounts.
accounts AS (SELECT id FROM point_accounts WHERE brand_id=$1),
wallet_totals AS (
 SELECT jsonb_build_object(
  'account_count',(SELECT count(*)::text FROM accounts),
  'available_points',coalesce(sum(points::numeric) FILTER(WHERE state='available'),0)::text,
  'frozen_points',coalesce(sum(points::numeric) FILTER(WHERE state IN ('manual_frozen','system_frozen')),0)::text,
  'withdrawal_points',coalesce(sum(points::numeric) FILTER(WHERE state='withdrawal'),0)::text,
  'total_points',coalesce(sum(points::numeric),0)::text
 ) AS totals FROM point_buckets WHERE brand_id=$1 AND account_id IN(SELECT id FROM accounts)
),
-- Withdrawals are a creation-time cohort with current order states.
withdrawal_base AS (
 SELECT o.* FROM withdrawal_orders o JOIN scope s ON s.id=o.brand_id
 WHERE o.created_at >= $2 AND o.created_at < $3
),
withdrawal_totals AS (
 SELECT jsonb_build_object(
  'order_count',count(*)::text,
  'requested_points',coalesce(sum(points::numeric),0)::text,
  'reviewing_count',count(*) FILTER(WHERE state='reviewing')::text,
  'reviewing_points',coalesce(sum(points::numeric) FILTER(WHERE state='reviewing'),0)::text,
  'processing_count',count(*) FILTER(WHERE state='processing')::text,
  'processing_points',coalesce(sum(points::numeric) FILTER(WHERE state='processing'),0)::text,
  'paid_count',count(*) FILTER(WHERE state='paid')::text,
  'paid_points',coalesce(sum(points::numeric) FILTER(WHERE state='paid'),0)::text,
  'rejected_count',count(*) FILTER(WHERE state='rejected')::text,
  'rejected_points',coalesce(sum(points::numeric) FILTER(WHERE state='rejected'),0)::text,
  'failed_count',count(*) FILTER(WHERE state='failed')::text,
  'failed_points',coalesce(sum(points::numeric) FILTER(WHERE state='failed'),0)::text,
  'cancelled_count',count(*) FILTER(WHERE state='cancelled')::text,
  'cancelled_points',coalesce(sum(points::numeric) FILTER(WHERE state='cancelled'),0)::text
 ) AS totals FROM withdrawal_base
),
-- Bind every commission amount to its unique immutable posting. Keep paid,
-- adjustment and applied correction legs separate to avoid multiplication.
commission_facts AS (
 SELECT l.id,l.created_at AS posted_at,t.agent_id,t.member_id,p.cycle_id,'paid'::text AS kind,
  (l.delta_snapshot->'commission'->>'available')::numeric AS delta_points
 FROM point_ledger_entries l JOIN commission_payment_targets t ON t.brand_id=l.brand_id AND t.ledger_entry_id=l.id
 JOIN commission_payments p ON p.brand_id=t.brand_id AND p.id=t.payment_id
 WHERE l.brand_id=$1 AND l.created_at >= $2 AND l.created_at < $3 AND t.state='paid'
  AND l.member_id=t.member_id AND l.entry_type='commission' AND l.reference_type='commission_payment_target' AND l.reference_id=t.id
 UNION ALL
 SELECT l.id,l.created_at,t.agent_id,t.member_id,p.cycle_id,'adjustment'::text,
  (l.delta_snapshot->'commission'->>'available')::numeric
 FROM point_ledger_entries l JOIN commission_adjustments a ON a.brand_id=l.brand_id AND a.ledger_entry_id=l.id
 JOIN commission_payment_targets t ON t.brand_id=a.brand_id AND t.id=a.target_id
 JOIN commission_payments p ON p.brand_id=a.brand_id AND p.id=a.payment_id
 WHERE l.brand_id=$1 AND l.created_at >= $2 AND l.created_at < $3
  AND l.member_id=t.member_id AND l.entry_type='commission_adjustment' AND l.reference_type='commission_adjustment' AND l.reference_id=a.id
 UNION ALL
 SELECT l.id,l.created_at,t.agent_id,t.member_id,x.cycle_id,'correction'::text,t.delta_points::numeric
 FROM commission_correction_execution_targets t
 JOIN commission_correction_executions x ON x.brand_id=t.brand_id AND x.cycle_id=t.cycle_id AND x.id=t.execution_id
 JOIN point_ledger_entries l ON l.brand_id=t.brand_id AND l.id=t.ledger_entry_id
 WHERE t.brand_id=$1 AND t.state='applied' AND t.delta_points<>0
  AND l.created_at >= $2 AND l.created_at < $3 AND l.member_id=t.member_id
  AND l.entry_type='commission_correction' AND l.reference_type='commission_correction_target' AND l.reference_id=t.id
  AND (l.delta_snapshot->'commission'->>'available')::numeric=t.delta_points::numeric
),
commission_totals AS (
 SELECT jsonb_build_object(
  'entry_count',count(*)::text,
  'paid_entry_count',count(*) FILTER(WHERE kind='paid')::text,
  'paid_points',coalesce(sum(delta_points) FILTER(WHERE kind='paid'),0)::text,
  'adjustment_entry_count',count(*) FILTER(WHERE kind='adjustment')::text,
  'adjustment_credit_points',coalesce(sum(greatest(delta_points,0)) FILTER(WHERE kind='adjustment'),0)::text,
  'adjustment_debit_points',coalesce(sum(greatest(-delta_points,0)) FILTER(WHERE kind='adjustment'),0)::text,
  'correction_entry_count',count(*) FILTER(WHERE kind='correction')::text,
  'correction_credit_points',coalesce(sum(greatest(delta_points,0)) FILTER(WHERE kind='correction'),0)::text,
  'correction_debit_points',coalesce(sum(greatest(-delta_points,0)) FILTER(WHERE kind='correction'),0)::text,
  'net_points',coalesce(sum(delta_points),0)::text
 ) AS totals FROM commission_facts
),
-- These audit and action witnesses intentionally match reporting.Reward.
-- A later revoke does not erase a valid grant; a reversal also remains
-- reportable in a window that excludes the original grant posting.
reward_facts AS (
 SELECT l.id,l.created_at AS posted_at,o.member_id,o.id AS order_id,'grant'::text AS kind,
  o.points::numeric AS points,o.points::numeric AS signed_points
 FROM point_ledger_entries l
 JOIN reward_order_actions a ON a.brand_id=l.brand_id AND a.ledger_entry_id=l.id
 JOIN reward_orders o ON o.brand_id=a.brand_id AND o.id=a.order_id
 JOIN audit_logs witness ON witness.id=a.audit_log_id AND witness.brand_id=a.brand_id AND witness.actor_id=a.actor_id
 WHERE l.brand_id=$1 AND l.created_at >= $2 AND l.created_at < $3
  AND l.entry_type='reward_grant' AND l.reference_type='reward_order' AND l.reference_id=o.id
  AND o.grant_ledger_entry_id=l.id AND l.member_id=o.member_id AND l.actor_type='admin'
  AND l.operation_key='reward-grant:'||o.id::text AND l.reversal_of IS NULL
  AND a.operation='grant' AND a.version=1 AND a.state_after='granted' AND a.actor_id=l.actor_id AND a.reason=l.reason
  AND witness.actor_type='admin' AND witness.action='reward.order.grant' AND witness.resource_type='reward_order'
  AND witness.resource_id=o.id AND witness.reason=a.reason AND witness.request_id=l.request_id
  AND witness.after_json->>'action_id'=a.id::text AND witness.after_json->>'version'=a.version::text
  AND witness.before_json='null'::jsonb AND witness.after_json->>'state'=a.state_after
  AND witness.after_json->>'member_id'=o.member_id::text AND witness.after_json->>'points'=o.points::text
  AND l.delta_snapshot=jsonb_build_object('recharge',jsonb_build_object('available','0','manual_frozen','0','system_frozen','0','withdrawal','0'),
   'winning',jsonb_build_object('available','0','manual_frozen','0','system_frozen','0','withdrawal','0'),
   'gift',jsonb_build_object('available',o.points::text,'manual_frozen','0','system_frozen','0','withdrawal','0'),
   'commission',jsonb_build_object('available','0','manual_frozen','0','system_frozen','0','withdrawal','0'))
  AND l.source_allocation=jsonb_build_array(jsonb_build_object('source','gift','state','available','points',o.points::text))
 UNION ALL
 SELECT l.id,l.created_at,o.member_id,o.id,'reversal'::text,o.points::numeric,-o.points::numeric
 FROM point_ledger_entries l
 JOIN reward_order_actions a ON a.brand_id=l.brand_id AND a.ledger_entry_id=l.id
 JOIN reward_orders o ON o.brand_id=a.brand_id AND o.id=a.order_id
 JOIN audit_logs witness ON witness.id=a.audit_log_id AND witness.brand_id=a.brand_id AND witness.actor_id=a.actor_id
 WHERE l.brand_id=$1 AND l.created_at >= $2 AND l.created_at < $3
  AND l.entry_type='reward_reversal' AND l.reference_type='reward_order' AND l.reference_id=o.id
  AND o.state='revoked' AND o.revoke_ledger_entry_id=l.id AND l.member_id=o.member_id
  AND l.actor_type='admin' AND l.operation_key='reward-revoke:'||o.id::text AND l.reversal_of=o.grant_ledger_entry_id
  AND a.operation IN('revoke','retry') AND a.state_after='revoked' AND a.actor_id=l.actor_id AND a.reason=l.reason
  AND witness.actor_type='admin' AND witness.action='reward.order.'||a.operation AND witness.resource_type='reward_order'
  AND witness.resource_id=o.id AND witness.reason=a.reason AND witness.request_id=l.request_id
  AND witness.after_json->>'action_id'=a.id::text AND witness.after_json->>'version'=a.version::text
  AND witness.after_json->>'state'=a.state_after AND witness.before_json->>'version'=(a.version-1)::text
  AND witness.before_json->>'state'=a.state_before
  AND l.delta_snapshot=jsonb_build_object('recharge',jsonb_build_object('available','0','manual_frozen','0','system_frozen','0','withdrawal','0'),
   'winning',jsonb_build_object('available','0','manual_frozen','0','system_frozen','0','withdrawal','0'),
   'gift',jsonb_build_object('available','-'||o.points::text,'manual_frozen','0','system_frozen','0','withdrawal','0'),
   'commission',jsonb_build_object('available','0','manual_frozen','0','system_frozen','0','withdrawal','0'))
  AND l.source_allocation=jsonb_build_array(jsonb_build_object('source','gift','state','available','points',o.points::text))
),
reward_totals AS (
 SELECT jsonb_build_object(
  'entry_count',count(*)::text,
  'grant_entry_count',count(*) FILTER(WHERE kind='grant')::text,
  'grant_points',coalesce(sum(points) FILTER(WHERE kind='grant'),0)::text,
  'reversal_entry_count',count(*) FILTER(WHERE kind='reversal')::text,
  'reversal_points',coalesce(sum(points) FILTER(WHERE kind='reversal'),0)::text,
  'net_points',coalesce(sum(signed_points),0)::text
 ) AS totals FROM reward_facts
),
-- Reward orders are their current projection, selected by created_at cohort.
reward_order_base AS (
 SELECT o.id,o.member_id,o.points,o.state FROM reward_orders o JOIN scope s ON s.id=o.brand_id
 WHERE o.created_at >= $2 AND o.created_at < $3
),
reward_order_totals AS (
 SELECT jsonb_build_object(
  'order_count',count(*)::text,
  'original_points',coalesce(sum(points::numeric),0)::text,
  'granted_count',count(*) FILTER(WHERE state='granted')::text,
  'granted_points',coalesce(sum(points::numeric) FILTER(WHERE state='granted'),0)::text,
  'pending_count',count(*) FILTER(WHERE state='revocation_pending')::text,
  'pending_points',coalesce(sum(points::numeric) FILTER(WHERE state='revocation_pending'),0)::text,
  'revoked_count',count(*) FILTER(WHERE state='revoked')::text,
  'revoked_points',coalesce(sum(points::numeric) FILTER(WHERE state='revoked'),0)::text
 ) AS totals FROM reward_order_base
)
SELECT jsonb_build_object(
 'brand_id',scope.id::text,
 'format_version',1,
 'snapshot_at',statement_timestamp(),
 'timezone',scope.timezone,
 'from',$2,
 'to',$3,
 'betting',(SELECT totals FROM betting_totals),
 'ledger',(SELECT totals FROM ledger_totals),
 'wallet_snapshot',jsonb_build_object('at_snapshot',statement_timestamp(),'balances',(SELECT totals FROM wallet_totals)),
 'withdrawals',(SELECT totals FROM withdrawal_totals),
 'commissions',(SELECT totals FROM commission_totals),
 'rewards',(SELECT totals FROM reward_totals),
 'reward_orders',(SELECT totals FROM reward_order_totals)
)
FROM scope
$$;

DO $$
DECLARE app_schema text := current_schema();
BEGIN
 EXECUTE format('ALTER FUNCTION %I.report_archive_capture(uuid,timestamptz,timestamptz,text) SET search_path = pg_catalog, %I, pg_temp', app_schema, app_schema);
END $$;
