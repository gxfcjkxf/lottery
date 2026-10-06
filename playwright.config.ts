import { defineConfig, devices } from '@playwright/test'

export default defineConfig({
  testDir: './tests/browser',
  fullyParallel: true,
  timeout: 30_000,
  retries: process.env.CI ? 1 : 0,
  use: {
    // Existing Chinese scenarios explicitly select their UI language. Production
    // defaults and the language-switch scenarios remain independently tested.
    storageState: { cookies: [], origins: [{ origin: 'http://localhost:5174', localStorage: [{ name: 'lottery.admin.locale', value: 'zh-CN' }] }] },
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    launchOptions: {
      executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH,
    },
  },
  projects: [
    { name: 'desktop', use: { ...devices['Desktop Chrome'], viewport: { width: 1440, height: 900 } } },
    { name: 'mobile', use: { ...devices['Desktop Chrome'], viewport: { width: 360, height: 800 }, isMobile: true, hasTouch: true } },
  ],
  webServer: [
    { command: 'pnpm dev:user -- --host 127.0.0.1', url: 'http://localhost:5173', reuseExistingServer: !process.env.CI },
    { command: 'pnpm dev:admin -- --host 127.0.0.1', url: 'http://localhost:5174', reuseExistingServer: !process.env.CI },
  ],
})
