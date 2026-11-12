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
    await expect(dialog.locator('.notification-bet-summary')).toContainText(locale === 'en' ? 'Placed at' : '投注时间')
    await expect(dialog).not.toContainText(orderId)
    await expect(dialog.locator('.bet-review-table tbody tr')).toHaveCount(5)
    await expect(dialog.locator('.bet-review-table td').first()).toHaveCSS('padding-left','8px')
    await expect(dialog.locator('.bet-review-table td').last()).toHaveCSS('padding-right','8px')
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

for (const locale of ['en', 'zh-CN'] as const) {
  test(`compact notification records without notification timestamps (${locale})`, async ({ page }, info) => {
    await installLayoutApi(page, locale)
    await installOrdersLayoutApi(page)
    const snapshot = configuredDemo(JSON.parse(readFileSync('backend/internal/rules/config/mark_six_v2.json', 'utf8')))
    const play = snapshot.catalog.plays.find(play => play.id === 'special_numbers')!
    const cases = [
      ['bet.order.won', '489.9', '+489.9'],
      ['bet.order.prize_reversed', '489.9', '-489.9'],
      ['recharge.confirmed', '100', '+100'],
      ['reward.order.granted', '50', '+50'],
      ['reward.order.revoked', '50', '-50'],
      ['commission.paid', '20', '+20'],
      ['commission.corrected', '-12.5', '-12.5'],
      ['withdrawal.order.reviewing', '10', '10'],
      ['withdrawal.order.paid', '10', '10'],
      ['draw.result.corrected', null, null],
    ] as const
    const orderId = 'aaaaaaaa-aaaa-4aaa-8aaa-000000000042'
    await page.route('**/api/v1/notifications?*', route => route.fulfill({ json: { success: true, data: {
      brand_id: gameId, member_id: gameId, unread_count: '0', total_count: String(cases.length), limit: 20, offset: 0,
      items: cases.map(([event, points], index) => ({
        id: `aaaaaaaa-aaaa-4aaa-8aaa-${String(index + 100).padStart(12, '0')}`, brand_id: gameId, member_id: gameId,
        event_type: event, template_key: event, template_version: 1, content: { i18n_key: event, i18n_version: 1 },
        payload: { resource_id: orderId, points, ...(event.startsWith('draw.') ? { draw: {
          game_id: gameId, period_id: gameId, period_no: '2026310', result: { regular: [], special: [], digits: [0, 1, 0] },
          drawn_at: '2026-11-06T12:00:00Z', previous_draw_id: gameId,
        } } : {}) }, created_at: '2026-11-06T14:32:27Z', read_at: '2026-11-06T14:33:00Z',
      })),
    } } }))
    await page.route(`**/api/v1/configured/orders/${orderId}`, route => route.fulfill({ json: { success: true, data: {
      brand_id: gameId, member_id: gameId, order: { id: orderId, game_id: gameId, game_name: locale === 'en' ? 'New Macau Mark Six' : '新澳门六合彩',
        period_no: '2026310', status: 'settled', placed_at: '2026-11-06T10:40:07Z', snapshot,
        quote: { bet_points: '10', lines: [{ play_id: play.id, options: ['47'], plate: 'A', stake: '10', prices: { win: '48.99' } }] }, result: { prize_points: '489.9' },
      },
    } } }))
    await page.goto(`${origins.user}/notifications`)
    await expect(page.locator('.notification-summary')).toHaveCount(cases.length)
    await expect(page.locator('.notification-summary time')).toHaveCount(0)
    const preset = page.viewportSize()!
    const widths = info.project.name.includes('reflow') ? [320, 560, 700, 1440] : [preset.width]
    for (const width of widths) {
      if (info.project.name.includes('reflow')) await page.setViewportSize({ width, height: 800 })
      for (const [index, [event, , amount]] of cases.entries()) {
        await page.locator('.notification-summary').nth(index).click()
        const dialog = page.getByRole('dialog')
        await expect(dialog).toHaveClass(/notification-dialog/)
        await expect(dialog).toHaveAttribute('aria-label', notificationCatalog[event][locale].title)
        await expect(dialog.locator('.notification-body')).toHaveCount(0)
        await expect(dialog).not.toContainText('22:32:27')
        await expect(dialog).not.toContainText(orderId)
        await expect(dialog).not.toContainText(/历史记录|此记录会保留|Historical record|this record is retained/)
        if (amount) await expect(dialog.locator('.notification-amount strong')).toHaveText(amount)
        else await expect(dialog).toContainText('010')
        if (event.startsWith('bet.')) await expect(dialog.locator('.bet-review-table')).toContainText('48.99')
        await expectInsideViewport(dialog)
        const measure = await dialog.evaluate(node => ({
          width: node.getBoundingClientRect().width,
          overflow: [...node.querySelectorAll('table,td,dd,.notification-detail')].some(element => element.scrollWidth > element.clientWidth + 1),
          pageOverflow: document.documentElement.scrollWidth > innerWidth + 1,
        }))
        expect(measure.width).toBeLessThanOrEqual(640)
        expect(measure.overflow).toBe(false)
        expect(measure.pageOverflow).toBe(false)
        if (width === widths[widths.length - 1] && [0, 6, 9].includes(index)) await page.screenshot({ path: info.outputPath(`notification-${index}-${locale}.png`), fullPage: true })
        await dialog.locator('.detail-modal-close').click()
        await expect(dialog).toHaveCount(0)
      }
    }
  })
}

for (const locale of ['en', 'zh-CN'] as const) {
  test(`unread badges follow message receipts (${locale})`, async ({ page }, info) => {
    await installLayoutApi(page, locale)
    await installOrdersLayoutApi(page)
    let unread = '1'
    const id = 'aaaaaaaa-aaaa-4aaa-8aaa-000000000043'
    await page.route('**/api/v1/notifications?*', route => {
      const query = new URL(route.request().url()).searchParams
      return route.fulfill({ json: { success: true, data: {
        brand_id: gameId, member_id: gameId, unread_count: unread, total_count: '1', limit: Number(query.get('limit')), offset: Number(query.get('offset')),
        items: [{ id, brand_id: gameId, member_id: gameId, event_type: 'member.joined', template_key: 'member.joined', template_version: 1,
          content: notificationCatalog['member.joined'], payload: { resource_id: gameId, points: null }, created_at: '2026-10-28T14:14:05Z', read_at: unread === '0' ? '2026-10-28T14:15:00Z' : null }],
      } } })
    })
    await page.route('**/api/v1/notifications/read', route => {
      unread = '0'
      return route.fulfill({ json: { success: true, data: { brand_id: gameId, member_id: gameId, ids: [id], changed: 1, unread_count: '0' } } })
    })
    await page.goto(`${origins.user}/announcements`)
    const tab = page.getByRole('tab', { name: locale === 'en' ? /^Messages/ : /^消息/ })
    await expect(tab.locator('.unread-badge')).toHaveText('1')
    await expect(page.locator('.workspace-menu-unread-dot')).toBeVisible()
    await expect(page.locator('.workspace-menu-trigger')).toHaveAccessibleName(locale === 'en' ? 'Menu, 1 unread notifications' : '菜单, 1 条未读消息')
    await page.locator('.workspace-trailing button').last().click()
    await expect(page.locator('.workspace-menu a[href="/announcements"] .unread-badge')).toHaveText('1')
    await page.locator('.workspace-menu-close').click()
    await page.screenshot({ path: info.outputPath(`unread-${locale}.png`), fullPage: true })
    await tab.click()
    await expect(page.locator('.notification-summary')).toBeVisible()
    await expect(tab.locator('.unread-badge')).toHaveText('1')
    await page.locator('.notification-summary').click()
    await expect(page.locator('.unread-badge')).toHaveCount(0)
    await expect(page.locator('.workspace-menu-unread-dot')).toHaveCount(0)
    await page.locator('.detail-modal-close').click()
    await page.getByRole('tab', { name: locale === 'en' ? 'Announcements' : '公告', exact: true }).click()
    await expect(page.locator('.unread-badge')).toHaveCount(0)
    const preset = page.viewportSize()!
    for (const width of info.project.name.includes('reflow') ? [320, 640, 1000, 1440] : [preset.width]) {
      if (info.project.name.includes('reflow')) await page.setViewportSize({ width, height: 800 })
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1)).toBe(true)
    }
  })
}
