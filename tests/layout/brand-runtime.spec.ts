import { expect, test } from '@playwright/test'
import { gameId, origins, installLayoutApi, expectNoPageOverflow } from './fixture'

for (const locale of ['zh-CN', 'en'] as const) {
 test(`built brand console loads login, settings and audit without retired notification modules (${locale})`, async ({ page }, info) => {
  const zh = locale === 'zh-CN', at = '2026-10-27T00:00:00Z'
  const errors: string[] = [], failedAssets: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  page.on('response', response => { if(new URL(response.url()).pathname.startsWith('/assets/') && response.status() >= 400) failedAssets.push(response.url()) })
  await page.addInitScript(({locale}) => localStorage.setItem('lottery.admin.locale', locale), {locale})
  await installLayoutApi(page, locale)
  // Read-only synthetic sessions; no login or business writes reach a real service.
  let signedIn = false
  await page.route('**/api/v1/admin/**', route => {
   expect(route.request().method()).toBe('GET')
   const path = new URL(route.request().url()).pathname
   const reply = (data: unknown) => route.fulfill({json:{success:true,data}})
   if(path.endsWith('/me')) return signedIn
    ? reply({account:{id:gameId,username:'runtime_reviewer',super_admin:false,brand_ids:[gameId],permissions:[],permissions_by_brand:{[gameId]:['audit.view.brand','brand_operation.view.brand','notification_template.view.brand','notification_template.write.brand','notification.view.brand','notification.retry.brand']},platform_permissions:[]}})
    : route.fulfill({status:401,json:{success:false,error:{code:'UNAUTHORIZED',message:'Sign in required'}}})
   if(path.endsWith('/brands')) return reply({items:[{id:gameId,code:'layout',name:'Runtime trial',status:'active',allow_custom_games:true,member_count:'0',agent_count:'0',recharge_quota_available:'0',created_at:at}]})
   expect(route.request().headers()['x-brand-id']).toBe(gameId)
   if(path.endsWith('/brand-operation')) return reply({brand_id:gameId,version:1,name:'Runtime trial',status:'active',updated_at:at})
   if(path.endsWith('/filter-options')) return reply({brand_id:gameId,items:[],limit:100,offset:0})
   if(path.endsWith('/audit')) return reply({items:[{id:gameId,brand_id:gameId,action:'user.update',actor_type:'admin',actor_id:gameId,actor_username:'runtime_reviewer',resource_type:'user',resource_id:gameId,reason:'',request_id:'runtime-read',created_at:at,ip_address:'127.0.0.1',before_json:{display_name:'Before'},after_json:{display_name:'After'}}]})
   throw Error(`Unexpected runtime request: ${path}`)
  })
  await page.goto(origins.brand)
  await expect(page.locator('.login-entry__form')).toBeVisible()
  await expect(page.locator('.management-console')).toHaveCount(0)
  signedIn = true
  await page.reload()
  await expect(page.locator('.management-console')).toBeVisible()
  const navigate = async (name: string) => {
   const more = page.locator('.mobile-nav > button').last()
   const mobile = await more.isVisible()
   if(mobile && await more.getAttribute('aria-expanded') !== 'true') await more.click()
   const menu = page.locator(mobile ? '.mobile-more-menu' : '.side-nav')
   await expect(menu.getByRole('button',{name:zh?'通知投递':'Notification deliveries',exact:true})).toHaveCount(0)
   await menu.getByRole('button',{name,exact:true}).click()
   await expect(page.locator('.breadcrumbs b')).toHaveText(name)
  }
  await expect(page.locator('.top-actions .icon-button')).toHaveCount(0)
  await navigate(zh?'品牌设置':'Brand settings')
  await expect(page.locator('.brand-operation')).toBeVisible()
  await expect(page.locator('.console-tabs').getByRole('button',{name:zh?'通知模板':'Notification templates',exact:true})).toHaveCount(0)
  await navigate(zh?'审计日志':'Audit log')
  const table = page.locator('.audit-table')
  await expect(table).toContainText('runtime_reviewer')
  await table.getByRole('button').click()
  await expect(page.locator('.audit-snapshots')).toContainText('Before')
  const preset = page.viewportSize()!
  for(const width of info.project.name.endsWith('reflow') ? [320,700,900,1440] : [preset.width]) {
   if(info.project.name.endsWith('reflow')) await page.setViewportSize({width,height:preset.height})
   await expectNoPageOverflow(page)
  }
  await page.screenshot({path:info.outputPath(`brand-runtime-${locale}.png`),fullPage:true})
  expect(errors).toEqual([])
  expect(failedAssets).toEqual([])
 })
}
