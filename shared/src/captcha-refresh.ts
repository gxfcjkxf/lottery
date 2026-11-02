import { onMounted, onUnmounted, ref } from 'vue'
import type { AuthChallenge } from './auth'

// Refresh before expiry; hidden tabs check again as soon as they become active.
export function useCaptchaRefresh(
  challenge: () => AuthChallenge | null,
  refresh: () => Promise<void>,
  paused: () => boolean,
) {
  const now = ref(Date.now())
  let timer: ReturnType<typeof setInterval>
  function check() {
    now.value = Date.now()
    const current = challenge()
    if (!document.hidden && !paused() && current && Date.parse(current.expires_at) - now.value <= 30_000) {
      void refresh()
    }
  }
  onMounted(() => {
    timer = setInterval(check, 1000)
    document.addEventListener('visibilitychange', check)
    window.addEventListener('focus', check)
  })
  onUnmounted(() => {
    clearInterval(timer)
    document.removeEventListener('visibilitychange', check)
    window.removeEventListener('focus', check)
  })
  return now
}
