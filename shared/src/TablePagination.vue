<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { withGlobalLoading } from './global-loading'
import type { LocaleCode } from './brand'
import { createCatalogI18n } from './localization'
import { paginationCatalog } from './pagination-catalog'
import SelectControl from './SelectControl.vue'
import SiteIcon from './SiteIcon'

const props = defineProps<{
  page: number; count: number; total: string; locale: LocaleCode; ariaLabel?: string
  pageSize?: number; previousDisabled?: boolean; nextDisabled?: boolean
  previousLabel?: string; nextLabel?: string; goTo?: (page: number) => unknown
  reachablePage?: number; changePageSize: (size: number) => unknown; busy?: boolean;
}>()
const i18n = createCatalogI18n(paginationCatalog, props.locale)
watch(() => props.locale, value => i18n.setLocale(value))
const summary = computed(() => i18n.t(props.count === 1 ? 'summarySingle' : 'summary', { page: props.total === '0' ? 1 : props.page, count: props.count, total: props.total }))
const pageCount = computed(() => props.pageSize ? Math.max(1, Math.ceil(Number(props.total) / props.pageSize)) : 1)
const reachable = computed(() => Math.min(pageCount.value, props.reachablePage ?? pageCount.value))
const pages = computed(() => {
  const selected = new Set([1, props.page - 1, props.page, props.page + 1, pageCount.value])
  const result: Array<number | string> = []
  let previous = 0
  for (const page of [...selected].filter(value => value >= 1 && value <= pageCount.value).sort((a, b) => a - b)) {
    if (previous && page - previous > 1) result.push(`gap-${page}`)
    result.push(page); previous = page
  }
  return result
})
const jump = ref(''), pending = ref(false)
const disabled = computed(() => pending.value || props.busy || Boolean(props.previousDisabled && props.nextDisabled))
const jumpPage = computed(() => /^\d+$/.test(jump.value) ? Number(jump.value) : 0)
const canJump = computed(() => !disabled.value && jumpPage.value >= 1 && jumpPage.value <= reachable.value && jumpPage.value !== props.page)
async function navigate(page: number) {
  if (disabled.value || page === props.page || page < 1 || page > reachable.value || !props.goTo) return
  pending.value = true
  try { await props.goTo(page); jump.value = '' } finally { pending.value = false }
}
async function resize(size: number) {
  if (pending.value || props.busy || size === props.pageSize) return
  pending.value = true
  try { await props.changePageSize(size); jump.value = '' } finally { pending.value = false }
}
</script>

<template>
  <nav v-if="count > 0 && total !== '0'" class="table-pagination" :aria-label="ariaLabel ?? i18n.t('navigation')">
    <span class="table-pagination-summary"><span class="table-pagination-desktop">{{ summary }}</span><span class="table-pagination-mobile">{{ i18n.t('total', { total }) }}</span></span>
    <div v-if="goTo && pageSize" class="table-pagination-actions">
      <div class="table-pagination-pages">
        <button type="button" class="button button-secondary table-pagination-mobile" :aria-label="i18n.t('first')" :disabled="disabled || page <= 1" @click="withGlobalLoading(navigate, 1)"><SiteIcon name="first" /></button>
        <button type="button" class="button button-secondary table-pagination-arrow" :aria-label="previousLabel ?? i18n.t('previous')" :disabled="pending || previousDisabled || page <= 1" @click="withGlobalLoading(navigate, page - 1)"><SiteIcon name="back" /></button>
        <template v-for="item in pages" :key="item">
          <button v-if="typeof item === 'number'" type="button" class="button table-pagination-page" :class="item === page ? 'button-primary is-current' : 'button-secondary'" :aria-label="i18n.t('page', { page: item })" :aria-current="item === page ? 'page' : undefined" :disabled="disabled || item > reachable" @click="item !== page && withGlobalLoading(navigate, item)">{{ item }}</button>
          <span v-else class="table-pagination-gap" aria-hidden="true">…</span>
        </template>
        <button type="button" class="button button-secondary table-pagination-arrow" :aria-label="nextLabel ?? i18n.t('next')" :disabled="pending || nextDisabled || page >= pageCount" @click="withGlobalLoading(navigate, page + 1)"><SiteIcon name="next" /></button>
        <button type="button" class="button button-secondary table-pagination-mobile" :aria-label="i18n.t('last')" :disabled="disabled || nextDisabled || page >= pageCount || pageCount > reachable" @click="withGlobalLoading(navigate, pageCount)"><SiteIcon name="last" /></button>
      </div>
      <div class="table-pagination-jump" @keydown.enter.prevent="canJump && withGlobalLoading(navigate, jumpPage)">
        <label><span>{{ i18n.t('jump') }}</span><input :value="jump" @input="jump = ($event.target as HTMLInputElement).value" class="table-pagination-input" type="text" inputmode="numeric" @keydown.enter.prevent="canJump && withGlobalLoading(navigate, jumpPage)" :aria-label="i18n.t('jumpPage')" :disabled="disabled" /><span>{{ i18n.t('pageUnit') }}</span></label>
        <button type="button" class="button button-secondary" :disabled="!canJump" @click="canJump && withGlobalLoading(navigate, jumpPage)">{{ i18n.t('go') }}</button>
      </div>
      <SelectControl class="table-pagination-size" :model-value="pageSize" :options="[10, 20, 50, 100].map(size => ({ value: size, label: i18n.t('pageSize', { count: size }) }))" :label="i18n.t('pageSizeLabel')" :disabled="pending || busy" @change="(size) => withGlobalLoading(resize, Number(size))" />
    </div>
  </nav>
</template>

<style scoped>
nav.table-pagination { display:flex; align-items:center; justify-content:space-between; flex-wrap:wrap; gap:12px; min-width:0; margin:0; padding:8px 12px; border-top:1px solid var(--line); background:transparent; color:var(--muted); font-size:var(--body-font-size); line-height:1.5; }
.table-pagination > .table-pagination-summary { flex:1 1 auto; min-width:0; margin:0; color:var(--muted); font-size:var(--body-font-size); line-height:1.5; text-align:start; overflow-wrap:anywhere; }
.table-pagination > .table-pagination-actions, .table-pagination-pages, .table-pagination-jump, .table-pagination-jump label { display:flex; align-items:center; flex-wrap:wrap; gap:6px; min-width:0; }
.table-pagination > .table-pagination-actions { justify-content:flex-end; gap:12px; }
.table-pagination .button { flex:0 0 auto; min-width:var(--control-height); height:var(--control-height); padding:4px 8px; font-size:var(--body-font-size); font-weight:400; font-variant-numeric:tabular-nums; }
.table-pagination .is-current { background:var(--primary); border-color:var(--primary); color:var(--on-primary); font-weight:650; }
.table-pagination-gap { padding:0 2px; }
.table-pagination-jump { margin:0; }
nav.table-pagination .table-pagination-jump label { display:flex; flex-direction:row; align-items:center; flex-wrap:nowrap; gap:6px; width:auto; margin:0; white-space:nowrap; }
#app .table-pagination .table-pagination-input { --input-width:52px; box-sizing:border-box; width:52px; min-width:0; height:var(--control-height); margin:0; padding:4px 6px; border:1px solid var(--line); border-radius:var(--radius-sm); background:var(--surface); color:var(--ink); font-size:var(--body-font-size); text-align:center; }
nav.table-pagination .table-pagination-mobile { display:none!important; }
@media(max-width:640px), (hover:none) and (pointer:coarse) {
  nav.table-pagination { align-items:center; flex-wrap:nowrap; gap:4px; padding:6px 4px; }
  .table-pagination > .table-pagination-actions { flex:0 0 auto; width:auto; justify-content:flex-start; gap:0; }
  .table-pagination-pages { width:auto; max-width:100%; justify-content:flex-start; gap:4px; flex-wrap:nowrap; }
  nav.table-pagination .table-pagination-mobile { display:inline-flex!important; }
  nav.table-pagination .table-pagination-desktop,
  .table-pagination-page:not(.is-current), .table-pagination-gap,
  .table-pagination-jump, .table-pagination-size { display:none!important; }
  .table-pagination .is-current { white-space:nowrap; }
  .table-pagination .button { width:44px; min-width:44px; padding:4px; }
}
@media(max-width:900px), (pointer:coarse) { #app .table-pagination .table-pagination-input { --input-width:52px; font-size:16px; } }
@media(min-width:901px) and (pointer:fine) { nav.table-pagination { padding:6px 12px; gap:8px; } }
</style>
