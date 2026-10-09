import { readFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';

// Checks a successful, unique-order 500-user/500-rps controlled baseline.
// Replay and cutoff profiles have different expected outcomes and are not accepted here.
export function checkCapacityBaseline(data) {
  const r = data?.report;
  const i = data?.integrity;
  const planned = 500 * data?.scheduled_seconds;
  if (data?.schema_version !== 1 || data?.users !== 500 || data?.plan?.rate !== 500 ||
      data?.duplicate_group !== 1 || data?.cutoff_seconds_from_fixture_creation !== 0 ||
      !Number.isSafeInteger(planned) || planned <= 0 || !r || !i) {
    throw new Error('Expected a current 500-user, 500-rps unique-order baseline report');
  }
  if ([r.planned, r.started, r.completed, r.succeeded, r.status_counts?.['201']].some(n => n !== planned) ||
      r.dropped !== 0 || r.backpressure_dropped !== 0 || data.failed_or_dropped !== 0 ||
      Object.keys(r.error_counts).length !== 0 || Object.keys(r.status_counts).length !== 1 ||
      data.run_error || data.integrity_error) {
    throw new Error('Baseline contains failed, dropped, incomplete or unexpected requests');
  }
  if ([i.orders, i.debits, i.stake_points, i.debit_points, i.known_unique_receipts].some(n => n !== planned) ||
      i.balance_points !== 500 * 100000 - planned ||
      [i.bad_order_links, i.bad_account_balances, i.late_orders, i.unknown_committed_orders].some(n => n !== 0)) {
    throw new Error('Baseline financial integrity does not match successful unique orders');
  }
  if (data.notification_worker_enabled !== true || data.notification_worker_errors !== 0 ||
      data.sample_errors !== 0 || data.database_delta?.deadlocks !== 0) {
    throw new Error('Baseline has worker, sampling or database failures');
  }
  if (data.production_capacity_accepted !== false || data.replica_and_redis_capacity_verified !== false) {
    throw new Error('A controlled baseline must not claim production or replica capacity acceptance');
  }
  return {
    controlled_baseline_passed: true,
    production_capacity_accepted: false,
    users: data.users,
    brands: data.brands,
    seconds: data.scheduled_seconds,
    orders: i.orders,
    new_order_tps: data.new_order_tps_over_wall,
    p95_ms: r.end_to_end.p95_ms,
    p99_ms: r.end_to_end.p99_ms,
  };
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    if (process.argv.length !== 3) throw new Error('Usage: node scripts/check-capacity-baseline.mjs REPORT.json');
    console.log(JSON.stringify(checkCapacityBaseline(JSON.parse(readFileSync(process.argv[2], 'utf8'))), null, 2));
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  }
}
