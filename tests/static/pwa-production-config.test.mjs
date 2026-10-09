import { readFileSync } from 'node:fs';
import assert from 'node:assert/strict';
import test from 'node:test';
const read = path => readFileSync(new URL(`../../${path}`, import.meta.url), 'utf8');

test('production PWA verification owns all three preview ports and cannot proxy to a backend', () => {
  const config = read('playwright.pwa.config.ts');
  const preview = read('vite.pwa-preview.config.mjs');
  assert.match(preview, /proxy: \{\}/);
  assert.match(preview, /host: '127.0.0.1'/);
  for (const port of [15173, 15174, 15175]) assert.ok(config.includes(`--port ${port} --strictPort`));
  assert.equal((config.match(/reuseExistingServer: false/g) ?? []).length, 3);
  assert.equal((config.match(/vite.pwa-preview.config.mjs/g) ?? []).length, 3);
  assert.match(config, /retries: 0/);
  assert.doesNotMatch(config, /DATABASE_URL|previewSupervisor|spawnSync/);
});

test('PWA CI verifies genuine production bundles independently of financial databases', () => {
  const workflow = read('.github/workflows/ci.yaml').split('  pwa-production-cache:')[1].split('  administration-entries:')[0];
  assert.match(workflow, /pnpm test:pwa/);
  assert.match(workflow, /prepare-ci-browser.mjs/);
  assert.doesNotMatch(workflow, /postgres|DATABASE_URL/);
  const scripts = JSON.parse(read('package.json')).scripts;
  assert.equal(scripts['test:pwa'], 'pnpm build && playwright test --config playwright.pwa.config.ts');
});
