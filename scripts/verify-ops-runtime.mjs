import { spawn, spawnSync } from 'node:child_process';
import { accessSync, constants, createWriteStream, mkdirSync, mkdtempSync, readFileSync, realpathSync, statSync, writeFileSync } from 'node:fs';
import { isAbsolute, join, resolve } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';

const database = process.env.DATABASE_URL;
if (process.env.APP_ENV !== 'test' || process.env.OPS_RUNTIME_FIXTURE_CONFIRM !== 'owned_synthetic_database' || !/^postgres:\/\/lottery_test@127\.0\.0\.1:(?:5432|55432)\/lottery_ops_acceptance\?sslmode=disable$/.test(database ?? '') || process.env.DATABASE_READ_URL || process.env.DATABASE_READ_URLS) {
  throw new Error('Operations acceptance requires the exact explicitly owned loopback test database');
}
function executable(value, name) {
  if (!value || !isAbsolute(value) || /[\0\r\n]/.test(value) || !statSync(realpathSync(value)).isFile()) throw new Error(`${name} must name an absolute local executable`);
  accessSync(value, constants.X_OK); return value;
}
const platform = executable(process.env.PLATFORM_BIN, 'PLATFORM_BIN');
const psql = executable(process.env.POSTGRES_PSQL_BIN, 'POSTGRES_PSQL_BIN');
mkdirSync(resolve('.local'), { recursive: true });
const owned = mkdtempSync(resolve('.local/ops-runtime-'));
const key = join(owned, 'auth.key'), tokenPath = join(owned, 'metrics.token');
const [apiPort, metricsPort, workerPort] = process.env.CI === 'true' ? [8080, 9091, 9092] : [15291, 15294, 15295];
const api = `http://127.0.0.1:${apiPort}`, metrics = `http://127.0.0.1:${metricsPort}`, workerMetrics = `http://127.0.0.1:${workerPort}`;
const childEnv = { ...process.env, APP_ENV: 'test', DATABASE_URL: database, DATABASE_READ_URL: '', DATABASE_READ_URLS: '', AUTH_KEY_FILE: key, AUTH_KEY: '', HTTP_ADDR: `127.0.0.1:${apiPort}`, API_METRICS_ADDR: `127.0.0.1:${metricsPort}`, WORKER_METRICS_ADDR: `127.0.0.1:${workerPort}`, METRICS_TOKEN_FILE: tokenPath, TRACE_OTLP_ENDPOINT: '', TRACE_SAMPLE_RATIO: '0.1', DB_MAX_CONNS: '5', TRUSTED_PROXY_CIDRS: '' };
for (const name of Object.keys(childEnv)) if (name.startsWith('PG') || name.startsWith('OTEL_')) delete childEnv[name];
function command(binary, args, extra = {}) {
  const result = spawnSync(binary, args, { env: { ...childEnv, ...extra }, encoding: 'utf8', timeout: 60_000, maxBuffer: 1_048_576, shell: false });
  if (result.status !== 0 || result.error || result.signal) throw new Error(`Owned command failed at ${args[0]}`);
  return result.stdout;
}
const sql = text => command(psql, ['-X', '-At', '--no-password', '--set=ON_ERROR_STOP=1', '--dbname', database, '--command', text]);
if (sql("SELECT current_database(),current_user,(SELECT count(*) FROM pg_tables WHERE schemaname='public');").trim() !== 'lottery_ops_acceptance|lottery_test|0') throw new Error('Acceptance database must start with an empty public schema');
command(platform, ['generate-auth-key', key]);
command(platform, ['generate-metrics-token', tokenPath]);
// Neither startup command nor the read-only check may repair an empty schema.
for (const mode of ['check', 'serve', 'worker']) {
  const rejected = spawnSync(platform, [mode], { env: childEnv, encoding: 'utf8', timeout: 10_000, maxBuffer: 1_048_576, shell: false });
  if (rejected.status !== 1 || rejected.error || rejected.signal) throw new Error('Incompatible schema did not fail closed');
}
if (sql("SELECT count(*) FROM pg_tables WHERE schemaname='public';").trim() !== '0') throw new Error('Rejected startup migrated an empty schema');
command(platform, ['migrate']);
command(platform, ['check']);
command(platform, ['seed']);
const token = readFileSync(tokenPath, 'utf8');
if (!/^[A-Za-z0-9_-]{43}$/.test(token)) throw new Error('Generated private bearer token is invalid');
const tables = sql("SELECT tablename FROM pg_tables WHERE schemaname='public' ORDER BY tablename;").trim().split('\n');
if (tables.length < 60 || tables.some(name => !/^[a-z_]+$/.test(name))) throw new Error('Owned table inventory is incomplete');
const fingerprint = () => sql(tables.map(name => `SELECT '${name}',md5(coalesce(jsonb_agg(to_jsonb(t) ORDER BY to_jsonb(t)::text),'[]'::jsonb)::text) FROM public."${name}" t;`).join('\n'));
const before = fingerprint();
const children = [];
function launch(mode, env = {}) {
  const log = createWriteStream(join(owned, `${mode}-${children.length}.log`), { flags: 'wx', mode: 0o600 });
  const child = spawn(platform, [mode], { env: { ...childEnv, ...env }, stdio: ['ignore', 'pipe', 'pipe'], shell: false });
  child.stdout.pipe(log, { end: false }); child.stderr.pipe(log, { end: false });
  const entry = { child, log, result: null, error: null, done: null };
  entry.done = new Promise(resolveDone => {
    child.once('error', () => { entry.error = true; });
    child.once('close', (code, signal) => { entry.result = { code, signal }; log.end(); resolveDone(); });
  });
  children.push(entry); return entry;
}
async function stop(entry) {
  if (entry.result || entry.error) return;
  entry.child.kill('SIGTERM');
  const timely = await Promise.race([entry.done.then(() => true), delay(15_000, undefined, { ref: false }).then(() => false)]);
  if (!timely) { entry.child.kill('SIGKILL'); await entry.done; throw new Error('Owned child did not stop gracefully'); }
  if (entry.result?.code !== 0) throw new Error('Owned child failed during graceful shutdown');
}
async function request(base, path, status = 200, authenticated = true, options = {}) {
  const response = await fetch(base + path, { ...options, headers: { ...(authenticated ? { Authorization: `Bearer ${token}` } : {}), ...options.headers }, signal: AbortSignal.timeout(4_000) });
  if (response.status !== status) throw new Error(`Unexpected HTTP status for ${path.split('?')[0]}: ${response.status}, expected ${status}`);
  return response;
}
async function waitReady(base, entry, authenticated = true) {
  const deadline = Date.now() + 15_000;
  while (Date.now() < deadline) {
    if (entry.result || entry.error) throw new Error('Owned service stopped before readiness');
    try { if ((await fetch(base + '/health/ready', { headers: authenticated ? { Authorization: `Bearer ${token}` } : {}, signal: AbortSignal.timeout(2_500) })).status === 200) return; } catch { /* The same live child may still be starting. */ }
    await delay(100);
  }
  throw new Error('Owned service never became ready');
}
function requireMetric(body, name, expected) {
  const line = body.split('\n').find(row => row.startsWith(name + ' ') || row.startsWith(name + '{'));
  if (!line || (expected !== undefined && Number(line.slice(line.lastIndexOf(' ') + 1)) !== expected)) throw new Error(`Expected operational metric ${name} is absent or invalid`);
}
async function waitSnapshot(base) {
  const deadline = Date.now() + 5_000;
  while (Date.now() < deadline) {
    const body = await (await request(base, '/metrics')).text();
    if (/^lottery_ops_snapshot_success 1$/m.test(body)) return body;
    await delay(100);
  }
  throw new Error('Initial read-only operational snapshot did not complete');
}
async function refuses(mode, override) {
  const entry = launch(mode, override);
  const terminal = await Promise.race([entry.done.then(() => true), delay(10_000, undefined, { ref: false }).then(() => false)]);
  if (!terminal || entry.result?.code !== 1) { if (!entry.result) entry.child.kill('SIGTERM'); throw new Error('Invalid startup did not fail closed'); }
}
let apiProcess, workerProcess;
try {
  apiProcess = launch('serve');
  await waitReady(api, apiProcess, false);
  await waitReady(metrics, apiProcess);
  workerProcess = launch('worker');
  await waitReady(workerMetrics, workerProcess);
  for (const base of [metrics, workerMetrics]) {
    for (const path of ['/metrics', '/health/live', '/health/ready']) {
      await request(base, path, 401, false);
      await request(base, path, 401, false, { headers: { Authorization: 'Bearer wrong-owned-token' } });
      const good = await request(base, path);
      if (good.headers.get('cache-control') !== 'no-store' || good.headers.get('x-content-type-options') !== 'nosniff') throw new Error('Protected response lost its no-store guard');
    }
    await request(base, '/metrics?token=not-an-authorization-header', 401, false);
    await request(base, '/metrics', 405, true, { method: 'POST' });
    await request(base, '/debug/pprof', 404);
  }
  await request(api, '/metrics', 404, false);
  for (let i = 0; i < 100; i++) await request(api, `/missing-owned-${i}?private_query=private-value`, 404, false);
  await request(api, '/api/v1/context', 200, false, { headers: { Host: `aurora.localhost:${apiPort}` } });
  const apiBody = await waitSnapshot(metrics);
  const workerBody = await waitSnapshot(workerMetrics);
  for (const body of [apiBody, workerBody]) {
    requireMetric(body, 'lottery_build_info', 1);
    requireMetric(body, 'lottery_database_up', 1);
    requireMetric(body, 'lottery_ops_snapshot_success', 1);
    if (['private-value', 'missing-owned-', 'localhost', 'Bearer ', token, key, database].some(value => body.includes(value))) throw new Error('Operational metrics contain request or credential data');
  }
  if (!apiBody.includes('route="unmatched"') || !apiBody.includes('route="/api/v1/context"')) throw new Error('Actual router did not hand off canonical patterns');
  for (const component of ['period_tick', 'notification']) {
    if (!workerBody.split('\n').some(row => row.startsWith('lottery_worker_runs_total{') && row.includes(`component="${component}"`) && Number(row.slice(row.lastIndexOf(' ') + 1)) >= 1)) throw new Error('Critical worker loop did not complete');
  }
  if (fingerprint() !== before) throw new Error('Operational startup, scraping or empty worker loops changed persistent business data');
  await refuses('serve', { AUTH_KEY_FILE: join(owned, 'missing-auth.key') });
  await refuses('worker', { WORKER_METRICS_ADDR: `127.0.0.1:${metricsPort}` });
  await stop(workerProcess); await stop(apiProcess);
  const privateWrong = join(owned, 'invalid-private-token');
  writeFileSync(privateWrong, 'too-short', { flag: 'wx', mode: 0o600 });
  await refuses('serve', { METRICS_TOKEN_FILE: privateWrong });
  await refuses('serve', { API_METRICS_ADDR: `0.0.0.0:${metricsPort}` });
  await refuses('worker', { WORKER_METRICS_ADDR: `192.168.1.2:${workerPort}` });
  if (fingerprint() !== before) throw new Error('Rejected startup changed persistent business data');
  writeFileSync(join(owned, 'result.json'), JSON.stringify({ success: true, checks: ['private_auth', 'loopback_only', 'canonical_routes', 'core_worker_readiness', 'readonly_snapshot', 'incompatible_schema', 'invalid_startup', 'graceful_shutdown'], table_count: tables.length, financial_retries: 0, process_restarts: 0 }), { flag: 'wx', mode: 0o600 });
  console.log(`Operations runtime acceptance passed against ${tables.length} unchanged tables; evidence: ${owned}`);
} finally {
  for (const entry of children.reverse()) if (!entry.result && !entry.error) { try { await stop(entry); } catch { /* Preserve the original failure; only owned children are touched. */ } }
}
