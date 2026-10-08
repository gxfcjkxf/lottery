import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import { prepareRewardBrowser, readRewardBrowserConfiguration, runRewardCommand } from '../../scripts/prepare-reward-browser.mjs';

const workflow = readFileSync(new URL('../../.github/workflows/ci.yaml', import.meta.url), 'utf8');
const source = readFileSync(new URL('../../scripts/prepare-reward-browser.mjs', import.meta.url), 'utf8');
const job = /^  reward-browser:\n([\s\S]*?)(?=^  [a-z][a-z-]*:\n|(?![\s\S]))/m.exec(workflow)?.[1];
const database = viewport => `lottery_rewards_ui_${viewport}_s22`;
const emptySchema = viewport => `${database(viewport)}|lottery_test|0\n`;
function environment(viewport = 'desktop') {
  return {
    APP_ENV: 'test', REWARD_UI_VIEWPORT: viewport, REWARD_UI_FIXTURE_CONFIRM: 'owned_synthetic_database',
    DATABASE_URL: `postgres://lottery_test@127.0.0.1:55432/${database(viewport)}?sslmode=disable`,
    PLATFORM_BIN: process.execPath,
    TEST_REWARD_ADMIN_PASSWORD: 'reward-static-owned-operator-password',
    TEST_REWARD_SUPER_PASSWORD: 'reward-static-owned-reader-password',
  };
}

test('reward browser matrix uses fresh viewport databases, normal CLI bootstrap and bounded Chromium setup', () => {
  assert.ok(job, 'dedicated reward-browser job required');
  assert.match(job, /timeout-minutes: 20/);
  assert.match(job, /viewport: \[desktop, mobile\]/);
  assert.match(job, /POSTGRES_DB: lottery_rewards_ui_\$\{\{ matrix\.viewport \}\}_s22/);
  assert.match(job, /POSTGRES_USER: lottery_test/);
  assert.match(job, /POSTGRES_HOST_AUTH_METHOD: trust/);
  assert.match(job, /ports: \["127\.0\.0\.1:5432:5432"\]/);
  assert.match(job, /DATABASE_URL: postgres:\/\/lottery_test@127\.0\.0\.1:5432\/lottery_rewards_ui_\$\{\{ matrix\.viewport \}\}_s22\?sslmode=disable/);
  assert.match(job, /APP_ENV: test/);
  assert.match(job, /REWARD_UI_VIEWPORT: \$\{\{ matrix\.viewport \}\}/);
  assert.match(job, /REWARD_UI_FIXTURE_CONFIRM: owned_synthetic_database/);
  assert.match(job, /TEST_ADMIN_ORIGIN: http:\/\/localhost:5174/);
  for (const name of ['TEST_REWARD_ADMIN_PASSWORD', 'TEST_REWARD_SUPER_PASSWORD']) assert.match(job, new RegExp(`^      ${name}: .+$`, 'm'));
  assert.match(job, /PLATFORM_BIN: \.local\/reward-platform/);
  assert.match(job, /go -C backend build -o \.\.\/\.local\/reward-platform \.\/cmd\/platform/);
  assert.match(job, /\.local\/reward-platform generate-auth-key \.local\/auth\.key/);
  assert.match(job, /node scripts\/prepare-reward-browser\.mjs/);
  assert.match(job, /\.local\/reward-platform serve > reward-browser-api\.log 2>&1 &/);
  assert.match(job, /name: Prepare and verify Chromium\n        timeout-minutes: 7\n        run: node scripts\/prepare-ci-browser\.mjs/);
  assert.doesNotMatch(job, /POSTGRES_PASSWORD:|BOOTSTRAP_ADMIN_PASSWORD:|browserfixture|\bplatform\s+worker\b|\breward-platform\s+worker\b|continue-on-error|--with-deps/);
  assert.doesNotMatch(job, /UPDATE |INSERT INTO |DELETE FROM |TRUNCATE|DROP |reset|auth_rate_limits|X-Forwarded-For|payment-policy/i);
});

test('reward browser CI runs the exact spec once per project and rejects zero tests or skipped tests', () => {
  assert.match(job, /pnpm exec playwright test tests\/browser\/rewards\.spec\.ts --project=\$\{\{ matrix\.viewport \}\} --workers=1 --retries=0 --reporter=list,json/);
  assert.match(job, /PLAYWRIGHT_JSON_OUTPUT_NAME: \.local\/reward-browser-report\.json/);
  assert.match(job, /stats\.skipped !== 0/);
  assert.match(job, /stats\.expected > 0/);
  assert.doesNotMatch(job, /--pass-with-no-tests|--retries=([1-9]\d*)|cat .*auth\.key|\.local\/\*|echo .*AUTH_KEY/);
  assert.match(job, /name: reward-browser-\$\{\{ matrix\.viewport \}\}-failure-traces/);
  const start = workflow.indexOf('  commission-cycles-browser:');
  const end = workflow.indexOf('  withdrawal-browser:', start);
  assert.ok(start > workflow.indexOf('  reward-browser:') && end > start);
  assert.doesNotMatch(workflow.slice(start, end), /reward-browser:|prepare-reward-browser\.mjs|rewards\.spec\.ts/);
});

test('initializer accepts only the exact owned database, viewport, loopback, port and single SSL parameter', () => {
  for (const viewport of ['desktop', 'mobile']) {
    for (const host of ['127.0.0.1', 'localhost']) {
      for (const port of ['5432', '55432']) {
        for (const scheme of ['postgres', 'postgresql']) {
          const config = readRewardBrowserConfiguration({ ...environment(viewport), DATABASE_URL: `${scheme}://lottery_test@${host}:${port}/${database(viewport)}?sslmode=disable` });
          assert.equal(config.database, database(viewport));
          assert.equal(config.viewport, viewport);
          assert.equal(config.psqlBin, 'psql');
          assert.equal(config.adminOrigin, 'http://localhost:5174');
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
    url.replace('_desktop_s22', '_mobile_s22'), url.replace('_s22', '_s22_followup'),
    url.replace(database('desktop'), 'postgres'), url.replace('_s22', '%5fs22'),
    `${url}#fragment`, `${url}#`, `${url}&sslmode=disable`, `${url}&host=remote.example`,
    `${url}&options=-csearch_path=other`, `${url}&`, url.replace('disable', 'require'),
    url.replace('sslmode=disable', ''), url.replace('postgres:', 'http:'), ` ${url}`, `${url}\n`,
  ]) assert.throws(() => readRewardBrowserConfiguration({ ...environment(), DATABASE_URL }), /owned.*URL/i, DATABASE_URL);
});

test('unsafe ownership, read configuration, missing binaries and missing credentials fail before any command', () => {
  for (const changed of [
    { APP_ENV: 'production' }, { APP_ENV: 'development' }, { REWARD_UI_VIEWPORT: '' }, { REWARD_UI_VIEWPORT: 'tablet' },
    { REWARD_UI_FIXTURE_CONFIRM: '' }, { REWARD_UI_FIXTURE_CONFIRM: 'yes' },
    { DATABASE_READ_URL: environment().DATABASE_URL }, { DATABASE_READ_URLS: '[]' }, { DATABASE_READ_URL: ' ' },
    { PLATFORM_BIN: undefined }, { PLATFORM_BIN: 'platform' }, { PLATFORM_BIN: '/nonexistent/reward-platform' },
    { POSTGRES_PSQL_BIN: 'psql --command=unsafe' }, { POSTGRES_PSQL_BIN: '/nonexistent/psql' },
    { TEST_REWARD_ADMIN_PASSWORD: undefined }, { TEST_REWARD_SUPER_PASSWORD: 'short' },
    { TEST_REWARD_SUPER_PASSWORD: 'x'.repeat(129) }, { TEST_REWARD_ADMIN_PASSWORD: 'x'.repeat(16) + '\u0000' },
  ]) {
    let calls = 0;
    assert.throws(() => prepareRewardBrowser({ env: { ...environment(), ...changed }, runCommand: () => { calls++; return emptySchema('desktop'); } }));
    assert.equal(calls, 0);
  }
});

test('schema check precedes four normal CLI writes with separately scoped bootstrap passwords', () => {
  const env = { ...environment(), POSTGRES_PSQL_BIN: process.execPath, BOOTSTRAP_ADMIN_PASSWORD: 'unrelated-password-must-not-be-inherited', PGHOST: 'remote.example', PGOPTIONS: '-c search_path=unsafe' };
  const calls = [];
  const result = prepareRewardBrowser({ env, runCommand: (command, args, options) => { calls.push({ command, args, ...options }); return calls.length === 1 ? emptySchema('desktop') : ''; } });
  assert.deepEqual(result, { viewport: 'desktop', database: database('desktop') });
  assert.equal(calls.length, 5);
  assert.equal(calls[0].command, readRewardBrowserConfiguration(env).psqlBin);
  assert.deepEqual(calls[0].args.slice(0, 6), ['-X', '-At', '--no-password', '--set=ON_ERROR_STOP=1', '--dbname', env.DATABASE_URL]);
  assert.equal(calls[0].args[6], '--command');
  assert.match(calls[0].args[7], /^SELECT current_database\(\), current_user, count\(\*\) FROM pg_catalog\.pg_class/);
  assert.match(calls[0].args[7], /n\.nspname = 'public'/);
  assert.equal(calls[0].timeoutMs, 60_000);
  assert.deepEqual(calls.slice(1).map(call => call.args), [
    ['migrate'], ['seed'],
    ['create-admin', '--username', 'reward_s22_operator', '--brand', 'aurora'],
    ['create-admin', '--username', 'reward_s22_reader', '--super'],
  ]);
  for (const call of calls) {
    assert.ok(Object.isFrozen(call.env));
    assert.equal(call.env.APP_ENV, 'test');
    assert.equal(call.env.DATABASE_URL, env.DATABASE_URL);
    for (const name of ['TEST_REWARD_ADMIN_PASSWORD', 'TEST_REWARD_SUPER_PASSWORD', 'PGHOST', 'PGOPTIONS', 'DATABASE_READ_URL', 'DATABASE_READ_URLS']) assert.equal(call.env[name], undefined);
    assert.ok(!call.args.includes(env.TEST_REWARD_ADMIN_PASSWORD));
    assert.ok(!call.args.includes(env.TEST_REWARD_SUPER_PASSWORD));
  }
  for (const call of calls.slice(0, 3)) assert.equal(call.env.BOOTSTRAP_ADMIN_PASSWORD, undefined);
  assert.equal(calls[3].env.BOOTSTRAP_ADMIN_PASSWORD, env.TEST_REWARD_ADMIN_PASSWORD);
  assert.equal(calls[4].env.BOOTSTRAP_ADMIN_PASSWORD, env.TEST_REWARD_SUPER_PASSWORD);
  for (const call of calls.slice(1)) assert.equal(call.timeoutMs, 180_000);
});

test('nonempty schema or a different observed database identity cannot start migrations', () => {
  for (const output of [
    `${database('desktop')}|lottery_test|1\n`, `${database('desktop')}|lottery_test|999\n`,
    `${database('mobile')}|lottery_test|0\n`, `${database('desktop')}|postgres|0\n`,
    '', '0\n', `${emptySchema('desktop')}extra\n`, undefined,
  ]) {
    let calls = 0;
    assert.throws(() => prepareRewardBrowser({ env: environment(), runCommand: () => { calls++; return output; } }), /no public tables/);
    assert.equal(calls, 1);
  }
});

test('command failure stops initialization without exposing child output or retrying writes', () => {
  for (const failAt of [1, 2, 3, 4, 5]) {
    let calls = 0;
    assert.throws(() => prepareRewardBrowser({ env: environment(), runCommand: () => {
      if (++calls === failAt) throw new Error('private-auth-key-and-password-sentinel');
      return calls === 1 ? emptySchema('desktop') : '';
    } }), error => /initialization failed at/.test(error.message) && !error.message.includes('sentinel'));
    assert.equal(calls, failAt);
  }
  assert.doesNotMatch(source, /\b(?:INSERT INTO|UPDATE \w+ SET|DELETE FROM|TRUNCATE|DROP (?:TABLE|SCHEMA))\b|\['worker'\]|shell:\s*true|console\.(?:log|error)\([^\n]*(?:config|schema|Password|stdout|stderr)/i);
});

test('native command wrapper uses bounded children and redacts failures', () => {
  assert.equal(runRewardCommand(process.execPath, ['-e', 'process.stdout.write("ok")'], { timeoutMs: 1_000 }), 'ok');
  assert.throws(() => runRewardCommand(process.execPath, ['-e', 'process.stderr.write("private-key-sentinel"); process.exit(9)'], { timeoutMs: 1_000 }), error => error.message === 'Reward browser command failed');
  assert.throws(() => runRewardCommand('/nonexistent/reward-browser-command', [], { timeoutMs: 1_000 }), /Reward browser command failed/);
  assert.throws(() => runRewardCommand(process.execPath, ['-e', 'setTimeout(() => {}, 5000)'], { timeoutMs: 50 }), /Reward browser command failed/);
  for (const timeoutMs of [0, -1, 1.5, 240_001, Infinity, NaN]) assert.throws(() => runRewardCommand(process.execPath, [], { timeoutMs }), /timeout/);
});

test('initializer CLI refuses unacknowledged database access without printing secrets', () => {
  const result = spawnSync(process.execPath, [new URL('../../scripts/prepare-reward-browser.mjs', import.meta.url).pathname], {
    env: { ...environment(), REWARD_UI_FIXTURE_CONFIRM: '', AUTH_KEY: 'private-auth-key-sentinel' },
    encoding: 'utf8', timeout: 1_000,
  });
  assert.equal(result.status, 1);
  assert.equal(result.stdout, '');
  assert.match(result.stderr, /explicitly owned test database/);
  assert.doesNotMatch(result.stderr, /sentinel|reward-static-owned|postgres:\/\//);
});
