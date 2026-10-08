import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const workflow = readFileSync(new URL('../../.github/workflows/ci.yaml', import.meta.url), 'utf8');
const config = readFileSync(new URL('../../playwright.report-archive-policy.config.ts', import.meta.url), 'utf8');

test('policy browser CI uses normal bootstrap in separate owned viewport databases', () => {
  const job = /^  report-archive-policy-browser:\n([\s\S]*?)(?=^  [a-z][a-z-]*:\n|(?![\s\S]))/m.exec(workflow)?.[1];
  assert.ok(job);
  assert.match(job, /viewport: \[desktop, mobile\]/);
  assert.match(job, /POSTGRES_DB: lottery_archive_policy_browser_\$\{\{ matrix\.viewport \}\}/);
  assert.match(job, /REPORT_ARCHIVE_POLICY_FIXTURE_CONFIRM: owned_synthetic_database/);
  assert.match(job, /export PLATFORM_BIN="\$PWD\/\.local\/archive-policy-platform"/);
  assert.match(job, /node scripts\/init-report-archive-policy-browser\.mjs/);
  assert.match(job, /pnpm exec playwright test --config=playwright\.report-archive-policy\.config\.ts --project=\$\{\{ matrix\.viewport \}\} --workers=1 --retries=0/);
  assert.match(job, /stats\.skipped !== 0 \|\| stats\.unexpected !== 0 \|\| stats\.flaky !== 0 \|\| stats\.expected !== 1/);
  assert.doesNotMatch(job, /browserfixture|continue-on-error|\.local\/archive-policy-platform worker/);
  assert.match(config, /testMatch: 'report-archive-policy\.spec\.ts'/);
  assert.match(config, /retries: 0/);
});

test('policy browser verifies genuine lost ACK, later config, original replay and unchanged economics', () => {
  const spec = readFileSync(new URL('../browser/report-archive-policy.spec.ts', import.meta.url), 'utf8');
  assert.match(spec, /route\.fetch\(\)/);
  assert.match(spec, /route\.abort\('failed'\)/);
  assert.match(spec, /writes\[1\]\)\.toEqual\(writes\[0\]\)/);
  assert.match(spec, /expect\(await economics\(\)\)\.toEqual\(before\)/);
  assert.match(spec, /version: 3, daily_enabled: false, monthly_enabled: false/);
  assert.match(spec, /getByTestId\('archive-policy-replay-check'\)\.check\(\)/);
  assert.doesNotMatch(spec, /test\.skip|force:\s*true/);
});
