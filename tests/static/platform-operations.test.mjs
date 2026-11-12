import { readFileSync } from 'node:fs';
import assert from 'node:assert/strict';
import test from 'node:test';

const read = path => readFileSync(new URL(`../../${path}`, import.meta.url), 'utf8');

test('monthly retention replaces daily and monthly snapshot entries', () => {
  const api=read('platform-web/src/data-retention-api.ts'),app=read('platform-web/src/App.vue');
  assert.match(api,/X-Data-Retention-Actor-ID/);assert.match(api,/data-retention-policy/);
  assert.match(app,/<DataRetentionPanel/);assert.match(app,/<NotificationsPanel/);
  assert.doesNotMatch(app,/ArchivesPanel|ArchiveTasksPanel|archive-tasks/);
});
test('retention uses shared lists, exact counts and no export entry', () => {
  const panel=read('platform-web/src/DataRetentionPanel.vue');
  assert.match(panel,/<ManagementList/);assert.match(panel,/<ManagementTable/);assert.match(panel,/<TablePagination/);
  assert.match(panel,/BigInt\(c\[key\]\)/);assert.match(panel,/onBeforeUnmount/);
  assert.doesNotMatch(panel,/api\.download|URL\.createObjectURL|\.download\s*=|v-html|parseFloat/);
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
