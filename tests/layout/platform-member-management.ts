import { expectRealEntry, expectListLayout } from './real-page-review'
import { selectControl } from '../select-control'
import { test, expect } from '@playwright/test'
import { gameId, origins, installLayoutApi, openPlatformBrandList, expectInsideViewport } from './fixture'

export function registerPlatformMemberManagementTests(reflow = false) {
  for (const locale of ['en', 'zh-CN'] as const) test(`platform member directory uses full search, presence and shared actions (${locale})`, async ({ page }, info) => {
    const zh = locale === 'zh-CN', otherBrand = 'bbbbbbbb-bbbb-4bbb-8bbb-000000000001'
    await installLayoutApi(page, locale)
    const errors: string[] = [], queries: URL[] = [], walletReads: string[] = []
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
      if (path.endsWith('/brands')) return reply({ items: [{ id: gameId, name: 'Aurora', code: 'aurora', status:'active',member_count:'12',agent_count:'3',recharge_quota_available:'1000',created_at:'2026-10-24T00:00:00Z' }, { id: otherBrand, name: 'Second', code: 'second', status:'active',member_count:'12',agent_count:'3',recharge_quota_available:'1000',created_at:'2026-10-24T00:00:00Z' }] , total_count: '2'})
      if (path.includes('/wallets/')) {
        expect(req.headers()['x-brand-id']).toBe(gameId)
        const id=path.split('/wallets/')[1]!.split('/')[0]!
        expect(members.some(member=>member.id===id)).toBe(true)
        walletReads.push(path)
        if(path.endsWith('/ledger'))return reply({items:[], total_count: '0'})
        const by_source=Object.fromEntries(['recharge','winning','gift','commission'].map(source=>[source,{available:source==='recharge'?'9007199254740993':'0',manual_frozen:'0',system_frozen:'0',withdrawal:'0'}]))
        return route.fulfill({json:{success:true,data:{account_id:id,brand_id:gameId,member_id:id,version:1,display_points:'9007199254740993',available_points:'9007199254740993',frozen_points:'0',withdrawal_points:'0',recharge_points:'9007199254740993',winning_points:'0',gift_points:'0',commission_points:'0',manual_frozen_points:'0',system_frozen_points:'0',by_source},associations:{[`member:${id}`]:{name:members.find(member=>member.id===id)!.username}}}})
      }
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
    await page.getByTestId('platform-brand-list').locator('.member-menu-trigger').first().click()
    await page.getByRole('menuitem', { name: zh ? '会员' : 'Members', exact: true }).click()
    await expect(page.getByRole('heading', { name: zh ? '会员管理' : 'Member management', exact: true })).toBeVisible()
    await expect(page.locator('.member-filters [data-refresh-button]')).toHaveCount(0)
    const rows = () => page.locator('.platform-member-table tbody tr:has(td:not(.empty-state)),.platform-member-cards .user-card').filter({ visible: true })
    const pagination = page.getByTestId('platform-member-pages')
    await expect(rows()).toHaveCount(50); await expect(pagination.locator('.table-pagination-summary .table-pagination-desktop')).toHaveText(zh ? '第 1 页 · 本页 50 条 · 共 51 条' : 'Page 1 · 50 records on this page · Total: 51')
    await expect(rows().first()).toContainText('9007199254740993')
    await expect(rows().first().getByRole('button', { name: 'member001', exact: true })).toHaveCount(1)
    await pagination.getByRole('button', { name: zh ? '下一页' : 'Next page', exact: true }).click()
    await expect(rows()).toHaveCount(1); await expect(rows()).toContainText('member051')
    await expect(pagination.locator('.table-pagination-summary .table-pagination-desktop')).toHaveText(zh ? '第 2 页 · 本页 1 条 · 共 51 条' : 'Page 2 · 1 record on this page · Total: 51')
    const search = page.getByRole('textbox', { name: zh ? '搜索账号或手机号' : 'Search account or phone', exact: true })
    await search.fill('MEMBER001'); await search.press('Enter')
    await expect(rows()).toHaveCount(1); await expect(rows()).toContainText('member001')
    expect(queries.at(-1)!.searchParams.get('q')).toBe('MEMBER001'); expect(queries.at(-1)!.searchParams.get('offset')).toBe('0')
    const clear = () => page.getByRole('button', { name: zh ? '清空' : 'Clear', exact: true })
    await clear().click()
    const online = page.getByRole('combobox', { name: zh ? '在线状态' : 'Online status', exact: true })
    await selectControl(online, 'false'); await page.locator('.member-filters button[type=submit]').click()
    await expect(rows()).toHaveCount(25); expect(queries.at(-1)!.searchParams.get('online')).toBe('false')
    await clear().click(); await expect(rows()).toHaveCount(50)
    await rows().first().getByRole('button', { name: 'member001', exact: true }).click()
    const detail = page.getByRole('dialog', { name: zh ? '会员详情' : 'Member details', exact: true })
    await expect(detail).toContainText('9007199254740993'); await expectInsideViewport(detail); await page.keyboard.press('Escape')
    await rows().first().locator('.member-menu-trigger').click()
    const menu = page.getByRole('menu', { name: zh ? 'member001 的操作' : 'Actions for member001', exact: true })
    await expect(menu.getByRole('menuitem')).toHaveCount(4); await expectInsideViewport(menu)
    await expect(menu.getByRole('menuitem', { name: zh ? '积分' : 'Points', exact: true })).toBeVisible()
    await menu.getByRole('menuitem', { name: zh ? '会员详情' : 'Member details', exact: true }).click(); await expect(detail).toBeVisible(); await page.keyboard.press('Escape')
    await search.fill('member001'); await search.press('Enter')
    await selectControl(page.locator(".topbar-brand-picker .select-control-trigger"), otherBrand)
    await expect(search).toHaveValue(''); await expect(online).toHaveAttribute('data-value', ''); await expect(rows()).toHaveCount(0); await expect(pagination).toContainText(zh ? '共 0 条' : 'Total: 0')
    expect(queries.at(-1)!.searchParams.has('q')).toBe(false)
    await selectControl(page.locator(".topbar-brand-picker .select-control-trigger"), gameId)
    await expect(rows()).toHaveCount(50)
    const beforeWallet=queries.length
    await rows().first().locator('.member-menu-trigger').click()
    await menu.getByRole('menuitem',{name:zh?'积分':'Points',exact:true}).click()
    const balances=page.getByTestId('platform-wallet-balances')
    await expect(balances).toBeVisible()
    await expect(balances.getByRole('heading',{name:zh?'积分':'Points',exact:true})).toBeVisible()
    await expect(balances).toContainText('9007199254740993')
    expect(queries).toHaveLength(beforeWallet)
    expect(walletReads).toEqual([`/api/v1/platform/wallets/${members[0]!.id}`])
    const wallet=page.getByTestId('platform-wallet')
    await expect(wallet.getByRole('textbox')).toHaveCount(0)
    await expect(page.getByRole('dialog')).toBeVisible()
    await wallet.locator('[data-refresh-button]').click()
    await expect.poll(()=>walletReads.length).toBe(2)
    expect(queries).toHaveLength(beforeWallet)
    const presetWallet=page.viewportSize()!
    if(reflow)await page.setViewportSize({width:320,height:720})
    await expectRealEntry(page,'platform');await expectListLayout(page)
    await page.screenshot({path:info.outputPath(`member-wallet-${locale}.png`),fullPage:true})
    if(reflow)await page.setViewportSize(presetWallet)
    await page.keyboard.press('Escape')
    await expect(balances).toHaveCount(0)
    await expect(rows()).toHaveCount(50)
    expect(queries).toHaveLength(beforeWallet)
    await selectControl(page.locator(".topbar-brand-picker .select-control-trigger"), otherBrand)
    await expect(balances).toHaveCount(0)
    expect(walletReads).toHaveLength(2)
    await selectControl(page.locator(".topbar-brand-picker .select-control-trigger"), gameId)
    allowed = false; await page.reload(); await openPlatformBrandList(page, locale)
    await page.getByTestId('platform-brand-list').locator('.member-menu-trigger').first().click()
    await page.getByRole('menuitem', { name: zh ? '会员' : 'Members', exact: true }).click()
    await expect(rows().first()).toContainText(zh ? '需要积分查看权限' : 'Points view permission required')
    await rows().first().locator('.member-menu-trigger').click(); await expect(menu.getByRole('menuitem')).toHaveCount(1); await page.keyboard.press('Escape')
    const preset = page.viewportSize()!
    for (const width of reflow ? [320, 640, 900, 1440] : [preset.width]) { if (reflow) await page.setViewportSize({ width, height: preset.height }); await expectRealEntry(page,'platform');await expectListLayout(page) }
    await page.screenshot({ path: info.outputPath(`platform-members-${locale}.png`) }); expect(errors).toEqual([])
  })
}
