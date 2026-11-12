import { resolve } from 'node:path'
import { selectControl } from '../select-control'
import { expect, test } from '@playwright/test'
import { expectNoPageOverflow, gameId, installLayoutApi, origins } from './fixture'

for (const locale of ['zh-CN', 'en'] as const) test(`member finance opens from row actions and preserves the member query (${locale})`, async ({ page }, info) => {
  test.setTimeout(120_000)
  await installLayoutApi(page, locale)
  const zh = locale === 'zh-CN', memberId = 'bbbbbbbb-bbbb-4bbb-8bbb-000000000001'
  let walletAllowed = true
  let loseAdjustmentResponse = true
  const writes: { path: string; body: unknown; key: string | undefined }[] = []
  const memberOffsets: string[] = []
  const queries: string[] = [], errors: string[] = [], rechargeQueries: string[] = [], quotaQueries: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  const buckets = Object.fromEntries(['recharge', 'winning', 'gift', 'commission'].map(source => [source, { available: '0', manual_frozen: '0', system_frozen: '0', withdrawal: '0' }]))
  await page.route('**/api/v1/admin/**', async route => {
    const req = route.request(), url = new URL(req.url()), path = url.pathname
    const reply = (data: unknown) => route.fulfill({ json: { success: true, data, associations: { [`brand:${gameId}`]: { name: 'Aurora' }, [`member:${memberId}`]: { name: 'member001' }, [`admin:${gameId}`]: { name: 'finance_reviewer' } } } })
    if (req.method() === 'POST') {
      expect(req.headers()['x-brand-id']).toBe(gameId)
      expect(path).toMatch(new RegExp(`/wallets/${memberId}/(freeze|adjust)$`))
      writes.push({ path, body: req.postDataJSON(), key: req.headers()['idempotency-key'] })
      if (loseAdjustmentResponse) { loseAdjustmentResponse = false; return route.abort('failed') }
      return reply({})
    }
    expect(req.method()).toBe('GET')
    if (path.endsWith('/me')) return reply({ account: { id: gameId, username: 'finance_reviewer', super_admin: false, brand_ids: [gameId], permissions: [], permissions_by_brand: { [gameId]: ['user.view.brand', ...(walletAllowed ? ['wallet.view.brand'] : []), 'recharge.view.brand', 'wallet.freeze.brand', 'wallet.adjust.brand'] } } })
    if (path.endsWith('/brands')) return reply({ items: [{ id: gameId, code: 'layout', name: 'Aurora', status: 'active', member_count: '1', agent_count: '0', recharge_quota_available: '0', created_at: '2026-10-24T00:00:00Z' }] , total_count: '1'})
    expect(req.headers()['x-brand-id']).toBe(gameId)
    if (path.endsWith('/filter-options')) return reply({brand_id:gameId,items:[{id:memberId,name:'member001'}],limit:100,offset:0})
    if (path.endsWith('/users')) {
      queries.push(url.searchParams.get('q') ?? '')
      memberOffsets.push(url.searchParams.get('offset') ?? '')
      return reply({ items: [{ id: memberId, global_user_id: memberId, brand_id: gameId, username: 'member001', phone: '', display_name: '', notes: '', tags: [], status: 'normal', online: false, display_points: '0', joined_at: '2026-10-24T00:00:00Z' }], limit: Number(url.searchParams.get('limit')), offset: Number(url.searchParams.get('offset')), total_count: '101' })
    }
    if (path.endsWith(`/wallets/${memberId}`)) return reply({ account_id: gameId, brand_id: gameId, member_id: memberId, version: 1, display_points: '0', available_points: '0', frozen_points: '0', withdrawal_points: '0', recharge_points: '0', winning_points: '0', gift_points: '0', commission_points: '0', manual_frozen_points: '0', system_frozen_points: '0', by_source: buckets })
    if (path.endsWith('/recharge-quota/entries')) {
      quotaQueries.push(url.searchParams.get('offset')!)
      const start = Number(url.searchParams.get('offset'))
      return reply({ items: Array.from({length: start ? 1 : 11}, (_, i) => ({id:`quota-${start+i}`,brand_id:gameId,kind:'grant',points:'10',before_available:'90',after_available:'100',actor_name:'finance_reviewer',created_at:'2026-10-24T00:00:00Z'})) , total_count: '11'})
    }
    if (path.endsWith('/recharge-quota')) return reply({ brand_id: gameId, available: '100' })
    if (path.endsWith('/recharges')) { rechargeQueries.push(url.searchParams.get('member_id') ?? ''); return reply({ total_count: '1', items: [{ id: gameId, brand_id: gameId, member_id: memberId, account_id: gameId, method: 'manual', points: '10', state: 'confirmed', proof_reference: '', remark: '', version: 1, created_by: gameId, created_at: '2026-10-24T00:00:00Z' }] }) }
    if (path.endsWith('/repair-preview')) return reply({ account_id: gameId, member_id: memberId, version: 1, ledger_version: 1, actual: buckets, expected: buckets, repairable: false, consistent: true, issues: [], token: '' })
    if (path.endsWith('/repairs')) return reply({ items: [] , total_count: '0'})
    if (path.endsWith('/ledger')) return reply({ total_count: '1', items: [{
      id: 'ledger-layout-freeze', brand_id: gameId, account_id: gameId, member_id: memberId,
      entry_type: 'freeze', reference_type: 'wallet', reference_id: gameId, operation_key: 'internal-layout-key',
      before_snapshot: { ...buckets, recharge: { ...buckets.recharge, available: '100' } },
      delta_snapshot: { ...buckets, recharge: { ...buckets.recharge, available: '-20', manual_frozen: '20' } },
      after_snapshot: { ...buckets, recharge: { ...buckets.recharge, available: '80', manual_frozen: '20' } },
      source_allocation: [{ source: 'recharge', state: 'available', points: '20' }],
      reason: 'confirmed configured stake', actor_type: 'admin', actor_id: gameId,
      reversal_of: null, created_at: '2026-10-24T00:00:00Z',
    }] })
    if (path.endsWith('/reconciliation')) return reply({ consistent: true, account_id: gameId, member_id: memberId, version: 1, entry_count: 0, expected: buckets, actual: buckets, issues: [] })
    throw Error(`Unexpected finance request: ${path}`)
  })
  await page.goto(origins.brand)
  await expect(page.locator('.management-console')).toBeVisible()
  const preset = page.viewportSize()!
  const moreMenu = page.locator('.mobile-nav > button').last()
  if (await moreMenu.isVisible()) await moreMenu.click()
  const nav = page.locator(await moreMenu.isVisible() ? '.mobile-more-menu' : '.side-nav')
  for (const name of [zh ? '积分' : 'Points', zh ? '账本' : 'Ledger']) await expect(nav.getByRole('button', {name, exact:true})).toHaveCount(0)
  await nav.getByRole('button', {name:zh?'会员管理':'Member management',exact:true}).click()
  const list = page.locator('.directory-page')
  const input = list.locator('.filters input').first()
  await input.fill('member001')
  await list.locator('.filters').getByRole('button', {name:zh?'查询':'Search',exact:true}).click()
  await expect(input).toHaveValue('member001')
  await list.locator('.table-pagination').getByRole('button',{name:zh?'下一页':'Next page',exact:true}).click()
  await expect.poll(() => memberOffsets.at(-1)).toBe('100')
  const openFinance = async (name: string) => {
    await list.locator('.member-menu-trigger').click()
    await page.getByRole('menuitem', { name, exact: true }).click()
  }
  for (const name of [zh ? '积分' : 'Points', zh ? '账本' : 'Ledger']) {
    await openFinance(name)
    const area = page.locator('.finance-management'), detail = area.getByRole('dialog')
    await expect(detail).toBeVisible()
    await expect(detail).toHaveAttribute('aria-label', `${name} · member001`)
    await expect(area.locator('.page-head, .finance-members, .lookup')).toHaveCount(0)
    await expect(list).toHaveCount(1)
    if (name === (zh ? '积分' : 'Points')) {
      const form = detail.locator('.operations-grid form')
      await expect(form).toHaveCount(1)
      await expect(form.getByRole('heading')).toHaveText(zh ? '积分调整' : 'Points adjustment')
      const type = form.getByRole('combobox', { name: zh ? '调整类型' : 'Adjustment type', exact: true })
      await type.click()
      await expect(page.getByRole('listbox', { name: zh ? '调整类型' : 'Adjustment type', exact: true }).getByRole('option')).toHaveCount(5)
      await page.keyboard.press('Escape')
      const amount = form.getByLabel(zh ? '调整额' : 'Amount', { exact: true })
      const submit = form.getByRole('button', { name: zh ? '提交调整' : 'Submit adjustment', exact: true })
      for (const [value, delta] of [['gift', '25'], ['commission', '-10'], ['freeze', '60']]) {
        await selectControl(type, value)
        if (value === 'freeze') {
          await amount.fill('-1'); await expect(submit).toBeDisabled()
          await amount.fill('0'); await expect(submit).toBeDisabled()
        }
        await amount.fill(delta)
        const remark = form.getByLabel(zh ? '备注' : 'Remark', { exact: true })
        await remark.fill(value === 'gift' ? '可选备注 adjustment' : '')
        const readsBeforeWrite = queries.length
        await submit.click()
        if (value === 'gift') {
          const retry = detail.getByRole('button', {name:zh?'重试原请求':'Retry original request',exact:true})
          await expect(retry).toBeVisible()
          await expect(amount).toBeDisabled()
          await expect(type).toBeDisabled()
          await expect(detail.locator('.detail-modal-close')).toBeDisabled()
          await page.keyboard.press('Escape')
          await expect(detail).toBeVisible()
          await retry.click()
          await expect(amount).toHaveValue('')
          expect(writes[1]).toEqual(writes[0])
        }
        await expect(amount).toHaveValue('')
        await expect.poll(() => queries.length).toBeGreaterThan(readsBeforeWrite)
        const write = writes.at(-1)!
        expect(write.path).toBe(`/api/v1/admin/wallets/${memberId}/${value === 'freeze' ? 'freeze' : 'adjust'}`)
        expect(write.body).toEqual(value === 'freeze' ? { points: delta, reason: '' } : { source: value, delta, reason: value === 'gift' ? '可选备注 adjustment' : '' })
        expect(write.key).toBeTruthy()
        const dismiss = page.locator('.operation-toast-stack').getByRole('button', { name: zh ? '关闭提示' : 'Dismiss message', exact: true })
        while (await dismiss.count()) await dismiss.first().click()
      }
      for (const width of info.project.name.includes('reflow') ? [320, 640, 900, 901, 1040, 1440] : [preset.width]) {
        if (info.project.name.includes('reflow')) await page.setViewportSize({ width, height: preset.height })
        await expectNoPageOverflow(page)
        expect((await amount.boundingBox())!.width).toBeLessThanOrEqual(140)
        await detail.locator('.detail-modal-content').evaluate(node => { node.scrollTop = 0 })
        await page.screenshot({ path: info.outputPath(`points-adjustment-${locale}-${width}.png`) })
      }
    }
    if (name === (zh ? '账本' : 'Ledger')) {
      const reconciliationDetails = detail.locator('details').filter({ has: page.locator('.recon-table') })
      await reconciliationDetails.locator('summary').click()
      await expect(reconciliationDetails.locator('.reconciliation-time')).toContainText(zh ? '查询时间' : 'Query time')
      await expect(reconciliationDetails.locator('time')).toHaveAttribute('datetime', /\d{4}-\d{2}-\d{2}T/)
      await expect(reconciliationDetails.locator('time')).toHaveText(/\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}/)
      await expect(reconciliationDetails.locator('.recon-row')).toHaveCount(0)
      await expect(reconciliationDetails.locator('.empty-state')).toHaveText(zh ? '各类积分余额均为 0。' : 'All points balances are zero.')
      const categories = reconciliationDetails.getByRole('checkbox', { name: zh ? '显示全部分类' : 'Show all categories', exact: true })
      await categories.check(); await expect(reconciliationDetails.locator('.recon-row')).toHaveCount(16)
      await categories.uncheck(); await expect(reconciliationDetails.locator('.recon-row')).toHaveCount(0)
      await page.screenshot({ path: info.outputPath(`reconciliation-zero-${locale}.png`) })
      await page.route('**/api/v1/admin/wallets/*/reconciliation', route => route.fulfill({ json: { success: true, data: {
        consistent: false, account_id: gameId, member_id: memberId, version: 1, entry_count: 1, issues: [],
        expected: { ...buckets, recharge: { ...buckets.recharge, available: '120' }, winning: { ...buckets.winning, available: '5' } },
        actual: { ...buckets, recharge: { ...buckets.recharge, available: '120' } },
      } } }))
      await page.keyboard.press('Escape')
      await openFinance(name)
      await expect(reconciliationDetails).toHaveAttribute('open', '')
      await expect(reconciliationDetails.locator('.recon-row')).toHaveCount(2)
      await expect(reconciliationDetails.locator('.recon-row').first()).toContainText('120')
      await expect(reconciliationDetails.locator('.recon-row').last()).toContainText('5')
      await expect(reconciliationDetails.locator('.recon-row').last()).toContainText('0')
      await categories.check(); await expect(reconciliationDetails.locator('.recon-row')).toHaveCount(16)
      await categories.uncheck(); await expect(reconciliationDetails.locator('.recon-row')).toHaveCount(2)
      await page.screenshot({ path: info.outputPath(`reconciliation-filtered-${locale}.png`) })
      await reconciliationDetails.locator('summary').click()
      const table = detail.locator('.ledger-table')
      await expect(table.locator('tbody tr')).toHaveCount(2)
      await expect(table).toContainText(zh ? '确认投注扣减' : 'Confirmed stake deduction')
      await expect(table).not.toContainText('confirmed configured stake')
      await expect(table).toContainText('−20')
      await expect(table).toContainText('+20')
      await expect(table.getByRole('button', { name: zh ? '整笔原路解冻' : 'Unfreeze entire original amount', exact: true })).toHaveCount(1)
      await expect(detail).not.toContainText('ledger-layout-freeze')
      await expect(detail).not.toContainText(zh ? '积分桶' : 'bucket')
      const all = detail.getByRole('checkbox', { name: zh ? '显示未变动的积分明细' : 'Include unchanged balances' })
      await all.check(); await expect(table.locator('tbody tr')).toHaveCount(16)
      await all.uncheck(); await expect(table.locator('tbody tr')).toHaveCount(2)
      for (const width of info.project.name.includes('reflow') ? [320, 640, 900, 901, 1040, 1440] : [preset.width]) {
        if (info.project.name.includes('reflow')) await page.setViewportSize({ width, height: preset.height })
        await expectNoPageOverflow(page)
        const time = table.locator('.ledger-time').first()
        expect(await time.evaluate(node => getComputedStyle(node).whiteSpace)).toBe('nowrap')
        expect(await time.evaluate(node => node.getClientRects().length)).toBe(1)
        expect(await time.evaluate(node => node.getBoundingClientRect().right <= node.closest('td')!.getBoundingClientRect().right)).toBe(true)
        expect(await detail.evaluate(node => node.scrollWidth <= node.clientWidth + 1)).toBe(true)
        await page.screenshot({ path: info.outputPath(`ledger-table-${locale}-${width}.png`) })
        const pagination=detail.locator('.table-pagination')
        await pagination.scrollIntoViewIfNeeded()
        await expect(pagination).toBeInViewport()
        await expect(pagination.locator('.table-pagination-summary .table-pagination-desktop')).toHaveText(zh?'第 1 页 · 本页 1 条 · 共 1 条':'Page 1 · 1 record on this page · Total: 1')
        await expect(pagination.locator('.table-pagination-actions')).toHaveCSS('gap','12px')
        await pagination.screenshot({path:info.outputPath(`ledger-pagination-${locale}-${width}.png`)})
      }
    }
    await page.screenshot({ path: info.outputPath(`finance-dialog-${locale}-${name}.png`) })
    if (name !== (zh ? '余额修复' : 'Balance repair')) await expect(area.locator(name === (zh ? '积分' : 'Points') ? '.wallet-panel' : '.recon-table')).toHaveCount(1)
    await page.keyboard.press('Escape')
    await expect(detail).toHaveCount(0)
    await expect(input).toHaveValue('member001')
    expect(queries.at(-1)).toBe('member001')
    expect(memberOffsets.at(-1)).toBe('100')
    await expect(list.locator('.member-menu-trigger')).toBeFocused()
    await expect(list.getByRole('button', { name: 'member001', exact: true })).toBeVisible()
    await expect(page.locator('.breadcrumbs b')).toHaveText(zh?'会员管理':'Member management')
  }
  const more = page.locator('.mobile-nav > button').last(), mobile = await more.isVisible()
  if (mobile) { await more.click(); await expect(more).toHaveAttribute('aria-expanded', 'true') }
  await page.locator(mobile ? '.mobile-more-menu' : '.side-nav').getByRole('button', { name: zh ? '充值记录' : 'Recharge records', exact: true }).click()
  const records = page.locator('.console-recharges-table')
  await expect(records.locator('tbody tr')).toHaveCount(1)
  await expect(page.locator('.finance-management > .console-panel > .table-pagination')).toBeVisible()
  expect(quotaQueries).toEqual([])
  await expect(page.getByTestId('recharge-quota')).toHaveCount(0)
  const area = page.locator('.finance-management'), filter = area.locator('.recharge-filters')
  await expect(area.getByRole('heading', {name:zh?'充值单':'Recharge orders',exact:true})).toHaveCount(0)
  await filter.locator('input').fill('member001')
  await filter.getByRole('option',{name:'member001',exact:true}).click()
  await filter.getByRole('button', {name:zh?'查询':'Search',exact:true}).click()
  await expect.poll(()=>rechargeQueries.at(-1)).toBe(memberId)
  await filter.getByRole('button',{name:zh?'清空':'Clear',exact:true}).click()
  await expect.poll(()=>rechargeQueries.at(-1)).toBe('')
  for (const width of info.project.name.includes('reflow') ? [320,640,900,901,1440] : [preset.width]) {
    if (info.project.name.includes('reflow')) await page.setViewportSize({width,height:preset.height})
    await expectNoPageOverflow(page)
    await page.screenshot({path:info.outputPath(`recharge-records-${locale}-${width}.png`)})

  }
  await records.getByRole('button', { name: zh ? '详情' : 'Details', exact: true }).click()
  const rechargeDetail = page.getByRole('dialog', { name: zh ? '详情' : 'Details', exact: true })
  await expect(rechargeDetail).toBeVisible()
  await expect(rechargeDetail).toContainText('10')
  await page.keyboard.press('Escape'); await expect(rechargeDetail).toHaveCount(0)
  await expect(records.locator('tbody tr')).toHaveCount(1)
  await expect(page.locator('.finance-management > .console-panel > .table-pagination')).toBeVisible()
  walletAllowed = false
  await page.reload()
  await expect(page.locator('.management-console')).toBeVisible()
  if (await moreMenu.isVisible()) await moreMenu.click()
  await page.locator(await moreMenu.isVisible() ? '.mobile-more-menu' : '.side-nav').getByRole('button', {name:zh?'会员管理':'Member management',exact:true}).click()
  await list.locator('.member-menu-trigger').click()
  for (const name of [zh?'积分':'Points',zh?'账本':'Ledger']) await expect(page.getByRole('menuitem',{name,exact:true})).toHaveCount(0)
  expect(errors).toEqual([])
})

for (const locale of ['zh-CN', 'en'] as const) test(`reports load the default range without a query click (${locale})`, async ({ page }, info) => {
  await installLayoutApi(page, locale)
  const zh = locale === 'zh-CN', reads: string[] = [], errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  await page.route('**/api/v1/admin/**', async route => {
    const req = route.request(), url = new URL(req.url()), path = url.pathname
    expect(req.method()).toBe('GET')
    const reply = (data: unknown) => route.fulfill({ json: { success: true, request_id: 'report-layout-read', data, associations: { [`brand:${gameId}`]: { name: 'Aurora' } } } })
    if (path.endsWith('/me')) return reply({ account: { id: gameId, username: 'report_reviewer', super_admin: false, brand_ids: [gameId], permissions: [], permissions_by_brand: { [gameId]: ['report_betting.view.brand', 'report_ledger.view.brand', 'report_withdrawal.view.brand', 'report_reward.view.brand', 'report_attribution.view.brand', 'commission.view.brand', 'report_commission.view.brand'] } } })
    if (path.endsWith('/brands')) return reply({ items: [{ id: gameId, code: 'layout', name: 'Aurora', status: 'active', member_count: '0', agent_count: '0', recharge_quota_available: '0', created_at: '2026-10-24T00:00:00Z' }] , total_count: '1'})
    expect(req.headers()['x-brand-id']).toBe(gameId)
    if (path.endsWith('/filter-options')) return reply({ brand_id: gameId, items: url.searchParams.get('search') ? [{ id: gameId, name: 'agent_trial' }] : [], limit: 100, offset: 0 , total_count: String((url.searchParams.get('search') ? [{ id: gameId, name: 'agent_trial' }] : []).length)})
    if (path.endsWith('/reports/betting') || path.endsWith('/reports/ledger')) {
      reads.push(path)
      const ledger = path.endsWith('/ledger')
      const fields = ledger ? ['entry_count', 'net_points', 'recharge_points', 'prize_credit_points', 'prize_reversal_points', 'refund_points'] : ['order_count', 'stake_points', 'placed_count', 'won_count', 'lost_count', 'abnormal_count', 'cancelled_count', 'refund_points', 'settled_stake_points', 'unfinalized_stake_points', 'abnormal_stake_points', 'current_prize_points', 'correction_open_count']
      return reply({ brand_id: gameId, snapshot_at: '2026-10-26T00:00:00Z', timezone: 'Asia/Singapore', query: { from: url.searchParams.get('from'), to: url.searchParams.get('to'), group_by: url.searchParams.get('group_by'), limit: Number(url.searchParams.get('limit')), offset: Number(url.searchParams.get('offset')), game_id: null, member_id: null }, summary: Object.fromEntries(fields.map(key => [key, '0'])), items: [], total_groups: '0', ...(ledger ? { balances: { account_count: '0', available_points: '0', frozen_points: '0', withdrawal_points: '0', total_points: '0' } } : {}) })
    }
    const extraReports: Record<string, string[]> = {
      withdrawal: ['order_count', 'requested_points', 'reviewing_count', 'reviewing_points', 'processing_count', 'processing_points', 'paid_count', 'paid_points', 'rejected_count', 'rejected_points', 'failed_count', 'failed_points', 'cancelled_count', 'cancelled_points'],
      rewards: ['entry_count', 'grant_entry_count', 'grant_points', 'reversal_entry_count', 'reversal_points', 'net_points'],
      attribution: ['order_count', 'stake_points', 'placed_count', 'won_count', 'lost_count', 'abnormal_count', 'cancelled_count', 'refund_points', 'settled_stake_points', 'unfinalized_stake_points', 'abnormal_stake_points', 'current_prize_points', 'correction_open_count', 'final_lost_stake_points'],
      commission: ['entry_count', 'paid_entry_count', 'paid_points', 'adjustment_entry_count', 'adjustment_credit_points', 'adjustment_debit_points', 'correction_entry_count', 'correction_credit_points', 'correction_debit_points', 'net_points'],
      'commission-analysis': ['observed_calculated_points', 'calculated_points', 'paid_entry_count', 'paid_points', 'adjustment_entry_count', 'adjustment_credit_points', 'adjustment_debit_points', 'correction_entry_count', 'correction_credit_points', 'correction_debit_points', 'posting_entry_count', 'actual_net_points', 'manual_adjustment_net_points', 'effective_target_points', 'calculation_minus_actual_points', 'effective_minus_actual_points'],
    }
    const kind = path.split('/reports/')[1], reportFields = extraReports[kind!]
    if (reportFields) {
      const query: Record<string, unknown> = { from: url.searchParams.get('from'), to: url.searchParams.get('to'), group_by: url.searchParams.get('group_by'), limit: Number(url.searchParams.get('limit')), offset: Number(url.searchParams.get('offset')), member_id: null }
      if (kind === 'withdrawal') query.game_id = null
      if (kind === 'rewards') query.order_id = null
      if (['commission', 'commission-analysis', 'attribution'].includes(kind!)) query.agent_id = url.searchParams.get('agent_id')
      if (['commission', 'commission-analysis'].includes(kind!)) query.cycle_id = null
      if (kind === 'attribution') Object.assign(query, { game_id: null, agent_scope: url.searchParams.get('agent_scope'), join_method: null })
      const summary = Object.fromEntries(reportFields.map(field => [field, '0']))
      if (kind === 'commission-analysis') Object.assign(summary, { calculation_complete: true, effective_target_complete: true })
      return reply({ brand_id: gameId, snapshot_at: '2026-10-26T00:00:00Z', timezone: 'Asia/Singapore', query, summary, items: [], total_groups: '0', ...(kind === 'commission-analysis' ? { coverage: { selected_cycle_count: '0', ready_cycle_count: '0', unready_cycle_count: '0' } } : {}) })
    }
    throw Error(`Unexpected default report request: ${path}`)
  })
  await page.goto(origins.brand)
  await expect(page.locator('.management-console')).toBeVisible()
  const more = page.locator('.mobile-nav > button').last(), mobile = await more.isVisible()
  if (mobile) { await more.click(); await expect(more).toHaveAttribute('aria-expanded', 'true') }
  await page.locator(mobile ? '.mobile-more-menu' : '.side-nav').getByText(zh ? '报表管理' : 'Reports', { exact: true }).click()
  await page.locator(mobile ? '.mobile-more-menu' : '.side-nav').getByRole('button', { name: zh ? '业务报表' : 'Business reports', exact: true }).click()
  await expect.poll(() => reads.length).toBe(2)
  await expect(page.locator('.reports-summary').first()).toBeVisible()
  const preset = page.viewportSize()!
  for (const width of info.project.name.includes('reflow') ? [320, 640, 900, 901, 1040, 1440] : [preset.width]) {
    if (info.project.name.includes('reflow')) await page.setViewportSize({ width, height: preset.height })
    await expectNoPageOverflow(page)
    const fields = page.locator('.reports-filter-grid'), last = fields.locator('label').last(), query = fields.locator('button').last()
    const a = (await last.boundingBox())!, b = (await query.boundingBox())!
    if (Math.abs(a.y + a.height - b.y - b.height) < 2) expect(b.x - a.x - a.width).toBeLessThanOrEqual(12)
    const spacing = await page.locator('.reports-management').evaluate(root => {
      const tabs = root.querySelector('.reports-tabs')!.getBoundingClientRect()
      const panel = root.querySelector('[role="tabpanel"]')!.getBoundingClientRect()
      const content = root.closest('.page-content')!
      const expected = parseFloat(getComputedStyle(root).getPropertyValue('--console-block-gap'))
      const summary = root.querySelector('.report-summary-tags')!
      const box = summary.getBoundingClientRect()
      const tag = summary.firstElementChild!.getBoundingClientRect()
      return { expected, gap: panel.top - tabs.bottom, top: parseFloat(getComputedStyle(content).paddingTop), inset: tag.top - box.top }
    })
    expect(spacing.gap).toBeCloseTo(spacing.expected, 0)
    expect(spacing.top).toBe(spacing.expected)
    expect(spacing.inset).toBeGreaterThanOrEqual(2)
    await page.screenshot({ path: info.outputPath(`default-reports-${locale}-${width}.png`) })
    const reportTabs = page.locator('.reports-tabs').getByRole('tab')
    for (let index = 0; index < 5; index++) {
      await reportTabs.nth(index).click()
      const panel = page.getByRole('tabpanel')
      await expect(panel.locator('.report-results:visible .empty-state')).toHaveCount(1)
      await expect(panel.locator('h3')).toHaveCount(0)
      await expect(panel.locator('.reports-context, .context, .ar-context')).toHaveCount(0)
      const filters = panel.locator('.report-filters:visible, .reports-filter-grid:visible'), results = panel.locator('.report-results:visible')
      await expect(filters).toHaveCount(1)
      await expect(results).toHaveCount(1)
      await expect(filters.getByRole('button', { name: zh ? '查询' : 'Query', exact: true })).toBeVisible()
      await expect(panel.getByRole('button', { name: zh ? '导出完整 CSV' : 'Export full CSV', exact: true })).toHaveCount(0)
      const boxes = await Promise.all([filters.boundingBox(), results.boundingBox()])
      expect(boxes[0]!.y + boxes[0]!.height).toBeLessThan(boxes[1]!.y)
      const labels = await results.locator('.report-metrics dt, .commission-metrics dt, .coverage dt').evaluateAll(nodes => nodes.map(label => {
        const value = label.nextElementSibling!, a = label.getBoundingClientRect(), b = value.getBoundingClientRect()
        return { text: label.textContent, nowrap: getComputedStyle(label).whiteSpace, fits: label.scrollWidth <= label.clientWidth + 1, separated: a.right <= b.left + 1 || a.bottom <= b.top + 1 }
      }))
      expect(labels.length).toBeGreaterThan(0)
      for (const label of labels) {
        expect(label.nowrap, label.text!).toBe('normal')
        expect(label.fits, label.text!).toBe(true)
        expect(label.separated, label.text!).toBe(true)
      }
      if(width === (info.project.name.includes('reflow') ? 320 : preset.width)) {
        const summaryChoice=filters.getByRole('checkbox',{name:zh?'显示汇总':'Show summary',exact:true})
        const detailsChoice=filters.getByRole('checkbox',{name:zh?'显示明细':'Show details',exact:true})
        const readCount=reads.length
        await expect(summaryChoice).toBeChecked();await expect(detailsChoice).toBeChecked()
        await summaryChoice.uncheck();await expect(results.locator('.report-summary-tags')).toHaveCount(0)
        await expect(results.locator('.empty-state')).toBeVisible()
        await detailsChoice.uncheck();await expect(results).toBeHidden()
        await summaryChoice.check();await expect(results.locator('.report-summary-tags').first()).toBeVisible()
        await expect(results.locator('.empty-state')).toHaveCount(0)
        await expect(panel.locator('.table-pagination')).toHaveCount(0)
        await detailsChoice.check();await expect(results.locator('.empty-state')).toBeVisible()
        expect(reads.length).toBe(readCount)
      }
      await expectNoPageOverflow(page)
      await page.locator('.reports-tabs button').evaluateAll(async buttons => { await Promise.all(buttons.flatMap(button => button.getAnimations().map(animation => animation.finished))) })
      await page.screenshot({ path: info.outputPath(`unified-report-${locale}-${index}-${width}.png`) })
    }
    await reportTabs.first().click()
  }
  await page.locator('.reports-tabs').getByRole('tab', { name: zh ? '账本报表' : 'Ledger report', exact: true }).click()
  await expect(page.locator('#ledger-results').locator('.reports-summary').first()).toBeVisible()
  await page.locator('.reports-tabs').getByRole('tab').first().click()
  const bettingFilters=page.locator('.reports-filter-grid')
  const agentSearch=bettingFilters.getByRole('searchbox',{name:zh?'代理账号':'Agent account',exact:true})
  await agentSearch.fill('agent_trial')
  await page.getByRole('option',{name:'agent_trial',exact:true}).click()
  const agentQuery=page.waitForRequest(req=>req.url().includes('/reports/attribution?'))
  await bettingFilters.getByRole('button',{name:zh?'查询':'Query',exact:true}).click()
  expect(new URL((await agentQuery).url()).searchParams.get('agent_id')).toBe(gameId)
  await expect(page.locator('#betting-results .empty-state')).toBeVisible()
  const reportReads = reads.length
  const tabs = page.locator('.reports-tabs').getByRole('tab')
  await expect(tabs).toHaveCount(5)
  await expect.poll(async () => (await tabs.nth(1).evaluate(el => getComputedStyle(el).backgroundColor)) !== (await tabs.first().evaluate(el => getComputedStyle(el).backgroundColor))).toBe(true)
  await expect(page.getByRole('button', { name: zh ? '佣金报表' : 'Commission reports', exact: true })).toHaveCount(0)
  await tabs.first().focus()
  await page.keyboard.press('End')
  await expect(tabs.last()).toBeFocused()
  await page.keyboard.press('Enter')
  await expect(tabs.last()).toHaveAttribute('aria-selected', 'true')
  await expect(page.locator('.commission-report')).toBeVisible()
  await expect(page.locator('.reports-filter-grid')).toBeHidden()
  for (const index of [2, 3, 4]) {
    await tabs.nth(index).click()
    await expect(tabs.nth(index)).toHaveAttribute('aria-selected', 'true')
    await expect(page.getByRole('tabpanel')).toHaveCount(1)
    await expect(page.locator('.reports-filter-grid')).toBeHidden()
    await expectNoPageOverflow(page)
    await page.screenshot({ path: info.outputPath(`report-tab-${locale}-${index}.png`) })
  }
  await tabs.first().click()
  await expect(page.locator('.reports-filter-grid')).toBeVisible()
  expect(reads).toHaveLength(reportReads)
  expect(errors).toEqual([])
})

for (const locale of ['zh-CN','en'] as const) test(`platform quota management opens from its own menu (${locale})`, async ({page}, info) => {
  await installLayoutApi(page,locale)
  const zh=locale==='zh-CN', at='2026-10-24T00:00:00Z', historyReads:string[]=[], errors:string[]=[]
  page.on('pageerror',error=>errors.push(error.message))
  await page.route('**/api/v1/platform/**', async route=>{
    const req=route.request(), url=new URL(req.url()), path=url.pathname
    expect(req.method()).toBe('GET')
    const reply=(data:unknown)=>route.fulfill({json:{success:true,data}})
    if(path.endsWith('/presentation')) return reply({display_name:'Layout trial',logo_text:'L',logo_url:''})
    if(path.endsWith('/me')) return reply({account:{id:gameId,username:'quota_reviewer',super_admin:true,platform_permissions:['recharge.view.platform','recharge_quota.write.platform']}})
    if(path.endsWith('/platform-settings')) return reply({display_name:'Layout trial',logo_text:'L',logo_url:'',timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss',version:1,updated_at:at})
    if(path.endsWith('/brands')) return reply({items:[{id:gameId,name:'Aurora',code:'aurora',status:'active',member_count:'0',agent_count:'0',recharge_quota_available:'880',created_at:at}], total_count: '1'})
    expect(req.headers()['x-brand-id']).toBe(gameId)
    if(path.endsWith('/recharges')) return reply({items:[], total_count: '0'})
    if(path.endsWith('/recharge-quota')) return reply({brand_id:gameId,available:'880'})
    if(path.endsWith('/recharge-quota/entries')) {historyReads.push(path); return reply({items:[], total_count: '0'})}
    throw Error(`Unexpected recharge request: ${path}`)
  })
  await page.goto(origins.platform)
  await expect(page.locator('.management-console')).toBeVisible()
  const toggle=page.locator('button[aria-controls="platform-navigation"]')
  if(await toggle.isVisible() && await toggle.getAttribute('aria-expanded')==='false') await toggle.click()
  const funds=page.locator('.sidebar').getByRole('button',{name:zh?'资金':'Finance',exact:true})
  if(await funds.getAttribute('aria-expanded')==='false') await funds.click()
  await page.getByTestId('platform-wallet-tabs').getByRole('button',{name:zh?'充值记录':'Recharge records',exact:true}).click()
  await selectControl(page.locator(".topbar-brand-picker .select-control-trigger"), gameId)
  const records=page.getByTestId('platform-recharges')
  await expect(records).toBeVisible()
  expect(historyReads).toHaveLength(0)
  await expect(records.getByTestId('recharge-quota')).toHaveCount(0)
  if(await toggle.isVisible() && await toggle.getAttribute('aria-expanded')==='false') await toggle.click()
  await page.getByTestId('platform-wallet-tabs').getByRole('button',{name:zh?'额度管理':'Quota management',exact:true}).click()
  const quota=page.getByTestId('recharge-quota')
  await expect(quota).toContainText('880')
  await expect(quota.locator('.quota-form')).toHaveCount(0)
  await quota.getByRole('button',{name:locale==='en'?'Operation':'额度操作',exact:true}).click()
  await expect(quota.locator('.quota-form')).toHaveCount(1)
  await expect(page.getByRole('dialog')).toHaveCount(1)
  const preset=page.viewportSize()!
  for(const width of info.project.name.includes('reflow')?[320,640,900,901,1440]:[preset.width]) {
    if(info.project.name.includes('reflow')) await page.setViewportSize({width,height:preset.height})
    await expectNoPageOverflow(page)
    await page.screenshot({path:info.outputPath(`platform-recharges-${locale}-${width}.png`)})
    await expect(quota).toContainText(zh?'暂无额度记录。':'No quota records.')
    await page.screenshot({path:info.outputPath(`platform-quota-page-${locale}-${width}.png`)})
  }
  expect(errors).toEqual([])
})

for (const locale of ['zh-CN', 'en'] as const) test(`withdrawal query defaults to reviewing and uses standard heading (${locale})`, async ({ page }, info) => {
  await installLayoutApi(page, locale)
  const zh = locale === 'zh-CN', queries: string[] = [], errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  await page.route('**/api/v1/admin/**', async route => {
    const req = route.request(), url = new URL(req.url()), path = url.pathname
    expect(req.method()).toBe('GET')
    const reply = (data: unknown) => route.fulfill({ json: { success: true, data } })
    if (path.endsWith('/me')) return reply({ account: { id: gameId, username: 'withdrawal_reviewer', super_admin: false, brand_ids: [gameId], permissions: [], permissions_by_brand: { [gameId]: ['withdrawal.view.brand'] } } })
    if (path.endsWith('/brands')) return reply({ items: [{ id: gameId, code: 'layout', name: 'Aurora', status: 'active', member_count: '0', agent_count: '0', recharge_quota_available: '0', created_at: '2026-10-24T00:00:00Z' }] , total_count: '1'})
    expect(req.headers()['x-brand-id']).toBe(gameId)
    if (path.endsWith('/withdrawals')) {
      queries.push(url.searchParams.get('state') ?? '')
      return reply({ brand_id: gameId, items: [], limit: Number(url.searchParams.get('limit')), offset: Number(url.searchParams.get('offset')), has_more: false , total_count: '0'})
    }
    throw Error(`Unexpected withdrawal request: ${path}`)
  })
  await page.goto(origins.brand)
  await expect(page.locator('.management-console')).toBeVisible()
  const more = page.locator('.mobile-nav > button').last(), mobile = await more.isVisible()
  if (mobile) await more.click()
  await page.locator(mobile ? '.mobile-more-menu' : '.side-nav').getByRole('button', { name: zh ? '提现申请' : 'Withdrawal requests', exact: true }).click()
  const area = page.locator('.withdrawal-management'), form = area.locator('form.withdrawal-filters')
  await expect.poll(() => queries.length).toBeGreaterThan(0)
  expect(queries.every(state => state === 'reviewing')).toBe(true)
  await expect(form.locator(".select-control-trigger")).toHaveAttribute('data-value', 'reviewing')
  const input = form.getByRole('searchbox', { name: zh ? '会员账号' : 'Member account', exact: true })
  await expect(input).toBeVisible()
  await expect(form.getByRole('button', { name: zh ? '查询' : 'Search', exact: true })).toBeVisible()
  const heading = area.locator('.page-heading')
  await expect(heading).toHaveCSS('clip-path', 'inset(50%)')
  await expect(page.locator('.breadcrumbs b')).toHaveText(zh ? '提现申请' : 'Withdrawal requests')
  expect(await heading.evaluate(el => ({ background: getComputedStyle(el).backgroundColor, padding: getComputedStyle(el).padding, shadow: getComputedStyle(el).boxShadow }))).toEqual({ background: 'rgba(0, 0, 0, 0)', padding: '0px', shadow: 'none' })
  const preset = page.viewportSize()!
  for (const width of info.project.name.includes('reflow') ? [320, 640, 900, 901, 1040, 1440] : [preset.width]) {
    if (info.project.name.includes('reflow')) await page.setViewportSize({ width, height: preset.height })
    await expectNoPageOverflow(page)
    await expect(heading).toHaveCSS('clip-path', 'inset(50%)')
    await page.screenshot({ path: info.outputPath(`withdrawal-query-${locale}-${width}.png`) })
  }
  await selectControl(form.locator(".select-control-trigger"), '')
  await form.getByRole('button', { name: zh ? '查询' : 'Search', exact: true }).click()
  await expect.poll(() => queries.at(-1)).toBe('')
  await expect(area.locator('.empty-state')).toBeVisible()
  expect(errors).toEqual([])
})

for(const locale of ['en','zh-CN'] as const)test(`platform reports display summary tags and details independently (${locale})`,async({page},info)=>{
 const fields=['order_count','stake_points','placed_count','won_count','lost_count','abnormal_count','cancelled_count','refund_points','settled_stake_points','unfinalized_stake_points','abnormal_stake_points','current_prize_points','correction_open_count']
 const errors:string[]=[];let reads=0
 page.on('pageerror',error=>errors.push(error.message))
 const source=await(await page.request.get(origins.brand+'/src/ReportsManagement.vue')).text(),vuePath=source.match(/from "([^"]*deps\/vue\.js[^"]*)"/)![1]
 await page.route('**/api/**',route=>{
  expect(route.request().method()).toBe('GET')
  const url=new URL(route.request().url())
  if(url.pathname.endsWith('/filter-options'))return route.fulfill({json:{success:true,data:{brand_id:'11111111-1111-4111-8111-111111111111',items:[],limit:100,offset:0}}})
  expect(url.pathname).toBe('/api/v1/platform/reports/betting');reads++
  const summary=Object.fromEntries(fields.map(field=>[field,field==='stake_points'?'120':'1']))
  return route.fulfill({json:{success:true,data:{brand_id:'11111111-1111-4111-8111-111111111111',snapshot_at:'2026-11-02T00:00:00Z',timezone:'Asia/Singapore',query:{from:url.searchParams.get('from'),to:url.searchParams.get('to'),group_by:'day',limit:Number(url.searchParams.get('limit')),offset:0,game_id:null,member_id:null},summary,items:[{key:'2026-11-02',label:'2026-11-02',totals:summary}],total_groups:'1'}}})
 })
 const url=origins.brand+'/__platform-report-display'
 await page.route(url,route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module">import{createApp}from'${vuePath}';import Harness from'/@fs/${resolve('tests/layout/NavFiltersHarness.vue')}';import'/@fs/${resolve('shared/src/styles.css')}';import'/@fs/${resolve('platform-web/src/style.css')}';import'/@fs/${resolve('shared/src/management.css')}';import'/@fs/${resolve('shared/src/controls.css')}';createApp(Harness,${JSON.stringify({locale,view:'platform'})}).mount('#app')</script></html>`}))
 await page.goto(url)
 const summary=page.getByTestId('platform-reports-summary'),details=page.getByTestId('platform-reports-list'),pagination=page.getByTestId('platform-reports-pages')
 await expect(summary.locator('.report-summary-tags > div')).toHaveCount(fields.length)
 await expect(details).toBeVisible();await expect(pagination).toBeVisible()
 await expect(page.getByRole('button',{name:/导出|Export/})).toHaveCount(0)
 await expect(page.getByRole('heading',{name:locale==='en'?'Grouped data':'分组数据',exact:true})).toHaveCount(0)
 await expect(summary.getByRole('heading')).toHaveCount(0)
 const showSummary=page.getByRole('checkbox',{name:locale==='en'?'Show summary':'显示汇总',exact:true}),showDetails=page.getByRole('checkbox',{name:locale==='en'?'Show details':'显示明细',exact:true})
 const readCount=reads
 await showSummary.uncheck();await expect(summary).toHaveCount(0);await expect(details).toBeVisible()
 await showDetails.uncheck();await expect(details).toHaveCount(0);await expect(pagination).toHaveCount(0)
 await showSummary.check();await expect(summary).toBeVisible();await expect(details).toHaveCount(0)
 await showDetails.check();await expect(details).toContainText('120');await expect(pagination).toBeVisible()
 expect(reads).toBe(readCount)
 const preset=page.viewportSize()!
 for(const width of info.project.name.includes('reflow')?[320,640,900,901,1440]:[preset.width]){
  if(info.project.name.includes('reflow'))await page.setViewportSize({width,height:preset.height})
  await expectNoPageOverflow(page)
  await page.screenshot({path:info.outputPath(`report-display-${locale}-${width}.png`),fullPage:true})
 }
 expect(errors).toEqual([])
})
