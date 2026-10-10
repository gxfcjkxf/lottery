import { readFileSync } from 'node:fs';
import assert from 'node:assert/strict';
import test from 'node:test';
const read = path => readFileSync(new URL(`../../${path}`, import.meta.url), 'utf8');
test('operational policy queries remain GET-only and separated from verification and writes', () => {
  const api = read('platform-web/src/operational-policies-api.ts');
  const panel = read('platform-web/src/OperationalPoliciesPanel.vue');
  for (const path of ['bet-policy', 'settlement-policy', 'agent-policy', 'compliance-policy', 'commission-payment-policy', 'commission-correction-policy']) assert.ok(api.includes(`'/${path}'`));
  assert.match(api, /method: 'GET'/);
  assert.match(api, /X-Brand-ID/);
  assert.match(api, /response.status !== 200/);
  assert.doesNotMatch(api, /method: '(POST|PUT|PATCH|DELETE)'/);
  assert.match(panel, /Real verification and risk adapters are not connected/);
  assert.match(panel, /onBeforeUnmount/);
  assert.match(panel, /selected.value === kind/);
  assert.doesNotMatch(panel, /as any|v-html|parseFloat|Number\(.*points/);
});
