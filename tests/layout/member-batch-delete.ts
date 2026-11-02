import { test, expect } from '@playwright/test'
import { gameId, origins, installLayoutApi, openPlatformBrandList, expectInsideViewport, expectNoPageOverflow } from './fixture'

export function registerMemberBatchDeleteTests(reflow = false) {
  for (const app of ['brand', 'platform'] as const) for (const locale of ['en', 'zh-CN'] as const) {
    test(`${app} batch member deletion preserves retries and reports partial results (${locale})`, async ({ page }, info) => {
      const zh = locale === 'zh-CN'
      await installLayoutApi(page, locale)
      let allowed = true
      let members = Array.from({ length: 3 }, (_, i) => ({
        id: `bbbbbbbb-bbbb-4bbb-8bbb-${String(i + 1).padStart(12, '0')}`,
        global_user_id: `cccccccc-cccc-4ccc-8ccc-${String(i + 1).padStart(12, '0')}`,
        brand_id: gameId, username: `batch_member_${i + 1}`, phone: '', display_name: '', notes: '',
        tags: [], status: 'normal', online: false, display_points: null, joined_at: '2026-10-25T00:00:00Z',
      }))
      const source = [...members]
      const attempts: { name: string; key: string; body: unknown }[] = []
      const base = `/api/v1/${app === 'brand' ? 'admin' : 'platform'}`
      await page.route(`**${base}/**`, async route => {
        const req = route.request(), url = new URL(req.url()), path = url.pathname
        const reply = (data: unknown) => route.fulfill({ json: { success: true, data } })
        if (path === '/api/v1/platform/platform-settings') return reply({ display_name: 'Layout trial', logo_text: 'L', logo_url: '', timezone: 'Asia/Singapore', date_format: 'YYYY-MM-DD', time_format: 'HH:mm:ss', version: 1, updated_at: '2026-10-25T00:00:00Z' })
        if (path === `${base}/me`) {
          const grants = app === 'brand' ? ['user.view.brand', ...(allowed ? ['user.delete.brand'] : [])] : ['user.view.platform', ...(allowed ? ['user.delete.platform'] : [])]
          return reply({ account: { id: gameId, username: 'batch_reviewer', super_admin: app === 'platform', brand_ids: [gameId], permissions_by_brand: { [gameId]: grants }, platform_permissions: grants } })
        }
        if (path === `${base}/brands`) return reply({ items: [{ id: gameId, name: 'Aurora', code: 'aurora', status: 'active', member_count: '3', agent_count: '0', recharge_quota_available: null, created_at: '2026-10-25T00:00:00Z' }] })
        if (path === `${base}/users`) {
          const limit = Number(url.searchParams.get('limit')), offset = Number(url.searchParams.get('offset'))
          const filtered = members.filter(member => !url.searchParams.get('q') || member.username.includes(url.searchParams.get('q')!))
          return reply({ items: filtered.slice(offset, offset + limit), limit, offset, total_count: String(filtered.length) })
        }
        if (path.endsWith('/delete')) {
          const member = source.find(member => path.includes(app === 'brand' ? member.id : member.global_user_id))!
          expect(member).toBeTruthy()
          expect(req.method()).toBe('POST')
          expect(req.headers()['x-brand-id']).toBe(app === 'brand' ? gameId : undefined)
          expect(req.postDataJSON()).toEqual(app === 'brand' ? {} : { brand_id: gameId })
          attempts.push({ name: member.username, key: req.headers()['idempotency-key']!, body: req.postDataJSON() })
          const count = attempts.filter(attempt => attempt.name === member.username).length
          if (member === source[1] && count === 1) {
            members = members.filter(item => item.id !== member.id)
            return route.abort('failed')
          }
          if (member === source[2] && count === 1) return route.fulfill({ status: 403, json: { success: false, error: { code: 'PERMISSION_DENIED', message: 'Permission denied' } } })
          members = members.filter(item => item.id !== member.id)
          return reply(app === 'brand' ? { audit_log_id: gameId } : { id: member.global_user_id, account_name: member.username, audit_log_id: gameId })
        }
        return route.fallback()
      })
      async function openMembers() {
        await page.goto(origins[app])
        if (app === 'platform') {
          await openPlatformBrandList(page, locale)
          await page.getByTestId('platform-brand-list').locator('.member-menu-trigger').first().click()
          await page.getByRole('menuitem', { name: zh ? '会员' : 'Members', exact: true }).click()
        } else {
          await expect(page.locator('.management-console')).toBeVisible()
          if (await page.locator('.mobile-nav').isVisible()) await page.locator('.mobile-nav button').nth(1).click()
          else await page.locator('.side-nav').getByRole('button', { name: zh ? '会员管理' : 'Member management', exact: true }).click()
        }
        await expect(page.getByRole('button', { name: 'batch_member_1', exact: true }).first()).toBeVisible()
      }
      await openMembers()
      const toolbar = page.locator('.member-batch-toolbar')
      const remove = toolbar.getByRole('button', { name: zh ? '批量删除' : 'Delete selected', exact: true })
      await expect(remove).toBeDisabled()
      await page.getByRole('checkbox', { name: zh ? '选择账号 batch_member_1' : 'Select batch_member_1', exact: true }).check()
      await expect(toolbar.getByRole('status')).toHaveText(zh ? '已选 1 个' : '1 selected')
      await toolbar.getByRole('checkbox').check()
      await expect(toolbar.getByRole('status')).toHaveText(zh ? '已选 3 个' : '3 selected')
      await expectNoPageOverflow(page)
      await page.screenshot({ path: info.outputPath(`batch-selection-${app}-${locale}.png`) })
      if (reflow) {
        const original = page.viewportSize()!
        await page.setViewportSize({ width: 320, height: original.height })
        await expectNoPageOverflow(page)
        await expectInsideViewport(toolbar)
        await page.screenshot({ path: info.outputPath(`batch-selection-320-${app}-${locale}.png`) })
        await page.setViewportSize(original)
      }
      await remove.click()
      const dialog = page.getByRole('dialog', { name: zh ? '批量删除账号' : 'Delete accounts', exact: true })
      await expect(dialog.locator('li')).toHaveCount(3)
      await expectInsideViewport(dialog)
      await dialog.locator('.detail-modal-close').click()
      expect(attempts).toHaveLength(0)
      await remove.click()
      await dialog.getByRole('button', { name: zh ? '删除 3 个账号' : 'Delete 3 accounts', exact: true }).click()
      await expect(dialog).toContainText(zh ? '已删除 1 个 · 未完成 2 个' : '1 deleted · 2 unfinished')
      await expect(dialog.locator('.detail-modal-close')).toBeDisabled()
      await page.keyboard.press('Escape')
      await expect(dialog).toBeVisible()
      const viewport = page.viewportSize()!
      for (const width of reflow ? [...Array.from({ length: 9 }, (_, i) => 320 + i * 20), ...[620, 640, 700, 768, 820, 900, 1000, 1080, 1200, 1440].flatMap(width => [width - 1, width, width + 1]), 1024] : [viewport.width]) {
        if (reflow) await page.setViewportSize({ width, height: viewport.height })
        await expectInsideViewport(dialog)
        await expectNoPageOverflow(page)
        if (reflow && width === 320) await page.screenshot({ path: info.outputPath(`batch-dialog-320-${app}-${locale}.png`) })
      }
      await page.screenshot({ path: info.outputPath(`batch-delete-${app}-${locale}.png`) })
      if (reflow) await page.setViewportSize(viewport)
      const retry = dialog.getByRole('button', { name: zh ? '重试未完成项' : 'Retry unfinished', exact: true })
      await retry.click()
      await expect(dialog).toContainText(zh ? '已删除 2 个 · 未完成 1 个' : '2 deleted · 1 unfinished')
      await expect(dialog).toContainText(zh ? '删除失败' : 'Failed')
      await retry.click()
      await expect(app === 'brand' ? page.locator('.operation-toast-stack') : dialog).toContainText(zh ? '已删除 3 个 · 未完成 0 个' : '3 deleted · 0 unfinished')
      expect(attempts.filter(attempt => attempt.name === source[0]!.username)).toHaveLength(1)
      const uncertainAttempts = attempts.filter(attempt => attempt.name === source[1]!.username)
      expect(uncertainAttempts).toHaveLength(2)
      expect(uncertainAttempts[1]).toEqual(uncertainAttempts[0])
      const failedAttempts = attempts.filter(attempt => attempt.name === source[2]!.username)
      expect(failedAttempts).toHaveLength(2)
      expect(failedAttempts[1]!.key).not.toBe(failedAttempts[0]!.key)
      if (app === 'platform') await dialog.locator('.detail-modal-close').click()
      await expect(dialog).toHaveCount(0)
      await expect(page.locator('.member-batch-toolbar')).toHaveCount(0)
      allowed = false; members = [...source]
      await openMembers()
      await expect(page.locator('.member-batch-toolbar')).toHaveCount(0)
      await expect(page.locator('.member-account-select input')).toHaveCount(0)
    })
  }
}
