import { test } from 'node:test';
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { readFileSync } from 'node:fs';

const doc = JSON.parse(readFileSync(new URL('../../docs/openapi.json', import.meta.url), 'utf8'));
const go = process.env.LOTTERY_GO_BIN ?? 'go';
const operationKey = ({ method, path }) => `${method.toUpperCase()} ${path}`;

test('rule review contract permits authorized self-review without removing validation or platform isolation', () => {
  for (const decision of ['approve', 'reject']) {
    const operation = doc.paths[`/api/v1/admin/rule-versions/{id}/${decision}`].post;
    assert.match(operation.description, /rule\.review\.brand/);
    assert.match(operation.description, /may review rules they created or edited/);
    assert.match(operation.description, /validation.*warning/i);
    assert.match(operation.description, /SUPER_ADMIN is read-only/);
    assert.doesNotMatch(operation.description, /different authorized reviewer|creator\/editor/);
    assert.equal(doc.paths[`/api/v1/platform/rule-versions/{id}/${decision}`], undefined);
  }
});

test('platform admin entry documents only registered independent routes and its own cookie boundary', () => {
  const result = spawnSync(go, ['run', '-buildvcs=false', './cmd/route-inventory'], {
    cwd: new URL('../../backend/', import.meta.url),
    encoding: 'utf8',
    env: { ...process.env, CGO_ENABLED: '0' },
  });
  assert.equal(result.status, 0, result.stderr || result.error?.message);
  const routes = JSON.parse(result.stdout);
  const platformRoutes = routes.filter(route => route.path.startsWith('/api/v1/platform/'));
  const expectedPlatformRoutes = routes
    .filter(route => route.path.startsWith('/api/v1/admin/'))
    .filter(route => route.method === 'GET'
      || route.method === 'POST' && ['/api/v1/admin/auth/login', '/api/v1/admin/auth/logout', '/api/v1/admin/brands', '/api/v1/admin/accounts', '/api/v1/admin/accounts/{id}/reset-password', '/api/v1/admin/roles'].includes(route.path)
      || route.method === 'PATCH' && ['/api/v1/admin/accounts/{id}', '/api/v1/admin/roles/{id}', '/api/v1/admin/brand-operation'].includes(route.path))
    .map(route => ({ ...route, path: route.path.replace('/api/v1/admin/', '/api/v1/platform/') }));
  for (const [method, path] of [['GET', '/platform-accounts'], ['GET', '/platform-roles'], ['POST', '/platform-accounts'], ['PATCH', '/platform-accounts/{id}'], ['POST', '/platform-accounts/{id}/reset-password']]) {
    expectedPlatformRoutes.push({ method, path: `/api/v1/platform${path}` });
  }
  assert.deepEqual(platformRoutes.map(operationKey).sort(), expectedPlatformRoutes.map(operationKey).sort());

  const documentedPlatformRoutes = Object.entries(doc.paths)
    .filter(([path]) => path.startsWith('/api/v1/platform/'))
    .flatMap(([path, methods]) => Object.keys(methods).map(method => `${method.toUpperCase()} ${path}`));
  assert.deepEqual(documentedPlatformRoutes.sort(), platformRoutes.map(operationKey).sort());

  const schemes = doc.components.securitySchemes;
  assert.equal(schemes.adminCookie.in, 'cookie');
  assert.equal(schemes.adminCookie.name, 'lottery_admin');
  assert.equal(schemes.platformAdminCookie.in, 'cookie');
  assert.equal(schemes.platformAdminCookie.name, 'lottery_platform_admin');
  assert.notEqual(schemes.adminCookie.name, schemes.platformAdminCookie.name);

  for (const route of platformRoutes) {
    const operation = doc.paths[route.path][route.method.toLowerCase()];
    if (route.path === '/api/v1/platform/auth/login') assert.deepEqual(operation.security, []);
    else assert.deepEqual(operation.security, [{ adminBearer: [] }, { platformAdminCookie: [] }]);
    assert.match(operation.description, /only super administrator accounts are accepted/i);
    assert.match(operation.description, /brand staff accounts are rejected/i);
  }
  for (const route of routes.filter(route => route.path.startsWith('/api/v1/admin/'))) {
    const operation = doc.paths[route.path]?.[route.method.toLowerCase()];
    if (!operation) continue;
    if (route.path === '/api/v1/admin/auth/login') {
      assert.deepEqual(operation.security, []);
      continue;
    }
    assert.deepEqual(operation.security, [{ adminBearer: [] }, { adminCookie: [] }]);
    assert.match(operation.description, /brand staff entry/i);
    assert.match(operation.description, /platform accounts are rejected/i);
  }

  const platformWrites = platformRoutes.filter(route => route.method !== 'GET').map(operationKey).sort();
  assert.deepEqual(platformWrites, [
    'PATCH /api/v1/platform/accounts/{id}',
    'PATCH /api/v1/platform/brand-operation',
    'PATCH /api/v1/platform/platform-accounts/{id}',
    'PATCH /api/v1/platform/roles/{id}',
    'POST /api/v1/platform/accounts',
    'POST /api/v1/platform/accounts/{id}/reset-password',
    'POST /api/v1/platform/auth/login',
    'POST /api/v1/platform/auth/logout',
    'POST /api/v1/platform/brands',
    'POST /api/v1/platform/platform-accounts',
    'POST /api/v1/platform/platform-accounts/{id}/reset-password',
    'POST /api/v1/platform/roles',
  ]);
  assert.ok(!platformRoutes.some(route => /\/(?:approve|members?)(?:\/|$)/i.test(route.path)));
});
