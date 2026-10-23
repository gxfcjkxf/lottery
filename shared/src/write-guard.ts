import type { Directive } from 'vue'

export const WRITE_GUARD_MS = 1000

/** A UI submission gate. The server remains responsible for idempotency and money. */
export function createWriteGate(onChange: (locked: boolean) => void) {
  let locked = false, busy = false, until = 0
  let timer: ReturnType<typeof setTimeout> | undefined
  function sync() {
    clearTimeout(timer)
    timer = undefined
    const remaining = until - performance.now()
    locked = busy || remaining > 0
    onChange(locked)
    if (remaining > 0) timer = setTimeout(sync, remaining)
  }
  return {
    get locked() { return locked },
    enter() {
      if (locked) return false
      until = performance.now() + WRITE_GUARD_MS
      sync()
      return true
    },
    updateBusy(value: boolean) {
      if (value && !busy && !locked) until = performance.now() + WRITE_GUARD_MS
      busy = value
      sync()
    },
    dispose() { clearTimeout(timer) },
  }
}

type Guard = ReturnType<typeof createWriteGate>
const guards = new WeakMap<HTMLElement, { gate: Guard; listener: EventListener; event: string; ariaDisabled: string | null }>()

/** Bind to the complete operation's busy flag, not just the HTTP response headers.
 * Forms capture submit (including Enter/requestSubmit); standalone buttons capture click.
 * Capture blocking and aria-disabled remain active even when Vue clears :disabled early.
 */
export const vWriteGuard: Directive<HTMLElement, boolean> = {
  mounted(element, binding) {
    if (typeof HTMLElement === 'undefined' || !(element instanceof HTMLElement)) return
    const ariaDisabled = element.getAttribute('aria-disabled')
    const gate = createWriteGate(locked => {
      if (locked) {
        element.dataset.writeLocked = 'true'
        element.setAttribute('aria-disabled', 'true')
      } else {
        delete element.dataset.writeLocked
        if (ariaDisabled === null) element.removeAttribute('aria-disabled')
        else element.setAttribute('aria-disabled', ariaDisabled)
      }
    })
    const listener: EventListener = event => {
      // A retry button can replace submit inside the same form after a fast failure.
      // Keep that replacement behind the original form's minimum interval as well.
      const blocked = element.tagName === 'FORM' && event.type === 'click' ? gate.locked : !gate.enter()
      if (blocked) { event.preventDefault(); event.stopImmediatePropagation() }
    }
    const event = element.tagName === 'FORM' ? 'submit' : element.tagName === 'INPUT' ? 'change' : 'click'
    guards.set(element, { gate, listener, event, ariaDisabled })
    element.addEventListener(event, listener, true)
    if (element.tagName === 'FORM') element.addEventListener('click', listener, true)
    gate.updateBusy(Boolean(binding.value))
  },
  updated(element, binding) { guards.get(element)?.gate.updateBusy(Boolean(binding.value)) },
  beforeUnmount(element) {
    const guard = guards.get(element)
    if (!guard) return
    guard.gate.dispose()
    element.removeEventListener(guard.event, guard.listener, true)
    if (element.tagName === 'FORM') element.removeEventListener('click', guard.listener, true)
    if (guard.ariaDisabled === null) element.removeAttribute('aria-disabled')
    else element.setAttribute('aria-disabled', guard.ariaDisabled)
    delete element.dataset.writeLocked
    guards.delete(element)
  },
}
