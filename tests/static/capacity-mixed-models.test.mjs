import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { checkCapacityMixedModels } from '../../scripts/check-capacity-mixed-models.mjs';

function report() {
  return {
    schema_version: 1, profile: 'betting_mixed_models_compound', users: 500, brands: 2, pool_max_connections: 20, scheduled_seconds: 30,
    plan: { rate: 500, duration: 30_000_000_000, max_in_flight: 500 },
    report: { planned: 15000, started: 15000, completed: 15000, succeeded: 15000, status_counts: { 201: 15000 }, code_counts: { '': 15000 }, error_counts: {}, dropped: 0, backpressure_dropped: 0, end_to_end: { p95_ms: 12, p99_ms: 24 } },
    integrity: { orders: 15000, debits: 15000, known_unique_receipts: 15000, stake_points: 1215000, debit_points: 1215000, balance_points: 48785000, bad_order_links: 0, bad_account_balances: 0, late_orders: 0, unknown_committed_orders: 0 },
    cases: [
      { model: 'DIGITS_0_9', combinations: 27, multiplier: 3, expected_points: 81, orders: 5000, stake_points: 405000 },
      { model: 'X_PLUS_Y', combinations: 18, multiplier: 4, expected_points: 72, orders: 5000, stake_points: 360000 },
      { model: 'M_SELECT_N', combinations: 18, multiplier: 5, expected_points: 90, orders: 5000, stake_points: 450000 },
    ],
    request_checks_passed: true, financial_checks_passed: true, worker_checks_passed: true,
    notification_worker_enabled: true, notification_worker_errors: 0, sample_errors: 0, database_delta: { deadlocks: 0, counterResetDetected: false },
    production_capacity_accepted: false, replica_and_redis_capacity_verified: false,
  };
}

test('mixed compound profiles preserve amount-based accounting instead of one point per order', () => {
  assert.deepEqual(checkCapacityMixedModels(report()), { controlled_mixed_models_passed: true, production_capacity_accepted: false, orders: 15000, stake_points: 1215000, balance_points: 48785000, p95_ms: 12, p99_ms: 24 });
});
test('correct totals cannot conceal missing, duplicated or mismatched model cohorts', () => {
  for (const mutate of [data => data.cases.pop(), data => { data.cases[1] = { ...data.cases[0] }; }, data => { data.cases[2].orders--; }, data => { data.cases[0].combinations = 1; }, data => { data.cases[1].expected_points = '72'; }]) {
    const data = report(); mutate(data); assert.throws(() => checkCapacityMixedModels(data), /model cohort/);
  }
});
test('partial requests, wrong ledger amounts and unknown commits fail independently', () => {
  for (const mutate of [data => { data.report.completed--; }, data => { data.report.dropped++; }, data => { data.integrity.debit_points--; }, data => { data.integrity.balance_points--; }, data => { data.integrity.unknown_committed_orders++; }]) {
    const data = report(); mutate(data); assert.throws(() => checkCapacityMixedModels(data));
  }
});
test('worker errors, counter resets and missing latencies cannot pass', () => {
  for (const mutate of [data => { data.notification_worker_errors++; }, data => { data.database_delta.counterResetDetected = true; }, data => { delete data.report.end_to_end; }, data => { data.sample_errors++; }]) {
    const data = report(); mutate(data); assert.throws(() => checkCapacityMixedModels(data));
  }
});
test('controlled evidence never accepts production or replication claims', () => {
  const data = report(); data.production_capacity_accepted = true;
  assert.throws(() => checkCapacityMixedModels(data), /cannot claim production/);
});

for (const name of ['current-mixed-models-compound-500-20261010.json', 'current-mixed-models-compound-500-verified-20261010.json']) {
  test(`archived mixed-model report ${name} passes independent financial checks`, () => {
    const data = JSON.parse(readFileSync(new URL(`../../docs/performance/${name}`, import.meta.url), 'utf8'));
    assert.equal(checkCapacityMixedModels(data).orders, 15000);
    assert.ok(data.database_samples.length >= 25);
  });
}
