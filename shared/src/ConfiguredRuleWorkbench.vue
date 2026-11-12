<script setup lang="ts">
import ClearableInput from './ClearableInput.vue'
import DetailModal from './DetailModal.vue'
import ManagementTable from './ManagementTable.vue'
import SelectControl from './SelectControl.vue'
import BusinessDetails from './BusinessDetails.vue'
import { bindingText, bindingErrors, parseBindingNumbers, configuredPriceRows } from './configured-rule-editor'
import { computed, onUnmounted, ref, shallowRef, watch } from 'vue'
import { withGlobalLoading } from './global-loading'
import ConfiguredNodeEditor from './ConfiguredNodeEditor.vue'
import ConfiguredSelection from './ConfiguredSelection.vue'
import ConfiguredPublishing from './ConfiguredPublishing.vue'
import type { RuleModel } from './rules'
import { createCatalogI18n, type LocalizedMessage } from './localization'
import { configuredRuleCatalog } from './configured-rule-catalog'
import { addConfiguredNumberGroup, cloneConfiguredPlay, configuredDemo, createConfiguredPreviewClient, parseConfiguredSnapshot, type ConfiguredSnapshot, type ConfiguredRequest, type ConfiguredPreviewResult, type RuleNode, type ConfiguredPlay } from './configured-rules'
const props = defineProps<{ surface: 'platform' | 'admin'; brandId: string; locale: 'en' | 'zh-CN';gameId?:string;expanded?:boolean;model?:RuleModel;canWrite?:boolean }>()
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
const publicationLocked = ref(false)
const locked = computed(() => busy.value || publicationLocked.value)
const templateName = ref('mark_six')
const templates = ['mark_six','canada_28','canada_996','canada_vip','canada_42','canada_web','canada_50','canada_baccarat','canada_niuniu']
const regular = ref('1,2,3,4,5,6'), special = ref('7'), digits = ref('')
const groupNameEn = ref(''), groupNameZh = ref(''), groupNumbers = ref(''), groupTarget = ref<'all' | 'regular' | 'special'>('special'), groupPosition = ref(''), groupPush = ref('')
const groupPrices = ref<Record<string, string>>({})
const addMode = ref<'copy' | 'group'>('copy')
const attributeId = ref('')
const bindingTexts = ref<Record<string, Record<string, string>>>({})
const modalDraft = shallowRef<ConfiguredSnapshot | null>(null)
const savedSnapshot = ref('')
const modal = ref<'play' | 'add' | 'bindings' | 'prices' | 'domain' | null>(null)
const tab = ref<'basic' | 'settlement' | 'simulation'>('basic')
const search = ref(''), groupFilter = ref('')
const working = computed(() => modalDraft.value ?? draft.value)
const editable = computed(() => !props.gameId || (props.surface === 'platform' && !!props.canWrite))
const dirty = computed(() => !!draft.value && JSON.stringify(draft.value) !== savedSnapshot.value)
const play = computed(() => working.value?.catalog.plays.find(p => p.id === editId.value))
const groups = computed(() => [...new Set(draft.value?.catalog.plays.map(p => p.group))])
const groupLabel = (group: string) => `group.${group}` in configuredRuleCatalog.en ? String(t(`group.${group}`)) : group
const rows = computed(() => (draft.value?.catalog.plays ?? []).filter(p => (!groupFilter.value || p.group === groupFilter.value) && `${p.label[props.locale]} ${p.id} ${groupLabel(p.group)}`.toLowerCase().includes(search.value.trim().toLowerCase())))
const columns = computed(() => [{key:'name',label:String(t('play'))},{key:'group',label:String(t('group'))},{key:'mode',label:String(t('mode'))},{key:'count',label:String(t('selectionCount'))},{key:'action',label:String(t('actions'))}])
const basicKeys = ['label','group','selection','stake_mode','unit_points','max_multiplier']
const basic = computed(() => Object.fromEntries(basicKeys.filter(key=>play.value && key in play.value).map(key=>[key,(play.value as unknown as Record<string,unknown>)[key]])))
const settlement = computed(() => Object.fromEntries(Object.entries(play.value ?? {}).filter(([key]) => !basicKeys.includes(key) && !['id','source_code'].includes(key))))
defineExpose({ confirmLeave: () => !(dirty.value || modalChanged.value) || window.confirm(String(t('discardRules'))) })
const attribute = computed(() => modalDraft.value?.catalog.attributes.find(item => item.id === attributeId.value))
const bindingsChanged = computed(() => modal.value === 'bindings' && !!modalDraft.value && JSON.stringify(bindingTexts.value) !== JSON.stringify(bindingText(modalDraft.value)))
const bindingIssues = computed(() => modal.value === 'bindings' && modalDraft.value ? bindingErrors(modalDraft.value.catalog, bindingTexts.value) : {})
const invalidBindings = computed(() => Object.values(bindingIssues.value).some(rows => Object.keys(rows).length > 0))
const attributeRows = computed(() => attribute.value?.labels.map(id => ({ id })) ?? [])
const attributeName = (id: string) => `attribute.${id}` in configuredRuleCatalog.en ? String(t(`attribute.${id}`)) : id
const bindingName = (id: string) => `binding.${id}` in configuredRuleCatalog.en ? String(t(`binding.${id}`)) : id
const priceRows = computed(() => {
  if (modal.value !== 'prices' || !modalDraft.value) return []
  const rows = configuredPriceRows(modalDraft.value, props.locale)
  return rows.map((row, index) => ({ ...row, label: rows.filter(item => item.label === row.label).length > 1 ? String(t('oddsNumber', { name: row.label, index: rows.slice(0, index + 1).filter(item => item.label === row.label).length })) : row.label }))
})
const priceColumns = computed(() => [{ key: 'name', label: String(t('priceItem')) }, ...modalDraft.value!.catalog.plates.map(plate => ({ key: plate, label: plate }))])
const addChanged = computed(() => modal.value === 'add' && !!(newId.value || groupNameEn.value || groupNameZh.value || groupNumbers.value || groupPosition.value || groupPush.value || Object.values(groupPrices.value).some(Boolean)))
const modalChanged = computed(() => !!modalDraft.value && (JSON.stringify(modalDraft.value) !== JSON.stringify(draft.value) || bindingsChanged.value || addChanged.value))
function invalidate() { verified.value = null; result.value = null; error.value = ''; remoteError.value = null; generation++ }
watch(modalDraft, invalidate, {deep:true,flush:'sync'})
function openEditor(kind: NonNullable<typeof modal.value>, id = editId.value) {
 if (!draft.value) return
 editId.value = id; modalDraft.value = copy(draft.value); modal.value = kind; tab.value = 'basic'; invalidate()
 if (kind === 'add') { addMode.value = 'copy'; newId.value = ''; groupNameEn.value = ''; groupNameZh.value = ''; groupNumbers.value = ''; groupPosition.value = ''; groupPush.value = ''; groupTarget.value = 'special'; groupPrices.value = {} }
 if (kind === 'bindings') { bindingTexts.value = bindingText(modalDraft.value); attributeId.value = modalDraft.value.catalog.attributes[0]?.id ?? '' }
}
function closeEditor() {
 if (modalChanged.value && !window.confirm(String(t('discardChanges')))) return
 modal.value = null; modalDraft.value = null; invalidate()
}
function applyEditor() {
 if (!modalDraft.value || !editable.value || locked.value) return
 if (modal.value === 'bindings') {
   if (invalidBindings.value) return
   modalDraft.value = { ...modalDraft.value, bindings: { ...modalDraft.value.bindings, attributes: Object.fromEntries(Object.entries(bindingTexts.value).map(([id, rows]) => [id, Object.fromEntries(Object.entries(rows).map(([label, text]) => [label, parseBindingNumbers(text)!]))])) } }
 }
 draft.value = modalDraft.value; modalDraft.value = null; modal.value = null
}
function patchPlay(value: RuleNode) { if (play.value) editPlay({...play.value,...value as object} as unknown as RuleNode) }
function beforeUnload(event: BeforeUnloadEvent) { if (dirty.value || modalChanged.value) { event.preventDefault(); event.returnValue = '' } }
window.addEventListener('beforeunload', beforeUnload)
onUnmounted(() => window.removeEventListener('beforeunload', beforeUnload))

const node = (value: unknown) => value as RuleNode
const copy = <T,>(value: T): T => JSON.parse(JSON.stringify(value)) as T
let generation = 0
onUnmounted(() => { generation++ })
watch(draft, () => { verified.value = null; result.value = null; error.value = ''; remoteError.value = null; generation++ }, { deep: true, flush: 'sync' })
watch([request, regular, special, digits], () => { result.value = null; generation++ }, { deep: true, flush: 'sync' })
watch(() => [props.surface, props.brandId], () => { generation++; draft.value = null; verified.value = null; result.value = null; error.value = '' })
function install(snapshot: ConfiguredSnapshot) {
  savedSnapshot.value = JSON.stringify(snapshot); draft.value = snapshot; editId.value = snapshot.catalog.plays[0]!.id
  request.value = { play_id: editId.value, options: [], plate: snapshot.catalog.plates[0]!, points: snapshot.catalog.plays[0]!.stake_mode === 'unit_multiplier' ? '0' : '10', multiplier: '1' }
}
async function run(action: () => Promise<void>) { busy.value = true; error.value = ''; remoteError.value = null; try { await action() } catch (cause) { if ((cause as { status?: number }).status === 401) { emit('failure', cause); return } const code = (cause as { code?: string }).code; if (code === 'RULE_INVALID' || (!code && cause instanceof Error && !(cause instanceof TypeError))) error.value = 'invalid'; else remoteError.value = i18n.errorMessage(cause, 'read'); } finally { busy.value = false } }
async function load() { if (dirty.value && !window.confirm(String(t('discardChanges')))) return; await run(async () => { const token = generation; const catalog = await api.value.template(templateName.value); if (token !== generation) return;if(props.model&&JSON.stringify(catalog.model)!==JSON.stringify(props.model)){error.value='templateModelMismatch';return;} install(configuredDemo(catalog)); savedSnapshot.value = ''; synthetic.value = true }) }
async function validate() {
  if (!working.value) return
  await run(async () => { const snapshot = copy(working.value!); const token = generation; const response = await api.value.preview(snapshot); if (token !== generation) return; verified.value = snapshot; const selected = snapshot.catalog.plays.find(p => p.id === editId.value)!; request.value = { play_id: selected.id, options: [], plate: snapshot.catalog.plates[0]!, points: selected.stake_mode === 'unit_multiplier' ? '0' : '10', multiplier: '1' }; result.value = response })
}
async function imported(event: Event) {
  const input = event.target as HTMLInputElement, file = input.files?.[0]
  if (!file) return
  await run(async () => { const token = generation; if (file.size > 4 * 1024 * 1024) throw new Error('Draft too large'); const snapshot = parseConfiguredSnapshot(await file.text()); if (token !== generation) return; await api.value.preview(snapshot); if (token !== generation) return; install(snapshot); savedSnapshot.value = ''; synthetic.value = snapshot.prices.version === 'synthetic-prices' })
  input.value = ''
}
function duplicate() {
  if (!modalDraft.value || !play.value || !editable.value || locked.value) return
  try { const next = copy(modalDraft.value); const added = cloneConfiguredPlay(next, play.value, newId.value); modalDraft.value = next; editId.value = added.id; modal.value = 'play'; request.value = { ...request.value, play_id: added.id, options: [] }; newId.value = '' } catch { error.value = 'invalid' }
}
function createGroup() {
  if (!modalDraft.value || !editable.value || locked.value) return
  try {
    const next = copy(modalDraft.value)
    const position = String(groupPosition.value).trim(), push = String(groupPush.value).trim()
    if ((position && !/^\d+$/.test(position)) || (push && !/^\d+$/.test(push))) throw new Error('Invalid number')
    const created = addConfiguredNumberGroup(next, { id: newId.value, label: { en: groupNameEn.value, 'zh-CN': groupNameZh.value }, numbers: parseNumbers(groupNumbers.value), target: groupTarget.value, ...(position ? { position: Number(position) - 1 } : {}), ...(push ? { pushNumber: Number(push) } : {}), prices: copy(groupPrices.value) })
    modalDraft.value = next; editId.value = created.id; modal.value = 'play'; newId.value = ''
  } catch { error.value = 'invalid' }
}
function editPlay(value: RuleNode) { if (!modalDraft.value || !play.value) return; const index = modalDraft.value.catalog.plays.indexOf(play.value); const next = copy(modalDraft.value); next.catalog.plays[index] = value as unknown as ConfiguredPlay; modalDraft.value = next; editId.value = next.catalog.plays[index]!.id }
function editPart(part: 'domain', value: RuleNode) { if (!modalDraft.value) return; const next = copy(modalDraft.value); if (part === 'domain' && play.value) next.catalog.domains[play.value.selection.domain] = value as unknown as ConfiguredSnapshot['catalog']['domains'][string]; modalDraft.value = next }
function editPrice(plate: string, key: string, value: string) {
  if (!modalDraft.value || !editable.value || locked.value) return
  const prices = modalDraft.value.prices
  modalDraft.value = { ...modalDraft.value, prices: { ...prices, plates: { ...prices.plates, [plate]: { ...prices.plates[plate], [key]: value } } } }
}
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
  <component :is="gameId ? 'section' : 'details'" :open="gameId ? undefined : expanded" class="configured-workbench" data-testid="configured-workbench">
    <summary v-if="!gameId">{{ t('title') }}</summary>

    <details v-if="surface === 'platform' || !gameId" class="configured-setup" :open="!draft"><summary>{{ t('draftTools') }}</summary>
    <div class="configured-actions"><label>{{ t('templateChoice') }}<SelectControl v-model="templateName" :disabled="locked" :label="String(t('templateChoice'))" :options="(templates).map((name) => ({ value: name, label: (String(t(`template.${name}`))).trim() }))" /></label><button type="button" class="button button-secondary" :disabled="locked" @click="withGlobalLoading(load)">{{ t('template') }}</button><button type="button" class="button button-secondary" :disabled="locked" @click="fileInput?.click()">{{ t('import') }}</button><input ref="fileInput" type="file" accept="application/json,.json" hidden :disabled="locked" @change="withGlobalLoading(imported, $event)" /></div>
    </details>
    <p v-if="synthetic">{{ t('synthetic') }}</p>
    <p v-if="error" class="message error" role="alert">{{ t(error) }}</p>
    <p v-if="remoteError" class="message error" role="alert">{{ i18n.displayError(remoteError) }}</p>
    <template v-if="draft">
      <div class="rule-toolbar">
        <label class="rule-search"><ClearableInput v-model="search" :locale="locale" :aria-label="t('searchPlays')" :placeholder="t('searchPlays')" /></label>
        <label><SelectControl v-model="groupFilter" :label="String(t('group'))" :options="[{value:'',label:String(t('allGroups'))},...groups.map(value=>({value,label:groupLabel(value)}))]" /></label>
        <div class="configured-actions rule-tools">
          <button v-if="editable" type="button" class="button button-primary" :disabled="locked" @click="openEditor('add')">{{ t('addPlay') }}</button>
          <button type="button" class="button button-secondary" :disabled="locked" @click="openEditor('bindings')">{{ t('bindings') }}</button>
          <button type="button" class="button button-secondary" :disabled="locked" @click="openEditor('prices')">{{ t('prices') }}</button>
        </div>
      </div>
      <ManagementTable :entries="rows" :columns="columns" :empty="!rows.length" :empty-label="String(t('noMatchingPlays'))" table-class="rule-directory">
        <template #name="{entry}"><button type="button" class="row-action" :disabled="locked" @click="openEditor('play',entry.id)">{{ entry.label[locale] }}</button></template>
        <template #group="{entry}">{{ groupLabel(entry.group) }}</template>
        <template #mode="{entry}">{{ t(entry.selection.mode) }}</template>
        <template #count="{entry}">{{ entry.selection.min === entry.selection.max ? entry.selection.min : `${entry.selection.min}–${entry.selection.max}` }}</template>
        <template #action="{entry}"><button type="button" class="button button-secondary" :disabled="locked" @click="openEditor('play',entry.id)">{{ t(editable ? 'configurePlay' : 'viewPlay') }}</button></template>
      </ManagementTable>
      <p v-if="dirty" class="rule-dirty" role="status">{{ t('unsavedRules') }}</p>
    </template>
    <DetailModal class="rule-workbench-modal" :class="{'rule-flat-modal': modal === 'add' || modal === 'bindings' || modal === 'prices'}" v-if="modal && modalDraft" :title="String(modal === 'play' ? play?.label[locale] : t(modal === 'add' ? 'addPlay' : modal))" :close-label="String(t('closeEditor'))" :close-disabled="locked" @close="closeEditor">
      <div class="configured-fields rule-editor" data-testid="rule-editor">
        <p v-if="error" class="message error" role="alert">{{ t(error) }}</p>
        <p v-if="remoteError" class="message error" role="alert">{{ i18n.displayError(remoteError) }}</p>
        <template v-if="modal === 'play' && play">
          <div class="console-tabs" :aria-label="String(t('configurePlay'))">
            <button v-for="item in (['basic','settlement','simulation'] as const)" :key="item" type="button" :class="{selected:tab===item}" :aria-pressed="tab===item" :disabled="locked" @click="tab=item">{{ t(item === 'basic' ? 'basicSelection' : item === 'settlement' ? 'winningSettlement' : 'simulation') }}</button>
          </div>
          <div v-if="tab==='basic'" class="configured-fields">
          <button type="button" class="button button-secondary rule-domain-link" :disabled="locked" @click="modal='domain'">{{ t('domain') }}</button>
          <fieldset class="configured-fields" :disabled="locked || !editable">
            <ConfiguredNodeEditor :model-value="node(basic)" :locale="locale" expanded-objects columns @update:model-value="patchPlay" />
            <details><summary>{{ t('technicalFields') }}</summary><ConfiguredNodeEditor :model-value="node(Object.fromEntries(Object.entries(play).filter(([key])=>['id','source_code'].includes(key))))" :locale="locale" @update:model-value="patchPlay" /></details>
          </fieldset>
          </div>
          <fieldset v-else-if="tab==='settlement'" class="configured-fields" :disabled="locked || !editable"><ConfiguredNodeEditor :model-value="node(settlement)" :locale="locale" columns expanded-objects @update:model-value="patchPlay" /></fieldset>
          <fieldset v-else class="configured-fields" :disabled="locked">
      <button type="button" class="button button-primary" @click="withGlobalLoading(validate)">{{ t('validate') }}</button>
      <p role="status">{{ verified ? t('validated') : t('stale') }}</p>
      <template v-if="verified">
        <h3>{{ t('preview') }}</h3><ConfiguredSelection v-model="request" :snapshot="verified" :locale="locale" />
        <button type="button" class="button button-secondary" @click="withGlobalLoading(calculate, false)">{{ t('quote') }}</button>
        <label>{{ t('regular') }}<input v-model="regular" /></label><label>{{ t('special') }}<input v-model="special" /></label><label>{{ t('digits') }}<input v-model="digits" /></label>
        <button type="button" class="button button-primary" @click="withGlobalLoading(calculate, true)">{{ t('simulate') }}</button>
      </template>
      <dl v-if="result?.quote"><dt>{{ t('entries') }}</dt><dd>{{ result.quote.lines.length }}</dd><dt>{{ t('bet') }}</dt><dd>{{ result.quote.bet_points }}</dd></dl>
      <template v-if="result?.result"><dl><dt>{{ t('prize') }}</dt><dd>{{ result.result.prize_points }}</dd><dt>{{ t('refund') }}</dt><dd>{{ result.result.refund_points }}</dd><dt>{{ t('effective') }}</dt><dd>{{ result.result.effective_points }}</dd></dl><p v-for="(line, index) in result.result.lines" :key="index">{{ index + 1 }} · {{ t(line.outcome) }} · {{ t('prize') }} {{ line.prize_points }} · {{ t('refund') }} {{ line.refund_points }}</p></template>
      <details v-if="result"><summary>{{ t('details') }}</summary><BusinessDetails :value="result" :locale="props.locale" /></details>

          </fieldset>
        </template>
        <fieldset v-else-if="modal==='add'" class="configured-fields" :disabled="locked || !editable">
          <div class="console-tabs" :aria-label="String(t('creationMethod'))">
            <button v-for="method in (['copy','group'] as const)" :key="method" type="button" :class="{selected:addMode===method}" :aria-pressed="addMode===method" @click="addMode=method">{{ t(method === 'copy' ? 'copyPlay' : 'numberGroupPlay') }}</button>
          </div>
          <div class="rule-flat-fields">
            <label v-if="addMode==='copy'" class="rule-full-width">{{ t('copySource') }}<SelectControl v-model="editId" :label="String(t('copySource'))" :options="modalDraft.catalog.plays.map(p=>({value:p.id,label:p.label[locale]}))" /></label>
            <label class="rule-full-width">{{ t('newPlayId') }}<input v-model.trim="newId" /></label>
            <template v-if="addMode==='group'">
              <label>{{ t('nameZh') }}<input v-model="groupNameZh" /></label>
              <label>{{ t('nameEn') }}<input v-model="groupNameEn" /></label>
              <label class="rule-full-width">{{ t('groupNumbers') }}<textarea v-model="groupNumbers" rows="2" /></label>
              <label>{{ t('groupTarget') }}<SelectControl v-model="groupTarget" :label="String(t('groupTarget'))" :options="[{ value: 'special', label: String(t('special')) }, { value: 'regular', label: String(t('regular')) }, { value: 'all', label: String(t('all')) }]" /></label>
              <label>{{ t('groupPosition') }}<input v-model="groupPosition" type="number" min="1" step="1" /></label>
              <label class="rule-full-width">{{ t('groupPush') }}<input v-model="groupPush" type="number" min="0" step="1" /></label>
              <div class="configured-fields rule-full-width">
                <h3 class="rule-field-heading">{{ t('initialOdds') }}</h3>
                <div class="rule-initial-prices">
                  <label v-for="plate in modalDraft.catalog.plates" :key="plate">{{ t('groupPrice', { plate }) }}<input v-model="groupPrices[plate]" inputmode="decimal" /></label>
                </div>
              </div>
            </template>
          </div>
        </fieldset>
        <fieldset v-else-if="modal==='bindings'" class="configured-fields" :disabled="locked">
          <label v-if="modalDraft.catalog.attributes.length">{{ t('attribute') }}<SelectControl v-model="attributeId" :label="String(t('attribute'))" :options="modalDraft.catalog.attributes.map(item=>({value:item.id,label:attributeName(item.id)}))" /></label>
          <ManagementTable :entries="attributeRows" :columns="[{key:'name',label:String(t('group'))},{key:'numbers',label:String(t('boundNumbers'))}]" :empty="!attributeRows.length" :empty-label="String(t('noAttributes'))" table-class="rule-binding-table">
            <template #name="{entry}">{{ bindingName(entry.id) }}</template>
            <template #numbers="{entry}">
              <div class="rule-binding-field">
                <input :aria-label="String(t('bindingNumbersFor', {name:bindingName(entry.id)}))" :readonly="!editable" v-model="bindingTexts[attributeId]![entry.id]" :aria-invalid="!!bindingIssues[attributeId]?.[entry.id]" :aria-describedby="bindingIssues[attributeId]?.[entry.id] ? `binding-error-${entry.id}` : undefined" />
              </div>
              <span v-if="bindingIssues[attributeId]?.[entry.id]" :id="`binding-error-${entry.id}`" class="rule-field-error" role="alert">{{ t(bindingIssues[attributeId]![entry.id]!) }}</span>
            </template>
          </ManagementTable>
          <p v-if="bindingIssues[attributeId]?.['']" class="rule-field-error" role="alert">{{ t('bindingIncomplete') }}</p>
        </fieldset>
        <fieldset v-else-if="modal==='prices'" class="configured-fields" :disabled="locked || !editable">
          <ManagementTable :style="{'--rule-price-columns':Math.min(3,modalDraft.catalog.plates.length)}" :entries="priceRows" :columns="priceColumns" :empty="!priceRows.length" :empty-label="String(t('noPrices'))" table-class="rule-price-table">
            <template #name="{entry}">{{ entry.label }}</template>
            <template v-for="plate in modalDraft.catalog.plates" :key="plate" #[plate]="{entry}">
              <input :value="modalDraft.prices.plates[plate]![entry.id]" @input="editPrice(plate,entry.id,($event.target as HTMLInputElement).value)" inputmode="decimal" :aria-label="String(t('priceFor', {name:entry.label,plate}))" />
            </template>
          </ManagementTable>
        </fieldset>
        <fieldset v-else class="configured-fields" :disabled="locked || !editable">
          <template v-if="modal==='domain' && play">
            <p class="rule-shared-notice" role="status">{{ t('sharedDomainNotice', {plays:modalDraft.catalog.plays.filter(p=>p.selection.domain===play!.selection.domain).map(p=>p.label[locale]).join('、')}) }}</p>
            <ConfiguredNodeEditor :model-value="node(modalDraft.catalog.domains[play.selection.domain])" :locale="locale" expanded-objects @update:model-value="editPart('domain',$event)" />
          </template>
        </fieldset>
      </div>
      <template v-if="editable||modal==='domain'" #footer>
        <button v-if="modal==='domain'" type="button" class="button button-secondary" :disabled="locked" @click="modal='play'">{{ t('backToPlay') }}</button>
        <button v-if="editable && modal==='add'" type="button" class="button button-primary" :disabled="locked" @click="addMode==='copy' ? duplicate() : createGroup()">{{ t('continueSetup') }}</button>
        <button v-else-if="editable" type="button" class="button button-primary" :disabled="locked || !modalChanged || invalidBindings" @click="applyEditor">{{ t('apply') }}</button>
      </template>
    </DetailModal>
    <p v-if="!draft && surface === 'platform'" class="empty-state">{{ t('empty') }}</p>
    <ConfiguredPublishing :class="{'rule-save-bar':surface==='platform'}" v-if="gameId" :surface="surface" :brand-id="brandId" :game-id="gameId" :locale="locale" :snapshot="draft" :can-write="canWrite" @load="install" @locked="publicationLocked=$event" @failure="emit('failure',$event)" />
  </component>
</template>
<style scoped>
.configured-workbench{min-width:0;font-size:14px;display:grid;gap:12px}.configured-workbench:is(details){display:block;border:1px solid var(--line);border-radius:var(--radius);padding:12px}.configured-workbench:is(details)>:not(summary){margin-top:12px}.configured-workbench p:not(.message){margin:0;color:var(--muted);line-height:1.6}.configured-setup>.configured-actions{margin-top:8px}.configured-setup>p{margin-top:8px}.configured-workbench summary{cursor:pointer;min-height:var(--control-height);align-content:center;overflow-wrap:anywhere}.configured-fields{border:0;padding:0;margin:0;display:grid;gap:12px;min-width:0}.configured-fields label{display:grid;gap:6px;min-width:0}.configured-actions{display:flex;flex-wrap:wrap;align-items:end;gap:12px}.configured-actions>*{min-width:0;max-width:100%}.configured-actions input[type=file]{max-width:220px}.configured-fields input, .configured-fields textarea{min-height:var(--control-height);max-width:100%;width:100%;box-sizing:border-box;min-width:0}.configured-fields textarea{font:var(--body-font-size) monospace}.configured-fields>.button{justify-self:start;max-width:100%;white-space:normal;height:auto;min-height:var(--control-height)}.configured-workbench pre{white-space:pre-wrap;overflow-wrap:anywhere;font-size:var(--body-font-size);min-width:0}.configured-workbench dl{display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1fr);gap:8px}.configured-workbench dd{margin:0}
</style>

<style scoped>
.rule-toolbar{display:flex;align-items:end;flex-wrap:wrap;gap:12px}.rule-toolbar>label{display:grid;gap:6px;min-width:0}.rule-search{flex:0 1 260px}.rule-search :deep(.clearable-input){width:100%}.rule-search input{width:100%;min-width:0}.rule-tools{margin-left:auto;gap:8px}.rule-editor{gap:16px}.rule-editor .console-tabs{flex-wrap:wrap}.rule-editor :deep(.configured-node input){min-height:var(--control-height);width:100%;max-width:none}.rule-editor :deep(.select-control){width:100%;max-width:none}.rule-editor :deep(.configured-node summary){min-height:var(--control-height);align-content:center}.rule-dirty{color:var(--warning-text)!important}.rule-domain-link{justify-self:start}.rule-save-bar{position:sticky;bottom:0;z-index:1;padding:12px 0;background:var(--surface);border-top:1px solid var(--line)}.rule-shared-notice{color:var(--ink-secondary)!important}.rule-directory :deep(td){vertical-align:middle}
@media(max-width:620px){.rule-toolbar>label{flex:1 1 100%;width:100%}.rule-tools{margin-left:0;width:100%}.rule-tools>.button{flex:1 1 auto}}
</style>

<style scoped>
.rule-flat-modal.detail-modal{width:min(760px,calc(100vw - 24px));max-width:calc(100vw - 24px)}
.rule-flat-fields{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px;min-width:0}
.rule-full-width{grid-column:1/-1}
.rule-initial-prices{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(100%,120px),1fr));gap:12px}
.rule-field-heading{font-size:var(--body-font-size);font-weight:600;margin:0}
.rule-flat-modal textarea{font-family:var(--font-family);resize:vertical}
.rule-binding-field{width:100%}
.rule-binding-field input{--input-width:100%}
:global(#app .rule-flat-modal .rule-binding-table :is(th,td)){text-align:left}
:global(#app .rule-flat-modal .rule-price-table th:not(:first-child)){text-align:right}
.rule-flat-modal :deep(.rule-price-table input){--input-width:100%}
.rule-field-error{color:var(--danger-text)!important;font-size:var(--body-font-size)}
.rule-flat-modal :deep(.console-table){width:100%;table-layout:fixed}
.rule-flat-modal :deep(.console-table td){white-space:normal;overflow-wrap:anywhere;vertical-align:middle}
.rule-flat-modal :deep(.rule-binding-table th:first-child){width:100px}
.rule-flat-modal :deep(.rule-price-table th:first-child){width:42%}
.rule-flat-modal :deep(.rule-price-table input){text-align:right;font-variant-numeric:tabular-nums}
@media(max-width:620px){.rule-flat-fields{grid-template-columns:minmax(0,1fr)}}
@media(max-width:1080px){
.rule-flat-modal :deep(.console-responsive-table tbody){display:block}
.rule-flat-modal :deep(.console-responsive-table tbody>tr){display:grid;grid-template-columns:repeat(auto-fit,minmax(min(100%,100px),1fr));border:0;border-bottom:1px solid var(--line);border-radius:0;padding:8px 0;gap:8px}
.rule-flat-modal :deep(.console-responsive-table td){padding:0}
.rule-flat-modal :deep(.console-responsive-table td:first-child){grid-column:1/-1}
.rule-flat-modal :deep(.rule-price-table tbody>tr){grid-template-columns:repeat(var(--rule-price-columns),minmax(0,1fr))}
.rule-flat-modal :deep(.rule-price-table td:first-child::before){content:none}
.rule-flat-modal :deep(.console-responsive-table td:last-child){grid-column:auto}
.rule-flat-modal :deep(.rule-binding-table tbody>tr){grid-template-columns:64px minmax(0,1fr)}
.rule-flat-modal :deep(.rule-binding-table td:first-child){grid-column:auto}
.rule-flat-modal :deep(.rule-binding-table td::before){content:none}
}
</style>
