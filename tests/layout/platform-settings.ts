import { selectControl } from '../select-control'
import { expect, test } from '@playwright/test'
import {installConsoleSession,openPlatformPage} from './real-console-session'
import {expectRealEntry,expectListLayout} from './real-page-review'
import { origins, expectInsideViewport } from './fixture'

export function registerPlatformSettingsEditorTests(reflow = false) {
 for (const locale of ['en', 'zh-CN'] as const) {
  test(`platform general settings save inline (${locale})`, async ({ page }, info) => {
   const zh = locale === 'zh-CN', errors: string[] = [], writes: { body: unknown; key: string | undefined }[] = []
   page.on('pageerror', error => errors.push(error.message))
   const initial = {theme:'jade',version: 3, display_name: 'Platform', logo_text: 'P', logo_url: '', timezone: 'Asia/Singapore', date_format: 'YYYY-MM-DD', time_format: 'HH:mm:ss', updated_at: '2026-10-21T00:00:00Z'}
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
    writes.push({ body, key: request.headers()['idempotency-key'] })
    if (writes.length === 1) return route.abort('failed')
    return route.fulfill({json: {success: true, data: {...initial, ...body, version: 4}}})
   })
   await installConsoleSession(page,'platform',locale,'aaaaaaaa-aaaa-4aaa-8aaa-000000000001',['platform_settings.view.platform','platform_settings.write.platform'],'22222222-2222-4222-8222-222222222222',initial)
   await page.goto(origins.platform);await expect(page.locator('.management-console')).toBeVisible();await openPlatformPage(page,zh?'设置':'Settings');await expectRealEntry(page,'platform')
   const tabs = page.locator('.platform-settings-tabs')
   await expect(tabs.getByRole('button')).toHaveCount(4)
   await expect(tabs.getByRole('button', {name: zh ? '基础' : 'General', exact:true})).toHaveAttribute('aria-pressed', 'true')
   const form = page.locator('.platform-settings-form')
   const name = form.getByLabel(zh ? '平台名称' : 'Platform name', {exact: true})
   const save = form.getByRole('button', {name: zh ? '保存设置' : 'Save settings', exact: true})
   await expect(name).toHaveValue('Platform')
   await expect(page.getByRole('dialog')).toHaveCount(0)
   const preset = page.viewportSize()!
   for (const width of reflow ? [320, 640, 900, 1440] : [preset.width]) {
    if (reflow) await page.setViewportSize({width, height: preset.height})
    await expectListLayout(page)
    await page.screenshot({path: info.outputPath(`platform-general-${locale}-${width}.png`)})
   }
   await name.fill('Updated platform')
   const selectors = form.locator('.select-control-trigger')
   await selectors.nth(1).click()
   const popup = page.getByRole('listbox').filter({visible:true})
   await expectInsideViewport(popup)
   await page.screenshot({path: info.outputPath(`platform-settings-timezone-${locale}.png`)})
   await popup.locator('[data-value="UTC"]').click()
   await selectControl(selectors.nth(2), 'DD/MM/YYYY')
   await selectControl(selectors.nth(3), 'hh:mm:ss A')
   await save.click()
   const retry = form.getByRole('button', {name: zh ? '重试' : 'Retry', exact: true})
   await expect(retry).toBeEnabled()
   await expect(name).toBeDisabled()
   expect(writes).toHaveLength(1)
   await retry.click()
   await expect(save).toBeEnabled()
   await expect(name).toHaveValue('Updated platform')
   expect(writes).toHaveLength(2)
   expect(writes[1]).toEqual(writes[0])
   await expect(page.getByRole('dialog')).toHaveCount(0)
   await tabs.getByRole('button', {name: zh ? '佣金结算' : 'Commission settlement', exact:true}).click()
   await expect(page.locator('.commission-fields')).toBeVisible()
   await expect(form).not.toBeVisible()
   await expectListLayout(page)
   await tabs.getByRole('button', {name: zh ? '基础' : 'General', exact:true}).click()
   await expect(name).toHaveValue('Updated platform')
   expect(errors).toEqual([])
  })
 }
}

export function registerTemplateEditorTests(reflow = false) {
 for (const locale of ['en', 'zh-CN'] as const) test(`platform template editor saves inline (${locale})`, async ({page}, info) => {
  const id='aaaaaaaa-aaaa-4aaa-8aaa-000000000001', at='2026-10-21T00:00:00Z'
  const errors:string[]=[];page.on('pageerror',cause=>errors.push(cause.message))
  const writes: { key: string | undefined; body: string | null }[] = []
  let item={key:'member.joined',version:1,content:{en:{title:'Welcome',body:'Hello'},'zh-CN':{title:'欢迎',body:'你好'}},updated_at:at,audit_log_id:id}
  await page.route('**/api/**',route=>{
   const req=route.request(),path=new URL(req.url()).pathname
   if(req.method()==='GET'){
    expect(path).toBe('/api/v1/platform/notification-templates')
    return route.fulfill({json:{success:true,data:{items:[item]}}})
   }
   expect(req.method()).toBe('PUT');expect(path).toBe('/api/v1/platform/notification-templates/member.joined')
   expect(req.headers()['x-brand-id']).toBeUndefined()
   const key=req.headers()['idempotency-key']
   expect(key).toBeTruthy();expect(req.postDataJSON().version).toBe(1)
   writes.push({ key, body: req.postData() })
   if (writes.length === 1) {
    item = {...item,content:req.postDataJSON().content,version:2}
    return route.abort('failed')
   }
   return route.fulfill({json:{success:true,data:item}})
  })
  await installConsoleSession(page,'platform',locale,id,['notification_template.view.platform','notification_template.write.platform'],id)
  await page.goto(origins.platform);await expect(page.locator('.management-console')).toBeVisible()
  const openTemplates = async () => {
   await openPlatformPage(page,locale==='en'?'Operations':'运营')
   await openPlatformPage(page,locale==='en'?'Notification templates':'通知模板')
  }
  await openTemplates();await expectRealEntry(page,'platform')
  const row=page.locator('.template-table .row-action')
  await row.click()
  const editor=page.getByTestId('platform-template-detail')
  const submit=editor.locator('button[type=submit]'),preset=page.viewportSize()!
  await expect(row).toHaveAttribute('aria-pressed','true')
  await expect(editor.getByRole('heading',{level:2})).toBeFocused()
  await expect(page.getByRole('dialog')).toHaveCount(0)
  for(const width of reflow?[320,640,900,1440]:[preset.width]){
   if(reflow)await page.setViewportSize({width,height:preset.height})
   await expectListLayout(page)
   const body = await editor.locator('textarea').first().evaluate(el => ({
    height: el.getBoundingClientRect().height,
    lineHeight: parseFloat(getComputedStyle(el).lineHeight),
   }))
   expect(body.height).toBeGreaterThanOrEqual(body.lineHeight * 3)
   await page.screenshot({path:info.outputPath(`template-actions-${locale}-${width}.png`)})
   await submit.scrollIntoViewIfNeeded();await expect(submit).toBeInViewport()
  }
  const title=editor.locator('input').last()
  await title.fill('Updated welcome')
  await submit.click()
  const retry = editor.getByRole('button',{name:locale==='en'?'Retry':'重试',exact:true})
  await expect(retry).toBeEnabled()
  await expect(title).toBeDisabled()
  await expect(row).toBeDisabled()
  expect(writes).toHaveLength(1)
  await openPlatformPage(page,locale==='en'?'Home':'首页')
  await openTemplates()
  await expect(title).toHaveValue('Updated welcome')
  await retry.click()
  await expect(title).toBeEnabled()
  await expect(row).toHaveAttribute('aria-pressed','true')
  await expect(page.getByRole('dialog')).toHaveCount(0)
  expect(writes).toHaveLength(2)
  expect(writes[1]).toEqual(writes[0])
  expect(errors).toEqual([])
 })
}
