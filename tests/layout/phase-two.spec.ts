import { expect, test, type Page, type TestInfo } from '@playwright/test'
import { resolve } from 'node:path'
import { origins, expectNoPageOverflow, gameId } from './fixture'
const brand = '11111111-1111-4111-8111-111111111111', at = '2026-10-27T00:00:00Z'
const effective = { display_name:'Trial',logo_text:'T',logo_url:null,favicon_url:null,default_locale:'en',available_locales:['en','zh-CN'],content:{en:{tagline:'',announcement:''},'zh-CN':{tagline:'',announcement:''}} }
async function open(page: Page, locale: 'en'|'zh-CN', view: 'presentation'|'compliance'|'templates'|'workbench') {
 const source = await (await page.request.get(origins.brand+'/src/ReportsManagement.vue')).text()
 const vuePath = source.match(/from "([^"]*deps\/vue\.js[^"]*)"/)![1]
 await page.route('**/__phase-two', route => route.fulfill({ contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module">import{createApp}from'${vuePath}';import Harness from'/@fs/${resolve(view==='presentation'?'tests/layout/PageActionsHarness.vue':'tests/layout/PhaseTwoSettingsHarness.vue')}';import'/@fs/${resolve('shared/src/styles.css')}';import'/@fs/${resolve('admin-web/src/style.css')}';import'/@fs/${resolve('shared/src/management.css')}';import'/@fs/${resolve('shared/src/controls.css')}';import'/@fs/${resolve('shared/src/refresh-button.css')}';import'/@fs/${resolve('shared/src/key-value.css')}';createApp(Harness,${JSON.stringify({locale,view})}).mount('#app')</script></html>` }))
 await page.goto(origins.brand+'/__phase-two')
}
async function review(page:Page,info:TestInfo,name:string) {
 const preset=page.viewportSize()!
 for(const width of info.project.name.includes('reflow')?[320,640,900,1440]:[preset.width]) {
  if(info.project.name.includes('reflow'))await page.setViewportSize({width,height:preset.height})
  await expectNoPageOverflow(page)
  await page.screenshot({path:info.outputPath(`${name}-${width}.png`),fullPage:true})
 }
}
for(const locale of ['en','zh-CN'] as const) {
 test(`phase two presentation saves directly and reloads unknown results without losing drafts (${locale})`,async({page},info)=>{
  const brand=gameId
  let version=1, config={...effective}, writes=0
  await page.route('**/api/**',async route=>{
   const path=new URL(route.request().url()).pathname
   const reply=(data:unknown)=>route.fulfill({json:{success:true,data}})
   if(path.endsWith('/defaults'))return reply(effective)
   expect(path).toBe('/api/v1/admin/brand-presentation')
   const record=()=>({brand_id:brand,version,status:'active',base_name:'Trial',config,effective:{...effective,...config},updated_at:at})
   if(route.request().method()==='PUT'){
    writes++;const body=route.request().postDataJSON();expect(body.version).toBe(version)
    expect(route.request().headers()['idempotency-key']).toBeTruthy()
    config=body.config;version++
    if(writes===1)return route.fulfill({status:503,json:{success:false,error:{code:'SERVICE_UNAVAILABLE',message:'Synthetic lost receipt'}}})
    return reply({...record(),audit_log_id:brand})
   }
   expect(route.request().method()).toBe('GET');return reply(record())
  })
  await open(page,locale,'presentation')
  const input=page.locator('.brand-name-field input'),save=page.getByRole('button',{name:locale==='en'?'Save':'保存',exact:true})
  await expect(input).toHaveValue('Trial');await input.fill('Draft name');await save.click()
  await expect.poll(()=>writes).toBe(1);await expect(page.getByRole('dialog')).toHaveCount(0)
  await expect(input).toHaveValue('Draft name');await expect(save).toBeDisabled()
  await page.getByRole('button',{name:locale==='en'?'Reload':'重新读取',exact:true}).click()
  await expect(input).toHaveValue('Draft name');await expect(save).toBeEnabled()
  await expect(save.locator('xpath=ancestor::form[1]')).not.toHaveAttribute('data-write-locked','true')
  await input.fill('Final name');await save.click();await expect.poll(()=>writes).toBe(2)
  await expect(save).toBeEnabled();await review(page,info,`presentation-${locale}`)
  await page.reload()
  await expect(input).toHaveValue('Final name');expect(writes).toBe(2);await expect(page.getByRole('dialog')).toHaveCount(0)
 })
 test(`phase two policy preserves enabled gates without manual pseudo checks (${locale})`,async({page},info)=>{
  let writes=0
  await page.route('**/api/**',route=>{
   expect(route.request().method()).toBe('GET');writes+=Number(route.request().method()!=='GET')
   expect(new URL(route.request().url()).pathname).toBe('/api/v1/admin/compliance-policy')
   return route.fulfill({json:{success:true,data:{brand_id:brand,version:1,updated_at:at,config:{age_enabled:false,minimum_age:null,region_enabled:false,allowed_countries:[],identity_enabled:false,account_risk_enabled:false,betting_risk_enabled:true,exclusion_enabled:false,responsible_gambling_enabled:false}}}})
  })
  await open(page,locale,'compliance')
  await expect(page.locator('.compliance-capability-note')).toBeVisible()
  await expect(page.locator('.compliance input[type=checkbox]')).toHaveCount(7)
  await expect(page.locator('.compliance input[type=checkbox]').nth(4)).toBeChecked()
  await expect(page.locator('.check-result')).toHaveCount(0)
  await expect(page.getByRole('button',{name:locale==='en'?'Run check':'运行检查',exact:true})).toHaveCount(0)
  expect(writes).toBe(0);await review(page,info,`policy-${locale}`)
 })
 test(`phase two template rereads current version and keeps the editable draft (${locale})`,async({page},info)=>{
  let version=1,writes=0,content={en:{title:'Welcome',body:'Hello'},'zh-CN':{title:'欢迎',body:'你好'}}
  await page.route('**/api/**',route=>{
   const path=new URL(route.request().url()).pathname
   const template=()=>({key:'member.joined',version,content,updated_at:at,audit_log_id:brand})
   const reply=(data:unknown)=>route.fulfill({json:{success:true,data}})
   if(route.request().method()==='PUT'){
    expect(path).toBe('/api/v1/platform/notification-templates/member.joined')
    const body=route.request().postDataJSON();expect(body.version).toBe(version);content=body.content;version++;writes++
    if(writes===1)return route.fulfill({status:503,json:{success:false,error:{code:'SERVICE_UNAVAILABLE',message:'Synthetic lost receipt'}}})
    return reply(template())
   }
   expect(route.request().method()).toBe('GET');expect(path).toBe('/api/v1/platform/notification-templates');return reply({items:[template()]})
  })
  await open(page,locale,'templates');await page.locator('.template-table .row-action').click()
  const dialog=page.getByRole('dialog'),input=dialog.locator('input').first(),save=dialog.locator('button[type=submit]')
  await input.fill('Draft template');await save.click();await expect.poll(()=>writes).toBe(1)
  await expect(input).toBeDisabled();await dialog.getByRole('button',{name:locale==='en'?'Reload current template':'重新读取当前模板',exact:true}).click()
  await expect(input).toHaveValue('Draft template');await expect(input).toBeEnabled()
  await expect(save.locator('xpath=ancestor::form[1]')).not.toHaveAttribute('data-write-locked','true')
  await input.fill('Final template');await save.click();await expect.poll(()=>writes).toBe(2)
  await expect(input).toBeEnabled();await review(page,info,`template-${locale}`)
 })
}

for(const locale of ['en','zh-CN'] as const)test(`phase two workbench shows actionable commission counters (${locale})`,async({page},info)=>{
 const keys=['discovery_pending_count','discovery_failed_count','cycle_processing_count','cycle_waiting_count','cycle_ready_count','cycle_stale_count','cycle_failed_count','payment_awaiting_approval_count','payment_processing_count','payment_blocked_count','payment_failed_count','plan_processing_count','plan_ready_count','plan_failed_count','execution_awaiting_approval_count','execution_processing_count','execution_paused_count','execution_failed_count']
 const snapshot:Record<string,unknown>={brand_id:brand,snapshot_at:at,timezone:'UTC',day_from:at}
 for(const key of ['brand','orders','today_bets','recharges','ledger','balances','withdrawals','rewards'])snapshot[key]={status:'forbidden',data:null}
 snapshot.commissions={status:'ready',data:Object.fromEntries(keys.map((key,index)=>[key,String(index+1)]))}
 await page.route('**/api/**',route=>{expect(route.request().method()).toBe('GET');expect(new URL(route.request().url()).pathname).toBe('/api/v1/admin/workbench');return route.fulfill({json:{success:true,data:snapshot,associations:{['brand:'+brand]:{name:'Trial'}}}})})
 await open(page,locale,'workbench')
 const card=page.locator('article').filter({has:page.getByRole('heading',{name:locale==='en'?'Commissions':'佣金',exact:true})})
 await expect(card.locator('.metrics > div')).toHaveCount(7)
 await expect(card).toContainText(locale==='en'?'Payments awaiting approval':'付款待审批')
 await expect(card).not.toContainText(locale==='en'?'Discovery pending':'待发现')
 await review(page,info,`workbench-${locale}`)
})
