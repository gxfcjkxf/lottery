import { existsSync } from 'node:fs';
import { isAbsolute } from 'node:path';
import { defineConfig, devices } from '@playwright/test';
import baseline from './playwright.config';

// Use pnpm test:commission-responsive <project> to run files in business order.
// A default whole-config invocation sorts files alphabetically and is not this workflow.

const executablePath = process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH;
if (!executablePath || !isAbsolute(executablePath) || !existsSync(executablePath)) {
  throw new Error('Commission responsive acceptance requires an explicit installed Chromium executable');
}
for (const name of ['COMMISSION_FIXTURE_BIN', 'COMMISSION_FIXTURE_CONFIRM', 'TEST_COMMISSION_ADMIN_PASSWORD']) {
  if (!process.env[name]) throw new Error(`${name} is required for commission responsive acceptance`);
}
if (process.env.COMMISSION_FIXTURE_CONFIRM !== 'owned_synthetic_database') {
  throw new Error('COMMISSION_FIXTURE_CONFIRM must be owned_synthetic_database');
}

export default defineConfig({
  ...baseline,
  testMatch: [
    'commission-cycles.spec.ts',
    'commission-payments.spec.ts',
    'commission-adjustments.spec.ts',
    'commission-reports.spec.ts',
    'commission-analysis.spec.ts',
  ],
  fullyParallel: false,
  workers: 1,
  retries: 0,
  use: { ...baseline.use, launchOptions: { executablePath } },
  projects: [
    { name: 'tablet768', use: { ...devices['Desktop Chrome'], viewport: { width: 768, height: 1024 }, hasTouch: true } },
    { name: 'laptop1024', use: { ...devices['Desktop Chrome'], viewport: { width: 1024, height: 768 } } },
  ],
  webServer: [
    { command: 'pnpm dev:user -- --host 127.0.0.1', url: 'http://localhost:5173', reuseExistingServer: false },
    { command: 'pnpm dev:admin -- --host 127.0.0.1', url: 'http://localhost:5174', reuseExistingServer: false },
    { command: 'pnpm dev:platform', url: 'http://127.0.0.1:5175', reuseExistingServer: false },
  ],
});
