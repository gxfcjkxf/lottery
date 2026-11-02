import { defineConfig, devices } from '@playwright/test'

export default defineConfig({
  testDir: './tests/deployment',
  workers: 1,
  retries: 0,
  timeout: 120_000,
  outputDir: 'test-results/deployment',
  reporter: [['list']],
  use: { actionTimeout: 10_000, navigationTimeout: 10_000, serviceWorkers: 'block', trace: 'retain-on-failure', screenshot: 'only-on-failure' },
  projects: [
    { name: 'desktop', use: { ...devices['Desktop Chrome'], launchOptions: { executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH } } },
    { name: 'android', use: { ...devices['Pixel 7'], launchOptions: { executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH } } },
    { name: 'ios', use: { ...devices['iPhone 13'], launchOptions: { executablePath: process.env.PLAYWRIGHT_WEBKIT_EXECUTABLE_PATH } } },
    { name: 'ios-landscape', use: { ...devices['iPhone 13 landscape'], launchOptions: { executablePath: process.env.PLAYWRIGHT_WEBKIT_EXECUTABLE_PATH } } },
  ],
})
