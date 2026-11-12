import { test, expect } from '@playwright/test'
import { installLayoutApi, installOrdersLayoutApi, origins, gameId, expectNoPageOverflow, expectInsideViewport } from './fixture'

for (const locale of ['en', 'zh-CN'] as const) {
  test(`withdrawal menus, application and receipt recovery (${locale})`, async ({ page }, info) => {
    await installLayoutApi(page, locale)
    await installOrdersLayoutApi(page)
    const zh = locale === 'zh-CN'
    const action = zh ? '提现' : 'Withdraw'
    const close = zh ? '关闭' : 'Close'
    const at = '2026-10-27T03:20:52Z'
    const row = { id: gameId, brand_id: gameId, member_id: gameId, account_id: gameId, points: '50', state: 'reviewing', version: 1,
      source_allocation: [{ source: 'recharge', state: 'available', points: '30' }, { source: 'winning', state: 'available', points: '20' }], reserve_entry_id: gameId,
      release_entry_id: null, paid_entry_id: null, cycle_from_at: null, cycle_from_version: '0', reserve_version: '1', created_at: at, updated_at: at,
      reviewed_at: null, completed_at: null, decision_reason: '', audit_log_id: gameId }
    const attempts: { key: string; body: string | null }[] = []
    let created = false
    await page.route('**/api/v1/withdrawal*', async route => {
      const path = new URL(route.request().url()).pathname
      const respond = (data: unknown) => route.fulfill({ json: { success: true, data } })
      if (path === '/api/v1/withdrawal-availability') return respond({ points_step: '10', available_points: '100', source_balances: [{source:'recharge',state:'available',points:'30'},{source:'winning',state:'available',points:'70'}], actor_context: 'a'.repeat(64), brand_id: gameId, member_id: gameId, policy_enabled: true, eligibility_configured: true, can_apply: true, reason_code: 'AVAILABLE', min_points: '10', max_points: '100', allowed_sources: ['recharge','winning'], real_payments: false })
      if (path === '/api/v1/withdrawal-qualification') return respond({ brand_id: gameId, member_id: gameId, account_id: gameId, base_points: '10', valid_points: '100', valid_order_count: '2', credit_numerator: '100', credit_denominator: '1', meets_turnover: true, cycle_from_at: null, cycle_from_version: '0', cutoff_at: at, cutoff_version: '8' })
      if (path === '/api/v1/withdrawals' && route.request().method() === 'GET') return respond({ brand_id: gameId, items: created ? [row] : [], limit: 20, offset: 0, has_more: false, total_count: created ? '1' : '0' })
      if (path === '/api/v1/withdrawals' && route.request().method() === 'POST') {
        attempts.push({ key: route.request().headers()['idempotency-key']!, body: route.request().postData() })
        if (attempts.length === 1) return route.fulfill({ status: 503, json: { success: false, error: { code: 'TEST_LOST_RECEIPT', message: 'Unknown result' } } })
        created = true
        return route.fulfill({ status: 201, json: { success: true, data: row } })
      }
      throw new Error(`Unexpected withdrawal request: ${path}`)
    })
    const errors: string[] = []
    page.on('pageerror', error => errors.push(error.message))
    await page.goto(`${origins.user}/withdraw`)
    await expect(page.locator('.workspace-page-title')).toHaveText(zh ? '提现记录' : 'Withdrawal records')
    await expect(page.locator('.withdrawal-records')).toContainText(zh ? '暂无提现申请。' : 'No withdrawal requests yet.')
    await expect(page.getByRole('dialog')).toHaveCount(0)
    const expectToolbarSpacing = async () => {
      const toolbar = (await page.locator('.withdrawal-toolbar').boundingBox())!
      const records = (await page.locator('.withdrawal-records').boundingBox())!
      const mobile=await page.evaluate(()=>matchMedia('(max-width:900px), (pointer:coarse)').matches)
      expect(records.y - toolbar.y - toolbar.height).toBeCloseTo(mobile?8:12, 0)
    }
    await expectToolbarSpacing()
    await expect(page.locator('.workspace-sidebar a[href="/withdraw"]')).toHaveText(action)
    if (info.project.name.includes('reflow')) {
      for (const width of [320, 700, 701, 999, 1000, 1001, 1440]) {
        await page.setViewportSize({ width, height: 800 })
        await expectNoPageOverflow(page)
        await expectToolbarSpacing()
      }
      await page.setViewportSize({ width: 320, height: 640 })
    }
    await page.getByRole('button', { name: zh ? '菜单' : 'Menu', exact: true }).click()
    const menu = page.locator('.workspace-menu')
    await menu.locator('a[href="/withdraw"]').click()
    // Selecting the current route also closes the quick menu.
    await expect(page.locator('.workspace-menu-layer')).toBeHidden()
    await page.locator('.withdrawal-toolbar').getByRole('button', { name: action, exact: true }).click()
    const dialog = page.getByRole('dialog')
    await expect(dialog).toBeVisible()
    await expectInsideViewport(dialog)
    await expectNoPageOverflow(page)
    const submit = dialog.getByRole('button', { name: zh ? '提交' : 'Submit', exact: true })
    await expect(submit).toBeDisabled()
    await dialog.getByRole('textbox').fill('5')
    await expect(submit).toBeDisabled()
    await expect(dialog.getByRole('textbox')).toHaveCount(1)
    await expect(dialog).not.toContainText(zh ? '可用来源分配' : 'Available source allocation')
    await dialog.getByRole('textbox').fill('55')
    await expect(submit).toBeDisabled()
    await expect(dialog).toContainText(zh ? '10 的整数倍' : 'multiple of 10')
    await dialog.getByRole('textbox').fill('50')
    await expect(submit).toBeEnabled()
    await page.screenshot({ path: `test-results/layout/withdrawal-${info.project.name}-${locale}.png` })
    await submit.click()
    await dialog.getByRole('button', { name: zh ? '重试原申请' : 'Retry original request', exact: true }).click()
    await expect(dialog).toHaveCount(0)
    expect(attempts).toHaveLength(2)
    expect(attempts[0]).toEqual(attempts[1])
    expect(JSON.parse(attempts[0].body!)).toEqual({ points: '50', source_allocation: row.source_allocation })
    await expect(page.locator('.withdrawal-records')).toContainText(zh ? '50 积分' : '50 pts')
    await expect(page.locator('.operation-toast-stack .toast-success')).toContainText(zh ? '申请已提交' : 'Request submitted')
    await page.locator('.withdrawal-toolbar button').click()
    await page.getByRole('dialog').getByRole('button', { name: close, exact: true }).click()
    await expect(page.getByRole('dialog')).toHaveCount(0)
    await expect(page.locator('.withdrawal-toolbar button')).toBeFocused()
    await expectNoPageOverflow(page)
    await page.screenshot({ path: `test-results/layout/withdrawal-records-${info.project.name}-${locale}.png` })
    if (info.project.name.includes('reflow')) {
      await page.setViewportSize({ width: 1440, height: 900 })
      await page.screenshot({ path: `test-results/layout/withdrawal-desktop-records-${info.project.name}-${locale}.png` })
      await page.locator('.withdrawal-toolbar button').click()
      await expectInsideViewport(page.getByRole('dialog'))
      await expectNoPageOverflow(page)
      await page.screenshot({ path: `test-results/layout/withdrawal-desktop-application-${info.project.name}-${locale}.png` })
    }
    expect(errors).toEqual([])
  })
}
