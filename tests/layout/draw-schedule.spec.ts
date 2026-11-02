import {test,expect} from '@playwright/test'
import {resolve} from 'node:path'
import {origins,gameId,expectNoPageOverflow,installLayoutApi} from './fixture'
for(const locale of ['en','zh-CN'] as const)test(`game-level draw times and collection delay ${locale}`,async({page},info)=>{
 const zh=locale==='zh-CN',errors:string[]=[];page.on('pageerror',error=>errors.push(error.message))
 const schedule={version:1,enabled:true,mode:'daily',draw_time:'21:30:00',open_before_hours:24,close_before_seconds:15,collect_after_seconds:30}
 const writes:Array<{key:string;body:typeof schedule}>=[]
 await page.route('**/api/**',route=>{
  const req=route.request(),path=new URL(req.url()).pathname
  if(req.method()==='GET'&&path.endsWith('/automation'))return route.fulfill({json:{success:true,data:{schedule,plan:null,source_check:null,attempts:[],allowed_origins:[]}}})
  expect(path).toBe(`/api/v1/platform/platform-games/${gameId}/draw-schedule`)
  expect(req.method()).toBe('POST');writes.push({key:req.headers()['idempotency-key']!,body:req.postDataJSON()})
  if(writes.length===1)return route.fulfill({status:503,json:{success:false,error:{code:'SERVICE_UNAVAILABLE',message:'private detail'}}})
  expect(writes[1]).toEqual(writes[0]);return route.fulfill({json:{success:true,data:{...writes[1]!.body,version:2}}})
 })
 const settings={timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss'}
 await page.route('**/__schedule',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app" class="management-console"></div><script type="module">import {createApp,h} from '/node_modules/.vite/deps/vue.js';import C from '/@fs${resolve('shared/src/DrawScheduleSettings.vue')}';import Host from '/@fs${resolve('shared/src/OperationToastHost.vue')}';import {setPlatformDisplaySettings} from '/@fs${resolve('shared/src/display-settings.ts')}';import '/@fs${resolve('shared/src/styles.css')}';import '/@fs${resolve('shared/src/management.css')}';import '/@fs${resolve('shared/src/controls.css')}';import '/@fs${resolve('shared/src/form-validation.css')}';setPlatformDisplaySettings(${JSON.stringify(settings)});createApp({render:()=>[h(C,${JSON.stringify({gameId,timezone:'Asia/Macau',locale,canWrite:true})}),h(Host,{locale:'${locale}'})]}).mount('#app')</script></html>`}))
 await page.goto(origins.platform+'/__schedule')
 await expect(page.getByRole('region',{name:zh?'时间设置':'Time settings'})).toBeVisible()
 await page.getByLabel(zh?'开奖前开盘（小时）':'Open before draw (hours)',{exact:true}).fill('48')
 await page.getByLabel(zh?'开奖前封盘（秒）':'Close before draw (seconds)',{exact:true}).fill('60')
 await page.getByLabel(zh?'开奖后开始采集（秒）':'Start collection after draw (seconds)',{exact:true}).fill('90')
 const sizes=info.project.name.endsWith('reflow')?[{width:1440,height:900},{width:768,height:1024},{width:320,height:640}]:[]
 for(const size of sizes){await page.setViewportSize(size);await expectNoPageOverflow(page)}
 await page.screenshot({path:info.outputPath(`schedule-${locale}.png`)})
 await page.getByRole('button',{name:zh?'保存设置':'Save settings',exact:true}).click()
 await expect(page.locator('.toast-error')).toBeVisible()
 await expect(page.getByLabel(zh?'开奖后开始采集（秒）':'Start collection after draw (seconds)',{exact:true})).toBeDisabled()
 expect(writes[0]!.body).toEqual({...schedule,open_before_hours:48,close_before_seconds:60,collect_after_seconds:90})
 await page.locator('.toast-error button').click()
 await page.getByRole('button',{name:zh?'重试原请求':'Retry original request',exact:true}).click()
 await expect(page.locator('.toast-success')).toBeVisible()
 await expect(page.getByLabel(zh?'开奖后开始采集（秒）':'Start collection after draw (seconds)',{exact:true})).toBeEnabled()
 await expectNoPageOverflow(page);expect(errors).toEqual([])
})
test('Mark Six schedule belongs to the game, source has result fields only',async({page},info)=>{
 await installLayoutApi(page,'zh-CN')
 const schedule={version:1,enabled:true,mode:'daily',draw_time:'21:32:32',open_before_hours:24,close_before_seconds:15,collect_after_seconds:60}
 const model={model:'X_PLUS_Y',regular_count:6,special_count:1,regular_pool:{min:1,max:49,allow_repeat:false},special_pool:{min:1,max:49,allow_repeat:false},allow_repeat:false,ordered:true,length:0,pool_size:0,total_count:0}
 const game={id:gameId,name:'新澳门六合彩',version:1,model,status:'active',timezone:'Asia/Macau',rule_published:true}
 const period={id:gameId,game_id:gameId,period_no:'2026302',bet_start_at:'2026-10-28T13:32:32Z',bet_end_at:'2026-10-29T13:32:17Z',draw_at:'2026-10-29T13:32:32Z',result:null,published_at:null,state:'scheduled'}
 const mapping={format:'json',schedule:'game',time_format:'custom',time_layout:'2006-01-02 15:04:05',timezone:'Asia/Macau',period:{path:'/0/expect',attribute:''},draw_time:{path:'/0/openTime',attribute:''},regular:{path:'/0/openCode',attribute:'',separator:',',offset:0,count:6},special:{path:'/0/openCode',attribute:'',separator:',',offset:6,count:1},digits:null,next_period:null,next_draw_time:null,complete:null,period_query:'',game_query:''}
 const plan={version:2,enabled:true,source_type:'mapped',source_url:'https://macaumarksix.com/api/macaujc2.com',mapping,anchor_draw_at:null,anchor_period_no:null,interval_seconds:null,open_before_seconds:null,close_before_seconds:0,credential_ref:''}
 await page.route('**/api/**',route=>{
  const request=route.request(),path=new URL(request.url()).pathname;expect(request.method()).toBe('GET')
  const ok=(data:unknown)=>route.fulfill({json:{success:true,data}})
  if(path==='/api/v1/platform/me')return ok({account:{id:gameId,username:'schedule_reviewer',super_admin:true,platform_permissions:['platform_game.view.platform','platform_game.write.platform']}})
  if(path==='/api/v1/platform/platform-settings')return ok({display_name:'Layout trial',logo_text:'L',logo_url:'',timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss',version:1,updated_at:'2026-10-28T00:00:00Z'})
  if(path==='/api/v1/platform/brands')return ok({items:[]})
  if(path==='/api/v1/platform/presentation'||path==='/api/v1/display-settings')return route.fallback()
  if(path==='/api/v1/platform/platform-games')return ok({games:[game],limit:50,offset:0,total_count:'1'})
  if(path.endsWith('/'+gameId))return ok({game,periods:[period],total_count:'1'})
  if(path.endsWith('/automation'))return ok({schedule,plan,source_check:null,attempts:[],allowed_origins:['https://macaumarksix.com']})
  if(path.endsWith('/draw-overview'))return ok({total_count:'0',years:['2026'],year:'2026',has_more:false,pending_has_more:false,items:[],pending:[period]})
  if(path.endsWith('/draw-sources'))return ok({version:2,active_source_id:gameId,active_source_revision:2,items:[{id:gameId,name:'新澳门公开来源',revision:2,plan}]})
  throw new Error('Unexpected request '+path)
 })
 const settings={timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss'}
 await page.route('**/__schedule-page*',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app" class="management-console" style="padding:12px"></div><script type="module">import {createApp,h} from '/node_modules/.vite/deps/vue.js';import C from '/@fs${resolve('shared/src/PlatformLottery.vue')}';import {setPlatformDisplaySettings} from '/@fs${resolve('shared/src/display-settings.ts')}';import '/@fs${resolve('shared/src/styles.css')}';import '/@fs${resolve('shared/src/management.css')}';import '/@fs${resolve('shared/src/controls.css')}';import '/@fs${resolve('shared/src/form-validation.css')}';setPlatformDisplaySettings(${JSON.stringify(settings)});createApp({render:()=>h(C,${JSON.stringify({surface:'platform',brandId:'',initialGameId:gameId,drawsOnly:!new URL(route.request().url()).searchParams.has('catalog'),locale:'zh-CN',canManage:true,canConnect:false,displaySettings:settings})})}).mount('#app')</script></html>`}))
 await page.goto(origins.platform+'/__schedule-page')
 await expect(page.getByRole('button',{name:'设置开盘日期时间',exact:true})).toHaveCount(0)
 await expect(page.getByRole('button',{name:'时间设置',exact:true})).toHaveCount(0)
 await expect(page.getByRole('button',{name:'手动开奖结果',exact:true})).toHaveCount(0)
 await expect(page.getByRole('button',{name:'创建期次',exact:true})).toHaveCount(0)
 await expect(page.getByTestId('draw-overview')).toBeVisible()
 await expect(page.getByTestId('draw-overview')).not.toContainText('平台封盘时间')
 await page.goto(origins.platform)
 await expect(page.locator('.management-console')).toBeVisible()
 const navigation=page.locator('button[aria-controls="platform-navigation"]')
 if(await navigation.isVisible())await navigation.click()
 await page.locator('.sidebar').getByRole('button',{name:'彩种',exact:true}).click()
 await expect(page.getByTestId('platform-brand-required')).toHaveCount(0)
 const row=page.getByRole('row').filter({hasText:game.name})
 await expect(row).toBeVisible()
 if(info.project.name.endsWith('reflow'))for(const size of [{width:1440,height:900},{width:1024,height:768},{width:768,height:1024},{width:701,height:600},{width:700,height:600},{width:320,height:640}]){await page.setViewportSize(size);await expectNoPageOverflow(page)}
 await expectNoPageOverflow(page)
 await page.screenshot({path:info.outputPath('game-settings-catalog.png'),fullPage:true})
 await row.getByRole('button',{name:'期次',exact:true}).click()
 await expect(page.getByTestId('platform-brand-required')).toHaveCount(0)
 await expect(page.getByRole('button',{name:'创建期次',exact:true})).toHaveCount(0)
 await expect(page.getByRole('columnheader',{name:'开奖结果',exact:true})).toHaveCount(0)
 await page.getByRole('button',{name:'返回彩种列表',exact:true}).click()
 await row.getByRole('button',{name:'时间设置',exact:true}).click()
 await expect(page.getByTestId('platform-brand-required')).toHaveCount(0)
 await expect(page.locator('.topbar-brand-picker')).toHaveCount(0)
 await expect(page.getByLabel('开奖后开始采集（秒）',{exact:true})).toHaveValue('60')
 if(info.project.name.endsWith('reflow'))await page.setViewportSize({width:320,height:640})
 await expectNoPageOverflow(page)
 await page.screenshot({path:info.outputPath('game-schedule-integrated.png'),fullPage:true})
 await page.getByRole('button',{name:'返回彩种列表',exact:true}).click()
 await row.getByRole('button',{name:'开奖数据源',exact:true}).click()
 await expect(page.getByTestId('platform-brand-required')).toHaveCount(0)
 await expect(page.getByLabel('首期开奖时间',{exact:true})).toHaveCount(0)
 await expect(page.getByLabel('开奖前开盘（秒）',{exact:true})).toHaveCount(0)
 await expect(page.getByLabel('期次排期方式',{exact:true})).toHaveCount(0)
 await expect(page.getByRole('heading',{name:'字段映射'})).toBeVisible()
 await expectNoPageOverflow(page)
})
