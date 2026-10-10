import { existsSync } from 'node:fs'
import { isAbsolute } from 'node:path'
import { defineConfig, devices } from '@playwright/test'
import baseline from './playwright.config'

const executablePath = process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH
if (!executablePath || !isAbsolute(executablePath) || !existsSync(executablePath)) throw new Error('Qualification acceptance requires an explicit installed Chromium executable')
for (const name of ['TEST_QUAL_ADMIN_PASSWORD', 'TEST_QUAL_USER_PASSWORD']) {
  if (!process.env[name]) throw new Error(`${name} is required for real qualification acceptance`)
}

export default defineConfig({
  ...baseline,
  testMatch: ['withdrawal-qualification.spec.ts'],
  fullyParallel: false,
  workers: 1,
  retries: 0,
  use: { ...baseline.use, launchOptions: { executablePath } },
  projects: [
    { name: 'desktop', use: { ...devices['Desktop Chrome'], viewport: { width: 1440, height: 900 } } },
    { name: 'mobile', use: { ...devices['Desktop Chrome'], viewport: { width: 360, height: 800 }, isMobile: true, hasTouch: true } },
    { name: 'tablet768', use: { ...devices['Desktop Chrome'], viewport: { width: 768, height: 1024 }, hasTouch: true } },
    { name: 'laptop1024', use: { ...devices['Desktop Chrome'], viewport: { width: 1024, height: 768 } } },
  ],
  webServer: [
    { command: 'pnpm dev:user -- --host 127.0.0.1', url: 'http://localhost:5173', reuseExistingServer: false },
    { command: 'pnpm dev:admin -- --host 127.0.0.1', url: 'http://localhost:5174', reuseExistingServer: false },
    { command: 'pnpm dev:platform', url: 'http://127.0.0.1:5175', reuseExistingServer: false },
  ],
})
