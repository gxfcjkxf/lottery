import { openPlatformPage } from './real-console-session'
import { selectControl } from '../select-control'
import { expect, test } from '@playwright/test'
import { gameId, origins, installLayoutApi, expectNoPageOverflow, expectInsideViewport } from './fixture'
export function registerSecuritySettingsTests(reflow = false) {
 for (const locale of ['en','zh-CN'] as const) test(`platform owns authentication configuration (${locale})`,async({page},info)=>{
  const zh=locale==='zh-CN';await installLayoutApi(page,locale)
  const errors:string[]=[],reads:string[]=[],writes:{body:string|null;key:string|undefined}[]=[]
  page.on('pageerror',error=>errors.push(error.message))
  let auth={version:3,inherit_platform:false,captcha_enabled:false,telegram_enabled:false,telegram_client_id:'',audit_log_id:gameId}
  let policy={inherit_platform:false,brand_id:'',version:1,config:{age_enabled:false,minimum_age:null as number|null,region_enabled:false,allowed_countries:[] as string[],identity_enabled:false,account_risk_enabled:false,betting_risk_enabled:false,exclusion_enabled:false,responsible_gambling_enabled:false},updated_at:'2026-10-21T00:00:00Z',audit_log_id:gameId}
  let lose=true,conflict=true
  const receipts=new Map<string,typeof auth>()
  await page.route('**/api/v1/platform/**',async route=>{
   const req=route.request(),path=new URL(req.url()).pathname
   const reply=(data:unknown)=>route.fulfill({json:{success:true,data}})
   if(req.method()==='GET'){
    reads.push(path)
    if(path.endsWith('/presentation'))return reply({display_name:'Platform',logo_text:'P',logo_url:''})
    if(path.endsWith('/me'))return reply({account:{id:gameId,username:'security_reviewer',super_admin:true,platform_permissions:['auth_config.view.platform','auth_config.write.platform','compliance_policy.view.platform','compliance_policy.write.platform']}})
    if(path.endsWith('/platform-settings'))return reply({theme:'jade',version:1,display_name:'Platform',logo_text:'P',logo_url:'',timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss',updated_at:'2026-10-21T00:00:00Z'})
    if(path.endsWith('/brands'))return reply({items:[{id:gameId,code:'aurora',name:'Aurora',status:'active',member_count:'12',agent_count:'3',recharge_quota_available:'1000',created_at:'2026-10-24T00:00:00Z'}]})
    if(!path.endsWith('/auth-settings') && !path.endsWith('/compliance-policy')) expect(req.headers()['x-brand-id']).toBe(gameId)
    if(path.endsWith('/auth-settings')){expect(req.headers()['x-brand-id']).toBeUndefined();return reply(auth)}
    if(path.endsWith('/compliance-policy')){expect(req.headers()['x-brand-id']).toBeUndefined();return reply({...policy,brand_id:''})}
    throw Error(`Unexpected security read: ${path}`)
   }
   expect(req.headers()['x-brand-id']).toBeUndefined()
   const key=req.headers()['idempotency-key'];expect(key).toBeTruthy();writes.push({body:req.postData(),key})
   const body=req.postDataJSON();expect(body.reason).toBe('')
   if(path.endsWith('/auth-settings')){
    expect(req.method()).toBe('PATCH')
    if(receipts.has(key))return reply(receipts.get(key))
    expect(body.version).toBe(auth.version);auth={...auth,...body,version:body.version+1};delete (auth as Record<string,unknown>).reason
    receipts.set(key,auth);if(lose){lose=false;return route.abort('failed')}return reply(auth)
   }
   if(path.endsWith('/compliance-policy')){
    expect(req.method()).toBe('PUT')
    if(conflict){conflict=false;policy={...policy,version:policy.version+1};return route.fulfill({status:409,json:{success:false,error:{code:'COMPLIANCE_VERSION_CONFLICT',message:'Synthetic conflict'}}})}
    expect(body.version).toBe(policy.version);policy={...policy,version:body.version+1,config:body.config};return reply(policy)
   }
   throw Error(`Unexpected security write: ${path}`)
  })
  await page.goto(origins.platform)
  await expect(page.locator('.account-trigger')).toBeVisible()
  const menu=page.locator('button[aria-controls="platform-navigation"]')
  if(await menu.isVisible())await menu.click()
  await expect(page.locator('.sidebar').getByRole('button',{name:zh?'认证与风控':'Authentication & risk',exact:true})).toHaveCount(0)
  await openPlatformPage(page,zh?'设置':'Settings')
  const tabs = page.locator('.platform-settings-tabs')
  await expect(tabs.getByRole('button')).toHaveCount(4)
  await tabs.getByRole('button',{name:zh?'登录':'Login',exact:true}).click()
  await selectControl(page.locator(".topbar-brand-picker .select-control-trigger"), gameId)
  const settings=page.locator('.security-settings')
  let panel=settings.locator('[data-kind=auth]')
  await expect(settings.locator('[data-kind=compliance]')).toHaveCount(0)
  await expect(settings.locator('.console-tabs')).toHaveCount(0)
  await expect(settings.getByText(zh?'继承平台设置':'Inherit platform settings',{exact:true})).toHaveCount(0)
  const form = panel.locator('form')
  const captcha = form.getByRole('checkbox', {name: zh ? '启用验证码' : 'Require CAPTCHA', exact: true})
  const save = form.getByRole('button', {name: zh ? '保存' : 'Save', exact: true})
  await expect(captcha).toBeEnabled()
  await expect(captcha).not.toBeChecked()
  await expect(page.getByRole('dialog')).toHaveCount(0)
  const preset=page.viewportSize()!
  for (const width of reflow ? [320,620,preset.width] : [preset.width]) {
   if (reflow) await page.setViewportSize({width,height:preset.height})
   await expectNoPageOverflow(page)
   await save.scrollIntoViewIfNeeded()
   await expectInsideViewport(save)
   await page.screenshot({path:info.outputPath(`login-direct-save-${locale}-${width}.png`)})
  }
  await captcha.check()
  await save.click()
  const retry = form.getByRole('button', {name: zh ? '重试' : 'Retry', exact: true})
  await expect(retry).toBeEnabled()
  expect(writes).toHaveLength(1)
  await expect(captcha).toBeDisabled()
  await expect(tabs.getByRole('button',{name:zh?'基础':'General',exact:true})).toBeDisabled()
  await expectNoPageOverflow(page)
  await retry.scrollIntoViewIfNeeded()
  await page.screenshot({path:info.outputPath(`login-direct-retry-${locale}.png`)})
  await retry.click()
  await expect(save).toBeEnabled()
  await expect(captcha).toBeChecked()
  await expect(page.getByRole('dialog')).toHaveCount(0)
  expect(writes).toHaveLength(2)
  expect(writes[0]).toEqual(writes[1])
  await expect(tabs.getByRole('button',{name:zh?'风控与合规':'Risk & compliance',exact:true})).toHaveCount(0)
  expect(reads.some(path=>/history|gates|checks/.test(path))).toBe(false);expect(errors).toEqual([])
  await page.screenshot({path:info.outputPath(`security-settings-${locale}.png`),fullPage:true})
 })
}
