import { readFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';

export function checkCapacitySettlement(data) {
  return checkCapacityFinancialCore(data, 'betting_with_settlement', 49989500);
}

export function checkCapacityFinancialCore(data, profile, balance) {
  const r = data?.report;
  const i = data?.integrity;
  if (data?.schema_version !== 1 || data.profile !== profile || data.users !== 500 || data.brands !== 2 ||
      data.pool_max_connections !== 20 || data.settlement_workers !== 2 || data.scheduled_seconds !== 30 ||
      data.plan?.rate !== 500 || data.plan?.duration !== 30_000_000_000 || data.plan?.max_in_flight !== 500 || !r || !i) {
    throw new Error('Expected the controlled 500-user betting and settlement profile');
  }
  if ([r.planned, r.started, r.completed, r.succeeded, r.status_counts?.['201']].some(n => n !== 15000) ||
      r.dropped !== 0 || r.backpressure_dropped !== 0 || Object.keys(r.status_counts).length !== 1 ||
      Object.keys(r.error_counts).length !== 0 || Object.keys(r.code_counts).length !== 1 || r.code_counts[''] !== 15000 ||
      data.request_checks_passed !== true) throw new Error('Mixed load has unsuccessful or incomplete requests');
  if ([i.orders, i.debits, i.stake_points, i.debit_points, i.known_unique_receipts].some(n => n !== 15500) ||
      i.balance_points !== balance || [i.bad_order_links, i.bad_account_balances, i.late_orders, i.unknown_committed_orders].some(n => n !== 0) ||
      data.background_orders !== 500 || data.background_completed_jobs !== 2 || data.settled_periods !== 2 ||
      [data.paid_targets, data.prize_entries, data.prize_credits_during_load, data.won_orders].some(n => n !== 500) ||
      data.prize_points !== 5000 || data.bad_prize_links !== 0 || data.financial_checks_passed !== true) {
    throw new Error('Mixed load financial or actual overlap checks failed');
  }
  if (data.settlement_worker_errors !== 0 || data.notification_worker_errors !== 0 || data.worker_checks_passed !== true ||
      data.database_delta?.deadlocks !== 0 || data.database_delta?.counterResetDetected !== false) throw new Error('Mixed load worker or database failure');
  if (data.production_capacity_accepted !== false || data.replica_and_redis_capacity_verified !== false) throw new Error('Controlled mixed load cannot claim production acceptance');
  if (!Number.isFinite(r.end_to_end?.p95_ms) || !Number.isFinite(r.end_to_end?.p99_ms)) throw new Error('Missing latency summary');
  return { controlled_settlement_load_passed: true, production_capacity_accepted: false, new_bets: 15000, background_prizes: 500, prize_points: 5000, balance_points: i.balance_points, p95_ms: r.end_to_end.p95_ms, p99_ms: r.end_to_end.p99_ms };
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    if (process.argv.length !== 3) throw new Error('Usage: node scripts/check-capacity-settlement.mjs REPORT.json');
    console.log(JSON.stringify(checkCapacitySettlement(JSON.parse(readFileSync(process.argv[2], 'utf8'))), null, 2));
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  }
}
