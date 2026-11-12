import { defineConfig } from '@playwright/test'
import base from './playwright.cash.config'
export default defineConfig({
  ...base,
  testMatch: 'auth-consent.spec.ts',
  outputDir: '.local/auth-consent/browser-results',
  reporter: [['list'], ['json', { outputFile: '.local/auth-consent/results.json' }]],
})
