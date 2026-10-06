import { describe, expect, it, vi } from 'vitest'
import { installVisibleViewport } from './responsive-viewport'

function fixture(width: number | null, fallback = 360) {
  const viewport = width === null ? null : Object.assign(new EventTarget(), { width })
  const window = Object.assign(new EventTarget(), { visualViewport: viewport })
  const setProperty = vi.fn()
  const document = { documentElement: { clientWidth: fallback, style: { setProperty } } }
  const dispose = installVisibleViewport(window as unknown as Window, document as unknown as Document)
  return { window, viewport, document, setProperty, dispose }
}
describe('fixed navigation visible width', () => {
  it('uses the actual visible width rather than an expanded layout viewport', () => {
    const state = fixture(360, 373)
    expect(state.setProperty).toHaveBeenLastCalledWith('--admin-visible-width', '360px')
    state.viewport!.width = 768
    state.viewport!.dispatchEvent(new Event('resize'))
    expect(state.setProperty).toHaveBeenLastCalledWith('--admin-visible-width', '768px')
  })
  it('uses document width when VisualViewport is unavailable', () => {
    const state = fixture(null)
    expect(state.setProperty).toHaveBeenLastCalledWith('--admin-visible-width', '360px')
    state.document.documentElement.clientWidth = 1024
    state.window.dispatchEvent(new Event('resize'))
    expect(state.setProperty).toHaveBeenLastCalledWith('--admin-visible-width', '1024px')
  })
  it('does not install invalid zero or non-finite widths', () => {
    const state = fixture(NaN, 0)
    expect(state.setProperty).not.toHaveBeenCalled()
    state.document.documentElement.clientWidth = 360
    state.window.dispatchEvent(new Event('resize'))
    expect(state.setProperty).toHaveBeenLastCalledWith('--admin-visible-width', '360px')
  })
  it('removes both listeners on disposal for development hot reload', () => {
    const state = fixture(360)
    state.setProperty.mockClear()
    state.dispose()
    state.viewport!.dispatchEvent(new Event('resize'))
    state.window.dispatchEvent(new Event('resize'))
    expect(state.setProperty).not.toHaveBeenCalled()
  })
})
