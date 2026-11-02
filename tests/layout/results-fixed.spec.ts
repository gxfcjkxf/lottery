import { test, expect } from '@playwright/test'
import { installLayoutApi, installOrdersLayoutApi, origins, gameId, expectNoPageOverflow } from './fixture'

for (const locale of ['zh-CN', 'en'] as const) {
  test(`results controls stay visible while records scroll (${locale})`, async ({ page }, info) => {
    await installLayoutApi(page, locale)
    await installOrdersLayoutApi(page)
    await page.route('**/api/v1/draw-archive?*', route => {
      const url = new URL(route.request().url()), offset = Number(url.searchParams.get('offset'))
      return route.fulfill({ json: { success: true, data: {
        game_id: url.searchParams.get('game_id'), game_name: 'Mark Six', mark_six: true,
        timezone: 'Asia/Singapore', daily: false, choices: ['2026', '2025'], selected: '2026', has_more: offset === 0,
        items: Array.from({ length: 50 }, (_, index) => ({
          period_no: String(2026050 - offset - index), draw_date: '2026-10-27', drawn_at: null,
          result: { regular: [1, 2, 3, 4, 5, 6], special: [7], digits: [] }, regular_order: [1, 2, 3, 4, 5, 6],
          attributes: Object.fromEntries([1, 2, 3, 4, 5, 6, 7].map(number => [String(number).padStart(2, '0'), { wave: 'red', zodiac: 'rat', element: 'wood' }])),
        })),
       total_count: '100'} } })
    })
    await page.goto(`${origins.user}/results?game_id=${gameId}`)
    const list = page.locator('.archive-list'), toolbar = page.locator('.archive-toolbar'), pagination = page.locator('.draw-archive .page-actions')
    await expect(list.locator('.archive-row')).toHaveCount(50)
    const preset = page.viewportSize()!
    for (const width of info.project.name.includes('reflow') ? [320, 620, 999, 1000, 1001, 1440] : [preset.width]) {
      if (info.project.name.includes('reflow')) await page.setViewportSize({ width, height: 800 })
      await list.evaluate(node => { node.scrollTop = 0 })
      const top = (await toolbar.boundingBox())!, bottom = (await pagination.boundingBox())!, content = (await list.boundingBox())!
      expect(content.height).toBeGreaterThanOrEqual(80)
      await toolbar.evaluate(node => { node.scrollTop = node.scrollHeight })
      await expect(toolbar.getByLabel(locale === 'en' ? 'Number order' : '大小顺序', { exact: true })).toBeInViewport()
      await toolbar.evaluate(node => { node.scrollTop = 0 })
      expect(content.y).toBeGreaterThanOrEqual(top.y + top.height)
      expect(content.y + content.height).toBeLessThanOrEqual(bottom.y)
      expect(bottom.y + bottom.height).toBeLessThanOrEqual(page.viewportSize()!.height)
      const nav = page.locator('.mobile-primary-nav')
      if (await nav.isVisible()) expect(bottom.y + bottom.height).toBeLessThanOrEqual((await nav.boundingBox())!.y)
      const columns = await list.locator('.archive-row').first().evaluate(row => {
        const period = row.querySelector('.archive-period')!.getBoundingClientRect(), date = row.querySelector('.archive-time')!.getBoundingClientRect(), result = row.querySelector('.archive-result')!.getBoundingClientRect()
        const balls = [...row.querySelectorAll('.archive-ball')].map(ball => ball.getBoundingClientRect())
        return { period: { left:period.left,right: period.right, top: period.top, bottom: period.bottom }, date: { left: date.left, right: date.right, top: date.top }, result: { left: result.left, top: result.top }, fits: balls.every(ball => ball.left >= result.left - 1 && ball.right <= row.getBoundingClientRect().right + 1), aligned: balls.every(ball => Math.abs(ball.top - balls[0]!.top) < 1) }
      })
      if(width<1000){
        expect(columns.date.left).toBe(columns.period.left)
        expect(columns.date.top).toBeGreaterThanOrEqual(columns.period.bottom)
        expect(columns.result.left).toBeGreaterThan(Math.max(columns.date.right,columns.period.right))
      }else{
        expect(columns.date.left).toBeGreaterThan(columns.period.right)
        expect(columns.result.left).toBeGreaterThan(columns.date.right)
        expect(columns.date.top).toBeLessThan(columns.period.bottom)
      }
      expect(columns.result.top).toBeLessThan(columns.period.bottom)
      expect(columns.fits).toBe(true)
      expect(columns.aligned).toBe(true)
      await list.evaluate(node => { node.scrollTop = node.scrollHeight })
      expect(await list.evaluate(node => node.scrollTop)).toBeGreaterThan(0)
      expect((await toolbar.boundingBox())!.y).toBe(top.y)
      expect((await pagination.boundingBox())!.y).toBe(bottom.y)
      await expect(list.locator('.archive-row').last()).toBeInViewport()
      await expectNoPageOverflow(page)
    }
    await page.screenshot({ path: info.outputPath(`results-fixed-${locale}.png`) })
    await pagination.getByRole('button', { name: locale === 'en' ? 'Next' : '下一页', exact: true }).click()
    await expect(list.locator('.archive-period').first()).toHaveText('2026000')
    expect(await list.evaluate(node => node.scrollTop)).toBe(0)
  })
}
