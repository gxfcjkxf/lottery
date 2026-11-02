import { test, expect } from '@playwright/test'
import { gameId, origins, installLayoutApi, expectNoPageOverflow, expectInsideViewport } from './fixture'

for (const locale of ['en', 'zh-CN'] as const) test(`reset password without scope warning or checkbox (${locale})`, async ({ page }, info) => {
  const zh = locale === 'zh-CN'
  await installLayoutApi(page, locale)
  let writes = 0
  const member = { id: gameId, global_user_id: gameId, brand_id: gameId, username: 'test001', phone: '', display_name: '', notes: '', tags: [], status: 'normal', joined_at: '2026-10-21T00:00:00Z', online: false, display_points: '0' }
  await page.route('**/api/v1/admin/**', async route => {
    const req = route.request(), path = new URL(req.url()).pathname
    const reply = (data: unknown) => route.fulfill({ json: { success: true, data } })
    if (path.endsWith('/me')) return reply({ account: { id: gameId, username: 'reset_reviewer', super_admin: false, brand_ids: [gameId], permissions_by_brand: { [gameId]: ['user.view.brand', 'user.write.brand', 'user.password_reset.brand'] } } })
    if (path.endsWith('/brands')) return reply({ items: [{ id: gameId, name: 'Aurora', code: 'aurora', status: 'active', member_count: '1', agent_count: '0', recharge_quota_available: '0', created_at: '2026-10-21T00:00:00Z' }], total_count: '1' })
    if (path.endsWith('/users') && req.method() === 'GET') return reply({ items: [member], limit: 100, offset: 0, total_count: '1' })
    if (path.endsWith('/reset-password')) {
      expect(req.method()).toBe('POST')
      expect(req.postDataJSON().password).toBe('new-password-2026')
      expect(req.headers()['idempotency-key']).toBeTruthy()
      writes++
      return reply({ audit_log_id: gameId })
    }
    throw new Error(`Unexpected reset request: ${path}`)
  })
  await page.goto(origins.brand)
  await expect(page.locator('.management-console')).toBeVisible()
  if (await page.locator('.mobile-nav').isVisible()) await page.locator('.mobile-nav button').nth(1).click()
  else await page.locator('.side-nav').getByRole('button', { name: zh ? '会员管理' : 'Member management', exact: true }).click()
  await page.getByRole('button', { name: zh ? 'test001 的操作' : 'Actions for test001', exact: true }).click()
  await page.getByRole('menuitem', { name: zh ? '编辑' : 'Edit', exact: true }).click()
  const edit = page.getByRole('dialog', { name: zh ? '编辑会员' : 'Edit member', exact: true })
  await edit.getByRole('button', { name: zh ? '重置密码' : 'Reset password', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: zh ? '重置密码' : 'Reset password', exact: true })
  await expect(dialog).toBeVisible()
  await expect(dialog.getByRole('checkbox')).toHaveCount(0)
  await expect(dialog.locator('.danger-note')).toHaveCount(0)
  const submit = dialog.getByRole('button', { name: zh ? '确认重置密码' : 'Confirm reset', exact: true })
  await expect(submit).toBeDisabled()
  await dialog.locator('input[type=password]').fill('new-password-2026')
  await expect(submit).toBeEnabled()
  const viewport = page.viewportSize()!
  for (const width of info.project.name.endsWith('reflow') ? [320, 640, 900, 1440] : [viewport.width]) {
    if (info.project.name.endsWith('reflow')) await page.setViewportSize({ width, height: viewport.height })
    await expectInsideViewport(dialog)
    await expectNoPageOverflow(page)
    await page.screenshot({ path: info.outputPath(`password-reset-${locale}-${width}.png`) })
  }
  await submit.click()
  await expect(dialog).not.toBeVisible()
  await expect(edit).toBeVisible()
  expect(writes).toBe(1)
})
