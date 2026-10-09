import { readFileSync } from 'node:fs';
import { test } from 'node:test';
import assert from 'node:assert/strict';
const read = name => readFileSync(new URL(`../../${name}`, import.meta.url), 'utf8');
const config=read('playwright.audit-exports.config.ts'),ci=read('.github/workflows/ci.yaml'),spec=read('tests/browser/audit-exports.spec.ts');
test('audit export double layout CI uses owned normal initialization and bounded no-retry test',()=>{
  assert.match(config,/testMatch: 'audit-exports.spec.ts'/);assert.match(config,/timeout: 60_000/);assert.match(config,/retries: 0/);assert.match(config,/timezoneId: 'Asia\/Singapore'/);
  const job=ci.split('  audit-exports-browser:')[1]?.split('  draw-notifications-browser:')[0];assert.ok(job);
  assert.match(job,/viewport: \[desktop, mobile\]/);assert.match(job,/PLATFORM_BIN: \$\{\{ github\.workspace \}\}\/\.local\/audit-export-platform/);assert.match(job,/POSTGRES_PSQL_BIN: \/usr\/bin\/psql/);assert.match(job,/init-attribution-browser.mjs/);assert.match(job,/audit-export-platform serve/);assert.match(job,/stats.expected !== 1/);
  assert.match(ci,/--grep-invert=.*genuine brand audit filters and validated full CSV preserve finances on both layouts/);
  assert.doesNotMatch(spec,/test\.skip\(|force:\s*true|route\.fulfill|\b(?:INSERT|UPDATE|DELETE)\s+.*\b(?:INTO|SET|FROM)\b/i);
});
test('audit browser checks real scope, digest, immutable financial state, UTC and both languages',()=>{
  for(const fragment of ['owned_synthetic_database','lottery_attribution_browser_','readerContext','toBe(403)','download.createReadStream','createHash','x-audit-export-id','economicState()).toEqual(before)','From (UTC)','Resource UUID','admin-language','导出完整 CSV','document.documentElement.scrollWidth','pageErrors).toEqual([])'])assert.ok(spec.includes(fragment),fragment);
});
