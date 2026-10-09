import { readFileSync } from 'node:fs';
import assert from 'node:assert/strict';
import test from 'node:test';
import { checkCapacityCommission } from '../../scripts/check-capacity-commission.mjs';

const report = () => JSON.parse(readFileSync(new URL('../../docs/performance/current-betting-settlement-withdrawals-commission-500-20261010.json', import.meta.url), 'utf8'));

test('actual archived commission combination passes with nonproduction scope', () => {
  const result = checkCapacityCommission(report());
  assert.equal(result.controlled_commission_load_passed, true);
  assert.equal(result.production_capacity_accepted, false);
});
test('commission must be credited and cycles completed during the foreground load', () => {
  for (const field of ['commission_ready_cycles', 'commission_paid_payments', 'commission_entries', 'commission_points', 'commission_credits_during_load', 'commission_cycle_targets']) {
    const data = report(); data[field]--;
    assert.throws(() => checkCapacityCommission(data), /Commission/);
  }
});
test('commission rejects broken linkage and window attribution', () => {
  for (const field of ['commission_bad_links', 'commission_out_of_window_orders']) {
    const data = report(); data[field]++;
    assert.throws(() => checkCapacityCommission(data), /Commission/);
  }
  const data = report(); data.commission_boundary_utc = data.load_ended_at_utc;
  assert.throws(() => checkCapacityCommission(data), /Commission/);
});
test('commission combination still checks withdrawals and total wallet balance', () => {
  const data = report(); data.withdrawal_bad_links++;
  assert.throws(() => checkCapacityCommission(data), /withdrawal/);
  const badBalance = report(); badBalance.integrity.balance_points++;
  assert.throws(() => checkCapacityCommission(badBalance), /financial/);
});
