import { test } from 'node:test';
import assert from 'node:assert/strict';
import { existsSync, readFileSync } from 'node:fs';

const workflow = readFileSync(new URL('../../.github/workflows/ci.yaml', import.meta.url), 'utf8');
const jobs = [...workflow.split('\njobs:\n')[1].matchAll(/^  ([a-z][a-z-]*):\n([\s\S]*?)(?=^  [a-z][a-z-]*:|$(?![\s\S]))/gm)];

test('push and PR run quick checks; manual dispatch enables every existing regression job', () => {
  assert.match(workflow, /on:\n  push:\n  pull_request:\n  workflow_dispatch:/);
  assert.match(workflow, /run-name: .*'Full regression'.*'Quick checks'/);
  assert.equal(jobs.length, 21);
  const quick = ['frontend', 'api-contract', 'backend-quick'];
  assert.deepEqual(jobs.filter(([, , body]) => !/    if:/.test(body)).map(([, name]) => name).sort(), [...quick].sort());
  for (const [, name, body] of jobs) {
    if (quick.includes(name)) continue;
    assert.match(body, /if: github\.event_name == 'workflow_dispatch'/, name);
    assert.doesNotMatch(body, /needs: changes/, name);
  }
  let expanded = jobs.length;
  for (const [, , body] of jobs) {
    const dimensions = [...body.matchAll(/^        \w+: \[([^\]]+)\]/gm)];
    if (dimensions.length) expanded += dimensions.reduce((total, [, values]) => total * values.split(',').length, 1) - 1;
  }
  assert.equal(expanded, 39);
  assert.doesNotMatch(workflow, /business_changed|classify-ci-changes/);
  assert.equal(existsSync(new URL('../../scripts/classify-ci-changes.mjs', import.meta.url)), false);
});

test('new quick runs cancel old quick runs but never share a concurrency group with full regression', () => {
  assert.match(workflow, /group: \$\{\{ github\.workflow \}\}-\$\{\{ github\.ref \}\}-\$\{\{ github\.event_name == 'workflow_dispatch' \}\}/);
  assert.match(workflow, /cancel-in-progress: \$\{\{ github\.event_name != 'workflow_dispatch' \}\}/);
});

test('backend quick checks build and vet all packages without claiming database or race coverage', () => {
  const backend = jobs.find(([, name]) => name === 'backend-quick')?.[2];
  assert.ok(backend);
  assert.match(backend, /TEST_DATABASE_URL: ''/);
  assert.match(backend, /go vet \.\/\.\.\./);
  assert.match(backend, /go test -count=1 -timeout=2m \.\/\.\.\./);
  assert.match(backend, /go build \.\/cmd\/platform/);
  assert.doesNotMatch(backend, /services:|go test -race/);
  assert.match(backend, /not database\/financial acceptance/);
});
