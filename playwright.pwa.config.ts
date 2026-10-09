import { defineConfig, devices } from '@playwright/test'

const chromiumPath = process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH
if (!chromiumPath) {
  throw new Error('PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH is required to run the production PWA suite.')
}

export default defineConfig({
  testDir: './tests/pwa',
  fullyParallel: true,
  timeout: 30_000,
  retries: 0,
  workers: 2,
  use: {
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    launchOptions: { executablePath: chromiumPath },
  },
  projects: [
    {
      name: 'user-desktop1440',
      use: { ...devices['Desktop Chrome'], viewport: { width: 1440, height: 900 }, baseURL: 'http://127.0.0.1:15173' },
    },
    {
      name: 'user-mobile360',
      use: { ...devices['Desktop Chrome'], viewport: { width: 360, height: 800 }, isMobile: true, hasTouch: true, baseURL: 'http://127.0.0.1:15173' },
    },
    {
      name: 'brand-desktop1440',
      use: { ...devices['Desktop Chrome'], viewport: { width: 1440, height: 900 }, baseURL: 'http://127.0.0.1:15174' },
    },
    {
      name: 'brand-mobile360',
      use: { ...devices['Desktop Chrome'], viewport: { width: 360, height: 800 }, isMobile: true, hasTouch: true, baseURL: 'http://127.0.0.1:15174' },
    },
    {
      name: 'platform-desktop1440',
      use: { ...devices['Desktop Chrome'], viewport: { width: 1440, height: 900 }, baseURL: 'http://127.0.0.1:15175' },
    },
    {
      name: 'platform-mobile360',
      use: { ...devices['Desktop Chrome'], viewport: { width: 360, height: 800 }, isMobile: true, hasTouch: true, baseURL: 'http://127.0.0.1:15175' },
    },
  ],
  webServer: [
    {
      command: 'pnpm --filter @lottery/user-web exec vite preview --config ../vite.pwa-preview.config.mjs --host 127.0.0.1 --port 15173 --strictPort',
      url: 'http://127.0.0.1:15173',
      timeout: 30_000,
      reuseExistingServer: false,
    },
    {
      command: 'pnpm --filter @lottery/admin-web exec vite preview --config ../vite.pwa-preview.config.mjs --host 127.0.0.1 --port 15174 --strictPort',
      url: 'http://127.0.0.1:15174',
      timeout: 30_000,
      reuseExistingServer: false,
    },
    {
      command: 'pnpm --filter @lottery/platform-web exec vite preview --config ../vite.pwa-preview.config.mjs --host 127.0.0.1 --port 15175 --strictPort',
      url: 'http://127.0.0.1:15175',
      timeout: 30_000,
      reuseExistingServer: false,
    },
  ],
})
