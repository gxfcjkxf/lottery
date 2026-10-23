import { onUnmounted, ref, watch, type Ref } from 'vue'
import { createPlatformLotteryApi, type Game } from './lottery-api'

/** Brand-scoped game selection shared by draws and order filters. */
export function useBrandGames(brandId: Ref<string>, scope: () => string, report: (cause: unknown) => void, api = createPlatformLotteryApi()) {
  const games = ref<Game[]>([])
  const selectedGameId = ref('')
  const loading = ref(false)
  const revision = ref(0)
  let generation = 0
  watch(() => [brandId.value, scope(), revision.value] as const, async ([brand, currentScope]) => {
    const request = ++generation
    games.value = []; selectedGameId.value = ''; loading.value = false
    if (!brand || !currentScope) return
    loading.value = true
    try {
      const rows: Game[] = []
      for (let offset = 0; ; offset += 100) {
        const page = await api.games(brand, 100, offset)
        if (request !== generation) return
        rows.push(...page)
        if (page.length < 100) break
      }
      games.value = rows
    } catch (cause) { if (request === generation) report(cause) }
    finally { if (request === generation) loading.value = false }
  }, { immediate: true, flush: 'sync' })
  onUnmounted(() => { generation++ })
  return { games, selectedGameId, loading, refresh: () => { revision.value++ } }
}
