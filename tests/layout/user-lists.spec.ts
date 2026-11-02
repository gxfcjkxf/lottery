import { expectRealEntry, expectListLayout } from './real-page-review'
import {test,expect,type Page,type TestInfo} from '@playwright/test'
import {installLayoutApi,installWalletLayoutApi,origins,gameId,expectNoPageOverflow} from './fixture'
import {selectControl} from '../select-control'
const at='2026-11-02T00:00:00Z'
const id=(n:number)=>`bbbbbbbb-bbbb-4bbb-8bbb-${String(n).padStart(12,'0')}`
const withdrawal={id:gameId,brand_id:gameId,member_id:gameId,account_id:gameId,points:'50',state:'reviewing',version:1,source_allocation:[{source:'recharge',state:'available',points:'50'}],reserve_entry_id:gameId,release_entry_id:null,paid_entry_id:null,cycle_from_at:null,cycle_from_version:'0',reserve_version:'1',created_at:at,updated_at:at,reviewed_at:null,completed_at:null,decision_reason:'',audit_log_id:gameId}
async function setup(page:Page,locale:string){
 page.setDefaultTimeout(8000)
 await installLayoutApi(page,locale);await installWalletLayoutApi(page)
 await page.route('**/api/v1/withdrawal-availability',r=>r.fulfill({json:{success:true,data:{points_step:'10',available_points:'100',source_balances:[{source:'recharge',state:'available',points:'100'}],actor_context:'a'.repeat(64),brand_id:gameId,member_id:gameId,policy_enabled:false,eligibility_configured:true,can_apply:false,reason_code:'WITHDRAWAL_DISABLED',min_points:'10',max_points:'100',allowed_sources:['recharge'],real_payments:false}}}))
}
async function review(page:Page,info:TestInfo,label:string){
 await expectRealEntry(page,'user')
 const preset=page.viewportSize()!
 for(const width of info.project.name.endsWith('reflow')?[320,620,900,1440]:[preset.width]){
  if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:preset.height})
  await expectListLayout(page)
  const surface=page.locator('.user-record-list').first();await expect(surface).toBeVisible();await expect(surface).toHaveCSS('padding','0px')
  await expect(surface.locator('.user-record-list')).toHaveCount(0)
  await page.screenshot({path:info.outputPath(`${label}-${width}.png`),fullPage:true})
 }
 if(info.project.name.endsWith('reflow'))await page.setViewportSize(preset)
}
test('refresh waits for session restoration before rendering user pages', async ({page}) => {
 await setup(page, 'zh-CN')
 await page.route('**/api/v1/me/referrals?*', r => r.fulfill({json:{success:true,data:{brand_id:gameId,member_id:gameId,items:[],limit:20,offset:0,total_count:'0'}}}))
 await page.route('**/api/v1/recharges?*', r => r.fulfill({json:{success:true,data:{brand_id:gameId,member_id:gameId,snapshot_at:at,state:null,items:[],limit:20,offset:0,total_count:'0'}}}))
 await page.route('**/api/v1/withdrawals?*', r => r.fulfill({json:{success:true,data:{brand_id:gameId,items:[],limit:20,offset:0,has_more:false,total_count:'0'}}}))
 for (const path of ['/orders', `/configured/${gameId}`, '/wallet/ledger', '/invites', '/notifications', '/recharge', '/withdraw']) {
  let release!: () => void
  let started!: () => void
  const gate = new Promise<void>(resolve => { release = resolve })
  const requested = new Promise<void>(resolve => { started = resolve })
  let reads = 0
  const handler = async (route: import('@playwright/test').Route) => { if (++reads === 2) { started(); await gate } await route.fallback() }
  await page.route('**/api/v1/me', handler)
  try {
   await page.goto(origins.user + path)
   await requested
   await expect(page.locator('#main-content')).toHaveCount(0)
   await expect(page.locator('.global-loading-overlay')).toBeVisible()
  } finally { release() }
  await expect(page.locator('#main-content')).toBeVisible()
  await page.unroute('**/api/v1/me', handler)
 }
})
for(const locale of ['en','zh-CN'] as const){
 test(`user record lists retain successful empty pagination (${locale})`,async({page},info)=>{
  const zh=locale==='zh-CN',errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await setup(page,locale)
  await page.route('**/api/v1/wallet/ledger?*',r=>r.fulfill({json:{success:true,data:{items:[],total_count:'0'}}}))
  await page.route('**/api/v1/recharges?*',r=>r.fulfill({json:{success:true,data:{brand_id:gameId,member_id:gameId,snapshot_at:at,state:null,items:[],limit:20,offset:0,total_count:'0'}}}))
  await page.route('**/api/v1/withdrawals?*',r=>r.fulfill({json:{success:true,data:{brand_id:gameId,items:[],limit:20,offset:0,has_more:false,total_count:'0'}}}))
  await page.route('**/api/v1/me/referrals?*',r=>r.fulfill({json:{success:true,data:{brand_id:gameId,member_id:gameId,items:[],limit:20,offset:0,total_count:'0'}}}))
  await page.route('**/api/v1/me/join-codes?*',r=>r.fulfill({json:{success:true,data:{brand_id:gameId,member_id:gameId,items:[],limit:20,offset:0,total_count:'0'}}}))
  for(const path of ['/wallet/ledger','/recharge','/withdraw','/orders','/invites',`/results?game_id=${gameId}`,'/notifications']){
   await page.goto(origins.user+path);const surface=page.locator('.user-record-list').first(),pager=surface.locator('.table-pagination')
   await expect(surface.locator('.empty-state').first(),path).toBeVisible()
   const mobileNotifications=path==='/notifications'&&page.viewportSize()!.width<=620
   if(mobileNotifications || path==='/orders')await expect(pager).toHaveCount(0)
   else {await expect(pager).toBeVisible();await expect(pager).toContainText(zh?'共 0 条':'Total: 0');await expect(pager.locator('.table-pagination-arrow').first()).toBeDisabled();await expect(pager.locator('.table-pagination-arrow').last()).toBeDisabled()}
   await review(page,info,path.split('?')[0].replaceAll('/','-'))
   if(path==='/invites'){await page.getByRole('tab',{name:zh?'推荐码':'Referral code',exact:true}).click();await expect(surface.locator('.empty-state')).toBeVisible();await expect(pager).toContainText(zh?'共 0 条':'Total: 0')}
   if(info.project.name.endsWith('reflow'))await page.setViewportSize(info.project.use.viewport!)
  }
  expect(errors).toEqual([])
 })
 test(`recharge query and paging preserve applied data after failed reads (${locale})`,async({page},info)=>{
  await setup(page,locale);let fail=false;const reads:URL[]=[]
  const all=Array.from({length:21},(_,i)=>({method:'manual',id:id(21-i),brand_id:gameId,member_id:gameId,points:String(i+100),state:'pending',version:'1',created_at:at,confirmed_at:null,ledger_entry_id:null}))
  await page.route('**/api/v1/recharges?*',r=>{const url=new URL(r.request().url());reads.push(url);if(fail){fail=false;return r.fulfill({status:503,json:{success:false,error:{code:'TEST_READ_FAILED',message:'Synthetic failure'}}})}const state=url.searchParams.get('state'),rows=state==='confirmed'?[]:all,offset=Number(url.searchParams.get('offset'));return r.fulfill({json:{success:true,data:{brand_id:gameId,member_id:gameId,snapshot_at:at,state,items:rows.slice(offset,offset+20),limit:20,offset,total_count:String(rows.length)}}})})
  await page.goto(origins.user+'/recharge');const surface=page.locator('.user-record-list'),pager=surface.locator('.table-pagination'),next=pager.locator('.table-pagination-arrow').last()
  await expect(surface.locator('.recharge-record')).toHaveCount(20);fail=true;await next.click();await expect(pager.locator('[aria-current=page]')).toHaveText('1');await expect(surface.locator('.recharge-record')).toHaveCount(20)
  await selectControl(surface.getByRole('combobox'),'confirmed');await next.click();expect(reads.at(-1)!.searchParams.get('state')).toBe(null);await expect(pager.locator('[aria-current=page]')).toHaveText('2')
  fail=true;await surface.locator('button[type=submit]').click();await expect(pager.locator('[aria-current=page]')).toHaveText('2');await expect(surface.locator('.recharge-record')).toHaveCount(1)
  await surface.locator('button[type=submit]').click();await expect(surface.locator('.empty-state')).toBeVisible();await expect(pager).toContainText(locale==='en'?'Total: 0':'共 0 条')
  await surface.getByRole('button',{name:locale==='en'?'Clear':'清空',exact:true}).click();await expect(surface.locator('.recharge-record')).toHaveCount(20)
  await review(page,info,'recharge-populated')
 })
 test(`withdrawal records paginate and retain the previous page on failure (${locale})`,async({page},info)=>{
  await setup(page,locale);let fail=false;const offsets:number[]=[]
  await page.route('**/api/v1/withdrawals?*',r=>{const offset=Number(new URL(r.request().url()).searchParams.get('offset'));offsets.push(offset);if(fail){fail=false;return r.fulfill({status:503,json:{success:false,error:{code:'TEST_READ_FAILED',message:'Synthetic failure'}}})}return r.fulfill({json:{success:true,data:{brand_id:gameId,items:Array.from({length:offset?1:20},(_,i)=>({...withdrawal,id:id(offset+i+1)})),limit:20,offset,has_more:offset===0,total_count:'21'}}})})
  await page.goto(origins.user+'/withdraw');const surface=page.locator('.withdrawal-records'),pager=surface.locator('.table-pagination'),next=pager.locator('.table-pagination-arrow').last()
  await expect(surface.locator('.withdrawal-order')).toHaveCount(20);fail=true;await next.click();await expect(pager.locator('[aria-current=page]')).toHaveText('1');await expect(surface.locator('.withdrawal-order')).toHaveCount(20)
  await next.click();await expect(surface.locator('.withdrawal-order')).toHaveCount(1);await expect(pager.locator('[aria-current=page]')).toHaveText('2');expect(offsets).toEqual([0,20,20])
  await review(page,info,'withdrawal-populated')
 })
}

for(const locale of ['en','zh-CN'] as const)test(`ledger uses real totals and preserves the page after a failed read (${locale})`,async({page},info)=>{
 await setup(page,locale);let total=50,fail=false
 const buckets=(available:string,manual:string)=>Object.fromEntries(['recharge','winning','gift','commission'].map(source=>[source,{available:source==='recharge'?available:'0',manual_frozen:source==='recharge'?manual:'0',system_frozen:source==='recharge'?'30':'0',withdrawal:source==='recharge'?'40':'0'}]))
 const entry={brand_id:gameId,member_id:gameId,account_id:gameId,entry_type:'freeze',reference_type:'manual',reference_id:gameId,before_snapshot:buckets('130','10'),after_snapshot:buckets('120','20'),delta_snapshot:Object.fromEntries(['recharge','winning','gift','commission'].map(source=>[source,{available:source==='recharge'?'-10':'0',manual_frozen:source==='recharge'?'10':'0',system_frozen:'0',withdrawal:'0'}])),reason:'',version:2,created_at:at}
 await page.route('**/api/v1/wallet/ledger?*',r=>{if(fail){fail=false;return r.fulfill({status:503,json:{success:false,error:{code:'TEST_READ_FAILED',message:'Synthetic failure'}}})}const offset=Number(new URL(r.request().url()).searchParams.get('offset'));return r.fulfill({json:{success:true,data:{items:Array.from({length:Math.min(50,total-offset)},(_,i)=>({...entry,id:id(offset+i+1)})),total_count:String(total)}}})})
 await page.goto(origins.user+'/wallet/ledger');await expect(page.locator('.workspace-menu .workspace-signout')).toHaveCount(1);const surface=page.locator('.ledger-panel'),pager=surface.locator('.table-pagination'),next=pager.locator('.table-pagination-arrow').last()
 await expect(surface.locator('.ledger-entry')).toHaveCount(50);await expect(next).toBeDisabled()
 total=51;await page.getByRole('button',{name:locale==='en'?'Menu':'菜单',exact:true}).click();await page.locator('#user-page-refresh button').click();await expect(next).toBeEnabled();await page.locator('.workspace-menu-close').click();fail=true;await next.click()
 await expect(surface.locator('.ledger-entry')).toHaveCount(50);await expect(pager.locator('[aria-current=page]')).toHaveText('1')
 await next.click();await expect(surface.locator('.ledger-entry')).toHaveCount(1);await expect(pager.locator('[aria-current=page]')).toHaveText('2');await expect(next).toBeDisabled()
 await review(page,info,'ledger-populated')
})
