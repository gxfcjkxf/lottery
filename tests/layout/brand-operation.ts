import { expect, test } from '@playwright/test'
import { origins, gameId, openPlatformBrandList, expectNoPageOverflow, expectInsideViewport } from './fixture'

export function registerBrandOperationLayoutTests(reflow = false) {
  for (const locale of ['en', 'zh-CN'] as const) {
    test(`brand status schedules, stopped state and original-request retry (${locale})`, async ({ page }, info) => {
      await page.context().addCookies([{ name: 'lottery-locale', value: locale, url: origins.platform }])
      const settings = { timezone: 'Asia/Singapore', date_format: 'YYYY-MM-DD', time_format: 'HH:mm:ss' }
      let record: Record<string, unknown> = { brand_id: gameId, name: 'Aurora', version: 2, status: 'active', updated_at: '2026-10-18T00:00:00Z' }
      const attempts: { body: string | null; key: string | undefined }[] = []
      let loseReceipt = true
      await page.route('**/api/**', async route => {
        const req = route.request(), path = new URL(req.url()).pathname
        const reply = (data: unknown) => route.fulfill({ json: { success: true, data } })
        if (path === '/api/v1/platform/presentation') return reply({ display_name: 'Layout', logo_text: 'L', logo_url: '' })
        if (path === '/api/v1/display-settings') return reply(settings)
        if (path === '/api/v1/platform/me') return reply({ account: { id: gameId, username: 'layout', super_admin: true, platform_permissions: ['brand_operation.view.platform', 'brand_operation.write.platform'] } })
        if (path === '/api/v1/platform/platform-settings') return reply({ ...settings, display_name: 'Layout', logo_text: 'L', logo_url: '', version: 1, updated_at: '2026-10-18T00:00:00Z' })
        if (path === '/api/v1/platform/brands') return reply({ items: [{ id: gameId, name: 'Aurora', code: 'aurora', status: record.status }] })
        if (path === '/api/v1/platform/brand-operation') {
          expect(req.headers()['x-brand-id']).toBe(gameId)
          if (req.method() === 'GET') return reply(record)
          expect(req.method()).toBe('PATCH')
          const body = req.postDataJSON()
          attempts.push({ body: req.postData(), key: req.headers()['idempotency-key'] })
          if (attempts.length === 1 || body.status === 'disabled' || body.status === 'active') {
            record = { ...record, version: body.version + 1, status: body.pause_starts_at ? 'active' : body.status, pause_starts_at: body.pause_starts_at, pause_ends_at: body.pause_ends_at, audit_log_id: gameId }
          }
          if (loseReceipt) { loseReceipt = false; return route.abort('failed') }
          return reply(record)
        }
        throw new Error(`Unconfigured brand-operation request: ${req.method()} ${path}`)
      })
      await page.goto(origins.platform)
      await expect(page.locator('.management-console')).toBeVisible()
      if (await page.locator('button[aria-controls="platform-navigation"]').isVisible()) {
        const navigation = page.locator('.platform-mobile-nav')
        await expect(navigation.locator('button')).toHaveCount(4)
        await expectInsideViewport(navigation)
        const menu = navigation.getByRole('button', { name: locale === 'en' ? 'Menu' : '菜单', exact: true })
        await menu.click()
        const directory = page.locator('#platform-navigation')
        await expectInsideViewport(directory)
        const settings = directory.locator('.nav-item').last()
        await settings.scrollIntoViewIfNeeded()
        await expectInsideViewport(settings)
        expect((await settings.boundingBox())!.height).toBeGreaterThanOrEqual(44)
        await expectNoPageOverflow(page)
        await page.screenshot({ path: info.outputPath(`platform-menu-${locale}.png`) })
        await menu.click()
        await expect(directory).toBeHidden()
        await page.screenshot({ path: info.outputPath(`platform-directory-${locale}.png`) })
      }
      await openPlatformBrandList(page, locale)
      const manage = page.getByRole('button', { name: locale === 'en' ? 'Manage status' : '管理运行状态', exact: true })
      await manage.click()
      const dialog = page.getByRole('dialog', { name: locale === 'en' ? 'Brand operating status' : '品牌运行状态', exact: true })
      await expect(dialog).toBeVisible()
      await expect(dialog).not.toContainText('submit()')
      await expect(dialog).not.toContainText('Asia/Singapore')
      await expect(dialog).not.toContainText('v2')
      await expect(dialog).not.toContainText(locale === 'en' ? 'Configuration version' : '配置版本')
      const confirm = dialog.getByRole('button', { name: locale === 'en' ? 'Confirm status change' : '确认变更状态', exact: true })
      await expect(confirm).toBeEnabled()
      await expect(dialog.getByRole('textbox')).toHaveCount(0)
      const selects = dialog.locator('.brand-operation-options select')
      await selects.nth(1).selectOption('true')
      const controls = dialog.locator('.platform-temporal-input')
      await expect(controls).toHaveCount(2)
      await expect(controls.first().locator('select').first()).toHaveCSS('background-repeat', 'no-repeat')
      await expect(controls.first().locator('select').first()).toHaveCSS('background-size', '20px 20px')
      // Choose dates and times through the actual shared controls.
      for (const [i, hour] of [[0, '12'], [1, '14']] as const) {
        await controls.nth(i).locator('.platform-date-input-button').click()
        const calendar = dialog.locator('.platform-calendar:popover-open')
        await calendar.locator('.platform-calendar-heading button').last().click()
        await calendar.locator('.platform-calendar-grid button').first().click()
        await controls.nth(i).locator('.platform-time-input select').nth(0).selectOption(hour)
        await controls.nth(i).locator('.platform-time-input select').nth(1).selectOption('00')
      }
      if (reflow) {
        for (const width of [320, 360, 412, 619, 620, 621, 768, 1024, 1440]) {
          await page.setViewportSize({ width, height: 600 })
          await expectNoPageOverflow(page)
          await expectInsideViewport(dialog)
        }
        await page.setViewportSize({ width: 320, height: 480 })
      }
      await expectNoPageOverflow(page)
      await expectInsideViewport(dialog)
      await page.screenshot({ path: info.outputPath(`brand-operation-${locale}.png`) })
      await confirm.click()
      const retry = dialog.getByRole('button', { name: locale === 'en' ? 'Retry the same request' : '使用原请求重试', exact: true })
      await expect(retry).toBeVisible()
      expect(JSON.parse(attempts[0].body!).reason).toBe('')
      await expect(dialog.locator('form')).toHaveCount(0)
      await retry.click()
      await expect(dialog).toHaveCount(0)
      expect(attempts[0]).toEqual(attempts[1])
      const scheduled = JSON.parse(attempts[0].body!)
      expect(scheduled).toMatchObject({ version: 2, status: 'paused', reason: '' })
      expect(Date.parse(scheduled.pause_ends_at) - Date.parse(scheduled.pause_starts_at)).toBe(7200000)
      await manage.click()
      await expect(dialog).toContainText(locale === 'en' ? 'Scheduled pause' : '预约暂停时间')
      await selects.first().selectOption('disabled')
      await confirm.click()
      await expect(dialog).toHaveCount(0)
      expect(JSON.parse(attempts.at(-1)!.body!)).toMatchObject({ status: 'disabled', reason: '' })
      await manage.click()
      await expect(dialog).toContainText(locale === 'en' ? 'Stopped' : '已停止')
      await expect(selects.first()).toHaveValue('active')
      await confirm.click()
      await expect(dialog).toHaveCount(0)
      await expectNoPageOverflow(page)
    })
  }
}
