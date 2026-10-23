import { defineConfig, devices } from '@playwright/test'

// Device presets retain viewport, screen, DPR, touch and user-agent together.
// Reflow projects deliberately vary the viewport; they do not represent a handset.
const chromium = { browserName: 'chromium' as const, launchOptions: { executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH } }
const webkit = { browserName: 'webkit' as const, launchOptions: { executablePath: process.env.PLAYWRIGHT_WEBKIT_EXECUTABLE_PATH } }

export default defineConfig({
  testDir: './tests/layout',
  fullyParallel: false,
  workers: 2,
  retries: 0,
  timeout: 60_000,
  outputDir: 'test-results/layout',
  reporter: [['list'], ['json', { outputFile: 'test-results/layout/results.json' }]],
  use: { trace: 'retain-on-failure', screenshot: 'only-on-failure', serviceWorkers: 'block' },
  projects: [
    { name: 'android-pixel7', testMatch: 'device.spec.ts', use: { ...devices['Pixel 7'], contextOptions: { screen: devices['Pixel 7'].screen }, ...chromium } },
    { name: 'android-pixel7-landscape', testMatch: 'device.spec.ts', use: { ...devices['Pixel 7 landscape'], contextOptions: { screen: devices['Pixel 7 landscape'].screen }, ...chromium } },
    { name: 'ios-iphone-se', testMatch: 'device.spec.ts', use: { ...devices['iPhone SE'], ...webkit } },
    { name: 'ios-iphone13', testMatch: 'device.spec.ts', use: { ...devices['iPhone 13'], contextOptions: { screen: devices['iPhone 13'].screen }, ...webkit } },
    { name: 'ios-iphone13-large', testMatch: 'device.spec.ts', use: { ...devices['iPhone 13 Pro Max'], contextOptions: { screen: devices['iPhone 13 Pro Max'].screen }, ...webkit } },
    { name: 'ios-iphone13-landscape', testMatch: 'device.spec.ts', use: { ...devices['iPhone 13 landscape'], contextOptions: { screen: devices['iPhone 13 landscape'].screen }, ...webkit } },
    { name: 'chromium-reflow', testMatch: ['reflow.spec.ts', 'loading.spec.ts'], use: { ...devices['Desktop Chrome'], ...chromium } },
    { name: 'webkit-reflow', testMatch: ['reflow.spec.ts', 'loading.spec.ts'], use: { ...devices['Desktop Safari'], ...webkit } },
  ],
  webServer: [
    { env: {VITE_USER_APP_ORIGIN: `http://127.0.0.1:${process.env.LOTTERY_LAYOUT_PORT_BASE ?? 15273}`}, command: 'pnpm --filter @lottery/agent-web exec vite --host 127.0.0.1 --port 15276 --strictPort', url: 'http://127.0.0.1:15276', reuseExistingServer: false },
    { command: 'pnpm --filter @lottery/user-web exec vite --host 127.0.0.1 --port 15273 --strictPort', url: 'http://127.0.0.1:15273', reuseExistingServer: false },
    { command: 'pnpm --filter @lottery/admin-web exec vite --host 127.0.0.1 --port 15274 --strictPort', url: 'http://127.0.0.1:15274', reuseExistingServer: false },
    { command: 'pnpm --filter @lottery/platform-web exec vite --host 127.0.0.1 --port 15275 --strictPort', url: 'http://127.0.0.1:15275', reuseExistingServer: false },
  ],
})
