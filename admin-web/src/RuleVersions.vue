<script setup lang="ts">
import { computed, ref, watch } from "vue";
import RuleDefinitionEditor from "./RuleDefinitionEditor.vue";
import RuleModelEditor from "./RuleModelEditor.vue";
import RuleCasesEditor from "./RuleCasesEditor.vue";
import {
  defaultRuleDefinition,
  normalizeEditorDefinition,
  validateEditorDefinition,
  validateEditorModel,
} from "./rule-editor";
import { defaultRuleCase, validateRuleCases } from "./rule-cases-editor";
import { AdminApiError, type AdminAccount } from "./admin-api";
import { useAdminI18n } from "./i18n";
import {
  buildRuleSimulationRequest,
  RULE_TEMPLATE_LABELS,
  type RuleSimulationForm,
  type RuleTemplate,
  type RuleDefinition,
  type RuleModel,
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
  type RuleValidationCase,
} from "./rule-versions-api";

const props = defineProps<{ account: AdminAccount; brandId: string }>();
const { t } = useAdminI18n();
const ui = (zh: string, en: string = zh) => t(zh, en);
const display = (zh: string) => {
  const known: Record<string, string> = {
    "草稿": "Draft", "待审核": "Pending review", "已批准": "Approved", "生效中": "Active", "已过期": "Expired", "已拒绝": "Rejected", "已回滚": "Rolled back",
    "请求失败，请重试。": "Request failed. Please try again.", "每次操作都必须填写原因。": "A reason is required for every operation.",
    "响应品牌与当前品牌不匹配，请重新读取。": "The response does not match the current brand. Reload and try again.",
    "玩法响应不属于当前彩种。": "The play response does not belong to the selected game.", "版本响应不属于当前玩法。": "The version response does not belong to the selected play.",
    "当前通用定义不能无损还原为快捷模板，请继续使用通用编辑器；不会覆盖已有条件。": "This generic definition cannot be losslessly converted to a quick template. Continue with the visual editor; existing conditions will not be overwritten.",
    "请求超过 16 KiB 上限，请缩减本次规则或验证用例；未发送请求。": "The request exceeds the 16 KiB limit. Reduce the rule or validation cases; no request was sent.",
    "保存结果不属于当前玩法，请重新读取。": "The save result does not belong to the current play. Reload and try again.", "请填写彩种代码和名称。": "Enter a game code and name.",
    "自定义彩种有未解析或无效输入，请先修正。": "The custom game contains incomplete or invalid input. Fix it first.",
    "请填写彩种 ID、玩法代码和名称。": "Enter the game ID, play code, and name.", "创建结果不属于指定彩种。": "The creation result does not belong to the specified game.",
    "规则有未解析或无效输入，请先修正。": "The rule contains incomplete or invalid input. Fix it first.", "规则模型与当前彩种不一致，请选择同模型模板。": "The rule model does not match the current game. Choose a template with the same model.",
    "规则配置已修改，请先保存草稿，再验证保存的定义。": "The rule configuration has changed. Save the draft before validating the saved definition.",
    "验证用例有未解析或无效输入，请先修正。": "The validation cases contain incomplete or invalid input. Fix them first.", "批准前请确认已阅读验证结果和全部警告。": "Confirm that you have reviewed the validation results and all warnings before approving.",
    "彩种已创建": "Game created.", "玩法已创建": "Play created.", "草稿已保存，尚未提交审核": "Draft saved; it has not been submitted for review.",
    "草稿修改已保存，请重新验证": "Draft changes saved. Validate again.", "服务端验证结果已保存，请检查通过状态与警告": "Server validation results saved. Check the pass status and warnings.",
    "已提交审核，等待其他品牌管理员决定": "Submitted for review; waiting for another brand administrator.", "审批决定已保存，生效状态以服务端返回及期次绑定为准": "Review decision saved. The server response and period binding determine activation.",
    "已拒绝，原定义保留在历史中": "Rejected; the original definition remains in history.", "已克隆为新草稿，来源版本保持不变": "Cloned as a new draft; the source version is unchanged.",
    "版本或状态冲突，请重新读取历史确认最新状态，再重新操作。": "Version or status conflict. Reload history to confirm the latest state before trying again.",
    "单位积分（正整数）": "Unit points (positive whole number)", "赔率（正十进制数，最多 6 位小数）": "Odds (positive decimal, up to 6 decimal places)",
    "中奖封顶积分（正整数，空白不限）": "Prize cap points (positive whole number; blank for unlimited)", "投注限额积分（正整数，空白不限）": "Bet limit points (positive whole number; blank for unlimited)",
    "三同号（0 否 / 1 是；空白不启用）": "Three of a kind (0 no / 1 yes; blank to disable)", "首尾同号（0 否 / 1 是；空白不启用）": "First and last match (0 no / 1 yes; blank to disable)",
    "奇数个数（0–3；空白不启用）": "Odd number count (0–3; blank to disable)", "和值（0–27；空白不启用）": "Sum (0–27; blank to disable)",
  };
  // The immutable receipt ID is business data; translate only our known
  // action caption, keeping the ID byte-for-byte unchanged.
  const receipt = /^(.+)（ID：([0-9a-f-]{36})）。$/.exec(zh);
  if (receipt && known[receipt[1]]) return ui(zh, `${known[receipt[1]]} (ID: ${receipt[2]}).`);
  return ui(zh, known[zh] ?? zh);
};
const statusLabel = (status: RuleVersionStatus) => display(({ draft: "草稿", pending_review: "待审核", approved: "已批准", active: "生效中", expired: "已过期", rejected: "已拒绝", rolled_back: "已回滚" } as const)[status]);
const playStatus = (value: string) => {
  const known: Record<string, string> = { active: "Active", inactive: "Inactive", draft: "Draft", archived: "Archived", enabled: "Enabled", disabled: "Disabled" };
  return known[value] ? ui(value, known[value]) : value;
};
const templateLabel = (value: string) => {
  const names: Record<string, string> = { "特别号命中": "Special-number match", "数字直选（三位）": "Straight three-digit play", "数字特征": "Digit features", "排除号码": "Excluded numbers", "号码属性（特别号）": "Number attributes (special number)", "M 选 N 全中": "M-choose-N all matched" };
  return ui(value, names[value] ?? value);
};
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
const initialModel = () =>
  buildRuleSimulationRequest("special", defaultRuleVersionForm("special"))
    .definition.model;
const editorMode = ref<"template" | "advanced">("template");
const advancedDefinition = ref<RuleDefinition>(
  defaultRuleDefinition(initialModel()),
);
const advancedValid = ref(true);
const advancedModelValid = ref(true);
const advancedCases = ref<RuleValidationCase[]>([]);
const casesValid = ref(true);
const customGame = ref(false);
const customGameModel = ref<RuleModel>(initialModel());
const customGameValid = ref(true);
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
const editable = computed(() => newDraft.value || isDraft.value);
const dirty = computed(() => {
  if (!selected.value) return false;
  try {
    return (
      effectMode.value !== selected.value.effect_mode ||
      ruleDefinitionSignature(
        editorMode.value === "advanced"
          ? advancedDefinition.value
          : buildRuleSimulationRequest(template.value, form.value).definition,
      ) !== ruleDefinitionSignature(selected.value.definition) ||
      (editorMode.value === "advanced" &&
        (!advancedValid.value || !advancedModelValid.value))
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
    (editorMode.value !== "advanced" || casesValid.value) &&
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
      editorMode.value,
      advancedDefinition.value,
      advancedCases.value,
      advancedValid.value,
      advancedModelValid.value,
      casesValid.value,
      customGame.value,
      customGameModel.value,
      customGameValid.value,
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
    editorMode.value = "template";
    advancedDefinition.value = defaultRuleDefinition(initialModel());
    advancedCases.value = [];
    advancedValid.value = true;
    advancedModelValid.value = true;
    casesValid.value = true;
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
    // A newly created/catalogued play has an authoritative game association.
    // Preserve that model for custom drafts instead of falling back to 6+1.
    const known = plays.value.find((item) => item.id === id.trim());
    if (known && games.value.some((item) => item.id === known.game_id))
      gameId.value = known.game_id;
    else {
      gameId.value = "";
      plays.value = [];
    }
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
    editorMode.value =
      !restored || (preserveInputs && editorMode.value === "advanced")
        ? "advanced"
        : "template";
    advancedDefinition.value = normalizeEditorDefinition(record.definition);
    if (!preserveInputs) {
      const storedCases =
        record.validation?.cases.flatMap((item) =>
          item.input ? [item.input] : [],
        ) ?? [];
      advancedCases.value = storedCases.length
        ? (JSON.parse(JSON.stringify(storedCases)) as RuleValidationCase[])
        : [defaultRuleCase(advancedDefinition.value)];
      casesValid.value = true;
    }
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
    const templateDefinition = buildRuleSimulationRequest(
      template.value,
      form.value,
    ).definition;
    advancedDefinition.value = defaultRuleDefinition(
      selectedGame.value?.model ?? templateDefinition.model,
    );
    advancedCases.value = [defaultRuleCase(advancedDefinition.value)];
    editorMode.value =
      ruleDefinitionSignature(
        defaultRuleDefinition(templateDefinition.model),
      ) === ruleDefinitionSignature(advancedDefinition.value)
        ? "template"
        : "advanced";
  });
  error.value = "";
  notice.value = "";
}
function changeTemplate() {
  form.value = defaultRuleVersionForm(template.value);
}
function changeEditorMode(mode: "template" | "advanced") {
  if (
    busy.value ||
    definitionLocked.value ||
    !rights.value.rulesWrite ||
    mode === editorMode.value
  )
    return;
  try {
    if (mode === "advanced") {
      advancedDefinition.value = normalizeEditorDefinition(
        buildRuleSimulationRequest(template.value, form.value).definition,
      );
      const storedCases =
        selected.value?.validation?.cases.flatMap((item) =>
          item.input ? [item.input] : [],
        ) ?? [];
      advancedCases.value = storedCases.length
        ? (JSON.parse(JSON.stringify(storedCases)) as RuleValidationCase[])
        : [defaultRuleCase(advancedDefinition.value)];
    } else {
      const restored = restoreRuleVersionTemplate(advancedDefinition.value);
      if (!restored)
        throw new Error(
          "当前通用定义不能无损还原为快捷模板，请继续使用通用编辑器；不会覆盖已有条件。",
        );
      template.value = restored.template;
      form.value = restored.form;
    }
    editorMode.value = mode;
    validationInputsChanged.value = true;
    validationDisplay.value = null;
  } catch (cause) {
    showError(cause);
  }
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
  if (new TextEncoder().encode(JSON.stringify(body)).length > 16 * 1024) {
    showError(
      new Error("请求超过 16 KiB 上限，请缩减本次规则或验证用例；未发送请求。"),
    );
    return;
  }
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
    const model = customGame.value
      ? customGameModel.value
      : buildRuleSimulationRequest(
          input.template,
          defaultRuleVersionForm(input.template),
        ).definition.model;
    if (customGame.value && !customGameValid.value)
      throw new Error("自定义彩种有未解析或无效输入，请先修正。");
    validateEditorModel(model);
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
      : editorMode.value === "advanced"
        ? advancedDefinition.value
        : buildRuleSimulationRequest(template.value, form.value).definition;
    if (
      editorMode.value === "advanced" &&
      (!advancedValid.value || !advancedModelValid.value)
    )
      throw new Error("规则有未解析或无效输入，请先修正。");
    validateEditorDefinition(definition);
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
        (result) => applyVersion(result, editorMode.value === "advanced"),
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
    if (!record || !isDraft.value) return;
    if (dirty.value)
      throw new Error("规则配置已修改，请先保存草稿，再验证保存的定义。");
    const body = {
      version: record.version,
      cases:
        editorMode.value === "advanced"
          ? advancedCases.value
          : [
              buildRuleValidationCase(
                template.value,
                form.value,
                expected.value,
              ),
            ],
      reason: reason(draftReason.value),
    };
    if (editorMode.value === "advanced" && !casesValid.value)
      throw new Error("验证用例有未解析或无效输入，请先修正。");
    validateRuleCases(record.definition, body.cases);
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
  [template, form, effectMode, expected, advancedDefinition, advancedCases],
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
    customGame.value = false;
    customGameModel.value = initialModel();
    customGameValid.value = true;
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
        <p class="eyebrow">{{ t("规则版本管理", "Rule version management") }}</p>
        <h2>{{ t("彩种、玩法与规则审批", "Games, plays, and rule approvals") }}</h2>
      </div>
      <span>{{ t("品牌 · ", "Brand · ") }}{{ brandId || t("未选择", "Not selected") }}</span>
    </header>
    <p class="notice">
      {{ t("所有保存、验证和审批均调用真实接口。提交审核后定义冻结，创建者和所有编辑者均不能审核该版本。权限及生效状态由服务端最终判定。", "All saves, validations, and reviews use live endpoints. After submission for review, the definition is frozen; neither its creator nor any editor may review it. The server makes the final decision on permissions and activation status.") }}
    </p>
    <p v-if="account.super_admin" class="notice">
      {{ t("超级管理员仅可按显式读取权限查看，不可创建、修改、验证、提交或审批。", "Super administrators may view only with explicit read permission; they cannot create, edit, validate, submit, or review.") }}
    </p>
    <p v-if="error" class="notice error" role="alert">{{ display(error) }}</p>
    <p v-if="notice" class="notice success" role="status">{{ display(notice) }}</p>
    <button
      v-if="conflict && rights.rulesView && playId"
      type="button"
      :disabled="busy || loadingVersions"
      @click="loadVersions()"
    >
      {{ t("重新读取版本历史（将清除未保存输入）", "Reload version history (unsaved input will be cleared)") }}
    </button>

    <div class="catalog-grid">
      <section class="card">
        <h3>{{ t("1 · 选择彩种和玩法", "1 · Select a game and play") }}</h3>
        <template v-if="rights.gamesView">
          <button
            type="button"
            :disabled="busy || loadingGames"
            @click="loadGames"
          >
            {{ loadingGames ? t("读取彩种中…", "Loading games…") : t("刷新彩种", "Refresh games") }}
          </button>
          <label
            >{{ t("彩种", "Game") }}<select
              :value="gameId"
              :disabled="busy || loadingGames"
              @change="chooseGame(($event.target as HTMLSelectElement).value)"
            >
              <option value="">{{ t("请选择彩种", "Select a game") }}</option>
              <option v-for="item in games" :key="item.id" :value="item.id">
                {{ item.name }} · {{ item.code }} · {{ item.model.model }}
              </option>
            </select></label
          >
          <p v-if="!loadingGames && !games.length" class="hint">
            {{ t("当前品牌没有可显示的彩种。", "No games are available for this brand.") }}
          </p>
          <label
            >{{ t("玩法", "Play") }}<select
              :value="playId"
              :disabled="busy || loadingPlays || !gameId"
              @change="choosePlay(($event.target as HTMLSelectElement).value)"
            >
              <option value="">
                {{ loadingPlays ? t("读取玩法中…", "Loading plays…") : t("请选择玩法", "Select a play") }}
              </option>
              <option v-for="item in plays" :key="item.id" :value="item.id">
                {{ item.name }} · {{ item.code }} · {{ playStatus(item.status) }}
              </option>
            </select></label
          >
          <p v-if="chosenPlay?.active_version_id" class="hint">
            {{ t("当前生效版本 ID：", "Active version ID: ") }}{{ chosenPlay.active_version_id }}
          </p>
        </template>
        <p v-else class="hint">
          {{ t("缺少 game.view.brand/platform，不能读取彩种或玩法目录。写权限不自动包含读取权限。", "Missing game.view.brand/platform permission. The game and play catalogs cannot be loaded. Write permission does not include read permission.") }}
        </p>
        <form
          v-if="rights.rulesView || rights.rulesWrite"
          @submit.prevent="choosePlay(directPlayId, true)"
        >
          <label
            >{{ t("直接指定玩法 ID", "Enter play ID directly") }}<input
              v-model.trim="directPlayId"
              required
              :disabled="busy"
              :placeholder="t('玩法 UUID', 'Play UUID')"
          /></label>
          <button type="submit" :disabled="busy || !directPlayId">
            {{ t("选择玩法", "Select play") }}{{ rights.rulesView ? t("并读取历史", " and load history") : t("以创建草稿", " to create a draft") }}
          </button>
        </form>
      </section>
      <section class="card">
        <h3>{{ t("2 · 版本历史", "2 · Version history") }}</h3>
        <p v-if="!rights.rulesView" class="hint">
          {{ t("缺少 rule.view.brand/platform，不能读取版本历史。其他步骤的权限不自动包含读取权限。", "Missing rule.view.brand/platform permission. Version history cannot be loaded; permissions for other steps do not include read access.") }}
        </p>
        <template v-else>
          <button
            type="button"
            :disabled="busy || loadingVersions || !playId"
            @click="loadVersions()"
          >
            {{ loadingVersions ? t("读取历史中…", "Loading history…") : t("刷新版本历史", "Refresh version history") }}
          </button>
          <p v-if="!playId" class="hint">{{ t("先选择玩法。", "Select a play first.") }}</p>
          <p v-else-if="!loadingVersions && !versions.length" class="hint">
            {{ t("当前玩法没有可显示的版本。", "No versions are available for this play.") }}
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
                  >{{ ui(`第 ${record.version_no} 版`, `Version ${record.version_no}`) }} ·
                  {{ statusLabel(record.status) }}</strong
                ><span
                  >CAS {{ record.version }} ·
                  {{
                    record.effect_mode === "immediate"
                      ? t("立即生效", "Effective immediately")
                      : t("下一期生效", "Effective next period")
                  }}</span
                ><small
                  >{{ record.updated_at }} · {{ t("创建者", "Created by") }}
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
          {{ t("新建规则草稿", "Create rule draft") }}
        </button>
      </section>
    </div>

    <div v-if="rights.gamesWrite" class="catalog-grid">
      <details class="card">
        <summary>{{ t("创建彩种（模板或自定义号码模型）", "Create game (template or custom number model)") }}</summary>
        <form @submit.prevent="createGame">
          <fieldset :disabled="busy" class="form-grid">
            <label>{{ t("代码", "Code") }}<input v-model.trim="gameCreate.code" required /></label
            ><label
              >{{ t("名称", "Name") }}<input v-model.trim="gameCreate.name" required
            /></label>
            <label class="check wide"
              ><input
                v-model="customGame"
                type="checkbox"
              />{{ t("自定义彩种号码模型（任意合法数量与号码池）", "Custom game number model (any valid counts and number pools)") }}</label
            >
            <label v-if="!customGame"
              >{{ t("模型模板", "Model template") }}<select v-model="gameCreate.template">
                <option
                  v-for="[value, label] in templates"
                  :key="value"
                  :value="value"
                >
                  {{ templateLabel(label) }} · {{ ruleTemplateModel(value) }}
                </option>
              </select></label
            >
            <RuleModelEditor
              v-if="customGame"
              :key="brandId"
              class="wide"
              v-model="customGameModel"
              :disabled="busy"
              @validity="customGameValid = $event"
            />
            <label
              >{{ t("IANA 时区", "IANA time zone") }}<input
                v-model.trim="gameCreate.timezone"
                required
                placeholder="Asia/Manila"
            /></label>
            <label class="wide"
              >{{ t("创建原因", "Creation reason") }}<textarea
                v-model="gameCreate.reason"
                required
                rows="2"
              /></label
            ><button
              class="wide"
              type="submit"
              :disabled="customGame && !customGameValid"
            >
              {{ t("创建彩种", "Create game") }}
            </button>
          </fieldset>
        </form>
      </details>
      <details class="card">
        <summary>{{ t("创建玩法", "Create play") }}</summary>
        <form @submit.prevent="createPlay">
          <fieldset :disabled="busy" class="form-grid">
            <label class="wide"
              >{{ t("彩种 ID", "Game ID") }}<input v-model.trim="playCreate.gameId" required
            /></label>
            <label>{{ t("代码", "Code") }}<input v-model.trim="playCreate.code" required /></label
            ><label
              >{{ t("名称", "Name") }}<input v-model.trim="playCreate.name" required
            /></label>
            <label class="wide"
              >{{ t("创建原因", "Creation reason") }}<textarea
                v-model="playCreate.reason"
                required
                rows="2"
              /></label
            ><button class="wide" type="submit">{{ t("创建玩法", "Create play") }}</button>
          </fieldset>
        </form>
      </details>
    </div>

    <section v-if="selected" class="card history">
      <h3>
        {{ ui(`第 ${selected.version_no} 版`, `Version ${selected.version_no}`) }} · {{ statusLabel(selected.status) }}
      </h3>
      <dl>
        <dt>{{ t("版本 ID / CAS", "Version ID / CAS") }}</dt>
        <dd>{{ selected.id }} / {{ selected.version }}</dd>
        <dt>{{ t("创建者", "Created by") }}</dt>
        <dd>{{ selected.created_by }}</dd>
        <dt>{{ t("审核者 / 意见", "Reviewer / comment") }}</dt>
        <dd>
          {{ selected.reviewed_by || t("尚无", "None yet") }} /
          {{ selected.review_comment || t("尚无", "None yet") }}
        </dd>
        <dt>{{ t("生效时间 / 期次 / 序号", "Effective time / period / sequence") }}</dt>
        <dd>
          {{ selected.effective_at || t("尚无", "None yet") }} /
          {{ selected.effective_period_id || t("尚无", "None yet") }} /
          {{ selected.effective_sequence ?? t("尚无", "None yet") }}
        </dd>
        <dt>{{ t("来源版本", "Source version") }}</dt>
        <dd>{{ selected.source_version_id || t("无", "None") }}</dd>
        <dt v-if="selected.audit_log_id">{{ t("审计 ID", "Audit ID") }}</dt>
        <dd v-if="selected.audit_log_id">{{ selected.audit_log_id }}</dd>
      </dl>
      <details>
        <summary>{{ t("已保存规则定义（只读 JSON）", "Saved rule definition (read-only JSON)") }}</summary>
        <pre>{{ pretty(selected.definition) }}</pre>
      </details>
      <details v-if="!isDraft" class="saved-rule-summary">
        <summary>{{ t("已保存规则可视化摘要（只读）", "Saved visual rule summary (read-only)") }}</summary>
        <RuleDefinitionEditor
          :key="`${selected.id}:${selected.version}:readonly`"
          :model-value="normalizeEditorDefinition(selected.definition)"
          :disabled="true"
        />
      </details>
      <p v-if="isDraft && !supported" class="notice">
        {{ t("当前定义使用通用可视化编辑器完整保留。普通草稿可以编辑；回滚草稿定义保持锁定，不会用快捷模板覆盖原定义。", "The definition is fully preserved in the visual editor. Regular drafts can be edited; rollback draft definitions stay locked and are never overwritten by a quick template.") }}
      </p>
      <p v-if="isDraft && selected.source_version_id" class="notice">
        {{ t("这是回滚草稿，定义必须与来源版本完全相同，只允许调整生效方式。仍需重新验证，并由未参与创建或编辑的其他管理员审核。", "This is a rollback draft. Its definition must exactly match the source version; only the effective mode can change. It still requires validation and review by an administrator who did not create or edit it.") }}
      </p>
      <form
        v-if="isDraft && selected.source_version_id && rights.rulesWrite"
        :aria-label="t('回滚草稿生效方式', 'Rollback draft effective mode')"
        @submit.prevent="persistDraft"
      >
        <fieldset :disabled="busy">
          <legend>{{ t("回滚草稿 · 仅调整生效方式", "Rollback draft · change effective mode only") }}</legend>
          <label
            >{{ t("生效方式", "Effective mode") }}<select v-model="effectMode">
              <option value="immediate">{{ t("批准后立即生效", "Effective immediately after approval") }}</option>
              <option value="next_period">{{ t("下一期生效", "Effective next period") }}</option>
            </select></label
          >
          <label
            >{{ t("变更原因", "Change reason") }}<textarea v-model="draftReason" required rows="2" /></label
          ><button type="submit">{{ t("仅保存生效方式", "Save effective mode only") }}</button>
        </fieldset>
      </form>
      <p v-if="!isDraft" class="hint">
        {{ t("该版本不可修改。需要变更时可克隆为新草稿，重新验证和提交审批。", "This version cannot be edited. Clone it as a new draft to make changes, then validate and submit it for review.") }}
      </p>
    </section>

    <section v-if="editable" class="card editor">
      <h3>{{ t("3 · ", "3 · ") }}{{ newDraft ? t("新建草稿", "New draft") : t("草稿配置与验证", "Draft configuration and validation") }}</h3>
      <label
        >{{ t("编辑方式", "Editor mode") }}<select
          :value="editorMode"
          :disabled="busy || !rights.rulesWrite || definitionLocked"
          @change="
            changeEditorMode(
              ($event.target as HTMLSelectElement).value as
                'template' | 'advanced',
            )
          "
        >
          <option value="template">{{ t("六个快捷模板", "Six quick templates") }}</option>
          <option value="advanced">{{ t("通用可视化编辑器", "Visual editor") }}</option>
        </select></label
      >
      <form @submit.prevent="persistDraft">
        <template v-if="editorMode === 'advanced' && newDraft && !selectedGame">
          <p class="notice">
            {{ t("未读取目标彩种模型。请明确配置与目标彩种完全一致的模型；服务器会核对，不会修改彩种。", "The target game's model has not been loaded. Configure a model that exactly matches the target; the server will verify it and will not modify the game.") }}
          </p>
          <RuleModelEditor
            v-model="advancedDefinition.model"
            :disabled="busy || !rights.rulesWrite"
            @validity="advancedModelValid = $event"
          />
        </template>
        <RuleDefinitionEditor
          v-if="editorMode === 'advanced'"
          :key="`${selected?.id ?? 'new'}:${selected?.version ?? 0}:definition`"
          v-model="advancedDefinition"
          :disabled="busy || !rights.rulesWrite || definitionLocked"
          @validity="advancedValid = $event"
        />
        <fieldset
          v-else
          :disabled="busy || !rights.rulesWrite || definitionLocked"
          class="form-grid"
        >
          <label class="wide"
            >{{ t("模板", "Template") }}<select v-model="template" @change="changeTemplate">
              <option
                v-for="[value, label] in templates"
                :key="value"
                :value="value"
                :disabled="
                  !!selectedGame &&
                  ruleTemplateModel(value) !== selectedGame.model.model
                "
              >
                {{ templateLabel(label) }}
              </option>
            </select></label
          >
          <label v-for="input in commonInputs" :key="input.key"
            >{{ display(input.label)
            }}<input
              v-model.trim="form[input.key]"
              :inputmode="input.key === 'odds' ? 'decimal' : 'numeric'"
          /></label>
          <label
            >{{ t("舍入范围", "Rounding scope") }}<select v-model="form.roundingScope">
              <option value="order">{{ t("整注汇总后舍入", "Round after summing the order") }}</option>
              <option value="line">{{ t("每组合舍入后汇总", "Round each combination, then sum") }}</option>
              <option value="tier">{{ t("每奖级舍入后汇总", "Round each prize tier, then sum") }}</option>
            </select></label
          >
          <label v-if="template === 'features'" class="wide"
            >{{ t("特征条件关系", "Feature condition join") }}<select v-model="form.conditionJoin">
              <option value="all">{{ t("全部满足（AND）", "All must match (AND)") }}</option>
              <option value="any">{{ t("任一满足（OR）", "Any may match (OR)") }}</option>
            </select></label
          >
          <template v-if="template === 'attributes'"
            ><label
              >{{ t("红色号码映射", "Red-number mapping") }}<input
                v-model.trim="form.redNumbers"
                inputmode="numeric" /></label
            ><label
              >{{ t("蓝色号码映射", "Blue-number mapping") }}<input
                v-model.trim="form.blueNumbers"
                inputmode="numeric" /></label
          ></template>
        </fieldset>
        <label v-if="!definitionLocked"
          >{{ t("生效方式", "Effective mode") }}<select
            v-model="effectMode"
            :disabled="busy || !rights.rulesWrite"
          >
            <option value="immediate">{{ t("批准后立即生效", "Effective immediately after approval") }}</option>
            <option value="next_period">{{ t("下一期生效", "Effective next period") }}</option>
          </select></label
        >
        <p class="hint">
          {{ t("积分和倍率使用整数字符串，赔率精确计算。快捷模板倍率上限 1000；通用编辑器可配置倍率、组合和封顶。定义与审核结果以服务端为准，单次请求最多 16 KiB。", "Points and multipliers use integer strings; odds are calculated precisely. Quick templates support multipliers up to 1,000; the visual editor supports configurable multipliers, combinations, and caps. The server is authoritative for definitions and review results. Requests are limited to 16 KiB.") }}
        </p>
        <label v-if="rights.rulesWrite && !definitionLocked"
          >{{ t("保存原因", "Save reason") }}<textarea
            v-model="draftReason"
            required
            rows="2"
            :disabled="busy"
          />
        </label>
        <button
          v-if="rights.rulesWrite && !definitionLocked"
          type="submit"
          :disabled="
            busy ||
            (editorMode === 'advanced' &&
              (!advancedValid || !advancedModelValid))
          "
        >
          {{ newDraft ? t("保存新草稿", "Save new draft") : t("保存草稿修改", "Save draft changes") }}
        </button>
      </form>

      <form @submit.prevent="validateDraft">
        <template v-if="editorMode === 'advanced'">
          <RuleCasesEditor
            :key="`${selected?.id ?? 'new'}:${selected?.version ?? 0}:cases`"
            v-model="advancedCases"
            :definition="advancedDefinition"
            :disabled="busy || (!rights.validate && !rights.rulesWrite)"
            @validity="casesValid = $event"
          />
          <label v-if="rights.validate && selected"
            >{{ t("验证原因", "Validation reason") }}<textarea
              v-model="draftReason"
              required
              rows="2"
              :disabled="busy"
            />
          </label>
          <button
            v-if="rights.validate && selected"
            type="submit"
            :disabled="busy || dirty || !isDraft || !casesValid"
          >
            {{ t("验证已保存草稿并保存报告", "Validate saved draft and save report") }}
          </button>
        </template>
        <fieldset
          v-else
          :disabled="busy || (!rights.validate && !rights.rulesWrite)"
          class="form-grid"
        >
          <legend>{{ t("完整验证用例 · 输入选号、开奖和独立预期", "Complete validation case · enter selections, draw, and independent expectations") }}</legend>
          <label v-if="template === 'special'"
            >{{ t("特别号候选", "Special-number candidates") }}<input
              v-model.trim="form.specialNumbers"
              inputmode="numeric"
              placeholder="7,19,31,43"
          /></label>
          <template v-if="template === 'm-select-n'"
            ><label
              >{{ t("所选普通号（6 个）", "Selected regular numbers (6)") }}<input
                v-model.trim="form.regularNumbers"
                inputmode="numeric" /></label
            ><label
              >{{ t("所选特别号（1 个）", "Selected special number (1)") }}<input
                v-model.trim="form.specialNumbers"
                inputmode="numeric" /></label
          ></template>
          <label v-if="template === 'exclude'"
            >{{ t("排除号码（逗号分隔，数量也属于规则配置）", "Excluded numbers (comma-separated; the count is part of the rule configuration)") }}<input
              v-model.trim="form.excludedNumbers"
              inputmode="numeric"
          /></label>
          <template v-if="template === 'digits'"
            ><label v-for="(_, index) in form.digitCandidates" :key="index"
              >{{ ui(`第 ${index + 1} 位候选（0–9，逗号分隔）`, `Position ${index + 1} candidates (0–9, comma-separated)`) }}<input
                v-model.trim="form.digitCandidates[index]"
                inputmode="numeric" /></label
          ></template>
          <template v-if="template === 'features'"
            ><label v-for="input in featureInputs" :key="input.key"
              >{{ display(input.label)
              }}<input
                v-model.trim="form.featureValues[input.key]"
                inputmode="numeric"
            /></label>
            <p class="hint wide">
              {{ t("每项仅选一个值。新增或移除特征需先保存草稿，允许值由模板确定。", "Select one value per feature. Save the draft before adding or removing a feature; allowed values are set by the template.") }}
            </p></template
          >
          <div v-if="template === 'attributes'" class="wide">
            <span>{{ t("所选颜色（可多选）", "Selected colors (multiple allowed)") }}</span
            ><label class="check"
              ><input
                v-model="form.selectedColors"
                type="checkbox"
                value="red"
              />{{ t("红", "Red") }}</label
            ><label class="check"
              ><input
                v-model="form.selectedColors"
                type="checkbox"
                value="blue"
              />{{ t("蓝", "Blue") }}</label
            >
          </div>
          <label v-if="template === 'digits' || template === 'features'"
            >{{ t("三位开奖结果", "Three-digit draw") }}<input
              v-model.trim="form.drawDigits"
              inputmode="numeric"
          /></label>
          <template v-else
            ><label
              >{{ t("开奖普通号（6 个）", "Regular draw numbers (6)") }}<input
                v-model.trim="form.drawRegular"
                inputmode="numeric" /></label
            ><label
              >{{ t("开奖特别号（1 个）", "Special draw number (1)") }}<input
                v-model.trim="form.drawSpecial"
                inputmode="numeric" /></label
          ></template>
          <label
            >{{ t("验证倍率（1–1000 整数）", "Validation multiplier (integer, 1–1,000)") }}<input
              v-model.trim="form.multiplier"
              inputmode="numeric"
          /></label>
          <label>{{ t("用例名称", "Case name") }}<input v-model.trim="expected.name" required /></label>
          <label
            >{{ t("预期投注积分", "Expected bet points") }}<input
              v-model.trim="expected.bet"
              inputmode="numeric"
              required
              :placeholder="t('规范非负整数', 'Canonical non-negative integer')"
          /></label>
          <label
            >{{ t("预期中奖积分", "Expected prize points") }}<input
              v-model.trim="expected.prize"
              inputmode="numeric"
              required
              :placeholder="t('规范非负整数', 'Canonical non-negative integer')"
          /></label>
          <label
            >{{ t("预期是否中奖", "Expected win") }}<select v-model="expected.won">
              <option :value="true">{{ t("中奖", "Won") }}</option>
              <option :value="false">{{ t("未中奖", "Not won") }}</option>
            </select></label
          >
          <label v-if="rights.validate && selected" class="wide"
            >{{ t("验证原因", "Validation reason") }}<textarea v-model="draftReason" required rows="2" />
          </label>
          <button
            v-if="rights.validate && selected"
            class="wide"
            type="submit"
            :disabled="busy || dirty || !isDraft"
          >
            {{ t("验证已保存草稿并保存报告", "Validate saved draft and save report") }}
          </button>
        </fieldset>
      </form>
      <p v-if="newDraft" class="hint">
        {{ t("先保存草稿，再调用验证接口；预期结果由运营填写，未调用 API 时不会显示验证成功。", "Save the draft before calling validation. Operators enter the expected results; validation is not shown as successful until the API has been called.") }}
      </p>
      <p v-else-if="dirty" class="notice">
        {{ t("输入与已保存配置不一致，暂不可验证或提交审核。普通草稿需先保存配置；回滚草稿的特征项目和排除数量必须与原定义一致。修改不会自动保存。", "The input differs from the saved configuration, so validation and review submission are unavailable. Save regular drafts first; rollback drafts must retain the source definition's feature set and exclusion count. Changes are not saved automatically.") }}
      </p>
      <p v-else-if="validationInputsChanged" class="hint">
        {{ t("用例输入已变化，已清除旧验证显示，请重新验证。", "Case input changed; the previous validation display was cleared. Validate again.") }}
      </p>
      <form
        v-if="rights.submit && selected && isDraft"
        @submit.prevent="submitReview"
      >
        <fieldset :disabled="busy">
          <label
            >{{ t("提交审核原因", "Review submission reason") }}<textarea v-model="draftReason" required rows="2" />
          </label>
          <button type="submit" :disabled="!readyToSubmit">
            {{ t("提交审核（需服务端验证通过）", "Submit for review (server validation must pass)") }}
          </button>
        </fieldset>
      </form>
    </section>

    <section
      v-if="validationDisplay"
      class="card validation"
      aria-live="polite"
    >
      <h3>{{ t("服务端验证：", "Server validation: ") }}{{ validationDisplay.passed ? t("通过", "Passed") : t("未通过", "Failed") }}</h3>
      <p class="hint">{{ t("定义校验指纹：", "Definition validation fingerprint: ") }}{{ validationDisplay.definition_hash }}</p>
      <ul v-if="validationDisplay.warnings?.length">
        <li v-for="(warning, index) in validationDisplay.warnings" :key="index">
          {{ warning }}
        </li>
      </ul>
      <p v-else class="hint">{{ t("服务端未返回警告。", "The server returned no warnings.") }}</p>
      <div
        v-for="(report, index) in validationDisplay.cases"
        :key="index"
        class="case-report"
      >
        <h4>
          {{ report.name }} · {{ report.matched ? t("符合预期", "Matches expectation") : t("不符合预期", "Does not match expectation") }}
        </h4>
        <dl>
          <dt>{{ t("投注积分 · 预期 / 实际", "Bet points · expected / actual") }}</dt>
          <dd>
            {{ report.expected_bet_points }} / {{ report.actual_bet_points }}
          </dd>
          <dt>{{ t("中奖积分 · 预期 / 实际", "Prize points · expected / actual") }}</dt>
          <dd>
            {{ report.expected_prize_points }} /
            {{ report.actual_prize_points }}
          </dd>
          <dt>{{ t("中奖 · 预期 / 实际", "Win · expected / actual") }}</dt>
          <dd>
            {{ report.expected_won ? t("是", "Yes") : t("否", "No") }} /
            {{ report.actual_won ? t("是", "Yes") : t("否", "No") }}
          </dd>
        </dl>
      </div>
      <ul v-if="validationDisplay.findings?.length">
        <li v-for="(finding, index) in validationDisplay.findings" :key="index">
          {{ finding.blocking ? t("阻断", "Blocking") : t("提示", "Notice") }} · {{ finding.code }}：{{
            finding.message
          }}
        </li>
      </ul>
      <details>
        <summary>{{ t("验证用例和发现（只读）", "Validation cases and findings (read-only)") }}</summary>
        <pre>{{
          pretty({
            cases: validationDisplay.cases,
            findings: validationDisplay.findings,
          })
        }}</pre>
      </details>
    </section>

    <section v-if="selected?.status === 'pending_review'" class="card review">
      <h3>{{ t("4 · 品牌管理员审核", "4 · Brand administrator review") }}</h3>
      <p class="notice">
        {{ t("审批会影响正式规则生效。请逐项阅读定义、验证报告、全部警告和生效方式；服务端会检查审核人是否为创建者或曾参与编辑，以及权限和版本。", "Approval affects the live rule. Review the definition, validation report, all warnings, and effective mode. The server checks whether the reviewer created or edited the version, as well as permissions and version state.") }}
      </p>
      <p v-if="selected.created_by === account.id" class="hint">
        {{ t("你是该版本的创建者，不能批准或拒绝自己的版本，请由另一名具备品牌审核权限的管理员处理。", "You created this version and cannot approve or reject it. Ask another administrator with brand review permission to handle it.") }}
      </p>
      <p v-else-if="!reviewAllowed" class="hint">
        {{ t("当前账号没有本品牌 rule.review.brand 权限，或不具备服务端要求的品牌管理员审核身份。", "This account lacks rule.review.brand permission for this brand or does not meet the server's brand-administrator review requirements.") }}
      </p>
      <form v-else @submit.prevent="decide(true)">
        <fieldset :disabled="busy">
          <label
            >{{ t("审核原因（批准和拒绝均必填）", "Review reason (required for approval and rejection)") }}<textarea
              v-model="review.reason"
              required
              rows="3"
            />
          </label>
          <label class="check"
            ><input
              v-model="review.acknowledged"
              type="checkbox"
            />{{ t("我已阅读验证结果、全部警告及生效范围，确认批准。", "I have reviewed the validation results, all warnings, and the effective scope, and confirm approval.") }}</label
          >
          <div class="actions">
            <button type="submit" :disabled="!review.acknowledged">
              {{ t("批准该版本", "Approve this version") }}</button
            ><button
              type="button"
              class="danger"
              :disabled="!review.reason.trim()"
              @click="decide(false)"
            >
              {{ t("拒绝并记录原因", "Reject and record reason") }}
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
        <legend>{{ t("克隆当前版本为新草稿", "Clone current version as a new draft") }}</legend>
        <label
          >{{ t("新草稿生效方式", "New draft effective mode") }}<select v-model="clone.effectMode">
            <option value="immediate">{{ t("批准后立即生效", "Effective immediately after approval") }}</option>
            <option value="next_period">{{ t("下一期生效", "Effective next period") }}</option>
          </select></label
        >
        <label
          >{{ t("克隆原因", "Clone reason") }}<textarea v-model="clone.reason" required rows="2" /></label
        ><button class="wide" type="submit">{{ t("克隆（来源定义保持不变）", "Clone (source definition remains unchanged)") }}</button>
      </fieldset>
    </form>
    <p v-if="busy" role="status" class="hint">{{ t("请求处理中，请等待服务端结果…", "Request in progress. Please wait for the server result…") }}</p>
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
