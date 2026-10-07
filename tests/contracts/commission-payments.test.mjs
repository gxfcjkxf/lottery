import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import Ajv2020 from 'ajv/dist/2020.js';
import addFormats from 'ajv-formats';
import { operations, schemas } from '../../scripts/openapi-commission-payments.mjs';

const doc = JSON.parse(readFileSync(new URL('../../docs/openapi.json', import.meta.url), 'utf8'));
const ajv = new Ajv2020({ strict: false, allErrors: true });
addFormats(ajv);
ajv.addSchema({ $id: 'urn:lottery:commission-payment-contract', components: { schemas: { ...doc.components.schemas, ...schemas } } });
const validate = name => ajv.compile({ $ref: `urn:lottery:commission-payment-contract#/components/schemas/${name}` });

test('commission payment contract exposes exactly six brand-scoped operations', () => {
  assert.deepEqual(operations.map(({ method, path }) => `${method} ${path}`).sort(), [
    'GET /api/v1/admin/commission-payment-policy',
    'PUT /api/v1/admin/commission-payment-policy',
    'GET /api/v1/admin/commission-payments',
    'GET /api/v1/admin/commission-payments/{id}',
    'POST /api/v1/admin/commission-payments/{id}/approve',
    'POST /api/v1/admin/commission-payments/{id}/retry',
  ].sort());
  for (const operation of operations) {
    assert.equal(operation.auth, 'admin');
    assert.equal(operation.brandHeader, true);
    if(operation.method!=='GET'){
      const actor=operation.parameters.find(p=>p.name==='X-Commission-Payment-Actor-ID');
      assert.equal(actor?.in,'header');assert.equal(actor?.required,true);
    }
  }
  assert.deepEqual(operations.find(op => op.operationId === 'approveCommissionPayment').permissions, ['commission_payment.approve.brand']);
  assert.deepEqual(operations.find(op => op.operationId === 'retryCommissionPayment').permissions, ['commission_payment.retry.brand']);
  assert.deepEqual(operations.find(op => op.operationId === 'updateCommissionPaymentPolicy').permissions, ['commission_payment_policy.write.brand']);
});

test('commission payment schemas are closed, precise and omit target details', () => {
  for (const name of ['CommissionPaymentPolicy', 'CommissionPaymentPolicyInput', 'CommissionPayment', 'CommissionPaymentPage', 'CommissionPaymentAction']) {
    assert.equal(schemas[name].additionalProperties, false, `${name} should be closed`);
  }
  assert.deepEqual(schemas.CommissionPaymentPolicyInput.required, ['version', 'enabled', 'reason']);
  assert.deepEqual(schemas.CommissionPaymentPolicy.properties.audit_log_id.anyOf, [
    { $ref: '#/components/schemas/UUID' },
    { type: 'string', const: '' },
  ]);
  assert.deepEqual(schemas.CommissionPaymentAction.required, ['version', 'reason']);
  assert.deepEqual(schemas.CommissionPayment.properties.state.enum, ['awaiting_approval', 'paying', 'paid', 'failed', 'stale', 'blocked']);
  assert.deepEqual(schemas.CommissionPayment.properties.payout_mode.enum, ['manual', 'automatic', 'mixed', 'none']);
  for (const field of ['total_points', 'paid_points', 'target_count', 'paid_count']) {
    assert.equal(schemas.CommissionPayment.properties[field].type, 'string');
  }
  assert.ok(!Object.keys(schemas.CommissionPayment.properties).some(field => /^(targets?|member_id|agent_id|account_id)$/.test(field)));
  assert.match(schemas.CommissionPayment.description, /Mixed mode remains blocked/);
});

test('the initial disabled payment policy response accepts its empty audit id', () => {
  const initial = {
    brand_id: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa',
    version: 1,
    enabled: false,
    audit_log_id: '',
    updated_at: '2026-10-07T00:00:00Z',
  };
  const check = validate('CommissionPaymentPolicy');
  assert.ok(check(initial), JSON.stringify(check.errors));
  assert.ok(!check({ ...initial, audit_log_id: null }), 'database NULL maps to empty string in this DTO');
  assert.ok(!check({ ...initial, audit_log_id: 'not-a-uuid' }));
  assert.ok(check({ ...initial, version: 2, enabled: true, audit_log_id: initial.brand_id }));
});
