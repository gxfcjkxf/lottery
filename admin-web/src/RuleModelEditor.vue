<script setup lang="ts">
import { computed, ref, watch } from "vue";
import type { RuleModel, RuleModelName, RulePool } from "./rule-simulation-api";
import { parseEditorNumbers, validateEditorModel } from "./rule-editor";
import { adminKey, useAdminI18n } from "./i18n";
const { t, display } = useAdminI18n();
const errorEnglish: Record<string, string> = {
  "号码模型无效": "admin.rule.model.invalid", "请输入 0–1000000 的规范整数": "admin.rule.model.integer",
  "切换到号码列表前，请将号码范围调整到 10000 个以内": "admin.rule.model.reduceRange",
  "号码列表须非空、不重复，每个号码不超过 1000000": "admin.rule.model.listNonEmpty", "号码列表无效": "admin.rule.model.listInvalid",
  "彩种模型格式无效。": "admin.rule.model.format", "彩种模型类型无效。": "admin.rule.model.type",
  "号码范围必须在 0–1,000,000 内且最小值不大于最大值。": "admin.rule.model.range",
  "重复设置无效。": "admin.rule.model.repeat", "号码列表最多 10,000 个。": "admin.rule.model.maxList",
  "不能同时设置范围和号码列表。": "admin.rule.model.rangeAndList", "号码必须唯一且在 0–1,000,000 内。": "admin.rule.model.uniqueRange",
  "号码池大小必须为 1–10,000。": "admin.rule.model.poolSize", "模型的重复或有序设置无效。": "admin.rule.model.repeatOrdering",
  "号码池不能为空。": "admin.rule.model.poolEmpty", "号码数量超过不重复号码池容量。": "admin.rule.model.countExceeds",
  "M 选 N 的普通池和特别池必须是相同的 M 个号码。": "admin.rule.model.samePools",
  "X+Y 模型数量或专属字段无效。": "admin.rule.model.xyInvalid",
  "M 选 N 必须满足 0 < N1+N2=N < M，且普通、特别号码均不重复。": "admin.rule.model.mselectInvalid",
  "0–9 数字模型要求 1–10 位有序数字，普通和特别号码池为空。": "admin.rule.model.digitsInvalid",
};
function uiError(value: string) {
  if (errorEnglish[value]) return t(adminKey(errorEnglish[value]!));
  const poolMatch = value.match(/^(普通号码池|特别号码池)(.*)$/);
  if (poolMatch) {
    const labelKey = poolMatch[1] === "普通号码池" ? "admin.rule.model.poolSizeLabel.regular" : "admin.rule.model.poolSizeLabel.special";
    const label = t(adminKey(labelKey));
    const suffix = poolMatch[2]!;
    const suffixes: Record<string, string> = {
      "格式无效。": "admin.rule.model.poolFormat", "重复设置无效。": "admin.rule.model.poolRepeat",
      "号码列表最多 10,000 个。": "admin.rule.model.poolMaxList", "不能同时设置范围和号码列表。": "admin.rule.model.poolRangeAndList",
      "号码必须唯一且在 0–1,000,000 内。": "admin.rule.model.poolUniqueRange", "号码池大小必须为 1–10,000。": "admin.rule.model.poolSizeError",
      "号码范围必须在 0–1,000,000 内且最小值不大于最大值。": "admin.rule.model.poolRangeError",
    };
    if (suffix.startsWith("包含未知字段：")) return t('admin.rule.model.poolUnknown', { label, field: suffix.slice(7, -1) });
    if (suffix.startsWith("数量必须为 ")) return t('admin.rule.model.poolCount', { label, count: suffix.slice(6, -1) });
    return suffixes[suffix] ? t(adminKey(suffixes[suffix]!), { label }) : display(value);
  }
  const field = value.match(/^(regular_count|special_count|pool_size|total_count|length)必须为 (.+) 的整数。$/);
  if (field) return t('admin.rule.model.fieldInteger', { field: field[1]!, range: field[2]!.replace(" 的整数", "") });
  return display(value);
}
const props = defineProps<{ modelValue: RuleModel; disabled?: boolean }>();
const emit = defineEmits<{
  (e: "update:modelValue", model: RuleModel): void;
  (e: "validity", valid: boolean): void;
}>();
const model = computed(() => props.modelValue);
const drafts = ref<Record<string, string>>({});
const errors = ref<Record<string, string>>({});
const poolModes = ref<Record<string, string>>({
  regular: props.modelValue.regular_pool.values?.length ? "values" : "range",
  special: props.modelValue.special_pool.values?.length ? "values" : "range",
});
const problem = computed(() => {
  try {
    validateEditorModel(model.value);
    return "";
  } catch (cause) {
    return cause instanceof Error ? cause.message : "号码模型无效";
  }
});
watch(
  [problem, errors],
  () => emit("validity", !problem.value && !Object.keys(errors.value).length),
  { deep: true, immediate: true },
);
function report(key: string, message: string) {
  errors.value = { ...errors.value, [key]: message };
}
function clear(key: string) {
  const next = { ...errors.value };
  delete next[key];
  errors.value = next;
}
function update(fn: (m: RuleModel) => void) {
  if (props.disabled) return;
  const next = JSON.parse(JSON.stringify(model.value)) as RuleModel;
  fn(next);
  if (next.model === "M_SELECT_N") {
    next.special_pool = JSON.parse(JSON.stringify(next.regular_pool));
    next.pool_size =
      next.regular_pool.values.length ||
      next.regular_pool.max - next.regular_pool.min + 1;
    next.total_count = next.regular_count + next.special_count;
  }
  emit("update:modelValue", next);
}
function number(
  key: string,
  text: string,
  fn: (m: RuleModel, n: number) => void,
) {
  if (props.disabled) return;
  drafts.value = { ...drafts.value, [key]: text };
  if (
    !/^(0|[1-9]\d*)$/.test(text) ||
    !Number.isSafeInteger(Number(text)) ||
    Number(text) > 1_000_000
  ) {
    report(key, "请输入 0–1000000 的规范整数");
    return;
  }
  clear(key);
  update((m) => fn(m, Number(text)));
}
const value = (key: string, n: number) => drafts.value[key] ?? String(n);
const emptyPool = (): RulePool => ({
  min: 0,
  max: 0,
  values: [],
  allow_repeat: false,
});
function changeType(type: RuleModelName) {
  if (props.disabled) return;
  drafts.value = {};
  errors.value = {};
  poolModes.value = { regular: "range", special: "range" };
  const next: RuleModel = {
    model: type,
    regular_pool: emptyPool(),
    special_pool: emptyPool(),
    regular_count: 0,
    special_count: 0,
    pool_size: 0,
    total_count: 0,
    length: 0,
    allow_repeat: false,
    ordered: false,
  };
  if (type === "DIGITS_0_9") {
    next.length = 3;
    next.allow_repeat = true;
    next.ordered = true;
  } else {
    next.regular_count = 6;
    next.special_count = 1;
    next.regular_pool = { min: 1, max: 49, values: [], allow_repeat: false };
    next.special_pool = { min: 1, max: 49, values: [], allow_repeat: false };
    if (type === "M_SELECT_N") {
      next.pool_size = 49;
      next.total_count = 7;
    }
  }
  emit("update:modelValue", next);
}
function changePoolMode(key: "regular" | "special", mode: string) {
  if (props.disabled) return;
  const p = model.value[`${key}_pool`];
  if (mode === "values" && (p.max - p.min + 1 > 10000 || p.max < p.min)) {
    report(
      `${key}-values`,
      "切换到号码列表前，请将号码范围调整到 10000 个以内",
    );
    return;
  }
  poolModes.value = { ...poolModes.value, [key]: mode };
  delete drafts.value[`${key}-values`];
  clear(`${key}-values`);
  update((m) => {
    m[`${key}_pool`].values =
      mode === "values"
        ? p.values.length
          ? [...p.values]
          : Array.from({ length: p.max - p.min + 1 }, (_, i) => p.min + i)
        : [];
  });
}
function values(key: "regular" | "special", text: string) {
  if (props.disabled) return;
  drafts.value = { ...drafts.value, [`${key}-values`]: text };
  try {
    const numbers = parseEditorNumbers(text, 10000);
    if (
      !numbers.length ||
      new Set(numbers).size !== numbers.length ||
      numbers.some((n) => n > 1_000_000)
    )
      throw new Error("号码列表须非空、不重复，每个号码不超过 1000000");
    clear(`${key}-values`);
    update((m) => {
      m[`${key}_pool`].values = numbers;
    });
  } catch (cause) {
    report(
      `${key}-values`,
      cause instanceof Error ? cause.message : "号码列表无效",
    );
  }
}
const poolKeys = computed(
  () =>
    (model.value.model === "M_SELECT_N"
      ? ["regular"]
      : ["regular", "special"]) as ("regular" | "special")[],
);
</script>
<template>
  <fieldset :disabled="disabled" class="model-editor">
    <legend>{{ t('admin.copy.2a22ea5f') }}</legend>
    <p class="hint">
      {{ t('admin.copy.41965e0a') }}
    </p>
    <p v-if="problem" class="error" role="alert">{{ uiError(problem) }}</p>
    <p v-for="(message, key) in errors" :key="key" class="error" role="alert">
      {{ uiError(message) }}
    </p>
    <label
      >{{ t('admin.copy.570709e7') }}<select
        :value="model.model"
        @change="
          changeType(
            ($event.target as HTMLSelectElement).value as RuleModelName,
          )
        "
      >
        <option value="X_PLUS_Y">{{ t('admin.copy.f8a46915') }}</option>
        <option value="M_SELECT_N">{{ t('admin.copy.7b304eec') }}</option>
        <option value="DIGITS_0_9">{{ t('admin.copy.1a83a92a') }}</option>
      </select></label
    >
    <template v-if="model.model === 'DIGITS_0_9'">
      <label
        >{{ t('admin.copy.37ed5da2') }}<input
          :value="value('length', model.length)"
          inputmode="numeric"
          @input="
            number(
              'length',
              ($event.target as HTMLInputElement).value,
              (m, n) => {
                m.length = n;
              },
            )
          "
      /></label>
      <label class="check"
        ><input
          type="checkbox"
          :checked="model.allow_repeat"
          @change="
            update((m) => {
              m.allow_repeat = ($event.target as HTMLInputElement).checked;
            })
          "
        />{{ t('admin.copy.c7a68335') }}</label
      >
    </template>
    <template v-else>
      <div class="grid">
        <label
          >{{ t('admin.copy.b2c3bf7d') }}<input
            :value="value('regular-count', model.regular_count)"
            inputmode="numeric"
            @input="
              number(
                'regular-count',
                ($event.target as HTMLInputElement).value,
                (m, n) => {
                  m.regular_count = n;
                },
              )
            " /></label
        ><label
          >{{ t('admin.copy.636ec3f0') }}<input
            :value="value('special-count', model.special_count)"
            inputmode="numeric"
            @input="
              number(
                'special-count',
                ($event.target as HTMLInputElement).value,
                (m, n) => {
                  m.special_count = n;
                },
              )
            "
        /></label>
      </div>
      <p v-if="model.model === 'M_SELECT_N'" class="hint">
        {{ t('admin.rule.model.preview', { pool: model.pool_size, total: model.total_count }) }}
      </p>
      <label class="check"
        ><input
          type="checkbox"
          :checked="model.ordered"
          @change="
            update((m) => {
              m.ordered = ($event.target as HTMLInputElement).checked;
            })
          "
        />{{ t('admin.copy.eb203aa8') }}</label
      >
      <fieldset v-for="key in poolKeys" :key="key" class="pool">
        <legend>
          {{
            model.model === "M_SELECT_N"
              ? t('admin.copy.7df5d345')
              : key === "regular"
                ? t('admin.copy.ec53bf7a')
                : t('admin.copy.be1441e1')
          }}
        </legend>
        <label
          >{{ t('admin.copy.8b4c5955') }}<select
            :value="poolModes[key]"
            @change="
              changePoolMode(key, ($event.target as HTMLSelectElement).value)
            "
          >
            <option value="range">{{ t('admin.copy.700a8077') }}</option>
            <option value="values">{{ t('admin.copy.81af00e1') }}</option>
          </select></label
        >
        <div v-if="poolModes[key] === 'range'" class="grid">
          <label
            >{{ t('admin.copy.764194d1') }}<input
              :value="value(`${key}-min`, model[`${key}_pool`].min)"
              inputmode="numeric"
              @input="
                number(
                  `${key}-min`,
                  ($event.target as HTMLInputElement).value,
                  (m, n) => {
                    m[`${key}_pool`].min = n;
                  },
                )
              "
          /></label>
          <label
            >{{ t('admin.copy.70976f43') }}<input
              :value="value(`${key}-max`, model[`${key}_pool`].max)"
              inputmode="numeric"
              @input="
                number(
                  `${key}-max`,
                  ($event.target as HTMLInputElement).value,
                  (m, n) => {
                    m[`${key}_pool`].max = n;
                  },
                )
              "
          /></label>
        </div>
        <label v-else
          >{{ t('admin.copy.05d1f39d') }}<textarea
            :value="
              drafts[`${key}-values`] ?? model[`${key}_pool`].values.join(',')
            "
            rows="3"
            @input="values(key, ($event.target as HTMLTextAreaElement).value)"
          ></textarea>
        </label>
        <label v-if="model.model === 'X_PLUS_Y'" class="check"
          ><input
            type="checkbox"
            :checked="model[`${key}_pool`].allow_repeat"
            @change="
              update((m) => {
                m[`${key}_pool`].allow_repeat = (
                  $event.target as HTMLInputElement
                ).checked;
              })
            "
          />{{ t('admin.copy.bd5d72e7') }}</label
        >
      </fieldset>
    </template>
  </fieldset>
</template>
<style scoped>
.model-editor {
  display: grid;
  gap: 12px;
  min-width: 0;
  border: 1px solid #dfe4ee;
  border-radius: 7px;
  padding: 13px;
}
.grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}
.pool {
  display: grid;
  gap: 12px;
  border: 1px solid #e4e7ee;
  border-radius: 6px;
  padding: 12px;
  min-width: 0;
}
legend {
  font-size: 12px;
  font-weight: 700;
}
label {
  display: grid;
  gap: 5px;
  font-size: 11px;
  color: #505c70;
}
input,
select,
textarea {
  width: 100%;
  min-width: 0;
  border: 1px solid #dce1eb;
  border-radius: 5px;
  padding: 8px;
  font: inherit;
  background: var(--surface);
}
.check {
  display: flex;
  gap: 7px;
  align-items: center;
}
.check input {
  width: auto;
}
.hint {
  font-size: 11px;
  color: #6e7a91;
  line-height: 1.6;
}
.error {
  color: #b34a41;
  font-size: 11px;
  overflow-wrap: anywhere;
}
@media (max-width: 650px) {
  .grid {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
