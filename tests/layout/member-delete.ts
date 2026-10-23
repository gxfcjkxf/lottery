import { expect, test } from '@playwright/test'
import { origins, gameId, openPlatformBrandList, expectInsideViewport, expectNoPageOverflow } from './fixture'

export function registerMemberDeleteTests(reflow = false) {
  for (const locale of ['en', 'zh-CN'] as const) {
    test(`platform deletes the global account with a named receipt and exact retry (${locale})`, async ({ page }, info) => {
      await page.context().addCookies([{ name: 'lottery-locale', value: locale, url: origins.platform }])
      const userId = 'bbbbbbbb-bbbb-4bbb-8bbb-000000000001'
      const member = { id: gameId, global_user_id: userId, brand_id: gameId, username: 'test001', phone: '', display_name: '', notes: '', status: 'normal', joined_at: '2026-10-18T00:00:00Z', tags: [], online: false, display_points: null }
      let deleted = false, allowed = true
      const attempts: { body: string | null; key?: string }[] = []
      await page.route('**/api/**', async route => {
        const req = route.request(), path = new URL(req.url()).pathname
        const reply = (data: unknown) => route.fulfill({ json: { success: true, data } })
        const settings = { timezone: 'Asia/Singapore', date_format: 'YYYY-MM-DD', time_format: 'HH:mm:ss' }
        if (path === '/api/v1/platform/presentation') return reply({ display_name: 'Layout', logo_text: 'L', logo_url: '' })
        if (path === '/api/v1/display-settings') return reply(settings)
        if (path === '/api/v1/platform/me') return reply({ account: { id: gameId, username: 'layout', super_admin: true, platform_permissions: allowed ? ['user.view.platform', 'user.delete.platform'] : ['user.view.platform'] } })
        if (path === '/api/v1/platform/platform-settings') return reply({ ...settings, display_name: 'Layout', logo_text: 'L', logo_url: '', version: 1, updated_at: '2026-10-18T00:00:00Z' })
        if (path === '/api/v1/platform/brands') return reply({ items: [{ id: gameId, name: 'Aurora', code: 'aurora', status: 'active' }] })
        if (path === '/api/v1/platform/users') { const url = new URL(req.url()); return reply({ items: deleted ? [] : [member], limit: Number(url.searchParams.get('limit')), offset: Number(url.searchParams.get('offset')), total_count: deleted ? '0' : '1' }) }
        if (path === `/api/v1/platform/global-users/${userId}/delete`) {
          expect(req.method()).toBe('POST')
          expect(req.headers()['x-brand-id']).toBeUndefined()
          attempts.push({ body: req.postData(), key: req.headers()['idempotency-key'] })
          deleted = true
          if (attempts.length === 1) return route.abort('failed')
          return reply({ id: userId, account_name: 'test001', audit_log_id: gameId })
        }
        throw new Error(`Unexpected deletion request: ${req.method()} ${path}`)
      })
      await page.goto(origins.platform)
      await openPlatformBrandList(page, locale)
      await page.getByRole('button', { name: locale === 'en' ? 'View members' : '查看会员', exact: true }).filter({ visible: true }).click()
      const label = locale === 'en' ? 'Delete member account' : '删除会员账号'
      await page.locator('.member-menu-trigger').filter({ visible: true }).click()
      await page.getByRole('menuitem', { name: label, exact: true }).click()
      const dialog = page.getByRole('dialog', { name: label, exact: true })
      await expect(dialog).toContainText(locale === 'en' ? 'all brands' : '所有品牌')
      await expect(dialog).toContainText('test001')
      if (reflow) {
        for (const width of [320, 480, 768, 1440]) {
          await page.setViewportSize({ width, height: 600 })
          await expectInsideViewport(dialog)
          await expectNoPageOverflow(page)
        }
      }
      await expectInsideViewport(dialog)
      await page.screenshot({ path: info.outputPath(`global-account-delete-${locale}.png`) })
      await dialog.getByRole('button', { name: label, exact: true }).click()
      await expect(page.locator('.toast')).toHaveCount(0)
      const retry = dialog.getByRole('button', { name: locale === 'en' ? 'Retry the same deletion' : '重试原删除请求', exact: true })
      await expect(retry).toBeVisible()
      await retry.click()
      await expect(dialog).toHaveCount(0)
      await expect(page.locator('.toast-message')).toHaveText(locale === 'en' ? 'Account “test001” deleted successfully.' : '删除账号「test001」成功。')
      expect(attempts).toHaveLength(2)
      expect(attempts[1]).toEqual(attempts[0])
      expect(attempts[0].body).toBe('{}')
      await page.screenshot({ path: info.outputPath(`global-account-deleted-${locale}.png`) })
      // A read-only platform account must not receive a deletion entry.
      allowed = false; deleted = false
      await page.reload()
      await openPlatformBrandList(page, locale)
      await page.getByRole('button', { name: locale === 'en' ? 'View members' : '查看会员', exact: true }).filter({ visible: true }).click()
      await expect(page.getByRole('button', { name: 'test001', exact: true }).first()).toBeVisible()
      await page.locator('.member-menu-trigger').filter({ visible: true }).click()
      await expect(page.getByRole('menu', { name: /test001/ }).filter({ visible: true })).toBeVisible()
      await expect(page.getByRole('menuitem', { name: label, exact: true })).toHaveCount(0)
    })
  }
}
