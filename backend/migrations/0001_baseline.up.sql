-- Current schema for fresh installations only. No upgrade or data backfill.


SET LOCAL statement_timeout = 0;
SET LOCAL lock_timeout = 0;
SET LOCAL idle_in_transaction_session_timeout = 0;
SET LOCAL transaction_timeout = 0;
SET LOCAL client_encoding = 'UTF8';
SET LOCAL standard_conforming_strings = on;

SET LOCAL check_function_bodies = false;
SET LOCAL xmloption = content;
SET LOCAL client_min_messages = warning;
SET LOCAL row_security = off;

CREATE FUNCTION advance_commission_evidence_epoch() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF TG_TABLE_NAME='periods' THEN
  IF (OLD.status='settled' OR NEW.status='settled') AND
   (NEW.status,NEW.draw_result_id,NEW.current_settlement_job_id) IS DISTINCT FROM
   (OLD.status,OLD.draw_result_id,OLD.current_settlement_job_id) THEN
   UPDATE commission_cycles c SET evidence_epoch=evidence_epoch+1 WHERE c.brand_id=NEW.brand_id
    AND c.window_from<NEW.bet_end_at AND c.window_to>NEW.bet_start_at
    AND EXISTS(SELECT 1 FROM bet_orders o WHERE o.brand_id=NEW.brand_id AND o.period_id=NEW.id AND o.placed_at>=c.window_from AND o.placed_at<c.window_to);
  END IF;
 ELSE
  IF (NEW.status,NEW.prize_points,NEW.settlement_calculation_id,NEW.payout_entry_id,NEW.refund_entry_id) IS DISTINCT FROM
   (OLD.status,OLD.prize_points,OLD.settlement_calculation_id,OLD.payout_entry_id,OLD.refund_entry_id) AND
   (OLD.status='abnormal' OR EXISTS(SELECT 1 FROM periods WHERE brand_id=NEW.brand_id AND id=NEW.period_id AND status='settled')) THEN
   UPDATE commission_cycles c SET evidence_epoch=evidence_epoch+1 WHERE c.brand_id=NEW.brand_id AND c.window_from<=NEW.placed_at AND c.window_to>NEW.placed_at;
  END IF;
 END IF;
 RETURN NULL;
END $$;

CREATE FUNCTION agent_effective_mode_for_path(p_brand uuid, p_path uuid[], p_override_id uuid, p_override_config jsonb, p_policy_mode text) RETURNS text
    LANGUAGE sql STABLE
    AS $$
 SELECT coalesce((
  SELECT CASE WHEN a.id=p_override_id THEN p_override_config->>'mode' ELSE a.config->>'mode' END
  FROM unnest(p_path) WITH ORDINALITY AS part(id,depth)
  JOIN agent_nodes a ON a.brand_id=p_brand AND a.id=part.id
  WHERE CASE WHEN a.id=p_override_id THEN p_override_config->'mode' ELSE a.config->'mode' END<>'null'::jsonb
  ORDER BY part.depth DESC LIMIT 1
 ),p_policy_mode)
$$;

CREATE FUNCTION agent_revision_committed() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE v bigint;
BEGIN
 IF NEW.agent_id IS NULL THEN SELECT version INTO v FROM brand_agent_policies WHERE brand_id=NEW.brand_id;
 ELSE SELECT version INTO v FROM agent_nodes WHERE brand_id=NEW.brand_id AND id=NEW.agent_id; END IF;
 IF NOT FOUND OR v<NEW.version THEN RAISE EXCEPTION 'orphan agent revision'; END IF;RETURN NULL;
END $$;

CREATE FUNCTION brand_business_inventory(b uuid) RETURNS jsonb
    LANGUAGE plpgsql STABLE
    SET search_path TO 'pg_catalog'
    AS $_$
DECLARE
    v_schema constant text := (SELECT n.nspname FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE p.oid='brand_business_inventory(uuid)'::regprocedure);
    v_tables constant text[] := '{bet_order_exceptions,bet_order_judgments,bet_orders,commission_adjustment_heads,commission_adjustments,commission_allocations,commission_calculations,commission_correction_balance_heads,commission_correction_cycle_holds,commission_correction_execution_steps,commission_correction_execution_targets,commission_correction_executions,commission_correction_plan_steps,commission_correction_plan_targets,commission_correction_plans,commission_cycle_steps,commission_cycle_targets,commission_cycles,commission_earnings,commission_payment_targets,commission_payments,commission_runs,draw_correction_failures,draw_correction_targets,draw_corrections,period_cancellation_targets,period_cancellations,point_accounts,point_buckets,point_ledger_entries,recharge_orders,reward_order_actions,reward_orders,settlement_calculations,settlement_failures,settlement_jobs,settlement_targets,withdrawal_operation_receipts,withdrawal_order_transitions,withdrawal_orders,withdrawal_turnover_cycles}'::text[];
    v_snapshot_sql constant text := E'
WITH source_rows AS MATERIALIZED (SELECT ''bet_order_exceptions''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, encode(pg_catalog.sha256(convert_to(to_jsonb(s)::text,''UTF8'')),''hex'') AS row_hash FROM bet_order_exceptions s WHERE s.brand_id=$1 UNION ALL SELECT ''bet_order_judgments''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, encode(pg_catalog.sha256(convert_to(to_jsonb(s)::text,''UTF8'')),''hex'') AS row_hash FROM bet_order_judgments s WHERE s.brand_id=$1 UNION ALL SELECT ''bet_orders''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, encode(pg_catalog.sha256(convert_to(to_jsonb(s)::text,''UTF8'')),''hex'') AS row_hash FROM bet_orders s WHERE s.brand_id=$1 UNION ALL SELECT ''commission_adjustment_heads''::text AS source_table, concat_ws(''/'',s.target_id::text) AS source_id, encode(pg_catalog.sha256(convert_to(to_jsonb(s)::text,''UTF8'')),''hex'') AS row_hash FROM commission_adjustment_heads s WHERE s.brand_id=$1 UNION ALL SELECT ''commission_adjustments''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, encode(pg_catalog.sha256(convert_to(to_jsonb(s)::text,''UTF8'')),''hex'') AS row_hash FROM commission_adjustments s WHERE s.brand_id=$1 UNION ALL SELECT ''commission_allocations''::text AS source_table, concat_ws(''/'',s.run_id::text,s.order_id::text,s.agent_id::text) AS source_id, encode(pg_catalog.sha256(convert_to(to_jsonb(s)::text,''UTF8'')),''hex'') AS row_hash FROM commission_allocations s WHERE s.brand_id=$1 UNION ALL SELECT ''commission_calculations''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, encode(pg_catalog.sha256(convert_to(to_jsonb(s)::text,''UTF8'')),''hex'') AS row_hash FROM commission_calculations s WHERE s.brand_id=$1 UNION ALL SELECT ''commission_correction_balance_heads''::text AS source_table, concat_ws(''/'',s.cycle_id::text,s.agent_id::text) AS source_id, encode(pg_catalog.sha256(convert_to(to_jsonb(s)::text,''UTF8'')),''hex'') AS row_hash FROM commission_correction_balance_heads s WHERE s.brand_id=$1 UNION ALL SELECT ''commission_correction_cycle_holds''::text AS source_table, concat_ws(''/'',s.cycle_id::text) AS source_id, encode(pg_catalog.sha256(convert_to(to_jsonb(s)::text,''UTF8'')),''hex'') AS row_hash FROM commission_correction_cycle_holds s WHERE s.brand_id=$1 UNION ALL SELECT ''commission_correction_execution_steps''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, encode(pg_catalog.sha256(convert_to(to_jsonb(s)::text,''UTF8'')),''hex'') AS row_hash FROM commission_correction_execution_steps s WHERE s.brand_id=$1 UNION ALL SELECT ''commission_correction_execution_targets''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, encode(pg_catalog.sha256(convert_to(to_jsonb(s)::text,''UTF8'')),''hex'') AS row_hash FROM commission_correction_execution_targets s WHERE s.brand_id=$1 UNION ALL SELECT ''commission_correction_executions''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, encode(pg_catalog.sha256(convert_to(to_jsonb(s)::text,''UTF8'')),''hex'') AS row_hash FROM commission_correction_executions s WHERE s.brand_id=$1 UNION ALL SELECT ''commission_correction_plan_steps''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, encode(pg_catalog.sha256(convert_to(to_jsonb(s)::text,''UTF8'')),''hex'') AS row_hash FROM commission_correction_plan_steps s WHERE s.brand_id=$1 UNION ALL SELECT ''commission_correction_plan_targets''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, encode(pg_catalog.sha256(convert_to(to_jsonb(s)::text,''UTF8'')),''hex'') AS row_hash FROM commission_correction_plan_targets s WHERE s.brand_id=$1 UNION ALL SELECT ''commission_correction_plans''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, encode(pg_catalog.sha256(convert_to(to_jsonb(s)::text,''UTF8'')),''hex'') AS row_hash FROM commission_correction_plans s WHERE s.brand_id=$1 UNION ALL SELECT ''commission_cycle_steps''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, encode(pg_catalog.sha256(convert_to(to_jsonb(s)::text,''UTF8'')),''hex'') AS row_hash FROM commission_cycle_steps s WHERE s.brand_id=$1 UNION ALL SELECT ''commission_cycle_targets''::text AS source_table, concat_ws(''/'',s.cycle_id::text,s.order_id::text) AS source_id, encode(pg_catalog.sha256(convert_to(to_jsonb(s)::text,''UTF8'')),''hex'') AS row_hash FROM commission_cycle_targets s WHERE s.brand_id=$1 UNION ALL SELECT ''commission_cycles''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, encode(pg_catalog.sha256(convert_to(to_jsonb(s)::text,''UTF8'')),''hex'') AS row_hash FROM commission_cycles s WHERE s.brand_id=$1 UNION ALL SELECT ''commission_earnings''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, encode(pg_catalog.sha256(convert_to(to_jsonb(s)::text,''UTF8'')),''hex'') AS row_hash FROM commission_earnings s WHERE s.brand_id=$1 UNION ALL SELECT ''commission_payment_targets''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, encode(pg_catalog.sha256(convert_to(to_jsonb(s)::text,''UTF8'')),''hex'') AS row_hash FROM commission_payment_targets s WHERE s.brand_id=$1 UNION ALL SELECT ''commission_payments''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, encode(pg_catalog.sha256(convert_to(to_jsonb(s)::text,''UTF8'')),''hex'') AS row_hash FROM commission_payments s WHERE s.brand_id=$1 UNION ALL SELECT ''commission_runs''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, encode(pg_catalog.sha256(convert_to(to_jsonb(s)::text,''UTF8'')),''hex'') AS row_hash FROM commission_runs s WHERE s.brand_id=$1 UNION ALL SELECT ''draw_correction_failures''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, encode(pg_catalog.sha256(convert_to(to_jsonb(s)::text,''UTF8'')),''hex'') AS row_hash FROM draw_correction_failures s WHERE s.brand_id=$1 UNION ALL SELECT ''draw_correction_targets''::text AS source_table, concat_ws(''/'',s.correction_id::text,s.order_id::text) AS source_id, encode(pg_catalog.sha256(convert_to(to_jsonb(s)::text,''UTF8'')),''hex'') AS row_hash FROM draw_correction_targets s WHERE s.brand_id=$1 UNION ALL SELECT ''draw_corrections''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, encode(pg_catalog.sha256(convert_to(to_jsonb(s)::text,''UTF8'')),''hex'') AS row_hash FROM draw_corrections s WHERE s.brand_id=$1 UNION ALL SELECT ''period_cancellation_targets''::text AS source_table, concat_ws(''/'',s.cancellation_id::text,s.order_id::text) AS source_id, encode(pg_catalog.sha256(convert_to(to_jsonb(s)::text,''UTF8'')),''hex'') AS row_hash FROM period_cancellation_targets s WHERE s.brand_id=$1 UNION ALL SELECT ''period_cancellations''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, encode(pg_catalog.sha256(convert_to(to_jsonb(s)::text,''UTF8'')),''hex'') AS row_hash FROM period_cancellations s WHERE s.brand_id=$1 UNION ALL SELECT ''point_accounts''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, encode(pg_catalog.sha256(convert_to(to_jsonb(s)::text,''UTF8'')),''hex'') AS row_hash FROM point_accounts s WHERE s.brand_id=$1 UNION ALL SELECT ''point_buckets''::text AS source_table, concat_ws(''/'',s.brand_id::text,s.account_id::text,s.source::text,s.state::text) AS source_id, encode(pg_catalog.sha256(convert_to(to_jsonb(s)::text,''UTF8'')),''hex'') AS row_hash FROM point_buckets s WHERE s.brand_id=$1 UNION ALL SELECT ''point_ledger_entries''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, encode(pg_catalog.sha256(convert_to(to_jsonb(s)::text,''UTF8'')),''hex'') AS row_hash FROM point_ledger_entries s WHERE s.brand_id=$1 UNION ALL SELECT ''recharge_orders''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, encode(pg_catalog.sha256(convert_to(to_jsonb(s)::text,''UTF8'')),''hex'') AS row_hash FROM recharge_orders s WHERE s.brand_id=$1 UNION ALL SELECT ''reward_order_actions''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, encode(pg_catalog.sha256(convert_to(to_jsonb(s)::text,''UTF8'')),''hex'') AS row_hash FROM reward_order_actions s WHERE s.brand_id=$1 UNION ALL SELECT ''reward_orders''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, encode(pg_catalog.sha256(convert_to(to_jsonb(s)::text,''UTF8'')),''hex'') AS row_hash FROM reward_orders s WHERE s.brand_id=$1 UNION ALL SELECT ''settlement_calculations''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, encode(pg_catalog.sha256(convert_to(to_jsonb(s)::text,''UTF8'')),''hex'') AS row_hash FROM settlement_calculations s WHERE s.brand_id=$1 UNION ALL SELECT ''settlement_failures''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, encode(pg_catalog.sha256(convert_to(to_jsonb(s)::text,''UTF8'')),''hex'') AS row_hash FROM settlement_failures s WHERE s.brand_id=$1 UNION ALL SELECT ''settlement_jobs''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, encode(pg_catalog.sha256(convert_to(to_jsonb(s)::text,''UTF8'')),''hex'') AS row_hash FROM settlement_jobs s WHERE s.brand_id=$1 UNION ALL SELECT ''settlement_targets''::text AS source_table, concat_ws(''/'',s.job_id::text,s.order_id::text) AS source_id, encode(pg_catalog.sha256(convert_to(to_jsonb(s)::text,''UTF8'')),''hex'') AS row_hash FROM settlement_targets s WHERE s.brand_id=$1 UNION ALL SELECT ''withdrawal_operation_receipts''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, encode(pg_catalog.sha256(convert_to(to_jsonb(s)::text,''UTF8'')),''hex'') AS row_hash FROM withdrawal_operation_receipts s WHERE s.brand_id=$1 UNION ALL SELECT ''withdrawal_order_transitions''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, encode(pg_catalog.sha256(convert_to(to_jsonb(s)::text,''UTF8'')),''hex'') AS row_hash FROM withdrawal_order_transitions s WHERE s.brand_id=$1 UNION ALL SELECT ''withdrawal_orders''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, encode(pg_catalog.sha256(convert_to(to_jsonb(s)::text,''UTF8'')),''hex'') AS row_hash FROM withdrawal_orders s WHERE s.brand_id=$1 UNION ALL SELECT ''withdrawal_turnover_cycles''::text AS source_table, concat_ws(''/'',s.brand_id::text,s.member_id::text) AS source_id, encode(pg_catalog.sha256(convert_to(to_jsonb(s)::text,''UTF8'')),''hex'') AS row_hash FROM withdrawal_turnover_cycles s WHERE s.brand_id=$1),
refs AS MATERIALIZED (
    SELECT source_table,source_id,''MISSING_PARENT_REFERENCE''::text AS code,
           reference_key,parent_table,missing FROM (SELECT ''bet_order_exceptions''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''bet_order_exceptions_brand_id_job_id_fkey''::text AS reference_key, ''settlement_jobs''::text AS parent_table, NOT EXISTS (SELECT 1 FROM settlement_jobs p WHERE p.brand_id=s.brand_id AND p.id=s.job_id AND p.brand_id=$1) AS missing FROM bet_order_exceptions s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.job_id IS NOT NULL UNION ALL SELECT ''bet_order_exceptions''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''bet_order_exceptions_brand_id_order_id_fkey''::text AS reference_key, ''bet_orders''::text AS parent_table, NOT EXISTS (SELECT 1 FROM bet_orders p WHERE p.brand_id=s.brand_id AND p.id=s.order_id AND p.brand_id=$1) AS missing FROM bet_order_exceptions s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.order_id IS NOT NULL UNION ALL SELECT ''bet_order_exceptions''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''bet_order_exceptions_marked_by_fkey''::text AS reference_key, ''admin_accounts''::text AS parent_table, NOT EXISTS (SELECT 1 FROM admin_accounts p WHERE p.id=s.marked_by) AS missing FROM bet_order_exceptions s WHERE s.brand_id=$1 AND s.marked_by IS NOT NULL UNION ALL SELECT ''bet_order_judgments''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''bet_order_judgments_brand_id_game_id_period_id_draw_result_fkey''::text AS reference_key, ''draw_results''::text AS parent_table, NOT EXISTS (SELECT 1 FROM draw_results p WHERE p.brand_id=s.brand_id AND p.game_id=s.game_id AND p.period_id=s.period_id AND p.id=s.draw_result_id AND p.brand_id=$1) AS missing FROM bet_order_judgments s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.game_id IS NOT NULL AND s.period_id IS NOT NULL AND s.draw_result_id IS NOT NULL UNION ALL SELECT ''bet_order_judgments''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''bet_order_judgments_brand_id_game_id_period_id_fkey''::text AS reference_key, ''periods''::text AS parent_table, NOT EXISTS (SELECT 1 FROM periods p WHERE p.brand_id=s.brand_id AND p.game_id=s.game_id AND p.id=s.period_id AND p.brand_id=$1) AS missing FROM bet_order_judgments s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.game_id IS NOT NULL AND s.period_id IS NOT NULL UNION ALL SELECT ''bet_order_judgments''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''bet_order_judgments_brand_id_period_id_order_id_fkey''::text AS reference_key, ''bet_orders''::text AS parent_table, NOT EXISTS (SELECT 1 FROM bet_orders p WHERE p.brand_id=s.brand_id AND p.period_id=s.period_id AND p.id=s.order_id AND p.brand_id=$1) AS missing FROM bet_order_judgments s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.period_id IS NOT NULL AND s.order_id IS NOT NULL UNION ALL SELECT ''bet_order_judgments''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''bet_order_judgments_judged_by_fkey''::text AS reference_key, ''admin_accounts''::text AS parent_table, NOT EXISTS (SELECT 1 FROM admin_accounts p WHERE p.id=s.judged_by) AS missing FROM bet_order_judgments s WHERE s.brand_id=$1 AND s.judged_by IS NOT NULL UNION ALL SELECT ''bet_orders''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''bet_orders_brand_id_account_id_brand_member_id_fkey''::text AS reference_key, ''point_accounts''::text AS parent_table, NOT EXISTS (SELECT 1 FROM point_accounts p WHERE p.brand_id=s.brand_id AND p.id=s.account_id AND p.brand_member_id=s.brand_member_id AND p.brand_id=$1) AS missing FROM bet_orders s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.account_id IS NOT NULL AND s.brand_member_id IS NOT NULL UNION ALL SELECT ''bet_orders''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''bet_orders_brand_id_account_id_debit_entry_id_fkey''::text AS reference_key, ''point_ledger_entries''::text AS parent_table, NOT EXISTS (SELECT 1 FROM point_ledger_entries p WHERE p.brand_id=s.brand_id AND p.account_id=s.account_id AND p.id=s.debit_entry_id AND p.brand_id=$1) AS missing FROM bet_orders s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.account_id IS NOT NULL AND s.debit_entry_id IS NOT NULL UNION ALL SELECT ''bet_orders''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''bet_orders_brand_id_account_id_refund_entry_id_fkey''::text AS reference_key, ''point_ledger_entries''::text AS parent_table, NOT EXISTS (SELECT 1 FROM point_ledger_entries p WHERE p.brand_id=s.brand_id AND p.account_id=s.account_id AND p.id=s.refund_entry_id AND p.brand_id=$1) AS missing FROM bet_orders s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.account_id IS NOT NULL AND s.refund_entry_id IS NOT NULL UNION ALL SELECT ''bet_orders''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''bet_orders_brand_id_brand_member_id_global_user_id_fkey''::text AS reference_key, ''brand_members''::text AS parent_table, NOT EXISTS (SELECT 1 FROM brand_members p WHERE p.brand_id=s.brand_id AND p.id=s.brand_member_id AND p.global_user_id=s.global_user_id AND p.brand_id=$1) AS missing FROM bet_orders s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.brand_member_id IS NOT NULL AND s.global_user_id IS NOT NULL UNION ALL SELECT ''bet_orders''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''bet_orders_brand_id_game_id_period_id_fkey''::text AS reference_key, ''periods''::text AS parent_table, NOT EXISTS (SELECT 1 FROM periods p WHERE p.brand_id=s.brand_id AND p.game_id=s.game_id AND p.id=s.period_id AND p.brand_id=$1) AS missing FROM bet_orders s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.game_id IS NOT NULL AND s.period_id IS NOT NULL UNION ALL SELECT ''bet_orders''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''bet_orders_brand_id_game_id_play_id_rule_version_id_fkey''::text AS reference_key, ''rule_versions''::text AS parent_table, NOT EXISTS (SELECT 1 FROM rule_versions p WHERE p.brand_id=s.brand_id AND p.game_id=s.game_id AND p.play_id=s.play_id AND p.id=s.rule_version_id AND p.brand_id=$1) AS missing FROM bet_orders s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.game_id IS NOT NULL AND s.play_id IS NOT NULL AND s.rule_version_id IS NOT NULL UNION ALL SELECT ''bet_orders''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''bet_orders_brand_id_id_settlement_calculation_id_fkey''::text AS reference_key, ''settlement_calculations''::text AS parent_table, NOT EXISTS (SELECT 1 FROM settlement_calculations p WHERE p.brand_id=s.brand_id AND p.order_id=s.id AND p.id=s.settlement_calculation_id AND p.brand_id=$1) AS missing FROM bet_orders s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.id IS NOT NULL AND s.settlement_calculation_id IS NOT NULL UNION ALL SELECT ''bet_orders''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''bet_orders_payout_entry_id_fkey''::text AS reference_key, ''point_ledger_entries''::text AS parent_table, NOT EXISTS (SELECT 1 FROM point_ledger_entries p WHERE p.id=s.payout_entry_id AND p.brand_id=$1) AS missing FROM bet_orders s WHERE s.brand_id=$1 AND s.payout_entry_id IS NOT NULL UNION ALL SELECT ''commission_adjustment_heads''::text AS source_table, concat_ws(''/'',s.target_id::text) AS source_id, ''commission_adjustment_heads_brand_id_target_id_fkey''::text AS reference_key, ''commission_payment_targets''::text AS parent_table, NOT EXISTS (SELECT 1 FROM commission_payment_targets p WHERE p.brand_id=s.brand_id AND p.id=s.target_id AND p.brand_id=$1) AS missing FROM commission_adjustment_heads s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.target_id IS NOT NULL UNION ALL SELECT ''commission_adjustment_heads''::text AS source_table, concat_ws(''/'',s.target_id::text) AS source_id, ''commission_adjustment_heads_brand_id_target_id_last_adjust_fkey''::text AS reference_key, ''commission_adjustments''::text AS parent_table, NOT EXISTS (SELECT 1 FROM commission_adjustments p WHERE p.brand_id=s.brand_id AND p.target_id=s.target_id AND p.id=s.last_adjustment_id AND p.brand_id=$1) AS missing FROM commission_adjustment_heads s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.target_id IS NOT NULL AND s.last_adjustment_id IS NOT NULL UNION ALL SELECT ''commission_adjustments''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_adjustments_audit_log_id_fkey''::text AS reference_key, ''audit_logs''::text AS parent_table, NOT EXISTS (SELECT 1 FROM audit_logs p WHERE p.id=s.audit_log_id AND p.brand_id=$1) AS missing FROM commission_adjustments s WHERE s.brand_id=$1 AND s.audit_log_id IS NOT NULL UNION ALL SELECT ''commission_adjustments''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_adjustments_brand_id_payment_id_fkey''::text AS reference_key, ''commission_payments''::text AS parent_table, NOT EXISTS (SELECT 1 FROM commission_payments p WHERE p.brand_id=s.brand_id AND p.id=s.payment_id AND p.brand_id=$1) AS missing FROM commission_adjustments s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.payment_id IS NOT NULL UNION ALL SELECT ''commission_adjustments''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_adjustments_brand_id_target_id_fkey''::text AS reference_key, ''commission_adjustment_heads''::text AS parent_table, NOT EXISTS (SELECT 1 FROM commission_adjustment_heads p WHERE p.brand_id=s.brand_id AND p.target_id=s.target_id AND p.brand_id=$1) AS missing FROM commission_adjustments s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.target_id IS NOT NULL UNION ALL SELECT ''commission_adjustments''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_adjustments_created_by_fkey''::text AS reference_key, ''admin_accounts''::text AS parent_table, NOT EXISTS (SELECT 1 FROM admin_accounts p WHERE p.id=s.created_by) AS missing FROM commission_adjustments s WHERE s.brand_id=$1 AND s.created_by IS NOT NULL UNION ALL SELECT ''commission_adjustments''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_adjustments_ledger_entry_id_fkey''::text AS reference_key, ''point_ledger_entries''::text AS parent_table, NOT EXISTS (SELECT 1 FROM point_ledger_entries p WHERE p.id=s.ledger_entry_id AND p.brand_id=$1) AS missing FROM commission_adjustments s WHERE s.brand_id=$1 AND s.ledger_entry_id IS NOT NULL UNION ALL SELECT ''commission_allocations''::text AS source_table, concat_ws(''/'',s.run_id::text,s.order_id::text,s.agent_id::text) AS source_id, ''commission_allocations_brand_id_agent_id_fkey''::text AS reference_key, ''agent_nodes''::text AS parent_table, NOT EXISTS (SELECT 1 FROM agent_nodes p WHERE p.brand_id=s.brand_id AND p.id=s.agent_id AND p.brand_id=$1) AS missing FROM commission_allocations s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.agent_id IS NOT NULL UNION ALL SELECT ''commission_allocations''::text AS source_table, concat_ws(''/'',s.run_id::text,s.order_id::text,s.agent_id::text) AS source_id, ''commission_allocations_brand_id_cycle_id_run_id_calculatio_fkey''::text AS reference_key, ''commission_calculations''::text AS parent_table, NOT EXISTS (SELECT 1 FROM commission_calculations p WHERE p.brand_id=s.brand_id AND p.cycle_id=s.cycle_id AND p.run_id=s.run_id AND p.id=s.calculation_id AND p.brand_id=$1) AS missing FROM commission_allocations s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.cycle_id IS NOT NULL AND s.run_id IS NOT NULL AND s.calculation_id IS NOT NULL UNION ALL SELECT ''commission_allocations''::text AS source_table, concat_ws(''/'',s.run_id::text,s.order_id::text,s.agent_id::text) AS source_id, ''commission_allocations_brand_id_member_id_fkey''::text AS reference_key, ''brand_members''::text AS parent_table, NOT EXISTS (SELECT 1 FROM brand_members p WHERE p.brand_id=s.brand_id AND p.id=s.member_id AND p.brand_id=$1) AS missing FROM commission_allocations s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.member_id IS NOT NULL UNION ALL SELECT ''commission_calculations''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_calculations_audit_log_id_fkey''::text AS reference_key, ''audit_logs''::text AS parent_table, NOT EXISTS (SELECT 1 FROM audit_logs p WHERE p.id=s.audit_log_id AND p.brand_id=$1) AS missing FROM commission_calculations s WHERE s.brand_id=$1 AND s.audit_log_id IS NOT NULL UNION ALL SELECT ''commission_calculations''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_calculations_brand_id_account_id_member_id_fkey''::text AS reference_key, ''point_accounts''::text AS parent_table, NOT EXISTS (SELECT 1 FROM point_accounts p WHERE p.brand_id=s.brand_id AND p.id=s.account_id AND p.brand_member_id=s.member_id AND p.brand_id=$1) AS missing FROM commission_calculations s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.account_id IS NOT NULL AND s.member_id IS NOT NULL UNION ALL SELECT ''commission_calculations''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_calculations_brand_id_cycle_id_order_id_fkey''::text AS reference_key, ''commission_cycle_targets''::text AS parent_table, NOT EXISTS (SELECT 1 FROM commission_cycle_targets p WHERE p.brand_id=s.brand_id AND p.cycle_id=s.cycle_id AND p.order_id=s.order_id AND p.brand_id=$1) AS missing FROM commission_calculations s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.cycle_id IS NOT NULL AND s.order_id IS NOT NULL UNION ALL SELECT ''commission_calculations''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_calculations_brand_id_cycle_id_run_id_fkey''::text AS reference_key, ''commission_runs''::text AS parent_table, NOT EXISTS (SELECT 1 FROM commission_runs p WHERE p.brand_id=s.brand_id AND p.cycle_id=s.cycle_id AND p.id=s.run_id AND p.brand_id=$1) AS missing FROM commission_calculations s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.cycle_id IS NOT NULL AND s.run_id IS NOT NULL UNION ALL SELECT ''commission_correction_balance_heads''::text AS source_table, concat_ws(''/'',s.cycle_id::text,s.agent_id::text) AS source_id, ''commission_correction_balance__brand_id_original_target_id_fkey''::text AS reference_key, ''commission_payment_targets''::text AS parent_table, NOT EXISTS (SELECT 1 FROM commission_payment_targets p WHERE p.brand_id=s.brand_id AND p.id=s.original_target_id AND p.brand_id=$1) AS missing FROM commission_correction_balance_heads s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.original_target_id IS NOT NULL UNION ALL SELECT ''commission_correction_balance_heads''::text AS source_table, concat_ws(''/'',s.cycle_id::text,s.agent_id::text) AS source_id, ''commission_correction_balance_brand_id_last_execution_targ_fkey''::text AS reference_key, ''commission_correction_execution_targets''::text AS parent_table, NOT EXISTS (SELECT 1 FROM commission_correction_execution_targets p WHERE p.brand_id=s.brand_id AND p.id=s.last_execution_target_id AND p.brand_id=$1) AS missing FROM commission_correction_balance_heads s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.last_execution_target_id IS NOT NULL UNION ALL SELECT ''commission_correction_balance_heads''::text AS source_table, concat_ws(''/'',s.cycle_id::text,s.agent_id::text) AS source_id, ''commission_correction_balance_heads_brand_id_cycle_id_fkey''::text AS reference_key, ''commission_cycles''::text AS parent_table, NOT EXISTS (SELECT 1 FROM commission_cycles p WHERE p.brand_id=s.brand_id AND p.id=s.cycle_id AND p.brand_id=$1) AS missing FROM commission_correction_balance_heads s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.cycle_id IS NOT NULL UNION ALL SELECT ''commission_correction_balance_heads''::text AS source_table, concat_ws(''/'',s.cycle_id::text,s.agent_id::text) AS source_id, ''commission_correction_balance_heads_brand_id_member_id_fkey''::text AS reference_key, ''brand_members''::text AS parent_table, NOT EXISTS (SELECT 1 FROM brand_members p WHERE p.brand_id=s.brand_id AND p.id=s.member_id AND p.brand_id=$1) AS missing FROM commission_correction_balance_heads s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.member_id IS NOT NULL UNION ALL SELECT ''commission_correction_cycle_holds''::text AS source_table, concat_ws(''/'',s.cycle_id::text) AS source_id, ''commission_correction_cycle_holds_audit_log_id_fkey''::text AS reference_key, ''audit_logs''::text AS parent_table, NOT EXISTS (SELECT 1 FROM audit_logs p WHERE p.id=s.audit_log_id AND p.brand_id=$1) AS missing FROM commission_correction_cycle_holds s WHERE s.brand_id=$1 AND s.audit_log_id IS NOT NULL UNION ALL SELECT ''commission_correction_cycle_holds''::text AS source_table, concat_ws(''/'',s.cycle_id::text) AS source_id, ''commission_correction_cycle_holds_blocked_execution_id_fkey''::text AS reference_key, ''commission_correction_executions''::text AS parent_table, NOT EXISTS (SELECT 1 FROM commission_correction_executions p WHERE p.id=s.blocked_execution_id AND p.brand_id=$1) AS missing FROM commission_correction_cycle_holds s WHERE s.brand_id=$1 AND s.blocked_execution_id IS NOT NULL UNION ALL SELECT ''commission_correction_cycle_holds''::text AS source_table, concat_ws(''/'',s.cycle_id::text) AS source_id, ''commission_correction_cycle_holds_brand_id_cycle_id_fkey''::text AS reference_key, ''commission_cycles''::text AS parent_table, NOT EXISTS (SELECT 1 FROM commission_cycles p WHERE p.brand_id=s.brand_id AND p.id=s.cycle_id AND p.brand_id=$1) AS missing FROM commission_correction_cycle_holds s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.cycle_id IS NOT NULL UNION ALL SELECT ''commission_correction_execution_steps''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_correction_execution_step_brand_id_execution_id_fkey''::text AS reference_key, ''commission_correction_executions''::text AS parent_table, NOT EXISTS (SELECT 1 FROM commission_correction_executions p WHERE p.brand_id=s.brand_id AND p.id=s.execution_id AND p.brand_id=$1) AS missing FROM commission_correction_execution_steps s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.execution_id IS NOT NULL UNION ALL SELECT ''commission_correction_execution_steps''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_correction_execution_step_paused_plan_target_id_fkey''::text AS reference_key, ''commission_correction_plan_targets''::text AS parent_table, NOT EXISTS (SELECT 1 FROM commission_correction_plan_targets p WHERE p.id=s.paused_plan_target_id AND p.brand_id=$1) AS missing FROM commission_correction_execution_steps s WHERE s.brand_id=$1 AND s.paused_plan_target_id IS NOT NULL UNION ALL SELECT ''commission_correction_execution_steps''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_correction_execution_steps_actor_id_fkey''::text AS reference_key, ''admin_accounts''::text AS parent_table, NOT EXISTS (SELECT 1 FROM admin_accounts p WHERE p.id=s.actor_id) AS missing FROM commission_correction_execution_steps s WHERE s.brand_id=$1 AND s.actor_id IS NOT NULL UNION ALL SELECT ''commission_correction_execution_steps''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_correction_execution_steps_audit_log_id_fkey''::text AS reference_key, ''audit_logs''::text AS parent_table, NOT EXISTS (SELECT 1 FROM audit_logs p WHERE p.id=s.audit_log_id AND p.brand_id=$1) AS missing FROM commission_correction_execution_steps s WHERE s.brand_id=$1 AND s.audit_log_id IS NOT NULL UNION ALL SELECT ''commission_correction_execution_steps''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_correction_execution_steps_target_id_fkey''::text AS reference_key, ''commission_correction_execution_targets''::text AS parent_table, NOT EXISTS (SELECT 1 FROM commission_correction_execution_targets p WHERE p.id=s.target_id AND p.brand_id=$1) AS missing FROM commission_correction_execution_steps s WHERE s.brand_id=$1 AND s.target_id IS NOT NULL UNION ALL SELECT ''commission_correction_execution_targets''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_correction_executi_brand_id_cycle_id_execution__fkey''::text AS reference_key, ''commission_correction_executions''::text AS parent_table, NOT EXISTS (SELECT 1 FROM commission_correction_executions p WHERE p.brand_id=s.brand_id AND p.cycle_id=s.cycle_id AND p.id=s.execution_id AND p.brand_id=$1) AS missing FROM commission_correction_execution_targets s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.cycle_id IS NOT NULL AND s.execution_id IS NOT NULL UNION ALL SELECT ''commission_correction_execution_targets''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_correction_execution_targets_audit_log_id_fkey''::text AS reference_key, ''audit_logs''::text AS parent_table, NOT EXISTS (SELECT 1 FROM audit_logs p WHERE p.id=s.audit_log_id AND p.brand_id=$1) AS missing FROM commission_correction_execution_targets s WHERE s.brand_id=$1 AND s.audit_log_id IS NOT NULL UNION ALL SELECT ''commission_correction_execution_targets''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_correction_execution_targets_brand_id_member_id_fkey''::text AS reference_key, ''brand_members''::text AS parent_table, NOT EXISTS (SELECT 1 FROM brand_members p WHERE p.brand_id=s.brand_id AND p.id=s.member_id AND p.brand_id=$1) AS missing FROM commission_correction_execution_targets s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.member_id IS NOT NULL UNION ALL SELECT ''commission_correction_execution_targets''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_correction_execution_targets_ledger_entry_id_fkey''::text AS reference_key, ''point_ledger_entries''::text AS parent_table, NOT EXISTS (SELECT 1 FROM point_ledger_entries p WHERE p.id=s.ledger_entry_id AND p.brand_id=$1) AS missing FROM commission_correction_execution_targets s WHERE s.brand_id=$1 AND s.ledger_entry_id IS NOT NULL UNION ALL SELECT ''commission_correction_executions''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_correction_executions_approval_audit_log_id_fkey''::text AS reference_key, ''audit_logs''::text AS parent_table, NOT EXISTS (SELECT 1 FROM audit_logs p WHERE p.id=s.approval_audit_log_id AND p.brand_id=$1) AS missing FROM commission_correction_executions s WHERE s.brand_id=$1 AND s.approval_audit_log_id IS NOT NULL UNION ALL SELECT ''commission_correction_executions''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_correction_executions_approved_by_fkey''::text AS reference_key, ''admin_accounts''::text AS parent_table, NOT EXISTS (SELECT 1 FROM admin_accounts p WHERE p.id=s.approved_by) AS missing FROM commission_correction_executions s WHERE s.brand_id=$1 AND s.approved_by IS NOT NULL UNION ALL SELECT ''commission_correction_executions''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_correction_executions_brand_id_cycle_id_plan_id_fkey''::text AS reference_key, ''commission_correction_plans''::text AS parent_table, NOT EXISTS (SELECT 1 FROM commission_correction_plans p WHERE p.brand_id=s.brand_id AND p.cycle_id=s.cycle_id AND p.id=s.plan_id AND p.brand_id=$1) AS missing FROM commission_correction_executions s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.cycle_id IS NOT NULL AND s.plan_id IS NOT NULL UNION ALL SELECT ''commission_correction_executions''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_correction_executions_creation_audit_log_id_fkey''::text AS reference_key, ''audit_logs''::text AS parent_table, NOT EXISTS (SELECT 1 FROM audit_logs p WHERE p.id=s.creation_audit_log_id AND p.brand_id=$1) AS missing FROM commission_correction_executions s WHERE s.brand_id=$1 AND s.creation_audit_log_id IS NOT NULL UNION ALL SELECT ''commission_correction_executions''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_correction_executions_last_audit_log_id_fkey''::text AS reference_key, ''audit_logs''::text AS parent_table, NOT EXISTS (SELECT 1 FROM audit_logs p WHERE p.id=s.last_audit_log_id AND p.brand_id=$1) AS missing FROM commission_correction_executions s WHERE s.brand_id=$1 AND s.last_audit_log_id IS NOT NULL UNION ALL SELECT ''commission_correction_executions''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_correction_executions_paused_plan_target_id_fkey''::text AS reference_key, ''commission_correction_plan_targets''::text AS parent_table, NOT EXISTS (SELECT 1 FROM commission_correction_plan_targets p WHERE p.id=s.paused_plan_target_id AND p.brand_id=$1) AS missing FROM commission_correction_executions s WHERE s.brand_id=$1 AND s.paused_plan_target_id IS NOT NULL UNION ALL SELECT ''commission_correction_plan_steps''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_correction_plan_steps_actor_id_fkey''::text AS reference_key, ''admin_accounts''::text AS parent_table, NOT EXISTS (SELECT 1 FROM admin_accounts p WHERE p.id=s.actor_id) AS missing FROM commission_correction_plan_steps s WHERE s.brand_id=$1 AND s.actor_id IS NOT NULL UNION ALL SELECT ''commission_correction_plan_steps''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_correction_plan_steps_audit_log_id_fkey''::text AS reference_key, ''audit_logs''::text AS parent_table, NOT EXISTS (SELECT 1 FROM audit_logs p WHERE p.id=s.audit_log_id AND p.brand_id=$1) AS missing FROM commission_correction_plan_steps s WHERE s.brand_id=$1 AND s.audit_log_id IS NOT NULL UNION ALL SELECT ''commission_correction_plan_steps''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_correction_plan_steps_brand_id_plan_id_fkey''::text AS reference_key, ''commission_correction_plans''::text AS parent_table, NOT EXISTS (SELECT 1 FROM commission_correction_plans p WHERE p.brand_id=s.brand_id AND p.id=s.plan_id AND p.brand_id=$1) AS missing FROM commission_correction_plan_steps s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.plan_id IS NOT NULL UNION ALL SELECT ''commission_correction_plan_targets''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_correction_plan_ta_brand_id_previous_correction_fkey''::text AS reference_key, ''commission_correction_execution_targets''::text AS parent_table, NOT EXISTS (SELECT 1 FROM commission_correction_execution_targets p WHERE p.brand_id=s.brand_id AND p.id=s.previous_correction_target_id AND p.brand_id=$1) AS missing FROM commission_correction_plan_targets s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.previous_correction_target_id IS NOT NULL UNION ALL SELECT ''commission_correction_plan_targets''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_correction_plan_tar_brand_id_original_target_id_fkey''::text AS reference_key, ''commission_payment_targets''::text AS parent_table, NOT EXISTS (SELECT 1 FROM commission_payment_targets p WHERE p.brand_id=s.brand_id AND p.id=s.original_target_id AND p.brand_id=$1) AS missing FROM commission_correction_plan_targets s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.original_target_id IS NOT NULL UNION ALL SELECT ''commission_correction_plan_targets''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_correction_plan_targets_brand_id_member_id_fkey''::text AS reference_key, ''brand_members''::text AS parent_table, NOT EXISTS (SELECT 1 FROM brand_members p WHERE p.brand_id=s.brand_id AND p.id=s.member_id AND p.brand_id=$1) AS missing FROM commission_correction_plan_targets s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.member_id IS NOT NULL UNION ALL SELECT ''commission_correction_plan_targets''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_correction_plan_targets_brand_id_plan_id_fkey''::text AS reference_key, ''commission_correction_plans''::text AS parent_table, NOT EXISTS (SELECT 1 FROM commission_correction_plans p WHERE p.brand_id=s.brand_id AND p.id=s.plan_id AND p.brand_id=$1) AS missing FROM commission_correction_plan_targets s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.plan_id IS NOT NULL UNION ALL SELECT ''commission_correction_plan_targets''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_correction_plan_targets_creation_audit_log_id_fkey''::text AS reference_key, ''audit_logs''::text AS parent_table, NOT EXISTS (SELECT 1 FROM audit_logs p WHERE p.id=s.creation_audit_log_id AND p.brand_id=$1) AS missing FROM commission_correction_plan_targets s WHERE s.brand_id=$1 AND s.creation_audit_log_id IS NOT NULL UNION ALL SELECT ''commission_correction_plan_targets''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_correction_plan_targets_earning_id_fkey''::text AS reference_key, ''commission_earnings''::text AS parent_table, NOT EXISTS (SELECT 1 FROM commission_earnings p WHERE p.id=s.earning_id AND p.brand_id=$1) AS missing FROM commission_correction_plan_targets s WHERE s.brand_id=$1 AND s.earning_id IS NOT NULL UNION ALL SELECT ''commission_correction_plans''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_correction_plans_brand_id_cycle_id_run_id_fkey''::text AS reference_key, ''commission_runs''::text AS parent_table, NOT EXISTS (SELECT 1 FROM commission_runs p WHERE p.brand_id=s.brand_id AND p.cycle_id=s.cycle_id AND p.id=s.run_id AND p.brand_id=$1) AS missing FROM commission_correction_plans s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.cycle_id IS NOT NULL AND s.run_id IS NOT NULL UNION ALL SELECT ''commission_correction_plans''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_correction_plans_brand_id_payment_id_fkey''::text AS reference_key, ''commission_payments''::text AS parent_table, NOT EXISTS (SELECT 1 FROM commission_payments p WHERE p.brand_id=s.brand_id AND p.id=s.payment_id AND p.brand_id=$1) AS missing FROM commission_correction_plans s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.payment_id IS NOT NULL UNION ALL SELECT ''commission_correction_plans''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_correction_plans_creation_audit_log_id_fkey''::text AS reference_key, ''audit_logs''::text AS parent_table, NOT EXISTS (SELECT 1 FROM audit_logs p WHERE p.id=s.creation_audit_log_id AND p.brand_id=$1) AS missing FROM commission_correction_plans s WHERE s.brand_id=$1 AND s.creation_audit_log_id IS NOT NULL UNION ALL SELECT ''commission_correction_plans''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_correction_plans_last_audit_log_id_fkey''::text AS reference_key, ''audit_logs''::text AS parent_table, NOT EXISTS (SELECT 1 FROM audit_logs p WHERE p.id=s.last_audit_log_id AND p.brand_id=$1) AS missing FROM commission_correction_plans s WHERE s.brand_id=$1 AND s.last_audit_log_id IS NOT NULL UNION ALL SELECT ''commission_cycle_steps''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_cycle_steps_audit_log_id_fkey''::text AS reference_key, ''audit_logs''::text AS parent_table, NOT EXISTS (SELECT 1 FROM audit_logs p WHERE p.id=s.audit_log_id AND p.brand_id=$1) AS missing FROM commission_cycle_steps s WHERE s.brand_id=$1 AND s.audit_log_id IS NOT NULL UNION ALL SELECT ''commission_cycle_steps''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_cycle_steps_brand_id_cycle_id_fkey''::text AS reference_key, ''commission_cycles''::text AS parent_table, NOT EXISTS (SELECT 1 FROM commission_cycles p WHERE p.brand_id=s.brand_id AND p.id=s.cycle_id AND p.brand_id=$1) AS missing FROM commission_cycle_steps s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.cycle_id IS NOT NULL UNION ALL SELECT ''commission_cycle_steps''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_cycle_steps_brand_id_cycle_id_run_id_fkey''::text AS reference_key, ''commission_runs''::text AS parent_table, NOT EXISTS (SELECT 1 FROM commission_runs p WHERE p.brand_id=s.brand_id AND p.cycle_id=s.cycle_id AND p.id=s.run_id AND p.brand_id=$1) AS missing FROM commission_cycle_steps s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.cycle_id IS NOT NULL AND s.run_id IS NOT NULL UNION ALL SELECT ''commission_cycle_targets''::text AS source_table, concat_ws(''/'',s.cycle_id::text,s.order_id::text) AS source_id, ''commission_cycle_targets_brand_id_cycle_id_fkey''::text AS reference_key, ''commission_cycles''::text AS parent_table, NOT EXISTS (SELECT 1 FROM commission_cycles p WHERE p.brand_id=s.brand_id AND p.id=s.cycle_id AND p.brand_id=$1) AS missing FROM commission_cycle_targets s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.cycle_id IS NOT NULL UNION ALL SELECT ''commission_cycle_targets''::text AS source_table, concat_ws(''/'',s.cycle_id::text,s.order_id::text) AS source_id, ''commission_cycle_targets_brand_id_order_id_fkey''::text AS reference_key, ''bet_orders''::text AS parent_table, NOT EXISTS (SELECT 1 FROM bet_orders p WHERE p.brand_id=s.brand_id AND p.id=s.order_id AND p.brand_id=$1) AS missing FROM commission_cycle_targets s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.order_id IS NOT NULL UNION ALL SELECT ''commission_cycles''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_cycles_brand_id_anchor_order_id_fkey''::text AS reference_key, ''bet_orders''::text AS parent_table, NOT EXISTS (SELECT 1 FROM bet_orders p WHERE p.brand_id=s.brand_id AND p.id=s.anchor_order_id AND p.brand_id=$1) AS missing FROM commission_cycles s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.anchor_order_id IS NOT NULL UNION ALL SELECT ''commission_cycles''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_cycles_brand_id_fkey''::text AS reference_key, ''brands''::text AS parent_table, NOT EXISTS (SELECT 1 FROM brands p WHERE p.id=s.brand_id) AS missing FROM commission_cycles s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL UNION ALL SELECT ''commission_cycles''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_cycles_brand_id_id_current_run_id_fkey''::text AS reference_key, ''commission_runs''::text AS parent_table, NOT EXISTS (SELECT 1 FROM commission_runs p WHERE p.brand_id=s.brand_id AND p.cycle_id=s.id AND p.id=s.current_run_id AND p.brand_id=$1) AS missing FROM commission_cycles s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.id IS NOT NULL AND s.current_run_id IS NOT NULL UNION ALL SELECT ''commission_cycles''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_cycles_created_by_fkey''::text AS reference_key, ''admin_accounts''::text AS parent_table, NOT EXISTS (SELECT 1 FROM admin_accounts p WHERE p.id=s.created_by) AS missing FROM commission_cycles s WHERE s.brand_id=$1 AND s.created_by IS NOT NULL UNION ALL SELECT ''commission_cycles''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_cycles_creation_audit_log_id_fkey''::text AS reference_key, ''audit_logs''::text AS parent_table, NOT EXISTS (SELECT 1 FROM audit_logs p WHERE p.id=s.creation_audit_log_id AND p.brand_id=$1) AS missing FROM commission_cycles s WHERE s.brand_id=$1 AND s.creation_audit_log_id IS NOT NULL UNION ALL SELECT ''commission_earnings''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_earnings_brand_id_agent_id_fkey''::text AS reference_key, ''agent_nodes''::text AS parent_table, NOT EXISTS (SELECT 1 FROM agent_nodes p WHERE p.brand_id=s.brand_id AND p.id=s.agent_id AND p.brand_id=$1) AS missing FROM commission_earnings s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.agent_id IS NOT NULL UNION ALL SELECT ''commission_earnings''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_earnings_brand_id_cycle_id_run_id_fkey''::text AS reference_key, ''commission_runs''::text AS parent_table, NOT EXISTS (SELECT 1 FROM commission_runs p WHERE p.brand_id=s.brand_id AND p.cycle_id=s.cycle_id AND p.id=s.run_id AND p.brand_id=$1) AS missing FROM commission_earnings s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.cycle_id IS NOT NULL AND s.run_id IS NOT NULL UNION ALL SELECT ''commission_earnings''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_earnings_brand_id_member_id_fkey''::text AS reference_key, ''brand_members''::text AS parent_table, NOT EXISTS (SELECT 1 FROM brand_members p WHERE p.brand_id=s.brand_id AND p.id=s.member_id AND p.brand_id=$1) AS missing FROM commission_earnings s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.member_id IS NOT NULL UNION ALL SELECT ''commission_payment_targets''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_payment_targets_audit_log_id_fkey''::text AS reference_key, ''audit_logs''::text AS parent_table, NOT EXISTS (SELECT 1 FROM audit_logs p WHERE p.id=s.audit_log_id AND p.brand_id=$1) AS missing FROM commission_payment_targets s WHERE s.brand_id=$1 AND s.audit_log_id IS NOT NULL UNION ALL SELECT ''commission_payment_targets''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_payment_targets_brand_id_member_id_fkey''::text AS reference_key, ''brand_members''::text AS parent_table, NOT EXISTS (SELECT 1 FROM brand_members p WHERE p.brand_id=s.brand_id AND p.id=s.member_id AND p.brand_id=$1) AS missing FROM commission_payment_targets s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.member_id IS NOT NULL UNION ALL SELECT ''commission_payment_targets''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_payment_targets_brand_id_payment_id_fkey''::text AS reference_key, ''commission_payments''::text AS parent_table, NOT EXISTS (SELECT 1 FROM commission_payments p WHERE p.brand_id=s.brand_id AND p.id=s.payment_id AND p.brand_id=$1) AS missing FROM commission_payment_targets s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.payment_id IS NOT NULL UNION ALL SELECT ''commission_payment_targets''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_payment_targets_ledger_entry_id_fkey''::text AS reference_key, ''point_ledger_entries''::text AS parent_table, NOT EXISTS (SELECT 1 FROM point_ledger_entries p WHERE p.id=s.ledger_entry_id AND p.brand_id=$1) AS missing FROM commission_payment_targets s WHERE s.brand_id=$1 AND s.ledger_entry_id IS NOT NULL UNION ALL SELECT ''commission_payments''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_payments_approval_audit_log_id_fkey''::text AS reference_key, ''audit_logs''::text AS parent_table, NOT EXISTS (SELECT 1 FROM audit_logs p WHERE p.id=s.approval_audit_log_id AND p.brand_id=$1) AS missing FROM commission_payments s WHERE s.brand_id=$1 AND s.approval_audit_log_id IS NOT NULL UNION ALL SELECT ''commission_payments''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_payments_approved_by_fkey''::text AS reference_key, ''admin_accounts''::text AS parent_table, NOT EXISTS (SELECT 1 FROM admin_accounts p WHERE p.id=s.approved_by) AS missing FROM commission_payments s WHERE s.brand_id=$1 AND s.approved_by IS NOT NULL UNION ALL SELECT ''commission_payments''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_payments_brand_id_cycle_id_run_id_fkey''::text AS reference_key, ''commission_runs''::text AS parent_table, NOT EXISTS (SELECT 1 FROM commission_runs p WHERE p.brand_id=s.brand_id AND p.cycle_id=s.cycle_id AND p.id=s.run_id AND p.brand_id=$1) AS missing FROM commission_payments s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.cycle_id IS NOT NULL AND s.run_id IS NOT NULL UNION ALL SELECT ''commission_payments''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_payments_creation_audit_log_id_fkey''::text AS reference_key, ''audit_logs''::text AS parent_table, NOT EXISTS (SELECT 1 FROM audit_logs p WHERE p.id=s.creation_audit_log_id AND p.brand_id=$1) AS missing FROM commission_payments s WHERE s.brand_id=$1 AND s.creation_audit_log_id IS NOT NULL UNION ALL SELECT ''commission_payments''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_payments_last_audit_log_id_fkey''::text AS reference_key, ''audit_logs''::text AS parent_table, NOT EXISTS (SELECT 1 FROM audit_logs p WHERE p.id=s.last_audit_log_id AND p.brand_id=$1) AS missing FROM commission_payments s WHERE s.brand_id=$1 AND s.last_audit_log_id IS NOT NULL UNION ALL SELECT ''commission_runs''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''commission_runs_brand_id_cycle_id_fkey''::text AS reference_key, ''commission_cycles''::text AS parent_table, NOT EXISTS (SELECT 1 FROM commission_cycles p WHERE p.brand_id=s.brand_id AND p.id=s.cycle_id AND p.brand_id=$1) AS missing FROM commission_runs s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.cycle_id IS NOT NULL UNION ALL SELECT ''draw_correction_failures''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''draw_correction_failures_brand_id_correction_id_fkey''::text AS reference_key, ''draw_corrections''::text AS parent_table, NOT EXISTS (SELECT 1 FROM draw_corrections p WHERE p.brand_id=s.brand_id AND p.id=s.correction_id AND p.brand_id=$1) AS missing FROM draw_correction_failures s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.correction_id IS NOT NULL UNION ALL SELECT ''draw_correction_failures''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''draw_correction_failures_correction_id_order_id_fkey''::text AS reference_key, ''draw_correction_targets''::text AS parent_table, NOT EXISTS (SELECT 1 FROM draw_correction_targets p WHERE p.correction_id=s.correction_id AND p.order_id=s.order_id AND p.brand_id=$1) AS missing FROM draw_correction_failures s WHERE s.brand_id=$1 AND s.correction_id IS NOT NULL AND s.order_id IS NOT NULL UNION ALL SELECT ''draw_correction_targets''::text AS source_table, concat_ws(''/'',s.correction_id::text,s.order_id::text) AS source_id, ''draw_correction_targets_brand_id_old_payout_entry_id_fkey''::text AS reference_key, ''point_ledger_entries''::text AS parent_table, NOT EXISTS (SELECT 1 FROM point_ledger_entries p WHERE p.brand_id=s.brand_id AND p.id=s.old_payout_entry_id AND p.brand_id=$1) AS missing FROM draw_correction_targets s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.old_payout_entry_id IS NOT NULL UNION ALL SELECT ''draw_correction_targets''::text AS source_table, concat_ws(''/'',s.correction_id::text,s.order_id::text) AS source_id, ''draw_correction_targets_brand_id_order_id_old_calculation__fkey''::text AS reference_key, ''settlement_calculations''::text AS parent_table, NOT EXISTS (SELECT 1 FROM settlement_calculations p WHERE p.brand_id=s.brand_id AND p.order_id=s.order_id AND p.id=s.old_calculation_id AND p.brand_id=$1) AS missing FROM draw_correction_targets s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.order_id IS NOT NULL AND s.old_calculation_id IS NOT NULL UNION ALL SELECT ''draw_correction_targets''::text AS source_table, concat_ws(''/'',s.correction_id::text,s.order_id::text) AS source_id, ''draw_correction_targets_brand_id_period_id_correction_id_fkey''::text AS reference_key, ''draw_corrections''::text AS parent_table, NOT EXISTS (SELECT 1 FROM draw_corrections p WHERE p.brand_id=s.brand_id AND p.period_id=s.period_id AND p.id=s.correction_id AND p.brand_id=$1) AS missing FROM draw_correction_targets s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.period_id IS NOT NULL AND s.correction_id IS NOT NULL UNION ALL SELECT ''draw_correction_targets''::text AS source_table, concat_ws(''/'',s.correction_id::text,s.order_id::text) AS source_id, ''draw_correction_targets_brand_id_period_id_order_id_fkey''::text AS reference_key, ''bet_orders''::text AS parent_table, NOT EXISTS (SELECT 1 FROM bet_orders p WHERE p.brand_id=s.brand_id AND p.period_id=s.period_id AND p.id=s.order_id AND p.brand_id=$1) AS missing FROM draw_correction_targets s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.period_id IS NOT NULL AND s.order_id IS NOT NULL UNION ALL SELECT ''draw_correction_targets''::text AS source_table, concat_ws(''/'',s.correction_id::text,s.order_id::text) AS source_id, ''draw_correction_targets_brand_id_reversal_entry_id_fkey''::text AS reference_key, ''point_ledger_entries''::text AS parent_table, NOT EXISTS (SELECT 1 FROM point_ledger_entries p WHERE p.brand_id=s.brand_id AND p.id=s.reversal_entry_id AND p.brand_id=$1) AS missing FROM draw_correction_targets s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.reversal_entry_id IS NOT NULL UNION ALL SELECT ''draw_corrections''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''draw_corrections_brand_id_game_id_period_id_draw_result_id_fkey''::text AS reference_key, ''draw_results''::text AS parent_table, NOT EXISTS (SELECT 1 FROM draw_results p WHERE p.brand_id=s.brand_id AND p.game_id=s.game_id AND p.period_id=s.period_id AND p.id=s.draw_result_id AND p.brand_id=$1) AS missing FROM draw_corrections s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.game_id IS NOT NULL AND s.period_id IS NOT NULL AND s.draw_result_id IS NOT NULL UNION ALL SELECT ''draw_corrections''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''draw_corrections_brand_id_game_id_period_id_previous_draw__fkey''::text AS reference_key, ''draw_results''::text AS parent_table, NOT EXISTS (SELECT 1 FROM draw_results p WHERE p.brand_id=s.brand_id AND p.game_id=s.game_id AND p.period_id=s.period_id AND p.id=s.previous_draw_result_id AND p.brand_id=$1) AS missing FROM draw_corrections s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.game_id IS NOT NULL AND s.period_id IS NOT NULL AND s.previous_draw_result_id IS NOT NULL UNION ALL SELECT ''draw_corrections''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''draw_corrections_brand_id_period_id_new_job_id_fkey''::text AS reference_key, ''settlement_jobs''::text AS parent_table, NOT EXISTS (SELECT 1 FROM settlement_jobs p WHERE p.brand_id=s.brand_id AND p.period_id=s.period_id AND p.id=s.new_job_id AND p.brand_id=$1) AS missing FROM draw_corrections s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.period_id IS NOT NULL AND s.new_job_id IS NOT NULL UNION ALL SELECT ''draw_corrections''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''draw_corrections_brand_id_period_id_previous_job_id_fkey''::text AS reference_key, ''settlement_jobs''::text AS parent_table, NOT EXISTS (SELECT 1 FROM settlement_jobs p WHERE p.brand_id=s.brand_id AND p.period_id=s.period_id AND p.id=s.previous_job_id AND p.brand_id=$1) AS missing FROM draw_corrections s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.period_id IS NOT NULL AND s.previous_job_id IS NOT NULL UNION ALL SELECT ''draw_corrections''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''draw_corrections_brand_id_policy_version_fkey''::text AS reference_key, ''settlement_policy_history''::text AS parent_table, NOT EXISTS (SELECT 1 FROM settlement_policy_history p WHERE p.brand_id=s.brand_id AND p.version=s.policy_version AND p.brand_id=$1) AS missing FROM draw_corrections s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.policy_version IS NOT NULL UNION ALL SELECT ''draw_corrections''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''draw_corrections_created_by_fkey''::text AS reference_key, ''admin_accounts''::text AS parent_table, NOT EXISTS (SELECT 1 FROM admin_accounts p WHERE p.id=s.created_by) AS missing FROM draw_corrections s WHERE s.brand_id=$1 AND s.created_by IS NOT NULL UNION ALL SELECT ''period_cancellation_targets''::text AS source_table, concat_ws(''/'',s.cancellation_id::text,s.order_id::text) AS source_id, ''period_cancellation_targets_brand_id_period_id_cancellatio_fkey''::text AS reference_key, ''period_cancellations''::text AS parent_table, NOT EXISTS (SELECT 1 FROM period_cancellations p WHERE p.brand_id=s.brand_id AND p.period_id=s.period_id AND p.id=s.cancellation_id AND p.brand_id=$1) AS missing FROM period_cancellation_targets s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.period_id IS NOT NULL AND s.cancellation_id IS NOT NULL UNION ALL SELECT ''period_cancellation_targets''::text AS source_table, concat_ws(''/'',s.cancellation_id::text,s.order_id::text) AS source_id, ''period_cancellation_targets_brand_id_period_id_order_id_fkey''::text AS reference_key, ''bet_orders''::text AS parent_table, NOT EXISTS (SELECT 1 FROM bet_orders p WHERE p.brand_id=s.brand_id AND p.period_id=s.period_id AND p.id=s.order_id AND p.brand_id=$1) AS missing FROM period_cancellation_targets s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.period_id IS NOT NULL AND s.order_id IS NOT NULL UNION ALL SELECT ''period_cancellation_targets''::text AS source_table, concat_ws(''/'',s.cancellation_id::text,s.order_id::text) AS source_id, ''period_cancellation_targets_refund_entry_id_fkey''::text AS reference_key, ''point_ledger_entries''::text AS parent_table, NOT EXISTS (SELECT 1 FROM point_ledger_entries p WHERE p.id=s.refund_entry_id AND p.brand_id=$1) AS missing FROM period_cancellation_targets s WHERE s.brand_id=$1 AND s.refund_entry_id IS NOT NULL UNION ALL SELECT ''period_cancellations''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''period_cancellations_brand_id_game_id_period_id_draw_resul_fkey''::text AS reference_key, ''draw_results''::text AS parent_table, NOT EXISTS (SELECT 1 FROM draw_results p WHERE p.brand_id=s.brand_id AND p.game_id=s.game_id AND p.period_id=s.period_id AND p.id=s.draw_result_id AND p.brand_id=$1) AS missing FROM period_cancellations s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.game_id IS NOT NULL AND s.period_id IS NOT NULL AND s.draw_result_id IS NOT NULL UNION ALL SELECT ''period_cancellations''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''period_cancellations_brand_id_game_id_period_id_fkey''::text AS reference_key, ''periods''::text AS parent_table, NOT EXISTS (SELECT 1 FROM periods p WHERE p.brand_id=s.brand_id AND p.game_id=s.game_id AND p.id=s.period_id AND p.brand_id=$1) AS missing FROM period_cancellations s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.game_id IS NOT NULL AND s.period_id IS NOT NULL UNION ALL SELECT ''period_cancellations''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''period_cancellations_created_by_fkey''::text AS reference_key, ''admin_accounts''::text AS parent_table, NOT EXISTS (SELECT 1 FROM admin_accounts p WHERE p.id=s.created_by) AS missing FROM period_cancellations s WHERE s.brand_id=$1 AND s.created_by IS NOT NULL UNION ALL SELECT ''point_accounts''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''point_accounts_brand_id_brand_member_id_fkey''::text AS reference_key, ''brand_members''::text AS parent_table, NOT EXISTS (SELECT 1 FROM brand_members p WHERE p.brand_id=s.brand_id AND p.id=s.brand_member_id AND p.brand_id=$1) AS missing FROM point_accounts s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.brand_member_id IS NOT NULL UNION ALL SELECT ''point_buckets''::text AS source_table, concat_ws(''/'',s.brand_id::text,s.account_id::text,s.source::text,s.state::text) AS source_id, ''point_buckets_brand_id_account_id_fkey''::text AS reference_key, ''point_accounts''::text AS parent_table, NOT EXISTS (SELECT 1 FROM point_accounts p WHERE p.brand_id=s.brand_id AND p.id=s.account_id AND p.brand_id=$1) AS missing FROM point_buckets s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.account_id IS NOT NULL UNION ALL SELECT ''point_ledger_entries''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''ledger_account_member_fk''::text AS reference_key, ''point_accounts''::text AS parent_table, NOT EXISTS (SELECT 1 FROM point_accounts p WHERE p.brand_id=s.brand_id AND p.id=s.account_id AND p.brand_member_id=s.member_id AND p.brand_id=$1) AS missing FROM point_ledger_entries s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.account_id IS NOT NULL AND s.member_id IS NOT NULL UNION ALL SELECT ''point_ledger_entries''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''ledger_reversal_same_brand''::text AS reference_key, ''point_ledger_entries''::text AS parent_table, NOT EXISTS (SELECT 1 FROM point_ledger_entries p WHERE p.brand_id=s.brand_id AND p.account_id=s.account_id AND p.id=s.reversal_of AND p.brand_id=$1) AS missing FROM point_ledger_entries s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.account_id IS NOT NULL AND s.reversal_of IS NOT NULL UNION ALL SELECT ''point_ledger_entries''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''point_ledger_entries_brand_id_account_id_fkey''::text AS reference_key, ''point_accounts''::text AS parent_table, NOT EXISTS (SELECT 1 FROM point_accounts p WHERE p.brand_id=s.brand_id AND p.id=s.account_id AND p.brand_id=$1) AS missing FROM point_ledger_entries s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.account_id IS NOT NULL UNION ALL SELECT ''point_ledger_entries''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''point_ledger_entries_reversal_of_fkey''::text AS reference_key, ''point_ledger_entries''::text AS parent_table, NOT EXISTS (SELECT 1 FROM point_ledger_entries p WHERE p.id=s.reversal_of AND p.brand_id=$1) AS missing FROM point_ledger_entries s WHERE s.brand_id=$1 AND s.reversal_of IS NOT NULL UNION ALL SELECT ''recharge_orders''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''recharge_orders_brand_id_account_id_ledger_entry_id_fkey''::text AS reference_key, ''point_ledger_entries''::text AS parent_table, NOT EXISTS (SELECT 1 FROM point_ledger_entries p WHERE p.brand_id=s.brand_id AND p.account_id=s.account_id AND p.id=s.ledger_entry_id AND p.brand_id=$1) AS missing FROM recharge_orders s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.account_id IS NOT NULL AND s.ledger_entry_id IS NOT NULL UNION ALL SELECT ''recharge_orders''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''recharge_orders_brand_id_account_id_member_id_fkey''::text AS reference_key, ''point_accounts''::text AS parent_table, NOT EXISTS (SELECT 1 FROM point_accounts p WHERE p.brand_id=s.brand_id AND p.id=s.account_id AND p.brand_member_id=s.member_id AND p.brand_id=$1) AS missing FROM recharge_orders s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.account_id IS NOT NULL AND s.member_id IS NOT NULL UNION ALL SELECT ''recharge_orders''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''recharge_orders_confirmed_by_fkey''::text AS reference_key, ''admin_accounts''::text AS parent_table, NOT EXISTS (SELECT 1 FROM admin_accounts p WHERE p.id=s.confirmed_by) AS missing FROM recharge_orders s WHERE s.brand_id=$1 AND s.confirmed_by IS NOT NULL UNION ALL SELECT ''recharge_orders''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''recharge_orders_created_by_fkey''::text AS reference_key, ''admin_accounts''::text AS parent_table, NOT EXISTS (SELECT 1 FROM admin_accounts p WHERE p.id=s.created_by) AS missing FROM recharge_orders s WHERE s.brand_id=$1 AND s.created_by IS NOT NULL UNION ALL SELECT ''reward_order_actions''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''reward_order_actions_actor_id_fkey''::text AS reference_key, ''admin_accounts''::text AS parent_table, NOT EXISTS (SELECT 1 FROM admin_accounts p WHERE p.id=s.actor_id) AS missing FROM reward_order_actions s WHERE s.brand_id=$1 AND s.actor_id IS NOT NULL UNION ALL SELECT ''reward_order_actions''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''reward_order_actions_audit_log_id_fkey''::text AS reference_key, ''audit_logs''::text AS parent_table, NOT EXISTS (SELECT 1 FROM audit_logs p WHERE p.id=s.audit_log_id AND p.brand_id=$1) AS missing FROM reward_order_actions s WHERE s.brand_id=$1 AND s.audit_log_id IS NOT NULL UNION ALL SELECT ''reward_order_actions''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''reward_order_actions_brand_id_order_id_fkey''::text AS reference_key, ''reward_orders''::text AS parent_table, NOT EXISTS (SELECT 1 FROM reward_orders p WHERE p.brand_id=s.brand_id AND p.id=s.order_id AND p.brand_id=$1) AS missing FROM reward_order_actions s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.order_id IS NOT NULL UNION ALL SELECT ''reward_order_actions''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''reward_order_actions_ledger_entry_id_fkey''::text AS reference_key, ''point_ledger_entries''::text AS parent_table, NOT EXISTS (SELECT 1 FROM point_ledger_entries p WHERE p.id=s.ledger_entry_id AND p.brand_id=$1) AS missing FROM reward_order_actions s WHERE s.brand_id=$1 AND s.ledger_entry_id IS NOT NULL UNION ALL SELECT ''reward_orders''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''reward_orders_brand_id_member_id_fkey''::text AS reference_key, ''brand_members''::text AS parent_table, NOT EXISTS (SELECT 1 FROM brand_members p WHERE p.brand_id=s.brand_id AND p.id=s.member_id AND p.brand_id=$1) AS missing FROM reward_orders s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.member_id IS NOT NULL UNION ALL SELECT ''reward_orders''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''reward_orders_created_by_fkey''::text AS reference_key, ''admin_accounts''::text AS parent_table, NOT EXISTS (SELECT 1 FROM admin_accounts p WHERE p.id=s.created_by) AS missing FROM reward_orders s WHERE s.brand_id=$1 AND s.created_by IS NOT NULL UNION ALL SELECT ''reward_orders''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''reward_orders_creation_audit_log_id_fkey''::text AS reference_key, ''audit_logs''::text AS parent_table, NOT EXISTS (SELECT 1 FROM audit_logs p WHERE p.id=s.creation_audit_log_id AND p.brand_id=$1) AS missing FROM reward_orders s WHERE s.brand_id=$1 AND s.creation_audit_log_id IS NOT NULL UNION ALL SELECT ''reward_orders''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''reward_orders_grant_ledger_entry_id_fkey''::text AS reference_key, ''point_ledger_entries''::text AS parent_table, NOT EXISTS (SELECT 1 FROM point_ledger_entries p WHERE p.id=s.grant_ledger_entry_id AND p.brand_id=$1) AS missing FROM reward_orders s WHERE s.brand_id=$1 AND s.grant_ledger_entry_id IS NOT NULL UNION ALL SELECT ''reward_orders''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''reward_orders_last_audit_log_id_fkey''::text AS reference_key, ''audit_logs''::text AS parent_table, NOT EXISTS (SELECT 1 FROM audit_logs p WHERE p.id=s.last_audit_log_id AND p.brand_id=$1) AS missing FROM reward_orders s WHERE s.brand_id=$1 AND s.last_audit_log_id IS NOT NULL UNION ALL SELECT ''reward_orders''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''reward_orders_revoke_ledger_entry_id_fkey''::text AS reference_key, ''point_ledger_entries''::text AS parent_table, NOT EXISTS (SELECT 1 FROM point_ledger_entries p WHERE p.id=s.revoke_ledger_entry_id AND p.brand_id=$1) AS missing FROM reward_orders s WHERE s.brand_id=$1 AND s.revoke_ledger_entry_id IS NOT NULL UNION ALL SELECT ''settlement_calculations''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''settlement_calculations_brand_id_period_id_job_id_fkey''::text AS reference_key, ''settlement_jobs''::text AS parent_table, NOT EXISTS (SELECT 1 FROM settlement_jobs p WHERE p.brand_id=s.brand_id AND p.period_id=s.period_id AND p.id=s.job_id AND p.brand_id=$1) AS missing FROM settlement_calculations s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.period_id IS NOT NULL AND s.job_id IS NOT NULL UNION ALL SELECT ''settlement_calculations''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''settlement_calculations_brand_id_period_id_order_id_fkey''::text AS reference_key, ''bet_orders''::text AS parent_table, NOT EXISTS (SELECT 1 FROM bet_orders p WHERE p.brand_id=s.brand_id AND p.period_id=s.period_id AND p.id=s.order_id AND p.brand_id=$1) AS missing FROM settlement_calculations s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.period_id IS NOT NULL AND s.order_id IS NOT NULL UNION ALL SELECT ''settlement_failures''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''settlement_failures_brand_id_job_id_fkey''::text AS reference_key, ''settlement_jobs''::text AS parent_table, NOT EXISTS (SELECT 1 FROM settlement_jobs p WHERE p.brand_id=s.brand_id AND p.id=s.job_id AND p.brand_id=$1) AS missing FROM settlement_failures s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.job_id IS NOT NULL UNION ALL SELECT ''settlement_failures''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''settlement_failures_job_id_order_id_fkey''::text AS reference_key, ''settlement_targets''::text AS parent_table, NOT EXISTS (SELECT 1 FROM settlement_targets p WHERE p.job_id=s.job_id AND p.order_id=s.order_id AND p.brand_id=$1) AS missing FROM settlement_failures s WHERE s.brand_id=$1 AND s.job_id IS NOT NULL AND s.order_id IS NOT NULL UNION ALL SELECT ''settlement_jobs''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''settlement_jobs_approved_by_fkey''::text AS reference_key, ''admin_accounts''::text AS parent_table, NOT EXISTS (SELECT 1 FROM admin_accounts p WHERE p.id=s.approved_by) AS missing FROM settlement_jobs s WHERE s.brand_id=$1 AND s.approved_by IS NOT NULL UNION ALL SELECT ''settlement_jobs''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''settlement_jobs_brand_id_game_id_period_id_draw_result_id_fkey''::text AS reference_key, ''draw_results''::text AS parent_table, NOT EXISTS (SELECT 1 FROM draw_results p WHERE p.brand_id=s.brand_id AND p.game_id=s.game_id AND p.period_id=s.period_id AND p.id=s.draw_result_id AND p.brand_id=$1) AS missing FROM settlement_jobs s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.game_id IS NOT NULL AND s.period_id IS NOT NULL AND s.draw_result_id IS NOT NULL UNION ALL SELECT ''settlement_jobs''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''settlement_jobs_brand_id_period_id_correction_id_fkey''::text AS reference_key, ''draw_corrections''::text AS parent_table, NOT EXISTS (SELECT 1 FROM draw_corrections p WHERE p.brand_id=s.brand_id AND p.period_id=s.period_id AND p.id=s.correction_id AND p.brand_id=$1) AS missing FROM settlement_jobs s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.period_id IS NOT NULL AND s.correction_id IS NOT NULL UNION ALL SELECT ''settlement_jobs''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''settlement_jobs_brand_id_period_id_previous_job_id_fkey''::text AS reference_key, ''settlement_jobs''::text AS parent_table, NOT EXISTS (SELECT 1 FROM settlement_jobs p WHERE p.brand_id=s.brand_id AND p.period_id=s.period_id AND p.id=s.previous_job_id AND p.brand_id=$1) AS missing FROM settlement_jobs s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.period_id IS NOT NULL AND s.previous_job_id IS NOT NULL UNION ALL SELECT ''settlement_jobs''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''settlement_jobs_brand_id_policy_version_fkey''::text AS reference_key, ''settlement_policy_history''::text AS parent_table, NOT EXISTS (SELECT 1 FROM settlement_policy_history p WHERE p.brand_id=s.brand_id AND p.version=s.policy_version AND p.brand_id=$1) AS missing FROM settlement_jobs s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.policy_version IS NOT NULL UNION ALL SELECT ''settlement_jobs''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''settlement_jobs_created_by_fkey''::text AS reference_key, ''admin_accounts''::text AS parent_table, NOT EXISTS (SELECT 1 FROM admin_accounts p WHERE p.id=s.created_by) AS missing FROM settlement_jobs s WHERE s.brand_id=$1 AND s.created_by IS NOT NULL UNION ALL SELECT ''settlement_targets''::text AS source_table, concat_ws(''/'',s.job_id::text,s.order_id::text) AS source_id, ''settlement_targets_brand_id_order_id_calculation_id_fkey''::text AS reference_key, ''settlement_calculations''::text AS parent_table, NOT EXISTS (SELECT 1 FROM settlement_calculations p WHERE p.brand_id=s.brand_id AND p.order_id=s.order_id AND p.id=s.calculation_id AND p.brand_id=$1) AS missing FROM settlement_targets s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.order_id IS NOT NULL AND s.calculation_id IS NOT NULL UNION ALL SELECT ''settlement_targets''::text AS source_table, concat_ws(''/'',s.job_id::text,s.order_id::text) AS source_id, ''settlement_targets_brand_id_period_id_job_id_fkey''::text AS reference_key, ''settlement_jobs''::text AS parent_table, NOT EXISTS (SELECT 1 FROM settlement_jobs p WHERE p.brand_id=s.brand_id AND p.period_id=s.period_id AND p.id=s.job_id AND p.brand_id=$1) AS missing FROM settlement_targets s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.period_id IS NOT NULL AND s.job_id IS NOT NULL UNION ALL SELECT ''settlement_targets''::text AS source_table, concat_ws(''/'',s.job_id::text,s.order_id::text) AS source_id, ''settlement_targets_brand_id_period_id_order_id_fkey''::text AS reference_key, ''bet_orders''::text AS parent_table, NOT EXISTS (SELECT 1 FROM bet_orders p WHERE p.brand_id=s.brand_id AND p.period_id=s.period_id AND p.id=s.order_id AND p.brand_id=$1) AS missing FROM settlement_targets s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.period_id IS NOT NULL AND s.order_id IS NOT NULL UNION ALL SELECT ''settlement_targets''::text AS source_table, concat_ws(''/'',s.job_id::text,s.order_id::text) AS source_id, ''settlement_targets_payout_entry_id_fkey''::text AS reference_key, ''point_ledger_entries''::text AS parent_table, NOT EXISTS (SELECT 1 FROM point_ledger_entries p WHERE p.id=s.payout_entry_id AND p.brand_id=$1) AS missing FROM settlement_targets s WHERE s.brand_id=$1 AND s.payout_entry_id IS NOT NULL UNION ALL SELECT ''withdrawal_operation_receipts''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''withdrawal_operation_receipts_brand_id_order_id_fkey''::text AS reference_key, ''withdrawal_orders''::text AS parent_table, NOT EXISTS (SELECT 1 FROM withdrawal_orders p WHERE p.brand_id=s.brand_id AND p.id=s.order_id AND p.brand_id=$1) AS missing FROM withdrawal_operation_receipts s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.order_id IS NOT NULL UNION ALL SELECT ''withdrawal_order_transitions''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''withdrawal_order_transitions_audit_log_id_fkey''::text AS reference_key, ''audit_logs''::text AS parent_table, NOT EXISTS (SELECT 1 FROM audit_logs p WHERE p.id=s.audit_log_id AND p.brand_id=$1) AS missing FROM withdrawal_order_transitions s WHERE s.brand_id=$1 AND s.audit_log_id IS NOT NULL UNION ALL SELECT ''withdrawal_order_transitions''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''withdrawal_order_transitions_brand_id_order_id_fkey''::text AS reference_key, ''withdrawal_orders''::text AS parent_table, NOT EXISTS (SELECT 1 FROM withdrawal_orders p WHERE p.brand_id=s.brand_id AND p.id=s.order_id AND p.brand_id=$1) AS missing FROM withdrawal_order_transitions s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.order_id IS NOT NULL UNION ALL SELECT ''withdrawal_orders''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''withdrawal_orders_brand_id_account_id_member_id_fkey''::text AS reference_key, ''point_accounts''::text AS parent_table, NOT EXISTS (SELECT 1 FROM point_accounts p WHERE p.brand_id=s.brand_id AND p.id=s.account_id AND p.brand_member_id=s.member_id AND p.brand_id=$1) AS missing FROM withdrawal_orders s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.account_id IS NOT NULL AND s.member_id IS NOT NULL UNION ALL SELECT ''withdrawal_orders''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''withdrawal_orders_brand_id_account_id_paid_entry_id_fkey''::text AS reference_key, ''point_ledger_entries''::text AS parent_table, NOT EXISTS (SELECT 1 FROM point_ledger_entries p WHERE p.brand_id=s.brand_id AND p.account_id=s.account_id AND p.id=s.paid_entry_id AND p.brand_id=$1) AS missing FROM withdrawal_orders s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.account_id IS NOT NULL AND s.paid_entry_id IS NOT NULL UNION ALL SELECT ''withdrawal_orders''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''withdrawal_orders_brand_id_account_id_release_entry_id_fkey''::text AS reference_key, ''point_ledger_entries''::text AS parent_table, NOT EXISTS (SELECT 1 FROM point_ledger_entries p WHERE p.brand_id=s.brand_id AND p.account_id=s.account_id AND p.id=s.release_entry_id AND p.brand_id=$1) AS missing FROM withdrawal_orders s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.account_id IS NOT NULL AND s.release_entry_id IS NOT NULL UNION ALL SELECT ''withdrawal_orders''::text AS source_table, concat_ws(''/'',s.id::text) AS source_id, ''withdrawal_orders_brand_id_account_id_reserve_entry_id_fkey''::text AS reference_key, ''point_ledger_entries''::text AS parent_table, NOT EXISTS (SELECT 1 FROM point_ledger_entries p WHERE p.brand_id=s.brand_id AND p.account_id=s.account_id AND p.id=s.reserve_entry_id AND p.brand_id=$1) AS missing FROM withdrawal_orders s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.account_id IS NOT NULL AND s.reserve_entry_id IS NOT NULL UNION ALL SELECT ''withdrawal_turnover_cycles''::text AS source_table, concat_ws(''/'',s.brand_id::text,s.member_id::text) AS source_id, ''withdrawal_turnover_cycles_brand_id_account_id_member_id_fkey''::text AS reference_key, ''point_accounts''::text AS parent_table, NOT EXISTS (SELECT 1 FROM point_accounts p WHERE p.brand_id=s.brand_id AND p.id=s.account_id AND p.brand_member_id=s.member_id AND p.brand_id=$1) AS missing FROM withdrawal_turnover_cycles s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.account_id IS NOT NULL AND s.member_id IS NOT NULL UNION ALL SELECT ''withdrawal_turnover_cycles''::text AS source_table, concat_ws(''/'',s.brand_id::text,s.member_id::text) AS source_id, ''withdrawal_turnover_cycles_brand_id_last_paid_order_id_fkey''::text AS reference_key, ''withdrawal_orders''::text AS parent_table, NOT EXISTS (SELECT 1 FROM withdrawal_orders p WHERE p.brand_id=s.brand_id AND p.id=s.last_paid_order_id AND p.brand_id=$1) AS missing FROM withdrawal_turnover_cycles s WHERE s.brand_id=$1 AND s.brand_id IS NOT NULL AND s.last_paid_order_id IS NOT NULL) fk_refs
    UNION ALL
    SELECT source_table,source_id,''MISSING_REQUIRED_LEDGER_REFERENCE''::text AS code,
           reference_key,parent_table,missing
      FROM (SELECT ''bet_orders''::text, concat_ws(''/'',s.id::text) AS source_id, ''debit_entry_id''::text, ''point_ledger_entries''::text, true FROM bet_orders s WHERE s.brand_id=$1 AND s.debit_entry_id IS NULL UNION ALL SELECT ''bet_orders''::text, concat_ws(''/'',s.id::text), ''refund_entry_id''::text, ''point_ledger_entries''::text, true FROM bet_orders s WHERE s.brand_id=$1 AND s.status IN (''bet_cancelled'',''judged_cancelled'') AND s.refund_entry_id IS NULL UNION ALL SELECT ''recharge_orders''::text, concat_ws(''/'',s.id::text), ''ledger_entry_id''::text, ''point_ledger_entries''::text, true FROM recharge_orders s WHERE s.brand_id=$1 AND s.state=''confirmed'' AND s.ledger_entry_id IS NULL UNION ALL SELECT ''withdrawal_orders''::text, concat_ws(''/'',s.id::text), ''reserve_entry_id''::text, ''point_ledger_entries''::text, true FROM withdrawal_orders s WHERE s.brand_id=$1 AND s.reserve_entry_id IS NULL UNION ALL SELECT ''withdrawal_orders''::text, concat_ws(''/'',s.id::text), ''paid_entry_id''::text, ''point_ledger_entries''::text, true FROM withdrawal_orders s WHERE s.brand_id=$1 AND s.state=''paid'' AND s.paid_entry_id IS NULL UNION ALL SELECT ''withdrawal_orders''::text, concat_ws(''/'',s.id::text), ''release_entry_id''::text, ''point_ledger_entries''::text, true FROM withdrawal_orders s WHERE s.brand_id=$1 AND s.state IN (''rejected'',''failed'',''cancelled'') AND s.release_entry_id IS NULL UNION ALL SELECT ''reward_orders''::text, concat_ws(''/'',s.id::text), ''grant_ledger_entry_id''::text, ''point_ledger_entries''::text, true FROM reward_orders s WHERE s.brand_id=$1 AND s.state IN (''granted'',''revocation_pending'',''revoked'') AND s.grant_ledger_entry_id IS NULL UNION ALL SELECT ''reward_orders''::text, concat_ws(''/'',s.id::text), ''revoke_ledger_entry_id''::text, ''point_ledger_entries''::text, true FROM reward_orders s WHERE s.brand_id=$1 AND s.state=''revoked'' AND s.revoke_ledger_entry_id IS NULL UNION ALL SELECT ''reward_order_actions''::text, concat_ws(''/'',s.id::text), ''ledger_entry_id''::text, ''point_ledger_entries''::text, true FROM reward_order_actions s WHERE s.brand_id=$1 AND (s.operation=''grant'' OR s.state_after=''revoked'') AND s.ledger_entry_id IS NULL UNION ALL SELECT ''commission_payment_targets''::text, concat_ws(''/'',s.id::text), ''ledger_entry_id''::text, ''point_ledger_entries''::text, true FROM commission_payment_targets s WHERE s.brand_id=$1 AND s.state=''paid'' AND s.points>0 AND s.ledger_entry_id IS NULL UNION ALL SELECT ''commission_adjustments''::text, concat_ws(''/'',s.id::text), ''ledger_entry_id''::text, ''point_ledger_entries''::text, true FROM commission_adjustments s WHERE s.brand_id=$1 AND s.delta_points<>0 AND s.ledger_entry_id IS NULL UNION ALL SELECT ''commission_correction_execution_targets''::text, concat_ws(''/'',s.id::text), ''ledger_entry_id''::text, ''point_ledger_entries''::text, true FROM commission_correction_execution_targets s WHERE s.brand_id=$1 AND s.state=''applied'' AND s.delta_points<>0 AND s.ledger_entry_id IS NULL UNION ALL SELECT ''settlement_targets''::text, concat_ws(''/'',s.job_id::text,s.order_id::text), ''payout_entry_id''::text, ''point_ledger_entries''::text, true FROM settlement_targets s WHERE s.brand_id=$1 AND s.state=''paid'' AND s.payout_entry_id IS NULL AND EXISTS (SELECT 1 FROM settlement_calculations c WHERE c.brand_id=s.brand_id AND c.id=s.calculation_id AND c.prize_points>0) UNION ALL SELECT ''draw_correction_targets''::text, concat_ws(''/'',s.correction_id::text,s.order_id::text), ''old_payout_entry_id''::text, ''point_ledger_entries''::text, true FROM draw_correction_targets s WHERE s.brand_id=$1 AND s.old_prize_points>0 AND s.old_payout_entry_id IS NULL UNION ALL SELECT ''draw_correction_targets''::text, concat_ws(''/'',s.correction_id::text,s.order_id::text), ''reversal_entry_id''::text, ''point_ledger_entries''::text, true FROM draw_correction_targets s WHERE s.brand_id=$1 AND s.state=''reversed'' AND s.old_prize_points>0 AND s.reversal_entry_id IS NULL) AS required_refs(source_table,source_id,reference_key,parent_table,missing)
),
findings AS MATERIALIZED (
    SELECT source_table,source_id,code,reference_key,parent_table
      FROM refs WHERE missing
),
issue_doc AS (
    SELECT coalesce(jsonb_agg(jsonb_build_object(
             ''source_table'',source_table,''source_id'',source_id,''code'',code,
             ''reference_key'',reference_key,''parent_table'',parent_table)
             ORDER BY source_table COLLATE "C",source_id COLLATE "C",
                      reference_key COLLATE "C",code COLLATE "C",parent_table COLLATE "C"),
             ''[]''::jsonb) AS issues
      FROM findings
),
row_material AS (
    SELECT coalesce(string_agg(source_table||''|''||source_id||''|''||row_hash,chr(10)
             ORDER BY source_table COLLATE "C",source_id COLLATE "C"),'''') AS material
      FROM source_rows
),
coverage AS (
    SELECT coalesce(jsonb_agg(jsonb_build_object(
             ''source_table'',t.source_table,
             ''source_row_count'',coalesce(sr.n,0)::text,
             ''reference_count'',coalesce(rr.n,0)::text,
             ''issue_count'',coalesce(ff.n,0)::text)
             ORDER BY t.ordinality), ''[]''::jsonb) AS items
      FROM unnest($2::text[]) WITH ORDINALITY t(source_table,ordinality)
      LEFT JOIN LATERAL (SELECT count(*) AS n FROM source_rows x WHERE x.source_table=t.source_table) sr ON true
      LEFT JOIN LATERAL (SELECT count(*) AS n FROM refs x WHERE x.source_table=t.source_table) rr ON true
      LEFT JOIN LATERAL (SELECT count(*) AS n FROM findings x WHERE x.source_table=t.source_table) ff ON true
),
totals AS (
    SELECT (SELECT count(*) FROM source_rows) AS source_count,
           (SELECT count(*) FROM refs) AS reference_count,
           (SELECT count(*) FROM findings) AS issue_count
),
source_id_check AS (
    SELECT coalesce(bool_or(octet_length(source_id)>500 OR
                    source_id !~ ''^[A-Za-z0-9_.:/-]+$''),false) AS invalid
      FROM source_rows
),
visible AS (
    SELECT coalesce(jsonb_agg(x.value ORDER BY x.ord),''[]''::jsonb) AS issues
      FROM jsonb_array_elements((SELECT issues FROM issue_doc)) WITH ORDINALITY x(value,ord)
     WHERE x.ord<=100
)
SELECT jsonb_build_object(
    ''brand_id'',$1,''snapshot_at'',statement_timestamp(),''schema_version'',1,
    ''source_row_count'',totals.source_count::text,
    ''reference_count'',totals.reference_count::text,
    ''issue_count'',totals.issue_count::text,
    ''issues_truncated'',totals.issue_count>100,
    ''consistent'',totals.issue_count=0,
    ''fingerprint'',encode(pg_catalog.sha256(convert_to(
       $1::text||E''\\n1\\n''||row_material.material||E''\\nF|''||issue_doc.issues::text,''UTF8'')),''hex''),
    ''coverage'',coverage.items,''issues'',visible.issues),
    source_id_check.invalid
  FROM totals CROSS JOIN issue_doc CROSS JOIN row_material CROSS JOIN coverage CROSS JOIN visible CROSS JOIN source_id_check
';
    v_table text;
    v_count bigint;
    v_total bigint := 0;
    v_brand_exists boolean;
    v_out jsonb;
    v_invalid_source_id boolean;
BEGIN
    IF b IS NULL THEN RETURN NULL; END IF;
    EXECUTE format('SELECT EXISTS(SELECT 1 FROM %I.%I WHERE id=$1)',v_schema,'brands')
       INTO v_brand_exists USING b;
    IF NOT v_brand_exists THEN RETURN NULL; END IF;

    -- Cheap bounded prepass: no source rows or parent records are materialized
    -- until the total across all 41 source tables is known to fit.
    FOREACH v_table IN ARRAY v_tables LOOP
        EXECUTE format('SELECT count(*) FROM %I.%I WHERE brand_id=$1',v_schema,v_table)
           INTO v_count USING b;
        v_total := v_total + v_count;
        IF v_total > 100000 THEN
            RAISE EXCEPTION 'brand business inventory exceeds 100000 source rows'
                USING ERRCODE='54000';
        END IF;
    END LOOP;

    EXECUTE v_snapshot_sql INTO v_out,v_invalid_source_id USING b,v_tables;
    IF v_invalid_source_id THEN
        RAISE EXCEPTION 'brand business inventory source_id is outside the documented bounds'
            USING ERRCODE='22023';
    END IF;
    RETURN v_out;
END
$_$;

CREATE FUNCTION capture_bet_commission_snapshot() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE f brand_commission_policies; p brand_agent_policies; m brand_members; n agent_nodes;
 financial_revision uuid; agency_revision uuid; chain jsonb; expected_count int; captured_count int;
BEGIN
 IF TG_OP='UPDATE' THEN
  IF NEW.commission_rule_snapshot IS DISTINCT FROM OLD.commission_rule_snapshot THEN RAISE EXCEPTION 'bet commission snapshot immutable'; END IF;
  RETURN NEW;
 END IF;
 SELECT * INTO f FROM brand_commission_policies WHERE brand_id=NEW.brand_id FOR SHARE;
 IF NOT FOUND THEN RAISE EXCEPTION 'commission financial policy missing'; END IF;
 SELECT * INTO p FROM brand_agent_policies WHERE brand_id=NEW.brand_id FOR SHARE;
 IF NOT FOUND THEN RAISE EXCEPTION 'commission agency policy missing'; END IF;
 SELECT id INTO financial_revision FROM commission_policy_revisions WHERE brand_id=NEW.brand_id AND version=f.version AND config=f.config AND created_at<=NEW.placed_at;
 IF NOT FOUND THEN RAISE EXCEPTION 'commission financial revision missing'; END IF;
 SELECT id INTO agency_revision FROM agent_config_revisions WHERE brand_id=NEW.brand_id AND agent_id IS NULL AND version=p.version AND config=p.config AND created_at<=NEW.placed_at;
 IF NOT FOUND THEN RAISE EXCEPTION 'commission agency revision missing'; END IF;
 IF f.config->'enabled'='true'::jsonb AND (p.config->'enabled'<>'true'::jsonb OR f.config->'calendar'->>'cycle' IS DISTINCT FROM p.config->>'cycle') THEN
  RAISE EXCEPTION 'commission calendar inconsistent'; END IF;
 SELECT * INTO m FROM brand_members WHERE brand_id=NEW.brand_id AND id=NEW.brand_member_id;
 IF NOT FOUND THEN RAISE EXCEPTION 'commission order member missing'; END IF;
 chain:='[]'::jsonb;
 IF m.attribution_snapshot->>'agent_id' IS NOT NULL THEN
  SELECT * INTO n FROM agent_nodes WHERE brand_id=NEW.brand_id AND id=(m.attribution_snapshot->>'agent_id')::uuid;
  IF NOT FOUND THEN RAISE EXCEPTION 'commission agent lineage missing'; END IF;
  expected_count:=cardinality(n.path);
  SELECT count(*),jsonb_agg(jsonb_build_object('id',a.id,'member_id',a.member_id,'parent_id',a.parent_id,'depth',a.depth,
   'revision_id',r.id,'version',a.version::text,'config',a.config,
   'effective_mode',agent_effective_mode_for_path(a.brand_id,a.path,NULL,NULL,p.config->>'mode')) ORDER BY a.depth)
  INTO captured_count,chain FROM agent_nodes a JOIN agent_config_revisions r ON r.brand_id=a.brand_id AND r.agent_id=a.id AND r.version=a.version AND r.config=a.config AND r.created_at<=NEW.placed_at
   WHERE a.brand_id=NEW.brand_id AND a.id=ANY(n.path);
  IF captured_count<>expected_count THEN RAISE EXCEPTION 'commission agent revision missing'; END IF;
  IF EXISTS(SELECT 1 FROM agent_nodes a JOIN agent_nodes parent ON parent.brand_id=a.brand_id AND parent.id=a.parent_id
   WHERE a.brand_id=NEW.brand_id AND a.id=ANY(n.path) AND
    agent_effective_mode_for_path(a.brand_id,a.path,NULL,NULL,p.config->>'mode') IS DISTINCT FROM
    agent_effective_mode_for_path(parent.brand_id,parent.path,NULL,NULL,p.config->>'mode')) THEN RAISE EXCEPTION 'commission chain mode mismatch'; END IF;
 END IF;
 NEW.commission_rule_snapshot:=jsonb_build_object('schema_version',1,'brand_id',NEW.brand_id,'member_id',NEW.brand_member_id,
  'captured_at',NEW.placed_at,'financial_policy',jsonb_build_object('revision_id',financial_revision,'version',f.version::text,'config',f.config),
  'agency_policy',jsonb_build_object('revision_id',agency_revision,'version',p.version::text,'config',p.config),'agent_path',chain);
 RETURN NEW;
END $$;

CREATE FUNCTION capture_bet_withdrawal_snapshot() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
 brand_policy brand_withdrawal_policies;
 game_policy game_withdrawal_policies;
 effective_config jsonb;
 brand_revision uuid;
 game_revision uuid;
BEGIN
 SELECT * INTO brand_policy FROM brand_withdrawal_policies
  WHERE brand_id=NEW.brand_id FOR SHARE;
 IF NOT FOUND THEN RAISE EXCEPTION 'withdrawal snapshot policy unavailable'; END IF;
 SELECT * INTO game_policy FROM game_withdrawal_policies
  WHERE brand_id=NEW.brand_id AND game_id=NEW.game_id FOR SHARE;
 IF NOT FOUND THEN RAISE EXCEPTION 'withdrawal snapshot game policy unavailable'; END IF;

 IF game_policy.config->'turnover_multiple'<>'null'::jsonb THEN
  effective_config:=game_policy.config;
 ELSE
  effective_config:=brand_policy.config;
 END IF;
 IF NOT valid_positive_withdrawal_multiple(effective_config->'turnover_multiple') THEN
  RAISE EXCEPTION 'invalid effective withdrawal multiple';
 END IF;
 SELECT id INTO brand_revision FROM withdrawal_policy_revisions
  WHERE brand_id=NEW.brand_id AND game_id IS NULL AND version=brand_policy.version
    AND config=brand_policy.config;
 IF NOT FOUND THEN RAISE EXCEPTION 'matching brand withdrawal revision unavailable'; END IF;
 SELECT id INTO game_revision FROM withdrawal_policy_revisions
  WHERE brand_id=NEW.brand_id AND game_id=NEW.game_id AND version=game_policy.version
    AND config=game_policy.config;
 IF NOT FOUND THEN RAISE EXCEPTION 'matching game withdrawal revision unavailable'; END IF;

 IF game_policy.config->'turnover_multiple'<>'null'::jsonb THEN
  NEW.withdrawal_rule_snapshot:=jsonb_build_object(
   'schema_version',1,
   'multiple',game_policy.config->>'turnover_multiple',
   'source','game',
   'brand_version',brand_policy.version::text,
   'game_version',game_policy.version::text,
   'brand_revision_id',brand_revision::text,
   'game_revision_id',game_revision::text
  );
 ELSE
  NEW.withdrawal_rule_snapshot:=jsonb_build_object(
   'schema_version',1,
   'multiple',brand_policy.config->>'turnover_multiple',
   'source','brand',
   'brand_version',brand_policy.version::text,
   'game_version',game_policy.version::text,
   'brand_revision_id',brand_revision::text,
   'game_revision_id',game_revision::text
  );
 END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION capture_commission_correction_balance_head() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE original uuid;
BEGIN
 IF OLD.state='pending' AND NEW.state='applied' THEN
  SELECT original_target_id INTO original FROM commission_correction_plan_targets WHERE brand_id=NEW.brand_id AND id=NEW.plan_target_id;
  IF EXISTS(SELECT 1 FROM commission_correction_balance_heads WHERE cycle_id=NEW.cycle_id AND agent_id=NEW.agent_id) THEN
   UPDATE commission_correction_balance_heads SET version=NEW.financial_version,points=NEW.points_after,last_execution_target_id=NEW.id WHERE cycle_id=NEW.cycle_id AND agent_id=NEW.agent_id;
  ELSE
   INSERT INTO commission_correction_balance_heads(brand_id,cycle_id,agent_id,member_id,version,points,original_target_id,last_execution_target_id)
    VALUES(NEW.brand_id,NEW.cycle_id,NEW.agent_id,NEW.member_id,NEW.financial_version,NEW.points_after,original,NEW.id);
  END IF;
 END IF;
 RETURN NULL;
END $$;

CREATE FUNCTION capture_commission_correction_policy() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN INSERT INTO commission_correction_policy_revisions(brand_id,version,enabled,audit_log_id) VALUES(NEW.brand_id,NEW.version,NEW.enabled,NEW.audit_log_id); RETURN NULL; END $$;

CREATE FUNCTION capture_commission_payment_policy() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 INSERT INTO commission_payment_policy_revisions(brand_id,version,enabled,audit_log_id) VALUES(NEW.brand_id,NEW.version,NEW.enabled,NEW.audit_log_id); RETURN NULL;
END $$;

CREATE FUNCTION capture_member_attribution() RETURNS trigger
    LANGUAGE plpgsql
    AS $_$
DECLARE c join_codes;n agent_nodes;selected_kind text;selected_code text;chain jsonb;policy brand_agent_policies;source_user uuid;join_time timestamptz;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'member provenance cannot be deleted';END IF;
 IF TG_OP='UPDATE' THEN
  IF ROW(NEW.id,NEW.brand_id,NEW.global_user_id,NEW.join_method,NEW.join_domain,NEW.joined_at,NEW.created_by,NEW.attribution_snapshot) IS DISTINCT FROM ROW(OLD.id,OLD.brand_id,OLD.global_user_id,OLD.join_method,OLD.join_domain,OLD.joined_at,OLD.created_by,OLD.attribution_snapshot) THEN RAISE EXCEPTION 'member provenance immutable';END IF;
  RETURN NEW;
 END IF;
 IF NEW.attribution_snapshot<>'{}'::jsonb THEN
  IF jsonb_typeof(NEW.attribution_snapshot)<>'object' THEN RAISE EXCEPTION 'invalid join-code selection' USING ERRCODE='23514',CONSTRAINT='join_code_unavailable';END IF;
  IF (SELECT count(*) FROM jsonb_object_keys(NEW.attribution_snapshot))<>2 THEN RAISE EXCEPTION 'invalid join-code selection' USING ERRCODE='23514',CONSTRAINT='join_code_unavailable';END IF;
  selected_kind:=NEW.attribution_snapshot->>'kind';selected_code:=NEW.attribution_snapshot->>'code';
  IF NEW.join_method='operator' AND NEW.created_by IS NULL THEN RAISE EXCEPTION 'operator code attribution requires creator' USING ERRCODE='23514',CONSTRAINT='join_code_unavailable';END IF;
  IF selected_kind NOT IN ('agent','referral') OR selected_code IS NULL OR selected_code!~'^[A-F0-9]{24}$' OR (NEW.join_method<>'operator' AND NEW.join_method<>selected_kind||'_code') THEN RAISE EXCEPTION 'invalid join-code selection' USING ERRCODE='23514',CONSTRAINT='join_code_unavailable';END IF;
  SELECT * INTO policy FROM brand_agent_policies WHERE brand_id=NEW.brand_id FOR SHARE;
  SELECT * INTO c FROM join_codes WHERE brand_id=NEW.brand_id AND join_codes.code=selected_code AND join_codes.kind=selected_kind FOR SHARE;
  IF NOT FOUND OR NOT join_code_usable(c) OR EXISTS(SELECT 1 FROM brand_members WHERE brand_id=NEW.brand_id AND id=c.owner_member_id AND global_user_id=NEW.global_user_id) THEN RAISE EXCEPTION 'join code unavailable' USING ERRCODE='23514',CONSTRAINT='join_code_unavailable';END IF;
  SELECT global_user_id INTO source_user FROM brand_members WHERE brand_id=NEW.brand_id AND id=c.owner_member_id;
  PERFORM 1 FROM global_users WHERE id=source_user FOR SHARE;
  PERFORM 1 FROM brand_members WHERE brand_id=NEW.brand_id AND id=c.owner_member_id FOR SHARE;
  join_time:=clock_timestamp();
  IF NOT join_code_usable_at(c,join_time) THEN RAISE EXCEPTION 'join code unavailable' USING ERRCODE='23514',CONSTRAINT='join_code_unavailable';END IF;
  IF selected_kind='agent' THEN
   SELECT * INTO n FROM agent_nodes WHERE brand_id=NEW.brand_id AND id=c.agent_id;
   SELECT jsonb_agg(jsonb_build_object('id',a.id,'version',a.version,'config',a.config) ORDER BY a.depth) INTO chain FROM agent_nodes a WHERE a.brand_id=NEW.brand_id AND a.id=ANY(n.path);
  END IF;
 ELSIF NEW.join_method NOT IN ('domain','operator') THEN RAISE EXCEPTION 'join code required' USING ERRCODE='23514',CONSTRAINT='join_code_unavailable';
 END IF;
 NEW.joined_at:=coalesce(join_time,clock_timestamp());
 NEW.attribution_snapshot:=jsonb_build_object('schema_version',1,'join_method',NEW.join_method,'join_domain',NEW.join_domain,'joined_at',NEW.joined_at,'code_id',c.id,'code_version',c.version,'code',c.code,'code_kind',c.kind,'owner_member_id',c.owner_member_id,'agent_id',c.agent_id,'referrer_member_id',CASE WHEN c.kind='referral' THEN c.owner_member_id ELSE NULL END,'agent_path',coalesce(to_jsonb(n.path),'[]'::jsonb),'agent_configs_at_join',coalesce(chain,'[]'::jsonb),'agent_policy_at_join',CASE WHEN selected_kind='agent' THEN jsonb_build_object('version',policy.version,'config',policy.config) ELSE NULL END);
 RETURN NEW;
END $_$;

CREATE FUNCTION capture_order_attribution() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE m brand_members;p brand_agent_policies;n agent_nodes;chain jsonb;
BEGIN
 IF TG_OP='UPDATE' THEN
  IF NEW.attribution_snapshot IS DISTINCT FROM OLD.attribution_snapshot THEN RAISE EXCEPTION 'order attribution immutable';END IF;RETURN NEW;
 END IF;
 SELECT * INTO m FROM brand_members WHERE brand_id=NEW.brand_id AND id=NEW.brand_member_id;
 IF NOT FOUND THEN RAISE EXCEPTION 'order member missing';END IF;
 SELECT * INTO p FROM brand_agent_policies WHERE brand_id=NEW.brand_id FOR SHARE;
 IF m.attribution_snapshot->>'agent_id' IS NOT NULL THEN
  SELECT * INTO n FROM agent_nodes WHERE brand_id=NEW.brand_id AND id=(m.attribution_snapshot->>'agent_id')::uuid;
  SELECT jsonb_agg(jsonb_build_object('id',a.id,'parent_id',a.parent_id,'depth',a.depth,'version',a.version,'config',a.config) ORDER BY a.depth) INTO chain FROM agent_nodes a WHERE a.brand_id=NEW.brand_id AND a.id=ANY(n.path);
 END IF;
 NEW.attribution_snapshot:=jsonb_build_object('schema_version',1,'member_attribution',m.attribution_snapshot,'agent_policy',jsonb_build_object('version',p.version,'config',p.config),'agent_configs_at_bet',coalesce(chain,'[]'::jsonb),'captured_at',clock_timestamp(),'commission_policy',NULL);
 RETURN NEW;
END $$;

CREATE FUNCTION capture_report_archive_policy() RETURNS trigger
    LANGUAGE plpgsql
    AS $$ BEGIN INSERT INTO report_archive_policy_revisions SELECT NEW.*; RETURN NULL; END $$;

CREATE FUNCTION commission_analysis_run_valid(b uuid, cid uuid, rid uuid) RETURNS boolean
    LANGUAGE plpgsql STABLE
    AS $$
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
   OR validated_point_snapshot(l.delta_snapshot) IS DISTINCT FROM lottery_business_allocation_delta(o.deduction_allocation,true)
   OR NOT lottery_business_ledger_applied(l,b,x.account_id)
   OR x.prize_points>0 AND (pl.id IS NULL OR pl.brand_id IS DISTINCT FROM b OR pl.account_id IS DISTINCT FROM x.account_id OR pl.member_id IS DISTINCT FROM x.member_id
    OR pl.entry_type<>'prize' OR pl.reference_type<>'settlement_calculation' OR pl.reference_id IS DISTINCT FROM x.calculation_id
    OR pl.operation_key<>'settlement-payout:'||x.calculation_id::text
    OR validated_point_snapshot(pl.delta_snapshot) IS DISTINCT FROM jsonb_set(point_zero_snapshot(),'{winning,available}',to_jsonb(x.prize_points::text))
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

CREATE FUNCTION commission_calendar_boundary(wall timestamp without time zone, tz text) RETURNS timestamp with time zone
    LANGUAGE plpgsql STABLE
    AS $$
DECLARE candidates timestamptz[];
BEGIN
 -- Match the Go calendar's exact round-trip resolver. PostgreSQL's default
 -- timezone conversion alone silently selects ambiguous/nonexistent walls.
 SELECT array_agg(DISTINCT candidate) INTO candidates FROM (
  SELECT (wall AT TIME ZONE 'UTC')-((sample AT TIME ZONE tz)-(sample AT TIME ZONE 'UTC')) candidate
  FROM generate_series((wall AT TIME ZONE 'UTC')-interval '36 hours',(wall AT TIME ZONE 'UTC')+interval '36 hours',interval '15 minutes') sample
 ) resolved WHERE candidate AT TIME ZONE tz=wall;
 IF coalesce(cardinality(candidates),0)<>1 THEN RAISE EXCEPTION 'invalid commission calendar boundary'; END IF;
 RETURN candidates[1];
END $$;

CREATE FUNCTION commission_calendar_window(c jsonb, at_time timestamp with time zone) RETURNS TABLE(window_from timestamp with time zone, window_to timestamp with time zone)
    LANGUAGE plpgsql STABLE
    AS $$
DECLARE tz text; wall_date date; anchor date; candidate_date date; month_start date; month_last date;
 boundary_clock time; candidate timestamptz; day_number integer; delta integer; offset_days integer;
BEGIN
 IF at_time IS NULL OR NOT isfinite(at_time) OR valid_commission_policy(jsonb_build_object('enabled',true,'calendar',c,'payout_mode','manual')) IS DISTINCT FROM true THEN
  RAISE EXCEPTION 'invalid commission calendar'; END IF;
 tz:=c->>'timezone';boundary_clock:=(c->>'boundary_time')::time;wall_date:=(at_time AT TIME ZONE tz)::date;
 IF c->>'cycle'='weekly' THEN
  offset_days:=mod(extract(dow FROM wall_date)::integer-(c->>'weekday')::integer+7,7);
  anchor:=wall_date-offset_days;
  window_from:=commission_calendar_boundary(anchor+boundary_clock,tz);
  IF at_time<window_from THEN anchor:=anchor-7;window_from:=commission_calendar_boundary(anchor+boundary_clock,tz);END IF;
  window_to:=commission_calendar_boundary((anchor+7)+boundary_clock,tz);
 ELSE
  month_start:=date_trunc('month',wall_date)::date;day_number:=(c->>'month_day')::integer;
  FOR delta IN 0..2 LOOP
   anchor:=(month_start+make_interval(months=>-delta))::date;month_last:=(anchor+interval '1 month'-interval '1 day')::date;
   IF day_number>extract(day FROM month_last)::integer AND c->>'short_month'='skip' THEN CONTINUE;END IF;
   candidate_date:=anchor+(least(day_number,extract(day FROM month_last)::integer)-1);
   candidate:=commission_calendar_boundary(candidate_date+boundary_clock,tz);
   IF candidate<=at_time THEN window_from:=candidate;EXIT;END IF;
  END LOOP;
  FOR delta IN 0..2 LOOP
   anchor:=(month_start+make_interval(months=>delta))::date;month_last:=(anchor+interval '1 month'-interval '1 day')::date;
   IF day_number>extract(day FROM month_last)::integer AND c->>'short_month'='skip' THEN CONTINUE;END IF;
   candidate_date:=anchor+(least(day_number,extract(day FROM month_last)::integer)-1);
   candidate:=commission_calendar_boundary(candidate_date+boundary_clock,tz);
   IF candidate>window_from THEN window_to:=candidate;EXIT;END IF;
  END LOOP;
 END IF;
 IF window_from IS NULL OR window_to IS NULL THEN RAISE EXCEPTION 'invalid commission calendar window';END IF;
 RETURN NEXT;
END $$;

CREATE FUNCTION commission_correction_candidates(b uuid, pid uuid, rid uuid) RETURNS TABLE(agent_id uuid, member_id uuid, original_target_id uuid, earning_id uuid, adjustment_version bigint, points_before bigint, points_after bigint, delta_points bigint)
    LANGUAGE sql STABLE
    AS $$ SELECT agent_id,member_id,original_target_id,earning_id,adjustment_version,points_before,points_after,delta_points FROM commission_correction_candidates_v2(b,pid,rid) $$;

CREATE FUNCTION commission_correction_candidates_v2(b uuid, pid uuid, rid uuid) RETURNS TABLE(agent_id uuid, member_id uuid, original_target_id uuid, earning_id uuid, adjustment_version bigint, points_before bigint, points_after bigint, delta_points bigint, previous_correction_target_id uuid, financial_version bigint)
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

CREATE FUNCTION commission_correction_execution_current(b uuid, xid uuid) RETURNS boolean
    LANGUAGE sql STABLE
    AS $$
 SELECT EXISTS(SELECT 1 FROM commission_correction_executions x JOIN commission_correction_plans p ON p.brand_id=x.brand_id AND p.id=x.plan_id
  WHERE x.brand_id=b AND x.id=xid AND p.state='ready' AND p.version=x.plan_version AND p.run_id=x.run_id AND p.evidence_epoch=x.evidence_epoch
   AND commission_payment_evidence_current(b,x.cycle_id,x.run_id,x.evidence_epoch))
$$;

CREATE FUNCTION commission_correction_heads_valid(b uuid, cid uuid) RETURNS boolean
    LANGUAGE sql STABLE
    AS $$
 SELECT NOT EXISTS(SELECT 1 FROM commission_correction_balance_heads h
  LEFT JOIN commission_correction_execution_targets t ON t.brand_id=h.brand_id AND t.id=h.last_execution_target_id
  LEFT JOIN audit_logs a ON a.id=t.audit_log_id LEFT JOIN point_ledger_entries l ON l.id=t.ledger_entry_id
  WHERE h.brand_id=b AND h.cycle_id=cid AND (t.id IS NULL OR t.state<>'applied' OR t.cycle_id<>cid OR t.agent_id<>h.agent_id OR t.member_id<>h.member_id OR
   t.points_after<>h.points OR t.financial_version<>h.version OR a.id IS NULL OR a.brand_id IS DISTINCT FROM b OR a.actor_type<>'system' OR a.actor_id IS NOT NULL OR
   a.action<>'commission.correction_execution.target' OR a.resource_type<>'commission_correction_target' OR a.resource_id IS DISTINCT FROM t.id OR
   a.before_json IS DISTINCT FROM jsonb_build_object('state','pending') OR
   a.after_json IS DISTINCT FROM jsonb_build_object('state','applied','execution_id',t.execution_id,'plan_target_id',t.plan_target_id,
    'points_before',t.points_before::text,'points_after',t.points_after::text,'delta_points',t.delta_points::text,'ledger_entry_id',t.ledger_entry_id,'financial_version',t.financial_version) OR
   t.delta_points<>0 AND (l.id IS NULL OR l.brand_id<>b OR l.member_id<>h.member_id OR l.entry_type<>'commission_correction' OR l.reference_type<>'commission_correction_target' OR l.reference_id IS DISTINCT FROM t.id OR
    l.operation_key<>'commission-correction:'||t.id::text OR l.request_id<>a.request_id OR l.actor_type<>'system' OR l.actor_id IS NOT NULL OR l.reversal_of IS NOT NULL OR
    l.delta_snapshot IS DISTINCT FROM jsonb_set(point_zero_snapshot(),'{commission,available}',to_jsonb(t.delta_points::text)) OR
    l.source_allocation IS DISTINCT FROM jsonb_build_array(jsonb_build_object('source','commission','state','available','points',abs(t.delta_points)::text))) OR t.delta_points=0 AND t.ledger_entry_id IS NOT NULL))
$$;

CREATE FUNCTION commission_correction_mode(original text, corrected text) RETURNS text
    LANGUAGE sql IMMUTABLE
    AS $$
 SELECT CASE WHEN original NOT IN('manual','automatic','mixed','none') OR corrected NOT IN('manual','automatic','mixed','none') THEN NULL
  WHEN original='mixed' OR corrected='mixed' OR original<>corrected AND original<>'none' AND corrected<>'none' THEN 'mixed'
  WHEN original='none' THEN corrected ELSE original END
$$;

CREATE FUNCTION commission_correction_source_valid(b uuid, pid uuid, rid uuid) RETURNS boolean
    LANGUAGE sql STABLE
    AS $$
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

CREATE FUNCTION commission_payment_evidence_current(b uuid, cid uuid, rid uuid, epoch bigint) RETURNS boolean
    LANGUAGE sql STABLE
    AS $$
 SELECT EXISTS(SELECT 1 FROM commission_cycles c JOIN commission_runs r ON r.brand_id=c.brand_id AND r.cycle_id=c.id AND r.id=c.current_run_id
  WHERE c.brand_id=b AND c.id=cid AND r.id=rid AND c.state='ready' AND c.scan_complete AND r.state='ready' AND c.evidence_epoch=epoch AND r.evidence_epoch=epoch
  AND c.target_count=(SELECT count(*) FROM commission_calculations x WHERE x.run_id=rid)
  AND NOT EXISTS(SELECT 1 FROM commission_calculations x
   LEFT JOIN bet_orders o ON o.brand_id=x.brand_id AND o.id=x.order_id
   LEFT JOIN periods p ON p.brand_id=o.brand_id AND p.id=o.period_id
   LEFT JOIN settlement_calculations sc ON sc.brand_id=x.brand_id AND sc.id=x.calculation_id
   LEFT JOIN settlement_jobs j ON j.brand_id=x.brand_id AND j.id=x.job_id
   WHERE x.run_id=rid AND (o.id IS NULL OR
    (x.status,x.member_id,x.account_id,x.stake_points,x.prize_points,x.rule_snapshot) IS DISTINCT FROM
    (o.status,o.brand_member_id,o.account_id,o.total_points,o.prize_points,o.commission_rule_snapshot) OR
    (o.status IN('won','lost') AND (p.status IS DISTINCT FROM 'settled' OR p.current_settlement_job_id IS DISTINCT FROM x.job_id OR
     o.settlement_calculation_id IS DISTINCT FROM x.calculation_id OR j.state IS DISTINCT FROM 'completed' OR j.generation IS DISTINCT FROM x.generation OR
     j.draw_result_id IS DISTINCT FROM p.draw_result_id OR sc.job_id IS DISTINCT FROM x.job_id OR sc.order_id IS DISTINCT FROM x.order_id OR
     sc.prize_points IS DISTINCT FROM x.prize_points OR sc.won IS DISTINCT FROM (o.status='won') OR
     EXISTS(SELECT 1 FROM draw_corrections dc WHERE dc.brand_id=b AND dc.period_id=p.id AND dc.state<>'completed'))))))
$$;

CREATE FUNCTION commission_payment_has_actual_money(b uuid, pid uuid) RETURNS boolean
    LANGUAGE sql STABLE
    AS $$
 SELECT EXISTS(SELECT 1 FROM commission_payment_targets t WHERE t.brand_id=b AND t.payment_id=pid AND
  (t.ledger_entry_id IS NOT NULL OR EXISTS(SELECT 1 FROM point_ledger_entries l WHERE l.brand_id=b AND l.reference_type='commission_payment_target' AND l.reference_id=t.id)))
 OR EXISTS(SELECT 1 FROM commission_adjustments a WHERE a.brand_id=b AND a.payment_id=pid AND
  (a.ledger_entry_id IS NOT NULL OR EXISTS(SELECT 1 FROM point_ledger_entries l WHERE l.brand_id=b AND l.reference_type='commission_adjustment' AND l.reference_id=a.id)))
 OR EXISTS(SELECT 1 FROM commission_correction_plans p
  JOIN commission_correction_executions x ON x.brand_id=p.brand_id AND x.plan_id=p.id
  JOIN commission_correction_execution_targets t ON t.brand_id=x.brand_id AND t.execution_id=x.id
  WHERE p.brand_id=b AND p.payment_id=pid AND t.state='applied' AND t.ledger_entry_id IS NOT NULL)
$$;

CREATE FUNCTION commission_payment_mode(rid uuid) RETURNS text
    LANGUAGE sql STABLE
    AS $$
 SELECT CASE count(DISTINCT rule_snapshot->'financial_policy'->'config'->>'payout_mode') WHEN 0 THEN 'none' WHEN 1 THEN min(rule_snapshot->'financial_policy'->'config'->>'payout_mode') ELSE 'mixed' END
 FROM commission_calculations WHERE run_id=rid AND reason='eligible'
$$;

CREATE FUNCTION commission_policy_revision_committed() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM brand_commission_policies WHERE brand_id=NEW.brand_id AND version>=NEW.version) THEN
  RAISE EXCEPTION 'orphan commission policy revision'; END IF;
 RETURN NULL;
END $$;

CREATE FUNCTION compliance_stub_checks(v jsonb) RETURNS jsonb
    LANGUAGE sql IMMUTABLE
    AS $$
 SELECT jsonb_build_array(
 jsonb_build_object('check','age','enabled',v->'age_enabled','decision',CASE WHEN v->'age_enabled'='true'::jsonb THEN 'review' ELSE 'allow' END,'reason_code',CASE WHEN v->'age_enabled'='true'::jsonb THEN 'ADAPTER_NOT_CONFIGURED' ELSE 'CHECK_DISABLED' END),
 jsonb_build_object('check','region','enabled',v->'region_enabled','decision',CASE WHEN v->'region_enabled'='true'::jsonb THEN 'review' ELSE 'allow' END,'reason_code',CASE WHEN v->'region_enabled'='true'::jsonb THEN 'ADAPTER_NOT_CONFIGURED' ELSE 'CHECK_DISABLED' END),
 jsonb_build_object('check','identity','enabled',v->'identity_enabled','decision',CASE WHEN v->'identity_enabled'='true'::jsonb THEN 'review' ELSE 'allow' END,'reason_code',CASE WHEN v->'identity_enabled'='true'::jsonb THEN 'ADAPTER_NOT_CONFIGURED' ELSE 'CHECK_DISABLED' END))
$$;

SET LOCAL default_tablespace = '';

SET LOCAL default_table_access_method = heap;

CREATE TABLE draw_correction_targets (
    brand_id uuid NOT NULL,
    correction_id uuid NOT NULL,
    period_id uuid NOT NULL,
    order_id uuid NOT NULL,
    old_order_version bigint NOT NULL,
    old_order_status text NOT NULL,
    old_calculation_id uuid,
    old_payout_entry_id uuid,
    old_prize_points bigint NOT NULL,
    state text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    reversal_entry_id uuid,
    reset_order_version bigint,
    error_code text,
    CONSTRAINT draw_correction_targets_check CHECK (((old_prize_points > 0) = (old_payout_entry_id IS NOT NULL))),
    CONSTRAINT draw_correction_targets_check1 CHECK (((state = 'reversed'::text) = (reset_order_version IS NOT NULL))),
    CONSTRAINT draw_correction_targets_check2 CHECK (((state = 'reversed'::text) OR (reversal_entry_id IS NULL))),
    CONSTRAINT draw_correction_targets_old_order_version_check CHECK ((old_order_version > 0)),
    CONSTRAINT draw_correction_targets_old_prize_points_check CHECK ((old_prize_points >= 0)),
    CONSTRAINT draw_correction_targets_state_check CHECK ((state = ANY (ARRAY['pending'::text, 'reversed'::text, 'unchanged'::text, 'excluded'::text, 'failed'::text]))),
    CONSTRAINT draw_correction_targets_version_check CHECK ((version > 0))
);

CREATE FUNCTION correction_reversal_is_applied(t draw_correction_targets) RETURNS boolean
    LANGUAGE plpgsql
    AS $$
DECLARE l point_ledger_entries; original point_ledger_entries; zero_bucket jsonb; expected jsonb;
BEGIN
 IF t.old_prize_points=0 THEN RETURN t.old_payout_entry_id IS NULL AND t.reversal_entry_id IS NULL; END IF;
 SELECT * INTO original FROM point_ledger_entries WHERE id=t.old_payout_entry_id;
 SELECT * INTO l FROM point_ledger_entries WHERE id=t.reversal_entry_id;
 zero_bucket:=jsonb_build_object('available','0','manual_frozen','0','system_frozen','0','withdrawal','0');
 expected:=jsonb_build_object('recharge',zero_bucket,'gift',zero_bucket,'commission',zero_bucket,'winning',zero_bucket||jsonb_build_object('available',(-t.old_prize_points)::text));
 RETURN COALESCE(l.id IS NOT NULL AND original.id IS NOT NULL AND l.brand_id=t.brand_id AND l.account_id=original.account_id AND l.member_id=original.member_id AND l.entry_type='prize_reversal' AND l.reference_type='draw_correction' AND l.reference_id=t.correction_id AND l.reversal_of=t.old_payout_entry_id AND l.operation_key='draw-correction:'||t.correction_id::text||':'||t.order_id::text AND l.actor_type='system' AND l.source_allocation=original.source_allocation AND validated_point_snapshot(l.delta_snapshot)=expected AND
  EXISTS(SELECT 1 FROM audit_logs a WHERE a.brand_id=t.brand_id AND a.action='points.prize_reversal' AND a.actor_type='system' AND a.resource_id=l.account_id AND a.after_json->>'ledger_entry_id'=l.id::text AND a.before_json=l.before_snapshot AND a.after_json->'balance'=l.after_snapshot),FALSE);
END $$;

CREATE FUNCTION default_agent_policy() RETURNS jsonb
    LANGUAGE sql IMMUTABLE
    AS $$
 SELECT '{"enabled":false,"max_depth":5,"ratio_cap":"0","mode":"loss","cycle":"monthly"}'::jsonb
$$;

CREATE FUNCTION default_brand_bet_policy() RETURNS jsonb
    LANGUAGE sql IMMUTABLE
    AS $$ SELECT '{"min_bet_points":"1","max_bet_points":null,"max_period_points":null,"max_user_period_points":null,"user_cancel_allowed":false}'::jsonb $$;

CREATE FUNCTION default_brand_withdrawal_policy() RETURNS jsonb
    LANGUAGE sql IMMUTABLE
    AS $$
 SELECT '{"enabled":false,"min_points":"1","max_points":null,"allowed_sources":["recharge","winning","gift"],"review_mode":"manual","turnover_multiple":"1"}'::jsonb
$$;

CREATE FUNCTION default_commission_policy() RETURNS jsonb
    LANGUAGE sql IMMUTABLE
    AS $$
 SELECT '{"enabled":false,"calendar":null,"payout_mode":"manual"}'::jsonb
$$;

CREATE FUNCTION default_compliance_config() RETURNS jsonb
    LANGUAGE sql IMMUTABLE
    AS $$
 SELECT '{"age_enabled":false,"minimum_age":null,"region_enabled":false,"allowed_countries":[],"identity_enabled":false}'::jsonb
$$;

CREATE FUNCTION default_game_bet_policy() RETURNS jsonb
    LANGUAGE sql IMMUTABLE
    AS $$ SELECT '{"min_bet_points":{"mode":"inherit","points":null},"max_bet_points":{"mode":"inherit","points":null},"max_period_points":{"mode":"inherit","points":null},"max_user_period_points":{"mode":"inherit","points":null},"user_cancel_allowed":null}'::jsonb $$;

CREATE FUNCTION draw_notice_current_row(row_xid xid) RETURNS boolean
    LANGUAGE plpgsql
    AS $$
DECLARE top_xid numeric:=pg_current_xact_id()::text::numeric; full_xid numeric;
BEGIN
 full_xid:=top_xid-mod(top_xid,4294967296)+row_xid::text::numeric;
 IF full_xid<top_xid THEN full_xid:=full_xid+4294967296; END IF;
 RETURN pg_xact_status(full_xid::text::xid8)='in progress';
EXCEPTION WHEN OTHERS THEN RETURN false;
END $$;

CREATE FUNCTION emit_commission_adjusted_notification() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE member uuid;
BEGIN
 IF OLD.ledger_entry_id IS NULL AND NEW.ledger_entry_id IS NOT NULL THEN
  SELECT member_id INTO member FROM commission_payment_targets WHERE brand_id=NEW.brand_id AND id=NEW.target_id;
  INSERT INTO outbox_events(id,brand_id,event_type,aggregate_id,payload) VALUES(
   gen_random_uuid(),NEW.brand_id,'commission.adjusted',NEW.id,
   jsonb_build_object('member_id',member::text,'resource_id',NEW.id::text,'points',NEW.delta_points::text,
    'ledger_entry_id',NEW.ledger_entry_id::text,'target_id',NEW.target_id::text)) ON CONFLICT DO NOTHING;
 END IF;
 RETURN NULL;
END $$;

CREATE FUNCTION emit_commission_correction_notification() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF OLD.state='pending' AND NEW.state='applied' AND NEW.delta_points<>0 THEN
  INSERT INTO outbox_events(id,brand_id,event_type,aggregate_id,payload) VALUES(gen_random_uuid(),NEW.brand_id,'commission.corrected',NEW.id,
   jsonb_build_object('member_id',NEW.member_id::text,'resource_id',NEW.id::text,'points',NEW.delta_points::text,
    'ledger_entry_id',NEW.ledger_entry_id::text,'target_id',NEW.id::text));
 END IF;
 RETURN NULL;
END $$;

CREATE FUNCTION emit_commission_paid_notification() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF OLD.state='pending' AND NEW.state='paid' AND NEW.points>0 THEN
  INSERT INTO outbox_events(id,brand_id,event_type,aggregate_id,payload) VALUES(
   gen_random_uuid(),NEW.brand_id,'commission.paid',NEW.id,
   jsonb_build_object('member_id',NEW.member_id::text,'resource_id',NEW.id::text,'points',NEW.points::text,
    'ledger_entry_id',NEW.ledger_entry_id::text,'target_id',NEW.id::text)) ON CONFLICT DO NOTHING;
 END IF;
 RETURN NULL;
END $$;

CREATE FUNCTION emit_reward_notification() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE a reward_order_actions;
BEGIN
 IF (OLD.grant_ledger_entry_id IS NULL AND NEW.grant_ledger_entry_id IS NOT NULL) OR OLD.version<>NEW.version THEN
  SELECT * INTO a FROM reward_order_actions WHERE brand_id=NEW.brand_id AND order_id=NEW.id AND version=NEW.version;
  IF a.id IS NULL THEN RAISE EXCEPTION 'reward notification requires matching action'; END IF;
  INSERT INTO outbox_events(id,brand_id,event_type,aggregate_id,payload) VALUES(gen_random_uuid(),NEW.brand_id,'reward.order.'||a.state_after,a.id,
   jsonb_build_object('member_id',NEW.member_id::text,'resource_id',NEW.id::text,'points',NEW.points::text,
    'action_id',a.id::text,'version',a.version,'audit_log_id',a.audit_log_id::text));
 END IF;
 RETURN NULL;
END $$;

CREATE FUNCTION enforce_admin_role_brand() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE role_brand uuid;
BEGIN
 SELECT brand_id INTO role_brand FROM roles WHERE id=NEW.role_id;
 IF role_brand IS NOT NULL AND NOT EXISTS
 (SELECT 1 FROM admin_brand_scopes WHERE account_id=NEW.account_id AND brand_id=role_brand) THEN
  RAISE EXCEPTION 'role is outside account brand scope' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION enqueue_commission_discovery() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF NEW.commission_rule_snapshot->'financial_policy'->'config'->'enabled'='true'::jsonb THEN
  INSERT INTO commission_discovery(id,brand_id) VALUES(NEW.id,NEW.brand_id);
 END IF;
 RETURN NULL;
END $$;

CREATE FUNCTION enqueue_in_app_event() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF NEW.brand_id IS NOT NULL AND notification_template_defaults() ? NEW.event_type THEN
  INSERT INTO notification_deliveries(event_id,brand_id) VALUES(NEW.id,NEW.brand_id) ON CONFLICT DO NOTHING;
 END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_agency_commission_calendar() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE c jsonb;
BEGIN
 SELECT config INTO c FROM brand_commission_policies WHERE brand_id=NEW.brand_id FOR SHARE;
 IF c->'enabled'='true'::jsonb AND
  (NEW.config->'enabled'<>'true'::jsonb OR NEW.config->>'cycle' IS DISTINCT FROM c->'calendar'->>'cycle') THEN
  RAISE EXCEPTION 'disable financial commission policy before changing agency cycle';
 END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_agent_node() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE p jsonb; parent agent_nodes;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'agent identity cannot be deleted'; END IF;
 SELECT config INTO p FROM brand_agent_policies WHERE brand_id=NEW.brand_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION 'agent policy missing'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.version<>1 OR p->'enabled'<>'true'::jsonb OR NOT EXISTS(SELECT 1 FROM brand_members WHERE brand_id=NEW.brand_id AND id=NEW.member_id AND status='normal') THEN RAISE EXCEPTION 'agent admission invalid'; END IF;
 ELSE
  IF (to_jsonb(NEW)-ARRAY['version','config','updated_at']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['version','config','updated_at']) OR NEW.version<>OLD.version+1 THEN RAISE EXCEPTION 'agent identity/path/parent immutable'; END IF;
 END IF;
 IF NEW.depth>(p->>'max_depth')::int OR (NEW.config->>'ratio')::numeric>(p->>'ratio_cap')::numeric THEN RAISE EXCEPTION 'agent exceeds policy limit'; END IF;
 IF NEW.parent_id IS NULL THEN
  IF NEW.depth<>1 OR NEW.path<>ARRAY[NEW.id] THEN RAISE EXCEPTION 'invalid root lineage'; END IF;
 ELSE
  SELECT * INTO parent FROM agent_nodes WHERE brand_id=NEW.brand_id AND id=NEW.parent_id;
  IF NOT FOUND OR NEW.depth<>parent.depth+1 OR NEW.path<>array_append(parent.path,NEW.id) OR (NEW.config->>'ratio')::numeric>(parent.config->>'ratio')::numeric THEN RAISE EXCEPTION 'invalid child lineage or ratio'; END IF;
  IF TG_OP='INSERT' AND (parent.config->>'status'<>'active' OR parent.config->'can_create_children'<>'true'::jsonb OR EXISTS(SELECT 1 FROM agent_nodes WHERE brand_id=NEW.brand_id AND id=ANY(parent.path) AND config->>'status'<>'active')) THEN RAISE EXCEPTION 'parent cannot develop children'; END IF;
 END IF;
 IF EXISTS(SELECT 1 FROM agent_nodes WHERE brand_id=NEW.brand_id AND parent_id=NEW.id AND (config->>'ratio')::numeric>(NEW.config->>'ratio')::numeric) THEN RAISE EXCEPTION 'existing child exceeds new ratio'; END IF;
 NEW.updated_at:=clock_timestamp();RETURN NEW;
END $$;

CREATE FUNCTION guard_agent_policy() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'agent policies cannot be deleted'; END IF;
 IF TG_OP='INSERT' THEN IF NEW.version<>1 OR NEW.config<>default_agent_policy() THEN RAISE EXCEPTION 'initial disabled agent policy required'; END IF;
 ELSE
  IF (to_jsonb(NEW)-ARRAY['version','config','updated_at']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['version','config','updated_at']) OR NEW.version<>OLD.version+1 THEN RAISE EXCEPTION 'agent policy identity/version immutable'; END IF;
  IF EXISTS(SELECT 1 FROM agent_nodes WHERE brand_id=NEW.brand_id AND ((config->>'ratio')::numeric>(NEW.config->>'ratio_cap')::numeric OR depth>(NEW.config->>'max_depth')::int)) THEN RAISE EXCEPTION 'existing agents exceed new policy'; END IF;
 END IF;
 NEW.updated_at:=clock_timestamp();RETURN NEW;
END $$;

CREATE FUNCTION guard_agent_revision() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE v bigint; cfg jsonb; a audit_logs;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'agent configuration history immutable'; END IF;
 IF NEW.agent_id IS NULL THEN SELECT version,config INTO v,cfg FROM brand_agent_policies WHERE brand_id=NEW.brand_id;
 ELSE SELECT version,config INTO v,cfg FROM agent_nodes WHERE brand_id=NEW.brand_id AND id=NEW.agent_id; END IF;
 IF NOT FOUND OR (NEW.version=1 AND (v<>1 OR cfg<>NEW.config)) OR (NEW.version>1 AND v<>NEW.version-1) THEN RAISE EXCEPTION 'agent revision must belong to current/next version'; END IF;
 IF NEW.actor_type<>'system' THEN
  SELECT * INTO a FROM audit_logs WHERE id=NEW.audit_log_id;
  IF NOT FOUND OR a.brand_id<>NEW.brand_id OR a.actor_type<>NEW.actor_type OR a.actor_id IS DISTINCT FROM NEW.actor_id OR
   a.resource_type<>(CASE WHEN NEW.agent_id IS NULL THEN 'agent_policy' ELSE 'agent_node' END) OR
   a.resource_id IS DISTINCT FROM coalesce(NEW.agent_id,NEW.brand_id) OR a.reason<>NEW.reason OR
   (a.after_json->>'version') IS DISTINCT FROM NEW.version::text OR (a.after_json->'config') IS DISTINCT FROM NEW.config THEN RAISE EXCEPTION 'agent revision needs matching audit'; END IF;
 END IF;
 NEW.created_at:=clock_timestamp();RETURN NEW;
END $$;

CREATE FUNCTION guard_agent_uniform_node_mode() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE policy_mode text; parent_mode text; parent_path uuid[]; node_mode text;
BEGIN
 SELECT config->>'mode' INTO policy_mode FROM brand_agent_policies
  WHERE brand_id=NEW.brand_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION 'agent policy missing'; END IF;

 IF NEW.parent_id IS NOT NULL THEN
  SELECT parent.path,agent_effective_mode_for_path(NEW.brand_id,parent.path,NULL,NULL,policy_mode)
   INTO parent_path,parent_mode FROM agent_nodes parent
   WHERE parent.brand_id=NEW.brand_id AND parent.id=NEW.parent_id;
  IF NOT FOUND THEN RAISE EXCEPTION 'agent parent missing'; END IF;
  IF TG_OP='INSERT' AND EXISTS(
   SELECT 1 FROM agent_nodes child
   JOIN agent_nodes ancestor ON ancestor.brand_id=child.brand_id AND ancestor.id=child.parent_id
   WHERE child.brand_id=NEW.brand_id AND child.id=ANY(parent_path)
    AND agent_effective_mode_for_path(child.brand_id,child.path,NULL,NULL,policy_mode)
       IS DISTINCT FROM agent_effective_mode_for_path(ancestor.brand_id,ancestor.path,NULL,NULL,policy_mode)
  ) THEN
   RAISE EXCEPTION 'agent parent has a mixed effective mode ancestor chain' USING ERRCODE='23514',CONSTRAINT='agent_ancestor_mode_chain_consistent';
  END IF;
  node_mode:=coalesce(NULLIF(NEW.config->>'mode',''),parent_mode);
  IF node_mode IS DISTINCT FROM parent_mode THEN
   RAISE EXCEPTION 'child effective mode must match parent' USING ERRCODE='23514',CONSTRAINT='agent_child_mode_matches_parent';
  END IF;
 ELSE
  node_mode:=coalesce(NULLIF(NEW.config->>'mode',''),policy_mode);
 END IF;

 IF TG_OP='UPDATE' THEN
  -- Evaluate the proposed config over this node's complete existing subtree.
  -- Every child must retain the same effective mode as its parent.
  IF EXISTS(
   SELECT 1 FROM agent_nodes child
   WHERE child.brand_id=NEW.brand_id
    AND (child.path @> ARRAY[NEW.id]::uuid[] OR child.id=ANY(NEW.path))
    AND child.parent_id IS NOT NULL
    AND agent_effective_mode_for_path(NEW.brand_id,child.path,NEW.id,NEW.config,policy_mode)
       IS DISTINCT FROM agent_effective_mode_for_path(
        NEW.brand_id,(SELECT parent.path FROM agent_nodes parent WHERE parent.brand_id=child.brand_id AND parent.id=child.parent_id),
        NEW.id,NEW.config,policy_mode)
  ) THEN
   RAISE EXCEPTION 'agent update would mismatch descendant mode' USING ERRCODE='23514',CONSTRAINT='agent_descendant_mode_matches_parent';
  END IF;
 END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_agent_uniform_policy_mode() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF NEW.config->>'mode' IS DISTINCT FROM OLD.config->>'mode' AND EXISTS(
  SELECT 1 FROM agent_nodes child
  JOIN agent_nodes parent ON parent.brand_id=child.brand_id AND parent.id=child.parent_id
  WHERE child.brand_id=NEW.brand_id
   AND agent_effective_mode_for_path(child.brand_id,child.path,NULL,NULL,NEW.config->>'mode')
      IS DISTINCT FROM agent_effective_mode_for_path(parent.brand_id,parent.path,NULL,NULL,NEW.config->>'mode')
 ) THEN
  RAISE EXCEPTION 'brand mode update would mismatch agent hierarchy' USING ERRCODE='23514',CONSTRAINT='agent_brand_mode_preserves_hierarchy';
 END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_bet_exception() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'bet exception evidence cannot be changed'; END IF;
 IF NOT EXISTS(SELECT 1 FROM bet_orders WHERE brand_id=NEW.brand_id AND id=NEW.order_id AND status='placed' AND version=NEW.order_version-1) THEN RAISE EXCEPTION 'invalid exception evidence state'; END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_bet_judgment() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'judgment evidence cannot be changed'; END IF;
 IF NOT EXISTS(SELECT 1 FROM bet_orders o JOIN periods p ON p.id=o.period_id WHERE o.id=NEW.order_id AND o.brand_id=NEW.brand_id AND o.version=NEW.order_version-1 AND o.status IN ('placed','abnormal') AND p.status NOT IN ('settling','settled') AND p.draw_result_id IS NOT DISTINCT FROM NEW.draw_result_id) THEN RAISE EXCEPTION 'invalid judgment state or result snapshot'; END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_bet_order() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
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

CREATE FUNCTION guard_bet_withdrawal_snapshot_update() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF NEW.withdrawal_rule_snapshot IS DISTINCT FROM OLD.withdrawal_rule_snapshot THEN
  RAISE EXCEPTION 'bet withdrawal snapshot is immutable';
 END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_brand_domain_binding() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF TG_OP='DELETE' OR NEW.id IS DISTINCT FROM OLD.id OR NEW.brand_id IS DISTINCT FROM OLD.brand_id OR NEW.domain IS DISTINCT FROM OLD.domain THEN RAISE EXCEPTION 'domain binding immutable';END IF;
 IF NEW.is_primary AND NOT NEW.enabled THEN RAISE EXCEPTION 'primary domain must be enabled';END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_brand_domain_revision() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE b brands;a audit_logs;projection jsonb;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'domain history immutable';END IF;
 SELECT * INTO b FROM brands WHERE id=NEW.brand_id;
 SELECT * INTO a FROM audit_logs WHERE id=NEW.audit_log_id;
 SELECT coalesce(jsonb_agg(jsonb_build_object('id',d.id,'domain',d.domain,'enabled',d.enabled,'is_primary',d.is_primary) ORDER BY d.domain,d.id),'[]'::jsonb) INTO projection FROM brand_domains d WHERE brand_id=NEW.brand_id;
 IF NOT FOUND OR b.id IS NULL OR a.id IS NULL OR b.config_version IS DISTINCT FROM NEW.version
 OR NEW.domains IS DISTINCT FROM projection OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.actor_type IS DISTINCT FROM 'admin'
 OR a.actor_id IS DISTINCT FROM NEW.changed_by OR a.action IS DISTINCT FROM 'brand_domains.update' OR a.resource_type IS DISTINCT FROM 'brand_domains'
 OR a.resource_id IS DISTINCT FROM NEW.brand_id OR a.reason IS DISTINCT FROM NEW.reason
 OR a.before_json->>'brand_id' IS DISTINCT FROM NEW.brand_id::text OR (a.before_json->>'version')::bigint IS DISTINCT FROM (NEW.version-1)
 OR a.before_json->'domains' IS DISTINCT FROM NEW.before_domains OR a.after_json->'domains' IS DISTINCT FROM NEW.domains
 OR a.after_json->>'brand_id' IS DISTINCT FROM NEW.brand_id::text OR (a.after_json->>'version')::bigint IS DISTINCT FROM NEW.version
 OR a.after_json->>'status' IS DISTINCT FROM b.status OR (a.after_json->>'updated_at')::timestamptz IS DISTINCT FROM b.updated_at THEN RAISE EXCEPTION 'domain audit witness missing';END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_brand_operation_revision() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE b brands; a audit_logs;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'brand operation history is immutable'; END IF;
 SELECT * INTO b FROM brands WHERE id=NEW.brand_id;
 SELECT * INTO a FROM audit_logs WHERE id=NEW.audit_log_id;
 IF NOT FOUND OR b.id IS NULL OR b.config_version<>NEW.version OR b.status<>NEW.status OR
    a.brand_id IS DISTINCT FROM NEW.brand_id OR a.actor_type IS DISTINCT FROM 'admin' OR a.actor_id IS DISTINCT FROM NEW.changed_by OR
    a.action IS DISTINCT FROM 'brand_operation.update' OR a.resource_type IS DISTINCT FROM 'brand_operation' OR a.resource_id IS DISTINCT FROM NEW.brand_id OR a.reason IS DISTINCT FROM NEW.reason OR
    a.before_json->>'brand_id' IS DISTINCT FROM NEW.brand_id::text OR
    a.before_json->>'version' IS DISTINCT FROM (NEW.version-1)::text OR
    a.before_json->>'status' IS DISTINCT FROM NEW.previous_status OR
    a.before_json->>'name' IS DISTINCT FROM b.name OR
    a.after_json->>'brand_id' IS DISTINCT FROM NEW.brand_id::text OR
    a.after_json->>'version' IS DISTINCT FROM NEW.version::text OR
    a.after_json->>'status' IS DISTINCT FROM NEW.status OR
    a.after_json->>'name' IS DISTINCT FROM b.name OR
    (a.after_json->>'updated_at')::timestamptz IS DISTINCT FROM b.updated_at THEN
  RAISE EXCEPTION 'brand operation history requires matching current brand and audit witness';
 END IF;
 NEW.created_at:=clock_timestamp();
 RETURN NEW;
END $$;

CREATE FUNCTION guard_brand_presentation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'presentation cannot be deleted';END IF;
 IF NEW.brand_id IS DISTINCT FROM OLD.brand_id OR NEW.version<=OLD.version OR NOT EXISTS(SELECT 1 FROM brands WHERE id=NEW.brand_id AND config_version=NEW.version) THEN RAISE EXCEPTION 'presentation identity/version invalid';END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_brand_presentation_revision() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE b brands;p brand_presentations;a audit_logs;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'presentation history immutable';END IF;
 SELECT * INTO b FROM brands WHERE id=NEW.brand_id;
 SELECT * INTO p FROM brand_presentations WHERE brand_id=NEW.brand_id;
 SELECT * INTO a FROM audit_logs WHERE id=NEW.audit_log_id;
 IF NOT FOUND OR b.id IS NULL OR p.brand_id IS NULL OR p.version IS DISTINCT FROM NEW.version OR b.config_version IS DISTINCT FROM NEW.version
 OR p.config IS DISTINCT FROM NEW.config OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.actor_type IS DISTINCT FROM 'admin'
 OR a.actor_id IS DISTINCT FROM NEW.changed_by OR a.action IS DISTINCT FROM 'brand_presentation.update'
 OR a.resource_type IS DISTINCT FROM 'brand_presentation' OR a.resource_id IS DISTINCT FROM NEW.brand_id OR a.reason IS DISTINCT FROM NEW.reason
 OR a.before_json->>'brand_id' IS DISTINCT FROM NEW.brand_id::text OR (a.before_json->>'version')::bigint IS DISTINCT FROM (NEW.version-1)
 OR a.after_json->>'brand_id' IS DISTINCT FROM NEW.brand_id::text OR (a.after_json->>'version')::bigint IS DISTINCT FROM NEW.version
 OR a.after_json->>'status' IS DISTINCT FROM b.status OR a.after_json->>'base_name' IS DISTINCT FROM b.name
 OR a.after_json->'config' IS DISTINCT FROM NEW.config OR a.after_json->'effective' IS DISTINCT FROM NEW.effective
 OR (a.after_json->>'updated_at')::timestamptz IS DISTINCT FROM b.updated_at THEN RAISE EXCEPTION 'presentation audit witness missing';END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_cancellation_target() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'cancellation target cannot be deleted'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.state<>'pending' OR NEW.version<>1 OR NOT EXISTS(SELECT 1 FROM bet_orders WHERE id=NEW.order_id AND status IN ('placed','abnormal')) OR NOT EXISTS(SELECT 1 FROM period_cancellations c JOIN periods p ON p.id=c.period_id WHERE c.id=NEW.cancellation_id AND c.state='processing' AND p.version=c.period_version-1) THEN RAISE EXCEPTION 'invalid cancellation target'; END IF;
 ELSE
  IF (to_jsonb(NEW)-ARRAY['state','version','refund_entry_id','error_code']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['state','version','refund_entry_id','error_code']) OR NEW.version<>OLD.version+1 OR NOT ((OLD.state='pending' AND NEW.state IN ('refunded','already_refunded','failed')) OR (OLD.state='failed' AND NEW.state='pending')) THEN RAISE EXCEPTION 'invalid cancellation target update'; END IF;
  IF NEW.state IN ('refunded','already_refunded') AND NOT EXISTS(SELECT 1 FROM bet_orders WHERE id=NEW.order_id AND brand_id=NEW.brand_id AND status IN ('bet_cancelled','judged_cancelled') AND refund_entry_id=NEW.refund_entry_id) THEN RAISE EXCEPTION 'missing cancellation refund witness'; END IF;
  IF OLD.state='failed' AND NEW.state='pending' AND NOT EXISTS(SELECT 1 FROM period_cancellations WHERE id=NEW.cancellation_id AND state='processing') THEN RAISE EXCEPTION 'retry requires processing task'; END IF;
 END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_closed_commission_admission() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 -- Alphabetically after capture_bet_commission_snapshot, which holds SHARE.
 IF EXISTS(SELECT 1 FROM commission_cycles WHERE brand_id=NEW.brand_id AND window_from<=NEW.placed_at AND window_to>NEW.placed_at) THEN
  RAISE EXCEPTION 'bet belongs to sealed commission window'; END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_commission_adjustment() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE h commission_adjustment_heads; t commission_payment_targets; p commission_payments; log audit_logs; policy brand_point_policies;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'commission adjustments immutable'; END IF;
 IF TG_OP='UPDATE' THEN
  IF OLD.ledger_entry_id IS NOT NULL OR NEW.ledger_entry_id IS NULL OR OLD.creation_xid<>pg_current_xact_id() OR
   (to_jsonb(NEW)-'ledger_entry_id') IS DISTINCT FROM (to_jsonb(OLD)-'ledger_entry_id') THEN RAISE EXCEPTION 'commission adjustment only binds its ledger once in its creation transaction'; END IF;
  RETURN NEW;
 END IF;
 SELECT * INTO t FROM commission_payment_targets WHERE brand_id=NEW.brand_id AND id=NEW.target_id;
 SELECT * INTO p FROM commission_payments WHERE brand_id=NEW.brand_id AND id=NEW.payment_id;
 PERFORM 1 FROM commission_cycles WHERE brand_id=NEW.brand_id AND id=p.cycle_id FOR UPDATE NOWAIT;
 IF NOT FOUND OR p.state IS DISTINCT FROM 'paid' OR t.state IS DISTINCT FROM 'paid' OR t.payment_id IS DISTINCT FROM NEW.payment_id OR
  NOT commission_payment_evidence_current(NEW.brand_id,p.cycle_id,p.run_id,p.evidence_epoch) THEN RAISE EXCEPTION 'manual adjustment requires complete current evidence, not a blocked correction'; END IF;
 PERFORM 1 FROM brands WHERE id=NEW.brand_id AND status<>'disabled' FOR SHARE NOWAIT;
 IF NOT FOUND THEN RAISE EXCEPTION 'brand does not admit manual commission adjustment'; END IF;
 PERFORM 1 FROM brand_commission_payment_policies WHERE brand_id=NEW.brand_id AND enabled FOR SHARE NOWAIT;
 IF NOT FOUND THEN RAISE EXCEPTION 'commission financial writes disabled'; END IF;
 SELECT * INTO h FROM commission_adjustment_heads WHERE brand_id=NEW.brand_id AND target_id=NEW.target_id FOR UPDATE NOWAIT;
 IF h.target_id IS NULL OR NEW.version<>h.version+1 OR NEW.points_before<>h.points OR NEW.ledger_entry_id IS NOT NULL THEN RAISE EXCEPTION 'manual adjustment needs current target version'; END IF;
 SELECT * INTO policy FROM brand_point_policies WHERE brand_id=NEW.brand_id FOR SHARE NOWAIT;
 IF policy.version IS DISTINCT FROM NEW.point_policy_version OR (policy.max_adjustment_points IS NOT NULL AND abs(NEW.delta_points::numeric)>policy.max_adjustment_points) THEN RAISE EXCEPTION 'commission adjustment exceeds captured point policy'; END IF;
 SELECT * INTO log FROM audit_logs WHERE id=NEW.audit_log_id;
 IF log.id IS NULL OR log.brand_id IS DISTINCT FROM NEW.brand_id OR log.actor_type<>'admin' OR log.actor_id IS DISTINCT FROM NEW.created_by OR
  log.action<>'commission.adjustment.create' OR log.resource_type<>'commission_adjustment' OR log.resource_id IS DISTINCT FROM NEW.id OR log.reason<>NEW.reason OR
  log.before_json->>'version' IS DISTINCT FROM h.version::text OR log.before_json->>'points' IS DISTINCT FROM NEW.points_before::text OR
  log.after_json->>'version' IS DISTINCT FROM NEW.version::text OR log.after_json->>'points' IS DISTINCT FROM NEW.points_after::text OR
  log.after_json->>'delta_points' IS DISTINCT FROM NEW.delta_points::text OR log.after_json->>'target_id' IS DISTINCT FROM NEW.target_id::text OR
  log.after_json->>'payment_id' IS DISTINCT FROM NEW.payment_id::text THEN RAISE EXCEPTION 'manual commission adjustment needs matching single-operator approval audit'; END IF;
 NEW.creation_xid:=pg_current_xact_id();RETURN NEW;
END $$;

CREATE FUNCTION guard_commission_adjustment_credit() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE a commission_adjustments; t commission_payment_targets; p commission_payments; expected jsonb; policy brand_point_policies; total_after numeric;
BEGIN
 IF NEW.entry_type<>'commission_adjustment' AND NEW.reference_type<>'commission_adjustment' THEN RETURN NEW; END IF;
 SELECT * INTO a FROM commission_adjustments WHERE brand_id=NEW.brand_id AND id=NEW.reference_id FOR UPDATE NOWAIT;
 SELECT * INTO t FROM commission_payment_targets WHERE brand_id=NEW.brand_id AND id=a.target_id;
 SELECT * INTO p FROM commission_payments WHERE brand_id=NEW.brand_id AND id=a.payment_id FOR UPDATE NOWAIT;
 PERFORM 1 FROM commission_cycles WHERE brand_id=NEW.brand_id AND id=p.cycle_id FOR UPDATE NOWAIT;
 IF NOT FOUND OR a.id IS NULL OR a.creation_xid<>pg_current_xact_id() OR a.ledger_entry_id IS NOT NULL OR p.state IS DISTINCT FROM 'paid' OR
  NOT commission_payment_evidence_current(NEW.brand_id,p.cycle_id,p.run_id,p.evidence_epoch) THEN RAISE EXCEPTION 'adjustment posting requires its current approved business record'; END IF;
 PERFORM 1 FROM brand_commission_payment_policies WHERE brand_id=NEW.brand_id AND enabled FOR SHARE NOWAIT;
 IF NOT FOUND THEN RAISE EXCEPTION 'adjustment financial switch disabled'; END IF;
 SELECT * INTO policy FROM brand_point_policies WHERE brand_id=NEW.brand_id FOR SHARE NOWAIT;
 SELECT sum(bucket.value::text::numeric) INTO total_after FROM jsonb_each(NEW.after_snapshot) src CROSS JOIN LATERAL jsonb_each_text(src.value) bucket;
 IF policy.version IS DISTINCT FROM a.point_policy_version OR policy.max_balance_points IS NOT NULL AND total_after>policy.max_balance_points THEN RAISE EXCEPTION 'adjustment current balance policy mismatch'; END IF;
 expected:=jsonb_set(point_zero_snapshot(),'{commission,available}',to_jsonb(a.delta_points::text));
 IF NEW.entry_type<>'commission_adjustment' OR NEW.reference_type<>'commission_adjustment' OR NEW.member_id IS DISTINCT FROM t.member_id OR
  NEW.actor_type<>'admin' OR NEW.actor_id IS DISTINCT FROM a.created_by OR NEW.operation_key IS DISTINCT FROM 'commission-adjustment:'||a.id::text OR NEW.reversal_of IS NOT NULL OR
  NEW.delta_snapshot IS DISTINCT FROM expected OR NEW.source_allocation IS DISTINCT FROM jsonb_build_array(jsonb_build_object('source','commission','state','available','points',abs(a.delta_points)::text)) THEN RAISE EXCEPTION 'adjustment must post exact C-available difference for original beneficiary'; END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_commission_adjustment_head() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE t commission_payment_targets; a commission_adjustments;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'commission adjustment head cannot be deleted'; END IF;
 SELECT * INTO t FROM commission_payment_targets WHERE brand_id=NEW.brand_id AND id=NEW.target_id;
 IF t.state IS DISTINCT FROM 'paid' THEN RAISE EXCEPTION 'commission adjustment head requires completed target'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.version<>1 OR NEW.points<>t.points OR NEW.last_adjustment_id IS NOT NULL THEN RAISE EXCEPTION 'commission head starts at original paid amount'; END IF;
 ELSE
  IF NEW.brand_id<>OLD.brand_id OR NEW.target_id<>OLD.target_id OR NEW.version<>OLD.version+1 OR NEW.points=OLD.points OR NEW.last_adjustment_id IS NULL THEN RAISE EXCEPTION 'commission adjustment version/identity mismatch'; END IF;
  SELECT * INTO a FROM commission_adjustments WHERE brand_id=NEW.brand_id AND target_id=NEW.target_id AND id=NEW.last_adjustment_id;
  IF a.id IS NULL OR a.version<>NEW.version OR a.points_before<>OLD.points OR a.points_after<>NEW.points OR a.ledger_entry_id IS NULL THEN RAISE EXCEPTION 'head requires completed immutable adjustment'; END IF;
 END IF; RETURN NEW;
END $$;

CREATE FUNCTION guard_commission_allocation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE c commission_calculations; node jsonb; next_node jsonb; rate numeric;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'commission allocations immutable'; END IF;
 SELECT * INTO c FROM commission_calculations WHERE brand_id=NEW.brand_id AND cycle_id=NEW.cycle_id AND run_id=NEW.run_id AND id=NEW.calculation_id;
 SELECT value INTO node FROM jsonb_array_elements(c.rule_snapshot->'agent_path') WHERE value->>'id'=NEW.agent_id::text;
 SELECT value INTO next_node FROM jsonb_array_elements(c.rule_snapshot->'agent_path') WHERE (value->>'depth')::int=(node->>'depth')::int+1;
 rate:=((node->'config'->>'ratio')::numeric-coalesce((next_node->'config'->>'ratio')::numeric,0))*1000000;
 IF c.id IS NULL OR c.reason<>'eligible' OR c.order_id<>NEW.order_id OR node IS NULL OR node->>'member_id' IS DISTINCT FROM NEW.member_id::text OR
  rate<0 OR NEW.numerator::numeric*1000000 IS DISTINCT FROM c.base_points::numeric*rate*NEW.denominator::numeric THEN RAISE EXCEPTION 'commission differential allocation mismatch'; END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_commission_calculation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE c commission_cycles; r commission_runs; o bet_orders; p periods; calc settlement_calculations;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'commission calculations immutable'; END IF;
 SELECT * INTO c FROM commission_cycles WHERE brand_id=NEW.brand_id AND id=NEW.cycle_id;
 SELECT * INTO r FROM commission_runs WHERE id=NEW.run_id;
 SELECT * INTO o FROM bet_orders WHERE brand_id=NEW.brand_id AND id=NEW.order_id;
 SELECT * INTO p FROM periods WHERE brand_id=NEW.brand_id AND id=o.period_id;
 IF c.state<>'calculating' OR c.current_run_id IS DISTINCT FROM NEW.run_id OR r.state<>'calculating' OR o.id IS NULL OR
  NEW.rule_snapshot IS DISTINCT FROM o.commission_rule_snapshot OR NEW.status<>o.status OR NEW.member_id<>o.brand_member_id OR NEW.account_id<>o.account_id OR
  NEW.stake_points<>o.total_points OR NEW.prize_points<>o.prize_points OR r.evidence_epoch<>c.evidence_epoch OR
  NOT EXISTS(SELECT 1 FROM audit_logs a WHERE a.id=NEW.audit_log_id AND a.brand_id=NEW.brand_id AND a.action='commission.cycle.calculation_page' AND a.resource_type='commission_cycle' AND a.resource_id=NEW.cycle_id) THEN RAISE EXCEPTION 'commission calculation identity/evidence mismatch'; END IF;
 IF o.status IN('won','lost') THEN
  IF NEW.reason NOT IN('eligible','policy_disabled','unattributed','other_cycle') THEN RAISE EXCEPTION 'commission final-order disposition mismatch'; END IF;
  SELECT * INTO calc FROM settlement_calculations WHERE brand_id=NEW.brand_id AND id=o.settlement_calculation_id;
  IF p.status<>'settled' OR NEW.job_id IS DISTINCT FROM p.current_settlement_job_id OR NEW.calculation_id IS DISTINCT FROM calc.id OR
   NOT EXISTS(SELECT 1 FROM settlement_jobs j WHERE j.id=NEW.job_id AND j.brand_id=NEW.brand_id AND j.state='completed' AND j.generation=NEW.generation AND j.draw_result_id=p.draw_result_id) OR
   calc.job_id IS DISTINCT FROM NEW.job_id OR calc.order_id IS DISTINCT FROM NEW.order_id OR calc.prize_points<>NEW.prize_points OR
   calc.won<>(o.status='won') THEN RAISE EXCEPTION 'commission calculation final generation mismatch'; END IF;
 ELSE
  IF NEW.job_id IS NOT NULL OR NEW.calculation_id IS NOT NULL OR NEW.generation IS NOT NULL OR NEW.base_points<>0 OR
   NOT ((o.status IN('bet_cancelled','judged_cancelled') AND NEW.reason='cancelled') OR (o.status='abnormal' AND NEW.reason='abnormal')) THEN RAISE EXCEPTION 'commission excluded order mismatch'; END IF;
 END IF;
 IF NEW.reason='eligible' THEN
  IF NEW.rule_snapshot->'financial_policy'->'config'->'enabled'<>'true'::jsonb OR jsonb_array_length(NEW.rule_snapshot->'agent_path')=0 OR
   NEW.base_points IS DISTINCT FROM (CASE WHEN NEW.rule_snapshot->'agent_path'->0->>'effective_mode'='turnover' OR o.status='lost' THEN o.total_points ELSE 0 END) THEN RAISE EXCEPTION 'commission eligible base mismatch'; END IF;
 ELSIF NEW.base_points<>0 THEN RAISE EXCEPTION 'excluded commission must have zero base'; END IF;
 IF NEW.reason='policy_disabled' AND NEW.rule_snapshot->'financial_policy'->'config'->'enabled'<>'false'::jsonb THEN RAISE EXCEPTION 'commission policy-disabled disposition mismatch'; END IF;
 IF NEW.reason='unattributed' AND (NEW.rule_snapshot->'financial_policy'->'config'->'enabled'<>'true'::jsonb OR jsonb_array_length(NEW.rule_snapshot->'agent_path')<>0) THEN RAISE EXCEPTION 'commission unattributed disposition mismatch'; END IF;
 IF NEW.reason='other_cycle' AND (NEW.rule_snapshot->'financial_policy'->'config'->'enabled'<>'true'::jsonb OR NEW.rule_snapshot->'financial_policy'->'config'->'calendar' IS NOT DISTINCT FROM c.calendar) THEN RAISE EXCEPTION 'commission other-cycle disposition mismatch'; END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_commission_correction_balance_head() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE t commission_correction_execution_targets; p commission_correction_plan_targets;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'correction actual net head retained'; END IF;
 SELECT * INTO t FROM commission_correction_execution_targets WHERE brand_id=NEW.brand_id AND id=NEW.last_execution_target_id;
 SELECT * INTO p FROM commission_correction_plan_targets WHERE brand_id=NEW.brand_id AND id=t.plan_target_id;
 IF t.id IS NULL OR t.state<>'applied' OR t.creation_xid<>pg_current_xact_id() OR t.cycle_id<>NEW.cycle_id OR t.agent_id<>NEW.agent_id OR t.member_id<>NEW.member_id OR
  t.points_after<>NEW.points OR t.financial_version<>NEW.version OR NEW.original_target_id IS DISTINCT FROM p.original_target_id THEN RAISE EXCEPTION 'actual net head requires current transaction financial target'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.version<>1 OR t.base_financial_version IS NOT NULL THEN RAISE EXCEPTION 'first actual net head starts from original awarded basis'; END IF;
 ELSE
  IF NEW.brand_id<>OLD.brand_id OR NEW.cycle_id<>OLD.cycle_id OR NEW.agent_id<>OLD.agent_id OR NEW.member_id<>OLD.member_id OR NEW.original_target_id IS DISTINCT FROM OLD.original_target_id OR
   NEW.version<>OLD.version+1 OR t.base_financial_version IS DISTINCT FROM OLD.version OR t.points_before<>OLD.points OR p.previous_correction_target_id IS DISTINCT FROM OLD.last_execution_target_id THEN RAISE EXCEPTION 'actual net chain cannot skip or replace a prior posted difference'; END IF;
 END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_commission_correction_cycle_hold() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE s commission_correction_execution_steps; x commission_correction_executions;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'correction cycle hold history retained'; END IF;
 PERFORM 1 FROM commission_cycles WHERE brand_id=NEW.brand_id AND id=NEW.cycle_id FOR UPDATE NOWAIT;
 IF NOT FOUND THEN RAISE EXCEPTION 'correction hold cycle mismatch'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.version<>1 OR NEW.active OR NEW.blocked_execution_id IS NOT NULL OR NEW.audit_log_id IS NOT NULL THEN RAISE EXCEPTION 'correction hold begins released without fabricated event'; END IF;
 ELSE
  SELECT * INTO s FROM commission_correction_execution_steps WHERE brand_id=NEW.brand_id AND audit_log_id=NEW.audit_log_id;
  SELECT * INTO x FROM commission_correction_executions WHERE brand_id=NEW.brand_id AND id=s.execution_id;
  IF NEW.brand_id<>OLD.brand_id OR NEW.cycle_id<>OLD.cycle_id OR NEW.version<>OLD.version+1 OR s.id IS NULL OR x.cycle_id<>NEW.cycle_id THEN RAISE EXCEPTION 'hold requires matching cycle action'; END IF;
  IF NEW.active THEN
   IF OLD.active OR s.operation<>'pause' OR s.actor_type<>'system' OR s.error_code<>'COMMISSION_CORRECTION_AVAILABLE_INSUFFICIENT' OR NEW.blocked_execution_id IS DISTINCT FROM x.id THEN RAISE EXCEPTION 'cycle pause requires real insufficient attempt'; END IF;
  ELSE
   IF NOT OLD.active OR s.operation<>'continue' OR s.actor_type<>'admin' OR NEW.blocked_execution_id IS DISTINCT FROM OLD.blocked_execution_id THEN RAISE EXCEPTION 'cycle hold needs explicit administrator release'; END IF;
  END IF;
  NEW.updated_at:=clock_timestamp();
 END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_commission_correction_execution() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE p commission_correction_plans; s commission_correction_execution_steps; a audit_logs; held boolean; expected text; t commission_correction_plan_targets; available bigint;
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

CREATE FUNCTION guard_commission_correction_execution_step() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE x commission_correction_executions; a audit_logs;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'correction execution steps immutable'; END IF;
 SELECT * INTO x FROM commission_correction_executions WHERE brand_id=NEW.brand_id AND id=NEW.execution_id FOR UPDATE NOWAIT;
 SELECT * INTO a FROM audit_logs WHERE id=NEW.audit_log_id;
 IF x.id IS NULL OR NEW.version<>(CASE WHEN NEW.operation='create' THEN x.version ELSE x.version+1 END) OR
  NEW.operation='create' AND (x.version<>1 OR x.creation_xid<>pg_current_xact_id() OR NEW.to_state<>x.state OR NEW.paused_plan_target_id IS DISTINCT FROM x.paused_plan_target_id OR NEW.error_code IS DISTINCT FROM x.last_error_code) OR
  NEW.operation<>'create' AND NEW.from_state IS DISTINCT FROM x.state OR a.id IS NULL OR a.brand_id IS DISTINCT FROM NEW.brand_id OR
  a.action<>'commission.correction_execution.'||NEW.operation OR a.resource_type<>'commission_correction_execution' OR a.resource_id IS DISTINCT FROM NEW.execution_id OR
  a.actor_type<>NEW.actor_type OR a.actor_id IS DISTINCT FROM NEW.actor_id OR a.request_id<>NEW.request_id OR a.reason<>NEW.reason OR
  a.after_json IS DISTINCT FROM jsonb_build_object('version',NEW.version,'state',NEW.to_state,'target_id',NEW.target_id,'paused_plan_target_id',NEW.paused_plan_target_id,'error_code',NEW.error_code) OR
  a.before_json IS DISTINCT FROM (CASE WHEN NEW.operation='create' THEN 'null'::jsonb ELSE jsonb_build_object('version',x.version,'state',x.state) END) THEN RAISE EXCEPTION 'correction execution step/audit mismatch'; END IF;
 IF NEW.operation IN('approve','continue','retry') THEN
  IF NEW.actor_type<>'admin' OR NOT EXISTS(SELECT 1 FROM admin_accounts WHERE id=NEW.actor_id AND status='active' AND NOT is_super_admin) THEN RAISE EXCEPTION 'correction action requires current ordinary admin'; END IF;
 ELSIF NEW.actor_type<>'system' OR NEW.actor_id IS NOT NULL THEN RAISE EXCEPTION 'correction financial work requires system actor'; END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_commission_correction_execution_target() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE x commission_correction_executions; p commission_correction_plans; t commission_correction_plan_targets; h commission_correction_balance_heads; s commission_correction_execution_steps; a audit_logs; base bigint;
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

CREATE FUNCTION guard_commission_correction_notification_outbox() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF TG_OP='DELETE' THEN
  IF OLD.event_type='commission.corrected' THEN RAISE EXCEPTION 'correction notification event immutable'; END IF;
  RETURN OLD;
 END IF;
 IF TG_OP='UPDATE' THEN
  IF OLD.event_type='commission.corrected' OR NEW.event_type='commission.corrected' THEN
   IF (to_jsonb(NEW)-'published_at') IS DISTINCT FROM (to_jsonb(OLD)-'published_at') THEN RAISE EXCEPTION 'correction notification event immutable except published_at'; END IF;
   IF NOT valid_commission_correction_notification_event(NEW.brand_id,NEW.event_type,NEW.aggregate_id,NEW.payload) THEN
    RAISE EXCEPTION 'correction notification requires immutable posting evidence'; END IF;
  END IF;
  RETURN NEW;
 END IF;
 IF NEW.event_type='commission.corrected' AND
  (NOT valid_commission_correction_notification_event(NEW.brand_id,NEW.event_type,NEW.aggregate_id,NEW.payload) OR NOT EXISTS(
   SELECT 1 FROM commission_correction_execution_targets WHERE brand_id=NEW.brand_id AND id=NEW.aggregate_id AND creation_xid=pg_current_xact_id())) THEN
  RAISE EXCEPTION 'correction notification requires current real nonzero posting';
 END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_commission_correction_plan() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE c commission_cycles; pay commission_payments; step commission_correction_plan_steps; a audit_logs; n bigint; before_sum numeric; after_sum numeric; credits numeric; debits numeric;
BEGIN
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
  IF (to_jsonb(NEW)-(ARRAY['state','version','planned_count','cursor_agent_id','last_audit_log_id','last_error_code','updated_at','next_work_at'])) IS DISTINCT FROM
   (to_jsonb(OLD)-(ARRAY['state','version','planned_count','cursor_agent_id','last_audit_log_id','last_error_code','updated_at','next_work_at'])) OR NEW.version<>OLD.version+1
   THEN RAISE EXCEPTION 'correction plan identity and amounts immutable'; END IF;
  SELECT * INTO step FROM commission_correction_plan_steps WHERE brand_id=NEW.brand_id AND plan_id=NEW.id AND version=NEW.version;
  IF step.id IS NULL OR step.from_state IS DISTINCT FROM OLD.state OR step.to_state<>NEW.state OR step.audit_log_id<>NEW.last_audit_log_id OR
   step.planned_count<>NEW.planned_count OR step.cursor_agent_id IS DISTINCT FROM NEW.cursor_agent_id OR step.error_code IS DISTINCT FROM NEW.last_error_code THEN RAISE EXCEPTION 'correction plan update requires matching immutable step'; END IF;
  IF NOT ((step.operation='page' AND OLD.state='planning' AND NEW.state='planning' AND NEW.planned_count>OLD.planned_count AND NEW.planned_count<=OLD.planned_count+100) OR
   (step.operation='ready' AND OLD.state='planning' AND NEW.state='ready' AND NEW.planned_count=OLD.planned_count) OR
   (step.operation='invalidate' AND OLD.state<>'stale' AND NEW.state='stale' AND NEW.planned_count=OLD.planned_count) OR
   (step.operation='fail' AND OLD.state='planning' AND NEW.state='failed' AND NEW.planned_count=OLD.planned_count) OR
   (step.operation='retry' AND OLD.state='failed' AND NEW.state='planning' AND NEW.planned_count=OLD.planned_count)) THEN RAISE EXCEPTION 'correction plan transition invalid'; END IF;
  IF step.operation NOT IN('invalidate','fail') THEN
   PERFORM 1 FROM brands WHERE id=NEW.brand_id AND status<>'disabled' FOR SHARE NOWAIT;
   IF NOT FOUND THEN RAISE EXCEPTION 'correction plan brand disabled'; END IF;
   PERFORM 1 FROM brand_commission_payment_policies WHERE brand_id=NEW.brand_id AND enabled FOR SHARE NOWAIT;
   IF NOT FOUND OR NOT commission_payment_evidence_current(NEW.brand_id,NEW.cycle_id,NEW.run_id,NEW.evidence_epoch) OR NOT commission_correction_source_valid(NEW.brand_id,NEW.payment_id,NEW.run_id) THEN RAISE EXCEPTION 'correction plan evidence unavailable'; END IF;
  END IF;
  IF step.operation='invalidate' AND commission_payment_evidence_current(NEW.brand_id,NEW.cycle_id,NEW.run_id,NEW.evidence_epoch) THEN RAISE EXCEPTION 'cannot stale a current correction plan'; END IF;
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

CREATE FUNCTION guard_commission_correction_plan_step() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE p commission_correction_plans; a audit_logs;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'commission correction plan steps immutable'; END IF;
 SELECT * INTO p FROM commission_correction_plans WHERE brand_id=NEW.brand_id AND id=NEW.plan_id FOR UPDATE NOWAIT;
 SELECT * INTO a FROM audit_logs WHERE id=NEW.audit_log_id;
 IF p.id IS NULL OR NEW.version<>(CASE WHEN NEW.operation='create' THEN p.version ELSE p.version+1 END) OR
  NEW.operation='create' AND (p.version<>1 OR p.creation_xid<>pg_current_xact_id() OR NEW.to_state<>p.state OR NEW.planned_count<>p.planned_count OR NEW.cursor_agent_id IS DISTINCT FROM p.cursor_agent_id OR NEW.error_code IS DISTINCT FROM p.last_error_code) OR NEW.operation<>'create' AND NEW.from_state IS DISTINCT FROM p.state OR
  a.id IS NULL OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.resource_type<>'commission_correction_plan' OR a.resource_id IS DISTINCT FROM NEW.plan_id OR
  a.action<>'commission.correction_plan.'||NEW.operation OR a.actor_type<>NEW.actor_type OR a.actor_id IS DISTINCT FROM NEW.actor_id OR a.reason<>NEW.reason OR a.request_id<>NEW.request_id OR
  a.after_json IS DISTINCT FROM jsonb_build_object('version',NEW.version,'state',NEW.to_state,'planned_count',NEW.planned_count::text,'cursor_agent_id',NEW.cursor_agent_id,'error_code',NEW.error_code) OR
  a.before_json IS DISTINCT FROM (CASE WHEN NEW.operation='create' THEN 'null'::jsonb ELSE jsonb_build_object('version',p.version,'state',p.state) END)
 THEN RAISE EXCEPTION 'commission correction step requires exact live audit and version'; END IF;
 IF NEW.operation='retry' THEN
  IF NEW.actor_type<>'admin' OR NOT EXISTS(SELECT 1 FROM admin_accounts WHERE id=NEW.actor_id AND status='active' AND NOT is_super_admin) THEN RAISE EXCEPTION 'retry requires current ordinary administrator'; END IF;
 ELSIF NEW.actor_type<>'system' OR NEW.actor_id IS NOT NULL THEN RAISE EXCEPTION 'preparation steps require system actor'; END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_commission_correction_plan_target() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE p commission_correction_plans; candidate record; step commission_correction_plan_steps;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'correction plan targets immutable'; END IF;
 SELECT * INTO p FROM commission_correction_plans WHERE brand_id=NEW.brand_id AND id=NEW.plan_id FOR UPDATE NOWAIT;
 SELECT * INTO step FROM commission_correction_plan_steps WHERE plan_id=NEW.plan_id AND version=NEW.plan_version;
 IF p.state IS DISTINCT FROM 'planning' OR NEW.creation_xid<>pg_current_xact_id() OR NEW.plan_version<>p.version+1 OR NEW.agent_id<=p.cursor_agent_id OR
  step.operation IS DISTINCT FROM 'page' OR step.audit_log_id IS DISTINCT FROM NEW.creation_audit_log_id OR
  NOT commission_payment_evidence_current(p.brand_id,p.cycle_id,p.run_id,p.evidence_epoch) OR NOT commission_correction_source_valid(p.brand_id,p.payment_id,p.run_id) OR
  NOT commission_correction_heads_valid(p.brand_id,p.cycle_id) THEN RAISE EXCEPTION 'correction target requires live audited preparation'; END IF;
 SELECT * INTO candidate FROM commission_correction_candidates_v2(p.brand_id,p.payment_id,p.run_id) WHERE agent_id=NEW.agent_id;
 IF NOT FOUND OR NEW.member_id<>candidate.member_id OR NEW.original_target_id IS DISTINCT FROM candidate.original_target_id OR NEW.earning_id IS DISTINCT FROM candidate.earning_id OR
  NEW.adjustment_version IS DISTINCT FROM candidate.adjustment_version OR NEW.points_before<>candidate.points_before OR NEW.points_after<>candidate.points_after OR NEW.delta_points<>candidate.delta_points OR
  NEW.previous_correction_target_id IS DISTINCT FROM candidate.previous_correction_target_id OR NEW.financial_version IS DISTINCT FROM candidate.financial_version THEN RAISE EXCEPTION 'correction target must match actual financial net and calculation'; END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_commission_correction_policy() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE a audit_logs;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'correction financial policy retained'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.version<>1 OR NEW.enabled OR NEW.audit_log_id IS NOT NULL THEN RAISE EXCEPTION 'correction rollout begins disabled'; END IF;
 ELSE
  SELECT * INTO a FROM audit_logs WHERE id=NEW.audit_log_id;
  IF NEW.brand_id<>OLD.brand_id OR NEW.version<>OLD.version+1 OR a.id IS NULL OR a.brand_id IS DISTINCT FROM NEW.brand_id OR
   a.action<>'commission.correction_policy.update' OR a.resource_type<>'commission_correction_policy' OR a.resource_id IS DISTINCT FROM NEW.brand_id OR a.actor_type<>'admin' OR
   NOT EXISTS(SELECT 1 FROM admin_accounts WHERE id=a.actor_id AND status='active' AND NOT is_super_admin) OR
   a.before_json IS DISTINCT FROM jsonb_build_object('version',OLD.version,'enabled',OLD.enabled) OR a.after_json IS DISTINCT FROM jsonb_build_object('version',NEW.version,'enabled',NEW.enabled) THEN RAISE EXCEPTION 'correction rollout update requires current ordinary admin audit'; END IF;
  NEW.updated_at:=clock_timestamp();
 END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_commission_correction_policy_revision() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF TG_OP<>'INSERT' OR NOT EXISTS(SELECT 1 FROM brand_commission_correction_policies WHERE brand_id=NEW.brand_id AND version=NEW.version AND enabled=NEW.enabled AND audit_log_id IS NOT DISTINCT FROM NEW.audit_log_id) THEN RAISE EXCEPTION 'correction policy history immutable'; END IF; RETURN NEW;
END $$;

CREATE FUNCTION guard_commission_cycle() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE a audit_logs; o bet_orders; step commission_cycle_steps; added bigint;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'commission cycle history immutable'; END IF;
 IF TG_OP='INSERT' THEN
  PERFORM 1 FROM brand_commission_policies WHERE brand_id=NEW.brand_id FOR UPDATE;
  SELECT * INTO o FROM bet_orders WHERE brand_id=NEW.brand_id AND id=NEW.anchor_order_id;
  IF NOT FOUND OR o.commission_rule_snapshot IS NULL OR o.commission_rule_snapshot->'financial_policy'->'config'->'enabled' IS DISTINCT FROM 'true'::jsonb OR
   o.commission_rule_snapshot->'financial_policy'->'config'->'calendar' IS DISTINCT FROM NEW.calendar OR
   o.placed_at<NEW.window_from OR o.placed_at>=NEW.window_to OR NEW.window_to>clock_timestamp() OR
   NEW.state<>'enumerating' OR NEW.version<>1 OR NEW.evidence_epoch<>0 OR NEW.target_count<>0 OR NEW.scan_complete OR NEW.current_run_id IS NOT NULL OR
   NEW.manifest_cursor_at IS NOT NULL OR NEW.manifest_cursor_id IS NOT NULL THEN RAISE EXCEPTION 'invalid closed commission cycle admission'; END IF;
  IF NOT EXISTS(SELECT 1 FROM commission_calendar_window(NEW.calendar,o.placed_at) w WHERE w.window_from=NEW.window_from AND w.window_to=NEW.window_to) THEN
   RAISE EXCEPTION 'commission cycle calendar window mismatch'; END IF;
  IF NEW.creation_actor_type='system' AND NOT EXISTS(SELECT 1 FROM commission_discovery d WHERE d.brand_id=NEW.brand_id AND d.id=NEW.anchor_order_id AND
   d.state='pending' AND d.window_from=NEW.window_from AND d.window_to=NEW.window_to) THEN RAISE EXCEPTION 'system cycle requires discovered saved window'; END IF;
  SELECT * INTO a FROM audit_logs WHERE id=NEW.creation_audit_log_id;
  IF NOT FOUND OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.actor_type IS DISTINCT FROM NEW.creation_actor_type OR a.actor_id IS DISTINCT FROM NEW.created_by OR
   a.resource_type<>'commission_cycle' OR a.resource_id IS DISTINCT FROM NEW.id OR a.action<>'commission.cycle.create' OR a.reason<>NEW.reason OR
   a.after_json->>'version' IS DISTINCT FROM '1' OR a.after_json->>'state' IS DISTINCT FROM 'enumerating' OR a.after_json->>'anchor_order_id' IS DISTINCT FROM NEW.anchor_order_id::text OR
   (a.after_json->>'window_from')::timestamptz IS DISTINCT FROM NEW.window_from OR (a.after_json->>'window_to')::timestamptz IS DISTINCT FROM NEW.window_to OR
   a.after_json->'calendar' IS DISTINCT FROM NEW.calendar THEN RAISE EXCEPTION 'commission cycle needs matching creation audit'; END IF;
 ELSE
  IF (to_jsonb(NEW)-ARRAY['state','version','target_count','manifest_cursor_at','manifest_cursor_id','scan_complete','current_run_id','updated_at','next_work_at','last_error_code','evidence_epoch']) IS DISTINCT FROM
   (to_jsonb(OLD)-ARRAY['state','version','target_count','manifest_cursor_at','manifest_cursor_id','scan_complete','current_run_id','updated_at','next_work_at','last_error_code','evidence_epoch']) THEN RAISE EXCEPTION 'commission cycle identity immutable'; END IF;
  IF (to_jsonb(NEW)-ARRAY['evidence_epoch']) IS NOT DISTINCT FROM (to_jsonb(OLD)-ARRAY['evidence_epoch']) THEN
   IF NEW.evidence_epoch<>OLD.evidence_epoch+1 THEN RAISE EXCEPTION 'commission evidence epoch cannot rewind'; END IF;
   RETURN NEW;
  END IF;
  IF (to_jsonb(NEW)-ARRAY['next_work_at']) IS NOT DISTINCT FROM (to_jsonb(OLD)-ARRAY['next_work_at']) THEN RETURN NEW; END IF;
  IF NEW.version<>OLD.version+1 THEN RAISE EXCEPTION 'commission cycle version invalid'; END IF;
  SELECT * INTO step FROM commission_cycle_steps WHERE cycle_id=NEW.id AND version=NEW.version;
  IF NOT FOUND OR step.from_state<>OLD.state OR step.to_state<>NEW.state THEN RAISE EXCEPTION 'commission cycle update needs step'; END IF;
  IF NOT ((OLD.state='enumerating' AND NEW.state IN('enumerating','waiting','failed')) OR
   (OLD.state='waiting' AND NEW.state IN('calculating','failed')) OR
   (OLD.state='calculating' AND NEW.state IN('calculating','summarizing','waiting','failed')) OR
   (OLD.state='summarizing' AND NEW.state IN('summarizing','ready','waiting','failed')) OR
   (OLD.state='ready' AND NEW.state='waiting') OR (OLD.state='failed' AND NEW.state IN('enumerating','waiting'))) THEN RAISE EXCEPTION 'invalid commission cycle transition'; END IF;
  IF NEW.evidence_epoch<>OLD.evidence_epoch THEN RAISE EXCEPTION 'commission step cannot rewrite its evidence fence'; END IF;
  IF NEW.state='ready' AND NOT EXISTS(SELECT 1 FROM commission_runs r WHERE r.id=NEW.current_run_id AND r.cycle_id=NEW.id AND r.state='ready' AND r.evidence_epoch=NEW.evidence_epoch) THEN RAISE EXCEPTION 'commission cycle requires complete current evidence'; END IF;
  IF OLD.state='enumerating' AND NEW.state IN('enumerating','waiting') THEN
   SELECT count(*) INTO added FROM commission_cycle_targets WHERE cycle_id=NEW.id AND creation_xid=pg_current_xact_id();
   IF NEW.target_count<>OLD.target_count+added THEN RAISE EXCEPTION 'commission manifest count mismatch'; END IF;
   IF NEW.scan_complete AND EXISTS(SELECT 1 FROM bet_orders b WHERE b.brand_id=NEW.brand_id AND b.placed_at>=NEW.window_from AND b.placed_at<NEW.window_to AND
    NOT EXISTS(SELECT 1 FROM commission_cycle_targets t WHERE t.cycle_id=NEW.id AND t.order_id=b.id)) THEN RAISE EXCEPTION 'commission manifest incomplete'; END IF;
  ELSE
   IF (NEW.target_count,NEW.manifest_cursor_at,NEW.manifest_cursor_id,NEW.scan_complete) IS DISTINCT FROM
    (OLD.target_count,OLD.manifest_cursor_at,OLD.manifest_cursor_id,OLD.scan_complete) THEN RAISE EXCEPTION 'commission manifest sealed'; END IF;
  END IF;
 END IF;
 NEW.updated_at:=clock_timestamp();RETURN NEW;
END $$;

CREATE FUNCTION guard_commission_discovery() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE o bet_orders; a audit_logs; op text;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'commission discovery history immutable'; END IF;
 SELECT * INTO o FROM bet_orders WHERE brand_id=NEW.brand_id AND id=NEW.id;
 IF NOT FOUND OR o.commission_rule_snapshot IS NULL OR
  o.commission_rule_snapshot->'financial_policy'->'config'->'enabled' IS DISTINCT FROM 'true'::jsonb THEN
  RAISE EXCEPTION 'commission discovery requires enabled saved policy'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.state<>'pending' OR NEW.version<>1 OR NEW.cycle_id IS NOT NULL OR NEW.window_from IS NOT NULL OR NEW.last_error_code IS NOT NULL OR NEW.last_audit_log_id IS NOT NULL THEN
   RAISE EXCEPTION 'invalid initial commission discovery'; END IF;
  NEW.created_at:=clock_timestamp();NEW.updated_at:=NEW.created_at;RETURN NEW;
 END IF;
 IF (NEW.id,NEW.brand_id,NEW.created_at) IS DISTINCT FROM (OLD.id,OLD.brand_id,OLD.created_at) OR
  (OLD.window_from IS NOT NULL AND (NEW.window_from,NEW.window_to) IS DISTINCT FROM (OLD.window_from,OLD.window_to)) THEN
  RAISE EXCEPTION 'commission discovery identity immutable'; END IF;
 IF NEW.window_from IS NOT NULL AND (o.placed_at<NEW.window_from OR o.placed_at>=NEW.window_to) THEN
  RAISE EXCEPTION 'commission discovery window excludes original bet'; END IF;
 IF NEW.window_from IS NOT NULL AND OLD.window_from IS NULL AND NOT EXISTS(
  SELECT 1 FROM commission_calendar_window(o.commission_rule_snapshot->'financial_policy'->'config'->'calendar',o.placed_at) w
  WHERE w.window_from=NEW.window_from AND w.window_to=NEW.window_to) THEN RAISE EXCEPTION 'commission discovery calendar window mismatch'; END IF;
 IF NEW.state=OLD.state AND NEW.version=OLD.version AND NEW.cycle_id IS NOT DISTINCT FROM OLD.cycle_id AND
  NEW.last_error_code IS NOT DISTINCT FROM OLD.last_error_code AND NEW.last_audit_log_id IS NOT DISTINCT FROM OLD.last_audit_log_id THEN
  IF OLD.state<>'pending' THEN RAISE EXCEPTION 'only pending discovery can reschedule'; END IF;
 ELSE
  IF NEW.version<>OLD.version+1 OR NOT ((OLD.state='pending' AND NEW.state IN('registered','failed')) OR (OLD.state='failed' AND NEW.state='pending')) THEN
   RAISE EXCEPTION 'invalid commission discovery transition'; END IF;
  op:=CASE NEW.state WHEN 'registered' THEN 'register' WHEN 'failed' THEN 'failure' ELSE 'retry' END;
  SELECT * INTO a FROM audit_logs WHERE id=NEW.last_audit_log_id;
  IF NOT FOUND OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.resource_type<>'commission_discovery' OR a.resource_id IS DISTINCT FROM NEW.id OR
   a.action IS DISTINCT FROM 'commission.discovery.'||op OR a.before_json->>'version' IS DISTINCT FROM OLD.version::text OR
   a.before_json->>'state' IS DISTINCT FROM OLD.state OR a.after_json->>'version' IS DISTINCT FROM NEW.version::text OR
   a.after_json->>'state' IS DISTINCT FROM NEW.state OR a.after_json->>'cycle_id' IS DISTINCT FROM NEW.cycle_id::text OR
   a.after_json->>'last_error_code' IS DISTINCT FROM NEW.last_error_code OR
   ((op='retry') AND (a.actor_type<>'admin' OR a.actor_id IS NULL)) OR
   ((op<>'retry') AND (a.actor_type<>'system' OR a.actor_id IS NOT NULL)) THEN
   RAISE EXCEPTION 'commission discovery transition needs matching audit'; END IF;
 END IF;
 IF NEW.state='registered' AND NOT EXISTS(SELECT 1 FROM commission_cycles c WHERE c.brand_id=NEW.brand_id AND c.id=NEW.cycle_id AND
  c.window_from=NEW.window_from AND c.window_to=NEW.window_to AND c.window_to<=clock_timestamp()) THEN
  RAISE EXCEPTION 'discovery needs actual closed cycle'; END IF;
 NEW.updated_at:=clock_timestamp();RETURN NEW;
END $$;

CREATE FUNCTION guard_commission_earning() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE r commission_runs; c commission_cycles; micro numeric;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'commission earnings immutable'; END IF;
 SELECT * INTO r FROM commission_runs WHERE brand_id=NEW.brand_id AND cycle_id=NEW.cycle_id AND id=NEW.run_id;
 SELECT * INTO c FROM commission_cycles WHERE brand_id=NEW.brand_id AND id=NEW.cycle_id;
 SELECT sum(numerator::numeric*(1000000/denominator::numeric)) INTO micro FROM commission_allocations WHERE run_id=NEW.run_id AND agent_id=NEW.agent_id AND member_id=NEW.member_id;
 IF r.state<>'summarizing' OR c.state<>'summarizing' OR c.current_run_id IS DISTINCT FROM NEW.run_id OR r.evidence_epoch<>c.evidence_epoch OR micro IS NULL OR NEW.numerator::numeric*1000000 IS DISTINCT FROM micro*NEW.denominator::numeric OR
  NEW.points::numeric IS DISTINCT FROM floor((micro*2+1000000)/2000000) THEN RAISE EXCEPTION 'commission cycle earning total mismatch'; END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_commission_notification_outbox() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF TG_OP='DELETE' THEN
  IF OLD.event_type IN('commission.paid','commission.adjusted') THEN RAISE EXCEPTION 'commission notification event is immutable'; END IF;
  RETURN OLD;
 END IF;
 IF TG_OP='UPDATE' THEN
  IF OLD.event_type IN('commission.paid','commission.adjusted') OR NEW.event_type IN('commission.paid','commission.adjusted') THEN
   IF (to_jsonb(NEW)-'published_at') IS DISTINCT FROM (to_jsonb(OLD)-'published_at') THEN RAISE EXCEPTION 'commission notification event is immutable except published_at'; END IF;
   IF NEW.brand_id IS NULL OR NOT valid_commission_notification_event(NEW.brand_id,NEW.event_type,NEW.aggregate_id,NEW.payload) THEN RAISE EXCEPTION 'commission notification event must bind real immutable ledger evidence'; END IF;
  END IF;
  RETURN NEW;
 END IF;
 IF NEW.event_type IN('commission.paid','commission.adjusted') AND
  (NEW.brand_id IS NULL OR NOT valid_commission_notification_event(NEW.brand_id,NEW.event_type,NEW.aggregate_id,NEW.payload)) THEN
  RAISE EXCEPTION 'commission notification event must bind real immutable ledger evidence';
 END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_commission_payment() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE a audit_logs; mode text; expected_state text; credits boolean;
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
  IF (to_jsonb(NEW)-ARRAY['state','version','approved_by','approval_actor_type','approval_audit_log_id','last_audit_log_id','last_error_code','updated_at','next_work_at']) IS DISTINCT FROM
   (to_jsonb(OLD)-ARRAY['state','version','approved_by','approval_actor_type','approval_audit_log_id','last_audit_log_id','last_error_code','updated_at','next_work_at']) THEN RAISE EXCEPTION 'payment identity and totals immutable'; END IF;
  IF (to_jsonb(NEW)-ARRAY['next_work_at']) IS NOT DISTINCT FROM (to_jsonb(OLD)-ARRAY['next_work_at']) THEN RETURN NEW; END IF;
  IF NEW.version<>OLD.version+1 OR NOT((OLD.state='awaiting_approval' AND NEW.state IN('paying','stale','blocked')) OR
   (OLD.state='paying' AND NEW.state IN('paying','paid','failed','stale','blocked')) OR (OLD.state='failed' AND NEW.state IN('paying','stale','blocked')) OR
   (OLD.state='paid' AND NEW.state IN('blocked','stale'))) THEN RAISE EXCEPTION 'payment state/version conflict'; END IF;
  SELECT * INTO a FROM audit_logs WHERE id=NEW.last_audit_log_id;
  IF a.id IS NULL OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.resource_type<>'commission_payment' OR a.resource_id IS DISTINCT FROM NEW.id OR
   a.before_json->>'version' IS DISTINCT FROM OLD.version::text OR a.before_json->>'state' IS DISTINCT FROM OLD.state OR
   a.after_json->>'version' IS DISTINCT FROM NEW.version::text OR a.after_json->>'state' IS DISTINCT FROM NEW.state OR
   a.action NOT IN('commission.payment.approve','commission.payment.retry','commission.payment.post','commission.payment.complete','commission.payment.fail','commission.payment.invalidate') THEN RAISE EXCEPTION 'payment state transition audit mismatch'; END IF;
  IF NOT ((a.action='commission.payment.approve' AND OLD.state='awaiting_approval' AND NEW.state='paying' AND a.actor_type='admin' AND a.actor_id IS NOT NULL) OR
   (a.action='commission.payment.retry' AND OLD.state='failed' AND NEW.state='paying' AND a.actor_type='admin' AND a.actor_id IS NOT NULL) OR
   (a.action='commission.payment.post' AND OLD.state='paying' AND NEW.state='paying' AND a.actor_type='system' AND a.actor_id IS NULL) OR
   (a.action='commission.payment.complete' AND OLD.state='paying' AND NEW.state='paid' AND a.actor_type='system' AND a.actor_id IS NULL) OR
   (a.action='commission.payment.fail' AND OLD.state='paying' AND NEW.state='failed' AND a.actor_type='system' AND a.actor_id IS NULL) OR
   (a.action='commission.payment.invalidate' AND NEW.state IN('blocked','stale') AND a.actor_type='system' AND a.actor_id IS NULL)) THEN RAISE EXCEPTION 'payment operation/state/actor mismatch'; END IF;
  IF OLD.approval_audit_log_id IS NOT NULL AND (NEW.approved_by,NEW.approval_actor_type,NEW.approval_audit_log_id) IS DISTINCT FROM (OLD.approved_by,OLD.approval_actor_type,OLD.approval_audit_log_id) THEN RAISE EXCEPTION 'payment approval immutable'; END IF;
  IF OLD.state='awaiting_approval' AND NEW.state='paying' THEN
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

CREATE FUNCTION guard_commission_payment_credit() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE t commission_payment_targets; p commission_payments; expected jsonb;
BEGIN
 IF NEW.entry_type<>'commission' AND NEW.reference_type<>'commission_payment_target' THEN RETURN NEW; END IF;
 SELECT * INTO t FROM commission_payment_targets WHERE brand_id=NEW.brand_id AND id=NEW.reference_id FOR UPDATE NOWAIT;
 SELECT * INTO p FROM commission_payments WHERE brand_id=NEW.brand_id AND id=t.payment_id FOR UPDATE NOWAIT;
 PERFORM 1 FROM commission_cycles WHERE id=p.cycle_id AND brand_id=NEW.brand_id FOR UPDATE NOWAIT;
 IF NOT FOUND THEN RAISE EXCEPTION 'commission credit missing cycle'; END IF;
 PERFORM 1 FROM brand_commission_payment_policies WHERE brand_id=NEW.brand_id AND enabled FOR SHARE NOWAIT;
 IF NOT FOUND OR NOT commission_payment_evidence_current(NEW.brand_id,p.cycle_id,p.run_id,p.evidence_epoch) THEN RAISE EXCEPTION 'commission credit rollout or evidence unavailable'; END IF;
 PERFORM 1 FROM brands WHERE id=NEW.brand_id AND status<>'disabled' FOR SHARE NOWAIT;
 expected:=jsonb_set(point_zero_snapshot(),'{commission,available}',to_jsonb(t.points::text));
 IF NOT FOUND OR t.id IS NULL OR p.state IS DISTINCT FROM 'paying' OR p.approval_audit_log_id IS NULL OR t.state<>'pending' OR t.points<=0 OR
  NEW.entry_type<>'commission' OR NEW.reference_type<>'commission_payment_target' OR NEW.member_id IS DISTINCT FROM t.member_id OR
  NEW.operation_key IS DISTINCT FROM 'commission-payment:'||t.id::text OR NEW.actor_type<>'system' OR NEW.actor_id IS NOT NULL OR NEW.reversal_of IS NOT NULL OR
  NEW.delta_snapshot IS DISTINCT FROM expected OR NEW.source_allocation IS DISTINCT FROM jsonb_build_array(jsonb_build_object('source','commission','state','available','points',t.points::text)) THEN
  RAISE EXCEPTION 'commission credit must match unique approved target'; END IF; RETURN NEW;
END $$;

CREATE FUNCTION guard_commission_payment_policy() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE a audit_logs;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'commission payment policy cannot be deleted'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.version<>1 OR NEW.enabled OR NEW.audit_log_id IS NOT NULL THEN RAISE EXCEPTION 'payment policy starts disabled'; END IF;
 ELSE
  IF NEW.brand_id<>OLD.brand_id OR NEW.version<>OLD.version+1 THEN RAISE EXCEPTION 'payment policy version mismatch'; END IF;
  SELECT * INTO a FROM audit_logs WHERE id=NEW.audit_log_id;
  IF a.id IS NULL OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.actor_type<>'admin' OR a.actor_id IS NULL OR
   a.action<>'commission.payment_policy.update' OR a.resource_type<>'commission_payment_policy' OR a.resource_id IS DISTINCT FROM NEW.brand_id OR
   a.before_json->>'version' IS DISTINCT FROM OLD.version::text OR a.before_json->'enabled' IS DISTINCT FROM to_jsonb(OLD.enabled) OR
   a.after_json->>'version' IS DISTINCT FROM NEW.version::text OR a.after_json->'enabled' IS DISTINCT FROM to_jsonb(NEW.enabled) THEN
   RAISE EXCEPTION 'payment policy requires matching audit'; END IF;
 END IF;
 NEW.updated_at:=clock_timestamp(); RETURN NEW;
END $$;

CREATE FUNCTION guard_commission_payment_policy_revision() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF TG_OP<>'INSERT' OR NOT EXISTS(SELECT 1 FROM brand_commission_payment_policies p WHERE p.brand_id=NEW.brand_id AND p.version=NEW.version AND p.enabled=NEW.enabled AND p.audit_log_id IS NOT DISTINCT FROM NEW.audit_log_id) THEN
  RAISE EXCEPTION 'payment policy revisions immutable and bound'; END IF; RETURN NEW;
END $$;

CREATE FUNCTION guard_commission_payment_target() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE p commission_payments; e commission_earnings; a audit_logs;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'commission payment target cannot be deleted'; END IF;
 SELECT * INTO p FROM commission_payments WHERE brand_id=NEW.brand_id AND id=NEW.payment_id FOR UPDATE NOWAIT;
 SELECT * INTO e FROM commission_earnings WHERE id=NEW.earning_id;
 IF p.state IS DISTINCT FROM 'paying' OR (e.brand_id,e.cycle_id,e.run_id,e.member_id,e.agent_id,e.points) IS DISTINCT FROM
  (NEW.brand_id,p.cycle_id,p.run_id,NEW.member_id,NEW.agent_id,NEW.points) THEN RAISE EXCEPTION 'payment target must bind approved earning'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.state<>'pending' OR NEW.ledger_entry_id IS NOT NULL OR NEW.audit_log_id IS NOT NULL OR NEW.paid_at IS NOT NULL OR NEW.payment_version<>p.version THEN RAISE EXCEPTION 'payment target starts pending at current version'; END IF;
 ELSE
  IF OLD.state<>'pending' OR NEW.state<>'paid' OR
   (to_jsonb(NEW)-ARRAY['state','ledger_entry_id','audit_log_id','paid_at']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['state','ledger_entry_id','audit_log_id','paid_at']) THEN RAISE EXCEPTION 'payment target identity immutable'; END IF;
  SELECT * INTO a FROM audit_logs WHERE id=NEW.audit_log_id;
  IF a.id IS NULL OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.actor_type<>'system' OR a.actor_id IS NOT NULL OR
   a.action<>'commission.payment.target' OR a.resource_type<>'commission_payment_target' OR a.resource_id IS DISTINCT FROM NEW.id OR
   a.after_json->>'points' IS DISTINCT FROM NEW.points::text OR a.after_json->>'ledger_entry_id' IS DISTINCT FROM NEW.ledger_entry_id::text THEN RAISE EXCEPTION 'payment target audit mismatch'; END IF;
 END IF; RETURN NEW;
END $$;

CREATE FUNCTION guard_commission_policy() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE agency jsonb;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'commission policy cannot be deleted'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.version<>1 OR NEW.config<>default_commission_policy() THEN RAISE EXCEPTION 'initial disabled commission policy required'; END IF;
 ELSE
  IF (to_jsonb(NEW)-ARRAY['version','config','updated_at']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['version','config','updated_at']) OR
   NEW.version<>OLD.version+1 THEN RAISE EXCEPTION 'commission policy identity/version immutable'; END IF;
 END IF;
 SELECT config INTO agency FROM brand_agent_policies WHERE brand_id=NEW.brand_id FOR SHARE;
 IF NEW.config->'enabled'='true'::jsonb AND
  (NOT FOUND OR agency->'enabled'<>'true'::jsonb OR NEW.config->'calendar'->>'cycle' IS DISTINCT FROM agency->>'cycle') THEN
  RAISE EXCEPTION 'commission policy needs matching enabled agency policy';
 END IF;
 NEW.updated_at:=clock_timestamp(); RETURN NEW;
END $$;

CREATE FUNCTION guard_commission_policy_revision() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE p brand_commission_policies; a audit_logs;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'commission policy history immutable'; END IF;
 SELECT * INTO p FROM brand_commission_policies WHERE brand_id=NEW.brand_id;
 IF NOT FOUND OR (NEW.version=1 AND (p.version<>1 OR p.config<>NEW.config)) OR
  (NEW.version>1 AND p.version<>NEW.version-1) THEN RAISE EXCEPTION 'commission revision must belong to current/next version'; END IF;
 IF NEW.version>1 THEN
  SELECT * INTO a FROM audit_logs WHERE id=NEW.audit_log_id;
  IF NOT FOUND OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.actor_type<>'admin' OR a.actor_id IS DISTINCT FROM NEW.changed_by OR
   a.action<>'commission.policy.update' OR a.resource_type<>'commission_policy' OR a.resource_id IS DISTINCT FROM NEW.brand_id OR
   a.reason<>NEW.reason OR (a.after_json->>'version') IS DISTINCT FROM NEW.version::text OR
   a.after_json->'config' IS DISTINCT FROM NEW.config OR a.after_json->>'revision_id' IS DISTINCT FROM NEW.id::text THEN
   RAISE EXCEPTION 'commission policy revision needs matching audit';
  END IF;
 END IF;
 NEW.created_at:=clock_timestamp(); RETURN NEW;
END $$;

CREATE FUNCTION guard_commission_run() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE c commission_cycles; actual_epoch bigint;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'commission runs immutable'; END IF;
 SELECT * INTO c FROM commission_cycles WHERE brand_id=NEW.brand_id AND id=NEW.cycle_id;
 actual_epoch:=c.evidence_epoch;
 IF TG_OP='INSERT' THEN
  IF c.id IS NULL OR c.state<>'waiting' OR NOT c.scan_complete OR NEW.state<>'calculating' OR NEW.evidence_epoch<>actual_epoch OR
   NEW.generation<>(SELECT coalesce(max(generation),0)+1 FROM commission_runs WHERE cycle_id=NEW.cycle_id) OR
   NEW.cursor_at IS NOT NULL OR NEW.cursor_order_id IS NOT NULL OR NEW.earnings_cursor_agent IS NOT NULL THEN RAISE EXCEPTION 'invalid commission run'; END IF;
 ELSE
  IF (to_jsonb(NEW)-ARRAY['state','cursor_at','cursor_order_id','earnings_cursor_agent']) IS DISTINCT FROM
   (to_jsonb(OLD)-ARRAY['state','cursor_at','cursor_order_id','earnings_cursor_agent']) OR OLD.state='abandoned' OR
   (OLD.state='ready' AND NEW.state<>'abandoned') OR
   c.current_run_id IS DISTINCT FROM NEW.id OR NEW.state NOT IN(OLD.state,'summarizing','ready','abandoned') THEN RAISE EXCEPTION 'invalid commission run transition'; END IF;
  IF NEW.state='summarizing' AND OLD.state='calculating' AND
   (EXISTS(SELECT 1 FROM commission_cycle_targets t WHERE t.cycle_id=NEW.cycle_id AND NOT EXISTS(SELECT 1 FROM commission_calculations x WHERE x.run_id=NEW.id AND x.order_id=t.order_id)) OR
    EXISTS(SELECT 1 FROM commission_calculations x CROSS JOIN LATERAL jsonb_array_elements(x.rule_snapshot->'agent_path') node
     WHERE x.run_id=NEW.id AND x.reason='eligible' AND NOT EXISTS(SELECT 1 FROM commission_allocations a WHERE a.run_id=x.run_id AND a.order_id=x.order_id AND a.agent_id=(node->>'id')::uuid))) THEN RAISE EXCEPTION 'commission run calculations incomplete'; END IF;
  IF NEW.state='ready' AND (OLD.state<>'summarizing' OR NEW.evidence_epoch<>actual_epoch OR
   EXISTS(SELECT 1 FROM commission_allocations a WHERE a.run_id=NEW.id AND NOT EXISTS(SELECT 1 FROM commission_earnings e WHERE e.run_id=a.run_id AND e.agent_id=a.agent_id AND e.member_id=a.member_id))) THEN RAISE EXCEPTION 'commission run earnings incomplete'; END IF;
 END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_commission_step() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE c commission_cycles; a audit_logs;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'commission steps immutable'; END IF;
 SELECT * INTO c FROM commission_cycles WHERE brand_id=NEW.brand_id AND id=NEW.cycle_id;
 SELECT * INTO a FROM audit_logs WHERE id=NEW.audit_log_id;
 IF c.id IS NULL OR NEW.version<>c.version+1 OR NEW.from_state<>c.state OR a.id IS NULL OR a.brand_id IS DISTINCT FROM NEW.brand_id OR
  a.actor_type<>NEW.actor_type OR a.actor_id IS DISTINCT FROM NEW.actor_id OR a.resource_type<>'commission_cycle' OR a.resource_id IS DISTINCT FROM NEW.cycle_id OR
  a.action IS DISTINCT FROM 'commission.cycle.'||NEW.operation OR a.reason<>NEW.reason OR
  a.after_json->>'version' IS DISTINCT FROM NEW.version::text OR a.after_json->>'state' IS DISTINCT FROM NEW.to_state THEN RAISE EXCEPTION 'commission step needs matching audit'; END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_commission_target() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE c commission_cycles; o bet_orders;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'commission manifest immutable'; END IF;
 SELECT * INTO c FROM commission_cycles WHERE brand_id=NEW.brand_id AND id=NEW.cycle_id;
 SELECT * INTO o FROM bet_orders WHERE brand_id=NEW.brand_id AND id=NEW.order_id;
 IF c.id IS NULL OR c.state<>'enumerating' OR c.scan_complete OR o.id IS NULL OR NEW.placed_at<>o.placed_at OR
  o.placed_at<c.window_from OR o.placed_at>=c.window_to OR
  (c.manifest_cursor_at IS NOT NULL AND (NEW.placed_at,NEW.order_id)<=(c.manifest_cursor_at,c.manifest_cursor_id)) THEN RAISE EXCEPTION 'invalid commission manifest target'; END IF;
 NEW.creation_xid:=pg_current_xact_id(); RETURN NEW;
END $$;

CREATE FUNCTION guard_compliance_decision() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE p brand_compliance_policies;a audit_logs;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'compliance decision immutable'; END IF;
 SELECT * INTO p FROM brand_compliance_policies WHERE brand_id=NEW.brand_id;
 SELECT * INTO a FROM audit_logs WHERE id=NEW.audit_log_id;
 IF NOT FOUND OR p.brand_id IS NULL OR p.version IS DISTINCT FROM NEW.policy_version OR p.config IS DISTINCT FROM NEW.config
 OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.actor_type IS DISTINCT FROM 'admin' OR a.actor_id IS DISTINCT FROM NEW.created_by
 OR a.action IS DISTINCT FROM 'compliance.check' OR a.resource_type IS DISTINCT FROM 'compliance_decision' OR a.resource_id IS DISTINCT FROM NEW.id
 OR a.reason IS DISTINCT FROM NEW.reason OR a.after_json->>'id' IS DISTINCT FROM NEW.id::text OR a.after_json->>'brand_id' IS DISTINCT FROM NEW.brand_id::text
 OR (a.after_json->>'policy_version')::bigint IS DISTINCT FROM NEW.policy_version OR a.after_json->'config' IS DISTINCT FROM NEW.config
 OR a.after_json->>'operation' IS DISTINCT FROM NEW.operation OR a.after_json->>'decision' IS DISTINCT FROM NEW.decision
 OR a.after_json->'checks' IS DISTINCT FROM NEW.checks OR a.after_json->>'adapter_mode' IS DISTINCT FROM NEW.adapter_mode
 OR (a.after_json->>'created_at')::timestamptz IS DISTINCT FROM NEW.created_at THEN RAISE EXCEPTION 'matching compliance decision audit required'; END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_compliance_gate_rejection() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE p compliance_policy_revisions;a audit_logs;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'compliance admission rejection immutable'; END IF;
 SELECT * INTO p FROM compliance_policy_revisions WHERE brand_id=NEW.brand_id AND version=NEW.policy_version;
 SELECT * INTO a FROM audit_logs WHERE id=NEW.audit_log_id;
 IF NOT FOUND OR p.brand_id IS NULL OR p.config IS DISTINCT FROM NEW.config
 OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.actor_type IS DISTINCT FROM (CASE WHEN NEW.actor_type='anonymous' THEN 'system' ELSE NEW.actor_type END)
 OR a.actor_id IS DISTINCT FROM NEW.actor_id OR a.action IS DISTINCT FROM 'compliance.gate.reject'
 OR a.resource_type IS DISTINCT FROM 'compliance_gate' OR a.resource_id IS DISTINCT FROM NEW.id
 OR a.reason IS DISTINCT FROM 'COMPLIANCE_REVIEW_REQUIRED' OR a.request_id IS DISTINCT FROM NEW.request_id
 OR a.after_json->>'id' IS DISTINCT FROM NEW.id::text OR a.after_json->>'brand_id' IS DISTINCT FROM NEW.brand_id::text
 OR (a.after_json->>'policy_version')::bigint IS DISTINCT FROM NEW.policy_version OR a.after_json->'config' IS DISTINCT FROM NEW.config
 OR a.after_json->>'operation' IS DISTINCT FROM NEW.operation OR a.after_json->>'action' IS DISTINCT FROM NEW.action
 OR a.after_json->>'decision' IS DISTINCT FROM NEW.decision OR a.after_json->'checks' IS DISTINCT FROM NEW.checks
 OR a.after_json->>'adapter_mode' IS DISTINCT FROM NEW.adapter_mode OR a.after_json->>'actor_type' IS DISTINCT FROM NEW.actor_type
 OR a.after_json->'actor_id' IS DISTINCT FROM coalesce(to_jsonb(NEW.actor_id::text),'null'::jsonb)
 OR a.after_json->'member_id' IS DISTINCT FROM coalesce(to_jsonb(NEW.member_id::text),'null'::jsonb)
 OR (a.after_json->>'created_at')::timestamptz IS DISTINCT FROM NEW.created_at THEN RAISE EXCEPTION 'matching compliance gate audit required'; END IF;
 IF NEW.actor_type='admin' AND NOT EXISTS(SELECT 1 FROM admin_accounts WHERE id=NEW.actor_id)
 OR NEW.actor_type='user' AND NOT EXISTS(SELECT 1 FROM global_users WHERE id=NEW.actor_id)
 OR NEW.member_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM brand_members WHERE brand_id=NEW.brand_id AND id=NEW.member_id AND global_user_id=NEW.actor_id)
 THEN RAISE EXCEPTION 'compliance rejection actor/brand mismatch'; END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_compliance_policy() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'compliance policy cannot be deleted'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.version<>1 OR NEW.config<>default_compliance_config() THEN RAISE EXCEPTION 'disabled initial compliance policy required'; END IF;
 ELSE
  IF NEW.brand_id IS DISTINCT FROM OLD.brand_id OR NEW.version<>OLD.version+1 THEN RAISE EXCEPTION 'compliance identity/version invalid'; END IF;
 END IF;
 NEW.updated_at:=clock_timestamp();RETURN NEW;
END $$;

CREATE FUNCTION guard_compliance_revision() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE p brand_compliance_policies;a audit_logs;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'compliance revision immutable'; END IF;
 SELECT * INTO p FROM brand_compliance_policies WHERE brand_id=NEW.brand_id;
 IF NOT FOUND OR p.version IS DISTINCT FROM NEW.version OR p.config IS DISTINCT FROM NEW.config THEN RAISE EXCEPTION 'compliance revision requires current policy'; END IF;
 IF NEW.version=1 THEN
  IF NEW.config<>default_compliance_config() THEN RAISE EXCEPTION 'initial compliance history must be disabled'; END IF;
 ELSE
  SELECT * INTO a FROM audit_logs WHERE id=NEW.audit_log_id;
  IF NOT FOUND OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.actor_type IS DISTINCT FROM 'admin' OR a.actor_id IS DISTINCT FROM NEW.changed_by
  OR a.action IS DISTINCT FROM 'compliance.policy.update' OR a.resource_type IS DISTINCT FROM 'compliance_policy' OR a.resource_id IS DISTINCT FROM NEW.brand_id
  OR a.reason IS DISTINCT FROM NEW.reason OR a.after_json->>'brand_id' IS DISTINCT FROM NEW.brand_id::text
  OR (a.after_json->>'version')::bigint IS DISTINCT FROM NEW.version OR a.after_json->'config' IS DISTINCT FROM NEW.config
  OR (a.before_json->>'version')::bigint IS DISTINCT FROM NEW.version-1 THEN RAISE EXCEPTION 'matching compliance audit required'; END IF;
 END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_current_settlement_write() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE job uuid;
BEGIN
 IF TG_TABLE_NAME='settlement_calculations' THEN job:=NEW.job_id;
 ELSIF OLD.status='placed' AND NEW.status IN ('won','lost') THEN SELECT job_id INTO job FROM settlement_calculations WHERE id=NEW.settlement_calculation_id;
 ELSE RETURN NEW; END IF;
 IF NOT EXISTS(SELECT 1 FROM settlement_jobs j JOIN periods p ON p.id=j.period_id WHERE j.id=job AND p.current_settlement_job_id=j.id AND p.draw_result_id=j.draw_result_id AND NOT EXISTS(SELECT 1 FROM draw_corrections c WHERE c.previous_job_id=j.id AND c.state<>'completed')) THEN RAISE EXCEPTION 'superseded generation cannot calculate or pay'; END IF;
 RETURN NEW; END $$;

CREATE FUNCTION guard_draw_correction() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'correction history cannot be deleted'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.version<>1 OR NOT EXISTS(SELECT 1 FROM periods p JOIN draw_results d ON d.id=NEW.draw_result_id JOIN audit_logs a ON a.resource_id=NEW.id AND a.resource_type='draw_correction' AND a.action='draw.correct'
   WHERE p.id=NEW.period_id AND p.brand_id=NEW.brand_id AND p.draw_result_id=NEW.previous_draw_result_id AND p.version=NEW.period_version-1 AND d.corrected_from_id=NEW.previous_draw_result_id AND d.kind='manual' AND d.created_by=NEW.created_by AND a.actor_id=NEW.created_by AND a.brand_id=NEW.brand_id AND
   ((NEW.previous_job_id IS NULL AND p.current_settlement_job_id IS NULL AND p.status='drawn' AND NEW.state='completed') OR (NEW.previous_job_id=p.current_settlement_job_id AND p.status IN ('settling','settled') AND NEW.state='reversing' AND NEW.new_job_id IS NULL AND EXISTS(SELECT 1 FROM brand_settlement_policies b WHERE b.brand_id=p.brand_id AND b.version=NEW.policy_version AND b.mode=NEW.mode)))) THEN RAISE EXCEPTION 'invalid correction start or audit witness'; END IF;
 ELSE
  IF (to_jsonb(NEW)-ARRAY['state','version','new_job_id','completed_at','last_error_code']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['state','version','new_job_id','completed_at','last_error_code']) OR NEW.version<>OLD.version+1 OR OLD.state='completed' THEN RAISE EXCEPTION 'immutable correction identity'; END IF;
  IF OLD.state<>NEW.state AND NOT ((OLD.state='reversing' AND NEW.state IN ('failed','resettling')) OR (OLD.state='failed' AND NEW.state='reversing') OR (OLD.state='resettling' AND NEW.state='completed')) THEN RAISE EXCEPTION 'invalid correction transition'; END IF;
  IF NEW.new_job_id IS DISTINCT FROM OLD.new_job_id AND NOT (OLD.state='reversing' AND NEW.state='resettling' AND OLD.new_job_id IS NULL AND NEW.new_job_id IS NOT NULL) THEN RAISE EXCEPTION 'invalid correction child job'; END IF;
 END IF;
 RETURN NEW; END $$;

CREATE FUNCTION guard_draw_correction_target() RETURNS trigger
    LANGUAGE plpgsql
    AS $$ BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'correction target history cannot be deleted'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.version<>1 OR NEW.reversal_entry_id IS NOT NULL OR NEW.reset_order_version IS NOT NULL OR NEW.error_code IS NOT NULL OR NOT EXISTS(SELECT 1 FROM bet_orders o JOIN draw_corrections c ON c.id=NEW.correction_id WHERE c.state='reversing' AND c.version=1 AND o.id=NEW.order_id AND o.brand_id=NEW.brand_id AND o.period_id=NEW.period_id AND o.version=NEW.old_order_version AND o.status=NEW.old_order_status AND o.settlement_calculation_id IS NOT DISTINCT FROM NEW.old_calculation_id AND o.payout_entry_id IS NOT DISTINCT FROM NEW.old_payout_entry_id AND o.prize_points=NEW.old_prize_points AND ((o.status IN ('placed','won','lost') AND NEW.state='pending') OR (o.status IN ('abnormal','bet_cancelled','judged_cancelled') AND NEW.state='excluded'))) THEN RAISE EXCEPTION 'invalid correction target snapshot'; END IF;
 ELSE
  IF (to_jsonb(NEW)-ARRAY['state','version','reversal_entry_id','reset_order_version','error_code']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['state','version','reversal_entry_id','reset_order_version','error_code']) OR NEW.version<>OLD.version+1 OR OLD.state IN ('reversed','unchanged','excluded') THEN RAISE EXCEPTION 'immutable correction target'; END IF;
  IF NOT ((OLD.state='pending' AND NEW.state IN ('reversed','unchanged','excluded','failed')) OR (OLD.state='failed' AND NEW.state='pending')) THEN RAISE EXCEPTION 'invalid correction target transition'; END IF;
  IF NEW.state='reversed' AND (NEW.old_order_status NOT IN ('won','lost') OR NEW.reset_order_version<>NEW.old_order_version+1 OR (NEW.old_prize_points>0)<>(NEW.reversal_entry_id IS NOT NULL)) THEN RAISE EXCEPTION 'invalid reset version or reversal'; END IF;
  IF NEW.state='unchanged' AND NEW.old_order_status<>'placed' THEN RAISE EXCEPTION 'unsettled target only can be unchanged'; END IF;
 END IF;
 RETURN NEW; END $$;

CREATE FUNCTION guard_draw_notification_content() RETURNS trigger
    LANGUAGE plpgsql
    AS $_$
BEGIN
 IF NEW.event_type IN('draw.result.published','draw.result.corrected') AND NOT EXISTS(
  SELECT 1 FROM outbox_events e JOIN draw_notification_recipients r ON r.brand_id=e.brand_id AND r.id=e.aggregate_id
  JOIN draw_notification_publications p ON p.brand_id=r.brand_id AND p.draw_result_id=r.draw_result_id
  WHERE e.id=NEW.event_id AND e.brand_id=NEW.brand_id AND e.event_type=NEW.event_type AND r.member_id=NEW.member_id
   AND valid_draw_notification_event(e.brand_id,e.event_type,e.aggregate_id,e.payload)
   AND NEW.payload#>>'{draw,drawn_at}' ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}([.][0-9]{1,6})?Z$'
   AND (NEW.payload#>>'{draw,drawn_at}')::timestamptz=p.drawn_at
   AND (NEW.payload#-'{draw,drawn_at}')=jsonb_build_object('resource_id',p.draw_result_id::text,'points',NULL,'draw',jsonb_build_object(
    'game_id',p.game_id::text,'period_id',p.period_id::text,'period_no',p.period_no,'result',p.result,
    'previous_draw_id',p.previous_draw_id::text))
 ) THEN RAISE EXCEPTION 'draw inbox requires the exact historical public facts'; END IF;
 RETURN NEW;
END $_$;

CREATE FUNCTION guard_draw_notification_outbox() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF TG_OP='DELETE' THEN
  IF OLD.event_type IN('draw.result.published','draw.result.corrected') THEN RAISE EXCEPTION 'draw notification event immutable'; END IF;
  RETURN OLD;
 END IF;
 IF TG_OP='UPDATE' THEN
  IF OLD.event_type IN('draw.result.published','draw.result.corrected') OR NEW.event_type IN('draw.result.published','draw.result.corrected') THEN
   IF (to_jsonb(NEW)-'published_at') IS DISTINCT FROM (to_jsonb(OLD)-'published_at') THEN RAISE EXCEPTION 'draw notification event immutable except publication acknowledgement'; END IF;
   IF NOT valid_draw_notification_event(NEW.brand_id,NEW.event_type,NEW.aggregate_id,NEW.payload) THEN RAISE EXCEPTION 'draw notification evidence required'; END IF;
  END IF;
  RETURN NEW;
 END IF;
 IF NEW.event_type IN('draw.result.published','draw.result.corrected') AND
  (NOT valid_draw_notification_event(NEW.brand_id,NEW.event_type,NEW.aggregate_id,NEW.payload) OR NOT EXISTS(
   SELECT 1 FROM draw_notification_recipients r WHERE r.brand_id=NEW.brand_id AND r.id=NEW.aggregate_id AND r.creation_xid=pg_current_xact_id()
  )) THEN RAISE EXCEPTION 'new draw event requires the current immutable audience'; END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_draw_notification_publication() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE valid boolean;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'published draw notification evidence immutable'; END IF;
 IF NEW.creation_xid IS DISTINCT FROM pg_current_xact_id() THEN RAISE EXCEPTION 'current publication transaction required'; END IF;
 SELECT EXISTS(
  SELECT 1 FROM draw_results d JOIN periods p ON p.brand_id=d.brand_id AND p.game_id=d.game_id AND p.id=d.period_id
  JOIN audit_logs a ON a.brand_id=d.brand_id AND a.id=NEW.audit_log_id
  WHERE d.id=NEW.draw_result_id AND d.brand_id=NEW.brand_id AND d.game_id=NEW.game_id AND d.period_id=NEW.period_id
   AND p.draw_result_id=d.id AND draw_notice_current_row(p.xmin) AND draw_notice_current_row(a.xmin)
   AND NEW.period_no=p.period_no AND NEW.result=d.result AND NEW.drawn_at=d.drawn_at
   AND NEW.previous_draw_id IS NOT DISTINCT FROM d.corrected_from_id
   AND NEW.event_type=CASE WHEN d.corrected_from_id IS NULL THEN 'draw.result.published' ELSE 'draw.result.corrected' END
   AND (
    (a.action='draw.lock' AND a.resource_type='draw_result' AND a.resource_id=d.id
     AND a.after_json->>'id'=d.id::text AND a.after_json->>'period_id'=p.id::text)
    OR
    (a.resource_type='draw_correction' AND a.after_json->>'draw_result_id'=d.id::text
     AND EXISTS(SELECT 1 FROM draw_corrections c WHERE c.brand_id=d.brand_id AND c.id=a.resource_id
      AND c.period_id=p.id AND c.draw_result_id=d.id AND c.previous_draw_result_id=d.corrected_from_id
      AND ((a.action='draw.correct' AND c.state='completed') OR (a.action='draw.correction.publish' AND c.state='resettling'))))
   )
 ) INTO valid;
 IF valid IS DISTINCT FROM true THEN RAISE EXCEPTION 'matching current published result and audit required'; END IF;
 NEW.created_at:=clock_timestamp(); RETURN NEW;
END $$;

CREATE FUNCTION guard_draw_notification_recipient() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'published draw audience immutable'; END IF;
 IF NEW.creation_xid IS DISTINCT FROM pg_current_xact_id() OR NOT EXISTS(
  SELECT 1 FROM draw_notification_publications p JOIN bet_orders o ON o.brand_id=p.brand_id AND o.period_id=p.period_id
  WHERE p.brand_id=NEW.brand_id AND p.draw_result_id=NEW.draw_result_id AND p.creation_xid=pg_current_xact_id()
   AND o.brand_member_id=NEW.member_id
 ) THEN RAISE EXCEPTION 'recipient must belong to this current publication betting audience'; END IF;
 NEW.created_at:=clock_timestamp(); RETURN NEW;
END $$;

CREATE FUNCTION guard_join_code() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE p jsonb;source_user uuid;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'join codes cannot be deleted';END IF;
 PERFORM 1 FROM brand_agent_policies WHERE brand_id=NEW.brand_id FOR SHARE;
 IF TG_OP='INSERT' THEN
  IF NEW.version<>1 THEN RAISE EXCEPTION 'initial code version must be one';END IF;
 ELSE
  IF (to_jsonb(NEW)-ARRAY['version','status','starts_at','expires_at','updated_at']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['version','status','starts_at','expires_at','updated_at']) OR NEW.version<>OLD.version+1 THEN RAISE EXCEPTION 'join-code identity/version immutable';END IF;
 END IF;
 IF NEW.kind='agent' AND NOT EXISTS(SELECT 1 FROM agent_nodes WHERE brand_id=NEW.brand_id AND id=NEW.agent_id AND member_id=NEW.owner_member_id) THEN RAISE EXCEPTION 'agent code owner mismatch' USING ERRCODE='23514',CONSTRAINT='join_code_unavailable';END IF;
 -- Creating/enabling is not permitted for a disabled source. Expiration itself
 -- can be in the past; it simply prevents use, rather than rewriting history.
 IF NEW.status='active' THEN
  SELECT global_user_id INTO source_user FROM brand_members WHERE brand_id=NEW.brand_id AND id=NEW.owner_member_id;
  PERFORM 1 FROM global_users WHERE id=source_user FOR SHARE;
  PERFORM 1 FROM brand_members m JOIN global_users u ON u.id=m.global_user_id JOIN brands b ON b.id=m.brand_id WHERE m.brand_id=NEW.brand_id AND m.id=NEW.owner_member_id AND m.status='normal' AND u.status='active' AND b.status<>'disabled' FOR SHARE OF m;
  IF NOT FOUND THEN RAISE EXCEPTION 'join-code source unavailable' USING ERRCODE='23514',CONSTRAINT='join_code_unavailable';END IF;
  IF NEW.kind='agent' THEN
   SELECT config INTO p FROM brand_agent_policies WHERE brand_id=NEW.brand_id;
   IF p->'enabled'<>'true'::jsonb OR NOT EXISTS(SELECT 1 FROM agent_nodes n WHERE n.brand_id=NEW.brand_id AND n.id=NEW.agent_id AND n.member_id=NEW.owner_member_id AND NOT EXISTS(SELECT 1 FROM agent_nodes a WHERE a.brand_id=n.brand_id AND a.id=ANY(n.path) AND a.config->>'status'<>'active')) THEN RAISE EXCEPTION 'join-code source unavailable' USING ERRCODE='23514',CONSTRAINT='join_code_unavailable';END IF;
  END IF;
 END IF;
 NEW.updated_at:=clock_timestamp();RETURN NEW;
END $$;

CREATE FUNCTION guard_join_code_revision() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE c join_codes;a audit_logs;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'join-code history immutable';END IF;
 SELECT * INTO c FROM join_codes WHERE brand_id=NEW.brand_id AND id=NEW.code_id;
 IF NOT FOUND OR ROW(c.version,c.status,c.starts_at,c.expires_at) IS DISTINCT FROM ROW(NEW.version,NEW.status,NEW.starts_at,NEW.expires_at) THEN RAISE EXCEPTION 'join-code history does not match current version';END IF;
 SELECT * INTO a FROM audit_logs WHERE id=NEW.audit_log_id;
 IF NOT FOUND OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.actor_type<>'admin' OR a.actor_id IS DISTINCT FROM NEW.actor_id OR a.resource_type<>'join_code' OR a.resource_id IS DISTINCT FROM NEW.code_id OR a.reason<>NEW.reason
 OR a.action IS DISTINCT FROM (CASE WHEN NEW.version=1 THEN 'join_code.create' ELSE 'join_code.update' END)
 OR (NEW.version=1 AND NEW.actor_id IS DISTINCT FROM c.created_by)
 OR a.after_json->>'id' IS DISTINCT FROM c.id::text OR a.after_json->>'brand_id' IS DISTINCT FROM c.brand_id::text OR a.after_json->>'kind' IS DISTINCT FROM c.kind OR a.after_json->>'code' IS DISTINCT FROM c.code OR a.after_json->>'owner_member_id' IS DISTINCT FROM c.owner_member_id::text OR (a.after_json->>'agent_id')::uuid IS DISTINCT FROM c.agent_id
 OR (a.after_json->>'version')::bigint IS DISTINCT FROM NEW.version OR a.after_json->>'status' IS DISTINCT FROM NEW.status
 OR (a.after_json->>'starts_at')::timestamptz IS DISTINCT FROM NEW.starts_at OR (a.after_json->>'expires_at')::timestamptz IS DISTINCT FROM NEW.expires_at THEN RAISE EXCEPTION 'join-code audit witness missing';END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_notification_content() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'notification content is immutable'; END IF;
 IF (to_jsonb(NEW)-'read_at') IS DISTINCT FROM (to_jsonb(OLD)-'read_at') OR
    (OLD.read_at IS NOT NULL AND NEW.read_at IS DISTINCT FROM OLD.read_at) OR
    NEW.read_at IS NULL THEN RAISE EXCEPTION 'only the first read timestamp may be recorded'; END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_notification_template() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'notification template cannot be deleted'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.version<>1 OR NEW.content IS DISTINCT FROM notification_template_defaults()->NEW.template_key THEN RAISE EXCEPTION 'initial template must match default'; END IF;
 ELSIF NEW.brand_id IS DISTINCT FROM OLD.brand_id OR NEW.template_key IS DISTINCT FROM OLD.template_key OR NEW.version<>OLD.version+1 THEN
  RAISE EXCEPTION 'notification template identity/version invalid';
 END IF;
 NEW.updated_at:=clock_timestamp();RETURN NEW;
END $$;

CREATE FUNCTION guard_notification_template_revision() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE p notification_templates;a audit_logs;previous jsonb;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'notification template revision immutable'; END IF;
 SELECT * INTO p FROM notification_templates WHERE brand_id=NEW.brand_id AND template_key=NEW.template_key;
 IF NOT FOUND OR p.version IS DISTINCT FROM NEW.version OR p.content IS DISTINCT FROM NEW.content THEN RAISE EXCEPTION 'template revision requires current configuration'; END IF;
 IF NEW.version=1 THEN
  IF NEW.content IS DISTINCT FROM notification_template_defaults()->NEW.template_key THEN RAISE EXCEPTION 'initial history requires default'; END IF;
 ELSE
  SELECT content INTO previous FROM notification_template_revisions WHERE brand_id=NEW.brand_id AND template_key=NEW.template_key AND version=NEW.version-1;
  SELECT * INTO a FROM audit_logs WHERE id=NEW.audit_log_id;
  IF NOT FOUND OR previous IS NULL OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.actor_type IS DISTINCT FROM 'admin' OR a.actor_id IS DISTINCT FROM NEW.changed_by
  OR a.action IS DISTINCT FROM 'notification.template.update' OR a.resource_type IS DISTINCT FROM 'notification_template' OR a.resource_id IS DISTINCT FROM NEW.brand_id
  OR a.reason IS DISTINCT FROM NEW.reason OR a.after_json->>'brand_id' IS DISTINCT FROM NEW.brand_id::text OR a.after_json->>'key' IS DISTINCT FROM NEW.template_key
  OR (a.after_json->>'version')::bigint IS DISTINCT FROM NEW.version OR a.after_json->'content' IS DISTINCT FROM NEW.content
  OR a.before_json->>'brand_id' IS DISTINCT FROM NEW.brand_id::text OR a.before_json->>'key' IS DISTINCT FROM NEW.template_key
  OR (a.before_json->>'version')::bigint IS DISTINCT FROM NEW.version-1 OR a.before_json->'content' IS DISTINCT FROM previous THEN RAISE EXCEPTION 'matching template audit required'; END IF;
 END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_notification_template_snapshot() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE p notification_templates;
BEGIN
 SELECT * INTO p FROM notification_templates WHERE brand_id=NEW.brand_id AND template_key=NEW.template_key FOR SHARE;
 IF NOT FOUND OR NEW.content IS NULL OR NEW.template_version IS DISTINCT FROM p.version OR NEW.content IS DISTINCT FROM p.content THEN RAISE EXCEPTION 'current immutable template snapshot required'; END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_period_cancellation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'cancellation history cannot be deleted'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.version<>1 OR NEW.state NOT IN ('processing','completed') OR NOT EXISTS(SELECT 1 FROM periods WHERE id=NEW.period_id AND brand_id=NEW.brand_id AND version=NEW.period_version-1 AND status NOT IN ('settling','settled','bet_cancelled','judged_cancelled')) THEN RAISE EXCEPTION 'invalid cancellation start'; END IF;
 ELSE
  IF (to_jsonb(NEW)-ARRAY['state','version','completed_at','last_error_code']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['state','version','completed_at','last_error_code']) OR NEW.version<>OLD.version+1 THEN RAISE EXCEPTION 'immutable cancellation identity or invalid version'; END IF;
  IF NOT ((OLD.state='processing' AND NEW.state IN ('failed','completed')) OR (OLD.state='failed' AND NEW.state='processing')) THEN RAISE EXCEPTION 'invalid cancellation task transition'; END IF;
  IF NEW.state='completed' AND EXISTS(SELECT 1 FROM period_cancellation_targets WHERE cancellation_id=NEW.id AND state IN ('pending','failed')) THEN RAISE EXCEPTION 'unfinished cancellation cannot complete'; END IF;
 END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_period_history() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE old_kind text; new_kind text; predecessor uuid; c draw_corrections;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'period history cannot be deleted'; END IF;
 IF NEW.id<>OLD.id OR NEW.brand_id<>OLD.brand_id OR NEW.game_id<>OLD.game_id OR NEW.period_no<>OLD.period_no OR NEW.sequence<>OLD.sequence OR NEW.bet_start_at<>OLD.bet_start_at OR NEW.bet_end_at<>OLD.bet_end_at OR NEW.draw_at<>OLD.draw_at OR NEW.schedule_id IS DISTINCT FROM OLD.schedule_id OR NEW.created_at<>OLD.created_at THEN RAISE EXCEPTION 'immutable period identity'; END IF;
 IF NEW.version=OLD.version AND NEW.status=OLD.status AND NEW.state_reason=OLD.state_reason AND NEW.draw_result_id IS NOT DISTINCT FROM OLD.draw_result_id AND NEW.current_settlement_job_id IS NOT DISTINCT FROM OLD.current_settlement_job_id AND NEW.current_correction_id IS NOT DISTINCT FROM OLD.current_correction_id AND OLD.status='waiting_draw' AND OLD.draw_result_id IS NULL THEN RETURN NEW; END IF;
 IF NEW.version<>OLD.version+1 THEN RAISE EXCEPTION 'invalid period version'; END IF;
 IF NEW.current_correction_id IS DISTINCT FROM OLD.current_correction_id THEN
  SELECT * INTO c FROM draw_corrections WHERE id=NEW.current_correction_id;
  IF c.id IS NULL OR c.previous_draw_result_id IS DISTINCT FROM OLD.draw_result_id OR c.period_version<>NEW.version OR c.previous_job_id IS DISTINCT FROM OLD.current_settlement_job_id OR NEW.current_settlement_job_id IS DISTINCT FROM OLD.current_settlement_job_id THEN RAISE EXCEPTION 'invalid correction period admission'; END IF;
  IF NOT ((c.state='completed' AND c.previous_job_id IS NULL AND OLD.status='drawn' AND NEW.status='drawn' AND NEW.draw_result_id=c.draw_result_id) OR (c.state='reversing' AND OLD.status IN ('settled','settling') AND NEW.status='settling' AND NEW.draw_result_id=OLD.draw_result_id)) THEN RAISE EXCEPTION 'invalid correction period transition'; END IF;
  RETURN NEW;
 END IF;
 IF NEW.current_settlement_job_id IS DISTINCT FROM OLD.current_settlement_job_id THEN
  IF OLD.status='drawn' AND NEW.status='settling' AND OLD.current_settlement_job_id IS NULL AND NEW.draw_result_id=OLD.draw_result_id AND EXISTS(SELECT 1 FROM settlement_jobs j WHERE j.id=NEW.current_settlement_job_id AND j.period_id=OLD.id AND j.period_version=NEW.version AND j.generation=1) THEN RETURN NEW; END IF;
  SELECT * INTO c FROM draw_corrections WHERE id=OLD.current_correction_id;
  IF c.id IS NULL OR c.state<>'resettling' OR c.new_job_id IS DISTINCT FROM NEW.current_settlement_job_id OR c.previous_job_id IS DISTINCT FROM OLD.current_settlement_job_id OR c.previous_draw_result_id<>OLD.draw_result_id OR c.draw_result_id<>NEW.draw_result_id OR OLD.status<>'settling' OR NEW.status<>'settling' OR EXISTS(SELECT 1 FROM draw_correction_targets WHERE correction_id=c.id AND state IN ('pending','failed')) THEN RAISE EXCEPTION 'publication requires finished reversals and child generation'; END IF;
  RETURN NEW;
 END IF;
 IF NEW.draw_result_id IS DISTINCT FROM OLD.draw_result_id THEN
  SELECT kind,corrected_from_id INTO new_kind,predecessor FROM draw_results WHERE id=NEW.draw_result_id;
  IF OLD.status='waiting_draw' AND NEW.status='drawn' AND OLD.draw_result_id IS NULL AND NEW.draw_result_id IS NOT NULL AND predecessor IS NULL THEN NULL;
  ELSIF OLD.status='drawn' AND NEW.status='drawn' AND new_kind='manual' AND predecessor=OLD.draw_result_id THEN
   SELECT kind INTO old_kind FROM draw_results WHERE id=OLD.draw_result_id; IF old_kind NOT IN ('api','dom') THEN RAISE EXCEPTION 'manual result already locked'; END IF;
  ELSE RAISE EXCEPTION 'invalid result pointer transition'; END IF;
  IF NEW.draw_claim_token IS NOT NULL OR NEW.draw_claim_until IS NOT NULL THEN RAISE EXCEPTION 'locked result cannot retain claim'; END IF;
 ELSIF NEW.status='drawn' AND OLD.status='waiting_draw' THEN RAISE EXCEPTION 'draw requires result'; END IF;
 IF NEW.status IN ('bet_cancelled','judged_cancelled') THEN
  IF NOT (OLD.status='pending' AND NOT EXISTS(SELECT 1 FROM bet_orders WHERE period_id=OLD.id)) AND NOT EXISTS(SELECT 1 FROM period_cancellations WHERE period_id=OLD.id AND brand_id=OLD.brand_id AND period_version=NEW.version AND mode=NEW.status AND draw_result_id IS NOT DISTINCT FROM OLD.draw_result_id) THEN RAISE EXCEPTION 'cancellation requires durable refund task'; END IF;
  IF NEW.draw_claim_token IS NOT NULL OR NEW.draw_claim_until IS NOT NULL OR NEW.draw_next_poll_at IS NOT NULL THEN RAISE EXCEPTION 'cancelled period cannot retain draw lease'; END IF;
 END IF;
 IF NOT ((OLD.status='pending' AND NEW.status IN ('betting','judged_cancelled')) OR (OLD.status='betting' AND NEW.status IN ('closed','bet_cancelled','judged_cancelled')) OR (OLD.status='closed' AND NEW.status IN ('waiting_draw','judged_cancelled')) OR (OLD.status='waiting_draw' AND NEW.status IN ('drawn','judged_cancelled')) OR (OLD.status='drawn' AND NEW.status='drawn' AND NEW.draw_result_id IS DISTINCT FROM OLD.draw_result_id) OR (OLD.status='drawn' AND NEW.status IN ('settling','judged_cancelled')) OR (OLD.status='settling' AND NEW.status='settled')) THEN RAISE EXCEPTION 'invalid period transition'; END IF;
 RETURN NEW; END $$;

CREATE FUNCTION guard_point_ledger_snapshot() RETURNS trigger
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
  SELECT source,jsonb_object_agg(state,points::text) states FROM point_buckets
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
   jsonb_typeof(item->'points') IS DISTINCT FROM 'string' OR item->>'points' !~ '^[1-9][0-9]*$' OR
   (item->>'points')::numeric>9223372036854775807 THEN RAISE EXCEPTION 'invalid ledger source allocation'; END IF;
  seen:=array_append(seen,src);
 END LOOP;
 IF NEW.reversal_of IS NOT NULL THEN
  SELECT * INTO original FROM point_ledger_entries WHERE brand_id=NEW.brand_id AND account_id=NEW.account_id AND id=NEW.reversal_of AND member_id=NEW.member_id;
  expected:=validated_point_snapshot(original.delta_snapshot);
  IF original.id IS NULL OR original.reversal_of IS NOT NULL OR expected IS NULL OR NEW.source_allocation IS DISTINCT FROM original.source_allocation THEN RAISE EXCEPTION 'invalid original-source reversal'; END IF;
  FOREACH src IN ARRAY ARRAY['recharge','winning','gift','commission'] LOOP
   FOREACH st IN ARRAY ARRAY['available','manual_frozen','system_frozen','withdrawal'] LOOP
    expected:=jsonb_set(expected,ARRAY[src,st],to_jsonb((-(expected->src->>st)::numeric)::text));
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

CREATE FUNCTION guard_point_reconciliation_evidence() RETURNS trigger
    LANGUAGE plpgsql
    AS $_$
DECLARE t point_reconciliation_targets; j point_reconciliation_jobs; a audit_logs; p point_accounts; actual jsonb; expected jsonb; business jsonb; n bigint; wanted text;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'wallet reconciliation evidence is immutable'; END IF;
 SELECT * INTO j FROM point_reconciliation_jobs WHERE id=NEW.job_id AND brand_id=NEW.brand_id;
 SELECT * INTO a FROM audit_logs WHERE id=NEW.audit_log_id;
 IF j.id IS NULL OR a.id IS NULL OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.resource_type IS DISTINCT FROM 'wallet_reconciliation_job' AND TG_TABLE_NAME='point_reconciliation_retries' THEN
  RAISE EXCEPTION 'reconciliation evidence lacks scoped job and audit';
 END IF;
 IF TG_TABLE_NAME='point_reconciliation_retries' THEN
  IF j.state<>'failed' OR j.last_failure_id IS DISTINCT FROM NEW.previous_failure_id OR NEW.version<>j.version+1 OR a.actor_type IS DISTINCT FROM 'admin' OR a.actor_id IS DISTINCT FROM NEW.created_by OR a.action IS DISTINCT FROM 'wallet.reconciliation.retry' OR a.resource_id IS DISTINCT FROM NEW.job_id OR a.reason IS DISTINCT FROM NEW.reason OR a.after_json->>'version' IS DISTINCT FROM NEW.version::text THEN
   RAISE EXCEPTION 'reconciliation retry lacks matching failed version and audit';
  END IF;
  RETURN NEW;
 END IF;
 SELECT * INTO t FROM point_reconciliation_targets WHERE id=NEW.target_id AND brand_id=NEW.brand_id AND job_id=NEW.job_id;
 IF t.id IS NULL OR t.state<>'pending' OR j.state NOT IN('pending','running') OR a.actor_type IS DISTINCT FROM 'system' OR a.resource_type IS DISTINCT FROM 'wallet_reconciliation_target' OR a.resource_id IS DISTINCT FROM NEW.target_id OR a.after_json->>'job_id' IS DISTINCT FROM NEW.job_id::text THEN
  RAISE EXCEPTION 'reconciliation observation lacks pending target and audit';
 END IF;
 IF TG_TABLE_NAME='point_reconciliation_failures' THEN
  IF NEW.attempt_count<>t.attempt_count+1 OR a.action IS DISTINCT FROM 'wallet.reconciliation.failed' OR a.after_json->>'error_code' IS DISTINCT FROM NEW.error_code THEN RAISE EXCEPTION 'failure audit mismatch'; END IF;
  RETURN NEW;
 END IF;
 SELECT * INTO p FROM point_accounts WHERE id=t.account_id AND brand_id=t.brand_id;
 SELECT jsonb_object_agg(source,states) INTO actual FROM (
  SELECT source,jsonb_object_agg(state,points::text) states FROM point_buckets WHERE brand_id=t.brand_id AND account_id=t.account_id GROUP BY source
 ) v;
 SELECT count(*) INTO n FROM point_ledger_entries WHERE brand_id=t.brand_id AND account_id=t.account_id;
 SELECT after_snapshot INTO expected FROM point_ledger_entries WHERE brand_id=t.brand_id AND account_id=t.account_id ORDER BY version DESC LIMIT 1;
 IF expected IS NULL THEN expected:=point_zero_snapshot(); ELSE expected:=validated_point_snapshot(expected); END IF;
 IF j.check_scope='wallet_and_business' THEN
  business:=point_business_preview(t.brand_id,t.account_id);
  IF NEW.business_preview IS NULL OR business IS NULL OR NEW.business_preview IS DISTINCT FROM business OR
   a.after_json->'business_preview' IS DISTINCT FROM business THEN RAISE EXCEPTION 'full reconciliation requires exact current business witness'; END IF;
 ELSIF NEW.business_preview IS NOT NULL OR a.after_json ? 'business_preview' AND a.after_json->'business_preview'<>'null'::jsonb THEN
  RAISE EXCEPTION 'wallet-only observation cannot claim business coverage';
 END IF;
 wanted:=CASE WHEN business IS NOT NULL AND business->>'consistent'<>'true' THEN 'corrupt'
  WHEN NEW.preview->>'consistent'='true' THEN 'consistent' WHEN NEW.preview->>'repairable'='true' THEN 'repairable' ELSE 'corrupt' END;
 IF a.action IS DISTINCT FROM 'wallet.reconciliation.checked' OR a.after_json->>'outcome' IS DISTINCT FROM NEW.outcome OR a.after_json->'preview' IS DISTINCT FROM NEW.preview OR (a.after_json->>'checked_at')::timestamptz IS DISTINCT FROM NEW.checked_at OR
  NEW.preview->>'account_id' IS DISTINCT FROM t.account_id::text OR NEW.preview->>'member_id' IS DISTINCT FROM t.member_id::text OR NEW.preview->>'version' IS DISTINCT FROM p.version::text OR NEW.preview->>'ledger_version' IS DISTINCT FROM n::text OR NEW.preview->'actual' IS DISTINCT FROM COALESCE(actual,'{}'::jsonb) OR jsonb_typeof(NEW.preview->'issues') IS DISTINCT FROM 'array' OR jsonb_typeof(NEW.preview->'consistent') IS DISTINCT FROM 'boolean' OR jsonb_typeof(NEW.preview->'repairable') IS DISTINCT FROM 'boolean' OR COALESCE(NEW.preview->>'token','') !~ '^[0-9a-f]{64}$' OR NEW.outcome IS DISTINCT FROM wanted THEN
  RAISE EXCEPTION 'reconciliation result audit or observed wallet mismatch';
 END IF;
 -- Full business failures change the combined outcome, never weaken the
 -- independent wallet proof (including its expected balance and pass flags).
 IF (NEW.preview->>'consistent'='true' OR NEW.preview->>'repairable'='true') AND NEW.preview->'expected' IS DISTINCT FROM expected THEN RAISE EXCEPTION 'intact ledger expected balance mismatch'; END IF;
 IF NEW.preview->>'consistent'='true' AND (NEW.preview->>'repairable'<>'false' OR jsonb_array_length(NEW.preview->'issues')<>0 OR NEW.preview->'actual' IS DISTINCT FROM NEW.preview->'expected' OR p.version<>n) THEN RAISE EXCEPTION 'false consistent reconciliation'; END IF;
 RETURN NEW;
END $_$;

CREATE FUNCTION guard_point_reconciliation_job() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE a audit_logs;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'reconciliation jobs cannot be deleted'; END IF;
 IF TG_OP='INSERT' THEN
  SELECT * INTO a FROM audit_logs WHERE id=NEW.creation_audit_log_id;
  IF NEW.state<>'pending' OR NEW.version<>1 OR NEW.creation_xid IS DISTINCT FROM pg_current_xact_id() OR NEW.started_at IS NOT NULL OR NEW.completed_at IS NOT NULL OR NEW.last_failure_id IS NOT NULL OR a.id IS NULL OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.actor_type IS DISTINCT FROM 'admin' OR a.actor_id IS DISTINCT FROM NEW.created_by OR a.resource_type IS DISTINCT FROM 'wallet_reconciliation_job' OR a.resource_id IS DISTINCT FROM NEW.id OR a.action IS DISTINCT FROM 'wallet.reconciliation.create' OR a.reason IS DISTINCT FROM NEW.reason OR a.after_json->>'target_count' IS DISTINCT FROM NEW.target_count::text OR (a.after_json->>'created_at')::timestamptz IS DISTINCT FROM NEW.created_at THEN RAISE EXCEPTION 'reconciliation creation audit mismatch'; END IF;
 ELSE
  IF (NEW.id,NEW.brand_id,NEW.target_count,NEW.created_by,NEW.reason,NEW.created_at,NEW.creation_audit_log_id,NEW.creation_xid) IS DISTINCT FROM (OLD.id,OLD.brand_id,OLD.target_count,OLD.created_by,OLD.reason,OLD.created_at,OLD.creation_audit_log_id,OLD.creation_xid) OR NEW.version<>OLD.version+1 OR OLD.state='completed' OR OLD.started_at IS NOT NULL AND NEW.started_at IS DISTINCT FROM OLD.started_at THEN RAISE EXCEPTION 'invalid reconciliation job identity or version'; END IF;
  IF OLD.state='failed' THEN
   IF NEW.state<>'pending' OR NOT EXISTS(SELECT 1 FROM point_reconciliation_retries WHERE job_id=NEW.id AND version=NEW.version AND previous_failure_id=OLD.last_failure_id) THEN RAISE EXCEPTION 'failed job requires audited manual retry'; END IF;
  ELSIF NEW.state NOT IN('running','completed','failed') THEN RAISE EXCEPTION 'invalid reconciliation transition'; END IF;
 END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_point_reconciliation_scope() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE a audit_logs;
BEGIN
 IF TG_OP='UPDATE' AND NEW.check_scope IS DISTINCT FROM OLD.check_scope THEN
  RAISE EXCEPTION 'reconciliation scope is immutable';
 END IF;
 IF TG_OP='INSERT' THEN
  SELECT * INTO a FROM audit_logs WHERE id=NEW.creation_audit_log_id;
  IF coalesce(a.after_json->>'check_scope','wallet') IS DISTINCT FROM NEW.check_scope THEN
   RAISE EXCEPTION 'reconciliation scope requires explicit matching creation audit';
  END IF;
 END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_point_reconciliation_target() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE j point_reconciliation_jobs;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'reconciliation targets cannot be deleted'; END IF;
 SELECT * INTO j FROM point_reconciliation_jobs WHERE id=NEW.job_id AND brand_id=NEW.brand_id;
 IF TG_OP='INSERT' THEN
  IF j.version<>1 OR j.state<>'pending' OR NEW.state<>'pending' OR NEW.attempt_count<>0 OR j.creation_xid IS DISTINCT FROM pg_current_xact_id() THEN RAISE EXCEPTION 'target must be captured in the creation transaction'; END IF;
 ELSE
  IF (NEW.id,NEW.brand_id,NEW.job_id,NEW.account_id,NEW.member_id) IS DISTINCT FROM (OLD.id,OLD.brand_id,OLD.job_id,OLD.account_id,OLD.member_id) OR OLD.state='checked' THEN RAISE EXCEPTION 'reconciliation target identity and results are immutable'; END IF;
  IF NEW.state='checked' THEN
   IF OLD.state<>'pending' OR NEW.attempt_count<>OLD.attempt_count+1 OR NOT EXISTS(SELECT 1 FROM point_reconciliation_results WHERE target_id=NEW.id) THEN RAISE EXCEPTION 'checked target requires observation'; END IF;
  ELSIF NEW.state='failed' THEN
   IF OLD.state<>'pending' OR NEW.attempt_count<>OLD.attempt_count+1 OR NOT EXISTS(SELECT 1 FROM point_reconciliation_failures WHERE target_id=NEW.id AND attempt_count=NEW.attempt_count) THEN RAISE EXCEPTION 'failed target requires attempt'; END IF;
  ELSIF NEW.state='pending' THEN
   IF NEW.attempt_count<>OLD.attempt_count OR OLD.state='failed' AND (j.state<>'failed' OR NOT EXISTS(SELECT 1 FROM point_reconciliation_retries WHERE job_id=j.id AND version=j.version+1)) THEN RAISE EXCEPTION 'target retry requires job retry evidence'; END IF;
  END IF;
 END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_report_archive_automatic_task() RETURNS trigger
    LANGUAGE plpgsql
    AS $_$
DECLARE p report_archive_policy_revisions; a audit_logs; target report_archives; boundary timestamp; ending timestamp; expected jsonb;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'automatic archive task cannot be deleted'; END IF;
 SELECT * INTO p FROM report_archive_policy_revisions WHERE brand_id=NEW.brand_id AND version=NEW.policy_version;
 IF p.brand_id IS NULL OR NEW.timezone IS DISTINCT FROM p.timezone OR (NEW.kind='daily' AND (NOT p.daily_enabled OR NEW.period_key<p.daily_start_period)) OR (NEW.kind='monthly' AND (NOT p.monthly_enabled OR NEW.period_key<p.monthly_start_period)) THEN RAISE EXCEPTION 'automatic task requires saved enabled policy'; END IF;
 IF TG_OP='INSERT' THEN
 IF NEW.kind='daily' AND NEW.period_key ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}$' THEN boundary:=make_timestamp(substring(NEW.period_key,1,4)::integer,substring(NEW.period_key,6,2)::integer,substring(NEW.period_key,9,2)::integer,0,0,0);ending:=boundary+interval '1 day';
 ELSIF NEW.kind='monthly' AND NEW.period_key ~ '^[0-9]{4}-[0-9]{2}$' THEN boundary:=make_timestamp(substring(NEW.period_key,1,4)::integer,substring(NEW.period_key,6,2)::integer,1,0,0,0);ending:=boundary+interval '1 month'; ELSE RAISE EXCEPTION 'automatic task requires canonical period'; END IF;
 IF extract(year FROM boundary) NOT BETWEEN 1 AND 9998 OR extract(year FROM NEW.from_at AT TIME ZONE 'UTC')<1 OR extract(year FROM NEW.to_at AT TIME ZONE 'UTC')>9999 OR NEW.from_at IS DISTINCT FROM (CASE WHEN NEW.kind='monthly' THEN report_archive_period_end_boundary(boundary,NEW.timezone) ELSE report_archive_period_boundary(boundary,NEW.timezone) END) OR NEW.to_at IS DISTINCT FROM report_archive_period_end_boundary(ending,NEW.timezone) OR NEW.to_at>clock_timestamp() THEN RAISE EXCEPTION 'automatic task calendar is not elapsed'; END IF;
 END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.state<>'pending' OR NEW.version<>1 OR NEW.attempt_count<>0 OR NEW.archive_id IS NOT NULL OR NEW.last_error_code IS NOT NULL OR NEW.last_audit_log_id<>NEW.creation_audit_log_id OR NEW.created_at IS DISTINCT FROM statement_timestamp() OR NEW.updated_at IS DISTINCT FROM NEW.created_at THEN RAISE EXCEPTION 'automatic task begins pending'; END IF;
  SELECT * INTO a FROM audit_logs WHERE id=NEW.creation_audit_log_id;
  expected:=jsonb_build_object('id',NEW.id,'kind',NEW.kind,'period_key',NEW.period_key,'timezone',NEW.timezone,'policy_version',NEW.policy_version);
  IF a.id IS NULL OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.actor_type<>'system' OR a.actor_id IS NOT NULL OR a.action<>'report_archive.task.enqueue' OR a.resource_type<>'report_archive_task' OR a.resource_id IS DISTINCT FROM NEW.id OR a.before_json<>'null'::jsonb OR a.after_json IS DISTINCT FROM expected OR
   NOT EXISTS(SELECT 1 FROM brand_report_archive_policies c JOIN brands b ON b.id=c.brand_id WHERE c.brand_id=NEW.brand_id AND c.version=NEW.policy_version AND b.status<>'disabled' AND ((NEW.kind='daily' AND c.daily_enabled) OR (NEW.kind='monthly' AND c.monthly_enabled))) THEN RAISE EXCEPTION 'automatic task requires audited current discovery'; END IF;
  RETURN NEW;
 END IF;
 IF (to_jsonb(NEW)-ARRAY['state','version','attempt_count','archive_id','last_error_code','last_audit_log_id','updated_at']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['state','version','attempt_count','archive_id','last_error_code','last_audit_log_id','updated_at']) OR NEW.version<>OLD.version+1 OR NEW.updated_at IS DISTINCT FROM statement_timestamp() THEN RAISE EXCEPTION 'automatic task scope and history immutable'; END IF;
 SELECT * INTO a FROM audit_logs WHERE id=NEW.last_audit_log_id;
 IF a.id IS NULL OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.resource_type<>'report_archive_task' OR a.resource_id IS DISTINCT FROM NEW.id OR a.before_json IS DISTINCT FROM jsonb_build_object('state',OLD.state,'version',OLD.version,'attempt_count',OLD.attempt_count,'archive_id',OLD.archive_id,'last_error_code',OLD.last_error_code) OR a.after_json IS DISTINCT FROM jsonb_build_object('state',NEW.state,'version',NEW.version,'attempt_count',NEW.attempt_count,'archive_id',NEW.archive_id,'last_error_code',NEW.last_error_code) THEN RAISE EXCEPTION 'automatic task transition requires exact audit'; END IF;
 IF OLD.state='failed' AND NEW.state='pending' THEN
  IF a.action<>'report_archive.task.retry' OR a.actor_type<>'admin' OR a.actor_id IS NULL OR NEW.attempt_count<>OLD.attempt_count THEN RAISE EXCEPTION 'automatic task retry requires administrator'; END IF;
 ELSIF OLD.state='pending' AND NEW.state IN('completed','skipped','failed') THEN
  IF a.action<>'report_archive.task.finish' OR a.actor_type<>'system' OR a.actor_id IS NOT NULL OR NEW.attempt_count<>OLD.attempt_count+1 THEN RAISE EXCEPTION 'automatic task finish requires system attempt'; END IF;
 ELSE RAISE EXCEPTION 'invalid automatic task transition'; END IF;
 IF NEW.state IN('completed','skipped') THEN
  SELECT * INTO target FROM report_archives WHERE id=NEW.archive_id AND brand_id=NEW.brand_id AND kind=NEW.kind AND period_key=NEW.period_key;
  IF target.id IS NULL OR target.payload_sha256 IS DISTINCT FROM encode(sha256(convert_to(target.payload::text,'UTF8')),'hex') OR (NEW.state='completed' AND (target.automatic_task_id IS DISTINCT FROM NEW.id OR target.automatic_policy_version IS DISTINCT FROM NEW.policy_version)) OR (NEW.state='skipped' AND target.automatic_task_id IS NOT DISTINCT FROM NEW.id) THEN RAISE EXCEPTION 'automatic task outcome needs genuine retained archive'; END IF;
 END IF;
 RETURN NEW;
END $_$;

CREATE FUNCTION guard_report_archive_cursor() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE p report_archive_policy_revisions; key text; advanced text; d timestamp; ending timestamp; f timestamptz; t timestamptz; steps integer:=0;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'archive discovery cursor cannot be deleted'; END IF;
 SELECT * INTO p FROM report_archive_policy_revisions WHERE brand_id=NEW.brand_id AND version=NEW.policy_version;
 IF p.brand_id IS NULL THEN RAISE EXCEPTION 'archive cursor lacks policy snapshot'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.next_period_key IS DISTINCT FROM (CASE WHEN NEW.kind='daily' THEN p.daily_start_period ELSE p.monthly_start_period END) OR (NEW.kind='daily' AND NOT p.daily_enabled) OR (NEW.kind='monthly' AND NOT p.monthly_enabled) THEN RAISE EXCEPTION 'archive cursor starts at saved activation only'; END IF;
 ELSE
  IF NEW.brand_id<>OLD.brand_id OR NEW.policy_version<>OLD.policy_version OR NEW.kind<>OLD.kind OR OLD.next_period_key IS NULL OR NEW.next_period_key IS NOT DISTINCT FROM OLD.next_period_key THEN RAISE EXCEPTION 'invalid archive cursor change'; END IF;
  key:=OLD.next_period_key;
  WHILE key IS DISTINCT FROM NEW.next_period_key LOOP
   steps:=steps+1;IF steps>107 OR key IS NULL THEN RAISE EXCEPTION 'archive cursor advance exceeds bounded evidence'; END IF;
   IF NEW.kind='daily' THEN d:=make_timestamp(substring(key,1,4)::integer,substring(key,6,2)::integer,substring(key,9,2)::integer,0,0,0);ending:=d+interval '1 day';advanced:=to_char(ending,'YYYY-MM-DD');
   ELSE d:=make_timestamp(substring(key,1,4)::integer,substring(key,6,2)::integer,1,0,0,0);ending:=d+interval '1 month';advanced:=to_char(ending,'YYYY-MM'); END IF;
   f:=CASE WHEN NEW.kind='monthly' THEN report_archive_period_end_boundary(d,p.timezone) ELSE report_archive_period_boundary(d,p.timezone) END;t:=report_archive_period_end_boundary(ending,p.timezone);
   IF f IS NOT NULL AND t IS NOT NULL AND (t>clock_timestamp() OR NOT EXISTS(SELECT 1 FROM report_archive_automatic_tasks j WHERE j.brand_id=NEW.brand_id AND j.kind=NEW.kind AND j.period_key=key)) THEN RAISE EXCEPTION 'archive cursor cannot skip an undiscovered elapsed window'; END IF;
   IF extract(year FROM ending)>9998 THEN advanced:=NULL; END IF;
   key:=advanced;
  END LOOP;
 END IF;
 NEW.due_at:=report_archive_cursor_due(NEW.kind,NEW.next_period_key,p.timezone);
 RETURN NEW;
END $$;

CREATE FUNCTION guard_report_archive_policy() RETURNS trigger
    LANGUAGE plpgsql
    AS $_$
DECLARE a audit_logs; expected jsonb;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'archive policy cannot be deleted'; END IF;
 IF NEW.timezone='Local' OR NOT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=NEW.timezone) THEN RAISE EXCEPTION 'invalid archive policy timezone'; END IF;
 IF NEW.daily_start_period IS NOT NULL THEN
  IF NEW.daily_start_period !~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}$' OR substring(NEW.daily_start_period,1,4)::integer NOT BETWEEN 1 AND 9998 THEN RAISE EXCEPTION 'invalid daily activation'; END IF;
  PERFORM make_date(substring(NEW.daily_start_period,1,4)::integer,substring(NEW.daily_start_period,6,2)::integer,substring(NEW.daily_start_period,9,2)::integer);
 END IF;
 IF NEW.monthly_start_period IS NOT NULL THEN
  IF NEW.monthly_start_period !~ '^[0-9]{4}-[0-9]{2}$' OR substring(NEW.monthly_start_period,1,4)::integer NOT BETWEEN 1 AND 9998 THEN RAISE EXCEPTION 'invalid monthly activation'; END IF;
  PERFORM make_date(substring(NEW.monthly_start_period,1,4)::integer,substring(NEW.monthly_start_period,6,2)::integer,1);
 END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.version<>1 OR NEW.daily_enabled OR NEW.monthly_enabled OR NEW.daily_start_period IS NOT NULL OR NEW.monthly_start_period IS NOT NULL OR NEW.audit_log_id IS NOT NULL OR NOT EXISTS(SELECT 1 FROM brands WHERE id=NEW.brand_id AND timezone=NEW.timezone) THEN RAISE EXCEPTION 'archive policy initializes disabled only'; END IF;
  RETURN NEW;
 END IF;
 IF NEW.brand_id<>OLD.brand_id OR NEW.version<>OLD.version+1 OR NEW.updated_at IS DISTINCT FROM statement_timestamp() THEN RAISE EXCEPTION 'archive policy requires next version'; END IF;
 SELECT * INTO a FROM audit_logs WHERE id=NEW.audit_log_id;
 expected:=jsonb_build_object('brand_id',NEW.brand_id,'version',NEW.version,'daily_enabled',NEW.daily_enabled,'monthly_enabled',NEW.monthly_enabled,'daily_start_period',NEW.daily_start_period,'monthly_start_period',NEW.monthly_start_period,'timezone',NEW.timezone);
 IF a.id IS NULL OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.actor_type<>'admin' OR a.actor_id IS NULL OR a.action<>'report_archive.policy.update' OR a.resource_type<>'report_archive_policy' OR a.resource_id IS DISTINCT FROM NEW.brand_id OR a.reason IS NULL OR btrim(a.reason)='' OR
  (a.before_json-'updated_at') IS DISTINCT FROM (to_jsonb(OLD)-'updated_at') OR (a.before_json->>'updated_at')::timestamptz IS DISTINCT FROM OLD.updated_at OR a.after_json IS DISTINCT FROM expected THEN RAISE EXCEPTION 'archive policy requires exact audited intent'; END IF;
 IF NOT EXISTS(SELECT 1 FROM brands WHERE id=NEW.brand_id AND status<>'disabled' AND timezone=NEW.timezone) THEN RAISE EXCEPTION 'archive policy requires current enabled brand'; END IF;
 RETURN NEW;
END $_$;

CREATE FUNCTION guard_report_archive_policy_revision() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN IF TG_OP<>'INSERT' OR NOT EXISTS(SELECT 1 FROM brand_report_archive_policies p WHERE p.brand_id=NEW.brand_id AND to_jsonb(p)=to_jsonb(NEW)) THEN RAISE EXCEPTION 'archive policy revision is immutable'; END IF; RETURN NEW; END $$;

CREATE FUNCTION guard_report_archive_version() RETURNS trigger
    LANGUAGE plpgsql STABLE
    AS $_$
DECLARE prior report_archives; witness audit_logs; expected jsonb; original_zone text; boundary timestamp; next_boundary timestamp; first_at timestamptz; end_at timestamptz;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'report archives are immutable'; END IF;
 IF NEW.automatic_task_id IS NOT NULL THEN
  PERFORM pg_advisory_xact_lock(hashtextextended(TG_TABLE_SCHEMA||':report-archive:'||NEW.brand_id::text||':'||NEW.kind||':'||NEW.period_key,0));
  IF NOT report_archive_automatic_valid(NEW) THEN RAISE EXCEPTION 'automatic archive lacks task and snapshot evidence'; END IF;
  RETURN NEW;
 END IF;
 PERFORM pg_advisory_xact_lock(hashtextextended(TG_TABLE_SCHEMA||':report-archive:'||NEW.brand_id::text||':'||NEW.kind||':'||NEW.period_key,0));
 SELECT * INTO prior FROM report_archives WHERE brand_id=NEW.brand_id AND kind=NEW.kind AND period_key=NEW.period_key ORDER BY revision DESC LIMIT 1;
 IF prior.id IS NOT NULL THEN
  IF prior.payload_sha256 IS DISTINCT FROM encode(sha256(convert_to(prior.payload::text,'UTF8')),'hex') THEN RAISE EXCEPTION 'archive predecessor integrity cannot be proven'; END IF;
  IF NEW.previous_id IS DISTINCT FROM prior.id OR NEW.revision<>prior.revision+1 OR NEW.timezone IS DISTINCT FROM prior.timezone OR NEW.from_at IS DISTINCT FROM prior.from_at OR NEW.to_at IS DISTINCT FROM prior.to_at THEN
   RAISE EXCEPTION 'archive revision must extend the original immutable calendar scope';
  END IF;
 ELSE
  SELECT timezone INTO original_zone FROM brands WHERE id=NEW.brand_id;
  IF NEW.previous_id IS NOT NULL OR NEW.revision<>1 OR NEW.timezone IS DISTINCT FROM original_zone THEN RAISE EXCEPTION 'first archive requires current brand timezone'; END IF;
 END IF;
 -- Revisions reuse the original absolute interval, even if a later timezone
 -- rules database would resolve the same civil key differently.
 IF prior.id IS NULL THEN
 IF NEW.kind='daily' AND NEW.period_key ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}$' THEN
  boundary:=make_timestamp(substring(NEW.period_key,1,4)::integer,substring(NEW.period_key,6,2)::integer,substring(NEW.period_key,9,2)::integer,0,0,0);
  next_boundary:=boundary+interval '1 day';
 ELSIF NEW.kind='monthly' AND NEW.period_key ~ '^[0-9]{4}-[0-9]{2}$' THEN
  boundary:=make_timestamp(substring(NEW.period_key,1,4)::integer,substring(NEW.period_key,6,2)::integer,1,0,0,0);
  next_boundary:=boundary+interval '1 month';
 ELSE RAISE EXCEPTION 'archive requires canonical daily or monthly period'; END IF;
 IF extract(year FROM boundary) NOT BETWEEN 1 AND 9998 OR NEW.timezone='Local' OR NOT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=NEW.timezone) THEN RAISE EXCEPTION 'archive calendar is invalid'; END IF;
 first_at:=CASE WHEN NEW.kind='monthly' THEN report_archive_period_end_boundary(boundary,NEW.timezone) ELSE report_archive_period_boundary(boundary,NEW.timezone) END; end_at:=report_archive_period_end_boundary(next_boundary,NEW.timezone);
 IF first_at IS NULL OR end_at IS NULL OR NEW.from_at IS DISTINCT FROM first_at OR NEW.to_at IS DISTINCT FROM end_at OR extract(year FROM first_at AT TIME ZONE 'UTC')<1 OR extract(year FROM end_at AT TIME ZONE 'UTC')>9999 THEN RAISE EXCEPTION 'archive boundaries do not match the saved calendar'; END IF;
 END IF;
 IF NEW.snapshot_at IS DISTINCT FROM statement_timestamp() OR NEW.created_at IS DISTINCT FROM NEW.snapshot_at OR NEW.to_at>NEW.snapshot_at THEN RAISE EXCEPTION 'archive requires an elapsed period and the actual capture timestamp'; END IF;
 SELECT * INTO witness FROM audit_logs WHERE id=NEW.audit_log_id;
 IF witness.id IS NULL OR witness.brand_id IS DISTINCT FROM NEW.brand_id OR witness.actor_type IS DISTINCT FROM 'admin' OR witness.actor_id IS DISTINCT FROM NEW.created_by OR
  witness.action IS DISTINCT FROM 'report_archive.create' OR witness.resource_type IS DISTINCT FROM 'report_archive' OR witness.resource_id IS DISTINCT FROM NEW.id OR
  witness.reason IS DISTINCT FROM NEW.reason OR witness.request_id IS DISTINCT FROM NEW.request_id OR
  witness.before_json IS DISTINCT FROM (CASE WHEN prior.id IS NULL THEN 'null'::jsonb ELSE jsonb_build_object('id',prior.id,'revision',prior.revision,'payload_sha256',prior.payload_sha256) END) OR
  (witness.after_json-ARRAY['from','to']) IS DISTINCT FROM jsonb_build_object('id',NEW.id,'brand_id',NEW.brand_id,'kind',NEW.kind,'period_key',NEW.period_key,'timezone',NEW.timezone,'revision',NEW.revision,'previous_id',NEW.previous_id) OR
  (witness.after_json->>'from')::timestamptz IS DISTINCT FROM NEW.from_at OR (witness.after_json->>'to')::timestamptz IS DISTINCT FROM NEW.to_at THEN
  RAISE EXCEPTION 'archive lacks matching creation audit';
 END IF;
 expected:=report_archive_capture(NEW.brand_id,NEW.from_at,NEW.to_at,NEW.timezone);
 IF expected IS NULL OR NEW.payload IS DISTINCT FROM expected OR NEW.payload_sha256 IS DISTINCT FROM encode(sha256(convert_to(expected::text,'UTF8')),'hex') THEN
  RAISE EXCEPTION 'archive requires exact source snapshot and canonical SHA-256';
 END IF;
 RETURN NEW;
END $_$;

CREATE FUNCTION guard_reward_action() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE o reward_orders; log audit_logs; l point_ledger_entries;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'reward actions immutable'; END IF;
 IF TG_OP='UPDATE' THEN
  IF OLD.creation_xid<>pg_current_xact_id() OR OLD.ledger_entry_id IS NOT NULL OR NEW.ledger_entry_id IS NULL OR
   (to_jsonb(NEW)-'ledger_entry_id') IS DISTINCT FROM (to_jsonb(OLD)-'ledger_entry_id') THEN RAISE EXCEPTION 'reward action may only bind ledger in creation transaction'; END IF;
  SELECT * INTO l FROM point_ledger_entries WHERE id=NEW.ledger_entry_id;
  IF l.brand_id IS DISTINCT FROM NEW.brand_id OR l.reference_type IS DISTINCT FROM 'reward_order' OR l.reference_id IS DISTINCT FROM NEW.order_id OR
   l.actor_type IS DISTINCT FROM 'admin' OR l.actor_id IS DISTINCT FROM NEW.actor_id OR l.reason IS DISTINCT FROM NEW.reason OR
   l.entry_type IS DISTINCT FROM (CASE NEW.operation WHEN 'grant' THEN 'reward_grant' ELSE 'reward_reversal' END) THEN RAISE EXCEPTION 'reward action ledger mismatch'; END IF;
  RETURN NEW;
 END IF;
 SELECT * INTO o FROM reward_orders WHERE brand_id=NEW.brand_id AND id=NEW.order_id FOR UPDATE NOWAIT;
 PERFORM 1 FROM admin_accounts WHERE id=NEW.actor_id AND status='active' AND NOT is_super_admin FOR SHARE NOWAIT;
 IF NOT FOUND THEN RAISE EXCEPTION 'reward action requires active ordinary operator'; END IF;
 IF NEW.operation<>'grant' THEN
  PERFORM 1 FROM point_accounts WHERE brand_id=NEW.brand_id AND brand_member_id=o.member_id FOR UPDATE NOWAIT;
  IF NOT FOUND OR (NEW.state_after='revocation_pending' AND NOT EXISTS(
   SELECT 1 FROM point_buckets b JOIN point_accounts p ON p.brand_id=b.brand_id AND p.id=b.account_id
   WHERE p.brand_id=NEW.brand_id AND p.brand_member_id=o.member_id AND b.source='gift' AND b.state='available' AND b.points<o.points)) THEN
   RAISE EXCEPTION 'reward pending requires original gift available shortage'; END IF;
 END IF;
 SELECT * INTO log FROM audit_logs WHERE id=NEW.audit_log_id;
 IF o.id IS NULL OR NEW.ledger_entry_id IS NOT NULL OR
  (NEW.operation='grant' AND (o.creation_xid<>pg_current_xact_id() OR o.version<>1 OR o.state<>'granted' OR NEW.actor_id<>o.created_by OR NEW.audit_log_id<>o.creation_audit_log_id)) OR
  (NEW.operation<>'grant' AND (o.grant_ledger_entry_id IS NULL OR NEW.version<>o.version+1 OR NEW.state_before IS DISTINCT FROM o.state)) OR
  log.brand_id IS DISTINCT FROM NEW.brand_id OR log.actor_type IS DISTINCT FROM 'admin' OR log.actor_id IS DISTINCT FROM NEW.actor_id OR
  log.action IS DISTINCT FROM 'reward.order.'||NEW.operation OR log.resource_type IS DISTINCT FROM 'reward_order' OR log.resource_id IS DISTINCT FROM NEW.order_id OR
  log.reason IS DISTINCT FROM NEW.reason OR log.after_json->>'action_id' IS DISTINCT FROM NEW.id::text OR
  log.after_json->>'version' IS DISTINCT FROM NEW.version::text OR log.after_json->>'state' IS DISTINCT FROM NEW.state_after OR
  (NEW.operation<>'grant' AND (log.before_json->>'version' IS DISTINCT FROM o.version::text OR log.before_json->>'state' IS DISTINCT FROM o.state)) THEN
  RAISE EXCEPTION 'reward action requires matching explicit operator audit and version'; END IF;
 NEW.creation_xid:=pg_current_xact_id();RETURN NEW;
END $$;

CREATE FUNCTION guard_reward_credit() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE o reward_orders; a reward_order_actions; expected jsonb; policy brand_point_policies; total_after numeric;
BEGIN
 IF NEW.entry_type NOT IN('reward_grant','reward_reversal') AND NEW.reference_type<>'reward_order' AND
  NOT EXISTS(SELECT 1 FROM point_ledger_entries WHERE id=NEW.reversal_of AND entry_type='reward_grant') THEN RETURN NEW; END IF;
 SELECT * INTO o FROM reward_orders WHERE brand_id=NEW.brand_id AND id=NEW.reference_id FOR UPDATE NOWAIT;
 SELECT * INTO a FROM reward_order_actions WHERE brand_id=NEW.brand_id AND order_id=o.id AND
  version=CASE NEW.entry_type WHEN 'reward_grant' THEN 1 ELSE o.version+1 END;
 PERFORM 1 FROM brands WHERE id=NEW.brand_id AND status<>'disabled' FOR SHARE NOWAIT;
 IF NOT FOUND OR o.id IS NULL OR a.id IS NULL OR a.creation_xid<>pg_current_xact_id() OR a.ledger_entry_id IS NOT NULL OR
  NEW.reference_type<>'reward_order' OR NEW.member_id IS DISTINCT FROM o.member_id OR NEW.actor_type IS DISTINCT FROM 'admin' OR
  NEW.actor_id IS DISTINCT FROM a.actor_id OR NEW.reason IS DISTINCT FROM a.reason THEN RAISE EXCEPTION 'reward posting requires current explicit business action'; END IF;
 IF NEW.entry_type='reward_grant' THEN
  IF o.state<>'granted' OR o.version<>1 OR o.creation_xid<>pg_current_xact_id() OR o.grant_ledger_entry_id IS NOT NULL OR a.operation<>'grant' OR
   NEW.operation_key IS DISTINCT FROM 'reward-grant:'||o.id::text OR NEW.reversal_of IS NOT NULL THEN RAISE EXCEPTION 'reward grant cannot repeat or change identity'; END IF;
  SELECT * INTO policy FROM brand_point_policies WHERE brand_id=NEW.brand_id FOR SHARE NOWAIT;
  SELECT sum(bucket.value::numeric) INTO total_after FROM jsonb_each(NEW.after_snapshot) src CROSS JOIN LATERAL jsonb_each_text(src.value) bucket;
  IF policy.version IS DISTINCT FROM o.point_policy_version OR policy.max_balance_points IS NOT NULL AND total_after>policy.max_balance_points THEN
   RAISE EXCEPTION 'reward grant current point policy mismatch'; END IF;
  expected:=jsonb_set(point_zero_snapshot(),'{gift,available}',to_jsonb(o.points::text));
 ELSIF NEW.entry_type='reward_reversal' THEN
  IF o.state NOT IN('granted','revocation_pending') OR o.grant_ledger_entry_id IS NULL OR o.revoke_ledger_entry_id IS NOT NULL OR
   a.operation NOT IN('revoke','retry') OR a.state_after<>'revoked' OR NEW.reversal_of IS DISTINCT FROM o.grant_ledger_entry_id OR
   NEW.operation_key IS DISTINCT FROM 'reward-revoke:'||o.id::text THEN RAISE EXCEPTION 'reward revocation must reverse original grant exactly once'; END IF;
  expected:=jsonb_set(point_zero_snapshot(),'{gift,available}',to_jsonb((-o.points)::text));
 ELSE RAISE EXCEPTION 'generic reversal cannot bypass reward revocation'; END IF;
 IF NEW.delta_snapshot IS DISTINCT FROM expected OR NEW.source_allocation IS DISTINCT FROM
  jsonb_build_array(jsonb_build_object('source','gift','state','available','points',o.points::text)) THEN RAISE EXCEPTION 'reward moves only exact gift available amount'; END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_reward_notification_outbox() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF TG_OP='DELETE' THEN
  IF OLD.event_type IN('reward.order.granted','reward.order.revocation_pending','reward.order.revoked') THEN RAISE EXCEPTION 'reward notification event immutable'; END IF;
  RETURN OLD;
 END IF;
 IF TG_OP='UPDATE' THEN
  IF OLD.event_type IN('reward.order.granted','reward.order.revocation_pending','reward.order.revoked') OR NEW.event_type IN('reward.order.granted','reward.order.revocation_pending','reward.order.revoked') THEN
   IF (to_jsonb(NEW)-'published_at') IS DISTINCT FROM (to_jsonb(OLD)-'published_at') THEN RAISE EXCEPTION 'reward notification event immutable except published_at'; END IF;
   IF NOT valid_reward_notification_event(NEW.brand_id,NEW.event_type,NEW.aggregate_id,NEW.payload) THEN RAISE EXCEPTION 'reward notification requires real immutable business evidence'; END IF;
  END IF;
  RETURN NEW;
 END IF;
 IF NEW.event_type IN('reward.order.granted','reward.order.revocation_pending','reward.order.revoked') AND
  (NOT valid_reward_notification_event(NEW.brand_id,NEW.event_type,NEW.aggregate_id,NEW.payload) OR NOT EXISTS(
   SELECT 1 FROM reward_order_actions WHERE brand_id=NEW.brand_id AND id=NEW.aggregate_id AND creation_xid=pg_current_xact_id())) THEN
  RAISE EXCEPTION 'reward notification requires current business action and immutable evidence';
 END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_reward_order() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE log audit_logs; a reward_order_actions;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'reward order history immutable'; END IF;
 PERFORM 1 FROM brands WHERE id=NEW.brand_id AND status<>'disabled' FOR SHARE NOWAIT;
 IF NOT FOUND THEN RAISE EXCEPTION 'reward brand does not admit writes'; END IF;
 IF TG_OP='INSERT' THEN
  PERFORM 1 FROM admin_accounts WHERE id=NEW.created_by AND status='active' AND NOT is_super_admin FOR SHARE NOWAIT;
  IF NOT FOUND THEN RAISE EXCEPTION 'reward grant requires active ordinary operator'; END IF;
  SELECT * INTO log FROM audit_logs WHERE id=NEW.creation_audit_log_id;
  IF NEW.version<>1 OR NEW.state<>'granted' OR NEW.grant_ledger_entry_id IS NOT NULL OR NEW.revoke_ledger_entry_id IS NOT NULL OR
   NEW.last_audit_log_id<>NEW.creation_audit_log_id OR log.brand_id IS DISTINCT FROM NEW.brand_id OR log.actor_type IS DISTINCT FROM 'admin' OR
   log.actor_id IS DISTINCT FROM NEW.created_by OR log.action IS DISTINCT FROM 'reward.order.grant' OR log.resource_type IS DISTINCT FROM 'reward_order' OR
   log.resource_id IS DISTINCT FROM NEW.id OR log.reason IS DISTINCT FROM NEW.reason OR log.after_json->>'version' IS DISTINCT FROM '1' OR
   log.after_json->>'state' IS DISTINCT FROM 'granted' OR log.after_json->>'member_id' IS DISTINCT FROM NEW.member_id::text OR
   log.after_json->>'points' IS DISTINCT FROM NEW.points::text THEN RAISE EXCEPTION 'reward grant requires matching manual approval audit'; END IF;
  NEW.creation_xid:=pg_current_xact_id();
 ELSE
  IF OLD.grant_ledger_entry_id IS NULL AND NEW.grant_ledger_entry_id IS NOT NULL AND OLD.creation_xid=pg_current_xact_id() AND
   (to_jsonb(NEW)-'grant_ledger_entry_id') IS NOT DISTINCT FROM (to_jsonb(OLD)-'grant_ledger_entry_id') THEN RETURN NEW; END IF;
  IF (to_jsonb(NEW)-ARRAY['state','version','revoke_ledger_entry_id','last_audit_log_id','last_error_code','revoked_at','updated_at']) IS DISTINCT FROM
   (to_jsonb(OLD)-ARRAY['state','version','revoke_ledger_entry_id','last_audit_log_id','last_error_code','revoked_at','updated_at']) OR
   OLD.grant_ledger_entry_id IS NULL OR NEW.version<>OLD.version+1 OR OLD.state='revoked' OR NEW.state NOT IN('revocation_pending','revoked') THEN
   RAISE EXCEPTION 'reward state, version or immutable identity conflict'; END IF;
  SELECT * INTO a FROM reward_order_actions WHERE brand_id=NEW.brand_id AND order_id=NEW.id AND version=NEW.version;
  IF a.id IS NULL OR a.state_before IS DISTINCT FROM OLD.state OR a.state_after IS DISTINCT FROM NEW.state OR
   a.audit_log_id IS DISTINCT FROM NEW.last_audit_log_id OR (NEW.state='revoked' AND a.ledger_entry_id IS DISTINCT FROM NEW.revoke_ledger_entry_id) OR
   (NEW.state='revocation_pending' AND a.ledger_entry_id IS NOT NULL) THEN RAISE EXCEPTION 'reward state requires explicit immutable action'; END IF;
  NEW.updated_at:=clock_timestamp();
  NEW.revoked_at:=CASE WHEN NEW.state='revoked' THEN NEW.updated_at ELSE NULL END;
 END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_rule_version_history() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'rule history cannot be deleted'; END IF;
 IF NEW.id<>OLD.id OR NEW.brand_id<>OLD.brand_id OR NEW.game_id<>OLD.game_id OR NEW.play_id<>OLD.play_id OR NEW.version_no<>OLD.version_no OR NEW.created_by<>OLD.created_by OR NEW.created_at<>OLD.created_at OR NEW.source_version_id IS DISTINCT FROM OLD.source_version_id OR NEW.version<>OLD.version+1 THEN RAISE EXCEPTION 'immutable rule identity or invalid version'; END IF;
 IF OLD.status<>'draft' AND (NEW.definition IS DISTINCT FROM OLD.definition OR NEW.definition_sha256<>OLD.definition_sha256 OR NEW.effect_mode<>OLD.effect_mode OR NEW.validation IS DISTINCT FROM OLD.validation) THEN RAISE EXCEPTION 'reviewed definition is immutable'; END IF;
 IF OLD.source_version_id IS NOT NULL AND (NEW.definition IS DISTINCT FROM OLD.definition OR NEW.definition_sha256<>OLD.definition_sha256) THEN RAISE EXCEPTION 'rollback source definition is immutable'; END IF;
 IF NOT ((OLD.status='draft' AND NEW.status IN ('draft','pending_review')) OR (OLD.status='pending_review' AND NEW.status IN ('approved','active','rejected')) OR (OLD.status='approved' AND NEW.status='active') OR (OLD.status='active' AND NEW.status IN ('expired','rolled_back'))) THEN RAISE EXCEPTION 'invalid rule transition'; END IF;
 IF NEW.reviewed_by IS NOT NULL AND EXISTS(SELECT 1 FROM rule_version_contributors WHERE rule_version_id=OLD.id AND admin_id=NEW.reviewed_by) THEN RAISE EXCEPTION 'rule contributor cannot review'; END IF;
 IF OLD.reviewed_by IS NOT NULL AND (NEW.reviewed_by IS DISTINCT FROM OLD.reviewed_by OR NEW.reviewed_at IS DISTINCT FROM OLD.reviewed_at OR NEW.review_comment<>OLD.review_comment) THEN RAISE EXCEPTION 'immutable review evidence'; END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_settlement_bet_update() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE l point_ledger_entries; c settlement_calculations; j settlement_jobs;
BEGIN
 IF (to_jsonb(NEW)-ARRAY['status','version','refund_entry_id','cancelled_at','cancel_reason','settlement_calculation_id','payout_entry_id','prize_points','settled_at']) IS DISTINCT FROM
 (to_jsonb(OLD)-ARRAY['status','version','refund_entry_id','cancelled_at','cancel_reason','settlement_calculation_id','payout_entry_id','prize_points','settled_at']) OR NEW.version<>OLD.version+1 THEN RAISE EXCEPTION 'immutable bet identity or invalid version'; END IF;
 IF OLD.status IN ('won','lost') AND NEW.status='placed' THEN
  IF NEW.refund_entry_id IS DISTINCT FROM OLD.refund_entry_id OR NEW.cancelled_at IS DISTINCT FROM OLD.cancelled_at OR NEW.cancel_reason IS DISTINCT FROM OLD.cancel_reason OR NEW.settlement_calculation_id IS NOT NULL OR NEW.payout_entry_id IS NOT NULL OR NEW.prize_points<>0 OR NEW.settled_at IS NOT NULL OR NOT EXISTS(
   SELECT 1 FROM draw_correction_targets t JOIN draw_corrections dc ON dc.id=t.correction_id JOIN periods p ON p.id=dc.period_id
   WHERE t.brand_id=OLD.brand_id AND t.order_id=OLD.id AND t.old_order_version=OLD.version AND t.old_order_status=OLD.status AND t.old_calculation_id=OLD.settlement_calculation_id AND t.old_payout_entry_id IS NOT DISTINCT FROM OLD.payout_entry_id AND t.old_prize_points=OLD.prize_points AND t.state='reversed' AND t.reset_order_version=NEW.version AND dc.state='reversing' AND p.current_correction_id=dc.id AND p.draw_result_id=dc.previous_draw_result_id AND correction_reversal_is_applied(t) IS TRUE AND
   (t.old_prize_points=0 OR EXISTS(SELECT 1 FROM point_ledger_entries le JOIN point_accounts pa ON pa.id=le.account_id WHERE le.id=t.reversal_entry_id AND pa.version=le.version AND (SELECT count(*) FROM point_buckets pb WHERE pb.brand_id=le.brand_id AND pb.account_id=le.account_id AND pb.points::numeric=(validated_point_snapshot(le.after_snapshot)->pb.source->>pb.state)::numeric)=16)))
   THEN RAISE EXCEPTION 'order reset requires applied full reversal'; END IF;
  RETURN NEW;
 END IF;
 IF OLD.status='placed' AND NEW.status IN ('won','lost') THEN
  SELECT * INTO c FROM settlement_calculations WHERE brand_id=NEW.brand_id AND order_id=NEW.id AND id=NEW.settlement_calculation_id;
  SELECT * INTO j FROM settlement_jobs WHERE id=c.job_id;
  IF c.id IS NULL OR c.order_version<>OLD.version OR c.definition_hash<>OLD.definition_hash OR NEW.prize_points<>c.prize_points OR NEW.status<>(CASE WHEN c.won THEN 'won' ELSE 'lost' END) OR j.state<>'paying' OR NOT EXISTS(SELECT 1 FROM periods WHERE id=j.period_id AND status='settling' AND draw_result_id=j.draw_result_id) OR NEW.refund_entry_id IS NOT NULL OR NEW.cancelled_at IS NOT NULL OR NEW.cancel_reason<>'' THEN RAISE EXCEPTION 'invalid payout calculation evidence'; END IF;
  IF c.prize_points>0 THEN
   SELECT * INTO l FROM point_ledger_entries WHERE id=NEW.payout_entry_id;
   IF l.id IS NULL OR l.brand_id<>NEW.brand_id OR l.member_id<>NEW.brand_member_id OR l.entry_type<>'prize' OR l.reference_type<>'settlement_calculation' OR l.reference_id IS DISTINCT FROM c.id OR l.actor_type<>'system' OR l.reversal_of IS NOT NULL OR l.source_allocation<>jsonb_build_array(jsonb_build_object('source','winning','state','available','points',c.prize_points::text)) OR
   (l.delta_snapshot->'winning'->>'available')::numeric<>c.prize_points OR EXISTS(SELECT 1 FROM jsonb_each(l.delta_snapshot) s WHERE (s.value->>'manual_frozen')::numeric<>0 OR (s.value->>'system_frozen')::numeric<>0 OR (s.value->>'withdrawal')::numeric<>0 OR (s.key<>'winning' AND (s.value->>'available')::numeric<>0)) THEN RAISE EXCEPTION 'invalid winning ledger evidence'; END IF;
  END IF;
  RETURN NEW;
 END IF;
 IF NEW.settlement_calculation_id IS DISTINCT FROM OLD.settlement_calculation_id OR NEW.payout_entry_id IS DISTINCT FROM OLD.payout_entry_id OR NEW.prize_points<>OLD.prize_points OR NEW.settled_at IS DISTINCT FROM OLD.settled_at THEN RAISE EXCEPTION 'settlement fields are immutable outside payout'; END IF;
 IF OLD.status='placed' AND NEW.status='abnormal' THEN
  IF NEW.refund_entry_id IS DISTINCT FROM OLD.refund_entry_id OR NEW.cancelled_at IS DISTINCT FROM OLD.cancelled_at OR NEW.cancel_reason IS DISTINCT FROM OLD.cancel_reason OR NOT EXISTS(SELECT 1 FROM bet_order_exceptions WHERE brand_id=NEW.brand_id AND order_id=NEW.id AND order_version=NEW.version) THEN RAISE EXCEPTION 'invalid exception classification'; END IF;
  RETURN NEW;
 END IF;
 IF NOT (OLD.status IN ('placed','abnormal') AND NEW.status IN ('bet_cancelled','judged_cancelled')) THEN RAISE EXCEPTION 'invalid bet transition'; END IF;
 IF NEW.status='judged_cancelled' AND NOT EXISTS(SELECT 1 FROM bet_order_judgments WHERE brand_id=NEW.brand_id AND order_id=NEW.id AND order_version=NEW.version AND reason=NEW.cancel_reason) AND NOT EXISTS(SELECT 1 FROM period_cancellation_targets t JOIN period_cancellations pc ON pc.id=t.cancellation_id JOIN periods phase ON phase.id=pc.period_id WHERE t.brand_id=NEW.brand_id AND t.order_id=NEW.id AND t.state='pending' AND pc.mode='judged_cancelled' AND pc.state='processing' AND phase.status=pc.mode AND phase.version=pc.period_version) THEN RAISE EXCEPTION 'judged refund needs individual or period judgment witness'; END IF;
 SELECT * INTO l FROM point_ledger_entries WHERE id=NEW.refund_entry_id;
 IF l.entry_type<>'refund' OR l.reference_type<>'bet_order' OR l.reference_id IS DISTINCT FROM OLD.id OR l.reversal_of IS DISTINCT FROM OLD.debit_entry_id OR l.source_allocation<>OLD.deduction_allocation OR NEW.cancel_reason='' THEN RAISE EXCEPTION 'invalid bet refund evidence'; END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_settlement_job() RETURNS trigger
    LANGUAGE plpgsql
    AS $$ BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'settlement jobs cannot be deleted'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.version<>1 OR NEW.state<>'processing' OR NEW.approved_by IS NOT NULL THEN RAISE EXCEPTION 'invalid settlement start'; END IF;
  IF NEW.generation=1 THEN
   IF NOT EXISTS(SELECT 1 FROM periods p JOIN brand_settlement_policies b ON b.brand_id=p.brand_id WHERE p.brand_id=NEW.brand_id AND p.id=NEW.period_id AND p.current_settlement_job_id IS NULL AND p.status='drawn' AND p.version=NEW.period_version-1 AND p.draw_result_id=NEW.draw_result_id AND b.version=NEW.policy_version AND b.mode=NEW.mode) THEN RAISE EXCEPTION 'invalid initial generation'; END IF;
  ELSE
   IF NOT EXISTS(SELECT 1 FROM draw_corrections c JOIN settlement_jobs previous ON previous.id=c.previous_job_id JOIN periods p ON p.id=c.period_id WHERE c.id=NEW.correction_id AND c.brand_id=NEW.brand_id AND c.period_id=NEW.period_id AND c.state='reversing' AND c.new_job_id IS NULL AND c.draw_result_id=NEW.draw_result_id AND c.previous_job_id=NEW.previous_job_id AND previous.generation+1=NEW.generation AND c.policy_version=NEW.policy_version AND c.mode=NEW.mode AND c.created_by=NEW.created_by AND p.current_correction_id=c.id AND p.current_settlement_job_id=c.previous_job_id AND p.version=NEW.period_version-1 AND NOT EXISTS(SELECT 1 FROM draw_correction_targets t WHERE t.correction_id=c.id AND t.state IN ('pending','failed'))) THEN RAISE EXCEPTION 'new generation requires completed original reversals'; END IF;
  END IF;
 ELSE
  IF NOT EXISTS(SELECT 1 FROM periods p WHERE p.id=OLD.period_id AND p.current_settlement_job_id=OLD.id) OR EXISTS(SELECT 1 FROM draw_corrections c WHERE c.previous_job_id=OLD.id AND c.state<>'completed') THEN RAISE EXCEPTION 'superseded settlement generation is immutable'; END IF;
  IF (to_jsonb(NEW)-ARRAY['state','resume_state','version','approved_by','completed_at','last_error_code']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['state','resume_state','version','approved_by','completed_at','last_error_code']) OR NEW.version<>OLD.version+1 OR OLD.state='completed' THEN RAISE EXCEPTION 'immutable settlement job identity'; END IF;
  IF NEW.approved_by IS DISTINCT FROM OLD.approved_by AND NOT (OLD.state='awaiting_approval' AND NEW.state='paying' AND OLD.approved_by IS NULL AND NEW.approved_by IS NOT NULL AND NEW.mode='manual') THEN RAISE EXCEPTION 'invalid settlement approval'; END IF;
  IF OLD.state<>NEW.state AND NOT ((OLD.state='processing' AND NEW.state IN ('paying','awaiting_approval','failed')) OR (OLD.state='awaiting_approval' AND NEW.state='paying') OR (OLD.state='paying' AND NEW.state IN ('completed','failed')) OR (OLD.state='failed' AND NEW.state=OLD.resume_state)) THEN RAISE EXCEPTION 'invalid settlement job transition'; END IF;
 END IF; RETURN NEW; END $$;

CREATE FUNCTION guard_settlement_policy() RETURNS trigger
    LANGUAGE plpgsql
    AS $$ BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'settlement policy cannot be deleted'; END IF;
 IF NEW.brand_id<>OLD.brand_id OR NEW.version<>OLD.version+1 OR NEW.updated_at<OLD.updated_at OR
 NOT EXISTS(SELECT 1 FROM settlement_policy_history h JOIN audit_logs a ON a.id=h.audit_log_id WHERE h.brand_id=NEW.brand_id AND h.version=NEW.version AND h.mode IS NOT DISTINCT FROM NEW.mode AND a.action='settlement_policy.write' AND a.brand_id=NEW.brand_id) THEN RAISE EXCEPTION 'settlement policy requires versioned audit evidence'; END IF;
 RETURN NEW; END $$;

CREATE FUNCTION guard_settlement_target() RETURNS trigger
    LANGUAGE plpgsql
    AS $$ BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'settlement target history cannot be deleted'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.version<>1 OR NEW.calculation_id IS NOT NULL OR NEW.payout_entry_id IS NOT NULL OR NEW.error_code IS NOT NULL OR NOT EXISTS(SELECT 1 FROM bet_orders o JOIN settlement_jobs j ON j.id=NEW.job_id WHERE o.id=NEW.order_id AND o.period_id=NEW.period_id AND j.state='processing' AND j.version=1 AND ((o.status='placed' AND NEW.state='pending') OR (o.status IN ('abnormal','bet_cancelled','judged_cancelled') AND NEW.state='excluded'))) THEN RAISE EXCEPTION 'invalid initial settlement target'; END IF;
 ELSE
  IF (to_jsonb(NEW)-ARRAY['state','version','calculation_id','payout_entry_id','error_code']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['state','version','calculation_id','payout_entry_id','error_code']) OR NEW.version<>OLD.version+1 OR OLD.state IN ('paid','excluded') THEN RAISE EXCEPTION 'immutable settlement target identity'; END IF;
  IF NOT ((OLD.state='pending' AND NEW.state IN ('ready','excluded','failed')) OR (OLD.state='ready' AND NEW.state IN ('paid','excluded','failed')) OR (OLD.state='failed' AND NEW.state IN ('pending','ready','excluded'))) THEN RAISE EXCEPTION 'invalid settlement target transition'; END IF;
  IF OLD.calculation_id IS NOT NULL AND OLD.calculation_id IS DISTINCT FROM NEW.calculation_id THEN RAISE EXCEPTION 'immutable target calculation'; END IF;
 END IF;
 RETURN NEW; END $$;

CREATE FUNCTION guard_unexecuted_commission_correction_credit() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE t commission_correction_execution_targets; x commission_correction_executions; p commission_correction_plans; s commission_correction_execution_steps; cap bigint; total numeric;
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

CREATE FUNCTION guard_withdrawal_cycle() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE o withdrawal_orders;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'withdrawal cycle cannot be deleted'; END IF;
 SELECT * INTO o FROM withdrawal_orders WHERE brand_id=NEW.brand_id AND id=NEW.last_paid_order_id;
 IF o.id IS NULL OR o.state<>'paid' OR o.member_id<>NEW.member_id OR o.account_id<>NEW.account_id OR o.created_at<>NEW.cutoff_at OR o.reserve_version<>NEW.cutoff_version THEN RAISE EXCEPTION 'withdrawal cycle requires actual paid order submission boundary'; END IF;
 IF TG_OP='UPDATE' AND (ROW(NEW.brand_id,NEW.member_id,NEW.account_id) IS DISTINCT FROM ROW(OLD.brand_id,OLD.member_id,OLD.account_id) OR NEW.cutoff_version<=OLD.cutoff_version OR NEW.cutoff_at<OLD.cutoff_at) THEN RAISE EXCEPTION 'withdrawal cycle cannot move backwards'; END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_withdrawal_evidence() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE o withdrawal_orders; a audit_logs; prev withdrawal_order_transitions;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'withdrawal evidence is immutable'; END IF;
 SELECT * INTO o FROM withdrawal_orders WHERE brand_id=NEW.brand_id AND id=NEW.order_id;
 IF o.id IS NULL THEN RAISE EXCEPTION 'withdrawal evidence has no scoped order'; END IF;
 IF TG_TABLE_NAME='withdrawal_order_transitions' THEN
  SELECT * INTO a FROM audit_logs WHERE id=NEW.audit_log_id;
  IF a.id IS NULL OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.resource_type<>'withdrawal_order' OR a.resource_id IS DISTINCT FROM NEW.order_id OR a.actor_type<>NEW.actor_type OR a.actor_id IS DISTINCT FROM NEW.actor_id OR a.action IS DISTINCT FROM 'withdrawal.'||NEW.to_state OR a.reason IS DISTINCT FROM NEW.reason OR a.before_json->>'state' IS DISTINCT FROM NEW.from_state OR a.after_json->>'state' IS DISTINCT FROM NEW.to_state OR a.after_json->>'version' IS DISTINCT FROM NEW.version::text THEN RAISE EXCEPTION 'withdrawal transition has no scoped audit'; END IF;
  IF NEW.version=1 THEN
   IF NEW.from_state<>'' OR NEW.to_state<>'reviewing' OR NEW.actor_type<>'user' OR NEW.created_at<>o.created_at OR NEW.actor_id IS DISTINCT FROM (SELECT global_user_id FROM brand_members WHERE brand_id=o.brand_id AND id=o.member_id) THEN RAISE EXCEPTION 'invalid withdrawal creation evidence'; END IF;
  ELSE
   SELECT * INTO prev FROM withdrawal_order_transitions WHERE order_id=NEW.order_id AND version=NEW.version-1;
   IF prev.id IS NULL OR NEW.from_state<>prev.to_state OR NEW.created_at<prev.created_at OR NOT(
    NEW.from_state='reviewing' AND NEW.to_state IN('processing','rejected','cancelled') OR
    NEW.from_state='processing' AND NEW.to_state IN('paid','failed','cancelled')) THEN RAISE EXCEPTION 'invalid withdrawal transition evidence'; END IF;
   IF NEW.actor_type='user' OR NEW.actor_type='system' AND (NEW.to_state<>'processing' OR o.policy_snapshot->'config'->>'review_mode' IS DISTINCT FROM 'automatic') THEN RAISE EXCEPTION 'unauthorized withdrawal evidence actor'; END IF;
  END IF;
 ELSE
  IF NEW.response->>'id' IS DISTINCT FROM NEW.order_id::text OR NEW.response->>'brand_id' IS DISTINCT FROM NEW.brand_id::text OR NOT EXISTS(SELECT 1 FROM withdrawal_order_transitions WHERE order_id=NEW.order_id AND version=(NEW.response->>'version')::bigint AND to_state=NEW.response->>'state') THEN RAISE EXCEPTION 'invalid withdrawal original receipt'; END IF;
 END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_withdrawal_order_projection() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'withdrawal orders cannot be deleted'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.version<>1 OR NEW.state<>'reviewing' OR NEW.updated_at<>NEW.created_at THEN RAISE EXCEPTION 'withdrawal must start in review'; END IF;
 ELSE
  IF ROW(NEW.id,NEW.brand_id,NEW.member_id,NEW.account_id,NEW.points,NEW.source_allocation,NEW.policy_snapshot,NEW.eligibility_evidence,NEW.cycle_from_at,NEW.cycle_from_version,NEW.reserve_version,NEW.reserve_entry_id,NEW.created_at)
   IS DISTINCT FROM ROW(OLD.id,OLD.brand_id,OLD.member_id,OLD.account_id,OLD.points,OLD.source_allocation,OLD.policy_snapshot,OLD.eligibility_evidence,OLD.cycle_from_at,OLD.cycle_from_version,OLD.reserve_version,OLD.reserve_entry_id,OLD.created_at) THEN RAISE EXCEPTION 'withdrawal request facts are immutable'; END IF;
  IF NEW.version<>OLD.version+1 OR NEW.updated_at<OLD.updated_at OR NOT(
   OLD.state='reviewing' AND NEW.state IN('processing','rejected','cancelled') OR
   OLD.state='processing' AND NEW.state IN('paid','failed','cancelled')) THEN RAISE EXCEPTION 'invalid withdrawal transition'; END IF;
  IF OLD.reviewed_at IS NOT NULL AND NEW.reviewed_at IS DISTINCT FROM OLD.reviewed_at THEN RAISE EXCEPTION 'review time is immutable'; END IF;
 END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_withdrawal_outbox() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE o withdrawal_orders; t withdrawal_order_transitions;
BEGIN
 IF TG_OP='DELETE' THEN
  IF OLD.event_type LIKE 'withdrawal.order.%' THEN RAISE EXCEPTION 'withdrawal event immutable'; END IF;
  RETURN OLD;
 END IF;
 IF TG_OP='UPDATE' THEN
  IF OLD.event_type LIKE 'withdrawal.order.%' OR NEW.event_type LIKE 'withdrawal.order.%' THEN
   IF OLD.event_type NOT LIKE 'withdrawal.order.%' OR (to_jsonb(NEW)-'published_at') IS DISTINCT FROM (to_jsonb(OLD)-'published_at') THEN RAISE EXCEPTION 'withdrawal event immutable'; END IF;
  END IF;
  RETURN NEW;
 END IF;
 IF NEW.event_type NOT LIKE 'withdrawal.order.%' THEN RETURN NEW; END IF;
 SELECT * INTO o FROM withdrawal_orders WHERE brand_id=NEW.brand_id AND id=NEW.aggregate_id;
 IF NOT FOUND THEN RAISE EXCEPTION 'withdrawal event requires its order'; END IF;
 SELECT * INTO t FROM withdrawal_order_transitions WHERE brand_id=o.brand_id AND order_id=o.id AND version=(NEW.payload->>'version')::bigint;
 IF NOT FOUND OR NEW.event_type IS DISTINCT FROM 'withdrawal.order.'||t.to_state
 OR NEW.payload IS DISTINCT FROM jsonb_build_object('member_id',o.member_id::text,'resource_id',o.id::text,'points',o.points::text,'status',t.to_state,'version',t.version,'audit_log_id',t.audit_log_id::text) THEN
  RAISE EXCEPTION 'withdrawal event requires matching immutable transition';
 END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION guard_withdrawal_policy_current() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'withdrawal policy cannot be deleted'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.version<>1 THEN RAISE EXCEPTION 'withdrawal policy starts at version one'; END IF;
 ELSE
  IF (to_jsonb(NEW)-ARRAY['version','config','updated_at']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['version','config','updated_at']) OR NEW.version<>OLD.version+1 THEN RAISE EXCEPTION 'immutable policy scope or invalid version'; END IF;
 END IF;
 NEW.updated_at:=clock_timestamp();
 RETURN NEW;
END $$;

CREATE FUNCTION guard_withdrawal_policy_revision() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE current_version bigint; current_config jsonb;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'withdrawal policy history is immutable'; END IF;
 IF NEW.game_id IS NULL THEN SELECT version,config INTO current_version,current_config FROM brand_withdrawal_policies WHERE brand_id=NEW.brand_id;
 ELSE SELECT version,config INTO current_version,current_config FROM game_withdrawal_policies WHERE brand_id=NEW.brand_id AND game_id=NEW.game_id; END IF;
 IF NOT FOUND OR (NEW.version=1 AND (current_version<>1 OR current_config<>NEW.config)) OR (NEW.version>1 AND current_version<>NEW.version-1) THEN RAISE EXCEPTION 'revision must belong to current or next policy version'; END IF;
 NEW.created_at:=clock_timestamp();
 RETURN NEW;
END $$;

CREATE FUNCTION immutable_admin_role_identity() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF NEW.brand_id IS DISTINCT FROM OLD.brand_id OR NEW.code IS DISTINCT FROM OLD.code THEN
  RAISE EXCEPTION 'role brand and code are immutable' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;

CREATE TABLE brands (
    id uuid NOT NULL,
    code text NOT NULL,
    name text NOT NULL,
    status text NOT NULL,
    default_locale text DEFAULT 'en'::text NOT NULL,
    timezone text DEFAULT 'Asia/Manila'::text NOT NULL,
    theme jsonb DEFAULT '{}'::jsonb NOT NULL,
    config_version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    auth_config jsonb DEFAULT '{"captcha_enabled": false, "telegram_enabled": false, "service_terms_version": "dev-1", "privacy_policy_version": "dev-1"}'::jsonb NOT NULL,
    CONSTRAINT brands_status_check CHECK ((status = ANY (ARRAY['active'::text, 'paused'::text, 'disabled'::text])))
);

CREATE FUNCTION initial_brand_presentation(b brands) RETURNS jsonb
    LANGUAGE sql IMMUTABLE
    AS $_$
 SELECT jsonb_build_object(
 'display_name',NULL,'logo_text',NULL,'logo_url',NULL,'favicon_url',NULL,
 'primary_color',CASE WHEN b.theme->>'primary_color' ~ '^#[0-9a-fA-F]{6}$' THEN b.theme->'primary_color' ELSE NULL END,
 'accent_color',CASE WHEN b.theme->>'accent_color' ~ '^#[0-9a-fA-F]{6}$' THEN b.theme->'accent_color' ELSE NULL END,
 'success_color',CASE WHEN b.theme->>'success_color' ~ '^#[0-9a-fA-F]{6}$' THEN b.theme->'success_color' ELSE NULL END,
 'warning_color',CASE WHEN b.theme->>'warning_color' ~ '^#[0-9a-fA-F]{6}$' THEN b.theme->'warning_color' ELSE NULL END,
 'danger_color',CASE WHEN b.theme->>'danger_color' ~ '^#[0-9a-fA-F]{6}$' THEN b.theme->'danger_color' ELSE NULL END,
 'font_family',NULL,'font_scale',NULL,'radius',NULL,'shadow',NULL,
 'default_locale',CASE WHEN b.default_locale IN('en','zh-CN') THEN b.default_locale WHEN b.default_locale='zh' THEN 'zh-CN' ELSE NULL END,'available_locales',NULL,'content',NULL)
$_$;

CREATE FUNCTION initialize_agent_policy() RETURNS trigger
    LANGUAGE plpgsql
    AS $$ BEGIN INSERT INTO brand_agent_policies(brand_id) VALUES(NEW.id);RETURN NULL;END $$;

CREATE FUNCTION initialize_agent_policy_revision() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 INSERT INTO agent_config_revisions(brand_id,version,config,actor_type,reason) VALUES(NEW.brand_id,1,NEW.config,'system','Initial disabled agent policy');RETURN NULL;
END $$;

CREATE FUNCTION initialize_bet_policy() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF TG_TABLE_NAME='brands' THEN INSERT INTO brand_bet_policies(brand_id,config) VALUES(NEW.id,default_brand_bet_policy());
 ELSE INSERT INTO game_bet_policies(brand_id,game_id,config) VALUES(NEW.brand_id,NEW.id,default_game_bet_policy()); END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION initialize_brand_compliance() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 INSERT INTO brand_compliance_policies(brand_id) VALUES(NEW.id);
 INSERT INTO compliance_policy_revisions(brand_id,version,config,reason) VALUES(NEW.id,1,default_compliance_config(),'Initial disabled compliance configuration');
 RETURN NEW;
END $$;

CREATE FUNCTION initialize_brand_notification_templates() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 INSERT INTO notification_templates(brand_id,template_key,content) SELECT NEW.id,key,value FROM jsonb_each(notification_template_defaults());
 INSERT INTO notification_template_revisions(brand_id,template_key,version,content,reason)
 SELECT NEW.id,key,1,value,'Initial in-app notification template' FROM jsonb_each(notification_template_defaults());
 RETURN NEW;
END $$;

CREATE FUNCTION initialize_brand_point_policy() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN INSERT INTO brand_point_policies(brand_id) VALUES(NEW.id); RETURN NEW; END $$;

CREATE FUNCTION initialize_brand_presentation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN INSERT INTO brand_presentations(brand_id,version,config)VALUES(NEW.id,NEW.config_version,initial_brand_presentation(NEW));RETURN NEW;END $$;

CREATE FUNCTION initialize_commission_adjustment_head() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF OLD.state='pending' AND NEW.state='paid' THEN INSERT INTO commission_adjustment_heads(target_id,brand_id,points) VALUES(NEW.id,NEW.brand_id,NEW.points); END IF;
 RETURN NULL;
END $$;

CREATE FUNCTION initialize_commission_correction_policy() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN INSERT INTO brand_commission_correction_policies(brand_id) VALUES(NEW.id); RETURN NULL; END $$;

CREATE FUNCTION initialize_commission_payment_policy() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN INSERT INTO brand_commission_payment_policies(brand_id) VALUES(NEW.id); RETURN NULL; END $$;

CREATE FUNCTION initialize_commission_point_buckets() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 INSERT INTO point_buckets(brand_id,account_id,source,state,points)
 SELECT NEW.brand_id,NEW.id,'commission',s,0
 FROM unnest(ARRAY['available','manual_frozen','system_frozen','withdrawal']) s;
 RETURN NULL;
END $$;

CREATE FUNCTION initialize_commission_policy() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN INSERT INTO brand_commission_policies(brand_id) VALUES(NEW.id); RETURN NULL; END $$;

CREATE FUNCTION initialize_commission_policy_revision() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 INSERT INTO commission_policy_revisions(brand_id,version,config,reason) VALUES(NEW.brand_id,1,NEW.config,'Initial disabled commission financial policy'); RETURN NULL;
END $$;

CREATE FUNCTION initialize_report_archive_policy() RETURNS trigger
    LANGUAGE plpgsql
    AS $$ BEGIN INSERT INTO brand_report_archive_policies(brand_id,timezone) VALUES(NEW.id,NEW.timezone); RETURN NULL; END $$;

CREATE FUNCTION initialize_settlement_policy() RETURNS trigger
    LANGUAGE plpgsql
    AS $$ BEGIN
 INSERT INTO brand_settlement_policies(brand_id) VALUES(NEW.id); RETURN NEW; END $$;

CREATE FUNCTION initialize_withdrawal_policy() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF TG_TABLE_NAME='brands' THEN INSERT INTO brand_withdrawal_policies(brand_id) VALUES(NEW.id);
 ELSE INSERT INTO game_withdrawal_policies(brand_id,game_id) VALUES(NEW.brand_id,NEW.id); END IF;
 RETURN NULL;
END $$;

CREATE FUNCTION initialize_withdrawal_policy_revision() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 INSERT INTO withdrawal_policy_revisions(brand_id,game_id,version,config,reason) VALUES(NEW.brand_id,NULLIF(to_jsonb(NEW)->>'game_id','')::uuid,1,NEW.config,'Initial withdrawal policy');
 RETURN NULL;
END $$;

CREATE TABLE join_codes (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    kind text NOT NULL,
    code text NOT NULL,
    owner_member_id uuid NOT NULL,
    agent_id uuid,
    status text DEFAULT 'active'::text NOT NULL,
    starts_at timestamp with time zone,
    expires_at timestamp with time zone,
    version bigint DEFAULT 1 NOT NULL,
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT join_codes_check CHECK (((kind = 'agent'::text) = (agent_id IS NOT NULL))),
    CONSTRAINT join_codes_check1 CHECK (((starts_at IS NULL) OR (expires_at IS NULL) OR (starts_at < expires_at))),
    CONSTRAINT join_codes_code_check CHECK ((code ~ '^[A-F0-9]{24}$'::text)),
    CONSTRAINT join_codes_kind_check CHECK ((kind = ANY (ARRAY['agent'::text, 'referral'::text]))),
    CONSTRAINT join_codes_status_check CHECK ((status = ANY (ARRAY['active'::text, 'disabled'::text]))),
    CONSTRAINT join_codes_version_check CHECK ((version > 0))
);

CREATE FUNCTION join_code_usable(c join_codes) RETURNS boolean
    LANGUAGE sql
    AS $$ SELECT join_code_usable_at(c,clock_timestamp()) $$;

CREATE FUNCTION join_code_usable_at(c join_codes, at_time timestamp with time zone) RETURNS boolean
    LANGUAGE sql
    AS $$
 SELECT c.status='active' AND (c.starts_at IS NULL OR c.starts_at<=at_time) AND (c.expires_at IS NULL OR c.expires_at>at_time)
 AND EXISTS(SELECT 1 FROM brand_members m JOIN global_users u ON u.id=m.global_user_id JOIN brands b ON b.id=m.brand_id WHERE m.brand_id=c.brand_id AND m.id=c.owner_member_id AND m.status='normal' AND u.status='active' AND b.status<>'disabled')
 AND (c.kind='referral' OR EXISTS(SELECT 1 FROM agent_nodes n JOIN brand_agent_policies p ON p.brand_id=n.brand_id WHERE n.brand_id=c.brand_id AND n.id=c.agent_id AND n.member_id=c.owner_member_id AND p.config->'enabled'='true'::jsonb AND NOT EXISTS(SELECT 1 FROM agent_nodes a WHERE a.brand_id=n.brand_id AND a.id=ANY(n.path) AND a.config->>'status'<>'active')))
$$;

CREATE FUNCTION lottery_business_allocation_delta(v jsonb, negative boolean) RETURNS jsonb
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
   jsonb_typeof(item->'points') IS DISTINCT FROM 'string' OR item->>'points' !~ '^[1-9][0-9]*$' THEN RETURN NULL; END IF;
  amount:=(item->>'points')::numeric;
  IF amount>9223372036854775807 THEN RETURN NULL; END IF;
  seen:=array_append(seen,src);
  expected:=jsonb_set(expected,ARRAY[src,st],to_jsonb((CASE WHEN negative THEN -amount ELSE amount END)::text),false);
 END LOOP;
 RETURN expected;
EXCEPTION WHEN OTHERS THEN RETURN NULL;
END $_$;

CREATE TABLE point_ledger_entries (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    account_id uuid NOT NULL,
    entry_type text NOT NULL,
    reference_type text NOT NULL,
    reference_id uuid,
    operation_key text NOT NULL,
    before_snapshot jsonb NOT NULL,
    delta_snapshot jsonb NOT NULL,
    after_snapshot jsonb NOT NULL,
    source_allocation jsonb DEFAULT '[]'::jsonb NOT NULL,
    reason text NOT NULL,
    actor_id uuid,
    request_id text NOT NULL,
    reversal_of uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    member_id uuid NOT NULL,
    version bigint NOT NULL,
    request_hash text NOT NULL,
    actor_type text NOT NULL,
    CONSTRAINT ledger_actor_type CHECK ((actor_type = ANY (ARRAY['user'::text, 'admin'::text, 'system'::text]))),
    CONSTRAINT ledger_hash_valid CHECK ((request_hash ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT ledger_version_positive CHECK ((version > 0))
);

CREATE FUNCTION lottery_business_ledger_applied(l point_ledger_entries, b uuid, a uuid) RETURNS boolean
    LANGUAGE plpgsql STABLE
    AS $$
DECLARE before_value jsonb; delta_value jsonb; after_value jsonb; src text; st text;
BEGIN
 IF l.id IS NULL OR l.brand_id IS DISTINCT FROM b OR l.account_id IS DISTINCT FROM a OR
  l.version IS NULL OR l.version<1 OR
  valid_point_snapshot(l.before_snapshot,false) IS NOT TRUE OR
  valid_point_snapshot(l.delta_snapshot,true) IS NOT TRUE OR
  valid_point_snapshot(l.after_snapshot,false) IS NOT TRUE OR
  NOT EXISTS(SELECT 1 FROM point_accounts pa WHERE pa.brand_id=b AND pa.id=a AND pa.brand_member_id=l.member_id AND pa.version>=l.version) THEN
  RETURN false;
 END IF;
 before_value:=validated_point_snapshot(l.before_snapshot);
 delta_value:=validated_point_snapshot(l.delta_snapshot);
 after_value:=validated_point_snapshot(l.after_snapshot);
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
  validated_point_snapshot(prev.after_snapshot)=before_value) THEN
  RETURN false;
 END IF;
 RETURN COALESCE(EXISTS(
  SELECT 1 FROM audit_logs x WHERE x.brand_id=b AND x.actor_type=l.actor_type AND x.actor_id IS NOT DISTINCT FROM l.actor_id AND
   x.action='points.'||l.entry_type AND x.resource_type='point_account' AND x.resource_id=a AND x.request_id=l.request_id AND
   x.before_json=l.before_snapshot AND x.after_json->>'ledger_entry_id'=l.id::text AND x.after_json->'balance'=l.after_snapshot
 ),false);
EXCEPTION WHEN OTHERS THEN RETURN false;
END $$;

CREATE FUNCTION lottery_business_negate_snapshot(v jsonb) RETURNS jsonb
    LANGUAGE plpgsql IMMUTABLE
    AS $$
DECLARE expected jsonb:=validated_point_snapshot(v); src text; st text; amount numeric;
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

CREATE FUNCTION validated_point_snapshot(v jsonb) RETURNS jsonb
    LANGUAGE plpgsql IMMUTABLE
    AS $$
BEGIN
 IF valid_point_snapshot(v,true) IS NOT TRUE THEN RETURN NULL; END IF;
 RETURN v;
END $$;

CREATE FUNCTION notification_template_defaults() RETURNS jsonb
    LANGUAGE sql IMMUTABLE
    AS $$SELECT '{"bet.order.won": {"en": {"body": "Historical record: {points} points were credited as this order''s prize. This records the credit, not your current wallet balance or a guaranteed final outcome. Any correction will appear as a separate prize event; this record is retained.", "title": "Prize credit recorded"}, "zh-CN": {"body": "历史记录：此注单的 {points} 积分奖金已记入账本。此记录仅表示该笔入账，不代表当前钱包余额，也不保证最终结果。任何更正都会作为单独的奖金事件记录；此记录会保留。", "title": "派奖入账记录"}}, "member.joined": {"en": {"body": "Your membership is ready. Welcome aboard.", "title": "Welcome"}, "zh-CN": {"body": "您的会员账户已准备就绪，欢迎加入。", "title": "欢迎"}}, "commission.paid": {"en": {"body": "Historical record: {points} points were credited to your commission wallet. This records a past credit, not new income or an external payment forecast. Check your current wallet balance; this record is retained.", "title": "Commission credit recorded"}, "zh-CN": {"body": "历史记录：{points} 积分曾记入佣金钱包。此记录表示过去的入账，不是新收入预测或外部付款承诺。请查看当前钱包余额；此记录会保留。", "title": "佣金入账记录"}}, "bet.order.placed": {"en": {"body": "Order submitted: {points} points.", "title": "Order submitted"}, "zh-CN": {"body": "注单已提交，涉及 {points} 积分。", "title": "注单已提交"}}, "bet.order.abnormal": {"en": {"body": "Your order needs manual review. Points involved: {points}.", "title": "Order needs review"}, "zh-CN": {"body": "您的注单需要人工处理，涉及积分：{points}。", "title": "注单待人工处理"}}, "recharge.confirmed": {"en": {"body": "Recharge confirmed: {points} points.", "title": "Recharge confirmed"}, "zh-CN": {"body": "充值已确认：{points} 积分。", "title": "充值已确认"}}, "bet.order.cancelled": {"en": {"body": "Cancelled. {points} points were returned to your original balance.", "title": "Order cancelled"}, "zh-CN": {"body": "注单已取消，{points} 积分已原路退回。", "title": "注单已取消"}}, "commission.adjusted": {"en": {"body": "Historical record: a commission adjustment of {points} points was recorded. This records a past adjustment, not new income or an external payment forecast. Check your current wallet balance; this record is retained.", "title": "Commission adjustment recorded"}, "zh-CN": {"body": "历史记录：曾调整 {points} 积分。此记录表示过去的调整，不是新收入预测或外部付款承诺。请查看当前钱包余额；此记录会保留。", "title": "佣金调整记录"}}, "commission.corrected": {"en": {"body": "Historical record: a commission correction of {points} points was posted to your commission available balance. Positive points record a past additional credit; negative points record a past recovery. This is not your current balance, new income or an external payment. Check your current wallet; this record is retained.", "title": "Commission correction recorded"}, "zh-CN": {"body": "历史记录：佣金可用积分曾发生 {points} 积分更正。正数表示过去的补发，负数表示过去的追回；不代表当前余额、新收入或外部付款。请查看当前钱包；此记录会保留。", "title": "佣金更正记录"}}, "reward.order.granted": {"en": {"body": "Historical record: {points} points were credited to your gift available balance. This records a past grant, not your current wallet balance or an external payment. Check your current wallet; this record is retained.", "title": "Reward grant recorded"}, "zh-CN": {"body": "历史记录：{points} 积分曾记入赠送可用积分。此记录表示过去的发放，不代表当前钱包余额或外部付款。请查看当前钱包；此记录会保留。", "title": "奖励发放记录"}}, "reward.order.revoked": {"en": {"body": "Historical record: the full original reward of {points} points was reversed from your gift available balance. This records a past reversal, not your current wallet balance or an external payment. Check your current wallet; this record is retained.", "title": "Reward reversal recorded"}, "zh-CN": {"body": "历史记录：原奖励全额 {points} 积分曾从赠送可用积分撤销。此记录表示过去的撤销，不代表当前钱包余额或外部付款。请查看当前钱包；此记录会保留。", "title": "奖励撤销记录"}}, "draw.result.corrected": {"en": {"body": "Historical result ID {resource_id}: the corrected result for this period. This is not a guarantee of a win or prize payment. View the actual result in the app; users cannot edit this notice.", "title": "Draw result corrected"}, "zh-CN": {"body": "历史开奖结果 ID：{resource_id}，表示本期已更正的结果，不代表中奖或派奖保证。请在应用中查看实际结果；用户不能编辑此通知。", "title": "开奖结果已更正"}}, "draw.result.published": {"en": {"body": "Historical result ID {resource_id}: the result published for this period. This is not a guarantee of a win or prize payment. View the actual result in the app; users cannot edit this notice.", "title": "Draw result published"}, "zh-CN": {"body": "历史开奖结果 ID：{resource_id}，表示本期已公布的结果，不代表中奖或派奖保证。请在应用中查看实际结果；用户不能编辑此通知。", "title": "开奖结果已公布"}}, "withdrawal.order.paid": {"en": {"body": "Historical withdrawal status: paid. Points involved: {points}. This is an internal points record, not proof of an external transfer. Check the withdrawal order for its current state.", "title": "Withdrawal status recorded"}, "zh-CN": {"body": "历史提现状态：已提现，涉及 {points} 积分。此为内部积分记录，不证明外部转账；请查询提现订单的最新状态。", "title": "提现状态记录"}}, "withdrawal.order.failed": {"en": {"body": "Historical withdrawal status: failed. Points involved: {points}. This is an internal points record, not proof of an external transfer. Check the withdrawal order for its current state.", "title": "Withdrawal status recorded"}, "zh-CN": {"body": "历史提现状态：失败，涉及 {points} 积分。此为内部积分记录，不证明外部转账；请查询提现订单的最新状态。", "title": "提现状态记录"}}, "bet.order.prize_reversed": {"en": {"body": "Historical record: the full original prize amount of {points} points for this order was reversed. This records the reversal, not your current wallet balance. Any later prize correction will appear as a separate event; this record is retained.", "title": "Prize reversal recorded"}, "zh-CN": {"body": "历史记录：此注单原奖金全额 {points} 积分已冲回。此记录仅表示该笔冲正，不代表当前钱包余额。之后如有奖金更正，会作为单独事件记录；此记录会保留。", "title": "奖金冲正记录"}}, "withdrawal.order.rejected": {"en": {"body": "Historical withdrawal status: rejected. Points involved: {points}. This is an internal points record, not proof of an external transfer. Check the withdrawal order for its current state.", "title": "Withdrawal status recorded"}, "zh-CN": {"body": "历史提现状态：已驳回，涉及 {points} 积分。此为内部积分记录，不证明外部转账；请查询提现订单的最新状态。", "title": "提现状态记录"}}, "bet.order.judged_cancelled": {"en": {"body": "Cancelled after review. {points} points were returned to your original balance.", "title": "Order cancelled after review"}, "zh-CN": {"body": "注单经判定已取消，{points} 积分已原路退回。", "title": "注单已判定取消"}}, "withdrawal.order.cancelled": {"en": {"body": "Historical withdrawal status: cancelled. Points involved: {points}. This is an internal points record, not proof of an external transfer. Check the withdrawal order for its current state.", "title": "Withdrawal status recorded"}, "zh-CN": {"body": "历史提现状态：已取消，涉及 {points} 积分。此为内部积分记录，不证明外部转账；请查询提现订单的最新状态。", "title": "提现状态记录"}}, "withdrawal.order.reviewing": {"en": {"body": "Historical withdrawal status: reviewing. Points involved: {points}. This is an internal points record, not proof of an external transfer. Check the withdrawal order for its current state.", "title": "Withdrawal status recorded"}, "zh-CN": {"body": "历史提现状态：审核中，涉及 {points} 积分。此为内部积分记录，不证明外部转账；请查询提现订单的最新状态。", "title": "提现状态记录"}}, "withdrawal.order.processing": {"en": {"body": "Historical withdrawal status: processing. Points involved: {points}. This is an internal points record, not proof of an external transfer. Check the withdrawal order for its current state.", "title": "Withdrawal status recorded"}, "zh-CN": {"body": "历史提现状态：提现中，涉及 {points} 积分。此为内部积分记录，不证明外部转账；请查询提现订单的最新状态。", "title": "提现状态记录"}}, "reward.order.revocation_pending": {"en": {"body": "Historical record: a full reward reversal of {points} points was requested and recorded as awaiting operator handling. No points moved in this attempt. It does not retry, unfreeze or deduct automatically; check the current wallet.", "title": "Reward revocation pending recorded"}, "zh-CN": {"body": "历史记录：曾申请全额撤销 {points} 积分奖励，并记录为待运营处理。本次没有积分变动，不会自动重试、解冻或扣除；请查看当前钱包。", "title": "奖励撤销待处理记录"}}}'::jsonb$$;

CREATE FUNCTION point_business_family(kind text) RETURNS text
    LANGUAGE sql IMMUTABLE
    AS $$
 SELECT CASE
  WHEN kind IN('adjust','adjustment','freeze','unfreeze','withdrawal_transfer','reversal') THEN 'manual'
  WHEN kind IN('withdrawal_reserve','withdrawal_release','withdrawal_paid') THEN 'withdrawal'
  WHEN kind IN('reward_grant','reward_reversal') THEN 'reward'
  WHEN kind='recharge' THEN 'recharge'
  WHEN kind IN('bet','refund','prize','prize_reversal','commission','commission_adjustment','commission_correction') THEN kind
  ELSE 'unknown' END
$$;

CREATE FUNCTION point_business_preview(b uuid, account uuid) RETURNS jsonb
    LANGUAGE sql STABLE
    AS $$
 WITH scope AS MATERIALIZED (
  SELECT id,brand_member_id,version FROM point_accounts WHERE brand_id=b AND id=account
 ), ledgers AS MATERIALIZED (
  SELECT l.*,point_business_family(l.entry_type) AS family FROM point_ledger_entries l
  JOIN scope s ON s.id=l.account_id WHERE l.brand_id=b
 ), witnesses AS MATERIALIZED (
  SELECT * FROM point_lottery_business_witnesses(b,account)
  UNION ALL SELECT * FROM point_finance_business_witnesses(b,account)
  UNION ALL
  SELECT 'manual',l.id,l.id,
   (SELECT count(*)=1 FROM audit_logs a
    WHERE a.brand_id=b AND a.actor_type=l.actor_type AND a.actor_id IS NOT DISTINCT FROM l.actor_id
     AND a.action='points.'||l.entry_type AND a.resource_type='point_account' AND a.resource_id=account
     AND a.reason=l.reason AND a.request_id=l.request_id
     AND a.after_json->>'ledger_entry_id'=l.id::text
     AND a.before_json=l.before_snapshot AND a.after_json=jsonb_build_object('ledger_entry_id',l.id,'version',l.version,'balance',l.after_snapshot)),
   l.entry_type FROM ledgers l WHERE l.family='manual'
 ), problems AS MATERIALIZED (
  SELECT l.family,'UNSUPPORTED_LEDGER_TYPE'::text AS code,l.entry_type,l.id AS ledger_id,
   'ledger'::text AS resource_type,l.id AS resource_id FROM ledgers l WHERE l.family='unknown'
  UNION ALL
  SELECT l.family,'INVALID_BUSINESS_BINDING',l.entry_type,l.id,'ledger',l.id FROM ledgers l
   WHERE l.family<>'manual' AND (SELECT count(*) FROM audit_logs a
    WHERE a.brand_id=b AND a.actor_type=l.actor_type AND a.actor_id IS NOT DISTINCT FROM l.actor_id
     AND a.action='points.'||l.entry_type AND a.resource_type='point_account' AND a.resource_id=account
     AND a.reason=l.reason AND a.request_id=l.request_id AND a.after_json->>'ledger_entry_id'=l.id::text
     AND a.before_json=l.before_snapshot AND a.after_json=jsonb_build_object('ledger_entry_id',l.id,'version',l.version,'balance',l.after_snapshot))<>1
  UNION ALL
  SELECT l.family,'MISSING_BUSINESS_RECORD',l.entry_type,l.id,'ledger',l.id FROM ledgers l
   WHERE l.family NOT IN('unknown','manual') AND NOT EXISTS(SELECT 1 FROM witnesses w WHERE w.ledger_id=l.id)
  UNION ALL
  SELECT w.family,'MISSING_LEDGER_ENTRY',w.expected_entry_type,w.ledger_id,w.family,w.source_id FROM witnesses w
   WHERE w.ledger_id IS NULL OR NOT EXISTS(SELECT 1 FROM ledgers l WHERE l.id=w.ledger_id)
  UNION ALL
  SELECT w.family,CASE WHEN w.family='manual' THEN 'INVALID_MANUAL_AUDIT' ELSE 'INVALID_BUSINESS_BINDING' END,
   w.expected_entry_type,w.ledger_id,w.family,w.source_id FROM witnesses w
   WHERE w.ledger_id IS NOT NULL AND w.source_valid IS DISTINCT FROM true AND EXISTS(SELECT 1 FROM ledgers l WHERE l.id=w.ledger_id)
  UNION ALL
  SELECT l.family,'DUPLICATE_BUSINESS_BINDING',l.entry_type,l.id,'ledger',l.id FROM ledgers l
   WHERE (SELECT count(*) FROM witnesses w WHERE w.ledger_id=l.id)>1
 ), families AS (
  SELECT unnest(ARRAY['bet','commission','commission_adjustment','commission_correction','manual','prize','prize_reversal','recharge','refund','reward','unknown','withdrawal']) AS family
 ), coverage AS (
  SELECT f.family,(SELECT count(*)::text FROM ledgers l WHERE l.family=f.family) AS ledger_entry_count,
   (SELECT count(*)::text FROM witnesses w WHERE w.family=f.family) AS business_reference_count,
   (SELECT count(*)::text FROM problems p WHERE p.family=f.family) AS issue_count FROM families f
 ), limited AS (
  SELECT code,entry_type,ledger_id AS ledger_entry_id,resource_type,resource_id FROM problems
  ORDER BY code COLLATE "C",resource_type COLLATE "C",coalesce(resource_id::text,''),coalesce(ledger_id::text,''),entry_type COLLATE "C" LIMIT 100
 ), material AS (
  SELECT jsonb_build_object('brand_id',b,'account_id',account,'account_version',(SELECT version FROM scope),
   'ledgers',coalesce((SELECT jsonb_agg(to_jsonb(l) ORDER BY version,id) FROM ledgers l),'[]'::jsonb),
   'witnesses',coalesce((SELECT jsonb_agg(to_jsonb(w) ORDER BY family COLLATE "C",source_id,ledger_id,expected_entry_type COLLATE "C") FROM witnesses w),'[]'::jsonb),
   'problems',coalesce((SELECT jsonb_agg(to_jsonb(p) ORDER BY code COLLATE "C",resource_type COLLATE "C",resource_id,ledger_id,entry_type COLLATE "C") FROM problems p),'[]'::jsonb)) AS data
 )
 SELECT jsonb_build_object('account_id',s.id,'member_id',s.brand_member_id,'account_version',s.version,
  'ledger_entry_count',(SELECT count(*)::text FROM ledgers),'business_reference_count',(SELECT count(*)::text FROM witnesses),
  'issue_count',(SELECT count(*)::text FROM problems),'issues_truncated',(SELECT count(*)>100 FROM problems),
  'consistent',NOT EXISTS(SELECT 1 FROM problems),'fingerprint',encode(sha256(convert_to(material.data::text,'UTF8')),'hex'),
  'issues',coalesce((SELECT jsonb_agg(to_jsonb(x) ORDER BY code COLLATE "C",resource_type COLLATE "C",coalesce(resource_id::text,''),coalesce(ledger_entry_id::text,''),entry_type COLLATE "C") FROM limited x),'[]'::jsonb),
  'coverage',(SELECT jsonb_agg(to_jsonb(c) ORDER BY family COLLATE "C") FROM coverage c))
 FROM scope s CROSS JOIN material
$$;

CREATE FUNCTION point_finance_business_witnesses(b uuid, a uuid) RETURNS TABLE(family text, source_id uuid, ledger_id uuid, source_valid boolean, expected_entry_type text)
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

CREATE FUNCTION point_lottery_business_witnesses(b uuid, a uuid) RETURNS TABLE(family text, source_id uuid, ledger_id uuid, source_valid boolean, expected_entry_type text)
    LANGUAGE sql STABLE
    AS $$
 WITH recharge_edges AS (
  SELECT 'recharge'::text AS family,r.id AS source_id,r.ledger_entry_id AS ledger_id,'recharge'::text AS expected_entry_type,
   COALESCE(r.state='confirmed' AND r.confirmed_at IS NOT NULL AND r.confirmed_by IS NOT NULL AND
    pa.brand_member_id=r.member_id AND l.id IS NOT NULL AND l.brand_id=r.brand_id AND l.account_id=r.account_id AND l.member_id=r.member_id AND
    l.entry_type='recharge' AND l.reference_type='recharge' AND l.reference_id=r.id AND l.operation_key='recharge-confirm:'||r.id::text AND
    l.actor_type='admin' AND l.actor_id=r.confirmed_by AND l.reason<>'' AND l.reversal_of IS NULL AND
    l.source_allocation=jsonb_build_array(jsonb_build_object('source','recharge','state','available','points',r.points::text)) AND
    validated_point_snapshot(l.delta_snapshot)=lottery_business_allocation_delta(l.source_allocation,false) AND
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
    validated_point_snapshot(l.delta_snapshot)=lottery_business_allocation_delta(o.deduction_allocation,true) AND
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
    validated_point_snapshot(l.delta_snapshot)=lottery_business_negate_snapshot(debit.delta_snapshot) AND lottery_business_ledger_applied(l,b,a) AND
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
    validated_point_snapshot(l.delta_snapshot)=lottery_business_allocation_delta(l.source_allocation,false) AND lottery_business_ledger_applied(l,b,a) AND
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
    validated_point_snapshot(original.delta_snapshot)=lottery_business_allocation_delta(original.source_allocation,false) AND
    lottery_business_ledger_applied(original,b,a) AND
    l.id IS NOT NULL AND lottery_business_ledger_applied(l,b,a) AND validated_point_snapshot(l.delta_snapshot)=lottery_business_negate_snapshot(original.delta_snapshot) AND
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

CREATE FUNCTION point_zero_snapshot() RETURNS jsonb
    LANGUAGE sql IMMUTABLE
    AS $$
 SELECT jsonb_object_agg(s,jsonb_build_object('available','0','manual_frozen','0','system_frozen','0','withdrawal','0'))
 FROM unnest(ARRAY['recharge','winning','gift','commission']) s
$$;

CREATE FUNCTION protect_assigned_admin_scope() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF TG_OP='UPDATE' AND NEW.account_id=OLD.account_id AND NEW.brand_id=OLD.brand_id THEN
  RETURN NEW;
 END IF;
 IF EXISTS(SELECT 1 FROM admin_account_roles ar JOIN roles r ON r.id=ar.role_id
 WHERE ar.account_id=OLD.account_id AND r.brand_id=OLD.brand_id) THEN
  RAISE EXCEPTION 'remove assigned brand roles before changing account scope' USING ERRCODE='23514';
 END IF;
 IF TG_OP='DELETE' THEN RETURN OLD; END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION reject_immutable_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 RAISE EXCEPTION 'append-only table cannot be updated or deleted';
END;
$$;

CREATE TABLE report_archives (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    kind text NOT NULL,
    period_key text NOT NULL,
    timezone text NOT NULL,
    from_at timestamp with time zone NOT NULL,
    to_at timestamp with time zone NOT NULL,
    revision bigint NOT NULL,
    previous_id uuid,
    snapshot_at timestamp with time zone NOT NULL,
    created_by uuid,
    reason text NOT NULL,
    request_id text NOT NULL,
    payload jsonb NOT NULL,
    payload_sha256 text NOT NULL,
    audit_log_id uuid NOT NULL,
    created_at timestamp with time zone NOT NULL,
    automatic_task_id uuid,
    automatic_policy_version bigint,
    CONSTRAINT report_archive_automatic_identity CHECK ((((created_by IS NULL) = (automatic_task_id IS NOT NULL)) AND ((automatic_task_id IS NULL) = (automatic_policy_version IS NULL)))),
    CONSTRAINT report_archives_check CHECK ((to_at > from_at)),
    CONSTRAINT report_archives_check1 CHECK (((revision = 1) = (previous_id IS NULL))),
    CONSTRAINT report_archives_check2 CHECK (((created_at = snapshot_at) AND (snapshot_at >= to_at))),
    CONSTRAINT report_archives_kind_check CHECK ((kind = ANY (ARRAY['daily'::text, 'monthly'::text]))),
    CONSTRAINT report_archives_payload_check CHECK ((jsonb_typeof(payload) = 'object'::text)),
    CONSTRAINT report_archives_payload_sha256_check CHECK ((payload_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT report_archives_reason_check CHECK ((((octet_length(reason) >= 1) AND (octet_length(reason) <= 500)) AND (btrim(reason) = reason) AND (reason !~ '[\r\n]'::text))),
    CONSTRAINT report_archives_request_id_check CHECK (((length(request_id) >= 1) AND (length(request_id) <= 80))),
    CONSTRAINT report_archives_revision_check CHECK (((revision >= 1) AND (revision <= '9007199254740991'::bigint)))
);

CREATE FUNCTION report_archive_automatic_valid(r report_archives) RETURNS boolean
    LANGUAGE sql STABLE
    AS $$
 SELECT EXISTS(SELECT 1 FROM report_archive_automatic_tasks j JOIN report_archive_policy_revisions p ON p.brand_id=j.brand_id AND p.version=j.policy_version
 JOIN brand_report_archive_policies c ON c.brand_id=j.brand_id JOIN brands b ON b.id=j.brand_id JOIN audit_logs a ON a.id=r.audit_log_id
 WHERE j.id=r.automatic_task_id AND j.brand_id=r.brand_id AND j.state='pending' AND j.policy_version=r.automatic_policy_version AND j.kind=r.kind AND j.period_key=r.period_key AND j.timezone=r.timezone AND j.from_at=r.from_at AND j.to_at=r.to_at
 AND b.status<>'disabled' AND ((j.kind='daily' AND c.daily_enabled AND p.daily_enabled) OR (j.kind='monthly' AND c.monthly_enabled AND p.monthly_enabled))
 AND r.created_by IS NULL AND r.revision=1 AND r.previous_id IS NULL AND r.snapshot_at=statement_timestamp() AND r.created_at=r.snapshot_at AND r.to_at<=r.snapshot_at
 AND NOT EXISTS(SELECT 1 FROM report_archives old WHERE old.brand_id=r.brand_id AND old.kind=r.kind AND old.period_key=r.period_key)
 AND a.brand_id=r.brand_id AND a.actor_type='system' AND a.actor_id IS NULL AND a.action='report_archive.create.automatic' AND a.resource_type='report_archive' AND a.resource_id=r.id AND a.reason=r.reason AND a.request_id=r.request_id AND a.before_json='null'::jsonb
 AND a.after_json=jsonb_build_object('id',r.id,'task_id',j.id,'policy_version',j.policy_version)
 AND r.payload=report_archive_capture(r.brand_id,r.from_at,r.to_at,r.timezone) AND r.payload_sha256=encode(sha256(convert_to(r.payload::text,'UTF8')),'hex'))
$$;

CREATE FUNCTION report_archive_capture(b uuid, f timestamp with time zone, t timestamp with time zone, z text DEFAULT NULL::text) RETURNS jsonb
    LANGUAGE sql STABLE
    AS $_$
WITH scope AS (
 SELECT id, CASE WHEN $4 IS NULL THEN timezone ELSE $4 END AS timezone
 FROM brands
 WHERE id=$1 AND ($4 IS NULL OR EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=$4))
),

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
$_$;

CREATE FUNCTION report_archive_cursor_due(k text, key text, z text) RETURNS timestamp with time zone
    LANGUAGE plpgsql STABLE
    AS $_$
DECLARE d timestamp; e timestamp; f timestamptz; t timestamptz;
BEGIN
 IF key IS NULL THEN RETURN NULL; END IF;
 IF k='daily' AND key ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}$' THEN d:=make_timestamp(substring(key,1,4)::integer,substring(key,6,2)::integer,substring(key,9,2)::integer,0,0,0);e:=d+interval '1 day';
 ELSIF k='monthly' AND key ~ '^[0-9]{4}-[0-9]{2}$' THEN d:=make_timestamp(substring(key,1,4)::integer,substring(key,6,2)::integer,1,0,0,0);e:=d+interval '1 month'; ELSE RAISE EXCEPTION 'invalid archive cursor key'; END IF;
 IF extract(year FROM d) NOT BETWEEN 1 AND 9998 THEN RAISE EXCEPTION 'invalid archive cursor year'; END IF;
 f:=CASE WHEN k='monthly' THEN report_archive_period_end_boundary(d,z) ELSE report_archive_period_boundary(d,z) END;t:=report_archive_period_end_boundary(e,z);
 IF f IS NULL OR t IS NULL THEN RETURN statement_timestamp(); END IF;
 RETURN t;
END $_$;

CREATE FUNCTION report_archive_period_boundary(civil timestamp without time zone, z text) RETURNS timestamp with time zone
    LANGUAGE sql STABLE
    AS $$
 WITH offsets AS (
  SELECT DISTINCT (probe AT TIME ZONE z)-(probe AT TIME ZONE 'UTC') AS offset_value
  FROM generate_series((civil AT TIME ZONE 'UTC')-interval '48 hours',(civil AT TIME ZONE 'UTC')+interval '48 hours',interval '30 minutes') probe
 ), candidates AS (
  SELECT DISTINCT (civil-offset_value) AT TIME ZONE 'UTC' AS candidate FROM offsets
  WHERE (((civil-offset_value) AT TIME ZONE 'UTC') AT TIME ZONE z)::date=civil::date
 ) SELECT min(candidate) FROM candidates
$$;

CREATE FUNCTION report_archive_period_end_boundary(civil timestamp without time zone, z text) RETURNS timestamp with time zone
    LANGUAGE plpgsql STABLE
    AS $$
DECLARE step integer; result timestamptz;
BEGIN
 FOR step IN 0..7 LOOP
  result:=report_archive_period_boundary(civil+step*interval '1 day',z);
  IF result IS NOT NULL THEN RETURN result; END IF;
 END LOOP;
 RETURN NULL;
END $$;

CREATE FUNCTION require_agent_revision() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE node uuid:=NULLIF(to_jsonb(NEW)->>'id','')::uuid;
BEGIN
 IF NOT EXISTS(SELECT 1 FROM agent_config_revisions WHERE brand_id=NEW.brand_id AND agent_id IS NOT DISTINCT FROM node AND version=NEW.version AND config=NEW.config) THEN RAISE EXCEPTION 'agent current version needs revision'; END IF;
 RETURN NULL;
END $$;

CREATE FUNCTION require_brand_domain_history() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE projection jsonb;v bigint;
BEGIN
 SELECT config_version INTO v FROM brands WHERE id=NEW.brand_id;
 SELECT coalesce(jsonb_agg(jsonb_build_object('id',d.id,'domain',d.domain,'enabled',d.enabled,'is_primary',d.is_primary) ORDER BY d.domain,d.id),'[]'::jsonb) INTO projection FROM brand_domains d WHERE brand_id=NEW.brand_id;
 IF NOT EXISTS(SELECT 1 FROM brand_domain_revisions h WHERE h.brand_id=NEW.brand_id AND h.version=v AND h.domains=projection) THEN RAISE EXCEPTION 'domain change requires audited history';END IF;
 RETURN NULL;
END $$;

CREATE FUNCTION require_brand_presentation_history() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE p brand_presentations;
BEGIN
 SELECT * INTO p FROM brand_presentations WHERE brand_id=NEW.brand_id;
 IF NOT EXISTS(SELECT 1 FROM brand_presentation_revisions h WHERE h.brand_id=p.brand_id AND h.version=p.version AND h.config=p.config) THEN RAISE EXCEPTION 'presentation requires audited history';END IF;
 RETURN NULL;
END $$;

CREATE FUNCTION require_commission_adjustment_commit() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE a commission_adjustments;
BEGIN
 SELECT * INTO a FROM commission_adjustments WHERE id=NEW.id;
 IF a.ledger_entry_id IS NULL OR NOT EXISTS(SELECT 1 FROM commission_adjustment_heads h WHERE h.brand_id=a.brand_id AND h.target_id=a.target_id AND h.version>=a.version) OR
  NOT EXISTS(SELECT 1 FROM point_ledger_entries l JOIN commission_payment_targets t ON t.brand_id=a.brand_id AND t.id=a.target_id WHERE l.id=a.ledger_entry_id AND l.brand_id=a.brand_id AND l.member_id=t.member_id AND l.entry_type='commission_adjustment' AND l.reference_type='commission_adjustment' AND l.reference_id=a.id AND l.actor_type='admin' AND l.actor_id=a.created_by AND l.operation_key='commission-adjustment:'||a.id::text) THEN RAISE EXCEPTION 'orphan commission adjustment cannot commit'; END IF;
 RETURN NULL;
END $$;

CREATE FUNCTION require_commission_adjustment_ledger_commit() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM commission_adjustments WHERE brand_id=NEW.brand_id AND id=NEW.reference_id AND ledger_entry_id=NEW.id) THEN RAISE EXCEPTION 'orphan commission adjustment ledger'; END IF;RETURN NULL;
END $$;

CREATE FUNCTION require_commission_correction_execution_commit() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE x commission_correction_executions;
BEGIN
 SELECT * INTO x FROM commission_correction_executions WHERE id=NEW.id;
 IF NOT EXISTS(SELECT 1 FROM commission_correction_execution_steps WHERE execution_id=x.id AND version=1 AND operation='create' AND audit_log_id=x.creation_audit_log_id) OR
  NOT EXISTS(SELECT 1 FROM commission_correction_execution_steps WHERE execution_id=x.id AND version=x.version AND audit_log_id=x.last_audit_log_id) THEN RAISE EXCEPTION 'orphan correction execution'; END IF;
 RETURN NULL;
END $$;

CREATE FUNCTION require_commission_correction_execution_step_commit() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM commission_correction_executions WHERE id=NEW.execution_id AND version>=NEW.version) OR
  NEW.operation='apply' AND NOT EXISTS(SELECT 1 FROM commission_correction_execution_targets WHERE id=NEW.target_id AND execution_id=NEW.execution_id AND state='applied' AND execution_version=NEW.version-1) THEN RAISE EXCEPTION 'orphan correction execution step'; END IF; RETURN NULL;
END $$;

CREATE FUNCTION require_commission_correction_execution_target_commit() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE t commission_correction_execution_targets;
BEGIN
 SELECT * INTO t FROM commission_correction_execution_targets WHERE id=NEW.id;
 IF t.state<>'applied' OR NOT EXISTS(SELECT 1 FROM commission_correction_executions x JOIN commission_correction_execution_steps s ON s.execution_id=x.id AND s.version=t.execution_version+1 AND s.operation='apply' AND s.target_id=t.id
  WHERE x.id=t.execution_id AND x.version>t.execution_version) OR NOT EXISTS(SELECT 1 FROM commission_correction_balance_heads h WHERE h.cycle_id=t.cycle_id AND h.agent_id=t.agent_id AND h.version>=t.financial_version) THEN RAISE EXCEPTION 'orphan/partial correction financial target'; END IF;
 RETURN NULL;
END $$;

CREATE FUNCTION require_commission_correction_ledger_commit() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM commission_correction_execution_targets WHERE brand_id=NEW.brand_id AND id=NEW.reference_id AND state='applied' AND ledger_entry_id=NEW.id) THEN RAISE EXCEPTION 'orphan correction financial ledger'; END IF; RETURN NULL;
END $$;

CREATE FUNCTION require_commission_correction_notification_commit() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF NEW.state='applied' AND NEW.delta_points<>0 AND NOT EXISTS(
  SELECT 1 FROM outbox_events e WHERE e.brand_id=NEW.brand_id AND e.aggregate_id=NEW.id AND e.event_type='commission.corrected'
   AND valid_commission_correction_notification_event(e.brand_id,e.event_type,e.aggregate_id,e.payload)) THEN
  RAISE EXCEPTION 'new correction posting requires matching immutable notification event';
 END IF;
 RETURN NULL;
END $$;

CREATE FUNCTION require_commission_correction_plan_commit() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE p commission_correction_plans;
BEGIN
 SELECT * INTO p FROM commission_correction_plans WHERE id=NEW.id;
 IF NOT EXISTS(SELECT 1 FROM commission_correction_plan_steps s WHERE s.plan_id=p.id AND s.version=1 AND s.operation='create' AND s.audit_log_id=p.creation_audit_log_id) OR
  NOT EXISTS(SELECT 1 FROM commission_correction_plan_steps s WHERE s.plan_id=p.id AND s.version=p.version AND s.audit_log_id=p.last_audit_log_id) OR
  p.planned_count<>(SELECT count(*) FROM commission_correction_plan_targets WHERE plan_id=p.id) THEN RAISE EXCEPTION 'orphan or partial correction plan cannot commit'; END IF;
 RETURN NULL;
END $$;

CREATE FUNCTION require_commission_correction_plan_step_commit() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM commission_correction_plans p WHERE p.id=NEW.plan_id AND p.version>=NEW.version) THEN RAISE EXCEPTION 'orphan correction step'; END IF; RETURN NULL;
END $$;

CREATE FUNCTION require_commission_correction_plan_target_commit() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM commission_correction_plans p WHERE p.id=NEW.plan_id AND p.version>=NEW.plan_version AND p.planned_count=(SELECT count(*) FROM commission_correction_plan_targets WHERE plan_id=p.id)) THEN RAISE EXCEPTION 'orphan correction target'; END IF; RETURN NULL;
END $$;

CREATE FUNCTION require_commission_notification_commit() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF TG_TABLE_NAME='commission_payment_targets' THEN
  IF NEW.state='paid' AND NEW.points>0 AND NOT EXISTS(SELECT 1 FROM outbox_events e WHERE e.brand_id=NEW.brand_id AND e.event_type='commission.paid' AND e.aggregate_id=NEW.id AND
   valid_commission_notification_event(e.brand_id,e.event_type,e.aggregate_id,e.payload)) THEN RAISE EXCEPTION 'positive paid commission target requires its immutable notification event'; END IF;
 ELSIF OLD.ledger_entry_id IS NULL AND NEW.ledger_entry_id IS NOT NULL THEN
  IF NOT EXISTS(SELECT 1 FROM outbox_events e WHERE e.brand_id=NEW.brand_id AND e.event_type='commission.adjusted' AND e.aggregate_id=NEW.id AND
   valid_commission_notification_event(e.brand_id,e.event_type,e.aggregate_id,e.payload)) THEN RAISE EXCEPTION 'completed commission adjustment requires its immutable notification event'; END IF;
 END IF;
 RETURN NULL;
END $$;

CREATE FUNCTION require_commission_payment_credit_commit() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF NEW.entry_type='commission' OR NEW.reference_type='commission_payment_target' THEN
  IF NOT EXISTS(SELECT 1 FROM commission_payment_targets t WHERE t.brand_id=NEW.brand_id AND t.id=NEW.reference_id AND t.state='paid' AND t.ledger_entry_id=NEW.id) THEN RAISE EXCEPTION 'orphan commission credit'; END IF;
 END IF; RETURN NULL;
END $$;

CREATE FUNCTION require_commission_payment_target_commit() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF NEW.state='paid' AND NOT EXISTS(SELECT 1 FROM commission_payments p JOIN audit_logs a ON a.brand_id=p.brand_id AND a.resource_id=p.id
  WHERE p.brand_id=NEW.brand_id AND p.id=NEW.payment_id AND p.version>NEW.payment_version AND a.action='commission.payment.post' AND a.resource_type='commission_payment' AND
   a.actor_type='system' AND a.actor_id IS NULL AND a.before_json->>'version'=NEW.payment_version::text AND a.after_json->>'version'=(NEW.payment_version+1)::text AND
   a.after_json->>'target_id'=NEW.id::text) THEN RAISE EXCEPTION 'paid target needs matching committed payment step'; END IF;
 IF NEW.state='paid' AND NEW.points>0 AND NOT EXISTS(SELECT 1 FROM point_ledger_entries l WHERE l.id=NEW.ledger_entry_id AND l.brand_id=NEW.brand_id AND l.member_id=NEW.member_id AND
  l.entry_type='commission' AND l.reference_type='commission_payment_target' AND l.reference_id=NEW.id AND l.operation_key='commission-payment:'||NEW.id::text) THEN RAISE EXCEPTION 'paid commission target missing credit'; END IF; RETURN NULL;
END $$;

CREATE FUNCTION require_commission_policy_revision() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM commission_policy_revisions WHERE brand_id=NEW.brand_id AND version=NEW.version AND config=NEW.config) THEN
  RAISE EXCEPTION 'commission current version needs revision'; END IF;
 RETURN NULL;
END $$;

CREATE FUNCTION require_commission_run_commit() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM commission_cycles c WHERE c.brand_id=NEW.brand_id AND c.id=NEW.cycle_id AND c.current_run_id=NEW.id) THEN RAISE EXCEPTION 'orphan commission run'; END IF;
 RETURN NULL;
END $$;

CREATE FUNCTION require_commission_step_commit() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM commission_cycles WHERE id=NEW.cycle_id AND version>=NEW.version) THEN RAISE EXCEPTION 'orphan commission step'; END IF;
 RETURN NULL;
END $$;

CREATE FUNCTION require_compliance_history() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM compliance_policy_revisions WHERE brand_id=NEW.brand_id AND version=NEW.version AND config=NEW.config) THEN RAISE EXCEPTION 'compliance policy requires immutable history'; END IF;
 RETURN NULL;
END $$;

CREATE FUNCTION require_draw_notification_publication_commit() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF EXISTS(SELECT DISTINCT o.brand_member_id FROM bet_orders o WHERE o.brand_id=NEW.brand_id AND o.period_id=NEW.period_id
  EXCEPT SELECT r.member_id FROM draw_notification_recipients r WHERE r.brand_id=NEW.brand_id AND r.draw_result_id=NEW.draw_result_id)
  OR EXISTS(SELECT 1 FROM draw_notification_recipients r WHERE r.brand_id=NEW.brand_id AND r.draw_result_id=NEW.draw_result_id
   AND NOT EXISTS(SELECT 1 FROM outbox_events e WHERE e.brand_id=r.brand_id AND e.aggregate_id=r.id AND e.event_type=NEW.event_type
    AND valid_draw_notification_event(e.brand_id,e.event_type,e.aggregate_id,e.payload))) THEN
  RAISE EXCEPTION 'publication requires the complete unique audience and every matching event';
 END IF;
 RETURN NULL;
END $$;

CREATE FUNCTION require_join_code_revision() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE c join_codes;k uuid;
BEGIN
 IF TG_TABLE_NAME='join_codes' THEN k:=NEW.id;ELSE k:=NEW.code_id;END IF;
 SELECT * INTO c FROM join_codes WHERE id=k;
 IF NOT FOUND OR NOT EXISTS(SELECT 1 FROM join_code_revisions h WHERE h.brand_id=c.brand_id AND h.code_id=c.id AND h.version=c.version AND ROW(h.status,h.starts_at,h.expires_at) IS NOT DISTINCT FROM ROW(c.status,c.starts_at,c.expires_at)) THEN RAISE EXCEPTION 'join-code current version requires audited history';END IF;
 RETURN NULL;
END $$;

CREATE FUNCTION require_notification_template_history() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM notification_template_revisions WHERE brand_id=NEW.brand_id AND template_key=NEW.template_key AND version=NEW.version AND content=NEW.content) THEN RAISE EXCEPTION 'template requires immutable history'; END IF;
 RETURN NULL;
END $$;

CREATE FUNCTION require_report_archive_automatic_link() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN IF NEW.automatic_task_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM report_archive_automatic_tasks j WHERE j.id=NEW.automatic_task_id AND j.brand_id=NEW.brand_id AND j.state='completed' AND j.archive_id=NEW.id) THEN RAISE EXCEPTION 'automatic archive needs atomic completed task'; END IF; RETURN NULL; END $$;

CREATE FUNCTION require_reward_action_commit() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE a reward_order_actions; o reward_orders;
BEGIN
 SELECT * INTO a FROM reward_order_actions WHERE id=NEW.id;
 SELECT * INTO o FROM reward_orders WHERE brand_id=a.brand_id AND id=a.order_id;
 IF o.id IS NULL OR a.version>o.version OR
  (a.state_after='revocation_pending' AND a.ledger_entry_id IS NOT NULL) OR
  (a.state_after<>'revocation_pending' AND (a.ledger_entry_id IS NULL OR NOT EXISTS(
   SELECT 1 FROM point_ledger_entries l WHERE l.id=a.ledger_entry_id AND l.brand_id=a.brand_id AND l.member_id=o.member_id AND
   l.reference_type='reward_order' AND l.reference_id=o.id AND l.actor_type='admin' AND l.actor_id=a.actor_id AND
   l.entry_type=CASE a.operation WHEN 'grant' THEN 'reward_grant' ELSE 'reward_reversal' END))) THEN RAISE EXCEPTION 'orphan reward action cannot commit'; END IF;
 RETURN NULL;
END $$;

CREATE FUNCTION require_reward_ledger_commit() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF NEW.entry_type IN('reward_grant','reward_reversal') OR NEW.reference_type='reward_order' THEN
  IF NOT EXISTS(SELECT 1 FROM reward_order_actions WHERE brand_id=NEW.brand_id AND order_id=NEW.reference_id AND ledger_entry_id=NEW.id) THEN
   RAISE EXCEPTION 'orphan reward ledger cannot commit'; END IF;
 END IF; RETURN NULL;
END $$;

CREATE FUNCTION require_reward_notification_commit() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM outbox_events e WHERE e.brand_id=NEW.brand_id AND e.aggregate_id=NEW.id AND e.event_type='reward.order.'||NEW.state_after AND
  valid_reward_notification_event(e.brand_id,e.event_type,e.aggregate_id,e.payload)) THEN
  RAISE EXCEPTION 'new reward action requires matching immutable notification event';
 END IF;
 RETURN NULL;
END $$;

CREATE FUNCTION require_reward_order_commit() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE o reward_orders; a reward_order_actions;
BEGIN
 SELECT * INTO o FROM reward_orders WHERE id=NEW.id;
 SELECT * INTO a FROM reward_order_actions WHERE brand_id=o.brand_id AND order_id=o.id AND version=o.version;
 IF o.grant_ledger_entry_id IS NULL OR a.id IS NULL OR a.state_after<>o.state OR a.audit_log_id<>o.last_audit_log_id OR
  NOT EXISTS(SELECT 1 FROM reward_order_actions g JOIN point_ledger_entries l ON l.id=g.ledger_entry_id WHERE g.brand_id=o.brand_id AND g.order_id=o.id AND g.operation='grant' AND g.version=1 AND
   g.ledger_entry_id=o.grant_ledger_entry_id AND l.brand_id=o.brand_id AND l.member_id=o.member_id AND l.reference_id=o.id AND l.reference_type='reward_order' AND l.entry_type='reward_grant') OR
  (o.state='revoked' AND a.ledger_entry_id IS DISTINCT FROM o.revoke_ledger_entry_id) OR
  (o.state='revocation_pending' AND a.ledger_entry_id IS NOT NULL) THEN RAISE EXCEPTION 'orphan or incomplete reward order cannot commit'; END IF;
 RETURN NULL;
END $$;

CREATE FUNCTION require_withdrawal_transition_outbox() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM outbox_events WHERE brand_id=NEW.brand_id AND aggregate_id=NEW.order_id AND event_type='withdrawal.order.'||NEW.to_state AND payload->>'version'=NEW.version::text AND payload->>'audit_log_id'=NEW.audit_log_id::text) THEN
  RAISE EXCEPTION 'withdrawal transition requires transactional outbox evidence';
 END IF;
 RETURN NULL;
END $$;

CREATE FUNCTION valid_agent_config(v jsonb, brand_scope boolean) RETURNS boolean
    LANGUAGE plpgsql IMMUTABLE
    AS $_$
BEGIN
 IF jsonb_typeof(v)<>'object' THEN RETURN false; END IF;
 IF brand_scope THEN
  RETURN (SELECT count(*)=5 FROM jsonb_object_keys(v)) AND v ?& ARRAY['enabled','max_depth','ratio_cap','mode','cycle']
   AND jsonb_typeof(v->'enabled')='boolean' AND jsonb_typeof(v->'max_depth')='number'
   AND v->>'max_depth' ~ '^[1-9][0-9]?$' AND (v->>'max_depth')::int<=32
   AND jsonb_typeof(v->'ratio_cap')='string' AND valid_agent_ratio(v->>'ratio_cap')
   AND v->>'mode' IN ('loss','turnover') AND v->>'cycle' IN ('weekly','monthly');
 END IF;
 RETURN (SELECT count(*)=4 FROM jsonb_object_keys(v)) AND v ?& ARRAY['ratio','mode','status','can_create_children']
  AND jsonb_typeof(v->'ratio')='string' AND valid_agent_ratio(v->>'ratio')
  AND (v->'mode'='null'::jsonb OR v->>'mode' IN ('loss','turnover'))
  AND v->>'status' IN ('active','disabled') AND jsonb_typeof(v->'can_create_children')='boolean';
EXCEPTION WHEN OTHERS THEN RETURN false;
END $_$;

CREATE FUNCTION valid_agent_ratio(v text) RETURNS boolean
    LANGUAGE sql IMMUTABLE
    AS $_$
 SELECT coalesce(v ~ '^(0|1|0\.[0-9]{0,5}[1-9])$',false)
$_$;

CREATE FUNCTION valid_bet_withdrawal_snapshot(v jsonb) RETURNS boolean
    LANGUAGE plpgsql IMMUTABLE
    AS $_$
DECLARE
 key_count integer;
 field text;
 version_text text;
 id_text text;
BEGIN
 IF jsonb_typeof(v)<>'object' THEN RETURN false; END IF;
 SELECT count(*) INTO key_count FROM jsonb_object_keys(v);
 IF key_count<>7 OR NOT v ?& ARRAY[
  'schema_version','multiple','source','brand_version','game_version',
  'brand_revision_id','game_revision_id'
 ] THEN RETURN false; END IF;
 IF jsonb_typeof(v->'schema_version')<>'number' OR v->'schema_version'<>'1'::jsonb OR
    NOT valid_positive_withdrawal_multiple(v->'multiple') OR
    jsonb_typeof(v->'source')<>'string' OR v->>'source' NOT IN ('brand','game') THEN
  RETURN false;
 END IF;
 FOREACH field IN ARRAY ARRAY['brand_version','game_version'] LOOP
  IF jsonb_typeof(v->field)<>'string' THEN RETURN false; END IF;
  version_text:=v->>field;
  IF version_text !~ '^[1-9][0-9]*$' OR version_text::numeric>9223372036854775807 THEN
   RETURN false;
  END IF;
 END LOOP;
 FOREACH field IN ARRAY ARRAY['brand_revision_id','game_revision_id'] LOOP
  IF jsonb_typeof(v->field)<>'string' THEN RETURN false; END IF;
  id_text:=v->>field;
  IF id_text !~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$' OR
     id_text::uuid::text<>id_text THEN
   RETURN false;
  END IF;
 END LOOP;
 RETURN true;
EXCEPTION WHEN OTHERS THEN RETURN false;
END $_$;

CREATE FUNCTION valid_commission_correction_notification_event(b uuid, k text, aggregate uuid, payload jsonb) RETURNS boolean
    LANGUAGE plpgsql STABLE
    AS $_$
DECLARE amount bigint; valid boolean;
BEGIN
 IF b IS NULL OR aggregate IS NULL OR k IS DISTINCT FROM 'commission.corrected' OR
  jsonb_typeof(payload) IS DISTINCT FROM 'object' OR (SELECT count(*) FROM jsonb_object_keys(payload))<>5 OR
  NOT(payload ?& ARRAY['member_id','resource_id','points','ledger_entry_id','target_id']) OR
  jsonb_typeof(payload->'member_id') IS DISTINCT FROM 'string' OR jsonb_typeof(payload->'resource_id') IS DISTINCT FROM 'string' OR
  jsonb_typeof(payload->'points') IS DISTINCT FROM 'string' OR jsonb_typeof(payload->'ledger_entry_id') IS DISTINCT FROM 'string' OR
  jsonb_typeof(payload->'target_id') IS DISTINCT FROM 'string' OR
  payload->>'points' !~ '^-?[1-9][0-9]*$' OR length(replace(payload->>'points','-',''))>19 THEN RETURN false; END IF;
 amount:=(payload->>'points')::bigint;
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

CREATE FUNCTION valid_commission_fraction(n text, d text) RETURNS boolean
    LANGUAGE plpgsql IMMUTABLE
    AS $_$
BEGIN
 IF n IS NULL OR d IS NULL OR length(n)>100 OR n !~ '^(0|[1-9][0-9]*)$' OR d !~ '^[1-9][0-9]{0,6}$' THEN RETURN false; END IF;
 RETURN d::numeric<=1000000 AND mod(1000000,d::numeric)=0 AND gcd(n::numeric,d::numeric)=1;
EXCEPTION WHEN OTHERS THEN RETURN false;
END
$_$;

CREATE FUNCTION valid_commission_notification_event(b uuid, k text, aggregate uuid, payload jsonb) RETURNS boolean
    LANGUAGE plpgsql STABLE
    AS $_$
DECLARE v_member_id uuid; v_resource_id uuid; v_target_id uuid; v_ledger_id uuid; v_amount bigint; target commission_payment_targets; adjustment commission_adjustments; valid boolean;
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
 IF (payload->>'points') !~ '^-?[1-9][0-9]*$' OR length(replace(payload->>'points','-',''))>19 THEN RETURN false; END IF;
 v_amount:=(payload->>'points')::bigint;
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

CREATE FUNCTION valid_commission_policy(v jsonb) RETURNS boolean
    LANGUAGE plpgsql STABLE
    AS $_$
DECLARE c jsonb;
BEGIN
 IF jsonb_typeof(v)<>'object' OR (SELECT count(*) FROM jsonb_object_keys(v))<>3 OR
  NOT v ?& ARRAY['enabled','calendar','payout_mode'] OR jsonb_typeof(v->'enabled')<>'boolean' OR
  jsonb_typeof(v->'payout_mode')<>'string' OR v->>'payout_mode' NOT IN ('manual','automatic') THEN RETURN false; END IF;
 c:=v->'calendar';
 IF c='null'::jsonb THEN RETURN v->'enabled'='false'::jsonb; END IF;
 IF jsonb_typeof(c)<>'object' OR (SELECT count(*) FROM jsonb_object_keys(c))<>6 OR
  NOT c ?& ARRAY['timezone','cycle','boundary_time','weekday','month_day','short_month'] OR
  jsonb_typeof(c->'timezone')<>'string' OR c->>'timezone'='Local' OR
  NOT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=c->>'timezone') OR
  jsonb_typeof(c->'boundary_time')<>'string' OR c->>'boundary_time' !~ '^([01][0-9]|2[0-3]):[0-5][0-9]:[0-5][0-9]$' OR
  jsonb_typeof(c->'cycle')<>'string' OR jsonb_typeof(c->'short_month')<>'string' THEN RETURN false; END IF;
 IF c->>'cycle'='weekly' THEN
  RETURN c->'month_day'='null'::jsonb AND c->>'short_month'='' AND
   jsonb_typeof(c->'weekday')='number' AND c->>'weekday' ~ '^[0-6]$';
 ELSIF c->>'cycle'='monthly' THEN
  RETURN c->'weekday'='null'::jsonb AND c->>'short_month' IN ('last_day','skip') AND
   jsonb_typeof(c->'month_day')='number' AND c->>'month_day' ~ '^[1-9][0-9]?$' AND (c->>'month_day')::int<=31;
 END IF;
 RETURN false;
EXCEPTION WHEN OTHERS THEN RETURN false;
END $_$;

CREATE FUNCTION valid_compliance_config(v jsonb) RETURNS boolean
    LANGUAGE plpgsql IMMUTABLE
    AS $_$
DECLARE countries jsonb; n integer;
BEGIN
 IF jsonb_typeof(v)<>'object' OR (SELECT count(*) FROM jsonb_object_keys(v))<>5
 OR NOT(v ?& ARRAY['age_enabled','minimum_age','region_enabled','allowed_countries','identity_enabled'])
 OR jsonb_typeof(v->'age_enabled')<>'boolean' OR jsonb_typeof(v->'region_enabled')<>'boolean'
 OR jsonb_typeof(v->'identity_enabled')<>'boolean' OR jsonb_typeof(v->'allowed_countries')<>'array' THEN RETURN false; END IF;
 IF v->'minimum_age'<>'null'::jsonb AND (jsonb_typeof(v->'minimum_age')<>'number' OR v->>'minimum_age' !~ '^[0-9]+$' OR (v->>'minimum_age')::integer NOT BETWEEN 18 AND 120) THEN RETURN false; END IF;
 IF v->'age_enabled'='true'::jsonb AND v->'minimum_age'='null'::jsonb THEN RETURN false; END IF;
 n:=jsonb_array_length(v->'allowed_countries');
 IF n>250 OR (v->'region_enabled'='true'::jsonb AND n=0)
 OR EXISTS(SELECT 1 FROM jsonb_array_elements(v->'allowed_countries') e WHERE jsonb_typeof(e)<>'string' OR e#>>'{}' !~ '^[A-Z]{2}$') THEN RETURN false; END IF;
 SELECT coalesce(jsonb_agg(x ORDER BY x COLLATE "C"),'[]'::jsonb) INTO countries FROM (SELECT DISTINCT jsonb_array_elements_text(v->'allowed_countries') x) q;
 RETURN countries=v->'allowed_countries';
EXCEPTION WHEN OTHERS THEN RETURN false;
END $_$;

CREATE FUNCTION valid_draw_notification_event(b uuid, k text, aggregate uuid, payload jsonb) RETURNS boolean
    LANGUAGE plpgsql STABLE
    AS $$
BEGIN
 IF b IS NULL OR aggregate IS NULL OR k NOT IN('draw.result.published','draw.result.corrected')
  OR jsonb_typeof(payload) IS DISTINCT FROM 'object' OR (SELECT count(*) FROM jsonb_object_keys(payload))<>4
  OR NOT(payload ?& ARRAY['member_id','resource_id','publication_id','recipient_id']) THEN RETURN false; END IF;
 RETURN EXISTS(SELECT 1 FROM draw_notification_recipients r
  JOIN draw_notification_publications p ON p.brand_id=r.brand_id AND p.draw_result_id=r.draw_result_id
  WHERE r.brand_id=b AND r.id=aggregate AND p.event_type=k
   AND payload=jsonb_build_object('member_id',r.member_id::text,'resource_id',p.draw_result_id::text,
    'publication_id',p.draw_result_id::text,'recipient_id',r.id::text));
EXCEPTION WHEN OTHERS THEN RETURN false;
END $$;

CREATE FUNCTION valid_notification_template_content(k text, v jsonb) RETURNS boolean
    LANGUAGE plpgsql IMMUTABLE
    AS $$
DECLARE lang text; c jsonb; title text; body text; rest text;
BEGIN
 IF NOT(notification_template_defaults() ? k) OR jsonb_typeof(v)<>'object'
 OR (SELECT count(*) FROM jsonb_object_keys(v))<>2 OR NOT(v ?& ARRAY['en','zh-CN']) THEN RETURN false; END IF;
 FOREACH lang IN ARRAY ARRAY['en','zh-CN'] LOOP
  c:=v->lang;
  IF jsonb_typeof(c)<>'object' OR (SELECT count(*) FROM jsonb_object_keys(c))<>2
  OR NOT(c ?& ARRAY['title','body']) OR jsonb_typeof(c->'title')<>'string' OR jsonb_typeof(c->'body')<>'string' THEN RETURN false; END IF;
  title:=c->>'title'; body:=c->>'body';
  IF title<>btrim(title) OR body<>btrim(body) OR octet_length(title) NOT BETWEEN 1 AND 120 OR octet_length(body) NOT BETWEEN 1 AND 1200
  OR title ~ '[[:cntrl:]<>]' OR replace(replace(body,chr(10),''),chr(9),'') ~ '[[:cntrl:]<>]'
  OR (title||body) ~* '(https?:|javascript:|data:|www[.])' THEN RETURN false; END IF;
  rest:=replace(replace(title||body,'{points}',''),'{resource_id}','');
  IF rest ~ '[{}]' THEN RETURN false; END IF;
  IF k IN('draw.result.published','draw.result.corrected') THEN
   IF position('{points}' IN title||body)>0 OR position('{resource_id}' IN body)=0 THEN RETURN false; END IF;
  ELSIF (k='member.joined' AND position('{points}' IN title||body)>0)
  OR (k<>'member.joined' AND position('{points}' IN body)=0) THEN RETURN false;
  END IF;
 END LOOP;
 RETURN true;
EXCEPTION WHEN OTHERS THEN RETURN false;
END $$;

CREATE FUNCTION valid_point_snapshot(v jsonb, signed_amounts boolean) RETURNS boolean
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
    v->src->>st !~ (CASE WHEN signed_amounts THEN '^(0|-?[1-9][0-9]*)$' ELSE '^(0|[1-9][0-9]*)$' END) THEN RETURN false; END IF;
   n:=(v->src->>st)::numeric;
   IF n < -9223372036854775808 OR n > 9223372036854775807 THEN RETURN false; END IF;
   total:=total+n;
  END LOOP;
 END LOOP;
 RETURN signed_amounts OR total<=9223372036854775807;
EXCEPTION WHEN OTHERS THEN RETURN false;
END $_$;

CREATE FUNCTION valid_positive_withdrawal_multiple(v jsonb) RETURNS boolean
    LANGUAGE plpgsql IMMUTABLE
    AS $$
BEGIN
 RETURN coalesce(valid_withdrawal_multiple(v) AND (v#>>'{}')::numeric>0,false);
EXCEPTION WHEN OTHERS THEN RETURN false;
END $$;

CREATE FUNCTION valid_reward_notification_event(b uuid, k text, aggregate uuid, payload jsonb) RETURNS boolean
    LANGUAGE plpgsql STABLE
    AS $_$
DECLARE a reward_order_actions; o reward_orders; log audit_logs; l point_ledger_entries; amount bigint; expected jsonb;
BEGIN
 IF b IS NULL OR aggregate IS NULL OR k IS NULL OR k NOT IN('reward.order.granted','reward.order.revocation_pending','reward.order.revoked') OR
  jsonb_typeof(payload) IS DISTINCT FROM 'object' OR (SELECT count(*) FROM jsonb_object_keys(payload))<>6 OR
  NOT(payload ?& ARRAY['member_id','resource_id','points','action_id','version','audit_log_id']) OR
  jsonb_typeof(payload->'member_id') IS DISTINCT FROM 'string' OR jsonb_typeof(payload->'resource_id') IS DISTINCT FROM 'string' OR
  jsonb_typeof(payload->'points') IS DISTINCT FROM 'string' OR jsonb_typeof(payload->'action_id') IS DISTINCT FROM 'string' OR
  jsonb_typeof(payload->'version') IS DISTINCT FROM 'number' OR jsonb_typeof(payload->'audit_log_id') IS DISTINCT FROM 'string' THEN RETURN false; END IF;
 IF (payload->>'points') !~ '^[1-9][0-9]*$' OR length(payload->>'points')>19 OR
  (payload->>'version') !~ '^[1-9][0-9]*$' OR (payload->>'version')::numeric>9007199254740991 THEN RETURN false; END IF;
 amount:=(payload->>'points')::bigint;
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
 IF a.state_after='revocation_pending' THEN
  RETURN a.operation IN('revoke','retry') AND a.version>1 AND a.ledger_entry_id IS NULL;
 END IF;
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
 RETURN a.state_after='revoked' AND a.operation IN('revoke','retry') AND a.version>1 AND a.ledger_entry_id=o.revoke_ledger_entry_id AND
  l.entry_type='reward_reversal' AND l.operation_key='reward-revoke:'||o.id::text AND l.reversal_of=o.grant_ledger_entry_id AND l.delta_snapshot=expected;
EXCEPTION WHEN OTHERS THEN RETURN false;
END $_$;

CREATE FUNCTION valid_withdrawal_multiple(v jsonb) RETURNS boolean
    LANGUAGE plpgsql IMMUTABLE
    AS $_$
BEGIN
 RETURN jsonb_typeof(v)='string' AND (v#>>'{}') ~ '^(0|[1-9][0-9]{0,6})([.][0-9]{0,5}[1-9])?$' AND (v#>>'{}')::numeric<=1000000;
EXCEPTION WHEN OTHERS THEN RETURN false;
END $_$;

CREATE FUNCTION valid_withdrawal_policy(v jsonb, game_scope boolean) RETURNS boolean
    LANGUAGE plpgsql IMMUTABLE
    AS $_$
DECLARE key_count integer; sources integer;
BEGIN
 IF jsonb_typeof(v)<>'object' THEN RETURN false; END IF;
 SELECT count(*) INTO key_count FROM jsonb_object_keys(v);
 IF game_scope THEN RETURN key_count=1 AND v ? 'turnover_multiple' AND (v->'turnover_multiple'='null'::jsonb OR valid_withdrawal_multiple(v->'turnover_multiple')); END IF;
 IF key_count<>6 OR NOT v ?& ARRAY['enabled','min_points','max_points','allowed_sources','review_mode','turnover_multiple'] OR jsonb_typeof(v->'enabled')<>'boolean' OR jsonb_typeof(v->'min_points')<>'string' OR NOT (v->>'min_points') ~ '^[1-9][0-9]{0,18}$' OR (v->>'min_points')::numeric>9223372036854775807 OR jsonb_typeof(v->'allowed_sources')<>'array' OR jsonb_typeof(v->'review_mode')<>'string' OR v->>'review_mode' NOT IN ('manual','automatic') OR NOT valid_withdrawal_multiple(v->'turnover_multiple') THEN RETURN false; END IF;
 IF v->'max_points'<>'null'::jsonb AND (jsonb_typeof(v->'max_points')<>'string' OR NOT (v->>'max_points') ~ '^[1-9][0-9]{0,18}$' OR (v->>'max_points')::numeric>9223372036854775807 OR (v->>'max_points')::numeric<(v->>'min_points')::numeric) THEN RETURN false; END IF;
 sources:=jsonb_array_length(v->'allowed_sources');
 IF sources<1 OR sources>4 OR EXISTS(SELECT 1 FROM jsonb_array_elements(v->'allowed_sources') s WHERE jsonb_typeof(s)<>'string' OR s#>>'{}' NOT IN ('recharge','winning','gift','commission')) OR (SELECT count(DISTINCT s) FROM jsonb_array_elements(v->'allowed_sources') s)<>sources THEN RETURN false; END IF;
 RETURN true;
EXCEPTION WHEN OTHERS THEN RETURN false;
END $_$;

CREATE FUNCTION validate_bet_exception_final_state() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM bet_orders WHERE brand_id=NEW.brand_id AND id=NEW.order_id AND version>=NEW.order_version AND status IN ('abnormal','bet_cancelled','judged_cancelled')) THEN RAISE EXCEPTION 'exception evidence must commit with its classification'; END IF;
 RETURN NULL;
END $$;

CREATE FUNCTION validate_bet_judgment_final_state() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM bet_orders WHERE id=NEW.order_id AND brand_id=NEW.brand_id AND version=NEW.order_version AND status='judged_cancelled' AND refund_entry_id IS NOT NULL AND cancel_reason=NEW.reason) THEN RAISE EXCEPTION 'judgment must commit with exact cancelled order and refund'; END IF;
 RETURN NULL;
END $$;

CREATE FUNCTION validate_brand_creation_record() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM brands b WHERE b.id=NEW.brand_id AND b.code=NEW.code AND b.name=NEW.name AND b.default_locale=NEW.default_locale AND b.timezone=NEW.timezone AND b.status=NEW.status AND b.config_version=NEW.version AND b.created_at=NEW.created_at)
 OR NOT EXISTS(SELECT 1 FROM audit_logs a WHERE a.id=NEW.audit_log_id AND a.brand_id=NEW.brand_id AND a.actor_type='admin' AND a.actor_id=NEW.created_by AND a.action='brand.create' AND a.resource_type='brand' AND a.resource_id=NEW.brand_id AND a.reason=NEW.reason AND a.after_json->>'code'=NEW.code AND a.after_json->>'name'=NEW.name AND a.after_json->>'status'=NEW.status AND a.after_json->>'default_locale'=NEW.default_locale AND a.after_json->>'timezone'=NEW.timezone)
 THEN RAISE EXCEPTION 'brand creation requires matching initial brand and audit'; END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION validate_cancellation_progress() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE job_id uuid; job_state text; has_failed boolean;
BEGIN
 IF TG_TABLE_NAME='period_cancellations' THEN job_id:=NEW.id; ELSE job_id:=NEW.cancellation_id; END IF;
 SELECT state INTO job_state FROM period_cancellations WHERE id=job_id;
 SELECT EXISTS(SELECT 1 FROM period_cancellation_targets WHERE cancellation_id=job_id AND state='failed') INTO has_failed;
 IF (job_state='failed')<>has_failed OR (job_state='completed' AND EXISTS(SELECT 1 FROM period_cancellation_targets WHERE cancellation_id=job_id AND state='pending')) THEN RAISE EXCEPTION 'cancellation state must match committed progress'; END IF;
 RETURN NULL;
END $$;

CREATE FUNCTION validate_correction_final_state() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE c draw_corrections; p periods; j settlement_jobs;
BEGIN
 IF TG_TABLE_NAME='draw_corrections' THEN SELECT * INTO c FROM draw_corrections WHERE id=NEW.id; ELSE SELECT * INTO c FROM draw_corrections WHERE id=NEW.correction_id; END IF;
 SELECT * INTO p FROM periods WHERE id=c.period_id;
 IF c.target_count<>(SELECT count(*) FROM draw_correction_targets WHERE correction_id=c.id) OR
 (c.state IN ('resettling','completed') AND EXISTS(SELECT 1 FROM draw_correction_targets WHERE correction_id=c.id AND state IN ('pending','failed'))) OR
 EXISTS(SELECT 1 FROM draw_correction_targets t JOIN bet_orders o ON o.id=t.order_id WHERE t.correction_id=c.id AND ((t.state='reversed' AND (o.version<t.reset_order_version OR correction_reversal_is_applied(t) IS NOT TRUE)) OR (t.state='excluded' AND o.status NOT IN ('abnormal','bet_cancelled','judged_cancelled')))) THEN RAISE EXCEPTION 'correction targets lack committed evidence'; END IF;
 IF c.state<>'completed' THEN
  IF p.current_correction_id IS DISTINCT FROM c.id OR p.status<>'settling' THEN RAISE EXCEPTION 'active correction requires period witness'; END IF;
 END IF;
 IF c.state IN ('reversing','failed') AND (p.draw_result_id<>c.previous_draw_result_id OR p.current_settlement_job_id IS DISTINCT FROM c.previous_job_id OR p.version<>c.period_version) THEN RAISE EXCEPTION 'correction must not publish before reversals finish'; END IF;
 IF c.new_job_id IS NOT NULL THEN
  SELECT * INTO j FROM settlement_jobs WHERE id=c.new_job_id;
  IF j.correction_id IS DISTINCT FROM c.id OR j.previous_job_id IS DISTINCT FROM c.previous_job_id OR j.draw_result_id<>c.draw_result_id OR j.policy_version<>c.policy_version OR j.mode<>c.mode OR (c.state='completed')<>(j.state='completed') THEN RAISE EXCEPTION 'correction requires matching resettlement generation'; END IF;
 END IF;
 RETURN NULL; END $$;

CREATE FUNCTION validate_period_cancellation_start() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM periods WHERE id=NEW.period_id AND brand_id=NEW.brand_id AND status=NEW.mode AND version=NEW.period_version AND draw_result_id IS NOT DISTINCT FROM NEW.draw_result_id) OR (SELECT count(*) FROM period_cancellation_targets WHERE cancellation_id=NEW.id)<>NEW.target_count OR EXISTS(SELECT 1 FROM bet_orders o WHERE o.brand_id=NEW.brand_id AND o.period_id=NEW.period_id AND o.status IN ('placed','abnormal') AND NOT EXISTS(SELECT 1 FROM period_cancellation_targets t WHERE t.cancellation_id=NEW.id AND t.order_id=o.id)) THEN RAISE EXCEPTION 'cancellation must commit with period and all targets'; END IF;
 RETURN NULL;
END $$;

CREATE FUNCTION validate_period_settlement_witness() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE p periods; j settlement_jobs; c draw_corrections;
BEGIN
 SELECT * INTO p FROM periods WHERE id=NEW.id;
 IF p.status IN ('settling','settled') THEN
  SELECT * INTO c FROM draw_corrections WHERE id=p.current_correction_id AND state IN ('reversing','failed');
  IF c.id IS NOT NULL THEN
   IF p.status<>'settling' OR p.draw_result_id<>c.previous_draw_result_id OR p.version<>c.period_version OR p.current_settlement_job_id IS DISTINCT FROM c.previous_job_id THEN RAISE EXCEPTION 'reversing correction period witness mismatch'; END IF;
  ELSE
   SELECT * INTO j FROM settlement_jobs WHERE id=p.current_settlement_job_id;
   IF j.id IS NULL OR p.draw_result_id IS DISTINCT FROM j.draw_result_id OR (p.status='settled')<>(j.state='completed') OR p.version<>j.period_version+(CASE WHEN p.status='settled' THEN 1 ELSE 0 END) THEN RAISE EXCEPTION 'period settlement requires current durable generation'; END IF;
  END IF;
 END IF; RETURN NULL; END $$;

CREATE FUNCTION validate_point_ledger_projection() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE a point_accounts; tip point_ledger_entries; actual jsonb;
BEGIN
 SELECT * INTO a FROM point_accounts WHERE brand_id=NEW.brand_id AND id=NEW.account_id;
 SELECT * INTO tip FROM point_ledger_entries WHERE brand_id=NEW.brand_id AND account_id=NEW.account_id ORDER BY version DESC LIMIT 1;
 SELECT jsonb_object_agg(source,states) INTO actual FROM (
  SELECT source,jsonb_object_agg(state,points::text) states FROM point_buckets
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

CREATE FUNCTION validate_point_reconciliation_job() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE j point_reconciliation_jobs; n bigint;
BEGIN
 SELECT * INTO j FROM point_reconciliation_jobs WHERE id=NEW.id;
 IF j.version=1 OR j.state='completed' THEN
  SELECT count(*) INTO n FROM point_reconciliation_targets WHERE job_id=j.id;
  IF n<>j.target_count THEN RAISE EXCEPTION 'reconciliation target scope differs'; END IF;
 END IF;
 IF j.state='completed' AND EXISTS(SELECT 1 FROM point_reconciliation_targets WHERE job_id=j.id AND state<>'checked') OR j.state IN('pending','running') AND EXISTS(SELECT 1 FROM point_reconciliation_targets WHERE job_id=j.id AND state='failed') OR j.state='failed' AND NOT EXISTS(SELECT 1 FROM point_reconciliation_failures f JOIN point_reconciliation_targets t ON t.id=f.target_id WHERE f.id=j.last_failure_id AND t.state='failed' AND t.attempt_count=f.attempt_count) THEN RAISE EXCEPTION 'reconciliation job and targets disagree'; END IF;
 RETURN NULL;
END $$;

CREATE FUNCTION validate_settlement_calculation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM bet_orders o JOIN settlement_jobs j ON j.id=NEW.job_id JOIN periods p ON p.id=j.period_id JOIN draw_results d ON d.id=j.draw_result_id
 WHERE o.id=NEW.order_id AND o.brand_id=NEW.brand_id AND o.status='placed' AND o.version=NEW.order_version AND o.definition_hash=NEW.definition_hash AND j.state='processing' AND p.status='settling' AND p.draw_result_id=j.draw_result_id AND d.result_hash=NEW.draw_hash) OR
 NEW.calculation->>'won' IS DISTINCT FROM NEW.won::text OR NEW.calculation->>'prize_points' IS DISTINCT FROM NEW.prize_points::text THEN RAISE EXCEPTION 'invalid locked settlement calculation'; END IF;
 RETURN NEW; END $$;

CREATE FUNCTION validate_settlement_final_state() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE j settlement_jobs; p periods; current_job boolean;
BEGIN
 IF TG_TABLE_NAME='settlement_jobs' THEN SELECT * INTO j FROM settlement_jobs WHERE id=NEW.id; ELSE SELECT * INTO j FROM settlement_jobs WHERE id=NEW.job_id; END IF;
 SELECT * INTO p FROM periods WHERE id=j.period_id;
 current_job:=p.current_settlement_job_id=j.id AND NOT EXISTS(SELECT 1 FROM draw_corrections c WHERE c.previous_job_id=j.id AND c.state<>'completed');
 IF j.target_count<>(SELECT count(*) FROM settlement_targets WHERE job_id=j.id) OR
 (j.state IN ('awaiting_approval','paying','completed') AND EXISTS(SELECT 1 FROM settlement_targets WHERE job_id=j.id AND state IN ('pending','failed'))) OR
 (j.state='completed' AND EXISTS(SELECT 1 FROM settlement_targets WHERE job_id=j.id AND state NOT IN ('paid','excluded'))) THEN RAISE EXCEPTION 'settlement target counts must match generation'; END IF;
 IF current_job AND (p.draw_result_id IS DISTINCT FROM j.draw_result_id OR p.status NOT IN ('settling','settled') OR (j.state='completed')<>(p.status='settled') OR
 EXISTS(SELECT 1 FROM bet_orders o WHERE o.period_id=j.period_id AND NOT EXISTS(SELECT 1 FROM settlement_targets t WHERE t.job_id=j.id AND t.order_id=o.id)) OR
 EXISTS(SELECT 1 FROM settlement_targets t JOIN bet_orders o ON o.id=t.order_id WHERE t.job_id=j.id AND ((t.state='paid' AND (o.status NOT IN ('won','lost') OR o.settlement_calculation_id IS DISTINCT FROM t.calculation_id OR o.payout_entry_id IS DISTINCT FROM t.payout_entry_id)) OR (t.state='excluded' AND o.status NOT IN ('abnormal','bet_cancelled','judged_cancelled'))))) THEN RAISE EXCEPTION 'current settlement generation and period must commit together'; END IF;
 RETURN NULL; END $$;

CREATE FUNCTION validate_settlement_payout_witness() RETURNS trigger
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
   (l.before_snapshot->src->>bucket) !~ '^(0|[1-9][0-9]*)$' OR (l.after_snapshot->src->>bucket) !~ '^(0|[1-9][0-9]*)$' THEN RAISE EXCEPTION 'invalid payout integer snapshot'; END IF;
   IF (l.before_snapshot->src->>bucket)::numeric+(expected->src->>bucket)::numeric IS DISTINCT FROM (l.after_snapshot->src->>bucket)::numeric OR
   NOT EXISTS(SELECT 1 FROM point_buckets b WHERE b.brand_id=NEW.brand_id AND b.account_id=NEW.account_id AND b.source=src AND b.state=bucket AND b.points::numeric=(l.after_snapshot->src->>bucket)::numeric) THEN RAISE EXCEPTION 'payout snapshot must match applied buckets'; END IF;
  END LOOP;
 END LOOP;
 IF NOT EXISTS(SELECT 1 FROM point_accounts a WHERE a.brand_id=NEW.brand_id AND a.id=NEW.account_id AND a.version=l.version) OR
 NOT EXISTS(SELECT 1 FROM point_ledger_entries prev WHERE prev.brand_id=NEW.brand_id AND prev.account_id=NEW.account_id AND prev.version=l.version-1 AND validated_point_snapshot(prev.after_snapshot)=l.before_snapshot) OR
 NOT EXISTS(SELECT 1 FROM audit_logs a WHERE a.brand_id=NEW.brand_id AND a.actor_type='system' AND a.action='points.prize' AND a.resource_type='point_account' AND a.resource_id=NEW.account_id AND a.before_json=l.before_snapshot AND a.after_json->>'ledger_entry_id'=l.id::text AND a.after_json->'balance'=l.after_snapshot) THEN RAISE EXCEPTION 'payout requires applied ledger tip and audit witness'; END IF;
 RETURN NEW;
END $_$;

CREATE FUNCTION validate_settlement_preview_context() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM bet_orders o JOIN periods p ON p.id=o.period_id JOIN draw_results d ON d.id=p.draw_result_id
 WHERE o.id=NEW.order_id AND o.brand_id=NEW.brand_id AND o.game_id=NEW.game_id AND o.period_id=NEW.period_id
 AND o.version=NEW.order_version AND o.status=NEW.order_status AND o.definition_hash=NEW.definition_hash
 AND p.version=NEW.period_version AND p.status='drawn' AND p.draw_result_id=NEW.draw_result_id AND d.result_hash=NEW.draw_hash AND d.result=NEW.draw) THEN
 RAISE EXCEPTION 'settlement preview requires current locked order and result'; END IF;
 IF NOT EXISTS(SELECT 1 FROM audit_logs a WHERE a.id=NEW.audit_log_id AND a.brand_id=NEW.brand_id AND a.action='settlement.preview'
 AND a.resource_type='settlement_preview' AND a.resource_id=NEW.id AND a.actor_id=NEW.created_by) THEN RAISE EXCEPTION 'settlement preview requires audit witness'; END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION validate_withdrawal_funds() RETURNS trigger
    LANGUAGE plpgsql
    AS $_$
DECLARE o withdrawal_orders; e point_ledger_entries; f point_ledger_entries; latest withdrawal_order_transitions; allocation jsonb; total numeric:=0; expected jsonb:='{}'; item jsonb; src text; n bigint; seen text[]:='{}'; h bigint;
BEGIN
 SELECT * INTO o FROM withdrawal_orders WHERE id=NEW.id;
 SELECT * INTO e FROM point_ledger_entries WHERE id=o.reserve_entry_id;
 IF e.brand_id IS DISTINCT FROM o.brand_id OR e.account_id IS DISTINCT FROM o.account_id OR e.member_id IS DISTINCT FROM o.member_id OR e.entry_type<>'withdrawal_reserve' OR e.reference_type<>'withdrawal' OR e.reference_id IS DISTINCT FROM o.id OR e.version<>o.reserve_version OR e.reversal_of IS NOT NULL OR e.source_allocation IS DISTINCT FROM o.source_allocation THEN RAISE EXCEPTION 'withdrawal reserve lacks exact ledger evidence'; END IF;
 IF jsonb_array_length(o.source_allocation) NOT BETWEEN 1 AND 4 THEN RAISE EXCEPTION 'invalid withdrawal allocation'; END IF;
 FOR item IN SELECT value FROM jsonb_array_elements(o.source_allocation) LOOP
  src:=item->>'source';
  IF jsonb_typeof(item)<>'object' OR (SELECT count(*) FROM jsonb_object_keys(item))<>3 OR NOT(item ?& ARRAY['source','state','points']) OR src NOT IN('recharge','winning','gift','commission') OR src=ANY(seen) OR item->>'state'<>'available' OR jsonb_typeof(item->'points')<>'string' OR item->>'points' !~ '^[1-9][0-9]*$' THEN RAISE EXCEPTION 'invalid withdrawal source allocation'; END IF;
  n:=(item->>'points')::bigint; total:=total+n; seen:=array_append(seen,src);
 END LOOP;
 IF total<>o.points THEN RAISE EXCEPTION 'withdrawal allocation does not sum to amount'; END IF;
 FOREACH src IN ARRAY ARRAY['recharge','winning','gift','commission'] LOOP
  SELECT coalesce(sum((value->>'points')::bigint),0) INTO n FROM jsonb_array_elements(o.source_allocation) WHERE value->>'source'=src;
  expected:=expected||jsonb_build_object(src,jsonb_build_object('available',(-n)::text,'manual_frozen','0','system_frozen','0','withdrawal',n::text));
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

CREATE FUNCTION validate_withdrawal_policy_current() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE selected_game uuid:=NULLIF(to_jsonb(NEW)->>'game_id','')::uuid;
BEGIN
 IF NOT EXISTS(SELECT 1 FROM withdrawal_policy_revisions r WHERE r.brand_id=NEW.brand_id AND r.game_id IS NOT DISTINCT FROM selected_game AND r.version=NEW.version AND r.config=NEW.config) THEN RAISE EXCEPTION 'current policy must commit with matching immutable revision'; END IF;
 RETURN NULL;
END $$;

CREATE FUNCTION validate_withdrawal_policy_revision_final() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE current_version bigint;
BEGIN
 IF NEW.game_id IS NULL THEN SELECT version INTO current_version FROM brand_withdrawal_policies WHERE brand_id=NEW.brand_id;
 ELSE SELECT version INTO current_version FROM game_withdrawal_policies WHERE brand_id=NEW.brand_id AND game_id=NEW.game_id; END IF;
 IF NOT FOUND OR current_version<NEW.version THEN RAISE EXCEPTION 'policy revision cannot commit ahead of current policy'; END IF;
 RETURN NULL;
END $$;

CREATE TABLE admin_account_roles (
    account_id uuid NOT NULL,
    role_id uuid NOT NULL
);

CREATE TABLE admin_accounts (
    id uuid NOT NULL,
    username text NOT NULL,
    password_hash text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    is_super_admin boolean DEFAULT false NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    CONSTRAINT admin_accounts_status_check CHECK ((status = ANY (ARRAY['active'::text, 'disabled'::text]))),
    CONSTRAINT admin_accounts_version_check CHECK ((version > 0))
);

CREATE TABLE admin_brand_scopes (
    account_id uuid NOT NULL,
    brand_id uuid NOT NULL
);

CREATE TABLE agent_config_revisions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    brand_id uuid NOT NULL,
    agent_id uuid,
    version bigint NOT NULL,
    config jsonb NOT NULL,
    actor_type text NOT NULL,
    actor_id uuid,
    reason text NOT NULL,
    audit_log_id uuid,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT agent_config_revisions_actor_type_check CHECK ((actor_type = ANY (ARRAY['system'::text, 'admin'::text, 'user'::text]))),
    CONSTRAINT agent_config_revisions_check CHECK ((valid_agent_config(config, (agent_id IS NULL)) IS TRUE)),
    CONSTRAINT agent_config_revisions_check1 CHECK ((((agent_id IS NULL) AND (version = 1)) = ((actor_type = 'system'::text) AND (actor_id IS NULL) AND (audit_log_id IS NULL)))),
    CONSTRAINT agent_config_revisions_check2 CHECK (((actor_type = 'system'::text) OR ((actor_id IS NOT NULL) AND (audit_log_id IS NOT NULL)))),
    CONSTRAINT agent_config_revisions_reason_check CHECK (((length(TRIM(BOTH FROM reason)) > 0) AND (octet_length(reason) <= 500))),
    CONSTRAINT agent_config_revisions_version_check CHECK ((version > 0))
);

CREATE TABLE agent_nodes (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    member_id uuid NOT NULL,
    parent_id uuid,
    depth integer NOT NULL,
    path uuid[] NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    config jsonb NOT NULL,
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT agent_nodes_check CHECK (((array_ndims(path) = 1) AND (cardinality(path) = depth) AND (path[depth] = id))),
    CONSTRAINT agent_nodes_check1 CHECK (((parent_id IS NULL) = (depth = 1))),
    CONSTRAINT agent_nodes_check2 CHECK ((parent_id IS DISTINCT FROM id)),
    CONSTRAINT agent_nodes_config_check CHECK ((valid_agent_config(config, false) IS TRUE)),
    CONSTRAINT agent_nodes_depth_check CHECK (((depth >= 1) AND (depth <= 32))),
    CONSTRAINT agent_nodes_version_check CHECK ((version > 0))
);

CREATE TABLE audit_logs (
    id uuid NOT NULL,
    brand_id uuid,
    actor_type text NOT NULL,
    actor_id uuid,
    action text NOT NULL,
    resource_type text NOT NULL,
    resource_id uuid,
    before_json jsonb,
    after_json jsonb,
    reason text DEFAULT ''::text NOT NULL,
    request_id text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    ip_address text,
    CONSTRAINT audit_logs_actor_type_check CHECK ((actor_type = ANY (ARRAY['user'::text, 'admin'::text, 'system'::text])))
);

CREATE TABLE auth_challenges (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    kind text NOT NULL,
    proof_hash text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    used_at timestamp with time zone,
    CONSTRAINT auth_challenges_kind_check CHECK ((kind = ANY (ARRAY['captcha'::text, 'telegram'::text])))
);

CREATE TABLE auth_rate_limits (
    bucket_hash text NOT NULL,
    attempt_count integer NOT NULL,
    window_start timestamp with time zone NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    CONSTRAINT auth_rate_limits_attempt_count_check CHECK ((attempt_count > 0))
);

CREATE TABLE bet_order_exceptions (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    order_id uuid NOT NULL,
    order_version bigint NOT NULL,
    marked_by uuid,
    reason text NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    source text DEFAULT 'manual'::text NOT NULL,
    job_id uuid,
    error_code text,
    CONSTRAINT bet_order_exceptions_check CHECK ((((source = 'manual'::text) AND (marked_by IS NOT NULL) AND (job_id IS NULL) AND (error_code IS NULL)) OR ((source = 'system'::text) AND (marked_by IS NULL) AND (job_id IS NOT NULL) AND (error_code IS NOT NULL)))),
    CONSTRAINT bet_order_exceptions_order_version_check CHECK ((order_version > 1)),
    CONSTRAINT bet_order_exceptions_reason_check CHECK (((length(TRIM(BOTH FROM reason)) > 0) AND (octet_length(reason) <= 500))),
    CONSTRAINT bet_order_exceptions_source_check CHECK ((source = ANY (ARRAY['manual'::text, 'system'::text])))
);

CREATE TABLE bet_order_judgments (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    game_id uuid NOT NULL,
    period_id uuid NOT NULL,
    order_id uuid NOT NULL,
    order_version bigint NOT NULL,
    cause text NOT NULL,
    draw_result_id uuid,
    judged_by uuid NOT NULL,
    reason text NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT bet_order_judgments_cause_check CHECK ((cause = ANY (ARRAY['no_result'::text, 'invalid_result'::text]))),
    CONSTRAINT bet_order_judgments_check CHECK (((cause <> 'no_result'::text) OR (draw_result_id IS NULL))),
    CONSTRAINT bet_order_judgments_order_version_check CHECK ((order_version > 1)),
    CONSTRAINT bet_order_judgments_reason_check CHECK (((length(TRIM(BOTH FROM reason)) > 0) AND (octet_length(reason) <= 500)))
);

CREATE TABLE bet_orders (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    global_user_id uuid NOT NULL,
    brand_member_id uuid NOT NULL,
    account_id uuid NOT NULL,
    game_id uuid NOT NULL,
    period_id uuid NOT NULL,
    play_id uuid NOT NULL,
    rule_version_id uuid NOT NULL,
    definition_snapshot jsonb NOT NULL,
    definition_hash text NOT NULL,
    status text DEFAULT 'placed'::text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    selection_raw jsonb NOT NULL,
    selection_normalized jsonb NOT NULL,
    expanded_bets jsonb NOT NULL,
    unit_points bigint NOT NULL,
    combination_count integer NOT NULL,
    multiplier bigint NOT NULL,
    total_points bigint NOT NULL,
    deduction_allocation jsonb NOT NULL,
    policy_snapshot jsonb NOT NULL,
    brand_policy_version bigint NOT NULL,
    game_policy_version bigint NOT NULL,
    debit_entry_id uuid NOT NULL,
    refund_entry_id uuid,
    client_key text NOT NULL,
    placed_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    cancelled_at timestamp with time zone,
    cancel_reason text DEFAULT ''::text NOT NULL,
    settlement_calculation_id uuid,
    payout_entry_id uuid,
    prize_points bigint DEFAULT 0 NOT NULL,
    settled_at timestamp with time zone,
    attribution_snapshot jsonb NOT NULL,
    withdrawal_rule_snapshot jsonb NOT NULL,
    commission_rule_snapshot jsonb NOT NULL,
    CONSTRAINT bet_commission_snapshot_object CHECK ((jsonb_typeof(commission_rule_snapshot) = 'object'::text)),
    CONSTRAINT bet_order_withdrawal_snapshot_valid CHECK (((withdrawal_rule_snapshot IS NULL) OR valid_bet_withdrawal_snapshot(withdrawal_rule_snapshot))),
    CONSTRAINT bet_orders_brand_policy_version_check CHECK ((brand_policy_version > 0)),
    CONSTRAINT bet_orders_check CHECK ((jsonb_array_length(expanded_bets) = combination_count)),
    CONSTRAINT bet_orders_check1 CHECK (((total_points)::numeric = (((unit_points)::numeric * (combination_count)::numeric) * (multiplier)::numeric))),
    CONSTRAINT bet_orders_check2 CHECK (((status = ANY (ARRAY['bet_cancelled'::text, 'judged_cancelled'::text])) = (refund_entry_id IS NOT NULL))),
    CONSTRAINT bet_orders_check3 CHECK (((refund_entry_id IS NOT NULL) = (cancelled_at IS NOT NULL))),
    CONSTRAINT bet_orders_check4 CHECK ((((status = ANY (ARRAY['won'::text, 'lost'::text])) AND (settlement_calculation_id IS NOT NULL) AND (settled_at IS NOT NULL)) OR ((status <> ALL (ARRAY['won'::text, 'lost'::text])) AND (settlement_calculation_id IS NULL) AND (settled_at IS NULL) AND (prize_points = 0) AND (payout_entry_id IS NULL)))),
    CONSTRAINT bet_orders_check5 CHECK (((prize_points > 0) = (payout_entry_id IS NOT NULL))),
    CONSTRAINT bet_orders_client_key_check CHECK ((client_key ~ '^[A-Za-z0-9_:.-]{8,128}$'::text)),
    CONSTRAINT bet_orders_combination_count_check CHECK (((combination_count >= 1) AND (combination_count <= 10000))),
    CONSTRAINT bet_orders_deduction_allocation_check CHECK ((jsonb_typeof(deduction_allocation) = 'array'::text)),
    CONSTRAINT bet_orders_definition_hash_check CHECK ((definition_hash ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT bet_orders_definition_snapshot_check CHECK ((jsonb_typeof(definition_snapshot) = 'object'::text)),
    CONSTRAINT bet_orders_expanded_bets_check CHECK ((jsonb_typeof(expanded_bets) = 'array'::text)),
    CONSTRAINT bet_orders_game_policy_version_check CHECK ((game_policy_version > 0)),
    CONSTRAINT bet_orders_multiplier_check CHECK ((multiplier > 0)),
    CONSTRAINT bet_orders_policy_snapshot_check CHECK ((jsonb_typeof(policy_snapshot) = 'object'::text)),
    CONSTRAINT bet_orders_prize_points_check CHECK ((prize_points >= 0)),
    CONSTRAINT bet_orders_selection_normalized_check CHECK ((jsonb_typeof(selection_normalized) = 'object'::text)),
    CONSTRAINT bet_orders_selection_raw_check CHECK ((jsonb_typeof(selection_raw) = 'object'::text)),
    CONSTRAINT bet_orders_status_check CHECK ((status = ANY (ARRAY['placed'::text, 'abnormal'::text, 'won'::text, 'lost'::text, 'settled'::text, 'bet_cancelled'::text, 'judged_cancelled'::text]))),
    CONSTRAINT bet_orders_total_points_check CHECK ((total_points > 0)),
    CONSTRAINT bet_orders_unit_points_check CHECK ((unit_points > 0)),
    CONSTRAINT bet_orders_version_check CHECK ((version > 0))
);

CREATE TABLE brand_agent_policies (
    brand_id uuid NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    config jsonb DEFAULT default_agent_policy() NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT brand_agent_policies_config_check CHECK ((valid_agent_config(config, true) IS TRUE)),
    CONSTRAINT brand_agent_policies_version_check CHECK ((version > 0))
);

CREATE TABLE brand_bet_policies (
    brand_id uuid NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    config jsonb NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT brand_bet_policies_config_check CHECK ((jsonb_typeof(config) = 'object'::text)),
    CONSTRAINT brand_bet_policies_version_check CHECK ((version > 0))
);

CREATE TABLE brand_commission_correction_policies (
    brand_id uuid NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    enabled boolean DEFAULT false NOT NULL,
    audit_log_id uuid,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT brand_commission_correction_policies_version_check CHECK (((version >= 1) AND (version <= '9007199254740991'::bigint)))
);

CREATE TABLE brand_commission_payment_policies (
    brand_id uuid NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    enabled boolean DEFAULT false NOT NULL,
    audit_log_id uuid,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT brand_commission_payment_policies_version_check CHECK (((version >= 1) AND (version <= '9007199254740991'::bigint)))
);

CREATE TABLE brand_commission_policies (
    brand_id uuid NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    config jsonb DEFAULT default_commission_policy() NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT brand_commission_policies_config_check CHECK ((valid_commission_policy(config) IS TRUE)),
    CONSTRAINT brand_commission_policies_version_check CHECK ((version > 0))
);

CREATE TABLE brand_compliance_policies (
    brand_id uuid NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    config jsonb DEFAULT default_compliance_config() NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT brand_compliance_policies_config_check CHECK ((valid_compliance_config(config) IS TRUE)),
    CONSTRAINT brand_compliance_policies_version_check CHECK ((version > 0))
);

CREATE TABLE brand_creation_records (
    brand_id uuid NOT NULL,
    code text NOT NULL,
    name text NOT NULL,
    default_locale text NOT NULL,
    timezone text NOT NULL,
    status text NOT NULL,
    version bigint NOT NULL,
    created_by uuid NOT NULL,
    reason text NOT NULL,
    audit_log_id uuid NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT brand_creation_records_code_check CHECK ((code ~ '^[a-z][a-z0-9_]{0,47}$'::text)),
    CONSTRAINT brand_creation_records_default_locale_check CHECK ((default_locale = ANY (ARRAY['en'::text, 'zh-CN'::text]))),
    CONSTRAINT brand_creation_records_name_check CHECK (((length(TRIM(BOTH FROM name)) > 0) AND (octet_length(name) <= 120))),
    CONSTRAINT brand_creation_records_reason_check CHECK (((length(TRIM(BOTH FROM reason)) > 0) AND (octet_length(reason) <= 500))),
    CONSTRAINT brand_creation_records_status_check CHECK ((status = 'paused'::text)),
    CONSTRAINT brand_creation_records_timezone_check CHECK (((length(timezone) > 0) AND (octet_length(timezone) <= 80))),
    CONSTRAINT brand_creation_records_version_check CHECK ((version = 1))
);

CREATE TABLE brand_domain_revisions (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    version bigint NOT NULL,
    changed_by uuid NOT NULL,
    reason text NOT NULL,
    audit_log_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    before_domains jsonb NOT NULL,
    domains jsonb NOT NULL,
    CONSTRAINT brand_domain_revisions_before_domains_check CHECK ((jsonb_typeof(before_domains) = 'array'::text)),
    CONSTRAINT brand_domain_revisions_domains_check CHECK ((jsonb_typeof(domains) = 'array'::text)),
    CONSTRAINT brand_domain_revisions_reason_check CHECK (((length(TRIM(BOTH FROM reason)) > 0) AND (octet_length(reason) <= 500))),
    CONSTRAINT brand_domain_revisions_version_check CHECK ((version > 1))
);

CREATE TABLE brand_domains (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    domain text NOT NULL,
    is_primary boolean DEFAULT false NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    CONSTRAINT brand_domains_domain_check CHECK ((domain = lower(domain)))
);

CREATE TABLE brand_members (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    global_user_id uuid NOT NULL,
    status text DEFAULT 'normal'::text NOT NULL,
    display_name text DEFAULT ''::text NOT NULL,
    notes text DEFAULT ''::text NOT NULL,
    profile_snapshot jsonb DEFAULT '{}'::jsonb NOT NULL,
    join_method text NOT NULL,
    join_domain text,
    attribution_snapshot jsonb DEFAULT '{}'::jsonb NOT NULL,
    privacy_policy_version text NOT NULL,
    service_terms_version text NOT NULL,
    accepted_at timestamp with time zone DEFAULT now(),
    joined_at timestamp with time zone DEFAULT now() NOT NULL,
    terms_accepted boolean DEFAULT true NOT NULL,
    created_by uuid,
    CONSTRAINT brand_members_join_method_check CHECK ((join_method = ANY (ARRAY['domain'::text, 'agent_code'::text, 'referral_code'::text, 'operator'::text]))),
    CONSTRAINT brand_members_status_check CHECK ((status = ANY (ARRAY['normal'::text, 'frozen'::text, 'disabled'::text, 'expired'::text, 'cancelled'::text]))),
    CONSTRAINT member_consent_consistent CHECK (((terms_accepted AND (accepted_at IS NOT NULL)) OR ((NOT terms_accepted) AND (accepted_at IS NULL))))
);

CREATE TABLE brand_operation_revisions (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    version bigint NOT NULL,
    previous_status text NOT NULL,
    status text NOT NULL,
    changed_by uuid NOT NULL,
    reason text NOT NULL,
    audit_log_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT brand_operation_revisions_check CHECK ((previous_status <> status)),
    CONSTRAINT brand_operation_revisions_previous_status_check CHECK ((previous_status = ANY (ARRAY['active'::text, 'paused'::text]))),
    CONSTRAINT brand_operation_revisions_reason_check CHECK (((length(TRIM(BOTH FROM reason)) > 0) AND (octet_length(reason) <= 500))),
    CONSTRAINT brand_operation_revisions_status_check CHECK ((status = ANY (ARRAY['active'::text, 'paused'::text]))),
    CONSTRAINT brand_operation_revisions_version_check CHECK ((version > 1))
);

CREATE TABLE brand_point_policies (
    brand_id uuid NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    max_balance_points bigint,
    max_recharge_points bigint,
    max_adjustment_points bigint,
    CONSTRAINT brand_point_policies_max_adjustment_points_check CHECK ((max_adjustment_points > 0)),
    CONSTRAINT brand_point_policies_max_balance_points_check CHECK ((max_balance_points > 0)),
    CONSTRAINT brand_point_policies_max_recharge_points_check CHECK ((max_recharge_points > 0)),
    CONSTRAINT brand_point_policies_version_check CHECK ((version > 0))
);

CREATE TABLE brand_presentation_revisions (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    version bigint NOT NULL,
    config jsonb NOT NULL,
    effective jsonb NOT NULL,
    changed_by uuid NOT NULL,
    reason text NOT NULL,
    audit_log_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT brand_presentation_revisions_reason_check CHECK (((length(TRIM(BOTH FROM reason)) > 0) AND (octet_length(reason) <= 500))),
    CONSTRAINT brand_presentation_revisions_version_check CHECK ((version > 1))
);

CREATE TABLE brand_presentations (
    brand_id uuid NOT NULL,
    version bigint NOT NULL,
    config jsonb NOT NULL,
    CONSTRAINT brand_presentations_config_check CHECK ((jsonb_typeof(config) = 'object'::text)),
    CONSTRAINT brand_presentations_version_check CHECK ((version > 0))
);

CREATE TABLE brand_report_archive_policies (
    brand_id uuid NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    daily_enabled boolean DEFAULT false NOT NULL,
    monthly_enabled boolean DEFAULT false NOT NULL,
    daily_start_period text,
    monthly_start_period text,
    timezone text NOT NULL,
    audit_log_id uuid,
    updated_at timestamp with time zone DEFAULT statement_timestamp() NOT NULL,
    CONSTRAINT brand_report_archive_policies_check CHECK (((NOT daily_enabled) OR (daily_start_period IS NOT NULL))),
    CONSTRAINT brand_report_archive_policies_check1 CHECK (((NOT monthly_enabled) OR (monthly_start_period IS NOT NULL))),
    CONSTRAINT brand_report_archive_policies_version_check CHECK (((version >= 1) AND (version <= '9007199254740991'::bigint)))
);

CREATE TABLE brand_settlement_policies (
    brand_id uuid NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    mode text,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT brand_settlement_policies_mode_check CHECK ((mode = ANY (ARRAY['automatic'::text, 'manual'::text]))),
    CONSTRAINT brand_settlement_policies_version_check CHECK ((version > 0))
);

CREATE TABLE brand_withdrawal_policies (
    brand_id uuid NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    config jsonb DEFAULT default_brand_withdrawal_policy() NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT brand_withdrawal_policies_config_check CHECK (valid_withdrawal_policy(config, false)),
    CONSTRAINT brand_withdrawal_policies_version_check CHECK ((version > 0))
);

CREATE TABLE commission_adjustment_heads (
    target_id uuid NOT NULL,
    brand_id uuid NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    points bigint NOT NULL,
    last_adjustment_id uuid,
    CONSTRAINT commission_adjustment_heads_points_check CHECK ((points >= 0)),
    CONSTRAINT commission_adjustment_heads_version_check CHECK (((version >= 1) AND (version <= '9007199254740991'::bigint)))
);

CREATE TABLE commission_adjustments (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    target_id uuid NOT NULL,
    payment_id uuid NOT NULL,
    version bigint NOT NULL,
    points_before bigint NOT NULL,
    points_after bigint NOT NULL,
    delta_points bigint NOT NULL,
    ledger_entry_id uuid,
    audit_log_id uuid NOT NULL,
    created_by uuid NOT NULL,
    reason text NOT NULL,
    point_policy_version bigint NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    creation_xid xid8 DEFAULT pg_current_xact_id() NOT NULL,
    CONSTRAINT commission_adjustments_check CHECK ((((points_after)::numeric - (points_before)::numeric) = (delta_points)::numeric)),
    CONSTRAINT commission_adjustments_delta_points_check CHECK ((delta_points <> 0)),
    CONSTRAINT commission_adjustments_point_policy_version_check CHECK ((point_policy_version > 0)),
    CONSTRAINT commission_adjustments_points_after_check CHECK ((points_after >= 0)),
    CONSTRAINT commission_adjustments_points_before_check CHECK ((points_before >= 0)),
    CONSTRAINT commission_adjustments_reason_check CHECK ((((octet_length(reason) >= 1) AND (octet_length(reason) <= 500)) AND (reason = TRIM(BOTH FROM reason)))),
    CONSTRAINT commission_adjustments_version_check CHECK (((version >= 2) AND (version <= '9007199254740991'::bigint)))
);

CREATE TABLE commission_allocations (
    brand_id uuid NOT NULL,
    cycle_id uuid NOT NULL,
    run_id uuid NOT NULL,
    calculation_id uuid NOT NULL,
    order_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    member_id uuid NOT NULL,
    numerator text NOT NULL,
    denominator text NOT NULL,
    CONSTRAINT commission_allocations_check CHECK ((valid_commission_fraction(numerator, denominator) IS TRUE))
);

CREATE TABLE commission_calculations (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    cycle_id uuid NOT NULL,
    run_id uuid NOT NULL,
    order_id uuid NOT NULL,
    reason text NOT NULL,
    status text NOT NULL,
    member_id uuid NOT NULL,
    account_id uuid NOT NULL,
    stake_points bigint NOT NULL,
    prize_points bigint NOT NULL,
    base_points bigint NOT NULL,
    job_id uuid,
    calculation_id uuid,
    generation bigint,
    rule_snapshot jsonb NOT NULL,
    audit_log_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT commission_calculations_base_points_check CHECK ((base_points >= 0)),
    CONSTRAINT commission_calculations_check CHECK (((job_id IS NULL) = (calculation_id IS NULL))),
    CONSTRAINT commission_calculations_check1 CHECK (((job_id IS NULL) = (generation IS NULL))),
    CONSTRAINT commission_calculations_generation_check CHECK (((generation IS NULL) OR (generation > 0))),
    CONSTRAINT commission_calculations_prize_points_check CHECK ((prize_points >= 0)),
    CONSTRAINT commission_calculations_reason_check CHECK ((reason = ANY (ARRAY['eligible'::text, 'policy_disabled'::text, 'unattributed'::text, 'other_cycle'::text, 'cancelled'::text, 'abnormal'::text]))),
    CONSTRAINT commission_calculations_rule_snapshot_check CHECK ((jsonb_typeof(rule_snapshot) = 'object'::text)),
    CONSTRAINT commission_calculations_stake_points_check CHECK ((stake_points > 0))
);

CREATE TABLE commission_correction_balance_heads (
    brand_id uuid NOT NULL,
    cycle_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    member_id uuid NOT NULL,
    version bigint NOT NULL,
    points bigint NOT NULL,
    original_target_id uuid,
    last_execution_target_id uuid NOT NULL,
    CONSTRAINT commission_correction_balance_heads_points_check CHECK ((points >= 0)),
    CONSTRAINT commission_correction_balance_heads_version_check CHECK (((version >= 1) AND (version <= '9007199254740991'::bigint)))
);

CREATE TABLE commission_correction_cycle_holds (
    cycle_id uuid NOT NULL,
    brand_id uuid NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    active boolean DEFAULT false NOT NULL,
    blocked_execution_id uuid,
    audit_log_id uuid,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT commission_correction_cycle_holds_check CHECK (((NOT active) OR ((blocked_execution_id IS NOT NULL) AND (audit_log_id IS NOT NULL)))),
    CONSTRAINT commission_correction_cycle_holds_version_check CHECK (((version >= 1) AND (version <= '9007199254740991'::bigint)))
);

CREATE TABLE commission_correction_execution_steps (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    execution_id uuid NOT NULL,
    version bigint NOT NULL,
    from_state text,
    to_state text NOT NULL,
    operation text NOT NULL,
    target_id uuid,
    paused_plan_target_id uuid,
    error_code text,
    actor_type text NOT NULL,
    actor_id uuid,
    reason text NOT NULL,
    request_id text NOT NULL,
    audit_log_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT commission_correction_execution_steps_actor_type_check CHECK ((actor_type = ANY (ARRAY['admin'::text, 'system'::text]))),
    CONSTRAINT commission_correction_execution_steps_check CHECK (((actor_type = 'admin'::text) = (actor_id IS NOT NULL))),
    CONSTRAINT commission_correction_execution_steps_check1 CHECK (((version = 1) = (from_state IS NULL))),
    CONSTRAINT commission_correction_execution_steps_check2 CHECK (((operation = 'apply'::text) = (target_id IS NOT NULL))),
    CONSTRAINT commission_correction_execution_steps_operation_check CHECK ((operation = ANY (ARRAY['create'::text, 'approve'::text, 'apply'::text, 'complete'::text, 'pause'::text, 'continue'::text, 'retry'::text, 'fail'::text, 'invalidate'::text]))),
    CONSTRAINT commission_correction_execution_steps_reason_check CHECK (((octet_length(reason) >= 1) AND (octet_length(reason) <= 500))),
    CONSTRAINT commission_correction_execution_steps_version_check CHECK (((version >= 1) AND (version <= '9007199254740991'::bigint)))
);

CREATE TABLE commission_correction_execution_targets (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    cycle_id uuid NOT NULL,
    execution_id uuid NOT NULL,
    plan_target_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    member_id uuid NOT NULL,
    points_before bigint NOT NULL,
    points_after bigint NOT NULL,
    delta_points bigint NOT NULL,
    base_financial_version bigint,
    financial_version bigint,
    execution_version bigint NOT NULL,
    state text DEFAULT 'pending'::text NOT NULL,
    ledger_entry_id uuid,
    audit_log_id uuid,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    applied_at timestamp with time zone,
    creation_xid xid8 DEFAULT pg_current_xact_id() NOT NULL,
    CONSTRAINT commission_correction_execution_ta_base_financial_version_check CHECK (((base_financial_version >= 1) AND (base_financial_version <= '9007199254740991'::bigint))),
    CONSTRAINT commission_correction_execution_targets_check CHECK ((((points_after)::numeric - (points_before)::numeric) = (delta_points)::numeric)),
    CONSTRAINT commission_correction_execution_targets_check1 CHECK (((state = 'applied'::text) = (applied_at IS NOT NULL))),
    CONSTRAINT commission_correction_execution_targets_check2 CHECK (((state = 'applied'::text) = (audit_log_id IS NOT NULL))),
    CONSTRAINT commission_correction_execution_targets_check3 CHECK (((state = 'applied'::text) = (financial_version IS NOT NULL))),
    CONSTRAINT commission_correction_execution_targets_check4 CHECK (((state <> 'pending'::text) OR (ledger_entry_id IS NULL))),
    CONSTRAINT commission_correction_execution_targets_check5 CHECK (((state <> 'applied'::text) OR ((delta_points = 0) = (ledger_entry_id IS NULL)))),
    CONSTRAINT commission_correction_execution_targets_execution_version_check CHECK (((execution_version >= 1) AND (execution_version <= '9007199254740991'::bigint))),
    CONSTRAINT commission_correction_execution_targets_financial_version_check CHECK (((financial_version >= 1) AND (financial_version <= '9007199254740991'::bigint))),
    CONSTRAINT commission_correction_execution_targets_points_after_check CHECK ((points_after >= 0)),
    CONSTRAINT commission_correction_execution_targets_points_before_check CHECK ((points_before >= 0)),
    CONSTRAINT commission_correction_execution_targets_state_check CHECK ((state = ANY (ARRAY['pending'::text, 'applied'::text])))
);

CREATE TABLE commission_correction_executions (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    cycle_id uuid NOT NULL,
    plan_id uuid NOT NULL,
    run_id uuid NOT NULL,
    plan_version bigint NOT NULL,
    evidence_epoch bigint NOT NULL,
    payout_mode text NOT NULL,
    state text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    credit_points numeric NOT NULL,
    debit_points numeric NOT NULL,
    net_points numeric NOT NULL,
    target_count bigint NOT NULL,
    approved_by uuid,
    approval_actor_type text,
    approval_audit_log_id uuid,
    paused_plan_target_id uuid,
    last_error_code text,
    creation_audit_log_id uuid NOT NULL,
    last_audit_log_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    next_work_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    creation_xid xid8 DEFAULT pg_current_xact_id() NOT NULL,
    CONSTRAINT commission_correction_executions_approval_actor_type_check CHECK ((approval_actor_type = ANY (ARRAY['admin'::text, 'system'::text]))),
    CONSTRAINT commission_correction_executions_check CHECK (((scale(net_points) = 0) AND (net_points = (credit_points - debit_points)))),
    CONSTRAINT commission_correction_executions_check1 CHECK (((approval_actor_type IS NULL) = (approval_audit_log_id IS NULL))),
    CONSTRAINT commission_correction_executions_check2 CHECK (((NOT ((approval_actor_type = 'admin'::text) IS DISTINCT FROM (approved_by IS NOT NULL))) OR ((approval_actor_type IS NULL) AND (approved_by IS NULL)))),
    CONSTRAINT commission_correction_executions_check3 CHECK (((state <> ALL (ARRAY['applying'::text, 'completed'::text, 'paused'::text, 'failed'::text])) OR (approval_audit_log_id IS NOT NULL))),
    CONSTRAINT commission_correction_executions_check4 CHECK (((state = ANY (ARRAY['paused'::text, 'failed'::text])) = (last_error_code IS NOT NULL))),
    CONSTRAINT commission_correction_executions_check5 CHECK (((NOT ((last_error_code = 'COMMISSION_CORRECTION_AVAILABLE_INSUFFICIENT'::text) IS DISTINCT FROM (paused_plan_target_id IS NOT NULL))) OR ((last_error_code IS NULL) AND (paused_plan_target_id IS NULL)))),
    CONSTRAINT commission_correction_executions_credit_points_check CHECK (((credit_points >= (0)::numeric) AND (scale(credit_points) = 0))),
    CONSTRAINT commission_correction_executions_debit_points_check CHECK (((debit_points >= (0)::numeric) AND (scale(debit_points) = 0))),
    CONSTRAINT commission_correction_executions_evidence_epoch_check CHECK ((evidence_epoch >= 0)),
    CONSTRAINT commission_correction_executions_payout_mode_check CHECK ((payout_mode = ANY (ARRAY['manual'::text, 'automatic'::text, 'mixed'::text, 'none'::text]))),
    CONSTRAINT commission_correction_executions_plan_version_check CHECK (((plan_version >= 1) AND (plan_version <= '9007199254740991'::bigint))),
    CONSTRAINT commission_correction_executions_state_check CHECK ((state = ANY (ARRAY['awaiting_approval'::text, 'applying'::text, 'completed'::text, 'paused'::text, 'failed'::text, 'stale'::text]))),
    CONSTRAINT commission_correction_executions_target_count_check CHECK (((target_count >= 0) AND (target_count <= 100000))),
    CONSTRAINT commission_correction_executions_version_check CHECK (((version >= 1) AND (version <= '9007199254740991'::bigint)))
);

CREATE TABLE commission_correction_plan_steps (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    plan_id uuid NOT NULL,
    version bigint NOT NULL,
    from_state text,
    to_state text NOT NULL,
    operation text NOT NULL,
    planned_count bigint NOT NULL,
    cursor_agent_id uuid,
    error_code text,
    actor_type text NOT NULL,
    actor_id uuid,
    reason text NOT NULL,
    request_id text NOT NULL,
    audit_log_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT commission_correction_plan_steps_actor_type_check CHECK ((actor_type = ANY (ARRAY['admin'::text, 'system'::text]))),
    CONSTRAINT commission_correction_plan_steps_check CHECK (((actor_type = 'admin'::text) = (actor_id IS NOT NULL))),
    CONSTRAINT commission_correction_plan_steps_check1 CHECK (((version = 1) = (from_state IS NULL))),
    CONSTRAINT commission_correction_plan_steps_operation_check CHECK ((operation = ANY (ARRAY['create'::text, 'page'::text, 'ready'::text, 'invalidate'::text, 'fail'::text, 'retry'::text]))),
    CONSTRAINT commission_correction_plan_steps_planned_count_check CHECK ((planned_count >= 0)),
    CONSTRAINT commission_correction_plan_steps_reason_check CHECK (((octet_length(reason) >= 1) AND (octet_length(reason) <= 500))),
    CONSTRAINT commission_correction_plan_steps_version_check CHECK (((version >= 1) AND (version <= '9007199254740991'::bigint)))
);

CREATE TABLE commission_correction_plan_targets (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    plan_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    member_id uuid NOT NULL,
    original_target_id uuid,
    earning_id uuid,
    adjustment_version bigint,
    points_before bigint NOT NULL,
    points_after bigint NOT NULL,
    delta_points bigint NOT NULL,
    plan_version bigint NOT NULL,
    creation_audit_log_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    creation_xid xid8 DEFAULT pg_current_xact_id() NOT NULL,
    previous_correction_target_id uuid,
    financial_version bigint,
    CONSTRAINT commission_correction_plan_targets_check CHECK ((((points_after)::numeric - (points_before)::numeric) = (delta_points)::numeric)),
    CONSTRAINT commission_correction_plan_targets_check1 CHECK (((original_target_id IS NOT NULL) OR (earning_id IS NOT NULL) OR (previous_correction_target_id IS NOT NULL))),
    CONSTRAINT commission_correction_plan_targets_check2 CHECK (((original_target_id IS NULL) = (adjustment_version IS NULL))),
    CONSTRAINT commission_correction_plan_targets_check3 CHECK (((previous_correction_target_id IS NULL) = (financial_version IS NULL))),
    CONSTRAINT commission_correction_plan_targets_financial_version_check CHECK (((financial_version >= 1) AND (financial_version <= '9007199254740991'::bigint))),
    CONSTRAINT commission_correction_plan_targets_plan_version_check CHECK (((plan_version >= 2) AND (plan_version <= '9007199254740991'::bigint))),
    CONSTRAINT commission_correction_plan_targets_points_after_check CHECK ((points_after >= 0)),
    CONSTRAINT commission_correction_plan_targets_points_before_check CHECK ((points_before >= 0))
);

CREATE TABLE commission_correction_plans (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    cycle_id uuid NOT NULL,
    payment_id uuid NOT NULL,
    run_id uuid NOT NULL,
    evidence_epoch bigint NOT NULL,
    payout_mode text NOT NULL,
    state text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    before_points numeric NOT NULL,
    calculated_points numeric NOT NULL,
    credit_points numeric NOT NULL,
    debit_points numeric NOT NULL,
    net_points numeric NOT NULL,
    target_count bigint NOT NULL,
    planned_count bigint DEFAULT 0 NOT NULL,
    cursor_agent_id uuid,
    creation_audit_log_id uuid NOT NULL,
    last_audit_log_id uuid NOT NULL,
    last_error_code text,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    next_work_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    creation_xid xid8 DEFAULT pg_current_xact_id() NOT NULL,
    CONSTRAINT commission_correction_plans_before_points_check CHECK (((before_points >= (0)::numeric) AND (scale(before_points) = 0))),
    CONSTRAINT commission_correction_plans_calculated_points_check CHECK (((calculated_points >= (0)::numeric) AND (scale(calculated_points) = 0))),
    CONSTRAINT commission_correction_plans_check CHECK (((planned_count >= 0) AND (planned_count <= target_count))),
    CONSTRAINT commission_correction_plans_check1 CHECK (((state = ANY (ARRAY['failed'::text])) = (last_error_code IS NOT NULL))),
    CONSTRAINT commission_correction_plans_check3 CHECK (((net_points IS NULL) OR ((net_points = (credit_points - debit_points)) AND (net_points = (calculated_points - before_points))))),
    CONSTRAINT commission_correction_plans_check4 CHECK (((state <> 'ready'::text) OR ((planned_count = target_count) AND (credit_points IS NOT NULL)))),
    CONSTRAINT commission_correction_plans_check6 CHECK (((planned_count = 0) = (cursor_agent_id IS NULL))),
    CONSTRAINT commission_correction_plans_credit_points_check CHECK (((credit_points >= (0)::numeric) AND (scale(credit_points) = 0))),
    CONSTRAINT commission_correction_plans_debit_points_check CHECK (((debit_points >= (0)::numeric) AND (scale(debit_points) = 0))),
    CONSTRAINT commission_correction_plans_evidence_epoch_check CHECK ((evidence_epoch >= 0)),
    CONSTRAINT commission_correction_plans_net_points_check CHECK ((scale(net_points) = 0)),
    CONSTRAINT commission_correction_plans_payout_mode_check CHECK ((payout_mode = ANY (ARRAY['manual'::text, 'automatic'::text, 'mixed'::text, 'none'::text]))),
    CONSTRAINT commission_correction_plans_state_check CHECK ((state = ANY (ARRAY['planning'::text, 'ready'::text, 'failed'::text, 'stale'::text]))),
    CONSTRAINT commission_correction_plans_target_count_check CHECK (((target_count >= 0) AND (target_count <= 100000))),
    CONSTRAINT commission_correction_plans_version_check CHECK (((version >= 1) AND (version <= '9007199254740991'::bigint)))
);

CREATE TABLE commission_correction_policy_revisions (
    brand_id uuid NOT NULL,
    version bigint NOT NULL,
    enabled boolean NOT NULL,
    audit_log_id uuid,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL
);

CREATE TABLE commission_cycle_steps (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    cycle_id uuid NOT NULL,
    version bigint NOT NULL,
    from_state text NOT NULL,
    to_state text NOT NULL,
    operation text NOT NULL,
    run_id uuid,
    reason text NOT NULL,
    actor_type text NOT NULL,
    actor_id uuid,
    audit_log_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT commission_cycle_steps_actor_type_check CHECK ((actor_type = ANY (ARRAY['system'::text, 'admin'::text]))),
    CONSTRAINT commission_cycle_steps_check CHECK (((actor_type = 'admin'::text) = (actor_id IS NOT NULL))),
    CONSTRAINT commission_cycle_steps_reason_check CHECK ((((octet_length(reason) >= 1) AND (octet_length(reason) <= 500)) AND (TRIM(BOTH FROM reason) = reason))),
    CONSTRAINT commission_cycle_steps_version_check CHECK ((version > 1))
);

CREATE TABLE commission_cycle_targets (
    brand_id uuid NOT NULL,
    cycle_id uuid NOT NULL,
    order_id uuid NOT NULL,
    placed_at timestamp with time zone NOT NULL,
    creation_xid xid8 DEFAULT pg_current_xact_id() NOT NULL
);

CREATE TABLE commission_cycles (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    window_from timestamp with time zone NOT NULL,
    window_to timestamp with time zone NOT NULL,
    anchor_order_id uuid NOT NULL,
    calendar jsonb NOT NULL,
    state text DEFAULT 'enumerating'::text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    evidence_epoch bigint DEFAULT 0 NOT NULL,
    target_count bigint DEFAULT 0 NOT NULL,
    manifest_cursor_at timestamp with time zone,
    manifest_cursor_id uuid,
    scan_complete boolean DEFAULT false NOT NULL,
    current_run_id uuid,
    created_by uuid,
    reason text NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    next_work_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    last_error_code text,
    creation_audit_log_id uuid NOT NULL,
    creation_actor_type text DEFAULT 'admin'::text NOT NULL,
    CONSTRAINT commission_cycles_check CHECK ((window_from < window_to)),
    CONSTRAINT commission_cycles_check1 CHECK (((manifest_cursor_at IS NULL) = (manifest_cursor_id IS NULL))),
    CONSTRAINT commission_cycles_check2 CHECK (((state = 'failed'::text) = (last_error_code IS NOT NULL))),
    CONSTRAINT commission_cycles_check3 CHECK (((state = ANY (ARRAY['enumerating'::text, 'failed'::text])) OR scan_complete)),
    CONSTRAINT commission_cycles_check4 CHECK (((state <> ALL (ARRAY['calculating'::text, 'summarizing'::text, 'ready'::text])) OR (current_run_id IS NOT NULL))),
    CONSTRAINT commission_cycles_check5 CHECK (((creation_actor_type = 'admin'::text) = (created_by IS NOT NULL))),
    CONSTRAINT commission_cycles_creation_actor_type_check CHECK ((creation_actor_type = ANY (ARRAY['admin'::text, 'system'::text]))),
    CONSTRAINT commission_cycles_evidence_epoch_check CHECK ((evidence_epoch >= 0)),
    CONSTRAINT commission_cycles_reason_check CHECK ((((octet_length(reason) >= 1) AND (octet_length(reason) <= 500)) AND (TRIM(BOTH FROM reason) = reason))),
    CONSTRAINT commission_cycles_state_check CHECK ((state = ANY (ARRAY['enumerating'::text, 'waiting'::text, 'calculating'::text, 'summarizing'::text, 'ready'::text, 'failed'::text]))),
    CONSTRAINT commission_cycles_target_count_check CHECK ((target_count >= 0)),
    CONSTRAINT commission_cycles_version_check CHECK (((version >= 1) AND (version <= '9007199254740991'::bigint)))
);

CREATE TABLE commission_discovery (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    state text DEFAULT 'pending'::text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    cycle_id uuid,
    window_from timestamp with time zone,
    window_to timestamp with time zone,
    next_check_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    last_error_code text,
    last_audit_log_id uuid,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT commission_discovery_check CHECK (((window_from IS NULL) = (window_to IS NULL))),
    CONSTRAINT commission_discovery_check1 CHECK (((window_from IS NULL) OR (window_from < window_to))),
    CONSTRAINT commission_discovery_check2 CHECK (((state = 'registered'::text) = (cycle_id IS NOT NULL))),
    CONSTRAINT commission_discovery_check3 CHECK (((state = 'failed'::text) = (last_error_code IS NOT NULL))),
    CONSTRAINT commission_discovery_check4 CHECK (((state <> 'registered'::text) OR (window_from IS NOT NULL))),
    CONSTRAINT commission_discovery_state_check CHECK ((state = ANY (ARRAY['pending'::text, 'registered'::text, 'failed'::text]))),
    CONSTRAINT commission_discovery_version_check CHECK (((version >= 1) AND (version <= '9007199254740991'::bigint)))
);

CREATE TABLE commission_earnings (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    cycle_id uuid NOT NULL,
    run_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    member_id uuid NOT NULL,
    numerator text NOT NULL,
    denominator text NOT NULL,
    points bigint NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT commission_earnings_check CHECK ((valid_commission_fraction(numerator, denominator) IS TRUE)),
    CONSTRAINT commission_earnings_points_check CHECK ((points >= 0))
);

CREATE TABLE commission_payment_policy_revisions (
    brand_id uuid NOT NULL,
    version bigint NOT NULL,
    enabled boolean NOT NULL,
    audit_log_id uuid,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL
);

CREATE TABLE commission_payment_targets (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    payment_id uuid NOT NULL,
    earning_id uuid NOT NULL,
    points bigint NOT NULL,
    member_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    payment_version bigint NOT NULL,
    state text DEFAULT 'pending'::text NOT NULL,
    ledger_entry_id uuid,
    audit_log_id uuid,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    paid_at timestamp with time zone,
    CONSTRAINT commission_payment_targets_check CHECK (((state = 'paid'::text) = (paid_at IS NOT NULL))),
    CONSTRAINT commission_payment_targets_check1 CHECK (((state = 'paid'::text) = (audit_log_id IS NOT NULL))),
    CONSTRAINT commission_payment_targets_check2 CHECK (((state <> 'pending'::text) OR (ledger_entry_id IS NULL))),
    CONSTRAINT commission_payment_targets_check3 CHECK (((state <> 'paid'::text) OR ((points = 0) = (ledger_entry_id IS NULL)))),
    CONSTRAINT commission_payment_targets_payment_version_check CHECK ((payment_version > 0)),
    CONSTRAINT commission_payment_targets_points_check CHECK ((points >= 0)),
    CONSTRAINT commission_payment_targets_state_check CHECK ((state = ANY (ARRAY['pending'::text, 'paid'::text])))
);

CREATE TABLE commission_payments (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    cycle_id uuid NOT NULL,
    run_id uuid NOT NULL,
    evidence_epoch bigint NOT NULL,
    payout_mode text NOT NULL,
    state text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    total_points numeric NOT NULL,
    target_count bigint NOT NULL,
    approved_by uuid,
    approval_actor_type text,
    approval_audit_log_id uuid,
    creation_audit_log_id uuid NOT NULL,
    last_audit_log_id uuid NOT NULL,
    last_error_code text,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    next_work_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT commission_payments_approval_actor_type_check CHECK ((approval_actor_type = ANY (ARRAY['admin'::text, 'system'::text]))),
    CONSTRAINT commission_payments_check CHECK (((NOT ((approval_actor_type = 'admin'::text) IS DISTINCT FROM (approved_by IS NOT NULL))) OR ((approval_actor_type IS NULL) AND (approved_by IS NULL)))),
    CONSTRAINT commission_payments_check1 CHECK (((approval_actor_type IS NULL) = (approval_audit_log_id IS NULL))),
    CONSTRAINT commission_payments_check2 CHECK (((state <> ALL (ARRAY['paying'::text, 'paid'::text, 'failed'::text])) OR (approval_audit_log_id IS NOT NULL))),
    CONSTRAINT commission_payments_check3 CHECK (((state = ANY (ARRAY['failed'::text, 'blocked'::text])) = (last_error_code IS NOT NULL))),
    CONSTRAINT commission_payments_evidence_epoch_check CHECK ((evidence_epoch >= 0)),
    CONSTRAINT commission_payments_payout_mode_check CHECK ((payout_mode = ANY (ARRAY['manual'::text, 'automatic'::text, 'mixed'::text, 'none'::text]))),
    CONSTRAINT commission_payments_state_check CHECK ((state = ANY (ARRAY['awaiting_approval'::text, 'paying'::text, 'paid'::text, 'failed'::text, 'stale'::text, 'blocked'::text]))),
    CONSTRAINT commission_payments_target_count_check CHECK ((target_count >= 0)),
    CONSTRAINT commission_payments_total_points_check CHECK (((total_points >= (0)::numeric) AND (scale(total_points) = 0))),
    CONSTRAINT commission_payments_version_check CHECK (((version >= 1) AND (version <= '9007199254740991'::bigint)))
);

CREATE TABLE commission_policy_revisions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    brand_id uuid NOT NULL,
    version bigint NOT NULL,
    config jsonb NOT NULL,
    changed_by uuid,
    reason text NOT NULL,
    audit_log_id uuid,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT commission_policy_revisions_check CHECK (((version = 1) = ((changed_by IS NULL) AND (audit_log_id IS NULL)))),
    CONSTRAINT commission_policy_revisions_check1 CHECK (((version = 1) OR ((changed_by IS NOT NULL) AND (audit_log_id IS NOT NULL)))),
    CONSTRAINT commission_policy_revisions_config_check CHECK ((valid_commission_policy(config) IS TRUE)),
    CONSTRAINT commission_policy_revisions_reason_check CHECK (((length(TRIM(BOTH FROM reason)) > 0) AND (octet_length(reason) <= 500))),
    CONSTRAINT commission_policy_revisions_version_check CHECK ((version > 0))
);

CREATE TABLE commission_runs (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    cycle_id uuid NOT NULL,
    generation bigint NOT NULL,
    evidence_epoch bigint NOT NULL,
    state text NOT NULL,
    cursor_at timestamp with time zone,
    cursor_order_id uuid,
    earnings_cursor_agent uuid,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT commission_runs_check CHECK (((cursor_at IS NULL) = (cursor_order_id IS NULL))),
    CONSTRAINT commission_runs_evidence_epoch_check CHECK ((evidence_epoch >= 0)),
    CONSTRAINT commission_runs_generation_check CHECK ((generation > 0)),
    CONSTRAINT commission_runs_state_check CHECK ((state = ANY (ARRAY['calculating'::text, 'summarizing'::text, 'ready'::text, 'abandoned'::text])))
);

CREATE TABLE compliance_decisions (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    policy_version bigint NOT NULL,
    config jsonb NOT NULL,
    operation text NOT NULL,
    decision text NOT NULL,
    checks jsonb NOT NULL,
    adapter_mode text NOT NULL,
    created_by uuid NOT NULL,
    reason text NOT NULL,
    audit_log_id uuid NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT compliance_decisions_adapter_mode_check CHECK ((adapter_mode = 'stub'::text)),
    CONSTRAINT compliance_decisions_check CHECK ((checks = compliance_stub_checks(config))),
    CONSTRAINT compliance_decisions_check1 CHECK ((decision =
CASE
    WHEN (((config -> 'age_enabled'::text) = 'true'::jsonb) OR ((config -> 'region_enabled'::text) = 'true'::jsonb) OR ((config -> 'identity_enabled'::text) = 'true'::jsonb)) THEN 'review'::text
    ELSE 'allow'::text
END)),
    CONSTRAINT compliance_decisions_config_check CHECK ((valid_compliance_config(config) IS TRUE)),
    CONSTRAINT compliance_decisions_decision_check CHECK ((decision = ANY (ARRAY['allow'::text, 'review'::text, 'deny'::text, 'freeze'::text]))),
    CONSTRAINT compliance_decisions_operation_check CHECK ((operation = ANY (ARRAY['registration'::text, 'betting'::text, 'withdrawal'::text]))),
    CONSTRAINT compliance_decisions_reason_check CHECK (((length(TRIM(BOTH FROM reason)) > 0) AND (octet_length(reason) <= 500)))
);

CREATE TABLE compliance_gate_rejections (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    policy_version bigint NOT NULL,
    config jsonb NOT NULL,
    operation text NOT NULL,
    action text NOT NULL,
    decision text NOT NULL,
    checks jsonb NOT NULL,
    adapter_mode text NOT NULL,
    actor_type text NOT NULL,
    actor_id uuid,
    member_id uuid,
    request_id text NOT NULL,
    audit_log_id uuid NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT compliance_gate_rejections_action_check CHECK ((action = ANY (ARRAY['register'::text, 'join'::text, 'operator_join'::text, 'bet_preview'::text, 'bet_place'::text]))),
    CONSTRAINT compliance_gate_rejections_actor_type_check CHECK ((actor_type = ANY (ARRAY['anonymous'::text, 'user'::text, 'admin'::text]))),
    CONSTRAINT compliance_gate_rejections_adapter_mode_check CHECK ((adapter_mode = 'stub'::text)),
    CONSTRAINT compliance_gate_rejections_check CHECK ((checks = compliance_stub_checks(config))),
    CONSTRAINT compliance_gate_rejections_check1 CHECK ((((operation = 'registration'::text) AND (action = ANY (ARRAY['register'::text, 'join'::text, 'operator_join'::text]))) OR ((operation = 'betting'::text) AND (action = ANY (ARRAY['bet_preview'::text, 'bet_place'::text]))))),
    CONSTRAINT compliance_gate_rejections_check2 CHECK ((((actor_type = 'anonymous'::text) AND (action = 'register'::text) AND (actor_id IS NULL) AND (member_id IS NULL)) OR ((actor_type = 'admin'::text) AND (action = 'operator_join'::text) AND (actor_id IS NOT NULL) AND (member_id IS NULL)) OR ((actor_type = 'user'::text) AND (actor_id IS NOT NULL) AND ((action = 'join'::text) OR ((action = ANY (ARRAY['bet_preview'::text, 'bet_place'::text])) AND (member_id IS NOT NULL)))))),
    CONSTRAINT compliance_gate_rejections_config_check CHECK ((valid_compliance_config(config) IS TRUE)),
    CONSTRAINT compliance_gate_rejections_config_check1 CHECK ((((config -> 'age_enabled'::text) = 'true'::jsonb) OR ((config -> 'region_enabled'::text) = 'true'::jsonb) OR ((config -> 'identity_enabled'::text) = 'true'::jsonb))),
    CONSTRAINT compliance_gate_rejections_decision_check CHECK ((decision = 'review'::text)),
    CONSTRAINT compliance_gate_rejections_operation_check CHECK ((operation = ANY (ARRAY['registration'::text, 'betting'::text]))),
    CONSTRAINT compliance_gate_rejections_request_id_check CHECK (((length(request_id) >= 1) AND (length(request_id) <= 80)))
);

CREATE TABLE compliance_policy_revisions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    brand_id uuid NOT NULL,
    version bigint NOT NULL,
    config jsonb NOT NULL,
    changed_by uuid,
    reason text NOT NULL,
    audit_log_id uuid,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT compliance_policy_revisions_check CHECK (((version = 1) = ((changed_by IS NULL) AND (audit_log_id IS NULL)))),
    CONSTRAINT compliance_policy_revisions_check1 CHECK (((version = 1) OR ((changed_by IS NOT NULL) AND (audit_log_id IS NOT NULL)))),
    CONSTRAINT compliance_policy_revisions_config_check CHECK ((valid_compliance_config(config) IS TRUE)),
    CONSTRAINT compliance_policy_revisions_reason_check CHECK (((length(TRIM(BOTH FROM reason)) > 0) AND (octet_length(reason) <= 500))),
    CONSTRAINT compliance_policy_revisions_version_check CHECK ((version > 0))
);

CREATE TABLE consumed_events (
    consumer text NOT NULL,
    event_id uuid NOT NULL,
    consumed_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE draw_attempt_batches (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    game_id uuid NOT NULL,
    period_id uuid NOT NULL,
    source_set_id uuid NOT NULL,
    observed_period_version bigint NOT NULL,
    status text NOT NULL,
    attempts jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT draw_attempt_batches_attempts_check CHECK (((jsonb_typeof(attempts) = 'array'::text) AND (jsonb_array_length(attempts) <= 16))),
    CONSTRAINT draw_attempt_batches_observed_period_version_check CHECK ((observed_period_version > 0)),
    CONSTRAINT draw_attempt_batches_status_check CHECK ((status = ANY (ARRAY['accepted'::text, 'no_data'::text, 'failed'::text, 'discarded'::text])))
);

CREATE TABLE draw_correction_failures (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    correction_id uuid NOT NULL,
    order_id uuid,
    correction_version bigint NOT NULL,
    error_code text NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL
);

CREATE TABLE draw_corrections (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    game_id uuid NOT NULL,
    period_id uuid NOT NULL,
    previous_draw_result_id uuid NOT NULL,
    draw_result_id uuid NOT NULL,
    period_version bigint NOT NULL,
    previous_job_id uuid,
    new_job_id uuid,
    policy_version bigint,
    mode text,
    state text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    target_count bigint NOT NULL,
    created_by uuid NOT NULL,
    reason text NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    completed_at timestamp with time zone,
    last_error_code text,
    CONSTRAINT draw_corrections_check CHECK ((previous_draw_result_id <> draw_result_id)),
    CONSTRAINT draw_corrections_check1 CHECK (((state = 'completed'::text) = (completed_at IS NOT NULL))),
    CONSTRAINT draw_corrections_check2 CHECK (((state = 'failed'::text) = (last_error_code IS NOT NULL))),
    CONSTRAINT draw_corrections_check3 CHECK ((((previous_job_id IS NULL) AND (new_job_id IS NULL) AND (policy_version IS NULL) AND (mode IS NULL) AND (state = 'completed'::text) AND (target_count = 0)) OR ((previous_job_id IS NOT NULL) AND (policy_version IS NOT NULL) AND (mode IS NOT NULL)))),
    CONSTRAINT draw_corrections_check4 CHECK (((state <> 'resettling'::text) OR (new_job_id IS NOT NULL))),
    CONSTRAINT draw_corrections_mode_check CHECK ((mode = ANY (ARRAY['automatic'::text, 'manual'::text]))),
    CONSTRAINT draw_corrections_period_version_check CHECK ((period_version > 1)),
    CONSTRAINT draw_corrections_reason_check CHECK (((length(btrim(reason)) > 0) AND (octet_length(reason) <= 500))),
    CONSTRAINT draw_corrections_state_check CHECK ((state = ANY (ARRAY['reversing'::text, 'resettling'::text, 'failed'::text, 'completed'::text]))),
    CONSTRAINT draw_corrections_target_count_check CHECK ((target_count >= 0)),
    CONSTRAINT draw_corrections_version_check CHECK ((version > 0))
);

CREATE TABLE draw_notification_publications (
    draw_result_id uuid NOT NULL,
    brand_id uuid NOT NULL,
    game_id uuid NOT NULL,
    period_id uuid NOT NULL,
    event_type text NOT NULL,
    period_no text NOT NULL,
    result jsonb NOT NULL,
    drawn_at timestamp with time zone NOT NULL,
    previous_draw_id uuid,
    audit_log_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    creation_xid xid8 DEFAULT pg_current_xact_id() NOT NULL,
    CONSTRAINT draw_notification_publications_check CHECK (((event_type = 'draw.result.published'::text) = (previous_draw_id IS NULL))),
    CONSTRAINT draw_notification_publications_event_type_check CHECK ((event_type = ANY (ARRAY['draw.result.published'::text, 'draw.result.corrected'::text])))
);

CREATE TABLE draw_notification_recipients (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    draw_result_id uuid NOT NULL,
    member_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    creation_xid xid8 DEFAULT pg_current_xact_id() NOT NULL
);

CREATE TABLE draw_results (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    game_id uuid NOT NULL,
    period_id uuid NOT NULL,
    source_id uuid NOT NULL,
    kind text NOT NULL,
    result jsonb NOT NULL,
    result_hash text NOT NULL,
    drawn_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    created_by uuid,
    corrected_from_id uuid,
    CONSTRAINT draw_results_check CHECK (((kind = 'manual'::text) = (created_by IS NOT NULL))),
    CONSTRAINT draw_results_kind_check CHECK ((kind = ANY (ARRAY['api'::text, 'dom'::text, 'manual'::text]))),
    CONSTRAINT draw_results_result_check CHECK ((jsonb_typeof(result) = 'object'::text)),
    CONSTRAINT draw_results_result_hash_check CHECK ((result_hash ~ '^[0-9a-f]{64}$'::text))
);

CREATE TABLE draw_source_sets (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    game_id uuid NOT NULL,
    revision bigint NOT NULL,
    sources jsonb NOT NULL,
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT draw_source_sets_revision_check CHECK ((revision > 0)),
    CONSTRAINT draw_source_sets_sources_check CHECK (((jsonb_typeof(sources) = 'array'::text) AND (jsonb_array_length(sources) <= 16)))
);

CREATE TABLE draw_sources (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    game_id uuid NOT NULL,
    type text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT draw_sources_type_check CHECK ((type = ANY (ARRAY['api'::text, 'dom'::text, 'manual'::text])))
);

CREATE TABLE game_bet_policies (
    brand_id uuid NOT NULL,
    game_id uuid NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    config jsonb NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT game_bet_policies_config_check CHECK ((jsonb_typeof(config) = 'object'::text)),
    CONSTRAINT game_bet_policies_version_check CHECK ((version > 0))
);

CREATE TABLE game_schedules (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    game_id uuid NOT NULL,
    revision bigint NOT NULL,
    spec jsonb NOT NULL,
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT game_schedules_revision_check CHECK ((revision > 0)),
    CONSTRAINT game_schedules_spec_check CHECK ((jsonb_typeof(spec) = 'object'::text))
);

CREATE TABLE game_withdrawal_policies (
    brand_id uuid NOT NULL,
    game_id uuid NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    config jsonb DEFAULT '{"turnover_multiple": null}'::jsonb NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT game_withdrawal_policies_config_check CHECK (valid_withdrawal_policy(config, true)),
    CONSTRAINT game_withdrawal_policies_version_check CHECK ((version > 0))
);

CREATE TABLE games (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    code text NOT NULL,
    name text NOT NULL,
    model jsonb NOT NULL,
    timezone text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    started_sequence bigint DEFAULT 0 NOT NULL,
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    schedule_id uuid,
    draw_source_set_id uuid,
    CONSTRAINT games_code_check CHECK ((code ~ '^[a-z][a-z0-9_]{0,47}$'::text)),
    CONSTRAINT games_model_check CHECK (((jsonb_typeof(model) = 'object'::text) AND ((model ->> 'model'::text) = ANY (ARRAY['X_PLUS_Y'::text, 'M_SELECT_N'::text, 'DIGITS_0_9'::text])))),
    CONSTRAINT games_name_check CHECK (((length(name) >= 1) AND (length(name) <= 120))),
    CONSTRAINT games_started_sequence_check CHECK ((started_sequence >= 0)),
    CONSTRAINT games_status_check CHECK ((status = ANY (ARRAY['active'::text, 'paused'::text, 'disabled'::text]))),
    CONSTRAINT games_version_check CHECK ((version > 0))
);

CREATE TABLE global_users (
    id uuid NOT NULL,
    username text,
    phone text,
    password_hash text,
    telegram_user_id text,
    status text DEFAULT 'active'::text NOT NULL,
    username_set_at timestamp with time zone,
    phone_set_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT global_users_check CHECK (((username IS NOT NULL) OR (phone IS NOT NULL) OR (telegram_user_id IS NOT NULL))),
    CONSTRAINT global_users_status_check CHECK ((status = ANY (ARRAY['active'::text, 'disabled'::text, 'deleted'::text]))),
    CONSTRAINT global_users_username_check CHECK ((username = lower(username)))
);

CREATE TABLE idempotency_requests (
    brand_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    operation text NOT NULL,
    key text NOT NULL,
    request_hash text NOT NULL,
    status_code integer,
    response jsonb,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE join_code_revisions (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    code_id uuid NOT NULL,
    version bigint NOT NULL,
    status text NOT NULL,
    starts_at timestamp with time zone,
    expires_at timestamp with time zone,
    actor_id uuid NOT NULL,
    reason text NOT NULL,
    audit_log_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT join_code_revisions_reason_check CHECK (((length(TRIM(BOTH FROM reason)) > 0) AND (octet_length(reason) <= 500))),
    CONSTRAINT join_code_revisions_status_check CHECK ((status = ANY (ARRAY['active'::text, 'disabled'::text]))),
    CONSTRAINT join_code_revisions_version_check CHECK ((version > 0))
);

CREATE TABLE notification_deliveries (
    event_id uuid NOT NULL,
    brand_id uuid NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    attempt_count integer DEFAULT 0 NOT NULL,
    last_error text,
    next_attempt_at timestamp with time zone DEFAULT now() NOT NULL,
    sent_at timestamp with time zone,
    CONSTRAINT notification_deliveries_attempt_count_check CHECK ((attempt_count >= 0)),
    CONSTRAINT notification_deliveries_check CHECK (((status = 'sent'::text) = (sent_at IS NOT NULL))),
    CONSTRAINT notification_deliveries_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'sent'::text, 'failed'::text])))
);

CREATE TABLE notification_template_revisions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    brand_id uuid NOT NULL,
    template_key text NOT NULL,
    version bigint NOT NULL,
    content jsonb NOT NULL,
    changed_by uuid,
    reason text NOT NULL,
    audit_log_id uuid,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT notification_template_revisions_check CHECK ((valid_notification_template_content(template_key, content) IS TRUE)),
    CONSTRAINT notification_template_revisions_check1 CHECK (((version = 1) = ((changed_by IS NULL) AND (audit_log_id IS NULL)))),
    CONSTRAINT notification_template_revisions_check2 CHECK (((version = 1) OR ((changed_by IS NOT NULL) AND (audit_log_id IS NOT NULL)))),
    CONSTRAINT notification_template_revisions_reason_check CHECK (((length(TRIM(BOTH FROM reason)) > 0) AND (octet_length(reason) <= 500))),
    CONSTRAINT notification_template_revisions_version_check CHECK (((version >= 1) AND (version <= '9007199254740991'::bigint)))
);

CREATE TABLE notification_templates (
    brand_id uuid NOT NULL,
    template_key text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    content jsonb NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT notification_templates_check CHECK ((valid_notification_template_content(template_key, content) IS TRUE)),
    CONSTRAINT notification_templates_version_check CHECK (((version >= 1) AND (version <= '9007199254740991'::bigint)))
);

CREATE TABLE notifications (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    member_id uuid NOT NULL,
    event_id uuid NOT NULL,
    event_type text NOT NULL,
    template_key text NOT NULL,
    template_version bigint NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    read_at timestamp with time zone,
    content jsonb NOT NULL,
    CONSTRAINT notification_content_shape CHECK ((content IS NOT NULL) AND (valid_notification_template_content(template_key, content) IS TRUE)),
    CONSTRAINT notifications_check CHECK ((template_key = event_type)),
    CONSTRAINT notifications_event_type_check CHECK ((event_type = ANY (ARRAY['member.joined'::text, 'recharge.confirmed'::text, 'bet.order.placed'::text, 'bet.order.cancelled'::text, 'bet.order.judged_cancelled'::text, 'bet.order.abnormal'::text, 'bet.order.won'::text, 'bet.order.prize_reversed'::text, 'withdrawal.order.reviewing'::text, 'withdrawal.order.processing'::text, 'withdrawal.order.paid'::text, 'withdrawal.order.rejected'::text, 'withdrawal.order.failed'::text, 'withdrawal.order.cancelled'::text, 'commission.paid'::text, 'commission.adjusted'::text, 'commission.corrected'::text, 'reward.order.granted'::text, 'reward.order.revocation_pending'::text, 'reward.order.revoked'::text, 'draw.result.published'::text, 'draw.result.corrected'::text]))),
    CONSTRAINT notifications_payload_check CHECK ((jsonb_typeof(payload) = 'object'::text)),
    CONSTRAINT notifications_template_version_check CHECK (((template_version >= 1) AND (template_version <= '9007199254740991'::bigint)))
);

CREATE TABLE outbox_events (
    id uuid NOT NULL,
    brand_id uuid,
    event_type text NOT NULL,
    aggregate_id uuid NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    published_at timestamp with time zone,
    attempts integer DEFAULT 0 NOT NULL
);

CREATE TABLE period_cancellation_failures (
    id uuid NOT NULL,
    cancellation_id uuid NOT NULL,
    order_id uuid NOT NULL,
    job_version bigint NOT NULL,
    code text NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT period_cancellation_failures_code_check CHECK (((length(code) > 0) AND (octet_length(code) <= 64))),
    CONSTRAINT period_cancellation_failures_job_version_check CHECK ((job_version > 0))
);

CREATE TABLE period_cancellation_targets (
    cancellation_id uuid NOT NULL,
    brand_id uuid NOT NULL,
    period_id uuid NOT NULL,
    order_id uuid NOT NULL,
    state text DEFAULT 'pending'::text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    refund_entry_id uuid,
    error_code text DEFAULT ''::text NOT NULL,
    CONSTRAINT period_cancellation_targets_check CHECK (((state = ANY (ARRAY['refunded'::text, 'already_refunded'::text])) = (refund_entry_id IS NOT NULL))),
    CONSTRAINT period_cancellation_targets_check1 CHECK (((state = 'failed'::text) = (error_code <> ''::text))),
    CONSTRAINT period_cancellation_targets_state_check CHECK ((state = ANY (ARRAY['pending'::text, 'refunded'::text, 'already_refunded'::text, 'failed'::text]))),
    CONSTRAINT period_cancellation_targets_version_check CHECK ((version > 0))
);

CREATE TABLE period_cancellations (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    game_id uuid NOT NULL,
    period_id uuid NOT NULL,
    period_version bigint NOT NULL,
    draw_result_id uuid,
    mode text NOT NULL,
    cause text NOT NULL,
    state text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    target_count bigint NOT NULL,
    reason text NOT NULL,
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    completed_at timestamp with time zone,
    last_error_code text DEFAULT ''::text NOT NULL,
    CONSTRAINT period_cancellations_cause_check CHECK ((cause = ANY (ARRAY['operator_cancel'::text, 'no_result'::text, 'invalid_result'::text]))),
    CONSTRAINT period_cancellations_check CHECK (((state = 'completed'::text) = (completed_at IS NOT NULL))),
    CONSTRAINT period_cancellations_check1 CHECK (((completed_at IS NULL) OR (completed_at >= created_at))),
    CONSTRAINT period_cancellations_check2 CHECK (((state = 'failed'::text) = (last_error_code <> ''::text))),
    CONSTRAINT period_cancellations_check3 CHECK ((((mode = 'bet_cancelled'::text) AND (cause = 'operator_cancel'::text)) OR ((mode = 'judged_cancelled'::text) AND (cause = ANY (ARRAY['no_result'::text, 'invalid_result'::text]))))),
    CONSTRAINT period_cancellations_check4 CHECK (((cause <> 'no_result'::text) OR (draw_result_id IS NULL))),
    CONSTRAINT period_cancellations_mode_check CHECK ((mode = ANY (ARRAY['bet_cancelled'::text, 'judged_cancelled'::text]))),
    CONSTRAINT period_cancellations_period_version_check CHECK ((period_version > 1)),
    CONSTRAINT period_cancellations_reason_check CHECK (((length(TRIM(BOTH FROM reason)) > 0) AND (octet_length(reason) <= 500))),
    CONSTRAINT period_cancellations_state_check CHECK ((state = ANY (ARRAY['processing'::text, 'failed'::text, 'completed'::text]))),
    CONSTRAINT period_cancellations_target_count_check CHECK ((target_count >= 0)),
    CONSTRAINT period_cancellations_version_check CHECK ((version > 0))
);

CREATE TABLE period_rule_versions (
    brand_id uuid NOT NULL,
    game_id uuid NOT NULL,
    period_id uuid NOT NULL,
    play_id uuid NOT NULL,
    rule_version_id uuid NOT NULL
);

CREATE TABLE periods (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    game_id uuid NOT NULL,
    period_no text NOT NULL,
    sequence bigint NOT NULL,
    bet_start_at timestamp with time zone NOT NULL,
    bet_end_at timestamp with time zone NOT NULL,
    draw_at timestamp with time zone NOT NULL,
    status text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    schedule_id uuid,
    state_reason text DEFAULT ''::text NOT NULL,
    draw_result_id uuid,
    draw_claim_token uuid,
    draw_claim_until timestamp with time zone,
    draw_next_poll_at timestamp with time zone,
    current_settlement_job_id uuid,
    current_correction_id uuid,
    CONSTRAINT periods_check CHECK (((bet_start_at < bet_end_at) AND (bet_end_at <= draw_at))),
    CONSTRAINT periods_check1 CHECK (((draw_claim_token IS NULL) = (draw_claim_until IS NULL))),
    CONSTRAINT periods_check2 CHECK (((status <> ALL (ARRAY['drawn'::text, 'settling'::text, 'settled'::text])) OR (draw_result_id IS NOT NULL))),
    CONSTRAINT periods_period_no_check CHECK (((length(period_no) >= 1) AND (length(period_no) <= 80))),
    CONSTRAINT periods_sequence_check CHECK ((sequence > 0)),
    CONSTRAINT periods_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'betting'::text, 'closed'::text, 'waiting_draw'::text, 'drawn'::text, 'settling'::text, 'settled'::text, 'bet_cancelled'::text, 'judged_cancelled'::text]))),
    CONSTRAINT periods_version_check CHECK ((version > 0))
);

CREATE TABLE permissions (
    key text NOT NULL
);

CREATE TABLE platform_domains (
    domain text NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    CONSTRAINT platform_domains_domain_check CHECK ((domain = lower(domain)))
);

CREATE TABLE platform_idempotency_requests (
    actor_id uuid NOT NULL,
    operation text NOT NULL,
    key text NOT NULL,
    request_hash text NOT NULL,
    status_code integer,
    response jsonb,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE play_definitions (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    game_id uuid NOT NULL,
    code text NOT NULL,
    name text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    active_version_id uuid,
    version bigint DEFAULT 1 NOT NULL,
    next_version_no bigint DEFAULT 1 NOT NULL,
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT play_definitions_code_check CHECK ((code ~ '^[a-z][a-z0-9_]{0,47}$'::text)),
    CONSTRAINT play_definitions_name_check CHECK (((length(name) >= 1) AND (length(name) <= 120))),
    CONSTRAINT play_definitions_next_version_no_check CHECK ((next_version_no > 0)),
    CONSTRAINT play_definitions_status_check CHECK ((status = ANY (ARRAY['active'::text, 'paused'::text, 'disabled'::text]))),
    CONSTRAINT play_definitions_version_check CHECK ((version > 0))
);

CREATE TABLE point_accounts (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    brand_member_id uuid NOT NULL,
    version bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT point_account_version_nonnegative CHECK ((version >= 0))
);

CREATE TABLE point_balance_repairs (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    account_id uuid NOT NULL,
    member_id uuid NOT NULL,
    version bigint NOT NULL,
    before_snapshot jsonb NOT NULL,
    after_snapshot jsonb NOT NULL,
    reason text NOT NULL,
    actor_id uuid NOT NULL,
    request_id text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT point_balance_repairs_version_check CHECK ((version >= 0))
);

CREATE TABLE point_buckets (
    brand_id uuid NOT NULL,
    account_id uuid NOT NULL,
    source text NOT NULL,
    state text NOT NULL,
    points bigint DEFAULT 0 NOT NULL,
    CONSTRAINT point_buckets_points_check CHECK ((points >= 0)),
    CONSTRAINT point_buckets_source_check CHECK ((source = ANY (ARRAY['recharge'::text, 'winning'::text, 'gift'::text, 'commission'::text]))),
    CONSTRAINT point_buckets_state_check CHECK ((state = ANY (ARRAY['available'::text, 'manual_frozen'::text, 'system_frozen'::text, 'withdrawal'::text])))
);

CREATE TABLE point_reconciliation_failures (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    job_id uuid NOT NULL,
    target_id uuid NOT NULL,
    attempt_count integer NOT NULL,
    error_code text NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    audit_log_id uuid NOT NULL,
    CONSTRAINT point_reconciliation_failures_attempt_count_check CHECK ((attempt_count > 0)),
    CONSTRAINT point_reconciliation_failures_error_code_check CHECK ((error_code = 'CHECK_FAILED'::text))
);

CREATE TABLE point_reconciliation_jobs (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    state text DEFAULT 'pending'::text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    target_count integer NOT NULL,
    created_by uuid NOT NULL,
    reason text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    last_step_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    started_at timestamp with time zone,
    completed_at timestamp with time zone,
    creation_audit_log_id uuid NOT NULL,
    creation_xid xid8 DEFAULT pg_current_xact_id() NOT NULL,
    last_failure_id uuid,
    check_scope text DEFAULT 'wallet'::text NOT NULL,
    CONSTRAINT point_reconciliation_jobs_check CHECK (((state = 'completed'::text) = (completed_at IS NOT NULL))),
    CONSTRAINT point_reconciliation_jobs_check1 CHECK (((state <> ALL (ARRAY['running'::text, 'completed'::text])) OR (started_at IS NOT NULL))),
    CONSTRAINT point_reconciliation_jobs_check2 CHECK (((state = 'failed'::text) = (last_failure_id IS NOT NULL))),
    CONSTRAINT point_reconciliation_jobs_check3 CHECK (((started_at IS NULL) OR (started_at >= created_at))),
    CONSTRAINT point_reconciliation_jobs_check4 CHECK (((completed_at IS NULL) OR (completed_at >= started_at))),
    CONSTRAINT point_reconciliation_jobs_check_scope_check CHECK ((check_scope = ANY (ARRAY['wallet'::text, 'wallet_and_business'::text]))),
    CONSTRAINT point_reconciliation_jobs_reason_check CHECK ((((octet_length(reason) >= 1) AND (octet_length(reason) <= 500)) AND (TRIM(BOTH FROM reason) = reason))),
    CONSTRAINT point_reconciliation_jobs_state_check CHECK ((state = ANY (ARRAY['pending'::text, 'running'::text, 'completed'::text, 'failed'::text]))),
    CONSTRAINT point_reconciliation_jobs_target_count_check CHECK (((target_count >= 0) AND (target_count <= 100000))),
    CONSTRAINT point_reconciliation_jobs_version_check CHECK (((version >= 1) AND (version <= '9007199254740991'::bigint)))
);

CREATE TABLE point_reconciliation_results (
    target_id uuid NOT NULL,
    brand_id uuid NOT NULL,
    job_id uuid NOT NULL,
    outcome text NOT NULL,
    preview jsonb NOT NULL,
    checked_at timestamp with time zone NOT NULL,
    audit_log_id uuid NOT NULL,
    business_preview jsonb,
    CONSTRAINT point_reconciliation_results_business_preview_check CHECK (((business_preview IS NULL) OR (jsonb_typeof(business_preview) = 'object'::text))),
    CONSTRAINT point_reconciliation_results_outcome_check CHECK ((outcome = ANY (ARRAY['consistent'::text, 'repairable'::text, 'corrupt'::text]))),
    CONSTRAINT point_reconciliation_results_preview_check CHECK ((jsonb_typeof(preview) = 'object'::text))
);

CREATE TABLE point_reconciliation_retries (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    job_id uuid NOT NULL,
    version bigint NOT NULL,
    previous_failure_id uuid NOT NULL,
    created_by uuid NOT NULL,
    reason text NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    audit_log_id uuid NOT NULL,
    CONSTRAINT point_reconciliation_retries_reason_check CHECK ((((octet_length(reason) >= 1) AND (octet_length(reason) <= 500)) AND (TRIM(BOTH FROM reason) = reason))),
    CONSTRAINT point_reconciliation_retries_version_check CHECK ((version > 1))
);

CREATE TABLE point_reconciliation_targets (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    job_id uuid NOT NULL,
    account_id uuid NOT NULL,
    member_id uuid NOT NULL,
    state text DEFAULT 'pending'::text NOT NULL,
    attempt_count integer DEFAULT 0 NOT NULL,
    next_check_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT point_reconciliation_targets_attempt_count_check CHECK ((attempt_count >= 0)),
    CONSTRAINT point_reconciliation_targets_state_check CHECK ((state = ANY (ARRAY['pending'::text, 'checked'::text, 'failed'::text])))
);

CREATE TABLE recharge_orders (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    member_id uuid NOT NULL,
    account_id uuid NOT NULL,
    points bigint NOT NULL,
    state text DEFAULT 'pending'::text NOT NULL,
    proof_reference text DEFAULT ''::text NOT NULL,
    remark text DEFAULT ''::text NOT NULL,
    created_by uuid NOT NULL,
    confirmed_by uuid,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    confirmed_at timestamp with time zone,
    ledger_entry_id uuid,
    CONSTRAINT recharge_orders_check CHECK ((((state = 'confirmed'::text) AND (confirmed_at IS NOT NULL) AND (confirmed_by IS NOT NULL) AND (ledger_entry_id IS NOT NULL)) OR ((state <> 'confirmed'::text) AND (confirmed_at IS NULL) AND (confirmed_by IS NULL) AND (ledger_entry_id IS NULL)))),
    CONSTRAINT recharge_orders_points_check CHECK ((points > 0)),
    CONSTRAINT recharge_orders_state_check CHECK ((state = ANY (ARRAY['pending'::text, 'confirmed'::text, 'cancelled'::text]))),
    CONSTRAINT recharge_orders_version_check CHECK ((version > 0))
);

CREATE TABLE report_archive_automatic_cursors (
    brand_id uuid NOT NULL,
    policy_version bigint NOT NULL,
    kind text NOT NULL,
    next_period_key text,
    due_at timestamp with time zone,
    CONSTRAINT report_archive_automatic_cursors_kind_check CHECK ((kind = ANY (ARRAY['daily'::text, 'monthly'::text])))
);

CREATE TABLE report_archive_automatic_tasks (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    policy_version bigint NOT NULL,
    kind text NOT NULL,
    period_key text NOT NULL,
    timezone text NOT NULL,
    from_at timestamp with time zone NOT NULL,
    to_at timestamp with time zone NOT NULL,
    state text DEFAULT 'pending'::text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    attempt_count bigint DEFAULT 0 NOT NULL,
    archive_id uuid,
    last_error_code text,
    creation_audit_log_id uuid NOT NULL,
    last_audit_log_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT statement_timestamp() NOT NULL,
    updated_at timestamp with time zone DEFAULT statement_timestamp() NOT NULL,
    CONSTRAINT report_archive_automatic_tasks_attempt_count_check CHECK (((attempt_count >= 0) AND (attempt_count <= '9007199254740991'::bigint))),
    CONSTRAINT report_archive_automatic_tasks_check CHECK ((to_at > from_at)),
    CONSTRAINT report_archive_automatic_tasks_check1 CHECK (((state = ANY (ARRAY['completed'::text, 'skipped'::text])) = (archive_id IS NOT NULL))),
    CONSTRAINT report_archive_automatic_tasks_check2 CHECK (((state = 'failed'::text) = (last_error_code IS NOT NULL))),
    CONSTRAINT report_archive_automatic_tasks_kind_check CHECK ((kind = ANY (ARRAY['daily'::text, 'monthly'::text]))),
    CONSTRAINT report_archive_automatic_tasks_last_error_code_check CHECK ((last_error_code = 'ARCHIVE_FAILED'::text)),
    CONSTRAINT report_archive_automatic_tasks_state_check CHECK ((state = ANY (ARRAY['pending'::text, 'completed'::text, 'skipped'::text, 'failed'::text]))),
    CONSTRAINT report_archive_automatic_tasks_version_check CHECK (((version >= 1) AND (version <= '9007199254740991'::bigint)))
);

CREATE TABLE report_archive_policy_revisions (
    brand_id uuid NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    daily_enabled boolean DEFAULT false NOT NULL,
    monthly_enabled boolean DEFAULT false NOT NULL,
    daily_start_period text,
    monthly_start_period text,
    timezone text NOT NULL,
    audit_log_id uuid,
    updated_at timestamp with time zone DEFAULT statement_timestamp() NOT NULL,
    CONSTRAINT brand_report_archive_policies_check CHECK (((NOT daily_enabled) OR (daily_start_period IS NOT NULL))),
    CONSTRAINT brand_report_archive_policies_check1 CHECK (((NOT monthly_enabled) OR (monthly_start_period IS NOT NULL))),
    CONSTRAINT brand_report_archive_policies_version_check CHECK (((version >= 1) AND (version <= '9007199254740991'::bigint)))
);

CREATE TABLE reward_order_actions (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    order_id uuid NOT NULL,
    version bigint NOT NULL,
    operation text NOT NULL,
    state_before text,
    state_after text NOT NULL,
    actor_id uuid NOT NULL,
    reason text NOT NULL,
    audit_log_id uuid NOT NULL,
    ledger_entry_id uuid,
    creation_xid xid8 DEFAULT pg_current_xact_id() NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT reward_order_actions_check CHECK ((((operation = 'grant'::text) AND (version = 1) AND (state_before IS NULL) AND (state_after = 'granted'::text)) OR ((operation = 'revoke'::text) AND (version > 1) AND (state_before = 'granted'::text) AND (state_after = ANY (ARRAY['revocation_pending'::text, 'revoked'::text]))) OR ((operation = 'retry'::text) AND (version > 1) AND (state_before = 'revocation_pending'::text) AND (state_after = ANY (ARRAY['revocation_pending'::text, 'revoked'::text]))))),
    CONSTRAINT reward_order_actions_operation_check CHECK ((operation = ANY (ARRAY['grant'::text, 'revoke'::text, 'retry'::text]))),
    CONSTRAINT reward_order_actions_reason_check CHECK (((length(reason) > 0) AND (octet_length(reason) <= 500))),
    CONSTRAINT reward_order_actions_state_after_check CHECK ((state_after = ANY (ARRAY['granted'::text, 'revocation_pending'::text, 'revoked'::text]))),
    CONSTRAINT reward_order_actions_version_check CHECK (((version >= 1) AND (version <= '9007199254740991'::bigint)))
);

CREATE TABLE reward_orders (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    member_id uuid NOT NULL,
    points bigint NOT NULL,
    state text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    grant_ledger_entry_id uuid,
    revoke_ledger_entry_id uuid,
    creation_audit_log_id uuid NOT NULL,
    last_audit_log_id uuid NOT NULL,
    created_by uuid NOT NULL,
    reason text NOT NULL,
    point_policy_version bigint NOT NULL,
    last_error_code text,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    revoked_at timestamp with time zone,
    creation_xid xid8 DEFAULT pg_current_xact_id() NOT NULL,
    CONSTRAINT reward_orders_check CHECK (((state = 'revoked'::text) = (revoke_ledger_entry_id IS NOT NULL))),
    CONSTRAINT reward_orders_check1 CHECK (((state = 'revoked'::text) = (revoked_at IS NOT NULL))),
    CONSTRAINT reward_orders_check2 CHECK (((state = 'revocation_pending'::text) = (last_error_code IS NOT NULL))),
    CONSTRAINT reward_orders_check3 CHECK (((state <> 'granted'::text) OR (version = 1))),
    CONSTRAINT reward_orders_last_error_code_check CHECK (((last_error_code IS NULL) OR (last_error_code = 'REWARD_AVAILABLE_INSUFFICIENT'::text))),
    CONSTRAINT reward_orders_point_policy_version_check CHECK ((point_policy_version > 0)),
    CONSTRAINT reward_orders_points_check CHECK ((points > 0)),
    CONSTRAINT reward_orders_reason_check CHECK (((length(reason) > 0) AND (octet_length(reason) <= 500))),
    CONSTRAINT reward_orders_state_check CHECK ((state = ANY (ARRAY['granted'::text, 'revocation_pending'::text, 'revoked'::text]))),
    CONSTRAINT reward_orders_version_check CHECK (((version >= 1) AND (version <= '9007199254740991'::bigint)))
);

CREATE TABLE role_permissions (
    role_id uuid NOT NULL,
    permission_key text NOT NULL
);

CREATE TABLE roles (
    id uuid NOT NULL,
    code text NOT NULL,
    name text NOT NULL,
    brand_id uuid,
    status text DEFAULT 'active'::text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    is_bootstrap boolean DEFAULT false NOT NULL,
    CONSTRAINT roles_status_check CHECK ((status = ANY (ARRAY['active'::text, 'disabled'::text]))),
    CONSTRAINT roles_version_check CHECK ((version > 0))
);

CREATE TABLE rule_version_contributors (
    rule_version_id uuid NOT NULL,
    admin_id uuid NOT NULL
);

CREATE TABLE rule_versions (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    game_id uuid NOT NULL,
    play_id uuid NOT NULL,
    version_no bigint NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    definition jsonb NOT NULL,
    definition_sha256 text NOT NULL,
    status text DEFAULT 'draft'::text NOT NULL,
    effect_mode text NOT NULL,
    validation jsonb,
    created_by uuid NOT NULL,
    reviewed_by uuid,
    review_comment text DEFAULT ''::text NOT NULL,
    reviewed_at timestamp with time zone,
    effective_at timestamp with time zone,
    effective_period_id uuid,
    effective_sequence bigint,
    source_version_id uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT rule_versions_check CHECK (((reviewed_by IS NULL) OR (reviewed_by <> created_by))),
    CONSTRAINT rule_versions_check1 CHECK ((((status = ANY (ARRAY['draft'::text, 'pending_review'::text])) AND (reviewed_by IS NULL) AND (reviewed_at IS NULL)) OR ((status <> ALL (ARRAY['draft'::text, 'pending_review'::text])) AND (reviewed_by IS NOT NULL) AND (reviewed_at IS NOT NULL)))),
    CONSTRAINT rule_versions_check2 CHECK (((status <> ALL (ARRAY['pending_review'::text, 'approved'::text, 'active'::text, 'expired'::text, 'rolled_back'::text])) OR ((((validation ->> 'passed'::text) = 'true'::text) AND ((validation ->> 'definition_hash'::text) = definition_sha256)) IS TRUE))),
    CONSTRAINT rule_versions_check3 CHECK (((status <> ALL (ARRAY['active'::text, 'expired'::text, 'rolled_back'::text])) OR (effective_at IS NOT NULL))),
    CONSTRAINT rule_versions_check4 CHECK (((status <> 'approved'::text) OR (((effect_mode = 'next_period'::text) AND (effective_sequence > 0) AND (effective_at IS NULL)) IS TRUE))),
    CONSTRAINT rule_versions_definition_check CHECK ((jsonb_typeof(definition) = 'object'::text)),
    CONSTRAINT rule_versions_definition_sha256_check CHECK ((definition_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT rule_versions_effect_mode_check CHECK ((effect_mode = ANY (ARRAY['immediate'::text, 'next_period'::text]))),
    CONSTRAINT rule_versions_status_check CHECK ((status = ANY (ARRAY['draft'::text, 'pending_review'::text, 'approved'::text, 'active'::text, 'expired'::text, 'rejected'::text, 'rolled_back'::text]))),
    CONSTRAINT rule_versions_version_check CHECK ((version > 0)),
    CONSTRAINT rule_versions_version_no_check CHECK ((version_no > 0))
);

CREATE TABLE sessions (
    id uuid NOT NULL,
    token_hash text NOT NULL,
    user_id uuid,
    member_id uuid,
    admin_id uuid,
    brand_id uuid,
    expires_at timestamp with time zone NOT NULL,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT sessions_check CHECK ((((user_id IS NOT NULL) AND (member_id IS NOT NULL) AND (admin_id IS NULL) AND (brand_id IS NOT NULL)) OR ((admin_id IS NOT NULL) AND (user_id IS NULL) AND (member_id IS NULL) AND (brand_id IS NULL))))
);

CREATE TABLE settlement_calculations (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    job_id uuid NOT NULL,
    period_id uuid NOT NULL,
    order_id uuid NOT NULL,
    order_version bigint NOT NULL,
    definition_hash text NOT NULL,
    draw_hash text NOT NULL,
    won boolean NOT NULL,
    prize_points bigint NOT NULL,
    calculation jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT settlement_calculations_order_version_check CHECK ((order_version > 0)),
    CONSTRAINT settlement_calculations_prize_points_check CHECK ((prize_points >= 0))
);

CREATE TABLE settlement_failures (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    job_id uuid NOT NULL,
    order_id uuid,
    job_version bigint NOT NULL,
    phase text NOT NULL,
    error_code text NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT settlement_failures_phase_check CHECK ((phase = ANY (ARRAY['processing'::text, 'paying'::text])))
);

CREATE TABLE settlement_jobs (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    game_id uuid NOT NULL,
    period_id uuid NOT NULL,
    draw_result_id uuid NOT NULL,
    period_version bigint NOT NULL,
    policy_version bigint NOT NULL,
    mode text NOT NULL,
    state text DEFAULT 'processing'::text NOT NULL,
    resume_state text,
    version bigint DEFAULT 1 NOT NULL,
    target_count bigint NOT NULL,
    created_by uuid NOT NULL,
    approved_by uuid,
    reason text NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    completed_at timestamp with time zone,
    last_error_code text,
    generation bigint DEFAULT 1 NOT NULL,
    previous_job_id uuid,
    correction_id uuid,
    CONSTRAINT settlement_jobs_check CHECK (((state = 'completed'::text) = (completed_at IS NOT NULL))),
    CONSTRAINT settlement_jobs_check1 CHECK (((state = 'failed'::text) = (last_error_code IS NOT NULL))),
    CONSTRAINT settlement_jobs_check2 CHECK (((state = 'failed'::text) = (resume_state IS NOT NULL))),
    CONSTRAINT settlement_jobs_check3 CHECK (((mode = 'manual'::text) OR (approved_by IS NULL))),
    CONSTRAINT settlement_jobs_check4 CHECK (((state <> 'paying'::text) OR (mode = 'automatic'::text) OR (approved_by IS NOT NULL))),
    CONSTRAINT settlement_jobs_check5 CHECK ((((generation = 1) AND (previous_job_id IS NULL) AND (correction_id IS NULL)) OR ((generation > 1) AND (previous_job_id IS NOT NULL) AND (correction_id IS NOT NULL)))),
    CONSTRAINT settlement_jobs_generation_check CHECK ((generation > 0)),
    CONSTRAINT settlement_jobs_mode_check CHECK ((mode = ANY (ARRAY['automatic'::text, 'manual'::text]))),
    CONSTRAINT settlement_jobs_period_version_check CHECK ((period_version > 1)),
    CONSTRAINT settlement_jobs_policy_version_check CHECK ((policy_version > 1)),
    CONSTRAINT settlement_jobs_reason_check CHECK (((length(btrim(reason)) > 0) AND (octet_length(reason) <= 500))),
    CONSTRAINT settlement_jobs_resume_state_check CHECK ((resume_state = ANY (ARRAY['processing'::text, 'paying'::text]))),
    CONSTRAINT settlement_jobs_state_check CHECK ((state = ANY (ARRAY['processing'::text, 'awaiting_approval'::text, 'paying'::text, 'failed'::text, 'completed'::text]))),
    CONSTRAINT settlement_jobs_target_count_check CHECK ((target_count >= 0)),
    CONSTRAINT settlement_jobs_version_check CHECK ((version > 0))
);

CREATE TABLE settlement_policy_history (
    brand_id uuid NOT NULL,
    version bigint NOT NULL,
    mode text,
    changed_by uuid NOT NULL,
    reason text NOT NULL,
    audit_log_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT settlement_policy_history_mode_check CHECK (((mode IS NULL) OR (mode = ANY (ARRAY['automatic'::text, 'manual'::text])))),
    CONSTRAINT settlement_policy_history_version_check CHECK ((version > 1))
);

CREATE TABLE settlement_previews (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    game_id uuid NOT NULL,
    period_id uuid NOT NULL,
    order_id uuid NOT NULL,
    order_version bigint NOT NULL,
    order_status text NOT NULL,
    period_version bigint NOT NULL,
    period_status text NOT NULL,
    draw_result_id uuid NOT NULL,
    definition_hash text NOT NULL,
    draw_hash text NOT NULL,
    draw jsonb NOT NULL,
    outcome text NOT NULL,
    error_code text,
    calculation jsonb,
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    reason text NOT NULL,
    audit_log_id uuid NOT NULL,
    CONSTRAINT settlement_previews_check CHECK ((((outcome = ANY (ARRAY['won'::text, 'lost'::text])) AND (error_code IS NULL) AND (calculation IS NOT NULL) AND (jsonb_typeof(calculation) = 'object'::text)) OR ((outcome = ANY (ARRAY['abnormal'::text, 'excluded'::text])) AND (error_code IS NOT NULL) AND (calculation IS NULL)))),
    CONSTRAINT settlement_previews_definition_hash_check CHECK ((definition_hash ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT settlement_previews_draw_check CHECK ((jsonb_typeof(draw) = 'object'::text)),
    CONSTRAINT settlement_previews_draw_hash_check CHECK ((draw_hash ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT settlement_previews_order_version_check CHECK ((order_version > 0)),
    CONSTRAINT settlement_previews_outcome_check CHECK ((outcome = ANY (ARRAY['won'::text, 'lost'::text, 'abnormal'::text, 'excluded'::text]))),
    CONSTRAINT settlement_previews_period_status_check CHECK ((period_status = 'drawn'::text)),
    CONSTRAINT settlement_previews_period_version_check CHECK ((period_version > 0)),
    CONSTRAINT settlement_previews_reason_check CHECK (((length(btrim(reason)) > 0) AND (octet_length(reason) <= 500)))
);

CREATE TABLE settlement_targets (
    brand_id uuid NOT NULL,
    job_id uuid NOT NULL,
    period_id uuid NOT NULL,
    order_id uuid NOT NULL,
    state text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    calculation_id uuid,
    payout_entry_id uuid,
    error_code text,
    CONSTRAINT settlement_targets_check CHECK (((state <> ALL (ARRAY['ready'::text, 'paid'::text])) OR (calculation_id IS NOT NULL))),
    CONSTRAINT settlement_targets_check1 CHECK (((state = 'paid'::text) OR (payout_entry_id IS NULL))),
    CONSTRAINT settlement_targets_state_check CHECK ((state = ANY (ARRAY['pending'::text, 'ready'::text, 'paid'::text, 'excluded'::text, 'failed'::text]))),
    CONSTRAINT settlement_targets_version_check CHECK ((version > 0))
);

CREATE TABLE withdrawal_operation_receipts (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    order_id uuid NOT NULL,
    actor_type text NOT NULL,
    actor_id uuid NOT NULL,
    client_key text NOT NULL,
    request_hash text NOT NULL,
    response jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT withdrawal_operation_receipts_actor_type_check CHECK ((actor_type = ANY (ARRAY['user'::text, 'admin'::text]))),
    CONSTRAINT withdrawal_operation_receipts_client_key_check CHECK ((client_key ~ '^[a-zA-Z0-9_.:-]{8,128}$'::text)),
    CONSTRAINT withdrawal_operation_receipts_request_hash_check CHECK ((request_hash ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT withdrawal_operation_receipts_response_check CHECK ((jsonb_typeof(response) = 'object'::text))
);

CREATE TABLE withdrawal_order_transitions (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    order_id uuid NOT NULL,
    version bigint NOT NULL,
    from_state text NOT NULL,
    to_state text NOT NULL,
    reason text NOT NULL,
    actor_type text NOT NULL,
    actor_id uuid,
    audit_log_id uuid NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT withdrawal_order_transitions_actor_type_check CHECK ((actor_type = ANY (ARRAY['user'::text, 'admin'::text, 'system'::text]))),
    CONSTRAINT withdrawal_order_transitions_check CHECK (((actor_type = 'system'::text) OR (actor_id IS NOT NULL))),
    CONSTRAINT withdrawal_order_transitions_reason_check CHECK (((octet_length(reason) >= 1) AND (octet_length(reason) <= 500))),
    CONSTRAINT withdrawal_order_transitions_version_check CHECK (((version >= 1) AND (version <= '9007199254740991'::bigint)))
);

CREATE TABLE withdrawal_orders (
    id uuid NOT NULL,
    brand_id uuid NOT NULL,
    member_id uuid NOT NULL,
    account_id uuid NOT NULL,
    points bigint NOT NULL,
    state text NOT NULL,
    version bigint NOT NULL,
    source_allocation jsonb NOT NULL,
    policy_snapshot jsonb NOT NULL,
    eligibility_evidence jsonb NOT NULL,
    cycle_from_at timestamp with time zone,
    cycle_from_version bigint NOT NULL,
    reserve_version bigint NOT NULL,
    reserve_entry_id uuid NOT NULL,
    release_entry_id uuid,
    paid_entry_id uuid,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    reviewed_at timestamp with time zone,
    completed_at timestamp with time zone,
    decision_reason text DEFAULT ''::text NOT NULL,
    CONSTRAINT withdrawal_orders_check CHECK ((reserve_version > cycle_from_version)),
    CONSTRAINT withdrawal_orders_check1 CHECK (((cycle_from_at IS NULL) = (cycle_from_version = 0))),
    CONSTRAINT withdrawal_orders_check2 CHECK (((cycle_from_at IS NULL) OR (cycle_from_at <= created_at))),
    CONSTRAINT withdrawal_orders_check3 CHECK (((updated_at >= created_at) AND ((reviewed_at IS NULL) OR (reviewed_at >= created_at)) AND ((completed_at IS NULL) OR (completed_at >= created_at)))),
    CONSTRAINT withdrawal_orders_check4 CHECK (((state = ANY (ARRAY['paid'::text, 'rejected'::text, 'failed'::text, 'cancelled'::text])) = (completed_at IS NOT NULL))),
    CONSTRAINT withdrawal_orders_check5 CHECK (((state = 'paid'::text) = (paid_entry_id IS NOT NULL))),
    CONSTRAINT withdrawal_orders_check6 CHECK (((state = ANY (ARRAY['rejected'::text, 'failed'::text, 'cancelled'::text])) = (release_entry_id IS NOT NULL))),
    CONSTRAINT withdrawal_orders_check7 CHECK (((state <> ALL (ARRAY['processing'::text, 'paid'::text, 'failed'::text])) OR (reviewed_at IS NOT NULL))),
    CONSTRAINT withdrawal_orders_check8 CHECK (((state <> 'reviewing'::text) OR (reviewed_at IS NULL))),
    CONSTRAINT withdrawal_orders_cycle_from_version_check CHECK ((cycle_from_version >= 0)),
    CONSTRAINT withdrawal_orders_decision_reason_check CHECK ((octet_length(decision_reason) <= 500)),
    CONSTRAINT withdrawal_orders_eligibility_evidence_check CHECK (((jsonb_typeof(eligibility_evidence) = 'object'::text) AND (octet_length((eligibility_evidence)::text) <= 16384))),
    CONSTRAINT withdrawal_orders_points_check CHECK ((points > 0)),
    CONSTRAINT withdrawal_orders_policy_snapshot_check CHECK ((jsonb_typeof(policy_snapshot) = 'object'::text)),
    CONSTRAINT withdrawal_orders_source_allocation_check CHECK ((jsonb_typeof(source_allocation) = 'array'::text)),
    CONSTRAINT withdrawal_orders_state_check CHECK ((state = ANY (ARRAY['reviewing'::text, 'processing'::text, 'paid'::text, 'rejected'::text, 'failed'::text, 'cancelled'::text]))),
    CONSTRAINT withdrawal_orders_version_check CHECK (((version >= 1) AND (version <= '9007199254740991'::bigint)))
);

CREATE TABLE withdrawal_policy_revisions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    brand_id uuid NOT NULL,
    game_id uuid,
    version bigint NOT NULL,
    config jsonb NOT NULL,
    changed_by uuid,
    reason text NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT withdrawal_policy_revisions_check CHECK (valid_withdrawal_policy(config, (game_id IS NOT NULL))),
    CONSTRAINT withdrawal_policy_revisions_check1 CHECK (((version = 1) = (changed_by IS NULL))),
    CONSTRAINT withdrawal_policy_revisions_reason_check CHECK (((length(TRIM(BOTH FROM reason)) > 0) AND (octet_length(reason) <= 500))),
    CONSTRAINT withdrawal_policy_revisions_version_check CHECK ((version > 0))
);

CREATE TABLE withdrawal_turnover_cycles (
    brand_id uuid NOT NULL,
    member_id uuid NOT NULL,
    account_id uuid NOT NULL,
    cutoff_at timestamp with time zone NOT NULL,
    cutoff_version bigint NOT NULL,
    last_paid_order_id uuid NOT NULL,
    CONSTRAINT withdrawal_turnover_cycles_cutoff_version_check CHECK ((cutoff_version > 0))
);

ALTER TABLE ONLY admin_account_roles
    ADD CONSTRAINT admin_account_roles_pkey PRIMARY KEY (account_id, role_id);

ALTER TABLE ONLY admin_accounts
    ADD CONSTRAINT admin_accounts_pkey PRIMARY KEY (id);

ALTER TABLE ONLY admin_accounts
    ADD CONSTRAINT admin_accounts_username_key UNIQUE (username);

ALTER TABLE ONLY admin_brand_scopes
    ADD CONSTRAINT admin_brand_scopes_pkey PRIMARY KEY (account_id, brand_id);

ALTER TABLE ONLY agent_config_revisions
    ADD CONSTRAINT agent_config_revisions_brand_id_agent_id_version_key UNIQUE NULLS NOT DISTINCT (brand_id, agent_id, version);

ALTER TABLE ONLY agent_config_revisions
    ADD CONSTRAINT agent_config_revisions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY agent_nodes
    ADD CONSTRAINT agent_nodes_brand_id_id_key UNIQUE (brand_id, id);

ALTER TABLE ONLY agent_nodes
    ADD CONSTRAINT agent_nodes_brand_id_member_id_key UNIQUE (brand_id, member_id);

ALTER TABLE ONLY agent_nodes
    ADD CONSTRAINT agent_nodes_pkey PRIMARY KEY (id);

ALTER TABLE ONLY audit_logs
    ADD CONSTRAINT audit_logs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY auth_challenges
    ADD CONSTRAINT auth_challenges_pkey PRIMARY KEY (id);

ALTER TABLE ONLY auth_rate_limits
    ADD CONSTRAINT auth_rate_limits_pkey PRIMARY KEY (bucket_hash);

ALTER TABLE ONLY bet_order_exceptions
    ADD CONSTRAINT bet_order_exceptions_order_id_key UNIQUE (order_id);

ALTER TABLE ONLY bet_order_exceptions
    ADD CONSTRAINT bet_order_exceptions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY bet_order_judgments
    ADD CONSTRAINT bet_order_judgments_order_id_key UNIQUE (order_id);

ALTER TABLE ONLY bet_order_judgments
    ADD CONSTRAINT bet_order_judgments_pkey PRIMARY KEY (id);

ALTER TABLE ONLY bet_orders
    ADD CONSTRAINT bet_orders_brand_id_brand_member_id_client_key_key UNIQUE (brand_id, brand_member_id, client_key);

ALTER TABLE ONLY bet_orders
    ADD CONSTRAINT bet_orders_brand_id_id_key UNIQUE (brand_id, id);

ALTER TABLE ONLY bet_orders
    ADD CONSTRAINT bet_orders_brand_id_period_id_id_key UNIQUE (brand_id, period_id, id);

ALTER TABLE ONLY bet_orders
    ADD CONSTRAINT bet_orders_pkey PRIMARY KEY (id);

ALTER TABLE ONLY brand_agent_policies
    ADD CONSTRAINT brand_agent_policies_pkey PRIMARY KEY (brand_id);

ALTER TABLE ONLY brand_bet_policies
    ADD CONSTRAINT brand_bet_policies_pkey PRIMARY KEY (brand_id);

ALTER TABLE ONLY brand_commission_correction_policies
    ADD CONSTRAINT brand_commission_correction_policies_pkey PRIMARY KEY (brand_id);

ALTER TABLE ONLY brand_commission_payment_policies
    ADD CONSTRAINT brand_commission_payment_policies_pkey PRIMARY KEY (brand_id);

ALTER TABLE ONLY brand_commission_policies
    ADD CONSTRAINT brand_commission_policies_pkey PRIMARY KEY (brand_id);

ALTER TABLE ONLY brand_compliance_policies
    ADD CONSTRAINT brand_compliance_policies_pkey PRIMARY KEY (brand_id);

ALTER TABLE ONLY brand_creation_records
    ADD CONSTRAINT brand_creation_records_audit_log_id_key UNIQUE (audit_log_id);

ALTER TABLE ONLY brand_creation_records
    ADD CONSTRAINT brand_creation_records_pkey PRIMARY KEY (brand_id);

ALTER TABLE ONLY brand_domain_revisions
    ADD CONSTRAINT brand_domain_revisions_brand_id_version_key UNIQUE (brand_id, version);

ALTER TABLE ONLY brand_domain_revisions
    ADD CONSTRAINT brand_domain_revisions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY brand_domains
    ADD CONSTRAINT brand_domains_domain_key UNIQUE (domain);

ALTER TABLE ONLY brand_domains
    ADD CONSTRAINT brand_domains_pkey PRIMARY KEY (id);

ALTER TABLE ONLY brand_members
    ADD CONSTRAINT brand_members_brand_id_global_user_id_key UNIQUE (brand_id, global_user_id);

ALTER TABLE ONLY brand_members
    ADD CONSTRAINT brand_members_brand_id_id_key UNIQUE (brand_id, id);

ALTER TABLE ONLY brand_members
    ADD CONSTRAINT brand_members_pkey PRIMARY KEY (id);

ALTER TABLE ONLY brand_operation_revisions
    ADD CONSTRAINT brand_operation_revisions_brand_id_version_key UNIQUE (brand_id, version);

ALTER TABLE ONLY brand_operation_revisions
    ADD CONSTRAINT brand_operation_revisions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY brand_point_policies
    ADD CONSTRAINT brand_point_policies_pkey PRIMARY KEY (brand_id);

ALTER TABLE ONLY brand_presentation_revisions
    ADD CONSTRAINT brand_presentation_revisions_brand_id_version_key UNIQUE (brand_id, version);

ALTER TABLE ONLY brand_presentation_revisions
    ADD CONSTRAINT brand_presentation_revisions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY brand_presentations
    ADD CONSTRAINT brand_presentations_pkey PRIMARY KEY (brand_id);

ALTER TABLE ONLY brand_report_archive_policies
    ADD CONSTRAINT brand_report_archive_policies_pkey PRIMARY KEY (brand_id);

ALTER TABLE ONLY brand_settlement_policies
    ADD CONSTRAINT brand_settlement_policies_pkey PRIMARY KEY (brand_id);

ALTER TABLE ONLY brand_withdrawal_policies
    ADD CONSTRAINT brand_withdrawal_policies_pkey PRIMARY KEY (brand_id);

ALTER TABLE brand_withdrawal_policies
    ADD CONSTRAINT brand_withdrawal_positive_multiple CHECK (valid_positive_withdrawal_multiple((config -> 'turnover_multiple'::text)));

ALTER TABLE ONLY brands
    ADD CONSTRAINT brands_code_key UNIQUE (code);

ALTER TABLE ONLY brands
    ADD CONSTRAINT brands_pkey PRIMARY KEY (id);

ALTER TABLE ONLY commission_adjustment_heads
    ADD CONSTRAINT commission_adjustment_heads_brand_id_target_id_key UNIQUE (brand_id, target_id);

ALTER TABLE ONLY commission_adjustment_heads
    ADD CONSTRAINT commission_adjustment_heads_pkey PRIMARY KEY (target_id);

ALTER TABLE ONLY commission_adjustments
    ADD CONSTRAINT commission_adjustments_brand_id_id_key UNIQUE (brand_id, id);

ALTER TABLE ONLY commission_adjustments
    ADD CONSTRAINT commission_adjustments_brand_id_target_id_id_key UNIQUE (brand_id, target_id, id);

ALTER TABLE ONLY commission_adjustments
    ADD CONSTRAINT commission_adjustments_ledger_entry_id_key UNIQUE (ledger_entry_id);

ALTER TABLE ONLY commission_adjustments
    ADD CONSTRAINT commission_adjustments_pkey PRIMARY KEY (id);

ALTER TABLE ONLY commission_adjustments
    ADD CONSTRAINT commission_adjustments_target_id_version_key UNIQUE (target_id, version);

ALTER TABLE ONLY commission_allocations
    ADD CONSTRAINT commission_allocations_pkey PRIMARY KEY (run_id, order_id, agent_id);

ALTER TABLE ONLY commission_calculations
    ADD CONSTRAINT commission_calculations_brand_id_cycle_id_run_id_id_key UNIQUE (brand_id, cycle_id, run_id, id);

ALTER TABLE ONLY commission_calculations
    ADD CONSTRAINT commission_calculations_pkey PRIMARY KEY (id);

ALTER TABLE ONLY commission_calculations
    ADD CONSTRAINT commission_calculations_run_id_order_id_key UNIQUE (run_id, order_id);

ALTER TABLE ONLY commission_correction_balance_heads
    ADD CONSTRAINT commission_correction_balance_heads_pkey PRIMARY KEY (cycle_id, agent_id);

ALTER TABLE ONLY commission_correction_cycle_holds
    ADD CONSTRAINT commission_correction_cycle_holds_pkey PRIMARY KEY (cycle_id);

ALTER TABLE ONLY commission_correction_execution_targets
    ADD CONSTRAINT commission_correction_executi_brand_id_cycle_id_agent_id_fi_key UNIQUE (brand_id, cycle_id, agent_id, financial_version);

ALTER TABLE ONLY commission_correction_execution_targets
    ADD CONSTRAINT commission_correction_execution_execution_id_plan_target_id_key UNIQUE (execution_id, plan_target_id);

ALTER TABLE ONLY commission_correction_execution_steps
    ADD CONSTRAINT commission_correction_execution_steps_audit_log_id_key UNIQUE (audit_log_id);

ALTER TABLE ONLY commission_correction_execution_steps
    ADD CONSTRAINT commission_correction_execution_steps_execution_id_version_key UNIQUE (execution_id, version);

ALTER TABLE ONLY commission_correction_execution_steps
    ADD CONSTRAINT commission_correction_execution_steps_pkey PRIMARY KEY (id);

ALTER TABLE ONLY commission_correction_execution_targets
    ADD CONSTRAINT commission_correction_execution_targets_brand_id_id_key UNIQUE (brand_id, id);

ALTER TABLE ONLY commission_correction_execution_targets
    ADD CONSTRAINT commission_correction_execution_targets_ledger_entry_id_key UNIQUE (ledger_entry_id);

ALTER TABLE ONLY commission_correction_execution_targets
    ADD CONSTRAINT commission_correction_execution_targets_pkey PRIMARY KEY (id);

ALTER TABLE ONLY commission_correction_executions
    ADD CONSTRAINT commission_correction_executions_brand_id_cycle_id_id_key UNIQUE (brand_id, cycle_id, id);

ALTER TABLE ONLY commission_correction_executions
    ADD CONSTRAINT commission_correction_executions_brand_id_id_key UNIQUE (brand_id, id);

ALTER TABLE ONLY commission_correction_executions
    ADD CONSTRAINT commission_correction_executions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY commission_correction_executions
    ADD CONSTRAINT commission_correction_executions_plan_id_key UNIQUE (plan_id);

ALTER TABLE ONLY commission_correction_plan_steps
    ADD CONSTRAINT commission_correction_plan_steps_audit_log_id_key UNIQUE (audit_log_id);

ALTER TABLE ONLY commission_correction_plan_steps
    ADD CONSTRAINT commission_correction_plan_steps_pkey PRIMARY KEY (id);

ALTER TABLE ONLY commission_correction_plan_steps
    ADD CONSTRAINT commission_correction_plan_steps_plan_id_version_key UNIQUE (plan_id, version);

ALTER TABLE ONLY commission_correction_plan_targets
    ADD CONSTRAINT commission_correction_plan_targets_brand_id_plan_id_id_key UNIQUE (brand_id, plan_id, id);

ALTER TABLE ONLY commission_correction_plan_targets
    ADD CONSTRAINT commission_correction_plan_targets_pkey PRIMARY KEY (id);

ALTER TABLE ONLY commission_correction_plan_targets
    ADD CONSTRAINT commission_correction_plan_targets_plan_id_agent_id_key UNIQUE (plan_id, agent_id);

ALTER TABLE ONLY commission_correction_plans
    ADD CONSTRAINT commission_correction_plans_brand_id_cycle_id_id_key UNIQUE (brand_id, cycle_id, id);

ALTER TABLE ONLY commission_correction_plans
    ADD CONSTRAINT commission_correction_plans_brand_id_id_key UNIQUE (brand_id, id);

ALTER TABLE ONLY commission_correction_plans
    ADD CONSTRAINT commission_correction_plans_cycle_id_run_id_key UNIQUE (cycle_id, run_id);

ALTER TABLE ONLY commission_correction_plans
    ADD CONSTRAINT commission_correction_plans_pkey PRIMARY KEY (id);

ALTER TABLE ONLY commission_correction_policy_revisions
    ADD CONSTRAINT commission_correction_policy_revisions_pkey PRIMARY KEY (brand_id, version);

ALTER TABLE ONLY commission_cycle_steps
    ADD CONSTRAINT commission_cycle_steps_cycle_id_version_key UNIQUE (cycle_id, version);

ALTER TABLE ONLY commission_cycle_steps
    ADD CONSTRAINT commission_cycle_steps_pkey PRIMARY KEY (id);

ALTER TABLE ONLY commission_cycle_targets
    ADD CONSTRAINT commission_cycle_targets_brand_id_cycle_id_order_id_key UNIQUE (brand_id, cycle_id, order_id);

ALTER TABLE ONLY commission_cycle_targets
    ADD CONSTRAINT commission_cycle_targets_pkey PRIMARY KEY (cycle_id, order_id);

ALTER TABLE ONLY commission_cycles
    ADD CONSTRAINT commission_cycles_brand_id_id_key UNIQUE (brand_id, id);

ALTER TABLE ONLY commission_cycles
    ADD CONSTRAINT commission_cycles_brand_id_window_from_window_to_key UNIQUE (brand_id, window_from, window_to);

ALTER TABLE ONLY commission_cycles
    ADD CONSTRAINT commission_cycles_pkey PRIMARY KEY (id);

ALTER TABLE ONLY commission_discovery
    ADD CONSTRAINT commission_discovery_brand_id_id_key UNIQUE (brand_id, id);

ALTER TABLE ONLY commission_discovery
    ADD CONSTRAINT commission_discovery_pkey PRIMARY KEY (id);

ALTER TABLE ONLY commission_earnings
    ADD CONSTRAINT commission_earnings_brand_id_cycle_id_run_id_id_key UNIQUE (brand_id, cycle_id, run_id, id);

ALTER TABLE ONLY commission_earnings
    ADD CONSTRAINT commission_earnings_pkey PRIMARY KEY (id);

ALTER TABLE ONLY commission_earnings
    ADD CONSTRAINT commission_earnings_run_id_agent_id_key UNIQUE (run_id, agent_id);

ALTER TABLE ONLY commission_payment_policy_revisions
    ADD CONSTRAINT commission_payment_policy_revisions_pkey PRIMARY KEY (brand_id, version);

ALTER TABLE ONLY commission_payment_targets
    ADD CONSTRAINT commission_payment_targets_brand_id_id_key UNIQUE (brand_id, id);

ALTER TABLE ONLY commission_payment_targets
    ADD CONSTRAINT commission_payment_targets_ledger_entry_id_key UNIQUE (ledger_entry_id);

ALTER TABLE ONLY commission_payment_targets
    ADD CONSTRAINT commission_payment_targets_payment_id_earning_id_key UNIQUE (payment_id, earning_id);

ALTER TABLE ONLY commission_payment_targets
    ADD CONSTRAINT commission_payment_targets_pkey PRIMARY KEY (id);

ALTER TABLE ONLY commission_payments
    ADD CONSTRAINT commission_payments_brand_id_id_key UNIQUE (brand_id, id);

ALTER TABLE ONLY commission_payments
    ADD CONSTRAINT commission_payments_cycle_id_run_id_key UNIQUE (cycle_id, run_id);

ALTER TABLE ONLY commission_payments
    ADD CONSTRAINT commission_payments_pkey PRIMARY KEY (id);

ALTER TABLE ONLY commission_policy_revisions
    ADD CONSTRAINT commission_policy_revisions_brand_id_id_key UNIQUE (brand_id, id);

ALTER TABLE ONLY commission_policy_revisions
    ADD CONSTRAINT commission_policy_revisions_brand_id_version_key UNIQUE (brand_id, version);

ALTER TABLE ONLY commission_policy_revisions
    ADD CONSTRAINT commission_policy_revisions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY commission_runs
    ADD CONSTRAINT commission_runs_brand_id_cycle_id_id_key UNIQUE (brand_id, cycle_id, id);

ALTER TABLE ONLY commission_runs
    ADD CONSTRAINT commission_runs_cycle_id_generation_key UNIQUE (cycle_id, generation);

ALTER TABLE ONLY commission_runs
    ADD CONSTRAINT commission_runs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY compliance_decisions
    ADD CONSTRAINT compliance_decisions_audit_log_id_key UNIQUE (audit_log_id);

ALTER TABLE ONLY compliance_decisions
    ADD CONSTRAINT compliance_decisions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY compliance_gate_rejections
    ADD CONSTRAINT compliance_gate_rejections_audit_log_id_key UNIQUE (audit_log_id);

ALTER TABLE ONLY compliance_gate_rejections
    ADD CONSTRAINT compliance_gate_rejections_pkey PRIMARY KEY (id);

ALTER TABLE ONLY compliance_policy_revisions
    ADD CONSTRAINT compliance_policy_revisions_brand_id_version_key UNIQUE (brand_id, version);

ALTER TABLE ONLY compliance_policy_revisions
    ADD CONSTRAINT compliance_policy_revisions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY consumed_events
    ADD CONSTRAINT consumed_events_pkey PRIMARY KEY (consumer, event_id);

ALTER TABLE ONLY draw_attempt_batches
    ADD CONSTRAINT draw_attempt_batches_pkey PRIMARY KEY (id);

ALTER TABLE ONLY draw_correction_failures
    ADD CONSTRAINT draw_correction_failures_pkey PRIMARY KEY (id);

ALTER TABLE ONLY draw_correction_targets
    ADD CONSTRAINT draw_correction_targets_pkey PRIMARY KEY (correction_id, order_id);

ALTER TABLE ONLY draw_corrections
    ADD CONSTRAINT draw_corrections_brand_id_id_key UNIQUE (brand_id, id);

ALTER TABLE ONLY draw_corrections
    ADD CONSTRAINT draw_corrections_brand_id_period_id_id_key UNIQUE (brand_id, period_id, id);

ALTER TABLE ONLY draw_corrections
    ADD CONSTRAINT draw_corrections_pkey PRIMARY KEY (id);

ALTER TABLE ONLY draw_notification_publications
    ADD CONSTRAINT draw_notification_publications_brand_id_draw_result_id_key UNIQUE (brand_id, draw_result_id);

ALTER TABLE ONLY draw_notification_publications
    ADD CONSTRAINT draw_notification_publications_pkey PRIMARY KEY (draw_result_id);

ALTER TABLE ONLY draw_notification_recipients
    ADD CONSTRAINT draw_notification_recipients_brand_id_draw_result_id_member_key UNIQUE (brand_id, draw_result_id, member_id);

ALTER TABLE ONLY draw_notification_recipients
    ADD CONSTRAINT draw_notification_recipients_pkey PRIMARY KEY (id);

ALTER TABLE ONLY draw_results
    ADD CONSTRAINT draw_results_brand_id_game_id_period_id_id_key UNIQUE (brand_id, game_id, period_id, id);

ALTER TABLE ONLY draw_results
    ADD CONSTRAINT draw_results_pkey PRIMARY KEY (id);

ALTER TABLE ONLY draw_source_sets
    ADD CONSTRAINT draw_source_sets_brand_id_game_id_id_key UNIQUE (brand_id, game_id, id);

ALTER TABLE ONLY draw_source_sets
    ADD CONSTRAINT draw_source_sets_brand_id_game_id_revision_key UNIQUE (brand_id, game_id, revision);

ALTER TABLE ONLY draw_source_sets
    ADD CONSTRAINT draw_source_sets_pkey PRIMARY KEY (id);

ALTER TABLE ONLY draw_sources
    ADD CONSTRAINT draw_sources_brand_id_game_id_id_key UNIQUE (brand_id, game_id, id);

ALTER TABLE ONLY draw_sources
    ADD CONSTRAINT draw_sources_brand_id_game_id_id_type_key UNIQUE (brand_id, game_id, id, type);

ALTER TABLE ONLY draw_sources
    ADD CONSTRAINT draw_sources_pkey PRIMARY KEY (id);

ALTER TABLE ONLY game_bet_policies
    ADD CONSTRAINT game_bet_policies_brand_id_game_id_key UNIQUE (brand_id, game_id);

ALTER TABLE ONLY game_bet_policies
    ADD CONSTRAINT game_bet_policies_pkey PRIMARY KEY (game_id);

ALTER TABLE ONLY game_schedules
    ADD CONSTRAINT game_schedules_brand_id_game_id_id_key UNIQUE (brand_id, game_id, id);

ALTER TABLE ONLY game_schedules
    ADD CONSTRAINT game_schedules_brand_id_game_id_revision_key UNIQUE (brand_id, game_id, revision);

ALTER TABLE ONLY game_schedules
    ADD CONSTRAINT game_schedules_pkey PRIMARY KEY (id);

ALTER TABLE ONLY game_withdrawal_policies
    ADD CONSTRAINT game_withdrawal_policies_pkey PRIMARY KEY (brand_id, game_id);

ALTER TABLE game_withdrawal_policies
    ADD CONSTRAINT game_withdrawal_positive_multiple CHECK ((((config -> 'turnover_multiple'::text) = 'null'::jsonb) OR valid_positive_withdrawal_multiple((config -> 'turnover_multiple'::text))));

ALTER TABLE ONLY games
    ADD CONSTRAINT games_brand_id_code_key UNIQUE (brand_id, code);

ALTER TABLE ONLY games
    ADD CONSTRAINT games_brand_id_id_key UNIQUE (brand_id, id);

ALTER TABLE ONLY games
    ADD CONSTRAINT games_pkey PRIMARY KEY (id);

ALTER TABLE ONLY global_users
    ADD CONSTRAINT global_users_phone_key UNIQUE (phone);

ALTER TABLE ONLY global_users
    ADD CONSTRAINT global_users_pkey PRIMARY KEY (id);

ALTER TABLE ONLY global_users
    ADD CONSTRAINT global_users_telegram_user_id_key UNIQUE (telegram_user_id);

ALTER TABLE ONLY global_users
    ADD CONSTRAINT global_users_username_key UNIQUE (username);

ALTER TABLE ONLY idempotency_requests
    ADD CONSTRAINT idempotency_requests_pkey PRIMARY KEY (brand_id, actor_id, operation, key);

ALTER TABLE ONLY join_code_revisions
    ADD CONSTRAINT join_code_revisions_brand_id_code_id_version_key UNIQUE (brand_id, code_id, version);

ALTER TABLE ONLY join_code_revisions
    ADD CONSTRAINT join_code_revisions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY join_codes
    ADD CONSTRAINT join_codes_brand_id_code_key UNIQUE (brand_id, code);

ALTER TABLE ONLY join_codes
    ADD CONSTRAINT join_codes_brand_id_id_key UNIQUE (brand_id, id);

ALTER TABLE ONLY join_codes
    ADD CONSTRAINT join_codes_pkey PRIMARY KEY (id);

ALTER TABLE ONLY point_ledger_entries
    ADD CONSTRAINT ledger_account_id_unique UNIQUE (brand_id, account_id, id);

ALTER TABLE ONLY point_ledger_entries
    ADD CONSTRAINT ledger_account_version_unique UNIQUE (brand_id, account_id, version);

ALTER TABLE ONLY point_ledger_entries
    ADD CONSTRAINT ledger_brand_id_unique UNIQUE (brand_id, id);

ALTER TABLE ONLY brand_members
    ADD CONSTRAINT members_identity_scope UNIQUE (brand_id, id, global_user_id);

ALTER TABLE ONLY notification_deliveries
    ADD CONSTRAINT notification_deliveries_brand_id_event_id_key UNIQUE (brand_id, event_id);

ALTER TABLE ONLY notification_deliveries
    ADD CONSTRAINT notification_deliveries_pkey PRIMARY KEY (event_id);

ALTER TABLE ONLY notification_template_revisions
    ADD CONSTRAINT notification_template_revisio_brand_id_template_key_version_key UNIQUE (brand_id, template_key, version);

ALTER TABLE ONLY notification_template_revisions
    ADD CONSTRAINT notification_template_revisions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY notification_templates
    ADD CONSTRAINT notification_templates_pkey PRIMARY KEY (brand_id, template_key);

ALTER TABLE ONLY notifications
    ADD CONSTRAINT notifications_event_id_member_id_key UNIQUE (event_id, member_id);

ALTER TABLE ONLY notifications
    ADD CONSTRAINT notifications_pkey PRIMARY KEY (id);

ALTER TABLE ONLY outbox_events
    ADD CONSTRAINT outbox_events_pkey PRIMARY KEY (id);

ALTER TABLE ONLY period_cancellation_failures
    ADD CONSTRAINT period_cancellation_failures_pkey PRIMARY KEY (id);

ALTER TABLE ONLY period_cancellation_targets
    ADD CONSTRAINT period_cancellation_targets_pkey PRIMARY KEY (cancellation_id, order_id);

ALTER TABLE ONLY period_cancellations
    ADD CONSTRAINT period_cancellations_brand_id_period_id_id_key UNIQUE (brand_id, period_id, id);

ALTER TABLE ONLY period_cancellations
    ADD CONSTRAINT period_cancellations_brand_id_period_id_key UNIQUE (brand_id, period_id);

ALTER TABLE ONLY period_cancellations
    ADD CONSTRAINT period_cancellations_pkey PRIMARY KEY (id);

ALTER TABLE ONLY period_rule_versions
    ADD CONSTRAINT period_rule_versions_pkey PRIMARY KEY (period_id, play_id);

ALTER TABLE ONLY periods
    ADD CONSTRAINT periods_brand_id_game_id_id_key UNIQUE (brand_id, game_id, id);

ALTER TABLE ONLY periods
    ADD CONSTRAINT periods_brand_id_game_id_period_no_key UNIQUE (brand_id, game_id, period_no);

ALTER TABLE ONLY periods
    ADD CONSTRAINT periods_brand_id_game_id_sequence_key UNIQUE (brand_id, game_id, sequence);

ALTER TABLE ONLY periods
    ADD CONSTRAINT periods_pkey PRIMARY KEY (id);

ALTER TABLE ONLY permissions
    ADD CONSTRAINT permissions_pkey PRIMARY KEY (key);

ALTER TABLE ONLY platform_domains
    ADD CONSTRAINT platform_domains_pkey PRIMARY KEY (domain);

ALTER TABLE ONLY platform_idempotency_requests
    ADD CONSTRAINT platform_idempotency_requests_pkey PRIMARY KEY (actor_id, operation, key);

ALTER TABLE ONLY play_definitions
    ADD CONSTRAINT play_definitions_brand_id_game_id_code_key UNIQUE (brand_id, game_id, code);

ALTER TABLE ONLY play_definitions
    ADD CONSTRAINT play_definitions_brand_id_game_id_id_key UNIQUE (brand_id, game_id, id);

ALTER TABLE ONLY play_definitions
    ADD CONSTRAINT play_definitions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY point_accounts
    ADD CONSTRAINT point_account_member_identity UNIQUE (brand_id, id, brand_member_id);

ALTER TABLE ONLY point_accounts
    ADD CONSTRAINT point_accounts_brand_id_brand_member_id_key UNIQUE (brand_id, brand_member_id);

ALTER TABLE ONLY point_accounts
    ADD CONSTRAINT point_accounts_brand_id_id_key UNIQUE (brand_id, id);

ALTER TABLE ONLY point_accounts
    ADD CONSTRAINT point_accounts_pkey PRIMARY KEY (id);

ALTER TABLE ONLY point_balance_repairs
    ADD CONSTRAINT point_balance_repairs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY point_buckets
    ADD CONSTRAINT point_buckets_pkey PRIMARY KEY (brand_id, account_id, source, state);

ALTER TABLE ONLY point_ledger_entries
    ADD CONSTRAINT point_ledger_entries_brand_id_operation_key_key UNIQUE (brand_id, operation_key);

ALTER TABLE ONLY point_ledger_entries
    ADD CONSTRAINT point_ledger_entries_pkey PRIMARY KEY (id);

ALTER TABLE ONLY point_reconciliation_failures
    ADD CONSTRAINT point_reconciliation_failures_brand_id_job_id_id_key UNIQUE (brand_id, job_id, id);

ALTER TABLE ONLY point_reconciliation_failures
    ADD CONSTRAINT point_reconciliation_failures_pkey PRIMARY KEY (id);

ALTER TABLE ONLY point_reconciliation_failures
    ADD CONSTRAINT point_reconciliation_failures_target_id_attempt_count_key UNIQUE (target_id, attempt_count);

ALTER TABLE ONLY point_reconciliation_jobs
    ADD CONSTRAINT point_reconciliation_jobs_brand_id_id_key UNIQUE (brand_id, id);

ALTER TABLE ONLY point_reconciliation_jobs
    ADD CONSTRAINT point_reconciliation_jobs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY point_reconciliation_results
    ADD CONSTRAINT point_reconciliation_results_pkey PRIMARY KEY (target_id);

ALTER TABLE ONLY point_reconciliation_retries
    ADD CONSTRAINT point_reconciliation_retries_job_id_version_key UNIQUE (job_id, version);

ALTER TABLE ONLY point_reconciliation_retries
    ADD CONSTRAINT point_reconciliation_retries_pkey PRIMARY KEY (id);

ALTER TABLE ONLY point_reconciliation_targets
    ADD CONSTRAINT point_reconciliation_targets_brand_id_job_id_account_id_key UNIQUE (brand_id, job_id, account_id);

ALTER TABLE ONLY point_reconciliation_targets
    ADD CONSTRAINT point_reconciliation_targets_brand_id_job_id_id_key UNIQUE (brand_id, job_id, id);

ALTER TABLE ONLY point_reconciliation_targets
    ADD CONSTRAINT point_reconciliation_targets_pkey PRIMARY KEY (id);

ALTER TABLE ONLY recharge_orders
    ADD CONSTRAINT recharge_orders_brand_id_id_key UNIQUE (brand_id, id);

ALTER TABLE ONLY recharge_orders
    ADD CONSTRAINT recharge_orders_ledger_entry_id_key UNIQUE (ledger_entry_id);

ALTER TABLE ONLY recharge_orders
    ADD CONSTRAINT recharge_orders_pkey PRIMARY KEY (id);

ALTER TABLE ONLY report_archive_automatic_cursors
    ADD CONSTRAINT report_archive_automatic_cursors_pkey PRIMARY KEY (brand_id, policy_version, kind);

ALTER TABLE ONLY report_archive_automatic_tasks
    ADD CONSTRAINT report_archive_automatic_tasks_brand_id_id_key UNIQUE (brand_id, id);

ALTER TABLE ONLY report_archive_automatic_tasks
    ADD CONSTRAINT report_archive_automatic_tasks_brand_id_kind_period_key_key UNIQUE (brand_id, kind, period_key);

ALTER TABLE ONLY report_archive_automatic_tasks
    ADD CONSTRAINT report_archive_automatic_tasks_pkey PRIMARY KEY (id);

ALTER TABLE ONLY report_archive_policy_revisions
    ADD CONSTRAINT report_archive_policy_revisions_pkey PRIMARY KEY (brand_id, version);

ALTER TABLE ONLY report_archives
    ADD CONSTRAINT report_archives_brand_id_id_key UNIQUE (brand_id, id);

ALTER TABLE ONLY report_archives
    ADD CONSTRAINT report_archives_brand_id_kind_period_key_revision_key UNIQUE (brand_id, kind, period_key, revision);

ALTER TABLE ONLY report_archives
    ADD CONSTRAINT report_archives_pkey PRIMARY KEY (id);

ALTER TABLE ONLY reward_order_actions
    ADD CONSTRAINT reward_order_actions_audit_log_id_key UNIQUE (audit_log_id);

ALTER TABLE ONLY reward_order_actions
    ADD CONSTRAINT reward_order_actions_brand_id_order_id_version_key UNIQUE (brand_id, order_id, version);

ALTER TABLE ONLY reward_order_actions
    ADD CONSTRAINT reward_order_actions_ledger_entry_id_key UNIQUE (ledger_entry_id);

ALTER TABLE ONLY reward_order_actions
    ADD CONSTRAINT reward_order_actions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY reward_orders
    ADD CONSTRAINT reward_orders_brand_id_id_key UNIQUE (brand_id, id);

ALTER TABLE ONLY reward_orders
    ADD CONSTRAINT reward_orders_creation_audit_log_id_key UNIQUE (creation_audit_log_id);

ALTER TABLE ONLY reward_orders
    ADD CONSTRAINT reward_orders_grant_ledger_entry_id_key UNIQUE (grant_ledger_entry_id);

ALTER TABLE ONLY reward_orders
    ADD CONSTRAINT reward_orders_pkey PRIMARY KEY (id);

ALTER TABLE ONLY reward_orders
    ADD CONSTRAINT reward_orders_revoke_ledger_entry_id_key UNIQUE (revoke_ledger_entry_id);

ALTER TABLE ONLY role_permissions
    ADD CONSTRAINT role_permissions_pkey PRIMARY KEY (role_id, permission_key);

ALTER TABLE ONLY roles
    ADD CONSTRAINT roles_brand_code_unique UNIQUE NULLS NOT DISTINCT (brand_id, code);

ALTER TABLE ONLY roles
    ADD CONSTRAINT roles_pkey PRIMARY KEY (id);

ALTER TABLE ONLY rule_version_contributors
    ADD CONSTRAINT rule_version_contributors_pkey PRIMARY KEY (rule_version_id, admin_id);

ALTER TABLE ONLY rule_versions
    ADD CONSTRAINT rule_versions_brand_id_game_id_play_id_id_key UNIQUE (brand_id, game_id, play_id, id);

ALTER TABLE ONLY rule_versions
    ADD CONSTRAINT rule_versions_brand_id_game_id_play_id_version_no_key UNIQUE (brand_id, game_id, play_id, version_no);

ALTER TABLE ONLY rule_versions
    ADD CONSTRAINT rule_versions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY sessions
    ADD CONSTRAINT sessions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY sessions
    ADD CONSTRAINT sessions_token_hash_key UNIQUE (token_hash);

ALTER TABLE ONLY settlement_calculations
    ADD CONSTRAINT settlement_calculations_brand_id_id_key UNIQUE (brand_id, id);

ALTER TABLE ONLY settlement_calculations
    ADD CONSTRAINT settlement_calculations_brand_id_order_id_id_key UNIQUE (brand_id, order_id, id);

ALTER TABLE ONLY settlement_calculations
    ADD CONSTRAINT settlement_calculations_job_id_order_id_key UNIQUE (job_id, order_id);

ALTER TABLE ONLY settlement_calculations
    ADD CONSTRAINT settlement_calculations_pkey PRIMARY KEY (id);

ALTER TABLE ONLY settlement_failures
    ADD CONSTRAINT settlement_failures_pkey PRIMARY KEY (id);

ALTER TABLE ONLY settlement_jobs
    ADD CONSTRAINT settlement_jobs_brand_id_id_key UNIQUE (brand_id, id);

ALTER TABLE ONLY settlement_jobs
    ADD CONSTRAINT settlement_jobs_brand_id_period_id_generation_key UNIQUE (brand_id, period_id, generation);

ALTER TABLE ONLY settlement_jobs
    ADD CONSTRAINT settlement_jobs_brand_id_period_id_id_key UNIQUE (brand_id, period_id, id);

ALTER TABLE ONLY settlement_jobs
    ADD CONSTRAINT settlement_jobs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY settlement_policy_history
    ADD CONSTRAINT settlement_policy_history_pkey PRIMARY KEY (brand_id, version);

ALTER TABLE ONLY settlement_previews
    ADD CONSTRAINT settlement_previews_brand_id_id_key UNIQUE (brand_id, id);

ALTER TABLE ONLY settlement_previews
    ADD CONSTRAINT settlement_previews_pkey PRIMARY KEY (id);

ALTER TABLE ONLY settlement_targets
    ADD CONSTRAINT settlement_targets_pkey PRIMARY KEY (job_id, order_id);

ALTER TABLE ONLY withdrawal_operation_receipts
    ADD CONSTRAINT withdrawal_operation_receipts_brand_id_actor_type_actor_id__key UNIQUE (brand_id, actor_type, actor_id, client_key);

ALTER TABLE ONLY withdrawal_operation_receipts
    ADD CONSTRAINT withdrawal_operation_receipts_pkey PRIMARY KEY (id);

ALTER TABLE ONLY withdrawal_order_transitions
    ADD CONSTRAINT withdrawal_order_transitions_order_id_version_key UNIQUE (order_id, version);

ALTER TABLE ONLY withdrawal_order_transitions
    ADD CONSTRAINT withdrawal_order_transitions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY withdrawal_orders
    ADD CONSTRAINT withdrawal_orders_brand_id_id_key UNIQUE (brand_id, id);

ALTER TABLE ONLY withdrawal_orders
    ADD CONSTRAINT withdrawal_orders_paid_entry_id_key UNIQUE (paid_entry_id);

ALTER TABLE ONLY withdrawal_orders
    ADD CONSTRAINT withdrawal_orders_pkey PRIMARY KEY (id);

ALTER TABLE ONLY withdrawal_orders
    ADD CONSTRAINT withdrawal_orders_release_entry_id_key UNIQUE (release_entry_id);

ALTER TABLE ONLY withdrawal_orders
    ADD CONSTRAINT withdrawal_orders_reserve_entry_id_key UNIQUE (reserve_entry_id);

ALTER TABLE ONLY withdrawal_policy_revisions
    ADD CONSTRAINT withdrawal_policy_revisions_brand_id_game_id_version_key UNIQUE NULLS NOT DISTINCT (brand_id, game_id, version);

ALTER TABLE ONLY withdrawal_policy_revisions
    ADD CONSTRAINT withdrawal_policy_revisions_pkey PRIMARY KEY (id);

ALTER TABLE withdrawal_policy_revisions
    ADD CONSTRAINT withdrawal_revision_positive_multiple CHECK ((((game_id IS NOT NULL) AND ((config -> 'turnover_multiple'::text) = 'null'::jsonb)) OR valid_positive_withdrawal_multiple((config -> 'turnover_multiple'::text))));

ALTER TABLE ONLY withdrawal_turnover_cycles
    ADD CONSTRAINT withdrawal_turnover_cycles_pkey PRIMARY KEY (brand_id, member_id);

CREATE INDEX agent_children_page ON agent_nodes USING btree (brand_id, parent_id, created_at, id);

CREATE INDEX agent_config_history ON agent_config_revisions USING btree (brand_id, agent_id, version DESC);

CREATE INDEX audit_brand_resource ON audit_logs USING btree (brand_id, resource_type, resource_id, created_at);

CREATE INDEX audit_brand_time_page ON audit_logs USING btree (brand_id, created_at DESC, id DESC);

CREATE INDEX audit_platform_time_page ON audit_logs USING btree (created_at DESC, id DESC);

CREATE INDEX auth_challenges_expiry ON auth_challenges USING btree (expires_at);

CREATE INDEX auth_rate_limit_expiry ON auth_rate_limits USING btree (expires_at);

CREATE INDEX bet_orders_game_history ON bet_orders USING btree (brand_id, game_id, placed_at DESC, id DESC);

CREATE INDEX bet_orders_member_history ON bet_orders USING btree (brand_id, brand_member_id, placed_at DESC, id DESC);

CREATE INDEX bet_orders_period_stakes ON bet_orders USING btree (brand_id, period_id) INCLUDE (brand_member_id, total_points, status);

CREATE INDEX bet_orders_report_placed ON bet_orders USING btree (brand_id, placed_at, id);

CREATE INDEX brand_operation_history ON brand_operation_revisions USING btree (brand_id, version DESC);

CREATE UNIQUE INDEX brand_primary_domain ON brand_domains USING btree (brand_id) WHERE (is_primary AND enabled);

CREATE INDEX business_reconciliation_point_audit_idx ON audit_logs USING btree (brand_id, ((after_json ->> 'ledger_entry_id'::text)), action, resource_id) WHERE (resource_type = 'point_account'::text);

CREATE INDEX cancellation_processing ON period_cancellations USING btree (created_at, id) WHERE (state = 'processing'::text);

CREATE INDEX cancellation_targets_pending ON period_cancellation_targets USING btree (cancellation_id, order_id) WHERE (state = 'pending'::text);

CREATE INDEX commission_adjustment_history ON commission_adjustments USING btree (brand_id, target_id, version DESC);

CREATE INDEX commission_allocations_sum ON commission_allocations USING btree (run_id, agent_id, member_id);

CREATE INDEX commission_analysis_ledger_binding ON point_ledger_entries USING btree (brand_id, reference_type, reference_id) WHERE (reference_type = ANY (ARRAY['commission_payment_target'::text, 'commission_adjustment'::text, 'commission_correction_target'::text]));

CREATE INDEX commission_analysis_ledger_family ON point_ledger_entries USING btree (brand_id, id) WHERE ((entry_type = ANY (ARRAY['commission'::text, 'commission_adjustment'::text, 'commission_correction'::text])) OR (reference_type = ANY (ARRAY['commission_payment_target'::text, 'commission_adjustment'::text, 'commission_correction_target'::text])));

CREATE INDEX commission_calculations_run ON commission_calculations USING btree (run_id, order_id);

CREATE INDEX commission_correction_execution_due ON commission_correction_executions USING btree (next_work_at, id) WHERE (state = 'applying'::text);

CREATE UNIQUE INDEX commission_correction_execution_live_cycle ON commission_correction_executions USING btree (cycle_id) WHERE (state <> 'stale'::text);

CREATE UNIQUE INDEX commission_correction_notification_once ON outbox_events USING btree (brand_id, event_type, aggregate_id) WHERE (event_type = 'commission.corrected'::text);

CREATE UNIQUE INDEX commission_correction_one_live_plan ON commission_correction_plans USING btree (cycle_id) WHERE (state <> 'stale'::text);

CREATE INDEX commission_correction_plan_due ON commission_correction_plans USING btree (next_work_at, id) WHERE (state = 'planning'::text);

CREATE INDEX commission_correction_posting_report_idx ON point_ledger_entries USING btree (brand_id, created_at, id) WHERE (entry_type = 'commission_correction'::text);

CREATE INDEX commission_cycles_due ON commission_cycles USING btree (next_work_at, id) WHERE (state <> ALL (ARRAY['ready'::text, 'failed'::text]));

CREATE INDEX commission_cycles_end_analysis ON commission_cycles USING btree (brand_id, window_to, id);

CREATE INDEX commission_cycles_history ON commission_cycles USING btree (brand_id, created_at DESC, id DESC);

CREATE INDEX commission_discovery_brand ON commission_discovery USING btree (brand_id, created_at DESC, id DESC);

CREATE INDEX commission_discovery_due ON commission_discovery USING btree (next_check_at, id) WHERE (state = 'pending'::text);

CREATE UNIQUE INDEX commission_notification_event_once ON outbox_events USING btree (brand_id, event_type, aggregate_id) WHERE (event_type = ANY (ARRAY['commission.paid'::text, 'commission.adjusted'::text]));

CREATE UNIQUE INDEX commission_payment_active_cycle ON commission_payments USING btree (cycle_id) WHERE (state <> 'stale'::text);

CREATE INDEX commission_payment_due ON commission_payments USING btree (next_work_at, id) WHERE (state <> ALL (ARRAY['stale'::text, 'blocked'::text]));

CREATE INDEX commission_report_posted_business_entries ON point_ledger_entries USING btree (brand_id, created_at, id) WHERE (entry_type = ANY (ARRAY['commission'::text, 'commission_adjustment'::text]));

CREATE INDEX commission_target_scan ON commission_cycle_targets USING btree (cycle_id, placed_at, order_id);

CREATE INDEX commission_target_xid ON commission_cycle_targets USING btree (cycle_id, creation_xid);

CREATE INDEX compliance_decisions_operation_page ON compliance_decisions USING btree (brand_id, operation, created_at DESC, id DESC);

CREATE INDEX compliance_decisions_page ON compliance_decisions USING btree (brand_id, created_at DESC, id DESC);

CREATE INDEX compliance_gate_rejections_operation_page ON compliance_gate_rejections USING btree (brand_id, operation, created_at DESC, id DESC);

CREATE INDEX compliance_gate_rejections_page ON compliance_gate_rejections USING btree (brand_id, created_at DESC, id DESC);

CREATE INDEX draw_attempts_period_history ON draw_attempt_batches USING btree (brand_id, period_id, created_at DESC, id DESC);

CREATE UNIQUE INDEX draw_notification_event_once ON outbox_events USING btree (brand_id, event_type, aggregate_id) WHERE (event_type = ANY (ARRAY['draw.result.published'::text, 'draw.result.corrected'::text]));

CREATE INDEX draw_results_period_history ON draw_results USING btree (brand_id, period_id, created_at DESC, id DESC);

CREATE INDEX join_codes_owner_page ON join_codes USING btree (brand_id, owner_member_id, created_at, id);

CREATE INDEX ledger_account_created ON point_ledger_entries USING btree (brand_id, account_id, created_at);

CREATE INDEX ledger_member_version ON point_ledger_entries USING btree (brand_id, member_id, version DESC);

CREATE INDEX ledger_reference ON point_ledger_entries USING btree (brand_id, reference_type, reference_id);

CREATE INDEX ledger_report_created ON point_ledger_entries USING btree (brand_id, created_at, id);

CREATE UNIQUE INDEX ledger_single_reversal ON point_ledger_entries USING btree (reversal_of) WHERE (reversal_of IS NOT NULL);

CREATE INDEX members_brand_created ON brand_members USING btree (brand_id, joined_at);

CREATE INDEX notification_delivery_pending ON notification_deliveries USING btree (next_attempt_at, event_id) WHERE (status = 'pending'::text);

CREATE INDEX notifications_member_page ON notifications USING btree (brand_id, member_id, created_at DESC, id DESC);

CREATE INDEX notifications_member_unread ON notifications USING btree (brand_id, member_id) WHERE (read_at IS NULL);

CREATE UNIQUE INDEX one_active_draw_correction ON draw_corrections USING btree (brand_id, period_id) WHERE (state <> 'completed'::text);

CREATE UNIQUE INDEX one_active_point_reconciliation ON point_reconciliation_jobs USING btree (brand_id) WHERE (state = ANY (ARRAY['pending'::text, 'running'::text]));

CREATE UNIQUE INDEX one_active_rule_per_play ON rule_versions USING btree (brand_id, game_id, play_id) WHERE (status = 'active'::text);

CREATE UNIQUE INDEX one_active_withdrawal ON withdrawal_orders USING btree (brand_id, member_id) WHERE (state = ANY (ARRAY['reviewing'::text, 'processing'::text]));

CREATE UNIQUE INDEX one_manual_source_per_game ON draw_sources USING btree (brand_id, game_id) WHERE (type = 'manual'::text);

CREATE UNIQUE INDEX one_scheduled_rule_per_play ON rule_versions USING btree (brand_id, game_id, play_id) WHERE (status = 'approved'::text);

CREATE INDEX outbox_unpublished ON outbox_events USING btree (created_at) WHERE (published_at IS NULL);

CREATE INDEX pending_draw_corrections ON draw_corrections USING btree (created_at, id) WHERE (state = 'reversing'::text);

CREATE INDEX periods_brand_draw_archive ON periods USING btree (brand_id, draw_at DESC, sequence DESC, id DESC);

CREATE INDEX periods_draw_poll ON periods USING btree (draw_next_poll_at, draw_at) WHERE ((status = 'waiting_draw'::text) AND (draw_result_id IS NULL));

CREATE INDEX periods_due ON periods USING btree (bet_start_at, bet_end_at, draw_at) WHERE (status = ANY (ARRAY['pending'::text, 'betting'::text, 'closed'::text]));

CREATE INDEX periods_game_draw_archive ON periods USING btree (brand_id, game_id, draw_at DESC, sequence DESC, id DESC);

CREATE INDEX point_reconciliation_due ON point_reconciliation_targets USING btree (job_id, next_check_at, id) WHERE (state = 'pending'::text);

CREATE INDEX point_reconciliation_job_history ON point_reconciliation_jobs USING btree (brand_id, created_at DESC, id DESC);

CREATE INDEX point_reconciliation_outcomes ON point_reconciliation_results USING btree (brand_id, job_id, outcome, target_id);

CREATE INDEX point_reconciliation_target_states ON point_reconciliation_targets USING btree (job_id, state);

CREATE INDEX point_reconciliation_worker_jobs ON point_reconciliation_jobs USING btree (last_step_at, id) WHERE (state = ANY (ARRAY['pending'::text, 'running'::text]));

CREATE INDEX point_repairs_member ON point_balance_repairs USING btree (brand_id, member_id, created_at DESC, id DESC);

CREATE INDEX recharge_brand_created ON recharge_orders USING btree (brand_id, created_at DESC, id DESC);

CREATE INDEX recharge_member_created ON recharge_orders USING btree (brand_id, member_id, created_at DESC, id DESC);

CREATE INDEX report_archive_automatic_pending ON report_archive_automatic_tasks USING btree (created_at, id) WHERE (state = 'pending'::text);

CREATE INDEX report_archive_history ON report_archives USING btree (brand_id, created_at DESC, id DESC);

CREATE UNIQUE INDEX report_archive_one_automatic_task ON report_archives USING btree (automatic_task_id) WHERE (automatic_task_id IS NOT NULL);

CREATE UNIQUE INDEX reward_notification_event_once ON outbox_events USING btree (brand_id, event_type, aggregate_id) WHERE (event_type = ANY (ARRAY['reward.order.granted'::text, 'reward.order.revocation_pending'::text, 'reward.order.revoked'::text]));

CREATE INDEX reward_orders_page ON reward_orders USING btree (brand_id, created_at DESC, id DESC);

CREATE INDEX reward_posting_report_window ON point_ledger_entries USING btree (brand_id, created_at, id) WHERE ((entry_type = ANY (ARRAY['reward_grant'::text, 'reward_reversal'::text])) AND (reference_type = 'reward_order'::text));

CREATE INDEX roles_brand_active ON roles USING btree (brand_id, status);

CREATE INDEX rules_play_history ON rule_versions USING btree (brand_id, play_id, version_no DESC);

CREATE INDEX sessions_user_active ON sessions USING btree (brand_id, member_id) WHERE (revoked_at IS NULL);

CREATE INDEX settlement_jobs_pending ON settlement_jobs USING btree (created_at, id) WHERE (state = ANY (ARRAY['processing'::text, 'paying'::text]));

CREATE INDEX settlement_preview_order_history ON settlement_previews USING btree (brand_id, order_id, created_at DESC, id DESC);

CREATE INDEX withdrawal_brand_queue ON withdrawal_orders USING btree (brand_id, state, created_at, id);

CREATE INDEX withdrawal_member_history ON withdrawal_orders USING btree (brand_id, member_id, created_at DESC, id DESC);

CREATE INDEX withdrawal_policy_history ON withdrawal_policy_revisions USING btree (brand_id, game_id, version DESC);

CREATE UNIQUE INDEX withdrawal_transition_outbox_unique ON outbox_events USING btree (brand_id, aggregate_id, ((payload ->> 'version'::text))) WHERE (event_type ~~ 'withdrawal.order.%'::text);

CREATE TRIGGER admin_role_brand_guard BEFORE INSERT OR UPDATE ON admin_account_roles FOR EACH ROW EXECUTE FUNCTION enforce_admin_role_brand();

CREATE TRIGGER admin_scope_assignment_guard BEFORE DELETE OR UPDATE ON admin_brand_scopes FOR EACH ROW EXECUTE FUNCTION protect_assigned_admin_scope();

CREATE CONSTRAINT TRIGGER agent_node_revision_required AFTER INSERT OR UPDATE ON agent_nodes DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_agent_revision();

CREATE CONSTRAINT TRIGGER agent_policy_revision_required AFTER INSERT OR UPDATE ON brand_agent_policies DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_agent_revision();

CREATE CONSTRAINT TRIGGER agent_revision_committed AFTER INSERT ON agent_config_revisions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION agent_revision_committed();

CREATE TRIGGER audit_immutable BEFORE DELETE OR UPDATE ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();

CREATE CONSTRAINT TRIGGER bet_exception_final_state AFTER INSERT ON bet_order_exceptions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_bet_exception_final_state();

CREATE CONSTRAINT TRIGGER bet_judgment_final_state AFTER INSERT ON bet_order_judgments DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_bet_judgment_final_state();

CREATE TRIGGER bet_order_admission_guard BEFORE INSERT OR DELETE ON bet_orders FOR EACH ROW EXECUTE FUNCTION guard_bet_order();

CREATE TRIGGER brand_agent_policy AFTER INSERT ON brands FOR EACH ROW EXECUTE FUNCTION initialize_agent_policy();

CREATE TRIGGER brand_commission_policy AFTER INSERT ON brands FOR EACH ROW EXECUTE FUNCTION initialize_commission_policy();

CREATE TRIGGER brand_compliance_initialized AFTER INSERT ON brands FOR EACH ROW EXECUTE FUNCTION initialize_brand_compliance();

CREATE TRIGGER brand_default_bet_policy AFTER INSERT ON brands FOR EACH ROW EXECUTE FUNCTION initialize_bet_policy();

CREATE TRIGGER brand_default_settlement_policy AFTER INSERT ON brands FOR EACH ROW EXECUTE FUNCTION initialize_settlement_policy();

CREATE TRIGGER brand_default_withdrawal_policy AFTER INSERT ON brands FOR EACH ROW EXECUTE FUNCTION initialize_withdrawal_policy();

CREATE TRIGGER brand_notification_templates_initialized AFTER INSERT ON brands FOR EACH ROW EXECUTE FUNCTION initialize_brand_notification_templates();

CREATE TRIGGER brand_point_policy_init AFTER INSERT ON brands FOR EACH ROW EXECUTE FUNCTION initialize_brand_point_policy();

CREATE TRIGGER brand_presentation_initialized AFTER INSERT ON brands FOR EACH ROW EXECUTE FUNCTION initialize_brand_presentation();

CREATE CONSTRAINT TRIGGER brand_withdrawal_policy_revision_required AFTER INSERT OR UPDATE ON brand_withdrawal_policies DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_withdrawal_policy_current();

CREATE CONSTRAINT TRIGGER cancellation_final_state AFTER INSERT ON period_cancellations DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_period_cancellation_start();

CREATE CONSTRAINT TRIGGER cancellation_job_progress AFTER UPDATE ON period_cancellations DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_cancellation_progress();

CREATE CONSTRAINT TRIGGER cancellation_target_progress AFTER UPDATE ON period_cancellation_targets DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_cancellation_progress();

CREATE TRIGGER capture_bet_commission_snapshot BEFORE INSERT OR UPDATE ON bet_orders FOR EACH ROW EXECUTE FUNCTION capture_bet_commission_snapshot();

CREATE TRIGGER capture_bet_withdrawal_snapshot BEFORE INSERT ON bet_orders FOR EACH ROW EXECUTE FUNCTION capture_bet_withdrawal_snapshot();

CREATE TRIGGER captured_commission_correction_balance_head AFTER UPDATE ON commission_correction_execution_targets FOR EACH ROW EXECUTE FUNCTION capture_commission_correction_balance_head();

CREATE TRIGGER captured_commission_correction_policy AFTER INSERT OR UPDATE ON brand_commission_correction_policies FOR EACH ROW EXECUTE FUNCTION capture_commission_correction_policy();

CREATE TRIGGER captured_report_archive_policy AFTER INSERT OR UPDATE ON brand_report_archive_policies FOR EACH ROW EXECUTE FUNCTION capture_report_archive_policy();

CREATE TRIGGER commission_adjusted_notification AFTER UPDATE ON commission_adjustments FOR EACH ROW EXECUTE FUNCTION emit_commission_adjusted_notification();

CREATE CONSTRAINT TRIGGER commission_adjusted_notification_commit AFTER UPDATE ON commission_adjustments DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_commission_notification_commit();

CREATE CONSTRAINT TRIGGER commission_adjustment_commit AFTER INSERT ON commission_adjustments DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_commission_adjustment_commit();

CREATE TRIGGER commission_adjustment_head_init AFTER UPDATE ON commission_payment_targets FOR EACH ROW EXECUTE FUNCTION initialize_commission_adjustment_head();

CREATE CONSTRAINT TRIGGER commission_adjustment_ledger_commit AFTER INSERT ON point_ledger_entries DEFERRABLE INITIALLY DEFERRED FOR EACH ROW WHEN (((new.entry_type = 'commission_adjustment'::text) OR (new.reference_type = 'commission_adjustment'::text))) EXECUTE FUNCTION require_commission_adjustment_ledger_commit();

CREATE TRIGGER commission_closed_window_admission BEFORE INSERT ON bet_orders FOR EACH ROW EXECUTE FUNCTION guard_closed_commission_admission();

CREATE CONSTRAINT TRIGGER commission_correction_execution_commit AFTER INSERT OR UPDATE ON commission_correction_executions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_commission_correction_execution_commit();

CREATE CONSTRAINT TRIGGER commission_correction_execution_step_commit AFTER INSERT ON commission_correction_execution_steps DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_commission_correction_execution_step_commit();

CREATE CONSTRAINT TRIGGER commission_correction_execution_target_commit AFTER INSERT OR UPDATE ON commission_correction_execution_targets DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_commission_correction_execution_target_commit();

CREATE CONSTRAINT TRIGGER commission_correction_ledger_commit AFTER INSERT ON point_ledger_entries DEFERRABLE INITIALLY DEFERRED FOR EACH ROW WHEN (((new.entry_type = 'commission_correction'::text) OR (new.reference_type = 'commission_correction_target'::text))) EXECUTE FUNCTION require_commission_correction_ledger_commit();

CREATE TRIGGER commission_correction_notification AFTER UPDATE ON commission_correction_execution_targets FOR EACH ROW EXECUTE FUNCTION emit_commission_correction_notification();

CREATE CONSTRAINT TRIGGER commission_correction_notification_commit AFTER UPDATE ON commission_correction_execution_targets DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_commission_correction_notification_commit();

CREATE CONSTRAINT TRIGGER commission_correction_plan_commit AFTER INSERT OR UPDATE ON commission_correction_plans DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_commission_correction_plan_commit();

CREATE CONSTRAINT TRIGGER commission_correction_plan_step_commit AFTER INSERT ON commission_correction_plan_steps DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_commission_correction_plan_step_commit();

CREATE CONSTRAINT TRIGGER commission_correction_plan_target_commit AFTER INSERT ON commission_correction_plan_targets DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_commission_correction_plan_target_commit();

CREATE TRIGGER commission_correction_policy_init AFTER INSERT ON brands FOR EACH ROW EXECUTE FUNCTION initialize_commission_correction_policy();

CREATE TRIGGER commission_order_epoch AFTER UPDATE ON bet_orders FOR EACH ROW EXECUTE FUNCTION advance_commission_evidence_epoch();

CREATE TRIGGER commission_paid_notification AFTER UPDATE ON commission_payment_targets FOR EACH ROW EXECUTE FUNCTION emit_commission_paid_notification();

CREATE CONSTRAINT TRIGGER commission_paid_notification_commit AFTER UPDATE ON commission_payment_targets DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_commission_notification_commit();

CREATE CONSTRAINT TRIGGER commission_payment_credit_commit AFTER INSERT ON point_ledger_entries DEFERRABLE INITIALLY DEFERRED FOR EACH ROW WHEN (((new.entry_type = 'commission'::text) OR (new.reference_type = 'commission_payment_target'::text))) EXECUTE FUNCTION require_commission_payment_credit_commit();

CREATE TRIGGER commission_payment_policy_init AFTER INSERT ON brands FOR EACH ROW EXECUTE FUNCTION initialize_commission_payment_policy();

CREATE TRIGGER commission_payment_policy_revision AFTER INSERT OR UPDATE ON brand_commission_payment_policies FOR EACH ROW EXECUTE FUNCTION capture_commission_payment_policy();

CREATE CONSTRAINT TRIGGER commission_payment_target_commit AFTER UPDATE ON commission_payment_targets DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_commission_payment_target_commit();

CREATE TRIGGER commission_period_epoch AFTER UPDATE ON periods FOR EACH ROW EXECUTE FUNCTION advance_commission_evidence_epoch();

CREATE TRIGGER commission_point_buckets AFTER INSERT ON point_accounts FOR EACH ROW EXECUTE FUNCTION initialize_commission_point_buckets();

CREATE CONSTRAINT TRIGGER commission_policy_revision_required AFTER INSERT OR UPDATE ON brand_commission_policies DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_commission_policy_revision();

CREATE CONSTRAINT TRIGGER commission_revision_committed AFTER INSERT ON commission_policy_revisions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION commission_policy_revision_committed();

CREATE CONSTRAINT TRIGGER commission_run_commit AFTER INSERT ON commission_runs DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_commission_run_commit();

CREATE CONSTRAINT TRIGGER commission_step_commit AFTER INSERT ON commission_cycle_steps DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_commission_step_commit();

CREATE CONSTRAINT TRIGGER complete_point_reconciliation_job AFTER INSERT OR UPDATE ON point_reconciliation_jobs DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_point_reconciliation_job();

CREATE CONSTRAINT TRIGGER compliance_history_required AFTER INSERT OR UPDATE ON brand_compliance_policies DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_compliance_history();

CREATE CONSTRAINT TRIGGER correction_final_state AFTER INSERT OR UPDATE ON draw_corrections DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_correction_final_state();

CREATE CONSTRAINT TRIGGER correction_target_final_state AFTER INSERT OR UPDATE ON draw_correction_targets DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_correction_final_state();

CREATE TRIGGER current_calculation_guard BEFORE INSERT ON settlement_calculations FOR EACH ROW EXECUTE FUNCTION guard_current_settlement_write();

CREATE TRIGGER current_payout_guard BEFORE UPDATE ON bet_orders FOR EACH ROW EXECUTE FUNCTION guard_current_settlement_write();

CREATE CONSTRAINT TRIGGER domain_history_required AFTER UPDATE ON brand_domains DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_brand_domain_history();

CREATE CONSTRAINT TRIGGER draw_notification_publication_commit AFTER INSERT ON draw_notification_publications DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_draw_notification_publication_commit();

CREATE TRIGGER enqueue_commission_discovery AFTER INSERT ON bet_orders FOR EACH ROW EXECUTE FUNCTION enqueue_commission_discovery();

CREATE TRIGGER enqueue_in_app_event AFTER INSERT ON outbox_events FOR EACH ROW EXECUTE FUNCTION enqueue_in_app_event();

CREATE TRIGGER game_default_bet_policy AFTER INSERT ON games FOR EACH ROW EXECUTE FUNCTION initialize_bet_policy();

CREATE TRIGGER game_default_withdrawal_policy AFTER INSERT ON games FOR EACH ROW EXECUTE FUNCTION initialize_withdrawal_policy();

CREATE CONSTRAINT TRIGGER game_withdrawal_policy_revision_required AFTER INSERT OR UPDATE ON game_withdrawal_policies DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_withdrawal_policy_current();

CREATE TRIGGER guarded_agency_commission_calendar BEFORE UPDATE OF config ON brand_agent_policies FOR EACH ROW EXECUTE FUNCTION guard_agency_commission_calendar();

CREATE TRIGGER guarded_agent_node BEFORE INSERT OR DELETE OR UPDATE ON agent_nodes FOR EACH ROW EXECUTE FUNCTION guard_agent_node();

CREATE TRIGGER guarded_agent_policy BEFORE INSERT OR DELETE OR UPDATE ON brand_agent_policies FOR EACH ROW EXECUTE FUNCTION guard_agent_policy();

CREATE TRIGGER guarded_agent_revision BEFORE INSERT OR DELETE OR UPDATE ON agent_config_revisions FOR EACH ROW EXECUTE FUNCTION guard_agent_revision();

CREATE TRIGGER guarded_agent_uniform_node_mode BEFORE INSERT OR UPDATE ON agent_nodes FOR EACH ROW EXECUTE FUNCTION guard_agent_uniform_node_mode();

CREATE TRIGGER guarded_agent_uniform_policy_mode BEFORE UPDATE OF config ON brand_agent_policies FOR EACH ROW EXECUTE FUNCTION guard_agent_uniform_policy_mode();

CREATE TRIGGER guarded_bet_judgment BEFORE INSERT OR DELETE OR UPDATE ON bet_order_judgments FOR EACH ROW EXECUTE FUNCTION guard_bet_judgment();

CREATE TRIGGER guarded_brand_withdrawal_policy BEFORE INSERT OR DELETE OR UPDATE ON brand_withdrawal_policies FOR EACH ROW EXECUTE FUNCTION guard_withdrawal_policy_current();

CREATE TRIGGER guarded_cancellation_target BEFORE INSERT OR DELETE OR UPDATE ON period_cancellation_targets FOR EACH ROW EXECUTE FUNCTION guard_cancellation_target();

CREATE TRIGGER guarded_commission_adjustment BEFORE INSERT OR DELETE OR UPDATE ON commission_adjustments FOR EACH ROW EXECUTE FUNCTION guard_commission_adjustment();

CREATE TRIGGER guarded_commission_adjustment_credit BEFORE INSERT ON point_ledger_entries FOR EACH ROW EXECUTE FUNCTION guard_commission_adjustment_credit();

CREATE TRIGGER guarded_commission_adjustment_head BEFORE INSERT OR DELETE OR UPDATE ON commission_adjustment_heads FOR EACH ROW EXECUTE FUNCTION guard_commission_adjustment_head();

CREATE TRIGGER guarded_commission_allocation BEFORE INSERT OR DELETE OR UPDATE ON commission_allocations FOR EACH ROW EXECUTE FUNCTION guard_commission_allocation();

CREATE TRIGGER guarded_commission_calculation BEFORE INSERT OR DELETE OR UPDATE ON commission_calculations FOR EACH ROW EXECUTE FUNCTION guard_commission_calculation();

CREATE TRIGGER guarded_commission_correction_balance_head BEFORE INSERT OR DELETE OR UPDATE ON commission_correction_balance_heads FOR EACH ROW EXECUTE FUNCTION guard_commission_correction_balance_head();

CREATE TRIGGER guarded_commission_correction_cycle_hold BEFORE INSERT OR DELETE OR UPDATE ON commission_correction_cycle_holds FOR EACH ROW EXECUTE FUNCTION guard_commission_correction_cycle_hold();

CREATE TRIGGER guarded_commission_correction_execution BEFORE INSERT OR DELETE OR UPDATE ON commission_correction_executions FOR EACH ROW EXECUTE FUNCTION guard_commission_correction_execution();

CREATE TRIGGER guarded_commission_correction_execution_step BEFORE INSERT OR DELETE OR UPDATE ON commission_correction_execution_steps FOR EACH ROW EXECUTE FUNCTION guard_commission_correction_execution_step();

CREATE TRIGGER guarded_commission_correction_execution_target BEFORE INSERT OR DELETE OR UPDATE ON commission_correction_execution_targets FOR EACH ROW EXECUTE FUNCTION guard_commission_correction_execution_target();

CREATE TRIGGER guarded_commission_correction_notification_outbox BEFORE INSERT OR DELETE OR UPDATE ON outbox_events FOR EACH ROW EXECUTE FUNCTION guard_commission_correction_notification_outbox();

CREATE TRIGGER guarded_commission_correction_plan BEFORE INSERT OR DELETE OR UPDATE ON commission_correction_plans FOR EACH ROW EXECUTE FUNCTION guard_commission_correction_plan();

CREATE TRIGGER guarded_commission_correction_plan_step BEFORE INSERT OR DELETE OR UPDATE ON commission_correction_plan_steps FOR EACH ROW EXECUTE FUNCTION guard_commission_correction_plan_step();

CREATE TRIGGER guarded_commission_correction_plan_target BEFORE INSERT OR DELETE OR UPDATE ON commission_correction_plan_targets FOR EACH ROW EXECUTE FUNCTION guard_commission_correction_plan_target();

CREATE TRIGGER guarded_commission_correction_policy BEFORE INSERT OR DELETE OR UPDATE ON brand_commission_correction_policies FOR EACH ROW EXECUTE FUNCTION guard_commission_correction_policy();

CREATE TRIGGER guarded_commission_correction_policy_revision BEFORE INSERT OR DELETE OR UPDATE ON commission_correction_policy_revisions FOR EACH ROW EXECUTE FUNCTION guard_commission_correction_policy_revision();

CREATE TRIGGER guarded_commission_cycle BEFORE INSERT OR DELETE OR UPDATE ON commission_cycles FOR EACH ROW EXECUTE FUNCTION guard_commission_cycle();

CREATE TRIGGER guarded_commission_discovery BEFORE INSERT OR DELETE OR UPDATE ON commission_discovery FOR EACH ROW EXECUTE FUNCTION guard_commission_discovery();

CREATE TRIGGER guarded_commission_earning BEFORE INSERT OR DELETE OR UPDATE ON commission_earnings FOR EACH ROW EXECUTE FUNCTION guard_commission_earning();

CREATE TRIGGER guarded_commission_notification_outbox BEFORE INSERT OR DELETE OR UPDATE ON outbox_events FOR EACH ROW EXECUTE FUNCTION guard_commission_notification_outbox();

CREATE TRIGGER guarded_commission_payment BEFORE INSERT OR DELETE OR UPDATE ON commission_payments FOR EACH ROW EXECUTE FUNCTION guard_commission_payment();

CREATE TRIGGER guarded_commission_payment_credit BEFORE INSERT ON point_ledger_entries FOR EACH ROW EXECUTE FUNCTION guard_commission_payment_credit();

CREATE TRIGGER guarded_commission_payment_policy BEFORE INSERT OR DELETE OR UPDATE ON brand_commission_payment_policies FOR EACH ROW EXECUTE FUNCTION guard_commission_payment_policy();

CREATE TRIGGER guarded_commission_payment_policy_revision BEFORE INSERT OR DELETE OR UPDATE ON commission_payment_policy_revisions FOR EACH ROW EXECUTE FUNCTION guard_commission_payment_policy_revision();

CREATE TRIGGER guarded_commission_payment_target BEFORE INSERT OR DELETE OR UPDATE ON commission_payment_targets FOR EACH ROW EXECUTE FUNCTION guard_commission_payment_target();

CREATE TRIGGER guarded_commission_policy BEFORE INSERT OR DELETE OR UPDATE ON brand_commission_policies FOR EACH ROW EXECUTE FUNCTION guard_commission_policy();

CREATE TRIGGER guarded_commission_revision BEFORE INSERT OR DELETE OR UPDATE ON commission_policy_revisions FOR EACH ROW EXECUTE FUNCTION guard_commission_policy_revision();

CREATE TRIGGER guarded_commission_run BEFORE INSERT OR DELETE OR UPDATE ON commission_runs FOR EACH ROW EXECUTE FUNCTION guard_commission_run();

CREATE TRIGGER guarded_commission_step BEFORE INSERT OR DELETE OR UPDATE ON commission_cycle_steps FOR EACH ROW EXECUTE FUNCTION guard_commission_step();

CREATE TRIGGER guarded_commission_target BEFORE INSERT OR DELETE OR UPDATE ON commission_cycle_targets FOR EACH ROW EXECUTE FUNCTION guard_commission_target();

CREATE TRIGGER guarded_compliance_decision BEFORE INSERT OR DELETE OR UPDATE ON compliance_decisions FOR EACH ROW EXECUTE FUNCTION guard_compliance_decision();

CREATE TRIGGER guarded_compliance_gate_rejection BEFORE INSERT OR DELETE OR UPDATE ON compliance_gate_rejections FOR EACH ROW EXECUTE FUNCTION guard_compliance_gate_rejection();

CREATE TRIGGER guarded_compliance_policy BEFORE INSERT OR DELETE OR UPDATE ON brand_compliance_policies FOR EACH ROW EXECUTE FUNCTION guard_compliance_policy();

CREATE TRIGGER guarded_compliance_revision BEFORE INSERT OR DELETE OR UPDATE ON compliance_policy_revisions FOR EACH ROW EXECUTE FUNCTION guard_compliance_revision();

CREATE TRIGGER guarded_domain_binding BEFORE DELETE OR UPDATE ON brand_domains FOR EACH ROW EXECUTE FUNCTION guard_brand_domain_binding();

CREATE TRIGGER guarded_domain_history BEFORE INSERT OR DELETE OR UPDATE ON brand_domain_revisions FOR EACH ROW EXECUTE FUNCTION guard_brand_domain_revision();

CREATE TRIGGER guarded_draw_correction BEFORE INSERT OR DELETE OR UPDATE ON draw_corrections FOR EACH ROW EXECUTE FUNCTION guard_draw_correction();

CREATE TRIGGER guarded_draw_correction_target BEFORE INSERT OR DELETE OR UPDATE ON draw_correction_targets FOR EACH ROW EXECUTE FUNCTION guard_draw_correction_target();

CREATE TRIGGER guarded_draw_notification_content BEFORE INSERT ON notifications FOR EACH ROW EXECUTE FUNCTION guard_draw_notification_content();

CREATE TRIGGER guarded_draw_notification_outbox BEFORE INSERT OR DELETE OR UPDATE ON outbox_events FOR EACH ROW EXECUTE FUNCTION guard_draw_notification_outbox();

CREATE TRIGGER guarded_draw_notification_publication BEFORE INSERT OR DELETE OR UPDATE ON draw_notification_publications FOR EACH ROW EXECUTE FUNCTION guard_draw_notification_publication();

CREATE TRIGGER guarded_draw_notification_recipient BEFORE INSERT OR DELETE OR UPDATE ON draw_notification_recipients FOR EACH ROW EXECUTE FUNCTION guard_draw_notification_recipient();

CREATE TRIGGER guarded_game_withdrawal_policy BEFORE INSERT OR DELETE OR UPDATE ON game_withdrawal_policies FOR EACH ROW EXECUTE FUNCTION guard_withdrawal_policy_current();

CREATE TRIGGER guarded_join_code BEFORE INSERT OR DELETE OR UPDATE ON join_codes FOR EACH ROW EXECUTE FUNCTION guard_join_code();

CREATE TRIGGER guarded_notification_template BEFORE INSERT OR DELETE OR UPDATE ON notification_templates FOR EACH ROW EXECUTE FUNCTION guard_notification_template();

CREATE TRIGGER guarded_notification_template_revision BEFORE INSERT OR DELETE OR UPDATE ON notification_template_revisions FOR EACH ROW EXECUTE FUNCTION guard_notification_template_revision();

CREATE TRIGGER guarded_period_cancellation BEFORE INSERT OR DELETE OR UPDATE ON period_cancellations FOR EACH ROW EXECUTE FUNCTION guard_period_cancellation();

CREATE TRIGGER guarded_point_reconciliation_job BEFORE INSERT OR DELETE OR UPDATE ON point_reconciliation_jobs FOR EACH ROW EXECUTE FUNCTION guard_point_reconciliation_job();

CREATE TRIGGER guarded_point_reconciliation_scope BEFORE INSERT OR UPDATE ON point_reconciliation_jobs FOR EACH ROW EXECUTE FUNCTION guard_point_reconciliation_scope();

CREATE TRIGGER guarded_point_reconciliation_target BEFORE INSERT OR DELETE OR UPDATE ON point_reconciliation_targets FOR EACH ROW EXECUTE FUNCTION guard_point_reconciliation_target();

CREATE TRIGGER guarded_presentation_history BEFORE INSERT OR DELETE OR UPDATE ON brand_presentation_revisions FOR EACH ROW EXECUTE FUNCTION guard_brand_presentation_revision();

CREATE TRIGGER guarded_presentation_update BEFORE DELETE OR UPDATE ON brand_presentations FOR EACH ROW EXECUTE FUNCTION guard_brand_presentation();

CREATE TRIGGER guarded_report_archive_automatic_task BEFORE INSERT OR DELETE OR UPDATE ON report_archive_automatic_tasks FOR EACH ROW EXECUTE FUNCTION guard_report_archive_automatic_task();

CREATE TRIGGER guarded_report_archive_cursor BEFORE INSERT OR DELETE OR UPDATE ON report_archive_automatic_cursors FOR EACH ROW EXECUTE FUNCTION guard_report_archive_cursor();

CREATE TRIGGER guarded_report_archive_policy BEFORE INSERT OR DELETE OR UPDATE ON brand_report_archive_policies FOR EACH ROW EXECUTE FUNCTION guard_report_archive_policy();

CREATE TRIGGER guarded_report_archive_policy_revision BEFORE INSERT OR DELETE OR UPDATE ON report_archive_policy_revisions FOR EACH ROW EXECUTE FUNCTION guard_report_archive_policy_revision();

CREATE TRIGGER guarded_report_archive_truncate BEFORE TRUNCATE ON report_archives FOR EACH STATEMENT EXECUTE FUNCTION guard_report_archive_version();

CREATE TRIGGER guarded_report_archive_version BEFORE INSERT OR DELETE OR UPDATE ON report_archives FOR EACH ROW EXECUTE FUNCTION guard_report_archive_version();

CREATE TRIGGER guarded_reward_action BEFORE INSERT OR DELETE OR UPDATE ON reward_order_actions FOR EACH ROW EXECUTE FUNCTION guard_reward_action();

CREATE TRIGGER guarded_reward_credit BEFORE INSERT ON point_ledger_entries FOR EACH ROW EXECUTE FUNCTION guard_reward_credit();

CREATE TRIGGER guarded_reward_notification_outbox BEFORE INSERT OR DELETE OR UPDATE ON outbox_events FOR EACH ROW EXECUTE FUNCTION guard_reward_notification_outbox();

CREATE TRIGGER guarded_reward_order BEFORE INSERT OR DELETE OR UPDATE ON reward_orders FOR EACH ROW EXECUTE FUNCTION guard_reward_order();

CREATE TRIGGER guarded_settlement_job BEFORE INSERT OR DELETE OR UPDATE ON settlement_jobs FOR EACH ROW EXECUTE FUNCTION guard_settlement_job();

CREATE TRIGGER guarded_settlement_policy BEFORE DELETE OR UPDATE ON brand_settlement_policies FOR EACH ROW EXECUTE FUNCTION guard_settlement_policy();

CREATE TRIGGER guarded_settlement_target BEFORE INSERT OR DELETE OR UPDATE ON settlement_targets FOR EACH ROW EXECUTE FUNCTION guard_settlement_target();

CREATE TRIGGER guarded_unexecuted_commission_correction_credit BEFORE INSERT ON point_ledger_entries FOR EACH ROW EXECUTE FUNCTION guard_unexecuted_commission_correction_credit();

CREATE TRIGGER guarded_withdrawal_policy_revision BEFORE INSERT OR DELETE OR UPDATE ON withdrawal_policy_revisions FOR EACH ROW EXECUTE FUNCTION guard_withdrawal_policy_revision();

CREATE TRIGGER immutable_bet_exception BEFORE INSERT OR DELETE OR UPDATE ON bet_order_exceptions FOR EACH ROW EXECUTE FUNCTION guard_bet_exception();

CREATE TRIGGER immutable_bet_order BEFORE UPDATE ON bet_orders FOR EACH ROW EXECUTE FUNCTION guard_settlement_bet_update();

CREATE TRIGGER immutable_bet_withdrawal_snapshot BEFORE UPDATE ON bet_orders FOR EACH ROW EXECUTE FUNCTION guard_bet_withdrawal_snapshot_update();

CREATE TRIGGER immutable_brand_creation_records BEFORE DELETE OR UPDATE ON brand_creation_records FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();

CREATE TRIGGER immutable_brand_operation_revision BEFORE INSERT OR DELETE OR UPDATE ON brand_operation_revisions FOR EACH ROW EXECUTE FUNCTION guard_brand_operation_revision();

CREATE TRIGGER immutable_cancellation_failures BEFORE DELETE OR UPDATE ON period_cancellation_failures FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();

CREATE TRIGGER immutable_draw_attempt_batches BEFORE DELETE OR UPDATE ON draw_attempt_batches FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();

CREATE TRIGGER immutable_draw_correction_failures BEFORE DELETE OR UPDATE ON draw_correction_failures FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();

CREATE TRIGGER immutable_draw_results BEFORE DELETE OR UPDATE ON draw_results FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();

CREATE TRIGGER immutable_draw_source_sets BEFORE DELETE OR UPDATE ON draw_source_sets FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();

CREATE TRIGGER immutable_draw_sources BEFORE DELETE OR UPDATE ON draw_sources FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();

CREATE TRIGGER immutable_game_schedules BEFORE DELETE OR UPDATE ON game_schedules FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();

CREATE TRIGGER immutable_join_code_history BEFORE INSERT OR DELETE OR UPDATE ON join_code_revisions FOR EACH ROW EXECUTE FUNCTION guard_join_code_revision();

CREATE TRIGGER immutable_member_attribution BEFORE INSERT OR DELETE OR UPDATE ON brand_members FOR EACH ROW EXECUTE FUNCTION capture_member_attribution();

CREATE TRIGGER immutable_order_attribution BEFORE INSERT OR UPDATE ON bet_orders FOR EACH ROW EXECUTE FUNCTION capture_order_attribution();

CREATE TRIGGER immutable_period_history BEFORE DELETE OR UPDATE ON periods FOR EACH ROW EXECUTE FUNCTION guard_period_history();

CREATE TRIGGER immutable_period_rules BEFORE DELETE OR UPDATE ON period_rule_versions FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();

CREATE TRIGGER immutable_point_reconciliation_failure BEFORE INSERT OR DELETE OR UPDATE ON point_reconciliation_failures FOR EACH ROW EXECUTE FUNCTION guard_point_reconciliation_evidence();

CREATE TRIGGER immutable_point_reconciliation_result BEFORE INSERT OR DELETE OR UPDATE ON point_reconciliation_results FOR EACH ROW EXECUTE FUNCTION guard_point_reconciliation_evidence();

CREATE TRIGGER immutable_point_reconciliation_retry BEFORE INSERT OR DELETE OR UPDATE ON point_reconciliation_retries FOR EACH ROW EXECUTE FUNCTION guard_point_reconciliation_evidence();

CREATE TRIGGER immutable_rule_contributors BEFORE DELETE OR UPDATE ON rule_version_contributors FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();

CREATE TRIGGER immutable_rule_history BEFORE DELETE OR UPDATE ON rule_versions FOR EACH ROW EXECUTE FUNCTION guard_rule_version_history();

CREATE TRIGGER immutable_settlement_calculation BEFORE DELETE OR UPDATE ON settlement_calculations FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();

CREATE TRIGGER immutable_settlement_failure BEFORE DELETE OR UPDATE ON settlement_failures FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();

CREATE TRIGGER immutable_settlement_policy_history BEFORE DELETE OR UPDATE ON settlement_policy_history FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();

CREATE TRIGGER immutable_settlement_preview BEFORE DELETE OR UPDATE ON settlement_previews FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();

CREATE TRIGGER initial_agent_policy_revision AFTER INSERT ON brand_agent_policies FOR EACH ROW EXECUTE FUNCTION initialize_agent_policy_revision();

CREATE TRIGGER initial_brand_withdrawal_policy_revision AFTER INSERT ON brand_withdrawal_policies FOR EACH ROW EXECUTE FUNCTION initialize_withdrawal_policy_revision();

CREATE TRIGGER initial_commission_revision AFTER INSERT ON brand_commission_policies FOR EACH ROW EXECUTE FUNCTION initialize_commission_policy_revision();

CREATE TRIGGER initial_game_withdrawal_policy_revision AFTER INSERT ON game_withdrawal_policies FOR EACH ROW EXECUTE FUNCTION initialize_withdrawal_policy_revision();

CREATE TRIGGER initialized_report_archive_policy AFTER INSERT ON brands FOR EACH ROW EXECUTE FUNCTION initialize_report_archive_policy();

CREATE CONSTRAINT TRIGGER join_code_history_complete AFTER INSERT ON join_code_revisions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_join_code_revision();

CREATE CONSTRAINT TRIGGER join_code_revision_required AFTER INSERT OR UPDATE ON join_codes DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_join_code_revision();

CREATE CONSTRAINT TRIGGER ledger_four_source_projection AFTER INSERT ON point_ledger_entries DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_point_ledger_projection();

CREATE TRIGGER ledger_four_source_snapshot BEFORE INSERT ON point_ledger_entries FOR EACH ROW EXECUTE FUNCTION guard_point_ledger_snapshot();

CREATE TRIGGER ledger_immutable BEFORE DELETE OR UPDATE ON point_ledger_entries FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();

CREATE TRIGGER notification_immutable BEFORE DELETE OR UPDATE ON notifications FOR EACH ROW EXECUTE FUNCTION guard_notification_content();

CREATE CONSTRAINT TRIGGER notification_template_history_required AFTER INSERT OR UPDATE ON notification_templates DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_notification_template_history();

CREATE TRIGGER notification_template_snapshot_required BEFORE INSERT ON notifications FOR EACH ROW EXECUTE FUNCTION guard_notification_template_snapshot();

CREATE CONSTRAINT TRIGGER period_settlement_witness AFTER UPDATE ON periods DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_period_settlement_witness();

CREATE TRIGGER point_repairs_append_only BEFORE DELETE OR UPDATE ON point_balance_repairs FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();

CREATE CONSTRAINT TRIGGER presentation_history_required AFTER UPDATE ON brand_presentations DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_brand_presentation_history();

CREATE CONSTRAINT TRIGGER required_report_archive_automatic_link AFTER INSERT ON report_archives DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_report_archive_automatic_link();

CREATE CONSTRAINT TRIGGER reward_action_commit AFTER INSERT ON reward_order_actions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_reward_action_commit();

CREATE CONSTRAINT TRIGGER reward_ledger_commit AFTER INSERT ON point_ledger_entries DEFERRABLE INITIALLY DEFERRED FOR EACH ROW WHEN (((new.entry_type = ANY (ARRAY['reward_grant'::text, 'reward_reversal'::text])) OR (new.reference_type = 'reward_order'::text))) EXECUTE FUNCTION require_reward_ledger_commit();

CREATE TRIGGER reward_notification AFTER UPDATE ON reward_orders FOR EACH ROW EXECUTE FUNCTION emit_reward_notification();

CREATE CONSTRAINT TRIGGER reward_notification_commit AFTER INSERT ON reward_order_actions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_reward_notification_commit();

CREATE CONSTRAINT TRIGGER reward_order_commit AFTER INSERT OR UPDATE ON reward_orders DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_reward_order_commit();

CREATE TRIGGER role_identity_guard BEFORE UPDATE OF brand_id, code ON roles FOR EACH ROW EXECUTE FUNCTION immutable_admin_role_identity();

CREATE CONSTRAINT TRIGGER settlement_job_final_state AFTER INSERT OR UPDATE ON settlement_jobs DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_settlement_final_state();

CREATE TRIGGER settlement_payout_witness BEFORE UPDATE ON bet_orders FOR EACH ROW EXECUTE FUNCTION validate_settlement_payout_witness();

CREATE CONSTRAINT TRIGGER settlement_target_final_state AFTER INSERT OR UPDATE ON settlement_targets DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_settlement_final_state();

CREATE TRIGGER valid_brand_creation_record BEFORE INSERT ON brand_creation_records FOR EACH ROW EXECUTE FUNCTION validate_brand_creation_record();

CREATE TRIGGER validate_settlement_preview_context BEFORE INSERT ON settlement_previews FOR EACH ROW EXECUTE FUNCTION validate_settlement_preview_context();

CREATE TRIGGER validated_settlement_calculation BEFORE INSERT ON settlement_calculations FOR EACH ROW EXECUTE FUNCTION validate_settlement_calculation();

CREATE TRIGGER withdrawal_cycle BEFORE INSERT OR DELETE OR UPDATE ON withdrawal_turnover_cycles FOR EACH ROW EXECUTE FUNCTION guard_withdrawal_cycle();

CREATE CONSTRAINT TRIGGER withdrawal_fund_evidence AFTER INSERT OR UPDATE ON withdrawal_orders DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_withdrawal_funds();

CREATE TRIGGER withdrawal_outbox_guard BEFORE INSERT OR DELETE OR UPDATE ON outbox_events FOR EACH ROW EXECUTE FUNCTION guard_withdrawal_outbox();

CREATE CONSTRAINT TRIGGER withdrawal_policy_revision_committed AFTER INSERT ON withdrawal_policy_revisions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_withdrawal_policy_revision_final();

CREATE TRIGGER withdrawal_projection BEFORE INSERT OR DELETE OR UPDATE ON withdrawal_orders FOR EACH ROW EXECUTE FUNCTION guard_withdrawal_order_projection();

CREATE TRIGGER withdrawal_receipt_evidence BEFORE INSERT OR DELETE OR UPDATE ON withdrawal_operation_receipts FOR EACH ROW EXECUTE FUNCTION guard_withdrawal_evidence();

CREATE TRIGGER withdrawal_transition_evidence BEFORE INSERT OR DELETE OR UPDATE ON withdrawal_order_transitions FOR EACH ROW EXECUTE FUNCTION guard_withdrawal_evidence();

CREATE CONSTRAINT TRIGGER withdrawal_transition_outbox_required AFTER INSERT ON withdrawal_order_transitions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_withdrawal_transition_outbox();

ALTER TABLE ONLY admin_account_roles
    ADD CONSTRAINT admin_account_roles_account_id_fkey FOREIGN KEY (account_id) REFERENCES admin_accounts(id);

ALTER TABLE ONLY admin_account_roles
    ADD CONSTRAINT admin_account_roles_role_id_fkey FOREIGN KEY (role_id) REFERENCES roles(id);

ALTER TABLE ONLY admin_brand_scopes
    ADD CONSTRAINT admin_brand_scopes_account_id_fkey FOREIGN KEY (account_id) REFERENCES admin_accounts(id);

ALTER TABLE ONLY admin_brand_scopes
    ADD CONSTRAINT admin_brand_scopes_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brands(id);

ALTER TABLE ONLY agent_config_revisions
    ADD CONSTRAINT agent_config_revisions_audit_log_id_fkey FOREIGN KEY (audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY agent_config_revisions
    ADD CONSTRAINT agent_config_revisions_brand_id_agent_id_fkey FOREIGN KEY (brand_id, agent_id) REFERENCES agent_nodes(brand_id, id);

ALTER TABLE ONLY agent_config_revisions
    ADD CONSTRAINT agent_config_revisions_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brands(id);

ALTER TABLE ONLY agent_nodes
    ADD CONSTRAINT agent_nodes_brand_id_member_id_fkey FOREIGN KEY (brand_id, member_id) REFERENCES brand_members(brand_id, id);

ALTER TABLE ONLY agent_nodes
    ADD CONSTRAINT agent_nodes_brand_id_parent_id_fkey FOREIGN KEY (brand_id, parent_id) REFERENCES agent_nodes(brand_id, id);

ALTER TABLE ONLY agent_nodes
    ADD CONSTRAINT agent_nodes_created_by_fkey FOREIGN KEY (created_by) REFERENCES admin_accounts(id);

ALTER TABLE ONLY audit_logs
    ADD CONSTRAINT audit_logs_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brands(id);

ALTER TABLE ONLY auth_challenges
    ADD CONSTRAINT auth_challenges_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brands(id);

ALTER TABLE ONLY bet_order_exceptions
    ADD CONSTRAINT bet_order_exceptions_brand_id_job_id_fkey FOREIGN KEY (brand_id, job_id) REFERENCES settlement_jobs(brand_id, id);

ALTER TABLE ONLY bet_order_exceptions
    ADD CONSTRAINT bet_order_exceptions_brand_id_order_id_fkey FOREIGN KEY (brand_id, order_id) REFERENCES bet_orders(brand_id, id);

ALTER TABLE ONLY bet_order_exceptions
    ADD CONSTRAINT bet_order_exceptions_marked_by_fkey FOREIGN KEY (marked_by) REFERENCES admin_accounts(id);

ALTER TABLE ONLY bet_order_judgments
    ADD CONSTRAINT bet_order_judgments_brand_id_game_id_period_id_draw_result_fkey FOREIGN KEY (brand_id, game_id, period_id, draw_result_id) REFERENCES draw_results(brand_id, game_id, period_id, id);

ALTER TABLE ONLY bet_order_judgments
    ADD CONSTRAINT bet_order_judgments_brand_id_game_id_period_id_fkey FOREIGN KEY (brand_id, game_id, period_id) REFERENCES periods(brand_id, game_id, id);

ALTER TABLE ONLY bet_order_judgments
    ADD CONSTRAINT bet_order_judgments_brand_id_period_id_order_id_fkey FOREIGN KEY (brand_id, period_id, order_id) REFERENCES bet_orders(brand_id, period_id, id);

ALTER TABLE ONLY bet_order_judgments
    ADD CONSTRAINT bet_order_judgments_judged_by_fkey FOREIGN KEY (judged_by) REFERENCES admin_accounts(id);

ALTER TABLE ONLY bet_orders
    ADD CONSTRAINT bet_orders_brand_id_account_id_brand_member_id_fkey FOREIGN KEY (brand_id, account_id, brand_member_id) REFERENCES point_accounts(brand_id, id, brand_member_id);

ALTER TABLE ONLY bet_orders
    ADD CONSTRAINT bet_orders_brand_id_account_id_debit_entry_id_fkey FOREIGN KEY (brand_id, account_id, debit_entry_id) REFERENCES point_ledger_entries(brand_id, account_id, id);

ALTER TABLE ONLY bet_orders
    ADD CONSTRAINT bet_orders_brand_id_account_id_refund_entry_id_fkey FOREIGN KEY (brand_id, account_id, refund_entry_id) REFERENCES point_ledger_entries(brand_id, account_id, id);

ALTER TABLE ONLY bet_orders
    ADD CONSTRAINT bet_orders_brand_id_brand_member_id_global_user_id_fkey FOREIGN KEY (brand_id, brand_member_id, global_user_id) REFERENCES brand_members(brand_id, id, global_user_id);

ALTER TABLE ONLY bet_orders
    ADD CONSTRAINT bet_orders_brand_id_game_id_period_id_fkey FOREIGN KEY (brand_id, game_id, period_id) REFERENCES periods(brand_id, game_id, id);

ALTER TABLE ONLY bet_orders
    ADD CONSTRAINT bet_orders_brand_id_game_id_play_id_rule_version_id_fkey FOREIGN KEY (brand_id, game_id, play_id, rule_version_id) REFERENCES rule_versions(brand_id, game_id, play_id, id);

ALTER TABLE ONLY bet_orders
    ADD CONSTRAINT bet_orders_brand_id_id_settlement_calculation_id_fkey FOREIGN KEY (brand_id, id, settlement_calculation_id) REFERENCES settlement_calculations(brand_id, order_id, id);

ALTER TABLE ONLY bet_orders
    ADD CONSTRAINT bet_orders_payout_entry_id_fkey FOREIGN KEY (payout_entry_id) REFERENCES point_ledger_entries(id);

ALTER TABLE ONLY brand_agent_policies
    ADD CONSTRAINT brand_agent_policies_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brands(id);

ALTER TABLE ONLY brand_bet_policies
    ADD CONSTRAINT brand_bet_policies_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brands(id);

ALTER TABLE ONLY brand_commission_correction_policies
    ADD CONSTRAINT brand_commission_correction_policies_audit_log_id_fkey FOREIGN KEY (audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY brand_commission_correction_policies
    ADD CONSTRAINT brand_commission_correction_policies_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brands(id);

ALTER TABLE ONLY brand_commission_payment_policies
    ADD CONSTRAINT brand_commission_payment_policies_audit_log_id_fkey FOREIGN KEY (audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY brand_commission_payment_policies
    ADD CONSTRAINT brand_commission_payment_policies_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brands(id);

ALTER TABLE ONLY brand_commission_policies
    ADD CONSTRAINT brand_commission_policies_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brands(id);

ALTER TABLE ONLY brand_compliance_policies
    ADD CONSTRAINT brand_compliance_policies_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brands(id);

ALTER TABLE ONLY brand_creation_records
    ADD CONSTRAINT brand_creation_records_audit_log_id_fkey FOREIGN KEY (audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY brand_creation_records
    ADD CONSTRAINT brand_creation_records_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brands(id);

ALTER TABLE ONLY brand_creation_records
    ADD CONSTRAINT brand_creation_records_created_by_fkey FOREIGN KEY (created_by) REFERENCES admin_accounts(id);

ALTER TABLE ONLY brand_domain_revisions
    ADD CONSTRAINT brand_domain_revisions_audit_log_id_fkey FOREIGN KEY (audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY brand_domain_revisions
    ADD CONSTRAINT brand_domain_revisions_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brands(id);

ALTER TABLE ONLY brand_domain_revisions
    ADD CONSTRAINT brand_domain_revisions_changed_by_fkey FOREIGN KEY (changed_by) REFERENCES admin_accounts(id);

ALTER TABLE ONLY brand_domains
    ADD CONSTRAINT brand_domains_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brands(id);

ALTER TABLE ONLY brand_members
    ADD CONSTRAINT brand_members_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brands(id);

ALTER TABLE ONLY brand_members
    ADD CONSTRAINT brand_members_created_by_fkey FOREIGN KEY (created_by) REFERENCES admin_accounts(id);

ALTER TABLE ONLY brand_members
    ADD CONSTRAINT brand_members_global_user_id_fkey FOREIGN KEY (global_user_id) REFERENCES global_users(id);

ALTER TABLE ONLY brand_operation_revisions
    ADD CONSTRAINT brand_operation_revisions_audit_log_id_fkey FOREIGN KEY (audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY brand_operation_revisions
    ADD CONSTRAINT brand_operation_revisions_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brands(id);

ALTER TABLE ONLY brand_operation_revisions
    ADD CONSTRAINT brand_operation_revisions_changed_by_fkey FOREIGN KEY (changed_by) REFERENCES admin_accounts(id);

ALTER TABLE ONLY brand_point_policies
    ADD CONSTRAINT brand_point_policies_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brands(id);

ALTER TABLE ONLY brand_presentation_revisions
    ADD CONSTRAINT brand_presentation_revisions_audit_log_id_fkey FOREIGN KEY (audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY brand_presentation_revisions
    ADD CONSTRAINT brand_presentation_revisions_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brands(id);

ALTER TABLE ONLY brand_presentation_revisions
    ADD CONSTRAINT brand_presentation_revisions_changed_by_fkey FOREIGN KEY (changed_by) REFERENCES admin_accounts(id);

ALTER TABLE ONLY brand_presentations
    ADD CONSTRAINT brand_presentations_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brands(id);

ALTER TABLE ONLY brand_report_archive_policies
    ADD CONSTRAINT brand_report_archive_policies_audit_log_id_fkey FOREIGN KEY (audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY brand_report_archive_policies
    ADD CONSTRAINT brand_report_archive_policies_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brands(id);

ALTER TABLE ONLY brand_settlement_policies
    ADD CONSTRAINT brand_settlement_policies_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brands(id);

ALTER TABLE ONLY brand_withdrawal_policies
    ADD CONSTRAINT brand_withdrawal_policies_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brands(id);

ALTER TABLE ONLY commission_adjustment_heads
    ADD CONSTRAINT commission_adjustment_heads_brand_id_target_id_fkey FOREIGN KEY (brand_id, target_id) REFERENCES commission_payment_targets(brand_id, id);

ALTER TABLE ONLY commission_adjustment_heads
    ADD CONSTRAINT commission_adjustment_heads_brand_id_target_id_last_adjust_fkey FOREIGN KEY (brand_id, target_id, last_adjustment_id) REFERENCES commission_adjustments(brand_id, target_id, id);

ALTER TABLE ONLY commission_adjustments
    ADD CONSTRAINT commission_adjustments_audit_log_id_fkey FOREIGN KEY (audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY commission_adjustments
    ADD CONSTRAINT commission_adjustments_brand_id_payment_id_fkey FOREIGN KEY (brand_id, payment_id) REFERENCES commission_payments(brand_id, id);

ALTER TABLE ONLY commission_adjustments
    ADD CONSTRAINT commission_adjustments_brand_id_target_id_fkey FOREIGN KEY (brand_id, target_id) REFERENCES commission_adjustment_heads(brand_id, target_id);

ALTER TABLE ONLY commission_adjustments
    ADD CONSTRAINT commission_adjustments_created_by_fkey FOREIGN KEY (created_by) REFERENCES admin_accounts(id);

ALTER TABLE ONLY commission_adjustments
    ADD CONSTRAINT commission_adjustments_ledger_entry_id_fkey FOREIGN KEY (ledger_entry_id) REFERENCES point_ledger_entries(id);

ALTER TABLE ONLY commission_allocations
    ADD CONSTRAINT commission_allocations_brand_id_agent_id_fkey FOREIGN KEY (brand_id, agent_id) REFERENCES agent_nodes(brand_id, id);

ALTER TABLE ONLY commission_allocations
    ADD CONSTRAINT commission_allocations_brand_id_cycle_id_run_id_calculatio_fkey FOREIGN KEY (brand_id, cycle_id, run_id, calculation_id) REFERENCES commission_calculations(brand_id, cycle_id, run_id, id);

ALTER TABLE ONLY commission_allocations
    ADD CONSTRAINT commission_allocations_brand_id_member_id_fkey FOREIGN KEY (brand_id, member_id) REFERENCES brand_members(brand_id, id);

ALTER TABLE ONLY commission_calculations
    ADD CONSTRAINT commission_calculations_audit_log_id_fkey FOREIGN KEY (audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY commission_calculations
    ADD CONSTRAINT commission_calculations_brand_id_account_id_member_id_fkey FOREIGN KEY (brand_id, account_id, member_id) REFERENCES point_accounts(brand_id, id, brand_member_id);

ALTER TABLE ONLY commission_calculations
    ADD CONSTRAINT commission_calculations_brand_id_cycle_id_order_id_fkey FOREIGN KEY (brand_id, cycle_id, order_id) REFERENCES commission_cycle_targets(brand_id, cycle_id, order_id);

ALTER TABLE ONLY commission_calculations
    ADD CONSTRAINT commission_calculations_brand_id_cycle_id_run_id_fkey FOREIGN KEY (brand_id, cycle_id, run_id) REFERENCES commission_runs(brand_id, cycle_id, id);

ALTER TABLE ONLY commission_correction_balance_heads
    ADD CONSTRAINT commission_correction_balance__brand_id_original_target_id_fkey FOREIGN KEY (brand_id, original_target_id) REFERENCES commission_payment_targets(brand_id, id);

ALTER TABLE ONLY commission_correction_balance_heads
    ADD CONSTRAINT commission_correction_balance_brand_id_last_execution_targ_fkey FOREIGN KEY (brand_id, last_execution_target_id) REFERENCES commission_correction_execution_targets(brand_id, id);

ALTER TABLE ONLY commission_correction_balance_heads
    ADD CONSTRAINT commission_correction_balance_heads_brand_id_cycle_id_fkey FOREIGN KEY (brand_id, cycle_id) REFERENCES commission_cycles(brand_id, id);

ALTER TABLE ONLY commission_correction_balance_heads
    ADD CONSTRAINT commission_correction_balance_heads_brand_id_member_id_fkey FOREIGN KEY (brand_id, member_id) REFERENCES brand_members(brand_id, id);

ALTER TABLE ONLY commission_correction_cycle_holds
    ADD CONSTRAINT commission_correction_cycle_holds_audit_log_id_fkey FOREIGN KEY (audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY commission_correction_cycle_holds
    ADD CONSTRAINT commission_correction_cycle_holds_blocked_execution_id_fkey FOREIGN KEY (blocked_execution_id) REFERENCES commission_correction_executions(id);

ALTER TABLE ONLY commission_correction_cycle_holds
    ADD CONSTRAINT commission_correction_cycle_holds_brand_id_cycle_id_fkey FOREIGN KEY (brand_id, cycle_id) REFERENCES commission_cycles(brand_id, id);

ALTER TABLE ONLY commission_correction_execution_targets
    ADD CONSTRAINT commission_correction_executi_brand_id_cycle_id_execution__fkey FOREIGN KEY (brand_id, cycle_id, execution_id) REFERENCES commission_correction_executions(brand_id, cycle_id, id);

ALTER TABLE ONLY commission_correction_execution_steps
    ADD CONSTRAINT commission_correction_execution_step_brand_id_execution_id_fkey FOREIGN KEY (brand_id, execution_id) REFERENCES commission_correction_executions(brand_id, id);

ALTER TABLE ONLY commission_correction_execution_steps
    ADD CONSTRAINT commission_correction_execution_step_paused_plan_target_id_fkey FOREIGN KEY (paused_plan_target_id) REFERENCES commission_correction_plan_targets(id);

ALTER TABLE ONLY commission_correction_execution_steps
    ADD CONSTRAINT commission_correction_execution_steps_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES admin_accounts(id);

ALTER TABLE ONLY commission_correction_execution_steps
    ADD CONSTRAINT commission_correction_execution_steps_audit_log_id_fkey FOREIGN KEY (audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY commission_correction_execution_steps
    ADD CONSTRAINT commission_correction_execution_steps_target_id_fkey FOREIGN KEY (target_id) REFERENCES commission_correction_execution_targets(id) DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE ONLY commission_correction_execution_targets
    ADD CONSTRAINT commission_correction_execution_targets_audit_log_id_fkey FOREIGN KEY (audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY commission_correction_execution_targets
    ADD CONSTRAINT commission_correction_execution_targets_brand_id_member_id_fkey FOREIGN KEY (brand_id, member_id) REFERENCES brand_members(brand_id, id);

ALTER TABLE ONLY commission_correction_execution_targets
    ADD CONSTRAINT commission_correction_execution_targets_ledger_entry_id_fkey FOREIGN KEY (ledger_entry_id) REFERENCES point_ledger_entries(id);

ALTER TABLE ONLY commission_correction_executions
    ADD CONSTRAINT commission_correction_executions_approval_audit_log_id_fkey FOREIGN KEY (approval_audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY commission_correction_executions
    ADD CONSTRAINT commission_correction_executions_approved_by_fkey FOREIGN KEY (approved_by) REFERENCES admin_accounts(id);

ALTER TABLE ONLY commission_correction_executions
    ADD CONSTRAINT commission_correction_executions_brand_id_cycle_id_plan_id_fkey FOREIGN KEY (brand_id, cycle_id, plan_id) REFERENCES commission_correction_plans(brand_id, cycle_id, id);

ALTER TABLE ONLY commission_correction_executions
    ADD CONSTRAINT commission_correction_executions_creation_audit_log_id_fkey FOREIGN KEY (creation_audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY commission_correction_executions
    ADD CONSTRAINT commission_correction_executions_last_audit_log_id_fkey FOREIGN KEY (last_audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY commission_correction_executions
    ADD CONSTRAINT commission_correction_executions_paused_plan_target_id_fkey FOREIGN KEY (paused_plan_target_id) REFERENCES commission_correction_plan_targets(id);

ALTER TABLE ONLY commission_correction_plan_steps
    ADD CONSTRAINT commission_correction_plan_steps_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES admin_accounts(id);

ALTER TABLE ONLY commission_correction_plan_steps
    ADD CONSTRAINT commission_correction_plan_steps_audit_log_id_fkey FOREIGN KEY (audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY commission_correction_plan_steps
    ADD CONSTRAINT commission_correction_plan_steps_brand_id_plan_id_fkey FOREIGN KEY (brand_id, plan_id) REFERENCES commission_correction_plans(brand_id, id);

ALTER TABLE ONLY commission_correction_plan_targets
    ADD CONSTRAINT commission_correction_plan_ta_brand_id_previous_correction_fkey FOREIGN KEY (brand_id, previous_correction_target_id) REFERENCES commission_correction_execution_targets(brand_id, id);

ALTER TABLE ONLY commission_correction_plan_targets
    ADD CONSTRAINT commission_correction_plan_tar_brand_id_original_target_id_fkey FOREIGN KEY (brand_id, original_target_id) REFERENCES commission_payment_targets(brand_id, id);

ALTER TABLE ONLY commission_correction_plan_targets
    ADD CONSTRAINT commission_correction_plan_targets_brand_id_member_id_fkey FOREIGN KEY (brand_id, member_id) REFERENCES brand_members(brand_id, id);

ALTER TABLE ONLY commission_correction_plan_targets
    ADD CONSTRAINT commission_correction_plan_targets_brand_id_plan_id_fkey FOREIGN KEY (brand_id, plan_id) REFERENCES commission_correction_plans(brand_id, id);

ALTER TABLE ONLY commission_correction_plan_targets
    ADD CONSTRAINT commission_correction_plan_targets_creation_audit_log_id_fkey FOREIGN KEY (creation_audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY commission_correction_plan_targets
    ADD CONSTRAINT commission_correction_plan_targets_earning_id_fkey FOREIGN KEY (earning_id) REFERENCES commission_earnings(id);

ALTER TABLE ONLY commission_correction_plans
    ADD CONSTRAINT commission_correction_plans_brand_id_cycle_id_run_id_fkey FOREIGN KEY (brand_id, cycle_id, run_id) REFERENCES commission_runs(brand_id, cycle_id, id);

ALTER TABLE ONLY commission_correction_plans
    ADD CONSTRAINT commission_correction_plans_brand_id_payment_id_fkey FOREIGN KEY (brand_id, payment_id) REFERENCES commission_payments(brand_id, id);

ALTER TABLE ONLY commission_correction_plans
    ADD CONSTRAINT commission_correction_plans_creation_audit_log_id_fkey FOREIGN KEY (creation_audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY commission_correction_plans
    ADD CONSTRAINT commission_correction_plans_last_audit_log_id_fkey FOREIGN KEY (last_audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY commission_correction_policy_revisions
    ADD CONSTRAINT commission_correction_policy_revisions_audit_log_id_fkey FOREIGN KEY (audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY commission_correction_policy_revisions
    ADD CONSTRAINT commission_correction_policy_revisions_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brand_commission_correction_policies(brand_id);

ALTER TABLE ONLY commission_cycle_steps
    ADD CONSTRAINT commission_cycle_steps_audit_log_id_fkey FOREIGN KEY (audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY commission_cycle_steps
    ADD CONSTRAINT commission_cycle_steps_brand_id_cycle_id_fkey FOREIGN KEY (brand_id, cycle_id) REFERENCES commission_cycles(brand_id, id);

ALTER TABLE ONLY commission_cycle_steps
    ADD CONSTRAINT commission_cycle_steps_brand_id_cycle_id_run_id_fkey FOREIGN KEY (brand_id, cycle_id, run_id) REFERENCES commission_runs(brand_id, cycle_id, id);

ALTER TABLE ONLY commission_cycle_targets
    ADD CONSTRAINT commission_cycle_targets_brand_id_cycle_id_fkey FOREIGN KEY (brand_id, cycle_id) REFERENCES commission_cycles(brand_id, id);

ALTER TABLE ONLY commission_cycle_targets
    ADD CONSTRAINT commission_cycle_targets_brand_id_order_id_fkey FOREIGN KEY (brand_id, order_id) REFERENCES bet_orders(brand_id, id);

ALTER TABLE ONLY commission_cycles
    ADD CONSTRAINT commission_cycles_brand_id_anchor_order_id_fkey FOREIGN KEY (brand_id, anchor_order_id) REFERENCES bet_orders(brand_id, id);

ALTER TABLE ONLY commission_cycles
    ADD CONSTRAINT commission_cycles_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brands(id);

ALTER TABLE ONLY commission_cycles
    ADD CONSTRAINT commission_cycles_brand_id_id_current_run_id_fkey FOREIGN KEY (brand_id, id, current_run_id) REFERENCES commission_runs(brand_id, cycle_id, id);

ALTER TABLE ONLY commission_cycles
    ADD CONSTRAINT commission_cycles_created_by_fkey FOREIGN KEY (created_by) REFERENCES admin_accounts(id);

ALTER TABLE ONLY commission_cycles
    ADD CONSTRAINT commission_cycles_creation_audit_log_id_fkey FOREIGN KEY (creation_audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY commission_discovery
    ADD CONSTRAINT commission_discovery_brand_id_cycle_id_fkey FOREIGN KEY (brand_id, cycle_id) REFERENCES commission_cycles(brand_id, id);

ALTER TABLE ONLY commission_discovery
    ADD CONSTRAINT commission_discovery_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brands(id);

ALTER TABLE ONLY commission_discovery
    ADD CONSTRAINT commission_discovery_brand_id_id_fkey FOREIGN KEY (brand_id, id) REFERENCES bet_orders(brand_id, id);

ALTER TABLE ONLY commission_discovery
    ADD CONSTRAINT commission_discovery_last_audit_log_id_fkey FOREIGN KEY (last_audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY commission_earnings
    ADD CONSTRAINT commission_earnings_brand_id_agent_id_fkey FOREIGN KEY (brand_id, agent_id) REFERENCES agent_nodes(brand_id, id);

ALTER TABLE ONLY commission_earnings
    ADD CONSTRAINT commission_earnings_brand_id_cycle_id_run_id_fkey FOREIGN KEY (brand_id, cycle_id, run_id) REFERENCES commission_runs(brand_id, cycle_id, id);

ALTER TABLE ONLY commission_earnings
    ADD CONSTRAINT commission_earnings_brand_id_member_id_fkey FOREIGN KEY (brand_id, member_id) REFERENCES brand_members(brand_id, id);

ALTER TABLE ONLY commission_payment_policy_revisions
    ADD CONSTRAINT commission_payment_policy_revisions_audit_log_id_fkey FOREIGN KEY (audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY commission_payment_policy_revisions
    ADD CONSTRAINT commission_payment_policy_revisions_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brands(id);

ALTER TABLE ONLY commission_payment_targets
    ADD CONSTRAINT commission_payment_targets_audit_log_id_fkey FOREIGN KEY (audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY commission_payment_targets
    ADD CONSTRAINT commission_payment_targets_brand_id_member_id_fkey FOREIGN KEY (brand_id, member_id) REFERENCES brand_members(brand_id, id);

ALTER TABLE ONLY commission_payment_targets
    ADD CONSTRAINT commission_payment_targets_brand_id_payment_id_fkey FOREIGN KEY (brand_id, payment_id) REFERENCES commission_payments(brand_id, id);

ALTER TABLE ONLY commission_payment_targets
    ADD CONSTRAINT commission_payment_targets_ledger_entry_id_fkey FOREIGN KEY (ledger_entry_id) REFERENCES point_ledger_entries(id);

ALTER TABLE ONLY commission_payments
    ADD CONSTRAINT commission_payments_approval_audit_log_id_fkey FOREIGN KEY (approval_audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY commission_payments
    ADD CONSTRAINT commission_payments_approved_by_fkey FOREIGN KEY (approved_by) REFERENCES admin_accounts(id);

ALTER TABLE ONLY commission_payments
    ADD CONSTRAINT commission_payments_brand_id_cycle_id_run_id_fkey FOREIGN KEY (brand_id, cycle_id, run_id) REFERENCES commission_runs(brand_id, cycle_id, id);

ALTER TABLE ONLY commission_payments
    ADD CONSTRAINT commission_payments_creation_audit_log_id_fkey FOREIGN KEY (creation_audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY commission_payments
    ADD CONSTRAINT commission_payments_last_audit_log_id_fkey FOREIGN KEY (last_audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY commission_policy_revisions
    ADD CONSTRAINT commission_policy_revisions_audit_log_id_fkey FOREIGN KEY (audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY commission_policy_revisions
    ADD CONSTRAINT commission_policy_revisions_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brands(id);

ALTER TABLE ONLY commission_policy_revisions
    ADD CONSTRAINT commission_policy_revisions_changed_by_fkey FOREIGN KEY (changed_by) REFERENCES admin_accounts(id);

ALTER TABLE ONLY commission_runs
    ADD CONSTRAINT commission_runs_brand_id_cycle_id_fkey FOREIGN KEY (brand_id, cycle_id) REFERENCES commission_cycles(brand_id, id);

ALTER TABLE ONLY compliance_decisions
    ADD CONSTRAINT compliance_decisions_audit_log_id_fkey FOREIGN KEY (audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY compliance_decisions
    ADD CONSTRAINT compliance_decisions_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brands(id);

ALTER TABLE ONLY compliance_decisions
    ADD CONSTRAINT compliance_decisions_brand_id_policy_version_fkey FOREIGN KEY (brand_id, policy_version) REFERENCES compliance_policy_revisions(brand_id, version);

ALTER TABLE ONLY compliance_decisions
    ADD CONSTRAINT compliance_decisions_created_by_fkey FOREIGN KEY (created_by) REFERENCES admin_accounts(id);

ALTER TABLE ONLY compliance_gate_rejections
    ADD CONSTRAINT compliance_gate_rejections_audit_log_id_fkey FOREIGN KEY (audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY compliance_gate_rejections
    ADD CONSTRAINT compliance_gate_rejections_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brands(id);

ALTER TABLE ONLY compliance_gate_rejections
    ADD CONSTRAINT compliance_gate_rejections_brand_id_member_id_fkey FOREIGN KEY (brand_id, member_id) REFERENCES brand_members(brand_id, id);

ALTER TABLE ONLY compliance_gate_rejections
    ADD CONSTRAINT compliance_gate_rejections_brand_id_policy_version_fkey FOREIGN KEY (brand_id, policy_version) REFERENCES compliance_policy_revisions(brand_id, version);

ALTER TABLE ONLY compliance_policy_revisions
    ADD CONSTRAINT compliance_policy_revisions_audit_log_id_fkey FOREIGN KEY (audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY compliance_policy_revisions
    ADD CONSTRAINT compliance_policy_revisions_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brands(id);

ALTER TABLE ONLY compliance_policy_revisions
    ADD CONSTRAINT compliance_policy_revisions_changed_by_fkey FOREIGN KEY (changed_by) REFERENCES admin_accounts(id);

ALTER TABLE ONLY draw_attempt_batches
    ADD CONSTRAINT draw_attempt_batches_brand_id_game_id_period_id_fkey FOREIGN KEY (brand_id, game_id, period_id) REFERENCES periods(brand_id, game_id, id);

ALTER TABLE ONLY draw_attempt_batches
    ADD CONSTRAINT draw_attempt_batches_brand_id_game_id_source_set_id_fkey FOREIGN KEY (brand_id, game_id, source_set_id) REFERENCES draw_source_sets(brand_id, game_id, id);

ALTER TABLE ONLY draw_correction_failures
    ADD CONSTRAINT draw_correction_failures_brand_id_correction_id_fkey FOREIGN KEY (brand_id, correction_id) REFERENCES draw_corrections(brand_id, id);

ALTER TABLE ONLY draw_correction_failures
    ADD CONSTRAINT draw_correction_failures_correction_id_order_id_fkey FOREIGN KEY (correction_id, order_id) REFERENCES draw_correction_targets(correction_id, order_id);

ALTER TABLE ONLY draw_correction_targets
    ADD CONSTRAINT draw_correction_targets_brand_id_old_payout_entry_id_fkey FOREIGN KEY (brand_id, old_payout_entry_id) REFERENCES point_ledger_entries(brand_id, id);

ALTER TABLE ONLY draw_correction_targets
    ADD CONSTRAINT draw_correction_targets_brand_id_order_id_old_calculation__fkey FOREIGN KEY (brand_id, order_id, old_calculation_id) REFERENCES settlement_calculations(brand_id, order_id, id);

ALTER TABLE ONLY draw_correction_targets
    ADD CONSTRAINT draw_correction_targets_brand_id_period_id_correction_id_fkey FOREIGN KEY (brand_id, period_id, correction_id) REFERENCES draw_corrections(brand_id, period_id, id);

ALTER TABLE ONLY draw_correction_targets
    ADD CONSTRAINT draw_correction_targets_brand_id_period_id_order_id_fkey FOREIGN KEY (brand_id, period_id, order_id) REFERENCES bet_orders(brand_id, period_id, id);

ALTER TABLE ONLY draw_correction_targets
    ADD CONSTRAINT draw_correction_targets_brand_id_reversal_entry_id_fkey FOREIGN KEY (brand_id, reversal_entry_id) REFERENCES point_ledger_entries(brand_id, id);

ALTER TABLE ONLY draw_corrections
    ADD CONSTRAINT draw_corrections_brand_id_game_id_period_id_draw_result_id_fkey FOREIGN KEY (brand_id, game_id, period_id, draw_result_id) REFERENCES draw_results(brand_id, game_id, period_id, id);

ALTER TABLE ONLY draw_corrections
    ADD CONSTRAINT draw_corrections_brand_id_game_id_period_id_previous_draw__fkey FOREIGN KEY (brand_id, game_id, period_id, previous_draw_result_id) REFERENCES draw_results(brand_id, game_id, period_id, id);

ALTER TABLE ONLY draw_corrections
    ADD CONSTRAINT draw_corrections_brand_id_period_id_new_job_id_fkey FOREIGN KEY (brand_id, period_id, new_job_id) REFERENCES settlement_jobs(brand_id, period_id, id);

ALTER TABLE ONLY draw_corrections
    ADD CONSTRAINT draw_corrections_brand_id_period_id_previous_job_id_fkey FOREIGN KEY (brand_id, period_id, previous_job_id) REFERENCES settlement_jobs(brand_id, period_id, id);

ALTER TABLE ONLY draw_corrections
    ADD CONSTRAINT draw_corrections_brand_id_policy_version_fkey FOREIGN KEY (brand_id, policy_version) REFERENCES settlement_policy_history(brand_id, version);

ALTER TABLE ONLY draw_corrections
    ADD CONSTRAINT draw_corrections_created_by_fkey FOREIGN KEY (created_by) REFERENCES admin_accounts(id);

ALTER TABLE ONLY draw_notification_publications
    ADD CONSTRAINT draw_notification_publications_audit_log_id_fkey FOREIGN KEY (audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY draw_notification_publications
    ADD CONSTRAINT draw_notification_publications_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brands(id);

ALTER TABLE ONLY draw_notification_publications
    ADD CONSTRAINT draw_notification_publications_draw_result_id_fkey FOREIGN KEY (draw_result_id) REFERENCES draw_results(id);

ALTER TABLE ONLY draw_notification_publications
    ADD CONSTRAINT draw_notification_publications_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id);

ALTER TABLE ONLY draw_notification_publications
    ADD CONSTRAINT draw_notification_publications_period_id_fkey FOREIGN KEY (period_id) REFERENCES periods(id);

ALTER TABLE ONLY draw_notification_publications
    ADD CONSTRAINT draw_notification_publications_previous_draw_id_fkey FOREIGN KEY (previous_draw_id) REFERENCES draw_results(id);

ALTER TABLE ONLY draw_notification_recipients
    ADD CONSTRAINT draw_notification_recipients_brand_id_draw_result_id_fkey FOREIGN KEY (brand_id, draw_result_id) REFERENCES draw_notification_publications(brand_id, draw_result_id);

ALTER TABLE ONLY draw_notification_recipients
    ADD CONSTRAINT draw_notification_recipients_brand_id_member_id_fkey FOREIGN KEY (brand_id, member_id) REFERENCES brand_members(brand_id, id);

ALTER TABLE ONLY draw_results
    ADD CONSTRAINT draw_results_brand_id_game_id_period_id_corrected_from_id_fkey FOREIGN KEY (brand_id, game_id, period_id, corrected_from_id) REFERENCES draw_results(brand_id, game_id, period_id, id);

ALTER TABLE ONLY draw_results
    ADD CONSTRAINT draw_results_brand_id_game_id_period_id_fkey FOREIGN KEY (brand_id, game_id, period_id) REFERENCES periods(brand_id, game_id, id);

ALTER TABLE ONLY draw_results
    ADD CONSTRAINT draw_results_brand_id_game_id_source_id_kind_fkey FOREIGN KEY (brand_id, game_id, source_id, kind) REFERENCES draw_sources(brand_id, game_id, id, type);

ALTER TABLE ONLY draw_results
    ADD CONSTRAINT draw_results_created_by_fkey FOREIGN KEY (created_by) REFERENCES admin_accounts(id);

ALTER TABLE ONLY draw_source_sets
    ADD CONSTRAINT draw_source_sets_brand_id_game_id_fkey FOREIGN KEY (brand_id, game_id) REFERENCES games(brand_id, id);

ALTER TABLE ONLY draw_source_sets
    ADD CONSTRAINT draw_source_sets_created_by_fkey FOREIGN KEY (created_by) REFERENCES admin_accounts(id);

ALTER TABLE ONLY draw_sources
    ADD CONSTRAINT draw_sources_brand_id_game_id_fkey FOREIGN KEY (brand_id, game_id) REFERENCES games(brand_id, id);

ALTER TABLE ONLY game_bet_policies
    ADD CONSTRAINT game_bet_policies_brand_id_game_id_fkey FOREIGN KEY (brand_id, game_id) REFERENCES games(brand_id, id);

ALTER TABLE ONLY game_schedules
    ADD CONSTRAINT game_schedules_brand_id_game_id_fkey FOREIGN KEY (brand_id, game_id) REFERENCES games(brand_id, id);

ALTER TABLE ONLY game_schedules
    ADD CONSTRAINT game_schedules_created_by_fkey FOREIGN KEY (created_by) REFERENCES admin_accounts(id);

ALTER TABLE ONLY game_withdrawal_policies
    ADD CONSTRAINT game_withdrawal_policies_brand_id_game_id_fkey FOREIGN KEY (brand_id, game_id) REFERENCES games(brand_id, id);

ALTER TABLE ONLY games
    ADD CONSTRAINT games_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brands(id);

ALTER TABLE ONLY games
    ADD CONSTRAINT games_brand_id_id_draw_source_set_id_fkey FOREIGN KEY (brand_id, id, draw_source_set_id) REFERENCES draw_source_sets(brand_id, game_id, id);

ALTER TABLE ONLY games
    ADD CONSTRAINT games_brand_id_id_schedule_id_fkey FOREIGN KEY (brand_id, id, schedule_id) REFERENCES game_schedules(brand_id, game_id, id);

ALTER TABLE ONLY games
    ADD CONSTRAINT games_created_by_fkey FOREIGN KEY (created_by) REFERENCES admin_accounts(id);

ALTER TABLE ONLY idempotency_requests
    ADD CONSTRAINT idempotency_requests_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brands(id);

ALTER TABLE ONLY join_code_revisions
    ADD CONSTRAINT join_code_revisions_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES admin_accounts(id);

ALTER TABLE ONLY join_code_revisions
    ADD CONSTRAINT join_code_revisions_audit_log_id_fkey FOREIGN KEY (audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY join_code_revisions
    ADD CONSTRAINT join_code_revisions_brand_id_code_id_fkey FOREIGN KEY (brand_id, code_id) REFERENCES join_codes(brand_id, id);

ALTER TABLE ONLY join_codes
    ADD CONSTRAINT join_codes_brand_id_agent_id_fkey FOREIGN KEY (brand_id, agent_id) REFERENCES agent_nodes(brand_id, id);

ALTER TABLE ONLY join_codes
    ADD CONSTRAINT join_codes_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brands(id);

ALTER TABLE ONLY join_codes
    ADD CONSTRAINT join_codes_brand_id_owner_member_id_fkey FOREIGN KEY (brand_id, owner_member_id) REFERENCES brand_members(brand_id, id);

ALTER TABLE ONLY join_codes
    ADD CONSTRAINT join_codes_created_by_fkey FOREIGN KEY (created_by) REFERENCES admin_accounts(id);

ALTER TABLE ONLY point_ledger_entries
    ADD CONSTRAINT ledger_account_member_fk FOREIGN KEY (brand_id, account_id, member_id) REFERENCES point_accounts(brand_id, id, brand_member_id);

ALTER TABLE ONLY point_ledger_entries
    ADD CONSTRAINT ledger_reversal_same_brand FOREIGN KEY (brand_id, account_id, reversal_of) REFERENCES point_ledger_entries(brand_id, account_id, id);

ALTER TABLE ONLY notification_deliveries
    ADD CONSTRAINT notification_deliveries_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brands(id);

ALTER TABLE ONLY notification_deliveries
    ADD CONSTRAINT notification_deliveries_event_id_fkey FOREIGN KEY (event_id) REFERENCES outbox_events(id);

ALTER TABLE ONLY notifications
    ADD CONSTRAINT notification_template_revision_fk FOREIGN KEY (brand_id, template_key, template_version) REFERENCES notification_template_revisions(brand_id, template_key, version);

ALTER TABLE ONLY notification_template_revisions
    ADD CONSTRAINT notification_template_revisions_audit_log_id_fkey FOREIGN KEY (audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY notification_template_revisions
    ADD CONSTRAINT notification_template_revisions_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brands(id);

ALTER TABLE ONLY notification_template_revisions
    ADD CONSTRAINT notification_template_revisions_changed_by_fkey FOREIGN KEY (changed_by) REFERENCES admin_accounts(id);

ALTER TABLE ONLY notification_templates
    ADD CONSTRAINT notification_templates_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brands(id);

ALTER TABLE ONLY notifications
    ADD CONSTRAINT notifications_brand_id_event_id_fkey FOREIGN KEY (brand_id, event_id) REFERENCES notification_deliveries(brand_id, event_id);

ALTER TABLE ONLY notifications
    ADD CONSTRAINT notifications_brand_id_member_id_fkey FOREIGN KEY (brand_id, member_id) REFERENCES brand_members(brand_id, id);

ALTER TABLE ONLY outbox_events
    ADD CONSTRAINT outbox_events_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brands(id);

ALTER TABLE ONLY period_cancellation_failures
    ADD CONSTRAINT period_cancellation_failures_cancellation_id_fkey FOREIGN KEY (cancellation_id) REFERENCES period_cancellations(id);

ALTER TABLE ONLY period_cancellation_failures
    ADD CONSTRAINT period_cancellation_failures_cancellation_id_order_id_fkey FOREIGN KEY (cancellation_id, order_id) REFERENCES period_cancellation_targets(cancellation_id, order_id);

ALTER TABLE ONLY period_cancellation_targets
    ADD CONSTRAINT period_cancellation_targets_brand_id_period_id_cancellatio_fkey FOREIGN KEY (brand_id, period_id, cancellation_id) REFERENCES period_cancellations(brand_id, period_id, id);

ALTER TABLE ONLY period_cancellation_targets
    ADD CONSTRAINT period_cancellation_targets_brand_id_period_id_order_id_fkey FOREIGN KEY (brand_id, period_id, order_id) REFERENCES bet_orders(brand_id, period_id, id);

ALTER TABLE ONLY period_cancellation_targets
    ADD CONSTRAINT period_cancellation_targets_refund_entry_id_fkey FOREIGN KEY (refund_entry_id) REFERENCES point_ledger_entries(id);

ALTER TABLE ONLY period_cancellations
    ADD CONSTRAINT period_cancellations_brand_id_game_id_period_id_draw_resul_fkey FOREIGN KEY (brand_id, game_id, period_id, draw_result_id) REFERENCES draw_results(brand_id, game_id, period_id, id);

ALTER TABLE ONLY period_cancellations
    ADD CONSTRAINT period_cancellations_brand_id_game_id_period_id_fkey FOREIGN KEY (brand_id, game_id, period_id) REFERENCES periods(brand_id, game_id, id);

ALTER TABLE ONLY period_cancellations
    ADD CONSTRAINT period_cancellations_created_by_fkey FOREIGN KEY (created_by) REFERENCES admin_accounts(id);

ALTER TABLE ONLY period_rule_versions
    ADD CONSTRAINT period_rule_versions_brand_id_game_id_period_id_fkey FOREIGN KEY (brand_id, game_id, period_id) REFERENCES periods(brand_id, game_id, id);

ALTER TABLE ONLY period_rule_versions
    ADD CONSTRAINT period_rule_versions_brand_id_game_id_play_id_rule_version_fkey FOREIGN KEY (brand_id, game_id, play_id, rule_version_id) REFERENCES rule_versions(brand_id, game_id, play_id, id);

ALTER TABLE ONLY periods
    ADD CONSTRAINT periods_brand_id_game_id_fkey FOREIGN KEY (brand_id, game_id) REFERENCES games(brand_id, id);

ALTER TABLE ONLY periods
    ADD CONSTRAINT periods_brand_id_game_id_id_draw_result_id_fkey FOREIGN KEY (brand_id, game_id, id, draw_result_id) REFERENCES draw_results(brand_id, game_id, period_id, id);

ALTER TABLE ONLY periods
    ADD CONSTRAINT periods_brand_id_game_id_schedule_id_fkey FOREIGN KEY (brand_id, game_id, schedule_id) REFERENCES game_schedules(brand_id, game_id, id);

ALTER TABLE ONLY periods
    ADD CONSTRAINT periods_brand_id_id_current_correction_id_fkey FOREIGN KEY (brand_id, id, current_correction_id) REFERENCES draw_corrections(brand_id, period_id, id);

ALTER TABLE ONLY periods
    ADD CONSTRAINT periods_brand_id_id_current_settlement_job_id_fkey FOREIGN KEY (brand_id, id, current_settlement_job_id) REFERENCES settlement_jobs(brand_id, period_id, id);

ALTER TABLE ONLY play_definitions
    ADD CONSTRAINT play_definitions_brand_id_game_id_fkey FOREIGN KEY (brand_id, game_id) REFERENCES games(brand_id, id);

ALTER TABLE ONLY play_definitions
    ADD CONSTRAINT play_definitions_brand_id_game_id_id_active_version_id_fkey FOREIGN KEY (brand_id, game_id, id, active_version_id) REFERENCES rule_versions(brand_id, game_id, play_id, id);

ALTER TABLE ONLY play_definitions
    ADD CONSTRAINT play_definitions_created_by_fkey FOREIGN KEY (created_by) REFERENCES admin_accounts(id);

ALTER TABLE ONLY point_accounts
    ADD CONSTRAINT point_accounts_brand_id_brand_member_id_fkey FOREIGN KEY (brand_id, brand_member_id) REFERENCES brand_members(brand_id, id);

ALTER TABLE ONLY point_balance_repairs
    ADD CONSTRAINT point_balance_repairs_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES admin_accounts(id);

ALTER TABLE ONLY point_balance_repairs
    ADD CONSTRAINT point_balance_repairs_brand_id_account_id_member_id_fkey FOREIGN KEY (brand_id, account_id, member_id) REFERENCES point_accounts(brand_id, id, brand_member_id);

ALTER TABLE ONLY point_buckets
    ADD CONSTRAINT point_buckets_brand_id_account_id_fkey FOREIGN KEY (brand_id, account_id) REFERENCES point_accounts(brand_id, id);

ALTER TABLE ONLY point_ledger_entries
    ADD CONSTRAINT point_ledger_entries_brand_id_account_id_fkey FOREIGN KEY (brand_id, account_id) REFERENCES point_accounts(brand_id, id);

ALTER TABLE ONLY point_ledger_entries
    ADD CONSTRAINT point_ledger_entries_reversal_of_fkey FOREIGN KEY (reversal_of) REFERENCES point_ledger_entries(id);

ALTER TABLE ONLY point_reconciliation_failures
    ADD CONSTRAINT point_reconciliation_failures_audit_log_id_fkey FOREIGN KEY (audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY point_reconciliation_failures
    ADD CONSTRAINT point_reconciliation_failures_brand_id_job_id_target_id_fkey FOREIGN KEY (brand_id, job_id, target_id) REFERENCES point_reconciliation_targets(brand_id, job_id, id);

ALTER TABLE ONLY point_reconciliation_jobs
    ADD CONSTRAINT point_reconciliation_jobs_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brands(id);

ALTER TABLE ONLY point_reconciliation_jobs
    ADD CONSTRAINT point_reconciliation_jobs_brand_id_id_last_failure_id_fkey FOREIGN KEY (brand_id, id, last_failure_id) REFERENCES point_reconciliation_failures(brand_id, job_id, id);

ALTER TABLE ONLY point_reconciliation_jobs
    ADD CONSTRAINT point_reconciliation_jobs_created_by_fkey FOREIGN KEY (created_by) REFERENCES admin_accounts(id);

ALTER TABLE ONLY point_reconciliation_jobs
    ADD CONSTRAINT point_reconciliation_jobs_creation_audit_log_id_fkey FOREIGN KEY (creation_audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY point_reconciliation_results
    ADD CONSTRAINT point_reconciliation_results_audit_log_id_fkey FOREIGN KEY (audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY point_reconciliation_results
    ADD CONSTRAINT point_reconciliation_results_brand_id_job_id_target_id_fkey FOREIGN KEY (brand_id, job_id, target_id) REFERENCES point_reconciliation_targets(brand_id, job_id, id);

ALTER TABLE ONLY point_reconciliation_retries
    ADD CONSTRAINT point_reconciliation_retries_audit_log_id_fkey FOREIGN KEY (audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY point_reconciliation_retries
    ADD CONSTRAINT point_reconciliation_retries_brand_id_job_id_fkey FOREIGN KEY (brand_id, job_id) REFERENCES point_reconciliation_jobs(brand_id, id);

ALTER TABLE ONLY point_reconciliation_retries
    ADD CONSTRAINT point_reconciliation_retries_brand_id_job_id_previous_fail_fkey FOREIGN KEY (brand_id, job_id, previous_failure_id) REFERENCES point_reconciliation_failures(brand_id, job_id, id);

ALTER TABLE ONLY point_reconciliation_retries
    ADD CONSTRAINT point_reconciliation_retries_created_by_fkey FOREIGN KEY (created_by) REFERENCES admin_accounts(id);

ALTER TABLE ONLY point_reconciliation_targets
    ADD CONSTRAINT point_reconciliation_targets_brand_id_account_id_member_id_fkey FOREIGN KEY (brand_id, account_id, member_id) REFERENCES point_accounts(brand_id, id, brand_member_id);

ALTER TABLE ONLY point_reconciliation_targets
    ADD CONSTRAINT point_reconciliation_targets_brand_id_job_id_fkey FOREIGN KEY (brand_id, job_id) REFERENCES point_reconciliation_jobs(brand_id, id);

ALTER TABLE ONLY recharge_orders
    ADD CONSTRAINT recharge_orders_brand_id_account_id_ledger_entry_id_fkey FOREIGN KEY (brand_id, account_id, ledger_entry_id) REFERENCES point_ledger_entries(brand_id, account_id, id);

ALTER TABLE ONLY recharge_orders
    ADD CONSTRAINT recharge_orders_brand_id_account_id_member_id_fkey FOREIGN KEY (brand_id, account_id, member_id) REFERENCES point_accounts(brand_id, id, brand_member_id);

ALTER TABLE ONLY recharge_orders
    ADD CONSTRAINT recharge_orders_confirmed_by_fkey FOREIGN KEY (confirmed_by) REFERENCES admin_accounts(id);

ALTER TABLE ONLY recharge_orders
    ADD CONSTRAINT recharge_orders_created_by_fkey FOREIGN KEY (created_by) REFERENCES admin_accounts(id);

ALTER TABLE ONLY report_archive_automatic_cursors
    ADD CONSTRAINT report_archive_automatic_cursors_brand_id_policy_version_fkey FOREIGN KEY (brand_id, policy_version) REFERENCES report_archive_policy_revisions(brand_id, version);

ALTER TABLE ONLY report_archives
    ADD CONSTRAINT report_archive_automatic_task_fk FOREIGN KEY (brand_id, automatic_task_id) REFERENCES report_archive_automatic_tasks(brand_id, id);

ALTER TABLE ONLY report_archive_automatic_tasks
    ADD CONSTRAINT report_archive_automatic_tasks_brand_id_archive_id_fkey FOREIGN KEY (brand_id, archive_id) REFERENCES report_archives(brand_id, id);

ALTER TABLE ONLY report_archive_automatic_tasks
    ADD CONSTRAINT report_archive_automatic_tasks_brand_id_policy_version_fkey FOREIGN KEY (brand_id, policy_version) REFERENCES report_archive_policy_revisions(brand_id, version);

ALTER TABLE ONLY report_archive_automatic_tasks
    ADD CONSTRAINT report_archive_automatic_tasks_creation_audit_log_id_fkey FOREIGN KEY (creation_audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY report_archive_automatic_tasks
    ADD CONSTRAINT report_archive_automatic_tasks_last_audit_log_id_fkey FOREIGN KEY (last_audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY report_archive_policy_revisions
    ADD CONSTRAINT report_archive_policy_revisions_audit_log_id_fkey FOREIGN KEY (audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY report_archive_policy_revisions
    ADD CONSTRAINT report_archive_policy_revisions_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brand_report_archive_policies(brand_id);

ALTER TABLE ONLY report_archives
    ADD CONSTRAINT report_archives_audit_log_id_fkey FOREIGN KEY (audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY report_archives
    ADD CONSTRAINT report_archives_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brands(id);

ALTER TABLE ONLY report_archives
    ADD CONSTRAINT report_archives_brand_id_previous_id_fkey FOREIGN KEY (brand_id, previous_id) REFERENCES report_archives(brand_id, id);

ALTER TABLE ONLY report_archives
    ADD CONSTRAINT report_archives_created_by_fkey FOREIGN KEY (created_by) REFERENCES admin_accounts(id);

ALTER TABLE ONLY reward_order_actions
    ADD CONSTRAINT reward_order_actions_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES admin_accounts(id);

ALTER TABLE ONLY reward_order_actions
    ADD CONSTRAINT reward_order_actions_audit_log_id_fkey FOREIGN KEY (audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY reward_order_actions
    ADD CONSTRAINT reward_order_actions_brand_id_order_id_fkey FOREIGN KEY (brand_id, order_id) REFERENCES reward_orders(brand_id, id);

ALTER TABLE ONLY reward_order_actions
    ADD CONSTRAINT reward_order_actions_ledger_entry_id_fkey FOREIGN KEY (ledger_entry_id) REFERENCES point_ledger_entries(id);

ALTER TABLE ONLY reward_orders
    ADD CONSTRAINT reward_orders_brand_id_member_id_fkey FOREIGN KEY (brand_id, member_id) REFERENCES brand_members(brand_id, id);

ALTER TABLE ONLY reward_orders
    ADD CONSTRAINT reward_orders_created_by_fkey FOREIGN KEY (created_by) REFERENCES admin_accounts(id);

ALTER TABLE ONLY reward_orders
    ADD CONSTRAINT reward_orders_creation_audit_log_id_fkey FOREIGN KEY (creation_audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY reward_orders
    ADD CONSTRAINT reward_orders_grant_ledger_entry_id_fkey FOREIGN KEY (grant_ledger_entry_id) REFERENCES point_ledger_entries(id);

ALTER TABLE ONLY reward_orders
    ADD CONSTRAINT reward_orders_last_audit_log_id_fkey FOREIGN KEY (last_audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY reward_orders
    ADD CONSTRAINT reward_orders_revoke_ledger_entry_id_fkey FOREIGN KEY (revoke_ledger_entry_id) REFERENCES point_ledger_entries(id);

ALTER TABLE ONLY role_permissions
    ADD CONSTRAINT role_permissions_permission_key_fkey FOREIGN KEY (permission_key) REFERENCES permissions(key);

ALTER TABLE ONLY role_permissions
    ADD CONSTRAINT role_permissions_role_id_fkey FOREIGN KEY (role_id) REFERENCES roles(id);

ALTER TABLE ONLY roles
    ADD CONSTRAINT roles_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brands(id);

ALTER TABLE ONLY rule_version_contributors
    ADD CONSTRAINT rule_version_contributors_admin_id_fkey FOREIGN KEY (admin_id) REFERENCES admin_accounts(id);

ALTER TABLE ONLY rule_version_contributors
    ADD CONSTRAINT rule_version_contributors_rule_version_id_fkey FOREIGN KEY (rule_version_id) REFERENCES rule_versions(id);

ALTER TABLE ONLY rule_versions
    ADD CONSTRAINT rule_versions_brand_id_game_id_effective_period_id_fkey FOREIGN KEY (brand_id, game_id, effective_period_id) REFERENCES periods(brand_id, game_id, id);

ALTER TABLE ONLY rule_versions
    ADD CONSTRAINT rule_versions_brand_id_game_id_play_id_fkey FOREIGN KEY (brand_id, game_id, play_id) REFERENCES play_definitions(brand_id, game_id, id);

ALTER TABLE ONLY rule_versions
    ADD CONSTRAINT rule_versions_brand_id_game_id_play_id_source_version_id_fkey FOREIGN KEY (brand_id, game_id, play_id, source_version_id) REFERENCES rule_versions(brand_id, game_id, play_id, id);

ALTER TABLE ONLY rule_versions
    ADD CONSTRAINT rule_versions_created_by_fkey FOREIGN KEY (created_by) REFERENCES admin_accounts(id);

ALTER TABLE ONLY rule_versions
    ADD CONSTRAINT rule_versions_reviewed_by_fkey FOREIGN KEY (reviewed_by) REFERENCES admin_accounts(id);

ALTER TABLE ONLY sessions
    ADD CONSTRAINT session_member_identity FOREIGN KEY (brand_id, member_id, user_id) REFERENCES brand_members(brand_id, id, global_user_id);

ALTER TABLE ONLY sessions
    ADD CONSTRAINT sessions_admin_id_fkey FOREIGN KEY (admin_id) REFERENCES admin_accounts(id);

ALTER TABLE ONLY sessions
    ADD CONSTRAINT sessions_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brands(id);

ALTER TABLE ONLY sessions
    ADD CONSTRAINT sessions_brand_id_member_id_fkey FOREIGN KEY (brand_id, member_id) REFERENCES brand_members(brand_id, id);

ALTER TABLE ONLY sessions
    ADD CONSTRAINT sessions_user_id_fkey FOREIGN KEY (user_id) REFERENCES global_users(id);

ALTER TABLE ONLY settlement_calculations
    ADD CONSTRAINT settlement_calculations_brand_id_period_id_job_id_fkey FOREIGN KEY (brand_id, period_id, job_id) REFERENCES settlement_jobs(brand_id, period_id, id);

ALTER TABLE ONLY settlement_calculations
    ADD CONSTRAINT settlement_calculations_brand_id_period_id_order_id_fkey FOREIGN KEY (brand_id, period_id, order_id) REFERENCES bet_orders(brand_id, period_id, id);

ALTER TABLE ONLY settlement_failures
    ADD CONSTRAINT settlement_failures_brand_id_job_id_fkey FOREIGN KEY (brand_id, job_id) REFERENCES settlement_jobs(brand_id, id);

ALTER TABLE ONLY settlement_failures
    ADD CONSTRAINT settlement_failures_job_id_order_id_fkey FOREIGN KEY (job_id, order_id) REFERENCES settlement_targets(job_id, order_id);

ALTER TABLE ONLY settlement_jobs
    ADD CONSTRAINT settlement_jobs_approved_by_fkey FOREIGN KEY (approved_by) REFERENCES admin_accounts(id);

ALTER TABLE ONLY settlement_jobs
    ADD CONSTRAINT settlement_jobs_brand_id_game_id_period_id_draw_result_id_fkey FOREIGN KEY (brand_id, game_id, period_id, draw_result_id) REFERENCES draw_results(brand_id, game_id, period_id, id);

ALTER TABLE ONLY settlement_jobs
    ADD CONSTRAINT settlement_jobs_brand_id_period_id_correction_id_fkey FOREIGN KEY (brand_id, period_id, correction_id) REFERENCES draw_corrections(brand_id, period_id, id);

ALTER TABLE ONLY settlement_jobs
    ADD CONSTRAINT settlement_jobs_brand_id_period_id_previous_job_id_fkey FOREIGN KEY (brand_id, period_id, previous_job_id) REFERENCES settlement_jobs(brand_id, period_id, id);

ALTER TABLE ONLY settlement_jobs
    ADD CONSTRAINT settlement_jobs_brand_id_policy_version_fkey FOREIGN KEY (brand_id, policy_version) REFERENCES settlement_policy_history(brand_id, version);

ALTER TABLE ONLY settlement_jobs
    ADD CONSTRAINT settlement_jobs_created_by_fkey FOREIGN KEY (created_by) REFERENCES admin_accounts(id);

ALTER TABLE ONLY settlement_policy_history
    ADD CONSTRAINT settlement_policy_history_audit_log_id_fkey FOREIGN KEY (audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY settlement_policy_history
    ADD CONSTRAINT settlement_policy_history_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brands(id);

ALTER TABLE ONLY settlement_policy_history
    ADD CONSTRAINT settlement_policy_history_changed_by_fkey FOREIGN KEY (changed_by) REFERENCES admin_accounts(id);

ALTER TABLE ONLY settlement_previews
    ADD CONSTRAINT settlement_previews_audit_log_id_fkey FOREIGN KEY (audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY settlement_previews
    ADD CONSTRAINT settlement_previews_brand_id_game_id_period_id_draw_result_fkey FOREIGN KEY (brand_id, game_id, period_id, draw_result_id) REFERENCES draw_results(brand_id, game_id, period_id, id);

ALTER TABLE ONLY settlement_previews
    ADD CONSTRAINT settlement_previews_brand_id_game_id_period_id_fkey FOREIGN KEY (brand_id, game_id, period_id) REFERENCES periods(brand_id, game_id, id);

ALTER TABLE ONLY settlement_previews
    ADD CONSTRAINT settlement_previews_brand_id_period_id_order_id_fkey FOREIGN KEY (brand_id, period_id, order_id) REFERENCES bet_orders(brand_id, period_id, id);

ALTER TABLE ONLY settlement_previews
    ADD CONSTRAINT settlement_previews_created_by_fkey FOREIGN KEY (created_by) REFERENCES admin_accounts(id);

ALTER TABLE ONLY settlement_targets
    ADD CONSTRAINT settlement_targets_brand_id_order_id_calculation_id_fkey FOREIGN KEY (brand_id, order_id, calculation_id) REFERENCES settlement_calculations(brand_id, order_id, id);

ALTER TABLE ONLY settlement_targets
    ADD CONSTRAINT settlement_targets_brand_id_period_id_job_id_fkey FOREIGN KEY (brand_id, period_id, job_id) REFERENCES settlement_jobs(brand_id, period_id, id);

ALTER TABLE ONLY settlement_targets
    ADD CONSTRAINT settlement_targets_brand_id_period_id_order_id_fkey FOREIGN KEY (brand_id, period_id, order_id) REFERENCES bet_orders(brand_id, period_id, id);

ALTER TABLE ONLY settlement_targets
    ADD CONSTRAINT settlement_targets_payout_entry_id_fkey FOREIGN KEY (payout_entry_id) REFERENCES point_ledger_entries(id);

ALTER TABLE ONLY withdrawal_operation_receipts
    ADD CONSTRAINT withdrawal_operation_receipts_brand_id_order_id_fkey FOREIGN KEY (brand_id, order_id) REFERENCES withdrawal_orders(brand_id, id);

ALTER TABLE ONLY withdrawal_order_transitions
    ADD CONSTRAINT withdrawal_order_transitions_audit_log_id_fkey FOREIGN KEY (audit_log_id) REFERENCES audit_logs(id);

ALTER TABLE ONLY withdrawal_order_transitions
    ADD CONSTRAINT withdrawal_order_transitions_brand_id_order_id_fkey FOREIGN KEY (brand_id, order_id) REFERENCES withdrawal_orders(brand_id, id);

ALTER TABLE ONLY withdrawal_orders
    ADD CONSTRAINT withdrawal_orders_brand_id_account_id_member_id_fkey FOREIGN KEY (brand_id, account_id, member_id) REFERENCES point_accounts(brand_id, id, brand_member_id);

ALTER TABLE ONLY withdrawal_orders
    ADD CONSTRAINT withdrawal_orders_brand_id_account_id_paid_entry_id_fkey FOREIGN KEY (brand_id, account_id, paid_entry_id) REFERENCES point_ledger_entries(brand_id, account_id, id);

ALTER TABLE ONLY withdrawal_orders
    ADD CONSTRAINT withdrawal_orders_brand_id_account_id_release_entry_id_fkey FOREIGN KEY (brand_id, account_id, release_entry_id) REFERENCES point_ledger_entries(brand_id, account_id, id);

ALTER TABLE ONLY withdrawal_orders
    ADD CONSTRAINT withdrawal_orders_brand_id_account_id_reserve_entry_id_fkey FOREIGN KEY (brand_id, account_id, reserve_entry_id) REFERENCES point_ledger_entries(brand_id, account_id, id);

ALTER TABLE ONLY withdrawal_policy_revisions
    ADD CONSTRAINT withdrawal_policy_revisions_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brands(id);

ALTER TABLE ONLY withdrawal_policy_revisions
    ADD CONSTRAINT withdrawal_policy_revisions_brand_id_game_id_fkey FOREIGN KEY (brand_id, game_id) REFERENCES games(brand_id, id);

ALTER TABLE ONLY withdrawal_policy_revisions
    ADD CONSTRAINT withdrawal_policy_revisions_changed_by_fkey FOREIGN KEY (changed_by) REFERENCES admin_accounts(id);

ALTER TABLE ONLY withdrawal_turnover_cycles
    ADD CONSTRAINT withdrawal_turnover_cycles_brand_id_account_id_member_id_fkey FOREIGN KEY (brand_id, account_id, member_id) REFERENCES point_accounts(brand_id, id, brand_member_id);

ALTER TABLE ONLY withdrawal_turnover_cycles
    ADD CONSTRAINT withdrawal_turnover_cycles_brand_id_last_paid_order_id_fkey FOREIGN KEY (brand_id, last_paid_order_id) REFERENCES withdrawal_orders(brand_id, id);

INSERT INTO permissions VALUES ('user.create.brand');
INSERT INTO permissions VALUES ('role.view.brand');
INSERT INTO permissions VALUES ('role.write.brand');
INSERT INTO permissions VALUES ('admin.view.brand');
INSERT INTO permissions VALUES ('admin.write.brand');
INSERT INTO permissions VALUES ('auth_config.view.brand');
INSERT INTO permissions VALUES ('auth_config.write.brand');
INSERT INTO permissions VALUES ('role.view.platform');
INSERT INTO permissions VALUES ('role.write.platform');
INSERT INTO permissions VALUES ('admin.view.platform');
INSERT INTO permissions VALUES ('admin.write.platform');
INSERT INTO permissions VALUES ('auth_config.view.platform');
INSERT INTO permissions VALUES ('auth_config.write.platform');
INSERT INTO permissions VALUES ('wallet.view.brand');
INSERT INTO permissions VALUES ('wallet.view.platform');
INSERT INTO permissions VALUES ('wallet.freeze.brand');
INSERT INTO permissions VALUES ('wallet.adjust.brand');
INSERT INTO permissions VALUES ('recharge.view.brand');
INSERT INTO permissions VALUES ('recharge.view.platform');
INSERT INTO permissions VALUES ('recharge.write.brand');
INSERT INTO permissions VALUES ('point_policy.view.brand');
INSERT INTO permissions VALUES ('point_policy.view.platform');
INSERT INTO permissions VALUES ('point_policy.write.brand');
INSERT INTO permissions VALUES ('wallet.repair.brand');
INSERT INTO permissions VALUES ('rule.simulate.brand');
INSERT INTO permissions VALUES ('rule.simulate.platform');
INSERT INTO permissions VALUES ('game.view.brand');
INSERT INTO permissions VALUES ('game.view.platform');
INSERT INTO permissions VALUES ('game.write.brand');
INSERT INTO permissions VALUES ('rule.view.brand');
INSERT INTO permissions VALUES ('rule.view.platform');
INSERT INTO permissions VALUES ('rule.write.brand');
INSERT INTO permissions VALUES ('rule.validate.brand');
INSERT INTO permissions VALUES ('rule.submit.brand');
INSERT INTO permissions VALUES ('rule.review.brand');
INSERT INTO permissions VALUES ('schedule.view.brand');
INSERT INTO permissions VALUES ('schedule.view.platform');
INSERT INTO permissions VALUES ('schedule.write.brand');
INSERT INTO permissions VALUES ('period.view.brand');
INSERT INTO permissions VALUES ('period.view.platform');
INSERT INTO permissions VALUES ('period.generate.brand');
INSERT INTO permissions VALUES ('draw_source.view.brand');
INSERT INTO permissions VALUES ('draw_source.view.platform');
INSERT INTO permissions VALUES ('draw_source.write.brand');
INSERT INTO permissions VALUES ('draw.view.brand');
INSERT INTO permissions VALUES ('draw.view.platform');
INSERT INTO permissions VALUES ('draw.manual_create.brand');
INSERT INTO permissions VALUES ('bet_policy.view.brand');
INSERT INTO permissions VALUES ('bet_policy.view.platform');
INSERT INTO permissions VALUES ('bet_policy.write.brand');
INSERT INTO permissions VALUES ('bet.view.brand');
INSERT INTO permissions VALUES ('bet.view.platform');
INSERT INTO permissions VALUES ('bet.cancel.brand');
INSERT INTO permissions VALUES ('bet.mark_abnormal.brand');
INSERT INTO permissions VALUES ('period.cancel.brand');
INSERT INTO permissions VALUES ('period.cancel_retry.brand');
INSERT INTO permissions VALUES ('bet.judge_cancel.brand');
INSERT INTO permissions VALUES ('withdrawal_policy.view.brand');
INSERT INTO permissions VALUES ('withdrawal_policy.view.platform');
INSERT INTO permissions VALUES ('withdrawal_policy.write.brand');
INSERT INTO permissions VALUES ('notification.view.brand');
INSERT INTO permissions VALUES ('notification.retry.brand');
INSERT INTO permissions VALUES ('notification.view.platform');
INSERT INTO permissions VALUES ('settlement.view.brand');
INSERT INTO permissions VALUES ('settlement.view.platform');
INSERT INTO permissions VALUES ('settlement.preview.brand');
INSERT INTO permissions VALUES ('settlement.run.brand');
INSERT INTO permissions VALUES ('settlement.approve.brand');
INSERT INTO permissions VALUES ('settlement.retry.brand');
INSERT INTO permissions VALUES ('settlement_policy.view.brand');
INSERT INTO permissions VALUES ('settlement_policy.view.platform');
INSERT INTO permissions VALUES ('settlement_policy.write.brand');
INSERT INTO permissions VALUES ('draw.correct.brand');
INSERT INTO permissions VALUES ('draw.correction_retry.brand');
INSERT INTO permissions VALUES ('report_betting.view.brand');
INSERT INTO permissions VALUES ('report_betting.view.platform');
INSERT INTO permissions VALUES ('report_ledger.view.brand');
INSERT INTO permissions VALUES ('report_ledger.view.platform');
INSERT INTO permissions VALUES ('agent.view.brand');
INSERT INTO permissions VALUES ('agent.view.platform');
INSERT INTO permissions VALUES ('agent.write.brand');
INSERT INTO permissions VALUES ('agent_policy.view.brand');
INSERT INTO permissions VALUES ('agent_policy.view.platform');
INSERT INTO permissions VALUES ('agent_policy.write.brand');
INSERT INTO permissions VALUES ('join_code.view.brand');
INSERT INTO permissions VALUES ('join_code.view.platform');
INSERT INTO permissions VALUES ('join_code.write.brand');
INSERT INTO permissions VALUES ('brand_operation.view.brand');
INSERT INTO permissions VALUES ('brand_operation.view.platform');
INSERT INTO permissions VALUES ('brand_operation.write.brand');
INSERT INTO permissions VALUES ('brand_operation.write.platform');
INSERT INTO permissions VALUES ('brand_presentation.view.brand');
INSERT INTO permissions VALUES ('brand_presentation.write.brand');
INSERT INTO permissions VALUES ('brand_presentation.view.platform');
INSERT INTO permissions VALUES ('brand_presentation.write.platform');
INSERT INTO permissions VALUES ('brand_domains.view.brand');
INSERT INTO permissions VALUES ('brand_domains.write.brand');
INSERT INTO permissions VALUES ('brand_domains.view.platform');
INSERT INTO permissions VALUES ('brand_domains.write.platform');
INSERT INTO permissions VALUES ('brand.create.platform');
INSERT INTO permissions VALUES ('compliance_policy.view.brand');
INSERT INTO permissions VALUES ('compliance_policy.view.platform');
INSERT INTO permissions VALUES ('compliance_policy.write.brand');
INSERT INTO permissions VALUES ('compliance_check.view.brand');
INSERT INTO permissions VALUES ('compliance_check.view.platform');
INSERT INTO permissions VALUES ('compliance_check.run.brand');
INSERT INTO permissions VALUES ('report_betting.export.brand');
INSERT INTO permissions VALUES ('report_betting.export.platform');
INSERT INTO permissions VALUES ('report_ledger.export.brand');
INSERT INTO permissions VALUES ('report_ledger.export.platform');
INSERT INTO permissions VALUES ('notification_template.view.brand');
INSERT INTO permissions VALUES ('notification_template.view.platform');
INSERT INTO permissions VALUES ('notification_template.write.brand');
INSERT INTO permissions VALUES ('wallet.reconcile.brand');
INSERT INTO permissions VALUES ('withdrawal.view.brand');
INSERT INTO permissions VALUES ('withdrawal.view.platform');
INSERT INTO permissions VALUES ('withdrawal.approve.brand');
INSERT INTO permissions VALUES ('withdrawal.reject.brand');
INSERT INTO permissions VALUES ('withdrawal.cancel.brand');
INSERT INTO permissions VALUES ('withdrawal.fail.brand');
INSERT INTO permissions VALUES ('withdrawal.mark_paid.brand');
INSERT INTO permissions VALUES ('report_withdrawal.view.brand');
INSERT INTO permissions VALUES ('report_withdrawal.view.platform');
INSERT INTO permissions VALUES ('report_withdrawal.export.brand');
INSERT INTO permissions VALUES ('report_withdrawal.export.platform');
INSERT INTO permissions VALUES ('commission_policy.view.brand');
INSERT INTO permissions VALUES ('commission_policy.view.platform');
INSERT INTO permissions VALUES ('commission_policy.write.brand');
INSERT INTO permissions VALUES ('commission.view.brand');
INSERT INTO permissions VALUES ('commission.view.platform');
INSERT INTO permissions VALUES ('commission.run.brand');
INSERT INTO permissions VALUES ('commission.retry.brand');
INSERT INTO permissions VALUES ('commission_payment.approve.brand');
INSERT INTO permissions VALUES ('commission_payment.retry.brand');
INSERT INTO permissions VALUES ('commission_payment_policy.write.brand');
INSERT INTO permissions VALUES ('commission_adjustment.write.brand');
INSERT INTO permissions VALUES ('report_commission.view.brand');
INSERT INTO permissions VALUES ('report_commission.view.platform');
INSERT INTO permissions VALUES ('report_commission.export.brand');
INSERT INTO permissions VALUES ('report_commission.export.platform');
INSERT INTO permissions VALUES ('reward.view.brand');
INSERT INTO permissions VALUES ('reward.view.platform');
INSERT INTO permissions VALUES ('reward.grant.brand');
INSERT INTO permissions VALUES ('reward.revoke.brand');
INSERT INTO permissions VALUES ('reward.retry.brand');
INSERT INTO permissions VALUES ('report_reward.view.brand');
INSERT INTO permissions VALUES ('report_reward.view.platform');
INSERT INTO permissions VALUES ('report_reward.export.brand');
INSERT INTO permissions VALUES ('report_reward.export.platform');
INSERT INTO permissions VALUES ('commission_correction.retry.brand');
INSERT INTO permissions VALUES ('commission_correction.approve.brand');
INSERT INTO permissions VALUES ('commission_correction.continue.brand');
INSERT INTO permissions VALUES ('commission_correction.execute_retry.brand');
INSERT INTO permissions VALUES ('commission_correction_policy.write.brand');
INSERT INTO permissions VALUES ('report_archive.view.brand');
INSERT INTO permissions VALUES ('report_archive.create.brand');
INSERT INTO permissions VALUES ('report_archive.download.brand');
INSERT INTO permissions VALUES ('report_archive.view.platform');
INSERT INTO permissions VALUES ('report_archive.download.platform');
INSERT INTO permissions VALUES ('report_archive_policy.write.brand');
INSERT INTO permissions VALUES ('report_archive_task.retry.brand');
INSERT INTO permissions VALUES ('report_attribution.view.brand');
INSERT INTO permissions VALUES ('report_attribution.view.platform');
INSERT INTO permissions VALUES ('report_attribution.export.brand');
INSERT INTO permissions VALUES ('report_attribution.export.platform');
INSERT INTO permissions VALUES ('audit.export.brand');
INSERT INTO permissions VALUES ('audit.export.platform');

DO $pin$ DECLARE app_schema text:=current_schema(); f record; BEGIN
 FOR f IN SELECT p.oid::regprocedure AS signature FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname=app_schema LOOP
  EXECUTE format('ALTER FUNCTION %s SET search_path TO pg_catalog, %I, pg_temp',f.signature,app_schema);
 END LOOP;
END $pin$;
