import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import Ajv2020 from 'ajv/dist/2020.js';
import addFormats from 'ajv-formats';
import { operations, schemas } from '../../scripts/openapi-rewards.mjs';
const doc = JSON.parse(readFileSync(new URL('../../docs/openapi.json', import.meta.url), 'utf8'));
const ajv = new Ajv2020({ strict: false, allErrors: true }); addFormats(ajv);
ajv.addSchema({ $id: 'urn:lottery:reward', components: { schemas: { ...doc.components.schemas, ...schemas } } });
const validate = name => ajv.compile({ $ref: `urn:lottery:reward#/components/schemas/${name}` });
const id = '11111111-1111-4111-8111-111111111111', time = '2026-10-08T00:00:00Z';
const granted = { id, brand_id: id, member_id: id, points: '9223372036854775807', state: 'granted', version: 1, grant_ledger_entry_id: id, revoke_ledger_entry_id: null,
  creation_audit_log_id: id, last_audit_log_id: id, last_error_code: null, created_by: id, reason: 'explicit grant', point_policy_version: '1', created_at: time, updated_at: time, revoked_at: null };
test('reward contract exposes exactly six scoped operations and three actor-bound writes', () => {
  assert.deepEqual(operations.map(o => `${o.method} ${o.path}`).sort(), [
    'GET /api/v1/admin/reward-orders', 'POST /api/v1/admin/reward-orders', 'GET /api/v1/admin/reward-orders/{id}',
    'GET /api/v1/admin/reward-orders/{id}/actions', 'POST /api/v1/admin/reward-orders/{id}/revoke', 'POST /api/v1/admin/reward-orders/{id}/retry-revocation',
  ].sort());
  for (const op of operations) {
    assert.equal(op.auth, 'admin'); assert.equal(op.brandHeader, true);
    if (op.method === 'POST') {
      assert.equal(op.idempotency, true); assert.deepEqual(op.parameters.map(p => [p.name, p.in, p.required]), [['X-Reward-Actor-ID', 'header', true]]);
      assert.match(op.permissions[0], /^reward\.(grant|revoke|retry)\.brand$/);
      assert.match(op.description, /shortage returns a durable successful/);
    }
  }
  assert.equal(operations.find(o => o.operationId === 'grantManualReward').successStatus, 201);
});
test('reward schemas are closed and preserve int64 point strings with no wallet evidence', () => {
  for (const s of Object.values(schemas)) assert.equal(s.additionalProperties, false);
  assert.equal(validate('RewardOrder')(granted), true);
  assert.equal(validate('RewardGrantInput')({ member_id: id, points: granted.points, reason: 'manual grant' }), true);
  for (const points of [25, '0', '-1', '01', '1.1']) assert.equal(validate('RewardGrantInput')({ member_id: id, points, reason: 'grant' }), false);
  assert.ok(!Object.keys(schemas.RewardOrder.properties).some(k => /account_id|bucket|snapshot/.test(k)));
  assert.equal(validate('RewardOrder')({ ...granted, hidden: 'field' }), false);
});
test('reward progress and action nullability distinguish pending from completed reversal', () => {
  const order = validate('RewardOrder'), action = validate('RewardOrderAction');
  const pending = { ...granted, state: 'revocation_pending', version: 2, last_error_code: 'REWARD_AVAILABLE_INSUFFICIENT' };
  const revoked = { ...granted, state: 'revoked', version: 3, revoke_ledger_entry_id: id, revoked_at: time };
  assert.equal(order(pending), true); assert.equal(order(revoked), true);
  for (const bad of [{ ...pending, last_error_code: null }, { ...pending, revoke_ledger_entry_id: id }, { ...revoked, revoke_ledger_entry_id: null }, { ...revoked, revoked_at: null }, { ...granted, version: 2 }]) assert.equal(order(bad), false);
  const row = { id, brand_id: id, order_id: id, version: 2, operation: 'revoke', state_before: 'granted', state_after: 'revocation_pending', actor_id: id, reason: 'shortage', audit_log_id: id, ledger_entry_id: null, created_at: time };
  assert.equal(action(row), true); assert.equal(action({ ...row, operation: 'retry', state_before: 'revocation_pending' }), true);
  assert.equal(action({ ...row, state_after: 'revoked', ledger_entry_id: id }), true);
  for (const bad of [{ ...row, ledger_entry_id: id }, { ...row, state_after: 'revoked' }, { ...row, operation: 'retry' }, { ...row, version: 1 }]) assert.equal(action(bad), false);
});
