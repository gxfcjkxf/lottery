import { defineConfig } from '@playwright/test';
import base from './playwright.config';

const adminOrigin = process.env.TEST_DRAW_NOTICE_ADMIN_ORIGIN;
const userOrigin = process.env.TEST_DRAW_NOTICE_USER_ORIGIN;

export default defineConfig({
  ...base,
  testMatch: 'draw-notifications.spec.ts',
  timeout: 60_000,
  expect: { timeout: 8_000 },
  retries: 0,
  workers: 1,
  webServer: [
    ...(adminOrigin === 'http://localhost:15292' ? [] : [{
      command: 'pnpm dev:admin -- --host 127.0.0.1',
      url: 'http://localhost:5174',
      reuseExistingServer: !process.env.CI,
    }]),
    ...(userOrigin === 'http://localhost:15293' ? [] : [{
      command: 'pnpm dev:user -- --host 127.0.0.1',
      url: 'http://localhost:5173',
      reuseExistingServer: !process.env.CI,
    }]),
  ],
  reporter: [['list'], ['json', { outputFile: '.local/draw-notifications-browser-report.json' }]],
});
