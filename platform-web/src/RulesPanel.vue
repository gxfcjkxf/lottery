<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { PlatformApiError } from './platform-api'
import { createPlatformRulesApi, type Play, type RuleVersion } from './rules-api'

const props = defineProps<{ brandId: string; gameId: string; locale: 'en' | 'zh-CN' }>()
const emit = defineEmits<{ failure: [cause: unknown] }>()
const api = createPlatformRulesApi()
const plays = ref<Play[]>([]), versions = ref<RuleVersion[]>([])
const play = ref<Play | null>(null), detail = ref<RuleVersion | null>(null)
const playOffset = ref(0), versionOffset = ref(0)
const morePlays = ref(false), moreVersions = ref(false)
const loading = ref(false), error = ref('')
let generation = 0
const copy = computed(() => props.locale === 'en' ? {
  title: 'Plays and rules', note: 'Read-only rule versions; review and publication belong to the brand', name: 'Play', code: 'Code', state: 'State', active: 'Active rule ID', versions: 'Rule versions', version: 'Version', mode: 'Effect mode', updated: 'Updated', previous: 'Previous page', next: 'Next page', empty: 'Nothing to show.', loading: 'Loading…', refresh: 'Refresh plays', detail: 'Rule version detail', definition: 'Rule definition', validation: 'Validation report', creator: 'Created by', reviewer: 'Reviewed by', comment: 'Review comment', effective: 'Effective at', sequence: 'Effective sequence', hash: 'Definition hash',
} : {
  title: '玩法与规则', note: '只读规则版本，审核和发布由品牌负责', name: '玩法', code: '代码', state: '状态', active: '生效规则编号', versions: '规则版本', version: '版本', mode: '生效方式', updated: '更新时间', previous: '上一页', next: '下一页', empty: '暂无记录。', loading: '加载中…', refresh: '刷新玩法', detail: '规则版本详情', definition: '规则定义', validation: '验证报告', creator: '创建人', reviewer: '审核人', comment: '审核意见', effective: '生效时间', sequence: '生效序号', hash: '规则摘要',
})
function report(cause: unknown) {
  if (cause instanceof PlatformApiError && (cause.status === 401 || cause.code === 'PLATFORM_ADMIN_REQUIRED')) emit('failure', cause)
  else error.value = cause instanceof Error ? cause.message : String(cause)
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
watch(() => [props.brandId, props.gameId], () => { playOffset.value = 0; versionOffset.value = 0; void loadPlays() }, { immediate: true, flush: 'sync' })
onBeforeUnmount(() => { ++generation })
</script>

<template>
  <div data-testid="platform-rules">
    <div class="panel-heading"><div><h2>{{ copy.title }}</h2><p>{{ copy.note }}</p></div><button class="secondary" :disabled="loading || !brandId" @click="loadPlays()">{{ copy.refresh }}</button></div>
    <p v-if="error" class="message error" role="alert">{{ error }}</p><p v-if="loading" class="loading-line">{{ copy.loading }}</p>
    <div class="table-wrap"><table><thead><tr><th>{{ copy.name }}</th><th>{{ copy.code }}</th><th>{{ copy.state }}</th><th>{{ copy.active }}</th></tr></thead><tbody>
      <tr v-for="row in plays" :key="row.id"><td><button class="row-action" :disabled="loading" @click="loadVersions(row)">{{ row.name }}</button><small>{{ row.id }}</small></td><td>{{ row.code }}</td><td>{{ row.status }}</td><td>{{ row.active_version_id || '—' }}</td></tr><tr v-if="!plays.length"><td colspan="4" class="empty-state">{{ copy.empty }}</td></tr>
    </tbody></table></div>
    <div class="wallet-pagination" data-testid="platform-play-pages"><button class="secondary" :disabled="loading || playOffset === 0" @click="loadPlays(playOffset - 50)">{{ copy.previous }}</button><span>{{ Math.floor(playOffset / 50) + 1 }}</span><button class="secondary" :disabled="loading || !morePlays" @click="loadPlays(playOffset + 50)">{{ copy.next }}</button></div>
    <section v-if="play" class="reward-detail" data-testid="platform-rule-versions"><div class="panel-heading"><h3>{{ play.name }} / {{ copy.versions }}</h3></div>
      <div class="table-wrap"><table><thead><tr><th>{{ copy.version }}</th><th>{{ copy.state }}</th><th>{{ copy.mode }}</th><th>{{ copy.updated }}</th></tr></thead><tbody>
        <tr v-for="row in versions" :key="row.id"><td><button class="row-action" :disabled="loading" @click="read(row)">{{ row.version_no }}</button><small>{{ row.id }}</small></td><td>{{ row.status }}</td><td>{{ row.effect_mode }}</td><td>{{ row.updated_at }}</td></tr><tr v-if="!versions.length"><td colspan="4" class="empty-state">{{ copy.empty }}</td></tr>
      </tbody></table></div>
      <div class="wallet-pagination" data-testid="platform-rule-pages"><button class="secondary" :disabled="loading || versionOffset === 0" @click="loadVersions(play, versionOffset - 50)">{{ copy.previous }}</button><span>{{ Math.floor(versionOffset / 50) + 1 }}</span><button class="secondary" :disabled="loading || !moreVersions" @click="loadVersions(play, versionOffset + 50)">{{ copy.next }}</button></div>
    </section>
    <section v-if="detail" class="reward-detail" data-testid="platform-rule-detail">
      <div class="panel-heading"><h3>{{ copy.detail }} / {{ detail.version_no }}</h3><span>{{ detail.status }}</span></div>
      <dl class="confirm-list reward-fields"><div><dt>{{ copy.creator }}</dt><dd>{{ detail.created_by }}</dd></div><div><dt>{{ copy.reviewer }}</dt><dd>{{ detail.reviewed_by || '—' }}</dd></div><div><dt>{{ copy.comment }}</dt><dd>{{ detail.review_comment || '—' }}</dd></div><div><dt>{{ copy.effective }}</dt><dd>{{ detail.effective_at || '—' }}</dd></div><div><dt>{{ copy.sequence }}</dt><dd>{{ detail.effective_sequence ?? '—' }}</dd></div><div><dt>{{ copy.hash }}</dt><dd>{{ detail.definition_hash }}</dd></div></dl>
      <div class="bet-snapshots"><details><summary>{{ copy.definition }}</summary><pre>{{ json(detail.definition) }}</pre></details><details><summary>{{ copy.validation }}</summary><pre>{{ json(detail.validation ?? null) }}</pre></details></div>
    </section>
  </div>
</template>
