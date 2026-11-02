import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import test from 'node:test';
import { assertInstallEnvironment } from '../../scripts/install-promtool.mjs';

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
