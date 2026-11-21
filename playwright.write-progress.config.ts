import {defineConfig,devices} from '@playwright/test'
import base from './playwright.layout.config'
export default defineConfig({
 ...base,
 testMatch:['write-progress*.spec.ts','cash-wallet.spec.ts'],
 grep:/write progress|agent shared create fields retain frozen requests|configured sales restore an uncertain purchase|profile payment account CRUD/,
 outputDir:'test-results/write-progress',
 reporter:[['list'],['json',{outputFile:'test-results/write-progress/results.json'}]],
 projects:[
  {name:'desktop',use:{...devices['Desktop Chrome']}},
  {name:'android',use:{...devices['Pixel 7']}},
  {name:'android-landscape',use:{...devices['Pixel 7 landscape']}},
  {name:'ios',use:{...devices['iPhone 13'],browserName:'webkit'}},
  {name:'ios-landscape',use:{...devices['iPhone 13 landscape'],browserName:'webkit'}},
  {name:'chromium-reflow',use:{...devices['Desktop Chrome'],viewport:{width:320,height:640}}},
  {name:'webkit-reflow',use:{...devices['Desktop Safari'],viewport:{width:320,height:640}}},
 ],
})
