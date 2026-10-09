import { spawnSync } from 'node:child_process';
import { accessSync, constants, realpathSync, statSync } from 'node:fs';
import { isAbsolute, resolve } from 'node:path';
import { pathToFileURL } from 'node:url';

const CONFIRMATION = 'owned_synthetic_database';
const SCHEMA_CHECK = "SELECT current_database(), current_user, count(*) FROM pg_catalog.pg_class AS c JOIN pg_catalog.pg_namespace AS n ON n.oid = c.relnamespace WHERE n.nspname = 'public' AND c.relkind IN ('r', 'p', 'f', 'v', 'm');";

function localExecutable(value) {
  if (typeof value !== 'string' || !value || !isAbsolute(value) || /[\u0000\r\n]/.test(value)) {
    throw new Error('An executable local binary path is required');
  }
  try {
    const executable = realpathSync(resolve(value));
    if (!statSync(executable).isFile()) throw new Error();
    accessSync(executable, constants.X_OK);
    return resolve(value);
  } catch {
    throw new Error('An executable local binary path is required');
  }
}

function password(value, name) {
  if (typeof value !== 'string' || value.includes('\u0000') || Buffer.byteLength(value, 'utf8') < 16 || Buffer.byteLength(value, 'utf8') > 128) {
    throw new Error(`${name} must contain 16-128 bytes`);
  }
  return value;
}

export function readRewardBrowserConfiguration(env = process.env) {
  const viewport = env.REWARD_UI_VIEWPORT;
  if ((viewport !== 'desktop' && viewport !== 'mobile') || env.APP_ENV !== 'test' || env.REWARD_UI_FIXTURE_CONFIRM !== CONFIRMATION) {
    throw new Error('Reward browser initialization requires an explicitly owned test database and desktop/mobile viewport');
  }
  // Read replicas would connect outside the single owned database, even if a
  // caller supplied an otherwise valid primary URL. Empty defaults are fine.
  for (const name of ['DATABASE_READ_URL', 'DATABASE_READ_URLS']) {
    if (env[name] !== undefined && env[name] !== '') throw new Error('Reward browser read database overrides are forbidden');
  }
  const databaseURL = env.DATABASE_URL;
  const database = `lottery_rewards_ui_${viewport}_s22`;
  // Validate the raw URL as well as its parsed fields. This excludes encoded
  // names, fragments, duplicate/extra query parameters and parser normalization.
  const exactURL = /^postgres(?:ql)?:\/\/lottery_test(?::[^@\s/?#]*)?@(127\.0\.0\.1|localhost):(5432|55432)\/lottery_rewards_ui_(desktop|mobile)_s22\?sslmode=disable$/;
  if (typeof databaseURL !== 'string' || !exactURL.test(databaseURL)) throw new Error('Exact owned reward browser PostgreSQL URL required');
  let url;
  try { url = new URL(databaseURL); }
  catch { throw new Error('Exact owned reward browser PostgreSQL URL required'); }
  if (url.username !== 'lottery_test' || url.pathname !== `/${database}` || url.search !== '?sslmode=disable' || url.hash !== '') {
    throw new Error('Exact owned reward browser PostgreSQL URL required');
  }
  return Object.freeze({
    viewport, databaseURL, database,
    platformBin: localExecutable(env.PLATFORM_BIN),
    psqlBin: localExecutable(env.POSTGRES_PSQL_BIN),
    adminOrigin: env.TEST_ADMIN_ORIGIN ?? 'http://localhost:5174',
    adminPassword: password(env.TEST_REWARD_ADMIN_PASSWORD, 'TEST_REWARD_ADMIN_PASSWORD'),
    superPassword: password(env.TEST_REWARD_SUPER_PASSWORD, 'TEST_REWARD_SUPER_PASSWORD'),
  });
}

export function runRewardCommand(command, args, { env, timeoutMs = 180_000 } = {}) {
  if (!Number.isSafeInteger(timeoutMs) || timeoutMs <= 0 || timeoutMs > 240_000) throw new Error('Invalid reward browser command timeout');
  const result = spawnSync(command, args, {
    env, timeout: timeoutMs, killSignal: 'SIGTERM', shell: false,
    encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'], maxBuffer: 1_048_576,
  });
  // Child output and command arguments may contain credentials. Never forward
  // them into errors or logs, including on psql or platform bootstrap failure.
  if (result.error || result.status !== 0 || result.signal) throw new Error('Reward browser command failed');
  return result.stdout;
}

export function prepareRewardBrowser({ env = process.env, runCommand = runRewardCommand } = {}) {
  const snapshot = { ...env };
  const config = readRewardBrowserConfiguration(snapshot);
  const childEnv = { ...snapshot, APP_ENV: 'test', DATABASE_URL: config.databaseURL, TEST_ADMIN_ORIGIN: config.adminOrigin };
  for (const name of Object.keys(childEnv)) {
    if (name.startsWith('PG') || ['DATABASE_READ_URL', 'DATABASE_READ_URLS', 'BOOTSTRAP_ADMIN_PASSWORD', 'TEST_REWARD_ADMIN_PASSWORD', 'TEST_REWARD_SUPER_PASSWORD'].includes(name)) delete childEnv[name];
  }
  Object.freeze(childEnv);
  const run = (stage, command, args, options) => {
    try { return runCommand(command, args, options); }
    catch { throw new Error(`Reward browser initialization failed at ${stage}`); }
  };
  const schema = run('schema check', config.psqlBin, [
    '-X', '-At', '--no-password', '--set=ON_ERROR_STOP=1', '--dbname', config.databaseURL, '--command', SCHEMA_CHECK,
  ], { env: Object.freeze({ ...childEnv, PGCONNECT_TIMEOUT: '5' }), timeoutMs: 60_000 });
  // No existing public tables are allowed, including empty tables from an old
  // run. The initializer never clears or repairs a previously used database.
  if (typeof schema !== 'string' || schema.trim() !== `${config.database}|lottery_test|0`) {
    throw new Error('Reward browser database must have no public tables and match the owned identity');
  }
  const platform = (stage, args, bootstrapPassword) => run(stage, config.platformBin, args, {
    env: bootstrapPassword === undefined ? childEnv : Object.freeze({ ...childEnv, BOOTSTRAP_ADMIN_PASSWORD: bootstrapPassword }),
    timeoutMs: 180_000,
  });
  platform('migrate', ['migrate']);
  platform('seed', ['seed']);
  platform('operator bootstrap', ['create-admin', '--username', 'reward_s22_operator', '--brand', 'aurora'], config.adminPassword);
  platform('reader bootstrap', ['create-admin', '--username', 'reward_s22_reader', '--super'], config.superPassword);
  return Object.freeze({ viewport: config.viewport, database: config.database });
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    if (process.argv.length !== 2) throw new Error('Reward browser initialization takes environment inputs only');
    const result = prepareRewardBrowser();
    console.log(`Owned reward browser database initialized for ${result.viewport}.`);
  } catch (error) {
    console.error(error instanceof Error ? error.message : 'Reward browser initialization failed');
    process.exitCode = 1;
  }
}
