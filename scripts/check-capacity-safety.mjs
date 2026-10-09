import { readFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';

const fail = (message) => { throw new Error(message); };
const count = (value) => Number.isSafeInteger(value) && value >= 0;
const exactCounts = (actual, expected) => actual && typeof actual === 'object' && !Array.isArray(actual) &&
  Object.keys(actual).length === Object.keys(expected).length &&
  Object.entries(expected).every(([key, value]) => actual[key] === value);

export function checkCapacitySafety(data, profile) {
  if (profile !== 'replay' && profile !== 'cutoff') fail("Profile must be 'replay' or 'cutoff'");
  const r = data?.report;
  const i = data?.integrity;
  const seconds = data?.scheduled_seconds;
  const planned = 500 * seconds;
  const cutoff = data?.cutoff_seconds_from_fixture_creation;
  const users = profile === 'replay' ? 1 : 500;
  if (data?.schema_version !== 1 || data?.users !== users ||
      (profile === 'replay' ? data?.brands !== 1 || data?.duplicate_group <= 1 || cutoff !== 0 :
        ![1, 2].includes(data?.brands) || data?.duplicate_group !== 1 || !Number.isSafeInteger(cutoff) || cutoff <= 0 || cutoff >= seconds) ||
      !Number.isSafeInteger(seconds) || seconds < 1 || seconds > 300 ||
      data?.plan?.rate !== 500 || data?.plan?.duration !== seconds * 1_000_000_000 || !Number.isSafeInteger(data?.duplicate_group) ||
      data.duplicate_group < 1 || data.duplicate_group > 100 || !Number.isSafeInteger(planned) || planned < 1 || planned > 1_000_000 ||
      !r || !i) fail('Report does not match the controlled profile and fixture limits');

  const rejected = profile === 'cutoff' ? r.status_counts?.['409'] : 0;
  const succeeded = profile === 'replay' ? planned : r.status_counts?.['201'];
  if (!count(succeeded) || !count(rejected) || (profile === 'cutoff' && (succeeded === 0 || rejected === 0)) ||
      r.planned !== planned || r.started !== planned || r.completed !== planned || r.succeeded !== succeeded ||
      r.dropped !== 0 || r.backpressure_dropped !== 0 || data.failed_or_dropped !== rejected ||
      !exactCounts(r.status_counts, profile === 'replay' ? { 201: planned } : { 201: succeeded, 409: rejected }) ||
      !exactCounts(r.code_counts, profile === 'replay' ? { '': planned } : { '': succeeded, BET_PERIOD_CLOSED: rejected }) ||
      !exactCounts(r.error_counts, profile === 'replay' ? {} : {
        callback_error: rejected, http_status: rejected, response_code: rejected, missing_order_id: rejected,
      }) || (profile === 'cutoff' && succeeded + rejected !== planned) ||
      'run_error' in data || 'integrity_error' in data) fail('Report contains failed, dropped, incomplete or unexpected requests');

  const orders = profile === 'replay' ? Math.ceil(planned / data.duplicate_group) : succeeded;
  if ([i.orders, i.debits, i.stake_points, i.debit_points, i.known_unique_receipts].some((n) => n !== orders) ||
      i.balance_points !== users * 100000 - orders ||
      [i.bad_order_links, i.bad_account_balances, i.late_orders, i.unknown_committed_orders].some((n) => n !== 0)) {
    fail('Financial integrity does not match the profile order count');
  }
  if (data.notification_worker_enabled !== true || data.notification_worker_errors !== 0 ||
      data.sample_errors !== 0 || data.database_delta?.deadlocks !== 0) fail('Worker, sampling or database safety check failed');
  if (data.production_capacity_accepted !== false || data.replica_and_redis_capacity_verified !== false) {
    fail('Controlled safety reports cannot claim production capacity acceptance');
  }
  if (!Number.isFinite(r.end_to_end?.p95_ms) || !Number.isFinite(r.end_to_end?.p99_ms)) fail('Latency summaries are missing');

  return {
    controlled_safety_passed: true, profile, production_capacity_accepted: false,
    users, brands: data.brands, seconds, planned, succeeded, rejected, orders,
    p95_ms: r.end_to_end.p95_ms, p99_ms: r.end_to_end.p99_ms,
  };
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    if (process.argv.length !== 4) throw new Error('Usage: node scripts/check-capacity-safety.mjs PROFILE REPORT.json');
    console.log(JSON.stringify(checkCapacitySafety(JSON.parse(readFileSync(process.argv[3], 'utf8')), process.argv[2]), null, 2));
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  }
}
