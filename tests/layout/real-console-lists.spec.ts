import { test, expect } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { gameId, origins, theme, installLayoutApi, openPlatformBrandList } from './fixture'
import { selectControl } from '../select-control'
import { expectRealEntry, expectListLayout, reviewRealPage } from './real-page-review'
import { overviewFixture } from './platform-overview'
import { REPORT_FIELDS } from '../../platform-web/src/reports-api'

const at = '2026-11-02T00:00:00Z'
// Use the real permission registry, rather than invented superuser bypasses.
const permissions = [...new Set([...(readFileSync('backend/migrations/0001_baseline.up.sql', 'utf8') + readFileSync('backend/cmd/platform/admin.go', 'utf8')).matchAll(/['"]([a-z_]+\.[a-z_]+\.(?:brand|platform))['"]/g)].map(match => match[1]))]
const brand = { id: gameId, code: 'review', name: 'Review brand', status: 'active', member_count: '0', agent_count: '0', recharge_quota_available: '0', created_at: at }

for (const surface of ['brand', 'platform'] as const) for (const locale of ['en', 'zh-CN'] as const) {
  test(`${surface} real navigation renders complete list pages (${locale})`, async ({ page }, info) => {
    test.setTimeout(180_000)
    page.setDefaultTimeout(8_000)
    const zh = locale === 'zh-CN', errors: string[] = [], reads: string[] = []
    page.on('pageerror', error => errors.push(error.message))
    await installLayoutApi(page, locale)
    const scope = surface === 'brand' ? 'admin' : 'platform'
    await page.route(`**/api/v1/${scope}/**`, async route => {
      const req = route.request(), url = new URL(req.url()), path = url.pathname, endpoint = path.slice(`/api/v1/${scope}`.length)
      expect(req.method(), path).toBe('GET'); reads.push(endpoint)
      const reply = (data: unknown) => route.fulfill({ json: { success: true, request_id: 'real-page-review', data } })
      if (endpoint === '/me') return reply({ account: { id: gameId, username: `${surface}_real_reviewer`, super_admin: surface === 'platform', brand_ids: [gameId], permissions: [], permissions_by_brand: { [gameId]: permissions.filter(p => p.endsWith('.brand')) }, platform_permissions: permissions.filter(p => p.endsWith('.platform')) } })
      if (endpoint === '/overview') { const { brands, ...summary } = overviewFixture(); return reply(surface === 'platform' ? overviewFixture() : { ...summary, brand_id: gameId }) }
      if (endpoint === '/brands') return reply({ items: [brand], total_count: '1' })
      if (endpoint === '/presentation') return reply({ display_name: 'Review platform', logo_text: 'R', logo_url: '' })
      if (endpoint === '/platform-settings') return reply({ version: 1, display_name: 'Review platform', logo_text: 'R', logo_url: '', timezone: 'Asia/Singapore', date_format: 'YYYY-MM-DD', time_format: 'HH:mm:ss', updated_at: at })
      if (endpoint === '/brand-presentation') return reply({brand_id:gameId,version:1,status:'active',base_name:brand.name,config:{},effective:{...theme,display_name:brand.name}})
      if (endpoint === '/agent-policy') return reply({brand_id:gameId,version:1,config:{enabled:true,max_depth:5,loss_ratio_cap:'0.25',turnover_ratio_cap:'0.03',mode:'loss',cycle:'monthly'},updated_at:at})
      if (endpoint === '/platform-permissions') return reply({items:permissions.filter(p=>p.endsWith('.platform'))})
      if (endpoint === '/permissions') return reply({ items: permissions.filter(p => p.endsWith('.brand')) })
      if (endpoint === '/recharges' && surface === 'platform') return reply({items:[],total_count:'0'})
      if (endpoint === '/recharge-quota') return reply({ brand_id: gameId, available: '0' })
      if (endpoint === '/filter-options') return reply({ brand_id: gameId, items: [], limit: 100, offset: 0 })
      if (endpoint === '/platform-games') return reply({ games: [], total_count: '0', limit: 50, offset: 0 })
      if (endpoint === '/notification-templates') return reply({ items: [] })
      if (/^\/reports\/(betting|ledger|withdrawal|commission|rewards|reward-orders|attribution)$/.test(endpoint)) {
        const kind = endpoint.split('/').at(-1)!.replace('reward-orders', 'reward_orders')
        const fields = kind === 'attribution' ? [...REPORT_FIELDS.betting, 'final_lost_stake_points'] : REPORT_FIELDS[kind as keyof typeof REPORT_FIELDS]
        const q = url.searchParams
        const query = {from:q.get('from'),to:q.get('to'),group_by:q.get('group_by'),limit:Number(q.get('limit')),offset:Number(q.get('offset')),
          ...(kind==='attribution'?{game_id:q.get('game_id'),member_id:q.get('member_id'),agent_id:q.get('agent_id'),agent_scope:q.get('agent_scope')??'direct',join_method:q.get('join_method')}:kind==='commission'?{agent_id:q.get('agent_id'),member_id:q.get('member_id'),cycle_id:q.get('cycle_id')}:['rewards','reward_orders'].includes(kind)?{member_id:q.get('member_id'),order_id:q.get('order_id')}:{game_id:q.get('game_id'),member_id:q.get('member_id')})}
        return reply({ brand_id: gameId, snapshot_at: at, timezone: 'Asia/Singapore', query, summary: Object.fromEntries(fields.map(key => [key, '0'])), items: [], total_groups: '0', ...(kind==='ledger'?{balances: { account_count: '0', available_points: '0', frozen_points: '0', withdrawal_points: '0', total_points: '0' }}:{}) })
      }
      // Each empty collection is explicitly registered. Unknown endpoints fail.
      if (['/users', '/agents', '/accounts', '/roles', '/platform-accounts', '/platform-roles', '/recharges', '/withdrawals', '/reward-orders', '/audit', '/commission-cycles', '/commission-payments', '/report-archives', '/notification-deliveries', '/configured-orders'].includes(endpoint)) return reply({ brand_id: gameId, items: [], total_count: '0', limit: Number(url.searchParams.get('limit') ?? 50), offset: 0, ...(endpoint === '/withdrawals' ? { has_more: false } : {}) })
      throw Error(`Unconfigured real ${surface} request: ${endpoint}`)
    })
    await page.goto(origins[surface])
    await expect(page.locator('.management-console')).toBeVisible()
    await expectRealEntry(page, surface)

    async function brandNav(label: string) {
      const mobile = await page.locator('.mobile-nav').isVisible()
      if (mobile) await page.locator('.mobile-nav button').last().click()
      await page.locator(mobile ? '.mobile-more-menu' : '.side-nav').getByRole('button', { name: label, exact: true }).click()
    }
    async function platformNav(label: string) {
      const toggle = page.locator('button[aria-controls="platform-navigation"]')
      if (await toggle.isVisible() && await toggle.getAttribute('aria-expanded') === 'false') await toggle.click()
      await page.locator('#platform-navigation').getByRole('button', { name: label, exact: true }).click()
    }
    async function review(label: string, endpoint: string, query = false) {
      const list = page.locator('.management-list').filter({ visible: true }).first()
      await expect(list, label).toBeVisible()
      if (query && !reads.includes(endpoint)) await list.locator('button[type=submit]').click()
      await expect.poll(() => {
        expect(errors, `${label}: application runtime errors`).toEqual([])
        return reads.some(path => path.split('?')[0] === endpoint)
      }, { message: `${label}: real page did not read ${endpoint}` }).toBe(true)
      await expect(list.locator('table').filter({visible:true}),label).toHaveCount(1)
      await expect(list.locator('table').filter({visible:true}).locator('thead'), label).toHaveCount(1)
      await expect(list.locator('td.empty-state').filter({visible:true}),label).toHaveCount(1)
      await expect(list.locator('.table-pagination'), label).toHaveCount(0)
      await reviewRealPage(page, info, `${surface}-${label}-${locale}`)
      if(surface==='brand' && ['/users','/accounts','/roles','/reward-orders'].includes(endpoint)){
        const selector=endpoint==='/users'?'button[aria-controls=member-create-form]':endpoint==='/reward-orders'?'[data-testid=rewards-grant-open]':'.button-primary'
        const create=list.locator('.management-list-toolbar > .console-page-actions').locator(selector)
        await expect(create).toHaveCount(1);await expect(create).toBeEnabled();await create.click()
        const dialog=page.getByRole('dialog');await expect(dialog).toBeVisible();await expect(dialog.locator('input').first()).toBeVisible()
        if(endpoint==='/users')await expect(dialog.locator('textarea').locator('..')).toHaveText(zh?'内部备注':'Internal notes')
        await expectListLayout(page)
        await page.screenshot({path:info.outputPath(`${surface}-${endpoint.slice(1)}-create-${locale}.png`),fullPage:true})
        await dialog.locator('.detail-modal-close').click();await expect(dialog).toHaveCount(0)
      }
    }
    if (surface === 'brand') {
      const pages = [
        [zh ? '会员管理' : 'Member management', '/users'],
        [zh ? '代理列表' : 'Agent list', '/agents'],
        [zh ? '彩种管理' : 'Game management', '/platform-games'],
        [zh ? '注单管理' : 'Bet order management', '/configured-orders'],
        [zh ? '管理账号' : 'Admin accounts', '/accounts'],
        [zh ? '角色权限' : 'Roles and permissions', '/roles'],
        [zh ? '充值记录' : 'Recharge records', '/recharges'],
        [zh ? '提现申请' : 'Withdrawal requests', '/withdrawals'],
        [zh ? '人工奖励' : 'Manual rewards', '/reward-orders'],
        [zh ? '佣金管理' : 'Commission management', '/commission-cycles'],
        [zh ? '审计日志' : 'Audit log', '/audit'],
      ]
      expect(await page.locator('.side-nav .nav-item').evaluateAll(items=>items.map(item=>item.getAttribute('aria-label')))).toEqual(expect.arrayContaining(pages.map(([label])=>label)))
      await expect(page.locator('.side-nav .nav-item')).toHaveCount(pages.length+3)
      for (const [label, endpoint] of pages) await test.step(label, async () => { await brandNav(label); await review(label, endpoint) })
      await brandNav(zh?'业务报表':'Business reports')
      for(const [tab,endpoint] of [['betting','betting'],['ledger','ledger'],['withdrawal','withdrawal'],['reward','rewards'],['posting','commission']]){
        await page.locator('#report-tab-'+tab).click()
        if(['betting','ledger'].includes(tab))await page.getByRole('button',{name:zh?'今天':'Today',exact:true}).click()
        await review('report-'+tab,'/reports/'+endpoint,true)
      }
      await page.locator('#report-tab-betting').click();await selectControl(page.getByRole('combobox',{name:zh?'分组':'Group by',exact:true}),'agent');await review('report-attribution','/reports/attribution',true)
    } else {
      await openPlatformBrandList(page, locale)
      const row = page.getByTestId('platform-brand-list').locator('tbody tr').first()
      await expect(row).toContainText('Review brand')
      await reviewRealPage(page, info, `platform-brands-${locale}`)
      await platformNav(zh ? '会员管理' : 'Member management')
      await selectControl(page.locator('.topbar-brand-picker .select-control-trigger'), gameId)
      await review('members', '/users')
      for (const [label, endpoint] of [[zh ? '管理' : 'Admin', '/platform-accounts'], [zh ? '奖励' : 'Rewards', '/reward-orders'], [zh ? '提现' : 'Withdrawals', '/withdrawals']]) {
        await test.step(label, async () => { await platformNav(label); await review(label, endpoint);if(endpoint==='/platform-accounts'){await page.getByTestId('platform-access-tabs').getByRole('tab').nth(1).click();await review('platform-roles','/platform-roles')} })
      }
      await platformNav(zh?'后台账号':'Back-office accounts');await review('brand-accounts','/accounts');await page.getByTestId('platform-access-tabs').getByRole('tab').nth(1).click();await review('brand-roles','/roles')
      await platformNav(zh ? '资金' : 'Finance')
      await platformNav(zh ? '充值记录' : 'Recharge records'); await review('recharges', '/recharges')
      await platformNav(zh?'财务报表':'Financial reports')
      for(const kind of ['betting','ledger','withdrawal','commission','rewards','reward_orders']){
        await selectControl(page.locator('.report-filters').getByRole('combobox').first(),kind)
        await review('report-'+kind,'/reports/'+kind.replace('reward_orders','reward-orders'),true)
        const totals=page.getByTestId('platform-reports-summary');await expect(totals.locator('dt')).toHaveCount(REPORT_FIELDS[kind as keyof typeof REPORT_FIELDS].length);await expect(totals.locator('dt').first()).toHaveCSS('text-transform','none')
      }
      await platformNav(zh ? '佣金' : 'Commission')
      for (const [label, endpoint] of [[zh ? '代理' : 'Agents', '/agents'], [zh ? '周期' : 'Cycles', '/commission-cycles'], [zh ? '派发' : 'Payments', '/commission-payments']]) { await platformNav(label); await review(label, endpoint) }
    }
    if(surface==='platform'){
      await platformNav(zh?'运营':'Operations')
      await platformNav(zh?'审计日志':'Audit log');await review('audit','/audit')
      await platformNav(zh?'归档记录':'Archive records');await review('archives','/report-archives')
      await platformNav(zh?'通知模板':'Notification templates')
      await expect(page.getByTestId('platform-template-list').locator('table')).toBeVisible();await reviewRealPage(page,info,'platform-templates-'+locale)
      await platformNav(zh?'彩种':'Games');await review('games','/platform-games')
      await platformNav(zh?'注单':'Bets');await review('orders','/configured-orders')
    }
    expect(errors).toEqual([])
  })
}
