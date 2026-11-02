import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createRenderer, defineComponent, ref } from 'vue'
import type { AuthChallenge } from './auth'
import { useCaptchaRefresh } from './captcha-refresh'

const renderer = createRenderer({
  patchProp() {}, insert() {}, remove() {},
  createElement: () => ({}), createText: () => ({}), createComment: () => ({}),
  setText() {}, setElementText() {}, parentNode: () => null, nextSibling: () => null,
})
let unmount: () => void
function mount() {
  const challenge = ref<AuthChallenge | null>({ id: 'first', expires_at: new Date(Date.now() + 300_000).toISOString() } as AuthChallenge)
  const paused = ref(false)
  const refresh = vi.fn(async () => { challenge.value = null })
  const app = renderer.createApp(defineComponent({
    setup() { useCaptchaRefresh(() => challenge.value, refresh, () => paused.value); return () => null },
  }))
  app.mount({})
  unmount = () => app.unmount()
  return { challenge, paused, refresh }
}
beforeEach(() => {
  vi.useFakeTimers()
  vi.stubGlobal('window', new EventTarget())
  vi.stubGlobal('document', Object.assign(new EventTarget(), { hidden: false }))
})
afterEach(() => { unmount(); vi.useRealTimers(); vi.unstubAllGlobals() })

describe('captcha expiry refresh', () => {
  it('refreshes 30 seconds before expiry, then uses the new challenge expiry', async () => {
    const view = mount()
    await vi.advanceTimersByTimeAsync(269_000)
    expect(view.refresh).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(1000)
    expect(view.refresh).toHaveBeenCalledOnce()
    view.challenge.value = { id: 'second', expires_at: new Date(Date.now() + 300_000).toISOString() } as AuthChallenge
    await vi.advanceTimersByTimeAsync(269_000)
    expect(view.refresh).toHaveBeenCalledOnce()
    await vi.advanceTimersByTimeAsync(1000)
    expect(view.refresh).toHaveBeenCalledTimes(2)
  })

  it('checks immediately when returning from a hidden or suspended page', async () => {
    const view = mount()
    Object.assign(document, { hidden: true })
    await vi.advanceTimersByTimeAsync(301_000)
    expect(view.refresh).not.toHaveBeenCalled()
    Object.assign(document, { hidden: false })
    document.dispatchEvent(new Event('visibilitychange'))
    expect(view.refresh).toHaveBeenCalledOnce()
    view.challenge.value = { id: 'third', expires_at: new Date(Date.now() + 300_000).toISOString() } as AuthChallenge
    vi.setSystemTime(Date.now() + 301_000)
    window.dispatchEvent(new Event('focus'))
    expect(view.refresh).toHaveBeenCalledTimes(2)
  })

  it('pauses during submission and stops after a failed refresh or unmount', async () => {
    const view = mount()
    view.paused.value = true
    await vi.advanceTimersByTimeAsync(301_000)
    expect(view.refresh).not.toHaveBeenCalled()
    view.paused.value = false
    await vi.advanceTimersByTimeAsync(1000)
    expect(view.refresh).toHaveBeenCalledOnce()
    await vi.advanceTimersByTimeAsync(300_000)
    expect(view.refresh).toHaveBeenCalledOnce()
    view.challenge.value = { id: 'last', expires_at: new Date(Date.now()).toISOString() } as AuthChallenge
    unmount()
    await vi.advanceTimersByTimeAsync(1000)
    window.dispatchEvent(new Event('focus'))
    document.dispatchEvent(new Event('visibilitychange'))
    expect(view.refresh).toHaveBeenCalledOnce()
    unmount = () => {}
  })
})
