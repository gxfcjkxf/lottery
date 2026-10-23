import { BusinessDetailsTestComponent as BusinessDetails } from "../../admin-web/src/business-details-test-helper";
import SiteIcon from "../../shared/src/SiteIcon";
import { h as temporalTestH } from "vue";
import * as DisplayShared from "@lottery/shared";
DisplayShared.setPlatformDisplaySettings({ timezone: "Asia/Singapore", date_format: "YYYY-MM-DD", time_format: "HH:mm:ss" });
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createRenderer, h, nextTick, ref, type Component } from 'vue'
import * as VueRuntime from 'vue'
import { compileScript, parse } from 'vue/compiler-sfc'
import { readFileSync } from 'node:fs'
import ts from 'typescript'
import * as Localization from '../../shared/src/localization'
import * as FormValidation from '../../shared/src/form-validation'
import * as LotteryApi from '../../shared/src/platform-lottery'
import * as LotteryTime from '../../shared/src/lottery-time'
import * as DateTime from '../../shared/src/date-time'
import * as PlayForm from '../../shared/src/platform-play-form'
import type { RuleModel } from '../../shared/src/rules'

function component(file = 'PlatformLottery.vue'): Component {
  const descriptor = parse(readFileSync(new URL(`../../shared/src/${file}`, import.meta.url), 'utf8')).descriptor
  const js = ts.transpileModule(compileScript(descriptor, { id: 'unified-catalog-test', inlineTemplate: true }).content, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText
  const modules: Record<string, unknown> = { "./SiteIcon": { default: SiteIcon }, "@lottery/shared/PlatformTemporalInput.vue": { props: ["modelValue", "type", "required", "disabled"], setup: (p: any, { emit }: any) => () => temporalTestH("input", { type: p.type, value: p.modelValue, required: p.required, disabled: p.disabled, "onUpdate:modelValue": (v: string) => emit("update:modelValue", v) }) }, "./global-loading": DisplayShared, "./display-settings": DisplayShared,  "@lottery/shared": DisplayShared,  './form-validation': FormValidation, vue: VueRuntime, './localization': Localization, './platform-lottery': LotteryApi, './lottery-time': LotteryTime, './date-time': DateTime, './platform-play-form': PlayForm, './AutomationSettings.vue': {default:{render:()=>null}}, './PlatformPlayRuleForm.vue': { default: { render: () => null } }, './NumberModelEditor.vue': { default: file === 'PlatformLottery.vue' ? component('NumberModelEditor.vue') : null }, './PlatformDateInput.vue': { default: file === 'PlatformLottery.vue' ? component('PlatformDateInput.vue') : null }, './PlatformTimeInput.vue': { default: file === 'PlatformLottery.vue' ? component('PlatformTimeInput.vue') : null } }
  const body = js.replace(/^import\s+\{([\s\S]*?)\}\s+from\s+['"]([^'"]+)['"];?\s*$/gm, (_m, bindings: string, specifier: string) => `const {${bindings.replace(/\s+as\s+/g, ': ')}} = __modules[${JSON.stringify(specifier)}];`)
    .replace(/^import ConfiguredRuleWorkbench from .*$/m, 'const ConfiguredRuleWorkbench = { render: () => null };').replace(/^import SiteIcon from .*$/m, 'const SiteIcon = __modules["./SiteIcon"].default;').replace(/^import DetailModal from .*$/m, 'const DetailModal = __modal;').replace(/^import NumberModelEditor from .*$/m, 'const NumberModelEditor = __modules["./NumberModelEditor.vue"].default;').replace(/^import PlatformDateInput from .*$/m, 'const PlatformDateInput = __modules["./PlatformDateInput.vue"].default;').replace(/^import PlatformTimeInput from .*$/m, 'const PlatformTimeInput = __modules["./PlatformTimeInput.vue"].default;').replace(/^import AutomationSettings from .*$/m, 'const AutomationSettings = __modules["./AutomationSettings.vue"].default;').replace(/^import PlatformPlayRuleForm from .*$/m, 'const PlatformPlayRuleForm = __modules["./PlatformPlayRuleForm.vue"].default;').replace(/^import BusinessDetails from .*$/m, 'const BusinessDetails = __businessDetails;').replace(/export\s+default\s+/, 'return ')
  return new Function('__modules', '__modal', '__businessDetails', body)(modules, { render: () => null }, BusinessDetails) as Component
}

type Node = { tag: string; text: string; props: Record<string, unknown>; children: Node[]; parent: Node | null }
const node = (tag: string, text = ''): Node => Object.assign({ tag, text, props: {}, children: [], parent: null }, { style: {}, addEventListener() {}, removeEventListener() {}, showPopover() {}, hidePopover() {}, focus() {}, getBoundingClientRect: () => ({ left: 0, top: 0, bottom: 40, height: 320, width: 340 }) })
const refreshTarget = node('nav');
const renderer = createRenderer<Node, Node>({
  querySelector: () => refreshTarget,
  createElement: tag => node(tag), createText: text => node('#text', text), createComment: text => node('#comment', text),
  setText: (n, text) => { n.text = text }, setElementText: (n, text) => { n.text = text; n.children = [] },
  patchProp: (n, key, _old, value) => { n.props[key] = value },
  insert(n, parent, anchor) {
    if (n.parent) n.parent.children.splice(n.parent.children.indexOf(n), 1)
    n.parent = parent
    const index = anchor ? parent.children.indexOf(anchor) : -1
    if (index < 0) parent.children.push(n); else parent.children.splice(index, 0, n)
  },
  remove(n) { if (n.parent) n.parent.children.splice(n.parent.children.indexOf(n), 1) },
  parentNode: n => n.parent, nextSibling: n => n.parent?.children[n.parent.children.indexOf(n) + 1] ?? null,
})
const text = (n: Node): string => n.text + n.children.map(text).join('')
const all = (n: Node, tag: string): Node[] => [...(n.tag === tag ? [n] : []), ...n.children.flatMap(child => all(child, tag))]
const click = (root: Node, label: string) => { (all(root, 'button').find(n => text(n) === label)!.props.onClick as () => void)() }
async function flush() { for (let i = 0; i < 40; i++) await Promise.resolve(); await nextTick() }
function mount(count: number, offlineIndex = -1) {
  const platform = Array.from({ length: count }, (_, i) => ({ id: `platform-${i}`, name: `Platform ${i}`, code: `p_${i}`, timezone: 'UTC', connected: false, status: i === offlineIndex ? 'offline' : 'active', deletable: false, brand_enabled: false, brand_version: 0, version: 1, model: { model: 'DIGITS_0_9', length: 3 } }))
  const fetcher = vi.fn<typeof fetch>(async input => {
    const offset = Number(new URL(String(input), 'http://local').searchParams.get('offset'))
    return new Response(JSON.stringify({ success: true, data: { games: platform.slice(offset, offset + 50), limit: 50, offset } }))
  })
  vi.stubGlobal('fetch', fetcher)
  const scope = ref({ catalogBrandId: '', brandName: '', brandGames: [] as Array<{ id: string; name: string; code: string; timezone: string }> })
  const events: unknown[][] = [], root = node('root')
  const PlatformLottery = component()
  const app = renderer.createApp({ setup: () => () => h(PlatformLottery, { surface: 'platform', brandId: '', locale: 'zh-CN', displaySettings: { timezone: 'Asia/Singapore', date_format: 'YYYY-MM-DD', time_format: 'HH:mm:ss' }, canManage: true, canConnect: false, unifiedCatalog: true, ...scope.value, onBrandDetail: (...args: unknown[]) => events.push(args) }) })
  app.mount(root)
  return { app, root, scope, events, fetcher }
}
afterEach(() => vi.unstubAllGlobals())

describe('unified platform game catalog', () => {
  it('opens only the requested user game history without loading the whole game catalog', async () => {
    const selected = ref('game-a')
    const fetcher = vi.fn<typeof fetch>(async input => {
      const id = String(input).includes('/game-a?') ? 'game-a' : 'game-b'
      return new Response(JSON.stringify({ success: true, data: { game: { id, name: id }, periods: [], plays: [] } }))
    })
    vi.stubGlobal('fetch', fetcher)
    const History = component()
    const app = renderer.createApp({ setup: () => () => h(History, { surface: 'user', brandId: '', locale: 'en', initialGameId: selected.value, canManage: false, canConnect: false }) })
    app.mount(node('root')); await flush()
    expect(fetcher).toHaveBeenCalledTimes(1)
    expect(String(fetcher.mock.calls[0][0])).toContain('/platform-games/game-a?')
    selected.value = 'game-b'; await flush()
    expect(fetcher).toHaveBeenCalledTimes(2)
    expect(String(fetcher.mock.calls[1][0])).toContain('/platform-games/game-b?')
    app.unmount()
  })
  it('shows offline games only through the status filter and prevents deleting used games', async () => {
    const m = mount(2, 1); await flush()
    expect(text(m.root)).toContain('Platform 0'); expect(text(m.root)).not.toContain('Platform 1')
    expect(all(m.root, 'button').find(n => text(n) === '删除彩种')!.props.disabled).toBe(true)
    const filter = all(m.root, 'select')[0]!
    ;(filter.props.onChange as (event: unknown) => void)({target:{value:'offline'}}); await flush()
    expect(text(m.root)).not.toContain('Platform 0'); expect(text(m.root)).toContain('Platform 1')
    expect(all(m.root, 'button').some(n => text(n) === '重新上架')).toBe(true)
    m.app.unmount()
  })
  it('keeps refresh with the pagination, not the create action', async () => {
    const m = mount(1); await flush()
    const refresh = all(m.root, 'button').find(n => text(n) === '刷新')!
    expect(refresh.parent!.props.class).toContain('wallet-pagination')
    expect(all(m.root, 'th').some(n => text(n) === '时区')).toBe(false)
    m.app.unmount()
  })
  it('defaults to platform games, merges only the selected brand and dispatches each brand entry', async () => {
    const m = mount(1); await flush()
    expect(all(m.root, 'table')).toHaveLength(1)
    expect(text(m.root)).toContain('Platform 0')
    expect(all(m.root, 'h2')).toHaveLength(0)
    m.scope.value = { catalogBrandId: 'brand-a', brandName: 'Aurora', brandGames: [{ id: 'a', name: 'Brand A', code: 'a', timezone: 'UTC' }] }
    await flush()
    expect(all(m.root, 'table')).toHaveLength(1)
    expect(text(m.root)).toContain('Platform 0')
    expect(text(m.root)).toContain('Brand A')
    const brandRow = all(m.root, 'tr').find(n => text(n).includes('Brand A'))!
    for (const label of ['期次', '玩法规则', '号码模型']) click(brandRow, label)
    expect(m.events).toEqual([['a', 'periods'], ['a', 'plays'], ['a', 'model']])
    m.scope.value = { catalogBrandId: 'brand-b', brandName: 'Harbor', brandGames: [{ id: 'b', name: 'Brand B', code: 'b', timezone: 'UTC' }] }
    await flush(); expect(text(m.root)).not.toContain('Brand A'); expect(text(m.root)).toContain('Brand B')
    m.scope.value = { catalogBrandId: '', brandName: '', brandGames: [] }
    await flush(); expect(text(m.root)).toContain('Platform 0'); expect(text(m.root)).not.toContain('Brand B')
    expect(m.fetcher).toHaveBeenCalledTimes(1)
    m.app.unmount()
  })
  it('paginates one merged collection and resets its page when changing brand', async () => {
    const m = mount(50); await flush()
    expect(m.fetcher).toHaveBeenCalledTimes(2)
    m.scope.value = { catalogBrandId: 'brand-a', brandName: 'Aurora', brandGames: [{ id: 'a', name: 'Brand A', code: 'a', timezone: 'UTC' }] }
    await flush(); expect(all(m.root, 'tr')).toHaveLength(51)
    click(m.root, '下一页'); await flush()
    expect(text(m.root)).toContain('Brand A'); expect(text(m.root)).not.toContain('Platform 0')
    m.scope.value = { catalogBrandId: '', brandName: '', brandGames: [] }
    await flush(); expect(text(m.root)).toContain('Platform 0')
    expect(all(m.root, 'button').find(n => text(n) === '下一页')!.props.disabled).toBe(true)
    m.app.unmount()
  })
})

describe('visual number model editor', () => {
  it('shows only relevant fields and synchronizes M-select-N pools and counts', async () => {
    const root = node('root'), draft = ref<RuleModel>({ model: 'DIGITS_0_9', regular_pool: { allow_repeat: false }, special_pool: { allow_repeat: false }, regular_count: 0, special_count: 0, pool_size: 0, total_count: 0, length: 3, allow_repeat: true, ordered: true })
    const Editor = component('NumberModelEditor.vue')
    const app = renderer.createApp({ setup: () => () => h(Editor, { modelValue: draft.value, locale: 'zh-CN', 'onUpdate:modelValue': (model: RuleModel) => { draft.value = model } }) })
    app.mount(root); await flush()
    expect(text(root)).toContain('位数'); expect(text(root)).not.toContain('普通号码数量')
    const change = (type: string) => (all(root, 'select')[0].props.onChange as (event: unknown) => void)({ target: { value: type } })
    change('X_PLUS_Y'); await flush()
    expect(text(root)).not.toContain('位数'); expect(text(root)).toContain('普通号码池'); expect(text(root)).toContain('特别号码池')
    change('M_SELECT_N'); await flush()
    expect(draft.value.pool_size).toBe(49); expect(draft.value.total_count).toBe(7)
    expect(draft.value.special_pool).toEqual(draft.value.regular_pool)
    expect(text(root)).toContain('号码池'); expect(text(root)).not.toContain('特别号码池')
    expect(all(root, 'pre')).toHaveLength(0)
    app.unmount()
  })
})

describe('platform calendar language', () => {
  it('switches the open calendar language without changing the selected date or platform date format', async () => {
    vi.stubGlobal('window', { innerWidth: 1440, innerHeight: 900, addEventListener() {}, removeEventListener() {} })
    const root = node('root'), locale = ref<'en' | 'zh-CN'>('zh-CN'), value = ref('2026-10-15')
    const DateInput = component('PlatformDateInput.vue')
    const app = renderer.createApp({ setup: () => () => h(DateInput, { modelValue: value.value, locale: locale.value, label: 'date', displaySettings: { timezone: 'Asia/Singapore', date_format: 'DD/MM/YYYY', time_format: 'HH:mm:ss' }, 'onUpdate:modelValue': (date: string) => { value.value = date } }) })
    app.mount(root); await flush()
    const toggle = all(root, 'button').find(n => n.props['aria-label'] === 'date')!
    ;(toggle.props.onClick as () => void)(); await flush()
    expect(text(root)).toContain('2026年10月')
    expect(text(root)).toContain('周日')
    expect(text(root)).toContain('今天')
    expect(all(root, 'input')[0].props.value).toBe('15/10/2026')
    locale.value = 'en'; await flush()
    expect(text(root)).toContain('October 2026')
    expect(text(root)).toContain('Sun')
    expect(text(root)).toContain('Today')
    expect(text(root)).not.toContain('今天')
    expect(all(root, 'input')[0].props.value).toBe('15/10/2026')
    const day = all(root, 'button').find(n => n.props['aria-label'] === '2026-10-16')!
    ;(day.props.onClick as () => void)(); await flush()
    expect(value.value).toBe('2026-10-16')
    expect(all(root, 'input')[0].props.value).toBe('16/10/2026')
    app.unmount()
  })
})
