import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import { isMissingSystemDependency, prepareChromium, probeChromium, runSetupCommand } from '../../scripts/prepare-ci-browser.mjs';

const workflow = readFileSync(new URL('../../.github/workflows/ci.yaml', import.meta.url), 'utf8');

test('Chromium preparation installs once and probes once when ready', async () => {
  const calls = [];
  await prepareChromium({
    installBrowser: async () => calls.push('install browser'),
    probeBrowser: async () => calls.push('probe'),
    installDependencies: async () => calls.push('install dependencies'),
  });
  assert.deepEqual(calls, ['install browser', 'probe']);
});

test('missing system dependencies authorize exactly one installation and reprobe', async () => {
  const calls = [];
  let probes = 0;
  await prepareChromium({
    installBrowser: async () => calls.push('install browser'),
    probeBrowser: async () => {
      calls.push('probe');
      if (++probes === 1) throw new Error('Host system is missing dependencies!');
    },
    installDependencies: async () => calls.push('install dependencies'),
  });
  assert.deepEqual(calls, ['install browser', 'probe', 'install dependencies', 'probe']);
});

test('setup and probe failures propagate unchanged without unauthorized fallback', async () => {
  const installError = new Error('browser download failed');
  const installCalls = [];
  await assert.rejects(prepareChromium({
    installBrowser: async () => { installCalls.push('install'); throw installError; },
    probeBrowser: async () => installCalls.push('probe'),
    installDependencies: async () => installCalls.push('dependencies'),
  }), error => error === installError);
  assert.deepEqual(installCalls, ['install']);

  for (const probeError of [
    new Error('browser crashed unexpectedly'),
    new Error('navigation timed out after 15000ms'),
  ]) {
    const calls = [];
    await assert.rejects(prepareChromium({
      installBrowser: async () => calls.push('install'),
      probeBrowser: async () => { calls.push('probe'); throw probeError; },
      installDependencies: async () => calls.push('dependencies'),
    }), error => error === probeError);
    assert.deepEqual(calls, ['install', 'probe']);
  }
});

test('fallback installation and second probe failures propagate without another attempt', async () => {
  const initialMissing = new Error('Host system is missing dependencies to run browsers.');
  const dependencyFailure = new Error('package installation failed');
  const calls = [];
  await assert.rejects(prepareChromium({
    installBrowser: async () => calls.push('browser'),
    probeBrowser: async () => { calls.push('probe'); throw initialMissing; },
    installDependencies: async () => { calls.push('dependencies'); throw dependencyFailure; },
  }), error => error === dependencyFailure);
  assert.deepEqual(calls, ['browser', 'probe', 'dependencies']);

  const reprobeFailure = new Error('Chromium still cannot launch');
  const retryCalls = [];
  let probeCount = 0;
  await assert.rejects(prepareChromium({
    installBrowser: async () => retryCalls.push('browser'),
    probeBrowser: async () => {
      retryCalls.push('probe');
      if (++probeCount === 1) throw initialMissing;
      throw reprobeFailure;
    },
    installDependencies: async () => retryCalls.push('dependencies'),
  }), error => error === reprobeFailure);
  assert.deepEqual(retryCalls, ['browser', 'probe', 'dependencies', 'probe']);
});

test('missing dependency detection accepts only explicit Playwright or Linux loader diagnostics', () => {
  for (const message of [
    'Host system is missing dependencies!',
    'Host system is missing dependencies to run browsers.',
    'error while loading shared libraries: libnss3.so.1: cannot open shared object file: No such file or directory',
    'error while loading shared libraries: libgtk-3.so.0: cannot open shared object file',
  ]) assert.equal(isMissingSystemDependency(new Error(message)), true, message);

  for (const error of [
    { message: 'Host system is missing dependencies!' },
    'Host system is missing dependencies!',
    new Error('browser process crashed'),
    new Error('missing executable at /path/to/chromium'),
    new Error('request timed out while downloading browser'),
    new Error('pnpm install failed'),
    new Error('error while loading shared libraries: libnss3.so.1: file not found'),
  ]) assert.equal(isMissingSystemDependency(error), false);
});

test('setup command wrapper runs a harmless child and reports failure and timeout', () => {
  assert.equal(runSetupCommand(process.execPath, ['-e', 'process.exit(0)'], { timeoutMs: 1_000, stdio: 'ignore' }), undefined);
  assert.throws(
    () => runSetupCommand(process.execPath, ['-e', 'process.exit(7)'], { timeoutMs: 1_000, stdio: 'ignore' }),
    /Browser setup command failed/,
  );
  assert.throws(() => runSetupCommand('/nonexistent/lottery-browser-setup-test', [], { timeoutMs: 1_000, stdio: 'ignore' }), /ENOENT/);
  assert.throws(() => runSetupCommand(process.execPath, ['-e', 'process.kill(process.pid,"SIGTERM")'], { timeoutMs: 1_000, stdio: 'ignore' }), /signal=SIGTERM/);

  const started = Date.now();
  assert.throws(
    () => runSetupCommand(process.execPath, ['-e', 'setTimeout(() => {}, 5000)'], { timeoutMs: 1_000, stdio: 'ignore' }),
    /ETIMEDOUT|timed out/i,
  );
  assert.ok(Date.now() - started < 3_000, 'timed child should be terminated promptly');
});

test('setup command timeout must be a positive safe integer no greater than four minutes', () => {
  for (const timeoutMs of [0, -1, 1.5, Number.MAX_SAFE_INTEGER + 1, 240_001, Infinity, NaN]) {
    assert.throws(
      () => runSetupCommand(process.execPath, ['-e', 'process.exit(0)'], { timeoutMs, stdio: 'ignore' }),
      /timeout/i,
      String(timeoutMs),
    );
  }
  assert.equal(runSetupCommand(process.execPath, ['-e', 'process.exit(0)'], { timeoutMs: 240_000, stdio: 'ignore' }), undefined);
});

test('all nine browser CI jobs use the bounded Chromium preparation step', () => {
  const blocks = [...workflow.matchAll(/^      - name: Prepare and verify Chromium\n        timeout-minutes: (\d+)\n        run: (.+)$/gm)];
  assert.equal(blocks.length, 9);
  for (const [, timeout, command] of blocks) {
    assert.equal(timeout, '7');
    assert.equal(command, 'node scripts/prepare-ci-browser.mjs');
  }
  assert.doesNotMatch(workflow, /--with-deps/);
  assert.doesNotMatch(workflow, /continue-on-error|--retries(?:\s+|=)(?!0\b)/);
});

test('browser CI keeps independent database and viewport or shard splits', () => {
  const commission = /^  commission-cycles-browser:\n([\s\S]*?)(?=^  [a-z][a-z-]*:\n|(?![\s\S]))/m.exec(workflow)?.[1];
  const browser = /^  browser:\n([\s\S]*?)(?=^  [a-z][a-z-]*:\n|(?![\s\S]))/m.exec(workflow)?.[1];
  assert.ok(commission);
  assert.ok(browser);
  assert.match(commission, /viewport: \[desktop, mobile\]/);
  assert.match(commission, /POSTGRES_DB: lottery_commission_ui_\$\{\{ matrix\.viewport \}\}_s16/);
  assert.match(browser, /project: \[desktop, mobile\]/);
  assert.match(browser, /shard: \[1, 2\]/);
});

test('browser setup CLI contains a Linux GitHub Actions guard', () => {
  const source = readFileSync(new URL('../../scripts/prepare-ci-browser.mjs', import.meta.url), 'utf8');
  assert.match(source, /process\.platform !== 'linux'/);
  assert.match(source, /process\.env\.GITHUB_ACTIONS !== 'true'/);
  assert.match(source, /process\.env\.RUNNER_OS !== 'Linux'/);
  assert.match(source, /if \(process\.argv\[1\][\s\S]*?main\(\)/);
  const result = spawnSync(process.execPath, [new URL('../../scripts/prepare-ci-browser.mjs', import.meta.url).pathname], {
    env: { ...process.env, GITHUB_ACTIONS: 'false', RUNNER_OS: 'Linux' }, encoding: 'utf8', timeout: 1_000,
  });
  assert.equal(result.status, 1);
  assert.match(result.stderr, /restricted to GitHub Actions Linux runners/);
  assert.equal(result.stdout, '');
});

test('browser probe verifies actual launch and document, then closes on success or content failure', async () => {
  for (const content of ['Chromium ready', 'wrong document']) {
    const calls = [];
    const chromium = { launch: async options => {
      calls.push(['launch', options]);
      return { newPage: async () => ({
        setContent: async (html, options) => calls.push(['content', html, options]),
        textContent: async (selector, options) => { calls.push(['read', selector, options]); return content; },
      }), close: async () => calls.push(['close']) };
    } };
    if (content === 'Chromium ready') await probeChromium(chromium);
    else await assert.rejects(probeChromium(chromium), /did not render/);
    assert.deepEqual(calls[0], ['launch', { headless: true, timeout: 15_000 }]);
    assert.match(calls[1][1], /<p>Chromium ready<\/p>/);
    assert.deepEqual(calls[1][2], { timeout: 10_000 });
    assert.deepEqual(calls[2], ['read', 'p', { timeout: 10_000 }]);
    assert.deepEqual(calls.at(-1), ['close']);
  }
  const missing = new Error('Host system is missing dependencies!');
  await assert.rejects(probeChromium({ launch: async () => { throw missing; } }), error => error === missing);
});
