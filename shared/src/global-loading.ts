import { computed, ref, shallowReactive, type Directive } from 'vue'

const foregroundOperations = ref(0)
export const globalLoadingActive = computed(() => foregroundOperations.value > 0)

/** Track the complete user operation, including response decoding and state updates.
 * Background timers/focus checks call their readers directly, never this helper.
 */
export async function withGlobalLoading<Args extends unknown[], T>(operation: (...args: Args) => T, ...args: Args): Promise<Awaited<T>> {
  foregroundOperations.value++
  try { return await operation(...args) }
  finally { foregroundOperations.value-- }
}

const messages = shallowReactive(new Map<HTMLElement, string>())
export const globalLoadingMessage = computed(() => [...messages.values()].at(-1) ?? '')

/** Route page-local read statuses to the single application loading indicator. */
function sync(element: HTMLElement, active: boolean) {
  if (typeof HTMLElement === 'undefined' || !(element instanceof HTMLElement)) return
  element.hidden = active
  element.dataset.globalLoading = String(active)
  if (!active) { messages.delete(element); return }
  const label = element.textContent?.trim()
  if (label) messages.set(element, label)
}
export const vGlobalLoading: Directive<HTMLElement, boolean | undefined> = {
  mounted: (element, binding) => sync(element, binding.value !== false),
  updated: (element, binding) => sync(element, binding.value !== false),
  beforeUnmount: element => { messages.delete(element) },
  getSSRProps: binding => binding.value === false ? {} : { hidden: true, 'data-global-loading': 'true' },
}
