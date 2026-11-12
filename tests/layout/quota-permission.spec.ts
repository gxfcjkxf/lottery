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
  const navigation=page.locator('button[aria-controls="platform-navigation"]')
  if(await navigation.isVisible() && await navigation.getAttribute('aria-expanded')==='false')await navigation.click()
  const funds=page.locator('.sidebar').getByRole('button',{name:locale==='en'?'Finance':'资金',exact:true})
  if(await funds.getAttribute('aria-expanded')==='false')await funds.click()
  await page.getByTestId('platform-wallet-tabs').getByRole('button',{name:locale==='en'?'Quota management':'额度管理',exact:true}).click()
  await page.locator('.topbar-brand-picker .select-control-trigger').click()
  await page.getByRole('option',{name:'Quota Brand',exact:true}).click()
  const dialog=page.getByTestId('recharge-quota'),form=dialog.locator('.quota-form')
  await expect(dialog.locator('.console-facts strong')).toHaveText('0')
  await expect(form).toHaveCount(0)
  const operation=dialog.getByRole('button',{name:locale==='en'?'Operation':'额度操作',exact:true})
  await expect(operation).toHaveCount(allowed?1:0)
  if(allowed){
   await operation.click()
   await expect(page.getByRole('dialog')).toBeVisible()
   const submit=form.locator('button[type=submit]');await expect(submit).toHaveText(locale==='en'?'Add quota':'增加额度');await expect(submit).toBeDisabled()
   await form.locator('input[inputmode=decimal]').fill('0.25');await expect(submit).toBeEnabled()
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
  await expect(form).toHaveCount(0)
  if(allowed)await expect(operation).toBeFocused()
 }
 expect(errors).toEqual([])
})

for(const locale of ['en','zh-CN'] as const)test(`brand quota management is a separate read-only page (${locale})`,async({page},info)=>{
 await installLayoutApi(page,locale)
 const at='2026-11-01T00:00:00Z',errors:string[]=[]
 page.on('pageerror',error=>errors.push(error.message))
 await page.route('**/api/v1/admin/**',route=>{
  expect(route.request().method()).toBe('GET')
  const url=new URL(route.request().url()),path=url.pathname,reply=(data:unknown)=>route.fulfill({json:{success:true,data}})
  if(path.endsWith('/me'))return reply({account:{id:gameId,username:'quota_page_review',super_admin:false,brand_ids:[gameId],permissions:[],permissions_by_brand:{[gameId]:['recharge.view.brand']}}})
  if(path.endsWith('/brands'))return reply({items:[{id:gameId,code:'quota_review',name:'Quota Brand',status:'active',member_count:'0',agent_count:'0',recharge_quota_available:'100',created_at:at}],total_count:'1'})
  expect(route.request().headers()['x-brand-id']).toBe(gameId)
  if(path.endsWith('/recharge-quota'))return reply({brand_id:gameId,available:'100'})
  if(path.endsWith('/recharge-quota/entries')){
   const offset=Number(url.searchParams.get('offset')),limit=Number(url.searchParams.get('limit'))
   return reply({items:Array.from({length:Math.min(limit,11-offset)},(_,i)=>({id:`quota-${offset+i}`,brand_id:gameId,kind:(offset+i)%2?'manual_adjustment':'manual_recharge',member_id:gameId,member_account:'test001',points:'10',before_available:'110',after_available:'100',actor_name:'quota_page_review',created_at:at})),total_count:'11'})
  }
  throw Error('Unexpected quota page request '+path)
 })
 await page.goto(origins.brand)
 await expect(page.locator('.management-console')).toBeVisible()
 const more=page.locator('.mobile-nav > button').last()
 if(await more.isVisible())await more.click()
 await page.locator(await more.isVisible()?'.mobile-more-menu':'.side-nav').getByRole('button',{name:locale==='en'?'Quota management':'额度管理',exact:true}).click()
 const quota=page.getByTestId('recharge-quota')
 await expect(quota.locator('tbody tr')).toHaveCount(10)
 await expect(quota.locator('.quota-form')).toHaveCount(0)
 await expect(quota.locator('tbody tr').first()).toContainText(locale==='en'?'Member test001':'会员 test001')
 await expect(quota.locator('thead')).toContainText(locale==='en'?'Source':'来源')
 await expect(quota.locator('thead')).toContainText(locale==='en'?'Destination':'去向')
 await expect(page.getByRole('dialog')).toHaveCount(0)
 await quota.getByRole('button',{name:locale==='en'?'Next':'下一页',exact:true}).click()
 await expect(quota.locator('tbody tr')).toHaveCount(1)
 const preset=page.viewportSize()!
 for(const width of info.project.name.endsWith('reflow')?[320,640,900,901,1440]:[preset.width]){
  if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:preset.height})
  await expectNoPageOverflow(page)
  await page.screenshot({path:info.outputPath(`brand-quota-${locale}-${width}.png`),fullPage:true})
 }
 expect(errors).toEqual([])
})
