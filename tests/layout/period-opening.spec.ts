import { selectControl } from '../select-control'
import {test,expect} from '@playwright/test'
import {resolve} from 'node:path'
import {origins,gameId,expectNoPageOverflow} from './fixture'
for(const locale of ['en','zh-CN'] as const)test(`automatic period opening saves both date and time ${locale}`,async({page},info)=>{
 await page.clock.install({time:new Date('2026-10-26T00:00:00Z')})
 const errors:string[]=[];page.on('pageerror',error=>errors.push(error.message))
 const zh=locale==='zh-CN',settings={timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss'}
 const game={id:gameId,name:'Mark Six',version:1,model:{model:'DIGITS_0_9',length:3,ordered:true},status:'active',rule_published:true}
 const period={id:gameId,game_id:gameId,period_no:'2026115',bet_start_at:'2026-10-26T02:00:00Z',bet_end_at:'2026-10-28T13:29:45Z',draw_at:'2026-10-28T13:30:00Z',result:null,published_at:null,state:'scheduled'}
 const writes:{body:Record<string,string>;key:string}[]=[]
 await page.route('**/api/**',route=>{
  const req=route.request(),path=new URL(req.url()).pathname,ok=(data:unknown)=>route.fulfill({json:{success:true,data}})
  if(req.method()==='POST'){
   expect(path).toBe(`/api/v1/platform/platform-games/periods/${gameId}/opening`)
   writes.push({body:req.postDataJSON(),key:req.headers()['idempotency-key']!})
   if(writes.length===1)return route.fulfill({status:503,json:{success:false,error:{code:'SERVICE_UNAVAILABLE',message:'Private upstream detail'}}})
   expect(writes[1]).toEqual(writes[0]);period.bet_start_at=writes[1]!.body.bet_start_at!;return ok(period)
  }
  if(path.endsWith('/draw-overview'))return ok({total_count:'0',years:['2026'],year:'2026',has_more:false,pending_has_more:false,items:[],pending:[period]})
  if(path.endsWith('/automation'))return ok({plan:{enabled:true,source_type:'mapped',source_url:'https://draw.example',open_before_seconds:120,close_before_seconds:15},source_check:null,attempts:[]})
  if(path.endsWith('/'+gameId))return ok({total_count:'1',game,rule_protocol:'configured',plays:[],periods:[period]})
  throw new Error('Unexpected test request '+path)
 })
 await page.route('**/__opening',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app" class="management-console"></div><script type="module">import {createApp,h} from '/node_modules/.vite/deps/vue.js';import C from '/@fs${resolve('shared/src/PlatformLottery.vue')}';import Host from '/@fs${resolve('shared/src/OperationToastHost.vue')}';import {setPlatformDisplaySettings} from '/@fs${resolve('shared/src/display-settings.ts')}';import '/@fs${resolve('shared/src/styles.css')}';import '/@fs${resolve('shared/src/management.css')}';import '/@fs${resolve('shared/src/controls.css')}';import '/@fs${resolve('shared/src/form-validation.css')}';setPlatformDisplaySettings(${JSON.stringify(settings)});createApp({render:()=>[h(C,${JSON.stringify({surface:'platform',brandId:'',initialGameId:gameId,drawsOnly:true,locale,canManage:true,canConnect:false,displaySettings:settings})}),h(Host,{locale:'${locale}'})]}).mount('#app')</script></html>`}))
 await page.goto(origins.platform+'/__opening')
 await page.getByRole('button',{name:zh?'设置开盘日期时间':'Set opening date and time',exact:true}).click()
 const dialog=page.getByRole('dialog');await expect(dialog).toBeVisible()
 await expect(dialog.locator('.platform-date-input-display')).toHaveValue('2026-10-26')
 await dialog.locator('.platform-date-input-button').click()
 await dialog.getByRole('button',{name:'2026-10-27',exact:true}).click()
 await selectControl(dialog.locator(".platform-time-input .select-control-trigger").nth(0), '09')
 await selectControl(dialog.locator(".platform-time-input .select-control-trigger").nth(1), '15')
 if(info.project.name.endsWith('reflow'))await page.setViewportSize({width:320,height:640})
 await expectNoPageOverflow(page);await page.screenshot({path:info.outputPath(`opening-form-${locale}.png`)})
 await dialog.locator('button[type=submit]').click()
 await expect(page.locator('.toast-error')).toBeVisible();await expect(dialog).toBeVisible()
 await expect(dialog.locator('.platform-date-input-button')).toBeDisabled()
 expect(writes[0]!.body).toEqual({bet_start_at:'2026-10-27T01:15:00.000Z',expected_bet_start_at:'2026-10-26T02:00:00Z'})
 expect(writes[0]!.key).toBeTruthy();await expect(page.locator('.toast-error')).not.toContainText('Private upstream detail')
 await page.locator('.toast-error button').click();await dialog.locator('button[type=submit]').click()
 await expect(dialog).toHaveCount(0);await expect(page.locator('.toast-success')).toBeVisible()
 await expect(page.locator('.draw-pending-grid')).toContainText('2026-10-27 09:15:00')
 await expectNoPageOverflow(page);expect(errors).toEqual([])
})
