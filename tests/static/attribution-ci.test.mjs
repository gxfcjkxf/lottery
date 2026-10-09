import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const workflow = readFileSync(new URL('../../.github/workflows/ci.yaml', import.meta.url), 'utf8');
const config = readFileSync(new URL('../../playwright.attribution-reports.config.ts', import.meta.url), 'utf8');
const spec = readFileSync(new URL('../browser/attribution-reports.spec.ts', import.meta.url), 'utf8');

test('attribution CI owns one normal-bootstrap database per viewport and rejects skipped results', () => {
  const job = /^  attribution-reports-browser:\n([\s\S]*?)(?=^  [a-z][a-z-]*:\n|(?![\s\S]))/m.exec(workflow)?.[1];
  assert.ok(job);
  assert.match(job, /viewport: \[desktop, mobile\]/);
  assert.match(job, /POSTGRES_DB: lottery_attribution_browser_\$\{\{ matrix\.viewport \}\}/);
  assert.match(job, /PLATFORM_BIN: \$\{\{ github\.workspace \}\}\/\.local\/attribution-platform/);
  assert.match(job, /POSTGRES_PSQL_BIN: \/usr\/bin\/psql/);
  assert.match(job, /node scripts\/init-attribution-browser\.mjs/);
  assert.match(job, /\.local\/attribution-platform worker > attribution-reports-worker\.log 2>&1 &/);
  assert.match(job, /pnpm exec playwright test --config=playwright\.attribution-reports\.config\.ts --project=\$\{\{ matrix\.viewport \}\} --workers=1 --retries=0/);
  assert.match(job, /stats\.skipped !== 0 \|\| stats\.unexpected !== 0 \|\| stats\.flaky !== 0 \|\| stats\.expected !== 1/);
  assert.doesNotMatch(job, /browserfixture|continue-on-error/);
  assert.match(config, /testMatch: 'attribution-reports\.spec\.ts'/);
  assert.match(config, /retries: 0/);
  assert.match(config, /pnpm dev:admin/);
});

test('owned attribution browser checks genuine scopes, full CSV and unchanged financial state', () => {
  assert.match(spec, /real attribution reports preserve saved agent scope and export unique order totals/);
  assert.match(spec, /agent_scope: 'downline'/);
  assert.match(spec, /createBettingGame/);
  assert.match(spec, /createHash\('sha256'\)/);
  assert.match(spec, /publicAttributionFields/);
  assert.doesNotMatch(spec, /test\.skip|force:\s*true/);
});
