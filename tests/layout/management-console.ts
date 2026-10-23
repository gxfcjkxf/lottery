import { expect, test, type Page } from '@playwright/test'
import { origins, gameId, expectNoPageOverflow, expectInsideViewport, expectCompactPageLayout } from './fixture'

const otherBrand = 'bbbbbbbb-bbbb-4bbb-8bbb-000000000001'
const memberId = 'cccccccc-cccc-4ccc-8ccc-000000000001'
const buckets = Object.fromEntries(['recharge', 'winning', 'gift', 'commission'].map(source => [source, { available: '0', manual_frozen: '0', system_frozen: '0', withdrawal: '0' }]))

async function openMembers(page: Page, zh: boolean) {
  await expect(page.locator('.management-console')).toBeVisible()
  await expect(page.locator('.console-brand-scope')).toHaveCount(0)
  if (await page.locator('.mobile-nav').isVisible()) await page.locator('.mobile-nav button').nth(1).click()
  else await page.locator('.side-nav').getByRole('button', { name: zh ? '会员管理' : 'Member management', exact: true }).click()
}
async function openFunds(page: Page, zh: boolean) {
  await expect(page.locator('.management-console')).toBeVisible()
  await expect(page.locator('.console-brand-scope')).toHaveCount(0)
  if (await page.locator('.mobile-nav').isVisible()) await page.locator('.mobile-nav button').nth(3).click()
  else await page.locator('.side-nav').getByRole('button', { name: /资金与账本|Funds.*ledger/i }).click()
}

async function openManagementPage(page: Page, name: string) {
  await expect(page.locator(".management-console")).toBeVisible()
  if (await page.locator('.mobile-nav').isVisible()) {
    await page.locator('.mobile-nav button').last().click()
    const item = page.locator('.mobile-more-menu').getByRole('button', { name, exact: true })
    if (!await item.isVisible()) await item.locator('xpath=ancestor::details').locator('summary').click()
    await item.click()
  } else await page.locator('.side-nav').getByRole('button', { name, exact: true }).click()
}

async function expectConsoleHeader(page: Page) {
  await expectCompactPageLayout(page)
  await expect(page.locator('.top-actions .search-box')).toHaveCount(0)
  await expectInsideViewport(page.locator('.topbar'))
}

export function registerManagementConsoleTests(reflow = false) {
  for (const locale of ['en', 'zh-CN'] as const) {
    test(`management console keeps member actions and financial state scoped (${locale})`, async ({ page }, info) => {
      const zh = locale === 'zh-CN'
      let hostBrandId = gameId
      await page.context().addCookies([{ name: 'lottery-locale', value: locale, url: origins.brand }])
      const errors: string[] = [], reads: { path: string; brand: string | undefined }[] = [], writes: string[] = []
      page.on('pageerror', error => errors.push(error.message))
      const grants = ['user.view.brand', 'user.create.brand', 'wallet.view.brand', 'recharge.view.brand', 'withdrawal.view.brand', 'brand_presentation.view.brand', 'brand_presentation.write.brand']
      const member = { id: memberId, global_user_id: memberId, brand_id: gameId, username: 'console_member', online: false, display_points: '0', display_name: 'Console member', phone: '', status: 'normal', tags: ['trial'], notes: 'Long member note for mobile layout', joined_at: '2026-10-19T00:00:00Z' }
      await page.route('**/api/**', async route => {
        const request = route.request(), url = new URL(request.url()), path = url.pathname, brand = request.headers()['x-brand-id']
        if (request.method() !== 'GET') writes.push(path)
        expect(request.method(), path).toBe('GET')
        reads.push({ path, brand })
        const reply = (data: unknown) => route.fulfill({ json: { success: true, data, associations: { [`member:${memberId}`]: { name: member.username } } } })
        if (path === '/api/v1/display-settings') return reply({ timezone: 'Asia/Singapore', date_format: 'YYYY-MM-DD', time_format: 'HH:mm:ss' })
        if (path === '/api/v1/context') return reply({ brand: { id: hostBrandId, name: hostBrandId === gameId ? 'Aurora' : 'Harbor', status: 'active', default_locale: locale, timezone: 'Asia/Singapore' } })
        if (path === '/api/v1/admin/me') return reply({ account: { id: gameId, username: 'console_operator', super_admin: false, permissions: [], platform_permissions: [], brand_ids: [gameId, otherBrand], permissions_by_brand: { [gameId]: grants, [otherBrand]: ['user.view.brand'] } } })
        if (path === '/api/v1/admin/brands') return reply({ items: [{ id: gameId, name: 'Aurora', code: 'aurora', status: 'active' }, { id: otherBrand, name: 'Harbor', code: 'harbor', status: 'active' }] })
        if (path === '/api/v1/admin/brand-presentation' || path === '/api/v1/admin/brand-presentation/defaults') {
          const effective = { display_name: 'Aurora', logo_text: 'A', logo_url: null, favicon_url: null,
            primary_color: '#059669', accent_color: '#f59e0b', success_color: '#10b981', warning_color: '#f59e0b', danger_color: '#ef4444',
            font_family: 'system', font_scale: 'standard', radius: 'round', shadow: 'subtle', default_locale: 'en',
            available_locales: ['en', 'zh-CN'], content: { en: { tagline: '', announcement: '' }, 'zh-CN': { tagline: '', announcement: '' } } }
          if(path.endsWith('/defaults')) return reply(effective)
          return reply({ brand_id: gameId, version: 1, status: 'active', base_name: 'Aurora',
            config: Object.fromEntries(Object.keys(effective).map(key => [key, null])), effective, updated_at: '2026-10-21T00:00:00Z' })
        }
        if (path === '/api/v1/admin/brand-presentation/history') return reply({ items: [], limit: 20, offset: 0 })
        if (path === '/api/v1/admin/users') return reply({ items: brand === gameId ? [member] : [], limit:100, offset:0, total_count:brand === gameId ? '1' : '0' })
        if (path === '/api/v1/admin/games') return reply({ games: [{ id: gameId, brand_id: gameId, name: 'Console game', code: 'console_game', version: 1, timezone: 'Asia/Singapore', status: 'active', model: { model: 'DIGITS_0_9', regular_count: 0, special_count: 0, pool_size: 0, total_count: 0, length: 3, allow_repeat: true, ordered: true, regular_pool: { min: 0, max: 0, values: [], allow_repeat: false }, special_pool: { min: 0, max: 0, values: [], allow_repeat: false } } }] })
        if (path === `/api/v1/admin/games/${gameId}/schedule`) return route.fulfill({ status: 404, json: { success: false, error: { code: 'NOT_FOUND', message: 'No saved schedule' } } })
        if (path === `/api/v1/admin/games/${gameId}/periods`) return reply({ periods: [{ id: memberId, brand_id: gameId, game_id: gameId, period_no: 'CONSOLE-20261019', sequence: 1, version: 1, status: 'waiting_draw', bet_start_at: '2026-10-19T00:00:00Z', bet_end_at: '2026-10-19T01:00:00Z', draw_at: '2026-10-19T01:05:00Z' }], limit: Number(url.searchParams.get('limit')), offset: Number(url.searchParams.get('offset')) })
        if (path === '/api/v1/admin/recharges') return reply({ items: [{ id: 'RECHARGE-CONSOLE-001', brand_id: gameId, member_id: memberId, points: '10', state: 'confirmed', version: 2, created_by: gameId, confirmed_by: gameId, created_at: '2026-10-19T00:00:00Z', confirmed_at: '2026-10-19T00:00:00Z', proof_reference: 'CONSOLE-PROOF', remark: 'Synthetic recharge details' }] })
        if (path === '/api/v1/admin/withdrawals') return reply({ brand_id: gameId, items: [], limit: 100, offset: 0, has_more: false })
        if (path === `/api/v1/admin/wallets/${memberId}`) return reply({ account_id: memberId, brand_id: gameId, member_id: memberId, version: 1, display_points: '0', available_points: '0', frozen_points: '0', withdrawal_points: '0', recharge_points: '0', winning_points: '0', gift_points: '0', commission_points: '0', manual_frozen_points: '0', system_frozen_points: '0', by_source: buckets })
        if (path === `/api/v1/admin/wallets/${memberId}/ledger`) return reply({ items: [] })
        if (path === `/api/v1/admin/wallets/${memberId}/reconciliation`) return reply({ consistent: true, account_id: memberId, member_id: memberId, version: 1, entry_count: 0, expected: buckets, actual: buckets, issues: [] })
        throw Error(`Unexpected console request: ${path}`)
      })
      await page.goto(origins.brand)
      await expect(page.locator('.management-console')).toBeVisible()
      await expect(page.locator('.console-brand-scope')).toHaveCount(0)
      await expect(page.locator('.directory-brand-bar, .brand-switch')).toHaveCount(0)
      await expectConsoleHeader(page)
      await openFunds(page, zh)
      await page.locator('.console-tabs').getByRole('button', { name: zh ? '充值记录' : 'Recharge records', exact: true }).click()
      await expect(page.locator('.console-tabs button[aria-current="page"]')).toHaveText(zh ? '充值记录' : 'Recharge records')
      await page.locator('.console-tabs').getByRole('button', { name: zh ? '提现申请' : 'Withdrawal requests', exact: true }).click()
      await expect(page.locator('.console-tabs button[aria-current="page"]')).toHaveText(zh ? '提现申请' : 'Withdrawal requests')
      await openMembers(page, zh)
      await expect(page.locator('.member-directory-table')).toContainText('console_member')
      await expect(page.locator('.provision')).toBeHidden()
      const create = page.locator('.console-page-actions').getByRole('button', { name: zh ? '添加会员' : 'Add member', exact: true })
      await create.click()
      await expect(page.locator('.provision')).toBeVisible()
      await page.locator('.provision input[autocomplete="off"]').first().fill('unsubmitted_draft')
      await page.keyboard.press('Escape'); await create.click()
      await expect(page.locator('.provision input[autocomplete="off"]').first()).toHaveValue('unsubmitted_draft')
      await page.keyboard.press('Escape')
      await page.getByRole('button', {name:'console_member',exact:true}).click()
      await page.getByRole('dialog').getByRole('button', { name: zh ? '查看积分' : 'View points', exact: true }).click()
      await expect(page.locator('.wallet-panel')).toBeVisible()
      await expect(page.locator('.finance-management .lookup input')).toHaveValue('console_member')
      await expect(page.locator('.finance-breakdown')).toHaveCount(2)
      await expect(page.locator('.bucket-table')).toBeHidden()
      await page.locator('.finance-breakdown').first().locator('summary').click()
      await expect(page.locator('.bucket-row')).toHaveCount(16)
      await expect(page.locator('.bucket-table')).toBeVisible()
      await page.locator('.finance-breakdown').first().locator('summary').click()
      const tabs = page.locator('.console-tabs')
      await tabs.getByRole('button', { name: zh ? '充值记录' : 'Recharge records', exact: true }).click()
      await expect(page.locator('.wallet-panel')).toBeHidden()
      await expect(page.locator('.console-recharges-table')).toContainText('RECHARGE-CONSOLE-001')
      await page.locator('.console-recharges-table').getByRole('button').click()
      await expect(page.locator('.recharge-row')).toBeVisible()
      await expect(page.locator('.recharge-row')).toContainText('CONSOLE-PROOF')
      await expect(page.locator('.recharge-row button')).toHaveCount(0)
      await page.screenshot({ path: info.outputPath(`console-recharges-${locale}.png`) })
      await tabs.getByRole('button', { name: zh ? '提现申请' : 'Withdrawal requests', exact: true }).click()
      await expect(page.locator('.withdrawal-management')).toBeVisible()
      await expect(page.locator('.withdrawal-error')).toHaveCount(0)
      const withdrawalFilter = page.locator('.withdrawal-filters input')
      await withdrawalFilter.fill('retained_filter')
      await tabs.getByRole('button', { name: zh ? '会员积分与账本' : 'Member points & ledger', exact: true }).click()
      await expect(page.locator('.wallet-panel')).toBeVisible()
      await tabs.getByRole('button', { name: zh ? '提现申请' : 'Withdrawal requests', exact: true }).click()
      await expect(withdrawalFilter).toHaveValue('retained_filter')
      if (reflow) {
        for (const width of [320, 360, 412, 640, 699, 700, 701, 768, 899, 900, 901, 1024, 1079, 1080, 1081, 1440]) {
          await page.setViewportSize({ width, height: 720 })
          await expectNoPageOverflow(page)
          await expectConsoleHeader(page)
        }
      }
      await expectNoPageOverflow(page)
      await page.screenshot({ path: info.outputPath(`console-finance-${locale}.png`) })
      await openMembers(page, zh)
      await expectNoPageOverflow(page)
      if (await page.locator('.mobile-nav').isVisible()) {
        await page.locator('.mobile-nav button').last().click()
        const menu = page.locator('.mobile-more-menu')
        await expectInsideViewport(menu)
        await expect(menu.locator('.console-mobile-group')).toHaveCount(7)
        await page.keyboard.press('Escape')
        await expect(menu).toHaveCount(0)
      }
      if ((page.viewportSize()?.width ?? 0) <= 700) {
        const clippedCells = await page.locator('.console-responsive-table td').evaluateAll(cells => cells.filter(cell => cell.scrollHeight > cell.clientHeight + 1).map(cell => cell.textContent))
        expect(clippedCells).toEqual([])
      }
      await page.screenshot({ path: info.outputPath(`console-members-${locale}.png`) })
      await page.locator('.account-trigger').click()
      await page.getByRole('menuitem', { name: zh ? '个人信息' : 'Profile', exact: true }).click()
      const profile = page.getByRole('dialog', { name: zh ? '个人信息' : 'Profile', exact: true })
      await expect(profile).toContainText('console_operator')
      await expectInsideViewport(profile)
      expect((await profile.boundingBox())!.width).toBeLessThanOrEqual(520)
      await profile.locator('.detail-modal-close').click()
      await expect(profile).toHaveCount(0)
      await page.locator('.account-trigger').click()
      await page.getByRole('menuitem', { name: zh ? '修改密码' : 'Change password', exact: true }).click()
      const password = page.getByRole('dialog', { name: zh ? '修改密码' : 'Change password', exact: true })
      await expect(password.locator('input[type="password"]')).toHaveCount(3)
      await expectInsideViewport(password)
      await expectInsideViewport(password.locator('.detail-modal-close'))
      await password.locator('.detail-modal-close').click()
      await expect(password).toHaveCount(0)
      await openManagementPage(page, zh ? '品牌设置' : 'Brand settings')
      await expect(page.locator('.brand-operation')).toBeVisible()
      await expect(page.locator('.brand-presentation')).toBeHidden()
      await expect(page.locator('.console-tabs button')).toHaveCount(3)
      await expect(page.locator('.side-nav')).not.toContainText(zh ? '风控与合规' : 'Risk & compliance')
      await page.locator('.console-tabs button').nth(1).click()
      const presentation = page.locator('.brand-presentation')
      await expect(presentation.locator('.presentation-fields')).toBeVisible()
      await expect(presentation.locator('header small, .presentation-summary')).toHaveCount(0)
      await expect(presentation.locator('.inherit-toggle')).toHaveCount(0)
      const choice = presentation.locator('.choice').first()
      await expect(choice.locator('input')).toHaveCSS('width', '18px')
      expect((await choice.boundingBox())!.height).toBeGreaterThanOrEqual(44)
      await expect(presentation.locator('input[type="text"]').first()).toBeEnabled()
      await expectNoPageOverflow(page)
      await presentation.screenshot({ path: info.outputPath(`brand-settings-presentation-${locale}.png`) })
      await page.locator('.console-tabs button').nth(2).click()
      await expect(page.locator('.brand-domains')).toBeVisible()
      await expect(page.locator('.console-tabs button')).toHaveCount(3)
      await expect(page.locator('.brand-operation')).toBeHidden()
      await openManagementPage(page, zh ? '期次和开奖' : 'Periods & draws')
      await expect(page.locator('.period-schedules')).toBeVisible()
      await expect(page.locator('.correction-management')).toBeHidden()
      await page.locator('.console-tabs button').nth(1).click()
      await expect(page.locator('.draw-management')).toBeVisible()
      await expect(page.locator('.period-schedules')).toBeHidden()
      await expectNoPageOverflow(page)
      grants.push('user.write.brand')
      await page.reload()
      await openMembers(page, zh)
      await page.locator('.member-menu-trigger').first().click()
      await page.getByRole('menuitem', { name: zh ? '编辑' : 'Edit', exact: true }).click()
      const editor = page.getByRole('dialog', { name: zh ? '编辑会员' : 'Edit member', exact: true })
      await expect(editor).toBeVisible()
      await expect(editor.getByRole('textbox', { name: zh ? '操作原因' : 'Reason', exact: true })).toHaveCount(0)
      await expect(editor.getByRole('button', { name: zh ? '保存' : 'Save', exact: true })).toBeEnabled()
      await expect(editor.locator('select option')).toHaveText(zh ? ['启用', '冻结', '已停用', '已过期', '已注销'] : ['Active', 'Frozen', 'Disabled', 'Expired', 'Cancelled'])
      await expectInsideViewport(editor)
      await page.keyboard.press('Escape')
      await expect(editor).toHaveCount(0)
      grants.push('game.view.brand', 'schedule.view.brand', 'schedule.write.brand', 'period.view.brand', 'period.generate.brand')
      await page.reload()
      await openManagementPage(page, zh ? '期次和开奖' : 'Periods & draws')
      const schedules = page.locator('.period-schedules')
      await expect(schedules.locator('.period-card')).toContainText('CONSOLE-20261019')
      const scheduleEditor = schedules.locator('.console-disclosure').first()
      await expect(scheduleEditor.locator('.form-grid')).toBeHidden()
      await scheduleEditor.locator('summary').click()
      await scheduleEditor.locator('input[placeholder="12:00:00, 19:00:00"]').fill('18:30:00')
      await scheduleEditor.locator('summary').click()
      await page.locator('.console-tabs button').nth(1).click()
      await expect(page.locator('.draw-management select').nth(1)).toContainText('CONSOLE-20261019')
      await expectNoPageOverflow(page)
      await page.locator('.console-tabs button').nth(0).click()
      await scheduleEditor.locator('summary').click()
      await expect(scheduleEditor.locator('input[placeholder="12:00:00, 19:00:00"]')).toHaveValue('18:30:00')
      await expectNoPageOverflow(page)
      hostBrandId = otherBrand
      await page.reload()
      await openMembers(page, zh)
      await expect(page.locator('.member-directory-table')).toHaveCount(0)
      await expect(page.locator('.provision')).toBeHidden()
      await openFunds(page, zh)
      await expect(page.locator('.wallet-panel')).toHaveCount(0)
      expect(writes).toEqual([])
      expect(reads.filter(read => read.path.includes('/wallets/')).every(read => read.brand === gameId)).toBe(true)
      expect(errors).toEqual([])
    })
  }
}

// The new financial views must retain the original pending action, not remount
// the withdrawal component and generate another request after a lost receipt.
export function registerFinancialViewRetryTest() {
  test('financial view switching retains the original withdrawal request and key', async ({ page }) => {
    await page.context().addCookies([{ name: 'lottery-locale', value: 'en', url: origins.brand }])
    const orderId = 'dddddddd-dddd-4ddd-8ddd-000000000001'
    const stamp = '2026-10-19T00:00:00Z'
    const order = { id: orderId, brand_id: gameId, member_id: memberId, account_id: memberId, points: '10', state: 'reviewing', version: 1, source_allocation: [{ source: 'recharge', state: 'available', points: '10' }], reserve_entry_id: orderId, release_entry_id: null, paid_entry_id: null, cycle_from_at: null, cycle_from_version: '0', reserve_version: '1', created_at: stamp, updated_at: stamp, reviewed_at: null, completed_at: null, decision_reason: '', audit_log_id: orderId }
    const attempts: { body: string | null; key: string | undefined; brand: string | undefined }[] = []
    await page.route('**/api/**', async route => {
      const req = route.request(), path = new URL(req.url()).pathname
      const reply = (data: unknown) => route.fulfill({ json: { success: true, data } })
      if (path === `/api/v1/admin/withdrawals/${orderId}/approve`) {
        expect(req.method()).toBe('POST')
        attempts.push({ body: req.postData(), key: req.headers()['idempotency-key'], brand: req.headers()['x-brand-id'] })
        if (attempts.length === 1) return route.fulfill({ status: 503, json: { success: false, error: { code: 'RECEIPT_LOST', message: 'Synthetic lost receipt' } } })
        return reply({ ...order, state: 'processing', version: 2, reviewed_at: stamp })
      }
      expect(req.method(), path).toBe('GET')
      if (path === '/api/v1/display-settings') return reply({ timezone: 'Asia/Singapore', date_format: 'YYYY-MM-DD', time_format: 'HH:mm:ss' })
      if (path === '/api/v1/context') return reply({ brand: { id: gameId, name: 'Aurora', status: 'active', default_locale: 'en', timezone: 'Asia/Singapore' } })
      if (path === '/api/v1/admin/me') return reply({ account: { id: gameId, username: 'withdrawal_operator', super_admin: false, permissions: [], platform_permissions: [], brand_ids: [gameId], permissions_by_brand: { [gameId]: ['user.view.brand', 'withdrawal.view.brand', 'withdrawal.approve.brand'] } } })
      if (path === '/api/v1/admin/brands') return reply({ items: [{ id: gameId, name: 'Aurora', code: 'aurora', status: 'active' }] })
      if (path === '/api/v1/admin/users') return reply({ items: [], limit:100, offset:0, total_count:'0' })
      if (path === '/api/v1/admin/workbench') return route.fulfill({ status: 403, json: { success: false, error: { code: 'FORBIDDEN', message: 'Synthetic denied read' } } })
      if (path === '/api/v1/admin/withdrawals') return reply({ brand_id: gameId, items: [order], limit: 100, offset: 0, has_more: false })
      if (path === `/api/v1/admin/withdrawals/${orderId}/history`) return reply({ brand_id: gameId, order_id: orderId, items: [] })
      throw Error(`Unexpected withdrawal fixture request: ${path}`)
    })
    await page.goto(origins.brand)
    await expect(page.locator('.console-brand-scope')).toHaveCount(0)
    await openFunds(page, false)
    const tabs = page.locator('.console-tabs')
    await tabs.getByRole('button', { name: 'Withdrawal requests', exact: true }).click()
    const panel = page.locator('.withdrawal-management')
    await panel.getByRole('button', { name: 'History', exact: true }).click()
    const dialog = page.getByRole('dialog')
    await dialog.getByRole('button', { name: 'Approve', exact: true }).click()
    await dialog.getByRole('button', { name: 'Confirm Approve', exact: true }).click()
    await expect(panel.getByRole('button', { name: 'Replay same action', exact: true })).toBeVisible()
    await dialog.locator('.detail-modal-close').click()
    await tabs.getByRole('button', { name: 'Member points & ledger', exact: true }).click()
    await tabs.getByRole('button', { name: 'Withdrawal requests', exact: true }).click()
    await expect(panel.getByRole('button', { name: 'Replay same action', exact: true })).toBeVisible()
    await panel.getByRole('button', { name: 'Replay same action', exact: true }).click()
    await expect(panel.getByRole('button', { name: 'Replay same action', exact: true })).toHaveCount(0)
    expect(attempts).toHaveLength(2)
    expect(attempts[0].key).toBeTruthy()
    expect(attempts[0].brand).toBe(gameId)
    expect(attempts[1]).toEqual(attempts[0])
    expect(JSON.parse(attempts[0].body!)).toEqual({ version: 1, reason: '' })
  })
}

export function registerFixedBrandScopeTests() {
  for (const mode of ['unassigned', 'unavailable'] as const) {
    test(`fixed brand scope blocks ${mode} host context without choosing an account brand`, async ({ page }) => {
      await page.context().addCookies([{ name: 'lottery-locale', value: 'en', url: origins.brand }])
      const businessReads: string[] = []
      await page.route('**/api/**', async route => {
        const req = route.request(), path = new URL(req.url()).pathname
        expect(req.method()).toBe('GET')
        const reply = (data: unknown) => route.fulfill({ json: { success: true, data } })
        if (path === '/api/v1/display-settings') return reply({ timezone: 'Asia/Singapore', date_format: 'YYYY-MM-DD', time_format: 'HH:mm:ss' })
        if (path === '/api/v1/context') {
          if (mode === 'unavailable') return route.fulfill({ status: 503, json: { success: false, error: { code: 'UNAVAILABLE', message: 'Synthetic context failure' } } })
          return reply({ brand: { id: otherBrand, name: 'Harbor', status: 'active' } })
        }
        if (path === '/api/v1/admin/me') return reply({ account: { id: gameId, username: 'aurora_only', super_admin: false, permissions: [], brand_ids: [gameId], permissions_by_brand: { [gameId]: ['user.view.brand'] } } })
        if (path === '/api/v1/admin/brands') return reply({ items: [{ id: gameId, name: 'Aurora', status: 'active' }] })
        businessReads.push(path)
        throw Error(`Unexpected business access without an authorized host brand: ${path}`)
      })
      await page.goto(origins.brand)
      const state = page.locator('.console-brand-scope [role="alert"]')
      await expect(state).toContainText(mode === 'unassigned' ? 'This account is not authorized' : 'Unable to identify the brand')
      await expect(page.locator('.directory-brand-bar, .brand-switch')).toHaveCount(0)
      await expect(page.locator('.workbench, .member-directory-table')).toHaveCount(0)
      await expectNoPageOverflow(page)
      expect(businessReads).toEqual([])
    })
  }
}
