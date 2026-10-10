import { existsSync } from 'node:fs'
import { isAbsolute } from 'node:path'
import { defineConfig, devices } from '@playwright/test'
import baseline from './playwright.config'

const executablePath = process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH
if (!executablePath || !isAbsolute(executablePath) || !existsSync(executablePath)) {
  throw new Error('Responsive acceptance requires an explicit installed Chromium executable')
}
for (const name of ['TEST_HARBOR_ADMIN_USERNAME', 'TEST_HARBOR_ADMIN_PASSWORD', 'TEST_RULE_REVIEWER_USERNAME', 'TEST_RULE_REVIEWER_PASSWORD']) {
  if (!process.env[name]) throw new Error(`${name} is required for real responsive betting acceptance`)
}

export default defineConfig({
  ...baseline,
  testMatch: ['auth.spec.ts', 'betting.spec.ts'],
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
})
