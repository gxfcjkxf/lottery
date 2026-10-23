<script setup lang="ts">
import { computed, ref, useId, watch } from "vue";
import type { RuleCondition, RuleDefinition } from "./rule-simulation-api";
import { adminKey, useAdminI18n } from "./i18n";

defineOptions({ name: "RuleConditionEditor" });
const { t, display } = useAdminI18n();
const fieldLabel = (field: string) => fieldLabels[field] ? t(adminKey(fieldLabels[field]!)) : display(field);
const targetLabel = (target: string) => targetLabels[target] ? t(adminKey(targetLabels[target]!)) : display(target);
const operatorLabel = (operator: string) => {
  const keys: Record<string, string> = { equals: "admin.rule.condition.operator.equals", in: "admin.rule.condition.operator.in", between: "admin.rule.condition.operator.between", selected: "admin.rule.condition.operator.selected" };
  return keys[operator] ? t(adminKey(keys[operator]!)) : display(operator);
};
const operatorErrorEnglish: Record<string, string> = {
  "A value is required.": 'admin.copy.58160b07',
  "Enter a whole number from 0 to 20,000,000.": 'admin.copy.15042eb9',
  "Enter 1–100 unique whole numbers from 0 to 20,000,000, separated by commas.": 'admin.copy.28a8ca65',
  "Both minimum and maximum are required.": 'admin.copy.b79d9f0c',
  "Enter whole numbers from 0 to 20,000,000 for both bounds.": 'admin.copy.a96b4c67',
};

const props = withDefaults(
  defineProps<{
    modelValue: RuleCondition;
    definition: RuleDefinition;
    disabled?: boolean;
    depth?: number;
    totalNodes?: number;
  }>(),
  { disabled: false, depth: 1, totalNodes: 1 },
);

const emit = defineEmits<{
  (event: "update:modelValue", value: RuleCondition): void;
  (event: "validity", value: boolean): void;
}>();

const MAX_DEPTH = 8;
const MAX_NODES = 128;
const MAX_VALUE = 20_000_000;
const fieldLabels: Record<string, string> = {
  regular_match: "admin.rule.condition.field.regular", special_match: "admin.rule.condition.field.special", position_match: "admin.rule.condition.field.position",
  excluded_match: "admin.rule.condition.field.excluded", draw_sum: "admin.rule.condition.field.sum", draw_odd_count: "admin.rule.condition.field.odd",
  draw_even_count: "admin.rule.condition.field.even", draw_unique_count: "admin.rule.condition.field.unique", draw_all_same: "admin.rule.condition.field.same",
  draw_first_last_same: "admin.rule.condition.field.firstLast", draw_span: "admin.rule.condition.field.span", draw_consecutive: "admin.rule.condition.field.consecutive",
  draw_digit: "admin.rule.condition.field.digit", draw_parity: "admin.rule.condition.field.parity", attribute_match: "admin.rule.condition.field.attribute",
};
const targetLabels: Record<string, string> = {
  all: "admin.rule.condition.target.all", regular: "admin.rule.condition.target.regular", special: "admin.rule.condition.target.special", digits: "admin.rule.condition.target.digits",
};
const id = useId();
const equalsDraft = ref("");
const listDraft = ref("");
const minDraft = ref("");
const maxDraft = ref("");
let childSequence = 0;
const nextChildKey = () => `${id}-child-${++childSequence}`;
const childKeys = ref(
  (props.modelValue.children ?? []).map(() => nextChildKey()),
);
const childValidity = ref<Record<string, boolean>>({});

type Operator = RuleCondition["op"];
type Field = NonNullable<RuleCondition["field"]>;
type Target = NonNullable<RuleCondition["target"]>;

const isLogic = computed(() =>
  ["all", "any", "not"].includes(props.modelValue.op),
);
const isDigitModel = computed(
  () => props.definition.model.model === "DIGITS_0_9",
);
const nodeCount = (condition: RuleCondition): number =>
  1 +
  (condition.children ?? []).reduce((sum, child) => sum + nodeCount(child), 0);
const currentTreeNodes = computed(() =>
  Math.max(props.totalNodes, nodeCount(props.modelValue)),
);
function listValues(raw: string): number[] | null {
  const tokens = raw.split(",").map((part) => part.trim());
  if (!raw.trim() || tokens.length > 100 || tokens.some((token) => !token))
    return null;
  const values = tokens.map(parseCanonicalNumber);
  if (values.some((value) => value === null)) return null;
  const parsed = values as number[];
  return new Set(parsed).size === parsed.length ? parsed : null;
}

const numberError = computed(() => {
  if (isLogic.value || props.modelValue.op === "selected") return "";
  if (props.modelValue.op === "equals") {
    if (!equalsDraft.value.trim()) return "A value is required.";
    if (parseCanonicalNumber(equalsDraft.value) === null)
      return "Enter a whole number from 0 to 20,000,000.";
  } else if (props.modelValue.op === "in") {
    if (!listValues(listDraft.value))
      return "Enter 1–100 unique whole numbers from 0 to 20,000,000, separated by commas.";
  } else if (props.modelValue.op === "between") {
    if (!minDraft.value.trim() || !maxDraft.value.trim())
      return "Both minimum and maximum are required.";
    if (
      parseCanonicalNumber(minDraft.value) === null ||
      parseCanonicalNumber(maxDraft.value) === null
    ) {
      return "Enter whole numbers from 0 to 20,000,000 for both bounds.";
    }
  }
  return "";
});
const isValid = computed(
  () =>
    !numberError.value &&
    (props.modelValue.children ?? []).every((_, index) => {
      const key = childKeys.value[index];
      return key !== undefined && childValidity.value[key] === true;
    }),
);
const canAddChild = computed(
  () =>
    !props.disabled &&
    props.modelValue.op !== "not" &&
    (props.modelValue.children?.length ?? 0) < 32 &&
    props.depth < MAX_DEPTH &&
    currentTreeNodes.value < MAX_NODES,
);
const canRemoveChild = computed(
  () => !props.disabled && (props.modelValue.children?.length ?? 0) > 1,
);

function cloneCondition(condition: RuleCondition): RuleCondition {
  return JSON.parse(JSON.stringify(condition)) as RuleCondition;
}

function emitReplacement(condition: RuleCondition) {
  if (props.disabled) return;
  if (!condition.children) {
    childKeys.value = [];
    childValidity.value = {};
  } else {
    childKeys.value = childKeys.value.slice(0, condition.children.length);
    while (childKeys.value.length < condition.children.length)
      childKeys.value.push(nextChildKey());
    const liveKeys = new Set(childKeys.value);
    childValidity.value = Object.fromEntries(
      Object.entries(childValidity.value).filter(([key]) => liveKeys.has(key)),
    );
  }
  if (condition.op !== props.modelValue.op) syncDrafts(condition);
  emit("update:modelValue", cloneCondition(condition));
}

function syncDrafts(condition: RuleCondition) {
  equalsDraft.value =
    condition.value === undefined ? "" : String(condition.value);
  listDraft.value = condition.values?.join(", ") ?? "";
  minDraft.value = condition.min === undefined ? "" : String(condition.min);
  maxDraft.value = condition.max === undefined ? "" : String(condition.max);
}

function reportChildValidity(key: string, valid: boolean) {
  childValidity.value = { ...childValidity.value, [key]: valid };
}

function updateLeaf(patch: Partial<RuleCondition>) {
  if (props.disabled || isLogic.value) return;
  const next: RuleCondition = { ...cloneCondition(props.modelValue), ...patch };
  delete next.children;
  emitReplacement(next);
}

function targetsForField(field: Field): Target[] {
  if (["regular_match", "special_match", "position_match"].includes(field))
    return [];
  if (field === "draw_digit" || field === "draw_parity")
    return isDigitModel.value ? ["digits"] : [];

  const targets: Target[] = isDigitModel.value
    ? ["digits"]
    : ["all", "regular", "special"];
  return targets.filter((target) => {
    const count =
      target === "all"
        ? props.definition.model.total_count
        : target === "regular"
          ? props.definition.model.regular_count
          : target === "special"
            ? props.definition.model.special_count
            : props.definition.model.length;
    if (field === "draw_unique_count" || field === "draw_all_same")
      return count >= 1;
    if (field === "draw_first_last_same") return count >= 2;
    return count >= 1;
  });
}

const fields = computed<Field[]>(() => {
  const available: Field[] = [];
  if (props.definition.selection.mode === "numbers" && !isDigitModel.value) {
    available.push("regular_match", "special_match");
  }
  if (props.definition.selection.mode === "numbers" && isDigitModel.value) {
    available.push("position_match");
  }
  if (props.definition.selection.mode === "exclude")
    available.push("excluded_match");
  available.push(
    "draw_sum",
    "draw_odd_count",
    "draw_even_count",
    "draw_unique_count",
    "draw_all_same",
    "draw_first_last_same",
    "draw_span",
    "draw_consecutive",
  );
  if (isDigitModel.value) available.push("draw_digit", "draw_parity");
  if (Object.keys(props.definition.number_attributes).length)
    available.push("attribute_match");
  return available.filter(
    (field) =>
      targetsForField(field).length > 0 ||
      ["regular_match", "special_match", "position_match"].includes(field),
  );
});

const operators = computed<Operator[]>(() =>
  props.definition.selection.mode === "features"
    ? ["equals", "in", "between", "selected"]
    : ["equals", "in", "between"],
);
const targets = computed(() =>
  props.modelValue.field ? targetsForField(props.modelValue.field) : [],
);
const attributeGroups = computed(() =>
  Object.keys(props.definition.number_attributes).sort(),
);
const attributeValues = computed(() => {
  const group = props.modelValue.attribute_group;
  if (!group || !Object.hasOwn(props.definition.number_attributes, group))
    return [];
  const values = Object.keys(
    props.definition.number_attributes[group] ?? {},
  ).sort();
  if (
    props.definition.selection.mode === "attributes" &&
    props.definition.selection.attribute_groups.includes(group)
  ) {
    values.push("$selection");
  }
  return values;
});
const featureKeys = computed(() =>
  Object.keys(props.definition.selection.feature_choices).sort(),
);

function availableField(preferred?: Field): Field {
  return preferred && fields.value.includes(preferred)
    ? preferred
    : (fields.value[0] ?? "draw_sum");
}

function initialLeaf(
  operator: Operator,
  preferredField?: Field,
  source?: RuleCondition,
): RuleCondition {
  const field = availableField(preferredField);
  const existingTarget = source?.target;
  const availableTargets = targetsForField(field);
  const target = availableTargets.includes(existingTarget as Target)
    ? existingTarget
    : availableTargets[0];
  const position = ["draw_digit", "draw_parity"].includes(field)
    ? source?.position !== undefined &&
      source.position < props.definition.model.length
      ? source.position
      : 0
    : undefined;
  const group =
    field === "attribute_match"
      ? source?.attribute_group &&
        Object.hasOwn(
          props.definition.number_attributes,
          source.attribute_group,
        )
        ? source.attribute_group
        : attributeGroups.value[0]
      : undefined;
  const allowedValues =
    group && Object.hasOwn(props.definition.number_attributes, group)
      ? Object.keys(props.definition.number_attributes[group] ?? {})
      : [];
  const canUseSelection = Boolean(
    group &&
    props.definition.selection.mode === "attributes" &&
    props.definition.selection.attribute_groups.includes(group),
  );
  const attributeValue =
    field === "attribute_match"
      ? source?.attribute_value &&
        (allowedValues.includes(source.attribute_value) ||
          (source.attribute_value === "$selection" && canUseSelection))
        ? source.attribute_value
        : (allowedValues[0] ?? (canUseSelection ? "$selection" : undefined))
      : undefined;
  const clean: RuleCondition = {
    op: operator,
    field,
    ...(target ? { target } : {}),
    ...(position !== undefined ? { position } : {}),
    ...(group && attributeValue
      ? { attribute_group: group, attribute_value: attributeValue }
      : {}),
  };

  if (operator === "equals") clean.value = source?.value ?? 0;
  if (operator === "in")
    clean.values = source?.values?.length ? [...source.values] : [0];
  if (operator === "between") {
    clean.min = source?.min ?? 0;
    clean.max = source?.max ?? 0;
  }
  if (operator === "selected") {
    const preferredKey = source?.selection_key;
    if (preferredKey && featureKeys.value.includes(preferredKey))
      clean.selection_key = preferredKey;
    else if (featureKeys.value[0]) clean.selection_key = featureKeys.value[0];
  }
  return clean;
}

function changeOperator(operator: Operator) {
  if (props.disabled || !operators.value.includes(operator)) return;
  emitReplacement(
    initialLeaf(operator, props.modelValue.field, props.modelValue),
  );
}

function changeConditionOperator(operator: Operator) {
  if (operator === "all" || operator === "any" || operator === "not")
    changeLogic(operator);
  else changeOperator(operator);
}

function changeField(field: Field) {
  if (props.disabled || isLogic.value || !fields.value.includes(field)) return;
  const next = initialLeaf(props.modelValue.op, field, props.modelValue);
  if (props.modelValue.op === "equals") next.value = props.modelValue.value;
  if (props.modelValue.op === "in")
    next.values = props.modelValue.values
      ? [...props.modelValue.values]
      : undefined;
  if (props.modelValue.op === "between") {
    next.min = props.modelValue.min;
    next.max = props.modelValue.max;
  }
  if (props.modelValue.op === "selected")
    next.selection_key = props.modelValue.selection_key;
  emitReplacement(next);
}

function changeTarget(target: Target) {
  if (
    props.disabled ||
    !props.modelValue.field ||
    !targetsForField(props.modelValue.field).includes(target)
  )
    return;
  updateLeaf({ target });
}

function changePosition(position: string) {
  if (props.disabled || position === "") return;
  const parsed = parseCanonicalNumber(position);
  if (parsed === null || parsed >= props.definition.model.length) return;
  updateLeaf({ position: parsed });
}

function changeAttributeGroup(group: string) {
  if (props.disabled || !attributeGroups.value.includes(group)) return;
  const labels = Object.keys(
    props.definition.number_attributes[group] ?? {},
  ).sort();
  const canUseSelection =
    props.definition.selection.mode === "attributes" &&
    props.definition.selection.attribute_groups.includes(group);
  updateLeaf({
    attribute_group: group,
    attribute_value: labels[0] ?? (canUseSelection ? "$selection" : ""),
  });
}

function changeAttributeValue(value: string) {
  if (props.disabled || !attributeValues.value.includes(value)) return;
  updateLeaf({ attribute_value: value });
}

function changeSelectionKey(key: string) {
  if (props.disabled || !featureKeys.value.includes(key)) return;
  updateLeaf({ selection_key: key });
}

function parseCanonicalNumber(raw: string): number | null {
  const text = raw.trim();
  if (!/^(0|[1-9]\d*)$/.test(text)) return null;
  const value = Number(text);
  return Number.isSafeInteger(value) && value >= 0 && value <= MAX_VALUE
    ? value
    : null;
}

function updateEquals(raw: string) {
  if (props.disabled) return;
  equalsDraft.value = raw;
  const value = parseCanonicalNumber(raw);
  if (value === null) return;
  updateLeaf({ value });
}

function updateList(raw: string) {
  if (props.disabled) return;
  listDraft.value = raw;
  const values = listValues(raw);
  if (values) updateLeaf({ values });
}

function updateRange(key: "min" | "max", raw: string) {
  if (props.disabled) return;
  if (key === "min") minDraft.value = raw;
  else maxDraft.value = raw;
  if (!raw.trim()) {
    const patch: Partial<RuleCondition> = { [key]: undefined };
    updateLeaf(patch);
    return;
  }
  const value = parseCanonicalNumber(raw);
  if (value === null) return;
  updateLeaf({ [key]: value });
}

function changeLogic(operator: "all" | "any" | "not") {
  if (
    props.disabled ||
    props.depth >= MAX_DEPTH ||
    (!isLogic.value && currentTreeNodes.value >= MAX_NODES)
  )
    return;
  const existing = props.modelValue.children?.map(cloneCondition) ?? [];
  const children =
    operator === "not"
      ? [existing[0] ?? initialLeaf("equals")]
      : existing.length
        ? existing
        : [initialLeaf("equals")];
  emitReplacement({ op: operator, children });
}

function addChild() {
  if (!canAddChild.value) return;
  const children = (props.modelValue.children ?? []).map(cloneCondition);
  children.push(initialLeaf("equals"));
  childKeys.value = [...childKeys.value, nextChildKey()];
  emitReplacement({ op: props.modelValue.op, children });
}

function removeChild(index: number) {
  if (!canRemoveChild.value) return;
  const children = (props.modelValue.children ?? []).map(cloneCondition);
  if (index < 0 || index >= children.length) return;
  children.splice(index, 1);
  const [removedKey] = childKeys.value.splice(index, 1);
  if (removedKey) {
    const nextValidity = { ...childValidity.value };
    delete nextValidity[removedKey];
    childValidity.value = nextValidity;
  }
  emitReplacement({ op: props.modelValue.op, children });
}

watch(
  () => props.modelValue.children?.length ?? 0,
  (length) => {
    childKeys.value = childKeys.value.slice(0, length);
    while (childKeys.value.length < length)
      childKeys.value.push(nextChildKey());
    const liveKeys = new Set(childKeys.value);
    childValidity.value = Object.fromEntries(
      Object.entries(childValidity.value).filter(([key]) => liveKeys.has(key)),
    );
  },
  { immediate: true, deep: true },
);

watch(
  () => props.modelValue,
  (condition, previous) => {
    if (!previous || condition.op !== previous.op) {
      syncDrafts(condition);
      return;
    }
    if (
      !numberError.value ||
      parseCanonicalNumber(equalsDraft.value) !== null
    ) {
      if (condition.value !== undefined)
        equalsDraft.value = String(condition.value);
    }
    if (!numberError.value || listValues(listDraft.value) !== null) {
      if (condition.values) listDraft.value = condition.values.join(", ");
    }
    if (!numberError.value || parseCanonicalNumber(minDraft.value) !== null) {
      if (condition.min !== undefined) minDraft.value = String(condition.min);
    }
    if (!numberError.value || parseCanonicalNumber(maxDraft.value) !== null) {
      if (condition.max !== undefined) maxDraft.value = String(condition.max);
    }
  },
  { immediate: true, deep: true },
);

watch(isValid, (valid) => emit("validity", valid), { immediate: true });
</script>

<template>
  <fieldset
    class="condition-node"
    :disabled="disabled"
    :aria-describedby="numberError ? `${id}-number-error` : undefined"
  >
    <legend>{{ t('admin.rule.condition.level', { depth }) }}</legend>

    <div class="condition-grid">
      <label :for="`${id}-operator`">{{ t('admin.copy.4fce95fa') }}</label>
      <select
        :id="`${id}-operator`"
        :value="modelValue.op"
        :disabled="disabled"
        @change="
          changeConditionOperator(
            ($event.target as HTMLSelectElement).value as Operator,
          )
        "
      >
        <option
          value="all"
          :disabled="
            depth >= MAX_DEPTH || (!isLogic && currentTreeNodes >= MAX_NODES)
          "
        >
          {{ t('admin.copy.2abf6c9f') }}
        </option>
        <option
          value="any"
          :disabled="
            depth >= MAX_DEPTH || (!isLogic && currentTreeNodes >= MAX_NODES)
          "
        >
          {{ t('admin.copy.19027a81') }}
        </option>
        <option
          value="not"
          :disabled="
            depth >= MAX_DEPTH || (!isLogic && currentTreeNodes >= MAX_NODES)
          "
        >
          {{ t('admin.copy.4136a472') }}
        </option>
        <option v-for="operator in operators" :key="operator" :value="operator">
          {{ operatorLabel(operator) }}
        </option>
        <option
          v-if="!isLogic && !operators.includes(modelValue.op)"
          :value="modelValue.op"
        >
          {{ modelValue.op }} {{ t('admin.copy.608481b9') }}
        </option>
      </select>
      <p class="replacement-note">
        {{ t('admin.copy.8b9c8ed6') }}
      </p>

      <template v-if="!isLogic">
        <label :for="`${id}-field`">{{ t('admin.copy.54ec0b76') }}</label>
        <select
          :id="`${id}-field`"
          :value="modelValue.field ?? ''"
          :disabled="disabled"
          @change="
            changeField(($event.target as HTMLSelectElement).value as Field)
          "
        >
          <option
            v-if="modelValue.field && !fields.includes(modelValue.field)"
            :value="modelValue.field"
          >
            {{ modelValue.field }} {{ t('admin.copy.ee345b92') }}
          </option>
          <option v-for="field in fields" :key="field" :value="field">
            {{ fieldLabel(field) }} ({{ field }})
          </option>
        </select>

        <template v-if="targets.length">
          <label :for="`${id}-target`">{{ t('admin.copy.82e8fae5') }}</label>
          <select
            :id="`${id}-target`"
            :value="modelValue.target ?? ''"
            :disabled="disabled"
            @change="
              changeTarget(($event.target as HTMLSelectElement).value as Target)
            "
          >
            <option
              v-if="modelValue.target && !targets.includes(modelValue.target)"
              :value="modelValue.target"
            >
              {{ modelValue.target }} {{ t('admin.copy.608481b9') }}
            </option>
            <option v-for="target in targets" :key="target" :value="target">
              {{ targetLabel(target) }} ({{ target }})
            </option>
          </select>
        </template>

        <template
          v-if="
            modelValue.field === 'draw_digit' ||
            modelValue.field === 'draw_parity'
          "
        >
          <label :for="`${id}-position`">{{ t('admin.copy.2e70ebca') }}</label>
          <select
            :id="`${id}-position`"
            :value="
              modelValue.position === undefined
                ? ''
                : String(modelValue.position)
            "
            :disabled="disabled"
            @change="changePosition(($event.target as HTMLSelectElement).value)"
          >
            <option value="" disabled>{{ t('admin.copy.60972b51') }}</option>
            <option
              v-for="position in definition.model.length"
              :key="position"
              :value="String(position - 1)"
            >
              {{ position }}
            </option>
          </select>
        </template>

        <template v-if="modelValue.field === 'attribute_match'">
          <label :for="`${id}-attribute-group`">{{ t('admin.copy.73dfdb49') }}</label>
          <select
            :id="`${id}-attribute-group`"
            :value="modelValue.attribute_group ?? ''"
            :disabled="disabled"
            @change="
              changeAttributeGroup(($event.target as HTMLSelectElement).value)
            "
          >
            <option value="" disabled>{{ t('admin.copy.20dff992') }}</option>
            <option
              v-for="group in attributeGroups"
              :key="group"
              :value="group"
            >
              {{ group }}
            </option>
            <option
              v-if="
                modelValue.attribute_group &&
                !attributeGroups.includes(modelValue.attribute_group)
              "
              :value="modelValue.attribute_group"
            >
              {{ modelValue.attribute_group }} {{ t('admin.copy.608481b9') }}
            </option>
          </select>
          <label :for="`${id}-attribute-value`"
            >{{ t('admin.copy.56e04ca9') }}</label
          >
          <select
            :id="`${id}-attribute-value`"
            :value="modelValue.attribute_value ?? ''"
            :disabled="disabled || !modelValue.attribute_group"
            @change="
              changeAttributeValue(($event.target as HTMLSelectElement).value)
            "
          >
            <option value="" disabled>{{ t('admin.copy.17d062d1') }}</option>
            <option
              v-for="value in attributeValues"
              :key="value"
              :value="value"
            >
              {{ value }}
            </option>
            <option
              v-if="
                modelValue.attribute_value &&
                !attributeValues.includes(modelValue.attribute_value)
              "
              :value="modelValue.attribute_value"
            >
              {{ modelValue.attribute_value }} {{ t('admin.copy.608481b9') }}
            </option>
          </select>
        </template>

        <template v-if="modelValue.op === 'selected'">
          <label :for="`${id}-selection-key`">{{ t('admin.copy.7c4ba52f') }}</label>
          <select
            :id="`${id}-selection-key`"
            :value="modelValue.selection_key ?? ''"
            :disabled="disabled"
            @change="
              changeSelectionKey(($event.target as HTMLSelectElement).value)
            "
          >
            <option value="" disabled>{{ t('admin.copy.d898effe') }}</option>
            <option v-for="key in featureKeys" :key="key" :value="key">
              {{ key }}
            </option>
            <option
              v-if="
                modelValue.selection_key &&
                !featureKeys.includes(modelValue.selection_key)
              "
              :value="modelValue.selection_key"
            >
              {{ modelValue.selection_key }} {{ t('admin.copy.608481b9') }}
            </option>
          </select>
        </template>

        <template v-if="modelValue.op === 'equals'">
          <label :for="`${id}-value`">{{ t('admin.copy.ee0923c4') }}</label>
          <input
            :id="`${id}-value`"
            :value="equalsDraft"
            type="text"
            inputmode="numeric"
            autocomplete="off"
            :disabled="disabled"
            @input="updateEquals(($event.target as HTMLInputElement).value)"
          />
        </template>
        <template v-else-if="modelValue.op === 'in'">
          <label :for="`${id}-values`">{{ t('admin.copy.a4451640') }}</label>
          <input
            :id="`${id}-values`"
            :value="listDraft"
            type="text"
            inputmode="text"
            autocomplete="off"
            :disabled="disabled"
            @input="updateList(($event.target as HTMLInputElement).value)"
          />
        </template>
        <template v-else-if="modelValue.op === 'between'">
          <label :for="`${id}-min`">{{ t('admin.copy.ff10a0ec') }}</label>
          <input
            :id="`${id}-min`"
            :value="minDraft"
            type="text"
            inputmode="numeric"
            autocomplete="off"
            :disabled="disabled"
            @input="
              updateRange('min', ($event.target as HTMLInputElement).value)
            "
          />
          <label :for="`${id}-max`">{{ t('admin.copy.728429d6') }}</label>
          <input
            :id="`${id}-max`"
            :value="maxDraft"
            type="text"
            inputmode="numeric"
            autocomplete="off"
            :disabled="disabled"
            @input="
              updateRange('max', ($event.target as HTMLInputElement).value)
            "
          />
        </template>
      </template>
    </div>

    <p
      v-if="numberError"
      :id="`${id}-number-error`"
      class="field-error"
      role="alert"
    >
      {{ operatorErrorEnglish[numberError] ? t(adminKey(operatorErrorEnglish[numberError]!)) : display(numberError) }}
    </p>

    <template v-if="isLogic">
      <div class="children-heading">
        <span>{{ t('admin.rule.condition.children', { count: modelValue.children?.length ?? 0 }) }}</span>
        <button type="button" :disabled="!canAddChild" @click="addChild">
          {{ t('admin.copy.2e65be2e') }}
        </button>
      </div>
      <p v-if="depth >= 8" class="limit-note">
        {{ t('admin.copy.5a09e475') }}
      </p>
      <p v-else-if="currentTreeNodes >= 128" class="limit-note">
        {{ t('admin.copy.54f7a397') }}
      </p>
      <div
        v-for="(child, index) in modelValue.children ?? []"
        :key="childKeys[index]"
        class="child-condition"
      >
        <div class="child-heading">
          <span>{{ t('admin.rule.condition.child', { index: index + 1 }) }}</span>
          <button
            type="button"
            :disabled="!canRemoveChild"
            @click="removeChild(index)"
          >
            {{ t('admin.copy.01f5024f') }}
          </button>
        </div>
        <RuleConditionEditor
          :model-value="child"
          :definition="definition"
          :disabled="disabled"
          :depth="depth + 1"
          :total-nodes="currentTreeNodes"
          @validity="(valid) => reportChildValidity(childKeys[index], valid)"
          @update:model-value="
            (replacement) => {
              const children = (modelValue.children ?? []).map(cloneCondition);
              children[index] = cloneCondition(replacement);
              emitReplacement({ op: modelValue.op, children });
            }
          "
        />
      </div>
      <p v-if="modelValue.op === 'not'" class="limit-note">
        {{ t('admin.copy.53b54a0f') }}
      </p>
    </template>
  </fieldset>
</template>

<style scoped>
.condition-node {
  min-width: 0;
  margin: 0;
  padding: 0.9rem;
  border: 1px solid #cbd5e1;
  border-radius: 0.65rem;
  background: var(--surface);
}

.condition-node legend {
  padding: 0 0.35rem;
  color: #172554;
  font-weight: 700;
}

.condition-grid {
  display: grid;
  grid-template-columns: minmax(9rem, 0.8fr) minmax(0, 1.5fr);
  gap: 0.65rem 0.8rem;
  align-items: center;
}

label,
.children-heading,
.child-heading {
  font-size: 0.92rem;
  font-weight: 600;
}

select,
input,
button {
  min-width: 0;
  min-height: 2.6rem;
  padding: 0.5rem 0.65rem;
  border: 1px solid #94a3b8;
  border-radius: 0.4rem;
  background: var(--surface);
  color: var(--ink);
  font: inherit;
}

button {
  cursor: pointer;
}

button:disabled,
select:disabled,
input:disabled {
  cursor: not-allowed;
  opacity: 0.65;
}

select:focus-visible,
input:focus-visible,
button:focus-visible {
  outline: 3px solid var(--primary);
  outline-offset: 2px;
}

.replacement-note,
.field-error,
.limit-note {
  grid-column: 1 / -1;
  margin: 0;
  font-size: 0.85rem;
}

.replacement-note,
.limit-note {
  color: var(--ink-secondary);
}

.field-error {
  margin-top: 0.65rem;
  color: var(--danger-text);
}

.children-heading,
.child-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  margin-top: 0.9rem;
}

.child-condition {
  margin-top: 0.75rem;
  padding-left: 0.75rem;
  border-left: 3px solid #bfdbfe;
}

.child-heading {
  margin: 0 0 0.45rem;
  color: var(--ink-secondary);
}

@media (max-width: 520px) {
  .condition-grid {
    grid-template-columns: minmax(0, 1fr);
    gap: 0.35rem;
  }

  .condition-grid label:not(:first-child) {
    margin-top: 0.45rem;
  }

  .condition-node {
    padding: 0.7rem;
  }

  .child-condition {
    padding-left: 0.45rem;
  }
}
</style>
