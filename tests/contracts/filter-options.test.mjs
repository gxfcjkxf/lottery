import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import Ajv2020 from 'ajv/dist/2020.js';
import addFormats from 'ajv-formats';
import { operations, schemas } from '../../scripts/openapi-filter-options.mjs';
import { commonSchemas } from '../../scripts/openapi-lib.mjs';

const document = JSON.parse(readFileSync(new URL('../../docs/openapi.json', import.meta.url), 'utf8'));
const ajv = new Ajv2020({ strict: false, allErrors: true });
addFormats(ajv);
ajv.addSchema({ $id: 'urn:lottery:filter-options', components: { schemas: { ...commonSchemas, ...schemas } } });
const page = ajv.compile({ $ref: 'urn:lottery:filter-options#/components/schemas/FilterOptionsPage' });
const id = '0199a000-0000-7000-8000-000000000001';

test('both registered filter-options entries are generated with distinct identities and correct scope grants', () => {
  for (const entry of ['admin', 'platform']) {
    const path = `/api/v1/${entry}/filter-options`;
    const operation = document.paths[path]?.get;
    assert.ok(operation, path);
    assert.equal(operation.operationId, `${entry}ListFilterOptions`);
    assert.equal(operation['x-idempotent-operation'], false);
    const scope = entry === 'admin' ? 'brand' : 'platform';
    assert.ok(operation['x-permissions'].includes(`report_reward.view.${scope}`));
    assert.ok(operation['x-permissions'].includes(`report_commission.view.${scope}`));
    assert.ok(operation['x-permissions'].every(grant => grant.endsWith(`.view.${scope}`)));
    assert.ok(operation.parameters.some(parameter => parameter.name === 'X-Brand-ID' && parameter.required));
    assert.equal(operation.responses['200'].headers['Cache-Control'].schema.const, 'no-store');
    assert.match(operation.description, /filter.options.view audit/);
  }
});

test('documented query is bounded and distinguishes target resource from option kind', () => {
  for (const operation of operations) {
    const parameters = Object.fromEntries(operation.parameters.map(parameter => [parameter.name, parameter]));
    assert.deepEqual(Object.keys(parameters).sort(), ['kind', 'limit', 'offset', 'resource']);
    assert.equal(parameters.resource.required, true);
    assert.equal(parameters.kind.required, true);
    assert.ok(parameters.resource.schema.enum.includes('report_reward_orders'));
    assert.ok(parameters.kind.schema.enum.includes('audit_resource'));
    assert.equal(parameters.limit.schema.default, 50);
    assert.equal(parameters.limit.schema.maximum, 100);
    assert.equal(parameters.offset.schema.maximum, 1000000);
    assert.match(parameters.kind.description, /report_commission: agent, member, cycle/);
    assert.match(operation.description, /Unknown or repeated query parameters/);
  }
});

test('filter option responses allow empty pages and reject undocumented shape and pagination', () => {
  const valid = { brand_id: id, items: [{ id, name: '账号 · 2026-10-20' }], limit: 50, offset: 0 };
  assert.ok(page(valid), JSON.stringify(page.errors));
  assert.ok(page({ ...valid, items: [] }));
  for (const invalid of [
    { ...valid, brand_id: 'invalid' },
    { ...valid, items: [{ id }] },
    { ...valid, items: [{ id, name: 'Account', secret: 'undeclared' }] },
    { ...valid, total: 1 },
    { ...valid, limit: 101 },
    { ...valid, offset: -1 },
  ]) assert.equal(page(invalid), false);
});
