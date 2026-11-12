import { expect, test } from '@playwright/test'
import { gameId, origins, installLayoutApi, expectNoPageOverflow } from './fixture'

for (const locale of ['zh-CN', 'en'] as const) {
  test(`brand split menus (${locale})`, async ({ page }, info) => {
    await installLayoutApi(page, locale)
    const errors: string[] = []
    page.on('pageerror', error => errors.push(error.message))
    await page.route('**/api/v1/admin/**', async route => {
      expect(route.request().method()).toBe('GET')
      const path = new URL(route.request().url()).pathname
      const reply = (data: unknown) => route.fulfill({ json: { success: true, data } })
      if (path.endsWith('/me')) return reply({ account: { id: gameId, username: 'menu_viewer', super_admin: false, brand_ids: [gameId], permissions: [], permissions_by_brand: { [gameId]: [] }, platform_permissions: [] } })
      if (path.endsWith('/brands')) return reply({ items: [{ id: gameId, code: 'layout', allow_custom_games: true, name: 'Layout trial', status: 'active', member_count: '0', agent_count: '0', recharge_quota_available: '0', created_at: '2026-10-24T00:00:00Z' }] })
      if (path.endsWith('/configured-orders')) return reply({items:[]})
      throw Error(`Unexpected split menu request: ${path}`)
    })
    await page.goto(origins.brand)
    await expect(page.locator('.management-console')).toBeVisible()
    const zh = locale === 'zh-CN'
    const labels = zh
      ? ['彩种管理', '注单管理', '充值记录', '提现申请', '佣金管理', '人工奖励', '业务报表', '管理账号', '角色权限']
      : ['Game management', 'Bet order management', 'Recharge records', 'Withdrawal requests', 'Commission management', 'Manual rewards', 'Business reports', 'Admin accounts', 'Roles and permissions']
    const preset = page.viewportSize()!
    const widths = info.project.name.includes('reflow') ? [320, 640, 900, 901, 1440] : [preset.width]
    for (const width of widths) {
      if (width !== preset.width || info.project.name.includes('reflow')) await page.setViewportSize({ width, height: preset.height })
      const more = page.locator('.mobile-nav > button').last()
      const mobile = await more.isVisible()
      if (mobile) {
        const labels = await page.locator('.mobile-nav small').evaluateAll(nodes => nodes.map(node => ({ height: node.getBoundingClientRect().height, line: Number.parseFloat(getComputedStyle(node).lineHeight) })))
        for (const label of labels) expect(label.height).toBeLessThanOrEqual(label.line + 1)
      }
      const openMenu = async () => {
        const dismiss = page.locator('.operation-toast-stack').getByRole('button', {name:zh?'关闭提示':'Dismiss message',exact:true})
        while (await dismiss.count()) await dismiss.first().click()
        if (mobile && await more.getAttribute('aria-expanded') !== 'true') await more.click()
      }
      await openMenu()
      const menu = page.locator(mobile ? '.mobile-more-menu' : '.side-nav')
      await expect(menu.getByRole('button', {name:zh?'余额修复':'Balance repair',exact:true})).toHaveCount(0)
      await expect(menu.getByRole('button', {name:zh?'批量对账':'Bulk reconciliation',exact:true})).toHaveCount(0)
      for (const name of zh ? ['积分', '账本'] : ['Points', 'Ledger']) await expect(menu.getByRole('button', { name, exact: true })).toHaveCount(0)
      for (const name of zh ? ['佣金核算', '佣金派发', '佣金更正'] : ['Commission calculations', 'Commission payouts', 'Commission corrections']) await expect(menu.getByRole('button', { name, exact: true })).toHaveCount(0)
      const captions = await menu.getByRole('button').allTextContents()
      expect(captions.join(' ')).not.toMatch(/期次和开奖|资金与账本|注单和异常|账号与权限|佣金和奖励|报表和对账|用户和成员/)
      for (const label of labels) await expect(menu.getByRole('button', { name: label, exact: true })).toHaveCount(1)
      for (const name of zh ? ['投注限额', '代理设置', '通知模板'] : ['Betting limits', 'Agent settings', 'Notification templates']) await expect(menu.getByRole('button', { name, exact: true })).toHaveCount(0)
      await expect(menu.getByRole('button', { name: zh ? '异常处理' : 'Exception handling', exact: true })).toHaveCount(0)
      await page.screenshot({ path: info.outputPath(`split-menu-${locale}-${width}.png`), fullPage: true })
      await expectNoPageOverflow(page)
      for (const label of labels) {
        await openMenu()
        await menu.getByRole('button', { name: label, exact: true }).click()
        await expect(page.locator('.breadcrumbs b')).toHaveText(label)
        await expect(page.locator('.content .console-tabs, .access-page .tabs')).toHaveCount(0)
        await expectNoPageOverflow(page)
        if (mobile) await expect(more).toHaveAttribute('aria-expanded', 'false')
      }
    }
    expect(errors).toEqual([])
  })
}

for (const locale of ['zh-CN', 'en'] as const) {
  test(`brand management accounts use the shared table (${locale})`, async ({ page }, info) => {
    await installLayoutApi(page, locale)
    const zh = locale === 'zh-CN', roleId = 'bbbbbbbb-bbbb-4bbb-8bbb-000000000001'
    const permissions = ['admin.view.brand', 'admin.write.brand', 'role.view.brand']
    const actor = { id: gameId, username: 'table_reviewer', status: 'active', super_admin: false, brand_ids: [gameId], role_ids: [roleId], role_codes: ['operations'], version: 1, permissions_by_brand: { [gameId]: permissions } }
    const target = { ...actor, id: 'cccccccc-cccc-4ccc-8ccc-000000000001', username: 'operations_account', status: 'disabled' }
    const errors: string[] = []
    page.on('pageerror', error => errors.push(error.message))
    await page.route('**/api/v1/admin/**', async route => {
      expect(route.request().method()).toBe('GET')
      const path = new URL(route.request().url()).pathname
      const reply = (data: unknown) => route.fulfill({ json: { success: true, data, associations: { [`role:${roleId}`]: { name: 'Operations', built_in: false } } } })
      if (path.endsWith('/me')) return reply({ account: actor })
      if (path.endsWith('/brands')) return reply({ items: [{ id: gameId, code: 'layout', allow_custom_games: true, name: 'Aurora', status: 'active', member_count: '0', agent_count: '0', recharge_quota_available: '0', created_at: '2026-10-25T00:00:00Z' }] })
      expect(route.request().headers()['x-brand-id']).toBe(gameId)
      if (path.endsWith('/accounts')) return reply({ items: [actor, target] })
      if (path.endsWith('/roles')) return reply({ items: [{ id: roleId, brand_id: gameId, code: 'operations', name: 'Operations', status: 'active', version: 1, is_bootstrap: false, permissions: ['admin.view.brand'] }] })
      throw Error(`Unexpected accounts request: ${path}`)
    })
    await page.goto(origins.brand)
    await expect(page.locator('.page-content [role=status]')).toContainText(zh ? '没有查看' : 'do not have permission')
    const more = page.locator('.mobile-nav > button').last()
    if (await more.isVisible()) {
      await more.click()
      await expect(more).toHaveAttribute('aria-expanded', 'true')
    }
    await page.locator(await more.isVisible() ? '.mobile-more-menu' : '.side-nav').getByRole('button', { name: zh ? '管理账号' : 'Admin accounts', exact: true }).click()
    await expect(page.locator('.breadcrumbs b')).toHaveText(zh ? '管理账号' : 'Admin accounts')
    const table = page.locator('.account-directory-table')
    await expect(table).toHaveClass(/console-table console-responsive-table/)
    await expect(table.locator('thead th')).toHaveText(zh ? ['账号', '角色', '权限并集', '状态', '操作'] : ['Account', 'Roles', 'Combined permissions', 'Status', 'Actions'])
    const rows = table.locator('tbody > tr')
    await expect(rows).toHaveCount(2)
    await expect(rows.first()).toContainText(zh ? '只读' : 'Read-only')
    await expect(rows.last()).toContainText('Operations')
    await expect(rows.last().locator('.badge')).toHaveText(zh ? '停用' : 'Disabled')
    await expect(page.locator('.table-pagination button').first()).toBeDisabled()
    await expect(page.locator('.table-pagination button').last()).toBeDisabled()
    const preset = page.viewportSize()!
    for (const width of info.project.name.includes('reflow') ? [320, 640, 900, 901, 1440] : [preset.width]) {
      if (info.project.name.includes('reflow')) await page.setViewportSize({ width, height: preset.height })
      const summary = rows.last().locator('summary')
      await summary.click()
      await expect(rows.last().locator('details')).toHaveAttribute('open', '')
      await expectNoPageOverflow(page)
      await page.screenshot({ path: info.outputPath(`management-accounts-${locale}-${width}.png`), fullPage: true })
      await summary.click()
      await rows.last().getByRole('button', { name: zh ? '编辑' : 'Edit', exact: true }).click()
      await expect(page.getByRole('dialog')).toBeVisible()
      await expectNoPageOverflow(page)
      await page.keyboard.press('Escape')
      await expect(page.getByRole('dialog')).toHaveCount(0)
    }
    expect(errors).toEqual([])
  })
}

for (const locale of ['zh-CN', 'en'] as const) {
  test(`brand platform games only expose availability (${locale})`, async ({ page }, info) => {
    await installLayoutApi(page, locale)
    const zh = locale === 'zh-CN', errors: string[] = [], writes: { path: string; body: unknown }[] = []
    const grants = ['platform_game.view.brand', 'platform_game.connect.brand', 'platform_game.sell.brand', 'game.view.brand']
    let platformReads = 0, brandReads = 0
    const games = Array.from({ length: 3 }, (_, i) => ({ id: `bbbbbbbb-bbbb-4bbb-8bbb-00000000000${i + 1}`, code: `game_${i}`, name: `Platform game ${i + 1}`, model: { model: 'DIGITS_0_9', length: 3, allow_repeat: true, ordered: true }, timezone: 'UTC', version: 1, connected: i !== 2, status: 'active', deletable: false, brand_enabled: i === 0, brand_version: i === 2 ? 0 : 1, rule_published: true, sales_status: 'ready' }))
    page.on('pageerror', error => errors.push(error.message))
    await page.route('**/api/v1/admin/**', async route => {
      const req = route.request(), path = new URL(req.url()).pathname
      const reply = (data: unknown, status = 200) => route.fulfill({ status, json: { success: true, data, associations: { [`brand:${gameId}`]: { name: 'Aurora' } } } })
      if (path.endsWith('/me')) return reply({ account: { id: gameId, username: 'availability_reviewer', super_admin: false, brand_ids: [gameId], permissions_by_brand: { [gameId]: grants } } })
      if (path.endsWith('/brands')) return reply({ items: [{ id: gameId, code: 'layout', allow_custom_games: true, name: 'Aurora', status: 'active', member_count: '0', agent_count: '0', recharge_quota_available: '0', created_at: '2026-10-25T00:00:00Z' }] })
      expect(req.headers()['x-brand-id']).toBe(gameId)
      if (path.endsWith('/platform-games') && req.method() === 'GET') { platformReads++; return reply({ games, limit: 50, offset: 0, total_count: String(games.length) }) }
      if (path.endsWith('/games') && req.method() === 'GET') { brandReads++; return reply({ games: [] }) }
      if (req.method() === 'POST') {
        expect(req.headers()['idempotency-key']).toBeTruthy()
        const game = games.find(game => path.includes(game.id))!
        expect(game).toBeTruthy()
        const body = req.postDataJSON(); writes.push({ path, body })
        if (path.endsWith('/connect')) { expect(body).toEqual({}); game.connected = true; game.brand_enabled = true; game.brand_version = 1; return reply(game, 201) }
        expect(path.endsWith('/availability')).toBe(true)
        expect(body.expected_version).toBe(game.brand_version)
        expect(Object.keys(body).sort()).toEqual(['enabled', 'expected_version', 'reason'])
        game.brand_enabled = body.enabled; game.brand_version++
        return reply(game)
      }
      throw Error(`Unexpected platform-game request: ${req.method()} ${path}`)
    })
    const enter = async () => {
      await page.goto(origins.brand)
      await expect(page.locator('.page-content [role=status]')).toContainText(zh ? '没有查看' : 'do not have permission')
      const more = page.locator('.mobile-nav > button').last()
      if (await more.isVisible()) { await more.click(); await expect(more).toHaveAttribute('aria-expanded', 'true') }
      await page.locator(await more.isVisible() ? '.mobile-more-menu' : '.side-nav').getByRole('button', { name: zh ? '彩种管理' : 'Game management', exact: true }).click()
    }
    await enter()
    await expect(page.getByTestId('brand-rules-page')).toHaveCount(0)
    await expect(page.getByRole('button', { name: zh ? '自有彩种规则' : 'Own game rules', exact: true })).toHaveCount(0)
    expect(brandReads).toBe(0)
    const table = page.locator('.brand-lottery-table'), rows = table.locator('tbody tr')
    await expect(page.locator('.platform-lottery-catalog-table')).toHaveCount(0);await expect(page.locator('.platform-lottery .table-pagination')).toBeVisible()
    await expect(rows).toHaveCount(3)
    await expect(table.locator('.member-menu-trigger')).toHaveCount(3)
    await expect(page.locator('.lottery-detail-page, [data-testid=automation-settings], [data-testid=configured-workbench]')).toHaveCount(0)
    const preset = page.viewportSize()!
    for (const width of info.project.name.includes('reflow') ? [320, 640, 820, 900, 901, 1440] : [preset.width]) {
      if (info.project.name.includes('reflow')) await page.setViewportSize({ width, height: preset.height })
      await expectNoPageOverflow(page)
      await page.screenshot({ path: info.outputPath(`brand-game-availability-${locale}-${width}.png`), fullPage: true })
    }
    for (const rowIndex of [0, 0, 2]) {
      await rows.nth(rowIndex).locator('.member-menu-trigger').click()
      const gameAction = games[rowIndex].connected && games[rowIndex].brand_enabled ? (zh ? '停用' : 'Disable') : (zh ? '启用' : 'Enable')
      await page.getByRole('menuitem',{name:gameAction,exact:true}).click()
      const dialog = page.getByRole('dialog')
      await expect(dialog).toBeVisible()
      const game = games[rowIndex], action = game.connected && game.brand_enabled ? (zh ? '停用' : 'Disable') : (zh ? '启用' : 'Enable')
      await expect(dialog.getByRole('heading')).toHaveText(zh ? `${action}「${game.name}」` : `${action} “${game.name}”`)
      await expect(dialog).not.toContainText(zh ? '本品牌' : 'for brand')
      await expectNoPageOverflow(page)
      await page.screenshot({ path: info.outputPath(`game-confirmation-${locale}-${writes.length}.png`), fullPage: true })
      await expect(dialog.locator('input, select, textarea')).toHaveCount(0)
      await dialog.locator('button[type=submit]').click()
      await expect(dialog).toHaveCount(0)
    }
    expect(writes.map(write => write.path.split('/').at(-1))).toEqual(['availability', 'availability', 'connect'])
    expect(games.map(game => game.brand_enabled)).toEqual([true, false, true])
    await expect(rows.last()).toContainText(zh ? '启用' : 'Enabled')
    const more = page.locator('.mobile-nav > button').last()
    if (await more.isVisible()) await more.click()
    await expect(page.locator(await more.isVisible()?'.mobile-more-menu':'.side-nav').getByRole('button',{name:zh?'规则配置':'Rule configuration',exact:true})).toHaveCount(0)
    grants.splice(1)
    await enter()
    await expect(table.getByRole('button')).toHaveCount(0)
    await expect(table.getByRole('columnheader',{name:zh?'操作':'Actions',exact:true})).toHaveCount(0)
    await expect(rows).toHaveCount(3)
    expect(errors).toEqual([])
  })
}

for (const locale of ['zh-CN', 'en'] as const) {
  test(`brand betting limits compact form (${locale})`, async ({ page }, info) => {
    await installLayoutApi(page, locale)
    const permissions = ['bet_policy.view.brand', 'bet_policy.write.brand']
    let configured = false, version = 1, minPoints = '1', saves = 0
    let uncertainKey: string | undefined
    let uncertainBody: unknown
    const secondGame = 'bbbbbbbb-bbbb-4bbb-8bbb-000000000001'
    await page.route('**/api/v1/admin/**', async route => {
      const path = new URL(route.request().url()).pathname
      const reply = (data: unknown) => route.fulfill({ json: { success: true, data } })
      if (route.request().method() === 'POST') {
        expect(path).toBe(`/api/v1/admin/platform-games/${gameId}/bet-limits`)
        const body = route.request().postDataJSON()
        if (!uncertainKey) {
          uncertainKey = route.request().headers()['idempotency-key']; uncertainBody = body
          return route.fulfill({ status: 503, json: { success: false, error: { code: 'SERVICE_UNAVAILABLE', message: 'Unavailable' } } })
        }
        if (saves === 0) {
          expect(route.request().headers()['idempotency-key']).toBe(uncertainKey)
          expect(body).toEqual(uncertainBody)
        }
        expect(body.version).toBe(version)
        expect(route.request().headers()['idempotency-key']).toBeTruthy()
        configured = true; version++; minPoints = body.min_points; saves++
        return reply({ ...body, version })
      }
      expect(route.request().method()).toBe('GET')
      if (path.endsWith('/me')) return reply({ account: { id: gameId, username: 'limits_reviewer', super_admin: false, brand_ids: [gameId], permissions_by_brand: { [gameId]: permissions } } })
      if (path.endsWith('/brands')) return reply({ items: [{ id: gameId, code: 'layout', name: 'Aurora', status: 'active', member_count: '0', agent_count: '0', recharge_quota_available: '0', created_at: '2026-10-25T00:00:00Z' }] })
      expect(route.request().headers()['x-brand-id']).toBe(gameId)
      if (path === '/api/v1/admin/bet-limits') return reply({ items: [{ id: gameId, name: 'Mark Six', configured }, { id: secondGame, name: locale === 'zh-CN' ? '加拿大快开彩种' : 'Canada fast draw', configured: true }] })
      if (path === `/api/v1/admin/platform-games/${gameId}/bet-limits`) return reply({ brand_id: gameId, game_id: gameId, version, min_points: minPoints, max_points: '100', max_period_points: '1000', max_user_period_points: '500' })
      throw Error(`Unexpected limits request: ${path}`)
    })
    await page.goto(origins.brand)
    await expect(page.locator('.page-content [role=status]')).toContainText(locale === 'zh-CN' ? '没有查看' : 'do not have permission')
    const more = page.locator('.mobile-nav > button').last()
    if (await more.isVisible()) await more.click()
    const title = locale === 'zh-CN' ? '投注限额' : 'Betting limits'
    await page.locator(await more.isVisible() ? '.mobile-more-menu' : '.side-nav').getByRole('button', { name: locale === 'zh-CN' ? '品牌设置' : 'Brand settings', exact: true }).click()
    await page.locator('.console-tabs').getByRole('button', { name: title, exact: true }).click()
    const panel = page.getByTestId('bet-policy-settings')
    await expect(page.locator('.console-tabs').getByRole('button', { name: title, exact: true })).toHaveClass(/selected/)
    await expect(panel.locator('.policy-header')).toHaveCount(0)
    await expect(panel.locator('input, form')).toHaveCount(0)
    await expect(panel.locator('.game-tags button')).toHaveCount(2)
    await expect(panel.locator('.policy-card').first()).not.toContainText(/品牌：|Brand:|品牌默认策略|Brand default/)
    await expect(panel.locator('.policy-header p, .brand-pill')).toHaveCount(0)
    await expect(panel.locator('.game-tag').first()).not.toHaveClass(/is-configured/)
    await expect(panel.locator('.game-tag').first().getByRole('button')).toHaveText(locale === 'zh-CN' ? '设置' : 'Set up')
    await expect(panel.locator('.game-tag').first().locator('.game-status')).toHaveText(locale === 'zh-CN' ? '未设置' : 'Not configured')
    await expect(panel.locator('.game-tag').last()).toHaveClass(/is-configured/)
    await expect(panel.locator('.game-tag').last().getByRole('button')).toHaveText(locale === 'zh-CN' ? '修改' : 'Edit')
    expect(await panel.locator('.game-tag').first().evaluate(node => getComputedStyle(node).backgroundColor)).not.toBe(await panel.locator('.game-tag').last().evaluate(node => getComputedStyle(node).backgroundColor))
    const preset = page.viewportSize()!
    for (const width of info.project.name.includes('reflow') ? [320, 640, 900, 901, 1440] : [preset.width]) {
      if (info.project.name.includes('reflow')) await page.setViewportSize({ width, height: preset.height })
      await expectNoPageOverflow(page)
      await page.screenshot({ path: info.outputPath(`betting-limits-${locale}-${width}.png`), fullPage: true })
      await panel.locator('.game-tag').first().getByRole('button').click()
      const dialog = page.getByRole('dialog')
      await expect(dialog).toBeVisible()
      await expect(dialog.locator('input')).toHaveCount(4)
      await expect(dialog.locator('select')).toHaveCount(0)
      await dialog.locator('input').first().fill('2')
      await expectNoPageOverflow(page)
      await page.screenshot({ path: info.outputPath(`game-limits-${locale}-${width}.png`), fullPage: true })
      const save = dialog.getByRole('button', { name: locale === 'zh-CN' ? '保存' : 'Save', exact: true })
      await save.scrollIntoViewIfNeeded()
      await expect(save).toBeVisible()
      await expect(save).toBeEnabled()
      await save.click()
      if (saves === 0) {
        await expect(dialog.locator('.action-row button')).toHaveText(locale === 'zh-CN' ? '使用原请求重试' : 'Retry original request')
        await expect(dialog).toBeVisible()
        await expect(panel.locator('.game-tag').first()).not.toHaveClass(/is-configured/)
        await page.locator('.toast-error button').click()
        await dialog.locator('.action-row button').click()
      }
      await expect(dialog).toHaveCount(0)
      await expect(panel.locator('input, form')).toHaveCount(0)
      await expect(panel.locator('.game-tag').first()).toHaveClass(/is-configured/)
      await expect(panel.locator('.game-tag').first().getByRole('button')).toHaveText(locale === 'zh-CN' ? '修改' : 'Edit')
      await expect(panel.locator('.game-tag').first().locator('.game-status')).toHaveText(locale === 'zh-CN' ? '已设置' : 'Configured')
      await page.screenshot({ path: info.outputPath(`betting-limits-saved-${locale}-${width}.png`), fullPage: true })
      await page.locator('.operation-toast-stack .toast button').evaluateAll(buttons => buttons.forEach(button => (button as HTMLButtonElement).click()))
      await panel.locator('.game-tag').first().getByRole('button').click()
      await expect(dialog.locator('input').first()).toHaveValue('2')
      await dialog.getByRole('button', { name: locale === 'zh-CN' ? '关闭' : 'Close', exact: true }).click()
    }
    expect(saves).toBeGreaterThan(0)
    await page.reload()
    await expect(page.locator('.management-console')).toBeVisible()
    if (await more.isVisible()) await more.click()
    await page.locator(await more.isVisible() ? '.mobile-more-menu' : '.side-nav').getByRole('button', { name: locale === 'zh-CN' ? '品牌设置' : 'Brand settings', exact: true }).click()
    await page.locator('.console-tabs').getByRole('button', { name: title, exact: true }).click()
    await expect(panel.locator('.game-tag').first()).toHaveClass(/is-configured/)
  })
}

for (const locale of ['zh-CN', 'en'] as const) {
  test(`withdrawal policies use shared directory and omit empty custom games (${locale})`, async ({ page }, info) => {
    await installLayoutApi(page, locale)
    let directoryReads = 0
    await page.route('**/api/v1/admin/**', async route => {
      expect(route.request().method()).toBe('GET')
      const url = new URL(route.request().url()), path = url.pathname
      const reply = (data: unknown) => route.fulfill({ json: { success: true, data } })
      if (path.endsWith('/me')) return reply({ account: { id: gameId, username: 'policy_directory_reviewer', super_admin: false, brand_ids: [gameId], permissions_by_brand: { [gameId]: ['withdrawal_policy.view.brand'] } } })
      if (path.endsWith('/brands')) return reply({ items: [{ id: gameId, code: 'layout', name: 'Aurora', status: 'active', member_count: '0', agent_count: '0', recharge_quota_available: '0', created_at: '2026-10-25T00:00:00Z' }] })
      expect(route.request().headers()['x-brand-id']).toBe(gameId)
      if (path.endsWith('/filter-options')) {
        expect(url.searchParams.get('resource')).toBe('withdrawal_policy')
        expect(url.searchParams.get('kind')).toBe('game')
        directoryReads++
        return reply({ brand_id: gameId, items: [], limit: 100, offset: 0 })
      }
      if (path.endsWith('/withdrawal-policy')) return reply({ points_step:'1', brand_id: gameId, version: 1, config: { enabled: false, min_points: '1', max_points: null, allowed_sources: ['recharge', 'winning', 'gift'], review_mode: 'manual', turnover_multiple: '1' }, updated_at: '2026-10-25T00:00:00Z' })
      throw Error(`Unexpected withdrawal directory request: ${path}`)
    })
    await page.goto(origins.brand)
    await expect(page.locator('.page-content [role=status]')).toContainText(locale === 'zh-CN' ? '没有查看' : 'do not have permission')
    const more = page.locator('.mobile-nav > button').last()
    if (await more.isVisible()) await more.click()
    await page.locator(await more.isVisible() ? '.mobile-more-menu' : '.side-nav').getByRole('button', { name: locale === 'zh-CN' ? '品牌设置' : 'Brand settings', exact: true }).click()
    await page.locator('.console-tabs').getByRole('button', { name: locale === 'zh-CN' ? '提现设置' : 'Withdrawal settings', exact: true }).click()
    const panel = page.getByTestId('withdrawal-policy-settings')
    await expect(panel.locator('.saved-summary')).toBeVisible()
    await expect.poll(() => directoryReads).toBe(1)
    await expect(panel.locator('.game-picker, .game-id-form')).toHaveCount(0)
    await expectNoPageOverflow(page)
    await page.screenshot({ path: info.outputPath(`withdrawal-directory-${locale}.png`), fullPage: true })
  })
}

for (const locale of ['zh-CN', 'en'] as const) {
  for (const writable of [true, false]) {
    test(`brand funds settings ${writable ? 'save and retry' : 'read only'} (${locale})`, async ({ page }, info) => {
      await installLayoutApi(page, locale)
      const zh = locale === 'zh-CN'
      const requests: string[] = [], errors: string[] = []
      const pointWrites: string[] = [], withdrawalWrites: { body: string | null; key: string | undefined }[] = []
      page.on('pageerror', error => errors.push(error.message))
      let points = { brand_id: gameId, version: 1, max_balance_points: '10000', max_recharge_points: null, max_adjustment_points: null }
      let withdrawal = { points_step: '1', brand_id: gameId, audit_log_id: gameId, version: 1, config: { enabled: true, min_points: '1', max_points: null, allowed_sources: ['recharge', 'winning', 'gift'], review_mode: 'manual', turnover_multiple: '1' }, updated_at: '2026-10-26T00:00:00Z' }
      let receiptLost = false
      await page.route('**/api/v1/admin/**', async route => {
        const request = route.request(), path = new URL(request.url()).pathname
        requests.push(`${request.method()} ${path}`)
        const reply = (data: unknown) => route.fulfill({ json: { success: true, data, associations: { [`brand:${gameId}`]: { name: 'Aurora' } } } })
        if (path.endsWith('/me')) return reply({ account: { id: gameId, username: 'funds_layout_reviewer', super_admin: false, brand_ids: [gameId], permissions_by_brand: { [gameId]: ['point_policy.view.brand', 'withdrawal_policy.view.brand', ...(writable ? ['point_policy.write.brand', 'withdrawal_policy.write.brand'] : [])] } } })
        if (path.endsWith('/brands')) return reply({ items: [{ id: gameId, code: 'layout', name: 'Aurora', status: 'active', member_count: '0', agent_count: '0', recharge_quota_available: '0', created_at: '2026-10-26T00:00:00Z' }] })
        expect(request.headers()['x-brand-id']).toBe(gameId)
        if (request.method() === 'GET') {
          if (path.endsWith('/cash/config')) return reply({version:1,rules:['CNY','USD','THB','USDT'].map(currency=>({currency,rate:'7',deposit_min:'1',deposit_max:'10000',withdrawal_min:'1',withdrawal_max:'10000',fee:'0',mode:'count',threshold:'2',enabled:true}))})
          if (path.endsWith('/cash/destinations')) return reply([])
          if (path.endsWith('/point-policy')) return reply(points)
          if (path.endsWith('/withdrawal-policy')) return reply(withdrawal)
          if (path.endsWith('/filter-options')) return reply({ brand_id: gameId, items: [], limit: 100, offset: 0 })
        }
        if (request.method() === 'PUT' && writable) {
          const body = request.postDataJSON()
          expect(request.headers()['idempotency-key']).toBeTruthy()
          expect(body.reason).toBe('')
          if (path.endsWith('/point-policy')) {
            pointWrites.push(request.postData()!)
            expect(body.version).toBe(points.version)
            points = { ...points, ...body, version: points.version + 1 }
            return reply(points)
          }
          if (path.endsWith('/withdrawal-policy')) {
            withdrawalWrites.push({ body: request.postData(), key: request.headers()['idempotency-key'] })
            if (!receiptLost) {
              expect(body.version).toBe(withdrawal.version)
              withdrawal = { ...withdrawal, points_step: body.points_step, config: body.config, version: withdrawal.version + 1 }
              receiptLost = true
              return route.abort('failed')
            }
            expect(withdrawalWrites[1]).toEqual(withdrawalWrites[0])
            return reply(withdrawal)
          }
        }
        throw Error(`Unexpected funds request: ${request.method()} ${path}`)
      })
      await page.goto(origins.brand)
      await expect(page.locator('.page-content [role=status]')).toContainText(zh ? '没有查看' : 'do not have permission')
      const more = page.locator('.mobile-nav > button').last()
      if (await more.isVisible()) {
        await more.click()
        await expect(more).toHaveAttribute('aria-expanded', 'true')
      }
      const menu = page.locator(await more.isVisible() ? '.mobile-more-menu' : '.side-nav')
      await expect(menu.getByRole('button', { name: zh ? '资金策略' : 'Financial policies', exact: true })).toHaveCount(0)
      await menu.getByRole('button', { name: zh ? '品牌设置' : 'Brand settings', exact: true }).click()
      const tabs = page.locator('.console-tabs')
      await expect(tabs.getByRole('button')).toHaveText(zh ? ['运行状态', '品牌展示', '资金', '提现设置', '投注限额', '代理设置', '公告', '联系方式'] : ['Status', 'Presentation', 'Funds', 'Withdrawal settings', 'Betting limits', 'Agent settings', 'Announcements', 'Contact details'])
      expect(requests.some(request => /point-policy|withdrawal-policy/.test(request))).toBe(false)
      await tabs.getByRole('button', { name: zh ? '资金' : 'Funds', exact: true }).click()
      const pointPanel = page.locator('.point-policy-settings'), withdrawalPanel = page.getByTestId('withdrawal-policy-settings')
      await expect(pointPanel.getByRole('heading')).toHaveText(zh ? '积分限额' : 'Points limits')
      await expect(pointPanel.locator(writable ? 'form' : '.current-limits')).toBeVisible()
      await expect(withdrawalPanel).toHaveCount(0)
      await tabs.getByRole('button', { name: zh ? '提现设置' : 'Withdrawal settings', exact: true }).click()
      await expect(pointPanel).toHaveCount(0)
      await expect(withdrawalPanel.getByRole('heading', { level: 2 })).toHaveCount(0)
      await expect(withdrawalPanel.locator(writable ? 'form' : '.saved-summary')).toBeVisible()
      await expect(page.locator('.brand-settings-panel .refresh-icon-control, .history-panel, .game-id-form')).toHaveCount(0)
      await expect(withdrawalPanel.locator('.game-picker')).toHaveCount(0)
      const preset = page.viewportSize()!
      for (const width of info.project.name.includes('reflow') ? [320, 360, 620, 621, 900, 1440] : [preset.width]) {
        if (width !== preset.width || info.project.name.includes('reflow')) await page.setViewportSize({ width, height: preset.height })
        await expectNoPageOverflow(page)
        await page.screenshot({ path: info.outputPath(`funds-${locale}-${writable}-${width}.png`), fullPage: true })
      }
      if (writable) {
        await expect(withdrawalPanel.locator('.saved-summary')).toHaveCount(0)
        for (const text of await page.locator('.brand-settings-panel label, .brand-settings-panel legend').allTextContents()) expect(text).not.toMatch(/品牌：|Brand:|[（()）]/)
        await tabs.getByRole('button', { name: zh ? '资金' : 'Funds', exact: true }).click()
        await pointPanel.getByLabel(zh ? '单笔充值上限' : 'Per-recharge limit', { exact: true }).fill('50')
        await pointPanel.getByRole('button', { name: zh ? '保存' : 'Save', exact: true }).click()
        await expect.poll(() => pointWrites.length).toBe(1)
        await tabs.getByRole('button', { name: zh ? '提现设置' : 'Withdrawal settings', exact: true }).click()
        await withdrawalPanel.getByLabel(zh ? '提现积分步长' : 'Withdrawal increment', { exact: true }).fill('10')
        await withdrawalPanel.getByLabel(zh ? '最低提现积分' : 'Minimum withdrawal points', { exact: true }).fill('20')
        await withdrawalPanel.locator('button[type=submit]').click()
        const dialog = page.getByRole('dialog')
        await expect(dialog).toBeVisible()
        await expect(dialog.locator('.business-details > .business-details-row > dt')).toHaveText(zh ? ['启用', '最低积分', '最高积分', '提现积分步长', '允许使用的积分来源', '审核方式', '流水倍数'] : ['Enabled', 'Minimum points', 'Maximum points', 'Withdrawal increment', 'Allowed point sources', 'Approval method', 'Turnover multiplier'])
        await expect(page.locator('.toast')).toHaveCount(0)
        await expectNoPageOverflow(page)
        await page.screenshot({ path: info.outputPath(`funds-confirm-${locale}.png`) })
        await page.keyboard.press('Escape')
        await expect(dialog).toHaveCount(0)
        expect(withdrawalWrites).toHaveLength(0)
        await withdrawalPanel.locator('button[type=submit]').click()
        await dialog.getByRole('button', { name: zh ? '确认保存配置' : 'Save configuration', exact: true }).click()
        await expect(withdrawalPanel.locator('[role=alert]')).toBeVisible()
        await tabs.getByRole('button', { name: zh ? '运行状态' : 'Status', exact: true }).click()
        await tabs.getByRole('button', { name: zh ? '提现设置' : 'Withdrawal settings', exact: true }).click()
        await expect(withdrawalPanel.locator('[role=alert]')).toBeVisible()
        await expect(withdrawalPanel.locator('#withdraw-brand-min')).toHaveValue('20')
        await withdrawalPanel.locator('[role=alert] button').first().click()
        await expect(withdrawalPanel.locator('[role=alert]')).toHaveCount(0)
        await expect(withdrawalPanel.locator('#withdraw-brand-min')).toHaveValue('20')
        await tabs.getByRole('button', { name: zh ? '资金' : 'Funds', exact: true }).click()
        await expect(pointPanel.getByLabel(zh ? '单笔充值上限' : 'Per-recharge limit', { exact: true })).toHaveValue('50')
        expect(withdrawalWrites).toHaveLength(2)
        expect(JSON.parse(withdrawalWrites[0].body!).points_step).toBe('10')
      } else {
        await expect(page.locator('.brand-settings-panel input, .brand-settings-panel form')).toHaveCount(0)
        expect(pointWrites).toHaveLength(0)
        expect(withdrawalWrites).toHaveLength(0)
      }
      expect(requests.some(request => /history|wallet|members/.test(request))).toBe(false)
      expect(errors).toEqual([])
    })
  }
}

for (const locale of ['zh-CN', 'en'] as const) {
  test(`brand commission configuration ownership (${locale})`, async ({ page }, info) => {
    await installLayoutApi(page, locale)
    const zh = locale === 'zh-CN'
    const failures: string[] = []
    page.on('pageerror', error => failures.push(error.message))
    await page.route('**/api/v1/admin/**', async route => {
      expect(route.request().method()).toBe('GET')
      const path = new URL(route.request().url()).pathname
      const reply = (data: unknown) => route.fulfill({ json: { success: true, data } })
      if (path.endsWith('/me')) return reply({ account: { id: gameId, username: 'commission_viewer', super_admin: false, brand_ids: [gameId], permissions: [], permissions_by_brand: { [gameId]: ['commission_policy.view.brand'] }, platform_permissions: [] } })
      if (path.endsWith('/brands')) return reply({ items: [{ id: gameId, code: 'layout', allow_custom_games: true, name: 'Layout trial', status: 'active', member_count: '0', agent_count: '0', recharge_quota_available: '0', created_at: '2026-10-24T00:00:00Z' }] })
      if (path.endsWith('/commission-policy')) return reply({ brand_id: gameId, version: 1, config: { enabled: true, calendar: { timezone: 'Asia/Singapore', cycle: 'weekly', boundary_time: '00:00:00', weekday: 1, month_day: null, short_month: '' }, payout_mode: 'manual' }, created_at: '2026-10-24T00:00:00Z', updated_at: '2026-10-24T00:00:00Z', revision_id: gameId })
      throw Error(`Unexpected commission ownership request: ${path}`)
    })
    await page.goto(origins.brand)
    await expect(page.locator('.management-console')).toBeVisible()
    const preset = page.viewportSize()!
    for (const width of info.project.name.includes('reflow') ? [320, 640, 900, 901, 1440] : [preset.width]) {
      if (info.project.name.includes('reflow')) await page.setViewportSize({ width, height: preset.height })
      const navigate = async (name: string) => {
        const more = page.locator('.mobile-nav > button').last()
        const mobile = await more.isVisible()
        if (mobile && await more.getAttribute('aria-expanded') !== 'true') await more.click()
        await page.locator(mobile ? '.mobile-more-menu' : '.side-nav').getByRole('button', { name, exact: true }).click()
      }
      await navigate(zh ? '品牌设置' : 'Brand settings')
      await page.locator('.console-tabs').getByRole('button', { name: zh ? '代理设置' : 'Agent settings', exact: true }).click()
      const panel = page.locator('article').filter({ has: page.getByRole('heading', { name: zh ? '平台佣金设置' : 'Platform commission settings', exact: true }) })
      await expect(panel).toContainText(zh ? '由总后台统一管理' : 'platform admin manages')
      await expect(panel.locator('dl')).toContainText(zh ? '每周' : 'Weekly')
      await expect(panel.locator('input, select, button')).toHaveCount(0)
      await expectNoPageOverflow(page)
      await page.screenshot({ path: info.outputPath(`commission-settings-${locale}-${width}.png`), fullPage: true })
      await navigate(zh ? '佣金管理' : 'Commission management')
      await expect(page.locator('.breadcrumbs b')).toHaveText(zh ? '佣金管理' : 'Commission management')
      await expect(page.getByRole('heading', { name: zh ? '佣金管理' : 'Commission management', exact: true })).toBeVisible()
      await expect(page.locator('.page-content')).toContainText(zh ? '没有此品牌佣金周期查看权限' : 'cannot view commission cycles')
      await expectNoPageOverflow(page)
    }
    expect(failures).toEqual([])
  })
}

for (const locale of ['zh-CN', 'en'] as const) {
  test(`brand agent settings load only policies (${locale})`, async ({ page }, info) => {
    await installLayoutApi(page, locale)
    const zh = locale === 'zh-CN', at = '2026-10-27T00:00:00Z'
    await page.route('**/api/v1/admin/**', async route => {
      expect(route.request().method()).toBe('GET')
      const path = new URL(route.request().url()).pathname
      const reply = (data: unknown) => route.fulfill({ json: { success: true, data } })
      if (path.endsWith('/me')) return reply({ account: { id: gameId, username: 'agent_settings_reviewer', super_admin: false, brand_ids: [gameId], permissions_by_brand: { [gameId]: ['agent_policy.view.brand', 'agent_policy.write.brand', 'commission_policy.view.brand'] } } })
      if (path.endsWith('/brands')) return reply({ items: [{ id: gameId, code: 'layout', name: 'Aurora', status: 'active', member_count: '0', agent_count: '0', recharge_quota_available: '0', created_at: at }] })
      expect(route.request().headers()['x-brand-id']).toBe(gameId)
      if (path.endsWith('/agent-policy')) return reply({ brand_id: gameId, version: 1, config: { enabled: true, max_depth: 5, loss_ratio_cap: '0.25', turnover_ratio_cap: '0.03', mode: 'loss', cycle: 'monthly' }, updated_at: at })
      if (path.endsWith('/commission-policy')) return reply({ brand_id: gameId, version: 1, config: { enabled: false, calendar: null, payout_mode: 'manual' }, created_at: at, updated_at: at, revision_id: gameId })
      throw Error(`Unexpected agent settings request: ${path}`)
    })
    await page.goto(origins.brand)
    await expect(page.locator('.management-console')).toBeVisible()
    const more = page.locator('.mobile-nav > button').last()
    if (await more.isVisible()) await more.click()
    await page.locator(await more.isVisible() ? '.mobile-more-menu' : '.side-nav').getByRole('button', { name: zh ? '品牌设置' : 'Brand settings', exact: true }).click()
    await page.locator('.console-tabs').getByRole('button', { name: zh ? '代理设置' : 'Agent settings', exact: true }).click()
    const panel = page.locator('.agent-management')
    await expect(panel.locator('.am-facts')).toContainText('25%')
    const preset = page.viewportSize()!
    for (const width of info.project.name.includes('reflow') ? [320, 900, 901, 1440] : [preset.width]) {
      if (info.project.name.includes('reflow')) await page.setViewportSize({ width, height: preset.height })
      await expectNoPageOverflow(page)
      await page.screenshot({ path: info.outputPath(`agent-settings-${locale}-${width}.png`), fullPage: true })
    }
    await panel.getByRole('button', { name: zh ? '编辑' : 'Edit', exact: true }).click()
    const dialog = page.getByRole('dialog')
    await expect(dialog.getByRole('textbox', { name: zh ? '占成比例上限' : 'Profit share cap', exact: true })).toHaveValue('25')
    await dialog.getByRole('button', { name: zh ? '关闭' : 'Close', exact: true }).click()
    await expect(dialog).toHaveCount(0)
  })
}

for (const locale of ['zh-CN', 'en'] as const) {
 test(`brand settings actions align inside panels (${locale})`, async ({page},info) => {
  await installLayoutApi(page,locale)
  const zh=locale==='zh-CN',at='2026-11-12T00:00:00Z'
  const errors:string[]=[]
  page.on('pageerror',error=>errors.push(error.message))
  const permissions=['brand_operation.view.brand','brand_operation.write.brand','point_policy.view.brand','point_policy.write.brand','agent_policy.view.brand','agent_policy.write.brand','commission_policy.view.brand']
  await page.route('**/api/v1/admin/**',route=>{
   expect(route.request().method()).toBe('GET')
   const path=new URL(route.request().url()).pathname
   const reply=(data:unknown)=>route.fulfill({json:{success:true,data}})
   if(path.endsWith('/me'))return reply({account:{id:gameId,username:'settings_actions_reviewer',super_admin:false,brand_ids:[gameId],permissions_by_brand:{[gameId]:permissions}}})
   if(path.endsWith('/brands'))return reply({items:[{id:gameId,code:'layout',name:'Aurora',status:'active',member_count:'0',agent_count:'0',recharge_quota_available:'0',created_at:at}]})
   if(path.endsWith('/brand-operation'))return reply({brand_id:gameId,name:'Aurora',status:'active',version:1,updated_at:at})
   if(path.endsWith('/point-policy'))return reply({brand_id:gameId,version:1,max_balance_points:'10000',max_recharge_points:null,max_adjustment_points:null})
   if(path.endsWith('/cash/config'))return reply({version:1,rules:['CNY','USD','THB','USDT'].map(currency=>({currency,rate:'7',deposit_min:'1',deposit_max:'10000',withdrawal_min:'1',withdrawal_max:'10000',fee:'0',mode:'count',threshold:'2',enabled:true}))})
   if(path.endsWith('/cash/destinations'))return reply([])
   if(path.endsWith('/agent-policy'))return reply({brand_id:gameId,version:1,config:{enabled:true,max_depth:5,loss_ratio_cap:'0.25',turnover_ratio_cap:'0.03',mode:'loss',cycle:'monthly'},updated_at:at})
   if(path.endsWith('/commission-policy'))return reply({brand_id:gameId,version:1,config:{enabled:false,calendar:null,payout_mode:'manual'},created_at:at,updated_at:at,revision_id:gameId})
   throw Error(`Unexpected settings request: ${path}`)
  })
  await page.goto(origins.brand)
  await expect(page.locator('.management-console')).toBeVisible()
  const more=page.locator('.mobile-nav > button').last()
  if(await more.isVisible())await more.click()
  await page.locator(await more.isVisible()?'.mobile-more-menu':'.side-nav').getByRole('button',{name:zh?'品牌设置':'Brand settings',exact:true}).click()
  const preset=page.viewportSize()!
  const review=async(panel:ReturnType<typeof page.locator>,name:string)=>{
   for(const width of info.project.name.includes('reflow')?[320,700,900,1440]:[preset.width]){
    if(info.project.name.includes('reflow'))await page.setViewportSize({width,height:preset.height})
    const actions=panel.locator('.ui-form-actions').last()
    await expect(actions).toBeVisible()
    const button=actions.locator('button').last(),area=await actions.boundingBox(),box=await button.boundingBox()
    expect(Math.abs(area!.x+area!.width-box!.x-box!.width)).toBeLessThanOrEqual(1)
    expect(await actions.evaluate(node=>getComputedStyle(node).justifyContent)).toBe('flex-end')
    await expectNoPageOverflow(page)
    await page.screenshot({path:info.outputPath(`${name}-${locale}-${width}.png`),fullPage:true})
   }
  }
  await review(page.locator('.brand-operation'),'status-actions')
  await page.locator('.console-tabs').getByRole('button',{name:zh?'资金':'Funds',exact:true}).click()
  const points=page.locator('.point-policy-settings')
  await expect(points.locator('form')).toBeVisible()
  await review(points,'points-actions')
  await page.locator('.console-tabs').getByRole('button',{name:zh?'代理设置':'Agent settings',exact:true}).click()
  const agents=page.locator('.agent-management')
  await expect(agents.locator('.am-facts')).toBeVisible()
  await review(agents,'agent-edit-actions')
  await agents.getByRole('button',{name:zh?'编辑':'Edit',exact:true}).click()
  await review(page.getByRole('dialog'),'agent-save-actions')
  await expect(page.locator('.toast')).toHaveCount(0)
  expect(errors).toEqual([])
 })
}
