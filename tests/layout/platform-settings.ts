import { expect, test } from '@playwright/test'
import { resolve } from 'node:path'
import { origins, expectInsideViewport, expectNoPageOverflow } from './fixture'

export function registerPlatformSettingsEditorTests(reflow = false) {
 for (const locale of ['en', 'zh-CN'] as const) {
  test(`platform settings edit in a modal (${locale})`, async ({ page }, info) => {
   const zh = locale === 'zh-CN', errors: string[] = [], writes: unknown[] = []
   page.on('pageerror', error => errors.push(error.message))
   const initial = {version: 3, display_name: 'Platform', logo_text: 'P', logo_url: '', timezone: 'Asia/Singapore', date_format: 'YYYY-MM-DD', time_format: 'HH:mm:ss', updated_at: '2026-10-21T00:00:00Z'}
   await page.route('**/api/**', async route => {
    const request = route.request()
    expect(new URL(request.url()).pathname).toBe('/api/v1/platform/platform-settings')
    expect(request.method()).toBe('PATCH')
    const body = request.postDataJSON()
    expect(request.headers()['idempotency-key']).toBeTruthy()
    expect(body.version).toBe(3)
    expect(body.display_name).toBe('Updated platform')
    writes.push(body)
    return route.fulfill({json: {success: true, data: {...initial, ...body, version: 4}}})
   })
   await page.route('**/__settings-editor', route => route.fulfill({contentType: 'text/html', body: `<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><body><div id="app" class="management-console"></div><script id="harness-data" type="application/json">${JSON.stringify({component:'PlatformSettingsPanel',props:{initial,locale},locale})}</script><script type="module" src="/@fs${resolve('platform-web/tests/layout-harness.ts')}"></script></body></html>`}))
   await page.goto(origins.platform + '/__settings-editor')
   const edit = page.getByRole('button', {name: zh ? '编辑设置' : 'Edit settings', exact: true})
   const dialog = page.getByRole('dialog', {name: zh ? '平台设置' : 'Platform settings', exact: true})
   await expect(page.locator('form')).toHaveCount(0)
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
    await expectNoPageOverflow(page)
   }
   await page.screenshot({path: info.outputPath(`platform-settings-editor-${locale}.png`)})
   await name.fill('Updated platform')
   await dialog.locator('button[type=submit], button:not([type])').last().click()
   await expect(dialog).toHaveCount(0)
   await expect(page.locator('.confirm-list')).toContainText('Updated platform')
   expect(writes).toHaveLength(1)
   await edit.click()
   await expect(name).toHaveValue('Updated platform')
   await dialog.getByRole('button', {name: zh ? '取消' : 'Cancel', exact: true}).click()
   await expect(dialog).toHaveCount(0)
   expect(errors).toEqual([])
  })
 }
}
