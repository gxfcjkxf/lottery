import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const doc = JSON.parse(readFileSync(new URL('../../docs/openapi.json', import.meta.url), 'utf8'));

test('commission discovery exposes only the audited list and idempotent retry operations', () => {
  const paths = Object.entries(doc.paths).filter(([path]) => /^\/api\/v1\/admin\/commission-discovery(?:\/|$)/.test(path));
  assert.deepEqual(paths.flatMap(([path, methods]) => Object.keys(methods).map(method => `${method.toUpperCase()} ${path}`)).sort(), [
    'GET /api/v1/admin/commission-discovery',
    'POST /api/v1/admin/commission-discovery/{id}/retry',
  ]);
  const list = doc.paths['/api/v1/admin/commission-discovery'].get;
  const retry = doc.paths['/api/v1/admin/commission-discovery/{id}/retry'].post;
  assert.deepEqual(list['x-permissions'], ['commission.view.brand', 'commission.view.platform']);
  assert.deepEqual(retry['x-permissions'], ['commission.retry.brand']);
  assert.deepEqual(doc.paths['/api/v1/platform/commission-discovery'].get['x-permissions'], ['commission.view.brand', 'commission.view.platform']);
  assert.ok(!Object.keys(doc.paths).some(path => /^\/api\/v1\/platform\/commission-discovery\//.test(path)));
  assert.equal(retry['x-idempotent-operation'], true);
  const discovery = doc.components.schemas.CommissionDiscovery;
  assert.equal(discovery.additionalProperties, false);
  assert.deepEqual(discovery.properties.state.enum, ['pending', 'registered', 'failed']);
  assert.equal(discovery.properties.cycle_id.anyOf[1].type, 'null');
  assert.equal(discovery.properties.window_from.anyOf[1].type, 'null');
  assert.ok(!Object.keys(doc.paths).some(path => /commission-discovery.*(?:payout|approve)/.test(path)));
});
