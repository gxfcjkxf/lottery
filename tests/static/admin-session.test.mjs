import { test } from 'node:test';
import assert from 'node:assert/strict';
import { rememberAdminSession, restoreAdminSession } from '../browser/support/admin-session.ts';

const origin = 'http://localhost:5174';
const brand = '0199a000-0000-7000-8000-000000000002';
const cookie = { name: 'lottery_admin', value: 'synthetic-unit-cookie', domain: 'localhost', path: '/', expires: -1, httpOnly: true, secure: false, sameSite: 'Lax' };
function context(account, status = 200) {
  const added = [], cleared = [], reads = [];
  return {
    added, cleared, reads,
    async addCookies(cookies) { added.push(cookies); },
    async clearCookies(options) { cleared.push(options); },
    request: { async get(url, options) { reads.push({ url, options }); return { status: () => status, json: async () => ({ success: true, data: { account } }) }; } },
  };
}

test('admin session restoration rechecks identity and brand, and copies only the admin cookie', async () => {
  const user = 'unit_bound_identity';
  const cookies = [ { ...cookie }, { ...cookie, name: 'lottery_user' } ];
  rememberAdminSession(user, cookies, origin, 'original-account');
  cookies[0].value = 'changed-after-cache';
  const ctx = context({ id: 'original-account', brand_ids: [brand] });
  assert.equal(await restoreAdminSession(ctx, user, brand, origin), true);
  assert.deepEqual(ctx.added, [[cookie]]);
  assert.deepEqual(ctx.reads, [{ url: `${origin}/api/v1/admin/me`, options: { headers: { 'X-Brand-ID': brand } } }]);
  assert.deepEqual(ctx.cleared, []);
});

test('admin session cache rejects identity changes and removes the stale entry', async () => {
  const user = 'unit_replaced_identity';
  rememberAdminSession(user, [cookie], origin, 'original-account');
  const ctx = context({ id: 'different-account', brand_ids: [brand] });
  assert.equal(await restoreAdminSession(ctx, user, brand, origin), false);
  assert.deepEqual(ctx.cleared, [{ name: 'lottery_admin' }]);
  const retry = context({ id: 'original-account', brand_ids: [brand] });
  assert.equal(await restoreAdminSession(retry, user, brand, origin), false);
  assert.deepEqual(retry.reads, []);
});

test('legacy sessions bind an identity on first verification and cannot switch accounts', async () => {
  const user = 'unit_legacy_identity';
  rememberAdminSession(user, [cookie]);
  assert.equal(await restoreAdminSession(context({ id: 'original-account', brand_ids: [brand] }), user, brand), true);
  assert.equal(await restoreAdminSession(context({ id: 'different-account', brand_ids: [brand] }), user, brand), false);
});

test('revoked and foreign-brand admin sessions are discarded', async () => {
  for (const [name, account, status] of [
    ['revoked', { id: 'original-account', brand_ids: [brand] }, 401],
    ['foreign', { id: 'original-account', brand_ids: ['other-brand'] }, 200],
    ['missing', { brand_ids: [brand] }, 200],
  ]) {
    const user = `unit_${name}_identity`;
    rememberAdminSession(user, [cookie]);
    const ctx = context(account, status);
    assert.equal(await restoreAdminSession(ctx, user, brand), false);
    assert.deepEqual(ctx.cleared, [{ name: 'lottery_admin' }]);
  }
});
