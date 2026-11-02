import { expectRealEntry, expectListLayout } from './real-page-review'
import { test, expect } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { configuredDemo } from '../../shared/src/configured-rules'
import { configuredTicketLabel as ticketLabel } from '../../shared/src/configured-business'
import { notificationCatalog } from '../../shared/src/notification-catalog'
import { installLayoutApi, installOrdersLayoutApi, origins, gameId, expectInsideViewport } from './fixture'

for (const locale of ['en', 'zh-CN'] as const) {
  test(`readable purchased ticket in notification (${locale})`, async ({ page }, info) => {
    page.setDefaultTimeout(8000)
    await installLayoutApi(page, locale)
    await installOrdersLayoutApi(page)
    const snapshot = configuredDemo(JSON.parse(readFileSync('backend/internal/rules/config/mark_six_v2.json', 'utf8')))
    const play = snapshot.catalog.plays.find(play => play.id === 'special_numbers')!
    const lines = ['1', '4', '20', '37', '46'].map(option => ({ play_id: play.id, options: [option], plate: 'A', stake: '5', prices: { win: '48.99' } }))
    const orderId = 'aaaaaaaa-aaaa-4aaa-8aaa-000000000042'
    const gameName = locale === 'en' ? 'New Macau Mark Six' : '新澳门六合彩'
    await page.route('**/api/v1/notifications?*', route => {
      expect(route.request().method()).toBe('GET')
      const query = new URL(route.request().url()).searchParams
      return route.fulfill({ json: { success: true, data: {
        brand_id: gameId, member_id: gameId, unread_count: '0', total_count: '1', limit: Number(query.get('limit')), offset: Number(query.get('offset')),
        items: [{ id: 'aaaaaaaa-aaaa-4aaa-8aaa-000000000043', brand_id: gameId, member_id: gameId, event_type: 'bet.order.placed', template_key: 'bet.order.placed', template_version: 1,
          content: notificationCatalog['bet.order.placed'], payload: { resource_id: orderId, points: '25' }, created_at: '2026-10-28T14:14:05Z', read_at: '2026-10-28T14:15:00Z' }],
      } } })
    })
    await page.route(`**/api/v1/configured/orders/${orderId}`, route => {
      expect(route.request().method()).toBe('GET')
      return route.fulfill({ json: { success: true, data: { brand_id: gameId, member_id: gameId, order: {
        id: orderId, game_id: gameId, game_name: gameName, period_no: '2026302', status: 'settled', placed_at: '2026-10-28T14:14:05Z',
        snapshot, quote: { bet_points: '25', lines }, result: { prize_points: '0' },
      } } } })
    })
    const errors: string[] = []
    page.on('pageerror', error => errors.push(error.message))
    await page.goto(`${origins.user}/notifications`)
    await page.locator('.notification-summary').click()
    const dialog = page.getByRole('dialog')
    await expect(dialog).toContainText(gameName)
    await expect(dialog).toContainText('2026302')
    await expect(dialog.locator('.notification-body')).toHaveCount(0)
    await expect(dialog.locator('.notification-bet-summary')).not.toContainText(locale === 'en' ? 'Placed at' : '投注时间')
    await expect(dialog).not.toContainText(orderId)
    await expect(dialog.locator('.bet-review-table tbody tr')).toHaveCount(5)
    await expect(dialog.locator('.bet-review-table tbody tr').first()).toContainText(ticketLabel(snapshot, lines[0], locale))
    await expect(dialog.locator('.bet-review-table tbody tr').first()).toContainText('48.99')
    const preset = page.viewportSize()!
    for (const width of info.project.name.includes('reflow') ? [320, 400, 620, 700, 1000, 1440] : [preset.width]) {
      if (info.project.name.includes('reflow')) await page.setViewportSize({ width, height: width === 620 ? 480 : 800 })
      await expectInsideViewport(dialog)
      await expectRealEntry(page,'user');await expectListLayout(page)
      const measurements = await dialog.evaluate(node => ({
        overflow: node.scrollWidth > node.clientWidth + 1,
        contentOverflow: [...node.querySelectorAll('table,td,dd')].some(element => element.scrollWidth > element.clientWidth + 1),
        smallText: [...node.querySelectorAll('td,dd,dt,p')].some(element => parseFloat(getComputedStyle(element).fontSize) < 14),
      }))
      expect(measurements).toEqual({ overflow: false, contentOverflow: false, smallText: false })
    }
    await page.screenshot({ path: info.outputPath(`notification-bet-${locale}.png`), fullPage: true })
    await dialog.locator('.bet-review-table tbody tr').last().scrollIntoViewIfNeeded()
    await expectInsideViewport(dialog.locator('.bet-review-table tbody tr').last())
    await dialog.locator('.detail-modal-close').click()
    await expect(dialog).toHaveCount(0)
    expect(errors).toEqual([])
  })
}
