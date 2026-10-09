import { defineConfig } from '@playwright/test';
import base from './playwright.config';

export default defineConfig({
  ...base,
  testMatch: 'audit-exports.spec.ts',
  timeout: 60_000,
  expect: { timeout: 8_000 },
  retries: 0,
  workers: 1,
  use: { ...base.use, timezoneId: 'Asia/Singapore' },
  webServer: process.env.TEST_AUDIT_ADMIN_ORIGIN === 'http://localhost:15292' ? [] : [{
    command: 'pnpm dev:admin -- --host 127.0.0.1',
    url: 'http://localhost:5174',
    reuseExistingServer: !process.env.CI,
  }],
  reporter: [['list'], ['json', { outputFile: '.local/audit-exports-browser-report.json' }]],
});
