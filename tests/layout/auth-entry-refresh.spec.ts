import { test, expect } from '@playwright/test'
import { installLayoutApi, origins, expectNoPageOverflow } from './fixture'

for (const locale of ['en', 'zh-CN'] as const) {
  test(`auth refresh has no announcement requests or inline configuration loader (${locale})`, async ({ page }, info) => {
    await installLayoutApi(page, locale)
    await page.route('**/api/v1/me', route => route.fulfill({ status: 401, json: { success: false, error: { code: 'UNAUTHORIZED', message: 'Sign in required' } } }))
    let announcementRequests = 0
    await page.route('**/api/v1/announcements', route => {
      announcementRequests++
      return route.fulfill({ status: 500, json: { success: false, error: { code: 'INTERNAL_ERROR', message: 'Unavailable' } } })
    })
    for (const path of ['/login', '/register']) {
      await page.goto(`${origins.user}${path}`)
      await page.reload()
      await expect(page.locator('input[autocomplete="username"]')).toBeVisible()
      await page.waitForLoadState('networkidle')
      expect(announcementRequests).toBe(0)
      await expect(page.locator('.full-button', { hasText: locale === 'zh-CN' ? '正在加载配置' : 'Loading settings' })).toHaveCount(0)
      await expect(page.getByText(locale === 'zh-CN' ? '公告读取失败' : 'Unable to load announcements', { exact: true })).toHaveCount(0)
      await expectNoPageOverflow(page)
      await page.screenshot({ path: info.outputPath(`${path.slice(1)}.png`), fullPage: true })
      if (info.project.name.endsWith('reflow')) {
        await page.setViewportSize({ width: 320, height: 640 })
        await expectNoPageOverflow(page)
      }
    }
  })
}
