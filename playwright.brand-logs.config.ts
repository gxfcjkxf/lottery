import { defineConfig } from '@playwright/test'
import layout from './playwright.layout.config'
export default defineConfig({
  ...layout,
  testMatch: 'brand-logs.spec.ts',
  projects: layout.projects!.map(project => ({ ...project, testMatch: 'brand-logs.spec.ts' })),
  webServer: [{ command: 'pnpm --filter @lottery/admin-web exec vite --host 127.0.0.1 --port 15274 --strictPort', url: 'http://127.0.0.1:15274', reuseExistingServer: false }],
  outputDir: 'test-results/brand-logs',
  reporter: 'list',
})
