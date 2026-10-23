import { expect, test } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { origins, gameId, expectNoPageOverflow } from './fixture'

const bootstrapPermissions = [...new Set(Array.from(
 readFileSync('backend/cmd/platform/admin.go', 'utf8')
  .matchAll(/"([a-z][a-z0-9_]*\.[a-z][a-z0-9_]*\.brand)"/g), match => match[1],
))]

export function registerAssociationNameTests() {
 for (const locale of ['en', 'zh-CN'] as const) {
  test(`brand role details do not break new account creation (${locale})`, async ({ page }, info) => {
   const zh=locale==='zh-CN', errors:string[]=[], role='bbbbbbbb-bbbb-4bbb-8bbb-000000000001', created='cccccccc-cccc-4ccc-8ccc-000000000001'
   const writes:{body:unknown;key:string|undefined}[]=[]
   page.on('pageerror',error=>errors.push(error.message))
   await page.context().addCookies([{name:'lottery-locale',value:locale,url:origins.platform}])
   await page.route('**/api/**',async route=>{
    const req=route.request(),path=new URL(req.url()).pathname
    const reply=(data:unknown,status=200)=>route.fulfill({status,json:{success:true,data,associations:{[`role:${role}`]:{name:'Bootstrap administrator',built_in:true},[`brand:${gameId}`]:{name:'Aurora'}}}})
    const display={timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss'}
    if(path==='/api/v1/platform/presentation')return reply({display_name:'Layout',logo_text:'L',logo_url:''})
    if(path==='/api/v1/display-settings')return reply(display)
    if(path==='/api/v1/platform/me')return reply({account:{id:gameId,username:'admin',super_admin:true,platform_permissions:['admin.view.platform','admin.write.platform','role.view.platform']}})
    if(path==='/api/v1/platform/platform-settings')return reply({...display,display_name:'Layout',logo_text:'L',logo_url:'',version:1,updated_at:'2026-10-18T00:00:00Z'})
    if(path==='/api/v1/platform/brands')return reply({items:[{id:gameId,name:'Aurora',code:'aurora',status:'active'}]})
    if(path==='/api/v1/platform/roles'){
     expect(req.method()).toBe('GET');expect(req.headers()['x-brand-id']).toBe(gameId)
     return reply({items:[{id:role,brand_id:gameId,name:'Bootstrap administrator',code:`bootstrap_${role}`,status:'active',version:1,is_bootstrap:true,permissions:bootstrapPermissions}]})
    }
    const account={id:created,username:'dialog_test',status:'active',version:1,super_admin:false,brand_ids:[gameId],role_ids:[role],role_codes:[`bootstrap_${role}`],audit_log_id:created}
    if(path==='/api/v1/platform/accounts'){
     expect(req.headers()['x-brand-id']).toBe(gameId)
     if(req.method()==='GET')return reply({items:writes.length?[account]:[]})
     expect(req.method()).toBe('POST');const body=req.postDataJSON()
     expect(body).toEqual({username:'dialog_test',password:'fixture-password-2026',role_ids:[role]})
     writes.push({body,key:req.headers()['idempotency-key']});expect(writes[0].key).toMatch(/^[0-9a-f-]{36}$/)
     return reply(account,201)
    }
    throw Error(`Unexpected creation request: ${req.method()} ${path}`)
   })
   await page.goto(origins.platform);await expect(page.locator('.app-frame')).toBeVisible()
   const menu=page.locator('button[aria-controls="platform-navigation"]')
   if(await menu.isVisible())await menu.click()
   if (await page.locator('button[aria-controls="platform-brand-tabs"]').getAttribute('aria-expanded') === 'false') await page.locator('button[aria-controls="platform-brand-tabs"]').click()
   await page.getByRole('button',{name:zh?'后台账号':'Back-office accounts',exact:true}).filter({visible:true}).click()
   await page.locator('.topbar-brand-picker select').selectOption(gameId)
   await page.getByRole('tab',{name:zh?'角色':'Roles',exact:true}).click()
   await page.getByTestId('platform-access-list').getByRole('button',{name:zh?'系统管理员':'System administrator',exact:true}).click()
   const details=page.getByRole('dialog')
   await expect(details).toContainText(zh?'会员 · 强制下线':'Members · Force sign out')
   await details.getByRole('button',{name:zh?'关闭':'Close',exact:true}).click()
   await page.getByRole('tab',{name:zh?'账号':'Accounts',exact:true}).click()
   const accountList=page.getByTestId('platform-access-list')
   const superAdminLabel=zh?'超级管理员':'Super admin'
   await expect(accountList.getByRole('columnheader',{name:superAdminLabel,exact:true})).toHaveCount(0)
   await expect(accountList.locator('td.empty-state')).toHaveAttribute('colspan','3')
   const newAccount=page.getByRole('button',{name:zh?'新建账号':'New account',exact:true})
   await newAccount.click()
   const form=page.getByRole('dialog',{name:zh?'新建账号':'New account',exact:true})
   await expect(form).toBeVisible()
   await form.getByRole('button',{name:zh?'取消':'Cancel',exact:true}).click()
   await expect(form).toHaveCount(0);await newAccount.click();await expect(form).toBeVisible()
   await form.locator('input[autocomplete="off"]').fill('dialog_test')
   await form.locator('input[type="password"]').fill('fixture-password-2026')
   await form.getByRole('checkbox').check()
   await page.screenshot({path:info.outputPath(`new-brand-account-${locale}.png`)})
   await form.getByRole('button',{name:zh?'保存':'Save',exact:true}).click()
   await expect(form).toHaveCount(0)
   await expect(page.getByTestId('platform-access-list')).toContainText('dialog_test')
   await expect(accountList.getByRole('columnheader')).toHaveCount(3)
   await expect(accountList.locator('tbody tr').first().getByRole('cell')).toHaveCount(3)
   await accountList.getByRole('button',{name:/dialog_test/}).click()
   await expect(page.getByTestId('platform-access-detail')).not.toContainText(superAdminLabel)
   expect(writes).toHaveLength(1);expect(errors).toEqual([])
   await expectNoPageOverflow(page)
  })
  test(`account roles show names without role-browsing permission (${locale})`, async ({ page }) => {
   await page.context().addCookies([{ name:'lottery-locale',value:locale,url:origins.platform }])
   const role = 'bbbbbbbb-bbbb-4bbb-8bbb-000000000001'
   const internalCode = `bootstrap_${role}`
   const account = { id:gameId,username:'named_admin',status:'active',version:1,super_admin:true,brand_ids:[],role_ids:[role],role_codes:[internalCode] }
   await page.route('**/api/**', async route => {
    expect(route.request().method()).toBe('GET')
    const path = new URL(route.request().url()).pathname
    const reply = (data:unknown) => route.fulfill({json:{success:true,data,associations:{[`role:${role}`]:{name:'Bootstrap administrator',built_in:true}}}})
    if(path==='/api/v1/platform/presentation')return reply({display_name:'Layout',logo_text:'L',logo_url:''})
    if(path==='/api/v1/display-settings')return reply({timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss'})
    if(path==='/api/v1/platform/me')return reply({account:{...account,platform_permissions:['admin.view.platform']}})
    if(path==='/api/v1/platform/platform-settings')return reply({timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss',display_name:'Layout',logo_text:'L',logo_url:'',version:1,updated_at:'2026-10-18T00:00:00Z'})
    if(path==='/api/v1/platform/brands')return reply({items:[]})
    if(path==='/api/v1/platform/platform-accounts')return reply({items:[account]})
    if(path===`/api/v1/platform/platform-accounts/${gameId}`)return reply(account)
    throw Error(`Unauthorized or unexpected association request: ${path}`)
   })
   await page.goto(origins.platform)
   await expect(page.locator('.app-frame')).toBeVisible()
   const mobileMenu=page.locator('button[aria-controls="platform-navigation"]')
   if(await mobileMenu.isVisible())await mobileMenu.click()
   await page.getByRole('button',{name:locale==='en'?'Admin accounts':'管理账号',exact:true}).filter({visible:true}).click()
   const list=page.getByTestId('platform-access-list')
   await expect(list).toContainText(locale==='en'?'System administrator':'系统管理员')
   await expect(list.getByRole('columnheader',{name:locale==='en'?'Super admin':'超级管理员',exact:true})).toHaveCount(0)
   await expect(list).not.toContainText(internalCode)
   await expect(list).not.toContainText(role)
   await list.getByRole('button',{name:/named_admin/}).click()
   const detail=page.getByTestId('platform-access-detail')
   await expect(detail).toContainText(locale==='en'?'System administrator':'系统管理员')
   await expect(detail).not.toContainText(locale==='en'?'Super admin':'超级管理员')
   await expect(detail).not.toContainText(gameId)
   await expect(detail).not.toContainText(role)
   await expectNoPageOverflow(page)
  })
 }
}
