import { securityIntents } from './security-settings-api'
import { readFileSync } from 'node:fs'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createRenderer, nextTick, type Component } from 'vue'
import * as Vue from 'vue'
import { compileScript, parse } from 'vue/compiler-sfc'
import ts from 'typescript'
import { PlatformApiError } from './platform-api'
import { useSessionReplacement } from '../../shared/src/session-replacement'

const key = 'lottery.platform.selectedBrand'
const storage = new Map<string, string>()
const api = {
  presentation: vi.fn(async () => ({ display_name: 'Trial', logo_text: 'T', logo_url: '' })),
  me: vi.fn(async () => ({ id: 'account-a', platform_permissions: [] })),
  platformSettings: vi.fn(async () => ({})),
  brands: vi.fn(async () => [{ id: 'brand-a' }, { id: 'brand-b' }]),
  users: vi.fn(async () => ({ items: [], limit: 50, offset: 0, total_count: '0' })), audit: vi.fn(async () => []),
  login: vi.fn(async () => {}), logout: vi.fn(async () => {}),
}
const renderer = createRenderer({
  createElement: () => ({}), createText: () => ({}), createComment: () => ({}),
  setText() {}, setElementText() {}, patchProp() {}, insert() {}, remove() {},
  parentNode: () => null, nextSibling: () => null,
})
const apps: Array<ReturnType<typeof renderer.createApp>> = []

async function mount() {
  vi.stubGlobal('window', new EventTarget())
  vi.stubGlobal('document', { title: '' })
  vi.stubGlobal('sessionStorage', {
    getItem: (name: string) => storage.get(name) ?? null,
    setItem: (name: string, value: string) => storage.set(name, value),
    removeItem: (name: string) => storage.delete(name),
  })
  const descriptor = parse(readFileSync(new URL('./App.vue', import.meta.url), 'utf8')).descriptor
  const compiled = compileScript(descriptor, { id: 'brand-session-test' }).content
  const modules: Record<string, unknown> = {
    vue: Vue,
    '@lottery/shared': {
      defaultBrand: {primary: '#059669'},
      applyBrowserBranding: (name: string) => {document.title=name},
      withGlobalLoading: (fn: () => unknown) => Promise.resolve(fn()),
      platformDisplaySettings: Vue.ref({}), setPlatformDisplaySettings() {},
      useSessionReplacement,
    },
    './security-settings-api': { securityIntents },
    './platform-api': { createPlatformApi: () => api, PlatformApiError },
    './use-brand-games': { useBrandGames: () => ({ games: Vue.ref([]), selectedGameId: Vue.ref(''), loading: Vue.ref(false), refresh() {} }) },
    './platform-error-message': { platformErrorMessage: String },
    './i18n': { usePlatformI18n: () => ({ locale: Vue.ref('zh-CN'), setLocale() {}, t: (value: string) => value }) },
  }
  const source = ts.transpileModule(compiled, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText
    .replace(/^import\s+\{([^}]+)\}\s+from\s+['"]([^'"]+)['"];?$/gm,
      (_match, bindings: string, path: string) => `const { ${bindings.replace(/\s+as\s+/g, ': ')} } = modules[${JSON.stringify(path)}] ?? {};`)
    .replace(/^import\s+(\w+)\s+from\s+['"][^'"]+['"];?$/gm, 'const $1 = {};')
    .replace('export default', 'return')
  const component = new Function('modules', source)(modules) as Component
  const app = renderer.createApp({ ...component, render: () => null })
  apps.push(app)
  app.mount({})
  await nextTick()
  await new Promise(resolve => setTimeout(resolve, 0))
  return (app._instance as unknown as { setupState: Record<string, any> }).setupState
}

afterEach(() => {
  apps.splice(0).forEach(app => app.unmount())
  storage.clear()
  vi.clearAllMocks()
  vi.unstubAllGlobals()
})

describe('platform brand selection during a login', () => {
  it.each(['AUTH_SESSION_REVOKED', 'AUTH_SESSION_REPLACED'])('opens a clean login page for an old %s cookie', async code => {
    api.me.mockRejectedValueOnce(new PlatformApiError('Old session', 401, code))
    const state = await mount()
    expect(state.authReady).toBe(true)
    expect(state.platformPresentation).not.toBeNull()
    expect(state.account).toBeNull()
    expect(state.error).toBe('')
    expect(state.sessionReplaced).toBe(false)
    expect(api.platformSettings).not.toHaveBeenCalled()
    expect(api.brands).not.toHaveBeenCalled()
    await state.login()
    expect(state.account.id).toBe('account-a')
    expect(state.error).toBe('')
  })

  it('keeps initialization service failures visible', async () => {
    const failure = new PlatformApiError('Unavailable', 503, 'SERVICE_UNAVAILABLE')
    api.me.mockRejectedValueOnce(failure)
    const state = await mount()
    expect(state.authReady).toBe(true)
    expect(state.account).toBeNull()
    expect(state.errorCause).toBe(failure)
  })

  it('still reports replacement of an active session', async () => {
    const state = await mount()
    api.me.mockImplementationOnce(async () => { throw new PlatformApiError('Replaced session', 401, 'AUTH_SESSION_REPLACED') })
    window.dispatchEvent(new Event('focus'))
    await nextTick()
    expect(state.account).toBeNull()
    expect(state.sessionReplaced).toBe(true)
  })

  it('opens the overview by default and returns there after logout', async () => {
    const state = await mount()
    expect(state.section).toBe('home')
    expect(state.showBrandPicker).toBe(false)
    await state.chooseSection('brands')
    await state.signOut()
    expect(state.section).toBe('home')
  })
  it('reuses the latest brand across functions and page reloads', async () => {
    const state = await mount()
    await state.chooseBrand('brand-a')
    for (const section of ['users', 'rewards', 'wallets', 'bets', 'games', 'draws', 'withdrawals', 'commission', 'brands']) {
      await state.chooseSection(section)
      expect(state.selectedBrand).toBe('brand-a')
    }
    await state.chooseBrandAccounts()
    expect(state.selectedBrand).toBe('brand-a')
    await state.chooseAuditView('audit')
    expect(api.audit).toHaveBeenLastCalledWith('brand-a', 51, 0)
    await state.chooseBrand('brand-b')
    const reloaded = await mount()
    expect(reloaded.selectedBrand).toBe('brand-b')
    await reloaded.chooseSection('users')
    expect(api.users).toHaveBeenLastCalledWith('brand-b', 50, 0, '', { query: '', online: '' })
  })

  it('clears a deselected brand and logout or expired login state', async () => {
    const state = await mount()
    await state.chooseBrand('brand-a')
    await state.chooseBrand('')
    expect(storage.has(key)).toBe(false)
    await state.chooseBrand('brand-b')
    await state.signOut()
    expect(storage.has(key)).toBe(false)
    expect(state.selectedBrand).toBe('')
    const loggedIn = await mount()
    await loggedIn.chooseBrand('brand-a')
    loggedIn.handleFailure(new PlatformApiError('Expired', 401, 'UNAUTHORIZED'))
    expect(storage.has(key)).toBe(false)
    expect(loggedIn.selectedBrand).toBe('')
  })

  it('does not restore a different account or a deleted brand', async () => {
    for (const saved of [{ accountId: 'other-account', brandId: 'brand-a' }, { accountId: 'account-a', brandId: 'deleted-brand' }]) {
      storage.set(key, JSON.stringify(saved))
      const state = await mount()
      expect(state.selectedBrand).toBe('')
      expect(storage.has(key)).toBe(false)
    }
  })

  it('starts a new explicit login without the previous brand', async () => {
    const state = await mount()
    await state.chooseBrand('brand-a')
    await state.login()
    expect(state.selectedBrand).toBe('')
    expect(storage.has(key)).toBe(false)
  })
})
