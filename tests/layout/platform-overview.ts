import { expect, test } from '@playwright/test'
import { selectControl } from '../select-control'
import { gameId, origins, installLayoutApi, expectNoPageOverflow } from './fixture'

export const overviewFixture = () => ({
  snapshot_at: '2026-10-20T06:30:00Z', timezone: 'Asia/Singapore', today: '2026-10-20',
  total_members: '28649', online_members: '386', unsettled_points: '9007199254740993',
  available_points: '6482150.25', frozen_points: '186400', withdrawal_points: '42800', withdrawal_count: '12',
  yesterday_same_time_stake: '1000000.50',
  days: Array.from({ length: 7 }, (_, i) => ({ day: `2026-10-${14 + i}`, stake_points: String(1000000 + i * 10000), net_points: i % 2 ? '-24000.75' : '100000.25', order_count: '8432', bettor_count: '1862', refund_points: '12600.50', new_members: '236' })),
  brands: [{ brand_id: gameId, brand_name: 'Aurora / 品牌名称与很长的品牌展示名称', stake_points: '9007199254740993', net_points: '-6800.25' }],
})

export function registerPlatformOverviewTests(reflow = false) {
  for (const locale of ['en', 'zh-CN'] as const) {
    test(`platform overview default home and responsive totals (${locale})`, async ({ page }, info) => {
      await page.clock.install()
      await installLayoutApi(page, locale)
      if (reflow) await page.addInitScript(({ id }) => sessionStorage.setItem('lottery.platform.selectedBrand', JSON.stringify({ accountId: id, brandId: id })), { id: gameId })
      const errors: string[] = []
      page.on('pageerror', error => errors.push(error.message))
      let reads = 0
      let mode: 'success' | 'failure' | 'hold' = 'success'
      let release: (() => void) | undefined
      await page.route('**/api/v1/platform/**', async route => {
        expect(route.request().method()).toBe('GET')
        const path = new URL(route.request().url()).pathname
        const reply = (data: unknown) => route.fulfill({ json: { success: true, data } })
        if (path.endsWith('/presentation')) return reply({ display_name: 'Platform', logo_text: 'P', logo_url: '' })
        if (path.endsWith('/me')) return reply({ account: { id: gameId, username: 'operator', super_admin: true, platform_permissions: ['overview.view.platform', 'withdrawal.view.platform'] } })
        if (path.endsWith('/platform-settings')) return reply({theme:'jade', timezone: 'Asia/Singapore', date_format: 'YYYY-MM-DD', time_format: 'HH:mm:ss', display_name: 'Platform', logo_text: 'P', logo_url: '', version: 1, updated_at: '2026-10-18T00:00:00Z' })
        if (path.endsWith('/brands')) return reply({ items: [{ id: gameId, code: 'aurora', name: 'Aurora', status: 'active', member_count: '12', agent_count: '3', recharge_quota_available: '1000', created_at: '2026-10-24T00:00:00Z' }] })
        if (path.endsWith('/cash/orders')) return reply({items:[],total_count:'0'})
        if (path.endsWith('/withdrawals')) {
          expect(new URL(route.request().url()).searchParams.get('state')).toBe('reviewing')
          expect(route.request().headers()['x-brand-id']).toBe(gameId)
          return reply({ brand_id: gameId, items: [], limit: 50, offset: 0, has_more: false, total_count: '0' })
        }
        if (path.endsWith('/overview')) {
          expect(route.request().headers()['x-brand-id']).toBeUndefined()
          reads++
          if (mode === 'hold') await new Promise<void>(resolve => { release = resolve })
          if (mode === 'failure') return route.fulfill({ status: 503, json: { success: false, error: { code: 'SERVICE_UNAVAILABLE', message: 'internal detail' } } })
          return reply(overviewFixture())
        }
        throw Error(`Unexpected overview request: ${path}`)
      })
      await page.goto(origins.platform)
      const overview = page.getByTestId('platform-overview')
      await expect(overview.locator('.overview-stats')).toBeVisible()
      await expect(page.locator('h1')).toHaveCount(1)
      await expect(page.locator('h1')).toHaveText(locale === 'en' ? 'Platform overview' : '经营总览')
      await expect(page.locator('.topbar-brand-picker')).toHaveCount(0)
      await expect(overview.locator('.page-heading button, .page-heading p, .overview-meta')).toHaveCount(0)
      await expect(overview.locator('.overview-stat p')).toHaveCount(4)
      await expect(overview.locator('.overview-current p')).toHaveCount(3)
      await expect(overview.locator('.overview-business .status-chip')).toHaveCount(0)
      await expect(overview).not.toContainText(locale === 'en' ? 'vs. yesterday at this time' : '较昨日同时段')
      await expect(overview).not.toContainText(locale === 'en' ? 'vs. the previous day' : '较前日')
      await expect(overview.locator('.overview-tasks')).toHaveCount(1)
      await expect(overview.locator('.overview-stats + .overview-tasks-priority')).toBeVisible()
      await expect(overview.locator('.overview-tasks .overview-task strong')).toHaveText('12')
      await expect(overview).not.toContainText(locale === 'en' ? 'No comparable prior stakes' : '前期无可比投注')
      await expect(overview).toContainText('9,007,199,254,740,993')
      await expect(overview.locator('.overview-current')).toContainText('6,482,150.25')
      await expect(overview.locator('.overview-business').first()).toContainText('+100,000.25')
      const preset = page.viewportSize()!
      const widths = reflow ? [...new Set([
        ...Array.from({ length: 9 }, (_, i) => 320 + i * 20),
        ...[620,640,700,768,820,900,1000,1080,1200,1440].flatMap(width => [width - 1, width, width + 1]),1024,
      ])] : [preset.width]
      for (const width of widths) {
        if (reflow) await page.setViewportSize({ width, height: preset.height })
        await expectNoPageOverflow(page)
        await expect(overview.locator('.overview-stats > article')).toHaveCount(4)
        await expect(overview.locator('.overview-chart-column')).toHaveCount(7)
      }
      const group = overview.getByRole('group')
      await group.getByRole('button', { name: locale === 'en' ? 'Settled net win' : '结算净赢', exact: true }).click()
      await expect(overview.locator('.overview-bar-loss')).toHaveCount(3)
      await group.getByRole('button', { name: locale === 'en' ? 'New members' : '新增会员', exact: true }).click()
      await expect(overview.locator('.overview-chart-value').first()).toHaveText('236')
      await expect(overview.locator('.overview-definitions, details')).toHaveCount(0)
      await page.screenshot({ path: info.outputPath(`overview-${locale}.png`), fullPage: true })
      await info.attach('overview-viewport.json', { body: JSON.stringify({ preset, widths, deviceScaleFactor: await page.evaluate(() => devicePixelRatio), screen: await page.evaluate(() => ({ width: screen.width, height: screen.height })) }), contentType: 'application/json' })
      if (reflow) {
        mode = 'hold'
        const before = reads
        await page.clock.fastForward(60000)
        await expect.poll(() => reads).toBeGreaterThan(before)
        await expect(page.locator('.global-loading-overlay')).toHaveCount(0)
        mode = 'failure'; release!()
        await expect(overview.getByRole('alert')).toBeVisible()
        await expect(overview.locator('.overview-stats')).toHaveCount(0)
        await expect(overview).not.toContainText('internal detail')
        mode = 'hold'; release = undefined
        await overview.getByRole('button', { name: locale === 'en' ? 'Retry' : '重试', exact: true }).click()
        await expect.poll(() => typeof release).toBe('function')
        await expect(page.locator('.global-loading-overlay')).toBeVisible()
        mode = 'success'; release!()
        await expect(overview.locator('.overview-stats')).toBeVisible()
        await expect(page.locator('.global-loading-overlay')).toHaveCount(0)
      }
      await overview.locator('.overview-tasks').getByRole('button', { name: locale === 'en' ? /Withdrawals to review/ : /提现待审核/ }).click()
      await expect(overview).toHaveCount(0)
      if (!reflow) {
        await expect(page.getByTestId('platform-brand-required')).toBeVisible()
        await expect(page.getByTestId('platform-withdrawals')).toHaveCount(0)
        await selectControl(page.locator('.topbar-brand-picker .select-control-trigger'), gameId)
      }
      await expect(page.getByTestId('platform-withdrawals')).toBeVisible()
      await expect(page.getByTestId('platform-withdrawals').locator('.empty-state')).toBeVisible()
      await expect(page.getByTestId('platform-withdrawals').getByRole('combobox', {name:locale === 'en' ? 'State' : '状态',exact:true})).toContainText(locale === 'en' ? 'Reviewing' : '审核中')
      await page.screenshot({ path: info.outputPath(`pending-withdrawals-${locale}.png`), fullPage: true })
      expect(errors).toEqual([])
    })
  }
}
