import { onMounted, onUnmounted, ref, watch } from 'vue'

const eventName = 'lottery:session-replaced'
export function notifySessionReplaced(code: string | undefined) {
  if (code === 'AUTH_SESSION_REPLACED' && typeof window !== 'undefined') {
    window.dispatchEvent(new Event(eventName))
  }
}

/** Recheck the current session: an earlier request may belong to a replaced login. */
export function useSessionReplacement(active: () => boolean, check: () => Promise<unknown>, clear: () => void) {
  const replaced = ref(false)
  let timer: ReturnType<typeof setInterval> | undefined
  let checking = false
  let generation = 0
  let disposed = false
  const invalidate = () => { replaced.value = true; clear() }
  const refresh = async () => {
    if (!active() || checking || disposed) return
    const startedGeneration = generation
    checking = true
    try { await check() } catch (cause) {
      if (!disposed && active() && startedGeneration === generation && cause && typeof cause === 'object' && 'code' in cause && cause.code === 'AUTH_SESSION_REPLACED') invalidate()
      // Network/other errors belong to the normal request UI; they do not prove replacement.
    } finally { checking = false }
  }
  watch(active, value => { generation++; if (value) replaced.value = false }, { flush: 'sync' })
  onMounted(() => {
    window.addEventListener(eventName, refresh)
    window.addEventListener('focus', refresh)
    timer = setInterval(refresh, 15000)
  })
  onUnmounted(() => {
    disposed = true
    generation++
    window.removeEventListener(eventName, refresh)
    window.removeEventListener('focus', refresh)
    clearInterval(timer)
  })
  return replaced
}
