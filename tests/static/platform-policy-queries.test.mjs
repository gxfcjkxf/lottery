import { readFileSync } from 'node:fs';
import assert from 'node:assert/strict';
import test from 'node:test';
const read = path => readFileSync(new URL(`../../${path}`, import.meta.url), 'utf8');

test('platform archive and financial policy clients are scoped GET-only readers', () => {
  for (const name of ['archive-tasks-api.ts', 'financial-policies-api.ts']) {
    const source = read(`platform-web/src/${name}`);
    assert.match(source, /method: 'GET'/);
    assert.match(source, /X-Brand-ID/);
    assert.match(source, /response.status !== 200/);
    assert.doesNotMatch(source, /method: '(POST|PUT|PATCH|DELETE)'/);
  }
});

test('policy readers preserve integer strings and isolate late requests without write buttons', () => {
  const finance = read('platform-web/src/FinancialPoliciesPanel.vue');
  const tasks = read('platform-web/src/ArchiveTasksPanel.vue');
  for (const panel of [finance, tasks]) {
    assert.match(panel, /onBeforeUnmount/);
    assert.match(panel, /flush: 'sync'/);
    assert.doesNotMatch(panel, /v-html|parseFloat|Number\(.*points|api\.(retry|update|create)/);
  }
  assert.match(finance, /a game override takes precedence/);
  assert.match(finance, /selected.value === kind/);
  assert.match(tasks, /separate snapshots/);
  assert.match(tasks, /does not mean this task produced a new archive/);
  assert.match(tasks, /detailLoading.value = false/);
});

test('policy browser checks genuine unchanged configuration plus explicit synthetic pagination', () => {
  const spec = read('tests/review/platform-policy-queries.spec.ts');
  assert.match(spec, /genuine archive and financial policies/);
  assert.match(spec, /synthetic archive task paging/);
  assert.match(spec, /toEqual\(data\)/);
  assert.match(spec, /\[404, 405\]/);
  assert.doesNotMatch(spec, /auth_rate_limits|X-Forwarded-For|DELETE FROM|force: true/);
});
