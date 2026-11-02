<script setup lang="ts">
import { computed } from 'vue';
import PlatformTimeInput from './PlatformTimeInput.vue';
import {platformDisplaySettings} from './display-settings';
import { formatDisplayDateTime } from './display-settings';
import { archiveScheduleCopy } from './archive-schedule-catalog';
import type { ArchiveSchedule, ArchiveNextRuns } from './archive-schedule';
type Model = ArchiveSchedule & { daily_enabled: boolean; monthly_enabled: boolean };
const props = defineProps<{ modelValue: Model; locale: 'en' | 'zh-CN'; disabled?: boolean; readonly?: boolean; nextRuns?: ArchiveNextRuns; timezone?: string }>();
const emit = defineEmits<{ 'update:modelValue': [value: Model] }>();
const copy = computed(() => archiveScheduleCopy(props.locale));
function update(key: keyof Model, event: Event) {
 const input = event.target as HTMLInputElement;
 const value = input.type === 'checkbox' ? input.checked : input.type === 'number' ? input.valueAsNumber : input.value;
 emit('update:modelValue', { ...props.modelValue, [key]: value });
}
function updateTime(kind:'daily'|'monthly',value:string){emit('update:modelValue',{...props.modelValue,[`${kind}_execution_time`]:value})}
function runTime(value: string) { return formatDisplayDateTime(value,props.locale); }

</script>
<template>
 <div class="archive-schedule-grid">
  <fieldset v-for="kind in (['daily', 'monthly'] as const)" :key="kind" class="archive-schedule-card">
   <legend>{{ copy[kind] }}</legend>
   <label class="archive-schedule-toggle"><span>{{ copy.enabled }}</span><input :data-testid="`archive-policy-${kind}`" type="checkbox" :checked="modelValue[`${kind}_enabled`]" :disabled="disabled || readonly" @change="update(`${kind}_enabled`, $event)" /></label>
   <div class="archive-schedule-fields">
    <label v-if="kind === 'monthly'"><span>{{ copy.executionDay }}</span><input data-testid="archive-policy-monthly-day" type="number" min="1" max="31" step="1" :value="modelValue.monthly_execution_day" :disabled="disabled || readonly" @input="update('monthly_execution_day', $event)" /></label>
    <div v-if="kind === 'daily'" class="archive-schedule-placeholder" aria-hidden="true"></div>
    <label><span>{{ copy.executionTime }}</span><PlatformTimeInput :data-testid="`archive-policy-${kind}-time`" :model-value="modelValue[`${kind}_execution_time`]" :display-settings="platformDisplaySettings!" :locale="locale" :label="`${copy[kind]} · ${copy.executionTime}`" :disabled="disabled || readonly" minute-precision @update:model-value="updateTime(kind, $event)" /></label>
    <label><span>{{ kind === 'daily' ? copy.dailyOffset : copy.monthlyOffset }}</span><input :data-testid="`archive-policy-${kind}-offset`" type="number" min="1" :max="kind === 'daily' ? 365 : 120" step="1" :value="modelValue[`${kind}_period_offset`]" :disabled="disabled || readonly" @input="update(`${kind}_period_offset`, $event)" /></label>
   </div>
   <dl v-if="nextRuns" class="archive-schedule-next">
    <dt>{{ copy.nextRun }}</dt><dd>{{ nextRuns[kind] ? runTime(nextRuns[kind]!.execute_at) : copy.disabled }}</dd>
    <template v-if="nextRuns[kind]"><dt>{{ copy.target }}</dt><dd>{{ nextRuns[kind]!.period_key }}</dd></template>
   </dl>
  </fieldset>
 </div>
</template>
<style scoped>
.archive-schedule-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px;min-width:0}
.archive-schedule-card{min-width:0;margin:0;padding:12px;border:1px solid var(--line);border-radius:var(--radius-md,8px)}
.archive-schedule-card legend{padding:0 4px;font-weight:600}
.archive-schedule-toggle{display:grid;grid-template-columns:minmax(0,2fr) minmax(0,3fr);align-items:center;gap:12px;margin-bottom:8px;min-height:var(--control-height)}
.archive-schedule-toggle input{width:18px;height:18px;flex:none}
.archive-schedule-fields{display:grid;gap:8px}
.archive-schedule-fields label{display:grid;grid-template-columns:minmax(0,2fr) minmax(0,3fr);align-items:center;gap:12px;min-width:0}.archive-schedule-fields label>span,.archive-schedule-toggle>span{min-width:0;overflow-wrap:anywhere}
.archive-schedule-fields input{box-sizing:border-box;width:100%;min-width:0;height:var(--control-height);color:var(--ink);background:var(--surface);border:1px solid var(--line);border-radius:var(--radius-sm,6px);padding:0 8px;font:inherit}
.archive-schedule-fields input:focus-visible{outline:2px solid var(--primary);outline-offset:2px}
.archive-schedule-placeholder{min-height:var(--control-height)}
.archive-schedule-next{display:grid;grid-template-columns:auto minmax(0,1fr);gap:4px 8px;margin:12px 0 0;padding-top:8px;border-top:1px solid var(--line)}
.archive-schedule-next dt{color:var(--muted)}.archive-schedule-next dd{margin:0;overflow-wrap:anywhere}
@media(max-width:700px){.archive-schedule-grid{grid-template-columns:minmax(0,1fr)}.archive-schedule-placeholder{display:none}}
</style>
