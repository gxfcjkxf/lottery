import {test,expect} from '@playwright/test'
import {installConsoleSession,openBrandPage,openPlatformPage} from './real-console-session'
import {expectRealEntry,expectListLayout} from './real-page-review'
import {origins,expectInsideViewport} from './fixture'
const brand='11111111-1111-4111-8111-111111111111',id='44444444-4444-4444-8444-444444444444',at='2026-10-24T00:00:00Z'
const withdrawal={id,brand_id:brand,member_id:id,account_id:id,points:'2500',state:'reviewing',version:1,source_allocation:[{source:'recharge',state:'available',points:'2500'}],reserve_entry_id:id,release_entry_id:null,paid_entry_id:null,cycle_from_at:null,cycle_from_version:'0',reserve_version:'1',created_at:at,updated_at:at,reviewed_at:null,completed_at:null,decision_reason:'',audit_log_id:id}
const reward={id,brand_id:brand,member_id:id,points:'2500',state:'granted',version:1,grant_ledger_entry_id:id,revoke_ledger_entry_id:null,creation_audit_log_id:id,last_audit_log_id:id,created_by:id,reason:'历史备注 historical note '.repeat(6).trim(),point_policy_version:'1',created_at:at,updated_at:at,revoked_at:null}
for(const locale of ['en','zh-CN'] as const)test(`finance lists and dialogs share layout (${locale})`,async({page},info)=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message))
 await page.route('**/api/**',route=>{
  expect(route.request().method()).toBe('GET')
  const url=new URL(route.request().url()),path=url.pathname
  let data:unknown
  if(path.endsWith('/cash/orders'))data={items:[],total_count:'0'}
  else if(path.endsWith('/reward-orders'))data={brand_id:brand,items:[reward],total_count:'1',limit:Number(url.searchParams.get('limit')),offset:0}
  else if(path.endsWith('/actions'))data={brand_id:brand,order_id:id,items:[],total_count:'0',limit:Number(url.searchParams.get('limit')),offset:0}
  else if(path.endsWith('/reward-orders/'+id))data=reward
  else if(path.endsWith('/withdrawals'))data={brand_id:brand,items:[withdrawal],has_more:false,limit:Number(url.searchParams.get('limit')),offset:0, total_count: '1'}
  else if(path.endsWith('/history'))data={brand_id:brand,order_id:id,items:[{id,version:1,from_state:'',to_state:'reviewing',reason:'',actor_type:'user',created_at:at,audit_log_id:id}], total_count: '1'}
  else if(path.endsWith('/withdrawals/'+id))data=withdrawal
  else throw Error(`Unexpected finance layout request ${path}`)
  return route.fulfill({json:{success:true,request_id:'finance-layout-review',data,associations:{[`member:${id}`]:{name:'finance_member'},[`brand:${brand}`]:{name:'Aurora'},[`admin:${id}`]:{name:'finance_reviewer'}}}})
 })
 for(const view of ['brand-withdrawals','brand-rewards','platform-withdrawals','platform-rewards']){
  const surface=view.startsWith('brand')?'brand':'platform'
  const grants=surface==='brand'?['recharge.view.brand','withdrawal.view.brand','withdrawal.approve.brand','reward.view.brand','reward.grant.brand']:['withdrawal.view.platform','reward.view.platform']
  await installConsoleSession(page,surface,locale,brand,grants)
  await page.goto(origins[surface]);await expect(page.locator('.management-console')).toBeVisible()
  if(surface==='brand')await openBrandPage(page,view.endsWith('withdrawals')?(locale==='en'?'Withdrawal requests':'提现申请'):(locale==='en'?'Manual rewards':'人工奖励'))
  else await openPlatformPage(page,view.endsWith('withdrawals')?(locale==='en'?'Withdrawals':'提现'):(locale==='en'?'Rewards':'奖励'),brand)
  await expectRealEntry(page,surface)
  const list=page.locator(view==='brand-withdrawals'?'.withdrawal-management .management-list':view==='platform-withdrawals'?'[data-testid=platform-withdrawals]':'.management-list').first(),row=list.locator('table tbody tr').first()
  await expect(row).toContainText('2500')
  await expect(list.locator('.table-pagination').first()).toBeVisible()
  const preset=page.viewportSize()!
  for(const width of info.project.name.includes('reflow')?[320,640,900,901,1440]:[preset.width]){
   if(info.project.name.includes('reflow'))await page.setViewportSize({width,height:preset.height})
   await expectListLayout(page)
   await page.screenshot({path:info.outputPath(`${view}-${locale}-${width}.png`)})
  }
  await row.getByRole('button').click()
  const dialog=page.getByRole('dialog')
  await expect(dialog).toBeVisible()
  for(const width of info.project.name.includes('reflow')?[320,640,900,1440]:[preset.width]){
   if(info.project.name.includes('reflow'))await page.setViewportSize({width,height:preset.height})
   await expectInsideViewport(dialog);await expectListLayout(page)
   const centered=await dialog.evaluate(el=>{const r=el.getBoundingClientRect();return Math.abs(r.x*2+r.width-innerWidth)<2&&Math.abs(r.y*2+r.height-innerHeight)<2})
   expect(centered).toBe(true)
   await page.screenshot({path:info.outputPath(`${view}-dialog-${locale}-${width}.png`)})
  }
  if(view==='brand-withdrawals'){
   await dialog.getByRole('button',{name:locale==='en'?'Approve':'通过',exact:true}).click()
   const confirmation=page.getByRole('dialog',{name:locale==='en'?'Approve':'通过',exact:true})
   await expect(confirmation).toBeVisible();await expect(confirmation).toContainText('2500')
   await expectInsideViewport(confirmation)
   await page.screenshot({path:info.outputPath(`withdrawal-confirm-${locale}.png`)})
   await page.keyboard.press('Escape');await expect(confirmation).toHaveCount(0)
   await expect(dialog).toBeVisible()
  }
  await page.keyboard.press('Escape');await expect(dialog).toHaveCount(0)
  if(info.project.name.includes('reflow'))await page.setViewportSize(preset)
 }
 expect(errors).toEqual([])
})
