import { orderTicketCatalog } from '../../shared/src/order-ticket-catalog'
import { createRenderer, h, nextTick, shallowRef, type Component } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { afterEach, describe, expect, it, vi } from 'vitest'
import * as VueRuntime from 'vue'
import * as RouterRuntime from 'vue-router'
import * as Shared from '@lottery/shared'
import * as HistoryApi from './betting-history-api'
import * as UserI18n from './i18n'
import { compileScript, parse } from 'vue/compiler-sfc'
import ts from 'typescript'
import { readFileSync } from 'node:fs'
function clientComponent(filename: string, modules: Record<string, unknown>): Component {
  const descriptor = parse(readFileSync(new URL(filename, import.meta.url), 'utf8')).descriptor
  const compiled = compileScript(descriptor, { id: filename, inlineTemplate: true }).content
  const javascript = ts.transpileModule(compiled, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText
  const body = javascript.replace(/^import\s+\{([\s\S]*?)\}\s+from\s+["']([^"']+)["'];?\s*$/gm, (_match, bindings: string, path: string) => `const {${bindings.replace(/\s+as\s+/g, ': ')}}=__modules[${JSON.stringify(path)}];`)
    .replace(/^import\s+(\w+)\s+from\s+["']([^"']+)["'];?\s*$/gm, (_match, binding: string, path: string) => `const ${binding}=__modules[${JSON.stringify(path)}];`).replace(/export\s+default\s+/, 'return ')
  return new Function('__modules', body)(modules) as Component
}
const commonModules = { vue: VueRuntime, '@lottery/shared': Shared, './i18n': UserI18n }
const OrderHistoryLayout = clientComponent('./OrderHistoryLayout.vue', commonModules)
const ticket = (kind: string) => ({ props: ['orders', 'order'], setup: (props: any) => () => h('div', { 'data-kind': kind }, (props.orders ?? [props.order]).map((order: any) => h('p', order.id + ' ' + (order.total_points ?? order.quote.bet_points)))) })
const OrderHistoryRecord = clientComponent('../../shared/src/OrderHistoryRecord.vue', {vue:VueRuntime,'./index':Shared,'./localization':Shared,'./order-ticket-catalog':{orderTicketCatalog},'./MarkSixOrders.vue':ticket('mark-six'),'./CanadaOrders.vue':ticket('canada'),'./ConfiguredOrderTicket.vue':ticket('configured'),'./SelectionSummary.vue':{render:()=>null},'./OrderPrize.vue':{render:()=>null}})
const BettingHistory = clientComponent('./BettingHistory.vue', { ...commonModules, 'vue-router': RouterRuntime, './betting-history-api': HistoryApi, './OrderHistoryLayout.vue': OrderHistoryLayout, '@lottery/shared/OrderHistoryRecord.vue': OrderHistoryRecord })
import { createUserI18n, userI18nKey } from './i18n'
import { setPlatformDisplaySettings } from '@lottery/shared'
import type { AuthProfile } from '../../shared/src/auth'

type Node = { tag: string; text: string; props: Record<string, any>; children: Node[]; parent: Node | null; value?: unknown; selected?: boolean; options: Node[]; addEventListener: (...args: any[]) => void; removeEventListener: (...args: any[]) => void; getRootNode: () => Node }
const node = (tag: string, text = ''): Node => ({ tag, text, props: {}, children: [], parent: null, get options() { return this.children.filter(n => n.tag === 'option') }, addEventListener() {}, removeEventListener() {}, getRootNode() { return this } })
const refreshTarget = node('refresh-target')
const renderer = createRenderer<Node, Node>({
  createElement: tag => node(tag), createText: text => node('#text', text), createComment: text => node('#comment', text),
  setText: (n, text) => { n.text = text }, setElementText: (n, text) => { n.text = text; n.children = [] },
  patchProp: (n, key, _old, value) => { n.props[key] = value; if (key === 'value') { n.value = value; (n as any)._value = value } },
  insert(n, parent, anchor) { if (n.parent) n.parent.children.splice(n.parent.children.indexOf(n), 1); n.parent = parent; const i = anchor ? parent.children.indexOf(anchor) : -1; if (i < 0) parent.children.push(n); else parent.children.splice(i, 0, n) },
  remove(n) { if (n.parent) n.parent.children.splice(n.parent.children.indexOf(n), 1) }, parentNode: n => n.parent,
  nextSibling: n => n.parent?.children[n.parent.children.indexOf(n) + 1] ?? null, querySelector: () => refreshTarget,
})
const text = (n: Node): string => n.text + n.children.map(text).join('')
const find = (n: Node, check: (n: Node) => boolean): Node | undefined => check(n) ? n : n.children.map(child => find(child, check)).find(Boolean)
const click = (n: Node | undefined) => { expect(n).toBeDefined(); n!.props.onClick() }
const apps: Array<{ unmount(): void }> = []
afterEach(() => { for (const app of apps.splice(0)) app.unmount(); vi.unstubAllGlobals() })
const brand = '11111111-1111-4111-8111-111111111111', member = '22222222-2222-4222-8222-222222222222'
const game = '33333333-3333-4333-8333-333333333333'
const profile = { user: { id: member, status: 'active' }, member: { id: member, brand_id: brand, status: 'active', display_name: 'test', joined_at: '2026-10-20T00:00:00Z' } } as AuthProfile
const entry = (kind = 'standard', id = 'record-1') => ({ id, kind, game_id: game, game_name: 'Retired named game', period_no: '2026-001', placed_at: '2026-10-20T00:00:00Z', order: { id, game_id: game, period_no: '2026-001', status: 'placed', total_points: '9007199254740993', prize_points: '0', multiplier: '1', selection_normalized: { regular: [1], special: [], digits: [] }, settled_at: null, items: [], attributes: {}, plate: 'A', profile: 'canada_niuniu', reserved_points: '9007199254740994', lost_points: '0', refund_points: '0', effective_points: '0', snapshot: { catalog: { plays: [] } }, quote: { lines: [], bet_points: '9007199254740993' }, result: null } })
const page = (items = [entry()], changes = {}) => ({ brand_id: brand, member_id: member, game_id: '', games: [{ id: game, name: 'Retired named game' }], items, limit: 20, next_cursor: null, ...changes })
const ok = (value: unknown) => new Response(JSON.stringify({ success: true, data: value }))
async function flush() { for (let i = 0; i < 40; i++) await Promise.resolve(); await nextTick() }
async function mount(fetcher: typeof fetch) {
  vi.stubGlobal('fetch', fetcher); vi.stubGlobal('navigator', { language: 'en', languages: ['en'] })
  setPlatformDisplaySettings({ timezone: 'Asia/Singapore', date_format: 'YYYY-MM-DD', time_format: 'HH:mm:ss' })
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/:pathMatch(.*)*', component: { render: () => null } as Component }] }); await router.push('/orders')
  const auth = shallowRef<AuthProfile | null>(profile), root = node('root'), i18n = createUserI18n(); i18n.setLocale('en')
  const app = renderer.createApp({ setup: () => () => h(BettingHistory, { auth: auth.value, brandCode: 'review', locale: i18n.locale.value === 'en' ? 'en' : 'zh' }) }).provide(userI18nKey, i18n).use(router)
  app.mount(root); apps.push(app); await flush(); return { root, auth, i18n }
}
describe('unified betting history', () => {
  it('renders all four sources together under actual game names and translates without refetching', async () => {
    const fetcher = vi.fn<typeof fetch>().mockImplementation(async () => ok(page(['configured', 'standard', 'mark-six', 'canada'].map((kind, i) => entry(kind, `ticket-${i}`)))))
    const m = await mount(fetcher)
    for (const kind of ['configured', 'mark-six', 'canada']) expect(find(m.root, n => n.props['data-kind'] === kind)).toBeDefined()
    expect(text(m.root).indexOf('Betting history')).toBeLessThan(text(m.root).indexOf('All games'))
    expect(find(m.root, n => n.props.class === 'history-filters')?.children.some(n => n.tag === 'label')).toBe(false)
    expect(text(m.root)).toContain('Retired named game'); expect(text(m.root)).toContain('9007199254740993')
    expect(find(m.root, n => n.props.class === 'order-categories')).toBeUndefined(); expect(fetcher).toHaveBeenCalledOnce()
    expect(find(m.root, n => n.tag === 'select')?.options.map(n => n.props.value)).toEqual(['', game])
    m.i18n.setLocale('zh-CN'); await flush(); expect(text(m.root)).toContain('全部彩种'); expect(fetcher).toHaveBeenCalledOnce()
  })
  it('shares next/previous cursors and resets pagination when the actual game changes', async () => {
    const fetcher = vi.fn<typeof fetch>().mockImplementation(async input => {
      const q = new URL(String(input), 'http://local').searchParams
      return ok(page([entry('standard', q.has('cursor') ? 'second-ticket' : 'first-ticket')], { game_id: q.get('game_id') ?? '', next_cursor: q.has('cursor') ? null : 'page-two' }))
    })
    const m = await mount(fetcher)
    click(find(m.root, n => n.tag === 'button' && text(n) === 'Next')); await flush()
    expect(String(fetcher.mock.calls.at(-1)![0])).toContain('cursor=page-two'); expect(text(m.root)).toContain('Page 2')
    click(find(m.root, n => n.tag === 'button' && text(n) === 'Previous')); await flush()
    expect(String(fetcher.mock.calls.at(-1)![0])).not.toContain('cursor=')
    const select = find(m.root, n => n.tag === 'select')!; select.props['onUpdate:modelValue'](game); select.props.onChange(); await flush()
    expect(String(fetcher.mock.calls.at(-1)![0])).toContain(`game_id=${game}`); expect(text(m.root)).toContain('Page 1')
    expect(String(fetcher.mock.calls.at(-1)![0])).not.toContain('cursor=')
  })
  it('rejects late responses after account changes and never renders the previous member’s tickets', async () => {
    let resolve!: (value: Response) => void
    const slow = new Promise<Response>(done => { resolve = done })
    const fetcher = vi.fn<typeof fetch>().mockReturnValueOnce(slow).mockImplementation(async () => ok(page([], { member_id: 'other-member' })))
    const m = await mount(fetcher); m.auth.value = { ...profile, member: { ...profile.member, id: 'other-member' } }; await flush()
    resolve(ok(page([entry('standard', 'private-old-ticket')]))); await flush()
    expect(text(m.root)).not.toContain('private-old-ticket'); expect(text(m.root)).toContain('No tickets yet')
  })
})
