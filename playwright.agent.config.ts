import {defineConfig,devices} from '@playwright/test'
import {readFileSync} from 'node:fs'
if(!process.env.AGENT_BROWSER_FIXTURE_FILE || !process.env.TEST_DATABASE_URL)throw Error('Owned agent fixture and test database are required')
const fixture=JSON.parse(readFileSync(process.env.AGENT_BROWSER_FIXTURE_FILE,'utf8'))
const database=new URL(process.env.TEST_DATABASE_URL).pathname.slice(1)
if(!database.includes('_test_') || fixture.database!==database)throw Error('Independent agent test database required')
process.env.AGENT_API_URL=fixture.url
export default defineConfig({testDir:'./tests/agent-portal',globalTeardown:'./tests/agent-portal/teardown.ts',workers:1,timeout:45000,outputDir:'test-results/agent-portal',reporter:'list',use:{baseURL:'http://127.0.0.1:15276',screenshot:'only-on-failure',trace:'retain-on-failure'},projects:[
 {name:'android',use:{...devices['Pixel 7'],contextOptions:{screen:devices['Pixel 7'].screen}}},{name:'android-landscape',use:{...devices['Pixel 7 landscape'],contextOptions:{screen:devices['Pixel 7 landscape'].screen}}},
 {name:'ios',use:{...devices['iPhone 13'],contextOptions:{screen:devices['iPhone 13'].screen},browserName:'webkit'}},{name:'ios-landscape',use:{...devices['iPhone 13 landscape'],contextOptions:{screen:devices['iPhone 13 landscape'].screen},browserName:'webkit'}},
 {name:'ios-320',use:{...devices['iPhone SE'],contextOptions:{screen:devices['iPhone SE'].screen},browserName:'webkit'}},{name:'desktop',use:{...devices['Desktop Chrome']}}
],webServer:{command:'pnpm --filter @lottery/agent-web exec vite --host 127.0.0.1 --port 15276 --strictPort',url:'http://127.0.0.1:15276',reuseExistingServer:false}})
