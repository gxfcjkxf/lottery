<script setup lang="ts">
import { computed, ref, watch } from "vue";
import RuleConditionEditor from "./RuleConditionEditor.vue";
import type {
  RuleCondition,
  RuleDefinition,
  RuleSelectionDefinition,
} from "./rule-simulation-api";
import { parseEditorNumbers, validateEditorDefinition } from "./rule-editor";
import { useAdminI18n } from "./i18n";
const { t } = useAdminI18n();
const ui = (zh: string, en: string = zh) => t(zh, en);
const errorEnglish: Record<string, string> = {
  "规则定义无效": "Invalid rule definition.", "请输入规范非负整数（最大 20000000）": "Enter a canonical non-negative integer (maximum 20,000,000).",
  "代码须以字母开头，最多 48 个字母、数字或下划线": "Codes must start with a letter and contain at most 48 letters, digits, or underscores.",
  "代码已存在": "This code already exists.", "该属性组仍被选号或条件引用，请先移除引用": "This attribute group is still referenced by selections or conditions. Remove those references first.",
  "该属性值仍被条件引用，请先修改条件": "This attribute value is still referenced by a condition. Update the condition first.",
  "该特征仍被条件引用，请先调整条件": "This feature is still referenced by a condition. Update the condition first.",
  "号码列表不能为空。": "The number list cannot be empty.", "号码列表无效": "Invalid number list.",
  "积分必须是规范的非负整数字符串，不得含小数、符号或前导零。": "Points must be a canonical non-negative integer string with no decimal, sign, or leading zero.",
  "号码输入上限必须为 0–10,000 的整数。": "The number input limit must be a whole number from 0 to 10,000.",
  "投注方式格式无效。": "Invalid selection mode format.", "投注数量或排除数量超出模型范围。": "Selection or exclusion count is outside the model range.",
  "特征选项格式无效。": "Invalid feature-choice format.", "号码属性格式无效。": "Invalid number-attribute format.",
  "号码属性组最多 16 个。": "There can be at most 16 number-attribute groups.",
  "数字模型的开奖范围必须为 all 或 digits。": "For a digit model, the draw target must be all or digits.",
  "非数字模型不能使用 digits 开奖范围。": "Non-digit models cannot use the digits draw target.",
  "条件必须指定有效的开奖范围。": "The condition must specify a valid draw target.", "条件字段无效。": "Invalid condition field.",
  "条件运算符无效。": "Invalid condition operator.", "叶条件不能包含子条件。": "Leaf conditions cannot contain child conditions.",
  "条件嵌套深度不能超过 8。": "Condition nesting cannot exceed 8 levels.", "每个奖级条件最多 128 个节点。": "Each prize-tier condition can contain at most 128 nodes.",
  "规则定义格式无效。": "Invalid rule definition format.", "schema_version 必须为 1。": "schema_version must be 1.",
  "rounding 必须为 half_up。": "rounding must be half_up.", "限额格式无效。": "Invalid limit format.",
  "混合奖级策略无效。": "Invalid mixed-tier policy.", "奖级格式无效。": "Invalid prize-tier format.",
  "同时存在排他和非排他奖级时必须显式选择 mixed_tier_policy。": "Choose mixed_tier_policy explicitly when exclusive and additive tiers are both present.",
  "赔率必须为正的精确十进制字符串，最多 10 位整数和 6 位小数。": "Odds must be a positive exact decimal string with at most 10 integer digits and 6 decimal places.",
};
const errorText = (value: string) => {
  const duplicate = value.match(/^号码 (\d+) 重复。$/);
  if (duplicate) return ui(value, `Number ${duplicate[1]} is duplicated.`);
  const tooMany = value.match(/^号码数量不能超过 (\d+) 个。$/);
  if (tooMany) return ui(value, `Number list cannot exceed ${tooMany[1]} items.`);
  const labelError = value.match(/^(号码|属性组|属性) (.+?)(格式无效。|含不属于模型号码池的号码。|不存在或重复。)$/);
  if (labelError) {
    const prefix = labelError[1] === "号码" ? "Number attribute" : labelError[1] === "属性组" ? "Attribute group" : "Attribute";
    const suffix: Record<string, string> = { "格式无效。": "has an invalid format.", "含不属于模型号码池的号码。": "contains numbers outside the model pools.", "不存在或重复。": "does not exist or is duplicated." };
    return ui(value, `${prefix} ${labelError[2]} ${suffix[labelError[3]!]}`);
  }
  return ui(value, errorEnglish[value] ?? value);
};

const props = defineProps<{ modelValue: RuleDefinition; disabled?: boolean }>();
const emit = defineEmits<{
  (e: "update:modelValue", value: RuleDefinition): void;
  (e: "validity", valid: boolean): void;
}>();
const definition = computed(() => props.modelValue);
const errors = ref<Record<string, string>>({});
const textDrafts = ref<Record<string, string>>({});
const conditionValidity = ref<Record<string, boolean>>({});
const tierKeys = ref(
  props.modelValue.prize_tiers.map(() => crypto.randomUUID()),
);
watch(
  () => props.modelValue.prize_tiers.length,
  (length) => {
    while (tierKeys.value.length < length)
      tierKeys.value.push(crypto.randomUUID());
    tierKeys.value = tierKeys.value.slice(0, length);
  },
);
const structuralError = computed(() => {
  try {
    validateEditorDefinition(definition.value);
    return "";
  } catch (cause) {
    return cause instanceof Error ? cause.message : "规则定义无效";
  }
});
watch(
  [structuralError, errors, conditionValidity],
  () =>
    emit(
      "validity",
      !structuralError.value &&
        !Object.keys(errors.value).length &&
        !Object.values(conditionValidity.value).includes(false),
    ),
  { deep: true, immediate: true },
);
const copy = () =>
  structuredClone(
    JSON.parse(JSON.stringify(definition.value)),
  ) as RuleDefinition;
function update(change: (draft: RuleDefinition) => void) {
  if (props.disabled) return;
  const draft = copy();
  change(draft);
  emit("update:modelValue", draft);
}
function error(key: string, cause: unknown) {
  errors.value = {
    ...errors.value,
    [key]: cause instanceof Error ? cause.message : String(cause),
  };
}
function clearError(key: string) {
  const next = { ...errors.value };
  delete next[key];
  errors.value = next;
}
function localFamily(oldBase: string, newBase?: string) {
  const transform = (source: Record<string, string>) =>
    Object.fromEntries(
      Object.entries(source).flatMap(([key, value]) => {
        if (key !== oldBase && !key.startsWith(`${oldBase}:`))
          return [[key, value]];
        return newBase === undefined
          ? []
          : [[newBase + key.slice(oldBase.length), value]];
      }),
    );
  errors.value = transform(errors.value);
  textDrafts.value = transform(textDrafts.value);
}
function integer(
  value: string,
  key: string,
  change: (d: RuleDefinition, n: number) => void,
) {
  if (props.disabled) return;
  if (
    !/^(0|[1-9]\d*)$/.test(value) ||
    !Number.isSafeInteger(Number(value)) ||
    Number(value) > 20_000_000
  ) {
    error(key, "请输入规范非负整数（最大 20000000）");
    return;
  }
  clearError(key);
  update((d) => change(d, Number(value)));
}
function csv(
  key: string,
  value: string,
  change: (d: RuleDefinition, values: number[]) => void,
  max = 10000,
) {
  if (props.disabled) return;
  textDrafts.value = { ...textDrafts.value, [key]: value };
  try {
    const values = parseEditorNumbers(value, max);
    clearError(key);
    update((d) => change(d, values));
  } catch (cause) {
    error(key, cause);
  }
}
const csvText = (key: string, values: number[]) =>
  textDrafts.value[key] ?? values.join(",");
function mapCondition(
  c: RuleCondition,
  fn: (c: RuleCondition) => void,
): RuleCondition {
  const next = structuredClone(c);
  fn(next);
  if (next.children)
    next.children = next.children.map((child) => mapCondition(child, fn));
  return next;
}
function hasCondition(predicate: (c: RuleCondition) => boolean) {
  const visit = (c: RuleCondition): boolean =>
    predicate(c) || (c.children ?? []).some(visit);
  return definition.value.prize_tiers.some((t) => visit(t.condition));
}
function identifier(value: string, existing: object, old?: string) {
  if (!/^[A-Za-z][A-Za-z0-9_]{0,47}$/.test(value))
    throw new Error("代码须以字母开头，最多 48 个字母、数字或下划线");
  if (value !== old && Object.hasOwn(existing, value))
    throw new Error("代码已存在");
  return value;
}
function unique(prefix: string, existing: object) {
  let i = 1;
  while (Object.hasOwn(existing, `${prefix}${i}`)) i++;
  return `${prefix}${i}`;
}
function firstNumber() {
  const m = definition.value.model;
  if (m.model === "DIGITS_0_9") return 0;
  const p = m.regular_count ? m.regular_pool : m.special_pool;
  return p.values?.[0] ?? p.min ?? 0;
}
function addGroup() {
  if (
    props.disabled ||
    Object.keys(definition.value.number_attributes).length >= 16
  )
    return;
  update((d) => {
    d.number_attributes[unique("Group", d.number_attributes)] = {
      Value1: [firstNumber()],
    };
  });
}
function renameGroup(old: string, name: string) {
  if (props.disabled) return;
  const key = `group:${old}`;
  try {
    identifier(name, definition.value.number_attributes, old);
    clearError(key);
    localFamily(`attribute:${old}`, `attribute:${name}`);
    localFamily(`label:${old}`, `label:${name}`);
    update((d) => {
      const values = d.number_attributes[old];
      delete d.number_attributes[old];
      d.number_attributes[name] = values;
      d.selection.attribute_groups = d.selection.attribute_groups.map((g) =>
        g === old ? name : g,
      );
      d.prize_tiers = d.prize_tiers.map((t) => ({
        ...t,
        condition: mapCondition(t.condition, (c) => {
          if (c.attribute_group === old) c.attribute_group = name;
        }),
      }));
    });
  } catch (cause) {
    error(key, cause);
  }
}
function removeGroup(group: string) {
  if (props.disabled) return;
  const key = `group:${group}`;
  if (
    definition.value.selection.attribute_groups.includes(group) ||
    hasCondition((c) => c.attribute_group === group)
  ) {
    error(key, "该属性组仍被选号或条件引用，请先移除引用");
    return;
  }
  clearError(key);
  localFamily(`attribute:${group}`);
  localFamily(`label:${group}`);
  update((d) => {
    delete d.number_attributes[group];
  });
}
function addLabel(group: string) {
  if (
    props.disabled ||
    Object.keys(definition.value.number_attributes[group]).length >= 32
  )
    return;
  update((d) => {
    d.number_attributes[group][unique("Value", d.number_attributes[group])] = [
      firstNumber(),
    ];
  });
}
function renameLabel(group: string, old: string, name: string) {
  if (props.disabled) return;
  const key = `label:${group}:${old}`;
  try {
    identifier(name, definition.value.number_attributes[group], old);
    clearError(key);
    localFamily(`attribute:${group}:${old}`, `attribute:${group}:${name}`);
    update((d) => {
      const values = d.number_attributes[group][old];
      delete d.number_attributes[group][old];
      d.number_attributes[group][name] = values;
      d.prize_tiers = d.prize_tiers.map((t) => ({
        ...t,
        condition: mapCondition(t.condition, (c) => {
          if (c.attribute_group === group && c.attribute_value === old)
            c.attribute_value = name;
        }),
      }));
    });
  } catch (cause) {
    error(key, cause);
  }
}
function removeLabel(group: string, label: string) {
  if (
    props.disabled ||
    Object.keys(definition.value.number_attributes[group]).length <= 1
  )
    return;
  const key = `label:${group}:${label}`;
  if (
    hasCondition(
      (c) => c.attribute_group === group && c.attribute_value === label,
    )
  ) {
    error(key, "该属性值仍被条件引用，请先修改条件");
    return;
  }
  clearError(key);
  localFamily(`attribute:${group}:${label}`);
  update((d) => {
    delete d.number_attributes[group][label];
  });
}
function setSelectionMode(mode: RuleSelectionDefinition["mode"]) {
  update((d) => {
    const features = d.selection.feature_choices;
    d.selection = {
      mode,
      regular_count: 0,
      special_count: 0,
      exclude_count: 0,
      attribute_groups: [],
      feature_choices: {},
    };
    if (mode === "numbers") {
      d.selection.regular_count = d.model.regular_count;
      d.selection.special_count = d.model.special_count;
    } else if (mode === "exclude") d.selection.exclude_count = 1;
    else if (mode === "attributes")
      d.selection.attribute_groups = Object.keys(d.number_attributes).slice(
        0,
        1,
      );
    else
      d.selection.feature_choices = Object.keys(features).length
        ? features
        : { Feature1: [0, 1] };
  });
}
function selectGroup(group: string, checked: boolean) {
  update((d) => {
    d.selection.attribute_groups = checked
      ? [...d.selection.attribute_groups, group]
      : d.selection.attribute_groups.filter((g) => g !== group);
  });
}
function addFeature() {
  if (
    props.disabled ||
    Object.keys(definition.value.selection.feature_choices).length >= 16
  )
    return;
  update((d) => {
    d.selection.feature_choices[
      unique("Feature", d.selection.feature_choices)
    ] = [0, 1];
  });
}
function renameFeature(old: string, name: string) {
  if (props.disabled) return;
  const key = `feature:${old}`;
  try {
    identifier(name, definition.value.selection.feature_choices, old);
    clearError(key);
    localFamily(`feature-values:${old}`, `feature-values:${name}`);
    update((d) => {
      const values = d.selection.feature_choices[old];
      delete d.selection.feature_choices[old];
      d.selection.feature_choices[name] = values;
      d.prize_tiers = d.prize_tiers.map((t) => ({
        ...t,
        condition: mapCondition(t.condition, (c) => {
          if (c.selection_key === old) c.selection_key = name;
        }),
      }));
    });
  } catch (cause) {
    error(key, cause);
  }
}
function removeFeature(key: string) {
  if (props.disabled) return;
  if (hasCondition((c) => c.selection_key === key)) {
    error(`feature:${key}`, "该特征仍被条件引用，请先调整条件");
    return;
  }
  clearError(`feature:${key}`);
  localFamily(`feature-values:${key}`);
  update((d) => {
    delete d.selection.feature_choices[key];
  });
}
function addTier() {
  if (props.disabled || definition.value.prize_tiers.length >= 32) return;
  update((d) => {
    const existing = Object.fromEntries(
      d.prize_tiers.map((t) => [t.code, true]),
    );
    d.prize_tiers.push({
      code: unique("Tier", existing),
      odds: "2",
      exclusive: true,
      cap_points: null,
      condition: structuredClone(d.prize_tiers[0].condition),
    });
  });
}
function removeTier(index: number) {
  if (props.disabled || definition.value.prize_tiers.length <= 1) return;
  const [removedKey] = tierKeys.value.splice(index, 1);
  const next = { ...conditionValidity.value };
  delete next[removedKey];
  conditionValidity.value = next;
  update((d) => {
    d.prize_tiers.splice(index, 1);
  });
}
function nodeCount(c: RuleCondition): number {
  return 1 + (c.children ?? []).reduce((n, child) => n + nodeCount(child), 0);
}
</script>

<template>
  <div class="definition-editor">
    <p class="hint">
      {{ t("彩种模型固定为", "The game model is fixed as") }}
      {{
        definition.model.model
      }}{{ t("；保存不会直接发布规则。修改选号方式后请检查全部奖级条件，并重新验证。", ". Saving does not publish the rule. After changing selection, review all prize-tier conditions and validate again.") }}
    </p>
    <p v-if="structuralError" role="alert" class="error">
      {{ errorText(structuralError) }}{{ t("（服务端验证仍是最终依据）", " (server validation remains authoritative)") }}
    </p>
    <p v-for="(message, key) in errors" :key="key" role="alert" class="error">
      {{ errorText(message) }}
    </p>
    <fieldset :disabled="disabled" class="grid">
      <legend>{{ t("积分、限额与舍入", "Points, limits, and rounding") }}</legend>
      <label
        >{{ t("单位积分", "Unit points") }}<input
          :value="definition.unit_points"
          inputmode="numeric"
          @input="
            update((d) => {
              d.unit_points = ($event.target as HTMLInputElement).value;
            })
          "
      /></label>
      <label
        >{{ t("中奖总封顶（空白不限）", "Total prize cap (blank for unlimited)") }}<input
          :value="definition.cap_points ?? ''"
          inputmode="numeric"
          @input="
            update((d) => {
              d.cap_points = ($event.target as HTMLInputElement).value || null;
            })
          "
      /></label>
      <label
        >{{ t("单注投注限额（空白不限）", "Per-bet limit (blank for unlimited)") }}<input
          :value="definition.limits.max_bet_points ?? ''"
          inputmode="numeric"
          @input="
            update((d) => {
              d.limits.max_bet_points =
                ($event.target as HTMLInputElement).value || null;
            })
          "
      /></label>
      <label
        >{{ t("最大倍投", "Maximum multiplier") }}<input
          :value="definition.limits.max_multiplier"
          inputmode="numeric"
          @input="
            update((d) => {
              d.limits.max_multiplier = (
                $event.target as HTMLInputElement
              ).value;
            })
          "
      /></label>
      <label
        >{{ t("最大组合数（1–10000）", "Maximum combinations (1–10,000)") }}<input
          :value="definition.limits.max_combinations"
          inputmode="numeric"
          @change="
            integer(
              ($event.target as HTMLInputElement).value,
              'max-combinations',
              (d, n) => {
                d.limits.max_combinations = n;
              },
            )
          "
      /></label>
      <label
        >{{ t("舍入层级", "Rounding scope") }}<select
          :value="definition.rounding_scope"
          @change="
            update((d) => {
              d.rounding_scope = ($event.target as HTMLSelectElement)
                .value as RuleDefinition['rounding_scope'];
            })
          "
        >
          <option value="order">{{ t("整注汇总四舍五入", "Round after summing the order") }}</option>
          <option value="line">{{ t("每组合四舍五入", "Round each combination") }}</option>
          <option value="tier">{{ t("每奖级四舍五入", "Round each prize tier") }}</option>
        </select></label
      >
      <label class="wide"
        >{{ t("混合排他/累加策略", "Mixed exclusive/additive policy") }}<select
          :value="definition.mixed_tier_policy"
          @change="
            update((d) => {
              d.mixed_tier_policy = ($event.target as HTMLSelectElement)
                .value as RuleDefinition['mixed_tier_policy'];
            })
          "
        >
          <option value="">{{ t("仅同类奖级（混合时必须选择策略）", "Same-type tiers only (choose a policy for mixed tiers)") }}</option>
          <option value="max_all">
            {{ t("命中排他奖级时，从全部命中奖级取最高金额", "If an exclusive tier wins, use the highest amount among all winning tiers") }}
          </option>
          <option value="max_exclusive_plus_additive">
            {{ t("最高排他金额，加上所有累加奖级", "Add all additive tiers to the highest exclusive amount") }}
          </option>
        </select></label
      >
    </fieldset>
    <fieldset :disabled="disabled" class="grid">
      <legend>{{ t("玩法选号方式", "Play selection mode") }}</legend>
      <label class="wide"
        >{{ t("选号方式", "Selection mode") }}<select
          :value="definition.selection.mode"
          @change="
            setSelectionMode(
              ($event.target as HTMLSelectElement)
                .value as RuleSelectionDefinition['mode'],
            )
          "
        >
          <option value="numbers">{{ t("号码 / 数字位置", "Numbers / digit positions") }}</option>
          <option value="exclude">{{ t("排除号码", "Excluded numbers") }}</option>
          <option value="attributes">{{ t("号码附加属性", "Number attributes") }}</option>
          <option value="features">{{ t("开奖结果特征", "Draw features") }}</option>
        </select></label
      >
      <template
        v-if="
          definition.selection.mode === 'numbers' &&
          definition.model.model !== 'DIGITS_0_9'
        "
      >
        <label
          >{{ t("单线普通选号数", "Regular picks per line") }}<input
            :value="definition.selection.regular_count"
            inputmode="numeric"
            @change="
              integer(
                ($event.target as HTMLInputElement).value,
                'regular-count',
                (d, n) => {
                  d.selection.regular_count = n;
                },
              )
            "
        /></label>
        <label
          >{{ t("单线特别选号数", "Special picks per line") }}<input
            :value="definition.selection.special_count"
            inputmode="numeric"
            @change="
              integer(
                ($event.target as HTMLInputElement).value,
                'special-count',
                (d, n) => {
                  d.selection.special_count = n;
                },
              )
            "
        /></label>
      </template>
      <label v-if="definition.selection.mode === 'exclude'"
        >{{ t("排除号码数量", "Excluded number count") }}<input
          :value="definition.selection.exclude_count"
          inputmode="numeric"
          @change="
            integer(
              ($event.target as HTMLInputElement).value,
              'exclude-count',
              (d, n) => {
                d.selection.exclude_count = n;
              },
            )
          "
      /></label>
      <div v-if="definition.selection.mode === 'attributes'" class="wide">
        <p>{{ t("用户可选择的属性组（至少一个）", "Attribute groups users can select (at least one)") }}</p>
        <label
          v-for="group in Object.keys(definition.number_attributes)"
          :key="group"
          class="check"
          ><input
            type="checkbox"
            :checked="definition.selection.attribute_groups.includes(group)"
            @change="
              selectGroup(group, ($event.target as HTMLInputElement).checked)
            "
          />{{ group }}</label
        >
      </div>
      <div
        v-if="definition.selection.mode === 'features'"
        class="wide feature-groups"
      >
        <article
          v-for="(values, key) in definition.selection.feature_choices"
          :key="key"
          class="grid row"
        >
          <label
            >{{ t("特征代码", "Feature key") }}<input
              :value="key"
              @change="
                renameFeature(
                  String(key),
                  ($event.target as HTMLInputElement).value,
                )
              "
          /></label>
          <label
            >{{ t("可选特征值（逗号分隔）", "Allowed feature values (comma-separated)") }}<input
              :value="csvText(`feature-values:${key}`, values)"
              @input="
                csv(
                  `feature-values:${key}`,
                  ($event.target as HTMLInputElement).value,
                  (d, n) => {
                    d.selection.feature_choices[key] = n;
                  },
                  100,
                )
              "
          /></label>
          <button type="button" @click="removeFeature(String(key))">
            {{ t("删除特征", "Remove feature") }}
          </button>
        </article>
        <button
          type="button"
          :disabled="
            Object.keys(definition.selection.feature_choices).length >= 16
          "
          @click="addFeature"
        >
          {{ t("新增特征", "Add feature") }}
        </button>
      </div>
    </fieldset>
    <fieldset :disabled="disabled">
      <legend>{{ t("号码附加属性（可多组、多值重叠）", "Number attributes (multiple groups and overlapping values are allowed)") }}</legend>
      <article
        v-for="(labels, group) in definition.number_attributes"
        :key="group"
        class="attribute-group"
      >
        <div class="grid">
          <label
            >{{ t("属性组代码", "Attribute group key") }}<input
              :value="group"
              @change="
                renameGroup(
                  String(group),
                  ($event.target as HTMLInputElement).value,
                )
              " /></label
          ><button type="button" @click="removeGroup(String(group))">
            {{ t("删除属性组", "Remove attribute group") }}
          </button>
        </div>
        <div v-for="(numbers, label) in labels" :key="label" class="grid row">
          <label
            >{{ t("属性值代码", "Attribute value key") }}<input
              :value="label"
              @change="
                renameLabel(
                  String(group),
                  String(label),
                  ($event.target as HTMLInputElement).value,
                )
              "
          /></label>
          <label
            >{{ t("属性对应号码（逗号分隔）", "Numbers for this attribute (comma-separated)") }}<input
              :value="csvText(`attribute:${group}:${label}`, numbers)"
              @input="
                csv(
                  `attribute:${group}:${label}`,
                  ($event.target as HTMLInputElement).value,
                  (d, n) => {
                    d.number_attributes[group][label] = n;
                  },
                )
              "
          /></label>
          <button
            type="button"
            :disabled="Object.keys(labels).length <= 1"
            @click="removeLabel(String(group), String(label))"
          >
            {{ t("删除属性值", "Remove attribute value") }}
          </button>
        </div>
        <button
          type="button"
          :disabled="Object.keys(labels).length >= 32"
          @click="addLabel(String(group))"
        >
          {{ t("新增属性值", "Add attribute value") }}
        </button>
      </article>
      <button
        type="button"
        :disabled="Object.keys(definition.number_attributes).length >= 16"
        @click="addGroup"
      >
        {{ t("新增属性组", "Add attribute group") }}
      </button>
    </fieldset>
    <fieldset :disabled="disabled">
      <legend>{{ ui(`奖级与中奖条件 · ${definition.prize_tiers.length} 个`, `Prize tiers and win conditions · ${definition.prize_tiers.length}`) }}</legend>
      <article
        v-for="(tier, index) in definition.prize_tiers"
        :key="tierKeys[index]"
        class="tier-editor"
      >
        <h4>{{ ui(`奖级 ${index + 1}`, `Prize tier ${index + 1}`) }} · {{ tier.code }}</h4>
        <div class="grid">
          <label
            >{{ t("奖级代码", "Prize tier code") }}<input
              :value="tier.code"
              maxlength="48"
              @input="
                update((d) => {
                  d.prize_tiers[index].code = (
                    $event.target as HTMLInputElement
                  ).value;
                })
              "
          /></label>
          <label
            >{{ t("奖级赔率（最多六位小数）", "Prize odds (up to six decimal places)") }}<input
              :value="tier.odds"
              inputmode="decimal"
              @input="
                update((d) => {
                  d.prize_tiers[index].odds = (
                    $event.target as HTMLInputElement
                  ).value;
                })
              "
          /></label>
          <label
            >{{ t("奖级封顶（空白不限）", "Prize cap (blank for unlimited)") }}<input
              :value="tier.cap_points ?? ''"
              inputmode="numeric"
              @input="
                update((d) => {
                  d.prize_tiers[index].cap_points =
                    ($event.target as HTMLInputElement).value || null;
                })
              "
          /></label>
          <label class="check"
            ><input
              type="checkbox"
              :checked="tier.exclusive"
              @change="
                update((d) => {
                  d.prize_tiers[index].exclusive = (
                    $event.target as HTMLInputElement
                  ).checked;
                })
              "
            />{{ t("排他奖级（按金额优先，不按排列顺序）", "Exclusive tier (prioritized by amount, not list order)") }}</label
          >
        </div>
        <RuleConditionEditor
          :model-value="tier.condition"
          :definition="definition"
          :disabled="disabled"
          :total-nodes="nodeCount(tier.condition)"
          @update:model-value="
            (condition) =>
              update((d) => {
                d.prize_tiers[index].condition = condition;
              })
          "
          @validity="
            (valid) => {
              conditionValidity[tierKeys[index]] = valid;
            }
          "
        />
        <button
          type="button"
          :disabled="definition.prize_tiers.length <= 1"
          @click="removeTier(index)"
        >
          {{ t("删除此奖级", "Remove this prize tier") }}
        </button>
      </article>
      <button
        type="button"
        :disabled="definition.prize_tiers.length >= 32"
        @click="addTier"
      >
        {{ t("新增奖级", "Add prize tier") }}
      </button>
    </fieldset>
  </div>
</template>

<style scoped>
.definition-editor {
  display: grid;
  gap: 14px;
  min-width: 0;
}
.grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
  align-items: end;
}
.wide {
  grid-column: 1/-1;
}
fieldset {
  border: 1px solid #e2e6ee;
  border-radius: 8px;
  padding: 14px;
  min-width: 0;
}
legend {
  font-size: 12px;
  font-weight: 700;
  color: #424e67;
}
label {
  display: grid;
  gap: 5px;
  font-size: 11px;
  color: #4f5a6d;
}
input,
select {
  width: 100%;
  min-width: 0;
  border: 1px solid #dce1eb;
  border-radius: 5px;
  padding: 8px;
  background: #fff;
  font: inherit;
}
button {
  border: 1px solid #dce1eb;
  border-radius: 5px;
  padding: 8px;
  background: #f8f9fc;
  font-size: 11px;
  color: #43516b;
}
.check {
  display: flex;
  align-items: center;
  gap: 8px;
}
.check input {
  width: auto;
}
.row {
  margin: 10px 0;
  padding: 10px;
  border: 1px solid #e7eaf0;
  border-radius: 6px;
}
.attribute-group,
.tier-editor {
  border: 1px solid #e2e6ee;
  border-radius: 7px;
  padding: 12px;
  margin: 10px 0;
}
.tier-editor h4 {
  margin: 0 0 12px;
  font-size: 13px;
}
.hint {
  font-size: 11px;
  color: #707a8e;
  line-height: 1.6;
}
.error {
  font-size: 11px;
  color: #b44a43;
  overflow-wrap: anywhere;
}
button:disabled,
input:disabled,
select:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}
@media (max-width: 650px) {
  .grid {
    grid-template-columns: minmax(0, 1fr);
  }
  .wide {
    grid-column: auto;
  }
  fieldset {
    padding: 10px;
  }
  .attribute-group,
  .tier-editor {
    padding: 9px;
  }
}
</style>
