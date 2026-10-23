import { readFileSync } from 'node:fs';
import assert from 'node:assert/strict';
import test from 'node:test';
const read = path => readFileSync(new URL(`../../${path}`, import.meta.url), 'utf8');
test('platform access keeps account scopes explicit and uses unified authentication errors', () => {
  const api = read('platform-web/src/access-api.ts');
  const panel = read('platform-web/src/AccessPanel.vue');
  assert.match(api, /extends PlatformApiError/);
  assert.match(api, /method: options.method \?\? 'GET'/);
  assert.match(api, /Idempotency-Key/);
  assert.match(api, /platform-accounts/);
  assert.doesNotMatch(api, /method: '(PUT|DELETE)'/);
  assert.match(api, /readPlatformEnvelope/);
  assert.match(panel, /onBeforeUnmount/);
  assert.match(panel, /grants.includes\('admin.write.platform'\)/);
  assert.match(panel, /grants.includes\('role.write.platform'\)/);
  assert.match(panel, /optionsReady\.value = mode === 'password' \|\| mode === 'account-delete'/);
  assert.match(panel, /access-delete-modal/);
  assert.doesNotMatch(panel, /total_count|v-html/);
});
test('brand shell no longer contains unreachable fictional directory or ledger state', () => {
  const source = read('admin-web/src/App.vue');
  assert.doesNotMatch(source, /const demoBrand|const ledger\s*=|aurora\.demo|Prototype v0\.1|showDemoNotice/);
  assert.match(source, /brandPermissionSet/);
  assert.match(source, /login-entry__form/);
});
test('account and role deletion notices identify the deleted target and remain localized', () => {
  const panel = read('platform-web/src/AccessPanel.vue');
  const catalog = read('platform-web/src/i18n-catalog.ts');
  assert.match(panel, /key: 'access.accountDeleted', name: target.username/);
  assert.match(panel, /key: 'access.roleDeleted', name: target.name/);
  assert.match(panel, /:message="ui\(notice.key, \{ name: notice.name \}\)"/);
  assert.match(catalog, /删除账号「\{name\}」成功/);
  assert.match(catalog, /Account “\{name\}” deleted successfully/);
});
