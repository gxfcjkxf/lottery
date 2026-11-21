import {defineConfig,devices} from '@playwright/test'
import {readFileSync} from 'node:fs'
import {resolve} from 'node:path'
const file=process.env.RETRY_REAL_FIXTURE_FILE
if(!file||!process.env.TEST_DATABASE_URL)throw Error('Owned real retry fixture and database required')
const fixture=JSON.parse(readFileSync(file,'utf8'))
const database=new URL(process.env.TEST_DATABASE_URL).pathname.slice(1)
if(!database.startsWith('lottery_test_retry_')||fixture.database!==database)throw Error('Dedicated retry test database required')
const port=Number(process.env.RETRY_REAL_PORT_BASE??16473)
const config=resolve('tests/real-retry/vite.config.mjs')
export default defineConfig({
 testDir:'./tests/real-retry',workers:1,retries:0,timeout:150000,
 outputDir:resolve(`.local/retry-real-20261116/results-${process.env.RETRY_REAL_PROJECT}`),
 reporter:[['list'],['json',{outputFile:resolve(`.local/retry-real-20261116/results-${process.env.RETRY_REAL_PROJECT}.json`)}]],
 use:{serviceWorkers:'block',trace:'retain-on-failure',screenshot:'only-on-failure'},
 projects:[{name:'desktop',use:{...devices['Desktop Chrome']}},{name:'android',use:{...devices['Pixel 7']}},{name:'ios',use:{...devices['iPhone 13'],browserName:'webkit'}}],
 webServer:['user','admin','platform','agent'].map((app,i)=>({command:`pnpm --filter @lottery/${app}-web exec vite --config ${config} --host 127.0.0.1 --port ${port+i} --strictPort`,url:`http://127.0.0.1:${port+i}`,reuseExistingServer:false,env:{RETRY_REAL_API_URL:fixture.url}}))
})
