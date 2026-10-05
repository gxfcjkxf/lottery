<script setup lang="ts">
import { computed, ref, watch } from "vue";
import type { RuleModel, RuleModelName, RulePool } from "./rule-simulation-api";
import { parseEditorNumbers, validateEditorModel } from "./rule-editor";
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
    <legend>自定义彩种号码模型</legend>
    <p class="hint">
      彩种创建后模型不可改写。M 是号码池大小，N1 为普通号、N2 为特别号；数字型 N
      是位数。
    </p>
    <p v-if="problem" class="error" role="alert">{{ problem }}</p>
    <p v-for="(message, key) in errors" :key="key" class="error" role="alert">
      {{ message }}
    </p>
    <label
      >号码模型<select
        :value="model.model"
        @change="
          changeType(
            ($event.target as HTMLSelectElement).value as RuleModelName,
          )
        "
      >
        <option value="X_PLUS_Y">普通号 X + 特别号 Y（独立号码池）</option>
        <option value="M_SELECT_N">M 选 N1 + N2（共享池，不重复）</option>
        <option value="DIGITS_0_9">0–9 数字，N 个有序位置</option>
      </select></label
    >
    <template v-if="model.model === 'DIGITS_0_9'">
      <label
        >数字位数 N（1–10）<input
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
        />允许重复数字，例如 121、111</label
      >
    </template>
    <template v-else>
      <div class="grid">
        <label
          >普通开奖数量 X / N1（0–10）<input
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
          >特别开奖数量 Y / N2（0–10）<input
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
        号码池 M = {{ model.pool_size }}，总开奖 N =
        {{ model.total_count }}；必须 0 &lt; N &lt;
        M，普通号与特别号之间也不重复。
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
        />普通 / 特别号码各自有序</label
      >
      <fieldset v-for="key in poolKeys" :key="key" class="pool">
        <legend>
          {{
            model.model === "M_SELECT_N"
              ? "共享号码池"
              : key === "regular"
                ? "普通号码池"
                : "特别号码池"
          }}
        </legend>
        <label
          >号码池定义方式<select
            :value="poolModes[key]"
            @change="
              changePoolMode(key, ($event.target as HTMLSelectElement).value)
            "
          >
            <option value="range">连续范围</option>
            <option value="values">明确号码列表</option>
          </select></label
        >
        <div v-if="poolModes[key] === 'range'" class="grid">
          <label
            >最小号码<input
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
            >最大号码<input
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
          >允许的号码（逗号分隔）<textarea
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
          />该号码池允许重复号码</label
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
  background: #fff;
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
