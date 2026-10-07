import { readFileSync } from 'node:fs'
import { createRenderer, createSSRApp, nextTick, type Component } from 'vue'
import * as VueRuntime from 'vue'
import { compileScript, parse } from 'vue/compiler-sfc'
import ts from 'typescript'
import { renderToString } from 'vue/server-renderer'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { AdminAccount } from './admin-api'
import * as AdminApi from './admin-api'
import * as AdminI18n from './i18n'
import * as CorrectionApi from './correction-api'
import * as CorrectionState from './correction-state'
import type { Correction, CorrectionBody, CorrectionContext } from './correction-api'
import CorrectionManagement from './CorrectionManagement.vue'
import { adminI18nKey, createAdminI18n } from './i18n'

const brandId = '00000000-0000-4000-8000-000000000001'
const periodId = '00000000-0000-4000-8000-000000000002'
const account: AdminAccount = {
  id: '00000000-0000-4000-8000-000000000003',
  super_admin: false,
  brand_ids: [brandId],
  permissions: [],
  permissions_by_brand: { [brandId]: ['draw.view.brand', 'draw.correct.brand'] },
}

async function render(locale: 'zh-CN' | 'en') {
  const i18n = createAdminI18n()
  i18n.setLocale(locale)
  const app = createSSRApp(CorrectionManagement, { account, brandId, initialPeriodId: periodId })
  app.provide(adminI18nKey, i18n)
  return renderToString(app)
}

describe('settlement and correction localization', () => {
  it('renders translated correction controls while keeping the period identifier stable', async () => {
    const english = await render('en')
    const chinese = await render('zh-CN')

    expect(english).toContain('Draw result correction')
    expect(english).toContain('Draw corrections cannot be undone.')
    expect(english).toContain('Period UUID')
    expect(chinese).toContain('开奖结果更正')
    expect(chinese).toContain('开奖结果更正不可撤回。')
    expect(chinese).toContain('期次 UUID')
    expect(english.match(new RegExp(`value="${periodId}"`))).not.toBeNull()
    expect(chinese.match(new RegExp(`value="${periodId}"`))).not.toBeNull()
  })

  it('does not translate or mutate API identifiers or user supplied values', async () => {
    const english = await render('en')
    expect(english).toContain(periodId)
    expect(english).not.toContain('value="Draw result correction"')
    expect(account.permissions_by_brand?.[brandId]).toContain('draw.correct.brand')
  })
})

// Compile the actual client template and mount it with Vue's renderer, not a
// mocked setup function. This exercises v-model, review, writes, and locale updates.
type HostNode = {
  tag: string; props: Record<string, unknown>; children: HostNode[]; text: string;
  parent?: HostNode; value: string; checked: boolean;
  addEventListener: (...args: unknown[]) => void;
  removeEventListener: (...args: unknown[]) => void;
  getRootNode: () => HostNode;
}
function element(tag: string): HostNode {
  return {
    tag, props: {}, children: [], text: '', value: '', checked: false,
    addEventListener() {}, removeEventListener() {}, getRootNode() { return this },
  }
}
const renderer = createRenderer<HostNode, HostNode>({
  createElement: element,
  createText: text => ({ ...element('#text'), text }),
  createComment: text => ({ ...element('#comment'), text }),
  setText: (node, text) => { node.text = text },
  setElementText: (node, text) => { node.text = text; node.children = [] },
  patchProp: (node, key, _old, value) => {
    node.props[key] = value
    if (key === 'value') node.value = String(value ?? '')
  },
  insert: (node, parent, anchor) => {
    if (node.parent) {
      const index = node.parent.children.indexOf(node)
      if (index >= 0) node.parent.children.splice(index, 1)
    }
    node.parent = parent
    const index = anchor ? parent.children.indexOf(anchor) : -1
    if (index < 0) parent.children.push(node)
    else parent.children.splice(index, 0, node)
  },
  remove: node => {
    if (!node.parent) return
    const index = node.parent.children.indexOf(node)
    if (index >= 0) node.parent.children.splice(index, 1)
    node.parent = undefined
  },
  parentNode: node => node.parent ?? null,
  nextSibling: node => node.parent?.children[node.parent.children.indexOf(node) + 1] ?? null,
})
function compileCorrection(api: ReturnType<typeof CorrectionApi.createCorrectionApi>): Component {
  const source = readFileSync(new URL('./CorrectionManagement.vue', import.meta.url), 'utf8')
  const descriptor = parse(source, { filename: 'CorrectionManagement.vue' }).descriptor
  const compiled = compileScript(descriptor, { id: 'settlement-localization-client', inlineTemplate: true }).content
  const javascript = ts.transpileModule(compiled, {
    compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext },
  }).outputText
  const modules: Record<string, unknown> = {
    vue: VueRuntime, './admin-api': AdminApi, './i18n': AdminI18n,
    './correction-api': { ...CorrectionApi, createCorrectionApi: () => api },
    './correction-state': CorrectionState,
  }
  const body = javascript.replace(/^import\s+\{([\s\S]*?)\}\s+from\s+["']([^"']+)["'];?\s*$/gm,
    (_match, bindings: string, specifier: string) => {
      if (!(specifier in modules)) throw new Error(`Unmapped component import: ${specifier}`)
      return `const {${bindings.replace(/\s+as\s+/g, ': ')}} = __modules[${JSON.stringify(specifier)}];`
    }).replace(/export\s+default\s+/, 'return ')
  return new Function('__modules', body)(modules) as Component
}
function textOf(node: HostNode): string { return node.text + node.children.map(textOf).join('') }
function findNode(node: HostNode, match: (node: HostNode) => boolean): HostNode | undefined {
  if (match(node)) return node
  for (const child of node.children) { const found = findNode(child, match); if (found) return found }
}
async function flush() { for (let i = 0; i < 20; i++) await Promise.resolve(); await nextTick() }
async function update(root: HostNode, id: string, value: unknown) {
  const node = findNode(root, candidate => candidate.props.id === id)
  expect(node).toBeDefined()
  ;(node!.props['onUpdate:modelValue'] as (value: unknown) => void)(value)
  await flush()
}
async function click(root: HostNode, label: string) {
  const node = findNode(root, candidate => candidate.tag === 'button' && textOf(candidate) === label)
  expect(node).toBeDefined()
  expect(node!.props.disabled).toBeFalsy()
  ;(node!.props.onClick as () => void)()
  await flush()
}
afterEach(() => { CorrectionState.frozenCorrectionWrites.clear(); vi.unstubAllGlobals() })

describe('settlement and correction localization live messages', () => {
  it.each([
    { outcome: 'unknown', status: 503, zh: '写入结果未知。原请求体和幂等键仍保留', en: 'The write outcome is unknown. The original request body' },
    { outcome: 'conflict', status: 409, zh: '服务器报告版本冲突', en: 'The server reported a version conflict.' },
    { outcome: 'rejected', status: 403, zh: '服务器明确拒绝请求', en: 'The server rejected the request.' },
    { outcome: 'confirmed', status: 200, zh: '更正已确认，并已读取最新服务器状态。', en: 'Correction confirmed; the latest server state has been loaded.' },
    { outcome: 'mismatched', status: 200, zh: '收到的回执与原请求不匹配', en: 'The receipt does not match the original request.' },
  ])('updates $outcome notices in the same mounted page without changing the request', async ({ outcome, status, zh, en }) => {
    vi.stubGlobal('Document', class {})
    vi.stubGlobal('ShadowRoot', class {})
    CorrectionState.frozenCorrectionWrites.clear()
    const drawId = '00000000-0000-4000-8000-000000000004'
    const gameId = '00000000-0000-4000-8000-000000000005'
    const reason = '用户原因 {name} / 原样保留'
    const externalError = '服务端业务错误原文 {name}'
    const context: CorrectionContext = {
      brand_id: brandId, game_id: gameId, period_id: periodId, period_version: 9,
      period_status: 'drawn', draw_result_id: drawId,
      draw: { regular: [1, 4], special: [7], digits: [] },
      model: { model: 'X_PLUS_Y', ordered: false }, current_job_id: null,
      current_job_version: null, policy_version: 3, mode: null,
      requires_resettlement: false, can_correct: true,
    }
    let receipt: Correction
    const create = vi.fn(async (_brand: string, _draw: string, body: CorrectionBody) => {
      if (status !== 200) throw new AdminApi.AdminApiError(externalError, status)
      receipt = {
        id: '00000000-0000-4000-8000-000000000006', brand_id: brandId,
        game_id: gameId, period_id: periodId, previous_draw_result_id: drawId,
        draw_result_id: '00000000-0000-4000-8000-000000000007', result: body.result,
        period_version: body.version + 1, previous_job_id: null, new_job_id: null,
        policy_version: null, mode: null, state: 'completed', version: 1,
        target_count: 0, created_by: account.id, reason: outcome === 'mismatched' ? '不同原因' : body.reason,
        created_at: '2026-10-08T00:00:00Z', completed_at: '2026-10-08T00:00:00Z', last_error_code: null,
        pending_count: 0, reversed_count: 0, unchanged_count: 0, excluded_count: 0,
        failed_count: 0, reverse_points: '0', reversed_points: '0', can_retry: false,
        new_job_state: null, new_job_version: null, new_job_error_code: null,
      }
      return receipt
    })
    const contextRead = vi.fn(async () => context)
    const api: ReturnType<typeof CorrectionApi.createCorrectionApi> = {
      ...CorrectionApi.createCorrectionApi(), context: contextRead, create,
      history: vi.fn(async () => ({ brand_id: brandId, period_id: periodId, items: [], limit: 20, offset: 0, has_more: false })),
      detail: vi.fn(async () => receipt),
      targets: vi.fn(async () => ({ brand_id: brandId, correction_id: receipt.id, items: [], limit: 20, offset: 0, has_more: false })),
    }
    const i18n = createAdminI18n()
    const root = element('root')
    const app = renderer.createApp(compileCorrection(api), { account, brandId, initialPeriodId: periodId })
    app.provide(adminI18nKey, i18n)
    app.mount(root)
    try {
      await flush()
      await update(root, 'correction-regular', '5, 2')
      await update(root, 'correction-reason', reason)
      await click(root, '核对并只更正未结算结果')
      await update(root, 'correction-confirmed', true)
      i18n.setLocale('en')
      await flush()
      expect(textOf(root)).toContain('Confirm this irreversible action')
      expect(textOf(root)).toContain(reason)
      expect(create).not.toHaveBeenCalled()
      await click(root, 'Confirm submission')
      expect(create).toHaveBeenCalledTimes(1)
      const request = create.mock.calls[0] as unknown as Parameters<typeof api.create>
      const serialized = JSON.stringify(request)
      expect(request[2]).toEqual({ version: 9, policy_version: null, result: { regular: [2, 5], special: [7], digits: [] }, reason })
      expect(request[3]).toBeTruthy()
      expect(textOf(root)).toContain(en)
      const frozen = CorrectionState.frozenCorrectionWrites.find(account.id, brandId, periodId)
      const retained = outcome === 'unknown' || outcome === 'mismatched'
      expect(Boolean(frozen)).toBe(retained)
      for (const locale of ['zh-CN', 'en', 'zh-CN'] as const) {
        i18n.setLocale(locale)
        await flush()
        expect(textOf(root)).toContain(locale === 'en' ? en : zh)
        expect(textOf(root)).not.toContain(locale === 'en' ? zh : en)
        if (status !== 200) expect(textOf(root)).toContain(externalError)
        expect(findNode(root, node => node.props.id === 'correction-reason')?.value).toBe(reason)
        expect(findNode(root, node => node.props.id === 'correction-regular')?.value).toBe('5, 2')
        expect(JSON.stringify(create.mock.calls[0])).toBe(serialized)
        expect(CorrectionState.frozenCorrectionWrites.find(account.id, brandId, periodId)).toBe(frozen)
        expect(create).toHaveBeenCalledTimes(1)
        expect(contextRead).toHaveBeenCalledTimes(1)
      }
      if (retained) {
        await click(root, '原样重试同一请求')
        expect(create).toHaveBeenCalledTimes(2)
        expect(JSON.stringify(create.mock.calls[1])).toBe(serialized)
      }
    } finally { app.unmount() }
  })
})
