import { spawnSync } from 'node:child_process';
import { createRequire } from 'node:module';
import { pathToFileURL } from 'node:url';

// CI bootstrap, not a browser-test retry. A healthy hosted image needs no apt
// update; only an actual missing-library diagnostic authorizes installing deps.
export function isMissingSystemDependency(error) {
  if (!(error instanceof Error)) return false;
  return /Host system is missing dependencies(?:!| to run browsers\.)/i.test(error.message)
    || /error while loading shared libraries:\s*lib[A-Za-z0-9_.+-]+\.so(?:\.[0-9]+)*:\s*cannot open shared object file/i.test(error.message);
}

// Browser downloads may take longer than three minutes on a hosted runner.
// Keep a five-minute child bound and the independent seven-minute CI step bound.
export function runSetupCommand(command, args, { timeoutMs = 300_000, env = process.env, stdio = 'inherit' } = {}) {
  if (!Number.isSafeInteger(timeoutMs) || timeoutMs <= 0 || timeoutMs > 300_000) throw new Error('Invalid browser setup timeout');
  const result = spawnSync(command, args, { timeout: timeoutMs, env, stdio, killSignal: 'SIGTERM' });
  if (result.error) throw result.error;
  if (result.status !== 0 || result.signal) throw new Error(`Browser setup command failed: status=${result.status}, signal=${result.signal ?? 'none'}`);
}

export async function prepareChromium({ installBrowser, probeBrowser, installDependencies, log = () => {} }) {
  await installBrowser();
  try {
    await probeBrowser();
    log('Chromium launch verified; no system-package changes required.');
    return;
  } catch (error) {
    if (!isMissingSystemDependency(error)) throw error;
    log('Chromium reports missing system dependencies; performing one bounded dependency installation.');
  }
  await installDependencies();
  await probeBrowser();
  log('Chromium launch verified after dependency installation.');
}

export async function probeChromium(chromium, executablePath) {
  let browser;
  try {
    browser = await chromium.launch({ headless: true, timeout: 15_000, ...(executablePath ? { executablePath } : {}) });
    const page = await browser.newPage();
    await page.setContent('<!doctype html><title>CI browser probe</title><p>Chromium ready</p>', { timeout: 10_000 });
    if (await page.textContent('p', { timeout: 10_000 }) !== 'Chromium ready') throw new Error('Chromium probe did not render the test document');
  } finally {
    if (browser) await browser.close();
  }
}

async function main() {
  // Never run privileged CI setup on a developer machine or production host.
  if (process.platform !== 'linux' || process.env.GITHUB_ACTIONS !== 'true' || process.env.RUNNER_OS !== 'Linux') {
    throw new Error('Browser setup is restricted to GitHub Actions Linux runners');
  }
  const require = createRequire(import.meta.url);
  const cli = require.resolve('@playwright/test/cli');
  const { chromium } = await import('@playwright/test');
  await prepareChromium({
    installBrowser: () => runSetupCommand(process.execPath, [cli, 'install', 'chromium'], {
      env: { ...process.env, PLAYWRIGHT_DOWNLOAD_CONNECTION_TIMEOUT: '60000' },
    }),
    // A root-owned timeout bounds apt and all its children, not just the pnpm
    // parent. The shipped Playwright CLI selects packages for the actual OS.
    installDependencies: () => runSetupCommand('sudo', ['-n', '--', 'timeout', '--kill-after=10s', '180s', process.execPath, cli, 'install-deps', 'chromium'], { timeoutMs: 200_000 }),
    probeBrowser: () => probeChromium(chromium),
    log: console.log,
  });
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  main().catch(error => {
    console.error(error instanceof Error ? error.message : 'Browser setup failed');
    process.exitCode = 1;
  });
}
