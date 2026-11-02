import {test,expect} from '@playwright/test'
import {registerAgentPortalTests} from './agent-portal'
import {registerWorkspaceTests,registerConfiguredSaleTests,registerConfiguredHistoryLayoutTests} from './workspace'
import {installLayoutApi,installWalletLayoutApi,origins,gameId,expectNoPageOverflow,expectInsideViewport} from './fixture'
export function registerPlayerPages(reflow=false){
registerAgentPortalTests(reflow)
registerWorkspaceTests()
registerConfiguredSaleTests()
registerConfiguredHistoryLayoutTests()
for(const locale of ['en','zh-CN'] as const){
 test(`player ledger details and shared pagination (${locale})`,async({page},info)=>{
  await installLayoutApi(page,locale);await installWalletLayoutApi(page)
  await page.goto(origins.user+'/wallet/ledger')
  const trigger=page.locator('.ledger-details'),dialog=page.getByRole('dialog')
  await expect(page.locator('.table-pagination')).toHaveCount(1)
  await expect(page.locator('.entry-change strong')).toHaveText('−10')
  await trigger.click();await expect(dialog).toBeVisible();await expect(dialog.locator('.ledger-result dd')).toHaveText('120')
  const preset=page.viewportSize()!
  for(const width of info.project.name.endsWith('reflow')?[320,620,900,1000,1440]:[preset.width]){
   if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:preset.height})
   await expectInsideViewport(dialog);await expectNoPageOverflow(page)
   await page.screenshot({path:info.outputPath(`ledger-detail-${locale}-${width}.png`)})
  }
  const buttons=page.locator('.table-pagination-arrow')
  await expect(buttons).toHaveCount(2);await expect(buttons.first()).toBeDisabled();await expect(buttons.last()).toBeDisabled()
 })
 test(`player recharge records keep shared modal and page actions (${locale})`,async({page},info)=>{
  await installLayoutApi(page,locale);await installWalletLayoutApi(page)
  const row={method:'manual',id:gameId,brand_id:gameId,member_id:gameId,points:'100',state:'pending',version:'1',created_at:'2026-10-27T03:20:52Z',confirmed_at:null,ledger_entry_id:null}
  const offsets:number[]=[]
  await page.route('**/api/v1/recharges**',route=>{
   expect(route.request().method()).toBe('GET');const url=new URL(route.request().url())
   if(url.pathname.endsWith('/'+gameId))return route.fulfill({json:{success:true,data:row}})
   const offset=Number(url.searchParams.get('offset'));offsets.push(offset)
   return route.fulfill({json:{success:true,data:{brand_id:gameId,member_id:gameId,snapshot_at:'2026-10-27T04:00:00Z',state:null,items:[row],limit:20,offset,total_count:'21'}}})
  })
  await page.goto(origins.user+'/recharge')
  const paginator=page.locator('.table-pagination')
  await expect(paginator).toHaveCount(1)
  await paginator.locator('.table-pagination-arrow').last().click();await expect.poll(()=>offsets.at(-1)).toBe(20)
  await expect(paginator.locator('.table-pagination-arrow').last()).toBeDisabled()
  await paginator.locator('.table-pagination-arrow').first().click();await expect.poll(()=>offsets.at(-1)).toBe(0)
  await page.locator('.recharge-record .text-button').click();const dialog=page.getByRole('dialog')
  await expect(dialog).toContainText('100');await expect(dialog.locator('.detail-panel')).toHaveCount(0)
  const preset=page.viewportSize()!
  for(const width of info.project.name.endsWith('reflow')?[320,620,900,1440]:[preset.width]){
   if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:preset.height})
   await expectInsideViewport(dialog);await expectNoPageOverflow(page)
   await page.screenshot({path:info.outputPath(`recharge-detail-${locale}-${width}.png`)})
  }
  await page.keyboard.press('Escape');await expect(dialog).toHaveCount(0)
 })
 test(`player profile completion uses the shared form modal (${locale})`,async({page},info)=>{
  await installLayoutApi(page,locale);await installWalletLayoutApi(page)
  const profile={user:{id:gameId,username:'layout-member',status:'active',phone:null},member:{id:gameId,brand_id:gameId,status:'normal',display_name:'Layout member',joined_at:'2026-10-17T00:00:00Z'}}
  let writes=0
  await page.route('**/api/v1/me',route=>route.fulfill({json:{success:true,data:profile}}))
  await page.route('**/api/v1/me/profile',route=>{
   expect(route.request().method()).toBe('PATCH');expect(route.request().postDataJSON()).toEqual({phone:'+6591234567'});writes++;profile.user.phone='+6591234567' as any
   return route.fulfill({json:{success:true,data:{audit_log_id:gameId}}})
  })
  await page.goto(origins.user+'/account')
  const name=locale==='en'?'Complete profile':'补充资料'
  const trigger=page.getByRole('button',{name,exact:true});await trigger.click()
  const dialog=page.getByRole('dialog',{name,exact:true});await expect(dialog).toBeVisible()
  const preset=page.viewportSize()!
  for(const width of info.project.name.endsWith('reflow')?[320,620,900,1440]:[preset.width]){
   if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:preset.height})
   await expectInsideViewport(dialog);await expectNoPageOverflow(page)
   await page.screenshot({path:info.outputPath(`profile-form-${locale}-${width}.png`)})
  }
  await dialog.locator('input[autocomplete=tel]').fill('+6591234567');await dialog.locator('button[type=submit]').click()
  await expect(dialog).toHaveCount(0);expect(writes).toBe(1);await expect(page.locator('.account-details')).toContainText('+6591234567')
 })
}


for(const locale of ['en','zh-CN'] as const)test(`player authentication forms share controls (${locale})`,async({page},info)=>{
 await installLayoutApi(page,locale)
 const preset=page.viewportSize()!
 for(const path of ['/login','/register']){
  await page.goto(origins.user+path)
  const form=page.locator('.auth-form');await expect(form).toBeVisible()
  for(const width of info.project.name.endsWith('reflow')?[320,620,900,1440]:[preset.width]){
   if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:preset.height})
   await expectNoPageOverflow(page)
   const password=form.locator('input[type=password]').first();await password.focus();await expectInsideViewport(password)
   const required=await page.evaluate(()=>matchMedia('(min-width:901px) and (pointer:fine)').matches?32:44)
   expect((await password.boundingBox())!.height).toBe(required)
   await page.screenshot({path:info.outputPath(`authentication-${path.slice(1)}-${locale}-${width}.png`)})
  }
 }
})

}
