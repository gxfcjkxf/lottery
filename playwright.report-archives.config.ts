import { defineConfig } from '@playwright/test';
import base from './playwright.config';

export default defineConfig({
  ...base,
  testMatch: 'report-archives.spec.ts',
  retries: 0,
  workers: 1,
  reporter: [
    ['list'],
    ['json', { outputFile: '.local/report-archives-browser-report.json' }],
  ],
});
