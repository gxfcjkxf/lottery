import { test, expect } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { configuredDemo } from '../../shared/src/configured-rules'
import { installLayoutApi, installOrdersLayoutApi, origins, gameId, expectNoPageOverflow } from './fixture'

for (const locale of ['en', 'zh-CN'] as const) {
  test(`one header title, one game picker and readable bet table (${locale})`, async ({ page }, info) => {
    await installLayoutApi(page, locale)
    await installOrdersLayoutApi(page)
    const snapshot = configuredDemo(JSON.parse(readFileSync('backend/internal/rules/config/mark_six_v2.json', 'utf8')))
    const play = snapshot.catalog.plays[0]!, option = snapshot.catalog.domains[play.selection.domain]!.options[0]!
    const id = 'aaaaaaaa-aaaa-4aaa-8aaa-000000000009'
    await page.route('**/api/v1/betting-history?*', route => {
      expect(route.request().method()).toBe('GET')
      return route.fulfill({ json: { success: true, data: {
        brand_id: gameId, member_id: gameId, game_id: new URL(route.request().url()).searchParams.get('game_id') ?? '',
        games: [{ id: gameId, name: 'Mark Six' }], limit: 20, total_count: '1', next_cursor: null,
        items: [{ kind: 'configured', id, game_id: gameId, game_name: 'Mark Six', period_no: '2026115', placed_at: '2026-10-27T03:20:52Z', order: {
          id, game_id: gameId, period_no: '2026115', status: 'placed', snapshot,
          quote: { bet_points: '40', lines: [{ play_id: play.id, options: [option.id], plate: 'A', stake: '40', prices: { [option.id]: '48.99' } }] }, result: null,
        } }],
      } } })
    })
    const errors: string[] = []
    page.on('pageerror', error => errors.push(error.message))
    await page.goto(`${origins.user}/orders`)
    const preset = page.viewportSize()!
    for (const width of info.project.name.includes('reflow') ? [320, 700, 701, 999, 1000, 1001, 1440] : [preset.width]) {
      if (info.project.name.includes('reflow')) await page.setViewportSize({ width, height: 800 })
      const title = page.locator('.workspace-page-title')
      await expect(title).toHaveText(locale === 'en' ? 'Betting history' : '投注记录')
      await expect(page.locator('h1')).toHaveCount(1)
      const back = await page.locator('.workspace-leading button').boundingBox(), heading = await title.boundingBox()
      expect(heading!.x).toBeGreaterThanOrEqual(back!.x + back!.width)
      await expect(page.locator('.workspace-game-picker .select-control-trigger')).toHaveCount(1)
      const picker = page.locator('.workspace-game-picker'), trigger = picker.getByRole('combobox')
      const header = page.locator('.workspace-topbar'), before = await header.boundingBox()
      await trigger.click()
      await expect(picker.getByRole('listbox')).toBeVisible()
      await expect(picker.getByRole('listbox')).toHaveCSS('position', 'fixed')
      expect(await header.boundingBox()).toEqual(before)
      await expectNoPageOverflow(page)
      await page.screenshot({ path: info.outputPath(`game-picker-${locale}-${width}.png`) })
      await trigger.press('ArrowDown'); await trigger.press('Escape')
      await expect(picker.getByRole('listbox')).toBeHidden()
      await expect(trigger).toBeFocused()
      await trigger.click(); await title.click()
      await expect(picker.getByRole('listbox')).toBeHidden()
      await expect(page.locator('main select, main details, main h1')).toHaveCount(0)
      await expect(page.locator('main')).not.toContainText('Mark Six')
      await expect(page.locator('main')).not.toContainText(id)
      await expect(page.locator('.history-table tbody')).toContainText('2026115')
      await expect(page.locator('.history-table tbody')).toContainText('48.99')
      await expect(page.locator('.history-table td[data-label="'+(locale==='en'?'Prize points':'中奖积分')+'"]')).toHaveText('—')
      await expect(page.locator('.history-content')).not.toContainText('48.99')
      await expect(page.locator('.history-odds')).toHaveText('48.99')
      await expect(page.locator('.history-status')).toHaveCSS('border-width', '0px')
      await expect(page.locator('.history-status')).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
      if (width > 700) {
        await expect(page.locator('.history-table')).toHaveCSS('table-layout', 'auto')
        const time = page.locator('.history-table time')
        await expect(time).toHaveCSS('white-space', 'nowrap')
        expect(await time.evaluate(node => { const range = document.createRange(); range.selectNodeContents(node); return range.getClientRects().length })).toBe(1)
      }
      if (width >= 1000) {
        const layout = await page.locator('.history-table').evaluate(table => {
          const content = document.querySelector('main.content')!, style = getComputedStyle(content)
          const row = table.querySelector('tbody tr')!
          const centers = [...row.querySelectorAll('td')].map(cell => {
            const range = document.createRange(); range.selectNodeContents(cell)
            const rect = range.getBoundingClientRect(); return rect.y + rect.height / 2
          })
          return { width: table.getBoundingClientRect().width, available: content.clientWidth - parseFloat(style.paddingLeft) - parseFloat(style.paddingRight), centers }
        })
        expect(Math.abs(layout.width - layout.available)).toBeLessThanOrEqual(2)
        expect(Math.max(...layout.centers) - Math.min(...layout.centers)).toBeLessThan(2)
      }
      await expectNoPageOverflow(page)
      if (width <= 700 || width <= 900 && await page.evaluate(() => matchMedia('(pointer: coarse)').matches)) {
        const cells = await page.locator('.history-table tbody tr').first().locator('td:visible').evaluateAll(nodes => nodes.map(node => { const r = node.getBoundingClientRect(); return { x: r.x, y: r.y, right: r.right, bottom: r.bottom } }))
        expect(cells[1]!.y).toBeGreaterThanOrEqual(cells[0]!.bottom)
        expect(cells[2]!.y).toBeGreaterThanOrEqual(cells[1]!.bottom)
        for (let i = 0; i < cells.length; i++) for (let j = i + 1; j < cells.length; j++) {
          const a = cells[i]!, b = cells[j]!
          expect(a.right <= b.x || b.right <= a.x || a.bottom <= b.y || b.bottom <= a.y).toBe(true)
        }
        await page.screenshot({ path: info.outputPath(`bet-mobile-${locale}-${width}.png`), fullPage: true })
      }
    }
    await page.screenshot({ path: info.outputPath(`bet-table-${locale}.png`), fullPage: true })
    const gamePicker=page.locator('.workspace-game-picker').getByRole('combobox')
    await gamePicker.press('Home');await gamePicker.press('Enter')
    await expect(page.locator('.history-table td[data-label="'+(locale==='en'?'Game':'彩种')+'"]')).toHaveText('Mark Six')
    await gamePicker.press('End');await gamePicker.press('Enter')
    await expect(page.locator('.history-table td[data-label="'+(locale==='en'?'Game':'彩种')+'"]')).toHaveCount(0)
    await expect(page).toHaveURL(/\/orders$/)
    for (const [path, title] of [['/help', locale === 'en' ? 'Help centre' : '帮助中心'], ['/account', locale === 'en' ? 'Me' : '个人'], ['/results', locale === 'en' ? 'Draw history' : '开奖记录']]) {
      await page.goto(`${origins.user}${path}`)
      await expect(page.locator('.workspace-page-title')).toHaveText(title)
      await expect(page.locator('h1')).toHaveCount(1)
      await expectNoPageOverflow(page)
    }
    expect(errors).toEqual([])
  })
}

for (const locale of ['en', 'zh-CN'] as const) {
  test(`bet content stays unchanged after settlement and extra amounts use columns (${locale})`, async ({ page }, info) => {
    await installLayoutApi(page, locale)
    await installOrdersLayoutApi(page)
    const snapshot = configuredDemo(JSON.parse(readFileSync('backend/internal/rules/config/mark_six_v2.json', 'utf8')))
    const play = snapshot.catalog.plays[0]!, option = snapshot.catalog.domains[play.selection.domain]!.options[0]!
    let settled = false, extraAmounts = false
    await page.route('**/api/v1/betting-history?*', route => {
      expect(route.request().method()).toBe('GET')
      const outcomes = extraAmounts ? ['lost', 'won', 'push', 'lost'] : ['lost']
      return route.fulfill({ json: { success: true, data: {
        brand_id: gameId, member_id: gameId, game_id: gameId,
        games: [{ id: gameId, name: 'Mark Six' }], limit: 20, total_count: String(outcomes.length), next_cursor: null,
        items: outcomes.map((outcome, index) => ({ kind: 'configured', id: `ticket-${index}`, game_id: gameId, game_name: 'Mark Six', period_no: '2026115', placed_at: '2026-10-27T03:20:52Z', order: {
          id: `ticket-${index}`, game_id: gameId, period_no: '2026115', status: settled ? 'settled' : 'placed',
          snapshot: index === 3 ? { ...snapshot, catalog: { ...snapshot.catalog, funding: 'reserve' } } : snapshot,
          quote: { bet_points: '40', ...(index === 3 ? { reserved_points: '120' } : {}), lines: [{ play_id: play.id, options: [option.id], plate: 'A', stake: '40', prices: { [option.id]: '48.99' } }] },
          result: settled ? { prize_points: outcome === 'won' ? '1960' : '0', refund_points: outcome === 'push' ? '40' : index === 3 ? '80' : '0', ...(index === 3 ? { consumed_points: '40' } : {}), lines: [{ outcome, prize_points: outcome === 'won' ? '1960' : '0' }] } : null,
        } })),
      } } })
    })
    await page.goto(`${origins.user}/orders`)
    await expect(page.locator('.history-content')).not.toBeEmpty()
    const content = await page.locator('.history-content').innerText()
    settled = true
    await page.reload()
    await expect(page.locator('.history-content')).toHaveText(content)
    await expect(page.locator('.history-table th')).toHaveCount(8)
    await expect(page.locator('.history-table td[data-label="'+(locale==='en'?'Prize points':'中奖积分')+'"]')).toHaveText('0')
    await expect(page.locator('.history-table')).not.toContainText(locale === 'en' ? 'Refund points' : '退本积分')
    extraAmounts = true
    await page.reload()
    await expect(page.locator('.history-table tbody tr')).toHaveCount(4)
    const refundLabel = locale === 'en' ? 'Refund points' : '退本积分'
    await expect(page.locator('.history-table th').filter({ hasText: refundLabel })).toHaveCount(1)
    for (const [index, prize, refund] of [[0, '0', '—'], [1, '1960', '—'], [2, '0', '40'], [3, '0', '—']] as const) {
      const row = page.locator('.history-table tbody tr').nth(index)
      await expect(row.locator('.history-content')).toHaveText(content)
      await expect(row.locator('td[data-label="'+(locale==='en'?'Prize points':'中奖积分')+'"]')).toHaveText(prize)
      await expect(row.locator(`td[data-label="${refundLabel}"]`)).toHaveText(refund)
    }
    const preset = page.viewportSize()!
    for (const width of info.project.name.includes('reflow') ? [320, 700, 701, 999, 1000, 1001, 1440] : [preset.width]) {
      if (info.project.name.includes('reflow')) await page.setViewportSize({ width, height: 800 })
      if (width <= 700 || width <= 900 && await page.evaluate(() => matchMedia('(pointer: coarse)').matches)) {
        await expect(page.locator('.history-amount-empty').first()).toBeHidden()
        for (const row of await page.locator('.history-table tbody tr').all()) {
          const cells = await row.locator('td:visible').evaluateAll(nodes => nodes.map(node => { const r = node.getBoundingClientRect(); return { x: r.x, y: r.y, right: r.right, bottom: r.bottom } }))
          for (let i = 0; i < cells.length; i++) for (let j = i + 1; j < cells.length; j++) {
            const a = cells[i]!, b = cells[j]!
            expect(a.right <= b.x || b.right <= a.x || a.bottom <= b.y || b.bottom <= a.y).toBe(true)
          }
        }
      }
      await expectNoPageOverflow(page)
    }
    await page.screenshot({ path: info.outputPath(`bet-settlement-${locale}.png`), fullPage: true })
  })
}
