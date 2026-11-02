import { expectRealEntry, expectListLayout } from './real-page-review'
import {test,expect} from '@playwright/test'
import {installLayoutApi,installWalletLayoutApi,origins,gameId,expectNoPageOverflow} from './fixture'
import {selectControl} from '../select-control'
const at='2026-11-02T00:00:00Z'
const id=(n:number)=>`bbbbbbbb-bbbb-4bbb-8bbb-${String(n).padStart(12,'0')}`
for(const locale of ['en','zh-CN'] as const){
 test(`user recharge and ledger retain records until pagination succeeds (${locale})`,async({page},info)=>{
  await installLayoutApi(page,locale);await installWalletLayoutApi(page)
  const buckets=Object.fromEntries(['recharge','winning','gift','commission'].map(source=>[source,{available:'0',manual_frozen:'0',system_frozen:'0',withdrawal:'0'}]))
  let fail=false,blocked=false,empty=false,release:()=>void=()=>{};let gate=Promise.resolve();const reads:URL[]=[]
  await page.route('**/api/v1/wallet/ledger?*',async route=>{
   const url=new URL(route.request().url());expect(route.request().method()).toBe('GET');reads.push(url)
   if(blocked)await gate
   if(fail){fail=false;return route.fulfill({status:503,json:{success:false,error:{code:'SERVICE_UNAVAILABLE',message:'Synthetic read failure'}}})}
   const offset=Number(url.searchParams.get('offset')),items=empty?[]:Array.from({length:offset===0?50:1},(_,i)=>({id:id(offset+i),brand_id:gameId,member_id:gameId,account_id:gameId,entry_type:'freeze',reference_type:'manual',reference_id:gameId,before_snapshot:buckets,delta_snapshot:buckets,after_snapshot:buckets,reason:'',version:offset+i+1,created_at:at}))
   return route.fulfill({json:{success:true,data:{items,total_count:empty?'0':'51'}}})
  })
  await page.route('**/api/v1/recharges?*',async route=>{
   const url=new URL(route.request().url());expect(route.request().method()).toBe('GET');reads.push(url)
   if(blocked)await gate
   if(fail){fail=false;return route.fulfill({status:503,json:{success:false,error:{code:'SERVICE_UNAVAILABLE',message:'Synthetic read failure'}}})}
   const offset=Number(url.searchParams.get('offset')),state=url.searchParams.get('state')
   const items=empty?[]:Array.from({length:offset===0?20:1},(_,i)=>({id:id(offset+i),brand_id:gameId,member_id:gameId,method:'manual',points:'100',state:state??'pending',version:'1',created_at:new Date(Date.parse(at)-(offset+i)*1000).toISOString(),confirmed_at:state==='confirmed'?at:null,ledger_entry_id:state==='confirmed'?gameId:null}))
   return route.fulfill({json:{success:true,data:{brand_id:gameId,member_id:gameId,snapshot_at:at,state,items,total_count:empty?'0':'21',limit:20,offset}}})
  })
  for(const [path,rowSelector,size] of [['/recharge','.recharge-record',20],['/wallet/ledger','.ledger-entry',50]] as const){
   empty=false;await page.goto(origins.user+path);await expect(page.locator('.workspace-menu .workspace-signout')).toHaveCount(1)
   const rows=page.locator(rowSelector),pager=page.locator('.table-pagination'),next=pager.locator('.table-pagination-arrow').last()
   await expect(rows).toHaveCount(size)
   blocked=true;gate=new Promise(r=>{release=r});fail=true
   try{await next.click();await expect(page.locator('.global-loading-overlay')).toBeVisible();await expect(rows).toHaveCount(size);await expect(pager.locator('[aria-current=page]')).toHaveText('1');await expect(next).toBeDisabled()}finally{blocked=false;release()}
   await expect(page.locator('.global-loading-overlay')).toBeHidden();await expect(rows).toHaveCount(size);await expect(pager.locator('[aria-current=page]')).toHaveText('1')
   await next.click();await expect(rows).toHaveCount(1);await expect(pager.locator('[aria-current=page]')).toHaveText('2');await expect(next).toBeDisabled()
   if(path==='/recharge'){
    const n=reads.length;await selectControl(page.getByRole('combobox'), 'confirmed');expect(reads.length).toBe(n)
    await page.locator('.ui-filter-bar button[type=submit]').click();await expect(rows).toHaveCount(20);await expect(pager.locator('[aria-current=page]')).toHaveText('1');expect(reads.at(-1)!.searchParams.get('state')).toBe('confirmed')
    await page.locator('.ui-filter-bar button[type=button]').filter({hasText:locale==='en'?/^Clear$/:/^清空$/}).click();await expect.poll(()=>reads.at(-1)!.searchParams.get('state')).toBeNull()
   }
   empty=true;await page.goto(origins.user+path);await expect(page.locator('.empty-state')).toBeVisible();await expect(pager).toContainText(locale==='en'?'Total: 0':'共 0 条')
   const preset=page.viewportSize()!;for(const width of info.project.name.endsWith('reflow')?[320,900,1440]:[preset.width]){if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:preset.height});await expectRealEntry(page,'user');await expectListLayout(page);await page.screenshot({path:info.outputPath(`empty-${path.replaceAll('/','-')}-${locale}-${width}.png`),fullPage:true})}
  }
 })
 test(`user withdrawal records share pagination and preserve failed pages (${locale})`,async({page},info)=>{
  await installLayoutApi(page,locale);await installWalletLayoutApi(page)
  let fail=false,empty=false;const offsets:number[]=[]
  await page.route('**/api/v1/withdrawal*',route=>{
   const req=route.request(),url=new URL(req.url());expect(req.method()).toBe('GET')
   if(url.pathname.endsWith('withdrawal-availability'))return route.fulfill({json:{success:true,data:{points_step:'1',available_points:'100',source_balances:[{source:'recharge',state:'available',points:'100'}],actor_context:'a'.repeat(64),brand_id:gameId,member_id:gameId,policy_enabled:false,eligibility_configured:false,can_apply:false,reason_code:'WITHDRAWAL_DISABLED',min_points:'1',max_points:null,allowed_sources:['recharge'],real_payments:false}}})
   expect(url.pathname).toBe('/api/v1/withdrawals');expect(url.searchParams.get('limit')).toBe('20');const offset=Number(url.searchParams.get('offset'));offsets.push(offset)
   if(fail){fail=false;return route.fulfill({status:503,json:{success:false,error:{code:'SERVICE_UNAVAILABLE',message:'Synthetic failure'}}})}
   const items=empty?[]:Array.from({length:offset===0?20:1},(_,i)=>({id:id(offset+i),brand_id:gameId,member_id:gameId,account_id:gameId,points:'50',state:'reviewing',version:1,source_allocation:[{source:'recharge',state:'available',points:'50'}],reserve_entry_id:gameId,release_entry_id:null,paid_entry_id:null,cycle_from_at:null,cycle_from_version:'0',reserve_version:'1',created_at:at,updated_at:at,reviewed_at:null,completed_at:null,decision_reason:'',audit_log_id:gameId}))
   return route.fulfill({json:{success:true,data:{brand_id:gameId,items,total_count:empty?'0':'21',limit:20,offset,has_more:!empty&&offset===0}}})
  })
  await page.goto(origins.user+'/withdraw');const rows=page.locator('.withdrawal-order'),pager=page.locator('.withdrawal-records .table-pagination'),next=pager.locator('.table-pagination-arrow').last()
  await expect(rows).toHaveCount(20);fail=true;await next.click();await expect(page.locator('.global-loading-overlay')).toBeHidden();await expect(rows).toHaveCount(20);await expect(pager.locator('[aria-current=page]')).toHaveText('1')
  await next.click();await expect(rows).toHaveCount(1);await expect(pager.locator('[aria-current=page]')).toHaveText('2');await expect(next).toBeDisabled();expect(offsets).toEqual([0,20,20])
  empty=true;await page.goto(origins.user+'/withdraw');await expect(page.locator('.withdrawal-records .empty-state')).toBeVisible();await expect(pager).toContainText(locale==='en'?'Total: 0':'共 0 条');await expectRealEntry(page,'user');await expectListLayout(page);await page.screenshot({path:info.outputPath(`withdraw-empty-${locale}.png`),fullPage:true})
 })
}
