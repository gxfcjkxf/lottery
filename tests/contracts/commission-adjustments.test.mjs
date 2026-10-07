import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import Ajv2020 from 'ajv/dist/2020.js';
import addFormats from 'ajv-formats';
import { operations, schemas } from '../../scripts/openapi-commission-adjustments.mjs';

const doc = JSON.parse(readFileSync(new URL('../../docs/openapi.json', import.meta.url), 'utf8'));
const ajv = new Ajv2020({ strict: false, allErrors: true });
addFormats(ajv);
ajv.addSchema({ $id: 'urn:lottery:commission-adjustment-contract', components: { schemas: { ...doc.components.schemas, ...schemas } } });
const validate = name => ajv.compile({ $ref: `urn:lottery:commission-adjustment-contract#/components/schemas/${name}` });

const brand = 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa';
const target = 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb';

test('commission adjustment contract exposes two reads and one actor-bound write', () => {
  assert.deepEqual(operations.map(({ method, path }) => `${method} ${path}`).sort(), [
    'GET /api/v1/admin/commission-payments/{id}/targets',
    'GET /api/v1/admin/commission-payment-targets/{id}/adjustments',
    'POST /api/v1/admin/commission-payment-targets/{id}/adjustments',
  ].sort());
  const write = operations.find(operation => operation.method === 'POST');
  assert.equal(write.auth, 'admin');
  assert.equal(write.brandHeader, true);
  assert.equal(write.idempotency, true);
  assert.equal(write.successStatus, 201);
  assert.deepEqual(write.permissions, ['commission_adjustment.write.brand']);
  assert.deepEqual(write.parameters.map(parameter => [parameter.name, parameter.in, parameter.required]), [
    ['X-Commission-Payment-Actor-ID', 'header', true],
  ]);
  assert.deepEqual(schemas.CommissionAdjustmentInput.required, ['version', 'points', 'reason']);
});

test('adjustment schemas are closed and preserve exact point values', () => {
  for (const name of Object.keys(schemas)) assert.equal(schemas[name].additionalProperties, false, `${name} must be closed`);
  assert.equal(schemas.CommissionAdjustmentInput.properties.points.$ref, '#/components/schemas/NonnegativeInt64String');
  assert.equal(schemas.CommissionAdjustment.properties.points_before.$ref, '#/components/schemas/NonnegativeInt64String');
  assert.equal(schemas.CommissionAdjustment.properties.point_policy_version.$ref, '#/components/schemas/PositiveInt64String');
  assert.deepEqual(schemas.CommissionAdjustment.properties.delta_points.allOf[1], { not: { const: '0' } });
  assert.ok(!Object.keys(schemas.CommissionPaymentTarget.properties).some(name => /account|wallet|rule|bucket/i.test(name)));
});

test('pending targets may have no head, while adjustments require a completed exact receipt', () => {
  const page = {
    brand_id: brand,
    payment_id: target,
    items: [{
      id: target, brand_id: brand, payment_id: target, earning_id: target, agent_id: target, member_id: target,
      original_points: '9007199254740993', state: 'pending', ledger_entry_id: null, paid_at: null,
      adjustment_version: null, adjusted_points: null, last_adjustment_id: null,
    }],
    total_count: '1', limit: 20, offset: 0,
  };
  const checkPage = validate('CommissionPaymentTargetPage');
  assert.ok(checkPage(page), JSON.stringify(checkPage.errors));
  assert.ok(!checkPage({ ...page, items: [{ ...page.items[0], original_points: 9007199254740993 }] }));
  assert.ok(!checkPage({ ...page, items: [{ ...page.items[0], account_id: target }] }));

  const adjustment = {
    id: target, brand_id: brand, target_id: target, payment_id: target, version: 2,
    points_before: '9007199254740993', points_after: '9007199254740992', delta_points: '-1',
    ledger_entry_id: target, audit_log_id: target, created_by: target, reason: 'manual review',
    point_policy_version: '9007199254740993', created_at: '2026-10-08T00:00:00Z',
  };
  const checkAdjustment = validate('CommissionAdjustment');
  assert.ok(checkAdjustment(adjustment), JSON.stringify(checkAdjustment.errors));
  assert.ok(!checkAdjustment({ ...adjustment, delta_points: '-0' }));
  assert.ok(!checkAdjustment({ ...adjustment, ledger_entry_id: null }));
});
