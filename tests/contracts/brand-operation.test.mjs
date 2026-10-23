import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import Ajv2020 from 'ajv/dist/2020.js';
import addFormats from 'ajv-formats';
import { schemas, operations } from '../../docs/openapi/identity.mjs';

const ajv = new Ajv2020({ strict: false, allErrors: true });
addFormats(ajv);
ajv.addSchema({ $id: 'urn:lottery:brand-operation', components: { schemas: { ...schemas, DateTime: { type: 'string', format: 'date-time' } } } });
const validate = ajv.compile({ $ref: 'urn:lottery:brand-operation#/components/schemas/AdminBrandOperationUpdateRequest' });

test('brand states accept optional or empty reasons and count Unicode characters', () => {
  for (const status of ['active', 'paused', 'disabled']) {
    for (const reason of [undefined, '', '中'.repeat(500), '😀'.repeat(500)]) {
      const body = { version: 1, status, ...(reason === undefined ? {} : { reason }) };
      assert.equal(validate(body), true, JSON.stringify(validate.errors));
    }
    for (const reason of [null, ' ', ' padded', 'padded ', '中'.repeat(501), '😀'.repeat(501)]) {
      assert.equal(validate({ version: 1, status, reason }), false);
    }
  }
  for (const body of [{ status: 'active' }, { version: 1 }, { version: 0, status: 'active' }, { version: 1, status: 'unknown' }]) {
    assert.equal(validate(body), false);
  }
});

test('pause windows require both timestamps and the paused target status', () => {
  const window = { pause_starts_at: '2030-01-01T01:00:00Z', pause_ends_at: '2030-01-01T02:00:00Z' };
  assert.equal(validate({ version: 1, status: 'paused', ...window }), true);
  for (const status of ['active', 'paused', 'disabled']) {
    assert.equal(validate({ version: 1, status, pause_starts_at: window.pause_starts_at }), false);
    assert.equal(validate({ version: 1, status, pause_ends_at: window.pause_ends_at }), false);
    if (status !== 'paused') assert.equal(validate({ version: 1, status, ...window }), false);
  }
  assert.equal(validate({ version: 1, status: 'paused', ...window, pause_starts_at: null }), false);
  assert.equal(validate({ version: 1, status: 'paused', ...window, pause_ends_at: 'invalid' }), false);
});

test('status writes document explicit grants, stopped-brand protection and original receipts', () => {
  const operation = operations.find(item => item.method === 'PATCH' && item.path === '/api/v1/admin/brand-operation');
  assert.equal(operation.idempotency, true);
  assert.deepEqual(operation.permissions, ['brand_operation.write.brand', 'brand_operation.write.platform']);
  assert.match(operation.description, /transition to or from disabled requires the platform grant/);
  assert.match(operation.description, /reason is optional and may be empty/);
  assert.match(operation.description, /original successful receipt/);
});

test('published OpenAPI preserves the source status rules for both entries', () => {
  const document = JSON.parse(readFileSync(new URL('../../docs/openapi.json', import.meta.url), 'utf8'));
  assert.deepEqual(document.components.schemas.AdminBrandOperationUpdateRequest, schemas.AdminBrandOperationUpdateRequest);
  for (const entry of ['admin', 'platform']) {
    const operation = document.paths[`/api/v1/${entry}/brand-operation`].patch;
    assert.match(operation.description, /reason is optional and may be empty/);
    assert.match(operation.description, /BRAND_OPERATION_STATE_CONFLICT/);
    assert.match(operation.description, /transition to or from disabled requires the platform grant/);
  }
});
