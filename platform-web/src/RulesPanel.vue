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
import { vRefreshButton } from '@lottery/shared';
const displayDate = (value: string | null | undefined) => formatDisplayDateTime(value, props.locale);
import { formatDisplayDateTime } from "@lottery/shared";
import { usePlatformI18n } from "./i18n"
const ui = usePlatformI18n().t
import DetailModal from "@lottery/shared/DetailModal.vue"
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { PlatformApiError } from './platform-api'
import { createPlatformRulesApi, type Play, type RuleVersion } from './rules-api'
import { formatPlatformDateTime, type PlatformDisplaySettings } from './platform-format'

const props = defineProps<{ brandId: string; gameId: string; locale: 'en' | 'zh-CN'; displaySettings: PlatformDisplaySettings }>()
const emit = defineEmits<{ failure: [cause: unknown] }>()
const api = createPlatformRulesApi()
const plays = ref<Play[]>([]), versions = ref<RuleVersion[]>([])
const play = ref<Play | null>(null), detail = ref<RuleVersion | null>(null)
const playOffset = ref(0), versionOffset = ref(0)
const morePlays = ref(false), moreVersions = ref(false)
const loading = ref(false), error = ref<string | import("@lottery/shared").LocalizedMessage>("")
let generation = 0
const copy = computed(() => props.locale === 'en' ? {
  title: ui('platform.copy.6934c281'), note: ui('platform.copy.5178fd5e'), name: ui('platform.copy.aba170ff'), code: ui('platform.copy.ba1081a4'), state: ui('platform.copy.274ae047'), active: ui('platform.copy.991a67a3'), configured: ui('platform.copy.4f14ffd8'), versions: ui('platform.copy.23015b4b'), version: ui('platform.copy.59405491'), mode: ui('platform.copy.4780ba42'), updated: ui('platform.copy.b8b9d3d6'), previous: ui('platform.copy.7f5aba9a'), next: ui('platform.copy.282be575'), empty: ui('platform.copy.ef417ef3'), refresh: ui('platform.copy.f97815a4'), detail: ui('platform.copy.8ce3d92e'), close: ui('platform.copy.27b2fd29'), definition: ui('platform.copy.c2f17b2e'), validation: ui('platform.copy.32d59320'), creator: ui('platform.copy.6d460b01'), reviewer: ui('platform.copy.cc38ac34'), comment: ui('platform.copy.e8577425'), effective: ui('platform.copy.b37371c4'), sequence: ui('platform.copy.e6147a20'), hash: ui('platform.copy.2b7420cc'),
} : {
  title: ui('platform.copy.6934c281'), note: ui('platform.copy.5178fd5e'), name: ui('platform.copy.aba170ff'), code: ui('platform.copy.ba1081a4'), state: ui('platform.copy.274ae047'), active: ui('platform.copy.991a67a3'), configured: ui('platform.copy.4f14ffd8'), versions: ui('platform.copy.23015b4b'), version: ui('platform.copy.59405491'), mode: ui('platform.copy.4780ba42'), updated: ui('platform.copy.b8b9d3d6'), previous: ui('platform.copy.7f5aba9a'), next: ui('platform.copy.282be575'), empty: ui('platform.copy.ef417ef3'), refresh: ui('platform.copy.f97815a4'), detail: ui('platform.copy.8ce3d92e'), close: ui('platform.copy.27b2fd29'), definition: ui('platform.copy.c2f17b2e'), validation: ui('platform.copy.32d59320'), creator: ui('platform.copy.6d460b01'), reviewer: ui('platform.copy.cc38ac34'), comment: ui('platform.copy.e8577425'), effective: ui('platform.copy.b37371c4'), sequence: ui('platform.copy.e6147a20'), hash: ui('platform.copy.2b7420cc'),
})
function report(cause: unknown) {
  if (cause instanceof PlatformApiError && (cause.status === 401 || cause.code === 'PLATFORM_ADMIN_REQUIRED')) emit('failure', cause)
  else error.value = cause instanceof Error ? publicErrorMessage(cause) : publicErrorMessage(cause)
}
async function loadPlays(offset = 0) {
  const request = ++generation, brandId = props.brandId, gameId = props.gameId
  plays.value = []; play.value = null; versions.value = []; detail.value = null; morePlays.value = false; moreVersions.value = false; error.value = ''
  if (!brandId || !gameId) { loading.value = false; return }
  loading.value = true
  try {
    const rows = await api.plays(brandId, gameId, 51, offset)
    if (request === generation) { plays.value = rows.slice(0, 50); morePlays.value = rows.length > 50; playOffset.value = offset }
  } catch (cause) { if (request === generation) report(cause) }
  finally { if (request === generation) loading.value = false }
}
async function loadVersions(selected: Play, offset = 0) {
  const request = ++generation
  play.value = selected; versions.value = []; detail.value = null; moreVersions.value = false; error.value = ''; loading.value = true
  try {
    const rows = await api.versions(props.brandId, props.gameId, selected.id, 51, offset)
    if (request === generation) { versions.value = rows.slice(0, 50); moreVersions.value = rows.length > 50; versionOffset.value = offset }
  } catch (cause) { if (request === generation) report(cause) }
  finally { if (request === generation) loading.value = false }
}
async function read(row: RuleVersion) {
  const request = ++generation
  detail.value = null; error.value = ''; loading.value = true
  try {
    const result = await api.read(props.brandId, props.gameId, row.play_id, row.id)
    if (request === generation) detail.value = result
  } catch (cause) { if (request === generation) report(cause) }
  finally { if (request === generation) loading.value = false }
}
const json = (value: unknown) => JSON.stringify(value, null, 2)
watch(() => [props.brandId, props.gameId], () => { playOffset.value = 0; versionOffset.value = 0; void withGlobalLoading(loadPlays) }, { immediate: true, flush: 'sync' })
onBeforeUnmount(() => { ++generation })
</script>

<template>
  <div data-testid="platform-rules">
    <div class="panel-heading"><div><h2>{{ copy.title }}</h2></div></div>
    <p v-if="error" class="message error" role="alert">{{ displayPublicError(error) }}</p>
    <div class="table-wrap"><table><thead><tr><th>{{ copy.name }}</th><th><BusinessDetails :value="copy.state" :locale="associationLocale" /></th><th>{{ copy.active }}</th></tr></thead><tbody>
      <tr v-for="row in plays" :key="row.id"><td><button class="row-action" :disabled="loading" @click="withGlobalLoading(loadVersions, row)">{{ row.name }}</button></td><td><BusinessDetails :value="row.status" :locale="associationLocale" /></td><td>{{ row.active_version_id ? copy.configured : '—' }}</td></tr><tr v-if="!plays.length"><td colspan="3" class="empty-state">{{ copy.empty }}</td></tr>
    </tbody></table></div>
    <div class="wallet-pagination" data-testid="platform-play-pages"><button class="button button-secondary" :disabled="loading || playOffset === 0" @click="withGlobalLoading(loadPlays, playOffset - 50)">{{ copy.previous }}</button><span>{{ Math.floor(playOffset / 50) + 1 }}</span><button class="button button-secondary" :disabled="loading || !morePlays" @click="withGlobalLoading(loadPlays, playOffset + 50)">{{ copy.next }}</button><button v-refresh-button="copy.refresh" class="button button-secondary" :disabled="loading || !brandId" @click="withGlobalLoading(loadPlays)">{{ copy.refresh }}</button></div>
    <section v-if="play" class="reward-detail" data-testid="platform-rule-versions"><div class="panel-heading"><h3>{{ play.name }} / {{ copy.versions }}</h3></div>
      <div class="table-wrap"><table><thead><tr><th>{{ copy.version }}</th><th><BusinessDetails :value="copy.state" :locale="associationLocale" /></th><th>{{ copy.mode }}</th><th>{{ copy.updated }}</th></tr></thead><tbody>
        <tr v-for="row in versions" :key="row.id"><td><button class="row-action" :disabled="loading" @click="withGlobalLoading(read, row)">{{ row.version_no }}</button></td><td><BusinessDetails :value="row.status" :locale="associationLocale" /></td><td>{{ row.effect_mode }}</td><td>{{ formatPlatformDateTime(row.updated_at, props.displaySettings, props.locale) }}</td></tr><tr v-if="!versions.length"><td colspan="4" class="empty-state">{{ copy.empty }}</td></tr>
      </tbody></table></div>
      <div class="wallet-pagination" data-testid="platform-rule-pages"><button class="button button-secondary" :disabled="loading || versionOffset === 0" @click="withGlobalLoading(loadVersions, play, versionOffset - 50)">{{ copy.previous }}</button><span>{{ Math.floor(versionOffset / 50) + 1 }}</span><button class="button button-secondary" :disabled="loading || !moreVersions" @click="withGlobalLoading(loadVersions, play, versionOffset + 50)">{{ copy.next }}</button></div>
    </section>
    <DetailModal v-if="detail" :title="copy.detail" :close-label="copy.close" @close="detail = null">
<section v-if="detail" class="reward-detail" data-testid="platform-rule-detail">
      <div class="detail-summary"><span><BusinessDetails :value="detail.status" :locale="associationLocale" /> · v{{ detail.version_no }}</span></div>
      <dl class="confirm-list reward-fields"><div><dt>{{ copy.creator }}</dt><dd><AssociationName :record="detail" kind="admin" :id="detail.created_by" :locale="associationLocale" /></dd></div><div><dt>{{ copy.reviewer }}</dt><dd><AssociationName v-if="detail.reviewed_by" :record="detail" kind="admin" :id="detail.reviewed_by" :locale="associationLocale" /><span v-else>{{ '—' }}</span></dd></div><div><dt>{{ copy.comment }}</dt><dd>{{ detail.review_comment || '—' }}</dd></div><div><dt>{{ copy.effective }}</dt><dd>{{ displayDate(detail.effective_at) }}</dd></div><div><dt>{{ copy.sequence }}</dt><dd>{{ detail.effective_sequence ?? '—' }}</dd></div></dl>
      <div class="bet-snapshots"><details><summary>{{ copy.definition }}</summary><BusinessDetails :value="detail.definition" :locale="associationLocale" /></details><details><summary>{{ copy.validation }}</summary><BusinessDetails :value="detail.validation ?? null" :locale="associationLocale" /></details></div>
    </section>
</DetailModal>
  </div>
</template>
