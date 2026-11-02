import { expect, test, type Page } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { configuredDemo, type ConfiguredCatalog, type ConfiguredSnapshot } from '../../shared/src/configured-rules'
import { selectControl } from '../select-control'

const origin = process.env.FLAT_UI_ORIGIN ?? 'http://127.0.0.1:15274'
const gameId = 'aaaaaaaa-aaaa-4aaa-8aaa-000000000001'
const rulesPath = `/api/v1/platform/platform-games/${gameId}/configured/rules`
const catalog = JSON.parse(readFileSync('backend/internal/rules/config/mark_six_v2.json', 'utf8')) as ConfiguredCatalog

async function mount(page: Page, locale: string, surface = 'platform', canWrite = true) {
  const props = { surface, canWrite, brandId: surface === 'admin' ? gameId : '', gameId, locale }
  await page.route('**/flat-dialogs-harness', route => route.fulfill({ contentType: 'text/html', body: `<!doctype html><html><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"></head><body><main id="app" class="management-console" style="padding:12px;min-width:0"></main><script id="harness-data" type="application/json">${JSON.stringify({ component: 'ConfiguredRuleWorkbench', props, locale })}</script><script type="module" src="/tests/layout-harness.ts"></script></body></html>` }))
  await page.goto(`${origin}/flat-dialogs-harness`)
}

async function fits(page: Page) {
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true)
  const dialog = page.getByRole('dialog')
  expect(await dialog.locator('.detail-modal-content').evaluate(el => el.scrollWidth <= el.clientWidth + 1)).toBe(true)
}

for (const locale of ['zh-CN', 'en'] as const) {
  test(`flat dialogs preserve changes, discard and unified save ${locale}`, async ({ page }, info) => {
    const zh = locale === 'zh-CN', errors: string[] = [], writes: ConfiguredSnapshot[] = []
    let snapshot = configuredDemo(catalog)
    const key = Object.keys(snapshot.prices.plates.A!)[0]!
    page.on('pageerror', e => errors.push(e.message))
    await page.route('**/api/v1/**', route => {
      const req = route.request()
      expect(new URL(req.url()).pathname).toBe(rulesPath)
      if (req.method() === 'POST') {
        expect(req.headers()['idempotency-key']).toBeTruthy()
        snapshot = req.postDataJSON().snapshot
        writes.push(snapshot)
      }
      return route.fulfill({ json: { success: true, data: { id: gameId, game_id: gameId, revision: writes.length + 1, snapshot, snapshot_hash: 'test', status: 'published', cases: [], validation: null } } })
    })
    await mount(page, locale)
    const dialog = page.getByRole('dialog')
    const open = async (name: string) => page.getByRole('button', { name, exact: true }).click()
    const apply = () => dialog.getByRole('button', { name: zh ? '应用配置' : 'Apply configuration', exact: true }).click()
    const close = async () => {
      page.once('dialog', d => d.accept())
      await dialog.getByRole('button', { name: zh ? '关闭' : 'Close', exact: true }).click()
    }
    await open(zh ? '添加玩法' : 'Add play')
    await expect(dialog.locator('details')).toHaveCount(0)
    await dialog.getByLabel(zh ? '新玩法标识' : 'New play ID', { exact: true }).fill('flat_copy')
    await open(zh ? '继续配置' : 'Continue')
    await apply()
    await open(zh ? '添加玩法' : 'Add play')
    await open(zh ? '新建号码组' : 'New number group')
    await dialog.getByLabel(zh ? '新玩法标识' : 'New play ID', { exact: true }).fill('flat_group')
    await dialog.getByLabel(zh ? '中文名称' : 'Chinese name', { exact: true }).fill('扁平号码组')
    await dialog.getByLabel(zh ? '英文名称' : 'English name', { exact: true }).fill('Flat group')
    await dialog.getByLabel(zh ? '覆盖号码' : 'Covered numbers', { exact: true }).fill('1,2,3')
    for (const plate of catalog.plates) await dialog.getByLabel(zh ? `${plate}盘赔率` : `Odds for plate ${plate}`, { exact: true }).fill('2.5')
    await fits(page)
    await page.screenshot({ path: info.outputPath(`add-${locale}.png`) })
    await open(zh ? '继续配置' : 'Continue')
    await apply()
    expect(writes).toHaveLength(0)

    await open(zh ? '属性绑定' : 'Attribute bindings')
    await expect(dialog.locator('details')).toHaveCount(0)
    const rat = dialog.getByLabel(zh ? '鼠号码' : 'Rat numbers', { exact: true })
    const original = await rat.inputValue()
    await rat.fill('1, nope')
    await expect(rat).toHaveAttribute('aria-invalid', 'true')
    await expect(dialog.getByRole('button', { name: zh ? '应用配置' : 'Apply configuration', exact: true })).toBeDisabled()
    await selectControl(dialog.getByRole('combobox', { name: zh ? '属性组' : 'Attribute group', exact: true }), 'wave')
    const green = dialog.getByLabel(zh ? '绿波号码' : 'Green numbers', { exact: true })
    await green.fill((await green.inputValue()).split(', ').filter(n => n !== '49').join(', '))
    await expect(dialog).toContainText(zh ? '请保留固定号码归属。' : 'Keep the fixed number assignments.')
    await close()
    await open(zh ? '属性绑定' : 'Attribute bindings')
    await expect(rat).toHaveValue(original)
    const reversed = original.split(', ').reverse().join(', ')
    await rat.fill(reversed)
    await fits(page)
    await page.screenshot({ path: info.outputPath(`bindings-${locale}.png`) })
    await apply()

    await open(zh ? '赔率表' : 'Odds table')
    await expect(dialog.locator('details')).toHaveCount(0)
    const price = dialog.locator('.rule-price-table tbody tr').first().locator('input').first()
    await price.fill('6.5')
    await close()
    await open(zh ? '赔率表' : 'Odds table')
    await expect(price).toHaveValue('2')
    await price.fill('3.5')
    if (info.project.name.endsWith('reflow')) {
      for (const width of [320, 480, 768, 1280]) {
        await page.setViewportSize({ width, height: 800 })
        await fits(page)
      }
    }
    await fits(page)
    await page.screenshot({ path: info.outputPath(`prices-${locale}.png`) })
    await apply()
    expect(writes).toHaveLength(0)
    await open(zh ? '保存' : 'Save')
    await expect.poll(() => writes.length).toBe(1)
    expect(writes[0]!.catalog.plays).toHaveLength(catalog.plays.length + 2)
    expect(writes[0]!.bindings.attributes.zodiac!.rat).toEqual(original.split(', ').map(Number).reverse())
    expect(writes[0]!.prices.plates.A![key]).toBe('3.5')
    expect(writes[0]!.prices.version).toBe('synthetic-prices')
    expect(errors).toEqual([])
  })

  test(`flat dialogs remain read only for platform and brand viewers ${locale}`, async ({ page }) => {
    const zh = locale === 'zh-CN', snapshot = configuredDemo(catalog), writes: string[] = []
    await page.route('**/api/v1/**', route => {
      const req = route.request(), path = new URL(req.url()).pathname
      if (req.method() !== 'GET') writes.push(path)
      const data = path.endsWith('/sales') ? { version: 1, rule_id: gameId, prices: snapshot.prices, enabled: false, min_points: '1', max_points: '100', max_period_points: '10000', max_user_period_points: '5000' } : { id: gameId, game_id: gameId, revision: 1, snapshot, status: 'published' }
      return route.fulfill({ json: { success: true, data } })
    })
    for (const surface of ['platform', 'admin']) {
      await mount(page, locale, surface, false)
      await expect(page.getByRole('button', { name: zh ? '添加玩法' : 'Add play', exact: true })).toHaveCount(0)
      await page.getByRole('button', { name: zh ? '属性绑定' : 'Attribute bindings', exact: true }).click()
      const dialog = page.getByRole('dialog')
      await selectControl(dialog.getByRole('combobox', { name: zh ? '属性组' : 'Attribute group', exact: true }), 'wave')
      await expect(dialog.getByLabel(zh ? '绿波号码' : 'Green numbers', { exact: true })).toHaveAttribute('readonly', '')
      await expect(dialog.getByRole('button', { name: zh ? '应用配置' : 'Apply configuration', exact: true })).toHaveCount(0)
      await dialog.getByRole('button', { name: zh ? '关闭' : 'Close', exact: true }).click()
      await page.getByRole('button', { name: zh ? '赔率表' : 'Odds table', exact: true }).click()
      await expect(dialog.locator('input').first()).toBeDisabled()
      await fits(page)
    }
    expect(writes).toEqual([])
  })
}
