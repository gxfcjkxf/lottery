import { expect, test } from '@playwright/test'
import { gameId, origins, installLayoutApi, expectNoPageOverflow } from './fixture'
import { selectControl } from '../select-control'

for (const locale of ['zh-CN', 'en'] as const) {
  test(`brand special number distribution (${locale})`, async ({ page }, info) => {
    const zh = locale === 'zh-CN'
    await installLayoutApi(page, locale)
    const games = ['香港六合彩', '澳门六合彩', '新澳门六合彩'].map((name, i) => ({ id: gameId.slice(0, -1) + (i + 1), name }))
    const queries: URLSearchParams[] = []
    const errors: string[] = []
    page.on('pageerror', error => errors.push(error.message))
    await page.route('**/api/v1/admin/**', async route => {
      expect(route.request().method()).toBe('GET')
      const url = new URL(route.request().url()), path = url.pathname
      const reply = (data: unknown) => route.fulfill({ json: { success: true, data } })
      if (path.endsWith('/me')) return reply({ account: { id: gameId, username: 'distribution_viewer', super_admin: false, brand_ids: [gameId], permissions: [], permissions_by_brand: { [gameId]: ['bet.view.brand'] }, platform_permissions: [] } })
      if (path.endsWith('/brands')) return reply({ items: [{ id: gameId, code: 'layout', name: 'Layout trial', status: 'active', member_count: '0', agent_count: '0', recharge_quota_available: '0', created_at: '2026-10-24T00:00:00Z' }] })
      if (path.endsWith('/bet-distribution/games')) return reply({ games })
      if (path.endsWith('/bet-distribution')) {
        expect(route.request().headers()['x-brand-id']).toBe(gameId)
        queries.push(url.searchParams)
        if (url.searchParams.get('period_no') === 'missing') return route.fulfill({ status: 404, json: { success: false, error: { code: 'BET_ORDER_NOT_FOUND', message: 'Not found' } } })
        return reply({ game_id: url.searchParams.get('game_id'), period_no: url.searchParams.get('period_no') || '20261103', total_stake: '510.75', numbers: Array.from({ length: 49 }, (_, i) => ({ number: i + 1, wave: i === 48 ? 'green' : ['red', 'blue', 'green'][i % 3], zodiac: ['rat', 'ox', 'tiger', 'rabbit', 'dragon', 'snake', 'horse', 'goat', 'monkey', 'rooster', 'dog', 'pig'][i % 12], stake: i % 12 === 0 ? '102.25' : '0.00', profit: url.searchParams.get('period_no') === 'zero' ? '0.00' : i === 4 ? '100.10' : i === 5 ? '509.60' : i === 25 ? '-100.10' : i === 12 ? '-1800.00' : i === 24 ? '-1600.00' : i % 12 === 0 ? '-2040.50' : i === 1 ? '700.49' : i === 2 ? '600.50' : '510.00' })) })
      }
      throw Error(`Unexpected distribution request: ${path}`)
    })
    await page.goto(origins.brand)
    await expect(page.locator('.management-console')).toBeVisible()
    const more = page.locator('.mobile-nav > button').last()
    const mobile = await more.isVisible()
    if (mobile) await more.click()
    const menu = page.locator(mobile ? '.mobile-more-menu' : '.side-nav')
    await menu.getByRole('button', { name: zh ? '注单分布' : 'Bet distribution', exact: true }).click()
    const cards = page.locator('.distribution-number')
    await expect(cards).toHaveCount(49)
    await expect(page.getByRole('heading', { name: zh ? '特码开奖盈利预测' : 'Special number draw profit forecast', exact: true })).toBeVisible()
    await expect(cards.first().locator('dt').nth(0)).toHaveText(zh ? '投注' : 'Stake')
    await expect(cards.first().locator('dt').nth(1)).toHaveText(zh ? '输赢' : 'Profit / loss')
    const thirdWinGroup = Array.from({ length: 49 }, (_, i) => i + 1).filter(n => ![1, 2, 3, 5, 13, 25, 26, 37, 49].includes(n))
    for (const [side, groups] of [['win', [[2], [3], thirdWinGroup]], ['loss', [[1, 37, 49], [13], [25]]]] as const) {
      const colors: string[] = []
      for (const [index, numbers] of groups.entries()) {
        const group = page.locator(`.distribution-top-${side}-${index + 1}`)
        await expect(group.locator('h3')).toHaveText(numbers.map(n => String(n).padStart(2, '0')))
        const backgrounds = await group.evaluateAll(nodes => nodes.map(node => getComputedStyle(node).backgroundColor))
        expect(new Set(backgrounds).size).toBe(1)
        colors.push(backgrounds[0]!)
      }
      expect(new Set(colors).size).toBe(3)
    }
    await expect(cards.nth(4)).not.toHaveClass(/distribution-top-/)
    await expect(cards.nth(25)).not.toHaveClass(/distribution-top-/)
    await expect(cards.first().locator('dd').first()).toHaveText('102')
    await expect(page.locator('.distribution-total strong')).toHaveText('511')
    await expect(cards.first().locator('.distribution-zodiac')).toHaveText(zh ? '鼠' : 'Rat')
    await expect(cards.nth(2).locator('.distribution-win')).toHaveText('+601')
    const waveColors: string[] = []
    for (const [index, wave] of ['red', 'blue', 'green'].entries()) {
      const number = cards.nth(index).locator('h3')
      await expect(number).toHaveClass(`lottery-wave-${wave}`)
      waveColors.push(await number.evaluate(node => getComputedStyle(node).backgroundColor))
    }
    expect(new Set(waveColors).size).toBe(3)
    await expect(cards.first().locator('.distribution-loss')).toHaveText('-2,041')
    await expect(cards.nth(1).locator('.distribution-win')).toHaveText('+700')
    await expect(page.getByRole('searchbox', { name: zh ? '期次' : 'Period', exact: true })).toHaveValue('20261103')
    const preset = page.viewportSize()!
    const sizes = info.project.name.includes('reflow') ? [{ width: 320, height: 568 }, { width: 768, height: 1024 }, { width: 1440, height: 900 }] : [preset]
    for (const size of sizes) {
      if (info.project.name.includes('reflow')) await page.setViewportSize(size)
      const readsBeforeSort = queries.length
      const sortControl = page.getByRole('group', { name: zh ? '排序' : 'Sort', exact: true })
      for (const [value, firstNumbers] of [['lossToWin', ['01', '37', '49']], ['winToLoss', ['02', '03', '04']], ['number', ['01', '02', '03']]] as const) {
        const label = value === 'number' ? (zh ? '号码' : 'Number') : value === 'lossToWin' ? (zh ? '输→赢' : 'Loss → Win') : (zh ? '赢→输' : 'Win → Loss')
        await sortControl.getByRole('button', { name: label, exact: true }).click()
        await expect(sortControl.getByRole('button', { pressed: true })).toHaveText(label)
        await expect(sortControl.getByRole('button')).toHaveCount(3)
        await expect.poll(async () => (await cards.locator('h3').allTextContents()).slice(0, 3)).toEqual([...firstNumbers])
        await expect(page.locator('.distribution-top-win-1 h3')).toHaveText('02')
        await expect.poll(async () => (await page.locator('.distribution-top-loss-1 h3').allTextContents()).sort()).toEqual(['01', '37', '49'])
      }
      expect(queries.length).toBe(readsBeforeSort)
      await expectNoPageOverflow(page)
      await page.screenshot({ path: info.outputPath(`distribution-${locale}-${size.width}.png`), fullPage: true })
    }
    for (const g of games.slice(1)) {
      await selectControl(page.getByRole('combobox', { name: zh ? '彩种' : 'Game', exact: true }), { value: g.id })
      await expect.poll(() => queries.at(-1)?.get('game_id')).toBe(g.id)
      expect(queries.at(-1)?.get('period_no')).toBe('')
      await expect(cards).toHaveCount(49)
    }
    const period = page.getByRole('searchbox', { name: zh ? '期次' : 'Period', exact: true })
    await period.fill('20261031')
    await page.getByRole('button', { name: zh ? '查询' : 'Search', exact: true }).click()
    await expect.poll(() => queries.at(-1)?.get('period_no')).toBe('20261031')
    await expect(cards).toHaveCount(49)
    await period.fill('zero')
    await page.getByRole('button', { name: zh ? '查询' : 'Search', exact: true }).click()
    await expect(cards.first().locator('dd').nth(1)).toHaveText('0')
    await expect(page.locator('[class*="distribution-top-"]')).toHaveCount(0)
    await period.fill('missing')
    await page.getByRole('button', { name: zh ? '查询' : 'Search', exact: true }).click()
    await expect(page.locator('.bet-distribution [role="alert"]')).toBeVisible()
    await expect(cards).toHaveCount(0)
    await period.fill('20261103')
    await page.getByRole('button', { name: zh ? '查询' : 'Search', exact: true }).click()
    await expect(cards).toHaveCount(49)
    expect(errors).toEqual([])
  })
}
