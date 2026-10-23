import { expect, test } from '@playwright/test'
import { resolve } from 'node:path'
import { readFileSync } from 'node:fs'

const origin = 'http://127.0.0.1:15273'
const brandId = 'aaaaaaaa-aaaa-4aaa-8aaa-000000000001'
const gameId = 'bbbbbbbb-bbbb-4bbb-8bbb-000000000001'
const snapshot = JSON.parse(readFileSync('backend/internal/rules/config/examples/lucky_group_preview.json', 'utf8'))

for (const locale of ['en', 'zh-CN'] as const) {
  test(`rules pages load on entry and retain list and draft state (${locale})`, async ({ page }, info) => {
    const zh = locale === 'zh-CN', reads: string[] = [], errors: string[] = []
    page.on('pageerror', error => { errors.push(error.message); console.error(error.message) })
    page.on('requestfailed', request => console.error(request.url(), request.failure()))
    const game = { id: gameId, name: 'Navigation game', code: 'navigation', model: snapshot.catalog.model, timezone: 'UTC', version: 1, connected: true, status: 'active', deletable: false, brand_enabled: true, brand_version: 1, rule_published: true, sales_status: 'ready' }
    // Synthetic GET receipts only: this test cannot read or write a development database.
    await page.route('**/api/**', route => {
      const request = route.request(), path = new URL(request.url()).pathname
      expect(request.method()).toBe('GET'); reads.push(path)
      const ok = (data: unknown) => route.fulfill({ json: { success: true, data } })
      if (path === '/api/v1/display-settings') return ok({ timezone: 'UTC', date_format: 'YYYY-MM-DD', time_format: 'HH:mm:ss' })
      if (path === '/api/v1/admin/platform-games') return ok({ games: [game], limit: 50, offset: 0 })
      if (path === `/api/v1/admin/platform-games/${gameId}`) return ok({ game, rule_protocol: 'configured', plays: [], periods: [], mark_six_configured: false })
      if (path.endsWith('/configured/versions')) return ok({ items: [{ id: gameId, game_id: gameId, revision: 1, status: 'published', snapshot, snapshot_hash: 'fixture', cases: [], validation: null }] })
      if (path.endsWith('/configured/sales')) return ok({ version: 1, rule_id: gameId, enabled: true, prices: snapshot.prices, min_points: '1', max_points: '100', max_period_points: '1000', max_user_period_points: '500' })
      if (path.endsWith('/configured/orders')) return ok({ items: [] })
      if (path === '/api/v1/admin/games') return ok({ games: [] })
      if (path.endsWith('/rule-preview/template')) return ok(snapshot.catalog)
      throw new Error(`Unexpected request ${path}`)
    })
    const permissions = ['platform_game.view.brand', 'platform_game.sell.brand', 'game.view.brand', 'play.view.brand', 'rule.view.brand', 'rule.simulate.brand']
    const account = { id: brandId, super_admin: false, brand_ids: [brandId], permissions, permissions_by_brand: { [brandId]: permissions } }
    await page.route('**/__rules-pages', route => route.fulfill({ contentType: 'text/html', body: `<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module">import{createApp}from'/node_modules/.vite/deps/vue.js';import C from'/@fs${resolve('admin-web/src/RulesPage.vue')}';import{createAdminI18n,adminI18nKey}from'/@fs${resolve('admin-web/src/i18n.ts')}';import'/@fs${resolve('shared/src/styles.css')}';import'/@fs${resolve('admin-web/src/style.css')}';import'/@fs${resolve('shared/src/controls.css')}';const i18n=createAdminI18n();i18n.setLocale(${JSON.stringify(locale)});createApp(C,${JSON.stringify({ account, brandId })}).provide(adminI18nKey,i18n).mount('#app')</script></html>` }))
    const noOverflow = async () => {
      const dimensions = await page.evaluate(() => ({ viewport: innerWidth, width: document.documentElement.scrollWidth, offenders: [...document.querySelectorAll('body *')].filter(el => el.getBoundingClientRect().width && el.getBoundingClientRect().right > innerWidth + 1).slice(-8).map(el => ({ tag: el.tagName, class: el.className, text: el.textContent?.slice(0,80), width: el.getBoundingClientRect().width })) }))
      expect(dimensions.width, JSON.stringify(dimensions)).toBeLessThanOrEqual(dimensions.viewport + 1)
    }
    const button = (en: string, cn: string) => page.getByRole('button', { name: zh ? cn : en, exact: true })
    await page.goto(origin + '/__rules-pages')
    await expect(page.getByText('Navigation game', { exact: true })).toBeVisible()
    expect(reads.filter(path => path !== '/api/v1/display-settings')).toEqual(['/api/v1/admin/platform-games'])
    await expect(page.locator('.rule-versions,.rule-simulator,[data-testid=configured-workbench]')).toHaveCount(0)
    await page.locator('.platform-lottery-filter select').selectOption('all')
    await noOverflow()
    await page.screenshot({ path: info.outputPath(`catalog-${locale}.png`), fullPage: true })
    await button('Play rules', '玩法规则').click()
    await expect(page.locator('.lottery-detail-page')).toBeVisible()
    await expect(page.getByTestId('lottery-catalog-page')).toBeHidden()
    await expect(page.getByRole('dialog')).toHaveCount(0)
    await expect(page.getByTestId('configured-workbench').getByRole('combobox', { name: zh ? '玩法' : 'Play', exact: true })).toBeVisible()
    await noOverflow()
    await button('Back to game list', '返回彩种列表').click()
    await expect(page.locator('.platform-lottery-filter select')).toHaveValue('all')
    await button('Number model', '号码模型').click()
    await expect(page.getByRole('dialog')).toBeVisible()
    await button('Close', '关闭').click()
    await button('Own game rules', '自有彩种规则').click()
    await expect(page.locator('.rule-versions')).toBeVisible()
    await expect.poll(() => reads.includes('/api/v1/admin/games')).toBe(true)
    await noOverflow()
    await button('Rule workbench', '规则工作台').click()
    const workbench = page.getByTestId('configured-workbench')
    await expect(workbench).toBeVisible()
    await button('Load sample template', '载入通用示例配置').click()
    const identifier = page.getByLabel(zh ? '新玩法标识' : 'New play identifier', { exact: true })
    await identifier.fill('retained_draft')
    await noOverflow()
    await button('Back to own game rules', '返回自有彩种规则').click()
    await button('Rule workbench', '规则工作台').click()
    await expect(identifier).toHaveValue('retained_draft')
    await button('Back to own game rules', '返回自有彩种规则').click()
    await button('Back to game list', '返回彩种列表').click()
    await expect(page.locator('.platform-lottery-filter select')).toHaveValue('all')
    await button('Rule simulation', '规则模拟').click()
    await expect(page.locator('.rule-simulator')).toBeVisible()
    await noOverflow()
    await page.screenshot({ path: info.outputPath(`simulation-${locale}.png`), fullPage: true })
    await button('Back to game list', '返回彩种列表').click()
    expect(reads.filter(path => path === '/api/v1/admin/platform-games')).toHaveLength(1)
    expect(errors).toEqual([])
  })
}
