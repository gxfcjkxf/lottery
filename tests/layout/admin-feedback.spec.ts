import { expect, test, type Locator } from '@playwright/test'
import { gameId, origins, installLayoutApi, expectNoPageOverflow } from './fixture'
import { overviewFixture } from './platform-overview'

for (const locale of ['en', 'zh-CN'] as const) test(`brand operations share bottom-right feedback and close successful forms (${locale})`, async ({ page }, info) => {
 await page.clock.install()
 await installLayoutApi(page, locale)
 const zh = locale === 'zh-CN', memberId = 'bbbbbbbb-bbbb-4bbb-8bbb-000000000001'
 const errors: string[] = [], recharges: { key: string | undefined; body: unknown }[] = []
 let reads = 0, edits = 0, creates = 0
 page.on('pageerror', error => errors.push(error.message))
 await page.route('**/api/v1/admin/**', async route => {
  const req = route.request(), path = new URL(req.url()).pathname
  const reply = (data: unknown) => route.fulfill({ json: { success: true, data } })
  if (path.endsWith('/me')) return reply({ account: { id: gameId, username: 'feedback_reviewer', super_admin: false, brand_ids: [gameId], permissions_by_brand: { [gameId]: ['overview.view.brand', 'user.view.brand', 'user.write.brand', 'user.create.brand', 'recharge.write.brand', 'recharge.view.brand'] } } })
  if (path.endsWith('/brands')) return reply({ items: [{ id: gameId, name: 'Aurora', code: 'aurora', status: 'active', member_count: '1', agent_count: '0', recharge_quota_available: '900', created_at: '2026-10-25T00:00:00Z' }] })
  expect(req.headers()['x-brand-id']).toBe(gameId)
  if (path.endsWith('/overview')) return reply({ ...overviewFixture(), brand_id: gameId, brands: [] })
  if (path.endsWith('/users') && req.method() === 'GET') { reads++; return reply({ items: [{ id: memberId, global_user_id: memberId, brand_id: gameId, username: 'test001', display_name: 'test001', phone: '', notes: '', tags: [], status: 'normal', joined_at: '2026-10-21T00:00:00Z', online: false, display_points: '0' }], limit: 100, offset: 0, total_count: '1' }) }
  if (path.endsWith('/users') && req.method() === 'POST') { creates++; expect(req.headers()['idempotency-key']).toBeTruthy(); return reply({ user_id: memberId, member_id: memberId, brand_id: gameId, terms_accepted: false, audit_log_id: gameId }) }
  if (path.endsWith(`/users/${memberId}`) && req.method() === 'PATCH') { edits++; expect(req.postDataJSON().notes).toBe('reviewed'); return reply({ audit_log_id: gameId }) }
  if (path.endsWith('/recharge-quota')) return reply({ brand_id: gameId, available: '900' })
  if (path.endsWith('/recharges/manual')) {
   recharges.push({ key: req.headers()['idempotency-key'], body: req.postDataJSON() })
   if (recharges.length === 1) return route.fulfill({ status: 503, json: { success: false, error: { code: 'INTERNAL_ERROR', message: 'Private failure detail' } } })
   return reply({ id: gameId, brand_id: gameId, member_id: memberId, points: '10', method: 'manual', state: 'confirmed', ledger_entry_id: gameId, confirmed_at: '2026-10-25T00:00:00Z' })
  }
  throw Error(`Unexpected feedback request: ${req.method()} ${path}`)
 })
 await page.goto(origins.brand)
 const overview = page.getByTestId('brand-overview'), metrics = overview.getByRole('group').getByRole('button')
 await expect(metrics).toHaveCount(3)
 const preset = page.viewportSize()!
 for (const width of info.project.name.endsWith('reflow') ? [320, 360, 480, 768, 900, 901, 1280, 1440] : [preset.width]) {
  if (info.project.name.endsWith('reflow')) await page.setViewportSize({ width, height: preset.height })
  for (let i = 0; i < 3; i++) {
   await metrics.nth(i).click()
   await overview.locator('.overview-trend h2').click()
   await expect(metrics.nth(i)).toHaveAttribute('aria-pressed', 'true')
   await expect(metrics.nth(i)).toHaveClass(/button-primary/)
   await expect(metrics.nth(i)).toHaveCSS('background-color', 'rgb(5, 150, 105)')
   await expect(metrics.nth(i)).toHaveCSS('color', 'rgb(255, 255, 255)')
  }
  await expectNoPageOverflow(page)
 }
 await page.screenshot({ path: info.outputPath('trend-selection.png') })
 if (info.project.name.endsWith('reflow')) await page.setViewportSize({ width: 320, height: preset.height })
 if (await page.locator('.mobile-nav').isVisible()) await page.locator('.mobile-nav button').nth(1).click()
 else await page.locator('.side-nav').getByRole('button', { name: zh ? '会员管理' : 'Member management', exact: true }).click()
 const openActions = () => page.locator('.member-directory-table').getByRole('button', { name: zh ? 'test001 的操作' : 'Actions for test001', exact: true }).click()
 async function feedbackVisible(toast: Locator) {
  await expect(toast).toBeVisible()
  await expectNoPageOverflow(page)
  const stack = page.locator('.operation-toast-stack')
  const bounds = await stack.boundingBox(); expect(bounds).not.toBeNull()
  expect(bounds!.x).toBeGreaterThanOrEqual(0); expect(bounds!.y).toBeGreaterThanOrEqual(0)
  expect(page.viewportSize()!.width - bounds!.x - bounds!.width).toBeLessThanOrEqual(21)
  expect(page.viewportSize()!.height - bounds!.y - bounds!.height).toBeLessThanOrEqual(21)
  expect(await toast.evaluate(e => { const r = e.getBoundingClientRect(); return !!document.elementFromPoint(r.x + r.width / 2, r.y + Math.min(r.height / 2, 24))?.closest('.toast') })).toBe(true)
 }
 await openActions(); await page.getByRole('menuitem', { name: zh ? '充值' : 'Recharge', exact: true }).click()
 const recharge = page.getByRole('dialog', { name: zh ? '人工充值' : 'Manual recharge', exact: true })
 await recharge.locator('input[inputmode=numeric]').fill('10'); await recharge.locator('button[type=submit]').click()
 await feedbackVisible(page.locator('.toast-error'))
 await expect(recharge).toBeVisible(); await expect(recharge.locator('input[inputmode=numeric]')).toBeDisabled()
 await expect(recharge.locator('form .message, form .success-note')).toHaveCount(0)
 await expect(page.locator('.operation-toast-stack')).not.toContainText('Private failure detail')
 await page.screenshot({ path: info.outputPath('recharge-error.png') })
 await page.locator('.toast-error button').click()
 const before = reads
 await recharge.locator('button[type=submit]').click()
 await expect(recharge).toHaveCount(0)
 const credited = page.locator('.toast-success').filter({ hasText: zh ? '人工充值已入账。' : 'Manual recharge credited.' })
 await feedbackVisible(credited); await expect.poll(() => reads).toBeGreaterThan(before)
 expect(recharges).toHaveLength(2); expect(recharges[1]).toEqual(recharges[0]); expect(recharges[0]!.key).toBeTruthy()
 await page.screenshot({ path: info.outputPath('recharge-success.png') })
 await credited.getByRole('button').focus()
 await page.clock.fastForward(4000); await expect(credited).toBeVisible()
 await page.locator('.member-filters button[type=submit]').focus()
 await page.clock.fastForward(4000); await expect(credited).toHaveCount(0)
 await openActions(); await page.getByRole('menuitem', { name: zh ? '编辑' : 'Edit', exact: true }).click()
 const edit = page.getByRole('dialog', { name: zh ? '编辑会员' : 'Edit member', exact: true })
 await edit.locator('textarea').fill('reviewed'); await edit.getByRole('button', { name: zh ? '保存' : 'Save', exact: true }).click()
 await expect(edit).toHaveCount(0); expect(edits).toBe(1); await feedbackVisible(page.locator('.toast-success'))
 await page.locator('.toast-success button').click()
 await page.getByRole('button', { name: zh ? '添加会员' : 'Add member', exact: true }).click()
 const create = page.getByRole('dialog', { name: zh ? '添加会员' : 'Add member', exact: true })
 await create.getByRole('textbox', { name: zh ? '账号' : 'Account', exact: true }).fill('review_member')
 await create.getByLabel(zh ? '初始密码' : 'Initial password', { exact: true }).fill('test-password-2026')
 await create.locator('button[type=submit],button:not([type])').last().click()
 await expect(create).not.toBeVisible(); expect(creates).toBe(1)
 await feedbackVisible(page.locator('.toast-success'))
 await expect(page.locator('.toast-success')).toContainText(zh ? '会员已添加' : 'Member added')
 await page.screenshot({ path: info.outputPath('member-created.png') })
 expect(errors).toEqual([])
})
