import { expect, test } from '@playwright/test'
import { gameId, origins, installLayoutApi, expectNoPageOverflow } from './fixture'
import { selectControl } from '../select-control'

for (const locale of ['zh-CN', 'en'] as const) {
  test(`brand special number distribution (${locale})`, async ({ page }, info) => {
  test.setTimeout(120_000)
    const zh = locale === 'zh-CN'
    await installLayoutApi(page, locale)
    const games = ['香港六合彩', '澳门六合彩', '新澳门六合彩', '加拿大28'].map((name, i) => ({ id: gameId.slice(0, -1) + (i + 1), name }))
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
        if (url.searchParams.get('game_id') === games[3]!.id) return reply({ number_digits: 3, game_id: games[3]!.id, period_no: url.searchParams.get('period_no') || '20261103', total_stake: url.searchParams.get('period_no') === 'zero' ? '0.00' : '1000.00', numbers: Array.from({ length: 1000 }, (_, n) => ({ number: n, wave: '', zodiac: '', stake: n / 100 + Math.floor(n / 10) % 10 + n % 10 >= 14 ? '2.00' : '0.00', profit: url.searchParams.get('period_no') === 'zero' ? '0.00' : n === 149 ? '-1500.00' : n === 148 ? '199.99' : n === 147 ? '200.00' : n === 146 ? '200.01' : n === 145 ? '0.00' : n === 144 ? '-0.01' : '1000.00' })) })
        return reply({ number_digits: 2, game_id: url.searchParams.get('game_id'), period_no: url.searchParams.get('period_no') || '20261103', total_stake: '510.75', numbers: Array.from({ length: 49 }, (_, i) => ({ number: i + 1, wave: i === 48 ? 'green' : ['red', 'blue', 'green'][i % 3], zodiac: ['rat', 'ox', 'tiger', 'rabbit', 'dragon', 'snake', 'horse', 'goat', 'monkey', 'rooster', 'dog', 'pig'][i % 12], stake: i % 12 === 0 ? '102.25' : '0.00', profit: url.searchParams.get('period_no') === 'zero' ? '0.00' : i === 4 ? '100.10' : i === 5 ? '509.60' : i === 25 ? '-100.10' : i === 12 ? '-1800.00' : i === 24 ? '-1600.00' : i % 12 === 0 ? '-2040.50' : i === 1 ? '700.49' : i === 2 ? '600.50' : '510.00' })) })
      }
      throw Error(`Unexpected distribution request: ${path}`)
    })
    await page.goto(origins.brand)
    await expect(page.locator('.management-console')).toBeVisible()
    const more = page.locator('.mobile-nav > button').last()
    const mobile = await more.isVisible()
    if (mobile) await more.click()
    const menu = page.locator(mobile ? '.mobile-more-menu' : '.side-nav')
    await menu.locator('summary').filter({ hasText: zh ? '投注管理' : 'Betting' }).click()
    await menu.getByRole('button', { name: zh ? '注单分布' : 'Bet distribution', exact: true }).click()
    const cards = page.locator('.distribution-number')
    await expect(cards).toHaveCount(49)
    await expect(page.getByRole('group', { name: zh ? '返奖率' : 'Payout rate', exact: true })).toHaveCount(0)
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
      const sortControl = page.getByRole('combobox', { name: zh ? '排序' : 'Sort', exact: true })
      for (const [value, firstNumbers] of [['lossToWin', ['01', '37', '49']], ['winToLoss', ['02', '03', '04']], ['number', ['01', '02', '03']]] as const) {
        const label = value === 'number' ? (zh ? '号码' : 'Number') : value === 'lossToWin' ? (zh ? '输→赢' : 'Loss → Win') : (zh ? '赢→输' : 'Win → Loss')
        await selectControl(sortControl, { label })
        await expect(sortControl.locator('.select-control-value > span:last-child')).toHaveText(label)
        await expect.poll(async () => (await cards.locator('h3').allTextContents()).slice(0, 3)).toEqual([...firstNumbers])
        await expect(page.locator('.distribution-top-win-1 h3')).toHaveText('02')
        await expect.poll(async () => (await page.locator('.distribution-top-loss-1 h3').allTextContents()).sort()).toEqual(['01', '37', '49'])
      }
      expect(queries.length).toBe(readsBeforeSort)
      const gameBox = await page.getByRole('combobox', { name: zh ? '彩种' : 'Game', exact: true }).boundingBox()
      const periodBox = await page.getByRole('searchbox', { name: zh ? '期次' : 'Period', exact: true }).boundingBox()
      expect(Math.abs(gameBox!.y - periodBox!.y)).toBeLessThanOrEqual(1)
      const actions = page.locator('.ui-filter-bar .distribution-query-actions')
      await expect(actions.getByRole('combobox', { name: zh ? '排序' : 'Sort', exact: true })).toBeVisible()
      const sortBox = await actions.getByRole('combobox').boundingBox()
      const queryBox = await actions.getByRole('button', { name: zh ? '查询' : 'Search', exact: true }).boundingBox()
      expect(Math.abs(sortBox!.y - queryBox!.y)).toBeLessThanOrEqual(1)
      if (size.width <= 900) {
        const first = await cards.nth(0).boundingBox(), second = await cards.nth(1).boundingBox(), third = await cards.nth(2).boundingBox()
        expect(Math.abs(first!.y - second!.y)).toBeLessThanOrEqual(1)
        expect(second!.x).toBeGreaterThan(first!.x + first!.width)
        expect(third!.y).toBeGreaterThan(first!.y + first!.height)
      }
      const stakeAmount = await cards.first().locator('.distribution-card-stake dd').boundingBox()
      const profitAmount = await cards.first().locator('.distribution-card-profit dd').boundingBox()
      expect(Math.abs(stakeAmount!.y - profitAmount!.y)).toBeLessThanOrEqual(1)
      expect(profitAmount!.x - stakeAmount!.x - stakeAmount!.width).toBeGreaterThanOrEqual(6)
      for (const [metric, title] of [['stake', zh ? '投注' : 'Stake'], ['profit', zh ? '输赢' : 'Profit / loss']]) {
        await expect(cards.first().locator(`.distribution-card-${metric} dd`)).toHaveAttribute('title', title!)
        const caption = await cards.first().locator(`.distribution-card-${metric} dt`).boundingBox()
        expect(caption!.width).toBeLessThanOrEqual(1)
      }
      const colors = await cards.first().locator('dd').evaluateAll(nodes => nodes.map(node => getComputedStyle(node).color))
      expect(new Set(colors).size).toBe(2)
      await expectNoPageOverflow(page)
      await page.screenshot({ path: info.outputPath(`distribution-${locale}-${size.width}.png`), fullPage: true })
    }
    for (const g of games.slice(1, 3)) {
      await selectControl(page.getByRole('combobox', { name: zh ? '彩种' : 'Game', exact: true }), { value: g.id })
      await expect.poll(() => queries.at(-1)?.get('game_id')).toBe(g.id)
      expect(queries.at(-1)?.get('period_no')).toBe('')
      await expect(cards).toHaveCount(49)
    }
    await selectControl(page.getByRole('combobox', { name: zh ? '彩种' : 'Game', exact: true }), { value: games[3]!.id })
    await expect(cards).toHaveCount(1000)
    await expect(cards.first().locator('h3')).toHaveText('000')
    await expect(cards.last().locator('h3')).toHaveText('999')
    await expect(page.locator('.distribution-zodiac')).toHaveCount(0)
    await expect(page.getByRole('heading', { name: zh ? '开奖盈利预测' : 'Draw profit forecast', exact: true })).toBeVisible()
    const rateMin = page.getByRole('spinbutton', { name: zh ? '最低' : 'Minimum', exact: true })
    const rateMax = page.getByRole('spinbutton', { name: zh ? '最高' : 'Maximum', exact: true })
    const search = page.getByRole('button', { name: zh ? '查询' : 'Search', exact: true })
    await rateMin.fill('80')
    await rateMax.fill('100')
    await search.click()
    await expect(cards.locator('h3')).toHaveText(['145', '147', '148'])
    await expect(cards.first().locator('dd').first()).toHaveText('100%')
    const rateSort = page.getByRole('combobox', { name: zh ? '排序' : 'Sort', exact: true })
    const readsBeforeRateSort = queries.length
    for (const [label, numbers] of [
      [zh ? '返奖率低→高' : 'Payout rate ↑', ['147', '148', '145']],
      [zh ? '返奖率高→低' : 'Payout rate ↓', ['145', '148', '147']],
    ] as const) {
      await selectControl(rateSort, { label })
      await expect(cards.locator('h3')).toHaveText([...numbers])
      await expect(rateSort.locator('.select-control-value > span:last-child')).toHaveText(label)
    }
    expect(queries.length).toBe(readsBeforeRateSort)
    for (const size of sizes) {
      if (info.project.name.includes('reflow')) await page.setViewportSize(size)
      const gameBox = await page.getByRole('combobox', { name: zh ? '彩种' : 'Game', exact: true }).boundingBox()
      const periodBox = await page.getByRole('searchbox', { name: zh ? '期次' : 'Period', exact: true }).boundingBox()
      expect(Math.abs(gameBox!.y - periodBox!.y)).toBeLessThanOrEqual(1)
      const actions = page.locator('.ui-filter-bar .distribution-query-actions')
      await expect(actions.getByRole('combobox', { name: zh ? '排序' : 'Sort', exact: true })).toBeVisible()
      const sortBox = await actions.getByRole('combobox').boundingBox()
      const queryBox = await actions.getByRole('button', { name: zh ? '查询' : 'Search', exact: true }).boundingBox()
      expect(Math.abs(sortBox!.y - queryBox!.y)).toBeLessThanOrEqual(1)
      if (size.width <= 900) {
        const first = await cards.nth(0).boundingBox(), second = await cards.nth(1).boundingBox(), third = await cards.nth(2).boundingBox()
        expect(Math.abs(first!.y - second!.y)).toBeLessThanOrEqual(1)
        expect(second!.x).toBeGreaterThan(first!.x + first!.width)
        expect(third!.y).toBeGreaterThan(first!.y + first!.height)
        const number = await cards.first().locator('h3').boundingBox()
        const rate = await cards.first().locator('.distribution-card-rate dd').boundingBox()
        expect(Math.abs(number!.y + number!.height / 2 - rate!.y - rate!.height / 2)).toBeLessThanOrEqual(1)
        expect(rate!.height).toBeLessThanOrEqual(24)
        const stake = await cards.first().locator('.distribution-card-stake').boundingBox()
        const profit = await cards.first().locator('.distribution-card-profit').boundingBox()
        expect(Math.abs(stake!.y - profit!.y)).toBeLessThanOrEqual(1)
        expect(profit!.x).toBeGreaterThan(stake!.x)
        const stakeValue = await cards.first().locator('.distribution-card-stake dd').boundingBox()
        const profitValue = await cards.first().locator('.distribution-card-profit dd').boundingBox()
        expect(profitValue!.x - stakeValue!.x - stakeValue!.width).toBeGreaterThanOrEqual(6)
        expect(first!.height).toBeLessThanOrEqual(60)
      }
      for (const [metric, title] of [['rate', zh ? '返奖率' : 'Payout rate'], ['stake', zh ? '投注' : 'Stake'], ['profit', zh ? '输赢' : 'Profit / loss']]) {
        await expect(cards.first().locator(`.distribution-card-${metric} dd`)).toHaveAttribute('title', title!)
      }
      const numberBox = await cards.first().locator('h3').boundingBox()
      const rateBox = await cards.first().locator('.distribution-card-rate dd').boundingBox()
      expect(Math.abs(numberBox!.y + numberBox!.height / 2 - rateBox!.y - rateBox!.height / 2)).toBeLessThanOrEqual(1)
      const colors = await cards.first().locator('dd').evaluateAll(nodes => nodes.map(node => getComputedStyle(node).color))
      expect(new Set(colors).size).toBe(3)
      await expectNoPageOverflow(page)
      await page.screenshot({ path: info.outputPath(`canada-distribution-${locale}-${size.width}.png`) })
    }
    if (info.project.name === 'chromium-reflow') {
      for (const theme of ['jade', 'vermilion', 'ocean', 'obsidian']) {
        for (const mode of ['light', 'dark']) {
          await page.evaluate(({ theme, mode }) => {
            document.documentElement.dataset.theme = theme
            document.documentElement.dataset.userTheme = mode
          }, { theme, mode })
          const colors = await cards.first().locator('dd').evaluateAll(nodes => nodes.map(node => getComputedStyle(node).color))
          expect(new Set(colors).size).toBe(3)
          await page.screenshot({ path: info.outputPath(`distribution-${theme}-${mode}.png`) })
        }
      }
    }
    await selectControl(rateSort, { value: 'number' })
    await rateMin.fill('150')
    await rateMax.fill('250')
    await search.click()
    await expect(cards.locator('h3')).toHaveText(['149'])
    await expect(cards.first().locator('dd').first()).toHaveText('250%')
    await rateMin.fill('251')
    await rateMax.fill('')
    await search.click()
    await expect(cards).toHaveCount(0)
    await expect(page.getByRole('status')).toHaveText(zh ? '暂无符合条件的结果。' : 'No matching outcomes.')
    await rateMin.fill('')
    await rateMax.fill('0')
    await search.click()
    await expect(cards).toHaveCount(994)
    const readsBeforeInvalid = queries.length
    await rateMin.fill('1.5')
    await rateMax.fill('200')
    await search.click()
    await expect(rateMin).toHaveAttribute('aria-invalid', 'true')
    expect(queries.length).toBe(readsBeforeInvalid)
    await rateMin.fill('200')
    await rateMax.fill('150')
    await search.click()
    expect(queries.length).toBe(readsBeforeInvalid)
    await rateMin.fill('0')
    await rateMax.fill('')
    const canadaPeriod = page.getByRole('searchbox', { name: zh ? '期次' : 'Period', exact: true })
    await canadaPeriod.fill('zero')
    await search.click()
    await expect(cards).toHaveCount(0)
    await rateMin.fill('')
    await search.click()
    await expect(cards).toHaveCount(1000)
    await expect(cards.first().locator('dd').first()).toHaveText('—')
    await selectControl(rateSort, { value: 'rateDescending' })
    await expect(cards.first().locator('h3')).toHaveText('000')
    await selectControl(page.getByRole('combobox', { name: zh ? '彩种' : 'Game', exact: true }), { value: games[0]!.id })
    await expect(cards).toHaveCount(49)
    await expect(rateMin).toHaveCount(0)
    await expect(rateSort.locator('.select-control-value > span:last-child')).toHaveText(zh ? '号码' : 'Number')
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
