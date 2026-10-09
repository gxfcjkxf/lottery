import { readFileSync } from 'node:fs';
import assert from 'node:assert/strict';
import test from 'node:test';

const read = path => readFileSync(new URL(`../../${path}`, import.meta.url), 'utf8');

test('platform operations clients expose only scoped read methods', () => {
  for (const file of ['archives-api.ts', 'notifications-api.ts']) {
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

test('archive original file is verified before download and reports capture-time balances', () => {
  const client = read('platform-web/src/archives-api.ts');
  const panel = read('platform-web/src/ArchivesPanel.vue');
  assert.match(client, /crypto\.subtle\.digest\('SHA-256'/);
  assert.match(client, /snapshotsMatch\(payload, selectedRecord.snapshot\)/);
  assert.match(client, /return \{ bytes, filename/);
  assert.ok(panel.indexOf('await api.download') < panel.indexOf('URL.createObjectURL'));
  assert.match(panel, /brand !== props.brandId/);
  assert.match(panel, /onBeforeUnmount/);
  assert.match(panel, /not balances at the end of the report window/);
  assert.doesNotMatch(panel, /v-html|Number\(.*points|parseFloat/);
});

test('notification content stays plaintext and page sentinels do not invent total counts', () => {
  const panel = read('platform-web/src/NotificationsPanel.vue');
  assert.doesNotMatch(panel, /v-html|total_count|totalCount/);
  assert.match(panel, /api.deliveries\(brandId, 51, offset\)/);
  assert.match(panel, /api.history\(brandId, template.key, 51, offset\)/);
  assert.match(panel, /rows.slice\(0, 50\)/);
  assert.match(panel, /selected.content\['zh-CN'\]/);
  assert.match(panel, /onBeforeUnmount/);
});
