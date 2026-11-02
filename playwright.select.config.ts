import { defineConfig, devices } from '@playwright/test'
export default defineConfig({
  testDir: './tests/layout', testMatch: ['select-control.spec.ts', 'select-features.spec.ts'], workers: 2,
  outputDir: 'test-results/select', reporter: 'list',
  use: { screenshot:'only-on-failure',trace:'retain-on-failure' },
  projects: [
    {name:'desktop-chrome',use:{...devices['Desktop Chrome']}},
    {name:'desktop-safari',use:{...devices['Desktop Safari']}},
    {name:'android',use:{...devices['Pixel 7']}},
    {name:'android-landscape',use:{...devices['Pixel 7 landscape']}},
    {name:'ios',use:{...devices['iPhone 13']}},
    {name:'ios-landscape',use:{...devices['iPhone 13 landscape']}},
    {name:'reflow-320',use:{...devices['Desktop Chrome'],viewport:{width:320,height:568}}},
  ],
  webServer:{command:'pnpm --filter @lottery/user-web exec vite --host 127.0.0.1 --port 15373 --strictPort',url:'http://127.0.0.1:15373',reuseExistingServer:false},
})
