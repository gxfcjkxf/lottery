import {test,expect} from '@playwright/test'
import {resolve} from 'node:path'
import {origins,expectNoPageOverflow} from './fixture'
for(const locale of ['en','zh-CN'] as const)test(`official calendar monthly grid and markers ${locale}`,async({page},info)=>{
 const zh=locale==='zh-CN',errors:string[]=[];page.on('pageerror',error=>errors.push(error.message))
 let reads=0
 const calendar={current_month:'2026-10',status:'synced',last_success_at:'2026-10-28T20:00:00Z',last_attempt_at:'2026-10-28T20:00:00Z',next_attempt_at:'2026-10-29T20:00:00Z',months:[{month:'2026-10',draw_dates:['03','06','08','10','13','15','17','20','22','24','27','31'].map(day=>'2026-10-'+day),snowball_dates:['2026-10-31'],presale_dates:['2026-10-30']},{month:'2026-11',draw_dates:['2026-11-03','2026-11-24'],snowball_dates:['2026-11-24'],presale_dates:[]}]}
 await page.route('**/api/**',route=>{expect(route.request().method()).toBe('GET');if(new URL(route.request().url()).pathname.endsWith('/automation'))return route.fulfill({json:{success:true,data:{schedule:{version:1,enabled:true,mode:'hkjc',draw_time:'21:30:00',open_before_hours:24,close_before_seconds:15,collect_after_seconds:90},plan:null,source_check:null,attempts:[],allowed_origins:[]}}});expect(new URL(route.request().url()).pathname).toBe('/api/v1/platform/platform-games/official-calendar');reads++;return route.fulfill({json:{success:true,data:calendar}})})
 const settings={theme:'jade',timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss'}
 await page.route('**/__official-calendar',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app" class="management-console" style="padding:12px"></div><script type="module">import{createApp,h}from'/node_modules/.vite/deps/vue.js';import C from'/src/OfficialDrawCalendar.vue';import S from'/@fs${resolve('shared/src/DrawScheduleSettings.vue')}';import{createPlatformI18n,platformI18nKey}from'/src/i18n.ts';import{setPlatformDisplaySettings}from'/@fs${resolve('shared/src/display-settings.ts')}';import'/@fs${resolve('shared/src/styles.css')}';import'/src/style.css';import'/@fs${resolve('shared/src/management.css')}';import'/@fs${resolve('shared/src/controls.css')}';import'/@fs${resolve('shared/src/refresh-button.css')}';import'/@fs${resolve('shared/src/themes.css')}';setPlatformDisplaySettings(${JSON.stringify(settings)});const i=createPlatformI18n();i.setLocale('${locale}');const app=createApp({render:()=>[h(S,{gameId:'calendar-layout',timezone:'Asia/Hong_Kong',locale:'${locale}',canWrite:true}),h(C,${JSON.stringify({locale,displaySettings:settings,canManage:true})})]});app.provide(platformI18nKey,i);app.mount('#app')</script></html>`}))
 await page.goto(origins.platform+'/__official-calendar');expect(errors).toEqual([])
 await expect(page.getByRole('heading',{name:zh?'官方六合彩搅珠日期':'Official Mark Six draw calendar'})).toBeVisible()
 await expect(page.getByText(zh?'下次计划同步':'Next scheduled sync',{exact:true})).toHaveCount(0)
 const checkPlacement=async()=>{
  const save=await page.getByRole('button',{name:zh?'保存设置':'Save settings',exact:true}).boundingBox()
  const panel=await page.getByTestId('official-calendar').boundingBox()
  const sync=await page.getByRole('button',{name:zh?'立即同步':'Sync now',exact:true}).boundingBox()
  const legend=await page.locator('.calendar-legend').boundingBox()
  expect(panel!.y-save!.y-save!.height).toBeGreaterThanOrEqual(8)
  expect(sync!.y).toBeGreaterThanOrEqual(legend!.y+legend!.height)
  expect(panel!.x+panel!.width-sync!.x-sync!.width).toBeCloseTo(13,0)
 }
 await checkPlacement()
 const refresh=page.locator('.official-calendar-heading [data-refresh-button]');await expect(refresh).toHaveCount(0);
 const grid=page.locator('.calendar-grid');await expect(grid).toBeVisible();await expect(grid.locator('th')).toHaveCount(7);await expect(grid.locator('tbody tr')).toHaveCount(5)
 await expect(grid.locator('time')).toHaveCount(35)
 await expect(grid.locator('time[datetime="2026-10-06"]')).toHaveClass(/calendar-draw/)
 await expect(grid.locator('time[datetime="2026-10-31"]')).toHaveClass(/calendar-snowball/)
 await expect(grid.locator('time[datetime="2026-10-30"]')).toHaveClass(/calendar-presale/)
 await expect(grid.locator('time[datetime="2026-09-27"]')).toHaveText('27')
 await expect(page.getByRole('button',{name:zh?'上个月':'Previous month',exact:true})).toBeDisabled()
 await page.getByRole('button',{name:zh?'下个月':'Next month',exact:true}).click()
 await expect(grid.locator('time[datetime="2026-11-24"]')).toHaveClass(/calendar-snowball/)
 await expect(page.getByRole('button',{name:zh?'下个月':'Next month',exact:true})).toBeDisabled()
 await page.getByRole('button',{name:zh?'上个月':'Previous month',exact:true}).click()
 await expect(grid.locator('time[datetime="2026-10-31"]')).toHaveClass(/calendar-snowball/)
 if(info.project.name.endsWith('reflow'))for(const size of [{width:1440,height:900},{width:768,height:1024},{width:320,height:640}]){await page.setViewportSize(size);await expectNoPageOverflow(page);await checkPlacement()}
 await expect(grid.locator('thead')).toBeVisible();await expectNoPageOverflow(page)
 await page.screenshot({fullPage:true,path:info.outputPath(`official-month-${locale}.png`)})
 expect(reads).toBe(1);expect(errors).toEqual([])
})
