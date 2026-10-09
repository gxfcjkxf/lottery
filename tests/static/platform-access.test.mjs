import { readFileSync } from 'node:fs';
import assert from 'node:assert/strict';
import test from 'node:test';
const read = path => readFileSync(new URL(`../../${path}`, import.meta.url), 'utf8');
test('platform access keeps server catalogs read-only and uses unified authentication errors', () => {
  const api = read('platform-web/src/access-api.ts');
  const panel = read('platform-web/src/AccessPanel.vue');
  assert.match(api, /extends PlatformApiError/);
  assert.match(api, /method: 'GET'/);
  assert.match(api, /response.status !== 200/);
  assert.doesNotMatch(api, /method: '(POST|PATCH|PUT|DELETE)'/);
  assert.match(panel, /No single-record request was made/);
  assert.match(panel, /not the selected account’s grants/);
  assert.match(panel, /onBeforeUnmount/);
  assert.doesNotMatch(panel, /total_count|v-html|\.write\(|\.create\(/);
});
test('brand shell no longer contains unreachable fictional directory or ledger state', () => {
  const source = read('admin-web/src/App.vue');
  assert.doesNotMatch(source, /const demoBrand|const ledger\s*=|aurora\.demo|Prototype v0\.1|showDemoNotice/);
  assert.match(source, /brandPermissionSet/);
  assert.match(source, /login-entry__form/);
});
