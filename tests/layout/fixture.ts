import { expect, type Page, type Locator } from '@playwright/test'
import { expectFilterSpacing } from './filter-spacing'
import {readFileSync} from 'node:fs'
import {currencies,type CashConfig,type CashBalance} from '../../shared/src/cash'
import {configuredDemo} from '../../shared/src/configured-rules'
import { redundantAnnotation } from '../ui-copy-contract.mjs'
const configuredSnapshot=configuredDemo(JSON.parse(readFileSync('backend/internal/rules/config/mark_six_v2.json','utf8')))

const portBase = Number(process.env.LOTTERY_LAYOUT_PORT_BASE ?? 15273)
if (!Number.isInteger(portBase) || portBase < 1024 || portBase > 65532) throw new Error('Invalid layout test port base')
export const origins = { agent: `http://127.0.0.1:${portBase + 3}`, user: `http://127.0.0.1:${portBase}`, brand: `http://127.0.0.1:${portBase + 1}`, platform: `http://127.0.0.1:${portBase + 2}` }
export const gameId = 'aaaaaaaa-aaaa-4aaa-8aaa-000000000001'

export async function expectCompactDisplayRows(page: Page) {
  await page.evaluate(() => {
    const probe = document.createElement('section');
    probe.id = 'key-value-layout-probe';
    probe.style.cssText = `box-sizing:border-box;width:${Math.min(320, innerWidth)}px;max-width:100%;padding:12px;font-size:14px`;
    probe.innerHTML = `<dl><div data-pair="short"><dt>Status / 状态</dt><dd>0</dd></div><div data-pair="long"><dt>ID / 编号</dt><dd>${'1234567890'.repeat(10)}</dd></div><div><dt>Note / 说明</dt><dd class="key-value-wide">Long description / 长内容可占用整行</dd></div></dl><dl data-pair="flat"><dt>Points / 积分</dt><dd>120</dd></dl>`;
    document.body.append(probe);
  });
  try {
    const measured = await page.locator('#key-value-layout-probe').evaluate(probe => ({
      width: probe.clientWidth, scrollWidth: probe.scrollWidth,
      rows: [...probe.querySelectorAll('[data-pair]')].map(row => {
        const label = row.querySelector('dt')!, value = row.querySelector('dd')!;
        const a = label.getBoundingClientRect(), b = value.getBoundingClientRect();
        return { labelRight: a.right, labelTop: a.top, labelBottom: a.bottom, valueLeft: b.left, valueTop: b.top, valueBottom: b.bottom, text: value.textContent, fontSize: Number.parseFloat(getComputedStyle(value).fontSize) };
      }),
    }));
    expect(measured.scrollWidth).toBeLessThanOrEqual(measured.width);
    expect(measured.rows).toHaveLength(3);
    for (const row of measured.rows) {
      expect(row.valueLeft).toBeGreaterThanOrEqual(row.labelRight);
      expect(row.valueTop).toBeLessThan(row.labelBottom);
      expect(row.labelTop).toBeLessThan(row.valueBottom);
      expect(row.fontSize).toBeGreaterThanOrEqual(11);
    }
    expect(measured.rows[1].text).toBe('1234567890'.repeat(10));
    const wide = page.locator('#key-value-layout-probe .key-value-wide');
    await expect(wide).toHaveCSS('grid-column-start', '1');
    await expectNoPageOverflow(page);
  } finally { await page.locator('#key-value-layout-probe').evaluate(node => node.remove()); }
}
const numbers = [3, 10, 7, 8, 36, 42, 20]
const waves = ['blue', 'blue', 'red', 'red', 'blue', 'blue', 'blue']
const zodiac = ['dragon', 'rooster', 'rat', 'pig', 'goat', 'ox', 'pig']
const elements = ['fire', 'fire', 'earth', 'wood', 'earth', 'metal', 'earth']
const snapshot = (values: string[]) => ({ ...Object.fromEntries(Array.from({ length: 49 }, (_, i) => [String(i + 1).padStart(2, '0'), values[i % values.length]])), ...Object.fromEntries(numbers.map((n, i) => [String(n).padStart(2, '0'), values[i]])) })
const attributes = { id: gameId, year: 2026, version: 1, effective_at: '2026-01-01T00:00:00Z', expires_at: '2027-01-01T00:00:00Z', waves: {...snapshot(waves),'49':'green'}, zodiac: snapshot(zodiac), elements: snapshot(elements) }
const result = { regular: numbers.slice(0, 6), special: [20], digits: [] }
export const theme = {
  display_name: 'Layout trial', logo_text: 'L', logo_url: null, favicon_url: null,
  default_locale: 'en', available_locales: ['en', 'zh-CN'],
  content: { en: { tagline: 'Layout trial', announcement: '' }, 'zh-CN': { tagline: '布局验证', announcement: '' } },
}

// Only explicit synthetic read fixtures. Unknown requests and any write fail the test.
// Financial/business acceptance remains in tests/browser with its real database.
export async function installLayoutApi(page: Page, locale: 'en' | 'zh-CN') {
  await page.context().addCookies(Object.values(origins).map(url => ({ name: 'lottery-locale', value: locale, url })))
  await page.route('**/api/**', async route => {
    expect(route.request().method(), route.request().url()).toBe('GET')
    const path = new URL(route.request().url()).pathname
    const respond = (data: unknown) => route.fulfill({ json: { success: true, data } })
    if (path === '/api/v1/platform/presentation') return respond({ display_name: 'Layout trial', logo_text: 'L', logo_url: '' })
    if (['/api/v1/admin/auth/config', '/api/v1/agent/auth/config'].includes(path)) return respond({captcha_enabled:false})
    if (path === '/api/v1/display-settings') return respond({ theme: 'jade', timezone: 'Asia/Singapore', date_format: 'YYYY-MM-DD', time_format: 'HH:mm:ss' })
    if (path === '/api/v1/configured/games') return respond({games:[{id:gameId,name:'Mark Six'}], total_count: '1'})
    if (path === '/api/v1/games') return respond({items:[], total_count: '0'})
    if (path === '/api/v1/cash') {
      const config: CashConfig = {version:1,rules:currencies.map(currency=>({currency,inherit:false,enabled:false,rate:'1',deposit_min:'1',deposit_max:'10000',withdrawal_min:'1',withdrawal_max:'10000',fee:'0',mode:'count',threshold:'1'}))}
      return respond({auto_usdt:false,actor_context:'a'.repeat(64),brand_id:gameId,member_id:gameId,balances:cashBalanceFixture(),destinations:[],platform:config,brand:config})
    }
    if (path === '/api/v1/cash/orders') return respond({items:[],total_count:'0'})
    if (path === '/api/v1/announcements') return respond({items:[]})
    if (path === '/api/v1/brand-contacts') return respond({items:[], default_id:null})
    if (path === '/api/v1/me/betting-preferences') return respond({brand_id:gameId,member_id:gameId,quick_amounts:[]})
    if (path === '/api/v1/betting-summary') return respond({brand_id:gameId,member_id:gameId,unsettled_points:'120',today_net_points:'-30'})
    if (path === '/api/v1/context') return respond({
      auth: { captcha_enabled: false, telegram_enabled: false, telegram_client_id: '' },
      available_locales: ['en', 'zh-CN'], brand: { id: gameId, code: 'layout', name: 'Layout trial', status: 'active', default_locale: 'en', timezone: 'Asia/Singapore', theme, config_version: 1 },
      features: { pwa: false, real_payments: false },
    })
    if (['/api/v1/agent/me', '/api/v1/me', '/api/v1/admin/me', '/api/v1/platform/me'].includes(path)) return route.fulfill({ status: 401, json: { success: false, error: { code: 'UNAUTHORIZED', message: 'Sign in required' } } })
    if (path === `/api/v1/platform-games/${gameId}`) {const now=Date.now();return respond({game:{id:gameId,name:'Mark Six'},plays:[],rule_protocol:'configured',periods:[{id:gameId,game_id:gameId,period_no:'2026111',bet_start_at:new Date(now-3600000).toISOString(),bet_end_at:new Date(now+3600000).toISOString(),draw_at:new Date(now+3660000).toISOString(),result:null,published_at:null}], total_count: '1'})}
    if (path === `/api/v1/configured/games/${gameId}`) {const now=Date.now();return respond({game:{id:gameId,name:'Mark Six',status:'active',brand_enabled:true},snapshot:configuredSnapshot,rule_id:gameId,snapshot_hash:'layout-explicit-hash',previous_draw:null,sales:{version:1,enabled:true},period:{id:gameId,period_no:'2026111',bet_start_at:new Date(now-3600000).toISOString(),bet_end_at:new Date(now+3600000).toISOString(),draw_at:new Date(now+3660000).toISOString()}})}
    throw new Error(`Unconfigured layout request: ${route.request().url()}`)
  })
}

export async function expectModalControls(page: Page) {
  const issues = await page.locator('dialog.detail-modal[open]').evaluateAll(dialogs => {
    const issues: string[] = []
    const visible = (el: Element) => { const box=el.getBoundingClientRect(); return box.width>0 && box.height>0 }
    for (const dialog of dialogs.filter(visible)) {
      const title=dialog.getAttribute('aria-label')
      const own=(selector:string)=>[...dialog.querySelectorAll(selector)].filter(el=>el.closest('dialog')===dialog && visible(el))
      if(own('.detail-modal-heading .detail-modal-close').length!==1)issues.push(`${title}: missing or duplicate header close`)
      for(const button of own('.detail-modal-content button,.detail-modal-footer button')) {
        if(/^(关闭|取消|Close|Cancel)$/i.test(button.textContent?.trim()??''))issues.push(`${title}: duplicate dismiss button`)
      }
      for(const button of own('.ui-form > .button')) {
        const form=button.parentElement!,style=getComputedStyle(form)
        const right=form.getBoundingClientRect().right-parseFloat(style.paddingRight)-parseFloat(style.borderRightWidth)
        if(Math.abs(right-button.getBoundingClientRect().right)>2)issues.push(`${title}: form submit not right aligned`)
      }
      for(const row of own('.ui-form-actions,.detail-modal-footer')) {
        if(row.closest('.ui-filter-bar'))continue
        const buttons=[...row.children].filter(el=>el.matches('button,.button') && visible(el))
        if(!buttons.length)continue
        const style=getComputedStyle(row),right=row.getBoundingClientRect().right-parseFloat(style.paddingRight)
        const lastRight=Math.max(...buttons.map(el=>el.getBoundingClientRect().right))
        if(style.justifyContent!=='flex-end'||Math.abs(right-lastRight)>2)issues.push(`${title}: actions not right aligned (${row.className})`)
        if(buttons.some(el=>getComputedStyle(el).flexGrow!=='0'))issues.push(`${title}: actions stretch (${row.className})`)
      }
    }
    return issues
  })
  expect(issues).toEqual([])
}

export async function expectPageActionControls(page: Page) {
  const issues = await page.locator('.ui-form-actions, .ui-form > .button').evaluateAll(elements => {
    const issues: string[] = []
    for (const element of elements) {
      if (element.closest('dialog, .ui-filter-bar, .login-entry, .auth-form')) continue
      const box = element.getBoundingClientRect()
      if (!box.width || !box.height) continue
      const row = element.matches('.button') ? element.parentElement! : element
      const style = getComputedStyle(row), bounds = row.getBoundingClientRect()
      const right = bounds.right - parseFloat(style.paddingRight) - parseFloat(style.borderRightWidth)
      const buttons = element.matches('.button') ? [element] : [...row.children].filter(child => child.matches('button, .button'))
      const boxes = buttons.map(button => button.getBoundingClientRect()).filter(box => box.width && box.height)
      for (const top of new Set(boxes.map(box => Math.round(box.top)))) {
        const edge = Math.max(...boxes.filter(box => Math.round(box.top) === top).map(box => box.right))
        if (Math.abs(right - edge) > 2) issues.push(`${row.className}: page actions not right aligned`)
      }
    }
    return issues
  })
  expect(issues).toEqual([])
}

export async function expectNoPageOverflow(page: Page) {
  const redundantCopy = await page.locator('h1,h2,h3,h4,th,dt,label,button,[role=option],input[placeholder],textarea[placeholder]').evaluateAll((elements, pattern) => {
    const forbidden = new RegExp(pattern, 'i')
    return elements.filter(el => el.getClientRects().length && getComputedStyle(el).visibility !== 'hidden').flatMap(el => {
      const values = [el.textContent?.trim(), el.getAttribute('placeholder'), el.getAttribute('aria-label')].filter((text): text is string => Boolean(text))
      return values.filter(text => forbidden.test(text))
    })
  }, redundantAnnotation.source)
  expect(redundantCopy, 'Visible labels and placeholders must omit redundant parenthetical instructions').toEqual([])
  await expectFilterSpacing(page)
  await expect.poll(() => page.evaluate(() => innerWidth)).toBe(page.viewportSize()!.width)
  await page.locator('.locale-picker__trigger').evaluateAll(async buttons => {
    await Promise.all(buttons.flatMap(button => button.getAnimations().map(animation => animation.finished)))
  })
  await page.locator('.locale-picker__trigger').evaluateAll(async buttons => {
    for (const button of buttons) getComputedStyle(button).width
    await Promise.all(buttons.flatMap(button => button.getAnimations().map(animation => animation.finished)))
  })
  const dimensions = await page.evaluate(() => ({
    viewport: innerWidth, document: document.documentElement.scrollWidth,
    overflow: document.documentElement.scrollWidth > innerWidth + 1 ? [...document.querySelectorAll('body *')].filter(node => node.getBoundingClientRect().width && (node.getBoundingClientRect().right > innerWidth + 1 || node.scrollWidth > node.clientWidth + 1)).slice(-12).map(node => ({ tag: node.tagName, class: node.className, right: node.getBoundingClientRect().right, width: node.clientWidth, scroll: node.scrollWidth, text: node.textContent?.slice(0, 80) })) : [],
    icons: [...document.querySelectorAll('svg.site-icon')].flatMap(icon => {
      const bounds = icon.getBoundingClientRect();
      if (!bounds.width && !bounds.height) return [];
      // CSS dimensions stay constant while an accordion chevron rotates.
      const style = getComputedStyle(icon), width = parseFloat(style.width), height = parseFloat(style.height);
      const stroke = style.strokeWidth;
      const size = icon.closest('.detail-modal-close') ? 16 : icon.closest('.topbar,.workspace-topbar') ? 20 : icon.closest('.login-entry') ? 20 : icon.closest('.management-console,.platform-login') ? 16 : 20;
      return width !== size || height !== size || stroke !== '1.75px'
        ? [{ icon: icon.getAttribute('class'), width, height, stroke }] : [];
    }),
    languages: [...document.querySelectorAll('.locale-picker__trigger')].flatMap(button => {
      const bounds = button.getBoundingClientRect();
      if (!bounds.width && !bounds.height) return [];
      const size = matchMedia('(min-width: 901px) and (pointer: fine)').matches ? 32 : 44;
      return [{ width: bounds.width, height: bounds.height, radius: getComputedStyle(button).borderRadius, size }];
    }),
  }))
  expect(dimensions.viewport, JSON.stringify(dimensions)).toBe(page.viewportSize()!.width)
  expect(dimensions.document, JSON.stringify(dimensions)).toBeLessThanOrEqual(dimensions.viewport + 1)
  expect(dimensions.icons).toEqual([])
  await expectModalControls(page)
  await expectPageActionControls(page)
  for (const language of dimensions.languages) {
    // Transformed drawer bounds can contain subpixel floating-point noise.
    expect(language.width).toBeCloseTo(language.size, 2)
    expect(language.height).toBeCloseTo(language.size, 2)
    expect(language.radius).toBe('10px')
  }
}

export async function installOrdersLayoutApi(page: Page) {
  await page.route('**/api/**', async route => {
    const path = new URL(route.request().url()).pathname;
    const respond = (data: unknown) => {
      expect(route.request().method()).toBe('GET');
      return route.fulfill({ json: { success: true, data } });
    };
    if (path === '/api/v1/me') return respond({
      user: { id: gameId, username: 'layout-member', status: 'active' },
      member: { id: gameId, brand_id: gameId, status: 'normal', display_name: 'Layout member', joined_at: '2026-10-17T00:00:00Z' },
    });
    if (['/api/v1/wallet/ledger', '/api/v1/configured/orders', `/api/v1/configured/games/${gameId}/orders`].includes(path)) return respond({ items: [] , total_count: '0'});
    if (path === '/api/v1/betting-history') {
      const query = new URL(route.request().url()).searchParams;
      return respond({
        brand_id: gameId, member_id: gameId, game_id: query.get('game_id') ?? '',
        games: [
          { id: gameId, name: 'Layout Mark Six' },
          { id: 'aaaaaaaa-aaaa-4aaa-8aaa-000000000002', name: 'Layout Canada 28' },
          { id: 'aaaaaaaa-aaaa-4aaa-8aaa-000000000003', name: 'Layout Racing' },
        ],
        items: [], limit: 20, next_cursor: null,
       total_count: '0'});
    }
    if (path === '/api/v1/draw-archive') return respond({game_id:new URL(route.request().url()).searchParams.get('game_id'),game_name:'Mark Six',mark_six:true,timezone:'Asia/Singapore',daily:false,choices:['2026','2025'],selected:new URL(route.request().url()).searchParams.get('selected') || '2026',items:[],has_more:false, total_count: '0'});
    if (path === '/api/v1/platform-games') return respond({ games: [], limit: 50, offset: 0 , total_count: '0'});
    if (path === '/api/v1/draw-results') {
      const query = new URL(route.request().url()).searchParams;
      return respond({ items: [], brand_status: 'active', has_more: false, limit: Number(query.get('limit')), offset: Number(query.get('offset')), server_time: new Date().toISOString() , total_count: '0'});
    }
    if (path === '/api/v1/notifications') {
      const query = new URL(route.request().url()).searchParams;
      return respond({ brand_id: gameId, member_id: gameId, items: [], unread_count: '0', limit: Number(query.get('limit')), offset: Number(query.get('offset')) , total_count: '0'});
    }
    if (path === '/api/v1/wallet') return respond({
      account_id: gameId, brand_id: gameId, member_id: gameId, version: 1,
      display_points: '0', available_points: '0', recharge_points: '0', frozen_points: '0', withdrawal_points: '0', winning_points: '0', gift_points: '0', commission_points: '0', manual_frozen_points: '0', system_frozen_points: '0',
      by_source: Object.fromEntries(['recharge', 'winning', 'gift', 'commission'].map(source => [source, { available: '0', manual_frozen: '0', system_frozen: '0', withdrawal: '0' }])),
    });
    return route.fallback();
  });
}

export async function expectInsideViewport(locator: Locator) {
  await expect(locator).toBeVisible()
  const bounds = await locator.evaluate(el => {
    const r = el.getBoundingClientRect()
    return { left: r.left, top: r.top, right: r.right, bottom: r.bottom, width: innerWidth, height: innerHeight }
  })
  expect(bounds.left).toBeGreaterThanOrEqual(-1)
  expect(bounds.top).toBeGreaterThanOrEqual(-1)
  expect(bounds.right).toBeLessThanOrEqual(bounds.width + 1)
  expect(bounds.bottom).toBeLessThanOrEqual(bounds.height + 1)
}

// A constrained vertical flex list must scroll instead of shrinking wrapped labels.
export async function expectPlayTabLabelsFit(page: Page) {
  const clipped = await page.locator('.configured-play-tabs button, .play-tabs button, .standard-play-groups button').evaluateAll(buttons => buttons
    .filter(button => button.getBoundingClientRect().width > 0)
    .filter(button => button.scrollHeight > button.clientHeight + 1)
    .map(button => ({ label: button.textContent, height: button.clientHeight, contentHeight: button.scrollHeight })))
  expect(clipped).toEqual([])
}

export async function expectRefreshControls(page: Page) {
  const menu=page.locator('.workspace-menu-layer');
  const open=await menu.count()>0 && !await menu.isVisible();
  if(open)await page.locator('.workspace-menu-trigger').click();
  const buttons = page.locator('[data-refresh-button]').filter({ visible: true });
  for (const button of await buttons.all()) {
    const management = await button.evaluate(el => !!el.closest('.management-console'));
    const compact = management && await page.evaluate(() => matchMedia('(min-width: 901px) and (pointer: fine)').matches);
    await expect(button).toHaveCSS('width', compact ? '32px' : '44px');
    await expect(button).toHaveCSS('height', compact ? '32px' : '44px');
    await expect(button).toHaveCSS('border-radius', '10px');
    await expect(button).toHaveCSS('margin', '0px');
    await expect(button).toHaveCSS('text-indent', '-9999px');
    expect(await button.getAttribute('aria-label')).toBeTruthy();
    expect(await button.getAttribute('title')).toBe(await button.getAttribute('aria-label'));
    const svg = button.locator('[data-refresh-icon] svg');
    await expect(svg).toBeVisible();
    await expect(svg).toHaveAttribute('stroke-width', '1.75');
    const bounds = await svg.boundingBox();
    expect(bounds!.width).toBeCloseTo(management ? 16 : 20,4);
    expect(bounds!.height).toBeCloseTo(management ? 16 : 20,4);
    const icon = await svg.evaluate(el => {
      const style = getComputedStyle(el);
      return { width: style.width, height: style.height, gap: getComputedStyle(el.closest('button')!.parentElement!).gap };
    });
    expect(icon).toMatchObject({ width: management ? '16px' : '20px', height: management ? '16px' : '20px', gap: '12px' });
  }
  if(open)await page.locator('.workspace-menu-close').click();
}

export const cashBalanceFixture = (): CashBalance[] => currencies.map(currency => ({currency,deposited:'0',withdrawable:'0',reserved:'0'}))

// Populated wallet snapshots deliberately distinguish available, frozen and reserved points.
export async function installWalletLayoutApi(page: Page) {
  await installOrdersLayoutApi(page);
  const config: CashConfig = {version:1,rules:currencies.map(currency=>({currency,inherit:false,enabled:false,rate:'1',deposit_min:'1',deposit_max:'10000',withdrawal_min:'1',withdrawal_max:'10000',fee:'0',mode:'count',threshold:'1'}))}
  await page.route('**/api/v1/cash',route=>{
    expect(route.request().method()).toBe('GET')
    return route.fulfill({json:{success:true,data:{auto_usdt:false,actor_context:'a'.repeat(64),brand_id:gameId,member_id:gameId,balances:cashBalanceFixture(),destinations:[],platform:config,brand:config}}})
  })
  await page.route('**/api/v1/cash/orders?**',route=>{
    expect(route.request().method()).toBe('GET')
    return route.fulfill({json:{success:true,data:{items:[],total_count:'0'}}})
  })
  const buckets = (available: string, manual: string) => Object.fromEntries(
    ['recharge', 'winning', 'gift', 'commission'].map(source => [source, source === 'recharge'
      ? { available, manual_frozen: manual, system_frozen: '30', withdrawal: '40' }
      : { available: '0', manual_frozen: '0', system_frozen: '0', withdrawal: '0' }]),
  );
  const before = buckets('130', '10');
  const after = buckets('120', '20');
  const delta = Object.fromEntries(['recharge', 'winning', 'gift', 'commission'].map(source => [source, {
    available: source === 'recharge' ? '-10' : '0', manual_frozen: source === 'recharge' ? '10' : '0', system_frozen: '0', withdrawal: '0',
  }]));
  await page.route('**/api/v1/wallet', route => route.fulfill({ json: { success: true, data: {
    account_id: gameId, brand_id: gameId, member_id: gameId, version: 2,
    display_points: '170', available_points: '120', frozen_points: '50', withdrawal_points: '40',
    recharge_points: '120', winning_points: '0', gift_points: '0', commission_points: '0', manual_frozen_points: '20', system_frozen_points: '30', by_source: after,
  } } }));
  await page.route('**/api/v1/wallet/ledger?**', route => route.fulfill({ json: { success: true, data: { items: [{
    id: gameId, brand_id: gameId, member_id: gameId, account_id: gameId,
    entry_type: 'freeze', reference_type: 'manual', reference_id: gameId,
    before_snapshot: before, delta_snapshot: delta, after_snapshot: after,
    reason: 'Layout fixture: manual freeze', version: 2, created_at: '2026-10-17T00:00:00Z',
  }] , total_count: '1'} } }));
}

// Measure the rendered chrome and page rhythm, including styles from scoped pages.
export async function expectCompactPageLayout(page: Page) {
  const metrics = await page.evaluate(() => {
    const visible = (element: Element) => element.getBoundingClientRect().width > 1 && element.getBoundingClientRect().height > 1
    return {
      desktop: matchMedia('(min-width:901px) and (pointer:fine)').matches,
      navigation: [...document.querySelectorAll('.management-console .side-nav :is(.nav-item,.nav-subitem)')].filter(visible).map(element=>{const css=getComputedStyle(element);return {top:parseFloat(css.paddingTop),bottom:parseFloat(css.paddingBottom)}}),
      headers: [...document.querySelectorAll('.management-console .topbar')].filter(visible).map(element => {
        const style = getComputedStyle(element)
        return { top: parseFloat(style.paddingTop), bottom: parseFloat(style.paddingBottom) }
      }),
      headings: [...document.querySelectorAll('.management-console .page-heading, .management-console .page-head, .management-console .withdrawal-heading, .workspace-frame .page-heading')].filter(visible).map(element => {
        const style = getComputedStyle(element)
        return { top: parseFloat(style.marginTop), bottom: parseFloat(style.marginBottom) }
      }),
      content: [...document.querySelectorAll('.management-console .main-column > .content, .management-console .main-shell > .page-content, .workspace-frame .main-column > .content')].filter(visible).map(element => parseFloat(getComputedStyle(element).paddingTop)),
    }
  })
  if(metrics.desktop)for(const row of metrics.navigation){expect(row.top).toBeLessThanOrEqual(4);expect(row.bottom).toBeLessThanOrEqual(4)}
  for (const header of metrics.headers) { expect(header.top).toBeLessThanOrEqual(4); expect(header.bottom).toBeLessThanOrEqual(4) }
  for (const heading of metrics.headings) { expect(heading.top).toBe(0); expect(heading.bottom).toBeLessThanOrEqual(12) }
  for (const top of metrics.content) expect(top).toBeLessThanOrEqual(12)
}

export async function openPlatformBrandList(page: Page, locale: 'en' | 'zh-CN') {
  await expect(page.locator('.management-console')).toBeVisible()
  const menu = page.locator('button[aria-controls="platform-navigation"]')
  if (await menu.isVisible() && await menu.getAttribute('aria-expanded') === 'false') await menu.click()
  const parent = page.locator('.sidebar button[aria-controls="platform-brand-tabs"]')
  if (await parent.getAttribute('aria-expanded') === 'false') await parent.click()
  await page.getByTestId('platform-brand-tabs').getByRole('button', { name: locale === 'en' ? 'Brand list' : '品牌列表', exact: true }).click()
  await expect(page.getByTestId('platform-brand-list')).toBeVisible()
}
