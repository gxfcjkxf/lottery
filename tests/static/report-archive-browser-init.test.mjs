import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import {
  initializeReportArchiveBrowser,
  readReportArchiveBrowserConfiguration,
  runReportArchiveCommand,
} from '../../scripts/init-report-archive-browser.mjs';

const workflow = readFileSync(new URL('../../.github/workflows/ci.yaml', import.meta.url), 'utf8');
const source = readFileSync(new URL('../../scripts/init-report-archive-browser.mjs', import.meta.url), 'utf8');
const browserSpec = readFileSync(new URL('../browser/report-archives.spec.ts', import.meta.url), 'utf8');
const browserConfig = readFileSync(new URL('../../playwright.report-archives.config.ts', import.meta.url), 'utf8');
const job = /^  report-archives-browser:\n([\s\S]*?)(?=^  [a-z][a-z-]*:\n|(?![\s\S]))/m.exec(workflow)?.[1];
const generalBrowserJob = /^  browser:\n([\s\S]*)$/m.exec(workflow)?.[1];
const database = viewport => `lottery_archive_browser_${viewport}`;
const emptySchema = viewport => `${database(viewport)}|lottery_test|0\n`;
function environment(viewport = 'desktop') {
  return {
    APP_ENV: 'test', REPORT_ARCHIVE_VIEWPORT: viewport,
    REPORT_ARCHIVE_FIXTURE_CONFIRM: 'owned_synthetic_database',
    DATABASE_URL: `postgres://lottery_test@127.0.0.1:55432/${database(viewport)}?sslmode=disable`,
    PLATFORM_BIN: process.execPath,
    TEST_ARCHIVE_ADMIN_USERNAME: `archive_browser_${viewport}`,
    TEST_ARCHIVE_ADMIN_PASSWORD: 'archive-static-owned-harbor-admin-password',
  };
}

test('archive browser CI owns separate desktop/mobile databases and runs the real spec', () => {
  assert.ok(job, 'dedicated report-archives-browser job required');
  assert.match(job, /timeout-minutes: 20/);
  assert.match(job, /viewport: \[desktop, mobile\]/);
  assert.match(job, /POSTGRES_DB: lottery_archive_browser_\$\{\{ matrix\.viewport \}\}/);
  assert.match(job, /POSTGRES_USER: lottery_test/);
  assert.match(job, /POSTGRES_HOST_AUTH_METHOD: trust/);
  assert.match(job, /POSTGRES_INITDB_ARGS: "-c max_locks_per_transaction=256"/);
  assert.match(job, /ports: \["127\.0\.0\.1:5432:5432"\]/);
  assert.match(job, /DATABASE_URL: postgres:\/\/lottery_test@127\.0\.0\.1:5432\/lottery_archive_browser_\$\{\{ matrix\.viewport \}\}\?sslmode=disable/);
  assert.match(job, /APP_ENV: test/);
  assert.match(job, /REPORT_ARCHIVE_VIEWPORT: \$\{\{ matrix\.viewport \}\}/);
  assert.match(job, /REPORT_ARCHIVE_FIXTURE_CONFIRM: owned_synthetic_database/);
  assert.match(job, /TEST_ARCHIVE_ORIGIN: http:\/\/localhost:5174/);
  assert.match(job, /TEST_ARCHIVE_API_ORIGIN: http:\/\/127\.0\.0\.1:8080/);
  assert.match(job, /TEST_ARCHIVE_ADMIN_USERNAME: archive_browser_\$\{\{ matrix\.viewport \}\}/);
  assert.match(job, /^      TEST_ARCHIVE_ADMIN_PASSWORD: .+$/m);
  assert.match(job, /go -C backend build -o \.\.\/\.local\/archive-platform \.\/cmd\/platform/);
  assert.match(job, /\.local\/archive-platform generate-auth-key \.local\/auth\.key/);
  assert.match(job, /node scripts\/init-report-archive-browser\.mjs/);
  assert.match(job, /name: Prepare and verify Chromium\n        timeout-minutes: 7\n        run: node scripts\/prepare-ci-browser\.mjs/);
  assert.match(job, /--config=playwright\.report-archives\.config\.ts --project=\$\{\{ matrix\.viewport \}\} --workers=1 --retries=0/);
  assert.match(browserConfig, /testMatch: 'report-archives\.spec\.ts'/);
  assert.match(browserConfig, /retries: 0/);
  assert.match(browserConfig, /workers: 1/);
  assert.match(browserConfig, /\['json', \{ outputFile: '\.local\/report-archives-browser-report\.json' \}\]/);
  assert.doesNotMatch(job, /--reporter=/);
  assert.match(job, /stats\.skipped !== 0/);
  assert.match(job, /stats\.expected > 0/);
  assert.doesNotMatch(job, /POSTGRES_PASSWORD:|browserfixture|continue-on-error|--with-deps|--retries=([1-9]\d*)/);
  assert.match(generalBrowserJob, /pnpm test:e2e --project=\$\{\{ matrix\.project \}\} --grep-invert='real report archives recover a lost committed receipt and keep old versions immutable\|real automatic archive tasks recover the original retry receipt after worker completion\|real archive policy activation recovers the original receipt after a later configuration change\|real attribution reports preserve saved agent scope and export unique order totals\|real Harbor draw notices publish and correct immutable facts on desktop and mobile' --shard=\$\{\{ matrix\.shard \}\}\/2 --workers=1 --retries=0/);
  assert.ok(browserSpec.includes(String.raw`const allowedBrowserOrigin = /^http:\/\/localhost:(?:5174|15292)$/;`));
  assert.ok(browserSpec.includes(String.raw`const allowedAPIOrigin = /^http:\/\/127\.0\.0\.1:(?:8080|15291)$/;`));
  assert.match(browserSpec, /page\.locator\("\.archives"\)/);
  assert.match(browserSpec, /page\.getByTestId\("admin-language"\)/);
});

test('initializer accepts only the exact empty owned loopback database and known PostgreSQL user', () => {
  for (const viewport of ['desktop', 'mobile']) {
    for (const host of ['127.0.0.1', 'localhost']) {
      for (const port of ['5432', '55432']) {
        for (const scheme of ['postgres', 'postgresql']) {
          const config = readReportArchiveBrowserConfiguration({
            ...environment(viewport),
            DATABASE_URL: `${scheme}://lottery_test@${host}:${port}/${database(viewport)}?sslmode=disable`,
          });
          assert.equal(config.database, database(viewport));
          assert.equal(config.viewport, viewport);
          assert.equal(config.username, `archive_browser_${viewport}`);
          assert.equal(config.psqlBin, 'psql');
          assert.ok(Object.isFrozen(config));
        }
      }
    }
  }
  const url = environment().DATABASE_URL;
  for (const DATABASE_URL of [
    url.replace('127.0.0.1', 'remote.example'), url.replace('127.0.0.1', '[::1]'),
    url.replace('lottery_test@', 'postgres@'), url.replace('lottery_test@', 'lottery%5ftest@'),
    url.replace(':55432/', ':5433/'), url.replace(':55432/', '/'),
    url.replace('browser_desktop?', 'browser_mobile?'), url.replace(database('desktop'), 'postgres'),
    url.replace(database('desktop'), `${database('desktop')}_extra`), `${url}#fragment`,
    `${url}&sslmode=disable`, `${url}&host=remote.example`, `${url}&`,
    url.replace('sslmode=disable', ''), url.replace('postgres:', 'http:'),
    url.replace('lottery_test@', 'lottery_test:private-password@'), ` ${url}`, `${url}\n`,
  ]) assert.throws(() => readReportArchiveBrowserConfiguration({ ...environment(), DATABASE_URL }), /owned.*URL/i, DATABASE_URL);
});

test('unacknowledged, misconfigured, replicated or uncredentialed initialization stops before commands', () => {
  for (const changed of [
    { APP_ENV: 'production' }, { APP_ENV: 'development' }, { REPORT_ARCHIVE_VIEWPORT: '' },
    { REPORT_ARCHIVE_VIEWPORT: 'tablet' }, { REPORT_ARCHIVE_FIXTURE_CONFIRM: '' },
    { REPORT_ARCHIVE_FIXTURE_CONFIRM: 'yes' }, { DATABASE_READ_URL: environment().DATABASE_URL },
    { DATABASE_READ_URLS: '[]' }, { DATABASE_READ_URL: ' ' }, { PLATFORM_BIN: undefined },
    { PLATFORM_BIN: 'platform' }, { PLATFORM_BIN: '/nonexistent/report-archive-platform' },
    { POSTGRES_PSQL_BIN: 'psql --command=unsafe' }, { POSTGRES_PSQL_BIN: '/nonexistent/psql' },
    { TEST_ARCHIVE_ADMIN_USERNAME: 'other_admin' }, { TEST_ARCHIVE_ADMIN_PASSWORD: undefined },
    { TEST_ARCHIVE_ADMIN_PASSWORD: 'short' }, { TEST_ARCHIVE_ADMIN_PASSWORD: 'x'.repeat(129) },
    { TEST_ARCHIVE_ADMIN_PASSWORD: 'x'.repeat(16) + '\u0000' },
  ]) {
    let calls = 0;
    assert.throws(() => initializeReportArchiveBrowser({ env: { ...environment(), ...changed }, runCommand: () => { calls++; return emptySchema('desktop'); } }));
    assert.equal(calls, 0);
  }
});

test('empty-schema check precedes normal migrate, seed and Harbor admin bootstrap with scoped password', () => {
  const env = {
    ...environment(), BOOTSTRAP_ADMIN_PASSWORD: 'unrelated-inherited-secret',
    PGHOST: 'remote.example', PGOPTIONS: '-c search_path=unsafe',
  };
  const calls = [];
  const result = initializeReportArchiveBrowser({
    env,
    runCommand: (command, args, options) => {
      calls.push({ command, args, ...options });
      return calls.length === 1 ? emptySchema('desktop') : '';
    },
  });
  assert.deepEqual(result, { viewport: 'desktop', database: database('desktop'), username: 'archive_browser_desktop' });
  assert.equal(calls.length, 4);
  assert.equal(calls[0].command, readReportArchiveBrowserConfiguration(env).psqlBin);
  assert.deepEqual(calls[0].args.slice(0, 6), ['-X', '-At', '--no-password', '--set=ON_ERROR_STOP=1', '--dbname', env.DATABASE_URL]);
  assert.equal(calls[0].args[6], '--command');
  assert.match(calls[0].args[7], /^SELECT current_database\(\), current_user, count\(\*\) FROM pg_catalog\.pg_class/);
  assert.match(calls[0].args[7], /n\.nspname = 'public'/);
  assert.equal(calls[0].timeoutMs, 60_000);
  assert.deepEqual(calls.slice(1).map(call => call.args), [
    ['migrate'], ['seed'], ['create-admin', '--username', 'archive_browser_desktop', '--brand', 'harbor'],
  ]);
  for (const call of calls) {
    assert.ok(Object.isFrozen(call.env));
    assert.equal(call.env.APP_ENV, 'test');
    assert.equal(call.env.DATABASE_URL, env.DATABASE_URL);
    for (const name of ['TEST_ARCHIVE_ADMIN_PASSWORD', 'PGHOST', 'PGOPTIONS', 'DATABASE_READ_URL', 'DATABASE_READ_URLS']) assert.equal(call.env[name], undefined);
    assert.ok(!call.args.includes(env.TEST_ARCHIVE_ADMIN_PASSWORD));
  }
  assert.equal(calls[3].env.BOOTSTRAP_ADMIN_PASSWORD, env.TEST_ARCHIVE_ADMIN_PASSWORD);
  assert.equal(calls[1].env.BOOTSTRAP_ADMIN_PASSWORD, undefined);
  assert.equal(calls[2].env.BOOTSTRAP_ADMIN_PASSWORD, undefined);
});

test('a nonempty or mismatched database is never migrated and failures stay redacted', () => {
  for (const output of [
    `${database('desktop')}|lottery_test|1\n`, `${database('mobile')}|lottery_test|0\n`,
    `${database('desktop')}|postgres|0\n`, '', undefined,
  ]) {
    let calls = 0;
    assert.throws(() => initializeReportArchiveBrowser({ env: environment(), runCommand: () => { calls++; return output; } }), /no public tables/);
    assert.equal(calls, 1);
  }
  for (const failAt of [1, 2, 3, 4]) {
    let calls = 0;
    assert.throws(() => initializeReportArchiveBrowser({
      env: environment(), runCommand: () => {
        if (++calls === failAt) throw new Error('private-password-sentinel');
        return calls === 1 ? emptySchema('desktop') : '';
      },
    }), error => /initialization failed at/.test(error.message) && !error.message.includes('sentinel'));
    assert.equal(calls, failAt);
  }
  assert.doesNotMatch(source, /\b(?:INSERT INTO|UPDATE \w+ SET|DELETE FROM|TRUNCATE|DROP (?:TABLE|SCHEMA))\b|\['worker'\]|shell:\s*true/i);
});

test('command wrapper bounds execution and redacts child output', () => {
  assert.equal(runReportArchiveCommand(process.execPath, ['-e', 'process.stdout.write("ok")'], { timeoutMs: 1_000 }), 'ok');
  assert.throws(() => runReportArchiveCommand(process.execPath, ['-e', 'process.stderr.write("private-sentinel"); process.exit(9)'], { timeoutMs: 1_000 }), error => error.message === 'Report archive browser command failed');
  assert.throws(() => runReportArchiveCommand('/nonexistent/report-archive-browser-command', [], { timeoutMs: 1_000 }), /command failed/);
  assert.throws(() => runReportArchiveCommand(process.execPath, ['-e', 'setTimeout(() => {}, 5000)'], { timeoutMs: 50 }), /command failed/);
  for (const timeoutMs of [0, -1, 1.5, 240_001, Infinity, NaN]) assert.throws(() => runReportArchiveCommand(process.execPath, [], { timeoutMs }), /timeout/);
});

test('initializer command line refuses an unacknowledged run without echoing secrets', () => {
  const result = spawnSync(process.execPath, [new URL('../../scripts/init-report-archive-browser.mjs', import.meta.url).pathname], {
    env: { ...environment(), REPORT_ARCHIVE_FIXTURE_CONFIRM: '', AUTH_KEY: 'private-auth-key-sentinel' },
    encoding: 'utf8', timeout: 1_000,
  });
  assert.equal(result.status, 1);
  assert.equal(result.stdout, '');
  assert.match(result.stderr, /explicitly owned test database/);
  assert.doesNotMatch(result.stderr, /sentinel|archive-static-owned|postgres:\/\//);
});
