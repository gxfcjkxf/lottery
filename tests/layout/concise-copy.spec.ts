import { test, expect } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { installConsoleSession, openPlatformPage } from './real-console-session'
import { expectRealEntry, expectListLayout, reviewRealPage } from './real-page-review'
import { gameId, origins } from './fixture'
import { selectControl } from '../select-control'

for (const locale of ['en', 'zh-CN'] as const) {
  test(`real source configuration uses concise bilingual labels (${locale})`, async ({ page }, info) => {
    page.setDefaultTimeout(8000)
    const zh = locale === 'zh-CN', errors: string[] = []
    page.on('pageerror', error => errors.push(error.message))
    const model = JSON.parse(readFileSync('backend/internal/rules/config/examples/lucky_group_preview.json', 'utf8')).catalog.model
    const game = { id: gameId, code: 'copy_review', name: 'Copy review', model, timezone: 'UTC', version: 1, status: 'active', deletable: false }
    await page.route('**/api/**', route => {
      expect(route.request().method()).toBe('GET')
      const path = new URL(route.request().url()).pathname
      const reply = (data: unknown) => route.fulfill({ json: { success: true, data } })
      if (path.endsWith('/platform-games')) return reply({ games: [game], total_count: '1', limit: 50, offset: 0 })
      if (path.endsWith('/' + gameId)) return reply({ game, rule_protocol: 'configured', plays: [], periods: [], total_count: '0', mark_six_configured: false })
      if (path.endsWith('/draw-sources')) return reply({ items: [], active_source_id: '', active_source_revision: 0, version: 0 })
      if (path.endsWith('/automation')) return reply({ plan: null, brand: null, allowed_origins: ['https://source.example'], attempts: [], source_check: null })
      throw Error('Unconfigured copy review request: ' + path)
    })
    await installConsoleSession(page, 'platform', locale, gameId, ['platform_game.view.platform', 'platform_game.write.platform'])
    await page.goto(origins.platform)
    await expect(page.locator('.management-console')).toBeVisible()
    await openPlatformPage(page, zh ? '彩种' : 'Games')
    await expectRealEntry(page, 'platform')
    await page.getByRole('row').filter({ hasText: game.name }).getByRole('button', { name: zh ? '开奖数据源' : 'Data sources', exact: true }).click()
    const editor = page.getByTestId('automation-settings')
    await expect(editor).toBeVisible()
    await selectControl(editor.getByRole('combobox', { name: zh ? '响应格式' : 'Response format', exact: true }), 'browser')
    await expect(editor.getByLabel(zh ? '忽略的辅助请求地址' : 'Blocked auxiliary URLs', { exact: true })).toBeVisible()
    await editor.getByRole('button', { name: zh ? '添加 JSON 查询' : 'Add JSON request', exact: true }).click()
    await selectControl(editor.getByRole('combobox', { name: zh ? '查询方法' : 'Query method', exact: true }), 'POST')
    await expect(editor.getByLabel(zh ? 'URL 编码表单正文' : 'URL-encoded form body', { exact: true })).toBeVisible()
    await selectControl(editor.getByRole('combobox', { name: zh ? '来源时间格式' : 'Source time format', exact: true }), 'custom')
    await expect(editor.getByLabel(zh ? 'Go 时间格式模板' : 'Go time layout', { exact: true })).toBeVisible()
    await expect(editor.getByLabel(zh ? '来源时区' : 'Source timezone', { exact: true })).toBeVisible()
    // The shared directive counts Unicode characters instead of UTF-16 units.
    const name = editor.getByLabel(zh ? '数据源名称' : 'Source name', { exact: true })
    await expect(name).toHaveAttribute('data-max-characters', '120')
    await name.fill('x'.repeat(121))
    await editor.locator('form').evaluate((form: HTMLFormElement) => form.requestSubmit())
    await expect(name).toHaveAttribute('aria-invalid', 'true')
    await expect(editor.locator('.inline-validation-error')).toContainText('120')
    await name.fill('Copy review source')
    await expect(editor.getByLabel(zh ? '渲染超时（毫秒）' : 'Render timeout (milliseconds)', { exact: true })).toHaveAttribute('max', '10000')
    await reviewRealPage(page, info, `source-copy-${locale}`)
    expect(errors).toEqual([])
  })

  test(`real commission details use concise table labels (${locale})`, async ({ page }, info) => {
    page.setDefaultTimeout(8000)
    const zh = locale === 'zh-CN', at = '2026-11-02T00:00:00Z', errors: string[] = []
    page.on('pageerror', error => errors.push(error.message))
    const cycle = { id: gameId, brand_id: gameId, window_from: at, window_to: '2026-11-09T00:00:00Z', anchor_order_id: gameId,
      calendar: { timezone: 'UTC', cycle: 'weekly', boundary_time: '00:00:00', weekday: 1, month_day: null, short_month: '' },
      state: 'ready', version: 1, target_count: '1', scan_complete: true, current_run_id: gameId, current_generation: '1', evidence_epoch: '1', evidence_current: true,
      calculated_count: '1', earning_count: '1', total_points: '5', created_by: gameId, creation_actor_type: 'admin', reason: '', created_at: at, updated_at: at, last_error_code: null, creation_audit_log_id: gameId }
    await page.route('**/api/**', route => {
      expect(route.request().method()).toBe('GET')
      const path = new URL(route.request().url()).pathname
      const reply = (data: unknown) => route.fulfill({ json: { success: true, data, associations: { [`cycle:${gameId}`]: { name: 'Review cycle', from: cycle.window_from, to: cycle.window_to }, [`agent:${gameId}`]: { name: 'review_agent' }, [`member:${gameId}`]: { name: 'review_member' } } } })
      if (path.endsWith('/commission-cycles')) return reply({ brand_id: gameId, items: [cycle], total_count: '1', limit: 50, offset: 0 })
      if (path.endsWith('/commission-cycles/' + gameId)) return reply(cycle)
      if (path.endsWith('/earnings')) return reply({ brand_id: gameId, cycle_id: gameId, items: [{ id: gameId, brand_id: gameId, cycle_id: gameId, run_id: gameId, agent_id: gameId, member_id: gameId, exact_amount: { numerator: '10', denominator: '2' }, points: '5', created_at: at }], total_count: '1', limit: 50, offset: 0 })
      throw Error('Unconfigured commission copy request: ' + path)
    })
    await installConsoleSession(page, 'platform', locale, gameId, ['agent.view.platform'])
    await page.goto(origins.platform)
    await expect(page.locator('.management-console')).toBeVisible()
    await openPlatformPage(page, zh ? '佣金' : 'Commission')
    await openPlatformPage(page, zh ? '周期' : 'Cycles', gameId)
    await expectRealEntry(page, 'platform')
    await page.getByTestId('platform-commission-list').locator('tbody .row-action').click()
    const detail = page.getByRole('dialog')
    await expect(detail.locator('th').nth(2)).toHaveText(zh ? '精确金额' : 'Exact amount')
    await expect(detail.locator('tbody tr td').nth(2)).toHaveText('10 / 2')
    await expect(detail.locator('tbody tr td').nth(2)).toHaveAttribute('data-label', zh ? '精确金额' : 'Exact amount')
    await expect(detail).not.toContainText(zh ? '无法读取关联信息' : 'Unable to load associated information')
    const preset = page.viewportSize()!
    for (const width of info.project.name.endsWith('reflow') ? [320, 620, 900, 1080, 1440] : [preset.width]) {
      if (info.project.name.endsWith('reflow')) await page.setViewportSize({ width, height: preset.height })
      const amount = detail.locator('tbody tr td').nth(2)
      await amount.scrollIntoViewIfNeeded()
      await expect(amount).toBeInViewport()
      await expectListLayout(page)
      await page.screenshot({ path: info.outputPath(`commission-copy-${locale}-${width}.png`), fullPage: true })
    }
    if (info.project.name.endsWith('reflow')) await page.setViewportSize(preset)
    expect(errors).toEqual([])
  })
}
