import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, readFileSync, realpathSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';
import {
  initializeReportArchivePolicyBrowser,
  runReportArchivePolicyCommand,
  validateReportArchivePolicyFixtureEnv,
} from '../../scripts/init-report-archive-policy-browser.mjs';

const tempDirectory = mkdtempSync(join(tmpdir(), 'report-archive-policy-browser-'));
const authKeyFile = join(tempDirectory, 'auth.key');
writeFileSync(authKeyFile, 'synthetic-local-auth-key');
after(() => rmSync(tempDirectory, { recursive: true, force: true }));

const database = viewport => `lottery_archive_policy_browser_${viewport}`;
const emptySchema = viewport => `${database(viewport)}|lottery_test|0\n`;
function environment(viewport = 'desktop', port = '55432') {
  return {
    APP_ENV: 'test',
    REPORT_ARCHIVE_POLICY_VIEWPORT: viewport,
    REPORT_ARCHIVE_POLICY_FIXTURE_CONFIRM: 'owned_synthetic_database',
    DATABASE_URL: `postgres://lottery_test@127.0.0.1:${port}/${database(viewport)}?sslmode=disable`,
    PLATFORM_BIN: process.execPath,
    POSTGRES_PSQL_BIN: process.execPath,
    AUTH_KEY_FILE: authKeyFile,
    TEST_ARCHIVE_POLICY_ADMIN_USERNAME: `archive_policy_browser_${viewport}`,
    TEST_ARCHIVE_POLICY_ADMIN_PASSWORD: 'archive-policy-static-synthetic-admin-password',
  };
}

test('accepts only desktop/mobile owned loopback databases on supported ports', () => {
  for (const viewport of ['desktop', 'mobile']) {
    for (const port of ['5432', '55432']) {
      const config = validateReportArchivePolicyFixtureEnv(environment(viewport, port));
      assert.equal(config.database, database(viewport));
      assert.equal(config.username, `archive_policy_browser_${viewport}`);
      assert.equal(config.databaseURL, environment(viewport, port).DATABASE_URL);
      assert.equal(config.authKeyFile, realpathSync(authKeyFile));
      assert.ok(Object.isFrozen(config));
    }
  }
});

test('rejects remote, privileged, encoded, mismatched and extra-parameter database URLs', () => {
  const base = environment().DATABASE_URL;
  for (const DATABASE_URL of [
    base.replace('127.0.0.1', 'localhost'), base.replace('127.0.0.1', '192.0.2.1'),
    base.replace('127.0.0.1', '[::1]'), base.replace('lottery_test@', 'postgres@'),
    base.replace('lottery_test@', 'lottery%5ftest@'), base.replace(':55432/', ':5433/'),
    base.replace('/lottery_archive_policy_browser_desktop?', '/postgres?'),
    base.replace('/lottery_archive_policy_browser_desktop?', '/lottery_archive_policy_browser_mobile?'),
    base.replace('browser_desktop?', 'browser%5fdesktop?'), base.replace('sslmode=disable', 'sslmode=require'),
    `${base}&host=remote.example`, `${base}&sslmode=disable`, `${base}#fragment`, `${base}\n`,
    base.replace('lottery_test@', 'lottery_test:secret@'),
  ]) assert.throws(() => validateReportArchivePolicyFixtureEnv({ ...environment(), DATABASE_URL }), /owned.*URL/i, DATABASE_URL);
});

test('rejects missing confirmation, replica overrides, bad admin credentials and invalid local paths before commands', () => {
  const changes = [
    { APP_ENV: 'production' }, { REPORT_ARCHIVE_POLICY_VIEWPORT: 'tablet' },
    { REPORT_ARCHIVE_POLICY_FIXTURE_CONFIRM: 'yes' },
    { DATABASE_READ_URL: 'postgres://remote/replica' }, { DATABASE_READ_URLS: '[]' },
    { TEST_ARCHIVE_POLICY_ADMIN_USERNAME: 'other' }, { TEST_ARCHIVE_POLICY_ADMIN_PASSWORD: undefined },
    { TEST_ARCHIVE_POLICY_ADMIN_PASSWORD: 'short' }, { TEST_ARCHIVE_POLICY_ADMIN_PASSWORD: 'x'.repeat(129) },
    { TEST_ARCHIVE_POLICY_ADMIN_PASSWORD: `${'x'.repeat(16)}\u0000` },
    { PLATFORM_BIN: 'platform' }, { PLATFORM_BIN: '/no/such/platform' },
    { POSTGRES_PSQL_BIN: undefined }, { POSTGRES_PSQL_BIN: '' },
    { POSTGRES_PSQL_BIN: 'psql --command=unsafe' }, { POSTGRES_PSQL_BIN: '/no/such/psql' },
    { AUTH_KEY_FILE: undefined }, { AUTH_KEY_FILE: join(tempDirectory, 'missing.key') },
    { AUTH_KEY_FILE: tempDirectory }, { AUTH_KEY_FILE: 'file:///tmp/auth.key' },
  ];
  for (const changed of changes) {
    let calls = 0;
    assert.throws(() => initializeReportArchivePolicyBrowser({
      env: { ...environment(), ...changed }, runCommand: () => { calls++; return emptySchema('desktop'); },
    }));
    assert.equal(calls, 0);
  }
});

test('checks the empty owned public schema before migrate, seed and Harbor admin bootstrap', () => {
  const env = {
    ...environment('desktop', '5432'),
    BOOTSTRAP_ADMIN_PASSWORD: 'inherited-private-secret',
    PGHOST: 'remote.example', PGOPTIONS: '-c search_path=unsafe',
  };
  const calls = [];
  const result = initializeReportArchivePolicyBrowser({
    env,
    runCommand: (command, args, options) => {
      calls.push({ command, args, ...options });
      return calls.length === 1 ? emptySchema('desktop') : '';
    },
  });
  assert.deepEqual(result, {
    viewport: 'desktop', database: database('desktop'), username: 'archive_policy_browser_desktop',
  });
  assert.equal(calls.length, 4);
  assert.equal(calls[0].command, process.execPath);
  assert.deepEqual(calls[0].args.slice(0, 6), [
    '-X', '-At', '--no-password', '--set=ON_ERROR_STOP=1', '--dbname', env.DATABASE_URL,
  ]);
  assert.match(calls[0].args[7], /^SELECT current_database\(\), current_user, count\(\*\)/);
  assert.equal(calls[0].timeoutMs, 60_000);
  assert.deepEqual(calls.slice(1).map(call => call.args), [
    ['migrate'], ['seed'],
    ['create-admin', '--username', 'archive_policy_browser_desktop', '--brand', 'harbor'],
  ]);
  for (const call of calls) {
    assert.ok(Object.isFrozen(call.env));
    assert.equal(call.env.APP_ENV, 'test');
    assert.equal(call.env.DATABASE_URL, env.DATABASE_URL);
    assert.equal(call.env.AUTH_KEY_FILE, realpathSync(authKeyFile));
    for (const name of [
      'TEST_ARCHIVE_POLICY_ADMIN_PASSWORD', 'PGHOST', 'PGOPTIONS', 'DATABASE_READ_URL',
      'DATABASE_READ_URLS',
    ]) assert.equal(call.env[name], undefined);
    assert.ok(!call.args.includes(env.TEST_ARCHIVE_POLICY_ADMIN_PASSWORD));
  }
  assert.equal(calls[3].env.BOOTSTRAP_ADMIN_PASSWORD, env.TEST_ARCHIVE_POLICY_ADMIN_PASSWORD);
  assert.equal(calls[1].env.BOOTSTRAP_ADMIN_PASSWORD, undefined);
  assert.equal(calls[2].env.BOOTSTRAP_ADMIN_PASSWORD, undefined);
});

test('nonempty or mismatched database and command failures stop without secret output or retries', () => {
  for (const output of [
    `${database('desktop')}|lottery_test|1\n`, `${database('mobile')}|lottery_test|0\n`,
    `${database('desktop')}|postgres|0\n`, '', undefined,
  ]) {
    let calls = 0;
    assert.throws(() => initializeReportArchivePolicyBrowser({
      env: environment(), runCommand: () => { calls++; return output; },
    }), /no public tables/);
    assert.equal(calls, 1);
  }
  for (const failAt of [1, 2, 3, 4]) {
    let calls = 0;
    assert.throws(() => initializeReportArchivePolicyBrowser({
      env: environment(),
      runCommand: () => {
        if (++calls === failAt) throw new Error('archive-policy-password-sentinel');
        return calls === 1 ? emptySchema('desktop') : '';
      },
    }), error => /initialization failed at/.test(error.message) && !error.message.includes('sentinel'));
    assert.equal(calls, failAt);
  }
  const source = readFileSync(new URL('../../scripts/init-report-archive-policy-browser.mjs', import.meta.url), 'utf8');
  assert.doesNotMatch(source, /INSERT\s+INTO|UPDATE\s+\w+\s+SET|DELETE\s+FROM|TRUNCATE|DROP\s+TABLE|shell:\s*true/i);
});

test('command wrapper bounds execution and returns redacted failures', () => {
  assert.equal(runReportArchivePolicyCommand(process.execPath, ['-e', 'process.stdout.write("ok")'], { timeoutMs: 1_000 }), 'ok');
  assert.throws(() => runReportArchivePolicyCommand(process.execPath, [
    '-e', 'process.stderr.write("private-sentinel"); process.exit(9)',
  ], { timeoutMs: 1_000 }), error => error.message === 'Report archive policy browser command failed');
  assert.throws(() => runReportArchivePolicyCommand('/no/such/command', [], { timeoutMs: 1_000 }), /command failed/);
  assert.throws(() => runReportArchivePolicyCommand(process.execPath, ['-e', 'setTimeout(() => {}, 5000)'], { timeoutMs: 50 }), /command failed/);
  for (const timeoutMs of [0, -1, 1.5, 240_001, Infinity, NaN]) {
    assert.throws(() => runReportArchivePolicyCommand(process.execPath, [], { timeoutMs }), /timeout/);
  }
});

test('CLI refuses unowned execution and never echoes configured secrets', () => {
  const result = spawnSync(process.execPath, [new URL('../../scripts/init-report-archive-policy-browser.mjs', import.meta.url).pathname], {
    env: { ...environment(), REPORT_ARCHIVE_POLICY_FIXTURE_CONFIRM: '', AUTH_KEY: 'private-auth-key-sentinel' },
    encoding: 'utf8', timeout: 1_000,
  });
  assert.equal(result.status, 1);
  assert.equal(result.stdout, '');
  assert.match(result.stderr, /explicitly owned test database/);
  assert.doesNotMatch(result.stderr, /sentinel|archive-policy-static|postgres:\/\//);
});
