<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useOperationToast, vWriteGuard, vInlineValidation, withGlobalLoading, vGlobalLoading, vRefreshButton } from '@lottery/shared'
import DetailModal from '@lottery/shared/DetailModal.vue'
import { AdminApiError, createIdempotencyKey, type AdminAccount } from './admin-api'
import { brandPermissionSet } from './brand-permissions'
import { useAdminI18n } from './i18n'
import { createBetLimitsApi, validBetLimits, limitFields, type BetLimitGame, type BetLimits } from './bet-limits-api'

const props = defineProps<{ account: AdminAccount; brandId: string; embedded?: boolean }>()
const emit = defineEmits<{ (event: 'session-invalid'): void }>()
const { t, errorMessage, displayError } = useAdminI18n()
const api = createBetLimitsApi()
const rights = computed(() => brandPermissionSet(props.account, props.brandId))
const canView = computed(() => rights.value.has('bet_policy.view.brand'))
const canWrite = computed(() => rights.value.has('bet_policy.write.brand'))
const games = ref<BetLimitGame[]>([])
const selected = ref<BetLimitGame | null>(null)
const record = ref<BetLimits | null>(null)
const draft = ref({ min_points: '', max_points: '', max_period_points: '', max_user_period_points: '' })
const loadingGames = ref(false), loadingLimits = ref(false), saving = ref(false)
const catalogFailed = ref(false), readFailed = ref(false)
const pending = ref<{ body: BetLimits; key: string } | null>(null)
const error = ref<string | import('@lottery/shared').LocalizedMessage>('')
const notice = ref('')
const fieldLabels = {
  min_points: 'admin.copy.3890e98a', max_points: 'admin.copy.1083c018',
  max_period_points: 'admin.copy.1455f7ed', max_user_period_points: 'admin.copy.46ccdedd',
} as const
const valid = computed(() => !!record.value && validBetLimits({ ...record.value, ...draft.value }))
let generation = 0, readSequence = 0

function report(cause: unknown) {
  if (cause instanceof AdminApiError && cause.status === 401) { emit('session-invalid'); return }
  error.value = errorMessage(cause) || t('admin.limits.failed')
}
async function loadGames() {
  if (!canView.value) return
  const ticket = generation
  loadingGames.value = true; catalogFailed.value = false; error.value = ''
  try { const value = await api.games(props.brandId); if (ticket === generation) games.value = value }
  catch (cause) { if (ticket === generation) { catalogFailed.value = true; report(cause) } }
  finally { if (ticket === generation) loadingGames.value = false }
}
async function loadLimits(preserveDraft = false) {
  if (!selected.value) return
  const game = selected.value.id, ticket = generation, read = ++readSequence
  loadingLimits.value = true; readFailed.value = false; error.value = ''
  try {
    const value = await api.read(props.brandId, game)
    if (ticket !== generation || read !== readSequence) return
    record.value = value
    if (!preserveDraft) for (const key of limitFields) draft.value[key] = value[key]
  } catch (cause) {
    if (ticket === generation && read === readSequence) {
      readFailed.value = true
      if (cause instanceof AdminApiError && cause.status === 404) error.value = t('admin.limits.unconfigured')
      else report(cause)
    }
  } finally { if (ticket === generation && read === readSequence) loadingLimits.value = false }
}
function openGame(game: BetLimitGame) {
  selected.value = game; record.value = null; notice.value = ''; error.value = ''
  draft.value = { min_points: '', max_points: '', max_period_points: '', max_user_period_points: '' }
  void withGlobalLoading(loadLimits)
}
function closeDialog() {
  if (saving.value || pending.value) return
  readSequence++; selected.value = null; record.value = null; loadingLimits.value = false; error.value = ''
}
async function save() {
  if (saving.value || !canWrite.value || readFailed.value || loadingLimits.value) return
  if (!pending.value) {
    if (!record.value || !valid.value) return
    pending.value = { body: { ...record.value, ...draft.value }, key: createIdempotencyKey() }
  }
  const request = pending.value, ticket = generation
  saving.value = true; error.value = ''; notice.value = ''
  try {
    const saved = await api.save(request.body, request.key)
    if (ticket !== generation) return
    record.value = saved; pending.value = null
    for (const key of limitFields) draft.value[key] = saved[key]
    games.value = games.value.map(game => game.id === saved.game_id ? { ...game, configured: true } : game)
    readSequence++; selected.value = null; record.value = null
    notice.value = t('admin.limits.saved')
  } catch (cause) {
    if (ticket !== generation) return
    if (cause instanceof AdminApiError && cause.status >= 400 && cause.status < 500) {
      pending.value = null
      if (cause.status === 409) {
        await loadLimits(true)
        error.value = t('admin.limits.conflict')
      } else report(cause)
    } else { error.value = t('admin.limits.unknown') }
  } finally { if (ticket === generation) saving.value = false }
}
watch(() => [props.brandId, props.account], () => {
  generation++; readSequence++; selected.value = null; record.value = null; pending.value = null
  games.value = []; loadingGames.value = false; loadingLimits.value = false; saving.value = false
  catalogFailed.value = false; readFailed.value = false; error.value = ''; notice.value = ''
  if (canView.value) void withGlobalLoading(loadGames)
}, { deep: true, immediate: true, flush: 'sync' })
onUnmounted(() => { generation++; readSequence++ })
useOperationToast(() => error.value, value => typeof value === 'string' ? value : displayError(value), 'error')
useOperationToast(() => notice.value, value => value, 'success')
</script>

<template>
  <section class="bet-policy-settings" :class="{ 'brand-settings-panel': embedded }" data-testid="bet-policy-settings">
    <header v-if="!embedded" class="policy-header"><h2>{{ t('admin.console.betPolicies') }}</h2></header>
    <p v-if="!canView" class="muted" role="status">{{ t('admin.copy.aebcd8b6') }}</p>
    <section v-else class="policy-card">
      <p v-global-loading v-if="loadingGames" class="muted" role="status">{{ t('admin.copy.4d221807') }}</p>
      <button v-refresh-button="t('admin.copy.3dc463ff')" v-else-if="catalogFailed" class="button button-secondary" type="button" @click="withGlobalLoading(loadGames)">{{ t('admin.copy.3dc463ff') }}</button>
      <p v-else-if="!games.length" class="muted empty-state" role="status">{{ t('admin.limits.empty') }}</p>
      <ul v-else class="game-tags">
        <li v-for="game in games" :key="game.id" class="game-tag" :class="{ 'is-configured': game.configured }">
          <h3>{{ game.name }}</h3>
          <div class="game-tag-footer">
            <span class="game-status">{{ t(game.configured ? 'admin.limits.configured' : 'admin.limits.notConfigured') }}</span>
            <button class="button button-secondary" type="button" aria-haspopup="dialog" :aria-label="t('admin.limits.gameTitle', { name: game.name })" @click="openGame(game)">{{ t(canWrite ? (game.configured ? 'admin.limits.edit' : 'admin.limits.configure') : 'admin.limits.view') }}</button>
          </div>
        </li>
      </ul>
    </section>
    <DetailModal v-if="selected" class="game-limits-dialog" :title="t('admin.limits.gameTitle', { name: selected.name })"
      :close-label="t('association.close')" :close-disabled="saving || !!pending" @close="closeDialog">
      <p v-global-loading v-if="loadingLimits" class="muted" role="status">{{ t('admin.copy.739dd9f0') }}</p>
      <div v-else-if="readFailed" class="read-recovery">
        <p class="muted" role="status">{{ typeof error === 'string' ? error : displayError(error) }}</p>
        <button v-refresh-button="t('admin.copy.3dc463ff')" class="button button-secondary" type="button" @click="withGlobalLoading(loadLimits, !!record)">{{ t('admin.copy.3dc463ff') }}</button>
      </div>
      <form v-else-if="record" v-write-guard="saving" v-inline-validation class="editor-grid ui-form" novalidate @submit.prevent="withGlobalLoading(save)">
        <label v-for="key in limitFields" :key="key">{{ t(fieldLabels[key]) }}
          <input v-model.trim="draft[key]" inputmode="decimal" autocomplete="off" :disabled="!canWrite || saving || !!pending" />
        </label>
        <p v-if="!valid" class="field-error wide" role="alert">{{ t('admin.limits.invalid') }}</p>
        <p v-if="pending && !saving" class="muted wide" role="status">{{ t('admin.limits.unknown') }}</p>
        <div v-if="canWrite" class="action-row wide ui-form-actions">
          <button class="button button-primary" type="submit" :disabled="saving || (!pending && !valid)">
            {{ saving ? t('admin.copy.2462134b') : pending ? t('admin.copy.c6a6aa0c') : t('admin.copy.cce06086') }}
          </button>
        </div>
      </form>
    </DetailModal>
  </section>
</template>

<style scoped>
.bet-policy-settings { min-width: 0; width: 100%; color: var(--ink); overflow-wrap: anywhere; }
.policy-header { margin-bottom: 12px; }
.policy-header h2 { margin: 0; font-size: 24px; }
.policy-card { min-width: 0; padding: 12px; background: var(--surface); border: 1px solid var(--line); border-radius: var(--radius); box-shadow: var(--shadow-sm); }
.game-tags { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 200px), 1fr)); gap: 8px; margin: 0; padding: 0; list-style: none; }
.game-tag { min-width: 0; display: flex; flex-direction: column; gap: 12px; padding: 12px; border: 1px solid var(--line); border-radius: var(--radius-sm); background: var(--canvas); }
.game-tag.is-configured { background: var(--success-bg); border-color: color-mix(in srgb, var(--success-text) 24%, var(--line)); }
.game-tag h3 { margin: 0; font-size: var(--body-font-size); font-weight: 600; line-height: 1.5; }
.game-tag-footer { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-top: auto; }
.game-status { color: var(--muted); font-size: var(--body-font-size); }
.is-configured .game-status { color: var(--success-text); }
.game-tag .button { flex-shrink: 0; }
.game-limits-dialog { width: min(680px, calc(100vw - 24px)); }
.editor-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; min-width: 0; }
.editor-grid label { display: grid; gap: 6px; min-width: 0; font-size: var(--body-font-size); font-weight: 600; }
.editor-grid input { width: 100%; min-width: 0; box-sizing: border-box; }
.wide { grid-column: 1 / -1; }
.muted { margin: 0; color: var(--muted); }
.field-error { margin: 0; color: var(--danger); }
.action-row { display: flex; justify-content: flex-end; }
.read-recovery { display: grid; justify-items: start; gap: 12px; }
@media (max-width: 620px) { .editor-grid { grid-template-columns: minmax(0, 1fr); } }
</style>
