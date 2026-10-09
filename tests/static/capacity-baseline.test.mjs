import { test } from 'node:test';
import assert from 'node:assert/strict';
import { checkCapacityBaseline } from '../../scripts/check-capacity-baseline.mjs';

function report() {
  return {
    schema_version: 1, users: 500, brands: 2, plan: { rate: 500 }, scheduled_seconds: 30,
    duplicate_group: 1, cutoff_seconds_from_fixture_creation: 0, failed_or_dropped: 0,
    report: { planned: 15000, started: 15000, completed: 15000, succeeded: 15000, dropped: 0, backpressure_dropped: 0, status_counts: { 201: 15000 }, error_counts: {}, end_to_end: { p95_ms: 10, p99_ms: 20 } },
    integrity: { orders: 15000, debits: 15000, stake_points: 15000, debit_points: 15000, balance_points: 49985000, known_unique_receipts: 15000, bad_order_links: 0, bad_account_balances: 0, late_orders: 0, unknown_committed_orders: 0 },
    notification_worker_enabled: true, notification_worker_errors: 0, sample_errors: 0, database_delta: { deadlocks: 0 }, production_capacity_accepted: false, replica_and_redis_capacity_verified: false, new_order_tps_over_wall: 499.9,
  };
}

test('controlled baseline records success without inventing a latency or production SLO', () => {
  const data = report();
  data.report.end_to_end.p99_ms = 300;
  assert.deepEqual(checkCapacityBaseline(data), { controlled_baseline_passed: true, production_capacity_accepted: false, users: 500, brands: 2, seconds: 30, orders: 15000, new_order_tps: 499.9, p95_ms: 10, p99_ms: 300 });
});
test('failed or dropped requests cannot pass merely because the remaining ledger balances', () => {
  const data = report();
  data.report.succeeded = 14999;
  assert.throws(() => checkCapacityBaseline(data), /failed, dropped/);
  const dropped = report();
  dropped.report.dropped = 1;
  assert.throws(() => checkCapacityBaseline(dropped), /failed, dropped/);
});
test('unacknowledged commits and balance differences fail the financial check', () => {
  const data = report();
  data.integrity.unknown_committed_orders = 1;
  assert.throws(() => checkCapacityBaseline(data), /financial integrity/);
  data.integrity.unknown_committed_orders = 0;
  data.integrity.balance_points--;
  assert.throws(() => checkCapacityBaseline(data), /financial integrity/);
});
test('replay and cutoff profiles require their own evidence, not this baseline gate', () => {
  const data = report();
  data.duplicate_group = 2;
  assert.throws(() => checkCapacityBaseline(data), /unique-order baseline/);
  data.duplicate_group = 1;
  data.cutoff_seconds_from_fixture_creation = 5;
  assert.throws(() => checkCapacityBaseline(data), /unique-order baseline/);
});
test('worker failure and unsupported production claims remain explicit failures', () => {
  const data = report();
  data.notification_worker_errors = 1;
  assert.throws(() => checkCapacityBaseline(data), /worker, sampling/);
  data.notification_worker_errors = 0;
  data.production_capacity_accepted = true;
  assert.throws(() => checkCapacityBaseline(data), /must not claim production/);
});
