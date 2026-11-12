// Keep spaces within names, messages and other multi-word values. Also remove
// copied zero-width spaces and word joiners, which String.trim() leaves behind.
const edgeWhitespace = /^[\s\p{Cc}\u200b\u2060]+|[\s\p{Cc}\u200b\u2060]+$/gu
const textInputTypes = new Set(['text', 'search', 'tel', 'url', 'email', 'password'])

function isTextControl(target: EventTarget | null): target is HTMLInputElement | HTMLTextAreaElement {
  return target instanceof HTMLTextAreaElement
    || target instanceof HTMLInputElement && textInputTypes.has(target.type)
}

function normalize(control: HTMLInputElement | HTMLTextAreaElement): boolean {
  if (control.disabled || control.readOnly) return false
  const value = control.value.replace(edgeWhitespace, '')
  if (value === control.value) return false
  control.value = value
  return true
}

/** Install before mounting Vue, so values are cleaned before local validation. */
export function installInputNormalization(document: Document): () => void {
  function commit(event: Event) {
    if (!isTextControl(event.target) || !normalize(event.target)) return
    // Update both regular and lazy v-model bindings before validation/handlers.
    event.target.dispatchEvent(new Event('input', { bubbles: true }))
    if (event.type !== 'change') event.target.dispatchEvent(new Event('change', { bubbles: true }))
  }
  function pasteInput(event: Event) {
    if (event instanceof InputEvent && event.inputType === 'insertFromPaste'
      && !event.isComposing && isTextControl(event.target)) normalize(event.target)
  }
  function submit(event: Event) {
    if (!(event.target instanceof HTMLFormElement)) return
    for (const control of Array.from(event.target.elements)) {
      if (!isTextControl(control) || !normalize(control)) continue
      control.dispatchEvent(new Event('input', { bubbles: true }))
      control.dispatchEvent(new Event('change', { bubbles: true }))
    }
  }
  function enter(event: KeyboardEvent) {
    if (event.key === 'Enter' && !event.isComposing && event.target instanceof HTMLInputElement) commit(event)
  }
  document.addEventListener('input', pasteInput, true)
  document.addEventListener('change', commit, true)
  document.addEventListener('blur', commit, true)
  document.addEventListener('submit', submit, true)
  document.addEventListener('keydown', enter, true)
  return () => {
    document.removeEventListener('input', pasteInput, true)
    document.removeEventListener('change', commit, true)
    document.removeEventListener('blur', commit, true)
    document.removeEventListener('submit', submit, true)
    document.removeEventListener('keydown', enter, true)
  }
}
