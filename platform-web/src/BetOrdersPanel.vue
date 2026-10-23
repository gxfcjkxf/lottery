<script setup lang="ts">
import BusinessDetails from '@lottery/shared/BusinessDetails.vue'
import { usePlatformI18n as useAssociationI18n } from "./i18n";
const associationLocale = useAssociationI18n().locale;
import { AssociationName } from "@lottery/shared";
import { usePlatformI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import SiteIcon from "@lottery/shared/SiteIcon";
import { vInlineValidation } from '@lottery/shared'
import { usePlatformI18n } from "./i18n"
const ui = usePlatformI18n().t
import DetailModal from "@lottery/shared/DetailModal.vue"
import { computed, ref, watch } from 'vue'
import { PlatformApiError } from './platform-api'
import { createPlatformBetApi, type BetOrder } from './bet-api'
import { lookupPlatformMember } from './member-lookup'
import { formatPlatformDateTime, type PlatformDisplaySettings } from './platform-format'

const props = defineProps<{ brandId: string; gameId: string; locale: 'en' | 'zh-CN'; displaySettings: PlatformDisplaySettings }>()
const emit = defineEmits<{ failure: [cause: unknown] }>()
const api = createPlatformBetApi()
const draftMember = ref('')
const memberFilter = ref('')
const orders = ref<BetOrder[]>([])
const detail = ref<BetOrder | null>(null)
const offset = ref(0)
const more = ref(false)
const loading = ref(false)
const error = ref<string | import("@lottery/shared").LocalizedMessage>("")
let generation = 0
async function submitMemberQuery() {
  const request = ++generation, brandId = props.brandId, gameId = props.gameId
  const username = draftMember.value.trim()
  orders.value = []; detail.value = null; more.value = false; error.value = ''; loading.value = true
  try {
    const member = username ? (await lookupPlatformMember(brandId, username)).id : ''
    if (request !== generation || brandId !== props.brandId || gameId !== props.gameId) return
    await load(0, member)
  } catch (cause) { if (request === generation) report(cause) }
  finally { if (request === generation) loading.value = false }
}
const copy = computed(() => props.locale === 'en' ? {
  title: ui('platform.copy.7ce4e2b7'), member: ui('platform.copy.3c207805'), query: ui('platform.copy.af1775f6'), order: ui('platform.copy.68419640'), points: ui('platform.copy.6b08271d'), state: ui('platform.copy.274ae047'), period: ui('platform.copy.af748127'), date: ui('platform.copy.9b57c1f0'), previous: ui('platform.copy.7f5aba9a'), next: ui('platform.copy.282be575'), empty: ui('platform.copy.9aec450c'), detail: ui('platform.copy.ca30a55c'), close: ui('platform.copy.27b2fd29'), game: ui('platform.copy.e9964c8b'), play: ui('platform.copy.aba170ff'), version: ui('platform.copy.073d61ac'), prize: ui('platform.copy.af7a2edc'), unit: ui('platform.copy.37ca364a'), combinations: ui('platform.copy.d061bced'), multiplier: ui('platform.copy.c7c39827'), rule: ui('platform.copy.095bcc1e'), hash: ui('platform.copy.2b7420cc'), snapshots: ui('platform.copy.176bea8d'), selection: ui('platform.copy.5fc847bd'), normalized: ui('platform.copy.f8ab27bb'), expanded: ui('platform.copy.f3856e26'), definition: ui('platform.copy.c2f17b2e'), policy: ui('platform.copy.34a28248'), allocation: ui('platform.copy.2407aee6'),
} : {
  title: ui('platform.copy.7ce4e2b7'), member: ui('platform.copy.3c207805'), query: ui('platform.copy.af1775f6'), order: ui('platform.copy.68419640'), points: ui('platform.copy.6b08271d'), state: ui('platform.copy.274ae047'), period: ui('platform.copy.af748127'), date: ui('platform.copy.9b57c1f0'), previous: ui('platform.copy.7f5aba9a'), next: ui('platform.copy.282be575'), empty: ui('platform.copy.9aec450c'), detail: ui('platform.copy.ca30a55c'), close: ui('platform.copy.27b2fd29'), game: ui('platform.copy.e9964c8b'), play: ui('platform.copy.aba170ff'), version: ui('platform.copy.073d61ac'), prize: ui('platform.copy.af7a2edc'), unit: ui('platform.copy.37ca364a'), combinations: ui('platform.copy.d061bced'), multiplier: ui('platform.copy.c7c39827'), rule: ui('platform.copy.095bcc1e'), hash: ui('platform.copy.2b7420cc'), snapshots: ui('platform.copy.176bea8d'), selection: ui('platform.copy.5fc847bd'), normalized: ui('platform.copy.f8ab27bb'), expanded: ui('platform.copy.f3856e26'), definition: ui('platform.copy.c2f17b2e'), policy: ui('platform.copy.34a28248'), allocation: ui('platform.copy.2407aee6'),
})
function report(cause: unknown) {
  if (cause instanceof PlatformApiError && (cause.status === 401 || cause.code === 'PLATFORM_ADMIN_REQUIRED')) emit('failure', cause)
  else error.value = cause instanceof Error ? publicErrorMessage(cause) : publicErrorMessage(cause)
}
async function load(nextOffset = 0, filter = memberFilter.value) {
  const request = ++generation
  const brandId = props.brandId
  const gameId = props.gameId
  orders.value = []; detail.value = null; more.value = false; error.value = ''
  if (!brandId) { loading.value = false; return }
  loading.value = true
  try {
    const rows = await api.list(brandId, filter, 51, nextOffset, gameId)
    if (request !== generation || brandId !== props.brandId) return
    orders.value = rows.slice(0, 50); more.value = rows.length > 50
    offset.value = nextOffset; memberFilter.value = filter
  } catch (cause) { if (request === generation) report(cause) }
  finally { if (request === generation) loading.value = false }
}
async function read(order: BetOrder) {
  const request = ++generation
  const brandId = props.brandId
  detail.value = null; error.value = ''; loading.value = true
  try {
    const current = await api.read(brandId, order.id)
    if (request === generation && brandId === props.brandId) detail.value = current
  } catch (cause) { if (request === generation) report(cause) }
  finally { if (request === generation) loading.value = false }
}
const json = (value: unknown) => JSON.stringify(value, null, 2)
const stateLabel = (state: BetOrder['status']) => (props.locale === 'en' ? {
  placed: ui('platform.copy.d8c3d45b'), bet_cancelled: ui('platform.copy.ad8a78ee'), judged_cancelled: ui('platform.copy.00a52a01'), abnormal: ui('platform.copy.d217c9e6'), won: ui('platform.copy.9dcd1df2'), lost: ui('platform.copy.74170da6'),
} : {
  placed: ui('platform.copy.d8c3d45b'), bet_cancelled: ui('platform.copy.ad8a78ee'), judged_cancelled: ui('platform.copy.00a52a01'), abnormal: ui('platform.copy.d217c9e6'), won: ui('platform.copy.9dcd1df2'), lost: ui('platform.copy.74170da6'),
})[state]
watch(() => [props.brandId, props.gameId] as const, ([brand], previous) => {
  if (!previous || brand !== previous[0]) { draftMember.value = ''; memberFilter.value = '' }
  offset.value = 0
  void withGlobalLoading(load)
}, { immediate: true, flush: 'sync' })
</script>

<template>
  <div data-testid="platform-bets">
    <form novalidate v-inline-validation class="panel wallet-query" @submit.prevent="withGlobalLoading(submitMemberQuery)">
      <label class="query-field">
        <div class="input-with-icon">
          <span class="input-icon"> <SiteIcon name="user" /> </span>
          <input v-model="draftMember" :disabled="loading || !brandId" autocomplete="off" :aria-label="copy.member" :placeholder="copy.member" />
        </div>
      </label>
      <button class="button button-primary" :disabled="loading || !brandId">{{ copy.query }}</button>
    </form>
    <p v-if="error" class="message error" role="alert">{{ displayPublicError(error) }}</p>
    <section class="panel reward-detail">
      <div class="table-wrap"><table><thead><tr><th>{{ copy.order }}</th><th>{{ copy.period }}</th><th>{{ copy.points }}</th><th><BusinessDetails :value="copy.state" :locale="associationLocale" /></th><th>{{ copy.date }}</th></tr></thead><tbody>
        <tr v-for="order in orders" :key="order.id"><td><button class="row-action" @click="withGlobalLoading(read, order)">{{ order.id }}</button><small><AssociationName :record="order" kind="member" :id="order.brand_member_id" :locale="associationLocale" /></small></td><td><AssociationName :record="order" kind="period" :id="order.period_id" :locale="associationLocale" /></td><td>{{ order.total_points }}</td><td>{{ stateLabel(order.status) }}</td><td>{{ formatPlatformDateTime(order.placed_at, props.displaySettings, props.locale) }}</td></tr>
        <tr v-if="!orders.length"><td colspan="5" class="empty-state">{{ copy.empty }}</td></tr>
      </tbody></table></div>
      <div class="wallet-pagination" data-testid="platform-bet-pages"><button class="button button-secondary" :disabled="loading || offset === 0 || !brandId" @click="withGlobalLoading(load, offset - 50)">{{ copy.previous }}</button><span>{{ Math.floor(offset / 50) + 1 }}</span><button class="button button-secondary" :disabled="loading || !more" @click="withGlobalLoading(load, offset + 50)">{{ copy.next }}</button></div>
    </section>
    <DetailModal v-if="detail" :title="copy.detail" :close-label="copy.close" @close="detail = null">
<section v-if="detail" class="detail-section" data-testid="platform-bet-detail">
      <div class="detail-summary"><span class="status-pill" :class="detail.status">{{ stateLabel(detail.status) }}</span></div>
      <dl class="confirm-list reward-fields">
        <div><dt>{{ copy.order }}</dt><dd>{{ detail.id }}</dd></div>
        <div><dt>{{ copy.game }}</dt><dd><AssociationName :record="detail" kind="game" :id="detail.game_id" :locale="associationLocale" /></dd></div><div><dt>{{ copy.period }}</dt><dd><AssociationName :record="detail" kind="period" :id="detail.period_id" :locale="associationLocale" /></dd></div><div><dt>{{ copy.play }}</dt><dd><AssociationName :record="detail" kind="play" :id="detail.play_id" :locale="associationLocale" /></dd></div>
        <div><dt>{{ copy.unit }}</dt><dd>{{ detail.unit_points }}</dd></div><div><dt>{{ copy.combinations }}</dt><dd>{{ detail.combination_count }}</dd></div><div><dt>{{ copy.multiplier }}</dt><dd>{{ detail.multiplier }}</dd></div>
        <div><dt>{{ copy.points }}</dt><dd>{{ detail.total_points }}</dd></div><div><dt>{{ copy.prize }}</dt><dd>{{ detail.prize_points }}</dd></div><div><dt>{{ copy.rule }}</dt><dd>{{ detail.rule_version_id }}</dd></div>
      </dl>
      <div class="bet-snapshots"><h3>{{ copy.snapshots }}</h3>
        <details><summary>{{ copy.selection }}</summary><BusinessDetails :value="detail.selection_raw" :locale="associationLocale" /></details>
        <details><summary>{{ copy.normalized }}</summary><BusinessDetails :value="detail.selection_normalized" :locale="associationLocale" /></details>
        <details><summary>{{ copy.expanded }}</summary><BusinessDetails :value="detail.expanded_bets" :locale="associationLocale" /></details>
        <details><summary>{{ copy.definition }}</summary><BusinessDetails :value="detail.definition_snapshot" :locale="associationLocale" /></details>
        <details><summary>{{ copy.policy }}</summary><BusinessDetails :value="detail.policy_snapshot" :locale="associationLocale" /></details>
        <details><summary>{{ copy.allocation }}</summary><BusinessDetails :value="detail.deduction_allocation" :locale="associationLocale" /></details>
      </div>
    </section>
</DetailModal>
  </div>
</template>
