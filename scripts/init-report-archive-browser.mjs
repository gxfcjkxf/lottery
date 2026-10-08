import { spawnSync } from 'node:child_process';
import { accessSync, constants, realpathSync, statSync } from 'node:fs';
import { resolve } from 'node:path';
import { pathToFileURL } from 'node:url';

const CONFIRMATION = 'owned_synthetic_database';
const SCHEMA_CHECK = "SELECT current_database(), current_user, count(*) FROM pg_catalog.pg_class AS c JOIN pg_catalog.pg_namespace AS n ON n.oid = c.relnamespace WHERE n.nspname = 'public' AND c.relkind IN ('r', 'p', 'f', 'v', 'm');";

function localExecutable(value, fallback) {
  if ((value === undefined || value === '') && fallback) return fallback;
  if (typeof value !== 'string' || !value || /[\u0000\r\n]/.test(value) || !/[\\/]/.test(value)) throw new Error('An executable local binary path is required');
  try {
    const executable = realpathSync(resolve(value));
    if (!statSync(executable).isFile()) throw new Error();
    accessSync(executable, constants.X_OK);
    return executable;
  } catch {
    throw new Error('An executable local binary path is required');
  }
}

function password(value) {
  if (typeof value !== 'string' || value.includes('\u0000') || Buffer.byteLength(value, 'utf8') < 16 || Buffer.byteLength(value, 'utf8') > 128) {
    throw new Error('TEST_ARCHIVE_ADMIN_PASSWORD must contain 16-128 bytes');
  }
  return value;
}

export function readReportArchiveBrowserConfiguration(env = process.env) {
  const viewport = env.REPORT_ARCHIVE_VIEWPORT;
  if ((viewport !== 'desktop' && viewport !== 'mobile') || env.APP_ENV !== 'test' || env.REPORT_ARCHIVE_FIXTURE_CONFIRM !== CONFIRMATION) {
    throw new Error('Report archive browser initialization requires an explicitly owned test database and desktop/mobile viewport');
  }
  for (const name of ['DATABASE_READ_URL', 'DATABASE_READ_URLS']) {
    if (env[name] !== undefined && env[name] !== '') throw new Error('Report archive browser read database overrides are forbidden');
  }
  const database = `lottery_archive_browser_${viewport}`;
  const databaseURL = env.DATABASE_URL;
  const exactURL = /^postgres(?:ql)?:\/\/lottery_test@(127\.0\.0\.1|localhost):(5432|55432)\/lottery_archive_browser_(desktop|mobile)\?sslmode=disable$/;
  if (typeof databaseURL !== 'string' || !exactURL.test(databaseURL)) throw new Error('Exact owned report archive browser PostgreSQL URL required');
  let url;
  try { url = new URL(databaseURL); }
  catch { throw new Error('Exact owned report archive browser PostgreSQL URL required'); }
  if (url.username !== 'lottery_test' || url.password || url.pathname !== `/${database}` || url.search !== '?sslmode=disable' || url.hash !== '') {
    throw new Error('Exact owned report archive browser PostgreSQL URL required');
  }
  const username = `archive_browser_${viewport}`;
  if (env.TEST_ARCHIVE_ADMIN_USERNAME !== username) throw new Error('Exact report archive browser administrator username required');
  return Object.freeze({
    viewport, databaseURL, database, username,
    platformBin: localExecutable(env.PLATFORM_BIN),
    psqlBin: localExecutable(env.POSTGRES_PSQL_BIN, 'psql'),
    adminPassword: password(env.TEST_ARCHIVE_ADMIN_PASSWORD),
  });
}

export function runReportArchiveCommand(command, args, { env, timeoutMs = 180_000 } = {}) {
  if (!Number.isSafeInteger(timeoutMs) || timeoutMs <= 0 || timeoutMs > 240_000) throw new Error('Invalid report archive browser command timeout');
  const result = spawnSync(command, args, {
    env, timeout: timeoutMs, killSignal: 'SIGTERM', shell: false,
    encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'], maxBuffer: 1_048_576,
  });
  if (result.error || result.status !== 0 || result.signal) throw new Error('Report archive browser command failed');
  return result.stdout;
}

export function initializeReportArchiveBrowser({ env = process.env, runCommand = runReportArchiveCommand } = {}) {
  const snapshot = { ...env };
  const config = readReportArchiveBrowserConfiguration(snapshot);
  const childEnv = { ...snapshot, APP_ENV: 'test', DATABASE_URL: config.databaseURL };
  for (const name of Object.keys(childEnv)) {
    if (name.startsWith('PG') || ['DATABASE_READ_URL', 'DATABASE_READ_URLS', 'BOOTSTRAP_ADMIN_PASSWORD', 'TEST_ARCHIVE_ADMIN_PASSWORD'].includes(name)) delete childEnv[name];
  }
  Object.freeze(childEnv);
  const run = (stage, command, args, options) => {
    try { return runCommand(command, args, options); }
    catch { throw new Error(`Report archive browser initialization failed at ${stage}`); }
  };
  const schema = run('schema check', config.psqlBin, [
    '-X', '-At', '--no-password', '--set=ON_ERROR_STOP=1', '--dbname', config.databaseURL, '--command', SCHEMA_CHECK,
  ], { env: Object.freeze({ ...childEnv, PGCONNECT_TIMEOUT: '5' }), timeoutMs: 60_000 });
  if (typeof schema !== 'string' || schema.trim() !== `${config.database}|lottery_test|0`) {
    throw new Error('Report archive browser database must have no public tables and match the owned identity');
  }
  const platform = (stage, args, bootstrapPassword) => run(stage, config.platformBin, args, {
    env: bootstrapPassword === undefined ? childEnv : Object.freeze({ ...childEnv, BOOTSTRAP_ADMIN_PASSWORD: bootstrapPassword }),
    timeoutMs: 180_000,
  });
  platform('migrate', ['migrate']);
  platform('seed', ['seed']);
  platform('brand administrator bootstrap', ['create-admin', '--username', config.username, '--brand', 'harbor'], config.adminPassword);
  return Object.freeze({ viewport: config.viewport, database: config.database, username: config.username });
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    if (process.argv.length !== 2) throw new Error('Report archive browser initialization takes environment inputs only');
    const result = initializeReportArchiveBrowser();
    console.log(`Owned report archive browser database initialized for ${result.viewport}.`);
  } catch (error) {
    console.error(error instanceof Error ? error.message : 'Report archive browser initialization failed');
    process.exitCode = 1;
  }
}
