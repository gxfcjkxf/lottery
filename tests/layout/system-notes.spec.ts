import { resolve } from 'node:path'
import { expect, test } from '@playwright/test'
import { expectNoPageOverflow, gameId, installLayoutApi, installOrdersLayoutApi, origins } from './fixture'

for (const locale of ['zh-CN', 'en'] as const) for (const view of ['user', 'brand', 'platform'] as const) {
  test(`system notes follow the selected language: ${view} (${locale})`, async ({ page }, info) => {
    const brand = view === 'user' ? gameId : '11111111-1111-4111-8111-111111111111'
    const id = '44444444-4444-4444-8444-444444444444', at = '2026-10-24T00:00:00Z'
    const manual = 'Keep this operator note unchanged.'
    const row = { id, brand_id: brand, member_id: view === 'user' ? gameId : id, account_id: id, points: '50', state: 'paid', version: 3,
      source_allocation: [{ source: 'recharge', state: 'available', points: '50' }], reserve_entry_id: id, release_entry_id: null, paid_entry_id: id,
      cycle_from_at: null, cycle_from_version: '0', reserve_version: '1', created_at: at, updated_at: at, reviewed_at: at, completed_at: at,
      decision_reason: 'automatic review by saved policy', audit_log_id: id }
    const history = { brand_id: brand, order_id: id, items: [
      { id, version: 1, from_state: '', to_state: 'reviewing', reason: 'withdrawal application', actor_type: 'user', created_at: at, audit_log_id: id },
      { id: gameId, version: 2, from_state: 'reviewing', to_state: 'processing', reason: 'automatic review by saved policy', actor_type: 'system', created_at: at, audit_log_id: id },
      { id: brand, version: 3, from_state: 'processing', to_state: 'paid', reason: manual, actor_type: 'admin', created_at: at, audit_log_id: id },
    ] }
    const errors: string[] = []
    page.on('pageerror', e => errors.push(e.message))
    if (view === 'user') { await installLayoutApi(page, locale); await installOrdersLayoutApi(page) }
    await page.route('**/api/**', async route => {
      const url = new URL(route.request().url()), path = url.pathname
      if (!path.includes('/withdrawal')) return route.fallback()
      expect(route.request().method()).toBe('GET')
      let data: unknown
      if (path.endsWith('/history')) data = history
      else if (path.endsWith('/withdrawals/' + id)) data = row
      else if (path.endsWith('/withdrawals')) data = { brand_id: brand, items: [row], limit: view === 'user' ? 20 : Number(url.searchParams.get('limit')), offset: 0, has_more: false, total_count: '1' }
      else if (path.endsWith('/withdrawal-qualification')) data = { brand_id: brand, member_id: gameId, account_id: id, base_points: '10', valid_points: '100', valid_order_count: '2', credit_numerator: '100', credit_denominator: '1', meets_turnover: true, cycle_from_at: null, cycle_from_version: '0', cutoff_at: at, cutoff_version: '8' }
      else if (path.endsWith('/withdrawal-availability')) data = { points_step: '1', available_points: '100', source_balances: [{ source: 'recharge', state: 'available', points: '100' }], actor_context: 'a'.repeat(64), brand_id: brand, member_id: gameId, policy_enabled: true, eligibility_configured: true, can_apply: true, reason_code: 'AVAILABLE', min_points: '10', max_points: '100', allowed_sources: ['recharge'], real_payments: false }
      else throw Error(`Unexpected withdrawal read: ${path}`)
      return route.fulfill({ json: { success: true, data, associations: { [`member:${row.member_id}`]: { name: 'test_member' } } } })
    })
    if (view === 'user') {
      await page.goto(origins.user + '/withdraw')
      await page.locator('.withdrawal-order').getByRole('button').click()
    } else {
      const source = await (await page.request.get(origins.brand + '/src/RewardsManagement.vue')).text()
      const vuePath = source.match(/from "([^"]*deps\/vue\.js[^"]*)"/)![1]
      const url = origins.brand + '/__system-notes-' + view
      const styles = ['shared/src/styles.css', `${view === 'brand' ? 'admin-web' : 'platform-web'}/src/style.css`, 'shared/src/management.css', 'shared/src/key-value.css', 'shared/src/controls.css']
      await page.route(url, route => route.fulfill({ contentType: 'text/html', body: `<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module">import{createApp}from'${vuePath}';import Harness from'/@fs/${resolve('tests/layout/FinanceConsistencyHarness.vue')}';${styles.map(file => `import'/@fs/${resolve(file)}';`).join('')}createApp(Harness,${JSON.stringify({ locale, view: view + '-withdrawals' })}).mount('#app')</script></html>` }))
      await page.goto(url)
      await page.locator('.console-panel table tbody tr').first().getByRole('button').click()
      if (view === 'platform') await page.getByRole('dialog').getByRole('button', { name: locale === 'en' ? 'View action history' : '查看操作记录', exact: true }).click()
    }
    const dialog = page.getByRole('dialog')
    await expect(dialog).toContainText(locale === 'zh-CN' ? '提交提现申请' : 'Withdrawal application submitted')
    await expect(dialog).toContainText(locale === 'zh-CN' ? '按已保存策略自动审核' : 'Automatically reviewed using the saved policy')
    await expect(dialog).toContainText(manual)
    await expect(dialog).not.toContainText('withdrawal application')
    await expect(dialog).not.toContainText('automatic review by saved policy')
    if (view === 'platform') {
      const actors = dialog.locator('[data-testid="platform-withdrawal-history"] tbody tr td:nth-child(3)')
      await expect(actors).toHaveText(locale === 'zh-CN' ? ['用户', '系统', '管理员'] : ['User', 'System', 'Administrator'])
    }
    if (view === 'brand') {
      const actors = dialog.locator('.withdrawal-history-row small')
      for (const [index, label] of (locale === 'zh-CN' ? ['会员', '系统', '管理员'] : ['User', 'System', 'Admin']).entries()) {
        await expect(actors.nth(index)).toContainText(label)
      }
    }
    const preset = page.viewportSize()!
    for (const width of info.project.name.includes('reflow') ? [320, 640, 1040, 1440] : [preset.width]) {
      if (info.project.name.includes('reflow')) await page.setViewportSize({ width, height: preset.height })
      await expectNoPageOverflow(page)
      expect(await dialog.evaluate(node => node.scrollWidth <= node.clientWidth + 1)).toBe(true)
      await dialog.screenshot({ path: info.outputPath(`${view}-${locale}-${width}.png`) })
    }
    expect(errors).toEqual([])
  })
}
