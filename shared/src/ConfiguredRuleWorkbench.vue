<script setup lang="ts">
import BusinessDetails from './BusinessDetails.vue'
import { computed, onUnmounted, ref, shallowRef, watch } from 'vue'
import { withGlobalLoading } from './global-loading'
import ConfiguredNodeEditor from './ConfiguredNodeEditor.vue'
import ConfiguredSelection from './ConfiguredSelection.vue'
import ConfiguredPublishing from './ConfiguredPublishing.vue'
import type { RuleModel } from './rules'
import { createCatalogI18n, type LocalizedMessage } from './localization'
import { configuredRuleCatalog } from './configured-rule-catalog'
import { addConfiguredNumberGroup, cloneConfiguredPlay, configuredDemo, createConfiguredPreviewClient, parseConfiguredSnapshot, type ConfiguredSnapshot, type ConfiguredRequest, type ConfiguredPreviewResult, type RuleNode, type ConfiguredPlay } from './configured-rules'
const props = defineProps<{ surface: 'platform' | 'admin'; brandId: string; locale: 'en' | 'zh-CN';gameId?:string;expanded?:boolean;model?:RuleModel }>()
const emit = defineEmits<{ failure: [cause: unknown] }>()
const i18n = createCatalogI18n(configuredRuleCatalog, props.locale)
watch(() => props.locale, value => i18n.setLocale(value))
const t = i18n.t
const api = computed(() => createConfiguredPreviewClient(props.surface, props.brandId))
const draft = shallowRef<ConfiguredSnapshot | null>(null)
const verified = shallowRef<ConfiguredSnapshot | null>(null)
const request = ref<ConfiguredRequest>({ play_id: '', options: [], plate: '', points: '10', multiplier: '1' })
const result = shallowRef<ConfiguredPreviewResult | null>(null)
const editId = ref(''), newId = ref(''), error = ref(''), busy = ref(false), synthetic = ref(false)
const remoteError = ref<LocalizedMessage | null>(null)
const fileInput = ref<HTMLInputElement | null>(null)
const templateName = ref('mark_six')
const templates = ['mark_six','canada_28','canada_996','canada_vip','canada_42','canada_web','canada_50','canada_baccarat','canada_niuniu']
const regular = ref('1,2,3,4,5,6'), special = ref('7'), digits = ref('')
const groupNameEn = ref(''), groupNameZh = ref(''), groupNumbers = ref(''), groupTarget = ref<'all' | 'regular' | 'special'>('special'), groupPosition = ref(''), groupPush = ref('')
const groupPrices = ref<Record<string, string>>({})
const play = computed(() => draft.value?.catalog.plays.find(p => p.id === editId.value))
const node = (value: unknown) => value as RuleNode
const sample=computed(()=>result.value?.result?{requests:[copy(request.value)],draw:{regular:parseNumbers(regular.value),special:parseNumbers(special.value),digits:parseNumbers(digits.value)}}:undefined)
const copy = <T,>(value: T): T => JSON.parse(JSON.stringify(value)) as T
let generation = 0
onUnmounted(() => { generation++ })
watch(draft, () => { verified.value = null; result.value = null; error.value = ''; remoteError.value = null; generation++ }, { deep: true, flush: 'sync' })
watch([request, regular, special, digits], () => { result.value = null; generation++ }, { deep: true, flush: 'sync' })
watch(() => [props.surface, props.brandId], () => { generation++; draft.value = null; verified.value = null; result.value = null; error.value = '' })
function install(snapshot: ConfiguredSnapshot) {
  draft.value = snapshot; editId.value = snapshot.catalog.plays[0]!.id
  request.value = { play_id: editId.value, options: [], plate: snapshot.catalog.plates[0]!, points: snapshot.catalog.plays[0]!.stake_mode === 'unit_multiplier' ? '0' : '10', multiplier: '1' }
}
async function run(action: () => Promise<void>) { busy.value = true; error.value = ''; remoteError.value = null; try { await action() } catch (cause) { const code = (cause as { code?: string }).code; if (code === 'RULE_INVALID' || (!code && cause instanceof Error && !(cause instanceof TypeError))) error.value = 'invalid'; else remoteError.value = i18n.errorMessage(cause, 'read'); if ((cause as { status?: number }).status === 401) emit('failure', cause) } finally { busy.value = false } }
async function load() { await run(async () => { const token = generation; const catalog = await api.value.template(templateName.value); if (token !== generation) return;if(props.model)catalog.model=copy(props.model); install(configuredDemo(catalog)); synthetic.value = true }) }
async function validate() {
  if (!draft.value) return
  await run(async () => { const snapshot = copy(draft.value!); const token = generation; const response = await api.value.preview(snapshot); if (token !== generation) return; verified.value = snapshot; const selected = snapshot.catalog.plays.find(p => p.id === editId.value)!; request.value = { play_id: selected.id, options: [], plate: snapshot.catalog.plates[0]!, points: selected.stake_mode === 'unit_multiplier' ? '0' : '10', multiplier: '1' }; result.value = response })
}
async function imported(event: Event) {
  const input = event.target as HTMLInputElement, file = input.files?.[0]
  if (!file) return
  await run(async () => { const token = generation; if (file.size > 4 * 1024 * 1024) throw new Error('Draft too large'); const snapshot = parseConfiguredSnapshot(await file.text()); if (token !== generation) return; await api.value.preview(snapshot); if (token !== generation) return; install(snapshot); synthetic.value = snapshot.prices.version === 'synthetic-prices' })
  input.value = ''
}
function exportDraft() {
  if (!draft.value) return
  const url = URL.createObjectURL(new Blob([JSON.stringify(draft.value, null, 2)], { type: 'application/json' }))
  const anchor = document.createElement('a'); anchor.href = url; anchor.download = 'rule-draft.json'; document.body.append(anchor); anchor.click(); anchor.remove()
  // Keep the blob alive while Safari starts reading the download.
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}
function duplicate() {
  if (!draft.value || !play.value) return
  try { const next = copy(draft.value); const added = cloneConfiguredPlay(next, play.value, newId.value); draft.value = next; editId.value = added.id; request.value = { ...request.value, play_id: added.id, options: [] }; newId.value = '' } catch { error.value = 'invalid' }
}
function createGroup() {
  if (!draft.value) return
  try {
    const next = copy(draft.value)
    const position = String(groupPosition.value).trim(), push = String(groupPush.value).trim()
    if ((position && !/^\d+$/.test(position)) || (push && !/^\d+$/.test(push))) throw new Error('Invalid number')
    const created = addConfiguredNumberGroup(next, { id: newId.value, label: { en: groupNameEn.value, 'zh-CN': groupNameZh.value }, numbers: parseNumbers(groupNumbers.value), target: groupTarget.value, ...(position ? { position: Number(position) - 1 } : {}), ...(push ? { pushNumber: Number(push) } : {}), prices: copy(groupPrices.value) })
    draft.value = next; editId.value = created.id; newId.value = ''
  } catch { error.value = 'invalid' }
}
function editPlay(value: RuleNode) { if (!draft.value || !play.value) return; const index = draft.value.catalog.plays.indexOf(play.value); const next = copy(draft.value); next.catalog.plays[index] = value as unknown as ConfiguredPlay; draft.value = next; editId.value = next.catalog.plays[index]!.id }
function editPart(part: 'bindings' | 'prices' | 'domain', value: RuleNode) { if (!draft.value) return; const next = copy(draft.value); if (part === 'domain' && play.value) next.catalog.domains[play.value.selection.domain] = value as unknown as ConfiguredSnapshot['catalog']['domains'][string]; else if (part === 'bindings') next.bindings = value as unknown as ConfiguredSnapshot['bindings']; else if (part === 'prices') next.prices = value as unknown as ConfiguredSnapshot['prices']; draft.value = next }
function parseNumbers(text: string) { if (!text.trim()) return []; if (!/^\d+(?:[\s,]+\d+)*$/.test(text.trim())) throw new Error('Invalid draw'); return text.trim().split(/[\s,]+/).map(Number) }
async function calculate(settle: boolean) {
  if (!verified.value) return
  await run(async () => {
    const token = generation
    const response = await api.value.preview(verified.value!, [copy(request.value)], settle ? { regular: parseNumbers(regular.value), special: parseNumbers(special.value), digits: parseNumbers(digits.value) } : undefined)
    if (token === generation) result.value = response
  })
}
</script>
<template>
  <details :open="!!gameId || expanded" class="configured-workbench" data-testid="configured-workbench">
    <summary>{{ t(gameId ? 'editorTitle' : 'title') }}</summary>
    <p>{{ t(gameId ? 'editorHint' : 'hint') }}</p>
    <div class="configured-actions"><label>{{ t('templateChoice') }}<select v-model="templateName" :disabled="busy"><option v-for="name in templates" :key="name" :value="name">{{ t(`template.${name}`) }}</option></select></label><button type="button" class="button button-secondary" :disabled="busy" @click="withGlobalLoading(load)">{{ t('template') }}</button><button type="button" class="button button-secondary" :disabled="busy" @click="fileInput?.click()">{{ t('import') }}</button><input ref="fileInput" type="file" accept="application/json,.json" hidden :disabled="busy" @change="withGlobalLoading(imported, $event)" /><button type="button" class="button button-secondary" :disabled="busy || !draft" @click="exportDraft">{{ t('export') }}</button></div>
    <p v-if="error" class="message error" role="alert">{{ t(error) }}</p>
    <p v-if="remoteError" class="message error" role="alert">{{ i18n.displayError(remoteError) }}</p>
    <p v-if="synthetic">{{ t('synthetic') }}</p>
    <fieldset v-if="draft" :disabled="busy" class="configured-fields">
      <label>{{ t('play') }}<select v-model="editId"><option v-for="p in draft.catalog.plays" :key="p.id" :value="p.id">{{ p.label[locale] }}</option></select></label>
      <div class="configured-actions"><label>{{ t('newId') }}<input v-model.trim="newId" /></label><button type="button" class="button button-secondary" @click="duplicate">{{ t('copy') }}</button></div>
      <details><summary>{{ t('newGroup') }}</summary><div class="configured-fields"><label>{{ t('nameEn') }}<input v-model="groupNameEn" /></label><label>{{ t('nameZh') }}<input v-model="groupNameZh" /></label><label>{{ t('groupNumbers') }}<input v-model="groupNumbers" /></label><label>{{ t('groupTarget') }}<select v-model="groupTarget"><option value="special">{{ t('special') }}</option><option value="regular">{{ t('regular') }}</option><option value="all">{{ t('all') }}</option></select></label><label>{{ t('groupPosition') }}<input v-model="groupPosition" type="number" min="1" step="1" /></label><label>{{ t('groupPush') }}<input v-model="groupPush" type="number" min="1" step="1" /></label><label v-for="plate in draft.catalog.plates" :key="plate">{{ t('groupPrice', { plate }) }}<input v-model="groupPrices[plate]" inputmode="decimal" /></label><button type="button" class="button button-primary" @click="createGroup">{{ t('createGroup') }}</button></div></details>
      <template v-if="play">
        <details><summary>{{ t('definition') }}</summary><ConfiguredNodeEditor :model-value="node(play)" :locale="locale" @update:model-value="editPlay" /></details>
        <details v-if="draft.catalog.domains[play.selection.domain]"><summary>{{ t('domain') }}</summary><ConfiguredNodeEditor :model-value="node(draft.catalog.domains[play.selection.domain])" :locale="locale" @update:model-value="editPart('domain', $event)" /></details>
      </template>
      <details><summary>{{ t('bindings') }}</summary><ConfiguredNodeEditor :model-value="node(draft.bindings)" :locale="locale" @update:model-value="editPart('bindings', $event)" /></details>
      <details><summary>{{ t('prices') }}</summary><ConfiguredNodeEditor :model-value="node(draft.prices)" :locale="locale" @update:model-value="editPart('prices', $event)" /></details>
      <button type="button" class="button button-primary" @click="withGlobalLoading(validate)">{{ t('validate') }}</button>
      <p role="status">{{ verified ? t('validated') : t('stale') }}</p>
      <template v-if="verified">
        <h3>{{ t('preview') }}</h3><ConfiguredSelection v-model="request" :snapshot="verified" :locale="locale" />
        <button type="button" class="button button-secondary" @click="withGlobalLoading(calculate, false)">{{ t('quote') }}</button>
        <label>{{ t('regular') }}<input v-model="regular" /></label><label>{{ t('special') }}<input v-model="special" /></label><label>{{ t('digits') }}<input v-model="digits" /></label><p>{{ t('inputHint') }}</p>
        <button type="button" class="button button-primary" @click="withGlobalLoading(calculate, true)">{{ t('simulate') }}</button>
      </template>
      <dl v-if="result?.quote"><dt>{{ t('entries') }}</dt><dd>{{ result.quote.lines.length }}</dd><dt>{{ t('bet') }}</dt><dd>{{ result.quote.bet_points }}</dd></dl>
      <template v-if="result?.result"><dl><dt>{{ t('prize') }}</dt><dd>{{ result.result.prize_points }}</dd><dt>{{ t('refund') }}</dt><dd>{{ result.result.refund_points }}</dd><dt>{{ t('effective') }}</dt><dd>{{ result.result.effective_points }}</dd></dl><p v-for="(line, index) in result.result.lines" :key="index">{{ index + 1 }} · {{ t(line.outcome) }} · {{ t('prize') }} {{ line.prize_points }} · {{ t('refund') }} {{ line.refund_points }}</p></template>
      <details v-if="result"><summary>{{ t('details') }}</summary><BusinessDetails :value="result" :locale="props.locale" /></details>
    </fieldset>
    <p v-else class="empty-state">{{ t('empty') }}</p>
    <ConfiguredPublishing v-if="gameId" :surface="surface" :brand-id="brandId" :game-id="gameId" :locale="locale" :snapshot="draft" :sample="sample" @load="install" />
  </details>
</template>
<style scoped>
.configured-workbench{border:1px solid var(--line);border-radius:var(--radius);padding:16px;margin:16px 0;min-width:0;font-size:14px;background:var(--surface)}.configured-workbench summary{cursor:pointer;min-height:44px;overflow-wrap:anywhere}.configured-fields{border:0;padding:0;margin:0;display:grid;gap:12px;min-width:0}.configured-fields label{display:grid;gap:6px;min-width:0}.configured-actions{display:flex;flex-wrap:wrap;align-items:end;gap:12px}.configured-actions>*{min-width:0;max-width:100%}.configured-actions input[type=file]{max-width:220px}.configured-fields input,.configured-fields select,.configured-fields textarea{min-height:44px;max-width:100%;width:100%;box-sizing:border-box;min-width:0}.configured-fields textarea{font:13px monospace}.configured-fields>.button{justify-self:start;max-width:100%;white-space:normal;height:auto;min-height:44px}.configured-workbench pre{white-space:pre-wrap;overflow-wrap:anywhere;font-size:12px;min-width:0}.configured-workbench dl{display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1fr);gap:8px}.configured-workbench dd{margin:0}
</style>
