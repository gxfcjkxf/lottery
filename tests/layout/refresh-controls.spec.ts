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
for(const locale of ['en','zh-CN'] as const)test(`console refresh positions and interaction (${locale})`,async({page},info)=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message))
 const reads:string[]=[];let hold=false,release:(()=>void)|undefined
 await page.route('**/api/**',async route=>{
  const request=route.request(),url=new URL(request.url()),path=url.pathname
  expect(request.method()).toBe('GET');reads.push(path)
  if(hold)await new Promise<void>(resolve=>release=resolve)
  const list={brand_id:gameId,items:[],total_count:'0',limit:Number(url.searchParams.get('limit')),offset:Number(url.searchParams.get('offset'))}
  let data:unknown
  if(['/api/v1/platform/platform-accounts','/api/v1/platform/platform-roles','/api/v1/platform/notification-deliveries'].includes(path))data={items:[],total_count:'0'}
  else if(['/api/v1/platform/report-archives','/api/v1/platform/commission-cycles','/api/v1/platform/commission-payments','/api/v1/platform/reward-orders'].includes(path))data=list
  else if(path==='/api/v1/platform/notification-templates')data={items:[]}
  else throw Error(`Unexpected refresh request ${path}`)
  await route.fulfill({json:{success:true,data}})
 })
 const base={brandId:gameId,locale,displaySettings}
 const cases:[string,Record<string,unknown>][]=[
  ['AccessPanel',{...base,scope:'platform',accountId:gameId,grants:[],initialTab:'accounts'}],
  ['AccessPanel',{...base,scope:'platform',accountId:gameId,grants:[],initialTab:'roles'}],
  ['ArchivesPanel',base],['CommissionPanel',{...base,initialTab:'cycles'}],['CommissionPanel',{...base,initialTab:'payments'}],
  ['NotificationsPanel',{...base,view:'deliveries',accountId:gameId,canManage:false}],
  ['NotificationTemplatesPanel',{...base,accountId:gameId,canManage:false}],['RewardsPanel',{...base,brandName:'Refresh review'}],
 ]
 const preset=page.viewportSize()!
 for(const [component,props] of cases){
  const before=reads.length;await mount(page,component,locale,props)
  await expect.poll(()=>reads.length).toBe(before+1)
  const refresh=page.locator('button[data-refresh-button]')
  await expect(refresh).toHaveCount(1);await expect(refresh).toBeEnabled();await expect(refresh.locator('svg')).toBeVisible()
  await expect(refresh).toHaveAttribute('aria-label',locale==='en'?'Refresh':'刷新')
  await expect(refresh).toHaveAttribute('title',locale==='en'?'Refresh':'刷新')
  await expect(page.locator('.table-pagination [data-refresh-button]')).toHaveCount(0)
  const widths=info.project.name.endsWith('reflow')?[320,900,901,1280]:[preset.width]
  for(const width of widths){
   if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:720})
   const box=await refresh.boundingBox(),heading=await page.locator('.console-page-heading').boundingBox()
   expect(box).not.toBeNull();expect(heading).not.toBeNull()
   expect(box!.width).toBe(box!.height)
   expect(Math.abs(box!.x+box!.width-heading!.x-heading!.width)).toBeLessThanOrEqual(1)
   expect(box!.height).toBe(width<=900||!info.project.name.endsWith('reflow')?44:32)
   await expect(refresh.locator('svg')).toHaveCSS('width','16px')
   await expectNoPageOverflow(page)
  }
  await refresh.focus();await expect(refresh).toBeFocused()
  hold=true;const oldBox=await refresh.boundingBox();await refresh.press('Enter')
  await expect(refresh).toBeDisabled();expect(await refresh.boundingBox()).toEqual(oldBox)
  await expect.poll(()=>Boolean(release)).toBe(true);hold=false;release!();release=undefined
  await expect(refresh).toBeEnabled();expect(reads.length).toBe(before+2)
  await expect(page.locator('html')).not.toHaveAttribute('data-refresh-failure',/.+/)
  await page.screenshot({path:info.outputPath(`${component}-${props.initialTab??''}-${locale}.png`),fullPage:true})
 }
 expect(errors).toEqual([])
})

for(const locale of ['en','zh-CN'] as const)test(`agent commission refresh and read recovery (${locale})`,async({page},info)=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));let reads=0,fail=false
 await page.route('**/api/**',route=>{
  expect(route.request().method()).toBe('GET');reads++
  if(fail)return route.fulfill({status:503,json:{success:false,error:{code:'UNAVAILABLE',message:'Unavailable'}}})
  const path=new URL(route.request().url()).pathname
  const data=path==='/api/v1/agent/wallet'?{commission_points:'0'}:path==='/api/v1/agent/commission'?{items:[],total_count:'0',credited_points:'0',debited_points:'0'}:path==='/api/v1/agent/children'?{items:[],total_count:'0'}:null
  if(!data)throw Error(`Unexpected agent refresh request ${path}`)
  return route.fulfill({json:{success:true,data}})
 })
 await mount(page,'AgentCommission',locale,{})
 const refresh=page.locator('[data-refresh-button]');await expect(refresh).toBeEnabled()
 const preset=page.viewportSize()!
 for(const width of info.project.name.endsWith('reflow')?[320,900,901,1280]:[preset.width]){
  if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:720})
  const box=await refresh.boundingBox(),heading=await page.locator('.console-page-heading').boundingBox()
  expect(Math.abs(box!.x+box!.width-heading!.x-heading!.width)).toBeLessThanOrEqual(1)
  await expectNoPageOverflow(page)
 }
 const before=reads;await refresh.focus();await refresh.press('Enter');await expect.poll(()=>reads).toBe(before+2)
 await page.screenshot({path:info.outputPath(`agent-refresh-${locale}.png`),fullPage:true})
 fail=true;await mount(page,'AgentChildren',locale,{})
 await expect(refresh).toBeVisible();await expect(refresh.locator('svg')).toBeVisible()
 await expect(refresh).toHaveAttribute('aria-label',locale==='en'?'Retry loading':'重新加载')
 fail=false;await refresh.click();await expect(refresh).toHaveCount(0);await expectNoPageOverflow(page)
 expect(errors).toEqual([])
})

for(const locale of ['en','zh-CN'] as const)test(`player page refresh remains in the menu (${locale})`,async({page},info)=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message))
 await installLayoutApi(page,locale);await installWalletLayoutApi(page)
 const preset=page.viewportSize()!
 for(const path of ['/wallet','/wallet/buckets','/wallet/ledger','/orders','/notifications','/results']){
  await page.goto(origins.user+path)
  const refresh=page.locator('#user-page-refresh [data-refresh-button]')
  await expect(refresh).toHaveCount(1)
  await expect(page.locator('[data-refresh-button]')).toHaveCount(1)
  const widths=info.project.name.endsWith('reflow')?[320,900,1280]:[preset.width]
  for(const width of widths){
   if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:720})
   await page.locator('.workspace-trailing button').click()
   await expect(refresh).toBeVisible();await expect(refresh).toBeEnabled()
   await expect(refresh).toHaveCSS('width','44px');await expect(refresh.locator('svg')).toHaveCSS('width','20px')
   const label=await refresh.getAttribute('aria-label');expect(label).toBeTruthy();await expect(refresh).toHaveAttribute('title',label!)
   await refresh.click();await expect(refresh).toBeEnabled();await expectNoPageOverflow(page)
   await page.locator('.workspace-menu-close').click();await expect(refresh).toHaveCount(1)
  }
  if(path==='/wallet')await page.screenshot({path:info.outputPath(`player-refresh-${locale}.png`),fullPage:true})
 }
 expect(errors).toEqual([])
})
