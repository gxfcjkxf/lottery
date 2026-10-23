import { expect, test } from '@playwright/test'
import { gameId, origins, installLayoutApi, expectNoPageOverflow } from './fixture'
import { overviewFixture } from './platform-overview'

export function registerBrandOverviewTests(reflow = false) {
  for (const locale of ['en', 'zh-CN'] as const) {
    test(`brand overview default home and isolated totals (${locale})`, async ({ page }, info) => {
      await page.clock.install()
      await installLayoutApi(page, locale)
      const errors: string[] = []
      page.on('pageerror', error => errors.push(error.message))
      let reads = 0, failure = false, hold = false
      let release: (() => void) | undefined
      await page.route('**/api/v1/admin/**', async route => {
        expect(route.request().method()).toBe('GET')
        const path = new URL(route.request().url()).pathname
        const reply = (data: unknown) => route.fulfill({ json: { success: true, data } })
        if (path.endsWith('/me')) return reply({ account: { id: gameId, username: 'brand_viewer', super_admin: false, brand_ids: [gameId], permissions: [], permissions_by_brand: { [gameId]: ['overview.view.brand'] }, platform_permissions: [] } })
        if (path.endsWith('/brands')) return reply({ items: [{ id: gameId, code: 'aurora', name: 'Aurora', status: 'active' }] })
        if (path.endsWith('/overview')) {
          expect(route.request().headers()['x-brand-id']).toBe(gameId)
          reads++
          if (hold) await new Promise<void>(resolve => { release = resolve })
          if (failure) return route.fulfill({ status: 503, json: { success: false, error: { code: 'SERVICE_UNAVAILABLE', message: 'private backend detail' } } })
          const { brands: _brands, ...summary } = overviewFixture()
          return reply({ ...summary, brand_id: gameId })
        }
        throw Error(`Unexpected brand overview request: ${path}`)
      })
      await page.goto(origins.brand)
      const overview = page.getByTestId('brand-overview')
      await expect(overview.locator('.overview-stats')).toBeVisible()
      await expect(page.locator('.brand-lockup small')).toHaveCount(0)
      await expect(page.locator('.side-nav > .nav-item')).toHaveText(locale === 'en' ? ['Home', 'Brand settings', 'Member management'] : ['首页', '品牌设置', '会员管理'])
      await expect(page.locator('.side-nav')).not.toContainText(locale === 'en' ? 'Operations workbench' : '运营工作台')
      await expect(page.locator('.console-nav-group summary')).not.toContainText([locale === 'en' ? 'Overview' : '概览'])
      const more = page.locator('.mobile-nav > button').last()
      if (await more.isVisible()) {
        await more.click()
        const menu = page.locator('.mobile-more-menu')
        await expect(menu).not.toContainText(locale === 'en' ? 'Operations workbench' : '运营工作台')
        await expect(menu.locator('h2')).not.toContainText([locale === 'en' ? 'Overview' : '概览'])
        await menu.getByRole('button', { name: locale === 'en' ? 'Home' : '首页', exact: true }).click()
      }
      await expect(page.locator('h1')).toHaveText(locale === 'en' ? 'Brand overview' : '品牌经营总览')
      await expect(overview.locator('.overview-ranking')).toHaveCount(0)
      await expect(overview.locator('.page-heading button, .page-heading p, .overview-meta, details')).toHaveCount(0)
      await expect(overview.locator('.overview-stat p')).toHaveCount(4)
      await expect(overview.locator('.overview-current p')).toHaveCount(3)
      await expect(overview.locator('.overview-business .status-chip')).toHaveCount(0)
      await expect(overview).toContainText('9,007,199,254,740,993')
      const preset = page.viewportSize()!
      const widths = reflow ? [...new Set([
        ...Array.from({ length: 9 }, (_, i) => 320 + i * 20),
        ...[620,640,700,768,820,900,1000,1080,1200,1440].flatMap(width => [width - 1, width, width + 1]),1024,
      ])] : [preset.width]
      for (const width of widths) {
        if (reflow) await page.setViewportSize({ width, height: preset.height })
        await expectNoPageOverflow(page)
        await expect(overview.locator('.overview-stats > article')).toHaveCount(4)
      }
      await overview.getByRole('group').getByRole('button', { name: locale === 'en' ? 'Settled net win' : '结算净赢', exact: true }).click()
      await expect(overview.locator('.overview-bar-loss')).toHaveCount(3)
      await page.screenshot({ path: info.outputPath(`brand-overview-${locale}.png`), fullPage: true })
      await info.attach('brand-overview-viewport.json', { body: JSON.stringify({ preset, widths, dpr: await page.evaluate(() => devicePixelRatio), screen: await page.evaluate(() => ({ width: screen.width, height: screen.height })) }), contentType: 'application/json' })
      if (reflow) {
        hold = true; const previous = reads
        await page.clock.fastForward(60000)
        await expect.poll(() => reads).toBeGreaterThan(previous)
        await expect(page.locator('.global-loading-overlay')).toHaveCount(0)
        failure = true; release!()
        await expect(overview.getByRole('alert')).toBeVisible()
        await expect(overview.locator('.overview-stats')).toHaveCount(0)
        await expect(overview).not.toContainText('private backend detail')
        release = undefined
        await overview.getByRole('button', { name: locale === 'en' ? 'Retry' : '重试', exact: true }).click()
        await expect.poll(() => typeof release).toBe('function')
        await expect(page.locator('.global-loading-overlay')).toBeVisible()
        failure = false; hold = false; release!()
        await expect(overview.locator('.overview-stats')).toBeVisible()
        await expect(page.locator('.global-loading-overlay')).toHaveCount(0)
        // Leave while a background read is pending, then return to a new home.
        hold = true; release = undefined; const before = reads
        await page.clock.fastForward(60000)
        await expect.poll(() => reads).toBeGreaterThan(before)
        await page.locator('.sidebar').getByRole('button', { name: locale === 'en' ? 'Audit log' : '审计日志', exact: true }).click()
        hold = false; release!()
        await expect(overview).toHaveCount(0)
        await expect(page.locator('.global-loading-overlay')).toHaveCount(0)
        const menu = page.locator('.mobile-nav > button').last()
        if (await menu.isVisible()) {
          await menu.click()
          await page.locator('.mobile-more-menu button').filter({ hasText: locale === 'en' ? /^Home$/ : /^首页$/ }).click()
        } else await page.locator('.sidebar .nav-item').filter({ hasText: locale === 'en' ? /^Home$/ : /^首页$/ }).click()
        await expect(overview.locator('.overview-stats')).toBeVisible()
      }
      expect(errors).toEqual([])
    })
  }
}
