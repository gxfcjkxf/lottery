import { readFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';

export function checkCapacityMixedModels(data) {
  const r = data?.report, i = data?.integrity;
  if (data?.schema_version !== 1 || data.profile !== 'betting_mixed_models_compound' ||
      data.users !== 500 || data.brands !== 2 || data.pool_max_connections !== 20 ||
      data.scheduled_seconds !== 30 || data.plan?.rate !== 500 || data.plan?.duration !== 30_000_000_000 ||
      data.plan?.max_in_flight !== 500 || !r || !i) throw new Error('Expected the fixed mixed-model compound capacity profile');
  if ([r.planned, r.started, r.completed, r.succeeded, r.status_counts?.['201']].some(n => n !== 15000) ||
      r.dropped !== 0 || r.backpressure_dropped !== 0 || Object.keys(r.status_counts).length !== 1 ||
      Object.keys(r.error_counts).length !== 0 || Object.keys(r.code_counts).length !== 1 || r.code_counts[''] !== 15000 ||
      data.request_checks_passed !== true || data.run_error || data.integrity_error) throw new Error('Incomplete or unsuccessful mixed-model requests');
  if ([i.orders, i.debits, i.known_unique_receipts].some(n => n !== 15000) ||
      i.stake_points !== 1215000 || i.debit_points !== 1215000 || i.balance_points !== 48785000 ||
      [i.bad_order_links, i.bad_account_balances, i.late_orders, i.unknown_committed_orders].some(n => n !== 0) ||
      data.financial_checks_passed !== true) throw new Error('Mixed-model financial integrity failed');
  const expected = [
    { model: 'DIGITS_0_9', combinations: 27, multiplier: 3, expected_points: 81, orders: 5000, stake_points: 405000 },
    { model: 'X_PLUS_Y', combinations: 18, multiplier: 4, expected_points: 72, orders: 5000, stake_points: 360000 },
    { model: 'M_SELECT_N', combinations: 18, multiplier: 5, expected_points: 90, orders: 5000, stake_points: 450000 },
  ];
  if (!Array.isArray(data.cases) || data.cases.length !== 3) throw new Error('Missing three actual model cohorts');
  for (const wanted of expected) {
    const matches = data.cases.filter(row => row.model === wanted.model);
    if (matches.length !== 1 || Object.entries(wanted).some(([key, value]) => matches[0][key] !== value)) throw new Error(`Incorrect model cohort: ${wanted.model}`);
  }
  if (data.notification_worker_enabled !== true || data.notification_worker_errors !== 0 || data.sample_errors !== 0 ||
      data.worker_checks_passed !== true || data.database_delta?.deadlocks !== 0 || data.database_delta?.counterResetDetected !== false) throw new Error('Mixed-model worker or database sampling failure');
  if (data.production_capacity_accepted !== false || data.replica_and_redis_capacity_verified !== false) throw new Error('Controlled mixed-model report cannot claim production capacity');
  if (!Number.isFinite(r.end_to_end?.p95_ms) || !Number.isFinite(r.end_to_end?.p99_ms) || r.end_to_end.p95_ms < 0 || r.end_to_end.p99_ms < r.end_to_end.p95_ms) throw new Error('Missing mixed-model latency evidence');
  return { controlled_mixed_models_passed: true, production_capacity_accepted: false, orders: i.orders, stake_points: i.stake_points, balance_points: i.balance_points, p95_ms: r.end_to_end.p95_ms, p99_ms: r.end_to_end.p99_ms };
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    if (process.argv.length !== 3) throw new Error('Usage: node scripts/check-capacity-mixed-models.mjs REPORT.json');
    console.log(JSON.stringify(checkCapacityMixedModels(JSON.parse(readFileSync(process.argv[2], 'utf8'))), null, 2));
  } catch (error) { console.error(error instanceof Error ? error.message : String(error)); process.exitCode = 1; }
}
