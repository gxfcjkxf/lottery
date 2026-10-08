import { defineConfig } from '@playwright/test';
import base from './playwright.config';

export default defineConfig({
  ...base,
  testMatch: 'report-archive-tasks.spec.ts',
  retries: 0,
  workers: 1,
  reporter: [['list'], ['json', { outputFile: '.local/report-archive-tasks-browser-report.json' }]],
});
