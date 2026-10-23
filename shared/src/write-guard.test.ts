import { afterEach, expect, it, vi } from 'vitest'
import { createWriteGate, WRITE_GUARD_MS } from './write-guard'

afterEach(() => vi.useRealTimers())

it('locks immediately and blocks click/Enter repeats for at least one second after a fast request', () => {
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'performance'] })
  const changed = vi.fn(), gate = createWriteGate(changed)
  expect(gate.enter()).toBe(true)
  gate.updateBusy(true)
  gate.updateBusy(false)
  expect(gate.enter()).toBe(false)
  vi.advanceTimersByTime(WRITE_GUARD_MS - 1)
  expect(gate.enter()).toBe(false)
  vi.advanceTimersByTime(1)
  expect(changed).toHaveBeenLastCalledWith(false)
  expect(gate.enter()).toBe(true)
  gate.dispose()
})

it('holds until the full operation finishes even when it takes more than one second', () => {
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'performance'] })
  const changed = vi.fn(), gate = createWriteGate(changed)
  gate.enter(); gate.updateBusy(true)
  vi.advanceTimersByTime(3000)
  expect(gate.enter()).toBe(false)
  expect(changed).toHaveBeenLastCalledWith(true)
  gate.updateBusy(false)
  expect(changed).toHaveBeenLastCalledWith(false)
  expect(gate.enter()).toBe(true)
  gate.dispose()
})

it('also guards an operation initiated without a DOM event and cancels timers on unmount', () => {
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'performance'] })
  const changed = vi.fn(), gate = createWriteGate(changed)
  gate.updateBusy(true); gate.updateBusy(false)
  expect(gate.enter()).toBe(false)
  gate.dispose()
  changed.mockClear(); vi.advanceTimersByTime(1000)
  expect(changed).not.toHaveBeenCalled()
})
