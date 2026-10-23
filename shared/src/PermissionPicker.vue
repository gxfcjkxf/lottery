<script setup lang="ts">
import {computed, watch} from 'vue'
import {createCatalogI18n} from './localization'
import {permissionActionLabel, permissionGroupLabel, permissionLabel} from './permission-labels'
import {permissionPickerCatalog} from './permission-picker'

const props = defineProps<{options: string[]; locale: 'en' | 'zh-CN'}>()
const model = defineModel<string[]>({required: true})
const i18n = createCatalogI18n(permissionPickerCatalog, props.locale)
watch(() => props.locale, value => i18n.setLocale(value))
const t = i18n.t
const groups = computed(() => {
  const result = new Map<string, {key: string; label: string; permissions: string[]}>()
  for (const permission of props.options) {
    const key = permission.split('.')[0]
    if (!result.has(key)) result.set(key, {key, label: permissionGroupLabel(permission, props.locale), permissions: []})
    result.get(key)!.permissions.push(permission)
  }
  return [...result.values()]
})
const selected = computed(() => new Set(model.value))
function count(permissions: string[]) { return permissions.filter(key => selected.value.has(key)).length }
function toggle(permissions: string[]) {
  const next = new Set(model.value)
  if (count(permissions) === permissions.length) permissions.forEach(key => next.delete(key))
  else permissions.forEach(key => next.add(key))
  model.value = [...next]
}
</script>

<template>
  <section class="permission-picker" :aria-label="t('permissions')">
    <div class="permission-picker-toolbar">
      <p>{{t('permissions')}} · {{model.length}} / {{options.length}}</p>
      <div class="permission-picker-actions">
        <button type="button" class="button button-secondary" @click="model = [...options]">{{t('selectAll')}}</button>
        <button type="button" class="button button-secondary" @click="model = []">{{t('clear')}}</button>
      </div>
    </div>
    <div class="permission-groups">
      <section v-for="group in groups" :key="group.key" class="permission-group" :aria-label="group.label">
        <h3><label class="permission-group-toggle">
          <input type="checkbox" :aria-label="group.label" :checked="count(group.permissions) === group.permissions.length" :indeterminate.prop="count(group.permissions) > 0 && count(group.permissions) < group.permissions.length" @change="toggle(group.permissions)" />
          <span>{{group.label}}</span><small>{{count(group.permissions)}} / {{group.permissions.length}}</small>
        </label></h3>
        <div class="permission-group-items">
          <label v-for="key in group.permissions" :key="key" class="permission-option">
            <input v-model="model" type="checkbox" :value="key" :aria-label="permissionLabel(key, locale)" />
            <span>{{permissionActionLabel(key, locale)}}</span>
          </label>
        </div>
      </section>
    </div>
    <p v-if="!options.length" class="empty-state">{{t('empty')}}</p>
  </section>
</template>

<style scoped>
.permission-picker { min-width: 0; }
.permission-picker-toolbar,.permission-picker-actions { display:flex; align-items:center; flex-wrap:wrap; gap:8px; }
.permission-picker-toolbar { justify-content:space-between; margin-bottom:12px; }
.permission-picker-toolbar p { margin:0; font-size:13px; }
.permission-picker button { min-height:44px; }
.permission-groups { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:12px; }
.permission-group { min-width:0; border:1px solid var(--line); border-radius:var(--radius-sm); background:var(--surface); padding:8px 12px; }
.permission-group h3 { margin:0; font-size:14px; }
.permission-picker label { display:flex; flex-direction:row; align-items:center; gap:8px; min-width:0; min-height:44px; margin:0; flex:0 1 auto; cursor:pointer; font-size:13px; color:var(--ink); overflow-wrap:anywhere; }
.permission-picker .permission-group-toggle { width:100%; font-size:14px; }
.permission-group-toggle small { margin-left:auto; color:var(--muted); font-size:12px; font-weight:400; white-space:nowrap; }
.permission-picker input[type=checkbox] { width:18px; height:18px; min-height:18px; margin:0; flex:0 0 auto; accent-color:var(--primary); }
.permission-group-items { display:flex; flex-wrap:wrap; column-gap:16px; border-top:1px solid var(--line); }
.permission-option { font-size:13px; font-weight:400; color:var(--ink); }
@media(max-width:720px) { .permission-groups { grid-template-columns:minmax(0,1fr); } }
</style>
