import {openPlatformPage} from './real-console-session'
import {test,expect,type Page,type TestInfo} from '@playwright/test'
import {resolve} from 'node:path'
import {origins,gameId,theme,installLayoutApi,expectNoPageOverflow} from './fixture'
async function review(page:Page,info:TestInfo,name:string){const preset=page.viewportSize()!;for(const width of info.project.name.includes('reflow')?[320,700,900,1440]:[preset.width]){if(info.project.name.includes('reflow'))await page.setViewportSize({width,height:preset.height});await expectNoPageOverflow(page);if(await page.locator('.detail-modal[open]').count())await page.locator('.detail-modal[open] .detail-modal-content').evaluate(node=>{node.scrollTop=0});await page.screenshot({path:info.outputPath(`${name}-${width}.png`)})}}
for(const locale of ['en','zh-CN'] as const){
 test(`security settings stay on platform when brand changes (${locale})`,async({page},info)=>{
  const zh=locale==='zh-CN',errors:string[]=[];page.on('pageerror',e=>errors.push(e.message))
  const auth={version:1,inherit_platform:false,captcha_enabled:true,telegram_enabled:false,telegram_client_id:'',}
  const records=new Map<string,any>(),writes:{brand:string;kind:string;inherit:boolean}[]=[]
  await page.route('**/api/**',route=>{
   const request=route.request(),brand=request.headers()['x-brand-id']??'',kind='auth',key=kind+brand
   expect(brand).toBe('')
   expect(request.url()).toMatch(/auth-settings$/); const record=records.get(key)??{...auth,inherit_platform:!!brand}
   if(request.method()!=='GET'){const body=request.postDataJSON();expect(body.version).toBe(record.version);writes.push({brand,kind,inherit:body.inherit_platform});Object.assign(record,body,{version:record.version+1,audit_log_id:gameId});delete record.reason}
   records.set(key,record);return route.fulfill({json:{success:true,data:record}})
  })
  const source=await(await page.request.get(origins.platform+'/src/SecuritySettingsPanel.vue')).text(),vue=source.match(/from "([^"]*deps\/vue\.js[^"]*)"/)![1]
  await page.route('**/__security',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module">import{createApp}from'${vue}';import Harness from'/@fs/${resolve('tests/layout/SecurityInheritanceHarness.vue')}';import'/@fs/${resolve('shared/src/styles.css')}';import'/@fs/${resolve('platform-web/src/style.css')}';import'/@fs/${resolve('shared/src/management.css')}';import'/@fs/${resolve('shared/src/controls.css')}';import'/@fs/${resolve('shared/src/key-value.css')}';createApp(Harness,${JSON.stringify({locale})}).mount('#app')</script>`}))
  await page.goto(origins.platform+'/__security')
  const settings=page.locator('.security-settings')
  await expect(settings.getByRole('button',{name:zh?'品牌设置':'Brand settings',exact:true})).toHaveCount(0)
  let panel=settings.locator('[data-kind=auth]'),edit=panel.getByRole('button',{name:zh?'编辑设置':'Edit settings',exact:true})
  await expect(panel.locator('h2')).toHaveCount(0)
  await expect(edit).toBeEnabled()
  await review(page,info,`platform-auth-summary-${locale}`)
  const fields=await panel.locator('dl').boundingBox(),action=await edit.boundingBox()
  expect(action!.y).toBeGreaterThanOrEqual(fields!.y+fields!.height)
  await edit.click();await expect(panel.getByRole('checkbox',{name:zh?'继承平台默认':'Inherit platform defaults'})).toHaveCount(0)
  await review(page,info,`platform-auth-${locale}`);await page.keyboard.press('Escape')
  await page.getByTestId('select-brand').click();await expect(settings.locator('.console-tabs')).toHaveCount(0)
  for(const kind of ['auth']){
   panel=settings.locator(`[data-kind=${kind}]`)
   edit=panel.getByRole('button',{name:zh?'编辑设置':'Edit settings',exact:true})
   await edit.click()
   await expect(panel.getByRole('checkbox',{name:zh?'继承平台默认':'Inherit platform defaults'})).toHaveCount(0)
   await expect(panel.locator('fieldset input').first()).toBeEnabled()

   await review(page,info,`platform-${kind}-${locale}`)
   await panel.locator('button[type=submit]').click();await page.getByRole('dialog',{name:zh?'确认设置变更':'Confirm changes'}).locator('.button-primary').click()
   await expect(panel.getByRole('dialog')).toHaveCount(0)
  }
  await page.getByTestId('clear-brand').click()
  await expect(settings.locator('.console-tabs')).toHaveCount(0)
  expect(writes).toEqual([{brand:'',kind:'auth',inherit:false}]);expect(errors).toEqual([])
 })
 test(`captcha login on user, brand and agent (${locale})`,async({page},info)=>{
  await page.clock.install()
  const zh=locale==='zh-CN';await installLayoutApi(page,locale)
  await page.route('**/api/v1/context',route=>route.fulfill({json:{success:true,data:{auth:{captcha_enabled:true,telegram_enabled:false,telegram_client_id:''},brand:{id:gameId,code:'layout',name:'Layout trial',status:'active',default_locale:'en',timezone:'Asia/Singapore',theme,config_version:1},available_locales:['en','zh-CN'],features:{pwa:false,real_payments:false},terms:{}}}}))
  await page.route('**/api/v1/agent/auth/config',route=>route.fulfill({json:{success:true,data:{captcha_enabled:true}}}))
  await page.route('**/api/v1/admin/auth/config',route=>route.fulfill({json:{success:true,data:{captcha_enabled:true}}}))
  let challenges=0,issuedAt=0;await page.route('**/auth/challenge',async route=>{const now=await page.evaluate(()=>Date.now());issuedAt=now;challenges++;return route.fulfill({json:{success:true,data:{id:gameId,puzzle:{image:'data:image/svg+xml,'+encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" width="300" height="160"><rect width="300" height="160" fill="teal"/></svg>'),piece:'data:image/svg+xml,'+encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" width="46" height="46"><rect width="46" height="46" fill="white"/></svg>'),width:300,height:160,piece_width:46,piece_height:46,y:50},expires_at:new Date(now+300000).toISOString()}}})})
  const submissions:any[]=[]
  await page.route('**/auth/login',route=>{submissions.push(route.request().postDataJSON());return route.fulfill({status:400,json:{success:false,error:{code:'CAPTCHA_INVALID',message:'CAPTCHA_INVALID'}}})})
  await page.route('**/agent/login',route=>{submissions.push(route.request().postDataJSON());return route.fulfill({status:400,json:{success:false,error:{code:'CAPTCHA_INVALID',message:'CAPTCHA_INVALID'}}})})
  for(const surface of ['user','brand','agent'] as const){
   await page.goto(origins[surface]+(surface==='user'?'/login':''));const captcha=page.locator(surface==='user'?'.captcha-box':'.login-captcha')
   await expect(captcha).toBeVisible();await expect(captcha.locator('img').first()).toBeVisible();await expect(captcha).not.toContainText(/有效期|提交失败后|Valid until/)
   await page.locator('input[autocomplete=username]').fill('security_test');await page.locator('input[type=password]').fill('security123');await captcha.locator('input[type=range]').fill('150')
   await review(page,info,`${surface}-captcha-${locale}`)
   const beforeExpiry=challenges;await page.clock.pauseAt(new Date(issuedAt+269000));expect(challenges).toBe(beforeExpiry)
   await page.clock.fastForward(2000);await expect.poll(()=>challenges).toBe(beforeExpiry+1)
   await page.clock.resume()
   await expect(captcha.locator('input')).toHaveValue('0');await captcha.locator('input[type=range]').fill('150')
   const previous=challenges;await page.locator('form button[type=submit],form button:not([type])').last().click()
   await expect.poll(()=>submissions.length).toBe(['user','brand','agent'].indexOf(surface)+1)
   expect(submissions.at(-1)).toMatchObject({captcha_id:gameId,captcha_answer:'150'});await expect.poll(()=>challenges).toBeGreaterThan(previous);await expect(captcha.locator('input')).toHaveValue('0')
  }
 })
}
test('actual platform entry opens defaults without selecting a brand',async({page},info)=>{
 test.skip(info.project.name!=='chromium-reflow','One application navigation check; device layout is covered above.')
 await installLayoutApi(page,'en')
 await page.route('**/api/v1/platform/**',route=>{
  const path=new URL(route.request().url()).pathname
  const reply=(data:unknown)=>route.fulfill({json:{success:true,data}})
  if(path.endsWith('/presentation'))return reply({display_name:'Trial platform',logo_text:'T',logo_url:''})
  if(path.endsWith('/me'))return reply({account:{id:gameId,username:'security_test',super_admin:true,platform_permissions:['auth_config.view.platform','auth_config.write.platform','platform_settings.view.platform','platform_settings.write.platform']}})
  if(path.endsWith('/platform-settings'))return reply({theme:'jade',version:1,display_name:'Trial platform',logo_text:'T',logo_url:'',timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss',updated_at:'2026-10-29T00:00:00Z'})
  if(path.endsWith('/brands'))return reply({items:[]})
  expect(route.request().headers()['x-brand-id']).toBeUndefined()
  if(path.endsWith('/auth-settings'))return reply({version:1,inherit_platform:false,captcha_enabled:false,telegram_enabled:false,telegram_client_id:'',})
  throw Error('Unexpected platform read: '+path)
 })
 await page.goto(origins.platform)
 await openPlatformPage(page,'Settings')
 const general=page.locator('.platform-settings-page > .panel')
 const generalEdit=general.getByRole('button',{name:'Edit settings',exact:true})
 await expect(generalEdit).toBeVisible()
 await review(page,info,'platform-general-summary')
 const fields=await general.locator('dl').boundingBox(),action=await generalEdit.boundingBox()
 expect(action!.y).toBeGreaterThanOrEqual(fields!.y+fields!.height)
 await page.locator('.platform-settings-tabs').getByRole('button',{name:'Login',exact:true}).click()
 const panel=page.locator('.security-settings');await expect(panel.locator('[data-kind=auth]').getByRole('button',{name:'Edit settings'})).toBeEnabled()
 await expect(page.getByTestId('platform-brand-required')).toHaveCount(0)
 await expect(panel.locator('[data-kind=compliance]')).toHaveCount(0)
 await expect(page.getByTestId('platform-brand-required')).toHaveCount(0);await expectNoPageOverflow(page)
})
