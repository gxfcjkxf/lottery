import {defineConfig,devices} from '@playwright/test'
export default defineConfig({
 testDir:'./tests/layout',testMatch:'data-retention.spec.ts',outputDir:'test-results/data-retention',fullyParallel:false,workers:2,retries:0,timeout:60000,
 reporter:[['list'],['json',{outputFile:'test-results/data-retention/results.json'}]],
 use:{baseURL:'http://127.0.0.1:15385',serviceWorkers:'block',trace:'retain-on-failure'},
 projects:[
  {name:'desktop',use:{...devices['Desktop Chrome']}},
  {name:'android',use:{...devices['Pixel 7']}},
  {name:'android-landscape',use:{...devices['Pixel 7 landscape']}},
  {name:'ios',use:{...devices['iPhone 13']}},
  {name:'ios-landscape',use:{...devices['iPhone 13 landscape']}},
  {name:'chromium-reflow',use:{...devices['Desktop Chrome']}},
  {name:'webkit-reflow',use:{...devices['Desktop Safari']}},
 ],
 webServer:{command:'pnpm --filter @lottery/platform-web exec vite --host 127.0.0.1 --port 15385 --strictPort',url:'http://127.0.0.1:15385',reuseExistingServer:false},
})
