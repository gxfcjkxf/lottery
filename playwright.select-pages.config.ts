import { defineConfig } from '@playwright/test'
import selection from './playwright.select.config'
export default defineConfig({
 ...selection, testDir:'./tests/layout', testMatch:'select-pages.spec.ts', outputDir:'test-results/select-pages',
 webServer:[
  {command:'pnpm --filter @lottery/user-web exec vite --host 127.0.0.1 --port 15273 --strictPort',url:'http://127.0.0.1:15273',reuseExistingServer:false},
  {command:'pnpm --filter @lottery/admin-web exec vite --host 127.0.0.1 --port 15274 --strictPort',url:'http://127.0.0.1:15274',reuseExistingServer:false},
  {command:'pnpm --filter @lottery/platform-web exec vite --host 127.0.0.1 --port 15275 --strictPort',url:'http://127.0.0.1:15275',reuseExistingServer:false},
  {env:{VITE_USER_APP_ORIGIN:'http://127.0.0.1:15273'},command:'pnpm --filter @lottery/agent-web exec vite --host 127.0.0.1 --port 15276 --strictPort',url:'http://127.0.0.1:15276',reuseExistingServer:false},
 ],
})
