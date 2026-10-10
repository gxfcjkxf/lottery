import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { documentationOnly } from '../../scripts/classify-ci-changes.mjs';

test('only Markdown and the handover assertion qualify for lighter CI', () => {
  assert.equal(documentationOnly(['README.md', 'docs/05-ui-spec.md', 'tests/static/current-handover.test.mjs']), true);
  for (const path of ['backend/internal/points/store.go', 'backend/migrations/0001_baseline.up.sql', 'admin-web/src/App.vue', 'user-web/src/Page.vue', 'platform-web/src/access-api.ts', 'shared/src/auth.ts', 'scripts/classify-ci-changes.mjs', 'docs/openapi.json', 'docs/openapi/lottery.mjs', 'tests/browser/auth.spec.ts', 'tests/static/ci-change-scope.test.mjs', '.github/workflows/ci.yaml', 'pnpm-lock.yaml']) {
    assert.equal(documentationOnly(['README.md', path]), false, path);
  }
  assert.equal(documentationOnly([]), false);
  const source = readFileSync(new URL('../../scripts/classify-ci-changes.mjs', import.meta.url), 'utf8');
  // Moving a source file to a documentation path must still include its deletion.
  assert.match(source, /'diff', '--no-renames', '--name-only', '-z'/);
});

test('all heavy jobs require change classification while contract and frontend checks always run', () => {
  const workflow = readFileSync(new URL('../../.github/workflows/ci.yaml', import.meta.url), 'utf8');
  const jobs = [...workflow.matchAll(/^  ([a-z][a-z-]*):\n([\s\S]*?)(?=^  [a-z][a-z-]*:|$(?![\s\S]))/gm)];
  assert.equal(jobs.length, 21);
  for (const [, name, body] of jobs) {
    if (['changes', 'frontend', 'api-contract'].includes(name)) {
      assert.doesNotMatch(body, /needs\.changes\.outputs\.business_changed/);
    } else {
      assert.match(body, /needs: changes/, name);
      assert.match(body, /if: needs\.changes\.outputs\.business_changed == 'true'/, name);
    }
  }
  assert.match(workflow, /fetch-depth: 0/);
  assert.match(workflow, /github\.event\.pull_request\.base\.sha/);
  assert.match(workflow, /github\.event\.before/);
  assert.match(workflow, /run: pnpm test\n/);
});
