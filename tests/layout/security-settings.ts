import { expect, test } from '@playwright/test'
import { gameId, origins, installLayoutApi, expectNoPageOverflow, expectInsideViewport } from './fixture'
export function registerSecuritySettingsTests(reflow = false) {
 for (const locale of ['en','zh-CN'] as const) test(`platform owns authentication and risk configuration (${locale})`,async({page},info)=>{
  const zh=locale==='zh-CN';await installLayoutApi(page,locale)
  const errors:string[]=[],reads:string[]=[],writes:{body:string|null;key:string|undefined}[]=[]
  page.on('pageerror',error=>errors.push(error.message))
  let auth={version:3,captcha_enabled:false,telegram_enabled:false,telegram_client_id:'',privacy_policy_version:'terms-1',service_terms_version:'terms-1',audit_log_id:gameId}
  let policy={brand_id:gameId,version:1,config:{age_enabled:false,minimum_age:null as number|null,region_enabled:false,allowed_countries:[] as string[],identity_enabled:false,account_risk_enabled:false,betting_risk_enabled:false,exclusion_enabled:false,responsible_gambling_enabled:false},updated_at:'2026-10-21T00:00:00Z',audit_log_id:gameId}
  let lose=true,conflict=true
  const receipts=new Map<string,typeof auth>()
  await page.route('**/api/v1/platform/**',async route=>{
   const req=route.request(),path=new URL(req.url()).pathname
   const reply=(data:unknown)=>route.fulfill({json:{success:true,data}})
   if(req.method()==='GET'){
    reads.push(path)
    if(path.endsWith('/presentation'))return reply({display_name:'Platform',logo_text:'P',logo_url:''})
    if(path.endsWith('/me'))return reply({account:{id:gameId,username:'security_reviewer',super_admin:true,platform_permissions:['auth_config.view.platform','auth_config.write.platform','compliance_policy.view.platform','compliance_policy.write.platform']}})
    if(path.endsWith('/platform-settings'))return reply({version:1,display_name:'Platform',logo_text:'P',logo_url:'',timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss',updated_at:'2026-10-21T00:00:00Z'})
    if(path.endsWith('/brands'))return reply({items:[{id:gameId,code:'aurora',name:'Aurora',status:'active'}]})
    expect(req.headers()['x-brand-id']).toBe(gameId)
    if(path.endsWith('/auth-settings'))return reply(auth)
    if(path.endsWith('/compliance-policy'))return reply(policy)
    throw Error(`Unexpected security read: ${path}`)
   }
   expect(req.headers()['x-brand-id']).toBe(gameId)
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
  await page.locator('.sidebar').getByRole('button',{name:zh?'运营':'Operations',exact:true}).click()
  await page.getByTestId('platform-operations-tabs').getByRole('button',{name:zh?'认证与风控':'Authentication & risk',exact:true}).click()
  await page.locator('.topbar-brand-picker select').selectOption(gameId)
  const panel=page.locator('.security-settings')
  const edit = panel.getByRole('button',{name:zh?'编辑设置':'Edit settings',exact:true})
  const editor = panel.getByRole('dialog',{name:zh?'认证设置':'Authentication',exact:true})
  await expect(panel.locator('form')).toHaveCount(0)
  await edit.click()
  await expect(editor).toBeVisible()
  await panel.locator('input[type=checkbox]').first().check()
  await page.keyboard.press('Escape')
  await expect(editor).toHaveCount(0)
  await edit.click()
  await expect(panel.locator('input[type=checkbox]').first()).not.toBeChecked()
  await panel.locator('input[type=checkbox]').first().check()
  await panel.locator('button[type=submit]').click()
  const dialog=page.getByRole('dialog',{name:zh?'确认设置变更':'Confirm changes',exact:true})
  expect(writes).toHaveLength(0)
  await page.keyboard.press('Escape');await expect(dialog).toHaveCount(0)
  await panel.locator('button[type=submit]').click();await dialog.locator('.button-primary').click()
  await expect(dialog.getByRole('button',{name:zh?'重试此操作':'Retry this operation',exact:true})).toBeEnabled()
  await page.keyboard.press('Escape')
  await panel.getByRole('button',{name:zh?'核对待处理变更':'Review pending change',exact:true}).click()
  await dialog.getByRole('button',{name:zh?'重试此操作':'Retry this operation',exact:true}).click()
  await expect(dialog).toHaveCount(0);expect(writes).toHaveLength(2);expect(writes[0]).toEqual(writes[1])
  await panel.getByRole('button',{name:zh?'风控与合规':'Risk & compliance',exact:true}).click()
  await edit.click()
  await panel.locator('input[type=checkbox]').first().check()
  await expect(panel.locator('input[type=number]')).toHaveValue('18')
  const preset=page.viewportSize()!
  for(const width of reflow?[320,359,360,361,699,700,701,900,1440]:[preset.width]){
   if(reflow)await page.setViewportSize({width,height:preset.height})
   await expectNoPageOverflow(page)
   await expectInsideViewport(panel.getByRole('dialog',{name:zh?'风控与合规':'Risk & compliance',exact:true}))
   await panel.locator('button[type=submit]').scrollIntoViewIfNeeded()
   await expectInsideViewport(panel.locator('button[type=submit]'))
  }
  await panel.locator('button[type=submit]').click();await dialog.locator('.button-primary').click()
  await expect(dialog.getByRole('button',{name:zh?'重新操作':'Start again',exact:true})).toBeEnabled()
  await dialog.getByRole('button',{name:zh?'重新操作':'Start again',exact:true}).click()
  await edit.click()
  await panel.locator('input[type=checkbox]').first().check();await panel.locator('button[type=submit]').click();await dialog.locator('.button-primary').click()
  await expect(dialog).toHaveCount(0);await expect(panel.getByRole('status')).toContainText(zh?'设置已保存':'Settings saved')
  expect(reads.some(path=>/history|gates|checks/.test(path))).toBe(false);expect(errors).toEqual([])
  await panel.screenshot({path:info.outputPath(`security-settings-${locale}.png`)})
 })
}
