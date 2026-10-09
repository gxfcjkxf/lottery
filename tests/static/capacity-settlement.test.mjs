import { readFileSync } from 'node:fs';
import assert from 'node:assert/strict';
import test from 'node:test';
import { checkCapacitySettlement } from '../../scripts/check-capacity-settlement.mjs';

const report = () => JSON.parse(readFileSync(new URL('../../docs/performance/current-betting-settlement-500-20261010.json', import.meta.url), 'utf8'));

test('actual committed betting and settlement report passes without production claims', () => {
  assert.equal(checkCapacitySettlement(report()).controlled_settlement_load_passed, true);
});
test('missing or failed betting requests cannot pass with balanced remaining wallets', () => {
  for (const field of ['started', 'completed', 'succeeded']) {
    const data = report(); data.report[field]--;
    assert.throws(() => checkCapacitySettlement(data), /unsuccessful or incomplete/);
  }
});
test('prizes must really occur during the load with correct links and wallet totals', () => {
  for (const field of ['prize_entries', 'prize_credits_during_load', 'won_orders', 'background_completed_jobs', 'settled_periods']) {
    const data = report(); data[field]--;
    assert.throws(() => checkCapacitySettlement(data), /financial or actual overlap/);
  }
  for (const field of ['balance_points', 'unknown_committed_orders', 'bad_account_balances']) {
    const data = report(); data.integrity[field]++;
    assert.throws(() => checkCapacitySettlement(data), /financial or actual overlap/);
  }
});
test('database and worker failures or production claims cannot be accepted', () => {
  for (const field of ['settlement_worker_errors', 'notification_worker_errors']) {
    const data = report(); data[field]++;
    assert.throws(() => checkCapacitySettlement(data), /worker or database/);
  }
  const data = report(); data.production_capacity_accepted = true;
  assert.throws(() => checkCapacitySettlement(data), /production acceptance/);
});
