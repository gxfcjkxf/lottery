import { readFileSync } from 'node:fs';
import assert from 'node:assert/strict';
import test from 'node:test';
import { checkCapacityWithdrawals } from '../../scripts/check-capacity-withdrawals.mjs';

const report = () => JSON.parse(readFileSync(new URL('../../docs/performance/current-betting-settlement-withdrawals-500-20261010.json', import.meta.url), 'utf8'));

test('actual archived withdrawal combination passes with nonproduction scope', () => {
  const result = checkCapacityWithdrawals(report());
  assert.equal(result.controlled_withdrawal_load_passed, true);
  assert.equal(result.production_capacity_accepted, false);
});
test('uncompleted applications or actions cannot count as successful withdrawal closure', () => {
  for (const field of ['started', 'completed', 'succeeded']) {
    const data = report(); data.withdrawal_report[field]--;
    assert.throws(() => checkCapacityWithdrawals(data), /withdrawal counts/);
  }
});
test('withdrawal credits must be linked and complete during the foreground load', () => {
  for (const field of ['withdrawal_paid_orders', 'withdrawal_paid_during_load', 'withdrawal_points']) {
    const data = report(); data[field]--;
    assert.throws(() => checkCapacityWithdrawals(data), /withdrawal counts/);
  }
  for (const field of ['withdrawal_pending_points', 'withdrawal_bad_links', 'withdrawal_bad_cycles']) {
    const data = report(); data[field]++;
    assert.throws(() => checkCapacityWithdrawals(data), /withdrawal counts/);
  }
});
test('withdrawal combination still rejects wallet differences and worker failures', () => {
  const data = report(); data.integrity.balance_points++;
  assert.throws(() => checkCapacityWithdrawals(data), /financial/);
  const failed = report(); failed.settlement_worker_errors++;
  assert.throws(() => checkCapacityWithdrawals(failed), /worker/);
});
