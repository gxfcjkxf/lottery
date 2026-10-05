<script setup lang="ts">
import { computed, ref, watch } from "vue";
import type { AdminAccount } from "./admin-api";
import { AdminApiError } from "./admin-api";
import {
  buildRuleSimulationRequest,
  canSimulateRules,
  createRuleSimulationApi,
  createRuleSimulationKeyTracker,
  DEFAULT_RULE_SIMULATION_FORM,
  parseIntegerList,
  RULE_TEMPLATE_LABELS,
  SAFE_CONDITION_FIELDS,
  type ConditionOperator,
  type RuleSimulation,
  type RuleSimulationAccount,
  type RuleSimulationForm,
  type RuleTemplate,
  type SafeConditionField,
} from "./rule-simulation-api";

const props = defineProps<{
  account: AdminAccount & Partial<RuleSimulationAccount>;
  brandId: string;
}>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const api = createRuleSimulationApi();
const template = ref<RuleTemplate>("special");
const templates = Object.entries(RULE_TEMPLATE_LABELS) as [
  RuleTemplate,
  string,
][];
const form = ref<RuleSimulationForm>(
  structuredClone(DEFAULT_RULE_SIMULATION_FORM),
);
const localError = ref("");
const requestError = ref("");
const loading = ref(false);
const result = ref<RuleSimulation | null>(null);
const keyFor = createRuleSimulationKeyTracker();
let generation = 0;
const allowed = computed(() => canSimulateRules(props.account, props.brandId));
const featureCount = computed(
  () =>
    Object.values(form.value.featureValues).filter((value) => value.trim())
      .length,
);
const baseLeafCount = computed(() =>
  template.value === "features" ? featureCount.value : 1,
);
const canAddCondition = computed(
  () => baseLeafCount.value + form.value.extraConditions.length < 8,
);

function resetResult() {
  result.value = null;
  localError.value = "";
  requestError.value = "";
}

function addCondition() {
  if (!canAddCondition.value) return;
  form.value.extraConditions.push({
    field: "draw_sum",
    operator: "equals",
    value: "0",
    max: "9",
    target: "all",
    position: "0",
  });
  resetResult();
}

function removeCondition(index: number) {
  form.value.extraConditions.splice(index, 1);
  resetResult();
}

function fieldsForSelect(): SafeConditionField[] {
  return SAFE_CONDITION_FIELDS;
}

function conditionNeedsPosition(field: SafeConditionField): boolean {
  return ["draw_digit", "draw_parity"].includes(field);
}

function conditionHasTarget(field: SafeConditionField): boolean {
  return !["regular_match", "special_match", "position_match"].includes(field);
}

function conditionTargets(field: SafeConditionField) {
  return conditionNeedsPosition(field)
    ? ["digits"]
    : ["all", "regular", "special", "digits"];
}

function conditionFieldChanged(
  condition: RuleSimulationForm["extraConditions"][number],
) {
  if (conditionNeedsPosition(condition.field)) condition.target = "digits";
}

function readIntegerField(
  raw: string,
  label: string,
  min: number,
  max: number,
) {
  const values = parseIntegerList(raw);
  if (!values.length) throw new Error(`${label}不能为空。`);
  if (values.some((value) => value < min || value > max))
    throw new Error(`${label}必须在 ${min} 至 ${max} 范围内。`);
  return values;
}

async function simulate() {
  if (!allowed.value || loading.value) return;
  resetResult();
  const brandId = props.brandId;
  let requestGeneration: number | null = null;
  try {
    if (template.value === "special") {
      readIntegerField(form.value.specialNumbers, "特别号候选", 1, 49);
      readIntegerField(form.value.drawRegular, "开奖普通号", 1, 49);
      readIntegerField(form.value.drawSpecial, "开奖特别号", 1, 49);
    } else if (template.value === "digits" || template.value === "features") {
      const draws = readIntegerField(form.value.drawDigits, "开奖数字", 0, 9);
      if (draws.length !== 3)
        throw new Error("三位数字开奖结果需要正好 3 位。");
      if (template.value === "digits") {
        form.value.digitCandidates.forEach((value, index) => {
          const candidates = readIntegerField(
            value,
            `第 ${index + 1} 位候选`,
            0,
            9,
          );
          if (new Set(candidates).size !== candidates.length)
            throw new Error("同一位置候选数字不能重复。");
        });
      }
    } else if (template.value === "exclude") {
      readIntegerField(form.value.excludedNumbers, "排除号码", 1, 49);
      readIntegerField(form.value.drawRegular, "开奖普通号", 1, 49);
      readIntegerField(form.value.drawSpecial, "开奖特别号", 1, 49);
    } else if (template.value === "attributes") {
      readIntegerField(form.value.redNumbers, "红色号码映射", 1, 49);
      readIntegerField(form.value.blueNumbers, "蓝色号码映射", 1, 49);
      readIntegerField(form.value.drawRegular, "开奖普通号", 1, 49);
      readIntegerField(form.value.drawSpecial, "开奖特别号", 1, 49);
      if (!form.value.selectedColors.length)
        throw new Error("请至少选择一种号码属性。");
    } else {
      const regular = readIntegerField(
        form.value.regularNumbers,
        "普通号码",
        1,
        49,
      );
      const special = readIntegerField(
        form.value.specialNumbers,
        "特别号码",
        1,
        49,
      );
      const drawRegular = readIntegerField(
        form.value.drawRegular,
        "开奖普通号",
        1,
        49,
      );
      const drawSpecial = readIntegerField(
        form.value.drawSpecial,
        "开奖特别号",
        1,
        49,
      );
      if (
        regular.length !== 6 ||
        special.length !== 1 ||
        drawRegular.length !== 6 ||
        drawSpecial.length !== 1
      )
        throw new Error(
          "M 选 N 需要六个普通号和一个特别号，开奖结果也须符合此数量。",
        );
    }
    const body = buildRuleSimulationRequest(template.value, form.value);
    requestGeneration = ++generation;
    loading.value = true;
    const output = await api.simulate(
      brandId,
      body,
      keyFor({ brand_id: brandId, ...body }),
    );
    if (requestGeneration !== generation || props.brandId !== brandId) return;
    result.value = output;
  } catch (cause) {
    if (
      requestGeneration !== null &&
      (requestGeneration !== generation || props.brandId !== brandId)
    )
      return;
    if (cause instanceof AdminApiError) {
      if (cause.status === 401) emit("session-invalid");
      requestError.value =
        cause.status === 409
          ? `${cause.message}；规则或幂等状态已变化，请调整输入后重试。`
          : cause.message;
    } else {
      localError.value =
        cause instanceof Error ? cause.message : "模拟请求无效。";
    }
  } finally {
    if (
      requestGeneration === null ||
      (requestGeneration === generation && props.brandId === brandId)
    )
      loading.value = false;
  }
}

function prettyTrace(value: unknown) {
  return JSON.stringify(value, null, 2);
}

watch(
  () => props.brandId,
  () => {
    generation++;
    loading.value = false;
    resetResult();
  },
);
watch(
  [template, form],
  () => {
    generation++;
    loading.value = false;
    resetResult();
  },
  { deep: true },
);
</script>

<template>
  <section class="rule-simulator">
    <header class="simulator-head">
      <div>
        <p class="eyebrow">REAL RULE SIMULATION</p>
        <h2>规则模拟器</h2>
        <p>通过受限模板构建规则，不接受脚本或自定义 JSON。</p>
      </div>
      <span class="brand-tag">品牌 · {{ brandId || "未选择" }}</span>
    </header>
    <div class="safety-banner">
      仅调用真实规则模拟接口：不会扣款、投注、发布或审核规则。模拟结果不代表规则已生效；旧审核演示仍是独立演示内容。
    </div>
    <p v-if="!allowed" class="notice error" role="alert">
      当前账号没有 rule.simulate.brand 或 rule.simulate.platform
      权限。超级管理员也必须有显式的平台模拟权限。
    </p>
    <template v-else>
      <form class="simulator-form" @submit.prevent="simulate">
        <label class="wide"
          >玩法模板
          <select v-model="template">
            <option
              v-for="[value, label] in templates"
              :key="value"
              :value="value"
            >
              {{ label }}
            </option>
          </select>
        </label>

        <template v-if="template === 'special'">
          <label
            >特别号候选（最多 49）<input
              v-model.trim="form.specialNumbers"
              inputmode="numeric"
              placeholder="7,19,31,43"
          /></label>
          <label
            >开奖普通号（6 个）<input
              v-model.trim="form.drawRegular"
              inputmode="numeric"
              placeholder="1,2,3,4,5,6"
          /></label>
          <label class="wide"
            >开奖特别号<input
              v-model.trim="form.drawSpecial"
              inputmode="numeric"
              placeholder="7"
          /></label>
        </template>

        <template v-else-if="template === 'digits'">
          <label v-for="(candidate, index) in form.digitCandidates" :key="index"
            >第 {{ index + 1 }} 位候选（0–9）
            <input
              v-model.trim="form.digitCandidates[index]"
              inputmode="numeric"
              placeholder="3,1,0"
            />
          </label>
          <label class="wide"
            >三位开奖结果<input
              v-model.trim="form.drawDigits"
              inputmode="numeric"
              placeholder="3,1,0"
          /></label>
        </template>

        <template v-else-if="template === 'features'">
          <label
            >三同号（0 否 / 1 是）<input
              v-model.trim="form.featureValues.three_kind"
              inputmode="numeric"
              placeholder="1"
          /></label>
          <label
            >首尾同号（0 否 / 1 是）<input
              v-model.trim="form.featureValues.same_ends"
              inputmode="numeric"
              placeholder="1"
          /></label>
          <label
            >奇数个数（0–3）<input
              v-model.trim="form.featureValues.odd_count"
              inputmode="numeric"
              placeholder="3"
          /></label>
          <label
            >和值（0–27）<input
              v-model.trim="form.featureValues.sum"
              inputmode="numeric"
              placeholder="4"
          /></label>
          <label class="wide"
            >三位开奖结果<input
              v-model.trim="form.drawDigits"
              inputmode="numeric"
              placeholder="3,1,0"
          /></label>
          <p class="wide hint">
            每项输入逗号分隔的可选值；奖级将通过 selected 条件与所选特征值比对。
          </p>
        </template>

        <template v-else-if="template === 'exclude'">
          <label
            >排除号码（49 选）<input
              v-model.trim="form.excludedNumbers"
              inputmode="numeric"
              placeholder="11,22,33"
          /></label>
          <label
            >开奖普通号（6 个）<input
              v-model.trim="form.drawRegular"
              inputmode="numeric"
              placeholder="1,2,3,4,5,6"
          /></label>
          <label
            >开奖特别号<input
              v-model.trim="form.drawSpecial"
              inputmode="numeric"
              placeholder="7"
          /></label>
        </template>

        <template v-else-if="template === 'attributes'">
          <label
            >红色号码映射<input
              v-model.trim="form.redNumbers"
              inputmode="numeric"
              placeholder="1,3,5,7,9"
          /></label>
          <label
            >蓝色号码映射<input
              v-model.trim="form.blueNumbers"
              inputmode="numeric"
              placeholder="2,4,6,8"
          /></label>
          <fieldset class="wide">
            <legend>下注属性（可多选）</legend>
            <label class="check"
              ><input
                v-model="form.selectedColors"
                type="checkbox"
                value="red"
              />红</label
            >
            <label class="check"
              ><input
                v-model="form.selectedColors"
                type="checkbox"
                value="blue"
              />蓝</label
            >
          </fieldset>
          <label
            >开奖普通号（6 个）<input
              v-model.trim="form.drawRegular"
              inputmode="numeric"
              placeholder="1,2,3,4,5,6"
          /></label>
          <label
            >开奖特别号<input
              v-model.trim="form.drawSpecial"
              inputmode="numeric"
              placeholder="7"
          /></label>
        </template>

        <template v-else>
          <label
            >所选普通号（6 个）<input
              v-model.trim="form.regularNumbers"
              inputmode="numeric"
              placeholder="1,2,3,4,5,6"
          /></label>
          <label
            >所选特别号<input
              v-model.trim="form.specialNumbers"
              inputmode="numeric"
              placeholder="7"
          /></label>
          <label
            >开奖普通号（6 个）<input
              v-model.trim="form.drawRegular"
              inputmode="numeric"
              placeholder="1,2,3,4,5,6"
          /></label>
          <label
            >开奖特别号<input
              v-model.trim="form.drawSpecial"
              inputmode="numeric"
              placeholder="7"
          /></label>
        </template>

        <label
          >单位积分（正整数）<input
            v-model.trim="form.unitPoints"
            inputmode="numeric"
        /></label>
        <label
          >赔率（最多 6 位小数）<input
            v-model.trim="form.odds"
            inputmode="decimal"
        /></label>
        <label
          >模拟倍数（最多 1000）<input
            v-model.trim="form.multiplier"
            inputmode="decimal"
        /></label>
        <label
          >全局积分封顶（留空不限）<input
            v-model.trim="form.capPoints"
            inputmode="numeric"
            placeholder="不限"
        /></label>
        <label class="wide"
          >投注积分限额（留空不限）<input
            v-model.trim="form.maxBetPoints"
            inputmode="numeric"
            placeholder="不限"
        /></label>
        <label class="wide"
          >舍入范围<select v-model="form.roundingScope">
            <option value="order">整注汇总后舍入（order）</option>
            <option value="line">每组合行舍入后汇总（line）</option>
            <option value="tier">每个奖级舍入后汇总（tier）</option></select
          ><small>舍入方式固定为 half_up；范围会写入模拟请求。</small></label
        >

        <details class="wide condition-editor">
          <summary>条件组合（白名单数值条件，最多 8 个）</summary>
          <label
            >组合关系<select v-model="form.conditionJoin">
              <option value="all">全部满足（AND）</option>
              <option value="any">任一满足（OR）</option>
            </select></label
          >
          <div
            v-for="(condition, index) in form.extraConditions"
            :key="index"
            class="condition-row"
          >
            <label
              >字段<select
                v-model="condition.field"
                @change="conditionFieldChanged(condition)"
              >
                <option
                  v-for="field in fieldsForSelect()"
                  :key="field"
                  :value="field"
                >
                  {{ field }}
                </option>
              </select></label
            >
            <label v-if="conditionHasTarget(condition.field)"
              >目标<select v-model="condition.target">
                <option
                  v-for="target in conditionTargets(condition.field)"
                  :key="target"
                  :value="target"
                >
                  {{
                    target === "all"
                      ? "全部"
                      : target === "regular"
                        ? "普通号"
                        : target === "special"
                          ? "特别号"
                          : "数字"
                  }}
                </option>
              </select></label
            >
            <label v-if="conditionNeedsPosition(condition.field)"
              >位置（0 起）<input
                v-model.trim="condition.position"
                inputmode="numeric"
            /></label>
            <label
              >比较<select v-model="condition.operator">
                <option value="equals">等于</option>
                <option value="in">属于列表</option>
                <option value="between">范围内</option>
              </select></label
            >
            <label
              >{{
                condition.operator === "between"
                  ? "最小值"
                  : "数值（逗号分隔）"
              }}<input v-model.trim="condition.value" inputmode="numeric"
            /></label>
            <label v-if="condition.operator === 'between'"
              >最大值<input v-model.trim="condition.max" inputmode="numeric"
            /></label>
            <button
              type="button"
              class="remove"
              @click="removeCondition(index)"
            >
              移除
            </button>
          </div>
          <button
            type="button"
            class="secondary"
            :disabled="!canAddCondition"
            @click="addCondition"
          >
            添加条件
          </button>
        </details>
        <p v-if="localError" class="notice error wide" role="alert">
          {{ localError }}
        </p>
        <p v-if="requestError" class="notice error wide" role="alert">
          {{ requestError }}
        </p>
        <button class="primary wide submit" type="submit" :disabled="loading">
          {{ loading ? "模拟中…" : "运行真实规则模拟" }}
        </button>
      </form>

      <section v-if="result" class="result-card" aria-live="polite">
        <header class="result-head">
          <div>
            <p class="eyebrow">SERVER RESULT</p>
            <h3>{{ result.won ? "模拟命中" : "未命中" }}</h3>
          </div>
          <span class="status" :class="result.won ? 'won' : 'miss'">{{
            result.won ? "中奖" : "未中奖"
          }}</span>
        </header>
        <div class="summary-grid">
          <div>
            <small>组合数</small><strong>{{ result.combination_count }}</strong>
          </div>
          <div>
            <small>倍数</small><strong>{{ result.multiplier }}</strong>
          </div>
          <div>
            <small>投注积分</small><strong>{{ result.bet_points }}</strong>
          </div>
          <div>
            <small>中奖积分</small><strong>{{ result.prize_points }}</strong>
          </div>
        </div>
        <p v-if="form.roundingScope === 'order'" class="rounding-note">
          当前为 order 舍入：整注汇总后再舍入。逐行 points
          是各组合独立舍入的展示值，不能直接相加作为总奖金；raw_points 展示精确
          n/d。
        </p>
        <ul v-if="result.warnings.length" class="warnings">
          <li v-for="(warning, index) in result.warnings" :key="index">
            {{ warning }}
          </li>
        </ul>
        <article
          v-for="(line, lineIndex) in result.lines"
          :key="lineIndex"
          class="line-result"
        >
          <h4>注单行 {{ lineIndex + 1 }} · {{ line.points }} 积分</h4>
          <div
            v-for="hit in line.hits"
            :key="hit.code"
            class="tier-hit"
            :class="{ matched: hit.selected }"
          >
            <div class="tier-head">
              <b>{{ hit.code }}</b
              ><span>{{
                hit.matched
                  ? hit.selected
                    ? "命中并计奖"
                    : "命中但未选中"
                  : "未命中"
              }}</span>
            </div>
            <p>
              精确原始积分 {{ hit.raw_points }} · 该奖级独立舍入值
              {{ hit.points }} ·
              {{ hit.exclusive ? "排他奖级" : "可累加奖级" }}
            </p>
            <details>
              <summary>条件解释 trace</summary>
              <pre>{{ prettyTrace(hit.trace) }}</pre>
            </details>
          </div>
        </article>
        <details class="normalized">
          <summary>服务端规范化选号</summary>
          <pre>{{ prettyTrace(result.normalized) }}</pre>
        </details>
      </section>
    </template>
  </section>
</template>

<style scoped>
.rule-simulator {
  width: 100%;
  min-width: 0;
  color: #252a36;
}
.simulator-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 14px;
  margin: 4px 0 14px;
}
.simulator-head h2 {
  font-size: 19px;
  margin: 3px 0 5px;
}
.simulator-head p {
  margin: 0;
  color: #737b8c;
  font-size: 11px;
  line-height: 1.55;
}
.eyebrow {
  font-size: 10px !important;
  letter-spacing: 1.2px;
  color: #8991a2 !important;
  font-weight: 700;
}
.brand-tag,
.status {
  border-radius: 20px;
  background: #f1f3f8;
  padding: 7px 11px;
  color: #626b7c;
  font-size: 11px;
  white-space: nowrap;
}
.safety-banner,
.notice {
  border-radius: 8px;
  padding: 10px 12px;
  font-size: 11px;
  line-height: 1.6;
  overflow-wrap: anywhere;
  margin: 12px 0;
}
.safety-banner {
  background: #fff6e7;
  color: #825b1a;
  border: 1px solid #f3e4c5;
}
.error {
  background: #fff0ef;
  color: #a83d36;
}
.simulator-form,
.result-card {
  background: #fff;
  border: 1px solid #e9ebf0;
  border-radius: 11px;
  padding: 16px;
  min-width: 0;
  box-shadow: 0 2px 8px #1e2a5008;
  margin: 13px 0;
}
.simulator-form {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}
.simulator-form > label,
.condition-editor > label {
  display: grid;
  gap: 6px;
  font-size: 11px;
  font-weight: 600;
  min-width: 0;
}
.wide {
  grid-column: 1/-1;
}
.simulator-form input,
.simulator-form select,
.condition-editor input,
.condition-editor select {
  box-sizing: border-box;
  width: 100%;
  min-width: 0;
  border: 1px solid #dfe3eb;
  border-radius: 7px;
  background: #fff;
  padding: 9px 10px;
  color: #252a36;
  font: inherit;
}
.simulator-form fieldset {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  border: 1px solid #dfe3eb;
  border-radius: 7px;
  min-width: 0;
}
.simulator-form legend {
  font-size: 11px;
  font-weight: 600;
}
.simulator-form .check {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11px;
}
.check input {
  width: auto;
}
.hint {
  margin: 0;
  color: #737b8c;
  font-size: 10px;
  line-height: 1.5;
}
.condition-editor {
  border-top: 1px solid #eff0f4;
  padding-top: 10px;
}
.condition-editor summary,
.tier-hit summary,
.normalized summary {
  cursor: pointer;
  font-size: 11px;
  font-weight: 700;
}
.condition-editor > label {
  margin: 10px 0;
}
.condition-row {
  display: grid;
  grid-template-columns: 1.1fr 1fr 1fr 1fr auto;
  gap: 7px;
  align-items: end;
  margin: 9px 0;
}
.condition-row label {
  display: grid;
  gap: 5px;
  font-size: 10px;
  min-width: 0;
}
.primary,
.secondary,
.remove {
  border-radius: 7px;
  padding: 9px 12px;
  font-size: 11px;
  font-weight: 700;
}
.primary {
  border: 0;
  background: #5969df;
  color: #fff;
}
.secondary,
.remove {
  border: 1px solid #e0e3ea;
  background: #fff;
  color: #586174;
}
.remove {
  padding: 8px;
}
.primary:disabled,
.secondary:disabled {
  opacity: 0.48;
  cursor: not-allowed;
}
.submit {
  justify-self: start;
}
.result-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
}
.result-head h3 {
  margin: 3px 0;
  font-size: 17px;
}
.status.won {
  background: #e9f6f0;
  color: #258763;
}
.status.miss {
  background: #f1f3f8;
}
.summary-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 9px;
  margin: 14px 0;
}
.summary-grid > div {
  display: grid;
  gap: 5px;
  padding: 10px;
  border: 1px solid #eceef3;
  border-radius: 8px;
  min-width: 0;
}
.summary-grid small {
  font-size: 10px;
  color: #737b8c;
}
.summary-grid strong {
  font-size: 16px;
  overflow-wrap: anywhere;
  font-variant-numeric: tabular-nums;
}
.rounding-note {
  margin: 8px 0;
  border-radius: 7px;
  padding: 10px;
  background: #eef4ff;
  color: #40537b;
  font-size: 11px;
  line-height: 1.6;
  overflow-wrap: anywhere;
}
.warnings {
  background: #fff6e7;
  color: #825b1a;
  border-radius: 7px;
  padding: 10px 10px 10px 28px;
  font-size: 11px;
  line-height: 1.6;
}
.line-result {
  border-top: 1px solid #eff0f4;
  padding: 12px 0;
}
.line-result h4 {
  margin: 0 0 9px;
  font-size: 12px;
}
.tier-hit {
  border: 1px solid #eceef3;
  border-radius: 8px;
  padding: 10px;
  margin: 7px 0;
  min-width: 0;
}
.tier-hit.matched {
  border-color: #99d4bd;
  background: #f5fbf8;
}
.tier-head {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  font-size: 11px;
}
.tier-hit p {
  font-size: 10px;
  color: #737b8c;
  overflow-wrap: anywhere;
}
.tier-hit pre,
.normalized pre {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  max-width: 100%;
  overflow-x: auto;
  background: #f7f8fa;
  padding: 9px;
  border-radius: 6px;
  font-size: 10px;
}
.normalized {
  border-top: 1px solid #eff0f4;
  padding-top: 12px;
}
.normalized summary {
  margin-bottom: 8px;
}
@media (max-width: 650px) {
  .simulator-head {
    flex-direction: column;
  }
  .brand-tag {
    white-space: normal;
  }
  .simulator-form,
  .result-card {
    padding: 13px;
  }
  .simulator-form {
    grid-template-columns: minmax(0, 1fr);
  }
  .wide {
    grid-column: auto;
  }
  .condition-row {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .condition-row .remove {
    justify-self: start;
  }
  .summary-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
