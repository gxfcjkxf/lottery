import { afterEach, expect, it, vi } from 'vitest'
import { globalLoadingActive, globalLoadingMessage, vGlobalLoading, withGlobalLoading } from './global-loading'

class StatusElement { hidden = false; dataset: Record<string, string> = {}; constructor(public textContent: string) {} }

it('does not treat an untracked background request as a foreground operation', async () => {
  let complete!: () => void
  const background = new Promise<void>(resolve => { complete = resolve })
  expect(globalLoadingActive.value).toBe(false)
  complete(); await background
  expect(globalLoadingActive.value).toBe(false)
})
it('tracks the entire foreground action, including work after the response, with independent concurrent requests', async () => {
  let response!: () => void, decode!: () => void, save!: () => void
  const headers = new Promise<void>(resolve => { response = resolve })
  const body = new Promise<void>(resolve => { decode = resolve })
  const write = new Promise<void>(resolve => { save = resolve })
  const first = withGlobalLoading(async () => { await headers; await body; return 'loaded' })
  const second = withGlobalLoading(() => write)
  expect(globalLoadingActive.value).toBe(true)
  response(); await Promise.resolve()
  expect(globalLoadingActive.value).toBe(true)
  save(); await second
  expect(globalLoadingActive.value).toBe(true)
  decode(); expect(await first).toBe('loaded')
  expect(globalLoadingActive.value).toBe(false)
})
it('preserves arguments, exact errors and unknown write results and always releases progress', async () => {
  const unknown = new Error('unknown write outcome')
  const action = vi.fn(async (key: string, body: object) => { expect(key).toBe('unchanged-key'); expect(body).toBe(input); throw unknown })
  const input = { version: '4' }
  await expect(withGlobalLoading(action, 'unchanged-key', input)).rejects.toBe(unknown)
  expect(action).toHaveBeenCalledOnce()
  expect(globalLoadingActive.value).toBe(false)
  await expect(withGlobalLoading(() => { throw unknown })).rejects.toBe(unknown)
  expect(globalLoadingActive.value).toBe(false)
})
const elements: StatusElement[] = []
function hook(name: 'mounted' | 'updated' | 'beforeUnmount', element: StatusElement, active?: boolean) {
  const callback = (vGlobalLoading as Record<string, unknown>)[name] as (...args: unknown[]) => void
  callback(element, { value: active })
}
afterEach(() => { elements.splice(0).forEach(el => hook('beforeUnmount', el)); vi.unstubAllGlobals() })
function mount(label: string) {
  vi.stubGlobal('HTMLElement', StatusElement)
  const el = new StatusElement(label); elements.push(el); hook('mounted', el); return el
}
it('uses one global label and hides only the local progress placeholder', () => {
  const first = mount('Reading profile…'), second = mount('Refreshing messages…')
  expect(first.hidden).toBe(true); expect(second.hidden).toBe(true)
  expect(globalLoadingMessage.value).toBe('Refreshing messages…')
  hook('beforeUnmount', second); expect(globalLoadingMessage.value).toBe('Reading profile…')
  hook('beforeUnmount', first); expect(globalLoadingMessage.value).toBe('')
})
it('updates the displayed language and restores an inactive non-loading state', () => {
  const el = mount('Loading…'); el.textContent = '正在读取资料…'; hook('updated', el)
  expect(globalLoadingMessage.value).toBe('正在读取资料…')
  hook('updated', el, false); expect(el.hidden).toBe(false); expect(globalLoadingMessage.value).toBe('')
})
