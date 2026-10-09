import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
const workflow = readFileSync(new URL('../../.github/workflows/ci.yaml', import.meta.url), 'utf8');
const job = /^  browser:\n([\s\S]*)$/m.exec(workflow)?.[1];
test('general browser CI covers two isolated shards for each viewport without bypassing auth limits', () => {
  assert.ok(job);
  assert.match(job, /project: \[desktop, mobile\]/);
  assert.match(job, /shard: \[1, 2\]/);
  assert.match(job, /services:\n      postgres:/);
  assert.match(job, /APP_ENV: test/);
  assert.match(job, /pnpm test:e2e --project=\$\{\{ matrix\.project \}\} --grep-invert='real report archives recover a lost committed receipt and keep old versions immutable\|real automatic archive tasks recover the original retry receipt after worker completion\|real archive policy activation recovers the original receipt after a later configuration change\|real attribution reports preserve saved agent scope and export unique order totals\|real Harbor draw notices publish and correct immutable facts on desktop and mobile\|genuine brand audit filters and validated full CSV preserve finances on both layouts' --shard=\$\{\{ matrix\.shard \}\}\/2 --workers=1 --retries=0\s*$/m);
  assert.match(job, /name: browser-failure-traces-\$\{\{ matrix\.project \}\}-\$\{\{ matrix\.shard \}\}/);
  assert.doesNotMatch(job, /continue-on-error|DELETE FROM auth_rate_limits|TRUNCATE|X-Forwarded-For|disable.*limit/i);
});
