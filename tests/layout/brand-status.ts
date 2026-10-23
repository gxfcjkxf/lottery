import { expect, test } from '@playwright/test'
import { gameId, origins, installLayoutApi, expectInsideViewport, expectNoPageOverflow } from './fixture'

export function registerBrandStatusTests(reflow = false) {
  for (const locale of ['en', 'zh-CN'] as const) {
    test(`brand status actions confirm and preserve original writes (${locale})`, async ({ page }, info) => {
      const zh = locale === 'zh-CN'
      await installLayoutApi(page, locale)
      const errors: string[] = []
      page.on('pageerror', cause => errors.push(cause.message))
      let record = { brand_id: gameId, version: 4, name: 'Aurora', status: 'active', updated_at: '2026-10-21T00:00:00Z', audit_log_id: gameId }
      let loseReceipt = true, conflict = false, reads = 0
      const attempts: { body: string | null; key: string | undefined }[] = []
      const receipts = new Map<string, typeof record>()
      await page.route('**/api/v1/admin/**', async route => {
        const request = route.request(), path = new URL(request.url()).pathname
        const reply = (data: unknown) => route.fulfill({ json: { success: true, data } })
        if (path.endsWith('/brand-operation')) {
          expect(request.headers()['x-brand-id']).toBe(gameId)
          if (request.method() === 'GET') { reads++; return reply(record) }
          expect(request.method()).toBe('PATCH')
          const key = request.headers()['idempotency-key'], body = request.postDataJSON()
          attempts.push({ body: request.postData(), key })
          if (receipts.has(key)) return reply(receipts.get(key))
          if (conflict) {
            conflict = false
            record = { ...record, status: 'paused', version: record.version + 1 }
            return route.fulfill({ status: 409, json: { success: false, error: { code: 'CONFLICT', message: 'Synthetic concurrent change' } } })
          }
          expect(body.version).toBe(record.version)
          expect(body.reason).toBe('')
          record = { ...record, status: body.status, version: body.version + 1 }
          receipts.set(key, record)
          if (loseReceipt) { loseReceipt = false; return route.abort('failed') }
          return reply(record)
        }
        expect(request.method()).toBe('GET')
        if (path.endsWith('/me')) return reply({ account: { id: gameId, username: 'status_reviewer', super_admin: false, brand_ids: [gameId], permissions: [], permissions_by_brand: { [gameId]: ['brand_operation.view.brand', 'brand_operation.write.brand'] }, platform_permissions: [] } })
        if (path.endsWith('/brands')) return reply({ items: [{ id: gameId, name: 'Aurora', code: 'aurora', status: 'active' }] })
        throw Error(`Unexpected status request: ${path}`)
      })
      await page.goto(origins.brand)
      await expect(page.locator('.management-console')).toBeVisible()
      if (await page.locator('.mobile-nav').isVisible()) {
        await page.locator('.mobile-nav button').last().click()
        await page.locator('.mobile-more-menu').getByRole('button', { name: zh ? '品牌设置' : 'Brand settings', exact: true }).click()
      } else await page.locator('.side-nav').getByRole('button', { name: zh ? '品牌设置' : 'Brand settings', exact: true }).click()
      const card = page.locator('.brand-operation')
      const pause = card.getByRole('button', { name: zh ? '暂停' : 'Pause', exact: true })
      const enable = card.getByRole('button', { name: zh ? '启用' : 'Enable', exact: true })
      const dialog = page.getByRole('dialog', { name: zh ? '确认状态变更' : 'Confirm status change', exact: true })
      const confirm = dialog.getByRole('button', { name: zh ? '提交' : 'Submit', exact: true })
      await expect(pause).toBeEnabled()
      await expect(card.locator('select, .brand-operation__history, .refresh-icon-control')).toHaveCount(0)
      await expect(card.getByRole('button')).toHaveCount(1)
      await pause.click()
      await expect(dialog).toBeVisible()
      expect(attempts).toHaveLength(0)
      await page.keyboard.press('Escape')
      await expect(dialog).toHaveCount(0)
      expect(attempts).toHaveLength(0)
      await pause.click()
      const preset = page.viewportSize()!
      for (const width of reflow ? [320, 640, 900, 1440] : [preset.width]) {
        if (reflow) await page.setViewportSize({ width, height: preset.height })
        await expectInsideViewport(dialog)
        await expectNoPageOverflow(page)
      }
      await page.screenshot({ path: info.outputPath(`brand-status-confirm-${locale}.png`) })
      await confirm.click()
      const retryAction = dialog.locator('.detail-modal-footer .button-primary')
      await expect(retryAction).toBeEnabled()
      await expect(dialog.getByRole('alert')).toBeVisible()
      expect(attempts).toHaveLength(1)
      await page.keyboard.press('Escape')
      await expect(dialog).toHaveCount(0)
      await expect(card.getByRole('button', { name: zh ? '核对待处理变更' : 'Review pending change', exact: true })).toBeVisible()
      await card.getByRole('button', { name: zh ? '核对待处理变更' : 'Review pending change', exact: true }).click()
      await retryAction.click()
      await expect(dialog).toHaveCount(0)
      expect(attempts).toHaveLength(2)
      expect(attempts[0]).toEqual(attempts[1])
      await expect(enable).toBeEnabled()
      await enable.click()
      await confirm.click()
      await expect(dialog).toHaveCount(0)
      await expect(pause).toBeEnabled()
      expect(attempts).toHaveLength(3)
      expect(attempts[2].key).not.toBe(attempts[0].key)
      conflict = true
      await pause.click()
      await confirm.click()
      const conflictAction = dialog.locator('.detail-modal-footer button')
      await expect(conflictAction).toBeEnabled()
      await expect(dialog.getByRole('alert')).toBeVisible()
      await conflictAction.click()
      await expect(dialog).toHaveCount(0)
      await expect(enable).toBeEnabled()
      expect(attempts).toHaveLength(4)
      expect(reads).toBe(4)
      await page.screenshot({ path: info.outputPath(`brand-status-page-${locale}.png`) })
      await expectNoPageOverflow(page)
      expect(errors).toEqual([])
    })
  }
}
