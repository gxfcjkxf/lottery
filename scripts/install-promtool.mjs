import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { appendFileSync, chmodSync, mkdirSync, mkdtempSync, renameSync, rmSync, statSync, writeFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

export const PROMTOOL_VERSION = '3.15.0';
export const PROMTOOL_ARCHIVE = `prometheus-${PROMTOOL_VERSION}.linux-amd64.tar.gz`;
export const PROMTOOL_URL = `https://github.com/prometheus/prometheus/releases/download/v${PROMTOOL_VERSION}/${PROMTOOL_ARCHIVE}`;
export const PROMTOOL_SHA256 = '2a542df32eac02ee17b9d844fb2aa1de00dafa5476579ba8a3ba862e9d572ea0';
export const MAX_DOWNLOAD_BYTES = 160 * 1024 * 1024;
export const DOWNLOAD_TIMEOUT_MS = 180_000;

export function assertInstallEnvironment({ platform = process.platform, arch = process.arch, githubActions = process.env.GITHUB_ACTIONS, testOverride = process.env.PROMTOOL_INSTALL_TEST } = {}) {
  if (platform !== 'linux' || arch !== 'x64') throw new Error('Pinned promtool installer supports Linux amd64 only');
  if (githubActions !== 'true' && testOverride !== '1') throw new Error('Pinned promtool installation is restricted to GitHub Actions');
}

async function readBounded(response) {
  if (!response.body) throw new Error('Pinned promtool download returned no body');
  const contentLength = Number(response.headers.get('content-length'));
  if (Number.isFinite(contentLength) && contentLength > MAX_DOWNLOAD_BYTES) throw new Error('Pinned promtool archive exceeds the 160 MiB limit');
  const chunks = [];
  let size = 0;
  for await (const chunk of response.body) {
    size += chunk.length;
    if (size > MAX_DOWNLOAD_BYTES) throw new Error('Pinned promtool archive exceeds the 160 MiB limit');
    chunks.push(chunk);
  }
  return Buffer.concat(chunks, size);
}

export async function installPromtool({ cwd = process.cwd(), fetchImpl = fetch, githubEnv = process.env.GITHUB_ENV } = {}) {
  assertInstallEnvironment();
  const ownedRoot = resolve(cwd, '.local');
  mkdirSync(ownedRoot, { recursive: true });
  const owned = mkdtempSync(join(ownedRoot, 'promtool-'));
  const archivePath = join(owned, PROMTOOL_ARCHIVE);
  const extractPath = join(owned, 'extract');
  const binaryPath = join(owned, 'promtool');
  let success = false;
  try {
    const response = await fetchImpl(PROMTOOL_URL, { redirect: 'follow', signal: AbortSignal.timeout(DOWNLOAD_TIMEOUT_MS) });
    if (!response.ok) throw new Error('Pinned promtool download failed');
    const archive = await readBounded(response);
    const digest = createHash('sha256').update(archive).digest('hex');
    if (digest !== PROMTOOL_SHA256) throw new Error('Pinned promtool archive checksum did not match');
    writeFileSync(archivePath, archive, { flag: 'wx', mode: 0o600 });
    mkdirSync(extractPath, { mode: 0o700 });
    const member = `prometheus-${PROMTOOL_VERSION}.linux-amd64/promtool`;
    const extracted = spawnSync('tar', ['-xzf', archivePath, '--no-same-owner', '--no-same-permissions', '--strip-components=1', '-C', extractPath, '--', member], {
      encoding: 'utf8', timeout: 30_000, maxBuffer: 1_048_576, shell: false,
    });
    if (extracted.error || extracted.status !== 0 || extracted.signal) throw new Error('Pinned promtool archive extraction failed');
    const extractedPath = join(extractPath, 'promtool');
    if (!statSync(extractedPath).isFile()) throw new Error('Pinned archive did not contain the promtool executable');
    renameSync(extractedPath, binaryPath);
    chmodSync(binaryPath, 0o755);
    rmSync(archivePath);
    rmSync(extractPath, { recursive: true });
    if (githubEnv) appendFileSync(githubEnv, `PROMTOOL_BIN=${binaryPath}\n`, { mode: 0o600 });
    success = true;
    return binaryPath;
  } finally {
    if (!success) rmSync(owned, { recursive: true, force: true });
  }
}

const invokedPath = process.argv[1] && resolve(process.argv[1]);
if (invokedPath === fileURLToPath(import.meta.url)) {
  try {
    const path = await installPromtool();
    console.log(`PROMTOOL_BIN=${path}`);
  } catch {
    console.error('Pinned promtool installation failed; no archive URL, response body, or environment value was emitted.');
    process.exitCode = 1;
  }
}
