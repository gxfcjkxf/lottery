import { onUnmounted, ref, watch, type Ref } from 'vue'
import { readAllFilterOptions, type FilterOption } from '../../shared/src/filter-options'

/** Game filters use the same brand and permission as the order list. */
export function useBrandGames(brandId: Ref<string>, scope: () => string, report: (cause: unknown) => void, read = readAllFilterOptions) {
  const games = ref<FilterOption[]>([])
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
      const rows = await read({ surface:'platform',brandId:brand,resource:'bet',kind:'game' })
      if (request !== generation) return
      games.value = rows
    } catch (cause) { if (request === generation) report(cause) }
    finally { if (request === generation) loading.value = false }
  }, { immediate: true, flush: 'sync' })
  onUnmounted(() => { generation++ })
  return { games, selectedGameId, loading, refresh: () => { revision.value++ } }
}
