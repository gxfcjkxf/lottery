import { defineConfig, devices } from '@playwright/test'
export default defineConfig({
  testDir:'./tests/layout',testMatch:'pagination.spec.ts',workers:2,retries:0,
  outputDir:'test-results/pagination',reporter:'list',
  use:{trace:'retain-on-failure',screenshot:'only-on-failure'},
  projects:[
    {name:'desktop',use:{...devices['Desktop Chrome']}},
    {name:'android',use:{...devices['Pixel 7']}},
    {name:'android-landscape',use:{...devices['Pixel 7 landscape']}},
    {name:'ios',use:{...devices['iPhone 13']}},
    {name:'ios-landscape',use:{...devices['iPhone 13 landscape']}},
    {name:'reflow',use:{...devices['Desktop Chrome'],viewport:{width:320,height:568}}},
  ],
  webServer:{command:'pnpm --filter @lottery/user-web exec vite --host 127.0.0.1 --port 15473 --strictPort',url:'http://127.0.0.1:15473',reuseExistingServer:false},
})
