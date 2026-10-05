<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { AdminApiError, type AdminAccount } from "./admin-api";
import {
  buildRuleSimulationRequest,
  RULE_TEMPLATE_LABELS,
  type RuleSimulationForm,
  type RuleTemplate,
} from "./rule-simulation-api";
import {
  buildRuleValidationCase,
  canReviewRuleVersion,
  canCloneRuleVersion,
  createRuleVersionKeyTracker,
  createRuleVersionRequestGuard,
  createRuleVersionsApi,
  defaultRuleVersionForm,
  restoreRuleVersionTemplate,
  ruleDefinitionSignature,
  ruleTemplateModel,
  ruleVersionPermissions,
  ruleVersionDefinitionLocked,
  type GameRecord,
  type PlayRecord,
  type RuleEffectMode,
  type RuleValidationReport,
  type RuleVersion,
  type RuleVersionStatus,
} from "./rule-versions-api";

const props = defineProps<{ account: AdminAccount; brandId: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const api = createRuleVersionsApi();
const guard = createRuleVersionRequestGuard();
const keyFor = createRuleVersionKeyTracker();
const rights = computed(() =>
  ruleVersionPermissions(props.account, props.brandId),
);
const templates = Object.entries(RULE_TEMPLATE_LABELS) as [
  RuleTemplate,
  string,
][];
const games = ref<GameRecord[]>([]);
const plays = ref<PlayRecord[]>([]);
const versions = ref<RuleVersion[]>([]);
const gameId = ref("");
const playId = ref("");
const directPlayId = ref("");
const selected = ref<RuleVersion | null>(null);
const newDraft = ref(false);
const supported = ref(true);
const template = ref<RuleTemplate>("special");
const form = ref<RuleSimulationForm>(defaultRuleVersionForm("special"));
const effectMode = ref<RuleEffectMode>("immediate");
const draftReason = ref("");
const expected = ref({ name: "", bet: "", prize: "", won: false });
const validationDisplay = ref<RuleValidationReport | null>(null);
const validationInputsChanged = ref(false);
const review = ref({ reason: "", acknowledged: false });
const clone = ref({ reason: "", effectMode: "next_period" as RuleEffectMode });
const gameCreate = ref({
  code: "",
  name: "",
  template: "special" as RuleTemplate,
  timezone: "UTC",
  reason: "",
});
const playCreate = ref({ gameId: "", code: "", name: "", reason: "" });
const loadingGames = ref(false);
const loadingPlays = ref(false);
const loadingVersions = ref(false);
const busy = ref(false);
const error = ref("");
const notice = ref("");
const conflict = ref(false);
let hydrating = false;
let activeWrite: ReturnType<typeof guard.capture> | null = null;

const statusLabels: Record<RuleVersionStatus, string> = {
  draft: "草稿",
  pending_review: "待审核",
  approved: "已批准",
  active: "生效中",
  expired: "已过期",
  rejected: "已拒绝",
  rolled_back: "已回滚",
};
const commonInputs = [
  { key: "unitPoints" as const, label: "单位积分（正整数）" },
  { key: "odds" as const, label: "赔率（正十进制数，最多 6 位小数）" },
  { key: "capPoints" as const, label: "中奖封顶积分（正整数，空白不限）" },
  { key: "maxBetPoints" as const, label: "投注限额积分（正整数，空白不限）" },
];
const featureInputs = [
  { key: "three_kind", label: "三同号（0 否 / 1 是；空白不启用）" },
  { key: "same_ends", label: "首尾同号（0 否 / 1 是；空白不启用）" },
  { key: "odd_count", label: "奇数个数（0–3；空白不启用）" },
  { key: "sum", label: "和值（0–27；空白不启用）" },
];
const selectedGame = computed(() =>
  games.value.find(
    (item) => item.id === (selected.value?.game_id || gameId.value),
  ),
);
const chosenPlay = computed(() =>
  plays.value.find((item) => item.id === playId.value),
);
const isDraft = computed(() => selected.value?.status === "draft");
const definitionLocked = computed(
  () => selected.value !== null && ruleVersionDefinitionLocked(selected.value),
);
const cloneAllowed = computed(
  () =>
    selected.value !== null &&
    canCloneRuleVersion(props.account, props.brandId, selected.value),
);
const editable = computed(
  () => newDraft.value || (isDraft.value && supported.value),
);
const dirty = computed(() => {
  if (!selected.value) return false;
  try {
    return (
      effectMode.value !== selected.value.effect_mode ||
      ruleDefinitionSignature(
        buildRuleSimulationRequest(template.value, form.value).definition,
      ) !== ruleDefinitionSignature(selected.value.definition)
    );
  } catch {
    return true;
  }
});
const readyToSubmit = computed(
  () =>
    isDraft.value &&
    !dirty.value &&
    !validationInputsChanged.value &&
    selected.value?.validation?.passed === true,
);
const reviewAllowed = computed(
  () =>
    selected.value !== null &&
    canReviewRuleVersion(props.account, props.brandId, selected.value),
);

function scope(lane: string): string {
  const base = [
    props.brandId,
    props.account.id,
    props.account.super_admin,
    props.account.permissions,
    props.account.permissions_by_brand,
    props.account.platform_permissions,
  ];
  if (lane === "plays") base.push(gameId.value);
  if (lane === "versions") base.push(playId.value);
  if (lane === "write")
    return JSON.stringify([
      base,
      gameId.value,
      playId.value,
      selected.value?.id,
      template.value,
      form.value,
      effectMode.value,
      draftReason.value,
      expected.value,
      review.value,
      clone.value,
      gameCreate.value,
      playCreate.value,
    ]);
  return JSON.stringify(base);
}
function current(ticket: ReturnType<typeof guard.capture>, permitted: boolean) {
  return guard.isCurrent(ticket, scope(ticket.lane), permitted);
}
function hydrate(action: () => void) {
  hydrating = true;
  try {
    action();
  } finally {
    hydrating = false;
  }
}
function resetEditor() {
  hydrate(() => {
    selected.value = null;
    newDraft.value = false;
    supported.value = true;
    template.value = "special";
    form.value = defaultRuleVersionForm("special");
    effectMode.value = "immediate";
    draftReason.value = "";
    expected.value = { name: "", bet: "", prize: "", won: false };
    review.value = { reason: "", acknowledged: false };
    clone.value = { reason: "", effectMode: "next_period" };
  });
  validationDisplay.value = null;
  validationInputsChanged.value = false;
  guard.invalidate("write");
  activeWrite = null;
  busy.value = false;
}
function showError(cause: unknown) {
  if (cause instanceof AdminApiError && cause.status === 401)
    emit("session-invalid");
  conflict.value = cause instanceof AdminApiError && cause.status === 409;
  const detail = cause instanceof Error ? cause.message : "请求失败，请重试。";
  const code =
    cause instanceof AdminApiError && cause.code ? ` [${cause.code}]` : "";
  error.value = conflict.value
    ? `${detail}${code}；版本或状态冲突，请重新读取历史确认最新状态，再重新操作。`
    : `${detail}${code}`;
}
function reason(value: string) {
  if (!value.trim()) throw new Error("每次操作都必须填写原因。");
  return value.trim();
}
function pretty(value: unknown) {
  return JSON.stringify(value, null, 2);
}
function checkBrand(item: { brand_id: string }) {
  if (item.brand_id !== props.brandId)
    throw new Error("响应品牌与当前品牌不匹配，请重新读取。");
}

async function loadGames() {
  if (!rights.value.gamesView) return;
  const brandId = props.brandId;
  const ticket = guard.capture("games", scope("games"));
  games.value = [];
  loadingGames.value = true;
  error.value = "";
  try {
    const result = await api.getGames(brandId);
    if (!current(ticket, rights.value.gamesView)) return;
    result.games.forEach(checkBrand);
    games.value = result.games;
  } catch (cause) {
    if (current(ticket, rights.value.gamesView)) {
      if (cause instanceof AdminApiError && cause.status === 403)
        void chooseGame("");
      showError(cause);
    }
  } finally {
    if (current(ticket, rights.value.gamesView)) loadingGames.value = false;
  }
}
async function chooseGame(id: string) {
  guard.invalidate("plays");
  guard.invalidate("versions");
  gameId.value = id;
  playCreate.value.gameId = id;
  playId.value = "";
  directPlayId.value = "";
  plays.value = [];
  versions.value = [];
  loadingPlays.value = false;
  loadingVersions.value = false;
  resetEditor();
  notice.value = "";
  error.value = "";
  if (!id || !rights.value.gamesView) return;
  const ticket = guard.capture("plays", scope("plays"));
  const brandId = props.brandId;
  loadingPlays.value = true;
  try {
    const result = await api.getPlays(brandId, id);
    if (!current(ticket, rights.value.gamesView)) return;
    result.plays.forEach((item) => {
      checkBrand(item);
      if (item.game_id !== id) throw new Error("玩法响应不属于当前彩种。");
    });
    plays.value = result.plays;
  } catch (cause) {
    if (current(ticket, rights.value.gamesView)) showError(cause);
  } finally {
    if (current(ticket, rights.value.gamesView)) loadingPlays.value = false;
  }
}
async function choosePlay(id: string, direct = false) {
  if (direct) {
    gameId.value = "";
    plays.value = [];
    guard.invalidate("plays");
    loadingPlays.value = false;
  }
  playId.value = id.trim();
  directPlayId.value = playId.value;
  versions.value = [];
  resetEditor();
  notice.value = "";
  error.value = "";
  await loadVersions();
}
function chooseVersion(record: RuleVersion, preserveInputs = false) {
  hydrate(() => {
    selected.value = record;
    newDraft.value = false;
    const restored = restoreRuleVersionTemplate(record.definition);
    supported.value = restored !== null;
    if (!preserveInputs && restored) {
      template.value = restored.template;
      form.value = restored.form;
    }
    effectMode.value = record.effect_mode;
    draftReason.value = "";
    if (!preserveInputs)
      expected.value = { name: "", bet: "", prize: "", won: false };
    review.value = { reason: "", acknowledged: false };
    clone.value = { reason: "", effectMode: record.effect_mode };
  });
  validationDisplay.value = record.validation ?? null;
  validationInputsChanged.value = false;
  guard.invalidate("write");
  activeWrite = null;
  busy.value = false;
}
async function loadVersions(focusId = selected.value?.id) {
  if (!rights.value.rulesView || !playId.value) return;
  const target = playId.value;
  const brandId = props.brandId;
  const ticket = guard.capture("versions", scope("versions"));
  versions.value = [];
  resetEditor();
  loadingVersions.value = true;
  error.value = "";
  try {
    const result = await api.getRuleVersions(brandId, target);
    if (!current(ticket, rights.value.rulesView)) return;
    result.versions.forEach((item) => {
      checkBrand(item);
      if (item.play_id !== target) throw new Error("版本响应不属于当前玩法。");
    });
    versions.value = result.versions;
    const record =
      result.versions.find((item) => item.id === focusId) ?? result.versions[0];
    if (record) chooseVersion(record);
    conflict.value = false;
  } catch (cause) {
    if (current(ticket, rights.value.rulesView)) showError(cause);
  } finally {
    if (current(ticket, rights.value.rulesView)) loadingVersions.value = false;
  }
}
function startDraft() {
  if (!rights.value.rulesWrite || !playId.value || busy.value) return;
  guard.invalidate("versions");
  loadingVersions.value = false;
  resetEditor();
  hydrate(() => {
    newDraft.value = true;
    const model = selectedGame.value?.model.model;
    template.value =
      templates.find(
        ([value]) => !model || ruleTemplateModel(value) === model,
      )?.[0] ?? "special";
    form.value = defaultRuleVersionForm(template.value);
  });
  error.value = "";
  notice.value = "";
}
function changeTemplate() {
  form.value = defaultRuleVersionForm(template.value);
}

async function mutate<T extends { brand_id: string; id: string }>(
  operation: string,
  target: string,
  body: unknown,
  permitted: () => boolean,
  request: (brand: string, key: string) => Promise<T>,
  apply: (result: T) => void,
  message: string,
) {
  if (busy.value || !permitted()) return;
  const ticket = guard.capture("write", scope("write"));
  const brandId = props.brandId;
  activeWrite = ticket;
  busy.value = true;
  error.value = "";
  notice.value = "";
  conflict.value = false;
  try {
    const result = await request(
      brandId,
      keyFor(brandId, operation, target, body),
    );
    if (!current(ticket, permitted())) return;
    checkBrand(result);
    apply(result);
    notice.value = `${message}（ID：${result.id}）。`;
  } catch (cause) {
    if (current(ticket, permitted())) showError(cause);
  } finally {
    if (activeWrite === ticket) {
      busy.value = false;
      activeWrite = null;
    }
  }
}
function applyVersion(record: RuleVersion, preserve = true) {
  if (record.play_id !== playId.value)
    throw new Error("保存结果不属于当前玩法，请重新读取。");
  versions.value = [
    record,
    ...versions.value.filter((item) => item.id !== record.id),
  ];
  chooseVersion(record, preserve);
}
async function createGame() {
  try {
    const input = gameCreate.value;
    if (!input.code.trim() || !input.name.trim())
      throw new Error("请填写彩种代码和名称。");
    new Intl.DateTimeFormat("en", { timeZone: input.timezone.trim() });
    const model = buildRuleSimulationRequest(
      input.template,
      defaultRuleVersionForm(input.template),
    ).definition.model;
    const body = {
      code: input.code.trim(),
      name: input.name.trim(),
      model,
      timezone: input.timezone.trim(),
      reason: reason(input.reason),
    };
    await mutate(
      "create-game",
      "games",
      body,
      () => rights.value.gamesWrite,
      (brand, key) => api.createGame(brand, body, key),
      (result) => {
        gameCreate.value = {
          code: "",
          name: "",
          template: "special",
          timezone: "UTC",
          reason: "",
        };
        playCreate.value.gameId = result.id;
        if (rights.value.gamesView) {
          games.value = [...games.value, result];
          void chooseGame(result.id);
        }
      },
      "彩种已创建",
    );
  } catch (cause) {
    showError(cause);
  }
}
async function createPlay() {
  try {
    const input = playCreate.value;
    const target = input.gameId.trim();
    if (!target || !input.code.trim() || !input.name.trim())
      throw new Error("请填写彩种 ID、玩法代码和名称。");
    const body = {
      code: input.code.trim(),
      name: input.name.trim(),
      reason: reason(input.reason),
    };
    await mutate(
      "create-play",
      target,
      body,
      () => rights.value.gamesWrite,
      (brand, key) => api.createPlay(brand, target, body, key),
      (result) => {
        if (result.game_id !== target)
          throw new Error("创建结果不属于指定彩种。");
        playCreate.value = { gameId: target, code: "", name: "", reason: "" };
        if (rights.value.gamesView && gameId.value === target)
          plays.value = [...plays.value, result];
        playId.value = result.id;
        directPlayId.value = result.id;
        versions.value = [];
        resetEditor();
        if (rights.value.rulesView) void loadVersions();
      },
      "玩法已创建",
    );
  } catch (cause) {
    showError(cause);
  }
}
async function persistDraft() {
  try {
    if (
      (!editable.value &&
        !(isDraft.value && selected.value?.source_version_id)) ||
      !playId.value
    )
      return;
    const definition = selected.value?.source_version_id
      ? selected.value.definition
      : buildRuleSimulationRequest(template.value, form.value).definition;
    if (
      selectedGame.value &&
      selectedGame.value.model.model !== definition.model.model
    )
      throw new Error("规则模型与当前彩种不一致，请选择同模型模板。");
    const common = {
      definition,
      effect_mode: effectMode.value,
      reason: reason(draftReason.value),
    };
    const record = selected.value;
    if (newDraft.value) {
      const body = { play_id: playId.value, ...common };
      await mutate(
        "create-draft",
        playId.value,
        body,
        () => rights.value.rulesWrite && newDraft.value,
        (brand, key) => api.createRuleVersion(brand, body, key),
        (result) => applyVersion(result),
        "草稿已保存，尚未提交审核",
      );
    } else if (record?.status === "draft") {
      const body = { version: record.version, ...common };
      await mutate(
        "update-draft",
        record.id,
        body,
        () => rights.value.rulesWrite && selected.value?.status === "draft",
        (brand, key) => api.updateRuleVersion(brand, record.id, body, key),
        (result) => applyVersion(result, !record.source_version_id),
        "草稿修改已保存，请重新验证",
      );
    }
  } catch (cause) {
    showError(cause);
  }
}
async function validateDraft() {
  try {
    const record = selected.value;
    if (!record || !isDraft.value || !supported.value) return;
    if (dirty.value)
      throw new Error("规则配置已修改，请先保存草稿，再验证保存的定义。");
    const body = {
      version: record.version,
      cases: [
        buildRuleValidationCase(template.value, form.value, expected.value),
      ],
      reason: reason(draftReason.value),
    };
    await mutate(
      "validate",
      record.id,
      body,
      () => rights.value.validate && selected.value?.status === "draft",
      (brand, key) => api.validateRuleVersion(brand, record.id, body, key),
      (result) => applyVersion(result),
      "服务端验证结果已保存，请检查通过状态与警告",
    );
  } catch (cause) {
    showError(cause);
  }
}
async function submitReview() {
  try {
    const record = selected.value;
    if (!record || !readyToSubmit.value) return;
    const body = { version: record.version, reason: reason(draftReason.value) };
    await mutate(
      "submit-review",
      record.id,
      body,
      () => rights.value.submit && readyToSubmit.value,
      (brand, key) => api.submitRuleVersion(brand, record.id, body, key),
      (result) => applyVersion(result),
      "已提交审核，等待其他品牌管理员决定",
    );
  } catch (cause) {
    showError(cause);
  }
}
async function decide(approve: boolean) {
  try {
    const record = selected.value;
    if (!record || !reviewAllowed.value) return;
    if (approve && !review.value.acknowledged)
      throw new Error("批准前请确认已阅读验证结果和全部警告。");
    const common = {
      version: record.version,
      reason: reason(review.value.reason),
    };
    const body = approve
      ? { ...common, warnings_acknowledged: review.value.acknowledged }
      : common;
    await mutate(
      approve ? "approve" : "reject",
      record.id,
      body,
      () => reviewAllowed.value,
      (brand, key) =>
        approve
          ? api.approveRuleVersion(
              brand,
              record.id,
              { ...common, warnings_acknowledged: review.value.acknowledged },
              key,
            )
          : api.rejectRuleVersion(brand, record.id, common, key),
      (result) => {
        applyVersion(result);
        if (rights.value.rulesView) void loadVersions(result.id);
      },
      approve
        ? "审批决定已保存，生效状态以服务端返回及期次绑定为准"
        : "已拒绝，原定义保留在历史中",
    );
  } catch (cause) {
    showError(cause);
  }
}
async function cloneVersion() {
  try {
    const record = selected.value;
    if (!record || !cloneAllowed.value) return;
    const body = {
      effect_mode: clone.value.effectMode,
      reason: reason(clone.value.reason),
    };
    await mutate(
      "clone",
      record.id,
      body,
      () => cloneAllowed.value,
      (brand, key) => api.cloneRuleVersion(brand, record.id, body, key),
      (result) => applyVersion(result, false),
      "已克隆为新草稿，来源版本保持不变",
    );
  } catch (cause) {
    showError(cause);
  }
}

watch(
  [template, form, effectMode, expected],
  () => {
    if (hydrating) return;
    guard.invalidate("write");
    validationDisplay.value = null;
    validationInputsChanged.value = true;
    notice.value = "";
  },
  { deep: true, flush: "sync" },
);
watch(
  () => [props.brandId, props.account],
  () => {
    guard.invalidate();
    games.value = [];
    plays.value = [];
    versions.value = [];
    gameId.value = "";
    playId.value = "";
    directPlayId.value = "";
    loadingGames.value = false;
    loadingPlays.value = false;
    loadingVersions.value = false;
    resetEditor();
    gameCreate.value = {
      code: "",
      name: "",
      template: "special",
      timezone: "UTC",
      reason: "",
    };
    playCreate.value = { gameId: "", code: "", name: "", reason: "" };
    error.value = "";
    notice.value = "";
    conflict.value = false;
    if (rights.value.gamesView) void loadGames();
  },
  { deep: true, immediate: true, flush: "sync" },
);
</script>

<template>
  <section class="rule-versions">
    <header class="section-head">
      <div>
        <p class="eyebrow">规则版本管理</p>
        <h2>彩种、玩法与规则审批</h2>
      </div>
      <span>品牌 · {{ brandId || "未选择" }}</span>
    </header>
    <p class="notice">
      所有保存、验证和审批均调用真实接口。提交审核后定义冻结，创建者和所有编辑者均不能审核该版本。权限及生效状态由服务端最终判定。
    </p>
    <p v-if="account.super_admin" class="notice">
      超级管理员仅可按显式读取权限查看，不可创建、修改、验证、提交或审批。
    </p>
    <p v-if="error" class="notice error" role="alert">{{ error }}</p>
    <p v-if="notice" class="notice success" role="status">{{ notice }}</p>
    <button
      v-if="conflict && rights.rulesView && playId"
      type="button"
      :disabled="busy || loadingVersions"
      @click="loadVersions()"
    >
      重新读取版本历史（将清除未保存输入）
    </button>

    <div class="catalog-grid">
      <section class="card">
        <h3>1 · 选择彩种和玩法</h3>
        <template v-if="rights.gamesView">
          <button
            type="button"
            :disabled="busy || loadingGames"
            @click="loadGames"
          >
            {{ loadingGames ? "读取彩种中…" : "刷新彩种" }}
          </button>
          <label
            >彩种<select
              :value="gameId"
              :disabled="busy || loadingGames"
              @change="chooseGame(($event.target as HTMLSelectElement).value)"
            >
              <option value="">请选择彩种</option>
              <option v-for="item in games" :key="item.id" :value="item.id">
                {{ item.name }} · {{ item.code }} · {{ item.model.model }}
              </option>
            </select></label
          >
          <p v-if="!loadingGames && !games.length" class="hint">
            当前品牌没有可显示的彩种。
          </p>
          <label
            >玩法<select
              :value="playId"
              :disabled="busy || loadingPlays || !gameId"
              @change="choosePlay(($event.target as HTMLSelectElement).value)"
            >
              <option value="">
                {{ loadingPlays ? "读取玩法中…" : "请选择玩法" }}
              </option>
              <option v-for="item in plays" :key="item.id" :value="item.id">
                {{ item.name }} · {{ item.code }} · {{ item.status }}
              </option>
            </select></label
          >
          <p v-if="chosenPlay?.active_version_id" class="hint">
            当前生效版本 ID：{{ chosenPlay.active_version_id }}
          </p>
        </template>
        <p v-else class="hint">
          缺少
          game.view.brand/platform，不能读取彩种或玩法目录。写权限不自动包含读取权限。
        </p>
        <form
          v-if="rights.rulesView || rights.rulesWrite"
          @submit.prevent="choosePlay(directPlayId, true)"
        >
          <label
            >直接指定玩法 ID<input
              v-model.trim="directPlayId"
              required
              :disabled="busy"
              placeholder="玩法 UUID"
          /></label>
          <button type="submit" :disabled="busy || !directPlayId">
            选择玩法{{ rights.rulesView ? "并读取历史" : "以创建草稿" }}
          </button>
        </form>
      </section>
      <section class="card">
        <h3>2 · 版本历史</h3>
        <p v-if="!rights.rulesView" class="hint">
          缺少
          rule.view.brand/platform，不能读取版本历史。其他步骤的权限不自动包含读取权限。
        </p>
        <template v-else>
          <button
            type="button"
            :disabled="busy || loadingVersions || !playId"
            @click="loadVersions()"
          >
            {{ loadingVersions ? "读取历史中…" : "刷新版本历史" }}
          </button>
          <p v-if="!playId" class="hint">先选择玩法。</p>
          <p v-else-if="!loadingVersions && !versions.length" class="hint">
            当前玩法没有可显示的版本。
          </p>
          <ul class="version-list">
            <li v-for="record in versions" :key="record.id">
              <button
                type="button"
                :class="{ selected: selected?.id === record.id }"
                :disabled="busy"
                @click="chooseVersion(record)"
              >
                <strong
                  >第 {{ record.version_no }} 版 ·
                  {{ statusLabels[record.status] }}</strong
                ><span
                  >CAS {{ record.version }} ·
                  {{
                    record.effect_mode === "immediate"
                      ? "立即生效"
                      : "下一期生效"
                  }}</span
                ><small
                  >{{ record.updated_at }} · 创建者
                  {{ record.created_by }}</small
                >
              </button>
            </li>
          </ul>
        </template>
        <button
          v-if="rights.rulesWrite"
          type="button"
          :disabled="busy || !playId"
          @click="startDraft"
        >
          新建规则草稿
        </button>
      </section>
    </div>

    <div v-if="rights.gamesWrite" class="catalog-grid">
      <details class="card">
        <summary>创建彩种（由模板确定模型）</summary>
        <form @submit.prevent="createGame">
          <fieldset :disabled="busy" class="form-grid">
            <label>代码<input v-model.trim="gameCreate.code" required /></label
            ><label
              >名称<input v-model.trim="gameCreate.name" required
            /></label>
            <label
              >模型模板<select v-model="gameCreate.template">
                <option
                  v-for="[value, label] in templates"
                  :key="value"
                  :value="value"
                >
                  {{ label }} · {{ ruleTemplateModel(value) }}
                </option>
              </select></label
            >
            <label
              >IANA 时区<input
                v-model.trim="gameCreate.timezone"
                required
                placeholder="Asia/Manila"
            /></label>
            <label class="wide"
              >创建原因<textarea
                v-model="gameCreate.reason"
                required
                rows="2"
              /></label
            ><button class="wide" type="submit">创建彩种</button>
          </fieldset>
        </form>
      </details>
      <details class="card">
        <summary>创建玩法</summary>
        <form @submit.prevent="createPlay">
          <fieldset :disabled="busy" class="form-grid">
            <label class="wide"
              >彩种 ID<input v-model.trim="playCreate.gameId" required
            /></label>
            <label>代码<input v-model.trim="playCreate.code" required /></label
            ><label
              >名称<input v-model.trim="playCreate.name" required
            /></label>
            <label class="wide"
              >创建原因<textarea
                v-model="playCreate.reason"
                required
                rows="2"
              /></label
            ><button class="wide" type="submit">创建玩法</button>
          </fieldset>
        </form>
      </details>
    </div>

    <section v-if="selected" class="card history">
      <h3>
        第 {{ selected.version_no }} 版 · {{ statusLabels[selected.status] }}
      </h3>
      <dl>
        <dt>版本 ID / CAS</dt>
        <dd>{{ selected.id }} / {{ selected.version }}</dd>
        <dt>创建者</dt>
        <dd>{{ selected.created_by }}</dd>
        <dt>审核者 / 意见</dt>
        <dd>
          {{ selected.reviewed_by || "尚无" }} /
          {{ selected.review_comment || "尚无" }}
        </dd>
        <dt>生效时间 / 期次 / 序号</dt>
        <dd>
          {{ selected.effective_at || "尚无" }} /
          {{ selected.effective_period_id || "尚无" }} /
          {{ selected.effective_sequence ?? "尚无" }}
        </dd>
        <dt>来源版本</dt>
        <dd>{{ selected.source_version_id || "无" }}</dd>
        <dt v-if="selected.audit_log_id">审计 ID</dt>
        <dd v-if="selected.audit_log_id">{{ selected.audit_log_id }}</dd>
      </dl>
      <details>
        <summary>已保存规则定义（只读 JSON）</summary>
        <pre>{{ pretty(selected.definition) }}</pre>
      </details>
      <p v-if="isDraft && !supported" class="notice">
        当前定义无法由这六个模板完整还原，保留只读显示。请通过支持该定义的管理工具修改；此处不会用模板覆盖原定义。
      </p>
      <p v-if="isDraft && selected.source_version_id" class="notice">
        这是回滚草稿，定义必须与来源版本完全相同，只允许调整生效方式。仍需重新验证，并由未参与创建或编辑的其他管理员审核。
      </p>
      <form
        v-if="isDraft && selected.source_version_id && rights.rulesWrite"
        aria-label="回滚草稿生效方式"
        @submit.prevent="persistDraft"
      >
        <fieldset :disabled="busy">
          <legend>回滚草稿 · 仅调整生效方式</legend>
          <label
            >生效方式<select v-model="effectMode">
              <option value="immediate">批准后立即生效</option>
              <option value="next_period">下一期生效</option>
            </select></label
          >
          <label
            >变更原因<textarea v-model="draftReason" required rows="2" /></label
          ><button type="submit">仅保存生效方式</button>
        </fieldset>
      </form>
      <p v-if="!isDraft" class="hint">
        该版本不可修改。需要变更时可克隆为新草稿，重新验证和提交审批。
      </p>
    </section>

    <section v-if="editable" class="card editor">
      <h3>3 · {{ newDraft ? "新建草稿" : "草稿配置与验证" }}</h3>
      <form @submit.prevent="persistDraft">
        <fieldset
          :disabled="busy || !rights.rulesWrite || definitionLocked"
          class="form-grid"
        >
          <label class="wide"
            >模板<select v-model="template" @change="changeTemplate">
              <option
                v-for="[value, label] in templates"
                :key="value"
                :value="value"
                :disabled="
                  !!selectedGame &&
                  ruleTemplateModel(value) !== selectedGame.model.model
                "
              >
                {{ label }}
              </option>
            </select></label
          >
          <label v-for="input in commonInputs" :key="input.key"
            >{{ input.label
            }}<input
              v-model.trim="form[input.key]"
              :inputmode="input.key === 'odds' ? 'decimal' : 'numeric'"
          /></label>
          <label
            >舍入范围<select v-model="form.roundingScope">
              <option value="order">整注汇总后舍入</option>
              <option value="line">每组合舍入后汇总</option>
              <option value="tier">每奖级舍入后汇总</option>
            </select></label
          >
          <label v-if="template === 'features'" class="wide"
            >特征条件关系<select v-model="form.conditionJoin">
              <option value="all">全部满足（AND）</option>
              <option value="any">任一满足（OR）</option>
            </select></label
          >
          <template v-if="template === 'attributes'"
            ><label
              >红色号码映射<input
                v-model.trim="form.redNumbers"
                inputmode="numeric" /></label
            ><label
              >蓝色号码映射<input
                v-model.trim="form.blueNumbers"
                inputmode="numeric" /></label
          ></template>
        </fieldset>
        <label v-if="!definitionLocked"
          >生效方式<select
            v-model="effectMode"
            :disabled="busy || !rights.rulesWrite"
          >
            <option value="immediate">批准后立即生效</option>
            <option value="next_period">下一期生效</option>
          </select></label
        >
        <p class="hint">
          积分和倍率使用整数字符串；赔率精确计算。组合上限 10000，倍率上限
          1000。生效方式以服务端审批和期次绑定结果为准。
        </p>
        <label v-if="rights.rulesWrite && !definitionLocked"
          >保存原因<textarea
            v-model="draftReason"
            required
            rows="2"
            :disabled="busy"
          />
        </label>
        <button
          v-if="rights.rulesWrite && !definitionLocked"
          type="submit"
          :disabled="busy"
        >
          {{ newDraft ? "保存新草稿" : "保存草稿修改" }}
        </button>
      </form>

      <form @submit.prevent="validateDraft">
        <fieldset
          :disabled="busy || (!rights.validate && !rights.rulesWrite)"
          class="form-grid"
        >
          <legend>完整验证用例 · 输入选号、开奖和独立预期</legend>
          <label v-if="template === 'special'"
            >特别号候选<input
              v-model.trim="form.specialNumbers"
              inputmode="numeric"
              placeholder="7,19,31,43"
          /></label>
          <template v-if="template === 'm-select-n'"
            ><label
              >所选普通号（6 个）<input
                v-model.trim="form.regularNumbers"
                inputmode="numeric" /></label
            ><label
              >所选特别号（1 个）<input
                v-model.trim="form.specialNumbers"
                inputmode="numeric" /></label
          ></template>
          <label v-if="template === 'exclude'"
            >排除号码（逗号分隔，数量也属于规则配置）<input
              v-model.trim="form.excludedNumbers"
              inputmode="numeric"
          /></label>
          <template v-if="template === 'digits'"
            ><label v-for="(_, index) in form.digitCandidates" :key="index"
              >第 {{ index + 1 }} 位候选（0–9，逗号分隔）<input
                v-model.trim="form.digitCandidates[index]"
                inputmode="numeric" /></label
          ></template>
          <template v-if="template === 'features'"
            ><label v-for="input in featureInputs" :key="input.key"
              >{{ input.label
              }}<input
                v-model.trim="form.featureValues[input.key]"
                inputmode="numeric"
            /></label>
            <p class="hint wide">
              每项仅选一个值。新增或移除特征需先保存草稿，允许值由模板确定。
            </p></template
          >
          <div v-if="template === 'attributes'" class="wide">
            <span>所选颜色（可多选）</span
            ><label class="check"
              ><input
                v-model="form.selectedColors"
                type="checkbox"
                value="red"
              />红</label
            ><label class="check"
              ><input
                v-model="form.selectedColors"
                type="checkbox"
                value="blue"
              />蓝</label
            >
          </div>
          <label v-if="template === 'digits' || template === 'features'"
            >三位开奖结果<input
              v-model.trim="form.drawDigits"
              inputmode="numeric"
          /></label>
          <template v-else
            ><label
              >开奖普通号（6 个）<input
                v-model.trim="form.drawRegular"
                inputmode="numeric" /></label
            ><label
              >开奖特别号（1 个）<input
                v-model.trim="form.drawSpecial"
                inputmode="numeric" /></label
          ></template>
          <label
            >验证倍率（1–1000 整数）<input
              v-model.trim="form.multiplier"
              inputmode="numeric"
          /></label>
          <label>用例名称<input v-model.trim="expected.name" required /></label>
          <label
            >预期投注积分<input
              v-model.trim="expected.bet"
              inputmode="numeric"
              required
              placeholder="规范非负整数"
          /></label>
          <label
            >预期中奖积分<input
              v-model.trim="expected.prize"
              inputmode="numeric"
              required
              placeholder="规范非负整数"
          /></label>
          <label
            >预期是否中奖<select v-model="expected.won">
              <option :value="true">中奖</option>
              <option :value="false">未中奖</option>
            </select></label
          >
          <label v-if="rights.validate && selected" class="wide"
            >验证原因<textarea v-model="draftReason" required rows="2" />
          </label>
          <button
            v-if="rights.validate && selected"
            class="wide"
            type="submit"
            :disabled="busy || dirty || !isDraft"
          >
            验证已保存草稿并保存报告
          </button>
        </fieldset>
      </form>
      <p v-if="newDraft" class="hint">
        先保存草稿，再调用验证接口；预期结果由运营填写，未调用 API
        时不会显示验证成功。
      </p>
      <p v-else-if="dirty" class="notice">
        输入与已保存配置不一致，暂不可验证或提交审核。普通草稿需先保存配置；回滚草稿的特征项目和排除数量必须与原定义一致。修改不会自动保存。
      </p>
      <p v-else-if="validationInputsChanged" class="hint">
        用例输入已变化，已清除旧验证显示，请重新验证。
      </p>
      <form
        v-if="rights.submit && selected && isDraft"
        @submit.prevent="submitReview"
      >
        <fieldset :disabled="busy">
          <label
            >提交审核原因<textarea v-model="draftReason" required rows="2" />
          </label>
          <button type="submit" :disabled="!readyToSubmit">
            提交审核（需服务端验证通过）
          </button>
        </fieldset>
      </form>
    </section>

    <section
      v-if="validationDisplay"
      class="card validation"
      aria-live="polite"
    >
      <h3>服务端验证：{{ validationDisplay.passed ? "通过" : "未通过" }}</h3>
      <p class="hint">定义校验指纹：{{ validationDisplay.definition_hash }}</p>
      <ul v-if="validationDisplay.warnings?.length">
        <li v-for="(warning, index) in validationDisplay.warnings" :key="index">
          {{ warning }}
        </li>
      </ul>
      <p v-else class="hint">服务端未返回警告。</p>
      <div
        v-for="(report, index) in validationDisplay.cases"
        :key="index"
        class="case-report"
      >
        <h4>
          {{ report.name }} · {{ report.matched ? "符合预期" : "不符合预期" }}
        </h4>
        <dl>
          <dt>投注积分 · 预期 / 实际</dt>
          <dd>
            {{ report.expected_bet_points }} / {{ report.actual_bet_points }}
          </dd>
          <dt>中奖积分 · 预期 / 实际</dt>
          <dd>
            {{ report.expected_prize_points }} /
            {{ report.actual_prize_points }}
          </dd>
          <dt>中奖 · 预期 / 实际</dt>
          <dd>
            {{ report.expected_won ? "是" : "否" }} /
            {{ report.actual_won ? "是" : "否" }}
          </dd>
        </dl>
      </div>
      <ul v-if="validationDisplay.findings?.length">
        <li v-for="(finding, index) in validationDisplay.findings" :key="index">
          {{ finding.blocking ? "阻断" : "提示" }} · {{ finding.code }}：{{
            finding.message
          }}
        </li>
      </ul>
      <details>
        <summary>验证用例和发现（只读）</summary>
        <pre>{{
          pretty({
            cases: validationDisplay.cases,
            findings: validationDisplay.findings,
          })
        }}</pre>
      </details>
    </section>

    <section v-if="selected?.status === 'pending_review'" class="card review">
      <h3>4 · 品牌管理员审核</h3>
      <p class="notice">
        审批会影响正式规则生效。请逐项阅读定义、验证报告、全部警告和生效方式；服务端会检查审核人是否为创建者或曾参与编辑，以及权限和版本。
      </p>
      <p v-if="selected.created_by === account.id" class="hint">
        你是该版本的创建者，不能批准或拒绝自己的版本，请由另一名具备品牌审核权限的管理员处理。
      </p>
      <p v-else-if="!reviewAllowed" class="hint">
        当前账号没有本品牌 rule.review.brand
        权限，或不具备服务端要求的品牌管理员审核身份。
      </p>
      <form v-else @submit.prevent="decide(true)">
        <fieldset :disabled="busy">
          <label
            >审核原因（批准和拒绝均必填）<textarea
              v-model="review.reason"
              required
              rows="3"
            />
          </label>
          <label class="check"
            ><input
              v-model="review.acknowledged"
              type="checkbox"
            />我已阅读验证结果、全部警告及生效范围，确认批准。</label
          >
          <div class="actions">
            <button type="submit" :disabled="!review.acknowledged">
              批准该版本</button
            ><button
              type="button"
              class="danger"
              :disabled="!review.reason.trim()"
              @click="decide(false)"
            >
              拒绝并记录原因
            </button>
          </div>
        </fieldset>
      </form>
    </section>
    <form
      v-if="selected && cloneAllowed"
      class="card"
      @submit.prevent="cloneVersion"
    >
      <fieldset :disabled="busy" class="form-grid">
        <legend>克隆当前版本为新草稿</legend>
        <label
          >新草稿生效方式<select v-model="clone.effectMode">
            <option value="immediate">批准后立即生效</option>
            <option value="next_period">下一期生效</option>
          </select></label
        >
        <label
          >克隆原因<textarea v-model="clone.reason" required rows="2" /></label
        ><button class="wide" type="submit">克隆（来源定义保持不变）</button>
      </fieldset>
    </form>
    <p v-if="busy" role="status" class="hint">请求处理中，请等待服务端结果…</p>
  </section>
</template>

<style scoped>
.rule-versions {
  min-width: 0;
  max-width: 100%;
  color: #263849;
  display: grid;
  gap: 16px;
}
.rule-versions *,
.rule-versions *::before,
.rule-versions *::after {
  box-sizing: border-box;
}
.section-head {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  align-items: center;
  justify-content: space-between;
}
.section-head span,
.hint,
dd,
p,
small {
  overflow-wrap: anywhere;
}
.eyebrow {
  color: #60746a;
  font-size: 12px;
  letter-spacing: 0.08em;
  margin: 0 0 6px;
}
h2,
h3 {
  margin: 0 0 12px;
}
.notice {
  margin: 0;
  padding: 12px;
  background: #f3f6ec;
  border: 1px solid #d4dec8;
  border-radius: 10px;
  line-height: 1.6;
}
.error {
  color: #942e31;
  background: #fff0ed;
  border-color: #edc5bf;
}
.success {
  color: #27613c;
  background: #edf8ef;
  border-color: #b7d8bf;
}
.catalog-grid,
.form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
  min-width: 0;
}
.card {
  min-width: 0;
  padding: 18px;
  border: 1px solid #dce4df;
  border-radius: 14px;
  background: #fff;
  display: grid;
  gap: 12px;
  align-content: start;
}
.wide {
  grid-column: 1 / -1;
}
form,
fieldset {
  min-width: 0;
  margin: 0;
  display: grid;
  gap: 12px;
}
fieldset {
  border: 0;
  padding: 0;
}
legend {
  margin-bottom: 10px;
  font-weight: 600;
}
label {
  display: grid;
  gap: 7px;
  min-width: 0;
  line-height: 1.45;
  font-size: 14px;
}
input,
select,
textarea {
  width: 100%;
  max-width: 100%;
  min-width: 0;
  border: 1px solid #cbd7d0;
  border-radius: 8px;
  padding: 10px;
  color: inherit;
  background: #fff;
  font: inherit;
}
textarea {
  resize: vertical;
}
button {
  min-width: 0;
  max-width: 100%;
  padding: 10px 14px;
  border: 1px solid #afc4b5;
  border-radius: 8px;
  background: #edf4e9;
  color: #24432e;
  font: inherit;
  cursor: pointer;
  white-space: normal;
  overflow-wrap: anywhere;
}
button:disabled {
  cursor: default;
  opacity: 0.55;
}
.danger {
  color: #8f3033;
  background: #fff0ed;
  border-color: #dfbbb4;
}
.actions {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}
.check {
  display: flex;
  gap: 8px;
  align-items: flex-start;
  padding: 5px 0;
}
.check input {
  width: 18px;
  flex: 0 0 18px;
  margin: 2px 0 0;
}
.hint {
  font-size: 13px;
  color: #63796b;
  line-height: 1.6;
  margin: 0;
}
.version-list {
  display: grid;
  gap: 9px;
  list-style: none;
  padding: 0;
  margin: 0;
}
.version-list button {
  width: 100%;
  text-align: left;
  display: grid;
  gap: 6px;
}
.version-list button.selected {
  border-color: #427c57;
  background: #dfedda;
}
.version-list small {
  color: #597064;
}
dl {
  display: grid;
  grid-template-columns: minmax(100px, 0.5fr) minmax(0, 1fr);
  gap: 9px;
  margin: 0;
  font-size: 13px;
}
dt {
  font-weight: 600;
}
dd {
  min-width: 0;
  margin: 0;
}
details {
  min-width: 0;
}
summary {
  cursor: pointer;
  font-weight: 600;
  line-height: 1.5;
}
details[open] > form,
details[open] > pre {
  margin-top: 12px;
}
pre {
  max-width: 100%;
  min-width: 0;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  word-break: break-word;
  padding: 12px;
  background: #f5f7f5;
  border-radius: 8px;
  font-size: 12px;
  line-height: 1.55;
}
.validation ul {
  padding-left: 22px;
  margin: 0;
  overflow-wrap: anywhere;
}
@media (max-width: 680px) {
  .catalog-grid,
  .form-grid {
    grid-template-columns: minmax(0, 1fr);
  }
  .card {
    padding: 13px;
  }
  dl {
    grid-template-columns: minmax(0, 1fr);
  }
  dd {
    margin-bottom: 7px;
  }
  .actions {
    display: grid;
  }
}
</style>
