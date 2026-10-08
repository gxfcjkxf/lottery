import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import {
  chmodSync, mkdirSync, mkdtempSync, rmSync, symlinkSync, writeFileSync,
} from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, join } from 'node:path';
import {
  initializeReportArchivePolicyBrowser,
  runReportArchivePolicyCommand,
} from '../../scripts/init-report-archive-policy-browser.mjs';

const tempDirectory = mkdtempSync(join(tmpdir(), 'report-archive-policy-executable-'));
after(() => rmSync(tempDirectory, { recursive: true, force: true }));

const authKeyFile = join(tempDirectory, 'auth.key');
writeFileSync(authKeyFile, 'synthetic-local-auth-key');

function environment(viewport, psqlBin, output) {
  return {
    APP_ENV: 'test',
    REPORT_ARCHIVE_POLICY_VIEWPORT: viewport,
    REPORT_ARCHIVE_POLICY_FIXTURE_CONFIRM: 'owned_synthetic_database',
    DATABASE_URL: `postgres://lottery_test@127.0.0.1:55432/lottery_archive_policy_browser_${viewport}?sslmode=disable`,
    PLATFORM_BIN: process.execPath,
    POSTGRES_PSQL_BIN: psqlBin,
    AUTH_KEY_FILE: authKeyFile,
    TEST_ARCHIVE_POLICY_ADMIN_USERNAME: `archive_policy_browser_${viewport}`,
    TEST_ARCHIVE_POLICY_ADMIN_PASSWORD: 'archive-policy-static-synthetic-admin-password',
    PSQL_FIXTURE_OUTPUT: output,
  };
}

function psqlFixtureSource() {
  return `#!${process.execPath}\nimport { basename } from 'node:path';
const args = process.argv.slice(2);
if (basename(process.argv[1]) !== 'psql'
  || args[0] !== '-X'
  || args[1] !== '-At'
  || args[2] !== '--no-password'
  || args[3] !== '--set=ON_ERROR_STOP=1'
  || args[4] !== '--dbname'
  || args[6] !== '--command'
  || !args[7]?.startsWith('SELECT current_database()')) {
  process.exit(23);
}
const database = new URL(args[5]).pathname.slice(1);
process.stdout.write(process.env.PSQL_FIXTURE_OUTPUT ?? \`${'${database}'}|lottery_test|0\\n\`);
`;
}

function installPsqlAlias(directory) {
  mkdirSync(directory, { recursive: true });
  const wrapper = join(directory, 'pg_wrapper');
  const alias = join(directory, 'psql');
  writeFileSync(wrapper, psqlFixtureSource());
  chmodSync(wrapper, 0o755);
  symlinkSync('pg_wrapper', alias);
  return alias;
}

function installDirectPsql(directory) {
  mkdirSync(directory, { recursive: true });
  const executable = join(directory, 'psql');
  writeFileSync(executable, psqlFixtureSource());
  chmodSync(executable, 0o755);
  return executable;
}

test('executes the schema query through the configured psql alias for both viewports', () => {
  const binDirectory = join(tempDirectory, 'postgres', 'bin');
  const alias = installPsqlAlias(binDirectory);
  const linkedBinDirectory = join(tempDirectory, 'postgres-bin-link');
  symlinkSync(binDirectory, linkedBinDirectory, 'dir');

  for (const viewport of ['desktop', 'mobile']) {
    for (const psqlBin of [alias, join(linkedBinDirectory, 'psql')]) {
      const calls = [];
      const result = initializeReportArchivePolicyBrowser({
        env: environment(viewport, psqlBin, `lottery_archive_policy_browser_${viewport}|lottery_test|0\n`),
        runCommand(command, args, options) {
          calls.push({ command, args, options });
          if (calls.length === 1) return runReportArchivePolicyCommand(command, args, options);
          return '';
        },
      });

      assert.deepEqual(result, {
        viewport,
        database: `lottery_archive_policy_browser_${viewport}`,
        username: `archive_policy_browser_${viewport}`,
      });
      assert.equal(calls.length, 4);
      assert.equal(calls[0].command, psqlBin);
      assert.equal(calls[0].args[6], '--command');
      assert.match(calls[0].args[7], /^SELECT current_database\(\)/);
      assert.deepEqual(calls.slice(1).map(call => call.args), [
        ['migrate'], ['seed'],
        ['create-admin', '--username', `archive_policy_browser_${viewport}`, '--brand', 'harbor'],
      ]);
    }
  }
});

test('rejects a mismatched schema result before invoking platform commands', () => {
  const psql = installDirectPsql(join(tempDirectory, 'wrong-schema-bin'));
  const calls = [];
  assert.throws(() => initializeReportArchivePolicyBrowser({
    env: environment('desktop', psql, 'another_database|lottery_test|0\n'),
    runCommand(command, args, options) {
      calls.push({ command, args, options });
      return runReportArchivePolicyCommand(command, args, options);
    },
  }), /no public tables and match the owned identity/);
  assert.equal(calls.length, 1);
  assert.equal(basename(calls[0].command), 'psql');
  assert.equal(calls[0].args[6], '--command');
});
