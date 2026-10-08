import { spawnSync } from 'node:child_process';
import { accessSync, constants, realpathSync, readdirSync, statSync } from 'node:fs';
import { delimiter, isAbsolute, join, resolve } from 'node:path';
import { homedir } from 'node:os';
import { pathToFileURL } from 'node:url';

const CONFIRMATION = 'owned_synthetic_database';
const BRAND = 'harbor';
const SCHEMA_CHECK = "SELECT current_database(), current_user, count(*) FROM pg_catalog.pg_class AS c JOIN pg_catalog.pg_namespace AS n ON n.oid = c.relnamespace WHERE n.nspname = 'public' AND c.relkind IN ('r', 'p', 'f', 'v', 'm');";

function localExecutable(value, { fallback = false, env = process.env } = {}) {
  if (value === undefined && fallback) {
    let cachedClients = [];
    try {
      cachedClients = readdirSync(join(homedir(), '.cache', 'lottery-tools'))
        .filter(name => /^postgres-app-/.test(name))
        .map(name => join(homedir(), '.cache', 'lottery-tools', name, 'bin', 'psql'));
    } catch { /* The CI image may provide psql through PATH instead. */ }
    const candidates = [...cachedClients, ...(env.PATH ?? '').split(delimiter).filter(Boolean).map(directory => join(directory, 'psql'))];
    for (const candidate of candidates) {
      try {
        const executable = realpathSync(candidate);
        if (statSync(executable).isFile()) {
          accessSync(executable, constants.X_OK);
          // Debian/Ubuntu psql may be a link to pg_wrapper, which dispatches
          // using the invoked client name. Validate its target but invoke psql.
          return resolve(candidate);
        }
      } catch { /* Try the next PATH directory. */ }
    }
    throw new Error('An absolute executable PostgreSQL client path is required');
  }
  if (typeof value !== 'string' || !value || !isAbsolute(value) || /[\u0000\r\n]/.test(value)) {
    throw new Error('An absolute executable binary path is required');
  }
  try {
    const executable = realpathSync(value);
    if (!statSync(executable).isFile()) throw new Error();
    accessSync(executable, constants.X_OK);
    return resolve(value);
  } catch {
    throw new Error('An absolute executable binary path is required');
  }
}

function ownedAuthKey(value) {
  if (typeof value !== 'string' || !value || /[\u0000\r\n]/.test(value)) throw new Error('A local regular AUTH_KEY_FILE is required');
  try {
    const path = resolve(value);
    if (!isAbsolute(path)) throw new Error();
    const realPath = realpathSync(path);
    if (!statSync(realPath).isFile()) throw new Error();
    return realPath;
  } catch {
    throw new Error('A local regular AUTH_KEY_FILE is required');
  }
}

function adminPassword(value) {
  if (typeof value !== 'string' || value.includes('\u0000') || Buffer.byteLength(value, 'utf8') < 16 || Buffer.byteLength(value, 'utf8') > 128) {
    throw new Error('TEST_ARCHIVE_POLICY_ADMIN_PASSWORD must contain 16-128 bytes');
  }
  return value;
}

export function validateReportArchivePolicyFixtureEnv(env = process.env) {
  const viewport = env.REPORT_ARCHIVE_POLICY_VIEWPORT;
  if ((viewport !== 'desktop' && viewport !== 'mobile') || env.APP_ENV !== 'test' || env.REPORT_ARCHIVE_POLICY_FIXTURE_CONFIRM !== CONFIRMATION) {
    throw new Error('Report archive policy browser initialization requires an explicitly owned test database and desktop/mobile viewport');
  }
  for (const name of ['DATABASE_READ_URL', 'DATABASE_READ_URLS']) {
    if (env[name] !== undefined && env[name] !== '') throw new Error('Report archive policy browser read database overrides are forbidden');
  }
  const database = `lottery_archive_policy_browser_${viewport}`;
  const databaseURL = env.DATABASE_URL;
  const exactURL = /^postgres(?:ql)?:\/\/lottery_test@127\.0\.0\.1:(5432|55432)\/lottery_archive_policy_browser_(desktop|mobile)\?sslmode=disable$/;
  if (typeof databaseURL !== 'string' || !exactURL.test(databaseURL)) throw new Error('Exact owned report archive policy PostgreSQL URL required');
  let url;
  try { url = new URL(databaseURL); }
  catch { throw new Error('Exact owned report archive policy PostgreSQL URL required'); }
  if (url.username !== 'lottery_test' || url.password || url.pathname !== `/${database}` || url.pathname !== decodeURIComponent(url.pathname) || url.search !== '?sslmode=disable' || url.hash !== '') {
    throw new Error('Exact owned report archive policy PostgreSQL URL required');
  }
  const username = `archive_policy_browser_${viewport}`;
  if (env.TEST_ARCHIVE_POLICY_ADMIN_USERNAME !== username) throw new Error('Exact report archive policy browser administrator username required');
  return Object.freeze({
    viewport,
    databaseURL,
    database,
    username,
    platformBin: localExecutable(env.PLATFORM_BIN),
    psqlBin: localExecutable(env.POSTGRES_PSQL_BIN, { fallback: true, env }),
    authKeyFile: ownedAuthKey(env.AUTH_KEY_FILE),
    adminPassword: adminPassword(env.TEST_ARCHIVE_POLICY_ADMIN_PASSWORD),
  });
}

export function runReportArchivePolicyCommand(command, args, { env, timeoutMs = 180_000 } = {}) {
  if (!Number.isSafeInteger(timeoutMs) || timeoutMs <= 0 || timeoutMs > 240_000) throw new Error('Invalid report archive policy browser command timeout');
  const result = spawnSync(command, args, {
    env, timeout: timeoutMs, killSignal: 'SIGTERM', shell: false,
    encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'], maxBuffer: 1_048_576,
  });
  if (result.error || result.status !== 0 || result.signal) throw new Error('Report archive policy browser command failed');
  return result.stdout;
}

export function initializeReportArchivePolicyBrowser({ env = process.env, runCommand = runReportArchivePolicyCommand } = {}) {
  const snapshot = { ...env };
  const config = validateReportArchivePolicyFixtureEnv(snapshot);
  const childEnv = { ...snapshot, APP_ENV: 'test', DATABASE_URL: config.databaseURL, AUTH_KEY_FILE: config.authKeyFile };
  for (const name of Object.keys(childEnv)) {
    if (name.startsWith('PG') || ['DATABASE_READ_URL', 'DATABASE_READ_URLS', 'BOOTSTRAP_ADMIN_PASSWORD', 'TEST_ARCHIVE_POLICY_ADMIN_PASSWORD'].includes(name)) delete childEnv[name];
  }
  Object.freeze(childEnv);
  const run = (stage, command, args, options) => {
    try { return runCommand(command, args, options); }
    catch { throw new Error(`Report archive policy browser initialization failed at ${stage}`); }
  };
  const schema = run('schema check', config.psqlBin, [
    '-X', '-At', '--no-password', '--set=ON_ERROR_STOP=1', '--dbname', config.databaseURL, '--command', SCHEMA_CHECK,
  ], { env: Object.freeze({ ...childEnv, PGCONNECT_TIMEOUT: '5' }), timeoutMs: 60_000 });
  if (typeof schema !== 'string' || schema.trim() !== `${config.database}|lottery_test|0`) {
    throw new Error('Report archive policy browser database must have no public tables and match the owned identity');
  }
  const platform = (stage, args, bootstrapPassword) => run(stage, config.platformBin, args, {
    env: bootstrapPassword === undefined ? childEnv : Object.freeze({ ...childEnv, BOOTSTRAP_ADMIN_PASSWORD: bootstrapPassword }),
    timeoutMs: 180_000,
  });
  platform('migrate', ['migrate']);
  platform('seed', ['seed']);
  platform('brand administrator bootstrap', ['create-admin', '--username', config.username, '--brand', BRAND], config.adminPassword);
  return Object.freeze({ viewport: config.viewport, database: config.database, username: config.username });
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    if (process.argv.length !== 2) throw new Error('Report archive policy browser initialization takes environment inputs only');
    const result = initializeReportArchivePolicyBrowser();
    console.log(`Owned report archive policy browser database initialized for ${result.viewport}.`);
  } catch (error) {
    console.error(error instanceof Error ? error.message : 'Report archive policy browser initialization failed');
    process.exitCode = 1;
  }
}
