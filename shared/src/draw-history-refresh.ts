import { onMounted, onUnmounted } from 'vue'

/** Keep visible draw history current without overlapping reads or resetting its filters. */
export function useDrawHistoryRefresh(enabled: () => boolean, busy: () => boolean, refresh: () => Promise<void>) {
 let timer: ReturnType<typeof setInterval>
 function check() {
  if (document.visibilityState === 'visible' && enabled() && !busy()) void refresh()
 }
 onMounted(() => {
  timer = setInterval(check, 5000)
  document.addEventListener('visibilitychange', check)
 })
 onUnmounted(() => {
  clearInterval(timer)
  document.removeEventListener('visibilitychange', check)
 })
}
