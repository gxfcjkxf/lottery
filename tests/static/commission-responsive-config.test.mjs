import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

test('commission responsive acceptance is serial and preserves real workflow file order', () => {
  const config = readFileSync(new URL('../../playwright.commission-responsive.config.ts', import.meta.url), 'utf8');
  const runner = readFileSync(new URL('../../scripts/run-commission-responsive.mjs', import.meta.url), 'utf8');
  assert.match(config, /width: 768, height: 1024/);
  assert.match(config, /width: 1024, height: 768/);
  assert.match(config, /workers: 1/);
  assert.match(config, /retries: 0/);
  assert.match(config, /fullyParallel: false/);
  assert.equal((config.match(/reuseExistingServer: false/g) ?? []).length, 3);
  assert.match(config, /isAbsolute\(executablePath\)/);
  assert.match(config, /existsSync\(executablePath\)/);
  assert.match(config, /COMMISSION_FIXTURE_CONFIRM !== 'owned_synthetic_database'/);
  const files = ['cycles', 'payments', 'adjustments', 'reports', 'analysis'].map(name => `commission-${name}.spec.ts`);
  const positions = files.map(file => runner.indexOf(file));
  assert.ok(positions.every(position => position >= 0));
  assert.deepEqual(positions, [...positions].sort((a, b) => a - b));
  assert.match(runner, /spawnSync/);
  assert.match(runner, /result.status !== 0/);
  assert.doesNotMatch(runner, /retry|--grep|catch/);
  for (const file of files) {
    const spec = readFileSync(new URL(`../browser/${file}`, import.meta.url), 'utf8');
    assert.doesNotMatch(spec, /project(?:\.name)?\s*===\s*['"]mobile['"]/);
  }
});
