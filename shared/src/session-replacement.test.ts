import { afterEach, describe, expect, it, vi } from 'vitest'
import { createRenderer, defineComponent, nextTick, ref, type Ref } from 'vue'
import { notifySessionReplaced, useSessionReplacement } from './session-replacement'
import { createApiClient } from './api'

const renderer = createRenderer({
  patchProp() {}, insert() {}, remove() {},
  createElement: () => ({}), createText: () => ({}), createComment: () => ({}),
  setText() {}, setElementText() {}, parentNode: () => null, nextSibling: () => null,
})
let unmount: (() => void) | undefined
function mount(check: () => Promise<unknown>) {
  const active = ref(true)
  const clear = vi.fn(() => { active.value = false })
  let replaced!: Ref<boolean>
  const app = renderer.createApp(defineComponent({
    setup() { replaced = useSessionReplacement(() => active.value, check, clear); return () => null },
  }))
  app.mount({})
  unmount = () => app.unmount()
  return { active, clear, replaced }
}
afterEach(() => { unmount?.(); unmount = undefined; vi.useRealTimers(); vi.unstubAllGlobals() })

describe('single-session UI', () => {
  it('clears private state on a replacement response and resets the notice after a new login', async () => {
    vi.stubGlobal('window', new EventTarget())
    const client = createApiClient({ fetcher: vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify({
      success: false, error: { code: 'AUTH_SESSION_REPLACED', message: 'server text' },
    }), { status: 401 })) })
    const view = mount(async () => { throw { code: 'AUTH_SESSION_REPLACED' } })
    await expect(client.request('/api/v1/me')).rejects.toMatchObject({ status: 401, code: 'AUTH_SESSION_REPLACED' })
    await nextTick()
    expect(view.clear).toHaveBeenCalledOnce()
    expect(view.active.value).toBe(false)
    expect(view.replaced.value).toBe(true)
    view.active.value = true
    await nextTick()
    expect(view.replaced.value).toBe(false)
    notifySessionReplaced('AUTH_SESSION_REVOKED')
    expect(view.active.value).toBe(true)
  })

  it('checks idle sessions every 15 seconds and on focus, and removes checks on unmount', async () => {
    vi.useFakeTimers()
    vi.stubGlobal('window', new EventTarget())
    const check = vi.fn(async () => {})
    const view = mount(check)
    await vi.advanceTimersByTimeAsync(15000)
    expect(check).toHaveBeenCalledOnce()
    window.dispatchEvent(new Event('focus'))
    await nextTick()
    expect(check).toHaveBeenCalledTimes(2)
    unmount!(); unmount = undefined
    await vi.advanceTimersByTimeAsync(15000)
    window.dispatchEvent(new Event('focus'))
    expect(check).toHaveBeenCalledTimes(2)
    expect(view.clear).not.toHaveBeenCalled()
  })

  it('does not report replacement for network failures; detects it on the next successful check', async () => {
    vi.useFakeTimers()
    vi.stubGlobal('window', new EventTarget())
    const check = vi.fn().mockRejectedValueOnce(new Error('offline')).mockRejectedValueOnce({ code: 'AUTH_SESSION_REPLACED' })
    const view = mount(check)
    await vi.advanceTimersByTimeAsync(15000)
    expect(view.active.value).toBe(true)
    await vi.advanceTimersByTimeAsync(15000)
    expect(view.replaced.value).toBe(true)
    expect(view.active.value).toBe(false)
    await vi.advanceTimersByTimeAsync(15000)
    expect(check).toHaveBeenCalledTimes(2)
  })
  it('ignores an old heartbeat failure after a new session has become active', async () => {
    vi.stubGlobal('window', new EventTarget())
    let rejectOld!: (reason: unknown) => void
    const check = vi.fn(() => new Promise((_, reject) => { rejectOld = reject }))
    const view = mount(check)
    window.dispatchEvent(new Event('focus'))
    view.active.value = false
    await nextTick()
    view.active.value = true
    await nextTick()
    rejectOld({ code: 'AUTH_SESSION_REPLACED' })
    await nextTick()
    expect(view.active.value).toBe(true)
    expect(view.replaced.value).toBe(false)
    expect(view.clear).not.toHaveBeenCalled()
  })

  it('rechecks the current session before treating a delayed request error as replacement', async () => {
    vi.stubGlobal('window', new EventTarget())
    const check = vi.fn(async () => {})
    const view = mount(check)
    notifySessionReplaced('AUTH_SESSION_REPLACED')
    await nextTick()
    expect(check).toHaveBeenCalledOnce()
    expect(view.active.value).toBe(true)
    expect(view.clear).not.toHaveBeenCalled()
  })

  it('ignores delayed heartbeat failure after unmount', async () => {
    vi.stubGlobal('window', new EventTarget())
    let rejectOld!: (reason: unknown) => void
    const view = mount(() => new Promise((_, reject) => { rejectOld = reject }))
    window.dispatchEvent(new Event('focus'))
    unmount!(); unmount = undefined
    rejectOld({ code: 'AUTH_SESSION_REPLACED' })
    await nextTick()
    expect(view.clear).not.toHaveBeenCalled()
  })

})
