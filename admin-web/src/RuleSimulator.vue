<script setup lang="ts">
import BusinessDetails from '@lottery/shared/BusinessDetails.vue'
import { AssociationName } from "@lottery/shared";
import { useAdminI18n as useAssociationI18n } from "./i18n";
const associationLocale = useAssociationI18n().locale;
import { useAdminI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import { vInlineValidation } from '@lottery/shared'
import { computed, ref, watch } from "vue";
import type { AdminAccount } from "./admin-api";
import { AdminApiError } from "./admin-api";
import { adminKey, useAdminI18n } from "./i18n";
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
const { t, display } = useAdminI18n();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const api = createRuleSimulationApi();
const template = ref<RuleTemplate>("special");
const templates = Object.entries(RULE_TEMPLATE_LABELS) as [
  RuleTemplate,
  string,
][];
const templateLabels: Record<string, string> = {
  "特别号命中": "admin.rule.template.special", "数字直选（三位）": "admin.rule.template.digits",
  "数字特征": "admin.rule.template.features", "排除号码": "admin.rule.template.exclude",
  "号码属性（特别号）": "admin.rule.template.attributes", "M 选 N 全中": "admin.rule.template.mselect",
};
const translatedTemplate = (value: string) => templateLabels[value] ? t(adminKey(templateLabels[value]!)) : display(value);
const translatedField = (value: SafeConditionField) => {
  const labels: Record<string, string> = {
    regular_match: 'admin.copy.2e8458c7', special_match: 'admin.copy.144bb84a',
    position_match: 'admin.copy.702fa755', excluded_match: 'admin.copy.fe3bf2dd',
    draw_sum: 'admin.copy.b169b132', draw_odd_count: 'admin.copy.d13641c1', draw_even_count: 'admin.copy.64aef7ff',
    draw_unique_count: 'admin.copy.bd8be16d', draw_all_same: 'admin.copy.d0aaa2ed',
    draw_first_last_same: 'admin.copy.3724f7e5', draw_span: 'admin.copy.d5367324',
    draw_consecutive: 'admin.copy.fee17cf2', draw_digit: 'admin.copy.5afccd6b',
    draw_parity: 'admin.copy.e17e2fc4', attribute_match: 'admin.copy.0574fa30',
  };
  return `${labels[value] ? t(adminKey(labels[value]!)) : display(value)} (${value})`;
};
const localErrorText = (value: string | import("@lottery/shared").LocalizedMessage) => {
  if (typeof value !== "string") return displayPublicError(value);
  const labels: Record<string, string> = {
    "特别号候选": "Special-number candidates", "开奖普通号": "Regular draw numbers", "开奖特别号": "Special draw number",
    "开奖数字": "Draw digits", "排除号码": "Excluded numbers", "红色号码映射": "Red-number mapping",
    "蓝色号码映射": "Blue-number mapping", "普通号码": "Regular numbers", "特别号码": "Special numbers",
  };
  const match = value.match(/^(.+?)(不能为空。|必须在 (\d+) 至 (\d+) 范围内。)$/);
  if (match) {
    const label = labels[match[1]!] ?? match[1]!;
    return match[2] === "不能为空。" ? display(`${match[1]}不能为空。`) : display(`${match[1]}必须在 ${match[3]} 至 ${match[4]} 范围内。`);
  }
  const pairs: Record<string, string> = {
    "三位数字开奖结果需要正好 3 位。": "admin.rule.case.digitCount", "同一位置候选数字不能重复。": "admin.rule.simulation.uniqueDigits",
    "请至少选择一种号码属性。": "admin.rule.simulation.attributes", "M 选 N 需要六个普通号和一个特别号，开奖结果也须符合此数量。": "admin.rule.simulation.mselect",
    "模拟请求无效。": "admin.rule.simulation.invalid",
  };
  return pairs[value] ? t(adminKey(pairs[value]!), { count: "3" }) : display(value);
};
const form = ref<RuleSimulationForm>(
  structuredClone(DEFAULT_RULE_SIMULATION_FORM),
);
const localError = ref<string | import("@lottery/shared").LocalizedMessage>("");
const requestError = ref<string | import("@lottery/shared").LocalizedMessage>("");
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
      requestError.value = publicErrorMessage(cause);
    } else {
      localError.value =
        cause instanceof Error ? publicErrorMessage(cause) : "模拟请求无效。";
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
        <h2>{{ t('admin.copy.19714e82') }}</h2>
      </div>
      <span class="brand-tag">{{ t('admin.copy.55bdd7a5') }}<AssociationName kind="brand" :id="brandId" :locale="associationLocale" /></span>
    </header>
    <div class="safety-banner">
      {{ t('admin.copy.1473f8c6') }}
    </div>
    <p v-if="!allowed" class="notice error" role="alert">
      {{ t('admin.copy.bb7c6849') }}
    </p>
    <template v-else>
      <form novalidate v-inline-validation class="simulator-form" @submit.prevent="withGlobalLoading(simulate)">
        <label class="wide"
          >{{ t('admin.copy.158affdb') }}
          <select v-model="template">
            <option
              v-for="[value, label] in templates"
              :key="value"
              :value="value"
            >
              {{ translatedTemplate(label) }}
            </option>
          </select>
        </label>

        <template v-if="template === 'special'">
          <label
            >{{ t('admin.copy.b420eed9') }}<input
              v-model.trim="form.specialNumbers"
              inputmode="numeric"
              placeholder="7,19,31,43"
          /></label>
          <label
            >{{ t('admin.copy.fcfeff00') }}<input
              v-model.trim="form.drawRegular"
              inputmode="numeric"
              placeholder="1,2,3,4,5,6"
          /></label>
          <label class="wide"
            >{{ t('admin.copy.55b44fc8') }}<input
              v-model.trim="form.drawSpecial"
              inputmode="numeric"
              placeholder="7"
          /></label>
        </template>

        <template v-else-if="template === 'digits'">
          <label v-for="(candidate, index) in form.digitCandidates" :key="index"
            >{{ t('admin.rule.case.position', { index: index + 1 }) }}
            <input
              v-model.trim="form.digitCandidates[index]"
              inputmode="numeric"
              placeholder="3,1,0"
            />
          </label>
          <label class="wide"
            >{{ t('admin.copy.89e6faa8') }}<input
              v-model.trim="form.drawDigits"
              inputmode="numeric"
              placeholder="3,1,0"
          /></label>
        </template>

        <template v-else-if="template === 'features'">
          <label
            >{{ t('admin.copy.255c1994') }}<input
              v-model.trim="form.featureValues.three_kind"
              inputmode="numeric"
              placeholder="1"
          /></label>
          <label
            >{{ t('admin.copy.ef7b22cd') }}<input
              v-model.trim="form.featureValues.same_ends"
              inputmode="numeric"
              placeholder="1"
          /></label>
          <label
            >{{ t('admin.copy.d28a71e7') }}<input
              v-model.trim="form.featureValues.odd_count"
              inputmode="numeric"
              placeholder="3"
          /></label>
          <label
            >{{ t('admin.copy.a9eec73c') }}<input
              v-model.trim="form.featureValues.sum"
              inputmode="numeric"
              placeholder="4"
          /></label>
          <label class="wide"
            >{{ t('admin.copy.89e6faa8') }}<input
              v-model.trim="form.drawDigits"
              inputmode="numeric"
              placeholder="3,1,0"
          /></label>
          <p class="wide hint">
            {{ t('admin.copy.7faa215e') }}
          </p>
        </template>

        <template v-else-if="template === 'exclude'">
          <label
            >{{ t('admin.copy.c9d4507d') }}<input
              v-model.trim="form.excludedNumbers"
              inputmode="numeric"
              placeholder="11,22,33"
          /></label>
          <label
            >{{ t('admin.copy.fcfeff00') }}<input
              v-model.trim="form.drawRegular"
              inputmode="numeric"
              placeholder="1,2,3,4,5,6"
          /></label>
          <label
            >{{ t('admin.copy.55b44fc8') }}<input
              v-model.trim="form.drawSpecial"
              inputmode="numeric"
              placeholder="7"
          /></label>
        </template>

        <template v-else-if="template === 'attributes'">
          <label
            >{{ t('admin.copy.f94469a6') }}<input
              v-model.trim="form.redNumbers"
              inputmode="numeric"
              placeholder="1,3,5,7,9"
          /></label>
          <label
            >{{ t('admin.copy.5ef620d2') }}<input
              v-model.trim="form.blueNumbers"
              inputmode="numeric"
              placeholder="2,4,6,8"
          /></label>
          <fieldset class="wide">
            <legend>{{ t('admin.copy.ee9db472') }}</legend>
            <label class="check"
              ><input
                v-model="form.selectedColors"
                type="checkbox"
                value="red"
              />{{ t('admin.copy.4a98e5d6') }}</label
            >
            <label class="check"
              ><input
                v-model="form.selectedColors"
                type="checkbox"
                value="blue"
              />{{ t('admin.copy.82e76224') }}</label
            >
          </fieldset>
          <label
            >{{ t('admin.copy.fcfeff00') }}<input
              v-model.trim="form.drawRegular"
              inputmode="numeric"
              placeholder="1,2,3,4,5,6"
          /></label>
          <label
            >{{ t('admin.copy.55b44fc8') }}<input
              v-model.trim="form.drawSpecial"
              inputmode="numeric"
              placeholder="7"
          /></label>
        </template>

        <template v-else>
          <label
            >{{ t('admin.copy.cb08d518') }}<input
              v-model.trim="form.regularNumbers"
              inputmode="numeric"
              placeholder="1,2,3,4,5,6"
          /></label>
          <label
            >{{ t('admin.copy.26fdc04c') }}<input
              v-model.trim="form.specialNumbers"
              inputmode="numeric"
              placeholder="7"
          /></label>
          <label
            >{{ t('admin.copy.fcfeff00') }}<input
              v-model.trim="form.drawRegular"
              inputmode="numeric"
              placeholder="1,2,3,4,5,6"
          /></label>
          <label
            >{{ t('admin.copy.55b44fc8') }}<input
              v-model.trim="form.drawSpecial"
              inputmode="numeric"
              placeholder="7"
          /></label>
        </template>

        <label
          >{{ t('admin.copy.fcaa2d0b') }}<input
            v-model.trim="form.unitPoints"
            inputmode="numeric"
        /></label>
        <label
          >{{ t('admin.copy.72812ead') }}<input
            v-model.trim="form.odds"
            inputmode="decimal"
        /></label>
        <label
          >{{ t('admin.copy.0e757f30') }}<input
            v-model.trim="form.multiplier"
            inputmode="decimal"
        /></label>
        <label
          >{{ t('admin.copy.4f64097d') }}<input
            v-model.trim="form.capPoints"
            inputmode="numeric"
            :placeholder="t('admin.copy.374fa8f7')"
        /></label>
        <label class="wide"
          >{{ t('admin.copy.0d72b609') }}<input
            v-model.trim="form.maxBetPoints"
            inputmode="numeric"
            :placeholder="t('admin.copy.374fa8f7')"
        /></label>
        <label class="wide"
          >{{ t('admin.copy.87ba003c') }}<select v-model="form.roundingScope">
            <option value="order">{{ t('admin.copy.ac1adbc4') }}</option>
            <option value="line">{{ t('admin.copy.affabdee') }}</option>
            <option value="tier">{{ t('admin.copy.c3176d1e') }}</option></select
          ><small>{{ t('admin.copy.d608ffbf') }}</small></label
        >

        <details class="wide condition-editor">
          <summary>{{ t('admin.copy.98d1fae7') }}</summary>
          <label
            >{{ t('admin.copy.e8435add') }}<select v-model="form.conditionJoin">
              <option value="all">{{ t('admin.copy.7674da94') }}</option>
              <option value="any">{{ t('admin.copy.ec366404') }}</option>
            </select></label
          >
          <div
            v-for="(condition, index) in form.extraConditions"
            :key="index"
            class="condition-row"
          >
            <label
              >{{ t('admin.copy.4a0386a5') }}<select
                v-model="condition.field"
                @change="conditionFieldChanged(condition)"
              >
                <option
                  v-for="field in fieldsForSelect()"
                  :key="field"
                  :value="field"
                >
                  {{ translatedField(field) }}
                </option>
              </select></label
            >
            <label v-if="conditionHasTarget(condition.field)"
              >{{ t('admin.copy.559998d7') }}<select v-model="condition.target">
                <option
                  v-for="target in conditionTargets(condition.field)"
                  :key="target"
                  :value="target"
                >
                  {{
                    target === "all"
                      ? t('admin.copy.ef22c1d2')
                      : target === "regular"
                        ? t('admin.copy.14f26052')
                        : target === "special"
                          ? t('admin.copy.308f60c1')
                          : t('admin.copy.0e8ca53a')
                  }}
                </option>
              </select></label
            >
            <label v-if="conditionNeedsPosition(condition.field)"
              >{{ t('admin.copy.a3c9898e') }}<input
                v-model.trim="condition.position"
                inputmode="numeric"
            /></label>
            <label
              >{{ t('admin.copy.b792e490') }}<select v-model="condition.operator">
                <option value="equals">{{ t('admin.copy.1e199c87') }}</option>
                <option value="in">{{ t('admin.copy.48da3113') }}</option>
                <option value="between">{{ t('admin.copy.dff3c801') }}</option>
              </select></label
            >
            <label
              >{{
                condition.operator === "between"
                  ? t('admin.copy.b55f1798')
                  : t('admin.copy.950abe20')
              }}<input v-model.trim="condition.value" inputmode="numeric"
            /></label>
            <label v-if="condition.operator === 'between'"
              >{{ t('admin.copy.bcb03f9a') }}<input v-model.trim="condition.max" inputmode="numeric"
            /></label>
            <button
              type="button"
              class="remove"
              @click="removeCondition(index)"
            >
              {{ t('admin.copy.cd87c66a') }}
            </button>
          </div>
          <button
            type="button"
            class="button button-secondary"
            :disabled="!canAddCondition"
            @click="addCondition"
          >
            {{ t('admin.copy.9aa54df7') }}
          </button>
        </details>
        <p v-if="localError" class="notice error wide" role="alert">
          {{ localErrorText(localError) }}
        </p>
        <p v-if="requestError" class="notice error wide" role="alert">
          {{ displayPublicError(requestError) }}
        </p>
        <button class="button button-primary wide submit" type="submit" :disabled="loading">
          {{ loading ? t('admin.copy.f0767da7') : t('admin.copy.650ec5cf') }}
        </button>
      </form>

      <section v-if="result" class="result-card" aria-live="polite">
        <header class="result-head">
          <div>
            <h3>{{ result.won ? t('admin.copy.7a22ae48') : t('admin.copy.158b5ef8') }}</h3>
          </div>
          <span class="status" :class="result.won ? 'won' : 'miss'">{{
            result.won ? t('admin.copy.638b1e06') : t('admin.copy.5fa501a3')
          }}</span>
        </header>
        <div class="summary-grid">
          <div>
            <small>{{ t('admin.copy.ba87920d') }}</small><strong>{{ result.combination_count }}</strong>
          </div>
          <div>
            <small>{{ t('admin.copy.a062ae4f') }}</small><strong>{{ result.multiplier }}</strong>
          </div>
          <div>
            <small>{{ t('admin.copy.a8314239') }}</small><strong>{{ result.bet_points }}</strong>
          </div>
          <div>
            <small>{{ t('admin.copy.c018fe9a') }}</small><strong>{{ result.prize_points }}</strong>
          </div>
        </div>
        <p v-if="form.roundingScope === 'order'" class="rounding-note">
          {{ t('admin.copy.30613ac7') }}
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
          <h4>{{ t('admin.rule.betLine', { index: lineIndex + 1 }) }} · {{ line.points }} {{ t('admin.copy.feacf287') }}</h4>
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
                    ? t('admin.copy.fd340701')
                    : t('admin.copy.fd143794')
                  : t('admin.copy.406ff986')
              }}</span>
            </div>
            <p>
              {{ t('admin.copy.1036bc1a') }} {{ hit.raw_points }} · {{ t('admin.copy.b244a5c2') }}
              {{ hit.points }} ·
              {{ hit.exclusive ? t('admin.copy.5c080016') : t('admin.copy.1606fa6e') }}
            </p>
            <details>
              <summary>{{ t('admin.copy.ca105086') }}</summary>
              <BusinessDetails :value="hit.trace" :locale="associationLocale" />
            </details>
          </div>
        </article>
        <details class="normalized">
          <summary>{{ t('admin.copy.8035cec8') }}</summary>
          <BusinessDetails :value="result.normalized" :locale="associationLocale" />
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
  color: var(--muted);
  font-size: 11px;
  line-height: 1.55;
}
.eyebrow {
  font-size: 11px !important;
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
  background: var(--surface);
  border: 1px solid var(--line);
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
  grid-template-columns: minmax(0, 1fr);
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
  background: var(--surface);
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
  color: var(--muted);
  font-size: 11px;
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
  font-size: 11px;
  min-width: 0;
}
.button-primary,
.button-secondary,
.remove {
  border-radius: 7px;
  padding: 9px 12px;
  font-size: 11px;
  font-weight: 700;
}
.button-primary {
  border: 0;
  background: var(--primary);
  color: var(--surface);
}
.button-secondary,
.remove {
  border: 1px solid #e0e3ea;
  background: var(--surface);
  color: #586174;
}
.remove {
  padding: 8px;
}
.button-primary:disabled,
.button-secondary:disabled {
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
  border: 1px solid var(--line);
  border-radius: 8px;
  min-width: 0;
}
.summary-grid small {
  font-size: 11px;
  color: var(--muted);
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
  border: 1px solid var(--line);
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
  font-size: 11px;
  color: var(--muted);
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
  font-size: 11px;
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
