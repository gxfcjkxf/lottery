-- Generate the installed query from the migration-time catalog. FK names,
-- tuples, and source primary keys are frozen here; runtime checks do not read
-- pg_constraint and use native, indexable parent-column predicates.
DO $migration$
DECLARE
    v_schema text := current_schema();
    v_sources text[] := ARRAY[
        'bet_order_exceptions','bet_order_judgments','bet_orders',
        'commission_adjustment_heads','commission_adjustments','commission_allocations',
        'commission_calculations','commission_correction_balance_heads',
        'commission_correction_cycle_holds','commission_correction_execution_steps',
        'commission_correction_execution_targets','commission_correction_executions',
        'commission_correction_plan_steps','commission_correction_plan_targets',
        'commission_correction_plans','commission_cycle_steps','commission_cycle_targets',
        'commission_cycles','commission_earnings','commission_payment_targets',
        'commission_payments','commission_runs','draw_correction_failures',
        'draw_correction_targets','draw_corrections','period_cancellation_targets',
        'period_cancellations','point_accounts','point_buckets','point_ledger_entries',
        'recharge_orders','reward_order_actions','reward_orders',
        'settlement_calculations','settlement_failures','settlement_jobs',
        'settlement_targets','withdrawal_operation_receipts',
        'withdrawal_order_transitions','withdrawal_orders','withdrawal_turnover_cycles'
    ];
    v_expected_primary_keys jsonb := '{
      "bet_order_exceptions":["id"],"bet_order_judgments":["id"],"bet_orders":["id"],
      "commission_adjustment_heads":["target_id"],"commission_adjustments":["id"],
      "commission_allocations":["run_id","order_id","agent_id"],"commission_calculations":["id"],
      "commission_correction_balance_heads":["cycle_id","agent_id"],
      "commission_correction_cycle_holds":["cycle_id"],"commission_correction_execution_steps":["id"],
      "commission_correction_execution_targets":["id"],"commission_correction_executions":["id"],
      "commission_correction_plan_steps":["id"],"commission_correction_plan_targets":["id"],
      "commission_correction_plans":["id"],"commission_cycle_steps":["id"],
      "commission_cycle_targets":["cycle_id","order_id"],"commission_cycles":["id"],
      "commission_earnings":["id"],"commission_payment_targets":["id"],
      "commission_payments":["id"],"commission_runs":["id"],"draw_correction_failures":["id"],
      "draw_correction_targets":["correction_id","order_id"],"draw_corrections":["id"],
      "period_cancellation_targets":["cancellation_id","order_id"],"period_cancellations":["id"],
      "point_accounts":["id"],"point_buckets":["brand_id","account_id","source","state"],
      "point_ledger_entries":["id"],"recharge_orders":["id"],"reward_order_actions":["id"],
      "reward_orders":["id"],"settlement_calculations":["id"],"settlement_failures":["id"],
      "settlement_jobs":["id"],"settlement_targets":["job_id","order_id"],
      "withdrawal_operation_receipts":["id"],"withdrawal_order_transitions":["id"],
      "withdrawal_orders":["id"],"withdrawal_turnover_cycles":["brand_id","member_id"]
    }'::jsonb;
    v_expected_fk_counts jsonb := '{
      "bet_order_exceptions":3,"bet_order_judgments":4,"bet_orders":8,
      "commission_adjustment_heads":2,"commission_adjustments":5,"commission_allocations":3,
      "commission_calculations":4,"commission_correction_balance_heads":4,
      "commission_correction_cycle_holds":3,"commission_correction_execution_steps":5,
      "commission_correction_execution_targets":4,"commission_correction_executions":6,
      "commission_correction_plan_steps":3,"commission_correction_plan_targets":6,
      "commission_correction_plans":4,"commission_cycle_steps":3,"commission_cycle_targets":2,
      "commission_cycles":5,"commission_earnings":3,"commission_payment_targets":4,
      "commission_payments":5,"commission_runs":1,"draw_correction_failures":2,
      "draw_correction_targets":5,"draw_corrections":6,"period_cancellation_targets":3,
      "period_cancellations":3,"point_accounts":1,"point_buckets":1,"point_ledger_entries":4,
      "recharge_orders":4,"reward_order_actions":4,"reward_orders":6,
      "settlement_calculations":2,"settlement_failures":2,"settlement_jobs":6,
      "settlement_targets":4,"withdrawal_operation_receipts":1,
      "withdrawal_order_transitions":2,"withdrawal_orders":4,"withdrawal_turnover_cycles":2
    }'::jsonb;
    v_pks jsonb;
    v_fks jsonb;
    v_actual_primary_keys jsonb;
    v_actual_fk_counts jsonb;
    v_source_sql text;
    v_reference_sql text := '';
    v_required_sql text := '';
    v_query text;
    v_body text;
    v_fk jsonb;
    v_pk jsonb;
    v_source_cols jsonb;
    v_parent_cols jsonb;
    v_source_id_expr text;
    v_parent_predicate text;
    v_nonnull_predicate text;
    v_table text;
BEGIN
    SELECT coalesce(jsonb_agg(jsonb_build_object(
               'table', t.relname,
               'columns', (SELECT jsonb_agg(a.attname ORDER BY k.ord)
                           FROM unnest(i.indkey) WITH ORDINALITY k(attnum,ord)
                           JOIN pg_attribute a ON a.attrelid=t.oid AND a.attnum=k.attnum
                           WHERE k.ord <= i.indnkeyatts)
           ) ORDER BY t.relname), '[]'::jsonb)
      INTO v_pks
      FROM pg_class t
      JOIN pg_namespace n ON n.oid=t.relnamespace
      JOIN pg_index i ON i.indrelid=t.oid AND i.indisprimary
     WHERE n.nspname=v_schema AND t.relname=ANY(v_sources);

    IF jsonb_array_length(v_pks) <> cardinality(v_sources) THEN
        RAISE EXCEPTION 'brand business inventory source table or primary key is missing';
    END IF;
    SELECT jsonb_object_agg(p.value->>'table',p.value->'columns')
      INTO v_actual_primary_keys FROM jsonb_array_elements(v_pks) p(value);
    IF v_actual_primary_keys IS DISTINCT FROM v_expected_primary_keys THEN
        RAISE EXCEPTION 'brand business inventory primary-key manifest differs from the supported schema';
    END IF;

    SELECT jsonb_object_agg(f.relname,f.fk_count)
      INTO v_actual_fk_counts
      FROM (
        SELECT src.relname,count(*) AS fk_count
          FROM pg_constraint c
          JOIN pg_class src ON src.oid=c.conrelid
          JOIN pg_namespace sn ON sn.oid=src.relnamespace
         WHERE c.contype='f' AND sn.nspname=v_schema AND src.relname=ANY(v_sources)
         GROUP BY src.relname
      ) f;
    IF v_actual_fk_counts IS DISTINCT FROM v_expected_fk_counts THEN
        RAISE EXCEPTION 'brand business inventory foreign-key manifest differs from the supported schema';
    END IF;

    IF EXISTS (
        SELECT 1 FROM pg_constraint c
        JOIN pg_class src ON src.oid=c.conrelid
        JOIN pg_namespace sn ON sn.oid=src.relnamespace
        WHERE c.contype='f' AND c.confmatchtype<>'s'
          AND sn.nspname=v_schema AND src.relname=ANY(v_sources)
    ) THEN
        RAISE EXCEPTION 'brand business inventory expects MATCH SIMPLE foreign keys';
    END IF;

    IF EXISTS (
        SELECT 1 FROM pg_constraint c
        JOIN pg_class src ON src.oid=c.conrelid
        JOIN pg_namespace sn ON sn.oid=src.relnamespace
        JOIN pg_class dst ON dst.oid=c.confrelid
        WHERE c.contype='f' AND sn.nspname=v_schema AND src.relname=ANY(v_sources)
          AND dst.relnamespace<>src.relnamespace
    ) THEN
        RAISE EXCEPTION 'brand business inventory foreign-key tuple manifest differs: external parent schema';
    END IF;

    SELECT coalesce(jsonb_agg(jsonb_build_object(
               'source', src.relname,
               'constraint', c.conname,
               'parent', dst.relname,
               'parent_has_brand', EXISTS (
                   SELECT 1 FROM pg_attribute ba
                    WHERE ba.attrelid=dst.oid AND ba.attname='brand_id'
                      AND ba.attnum>0 AND NOT ba.attisdropped),
               'source_columns', (SELECT jsonb_agg(sa.attname ORDER BY k.ord)
                    FROM unnest(c.conkey) WITH ORDINALITY k(attnum,ord)
                    JOIN pg_attribute sa ON sa.attrelid=src.oid AND sa.attnum=k.attnum),
               'parent_columns', (SELECT jsonb_agg(da.attname ORDER BY k.ord)
                    FROM unnest(c.confkey) WITH ORDINALITY k(attnum,ord)
                    JOIN pg_attribute da ON da.attrelid=dst.oid AND da.attnum=k.attnum)
           ) ORDER BY src.relname COLLATE "C",c.conname COLLATE "C"), '[]'::jsonb)
      INTO v_fks
      FROM pg_constraint c
      JOIN pg_class src ON src.oid=c.conrelid
      JOIN pg_namespace sn ON sn.oid=src.relnamespace
      JOIN pg_class dst ON dst.oid=c.confrelid
     WHERE c.contype='f' AND sn.nspname=v_schema AND src.relname=ANY(v_sources);

    -- Canonical manifest of the committed 0001-0071 schema: exact source,
    -- constraint label, parent table/brand ownership and ordered FK columns.
    -- Schema names are checked as local above, not hashed, so isolated schemas
    -- and configured application schemas have identical structural evidence.
    IF encode(pg_catalog.sha256(convert_to(v_fks::text,'UTF8')),'hex') <>
       'ce23a1a2f4631e2190b9fb3a87a00f474a3653207a5e712da8709f39a585fa86' THEN
        RAISE EXCEPTION 'brand business inventory foreign-key tuple manifest differs from the supported schema';
    END IF;

    SELECT string_agg(
             format('SELECT %L::text AS source_table, concat_ws(''/'',%s) AS source_id, '
                    'encode(pg_catalog.sha256(convert_to(to_jsonb(s)::text,''UTF8'')),''hex'') AS row_hash '
                    'FROM %I.%I s WHERE s.brand_id=$1',
                    t.relname,
                    (SELECT string_agg(format('s.%I::text',k.value),',' ORDER BY k.ord)
                       FROM jsonb_array_elements_text(pk.value->'columns') WITH ORDINALITY k(value,ord)),
                    v_schema,t.relname),
             ' UNION ALL ' ORDER BY t.relname)
      INTO v_source_sql
      FROM pg_class t
      JOIN pg_namespace n ON n.oid=t.relnamespace
      JOIN jsonb_array_elements(v_pks) pk(value) ON pk.value->>'table'=t.relname
     WHERE n.nspname=v_schema AND t.relname=ANY(v_sources);

    FOR v_fk IN SELECT e.value FROM jsonb_array_elements(v_fks) e(value)
                 ORDER BY e.value->>'source',e.value->>'constraint' COLLATE "C"
    LOOP
        v_source_cols := v_fk->'source_columns';
        v_parent_cols := v_fk->'parent_columns';
        v_pk := (SELECT p.value FROM jsonb_array_elements(v_pks) p(value)
                  WHERE p.value->>'table'=v_fk->>'source');
        SELECT string_agg(format('p.%I=s.%I',pcol.value,scol.value),' AND ' ORDER BY pcol.ord),
               string_agg(format('s.%I IS NOT NULL',scol.value),' AND ' ORDER BY scol.ord)
          INTO v_parent_predicate,v_nonnull_predicate
          FROM jsonb_array_elements_text(v_parent_cols) WITH ORDINALITY pcol(value,ord)
          JOIN jsonb_array_elements_text(v_source_cols) WITH ORDINALITY scol(value,ord) USING(ord);
        SELECT string_agg(format('s.%I::text',k.value),',' ORDER BY k.ord)
          INTO v_source_id_expr
          FROM jsonb_array_elements_text(v_pk->'columns') WITH ORDINALITY k(value,ord);

        v_reference_sql := concat_ws(' UNION ALL ',nullif(v_reference_sql,''),
            format('SELECT %L::text AS source_table, concat_ws(''/'',%s) AS source_id, '
                   '%L::text AS reference_key, %L::text AS parent_table, '
                   'NOT EXISTS (SELECT 1 FROM %I.%I p WHERE %s%s) AS missing '
                   'FROM %I.%I s WHERE s.brand_id=$1 AND %s',
                   v_fk->>'source',v_source_id_expr,v_fk->>'constraint',v_fk->>'parent',
                   v_schema,v_fk->>'parent',v_parent_predicate,
                   CASE WHEN (v_fk->>'parent_has_brand')::boolean THEN ' AND p.brand_id=$1' ELSE '' END,
                   v_schema,v_fk->>'source',v_nonnull_predicate));
    END LOOP;

    -- A NULL mandatory pointer is a distinct finding only in the actual
    -- movement states listed by the contract. Non-NULL pointers are checked
    -- by their migration-frozen FK tuple above.
    v_required_sql := concat_ws(' UNION ALL ',
      format('SELECT ''bet_orders''::text, concat_ws(''/'',s.id::text) AS source_id, ''debit_entry_id''::text, ''point_ledger_entries''::text, true FROM %I.bet_orders s WHERE s.brand_id=$1 AND s.debit_entry_id IS NULL',v_schema),
      format('SELECT ''bet_orders''::text, concat_ws(''/'',s.id::text), ''refund_entry_id''::text, ''point_ledger_entries''::text, true FROM %I.bet_orders s WHERE s.brand_id=$1 AND s.status IN (''bet_cancelled'',''judged_cancelled'') AND s.refund_entry_id IS NULL',v_schema),
      format('SELECT ''recharge_orders''::text, concat_ws(''/'',s.id::text), ''ledger_entry_id''::text, ''point_ledger_entries''::text, true FROM %I.recharge_orders s WHERE s.brand_id=$1 AND s.state=''confirmed'' AND s.ledger_entry_id IS NULL',v_schema),
      format('SELECT ''withdrawal_orders''::text, concat_ws(''/'',s.id::text), ''reserve_entry_id''::text, ''point_ledger_entries''::text, true FROM %I.withdrawal_orders s WHERE s.brand_id=$1 AND s.reserve_entry_id IS NULL',v_schema),
      format('SELECT ''withdrawal_orders''::text, concat_ws(''/'',s.id::text), ''paid_entry_id''::text, ''point_ledger_entries''::text, true FROM %I.withdrawal_orders s WHERE s.brand_id=$1 AND s.state=''paid'' AND s.paid_entry_id IS NULL',v_schema),
      format('SELECT ''withdrawal_orders''::text, concat_ws(''/'',s.id::text), ''release_entry_id''::text, ''point_ledger_entries''::text, true FROM %I.withdrawal_orders s WHERE s.brand_id=$1 AND s.state IN (''rejected'',''failed'',''cancelled'') AND s.release_entry_id IS NULL',v_schema),
      format('SELECT ''reward_orders''::text, concat_ws(''/'',s.id::text), ''grant_ledger_entry_id''::text, ''point_ledger_entries''::text, true FROM %I.reward_orders s WHERE s.brand_id=$1 AND s.state IN (''granted'',''revocation_pending'',''revoked'') AND s.grant_ledger_entry_id IS NULL',v_schema),
      format('SELECT ''reward_orders''::text, concat_ws(''/'',s.id::text), ''revoke_ledger_entry_id''::text, ''point_ledger_entries''::text, true FROM %I.reward_orders s WHERE s.brand_id=$1 AND s.state=''revoked'' AND s.revoke_ledger_entry_id IS NULL',v_schema),
      format('SELECT ''reward_order_actions''::text, concat_ws(''/'',s.id::text), ''ledger_entry_id''::text, ''point_ledger_entries''::text, true FROM %I.reward_order_actions s WHERE s.brand_id=$1 AND (s.operation=''grant'' OR s.state_after=''revoked'') AND s.ledger_entry_id IS NULL',v_schema),
      format('SELECT ''commission_payment_targets''::text, concat_ws(''/'',s.id::text), ''ledger_entry_id''::text, ''point_ledger_entries''::text, true FROM %I.commission_payment_targets s WHERE s.brand_id=$1 AND s.state=''paid'' AND s.points>0 AND s.ledger_entry_id IS NULL',v_schema),
      format('SELECT ''commission_adjustments''::text, concat_ws(''/'',s.id::text), ''ledger_entry_id''::text, ''point_ledger_entries''::text, true FROM %I.commission_adjustments s WHERE s.brand_id=$1 AND s.delta_points<>0 AND s.ledger_entry_id IS NULL',v_schema),
      format('SELECT ''commission_correction_execution_targets''::text, concat_ws(''/'',s.id::text), ''ledger_entry_id''::text, ''point_ledger_entries''::text, true FROM %I.commission_correction_execution_targets s WHERE s.brand_id=$1 AND s.state=''applied'' AND s.delta_points<>0 AND s.ledger_entry_id IS NULL',v_schema),
      format('SELECT ''settlement_targets''::text, concat_ws(''/'',s.job_id::text,s.order_id::text), ''payout_entry_id''::text, ''point_ledger_entries''::text, true FROM %I.settlement_targets s WHERE s.brand_id=$1 AND s.state=''paid'' AND s.payout_entry_id IS NULL AND EXISTS (SELECT 1 FROM %I.settlement_calculations c WHERE c.brand_id=s.brand_id AND c.id=s.calculation_id AND c.prize_points>0)',v_schema,v_schema),
      format('SELECT ''draw_correction_targets''::text, concat_ws(''/'',s.correction_id::text,s.order_id::text), ''old_payout_entry_id''::text, ''point_ledger_entries''::text, true FROM %I.draw_correction_targets s WHERE s.brand_id=$1 AND s.old_prize_points>0 AND s.old_payout_entry_id IS NULL',v_schema),
      format('SELECT ''draw_correction_targets''::text, concat_ws(''/'',s.correction_id::text,s.order_id::text), ''reversal_entry_id''::text, ''point_ledger_entries''::text, true FROM %I.draw_correction_targets s WHERE s.brand_id=$1 AND s.state=''reversed'' AND s.old_prize_points>0 AND s.reversal_entry_id IS NULL',v_schema));

    v_query := format($query$
WITH source_rows AS MATERIALIZED (%s),
refs AS MATERIALIZED (
    SELECT source_table,source_id,'MISSING_PARENT_REFERENCE'::text AS code,
           reference_key,parent_table,missing FROM (%s) fk_refs
    UNION ALL
    SELECT source_table,source_id,'MISSING_REQUIRED_LEDGER_REFERENCE'::text AS code,
           reference_key,parent_table,missing
      FROM (%s) AS required_refs(source_table,source_id,reference_key,parent_table,missing)
),
findings AS MATERIALIZED (
    SELECT source_table,source_id,code,reference_key,parent_table
      FROM refs WHERE missing
),
issue_doc AS (
    SELECT coalesce(jsonb_agg(jsonb_build_object(
             'source_table',source_table,'source_id',source_id,'code',code,
             'reference_key',reference_key,'parent_table',parent_table)
             ORDER BY source_table COLLATE "C",source_id COLLATE "C",
                      reference_key COLLATE "C",code COLLATE "C",parent_table COLLATE "C"),
             '[]'::jsonb) AS issues
      FROM findings
),
row_material AS (
    SELECT coalesce(string_agg(source_table||'|'||source_id||'|'||row_hash,chr(10)
             ORDER BY source_table COLLATE "C",source_id COLLATE "C"),'') AS material
      FROM source_rows
),
coverage AS (
    SELECT coalesce(jsonb_agg(jsonb_build_object(
             'source_table',t.source_table,
             'source_row_count',coalesce(sr.n,0)::text,
             'reference_count',coalesce(rr.n,0)::text,
             'issue_count',coalesce(ff.n,0)::text)
             ORDER BY t.ordinality), '[]'::jsonb) AS items
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
                    source_id !~ '^[A-Za-z0-9_.:/-]+$'),false) AS invalid
      FROM source_rows
),
visible AS (
    SELECT coalesce(jsonb_agg(x.value ORDER BY x.ord),'[]'::jsonb) AS issues
      FROM jsonb_array_elements((SELECT issues FROM issue_doc)) WITH ORDINALITY x(value,ord)
     WHERE x.ord<=100
)
SELECT jsonb_build_object(
    'brand_id',$1,'snapshot_at',statement_timestamp(),'schema_version',1,
    'source_row_count',totals.source_count::text,
    'reference_count',totals.reference_count::text,
    'issue_count',totals.issue_count::text,
    'issues_truncated',totals.issue_count>100,
    'consistent',totals.issue_count=0,
    'fingerprint',encode(pg_catalog.sha256(convert_to(
       $1::text||E'\n1\n'||row_material.material||E'\nF|'||issue_doc.issues::text,'UTF8')),'hex'),
    'coverage',coverage.items,'issues',visible.issues),
    source_id_check.invalid
  FROM totals CROSS JOIN issue_doc CROSS JOIN row_material CROSS JOIN coverage CROSS JOIN visible CROSS JOIN source_id_check
$query$,v_source_sql,v_reference_sql,v_required_sql);

    v_body := format($function$
DECLARE
    v_schema constant text := %L;
    v_tables constant text[] := %L::text[];
    v_snapshot_sql constant text := %L;
    v_table text;
    v_count bigint;
    v_total bigint := 0;
    v_brand_exists boolean;
    v_out jsonb;
    v_invalid_source_id boolean;
BEGIN
    IF b IS NULL THEN RETURN NULL; END IF;
    EXECUTE format('SELECT EXISTS(SELECT 1 FROM %%I.%%I WHERE id=$1)',v_schema,'brands')
       INTO v_brand_exists USING b;
    IF NOT v_brand_exists THEN RETURN NULL; END IF;

    -- Cheap bounded prepass: no source rows or parent records are materialized
    -- until the total across all 41 source tables is known to fit.
    FOREACH v_table IN ARRAY v_tables LOOP
        EXECUTE format('SELECT count(*) FROM %%I.%%I WHERE brand_id=$1',v_schema,v_table)
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
$function$,v_schema,v_sources::text,v_query);

    EXECUTE format(
      'CREATE FUNCTION brand_business_inventory(b uuid) RETURNS jsonb LANGUAGE plpgsql STABLE SECURITY INVOKER SET search_path=pg_catalog AS %L',
      v_body);
END
$migration$;
