import { test } from 'node:test';
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import {
  initializeReportArchiveTasksBrowser,
  readReportArchiveTasksBrowserConfiguration,
  runReportArchiveTasksCommand,
} from '../../scripts/init-report-archive-tasks-browser.mjs';

const database = viewport => `lottery_archive_tasks_browser_${viewport}`;
function environment(viewport = 'desktop') {
  return {
    APP_ENV: 'test', REPORT_ARCHIVE_TASKS_VIEWPORT: viewport,
    REPORT_ARCHIVE_TASKS_FIXTURE_CONFIRM: 'owned_synthetic_database',
    DATABASE_URL: `postgres://lottery_test@127.0.0.1:55432/${database(viewport)}?sslmode=disable`,
    PLATFORM_BIN: process.execPath,
    POSTGRES_PSQL_BIN: process.execPath,
    ARCHIVE_TASKS_FIXTURE_BIN: process.execPath,
    TEST_ARCHIVE_TASKS_ADMIN_USERNAME: `archive_tasks_browser_${viewport}`,
    TEST_ARCHIVE_TASKS_ADMIN_PASSWORD: 'owned-synthetic-archive-admin-password',
  };
}
const emptySchema = viewport => `${database(viewport)}|lottery_test|0\n`;
const fixtureResult = {
  task_id: '0199a000-0000-7000-8000-000000000099',
  brand_id: '0199a000-0000-7000-8000-000000000002',
  task_version: 2, state: 'failed', attempt_count: 1, archive_id: null,
  archive_count: '0', ledger_count: '0', total_points: '0',
};

test('fixture URL guard accepts only the selected owned desktop/mobile loopback database', () => {
  for (const viewport of ['desktop', 'mobile']) {
    for (const host of ['127.0.0.1', 'localhost']) {
      for (const port of ['5432', '55432']) {
        for (const scheme of ['postgres', 'postgresql']) {
          const env = { ...environment(viewport), DATABASE_URL: `${scheme}://lottery_test@${host}:${port}/${database(viewport)}?sslmode=disable` };
          const config = readReportArchiveTasksBrowserConfiguration(env);
          assert.equal(config.viewport, viewport);
          assert.equal(config.database, database(viewport));
          assert.equal(config.username, `archive_tasks_browser_${viewport}`);
          assert.equal(config.psqlBin, process.execPath);
          assert.ok(Object.isFrozen(config));
        }
      }
    }
  }
  const base = environment();
  const url = base.DATABASE_URL;
  for (const DATABASE_URL of [
    url.replace('127.0.0.1', 'db.example'), url.replace('lottery_test@', 'postgres@'),
    url.replace('lottery_test@', 'lottery%5ftest@'), url.replace('lottery_test@', 'lottery_test:secret@'),
    url.replace(':55432/', ':5433/'), url.replace(':55432/', '/'), url.replace(database('desktop'), 'postgres'),
    url.replace('browser_desktop?', 'browser_mobile?'), url.replace(database('desktop'), `${database('desktop')}_old`),
    `${url}#fragment`, `${url}&sslmode=disable`, `${url}&host=remote`, `${url}&`,
    url.replace('sslmode=disable', ''), url.replace('postgres:', 'http:'), ` ${url}`, `${url}\n`,
  ]) {
    assert.throws(() => readReportArchiveTasksBrowserConfiguration({ ...base, DATABASE_URL }), /owned.*URL/i, DATABASE_URL);
  }
});

test('configuration and executable validation reject unsafe runs before any command', () => {
  for (const changed of [
    { APP_ENV: 'production' }, { REPORT_ARCHIVE_TASKS_VIEWPORT: 'tablet' },
    { REPORT_ARCHIVE_TASKS_FIXTURE_CONFIRM: '' }, { DATABASE_READ_URL: ' ' }, { DATABASE_READ_URLS: '[]' },
    { PLATFORM_BIN: 'platform' }, { ARCHIVE_TASKS_FIXTURE_BIN: 'fixture' },
    { POSTGRES_PSQL_BIN: undefined }, { POSTGRES_PSQL_BIN: '' },
    { PLATFORM_BIN: '/missing/platform' }, { ARCHIVE_TASKS_FIXTURE_BIN: '/missing/fixture' },
    { POSTGRES_PSQL_BIN: '/missing/psql' }, { TEST_ARCHIVE_TASKS_ADMIN_USERNAME: 'other' },
    { TEST_ARCHIVE_TASKS_ADMIN_PASSWORD: undefined }, { TEST_ARCHIVE_TASKS_ADMIN_PASSWORD: 'short' },
    { TEST_ARCHIVE_TASKS_ADMIN_PASSWORD: 'x'.repeat(129) },
  ]) {
    let calls = 0;
    assert.throws(() => initializeReportArchiveTasksBrowser({ env: { ...environment(), ...changed }, runCommand: () => { calls++; return emptySchema('desktop'); } }));
    assert.equal(calls, 0);
  }
});

test('empty public schema precedes normal migrations, Harbor bootstrap and fixture prepare', () => {
  const env = { ...environment(), PGHOST: 'remote.invalid', DATABASE_READ_URL: '', BOOTSTRAP_ADMIN_PASSWORD: 'inherited-secret' };
  const calls = [];
  const result = initializeReportArchiveTasksBrowser({
    env,
    runCommand: (command, args, options) => {
      calls.push({ command, args, ...options });
      if (calls.length === 1) return emptySchema('desktop');
      if (calls.length === 5) return JSON.stringify(fixtureResult);
      return '';
    },
  });
  assert.deepEqual(result, fixtureResult);
  assert.equal(calls.length, 5);
  assert.deepEqual(calls[0].args.slice(0, 6), ['-X', '-At', '--no-password', '--set=ON_ERROR_STOP=1', '--dbname', env.DATABASE_URL]);
  assert.match(calls[0].args[7], /n\.nspname = 'public'/);
  assert.deepEqual(calls.slice(1, 4).map(call => call.args), [
    ['migrate'], ['seed'], ['create-admin', '--username', 'archive_tasks_browser_desktop', '--brand', 'harbor'],
  ]);
  assert.deepEqual(calls[4].args, ['prepare']);
  for (const call of calls) {
    assert.ok(Object.isFrozen(call.env));
    assert.equal(call.env.APP_ENV, 'test');
    assert.equal(call.env.DATABASE_URL, env.DATABASE_URL);
    for (const key of ['PGHOST', 'DATABASE_READ_URL', 'DATABASE_READ_URLS', 'TEST_ARCHIVE_TASKS_ADMIN_PASSWORD']) assert.equal(call.env[key], undefined);
    assert.ok(!call.args.includes(env.TEST_ARCHIVE_TASKS_ADMIN_PASSWORD));
  }
  assert.equal(calls[3].env.BOOTSTRAP_ADMIN_PASSWORD, env.TEST_ARCHIVE_TASKS_ADMIN_PASSWORD);
  assert.equal(calls[4].env.BOOTSTRAP_ADMIN_PASSWORD, undefined);
  assert.equal(calls[4].env.TEST_ARCHIVE_TASKS_ADMIN_USERNAME, 'archive_tasks_browser_desktop');
});

test('nonempty or mismatched database stops before migrations and command failures redact output', () => {
  for (const output of [
    `${database('desktop')}|lottery_test|1\n`, `${database('mobile')}|lottery_test|0\n`,
    `${database('desktop')}|postgres|0\n`, '', undefined,
  ]) {
    let calls = 0;
    assert.throws(() => initializeReportArchiveTasksBrowser({ env: environment(), runCommand: () => { calls++; return output; } }), /no public tables/);
    assert.equal(calls, 1);
  }
  for (const failAt of [1, 2, 3, 4, 5]) {
    let calls = 0;
    assert.throws(() => initializeReportArchiveTasksBrowser({ env: environment(), runCommand: () => {
      if (++calls === failAt) throw new Error('password-sentinel');
      return calls === 1 ? emptySchema('desktop') : calls === 5 ? JSON.stringify(fixtureResult) : '';
    } }), error => /failed at/.test(error.message) && !error.message.includes('sentinel'));
    assert.equal(calls, failAt);
  }
  assert.throws(() => initializeReportArchiveTasksBrowser({ env: environment(), runCommand: (_cmd, _args, options) => {
    return options.env === undefined ? '' : (options.timeoutMs === 60_000 ? emptySchema('desktop') : JSON.stringify({ ...fixtureResult, state: 'completed' }));
  } }), /expected failed retry task/);
});

test('command wrapper bounds execution and CLI rejects unacknowledged setup without exposing secrets', () => {
  assert.equal(runReportArchiveTasksCommand(process.execPath, ['-e', 'process.stdout.write("ok")'], { timeoutMs: 1_000 }), 'ok');
  assert.throws(() => runReportArchiveTasksCommand(process.execPath, ['-e', 'process.stderr.write("private-sentinel"); process.exit(9)'], { timeoutMs: 1_000 }), /command failed/);
  assert.throws(() => runReportArchiveTasksCommand(process.execPath, ['-e', 'setTimeout(() => {}, 3000)'], { timeoutMs: 30 }), /command failed/);
  for (const timeoutMs of [0, -1, 1.5, 240_001, Infinity, NaN]) assert.throws(() => runReportArchiveTasksCommand(process.execPath, [], { timeoutMs }), /timeout/);
  const result = spawnSync(process.execPath, [new URL('../../scripts/init-report-archive-tasks-browser.mjs', import.meta.url).pathname], {
    env: { ...environment(), REPORT_ARCHIVE_TASKS_FIXTURE_CONFIRM: '', AUTH_KEY: 'private-auth-sentinel' }, encoding: 'utf8', timeout: 1_000,
  });
  assert.equal(result.status, 1);
  assert.equal(result.stdout, '');
  assert.match(result.stderr, /explicitly owned test database/);
  assert.doesNotMatch(result.stderr, /sentinel|owned-synthetic|postgres:\/\//);
});
