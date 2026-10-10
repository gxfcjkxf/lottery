import { existsSync } from 'node:fs'
import { isAbsolute } from 'node:path'
import { defineConfig, devices } from '@playwright/test'

const executablePath = process.env.PLAYWRIGHT_WEBKIT_EXECUTABLE_PATH
if (!executablePath || !isAbsolute(executablePath) || !existsSync(executablePath)) {
  throw new Error('PLAYWRIGHT_WEBKIT_EXECUTABLE_PATH must be an absolute path to an existing WebKit executable.')
}

for (const name of [
  'TEST_ADMIN_USERNAME',
  'TEST_ADMIN_PASSWORD',
  'TEST_HARBOR_ADMIN_USERNAME',
  'TEST_HARBOR_ADMIN_PASSWORD',
  'TEST_COMPLIANCE_ADMIN_USERNAME',
  'TEST_COMPLIANCE_ADMIN_PASSWORD',
]) {
  if (!process.env[name]) throw new Error(`${name} is required to run the WebKit browser suite.`)
}

export default defineConfig({
  testDir: './tests/browser',
  testMatch: ['auth.spec.ts', 'management.spec.ts', 'compliance.spec.ts'],
  fullyParallel: false,
  workers: 1,
  timeout: 30_000,
  retries: 0,
  use: {
    browserName: 'webkit',
    storageState: { cookies: [], origins: [{ origin: 'http://localhost:5174', localStorage: [{ name: 'lottery.admin.locale', value: 'zh-CN' }] }] },
    trace: 'retain-on-failure',
    launchOptions: { executablePath },
    serviceWorkers: 'block',
  },
  projects: [
    { name: 'desktop', use: { ...devices['Desktop Safari'], viewport: { width: 1440, height: 900 } } },
    { name: 'mobile', use: { ...devices['iPhone 13'], viewport: { width: 360, height: 800 } } },
  ],
  webServer: [
    { command: 'pnpm dev:user -- --host 127.0.0.1', url: 'http://localhost:5173', reuseExistingServer: false },
    { command: 'pnpm dev:admin -- --host 127.0.0.1', url: 'http://localhost:5174', reuseExistingServer: false },
    { command: 'pnpm dev:platform', url: 'http://127.0.0.1:5175', reuseExistingServer: false },
  ],
})
