package reporting

import "strings"

// Aggregate per (cycle, captured beneficiary), not a join of mutable heads or
// wallet balances. Separate source families prevent multiplicative history.
func commissionAnalysisTotalsSQL(complete string) string {
	fields := []string{}
	number := func(key, expr string) { fields = append(fields, "'"+key+"',("+expr+")::text") }
	nullable := func(key, expr string) {
		fields = append(fields, "'"+key+"',CASE WHEN "+complete+" THEN ("+expr+")::text ELSE NULL END")
	}
	calc := "coalesce(sum(calculated),0)"
	effective := "coalesce(sum(effective),0)"
	actual := "coalesce(sum(paid+adj_credit-adj_debit+corr_credit-corr_debit),0)"
	number("observed_calculated_points", calc)
	nullable("calculated_points", calc)
	for _, v := range [][2]string{{"paid_entry_count", "paid_count"}, {"paid_points", "paid"}, {"adjustment_entry_count", "adj_count"}, {"adjustment_credit_points", "adj_credit"}, {"adjustment_debit_points", "adj_debit"}, {"correction_entry_count", "corr_count"}, {"correction_credit_points", "corr_credit"}, {"correction_debit_points", "corr_debit"}} {
		number(v[0], "coalesce(sum("+v[1]+"),0)")
	}
	number("posting_entry_count", "coalesce(sum(paid_count+adj_count+corr_count),0)")
	number("actual_net_points", actual)
	number("manual_adjustment_net_points", "coalesce(sum(adj_credit-adj_debit),0)")
	nullable("effective_target_points", effective)
	nullable("calculation_minus_actual_points", calc+"-("+actual+")")
	nullable("effective_minus_actual_points", effective+"-("+actual+")")
	fields = append(fields, "'calculation_complete',"+complete, "'effective_target_complete',"+complete)
	return "jsonb_build_object(" + strings.Join(fields, ",") + ")"
}

func commissionAnalysisSQL(group string) string {
	key := "cycle_id::text"
	complete := "coalesce(bool_and(complete),true)"
	if group == "agent" {
		key = "agent_id::text"
		complete += " AND NOT EXISTS(SELECT 1 FROM cohort WHERE NOT ready)"
	}
	summaryComplete := "NOT EXISTS(SELECT 1 FROM cohort WHERE NOT ready)"
	return `WITH scope AS (SELECT id,timezone FROM brands WHERE id=$1), validity AS (
 SELECT ($4::uuid IS NULL OR EXISTS(SELECT 1 FROM agent_nodes WHERE brand_id=$1 AND id=$4))
 AND ($5::uuid IS NULL OR EXISTS(SELECT 1 FROM brand_members WHERE brand_id=$1 AND id=$5))
 AND ($6::uuid IS NULL OR EXISTS(SELECT 1 FROM commission_cycles WHERE brand_id=$1 AND id=$6)) AS valid),
 selected AS MATERIALIZED (
 SELECT c.*,coalesce(commission_payment_evidence_current($1,c.id,c.current_run_id,c.evidence_epoch),false) AS current_evidence
 FROM commission_cycles c WHERE c.brand_id=$1 AND c.window_to >= $2 AND c.window_to < $3 AND ($6::uuid IS NULL OR c.id=$6)),
 cohort AS MATERIALIZED (
 SELECT c.*,CASE WHEN current_evidence THEN commission_analysis_run_valid($1,c.id,c.current_run_id) ELSE false END AS ready FROM selected c),
 raw_edges AS MATERIALIZED (
 SELECT t.id AS source_id,p.cycle_id,t.agent_id,t.member_id,t.ledger_entry_id AS ledger_id,t.audit_log_id,
 'paid'::text AS kind,'commission'::text AS entry_type,'commission_payment_target'::text AS reference_type,
 'commission-payment:'||t.id::text AS operation_key,t.points::numeric AS delta,p.evidence_epoch,
 'system'::text AS actor_type,NULL::uuid AS actor_id,
 (t.state='paid' AND t.paid_at IS NOT NULL AND p.id IS NOT NULL AND e.id IS NOT NULL
 AND (e.brand_id,e.cycle_id,e.run_id,e.agent_id,e.member_id,e.points)=(t.brand_id,p.cycle_id,p.run_id,t.agent_id,t.member_id,t.points)
 AND EXISTS(SELECT 1 FROM commission_adjustment_heads h WHERE h.brand_id=$1 AND h.target_id=t.id
 AND (h.version=1 AND h.points=t.points AND h.last_adjustment_id IS NULL OR h.version>1 AND EXISTS(
 SELECT 1 FROM commission_adjustments last WHERE last.brand_id=$1 AND last.target_id=t.id AND last.id=h.last_adjustment_id AND last.version=h.version AND last.points_after=h.points AND last.ledger_entry_id IS NOT NULL)))
 AND EXISTS(SELECT 1 FROM commission_cycles z WHERE z.brand_id=$1 AND z.id=p.cycle_id)
 AND a.brand_id=$1 AND a.actor_type='system' AND a.actor_id IS NULL AND a.action='commission.payment.target'
 AND a.resource_type='commission_payment_target' AND a.resource_id=t.id
 AND a.after_json->>'state'='paid' AND a.after_json->>'points'=t.points::text
 AND (a.after_json->>'ledger_entry_id') IS NOT DISTINCT FROM t.ledger_entry_id::text
 AND a.after_json->>'earning_id'=t.earning_id::text AND a.after_json->>'payment_id'=p.id::text
 AND p.approval_audit_log_id IS NOT NULL AND aa.brand_id=$1 AND aa.resource_type='commission_payment' AND aa.resource_id=p.id
 AND ((p.approval_actor_type='system' AND p.approved_by IS NULL AND aa.actor_type='system' AND aa.actor_id IS NULL
 AND aa.action='commission.payment.create' AND aa.id=p.creation_audit_log_id)
 OR (p.approval_actor_type='admin' AND aa.actor_type='admin' AND aa.actor_id=p.approved_by AND aa.action='commission.payment.approve'))
 ) AS source_valid
 FROM commission_payment_targets t LEFT JOIN commission_payments p ON p.brand_id=t.brand_id AND p.id=t.payment_id
 LEFT JOIN commission_earnings e ON e.id=t.earning_id LEFT JOIN audit_logs a ON a.id=t.audit_log_id LEFT JOIN audit_logs aa ON aa.id=p.approval_audit_log_id
 WHERE t.brand_id=$1 AND (t.state='paid' OR t.ledger_entry_id IS NOT NULL)
 UNION ALL
 SELECT x.id,p.cycle_id,t.agent_id,t.member_id,x.ledger_entry_id,x.audit_log_id,'adjustment','commission_adjustment','commission_adjustment',
 'commission-adjustment:'||x.id::text,x.delta_points::numeric,p.evidence_epoch,'admin',x.created_by,
 (t.id IS NOT NULL AND t.state='paid' AND p.id IS NOT NULL AND t.payment_id=x.payment_id
 AND EXISTS(SELECT 1 FROM commission_cycles z WHERE z.brand_id=$1 AND z.id=p.cycle_id)
 AND x.points_after::numeric-x.points_before::numeric=x.delta_points::numeric
 AND x.points_before IS NOT DISTINCT FROM CASE WHEN x.version=2 THEN t.points ELSE
 (SELECT prev.points_after FROM commission_adjustments prev WHERE prev.brand_id=$1 AND prev.target_id=x.target_id AND prev.version=x.version-1) END
 AND a.brand_id=$1 AND a.actor_type='admin' AND a.actor_id=x.created_by AND a.action='commission.adjustment.create'
 AND a.resource_type='commission_adjustment' AND a.resource_id=x.id AND a.reason=x.reason
 AND a.before_json->>'version'=(x.version-1)::text AND a.before_json->>'points'=x.points_before::text
 AND a.after_json->>'version'=x.version::text AND a.after_json->>'points'=x.points_after::text
 AND a.after_json->>'delta_points'=x.delta_points::text AND a.after_json->>'target_id'=x.target_id::text AND a.after_json->>'payment_id'=x.payment_id::text)
 FROM commission_adjustments x LEFT JOIN commission_payment_targets t ON t.brand_id=x.brand_id AND t.id=x.target_id
 LEFT JOIN commission_payments p ON p.brand_id=x.brand_id AND p.id=x.payment_id LEFT JOIN audit_logs a ON a.id=x.audit_log_id WHERE x.brand_id=$1
 UNION ALL
 SELECT t.id,x.cycle_id,t.agent_id,t.member_id,t.ledger_entry_id,t.audit_log_id,'correction','commission_correction','commission_correction_target',
 'commission-correction:'||t.id::text,t.delta_points::numeric,x.evidence_epoch,'system',NULL::uuid,
 (t.state='applied' AND t.applied_at IS NOT NULL AND x.id IS NOT NULL AND x.cycle_id=t.cycle_id
 AND EXISTS(SELECT 1 FROM commission_cycles z WHERE z.brand_id=$1 AND z.id=x.cycle_id)
 AND pt.id IS NOT NULL AND (pt.plan_id,pt.agent_id,pt.member_id,pt.points_before,pt.points_after,pt.delta_points)=(x.plan_id,t.agent_id,t.member_id,t.points_before,t.points_after,t.delta_points)
 AND t.points_after::numeric-t.points_before::numeric=t.delta_points::numeric
 AND s.id IS NOT NULL AND s.operation='apply' AND s.to_state='applying' AND s.target_id=t.id
 AND a.brand_id=$1 AND a.actor_type='system' AND a.actor_id IS NULL AND a.action='commission.correction_execution.target'
 AND a.resource_type='commission_correction_target' AND a.resource_id=t.id AND a.request_id=s.request_id
 AND a.before_json=jsonb_build_object('state','pending')
 AND a.after_json=jsonb_build_object('state','applied','execution_id',t.execution_id,'plan_target_id',t.plan_target_id,
 'points_before',t.points_before::text,'points_after',t.points_after::text,'delta_points',t.delta_points::text,'ledger_entry_id',t.ledger_entry_id,'financial_version',t.financial_version)
 AND aa.brand_id=$1 AND aa.resource_type='commission_correction_execution' AND aa.resource_id=x.id
 AND ((x.approval_actor_type='system' AND x.approved_by IS NULL AND aa.actor_type='system' AND aa.actor_id IS NULL AND aa.action='commission.correction_execution.create' AND aa.id=x.creation_audit_log_id)
 OR (x.approval_actor_type='admin' AND aa.actor_type='admin' AND aa.actor_id=x.approved_by AND aa.action='commission.correction_execution.approve')))
 FROM commission_correction_execution_targets t LEFT JOIN commission_correction_executions x ON x.brand_id=t.brand_id AND x.id=t.execution_id
 LEFT JOIN commission_correction_plan_targets pt ON pt.brand_id=t.brand_id AND pt.id=t.plan_target_id
 LEFT JOIN commission_correction_execution_steps s ON s.brand_id=t.brand_id AND s.execution_id=t.execution_id AND s.version=t.execution_version+1 AND s.operation='apply' AND s.target_id=t.id
 LEFT JOIN audit_logs a ON a.id=t.audit_log_id LEFT JOIN audit_logs aa ON aa.id=x.approval_audit_log_id
 WHERE t.brand_id=$1 AND (t.state='applied' OR t.ledger_entry_id IS NOT NULL)),
 relevant AS MATERIALIZED (
 SELECT e.*,
 coalesce(e.source_valid AND n.id IS NOT NULL AND n.member_id=e.member_id AND pa.id IS NOT NULL AND
 CASE WHEN e.delta=0 THEN e.ledger_id IS NULL
 ELSE l.id IS NOT NULL AND l.brand_id=$1 AND l.account_id=pa.id AND l.member_id=e.member_id
 AND l.entry_type=e.entry_type AND l.reference_type=e.reference_type AND l.reference_id=e.source_id AND l.operation_key=e.operation_key
 AND l.actor_type=e.actor_type AND l.actor_id IS NOT DISTINCT FROM e.actor_id AND l.reversal_of IS NULL
 AND l.request_id=a.request_id
 AND normalize_point_snapshot(l.delta_snapshot)=jsonb_set(point_zero_snapshot(),'{commission,available}',to_jsonb(e.delta::text))
 AND l.source_allocation=jsonb_build_array(jsonb_build_object('source','commission','state','available','points',abs(e.delta)::text))
 AND lottery_business_ledger_applied(l,$1,pa.id) END,false) AS valid
 FROM raw_edges e LEFT JOIN point_ledger_entries l ON l.id=e.ledger_id
 LEFT JOIN point_accounts pa ON pa.brand_id=$1 AND pa.brand_member_id=e.member_id
 LEFT JOIN agent_nodes n ON n.brand_id=$1 AND n.id=e.agent_id LEFT JOIN audit_logs a ON a.id=e.audit_log_id
 WHERE e.cycle_id IS NULL OR NOT EXISTS(SELECT 1 FROM commission_cycles c WHERE c.brand_id=$1 AND c.id=e.cycle_id) OR EXISTS(SELECT 1 FROM cohort c WHERE c.id=e.cycle_id)),
 duplicate_ledgers AS (
 SELECT ledger_id FROM raw_edges WHERE ledger_id IS NOT NULL GROUP BY ledger_id HAVING count(*)<>1),
 attributed_ledgers AS MATERIALIZED (
 SELECT e.ledger_id AS id FROM raw_edges e JOIN commission_cycles c ON c.brand_id=$1 AND c.id=e.cycle_id WHERE e.ledger_id IS NOT NULL
 UNION SELECT l.id FROM raw_edges e JOIN commission_cycles c ON c.brand_id=$1 AND c.id=e.cycle_id
 JOIN point_ledger_entries l ON l.brand_id=$1 AND l.reference_type=e.reference_type AND l.reference_id=e.source_id),
 integrity AS (
 SELECT NOT EXISTS(SELECT 1 FROM cohort WHERE current_evidence AND NOT ready)
 AND NOT EXISTS(SELECT 1 FROM relevant WHERE NOT valid OR cycle_id IS NULL)
 AND NOT EXISTS(SELECT 1 FROM relevant e WHERE e.kind='adjustment' AND NOT EXISTS(SELECT 1 FROM relevant original WHERE original.kind='paid' AND original.agent_id=e.agent_id AND original.member_id=e.member_id AND original.cycle_id=e.cycle_id))
 AND NOT EXISTS(SELECT 1 FROM relevant e JOIN duplicate_ledgers dup ON dup.ledger_id=e.ledger_id)
 AND NOT EXISTS(SELECT 1 FROM raw_edges e JOIN point_ledger_entries l ON l.brand_id=$1 AND l.reference_type=e.reference_type AND l.reference_id=e.source_id
 WHERE l.id IS DISTINCT FROM e.ledger_id)
 AND NOT EXISTS(SELECT 1 FROM point_ledger_entries l WHERE l.brand_id=$1 AND
 (l.entry_type IN('commission','commission_adjustment','commission_correction') OR l.reference_type IN('commission_payment_target','commission_adjustment','commission_correction_target'))
 AND NOT EXISTS(SELECT 1 FROM attributed_ledgers e WHERE e.id=l.id)) AS valid),
 coverage AS (
 SELECT jsonb_build_object('selected_cycle_count',count(*)::text,'ready_cycle_count',(count(*) FILTER(WHERE ready))::text,
 'unready_cycle_count',(count(*) FILTER(WHERE NOT ready))::text,
 'legacy_policy_blocked_cycle_count',(count(*) FILTER(WHERE EXISTS(SELECT 1 FROM commission_correction_plans p WHERE p.brand_id=$1 AND p.cycle_id=cohort.id AND p.state='blocked' AND p.last_error_code='COMMISSION_CORRECTION_MANUAL_POLICY_UNRESOLVED')))::text) AS data FROM cohort),
 identities AS (
 SELECT c.id AS cycle_id,e.agent_id,e.member_id FROM cohort c JOIN commission_earnings e ON e.brand_id=$1 AND e.cycle_id=c.id AND e.run_id=c.current_run_id
 UNION SELECT cycle_id,agent_id,member_id FROM relevant WHERE cycle_id IS NOT NULL
 UNION SELECT c.id,a.agent_id,a.member_id FROM cohort c JOIN commission_allocations a ON a.brand_id=$1 AND a.cycle_id=c.id AND a.run_id=c.current_run_id),
 chosen_identities AS (
 SELECT * FROM identities WHERE ($4::uuid IS NULL OR agent_id=$4) AND ($5::uuid IS NULL OR member_id=$5)),
 units AS (
 SELECT * FROM chosen_identities
 UNION ALL SELECT c.id,NULL::uuid,NULL::uuid FROM cohort c WHERE '` + group + `'='cycle' AND NOT EXISTS(SELECT 1 FROM chosen_identities i WHERE i.cycle_id=c.id)),
 finances AS (
 SELECT e.cycle_id,e.agent_id,e.member_id,
 count(*) FILTER(WHERE kind='paid' AND delta<>0) AS paid_count,coalesce(sum(delta) FILTER(WHERE kind='paid'),0) AS paid,
 count(*) FILTER(WHERE kind='adjustment' AND delta<>0) AS adj_count,coalesce(sum(greatest(delta,0)) FILTER(WHERE kind='adjustment'),0) AS adj_credit,
 coalesce(sum(greatest(-delta,0)) FILTER(WHERE kind='adjustment'),0) AS adj_debit,
 count(*) FILTER(WHERE kind='correction' AND delta<>0) AS corr_count,coalesce(sum(greatest(delta,0)) FILTER(WHERE kind='correction'),0) AS corr_credit,
 coalesce(sum(greatest(-delta,0)) FILTER(WHERE kind='correction'),0) AS corr_debit,
 coalesce(sum(delta) FILTER(WHERE kind='adjustment' AND e.evidence_epoch=c.evidence_epoch),0) AS effective_adjustment
 FROM relevant e JOIN cohort c ON c.id=e.cycle_id GROUP BY e.cycle_id,e.agent_id,e.member_id),
 base AS (
 SELECT u.*,c.ready AS complete,CASE WHEN c.ready THEN coalesce(e.points::numeric,0) ELSE 0 END AS calculated,
 CASE WHEN c.ready THEN coalesce(e.points::numeric,0)+coalesce(f.effective_adjustment,0) ELSE 0 END AS effective,
 coalesce(f.paid_count,0) AS paid_count,coalesce(f.paid,0) AS paid,coalesce(f.adj_count,0) AS adj_count,
 coalesce(f.adj_credit,0) AS adj_credit,coalesce(f.adj_debit,0) AS adj_debit,coalesce(f.corr_count,0) AS corr_count,
 coalesce(f.corr_credit,0) AS corr_credit,coalesce(f.corr_debit,0) AS corr_debit
 FROM units u JOIN cohort c ON c.id=u.cycle_id
 LEFT JOIN commission_earnings e ON e.brand_id=$1 AND e.cycle_id=c.id AND e.run_id=c.current_run_id AND e.agent_id=u.agent_id AND e.member_id=u.member_id
 LEFT JOIN finances f ON f.cycle_id=u.cycle_id AND f.agent_id=u.agent_id AND f.member_id=u.member_id),
 grouped AS (SELECT ` + key + ` AS key,` + key + ` AS label,` + commissionAnalysisTotalsSQL(complete) + ` AS totals FROM base GROUP BY ` + key + `),
 page AS (SELECT * FROM grouped ORDER BY key COLLATE "C" LIMIT $7 OFFSET $8)
 SELECT statement_timestamp(),scope.timezone,validity.valid,
 integrity.valid AND NOT EXISTS(SELECT 1 FROM base WHERE effective<0),coverage.data,
 (SELECT ` + commissionAnalysisTotalsSQL(summaryComplete) + ` FROM base),
 coalesce((SELECT jsonb_agg(to_jsonb(page) ORDER BY key COLLATE "C") FROM page),'[]'::jsonb),
 (SELECT count(*)::text FROM grouped)
 FROM scope CROSS JOIN validity CROSS JOIN integrity CROSS JOIN coverage`
}
