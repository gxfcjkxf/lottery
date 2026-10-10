<script setup lang="ts">
import { computed, ref } from "vue";
import { emptyBetSelection, exclusionPoolValues, randomBetSelection, rulePoolValues } from "./selection-actions";
import type {
  RuleDefinition,
  RuleTicketSelection,
} from "../../shared/src/rules";

const props = defineProps<{
  definition: RuleDefinition;
  modelValue: RuleTicketSelection;
  locale: "en" | "zh";
  disabled?: boolean;
}>();
const emit = defineEmits<{
  "update:modelValue": [value: RuleTicketSelection];
}>();
const zh = computed(() => props.locale === "zh");
const mode = computed(() => props.definition.selection.mode);
const numberLabels = computed(() => {
  const labels = new Map<number, string[]>();
  for (const [group, values] of Object.entries(props.definition.number_attributes ?? {})) {
    for (const [name, numbers] of Object.entries(values)) {
      for (const number of numbers) {
        const current = labels.get(number) ?? [];
        current.push(`${group}: ${name}`);
        labels.set(number, current);
      }
    }
  }
  return labels;
});
const actionError = ref(false);

function clearSelection() {
  if (props.disabled) return;
  actionError.value = false;
  emit("update:modelValue", emptyBetSelection());
}
function randomSelection() {
  if (props.disabled) return;
  actionError.value = false;
  try { emit("update:modelValue", randomBetSelection(props.definition)); }
  catch { actionError.value = true; }
}

const regularPool = computed(() =>
  props.definition.selection.regular_count > 0 ? rulePoolValues(props.definition.model.regular_pool) : [],
);
const specialPool = computed(() =>
  props.definition.selection.special_count > 0 ? rulePoolValues(props.definition.model.special_pool) : [],
);
const excludePool = computed(() => exclusionPoolValues(props.definition.model));
const digitCount = computed(() => props.definition.model.length);
const digitPool = Array.from({ length: 10 }, (_, i) => i);

function copy(): RuleTicketSelection {
  return {
    regular: [...(props.modelValue.regular ?? [])],
    special: [...(props.modelValue.special ?? [])],
    digits: (props.modelValue.digits ?? []).map((values) => [...values]),
    exclude: [...(props.modelValue.exclude ?? [])],
    attributes: Object.fromEntries(
      Object.entries(props.modelValue.attributes ?? {}).map(([k, v]) => [
        k,
        [...v],
      ]),
    ),
    features: Object.fromEntries(
      Object.entries(props.modelValue.features ?? {}).map(([k, v]) => [
        k,
        [...v],
      ]),
    ),
  };
}
function toggle(field: "regular" | "special" | "exclude", value: number) {
  const next = copy();
  const values = next[field] ?? [];
  next[field] = values.includes(value)
    ? values.filter((v) => v !== value)
    : [...values, value].sort((a, b) => a - b);
  emit("update:modelValue", next);
}
function toggleDigit(position: number, value: number) {
  const next = copy();
  next.digits ??= [];
  while (next.digits.length < digitCount.value) next.digits.push([]);
  const values = next.digits[position] ?? [];
  next.digits[position] = values.includes(value)
    ? values.filter((v) => v !== value)
    : [...values, value].sort((a, b) => a - b);
  emit("update:modelValue", next);
}
function toggleAttribute(group: string, value: string) {
  const next = copy();
  next.attributes ??= {};
  const values = next.attributes[group] ?? [];
  next.attributes[group] = values.includes(value)
    ? values.filter((v) => v !== value)
    : [...values, value];
  emit("update:modelValue", next);
}
function toggleFeature(name: string, value: number) {
  const next = copy();
  next.features ??= {};
  const values = next.features[name] ?? [];
  next.features[name] = values.includes(value)
    ? values.filter((v) => v !== value)
    : [...values, value];
  emit("update:modelValue", next);
}
</script>

<template>
  <div class="selection-editor" :aria-label="zh ? '投注选择' : 'Bet selection'">
    <div class="selection-actions">
      <button type="button" :disabled="disabled" @click="clearSelection">{{ zh ? '清空选号' : 'Clear selection' }}</button>
      <button type="button" :disabled="disabled" @click="randomSelection">{{ zh ? '随机选号' : 'Random selection' }}</button>
    </div>
    <p v-if="actionError" role="alert">{{ zh ? '无法根据当前玩法生成选号，请手动选择。' : 'Unable to generate a selection for this play. Please choose manually.' }}</p>
    <template v-if="mode === 'numbers'">
      <fieldset
        v-if="definition.model.model === 'DIGITS_0_9'"
        class="selection-group"
      >
        <legend>
          {{
            zh
              ? "每个位置可选一个或多个数字"
              : "Choose one or more candidates for each position"
          }}
        </legend>
        <div
          v-for="position in digitCount"
          :key="position"
          class="position-group"
        >
          <strong>{{
            zh ? `位置 ${position}` : `Position ${position}`
          }}</strong>
          <div class="choice-grid">
            <label v-for="value in digitPool" :key="value" class="choice-chip">
              <input
                type="checkbox"
                :aria-label="
                  zh
                    ? `位置 ${position} 选择数字 ${value}`
                    : `Choose digit ${value} for position ${position}`
                "
                :checked="modelValue.digits?.[position - 1]?.includes(value)"
                :disabled="disabled"
                @change="toggleDigit(position - 1, value)"
              />
              <span>{{ value }}</span>
              <small v-if="numberLabels.has(value)" class="number-attributes">{{ numberLabels.get(value)?.join(' · ') }}</small>
            </label>
          </div>
        </div>
      </fieldset>
      <fieldset v-else-if="regularPool.length" class="selection-group">
        <legend>
          {{
            zh
              ? `普通号码（需选 ${definition.selection.regular_count} 个）`
              : `Regular numbers (choose ${definition.selection.regular_count})`
          }}
        </legend>
        <div class="choice-grid">
          <label v-for="value in regularPool" :key="value" class="choice-chip">
            <input
              type="checkbox"
              :aria-label="
                zh ? `选择普通号码 ${value}` : `Choose regular number ${value}`
              "
              :checked="modelValue.regular?.includes(value)"
              :disabled="disabled"
              @change="toggle('regular', value)"
            /><span>{{ value }}</span>
            <small v-if="numberLabels.has(value)" class="number-attributes">{{ numberLabels.get(value)?.join(' · ') }}</small>
          </label>
        </div>
      </fieldset>
      <fieldset v-if="specialPool.length" class="selection-group">
        <legend>
          {{
            zh
              ? `特别号码（需选 ${definition.selection.special_count} 个）`
              : `Special numbers (choose ${definition.selection.special_count})`
          }}
        </legend>
        <div class="choice-grid">
          <label v-for="value in specialPool" :key="value" class="choice-chip">
            <input
              type="checkbox"
              :aria-label="
                zh ? `选择特别号码 ${value}` : `Choose special number ${value}`
              "
              :checked="modelValue.special?.includes(value)"
              :disabled="disabled"
              @change="toggle('special', value)"
            /><span>{{ value }}</span>
            <small v-if="numberLabels.has(value)" class="number-attributes">{{ numberLabels.get(value)?.join(' · ') }}</small>
          </label>
        </div>
      </fieldset>
    </template>
    <fieldset v-else-if="mode === 'exclude'" class="selection-group">
      <legend>
        {{
          zh
            ? `排除号码（需选 ${definition.selection.exclude_count} 个）`
            : `Exclude numbers (choose ${definition.selection.exclude_count})`
        }}
      </legend>
      <div class="choice-grid">
        <label v-for="value in excludePool" :key="value" class="choice-chip">
          <input
            type="checkbox"
            :aria-label="zh ? `排除号码 ${value}` : `Exclude number ${value}`"
            :checked="modelValue.exclude?.includes(value)"
            :disabled="disabled"
            @change="toggle('exclude', value)"
          /><span>{{ value }}</span>
          <small v-if="numberLabels.has(value)" class="number-attributes">{{ numberLabels.get(value)?.join(' · ') }}</small>
        </label>
      </div>
    </fieldset>
    <fieldset v-else-if="mode === 'attributes'" class="selection-group">
      <legend>{{ zh ? "选择号码属性" : "Choose number attributes" }}</legend>
      <div
        v-for="group in definition.selection.attribute_groups ?? []"
        :key="group"
        class="attribute-group"
      >
        <strong>{{ group }}</strong>
        <div class="choice-grid">
          <label
            v-for="value in Object.keys(
              definition.number_attributes?.[group] ?? {},
            )"
            :key="value"
            class="choice-chip"
          >
            <input
              type="checkbox"
              :aria-label="
                zh
                  ? `选择属性 ${group}：${value}`
                  : `Choose attribute ${group}: ${value}`
              "
              :checked="modelValue.attributes?.[group]?.includes(value)"
              :disabled="disabled"
              @change="toggleAttribute(group, value)"
            /><span>{{ value }}</span>
          </label>
        </div>
      </div>
    </fieldset>
    <fieldset v-else-if="mode === 'features'" class="selection-group">
      <legend>{{ zh ? "选择玩法特征" : "Choose feature values" }}</legend>
      <div
        v-for="(values, name) in definition.selection.feature_choices ?? {}"
        :key="name"
        class="attribute-group"
      >
        <strong>{{ name }}</strong>
        <div class="choice-grid">
          <label v-for="value in values" :key="value" class="choice-chip">
            <input
              type="checkbox"
              :aria-label="
                zh
                  ? `选择特征 ${name}：${value}`
                  : `Choose feature ${name}: ${value}`
              "
              :checked="modelValue.features?.[name]?.includes(value)"
              :disabled="disabled"
              @change="toggleFeature(name, value)"
            /><span>{{ value }}</span>
          </label>
        </div>
      </div>
    </fieldset>
    <p v-else role="alert">
      {{
        zh ? "暂不支持此选号模式。" : "This selection mode is not supported."
      }}
    </p>
  </div>
</template>

<style scoped>
.selection-editor {
  display: grid;
  gap: 1rem;
}
.selection-actions { display: flex; flex-wrap: wrap; gap: 0.5rem; }
.selection-actions button { min-height: 44px; padding: 0.5rem 0.9rem; border: 1px solid #dce5dc; border-radius: 10px; background: white; color: var(--brand-primary); cursor: pointer; }
.selection-actions button:disabled { opacity: 0.5; cursor: not-allowed; }
.selection-group {
  min-width: 0;
  margin: 0;
  padding: 1rem;
  border: 1px solid #e1e8e1;
  border-radius: 14px;
}
.selection-group legend {
  padding: 0 0.35rem;
  font-weight: 700;
}
.position-group,
.attribute-group {
  display: grid;
  gap: 0.65rem;
  margin-top: 0.9rem;
}
.choice-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(46px, 1fr));
  gap: 0.5rem;
}
.choice-chip {
  position: relative;
  display: grid;
  place-items: center;
  min-height: 44px;
  border: 1px solid #dce5dc;
  border-radius: 10px;
  cursor: pointer;
  font-variant-numeric: tabular-nums;
  padding: 0.25rem;
  gap: 0.15rem;
}
.number-attributes { max-width: 100%; font-size: 0.65rem; line-height: 1.3; overflow-wrap: anywhere; text-align: center; }
.choice-chip:has(input:checked) {
  border-color: var(--brand-primary);
  background: #eaf2e9;
  color: var(--brand-primary);
  font-weight: 700;
}
.choice-chip input {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  opacity: 0;
  cursor: pointer;
}
.choice-chip:focus-within {
  outline: 3px solid #84b89a;
  outline-offset: 2px;
}
@media (max-width: 420px) {
  .choice-grid {
    grid-template-columns: repeat(auto-fill, minmax(40px, 1fr));
    gap: 0.35rem;
  }
  .selection-group {
    padding: 0.75rem;
  }
}
</style>
