import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
const doc = JSON.parse(readFileSync(new URL('../../docs/openapi.json', import.meta.url), 'utf8'));
test('commission cycle APIs are exact, primary, audited and do not advertise payouts', () => {
  const routes = Object.entries(doc.paths).filter(([p]) => /^\/api\/v1\/admin\/commission-cycles(?:\/|$)/.test(p)).flatMap(([p,v]) => Object.keys(v).map(m => `${m.toUpperCase()} ${p}`));
  assert.deepEqual(routes.sort(), [
    'GET /api/v1/admin/commission-cycles', 'POST /api/v1/admin/commission-cycles', 'GET /api/v1/admin/commission-cycles/{id}',
    'GET /api/v1/admin/commission-cycles/{id}/earnings', 'POST /api/v1/admin/commission-cycles/{id}/retry',
    'GET /api/v1/admin/commission-cycles/{id}/runs', 'GET /api/v1/admin/commission-cycles/{id}/runs/{runID}/calculations',
    'GET /api/v1/admin/commission-cycles/{id}/runs/{runID}/earnings', 'GET /api/v1/admin/commission-cycles/{id}/runs/{runID}/allocations',
  ].sort());
  const platformReads = Object.entries(doc.paths).filter(([p]) => /^\/api\/v1\/platform\/commission-cycles(?:\/|$)/.test(p)).flatMap(([p,v]) => Object.keys(v).map(m => `${m.toUpperCase()} ${p}`));
  assert.deepEqual(platformReads.sort(), [
    'GET /api/v1/platform/commission-cycles', 'GET /api/v1/platform/commission-cycles/{id}',
    'GET /api/v1/platform/commission-cycles/{id}/earnings', 'GET /api/v1/platform/commission-cycles/{id}/runs',
    'GET /api/v1/platform/commission-cycles/{id}/runs/{runID}/calculations',
    'GET /api/v1/platform/commission-cycles/{id}/runs/{runID}/earnings',
    'GET /api/v1/platform/commission-cycles/{id}/runs/{runID}/allocations',
  ].sort());
  assert.ok(platformReads.every(route => route.startsWith('GET ')));
  assert.deepEqual(doc.paths['/api/v1/admin/commission-cycles'].post['x-permissions'], ['commission.run.brand']);
  assert.deepEqual(doc.paths['/api/v1/admin/commission-cycles/{id}/retry'].post['x-permissions'], ['commission.retry.brand']);
  assert.deepEqual(doc.components.schemas.CommissionCycle.properties.state.enum, ['enumerating','waiting','calculating','summarizing','ready','failed']);
  assert.match(doc.components.schemas.CommissionCycle.description, /not paid points/);
  const cyclePaths = Object.entries(doc.paths).filter(([p]) => /^\/api\/v1\/admin\/commission-cycles(?:\/|$)/.test(p));
  assert.ok(!cyclePaths.some(([p]) => /(?:payout|approve)/.test(p)));
  assert.ok(!cyclePaths.some(([, path]) => Object.values(path).some(operation => (operation['x-permissions'] ?? []).some(permission => permission.startsWith('commission_payment')))));
});
test('financial cycle DTOs preserve exact/null fields and omit private witness data', () => {
  const s = doc.components.schemas;
  for (const name of ['CommissionCycle','CommissionCycleEarning','CommissionRun','CommissionCalculation']) {
    assert.equal(s[name].additionalProperties,false);
    assert.ok(!Object.keys(s[name].properties).some(k => /snapshot|cursor|account_id|agent_path/.test(k)));
  }
  assert.deepEqual(s.CommissionCycleCreate.required,['anchor_order_id','reason']);
  assert.deepEqual(s.CommissionCycleRetry.required,['version','reason']);
  assert.equal(s.CommissionCalculation.properties.generation.anyOf[1].type,'null');
  assert.equal(s.CommissionCycle.properties.total_points.type,'string');
  assert.equal(s.CommissionExactAmount.properties.denominator.pattern,'^[1-9][0-9]*$');
  assert.equal(s.CommissionCycle.properties.created_by.anyOf[1].type,'null');
  assert.deepEqual(s.CommissionCycle.properties.creation_actor_type.enum,['admin','system']);
  assert.ok(!s.CommissionCycle.required.includes('creation_actor_type'));
});
