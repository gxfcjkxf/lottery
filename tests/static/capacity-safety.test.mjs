import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { checkCapacitySafety } from '../../scripts/check-capacity-safety.mjs';

test('committed current reports pass their explicit safety profiles', () => {
  for (const profile of ['replay', 'cutoff']) {
    const data = JSON.parse(readFileSync(new URL(`../../docs/performance/current-${profile}-500-20261010.json`, import.meta.url), 'utf8'));
    assert.equal(checkCapacitySafety(data, profile).controlled_safety_passed, true);
  }
});

function artifact(profile) {
  const seconds = 30;
  const planned = 500 * seconds;
  const replay = profile === 'replay';
  const users = replay ? 1 : 500;
  const duplicate = replay ? 10 : 1;
  const succeeded = replay ? planned : planned - 120;
  const rejected = replay ? 0 : 120;
  const orders = replay ? Math.ceil(planned / duplicate) : succeeded;
  return {
    schema_version: 1, users, brands: replay ? 1 : 2, plan: { rate: 500, duration: seconds * 1_000_000_000 }, scheduled_seconds: seconds,
    duplicate_group: duplicate, cutoff_seconds_from_fixture_creation: replay ? 0 : 10,
    failed_or_dropped: rejected,
    report: {
      planned, started: planned, completed: planned, succeeded, dropped: 0, backpressure_dropped: 0,
      status_counts: replay ? { 201: planned } : { 201: succeeded, 409: rejected },
      code_counts: replay ? { '': planned } : { '': succeeded, BET_PERIOD_CLOSED: rejected },
      error_counts: replay ? {} : { callback_error: rejected, http_status: rejected, response_code: rejected, missing_order_id: rejected },
      end_to_end: { p95_ms: 11, p99_ms: 24 },
    },
    integrity: {
      orders, debits: orders, stake_points: orders, debit_points: orders,
      balance_points: users * 100000 - orders, known_unique_receipts: orders,
      bad_order_links: 0, bad_account_balances: 0, late_orders: 0, unknown_committed_orders: 0,
    },
    notification_worker_enabled: true, notification_worker_errors: 0, sample_errors: 0,
    database_delta: { deadlocks: 0 }, production_capacity_accepted: false,
    replica_and_redis_capacity_verified: false,
  };
}

for (const profile of ['replay', 'cutoff']) {
  test(`${profile} artifact passes with its profile-specific counts`, () => {
    const data = artifact(profile);
    assert.equal(checkCapacitySafety(data, profile).controlled_safety_passed, true);
  });
}

test('rejects invalid profile, malformed counts, extra statuses and error codes', () => {
  assert.throws(() => checkCapacitySafety(artifact('replay'), 'baseline'), /Profile/);
  for (const mutate of [
    (d) => { d.report.completed--; },
    (d) => { d.report.status_counts['500'] = 1; },
    (d) => { d.report.code_counts.UNEXPECTED = 1; },
    (d) => { d.report.error_counts.unexpected = 1; },
    (d) => { d.report.succeeded--; },
  ]) {
    const d = artifact('replay'); mutate(d);
    assert.throws(() => checkCapacitySafety(d, 'replay'));
  }
});

test('rejects unknown commits, late orders and balance mismatches', () => {
  for (const field of ['unknown_committed_orders', 'late_orders', 'bad_account_balances']) {
    const d = artifact('cutoff'); d.integrity[field] = 1;
    assert.throws(() => checkCapacitySafety(d, 'cutoff'), /Financial integrity/);
  }
  const d = artifact('cutoff'); d.integrity.balance_points--;
  assert.throws(() => checkCapacitySafety(d, 'cutoff'), /Financial integrity/);
});

test('requires enabled healthy worker, healthy sampling and explicit nonproduction claims', () => {
  for (const [field, value] of [
    ['notification_worker_enabled', false], ['notification_worker_errors', 1], ['sample_errors', 1],
    ['production_capacity_accepted', true], ['replica_and_redis_capacity_verified', true],
  ]) {
    const d = artifact('replay'); d[field] = value;
    assert.throws(() => checkCapacitySafety(d, 'replay'));
  }
});

test('cutoff profile requires both closed-period and successful requests', () => {
  const d = artifact('cutoff'); d.report.status_counts['409'] = 0;
  assert.throws(() => checkCapacitySafety(d, 'cutoff'));
});
