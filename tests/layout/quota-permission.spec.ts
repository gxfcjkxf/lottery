import {expect,test} from '@playwright/test'
import {gameId,origins,installLayoutApi,openPlatformBrandList,expectNoPageOverflow} from './fixture'
for(const locale of ['en','zh-CN'] as const)test(`quota write permission restores the grant form (${locale})`,async({page},info)=>{
 await installLayoutApi(page,locale)
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));let canGrant=false
 await page.route('**/api/v1/platform/**',route=>{
  expect(route.request().method()).toBe('GET')
  const path=new URL(route.request().url()).pathname,reply=(data:unknown)=>route.fulfill({json:{success:true,data}})
  if(path.endsWith('/presentation'))return reply({display_name:'Quota review',logo_text:'Q',logo_url:''})
  if(path.endsWith('/me'))return reply({account:{id:gameId,username:'quota_permission_review',super_admin:true,platform_permissions:['brand.view.platform','recharge.view.platform',...(canGrant?['recharge_quota.write.platform']:[])]}})
  if(path.endsWith('/platform-settings'))return reply({display_name:'Quota review',logo_text:'Q',logo_url:'',timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss',version:1,updated_at:'2026-11-01T00:00:00Z'})
  if(path.endsWith('/brands'))return reply({items:[{id:gameId,name:'Quota Brand',code:'quota_review',status:'active',member_count:'0',agent_count:'0',recharge_quota_available:'0',created_at:'2026-11-01T00:00:00Z'}],total_count:'1'})
  expect(route.request().headers()['x-brand-id']).toBe(gameId)
  if(path.endsWith('/recharge-quota'))return reply({brand_id:gameId,available:'0'})
  if(path.endsWith('/recharge-quota/entries'))return reply({items:[],total_count:'0'})
  throw Error('Unexpected quota request '+path)
 })
 for(const allowed of [false,true]){
  canGrant=allowed;await page.goto(origins.platform);await openPlatformBrandList(page,locale)
  await page.getByRole('button',{name:locale==='en'?'Manage Quota Brand recharge quota':'管理 Quota Brand 的充值额度',exact:true}).click()
  const dialog=page.getByRole('dialog'),form=dialog.locator('.quota-form')
  await expect(dialog.locator('.console-facts strong')).toHaveText('0')
  await expect(form).toHaveCount(allowed?1:0)
  if(allowed){
   const submit=form.locator('button[type=submit]');await expect(submit).toHaveText(locale==='en'?'Add quota':'增加额度');await expect(submit).toBeDisabled()
   await form.locator('input[inputmode=numeric]').fill('100');await expect(submit).toBeEnabled()
   await form.getByRole('combobox').click();await page.getByRole('option',{name:locale==='en'?'Reclaim quota':'收回额度',exact:true}).click()
   await expect(submit).toBeDisabled()
   const preset=page.viewportSize()!
   for(const width of info.project.name.endsWith('reflow')?[320,1440]:[preset.width]){
    if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:preset.height})
    await expectNoPageOverflow(page)
   }
   await page.screenshot({path:info.outputPath(`quota-permission-${locale}.png`),fullPage:true})
  }
  await page.keyboard.press('Escape')
 }
 expect(errors).toEqual([])
})
