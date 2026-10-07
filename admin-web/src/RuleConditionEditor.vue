<script setup lang="ts">
import { computed, ref, useId, watch } from "vue";
import type { RuleCondition, RuleDefinition } from "./rule-simulation-api";
import { useAdminI18n } from "./i18n";

defineOptions({ name: "RuleConditionEditor" });
const { t } = useAdminI18n();
const ui = (zh: string, en: string = zh) => t(zh, en);
const fieldZh: Record<string, string> = {
  regular_match: "普通号命中数量", special_match: "特别号命中数量", position_match: "逐位命中数量",
  excluded_match: "被排除号码命中数量", draw_sum: "号码和值", draw_odd_count: "奇数个数",
  draw_even_count: "偶数个数", draw_unique_count: "不同号码个数", draw_all_same: "全部号码相同",
  draw_first_last_same: "首尾号码相同", draw_span: "号码跨度", draw_consecutive: "连续邻接对数",
  draw_digit: "指定位置数字", draw_parity: "指定位置奇偶（偶 0 / 奇 1）", attribute_match: "属性命中次数",
};
const targetZh: Record<string, string> = { all: "全部开奖结果", regular: "普通号码", special: "特别号码", digits: "数字各位" };
const fieldLabel = (field: string) => ui(fieldZh[field] ?? field, fieldLabels[field] ?? field);
const targetLabel = (target: string) => ui(targetZh[target] ?? target, targetLabels[target] ?? target);
const operatorLabel = (operator: string) => {
  const zh: Record<string, string> = { equals: "等于", in: "属于列表", between: "范围内", selected: "用户已选择" };
  const en: Record<string, string> = { equals: "Equals", in: "In list", between: "Between", selected: "Selected by user" };
  return ui(zh[operator] ?? operator, en[operator] ?? operator);
};
const operatorErrorEnglish: Record<string, [string, string]> = {
  "A value is required.": ["请输入比较值。", "A comparison value is required."],
  "Enter a whole number from 0 to 20,000,000.": ["请输入 0–20,000,000 的整数。", "Enter a whole number from 0 to 20,000,000."],
  "Enter 1–100 unique whole numbers from 0 to 20,000,000, separated by commas.": ["请输入 1–100 个以逗号分隔且不重复的整数，范围为 0–20,000,000。", "Enter 1–100 unique whole numbers from 0 to 20,000,000, separated by commas."],
  "Both minimum and maximum are required.": ["最小值和最大值均为必填项。", "Both minimum and maximum are required."],
  "Enter whole numbers from 0 to 20,000,000 for both bounds.": ["最小值和最大值都必须是 0–20,000,000 的整数。", "Enter whole numbers from 0 to 20,000,000 for both bounds."],
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
  regular_match: "Regular number matches",
  special_match: "Special number matches",
  position_match: "Position matches",
  excluded_match: "Excluded number matches",
  draw_sum: "Draw sum",
  draw_odd_count: "Odd number count",
  draw_even_count: "Even number count",
  draw_unique_count: "Unique number count",
  draw_all_same: "All numbers are the same",
  draw_first_last_same: "First and last numbers match",
  draw_span: "Number span",
  draw_consecutive: "Adjacent consecutive pairs",
  draw_digit: "Digit at position",
  draw_parity: "Digit parity at position (even 0 / odd 1)",
  attribute_match: "Attribute matches",
};
const targetLabels: Record<string, string> = {
  all: "All draw numbers",
  regular: "Regular numbers",
  special: "Special numbers",
  digits: "Each digit",
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
    ? ["all", "digits"]
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
    <legend>{{ ui(`条件 · 第 ${depth} 层`, `Condition · level ${depth}`) }}</legend>

    <div class="condition-grid">
      <label :for="`${id}-operator`">{{ t("条件运算", "Condition operator") }}</label>
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
          {{ t("全部成立 AND（all）", "All conditions AND (all)") }}
        </option>
        <option
          value="any"
          :disabled="
            depth >= MAX_DEPTH || (!isLogic && currentTreeNodes >= MAX_NODES)
          "
        >
          {{ t("任意成立 OR（any）", "Any condition OR (any)") }}
        </option>
        <option
          value="not"
          :disabled="
            depth >= MAX_DEPTH || (!isLogic && currentTreeNodes >= MAX_NODES)
          "
        >
          {{ t("取反 NOT（not）", "Negate NOT (not)") }}
        </option>
        <option v-for="operator in operators" :key="operator" :value="operator">
          {{ operatorLabel(operator) }}
        </option>
        <option
          v-if="!isLogic && !operators.includes(modelValue.op)"
          :value="modelValue.op"
        >
          {{ modelValue.op }} {{ t("（不可用）", "(unavailable)") }}
        </option>
      </select>
      <p class="replacement-note">
        {{ t("切换叶条件运算会清除旧比较参数；逻辑组合保留子条件，切换 NOT 只保留一个子条件。请重新核对。", "Changing a leaf operator clears its comparison values. Logical operators keep child conditions; switching to NOT keeps only one child. Review the result.") }}
      </p>

      <template v-if="!isLogic">
        <label :for="`${id}-field`">{{ t("判断字段", "Field") }}</label>
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
            {{ modelValue.field }} {{ t("（当前定义不可用）", "(unavailable for this definition)") }}
          </option>
          <option v-for="field in fields" :key="field" :value="field">
            {{ fieldLabel(field) }} ({{ field }})
          </option>
        </select>

        <template v-if="targets.length">
          <label :for="`${id}-target`">{{ t("开奖结果部分", "Draw target") }}</label>
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
              {{ modelValue.target }} {{ t("（不可用）", "(unavailable)") }}
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
          <label :for="`${id}-position`">{{ t("数字位置（从第 1 位开始）", "Digit position (starting at 1)") }}</label>
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
            <option value="" disabled>{{ t("请选择位置", "Select a position") }}</option>
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
          <label :for="`${id}-attribute-group`">{{ t("属性组", "Attribute group") }}</label>
          <select
            :id="`${id}-attribute-group`"
            :value="modelValue.attribute_group ?? ''"
            :disabled="disabled"
            @change="
              changeAttributeGroup(($event.target as HTMLSelectElement).value)
            "
          >
            <option value="" disabled>{{ t("请选择已配置的属性组", "Select a configured group") }}</option>
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
              {{ modelValue.attribute_group }} {{ t("（不可用）", "(unavailable)") }}
            </option>
          </select>
          <label :for="`${id}-attribute-value`"
            >{{ t("属性值（$selection 表示用户所选值）", "Attribute value ($selection means the user's selected value)") }}</label
          >
          <select
            :id="`${id}-attribute-value`"
            :value="modelValue.attribute_value ?? ''"
            :disabled="disabled || !modelValue.attribute_group"
            @change="
              changeAttributeValue(($event.target as HTMLSelectElement).value)
            "
          >
            <option value="" disabled>{{ t("请选择已配置的值", "Select a configured value") }}</option>
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
              {{ modelValue.attribute_value }} {{ t("（不可用）", "(unavailable)") }}
            </option>
          </select>
        </template>

        <template v-if="modelValue.op === 'selected'">
          <label :for="`${id}-selection-key`">{{ t("用户所选特征代码", "Selected feature key") }}</label>
          <select
            :id="`${id}-selection-key`"
            :value="modelValue.selection_key ?? ''"
            :disabled="disabled"
            @change="
              changeSelectionKey(($event.target as HTMLSelectElement).value)
            "
          >
            <option value="" disabled>{{ t("请选择已配置的特征", "Select a configured feature") }}</option>
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
              {{ modelValue.selection_key }} {{ t("（不可用）", "(unavailable)") }}
            </option>
          </select>
        </template>

        <template v-if="modelValue.op === 'equals'">
          <label :for="`${id}-value`">{{ t("比较值（0–20000000）", "Comparison value (0–20,000,000)") }}</label>
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
          <label :for="`${id}-values`">{{ t("比较值集合（逗号分隔）", "Comparison values (comma-separated)") }}</label>
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
          <label :for="`${id}-min`">{{ t("最小值（0–20,000,000）", "Minimum (0–20,000,000)") }}</label>
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
          <label :for="`${id}-max`">{{ t("最大值（0–20,000,000）", "Maximum (0–20,000,000)") }}</label>
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
      {{ operatorErrorEnglish[numberError] ? ui(operatorErrorEnglish[numberError]![0], operatorErrorEnglish[numberError]![1]) : numberError }}
    </p>

    <template v-if="isLogic">
      <div class="children-heading">
        <span>{{ ui(`子条件 · ${modelValue.children?.length ?? 0} / 32`, `Child conditions · ${modelValue.children?.length ?? 0} / 32`) }}</span>
        <button type="button" :disabled="!canAddChild" @click="addChild">
          {{ t("新增子条件", "Add child condition") }}
        </button>
      </div>
      <p v-if="depth >= 8" class="limit-note">
        {{ t("已达到最大嵌套深度（8 层）。", "Maximum nesting depth (8) reached.") }}
      </p>
      <p v-else-if="currentTreeNodes >= 128" class="limit-note">
        {{ t("已达到条件数量上限（128）。", "Maximum condition count (128) reached.") }}
      </p>
      <div
        v-for="(child, index) in modelValue.children ?? []"
        :key="childKeys[index]"
        class="child-condition"
      >
        <div class="child-heading">
          <span>{{ ui(`子条件 ${index + 1}`, `Child condition ${index + 1}`) }}</span>
          <button
            type="button"
            :disabled="!canRemoveChild"
            @click="removeChild(index)"
          >
            {{ t("删除子条件", "Remove child condition") }}
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
        {{ t("NOT 运算必须且只能有一个子条件。", "NOT requires exactly one child.") }}
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
  background: #fff;
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
  background: #fff;
  color: #0f172a;
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
  outline: 3px solid #2563eb;
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
  color: #475569;
}

.field-error {
  margin-top: 0.65rem;
  color: #b91c1c;
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
  color: #334155;
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
