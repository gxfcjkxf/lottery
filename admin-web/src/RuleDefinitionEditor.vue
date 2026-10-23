<script setup lang="ts">
import BusinessDetails from '@lottery/shared/BusinessDetails.vue'
import { computed, ref, watch } from "vue";
import RuleConditionEditor from "./RuleConditionEditor.vue";
import type {
  RuleCondition,
  RuleDefinition,
  RuleSelectionDefinition,
} from "./rule-simulation-api";
import { parseEditorNumbers, validateEditorDefinition } from "./rule-editor";
import { adminKey, useAdminI18n } from "./i18n";
const { t, display, locale } = useAdminI18n();
const errorEnglish: Record<string, string> = {
  "规则定义无效": "admin.rule.definition.invalid", "请输入规范非负整数（最大 20000000）": "admin.rule.definition.integer",
  "代码须以字母开头，最多 48 个字母、数字或下划线": "admin.rule.definition.code", "代码已存在": "admin.rule.definition.codeExists",
  "该属性组仍被选号或条件引用，请先移除引用": "admin.rule.definition.groupReferenced", "该属性值仍被条件引用，请先修改条件": "admin.rule.definition.valueReferenced",
  "该特征仍被条件引用，请先调整条件": "admin.rule.definition.featureReferenced", "号码列表不能为空。": "admin.rule.definition.listEmpty",
  "号码列表无效": "admin.rule.definition.listInvalid", "积分必须是规范的非负整数字符串，不得含小数、符号或前导零。": "admin.rule.definition.points",
  "号码输入上限必须为 0–10,000 的整数。": "admin.rule.definition.inputLimit", "投注方式格式无效。": "admin.rule.definition.selectionMode",
  "投注数量或排除数量超出模型范围。": "admin.rule.definition.selectionCount", "特征选项格式无效。": "admin.rule.definition.featureChoice",
  "号码属性格式无效。": "admin.rule.definition.attributes", "号码属性组最多 16 个。": "admin.rule.definition.attributeGroups",
  "数字模型的开奖范围必须为 all 或 digits。": "admin.rule.definition.drawTarget", "非数字模型不能使用 digits 开奖范围。": "admin.rule.definition.nonDigitTarget",
  "条件必须指定有效的开奖范围。": "admin.rule.definition.validTarget", "条件字段无效。": "admin.rule.definition.field",
  "条件运算符无效。": "admin.rule.definition.operator", "叶条件不能包含子条件。": "admin.rule.definition.leaf",
  "条件嵌套深度不能超过 8。": "admin.rule.definition.depth", "每个奖级条件最多 128 个节点。": "admin.rule.definition.nodes",
  "规则定义格式无效。": "admin.rule.definition.format", "schema_version 必须为 1。": "admin.rule.definition.schema",
  "rounding 必须为 half_up。": "admin.rule.definition.rounding", "限额格式无效。": "admin.rule.definition.limit",
  "混合奖级策略无效。": "admin.rule.definition.mixed", "奖级格式无效。": "admin.rule.definition.tier",
  "同时存在排他和非排他奖级时必须显式选择 mixed_tier_policy。": "admin.rule.definition.mixedRequired",
  "赔率必须为正的精确十进制字符串，最多 10 位整数和 6 位小数。": "admin.rule.definition.odds",
};
const errorText = (value: string) => {
  const duplicate = value.match(/^号码 (\d+) 重复。$/);
  if (duplicate) return t('admin.rule.case.duplicate', { number: duplicate[1]! });
  const tooMany = value.match(/^号码数量不能超过 (\d+) 个。$/);
  if (tooMany) return t('admin.rule.case.tooMany', { count: tooMany[1]! });
  const labelError = value.match(/^(号码|属性组|属性) (.+?)(格式无效。|含不属于模型号码池的号码。|不存在或重复。)$/);
  if (labelError) {
    const prefix = labelError[1] === "号码" ? "Number attribute" : labelError[1] === "属性组" ? "Attribute group" : "Attribute";
    const suffix: Record<string, string> = { "格式无效。": "has an invalid format.", "含不属于模型号码池的号码。": "contains numbers outside the model pools.", "不存在或重复。": "does not exist or is duplicated." };
    return display(value.replace(`${labelError[1]} `, `${prefix} `).replace(labelError[3]!, suffix[labelError[3]!]!));
  }
  return errorEnglish[value] ? t(adminKey(errorEnglish[value]!)) : display(value);
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
      {{ t('admin.copy.e1979ba1') }}
      <BusinessDetails :value="definition.model.model" :locale="locale" />{{ t('admin.copy.03a52837') }}
    </p>
    <p v-if="structuralError" role="alert" class="error">
      {{ errorText(structuralError) }}{{ t('admin.copy.ed8fcbd0') }}
    </p>
    <p v-for="(message, key) in errors" :key="key" role="alert" class="error">
      {{ errorText(message) }}
    </p>
    <fieldset :disabled="disabled" class="grid">
      <legend>{{ t('admin.copy.390c2830') }}</legend>
      <label
        >{{ t('admin.copy.c4755617') }}<input
          :value="definition.unit_points"
          inputmode="numeric"
          @input="
            update((d) => {
              d.unit_points = ($event.target as HTMLInputElement).value;
            })
          "
      /></label>
      <label
        >{{ t('admin.copy.624e2a40') }}<input
          :value="definition.cap_points ?? ''"
          inputmode="numeric"
          @input="
            update((d) => {
              d.cap_points = ($event.target as HTMLInputElement).value || null;
            })
          "
      /></label>
      <label
        >{{ t('admin.copy.89643bfe') }}<input
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
        >{{ t('admin.copy.49aecb4b') }}<input
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
        >{{ t('admin.copy.26d15090') }}<input
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
        >{{ t('admin.copy.b4b2c4bc') }}<select
          :value="definition.rounding_scope"
          @change="
            update((d) => {
              d.rounding_scope = ($event.target as HTMLSelectElement)
                .value as RuleDefinition['rounding_scope'];
            })
          "
        >
          <option value="order">{{ t('admin.copy.e238ef73') }}</option>
          <option value="line">{{ t('admin.copy.862d826d') }}</option>
          <option value="tier">{{ t('admin.copy.ed114269') }}</option>
        </select></label
      >
      <label class="wide"
        >{{ t('admin.copy.f268e873') }}<select
          :value="definition.mixed_tier_policy"
          @change="
            update((d) => {
              d.mixed_tier_policy = ($event.target as HTMLSelectElement)
                .value as RuleDefinition['mixed_tier_policy'];
            })
          "
        >
          <option value="">{{ t('admin.copy.692dbfe9') }}</option>
          <option value="max_all">
            {{ t('admin.copy.0684e474') }}
          </option>
          <option value="max_exclusive_plus_additive">
            {{ t('admin.copy.d0df01c3') }}
          </option>
        </select></label
      >
    </fieldset>
    <fieldset :disabled="disabled" class="grid">
      <legend>{{ t('admin.copy.aa6a03f8') }}</legend>
      <label class="wide"
        >{{ t('admin.copy.bb8a9ae6') }}<select
          :value="definition.selection.mode"
          @change="
            setSelectionMode(
              ($event.target as HTMLSelectElement)
                .value as RuleSelectionDefinition['mode'],
            )
          "
        >
          <option value="numbers">{{ t('admin.copy.0b0ea490') }}</option>
          <option value="exclude">{{ t('admin.copy.3eebf4d9') }}</option>
          <option value="attributes">{{ t('admin.copy.e29fe34c') }}</option>
          <option value="features">{{ t('admin.copy.b13dda6f') }}</option>
        </select></label
      >
      <template
        v-if="
          definition.selection.mode === 'numbers' &&
          definition.model.model !== 'DIGITS_0_9'
        "
      >
        <label
          >{{ t('admin.copy.c2bb9364') }}<input
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
          >{{ t('admin.copy.f100a9dd') }}<input
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
        >{{ t('admin.copy.9fb60612') }}<input
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
        <p>{{ t('admin.copy.bf891e58') }}</p>
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
            >{{ t('admin.copy.0ef02c76') }}<input
              :value="key"
              @change="
                renameFeature(
                  String(key),
                  ($event.target as HTMLInputElement).value,
                )
              "
          /></label>
          <label
            >{{ t('admin.copy.5d658fda') }}<input
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
            {{ t('admin.copy.5d7289d7') }}
          </button>
        </article>
        <button
          type="button"
          :disabled="
            Object.keys(definition.selection.feature_choices).length >= 16
          "
          @click="addFeature"
        >
          {{ t('admin.copy.605cac28') }}
        </button>
      </div>
    </fieldset>
    <fieldset :disabled="disabled">
      <legend>{{ t('admin.copy.457fd121') }}</legend>
      <article
        v-for="(labels, group) in definition.number_attributes"
        :key="group"
        class="attribute-group"
      >
        <div class="grid">
          <label
            >{{ t('admin.copy.c8fde3be') }}<input
              :value="group"
              @change="
                renameGroup(
                  String(group),
                  ($event.target as HTMLInputElement).value,
                )
              " /></label
          ><button type="button" @click="removeGroup(String(group))">
            {{ t('admin.copy.a0ba90f5') }}
          </button>
        </div>
        <div v-for="(numbers, label) in labels" :key="label" class="grid row">
          <label
            >{{ t('admin.copy.65a61248') }}<input
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
            >{{ t('admin.copy.2cbc95f3') }}<input
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
            {{ t('admin.copy.c63a9233') }}
          </button>
        </div>
        <button
          type="button"
          :disabled="Object.keys(labels).length >= 32"
          @click="addLabel(String(group))"
        >
          {{ t('admin.copy.0a40b982') }}
        </button>
      </article>
      <button
        type="button"
        :disabled="Object.keys(definition.number_attributes).length >= 16"
        @click="addGroup"
      >
        {{ t('admin.copy.c27ce3bc') }}
      </button>
    </fieldset>
    <fieldset :disabled="disabled">
      <legend>{{ t('admin.rule.tier.heading', { count: definition.prize_tiers.length }) }}</legend>
      <article
        v-for="(tier, index) in definition.prize_tiers"
        :key="tierKeys[index]"
        class="tier-editor"
      >
        <h4>{{ t('admin.rule.tier.title', { index: index + 1 }) }} · {{ tier.code }}</h4>
        <div class="grid">
          <label
            >{{ t('admin.copy.4428e8e3') }}<input
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
            >{{ t('admin.copy.5531bbe1') }}<input
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
            >{{ t('admin.copy.9c8ad07b') }}<input
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
            />{{ t('admin.copy.edc464fc') }}</label
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
          {{ t('admin.copy.76e2f874') }}
        </button>
      </article>
      <button
        type="button"
        :disabled="definition.prize_tiers.length >= 32"
        @click="addTier"
      >
        {{ t('admin.copy.6c596655') }}
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
  background: var(--surface);
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
