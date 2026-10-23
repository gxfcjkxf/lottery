import { test, expect } from '@playwright/test'
import { gameId, origins, installLayoutApi, openPlatformBrandList, expectInsideViewport, expectNoPageOverflow } from './fixture'

export function registerPlatformMemberManagementTests(reflow = false) {
  for (const locale of ['en', 'zh-CN'] as const) test(`platform member directory uses full search, presence and shared actions (${locale})`, async ({ page }, info) => {
    const zh = locale === 'zh-CN', otherBrand = 'bbbbbbbb-bbbb-4bbb-8bbb-000000000001'
    await installLayoutApi(page, locale)
    const errors: string[] = [], queries: URL[] = []
    let allowed = true
    page.on('pageerror', error => errors.push(error.message))
    const members = Array.from({ length: 51 }, (_, index) => ({
      id: `cccccccc-cccc-4ccc-8ccc-${String(index + 1).padStart(12, '0')}`, global_user_id: gameId, brand_id: gameId,
      username: `member${String(index + 1).padStart(3, '0')}`, display_name: `member${String(index + 1).padStart(3, '0')}`,
      phone: '', notes: '', tags: [], status: 'normal', joined_at: '2026-10-21T00:00:00Z', online: index % 2 === 0, display_points: '9007199254740993',
    }))
    await page.route('**/api/v1/platform/**', async route => {
      const req = route.request(), url = new URL(req.url()), path = url.pathname
      expect(req.method()).toBe('GET')
      const reply = (data: unknown) => route.fulfill({ json: { success: true, data } })
      if (path.endsWith('/presentation')) return reply({ display_name: 'Layout trial', logo_text: 'L', logo_url: '' })
      if (path.endsWith('/me')) return reply({ account: { id: gameId, username: 'directory_reviewer', super_admin: true, platform_permissions: allowed ? ['user.view.platform', 'wallet.view.platform', 'user.delete.platform'] : ['user.view.platform'] } })
      if (path.endsWith('/platform-settings')) return reply({ display_name: 'Layout trial', logo_text: 'L', logo_url: '', timezone: 'Asia/Singapore', date_format: 'YYYY-MM-DD', time_format: 'HH:mm:ss', version: 1, updated_at: '2026-10-21T00:00:00Z' })
      if (path.endsWith('/brands')) return reply({ items: [{ id: gameId, name: 'Aurora', code: 'aurora', status: 'active' }, { id: otherBrand, name: 'Second', code: 'second', status: 'active' }] })
      if (path.endsWith('/users')) {
        queries.push(url)
        const brand = req.headers()['x-brand-id'], q = url.searchParams.get('q')?.toLowerCase() ?? '', online = url.searchParams.get('online'), limit = Number(url.searchParams.get('limit')), offset = Number(url.searchParams.get('offset'))
        expect([gameId, otherBrand]).toContain(brand)
        const selected = members.filter(member => (!q || member.username.includes(q)) && (online === null || member.online === (online === 'true')) && brand === gameId)
        return reply({ items: selected.slice(offset, offset + limit).map(member => ({ ...member, display_points: allowed ? member.display_points : null })), limit, offset, total_count: String(selected.length) })
      }
      throw Error(`Unexpected directory request: ${path}`)
    })
    await page.goto(origins.platform); await openPlatformBrandList(page, locale)
    await page.getByRole('button', { name: zh ? '查看会员' : 'View members', exact: true }).first().click()
    await expect(page.getByRole('heading', { name: zh ? '会员管理' : 'Member management', exact: true })).toBeVisible()
    await expect(page.locator('.member-filters [data-refresh-button]')).toHaveCount(0)
    const rows = () => page.locator('.platform-member-table tbody tr,.platform-member-cards .user-card').filter({ visible: true })
    const pagination = page.getByTestId('platform-member-pages')
    await expect(rows()).toHaveCount(50); await expect(pagination).toContainText('51')
    await expect(rows().first()).toContainText('9007199254740993')
    await expect(rows().first().getByRole('button', { name: 'member001', exact: true })).toHaveCount(1)
    await pagination.getByRole('button', { name: zh ? '下一页' : 'Next page', exact: true }).click()
    await expect(rows()).toHaveCount(1); await expect(rows()).toContainText('member051')
    const search = page.getByRole('textbox', { name: zh ? '搜索账号、昵称或手机号' : 'Search account, name or phone', exact: true })
    await search.fill('MEMBER001'); await search.press('Enter')
    await expect(rows()).toHaveCount(1); await expect(rows()).toContainText('member001')
    expect(queries.at(-1)!.searchParams.get('q')).toBe('MEMBER001'); expect(queries.at(-1)!.searchParams.get('offset')).toBe('0')
    const clear = () => page.getByRole('button', { name: zh ? '清除筛选' : 'Clear filter', exact: true })
    await clear().click()
    const online = page.getByRole('combobox', { name: zh ? '在线状态' : 'Online status', exact: true })
    await online.selectOption('false'); await page.locator('.member-filters button[type=submit]').click()
    await expect(rows()).toHaveCount(25); expect(queries.at(-1)!.searchParams.get('online')).toBe('false')
    await clear().click(); await expect(rows()).toHaveCount(50)
    await rows().first().getByRole('button', { name: 'member001', exact: true }).click()
    const detail = page.getByRole('dialog', { name: zh ? '会员详情' : 'Member details', exact: true })
    await expect(detail).toContainText('9007199254740993'); await expectInsideViewport(detail); await page.keyboard.press('Escape')
    await rows().first().locator('.member-menu-trigger').click()
    const menu = page.getByRole('menu', { name: zh ? 'member001 的操作' : 'Actions for member001', exact: true })
    await expect(menu.getByRole('menuitem')).toHaveCount(3); await expectInsideViewport(menu)
    await expect(menu.getByRole('menuitem', { name: zh ? '查看积分' : 'View points', exact: true })).toBeVisible()
    await menu.getByRole('menuitem', { name: zh ? '会员详情' : 'Member details', exact: true }).click(); await expect(detail).toBeVisible(); await page.keyboard.press('Escape')
    await search.fill('member001'); await search.press('Enter')
    await page.locator('.topbar-brand-picker select').selectOption(otherBrand)
    await expect(search).toHaveValue(''); await expect(online).toHaveValue(''); await expect(rows()).toHaveCount(0); await expect(pagination).toContainText(zh ? '共 0' : '0 members total')
    expect(queries.at(-1)!.searchParams.has('q')).toBe(false)
    await page.locator('.topbar-brand-picker select').selectOption(gameId)
    allowed = false; await page.reload(); await openPlatformBrandList(page, locale)
    await page.getByRole('button', { name: zh ? '查看会员' : 'View members', exact: true }).first().click()
    await expect(rows().first()).toContainText(zh ? '需要积分查看权限' : 'Points view permission required')
    await rows().first().locator('.member-menu-trigger').click(); await expect(menu.getByRole('menuitem')).toHaveCount(1); await page.keyboard.press('Escape')
    const preset = page.viewportSize()!
    for (const width of reflow ? [320, 640, 900, 1440] : [preset.width]) { if (reflow) await page.setViewportSize({ width, height: preset.height }); await expectNoPageOverflow(page) }
    await page.screenshot({ path: info.outputPath(`platform-members-${locale}.png`) }); expect(errors).toEqual([])
  })
}
