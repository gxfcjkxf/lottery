import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, readFileSync, realpathSync, rmSync, symlinkSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';
import {
  initializeAttributionBrowser,
  runAttributionBrowserCommand,
  validateAttributionBrowserFixtureEnv,
} from '../../scripts/init-attribution-browser.mjs';

const tempDirectory = mkdtempSync(join(tmpdir(), 'attribution-browser-'));
const authKeyFile = join(tempDirectory, 'auth.key');
const psqlAlias = join(tempDirectory, 'psql');
writeFileSync(authKeyFile, 'synthetic-local-auth-key');
symlinkSync(process.execPath, psqlAlias);
after(() => rmSync(tempDirectory, { recursive: true, force: true }));
const browserSpec = readFileSync(new URL('../browser/attribution-reports.spec.ts', import.meta.url), 'utf8');
const browserConfig = readFileSync(new URL('../../playwright.attribution-reports.config.ts', import.meta.url), 'utf8');
const baseBrowserConfig = readFileSync(new URL('../../playwright.config.ts', import.meta.url), 'utf8');

const database = viewport => `lottery_attribution_browser_${viewport}`;
const emptySchema = viewport => `${database(viewport)}|lottery_test|0\n`;
function environment(viewport = 'desktop', port = '55432') {
  return {
    APP_ENV: 'test',
    REPORT_ATTRIBUTION_VIEWPORT: viewport,
    REPORT_ATTRIBUTION_FIXTURE_CONFIRM: 'owned_synthetic_database',
    DATABASE_URL: `postgres://lottery_test@127.0.0.1:${port}/${database(viewport)}?sslmode=disable`,
    PLATFORM_BIN: process.execPath,
    POSTGRES_PSQL_BIN: psqlAlias,
    AUTH_KEY_FILE: authKeyFile,
    TEST_ATTRIBUTION_ADMIN_USERNAME: `attribution_browser_${viewport}`,
    TEST_ATTRIBUTION_ADMIN_PASSWORD: 'attribution-static-synthetic-admin-password',
  };
}

test('accepts only desktop/mobile owned loopback databases on supported ports and preserves verified psql aliases', () => {
  for (const viewport of ['desktop', 'mobile']) {
    for (const port of ['5432', '55432']) {
      const config = validateAttributionBrowserFixtureEnv(environment(viewport, port));
      assert.equal(config.database, database(viewport));
      assert.equal(config.username, `attribution_browser_${viewport}`);
      assert.equal(config.databaseURL, environment(viewport, port).DATABASE_URL);
      assert.equal(config.authKeyFile, realpathSync(authKeyFile));
      assert.equal(config.psqlBin, psqlAlias);
      assert.equal(realpathSync(config.psqlBin), realpathSync(process.execPath));
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
    base.replace('/lottery_attribution_browser_desktop?', '/postgres?'),
    base.replace('/lottery_attribution_browser_desktop?', '/lottery_attribution_browser_mobile?'),
    base.replace('browser_desktop?', 'browser%5fdesktop?'), base.replace('sslmode=disable', 'sslmode=require'),
    `${base}&host=remote.example`, `${base}&sslmode=disable`, `${base}#fragment`, `${base}\n`,
    base.replace('lottery_test@', 'lottery_test:secret@'),
  ]) assert.throws(() => validateAttributionBrowserFixtureEnv({ ...environment(), DATABASE_URL }), /owned.*URL/i, DATABASE_URL);
});

test('rejects missing confirmation, read replicas, bad credentials and invalid local paths before commands', () => {
  const changes = [
    { APP_ENV: 'production' }, { REPORT_ATTRIBUTION_VIEWPORT: 'tablet' },
    { REPORT_ATTRIBUTION_FIXTURE_CONFIRM: 'yes' },
    { DATABASE_READ_URL: 'postgres://remote/replica' }, { DATABASE_READ_URLS: '[]' },
    { TEST_ATTRIBUTION_ADMIN_USERNAME: 'other' }, { TEST_ATTRIBUTION_ADMIN_PASSWORD: undefined },
    { TEST_ATTRIBUTION_ADMIN_PASSWORD: 'short' }, { TEST_ATTRIBUTION_ADMIN_PASSWORD: 'x'.repeat(129) },
    { TEST_ATTRIBUTION_ADMIN_PASSWORD: `${'x'.repeat(16)}\u0000` },
    { PLATFORM_BIN: 'platform' }, { PLATFORM_BIN: '/no/such/platform' },
    { POSTGRES_PSQL_BIN: 'psql --command=unsafe' }, { POSTGRES_PSQL_BIN: '/no/such/psql' },
    { AUTH_KEY_FILE: undefined }, { AUTH_KEY_FILE: join(tempDirectory, 'missing.key') },
    { AUTH_KEY_FILE: tempDirectory }, { AUTH_KEY_FILE: 'file:///tmp/auth.key' },
  ];
  for (const changed of changes) {
    let calls = 0;
    assert.throws(() => initializeAttributionBrowser({
      env: { ...environment(), ...changed }, runCommand: () => { calls++; return emptySchema('desktop'); },
    }));
    assert.equal(calls, 0);
  }
});

test('checks the exact empty owned public schema before normal migrate, seed and Harbor bootstrap', () => {
  const env = {
    ...environment('desktop', '5432'),
    BOOTSTRAP_ADMIN_PASSWORD: 'inherited-private-secret',
    AUTH_KEY: 'inherited-auth-key-sentinel',
    PGHOST: 'remote.example', PGOPTIONS: '-c search_path=unsafe',
  };
  const calls = [];
  const result = initializeAttributionBrowser({
    env,
    runCommand: (command, args, options) => {
      calls.push({ command, args, ...options });
      return calls.length === 1 ? emptySchema('desktop') : '';
    },
  });
  assert.deepEqual(result, {
    viewport: 'desktop', database: database('desktop'), username: 'attribution_browser_desktop',
  });
  assert.equal(calls.length, 4);
  assert.equal(calls[0].command, psqlAlias);
  assert.deepEqual(calls[0].args.slice(0, 6), [
    '-X', '-At', '--no-password', '--set=ON_ERROR_STOP=1', '--dbname', env.DATABASE_URL,
  ]);
  assert.match(calls[0].args[7], /^SELECT current_database\(\), current_user, count\(\*\)/);
  assert.equal(calls[0].timeoutMs, 60_000);
  assert.deepEqual(calls.slice(1).map(call => call.args), [
    ['migrate'], ['seed'],
    ['create-admin', '--username', 'attribution_browser_desktop', '--brand', 'harbor'],
  ]);
  for (const call of calls) {
    assert.ok(Object.isFrozen(call.env));
    assert.equal(call.env.APP_ENV, 'test');
    assert.equal(call.env.DATABASE_URL, env.DATABASE_URL);
    assert.equal(call.env.AUTH_KEY_FILE, realpathSync(authKeyFile));
    for (const name of [
      'TEST_ATTRIBUTION_ADMIN_PASSWORD', 'AUTH_KEY', 'PGHOST', 'PGOPTIONS', 'DATABASE_READ_URL',
      'DATABASE_READ_URLS',
    ]) assert.equal(call.env[name], undefined);
    assert.ok(!call.args.includes(env.TEST_ATTRIBUTION_ADMIN_PASSWORD));
  }
  assert.equal(calls[3].env.BOOTSTRAP_ADMIN_PASSWORD, env.TEST_ATTRIBUTION_ADMIN_PASSWORD);
  assert.equal(calls[1].env.BOOTSTRAP_ADMIN_PASSWORD, undefined);
  assert.equal(calls[2].env.BOOTSTRAP_ADMIN_PASSWORD, undefined);
});

test('nonempty or mismatched databases and command failures stop without secrets or retries', () => {
  for (const output of [
    `${database('desktop')}|lottery_test|1\n`, `${database('mobile')}|lottery_test|0\n`,
    `${database('desktop')}|postgres|0\n`, '', undefined,
  ]) {
    let calls = 0;
    assert.throws(() => initializeAttributionBrowser({
      env: environment(), runCommand: () => { calls++; return output; },
    }), /no public tables/);
    assert.equal(calls, 1);
  }
  for (const failAt of [1, 2, 3, 4]) {
    let calls = 0;
    assert.throws(() => initializeAttributionBrowser({
      env: environment(),
      runCommand: () => {
        if (++calls === failAt) throw new Error('attribution-password-sentinel');
        return calls === 1 ? emptySchema('desktop') : '';
      },
    }), error => /initialization failed at/.test(error.message) && !error.message.includes('sentinel'));
    assert.equal(calls, failAt);
  }
  const source = readFileSync(new URL('../../scripts/init-attribution-browser.mjs', import.meta.url), 'utf8');
  assert.doesNotMatch(source, /INSERT\s+INTO|UPDATE\s+\w+\s+SET|DELETE\s+FROM|TRUNCATE|DROP\s+TABLE|shell:\s*true/i);
});

test('command wrapper bounds execution and returns redacted failures', () => {
  assert.equal(runAttributionBrowserCommand(process.execPath, ['-e', 'process.stdout.write("ok")'], { timeoutMs: 1_000 }), 'ok');
  assert.throws(() => runAttributionBrowserCommand(process.execPath, [
    '-e', 'process.stderr.write("private-sentinel"); process.exit(9)',
  ], { timeoutMs: 1_000 }), error => error.message === 'Attribution browser command failed');
  assert.throws(() => runAttributionBrowserCommand('/no/such/command', [], { timeoutMs: 1_000 }), /command failed/);
  assert.throws(() => runAttributionBrowserCommand(process.execPath, ['-e', 'setTimeout(() => {}, 5000)'], { timeoutMs: 50 }), /command failed/);
  for (const timeoutMs of [0, -1, 1.5, 240_001, Infinity, NaN]) {
    assert.throws(() => runAttributionBrowserCommand(process.execPath, [], { timeoutMs }), /timeout/);
  }
});

test('CLI refuses unowned execution and never echoes configured secrets', () => {
  const result = spawnSync(process.execPath, [new URL('../../scripts/init-attribution-browser.mjs', import.meta.url).pathname], {
    env: { ...environment(), REPORT_ATTRIBUTION_FIXTURE_CONFIRM: '', AUTH_KEY: 'private-auth-key-sentinel' },
    encoding: 'utf8', timeout: 1_000,
  });
  assert.equal(result.status, 1);
  assert.equal(result.stdout, '');
  assert.match(result.stderr, /explicitly owned test database/);
  assert.doesNotMatch(result.stderr, /sentinel|attribution-static|postgres:\/\//);
});

test('dedicated desktop and 360px mobile browser coverage exercises real saved attribution and export', () => {
  assert.match(browserConfig, /testMatch: 'attribution-reports\.spec\.ts'/);
  assert.match(browserConfig, /timeout: 60_000/);
  assert.match(browserConfig, /timeout: 10_000/);
  assert.match(browserConfig, /retries: 0/);
  assert.match(browserConfig, /workers: 1/);
  assert.match(browserConfig, /webServer: process\.env\.TEST_ATTRIBUTION_ORIGIN === 'http:\/\/localhost:15292' \? \[\] : \[/);
  assert.match(browserConfig, /command: 'pnpm dev:admin -- --host 127\.0\.0\.1'/);
  assert.match(browserConfig, /url: 'http:\/\/localhost:5174'/);
  assert.match(browserConfig, /reuseExistingServer: !process\.env\.CI/);
  assert.match(baseBrowserConfig, /name: 'desktop',[\s\S]*?width: 1440, height: 900/);
  assert.match(baseBrowserConfig, /name: 'mobile',[\s\S]*?width: 360, height: 800/);
  assert.match(browserSpec, /real attribution reports preserve saved agent scope and export unique order totals/);
  assert.match(browserSpec, /test\('real attribution reports preserve saved agent scope and export unique order totals',[\s\S]*?assertAttributionFixtureEnvironment\(\);[\s\S]*?loginAdmin\(/);
  assert.match(browserSpec, /REPORT_ATTRIBUTION_FIXTURE_CONFIRM/);
  assert.match(browserSpec, /lottery_attribution_browser_\$\{viewport\}/);
  assert.match(browserSpec, /TEST_ATTRIBUTION_ORIGIN/);
  assert.match(browserSpec, /registerMember\(page\.request, rootCode\.code\)/);
  assert.match(browserSpec, /registerMember\(page\.request, childCode\.code\)/);
  assert.match(browserSpec, /expectPublicAttribution\(directAttribution, directMember\.memberId,[\s\S]*?code_id: rootCode\.id, source_code: rootCode\.code/);
  assert.match(browserSpec, /expectPublicAttribution\(childAttribution, childMember\.memberId,[\s\S]*?code_id: childCode\.id, source_code: childCode\.code/);
  assert.match(browserSpec, /expect\(Object\.keys\(value\)\.sort\(\)\)\.toEqual\(\[\.\.\.publicAttributionFields\]\.sort\(\)\)/);
  assert.match(browserSpec, /getByTestId\('attribution-group'\)\.selectOption\('game'\)/);
  assert.match(browserSpec, /getByTestId\('attribution-agent-scope'\)\.selectOption\('direct'\)/);
  assert.match(browserSpec, /selectOption\('join_method'\);[\s\S]*?getByTestId\('attribution-agent-scope'\)[\s\S]*?selectOption\('direct'\);[\s\S]*?getByTestId\('attribution-join-method'\)[\s\S]*?selectOption\(''\);[\s\S]*?Agent UUID \(optional\).*?\.fill\(''\);[\s\S]*?Game UUID \(optional\).*?\.fill\(''\);[\s\S]*?Member UUID \(optional\).*?\.fill\(''\);[\s\S]*?const sourceResponse/);
  assert.match(browserSpec, /\/bet-previews/);
  assert.match(browserSpec, /\/bet-orders/);
  assert.match(browserSpec, /\/reports\/attribution\/export/);
  assert.match(browserSpec, /economicState\(\)\)\.toEqual\(beforeRead\)/);
  assert.match(browserSpec, /reduce\(\(sum, item\) => sum \+ BigInt\(item\.totals\.stake_points\), 0n\)\)\.toBe\(4n\)/);
  assert.match(browserSpec, /key: 'operator', totals: expect\.objectContaining\(\{ order_count: '1' \}\)/);
  assert.match(browserSpec, /page\.screenshot\(\{ path: `\.local\/attribution-reports-\$\{info\.project\.name\}\.png`, fullPage: true \}\)/);
  assert.match(browserSpec, /Reports & reconciliation/);
  assert.doesNotMatch(browserSpec, /page\.route\(|route\.fulfill\(|route\.fetch\(/);
  assert.doesNotMatch(browserSpec, /test\.skip\(|force:\s*true/);
});
