<script setup lang="ts">
import BusinessDetails from '@lottery/shared/BusinessDetails.vue'
import { vWriteGuard } from "@lottery/shared";
import { useAdminI18n as useAssociationI18n } from "./i18n";
const associationLocale = useAssociationI18n().locale;
import { AssociationName } from "@lottery/shared";
import { useAdminI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import { vRefreshButton } from '@lottery/shared';
import { vGlobalLoading } from "@lottery/shared";
const displayDate = (value: string | null | undefined) => formatDisplayDateTime(value, displayLocale.value);
import { formatDisplayDateTime } from "@lottery/shared";
const displayLocale = useAdminI18n().locale;
import { vInlineValidation } from '@lottery/shared'
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
import { adminKey, useAdminI18n } from "./i18n";
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
const { t, locale, display: render } = useAdminI18n();
const display = (zh: string | import("@lottery/shared").LocalizedMessage) => {
  if (typeof zh !== "string") return displayPublicError(zh);
  const known: Record<string, string> = {
    "草稿": "Draft", "待审核": "Pending review", "已批准": "Approved", "生效中": "Active", "已过期": "Expired", "已拒绝": "Rejected", "已回滚": "Rolled back",
    "请求失败，请重试。": "Request failed. Please try again.",
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
    "已提交审核，等待审核决定": "Submitted for review; awaiting a decision.", "审批决定已保存，生效状态以服务端返回及期次绑定为准": "Review decision saved. The server response and period binding determine activation.",
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
  if (receipt && known[receipt[1]]) return t('admin.rule.version.receipt', { message: locale.value === "zh-CN" ? receipt[1]! : known[receipt[1]]!, id: receipt[2]! });
  return known[zh] && locale.value === "en" ? known[zh]! : render(zh);
};
const statusLabel = (status: RuleVersionStatus) => display(({ draft: "草稿", pending_review: "待审核", approved: "已批准", active: "生效中", expired: "已过期", rejected: "已拒绝", rolled_back: "已回滚" } as const)[status]);
const playStatus = (value: string) => {
  const keys: Record<string, import("./i18n").AdminTranslationKey> = {
    active: "admin.rule.play.active", inactive: "admin.rule.play.inactive", draft: "admin.rule.play.draft",
    archived: "admin.rule.play.archived", enabled: "admin.rule.play.enabled", disabled: "admin.rule.play.disabled",
  };
  if (!keys[value]) throw new Error("Unsupported play status");
  return t(keys[value]);
};
const templateLabel = (value: string) => {
  const keys: Record<string, import("./i18n").AdminTranslationKey> = {
    "特别号命中": "admin.rule.template.special", "数字直选（三位）": "admin.rule.template.digits",
    "数字特征": "admin.rule.template.features", "排除号码": "admin.rule.template.exclude",
    "号码属性（特别号）": "admin.rule.template.attributes", "M 选 N 全中": "admin.rule.template.mselect",
  };
  if (!keys[value]) throw new Error("Unsupported rule template");
  return t(keys[value]);
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
const error = ref<string | import("@lottery/shared").LocalizedMessage>("");
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
  error.value = publicErrorMessage(cause);
}
function reason(value: string) {
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
    validateEditorDefinition(normalizeEditorDefinition(definition));
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
      "已提交审核，等待审核决定",
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
        <h2>{{ t('admin.copy.8ac737a2') }}</h2>
      </div>
      <span>{{ t('admin.copy.55bdd7a5') }}<AssociationName kind="brand" :id="brandId" :locale="associationLocale" /></span>
    </header>
    <p class="notice">
      {{ t('admin.copy.61b2f577') }}
    </p>
    <p v-if="account.super_admin" class="notice">
      {{ t('admin.copy.3be75e15') }}
    </p>
    <p v-if="error" class="notice error" role="alert">{{ display(error) }}</p>
    <p v-if="notice" class="notice success" role="status">{{ display(notice) }}</p>
    <button v-refresh-button="t('admin.copy.b403c843')"
      v-if="conflict && rights.rulesView && playId"
      type="button"
      :disabled="busy || loadingVersions"
      @click="withGlobalLoading(loadVersions)"
    >
      {{ t('admin.copy.b403c843') }}
    </button>

    <div class="catalog-grid">
      <section class="card">
        <h3>{{ t('admin.copy.047e290f') }}</h3>
        <template v-if="rights.gamesView">
          <button v-refresh-button="loadingGames ? t('admin.copy.4bbccd78') : t('admin.copy.4422e4ac')"
            type="button"
            :disabled="busy || loadingGames"
            @click="withGlobalLoading(loadGames)"
          >
            {{ loadingGames ? t('admin.copy.4bbccd78') : t('admin.copy.4422e4ac') }}
          </button>
          <label
            >{{ t('admin.copy.7492bbfb') }}<select
              :value="gameId"
              :disabled="busy || loadingGames"
              @change="withGlobalLoading(chooseGame, ($event.target as HTMLSelectElement).value)"
            >
              <option value="">{{ t('admin.copy.3aba80cd') }}</option>
              <option v-for="item in games" :key="item.id" :value="item.id">
                {{ item.name }} · <BusinessDetails :value="item.model.model" :locale="associationLocale" />
              </option>
            </select></label
          >
          <p v-if="!loadingGames && !games.length" class="hint empty-state">
            {{ t('admin.copy.396947ce') }}
          </p>
          <label
            >{{ t('admin.copy.728b00ef') }}<select
              :value="playId"
              :disabled="busy || loadingPlays || !gameId"
              @change="withGlobalLoading(choosePlay, ($event.target as HTMLSelectElement).value)"
            >
              <option value="">
                {{ loadingPlays ? t('admin.copy.c198d008') : t('admin.copy.3c1f454d') }}
              </option>
              <option v-for="item in plays" :key="item.id" :value="item.id">
                {{ item.name }} · {{ playStatus(item.status) }}
              </option>
            </select></label
          >
          <p v-if="chosenPlay?.active_version_id" class="hint">
            {{ t('admin.copy.c8e49b92') }}{{ chosenPlay.active_version_id }}
          </p>
        </template>
        <p v-else class="hint">
          {{ t('admin.copy.7af15e08') }}
        </p>
        <form novalidate v-inline-validation
          v-if="rights.rulesView || rights.rulesWrite"
          @submit.prevent="withGlobalLoading(choosePlay, directPlayId, true)"
        >
          <label
            >{{ t('admin.copy.cf817637') }}<input
              v-model.trim="directPlayId"
              required
              :disabled="busy"
              :placeholder="t('admin.copy.fca59717')"
          /></label>
          <button type="submit" :disabled="busy || !directPlayId">
            {{ t('admin.copy.9a17cdc3') }}{{ rights.rulesView ? t('admin.copy.df866903') : t('admin.copy.6603e340') }}
          </button>
        </form>
      </section>
      <section class="card">
        <h3>{{ t('admin.copy.5b0737eb') }}</h3>
        <p v-if="!rights.rulesView" class="hint">
          {{ t('admin.copy.358ca7b1') }}
        </p>
        <template v-else>
          <button v-refresh-button="loadingVersions ? t('admin.copy.86388593') : t('admin.copy.e9176bb1')"
            type="button"
            :disabled="busy || loadingVersions || !playId"
            @click="withGlobalLoading(loadVersions)"
          >
            {{ loadingVersions ? t('admin.copy.86388593') : t('admin.copy.e9176bb1') }}
          </button>
          <p v-if="!playId" class="hint">{{ t('admin.copy.beace858') }}</p>
          <p v-else-if="!loadingVersions && !versions.length" class="hint empty-state">
            {{ t('admin.copy.b9554b92') }}
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
                  >{{ t('admin.rule.version', { version: record.version_no }) }} ·
                  {{ statusLabel(record.status) }}</strong
                ><span
                  >
                  {{
                    record.effect_mode === "immediate"
                      ? t('admin.copy.fc7c7e13')
                      : t('admin.copy.644b87af')
                  }}</span
                ><small
                  >{{ displayDate(record.updated_at) }} · {{ t('admin.copy.6ca804b8') }}
                  <AssociationName :record="record" kind="admin" :id="record.created_by" :locale="associationLocale" /></small
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
          {{ t('admin.copy.d8379e91') }}
        </button>
      </section>
    </div>

    <div v-if="rights.gamesWrite" class="catalog-grid">
      <details class="card">
        <summary>{{ t('admin.copy.a5d6be19') }}</summary>
        <form v-write-guard="busy" novalidate v-inline-validation @submit.prevent="withGlobalLoading(createGame)">
          <fieldset :disabled="busy" class="form-grid">
            <label>{{ t('admin.copy.e00bf3f2') }}<input v-model.trim="gameCreate.code" required /></label
            ><label
              >{{ t('admin.copy.e19e3eeb') }}<input v-model.trim="gameCreate.name" required
            /></label>
            <label class="check wide"
              ><input
                v-model="customGame"
                type="checkbox"
              />{{ t('admin.copy.2a27a6b0') }}</label
            >
            <label v-if="!customGame"
              >{{ t('admin.copy.df284c42') }}<select v-model="gameCreate.template">
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
              >{{ t('admin.copy.800dd59c') }}<input
                v-model.trim="gameCreate.timezone"
                required
                placeholder="Asia/Manila"
            /></label>
            <button
              class="wide"
              type="submit"
              :disabled="customGame && !customGameValid"
            >
              {{ t('admin.copy.ae708a9c') }}
            </button>
          </fieldset>
        </form>
      </details>
      <details class="card">
        <summary>{{ t('admin.copy.4a95e870') }}</summary>
        <form v-write-guard="busy" novalidate v-inline-validation @submit.prevent="withGlobalLoading(createPlay)">
          <fieldset :disabled="busy" class="form-grid">
            <label class="wide"
              >{{ t('admin.copy.20ad4b7f') }}<input v-model.trim="playCreate.gameId" required
            /></label>
            <label>{{ t('admin.copy.e00bf3f2') }}<input v-model.trim="playCreate.code" required /></label
            ><label
              >{{ t('admin.copy.e19e3eeb') }}<input v-model.trim="playCreate.name" required
            /></label>
            <button class="wide" type="submit">{{ t('admin.copy.4a95e870') }}</button>
          </fieldset>
        </form>
      </details>
    </div>

    <section v-if="selected" class="card history">
      <h3>
         {{ statusLabel(selected.status) }}
      </h3>
      <dl>
        <dt>{{ t('admin.copy.dd591447') }}</dt>
        <dd>{{ selected.id }}</dd>
        <dt>{{ t('admin.copy.6ca804b8') }}</dt>
        <dd><AssociationName :record="selected" kind="admin" :id="selected.created_by" :locale="associationLocale" /></dd>
        <dt>{{ t('admin.copy.cad23bbd') }}</dt>
        <dd>
          <AssociationName v-if="selected.reviewed_by" :record="selected" kind="admin" :id="selected.reviewed_by" :locale="associationLocale" /><span v-else>{{ t('admin.copy.d664da69') }}</span> {{ selected.review_comment || t('admin.copy.d664da69') }}
        </dd>
        <dt>{{ t('admin.copy.d48b9a76') }}</dt>
        <dd>
          {{ displayDate(selected.effective_at) }} /
          {{ selected.effective_period_id || t('admin.copy.d664da69') }} /
          {{ selected.effective_sequence ?? t('admin.copy.d664da69') }}
        </dd>
        <dt>{{ t('admin.copy.457e41ad') }}</dt>
        <dd>{{ selected.source_version_id || t('admin.copy.c2b41b33') }}</dd>


      </dl>
      <details>
        <summary>{{ t('admin.copy.6c1e0c40') }}</summary>
        <BusinessDetails :value="selected.definition" :locale="associationLocale" />
      </details>
      <details v-if="!isDraft" class="saved-rule-summary">
        <summary>{{ t('admin.copy.ba00d898') }}</summary>
        <RuleDefinitionEditor
          :key="`${selected.id}:${selected.version}:readonly`"
          :model-value="normalizeEditorDefinition(selected.definition)"
          :disabled="true"
        />
      </details>
      <p v-if="isDraft && !supported" class="notice">
        {{ t('admin.copy.08ce4cbb') }}
      </p>
      <p v-if="isDraft && selected.source_version_id" class="notice">
        {{ t('admin.copy.266c0896') }}
      </p>
      <form v-write-guard="busy" novalidate v-inline-validation
        v-if="isDraft && selected.source_version_id && rights.rulesWrite"
        :aria-label="t('admin.copy.1e8f369f')"
        @submit.prevent="withGlobalLoading(persistDraft)"
      >
        <fieldset :disabled="busy">
          <legend>{{ t('admin.copy.999dd607') }}</legend>
          <label
            >{{ t('admin.copy.1575ab32') }}<select v-model="effectMode">
              <option value="immediate">{{ t('admin.copy.5f1eae67') }}</option>
              <option value="next_period">{{ t('admin.copy.644b87af') }}</option>
            </select></label
          >
          <button type="submit">{{ t('admin.copy.7ddfaee5') }}</button>
        </fieldset>
      </form>
      <p v-if="!isDraft" class="hint">
        {{ t('admin.copy.7922957d') }}
      </p>
    </section>

    <section v-if="editable" class="card editor">
      <h3>{{ t('admin.copy.eaf6fa57') }}{{ newDraft ? t('admin.copy.d8d699fe') : t('admin.copy.651bc11e') }}</h3>
      <label
        >{{ t('admin.copy.7ff89358') }}<select
          :value="editorMode"
          :disabled="busy || !rights.rulesWrite || definitionLocked"
          @change="
            changeEditorMode(
              ($event.target as HTMLSelectElement).value as
                'template' | 'advanced',
            )
          "
        >
          <option value="template">{{ t('admin.copy.b76c643c') }}</option>
          <option value="advanced">{{ t('admin.copy.32da5226') }}</option>
        </select></label
      >
      <form v-write-guard="busy" novalidate v-inline-validation @submit.prevent="withGlobalLoading(persistDraft)">
        <template v-if="editorMode === 'advanced' && newDraft && !selectedGame">
          <p class="notice">
            {{ t('admin.copy.9eed50b5') }}
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
            >{{ t('admin.copy.a085fe79') }}<select v-model="template" @change="changeTemplate">
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
            >{{ t('admin.copy.87ba003c') }}<select v-model="form.roundingScope">
              <option value="order">{{ t('admin.copy.c8b62866') }}</option>
              <option value="line">{{ t('admin.copy.3a771232') }}</option>
              <option value="tier">{{ t('admin.copy.f19680be') }}</option>
            </select></label
          >
          <label v-if="template === 'features'" class="wide"
            >{{ t('admin.copy.93de1b9f') }}<select v-model="form.conditionJoin">
              <option value="all">{{ t('admin.copy.7674da94') }}</option>
              <option value="any">{{ t('admin.copy.ec366404') }}</option>
            </select></label
          >
          <template v-if="template === 'attributes'"
            ><label
              >{{ t('admin.copy.f94469a6') }}<input
                v-model.trim="form.redNumbers"
                inputmode="numeric" /></label
            ><label
              >{{ t('admin.copy.5ef620d2') }}<input
                v-model.trim="form.blueNumbers"
                inputmode="numeric" /></label
          ></template>
        </fieldset>
        <label v-if="!definitionLocked"
          >{{ t('admin.copy.1575ab32') }}<select
            v-model="effectMode"
            :disabled="busy || !rights.rulesWrite"
          >
            <option value="immediate">{{ t('admin.copy.5f1eae67') }}</option>
            <option value="next_period">{{ t('admin.copy.644b87af') }}</option>
          </select></label
        >
        <p class="hint">
          {{ t('admin.copy.c1e7c9de') }}
        </p>

        <button
          v-if="rights.rulesWrite && !definitionLocked"
          type="submit"
          :disabled="
            busy ||
            (editorMode === 'advanced' &&
              (!advancedValid || !advancedModelValid))
          "
        >
          {{ newDraft ? t('admin.copy.c69bee60') : t('admin.copy.eb05c8d8') }}
        </button>
      </form>

      <form v-write-guard="busy" novalidate v-inline-validation @submit.prevent="withGlobalLoading(validateDraft)">
        <template v-if="editorMode === 'advanced'">
          <RuleCasesEditor
            :key="`${selected?.id ?? 'new'}:${selected?.version ?? 0}:cases`"
            v-model="advancedCases"
            :definition="advancedDefinition"
            :disabled="busy || (!rights.validate && !rights.rulesWrite)"
            @validity="casesValid = $event"
          />

          <button
            v-if="rights.validate && selected"
            type="submit"
            :disabled="busy || dirty || !isDraft || !casesValid"
          >
            {{ t('admin.copy.71e031b3') }}
          </button>
        </template>
        <fieldset
          v-else
          :disabled="busy || (!rights.validate && !rights.rulesWrite)"
          class="form-grid"
        >
          <legend>{{ t('admin.copy.d1199771') }}</legend>
          <label v-if="template === 'special'"
            >{{ t('admin.copy.0302cab7') }}<input
              v-model.trim="form.specialNumbers"
              inputmode="numeric"
              placeholder="7,19,31,43"
          /></label>
          <template v-if="template === 'm-select-n'"
            ><label
              >{{ t('admin.copy.cb08d518') }}<input
                v-model.trim="form.regularNumbers"
                inputmode="numeric" /></label
            ><label
              >{{ t('admin.copy.e7cea210') }}<input
                v-model.trim="form.specialNumbers"
                inputmode="numeric" /></label
          ></template>
          <label v-if="template === 'exclude'"
            >{{ t('admin.copy.acd015ea') }}<input
              v-model.trim="form.excludedNumbers"
              inputmode="numeric"
          /></label>
          <template v-if="template === 'digits'"
            ><label v-for="(_, index) in form.digitCandidates" :key="index"
              >{{ t('admin.rule.case.position', { index: index + 1 }) }}<input
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
              {{ t('admin.copy.d213a808') }}
            </p></template
          >
          <div v-if="template === 'attributes'" class="wide">
            <span>{{ t('admin.copy.5ad7c49f') }}</span
            ><label class="check"
              ><input
                v-model="form.selectedColors"
                type="checkbox"
                value="red"
              />{{ t('admin.copy.4a98e5d6') }}</label
            ><label class="check"
              ><input
                v-model="form.selectedColors"
                type="checkbox"
                value="blue"
              />{{ t('admin.copy.82e76224') }}</label
            >
          </div>
          <label v-if="template === 'digits' || template === 'features'"
            >{{ t('admin.copy.89e6faa8') }}<input
              v-model.trim="form.drawDigits"
              inputmode="numeric"
          /></label>
          <template v-else
            ><label
              >{{ t('admin.copy.fcfeff00') }}<input
                v-model.trim="form.drawRegular"
                inputmode="numeric" /></label
            ><label
              >{{ t('admin.copy.c8d5b8a6') }}<input
                v-model.trim="form.drawSpecial"
                inputmode="numeric" /></label
          ></template>
          <label
            >{{ t('admin.copy.0b249936') }}<input
              v-model.trim="form.multiplier"
              inputmode="numeric"
          /></label>
          <label>{{ t('admin.copy.3c80afee') }}<input v-model.trim="expected.name" required /></label>
          <label
            >{{ t('admin.copy.aa8c440a') }}<input
              v-model.trim="expected.bet"
              inputmode="numeric"
              required
              :placeholder="t('admin.copy.3a1749c0')"
          /></label>
          <label
            >{{ t('admin.copy.25addb19') }}<input
              v-model.trim="expected.prize"
              inputmode="numeric"
              required
              :placeholder="t('admin.copy.3a1749c0')"
          /></label>
          <label
            >{{ t('admin.copy.12b4c538') }}<select v-model="expected.won">
              <option :value="true">{{ t('admin.copy.638b1e06') }}</option>
              <option :value="false">{{ t('admin.copy.5fa501a3') }}</option>
            </select></label
          >

          <button
            v-if="rights.validate && selected"
            class="wide"
            type="submit"
            :disabled="busy || dirty || !isDraft"
          >
            {{ t('admin.copy.71e031b3') }}
          </button>
        </fieldset>
      </form>
      <p v-if="newDraft" class="hint">
        {{ t('admin.copy.2b1cd8a7') }}
      </p>
      <p v-else-if="dirty" class="notice">
        {{ t('admin.copy.c04936db') }}
      </p>
      <p v-else-if="validationInputsChanged" class="hint">
        {{ t('admin.copy.a8a3dedb') }}
      </p>
      <form v-write-guard="busy" novalidate v-inline-validation
        v-if="rights.submit && selected && isDraft"
        @submit.prevent="withGlobalLoading(submitReview)"
      >
        <fieldset :disabled="busy">

          <button type="submit" :disabled="!readyToSubmit">
            {{ t('admin.copy.fdf143f8') }}
          </button>
        </fieldset>
      </form>
    </section>

    <section
      v-if="validationDisplay"
      class="card validation"
      aria-live="polite"
    >
      <h3>{{ t('admin.copy.a03178f3') }}{{ validationDisplay.passed ? t('admin.copy.14385fca') : t('admin.copy.f49253c3') }}</h3>

      <ul v-if="validationDisplay.warnings?.length">
        <li v-for="(warning, index) in validationDisplay.warnings" :key="index">
          {{ warning }}
        </li>
      </ul>
      <p v-else class="hint">{{ t('admin.copy.f91f449b') }}</p>
      <div
        v-for="(report, index) in validationDisplay.cases"
        :key="index"
        class="case-report"
      >
        <h4>
          {{ report.name }} · {{ report.matched ? t('admin.copy.89d58cc9') : t('admin.copy.e90f3382') }}
        </h4>
        <dl>
          <dt>{{ t('admin.copy.c1576e5d') }}</dt>
          <dd>
            {{ report.expected_bet_points }} / {{ report.actual_bet_points }}
          </dd>
          <dt>{{ t('admin.copy.f15611ea') }}</dt>
          <dd>
            {{ report.expected_prize_points }} /
            {{ report.actual_prize_points }}
          </dd>
          <dt>{{ t('admin.copy.738302ac') }}</dt>
          <dd>
            {{ report.expected_won ? t('admin.copy.1a264f45') : t('admin.copy.b92bb6e0') }} /
            {{ report.actual_won ? t('admin.copy.1a264f45') : t('admin.copy.b92bb6e0') }}
          </dd>
        </dl>
      </div>
      <ul v-if="validationDisplay.findings?.length">
        <li v-for="(finding, index) in validationDisplay.findings" :key="index">
          {{ finding.blocking ? t('admin.copy.76443d08') : t('admin.copy.d1d73b39') }} · {{ finding.code }}：{{
            finding.message
          }}
        </li>
      </ul>
      <details>
        <summary>{{ t('admin.copy.944f2a69') }}</summary>
        <BusinessDetails :value="{
            cases: validationDisplay.cases,
            findings: validationDisplay.findings,
          }" :locale="associationLocale" />
      </details>
    </section>

    <section v-if="selected?.status === 'pending_review'" class="card review">
      <h3>{{ t('admin.copy.d1c53498') }}</h3>
      <p class="notice">
        {{ t('admin.copy.6fc67a87') }}
      </p>
      <p v-if="!reviewAllowed" class="hint">
        {{ t('admin.copy.736e1a91') }}
      </p>
      <form v-write-guard="busy" novalidate v-inline-validation v-else @submit.prevent="withGlobalLoading(decide, true)">
        <fieldset :disabled="busy">

          <label class="check"
            ><input
              v-model="review.acknowledged"
              type="checkbox"
            />{{ t('admin.copy.1c171617') }}</label
          >
          <div class="actions">
            <button type="submit" :disabled="!review.acknowledged">
              {{ t('admin.copy.d75a2d5c') }}</button
            ><button
              v-write-guard="busy"
              type="button"
              class="button button-danger"
              @click="withGlobalLoading(decide, false)"
            >
              {{ t('admin.copy.d7f578ff') }}
            </button>
          </div>
        </fieldset>
      </form>
    </section>
    <form v-write-guard="busy" novalidate v-inline-validation
      v-if="selected && cloneAllowed"
      class="card"
      @submit.prevent="withGlobalLoading(cloneVersion)"
    >
      <fieldset :disabled="busy" class="form-grid">
        <legend>{{ t('admin.copy.0b157d4c') }}</legend>
        <label
          >{{ t('admin.copy.20d539bb') }}<select v-model="clone.effectMode">
            <option value="immediate">{{ t('admin.copy.5f1eae67') }}</option>
            <option value="next_period">{{ t('admin.copy.644b87af') }}</option>
          </select></label
        >
        <button class="wide" type="submit">{{ t('admin.copy.4108acba') }}</button>
      </fieldset>
    </form>
    <p v-global-loading v-if="busy" role="status" class="hint">{{ t('admin.copy.229230c6') }}</p>
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
  background: var(--surface);
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
  background: var(--surface);
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
.button-danger {
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
