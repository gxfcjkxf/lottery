<script setup lang="ts">
import { computed, ref, watch } from "vue";
import type { RuleDefinition } from "./rule-simulation-api";
import type { RuleValidationCase } from "./rule-versions-api";
import {
  defaultRuleCase,
  normalizeRuleCase,
  validateRuleCases,
} from "./rule-cases-editor";
import { useAdminI18n } from "./i18n";
const { t } = useAdminI18n();
const ui = (zh: string, en: string = zh) => t(zh, en);
const localError = (value: string) => {
  const pairs: Record<string, string> = {
    "Invalid validation cases.": "验证用例无效。",
    "Provide between 1 and 32 validation cases.": "请提供 1 至 32 个验证用例。",
    "A validation case must include a name.": "验证用例必须填写名称。",
    "Validation case names must be unique.": "验证用例名称必须唯一。",
    "Digit selections must contain one candidate list per position.": "数字选号必须为每个位置提供一个候选列表。",
    "Digit candidates must be from 0 to 9.": "数字候选必须在 0 至 9 之间。",
    "Number selections contain fields that do not apply to this mode.": "数字选号包含当前模式不适用的字段。",
    "Exclude selection contains fields that do not apply to this mode.": "排除选号包含当前模式不适用的字段。",
    "Excluded candidates contain values outside the model pools.": "排除候选包含号码池之外的值。",
    "Select values for every configured attribute group.": "请为每个已配置的属性组选择值。",
    "Select values for every configured feature.": "请为每个已配置的特征选择值。",
    "The definition pool cannot satisfy the draw count.": "规则号码池无法满足开奖数量。",
    "The definition pool is empty.": "规则号码池为空。",
    "A validation case is required.": "必须提供验证用例。",
    "Expected won must be true or false.": "预期中奖状态必须为 true 或 false。",
    "Multiplier exceeds the definition limit.": "倍投超过规则定义的上限。",
    "A complete rule definition is required.": "必须提供完整的规则定义。",
    "M-select pools and model cannot allow repeated numbers.": "M 选 N 的号码池和模型均不能允许号码重复。",
    "Unsupported rule model.": "不支持的号码模型。",
    "Digit number selection cannot use regular or special counts.": "数字选号不能配置普通号或特别号数量。",
    "Draw digits must be from 0 to 9.": "开奖数字必须在 0 至 9 之间。",
    "A number draw cannot contain digit positions.": "号码型开奖不能包含数字位置。",
    "M-select regular and special draw values must be distinct.": "M 选 N 的普通号与特别号开奖结果不能重复。",
    "Case names must be unique.": "用例名称必须唯一。",
    "M-select definition pool cannot supply distinct draw values.": "M 选 N 的定义号码池无法提供互不重复的开奖结果。",
    "Feature selection contains fields that do not apply to this mode.": "特征选号包含当前模式不适用的字段。",
  };
  const known = pairs[value];
  if (known) return t(known, value);
  const label = value.match(/^(.+?): (.+)$/);
  if (label) {
    const labels: Record<string, string> = {
      regularCandidates: "普通号候选", specialCandidates: "特别号候选", excludeCandidates: "排除号码",
      regularDraw: "开奖普通号", specialDraw: "开奖特别号", digitDraw: "开奖数字",
      multiplier: "验证倍投", expected_bet_points: "预期投注积分", expected_prize_points: "预期中奖积分",
    };
    const zh = labels[label[1]!] ?? label[1]!;
    const rest = label[2]!;
    const replacements: Record<string, string> = {
      "单项输入最长 2000 字符。": "单项输入最长 2000 个字符。", "单项最多 100 个输入值。": "单项最多可输入 100 个值。",
      "enter comma-separated canonical non-negative integers.": "请输入逗号分隔的规范非负整数。", "values must be safe integers.": "所有值必须是安全整数。",
    };
    return t(`${zh}：${replacements[rest] ?? rest}`, `${label[1]}: ${rest}`);
  }
  const digitCount = value.match(/^Digit draw must contain exactly (\d+) digits\.$/);
  if (digitCount) return t(`开奖数字必须正好包含 ${digitCount[1]} 位。`, value);
  return value;
};

const props = defineProps<{
  definition: RuleDefinition;
  modelValue: RuleValidationCase[];
  disabled?: boolean;
}>();
const emit = defineEmits<{
  (event: "update:modelValue", value: RuleValidationCase[]): void;
  (event: "validity", value: boolean): void;
}>();

interface CaseDraft {
  value: RuleValidationCase;
  regularCandidates: string;
  specialCandidates: string;
  digitCandidates: string[];
  featureCandidates: Record<string, string>;
  excludeCandidates: string;
  regularDraw: string;
  specialDraw: string;
  digitDraw: string;
}

const drafts = ref<CaseDraft[]>([]);
const inputErrors = ref<Record<string, string>>({});
const caseLimit = 32;

function csv(values: number[]): string {
  return values.join(",");
}

function draftFromCase(input: RuleValidationCase): CaseDraft {
  const value = normalizeRuleCase(input);
  return {
    value,
    regularCandidates: csv(value.selection.regular),
    specialCandidates: csv(value.selection.special),
    digitCandidates: value.selection.digits.map(csv),
    featureCandidates: Object.fromEntries(
      Object.entries(value.selection.features).map(([key, values]) => [
        key,
        csv(values),
      ]),
    ),
    excludeCandidates: csv(value.selection.exclude),
    regularDraw: csv(value.draw.regular),
    specialDraw: csv(value.draw.special),
    digitDraw: csv(value.draw.digits),
  };
}

watch(
  () => props.modelValue,
  (cases) => {
    drafts.value = Array.isArray(cases) ? cases.map(draftFromCase) : [];
    inputErrors.value = {};
  },
  { immediate: true, deep: true },
);

const selection = computed(() => props.definition.selection);
const model = computed(() => props.definition.model);
const attributeGroups = computed(() => selection.value.attribute_groups ?? []);
const featureKeys = computed(() =>
  Object.keys(selection.value.feature_choices ?? {}),
);
const attributeChoices = (group: string) =>
  Object.keys(props.definition.number_attributes?.[group] ?? {});
const featureChoices = (key: string) =>
  selection.value.feature_choices?.[key] ?? [];
const localErrors = computed(() => {
  const messages = Object.values(inputErrors.value);
  if (!messages.length && drafts.value.length) {
    try {
      validateRuleCases(
        props.definition,
        drafts.value.map((draft) => draft.value),
      );
    } catch (error) {
      messages.push(
        error instanceof Error ? error.message : "Invalid validation cases.",
      );
    }
  } else if (!messages.length && drafts.value.length === 0) {
    messages.push("Provide between 1 and 32 validation cases.");
  }
  return [...new Set(messages)];
});
const casesValid = computed(() => localErrors.value.length === 0);
watch(casesValid, (valid) => emit("validity", valid), { immediate: true });

function parseCsv(text: string, label: string): number[] {
  if (!text.trim()) return [];
  if (text.length > 2000) throw new Error(`${label}: 单项输入最长 2000 字符。`);
  const tokens = text.split(",").map((part) => part.trim());
  if (tokens.length > 100) throw new Error(`${label}: 单项最多 100 个输入值。`);
  if (tokens.some((token) => !/^(0|[1-9][0-9]*)$/.test(token)))
    throw new Error(
      `${label}: enter comma-separated canonical non-negative integers.`,
    );
  const values = tokens.map(Number);
  if (values.some((value) => !Number.isSafeInteger(value)))
    throw new Error(`${label}: values must be safe integers.`);
  return values;
}

function publishUserEdit() {
  // Structural actions must reparse the visible CSV, not resurrect its last
  // valid value after a failed edit. Preserve invalid drafts until corrected.
  const reparsedErrors: Record<string, string> = {};
  for (const [index, draft] of drafts.value.entries()) {
    const parse = (
      key: string,
      text: string,
      apply: (values: number[]) => void,
    ) => {
      try {
        apply(parseCsv(text, key));
      } catch (cause) {
        reparsedErrors[`${index}:${key}`] =
          cause instanceof Error ? cause.message : "无效号码输入";
      }
    };
    parse("regularCandidates", draft.regularCandidates, (n) => {
      draft.value.selection.regular = n;
    });
    parse("specialCandidates", draft.specialCandidates, (n) => {
      draft.value.selection.special = n;
    });
    parse("excludeCandidates", draft.excludeCandidates, (n) => {
      draft.value.selection.exclude = n;
    });
    parse("regularDraw", draft.regularDraw, (n) => {
      draft.value.draw.regular = n;
    });
    parse("specialDraw", draft.specialDraw, (n) => {
      draft.value.draw.special = n;
    });
    parse("digitDraw", draft.digitDraw, (n) => {
      draft.value.draw.digits = n;
    });
    draft.digitCandidates.forEach((text, position) =>
      parse(`digitCandidates:${position}`, text, (n) => {
        draft.value.selection.digits[position] = n;
      }),
    );
    for (const [key, text] of Object.entries(draft.featureCandidates))
      parse(`feature:${key}`, text, (n) => {
        draft.value.selection.features[key] = n;
      });
  }
  inputErrors.value = reparsedErrors;
  const errors = Object.values(inputErrors.value);
  let valid = false;
  if (!errors.length) {
    try {
      validateRuleCases(
        props.definition,
        drafts.value.map((draft) => draft.value),
      );
      valid = true;
    } catch (error) {
      errors.push(
        error instanceof Error ? error.message : "Invalid validation cases.",
      );
    }
  }
  const parseable = Object.keys(inputErrors.value).length === 0;
  emit("validity", valid);
  if (parseable)
    emit(
      "update:modelValue",
      drafts.value.map(
        (draft) =>
          JSON.parse(JSON.stringify(draft.value)) as RuleValidationCase,
      ),
    );
}

function changeText(
  index: number,
  field: string,
  text: string,
  apply: (draft: CaseDraft) => void,
) {
  const draft = drafts.value[index];
  if (!draft || props.disabled) return;
  const key = `${index}:${field}`;
  try {
    apply(draft);
    delete inputErrors.value[key];
  } catch (error) {
    inputErrors.value[key] =
      error instanceof Error ? error.message : "Invalid value.";
  }
  publishUserEdit();
}

function textField(
  index: number,
  field:
    "name" | "multiplier" | "expected_bet_points" | "expected_prize_points",
  value: string,
) {
  changeText(index, field, value, (draft) => {
    draft.value[field] = value;
  });
}

function csvField(
  index: number,
  field:
    | "regularCandidates"
    | "specialCandidates"
    | "excludeCandidates"
    | "regularDraw"
    | "specialDraw"
    | "digitDraw",
  value: string,
) {
  changeText(index, field, value, (draft) => {
    draft[field] = value;
    const values = parseCsv(value, fieldLabel(field));
    if (field === "regularCandidates") draft.value.selection.regular = values;
    else if (field === "specialCandidates")
      draft.value.selection.special = values;
    else if (field === "excludeCandidates")
      draft.value.selection.exclude = values;
    else if (field === "regularDraw") draft.value.draw.regular = values;
    else if (field === "specialDraw") draft.value.draw.special = values;
    else draft.value.draw.digits = values;
  });
}

function digitCandidate(index: number, position: number, text: string) {
  changeText(index, `digitCandidates:${position}`, text, (draft) => {
    draft.digitCandidates[position] = text;
    draft.value.selection.digits[position] = parseCsv(
      text,
      `Position ${position + 1}`,
    );
  });
}

function featureCandidate(index: number, key: string, text: string) {
  changeText(index, `feature:${key}`, text, (draft) => {
    draft.featureCandidates[key] = text;
    draft.value.selection.features[key] = parseCsv(text, `Feature ${key}`);
  });
}

function fieldLabel(field: string) {
  return (
    (
      {
        regularCandidates: "Regular candidates",
        specialCandidates: "Special candidates",
        excludeCandidates: "Excluded candidates",
        regularDraw: "Regular draw",
        specialDraw: "Special draw",
        digitDraw: "Digit draw",
      } as Record<string, string>
    )[field] ?? field
  );
}

function setWon(index: number, won: boolean) {
  if (props.disabled) return;
  const draft = drafts.value[index];
  if (!draft) return;
  draft.value.expected_won = won;
  publishUserEdit();
}

function toggleArray<T extends string | number>(
  values: T[],
  value: T,
  checked: boolean,
): T[] {
  return checked ? [...values, value] : values.filter((item) => item !== value);
}

function toggleAttribute(
  index: number,
  group: string,
  label: string,
  checked: boolean,
) {
  if (props.disabled) return;
  const values = drafts.value[index]?.value.selection.attributes[group];
  if (!drafts.value[index] || !values) return;
  drafts.value[index]!.value.selection.attributes[group] = toggleArray(
    values,
    label,
    checked,
  );
  publishUserEdit();
}

function uniqueName(base: string) {
  const used = new Set(drafts.value.map((draft) => draft.value.name.trim()));
  const bounded = (candidate: string, suffix = "") => {
    const safeBase = candidate.includes("\0") ? "Validation case" : candidate;
    const suffixBytes = new TextEncoder().encode(suffix).length;
    let output = "";
    for (const character of Array.from(safeBase)) {
      if (
        new TextEncoder().encode(output + character).length + suffixBytes >
        120
      )
        break;
      output += character;
    }
    return `${output.trimEnd()}${suffix}`;
  };
  const first = bounded(base);
  if (!used.has(first)) return first;
  for (let suffix = 2; suffix <= caseLimit + 1; suffix++) {
    const ending = ` (${suffix})`;
    const candidate = bounded(base, ending);
    if (!used.has(candidate)) return candidate;
  }
  return bounded(`Validation case ${drafts.value.length + 1}`);
}

function addCase() {
  if (props.disabled || drafts.value.length >= caseLimit) return;
  try {
    const value = defaultRuleCase(props.definition, drafts.value.length + 1);
    value.name = uniqueName(value.name);
    const draft = draftFromCase(value);
    drafts.value.push(draft);
    inputErrors.value = {};
    publishUserEdit();
  } catch (cause) {
    inputErrors.value = {
      ...inputErrors.value,
      action:
        cause instanceof Error ? cause.message : "请先完善规则定义，再添加用例",
    };
    emit("validity", false);
  }
}

function cloneCase(index: number) {
  if (props.disabled || drafts.value.length >= caseLimit) return;
  const source = drafts.value[index];
  if (!source) return;
  const cloned = JSON.parse(JSON.stringify(source)) as CaseDraft;
  cloned.value.name = uniqueName(`${cloned.value.name} copy`);
  drafts.value.splice(index + 1, 0, cloned);
  inputErrors.value = {};
  publishUserEdit();
}

function removeCase(index: number) {
  if (props.disabled) return;
  drafts.value.splice(index, 1);
  inputErrors.value = {};
  publishUserEdit();
}

defineExpose({ localErrors, casesValid });
</script>

<template>
  <section
    class="rule-cases-editor"
    :aria-disabled="disabled ? 'true' : 'false'"
  >
    <header class="cases-header">
      <div>
        <h3>{{ t("完整验证用例（最多 32 组）", "Complete validation cases (up to 32)") }}</h3>
        <p>
          {{ t("预期值由运营独立填写，服务器实际计算核对；初始占位值不代表验证通过。", "Operators enter expected values independently; the server checks its actual calculation. Initial placeholder values do not indicate a passing validation.") }}
        </p>
      </div>
      <button
        type="button"
        :disabled="disabled || drafts.length >= caseLimit"
        @click="addCase"
      >
        {{ t("新增用例", "Add case") }} <span>({{ drafts.length }}/{{ caseLimit }})</span>
      </button>
    </header>

    <p v-if="!drafts.length" class="empty-cases">
      {{ t("暂无用例。草稿准备好后可新增用例。", "No cases yet. Add one when the draft is ready.") }}
    </p>

    <article v-for="(draft, index) in drafts" :key="index" class="case-card">
      <header class="case-card-header">
        <label class="wide-field"
          >{{ t("用例名称", "Case name") }}
          <input
            :value="draft.value.name"
            maxlength="120"
            :disabled="disabled"
            @input="
              textField(
                index,
                'name',
                ($event.target as HTMLInputElement).value,
              )
            "
          />
        </label>
        <div class="case-actions">
          <button
            type="button"
            :disabled="disabled || drafts.length >= caseLimit"
            @click="cloneCase(index)"
          >
            {{ t("复制用例", "Duplicate case") }}
          </button>
          <button type="button" :disabled="disabled" @click="removeCase(index)">
            {{ t("删除用例", "Remove case") }}
          </button>
        </div>
      </header>

      <div
        v-if="selection.mode === 'numbers' && model.model === 'DIGITS_0_9'"
        class="case-grid"
      >
        <label v-for="position in model.length" :key="position"
          >{{ ui(`第 ${position} 位候选数字（逗号分隔）`, `Position ${position} candidate digits (comma-separated)`) }}
          <input
            :value="draft.digitCandidates[position - 1] ?? ''"
            :disabled="disabled"
            :placeholder="t('例如 0,3,8', 'e.g. 0,3,8')"
            @input="
              digitCandidate(
                index,
                position - 1,
                ($event.target as HTMLInputElement).value,
              )
            "
          />
        </label>
      </div>

      <div v-else-if="selection.mode === 'numbers'" class="case-grid">
        <label v-if="selection.regular_count"
          >{{ ui(`普通号候选（单线选 ${selection.regular_count} 个）`, `Regular candidates (choose ${selection.regular_count} per line)`) }}
          <input
            :value="draft.regularCandidates"
            :disabled="disabled"
            :placeholder="t('逗号分隔的号码池值', 'comma-separated pool values')"
            @input="
              csvField(
                index,
                'regularCandidates',
                ($event.target as HTMLInputElement).value,
              )
            "
          />
        </label>
        <label v-if="selection.special_count"
          >{{ ui(`特别号候选（单线选 ${selection.special_count} 个）`, `Special candidates (choose ${selection.special_count} per line)`) }}
          <input
            :value="draft.specialCandidates"
            :disabled="disabled"
            :placeholder="t('逗号分隔的号码池值', 'comma-separated pool values')"
            @input="
              csvField(
                index,
                'specialCandidates',
                ($event.target as HTMLInputElement).value,
              )
            "
          />
        </label>
      </div>

      <div v-else-if="selection.mode === 'exclude'" class="case-grid">
        <label
          >{{ ui(`排除号码（恰好 ${selection.exclude_count} 个）`, `Excluded numbers (exactly ${selection.exclude_count})`) }}
          <input
            :value="draft.excludeCandidates"
            :disabled="disabled"
            :placeholder="t('逗号分隔的号码池值', 'comma-separated pool values')"
            @input="
              csvField(
                index,
                'excludeCandidates',
                ($event.target as HTMLInputElement).value,
              )
            "
          />
        </label>
      </div>

      <div v-else-if="selection.mode === 'attributes'" class="option-groups">
        <fieldset
          v-for="group in attributeGroups"
          :key="group"
          :disabled="disabled"
        >
          <legend>{{ group }}</legend>
          <label
            v-for="label in attributeChoices(group)"
            :key="label"
            class="check-option"
          >
            <input
              type="checkbox"
              :checked="
                draft.value.selection.attributes[group]?.includes(label)
              "
              @change="
                toggleAttribute(
                  index,
                  group,
                  label,
                  ($event.target as HTMLInputElement).checked,
                )
              "
            />{{ label }}
          </label>
        </fieldset>
      </div>

      <div v-else-if="selection.mode === 'features'" class="case-grid">
        <label v-for="key in featureKeys" :key="key"
          >{{ ui(`特征 ${key} 候选值（允许：${featureChoices(key).join(", ")}）`, `Feature ${key} candidates (allowed: ${featureChoices(key).join(", ")})`) }}
          <input
            :value="draft.featureCandidates[key] ?? ''"
            :disabled="disabled"
            :placeholder="t('逗号分隔的允许值', 'comma-separated allowed values')"
            @input="
              featureCandidate(
                index,
                key,
                ($event.target as HTMLInputElement).value,
              )
            "
          />
        </label>
      </div>

      <div v-if="model.model === 'DIGITS_0_9'" class="case-grid">
        <label
          >{{ ui(`开奖数字（${model.length} 位，逗号分隔）`, `Draw digits (${model.length} positions, comma-separated)`) }}
          <input
            :value="draft.digitDraw"
            :disabled="disabled"
            :placeholder="t('例如 0,3,8', 'e.g. 0,3,8')"
            @input="
              csvField(
                index,
                'digitDraw',
                ($event.target as HTMLInputElement).value,
              )
            "
          />
        </label>
      </div>
      <div v-else class="case-grid">
        <label v-if="model.regular_count"
          >{{ ui(`开奖普通号（${model.regular_count} 个）`, `Regular draw (${model.regular_count} numbers)`) }}
          <input
            :value="draft.regularDraw"
            :disabled="disabled"
            :placeholder="t('逗号分隔的开奖结果', 'comma-separated draw values')"
            @input="
              csvField(
                index,
                'regularDraw',
                ($event.target as HTMLInputElement).value,
              )
            "
          />
        </label>
        <label v-if="model.special_count"
          >{{ ui(`开奖特别号（${model.special_count} 个）`, `Special draw (${model.special_count} numbers)`) }}
          <input
            :value="draft.specialDraw"
            :disabled="disabled"
            :placeholder="t('逗号分隔的开奖结果', 'comma-separated draw values')"
            @input="
              csvField(
                index,
                'specialDraw',
                ($event.target as HTMLInputElement).value,
              )
            "
          />
        </label>
      </div>

      <div class="case-grid expected-fields">
        <label
          >{{ ui(`验证倍投（1–${definition.limits.max_multiplier}）`, `Validation multiplier (1–${definition.limits.max_multiplier})`) }}
          <input
            :value="draft.value.multiplier"
            inputmode="numeric"
            :disabled="disabled"
            @input="
              textField(
                index,
                'multiplier',
                ($event.target as HTMLInputElement).value,
              )
            "
          />
        </label>
        <label
          >{{ t("预期投注积分", "Expected bet points") }}
          <input
            :value="draft.value.expected_bet_points"
            inputmode="numeric"
            :disabled="disabled"
            @input="
              textField(
                index,
                'expected_bet_points',
                ($event.target as HTMLInputElement).value,
              )
            "
          />
        </label>
        <label
          >{{ t("预期中奖积分", "Expected prize points") }}
          <input
            :value="draft.value.expected_prize_points"
            inputmode="numeric"
            :disabled="disabled"
            @input="
              textField(
                index,
                'expected_prize_points',
                ($event.target as HTMLInputElement).value,
              )
            "
          />
        </label>
        <label class="check-option won-option"
          ><input
            type="checkbox"
            :checked="draft.value.expected_won"
            :disabled="disabled"
            @change="setWon(index, ($event.target as HTMLInputElement).checked)"
          />{{ t("预期中奖", "Expected win") }}</label
        >
      </div>
    </article>

    <ul v-if="localErrors.length" class="case-errors" aria-live="polite">
      <li v-for="(error, index) in localErrors" :key="index">{{ localError(error) }}</li>
    </ul>
  </section>
</template>

<style scoped>
.rule-cases-editor {
  display: grid;
  gap: 14px;
}
.cases-header,
.case-card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.cases-header h3 {
  margin: 0;
  font-size: 1rem;
}
.cases-header p,
.empty-cases {
  margin: 4px 0 0;
  color: var(--muted, #687386);
  font-size: 0.88rem;
}
.case-card {
  display: grid;
  gap: 14px;
  padding: 16px;
  border: 1px solid var(--border, #d8dee8);
  border-radius: 12px;
}
.case-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(210px, 1fr));
  gap: 12px;
}
.case-grid label,
.wide-field {
  display: grid;
  gap: 6px;
  font-size: 0.88rem;
  font-weight: 600;
}
.case-grid input,
.wide-field input {
  min-width: 0;
  width: 100%;
  box-sizing: border-box;
}
.wide-field {
  flex: 1;
  max-width: 480px;
}
.case-actions {
  display: flex;
  gap: 8px;
}
.option-groups {
  display: grid;
  gap: 10px;
}
.option-groups fieldset {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 16px;
  border: 1px solid var(--border, #d8dee8);
  border-radius: 8px;
}
.option-groups legend {
  padding: 0 5px;
  font-weight: 600;
}
.check-option {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.won-option {
  align-self: end;
  min-height: 40px;
}
.case-errors {
  margin: 0;
  padding: 10px 12px 10px 30px;
  color: #a52424;
  background: #fff1f0;
  border-radius: 8px;
  font-size: 0.88rem;
}
button {
  cursor: pointer;
}
button:disabled,
input:disabled {
  cursor: not-allowed;
  opacity: 0.65;
}
@media (max-width: 560px) {
  .case-card-header {
    align-items: stretch;
    flex-direction: column;
  }
  .case-actions {
    justify-content: flex-end;
  }
}
</style>
