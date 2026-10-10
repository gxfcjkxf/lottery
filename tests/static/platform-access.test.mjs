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
  assert.match(panel, /No single-record request was made/);
  assert.match(panel, /not the selected account’s grants/);
  assert.match(panel, /onBeforeUnmount/);
  assert.match(panel, /grants.includes\('admin.write.platform'\)/);
  assert.match(panel, /grants.includes\('role.write.platform'\)/);
  assert.doesNotMatch(panel, /total_count|v-html/);
});
test('brand shell no longer contains unreachable fictional directory or ledger state', () => {
  const source = read('admin-web/src/App.vue');
  assert.doesNotMatch(source, /const demoBrand|const ledger\s*=|aurora\.demo|Prototype v0\.1|showDemoNotice/);
  assert.match(source, /brandPermissionSet/);
  assert.match(source, /login-entry__form/);
});
