import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import Ajv from 'ajv';
import { schemas } from '../../scripts/openapi-commission-policies.mjs';
const doc = JSON.parse(readFileSync(new URL('../../docs/openapi.json', import.meta.url), 'utf8'));
test('commission policy routes expose exact permission scopes and frozen-write contract', () => {
  const op = doc.paths['/api/v1/admin/commission-policy'];
  assert.deepEqual(op.get['x-permissions'], ['commission_policy.view.brand', 'commission_policy.view.platform']);
  assert.deepEqual(op.put['x-permissions'], ['commission_policy.write.brand']);
  assert.ok(op.put.parameters.some(p => p.name === 'Idempotency-Key' && p.required));
  assert.match(op.put.description, /before cached receipt replay/);
  assert.match(op.put.description, /legacy NULL is not backfilled/);
  assert.ok(!Object.keys(doc.paths).some(p => /commission.*(?:payout|batch)/.test(p)));
  assert.deepEqual(schemas.CommissionPolicyInput.required, ['version', 'config', 'reason']);
});
test('commission calendar contract rejects incomplete and mixed cycle shapes', () => {
  const ajv = new Ajv({ strict: false });
  const validate = ajv.compile(schemas.CommissionCalendar);
  const weekly = { timezone: 'UTC', cycle: 'weekly', boundary_time: '00:00:00', weekday: 1, month_day: null, short_month: '' };
  const monthly = { ...weekly, cycle: 'monthly', weekday: null, month_day: 31, short_month: 'last_day' };
  assert.equal(validate(weekly), true); assert.equal(validate(monthly), true);
  for (const v of [{}, { ...weekly, month_day: 1 }, { ...weekly, weekday: null }, { ...weekly, short_month: 'skip' }, { ...monthly, short_month: '' }, { ...monthly, month_day: 32 }, { ...weekly, boundary_time: '24:00:00' }, { ...weekly, extra: true }]) assert.equal(validate(v), false);
});
