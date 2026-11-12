import { expect, test, type Page, type TestInfo } from '@playwright/test'
import { resolve } from 'node:path'
import { origins, gameId, installLayoutApi, installWalletLayoutApi, expectNoPageOverflow } from './fixture'
const brand='11111111-1111-4111-8111-111111111111',orderId='22222222-2222-4222-8222-222222222222',member='33333333-3333-4333-8333-333333333333',audit='44444444-4444-4444-8444-444444444444',at='2026-10-28T00:00:00Z'
async function review(page:Page,info:TestInfo,name:string){
 const preset=page.viewportSize()!
 for(const width of info.project.name.includes('reflow')?[320,640,900,1440]:[preset.width]){
  if(info.project.name.includes('reflow'))await page.setViewportSize({width,height:preset.height})
  await expectNoPageOverflow(page);await page.screenshot({path:info.outputPath(`${name}-${width}.png`),fullPage:true})
 }
}
for(const locale of ['en','zh-CN'] as const){
 test(`phase three wallet ledger reads on entry without a permanent refresh (${locale})`,async({page},info)=>{
  await page.clock.install();await installLayoutApi(page,locale);await installWalletLayoutApi(page)
  let walletReads=0,ledgerReads=0
  await page.route('**/api/v1/wallet',async route=>{walletReads++;await route.fallback()})
  await page.route('**/api/v1/wallet/ledger?**',async route=>{ledgerReads++;await route.fallback()})
  await page.goto(origins.user+'/wallet/ledger')
  const entry=page.locator('.ledger-entry');await expect(entry).toHaveCount(1)
  await expect(entry.locator('.snapshot-grid')).toHaveCount(0)
  await expect(entry.locator('.ledger-balance-after')).toHaveText('120')
  const before=[walletReads,ledgerReads];await page.clock.fastForward(35000);expect([walletReads,ledgerReads]).toEqual(before)
  await page.getByRole('button',{name:locale==='en'?'Menu':'菜单',exact:true}).click()
  await expect(page.locator('#user-page-refresh button')).toHaveCount(0)
  expect(ledgerReads).toBe(before[1])
  await expect(entry.locator('.ledger-balance-after')).toHaveText('120')
  await page.locator('.workspace-menu-close').click()
  await review(page,info,`ledger-${locale}`)
 })
 test(`phase three agent commission pagination does not reload wallet (${locale})`,async({page},info)=>{
  await installLayoutApi(page,locale)
  let walletReads=0,postingReads=0,failWallet=false
  await page.route('**/api/v1/agent/**',route=>{
   expect(route.request().method()).toBe('GET')
   const url=new URL(route.request().url()),path=url.pathname,offset=Number(url.searchParams.get('offset')??0)
   const reply=(data:unknown)=>route.fulfill({json:{success:true,data}})
   if(path.endsWith('/me'))return reply({account:{agent_account_id:gameId,username:'phase3_agent'},agent:{id:gameId,agent_account_id:gameId,brand_id:gameId,depth:1,config:{can_create_children:true,mode:'loss',ratio:'0.2',status:'active'},effective_mode:'loss',policy_version:1,version:1},code:null})
   if(path.endsWith('/members'))return reply({items:[],total_count:'0',limit:20,offset:0,summary:{member_count:'0',available_points:'0',bet_count:'0',stake_points:'0',unsettled_points:'0',net_points:'0'}})
   if(path.endsWith('/wallet')){walletReads++;if(failWallet)return route.fulfill({status:503,json:{success:false,error:{code:'SERVICE_UNAVAILABLE',message:'Synthetic balance failure'}}});return reply({commission_points:'5',available_points:'5'})}
   if(path.endsWith('/commission')){postingReads++;return reply({items:[{id:orderId,created_at:at,entry_type:'commission',points:offset?'9':'7'}],total_count:'21',limit:20,offset,credited_points:'16',debited_points:'0'})}
   throw Error('Unexpected agent read '+path)
  })
  await page.goto(origins.agent)
  const nav=page.locator(page.viewportSize()!.width<=700?'.mobile-nav':'.side-nav')
  await nav.locator('a[href="#/commission"]').click()
  const panel=page.locator('.commission-page');await expect(panel).toContainText('+7');expect(walletReads).toBe(1)
  await panel.getByRole('button',{name:locale==='en'?'Next':'下一页',exact:true}).click();await expect(panel).toContainText('+9')
  expect(walletReads).toBe(1);expect(postingReads).toBe(2)
  await expect(panel.locator('[data-refresh-button]')).toHaveCount(0)
  failWallet=true;await nav.locator('a[href="#/members"]').click();await nav.locator('a[href="#/commission"]').click()
  await expect(panel.locator('[data-refresh-button]')).toBeVisible();expect(walletReads).toBe(2)
  failWallet=false;await panel.locator('[data-refresh-button]').click();await expect(panel.locator('[data-refresh-button]')).toHaveCount(0)
  expect(walletReads).toBe(3)
  await review(page,info,`commission-${locale}`)
 })
 test(`phase three policy errors translate directly without repeating requests (${locale})`,async({page},info)=>{
  let reads=0
  await page.route('**/api/**',route=>{
   expect(route.request().method()).toBe('GET');reads++
   return route.fulfill({json:{success:true,data:{brand_id:brand,version:1,updated_at:at,config:{age_enabled:false,minimum_age:null,region_enabled:false,allowed_countries:[],identity_enabled:false,account_risk_enabled:false,betting_risk_enabled:false,exclusion_enabled:false,responsible_gambling_enabled:false}}}})
  })
  const source=await(await page.request.get(origins.platform+'/src/RewardsPanel.vue')).text(),vuePath=source.match(/from "([^"]*deps\/vue\.js[^"]*)"/)![1]
  await page.route('**/__phase-three-policy',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module">import{createApp}from'${vuePath}';import Harness from'/@fs/${resolve('tests/layout/PhaseThreeHarness.vue')}';import'/@fs/${resolve('shared/src/styles.css')}';import'/@fs/${resolve('shared/src/management.css')}';import'/@fs/${resolve('shared/src/controls.css')}';createApp(Harness,${JSON.stringify({locale,kind:'policy'})}).mount('#app')</script></html>`}))
  await page.goto(origins.platform+'/__phase-three-policy')
  await page.locator('.compliance input[type=checkbox]').first().check()
  await page.locator('.compliance button[type=submit]').click()
  await expect(page.locator('.toast-message')).toHaveText(locale==='en'?'Review the configuration and correct any invalid settings.':'请核对配置并修正无效设置。')
  const before=reads;await page.getByTestId('switch-fixture-language').click()
  await expect(page.locator('.toast-message')).toHaveText(locale==='en'?'请核对配置并修正无效设置。':'Review the configuration and correct any invalid settings.')
  expect(reads).toBe(before);await review(page,info,`localized-policy-${locale}`)
 })
 for(const kind of ['withdrawals','rewards'] as const)test(`phase three ${kind} loads history on demand and ignores stale reads (${locale})`,async({page},info)=>{
  let histories=0,fail=true,hold=false,release=()=>{},lastBrand=brand
  let gate=Promise.resolve()
  const reward={id:orderId,brand_id:brand,member_id:member,points:'250',state:'revoked',version:2,grant_ledger_entry_id:audit,revoke_ledger_entry_id:audit,creation_audit_log_id:audit,last_audit_log_id:audit,created_by:member,reason:'Trial award',point_policy_version:'1',created_at:at,updated_at:at,revoked_at:at}
  const withdrawal={id:orderId,brand_id:brand,member_id:member,account_id:audit,points:'250',state:'reviewing',version:1,source_allocation:[{source:'recharge',state:'available',points:'250'}],reserve_entry_id:audit,release_entry_id:null,paid_entry_id:null,cycle_from_at:null,cycle_from_version:'0',reserve_version:'1',created_at:at,updated_at:at,reviewed_at:null,completed_at:null,decision_reason:'',audit_log_id:audit}
  await page.route('**/api/**',async route=>{
   expect(route.request().method()).toBe('GET')
   const url=new URL(route.request().url()),path=url.pathname,isHistory=path.endsWith('/history')||path.endsWith('/actions')
   const reply=(data:unknown)=>route.fulfill({json:{success:true,data,associations:{['member:'+member]:{name:'trial_member'},['actor:'+member]:{name:'trial_operator'}}}})
   lastBrand=route.request().headers()['x-brand-id']
   if(isHistory){
    histories++;if(hold)await gate
    if(fail)return route.fulfill({status:503,json:{success:false,error:{code:'SERVICE_UNAVAILABLE',message:'Synthetic history failure'}}})
    return reply(kind==='withdrawals'?{brand_id:brand,order_id:orderId,items:[{id:audit,version:1,from_state:'',to_state:'reviewing',reason:'Initial request',actor_type:'user',created_at:at,audit_log_id:audit}]}:{brand_id:brand,order_id:orderId,items:[{id:audit,brand_id:brand,order_id:orderId,version:2,operation:'revoke',state_before:'granted',state_after:'revoked',actor_id:member,reason:'Trial reversal',audit_log_id:audit,ledger_entry_id:audit,created_at:at}],total_count:'1',limit:100,offset:0})
   }
   if(path.endsWith('/'+orderId))return reply(kind==='withdrawals'?withdrawal:reward)
   return reply(kind==='withdrawals'?{brand_id:lastBrand,items:lastBrand===brand?[withdrawal]:[],limit:50,offset:0,has_more:false}:{brand_id:lastBrand,items:lastBrand===brand?[reward]:[],total_count:lastBrand===brand?'1':'0',limit:100,offset:0})
  })
  const source=await(await page.request.get(origins.platform+'/src/RewardsPanel.vue')).text(),vuePath=source.match(/from "([^"]*deps\/vue\.js[^"]*)"/)![1]
  await page.route('**/__phase-three',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module">import{createApp}from'${vuePath}';import Harness from'/@fs/${resolve('tests/layout/PhaseThreeHarness.vue')}';import'/@fs/${resolve('shared/src/styles.css')}';import'/@fs/${resolve('platform-web/src/style.css')}';import'/@fs/${resolve('shared/src/management.css')}';import'/@fs/${resolve('shared/src/controls.css')}';import'/@fs/${resolve('shared/src/refresh-button.css')}';createApp(Harness,${JSON.stringify({locale,kind})}).mount('#app')</script></html>`}))
  await page.goto(origins.platform+'/__phase-three');await page.getByRole('button',{name:orderId,exact:true}).click()
  const dialog=page.getByRole('dialog');await expect(dialog).toContainText('250');expect(histories).toBe(0)
  const load=()=>dialog.getByRole('button',{name:locale==='en'?'View action history':'查看操作记录',exact:true})
  await load().click();await expect.poll(()=>histories).toBe(1);await expect(dialog).toContainText('250')
  fail=false;await dialog.getByRole('button',{name:locale==='en'?'Retry action history':'重新读取操作记录',exact:true}).click()
  await expect.poll(()=>histories).toBe(2);await expect(dialog).toContainText(kind==='withdrawals'?'Initial request':'Trial reversal')
  await review(page,info,`${kind}-${locale}`)
  hold=true;gate=new Promise<void>(resolve=>{release=resolve})
  await load().click();await expect.poll(()=>histories).toBe(3)
  try{
   await page.keyboard.press('Escape');await expect(dialog).toHaveCount(0)
   await page.getByTestId('switch-fixture-brand').evaluate((button:HTMLButtonElement)=>button.click());await expect.poll(()=>lastBrand).not.toBe(brand)
  }finally{release()}
  await expect(dialog).toHaveCount(0);await expect(page.getByText('Trial reversal',{exact:true})).toHaveCount(0)
 })
}
