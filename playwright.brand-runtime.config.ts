import { defineConfig } from '@playwright/test'
import layout from './playwright.layout.config'
export default defineConfig({
  ...layout,
  projects: layout.projects!.map(project => ({ ...project, testMatch: 'brand-runtime.spec.ts' })),
  webServer: [{ command: 'pnpm --filter @lottery/admin-web exec vite preview --host 127.0.0.1 --port 15274 --strictPort', url: 'http://127.0.0.1:15274', reuseExistingServer: false }],
  outputDir: 'test-results/brand-runtime',
  reporter: 'list',
})
