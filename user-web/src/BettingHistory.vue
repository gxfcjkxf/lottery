<script setup lang="ts">
import OrderHistoryRecord from "@lottery/shared/OrderHistoryRecord.vue"
import { computed, onUnmounted, ref, shallowRef, watch } from 'vue'
import { RouterLink } from 'vue-router'
import { withGlobalLoading, vGlobalLoading, type LocalizedMessage } from '@lottery/shared'
import type { AuthProfile } from '../../shared/src/auth'
import OrderHistoryLayout from './OrderHistoryLayout.vue'
import { readBettingHistory, type HistoryPage } from './betting-history-api'
import { useUserI18n } from './i18n'

const props = defineProps<{ locale: 'en' | 'zh'; brandCode?: string; auth?: AuthProfile | null }>()
const emit = defineEmits<{ 'auth-expired': [] }>()
const i18n = useUserI18n(), ui = i18n.t
const localeCode = computed(() => props.locale === 'en' ? 'en' : 'zh-CN')
const selectedGame = ref(''), loading = ref(false), error = ref<LocalizedMessage | null>(null)
const page = shallowRef<HistoryPage | null>(null)
const games = shallowRef<HistoryPage['games']>([])
const cursors = ref<Array<string | null>>([null]), pageIndex = ref(0)
let generation = 0, controller: AbortController | null = null
const scope = computed(() => JSON.stringify([props.brandCode, props.auth?.member.brand_id, props.auth?.member.id]))
async function load(index = pageIndex.value, cursor = cursors.value[index] ?? null, passive = false) {
  const token = ++generation
  controller?.abort(); controller = new AbortController()
  error.value = null; if (!passive) loading.value = true
  const profile = props.auth
  if (!profile) { page.value = null; loading.value = false; return }
  try {
    const value = await readBettingHistory(props.brandCode, { brand_id: profile.member.brand_id, member_id: profile.member.id }, selectedGame.value, cursor, controller.signal)
    if (token !== generation) return
    page.value = value; games.value = value.games; pageIndex.value = index
    cursors.value = [...cursors.value.slice(0, index), cursor]
  } catch (cause) {
    if (token !== generation) return
    page.value = null; error.value = i18n.errorMessage(cause, 'read')
    if ((cause as { status?: number }).status === 401) emit('auth-expired')
  } finally { if (token === generation) loading.value = false }
}
function resetPage() { generation++; controller?.abort(); page.value = null; pageIndex.value = 0; cursors.value = [null] }
function changeGame() { resetPage(); void withGlobalLoading(load, 0, null) }
function refresh() { resetPage(); return load(0, null) }
watch(scope, () => { selectedGame.value = ''; games.value = []; resetPage(); void load(0, null) }, { immediate: true, flush: 'sync' })
const poll = setInterval(() => {
  if (props.auth && !loading.value) void load(pageIndex.value, cursors.value[pageIndex.value] ?? null, true)
}, 15_000)
onUnmounted(() => { generation++; controller?.abort(); clearInterval(poll) })
</script>

<template>
  <section class="betting-history" data-testid="betting-history">
    <OrderHistoryLayout :title="ui('user.orders.title')" :loading="loading" :empty="!!auth && !error && !!page && !page.items.length" @refresh="withGlobalLoading(refresh)">
      <template #filters>
        <div v-if="auth" class="history-filters">
          <select v-model="selectedGame" :aria-label="ui('user.orders.game')" :disabled="loading" @change="changeGame">
            <option value="">{{ ui('user.orders.allGames') }}</option>
            <option v-for="game in games" :key="game.id" :value="game.id">{{ game.name }}</option>
          </select>
        </div>
      </template>
      <p v-if="!auth">{{ ui('betting.signInHistory') }} <RouterLink to="/login">{{ ui('betting.signIn') }}</RouterLink></p>
      <p v-if="loading" v-global-loading role="status">{{ ui('betting.loadingOrders') }}</p>
      <p v-if="error" class="message error" role="alert">{{ i18n.displayError(error) }}</p>
      <OrderHistoryRecord v-for="item in page?.items ?? []" :key="`${item.kind}:${item.id}`" :item="item" :locale="localeCode"><template #details><RouterLink :to="`/orders/${encodeURIComponent(item.id)}`" class="text-link">{{ui('betting.details')}}</RouterLink></template></OrderHistoryRecord>
      <template #pagination>
        <nav v-if="auth" class="history-pagination" :aria-label="ui('user.orders.pages')">
          <button class="button button-secondary" :disabled="loading || pageIndex === 0" @click="withGlobalLoading(load, pageIndex - 1)">{{ ui('user.orders.previous') }}</button>
          <span>{{ ui('user.orders.page', { number: pageIndex + 1 }) }}</span>
          <button class="button button-secondary" :disabled="loading || !page?.next_cursor" @click="withGlobalLoading(load, pageIndex + 1, page!.next_cursor)">{{ ui('user.orders.next') }}</button>
        </nav>
      </template>
    </OrderHistoryLayout>
  </section>
</template>
<style scoped>
.betting-history { display: grid; gap: 12px; min-width: 0; max-width: 1100px; margin: 0 auto; }
.history-filters, .history-pagination { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
</style>
