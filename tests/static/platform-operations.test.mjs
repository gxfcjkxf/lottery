import { readFileSync } from 'node:fs';
import assert from 'node:assert/strict';
import test from 'node:test';

const read = path => readFileSync(new URL(`../../${path}`, import.meta.url), 'utf8');

test('archive queries remain scoped reads and notification templates retain explicit writes', () => {
  for (const file of ['archives-api.ts']) {
    const source = read(`platform-web/src/${file}`);
    assert.match(source, /X-Brand-ID/);
    assert.match(source, /method: 'GET'/);
    assert.doesNotMatch(source, /method: '(POST|PUT|PATCH|DELETE)'/);
  }
  const app = read('platform-web/src/App.vue');
  assert.match(app, /platform-operations-tabs/);
  assert.match(app, /<ArchivesPanel/);
  assert.match(app, /<NotificationsPanel/);
});

test('archive snapshots show capture-time balances without a file download entry in phase one', () => {
  const panel = read('platform-web/src/ArchivesPanel.vue');
  const catalog = read('platform-web/src/i18n-catalog.ts');
  assert.match(panel, /<ArchiveReport/);
  assert.match(panel, /brand !== props.brandId/);
  assert.match(panel, /onBeforeUnmount/);
  assert.match(catalog, /not balances at the end of the report window/);
  assert.doesNotMatch(panel, /api\.download|URL\.createObjectURL|\.download\s*=|v-html|Number\(.*points|parseFloat/);
});

test('notification content stays plaintext and page sentinels do not invent total counts', () => {
  const panel = read('platform-web/src/NotificationsPanel.vue');
  assert.doesNotMatch(panel, /v-html|total_count|totalCount/);
  assert.match(panel, /api.deliveries\(brandId, pageSize.value, offset\)/);
  const templates = read('platform-web/src/NotificationTemplatesPanel.vue');
  assert.doesNotMatch(templates, /loadHistory|api\.history|v-html/);
  assert.match(panel, /\[rows.items, readPaginationTotal\(rows\)\]/);
  assert.match(panel, /readPaginationTotal\(rows\)/);
  assert.match(panel, /deliveryMore\.value = BigInt\(offset \+ rows.items.length\) < BigInt\(deliveryTotal.value\)/);
  assert.match(templates, /template-languages/);
  assert.match(panel, /onBeforeUnmount/);
});
