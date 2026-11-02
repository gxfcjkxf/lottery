import { selectControl } from '../select-control'
import {test,expect} from '@playwright/test'
import {resolve} from 'node:path'
import {origins,expectNoPageOverflow} from './fixture'
for(const locale of ['en','zh-CN'] as const)for(const twelve of [false,true])test(`archive schedule uses platform minute controls ${locale} ${twelve?'12h':'24h'}`,async({page},info)=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message))
 const settings={timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:twelve?'hh:mm:ss A':'HH:mm:ss'}
 const initial={daily_enabled:true,monthly_enabled:true,daily_execution_time:'07:30',daily_period_offset:1,monthly_execution_day:5,monthly_execution_time:'06:00',monthly_period_offset:2}
 await page.route('**/api/**',route=>{throw Error('Schedule editing must not issue requests: '+route.request().url())})
 await page.route('**/__schedule',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app" class="management-console"></div><script type="module">import{createApp,h,ref}from'/node_modules/.vite/deps/vue.js';import C from'/@fs${resolve('shared/src/ArchiveScheduleForm.vue')}';import{setPlatformDisplaySettings}from'/@fs${resolve('shared/src/display-settings.ts')}';import'/@fs${resolve('shared/src/styles.css')}';import'/@fs${resolve('shared/src/management.css')}';import'/@fs${resolve('shared/src/controls.css')}';setPlatformDisplaySettings(${JSON.stringify(settings)});createApp({setup(){const model=ref(${JSON.stringify(initial)});return()=>h('main',{style:'padding:12px;min-width:0'},[h(C,{modelValue:model.value,locale:'${locale}','onUpdate:modelValue':value=>model.value=value}),h('output',{id:'model',hidden:true},JSON.stringify(model.value))])}}).mount('#app')</script></html>`}))
 await page.goto(origins.platform+'/__schedule')
 const daily=page.getByTestId('archive-policy-daily-time'),monthly=page.getByTestId('archive-policy-monthly-time')
 await expect(daily.locator(".select-control-trigger")).toHaveCount(twelve?3:2)
 await expect(page.locator('input[type=time],input[type=datetime-local]')).toHaveCount(0)
 await selectControl(daily.locator(".select-control-trigger").nth(0), twelve?'11':'23');await selectControl(daily.locator(".select-control-trigger").nth(1), '45')
 if(twelve)await selectControl(daily.locator(".select-control-trigger").nth(2), 'PM')
 await expect(page.locator('#model')).toContainText('"daily_execution_time":"23:45"')
 await expect(page.locator('#model')).toContainText('"monthly_execution_time":"06:00"')
 await expect(monthly.locator(".select-control-trigger").nth(0)).toHaveValue('06')
 const preset=page.viewportSize()!
 for(const width of info.project.name.endsWith('reflow')?[320,700,701,900,1280]:[preset.width]){
  if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:preset.height})
  await expectNoPageOverflow(page)
 }
 await page.screenshot({path:info.outputPath(`schedule-${locale}-${twelve?'12':'24'}.png`)})
 expect(errors).toEqual([])
})
