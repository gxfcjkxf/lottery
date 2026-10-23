import { test, expect } from '@playwright/test'
import { gameId, origins, installLayoutApi, openPlatformBrandList, expectInsideViewport, expectNoPageOverflow } from './fixture'
export function registerAgentDirectoryTests(reflow = false) {
 for (const surface of ['brand', 'platform'] as const) for (const locale of ['en', 'zh-CN'] as const) test(`agent directory supports full search and presence (${surface}, ${locale})`, async ({ page }, info) => {
  const zh = locale === 'zh-CN', platform = surface === 'platform', size = platform ? 50 : 20, at = '2026-10-21T00:00:00Z'
  await installLayoutApi(page, locale)
  const errors: string[] = [], queries: URL[] = []; page.on('pageerror', error => errors.push(error.message))
  const nodes = Array.from({ length: size + 1 }, (_, index) => {
   const id = `bbbbbbbb-bbbb-4bbb-8bbb-${String(index + 1).padStart(12, '0')}`, root = 'bbbbbbbb-bbbb-4bbb-8bbb-000000000001'
   return { id, brand_id: gameId, agent_account_id: id, parent_id: index ? root : null, depth: index ? 2 : 1, path: index ? [root, id] : [id], version: 1, config: { ratio: '0.08', mode: null, status: 'active', can_create_children: true }, effective_mode: 'loss', mode_source_agent_id: null, policy_version: 1, parent_version: index ? 1 : null, created_by: gameId, created_at: at, updated_at: at, username: `agent${String(index + 1).padStart(3, '0')}`, online: index % 2 === 0, display_points: '9007199254740993' }
  })
  await page.route(`**/api/v1/${platform ? 'platform' : 'admin'}/**`, async route => {
   const req = route.request(), url = new URL(req.url()), path = url.pathname; expect(req.method()).toBe('GET')
   const reply = (data: unknown) => route.fulfill({ json: { success: true, data } })
   if (path.endsWith('/presentation')) return reply({ display_name: 'Layout trial', logo_text: 'L', logo_url: '' })
   if (path.endsWith('/me')) return reply({ account: { id: gameId, username: 'agent_reviewer', super_admin: platform, brand_ids: [gameId], permissions_by_brand: { [gameId]: ['agent.view.brand','wallet.view.brand'] }, platform_permissions: platform ? ['agent.view.platform','wallet.view.platform'] : [] } })
   if (path.endsWith('/platform-settings')) return reply({ display_name: 'Layout trial', logo_text: 'L', logo_url: '', timezone: 'Asia/Singapore', date_format: 'YYYY-MM-DD', time_format: 'HH:mm:ss', version: 1, updated_at: at })
   if (path.endsWith('/brands')) return reply({ items: [{ id: gameId, name: 'Aurora', code: 'aurora', status: 'active' }] })
   expect(req.headers()['x-brand-id']).toBe(gameId)
   if (path.endsWith('/agents')) {
    queries.push(url); const q = url.searchParams.get('q')?.toLowerCase() ?? '', online = url.searchParams.get('online'), limit = Number(url.searchParams.get('limit')), offset = Number(url.searchParams.get('offset'))
    const items = nodes.filter(node => node.username.includes(q) && (online === null || node.online === (online === 'true')))
    return reply({ brand_id: gameId, items: items.slice(offset, offset + limit), limit, offset, total_count: String(items.length) })
   }
   const node = nodes.find(node => path.endsWith('/agents/' + node.id)); if (node) return reply(node)
   throw Error('Unexpected agent directory request: ' + path)
  })
  await page.goto(origins[surface]); await expect(page.locator('.management-console')).toBeVisible()
  if (platform) {
   await openPlatformBrandList(page, locale)
   const toggle = page.locator('button[aria-controls="platform-navigation"]'); if (await toggle.isVisible() && await toggle.getAttribute('aria-expanded') === 'false') await toggle.click()
   await page.locator('.sidebar > button').getByText(zh ? '佣金' : 'Commission', { exact: true }).click()
   await page.getByTestId('platform-commission-secondary-nav').getByRole('button', { name: zh ? '代理' : 'Agents', exact: true }).click()
   await page.locator('.topbar-brand-picker select').selectOption(gameId)
  } else {
   const mobile = page.locator('.mobile-nav'); if (await mobile.isVisible()) await mobile.locator('button').last().click()
   const menu = page.locator(await mobile.isVisible() ? '.mobile-more-menu' : '.side-nav')
   const item = menu.getByRole('button', { name: zh ? '代理列表' : 'Agent list', exact: true })
   if (!await item.isVisible()) await item.locator('xpath=ancestor::details').locator('summary').click()
   await item.click()
  }
  const table = page.locator('.agent-directory-table'), rows = table.locator('tbody tr'), pagination = page.locator('.table-pagination')
  await expect(rows).toHaveCount(size); await expect(pagination).toContainText(String(size + 1)); await expect(rows.first()).toContainText('9007199254740993')
  await expect(rows.first().getByRole('button', { name: 'agent001', exact: true })).toHaveCount(1)
  await expect(page.locator('.member-filters [data-refresh-button]')).toHaveCount(0)
  if (!platform) {
   await expect(page.locator('.page-footer')).toHaveCount(0)
   await expect(pagination).toHaveCSS('margin-top', '12px')
  }
  const initialReads = queries.length
  await page.locator('.member-filters button[type=submit]').click()
  await expect.poll(() => queries.length).toBe(initialReads + 1)
  await expect(rows).toHaveCount(size)
  await pagination.getByRole('button').last().click(); await expect(rows).toHaveCount(1); await expect(rows).toContainText(`agent${String(size + 1).padStart(3, '0')}`)
  const search = page.getByRole('textbox', { name: zh ? '搜索账号、昵称或手机号' : 'Search account, name or phone', exact: true })
  await search.fill('AGENT001'); await search.press('Enter'); await expect(rows).toHaveCount(1); await expect(rows).toContainText('agent001')
  expect(queries.at(-1)!.searchParams.get('q')).toBe('AGENT001'); expect(queries.at(-1)!.searchParams.get('offset')).toBe('0')
  const clear = () => page.getByRole('button', { name: zh ? '清除筛选' : 'Clear filter', exact: true })
  await clear().click(); const online = page.getByRole('combobox', { name: zh ? '在线状态' : 'Online status', exact: true })
  await online.selectOption('false'); await page.locator('.member-filters button[type=submit]').click(); await expect(rows).toHaveCount(size / 2)
  expect(queries.at(-1)!.searchParams.get('online')).toBe('false')
  await clear().click(); await expect(rows).toHaveCount(size)
  await rows.first().locator('.member-menu-trigger').click(); const menu = page.getByRole('menu', { name: zh ? '代理 agent001 的操作' : 'Actions for agent agent001', exact: true })
  await expectInsideViewport(menu); await expect(menu.getByRole('menuitem')).toHaveCount(platform ? 1 : 2); await menu.getByRole('menuitem').first().click()
  const detail = page.getByRole('dialog'); await expectInsideViewport(detail); await page.keyboard.press('Escape')
  const preset = page.viewportSize()!
  for (const width of reflow ? [320,640,900,1440] : [preset.width]) { if (reflow) await page.setViewportSize({ width, height: preset.height }); await expectNoPageOverflow(page) }
  await page.screenshot({ path: info.outputPath(`agents-${surface}-${locale}.png`) }); expect(errors).toEqual([])
 })
}
