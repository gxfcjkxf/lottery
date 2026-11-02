import { defineConfig, devices } from '@playwright/test'
const port = process.env.LOTTERY_LAYOUT_PORT_BASE ?? '15273'
export default defineConfig({
  testDir: './tests/layout', testMatch: ['configured-rules.spec.ts','rules-simplify.spec.ts','platform-game-readiness.spec.ts','configured-business.spec.ts','draw-config.spec.ts','role-permissions.spec.ts'], workers: 2, timeout: 60_000,
  outputDir: 'test-results/configured-rules', use: { screenshot: 'only-on-failure', trace: 'retain-on-failure', serviceWorkers: 'block' },
  projects: [
    { name: 'android', use: { ...devices['Pixel 7'] } },
    { name: 'android-landscape', use: { ...devices['Pixel 7 landscape'] } },
    { name: 'ios', use: { ...devices['iPhone 13'] } },
    { name: 'ios-landscape', use: { ...devices['iPhone 13 landscape'] } },
    { name: 'reflow-320', use: { ...devices['Desktop Chrome'], viewport: { width: 320, height: 720 } } },
    { name: 'desktop', use: { ...devices['Desktop Chrome'] } },
  ],
  webServer: { command: `pnpm --filter @lottery/user-web exec vite --host 127.0.0.1 --port ${port} --strictPort`, url: `http://127.0.0.1:${port}`, reuseExistingServer: false },
})
