import * as PaginationCatalog from '../../shared/src/pagination-catalog'
import * as Localization from '../../shared/src/localization'
import * as MarkSix from '../../shared/src/mark-six'
import * as Canada from '../../shared/src/canada'
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
const TablePagination = clientComponent('../../shared/src/TablePagination.vue', {vue: VueRuntime, './localization': Localization, './pagination-catalog': PaginationCatalog, './global-loading': Shared, './SelectControl.vue': { props: ['modelValue', 'options', 'label'], emits: ['change'], setup(_props: unknown, { emit }: { emit: (event: string, size: number) => void }) { return () => h('size-control', { onChange: (size: number) => emit('change', size) }) } }})
const commonModules = { '@lottery/shared/TablePagination.vue': TablePagination, vue: VueRuntime, '@lottery/shared': Shared, './i18n': UserI18n }
const OrderHistoryLayout = clientComponent('./OrderHistoryLayout.vue', commonModules)
const BettingHistoryTable = clientComponent('./BettingHistoryTable.vue', { ...commonModules, 'vue-router': RouterRuntime,
  '../../shared/src/mark-six': MarkSix, '../../shared/src/canada': Canada,
  '@lottery/shared/MarkSixLine.vue': { render: () => null }, '@lottery/shared/CanadaLine.vue': { render: () => null },
  '@lottery/shared/SelectionSummary.vue': { render: () => null },
})
const BettingHistory = clientComponent('./BettingHistory.vue', { ...commonModules, 'vue-router': RouterRuntime, './betting-history-api': HistoryApi, './OrderHistoryLayout.vue': OrderHistoryLayout, './BettingHistoryTable.vue': BettingHistoryTable })
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
const entry = (kind = 'configured', id = 'record-1') => ({ id, kind, game_id: game, game_name: 'Retired named game', period_no: '2026-001', placed_at: '2026-10-20T00:00:00Z', order: { id, game_id: game, period_no: '2026-001', status: 'placed', total_points: '9007199254740993', prize_points: '0', multiplier: '1', definition_snapshot: {prize_tiers: []}, selection_normalized: { regular: [1], special: [], digits: [] }, settled_at: null, items: [], attributes: {}, plate: 'A', profile: 'canada_niuniu', reserved_points: '9007199254740994', lost_points: '0', refund_points: '0', effective_points: '0', snapshot: { catalog: { plays: []  ,total_count: '0'} }, quote: { lines: [], bet_points: '9007199254740993' }, result: null  ,total_count: '0'} })
const page = (items = [entry()], changes = {}) => ({ brand_id: brand, member_id: member, game_id: '', games: [{ id: game, name: 'Retired named game' }], items, limit: 20, next_cursor: null, total_count: String(items.length), ...changes})
const ok = (value: unknown) => new Response(JSON.stringify({ success: true, data: value }))
async function flush() { for (let i = 0; i < 40; i++) await Promise.resolve(); await nextTick() }
async function mount(fetcher: typeof fetch, initialGameId = '') {
  vi.stubGlobal('fetch', fetcher); vi.stubGlobal('navigator', { language: 'en', languages: ['en'] })
  setPlatformDisplaySettings({ timezone: 'Asia/Singapore', date_format: 'YYYY-MM-DD', time_format: 'HH:mm:ss' })
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/:pathMatch(.*)*', component: { render: () => null } as Component }] }); await router.push('/orders')
  const selectedGameId = shallowRef(initialGameId)
  const auth = shallowRef<AuthProfile | null>(profile), root = node('root'), i18n = createUserI18n(); i18n.setLocale('en')
  const app = renderer.createApp({ setup: () => () => h(BettingHistory, { auth: auth.value, gameId: selectedGameId.value, brandCode: 'review', locale: i18n.locale.value === 'en' ? 'en' : 'zh' }) }).provide(userI18nKey, i18n).use(router)
  app.mount(root); apps.push(app); await flush(); return { root, auth, i18n, selectedGameId }
}
describe('unified betting history', () => {
  it('uses the navigation game and catalog even before any bets, and stays on history when switching', async () => {
    const other = '44444444-4444-4444-8444-444444444444'
    const fetcher = vi.fn<typeof fetch>().mockImplementation(async input => {
      const id = new URL(String(input), 'http://local').searchParams.get('game_id') ?? ''
      return ok(page([], { game_id: id, games: []  ,total_count: '0'}))
    })
    const m = await mount(fetcher, game)
    expect(find(m.root, n => n.tag === 'nav')).toBeUndefined()
    expect(find(m.root, n => n.tag === 'select-control')).toBeUndefined()
    expect(String(fetcher.mock.calls.at(-1)![0])).toContain(`game_id=${game}`)
    m.selectedGameId.value = other; await flush()
    expect(String(fetcher.mock.calls.at(-1)![0])).toContain(`game_id=${other}`)

  })

  it('renders configured tickets in one table without repeated game names or UUIDs and translates without refetching', async () => {
    const fetcher = vi.fn<typeof fetch>().mockImplementation(async () => ok(page(Array.from({length:4},(_,i)=>entry('configured',`ticket-${i}`)), {game_id: game})))
    const m = await mount(fetcher, game)
    expect(find(m.root, n => n.tag === 'table')).toBeDefined()
    expect(find(m.root, n => n.tag === 'tbody')?.children.filter(n => n.tag === 'tr')).toHaveLength(4)
    expect(text(m.root)).toContain('9007199254740993')
    expect(text(m.root)).not.toContain('Retired named game')
    expect(text(m.root)).not.toContain('ticket-0')
    expect(find(m.root, n => n.tag === 'select-control')).toBeUndefined()
    expect(text(m.root)).toContain('Bet details'); expect(fetcher).toHaveBeenCalledOnce()
    m.i18n.setLocale('zh-CN'); await flush(); expect(text(m.root)).toContain('投注内容'); expect(fetcher).toHaveBeenCalledOnce()
  })
  it('shares next/previous cursors and resets pagination when the actual game changes', async () => {
    const fetcher = vi.fn<typeof fetch>().mockImplementation(async input => {
      const q = new URL(String(input), 'http://local').searchParams
      return ok(page([entry('configured', q.has('cursor') ? 'second-ticket' : 'first-ticket')], { game_id: q.get('game_id') ?? '', next_cursor: q.has('cursor') ? null : 'page-two', total_count: '21' }))
    })
    const m = await mount(fetcher)
    click(find(m.root, n => n.tag === 'button' && n.props['aria-label'] === 'Next')); await flush()
    expect(String(fetcher.mock.calls.at(-1)![0])).toContain('cursor=page-two'); expect(text(m.root)).toContain('Page 2')
    click(find(m.root, n => n.tag === 'button' && n.props['aria-label'] === 'Previous')); await flush()
    expect(String(fetcher.mock.calls.at(-1)![0])).not.toContain('cursor=')
    m.selectedGameId.value = game; await flush()
    expect(String(fetcher.mock.calls.at(-1)![0])).toContain(`game_id=${game}`); expect(text(m.root)).toContain('Page 1')
    expect(String(fetcher.mock.calls.at(-1)![0])).not.toContain('cursor=')
  })
  it('reloads from the first page with the selected size', async () => {
    const fetcher = vi.fn<typeof fetch>().mockImplementation(async input => {
      const q = new URL(String(input), 'http://local').searchParams
      return ok(page([entry()], { limit: Number(q.get('limit')), next_cursor: 'next', total_count: '101' }))
    })
    const m = await mount(fetcher)
    click(find(m.root, n => n.tag === 'button' && n.props['aria-label'] === 'Next')); await flush()
    find(m.root, n => n.tag === 'size-control')!.props.onChange(50); await flush()
    const q = new URL(String(fetcher.mock.calls.at(-1)![0]), 'http://local').searchParams
    expect(q.get('limit')).toBe('50'); expect(q.has('cursor')).toBe(false); expect(text(m.root)).toContain('Page 1')
  })
  it('rejects late responses after account changes and never renders the previous member’s tickets', async () => {
    let resolve!: (value: Response) => void
    const slow = new Promise<Response>(done => { resolve = done })
    const fetcher = vi.fn<typeof fetch>().mockReturnValueOnce(slow).mockImplementation(async () => ok(page([], { member_id: 'other-member' })))
    const m = await mount(fetcher); m.auth.value = { ...profile, member: { ...profile.member, id: 'other-member' } }; await flush()
    resolve(ok(page([entry('configured', 'private-old-ticket')]))); await flush()
    expect(text(m.root)).not.toContain('private-old-ticket'); expect(text(m.root)).toContain('No bet orders match the current game and filters.')
  })
})
