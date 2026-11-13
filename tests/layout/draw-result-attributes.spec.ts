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
