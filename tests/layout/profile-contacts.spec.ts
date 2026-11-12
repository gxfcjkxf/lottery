import { test, expect } from '@playwright/test'
import { installLayoutApi, installOrdersLayoutApi, origins, gameId, expectNoPageOverflow } from './fixture'

for (const locale of ['en', 'zh-CN'] as const) {
  test(`optional profile contacts (${locale})`, async ({ page }, info) => {
    await installLayoutApi(page, locale)
    await installOrdersLayoutApi(page)
    const user: Record<string, string> = { id: gameId, username: 'contact_member', status: 'active' }
    await page.route('**/api/v1/me', route => route.fulfill({ json: { success: true, data: { user, member: { id: gameId, brand_id: gameId, status: 'normal', display_name: 'Contact member', joined_at: '2026-10-17T00:00:00Z' } } } }))
    await page.route('**/api/v1/me/profile', route => {
      expect(route.request().method()).toBe('PATCH')
      const body = route.request().postDataJSON()
      expect(body).toEqual({ wechat: 'wechat_member' })
      Object.assign(user, body)
      return route.fulfill({ json: { success: true, data: { audit_log_id: gameId } } })
    })
    await page.goto(`${origins.user}/account`)
    const edit = locale === 'en' ? 'Complete profile' : '补充资料'
    const phone = locale === 'en' ? 'Phone' : '手机号'
    const wechat = locale === 'en' ? 'WeChat' : '微信号'
    const preset = page.viewportSize()!
    for (const width of info.project.name.includes('reflow') ? [320, 700, 1000, 1440] : [preset.width]) {
      if (info.project.name.includes('reflow')) await page.setViewportSize({ width, height: 800 })
      await page.getByRole('button', { name: edit, exact: true }).click()
      const dialog = page.getByRole('dialog')
      for (const label of [phone, wechat, 'Telegram', 'WhatsApp']) {
        await expect(dialog.getByLabel(label, { exact: true })).not.toHaveAttribute('required')
        await expect(dialog.getByLabel(label, { exact: true })).toBeEditable()
      }
      await expectNoPageOverflow(page)
      await page.screenshot({ path: info.outputPath(`profile-${locale}-${width}.png`), fullPage: true })
      await dialog.getByRole('button', { name: locale === 'en' ? /Close/ : /关闭/ }).click()
    }
    await page.getByRole('button', { name: edit, exact: true }).click()
    await page.getByRole('dialog').getByLabel(wechat, { exact: true }).fill('wechat_member')
    await page.getByRole('dialog').getByRole('button', { name: locale === 'en' ? 'Save profile' : '保存资料', exact: true }).click()
    await expect(page.getByRole('dialog')).toHaveCount(0)
    await expect(page.locator('.account-details')).toContainText('wechat_member')
    await expectNoPageOverflow(page)
  })
}
