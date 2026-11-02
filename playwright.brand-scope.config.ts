import {defineConfig,devices} from '@playwright/test'
import {readFileSync} from 'node:fs'
if(!process.env.BRAND_SCOPE_FIXTURE_FILE||!process.env.TEST_DATABASE_URL)throw Error('Isolated brand scope fixture required')
const fixture=JSON.parse(readFileSync(process.env.BRAND_SCOPE_FIXTURE_FILE,'utf8'))
const database=new URL(process.env.TEST_DATABASE_URL).pathname.slice(1)
if(!database.includes('_test_')||fixture.database!==database)throw Error('Independent test database required')
export default defineConfig({testDir:'./tests/brand-scope',workers:1,timeout:30000,outputDir:'.local/brand-scope-fix/browser-results',reporter:'list',use:{baseURL:'http://localhost:15684',trace:'retain-on-failure',screenshot:'only-on-failure'},projects:[
{name:'android',use:{...devices['Pixel 7']}},{name:'android-landscape',use:{...devices['Pixel 7 landscape']}},
{name:'ios',use:{...devices['iPhone 13'],browserName:'webkit'}},{name:'ios-landscape',use:{...devices['iPhone 13 landscape'],browserName:'webkit'}},
{name:'ios-320',use:{...devices['iPhone SE'],browserName:'webkit'}},{name:'desktop',use:{...devices['Desktop Chrome']}}
],webServer:{command:'pnpm --filter @lottery/admin-web exec vite --config ../tests/brand-scope/vite.config.ts',url:'http://127.0.0.1:15684',reuseExistingServer:false}})
