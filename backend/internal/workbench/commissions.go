package workbench

// Each source returns one aggregate row. Never join per-agent payment,
// allocation or correction targets to task rows: that would multiply counts.
// Current-run validity is an inventory distinction, not financial approval.
const commissionTaskQuery = `SELECT jsonb_build_object(
 'discovery_pending_count',d.pending,'discovery_failed_count',d.failed,
 'cycle_processing_count',c.processing,'cycle_waiting_count',c.waiting,
 'cycle_ready_count',c.ready,'cycle_stale_count',c.stale,'cycle_failed_count',c.failed,
 'payment_awaiting_approval_count',p.awaiting,'payment_processing_count',p.processing,
 'payment_blocked_count',p.blocked,'payment_failed_count',p.failed,
 'plan_processing_count',a.processing,'plan_ready_count',a.ready,'plan_blocked_count',a.blocked,'plan_failed_count',a.failed,
 'execution_awaiting_approval_count',x.awaiting,'execution_processing_count',x.processing,
 'execution_paused_count',x.paused,'execution_failed_count',x.failed)
 FROM (
 SELECT count(*) FILTER(WHERE state='pending')::text pending,
 count(*) FILTER(WHERE state='failed')::text failed FROM commission_discovery WHERE brand_id=s.id
 ) d CROSS JOIN (
 SELECT count(*) FILTER(WHERE c.state IN('enumerating','calculating','summarizing'))::text processing,
 count(*) FILTER(WHERE c.state='waiting')::text waiting,
 count(*) FILTER(WHERE c.state='ready' AND coalesce(r.state='ready' AND r.evidence_epoch=c.evidence_epoch,false))::text ready,
 count(*) FILTER(WHERE c.state='ready' AND NOT coalesce(r.state='ready' AND r.evidence_epoch=c.evidence_epoch,false))::text stale,
 count(*) FILTER(WHERE c.state='failed')::text failed
 FROM commission_cycles c LEFT JOIN commission_runs r ON r.brand_id=c.brand_id AND r.cycle_id=c.id AND r.id=c.current_run_id
 WHERE c.brand_id=s.id
 ) c CROSS JOIN (
 SELECT count(*) FILTER(WHERE state='awaiting_approval')::text awaiting,
 count(*) FILTER(WHERE state='paying')::text processing,
 count(*) FILTER(WHERE state='blocked')::text blocked,
 count(*) FILTER(WHERE state='failed')::text failed FROM commission_payments WHERE brand_id=s.id
 ) p CROSS JOIN (
 SELECT count(*) FILTER(WHERE state='planning')::text processing,
 count(*) FILTER(WHERE state='ready')::text ready,
 count(*) FILTER(WHERE state='blocked')::text blocked,
 count(*) FILTER(WHERE state='failed')::text failed FROM commission_correction_plans WHERE brand_id=s.id
 ) a CROSS JOIN (
 SELECT count(*) FILTER(WHERE state='awaiting_approval')::text awaiting,
 count(*) FILTER(WHERE state='applying')::text processing,
 count(*) FILTER(WHERE state='paused')::text paused,
 count(*) FILTER(WHERE state='failed')::text failed FROM commission_correction_executions WHERE brand_id=s.id
 ) x`
