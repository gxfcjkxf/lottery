import { readFileSync } from 'node:fs';
import assert from 'node:assert/strict';
import test from 'node:test';

const read = path => readFileSync(new URL(`../../${path}`, import.meta.url), 'utf8');
test('platform report metrics all have explicit English and Chinese labels', () => {
  const api = read('platform-web/src/reports-api.ts');
  const panel = read('platform-web/src/ReportsPanel.vue');
  const labels = read('platform-web/src/report-labels.ts');
  const fieldBlock = api.split('export const REPORT_FIELDS')[1].split('export const REPORT_GROUPS')[0];
  const metrics = [...new Set([...fieldBlock.matchAll(/'([a-z_]+)'/g)].map(match => match[1]))];
  assert.ok(metrics.length > 30);
  for (const metric of metrics) assert.match(labels, new RegExp(`${metric}: \\['[^']+', '[^']+'\\]`));
  assert.doesNotMatch(panel, /\?\? key|Number\(.*points|parseFloat/);
  assert.match(panel, /Unsupported report label/);
});

test('report review accounts are created explicitly and retain ordinary authenticated worker sessions', () => {
  const workflow = read('.github/workflows/ci.yaml');
  for (const viewport of ['desktop1440', 'mobile360']) assert.ok(workflow.includes(`create-admin --username review_reports_${viewport} --super`));
  const spec = read('tests/review/platform-reports.spec.ts');
  const fixture = read('tests/review/platform-fixture.ts');
  assert.match(spec, /platformAccountPrefix: 'review_reports'/);
  assert.match(fixture, /identifier: `\$\{platformAccountPrefix\}_\$\{project\}`/);
  assert.match(fixture, /platformAccountPrefix: \['review_platform'/);
  assert.doesNotMatch(spec + fixture, /auth_rate_limits|X-Forwarded-For|DELETE FROM/);
});

test('platform CSV download is gated by verification and a frozen confirmed complete-filter request', () => {
  const client = read('platform-web/src/report-export-api.ts');
  const panel = read('platform-web/src/ReportsPanel.vue');
  assert.match(client, /normalizeReportQuery\(kind/);
  assert.match(client, /crypto\.subtle\.digest\('SHA-256'/);
  assert.match(client, /response\.status !== 200/);
  assert.match(client, /MAX_GROUPS = 10_000/);
  assert.match(client, /MAX_BYTES = 4 \* 1024 \* 1024/);
  assert.match(panel, /Object\.freeze\(\{ brandId: props\.brandId/);
  assert.ok(panel.indexOf('await exportApi.export') < panel.indexOf('URL.createObjectURL'));
  assert.match(panel, /request !== generation \|\| props\.brandId !== frozen\.brandId/);
  assert.match(panel, /limit: _limit, offset: _offset, \.\.\.query/);
});

test('CSV review uses explicit isolated export accounts without changing old viewer grants', () => {
  const workflow = read('.github/workflows/ci.yaml');
  for (const viewport of ['desktop1440', 'mobile360']) assert.ok(workflow.includes(`create-admin --username review_exports_${viewport} --super`));
  const spec = read('tests/review/platform-report-exports.spec.ts');
  assert.match(spec, /platformAccountPrefix: 'review_exports'/);
  assert.match(spec, /CSV audit must already be committed/);
  assert.doesNotMatch(spec, /auth_rate_limits|X-Forwarded-For|DELETE FROM/);
});
