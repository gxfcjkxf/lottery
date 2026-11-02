import { selectControl } from '../select-control'
import { expect, test } from '@playwright/test'
import {installConsoleSession,openPlatformPage} from './real-console-session'
import {expectRealEntry,expectListLayout} from './real-page-review'
import { origins, expectInsideViewport } from './fixture'

export function registerPlatformSettingsEditorTests(reflow = false) {
 for (const locale of ['en', 'zh-CN'] as const) {
  test(`platform settings edit in a modal (${locale})`, async ({ page }, info) => {
   const zh = locale === 'zh-CN', errors: string[] = [], writes: unknown[] = []
   page.on('pageerror', error => errors.push(error.message))
   const initial = {version: 3, display_name: 'Platform', logo_text: 'P', logo_url: '', timezone: 'Asia/Singapore', date_format: 'YYYY-MM-DD', time_format: 'HH:mm:ss', updated_at: '2026-10-21T00:00:00Z'}
   await page.route('**/api/**', async route => {
    const request = route.request()
    if (new URL(request.url()).pathname === '/api/v1/platform/commission-settings') {
     expect(request.method()).toBe('GET')
     return route.fulfill({json:{success:true,data:{version:1,payouts_enabled:false,config:{enabled:false,payout_mode:'manual',calendar:null}}}})
    }
    expect(new URL(request.url()).pathname).toBe('/api/v1/platform/platform-settings')
    expect(request.method()).toBe('PATCH')
    const body = request.postDataJSON()
    expect(request.headers()['idempotency-key']).toBeTruthy()
    expect(body.version).toBe(3)
    expect(body.display_name).toBe('Updated platform')
    expect(body.timezone).toBe('UTC')
    expect(body.date_format).toBe('DD/MM/YYYY')
    expect(body.time_format).toBe('hh:mm:ss A')
    writes.push(body)
    return route.fulfill({json: {success: true, data: {...initial, ...body, version: 4}}})
   })
   await installConsoleSession(page,'platform',locale,'aaaaaaaa-aaaa-4aaa-8aaa-000000000001',['platform_settings.write.platform'],'22222222-2222-4222-8222-222222222222',initial)
   await page.goto(origins.platform);await expect(page.locator('.management-console')).toBeVisible();await openPlatformPage(page,zh?'设置':'Settings');await expectRealEntry(page,'platform')
   const tabs = page.locator('.platform-settings-tabs')
   await expect(tabs.getByRole('button')).toHaveCount(3)
   await expect(tabs.getByRole('button', {name: zh ? '基础' : 'General', exact:true})).toHaveAttribute('aria-pressed', 'true')
   const edit = page.getByRole('button', {name: zh ? '编辑设置' : 'Edit settings', exact: true})
   const dialog = page.getByRole('dialog', {name: zh ? '设置' : 'Settings', exact: true})
   await expect(page.locator('.platform-settings-form')).toHaveCount(0)
   await edit.click()
   const name = dialog.getByLabel(zh ? '平台名称' : 'Platform name', {exact: true})
   await name.fill('Cancelled platform')
   await page.keyboard.press('Escape')
   await expect(dialog).toHaveCount(0)
   expect(writes).toHaveLength(0)
   await edit.click()
   await expect(name).toHaveValue('Platform')
   const preset = page.viewportSize()!
   for (const width of reflow ? [320, 640, 900, 1440] : [preset.width]) {
    if (reflow) await page.setViewportSize({width, height: preset.height})
    await expectInsideViewport(dialog)
    await expectListLayout(page)
   }
   await page.screenshot({path: info.outputPath(`platform-settings-editor-${locale}.png`)})
   await name.fill('Updated platform')
   const selectors = dialog.locator('.select-control-trigger')
   await selectors.nth(0).click()
   const popup = page.getByRole('listbox').filter({visible:true})
   await expectInsideViewport(popup)
   await page.screenshot({path: info.outputPath(`platform-settings-timezone-${locale}.png`)})
   await popup.locator('[data-value="UTC"]').click()
   await selectControl(selectors.nth(1), 'DD/MM/YYYY')
   await selectControl(selectors.nth(2), 'hh:mm:ss A')
   await dialog.locator('button[type=submit], button:not([type])').last().click()
   await expect(dialog).toHaveCount(0)
   await expect(page.locator('.confirm-list')).toContainText('Updated platform')
   expect(writes).toHaveLength(1)
   await edit.click()
   await expect(name).toHaveValue('Updated platform')
   await dialog.locator('.detail-modal-close').click()
   await expect(dialog).toHaveCount(0)
   await tabs.getByRole('button', {name: zh ? '佣金结算' : 'Commission settlement', exact:true}).click()
   await expect(page.locator('.commission-fields')).toBeVisible()
   await expect(page.locator('.confirm-list')).not.toBeVisible()
   await expectListLayout(page)
   await page.screenshot({path: info.outputPath(`platform-settings-commission-${locale}.png`)})
   await tabs.getByRole('button', {name: zh ? '基础' : 'General', exact:true}).click()
   await expect(page.locator('.confirm-list')).toContainText('Updated platform')
   expect(errors).toEqual([])
  })
 }
}

export function registerTemplateEditorTests(reflow = false) {
 for (const locale of ['en', 'zh-CN'] as const) test(`platform template editor aligns actions (${locale})`, async ({page}, info) => {
  const id='aaaaaaaa-aaaa-4aaa-8aaa-000000000001', at='2026-10-21T00:00:00Z'
  const errors:string[]=[];page.on('pageerror',cause=>errors.push(cause.message))
  let writes=0
  const item={key:'member.joined',version:1,content:{en:{title:'Welcome',body:'Hello'},'zh-CN':{title:'欢迎',body:'你好'}},updated_at:at,audit_log_id:id}
  await page.route('**/api/**',route=>{
   const req=route.request(),path=new URL(req.url()).pathname
   if(req.method()==='GET'){
    expect(path).toBe('/api/v1/platform/notification-templates')
    return route.fulfill({json:{success:true,data:{items:[item]}}})
   }
   expect(req.method()).toBe('PUT');expect(path).toBe('/api/v1/platform/notification-templates/member.joined')
   expect(req.headers()['idempotency-key']).toBeTruthy();expect(req.postDataJSON().version).toBe(1)
   writes++
   return route.fulfill({json:{success:true,data:{...item,content:req.postDataJSON().content,version:2}}})
  })
  await installConsoleSession(page,'platform',locale,id,['notification_template.view.platform','notification_template.write.platform'],id)
  await page.goto(origins.platform);await expect(page.locator('.management-console')).toBeVisible()
  await openPlatformPage(page,locale==='en'?'Operations':'运营');await openPlatformPage(page,locale==='en'?'Notification templates':'通知模板');await expectRealEntry(page,'platform')
  await page.locator('.template-table .row-action').click()
  const dialog=page.getByRole('dialog'),submit=dialog.locator('button[type=submit]'),preset=page.viewportSize()!
  for(const width of reflow?[320,640,900,1440]:[preset.width]){
   if(reflow)await page.setViewportSize({width,height:preset.height})
   await submit.scrollIntoViewIfNeeded();await expect(submit).toBeInViewport()
   await expectInsideViewport(dialog);await expectListLayout(page)
   await page.screenshot({path:info.outputPath(`template-actions-${locale}-${width}.png`)})
  }
  await dialog.locator('input').last().fill('Updated welcome')
  await submit.click();await expect.poll(()=>writes).toBe(1)
  await expect(submit).toBeEnabled();expect(errors).toEqual([])
 })
}
