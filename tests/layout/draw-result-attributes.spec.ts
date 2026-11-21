import {selectControl} from '../select-control'
import {test,expect} from '@playwright/test'
import {resolve} from 'node:path'
import {origins,gameId,expectNoPageOverflow} from './fixture'
for(const locale of ['zh-CN','en'] as const) test(`draw result saved attributes ${locale}`,async({page},info)=>{
 const numbers=[29,14,21,22,11,17,40],waves=['red','blue','green','green','green','green','red'],zodiacs=['tiger','snake','dog','rooster','monkey','tiger','rabbit']
 const attributes=Object.fromEntries(numbers.map((n,i)=>[String(n).padStart(2,'0'),{wave:waves[i],zodiac:zodiacs[i],element:'wood'}]))
 const errors:string[]=[];page.on('pageerror',error=>errors.push(error.message))
 await page.route('**/api/**',route=>{
  expect(route.request().method()).toBe('GET')
  expect(new URL(route.request().url()).pathname).toBe(`/api/v1/platform/platform-games/${gameId}/draw-overview`)
  return route.fulfill({json:{success:true,data:{mark_six:true,total_count:'1',years:['2026'],year:'2026',has_more:false,pending_has_more:false,pending:[],items:[{period_no:'2026118',draw_date:'2026-11-07',source:'hkjc',result:{regular:numbers.slice(0,6),special:numbers.slice(6),digits:[]},attributes}]}}})
 })
 await page.route('**/__draw-attributes',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app" class="management-console"></div><script type="module">import {createApp,h} from '/node_modules/.vite/deps/vue.js';import C from '/@fs${resolve('shared/src/PlatformDrawOverview.vue')}';import {setPlatformDisplaySettings} from '/@fs${resolve('shared/src/display-settings.ts')}';import '/@fs${resolve('shared/src/styles.css')}';import '/@fs${resolve('shared/src/management.css')}';import '/@fs${resolve('shared/src/controls.css')}';setPlatformDisplaySettings({timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss'});createApp({render:()=>h('main',{style:'padding:8px;min-width:0'},[h(C,{gameId:'${gameId}',locale:'${locale}',canManage:false})])}).mount('#app')</script></html>`}))
 await page.goto(origins.platform+'/__draw-attributes')
 await expect(page.locator('.draw-zodiac')).toHaveText(locale==='zh-CN'?['虎','蛇','狗','鸡','猴','虎','兔']:['Tiger','Snake','Dog','Rooster','Monkey','Tiger','Rabbit'])
 const balls=page.locator('.draw-ball')
 for(let i=0;i<7;i++)await expect(balls.nth(i)).toHaveClass(new RegExp(`lottery-wave-${waves[i]}`))
 const preset=page.viewportSize()!
 for(const width of info.project.name.includes('reflow')?[320,620,900,1000,1440]:[preset.width]){
  if(info.project.name.includes('reflow'))await page.setViewportSize({width,height:preset.height})
  await expectNoPageOverflow(page)
  expect(await balls.first().evaluate(el=>{const probe=document.createElement('span');probe.style.color='var(--lottery-wave-red)';el.append(probe);const color=getComputedStyle(probe).color;probe.remove();return getComputedStyle(el).backgroundColor===color})).toBe(true)
  await page.screenshot({path:info.outputPath(`draw-attributes-${locale}-${width}.png`),fullPage:true})
 }
 expect(errors).toEqual([])
})

for(const surface of ['admin','platform'] as const) for(const locale of ['zh-CN','en'] as const) test(`canada draw time to seconds ${surface} ${locale}`,async({page},info)=>{
 await page.clock.install({time:new Date('2026-11-14T03:28:40Z')})
 let published=false, newest=3506124, failRecent100=true
 let releaseRefresh:(()=>void)|undefined
 const requests:string[]=[]
 await page.route('**/api/**',async route=>{
  const query=new URL(route.request().url()).searchParams
  requests.push(query.get('recent') ? `recent-${query.get('recent')}` : query.get('date')||'')
  expect(query.get('year')).toBe('')
  const recent=Number(query.get('recent')||(!query.get('date')?50:0))
  if(recent===100 && failRecent100){failRecent100=false;return route.fulfill({status:503,json:{success:false,error:{code:'SERVICE_UNAVAILABLE',message:'Unavailable'}}})}
  const selected=recent ? '' : query.get('date')||'2026-11-14'
  if(published && !releaseRefresh) await new Promise<void>(resolve=>{releaseRefresh=resolve})
  return route.fulfill({json:{success:true,data:{recent_supported:true,recent,daily:true,today:'2026-11-14',yesterday:'2026-11-13',date:selected,mark_six:false,total_count:recent?String(recent):'1',years:[],year:'',has_more:false,pending_has_more:false,pending:[],items:recent?Array.from({length:recent},(_,index)=>({period_no:String(newest-index),draw_date:null,draw_at:`2026-11-${index<25?'14':'13'}T03:28:37Z`,drawn_at:`2026-11-${index<25?'14':'13'}T03:28:43Z`,regular_order:null,source:'published',attributes:{},result:{regular:[],special:[],digits:[1,2,3]}})):[{period_no:published?'3506124':'3506123',draw_date:selected,draw_at:`${selected}T03:28:37Z`,source:'published',result:{regular:[],special:[],digits:[1,2,3]},attributes:{}}]}}})
 })
 await page.route('**/__draw-time',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app" class="management-console"></div><script type="module">import {createApp,h} from '/node_modules/.vite/deps/vue.js';import C from '/@fs${resolve('shared/src/PlatformDrawOverview.vue')}';import {setPlatformDisplaySettings} from '/@fs${resolve('shared/src/display-settings.ts')}';import '/@fs${resolve('shared/src/styles.css')}';import '/@fs${resolve('shared/src/management.css')}';import '/@fs${resolve('shared/src/controls.css')}';setPlatformDisplaySettings({timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss'});createApp({render:()=>h('main',{style:'padding:8px;min-width:0'},[h(C,{gameId:'${gameId}',locale:'${locale}',surface:'${surface}',brandId:'test-brand',canManage:false})])}).mount('#app')</script></html>`}))
 await page.goto(origins.platform+'/__draw-time')
 const initialView=page.getByRole('combobox',{name:locale.startsWith('zh')?'查看方式':'View',exact:true})
 await expect(initialView).toHaveAttribute('data-value','recent-50')
 await expect(page.locator('.draw-overview tbody tr')).toHaveCount(50)
 await expectNoPageOverflow(page)
 await page.screenshot({path:info.outputPath(`default-recent-50-${locale}.png`)})
 await selectControl(initialView,'date')
 await expect(page.getByText('2026-11-14 11:28:37',{exact:true})).toBeVisible()
 await expect(page.getByText(locale==='zh-CN'?'开奖时间':'Draw time',{exact:true}).first()).toBeAttached()
 const preset=page.viewportSize()!
 for(const width of info.project.name.includes('reflow')?[320,620,900,1440]:[preset.width]){
  if(info.project.name.includes('reflow'))await page.setViewportSize({width,height:preset.height})
  await expectNoPageOverflow(page)
  await page.screenshot({path:info.outputPath(`draw-time-${locale}-${width}.png`),fullPage:true})
 }
 published=true
 await page.clock.runFor(5000)
 await expect.poll(()=>!!releaseRefresh).toBe(true)
 const pendingReads=requests.length
 await page.clock.runFor(10000)
 expect(requests.length).toBe(pendingReads)
 await expect(page.locator('[aria-busy]').first()).toHaveAttribute('aria-busy','false')
 releaseRefresh!()
 await expect(page.getByText('3506124',{exact:true}).or(page.getByText('3506124期',{exact:true}))).toBeVisible()
 const zh=locale==='zh-CN'
 await page.getByRole('button',{name:zh?'昨天':'Yesterday',exact:true}).click()
 await expect(page.getByText('2026-11-13 11:28:37',{exact:true})).toBeVisible()
 const historyReads=requests.length
 await page.clock.runFor(10000)
 expect(requests.length).toBe(historyReads)
 await page.getByRole('button',{name:zh?'当天':'Today',exact:true}).click()
 await expect(page.getByText('2026-11-14 11:28:37',{exact:true})).toBeVisible()
 await page.getByRole('button',{name:zh?'日期':'Date',exact:true}).click()
 await page.getByRole('button',{name:'2026-11-12',exact:true}).click()
 await expect(page.getByText('2026-11-12 11:28:37',{exact:true})).toBeVisible()
 const view=page.getByRole('combobox',{name:zh?'查看方式':'View',exact:true})
 await selectControl(view,'recent-50')
 const records=page.locator('.draw-overview tbody tr')
 await expect(records).toHaveCount(50)
 await expect(page.locator('.platform-date-input-display')).toHaveCount(0)
 newest=3506125
 await page.clock.runFor(5000)
 await expect(records.first()).toContainText('3506125')
 await selectControl(view,'recent-100')
 await expect(view).toHaveAttribute('data-value','recent-50')
 await expect(records).toHaveCount(50)
 await expect(page.locator('.draw-overview [role=alert]')).toHaveCount(0)
 await page.clock.runFor(5000)
 await expect(view).toHaveAttribute('data-value','recent-100')
 await expect(records).toHaveCount(100)
 await expect(records.last()).toContainText('3506026')
 await expectNoPageOverflow(page)
 await page.screenshot({path:info.outputPath(`recent-100-${locale}.png`)})
 await selectControl(view,'date')
 await expect(page.locator('.platform-date-input-display')).toHaveValue('2026-11-14')
 expect(requests).toContain('recent-50');expect(requests).toContain('recent-100')
 expect(requests).toContain('2026-11-13');expect(requests).toContain('2026-11-14');expect(requests).toContain('2026-11-12')
})

for(const locale of ['zh','en'] as const) test(`user canada draw time to seconds ${locale}`,async({page},info)=>{
 await page.clock.install({time:new Date('2026-11-14T03:28:40Z')})
 const errors:string[]=[];page.on('pageerror',error=>errors.push(error.message))
 let published=false, newest=3506124, failRecent100=true
 let releaseRefresh:(()=>void)|undefined
 const requests:string[]=[]
 await page.route('**/api/**',async route=>{
  const query=new URL(route.request().url()).searchParams
  requests.push(query.get('recent') ? `recent-${query.get('recent')}` : query.get('selected')||'')
  const recent=Number(query.get('recent')||(!query.get('selected')?50:0))
  if(recent===100 && failRecent100){failRecent100=false;return route.fulfill({status:503,json:{success:false,error:{code:'SERVICE_UNAVAILABLE',message:'Unavailable'}}})}
  const selected=recent ? '' : query.get('selected')||'2026-11-14'
  if(published && !releaseRefresh) await new Promise<void>(resolve=>{releaseRefresh=resolve})
  return route.fulfill({json:{success:true,data:{game_id:gameId,game_name:'Canada 28',mark_six:false,timezone:'Asia/Singapore',recent_supported:true,recent,daily:true,today:'2026-11-14',yesterday:'2026-11-13',total_count:recent?String(recent):'1',choices:['2026-11-14','2026-11-13'],selected,has_more:false,items:recent?Array.from({length:recent},(_,index)=>({period_no:String(newest-index),draw_date:null,draw_at:`2026-11-${index<25?'14':'13'}T03:28:37Z`,drawn_at:`2026-11-${index<25?'14':'13'}T03:28:43Z`,regular_order:null,source:'published',attributes:{},result:{regular:[],special:[],digits:[1,2,3]}})):[{period_no:published?'3506124':'3506123',draw_date:null,draw_at:`${selected}T03:28:37Z`,drawn_at:`${selected}T03:28:43Z`,regular_order:null,result:{regular:[],special:[],digits:[1,2,3]},attributes:{}}]}}})
 })
 await page.route('**/__user-draw-time',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><div id="user-page-refresh"></div><script type="module">import {createApp} from '/node_modules/.vite/deps/vue.js';import C from '/@fs${resolve('user-web/src/DrawArchive.vue')}';import {createUserI18n,userI18nKey} from '/src/i18n.ts';import {setPlatformDisplaySettings} from '/@fs${resolve('shared/src/display-settings.ts')}';import '/@fs${resolve('shared/src/styles.css')}';import '/@fs${resolve('shared/src/controls.css')}';setPlatformDisplaySettings({timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss'});const ui=createUserI18n();ui.setLocale('${locale==='zh'?'zh-CN':'en'}');createApp(C,{gameId:'${gameId}',kind:'configured',locale:'${locale}'}).provide(userI18nKey,ui).mount('#app')</script></html>`}))
 await page.goto(origins.user+'/__user-draw-time')
 const initialView=page.getByRole('combobox',{name:locale.startsWith('zh')?'查看方式':'View',exact:true})
 await expect(initialView).toHaveAttribute('data-value','recent-50')
 await expect(page.locator('.archive-row')).toHaveCount(50)
 await expectNoPageOverflow(page)
 await page.screenshot({path:info.outputPath(`default-recent-50-${locale}.png`)})
 await selectControl(initialView,'date')
 await expect(page.locator('.archive-time')).toHaveText('2026-11-14 11:28:37')
 await expect(page.locator('th').first()).toHaveText(locale==='zh'?'开奖时间/期数':'Draw time / period')
 const preset=page.viewportSize()!
 for(const width of info.project.name.includes('reflow')?[320,620,900,1440]:[preset.width]){
  if(info.project.name.includes('reflow'))await page.setViewportSize({width,height:preset.height})
  await expectNoPageOverflow(page)
  await page.screenshot({path:info.outputPath(`user-draw-time-${locale}-${width}.png`),fullPage:true})
 }
 published=true
 await page.clock.runFor(5000)
 await expect.poll(()=>!!releaseRefresh).toBe(true)
 const pendingReads=requests.length
 await page.clock.runFor(10000)
 expect(requests.length).toBe(pendingReads)
 await expect(page.locator('[aria-busy]').first()).toHaveAttribute('aria-busy','false')
 releaseRefresh!()
 await expect(page.getByText('3506124',{exact:true}).or(page.getByText('3506124期',{exact:true}))).toBeVisible()
 const zh=locale==='zh'
 await page.getByRole('button',{name:zh?'昨天':'Yesterday',exact:true}).click()
 await expect(page.locator('.archive-time')).toHaveText('2026-11-13 11:28:37')
 const historyReads=requests.length
 await page.clock.runFor(10000)
 expect(requests.length).toBe(historyReads)
 await page.getByRole('button',{name:zh?'当天':'Today',exact:true}).click()
 await expect(page.locator('.archive-time')).toHaveText('2026-11-14 11:28:37')
 await page.getByRole('button',{name:zh?'日期':'Date',exact:true}).click()
 await page.getByRole('button',{name:'2026-11-12',exact:true}).click()
 await expect(page.locator('.archive-time')).toHaveText('2026-11-12 11:28:37')
 const view=page.getByRole('combobox',{name:zh?'查看方式':'View',exact:true})
 await selectControl(view,'recent-50')
 const records=page.locator('.archive-row')
 await expect(records).toHaveCount(50)
 await expect(page.locator('.platform-date-input-display')).toHaveCount(0)
 newest=3506125
 await page.clock.runFor(5000)
 await expect(records.first()).toContainText('3506125')
 await selectControl(view,'recent-100')
 await expect(view).toHaveAttribute('data-value','recent-50')
 await expect(records).toHaveCount(50)
 await selectControl(view,'recent-100')
 await expect(records).toHaveCount(100)
 await expect(records.last()).toContainText('3506026')
 await expectNoPageOverflow(page)
 await page.screenshot({path:info.outputPath(`recent-100-${locale}.png`)})
 await selectControl(view,'date')
 await expect(page.locator('.platform-date-input-display')).toHaveValue('2026-11-14')
 expect(requests).toContain('recent-50');expect(requests).toContain('recent-100')
 expect(requests).toContain('2026-11-13');expect(requests).toContain('2026-11-14');expect(requests).toContain('2026-11-12')
 expect(errors).toEqual([])
})

for(const locale of ['zh-CN','en'] as const) test(`draw overview retries initial and historical reads ${locale}`,async({page},info)=>{
 await page.clock.install()
 let failures=1
 const requests:string[]=[]
 await page.route('**/api/**',route=>{
  const year=new URL(route.request().url()).searchParams.get('year')||'2026'
  requests.push(year)
  if(failures>0){failures--;return route.fulfill({status:503,json:{success:false,error:{code:'SERVICE_UNAVAILABLE',message:'Unavailable'}}})}
  return route.fulfill({json:{success:true,data:{recent:0,daily:false,mark_six:false,total_count:'0',years:['2026','2025','2024'],year,has_more:false,pending:[],pending_has_more:false,items:[]}}})
 })
 await page.route('**/__draw-retry',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app" class="management-console"></div><script type="module">import {createApp,h} from '/node_modules/.vite/deps/vue.js';import C from '/@fs${resolve('shared/src/PlatformDrawOverview.vue')}';import {setPlatformDisplaySettings} from '/@fs${resolve('shared/src/display-settings.ts')}';import '/@fs${resolve('shared/src/styles.css')}';import '/@fs${resolve('shared/src/management.css')}';import '/@fs${resolve('shared/src/controls.css')}';setPlatformDisplaySettings({timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss'});createApp({render:()=>h('main',{style:'padding:8px;min-width:0'},[h(C,{gameId:'${gameId}',locale:'${locale}',canManage:false})])}).mount('#app')</script></html>`}))
 await page.goto(origins.platform+'/__draw-retry')
 await expect(page.locator('.draw-overview [role=alert]')).toBeVisible()
 await page.clock.runFor(5000)
 const year=page.getByRole('combobox',{name:locale==='zh-CN'?'年份':'Year',exact:true})
 await expect(year).toHaveAttribute('data-value','2026')
 failures=2
 await selectControl(year,'2025')
 await expect.poll(()=>requests.length).toBe(3)
 await expect(page.locator('.draw-overview [role=alert]')).toHaveCount(0)
 await page.clock.runFor(5000)
 await expect.poll(()=>requests.length).toBe(4)
 await page.clock.runFor(5000)
 await expect(year).toHaveAttribute('data-value','2025')
 expect(requests).toEqual(['2026','2026','2025','2025','2025'])
 failures=1
 await selectControl(year,'2026')
 await expect.poll(()=>requests.length).toBe(6)
 await selectControl(year,'2024')
 await expect(year).toHaveAttribute('data-value','2024')
 await page.clock.runFor(5000)
 await expect.poll(()=>requests.slice(6)).toEqual(['2024','2024'])
 await expectNoPageOverflow(page)
 if(info.project.name.includes('reflow')) await page.setViewportSize({width:320,height:800})
 await expectNoPageOverflow(page)
 await page.screenshot({path:info.outputPath(`draw-retry-${locale}.png`),fullPage:true})
})
