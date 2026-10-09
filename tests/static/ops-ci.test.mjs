import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { assertInstallEnvironment, DOWNLOAD_TIMEOUT_MS, MAX_DOWNLOAD_BYTES, PROMTOOL_ARCHIVE, PROMTOOL_SHA256, PROMTOOL_URL, PROMTOOL_VERSION } from '../../scripts/install-promtool.mjs';

const workflow = readFileSync(new URL('../../.github/workflows/ci.yaml', import.meta.url), 'utf8');
const installer = readFileSync(new URL('../../scripts/install-promtool.mjs', import.meta.url), 'utf8');

test('newops-runtime uses the exact isolated PostgreSQL fixture and bounded normal verification commands', () => {
  const job = workflow.match(/^  newops-runtime:\n([\s\S]*?)(?=^  [\w-]+:\n|(?![\s\S]))/m)?.[1];
  assert.ok(job);
  assert.match(job, /timeout-minutes: 20/);
  assert.match(job, /image: postgres:17\.5-alpine/);
  assert.match(job, /POSTGRES_DB: lottery_ops_acceptance/);
  assert.match(job, /POSTGRES_USER: lottery_test/);
  assert.match(job, /POSTGRES_HOST_AUTH_METHOD: trust/);
  assert.match(job, /ports: \["127\.0\.0\.1:5432:5432"\]/);
  assert.match(job, /APP_ENV: test/);
  assert.match(job, /DATABASE_URL: postgres:\/\/lottery_test@127\.0\.0\.1:5432\/lottery_ops_acceptance\?sslmode=disable/);
  assert.match(job, /OPS_RUNTIME_FIXTURE_CONFIRM: owned_synthetic_database/);
  assert.match(job, /PLATFORM_BIN: \$\{\{ github\.workspace \}\}\/\.local\/ops-platform/);
  assert.match(job, /POSTGRES_PSQL_BIN: \/usr\/bin\/psql/);
  assert.match(job, /go -C backend build -o "\$PLATFORM_BIN" \.\/cmd\/platform/);
  assert.match(job, /pnpm install --frozen-lockfile/);
  assert.match(job, /node scripts\/install-promtool\.mjs/);
  assert.match(job, /node scripts\/verify-monitoring\.mjs/);
  assert.match(job, /node scripts\/verify-ops-runtime\.mjs/);
  assert.doesNotMatch(job, /playwright|chromium|pnpm test:e2e/);
});

test('installer pins official Prometheus release bytes and never extracts unrelated archive members', () => {
  assert.equal(PROMTOOL_VERSION, '3.15.0');
  assert.equal(PROMTOOL_ARCHIVE, 'prometheus-3.15.0.linux-amd64.tar.gz');
  assert.equal(PROMTOOL_SHA256, '2a542df32eac02ee17b9d844fb2aa1de00dafa5476579ba8a3ba862e9d572ea0');
  assert.equal(PROMTOOL_URL, 'https://github.com/prometheus/prometheus/releases/download/v3.15.0/prometheus-3.15.0.linux-amd64.tar.gz');
  assert.equal(MAX_DOWNLOAD_BYTES, 160 * 1024 * 1024);
  assert.equal(DOWNLOAD_TIMEOUT_MS, 180_000);
  assert.match(installer, /--no-same-owner', '--no-same-permissions', '--strip-components=1'.*member/);
  assert.match(installer, /AbortSignal\.timeout\(DOWNLOAD_TIMEOUT_MS\)/);
  assert.match(installer, /if \(!success\) rmSync\(owned, \{ recursive: true, force: true \}\)/);
  assert.match(installer, /PROMTOOL_BIN=\$\{binaryPath\}\\n/);
});

test('installer environment guard permits only Linux amd64 GitHub Actions or an explicit helper test override', () => {
  assert.doesNotThrow(() => assertInstallEnvironment({ platform: 'linux', arch: 'x64', githubActions: 'true' }));
  assert.doesNotThrow(() => assertInstallEnvironment({ platform: 'linux', arch: 'x64', githubActions: '', testOverride: '1' }));
  assert.throws(() => assertInstallEnvironment({ platform: 'darwin', arch: 'arm64', githubActions: 'true' }), /Linux amd64/);
  assert.throws(() => assertInstallEnvironment({ platform: 'linux', arch: 'x64', githubActions: '' }), /restricted to GitHub Actions/);
});

test('runtime acceptance rejects unowned databases before commands without printing credentials', () => {
  const script = new URL('../../scripts/verify-ops-runtime.mjs', import.meta.url);
  for (const env of [
    { APP_ENV: 'production', DATABASE_URL: 'postgres://private-marker-password@untrusted.example/customer' },
    { APP_ENV: 'test', OPS_RUNTIME_FIXTURE_CONFIRM: 'owned_synthetic_database', DATABASE_URL: 'postgres://lottery_test@127.0.0.1:55432/postgres?sslmode=disable' },
    { APP_ENV: 'test', OPS_RUNTIME_FIXTURE_CONFIRM: 'owned_synthetic_database', DATABASE_URL: 'postgres://lottery_test@127.0.0.1:55432/lottery_ops_acceptance?sslmode=disable', DATABASE_READ_URL: 'postgres://private-marker-password@untrusted.example/customer' },
  ]) {
    const child = spawnSync(process.execPath, [script.pathname], { env: { ...env, PLATFORM_BIN: '/private-marker-binary', POSTGRES_PSQL_BIN: '/private-marker-client' }, encoding: 'utf8', timeout: 3_000, shell: false });
    assert.equal(child.status, 1);
    assert.equal(child.stdout, '');
    assert.match(child.stderr, /exact explicitly owned loopback test database/);
    assert.doesNotMatch(child.stderr, /private-marker|untrusted\.example|ENOENT/);
  }
});
