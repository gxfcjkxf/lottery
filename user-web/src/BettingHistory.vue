<script setup lang="ts">
import TablePagination from "@lottery/shared/TablePagination.vue"
import { useOperationToast } from "@lottery/shared";
import BettingHistoryTable from "@lottery/shared/BettingHistoryTable.vue"
import { computed, onUnmounted, ref, shallowRef, watch } from 'vue'
import { RouterLink } from 'vue-router'
import { withGlobalLoading, vGlobalLoading, type LocalizedMessage } from '@lottery/shared'
import type { AuthProfile } from '../../shared/src/auth'
import OrderHistoryLayout from './OrderHistoryLayout.vue'
import { readBettingHistory, type HistoryPage } from './betting-history-api'
import { useUserI18n } from './i18n'

const props = defineProps<{ locale: 'en' | 'zh'; brandCode?: string; auth?: AuthProfile | null; gameId: string }>()
const emit = defineEmits<{ 'auth-expired': []; games: [games: HistoryPage['games']] }>()
const i18n = useUserI18n(), ui = i18n.t
const localeCode = computed(() => props.locale === 'en' ? 'en' : 'zh-CN')
const selectedGame = ref(''), loading = ref(false), error = ref<LocalizedMessage | null>(null)
const page = shallowRef<HistoryPage | null>(null)
const cursors = ref<Array<string | null>>([null]), pageIndex = ref(0), pageSize = ref(20)
let generation = 0, controller: AbortController | null = null
const scope = computed(() => JSON.stringify([props.brandCode, props.auth?.member.brand_id, props.auth?.member.id]))
async function load(index = pageIndex.value, cursor = cursors.value[index] ?? null, passive = false, limit = pageSize.value) {
  const token = ++generation
  controller?.abort(); controller = new AbortController()
  error.value = null; if (!passive) loading.value = true
  const profile = props.auth
  if (!profile) { page.value = null; loading.value = false; return }
  try {
    const value = await readBettingHistory(props.brandCode, { brand_id: profile.member.brand_id, member_id: profile.member.id }, selectedGame.value, cursor, controller.signal, limit)
    if (token !== generation) return
    page.value = value; emit('games', value.games); pageIndex.value = index; pageSize.value = limit
    cursors.value = [...cursors.value.slice(0, index), cursor]
  } catch (cause) {
    if (token !== generation) return
    error.value = i18n.errorMessage(cause, 'read')
    if ((cause as { status?: number }).status === 401) { page.value = null; emit('auth-expired') }
  } finally { if (token === generation) loading.value = false }
}
function resetPage() { generation++; controller?.abort(); page.value = null; pageIndex.value = 0; cursors.value = [null] }
function refresh() { return load() }
function changePageSize(size: number) { return load(0, null, false, size) }
watch(() => [scope.value, props.gameId] as const, ([nextScope, gameId], previous) => {
  const changedScope = nextScope !== previous?.[0]
  if (!changedScope && selectedGame.value === gameId) return
  selectedGame.value = gameId
  resetPage(); void load(0, null)
}, { immediate: true, flush: 'sync' })
const poll = setInterval(() => {
  if (props.auth && !loading.value) void load(pageIndex.value, cursors.value[pageIndex.value] ?? null, true)
}, 15_000)
onUnmounted(() => { generation++; controller?.abort(); clearInterval(poll) })

useOperationToast(error, value => i18n.displayError(value), 'error');
</script>

<template>
  <section class="betting-history" data-testid="betting-history">
    <OrderHistoryLayout :title="ui('user.orders.title')" :loading="loading" :error="!!error" :empty="!!auth && !!page && !page.items.length" @refresh="withGlobalLoading(refresh)">
      <p v-if="!auth">{{ ui('betting.signInHistory') }} <RouterLink to="/login">{{ ui('betting.signIn') }}</RouterLink></p>
      <p v-if="loading" v-global-loading role="status">{{ ui('betting.loadingOrders') }}</p>

      <BettingHistoryTable v-if="page?.items.length" :items="page.items" :locale="localeCode" :show-game="!gameId" />
      <template #pagination>
        <TablePagination v-if="auth && page" class="history-pagination" :aria-label="ui('user.orders.pages')" :total="page.total_count" :page="pageIndex + 1" :count="page.items.length" :locale="localeCode" :page-size="pageSize" :change-page-size="changePageSize" :previous-disabled="loading || pageIndex === 0" :next-disabled="loading || !page.next_cursor" :previous-label="ui('user.orders.previous')" :next-label="ui('user.orders.next')" :go-to="(target: number) => load(target - 1, target === pageIndex + 2 ? page!.next_cursor : cursors[target - 1])" :reachable-page="pageIndex + (page.next_cursor ? 2 : 1)" :busy="loading" />
      </template>
    </OrderHistoryLayout>
  </section>
</template>
<style scoped>
.betting-history { display: grid; gap: 12px; min-width: 0; width: 100%; margin: 0 auto; }
</style>
