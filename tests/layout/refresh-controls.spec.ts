import {test,expect,type Page} from '@playwright/test'
import {resolve} from 'node:path'
import {origins,gameId,expectNoPageOverflow,installLayoutApi,installWalletLayoutApi} from './fixture'
const displaySettings={timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss'}
async function mount(page:Page,component:string,locale:'en'|'zh-CN',componentProps:Record<string,unknown>){
 const source=await(await page.request.get(origins.platform+'/src/RewardsPanel.vue')).text()
 const vuePath=source.match(/from "([^"]*deps\/vue\.js[^"]*)"/)![1]
 await page.route('**/__refresh-controls',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module">import{createApp}from'${vuePath}';import Harness from'/@fs/${resolve('tests/layout/RefreshControlsHarness.vue')}';import'/@fs/${resolve('shared/src/styles.css')}';import'/src/style.css';import'/@fs/${resolve('shared/src/management.css')}';import'/@fs/${resolve('shared/src/controls.css')}';import'/@fs/${resolve('shared/src/refresh-button.css')}';createApp(Harness,${JSON.stringify({component,locale,componentProps})}).mount('#app')</script></html>`}))
 await page.goto(origins.platform+'/__refresh-controls')
}
for(const locale of ['en','zh-CN'] as const)test(`low-frequency lists expose refresh only for read recovery (${locale})`,async({page},info)=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message))
 let fail=false,reads=0
 await page.route('**/api/**',route=>{
  const request=route.request(),url=new URL(request.url()),path=url.pathname
  expect(request.method()).toBe('GET');reads++
  if(fail)return route.fulfill({status:503,json:{success:false,error:{code:'UNAVAILABLE',message:'Unavailable'}}})
  const list={brand_id:gameId,items:[],total_count:'0',limit:Number(url.searchParams.get('limit')),offset:Number(url.searchParams.get('offset'))}
  let data:unknown
  if(['/api/v1/platform/platform-accounts','/api/v1/platform/platform-roles','/api/v1/platform/notification-deliveries'].includes(path))data={items:[],total_count:'0'}
  else if(['/api/v1/platform/commission-cycles','/api/v1/platform/commission-payments','/api/v1/platform/reward-orders'].includes(path))data=list
  else if(path==='/api/v1/platform/data-retention-policy')data={version:1,enabled:false,execution_day:10,execution_time:'02:00',timezone:'Asia/Singapore',next_run:null,updated_at:'2026-11-12T00:00:00Z'}
  else if(path==='/api/v1/platform/data-retention-tasks')data={items:[],total_count:'0',limit:Number(url.searchParams.get('limit')),offset:Number(url.searchParams.get('offset'))}
  else if(path==='/api/v1/platform/notification-templates')data={items:[]}
  else if(path==='/api/v1/agent/wallet')data={commission_points:'0'}
  else if(path==='/api/v1/agent/commission')data={items:[],total_count:'0',credited_points:'0',debited_points:'0'}
  else if(path==='/api/v1/agent/children')data={items:[],total_count:'0'}
  else throw Error(`Unexpected refresh request ${path}`)
  return route.fulfill({json:{success:true,data}})
 })
 const base={brandId:gameId,locale,displaySettings}
 const cases:[string,Record<string,unknown>][]=[
  ['AccessPanel',{...base,scope:'platform',accountId:gameId,grants:[],initialTab:'accounts'}],
  ['AccessPanel',{...base,scope:'platform',accountId:gameId,grants:[],initialTab:'roles'}],
  ['DataRetentionPanel',{...base,accountId:gameId,canManage:true}],['CommissionPanel',{...base,initialTab:'cycles'}],['CommissionPanel',{...base,initialTab:'payments'}],
  ['NotificationsPanel',{...base,view:'deliveries',accountId:gameId,canManage:false}],
  ['NotificationTemplatesPanel',{...base,accountId:gameId,canManage:false}],['RewardsPanel',{...base,brandName:'Refresh review'}],
  ['AgentCommission',{}],['AgentChildren',{}],
 ]
 const preset=page.viewportSize()!
 for(const [component,props] of cases){
  const before=reads;fail=false;await mount(page,component,locale,props)
  await expect.poll(()=>reads).toBeGreaterThan(before)
  await expect(page.locator('.global-loading-overlay')).toHaveCount(0)
  const refresh=page.locator('button[data-refresh-button]')
  await expect(refresh).toHaveCount(0)
  for(const width of info.project.name.endsWith('reflow')?[320,900,901,1280]:[preset.width]){
   if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:preset.height})
   await expectNoPageOverflow(page)
  }
  await page.screenshot({path:info.outputPath(`${component}-${props.initialTab??''}-${locale}.png`),fullPage:true})
  fail=true;await mount(page,component,locale,props)
  await expect(refresh).toHaveCount(1);await expect(refresh).toBeEnabled()
  await expect(refresh.locator('svg')).toBeVisible()
  expect(await refresh.getAttribute('aria-label')).toBeTruthy()
  await expectNoPageOverflow(page)
  fail=false;await refresh.focus();await refresh.press('Enter')
  await expect(refresh).toHaveCount(0)
  await expectNoPageOverflow(page)
 }
 expect(errors).toEqual([])
})

for(const locale of ['en','zh-CN'] as const)test(`player history pages omit ordinary refresh (${locale})`,async({page},info)=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message))
 await installLayoutApi(page,locale);await installWalletLayoutApi(page)
 const preset=page.viewportSize()!
 for(const path of ['/wallet','/wallet/buckets','/wallet/ledger','/orders','/notifications','/results']){
  await page.goto(origins.user+path)
  await expect(page.locator('.workspace-menu-trigger')).toBeVisible()
  await expect(page.locator('[data-refresh-button]')).toHaveCount(0)
  for(const width of info.project.name.endsWith('reflow')?[320,900,1280]:[preset.width]){
   if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:preset.height})
   await page.locator('.workspace-menu-trigger').click()
   await expect(page.locator('#user-page-refresh button')).toHaveCount(0)
   await expectNoPageOverflow(page)
   await page.locator('.workspace-menu-close').click()
  }
  await page.screenshot({path:info.outputPath(`player-${path.replaceAll('/','-')}-${locale}.png`),fullPage:true})
 }
 expect(errors).toEqual([])
})

for(const locale of ['en','zh-CN'] as const)test(`fast-draw refresh is limited to today and read failures (${locale})`,async({page})=>{
 await page.clock.install({time:new Date('2026-11-06T04:00:00Z')})
 await installLayoutApi(page,locale);await installWalletLayoutApi(page)
 let fail=false
 await page.route('**/draw-archive?**',route=>{
  if(fail)return route.fulfill({status:503,json:{success:false,error:{code:'UNAVAILABLE',message:'Unavailable'}}})
  const url=new URL(route.request().url())
  return route.fulfill({json:{success:true,data:{game_id:url.searchParams.get('game_id'),game_name:'Draw',mark_six:false,timezone:'Asia/Singapore',daily:true,choices:['2026-11-06','2026-11-05'],selected:url.searchParams.get('selected')||'2026-11-06',items:[],total_count:'0',has_more:false}}})
 })
 await page.goto(origins.user+'/results')
 const refresh=page.locator('#user-page-refresh button')
 await expect(refresh).toHaveCount(1)
 const picker=page.locator('.archive-filter .select-control-trigger')
 await picker.click();await page.getByRole('option',{name:'2026-11-05',exact:true}).click()
 await expect(picker).toHaveAttribute('data-value','2026-11-05')
 await expect(refresh).toHaveCount(0)
 fail=true;await picker.click();await page.getByRole('option',{name:'2026-11-06',exact:true}).click()
 await expect(refresh).toHaveCount(1)
 fail=false;await page.locator('.workspace-menu-trigger').click();await refresh.click()
 await expect(refresh).toHaveCount(0)
 await page.locator('.workspace-menu-close').click()
 await expectNoPageOverflow(page)
})

for(const locale of ['en','zh-CN'] as const)test(`task records reread on opening and retain failure recovery (${locale})`,async({page},info)=>{
 let reads=0,fail=false
 const errors:string[]=[];page.on('pageerror',error=>errors.push(error.message))
 await page.route('**/api/**',route=>{
  expect(route.request().method()).toBe('GET')
  const url=new URL(route.request().url()),discovery=url.pathname.endsWith('/commission-discovery')
  if(discovery){reads++;if(fail)return route.fulfill({status:503,json:{success:false,error:{code:'UNAVAILABLE',message:'Unavailable'}}})}
  const item={id:gameId,brand_id:gameId,state:'registered',version:2,cycle_id:gameId,window_from:'2026-11-01T00:00:00Z',window_to:'2026-11-02T00:00:00Z',next_check_at:'2026-11-06T00:00:00Z',last_error_code:null,last_audit_log_id:gameId,created_at:'2026-11-01T00:00:00Z',updated_at:'2026-11-01T00:00:00Z'}
  return route.fulfill({json:{success:true,data:{brand_id:gameId,items:discovery?[item]:[],total_count:discovery?'1':'0',limit:Number(url.searchParams.get('limit')),offset:Number(url.searchParams.get('offset'))}}})
 })
 const account={id:gameId,super_admin:false,brand_ids:[gameId],permissions:[],permissions_by_brand:{[gameId]:['commission.view.brand']}}
 await mount(page,'CommissionCycles',locale,{brandId:gameId,brandStatus:'active',account})
 const open=page.getByRole('button',{name:locale==='en'?'Task records':'任务记录',exact:true})
 await open.click()
 const dialog=page.getByRole('dialog',{name:locale==='en'?'Task records':'任务记录',exact:true})
 await expect(dialog.locator('[data-discovery-id]')).toHaveCount(1)
 await expect(dialog.locator('[data-refresh-button]')).toHaveCount(0)
 const preset=page.viewportSize()!
 for(const width of info.project.name.endsWith('reflow')?[320,900,1280]:[preset.width]){
  if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:preset.height})
  await expectNoPageOverflow(page)
  await page.screenshot({path:info.outputPath(`task-records-${locale}-${width}.png`),fullPage:true})
 }
 await dialog.locator('.detail-modal-close').click();fail=true;await open.click()
 const retry=dialog.locator('[data-refresh-button]');await expect(retry).toBeEnabled()
 fail=false;await retry.click();await expect(retry).toHaveCount(0)
 expect(reads).toBe(3)
 await dialog.locator('.discovery-select').click()
 await expect(page.locator('dialog:visible [data-refresh-button]')).toHaveCount(0)
 expect(errors).toEqual([])
})
