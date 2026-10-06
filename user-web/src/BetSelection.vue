<script setup lang="ts">
import { computed } from "vue";
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

function pool(
  values: number[],
  min: number | undefined,
  max: number | undefined,
  required: boolean,
): number[] {
  if (!required) return [];
  if (Array.isArray(values) && values.length)
    return [...new Set(values)].sort((a, b) => a - b);
  const low = min ?? 0;
  const high = max ?? low;
  return high >= low && high - low <= 9999
    ? Array.from({ length: high - low + 1 }, (_, i) => low + i)
    : [];
}
const regularPool = computed(() =>
  pool(
    props.definition.model.regular_pool?.values ?? [],
    props.definition.model.regular_pool?.min,
    props.definition.model.regular_pool?.max,
    props.definition.selection.regular_count > 0,
  ),
);
const specialPool = computed(() =>
  pool(
    props.definition.model.special_pool?.values ?? [],
    props.definition.model.special_pool?.min,
    props.definition.model.special_pool?.max,
    props.definition.selection.special_count > 0,
  ),
);
const excludePool = computed(() => {
  if (props.definition.model.model === "DIGITS_0_9")
    return Array.from({ length: 10 }, (_, i) => i);
  const regular = props.definition.model.regular_pool;
  const special = props.definition.model.special_pool;
  const regularPresent = Boolean(
    regular?.values?.length ||
      regular?.min !== undefined ||
      regular?.max !== undefined ||
      props.definition.model.regular_count > 0,
  );
  const specialPresent = Boolean(
    special?.values?.length ||
      special?.min !== undefined ||
      special?.max !== undefined ||
      props.definition.model.special_count > 0,
  );
  return [
    ...new Set([
      ...pool(
        regular?.values ?? [],
        regular?.min,
        regular?.max,
        regularPresent,
      ),
      ...pool(
        special?.values ?? [],
        special?.min,
        special?.max,
        specialPresent,
      ),
    ]),
  ].sort((a, b) => a - b);
});
const digitCount = computed(
  () =>
    props.definition.model.length || props.definition.model.total_count || 0,
);
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
}
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
